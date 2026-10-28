// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.
//! The Status card that leads an existing object's drawer: what the router
//! reports about a network or a device, none of it anything the drawer's form
//! sets.
use crate::kind::DeviceKind;
use crate::page::{l3, state};
use crate::Model;
use serde_json::Value;
use std::net::Ipv6Addr;
use verso_plugin::{Property, SectionWidget, Widget, EM_DASH};

fn text(v: Option<&Value>) -> String {
    v.and_then(Value::as_str).unwrap_or("").into()
}
fn fact(label: &str, value: impl Into<String>) -> Property {
    let value = value.into();
    Property {
        label: label.into(),
        value: if value.is_empty() {
            EM_DASH.into()
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
// copied is a fact with its copy act, where it has a value to copy.
fn copied(label: &str, value: impl Into<String>) -> Property {
    let p = fact(label, value);
    Property {
        copy: p.value != EM_DASH,
        ..p
    }
}
/// addresses are the addresses netifd reports in one family, each with its
/// prefix length, an IPv6 prefix assignment's own address among them.
pub(crate) fn addresses(live: Option<&Value>, family: &str) -> Vec<String> {
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
/// uptime is how long netifd says the network has been up, in seconds for
/// the shell to write as every duration is written.
pub(crate) fn uptime(live: Option<&Value>) -> Property {
    match live.and_then(|l| l.get("uptime")).and_then(Value::as_u64) {
        Some(seconds) => Property::duration("Uptime", seconds),
        None => fact("Uptime", ""),
    }
}
/// speed is a port's negotiated link speed in bits per second, from netifd's
/// "1000F"; the shell writes it in the reader's language.
pub(crate) fn speed(runtime: Option<&Value>) -> Option<f64> {
    let raw = text(runtime.and_then(|v| v.get("speed")));
    let mbps = raw.trim_end_matches(['F', 'H']).parse::<u32>().ok()?;
    Some(f64::from(mbps) * 1e6)
}
// network_glyph is a network's mark: the globe for a line to the internet
// (brought up by DHCP or PPPoE, or in the wan zone), the network mark for
// one of the router's own.
fn network_glyph(m: &Model, net: &str) -> &'static str {
    let protocol = m.network(net).map(|n| n.get("proto")).unwrap_or_default();
    let wan_zone = m
        .zones
        .iter()
        .any(|z| z.get("name") == "wan" && z.list("network").iter().any(|n| n == net));
    if wan_zone || matches!(protocol.as_str(), "dhcp" | "dhcpv6" | "pppoe" | "pppoa") {
        "globe"
    } else {
        "network"
    }
}
// card is the Status card: the object's glyph, its facts, and the acts that
// run on it now, on its header.
fn card(icon: &str, children: Vec<Widget>, acts: Vec<Widget>) -> Widget {
    let control = (!acts.is_empty()).then(|| {
        Box::new(Widget::Stack {
            width: String::new(),
            compact: false,
            inline: true,
            divided: false,
            flush: false,
            children: acts,
        })
    });
    Widget::Section(SectionWidget {
        title: "Status".into(),
        icon: icon.into(),
        hairline: true,
        anchor: "status".into(),
        control,
        children,
        ..Default::default()
    })
}
// act is one of the header's acts: a button that posts its name and the
// network's with the drawer's form, which runs it there and then.
fn act(label: &str, glyph: &str, name: &str, net: &str) -> Widget {
    Widget::Button {
        label: label.into(),
        icon: glyph.into(),
        style: "act".into(),
        name: name.into(),
        value: net.into(),
        disabled: false,
        loading: false,
        live: false,
    }
}

/// network is a network's Status card: its state (its IPv6 companion's
/// counted in), the addresses it and its companion hold, its uptime, and
/// netifd's refusal when it has one. Restart and Bring down or up stand on
/// its header while netifd runs it.
pub fn network(m: &Model, net: &str) -> Widget {
    let live = m.live_network(net);
    let companion = m.companion(net).map(|c| c.id.clone());
    let mut nets = vec![net.to_string()];
    nets.extend(companion.clone());
    let runtime = m.live.get("devices").and_then(|v| v.get(l3(m, net)));
    let ip6: Vec<String> = nets
        .iter()
        .flat_map(|n| addresses(m.live_network(n), "ipv6-address"))
        .fold(vec![], |mut out, a| {
            if !out.contains(&a) {
                out.push(a);
            }
            out
        });
    let global: Vec<_> = ip6.iter().filter(|a| !ula(a)).cloned().collect();
    let local: Vec<_> = ip6.iter().filter(|a| ula(a)).cloned().collect();
    // The line's state is its own; its IPv6 companion's stands beside it, so a
    // provider that hands out no IPv6 reads as that and not as a line down.
    let mut facts = vec![prose(
        "State",
        state(m, std::slice::from_ref(&nets[0]), runtime, true).0,
    )];
    if let Some(half) = &companion {
        facts.push(prose(
            "IPv6 state",
            state(m, std::slice::from_ref(half), runtime, true).0,
        ));
    }
    facts.extend([
        copied("IPv4", addresses(live, "ipv4-address").join(", ")),
        copied("IPv6", global.join(", ")),
        copied("ULA", local.join(", ")),
        uptime(live),
    ]);
    let mut children = vec![Widget::properties(facts)];
    if let Some(errors) = live
        .and_then(|v| v.get("errors"))
        .and_then(Value::as_array)
        .filter(|e| !e.is_empty())
    {
        children.push(Widget::code(
            "netifd",
            &serde_json::to_string_pretty(errors).unwrap_or_default(),
        ));
    }
    let mut acts = vec![];
    if m.network(net).is_some() && live.is_some() {
        let up = live.and_then(|v| v.get("up")).and_then(Value::as_bool) == Some(true);
        acts.push(act("Restart", "refresh-cw", "restart", net));
        acts.push(if up {
            act("Bring down", "power", "down", net)
        } else {
            act("Bring up", "power", "up", net)
        });
    }
    card(network_glyph(m, net), children, acts)
}

/// device is a device's Status card: its state, its MAC address, a bridge's
/// live ports or a port's link and driver, and its error counts; a device
/// that carries no network says its own addresses.
pub fn device(m: &Model, name: &str) -> Widget {
    let runtime = m.live.get("devices").and_then(|v| v.get(name));
    let kind = DeviceKind::of(m, name, runtime);
    let carries: Vec<String> = m
        .networks
        .iter()
        .filter(|n| l3(m, &n.id) == name)
        .map(|n| n.id.clone())
        .collect();
    // A device something rests on — a network, a bridge it is a port of, a
    // VLAN made on it — is in use, and says "no link" when its cable is out.
    let used = !carries.is_empty()
        || m.devices
            .iter()
            .any(|d| d.get("ifname") == name || d.list("ports").iter().any(|p| p == name))
        || m.names()
            .iter()
            .any(|b| b != name && m.bridge_ports(b).iter().any(|p| p == name));
    let mut facts = vec![
        prose("State", state(m, &carries, runtime, used).0),
        copied("MAC address", text(runtime.and_then(|v| v.get("macaddr")))),
    ];
    if kind == DeviceKind::Bridge {
        facts.push(fact("Ports", m.bridge_ports(name).join(", ")));
    } else {
        facts.push(match speed(runtime) {
            Some(bps) => Property::rate("Link", bps),
            None => fact("Link", ""),
        });
        facts.push(fact("Driver", text(runtime.and_then(|v| v.get("driver")))));
    }
    let stats = runtime.and_then(|v| v.get("statistics"));
    let count = |k: &str| stats.and_then(|v| v.get(k)).and_then(Value::as_u64);
    facts.push(fact(
        "Errors",
        match (count("rx_errors"), count("tx_errors")) {
            (Some(rx), Some(tx)) => format!("RX {rx} · TX {tx}"),
            _ => String::new(),
        },
    ));
    if carries.is_empty() {
        for (label, family) in [("IPv4", "ipv4-address"), ("IPv6", "ipv6-address")] {
            let found = addresses(runtime, family);
            if !found.is_empty() {
                facts.push(copied(label, found.join(", ")));
            }
        }
    }
    card(kind.glyph(), vec![Widget::properties(facts)], vec![])
}
