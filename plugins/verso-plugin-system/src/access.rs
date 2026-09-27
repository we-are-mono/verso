// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.
use super::credentials::{key_command, ADD_KEY, REMOVE_KEY};
use super::sshkey;
use std::collections::BTreeMap;
use verso_plugin::{commit, json, Envelope, Form, Request, Section, SelectOption, Tone, Widget};
type Errors = BTreeMap<String, String>;
fn field(name: &str, label: &str, key: &str, value: &str, e: &Errors) -> Widget {
    super::keyed(name, label, key, value, e)
}
fn value(s: &Section<'_>, f: Option<&Form>, key: &str, default: &str) -> String {
    f.map(|f| f.get(key)).unwrap_or_else(|| {
        let v = s.scalar(key);
        if v.is_empty() {
            default.into()
        } else {
            v
        }
    })
}
fn switch(name: &str, label: &str, help: &str, on: bool) -> Widget {
    let mut w = Widget::switch_keyed(name, label, name, "", on);
    if let Widget::Switch { help: h, .. } = &mut w {
        *h = help.into();
    }
    w
}
// A settings form holds its Save until something in it changes, so the page
// spends no action colour on a form with nothing to stage.
fn form(kind: &str, section: &str, fields: Vec<Widget>, e: &Errors) -> Widget {
    let mut fields = fields;
    fields.insert(0, Widget::hidden("_access_config", kind));
    fields.insert(1, Widget::hidden("section", section));
    let config = if kind == "ssh" { "dropbear" } else { "uhttpd" };
    Widget::Form {
        style: "settings".into(),
        submit: "Save".into(),
        error: e.values().next().cloned().unwrap_or_default(),
        note: String::new(),
        target: String::new(),
        fields,
    }
    .at(config, section)
}
pub fn get(r: &Request) -> Envelope {
    if r.path != "/access" && r.path != "/access/" {
        return super::credentials::route(r, None);
    }
    page(r, None, &Errors::new())
}
fn page(r: &Request, posted: Option<&Form>, errors: &Errors) -> Envelope {
    let mut sections = vec![];
    let ssh = r.snapshot.sections_of_type("dropbear", "dropbear");
    for (index, s) in ssh.iter().enumerate() {
        let f = posted.filter(|f| f.get("_access_config") == "ssh" && f.get("section") == s.name());
        let empty = Errors::new();
        let e = if f.is_some() { errors } else { &empty };
        let mut interfaces = vec![SelectOption::new("", "All interfaces")];
        interfaces.extend(
            r.snapshot
                .sections_of_type("network", "interface")
                .into_iter()
                .filter(|n| n.name() != "loopback")
                .map(|n| SelectOption::new(&n.name(), &n.name())),
        );
        let fields = vec![
            Widget::select(
                "Interface",
                "Listen on",
                &value(s, f, "Interface", ""),
                interfaces,
                e.get("Interface").map(String::as_str).unwrap_or(""),
            )
            .writes("Interface"),
            field("Port", "Port", "Port", &value(s, f, "Port", "22"), e),
            switch(
                "PasswordAuth",
                "Allow password login",
                "",
                value(s, f, "PasswordAuth", "on") == if f.is_some() { "1" } else { "on" },
            ),
            switch(
                "RootPasswordAuth",
                "Allow root to log in with a password",
                "",
                value(s, f, "RootPasswordAuth", "on") == if f.is_some() { "1" } else { "on" },
            ),
        ];
        // A section commits its settings first, then holds what it keeps:
        // keys change at once and are no part of what its Save stages.
        let mut children = vec![form("ssh", &s.name(), fields, e)];
        if index == 0 {
            children.push(super::credentials::keys(
                r,
                posted,
                errors.get(ADD_KEY).map(String::as_str).unwrap_or(""),
            ));
        }
        let mut section = Widget::section("SSH", "", children).ruled();
        if ssh.len() > 1 {
            if let Widget::Section { meta, .. } = &mut section {
                *meta = s.name();
            }
        }
        sections.push(section);
    }
    if ssh.is_empty() {
        sections.push(
            Widget::section(
                "SSH",
                "",
                vec![
                    Widget::text("No SSH server is configured."),
                    Widget::link(
                        "Install packages",
                        "/system/packages/discover?q=dropbear",
                        "secondary",
                    ),
                ],
            )
            .ruled(),
        );
    }
    let web = r.snapshot.sections_of_type("uhttpd", "uhttpd");
    for (index, s) in web.iter().enumerate() {
        let f = posted.filter(|f| f.get("_access_config") == "web" && f.get("section") == s.name());
        let empty = Errors::new();
        let e = if f.is_some() { errors } else { &empty };
        let mut fields = vec![
            switch(
                "redirect_https",
                "Redirect to HTTPS",
                "",
                value(s, f, "redirect_https", "0") == "1",
            ),
            switch(
                "rfc1918_filter",
                "Block DNS rebinding",
                "Refuses requests from private addresses sent to the router's public address.",
                value(s, f, "rfc1918_filter", "0") == "1",
            ),
        ];
        let mut listeners = vec![];
        for (key, label, port_label) in [
            ("listen_http", "HTTP listeners", "HTTP port"),
            ("listen_https", "HTTPS listeners", "HTTPS port"),
        ] {
            if let Some(port) = shared_port(s, key) {
                let name = format!("{key}_port");
                let current = f.map(|f| f.get(&name)).unwrap_or(port);
                listeners.push(field(&name, port_label, key, &current, e));
            } else {
                let items = f
                    .map(|f| {
                        f.all(key)
                            .into_iter()
                            .filter(|v| !v.trim().is_empty())
                            .collect()
                    })
                    .unwrap_or_else(|| s.list(key));
                let mut list = Widget::list(key, label, "", &items, "").writes(key);
                if let Widget::List { style, prompt, .. } = &mut list {
                    *style = "rows".into();
                    *prompt = "Address:port".into();
                }
                listeners.push(list);
            }
        }
        // Scalar ports are one short pair; listener lists need their own rows.
        if listeners.iter().all(|w| matches!(w, Widget::Field { .. })) {
            fields.push(Widget::form_grid(2, listeners).labelled("Web ports", ""));
        } else {
            fields.extend(listeners);
        }
        let mut children = vec![form("web", &s.name(), fields, e)];
        if index == 0 {
            children.push(super::credentials::certificate(r));
        }
        let mut section = Widget::section(
            "Web interface",
            "These settings control the router's uhttpd web server.",
            children,
        )
        .ruled();
        if web.len() > 1 {
            if let Widget::Section { meta, .. } = &mut section {
                *meta = s.name();
            }
        }
        sections.push(section);
    }
    if web.is_empty() {
        sections.push(
            Widget::section(
                "Web interface",
                "",
                vec![
                    Widget::text("No uhttpd web server is configured."),
                    Widget::link(
                        "Install packages",
                        "/system/packages/discover?q=uhttpd",
                        "secondary",
                    ),
                ],
            )
            .ruled(),
        );
    }
    Envelope::page("Access", Widget::stack(sections)).with_width("form")
}
fn port(v: &str) -> bool {
    !v.is_empty() && v.bytes().all(|b| b.is_ascii_digit()) && v.parse::<u16>().is_ok_and(|n| n > 0)
}
fn listener(v: &str) -> bool {
    v.parse::<std::net::SocketAddr>()
        .is_ok_and(|a| a.port() > 0)
}
pub fn post(r: &Request, f: &Form) -> Envelope {
    if r.path != "/access" && r.path != "/access/" {
        return super::credentials::route(r, Some(f));
    }
    // Keys are kept where they are listed: a removal posts the key's own
    // fingerprint, a paste the key. Both are commands, run at once.
    let removed = f.get(REMOVE_KEY);
    if !removed.is_empty() {
        return key_command(
            page(r, None, &Errors::new()),
            "ssh-key-remove",
            "fingerprint",
            removed,
            "Key removed.",
        );
    }
    if !f.all(ADD_KEY).is_empty() {
        let pasted = f.get(ADD_KEY);
        return match sshkey::read(&pasted) {
            Ok(_) => key_command(
                page(r, Some(f), &Errors::new()),
                "ssh-key-add",
                "key",
                pasted.trim().to_string(),
                "Key added.",
            ),
            Err(reason) => page(r, Some(f), &Errors::from([(ADD_KEY.into(), reason.into())])),
        };
    }

    let kind = f.get("_access_config");
    let (config, typ) = match kind.as_str() {
        "ssh" => ("dropbear", "dropbear"),
        "web" => ("uhttpd", "uhttpd"),
        _ => {
            return page(
                r,
                Some(f),
                &Errors::from([("form".into(), "Choose the settings to save.".into())]),
            )
        }
    };
    let sections = r.snapshot.sections_of_type(config, typ);
    let Some(section) = sections.iter().find(|s| s.name() == f.get("section")) else {
        return page(
            r,
            Some(f),
            &Errors::from([(
                "form".into(),
                "These settings no longer exist. Reload the page.".into(),
            )]),
        );
    };
    let mut e = Errors::new();
    let values = if kind == "ssh" {
        if !port(&f.get("Port")) {
            e.insert("Port".into(), "Enter a port from 1 to 65535.".into());
        }
        let iface = f.get("Interface");
        if !iface.is_empty()
            && !r
                .snapshot
                .sections_of_type("network", "interface")
                .iter()
                .any(|n| n.name() == iface)
        {
            e.insert("Interface".into(), "Choose an existing interface.".into());
        }
        json!({"Port":f.get("Port"),"Interface":if iface.is_empty(){serde_json::Value::Null}else{json!(iface)},"PasswordAuth":if f.get("PasswordAuth")=="1"{"on"}else{"off"},"RootPasswordAuth":if f.get("RootPasswordAuth")=="1"{"on"}else{"off"}})
    } else {
        let http = submitted_listeners(section, f, "listen_http", &mut e);
        let https = submitted_listeners(section, f, "listen_https", &mut e);
        for value in http.iter().chain(https.iter()) {
            if !listener(value) {
                e.insert(
                    "listeners".into(),
                    "Enter listeners as address:port or [IPv6]:port.".into(),
                );
            }
        }
        if http.is_empty() && https.is_empty() {
            e.insert(
                "listeners".into(),
                "Keep at least one HTTP or HTTPS listener.".into(),
            );
        }
        if https
            .iter()
            .any(|v| http.iter().any(|h| listeners_overlap(h, v)))
        {
            e.insert(
                "listeners".into(),
                "HTTP and HTTPS cannot share the same listener.".into(),
            );
        }
        if f.get("redirect_https") == "1" && https.is_empty() {
            e.insert(
                "redirect_https".into(),
                "Add an HTTPS listener before enabling redirection.".into(),
            );
        }
        json!({"listen_http":if http.is_empty(){serde_json::Value::Null}else{json!(http)},"listen_https":if https.is_empty(){serde_json::Value::Null}else{json!(https)},"redirect_https":if f.get("redirect_https")=="1"{"1"}else{"0"},"rfc1918_filter":if f.get("rfc1918_filter")=="1"{"1"}else{"0"}})
    };
    if !e.is_empty() {
        return page(r, Some(f), &e);
    }
    let values = differing(section, &values);
    if values.is_empty() {
        return page(r, Some(f), &e);
    }
    page(r, Some(f), &e)
        .with_commit(vec![commit(config, &section.name(), values.into())])
        .with_notice(Tone::Success, "Access settings saved.")
}

// What the page shows for an option the router leaves unset, the same
// defaults it draws with (`value` above).
const DEFAULTS: &[(&str, &str)] = &[
    ("Port", "22"),
    ("PasswordAuth", "on"),
    ("RootPasswordAuth", "on"),
    ("redirect_https", "0"),
    ("rfc1918_filter", "0"),
];

// differing keeps what a save changes against what the page showed. An option
// written with the value it already holds still counts as a change waiting to
// be applied — counted on the chip and marked on the page as one — so only
// what differs is staged.
fn differing(
    section: &Section<'_>,
    values: &serde_json::Value,
) -> serde_json::Map<String, serde_json::Value> {
    let Some(values) = values.as_object() else {
        return serde_json::Map::new();
    };
    values
        .iter()
        .filter(|(key, value)| {
            let scalar = section.scalar(key);
            let list = section.list(key);
            match value {
                serde_json::Value::Null => !scalar.is_empty() || !list.is_empty(),
                serde_json::Value::Array(items) => {
                    let items: Vec<String> = items
                        .iter()
                        .map(|v| v.as_str().unwrap_or_default().to_string())
                        .collect();
                    items != list
                }
                serde_json::Value::String(wanted) => {
                    let shown = match scalar.is_empty() {
                        true => DEFAULTS
                            .iter()
                            .find(|(k, _)| k == key)
                            .map(|(_, v)| v.to_string())
                            .unwrap_or_default(),
                        false => scalar,
                    };
                    *wanted != shown
                }
                _ => true,
            }
        })
        .map(|(key, value)| (key.clone(), value.clone()))
        .collect()
}

// A single port keeps the reference's compact control while retaining every
// configured bind address. Unusual multi-port configurations keep their list.
fn shared_port(section: &Section<'_>, key: &str) -> Option<String> {
    let listeners = section.list(key);
    let mut shared = None;
    for listener in listeners {
        let address = listener.parse::<std::net::SocketAddr>().ok()?;
        if shared.is_some_and(|port| port != address.port()) {
            return None;
        }
        shared = Some(address.port());
    }
    Some(shared.map(|p| p.to_string()).unwrap_or_default())
}
fn submitted_listeners(
    section: &Section<'_>,
    form: &Form,
    key: &str,
    errors: &mut Errors,
) -> Vec<String> {
    if shared_port(section, key).is_some() {
        let name = format!("{key}_port");
        let value = form.get(&name);
        if value.is_empty() {
            return vec![];
        }
        if !port(&value) {
            errors.insert(name, "Enter a port from 1 to 65535.".into());
            return vec![];
        }
        let port = value.parse::<u16>().unwrap();
        let mut bindings = section.list(key);
        if bindings.is_empty() {
            bindings = vec!["0.0.0.0:0".into(), "[::]:0".into()];
        }
        return bindings
            .iter()
            .filter_map(|v| v.parse::<std::net::SocketAddr>().ok())
            .map(|mut addr| {
                addr.set_port(port);
                addr.to_string()
            })
            .collect();
    }
    let mut seen = std::collections::BTreeSet::new();
    form.all(key)
        .into_iter()
        .map(|s| s.trim().to_string())
        .filter(|s| !s.is_empty() && seen.insert(s.clone()))
        .collect()
}
fn listeners_overlap(a: &str, b: &str) -> bool {
    match (
        a.parse::<std::net::SocketAddr>(),
        b.parse::<std::net::SocketAddr>(),
    ) {
        (Ok(a), Ok(b)) => {
            a.port() == b.port()
                && a.is_ipv4() == b.is_ipv4()
                && (a.ip() == b.ip() || a.ip().is_unspecified() || b.ip().is_unspecified())
        }
        _ => false,
    }
}
