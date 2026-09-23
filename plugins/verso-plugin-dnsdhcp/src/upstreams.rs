// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

//! Where a question goes when this router cannot answer it — one listing, and a
//! page apiece for editing one entry.
//!
//! These are not sections: they are entries of one `list server` on the daemon,
//! and dnsmasq writes a domain-limited upstream as `/domain/server` — one string
//! doing two jobs. So a row is addressed by its position in the list, the page
//! edits the two halves separately, and any save rewrites the whole list in one
//! operation, which is the only way uci can state a change to a list.

use verso_plugin::{commit, CommitOp, Envelope, Form, TableRow, Tone, Widget};

use crate::dns;
use crate::form::{self, Errors};
use crate::model::{Dnsdhcp, Upstream, CONFIG};
use crate::page;

const SUB: &str = "Questions not answered locally go upstream — `list server`. \
A domain-limited entry routes only that domain.";

const NEW_SUB: &str = "Add a resolver this router asks when it cannot answer a name itself.";

const MISSING: &str = "That server isn’t here any more, so Verso showed you the DNS page instead.";

/// What an empty list means depends on one other option: with `noresolv` off,
/// dnsmasq falls back to the resolvers the internet connection handed this
/// router, so nothing listed is a working state; with it on, that fallback is
/// refused and there is nowhere left to ask.
const EMPTY_FALLBACK: &str = "No servers listed — lookups follow what the internet connection \
suggested.";

const EMPTY_NOWHERE: &str = "No servers listed, and the connection's own resolvers are ignored — \
nothing outside this network can be looked up.";

/// section renders the upstream listing that sits on the DNS page, plus the block
/// of options that govern how they are asked. Each row opens the entry's own
/// page; the tail adds a new one.
pub fn section(model: &Dnsdhcp, settings: Widget) -> Widget {
    let rows = model.upstreams.iter().map(row).collect();
    Widget::section(
        "Where lookups go",
        SUB,
        vec![
            page::listing(
                page::columns(&[("Server", "mono"), ("Limited to", "mono"), ("", "link")]),
                rows,
                match model.daemon.flag("noresolv", false) {
                    true => EMPTY_NOWHERE,
                    false => EMPTY_FALLBACK,
                },
                "New server",
                &page::new_server_href(),
            ),
            settings,
        ],
    )
}

fn row(upstream: &Upstream) -> TableRow {
    TableRow {
        id: format!("server-{}", upstream.index),
        cells: vec![
            page::address_cell(&upstream.server),
            page::mono_cell(&upstream.domain),
            page::edit_link_cell(page::server_href(upstream.index)),
        ],
        ..TableRow::default()
    }
}

/// blank answers a visit to the new-server page: an entry with nothing filled in.
pub fn blank() -> Envelope {
    editor(
        None,
        &Upstream {
            index: 0,
            server: String::new(),
            domain: String::new(),
        },
        &Errors::default(),
    )
}

/// edit answers a visit to one entry's page, or nothing when the sub-path names
/// no position this list holds.
pub fn edit(model: &Dnsdhcp, index: &str) -> Option<Envelope> {
    let index = index.parse::<usize>().ok()?;
    let upstream = model.upstreams.get(index)?;
    Some(editor(Some(index), upstream, &Errors::default()))
}

/// missing states that the sub-path names no entry — a stale link, or one someone
/// else removed — and answers with the DNS page.
pub fn missing(model: &Dnsdhcp) -> Envelope {
    dns::page(model).with_notice(Tone::Danger, MISSING)
}

/// create answers the new-server page's submission: the entry is appended and the
/// whole list rewritten, and the listing answers with it staged.
pub fn create(model: &mut Dnsdhcp, form: &Form) -> Envelope {
    let stated = submitted(model.upstreams.len(), form);
    let errors = validate(&stated);
    if !errors.is_empty() {
        return editor(None, &stated, &errors).with_notice(Tone::Danger, form::REFUSED);
    }
    model.upstreams.push(stated);
    let ops = rewrite(model);
    dns::page(model)
        .with_notice(Tone::Success, "Server added.")
        .with_commit(ops)
}

/// save answers one entry's page. A delete removes it and rewrites the list;
/// anything else is the page's own submission — refused onto the page, or saved
/// and answered with the listing.
pub fn save(model: &mut Dnsdhcp, index: &str, form: &Form) -> Option<Envelope> {
    let index = index.parse::<usize>().ok()?;
    if index >= model.upstreams.len() {
        return None;
    }
    if form::deletes(form) {
        model.upstreams.remove(index);
        let ops = rewrite(model);
        return Some(
            dns::page(model)
                .with_notice(Tone::Success, "Server deleted.")
                .with_commit(ops),
        );
    }

    let stated = submitted(index, form);
    let errors = validate(&stated);
    if !errors.is_empty() {
        return Some(
            editor(Some(index), &stated, &errors).with_notice(Tone::Danger, form::REFUSED),
        );
    }
    model.upstreams[index] = stated;
    let ops = rewrite(model);
    Some(
        dns::page(model)
            .with_notice(Tone::Success, "Server saved.")
            .with_commit(ops),
    )
}

/// submitted reads the page's two halves.
fn submitted(index: usize, form: &Form) -> Upstream {
    Upstream {
        index,
        server: form.get("server").trim().to_string(),
        domain: form.get("domain").trim().to_string(),
    }
}

/// editor composes the server page — the new page and the edit page, one screen.
fn editor(index: Option<usize>, upstream: &Upstream, errors: &Errors) -> Envelope {
    let (title, subheading) = match index {
        Some(_) => ("Edit server", heading(&upstream.server)),
        None => ("New server", NEW_SUB.to_string()),
    };
    let mut children = vec![Widget::Form {
        style: "page".into(),
        submit: String::new(),
        error: String::new(),
        fields: vec![
            form::text_field(
                "server",
                "Server",
                &upstream.server,
                "The resolver's address, optionally with a port as 1.1.1.1#5353.",
                errors,
            ),
            form::text_field(
                "domain",
                "Limited to domain",
                &upstream.domain,
                "Blank answers all names; set it to scope this server, e.g. corp.example.com.",
                errors,
            ),
        ],
        note: String::new(),
        target: String::new(),
    }];
    if index.is_some() {
        children.push(form::delete_form(
            "Delete server",
            &format!(
                "Delete {}? Questions it answered go to the servers left.",
                subject(&upstream.server)
            ),
        ));
    }
    page::dns_editor(title, &subheading, Widget::stack(children))
}

/// heading names the entry the page is about — the resolver's address, or its own
/// words when the entry carries none yet.
fn heading(server: &str) -> String {
    match server.is_empty() {
        true => "An unnamed server.".to_string(),
        false => server.to_string(),
    }
}

fn subject(server: &str) -> String {
    match server.is_empty() {
        true => "this server".to_string(),
        false => server.to_string(),
    }
}

/// rewrite states the whole list, renumbering what is left so a later page
/// addresses the entry the operator is looking at.
fn rewrite(model: &mut Dnsdhcp) -> Vec<CommitOp> {
    let written: Vec<String> = model
        .upstreams
        .iter()
        .map(|upstream| Upstream::written(&upstream.server, &upstream.domain))
        .collect();
    for (position, upstream) in model.upstreams.iter_mut().enumerate() {
        upstream.index = position;
    }
    let value = match written.is_empty() {
        // A list with nothing left is cleared; an empty list option is not a
        // thing uci writes.
        true => verso_plugin::Value::Null,
        false => verso_plugin::json!(written.clone()),
    };
    model.daemon.set_list("server", written);
    vec![commit(
        CONFIG,
        &model.daemon.section,
        verso_plugin::json!({ "server": value }),
    )]
}

/// validate answers dnsmasq's question about the entry it would have to parse.
fn validate(upstream: &Upstream) -> Errors {
    let mut errors = Errors::default();
    // dnsmasq reads `/domain/` with no server as "answer this domain from
    // nowhere", so an empty server is a value — but only alongside a domain.
    let server = upstream
        .server
        .split_once('#')
        .map_or((upstream.server.as_str(), None), |(address, port)| {
            (address, Some(port))
        });
    let addressed = form::valid_ipv4(server.0) || form::valid_ipv6(server.0);
    let ported = server.1.is_none_or(|port| form::valid_number(port, 65535));
    errors.check(
        "server",
        (addressed && ported) || (upstream.server.is_empty() && !upstream.domain.is_empty()),
        "Write the resolver's IP address, such as 1.1.1.1 or 2606:4700:4700::1111.",
    );
    errors.check(
        "domain",
        upstream.domain.is_empty() || upstream.domain.split('/').all(form::valid_hostname),
        "Write the domain this server answers for, such as corp.example.com.",
    );
    errors
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::fixture;
    use serde_json::Value as Json;

    fn body(env: Envelope) -> Json {
        serde_json::to_value(&env).expect("serialize")
    }

    fn listing_rows(model: &Dnsdhcp) -> Vec<Json> {
        let json = serde_json::to_value(section(model, Widget::text(""))).expect("serialize");
        json["children"][0]["rows"]
            .as_array()
            .expect("rows")
            .clone()
    }

    fn control(env: &Json, name: &str) -> Json {
        env["widget"]["children"][0]["fields"]
            .as_array()
            .expect("fields")
            .iter()
            .find(|f| f["name"] == name)
            .cloned()
            .unwrap_or_else(|| panic!("no control {name}"))
    }

    fn saved(index: &str, body_str: &str) -> (Dnsdhcp, Json) {
        let mut model = fixture::dnsdhcp();
        let env =
            save(&mut model, index, &Form::parse(body_str)).expect("the fixture holds this entry");
        (model, body(env))
    }

    #[test]
    fn every_row_leads_to_its_own_page_and_the_tail_leads_to_a_new_one() {
        let rows = listing_rows(&fixture::dnsdhcp());
        assert_eq!(rows[0]["cells"][0]["text"], "1.1.1.1");
        assert_eq!(
            rows[0]["cells"][1],
            serde_json::json!({"text": "—", "muted": true})
        );
        assert_eq!(rows[2]["cells"][0]["text"], "10.66.0.53");
        assert_eq!(rows[2]["cells"][1]["text"], "corp.example.com");
        for (index, row) in rows.iter().enumerate() {
            assert!(row.get("drawer").is_none(), "{row}");
            assert_eq!(
                row["cells"][2],
                serde_json::json!({"text": "Edit", "href": page::server_href(index)})
            );
        }
    }

    #[test]
    fn an_empty_list_says_which_of_the_two_silences_it_is() {
        let empty = |noresolv: bool| {
            let mut model = fixture::dnsdhcp();
            model.upstreams.clear();
            model.daemon.set("noresolv", noresolv.then_some("1"));
            let json = serde_json::to_value(section(&model, Widget::text(""))).expect("serialize");
            json["children"][0]["empty_text"]
                .as_str()
                .unwrap_or("")
                .to_string()
        };
        assert_eq!(empty(false), EMPTY_FALLBACK);
        assert_eq!(empty(true), EMPTY_NOWHERE);
    }

    #[test]
    fn the_edit_page_splits_the_entry_and_the_new_page_starts_empty() {
        let model = fixture::dnsdhcp();
        let scoped = body(edit(&model, "2").expect("entry"));
        assert_eq!(scoped["title"], "Edit server");
        assert_eq!(scoped["subheading"], "10.66.0.53");
        assert!(scoped.get("pages").is_none());
        assert_eq!(control(&scoped, "server")["value"], "10.66.0.53");
        assert_eq!(control(&scoped, "domain")["value"], "corp.example.com");
        assert_eq!(scoped["widget"]["children"][0]["style"], "page");
        assert_eq!(
            scoped["widget"]["children"][1]["fields"][1]["type"],
            "confirm"
        );

        let blank = body(blank());
        assert_eq!(blank["title"], "New server");
        assert_eq!(control(&blank, "server")["value"], "");
        assert_eq!(
            blank["widget"]["children"]
                .as_array()
                .expect("children")
                .len(),
            1
        );
    }

    #[test]
    fn creating_an_entry_appends_it_and_rewrites_the_list() {
        let mut model = fixture::dnsdhcp();
        let body = body(create(&mut model, &Form::parse("server=8.8.8.8")));
        assert_eq!(body["title"], "DNS");
        assert_eq!(body["notice"]["text"], "Server added.");
        assert_eq!(
            body["commit"][0]["values"]["server"],
            serde_json::json!([
                "1.1.1.1",
                "9.9.9.9",
                "/corp.example.com/10.66.0.53",
                "8.8.8.8"
            ])
        );
        assert_eq!(model.upstreams[3].server, "8.8.8.8");
    }

    #[test]
    fn saving_one_entry_rewrites_the_whole_list_and_lands_on_the_listing() {
        let (model, body) = saved("2", "server=10.66.0.54&domain=corp.example.com");
        assert_eq!(body["title"], "DNS");
        assert_eq!(
            body["commit"],
            serde_json::json!([{
                "config": "dhcp",
                "section": "dnsmasq_main",
                "values": {"server": ["1.1.1.1", "9.9.9.9", "/corp.example.com/10.66.0.54"]}
            }])
        );
        assert_eq!(model.upstreams[2].server, "10.66.0.54");
    }

    #[test]
    fn deleting_an_entry_renumbers_what_is_left() {
        let (model, body) = saved("0", "_delete=1");
        assert_eq!(body["title"], "DNS");
        assert_eq!(body["notice"]["text"], "Server deleted.");
        assert_eq!(
            body["commit"][0]["values"],
            serde_json::json!({"server": ["9.9.9.9", "/corp.example.com/10.66.0.53"]})
        );
        let positions: Vec<usize> = model.upstreams.iter().map(|up| up.index).collect();
        assert_eq!(positions, vec![0, 1]);
    }

    #[test]
    fn deleting_the_last_entry_clears_the_list() {
        let mut model = fixture::dnsdhcp();
        model.upstreams.truncate(1);
        let body = body(save(&mut model, "0", &Form::parse("_delete=1")).expect("entry"));
        assert_eq!(
            body["commit"][0]["values"],
            serde_json::json!({"server": null})
        );
    }

    #[test]
    fn an_entry_dnsmasq_would_not_read_is_marked_on_the_page_and_nothing_is_written() {
        for (index, body_str, field) in [
            ("0", "server=nowhere", "server"),
            ("0", "server=", "server"),
            ("0", "server=1.1.1.1%23http", "server"),
            ("0", "server=1.1.1.1&domain=not+a+domain", "domain"),
        ] {
            let (_, body) = saved(index, body_str);
            assert_eq!(body["title"], "Edit server", "{body_str}");
            assert!(body.get("commit").is_none(), "{body_str}");
            assert_eq!(body["notice"]["level"], "danger", "{body_str}");
            assert!(
                !control(&body, field)["error"]
                    .as_str()
                    .unwrap_or("")
                    .is_empty(),
                "{body_str}: {field} carries no error"
            );
        }

        // A domain with no server is dnsmasq's "answer this from nowhere".
        let (_, body) = saved("0", "server=&domain=ads.example.com");
        assert_eq!(
            body["commit"][0]["values"]["server"][0],
            "/ads.example.com/"
        );
    }

    #[test]
    fn a_refused_new_entry_comes_back_on_the_new_page_carrying_what_was_typed() {
        let mut model = fixture::dnsdhcp();
        let body = body(create(&mut model, &Form::parse("server=nowhere")));
        assert_eq!(body["title"], "New server");
        assert!(body.get("commit").is_none());
        assert_eq!(control(&body, "server")["value"], "nowhere");
        assert!(!control(&body, "server")["error"]
            .as_str()
            .unwrap_or("")
            .is_empty());
    }

    #[test]
    fn a_sub_path_naming_no_entry_answers_with_the_dns_page() {
        let model = fixture::dnsdhcp();
        for index in ["9", "", "last"] {
            assert!(edit(&model, index).is_none(), "{index}");
        }
        let mut model = fixture::dnsdhcp();
        assert!(save(&mut model, "9", &Form::parse("server=1.1.1.1")).is_none());
    }
}
