// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.
//! What a device is, in Verso's words: one closed list the Devices card's Type
//! column reads and each device's drawer takes its mark from. The config's
//! `type` and the kernel's kind are machine strings the drawer shows as they
//! are; this list is the classification, so it reads in words.
use crate::Model;
use serde_json::Value;

/// DeviceKind is one entry of the list. Whatever the list does not know is
/// Virtual, never a raw string.
#[derive(Debug, Clone, PartialEq, Eq)]
pub enum DeviceKind {
    Port,
    Bridge,
    /// A VLAN by its ID; `qinq` for an 802.1ad (service) tag.
    Vlan {
        vid: String,
        qinq: bool,
    },
    Pppoe,
    Tunnel,
    Wifi,
    Virtual,
}

fn text(v: Option<&Value>) -> &str {
    v.and_then(Value::as_str).unwrap_or("")
}

impl DeviceKind {
    /// of reads a device's kind from its config section first, where it has
    /// one, then from what the shell's topology and netifd report, then from
    /// its name.
    pub fn of(m: &Model, name: &str, runtime: Option<&Value>) -> DeviceKind {
        if let Some(d) = m.device(name) {
            match d.get("type").as_str() {
                "bridge" => return DeviceKind::Bridge,
                typ @ ("8021q" | "8021ad") => {
                    return DeviceKind::Vlan {
                        vid: d.get("vid"),
                        qinq: typ == "8021ad",
                    }
                }
                "" => {}
                _ => return DeviceKind::Virtual,
            }
        }
        let kind = text(runtime.and_then(|v| v.get("kind")));
        let devtype = text(runtime.and_then(|v| v.get("devtype")));
        let typ = text(runtime.and_then(|v| v.get("type")));
        let physical = runtime
            .and_then(|v| v.get("physical"))
            .and_then(Value::as_bool);
        let suffix = name
            .rsplit_once('.')
            .map(|(_, vid)| vid)
            .filter(|vid| vid.parse::<u16>().is_ok());
        if [kind, devtype, typ].contains(&"bridge")
            || runtime
                .and_then(|v| v.get("bridge"))
                .and_then(Value::as_bool)
                == Some(true)
        {
            DeviceKind::Bridge
        } else if kind == "pppoe" || name.starts_with("pppoe-") {
            DeviceKind::Pppoe
        } else if kind == "vlan" || devtype == "vlan" || suffix.is_some() {
            DeviceKind::Vlan {
                vid: suffix.unwrap_or_default().into(),
                qinq: false,
            }
        } else if kind == "wifi" || devtype == "wlan" {
            DeviceKind::Wifi
        } else if kind == "tunnel" {
            DeviceKind::Tunnel
        } else if kind == "port" || physical == Some(true) {
            DeviceKind::Port
        } else {
            DeviceKind::Virtual
        }
    }

    /// label is the Type column's word for it.
    pub fn label(&self) -> String {
        match self {
            DeviceKind::Port => "Port".into(),
            DeviceKind::Bridge => "Bridge".into(),
            DeviceKind::Vlan { vid, qinq } => {
                let tag = if vid.is_empty() {
                    "VLAN".to_string()
                } else {
                    format!("VLAN {vid}")
                };
                if *qinq {
                    format!("{tag} · 802.1ad")
                } else {
                    tag
                }
            }
            DeviceKind::Pppoe => "PPPoE".into(),
            DeviceKind::Tunnel => "Tunnel".into(),
            DeviceKind::Wifi => "Wi-Fi".into(),
            DeviceKind::Virtual => "Virtual".into(),
        }
    }

    /// glyph is its mark, the same one the chooser makes it with.
    pub fn glyph(&self) -> &'static str {
        match self {
            DeviceKind::Port => "ethernet-port",
            DeviceKind::Bridge => "git-merge",
            DeviceKind::Vlan { .. } => "tag",
            DeviceKind::Pppoe | DeviceKind::Tunnel => "git-branch",
            DeviceKind::Wifi => "wifi",
            DeviceKind::Virtual => "cpu",
        }
    }
}
