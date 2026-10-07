// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.
use super::credentials::{key_command, ADD_KEY, REMOVE_KEY};
use super::sshkey;
use std::collections::BTreeMap;
use verso_plugin::{
    commit, json, Envelope, Form, Request, Section, SectionWidget, SelectOption, Switch,
    Tone, Widget,
};
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
fn switch(name: &str, label: &str, help: &str, on: bool, e: &Errors) -> Widget {
    let mut w = Widget::switch_keyed(name, label, name, "", on);
    if let Widget::Switch(Switch { help: h, error, .. }) = &mut w {
        *h = help.into();
        *error = e.get(name).cloned().unwrap_or_default();
    }
    w
}
// A settings form holds its Save until something in it changes, so the page
// spends no action colour on a form with nothing to stage.
fn form(section: &str, fields: Vec<Widget>, e: &Errors) -> Widget {
    let mut fields = fields;
    fields.insert(0, Widget::hidden("_access_config", "ssh"));
    fields.insert(1, Widget::hidden("section", section));
    Widget::Form {
        style: "settings".into(),
        submit: "Save SSH settings".into(),
        error: Widget::refusal(&fields, e),
        note: String::new(),
        target: String::new(),
        fields,
    }
    .at("dropbear", section)
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
                e,
            ),
            switch(
                "RootPasswordAuth",
                "Allow root to log in with a password",
                "",
                value(s, f, "RootPasswordAuth", "on") == if f.is_some() { "1" } else { "on" },
                e,
            ),
        ];
        // A section commits its settings first, then holds what it keeps:
        // keys change at once and are no part of what its Save stages.
        let mut children = vec![form(&s.name(), fields, e)];
        if index == 0 {
            children.push(super::credentials::keys(
                r,
                posted,
                errors.get(ADD_KEY).map(String::as_str).unwrap_or(""),
            ));
        }
        let mut section = Widget::section("SSH", "", children).ruled();
        if ssh.len() > 1 {
            if let Widget::Section(SectionWidget { meta, .. }) = &mut section {
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
    // Verso serves its pages itself (ADR-017): its part here is the
    // certificate it serves them with; where it answers is the shell's.
    sections.push(super::credentials::certificate(r));
    Envelope::page("Access", Widget::stack(sections)).with_width("form")
}
fn port(v: &str) -> bool {
    !v.is_empty() && v.bytes().all(|b| b.is_ascii_digit()) && v.parse::<u16>().is_ok_and(|n| n > 0)
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

    if f.get("_access_config") != "ssh" {
        return page(
            r,
            Some(f),
            &Errors::from([("form".into(), "Choose the settings to save.".into())]),
        );
    }
    let sections = r.snapshot.sections_of_type("dropbear", "dropbear");
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
    let values = json!({"Port":f.get("Port"),"Interface":if iface.is_empty(){serde_json::Value::Null}else{json!(iface)},"PasswordAuth":if f.get("PasswordAuth")=="1"{"on"}else{"off"},"RootPasswordAuth":if f.get("RootPasswordAuth")=="1"{"on"}else{"off"}});
    if !e.is_empty() {
        return page(r, Some(f), &e);
    }
    let values = differing(section, &values);
    if values.is_empty() {
        return page(r, Some(f), &e);
    }
    page(r, Some(f), &e)
        .with_commit(vec![commit("dropbear", &section.name(), values.into())])
        .with_notice(Tone::Success, "Access settings saved.")
}

// What the page shows for an option the router leaves unset, the same
// defaults it draws with (`value` above).
const DEFAULTS: &[(&str, &str)] = &[
    ("Port", "22"),
    ("PasswordAuth", "on"),
    ("RootPasswordAuth", "on"),
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
