// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

//! Whether fw4's ruleset is loaded in the kernel, read from the live nftables
//! ruleset for the Overview's firewall status. Each rule's hit counters are the
//! firewall plugin's own read (ADR-027).

use serde_json::{json, Value};

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
}
