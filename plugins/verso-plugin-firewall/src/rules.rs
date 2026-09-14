// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

//! The Rules listing: every filtering rule under the traffic path it decides.
//!
//! Rules are grouped by the chain firewall4 evaluates them in, because that is
//! the only order that explains the outcome: a packet enters exactly one chain
//! and the first matching rule there decides it. Within a group, config order is
//! evaluation order and is shown untouched; the groups themselves follow the
//! order their first rule appears in, so the page still reads as the file.
//!
//! Because position is meaning here, the rows drag: the listing declares the uci
//! config its rows are sections of, and the shell stages the dropped sequence as
//! a `uci order` on it. A drag stays inside one group, since moving a rule
//! between chains would change which packets it sees rather than when.

use verso_plugin::{Envelope, TableCell, TableColumn, TableGroup, TableRow, Widget};

use crate::counters::Counters;
use crate::format;
use crate::model::{Firewall, Rule};
use crate::page;

const SUBHEADING: &str =
    "Control which traffic the router accepts, rejects, or forwards, in evaluation order.";

const SECTION_SUB: &str = "Each packet enters the group for its traffic path. Enabled rules are \
checked from top to bottom; the first matching Accept, Reject, or Drop rule decides the packet \
and stops evaluation. If no rule matches, the zone's default policy decides.";

const EMPTY: &str = "No rules yet — every packet is decided by its zone's default policy.";

/// page renders the Rules listing. Adding a rule is the one thing this page is
/// for beyond reading it, so it is the page's action, up beside the heading.
pub fn page(model: &Firewall, counters: &Counters) -> Envelope {
    let children = vec![
        page::filter("Filter — zone, protocol, port, comment…"),
        Widget::section("Traffic rules", SECTION_SUB, vec![table(model, counters)]),
    ];
    page::envelope(SUBHEADING, Widget::stack(children)).with_action(
        "New rule",
        &page::new_rule_href(),
        "plus",
    )
}

fn columns() -> Vec<TableColumn> {
    [
        ("", "reorder"),
        ("From", "endpoint"),
        ("To", "endpoint"),
        ("Protocol", "keyword"),
        ("Match", "mono"),
        ("Action", "pill"),
        ("Comment", "comment"),
        ("Hits", "num"),
        ("", "toggle"),
        ("", "link"),
    ]
    .into_iter()
    .map(|(label, kind)| TableColumn {
        width: Default::default(),

        label: label.into(),
        kind: kind.into(),
    })
    .collect()
}

fn table(model: &Firewall, counters: &Counters) -> Widget {
    let mut rows: Vec<TableRow> = Vec::with_capacity(model.rules.len());
    for lane in lanes(&model.rules) {
        let label = format::group_label(&lane.chain, lane.shared_dest.as_deref());
        for (position, rule) in lane.rules.iter().enumerate() {
            let mut row = row(rule, counters);
            if position == 0 {
                row.group = Some(TableGroup {
                    key: Default::default(),
                    to: Default::default(),
                    add_label: Default::default(),
                    add_href: Default::default(),
                    add_panel: Default::default(),

                    label: label.clone(),
                    chain: lane.chain.clone(),
                    tally: format!("{} rules", lane.rules.len()),
                });
            }
            rows.push(row);
        }
    }
    Widget::Table {
        add_label: Default::default(),
        add_href: Default::default(),
        note: Default::default(),
        stream: Default::default(),

        style: String::new(),
        title: String::new(),
        detail: String::new(),
        dense: false,
        align: String::new(),
        reorder_config: "firewall".into(),
        reorder_label: "Rule order".into(),
        columns: columns(),
        rows,
        drawer_label: String::new(),
        drawer_icon: String::new(),
        empty_text: EMPTY.into(),
    }
}

fn row(rule: &Rule, counters: &Counters) -> TableRow {
    TableRow {
        expanded: Default::default(),
        depth: Default::default(),
        muted: Default::default(),
        tags: Default::default(),
        panel: Default::default(),

        id: rule.section.clone(),
        key: String::new(),
        group: None,
        cells: vec![
            TableCell::default(),
            page::endpoint_cell(&rule.src_ips, &rule.src),
            page::endpoint_cell(&rule.dest_ips, &rule.dest),
            page::text_cell(&format::protocol(&rule.proto, &rule.family)),
            page::mono_cell(&format::match_summary(
                &rule.dest_ports,
                &rule.icmp_types,
                &rule.limit,
            )),
            page::pill_cell(&rule.target, format::target_tone(&rule.target)),
            page::text_cell(&rule.name),
            page::hits_cell(counters.packets(&rule.chain, &rule.identity)),
            page::toggle_cell(&rule.section, rule.enabled),
            page::edit_link_cell(page::rule_href(&rule.section)),
        ],
        drawer: None,
    }
}

/// Lane is one evaluation chain's run of rules, plus the destination zone the
/// whole run shares — the label says where forwarded traffic goes only when
/// every rule in the lane agrees on it.
struct Lane<'a> {
    chain: String,
    shared_dest: Option<String>,
    rules: Vec<&'a Rule>,
}

/// lanes buckets rules by evaluation chain, keeping the chains in the order
/// their first rule appears and the rules within a chain in config order.
fn lanes(rules: &[Rule]) -> Vec<Lane<'_>> {
    let mut lanes: Vec<Lane> = Vec::new();
    for rule in rules {
        match lanes.iter_mut().find(|lane| lane.chain == rule.chain) {
            Some(lane) => lane.rules.push(rule),
            None => lanes.push(Lane {
                chain: rule.chain.clone(),
                shared_dest: None,
                rules: vec![rule],
            }),
        }
    }
    for lane in lanes.iter_mut() {
        lane.shared_dest = shared_dest(&lane.rules);
    }
    lanes
}

fn shared_dest(rules: &[&Rule]) -> Option<String> {
    let first = rules.first()?;
    if first.dest.is_empty() || first.dest == "*" {
        return None;
    }
    rules
        .iter()
        .all(|rule| rule.dest == first.dest)
        .then(|| first.dest.clone())
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::fixture;
    use serde_json::Value;

    fn body(counters: &Counters) -> Value {
        let envelope = page(&fixture::firewall(), counters);
        serde_json::to_value(&envelope).expect("serialize")
    }

    fn table_of(body: &Value) -> Value {
        fixture::table(body, "Traffic rules")
    }

    #[test]
    fn the_page_carries_the_shared_firewall_frame() {
        let body = body(&Counters::default());
        assert_eq!(body["title"], "Firewall");
        assert_eq!(body["width"], "wide");
        assert_eq!(body["subheading"], SUBHEADING);
        assert_eq!(
            body["pages"],
            serde_json::json!([
                {"label": "Rules", "path": ""},
                {"label": "Port forwards", "path": "port-forwards"},
                {"label": "Zones", "path": "zones"}
            ])
        );
        // The lens is stated on every page; the shell decides whether a page has
        // enough to sift for it to render.
        assert_eq!(body["widget"]["children"][0]["type"], "filter");
        assert_eq!(body["widget"]["children"][1]["title"], "Traffic rules");
        assert!(body.get("commit").is_none());
        assert!(body.get("notice").is_none());
    }

    #[test]
    fn rules_are_grouped_into_the_chains_that_evaluate_them() {
        let body = body(&Counters::default());
        let rows = table_of(&body)["rows"].as_array().expect("rows").clone();
        let groups: Vec<(String, String, u64)> = rows
            .iter()
            .filter_map(|row| row.get("group"))
            .map(|group| {
                (
                    group["label"].as_str().unwrap_or_default().to_string(),
                    group["chain"].as_str().unwrap_or_default().to_string(),
                    group["tally"]
                        .as_str()
                        .unwrap_or_default()
                        .split_whitespace()
                        .next()
                        .unwrap_or_default()
                        .parse::<u64>()
                        .unwrap_or_default(),
                )
            })
            .collect();
        assert_eq!(
            groups,
            vec![
                ("WAN → Router".to_string(), "input_wan".to_string(), 4),
                ("Guest → Router".to_string(), "input_guest".to_string(), 2),
                (
                    "WAN → Forwarded traffic".to_string(),
                    "forward_wan".to_string(),
                    2
                ),
                ("Guest → LAN".to_string(), "forward_guest".to_string(), 1),
            ]
        );
        // Every rule is in exactly one lane, and a lane's run is contiguous.
        assert_eq!(rows.len(), 9);
    }

    #[test]
    fn a_row_states_the_path_the_match_and_the_verdict() {
        let body = body(&Counters::default());
        let rows = table_of(&body)["rows"].as_array().expect("rows").clone();
        assert_eq!(
            rows[1],
            serde_json::json!({
                "id": "allow_ping",
                "cells": [
                    {},
                    {"endpoints": [{"kind": "zone", "label": "wan"}]},
                    {"endpoints": [{"kind": "router", "label": "router"}]},
                    {"text": "icmp · v4"},
                    {"text": "echo-request", "emphasis": true},
                    {"text": "accept", "variant": "success"},
                    {"text": "Allow-Ping"},
                    {"text": "—", "muted": true},
                    {"on": true, "name": "allow_ping"},
                    {"text": "Edit", "href": "/plugins/firewall/rules/allow_ping"}
                ]
            })
        );
    }

    // A firewall with nothing in it is not an empty grid of column headings: the
    // listing says what the absence means, and the shell draws it as one row.
    #[test]
    fn a_config_with_no_rules_says_what_that_means() {
        let mut model = fixture::firewall();
        model.rules.clear();
        let body = serde_json::to_value(page(&model, &Counters::default())).expect("serialize");
        let table = table_of(&body);
        assert_eq!(table["empty_text"], EMPTY);
        assert!(table["rows"].as_array().expect("rows").is_empty());
    }

    #[test]
    fn every_row_leads_to_its_own_editor_and_the_page_leads_to_a_blank_one() {
        let body = body(&Counters::default());
        assert_eq!(
            body["action"],
            serde_json::json!({"label": "New rule", "href": "/plugins/firewall/rules/new", "icon": "plus"}),
            "adding a rule is the page's one doorway, not a control inside it"
        );
        assert!(
            fixture::section(&body, "Traffic rules")
                .get("control")
                .is_none(),
            "the heading of the listing carries no second Add"
        );
        let table = table_of(&body);
        assert_eq!(table["columns"][9], serde_json::json!({"kind": "link"}));
        for row in table["rows"].as_array().expect("rows") {
            let section = row["id"].as_str().expect("id");
            assert_eq!(
                row["cells"][9],
                serde_json::json!({
                    "text": "Edit",
                    "href": format!("/plugins/firewall/rules/{section}")
                })
            );
        }
    }

    // Only the rules drag. A port forward and a zone are matched by their own
    // fields, not by where they sit, so a grip on those listings would offer a
    // move that means nothing.
    #[test]
    fn only_the_rules_listing_carries_the_drag() {
        let model = fixture::firewall();
        let counters = Counters::default();
        let rules = table_of(&body(&counters));
        assert_eq!(rules["reorder_config"], "firewall");
        assert_eq!(rules["columns"][0], serde_json::json!({"kind": "reorder"}));
        for row in rules["rows"].as_array().expect("rows") {
            assert_eq!(row["cells"][0], serde_json::json!({}));
            assert!(!row["id"].as_str().unwrap_or_default().is_empty());
        }

        for (envelope, listing) in [
            (
                crate::redirects::page(&model, &counters),
                "Port forwards and redirects",
            ),
            (crate::zones::page(&model), "Zones"),
        ] {
            let page = serde_json::to_value(&envelope).expect("serialize");
            let table = fixture::table(&page, listing);
            assert!(table.get("reorder_config").is_none(), "{listing}");
            assert_ne!(table["columns"][0]["kind"], "reorder", "{listing}");
        }
    }

    #[test]
    fn an_address_match_shows_the_addresses_not_the_zone() {
        let body = body(&Counters::default());
        let rows = table_of(&body)["rows"].as_array().expect("rows").clone();
        let printer = rows
            .iter()
            .find(|row| row["id"] == "family_to_printer")
            .expect("row");
        assert_eq!(
            printer["cells"][2]["endpoints"],
            serde_json::json!([{"kind": "device", "label": "10.0.31.213"}])
        );
        assert_eq!(printer["cells"][3]["text"], "tcp/udp");
        assert_eq!(
            printer["cells"][4],
            serde_json::json!({"text": "—", "muted": true})
        );
    }

    #[test]
    fn a_disabled_rule_keeps_its_row_with_the_switch_off() {
        let body = body(&Counters::default());
        let rows = table_of(&body)["rows"].as_array().expect("rows").clone();
        let telnet = rows
            .iter()
            .find(|row| row["id"] == "block_telnet")
            .expect("row");
        assert_eq!(
            telnet["cells"][5],
            serde_json::json!({"text": "drop", "variant": "danger"})
        );
        assert_eq!(
            telnet["cells"][8],
            serde_json::json!({"name": "block_telnet"})
        );
    }

    #[test]
    fn a_long_icmp_set_and_a_rate_limit_read_as_a_summary() {
        let body = body(&Counters::default());
        let rows = table_of(&body)["rows"].as_array().expect("rows").clone();
        let icmpv6 = rows
            .iter()
            .find(|row| row["cells"][6]["text"] == "Allow-ICMPv6-Input")
            .expect("row");
        assert_eq!(icmpv6["cells"][3]["text"], "icmpv6");
        assert_eq!(icmpv6["cells"][4]["text"], "11 types · ≤1000/s");
    }

    #[test]
    fn brokered_counters_fill_the_hits_column_by_chain_and_identity() {
        let body = body(&fixture::counters());
        let rows = table_of(&body)["rows"].as_array().expect("rows").clone();
        let hits = |id: &str| {
            rows.iter()
                .find(|row| row["id"] == id)
                .map(|row| row["cells"][7].clone())
                .expect("row")
        };
        assert_eq!(hits("allow_ping"), serde_json::json!({"text": "1.4k"}));
        assert_eq!(hits("allow_dhcp_renew"), serde_json::json!({"text": "0"}));
        // The unnamed section is counted under firewall4's positional identity.
        assert_eq!(hits("cfg0af21e"), serde_json::json!({"text": "296"}));
        // A rule the ruleset carries no counter for states nothing.
        assert_eq!(
            hits("block_telnet"),
            serde_json::json!({"text": "—", "muted": true})
        );
    }
}
