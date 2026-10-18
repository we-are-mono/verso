// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

//! What every describe hook does before it says anything: gather the pending
//! changes into the runs one sentence each will cover, and learn what kind of
//! section a run touches.

use std::collections::HashMap;
use std::hash::Hash;

use crate::{Change, Snapshot};

/// group_changes gathers the changes into runs by the key each maps to, in the
/// order the keys first appear, so the review reads top to bottom as the changes
/// were staged. Each run holds the indices, into `changes`, of its changes; a
/// change whose key is None belongs to no run and keeps its raw line.
pub fn group_changes<'a, K, F>(changes: &'a [Change], mut key: F) -> Vec<(K, Vec<usize>)>
where
    K: Eq + Hash + Clone,
    F: FnMut(&'a Change) -> Option<K>,
{
    let mut runs: Vec<(K, Vec<usize>)> = Vec::new();
    let mut at: HashMap<K, usize> = HashMap::new();
    for (i, change) in changes.iter().enumerate() {
        let Some(k) = key(change) else {
            continue;
        };
        let run = *at.entry(k.clone()).or_insert_with(|| {
            runs.push((k, Vec::new()));
            runs.len() - 1
        });
        runs[run].1.push(i);
    }
    runs
}

/// section_type is the uci type of a section the changes touch: the type its
/// `add-section` change carries, else the staged snapshot's, else empty — a
/// removed section is gone from the snapshot and made by no change.
pub fn section_type(
    changes: &[Change],
    snapshot: &Snapshot,
    config: &str,
    section: &str,
) -> String {
    changes
        .iter()
        .find(|c| c.op == "add-section" && c.config == config && c.section == section)
        .map(|c| c.option.clone())
        .or_else(|| snapshot.section(config, section).map(|s| s.scalar(".type")))
        .unwrap_or_default()
}

#[cfg(test)]
mod tests {
    use super::*;
    use serde_json::json;

    fn change(config: &str, op: &str, section: &str, option: &str) -> Change {
        Change {
            config: config.into(),
            op: op.into(),
            section: section.into(),
            option: option.into(),
            value: String::new(),
        }
    }

    #[test]
    fn runs_keep_the_order_their_keys_first_appear() {
        let changes = vec![
            change("firewall", "set", "b", "x"),
            change("firewall", "set", "a", "x"),
            change("dhcp", "set", "a", "x"),
            change("firewall", "set", "b", "y"),
        ];
        let runs = group_changes(&changes, |c| {
            (c.config == "firewall").then_some(c.section.as_str())
        });
        assert_eq!(runs, vec![("b", vec![0, 3]), ("a", vec![1])]);
    }

    #[test]
    fn a_section_is_typed_by_the_change_that_made_it_first() {
        let snapshot = Snapshot::from_value(json!({"firewall": {
            "r1": {".type": "rule", ".name": "r1"},
            "cfg9": {".type": "zone", ".name": "cfg9"}
        }}));
        let changes = vec![
            change("dhcp", "add-section", "cfg9", "host"),
            change("firewall", "add-section", "cfg9", "redirect"),
        ];
        let typed = |s| section_type(&changes, &snapshot, "firewall", s);
        assert_eq!(typed("cfg9"), "redirect");
        assert_eq!(typed("r1"), "rule");
        assert_eq!(typed("gone"), "");
    }
}
