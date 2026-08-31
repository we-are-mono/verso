// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

//! The extra names this router answers, as one flat table.
//!
//! dnsmasq spells four kinds of extra name in four section types, and an A
//! record and an AAAA record are the same type told apart by the address in it.
//! An operator does not think in section types — they think "the name backup.lan
//! should resolve to that box" — so the table is one listing of typed rows and
//! the drawer's first control is the type itself. Changing it moves the record
//! between section types, which is a section removed and a section created; both
//! are staged together, so the record is never briefly gone.

use verso_plugin::{
    commit, commit_delete, commit_new, json, CommitOp, Form, Map, TableRow, Value, Widget,
};

use crate::form::{self, Errors};
use crate::format;
use crate::model::{Dnsdhcp, Record, RecordKind, CONFIG};
use crate::page;

const SUB: &str = "Extra names answered locally — `config domain`, `config cname`, \
`config srvhost`, `config mxhost`. Reserved devices already resolve; these are the names on top.";

/// Refusal is a record the operator stated and the daemon would not read: the
/// section it was about, the values as typed, and what is wrong with them.
pub struct Refusal {
    pub section: String,
    pub record: Record,
    pub errors: Errors,
}

/// Saved is what a record submission amounts to.
pub enum Saved {
    /// Written, with the sentence that states it.
    Ops(Vec<CommitOp>, &'static str),
    /// Nothing written, and the drawer to put back in front of the operator.
    Refused(Refusal),
    /// A submission naming no record this page drew.
    Unknown,
}

/// section renders the records listing and the block of options that govern how
/// local names are answered. A refusal reopens the drawer it came from, carrying
/// what was typed.
pub fn section(model: &Dnsdhcp, refusal: Option<&Refusal>, settings: Widget) -> Widget {
    let rows = model
        .records
        .iter()
        .map(|record| match refusal {
            Some(refused) if refused.section == record.section => {
                row(&refused.record, &refused.errors, true)
            }
            _ => row(record, &Errors::default(), false),
        })
        .collect();
    Widget::section(
        "Names on your network",
        SUB,
        vec![
            page::table(
                page::columns(&[("Type", "keyword"), ("Name", "mono"), ("Points to", "mono")]),
                rows,
            ),
            settings,
        ],
    )
}

fn row(record: &Record, errors: &Errors, open: bool) -> TableRow {
    TableRow {
        id: record.section.clone(),
        cells: vec![
            page::text_cell(record.kind.label()),
            page::mono_cell(&record.name),
            page::mono_cell(&format::points_to(&record.target, &qualifier(record))),
        ],
        drawer: drawer(record, errors, open),
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

fn drawer(record: &Record, errors: &Errors, open: bool) -> Option<verso_plugin::RowDrawer> {
    let title = match record.name.is_empty() {
        true => "Edit record".to_string(),
        false => format!("Edit record — {}", record.name),
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
                    Widget::hidden(form::KIND, form::RECORD),
                    Widget::hidden(form::SECTION, &record.section),
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
                                form::text_field(
                                    "weight",
                                    "Weight",
                                    &record.weight,
                                    "SRV only.",
                                    errors,
                                ),
                            ],
                        )],
                    ),
                ],
            },
            form::delete_form(
                form::RECORD,
                (form::SECTION, &record.section),
                "Delete record",
                &format!(
                    "Delete the record for {}? The name stops resolving on this network.",
                    subject(&record.name)
                ),
            ),
        ],
    )
}

fn subject(name: &str) -> String {
    match name.is_empty() {
        true => "this record".to_string(),
        false => format!("“{name}”"),
    }
}

/// save answers a record drawer's submission.
pub fn save(model: &mut Dnsdhcp, form: &Form) -> Saved {
    let section = form.get(form::SECTION);
    let Some(index) = model.record_index(&section) else {
        return Saved::Unknown;
    };
    if form::deletes(form) {
        let removed = model.records.remove(index);
        return Saved::Ops(vec![commit_delete(CONFIG, &removed.section)], "Record deleted.");
    }

    let Some(kind) = RecordKind::of(&form.get("kind")) else {
        // The type is a closed choice, so a value outside it is not something
        // this drawer offered.
        return Saved::Unknown;
    };
    let stated = submitted(&section, kind, form);
    let errors = validate(&stated);
    if !errors.is_empty() {
        return Saved::Refused(Refusal {
            section,
            record: stated,
            errors,
        });
    }

    let was = model.records[index].kind.section_type();
    let ops = match kind.section_type() == was {
        true => vec![commit(CONFIG, &section, values(&stated, true))],
        // A record that changed type is the same name in a different section
        // type, which uci can only express as one section gone and one added.
        false => vec![
            commit_delete(CONFIG, &section),
            commit_new(CONFIG, kind.section_type(), values(&stated, false)),
        ],
    };
    model.records[index] = stated;
    Saved::Ops(ops, "Record saved.")
}

/// submitted reads the drawer's values.
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

    fn rows(model: &Dnsdhcp, refusal: Option<&Refusal>) -> Vec<Json> {
        let json =
            serde_json::to_value(section(model, refusal, Widget::text(""))).expect("serialize");
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
    fn every_kind_reads_as_one_row_with_its_own_drawer() {
        let model = fixture::dnsdhcp();
        let rows = rows(&model, None);
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

        let drawer = &rows[4]["drawer"];
        assert_eq!(drawer["title"], "Edit record — _matrix._tcp.lan");
        assert!(drawer.get("open").is_none(), "a listing opens nothing");
        let fields = &drawer["children"][0]["fields"];
        assert_eq!(
            fields[0],
            serde_json::json!({"type": "field", "name": "_form", "kind": "hidden", "value": "record"})
        );
        assert_eq!(fields[2]["value"], "SRV");
        assert_eq!(fields[2]["kind"], "select");
        assert_eq!(fields[4]["value"], "nas.lan");
        // The delete lives in a form of its own, behind a confirm.
        assert_eq!(drawer["children"][1]["fields"][2]["name"], "_delete");
        assert_eq!(drawer["children"][1]["fields"][3]["type"], "confirm");
    }

    #[test]
    fn saving_a_record_states_every_option_its_type_carries() {
        let (model, saved) = submit("_form=record&_section=srv_matrix&kind=SRV&name=_matrix._tcp.lan&target=nas.lan&port=8449&priority=10&weight=");
        assert_eq!(
            ops(&saved),
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
        // The answer renders from the model, so it carries the accepted change.
        let index = model.record_index("srv_matrix").expect("record");
        assert_eq!(model.records[index].port, "8449");
    }

    #[test]
    fn changing_a_records_type_moves_it_between_section_types() {
        let (_, saved) = submit("_form=record&_section=cname_photos&kind=A&name=photos.lan&target=10.0.0.30");
        assert_eq!(
            ops(&saved),
            serde_json::json!([
                {"config": "dhcp", "section": "cname_photos", "delete": true},
                {"config": "dhcp", "section": "", "type": "domain",
                 "values": {"name": "photos.lan", "ip": "10.0.0.30"}}
            ])
        );

        // A and AAAA are one section type, so switching between them is a set.
        let (_, saved) = submit("_form=record&_section=domain_backup_v4&kind=AAAA&name=backup.lan&target=2a00:ee2::31");
        assert_eq!(
            ops(&saved),
            serde_json::json!([{
                "config": "dhcp", "section": "domain_backup_v4",
                "values": {"name": "backup.lan", "ip": "2a00:ee2::31"}
            }])
        );
    }

    #[test]
    fn deleting_a_record_removes_its_section() {
        let (model, saved) = submit("_form=record&_section=mx_lan&_delete=1");
        assert_eq!(
            ops(&saved),
            serde_json::json!([{"config": "dhcp", "section": "mx_lan", "delete": true}])
        );
        assert!(model.record_index("mx_lan").is_none());
    }

    #[test]
    fn a_value_dnsmasq_would_not_read_is_marked_and_nothing_is_written() {
        for (body, field) in [
            ("_form=record&_section=domain_backup_v4&kind=A&name=backup.lan&target=nas.lan", "target"),
            ("_form=record&_section=domain_backup_v6&kind=AAAA&name=backup.lan&target=10.0.0.31", "target"),
            ("_form=record&_section=domain_backup_v4&kind=A&name=&target=10.0.0.31", "name"),
            ("_form=record&_section=srv_matrix&kind=SRV&name=_matrix._tcp.lan&target=nas.lan&port=", "port"),
            ("_form=record&_section=srv_matrix&kind=SRV&name=_matrix._tcp.lan&target=nas.lan&port=8448&priority=high", "priority"),
        ] {
            let (_, saved) = submit(body);
            let Saved::Refused(refusal) = saved else {
                panic!("{body}: the submission should have been refused");
            };
            assert!(!refusal.errors.get(field).is_empty(), "{body}: {field} carries no error");
        }
    }

    #[test]
    fn a_refused_record_comes_back_open_carrying_what_was_typed() {
        let mut model = fixture::dnsdhcp();
        let Saved::Refused(refusal) = save(
            &mut model,
            &Form::parse("_form=record&_section=domain_backup_v4&kind=A&name=backup.lan&target=nowhere"),
        ) else {
            panic!("the submission should have been refused");
        };
        let rows = rows(&model, Some(&refusal));
        let drawer = &rows[0]["drawer"];
        assert_eq!(drawer["open"], true);
        assert_eq!(drawer["children"][0]["fields"][4]["value"], "nowhere");
        assert!(!drawer["children"][0]["fields"][4]["error"]
            .as_str()
            .unwrap_or("")
            .is_empty());
        // Every other row is untouched and closed.
        assert!(rows[1]["drawer"].get("open").is_none());
    }

    #[test]
    fn a_submission_naming_no_record_writes_nothing() {
        for body in [
            "_form=record&_section=no_such_record&kind=A&name=a.lan&target=10.0.0.1",
            "_form=record&_section=&kind=A&name=a.lan&target=10.0.0.1",
            "_form=record&_section=domain_backup_v4&kind=PTR&name=a.lan&target=10.0.0.1",
        ] {
            let (_, saved) = submit(body);
            assert!(matches!(saved, Saved::Unknown), "{body}");
        }
    }
}
