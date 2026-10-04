// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

//! The Port forwards listing: the redirects that rewrite where traffic goes.
//!
//! Only the dnat direction belongs here. A snat redirect rewrites the source of
//! outbound traffic — a different question, on a different chain, answered
//! nowhere on this page — so listing it beside port forwards would suggest the
//! two are the same kind of thing.
//!
//! Each row opens the forward's own panel beside the listing, and Add opens the
//! same panel blank (redirect_drawer). A forward is opened by address, so the
//! panel survives a reload and travels in a link.

use verso_plugin::{
    commit, commit_delete, commit_new, ColumnWidth, Envelope, Form, HeadingAct, RowDrawer,
    Snapshot, Table, TableColumn, TableRow, Tone, Widget,
};

use crate::counters::Counters;
use crate::fields::REMOVE_FIELD;
use crate::format;
use crate::model::{Firewall, Redirect, CONFIG};
use crate::page;
use crate::redirect_drawer::{self, NEW, OPEN};
use crate::redirect_form::RedirectForm;
use crate::rule_form::Errors;

pub const HEADING: &str = "Port forwards";

/// NOTE is what the listing leaves out, said once under the last row: a source
/// rewrite is a different question on a different chain, and a reader who knows
/// their config has one is owed the reason it is not here.
const NOTE: &str = "Source rewrites are not listed here — they change where outbound traffic \
appears to come from, not where inbound traffic goes.";

/// EMPTY is the listing with nothing in it: that there is nothing yet, and what
/// the firewall does without one.
const EMPTY: &str =
    "No port forwards yet — the internet cannot start a connection to anything on your network.";

const REFUSED: &str =
    "Some values aren’t ones the firewall accepts, so nothing was saved. They’re marked below.";

const GONE: &str = "That port forward is no longer in the config, so there was nothing to save.";

/// page renders the Port forwards listing: the bar that narrows it and the one
/// act that adds to it, then the grid — which, empty, says so in one row.
pub fn page(model: &Firewall, counters: &Counters) -> Envelope {
    listing(model, counters, None, None)
}

/// page_open is the listing with the panel its address names in front of it: a
/// forward's, read from the config, or a blank one. An address naming no port
/// forward opens nothing — a stale link lands on the listing.
pub fn page_open(
    snapshot: &Snapshot,
    model: &Firewall,
    counters: &Counters,
    query: &Form,
) -> Envelope {
    let section = query.get(OPEN).trim().to_string();
    if section == NEW {
        return blank(
            model,
            counters,
            &RedirectForm::default(),
            &Errors::default(),
        );
    }
    let open = forward(model, &section)
        .and_then(|_| snapshot.section(CONFIG, &section))
        .map(|uci| Open {
            section,
            form: RedirectForm::read(&uci),
            errors: Errors::default(),
        });
    listing(model, counters, None, open.as_ref())
}

/// save answers the open panel, or a removal confirmed on a row. A refusal stays
/// with its controls; a removal answers with the listing the forward is leaving.
pub fn save(model: &mut Firewall, counters: &Counters, query: &Form, form: &Form) -> Envelope {
    let removal = form.get(REMOVE_FIELD).trim().to_string();
    let section = match removal.is_empty() {
        true => query.get(OPEN).trim().to_string(),
        false => removal.clone(),
    };

    if section == NEW && removal.is_empty() {
        let redirect = RedirectForm::submitted(form);
        let errors = redirect.validate(&model.zone_names(), &model.ipsets);
        let answer = blank(model, counters, &redirect, &errors);
        if !errors.is_empty() {
            return answer.with_notice(Tone::Danger, REFUSED);
        }
        return answer
            .with_notice(Tone::Success, "Port forward added.")
            .with_commit(vec![commit_new(CONFIG, "redirect", redirect.values(false))]);
    }

    let Some(index) = model
        .redirects
        .iter()
        .position(|redirect| redirect.section == section && redirect.is_port_forward())
    else {
        return page(model, counters).with_notice(Tone::Danger, GONE);
    };
    if !removal.is_empty() {
        let removed = model.redirects.remove(index);
        return page(model, counters)
            .with_notice(Tone::Success, "Port forward deleted.")
            .with_commit(vec![commit_delete(CONFIG, &removed.section)]);
    }

    let redirect = RedirectForm::submitted(form);
    let errors = redirect.validate(&model.zone_names(), &model.ipsets);
    let open = Open {
        section: section.clone(),
        form: redirect,
        errors,
    };
    let answer = listing(model, counters, None, Some(&open));
    if !open.errors.is_empty() {
        return answer.with_notice(Tone::Danger, REFUSED);
    }
    answer
        .with_notice(Tone::Success, "Port forward saved.")
        .with_commit(vec![commit(CONFIG, &section, open.form.values(true))])
}

/// Open is the forward whose panel is in front of the listing: which one, the
/// values its controls start from, and whatever the last submission got wrong.
struct Open {
    section: String,
    form: RedirectForm,
    errors: Errors,
}

/// forward is the port forward a section names, if it names one.
fn forward<'a>(model: &'a Firewall, section: &str) -> Option<&'a Redirect> {
    model
        .redirects
        .iter()
        .find(|redirect| redirect.section == section && redirect.is_port_forward())
}

/// blank is the listing with the panel for a forward not made yet.
fn blank(model: &Firewall, counters: &Counters, form: &RedirectForm, errors: &Errors) -> Envelope {
    let drawer = redirect_drawer::drawer(model, None, form, errors);
    listing(model, counters, Some(drawer), None)
}

fn listing(
    model: &Firewall,
    counters: &Counters,
    blank: Option<RowDrawer>,
    open: Option<&Open>,
) -> Envelope {
    let forwards: Vec<&Redirect> = model
        .redirects
        .iter()
        .filter(|redirect| redirect.is_port_forward())
        .collect();
    page::envelope(HEADING, table(model, &forwards, counters, open)).with_act(act(blank))
}

/// act is the page's one forward act. Making a forward is editing one that
/// does not exist yet, so it opens the same panel a row's edit glyph opens.
fn act(blank: Option<RowDrawer>) -> HeadingAct {
    HeadingAct {
        label: "Add forward".into(),
        href: redirect_drawer::new_href(),
        opens_panel: true,
        drawer: blank,
        ..Default::default()
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

fn table(
    model: &Firewall,
    forwards: &[&Redirect],
    counters: &Counters,
    open: Option<&Open>,
) -> Widget {
    Widget::Table(Table {
        dense: true,
        columns: columns(),
        rows: forwards
            .iter()
            .map(|redirect| row(model, redirect, counters, open))
            .collect(),
        empty_text: EMPTY.into(),
        note: NOTE.into(),
        ..Default::default()
    })
}

/// row is one forward. Its name and its edit act both open the forward's panel
/// where it stands; its trash act asks on the row before anything is removed.
fn row(
    model: &Firewall,
    redirect: &Redirect,
    counters: &Counters,
    open: Option<&Open>,
) -> TableRow {
    let door = redirect_drawer::href(&redirect.section);
    let drawer = open
        .filter(|open| open.section == redirect.section)
        .map(|open| redirect_drawer::drawer(model, Some(redirect), &open.form, &open.errors));
    TableRow {
        depth: 0,
        expanded: Vec::new(),
        id: redirect.section.clone(),
        key: String::new(),
        group: None,
        muted: !redirect.enabled,
        tags: Vec::new(),
        cells: vec![
            page::name_cell(&redirect.name, door.clone()),
            page::mono_cell(&arrives_on(redirect)),
            page::mono_cell(&sent_to(redirect)),
            page::text_cell(&format::protocol(&redirect.proto, &redirect.family)),
            page::hits_cell(counters.packets(&redirect.chain, &redirect.identity)),
            page::acts_cell(
                &redirect.section,
                redirect.enabled,
                door.clone(),
                page::remove_act(
                    &redirect.section,
                    redirect_drawer::DELETE_TRIGGER,
                    redirect_drawer::DELETE_QUESTION,
                    redirect_drawer::DELETE_MESSAGE,
                ),
            ),
        ],
        drawer,
        // The row ships an empty frame and fetches this address when it opens,
        // so either of its doors opens the panel where it stands.
        panel: door,
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
        assert_eq!(body["pages"][1]["path"], "port-forwards");
        // The page's one act stands on its heading line, and opens the panel
        // rather than a page; nothing narrows a still listing.
        assert_eq!(
            body["act"],
            serde_json::json!({
                "label": "Add forward",
                "href": "/plugins/firewall/port-forwards?open=new",
                "opens_panel": true
            })
        );
        // The grid is the page itself, not a titled region inside it: the
        // heading already says what this page lists.
        assert_eq!(body["widget"]["type"], "table");
        assert_eq!(body["widget"]["note"], NOTE);
    }

    // Nothing forwarded is one row where the first forward would sit, saying
    // what the firewall does without one; the page's Add forward is the way in.
    #[test]
    fn a_config_with_no_port_forwards_says_so_in_one_row() {
        let mut model = fixture::firewall();
        model
            .redirects
            .retain(|redirect| !redirect.is_port_forward());
        let body = serde_json::to_value(page(&model, &Counters::default())).expect("serialize");

        assert_eq!(body["act"]["label"], "Add forward");
        let table = &body["widget"];
        assert_eq!(table["type"], "table");
        assert_eq!(table["rows"].as_array().map(Vec::len).unwrap_or(0), 0);
        assert_eq!(table["empty_text"], EMPTY);
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
                        "href": "/plugins/firewall/port-forwards?open=force_dns_guest"
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
                            "href": "/plugins/firewall/port-forwards?open=force_dns_guest"
                        },
                        // Deleting is asked on the row itself, and only
                        // then posted.
                        {
                            "icon": "trash-2",
                            "title": "Delete port forward",
                            "name": "_remove",
                            "value": "force_dns_guest",
                            "confirm_title": "Delete port forward “%s”?",
                            "confirm": crate::redirect_drawer::DELETE_MESSAGE
                        }
                    ]}
                ],
                "panel": "/plugins/firewall/port-forwards?open=force_dns_guest"
            })
        );
    }

    #[test]
    fn every_row_opens_its_own_panel() {
        let body = body(&Counters::default());
        let table = fixture::widget(&body, "table");
        assert_eq!(
            table["columns"][5],
            serde_json::json!({"kind": "actions", "width": "short"})
        );
        for row in table["rows"].as_array().expect("rows") {
            let section = row["id"].as_str().expect("id");
            let href = format!("/plugins/firewall/port-forwards?open={section}");
            // The name and the glyph at the row's trailing edge name the same
            // panel; the glyph is the way in.
            assert_eq!(row["cells"][0]["href"], href);
            assert_eq!(row["cells"][5]["actions"][1]["href"], href);
            assert_eq!(row["panel"], href);
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
