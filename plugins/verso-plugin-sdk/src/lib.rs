// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

//! The Verso plugin SDK.
//!
//! Everything common to a Verso plugin lives here so each plugin writes only its
//! own `get`/`post` logic (see verso-plugin-hostname2 for the ~40-line example):
//!
//! - [`serve`] — bind the unix socket and run the HTTP loop, routing GET → your
//!   `get` and POST → your `post`.
//! - [`Snapshot`] / [`Section`] — the read the shell injects (X-Verso-UCI).
//! - [`Form`] — a decoded POST submission.
//! - [`envelope`], [`card`], [`form`], [`field`], [`list`], [`raw`], [`commit`],
//!   [`with_commit`] — builders for the wire schema (docs/plugins.md).
//!
//! A plugin holds no session and writes nothing itself (ADR-007): it renders from
//! the snapshot and returns a commit intent the shell applies. This crate speaks
//! only the documented JSON contract; it shares no code with the shell.

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
    G: Fn(&Snapshot) -> Value + Send + Sync + 'static,
    P: Fn(&Form) -> Value + Send + Sync + 'static,
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
    G: Fn(&Snapshot) -> Value,
    P: Fn(&Form) -> Value,
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

// ---- widget / envelope builders (the wire schema) ----

/// envelope wraps a widget as the top-level reply (schema_version + title).
pub fn envelope(title: &str, widget: Value) -> Value {
    json!({ "schema_version": 1, "title": title, "widget": widget })
}

/// with_commit attaches a commit intent (the declarative writes the shell applies).
pub fn with_commit(mut envelope: Value, ops: Vec<Value>) -> Value {
    envelope["commit"] = Value::Array(ops);
    envelope
}

/// card is a titleless container of child widgets.
pub fn card(children: Vec<Value>) -> Value {
    json!({ "type": "card", "children": children })
}

/// raw is a Markdown note — the governed bridge, for when no widget fits (ADR-005).
pub fn raw(markdown: &str) -> Value {
    json!({ "type": "raw", "markdown": markdown })
}

/// form is a submittable set of fields; `success` is shown after a save (omit "").
pub fn form(submit: &str, success: &str, fields: Vec<Value>) -> Value {
    let mut f = json!({ "type": "form", "submit": submit, "fields": fields });
    if !success.is_empty() {
        f["success"] = json!(success);
    }
    f
}

/// field is a single text input carrying a declared datatype the shell enforces.
pub fn field(name: &str, label: &str, value: &str, datatype: &str, help: &str) -> Value {
    json!({
        "type": "field", "name": name, "label": label, "kind": "text",
        "value": value, "datatype": datatype, "help": help
    })
}

/// list is a repeatable text input (one value per row), each of the given datatype.
pub fn list(name: &str, label: &str, datatype: &str, items: &[String], help: &str) -> Value {
    json!({
        "type": "list", "name": name, "label": label, "kind": "text",
        "datatype": datatype, "items": items, "help": help
    })
}

/// commit is one declarative uci write for [`with_commit`].
pub fn commit(config: &str, section: &str, values: Value) -> Value {
    json!({ "config": config, "section": section, "values": values })
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
