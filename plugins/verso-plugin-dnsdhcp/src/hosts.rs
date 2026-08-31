// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

//! Devices pinned to an address — `config host`.
//!
//! A reservation is the one thing on these pages that is created rather than
//! edited, and it is created from a lease: the operator sees a device that has
//! an address and says "keep this one". So the same form serves both faces — the
//! Reserve panel on the Leases listing and the edit panel on the Reservations
//! listing — and the only difference is whether it names a section that already
//! exists.

use verso_plugin::{
    commit, commit_delete, commit_new, json, CommitOp, Form, Map, RowDrawer, TableRow, Value,
    Widget,
};

use crate::form::{self, Errors};
use crate::live::{Lease, Leases};
use crate::model::{Dnsdhcp, Options, CONFIG};
use crate::page;

/// TYPE is the uci section type a reservation is written as.
pub const TYPE: &str = "host";

const SUB: &str = "Devices pinned to an address — `config host`. Online is live: the device \
holds its lease right now.";

/// OWNED is every option this form writes. A save states all of them, so an
/// option the operator cleared is cleared on disk rather than left behind, and
/// an option this form does not draw is never touched.
const OWNED: [&str; 7] = ["name", "mac", "ip", "hostid", "duid", "leasetime", "tag"];

/// Host is one reservation as the form holds it.
#[derive(Default, Clone)]
pub struct Host {
    pub section: String,
    pub name: String,
    pub mac: String,
    pub ip: String,
    pub hostid: String,
    pub duid: String,
    pub leasetime: String,
    pub tag: String,
}

impl Host {
    /// read takes a reservation off the config.
    pub fn read(options: &Options) -> Host {
        Host {
            section: options.section.clone(),
            // uci writes several MACs onto one option to follow a device between
            // docks, and dnsmasq reads them all; the form edits them as written.
            mac: options.list("mac").join(" "),
            name: options.scalar("name").into(),
            ip: options.scalar("ip").into(),
            hostid: options.scalar("hostid").into(),
            duid: options.scalar("duid").into(),
            leasetime: options.scalar("leasetime").into(),
            tag: options.list("tag").join(" "),
        }
    }

    /// of_lease is the reservation a lease would become — the device as it is
    /// right now, ready to be kept.
    pub fn of_lease(lease: &Lease) -> Host {
        Host {
            name: lease.hostname.clone(),
            mac: lease.mac.clone(),
            ip: lease.ipv4.clone(),
            ..Host::default()
        }
    }

    fn submitted(form: &Form) -> Host {
        let field = |name: &str| form.get(name).trim().to_string();
        Host {
            section: field(form::SECTION),
            name: field("name"),
            mac: field("mac"),
            ip: field("ip"),
            hostid: field("hostid"),
            duid: field("duid"),
            leasetime: field("leasetime"),
            tag: field("tag"),
        }
    }

    /// subject is what a sentence calls this reservation.
    fn subject(&self) -> String {
        match self.name.is_empty() {
            true => "this device".to_string(),
            false => format!("“{}”", self.name),
        }
    }
}

/// Refusal is a reservation the operator stated and the daemon would not read.
pub struct Refusal {
    pub host: Host,
    pub errors: Errors,
}

/// Saved is what a reservation submission amounts to.
pub enum Saved {
    Ops(Vec<CommitOp>, &'static str),
    Refused(Refusal),
    Unknown,
}

/// section renders the reservations listing.
pub fn section(model: &Dnsdhcp, leases: &Leases, refusal: Option<&Refusal>) -> Widget {
    let rows = model
        .hosts
        .iter()
        .map(|options| {
            let host = Host::read(options);
            let online = leases.holder(&host.mac).is_some();
            match refusal {
                Some(refused) if refused.host.section == host.section => {
                    row(&refused.host, online, &refused.errors, true)
                }
                _ => row(&host, online, &Errors::default(), false),
            }
        })
        .collect();
    Widget::section(
        "Reservations",
        SUB,
        vec![page::table(
            page::columns(&[
                ("Name", "name"),
                ("MAC", "mono"),
                ("IPv4", "mono"),
                ("IPv6 suffix", "mono"),
                ("Online", "pill"),
            ]),
            rows,
        )],
    )
}

fn row(host: &Host, online: bool, errors: &Errors, open: bool) -> TableRow {
    let state = match online {
        true => page::pill_cell("online", "success"),
        false => page::pill_cell("", ""),
    };
    TableRow {
        id: host.section.clone(),
        cells: vec![
            page::name_cell(&host.name, ""),
            page::address_cell(&host.mac),
            page::address_cell(&host.ip),
            page::address_cell(&host.hostid),
            state,
        ],
        drawer: edit_drawer(host, errors, open),
        ..TableRow::default()
    }
}

/// edit_drawer is a reservation's own panel: the host form, and the confirm that
/// gives the device back to the pool.
fn edit_drawer(host: &Host, errors: &Errors, open: bool) -> Option<RowDrawer> {
    let title = match host.name.is_empty() {
        true => "Edit reservation".to_string(),
        false => format!("Edit reservation — {}", host.name),
    };
    form::drawer(
        &title,
        open,
        vec![
            host_form("Save", host, errors),
            form::delete_form(
                form::HOST,
                (form::SECTION, &host.section),
                "Delete reservation",
                &format!(
                    "Delete the reservation for {}? It falls back to a dynamic address.",
                    host.subject()
                ),
            ),
        ],
    )
}

/// reserve_drawer is the panel a live lease opens: the same form, prefilled from
/// the device as it is, naming no section — which is what makes the save create
/// one.
pub fn reserve_drawer(host: &Host, errors: &Errors, open: bool) -> Option<RowDrawer> {
    let title = match host.name.is_empty() {
        true => "Reserve address".to_string(),
        false => format!("Reserve address — {}", host.name),
    };
    form::drawer(&title, open, vec![host_form("Reserve", host, errors)])
}

/// host_form is the reservation form both panels carry.
fn host_form(submit: &str, host: &Host, errors: &Errors) -> Widget {
    Widget::Form {
        style: String::new(),
        submit: submit.into(),
        error: String::new(),
        fields: vec![
            Widget::hidden(form::KIND, form::HOST),
            Widget::hidden(form::SECTION, &host.section),
            form::text_field("name", "Name", &host.name, "", errors),
            form::text_field("mac", "MAC", &host.mac, "", errors),
            form::text_field("ip", "IPv4 address", &host.ip, "", errors),
            form::text_field(
                "hostid",
                "IPv6 suffix",
                &host.hostid,
                "Pins the interface part of the IPv6 address, e.g. ::30. Blank leaves IPv6 to SLAAC.",
                errors,
            ),
            Widget::disclosure(
                "Advanced — 3 more options",
                vec![
                    form::text_field(
                        "duid",
                        "DUID",
                        &host.duid,
                        "Match a DHCPv6 client by DUID instead of MAC.",
                        errors,
                    ),
                    form::text_field(
                        "leasetime",
                        "Lease time override",
                        &host.leasetime,
                        "Blank uses the network's lease length.",
                        errors,
                    ),
                    form::text_field(
                        "tag",
                        "Tag",
                        &host.tag,
                        "Hand this device the options set for a tag.",
                        errors,
                    ),
                ],
            ),
        ],
    }
}

/// save answers a reservation submission, from either panel.
pub fn save(model: &mut Dnsdhcp, form: &Form) -> Saved {
    let stated = Host::submitted(form);
    let existing = model.host_index(&stated.section);
    if !stated.section.is_empty() && existing.is_none() {
        return Saved::Unknown;
    }
    if form::deletes(form) {
        let Some(index) = existing else {
            return Saved::Unknown;
        };
        let removed = model.hosts.remove(index);
        return Saved::Ops(
            vec![commit_delete(CONFIG, &removed.section)],
            "Reservation deleted.",
        );
    }

    let errors = validate(&stated);
    if !errors.is_empty() {
        return Saved::Refused(Refusal {
            host: stated,
            errors,
        });
    }
    match existing {
        Some(index) => {
            let op = commit(CONFIG, &stated.section, values(&stated, true));
            apply(&mut model.hosts[index], &stated);
            Saved::Ops(vec![op], "Reservation saved.")
        }
        None => {
            let op = commit_new(CONFIG, TYPE, values(&stated, false));
            // The new section's uci name is the shell's to assign, so the answer
            // carries the reservation without one; the next read brings it back
            // named.
            let mut options = Options::default();
            apply(&mut options, &stated);
            model.hosts.push(options);
            Saved::Ops(vec![op], "Address reserved.")
        }
    }
}

/// apply reflects an accepted reservation in the model the answer renders from.
fn apply(options: &mut Options, host: &Host) {
    for (option, value) in fields(host) {
        options.set(option, (!value.is_empty()).then_some(value));
    }
}

/// fields pairs every option this form owns with the value it carries.
fn fields(host: &Host) -> [(&'static str, &str); 7] {
    [
        ("name", &host.name),
        ("mac", &host.mac),
        ("ip", &host.ip),
        ("hostid", &host.hostid),
        ("duid", &host.duid),
        ("leasetime", &host.leasetime),
        ("tag", &host.tag),
    ]
}

/// values is what the save writes: what the reservation states, plus — when the
/// section already exists — a null for each owned option it no longer states.
fn values(host: &Host, existing: bool) -> Value {
    let mut values = Map::new();
    for (option, value) in fields(host) {
        if !value.is_empty() {
            values.insert(option.to_string(), json!(value));
        }
    }
    if existing {
        for option in OWNED {
            values.entry(option.to_string()).or_insert(Value::Null);
        }
    }
    Value::Object(values)
}

/// validate answers the daemons' question. A reservation with no MAC and no DUID
/// matches nothing, and one that states no address, name or suffix does nothing
/// once it matches — dnsmasq skips both without a word.
fn validate(host: &Host) -> Errors {
    let mut errors = Errors::default();
    errors.check(
        "mac",
        form::valid_macs(&host.mac) || (host.mac.is_empty() && !host.duid.is_empty()),
        "Write the device's MAC address, such as 30:9c:23:5e:88:01.",
    );
    errors.check(
        "ip",
        host.ip.is_empty() || form::valid_ipv4(&host.ip),
        "Write the IPv4 address to keep for this device, such as 10.0.0.30.",
    );
    errors.check(
        "name",
        host.name.is_empty() || form::valid_hostname(&host.name),
        "Write the name this device answers to, such as nas.",
    );
    errors.check(
        "hostid",
        host.hostid.is_empty() || form::valid_hostid(&host.hostid),
        "Write the interface part of the IPv6 address, such as ::30.",
    );
    errors.check(
        "duid",
        host.duid.is_empty() || host.duid.chars().all(|c| c.is_ascii_hexdigit()),
        "Write the client's DUID as hexadecimal.",
    );
    errors.check(
        "leasetime",
        host.leasetime.is_empty() || form::valid_leasetime(&host.leasetime),
        "Write a length such as 12h, 30m, or infinite.",
    );
    if errors.is_empty() && host.ip.is_empty() && host.name.is_empty() && host.hostid.is_empty() {
        errors.check(
            "ip",
            false,
            "A reservation needs an address, a name, or an IPv6 suffix to hand out.",
        );
    }
    errors
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::fixture;
    use serde_json::Value as Json;

    fn rows(model: &Dnsdhcp, refusal: Option<&Refusal>) -> Vec<Json> {
        let json = serde_json::to_value(section(model, &fixture::leases(), refusal))
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
    fn a_reservation_reads_as_its_pinned_facts_and_whether_it_is_here() {
        let rows = rows(&fixture::dnsdhcp(), None);
        assert_eq!(rows[0]["id"], "host_nas");
        assert_eq!(rows[0]["cells"][0]["text"], "nas");
        assert_eq!(rows[0]["cells"][1]["text"], "30:9C:23:5E:88:01");
        assert_eq!(rows[0]["cells"][3]["text"], "::30");
        assert_eq!(rows[0]["cells"][4], serde_json::json!({"text": "online", "variant": "success"}));
        // A reservation whose device is not here says nothing rather than "offline".
        assert_eq!(rows[1]["cells"][4], serde_json::json!({}));
        assert_eq!(rows[1]["cells"][3], serde_json::json!({"text": "—", "muted": true}));

        let drawer = &rows[0]["drawer"];
        assert_eq!(drawer["title"], "Edit reservation — nas");
        let fields = &drawer["children"][0]["fields"];
        assert_eq!(
            fields[1],
            serde_json::json!({"type": "field", "name": "_section", "kind": "hidden", "value": "host_nas"})
        );
        assert_eq!(drawer["children"][0]["submit"], "Save");
        assert_eq!(drawer["children"][1]["fields"][3]["type"], "confirm");
    }

    #[test]
    fn reserving_a_lease_creates_the_section_it_needs() {
        let (model, saved) = submit(
            "_form=host&_section=&name=toms-iphone&mac=42:e6:ad:ff:b7:af&ip=10.0.0.142",
        );
        assert_eq!(
            ops(&saved),
            serde_json::json!([{
                "config": "dhcp", "section": "", "type": "host",
                "values": {"name": "toms-iphone", "mac": "42:e6:ad:ff:b7:af", "ip": "10.0.0.142"}
            }])
        );
        // The answer already reads the device as reserved.
        assert!(model.reserved("42:e6:ad:ff:b7:af"));
    }

    #[test]
    fn saving_a_reservation_clears_every_option_it_no_longer_states() {
        let (model, saved) = submit("_form=host&_section=host_nas&name=nas&mac=30:9C:23:5E:88:01&ip=10.0.0.30");
        assert_eq!(
            ops(&saved),
            serde_json::json!([{
                "config": "dhcp", "section": "host_nas",
                "values": {
                    "name": "nas", "mac": "30:9C:23:5E:88:01", "ip": "10.0.0.30",
                    "hostid": null, "duid": null, "leasetime": null, "tag": null
                }
            }])
        );
        let index = model.host_index("host_nas").expect("host");
        assert_eq!(Host::read(&model.hosts[index]).hostid, "");
    }

    #[test]
    fn deleting_a_reservation_removes_its_section() {
        let (model, saved) = submit("_form=host&_section=host_thermo&_delete=1");
        assert_eq!(
            ops(&saved),
            serde_json::json!([{"config": "dhcp", "section": "host_thermo", "delete": true}])
        );
        assert!(model.host_index("host_thermo").is_none());
    }

    #[test]
    fn a_reservation_the_daemons_would_skip_is_marked_and_nothing_is_written() {
        for (body, field) in [
            ("_form=host&_section=&mac=not-a-mac&ip=10.0.0.9", "mac"),
            ("_form=host&_section=&mac=30:9c:23:5e:88:01&ip=10.0.0.256", "ip"),
            ("_form=host&_section=&mac=30:9c:23:5e:88:01&ip=10.0.0.9&name=not+a+name", "name"),
            ("_form=host&_section=&mac=30:9c:23:5e:88:01&ip=10.0.0.9&hostid=::zz", "hostid"),
            ("_form=host&_section=&mac=30:9c:23:5e:88:01&ip=10.0.0.9&leasetime=forever", "leasetime"),
            // A reservation that matches a device and then hands it nothing.
            ("_form=host&_section=&mac=30:9c:23:5e:88:01", "ip"),
        ] {
            let (_, saved) = submit(body);
            let Saved::Refused(refusal) = saved else {
                panic!("{body}: the submission should have been refused");
            };
            assert!(!refusal.errors.get(field).is_empty(), "{body}: {field} carries no error");
        }
    }

    #[test]
    fn a_refused_reservation_comes_back_open_carrying_what_was_typed() {
        let mut model = fixture::dnsdhcp();
        let Saved::Refused(refusal) = save(
            &mut model,
            &Form::parse("_form=host&_section=host_nas&name=nas&mac=nope&ip=10.0.0.30"),
        ) else {
            panic!("the submission should have been refused");
        };
        let rows = rows(&model, Some(&refusal));
        assert_eq!(rows[0]["drawer"]["open"], true);
        assert_eq!(rows[0]["drawer"]["children"][0]["fields"][3]["value"], "nope");
        assert!(rows[1]["drawer"].get("open").is_none());
    }

    #[test]
    fn a_submission_naming_no_reservation_writes_nothing() {
        for body in [
            "_form=host&_section=no_such_host&mac=30:9c:23:5e:88:01&ip=10.0.0.9",
            "_form=host&_section=&_delete=1",
        ] {
            let (_, saved) = submit(body);
            assert!(matches!(saved, Saved::Unknown), "{body}");
        }
    }
}
