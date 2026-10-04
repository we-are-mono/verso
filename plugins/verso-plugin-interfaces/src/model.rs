// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.
use serde_json::{Map, Value};
use std::collections::{BTreeMap, BTreeSet};
use verso_plugin::{Request, Section, Snapshot};

#[derive(Clone, Default)]
pub struct Record {
    pub id: String,
    pub anonymous: bool,
    /// The section's uci type: `route` or `route6` tells a route's family.
    pub kind: String,
    pub values: Map<String, Value>,
}
impl Record {
    pub fn config_id(&self) -> &str {
        if self.anonymous {
            ""
        } else {
            &self.id
        }
    }
    pub fn get(&self, key: &str) -> String {
        self.values
            .get(key)
            .and_then(Value::as_str)
            .unwrap_or("")
            .into()
    }
    pub fn list(&self, key: &str) -> Vec<String> {
        strings(self.values.get(key))
    }
}
pub fn strings(v: Option<&Value>) -> Vec<String> {
    match v {
        Some(Value::Array(a)) => a
            .iter()
            .filter_map(Value::as_str)
            .map(String::from)
            .collect(),
        Some(Value::String(s)) => s.split_whitespace().map(String::from).collect(),
        _ => vec![],
    }
}
pub fn records(s: &Snapshot, config: &str, kind: &str) -> Vec<Record> {
    s.sections_of_type(config, kind)
        .iter()
        .map(|s| record(s, kind))
        .collect()
}
fn record(s: &Section, kind: &str) -> Record {
    Record {
        id: s.name(),
        anonymous: s.anonymous(),
        kind: kind.into(),
        values: s.entries().map(|(k, v)| (k.clone(), v.clone())).collect(),
    }
}
// routes is every static route, IPv4 and IPv6 together, in the order the file
// states them.
fn routes(s: &Snapshot) -> Vec<Record> {
    let mut all: Vec<(f64, Record)> = ["route", "route6"]
        .into_iter()
        .flat_map(|kind| {
            s.sections_of_type("network", kind)
                .iter()
                .map(|s| (s.index(), record(s, kind)))
                .collect::<Vec<_>>()
        })
        .collect();
    all.sort_by(|a, b| a.0.total_cmp(&b.0));
    all.into_iter().map(|(_, r)| r).collect()
}
#[derive(Clone, Default)]
pub struct Model {
    pub networks: Vec<Record>,
    pub devices: Vec<Record>,
    pub zones: Vec<Record>,
    pub dhcp: Vec<Record>,
    pub dhcp_servers: Vec<verso_plugin::dhcp::Server>,
    pub live: Value,
    pub wireless: Vec<Record>,
    pub routes: Vec<Record>,
}
impl Model {
    pub fn resolve_device(&self, name: &str) -> String {
        let mut name = name.to_owned();
        let mut seen = BTreeSet::new();
        while let Some(net) = name.strip_prefix('@') {
            if !seen.insert(net.to_owned()) {
                return String::new();
            }
            name = self
                .live_network(net)
                .and_then(|v| v.get("l3_device"))
                .and_then(Value::as_str)
                .map(String::from)
                .unwrap_or_else(|| {
                    self.network(net)
                        .map(|n| n.get("device"))
                        .unwrap_or_default()
                });
        }
        name
    }
    pub fn read(r: &Request) -> Self {
        Self {
            networks: records(&r.snapshot, "network", "interface"),
            devices: records(&r.snapshot, "network", "device"),
            zones: records(&r.snapshot, "firewall", "zone"),
            dhcp: records(&r.snapshot, "dhcp", "dhcp"),
            dhcp_servers: verso_plugin::dhcp::servers(&r.snapshot, &r.ubus),
            wireless: records(&r.snapshot, "wireless", "wifi-iface"),
            routes: routes(&r.snapshot),
            live: r.ubus.get("networkState").cloned().unwrap_or(Value::Null),
        }
    }
    pub fn names(&self) -> BTreeSet<String> {
        let mut names: BTreeSet<String> = self
            .devices
            .iter()
            .map(|d| d.get("name"))
            .filter(|s| !s.is_empty())
            .collect();
        for n in &self.networks {
            let d = self.resolve_device(&n.get("device"));
            if !d.is_empty() {
                names.insert(d);
            }
        }
        for d in &self.devices {
            names.extend(d.list("ports"));
            let parent = d.get("ifname");
            if !parent.is_empty() {
                names.insert(parent);
            }
        }
        if let Some(devices) = self.live.get("devices").and_then(Value::as_object) {
            names.extend(devices.keys().cloned());
            for device in devices.values() {
                names.extend(strings(device.get("bridge-members")));
            }
        }
        names.remove("lo");
        names
    }
    pub fn network(&self, id: &str) -> Option<&Record> {
        self.networks.iter().find(|r| r.id == id)
    }
    pub fn device(&self, name: &str) -> Option<&Record> {
        self.devices.iter().find(|r| r.get("name") == name)
    }
    pub fn zone(&self, net: &str) -> String {
        self.zones
            .iter()
            .find(|z| z.list("network").iter().any(|n| n == net))
            .map(|z| z.get("name"))
            .unwrap_or_default()
    }
    pub fn dhcp(&self, net: &str) -> Option<&Record> {
        self.dhcp.iter().find(|d| d.get("interface") == net)
    }
    pub fn live_network(&self, net: &str) -> Option<&Value> {
        self.live
            .get("interfaces")?
            .as_array()?
            .iter()
            .find(|n| n.get("interface").and_then(Value::as_str) == Some(net))
    }
    pub fn bridge_ports(&self, name: &str) -> Vec<String> {
        self.live
            .get("devices")
            .and_then(|v| v.get(name))
            .and_then(|v| v.get("bridge-members"))
            .map(|v| strings(Some(v)))
            .unwrap_or_else(|| {
                self.device(name)
                    .map(|d| d.list("ports"))
                    .unwrap_or_default()
            })
    }
    fn tree_root(&self, name: &str) -> bool {
        let runtime = &self.live["devices"][name];
        runtime["physical"].as_bool() == Some(true)
            || matches!(runtime["kind"].as_str(), Some("port" | "wifi" | "bridge"))
            || runtime["type"] == "bridge"
            || runtime["devtype"] == "bridge"
            || runtime["bridge"].as_bool() == Some(true)
            || self.device(name).is_some_and(|d| d.get("type") == "bridge")
    }
    pub fn parents(&self) -> BTreeMap<String, String> {
        let mut p = BTreeMap::new();
        for d in &self.devices {
            let name = d.get("name");
            let parent = d.get("ifname");
            if !parent.is_empty() {
                p.insert(name.clone(), parent);
            }
        }
        if let Some(devices) = self.live.get("devices").and_then(Value::as_object) {
            for (name, d) in devices {
                if self.tree_root(name) {
                    continue;
                }
                if let Some(parent) = d
                    .get("parent")
                    .and_then(Value::as_str)
                    .filter(|p| !p.is_empty())
                {
                    p.insert(name.clone(), parent.into());
                }
            }
        }
        for name in self.names() {
            if let Some((parent, vid)) = name.rsplit_once('.') {
                if vid.parse::<u16>().is_ok() {
                    p.entry(name.clone()).or_insert(parent.into());
                }
            }
        }
        // netifd's L3 device belongs below its transport (PPPoE, for example).
        for n in &self.networks {
            if let Some(l) = self.live_network(&n.id) {
                let l3 = l.get("l3_device").and_then(Value::as_str).unwrap_or("");
                let transport = self.resolve_device(&n.get("device"));
                if !l3.is_empty() && !transport.is_empty() && l3 != transport {
                    p.insert(l3.into(), transport);
                }
            }
        }
        // The reference tree groups software bridge members under the bridge,
        // ahead of their lower transport (e.g. br-iptv owns eth4.3999).
        // Hardware ports stay independent roots; membership is also in Ports.
        for bridge in self.names() {
            for member in self.bridge_ports(&bridge) {
                let runtime = &self.live["devices"][&member];
                let software = p.contains_key(&member)
                    || runtime["physical"].as_bool() == Some(false)
                    || matches!(
                        runtime["kind"].as_str(),
                        Some("virtual" | "vlan" | "pppoe" | "tunnel")
                    );
                if software && !self.tree_root(&member) && member != bridge {
                    p.insert(member, bridge.clone());
                }
            }
        }
        p.retain(|name, parent| name != parent && !self.tree_root(name));
        p
    }
}
