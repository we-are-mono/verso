// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

//! The describe hook: the firewall's pending changes rendered as plain sentences
//! for the review drawer. The shell sends every
//! coalesced change to `firewall`; this groups them by the section each touches
//! and names what happened to that object — added, removed, turned on or off, or
//! edited — resolving the section handle to the rule, forward, or zone name a
//! person set. A change it cannot name (a removed section is gone from the staged
//! snapshot) it simply does not cover, and the shell keeps that change's raw uci
//! line.

use std::collections::HashMap;

use verso_plugin::{Change, Description, Snapshot};

use crate::model::CONFIG;

/// describe answers the shell's pending-change list with one sentence per changed
/// firewall object, in first-seen order.
pub fn describe(changes: &[Change], snapshot: &Snapshot) -> Vec<Description> {
    // Group each change's position by the section it touches, keeping the order
    // the sections first appear so the drawer reads top-to-bottom as staged.
    let mut order: Vec<String> = Vec::new();
    let mut groups: HashMap<String, Vec<usize>> = HashMap::new();
    for (i, c) in changes.iter().enumerate() {
        if c.config != CONFIG || c.section.is_empty() {
            continue;
        }
        if !groups.contains_key(&c.section) {
            order.push(c.section.clone());
        }
        groups.entry(c.section.clone()).or_default().push(i);
    }

    let mut out = Vec::new();
    for section in &order {
        if let Some(d) = describe_section(section, &groups[section], changes, snapshot) {
            out.push(d);
        }
    }
    out
}

/// describe_section names what happened to one section, or returns None to leave
/// its changes as raw lines when the object cannot be named.
fn describe_section(
    section: &str,
    idxs: &[usize],
    changes: &[Change],
    snapshot: &Snapshot,
) -> Option<Description> {
    let added = idxs.iter().any(|&i| changes[i].op == "add-section");
    let removed = idxs.iter().any(|&i| changes[i].op == "remove-section");

    // A new section's type rides its add-section change; an existing one's comes
    // from the staged snapshot. A removed section is already gone from the
    // snapshot, so it has neither — and stays a raw line.
    let sec = snapshot.section(CONFIG, section);
    let typ = idxs
        .iter()
        .find(|&&i| changes[i].op == "add-section")
        .map(|&i| changes[i].option.clone())
        .or_else(|| sec.as_ref().map(|s| s.scalar(".type")))
        .unwrap_or_default();
    let kind = kind_label(&typ)?;

    // A crossing carries no name of its own — what it is, is the two zones it
    // joins — so it is named by those rather than by the `name` option every
    // other section type identifies itself with.
    let name = match typ.as_str() {
        "forwarding" => sec
            .as_ref()
            .map(|s| format!("{} to {}", s.scalar("src"), s.scalar("dest")))
            .unwrap_or_default(),
        _ => sec.as_ref().map(|s| s.scalar("name")).unwrap_or_default(),
    };
    let subject = match typ.as_str() {
        "forwarding" if !name.is_empty() => format!("the crossing from {name}"),
        _ => subject(kind, &name),
    };
    let covers = idxs.to_vec();

    if removed {
        return Some(Description::new(format!("Removed {subject}."), covers));
    }
    if added {
        return Some(Description::new(format!("Added {subject}."), covers));
    }
    // An existing section. The everyday case is the listing's on/off switch, whose
    // one change reads best as "Turned on/off"; anything wider is an edit, with the
    // raw lines beneath carrying the detail.
    if idxs.iter().all(|&i| changes[i].option == "enabled") {
        let turned_off = idxs
            .iter()
            .any(|&i| changes[i].op == "set" && changes[i].value == "0");
        let verb = if turned_off {
            "Turned off"
        } else {
            "Turned on"
        };
        return Some(Description::new(format!("{verb} {subject}."), covers));
    }
    Some(Description::new(format!("Edited {subject}."), covers))
}

/// kind_label is the plain word for a firewall section type, or None for a type
/// the drawer has no sentence for.
fn kind_label(typ: &str) -> Option<&'static str> {
    match typ {
        "rule" => Some("rule"),
        "redirect" => Some("port forward"),
        "zone" => Some("zone"),
        "forwarding" => Some("crossing"),
        _ => None,
    }
}

/// subject names the object a sentence is about: the named form where the operator
/// set a name, the bare kind otherwise (an anonymous rule the raw line pins down).
fn subject(kind: &str, name: &str) -> String {
    if name.is_empty() {
        format!("the {kind}")
    } else {
        format!("the {kind} \u{201c}{name}\u{201d}")
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use verso_plugin::json;

    fn snapshot(value: verso_plugin::Value) -> Snapshot {
        Snapshot::from_value(value)
    }

    fn change(op: &str, section: &str, option: &str, value: &str) -> Change {
        Change {
            config: CONFIG.to_string(),
            op: op.to_string(),
            section: section.to_string(),
            option: option.to_string(),
            value: value.to_string(),
        }
    }

    #[test]
    fn turning_a_named_rule_off_reads_plainly() {
        let snap = snapshot(json!({
            "firewall": { "r1": { ".type": "rule", ".name": "r1", "name": "Block Telnet" } }
        }));
        let changes = vec![change("set", "r1", "enabled", "0")];
        let out = describe(&changes, &snap);
        assert_eq!(out.len(), 1);
        assert_eq!(
            out[0].plain,
            "Turned off the rule \u{201c}Block Telnet\u{201d}."
        );
        assert_eq!(out[0].covers, vec![0]);
    }

    #[test]
    fn clearing_enabled_reads_as_turned_on() {
        let snap = snapshot(json!({
            "firewall": { "r1": { ".type": "rule", ".name": "r1", "name": "Block Telnet" } }
        }));
        let changes = vec![change("remove-option", "r1", "enabled", "")];
        let out = describe(&changes, &snap);
        assert_eq!(
            out[0].plain,
            "Turned on the rule \u{201c}Block Telnet\u{201d}."
        );
    }

    #[test]
    fn a_new_section_reads_as_added_and_covers_all_its_writes() {
        let snap = snapshot(json!({
            "firewall": { "cfg99": { ".type": "rule", ".name": "cfg99", "name": "Allow DNS" } }
        }));
        let changes = vec![
            change("add-section", "cfg99", "rule", ""),
            change("set", "cfg99", "name", "Allow DNS"),
            change("set", "cfg99", "dest_port", "53"),
        ];
        let out = describe(&changes, &snap);
        assert_eq!(out.len(), 1);
        assert_eq!(out[0].plain, "Added the rule \u{201c}Allow DNS\u{201d}.");
        assert_eq!(out[0].covers, vec![0, 1, 2]);
    }

    #[test]
    fn a_wider_edit_reads_as_edited() {
        let snap = snapshot(json!({
            "firewall": { "z1": { ".type": "zone", ".name": "z1", "name": "guest" } }
        }));
        let changes = vec![
            change("set", "z1", "input", "REJECT"),
            change("set", "z1", "forward", "ACCEPT"),
        ];
        let out = describe(&changes, &snap);
        assert_eq!(out[0].plain, "Edited the zone \u{201c}guest\u{201d}.");
        assert_eq!(out[0].covers, vec![0, 1]);
    }

    /// A crossing has no name — it is the two zones it joins — so the sentence
    /// says those. Written as a whole section, it reads as one line rather than
    /// as two raw uci sets nobody can place.
    #[test]
    fn a_crossing_is_named_by_the_zones_it_joins() {
        let snap = snapshot(json!({
            "firewall": {
                "cfg99": { ".type": "forwarding", ".name": "cfg99", "src": "lan", "dest": "wan" }
            }
        }));
        let changes = vec![
            change("add-section", "cfg99", "forwarding", ""),
            change("set", "cfg99", "src", "lan"),
            change("set", "cfg99", "dest", "wan"),
        ];
        let out = describe(&changes, &snap);
        assert_eq!(out.len(), 1);
        assert_eq!(out[0].plain, "Added the crossing from lan to wan.");
        assert_eq!(out[0].covers, vec![0, 1, 2]);

        // And one the file held switched off reads as what putting it back in
        // force is.
        let changes = vec![change("set", "cfg99", "enabled", "1")];
        assert_eq!(
            describe(&changes, &snap)[0].plain,
            "Turned on the crossing from lan to wan."
        );
    }

    #[test]
    fn a_removed_section_is_left_to_its_raw_line() {
        // Gone from the staged snapshot: no type, no name — not covered.
        let snap = snapshot(json!({ "firewall": {} }));
        let changes = vec![change("remove-section", "cfg01", "", "")];
        assert!(describe(&changes, &snap).is_empty());
    }

    #[test]
    fn an_unknown_config_is_ignored() {
        let snap = snapshot(json!({}));
        let mut c = change("set", "r1", "enabled", "0");
        c.config = "dhcp".to_string();
        assert!(describe(&[c], &snap).is_empty());
    }
}
