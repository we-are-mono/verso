// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

//! One OpenVPN instance, opened beside the listing.
//!
//! It answers three things in order: what the tunnel is doing now, what its
//! section says (and lets that change), and what its profile says. The
//! profile is the provider's file and stays as they wrote it, so it is read
//! out rather than edited; both files close the panel, as the router holds
//! them.

use verso_plugin::{
    commit, json, uci_text, CommitOp, Errors, Field, Form, Map, Property, RowDrawer, Value, Widget,
};

use crate::listing::{self, bytes};
use crate::model::{Instance, Profile, CONFIG};

const CONFIG_PATH: &str = "/etc/config/openvpn";
const PROFILE_TITLE: &str = "What the profile says";
const SIGN_IN_TITLE: &str = "Sign-in";
const SIGN_IN_LEDE: &str = "The profile asks for a username and password. A provider's OpenVPN \
     sign-in is often not the one for its website: Proton's, for one, is on its dashboard.";
const ENABLED_HELP: &str = "OpenVPN starts the tunnel when the router boots, and keeps it up.";
const KEEP_HELP: &str = "Leave empty to keep the saved password.";
const MASK: &str = "••••••••";

/// REFUSED is said over a panel that came back with its refusals on it.
pub const REFUSED: &str = "Some values are missing, so nothing was saved. They’re marked below.";

/// PANEL marks a submission as the panel's own form, which states every
/// control on it, rather than a row's start or stop.
pub const PANEL: &str = "panel";

/// Stated is what the panel's controls say: as the section holds it when the
/// panel opens, as typed once it is submitted. A password is never read back
/// onto the page, so it is only ever what was just typed.
pub struct Stated {
    pub enabled: bool,
    pub username: String,
    pub password: String,
}

impl Stated {
    pub fn of(instance: &Instance) -> Stated {
        Stated {
            enabled: instance.enabled,
            username: option(instance, "username"),
            password: String::new(),
        }
    }

    pub fn submitted(form: &Form) -> Stated {
        Stated {
            enabled: form.get("enabled") == "on",
            username: form.get("username").trim().to_string(),
            password: form.get("password"),
        }
    }

    /// writes is what saving this changes in the section. OpenVPN runs an
    /// instance only when it says `enabled 1`, so off is written out too; no
    /// username clears the sign-in whole; an empty password keeps the saved one.
    fn writes(&self) -> Map<String, Value> {
        let mut out = Map::new();
        out.insert(
            "enabled".into(),
            json!(if self.enabled { "1" } else { "0" }),
        );
        if self.username.is_empty() {
            out.insert("username".into(), Value::Null);
            out.insert("password".into(), Value::Null);
        } else {
            out.insert("username".into(), json!(self.username));
            if !self.password.is_empty() {
                out.insert("password".into(), json!(self.password));
            }
        }
        out
    }

    fn validate(&self, instance: &Instance) -> Errors {
        let mut errors = Errors::default();
        let saved = !option(instance, "password").is_empty();
        errors.check(
            "username",
            !self.username.is_empty() || self.password.is_empty(),
            "Enter the username that goes with this password.",
        );
        errors.check(
            "password",
            self.username.is_empty() || saved || !self.password.is_empty(),
            "Enter the password.",
        );
        errors
    }
}

fn option(instance: &Instance, key: &str) -> String {
    instance
        .values
        .get(key)
        .and_then(Value::as_str)
        .unwrap_or_default()
        .to_string()
}

/// drawer is the panel: one form, so Save closes it under everything it
/// saves — the tunnel's state, its switch, its sign-in, the profile read out,
/// and the section this writes, previewed as it changes, beside the profile
/// file.
pub fn drawer(instance: &Instance, now: u64, stated: &Stated, errors: &Errors) -> RowDrawer {
    let mut fields = vec![
        Widget::properties(facts(instance, now)),
        Widget::switch_keyed(
            "enabled",
            "Start with the router",
            "enabled",
            ENABLED_HELP,
            stated.enabled,
        )
        .at(CONFIG, &instance.name),
    ];
    let asks = instance.profile.as_ref().is_some_and(|p| p.asks_sign_in);
    if asks || !stated.username.is_empty() || !option(instance, "username").is_empty() {
        fields.push(sign_in(instance, stated, errors));
    }
    if let Some(profile) = &instance.profile {
        let mut items = vec![mono("Profile", &instance.config)];
        items.extend(reading(profile));
        fields.push(Widget::section(PROFILE_TITLE, "", vec![Widget::properties(items)]).ruled());
    }
    // The section as saving would leave it, with the password standing masked.
    let mut preview = instance.values.clone();
    for (key, value) in stated.writes() {
        match value {
            Value::Null => preview.remove(&key),
            value => preview.insert(key, value),
        };
    }
    if preview.contains_key("password") {
        preview.insert("password".into(), json!(MASK));
    }
    fields.push(Widget::config_preview(
        CONFIG_PATH,
        &uci_text("openvpn", &instance.name, &preview),
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

/// sign_in is the username and password OpenVPN's init hands the tunnel in
/// place of asking, which outrank whatever the profile says about it.
fn sign_in(instance: &Instance, stated: &Stated, errors: &Errors) -> Widget {
    let saved = !option(instance, "password").is_empty();
    let password = Widget::Field(Field {
        name: "password".into(),
        label: "Password".into(),
        kind: "password".into(),
        key: "password".into(),
        help: if saved {
            KEEP_HELP.into()
        } else {
            String::new()
        },
        error: errors.get("password").into(),
        ..Default::default()
    });
    let username = Widget::Field(Field {
        name: "username".into(),
        label: "Username".into(),
        kind: "text".into(),
        key: "username".into(),
        value: stated.username.clone(),
        error: errors.get("username").into(),
        ..Default::default()
    });
    Widget::section(SIGN_IN_TITLE, SIGN_IN_LEDE, vec![username, password]).ruled()
}

/// save states the panel's submission as the write the shell stages, or
/// gives back what was typed with what is missing.
pub fn save(instance: &Instance, form: &Form) -> Result<CommitOp, (Stated, Errors)> {
    let stated = Stated::submitted(form);
    let errors = stated.validate(instance);
    if !errors.is_empty() {
        return Err((stated, errors));
    }
    Ok(commit(
        CONFIG,
        &instance.name,
        Value::Object(stated.writes()),
    ))
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
        let instance = vpn.instance(name).unwrap();
        let stated = Stated::of(instance);
        serde_json::to_value(drawer(instance, NOW, &stated, &Errors::default())).unwrap()
    }

    /// signed_in is proton with a sign-in already saved.
    fn signed_in() -> Instance {
        let mut vpn = Vpn::read(&fixture::request("/"));
        let mut proton = vpn.instances.remove(0);
        proton.values.insert("username".into(), json!("Xk2nQ8+pmp"));
        proton.values.insert("password".into(), json!("hunter2"));
        proton
    }

    fn written(result: Result<CommitOp, (Stated, Errors)>) -> serde_json::Value {
        match result {
            Ok(op) => serde_json::to_value(op).unwrap()["values"].clone(),
            Err((_, errors)) => panic!("refused: {:?}", errors.get("password")),
        }
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
        assert_eq!(fields[2]["title"], SIGN_IN_TITLE);
        let reading = &fields[3];
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
        let files = &fields(&panel)[4..6];
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
        let on = written(save(proton, &Form::parse("enabled=on&panel=1")));
        assert_eq!(on["enabled"], "1");
        let off = written(save(proton, &Form::parse("panel=1")));
        assert_eq!(off["enabled"], "0");
    }

    #[test]
    fn a_profile_that_asks_for_a_sign_in_gets_the_fields_for_one() {
        let proton = panel("proton");
        let sign_in = &fields(&proton)[2]["children"];
        assert_eq!(sign_in[0]["key"], "username");
        assert_eq!(sign_in[1]["key"], "password");
        assert_eq!(sign_in[1]["kind"], "password");
        assert_eq!(sign_in[1]["value"].as_str().unwrap_or_default(), "");
        // work's profile carries its own keys and asks for nothing.
        let work = panel("work");
        assert!(fields(&work).iter().all(|f| f["title"] != SIGN_IN_TITLE));
    }

    #[test]
    fn a_saved_password_is_never_read_back_and_an_empty_one_keeps_it() {
        let proton = signed_in();
        let stated = Stated::of(&proton);
        let body = serde_json::to_value(drawer(&proton, NOW, &stated, &Errors::default())).unwrap();
        let text = body.to_string();
        assert!(
            !text.contains("hunter2"),
            "the password reached the page: {text}"
        );
        let fields = fields(&body);
        assert_eq!(fields[2]["children"][0]["value"], "Xk2nQ8+pmp");
        assert_eq!(fields[2]["children"][1]["help"], KEEP_HELP);
        assert!(fields[4]["value"]
            .as_str()
            .unwrap()
            .contains("option password '••••••••'"));
        let kept = written(save(
            &proton,
            &Form::parse("enabled=on&username=Xk2nQ8%2Bpmp&panel=1"),
        ));
        assert_eq!(kept, json!({"enabled": "1", "username": "Xk2nQ8+pmp"}));
    }

    #[test]
    fn a_new_sign_in_is_written_and_no_username_clears_it() {
        let vpn = Vpn::read(&fixture::request("/"));
        let proton = vpn.instance("proton").unwrap();
        let set = written(save(
            proton,
            &Form::parse("enabled=on&username=me&password=s3cret&panel=1"),
        ));
        assert_eq!(
            set,
            json!({"enabled": "1", "username": "me", "password": "s3cret"})
        );
        let cleared = written(save(&signed_in(), &Form::parse("enabled=on&panel=1")));
        assert_eq!(
            cleared,
            json!({"enabled": "1", "username": null, "password": null})
        );
    }

    #[test]
    fn half_a_sign_in_is_given_back_marked() {
        let vpn = Vpn::read(&fixture::request("/"));
        let proton = vpn.instance("proton").unwrap();
        let Err((stated, errors)) = save(proton, &Form::parse("username=me&panel=1")) else {
            panic!("a username with no password was saved");
        };
        assert_eq!(stated.username, "me");
        assert_eq!(errors.get("password"), "Enter the password.");
        let Err((_, errors)) = save(proton, &Form::parse("password=x&panel=1")) else {
            panic!("a password with no username was saved");
        };
        assert!(!errors.get("username").is_empty());
    }

    #[test]
    fn durations_read_the_way_a_person_says_them() {
        assert_eq!(duration(30), "1 min");
        assert_eq!(duration(3 * 3600 + 12 * 60), "3 h 12 min");
        assert_eq!(duration(2 * 86400 + 6 * 3600), "2 d 6 h");
    }
}
