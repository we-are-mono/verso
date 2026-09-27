// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

//! The extra names this router answers, as one flat table — and a page apiece for
//! editing one.
//!
//! dnsmasq spells four kinds of extra name in four section types, and an A record
//! and an AAAA record are the same type told apart by the address in it. An
//! operator does not think in section types — they think "the name backup.lan
//! should resolve to that box" — so the table is one listing of typed rows and
//! each row opens the record's own page. A record is a type, a name, an answer
//! and a couple of qualifiers, all seen at once — a page, not a panel beside the
//! listing.
//!
//! The type is the page's first control. Changing it moves the record between
//! section types, which is a section removed and a section created; both are
//! staged together, so the record is never briefly gone.

use verso_plugin::{
    commit, commit_delete, commit_new, json, Envelope, Form, Map, TableRow, Tone, Value, Widget,
};

use crate::dns;
use crate::form::{self, Errors};
use crate::format;
use crate::model::{Dnsdhcp, Record, RecordKind, CONFIG};
use crate::page;

const SUB: &str = "Extra names answered locally — `config domain`, `config cname`, \
`config srvhost`, `config mxhost`. Reserved devices already resolve; these are the names on top.";

const EMPTY: &str = "No extra names yet — reserved devices already answer by name.";

const NEW_SUB: &str = "Add a name this router answers, and what it answers with.";

const MISSING: &str = "That record isn’t here any more, so here is the DNS page instead.";

/// section renders the records listing that leads the DNS page, plus the block of
/// options that govern how local names are answered. Each row opens the record's
/// own page; the tail adds a new one.
pub fn section(model: &Dnsdhcp, settings: Widget) -> Widget {
    let rows = model.records.iter().map(row).collect();
    Widget::section(
        "Names on your network",
        SUB,
        vec![
            page::listing(
                page::columns(&[
                    ("Type", "keyword"),
                    ("Name", "mono"),
                    ("Points to", "mono"),
                    ("", "link"),
                ]),
                rows,
                EMPTY,
                "New record",
                &page::new_record_href(),
            ),
            settings,
        ],
    )
}

fn row(record: &Record) -> TableRow {
    TableRow {
        id: record.section.clone(),
        cells: vec![
            page::text_cell(record.kind.label()),
            page::mono_cell(&record.name),
            page::mono_cell(&format::points_to(&record.target, &qualifier(record))),
            page::edit_link_cell(page::record_href(&record.section)),
        ],
        ..TableRow::default()
    }
}

/// qualifier is the number that ranks or completes a record's answer: an SRV's
/// port, an MX's preference. The kinds without one answer with a name alone.
fn qualifier(record: &Record) -> String {
    match record.kind {
        RecordKind::Srv => record.port.clone(),
        RecordKind::Mx => record.priority.clone(),
        _ => String::new(),
    }
}

/// blank answers a visit to the new-record page: an A record with nothing filled
/// in, the everyday kind an operator reaches for first.
pub fn blank() -> Envelope {
    editor(None, &Record::blank(), &Errors::default())
}

/// edit answers a visit to one record's page, or nothing when the sub-path names
/// no record this config holds.
pub fn edit(model: &Dnsdhcp, section: &str) -> Option<Envelope> {
    let index = model.record_index(section)?;
    Some(editor(
        Some(section),
        &model.records[index],
        &Errors::default(),
    ))
}

/// missing states that the sub-path names no record — a stale link, or a record
/// someone else removed — and answers with the DNS page, which is somewhere real.
pub fn missing(model: &Dnsdhcp) -> Envelope {
    dns::page(model).with_notice(Tone::Danger, MISSING)
}

/// create answers the new-record page's submission. A record dnsmasq would not
/// read is refused with the offending controls marked; one it would is stated as
/// the section to add, and the listing answers with it staged.
pub fn create(model: &mut Dnsdhcp, form: &Form) -> Envelope {
    let Some(kind) = RecordKind::of(&form.get("kind")) else {
        // The type is a closed choice, so a value outside it is not something
        // this page offered — the blank page, so the operator can start again.
        return blank().with_notice(Tone::Danger, form::UNKNOWN);
    };
    let stated = submitted("", kind, form);
    let errors = validate(&stated);
    if !errors.is_empty() {
        return editor(None, &stated, &errors).with_notice(Tone::Danger, form::REFUSED);
    }
    let op = commit_new(CONFIG, kind.section_type(), values(&stated, false));
    model.records.push(stated);
    dns::page(model)
        .with_notice(Tone::Success, "Record added.")
        .with_commit(vec![op])
}

/// save answers one record's page. A body carrying the delete marker removes the
/// record and answers with the DNS page; anything else is the page's own
/// submission — refused onto the page with the offending controls marked, or
/// saved and answered with the listing.
pub fn save(model: &mut Dnsdhcp, section: &str, form: &Form) -> Option<Envelope> {
    let index = model.record_index(section)?;
    if form::deletes(form) {
        let removed = model.records.remove(index);
        return Some(
            dns::page(model)
                .with_notice(Tone::Success, "Record deleted.")
                .with_commit(vec![commit_delete(CONFIG, &removed.section)]),
        );
    }

    let Some(kind) = RecordKind::of(&form.get("kind")) else {
        return Some(missing(model));
    };
    let stated = submitted(section, kind, form);
    let errors = validate(&stated);
    if !errors.is_empty() {
        return Some(
            editor(Some(section), &stated, &errors).with_notice(Tone::Danger, form::REFUSED),
        );
    }

    let was = model.records[index].kind.section_type();
    let ops = match kind.section_type() == was {
        true => vec![commit(CONFIG, section, values(&stated, true))],
        // A record that changed type is the same name in a different section
        // type, which uci can only express as one section gone and one added.
        false => vec![
            commit_delete(CONFIG, section),
            commit_new(CONFIG, kind.section_type(), values(&stated, false)),
        ],
    };
    model.records[index] = stated;
    Some(
        dns::page(model)
            .with_notice(Tone::Success, "Record saved.")
            .with_commit(ops),
    )
}

/// editor composes the record page. `section` is the record being edited, or None
/// for a new one — which is also what decides whether the delete form is there at
/// all. The new page and the edit page are one screen.
fn editor(section: Option<&str>, record: &Record, errors: &Errors) -> Envelope {
    let (title, subheading) = match section {
        Some(_) => ("Edit record", heading(&record.name)),
        None => ("New record", NEW_SUB.to_string()),
    };
    let mut children = vec![Widget::Form {
        style: "page".into(),
        submit: String::new(),
        error: String::new(),
        fields: vec![
            form::select_field(
                "kind",
                "Type",
                record.kind.label(),
                form::choices(&[
                    ("A", "A — a name for an IPv4 address"),
                    ("AAAA", "AAAA — a name for an IPv6 address"),
                    ("CNAME", "CNAME — another name for a name"),
                    ("SRV", "SRV — where a service lives"),
                    ("MX", "MX — where mail for a domain goes"),
                ]),
                errors,
            ),
            form::text_field("name", "Name", &record.name, "", errors),
            form::text_field(
                "target",
                "Points to",
                &record.target,
                "An address for A and AAAA; a name for CNAME, SRV and MX.",
                errors,
            ),
            Widget::disclosure(
                "Advanced — 3 more options",
                vec![page::form_grid(
                    3,
                    vec![
                        form::text_field("port", "Port", &record.port, "SRV only.", errors),
                        form::text_field(
                            "priority",
                            "Priority",
                            &record.priority,
                            "SRV and MX. Lower wins.",
                            errors,
                        ),
                        form::text_field("weight", "Weight", &record.weight, "SRV only.", errors),
                    ],
                )
                .labelled("Port and ranking", "")],
            ),
        ],
        note: String::new(),
        target: String::new(),
    }];
    if section.is_some() {
        children.push(record_delete_form(record));
    }
    page::dns_editor(title, &subheading, Widget::stack(children))
}

/// heading names the record the page is about, falling back to its own words when
/// the record carries no name yet.
fn heading(name: &str) -> String {
    match name.is_empty() {
        true => "An unnamed record.".to_string(),
        false => name.to_string(),
    }
}

/// record_delete_form is the record page's one irreversible action.
fn record_delete_form(record: &Record) -> Widget {
    form::delete_form(
        "Delete record",
        &format!(
            "Delete the record for {}? The name stops resolving on this network.",
            subject(&record.name)
        ),
    )
}

fn subject(name: &str) -> String {
    match name.is_empty() {
        true => "this record".to_string(),
        false => format!("“{name}”"),
    }
}

/// submitted reads the page's values.
fn submitted(section: &str, kind: RecordKind, form: &Form) -> Record {
    let field = |name: &str| form.get(name).trim().to_string();
    Record {
        section: section.to_string(),
        kind,
        name: field("name"),
        target: field("target"),
        port: field("port"),
        priority: field("priority"),
        weight: field("weight"),
    }
}

/// validate answers dnsmasq's question — would it read this record — before the
/// write, because a record it cannot parse is skipped with nothing said.
fn validate(record: &Record) -> Errors {
    let mut errors = Errors::default();
    errors.check(
        "name",
        form::valid_hostname(&record.name),
        "Write the name being answered, such as nas.lan.",
    );
    let target_ok = match record.kind {
        RecordKind::A => form::valid_ipv4(&record.target),
        RecordKind::Aaaa => form::valid_ipv6(&record.target),
        _ => form::valid_hostname(&record.target),
    };
    errors.check("target", target_ok, target_help(record.kind));
    if record.kind == RecordKind::Srv {
        errors.check(
            "port",
            form::valid_number(&record.port, 65535),
            "An SRV record needs the port the service listens on.",
        );
    }
    for (field, value) in [("priority", &record.priority), ("weight", &record.weight)] {
        errors.check(
            field,
            value.is_empty() || form::valid_number(value, 65535),
            "Write a whole number from 0 to 65535, or leave it blank.",
        );
    }
    errors
}

fn target_help(kind: RecordKind) -> &'static str {
    match kind {
        RecordKind::A => "Write the IPv4 address this name answers with.",
        RecordKind::Aaaa => "Write the IPv6 address this name answers with.",
        _ => "Write the name this record points at, such as nas.lan.",
    }
}

/// values is what the save writes. Editing an existing section states every
/// option that section type carries, so a value the operator cleared is cleared
/// on disk rather than left behind; a new section has nothing to clear.
fn values(record: &Record, existing: bool) -> Value {
    let mut values = Map::new();
    let mut set = |option: &str, value: &str| {
        if !value.is_empty() {
            values.insert(option.to_string(), json!(value));
        }
    };
    set(record.kind.name_option(), &record.name);
    set(record.kind.target_option(), &record.target);
    if record.kind == RecordKind::Srv {
        set("port", &record.port);
        set("weight", &record.weight);
    }
    if let Some(option) = record.kind.priority_option() {
        set(option, &record.priority);
    }
    if existing {
        for option in owned(record.kind) {
            values.entry(option.to_string()).or_insert(Value::Null);
        }
    }
    Value::Object(values)
}

/// owned is every option this editor writes on a section of one type — the
/// contract that says which options a save may clear and which it must leave
/// exactly where it found them.
fn owned(kind: RecordKind) -> &'static [&'static str] {
    match kind {
        RecordKind::A | RecordKind::Aaaa => &["name", "ip"],
        RecordKind::Cname => &["cname", "target"],
        RecordKind::Srv => &["srv", "target", "port", "class", "weight"],
        RecordKind::Mx => &["domain", "relay", "pref"],
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::fixture;
    use serde_json::Value as Json;

    fn body(env: Envelope) -> Json {
        serde_json::to_value(&env).expect("serialize")
    }

    fn listing_rows() -> Vec<Json> {
        let model = fixture::dnsdhcp();
        let json = serde_json::to_value(section(&model, Widget::text(""))).expect("serialize");
        json["children"][0]["rows"]
            .as_array()
            .expect("rows")
            .clone()
    }

    fn edited(section: &str) -> Json {
        let model = fixture::dnsdhcp();
        body(edit(&model, section).expect("the fixture holds this record"))
    }

    fn fields(env: &Json) -> Json {
        env["widget"]["children"][0]["fields"].clone()
    }

    fn control(env: &Json, name: &str) -> Json {
        fields(env)
            .as_array()
            .expect("fields")
            .iter()
            .flat_map(|f| {
                // The advanced grid nests its controls one level down.
                match f["type"] == "disclosure" {
                    true => f["children"][0]["children"]
                        .as_array()
                        .cloned()
                        .unwrap_or_default(),
                    false => vec![f.clone()],
                }
            })
            .find(|f| f["name"] == name)
            .unwrap_or_else(|| panic!("no control {name}"))
    }

    fn saved(section: &str, body_str: &str) -> (Dnsdhcp, Json) {
        let mut model = fixture::dnsdhcp();
        let env = save(&mut model, section, &Form::parse(body_str))
            .expect("the fixture holds this record");
        (model, body(env))
    }

    fn created(body_str: &str) -> (Dnsdhcp, Json) {
        let mut model = fixture::dnsdhcp();
        let env = create(&mut model, &Form::parse(body_str));
        (model, body(env))
    }

    #[test]
    fn every_row_leads_to_its_own_page_and_the_tail_leads_to_a_new_one() {
        let rows = listing_rows();
        let typed: Vec<(&str, &str, &str)> = rows
            .iter()
            .map(|row| {
                (
                    row["cells"][0]["text"].as_str().unwrap_or(""),
                    row["cells"][1]["text"].as_str().unwrap_or(""),
                    row["cells"][2]["text"].as_str().unwrap_or(""),
                )
            })
            .collect();
        assert_eq!(
            typed,
            vec![
                ("A", "backup.lan", "10.0.0.31"),
                ("AAAA", "backup.lan", "2a00:ee2:2d00:2e00::31"),
                ("CNAME", "photos.lan", "nas.lan"),
                ("CNAME", "cloud.lan", "nas.lan"),
                ("SRV", "_matrix._tcp.lan", "nas.lan · 8448"),
                ("MX", "lan", "nas.lan · 10"),
            ]
        );
        // No row opens a panel; each leads to a page, and none carries a drawer.
        for row in &rows {
            assert!(row.get("drawer").is_none(), "{row}");
            let last = row["cells"]
                .as_array()
                .expect("cells")
                .last()
                .expect("cell");
            let section = row["id"].as_str().expect("id");
            assert_eq!(
                last,
                &serde_json::json!({"text": "Edit", "href": page::record_href(section)})
            );
        }
    }

    #[test]
    fn the_edit_page_carries_the_record_and_the_new_page_starts_empty() {
        let srv = edited("srv_matrix");
        assert_eq!(srv["title"], "Edit record");
        assert_eq!(srv["subheading"], "_matrix._tcp.lan");
        assert!(srv.get("pages").is_none(), "DNS declares no top bar");
        assert_eq!(control(&srv, "kind")["value"], "SRV");
        assert_eq!(control(&srv, "kind")["kind"], "select");
        assert_eq!(control(&srv, "target")["value"], "nas.lan");
        assert_eq!(control(&srv, "port")["value"], "8448");
        // The page form leaves its Save label to the shell, so it declares none.
        let form = &srv["widget"]["children"][0];
        assert_eq!(form["style"], "page");
        assert!(form.get("submit").is_none());
        // An existing record can be deleted; a new one cannot.
        let delete = &srv["widget"]["children"][1];
        assert_eq!(delete["fields"][0]["name"], "_delete");
        assert_eq!(delete["fields"][1]["type"], "confirm");

        let blank = body(blank());
        assert_eq!(blank["title"], "New record");
        assert_eq!(blank["subheading"], NEW_SUB);
        assert_eq!(control(&blank, "kind")["value"], "A");
        assert_eq!(control(&blank, "name")["value"], "");
        assert_eq!(
            blank["widget"]["children"]
                .as_array()
                .expect("children")
                .len(),
            1,
            "a record that does not exist yet cannot be deleted"
        );
    }

    #[test]
    fn creating_a_record_states_the_section_to_add_and_lands_on_the_listing() {
        let (model, body) = created("kind=CNAME&name=photos2.lan&target=nas.lan");
        assert_eq!(body["title"], "DNS");
        assert_eq!(body["notice"]["level"], "success");
        assert_eq!(
            body["commit"],
            serde_json::json!([{
                "config": "dhcp", "section": "", "type": "cname",
                "values": {"cname": "photos2.lan", "target": "nas.lan"}
            }])
        );
        // The answer already carries the new record.
        assert!(model.records.iter().any(|r| r.name == "photos2.lan"));
    }

    #[test]
    fn saving_a_record_states_every_option_its_type_carries() {
        let (model, body) = saved(
            "srv_matrix",
            "kind=SRV&name=_matrix._tcp.lan&target=nas.lan&port=8449&priority=10&weight=",
        );
        assert_eq!(body["title"], "DNS");
        assert_eq!(
            body["commit"],
            serde_json::json!([{
                "config": "dhcp",
                "section": "srv_matrix",
                "values": {
                    "srv": "_matrix._tcp.lan", "target": "nas.lan", "port": "8449",
                    "class": "10",
                    // The weight the operator cleared is cleared on disk.
                    "weight": null
                }
            }])
        );
        let index = model.record_index("srv_matrix").expect("record");
        assert_eq!(model.records[index].port, "8449");
    }

    #[test]
    fn changing_a_records_type_moves_it_between_section_types() {
        let (_, body) = saved("cname_photos", "kind=A&name=photos.lan&target=10.0.0.30");
        assert_eq!(
            body["commit"],
            serde_json::json!([
                {"config": "dhcp", "section": "cname_photos", "delete": true},
                {"config": "dhcp", "section": "", "type": "domain",
                 "values": {"name": "photos.lan", "ip": "10.0.0.30"}}
            ])
        );

        // A and AAAA are one section type, so switching between them is a set.
        let (_, body) = saved(
            "domain_backup_v4",
            "kind=AAAA&name=backup.lan&target=2a00:ee2::31",
        );
        assert_eq!(
            body["commit"],
            serde_json::json!([{
                "config": "dhcp", "section": "domain_backup_v4",
                "values": {"name": "backup.lan", "ip": "2a00:ee2::31"}
            }])
        );
    }

    #[test]
    fn deleting_a_record_removes_its_section_and_lands_on_the_listing() {
        let (model, body) = saved("mx_lan", "_delete=1");
        assert_eq!(body["title"], "DNS");
        assert_eq!(body["notice"]["text"], "Record deleted.");
        assert_eq!(
            body["commit"],
            serde_json::json!([{"config": "dhcp", "section": "mx_lan", "delete": true}])
        );
        assert!(model.record_index("mx_lan").is_none());
    }

    #[test]
    fn a_value_dnsmasq_would_not_read_is_marked_on_the_page_and_nothing_is_written() {
        for (section, body_str, field) in [
            (
                "domain_backup_v4",
                "kind=A&name=backup.lan&target=nas.lan",
                "target",
            ),
            (
                "domain_backup_v6",
                "kind=AAAA&name=backup.lan&target=10.0.0.31",
                "target",
            ),
            ("domain_backup_v4", "kind=A&name=&target=10.0.0.31", "name"),
            (
                "srv_matrix",
                "kind=SRV&name=_matrix._tcp.lan&target=nas.lan&port=",
                "port",
            ),
            (
                "srv_matrix",
                "kind=SRV&name=_matrix._tcp.lan&target=nas.lan&port=8448&priority=high",
                "priority",
            ),
        ] {
            let (_, body) = saved(section, body_str);
            assert_eq!(body["title"], "Edit record", "{body_str}");
            assert!(
                body.get("commit").is_none(),
                "{body_str}: nothing may be written"
            );
            assert_eq!(body["notice"]["level"], "danger", "{body_str}");
            assert!(
                !control(&body, field)["error"]
                    .as_str()
                    .unwrap_or("")
                    .is_empty(),
                "{body_str}: {field} carries no error"
            );
            // The submitted value comes back on its control.
            assert!(control(&body, field).get("value").is_some(), "{body_str}");
        }
    }

    #[test]
    fn a_refused_new_record_comes_back_on_the_new_page_carrying_what_was_typed() {
        let (_, body) = created("kind=A&name=backup.lan&target=nowhere");
        assert_eq!(body["title"], "New record");
        assert!(body.get("commit").is_none());
        assert_eq!(control(&body, "target")["value"], "nowhere");
        assert!(!control(&body, "target")["error"]
            .as_str()
            .unwrap_or("")
            .is_empty());
    }

    #[test]
    fn a_sub_path_naming_no_record_answers_with_the_dns_page() {
        let model = fixture::dnsdhcp();
        assert!(edit(&model, "no_such_record").is_none());

        let mut model = fixture::dnsdhcp();
        assert!(save(
            &mut model,
            "no_such_record",
            &Form::parse("kind=A&name=a.lan&target=10.0.0.1")
        )
        .is_none());

        let body = body(missing(&fixture::dnsdhcp()));
        assert_eq!(body["title"], "DNS");
        assert_eq!(body["notice"]["level"], "danger");
    }
}
