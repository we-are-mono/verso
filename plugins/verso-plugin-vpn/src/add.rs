// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

//! A new tunnel, set up one of the two ways one is: from the profile a VPN
//! provider hands out, or from a server's details typed in by hand.
//!
//! Both ways are the same panel, opened on the one question of which, and
//! drawn again for the way chosen. Both end in the same place: a profile
//! file, since details typed in are written out as the profile a provider
//! would have handed over, so every tunnel reads, edits and runs alike
//! afterwards. One save stages all of it: the profile beside the others, the
//! section that runs it on the next free tun device, a network on that
//! device, and the network in its zone.

use verso_plugin::{
    files::NEW_FILE_VERSION, json, uci_text, ApplyAction, CommitOp, Errors, Field, Form, Map,
    Property, RowDrawer, SelectOption, Value, Widget,
};

use crate::listing;
use crate::model::{Vpn, CONFIG};

/// NEW is what the address names in place of an instance while the panel
/// for a new tunnel is open.
pub const NEW: &str = "new";
/// ADD marks a submission as that panel's own.
pub const ADD: &str = "add";

pub const TITLE: &str = "Add a VPN";
const WAY_LABEL: &str = "Set it up from";
const FILE: &str = "file";
const DETAILS: &str = "details";
const PROMPT: &str = "Drop your provider's profile here";
const FILE_HELP: &str =
    "The .ovpn file from your provider's OpenVPN downloads. Choose the router or Linux version.";
const HOST_HELP: &str = "The server's name or address, as its operator gave it.";
const CA_HELP: &str = "The certificate the server's own is signed with, beginning \
     -----BEGIN CERTIFICATE-----. It is how the router knows it reached the real server.";
const REDIRECT_HELP: &str =
    "Every device on your network reaches the internet through the VPN, not only the server's own network.";
const MORE: &str = "Client certificate and TLS key";
const CERT_HELP: &str = "If the server checks this router by a certificate of its own.";
const TLS_HELP: &str =
    "If the server asks for one: the static key its operator gave out, beginning \
     -----BEGIN OpenVPN Static key V1-----.";
const ZONE_HELP: &str =
    "The firewall zone the tunnel's network joins. In wan, your devices reach the internet through it.";
const ENABLED_HELP: &str = "OpenVPN starts the tunnel when the router boots, and keeps it up.";
const PROFILE_SIGN_IN: &str = "The profile asks for a username and password. A provider's OpenVPN \
     sign-in is often not the one for its website: Proton's, for one, is on its dashboard.";
const DETAILS_SIGN_IN: &str = "If the server asks for a username and password.";
const DIR: &str = "/etc/openvpn/";
const LIMIT: usize = 32768;
/// BLOCKS are the key blocks a profile may carry inline, and the directives
/// that name the same keys as files beside it.
const BLOCKS: [&str; 8] = [
    "ca",
    "cert",
    "key",
    "tls-auth",
    "tls-crypt",
    "tls-crypt-v2",
    "secret",
    "pkcs12",
];

pub fn href() -> String {
    listing::href(NEW)
}

/// Draft is what the panel says: the way chosen, what that way asks, and the
/// choices both ways share.
pub struct Draft {
    pub way: String,
    // A profile file.
    pub text: String,
    pub file: String,
    // A server's details.
    pub host: String,
    pub port: String,
    pub proto: String,
    pub ca: String,
    pub cert: String,
    pub key: String,
    pub tls_key: String,
    pub tls_kind: String,
    pub redirect: bool,
    // Both.
    pub name: String,
    pub username: String,
    pub password: String,
    pub zone: String,
    pub enabled: bool,
}

impl Draft {
    pub fn blank() -> Draft {
        Draft {
            way: FILE.into(),
            text: String::new(),
            file: String::new(),
            host: String::new(),
            port: "1194".into(),
            proto: "udp".into(),
            ca: String::new(),
            cert: String::new(),
            key: String::new(),
            tls_key: String::new(),
            tls_kind: "tls-crypt".into(),
            redirect: true,
            name: String::new(),
            username: String::new(),
            password: String::new(),
            zone: "wan".into(),
            enabled: true,
        }
    }

    /// submitted reads the panel. A name not yet shown, or still the one a
    /// file or server suggested when that file or server changes, follows
    /// the suggestion; a name typed is kept.
    pub fn submitted(form: &Form, vpn: &Vpn) -> Draft {
        let blank = Draft::blank();
        let given = |key: &str, default: &str| match form.all(key).is_empty() {
            true => default.to_string(),
            false => form.get(key).trim().to_string(),
        };
        let way = match form.get("way").as_str() {
            DETAILS => DETAILS,
            _ => FILE,
        };
        let file = form.get("profile_name");
        let host = form.get("host").trim().to_string();
        let source = if way == DETAILS { &host } else { &file };
        let named_from = form.get("named_from");
        let typed = form.get("name").trim().to_string();
        let name = if form.all("name").is_empty()
            || (suggested(&typed, &named_from) && named_from != *source)
        {
            free(&suggest(source), vpn)
        } else {
            typed
        };
        // A switch is on until the panel has shown it; then as ticked, which
        // posts "1", or not, which posts nothing.
        let shown = !form.all("named_from").is_empty();
        let switch = |key: &str| !shown || !form.get(key).is_empty();
        Draft {
            way: way.into(),
            text: form.get("profile"),
            file,
            port: given("port", &blank.port),
            proto: given("proto", &blank.proto),
            ca: form.get("ca").trim().to_string(),
            cert: form.get("cert").trim().to_string(),
            key: form.get("key").trim().to_string(),
            tls_key: form.get("tls_key").trim().to_string(),
            tls_kind: given("tls_kind", &blank.tls_kind),
            redirect: switch("redirect"),
            host,
            name,
            username: form.get("username").trim().to_string(),
            password: form.get("password"),
            zone: given("zone", &blank.zone),
            enabled: switch("enabled"),
        }
    }

    fn details(&self) -> bool {
        self.way == DETAILS
    }

    /// source is what the name is suggested from: the file, or the server.
    fn source(&self) -> &str {
        if self.details() {
            &self.host
        } else {
            &self.file
        }
    }

    /// ready is whether the way has been answered far enough to ask the rest.
    fn ready(&self) -> bool {
        self.details() || !self.text.is_empty()
    }

    /// profile is the file the tunnel runs from: the one dropped in, or the
    /// one written from the details.
    pub fn profile(&self) -> String {
        if !self.details() {
            return self.text.clone();
        }
        let mut lines = vec![
            "client".to_string(),
            "dev tun".into(),
            format!("proto {}", self.proto),
            format!("remote {} {}", self.host, self.port),
            "resolv-retry infinite".into(),
            "nobind".into(),
            "persist-key".into(),
            "persist-tun".into(),
            "remote-cert-tls server".into(),
        ];
        if !self.username.is_empty() {
            lines.push("auth-user-pass".into());
        }
        if self.redirect {
            lines.push("redirect-gateway def1".into());
        }
        if !self.tls_key.is_empty() && self.tls_kind == "tls-auth" {
            lines.push("key-direction 1".into());
        }
        lines.push("verb 3".into());
        let mut block = |name: &str, body: &str| {
            if !body.is_empty() {
                lines.push(format!("<{name}>\n{body}\n</{name}>"));
            }
        };
        block("ca", &self.ca);
        block("cert", &self.cert);
        block("key", &self.key);
        let kind = if self.tls_kind == "tls-auth" {
            "tls-auth"
        } else {
            "tls-crypt"
        };
        block(kind, &self.tls_key);
        lines.join("\n") + "\n"
    }
}

/// free is a suggested name nothing on the router has yet: the suggestion,
/// or it with the first number after it that is free (vpn, vpn2, vpn3), so a
/// name the panel offers is never one it then refuses.
fn free(base: &str, vpn: &Vpn) -> String {
    let taken = |n: &str| vpn.instance(n).is_some() || vpn.taken.iter().any(|t| t == n);
    if !taken(base) {
        return base.into();
    }
    (2..)
        .map(|n| {
            let suffix = n.to_string();
            let stem: String = base.chars().take(12 - suffix.len()).collect();
            format!("{stem}{suffix}")
        })
        .find(|candidate| !taken(candidate))
        .unwrap_or_default()
}

/// suggested is whether a name is still what the panel offered for a source,
/// numbered on or not, rather than one typed over it.
fn suggested(name: &str, source: &str) -> bool {
    let base = suggest(source);
    let stem = name.trim_end_matches(|c: char| c.is_ascii_digit());
    name == base || (stem.len() < name.len() && !stem.is_empty() && base.starts_with(stem))
}

/// suggest is a tunnel's name from its file's, or its server's: what uci and
/// netifd take in a section name, short enough for a network, "vpn" when
/// nothing is left.
pub fn suggest(source: &str) -> String {
    let stem = source.split('.').next().unwrap_or("");
    let name: String = stem
        .to_ascii_lowercase()
        .chars()
        .map(|c| if c.is_ascii_alphanumeric() { c } else { '_' })
        .collect::<String>()
        .trim_matches('_')
        .chars()
        .take(12)
        .collect();
    match name.trim_end_matches('_') {
        "" => "vpn".into(),
        n if n.starts_with(|c: char| c.is_ascii_digit()) => {
            format!("vpn_{n}").chars().take(12).collect()
        }
        n => n.into(),
    }
}

/// Read is what a profile says that the panel needs to know.
#[derive(Default, Debug)]
pub struct Read {
    pub client: bool,
    pub proto: String,
    pub remotes: Vec<String>,
    pub asks_sign_in: bool,
    /// Key files the profile names beside it rather than carrying inline.
    pub missing: Vec<String>,
}

/// read is a profile as the panel judges it.
pub fn read(text: &str) -> Read {
    let mut out = Read::default();
    let mut inline = Vec::new();
    let mut open: Option<String> = None;
    let mut named = Vec::new();
    for line in text.lines() {
        let trimmed = line.trim();
        if let Some(name) = &open {
            if trimmed == format!("</{name}>") {
                open = None;
            }
            continue;
        }
        if let Some(name) = trimmed.strip_prefix('<').and_then(|t| t.strip_suffix('>')) {
            if BLOCKS.contains(&name) || name == "auth-user-pass" {
                inline.push(name.to_string());
                open = Some(name.to_string());
                continue;
            }
        }
        let mut words = trimmed.split_whitespace();
        let directive = words.next().unwrap_or("");
        let rest: Vec<&str> = words.collect();
        match directive {
            "client" | "tls-client" => out.client = true,
            "proto" => out.proto = rest.first().unwrap_or(&"").to_string(),
            "remote" => {
                if let Some(host) = rest.first() {
                    out.remotes.push(match rest.get(1) {
                        Some(port) => format!("{host}:{port}"),
                        None => host.to_string(),
                    });
                }
            }
            "auth-user-pass" => out.asks_sign_in = true,
            d if BLOCKS.contains(&d) => {
                if let Some(file) = rest.first().filter(|f| **f != "[inline]") {
                    named.push((d.to_string(), file.to_string()));
                }
            }
            _ => {}
        }
    }
    if inline.iter().any(|b| b == "auth-user-pass") {
        out.asks_sign_in = false;
    }
    out.missing = named
        .into_iter()
        .filter(|(d, _)| !inline.contains(d))
        .map(|(_, f)| f)
        .collect();
    out
}

/// drawer is the panel for this draft: the way, then what that way asks,
/// then what both share.
pub fn drawer(vpn: &Vpn, draft: &Draft, errors: &Errors) -> RowDrawer {
    let way = Widget::select(
        "way",
        WAY_LABEL,
        &draft.way,
        vec![
            SelectOption::new(FILE, "A profile file"),
            SelectOption::new(DETAILS, "The server's details"),
        ],
        "",
    )
    .reshapes();
    let mut fields = vec![way, Widget::hidden(ADD, "1")];
    if draft.details() {
        fields.extend(details(draft, errors));
    } else {
        fields.push(profile_field(draft, errors));
        if draft.ready() {
            fields.push(Widget::properties(found(&read(&draft.text))));
        }
    }
    if draft.ready() {
        fields.push(Widget::hidden("named_from", draft.source()));
        fields.push(text_field(
            "name",
            "Name",
            "openvpn",
            &draft.name,
            "",
            errors,
        ));
        let asks = draft.details() || read(&draft.text).asks_sign_in;
        if asks {
            fields.push(sign_in(draft, errors));
        }
        fields.push(zone(vpn, draft, errors));
        fields.push(Widget::switch_keyed(
            "enabled",
            "Start with the router",
            "enabled",
            ENABLED_HELP,
            draft.enabled,
        ));
        fields.push(Widget::config_preview(
            "What this writes",
            &preview(vpn, draft),
        ));
    }
    RowDrawer {
        title: TITLE.into(),
        closed: "/plugins/vpn/".into(),
        open: true,
        children: vec![Widget::Form {
            style: String::new(),
            submit: "Add".into(),
            error: String::new(),
            fields,
            note: String::new(),
            target: String::new(),
        }],
        ..Default::default()
    }
}

fn profile_field(draft: &Draft, errors: &Errors) -> Widget {
    Widget::Field(Field {
        name: "profile".into(),
        label: "Profile".into(),
        kind: "file".into(),
        style: "text".into(),
        accept: ".ovpn,.conf".into(),
        prompt: PROMPT.into(),
        value: draft.text.clone(),
        chosen: draft.file.clone(),
        help: FILE_HELP.into(),
        error: errors.get("profile").into(),
        reshapes: true,
        ..Default::default()
    })
}

/// details is what a server needs from the router: where it is, how to
/// reach it, the certificate that proves it is itself, and whether the whole
/// network goes through it. What only some servers ask is folded away.
fn details(draft: &Draft, errors: &Errors) -> Vec<Widget> {
    let proto = Widget::select(
        "proto",
        "Protocol",
        &draft.proto,
        vec![
            SelectOption::new("udp", "udp"),
            SelectOption::new("tcp", "tcp"),
        ],
        errors.get("proto"),
    );
    let mut port = text_field("port", "Port", "", &draft.port, "", errors);
    if let Widget::Field(Field { datatype, .. }) = &mut port {
        *datatype = "port".into();
    }
    let mut host = text_field("host", "Server", "remote", &draft.host, HOST_HELP, errors);
    if let Widget::Field(Field { datatype, .. }) = &mut host {
        *datatype = "host".into();
    }
    let more = !draft.cert.is_empty() || !draft.key.is_empty() || !draft.tls_key.is_empty();
    vec![
        host,
        port,
        proto,
        code_field("ca", "CA certificate", "ca", &draft.ca, CA_HELP, errors),
        Widget::switch_keyed(
            "redirect",
            "Send all traffic through the VPN",
            "redirect-gateway",
            REDIRECT_HELP,
            draft.redirect,
        ),
        Widget::Disclosure {
            style: "reveal".into(),
            summary: MORE.into(),
            open: more,
            children: vec![
                code_field(
                    "cert",
                    "Client certificate",
                    "cert",
                    &draft.cert,
                    CERT_HELP,
                    errors,
                ),
                code_field("key", "Client key", "key", &draft.key, "", errors),
                Widget::select(
                    "tls_kind",
                    "TLS key kind",
                    &draft.tls_kind,
                    vec![
                        SelectOption::new("tls-crypt", "tls-crypt"),
                        SelectOption::new("tls-auth", "tls-auth"),
                    ],
                    "",
                ),
                code_field("tls_key", "TLS key", "", &draft.tls_key, TLS_HELP, errors),
            ],
        },
    ]
}

fn text_field(
    name: &str,
    label: &str,
    key: &str,
    value: &str,
    help: &str,
    errors: &Errors,
) -> Widget {
    Widget::Field(Field {
        name: name.into(),
        label: label.into(),
        kind: "text".into(),
        key: key.into(),
        value: value.into(),
        help: help.into(),
        error: errors.get(name).into(),
        ..Default::default()
    })
}

fn code_field(
    name: &str,
    label: &str,
    key: &str,
    value: &str,
    help: &str,
    errors: &Errors,
) -> Widget {
    Widget::Field(Field {
        name: name.into(),
        label: label.into(),
        kind: "textarea".into(),
        style: "code".into(),
        key: key.into(),
        value: value.into(),
        help: help.into(),
        error: errors.get(name).into(),
        ..Default::default()
    })
}

/// found is what the chosen profile turned out to be, said before anything
/// is asked about it.
fn found(read: &Read) -> Vec<Property> {
    let mut out = vec![Property {
        label: "Kind".into(),
        value: if read.client {
            "OpenVPN client"
        } else {
            "OpenVPN server"
        }
        .into(),
        ..Default::default()
    }];
    if let Some(first) = read.remotes.first() {
        let servers = match read.remotes.len() {
            1 => first.clone(),
            n => format!("{first} and {} more", n - 1),
        };
        out.push(Property {
            label: "Servers".into(),
            value: servers,
            verbatim: true,
            ..Default::default()
        });
    }
    if !read.proto.is_empty() {
        out.push(Property {
            label: "Protocol".into(),
            value: read.proto.clone(),
            mono: true,
            ..Default::default()
        });
    }
    out
}

fn sign_in(draft: &Draft, errors: &Errors) -> Widget {
    let lede = if draft.details() {
        DETAILS_SIGN_IN
    } else {
        PROFILE_SIGN_IN
    };
    let username = text_field(
        "username",
        "Username",
        "username",
        &draft.username,
        "",
        errors,
    );
    let password = Widget::Field(Field {
        name: "password".into(),
        label: "Password".into(),
        kind: "password".into(),
        key: "password".into(),
        error: errors.get("password").into(),
        ..Default::default()
    });
    Widget::section("Sign-in", lede, vec![username, password]).ruled()
}

fn zone(vpn: &Vpn, draft: &Draft, errors: &Errors) -> Widget {
    let options = vpn
        .zones
        .iter()
        .map(|z| SelectOption::new(&z.name, &z.name))
        .collect();
    let mut select = Widget::select(
        "zone",
        "Firewall zone",
        &draft.zone,
        options,
        errors.get("zone"),
    );
    if let Widget::Field(Field { key, help, .. }) = &mut select {
        *key = "zone".into();
        *help = ZONE_HELP.into();
    }
    select
}

/// device is the tun device the new tunnel runs on: the first tunN nothing
/// else holds, named in its section so its network can name it too.
pub fn device(vpn: &Vpn) -> String {
    (0..)
        .map(|n| format!("tun{n}"))
        .find(|d| !vpn.devices.contains(d))
        .unwrap_or_default()
}

/// section is what the tunnel's own section says.
fn section(vpn: &Vpn, draft: &Draft) -> Map<String, Value> {
    let mut values = Map::new();
    values.insert(
        "enabled".into(),
        json!(if draft.enabled { "1" } else { "0" }),
    );
    values.insert("config".into(), json!(format!("{DIR}{}.ovpn", draft.name)));
    values.insert("dev".into(), json!(device(vpn)));
    if !draft.username.is_empty() {
        values.insert("username".into(), json!(draft.username));
        values.insert("password".into(), json!(draft.password));
    }
    values
}

fn preview(vpn: &Vpn, draft: &Draft) -> String {
    let mut openvpn = section(vpn, draft);
    if openvpn.contains_key("password") {
        openvpn.insert("password".into(), json!("••••••••"));
    }
    let mut network = Map::new();
    network.insert("proto".into(), json!("none"));
    network.insert("device".into(), json!(device(vpn)));
    let mut zone = Map::new();
    zone.insert("name".into(), json!(draft.zone));
    zone.insert("network".into(), json!(vec![draft.name.clone()]));
    [
        format!("# {CONFIG}"),
        uci_text("openvpn", &draft.name, &openvpn),
        String::new(),
        "# network".into(),
        uci_text("interface", &draft.name, &network),
        String::new(),
        "# firewall".into(),
        uci_text("zone", "", &zone),
    ]
    .join("\n")
}

fn pem(body: &str, kinds: &[&str]) -> bool {
    kinds
        .iter()
        .any(|k| body.contains(&format!("-----BEGIN {k}-----")))
}

/// validate is what keeps the draft from being a tunnel.
fn validate(vpn: &Vpn, draft: &Draft) -> Errors {
    let mut errors = Errors::default();
    if draft.details() {
        let host_ok = !draft.host.is_empty()
            && draft.host.len() <= 253
            && draft
                .host
                .chars()
                .all(|c| c.is_ascii_alphanumeric() || matches!(c, '.' | '-' | ':'));
        errors.check("host", host_ok, "Enter the server's name or address.");
        let port_ok = draft.port.parse::<u16>().is_ok_and(|p| p > 0);
        errors.check("port", port_ok, "Enter a port from 1 to 65535.");
        errors.check(
            "ca",
            pem(&draft.ca, &["CERTIFICATE"]),
            "Paste the CA certificate, from -----BEGIN CERTIFICATE----- to its END line.",
        );
        if !draft.cert.is_empty() || !draft.key.is_empty() {
            errors.check(
                "cert",
                pem(&draft.cert, &["CERTIFICATE"]),
                "Paste the client certificate, or leave both it and the key empty.",
            );
            errors.check(
                "key",
                pem(
                    &draft.key,
                    &[
                        "PRIVATE KEY",
                        "RSA PRIVATE KEY",
                        "EC PRIVATE KEY",
                        "ENCRYPTED PRIVATE KEY",
                    ],
                ),
                "Paste the client key, or leave both it and the certificate empty.",
            );
        }
        if !draft.tls_key.is_empty() {
            errors.check(
                "tls_key",
                pem(&draft.tls_key, &["OpenVPN Static key V1"]),
                "Paste the TLS key as its operator gave it, from its BEGIN line to its END line.",
            );
        }
    } else {
        let read = read(&draft.text);
        if draft.text.is_empty() {
            errors.field("profile", "Choose a profile.");
        } else if draft.text.len() > LIMIT || draft.text.contains('\0') {
            errors.field(
                "profile",
                "That file is larger than 32 KiB, so it isn’t a profile this router can use.",
            );
        } else if read.remotes.is_empty() {
            errors.field("profile", "This profile names no server to connect to.");
        } else if !read.missing.is_empty() {
            errors.field(
                "profile",
                &format!(
                    "This profile needs files that aren’t inside it: {}. Download the version with its keys inside.",
                    read.missing.join(", ")
                ),
            );
        }
    }
    let valid = !draft.name.is_empty()
        && draft.name.len() <= 12
        && draft.name.starts_with(|c: char| c.is_ascii_lowercase())
        && draft
            .name
            .chars()
            .all(|c| c.is_ascii_lowercase() || c.is_ascii_digit() || c == '_');
    if !valid {
        errors.field(
            "name",
            "Use up to 12 lowercase letters, digits or underscores, starting with a letter.",
        );
    } else if vpn.instance(&draft.name).is_some() || vpn.taken.contains(&draft.name) {
        errors.field("name", "Something on this router already has that name.");
    }
    let asks = draft.details() || read(&draft.text).asks_sign_in;
    if asks && !draft.username.is_empty() && draft.password.is_empty() {
        errors.field("password", "Enter the password.");
    }
    if asks && draft.username.is_empty() && !draft.password.is_empty() {
        errors.field(
            "username",
            "Enter the username that goes with this password.",
        );
    }
    if !vpn.zones.iter().any(|z| z.name == draft.zone) {
        errors.field("zone", "Choose a zone.");
    }
    errors
}

/// save is the draft as everything it stages, or the panel again with what
/// keeps it from being a tunnel.
pub fn save(vpn: &Vpn, draft: &Draft) -> Result<(Vec<CommitOp>, ApplyAction), Errors> {
    let errors = validate(vpn, draft);
    if !errors.is_empty() {
        return Err(errors);
    }
    let path = format!("{DIR}{}.ovpn", draft.name);
    let named = |config: &str, typ: &str, values: Map<String, Value>| CommitOp {
        config: config.into(),
        section: draft.name.clone(),
        section_type: typ.into(),
        delete: false,
        values: Value::Object(values),
    };
    let mut network = Map::new();
    network.insert("proto".into(), json!("none"));
    network.insert("device".into(), json!(device(vpn)));
    let zone = vpn
        .zones
        .iter()
        .find(|z| z.name == draft.zone)
        .expect("validated");
    let mut members = zone.networks.clone();
    members.push(draft.name.clone());
    let ops = vec![
        named(CONFIG, "openvpn", section(vpn, draft)),
        named("network", "interface", network),
        CommitOp {
            config: "firewall".into(),
            section: zone.section.clone(),
            section_type: String::new(),
            delete: false,
            values: json!({ "network": members }),
        },
    ];
    let stage = ApplyAction {
        name: "config-file-stage".into(),
        args: [
            ("path".into(), path),
            ("expected".into(), NEW_FILE_VERSION.into()),
            ("content".into(), draft.profile()),
        ]
        .into(),
    };
    Ok((ops, stage))
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::fixture;

    const PROTON: &str = "client\ndev tun\nproto udp\nremote 185.107.56.234 1194\nremote 185.107.56.234 80\nauth-user-pass\n<ca>\nCERT\n</ca>\n<tls-crypt>\nKEY\n</tls-crypt>\n";
    const CA: &str = "-----BEGIN CERTIFICATE-----\nMIIBca\n-----END CERTIFICATE-----";

    fn vpn() -> Vpn {
        Vpn::read(&fixture::request("/"))
    }

    fn from_file(text: &str) -> Draft {
        Draft {
            text: text.into(),
            file: "nl-free-12.protonvpn.udp.ovpn".into(),
            name: "nl_free_12".into(),
            username: "me".into(),
            password: "s3cret".into(),
            ..Draft::blank()
        }
    }

    fn by_hand() -> Draft {
        Draft {
            way: DETAILS.into(),
            host: "vpn.example.com".into(),
            ca: CA.into(),
            name: "home".into(),
            ..Draft::blank()
        }
    }

    fn fields(draft: &Draft) -> Vec<serde_json::Value> {
        let body = serde_json::to_value(drawer(&vpn(), draft, &Errors::default())).unwrap();
        body["children"][0]["fields"].as_array().unwrap().clone()
    }

    #[test]
    fn a_profile_reads_out_what_the_panel_needs() {
        let r = read(PROTON);
        assert!(r.client);
        assert_eq!(r.proto, "udp");
        assert_eq!(r.remotes, ["185.107.56.234:1194", "185.107.56.234:80"]);
        assert!(r.asks_sign_in);
        assert!(r.missing.is_empty());
        let split = read("client\nremote a 1194\nca ca.crt\ncert client.crt\n<key>\nK\n</key>\n");
        assert_eq!(split.missing, ["ca.crt", "client.crt"]);
    }

    #[test]
    fn a_name_comes_from_the_file_or_the_server() {
        assert_eq!(suggest("nl-free-12.protonvpn.udp.ovpn"), "nl_free_12");
        assert_eq!(suggest("Proton.ovpn"), "proton");
        assert_eq!(suggest("vpn.example.com"), "vpn");
        assert_eq!(suggest("203.0.113.7"), "vpn_203");
        assert_eq!(suggest("…….ovpn"), "vpn");
    }

    #[test]
    fn the_panel_opens_on_which_way_with_both_offered_alike() {
        let fields = fields(&Draft::blank());
        assert_eq!(fields[0]["name"], "way");
        assert_eq!(fields[0]["reshapes"], true);
        assert_eq!(fields[0]["options"][0]["value"], FILE);
        assert_eq!(fields[0]["options"][1]["value"], DETAILS);
        assert_eq!(fields[2]["kind"], "file");
        assert!(fields.iter().all(|f| f["name"] != "name"), "{fields:?}");
    }

    #[test]
    fn a_chosen_profile_is_said_back_then_asked_about() {
        let body =
            serde_json::to_value(drawer(&vpn(), &from_file(PROTON), &Errors::default())).unwrap();
        let text = body.to_string();
        assert!(text.contains("OpenVPN client"), "{text}");
        assert!(text.contains("185.107.56.234:1194 and 1 more"), "{text}");
        assert!(text.contains("\"Sign-in\""), "{text}");
        assert!(
            text.contains("dev 'tun1'"),
            "the next free device is offered: {text}"
        );
        assert!(
            !text.contains("s3cret"),
            "the password reached the page: {text}"
        );
    }

    #[test]
    fn the_details_way_asks_for_the_server_and_folds_what_few_need() {
        let fields = fields(&Draft {
            way: DETAILS.into(),
            ..Draft::blank()
        });
        let names: Vec<&str> = fields.iter().filter_map(|f| f["name"].as_str()).collect();
        for name in [
            "way", "host", "port", "proto", "ca", "redirect", "name", "enabled",
        ] {
            assert!(names.contains(&name), "no {name} in {names:?}");
        }
        let more = fields
            .iter()
            .find(|f| f["type"] == "disclosure")
            .expect("folded");
        assert_eq!(more["style"], "reveal");
        assert!(more.get("open").is_none());
        assert!(fields.iter().all(|f| f["kind"] != "file"));
    }

    #[test]
    fn details_are_written_out_as_the_profile_a_provider_would_hand_over() {
        let mut d = by_hand();
        d.username = "me".into();
        d.tls_key =
            "-----BEGIN OpenVPN Static key V1-----\nab\n-----END OpenVPN Static key V1-----".into();
        let profile = d.profile();
        for line in [
            "client",
            "proto udp",
            "remote vpn.example.com 1194",
            "remote-cert-tls server",
            "auth-user-pass",
            "redirect-gateway def1",
            "<ca>\n-----BEGIN CERTIFICATE-----",
            "<tls-crypt>\n-----BEGIN OpenVPN Static key V1-----",
        ] {
            assert!(profile.contains(line), "no {line:?} in\n{profile}");
        }
        // Read back the way a dropped-in profile is, it is one this panel takes.
        let r = read(&profile);
        assert_eq!(r.remotes, ["vpn.example.com:1194"]);
        assert!(r.missing.is_empty());
        d.tls_kind = "tls-auth".into();
        assert!(d.profile().contains("key-direction 1\n"));
        assert!(d.profile().contains("<tls-auth>"));
    }

    #[test]
    fn both_ways_stage_a_profile_its_section_its_network_and_its_zone() {
        let (ops, stage) = save(&vpn(), &from_file(PROTON)).expect("saved");
        let ops = serde_json::to_value(ops).unwrap();
        assert_eq!(ops[0]["section"], "nl_free_12");
        assert_eq!(ops[0]["values"]["dev"], "tun1");
        assert_eq!(ops[0]["values"]["username"], "me");
        assert_eq!(ops[1]["values"], json!({"proto": "none", "device": "tun1"}));
        assert_eq!(
            ops[2]["values"]["network"],
            json!(["wan", "wan6", "nl_free_12"])
        );
        assert_eq!(stage.args["path"], "/etc/openvpn/nl_free_12.ovpn");
        assert_eq!(stage.args["expected"], NEW_FILE_VERSION);
        assert_eq!(stage.args["content"], PROTON);

        let (ops, stage) = save(&vpn(), &by_hand()).expect("saved");
        let ops = serde_json::to_value(ops).unwrap();
        assert_eq!(ops[0]["values"]["config"], "/etc/openvpn/home.ovpn");
        assert!(ops[0]["values"].get("username").is_none());
        assert!(stage.args["content"].contains("remote vpn.example.com 1194"));
    }

    #[test]
    fn what_keeps_a_draft_from_being_a_tunnel_is_marked() {
        let vpn = vpn();
        let mut d = from_file("client\nremote a 1194\nca ca.crt\n");
        d.name = "proton".into();
        let Err(errors) = save(&vpn, &d) else {
            panic!("a profile with a key file beside it was added");
        };
        assert!(errors.get("profile").contains("ca.crt"));
        assert!(!errors.get("name").is_empty(), "proton is taken");

        let mut d = by_hand();
        d.host.clear();
        d.port = "70000".into();
        d.ca = "not a certificate".into();
        d.cert = "-----BEGIN CERTIFICATE-----\nx\n-----END CERTIFICATE-----".into();
        let Err(errors) = save(&vpn, &d) else {
            panic!("half a server was added");
        };
        for field in ["host", "port", "ca", "key"] {
            assert!(!errors.get(field).is_empty(), "{field} unmarked");
        }
    }

    #[test]
    fn a_way_chosen_again_keeps_what_both_share() {
        let form = Form::parse(
            "add=1&way=details&named_from=x.ovpn&name=x&zone=lan&enabled=1&username=me",
        );
        let d = Draft::submitted(&form, &vpn());
        assert!(d.details());
        assert_eq!(d.zone, "lan");
        assert_eq!(d.username, "me");
        assert!(!d.redirect, "a shown switch left unticked stays off");
        assert_eq!(d.port, "1194");
    }

    #[test]
    fn a_suggested_name_is_one_nothing_has_yet() {
        // vpn is a network on the fixture router, so the server's suggestion
        // steps to the next free name rather than one the panel then refuses.
        let form = Form::parse("add=1&way=details&host=vpn.example.com");
        assert_eq!(Draft::submitted(&form, &vpn()).name, "vpn2");
        // Still the panel's own once shown, a new server renames it.
        let form = Form::parse(
            "add=1&way=details&host=home.example.net&named_from=vpn.example.com&name=vpn2",
        );
        assert_eq!(Draft::submitted(&form, &vpn()).name, "home");
        // Typed over, it is kept.
        let form = Form::parse(
            "add=1&way=details&host=home.example.net&named_from=vpn.example.com&name=office",
        );
        assert_eq!(Draft::submitted(&form, &vpn()).name, "office");
    }
}
