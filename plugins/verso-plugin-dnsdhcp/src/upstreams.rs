// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

//! Where a question goes when this router cannot answer it.
//!
//! These are not sections: they are entries of one `list server` on the daemon,
//! and dnsmasq writes a domain-limited upstream as `/domain/server` — one string
//! doing two jobs. So a row is addressed by its position in the list, the drawer
//! edits the two halves separately, and any save rewrites the whole list in one
//! operation, which is the only way uci can state a change to a list.

use verso_plugin::{commit, CommitOp, Form, TableRow, Widget};

use crate::form::{self, Errors};
use crate::model::{Dnsdhcp, Upstream, CONFIG};
use crate::page;

const SUB: &str = "Questions not answered locally go upstream — `list server`. \
A domain-limited entry routes only that domain.";

/// What an empty list means depends on one other option: with `noresolv` off,
/// dnsmasq falls back to the resolvers the internet connection handed this
/// router, so nothing listed is a working state; with it on, that fallback is
/// refused and there is nowhere left to ask.
const EMPTY_FALLBACK: &str = "No servers listed — lookups follow what the internet connection \
suggested.";

const EMPTY_NOWHERE: &str = "No servers listed, and the connection's own resolvers are ignored — \
nothing outside this network can be looked up.";

/// Refusal is an upstream the operator stated and dnsmasq would not read.
pub struct Refusal {
    pub index: usize,
    pub upstream: Upstream,
    pub errors: Errors,
}

/// Saved is what an upstream submission amounts to.
pub enum Saved {
    Ops(Vec<CommitOp>, &'static str),
    Refused(Refusal),
    Unknown,
}

/// section renders the upstream listing plus the block of options that govern
/// how they are asked.
pub fn section(model: &Dnsdhcp, refusal: Option<&Refusal>, settings: Widget) -> Widget {
    let rows = model
        .upstreams
        .iter()
        .map(|upstream| match refusal {
            Some(refused) if refused.index == upstream.index => {
                row(&refused.upstream, &refused.errors, true)
            }
            _ => row(upstream, &Errors::default(), false),
        })
        .collect();
    Widget::section(
        "Where lookups go",
        SUB,
        vec![
            page::table(
                page::columns(&[("Server", "mono"), ("Limited to", "mono")]),
                rows,
                match model.daemon.flag("noresolv", false) {
                    true => EMPTY_NOWHERE,
                    false => EMPTY_FALLBACK,
                },
            ),
            settings,
        ],
    )
}

fn row(upstream: &Upstream, errors: &Errors, open: bool) -> TableRow {
    TableRow {
        id: format!("server-{}", upstream.index),
        cells: vec![
            page::address_cell(&upstream.server),
            page::mono_cell(&upstream.domain),
        ],
        drawer: drawer(upstream, errors, open),
        ..TableRow::default()
    }
}

fn drawer(upstream: &Upstream, errors: &Errors, open: bool) -> Option<verso_plugin::RowDrawer> {
    let index = upstream.index.to_string();
    let title = match upstream.server.is_empty() {
        true => "Edit upstream".to_string(),
        false => format!("Edit upstream — {}", upstream.server),
    };
    form::drawer(
        &title,
        open,
        vec![
            Widget::Form {
                style: String::new(),
                submit: "Save".into(),
                error: String::new(),
                fields: vec![
                    Widget::hidden(form::KIND, form::SERVER),
                    Widget::hidden(form::INDEX, &index),
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
            },
            form::delete_form(
                form::SERVER,
                (form::INDEX, &index),
                "Delete server",
                &format!(
                    "Delete upstream {}? Questions it answered go to the servers left.",
                    subject(&upstream.server)
                ),
            ),
        ],
    )
}

fn subject(server: &str) -> String {
    match server.is_empty() {
        true => "this entry".to_string(),
        false => server.to_string(),
    }
}

/// save answers an upstream drawer's submission.
pub fn save(model: &mut Dnsdhcp, form: &Form) -> Saved {
    let Some(index) = form.get(form::INDEX).parse::<usize>().ok() else {
        return Saved::Unknown;
    };
    if index >= model.upstreams.len() {
        return Saved::Unknown;
    }
    if form::deletes(form) {
        model.upstreams.remove(index);
        return Saved::Ops(rewrite(model), "Upstream deleted.");
    }

    let stated = Upstream {
        index,
        server: form.get("server").trim().to_string(),
        domain: form.get("domain").trim().to_string(),
    };
    let errors = validate(&stated);
    if !errors.is_empty() {
        return Saved::Refused(Refusal {
            index,
            upstream: stated,
            errors,
        });
    }
    model.upstreams[index] = stated;
    Saved::Ops(rewrite(model), "Upstream saved.")
}

/// rewrite states the whole list, renumbering what is left so a later drawer
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
    let server = upstream.server.split_once('#').map_or(
        (upstream.server.as_str(), None),
        |(address, port)| (address, Some(port)),
    );
    let addressed = form::valid_ipv4(server.0) || form::valid_ipv6(server.0);
    let ported = server.1.is_none_or(|port| form::valid_number(port, 65535));
    errors.check(
        "server",
        (addressed && ported) || (upstream.server.is_empty() && !upstream.domain.is_empty()),
        "Write the resolver's IP address, such as 1.1.1.1 or 2606:4700:4700::1111.",
    );
    errors.check(
        "domain",
        upstream.domain.is_empty()
            || upstream
                .domain
                .split('/')
                .all(form::valid_hostname),
        "Write the domain this server answers for, such as corp.example.com.",
    );
    errors
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::fixture;
    use serde_json::Value as Json;

    fn rows(model: &Dnsdhcp, refusal: Option<&Refusal>) -> Vec<Json> {
        let json = serde_json::to_value(section(model, refusal, Widget::text("")))
            .expect("serialize");
        json["children"][0]["rows"].as_array().expect("rows").clone()
    }

    fn submit(body: &str) -> (Dnsdhcp, Saved) {
        let mut model = fixture::dnsdhcp();
        let saved = save(&mut model, &Form::parse(body));
        (model, saved)
    }

    fn ops(saved: &Saved) -> Json {
        match saved {
            Saved::Ops(ops, _) => serde_json::to_value(ops).expect("serialize"),
            _ => panic!("the submission was not written"),
        }
    }

    #[test]
    fn an_entry_renders_split_and_carries_its_position() {
        let rows = rows(&fixture::dnsdhcp(), None);
        assert_eq!(rows[0]["cells"][0]["text"], "1.1.1.1");
        assert_eq!(rows[0]["cells"][1], serde_json::json!({"text": "—", "muted": true}));
        assert_eq!(rows[2]["cells"][0]["text"], "10.66.0.53");
        assert_eq!(rows[2]["cells"][1]["text"], "corp.example.com");

        let fields = &rows[2]["drawer"]["children"][0]["fields"];
        assert_eq!(
            fields[1],
            serde_json::json!({"type": "field", "name": "_index", "kind": "hidden", "value": "2"})
        );
        assert_eq!(fields[2]["value"], "10.66.0.53");
        assert_eq!(fields[3]["value"], "corp.example.com");
    }

    // An empty list is two different states, and saying the wrong one would tell
    // an operator their lookups work when nothing can answer them.
    #[test]
    fn an_empty_list_says_which_of_the_two_silences_it_is() {
        let empty = |noresolv: bool| {
            let mut model = fixture::dnsdhcp();
            model.upstreams.clear();
            model.daemon.set("noresolv", noresolv.then_some("1"));
            let json = serde_json::to_value(section(&model, None, Widget::text("")))
                .expect("serialize");
            json["children"][0]["empty_text"].as_str().unwrap_or("").to_string()
        };
        assert_eq!(empty(false), EMPTY_FALLBACK);
        assert_eq!(empty(true), EMPTY_NOWHERE);
    }

    #[test]
    fn saving_one_entry_rewrites_the_whole_list() {
        let (model, saved) = submit("_form=server&_index=2&server=10.66.0.54&domain=corp.example.com");
        assert_eq!(
            ops(&saved),
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
        let (model, saved) = submit("_form=server&_index=0&_delete=1");
        assert_eq!(
            ops(&saved)[0]["values"],
            serde_json::json!({"server": ["9.9.9.9", "/corp.example.com/10.66.0.53"]})
        );
        let positions: Vec<usize> = model.upstreams.iter().map(|up| up.index).collect();
        assert_eq!(positions, vec![0, 1]);
        assert_eq!(model.upstreams[0].server, "9.9.9.9");
    }

    #[test]
    fn deleting_the_last_entry_clears_the_list() {
        let mut model = fixture::dnsdhcp();
        model.upstreams.truncate(1);
        let saved = save(&mut model, &Form::parse("_form=server&_index=0&_delete=1"));
        assert_eq!(ops(&saved)[0]["values"], serde_json::json!({"server": null}));
    }

    #[test]
    fn an_entry_dnsmasq_would_not_read_is_marked_and_nothing_is_written() {
        for (body, field) in [
            ("_form=server&_index=0&server=nowhere", "server"),
            ("_form=server&_index=0&server=", "server"),
            ("_form=server&_index=0&server=1.1.1.1%23http", "server"),
            ("_form=server&_index=0&server=1.1.1.1&domain=not+a+domain", "domain"),
        ] {
            let (_, saved) = submit(body);
            let Saved::Refused(refusal) = saved else {
                panic!("{body}: the submission should have been refused");
            };
            assert!(!refusal.errors.get(field).is_empty(), "{body}: {field} carries no error");
        }

        // A domain with no server is dnsmasq's "answer this from nowhere".
        let (_, saved) = submit("_form=server&_index=0&server=&domain=ads.example.com");
        assert_eq!(
            ops(&saved)[0]["values"]["server"][0],
            "/ads.example.com/"
        );
    }

    #[test]
    fn a_refused_entry_comes_back_open_carrying_what_was_typed() {
        let mut model = fixture::dnsdhcp();
        let Saved::Refused(refusal) =
            save(&mut model, &Form::parse("_form=server&_index=1&server=nowhere"))
        else {
            panic!("the submission should have been refused");
        };
        let rows = rows(&model, Some(&refusal));
        assert_eq!(rows[1]["drawer"]["open"], true);
        assert_eq!(rows[1]["drawer"]["children"][0]["fields"][2]["value"], "nowhere");
        assert!(rows[0]["drawer"].get("open").is_none());
    }

    #[test]
    fn a_submission_naming_no_entry_writes_nothing() {
        for body in [
            "_form=server&_index=9&server=1.1.1.1",
            "_form=server&_index=&server=1.1.1.1",
            "_form=server&_index=last&server=1.1.1.1",
        ] {
            let (_, saved) = submit(body);
            assert!(matches!(saved, Saved::Unknown), "{body}");
        }
    }
}
