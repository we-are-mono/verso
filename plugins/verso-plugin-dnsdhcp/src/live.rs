// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

//! The live lease table, as the shell brokers it.
//!
//! Config says which addresses may be handed out; only this says which ones are.
//! It is the Leases listing and the count on each server's row — and it is a
//! read this plugin cannot make itself (ADR-007), so its absence is a page
//! without live state rather than an error: no leases read is the same to every
//! caller here as no leases held.

use verso_plugin::{Ubus, Value};

use crate::format::Subnet;

/// FUNCTION is the brokered read this plugin declares in its manifest.
pub const FUNCTION: &str = "dhcpLeases";

/// Lease is one device holding an address right now.
pub struct Lease {
    pub hostname: String,
    pub mac: String,
    pub ipv4: String,
    pub expires_at: i64,
}

/// Leases is the whole lease table for one render.
#[derive(Default)]
pub struct Leases {
    entries: Vec<Lease>,
    /// Whether the shell brokered the read at all — the difference between a
    /// router handing out nothing and a router this plugin could not ask.
    read: bool,
}

impl Leases {
    /// read takes the brokered result. An absent or malformed read is no leases,
    /// which every page states rather than fails on.
    pub fn read(ubus: &Ubus) -> Leases {
        let Some(result) = ubus.get(FUNCTION) else {
            return Leases::default();
        };
        let entries = result
            .get("leases")
            .and_then(Value::as_array)
            .map(|leases| leases.iter().map(Lease::read).collect())
            .unwrap_or_default();
        Leases {
            entries,
            read: true,
        }
    }

    /// all is every lease, in the order the shell read them (address order).
    pub fn all(&self) -> &[Lease] {
        &self.entries
    }

    /// known reports whether the lease table was read at all.
    pub fn known(&self) -> bool {
        self.read
    }

    /// on counts the devices holding an address on one network.
    pub fn on(&self, subnet: &Subnet) -> usize {
        self.entries
            .iter()
            .filter(|lease| subnet.holds(&lease.ipv4))
            .count()
    }
}

impl Lease {
    fn read(value: &Value) -> Lease {
        let text = |key: &str| {
            value
                .get(key)
                .and_then(Value::as_str)
                .unwrap_or("")
                .to_string()
        };
        Lease {
            hostname: text("hostname"),
            mac: text("mac"),
            ipv4: text("ipv4"),
            expires_at: value
                .get("expires_at")
                .and_then(Value::as_i64)
                .unwrap_or_default(),
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::fixture;

    #[test]
    fn the_brokered_table_reads_as_the_devices_holding_addresses() {
        let leases = fixture::leases();
        assert!(leases.known());
        let names: Vec<&str> = leases
            .all()
            .iter()
            .map(|lease| lease.hostname.as_str())
            .collect();
        assert_eq!(names, vec!["toms-iphone", "", "nas", "guest-tablet"]);
        assert_eq!(leases.all()[2].ipv4, "10.0.0.30");
    }

    #[test]
    fn leases_are_counted_against_the_network_they_sit_on() {
        let leases = fixture::leases();
        let lan = Subnet::read("10.0.0.1", "255.255.255.0").expect("lan");
        let guest = Subnet::read("10.0.20.1", "255.255.255.0").expect("guest");
        assert_eq!(leases.on(&lan), 3);
        assert_eq!(leases.on(&guest), 1);
    }

    #[test]
    fn an_unbrokered_read_is_no_leases_rather_than_an_error() {
        let leases = Leases::read(&Ubus::from_value(Value::Null));
        assert!(!leases.known());
        assert!(leases.all().is_empty());

        // A read that answered with nothing is an answer.
        let empty = Leases::read(&Ubus::from_value(
            serde_json::json!({"dhcpLeases": {"leases": []}}),
        ));
        assert!(empty.known());
        assert!(empty.all().is_empty());
    }
}
