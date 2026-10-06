// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

//! A new tunnel from the profile a VPN provider hands out.
//!
//! The panel opens on one question, which file, and is drawn again around
//! the file once it is chosen: what it is, a name, a sign-in if it asks for
//! one, the zone its traffic leaves by, and whether it starts with the
//! router. One save stages all of it: the profile beside the others, the
//! section that runs it on the next free tun device, a network on that
//! device, and the network in its zone.

use verso_plugin::{
    files::NEW_FILE_VERSION, json, uci_text, ApplyAction, CommitOp, Errors, Field, Form, Map,
    Property, RowDrawer, SelectOption, Value, Widget,
};

use crate::listing;
use crate::model::{Vpn, CONFIG};

/// NEW is what the address names in place of an instance while the import
/// panel is open.
pub const NEW: &str = "new";
/// IMPORT marks a submission as the import panel's own.
pub const IMPORT: &str = "import";

const TITLE: &str = "Import a profile";
const PROMPT: &str = "Drop your provider's profile here";
const FILE_HELP: &str =
    "The .ovpn file from your provider's OpenVPN downloads. Choose the router or Linux version.";
const ZONE_HELP: &str =
    "The firewall zone the tunnel's network joins. In wan, your devices reach the internet through it.";
const ENABLED_HELP: &str = "OpenVPN starts the tunnel when the router boots, and keeps it up.";
const SIGN_IN_LEDE: &str = "The profile asks for a username and password. A provider's OpenVPN \
     sign-in is often not the one for its website: Proton's, for one, is on its dashboard.";
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

/// Draft is what the panel says: the file as read, and the choices around it.
pub struct Draft {
    pub text: String,
    pub file: String,
    pub name: String,
    pub username: String,
    pub password: String,
    pub zone: String,
    pub enabled: bool,
}

impl Draft {
    pub fn blank() -> Draft {
        Draft {
            text: String::new(),
            file: String::new(),
            name: String::new(),
            username: String::new(),
            password: String::new(),
            zone: "wan".into(),
            enabled: true,
        }
    }

    /// submitted reads the panel. A form drawn before the file was chosen has
    /// no name yet, and a file chosen again names the tunnel afresh unless its
    /// name was typed: both take the name from the file.
    pub fn submitted(form: &Form) -> Draft {
        let file = form.get("profile_name");
        let named_from = form.get("named_from");
        let typed = form.get("name").trim().to_string();
        let name = if form.all("name").is_empty()
            || (named_from != file && typed == suggest(&named_from))
        {
            suggest(&file)
        } else {
            typed
        };
        Draft {
            text: form.get("profile"),
            file,
            name,
            username: form.get("username").trim().to_string(),
            password: form.get("password"),
            zone: match form.all("zone").is_empty() {
                true => "wan".into(),
                false => form.get("zone"),
            },
            // On until the panel has shown its switch; then as ticked, which
            // posts "1", or not, which posts nothing.
            enabled: form.all("name").is_empty() || !form.get("enabled").is_empty(),
        }
    }
}

/// suggest is a tunnel's name from its file's: what uci and netifd take in a
/// section name, short enough for a network, "vpn" when nothing is left.
pub fn suggest(file: &str) -> String {
    let stem = file.split('.').next().unwrap_or("");
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
        n => n.into(),
    }
}

/// Read is what a profile says that the import needs to know.
#[derive(Default, Debug)]
pub struct Read {
    pub client: bool,
    pub proto: String,
    pub remotes: Vec<String>,
    pub asks_sign_in: bool,
    /// Key files the profile names beside it rather than carrying inline.
    pub missing: Vec<String>,
}

/// read is the profile as the import judges it.
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

/// drawer is the import panel for this draft.
pub fn drawer(vpn: &Vpn, draft: &Draft, errors: &Errors) -> RowDrawer {
    let profile = Widget::Field(Field {
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
    });
    let mut fields = vec![profile, Widget::hidden(IMPORT, "1")];
    if !draft.text.is_empty() {
        let read = read(&draft.text);
        fields.push(Widget::hidden("named_from", &draft.file));
        fields.push(Widget::properties(found(&read)));
        fields.push(Widget::Field(Field {
            name: "name".into(),
            label: "Name".into(),
            kind: "text".into(),
            key: "openvpn".into(),
            value: draft.name.clone(),
            error: errors.get("name").into(),
            ..Default::default()
        }));
        if read.asks_sign_in {
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
            submit: "Import".into(),
            error: String::new(),
            fields,
            note: String::new(),
            target: String::new(),
        }],
        ..Default::default()
    }
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
    let username = Widget::Field(Field {
        name: "username".into(),
        label: "Username".into(),
        kind: "text".into(),
        key: "username".into(),
        value: draft.username.clone(),
        error: errors.get("username").into(),
        ..Default::default()
    });
    let password = Widget::Field(Field {
        name: "password".into(),
        label: "Password".into(),
        kind: "password".into(),
        key: "password".into(),
        error: errors.get("password").into(),
        ..Default::default()
    });
    Widget::section("Sign-in", SIGN_IN_LEDE, vec![username, password]).ruled()
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

/// validate is what keeps the draft from being a tunnel.
fn validate(vpn: &Vpn, draft: &Draft) -> Errors {
    let mut errors = Errors::default();
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
    if read.asks_sign_in && !draft.username.is_empty() && draft.password.is_empty() {
        errors.field("password", "Enter the password.");
    }
    if read.asks_sign_in && draft.username.is_empty() && !draft.password.is_empty() {
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
    let mut ops = vec![
        named(CONFIG, "openvpn", section(vpn, draft)),
        named("network", "interface", network),
    ];
    ops.push(CommitOp {
        config: "firewall".into(),
        section: zone.section.clone(),
        section_type: String::new(),
        delete: false,
        values: json!({ "network": members }),
    });
    let stage = ApplyAction {
        name: "config-file-stage".into(),
        args: [
            ("path".into(), path),
            ("expected".into(), NEW_FILE_VERSION.into()),
            ("content".into(), draft.text.clone()),
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

    fn vpn() -> Vpn {
        Vpn::read(&fixture::request("/"))
    }

    fn draft(text: &str) -> Draft {
        Draft {
            text: text.into(),
            file: "nl-free-12.protonvpn.udp.ovpn".into(),
            name: "nl_free_12".into(),
            username: "me".into(),
            password: "s3cret".into(),
            zone: "wan".into(),
            enabled: true,
        }
    }

    #[test]
    fn a_profile_reads_out_what_the_import_needs() {
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
    fn a_name_comes_from_the_file() {
        assert_eq!(suggest("nl-free-12.protonvpn.udp.ovpn"), "nl_free_12");
        assert_eq!(suggest("Proton.ovpn"), "proton");
        assert_eq!(suggest("…….ovpn"), "vpn");
    }

    #[test]
    fn the_panel_opens_on_the_file_alone() {
        let body =
            serde_json::to_value(drawer(&vpn(), &Draft::blank(), &Errors::default())).unwrap();
        let fields = body["children"][0]["fields"].as_array().unwrap();
        assert_eq!(fields[0]["kind"], "file");
        assert_eq!(fields[0]["style"], "text");
        assert_eq!(fields[0]["reshapes"], true);
        assert!(fields.iter().all(|f| f["name"] != "name"), "{fields:?}");
    }

    #[test]
    fn a_chosen_profile_is_said_back_then_asked_about() {
        let body =
            serde_json::to_value(drawer(&vpn(), &draft(PROTON), &Errors::default())).unwrap();
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
    fn an_import_stages_the_file_its_section_its_network_and_its_zone() {
        let (ops, stage) = save(&vpn(), &draft(PROTON)).expect("saved");
        let ops = serde_json::to_value(ops).unwrap();
        assert_eq!(ops[0]["config"], "openvpn");
        assert_eq!(ops[0]["section"], "nl_free_12");
        assert_eq!(ops[0]["type"], "openvpn");
        assert_eq!(ops[0]["values"]["config"], "/etc/openvpn/nl_free_12.ovpn");
        assert_eq!(ops[0]["values"]["dev"], "tun1");
        assert_eq!(ops[0]["values"]["username"], "me");
        assert_eq!(ops[1]["config"], "network");
        assert_eq!(ops[1]["values"], json!({"proto": "none", "device": "tun1"}));
        assert_eq!(ops[2]["config"], "firewall");
        assert_eq!(
            ops[2]["values"]["network"],
            json!(["wan", "wan6", "nl_free_12"])
        );
        assert_eq!(stage.name, "config-file-stage");
        assert_eq!(stage.args["path"], "/etc/openvpn/nl_free_12.ovpn");
        assert_eq!(stage.args["expected"], NEW_FILE_VERSION);
        assert_eq!(stage.args["content"], PROTON);
    }

    #[test]
    fn what_keeps_a_draft_from_being_a_tunnel_is_marked() {
        let vpn = vpn();
        let mut d = draft("client\nremote a 1194\nca ca.crt\n");
        d.name = "proton".into();
        let Err(errors) = save(&vpn, &d) else {
            panic!("a profile with a key file beside it was imported");
        };
        assert!(errors.get("profile").contains("ca.crt"));
        assert!(!errors.get("name").is_empty(), "proton is taken");
        let mut d = draft(PROTON);
        d.name = "Bad Name".into();
        d.password.clear();
        let Err(errors) = save(&vpn, &d) else {
            panic!("a bad name was imported");
        };
        assert!(!errors.get("name").is_empty());
        assert_eq!(errors.get("password"), "Enter the password.");
    }
}
