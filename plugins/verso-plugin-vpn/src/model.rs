// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

//! The router's tunnels as this page reads them: each OpenVPN instance from
//! its uci section and the helper's read of it, and every other tunnel device
//! someone brought up.

use verso_plugin::{Map, Request, Value};

pub const CONFIG: &str = "openvpn";

/// SAMPLES are the sections OpenWrt's openvpn package ships, each switched
/// off: examples of the config's shape, not anything this router runs. One is
/// listed only once someone turns it on.
const SAMPLES: [&str; 3] = ["custom_config", "sample_server", "sample_client"];

pub struct Vpn {
    pub instances: Vec<Instance>,
    /// Tunnels no OpenVPN instance holds: Tailscale, WireGuard, the rest.
    pub others: Vec<Tunnel>,
}

pub struct Instance {
    pub name: String,
    pub enabled: bool,
    /// The profile file the section names, if it names one.
    pub config: String,
    /// The section's options as uci holds them, for the panel's preview.
    pub values: Map<String, Value>,
    /// The network whose device this instance's tunnel is, if any names it.
    pub network: String,
    pub live: Live,
    pub profile: Option<Profile>,
    /// The profile file as the editor opens it: folded, as it will read once
    /// staged changes are applied, and the version a save replaces.
    pub file: Option<ProfileFile>,
    pub traffic: Option<Traffic>,
}

pub struct ProfileFile {
    pub content: String,
    pub version: String,
}

/// Live is what the helper read of an instance. An empty state is a read the
/// shell did not deliver: unknown, never "stopped".
#[derive(Default)]
pub struct Live {
    pub state: String,
    pub server: String,
    pub address: String,
    pub device: String,
    pub since: Option<u64>,
}

#[derive(Default)]
pub struct Profile {
    pub client: bool,
    pub proto: String,
    pub remotes: Vec<Remote>,
    pub cipher: String,
    pub verify: String,
    pub asks_sign_in: bool,
    pub redirect: bool,
    pub nopull: bool,
    pub blocks: Vec<String>,
    pub text: String,
}

pub struct Remote {
    pub host: String,
    pub port: String,
}

pub struct Traffic {
    pub rx: u64,
    pub tx: u64,
}

pub struct Tunnel {
    pub device: String,
    pub kind: String,
    pub up: bool,
    pub address: String,
    pub traffic: Traffic,
}

impl Vpn {
    pub fn read(request: &Request) -> Vpn {
        let state = request.ubus.get("vpnState");
        let tunnels: Vec<&Value> = state
            .and_then(|s| s["tunnels"].as_array())
            .map(|t| t.iter().collect())
            .unwrap_or_default();
        let networks: Vec<(String, String)> = request
            .snapshot
            .sections_of_type("network", "interface")
            .iter()
            .map(|s| (s.scalar("device"), s.name()))
            .collect();
        let network_of = |device: &str| {
            networks
                .iter()
                .find(|(d, _)| !device.is_empty() && d == device)
                .map(|(_, n)| n.clone())
                .unwrap_or_default()
        };

        let mut instances = Vec::new();
        for section in request.snapshot.sections_of_type(CONFIG, "openvpn") {
            let name = section.name();
            let enabled = section.scalar("enabled") == "1" || section.scalar("enable") == "1";
            if !enabled && SAMPLES.contains(&name.as_str()) {
                continue;
            }
            let read = state.map(|s| &s["instances"][name.as_str()]);
            let live = read.map(live).unwrap_or_default();
            // The device the instance holds now, or the one its section names.
            let device = match live.device.as_str() {
                "" => section.scalar("dev"),
                d => d.to_string(),
            };
            let traffic = tunnels
                .iter()
                .find(|t| !device.is_empty() && t["device"] == device.as_str())
                .map(|t| traffic(t));
            let config = section.scalar("config");
            let file = request
                .ubus
                .get("openvpnFiles")
                .and_then(|s| s["files"].as_array())
                .and_then(|files| {
                    files
                        .iter()
                        .find(|f| !config.is_empty() && f["path"] == config.as_str())
                })
                .filter(|f| f.get("error").is_none())
                .map(|f| ProfileFile {
                    content: text(f, "content"),
                    version: text(f, "version"),
                });
            instances.push(Instance {
                network: network_of(&device),
                file,
                config,
                values: section
                    .entries()
                    .filter(|(key, _)| !key.starts_with('.'))
                    .map(|(key, value)| (key.clone(), value.clone()))
                    .collect(),
                profile: read.and_then(|r| r.get("profile")).map(profile),
                enabled,
                live,
                traffic,
                name,
            });
        }

        let others = tunnels
            .iter()
            .filter(|t| t["kind"] != "openvpn")
            .map(|t| Tunnel {
                device: text(t, "device"),
                kind: text(t, "kind"),
                up: t["up"].as_bool().unwrap_or(false),
                address: t["addresses"]
                    .as_array()
                    .and_then(|a| a.first())
                    .and_then(Value::as_str)
                    .unwrap_or_default()
                    .to_string(),
                traffic: traffic(t),
            })
            .collect();
        Vpn { instances, others }
    }

    pub fn instance(&self, name: &str) -> Option<&Instance> {
        self.instances.iter().find(|i| i.name == name)
    }
}

fn text(v: &Value, key: &str) -> String {
    v[key].as_str().unwrap_or_default().to_string()
}

fn live(v: &Value) -> Live {
    Live {
        state: text(v, "state"),
        server: text(v, "server"),
        address: text(v, "address"),
        device: text(v, "device"),
        since: v["since"].as_u64(),
    }
}

fn traffic(v: &Value) -> Traffic {
    Traffic {
        rx: v["rx"].as_u64().unwrap_or(0),
        tx: v["tx"].as_u64().unwrap_or(0),
    }
}

fn profile(v: &Value) -> Profile {
    let flag = |key: &str| v[key].as_bool().unwrap_or(false);
    Profile {
        client: flag("client"),
        proto: text(v, "proto"),
        remotes: v["remotes"]
            .as_array()
            .map(|r| {
                r.iter()
                    .map(|r| Remote {
                        host: text(r, "host"),
                        port: text(r, "port"),
                    })
                    .collect()
            })
            .unwrap_or_default(),
        cipher: text(v, "cipher"),
        verify: text(v, "verify"),
        asks_sign_in: flag("asks_sign_in"),
        redirect: flag("redirect"),
        nopull: flag("nopull"),
        blocks: v["blocks"]
            .as_array()
            .map(|b| {
                b.iter()
                    .filter_map(Value::as_str)
                    .map(String::from)
                    .collect()
            })
            .unwrap_or_default(),
        text: text(v, "text"),
    }
}

impl Remote {
    /// endpoint is how the server reads where an address is expected.
    pub fn endpoint(&self) -> String {
        match self.port.as_str() {
            "" => self.host.clone(),
            port => format!("{}:{port}", self.host),
        }
    }
}
