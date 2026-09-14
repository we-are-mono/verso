// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.
//! Shared presentation of the shell's live DHCPv4 observations. Both plugins
//! use this vocabulary so a server has the same meaning on either page.
use crate::{Snapshot, Ubus, Value};
use std::collections::BTreeSet;

#[derive(Clone, Debug)]
pub struct Server {
    pub network: String,
    pub state: String,
    pub reason: String,
    pub pool: String,
    pub lease_time: String,
    pub leases: Option<u64>,
    pub configured: bool,
}
impl Server {
    pub fn href(&self) -> String {
        let network: String = self
            .network
            .bytes()
            .map(|b| {
                if b.is_ascii_alphanumeric() || b"-_.~".contains(&b) {
                    (b as char).to_string()
                } else {
                    format!("%{b:02X}")
                }
            })
            .collect();
        format!("/plugins/interfaces/edit?network={network}#dhcp-server")
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
            "running" => "Running",
            "stopped" if self.reason == "failed" => "Failed",
            "stopped" => "Stopped",
            "disabled" => "Disabled",
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
            "Running" => "DHCP server is running.",
            "Failed" => "DHCP server failed. Check System → Logs.",
            "Stopped" => "DHCP server is enabled but stopped.",
            "Disabled" => "DHCP server is disabled on this network.",
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
    let live = ubus.get("dhcpState").and_then(|v| v.get("networks"));
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
            json!({"dhcpState":{"networks":{"lan":{"enabled":false,"state":"disabled"}}}}),
        );
        let states = servers(&snapshot("0"), &live);
        assert_eq!(states.len(), 1);
        assert_eq!(states[0].tone(), "warning");
        assert_eq!(states[0].label(), "Pending enable");
    }
    #[test]
    fn stopped_and_missing_evidence_are_distinct() {
        let live = Ubus::from_value(
            json!({"dhcpState":{"networks":{"lan":{"enabled":true,"state":"stopped"}}}}),
        );
        assert_eq!(servers(&snapshot("0"), &live)[0].tone(), "danger");
        let unknown = servers(&snapshot("0"), &Ubus::from_value(json!({})));
        assert_eq!(unknown[0].label(), "Status unavailable");
        assert_eq!(unknown[0].tone(), "warning");
    }
    #[test]
    fn applied_running_server_and_pending_disable_share_one_vocabulary() {
        let live = Ubus::from_value(
            json!({"dhcpState":{"networks":{"lan":{"enabled":true,"state":"running","pool":"192.168.1.100–192.168.1.249","leases":0}}}}),
        );
        let running = servers(&snapshot("0"), &live);
        assert_eq!(running[0].tone(), "success");
        assert_eq!(running[0].leases, Some(0));
        assert_eq!(
            running[0].href(),
            "/plugins/interfaces/edit?network=lan#dhcp-server"
        );
        assert_eq!(servers(&snapshot("1"), &live)[0].label(), "Pending disable");
    }
}
