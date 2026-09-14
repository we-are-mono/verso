// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

//! The Port forwards listing: the redirects that rewrite where traffic goes.
//!
//! Only the dnat direction belongs here. A snat redirect rewrites the source of
//! outbound traffic — a different question, on a different chain, answered
//! nowhere on this page — so listing it beside port forwards would suggest the
//! two are the same kind of thing.

use verso_plugin::{Envelope, TableColumn, TableRow, Widget};

use crate::counters::Counters;
use crate::format;
use crate::model::{Firewall, Redirect};
use crate::page;

const SUBHEADING: &str = "Send matching traffic to the router itself or to a device behind it.";

const SECTION_SUB: &str = "Rewrites matching traffic before it routes. A redirect with no \
destination address targets the router itself; one with an address is an inbound port forward.";

const EMPTY_TITLE: &str = "Nothing is reachable from the internet yet";

const EMPTY_BODY: &str = "A port forward lets someone outside your home reach one device inside \
it — a game server, a camera, a media box. Until you add one, the internet cannot start a \
connection to anything on your network.";

/// page renders the Port forwards listing. Adding a forward is the one thing
/// this page is for beyond reading it, so it is the page's action.
pub fn page(model: &Firewall, counters: &Counters) -> Envelope {
    let forwards: Vec<&Redirect> = model
        .redirects
        .iter()
        .filter(|redirect| redirect.is_port_forward())
        .collect();
    let children = vec![
        page::filter("Filter — zone, port, IP, comment…"),
        Widget::section(
            "Port forwards and redirects",
            SECTION_SUB,
            vec![content(&forwards, counters)],
        ),
    ];
    page::envelope(SUBHEADING, Widget::stack(children)).with_action(
        "Add forward",
        &page::new_redirect_href(),
        "plus",
    )
}

/// content is the listing, or — with nothing to list — the invitation to make
/// the first one. This listing is the whole page, so its nothing is a designed
/// state with a doorway in it, not the quiet row the shell draws for a listing
/// that is one section among several.
fn content(forwards: &[&Redirect], counters: &Counters) -> Widget {
    if forwards.is_empty() {
        return Widget::Empty {
            variant: Default::default(),

            icon: "arrow-right".into(),
            title: EMPTY_TITLE.into(),
            body: EMPTY_BODY.into(),
            children: vec![Widget::link(
                "Add forward",
                &page::new_redirect_href(),
                "button",
            )],
        };
    }
    table(forwards, counters)
}

fn columns() -> Vec<TableColumn> {
    [
        ("From", "endpoint"),
        ("To", "endpoint"),
        ("Protocol", "keyword"),
        ("Port", "mono"),
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

fn table(forwards: &[&Redirect], counters: &Counters) -> Widget {
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
        reorder_config: String::new(),
        reorder_label: String::new(),
        columns: columns(),
        rows: forwards
            .iter()
            .map(|redirect| row(redirect, counters))
            .collect(),
        drawer_label: String::new(),
        drawer_icon: String::new(),
        // Nothing to forward is answered above, by the page's empty state.
        empty_text: String::new(),
    }
}

fn row(redirect: &Redirect, counters: &Counters) -> TableRow {
    let destination: Vec<String> = if redirect.dest_ip.is_empty() {
        Vec::new()
    } else {
        vec![redirect.dest_ip.clone()]
    };
    TableRow {
        expanded: Default::default(),
        depth: Default::default(),
        muted: Default::default(),
        tags: Default::default(),
        panel: Default::default(),

        id: redirect.section.clone(),
        key: String::new(),
        group: None,
        cells: vec![
            page::endpoint_cell(&redirect.src_ips, &redirect.src),
            // A redirect with no destination address sends the traffic to the
            // router itself, which is exactly what an empty zone means too.
            page::endpoint_cell(&destination, ""),
            page::text_cell(&format::protocol(&redirect.proto, &redirect.family)),
            page::mono_cell(&port_label(&redirect.src_dport, &redirect.dest_port)),
            page::text_cell(&redirect.name),
            page::hits_cell(counters.packets(&redirect.chain, &redirect.identity)),
            page::toggle_cell(&redirect.section, redirect.enabled),
            page::edit_link_cell(page::redirect_href(&redirect.section)),
        ],
        drawer: None,
    }
}

/// port_label states the rewrite the redirect performs: the incoming port and
/// the port it becomes, or the single port when the redirect only moves traffic
/// to another host.
fn port_label(incoming: &str, destination: &str) -> String {
    match (incoming, destination) {
        ("", port) | (port, "") => port.to_string(),
        (incoming, destination) if incoming == destination => incoming.to_string(),
        (incoming, destination) => format!("{incoming} → {destination}"),
    }
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

    fn rows(body: &Value) -> Vec<Value> {
        fixture::table(body, "Port forwards and redirects")["rows"]
            .as_array()
            .expect("rows")
            .clone()
    }

    #[test]
    fn the_page_carries_the_shared_firewall_frame() {
        let body = body(&Counters::default());
        assert_eq!(body["title"], "Firewall");
        assert_eq!(body["width"], "wide");
        assert_eq!(body["subheading"], SUBHEADING);
        assert_eq!(body["pages"][1]["path"], "port-forwards");
        assert_eq!(body["widget"]["children"][0]["type"], "filter");
        assert_eq!(
            body["widget"]["children"][1]["title"],
            "Port forwards and redirects"
        );
        assert_eq!(
            body["action"],
            serde_json::json!({
                "label": "Add forward",
                "href": "/plugins/firewall/port-forwards/new",
                "icon": "plus"
            })
        );
    }

    // Nothing forwarded is not an empty table under column headings — it is a
    // page with nothing on it, saying what putting something there would mean.
    #[test]
    fn a_config_with_no_port_forwards_invites_the_first_one() {
        let mut model = fixture::firewall();
        model
            .redirects
            .retain(|redirect| !redirect.is_port_forward());
        let body = serde_json::to_value(page(&model, &Counters::default())).expect("serialize");

        let section = fixture::section(&body, "Port forwards and redirects");
        let empty = &section["children"][0];
        assert_eq!(empty["type"], "empty");
        assert_eq!(empty["icon"], "arrow-right");
        assert_eq!(empty["title"], EMPTY_TITLE);
        assert_eq!(
            empty["children"][0],
            serde_json::json!({
                "type": "link",
                "label": "Add forward",
                "href": "/plugins/firewall/port-forwards/new",
                "style": "button"
            })
        );
    }

    #[test]
    fn only_the_dnat_direction_is_listed() {
        let ids: Vec<String> = rows(&body(&Counters::default()))
            .iter()
            .map(|row| row["id"].as_str().unwrap_or_default().to_string())
            .collect();
        assert_eq!(ids, vec!["force_dns_guest", "https_to_nas"]);
    }

    #[test]
    fn a_redirect_without_a_destination_address_targets_the_router() {
        let rows = rows(&body(&Counters::default()));
        assert_eq!(
            rows[0],
            serde_json::json!({
                "id": "force_dns_guest",
                "cells": [
                    {"endpoints": [{"kind": "zone", "label": "guest"}]},
                    {"endpoints": [{"kind": "router", "label": "router"}]},
                    {"text": "tcp/udp"},
                    {"text": "53", "emphasis": true},
                    {"text": "Force-DNS-to-AdGuard-guest"},
                    {"text": "—", "muted": true},
                    {"on": true, "name": "force_dns_guest"},
                    {"text": "Edit", "href": "/plugins/firewall/port-forwards/force_dns_guest"}
                ]
            })
        );
    }

    #[test]
    fn every_row_leads_to_its_own_editor() {
        let body = body(&Counters::default());
        let table = fixture::table(&body, "Port forwards and redirects");
        assert_eq!(table["columns"][7], serde_json::json!({"kind": "link"}));
        for row in table["rows"].as_array().expect("rows") {
            let section = row["id"].as_str().expect("id");
            assert_eq!(
                row["cells"][7],
                serde_json::json!({
                    "text": "Edit",
                    "href": format!("/plugins/firewall/port-forwards/{section}")
                })
            );
        }
    }

    #[test]
    fn a_rewritten_port_states_both_ends() {
        let rows = rows(&body(&Counters::default()));
        let nas = &rows[1];
        assert_eq!(
            nas["cells"][1]["endpoints"],
            serde_json::json!([{"kind": "device", "label": "10.0.0.30"}])
        );
        assert_eq!(nas["cells"][3]["text"], "8443 → 443");
        assert_eq!(nas["cells"][2]["text"], "tcp");
    }

    #[test]
    fn hits_come_from_the_redirects_own_nat_chain_never_its_reflection() {
        let rows = rows(&body(&fixture::counters()));
        assert_eq!(rows[1]["cells"][5], serde_json::json!({"text": "1.2k"}));
        assert_eq!(rows[0]["cells"][5], serde_json::json!({"text": "60.6k"}));
    }

    #[test]
    fn a_port_pair_reads_as_the_rewrite_it_performs() {
        assert_eq!(port_label("8443", "443"), "8443 → 443");
        assert_eq!(port_label("53", "53"), "53");
        assert_eq!(port_label("53", ""), "53");
        assert_eq!(port_label("", "53"), "53");
        assert_eq!(port_label("", ""), "");
    }
}
