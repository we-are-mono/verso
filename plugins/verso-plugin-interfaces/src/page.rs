// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.
use crate::editor::url;
use crate::status::{addresses, speed};
use crate::{Model, ROOT};
use serde_json::Value;
use std::collections::{BTreeMap, BTreeSet};
use verso_plugin::{
    pill_cell, Envelope, HeadingAct, RowDrawer, SectionWidget, Table, TableCell, TableChip,
    TableColumn, TableRow, TableRowAct, Tone, Widget, EM_DASH,
};

fn text(v: Option<&Value>) -> String {
    v.and_then(Value::as_str).unwrap_or("").into()
}
fn flag(v: Option<&Value>) -> Option<bool> {
    v.and_then(Value::as_bool)
}
fn cell(s: impl Into<String>) -> TableCell {
    TableCell {
        text: s.into(),
        ..Default::default()
    }
}
// glyph is the mark of a device's kind, the same one the chooser makes it
// with: a bridge, a VLAN, a tunnel or a physical port, else a device defined
// in software.
pub(crate) fn glyph(kind: &str) -> &'static str {
    match kind {
        "bridge" => "git-merge",
        k if k.starts_with("vlan") => "tag",
        k if k.starts_with("port") => "ethernet-port",
        "pppoe" | "tunnel" | "sit" | "ip6tnl" | "gre" | "gretap" | "vxlan" | "wireguard" => {
            "git-branch"
        }
        _ => "cpu",
    }
}
// kind is what a device is, as the config makes it or the kernel reports it.
pub(crate) fn kind(m: &Model, name: &str, runtime: Option<&Value>) -> String {
    if let Some(d) = m.device(name) {
        let typ = d.get("type");
        if matches!(typ.as_str(), "8021q" | "8021ad") {
            return format!("vlan {}", d.get("vid"));
        }
        if !typ.is_empty() {
            return typ;
        }
    }
    let runtime_kind = text(runtime.and_then(|v| v.get("kind")));
    let devtype = text(runtime.and_then(|v| v.get("devtype")));
    let typ = text(runtime.and_then(|v| v.get("type")));
    if typ == "bridge" || devtype == "bridge" || runtime_kind == "bridge" {
        return "bridge".into();
    }
    if name.starts_with("pppoe-") {
        return "pppoe".into();
    }
    if let Some((_, vid)) = name.rsplit_once('.') {
        if vid.parse::<u16>().is_ok() {
            return format!("vlan {vid}");
        }
    }
    if !runtime_kind.is_empty() {
        return runtime_kind;
    }
    if !devtype.is_empty() {
        return devtype;
    }
    if !typ.is_empty() && typ != "Network device" {
        return typ.to_lowercase();
    }
    EM_DASH.into()
}
pub(crate) fn state(
    m: &Model,
    nets: &[String],
    runtime: Option<&Value>,
    used: bool,
) -> (&'static str, &'static str) {
    let live: Vec<_> = nets.iter().filter_map(|n| m.live_network(n)).collect();
    if !nets.is_empty() && live.is_empty() && m.live.get("interfaces").is_some() {
        return ("not active", "neutral");
    }
    let carrier = flag(runtime.and_then(|v| v.get("carrier")));
    let logical_up: Vec<_> = live.iter().filter_map(|v| flag(v.get("up"))).collect();
    if !nets.is_empty() && live.iter().any(|v| flag(v.get("pending")) == Some(true)) {
        return ("pending", "warning");
    }
    if !nets.is_empty()
        && live.iter().any(|v| {
            v.get("errors")
                .and_then(Value::as_array)
                .is_some_and(|e| !e.is_empty())
        })
    {
        return ("error", "danger");
    }
    if flag(runtime.and_then(|v| v.get("present"))) == Some(false)
        || (!live.is_empty() && live.iter().all(|v| flag(v.get("available")) == Some(false)))
    {
        return ("unavailable", "danger");
    }
    if carrier == Some(false) && used {
        return ("no link", "danger");
    }
    if !logical_up.is_empty() {
        if logical_up.iter().all(|up| *up) {
            return ("up", "success");
        }
        if logical_up.iter().any(|up| *up) {
            return ("partial", "warning");
        }
        return ("down", "neutral");
    }
    match flag(runtime.and_then(|v| v.get("up"))) {
        Some(true) if carrier != Some(false) => ("up", "success"),
        Some(false) | Some(true) if !used => ("unused", "neutral"),
        Some(false) => ("down", "neutral"),
        _ => ("not reported", "neutral"),
    }
}
// listing is the inventory alone. Its act names a new interface's address, and
// the shell fetches the drawer from there, so the drawer's form posts back to
// the address that makes one.
pub fn listing(m: &Model) -> Envelope {
    inventory(m, None)
}
// with_drawer is the inventory with a panel open on its bar: a new
// interface's, or an object's editor (editor.rs), so that the editor's address
// shows the listing with the drawer open, and a request for the panel alone is
// answered from the same tree.
pub fn with_drawer(m: &Model, drawer: RowDrawer) -> Envelope {
    inventory(m, Some(drawer))
}

/// A network row's four act slots, in the order every listing's acts stand:
/// the state first, then what acts on it, then the way in, and the removal
/// last. A slot a row has no act for holds its place.
const POWER: usize = 0;
const RESTART: usize = 1;
const EDIT: usize = 2;
const DELETE: usize = 3;

// Rows is what the listing is drawn from: each row's device, each followed by
// what it carries, the networks each device carries, and each device's parent.
struct Rows {
    ordered: Vec<String>,
    owners: BTreeMap<String, Vec<String>>,
    parents: BTreeMap<String, String>,
}

fn rows(m: &Model) -> Rows {
    let parents = m.parents();
    let mut names = m.names();
    names.extend(parents.keys().cloned());
    names.extend(parents.values().cloned());
    let mut owners: BTreeMap<String, Vec<String>> = BTreeMap::new();
    let mut networks: BTreeSet<String> = m.networks.iter().map(|n| n.id.clone()).collect();
    if let Some(live) = m.live.get("interfaces").and_then(Value::as_array) {
        networks.extend(
            live.iter()
                .filter_map(|n| n.get("interface"))
                .filter_map(Value::as_str)
                .map(String::from),
        );
    }
    for net in networks {
        if net == "loopback" {
            continue;
        }
        let device = l3(m, &net);
        names.insert(device.clone());
        owners.entry(device).or_default().push(net);
    }
    names.remove("lo");
    let mut ordered = vec![];
    let mut seen = BTreeSet::new();
    fn visit(
        name: &str,
        names: &BTreeSet<String>,
        parents: &BTreeMap<String, String>,
        seen: &mut BTreeSet<String>,
        out: &mut Vec<String>,
    ) {
        if !seen.insert(name.into()) {
            return;
        }
        out.push(name.into());
        for child in names {
            if parents.get(child).is_some_and(|p| p == name) {
                visit(child, names, parents, seen, out);
            }
        }
    }
    for name in &names {
        if parents.get(name).is_none_or(|p| !names.contains(p)) {
            visit(name, &names, &parents, &mut seen, &mut ordered);
        }
    }
    for name in &names {
        visit(name, &names, &parents, &mut seen, &mut ordered);
    }
    Rows {
        ordered,
        owners,
        parents,
    }
}

// act_at is a row act that leads to an address.
fn act_at(icon: &str, title: &str, href: String) -> TableRowAct {
    TableRowAct {
        icon: icon.into(),
        title: title.into(),
        href,
        ..Default::default()
    }
}
// listed_networks are the Networks card's rows: each network in the order the
// config holds them, then any netifd alone reports, loopback and IPv6
// companions left out (a companion is its network's IPv6 card).
fn listed_networks(m: &Model) -> Vec<String> {
    let mut out: Vec<String> = m.networks.iter().map(|n| n.id.clone()).collect();
    if let Some(live) = m.live.get("interfaces").and_then(Value::as_array) {
        for net in live
            .iter()
            .filter_map(|n| n.get("interface"))
            .filter_map(Value::as_str)
        {
            if !out.iter().any(|o| o == net) {
                out.push(net.into());
            }
        }
    }
    out.retain(|n| n != "loopback" && m.companion_of(n).is_none());
    out
}
// runs_on is the device a network names, as the operator chose it; one riding
// another network, or known only to netifd, by the device it came up on.
fn runs_on(m: &Model, net: &str) -> String {
    let configured = m.network(net).map(|n| n.get("device")).unwrap_or_default();
    if configured.is_empty() || configured.starts_with('@') {
        l3(m, net)
    } else {
        configured
    }
}
// l3 is the device netifd brings a network up on.
pub(crate) fn l3(m: &Model, net: &str) -> String {
    let live = m.live_network(net);
    [
        text(live.and_then(|v| v.get("l3_device"))),
        text(live.and_then(|v| v.get("device"))),
        m.network(net)
            .map(|n| m.resolve_device(&n.get("device")))
            .unwrap_or_default(),
    ]
    .into_iter()
    .find(|d| !d.is_empty())
    .unwrap_or_else(|| net.into())
}
fn columns(spec: &[(&str, &str)]) -> Vec<TableColumn> {
    spec.iter()
        .map(|(label, kind)| TableColumn {
            label: (*label).into(),
            kind: (*kind).into(),
            ..TableColumn::default()
        })
        .collect()
}
// network_row is one network: its name with its DHCP chip, how it takes its
// address, the address observed, the device it runs on, its firewall zone and
// its state, which counts its IPv6 companion's.
fn network_row(m: &Model, net: &str) -> TableRow {
    let live = m.live_network(net);
    let configured = m.network(net).is_some();
    let mut nets = vec![net.to_string()];
    nets.extend(m.companion(net).map(|c| c.id.clone()));
    let runtime = m.live.get("devices").and_then(|v| v.get(l3(m, net)));
    let (state, tone) = state(m, &nets, runtime, true);
    // Only observed addresses belong in this column: a configured one is
    // the drawer's until Apply makes it real.
    let ipv4 = addresses(live, "ipv4-address");
    let protocol = m
        .network(net)
        .map(|n| n.get("proto"))
        .filter(|p| !p.is_empty())
        .unwrap_or_else(|| text(live.and_then(|v| v.get("proto"))));
    let zones: Vec<_> = m
        .zones
        .iter()
        .filter(|z| z.list("network").iter().any(|n| n == net))
        .map(|z| z.get("name"))
        .collect();
    let mut actions = (0..4).map(|_| TableRowAct::default()).collect::<Vec<_>>();
    if configured && live.is_some() {
        let up = flag(live.and_then(|v| v.get("up"))) == Some(true);
        actions[POWER] = TableRowAct {
            icon: "power".into(),
            title: if up { "Bring down" } else { "Bring up" }.into(),
            name: if up { "down" } else { "up" }.into(),
            value: net.into(),
            ..Default::default()
        };
        actions[RESTART] = TableRowAct {
            icon: "refresh-cw".into(),
            title: "Restart".into(),
            name: "restart".into(),
            value: net.into(),
            ..Default::default()
        };
    }
    if configured {
        actions[EDIT] = act_at("square-pen", "Edit", url(net, "", "edit"));
        actions[DELETE] = act_at("trash-2", "Delete", url(net, "", "delete"));
    }
    let mut chips = vec![];
    if let Some(server) = m
        .dhcp_servers
        .iter()
        .find(|s| s.network == net && s.exists())
    {
        chips.push(TableChip {
            label: "DHCP".into(),
            tone: server.tone().into(),
            title: server.title().into(),
            ..Default::default()
        });
    }
    let or_dash = |s: String| if s.is_empty() { EM_DASH.into() } else { s };
    TableRow {
        id: net.into(),
        // The row's drawer is its panel: the pencil, whose address this is,
        // opens it in place rather than leaving the listing.
        panel: actions[EDIT].href.clone(),
        cells: vec![
            TableCell { chips, ..cell(net) },
            cell(or_dash(protocol)),
            TableCell {
                copy: !ipv4.is_empty(),
                ..cell(ipv4.first().cloned().unwrap_or_else(|| EM_DASH.into()))
            },
            cell(runs_on(m, net)),
            cell(or_dash(zones.join(", "))),
            pill_cell(state, tone),
            TableCell {
                actions,
                ..Default::default()
            },
        ],
        ..Default::default()
    }
}
// used_by is what rests on a device: the networks it carries (a companion
// read as its network), the bridges it is a port of, and the devices made on
// it — a VLAN, a PPPoE session.
fn used_by(
    m: &Model,
    name: &str,
    nets: &[String],
    parents: &BTreeMap<String, String>,
) -> Vec<TableChip> {
    let mut chips: Vec<TableChip> = nets
        .iter()
        .filter(|n| m.companion_of(n).is_none())
        .map(|n| TableChip::entity("network", n))
        .collect();
    let mut names: BTreeSet<String> = m.names();
    names.extend(m.devices.iter().map(|d| d.get("name")));
    for bridge in &names {
        if bridge != name && m.bridge_ports(bridge).contains(&name.to_string()) {
            chips.push(TableChip {
                label: bridge.clone(),
                icon: "git-merge".into(),
                ..Default::default()
            });
        }
    }
    let ports = m.bridge_ports(name);
    for (child, _) in parents
        .iter()
        .filter(|(c, p)| *p == name && !ports.contains(c))
    {
        let runtime = m.live.get("devices").and_then(|v| v.get(child));
        chips.push(TableChip {
            label: child.clone(),
            icon: glyph(&kind(m, child, runtime)).into(),
            ..Default::default()
        });
    }
    chips
}
// device_row is one device: its name, its kind (a port's link speed after
// it), what rests on it and its state.
fn device_row(m: &Model, name: &str, chips: Vec<TableChip>, nets: &[String]) -> TableRow {
    let runtime = m.live.get("devices").and_then(|v| v.get(name));
    let configured = m.device(name).is_some();
    let kind = kind(m, name, runtime);
    let used = !chips.is_empty()
        || m.devices
            .iter()
            .any(|d| d.list("ports").iter().any(|p| p == name));
    let (state, tone) = state(m, nets, runtime, used);
    let mut actions = vec![TableRowAct::default(), TableRowAct::default()];
    if runtime.is_some() || configured {
        actions[0] = act_at("square-pen", "Edit", url("", name, "edit"));
    }
    if configured && !used {
        actions[1] = act_at("trash-2", "Delete", url("", name, "delete"));
    }
    TableRow {
        id: name.into(),
        panel: actions[0].href.clone(),
        cells: vec![
            cell(name),
            TableCell {
                rate: if kind == "port" { speed(runtime) } else { None },
                ..cell(kind)
            },
            TableCell { chips, ..cell("") },
            pill_cell(state, tone),
            TableCell {
                actions,
                ..Default::default()
            },
        ],
        ..Default::default()
    }
}
// listed says a device earns a row: the config names it, a network rests on
// it, something else uses it, or it is one of the router's own ports. A
// software device the kernel made on its own and nothing uses (sit0,
// ip6tnl0) has none.
fn listed(m: &Model, name: &str, nets: &[String], chips: &[TableChip]) -> bool {
    let runtime = &m.live["devices"][name];
    m.device(name).is_some()
        || !nets.is_empty()
        || !chips.is_empty()
        || m.networks
            .iter()
            .any(|n| m.resolve_device(&n.get("device")) == name)
        || m.devices
            .iter()
            .any(|d| d.list("ports").iter().any(|p| p == name))
        || runtime["physical"].as_bool() == Some(true)
        || runtime["kind"] == "port"
}
// card is one inventory on the page: a titled section holding its table,
// live, so the shell refreshes its rows as the router changes.
fn card(title: &str, table: Table) -> Widget {
    Widget::Section(SectionWidget {
        title: title.into(),
        hairline: true,
        children: vec![Widget::Table(Table {
            style: "live".into(),
            ..table
        })],
        ..Default::default()
    })
}
fn inventory(m: &Model, drawer: Option<RowDrawer>) -> Envelope {
    let Rows {
        ordered,
        owners,
        parents,
    } = rows(m);
    let networks = Table {
        columns: columns(&[
            ("Network", "reference"),
            ("Protocol", "keyword"),
            ("Address", "mono"),
            ("Device", "mono"),
            ("Zone", "keyword"),
            ("State", "status"),
            ("", "actions"),
        ]),
        rows: listed_networks(m)
            .iter()
            .map(|net| network_row(m, net))
            .collect(),
        empty_text: "No networks are configured or reported by the router.".into(),
        ..Default::default()
    };
    let devices = Table {
        columns: columns(&[
            ("Device", "mono"),
            ("Type", "keyword"),
            ("Used by", "reference"),
            ("State", "status"),
            ("", "actions"),
        ]),
        rows: ordered
            .iter()
            .filter_map(|name| {
                let nets = owners.get(name).cloned().unwrap_or_default();
                let chips = used_by(m, name, &nets, &parents);
                listed(m, name, &nets, &chips).then(|| device_row(m, name, chips, &nets))
            })
            .collect(),
        empty_text: "The router reports no devices.".into(),
        ..Default::default()
    };
    // The page's one act, on its heading line. It carries the panel open over
    // the listing — a new interface's, or an object's editor — so an address
    // naming either arrives with it in front of the operator; with none open,
    // the drawer is fetched from the act's own address.
    let act = HeadingAct {
        label: "Add interface".into(),
        href: format!("{ROOT}new"),
        icon: "plus".into(),
        opens_panel: true,
        drawer,
        ..Default::default()
    };
    let page = Envelope::page(
        "Interfaces",
        Widget::stack(vec![card("Networks", networks), card("Devices", devices)]),
    )
    .with_act(act)
    .with_width("wide");
    if m.live.get("interfaces").is_none() {
        page.with_notice(
            Tone::Warning,
            "Live status could not be read. Configured interfaces are still listed.",
        )
    } else {
        page
    }
}
