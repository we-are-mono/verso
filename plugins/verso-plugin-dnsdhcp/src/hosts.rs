// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

//! Devices pinned to an address — `config host` — as one listing, and a page
//! apiece for editing one.
//!
//! A reservation is a name, a MAC, an address and a handful of overrides, so it
//! is edited on a page of its own — the same screen for a new one and an existing
//! one. It is also the one thing here that is created rather than edited, and it
//! is created from a lease: the operator sees a device that has an address on the
//! Leases page and says "keep this one", which opens this page prefilled from the
//! device as it is.

use verso_plugin::{
    commit, commit_delete, commit_new, json, Envelope, Form, Map, TableRow, Tone, Value, Widget,
};

use crate::config;
use crate::form::{self, Errors};
use crate::leases;
use crate::live::{Lease, Leases};
use crate::model::{same_mac, Dnsdhcp, Options, CONFIG};
use crate::page;

/// TYPE is the uci section type a reservation is written as.
pub const TYPE: &str = "host";

const SUB: &str = "Devices pinned to an address — `config host`. Online is live: the device \
holds its lease right now.";

const EMPTY: &str = "No reserved addresses yet — reserve one from a device on the Leases page.";

const NEW_SUB: &str = "Keep an address with a device, so it always answers at the same place.";

const MISSING: &str =
    "That reservation isn’t here any more, so Verso showed you the configuration instead.";

/// OWNED is every option this page writes. A save states all of them, so an
/// option the operator cleared is cleared on disk rather than left behind, and
/// an option this page does not draw is never touched.
const OWNED: [&str; 7] = ["name", "mac", "ip", "hostid", "duid", "leasetime", "tag"];

/// Host is one reservation as the page holds it.
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
            // docks, and dnsmasq reads them all; the page edits them as written.
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
    /// right now, ready to be kept. A hostname the daemon would not read as a
    /// name is dropped rather than carried into a field that would then refuse
    /// the save; the edit page is where a name is added.
    pub fn of_lease(lease: &Lease) -> Host {
        Host {
            name: match form::valid_hostname(&lease.hostname) {
                true => lease.hostname.clone(),
                false => String::new(),
            },
            mac: lease.mac.clone(),
            ip: lease.ipv4.clone(),
            ..Host::default()
        }
    }

    fn submitted(section: &str, form: &Form) -> Host {
        let field = |name: &str| form.get(name).trim().to_string();
        Host {
            section: section.to_string(),
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

/// section renders the reservations listing on the DHCP configuration page. Each
/// row opens the reservation's own page; the tail adds a new one.
pub fn section(model: &Dnsdhcp, leases: &Leases) -> Widget {
    let rows = model
        .hosts
        .iter()
        .map(|options| {
            let host = Host::read(options);
            let online = leases.holder(&host.mac).is_some();
            row(&host, online)
        })
        .collect();
    Widget::section(
        "Reservations",
        SUB,
        vec![page::listing(
            page::columns(&[
                ("Name", "name"),
                ("MAC", "mono"),
                ("IPv4", "mono"),
                ("IPv6 suffix", "mono"),
                ("Online", "pill"),
                ("", "link"),
            ]),
            rows,
            EMPTY,
            "New reservation",
            &page::new_reservation_href(),
        )],
    )
}

fn row(host: &Host, online: bool) -> TableRow {
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
            page::edit_link_cell(page::reservation_href(&host.section)),
        ],
        ..TableRow::default()
    }
}

/// blank answers a visit to the new-reservation page. A `?reserve=<mac>` names a
/// device on the Leases page (the "Reserve this address" link), so the page opens
/// prefilled from that lease; without one it opens empty. A MAC no lease answers
/// to prefills nothing rather than erroring.
pub fn blank(leases: &Leases, query: &Form) -> Envelope {
    let mac = query.get(leases::RESERVE);
    let host = leases
        .all()
        .iter()
        .find(|lease| same_mac(&lease.mac, &mac))
        .map(Host::of_lease)
        .unwrap_or_default();
    editor(None, &host, &Errors::default())
}

/// edit answers a visit to one reservation's page, or nothing when the sub-path
/// names no reservation this config holds.
pub fn edit(model: &Dnsdhcp, section: &str) -> Option<Envelope> {
    let index = model.host_index(section)?;
    Some(editor(
        Some(section),
        &Host::read(&model.hosts[index]),
        &Errors::default(),
    ))
}

/// missing states that the sub-path names no reservation and answers with the
/// configuration page.
pub fn missing(model: &Dnsdhcp, leases: &Leases) -> Envelope {
    config::page(model, leases).with_notice(Tone::Danger, MISSING)
}

/// create answers the new-reservation page's submission: the section is added and
/// the configuration page answers with it staged.
pub fn create(model: &mut Dnsdhcp, leases: &Leases, form: &Form) -> Envelope {
    let stated = Host::submitted("", form);
    let errors = validate(&stated);
    if !errors.is_empty() {
        return editor(None, &stated, &errors).with_notice(Tone::Danger, form::REFUSED);
    }
    let op = commit_new(CONFIG, TYPE, values(&stated, false));
    // The new section's uci name is the shell's to assign, so the answer carries
    // the reservation without one; the next read brings it back named.
    let mut options = Options::default();
    apply(&mut options, &stated);
    model.hosts.push(options);
    config::page(model, leases)
        .with_notice(Tone::Success, "Address reserved.")
        .with_commit(vec![op])
}

/// save answers one reservation's page. A delete gives the device back to the
/// pool and answers with the configuration page; anything else is the page's own
/// submission — refused onto the page, or saved and answered with the listing.
pub fn save(model: &mut Dnsdhcp, leases: &Leases, section: &str, form: &Form) -> Option<Envelope> {
    let index = model.host_index(section)?;
    if form::deletes(form) {
        let removed = model.hosts.remove(index);
        return Some(
            config::page(model, leases)
                .with_notice(Tone::Success, "Reservation deleted.")
                .with_commit(vec![commit_delete(CONFIG, &removed.section)]),
        );
    }

    let stated = Host::submitted(section, form);
    let errors = validate(&stated);
    if !errors.is_empty() {
        return Some(
            editor(Some(section), &stated, &errors).with_notice(Tone::Danger, form::REFUSED),
        );
    }
    let op = commit(CONFIG, section, values(&stated, true));
    apply(&mut model.hosts[index], &stated);
    Some(
        config::page(model, leases)
            .with_notice(Tone::Success, "Reservation saved.")
            .with_commit(vec![op]),
    )
}

/// editor composes the reservation page — the new page and the edit page, one
/// screen. It keeps the DHCP top bar so the tabs stay put underneath the
/// operator.
fn editor(section: Option<&str>, host: &Host, errors: &Errors) -> Envelope {
    let (title, subheading, submit) = match section {
        Some(_) => ("Edit reservation", heading(&host.name), "Save changes"),
        None => ("New reservation", NEW_SUB.to_string(), "Add reservation"),
    };
    let mut children = vec![Widget::Form {
        style: "page".into(),
        submit: submit.into(),
        error: String::new(),
        fields: controls(host, errors, Subject::Reservation),
        note: String::new(),
        target: String::new(),
    }];
    children.push(footnote(section, host));
    if section.is_some() {
        children.push(form::delete_form(
            "Delete reservation",
            &format!(
                "Delete the reservation for {}? It falls back to a dynamic address.",
                host.subject()
            ),
        ));
    }
    page::dhcp_editor(title, &subheading, Widget::stack(children))
}

/// Subject is whose screen a reservation is being edited on, which decides one
/// thing: whether the device is still open to choice.
///
/// On a page of its own the reservation is the subject and every value it states
/// is a control, the MAC included — that is how a reservation is pointed at a
/// device in the first place. In a device's panel the device is the subject, and
/// the panel is already headed with it: retyping the MAC there would quietly
/// move the reservation to another device while the heading still named this
/// one, so the MAC rides as a hidden carrier and the panel edits what is
/// genuinely open.
#[derive(Clone, Copy, PartialEq)]
enum Subject {
    Reservation,
    Device,
}

/// controls is the reservation's editing surface, the same wherever it is shown.
/// The tips are on the three labels that are terms of art rather than plain
/// words — an operator who already knows them never opens one, and an operator
/// who does not is not made to leave the form to find out.
fn controls(host: &Host, errors: &Errors, subject: Subject) -> Vec<Widget> {
    let mac = match subject {
        Subject::Reservation => form::text_field("mac", "MAC", &host.mac, "", errors),
        Subject::Device => Widget::hidden("mac", &host.mac),
    };
    vec![
        form::text_field(
            "name",
            "Name",
            &host.name,
            "Also answers to this name on the local network.",
            errors,
        ),
        mac,
        form::text_field("ip", "IPv4 address", &host.ip, "", errors).explained(
            "The address this device is handed every time it asks, instead of whichever one \
             happens to be free. It has to sit inside the device’s own network and outside the \
             range unreserved devices draw from, or it will be handed out twice.",
            &format!("{CONFIG} {TYPE}"),
        ),
        form::text_field(
            "hostid",
            "IPv6 suffix",
            &host.hostid,
            "Pins the interface part of the IPv6 address, e.g. ::30. Blank leaves IPv6 to SLAAC.",
            errors,
        )
        .explained(
            "An IPv6 address is the network’s prefix followed by a suffix that picks out one \
             device on it. Pinning the suffix keeps this device at the same place even when the \
             provider hands the network a new prefix: the front of the address moves, the part \
             set here stays.",
            &format!("{CONFIG} {TYPE}"),
        ),
        // The one rule in this form: the fields above carry none, so the
        // boundary is what says the rest is optional.
        Widget::section(
            "Advanced",
            "Leave these unless something specific asks for them.",
            vec![
                form::text_field(
                    "duid",
                    "DUID",
                    &host.duid,
                    "Match a DHCPv6 client by DUID instead of MAC.",
                    errors,
                )
                .explained(
                    "Asking for an address over IPv6, a device names itself by a DUID — an \
                     identifier it makes up once and then keeps — rather than by its MAC. Fill \
                     this in only for a device whose IPv6 address the MAC above is not pinning; \
                     the DUID is readable off the device itself, or off the lease it holds.",
                    &format!("{CONFIG} {TYPE}"),
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
        )
        .ruled(),
    ]
}

/// footnote is what the save will put in the file, as it will put it. Someone
/// who knows uci checks the form said what they meant; everyone else reads the
/// fields and ignores this.
fn footnote(section: Option<&str>, host: &Host) -> Widget {
    // Not a live preview: this page's block is built where it is rendered, and
    // making it follow the form is a decision for this plugin's own pass.
    Widget::config(
        &format!("/etc/config/{CONFIG}"),
        &uci_preview(section, host),
    )
}

/// uci_preview is the reservation as uci would hold it: the section this save
/// lands in, then every option it states. An option the operator left blank is
/// left out, because a blank is a removal and the file simply will not carry
/// that line.
fn uci_preview(section: Option<&str>, host: &Host) -> String {
    let name = section.unwrap_or("@host[-1]");
    let mut out = format!("config {TYPE} '{name}'");
    for (option, value) in fields(host) {
        if !value.is_empty() {
            out.push_str(&format!("\n\toption {option} '{value}'"));
        }
    }
    out
}

/// heading names the reservation the page is about, or its own words when the
/// device carries no name yet.
fn heading(name: &str) -> String {
    match name.is_empty() {
        true => "An unnamed device.".to_string(),
        false => name.to_string(),
    }
}

/// apply reflects an accepted reservation in the model the answer renders from.
fn apply(options: &mut Options, host: &Host) {
    for (option, value) in fields(host) {
        options.set(option, (!value.is_empty()).then_some(value));
    }
}

/// fields pairs every option this page owns with the value it carries.
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

/// The words the reservation tab's commit row carries in a device's panel. A
/// reservation is reloaded rather than applied through the rollback window, so
/// the tab says exactly that instead of leaving the shell's general promise
/// standing. It names the role and not the daemon: `/etc/config/dhcp` is read by
/// dnsmasq and odhcpd together on a stock OpenWrt, and by neither on a board
/// resolving through something else — so the one thing this plugin can honestly
/// promise is that whatever serves DHCP here picks the change up at once.
const TAB_LABEL: &str = "Reserved address";
const TAB_CTA_NEW: &str = "Reserve address";
const TAB_CTA_EDIT: &str = "Save reservation";
const TAB_NOTE: &str = "Applies immediately — the DHCP server reloads, no rollback needed.";

/// entity_section is the reservation a device already holds, if any — the handle
/// a submitted tab saves into, so the same form creates or edits without the
/// caller having to know which.
pub fn entity_section(model: &Dnsdhcp, mac: &str) -> Option<String> {
    model
        .hosts
        .iter()
        .map(Host::read)
        .find(|host| same_mac(&host.mac, mac))
        .map(|host| host.section)
}

/// entity_tab answers the shell's request for this plugin's say about one
/// device: the reservation it already has, or the one it could be given,
/// prefilled from the lease it is holding right now.
///
/// The shell frames this as one tab of the device's panel beside whatever other
/// plugins had to say. It knows nothing of them, and they know nothing of it.
pub fn entity_tab(model: &Dnsdhcp, leases: &Leases, mac: &str) -> Envelope {
    // A device that does not exist yet: the same controls, empty, so making a
    // reservation and editing one are the same form. There is no device to be
    // the subject either, so the MAC is a control here as it is on the page.
    if mac == "new" {
        return tab(None, &Host::default(), Subject::Reservation, TAB_CTA_NEW);
    }
    let existing = model
        .hosts
        .iter()
        .map(Host::read)
        .find(|host| same_mac(&host.mac, mac));

    let (section, host, cta) = match existing {
        Some(host) => (Some(host.section.clone()), host, TAB_CTA_EDIT),
        None => {
            // No reservation yet: open on the lease this device is holding, so
            // the address it already has is the one being offered to keep.
            let host = leases
                .all()
                .iter()
                .find(|lease| same_mac(&lease.mac, mac))
                .map(Host::of_lease)
                .unwrap_or_else(|| Host {
                    mac: mac.to_string(),
                    ..Host::default()
                });
            (None, host, TAB_CTA_NEW)
        }
    };
    tab(section.as_deref(), &host, Subject::Device, cta)
}

/// tab is the reservation as one tab of a panel: the controls and the footnote,
/// and nothing else. The panel's own frame carries the heading, the commit row
/// and the close, and the listing row beside it already carries the removal —
/// so a delete inside here would be the same act offered twice.
fn tab(section: Option<&str>, host: &Host, subject: Subject, cta: &str) -> Envelope {
    let body = Widget::stack(vec![
        Widget::Form {
            style: "page".into(),
            submit: String::new(),
            error: String::new(),
            fields: controls(host, &Errors::default(), subject),
            note: String::new(),
            target: String::new(),
        },
        footnote(section, host),
    ]);
    Envelope::page(TAB_LABEL, body)
        .with_commit_row(cta, TAB_NOTE)
        .with_tab_state(&tab_state(section, host))
}

/// tab_state is where this device stands on the question the tab answers, for
/// the chip the shell hangs beside the label: the address it is pinned to, or
/// that it is pinned to none. A reservation is *for* an address, so the address
/// is the state — anything shorter would say less than the word it replaced.
fn tab_state(section: Option<&str>, host: &Host) -> String {
    match (section.is_some(), host.ip.is_empty()) {
        (false, _) => "none".to_string(),
        (true, true) => "reserved".to_string(),
        (true, false) => host.ip.clone(),
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

    fn listing_rows(model: &Dnsdhcp) -> Vec<Json> {
        let json = serde_json::to_value(section(model, &fixture::leases())).expect("serialize");
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
            // The advanced options are a section of the form, not a fold, so a
            // control may sit one level down inside it.
            .flat_map(|f| match f["type"] == "section" {
                true => f["children"].as_array().cloned().unwrap_or_default(),
                false => vec![f.clone()],
            })
            .find(|f| f["name"] == name)
            .unwrap_or_else(|| panic!("no control {name}"))
    }

    fn saved(section: &str, body_str: &str) -> (Dnsdhcp, Json) {
        let mut model = fixture::dnsdhcp();
        let env = save(
            &mut model,
            &fixture::leases(),
            section,
            &Form::parse(body_str),
        )
        .expect("the fixture holds this reservation");
        (model, body(env))
    }

    #[test]
    fn every_row_leads_to_its_own_page_and_states_whether_it_is_here() {
        let rows = listing_rows(&fixture::dnsdhcp());
        assert_eq!(rows[0]["id"], "host_nas");
        assert_eq!(rows[0]["cells"][0]["text"], "nas");
        assert_eq!(rows[0]["cells"][1]["text"], "30:9C:23:5E:88:01");
        assert_eq!(rows[0]["cells"][3]["text"], "::30");
        assert_eq!(
            rows[0]["cells"][4],
            serde_json::json!({"text": "online", "variant": "success"})
        );
        assert_eq!(
            rows[0]["cells"][5],
            serde_json::json!({"text": "Edit", "href": page::reservation_href("host_nas")})
        );
        // A reservation whose device is not here says nothing rather than "offline".
        assert_eq!(rows[1]["cells"][4], serde_json::json!({}));
        for row in &rows {
            assert!(row.get("drawer").is_none(), "{row}");
        }
    }

    #[test]
    fn the_edit_page_carries_the_reservation_and_keeps_the_dhcp_tabs() {
        let model = fixture::dnsdhcp();
        let nas = body(edit(&model, "host_nas").expect("reservation"));
        assert_eq!(nas["title"], "Edit reservation");
        assert_eq!(nas["subheading"], "nas");
        assert_eq!(nas["pages"][1]["path"], "config", "the DHCP tabs stay put");
        assert_eq!(control(&nas, "mac")["value"], "30:9C:23:5E:88:01");
        assert_eq!(control(&nas, "hostid")["value"], "::30");
        assert_eq!(nas["widget"]["children"][0]["style"], "page");
        // children[1] is the form's own footnote — the lines this save writes.
        assert_eq!(nas["widget"]["children"][1]["type"], "code");
        assert_eq!(nas["widget"]["children"][1]["label"], "/etc/config/dhcp");
        assert_eq!(nas["widget"]["children"][2]["fields"][1]["type"], "confirm");

        let blank = body(blank(&fixture::leases(), &Form::default()));
        assert_eq!(blank["title"], "New reservation");
        assert_eq!(control(&blank, "name")["value"], "");
        // A reservation that does not exist yet has a form and its footnote, and
        // nothing to delete.
        assert_eq!(
            blank["widget"]["children"]
                .as_array()
                .expect("children")
                .len(),
            2
        );
        assert_eq!(blank["widget"]["children"][1]["type"], "code");
    }

    #[test]
    fn the_new_page_opens_prefilled_from_the_lease_a_reserve_link_named() {
        let query = Form::parse("reserve=42:e6:ad:ff:b7:af");
        let prefilled = body(blank(&fixture::leases(), &query));
        assert_eq!(control(&prefilled, "name")["value"], "toms-iphone");
        assert_eq!(control(&prefilled, "mac")["value"], "42:e6:ad:ff:b7:af");
        assert_eq!(control(&prefilled, "ip")["value"], "10.0.0.142");

        // A MAC no lease answers to prefills nothing rather than erroring.
        for reserve in ["", "aa:bb:cc:dd:ee:ff", "nonsense"] {
            let body = body(blank(
                &fixture::leases(),
                &Form::parse(&format!("reserve={reserve}")),
            ));
            assert_eq!(control(&body, "mac")["value"], "", "{reserve}");
        }
    }

    #[test]
    fn a_devices_panel_edits_only_what_is_still_open_to_choice() {
        let model = fixture::dnsdhcp();
        let leases = fixture::leases();

        // The panel is headed with the device, so its MAC is settled: it rides
        // as a hidden carrier the save still posts, never as a control that
        // could point the reservation at some other device.
        let held = body(entity_tab(&model, &leases, "30:9C:23:5E:88:01"));
        assert_eq!(held["title"], "Reserved address");
        assert_eq!(held["cta"], "Save reservation");
        assert_eq!(control(&held, "mac")["kind"], "hidden");
        assert_eq!(control(&held, "mac")["value"], "30:9C:23:5E:88:01");

        // Removing the reservation is the listing row's own act, so the panel
        // carries no delete: the controls and the footnote are the whole tab.
        let children = held["widget"]["children"].as_array().expect("children");
        assert_eq!(children.len(), 2);
        assert_eq!(children[1]["type"], "code");

        // A device holding a lease and no reservation opens on that lease.
        let fresh = body(entity_tab(&model, &leases, "42:e6:ad:ff:b7:af"));
        assert_eq!(fresh["cta"], "Reserve address");
        assert_eq!(control(&fresh, "ip")["value"], "10.0.0.142");
        assert_eq!(control(&fresh, "mac")["kind"], "hidden");

        // No device at all: nothing is settled, so the MAC is a control again.
        let blank = body(entity_tab(&model, &leases, "new"));
        assert_eq!(control(&blank, "mac")["kind"], "text");
    }

    #[test]
    fn the_labels_that_are_terms_of_art_explain_themselves() {
        let model = fixture::dnsdhcp();
        let nas = body(edit(&model, "host_nas").expect("reservation"));
        for name in ["ip", "hostid", "duid"] {
            let field = control(&nas, name);
            assert!(
                field["tip"].as_str().is_some_and(|tip| !tip.is_empty()),
                "{name}"
            );
            assert_eq!(field["source"], "dhcp host", "{name}");
        }
        // A label that is already the plain word for the thing raises nothing,
        // and an explanation is the field's wherever it is edited.
        for name in ["name", "leasetime", "tag"] {
            assert!(control(&nas, name).get("tip").is_none(), "{name}");
        }
        let panel = body(entity_tab(&model, &fixture::leases(), "30:9C:23:5E:88:01"));
        assert_eq!(control(&panel, "ip")["tip"], control(&nas, "ip")["tip"]);
    }

    #[test]
    fn creating_a_reservation_states_the_section_and_lands_on_the_configuration() {
        let mut model = fixture::dnsdhcp();
        let body = body(create(
            &mut model,
            &fixture::leases(),
            &Form::parse("name=toms-iphone&mac=42:e6:ad:ff:b7:af&ip=10.0.0.142"),
        ));
        assert_eq!(body["title"], "DHCP");
        assert_eq!(body["notice"]["text"], "Address reserved.");
        assert_eq!(
            body["commit"],
            serde_json::json!([{
                "config": "dhcp", "section": "", "type": "host",
                "values": {"name": "toms-iphone", "mac": "42:e6:ad:ff:b7:af", "ip": "10.0.0.142"}
            }])
        );
        assert!(model.reserved("42:e6:ad:ff:b7:af"));
    }

    #[test]
    fn saving_a_reservation_clears_every_option_it_no_longer_states() {
        let (model, body) = saved("host_nas", "name=nas&mac=30:9C:23:5E:88:01&ip=10.0.0.30");
        assert_eq!(body["title"], "DHCP");
        assert_eq!(
            body["commit"],
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
    fn deleting_a_reservation_removes_its_section_and_lands_on_the_configuration() {
        let (model, body) = saved("host_thermo", "_delete=1");
        assert_eq!(body["title"], "DHCP");
        assert_eq!(body["notice"]["text"], "Reservation deleted.");
        assert_eq!(
            body["commit"],
            serde_json::json!([{"config": "dhcp", "section": "host_thermo", "delete": true}])
        );
        assert!(model.host_index("host_thermo").is_none());
    }

    #[test]
    fn a_reservation_the_daemons_would_skip_is_marked_on_the_page_and_nothing_is_written() {
        for (body_str, field) in [
            ("mac=not-a-mac&ip=10.0.0.9", "mac"),
            ("mac=30:9c:23:5e:88:01&ip=10.0.0.256", "ip"),
            ("mac=30:9c:23:5e:88:01&ip=10.0.0.9&name=not+a+name", "name"),
            ("mac=30:9c:23:5e:88:01&ip=10.0.0.9&hostid=::zz", "hostid"),
            (
                "mac=30:9c:23:5e:88:01&ip=10.0.0.9&leasetime=forever",
                "leasetime",
            ),
            // A reservation that matches a device and then hands it nothing.
            ("mac=30:9c:23:5e:88:01", "ip"),
        ] {
            let mut model = fixture::dnsdhcp();
            let body = body(create(
                &mut model,
                &fixture::leases(),
                &Form::parse(body_str),
            ));
            assert_eq!(body["title"], "New reservation", "{body_str}");
            assert!(body.get("commit").is_none(), "{body_str}");
            assert!(
                !control(&body, field)["error"]
                    .as_str()
                    .unwrap_or("")
                    .is_empty(),
                "{body_str}: {field} carries no error"
            );
        }
    }

    #[test]
    fn a_refused_edit_comes_back_on_the_page_carrying_what_was_typed() {
        let (_, body) = saved("host_nas", "name=nas&mac=nope&ip=10.0.0.30");
        assert_eq!(body["title"], "Edit reservation");
        assert!(body.get("commit").is_none());
        assert_eq!(control(&body, "mac")["value"], "nope");
        assert!(!control(&body, "mac")["error"]
            .as_str()
            .unwrap_or("")
            .is_empty());
    }

    #[test]
    fn a_sub_path_naming_no_reservation_answers_with_the_configuration() {
        let model = fixture::dnsdhcp();
        assert!(edit(&model, "no_such_host").is_none());

        let mut model = fixture::dnsdhcp();
        assert!(save(
            &mut model,
            &fixture::leases(),
            "no_such_host",
            &Form::parse("mac=30:9c:23:5e:88:01&ip=10.0.0.9")
        )
        .is_none());

        let body = body(missing(&fixture::dnsdhcp(), &fixture::leases()));
        assert_eq!(body["title"], "DHCP");
        assert_eq!(body["notice"]["level"], "danger");
    }
}
