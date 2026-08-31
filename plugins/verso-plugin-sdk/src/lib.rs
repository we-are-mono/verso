// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

//! The Verso plugin SDK.
//!
//! Everything common to a Verso plugin lives here so each plugin writes only its
//! own `get`/`post` logic (see verso-plugin-system for the example):
//!
//! - [`serve`] — bind the unix socket and run the HTTP loop, routing GET → your
//!   `get` and POST → your `post`.
//! - [`Snapshot`] / [`Section`] — the read the shell injects (X-Verso-UCI).
//! - [`Form`] — a decoded POST submission.
//! - [`Envelope`] / [`Widget`] / [`Tone`] — the typed wire schema
//!   (docs/plugins.md, ADR-006 §9). The `Widget` enum mirrors the shell's
//!   vocabulary, so your editor's completion is the catalog; there is no
//!   untyped builder set, and `raw` is one variant among many, for prose.
//!
//! A plugin holds no session and writes nothing itself (ADR-007): it renders from
//! the snapshot and returns a commit intent the shell applies. This crate speaks
//! only the documented JSON contract; it shares no code with the shell — the two
//! are pinned together by the conformance fixtures this crate's tests write and
//! the shell's tests decode.

use serde::Serialize;
use std::io::{Read, Write};
use std::os::unix::fs::PermissionsExt;
use std::os::unix::net::{UnixListener, UnixStream};
use std::path::Path;
use std::sync::Arc;
use std::{env, fs, thread};

pub use serde_json::{json, Value};

/// serve binds `/var/run/verso/<id>.sock` (override with `VERSO_<ID>_SOCKET`) and
/// serves forever: GET → `get`, POST → `post`. Each returns the full envelope
/// [`Value`]. A bind failure logs and exits; per-connection work runs on its own
/// thread. This never returns under normal operation.
pub fn serve<G, P>(id: &str, get: G, post: P)
where
    G: Fn(&Snapshot) -> Envelope + Send + Sync + 'static,
    P: Fn(&Form) -> Envelope + Send + Sync + 'static,
{
    let env_key = format!("VERSO_{}_SOCKET", id.to_uppercase());
    let socket = env::var(&env_key).unwrap_or_else(|_| format!("/var/run/verso/{id}.sock"));

    if let Some(dir) = Path::new(&socket).parent() {
        let _ = fs::create_dir_all(dir);
    }
    let _ = fs::remove_file(&socket); // clear a stale socket from a previous run

    let listener = match UnixListener::bind(&socket) {
        Ok(l) => l,
        Err(e) => {
            eprintln!("verso-plugin {id}: listen {socket}: {e}");
            std::process::exit(1);
        }
    };
    // Group-accessible (0660) so the shell (in group verso) can connect (ADR-007).
    let _ = fs::set_permissions(&socket, fs::Permissions::from_mode(0o660));
    eprintln!("verso-plugin {id} listening on {socket}");

    let get = Arc::new(get);
    let post = Arc::new(post);
    for stream in listener.incoming().flatten() {
        let (g, p) = (get.clone(), post.clone());
        thread::spawn(move || {
            let _ = handle(stream, g.as_ref(), p.as_ref());
        });
    }
}

fn handle<G, P>(mut stream: UnixStream, get: &G, post: &P) -> std::io::Result<()>
where
    G: Fn(&Snapshot) -> Envelope,
    P: Fn(&Form) -> Envelope,
{
    // Bound a slow or hostile peer (group verso: the shell or a sibling plugin):
    // a stalled read/write cannot pin this thread indefinitely, and an oversized
    // body is refused — the same posture verso-rpcd takes at the root boundary.
    const MAX_BODY: usize = 1 << 20;
    let _ = stream.set_read_timeout(Some(std::time::Duration::from_secs(15)));
    let _ = stream.set_write_timeout(Some(std::time::Duration::from_secs(15)));

    let mut buf = Vec::new();
    let mut tmp = [0u8; 4096];
    let header_end = loop {
        let n = stream.read(&mut tmp)?;
        if n == 0 {
            return Ok(());
        }
        buf.extend_from_slice(&tmp[..n]);
        if let Some(pos) = find(&buf, b"\r\n\r\n") {
            break pos + 4;
        }
        if buf.len() > MAX_BODY {
            return Ok(()); // runaway header — drop it
        }
    };

    let (method, headers) = parse_head(&String::from_utf8_lossy(&buf[..header_end]));
    let content_length = headers
        .iter()
        .find(|(k, _)| k == "content-length")
        .and_then(|(_, v)| v.trim().parse::<usize>().ok())
        .unwrap_or(0);

    if content_length > MAX_BODY {
        return Ok(()); // oversized body — drop it before reading
    }
    let mut body = buf[header_end..].to_vec();
    while body.len() < content_length {
        let n = stream.read(&mut tmp)?;
        if n == 0 {
            break;
        }
        body.extend_from_slice(&tmp[..n]);
    }

    let envelope = if method == "POST" {
        post(&Form::parse(&String::from_utf8_lossy(&body)))
    } else {
        let uci = headers
            .iter()
            .find(|(k, _)| k == "x-verso-uci")
            .map(|(_, v)| v.as_str())
            .unwrap_or("");
        get(&Snapshot::from_b64(uci))
    };

    let payload = serde_json::to_vec(&envelope).unwrap_or_default();
    let head = format!(
        "HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: {}\r\nConnection: close\r\n\r\n",
        payload.len()
    );
    stream.write_all(head.as_bytes())?;
    stream.write_all(&payload)?;
    stream.flush()
}

// ---- the read snapshot (docs/plugins.md) ----

/// Snapshot is the brokered UCI read the shell injects: config → section → table.
pub struct Snapshot(Value);

impl Snapshot {
    fn from_b64(b64: &str) -> Snapshot {
        if b64.is_empty() {
            return Snapshot(Value::Null);
        }
        match base64_decode(b64).and_then(|d| serde_json::from_slice(&d).ok()) {
            Some(v) => Snapshot(v),
            None => Snapshot(Value::Null),
        }
    }

    /// section returns `config`'s section by name.
    pub fn section(&self, config: &str, name: &str) -> Option<Section<'_>> {
        self.0.get(config)?.get(name)?.as_object().map(Section)
    }

    /// sections_of_type returns `config`'s sections whose `.type` is `typ`, ordered
    /// by `.index` so anonymous sections keep their on-disk order.
    pub fn sections_of_type(&self, config: &str, typ: &str) -> Vec<Section<'_>> {
        let obj = match self.0.get(config).and_then(Value::as_object) {
            Some(o) => o,
            None => return Vec::new(),
        };
        let mut secs: Vec<(f64, Section)> = obj
            .values()
            .filter_map(Value::as_object)
            .filter(|m| m.get(".type").and_then(Value::as_str) == Some(typ))
            .map(|m| {
                (
                    m.get(".index").and_then(Value::as_f64).unwrap_or(0.0),
                    Section(m),
                )
            })
            .collect();
        secs.sort_by(|a, b| a.0.partial_cmp(&b.0).unwrap_or(std::cmp::Ordering::Equal));
        secs.into_iter().map(|(_, s)| s).collect()
    }
}

/// Section is one config section: scalar options are strings, list options arrays.
pub struct Section<'a>(&'a serde_json::Map<String, Value>);

impl Section<'_> {
    /// name returns the section's UCI handle (`.name` in rpcd's snapshot).
    pub fn name(&self) -> String {
        self.0
            .get(".name")
            .and_then(Value::as_str)
            .unwrap_or("")
            .to_string()
    }

    /// scalar reads an option as a string, or "" if unset or a list.
    pub fn scalar(&self, option: &str) -> String {
        self.0
            .get(option)
            .and_then(Value::as_str)
            .unwrap_or("")
            .to_string()
    }

    /// list reads a list option as strings, or empty if unset or a scalar.
    pub fn list(&self, option: &str) -> Vec<String> {
        self.0
            .get(option)
            .and_then(Value::as_array)
            .map(|a| {
                a.iter()
                    .filter_map(|v| v.as_str().map(String::from))
                    .collect()
            })
            .unwrap_or_default()
    }
}

// ---- the POST submission ----

/// Form is a decoded urlencoded submission; a field may repeat (a list posts as a
/// multi-value field).
pub struct Form {
    pairs: Vec<(String, String)>,
}

impl Form {
    fn parse(body: &str) -> Form {
        let mut pairs = Vec::new();
        for pair in body.split('&') {
            if pair.is_empty() {
                continue;
            }
            let mut kv = pair.splitn(2, '=');
            let key = url_decode(kv.next().unwrap_or(""));
            let val = url_decode(kv.next().unwrap_or(""));
            pairs.push((key, val));
        }
        Form { pairs }
    }

    /// get returns the first value for `key` (empty if absent).
    pub fn get(&self, key: &str) -> String {
        self.pairs
            .iter()
            .find(|(k, _)| k == key)
            .map(|(_, v)| v.clone())
            .unwrap_or_default()
    }

    /// all returns every value for `key`, in order.
    pub fn all(&self, key: &str) -> Vec<String> {
        self.pairs
            .iter()
            .filter(|(k, _)| k == key)
            .map(|(_, v)| v.clone())
            .collect()
    }
}

// ---- the typed wire schema (ADR-006 §9) ----

/// Tone is the semantic state vocabulary the whole UI speaks (ADR-005): a
/// plugin names intent, never a colour; the shell maps the tone to palette.
#[derive(Serialize, Clone, Copy, PartialEq, Eq, Debug)]
#[serde(rename_all = "lowercase")]
pub enum Tone {
    Neutral,
    Success,
    Warning,
    Danger,
    Info,
}

fn is_false(b: &bool) -> bool {
    !*b
}

/// Widget is the typed mirror of the shell's widget vocabulary: compose these —
/// the editor's completion is the catalog. Every variant serializes to the
/// documented wire shape; the conformance fixtures pin that to the shell's
/// decoder. `Raw` is for prose only — an outcome belongs in the envelope's
/// notice, a machine value in `Code`/`Properties`, a nothing-here state in
/// `Empty` (ADR-005 §4).
#[derive(Serialize, Debug)]
#[serde(tag = "type", rename_all = "lowercase")]
pub enum Widget {
    /// A titled box of child widgets — the page-layout primitive.
    Card {
        #[serde(skip_serializing_if = "String::is_empty")]
        title: String,
        #[serde(skip_serializing_if = "String::is_empty")]
        subtitle: String,
        children: Vec<Widget>,
    },
    /// A titled region of the page, set apart with generous space. Meta is
    /// compact status text beside the title (an optional icon and "inline"
    /// position refine it).
    Section {
        title: String,
        #[serde(skip_serializing_if = "String::is_empty")]
        sub: String,
        #[serde(skip_serializing_if = "String::is_empty")]
        meta: String,
        #[serde(skip_serializing_if = "String::is_empty")]
        meta_icon: String,
        #[serde(skip_serializing_if = "String::is_empty")]
        meta_position: String,
        children: Vec<Widget>,
    },
    /// Vertical rhythm for its children; draws nothing itself. Width "compact"
    /// narrows the run for a short form column.
    Stack {
        #[serde(skip_serializing_if = "String::is_empty")]
        width: String,
        children: Vec<Widget>,
    },
    /// Side-by-side columns; the shell owns the responsive collapse.
    Grid { columns: u32, children: Vec<Widget> },
    /// A submittable set of fields; the shell threads CSRF and posts back here.
    /// Style "page" hands submission to the shell's staging capsule.
    Form {
        #[serde(skip_serializing_if = "String::is_empty")]
        style: String,
        #[serde(skip_serializing_if = "String::is_empty")]
        submit: String,
        fields: Vec<Widget>,
    },
    /// One labelled input with a declared datatype the shell enforces (ADR-008).
    /// Kind picks the control ("text", "select", "hidden", "datetime-local", …);
    /// options feed a select; error is the inline validation message (422).
    Field {
        name: String,
        #[serde(skip_serializing_if = "String::is_empty")]
        label: String,
        kind: String,
        value: String,
        #[serde(skip_serializing_if = "String::is_empty")]
        datatype: String,
        #[serde(skip_serializing_if = "Vec::is_empty")]
        options: Vec<SelectOption>,
        #[serde(skip_serializing_if = "String::is_empty")]
        error: String,
        #[serde(skip_serializing_if = "String::is_empty")]
        help: String,
    },
    /// A field-set gated by a toggle: `fields` show while it is on, `otherwise`
    /// while it is off. The shell realizes the show/hide (ADR-005 §7); the
    /// toggle posts under `name` so the plugin reads the chosen branch.
    Conditional {
        name: String,
        label: String,
        checked: bool,
        fields: Vec<Widget>,
        #[serde(skip_serializing_if = "Vec::is_empty")]
        otherwise: Vec<Widget>,
    },
    /// A repeatable input sharing one name — a list of ports, hosts, CIDRs.
    List {
        name: String,
        label: String,
        kind: String,
        datatype: String,
        items: Vec<String>,
        #[serde(skip_serializing_if = "String::is_empty")]
        help: String,
    },
    /// A boxed contextual notice beside content, toned by intent. An action's
    /// *outcome* belongs in the envelope's notice, not here.
    Callout {
        variant: Tone,
        #[serde(skip_serializing_if = "String::is_empty")]
        title: String,
        body: String,
        #[serde(skip_serializing_if = "is_false")]
        compact: bool,
    },
    /// A machine value in a monospace box, with an inline copy button.
    Code {
        #[serde(skip_serializing_if = "String::is_empty")]
        label: String,
        value: String,
        copy: bool,
    },
    /// The designed nothing-here state: icon, headline, reassurance, and the
    /// call(s) to action as children.
    Empty {
        icon: String,
        title: String,
        body: String,
        children: Vec<Widget>,
    },
    /// A label/value fact sheet.
    Properties { items: Vec<Property> },
    /// A status pill.
    Badge {
        variant: Tone,
        text: String,
        #[serde(skip_serializing_if = "is_false")]
        dot: bool,
    },
    /// A short run of styled prose (Markdown), inline in a composition.
    Text { markdown: String },
    /// A destructive action behind an explicit confirmation.
    Confirm {
        trigger: String,
        message: String,
        #[serde(skip_serializing_if = "String::is_empty")]
        confirm: String,
        #[serde(skip_serializing_if = "String::is_empty")]
        cancel: String,
    },
    /// The governed bridge: display-only Markdown **prose** no widget shapes
    /// (ADR-005 §4). Metered — its usage is the demand signal for the next
    /// widget.
    Raw { markdown: String },
}

impl Widget {
    /// card is a container of child widgets, optionally titled.
    pub fn card(title: &str, children: Vec<Widget>) -> Widget {
        Widget::Card {
            title: title.into(),
            subtitle: String::new(),
            children,
        }
    }

    /// section is a titled region of the page.
    pub fn section(title: &str, sub: &str, children: Vec<Widget>) -> Widget {
        Widget::Section {
            title: title.into(),
            sub: sub.into(),
            meta: String::new(),
            meta_icon: String::new(),
            meta_position: String::new(),
            children,
        }
    }

    /// stack lays children out vertically; width "compact" via the variant.
    pub fn stack(children: Vec<Widget>) -> Widget {
        Widget::Stack {
            width: String::new(),
            children,
        }
    }

    /// form is a submittable set of fields with the given submit label.
    pub fn form(submit: &str, fields: Vec<Widget>) -> Widget {
        Widget::Form {
            style: String::new(),
            submit: submit.into(),
            fields,
        }
    }

    /// field is a single text input carrying a datatype the shell enforces.
    pub fn field(name: &str, label: &str, value: &str, datatype: &str, help: &str) -> Widget {
        Widget::Field {
            name: name.into(),
            label: label.into(),
            kind: "text".into(),
            value: value.into(),
            datatype: datatype.into(),
            options: Vec::new(),
            error: String::new(),
            help: help.into(),
        }
    }

    /// select is a single-choice field over a closed option set.
    pub fn select(name: &str, label: &str, value: &str, options: Vec<SelectOption>, error: &str) -> Widget {
        Widget::Field {
            name: name.into(),
            label: label.into(),
            kind: "select".into(),
            value: value.into(),
            datatype: String::new(),
            options,
            error: error.into(),
            help: String::new(),
        }
    }

    /// hidden is the bare value carrier a form posts but a person never edits.
    pub fn hidden(name: &str, value: &str) -> Widget {
        Widget::Field {
            name: name.into(),
            label: String::new(),
            kind: "hidden".into(),
            value: value.into(),
            datatype: String::new(),
            options: Vec::new(),
            error: String::new(),
            help: String::new(),
        }
    }

    /// list is a repeatable text input (one value per row) of one datatype.
    pub fn list(name: &str, label: &str, datatype: &str, items: &[String], help: &str) -> Widget {
        Widget::List {
            name: name.into(),
            label: label.into(),
            kind: "text".into(),
            datatype: datatype.into(),
            items: items.to_vec(),
            help: help.into(),
        }
    }

    /// callout is a boxed contextual notice beside content.
    pub fn callout(variant: Tone, title: &str, body: &str) -> Widget {
        Widget::Callout {
            variant,
            title: title.into(),
            body: body.into(),
            compact: false,
        }
    }

    /// code shows a machine value with a copy button.
    pub fn code(label: &str, value: &str) -> Widget {
        Widget::Code {
            label: label.into(),
            value: value.into(),
            copy: true,
        }
    }

    /// empty is the designed nothing-here state.
    pub fn empty(icon: &str, title: &str, body: &str, children: Vec<Widget>) -> Widget {
        Widget::Empty {
            icon: icon.into(),
            title: title.into(),
            body: body.into(),
            children,
        }
    }

    /// text is a short run of styled prose.
    pub fn text(markdown: &str) -> Widget {
        Widget::Text {
            markdown: markdown.into(),
        }
    }

    /// raw is the governed prose bridge — reach for a widget first (ADR-005 §4).
    pub fn raw(markdown: &str) -> Widget {
        Widget::Raw {
            markdown: markdown.into(),
        }
    }
}

/// SelectOption is one choice of a select field.
#[derive(Serialize, Debug)]
pub struct SelectOption {
    pub value: String,
    pub label: String,
}

/// Property is one row of a [`Widget::Properties`] fact sheet.
#[derive(Serialize, Debug)]
pub struct Property {
    pub label: String,
    pub value: String,
    #[serde(skip_serializing_if = "is_false")]
    pub mono: bool,
    #[serde(skip_serializing_if = "is_false")]
    pub copy: bool,
}

/// Notice is the outcome of the action this render answers — shown by the shell
/// in its own flash slot (ADR-006 §4). State the outcome; never compose it.
#[derive(Serialize, Debug)]
pub struct Notice {
    pub level: Tone,
    pub text: String,
}

/// CommitOp is one declarative uci write the shell applies on the plugin's
/// behalf (ADR-007). An empty section with a section_type creates a new
/// section of that type, then sets the values on it.
#[derive(Serialize, Debug)]
pub struct CommitOp {
    pub config: String,
    pub section: String,
    #[serde(rename = "type", skip_serializing_if = "String::is_empty")]
    pub section_type: String,
    pub values: Value,
}

/// commit is one declarative uci write for [`Envelope::with_commit`].
pub fn commit(config: &str, section: &str, values: Value) -> CommitOp {
    CommitOp {
        config: config.into(),
        section: section.into(),
        section_type: String::new(),
        values,
    }
}

/// commit_new creates a new section of `section_type` and sets `values` on it.
pub fn commit_new(config: &str, section_type: &str, values: Value) -> CommitOp {
    CommitOp {
        config: config.into(),
        section: String::new(),
        section_type: section_type.into(),
        values,
    }
}

/// ApplyAction is one tightly typed non-UCI operation the shell performs after
/// the staged UCI transaction applies — only for a declared rpcd scope.
#[derive(Serialize, Debug)]
pub struct ApplyAction {
    pub name: String,
    pub args: std::collections::BTreeMap<String, String>,
}

/// Envelope is the typed top-level reply (ADR-006 §4).
#[derive(Serialize, Debug)]
pub struct Envelope {
    pub schema_version: u32,
    pub title: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    pub subheading: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    pub width: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub notice: Option<Notice>,
    pub widget: Widget,
    #[serde(skip_serializing_if = "Vec::is_empty")]
    pub commit: Vec<CommitOp>,
    #[serde(skip_serializing_if = "Vec::is_empty")]
    pub apply: Vec<ApplyAction>,
}

impl Envelope {
    /// page wraps a widget as the reply for one page render.
    pub fn page(title: &str, widget: Widget) -> Envelope {
        Envelope {
            schema_version: 1,
            title: title.into(),
            subheading: String::new(),
            width: String::new(),
            notice: None,
            widget,
            commit: Vec::new(),
            apply: Vec::new(),
        }
    }

    /// with_subheading sets the lede under the page heading.
    pub fn with_subheading(mut self, sub: &str) -> Envelope {
        self.subheading = sub.into();
        self
    }

    /// with_width sets the content-column preset ("narrow" | "normal" | "wide").
    pub fn with_width(mut self, width: &str) -> Envelope {
        self.width = width.into();
        self
    }

    /// with_notice states this render's outcome — shown in the shell's flash slot.
    pub fn with_notice(mut self, level: Tone, text: &str) -> Envelope {
        self.notice = Some(Notice {
            level,
            text: text.into(),
        });
        self
    }

    /// with_commit attaches the declarative writes the shell applies.
    pub fn with_commit(mut self, ops: Vec<CommitOp>) -> Envelope {
        self.commit = ops;
        self
    }

    /// with_apply attaches the typed post-apply operations.
    pub fn with_apply(mut self, ops: Vec<ApplyAction>) -> Envelope {
        self.apply = ops;
        self
    }
}

// ---- tiny internals (no dependencies) ----

fn parse_head(head: &str) -> (String, Vec<(String, String)>) {
    let mut lines = head.split("\r\n");
    let method = lines
        .next()
        .and_then(|l| l.split(' ').next())
        .unwrap_or("GET")
        .to_string();
    let headers = lines
        .filter_map(|line| line.split_once(':'))
        .map(|(k, v)| (k.trim().to_ascii_lowercase(), v.trim().to_string()))
        .collect();
    (method, headers)
}

fn find(haystack: &[u8], needle: &[u8]) -> Option<usize> {
    haystack.windows(needle.len()).position(|w| w == needle)
}

fn base64_decode(s: &str) -> Option<Vec<u8>> {
    fn sextet(c: u8) -> Option<u32> {
        match c {
            b'A'..=b'Z' => Some((c - b'A') as u32),
            b'a'..=b'z' => Some((c - b'a') as u32 + 26),
            b'0'..=b'9' => Some((c - b'0') as u32 + 52),
            b'+' => Some(62),
            b'/' => Some(63),
            _ => None,
        }
    }
    let mut out = Vec::new();
    let (mut acc, mut bits) = (0u32, 0u32);
    for &c in s.as_bytes() {
        if c == b'=' || c.is_ascii_whitespace() {
            continue;
        }
        acc = (acc << 6) | sextet(c)?;
        bits += 6;
        if bits >= 8 {
            bits -= 8;
            out.push((acc >> bits) as u8);
        }
    }
    Some(out)
}

fn url_decode(s: &str) -> String {
    let bytes = s.as_bytes();
    let mut out = Vec::with_capacity(bytes.len());
    let mut i = 0;
    while i < bytes.len() {
        match bytes[i] {
            b'+' => {
                out.push(b' ');
                i += 1;
            }
            b'%' if i + 2 < bytes.len() => match (hex(bytes[i + 1]), hex(bytes[i + 2])) {
                (Some(h), Some(l)) => {
                    out.push(h * 16 + l);
                    i += 3;
                }
                _ => {
                    out.push(b'%');
                    i += 1;
                }
            },
            c => {
                out.push(c);
                i += 1;
            }
        }
    }
    String::from_utf8_lossy(&out).into_owned()
}

fn hex(c: u8) -> Option<u8> {
    match c {
        b'0'..=b'9' => Some(c - b'0'),
        b'a'..=b'f' => Some(c - b'a' + 10),
        b'A'..=b'F' => Some(c - b'A' + 10),
        _ => None,
    }
}
