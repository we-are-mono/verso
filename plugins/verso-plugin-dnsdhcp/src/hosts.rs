// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

//! Devices pinned to an address — `config host` — as one listing on the DHCP
//! page, each edited in its own drawer.
//!
//! A reservation is an object, so it lives in its drawer (ADR-005 §8): its row
//! opens it, the listing's New opens it blank, a lease's Reserve opens it filled
//! from that lease, and the device panel's "Reserved address" tab is the same
//! form again. One set of controls draws all four.

use verso_plugin::{
    commit, commit_delete, commit_new, json, ColumnWidth, CommitOp, Envelope, Errors, Form, Map,
    RowDrawer, Table, TableCell, TableColumn, TableRow, TableRowAct, Value, Widget,
};

use crate::form;
use crate::live::{Lease, Leases};
use crate::model::{same_mac, Dnsdhcp, Options, CONFIG};
use crate::page;

/// TYPE is the uci section type a reservation is written as.
pub const TYPE: &str = "host";

/// REMOVE names the reservation a row's confirmed removal takes away.
pub const REMOVE: &str = "_remove";

const EMPTY: &str = "No device has a reserved address. Reserve one from its lease below.";

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
    /// right now, ready to be kept. A hostname the daemon would not read as a
    /// name is dropped rather than carried into a field that would then refuse
    /// the save.
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

    /// title is what the drawer is headed with: the device's name, or plainly
    /// that it has none.
    fn title(&self) -> String {
        match self.name.is_empty() {
            true => "An unnamed device".to_string(),
            false => self.name.clone(),
        }
    }

}

/// find is the reservation an address names: its uci section, or the MAC of the
/// device it pins — the device panel and the Devices page know a device by its
/// MAC alone.
pub fn find<'a>(model: &'a Dnsdhcp, key: &str) -> Option<&'a Options> {
    model.hosts.iter().find(|host| host.section == key).or_else(|| {
        model
            .hosts
            .iter()
            .find(|host| host.list("mac").iter().any(|mac| same_mac(mac, key)))
    })
}

/// table is the reservations listing. Each row opens its own drawer where it
/// stands, and the row `open` names arrives with that drawer already open.
pub fn table(model: &Dnsdhcp, open: Option<(String, RowDrawer)>) -> Widget {
    let mut open = open;
    let rows = model
        .hosts
        .iter()
        .map(|options| {
            let host = Host::read(options);
            let drawer = match &open {
                Some((section, _)) if *section == host.section => open.take().map(|(_, d)| d),
                _ => None,
            };
            row(model, &host, drawer)
        })
        .collect();
    Widget::Table(Table {
        columns: columns(&[
            ("Device", "name", ColumnWidth::Name),
            ("Network", "entity", ColumnWidth::Short),
            ("IPv4", "mono", ColumnWidth::Address),
            ("MAC", "mono", ColumnWidth::Address),
            ("IPv6 suffix", "mono", ColumnWidth::Grow),
            ("", "actions", ColumnWidth::Short),
        ]),
        rows,
        empty_text: EMPTY.into(),
        ..Default::default()
    })
}

fn row(model: &Dnsdhcp, host: &Host, drawer: Option<RowDrawer>) -> TableRow {
    let door = page::open_href(&host.section);
    let network = model.network_of(&host.ip);
    TableRow {
        id: host.section.clone(),
        cells: vec![
            TableCell {
                text: host.title(),
                href: door.clone(),
                ..TableCell::default()
            },
            page::network_cell(network),
            page::address_cell(&host.ip),
            page::mono_cell(&host.mac),
            page::mono_cell(&host.hostid),
            TableCell {
                actions: vec![TableRowAct {
                    icon: "pin-off".into(),
                    title: "Remove reservation".into(),
                    name: REMOVE.into(),
                    value: host.section.clone(),
                    confirm_title: "Remove the reservation for %s?".into(),
                    confirm: "The device keeps its address until its lease runs out, then gets \
                              whichever one is free."
                        .into(),
                    ..TableRowAct::default()
                }],
                ..TableCell::default()
            },
        ],
        drawer,
        panel: door,
        ..TableRow::default()
    }
}

fn columns(spec: &[(&str, &str, ColumnWidth)]) -> Vec<TableColumn> {
    spec.iter()
        .map(|(label, kind, width)| TableColumn {
            label: (*label).into(),
            kind: (*kind).into(),
            width: *width,
        })
        .collect()
}

/// drawer is one reservation's panel: an existing one (`section`), or a blank
/// one for a device not reserved yet, stated from `host` — the config's values
/// on a visit, a lease's on a Reserve, what was typed on a refused save.
pub fn drawer(section: Option<&str>, host: &Host, errors: &Errors) -> RowDrawer {
    let title = match section {
        Some(_) => host.title(),
        None => "New reservation".to_string(),
    };
    let mut fields = controls(host, errors, Subject::Reservation);
    fields.push(preview(section, host));
    RowDrawer {
        title,
        closed: page::root_href(),
        open: true,
        children: vec![page::panel_form("Save reservation", fields)],
        ..RowDrawer::default()
    }
}

/// blank is the drawer for a reservation not made yet. A `reserve` naming a
/// device that holds a lease opens it filled from that lease; any other value
/// opens it empty rather than erroring.
pub fn blank(leases: &Leases, reserve: &str) -> RowDrawer {
    let host = leases
        .all()
        .iter()
        .find(|lease| same_mac(&lease.mac, reserve))
        .map(Host::of_lease)
        .unwrap_or_default();
    drawer(None, &host, &Errors::default())
}

/// Submitted is what a drawer's save came to: the write it stages, and the
/// drawer to show while it stages — or, refused, the drawer carrying what was
/// typed and why it was refused.
pub struct Submitted {
    pub drawer: RowDrawer,
    pub commit: Option<CommitOp>,
}

/// submit reads one drawer's save back: a new reservation (`section` None) or
/// an existing one.
pub fn submit(section: Option<&str>, form: &Form) -> Submitted {
    let (stated, errors, commit) = read_back(section, form);
    Submitted {
        drawer: drawer(section, &stated, &errors),
        commit,
    }
}

/// read_back is a submission as the reservation it states, what is wrong with
/// it, and — when nothing is — the write it comes to.
fn read_back(section: Option<&str>, form: &Form) -> (Host, Errors, Option<CommitOp>) {
    let stated = Host::submitted(section.unwrap_or(""), form);
    let errors = validate(&stated);
    let commit = errors.is_empty().then(|| match section {
        Some(section) => commit(CONFIG, section, values(&stated, true)),
        None => commit_new(CONFIG, TYPE, values(&stated, false)),
    });
    (stated, errors, commit)
}

/// remove is the write that takes one reservation away, if the config holds it.
pub fn remove(model: &Dnsdhcp, section: &str) -> Option<CommitOp> {
    model
        .host_index(section)
        .map(|_| commit_delete(CONFIG, section))
}

/// Subject is whose screen a reservation is being edited on, which decides one
/// thing: whether the device is still open to choice.
///
/// In a drawer of its own the reservation is the subject and every value it
/// states is a control, the MAC included — that is how a reservation is pointed
/// at a device in the first place. In a device's panel the device is the
/// subject, and the panel is already headed with it: retyping the MAC there
/// would quietly move the reservation to another device while the heading still
/// named this one, so the MAC rides as a hidden carrier and the panel edits what
/// is genuinely open.
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

/// preview is what the save will put in the file, as it will put it, kept
/// current while the form is edited. Someone who knows uci checks the form said
/// what they meant; everyone else reads the fields and ignores this.
fn preview(section: Option<&str>, host: &Host) -> Widget {
    let stated: Vec<(&str, Value)> = fields(host)
        .into_iter()
        .filter(|(_, value)| !value.is_empty())
        .map(|(option, value)| (option, json!(value)))
        .collect();
    Widget::config_preview(
        &format!("/etc/config/{CONFIG}"),
        &page::uci_block(TYPE, section.unwrap_or(""), &stated),
    )
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

/// The words the reservation tab carries in a device's panel: its label, and the
/// verb its commit row says — reserving an address the first time, saving the
/// reservation after. A save stages like any other, so the act needs nothing
/// said beside it.
const TAB_LABEL: &str = "Reserved address";
const TAB_CTA_NEW: &str = "Reserve address";
const TAB_CTA_EDIT: &str = "Save reservation";

/// entity_tab answers the shell's request for this plugin's say about one
/// device: the reservation it already has, or the one it could be given,
/// prefilled from the lease it is holding right now.
///
/// The shell frames this as one tab of the device's panel beside whatever other
/// plugins had to say. It knows nothing of them, and they know nothing of it.
pub fn entity_tab(model: &Dnsdhcp, leases: &Leases, mac: &str) -> Envelope {
    let (section, host, subject) = entity_subject(model, leases, mac);
    tab(section.as_deref(), &host, subject, &Errors::default())
}

/// entity_save answers the device panel's submission of that tab: it saves into
/// the reservation the device already has, or creates the one it does not —
/// the same form either way, so the panel never asks which it is.
pub fn entity_save(model: &Dnsdhcp, leases: &Leases, mac: &str, form: &Form) -> Envelope {
    let (section, _, subject) = entity_subject(model, leases, mac);
    let (stated, errors, commit) = read_back(section.as_deref(), form);
    let answer = tab(section.as_deref(), &stated, subject, &errors);
    match commit {
        Some(op) => answer.with_commit(vec![op]),
        None => answer,
    }
}

/// entity_subject is the reservation a device's tab is about: the one it holds,
/// or the one it could be given — filled from the lease it is holding, so the
/// address it already has is the one being offered to keep. "new" is a device
/// that does not exist yet, so nothing is settled and the MAC is a control.
fn entity_subject(model: &Dnsdhcp, leases: &Leases, mac: &str) -> (Option<String>, Host, Subject) {
    if mac == "new" {
        return (None, Host::default(), Subject::Reservation);
    }
    if let Some(options) = find(model, mac) {
        let host = Host::read(options);
        return (Some(host.section.clone()), host, Subject::Device);
    }
    let host = leases
        .all()
        .iter()
        .find(|lease| same_mac(&lease.mac, mac))
        .map(Host::of_lease)
        .unwrap_or_else(|| Host {
            mac: mac.to_string(),
            ..Host::default()
        });
    (None, host, Subject::Device)
}

/// tab is the reservation as one tab of a panel: the controls and the preview,
/// and nothing else. The panel's own frame carries the heading, the commit row
/// and the close, and the listing row beside it already carries the removal —
/// so a delete inside here would be the same act offered twice.
fn tab(section: Option<&str>, host: &Host, subject: Subject, errors: &Errors) -> Envelope {
    let body = Widget::stack(vec![
        Widget::Form {
            style: "page".into(),
            submit: String::new(),
            error: String::new(),
            fields: controls(host, errors, subject),
            note: String::new(),
            target: String::new(),
        },
        preview(section, host),
    ]);
    let cta = match section {
        Some(_) => TAB_CTA_EDIT,
        None => TAB_CTA_NEW,
    };
    Envelope::page(TAB_LABEL, body)
        .with_commit_row(cta)
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

    macro_rules! json_of {
        ($value:expr) => {
            serde_json::to_value($value).expect("serialize")
        };
    }

    /// control finds a named control anywhere in a drawer or a tab.
    fn control(tree: &Json, name: &str) -> Json {
        fn walk(v: &Json, name: &str) -> Option<Json> {
            if v["name"] == name && v.get("type").is_some() {
                return Some(v.clone());
            }
            match v {
                Json::Object(m) => m.values().find_map(|c| walk(c, name)),
                Json::Array(a) => a.iter().find_map(|c| walk(c, name)),
                _ => None,
            }
        }
        walk(tree, name).unwrap_or_else(|| panic!("no control {name}"))
    }

    fn rows(model: &Dnsdhcp) -> Vec<Json> {
        json_of!(table(model, None))["rows"]
            .as_array()
            .expect("rows")
            .clone()
    }

    #[test]
    fn every_row_opens_its_own_drawer_and_removes_itself_only_after_asking() {
        let rows = rows(&fixture::dnsdhcp());
        let nas = &rows[0];
        assert_eq!(nas["id"], "host_nas");
        assert_eq!(nas["panel"], "/plugins/dnsdhcp/?open=host_nas");
        assert_eq!(nas["cells"][0]["text"], "nas");
        // The name points at the row's panel. The shell draws it as words and
        // gives the row the edit pencil to that address, since its own act
        // only removes.
        assert_eq!(nas["cells"][0]["href"], nas["panel"]);
        assert_eq!(nas["cells"][1]["chips"][0]["label"], "lan");
        assert_eq!(nas["cells"][2]["text"], "10.0.0.30");
        let remove = &nas["cells"][5]["actions"][0];
        assert_eq!(remove["icon"], "pin-off");
        assert_eq!(remove["name"], REMOVE);
        assert_eq!(remove["value"], "host_nas");
        assert!(remove["confirm_title"].as_str().is_some_and(|t| !t.is_empty()));
        // A listing with nothing open carries no rendered panel.
        assert!(rows.iter().all(|row| row.get("drawer").is_none()));
    }

    #[test]
    fn the_row_an_address_names_arrives_with_its_drawer_open() {
        let model = fixture::dnsdhcp();
        let host = Host::read(find(&model, "host_thermo").expect("host"));
        let open = drawer(Some("host_thermo"), &host, &Errors::default());
        let rows = json_of!(table(&model, Some(("host_thermo".into(), open))))["rows"].clone();
        assert!(rows[0].get("drawer").is_none());
        assert_eq!(rows[1]["drawer"]["open"], true);
        assert_eq!(rows[1]["drawer"]["title"], "thermo-attic");
        assert_eq!(rows[1]["drawer"]["closed"], "/plugins/dnsdhcp/");
    }

    #[test]
    fn a_reservation_is_found_by_its_section_or_by_its_devices_mac() {
        let model = fixture::dnsdhcp();
        assert_eq!(find(&model, "host_nas").unwrap().section, "host_nas");
        assert_eq!(find(&model, "30-9c-23-5e-88-01").unwrap().section, "host_nas");
        assert!(find(&model, "42:e6:ad:ff:b7:af").is_none());
    }

    #[test]
    fn a_blank_drawer_opens_filled_from_the_lease_it_was_reserved_from() {
        let prefilled = json_of!(blank(&fixture::leases(), "42:e6:ad:ff:b7:af"));
        assert_eq!(prefilled["title"], "New reservation");
        assert_eq!(control(&prefilled, "name")["value"], "toms-iphone");
        assert_eq!(control(&prefilled, "mac")["value"], "42:e6:ad:ff:b7:af");
        assert_eq!(control(&prefilled, "ip")["value"], "10.0.0.142");
        // A MAC no lease answers to opens the drawer empty rather than erroring.
        for reserve in ["", "aa:bb:cc:dd:ee:ff", "nonsense"] {
            let empty = json_of!(blank(&fixture::leases(), reserve));
            assert_eq!(control(&empty, "mac")["value"], "", "{reserve}");
        }
    }

    #[test]
    fn the_drawer_says_what_it_writes_while_it_is_edited() {
        let drawer = json_of!(blank(&fixture::leases(), "42:e6:ad:ff:b7:af"));
        let form = &drawer["children"][0];
        assert_eq!(form["submit"], "Save reservation");
        let preview = form["fields"]
            .as_array()
            .unwrap()
            .iter()
            .find(|f| f["type"] == "code")
            .expect("preview");
        assert_eq!(preview["live"], true);
        assert_eq!(preview["label"], "/etc/config/dhcp");
        assert!(preview["value"]
            .as_str()
            .unwrap()
            .contains("option mac '42:e6:ad:ff:b7:af'"));
    }

    #[test]
    fn a_new_reservation_states_its_section_and_an_edit_clears_what_it_dropped() {
        let made = submit(
            None,
            &Form::parse("name=toms-iphone&mac=42:e6:ad:ff:b7:af&ip=10.0.0.142"),
        );
        assert_eq!(
            json_of!(made.commit.expect("staged")),
            serde_json::json!({
                "config": "dhcp", "section": "", "type": "host",
                "values": {"name": "toms-iphone", "mac": "42:e6:ad:ff:b7:af", "ip": "10.0.0.142"}
            })
        );
        let edited = submit(
            Some("host_nas"),
            &Form::parse("name=nas&mac=30:9C:23:5E:88:01&ip=10.0.0.30"),
        );
        assert_eq!(
            json_of!(edited.commit.expect("staged"))["values"],
            serde_json::json!({
                "name": "nas", "mac": "30:9C:23:5E:88:01", "ip": "10.0.0.30",
                "hostid": null, "duid": null, "leasetime": null, "tag": null
            })
        );
    }

    #[test]
    fn a_reservation_the_daemons_would_skip_stays_in_its_drawer_and_writes_nothing() {
        for (body, field) in [
            ("mac=not-a-mac&ip=10.0.0.9", "mac"),
            ("mac=30:9c:23:5e:88:01&ip=10.0.0.256", "ip"),
            ("mac=30:9c:23:5e:88:01&ip=10.0.0.9&name=not+a+name", "name"),
            ("mac=30:9c:23:5e:88:01&ip=10.0.0.9&hostid=::zz", "hostid"),
            ("mac=30:9c:23:5e:88:01&ip=10.0.0.9&leasetime=forever", "leasetime"),
            // A reservation that matches a device and then hands it nothing.
            ("mac=30:9c:23:5e:88:01", "ip"),
        ] {
            let refused = submit(None, &Form::parse(body));
            assert!(refused.commit.is_none(), "{body}");
            let drawer = json_of!(refused.drawer);
            assert!(
                !control(&drawer, field)["error"].as_str().unwrap_or("").is_empty(),
                "{body}: {field} carries no error"
            );
        }
        // What was typed comes back as typed.
        let refused = json_of!(submit(Some("host_nas"), &Form::parse("mac=nope&ip=10.0.0.30")).drawer);
        assert_eq!(control(&refused, "mac")["value"], "nope");
    }

    #[test]
    fn removing_names_the_section_and_a_stale_one_removes_nothing() {
        let model = fixture::dnsdhcp();
        assert_eq!(
            json_of!(remove(&model, "host_thermo").expect("removal")),
            serde_json::json!({"config": "dhcp", "section": "host_thermo", "delete": true})
        );
        assert!(remove(&model, "no_such_host").is_none());
    }

    #[test]
    fn a_devices_panel_edits_only_what_is_still_open_to_choice() {
        let model = fixture::dnsdhcp();
        let leases = fixture::leases();

        // The panel is headed with the device, so its MAC is settled: it rides
        // as a hidden carrier the save still posts.
        let held = json_of!(entity_tab(&model, &leases, "30:9C:23:5E:88:01"));
        assert_eq!(held["title"], "Reserved address");
        assert_eq!(held["cta"], "Save reservation");
        assert_eq!(held["state"], "10.0.0.30");
        assert_eq!(control(&held, "mac")["kind"], "hidden");
        // Removing is the listing row's act, so the tab is the controls and the
        // preview.
        assert_eq!(held["widget"]["children"].as_array().unwrap().len(), 2);

        // A device holding a lease and no reservation opens on that lease.
        let fresh = json_of!(entity_tab(&model, &leases, "42:e6:ad:ff:b7:af"));
        assert_eq!(fresh["cta"], "Reserve address");
        assert_eq!(control(&fresh, "ip")["value"], "10.0.0.142");
        assert_eq!(control(&fresh, "mac")["kind"], "hidden");

        // No device at all: nothing is settled, so the MAC is a control again.
        let blank = json_of!(entity_tab(&model, &leases, "new"));
        assert_eq!(control(&blank, "mac")["kind"], "text");
    }

    #[test]
    fn a_devices_panel_saves_into_the_reservation_it_has_or_makes_one() {
        let model = fixture::dnsdhcp();
        let leases = fixture::leases();
        let saved = json_of!(entity_save(
            &model,
            &leases,
            "30:9C:23:5E:88:01",
            &Form::parse("name=nas&mac=30:9C:23:5E:88:01&ip=10.0.0.31"),
        ));
        assert_eq!(saved["commit"][0]["section"], "host_nas");
        let made = json_of!(entity_save(
            &model,
            &leases,
            "42:e6:ad:ff:b7:af",
            &Form::parse("name=toms-iphone&mac=42:e6:ad:ff:b7:af&ip=10.0.0.142"),
        ));
        assert_eq!(made["commit"][0]["type"], "host");
        let refused = json_of!(entity_save(
            &model,
            &leases,
            "42:e6:ad:ff:b7:af",
            &Form::parse("mac=42:e6:ad:ff:b7:af&ip=nope"),
        ));
        assert!(refused.get("commit").is_none());
        assert!(!control(&refused, "ip")["error"].as_str().unwrap_or("").is_empty());
    }

    #[test]
    fn the_labels_that_are_terms_of_art_explain_themselves() {
        let model = fixture::dnsdhcp();
        let nas = Host::read(find(&model, "host_nas").unwrap());
        let drawer = json_of!(drawer(Some("host_nas"), &nas, &Errors::default()));
        for name in ["ip", "hostid", "duid"] {
            let field = control(&drawer, name);
            assert!(field["tip"].as_str().is_some_and(|tip| !tip.is_empty()), "{name}");
            assert_eq!(field["source"], "dhcp host", "{name}");
        }
        for name in ["name", "leasetime", "tag"] {
            assert!(control(&drawer, name).get("tip").is_none(), "{name}");
        }
    }
}
