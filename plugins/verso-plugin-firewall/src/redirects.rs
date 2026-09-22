// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

//! The Port forwards listing: the redirects that rewrite where traffic goes.
//!
//! Only the dnat direction belongs here. A snat redirect rewrites the source of
//! outbound traffic — a different question, on a different chain, answered
//! nowhere on this page — so listing it beside port forwards would suggest the
//! two are the same kind of thing.

use verso_plugin::{ColumnWidth, Envelope, TableAction, TableColumn, TableRow, Widget};

use crate::counters::Counters;
use crate::format;
use crate::model::{Firewall, Redirect};
use crate::page;

pub const HEADING: &str = "Port forwards";

pub const SUBHEADING: &str = "Send matching traffic to the router itself or to a device behind it.";

/// NOTE is what the listing leaves out, said once under the last row: a source
/// rewrite is a different question on a different chain, and a reader who knows
/// their config has one is owed the reason it is not here.
const NOTE: &str = "Source rewrites are not listed here — they change where outbound traffic \
appears to come from, not where inbound traffic goes.";

const EMPTY_TITLE: &str = "Nothing is reachable from the internet yet";

const EMPTY_BODY: &str = "A port forward lets someone outside your home reach one device inside \
it — a game server, a camera, a media box. Until you add one, the internet cannot start a \
connection to anything on your network.";

/// page renders the Port forwards listing: the bar that narrows it and the one
/// act that adds to it, then the grid. With nothing to list the bar goes too —
/// there is nothing to narrow — and the empty state carries its own invitation.
pub fn page(model: &Firewall, counters: &Counters) -> Envelope {
    let forwards: Vec<&Redirect> = model
        .redirects
        .iter()
        .filter(|redirect| redirect.is_port_forward())
        .collect();
    let children = match forwards.is_empty() {
        true => vec![empty()],
        false => vec![bar(), table(&forwards, counters)],
    };
    page::envelope(HEADING, SUBHEADING, Widget::stack(children))
}

/// bar is the listing's controls: the lens, and the one forward act.
fn bar() -> Widget {
    Widget::ActionBar {
        style: String::new(),
        tabs: Vec::new(),
        filter: "Find a forward".into(),
        live: String::new(),
        action: Some(TableAction {
            label: "Add forward".into(),
            href: page::new_redirect_href(),
            ..TableAction::default()
        }),
        opens_panel: false,
        drawer: None,
    }
}

/// empty is the invitation to make the first forward. This listing is the whole
/// page, so its nothing is a designed state with a doorway in it, not the quiet
/// row the shell draws for a listing that is one section among several.
fn empty() -> Widget {
    Widget::Empty {
        icon: "arrow-right".into(),
        title: EMPTY_TITLE.into(),
        body: EMPTY_BODY.into(),
        variant: String::new(),
        children: vec![Widget::link(
            "Add forward",
            &page::new_redirect_href(),
            "button",
        )],
    }
}

fn columns() -> Vec<TableColumn> {
    [
        ("Name", "name", ColumnWidth::Grow),
        ("Arrives on", "mono", ColumnWidth::Address),
        ("Sent to", "mono", ColumnWidth::Address),
        ("Proto", "keyword", ColumnWidth::Short),
        ("Hits", "num", ColumnWidth::Count),
        ("", "actions", ColumnWidth::Short),
    ]
    .into_iter()
    .map(|(label, kind, width)| TableColumn {
        label: label.into(),
        kind: kind.into(),
        width,
    })
    .collect()
}

fn table(forwards: &[&Redirect], counters: &Counters) -> Widget {
    Widget::Table {
        style: String::new(),
        title: String::new(),
        detail: String::new(),
        dense: true,
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
        add_label: String::new(),
        add_href: String::new(),
        note: NOTE.into(),
        stream: None,
    }
}

fn row(redirect: &Redirect, counters: &Counters) -> TableRow {
    TableRow {
        depth: 0,
        expanded: Vec::new(),
        id: redirect.section.clone(),
        key: String::new(),
        group: None,
        muted: !redirect.enabled,
        tags: Vec::new(),
        cells: vec![
            page::name_cell(&redirect.name, page::redirect_href(&redirect.section)),
            page::mono_cell(&arrives_on(redirect)),
            page::mono_cell(&sent_to(redirect)),
            page::text_cell(&format::protocol(&redirect.proto, &redirect.family)),
            page::hits_cell(counters.packets(&redirect.chain, &redirect.identity)),
            page::acts_cell(
                &redirect.section,
                redirect.enabled,
                page::redirect_href(&redirect.section),
                page::redirect_href(&redirect.section),
            ),
        ],
        drawer: None,
        panel: String::new(),
    }
}

/// arrives_on is the door the traffic knocks on: where it arrives from and the
/// port it arrives at, which together are what someone outside would type.
/// A forward restricted to particular source addresses names those instead of
/// the zone — they are what it really answers to, and a row that said only the
/// zone would promise more than the forward allows.
fn arrives_on(redirect: &Redirect) -> String {
    let source = match (redirect.src_ips.is_empty(), redirect.src.as_str()) {
        (false, _) => redirect.src_ips.join(" "),
        (true, "" | "*") => String::new(),
        (true, zone) => zone.to_string(),
    };
    match (source.as_str(), redirect.src_dport.as_str()) {
        ("", "") => String::new(),
        ("", port) => port.to_string(),
        (zone, "") => zone.to_string(),
        (zone, port) => format!("{zone} · {port}"),
    }
}

/// sent_to is where the traffic ends up: the host and the port it is handed to.
/// A forward with no address targets the router itself, and a forward that does
/// not move the port says only the address — the port is already stated to its
/// left, and repeating it would read as a second fact.
fn sent_to(redirect: &Redirect) -> String {
    let host = match redirect.dest_ip.is_empty() {
        true => "router".to_string(),
        false => redirect.dest_ip.clone(),
    };
    let port = &redirect.dest_port;
    if port.is_empty() || *port == redirect.src_dport {
        return host;
    }
    format!("{host}:{port}")
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
        fixture::widget(body, "table")["rows"]
            .as_array()
            .expect("rows")
            .clone()
    }

    #[test]
    fn the_page_carries_the_shared_firewall_frame() {
        let body = body(&Counters::default());
        assert_eq!(body["title"], HEADING);
        assert_eq!(body["width"], "wide");
        assert_eq!(body["subheading"], SUBHEADING);
        assert_eq!(body["pages"][1]["path"], "port-forwards");
        // The listing's own controls sit between the heading and the rows: the
        // lens that narrows it, and the one act that adds to it.
        let bar = &body["widget"]["children"][0];
        assert_eq!(bar["type"], "actionbar");
        assert_eq!(bar["filter"], "Find a forward");
        assert_eq!(
            bar["action"],
            serde_json::json!({
                "label": "Add forward",
                "href": "/plugins/firewall/port-forwards/new"
            })
        );
        // The grid is the page's own content, not a titled region inside it:
        // the heading already says what this page lists.
        assert_eq!(body["widget"]["children"][1]["type"], "table");
        assert_eq!(fixture::widget(&body, "table")["note"], NOTE);
        assert!(body.get("action").is_none(), "the bar carries the one act");
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

        // With nothing to list the bar goes too: there is nothing to narrow,
        // and the invitation carries its own way in.
        assert_eq!(body["widget"]["children"].as_array().map(Vec::len), Some(1));
        let empty = &body["widget"]["children"][0];
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

    // The row reads as the sentence the forward is: this arrives here, and is
    // sent there. The name leads, because that is what a person looks for.
    #[test]
    fn a_redirect_without_a_destination_address_targets_the_router() {
        let rows = rows(&body(&Counters::default()));
        assert_eq!(
            rows[0],
            serde_json::json!({
                "id": "force_dns_guest",
                "cells": [
                    {
                        "text": "Force-DNS-to-AdGuard-guest",
                        "href": "/plugins/firewall/port-forwards/force_dns_guest"
                    },
                    {"text": "guest · 53", "emphasis": true},
                    {"text": "router", "emphasis": true},
                    {"text": "tcp/udp"},
                    {"text": "—", "muted": true},
                    {"actions": [
                        {
                            "icon": "power",
                            "title": "Disable",
                            "name": "force_dns_guest",
                            "value": "off"
                        },
                        {
                            "icon": "square-pen",
                            "title": "Edit",
                            "href": "/plugins/firewall/port-forwards/force_dns_guest"
                        },
                        // Deleting is a door to the confirmation on the
                        // forward's own page, never the act itself from here.
                        {
                            "icon": "trash-2",
                            "title": "Delete",
                            "href": "/plugins/firewall/port-forwards/force_dns_guest"
                        }
                    ]}
                ]
            })
        );
    }

    #[test]
    fn every_row_leads_to_its_own_editor() {
        let body = body(&Counters::default());
        let table = fixture::widget(&body, "table");
        assert_eq!(
            table["columns"][5],
            serde_json::json!({"kind": "actions", "width": "short"})
        );
        for row in table["rows"].as_array().expect("rows") {
            let section = row["id"].as_str().expect("id");
            let href = format!("/plugins/firewall/port-forwards/{section}");
            // Both doors lead to the same page: the name a reader reaches for,
            // and the glyph at the row's trailing edge.
            assert_eq!(row["cells"][0]["href"], href);
            assert_eq!(row["cells"][5]["actions"][1]["href"], href);
            assert_eq!(row["cells"][5]["actions"][0]["name"], section);
        }
    }

    #[test]
    fn a_rewritten_port_states_both_ends() {
        let rows = rows(&body(&Counters::default()));
        let nas = &rows[1];
        assert_eq!(nas["cells"][1]["text"], "wan · 8443");
        assert_eq!(nas["cells"][2]["text"], "10.0.0.30:443");
        assert_eq!(nas["cells"][3]["text"], "tcp");
    }

    #[test]
    fn hits_come_from_the_redirects_own_nat_chain_never_its_reflection() {
        let rows = rows(&body(&fixture::counters()));
        assert_eq!(rows[1]["cells"][4], serde_json::json!({"text": "1.2k"}));
        assert_eq!(rows[0]["cells"][4], serde_json::json!({"text": "60.6k"}));
    }

    // A forward that does not move the port says only where it lands: the port
    // is already stated to its left, and repeating it would read as a second
    // fact about the traffic.
    #[test]
    fn a_forward_that_keeps_its_port_states_the_host_alone() {
        let mut model = fixture::firewall();
        for redirect in model.redirects.iter_mut() {
            redirect.src_dport = "443".into();
            redirect.dest_port = "443".into();
            redirect.dest_ip = "10.0.0.30".into();
        }
        let body = serde_json::to_value(page(&model, &Counters::default())).expect("serialize");
        assert_eq!(rows(&body)[0]["cells"][2]["text"], "10.0.0.30");
    }

    // A forward restricted to particular source addresses names those rather
    // than the zone: they are what it really answers to.
    #[test]
    fn a_source_restricted_forward_names_the_addresses_it_answers() {
        let mut model = fixture::firewall();
        for redirect in model.redirects.iter_mut() {
            redirect.src_ips = vec!["203.0.113.7".into()];
        }
        let body = serde_json::to_value(page(&model, &Counters::default())).expect("serialize");
        assert_eq!(rows(&body)[0]["cells"][1]["text"], "203.0.113.7 · 53");
    }
}
