// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

//! The Routes page: every route the router has, as one listing. The custom
//! ones — `config route` and `config route6` in /etc/config/network — are each
//! edited in their own drawer (ADR-005 §8): the row opens it, New opens it
//! blank, and one set of controls draws both. The rest are the kernel's main
//! table, read live. The address names the open drawer (`?open=`).

use crate::editor::{mask_bits, uint};
use crate::model::Record;
use crate::{Model, ROOT};
use serde_json::{json, Map, Value};
use std::cmp::Reverse;
use std::collections::BTreeMap;
use std::net::{IpAddr, Ipv4Addr};
use verso_plugin::{
    commit, commit_delete, commit_new, uci_text, Change, ColumnWidth, CommitOp, Description,
    Envelope, Errors, Form, HeadingAct, RowDrawer, SelectOption, Snapshot, Table, TableCell,
    TableChip, TableColumn, TableGroup, TableRow, TableRowAct, Tone, Widget,
};

const NEW: &str = "new";
const DELETE: &str = "_delete";
/// TEXT is every option the form writes as typed; `disabled` and `onlink` are
/// its two switches, and `netmask` rides `target` where the section states one.
const TEXT: [&str; 9] = [
    "interface",
    "target",
    "gateway",
    "metric",
    "table",
    "source",
    "mtu",
    "type",
    "proto",
];
const TYPES: [(&str, &str); 7] = [
    ("local", "Local"),
    ("broadcast", "Broadcast"),
    ("multicast", "Multicast"),
    ("unreachable", "Unreachable"),
    ("prohibit", "Prohibit"),
    ("blackhole", "Blackhole"),
    ("anycast", "Anycast"),
];
const REFUSED: &str =
    "Some values aren’t ones netifd accepts, so nothing was saved. They’re marked below.";

fn listing_href() -> String {
    format!("{ROOT}routes")
}
fn open_href(key: &str) -> String {
    format!("{ROOT}routes?open={key}")
}

/// Route is one route as the form holds it.
#[derive(Default)]
struct Route {
    v6: bool,
    text: BTreeMap<&'static str, String>,
    enabled: bool,
    onlink: bool,
}
impl Route {
    fn read(r: &Record) -> Route {
        let mut text: BTreeMap<_, _> = TEXT.iter().map(|k| (*k, r.get(k))).collect();
        text.insert("target", target(&r.get("target"), &r.get("netmask")));
        Route {
            v6: r.kind == "route6",
            text,
            enabled: r.get("disabled") != "1",
            onlink: r.get("onlink") == "1",
        }
    }
    fn posted(v6: bool, f: &Form) -> Route {
        Route {
            v6,
            text: TEXT
                .iter()
                .map(|k| (*k, f.get(k).trim().to_string()))
                .collect(),
            enabled: f.get("enabled") == "1",
            onlink: f.get("onlink") == "1",
        }
    }
    fn get(&self, k: &str) -> &str {
        self.text.get(k).map(String::as_str).unwrap_or("")
    }
}

/// target is a route's destination as a person writes it: CIDR, with a
/// `netmask` the section states folded into the prefix length.
fn target(target: &str, netmask: &str) -> String {
    match (netmask.is_empty(), mask_bits(netmask)) {
        (true, _) => target.into(),
        (false, Some(bits)) => format!("{target}/{bits}"),
        (false, None) => format!("{target}/{netmask}"),
    }
}

/// page is the listing with the drawer `open` names in front of it: a route by
/// its section, `new` for a blank one of `family` (a lane's add), anything
/// else nothing.
pub fn page(m: &Model, open: &str, family: &str) -> Envelope {
    let drawer = match open {
        NEW => Some(drawer(
            m,
            None,
            &Route {
                v6: family == "ipv6",
                enabled: true,
                ..Route::default()
            },
            &Errors::default(),
        )),
        key => m
            .routes
            .iter()
            .find(|r| r.id == key)
            .map(|r| drawer(m, Some(r), &Route::read(r), &Errors::default())),
    };
    listing(m, open, drawer)
}

/// post answers the open drawer: its save, or its confirmed delete.
pub fn post(m: &Model, open: &str, f: &Form) -> Envelope {
    let existing = m.routes.iter().find(|r| r.id == open);
    if open != NEW && existing.is_none() {
        return listing(m, "", None).with_notice(
            Tone::Danger,
            "This route no longer exists, so nothing was saved.",
        );
    }
    if let (Some(r), "1") = (existing, f.get(DELETE).as_str()) {
        return listing(m, "", None)
            .with_commit(vec![commit_delete("network", &r.id)])
            .with_notice(Tone::Success, "Route removed.");
    }
    let family = f.get("family");
    let v6 = match existing {
        Some(r) => r.kind == "route6",
        None => family == "ipv6",
    };
    let route = Route::posted(v6, f);
    let mut errors = validate(m, &route);
    if existing.is_none() {
        errors.check(
            "family",
            matches!(family.as_str(), "ipv4" | "ipv6"),
            "Choose IPv4 or IPv6.",
        );
    }
    let answer = listing(m, open, Some(drawer(m, existing, &route, &errors)));
    if !errors.is_empty() {
        return answer.with_notice(Tone::Danger, REFUSED);
    }
    answer.with_commit(vec![write(existing, &route)])
}

fn listing(m: &Model, open: &str, drawer: Option<RowDrawer>) -> Envelope {
    let mut drawer = drawer;
    let blank = if open == NEW { drawer.take() } else { None };
    let kernel = kernel(m);
    let mut lines: Vec<Line> = m
        .routes
        .iter()
        .map(|r| Line::custom(r, kernel.as_deref()))
        .collect();
    // A kernel route that a custom route states is that route, listed once.
    let automatic: Vec<Line> = kernel
        .iter()
        .flatten()
        .filter(|k| {
            !lines
                .iter()
                .any(|c| c.main && c.v6 == k.v6 && c.network == k.network && c.gateway == k.gateway)
        })
        .cloned()
        .collect();
    lines.extend(automatic);
    lines.sort_by_key(|l| {
        (
            l.v6,
            Reverse(l.network.map(|(_, len)| len)),
            l.network.map(|(addr, _)| addr),
            l.metric.parse::<u64>().unwrap_or(0),
        )
    });
    let mut rows: Vec<TableRow> = vec![];
    for (i, line) in lines.iter().enumerate() {
        let open_here = line.custom.is_some_and(|r| r.id == open);
        let mut row = row(line, if open_here { drawer.take() } else { None });
        if i == 0 || lines[i - 1].v6 != line.v6 {
            row.group = Some(lane(&lines, line.v6));
        }
        rows.push(row);
    }
    // The page's one act, on its heading line, carrying the blank route's
    // panel. The lanes and the origin chips already say what each route is,
    // and a still listing is narrowed with the browser's own find.
    let act = HeadingAct {
        label: "New route".into(),
        href: open_href(NEW),
        icon: "plus".into(),
        opens_panel: true,
        drawer: blank,
        ..Default::default()
    };
    let table = Widget::Table(Table {
        columns: [
            ("Target", "reference", ColumnWidth::Address),
            ("Gateway", "mono", ColumnWidth::Address),
            ("Interface", "entity", ColumnWidth::Short),
            ("Metric", "num", ColumnWidth::Count),
            ("Origin", "entity", ColumnWidth::Word),
            ("", "actions", ColumnWidth::Short),
        ]
        .into_iter()
        .map(|(label, kind, width)| TableColumn {
            label: label.into(),
            kind: kind.into(),
            width,
        })
        .collect(),
        rows,
        empty_text: "No routes are configured or reported by the router.".into(),
        ..Default::default()
    });
    let page = Envelope::page("Routes", table)
        .with_act(act)
        .with_width("wide");
    match kernel {
        Some(_) => page,
        None => page.with_notice(
            Tone::Warning,
            "Live routes could not be read. Custom routes are still listed.",
        ),
    }
}

/// Line is one route of the listing: a custom one with the section behind it,
/// or one of the kernel's own, named by the protocol that added it.
#[derive(Clone)]
struct Line<'a> {
    v6: bool,
    network: Option<(IpAddr, u32)>,
    target: String,
    gateway: String,
    kind: String,
    interface: String,
    metric: String,
    origin: String,
    custom: Option<&'a Record>,
    /// Whether the route lives in the main table, the one the kernel lists.
    main: bool,
    /// Where a custom route stands when it is not simply in effect: "off", or
    /// "not active" when the router's table lacks it.
    state: &'static str,
}
impl<'a> Line<'a> {
    fn custom(r: &'a Record, kernel: Option<&[Line]>) -> Line<'a> {
        let route = Route::read(r);
        let network = network(route.get("target"), route.v6);
        let main = matches!(route.get("table"), "" | "main" | "254");
        let installed = |k: &[Line]| {
            k.iter().any(|l| {
                l.v6 == route.v6 && l.network == network && l.gateway == route.get("gateway")
            })
        };
        Line {
            v6: route.v6,
            network,
            target: route.get("target").into(),
            gateway: route.get("gateway").into(),
            kind: route.get("type").into(),
            interface: route.get("interface").into(),
            metric: route.get("metric").into(),
            origin: "config".into(),
            custom: Some(r),
            main,
            state: if !route.enabled {
                "off"
            } else if main && network.is_some() && kernel.is_some_and(|k| !installed(k)) {
                "not active"
            } else {
                ""
            },
        }
    }
}

/// kernel is the router's main table as listing lines, each cited by the
/// interface that owns its device; None when the live read is missing.
fn kernel(m: &Model) -> Option<Vec<Line<'static>>> {
    let routes = m.live.get("routes")?.as_array()?;
    let owner = |device: &str| {
        m.live
            .get("interfaces")
            .and_then(Value::as_array)
            .into_iter()
            .flatten()
            .find(|n| n.get("l3_device").and_then(Value::as_str) == Some(device))
            .and_then(|n| n.get("interface").and_then(Value::as_str))
            .unwrap_or(device)
            .to_string()
    };
    Some(
        routes
            .iter()
            .filter_map(|live| {
                let text = |k: &str| live.get(k).and_then(Value::as_str).unwrap_or("");
                let v6 = live.get("family").and_then(Value::as_u64) == Some(6);
                Some(Line {
                    v6,
                    network: Some(network(text("target"), v6)?),
                    target: text("target").into(),
                    gateway: text("gateway").into(),
                    kind: text("type").into(),
                    interface: owner(text("device")),
                    metric: live
                        .get("metric")
                        .and_then(Value::as_u64)
                        .unwrap_or(0)
                        .to_string(),
                    origin: text("proto").into(),
                    custom: None,
                    main: true,
                    state: "",
                })
            })
            .collect(),
    )
}

/// lane heads one family's run: what it holds, and an add that opens New route
/// with the family already chosen.
fn lane(lines: &[Line], v6: bool) -> TableGroup {
    let family = if v6 { "IPv6" } else { "IPv4" };
    let ours: Vec<_> = lines.iter().filter(|l| l.v6 == v6).collect();
    let custom = ours.iter().filter(|l| l.custom.is_some()).count();
    let mut tally = match ours.len() {
        1 => "1 route".to_string(),
        n => format!("{n} routes"),
    };
    if custom > 0 {
        tally.push_str(&format!(" · {custom} custom"));
    }
    TableGroup {
        key: family.to_lowercase(),
        label: family.into(),
        to: String::new(),
        chain: String::new(),
        tally,
        add_label: if v6 {
            "Add IPv6 route"
        } else {
            "Add IPv4 route"
        }
        .into(),
        add_text: "Add route".into(),
        add_href: format!("{}&family={}", open_href(NEW), family.to_lowercase()),
        add_panel: true,
    }
}

fn quiet(text: &str) -> TableCell {
    TableCell {
        text: text.into(),
        muted: true,
        ..Default::default()
    }
}
/// gateway_cell is where the traffic goes next: the gateway, else straight out
/// of the interface, else what a route that refuses traffic does with it.
fn gateway_cell(gateway: &str, kind: &str) -> TableCell {
    match (gateway, kind) {
        ("", "" | "unicast") => quiet("On-link"),
        ("", kind) => quiet(kind),
        (gw, _) => TableCell {
            text: gw.into(),
            copy: true,
            ..Default::default()
        },
    }
}
fn interface_cell(name: &str) -> TableCell {
    if name.is_empty() {
        return quiet("—");
    }
    TableCell {
        chips: vec![TableChip {
            icon: "network".into(),
            label: name.into(),
            tone: "accent".into(),
            ..Default::default()
        }],
        ..Default::default()
    }
}
fn metric_cell(metric: &str) -> TableCell {
    match metric {
        "" => quiet("—"),
        metric => TableCell {
            text: metric.into(),
            ..Default::default()
        },
    }
}

/// row draws one line. A custom route's target is the door to its drawer, and
/// only a route that is not in effect says anything about its state.
fn row(line: &Line, drawer: Option<RowDrawer>) -> TableRow {
    let door = line.custom.map(|r| open_href(&r.id)).unwrap_or_default();
    let mut target = TableCell {
        text: match line.network {
            Some((_, 0)) => "default".into(),
            _ => line.target.clone(),
        },
        href: door.clone(),
        copy: true,
        ..Default::default()
    };
    if !line.state.is_empty() {
        target.tag = line.state.into();
        target.tag_dot = true;
        if line.state == "not active" {
            target.tag_variant = "warning".into();
        }
    }
    let edit = line.custom.map(|_| TableRowAct {
        icon: "square-pen".into(),
        title: "Edit".into(),
        href: door.clone(),
        ..Default::default()
    });
    TableRow {
        id: line.custom.map(|r| r.id.clone()).unwrap_or_default(),
        muted: line.state == "off",
        cells: vec![
            target,
            gateway_cell(&line.gateway, &line.kind),
            interface_cell(&line.interface),
            metric_cell(&line.metric),
            TableCell {
                chips: vec![TableChip {
                    label: line.origin.clone(),
                    tone: if line.custom.is_some() { "accent" } else { "" }.into(),
                    ..Default::default()
                }],
                ..Default::default()
            },
            TableCell {
                actions: edit.into_iter().collect(),
                ..Default::default()
            },
        ],
        drawer,
        panel: door,
        ..Default::default()
    }
}

fn field(route: &Route, e: &Errors, key: &str, label: &str, help: &str) -> Widget {
    let mut w = Widget::field(key, label, route.get(key), "", help).writes(key);
    if let Widget::Field(f) = &mut w {
        f.error = e.get(key).into();
    }
    w
}
fn select(route: &Route, e: &Errors, key: &str, label: &str, options: Vec<SelectOption>) -> Widget {
    Widget::select(key, label, route.get(key), options, e.get(key)).writes(key)
}

/// drawer is one route's panel: an existing one (`existing`), or a blank one,
/// stated from `route` — the config's values on a visit, what was typed on a
/// refused save.
fn drawer(m: &Model, existing: Option<&Record>, route: &Route, e: &Errors) -> RowDrawer {
    let family = Widget::select(
        "family",
        "Family",
        if route.v6 { "ipv6" } else { "ipv4" },
        vec![
            SelectOption::new("ipv4", "IPv4"),
            SelectOption::new("ipv6", "IPv6"),
        ],
        e.get("family"),
    );
    let family = match existing {
        Some(_) => {
            family.locked("A route keeps its family. Delete it and add another to change it.")
        }
        None => family,
    };
    let interfaces = std::iter::once(SelectOption::new("", "Choose an interface"))
        .chain(
            m.networks
                .iter()
                .filter(|n| n.id != "loopback")
                .map(|n| SelectOption::new(&n.id, &n.id)),
        )
        .collect();
    let types = std::iter::once(SelectOption::new(
        if route.get("type") == "unicast" {
            "unicast"
        } else {
            ""
        },
        "Unicast",
    ))
    .chain(TYPES.iter().map(|(v, l)| SelectOption::new(v, l)))
    .collect();
    let mut fields = vec![
        family,
        select(route, e, "interface", "Interface", interfaces),
        field(route, e, "target", "Target", "The network this route reaches, such as 10.20.0.0/16."),
        field(
            route,
            e,
            "gateway",
            "Gateway",
            "The router that forwards this traffic. Leave empty when the network is on the interface itself.",
        ),
        field(route, e, "metric", "Metric", "Lower wins when two routes reach the same network."),
        Widget::switch_keyed("enabled", "Enabled", "disabled", "", route.enabled),
        Widget::section(
            "Further options",
            "Leave these unless something specific asks for them.",
            vec![
                field(route, e, "table", "Table", "A routing table number or name. Empty is the main table."),
                field(route, e, "source", "Source address", "The address traffic on this route is sent from."),
                field(route, e, "mtu", "MTU", ""),
                Widget::switch_keyed(
                    "onlink",
                    "Gateway is on the link",
                    "onlink",
                    "Use the gateway even when it sits outside the interface’s own network.",
                    route.onlink,
                ),
                select(route, e, "type", "Type", types),
                field(route, e, "proto", "Protocol", "The routing protocol this route is marked with."),
            ],
        )
        .ruled(),
    ];
    let values = values(existing, route);
    fields.push(Widget::config_preview(
        "/etc/config/network",
        &uci_text(
            if route.v6 { "route6" } else { "route" },
            existing.map(Record::config_id).unwrap_or(""),
            values.as_object().unwrap_or(&Map::new()),
        ),
    ));
    let mut children = vec![Widget::form("Save route", fields)];
    if existing.is_some() {
        children.push(Widget::Form {
            style: String::new(),
            submit: String::new(),
            error: String::new(),
            note: String::new(),
            target: String::new(),
            fields: vec![
                Widget::hidden(DELETE, "1"),
                Widget::Confirm {
                    trigger: "Delete route".into(),
                    title: String::new(),
                    message:
                        "Traffic for this network follows the remaining routes once you apply."
                            .into(),
                    confirm: "Delete route".into(),
                    cancel: String::new(),
                },
            ],
        });
    }
    RowDrawer {
        title: match existing {
            Some(_) => route.get("target").into(),
            None => "New route".into(),
        },
        closed: listing_href(),
        open: true,
        children,
        ..Default::default()
    }
}

/// network reads `addr[/len]` of the route's family; a bare address is a host.
fn network(s: &str, v6: bool) -> Option<(IpAddr, u32)> {
    let max = if v6 { 128 } else { 32 };
    let (addr, len) = s.split_once('/').unwrap_or((s, ""));
    let addr: IpAddr = addr.parse().ok()?;
    let len = if len.is_empty() {
        max
    } else {
        len.parse().ok()?
    };
    (addr.is_ipv6() == v6 && uint(&len.to_string(), 0, max)).then_some((addr, len))
}
/// host_bits_clear is the kernel's own condition on a route's target: nothing
/// set past the prefix.
fn host_bits_clear(addr: IpAddr, len: u32) -> bool {
    match addr {
        IpAddr::V4(a) => u32::from(a).checked_shl(len).unwrap_or(0) == 0,
        IpAddr::V6(a) => u128::from(a).checked_shl(len).unwrap_or(0) == 0,
    }
}
/// word is a routing table or protocol as iproute2 names one: a number, or a
/// name from its tables.
fn word(s: &str) -> bool {
    s.len() <= 64
        && s.bytes()
            .all(|b| b.is_ascii_alphanumeric() || b == b'_' || b == b'-')
}

fn validate(m: &Model, route: &Route) -> Errors {
    let mut e = Errors::default();
    let v6 = route.v6;
    let interface = route.get("interface");
    e.check(
        "interface",
        m.network(interface).is_some(),
        "Choose one of this router’s interfaces.",
    );
    match network(route.get("target"), v6) {
        None => e.field(
            "target",
            if v6 {
                "Write the IPv6 network to reach, such as 2001:db8::/32."
            } else {
                "Write the IPv4 network to reach, such as 10.20.0.0/16."
            },
        ),
        Some((addr, len)) => e.check(
            "target",
            host_bits_clear(addr, len),
            "Write the network’s first address: nothing may be set past the prefix length.",
        ),
    }
    let gateway = route.get("gateway");
    e.check(
        "gateway",
        gateway.is_empty() || gateway.parse::<IpAddr>().is_ok_and(|a| a.is_ipv6() == v6),
        if v6 {
            "Write the gateway as an IPv6 address, or leave it empty."
        } else {
            "Write the gateway as an IPv4 address, or leave it empty."
        },
    );
    let source = route.get("source");
    e.check(
        "source",
        source.is_empty() || network(source, v6).is_some(),
        "Write an address of the route’s family, or leave it empty.",
    );
    let metric = route.get("metric");
    e.check(
        "metric",
        metric.is_empty() || uint(metric, 0, u32::MAX),
        "Write a whole number, or leave it empty.",
    );
    let mtu = route.get("mtu");
    e.check(
        "mtu",
        mtu.is_empty() || uint(mtu, 68, 65535),
        "Write an MTU from 68 to 65535, or leave it empty.",
    );
    let kind = route.get("type");
    e.check(
        "type",
        matches!(kind, "" | "unicast") || TYPES.iter().any(|(v, _)| *v == kind),
        "Choose a route type.",
    );
    e.check(
        "table",
        word(route.get("table")),
        "Write a table number or name.",
    );
    e.check(
        "proto",
        word(route.get("proto")),
        "Write a protocol number or name.",
    );
    e
}

/// values is what a save writes: every option the route states, and — on a
/// section that exists — a null for each one it no longer states. An IPv4
/// target keeps the section's own way of stating it: `netmask` where it has
/// one, CIDR otherwise.
fn values(existing: Option<&Record>, route: &Route) -> Value {
    let mut v = Map::new();
    for k in TEXT {
        if !route.get(k).is_empty() {
            v.insert(k.into(), json!(route.get(k)));
        }
    }
    let masked = !route.v6 && existing.is_some_and(|r| !r.get("netmask").is_empty());
    if let (true, Some((addr, len))) = (masked, network(route.get("target"), false)) {
        let mask = Ipv4Addr::from(u32::MAX.checked_shl(32 - len).unwrap_or(0));
        v.insert("target".into(), json!(addr.to_string()));
        v.insert("netmask".into(), json!(mask.to_string()));
    }
    if !route.enabled {
        v.insert("disabled".into(), json!("1"));
    }
    if route.onlink {
        v.insert("onlink".into(), json!("1"));
    }
    if existing.is_some() {
        for k in TEXT.iter().chain(&["netmask", "disabled", "onlink"]) {
            v.entry(k.to_string()).or_insert(Value::Null);
        }
    }
    Value::Object(v)
}

fn write(existing: Option<&Record>, route: &Route) -> CommitOp {
    let values = values(existing, route);
    match existing {
        Some(r) => commit("network", &r.id, values),
        None => commit_new("network", if route.v6 { "route6" } else { "route" }, values),
    }
}

/// describe names each staged route by its target, one sentence per route
/// covering every change to it. A removed route is gone from the staged
/// snapshot, so its change keeps its raw line.
pub fn describe(changes: &[Change], s: &Snapshot) -> Vec<Description> {
    let mut order: Vec<&str> = vec![];
    let mut groups: BTreeMap<&str, Vec<usize>> = BTreeMap::new();
    for (i, c) in changes.iter().enumerate() {
        let typ = s
            .section("network", &c.section)
            .map(|s| s.scalar(".type"))
            .unwrap_or_default();
        if c.config != "network" || !matches!(typ.as_str(), "route" | "route6") {
            continue;
        }
        if !groups.contains_key(c.section.as_str()) {
            order.push(&c.section);
        }
        groups.entry(&c.section).or_default().push(i);
    }
    order
        .into_iter()
        .filter_map(|section| {
            let sec = s.section("network", section)?;
            let covers = groups.remove(section)?;
            let to = target(&sec.scalar("target"), &sec.scalar("netmask"));
            let plain = if covers.iter().any(|&i| changes[i].op == "add-section") {
                match sec.scalar("gateway").as_str() {
                    "" => format!("Added a route to {to} on {}.", sec.scalar("interface")),
                    gw => format!("Added a route to {to} via {gw}."),
                }
            } else {
                format!("Edited the route to {to}.")
            };
            Some(Description::new(plain, covers))
        })
        .collect()
}

#[cfg(test)]
mod tests {
    use super::*;
    use serde_json::{json, Value};
    use verso_plugin::{Request, Ubus};

    fn model() -> Model {
        Model::read(&Request {
            path: "/routes".into(),
            query: Form::parse(""),
            snapshot: Snapshot::from_value(json!({"network": {
                "lan": {".name": "lan", ".type": "interface", ".index": 0, "proto": "static"},
                "wan": {".name": "wan", ".type": "interface", ".index": 1, "proto": "dhcp"},
                "cfg02": {".name": "cfg02", ".type": "route", ".index": 2, ".anonymous": true,
                    "interface": "lan", "target": "10.20.0.0", "netmask": "255.255.0.0",
                    "gateway": "192.168.1.2", "metric": "10"},
                "cfg03": {".name": "cfg03", ".type": "route6", ".index": 3, ".anonymous": true,
                    "interface": "wan", "target": "2001:db8::/32"},
                "lab": {".name": "lab", ".type": "route", ".index": 4,
                    "interface": "lan", "target": "172.16.0.0/12", "disabled": "1", "mtu": "1400"}
            }})),
            ubus: Ubus::from_value(json!({"networkState": {
                "interfaces": [
                    {"interface": "lan", "l3_device": "br-lan"},
                    {"interface": "wan", "l3_device": "eth1"}
                ],
                "routes": [
                    {"family": 4, "target": "0.0.0.0/0", "gateway": "192.0.2.1", "device": "eth1", "metric": 0, "proto": "static"},
                    {"family": 4, "target": "10.0.0.0/24", "device": "br-lan", "metric": 0, "proto": "kernel", "source": "10.0.0.1"},
                    {"family": 4, "target": "10.20.0.0/16", "gateway": "192.168.1.2", "device": "br-lan", "metric": 10, "proto": "static"},
                    {"family": 4, "target": "172.17.0.0/16", "device": "docker0", "metric": 0, "proto": "kernel"},
                    {"family": 6, "target": "2001:db8:5::/64", "device": "br-lan", "metric": 1024, "proto": "ra"}
                ]
            }})),
        })
    }
    fn tables(tree: &Value) -> Vec<Value> {
        let mut out = vec![];
        fn walk(v: &Value, out: &mut Vec<Value>) {
            if v["type"] == "table" {
                out.push(v.clone());
            }
            match v {
                Value::Object(m) => m.values().for_each(|c| walk(c, out)),
                Value::Array(a) => a.iter().for_each(|c| walk(c, out)),
                _ => {}
            }
        }
        walk(tree, &mut out);
        out
    }

    fn targets(rows: &[Value]) -> Vec<String> {
        rows.iter()
            .map(|r| r["cells"][0]["text"].as_str().unwrap_or("").to_string())
            .collect()
    }

    #[test]
    fn one_table_reads_in_the_order_the_router_decides() {
        let view = json_of(page(&model(), "", ""));
        assert_eq!(view["title"], "Routes");
        assert_eq!(tables(&view).len(), 1);
        let rows = rows(&view);
        // Most specific first, the default last, one lane per family. The
        // kernel's 10.20.0.0/16 via 192.168.1.2 is custom cfg02, listed once.
        assert_eq!(
            targets(&rows),
            [
                "10.0.0.0/24",
                "10.20.0.0/16",
                "172.17.0.0/16",
                "172.16.0.0/12",
                "default",
                "2001:db8:5::/64",
                "2001:db8::/32",
            ]
        );
        assert_eq!(rows[0]["group"]["label"], "IPv4");
        assert_eq!(rows[0]["group"]["tally"], "5 routes · 2 custom");
        // The lane's add says what it does beside its glyph, as a firewall
        // lane's does; the family rides its accessible name.
        assert_eq!(rows[0]["group"]["add_text"], "Add route");
        assert_eq!(rows[0]["group"]["add_label"], "Add IPv4 route");
        assert_eq!(
            rows[0]["group"]["add_href"],
            "/plugins/interfaces/routes?open=new&family=ipv4"
        );
        assert_eq!(rows[5]["group"]["label"], "IPv6");
        assert_eq!(rows[5]["group"]["tally"], "2 routes · 1 custom");
        assert!(rows.iter().filter(|r| r.get("group").is_some()).count() == 2);
    }

    #[test]
    fn a_custom_route_is_a_door_and_the_kernels_are_read() {
        let rows = rows(&json_of(page(&model(), "", "")));
        let custom = &rows[1];
        assert_eq!(custom["id"], "cfg02");
        assert_eq!(custom["panel"], "/plugins/interfaces/routes?open=cfg02");
        let cells = &custom["cells"];
        assert_eq!(cells[0]["href"], custom["panel"]);
        assert_eq!(cells[1]["text"], "192.168.1.2");
        assert_eq!(cells[2]["chips"][0]["label"], "lan");
        assert_eq!(cells[3]["text"], "10");
        assert_eq!(cells[4]["chips"][0]["label"], "config");
        assert_eq!(cells[4]["chips"][0]["tone"], "accent");
        // In effect, so it says nothing about its state.
        assert!(cells[0].get("tag").is_none());

        let default = &rows[4];
        assert!(default.get("panel").is_none() && default.get("id").is_none());
        assert_eq!(default["cells"][1]["text"], "192.0.2.1");
        assert_eq!(default["cells"][2]["chips"][0]["label"], "wan");
        assert_eq!(default["cells"][4]["chips"][0]["label"], "static");
        assert!(default["cells"][4]["chips"][0].get("tone").is_none());
        assert!(default["cells"][5].get("actions").is_none());
        assert_eq!(rows[0]["cells"][1]["text"], "On-link");
        // A device no interface owns is cited by its own name.
        assert_eq!(rows[2]["cells"][2]["chips"][0]["label"], "docker0");
    }

    #[test]
    fn only_a_route_that_is_not_in_effect_speaks() {
        let rows = rows(&json_of(page(&model(), "", "")));
        let off = &rows[3];
        assert_eq!(off["id"], "lab");
        assert_eq!(off["muted"], true);
        assert_eq!(off["cells"][0]["tag"], "off");
        assert!(off["cells"][0].get("tag_variant").is_none());
        // Enabled in the config, missing from the router's table.
        let missing = &rows[6];
        assert_eq!(missing["id"], "cfg03");
        assert_eq!(missing["cells"][0]["tag"], "not active");
        assert_eq!(missing["cells"][0]["tag_variant"], "warning");
        assert!(missing.get("muted").is_none());
    }

    #[test]
    fn new_route_is_the_pages_act_and_nothing_narrows_the_listing() {
        // A still listing is scrolled and searched with the browser's own
        // find; the lanes and the origin chips already say what is custom.
        let view = json_of(page(&model(), "", ""));
        assert_eq!(view["act"]["label"], "New route");
        assert_eq!(view["act"]["href"], "/plugins/interfaces/routes?open=new");
        assert!(find(&view, &|v| v["type"] == "actionbar").is_none(), "{view}");
    }

    #[test]
    fn a_lanes_add_opens_new_route_in_its_family() {
        let view = json_of(page(&model(), "new", "ipv6"));
        assert_eq!(control(&drawer(&view), "family")["value"], "ipv6");
    }

    #[test]
    fn without_a_live_read_the_custom_routes_stand_alone_and_the_page_says_so() {
        let mut m = model();
        m.live = Value::Null;
        let view = json_of(page(&m, "", ""));
        assert_eq!(view["notice"]["level"], "warning");
        let rows = rows(&view);
        assert_eq!(rows.len(), 3);
        // Nothing to check against, so no route claims to be missing.
        assert!(rows.iter().all(|r| r["cells"][0]["tag"] != "not active"));
    }
    fn json_of(e: Envelope) -> Value {
        serde_json::to_value(e).expect("serialize")
    }
    fn find(tree: &Value, pred: &dyn Fn(&Value) -> bool) -> Option<Value> {
        if pred(tree) {
            return Some(tree.clone());
        }
        match tree {
            Value::Object(m) => m.values().find_map(|c| find(c, pred)),
            Value::Array(a) => a.iter().find_map(|c| find(c, pred)),
            _ => None,
        }
    }
    fn control(tree: &Value, name: &str) -> Value {
        find(tree, &|v| v["name"] == name && v.get("type").is_some())
            .unwrap_or_else(|| panic!("no control {name}"))
    }
    fn rows(tree: &Value) -> Vec<Value> {
        find(tree, &|v| v["type"] == "table").expect("table")["rows"]
            .as_array()
            .expect("rows")
            .clone()
    }
    fn drawer(tree: &Value) -> Value {
        find(tree, &|v| v["open"] == true && v.get("children").is_some()).expect("open drawer")
    }
    fn commit(e: &Envelope) -> Value {
        serde_json::to_value(&e.commit).expect("serialize")
    }

    #[test]
    fn nothing_configured_or_reported_says_so_in_plain_words() {
        let mut m = model();
        m.routes.clear();
        m.live = Value::Null;
        let page = json_of(page(&m, "", ""));
        assert_eq!(
            tables(&page)[0]["empty_text"],
            "No routes are configured or reported by the router."
        );
    }

    #[test]
    fn a_row_opens_filled_and_new_opens_blank() {
        let view = json_of(page(&model(), "cfg02", ""));
        let open = drawer(&view);
        assert_eq!(open["title"], "10.20.0.0/16");
        assert_eq!(open["closed"], "/plugins/interfaces/routes");
        assert_eq!(control(&open, "target")["value"], "10.20.0.0/16");
        assert_eq!(control(&open, "gateway")["value"], "192.168.1.2");
        assert_eq!(control(&open, "interface")["value"], "lan");
        assert_eq!(control(&open, "enabled")["on"], true);
        // The family is the section's type, fixed once it exists.
        assert_eq!(control(&open, "family")["style"], "locked");
        // Delete is a guarded act in the drawer.
        assert!(find(&open, &|v| v["type"] == "confirm").is_some());
        for key in [
            "table", "source", "mtu", "onlink", "type", "proto", "metric",
        ] {
            let c = control(&open, key);
            assert_eq!(c["key"], key, "{key}");
        }

        let view = json_of(page(&model(), "new", ""));
        let blank = drawer(&view);
        assert_eq!(blank["title"], "New route");
        assert_eq!(control(&blank, "target")["value"], "");
        assert_eq!(control(&blank, "family")["value"], "ipv4");
        assert!(control(&blank, "family").get("style").is_none());
        assert_eq!(control(&blank, "enabled")["on"], true);
        assert!(find(&blank, &|v| v["type"] == "confirm").is_none());
    }

    #[test]
    fn each_refusal_rides_its_field_and_nothing_stages() {
        let ok = "family=ipv4&interface=lan&target=10.30.0.0/16&enabled=1";
        for (body, field) in [
            ("family=ipv4&interface=lan&target=", "target"),
            ("family=ipv4&interface=lan&target=10.30.0.0/33", "target"),
            ("family=ipv4&interface=lan&target=2001:db8::/32", "target"),
            ("family=ipv4&interface=lan&target=10.30.0.1/16", "target"),
            ("family=ipv6&interface=lan&target=10.30.0.0/16", "target"),
            (
                "family=ipv4&interface=nope&target=10.30.0.0/16",
                "interface",
            ),
            ("family=ipv4&interface=&target=10.30.0.0/16", "interface"),
            (&*format!("{ok}&gateway=fe80::1"), "gateway"),
            (&*format!("{ok}&metric=-1"), "metric"),
            (&*format!("{ok}&metric=ten"), "metric"),
            (&*format!("{ok}&mtu=40"), "mtu"),
            (&*format!("{ok}&source=2001:db8::1"), "source"),
            (&*format!("{ok}&type=sideways"), "type"),
            (&*format!("{ok}&table=no%20spaces"), "table"),
            (&*format!("{ok}&proto=a%2Fb"), "proto"),
            ("family=ipx&interface=lan&target=10.30.0.0/16", "family"),
        ] {
            let e = post(&model(), "new", &Form::parse(body));
            assert!(e.commit.is_empty(), "{body} staged");
            let page = json_of(e);
            assert!(
                !control(&page, field)["error"]
                    .as_str()
                    .unwrap_or("")
                    .is_empty(),
                "{body}: {field} carries no error"
            );
        }
    }

    #[test]
    fn a_new_route_writes_only_what_was_set() {
        let v4 = post(
            &model(),
            "new",
            &Form::parse(
                "family=ipv4&interface=lan&target=10.30.0.0/16&gateway=192.168.1.3&enabled=1&type=",
            ),
        );
        assert_eq!(
            commit(&v4),
            json!([{"config": "network", "section": "", "type": "route",
                "values": {"interface": "lan", "target": "10.30.0.0/16", "gateway": "192.168.1.3"}}])
        );
        let v6 = post(
            &model(),
            "new",
            &Form::parse("family=ipv6&interface=wan&target=2001:db8:1::/48&metric=5&onlink=1"),
        );
        assert_eq!(
            commit(&v6),
            json!([{"config": "network", "section": "", "type": "route6",
                "values": {"interface": "wan", "target": "2001:db8:1::/48", "metric": "5",
                    "onlink": "1", "disabled": "1"}}])
        );
    }

    #[test]
    fn an_edit_keeps_the_sections_netmask_style_and_clears_what_was_emptied() {
        let e = post(
            &model(),
            "cfg02",
            &Form::parse("interface=lan&target=10.20.0.0/24&gateway=192.168.1.2&metric=&enabled=1"),
        );
        assert_eq!(
            commit(&e),
            json!([{"config": "network", "section": "cfg02", "values": {
                "interface": "lan", "target": "10.20.0.0", "netmask": "255.255.255.0",
                "gateway": "192.168.1.2", "metric": null, "disabled": null, "table": null,
                "source": null, "mtu": null, "onlink": null, "type": null, "proto": null}}])
        );
        // A section stating CIDR keeps CIDR; the family posted is ignored.
        let e = post(
            &model(),
            "lab",
            &Form::parse("family=ipv6&interface=lan&target=172.16.0.0/12&enabled=1&mtu=1400"),
        );
        let values = &commit(&e)[0]["values"];
        assert_eq!(values["target"], "172.16.0.0/12");
        assert_eq!(values["netmask"], Value::Null);
        assert_eq!(values["disabled"], Value::Null);
        assert_eq!(values["mtu"], "1400");
    }

    #[test]
    fn delete_removes_the_section_and_a_stale_one_removes_nothing() {
        let e = post(&model(), "cfg03", &Form::parse("_delete=1"));
        assert_eq!(
            commit(&e),
            json!([{"config": "network", "section": "cfg03", "delete": true}])
        );
        let page = json_of(e);
        assert!(find(&page, &|v| v["open"] == true && v.get("children").is_some()).is_none());
        let stale = post(&model(), "gone", &Form::parse("_delete=1"));
        assert!(stale.commit.is_empty());
        assert_eq!(json_of(stale)["notice"]["level"], "danger");
    }

    fn change(op: &str, section: &str, option: &str, value: &str) -> Change {
        Change {
            config: "network".into(),
            op: op.into(),
            section: section.into(),
            option: option.into(),
            value: value.into(),
        }
    }

    #[test]
    fn a_route_is_described_by_its_target() {
        let staged = Snapshot::from_value(json!({"network": {
            "cfg09": {".name": "cfg09", ".type": "route", "interface": "lan",
                "target": "10.20.0.0/16", "gateway": "192.168.1.2"},
            "cfg0a": {".name": "cfg0a", ".type": "route6", "interface": "wan", "target": "2001:db8::/32"},
            "cfg02": {".name": "cfg02", ".type": "route", "interface": "lan",
                "target": "10.20.0.0", "netmask": "255.255.0.0"},
            "lan": {".name": "lan", ".type": "interface"}
        }}));
        let changes = vec![
            change("add-section", "cfg09", "route", ""),
            change("set", "cfg09", "target", "10.20.0.0/16"),
            change("set", "lan", "mtu", "1400"),
            change("set", "cfg09", "gateway", "192.168.1.2"),
            change("add-section", "cfg0a", "route6", ""),
            change("set", "cfg02", "metric", "5"),
            change("remove-option", "cfg02", "gateway", ""),
        ];
        let out = describe(&changes, &staged);
        let read: Vec<_> = out
            .iter()
            .map(|d| (d.plain.as_str(), d.covers.clone()))
            .collect();
        assert_eq!(
            read,
            vec![
                (
                    "Added a route to 10.20.0.0/16 via 192.168.1.2.",
                    vec![0, 1, 3]
                ),
                ("Added a route to 2001:db8::/32 on wan.", vec![4]),
                ("Edited the route to 10.20.0.0/16.", vec![5, 6]),
            ]
        );
    }
}
