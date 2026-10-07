// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

//! The describe hook: General's and Access's pending changes as the review
//! drawer reads them — one sentence per setting a person changed, in the words
//! the page uses for it: "Spread packet handling over all cores: on". A setting
//! is a line of its own because it is one thing a person did; everything that
//! setting took folds into its sentence — a section that had to be made for it
//! (network's globals), a time zone written with its name, each server added
//! to or taken from the list. A change it does not know keeps its raw line.

use std::collections::BTreeMap;

use verso_plugin::{Change, Description, Snapshot};

/// How a setting's value reads.
#[derive(Clone, Copy)]
enum Reads {
    /// "on" or "off", from "1"/"on" and "0"/"off".
    Flag,
    /// The value itself.
    Value,
    /// The list's values, as they now stand, joined.
    List,
}

/// A setting the drawer names: the section type it lives in, its option, its
/// label on the page, and how its value reads. `folds` are options written with
/// it that are not a change of their own (the time zone rule with its name).
struct Setting {
    config: &'static str,
    kind: &'static str,
    option: &'static str,
    label: &'static str,
    reads: Reads,
    folds: &'static [&'static str],
}

const SETTINGS: &[Setting] = &[
    Setting {
        config: "system",
        kind: "system",
        option: "hostname",
        label: "Router name",
        reads: Reads::Value,
        folds: &[],
    },
    Setting {
        config: "system",
        kind: "system",
        option: "zonename",
        label: "Time zone",
        reads: Reads::Value,
        folds: &["timezone"],
    },
    Setting {
        config: "network",
        kind: "globals",
        option: "ula_prefix",
        label: "Private IPv6 prefix",
        reads: Reads::Value,
        folds: &[],
    },
    Setting {
        config: "network",
        kind: "globals",
        option: "packet_steering",
        label: "Spread packet handling over all cores",
        reads: Reads::Flag,
        folds: &[],
    },
    Setting {
        config: "system",
        kind: "timeserver",
        option: "enabled",
        label: "Keep the clock synced over the internet",
        reads: Reads::Flag,
        folds: &[],
    },
    Setting {
        config: "system",
        kind: "timeserver",
        option: "server",
        label: "Time servers",
        reads: Reads::List,
        folds: &[],
    },
    Setting {
        config: "system",
        kind: "timeserver",
        option: "enable_server",
        label: "Provide time to devices on this network",
        reads: Reads::Flag,
        folds: &[],
    },
    Setting {
        config: "dropbear",
        kind: "dropbear",
        option: "Port",
        label: "SSH port",
        reads: Reads::Value,
        folds: &[],
    },
    Setting {
        config: "dropbear",
        kind: "dropbear",
        option: "Interface",
        label: "SSH listens on",
        reads: Reads::Value,
        folds: &[],
    },
    Setting {
        config: "dropbear",
        kind: "dropbear",
        option: "PasswordAuth",
        label: "Allow password login",
        reads: Reads::Flag,
        folds: &[],
    },
    Setting {
        config: "dropbear",
        kind: "dropbear",
        option: "RootPasswordAuth",
        label: "Allow root to log in with a password",
        reads: Reads::Flag,
        folds: &[],
    },
];

/// describe answers the shell's pending-change list with one sentence per
/// changed setting, in first-seen order.
pub fn describe(changes: &[Change], snapshot: &Snapshot) -> Vec<Description> {
    // The type a section has, from the snapshot or the change that made it.
    let kind_of = |config: &str, section: &str| -> String {
        changes
            .iter()
            .find(|c| c.op == "add-section" && c.config == config && c.section == section)
            .map(|c| c.option.clone())
            .or_else(|| snapshot.section(config, section).map(|s| s.scalar(".type")))
            .unwrap_or_default()
    };
    let setting_of = |c: &Change| -> Option<&'static Setting> {
        let kind = kind_of(&c.config, &c.section);
        SETTINGS.iter().find(|s| {
            s.config == c.config
                && s.kind == kind
                && (s.option == c.option || s.folds.contains(&c.option.as_str()))
        })
    };

    // Each setting's run of changes, keyed by where it lives, in first-seen order.
    let mut order: Vec<(String, String, &'static str)> = Vec::new();
    let mut runs: BTreeMap<(String, String, &'static str), Vec<usize>> = BTreeMap::new();
    for (i, c) in changes.iter().enumerate() {
        if c.op == "add-section" || c.op == "remove-section" {
            continue;
        }
        let Some(setting) = setting_of(c) else {
            continue;
        };
        let key = (c.config.clone(), c.section.clone(), setting.option);
        if !runs.contains_key(&key) {
            order.push(key.clone());
        }
        runs.entry(key).or_default().push(i);
    }

    // A section made for a setting folds into the first setting's sentence in it.
    let mut out = Vec::new();
    let mut folded_sections: Vec<(String, String)> = Vec::new();
    for key in &order {
        let (config, section, option) = key;
        let setting = SETTINGS
            .iter()
            .find(|s| s.config == config && s.option == *option)
            .expect("a run is keyed by a known setting");
        let mut covers = runs[key].clone();
        let place = (config.clone(), section.clone());
        if !folded_sections.contains(&place) {
            if let Some(made) = changes
                .iter()
                .position(|c| c.op == "add-section" && &c.config == config && &c.section == section)
            {
                covers.push(made);
            }
            folded_sections.push(place);
        }
        covers.sort_unstable();
        let now = reading(setting, config, section, &runs[key], changes, snapshot);
        out.push(Description::new(
            format!("{}: {now}", setting.label),
            covers,
        ));
    }
    out
}

/// reading is how the setting's value now reads, for its sentence.
fn reading(
    setting: &Setting,
    config: &str,
    section: &str,
    run: &[usize],
    changes: &[Change],
    snapshot: &Snapshot,
) -> String {
    match setting.reads {
        Reads::List => {
            let now = snapshot
                .section(config, section)
                .map(|s| s.list(setting.option))
                .unwrap_or_default();
            if now.is_empty() {
                "none".into()
            } else {
                now.join(", ")
            }
        }
        Reads::Flag | Reads::Value => {
            let own = run
                .iter()
                .rev()
                .map(|&i| &changes[i])
                .find(|c| c.option == setting.option);
            match own {
                Some(c) if c.op == "remove-option" => "not set".into(),
                Some(c) => match setting.reads {
                    Reads::Flag => match c.value.as_str() {
                        "1" | "on" => "on".into(),
                        "0" | "off" => "off".into(),
                        other => other.into(),
                    },
                    _ => c.value.clone(),
                },
                None => String::new(),
            }
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use serde_json::json;

    fn change(config: &str, op: &str, section: &str, option: &str, value: &str) -> Change {
        Change {
            config: config.into(),
            op: op.into(),
            section: section.into(),
            option: option.into(),
            value: value.into(),
        }
    }

    fn staged() -> Snapshot {
        Snapshot::from_value(json!({
            "system": {
                "sys": {".name": "sys", ".type": "system", "hostname": "edge", "zonename": "Europe/Ljubljana"},
                "ntp": {".name": "ntp", ".type": "timeserver", "server": ["time.example.org", "pool.example.org"]}
            },
            "network": {"globals": {".name": "globals", ".type": "globals", "packet_steering": "1"}},
            "dropbear": {"main": {".name": "main", ".type": "dropbear", "PasswordAuth": "off"}}
        }))
    }

    #[test]
    fn a_section_made_for_a_setting_is_that_setting_s_one_line() {
        let changes = [
            change("network", "add-section", "globals", "globals", ""),
            change("network", "set", "globals", "packet_steering", "1"),
        ];
        let d = describe(&changes, &staged());
        assert_eq!(d.len(), 1, "{d:?}");
        assert_eq!(d[0].plain, "Spread packet handling over all cores: on");
        assert_eq!(d[0].covers, vec![0, 1]);
    }

    #[test]
    fn each_setting_is_its_own_line_in_its_page_s_words() {
        let changes = [
            change("system", "set", "sys", "hostname", "edge"),
            change("dropbear", "set", "main", "PasswordAuth", "off"),
        ];
        let d = describe(&changes, &staged());
        let plain: Vec<_> = d.iter().map(|d| d.plain.as_str()).collect();
        assert_eq!(plain, ["Router name: edge", "Allow password login: off"]);
    }

    #[test]
    fn what_a_setting_writes_with_it_folds_into_its_line() {
        // The time zone rule is written with its name; the list's values each
        // change on their own. Each setting is still one line.
        let changes = [
            change("system", "set", "sys", "zonename", "Europe/Ljubljana"),
            change(
                "system",
                "set",
                "sys",
                "timezone",
                "CET-1CEST,M3.5.0,M10.5.0/3",
            ),
            change("system", "list-add", "ntp", "server", "pool.example.org"),
            change("system", "list-del", "ntp", "server", "old.example.org"),
        ];
        let d = describe(&changes, &staged());
        assert_eq!(d.len(), 2, "{d:?}");
        assert_eq!(d[0].plain, "Time zone: Europe/Ljubljana");
        assert_eq!(d[0].covers, vec![0, 1]);
        assert_eq!(
            d[1].plain,
            "Time servers: time.example.org, pool.example.org"
        );
        assert_eq!(d[1].covers, vec![2, 3]);
    }

    #[test]
    fn a_change_it_does_not_know_keeps_its_raw_line() {
        let changes = [change("network", "set", "lan", "ipaddr", "192.168.2.1")];
        assert!(describe(&changes, &staged()).is_empty());
    }
}
