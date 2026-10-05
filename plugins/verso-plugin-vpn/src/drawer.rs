// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

//! One OpenVPN instance, opened beside the listing.
//!
//! It answers three things in order: what the tunnel is doing now, what its
//! section says (and lets that change), and what its profile says. The
//! profile is the provider's file and stays as they wrote it, so it is read
//! out rather than edited; both files close the panel, as the router holds
//! them.

use verso_plugin::{commit, json, uci_text, CommitOp, Form, Property, RowDrawer, Widget};

use crate::listing::{self, bytes};
use crate::model::{Instance, Profile, CONFIG};

const CONFIG_PATH: &str = "/etc/config/openvpn";
const PROFILE_TITLE: &str = "What the profile says";
const ENABLED_HELP: &str = "OpenVPN starts the tunnel when the router boots, and keeps it up.";

/// PANEL marks a submission as the panel's own form, which states every
/// control on it, rather than a row's start or stop.
pub const PANEL: &str = "panel";

/// drawer is the panel: one form, so Save closes it under everything it
/// saves — the tunnel's state, its switch, the profile read out, and the
/// section this writes, previewed as it changes, beside the profile file.
pub fn drawer(instance: &Instance, now: u64) -> RowDrawer {
    let mut fields = vec![
        Widget::properties(facts(instance, now)),
        Widget::switch_keyed(
            "enabled",
            "Start with the router",
            "enabled",
            ENABLED_HELP,
            instance.enabled,
        )
        .at(CONFIG, &instance.name),
    ];
    if let Some(profile) = &instance.profile {
        let mut items = vec![mono("Profile", &instance.config)];
        items.extend(reading(profile));
        fields.push(Widget::section(PROFILE_TITLE, "", vec![Widget::properties(items)]).ruled());
    }
    fields.push(Widget::config_preview(
        CONFIG_PATH,
        &uci_text("openvpn", &instance.name, &instance.values),
    ));
    if let Some(profile) = &instance.profile {
        fields.push(Widget::code(&instance.config, &profile.text));
    }
    fields.push(Widget::hidden(PANEL, "1"));
    RowDrawer {
        title: instance.name.clone(),
        closed: "/plugins/vpn/".into(),
        open: true,
        children: vec![Widget::form("Save", fields).at(CONFIG, &instance.name)],
        ..Default::default()
    }
}

/// facts is what the tunnel is doing now, as far as the helper could read.
fn facts(instance: &Instance, now: u64) -> Vec<Property> {
    let (state, tone) = listing::instance_state(instance);
    let mut out = vec![Property {
        label: "State".into(),
        value: state.into(),
        dot: if tone.is_empty() {
            "neutral".into()
        } else {
            tone.into()
        },
        ..Default::default()
    }];
    if let (true, Some(since)) = (instance.enabled, instance.live.since) {
        if instance.live.state == "connected" && now >= since {
            out.push(verbatim("Connected for", &duration(now - since)));
        }
    }
    if !instance.live.server.is_empty() {
        out.push(mono("Server", &instance.live.server));
    }
    if !instance.live.address.is_empty() {
        out.push(mono("Tunnel address", &instance.live.address));
    }
    if !instance.live.device.is_empty() {
        out.push(mono("Device", &instance.live.device));
    }
    if !instance.network.is_empty() {
        out.push(mono("Network", &instance.network));
    }
    if let Some(t) = &instance.traffic {
        out.push(verbatim(
            "Traffic",
            &format!("↓ {} · ↑ {}", bytes(t.rx), bytes(t.tx)),
        ));
    }
    out
}

/// save states the panel's submission as the write the shell stages.
/// OpenVPN runs an instance only when it says `enabled 1`, so off is written
/// out as well.
pub fn save(instance: &Instance, form: &Form) -> CommitOp {
    let enabled = if form.get("enabled") == "on" {
        "1"
    } else {
        "0"
    };
    commit(CONFIG, &instance.name, json!({ "enabled": enabled }))
}

/// reading is the profile read out: where it connects, how, and what it sends
/// through the tunnel.
fn reading(profile: &Profile) -> Vec<Property> {
    let mut out = Vec::new();
    if !profile.remotes.is_empty() {
        let servers: Vec<String> = profile.remotes.iter().map(|r| r.endpoint()).collect();
        let mut servers = mono("Servers", &servers.join(", "));
        if profile.random && profile.remotes.len() > 1 {
            servers.help = "Tried in random order.".into();
        }
        out.push(servers);
    }
    if !profile.proto.is_empty() {
        out.push(mono("Protocol", &profile.proto));
    }
    if !profile.cipher.is_empty() {
        out.push(mono("Encryption", &profile.cipher));
    }
    if profile.verify == "server" {
        out.push(words(
            "Server check",
            "The server must prove it holds the provider's certificate.",
        ));
    }
    if !profile.blocks.is_empty() {
        let mut keys = mono("Keys", &profile.blocks.join(", "));
        keys.help = "Inside the profile.".into();
        out.push(keys);
    }
    out.push(words(
        "Traffic",
        match (profile.redirect, profile.nopull) {
            (true, _) => "Everything goes through the VPN.",
            (false, true) => "Only the tunnel's own network goes through the VPN.",
            (false, false) => "What the server asks for goes through the VPN.",
        },
    ));
    out
}

fn mono(label: &str, value: &str) -> Property {
    Property {
        label: label.into(),
        value: value.into(),
        mono: true,
        ..Default::default()
    }
}

fn verbatim(label: &str, value: &str) -> Property {
    Property {
        label: label.into(),
        value: value.into(),
        verbatim: true,
        ..Default::default()
    }
}

fn words(label: &str, value: &str) -> Property {
    Property {
        label: label.into(),
        value: value.into(),
        ..Default::default()
    }
}

/// duration is a stretch of time the way a person says it, to its two
/// largest units.
pub fn duration(seconds: u64) -> String {
    let (d, h, m) = (seconds / 86400, seconds % 86400 / 3600, seconds % 3600 / 60);
    match (d, h) {
        (0, 0) => format!("{} min", m.max(1)),
        (0, _) => format!("{h} h {m} min"),
        _ => format!("{d} d {h} h"),
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::fixture;
    use crate::model::Vpn;

    const NOW: u64 = 1791295321 + 3 * 3600 + 12 * 60;

    fn panel(name: &str) -> serde_json::Value {
        let vpn = Vpn::read(&fixture::request("/"));
        serde_json::to_value(drawer(vpn.instance(name).unwrap(), NOW)).unwrap()
    }

    fn fields(panel: &serde_json::Value) -> Vec<serde_json::Value> {
        panel["children"][0]["fields"].as_array().unwrap().clone()
    }

    fn property<'a>(items: &'a serde_json::Value, label: &str) -> &'a serde_json::Value {
        items
            .as_array()
            .unwrap()
            .iter()
            .find(|p| p["label"] == label)
            .unwrap_or_else(|| panic!("no {label} in {items}"))
    }

    #[test]
    fn a_connected_tunnel_says_what_it_is_doing_first() {
        let panel = panel("proton");
        assert_eq!(panel["title"], "proton");
        assert_eq!(panel["open"], true);
        let facts = &fields(&panel)[0]["items"];
        assert_eq!(property(facts, "State")["value"], "connected");
        assert_eq!(property(facts, "State")["dot"], "success");
        assert_eq!(property(facts, "Connected for")["value"], "3 h 12 min");
        assert_eq!(property(facts, "Server")["value"], "185.107.56.234:1194");
        assert_eq!(property(facts, "Tunnel address")["value"], "10.96.0.14/16");
        assert_eq!(property(facts, "Device")["value"], "tun0");
        assert_eq!(property(facts, "Network")["value"], "vpn");
    }

    #[test]
    fn the_section_is_edited_and_the_profile_is_read_out() {
        let panel = panel("proton");
        // One form, so Save closes the panel under everything it saves.
        assert_eq!(panel["children"].as_array().unwrap().len(), 1);
        assert_eq!(panel["children"][0]["submit"], "Save");
        let fields = fields(&panel);
        assert_eq!(fields[1]["key"], "enabled");
        assert_eq!(fields[1]["on"], true);
        let reading = &fields[2];
        assert_eq!(reading["title"], PROFILE_TITLE);
        let items = &reading["children"][0]["items"];
        assert_eq!(
            property(items, "Profile")["value"],
            "/etc/openvpn/proton.ovpn"
        );
        assert_eq!(
            property(items, "Servers")["value"],
            "185.107.56.234:1194, 185.107.56.234:80, 185.107.56.234:4569"
        );
        assert_eq!(property(items, "Servers")["help"], "Tried in random order.");
        assert_eq!(property(items, "Encryption")["value"], "AES-256-GCM");
        assert_eq!(property(items, "Keys")["value"], "ca, tls-crypt");
        assert_eq!(
            property(items, "Traffic")["value"],
            "Everything goes through the VPN."
        );
    }

    #[test]
    fn both_files_close_the_panel_as_the_router_holds_them() {
        let panel = panel("proton");
        let files = &fields(&panel)[3..5];
        assert_eq!(files[0]["label"], CONFIG_PATH);
        assert_eq!(files[0]["live"], true, "the preview follows the switch");
        assert_eq!(
            files[0]["value"],
            "config openvpn 'proton'\n\toption config '/etc/openvpn/proton.ovpn'\n\toption enabled '1'"
        );
        assert_eq!(files[1]["label"], "/etc/openvpn/proton.ovpn");
        assert!(files[1]["value"]
            .as_str()
            .unwrap()
            .contains("<ca> … 31 lines … </ca>"));
    }

    #[test]
    fn a_stopped_tunnel_has_no_live_facts_to_state() {
        let panel = panel("work");
        let facts = fields(&panel)[0]["items"].as_array().unwrap().clone();
        assert_eq!(facts.len(), 1, "{facts:?}");
        assert_eq!(facts[0]["value"], "off");
    }

    #[test]
    fn save_writes_enabled_both_ways() {
        let vpn = Vpn::read(&fixture::request("/"));
        let proton = vpn.instance("proton").unwrap();
        let on = serde_json::to_value(save(proton, &Form::parse("enabled=on&panel=1"))).unwrap();
        assert_eq!(on["values"], json!({"enabled": "1"}));
        let off = serde_json::to_value(save(proton, &Form::parse("panel=1"))).unwrap();
        assert_eq!(off["values"], json!({"enabled": "0"}));
    }

    #[test]
    fn durations_read_the_way_a_person_says_them() {
        assert_eq!(duration(30), "1 min");
        assert_eq!(duration(3 * 3600 + 12 * 60), "3 h 12 min");
        assert_eq!(duration(2 * 86400 + 6 * 3600), "2 d 6 h");
    }
}
