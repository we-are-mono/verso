// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

//! The live lease table, as the shell brokers it.
//!
//! A policy is written against a MAC, which is what fw4 matches and what this
//! plugin is handed. A person does not think in MACs, so the lease table is read
//! for one thing only: the name to put in the rule and in the listing. It is a
//! read this plugin cannot make itself (ADR-007), and its absence is a device
//! named by its MAC rather than an error.

use verso_plugin::{Ubus, Value};

use crate::model::same_mac;

/// FUNCTION is the brokered read this plugin declares in its manifest.
pub const FUNCTION: &str = "dhcpLeases";

/// Leases is what the shell read, held only as the pairs this plugin needs.
#[derive(Default)]
pub struct Leases {
    names: Vec<(String, String)>,
}

impl Leases {
    /// read takes the brokered result. An absent or malformed read is no names,
    /// which every caller states rather than fails on.
    pub fn read(ubus: &Ubus) -> Leases {
        let Some(result) = ubus.get(FUNCTION) else {
            return Leases::default();
        };
        let names = result
            .get("leases")
            .and_then(Value::as_array)
            .map(|leases| {
                leases
                    .iter()
                    .map(|lease| {
                        let field = |key: &str| {
                            lease
                                .get(key)
                                .and_then(Value::as_str)
                                .unwrap_or("")
                                .to_string()
                        };
                        (field("mac"), field("hostname"))
                    })
                    .filter(|(mac, hostname)| !mac.is_empty() && !hostname.is_empty())
                    .collect()
            })
            .unwrap_or_default();
        Leases { names }
    }

    /// subject is what a sentence calls this device: the name it announced when
    /// it asked for an address, and its MAC when it announced none or the table
    /// could not be read. Either way the rule names something a person can find
    /// the device by.
    pub fn subject(&self, mac: &str) -> String {
        self.names
            .iter()
            .find(|(held, _)| same_mac(held, mac))
            .map(|(_, hostname)| hostname.clone())
            .unwrap_or_else(|| mac.to_string())
    }
}
