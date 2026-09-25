// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

//! Per-rule hit counters read from the live nftables ruleset.
//!
//! fw4 renders every UCI `rule` and `redirect` into nftables with a `counter`
//! statement and a `!fw4: <name>` comment, where the name is the section's `name`
//! option and defaults to fw4's section id. The kernel's own counters are
//! therefore the honest hit count for a configured rule, and the comment is the
//! only handle back to the section that produced it.
//!
//! One section can render into several nft rules — an address-family split, an
//! ICMP type set that needs a second match — sharing both chain and comment, so a
//! section's hits are the sum over its (chain, name) pair. A redirect's
//! reflection rules carry `<name> (reflection)` and stay their own entry, so a
//! port forward's counters are never inflated by them.

use serde_json::{json, Value};
use std::collections::HashMap;

// fw4 marks the rules it renders. Anything else in the table came from a user
// include and answers to no UCI section, so it is left out rather than offered
// under a name a section might share by coincidence.
const FW4_MARKER: &str = "!fw4: ";

/// Summarize the loaded fw4 table, independently of UCI or service startup.
/// Listing the ruleset succeeds even when fw4 is absent, so absence is distinct
/// from a failed read. Active means the three fw4 filter entry points exist;
/// it does not certify that the configured policy protects any particular host.
pub fn status(ruleset: &[u8]) -> Result<Value, String> {
    let document: Value =
        serde_json::from_slice(ruleset).map_err(|error| format!("parse nft json: {error}"))?;
    let entries = document["nftables"]
        .as_array()
        .ok_or("missing nftables array")?;
    let table = entries
        .iter()
        .filter_map(|entry| entry.get("table"))
        .find(|table| table["family"] == "inet" && table["name"] == "fw4");
    let Some(table) = table else {
        return Ok(json!({"state": "inactive", "rules": 0}));
    };
    let belongs = |entry: &&Value| entry["family"] == "inet" && entry["table"] == "fw4";
    let rules = entries
        .iter()
        .filter_map(|entry| entry.get("rule"))
        .filter(belongs)
        .count();
    let dormant = table["flags"]
        .as_array()
        .is_some_and(|flags| flags.iter().any(|flag| flag == "dormant"));
    let hooks = ["input", "forward", "output"]
        .iter()
        .filter(|hook| {
            entries
                .iter()
                .filter_map(|entry| entry.get("chain"))
                .filter(belongs)
                .any(|chain| {
                    chain["name"] == **hook && chain["hook"] == **hook && chain["type"] == "filter"
                })
        })
        .count();
    let state = if dormant || hooks == 0 {
        "inactive"
    } else if hooks == 3 {
        "active"
    } else {
        "partial"
    };
    Ok(json!({"state": state, "rules": rules}))
}

/// counters reduces `nft -j list table inet fw4` output to one entry per
/// (chain, name) — packets and bytes summed, the marker stripped from the name,
/// in the order the ruleset evaluates them. A rule carrying no comment or no
/// counter contributes nothing.
pub fn counters(ruleset: &[u8]) -> Result<Value, String> {
    let document: Value =
        serde_json::from_slice(ruleset).map_err(|error| format!("parse nft json: {error}"))?;
    Ok(json!({ "counters": totals(&document) }))
}

#[derive(Default)]
struct Hits {
    packets: u64,
    bytes: u64,
}

fn totals(document: &Value) -> Vec<Value> {
    let mut order: Vec<(String, String)> = Vec::new();
    let mut hits: HashMap<(String, String), Hits> = HashMap::new();
    for rule in rules(document) {
        let Some(chain) = rule.get("chain").and_then(Value::as_str) else {
            continue;
        };
        let Some(name) = rule
            .get("comment")
            .and_then(Value::as_str)
            .and_then(|comment| comment.strip_prefix(FW4_MARKER))
        else {
            continue;
        };
        let Some((packets, bytes)) = counter(rule) else {
            continue;
        };
        let key = (chain.to_string(), name.to_string());
        if !hits.contains_key(&key) {
            order.push(key.clone());
        }
        let total = hits.entry(key).or_default();
        total.packets = total.packets.saturating_add(packets);
        total.bytes = total.bytes.saturating_add(bytes);
    }
    let mut counters = Vec::with_capacity(order.len());
    for (chain, name) in order {
        let Some(total) = hits.remove(&(chain.clone(), name.clone())) else {
            continue;
        };
        counters.push(json!({
            "chain": chain,
            "name": name,
            "packets": total.packets,
            "bytes": total.bytes,
        }));
    }
    counters
}

fn rules(document: &Value) -> impl Iterator<Item = &Value> {
    document
        .get("nftables")
        .and_then(Value::as_array)
        .map(Vec::as_slice)
        .unwrap_or(&[])
        .iter()
        .filter_map(|item| item.get("rule"))
}

fn counter(rule: &Value) -> Option<(u64, u64)> {
    let counter = rule
        .get("expr")?
        .as_array()?
        .iter()
        .find_map(|expr| expr.get("counter"))?;
    Some((
        counter.get("packets").and_then(Value::as_u64).unwrap_or(0),
        counter.get("bytes").and_then(Value::as_u64).unwrap_or(0),
    ))
}

#[cfg(test)]
mod tests {
    use super::*;

    // A ruleset captured from a running device, so the reduction is pinned to the
    // shape nft actually emits rather than to a hand-written idea of it.
    const CAPTURED: &[u8] = include_bytes!("../testdata/fw4-nft.json");

    #[test]
    fn loaded_fw4_reports_active_and_counts_all_kernel_rules() {
        assert_eq!(
            status(CAPTURED).unwrap(),
            json!({"state":"active", "rules":53})
        );
    }

    #[test]
    fn unrelated_tables_do_not_provide_fw4_status_or_rules() {
        let mut document: Value = serde_json::from_slice(CAPTURED).unwrap();
        let entries = document["nftables"].as_array_mut().unwrap();
        entries.push(json!({"rule":{"family":"inet", "table":"verso_log", "chain":"input"}}));
        entries.push(json!({"rule":{"family":"ip", "table":"fw4", "chain":"input"}}));
        assert_eq!(
            status(&serde_json::to_vec(&document).unwrap()).unwrap()["rules"],
            53
        );
        for entry in document["nftables"].as_array_mut().unwrap() {
            for kind in ["table", "chain", "rule"] {
                if let Some(value) = entry.get_mut(kind) {
                    value["family"] = json!("ip");
                }
            }
        }
        assert_eq!(
            status(&serde_json::to_vec(&document).unwrap()).unwrap(),
            json!({"state":"inactive", "rules":0})
        );
    }

    #[test]
    fn absent_empty_and_dormant_tables_are_inactive() {
        for document in [
            json!({"nftables":[]}),
            json!({"nftables":[{"table":{"family":"inet", "name":"fw4"}}]}),
            {
                let mut document: Value = serde_json::from_slice(CAPTURED).unwrap();
                document["nftables"][1]["table"]["flags"] = json!(["dormant"]);
                document
            },
        ] {
            assert_eq!(
                status(&serde_json::to_vec(&document).unwrap()).unwrap()["state"],
                "inactive"
            );
        }
    }

    #[test]
    fn missing_filter_hooks_cannot_be_replaced_by_mangle_hooks() {
        let mut document: Value = serde_json::from_slice(CAPTURED).unwrap();
        let entries = document["nftables"].as_array_mut().unwrap();
        entries.retain(|entry| entry["chain"]["name"] != "forward");
        assert_eq!(
            status(&serde_json::to_vec(&document).unwrap()).unwrap()["state"],
            "partial"
        );
        document["nftables"]
            .as_array_mut()
            .unwrap()
            .retain(|entry| {
                entry["chain"]["name"] != "input" && entry["chain"]["name"] != "output"
            });
        assert_eq!(
            status(&serde_json::to_vec(&document).unwrap()).unwrap()["state"],
            "inactive"
        );
    }

    #[test]
    fn status_rejects_invalid_reads_instead_of_reporting_inactive() {
        for input in [
            b"nft failed".as_slice(),
            b"{}",
            b"null",
            br#"{"nftables":null}"#,
        ] {
            assert!(status(input).is_err());
        }
    }

    fn entries(ruleset: &[u8]) -> Vec<Value> {
        let result = counters(ruleset).expect("reduce");
        result["counters"].as_array().expect("array").clone()
    }

    fn find<'a>(entries: &'a [Value], chain: &str, name: &str) -> Option<&'a Value> {
        entries
            .iter()
            .find(|entry| entry["chain"] == chain && entry["name"] == name)
    }

    #[test]
    fn a_split_rule_sums_into_one_entry() {
        let entries = entries(CAPTURED);
        // fw4 rendered Allow-ICMPv6-Input as two nft rules in input_wan — a type
        // set and a type/code concat — 158 + 50 packets, 9984 + 3400 bytes.
        assert_eq!(
            find(&entries, "input_wan", "Allow-ICMPv6-Input"),
            Some(&json!({
                "chain": "input_wan",
                "name": "Allow-ICMPv6-Input",
                "packets": 208,
                "bytes": 13384,
            }))
        );
    }

    #[test]
    fn only_counted_fw4_rules_become_entries() {
        let entries = entries(CAPTURED);
        // 53 nft rules reduce to 14 identities: the jumps, rejects and mangles fw4
        // renders without a counter carry no hits to report.
        assert_eq!(entries.len(), 14);
        assert!(find(&entries, "input", "Handle inbound flows").is_none());
        assert!(find(&entries, "srcnat_wan", "Masquerade IPv4 wan traffic").is_none());
        // The first entry is the first counted rule in ruleset order.
        assert_eq!(entries[0]["chain"], "accept_from_lan");
        assert_eq!(entries[0]["name"], "accept lan IPv4/IPv6 traffic");
    }

    #[test]
    fn the_same_name_in_two_chains_stays_two_entries() {
        let entries = entries(CAPTURED);
        assert_eq!(
            find(&entries, "accept_from_lan", "accept lan IPv4/IPv6 traffic")
                .map(|entry| entry["packets"].clone()),
            Some(json!(333))
        );
        assert_eq!(
            find(&entries, "accept_to_lan", "accept lan IPv4/IPv6 traffic")
                .map(|entry| entry["packets"].clone()),
            Some(json!(576))
        );
    }

    #[test]
    fn a_redirect_and_its_reflection_are_separate_identities() {
        let ruleset = json!({"nftables": [
            {"rule": {"chain": "dstnat_wan", "comment": "!fw4: web",
                      "expr": [{"counter": {"packets": 12, "bytes": 1008}}, {"dnat": null}]}},
            {"rule": {"chain": "dstnat_wan", "comment": "!fw4: web (reflection)",
                      "expr": [{"counter": {"packets": 3, "bytes": 240}}, {"dnat": null}]}},
        ]})
        .to_string();
        let entries = entries(ruleset.as_bytes());
        assert_eq!(entries.len(), 2);
        assert_eq!(
            find(&entries, "dstnat_wan", "web").map(|entry| entry["packets"].clone()),
            Some(json!(12))
        );
        assert_eq!(
            find(&entries, "dstnat_wan", "web (reflection)").map(|entry| entry["packets"].clone()),
            Some(json!(3))
        );
    }

    #[test]
    fn rules_outside_fw4s_naming_are_left_out() {
        let ruleset = json!({"nftables": [
            {"rule": {"chain": "input_wan", "comment": "hand-written include",
                      "expr": [{"counter": {"packets": 9, "bytes": 900}}]}},
            {"rule": {"chain": "input_wan",
                      "expr": [{"counter": {"packets": 9, "bytes": 900}}]}},
            {"rule": {"chain": "input_wan", "comment": "!fw4: Allow-Ping", "expr": [{"accept": null}]}},
            {"table": {"name": "fw4"}},
        ]})
        .to_string();
        assert!(entries(ruleset.as_bytes()).is_empty());
    }

    #[test]
    fn a_ruleset_that_is_not_json_is_an_error() {
        assert!(counters(b"nft: command not found").is_err());
        // Valid JSON without the nftables array simply reports nothing.
        assert!(entries(b"{}").is_empty());
    }
}
