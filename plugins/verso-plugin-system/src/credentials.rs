// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.
use std::collections::BTreeMap;
use verso_plugin::{ApplyAction, Envelope, Form, Property, Request, Tone, Widget};
// An artifact is a document the router holds — a key, a certificate — and
// reads like one: named, with what it is for, then each fact's value beside
// its label, read line by line.
fn artifact(title: &str, purpose: &str, items: Vec<Property>) -> Widget {
    let mut facts = Widget::properties(items);
    if let Widget::Properties { align, .. } = &mut facts {
        *align = "left".into();
    }
    let mut card = Widget::card(title, vec![facts]);
    if let Widget::Card {
        style, subtitle, ..
    } = &mut card
    {
        *style = "artifact".into();
        *subtitle = purpose.into();
    }
    card
}
fn property(label: &str, value: &str, copy: bool) -> Property {
    Property {
        label: label.into(),
        value: value.into(),
        mono: true,
        copy,
        ..Default::default()
    }
}
fn value<'a>(r: &'a serde_json::Value, key: &str) -> &'a str {
    r.get(key).and_then(serde_json::Value::as_str).unwrap_or("")
}
pub fn keys(r: &Request) -> Widget {
    let mut children = vec![];
    if let Some(keys) = r
        .ubus
        .get("accessCredentials")
        .and_then(|v| v.get("keys"))
        .and_then(serde_json::Value::as_array)
    {
        if keys.is_empty() {
            children.push(Widget::text("No keys are authorized."));
        }
        for key in keys {
            let fingerprint = value(key, "fingerprint");
            children.push(artifact(
                "",
                "",
                vec![
                    property("Comment", value(key, "comment"), false),
                    property("Fingerprint", fingerprint, true),
                ],
            ));
            children.push(Widget::link(
                "Remove",
                &format!(
                    "/plugins/system/access/key/remove?fingerprint={}",
                    fingerprint.replace('+', "%2B").replace('/', "%2F")
                ),
                "secondary",
            ));
        }
    } else {
        children.push(Widget::text("Authorized keys could not be read."));
    }
    children.push(Widget::link(
        "Add a key",
        "/plugins/system/access/key/new",
        "secondary",
    ));
    Widget::section("Authorized keys", "", children)
}
// certificate_facts answers what someone at a browser's "not secure" page
// wants to know: is this my router's certificate (the fingerprint, written as
// the browser writes it), who vouches for it, and how long it has left — the
// stretch it is valid for, filled up to today. The fill turns marigold a
// month before the end and crimson in the last week.
fn certificate_facts(cert: &serde_json::Value) -> Vec<Property> {
    let days = |key| value(cert, key).parse::<u32>().unwrap_or(0);
    let (left, total) = (days("days_left"), days("days_total").max(1));
    let expired = value(cert, "expired") == "1";
    let tone = match left {
        _ if expired => Tone::Danger,
        0..=7 => Tone::Danger,
        8..=30 => Tone::Warning,
        _ => Tone::Success,
    };
    let signed = if value(cert, "self_signed") == "1" {
        Property {
            label: "Signed by".into(),
            value: "The router itself".into(),
            ..Default::default()
        }
        .marked(Tone::Warning)
        .noted("Browsers warn until you trust it on each device, or replace it.")
    } else {
        property("Signed by", value(cert, "issuer"), false).marked(Tone::Success)
    };
    let remaining = Property {
        label: "Time left".into(),
        value: if expired {
            "Expired".into()
        } else {
            format!("{left} d")
        },
        verbatim: !expired,
        ..Default::default()
    };
    vec![
        property("Issued to", value(cert, "subject"), true),
        signed,
        Property {
            label: "Valid".into(),
            ..Default::default()
        }
        .spanning(
            value(cert, "from"),
            value(cert, "until"),
            (total.saturating_sub(left) * 100 / total).min(100) as u8,
            tone,
        ),
        remaining.marked(tone),
        property("Fingerprint", value(cert, "fingerprint"), true)
            .noted("Compare it with the SHA-256 fingerprint your browser shows for this site."),
        property("File", value(cert, "file"), false),
    ]
}
pub fn certificate(r: &Request) -> Vec<Widget> {
    let mut children = vec![];
    if let Some(cert) = r
        .ubus
        .get("accessCredentials")
        .and_then(|v| v.get("certificate"))
    {
        if !value(cert, "fingerprint").is_empty() {
            children.push(artifact(
                "HTTPS certificate",
                "What this router shows browsers that connect over HTTPS.",
                certificate_facts(cert),
            ));
        } else {
            children.push(Widget::text("No readable web certificate was found."));
        }
    }
    // The router cannot obtain a trusted certificate itself; one issued
    // elsewhere is installed like any other, so there is one way in.
    let mut download = Widget::link("Download", "/system/access/certificate", "secondary");
    if let Widget::Link { icon, .. } = &mut download {
        *icon = "download".into();
    }
    let mut actions = Widget::stack(vec![
        Widget::link(
            "Install a certificate",
            "/plugins/system/access/certificate/install",
            "secondary",
        ),
        Widget::link(
            "Make a new one",
            "/plugins/system/access/certificate/new",
            "secondary",
        ),
        download,
    ]);
    if let Widget::Stack { inline, .. } = &mut actions {
        *inline = true;
    }
    children.push(actions);
    children
}
fn textarea(name: &str, label: &str, value: &str) -> Widget {
    let mut field = Widget::field(name, label, value, "", "");
    if let Widget::Field { kind, .. } = &mut field {
        *kind = "textarea".into();
    }
    field
}

pub fn route(r: &Request, form: Option<&Form>) -> Envelope {
    let path = r.path.trim_end_matches('/');
    let mut fields = vec![];
    let mut error = String::new();
    let mut command = None;
    let title;
    match path {
        "/access/key/new" => {
            title = "Add a key";
            let key = form.map(|f| f.get("key")).unwrap_or_default();
            fields.push(textarea("key", "Public key", &key));
            if form.is_some() {
                if key.lines().count() != 1
                    || !(key.starts_with("ssh-") || key.starts_with("ecdsa-"))
                    || key.split_whitespace().count() < 2
                {
                    error = "Paste one complete SSH public key.".into();
                } else {
                    command = Some(ApplyAction {
                        name: "ssh-key-add".into(),
                        args: BTreeMap::from([("key".into(), key)]),
                    });
                }
            }
        }
        "/access/key/remove" => {
            title = "Remove key";
            let fingerprint = r.query.get("fingerprint");
            fields.push(Widget::code("Fingerprint", &fingerprint));
            fields.push(Widget::text(
                "This key will no longer be able to sign in over SSH.",
            ));
            if form.is_some() {
                command = Some(ApplyAction {
                    name: "ssh-key-remove".into(),
                    args: BTreeMap::from([("fingerprint".into(), fingerprint)]),
                });
            }
        }
        "/access/certificate/new" => {
            title = "Make a new certificate";
            let hostname = form
                .map(|f| f.get("hostname"))
                .unwrap_or_else(|| super::facts(&r.snapshot).hostname);
            fields.push(Widget::field(
                "hostname",
                "Router name",
                &hostname,
                "host",
                "",
            ));
            fields.push(Widget::text("Creates a self-signed certificate valid for two years. The web server restarts immediately."));
            if form.is_some() {
                if !super::valid_server(&hostname) {
                    error = "Enter a valid hostname or IP address.".into();
                } else {
                    command = Some(ApplyAction {
                        name: "certificate-generate".into(),
                        args: BTreeMap::from([("hostname".into(), hostname)]),
                    });
                }
            }
        }
        "/access/certificate/install" => {
            title = "Install a certificate";
            let certificate = form.map(|f| f.get("certificate")).unwrap_or_default();
            fields.push(Widget::text("A trusted certificate is issued for a domain you control. Obtain it from your certificate authority, then paste the certificate and its private key here."));
            fields.push(textarea("certificate", "Certificate (PEM)", &certificate));
            // A failed attempt may keep its public certificate, never its key.
            fields.push(textarea("key", "Private key (PEM)", ""));
            fields.push(Widget::text(
                "The certificate and key must match. The web server restarts immediately.",
            ));
            if let Some(form) = form {
                if !certificate.contains("BEGIN CERTIFICATE")
                    || !form.get("key").contains("PRIVATE KEY")
                {
                    error = "Paste a PEM certificate and its private key.".into();
                } else {
                    command = Some(ApplyAction {
                        name: "certificate-install".into(),
                        args: BTreeMap::from([
                            ("certificate".into(), certificate),
                            ("key".into(), form.get("key")),
                        ]),
                    });
                }
            }
        }
        _ => {
            return Envelope::page("Access", Widget::text("This page does not exist."))
                .with_back("Access", "/system/access")
        }
    }
    let mut result = Envelope::page(
        title,
        Widget::Form {
            style: "page".into(),
            submit: if path.ends_with("remove") {
                "Remove"
            } else {
                "Save"
            }
            .into(),
            error,
            note: String::new(),
            fields,
        },
    )
    .with_back("Access", "/system/access")
    .with_width("form");
    if let Some(command) = command {
        result.commands = vec![command];
        result = result.with_notice(Tone::Success, "Access credentials updated.");
    }
    result
}
