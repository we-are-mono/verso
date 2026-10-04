// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.
use crate::{Model, ROOT};
use serde_json::Value;
use std::collections::{BTreeMap, BTreeSet};
use std::net::{IpAddr, Ipv4Addr, Ipv6Addr};
use verso_plugin::{
    commit, commit_delete, commit_new, json, CommitOp, Envelope, Field, Form, List, RowDrawer,
    SectionWidget, SelectOption, Tone, Widget,
};

type Errors = BTreeMap<String, String>;
#[derive(Default)]
struct Values {
    scalar: BTreeMap<String, String>,
    lists: BTreeMap<String, Vec<String>>,
}
impl Values {
    fn get(&self, k: &str) -> String {
        self.scalar.get(k).cloned().unwrap_or_default()
    }
    fn list(&self, k: &str) -> Vec<String> {
        self.lists.get(k).cloned().unwrap_or_default()
    }
    fn set(&mut self, k: &str, v: impl Into<String>) {
        self.scalar.insert(k.into(), v.into());
    }
    fn posted(f: &Form) -> Self {
        let keys="name device zone proto ipaddr netmask gateway hostname peerdns defaultroute username password ac service keepalive ip6assign auto mtu metric macaddr vid vlan_protocol stp igmp_snooping peeraddr ip6addr ip6prefix ttl port tunlink";
        let mut v = Self::default();
        for k in keys.split_whitespace() {
            v.set(
                k,
                if k == "password" {
                    f.get(k)
                } else {
                    f.get(k).trim().into()
                },
            );
        }
        for k in ["dns", "ports"] {
            let mut seen = BTreeSet::new();
            v.lists.insert(
                k.into(),
                f.all(k)
                    .iter()
                    .map(|x| x.trim().to_string())
                    .filter(|x| !x.is_empty() && seen.insert(x.clone()))
                    .collect(),
            );
        }
        v
    }
}
pub fn url(network: &str, device: &str, route: &str) -> String {
    format!(
        "{ROOT}{route}?network={}&device={}",
        encode(network),
        encode(device)
    )
}
fn encode(s: &str) -> String {
    s.bytes()
        .map(|b| {
            if b.is_ascii_alphanumeric() || b"-_.".contains(&b) {
                (b as char).to_string()
            } else {
                format!("%{b:02X}")
            }
        })
        .collect()
}
fn defaults(m: &Model, kind: &str, network: &str, device: &str) -> Values {
    let mut v = Values::default();
    for (k, x) in [
        (
            "proto",
            if kind == "wan" {
                "dhcp"
            } else if kind == "tunnel" {
                "gre"
            } else {
                "static"
            },
        ),
        ("netmask", "255.255.255.0"),
        ("auto", "1"),
        ("peerdns", "1"),
        ("defaultroute", "1"),
        ("vlan_protocol", "8021q"),
        ("stp", "1"),
    ] {
        v.set(k, x);
    }
    if let Some(r) = if !network.is_empty() {
        m.network(network)
    } else {
        m.device(device)
    } {
        for (k, x) in &r.values {
            if let Some(s) = x.as_str() {
                v.set(k, s);
            }
        }
        for k in ["ports", "dns"] {
            v.lists.insert(k.into(), r.list(k));
        }
        v.set(
            "name",
            if !network.is_empty() {
                network.into()
            } else {
                r.get("name")
            },
        );
        if !network.is_empty() {
            if r.get("proto") == "grev6" {
                v.set("peeraddr", r.get("peer6addr"));
            }
            v.set("zone", m.zone(network));
            v.set("auto", if r.get("auto") == "0" { "0" } else { "1" });
        } else {
            v.set("device", r.get("ifname"));
            v.set("vlan_protocol", r.get("type"));
        }
    } else if !device.is_empty() {
        v.set("name", device);
    }
    v.set("password", "");
    if kind == "wan" && m.zones.iter().any(|z| z.get("name") == "wan") {
        v.set("zone", "wan");
    }
    v
}
fn kind_of(m: &Model, network: &str, device: &str) -> Option<String> {
    if !network.is_empty() {
        let n = m.network(network)?;
        if n.id == "loopback" {
            return None;
        }
        return Some(
            if matches!(
                n.get("proto").as_str(),
                "gre" | "gretap" | "grev6" | "vxlan" | "6in4"
            ) {
                "tunnel"
            } else {
                "network"
            }
            .into(),
        );
    }
    if !m.names().contains(device) {
        return None;
    }
    Some(
        match m.device(device).map(|r| r.get("type")).as_deref() {
            Some("bridge") => "bridge",
            Some("8021q" | "8021ad") => "vlan",
            _ => "device",
        }
        .into(),
    )
}
// KINDS are what a new interface can be, in the order the Type choice offers
// them; a network first, because a new network is what Add interface most
// often makes.
const KINDS: [(&str, &str); 5] = [
    ("network", "Network"),
    ("wan", "Internet connection"),
    ("bridge", "Bridge"),
    ("vlan", "VLAN"),
    ("tunnel", "Tunnel"),
];
fn creatable(kind: &str) -> bool {
    KINDS.iter().any(|(k, _)| *k == kind)
}
// about is what the kind chosen is for, raised from the Type choice's label.
fn about(kind: &str) -> &'static str {
    match kind {
        "wan" => "A second line, or a replacement for the one you have: DHCP, PPPoE or a static address.",
        "bridge" => "Joins ports and Wi-Fi into one network, as though they were one switch. Split it into VLANs to run several networks over the same cables.",
        "vlan" => "Tags the traffic on a port with a number, so one cable carries several networks — your ISP’s VLAN 3900, or a managed switch downstairs.",
        "tunnel" => "IPv6 over an IPv4 line, GRE or VXLAN.",
        _ => "Its own address range, firewall zone and DHCP server — a guest network, one for the smart-home gadgets, a lab. Runs on a bridge, a VLAN or a single port.",
    }
}
// new_kind is the kind a new interface's form is for: the Type chosen on it,
// else the one its address names, else a network.
pub fn new_kind(f: &Form, query: &Form) -> String {
    [f.get("kind"), query.get("kind")]
        .into_iter()
        .find(|k| !k.is_empty())
        .unwrap_or_else(|| "network".into())
}
pub fn new(m: &Model, kind: &str) -> Envelope {
    let kind = if kind.is_empty() { "network" } else { kind };
    if !creatable(kind) {
        return crate::missing();
    }
    page(
        m,
        kind,
        "",
        "",
        &defaults(m, kind, "", ""),
        &Errors::new(),
        "",
    )
}
// reshaped is the new interface's drawer drawn again for the Type just chosen:
// that kind's own defaults, with what the form already said kept wherever it
// still means the same thing — the name, and the device a network, a line or
// a VLAN runs on, and the link's size and address. Nothing is refused: what
// was typed for the old kind is still being typed.
fn reshaped(m: &Model, kind: &str, f: &Form) -> Envelope {
    let posted = Values::posted(f);
    let mut v = defaults(m, kind, "", "");
    let mut kept = vec!["name", "mtu", "macaddr"];
    if matches!(kind, "network" | "wan" | "vlan") {
        kept.push("device");
    }
    for k in kept {
        let value = posted.get(k);
        if !value.is_empty() {
            v.set(k, value);
        }
    }
    page(m, kind, "", "", &v, &Errors::new(), "")
}
pub fn edit(m: &Model, network: &str, device: &str) -> Envelope {
    let Some(kind) = kind_of(m, network, device) else {
        return crate::missing();
    };
    page(
        m,
        &kind,
        network,
        device,
        &defaults(m, &kind, network, device),
        &Errors::new(),
        "",
    )
}
fn field(v: &Values, e: &Errors, key: &str, label: &str, datatype: &str) -> Widget {
    let value = v.get(key);
    let mut w = Widget::field(
        key,
        label,
        &value,
        if value.is_empty() { "" } else { datatype },
        "",
    )
    .writes(key);
    if let Widget::Field(Field { error, .. }) = &mut w {
        *error = e.get(key).cloned().unwrap_or_default();
    }
    w
}
fn select(v: &Values, e: &Errors, key: &str, label: &str, options: Vec<SelectOption>) -> Widget {
    Widget::select(
        key,
        label,
        &v.get(key),
        options,
        e.get(key).map(String::as_str).unwrap_or(""),
    )
    .writes(key)
}
fn opts(items: &[(&str, &str)]) -> Vec<SelectOption> {
    items.iter().map(|(v, l)| SelectOption::new(v, l)).collect()
}
fn check(v: &Values, key: &str, label: &str) -> Widget {
    Widget::switch_keyed(key, label, key, "", v.get(key) == "1")
}
fn list(v: &Values, e: &Errors, key: &str, label: &str, datatype: &str) -> Widget {
    let mut w = Widget::list(key, label, datatype, &v.list(key), "").writes(key);
    if let Widget::List(List { style, errors, .. }) = &mut w {
        *style = "rows".into();
        if let Some(e) = e.get(key) {
            errors.insert("0".into(), e.clone());
        }
    }
    w
}
fn section(title: &str, sub: &str, anchor: &str, children: Vec<Widget>) -> Widget {
    let mut w = Widget::section(title, sub, children).ruled();
    if let Widget::Section(SectionWidget { anchor: a, .. }) = &mut w {
        *a = anchor.into();
    }
    w
}
fn when(v: &Values, proto: &str, children: Vec<Widget>) -> Widget {
    Widget::When {
        name: "proto".into(),
        value: proto.into(),
        active: v.get("proto") == proto,
        children,
    }
}
// page is an interface's drawer open over the listing: an address naming it
// shows the listing with the drawer open, a request for the panel alone is
// answered from the same tree, and a save returns to the listing.
fn page(
    m: &Model,
    kind: &str,
    network: &str,
    device: &str,
    v: &Values,
    e: &Errors,
    error: &str,
) -> Envelope {
    crate::page::with_drawer(m, drawer(m, kind, network, device, v, e, error))
        .with_back("Interfaces", ROOT)
}
fn drawer(
    m: &Model,
    kind: &str,
    network: &str,
    device: &str,
    v: &Values,
    e: &Errors,
    error: &str,
) -> RowDrawer {
    let creating = network.is_empty() && device.is_empty();
    let is_device = matches!(kind, "bridge" | "vlan" | "device");
    let title = if !creating {
        if !network.is_empty() {
            network
        } else {
            device
        }
    } else {
        match kind {
            "wan" => "New internet connection",
            "bridge" => "New bridge",
            "vlan" => "New VLAN",
            "tunnel" => "New tunnel",
            _ => "New network",
        }
    };
    let name = field(v, e, "name", "Name", "")
        .writes(if is_device { "name" } else { "interface" })
        .explained(
            "Use a unique name without spaces. Other settings refer to this name.",
            "network",
        );
    let mut identity = vec![];
    // A new interface opens as a network, and its Type is the one choice that
    // decides which fields the rest of the drawer has, so changing it draws
    // the drawer again for that kind (reshaped). A tunnel to another site is
    // not one of them: WireGuard and OpenVPN have their own page.
    if creating {
        identity.push(
            Widget::select("kind", "Type", kind, opts(&KINDS), "")
                .reshapes()
                .explained(about(kind), ""),
        );
        identity.push(Widget::text(&format!(
            "A VPN tunnel is set up on [its own page]({ROOT}vpn)."
        )));
    }
    identity.push(name);
    let devices: Vec<SelectOption> = std::iter::once(SelectOption::new("", "Choose a device"))
        .chain(
            m.names()
                .into_iter()
                .filter(|n| n != device)
                .map(|n| SelectOption::new(&n, &n)),
        )
        .collect();
    if kind == "vlan" {
        identity.push(select(v, e, "device", "Runs on", devices));
        identity.push(field(v, e, "vid", "VLAN ID", "").explained(
            "A number from 1 to 4094. Both ends of the cable must use the same tag.",
            "network device",
        ));
        identity.push(
            select(
                v,
                e,
                "vlan_protocol",
                "Tag protocol",
                opts(&[("8021q", "802.1Q"), ("8021ad", "802.1ad")]),
            )
            .writes("type"),
        );
    } else if !is_device && kind != "tunnel" {
        identity.push(select(v, e, "device", "Runs on", devices));
    }
    if !is_device {
        let mut zones = vec![SelectOption::new("", "No zone")];
        zones.extend(
            m.zones
                .iter()
                .map(|z| SelectOption::new(&z.get("name"), &z.get("name"))),
        );
        identity.push(select(v, e, "zone", "Firewall zone", zones));
    }
    if kind == "bridge" {
        let choices = m
            .names()
            .into_iter()
            .filter(|n| n != device && !n.starts_with("pppoe-"))
            .map(|n| SelectOption::new(&n, &n))
            .collect();
        let mut ports = Widget::checks("ports", "Ports", &v.list("ports"), choices).writes("ports");
        if let Widget::Field(Field { error, .. }) = &mut ports {
            *error = e.get("ports").cloned().unwrap_or_default();
        }
        identity.extend([
            ports,
            check(v, "stp", "Spanning tree"),
            check(v, "igmp_snooping", "IGMP snooping"),
        ]);
    }
    // The drawer's title names the object, so its opening fields stand under
    // it with no heading of their own. It is ruled like every other subject:
    // the first draws no rule above itself, and it keeps its air before the
    // next one's.
    let mut sections = vec![section("", "", "identity", identity)];
    if kind == "tunnel" {
        let protocols = opts(&[
            ("gre", "GRE over IPv4"),
            ("gretap", "GRE Ethernet tunnel"),
            ("grev6", "GRE over IPv6"),
            ("vxlan", "VXLAN"),
            ("6in4", "IPv6 over IPv4"),
        ]);
        let mut addressing = vec![select(v, e, "proto", "Protocol", protocols)];
        for proto in ["gre", "gretap", "vxlan", "6in4"] {
            let mut fields = vec![
                field(v, e, "ipaddr", "Local IPv4 address", "ip4addr"),
                field(v, e, "peeraddr", "Remote IPv4 address", "ip4addr"),
            ];
            if proto == "vxlan" {
                fields.extend([
                    field(v, e, "vid", "VXLAN ID", ""),
                    field(v, e, "port", "Destination port", "port"),
                ]);
            }
            if proto == "6in4" {
                fields.extend([
                    field(v, e, "ip6addr", "Tunnel IPv6 address", ""),
                    field(v, e, "ip6prefix", "Routed IPv6 prefix", ""),
                ]);
            }
            addressing.push(when(v, proto, fields));
        }
        addressing.push(when(
            v,
            "grev6",
            vec![
                field(v, e, "ip6addr", "Local IPv6 address", "ip6addr"),
                field(v, e, "peeraddr", "Remote IPv6 address", "ip6addr").writes("peer6addr"),
            ],
        ));
        addressing.push(field(v, e, "ttl", "Time to live", ""));
        sections.push(section(
            "Addressing",
            "Tunnel endpoints must already be reachable through another network.",
            "addressing",
            addressing,
        ));
    } else if !is_device {
        let mut protocols = opts(&[
            ("static", "Static address"),
            ("dhcp", "DHCP client"),
            ("pppoe", "PPPoE"),
            ("dhcpv6", "DHCPv6 client"),
            ("none", "Unmanaged"),
        ]);
        if !protocols.iter().any(|o| o.value == v.get("proto")) {
            protocols.push(SelectOption::new(&v.get("proto"), &v.get("proto")));
        }
        let mut addressing = vec![select(v, e, "proto", "Protocol", protocols)];
        addressing.push(when(
            v,
            "static",
            vec![
                Widget::form_grid(
                    2,
                    vec![
                        field(v, e, "ipaddr", "Address", "ip4addr"),
                        field(v, e, "netmask", "Netmask", "ip4addr"),
                    ],
                )
                .labelled("IPv4 address", ""),
                field(v, e, "gateway", "Gateway", "ip4addr"),
                list(v, e, "dns", "DNS servers", "ipaddr"),
            ],
        ));
        for proto in ["dhcp", "dhcpv6"] {
            addressing.push(when(
                v,
                proto,
                vec![
                    field(v, e, "hostname", "Hostname", "hostname"),
                    check(v, "peerdns", "Use DNS from peer"),
                    check(v, "defaultroute", "Default route"),
                ],
            ));
        }
        let mut password = field(v, e, "password", "Password", "");
        if let Widget::Field(Field {
            kind, value, help, ..
        }) = &mut password
        {
            *kind = "password".into();
            value.clear();
            *help = "Leave empty to keep the existing password.".into();
        }
        addressing.push(when(
            v,
            "pppoe",
            vec![
                field(v, e, "username", "Username", ""),
                password,
                field(v, e, "service", "Service", ""),
                field(v, e, "ac", "Access concentrator", ""),
                field(v, e, "keepalive", "Keepalive", ""),
            ],
        ));
        sections.push(section(
            "Addressing",
            "How it gets its address. Changing this changes the fields below it.",
            "addressing",
            addressing,
        ));
        sections.push(section(
            "IPv6",
            "",
            "ipv6",
            vec![select(
                v,
                e,
                "ip6assign",
                "Delegated prefix",
                opts(&[
                    ("", "No prefix"),
                    ("60", "60-bit"),
                    ("62", "62-bit"),
                    ("64", "64-bit"),
                ]),
            )],
        ));
        // The network's DHCP server is its own object, edited on the DHCP
        // page; here it is stated as it stands, with the way there.
        if let Some(server) = crate::page::dhcp_part(m, network, false) {
            sections.push(section("", "", "dhcp-server", vec![server]));
        }
    }
    let mut advanced = vec![];
    if !is_device {
        advanced.push(check(v, "auto", "Bring up at boot"));
    }
    advanced.push(field(v, e, "mtu", "MTU", ""));
    if !is_device {
        advanced.push(field(v, e, "metric", "Metric", ""));
    }
    advanced.push(field(v, e, "macaddr", "MAC address override", ""));
    sections.push(section("Advanced", "", "advanced", advanced));
    let mut blocks = vec![];
    for op in operations(m, kind, network, device, v) {
        if op.delete || op.config != "network" {
            continue;
        }
        let typ = if op.section_type.is_empty() {
            if is_device {
                "device"
            } else {
                "interface"
            }
        } else {
            &op.section_type
        };
        let values = op.values.as_object().cloned().unwrap_or_default();
        let config_id = m
            .networks
            .iter()
            .chain(m.devices.iter())
            .find(|record| record.id == op.section)
            .map(|record| record.config_id())
            .unwrap_or(&op.section);
        blocks.push(crate::page::config_text(typ, config_id, &values));
    }
    // The form's own footnote: the lines it will write, kept current as it is
    // edited, at the foot of the form behind a hairline.
    sections.push(Widget::config_preview(
        "/etc/config/network",
        &blocks.join("\n\n"),
    ));
    let form = Widget::Form {
        style: String::new(),
        submit: "Save interface".into(),
        // Only a refusal no field owns is the form's; each field refusal rides
        // its field.
        error: error.into(),
        note: String::new(),
        target: String::new(),
        fields: sections,
    };
    // The editor is the object's drawer; closing it leaves the listing's own
    // address.
    RowDrawer {
        title: title.into(),
        open: true,
        closed: ROOT.into(),
        children: vec![form],
        ..Default::default()
    }
}
fn err(e: &mut Errors, k: &str, msg: &str) {
    e.entry(k.into()).or_insert(msg.into());
}
pub(crate) fn uint(v: &str, lo: u32, hi: u32) -> bool {
    !v.is_empty()
        && v.bytes().all(|b| b.is_ascii_digit())
        && v.parse::<u32>().is_ok_and(|n| (lo..=hi).contains(&n))
}
fn valid_name(v: &str, device: bool) -> bool {
    !v.is_empty()
        && v.len() <= 15
        && v.bytes().next().is_some_and(|b| b.is_ascii_lowercase())
        && v.bytes().all(|b| {
            b.is_ascii_lowercase()
                || b.is_ascii_digit()
                || b == b'_'
                || (device && (b == b'-' || b == b'.'))
        })
}
fn valid_mac(s: &str) -> bool {
    let b: Vec<_> = s.split(':').collect();
    b.len() == 6
        && b.iter()
            .all(|x| x.len() == 2 && u8::from_str_radix(x, 16).is_ok())
        && u8::from_str_radix(b[0], 16).is_ok_and(|x| x & 1 == 0)
        && s != "00:00:00:00:00:00"
}
pub(crate) fn mask_bits(s: &str) -> Option<u32> {
    let m = u32::from(s.parse::<Ipv4Addr>().ok()?);
    let bits = m.leading_ones();
    (bits > 0 && m == u32::MAX.checked_shl(32 - bits).unwrap_or(0)).then_some(bits)
}
fn prefix6(s: &str) -> bool {
    s.split_once('/')
        .is_some_and(|(a, p)| a.parse::<Ipv6Addr>().is_ok() && uint(p, 1, 128))
}
fn host(s: &str) -> bool {
    s.parse::<IpAddr>().is_ok()
        || (!s.is_empty()
            && s.len() <= 253
            && s.split('.').all(|p| {
                !p.is_empty()
                    && p.len() <= 63
                    && !p.starts_with('-')
                    && !p.ends_with('-')
                    && p.bytes().all(|b| b.is_ascii_alphanumeric() || b == b'-')
            }))
}
fn depends_on(m: &Model, name: &str, target: &str, seen: &mut BTreeSet<String>) -> bool {
    if name == target {
        return true;
    }
    if !seen.insert(name.into()) {
        return false;
    }
    m.device(name).is_some_and(|device| {
        device
            .list("ports")
            .iter()
            .any(|port| depends_on(m, port, target, seen))
            || (!device.get("ifname").is_empty()
                && depends_on(m, &device.get("ifname"), target, seen))
    })
}
fn validate(m: &Model, kind: &str, network: &str, device: &str, v: &Values) -> Errors {
    let mut e = Errors::new();
    let is_device = matches!(kind, "bridge" | "vlan" | "device");
    let name = v.get("name");
    if name.is_empty() || (name != network && name != device && !valid_name(&name, is_device)) {
        err(
            &mut e,
            "name",
            "Use 1–15 lowercase letters, numbers or underscores, starting with a letter.",
        );
    }
    if is_device {
        if name != device && m.names().contains(&name) {
            err(&mut e, "name", "A device with this name already exists.");
        }
        if !device.is_empty() && name != device {
            err(
                &mut e,
                "name",
                "An existing device keeps its name. Create a new device to use another name.",
            );
        }
    } else if name != network && m.network(&name).is_some() {
        err(&mut e, "name", "A network with this name already exists.");
    }
    if (!is_device && kind != "tunnel") || kind == "vlan" {
        let target = v.get("device");
        if !m.names().contains(&target) || target == name && is_device {
            err(&mut e, "device", "Choose an existing device.");
        }
    }
    if !is_device && kind != "tunnel" {
        let target = v.get("device");
        if m.networks.iter().any(|n| {
            n.id != network
                && n.get("device") == target
                && n.get("proto") == v.get("proto")
                && v.get("proto") != "none"
        }) {
            err(
                &mut e,
                "device",
                "Another network already uses this device with this protocol.",
            );
        }
    }
    if !v.get("zone").is_empty() && !m.zones.iter().any(|z| z.get("name") == v.get("zone")) {
        err(&mut e, "zone", "Choose an existing firewall zone.");
    }
    if kind == "bridge" {
        let ports = v.list("ports");
        if ports.is_empty() {
            err(&mut e, "ports", "Choose at least one port.");
        }
        for p in ports {
            if !m.names().contains(&p) || depends_on(m, &p, &name, &mut BTreeSet::new()) {
                err(
                    &mut e,
                    "ports",
                    "Choose existing ports that do not depend on this bridge.",
                );
            }
            if m.devices
                .iter()
                .any(|d| d.get("name") != device && d.list("ports").contains(&p))
            {
                err(
                    &mut e,
                    "ports",
                    "A selected port already belongs to another bridge.",
                );
            }
        }
    }
    if kind == "vlan" {
        if depends_on(m, &v.get("device"), &name, &mut BTreeSet::new()) {
            err(
                &mut e,
                "device",
                "Choose a device that does not depend on this VLAN.",
            );
        }
        if !uint(&v.get("vid"), 1, 4094) {
            err(&mut e, "vid", "Enter a VLAN ID from 1 to 4094.");
        }
        if !matches!(v.get("vlan_protocol").as_str(), "8021q" | "8021ad") {
            err(&mut e, "vlan_protocol", "Choose a tag protocol.");
        }
        if m.devices.iter().any(|d| {
            d.get("name") != device
                && d.get("ifname") == v.get("device")
                && d.get("vid") == v.get("vid")
                && d.get("type") == v.get("vlan_protocol")
        }) {
            err(
                &mut e,
                "vid",
                "This tag already exists on the selected device.",
            );
        }
    }
    if !v.get("mtu").is_empty() && !uint(&v.get("mtu"), 68, 9200) {
        err(&mut e, "mtu", "Enter an MTU from 68 to 9200.");
    }
    if !v.get("metric").is_empty() && !uint(&v.get("metric"), 0, u32::MAX) {
        err(&mut e, "metric", "Enter a non-negative integer.");
    }
    if !v.get("macaddr").is_empty() && !valid_mac(&v.get("macaddr")) {
        err(
            &mut e,
            "macaddr",
            "Enter a unicast MAC address, such as 02:11:22:33:44:55.",
        );
    }
    if is_device {
        return e;
    }
    let proto = v.get("proto");
    if let Some(protocols) = m.live.get("protocols").and_then(Value::as_object) {
        if !protocols.contains_key(&proto)
            && !matches!(proto.as_str(), "static" | "none")
            && m.network(network).is_none_or(|n| n.get("proto") != proto)
        {
            err(&mut e,"proto","This protocol is not installed on the router. Install its netifd support package first.");
        }
    }
    if kind == "tunnel" {
        if !matches!(
            proto.as_str(),
            "gre" | "gretap" | "grev6" | "vxlan" | "6in4"
        ) {
            err(&mut e, "proto", "Choose a supported tunnel protocol.");
        }
        let remote = v.get("peeraddr");
        if if proto == "grev6" {
            remote.parse::<Ipv6Addr>().is_err()
        } else {
            remote.parse::<Ipv4Addr>().is_err()
        } {
            err(&mut e, "peeraddr", "Enter the remote tunnel address.");
        }
        if proto == "grev6" {
            if !v.get("ip6addr").is_empty() && v.get("ip6addr").parse::<Ipv6Addr>().is_err() {
                err(&mut e, "ip6addr", "Enter a valid IPv6 address.");
            }
        } else if !v.get("ipaddr").is_empty() && v.get("ipaddr").parse::<Ipv4Addr>().is_err() {
            err(&mut e, "ipaddr", "Enter a valid IPv4 address.");
        }
        if proto == "vxlan" {
            if !uint(&v.get("vid"), 1, 16777215) {
                err(&mut e, "vid", "Enter a VXLAN ID from 1 to 16777215.");
            }
            if !v.get("port").is_empty() && !uint(&v.get("port"), 1, 65535) {
                err(&mut e, "port", "Enter a port from 1 to 65535.");
            }
        }
        if proto == "6in4" {
            if !prefix6(&v.get("ip6addr")) {
                err(
                    &mut e,
                    "ip6addr",
                    "Enter an IPv6 address with its prefix length.",
                );
            }
            if !v.get("ip6prefix").is_empty() && !prefix6(&v.get("ip6prefix")) {
                err(&mut e, "ip6prefix", "Enter a valid IPv6 prefix.");
            }
        }
        if !v.get("ttl").is_empty() && !uint(&v.get("ttl"), 1, 255) {
            err(&mut e, "ttl", "Enter a value from 1 to 255.");
        }
        return e;
    }
    if !matches!(
        proto.as_str(),
        "static" | "dhcp" | "dhcpv6" | "pppoe" | "none"
    ) && m.network(network).is_none_or(|n| n.get("proto") != proto)
    {
        err(&mut e, "proto", "Choose an available protocol.");
    }
    if proto == "static" {
        let addr = v.get("ipaddr").parse::<Ipv4Addr>();
        let bits = mask_bits(&v.get("netmask"));
        if addr.is_err() {
            err(&mut e, "ipaddr", "Enter a valid IPv4 address.");
        }
        if bits.is_none() {
            err(&mut e, "netmask", "Enter a contiguous IPv4 netmask.");
        }
        if let (Ok(addr), Some(bits)) = (addr, bits) {
            let ip = u32::from(addr);
            let mask = u32::MAX << (32 - bits);
            let host = ip & !mask;
            if bits < 31 && (host == 0 || host == !mask) {
                err(
                    &mut e,
                    "ipaddr",
                    "Use a host address, not the network or broadcast address.",
                );
            }
            if m.networks
                .iter()
                .any(|n| n.id != network && n.get("ipaddr") == v.get("ipaddr"))
            {
                err(
                    &mut e,
                    "ipaddr",
                    "Another network already uses this address.",
                );
            }
            let gw = v.get("gateway");
            if !gw.is_empty() {
                match gw.parse::<Ipv4Addr>() {
                    Ok(g) if u32::from(g) & mask == ip & mask && g != addr => {}
                    _ => err(
                        &mut e,
                        "gateway",
                        "Use a different address in this subnet, or leave the gateway empty.",
                    ),
                }
            }
        }
        for dns in v.list("dns") {
            if dns.parse::<IpAddr>().is_err() {
                err(&mut e, "dns", "Enter valid IP addresses for DNS servers.");
            }
        }
    }
    if matches!(proto.as_str(), "dhcp" | "dhcpv6")
        && !v.get("hostname").is_empty()
        && !host(&v.get("hostname"))
    {
        err(&mut e, "hostname", "Enter a valid hostname.");
    }
    if proto == "pppoe" {
        if v.get("username").is_empty() {
            err(
                &mut e,
                "username",
                "Enter the username supplied by your ISP.",
            );
        }
        if v.get("password").is_empty()
            && m.network(network)
                .is_none_or(|n| n.get("password").is_empty())
        {
            err(
                &mut e,
                "password",
                "Enter the password supplied by your ISP.",
            );
        }
        let keep = v.get("keepalive");
        if !keep.is_empty()
            && (keep.split_whitespace().count() != 2
                || !keep.split_whitespace().all(|n| uint(n, 1, 65535)))
        {
            err(
                &mut e,
                "keepalive",
                "Enter a failure count and interval, such as 5 60.",
            );
        }
    }
    // A network handing out addresses needs one of its own to hand out from;
    // turning the server off is the DHCP page's.
    if proto != "static" && m.dhcp(network).is_some_and(|d| d.get("ignore") != "1") {
        err(
            &mut e,
            "proto",
            "This network hands out addresses. Turn its DHCP server off on the DHCP page first.",
        );
    }
    if !matches!(v.get("ip6assign").as_str(), "" | "60" | "62" | "64") {
        err(&mut e, "ip6assign", "Choose a delegated prefix length.");
    }
    e
}
fn nullable(s: String) -> Value {
    if s.is_empty() {
        Value::Null
    } else {
        Value::String(s)
    }
}
fn named(config: &str, typ: &str, name: &str, values: Value) -> CommitOp {
    let mut op = commit_new(config, typ, values);
    op.section = name.into();
    op
}
fn relationships(m: &Model, old: &str, name: &str, zone: &str, ops: &mut Vec<CommitOp>) {
    for z in &m.zones {
        let before = z.list("network");
        let mut after: Vec<String> = before
            .iter()
            .filter(|n| *n != old && *n != name)
            .cloned()
            .collect();
        if z.get("name") == zone && !name.is_empty() {
            after.push(name.into());
        }
        if after != before {
            ops.push(commit(
                "firewall",
                &z.id,
                json!({"network":if after.is_empty(){Value::Null}else{json!(after)}}),
            ));
        }
    }
    if old != name && !old.is_empty() {
        for w in &m.wireless {
            let before = w.list("network");
            if before.iter().any(|n| n == old) {
                let after: Vec<_> = before
                    .iter()
                    .filter_map(|n| {
                        if n == old {
                            (!name.is_empty()).then_some(name.to_string())
                        } else {
                            Some(n.clone())
                        }
                    })
                    .collect();
                ops.push(commit("wireless", &w.id, json!({"network":after})));
            }
        }
        for r in &m.routes {
            if r.get("interface") == old {
                ops.push(if name.is_empty() {
                    commit_delete("network", &r.id)
                } else {
                    commit("network", &r.id, json!({"interface":name}))
                });
            }
        }
    }
}
pub fn save(m: &Model, kind: &str, network: &str, device: &str, f: &Form) -> Envelope {
    let creating = network.is_empty() && device.is_empty();
    let kind = if creating {
        if !creatable(kind) {
            return crate::missing();
        }
        if f.get("_action") == "reshape" {
            return reshaped(m, kind, f);
        }
        kind.into()
    } else {
        let Some(k) = kind_of(m, network, device) else {
            return crate::missing();
        };
        k
    };
    let v = Values::posted(f);
    let errors = validate(m, &kind, network, device, &v);
    if !errors.is_empty() {
        return page(m, &kind, network, device, &v, &errors, "");
    }
    let ops = operations(m, &kind, network, device, &v);
    // The shell follows Back only after every declarative write has staged.
    page(m, &kind, network, device, &v, &Errors::new(), "")
        .with_commit(ops)
        .with_notice(Tone::Success, "Interface saved.")
}
// The staged write and the live preview share this projection. Rendering an
// unfinished form may show unfinished values; only save's validation can stage it.
fn operations(m: &Model, kind: &str, network: &str, device: &str, v: &Values) -> Vec<CommitOp> {
    let creating = network.is_empty() && device.is_empty();
    let name = v.get("name");
    let is_device = matches!(kind, "bridge" | "vlan" | "device");
    let mut ops = vec![];
    let existing = if is_device {
        m.device(device)
    } else {
        m.network(network)
    };
    let mut values = existing.map(|n| n.values.clone()).unwrap_or_default();
    for k in ["mtu", "macaddr"] {
        values.insert(k.into(), nullable(v.get(k)));
    }
    if is_device {
        values.insert("name".into(), json!(name));
        if kind == "bridge" {
            values.insert("type".into(), json!("bridge"));
            values.insert("ports".into(), json!(v.list("ports")));
            for k in ["stp", "igmp_snooping"] {
                values.insert(k.into(), json!(if v.get(k) == "1" { "1" } else { "0" }));
            }
        }
        if kind == "vlan" {
            values.insert("type".into(), json!(v.get("vlan_protocol")));
            values.insert("ifname".into(), json!(v.get("device")));
            values.insert("vid".into(), json!(v.get("vid")));
        }
        ops.push(if let Some(existing) = existing {
            commit("network", &existing.id, json!(values))
        } else {
            commit_new("network", "device", json!(values))
        });
    } else {
        let proto = v.get("proto");
        let changed_proto = existing.is_some_and(|n| n.get("proto") != proto);
        if changed_proto {
            for k in [
                "ipaddr",
                "netmask",
                "gateway",
                "dns",
                "username",
                "password",
                "service",
                "ac",
                "keepalive",
                "hostname",
                "peerdns",
                "defaultroute",
                "peeraddr",
                "peer6addr",
                "ip6addr",
                "ip6prefix",
                "vid",
                "port",
                "ttl",
            ] {
                values.insert(k.into(), Value::Null);
            }
        }
        values.insert("proto".into(), json!(proto));
        values.insert(
            "auto".into(),
            json!(if v.get("auto") == "1" { "1" } else { "0" }),
        );
        values.insert("metric".into(), nullable(v.get("metric")));
        if kind == "tunnel" {
            for k in [
                "ipaddr",
                "peeraddr",
                "ip6addr",
                "ip6prefix",
                "vid",
                "port",
                "ttl",
            ] {
                values.insert(k.into(), nullable(v.get(k)));
            }
            if proto == "grev6" {
                values.insert("peer6addr".into(), nullable(v.get("peeraddr")));
                values.insert("peeraddr".into(), Value::Null);
            }
        } else {
            values.insert("device".into(), json!(v.get("device")));
            values.insert("ip6assign".into(), nullable(v.get("ip6assign")));
            match proto.as_str() {
                "static" => {
                    for k in ["ipaddr", "netmask", "gateway"] {
                        values.insert(k.into(), nullable(v.get(k)));
                    }
                    values.insert(
                        "dns".into(),
                        if v.list("dns").is_empty() {
                            Value::Null
                        } else {
                            json!(v.list("dns"))
                        },
                    );
                }
                "dhcp" | "dhcpv6" => {
                    values.insert("hostname".into(), nullable(v.get("hostname")));
                    for k in ["peerdns", "defaultroute"] {
                        values.insert(k.into(), json!(if v.get(k) == "1" { "1" } else { "0" }));
                    }
                }
                "pppoe" => {
                    for k in ["username", "service", "ac", "keepalive"] {
                        values.insert(k.into(), nullable(v.get(k)));
                    }
                    if !v.get("password").is_empty() {
                        values.insert("password".into(), json!(v.get("password")));
                    }
                }
                _ => {}
            }
        }
        if creating || name != network {
            values.retain(|_, v| !v.is_null());
            ops.push(named("network", "interface", &name, json!(values)));
        } else {
            ops.push(commit("network", network, json!(values)));
        }
        relationships(m, network, &name, &v.get("zone"), &mut ops);
        // A new network starts with a DHCP server of the daemon's defaults,
        // named after it as OpenWrt names its own; one renamed keeps its
        // server, which follows the new name. Editing the server is the DHCP
        // page's.
        if creating && kind == "network" {
            ops.push(named(
                "dhcp",
                "dhcp",
                &name,
                json!({"interface": name, "start": "100", "limit": "150", "leasetime": "12h"}),
            ));
        } else if let Some(old) = m.dhcp(network).filter(|_| name != network) {
            ops.push(commit("dhcp", &old.id, json!({"interface": name})));
        }
        if !creating && name != network {
            ops.push(commit_delete("network", network));
        }
    }
    ops
}
fn delete_blocked(m: &Model, network: &str, device: &str) -> bool {
    if !network.is_empty() {
        return network == "loopback" || m.network(network).is_none();
    }
    m.device(device).is_none()
        || m.networks.iter().any(|n| n.get("device") == device)
        || m.devices.iter().any(|d| {
            d.get("name") != device
                && (d.get("ifname") == device || d.list("ports").contains(&device.to_string()))
        })
}
pub fn delete_page(m: &Model, network: &str, device: &str) -> Envelope {
    if kind_of(m, network, device).is_none() {
        return crate::missing();
    }
    if delete_blocked(m, network, device) {
        return Envelope::page(
            "Delete interface",
            Widget::callout(
                Tone::Warning,
                "",
                "This device is still in use. Remove its network, bridge or VLAN references first.",
            ),
        )
        .with_back("Interfaces", ROOT);
    }
    Envelope::page("Delete interface",Widget::Form{style:"page".into(),submit:String::new(),error:String::new(),note:String::new(),target:String::new(),fields:vec![Widget::hidden("delete","1"),Widget::code("Name",if network.is_empty(){device}else{network}),Widget::Confirm{trigger:"Delete interface".into(),title:"Delete interface?".into(),message:"Its DHCP settings and network references will also be removed. The change takes effect when you apply.".into(),confirm:"Delete interface".into(),cancel:"Cancel".into()}]}).with_back("Interfaces",ROOT).with_width("narrow")
}
pub fn delete(m: &Model, network: &str, device: &str, f: &Form) -> Envelope {
    if f.get("delete") != "1" || delete_blocked(m, network, device) {
        return delete_page(m, network, device);
    }
    let mut ops = vec![];
    if !network.is_empty() {
        relationships(m, network, "", "", &mut ops);
        for d in &m.dhcp {
            if d.get("interface") == network {
                ops.push(commit_delete("dhcp", &d.id));
            }
        }
        ops.push(commit_delete("network", network));
    } else if let Some(d) = m.device(device) {
        ops.push(commit_delete("network", &d.id));
    }
    Envelope::page("Interfaces", Widget::stack(vec![]))
        .with_back("Interfaces", ROOT)
        .with_commit(ops)
        .with_notice(Tone::Success, "Interface removed.")
}
pub fn action(m: &Model, f: &Form) -> Envelope {
    for (action, notice) in [
        ("restart", "Interface restart requested."),
        ("up", "Interface start requested."),
        ("down", "Interface stop requested."),
    ] {
        let id = f.get(action);
        if id.is_empty() {
            continue;
        }
        if m.network(&id).is_none() || id == "loopback" || m.live_network(&id).is_none() {
            return crate::missing();
        }
        let mut page = crate::page::listing(m)
            .with_back("Interfaces", ROOT)
            .with_notice(Tone::Success, notice);
        page.commands = vec![verso_plugin::ApplyAction {
            name: format!("interface-{action}"),
            args: BTreeMap::from([("interface".into(), id)]),
        }];
        return page;
    }
    crate::missing()
}
