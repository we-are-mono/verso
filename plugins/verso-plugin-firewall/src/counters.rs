// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

//! The kernel's own hit counts, brokered to this plugin by the shell.
//!
//! firewall4 stamps every rule it renders with a counter and a comment naming
//! the section it came from, so a section's hits are the entry under its
//! evaluation chain and its firewall4 identity. Both halves matter: one name can
//! appear in several chains, and a redirect's reflection rules carry the same
//! name with a suffix, so only the exact pair is that section's traffic.
//!
//! The read is a read, not a guarantee. The operator may not be permitted it, the
//! helper may be unreachable, the ruleset may not have been loaded yet — in every
//! such case the listing simply has nothing to say about hits, which is honest,
//! and shows so.

use std::collections::HashMap;
use verso_plugin::{Ubus, Value};

/// FUNCTION is the brokered helper read this plugin declares in its manifest.
pub const FUNCTION: &str = "firewallCounters";

/// Counters is one render's hit counts, keyed by (chain, firewall4 identity).
#[derive(Default)]
pub struct Counters {
    packets: HashMap<(String, String), u64>,
}

impl Counters {
    /// read indexes the brokered result. A payload the shell did not send, or one
    /// shaped differently than the helper documents, indexes nothing.
    pub fn read(ubus: &Ubus) -> Counters {
        let entries = ubus
            .get(FUNCTION)
            .and_then(|result| result.get("counters"))
            .and_then(Value::as_array)
            .map(Vec::as_slice)
            .unwrap_or(&[]);
        let mut packets = HashMap::with_capacity(entries.len());
        for entry in entries {
            let (Some(chain), Some(name), Some(count)) = (
                entry.get("chain").and_then(Value::as_str),
                entry.get("name").and_then(Value::as_str),
                entry.get("packets").and_then(Value::as_u64),
            ) else {
                continue;
            };
            packets.insert((chain.to_string(), name.to_string()), count);
        }
        Counters { packets }
    }

    /// packets returns a section's hit count. None is "nothing was read for this
    /// section" — a disabled section renders no kernel rule at all — and is never
    /// the same statement as a count of zero.
    pub fn packets(&self, chain: &str, identity: &str) -> Option<u64> {
        self.packets
            .get(&(chain.to_string(), identity.to_string()))
            .copied()
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use verso_plugin::json;

    fn counters() -> Counters {
        Counters::read(&Ubus::from_value(json!({
            "firewallCounters": {"counters": [
                {"chain": "input_wan", "name": "Allow-Ping", "packets": 1400, "bytes": 117600},
                {"chain": "forward_wan", "name": "Allow-Ping", "packets": 7, "bytes": 588},
                {"chain": "dstnat_wan", "name": "HTTPS-to-NAS", "packets": 1200, "bytes": 96000},
                {"chain": "dstnat_wan", "name": "HTTPS-to-NAS (reflection)", "packets": 3, "bytes": 240},
                {"chain": "input_wan", "name": "@rule[6]", "packets": 0, "bytes": 0}
            ]}
        })))
    }

    #[test]
    fn a_count_is_found_by_chain_and_identity_together() {
        let counters = counters();
        assert_eq!(counters.packets("input_wan", "Allow-Ping"), Some(1400));
        assert_eq!(counters.packets("forward_wan", "Allow-Ping"), Some(7));
        assert_eq!(counters.packets("input_wan", "@rule[6]"), Some(0));
        assert_eq!(counters.packets("output_lan", "Allow-Ping"), None);
    }

    #[test]
    fn a_reflection_entry_is_not_its_port_forwards_traffic() {
        assert_eq!(counters().packets("dstnat_wan", "HTTPS-to-NAS"), Some(1200));
    }

    #[test]
    fn an_unbrokered_read_leaves_every_section_without_a_count() {
        let absent = Counters::read(&Ubus::from_value(Value::Null));
        assert_eq!(absent.packets("input_wan", "Allow-Ping"), None);

        let unexpected = Counters::read(&Ubus::from_value(json!({"firewallCounters": 12})));
        assert_eq!(unexpected.packets("input_wan", "Allow-Ping"), None);

        let incomplete = Counters::read(&Ubus::from_value(
            json!({"firewallCounters": {"counters": [{"chain": "input_wan"}]}}),
        ));
        assert_eq!(incomplete.packets("input_wan", "Allow-Ping"), None);
    }
}
