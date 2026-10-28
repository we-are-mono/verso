// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.
//! Shared presentation of the live DHCPv4 observations verso-plugin-dnsdhcp's
//! root half reads (its `dhcp-state` read, ADR-027). Both the DNS & DHCP and
//! the Interfaces plugins use this vocabulary so a server has the same meaning
//! on either page; each declares and is granted that read.
use crate::{url_encode, Snapshot, Ubus, Value};
use std::collections::BTreeSet;

/// The read a plugin declares (`verso-plugin dnsdhcp dhcp-state`) and finds the
/// servers' state under, and the version this vocabulary reads.
pub const DHCP_STATE: &str = "dnsdhcp.dhcp-state";
pub const DHCP_STATE_VERSION: u32 = 1;

#[derive(Clone, Debug)]
pub struct Server {
    pub network: String,
    pub state: String,
    pub reason: String,
    pub pool: String,
    pub lease_time: String,
    pub leases: Option<u64>,
    pub configured: bool,
    /// The network has a `config dhcp` section of its own.
    pub section: bool,
    /// The network has an address of its own to hand out from: static, with
    /// an address set.
    pub addressed: bool,
    /// The network's section hands out addresses (no `ignore 1`).
    pub serves: bool,
}
impl Server {
    /// exists says the network has a DHCP server: a section on a network with
    /// an address of its own, switched off or not, or one set to hand out
    /// addresses. The stock uplink section, which says not to serve on a
    /// network that learns its address, is no server.
    pub fn exists(&self) -> bool {
        self.section && (self.addressed || self.serves)
    }
    /// can_be_added says the network could take a server and has none: an
    /// address of its own and no section.
    pub fn can_be_added(&self) -> bool {
        self.addressed && !self.section
    }
    /// href is where this server is edited: its row on the DHCP page, with its
    /// drawer open.
    pub fn href(&self) -> String {
        format!("/plugins/dnsdhcp/?open={}", url_encode(&self.network))
    }
    pub fn tone(&self) -> &'static str {
        match self.state.as_str() {
            "running" => "success",
            "stopped" => "danger",
            "disabled" => "neutral",
            _ => "warning",
        }
    }
    pub fn label(&self) -> &'static str {
        match self.state.as_str() {
            "running" => "Serving",
            "stopped" if self.reason == "failed" => "Failed",
            "stopped" => "Stopped",
            "disabled" => "Off",
            _ => match self.reason.as_str() {
                "interface-down" => "Interface down",
                "interface-unavailable" => "Interface unavailable",
                "not-serving" => "Not serving",
                "pool-full" => "Pool full",
                "leases-unavailable" => "Leases unavailable",
                "partial" => "Partially running",
                "pending-enable" => "Pending enable",
                "pending-disable" => "Pending disable",
                _ => "Status unavailable",
            },
        }
    }
    pub fn title(&self) -> &'static str {
        match self.label() {
            "Serving" => "DHCP server is running.",
            "Failed" => "DHCP server failed. Check System → Logs.",
            "Stopped" => "DHCP server is enabled but stopped.",
            "Off" => "DHCP server is disabled on this network.",
            "Interface down" => "DHCP cannot serve clients while this interface is down.",
            "Interface unavailable" => {
                "DHCP cannot serve clients because this interface is unavailable."
            }
            "Not serving" => {
                "dnsmasq is running but is not serving this network. Check System → Logs."
            }
            "Pool full" => "DHCP is running, but all addresses in its dynamic pool are leased.",
            "Leases unavailable" => "DHCP is running, but its active leases could not be read.",
            "Partially running" => "Some DHCP server instances are stopped or have failed.",
            "Pending enable" => "DHCP is disabled. Apply the staged changes to enable it.",
            "Pending disable" => {
                "Disabling DHCP is staged. Apply the changes to stop serving this network."
            }
            _ => "DHCP server status could not be verified.",
        }
    }
    pub fn pool_label(&self) -> &str {
        match self.pool.as_str() {
            "" => "—",
            "reservations" => "Reservations only",
            value => value,
        }
    }
}
fn text(value: Option<&Value>, key: &str) -> String {
    value
        .and_then(|v| v.get(key))
        .and_then(Value::as_str)
        .unwrap_or("")
        .into()
}
pub fn servers(snapshot: &Snapshot, ubus: &Ubus) -> Vec<Server> {
    let configs = snapshot.sections_of_type("dhcp", "dhcp");
    let interfaces = snapshot.sections_of_type("network", "interface");
    // An address may be written as one option or as a list of them.
    let addressed = |network: &str| {
        interfaces.iter().any(|n| {
            n.name() == network
                && n.scalar("proto") == "static"
                && (!n.scalar("ipaddr").is_empty() || !n.list("ipaddr").is_empty())
        })
    };
    let live = ubus.read(DHCP_STATE, &[DHCP_STATE_VERSION]).and_then(|v| v.get("networks"));
    let mut networks: BTreeSet<String> = snapshot
        .sections_of_type("network", "interface")
        .iter()
        .filter(|n| n.scalar("proto") == "static" && n.name() != "loopback")
        .map(|n| n.name())
        .collect();
    networks.extend(
        configs
            .iter()
            .map(|s| s.scalar("interface").to_string())
            .filter(|n| !n.is_empty()),
    );
    if let Some(values) = live.and_then(Value::as_object) {
        networks.extend(values.keys().cloned());
    }
    networks
        .into_iter()
        .map(|network| {
            let config = configs.iter().find(|s| s.scalar("interface") == network);
            let enabled = config
                .is_some_and(|s| s.scalar("ignore") != "1" && s.scalar("dhcpv4") != "disabled");
            let observed = live.and_then(|v| v.get(&network));
            let mut state = text(observed, "state");
            let mut reason = text(observed, "reason");
            if state.is_empty() {
                state = if enabled { "warning" } else { "disabled" }.into();
            }
            if observed.is_none() && live.is_some() && enabled {
                reason = "pending-enable".into();
            }
            if let Some(applied) = observed
                .and_then(|v| v.get("enabled"))
                .and_then(Value::as_bool)
            {
                if applied != enabled {
                    state = "warning".into();
                    reason = if enabled {
                        "pending-enable"
                    } else {
                        "pending-disable"
                    }
                    .into();
                }
            }
            let addressed = addressed(&network);
            Server {
                network,
                state,
                reason,
                pool: text(observed, "pool"),
                lease_time: text(observed, "lease_time"),
                leases: observed
                    .and_then(|v| v.get("leases"))
                    .and_then(Value::as_u64),
                configured: config.is_some() || observed.is_some(),
                section: config.is_some(),
                addressed,
                serves: config.is_some_and(|s| s.scalar("ignore") != "1"),
            }
        })
        .collect()
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::json;
    fn snapshot(ignore: &str) -> Snapshot {
        Snapshot::from_value(
            json!({"network":{"lan":{".type":"interface",".name":"lan","proto":"static"},"wan":{".type":"interface",".name":"wan","proto":"dhcp"}},"dhcp":{"lan":{".type":"dhcp",".name":"lan","interface":"lan","ignore":ignore}}}),
        )
    }
    #[test]
    fn staged_enable_is_never_reported_running() {
        let live = Ubus::from_value(
            json!({"dnsdhcp.dhcp-state":{"version":1,"result":{"networks":{"lan":{"enabled":false,"state":"disabled"}}}}}),
        );
        let states = servers(&snapshot("0"), &live);
        assert_eq!(states.len(), 1);
        assert_eq!(states[0].tone(), "warning");
        assert_eq!(states[0].label(), "Pending enable");
    }
    #[test]
    fn stopped_and_missing_evidence_are_distinct() {
        let live = Ubus::from_value(
            json!({"dnsdhcp.dhcp-state":{"version":1,"result":{"networks":{"lan":{"enabled":true,"state":"stopped"}}}}}),
        );
        assert_eq!(servers(&snapshot("0"), &live)[0].tone(), "danger");
        let unknown = servers(&snapshot("0"), &Ubus::from_value(json!({})));
        assert_eq!(unknown[0].label(), "Status unavailable");
        assert_eq!(unknown[0].tone(), "warning");
        // A state at a version the SDK does not know is no evidence either,
        // never read as if it were the shape it knows.
        let newer = Ubus::from_value(
            json!({"dnsdhcp.dhcp-state":{"version":2,"result":{"networks":{"lan":{"enabled":true,"state":"stopped"}}}}}),
        );
        assert_eq!(servers(&snapshot("0"), &newer)[0].label(), "Status unavailable");
    }
    #[test]
    fn applied_running_server_and_pending_disable_share_one_vocabulary() {
        let live = Ubus::from_value(
            json!({"dnsdhcp.dhcp-state":{"version":1,"result":{"networks":{"lan":{"enabled":true,"state":"running","pool":"192.168.1.100–192.168.1.249","leases":0}}}}}),
        );
        let running = servers(&snapshot("0"), &live);
        assert_eq!(running[0].tone(), "success");
        assert_eq!(running[0].leases, Some(0));
        assert_eq!(running[0].label(), "Serving");
        assert_eq!(running[0].href(), "/plugins/dnsdhcp/?open=lan");
        assert_eq!(servers(&snapshot("1"), &live)[0].label(), "Pending disable");
    }
    #[test]
    fn a_server_exists_where_a_section_serves_or_could() {
        let routed = |dhcp: crate::Value| {
            Snapshot::from_value(json!({"network":{
                "lan":{".type":"interface",".name":"lan","proto":"static","ipaddr":"10.0.0.1","netmask":"255.255.255.0"},
                "guest":{".type":"interface",".name":"guest","proto":"static","ipaddr":["10.0.20.1/24"]},
                "wan":{".type":"interface",".name":"wan","proto":"dhcp"}},
                "dhcp":dhcp}))
        };
        let read = |dhcp: crate::Value| {
            servers(&routed(dhcp), &Ubus::from_value(json!({})))
                .into_iter()
                .map(|s| (s.network.clone(), s.exists(), s.can_be_added()))
                .collect::<Vec<_>>()
        };
        // An addressed network with a section has a server, switched off or
        // not; one with an address and no section could take one; the stock
        // uplink section, which says not to serve, is neither.
        assert_eq!(
            read(json!({
                "lan":{".type":"dhcp",".name":"lan","interface":"lan","ignore":"1"},
                "wan":{".type":"dhcp",".name":"wan","interface":"wan","ignore":"1"}})),
            vec![
                ("guest".into(), false, true),
                ("lan".into(), true, false),
                ("wan".into(), false, false),
            ]
        );
        // A section set to hand out addresses is a server even where the
        // network has no address to hand out from.
        let misplaced = read(json!({"wan":{".type":"dhcp",".name":"wan","interface":"wan"}}));
        assert!(misplaced.contains(&("wan".into(), true, false)), "{misplaced:?}");
    }
}
