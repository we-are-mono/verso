// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

//! The describe hook: DNS and DHCP pending changes as plain sentences for the
//! review drawer. The shell sends every coalesced
//! change to `dhcp`; this groups them by the section each touches and names what
//! happened to that object — a reservation, a network's DHCP, the service
//! settings, a DNS record — resolving the section handle to the name a person set.
//! A change it cannot name (a removed section is gone from the staged snapshot) it
//! does not cover, and the shell keeps that change's raw uci line.

use std::collections::HashMap;

use verso_plugin::{Change, Description, Section, Snapshot};

use crate::model::CONFIG;

/// describe answers the shell's pending-change list with one sentence per changed
/// object, in first-seen order.
pub fn describe(changes: &[Change], snapshot: &Snapshot) -> Vec<Description> {
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

    let sec = snapshot.section(CONFIG, section);
    let typ = idxs
        .iter()
        .find(|&&i| changes[i].op == "add-section")
        .map(|&i| changes[i].option.clone())
        .or_else(|| sec.as_ref().map(|s| s.scalar(".type")))
        .unwrap_or_default();
    let subject = subject(&typ, section, sec.as_ref())?;
    let covers = idxs.to_vec();

    let verb = if removed {
        "Removed"
    } else if added {
        "Added"
    } else {
        "Edited"
    };
    Some(Description::new(format!("{verb} {subject}."), covers))
}

/// subject names the object a sentence is about, by its uci section type, or None
/// for a type the drawer has no sentence for. It names only the *items* a person
/// lists and edits in a drawer — a DHCP server, a reservation, a DNS record — and
/// deliberately not the daemon (`dnsmasq`) section: that is a settings page,
/// where each option a person changes is its own change and stays on its own
/// line. Returning None leaves each such write to its raw line, so it counts on
/// its own.
fn subject(typ: &str, section: &str, sec: Option<&Section>) -> Option<String> {
    match typ {
        "dhcp" => {
            let network = sec
                .map(|s| s.scalar("interface"))
                .filter(|n| !n.is_empty())
                .unwrap_or_else(|| section.to_string());
            Some(format!("the DHCP server on \u{201c}{network}\u{201d}"))
        }
        "host" => {
            let name = sec.map(|s| s.scalar("name")).unwrap_or_default();
            let ip = sec.map(|s| s.scalar("ip")).unwrap_or_default();
            Some(if !name.is_empty() {
                format!("the reservation for \u{201c}{name}\u{201d}")
            } else if !ip.is_empty() {
                format!("the reservation for {ip}")
            } else {
                "the reservation".to_string()
            })
        }
        "cname" | "domain" | "srv" | "mxhost" => Some("the DNS record".to_string()),
        _ => None,
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
    fn a_new_reservation_reads_by_name_and_covers_its_writes() {
        let snap = snapshot(json!({
            "dhcp": { "host_nas": { ".type": "host", ".name": "host_nas", "name": "nas", "ip": "10.0.0.5" } }
        }));
        let changes = vec![
            change("add-section", "host_nas", "host", ""),
            change("set", "host_nas", "name", "nas"),
            change("set", "host_nas", "ip", "10.0.0.5"),
        ];
        let out = describe(&changes, &snap);
        assert_eq!(out.len(), 1);
        assert_eq!(
            out[0].plain,
            "Added the reservation for \u{201c}nas\u{201d}."
        );
        assert_eq!(out[0].covers, vec![0, 1, 2]);
    }

    #[test]
    fn a_pool_saved_from_its_drawer_reads_as_one_change_to_its_network() {
        // A DHCP server is an object edited in its own drawer, so a save is one
        // thing a person did, named by the network it serves, however many of
        // its options changed.
        let snap = snapshot(json!({
            "dhcp": { "cfg07": { ".type": "dhcp", ".name": "cfg07", "interface": "lan", "start": "100" } }
        }));
        let out = describe(
            &[
                change("set", "cfg07", "start", "50"),
                change("set", "cfg07", "leasetime", "24h"),
            ],
            &snap,
        );
        assert_eq!(out.len(), 1);
        assert_eq!(
            out[0].plain,
            "Edited the DHCP server on \u{201c}lan\u{201d}."
        );
        assert_eq!(out[0].covers, vec![0, 1]);
    }

    #[test]
    fn daemon_settings_are_not_grouped() {
        let snap = snapshot(json!({
            "dhcp": { "dnsmasq_main": { ".type": "dnsmasq", ".name": "dnsmasq_main" } }
        }));
        let out = describe(&[change("set", "dnsmasq_main", "logqueries", "1")], &snap);
        assert!(out.is_empty());
    }

    #[test]
    fn a_reservation_without_a_name_falls_back_to_its_ip() {
        let snap = snapshot(json!({
            "dhcp": { "h1": { ".type": "host", ".name": "h1", "ip": "10.0.0.9" } }
        }));
        let out = describe(&[change("set", "h1", "ip", "10.0.0.9")], &snap);
        assert_eq!(out[0].plain, "Edited the reservation for 10.0.0.9.");
    }

    #[test]
    fn a_removed_section_is_left_to_its_raw_line() {
        let snap = snapshot(json!({ "dhcp": {} }));
        assert!(describe(&[change("remove-section", "host_nas", "", "")], &snap).is_empty());
    }
}
