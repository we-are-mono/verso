// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.
use crate::sshkey;
use std::collections::BTreeMap;
use verso_plugin::{
    ApplyAction, CollectionAdd, CollectionItem, CollectionRemove, Envelope, Field, Form,
    HeadingAct, Property, RemoveConfirm, Request, RowDrawer, Tone, Widget,
};
// An artifact is a document the router holds — a certificate — and reads like
// one: named, with what it is for, then each fact's value beside its label,
// read line by line.
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
/// The field a pasted key arrives in, and the pair a key's removal posts.
pub const ADD_KEY: &str = "authorized_key";
pub const REMOVE_KEY: &str = "_key_remove";

// key_item is one authorized key: what its owner called it over the
// fingerprint someone checks it by, and its removal, asked in place. A key
// nobody named is known by its fingerprint alone.
fn key_item(key: &serde_json::Value) -> CollectionItem {
    let fingerprint = value(key, "fingerprint");
    let (title, detail) = match value(key, "comment") {
        "" => (fingerprint, ""),
        comment => (comment, fingerprint),
    };
    CollectionItem {
        title: title.into(),
        detail: detail.into(),
        remove: Some(CollectionRemove {
            name: REMOVE_KEY.into(),
            value: fingerprint.into(),
            confirm: RemoveConfirm {
                trigger: "Remove".into(),
                icon: "trash-2".into(),
                title: "Remove this key?".into(),
                message:
                    "Whoever holds it can no longer sign in over SSH. This takes effect at once."
                        .into(),
                confirm: "Remove key".into(),
                cancel: "Not now".into(),
            },
        }),
    }
}
// keys is the set of authorized keys, kept where they are listed: each removed
// from its own line after asking, the next pasted into the slot at the foot,
// which reads the key as `ssh-keygen -l` would while it is typed — the line to
// hold against the same one on the machine the key came from. A refused paste
// comes back as typed with its reason.
pub fn keys(r: &Request, posted: Option<&Form>, refused: &str) -> Widget {
    let set = match r
        .ubus
        .get("accessCredentials")
        .and_then(|v| v.get("keys"))
        .and_then(serde_json::Value::as_array)
    {
        Some(keys) => {
            let typed = posted.map(|f| f.get(ADD_KEY)).unwrap_or_default();
            let reading = sshkey::read(&typed)
                .map(|k| k.summary())
                .unwrap_or_default();
            Widget::Collection {
                items: keys.iter().map(key_item).collect(),
                empty: "No keys are authorized.".into(),
                add: Some(CollectionAdd {
                    label: "Add a key".into(),
                    name: ADD_KEY.into(),
                    value: if refused.is_empty() {
                        String::new()
                    } else {
                        typed
                    },
                    placeholder: "ssh-ed25519 AAAA… you@laptop".into(),
                    submit: "Add key".into(),
                    error: refused.into(),
                    open: false,
                    // a reading to hold against another by eye, not to paste:
                    // no copy control over the end of its line
                    preview: Some(Box::new(Widget::Code {
                        label: String::new(),
                        value: reading,
                        copy: false,
                        live: true,
                        grammar: String::new(),
                    })),
                }),
            }
        }
        None => Widget::text("Authorized keys could not be read."),
    };
    Widget::section("Authorized keys", "", vec![set])
}
// key_command answers a key's addition or removal: the command the shell runs
// (it parses the key again before writing), with what happened, on the page
// the change was made from.
pub fn key_command(page: Envelope, name: &str, arg: &str, value: String, done: &str) -> Envelope {
    let mut e = page.with_notice(Tone::Success, done);
    e.commands = vec![ApplyAction {
        name: name.into(),
        args: BTreeMap::from([(arg.into(), value)]),
    }];
    e
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
// certificate is the certificate Verso serves HTTPS with and the acts that
// replace or fetch it, held as one group so its acts read as the card's, not
// the section's — a part of the web section under a subheading of its own, as
// the keys are of SSH.
pub fn certificate(r: &Request) -> Widget {
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
    // Its acts are acts on a part of the section, in the dress the key set's
    // add wears, each led by the glyph of what it does. The two that replace
    // the certificate open their forms in a drawer over Access; Download is a
    // file, and leaves as one.
    let act = |label: &str, href: &str, glyph: &str, opens: bool| {
        let mut link = Widget::link(label, href, "act");
        if let Widget::Link { icon, panel, .. } = &mut link {
            *icon = glyph.into();
            *panel = opens;
        }
        link
    };
    let mut actions = Widget::stack(vec![
        act(
            "Install a certificate",
            "/plugins/system/access/certificate/install",
            "upload",
            true,
        ),
        act(
            "Make a new one",
            "/plugins/system/access/certificate/new",
            "refresh-cw",
            true,
        ),
        act("Download", "/system/access/certificate", "download", false),
    ]);
    if let Widget::Stack { inline, .. } = &mut actions {
        *inline = true;
    }
    children.push(actions);
    Widget::section("Certificates", "", vec![Widget::stack(children)])
}
// A PEM block is text the machine wrote, so it is typed in the code box.
fn textarea(name: &str, label: &str, value: &str) -> Widget {
    let mut field = Widget::field(name, label, value, "", "");
    if let Widget::Field(Field { kind, style, .. }) = &mut field {
        *kind = "textarea".into();
        *style = "code".into();
    }
    field
}

// route answers a certificate's act that replaces it: its form, in the drawer
// the act opens over Access. The act runs at once, so its button names it.
// A valid submission carries the command with the drawer still open, so a
// refusal from the router is said in the drawer; once the act has run, the
// shell closes it on Access read again.
pub fn route(r: &Request, form: Option<&Form>) -> Envelope {
    let path = r.path.trim_end_matches('/');
    let mut fields = vec![];
    let mut error = String::new();
    let mut command = None;
    let (title, submit);
    match path {
        // The router names the certificate itself, for every way the LAN reaches
        // it, as it does at first boot (ADR-017 §5): there is nothing to type.
        "/access/certificate/new" => {
            (title, submit) = ("Make a new certificate", "Make certificate");
            fields.push(Widget::text("Makes a self-signed certificate for this router's name and its addresses on your network, valid for two years. Devices that trusted the current one warn again until they trust the new one."));
            if form.is_some() {
                command = Some(ApplyAction {
                    name: "certificate-generate".into(),
                    args: BTreeMap::new(),
                });
            }
        }
        "/access/certificate/install" => {
            (title, submit) = ("Install a certificate", "Install certificate");
            let certificate = form.map(|f| f.get("certificate")).unwrap_or_default();
            fields.push(Widget::text("A trusted certificate is issued for a domain you control. Obtain it from your certificate authority, then paste the certificate and its private key here."));
            fields.push(textarea("certificate", "Certificate (PEM)", &certificate));
            // A failed attempt may keep its public certificate, never its key.
            fields.push(textarea("key", "Private key (PEM)", ""));
            fields.push(Widget::text(
                "The certificate and key must match. The router serves it from the next connection.",
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
    let drawer = RowDrawer {
        title: title.into(),
        open: true,
        closed: "/system/access".into(),
        children: vec![Widget::Form {
            style: String::new(),
            submit: submit.into(),
            error,
            note: String::new(),
            target: String::new(),
            fields,
        }],
        ..Default::default()
    };
    // At this address the page's one act is the one the drawer runs, and the
    // act carries its drawer open: a panel request is answered with the drawer
    // alone, and a visit with no script finds the act on the heading line.
    let act = HeadingAct {
        label: submit.into(),
        href: format!("/plugins/system{path}"),
        drawer: Some(drawer),
        ..Default::default()
    };
    let mut result = Envelope::page(title, Widget::stack(vec![]))
        .with_act(act)
        .with_back("Access", "/system/access");
    if let Some(command) = command {
        result.commands = vec![command];
        result = result.with_notice(Tone::Success, "Access credentials updated.");
    }
    result
}
