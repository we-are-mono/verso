// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

//! What this page's staged changes amount to, in the review: a tunnel
//! imported is one line, however many sections it took; a tunnel changed is
//! another.

use verso_plugin::{Change, Description, Snapshot};

use crate::model::CONFIG;

pub fn describe(changes: &[Change], _snapshot: &Snapshot) -> Vec<Description> {
    let mut out = Vec::new();
    let mut covered: Vec<usize> = Vec::new();
    let imported: Vec<&str> = changes
        .iter()
        .filter(|c| c.config == CONFIG && c.op == "add-section")
        .map(|c| c.section.as_str())
        .collect();
    for name in imported {
        // The zone it joined is the one a list entry names it in; the rest
        // of that zone's list, written again with it, is the same act.
        let zones: Vec<&str> = changes
            .iter()
            .filter(|c| c.config == "firewall" && c.op == "list-add" && c.value == name)
            .map(|c| c.section.as_str())
            .collect();
        let profile = format!("/{name}.ovpn");
        let covers: Vec<usize> = changes
            .iter()
            .enumerate()
            .filter(|(_, c)| {
                (c.config == CONFIG && c.section == name)
                    || (c.config == CONFIG && c.op == "file" && c.section.ends_with(&profile))
                    || (c.config == "network" && c.section == name)
                    || (c.config == "firewall" && zones.contains(&c.section.as_str()))
            })
            .map(|(i, _)| i)
            .collect();
        covered.extend(&covers);
        out.push(Description::new(
            format!("Imported the {name} tunnel"),
            covers,
        ));
    }
    let mut changed: Vec<(String, Vec<usize>)> = Vec::new();
    for (i, c) in changes.iter().enumerate() {
        if covered.contains(&i) || c.config != CONFIG {
            continue;
        }
        let what = match c.op.as_str() {
            "file" => format!(
                "Edited the {} profile",
                c.section.rsplit('/').next().unwrap_or("")
            ),
            _ => format!("Changed the {} tunnel", c.section),
        };
        match changed.iter_mut().find(|(w, _)| *w == what) {
            Some((_, covers)) => covers.push(i),
            None => changed.push((what, vec![i])),
        }
    }
    out.extend(
        changed
            .into_iter()
            .map(|(what, covers)| Description::new(what, covers)),
    );
    out
}

#[cfg(test)]
mod tests {
    use super::*;
    use verso_plugin::json;

    fn change(config: &str, op: &str, section: &str, option: &str, value: &str) -> Change {
        serde_json::from_value(json!({
            "config": config, "op": op, "section": section, "option": option, "value": value
        }))
        .unwrap()
    }

    #[test]
    fn an_import_is_one_line_however_many_sections_it_took() {
        let changes = vec![
            change("firewall", "remove-option", "cfg03dc81", "network", ""),
            change("firewall", "list-add", "cfg03dc81", "network", "wan"),
            change("firewall", "list-add", "cfg03dc81", "network", "nl_free_12"),
            change("network", "add-section", "nl_free_12", "interface", ""),
            change("network", "set", "nl_free_12", "device", "tun0"),
            change("openvpn", "add-section", "nl_free_12", "openvpn", ""),
            change("openvpn", "set", "nl_free_12", "dev", "tun0"),
            change(
                "openvpn",
                "file",
                "/etc/openvpn/nl_free_12.ovpn",
                "",
                "client",
            ),
            change("openvpn", "set", "proton", "enabled", "0"),
        ];
        let lines = describe(&changes, &Snapshot::from_value(json!({})));
        assert_eq!(lines.len(), 2);
        assert_eq!(lines[0].plain, "Imported the nl_free_12 tunnel");
        assert_eq!(lines[0].covers, (0..8).collect::<Vec<_>>());
        assert_eq!(lines[1].plain, "Changed the proton tunnel");
        assert_eq!(lines[1].covers, vec![8]);
    }

    #[test]
    fn a_profile_edited_on_its_own_says_so() {
        let changes = vec![change(
            "openvpn",
            "file",
            "/etc/openvpn/proton.ovpn",
            "",
            "client",
        )];
        let lines = describe(&changes, &Snapshot::from_value(json!({})));
        assert_eq!(lines[0].plain, "Edited the proton.ovpn profile");
    }
}
