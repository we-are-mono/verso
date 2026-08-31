// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

//! The Zones listing and the policy baseline underneath it.
//!
//! A zone is a name for a set of networks and the policy they share, so the row
//! shows both: what the zone claims, the address ranges that means in practice,
//! and what happens to traffic the rules did not decide. The subnets are derived
//! from the network config rather than the firewall config, which names
//! interfaces and never addresses.
//!
//! The global defaults sit on the same page because they are the other half of
//! the same answer: they decide traffic no zone claims, and they are what a zone
//! policy falls back to when the zone states none.

use verso_plugin::{
    Envelope, SettingsItem, SettingsPill, SettingsToggle, TableCell, TableColumn, TableRow, Widget,
};

use crate::format;
use crate::model::{Firewall, Zone};
use crate::page;

const SUBHEADING: &str =
    "Group networks under shared policy and define the firewall's baseline behavior.";

const ZONES_SUB: &str = "A zone groups networks (or raw devices) under one policy. **Input** \
governs traffic to the router itself, **Forward** governs traffic leaving the zone, and \
**Forwards to** lists its explicit forwardings.";

const DEFAULTS_SUB: &str = "The baseline applied before any zone or rule — `config defaults`.";

/// TOGGLE_ROWS name the defaults options plainly and say what each one does.
/// The order is the order they render; the uci name is the switch's form name.
const TOGGLE_ROWS: [(&str, &str, &str); 3] = [
    (
        "drop_invalid",
        "Drop invalid packets",
        "Discard packets that don't match any known connection.",
    ),
    (
        "synflood_protect",
        "SYN-flood protection",
        "Rate-limit half-open connections to blunt basic floods.",
    ),
    (
        "flow_offloading",
        "Software flow offloading",
        "Fast-path established connections for higher throughput.",
    ),
];

/// options names the defaults switches this page renders — the closed set a post
/// to it may name, so the page and its POST handler cannot drift apart.
pub fn options() -> impl Iterator<Item = &'static str> {
    TOGGLE_ROWS.iter().map(|(option, _, _)| *option)
}

/// page renders the Zones listing and the global defaults.
pub fn page(model: &Firewall) -> Envelope {
    let mut children = Vec::new();
    children.extend(page::filter(
        "Filter — zone, network, subnet…",
        model.zones.len(),
    ));
    children.push(Widget::section("Zones", ZONES_SUB, vec![table(model)]));
    children.push(Widget::section(
        "Global defaults",
        DEFAULTS_SUB,
        vec![defaults(model)],
    ));
    page::envelope(SUBHEADING, Widget::stack(children))
}

fn columns() -> Vec<TableColumn> {
    [
        ("Zone", "name"),
        ("Networks", "mono"),
        ("Subnets", "mono"),
        ("Input", "pill"),
        ("Forward", "pill"),
        ("Forwards to", ""),
        ("NAT", "pill"),
        ("", "pill"),
    ]
    .into_iter()
    .map(|(label, kind)| TableColumn {
        label: label.into(),
        kind: kind.into(),
    })
    .collect()
}

fn table(model: &Firewall) -> Widget {
    Widget::Table {
        style: String::new(),
        title: String::new(),
        detail: String::new(),
        condensed: false,
        align: String::new(),
        reorder_config: String::new(),
        reorder_label: String::new(),
        columns: columns(),
        rows: model.zones.iter().map(|zone| row(model, zone)).collect(),
        drawer_label: String::new(),
        drawer_icon: String::new(),
    }
}

fn row(model: &Firewall, zone: &Zone) -> TableRow {
    let nat = if zone.masq {
        page::pill_cell("NAT", "info")
    } else {
        TableCell::default()
    };
    TableRow {
        id: zone.section.clone(),
        key: String::new(),
        group: None,
        cells: vec![
            page::text_cell(&zone.name),
            page::mono_cell(&membership(zone)),
            page::mono_cell(&subnets(model, zone)),
            page::pill_cell(&zone.input, format::target_tone(&zone.input)),
            page::pill_cell(&zone.forward, format::target_tone(&zone.forward)),
            page::text_cell(&model.forwards_from(&zone.name).join(", ")),
            nat,
            page::edit_cell(),
        ],
        drawer: None,
    }
}

/// membership is what the zone claims: the logical networks it covers, and any
/// kernel device it claims directly — marked as such, because a raw device is
/// not a network the rest of the config knows by name.
fn membership(zone: &Zone) -> String {
    let mut parts: Vec<String> = zone.networks.clone();
    parts.extend(
        zone.devices
            .iter()
            .map(|device| format!("{device} (device)")),
    );
    parts.join(", ")
}

/// subnets is the address space the zone's networks actually cover, as far as
/// the network config commits to one. An interface that learns its address at
/// runtime contributes nothing rather than a guess.
fn subnets(model: &Firewall, zone: &Zone) -> String {
    zone.networks
        .iter()
        .filter_map(|network| model.interface(network))
        .filter_map(|interface| {
            format::subnet_label(
                &interface.ipaddr,
                &interface.netmask,
                &interface.ip6assign,
            )
        })
        .collect::<Vec<String>>()
        .join(", ")
}

fn defaults(model: &Firewall) -> Widget {
    let defaults = &model.defaults;
    let mut items = vec![SettingsItem {
        title: "Default policies".into(),
        desc: "What happens to traffic no zone claims.".into(),
        pills: [
            ("in", &defaults.input),
            ("out", &defaults.output),
            ("fwd", &defaults.forward),
        ]
        .into_iter()
        .map(|(direction, policy)| SettingsPill {
            variant: format::tone(format::target_tone(policy)),
            text: format!("{direction}: {policy}"),
        })
        .collect(),
        ..SettingsItem::default()
    }];
    items.extend(TOGGLE_ROWS.iter().map(|(option, title, desc)| SettingsItem {
        title: (*title).into(),
        desc: (*desc).into(),
        code: (*option).into(),
        toggle: Some(SettingsToggle {
            name: (*option).into(),
            on: defaults.state(option),
        }),
        ..SettingsItem::default()
    }));
    Widget::Settings {
        style: String::new(),
        title: String::new(),
        meta: String::new(),
        items,
        seam: None,
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::fixture;
    use serde_json::Value;

    fn body() -> Value {
        serde_json::to_value(page(&fixture::firewall())).expect("serialize")
    }

    fn rows(body: &Value) -> Vec<Value> {
        fixture::table(body, "Zones")["rows"]
            .as_array()
            .expect("rows")
            .clone()
    }

    #[test]
    fn the_page_carries_the_shared_firewall_frame_and_both_sections() {
        let body = body();
        assert_eq!(body["title"], "Firewall");
        assert_eq!(body["width"], "wide");
        assert_eq!(body["subheading"], SUBHEADING);
        assert_eq!(body["pages"][2]["path"], "zones");
        assert_eq!(body["widget"]["children"][0]["title"], "Zones");
        assert_eq!(body["widget"]["children"][1]["title"], "Global defaults");
        assert!(
            body.get("action").is_none(),
            "the zones listing has no editor, so the page offers no doorway to one"
        );
    }

    // The filter is a control for a listing too long to read whole.
    #[test]
    fn the_filter_appears_only_once_the_listing_outgrows_the_screen() {
        let leads = |zones: usize| {
            let mut model = fixture::firewall();
            while model.zones.len() < zones {
                model.zones.push(fixture::firewall().zones.remove(0));
            }
            model.zones.truncate(zones);
            let body = serde_json::to_value(page(&model)).expect("serialize");
            body["widget"]["children"][0]["type"].as_str().unwrap_or_default().to_string()
        };
        assert_eq!(leads(page::FILTER_THRESHOLD), "section");
        assert_eq!(leads(page::FILTER_THRESHOLD + 1), "filter");
    }

    #[test]
    fn a_zone_row_states_membership_policy_and_forwardings() {
        let rows = rows(&body());
        assert_eq!(
            rows[0],
            serde_json::json!({
                "id": "cfg02dc81",
                "cells": [
                    {"text": "lan"},
                    {"text": "lan, lan2", "emphasis": true},
                    {"text": "10.0.0.0/24 + v6", "emphasis": true},
                    {"text": "accept", "variant": "success"},
                    {"text": "accept", "variant": "success"},
                    {"text": "wan"},
                    {},
                    {"button": "Edit", "disabled": true}
                ]
            })
        );
    }

    #[test]
    fn an_uplink_with_no_static_address_states_no_subnet_and_shows_its_nat() {
        let wan = &rows(&body())[1];
        assert_eq!(wan["cells"][1]["text"], "wan, wan6");
        assert_eq!(
            wan["cells"][2],
            serde_json::json!({"text": "—", "muted": true})
        );
        assert_eq!(
            wan["cells"][3],
            serde_json::json!({"text": "reject", "variant": "warning"})
        );
        assert_eq!(wan["cells"][6], serde_json::json!({"text": "NAT", "variant": "info"}));
        assert_eq!(
            wan["cells"][5],
            serde_json::json!({"text": "—", "muted": true})
        );
    }

    #[test]
    fn a_zone_takes_the_defaults_policy_when_it_states_none() {
        let guest = &rows(&body())[2];
        assert_eq!(guest["cells"][0]["text"], "guest");
        assert_eq!(guest["cells"][2]["text"], "10.0.20.0/24 + v6");
        assert_eq!(guest["cells"][3]["text"], "reject");
        assert_eq!(guest["cells"][4]["text"], "reject");
    }

    #[test]
    fn a_zone_claiming_a_raw_device_says_so() {
        let tailscale = &rows(&body())[3];
        assert_eq!(tailscale["cells"][1]["text"], "tailscale0 (device)");
        assert_eq!(
            tailscale["cells"][2],
            serde_json::json!({"text": "—", "muted": true})
        );
    }

    #[test]
    fn the_defaults_card_reads_the_real_baseline() {
        let body = body();
        let settings = &fixture::section(&body, "Global defaults")["children"][0];
        assert_eq!(settings["type"], "settings");
        assert_eq!(
            settings["items"][0],
            serde_json::json!({
                "title": "Default policies",
                "desc": "What happens to traffic no zone claims.",
                "pills": [
                    {"variant": "warning", "text": "in: reject"},
                    {"variant": "success", "text": "out: accept"},
                    {"variant": "warning", "text": "fwd: reject"}
                ]
            })
        );
        assert_eq!(
            settings["items"][1],
            serde_json::json!({
                "title": "Drop invalid packets",
                "desc": "Discard packets that don't match any known connection.",
                "code": "drop_invalid",
                "toggle": {"name": "drop_invalid"}
            })
        );
        assert_eq!(
            settings["items"][2]["toggle"],
            serde_json::json!({"name": "synflood_protect", "on": true})
        );
        assert_eq!(
            settings["items"][3]["toggle"],
            serde_json::json!({"name": "flow_offloading"})
        );
    }
}
