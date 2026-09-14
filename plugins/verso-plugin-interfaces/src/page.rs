// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.
use crate::editor::url;
use crate::{Model, ROOT};
use serde_json::Value;
use std::collections::{BTreeMap, BTreeSet};
use std::net::Ipv6Addr;
use verso_plugin::{
    ActionTab, Envelope, Property, RowDrawer, TableAction, TableCell, TableChip, TableColumn,
    TableRow, TableRowAct, Tone, Widget,
};

fn text(v: Option<&Value>) -> String {
    v.and_then(Value::as_str).unwrap_or("").into()
}
// Accept old native payloads too during shell/plugin rolling upgrades.
fn flag(v: Option<&Value>) -> Option<bool> {
    match v {
        Some(Value::Bool(b)) => Some(*b),
        Some(Value::Number(n)) => n.as_i64().map(|n| n != 0),
        _ => None,
    }
}
fn cell(s: impl Into<String>) -> TableCell {
    TableCell {
        text: s.into(),
        ..Default::default()
    }
}
fn fact(label: &str, value: impl Into<String>) -> Property {
    let value = value.into();
    Property {
        label: label.into(),
        value: if value.is_empty() {
            "—".into()
        } else {
            value
        },
        mono: true,
        ..Default::default()
    }
}
fn prose(label: &str, value: &str) -> Property {
    Property {
        mono: false,
        ..fact(label, value)
    }
}
fn pair(left: Vec<Property>, right: Vec<Property>) -> Widget {
    Widget::Grid {
        columns: 2,
        style: "facts".into(),
        children: vec![
            Widget::Properties { items: left },
            Widget::Properties { items: right },
        ],
    }
}
fn configuration(children: Vec<Widget>) -> Widget {
    Widget::Grid {
        columns: 2,
        style: "configurations".into(),
        children,
    }
}
fn destination(label: &str, kind: &str, description: &str, key: &str, glyph: &str) -> Widget {
    let mut link = Widget::link(label, &format!("{ROOT}new?kind={kind}"), "choice");
    if let Widget::Link {
        desc, code, icon, ..
    } = &mut link
    {
        *desc = description.into();
        *code = key.into();
        *icon = glyph.into();
    }
    link
}
fn category(title: &str, children: Vec<Widget>) -> Widget {
    let mut section = Widget::section(title, "", children);
    if let Widget::Section { kicker, flush, .. } = &mut section {
        *kicker = true;
        *flush = true;
    }
    section
}
fn chooser(open: bool) -> RowDrawer {
    let mut vpn = Widget::link("A VPN tunnel", &format!("{ROOT}vpn"), "choice");
    if let Widget::Link {
        desc, code, icon, ..
    } = &mut vpn
    {
        *desc = "WireGuard and OpenVPN are set up on their own page, with keys, peers and codes to scan.".into();
        *code = "Tunnels page".into();
        *icon = "lock".into();
    }
    RowDrawer { title: "New interface".into(), size: "choices".into(), open, closed: ROOT.into(), children: vec![
        category("Networks", vec![
            destination("A network", "network", "Its own address range, firewall zone and DHCP server — a guest network, one for the smart-home gadgets, a lab. Runs on a bridge, a VLAN or a single port.", "config interface", "network"),
            destination("An internet connection", "wan", "A second line, or a replacement for the one you have: DHCP, PPPoE or a static address.", "config interface", "globe")]),
        category("Devices", vec![
            destination("A bridge", "bridge", "Joins ports and Wi-Fi into one network, as though they were one switch. Split it into VLANs to run several networks over the same cables.", "config device", "git-merge"),
            destination("A VLAN", "vlan", "Tags the traffic on a port with a number, so one cable carries several networks — your ISP’s VLAN 3900, or a managed switch downstairs.", "config device", "tag"), vpn,
            destination("Another kind of tunnel", "tunnel", "IPv6 over an IPv4 line, GRE or VXLAN.", "config interface", "git-branch")])], ..Default::default() }
}
pub(crate) fn config_text(typ: &str, id: &str, values: &serde_json::Map<String, Value>) -> String {
    let quote = |s: &str| {
        s.replace('\\', "\\\\")
            .replace('\'', "'\\''")
            .replace('\n', "\\n")
            .replace('\r', "\\r")
    };
    let mut lines = vec![if id.is_empty() {
        format!("config {typ}")
    } else {
        format!("config {typ} '{}'", quote(id))
    }];
    for (k, v) in values {
        if k == "password" || k == "private_key" {
            continue;
        }
        match v {
            Value::String(s) => lines.push(format!("\toption {k} '{}'", quote(s))),
            Value::Array(a) => {
                for s in a.iter().filter_map(Value::as_str) {
                    lines.push(format!("\tlist {k} '{}'", quote(s)));
                }
            }
            _ => {}
        }
    }
    lines.join("\n")
}
fn addresses(live: Option<&Value>, family: &str) -> Vec<String> {
    let Some(live) = live else { return vec![] };
    let mut entries = live
        .get(family)
        .and_then(Value::as_array)
        .cloned()
        .unwrap_or_default();
    if family == "ipv6-address" {
        if let Some(prefixes) = live.get("ipv6-prefix-assignment").and_then(Value::as_array) {
            entries.extend(
                prefixes
                    .iter()
                    .filter_map(|p| p.get("local-address").cloned()),
            );
        }
    }
    let mut out = vec![];
    for entry in entries {
        let address = text(entry.get("address"));
        if address.is_empty() {
            continue;
        }
        let value = match entry.get("mask").and_then(Value::as_u64) {
            Some(mask) => format!("{address}/{mask}"),
            None => address,
        };
        if !out.contains(&value) {
            out.push(value);
        }
    }
    out
}
fn ula(address: &str) -> bool {
    address
        .split('/')
        .next()
        .and_then(|a| a.parse::<Ipv6Addr>().ok())
        .is_some_and(|a| a.segments()[0] & 0xfe00 == 0xfc00)
}
fn uptime(live: Option<&Value>) -> String {
    let Some(s) = live.and_then(|l| l.get("uptime")).and_then(Value::as_u64) else {
        return String::new();
    };
    if s >= 86400 {
        format!("{}d {}h {}m", s / 86400, s % 86400 / 3600, s % 3600 / 60)
    } else if s >= 3600 {
        format!("{}h {}m", s / 3600, s % 3600 / 60)
    } else {
        format!("{}m {}s", s / 60, s % 60)
    }
}
fn speed(runtime: Option<&Value>) -> String {
    let raw = text(runtime.and_then(|v| v.get("speed")));
    let digits = raw.trim_end_matches(['F', 'H']);
    if let Ok(mbps) = digits.parse::<u32>() {
        if mbps >= 1000 {
            format!("{} Gb/s", f64::from(mbps) / 1000.0)
        } else {
            format!("{mbps} Mb/s")
        }
    } else {
        String::new()
    }
}
fn kind(m: &Model, name: &str, runtime: Option<&Value>) -> String {
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
    if runtime_kind == "port" {
        let rate = speed(runtime).replace(" Gb/s", " g").replace(" Mb/s", " m");
        return if rate.is_empty() {
            "port".into()
        } else {
            format!("port · {rate}")
        };
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
    "—".into()
}
fn state(
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
fn details(
    m: &Model,
    name: &str,
    nets: &[String],
    runtime: Option<&Value>,
    kind: &str,
) -> Vec<Widget> {
    let mut out = vec![];
    if nets.is_empty() {
        let ports = m.bridge_ports(name);
        let mtu = runtime
            .and_then(|v| v.get("mtu"))
            .and_then(Value::as_u64)
            .map(|n| n.to_string())
            .unwrap_or_default();
        let stats = runtime.and_then(|v| v.get("statistics"));
        let errors = match (
            stats
                .and_then(|v| v.get("rx_errors"))
                .and_then(Value::as_u64),
            stats
                .and_then(|v| v.get("tx_errors"))
                .and_then(Value::as_u64),
        ) {
            (Some(rx), Some(tx)) => format!("RX {rx} · TX {tx}"),
            _ => String::new(),
        };
        let (left, right) = if kind == "bridge" {
            let stp = flag(
                runtime
                    .and_then(|v| v.get("bridge-attributes"))
                    .and_then(|v| v.get("stp")),
            );
            (
                fact("Ports", ports.join(", ")),
                prose(
                    "STP",
                    match stp {
                        Some(true) => "On",
                        Some(false) => "Off",
                        _ => "Not reported",
                    },
                ),
            )
        } else {
            (
                fact("Link", speed(runtime)),
                fact("Driver", text(runtime.and_then(|v| v.get("driver")))),
            )
        };
        let mut left = vec![left, fact("MTU", mtu)];
        let mut right = vec![right, fact("Errors", errors)];
        let ip4 = addresses(runtime, "ipv4-address");
        let ip6 = addresses(runtime, "ipv6-address");
        if !ip4.is_empty() {
            left.push(fact("IPv4", ip4.join(", ")));
        }
        if !ip6.is_empty() {
            right.push(fact("IPv6", ip6.join(", ")));
        }
        out.push(pair(left, right));
    }
    for net in nets {
        let live = m.live_network(net);
        let ip4 = addresses(live, "ipv4-address");
        let ip6 = addresses(live, "ipv6-address");
        let protocol = text(live.and_then(|v| v.get("proto")));
        let mut left = vec![fact("IPv4", ip4.join(", ")), fact("Protocol", protocol)];
        let mut right = vec![
            fact(
                "IPv6",
                ip6.iter()
                    .filter(|a| !ula(a))
                    .cloned()
                    .collect::<Vec<_>>()
                    .join(", "),
            ),
            fact(
                "ULA",
                ip6.iter()
                    .filter(|a| ula(a))
                    .cloned()
                    .collect::<Vec<_>>()
                    .join(", "),
            ),
            fact("Uptime", uptime(live)),
        ];
        let dhcp = m.dhcp_servers.iter().find(|s| &s.network == net);
        if let Some(server) = dhcp {
            left.push(prose("DHCP server", server.label()));
            if !server.pool.is_empty() {
                left.push(if server.pool == "reservations" {
                    prose("DHCP pool", server.pool_label())
                } else {
                    fact("DHCP pool", server.pool_label())
                });
            }
            if !server.lease_time.is_empty() {
                left.push(fact("Lease time", &server.lease_time));
            }
            left.push(prose(
                "Active leases",
                &server
                    .leases
                    .map(|n| n.to_string())
                    .unwrap_or_else(|| "—".into()),
            ));
        }
        if nets.len() > 1 {
            left.insert(0, fact("Network", net));
            right.insert(
                0,
                prose(
                    "State",
                    state(m, std::slice::from_ref(net), runtime, true).0,
                ),
            );
        }
        for p in left.iter_mut().chain(right.iter_mut()) {
            p.copy = matches!(p.label.as_str(), "IPv4" | "IPv6" | "ULA") && p.value != "—";
        }
        let zones: Vec<_> = m
            .zones
            .iter()
            .filter(|z| z.list("network").contains(net))
            .collect();
        if !zones.is_empty() {
            left.push(fact(
                "Firewall zone",
                zones
                    .iter()
                    .map(|z| z.get("name"))
                    .collect::<Vec<_>>()
                    .join(", "),
            ));
        }
        out.push(pair(left, right));
        if let Some(server) = dhcp {
            out.push(Widget::link("Configure DHCP", &server.href(), "secondary"));
        }
        if let Some(errors) = live
            .and_then(|v| v.get("errors"))
            .and_then(Value::as_array)
            .filter(|e| !e.is_empty())
        {
            out.push(Widget::code(
                "netifd",
                &serde_json::to_string_pretty(errors).unwrap_or_default(),
            ));
        }
        let mut configs = vec![];
        if let Some(n) = m.network(net) {
            configs.push(Widget::code(
                "/etc/config/network · interface",
                &config_text("interface", n.config_id(), &n.values),
            ));
        }
        if let Some(d) = m.dhcp(net) {
            configs.push(Widget::code(
                "/etc/config/dhcp · dhcp",
                &config_text("dhcp", d.config_id(), &d.values),
            ));
        }
        if !configs.is_empty() {
            out.push(configuration(configs));
        }
        if nets.len() > 1 && m.network(net).is_some() {
            out.push(Widget::link(
                "Edit network",
                &url(net, "", "edit"),
                "secondary",
            ));
        }
    }
    if let Some(d) = m.device(name) {
        out.push(configuration(vec![Widget::code(
            "/etc/config/network · device",
            &config_text("device", d.config_id(), &d.values),
        )]));
    }
    if !nets.is_empty() && m.device(name).is_some() {
        out.push(Widget::link(
            "Edit device",
            &url("", name, "edit"),
            "secondary",
        ));
    }
    out
}

pub fn listing(m: &Model, open: bool) -> Envelope {
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
        let live = m.live_network(&net);
        let mut device = text(live.and_then(|v| v.get("l3_device")));
        if device.is_empty() {
            device = text(live.and_then(|v| v.get("device")));
        }
        if device.is_empty() {
            device = m
                .network(&net)
                .map(|n| m.resolve_device(&n.get("device")))
                .unwrap_or_default();
        }
        if device.is_empty() {
            device = net.clone();
        }
        names.insert(device.clone());
        owners.entry(device).or_default().push(net);
    }
    names.remove("lo");
    let mut ordered = vec![];
    let mut seen = BTreeSet::new();
    fn visit(
        name: &str,
        depth: u32,
        names: &BTreeSet<String>,
        parents: &BTreeMap<String, String>,
        seen: &mut BTreeSet<String>,
        out: &mut Vec<(String, u32)>,
    ) {
        if !seen.insert(name.into()) {
            return;
        }
        out.push((name.into(), depth));
        for child in names {
            if parents.get(child).is_some_and(|p| p == name) {
                visit(child, depth + 1, names, parents, seen, out);
            }
        }
    }
    for name in &names {
        if parents.get(name).is_none_or(|p| !names.contains(p)) {
            visit(name, 0, &names, &parents, &mut seen, &mut ordered);
        }
    }
    for name in &names {
        visit(name, 0, &names, &parents, &mut seen, &mut ordered);
    }
    let mut rows = vec![];
    let mut attention = 0;
    for (name, depth) in ordered {
        let runtime = m.live.get("devices").and_then(|v| v.get(&name));
        let nets = owners.get(&name).cloned().unwrap_or_default();
        let configured = m.device(&name);
        let physical = flag(runtime.and_then(|v| v.get("physical"))) == Some(true);
        let kind = kind(m, &name, runtime);
        let used = !nets.is_empty()
            || parents.values().any(|p| p == &name)
            || m.devices.iter().any(|d| d.list("ports").contains(&name))
            || m.live
                .get("devices")
                .and_then(Value::as_object)
                .is_some_and(|ds| {
                    ds.values()
                        .any(|d| crate::model::strings(d.get("bridge-members")).contains(&name))
                });
        let (state, tone) = state(m, &nets, runtime, used);
        let problem = tone == "danger"
            || tone == "warning"
            || m.dhcp_servers
                .iter()
                .any(|s| nets.contains(&s.network) && matches!(s.tone(), "danger" | "warning"));
        if problem {
            attention += 1;
        }
        let mut ipv4: Vec<_> = nets
            .iter()
            .flat_map(|n| addresses(m.live_network(n), "ipv4-address"))
            .collect();
        if nets.is_empty() {
            ipv4 = addresses(runtime, "ipv4-address");
        }
        // Only observed addresses belong in this column. Configured overrides
        // remain visible in the UCI block until Apply makes them real.
        let mac = text(runtime.and_then(|v| v.get("macaddr")));
        let mut actions = (0..4).map(|_| TableRowAct::default()).collect::<Vec<_>>();
        if let Some(net) = nets.iter().find(|n| m.network(n).is_some()) {
            if m.live_network(net).is_some() {
                actions[0] = TableRowAct {
                    icon: "refresh-cw".into(),
                    title: "Restart".into(),
                    name: "restart".into(),
                    value: net.clone(),
                    ..Default::default()
                };
                let up = flag(m.live_network(net).and_then(|v| v.get("up"))) == Some(true);
                actions[1] = TableRowAct {
                    icon: "power".into(),
                    title: if up { "Bring down" } else { "Bring up" }.into(),
                    name: if up { "down" } else { "up" }.into(),
                    value: net.clone(),
                    ..Default::default()
                };
            }
            actions[2] = TableRowAct {
                icon: "trash-2".into(),
                title: "Delete".into(),
                href: url(net, "", "delete"),
                ..Default::default()
            };
            actions[3] = TableRowAct {
                icon: "square-pen".into(),
                title: "Edit".into(),
                href: url(net, "", "edit"),
                ..Default::default()
            };
        } else {
            if configured.is_some() && !used {
                actions[2] = TableRowAct {
                    icon: "trash-2".into(),
                    title: "Delete".into(),
                    href: url("", &name, "delete"),
                    ..Default::default()
                };
            }
            if runtime.is_some() || configured.is_some() {
                actions[3] = TableRowAct {
                    icon: "square-pen".into(),
                    title: "Edit".into(),
                    href: url("", &name, "edit"),
                    ..Default::default()
                };
            }
        }
        rows.push(TableRow {
            id: name.clone(),
            depth,
            tags: if problem {
                vec!["attention".into()]
            } else {
                vec![]
            },
            expanded: details(m, &name, &nets, runtime, &kind),
            cells: vec![
                TableCell {
                    lead_icon: if physical { "physical" } else { "software" }.into(),
                    chips: nets
                        .iter()
                        .flat_map(|n| {
                            let mut chips = vec![TableChip {
                                icon: "network".into(),
                                label: n.clone(),
                                tone: "accent".into(),
                                ..Default::default()
                            }];
                            if let Some(server) = m
                                .dhcp_servers
                                .iter()
                                .find(|s| &s.network == n && s.configured)
                            {
                                chips.push(TableChip {
                                    label: "DHCP".into(),
                                    tone: server.tone().into(),
                                    title: server.title().into(),
                                    ..Default::default()
                                });
                            }
                            chips
                        })
                        .collect(),
                    ..cell(name)
                },
                cell(kind),
                TableCell {
                    copy: !ipv4.is_empty(),
                    ..cell(ipv4.first().cloned().unwrap_or_else(|| "—".into()))
                },
                TableCell {
                    copy: !mac.is_empty(),
                    ..cell(if mac.is_empty() { "—".into() } else { mac })
                },
                TableCell {
                    variant: tone.into(),
                    dot: true,
                    ..cell(state)
                },
                TableCell {
                    actions,
                    ..Default::default()
                },
            ],
            ..Default::default()
        });
    }
    let toolbar = Widget::ActionBar {
        style: "interfaces".into(),
        tabs: vec![ActionTab {
            label: "Problems".into(),
            count: attention,
            matches: "attention".into(),
            ..Default::default()
        }],
        filter: "Find an interface".into(),
        live: String::new(),
        action: Some(TableAction {
            label: "Add interface".into(),
            href: format!("{ROOT}?new=1"),
            icon: "plus".into(),
            ..Default::default()
        }),
        opens_panel: true,
        drawer: Some(chooser(open)),
    };
    let columns = [
        ("Device · network", "reference"),
        ("Type", "keyword"),
        ("Address", "mono"),
        ("MAC address", "mono"),
        ("State", "status"),
        ("", "actions"),
    ]
    .into_iter()
    .map(|(label, kind)| TableColumn {
        label: label.into(),
        kind: kind.into(),
        width: String::new(),
    })
    .collect();
    let table = Widget::Table {
        style: "interfaces".into(),
        title: String::new(),
        detail: String::new(),
        dense: false,
        align: String::new(),
        reorder_config: String::new(),
        reorder_label: String::new(),
        columns,
        rows,
        drawer_label: String::new(),
        drawer_icon: String::new(),
        empty_text: "No interfaces are configured or reported by the router.".into(),
        add_label: String::new(),
        add_href: String::new(),
        note: String::new(),
        stream: None,
    };
    let page = Envelope::page("Interfaces", Widget::stack(vec![toolbar, table])).with_width("wide");
    if m.live.get("interfaces").is_none() {
        page.with_notice(
            Tone::Warning,
            "Live status could not be read. Configured interfaces are still listed.",
        )
    } else {
        page
    }
}
