// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

//! The DNS page: how names are answered here.
//!
//! One face and no top bar. DNS has no live state to watch — nothing here
//! changes unless someone changes it — so a second face would be a tab that is
//! not about anything. The page is ordered by what an operator came to do: the
//! names they added, where the rest of the questions go, what is filtered, and
//! then the daemon's own depth, grouped by what each option governs and folded
//! behind a seam per group.
//!
//! Everything editable sits in one page form the staged-changes bar submits, so
//! a switch flipped in Protection and a number typed in Advanced are saved and
//! applied together. The drawers are their own forms: a record and an upstream
//! are objects, and each is saved on its own.

use verso_plugin::{Envelope, Tone, Widget};

use crate::form;
use crate::model::{Dnsdhcp, CONFIG, DAEMON_TYPE};
use crate::options::{self, on, on_by_default, paired, read, value, Changes, Check, Row};
use crate::page;
use crate::records;
use crate::upstreams;

/// NAMES governs how the names on this network are answered.
const NAMES: [Row; 2] = [
    paired(
        "domain",
        "domain · local",
        "Local domain",
        "Bare hostnames become names under this domain, and questions for it never go upstream.",
        Check::Hostname,
    ),
    on(
        "expandhosts",
        "Expand hosts",
        "Answer plain names with the local domain appended.",
    ),
];

const NAMES_FOLDED: [Row; 1] = [on(
    "nohosts",
    "Skip /etc/hosts",
    "Ignore the system hosts file when answering.",
)];

/// LOOKUPS governs how the upstream servers are asked.
const LOOKUPS: [Row; 1] = [on(
    "noresolv",
    "Use only these servers",
    "Ignore whatever the internet connection suggested.",
)];

const LOOKUPS_FOLDED: [Row; 3] = [
    on(
        "allservers",
        "Race all servers",
        "Ask every upstream at once; first answer wins.",
    ),
    on(
        "strictorder",
        "Ask in order",
        "Query upstreams strictly in the order listed.",
    ),
    read(
        "serversfile",
        "Additional servers file",
        "A file of extra upstream entries.",
    ),
];

/// PROTECTION is what never leaves the house, and what never gets in.
const PROTECTION: [Row; 4] = [
    on_by_default(
        "rebind_protection",
        "Rebind protection",
        "Refuse upstream answers that point at private addresses.",
    ),
    on(
        "localservice",
        "Local service only",
        "Answer DNS only for directly attached networks.",
    ),
    on_by_default(
        "boguspriv",
        "Keep private lookups private",
        "Never forward reverse lookups for private ranges upstream.",
    ),
    on(
        "domainneeded",
        "Require a domain",
        "Never forward bare hostnames upstream.",
    ),
];

const PROTECTION_FOLDED: [Row; 5] = [
    read(
        "address",
        "Block or redirect a domain",
        "Answer a whole domain with one address — or nothing, e.g. /ads.example.com/.",
    ),
    read(
        "rebind_domain",
        "Rebind exceptions",
        "Domains allowed to answer with private addresses.",
    ),
    on(
        "filter_aaaa",
        "Filter IPv6 answers",
        "Strip AAAA records from replies.",
    ),
    on(
        "filter_a",
        "Filter IPv4 answers",
        "Strip A records from replies.",
    ),
    on(
        "filterwin2k",
        "Filter Windows noise",
        "Drop the requests Windows sends constantly.",
    ),
];

/// The Advanced groups: the daemon's own depth, by what each option governs.
const CACHE: [Row; 3] = [
    value(
        "cachesize",
        "Cache size",
        "Answers kept in memory.",
        Check::Number(u32::MAX),
    ),
    on(
        "nonegcache",
        "Don’t cache misses",
        "Skip remembering negative answers.",
    ),
    value(
        "dnsforwardmax",
        "Max concurrent queries",
        "Upstream questions in flight at once.",
        Check::Number(u32::MAX),
    ),
];

const CACHE_FOLDED: [Row; 3] = [
    value(
        "min_cache_ttl",
        "Minimum TTL",
        "Cache short-lived answers at least this long.",
        Check::Number(u32::MAX),
    ),
    value(
        "max_cache_ttl",
        "Maximum TTL",
        "Cap how long any answer is kept.",
        Check::Number(u32::MAX),
    ),
    value(
        "ednspacket_max",
        "EDNS packet max",
        "Largest UDP answer accepted.",
        Check::Number(u32::MAX),
    ),
];

const DNSSEC: [Row; 2] = [
    on(
        "dnssec",
        "Validate signatures",
        "Check DNSSEC on upstream answers.",
    ),
    on_by_default(
        "dnsseccheckunsigned",
        "Distrust unsigned",
        "Verify that unsigned answers are legitimately unsigned.",
    ),
];

const LISTENING: [Row; 2] = [
    value(
        "port",
        "DNS port",
        "Where queries are answered.",
        Check::Number(65535),
    ),
    on(
        "logqueries",
        "Log queries",
        "Write every DNS question to the system log.",
    ),
];

const LISTENING_FOLDED: [Row; 3] = [
    read(
        "interface",
        "Listen only on",
        "Restrict answering to these interfaces.",
    ),
    read(
        "notinterface",
        "Never listen on",
        "Interfaces excluded from answering.",
    ),
    read(
        "addnhosts",
        "Extra hosts files",
        "Additional files answered like /etc/hosts.",
    ),
];

const ADVANCED_SUB: &str = "The daemon itself — `config dnsmasq`, grouped by what each option \
governs. The everyday ones are visible; the long tail folds below each card.";

/// Open is what a refused submission puts back in front of the operator: the
/// drawer it came from, carrying the values that were typed and the errors they
/// earned, or — for the page form, whose rows have nowhere to carry a message —
/// the refusal on the form itself.
#[derive(Default)]
pub struct Open {
    record: Option<records::Refusal>,
    upstream: Option<upstreams::Refusal>,
    refused: String,
}

/// page renders the DNS page.
pub fn page(model: &Dnsdhcp) -> Envelope {
    render(model, &Open::default())
}

fn render(model: &Dnsdhcp, open: &Open) -> Envelope {
    page::dns(Widget::stack(vec![
        page::filter("Filter everything — name, server, option…"),
        page::page_form(&open.refused, vec![
            records::section(
                model,
                open.record.as_ref(),
                block(model, "", &NAMES, &NAMES_FOLDED, ""),
            ),
            upstreams::section(
                model,
                open.upstream.as_ref(),
                block(model, "", &LOOKUPS, &LOOKUPS_FOLDED, ""),
            ),
            Widget::section(
                "Protection",
                "What never leaves the house, and what never gets in.",
                vec![block(model, "", &PROTECTION, &PROTECTION_FOLDED, "")],
            ),
            Widget::section(
                "Advanced",
                ADVANCED_SUB,
                vec![
                    block(model, "Cache & limits", &CACHE, &CACHE_FOLDED, ""),
                    block(model, "DNSSEC", &DNSSEC, &[], ""),
                    block(model, "Listening & logs", &LISTENING, &LISTENING_FOLDED, ""),
                ],
            ),
        ]),
    ]))
}

/// block renders one catalogue against the daemon section.
fn block(model: &Dnsdhcp, title: &str, rows: &[Row], folded: &[Row], lead: &str) -> Widget {
    Widget::Settings {
        style: String::new(),
        title: title.into(),
        meta: String::new(),
        items: options::items(rows, &model.daemon, ""),
        seam: options::fold(lead, options::items(folded, &model.daemon, "")),
    }
}

/// post answers a submission to this page: a drawer's, or the page form's.
pub fn post(model: &mut Dnsdhcp, form: &verso_plugin::Form) -> Envelope {
    match form::kind(form).as_deref() {
        Some(form::RECORD) => record(model, form),
        Some(form::SERVER) => upstream(model, form),
        Some(_) => render(model, &Open::default()).with_notice(Tone::Danger, form::UNKNOWN),
        None => save(model, form),
    }
}

fn record(model: &mut Dnsdhcp, form: &verso_plugin::Form) -> Envelope {
    match records::save(model, form) {
        records::Saved::Ops(ops, said) => render(model, &Open::default())
            .with_notice(Tone::Success, said)
            .with_commit(ops),
        records::Saved::Refused(refusal) => render(
            model,
            &Open {
                record: Some(refusal),
                ..Open::default()
            },
        )
        .with_notice(Tone::Danger, form::REFUSED),
        records::Saved::Unknown => {
            render(model, &Open::default()).with_notice(Tone::Danger, form::UNKNOWN)
        }
    }
}

fn upstream(model: &mut Dnsdhcp, form: &verso_plugin::Form) -> Envelope {
    match upstreams::save(model, form) {
        upstreams::Saved::Ops(ops, said) => render(model, &Open::default())
            .with_notice(Tone::Success, said)
            .with_commit(ops),
        upstreams::Saved::Refused(refusal) => render(
            model,
            &Open {
                upstream: Some(refusal),
                ..Open::default()
            },
        )
        .with_notice(Tone::Danger, form::REFUSED),
        upstreams::Saved::Unknown => {
            render(model, &Open::default()).with_notice(Tone::Danger, form::UNKNOWN)
        }
    }
}

/// save answers the page form: every catalogue this page drew, read back against
/// what the config states.
fn save(model: &mut Dnsdhcp, form: &verso_plugin::Form) -> Envelope {
    let was = model.daemon.scalar("domain").to_string();
    let mut changes = Changes::default();
    for rows in [
        &NAMES[..],
        &NAMES_FOLDED[..],
        &LOOKUPS[..],
        &LOOKUPS_FOLDED[..],
        &PROTECTION[..],
        &PROTECTION_FOLDED[..],
        &CACHE[..],
        &CACHE_FOLDED[..],
        &DNSSEC[..],
        &LISTENING[..],
        &LISTENING_FOLDED[..],
    ] {
        options::save(
            rows,
            &mut model.daemon,
            DAEMON_TYPE,
            "",
            form,
            &mut changes,
        );
    }
    pair_local_domain(model, &was, &mut changes);

    if let Some(refusal) = changes.refusal() {
        let refused = format!("{refusal}, so nothing was saved.");
        return render(
            model,
            &Open {
                refused: refused.clone(),
                ..Open::default()
            },
        )
        .with_notice(Tone::Danger, &refused);
    }
    let answer = render(model, &Open::default());
    if changes.is_empty() {
        return answer;
    }
    answer
        .with_notice(Tone::Success, "DNS settings saved.")
        .with_commit(changes.commit(CONFIG))
}

/// pair_local_domain moves `local` with `domain`. dnsmasq answers a domain
/// locally only when `local` names it, so a `domain` changed without it would
/// leave this network resolving under a name nothing serves.
fn pair_local_domain(model: &mut Dnsdhcp, was: &str, changes: &mut Changes) {
    let domain = model.daemon.scalar("domain").to_string();
    if domain == was {
        return;
    }
    let section = model.daemon.section.clone();
    match domain.is_empty() {
        true => {
            model.daemon.set("local", None);
            changes.write(&section, DAEMON_TYPE, "local", verso_plugin::Value::Null);
        }
        false => {
            let local = format!("/{domain}/");
            model.daemon.set("local", Some(&local));
            changes.write(
                &section,
                DAEMON_TYPE,
                "local",
                verso_plugin::Value::from(local),
            );
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::fixture;
    use serde_json::Value as Json;
    use verso_plugin::Form;

    fn body() -> Json {
        serde_json::to_value(page(&fixture::dnsdhcp())).expect("serialize")
    }

    fn sections(body: &Json) -> Vec<Json> {
        body["widget"]["children"][1]["fields"]
            .as_array()
            .expect("sections")
            .clone()
    }

    fn answer(body: &str) -> Json {
        let mut model = fixture::dnsdhcp();
        serde_json::to_value(post(&mut model, &Form::parse(body))).expect("serialize")
    }

    #[test]
    fn the_page_is_one_face_with_one_form_the_capsule_saves() {
        let body = body();
        assert_eq!(body["title"], "DNS");
        assert_eq!(body["width"], "wide");
        assert!(
            body.get("pages").is_none(),
            "DNS has no live state to watch, so it declares no top bar"
        );
        assert_eq!(body["widget"]["children"][0]["type"], "filter");

        let form = &body["widget"]["children"][1];
        assert_eq!(form["type"], "form");
        assert_eq!(form["style"], "page");
        assert!(
            form.get("submit").is_none(),
            "the staged-changes bar owns saving, so the form declares no button"
        );
        let sections = sections(&body);
        let titles: Vec<&str> = sections
            .iter()
            .map(|section| section["title"].as_str().unwrap_or(""))
            .collect();
        assert_eq!(
            titles,
            vec![
                "Names on your network",
                "Where lookups go",
                "Protection",
                "Advanced"
            ]
        );
    }

    #[test]
    fn every_block_reads_the_real_config_and_folds_its_long_tail() {
        let body = body();
        let names = &sections(&body)[0]["children"][1];
        assert_eq!(names["items"][0]["value"], "lan");
        assert_eq!(names["items"][0]["name"], "domain");
        assert_eq!(
            names["items"][0]["code"], "domain · local",
            "the chip names every option the row moves"
        );
        assert_eq!(names["items"][1]["toggle"], serde_json::json!({"name": "expandhosts", "on": true}));
        // The card states its long tail as it is; whether a tail this short is
        // worth a fold is the shell's call, and one row is not — it renders on
        // the card beside the rest.
        assert_eq!(names["seam"]["summary"], "1 more option");
        assert_eq!(names["seam"]["items"][0]["toggle"], serde_json::json!({"name": "nohosts"}));

        let protection = &sections(&body)[2]["children"][0];
        // An option the config never states still reads as what the daemon does.
        assert_eq!(
            protection["items"][0]["toggle"],
            serde_json::json!({"name": "rebind_protection", "on": true})
        );
        assert_eq!(
            protection["items"][1]["toggle"],
            serde_json::json!({"name": "localservice", "on": true})
        );
        assert_eq!(protection["seam"]["summary"], "5 more options");
        // An info row states the option and offers nothing to change.
        let exceptions = &protection["seam"]["items"][1];
        assert_eq!(exceptions["code"], "rebind_domain");
        assert_eq!(exceptions["value"], "plex.direct");
        assert!(exceptions.get("name").is_none());

        let advanced = &sections(&body)[3]["children"];
        assert_eq!(advanced[0]["title"], "Cache & limits");
        assert_eq!(advanced[0]["items"][0]["value"], "1000");
        assert_eq!(advanced[1]["title"], "DNSSEC");
        assert!(advanced[1].get("seam").is_none());
        assert_eq!(
            advanced[1]["items"][1]["toggle"],
            serde_json::json!({"name": "dnsseccheckunsigned", "on": true})
        );
        assert_eq!(advanced[2]["title"], "Listening & logs");
        assert_eq!(advanced[2]["seam"]["items"][1]["value"], "loopback");
    }

    #[test]
    fn the_page_form_writes_only_what_changed() {
        // The page as it stands, resubmitted, changes nothing.
        let unchanged = answer(
            "domain=lan&expandhosts=1&noresolv=1&rebind_protection=1&localservice=1&boguspriv=1&\
             domainneeded=1&dnsseccheckunsigned=1&cachesize=1000&dnsforwardmax=150&port=53",
        );
        assert!(unchanged.get("commit").is_none());
        assert!(unchanged.get("notice").is_none());

        let saved = answer(
            "domain=lan&expandhosts=1&noresolv=1&rebind_protection=1&boguspriv=1&domainneeded=1&\
             dnssec=1&dnsseccheckunsigned=1&cachesize=2000&dnsforwardmax=150&port=53",
        );
        assert_eq!(
            saved["commit"],
            serde_json::json!([{
                "config": "dhcp",
                "section": "dnsmasq_main",
                "values": {
                    // A switch that stopped posting is off.
                    "localservice": "0",
                    "dnssec": "1",
                    "cachesize": "2000"
                }
            }])
        );
        assert_eq!(saved["notice"]["level"], "success");
    }

    #[test]
    fn an_emptied_value_is_cleared_and_the_local_domain_moves_with_it() {
        let cleared = answer("domain=&expandhosts=1&noresolv=1&rebind_protection=1&localservice=1&boguspriv=1&domainneeded=1&dnsseccheckunsigned=1&cachesize=1000&dnsforwardmax=150&port=53");
        assert_eq!(
            cleared["commit"][0]["values"],
            serde_json::json!({"domain": null, "local": null})
        );

        let renamed = answer("domain=home&expandhosts=1&noresolv=1&rebind_protection=1&localservice=1&boguspriv=1&domainneeded=1&dnsseccheckunsigned=1&cachesize=1000&dnsforwardmax=150&port=53");
        assert_eq!(
            renamed["commit"][0]["values"],
            serde_json::json!({"domain": "home", "local": "/home/"})
        );
    }

    #[test]
    fn a_value_the_daemon_would_not_read_saves_nothing_and_says_so() {
        let refused = answer("domain=lan&expandhosts=1&noresolv=1&rebind_protection=1&localservice=1&boguspriv=1&domainneeded=1&dnsseccheckunsigned=1&cachesize=lots&dnsforwardmax=150&port=53");
        assert!(refused.get("commit").is_none());
        let said = "“Cache size” has to be a whole number, so nothing was saved.";
        assert_eq!(
            refused["notice"],
            serde_json::json!({"level": "danger", "text": said})
        );
        // The refusal rides the form as well as the notice: that is what makes
        // the answer a 422, which keeps the capsule on the page.
        assert_eq!(refused["widget"]["children"][1]["error"], said);
        // What was typed comes back on the row it was typed on.
        let advanced = &sections(&refused)[3]["children"][0];
        assert_eq!(advanced["items"][0]["value"], "lots");
    }

    #[test]
    fn a_drawer_submission_is_answered_by_the_drawer_and_not_the_page_form() {
        let saved = answer("_form=record&_section=cname_photos&kind=CNAME&name=photos.lan&target=nas.lan");
        assert_eq!(saved["notice"]["level"], "success");
        assert_eq!(saved["commit"][0]["section"], "cname_photos");

        let refused = answer("_form=server&_index=0&server=nowhere");
        assert!(refused.get("commit").is_none());
        assert_eq!(refused["notice"]["level"], "danger");
        let upstreams = &sections(&refused)[1]["children"][0]["rows"];
        assert_eq!(upstreams[0]["drawer"]["open"], true);
    }

    #[test]
    fn a_body_this_page_did_not_draw_changes_nothing() {
        let unknown = answer("_form=peer&_section=wg0");
        assert!(unknown.get("commit").is_none());
        assert_eq!(unknown["notice"]["text"], form::UNKNOWN);
    }
}
