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

use verso_plugin::{group_changes, section_type, Change, Description, Snapshot, SubjectKind};

use crate::model::{Firewall, CONFIG};
use crate::settings;

/// describe answers the shell's pending-change list with one sentence per changed
/// firewall object, in first-seen order.
pub fn describe(changes: &[Change], snapshot: &Snapshot) -> Vec<Description> {
    let runs = group_changes(changes, |c| {
        (c.config == CONFIG && !c.section.is_empty()).then_some(c.section.as_str())
    });
    let mut out = Vec::new();
    for (section, idxs) in runs {
        let typ = section_type(changes, snapshot, CONFIG, section);
        if typ == "defaults" {
            out.extend(describe_defaults(&idxs, changes, snapshot));
        } else if let Some(d) = describe_section(section, &typ, &idxs, changes, snapshot) {
            out.push(d);
        }
    }
    out
}

/// describe_defaults says each change to the global defaults as its row on
/// Settings does: that page is rendered from the staged config and each
/// changed option read off it, so the review uses the page's own label and
/// shows what its control now holds. A section made for the defaults folds into
/// the first row; an option the page does not draw keeps its raw line.
fn describe_defaults(idxs: &[usize], changes: &[Change], snapshot: &Snapshot) -> Vec<Description> {
    let page = settings::page(&Firewall::read(snapshot)).widget;
    let made = idxs
        .iter()
        .copied()
        .find(|&i| changes[i].op == "add-section");
    let mut out: Vec<Description> = Vec::new();
    for &i in idxs {
        let option = &changes[i].option;
        if changes[i].op == "add-section" {
            continue;
        }
        let Some(read) = page.reading(option) else {
            continue;
        };
        let mut covers = vec![i];
        if out.is_empty() {
            covers.extend(made);
            covers.sort_unstable();
        }
        let said = Description::setting(&read.label, &read.value, option, covers)
            .about(SubjectKind::Setting, &read.label);
        out.push(if read.words { said } else { said.verbatim() });
    }
    out
}

/// describe_section names what happened to one section of type `typ`, or
/// returns None to leave its changes as raw lines when the object cannot be
/// named — a removed section is already gone from the snapshot and has no type.
fn describe_section(
    section: &str,
    typ: &str,
    idxs: &[usize],
    changes: &[Change],
    snapshot: &Snapshot,
) -> Option<Description> {
    let added = idxs.iter().any(|&i| changes[i].op == "add-section");
    let removed = idxs.iter().any(|&i| changes[i].op == "remove-section");

    let sec = snapshot.section(CONFIG, section);
    let (kind, about) = kind_label(typ)?;

    // A crossing carries no name of its own — what it is, is the two zones it
    // joins — so it is named by those rather than by the `name` option every
    // other section type identifies itself with: "lan to wan" in a sentence,
    // "lan → wan" as what config history calls it.
    let (name, called) = match (typ, sec.as_ref()) {
        ("forwarding", Some(s)) => {
            let (src, dest) = (s.scalar("src"), s.scalar("dest"));
            (format!("{src} to {dest}"), format!("{src} \u{2192} {dest}"))
        }
        ("forwarding", None) => (String::new(), String::new()),
        (_, s) => {
            let name = s.map(|s| s.scalar("name")).unwrap_or_default();
            (name.clone(), name)
        }
    };
    let subject = match typ {
        "forwarding" if !name.is_empty() => format!("the crossing from {name}"),
        _ => subject(kind, &name),
    };
    let said = |plain: String| Description::new(plain, idxs.to_vec()).about(about, &called);

    if removed {
        return Some(said(format!("Removed {subject}.")));
    }
    if added {
        return Some(said(format!("Added {subject}.")));
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
        return Some(said(format!("{verb} {subject}.")));
    }
    Some(said(format!("Edited {subject}.")))
}

/// kind_label is the plain word for a firewall section type and the kind of
/// subject it is, or None for a type the drawer has no sentence for.
fn kind_label(typ: &str) -> Option<(&'static str, SubjectKind)> {
    match typ {
        "rule" => Some(("rule", SubjectKind::Rule)),
        "redirect" => Some(("port forward", SubjectKind::PortForward)),
        "zone" => Some(("zone", SubjectKind::Zone)),
        "forwarding" => Some(("crossing", SubjectKind::Crossing)),
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

    // A global default reads as its row on Settings does: the label that page
    // gives it, the value its control now shows (a switch on or off, a pick by
    // its words), and the option as its key — read off the page itself, so
    // the review can never say a setting in other words than the page.
    #[test]
    fn a_global_default_reads_as_its_settings_row() {
        let snap = snapshot(json!({"firewall": {"d": {
            ".type": "defaults", ".name": "d", "drop_invalid": "1", "input": "drop", "tcp_ecn": "2"
        }}}));
        let changes = vec![
            change("set", "d", "drop_invalid", "1"),
            change("set", "d", "input", "drop"),
            change("set", "d", "tcp_ecn", "2"),
            change("remove-option", "d", "synflood_protect", ""),
        ];
        let rows: Vec<_> = describe(&changes, &snap)
            .into_iter()
            .map(|d| {
                let s = d.setting.expect("a setting");
                (s.label, s.value, s.key, d.covers)
            })
            .collect();
        let row = |label: &str, value: &str, key: &str, at: usize| {
            (
                label.to_string(),
                value.to_string(),
                key.to_string(),
                vec![at],
            )
        };
        assert_eq!(
            rows,
            vec![
                row(
                    "Drop packets that belong to no connection",
                    "on",
                    "drop_invalid",
                    0
                ),
                row("Traffic to this router", "drop", "input", 1),
                row("Congestion notification", "Incoming only", "tcp_ecn", 2),
                row("Slow down SYN floods", "off", "synflood_protect", 3),
            ]
        );
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

    /// Each sentence names what it is about for config history: the kind of
    /// object and the name a person set, a crossing by the zones it joins,
    /// and an object with no name left to be called by its section.
    #[test]
    fn each_sentence_names_its_subject() {
        let snap = snapshot(json!({
            "firewall": {
                "r1": { ".type": "rule", ".name": "r1", "name": "Block Telnet" },
                "cfg0792bd": { ".type": "rule", ".name": "cfg0792bd" },
                "p1": { ".type": "redirect", ".name": "p1", "name": "NAS" },
                "z1": { ".type": "zone", ".name": "z1", "name": "guest" },
                "f1": { ".type": "forwarding", ".name": "f1", "src": "lan", "dest": "wan" }
            }
        }));
        let changes = vec![
            change("set", "r1", "enabled", "0"),
            change("set", "cfg0792bd", "target", "DROP"),
            change("set", "p1", "dest_port", "8080"),
            change("set", "z1", "input", "REJECT"),
            change("set", "f1", "enabled", "0"),
        ];
        let subjects: Vec<_> = describe(&changes, &snap)
            .into_iter()
            .map(|d| d.subject.map(|s| (s.kind, s.name)))
            .collect();
        assert_eq!(
            subjects,
            vec![
                Some((SubjectKind::Rule, "Block Telnet".to_string())),
                Some((SubjectKind::Rule, String::new())),
                Some((SubjectKind::PortForward, "NAS".to_string())),
                Some((SubjectKind::Zone, "guest".to_string())),
                Some((SubjectKind::Crossing, "lan \u{2192} wan".to_string())),
            ]
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
