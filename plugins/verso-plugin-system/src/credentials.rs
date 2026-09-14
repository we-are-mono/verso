// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.
use std::collections::BTreeMap;
use verso_plugin::{ApplyAction, Envelope, Form, Property, Request, Tone, Widget};
fn artifact(items: Vec<Property>) -> Widget {
    let mut card = Widget::card("", vec![Widget::Properties { items }]);
    if let Widget::Card { style, .. } = &mut card {
        *style = "artifact".into();
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
        for key in keys {
            let fingerprint = value(key, "fingerprint");
            children.push(artifact(vec![
                property("Comment", value(key, "comment"), false),
                property("Fingerprint", fingerprint, true),
            ]));
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
    Widget::section("Authorised keys", "", children)
}
pub fn certificate(r: &Request) -> Vec<Widget> {
    let mut children = vec![];
    if let Some(cert) = r
        .ubus
        .get("accessCredentials")
        .and_then(|v| v.get("certificate"))
    {
        if !value(cert, "fingerprint").is_empty() {
            children.push(artifact(vec![
                property("File", value(cert, "file"), false),
                property("Issued to", value(cert, "subject"), true),
                property("Signed by", value(cert, "issuer"), false),
                property("Made", value(cert, "from"), false),
                property("Valid until", value(cert, "until"), false),
                property("Fingerprint", value(cert, "fingerprint"), true),
            ]));
            if value(cert, "self_signed") == "1" {
                children.push(Widget::text("Self-signed — browsers warn until you install it as trusted on each device, or replace it."));
            }
        } else {
            children.push(Widget::text("No readable web certificate was found."));
        }
    }
    let mut actions = Widget::stack(vec![
        Widget::link(
            "Get a trusted certificate",
            "/plugins/system/access/certificate/trusted",
            "button",
        ),
        Widget::link(
            "Use my own",
            "/plugins/system/access/certificate/install",
            "secondary",
        ),
        Widget::link(
            "Make a new one",
            "/plugins/system/access/certificate/new",
            "secondary",
        ),
        Widget::link("Download", "/system/access/certificate", "secondary"),
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
            title = "Use my own certificate";
            let certificate = form.map(|f| f.get("certificate")).unwrap_or_default();
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
        "/access/certificate/trusted" => {
            return Envelope::page("Get a trusted certificate", Widget::stack(vec![
                Widget::text("A trusted certificate is issued for a domain you control. Use your certificate authority to obtain it, then install the certificate and its private key here."),
                Widget::link("Use my own", "/plugins/system/access/certificate/install", "button"),
            ])).with_back("Access", "/system/access").with_width("form");
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
