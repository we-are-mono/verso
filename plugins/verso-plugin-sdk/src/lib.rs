// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

//! The Verso plugin SDK.
//!
//! Everything common to a Verso plugin lives here so each plugin writes only its
//! own `get`/`post` logic (see verso-plugin-system for the example):
//!
//! - [`serve`] — bind the unix socket and run the HTTP loop, routing GET → your
//!   `get` and POST → your `post`.
//! - [`Request`] — what both handlers answer: the sub-path below the plugin's
//!   mount and the reads the shell injected with it.
//! - [`Snapshot`] / [`Section`] — the config read the shell injects (X-Verso-UCI).
//! - [`Ubus`] — the live-state reads the shell brokered (X-Verso-Ubus).
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

use serde::{Deserialize, Serialize};
use std::io::{Read, Write};
use std::os::unix::fs::PermissionsExt;
use std::os::unix::net::{UnixListener, UnixStream};
use std::path::Path;
use std::sync::Arc;
use std::{env, fs, thread};

pub use serde_json::{json, Map, Value};

pub mod dhcp;
mod errors;
pub use errors::Errors;
/// Hand-edited files a page lists and edits in place. See [`files`].
pub mod files;
mod sha256;
pub use sha256::{sha256, sha256_hex};
/// What a config can hold, as the daemon that reads it defines — and the guard
/// that every option is either rendered, deliberately left out, or unsupported
/// upstream. See [`vocabulary`].
pub mod vocabulary;

/// serve binds `/var/run/verso/<id>.sock` (override with `VERSO_<ID>_SOCKET`) and
/// serves forever: GET → `get`, POST → `post`. Both see the [`Request`] — the
/// page asked for and the snapshot read for it — and `post` additionally the
/// submitted [`Form`]; each returns the full [`Envelope`]. A bind failure logs
/// and exits; per-connection work runs on its own thread. This never returns
/// under normal operation.
pub fn serve<G, P>(id: &str, get: G, post: P)
where
    G: Fn(&Request) -> Envelope + Send + Sync + 'static,
    P: Fn(&Request, &Form) -> Envelope + Send + Sync + 'static,
{
    // A plugin with no describe hook leaves every pending change to the shell's
    // raw uci fallback.
    serve_described(id, get, post, |_, _| Vec::new());
}

/// serve_described is [`serve`] plus a describe hook: the shell POSTs its pending
/// change list here (marked `X-Verso-Describe`) and `describe` answers each run of
/// this plugin's changes in plain words, resolving section handles against the
/// injected [`Snapshot`]. A change the hook does not cover keeps its raw uci line
/// in the Review drawer, so a partial describer is as valid as a full one.
pub fn serve_described<G, P, D>(id: &str, get: G, post: P, describe: D)
where
    G: Fn(&Request) -> Envelope + Send + Sync + 'static,
    P: Fn(&Request, &Form) -> Envelope + Send + Sync + 'static,
    D: Fn(&[Change], &Snapshot) -> Vec<Description> + Send + Sync + 'static,
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
    // procd records stdout as info and stderr as err; a successful bind is info.
    println!("verso-plugin {id} listening on {socket}");

    let get = Arc::new(get);
    let post = Arc::new(post);
    let describe = Arc::new(describe);
    for stream in listener.incoming().flatten() {
        let (g, p, d) = (get.clone(), post.clone(), describe.clone());
        thread::spawn(move || {
            let _ = handle(stream, g.as_ref(), p.as_ref(), d.as_ref());
        });
    }
}

fn handle<G, P, D>(mut stream: UnixStream, get: &G, post: &P, describe: &D) -> std::io::Result<()>
where
    G: Fn(&Request) -> Envelope,
    P: Fn(&Request, &Form) -> Envelope,
    D: Fn(&[Change], &Snapshot) -> Vec<Description>,
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

    let (method, target, headers) = parse_head(&String::from_utf8_lossy(&buf[..header_end]));
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

    // Both reads ride every request the shell forwards, POST included, so a
    // submission renders its answer from the same reads a GET would have seen.
    let request = Request {
        path: request_path(&target),
        query: request_query(&target),
        snapshot: Snapshot::from_b64(header(&headers, "x-verso-uci")),
        ubus: Ubus::from_b64(header(&headers, "x-verso-ubus")),
    };

    // A describe call is the shell asking this plugin to put its pending changes
    // in plain words for the review drawer: it carries the change list in
    // the body and the same uci snapshot as a render, and answers with the
    // descriptions rather than a widget envelope.
    if !header(&headers, "x-verso-describe").is_empty() {
        let req: DescribeRequestBody = serde_json::from_slice(&body).unwrap_or_default();
        let descriptions = describe(&req.changes, &request.snapshot);
        let payload =
            serde_json::to_vec(&DescribeResponseBody { descriptions }).unwrap_or_default();
        let head = format!(
            "HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: {}\r\nConnection: close\r\n\r\n",
            payload.len()
        );
        stream.write_all(head.as_bytes())?;
        stream.write_all(&payload)?;
        return stream.flush();
    }

    let envelope = if method == "POST" {
        post(&request, &Form::parse(&String::from_utf8_lossy(&body)))
    } else {
        get(&request)
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

// ---- what a handler answers ----

/// Request is one page ask. Path is the sub-path below the plugin's mount — "/"
/// for the plugin's own root, "/zones" for a subpage — so a plugin with several
/// pages routes on it; Query is the request target's query string, which the
/// shell forwards verbatim and which addresses something *within* a page (which
/// record to open, which device to act on) rather than which page to render;
/// Snapshot and Ubus are the reads the shell took for this request, config and
/// live state.
pub struct Request {
    pub path: String,
    pub query: Form,
    pub snapshot: Snapshot,
    pub ubus: Ubus,
}

// ---- the describe hook ----

/// Change is one pending uci change the shell asks a plugin to describe. It is the
/// net effect of one target in named fields, with a normalized `op` so a describer
/// never reasons about a raw tuple's length. `op` is a small closed vocabulary:
///
/// - `set` — `option` set to `value` on `section`
/// - `add-section` — new `section`, `option` is its uci type
/// - `remove-option` — `option` cleared from `section`
/// - `remove-section` — `section` removed whole
/// - `list-add` — `value` added to the list `option` on `section`
/// - `list-del` — `value` removed from the list `option` on `section`
///
/// A field an op does not use is empty. A removed section is gone from the injected
/// [`Snapshot`] (which reflects the staged config), so describe a `remove-section`
/// only as far as its handle allows, and let the raw line stand otherwise.
#[derive(Deserialize, Default, Debug, Clone)]
pub struct Change {
    pub config: String,
    pub op: String,
    #[serde(default)]
    pub section: String,
    #[serde(default)]
    pub option: String,
    #[serde(default)]
    pub value: String,
}

/// Description is one plain-language sentence for a run of pending changes —
/// "Turned off the rule “Block Telnet”". `covers` names the indices, into the
/// slice the shell passed `describe`, that this sentence accounts for; one sentence
/// may fold several changes (a rename writes two options). A change no description
/// covers keeps its raw uci line in the drawer, so describe what reads well and
/// leave the rest.
#[derive(Serialize, Debug)]
pub struct Description {
    pub plain: String,
    pub covers: Vec<usize>,
}

impl Description {
    /// new is one sentence covering the given change indices.
    pub fn new(plain: impl Into<String>, covers: Vec<usize>) -> Description {
        Description {
            plain: plain.into(),
            covers,
        }
    }

    /// one is the common case: a sentence covering a single change by index.
    pub fn one(plain: impl Into<String>, index: usize) -> Description {
        Description::new(plain, vec![index])
    }
}

/// DescribeRequestBody is the JSON the shell POSTs to the describe hook.
#[derive(Deserialize, Default)]
struct DescribeRequestBody {
    #[serde(default)]
    changes: Vec<Change>,
}

/// DescribeResponseBody is the JSON the describe hook returns.
#[derive(Serialize)]
struct DescribeResponseBody {
    descriptions: Vec<Description>,
}

// ---- the read snapshot (docs/plugins.md) ----

/// Snapshot is the brokered UCI read the shell injects: config → section → table.
pub struct Snapshot(Value);

impl Snapshot {
    /// Whether the shell successfully supplied this config, including an empty
    /// one. Missing reads must not be reported as an empty configuration.
    pub fn has_config(&self, config: &str) -> bool {
        self.0.get(config).is_some_and(Value::is_object)
    }

    /// from_value wraps an already-decoded read in the shape the shell injects,
    /// so a plugin can exercise its own snapshot mapping against a fixture.
    pub fn from_value(read: Value) -> Snapshot {
        Snapshot(read)
    }

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
        let mut secs: Vec<Section> = obj
            .values()
            .filter_map(Value::as_object)
            .filter(|m| m.get(".type").and_then(Value::as_str) == Some(typ))
            .map(Section)
            .collect();
        secs.sort_by(|a, b| a.index().total_cmp(&b.index()));
        secs
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

    /// anonymous reports whether the section was written without a name, so its
    /// handle is uci's generated `cfgXXXXXX` rather than something an operator
    /// chose. Config that identifies such a section by position (`@rule[2]`)
    /// needs this to tell the two apart.
    pub fn anonymous(&self) -> bool {
        self.0
            .get(".anonymous")
            .and_then(|v| v.as_bool().or_else(|| v.as_i64().map(|n| n != 0)))
            .unwrap_or(false)
    }

    /// index is the section's place in its file (`.index`), which orders
    /// sections of different types that one listing shows together.
    pub fn index(&self) -> f64 {
        self.0.get(".index").and_then(Value::as_f64).unwrap_or(0.0)
    }

    /// scalar reads an option as a string, or "" if unset or a list.
    pub fn scalar(&self, option: &str) -> String {
        self.0
            .get(option)
            .and_then(Value::as_str)
            .unwrap_or("")
            .to_string()
    }

    /// entries is every option the section carries, uci's own meta (`.type`,
    /// `.name`, `.index`, `.anonymous`) left out. A plugin that holds a whole
    /// section — a settings page whose rows are a catalogue of option names —
    /// needs the values it did not think to ask for, so that a save can tell an
    /// option that is set from one that is merely defaulted.
    pub fn entries(&self) -> impl Iterator<Item = (&String, &Value)> {
        self.0.iter().filter(|(option, _)| !option.starts_with('.'))
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

// ---- the brokered live reads (docs/plugins.md) ----

/// Ubus is the second read the shell injects: the result of each helper function
/// the plugin declared in `acl.read` with scope `ubus`, keyed by function name. It
/// carries live system state a plugin cannot reach itself — nftables counters, for
/// one — read with the operator's session on the plugin's behalf.
pub struct Ubus(Value);

impl Ubus {
    /// from_value wraps already-decoded brokered reads keyed by function name,
    /// the counterpart to [`Snapshot::from_value`].
    pub fn from_value(reads: Value) -> Ubus {
        Ubus(reads)
    }

    fn from_b64(b64: &str) -> Ubus {
        if b64.is_empty() {
            return Ubus(Value::Null);
        }
        match base64_decode(b64).and_then(|d| serde_json::from_slice(&d).ok()) {
            Some(v) => Ubus(v),
            None => Ubus(Value::Null),
        }
    }

    /// get returns one function's result. None means the shell brokered nothing
    /// under that name — the operator may not perform the read, the helper was
    /// unreachable, or the plugin never declared it — so render without the data
    /// rather than treating its absence as an error.
    pub fn get(&self, function: &str) -> Option<&Value> {
        self.0.get(function)
    }
}

// ---- the POST submission ----

/// Form is a decoded urlencoded submission; a field may repeat (a list posts as a
/// multi-value field). A query string is the same shape, so [`Request::query`]
/// is read through this type too.
#[derive(Default)]
pub struct Form {
    pairs: Vec<(String, String)>,
}

impl Form {
    /// parse decodes an urlencoded body — what [`serve`] hands `post`, and what a
    /// plugin's own tests submit to it.
    pub fn parse(body: &str) -> Form {
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

impl Tone {
    /// as_str is this tone's wire word — the same lowercase name the serializer
    /// writes — for the few fields that carry a tone as a plain string.
    pub fn as_str(self) -> &'static str {
        match self {
            Tone::Neutral => "neutral",
            Tone::Success => "success",
            Tone::Warning => "warning",
            Tone::Danger => "danger",
            Tone::Info => "info",
        }
    }
}

fn is_false(b: &bool) -> bool {
    !*b
}

fn is_zero(n: &u32) -> bool {
    *n == 0
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
    /// Fields belonging to one selected value. Inactive fields do not submit.
    When {
        name: String,
        value: String,
        active: bool,
        children: Vec<Widget>,
    },
    /// A titled box of child widgets — the page-layout primitive.
    Card {
        #[serde(skip_serializing_if = "String::is_empty")]
        style: String,
        #[serde(skip_serializing_if = "String::is_empty")]
        title: String,
        #[serde(skip_serializing_if = "String::is_empty")]
        subtitle: String,
        children: Vec<Widget>,
    },
    /// A titled region of the page, set apart with generous space. Meta is
    /// compact status text beside the title (an optional icon and "inline"
    /// position refine it). Control is one compact widget placed beside the
    /// title — an object's enabled switch belongs there, not in the body.
    /// Flush drops the region's own top inset where the parent already pads.
    Section(SectionWidget),
    /// Vertical rhythm for its children; draws nothing itself. Width "compact"
    /// narrows the run for a short form column; Compact tightens the rhythm,
    /// Divided draws a hairline between entries, and Inline forms a wrapping
    /// row instead of a column.
    Stack {
        #[serde(skip_serializing_if = "String::is_empty")]
        width: String,
        #[serde(skip_serializing_if = "is_false")]
        compact: bool,
        #[serde(skip_serializing_if = "is_false")]
        inline: bool,
        #[serde(skip_serializing_if = "is_false")]
        divided: bool,
        /// Adds no rhythm of its own, for a run of children that already carry
        /// theirs — form rows, which each hold 12px above and below themselves.
        #[serde(skip_serializing_if = "is_false")]
        flush: bool,
        children: Vec<Widget>,
    },
    /// Side-by-side columns; the shell owns the responsive collapse. Style
    /// "form" groups related controls with labels above them, collapsing by
    /// available container width on both pages and drawers.
    ///
    /// A "form" group of plain typed values is fused by the shell into one
    /// control: one box split by hairlines, one label, one chip naming every
    /// option. Label is the words covering every part ("Connection rate" over
    /// a rate and a burst); each part keeps its own label as its name. Help
    /// is what the group is; without it the label explains each part.
    ///
    /// Join is the word that joins the parts where a hairline would split
    /// them ("to"), for parts that read as one sentence: typed values stay one
    /// box with the word in its frame, dropdowns stand side by side with the
    /// word between them.
    Grid(Grid),
    /// A submittable set of fields; the shell threads CSRF and posts back here.
    /// Style "page" stages its submission through the shell. Error is a
    /// refusal that belongs to the whole submission rather than to one control —
    /// shown above the fields, and enough on its own to make the answer a 422,
    /// so nothing is written and the shell keeps the operator on the form.
    /// It never summarises field refusals ("check the highlighted fields"):
    /// each rides its own control, and the shell's navigator counts them.
    Form {
        #[serde(skip_serializing_if = "String::is_empty")]
        style: String,
        #[serde(skip_serializing_if = "String::is_empty")]
        submit: String,
        #[serde(skip_serializing_if = "String::is_empty")]
        error: String,
        /// A line beside the submit, for the rare fact someone must know before
        /// pressing it and cannot learn anywhere else. An act says what it does
        /// and carries no explanation otherwise; leave this empty.
        #[serde(skip_serializing_if = "String::is_empty")]
        note: String,
        /// Where the options this form's controls write live,
        /// "config.section", when the form writes one uci section. Set it
        /// through `at`.
        #[serde(skip_serializing_if = "String::is_empty")]
        target: String,
        fields: Vec<Widget>,
    },
    /// One labelled input with a declared datatype the shell enforces (ADR-008).
    /// Kind picks the control ("text", "select", "checks", "hidden",
    /// "datetime-local", …); options feed a select or a set of checks, values
    /// are the checked members of that set; placeholder hints at the shape of a
    /// text value; error is the inline validation message (422).
    Field(Field),
    /// One persistent on/off setting, sharing its control with table toggle
    /// cells so a thing's enabled state looks the same in a listing and in its
    /// editor. Style "inline" sits it beside a section heading; style "locked"
    /// states a state nothing here changes — drawn set or clear, inert,
    /// posting nothing. A switch inside a form posts only when it is on.
    ///
    /// Switches alone in a labelled form grid are one setting asked of several
    /// things: the shell draws the grid's label once, with the options named
    /// once, and a checkbox row per switch, its `help` kept in view.
    Switch(Switch),
    /// The optional-match builder: the plugin declares the complete catalogue of
    /// conditions and which of them the object currently carries; the shell
    /// renders the active ones and keeps the rest in its Add-condition picker.
    /// An inactive item's fields are inert markup, so they post nothing — an
    /// absent field name in a submission means that condition is gone.
    Conditions {
        label: String,
        #[serde(skip_serializing_if = "String::is_empty")]
        help: String,
        items: Vec<ConditionItem>,
    },
    /// An expand/collapse region: the summary line, and the contents revealed
    /// beneath it. The home for the advanced-but-rarely-touched. Style
    /// "reveal" folds optional rows of a form behind one quiet act instead of
    /// a framed box; Open has it arrive unfolded, which a reveal does whenever
    /// what it folds already holds something.
    Disclosure {
        #[serde(skip_serializing_if = "String::is_empty")]
        style: String,
        summary: String,
        #[serde(skip_serializing_if = "is_false")]
        open: bool,
        children: Vec<Widget>,
    },
    /// A labelled hyperlink, optionally styled as a button. The href is the
    /// plugin's to choose — a route inside the shell, most often — and the shell
    /// owns the look and the URL policy.
    Link {
        #[serde(skip_serializing_if = "String::is_empty")]
        desc: String,
        #[serde(skip_serializing_if = "String::is_empty")]
        code: String,
        label: String,
        #[serde(skip_serializing_if = "String::is_empty")]
        icon: String,
        href: String,
        #[serde(skip_serializing_if = "String::is_empty")]
        style: String,
        /// The status style's act: the words on the control that changes the
        /// status (the label), where a setting's control stands.
        #[serde(skip_serializing_if = "String::is_empty")]
        act: String,
        /// Opens the destination as a drawer over this page rather than
        /// leaving it — for a destination that is its own thing, such as a
        /// package to install (`/system/packages/package?name=…`) or a
        /// certificate to replace. The status and act styles honour it.
        #[serde(skip_serializing_if = "is_false")]
        panel: bool,
    },
    /// A field-set gated by a toggle: `fields` show while it is on, `otherwise`
    /// while it is off. The shell realizes the show/hide (ADR-005 §7); the
    /// toggle posts under `name` so the plugin reads the chosen branch.
    Conditional {
        name: String,
        label: String,
        checked: bool,
        /// The option the toggle writes, worn as the mono chip a field's label
        /// wears — the gate is a form row like the ones it gates.
        #[serde(skip_serializing_if = "String::is_empty")]
        key: String,
        /// The line under the toggle: what turning it on means, in the place a
        /// field's helper line sits.
        #[serde(skip_serializing_if = "String::is_empty")]
        help: String,
        fields: Vec<Widget>,
        #[serde(skip_serializing_if = "Vec::is_empty")]
        otherwise: Vec<Widget>,
    },
    /// A repeatable input sharing one name — a list of ports, hosts, CIDRs.
    /// Style "tokens" is the compact editor for many short values, each removable
    /// on its own, with Prompt as the add-field's hint. Errors are keyed by an
    /// item's index as a string, which is how a repeating control says which row
    /// failed. An empty list posts nothing under its name.
    List(List),
    /// A boxed contextual notice beside content, toned by intent. An action's
    /// *outcome* belongs in the envelope's notice, not here.
    Callout {
        variant: Tone,
        #[serde(skip_serializing_if = "String::is_empty")]
        title: String,
        body: String,
        #[serde(skip_serializing_if = "is_false")]
        compact: bool,
        /// Links that resolve what the notice says (install what is missing),
        /// drawn as buttons inside the band, a cell under its words, in its ink.
        #[serde(skip_serializing_if = "Vec::is_empty")]
        acts: Vec<Widget>,
    },
    /// A machine value in a monospace box, with an inline copy button.
    Code {
        #[serde(skip_serializing_if = "String::is_empty")]
        label: String,
        value: String,
        copy: bool,
        /// Declares the block a preview of the form it sits in: what that form
        /// would write, kept current as it is edited rather than as it stood
        /// when the page was built. Intent only — the shell realizes it by
        /// asking this plugin to re-render from the values on screen, so the
        /// answer stays the plugin's reading of the daemon's grammar and never
        /// becomes a second copy of it in the browser.
        #[serde(skip_serializing_if = "is_false")]
        live: bool,
        /// The grammar the value is written in, when the shell should read
        /// it line by line rather than as one string: `uci` for a section as
        /// `/etc/config` spells it, so a keyword reads apart from the key and
        /// value after it. Empty is an opaque value — a key, a token — shown
        /// whole.
        #[serde(skip_serializing_if = "String::is_empty")]
        grammar: String,
    },
    /// A whole-page state that is not a listing — a page waiting on a
    /// precondition: icon, headline, reassurance, and the call(s) to action as
    /// children. A listing with nothing in it says so in the table's
    /// `empty_text` row instead.
    Empty {
        icon: String,
        title: String,
        body: String,
        /// Tones the icon by the tone vocabulary ("success" for a state that
        /// has arrived); empty wears the accent.
        #[serde(skip_serializing_if = "String::is_empty")]
        variant: String,
        children: Vec<Widget>,
    },
    /// A label/value fact sheet. Build it with [`Widget::properties`]. `align`
    /// "left" sets the values beside a fixed label column rather than on the
    /// right edge — the reading order for a document's facts, read line by line.
    Properties {
        items: Vec<Property>,
        #[serde(skip_serializing_if = "String::is_empty")]
        align: String,
    },
    /// Config sections as identical rows under fixed columns: each column
    /// declares a kind ("name", "mono", "keyword", "pill", "endpoint",
    /// "toggle", "comment", …) and that kind renders every cell in it the same
    /// way — rows cannot vary in shape, which is the point. Style "flat" (the
    /// default) draws bare hairline rows; Title/Detail draw the header band
    /// above them; Condensed tightens the rhythm; Align "top" pins cells to the
    /// row's top line. A listing whose sequence is meaning — evaluation order —
    /// adds a leading `{kind: "reorder"}` column and names the uci config its
    /// rows are sections of in `reorder_config`: the shell then drags the rows,
    /// posts the new sequence, and stages a `uci order` on that config. Row ids
    /// are the section names it reorders, and a row's group bounds the drag.
    /// With no rows at all the shell drops the column heads — they describe data
    /// — and states `empty_text` in one quiet row; leave it blank and the shell
    /// says so in its own words.
    ///
    /// A listing that is a run of events rather than a state of the config sets
    /// `stream` (see [`TableStream`]): it renders with no rows and the shell
    /// fills it as events arrive.
    Table(Table),
    /// The listing's own controls, between a page's heading and its rows: which
    /// slice you are looking at and how to narrow it. Everything on it acts on
    /// the rows below and nothing else, and all of it is client-side — nothing
    /// it does is a request, and nothing it does can fail. Tabs are the coarse
    /// cut, each priced with its own count and matched against a row's `tags`;
    /// `filter` is the free-text one, drawn over a live listing only. The
    /// page's own act is not here: it is the envelope's ([`HeadingAct`]).
    ActionBar(ActionBar),
    /// A direct action. Without `name` the button is inert — which is what a
    /// control the shell drives (a live listing's pause) wants. `live` says
    /// that the thing this button governs is running: the spinner turns beside
    /// the label and every affordance stays, because pressing it is how the
    /// running thing stops. `loading` is the other spinner — an in-flight
    /// submit — and it disables the button while it turns.
    Button {
        label: String,
        #[serde(skip_serializing_if = "String::is_empty")]
        icon: String,
        #[serde(skip_serializing_if = "String::is_empty")]
        style: String,
        #[serde(skip_serializing_if = "String::is_empty")]
        name: String,
        #[serde(skip_serializing_if = "String::is_empty")]
        value: String,
        #[serde(skip_serializing_if = "is_false")]
        disabled: bool,
        #[serde(skip_serializing_if = "is_false")]
        loading: bool,
        #[serde(skip_serializing_if = "is_false")]
        live: bool,
    },
    /// A block of option rows: a plainly-named option, a one-line description,
    /// the underlying option name as a code chip, and its state hard right — a
    /// switch for an on/off option, pills for a row that reads rather than
    /// toggles. Style "card" boxes the block. A Seam folds the block's long
    /// tail of rare options behind a collapsed line inside the same block —
    /// state the tail as it is, and the shell folds it only once there are
    /// three rows to fold: below that the fold's own line costs the height it
    /// would save, so those rows render on the block.
    Settings(Settings),
    /// The page-wide lens: one field that narrows every listing on the page at
    /// once. The plugin declares only the placeholder; the shell owns the
    /// behaviour — and whether the lens renders at all. Only a page with a live
    /// listing keeps it: a page that holds what it has is scrolled and searched
    /// with the browser's own find, however long it is, so there the shell
    /// removes the widget. State the lens your page would want and carry no
    /// rule of your own.
    Filter {
        #[serde(skip_serializing_if = "String::is_empty")]
        placeholder: String,
    },
    /// A short run of styled prose (Markdown), inline in a composition.
    Text { markdown: String },
    /// A destructive action behind an explicit confirmation.
    Confirm {
        trigger: String,
        /// An optional bold heading above the message, for a consequence too
        /// long to read as one paragraph.
        #[serde(skip_serializing_if = "String::is_empty")]
        title: String,
        message: String,
        #[serde(skip_serializing_if = "String::is_empty")]
        confirm: String,
        #[serde(skip_serializing_if = "String::is_empty")]
        cancel: String,
    },
    /// A short set of kept machine strings — authorized keys, a tunnel's
    /// peers — each its identity over a line of detail, removed in place after
    /// asking, the next added from a slot at the foot that unfolds where it
    /// stands. No rules: the set stands on space. See [`CollectionItem`] and
    /// [`CollectionAdd`].
    Collection {
        items: Vec<CollectionItem>,
        #[serde(skip_serializing_if = "String::is_empty")]
        empty: String,
        #[serde(skip_serializing_if = "Option::is_none")]
        add: Option<CollectionAdd>,
    },
    /// The governed bridge: display-only Markdown **prose** no widget shapes
    /// (ADR-005 §4). Metered — its usage is the demand signal for the next
    /// widget.
    Raw { markdown: String },
}

/// SectionWidget is the body of [`Widget::Section`]; set what differs from the
/// default.
#[derive(Serialize, Debug, Default)]
pub struct SectionWidget {
    pub title: String,
    /// The glyph of what the section is, by Lucide name, leading its
    /// title on a part's ledger line (an expanded row's network, its DHCP
    /// server, the bridge under them), so parts of one kind are told
    /// apart from parts of another at a glance. Identity, never state.
    #[serde(skip_serializing_if = "String::is_empty")]
    pub icon: String,
    /// The name this section is addressed by, so a link elsewhere on the page
    /// can bring it into view. A page long enough to be worth listing the
    /// parts of needs them addressable.
    #[serde(skip_serializing_if = "String::is_empty")]
    pub anchor: String,
    /// Sets the title as a small label over a group rather than a heading
    /// over a subject — a rail's "On this page", whose words name the list
    /// beneath them and must not compete with the headings it points at.
    #[serde(skip_serializing_if = "is_false")]
    pub kicker: bool,
    #[serde(skip_serializing_if = "String::is_empty")]
    pub sub: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    pub meta: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    pub meta_icon: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    pub meta_position: String,
    #[serde(skip_serializing_if = "is_false")]
    pub flush: bool,
    /// Rule this section off from whatever precedes it. It is the only rule
    /// a form draws, which is what makes it read as a boundary: fields
    /// inside a section are separated by space, sections by a hairline.
    #[serde(skip_serializing_if = "is_false")]
    pub hairline: bool,
    /// Where the options this section's controls write live,
    /// "config.section", when they all live in one place — said once here
    /// rather than on each control. Set it through `at`.
    #[serde(skip_serializing_if = "String::is_empty")]
    pub target: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub control: Option<Box<Widget>>,
    pub children: Vec<Widget>,
}

/// Grid is the body of [`Widget::Grid`]; set what differs from the default.
#[derive(Serialize, Debug, Default)]
pub struct Grid {
    #[serde(skip_serializing_if = "String::is_empty")]
    pub style: String,
    pub columns: u32,
    pub children: Vec<Widget>,
    #[serde(skip_serializing_if = "String::is_empty")]
    pub label: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    pub help: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    pub join: String,
}

/// Field is the body of [`Widget::Field`]; set what differs from the default.
#[derive(Serialize, Debug, Default)]
pub struct Field {
    pub name: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    pub label: String,
    pub kind: String,
    pub value: String,
    #[serde(skip_serializing_if = "Vec::is_empty")]
    pub values: Vec<String>,
    #[serde(skip_serializing_if = "String::is_empty")]
    pub placeholder: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    pub datatype: String,
    #[serde(skip_serializing_if = "Vec::is_empty")]
    pub options: Vec<SelectOption>,
    #[serde(skip_serializing_if = "String::is_empty")]
    pub error: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    pub help: String,
    /// The option this field writes, verbatim — "ipaddr", "leasetime". The
    /// shell sets it as a mono chip beside the label, so someone who knows
    /// the config can see which line they are editing without leaving the
    /// form, and someone who does not can ignore it.
    #[serde(skip_serializing_if = "String::is_empty")]
    pub key: String,
    /// Where `key` lives, "config.section", when the form or section around
    /// the field does not already say it. With `key` it is the option's full
    /// address, which is how the shell marks a field whose change waits on
    /// the stage, on every visit. Set it through `at`.
    #[serde(skip_serializing_if = "String::is_empty")]
    pub target: String,
    /// What the field is, for someone meeting it for the first time — raised
    /// from the label rather than kept on screen, because `help` already
    /// says what to put in the control and a paragraph beside every row
    /// would drown it. Set both through `explained`.
    #[serde(skip_serializing_if = "String::is_empty")]
    pub tip: String,
    /// What reads the option — "dhcp host", "ip route". The shell pairs it
    /// with `key` to close the tip, placing the setting in the config it
    /// belongs to.
    #[serde(skip_serializing_if = "String::is_empty")]
    pub source: String,
    /// What the number in the box is counted in — "Mbit/s", "seconds". The
    /// shell sets it inside the field's trailing edge, so the value and what
    /// it means read as one thing.
    #[serde(skip_serializing_if = "String::is_empty")]
    pub unit: String,
    /// The row's trailing remove affordance, for a form whose rows are a set
    /// someone adds to and takes from rather than a fixed list of settings:
    /// "yes" draws the glyph that clears this row, "lane" reserves its width
    /// on a row that cannot be removed, "" draws neither. Set "lane" on the
    /// fixed rows of such a form — without it their controls sit 28px left
    /// of the removable ones.
    #[serde(skip_serializing_if = "String::is_empty")]
    pub remove: String,
    /// The control's compact face where its option set is short enough to
    /// show whole: "segmented" draws a `checks` field as one strip of
    /// togglable chips rather than a grid of boxes. A set long enough to
    /// wrap belongs in the grid. For single-choice `select` fields the
    /// shell chooses radios or a native dropdown by option count,
    /// including when this style is supplied.
    #[serde(skip_serializing_if = "String::is_empty")]
    pub style: String,
    /// Marks a `select` whose value decides which fields the form has — what
    /// kind of object a New drawer makes. Set it through `reshapes`. A `text`
    /// file field may reshape too: the form is drawn again around the file.
    #[serde(skip_serializing_if = "is_false")]
    pub reshapes: bool,
    /// A `file` field's accepted types, as the browser's picker reads them:
    /// ".ovpn,.conf".
    #[serde(skip_serializing_if = "String::is_empty")]
    pub accept: String,
    /// A `file` field's sentence in its drop area: "Drop the profile here".
    #[serde(skip_serializing_if = "String::is_empty")]
    pub prompt: String,
    /// The name of the file a `file` field of style `text` already read. That
    /// style has the browser read the chosen file and post what it says as
    /// the field's value, and its name as `<name>_name`; a form drawn again
    /// around it sets both back, so it still says which file it holds.
    #[serde(skip_serializing_if = "String::is_empty")]
    pub chosen: String,
}

/// Switch is the body of [`Widget::Switch`]; set what differs from the default.
#[derive(Serialize, Debug, Default)]
pub struct Switch {
    pub name: String,
    pub label: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    pub off_label: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    pub help: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    pub style: String,
    /// The option this switch writes, verbatim — "drop_invalid". It rides
    /// beside the label as a mono chip exactly as a field's does: a state to
    /// flip is as much a line of the config as a value to type.
    #[serde(skip_serializing_if = "String::is_empty")]
    pub key: String,
    /// Where `key` lives, as a field's `target`. Set it through `at`.
    #[serde(skip_serializing_if = "String::is_empty")]
    pub target: String,
    /// The longer answer to "what even is this", raised onto the label. Help
    /// is the same sentence said as a plugin already wrote it; where both
    /// are set the label raises this one.
    #[serde(skip_serializing_if = "String::is_empty")]
    pub tip: String,
    /// What reads the option — "firewall defaults". With `key` it makes the
    /// tip's footer, placing the option in the config it belongs to.
    #[serde(skip_serializing_if = "String::is_empty")]
    pub source: String,
    /// The label is a machine string — a path — rather than words: set in
    /// mono and never looked up in a catalog.
    #[serde(skip_serializing_if = "is_false")]
    pub verbatim: bool,
    /// The refusal of the state the switch was set to, drawn as a field's
    /// is: the band under the label, named by the checkbox.
    #[serde(skip_serializing_if = "String::is_empty")]
    pub error: String,
    #[serde(skip_serializing_if = "is_false")]
    pub on: bool,
}

/// List is the body of [`Widget::List`]; set what differs from the default.
#[derive(Serialize, Debug, Default)]
pub struct List {
    pub name: String,
    pub label: String,
    pub kind: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    pub style: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    pub prompt: String,
    pub datatype: String,
    pub items: Vec<String>,
    #[serde(skip_serializing_if = "std::collections::BTreeMap::is_empty")]
    pub errors: std::collections::BTreeMap<String, String>,
    #[serde(skip_serializing_if = "String::is_empty")]
    pub help: String,
    /// The option this list writes, verbatim, for the mono chip beside the
    /// label — a list of values is as much a line of the config as one value.
    #[serde(skip_serializing_if = "String::is_empty")]
    pub key: String,
    /// Where `key` lives, as a field's `target`. Set it through `at`.
    #[serde(skip_serializing_if = "String::is_empty")]
    pub target: String,
    /// The longer explanation, raised onto the label as a field's is.
    #[serde(skip_serializing_if = "String::is_empty")]
    pub tip: String,
    /// The values worth offering. The control suggests them as it is typed in
    /// and takes as many as are wanted, while still accepting anything the
    /// config accepts — a closed choice would be wrong where the daemon reads
    /// more than anyone would think to list.
    #[serde(skip_serializing_if = "Vec::is_empty")]
    pub options: Vec<SelectOption>,
    /// The row's trailing remove affordance, as a field's: "yes" draws the
    /// glyph that clears the row, "lane" reserves its width.
    #[serde(skip_serializing_if = "String::is_empty")]
    pub remove: String,
}

/// Table is the body of [`Widget::Table`]; set what differs from the default.
#[derive(Serialize, Debug, Default)]
pub struct Table {
    #[serde(skip_serializing_if = "String::is_empty")]
    pub style: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    pub title: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    pub detail: String,
    /// Pulls the row's edge inset in for a listing carrying many columns: the
    /// same rows at the same height, with less of the measure spent holding
    /// values off the table's edge. A judgement about horizontal room, so the
    /// listing that knows how many columns it has is the one that says it.
    #[serde(skip_serializing_if = "is_false")]
    pub dense: bool,
    #[serde(skip_serializing_if = "String::is_empty")]
    pub reorder_config: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    pub reorder_label: String,
    pub columns: Vec<TableColumn>,
    pub rows: Vec<TableRow>,
    #[serde(skip_serializing_if = "String::is_empty")]
    pub drawer_label: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    pub drawer_icon: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    pub empty_text: String,
    /// A quiet add affordance the shell renders after the last row — the
    /// next entry of the listing, where it will land. For a grouped table,
    /// prefer per-lane adds on [`TableGroup`]. Empty renders no tail row.
    #[serde(skip_serializing_if = "String::is_empty")]
    pub add_label: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    pub add_href: String,
    /// The one caveat or reading note about the listing, set under the last
    /// row: what the order means, what the listing leaves out. Below the
    /// grid rather than above it, because it is a caption for something the
    /// reader has already seen rather than a paragraph to get past first.
    #[serde(skip_serializing_if = "String::is_empty")]
    pub note: String,
    /// The live source this listing's rows arrive from, if any.
    #[serde(skip_serializing_if = "Option::is_none")]
    pub stream: Option<TableStream>,
}

/// ActionBar is the body of [`Widget::ActionBar`]; set what differs from the
/// default.
#[derive(Serialize, Debug, Default)]
pub struct ActionBar {
    #[serde(skip_serializing_if = "Vec::is_empty")]
    pub tabs: Vec<ActionTab>,
    /// The search field's placeholder. The shell draws the field over a live
    /// listing only; over a still one the browser's own find serves.
    #[serde(skip_serializing_if = "String::is_empty")]
    pub filter: String,
    /// The label of the control that holds a running listing still —
    /// "Live" — which the shell turns to Paused and back as it is
    /// pressed. It is the one thing on the bar that is not a narrowing,
    /// and it belongs here because what it governs is the rows.
    #[serde(skip_serializing_if = "String::is_empty")]
    pub live: String,
}

/// HeadingAct is a page's one act, which the shell stands on the heading line
/// beside the title: making a new subject of the kind the page lists, or,
/// quietly (`style: "quiet"`), taking something away from it (a log's
/// download). It is the page's, not the listing's, so it travels in the
/// envelope ([`Envelope::with_act`]) rather than in the widget tree. Making one
/// is editing one that does not exist yet, so an act may open a panel:
/// `opens_panel` says its href is a panel this plugin renders, `drawer`
/// carries the blank object's panel itself. The drawer's own `open` makes an
/// address asking for a new object arrive with the panel in front of the
/// operator; the href is what a browser with no script follows.
#[derive(Serialize, Debug, Default)]
pub struct HeadingAct {
    pub label: String,
    pub href: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    pub icon: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    pub style: String,
    #[serde(skip_serializing_if = "is_false")]
    pub opens_panel: bool,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub drawer: Option<RowDrawer>,
}

/// Settings is the body of [`Widget::Settings`]; set what differs from the
/// default.
#[derive(Serialize, Debug, Default)]
pub struct Settings {
    #[serde(skip_serializing_if = "String::is_empty")]
    pub style: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    pub title: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    pub meta: String,
    /// Lowers the row padding only — same anatomy, tighter, for a list of
    /// many short facts.
    #[serde(skip_serializing_if = "is_false")]
    pub condensed: bool,
    pub items: Vec<SettingsItem>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub seam: Option<SettingsSeam>,
}

impl Widget {
    /// refusal is a form's `error` from a submission's refusals keyed by
    /// control name: the first one no control among `fields` draws, however
    /// deep it stands. A refusal a field or switch already carries is never
    /// said again above the form; one no control draws is the form's, so it
    /// is never lost.
    pub fn refusal(
        fields: &[Widget],
        errors: &std::collections::BTreeMap<String, String>,
    ) -> String {
        errors
            .iter()
            .find(|(name, _)| !fields.iter().any(|w| w.draws_refusal(name)))
            .map(|(_, e)| e.clone())
            .unwrap_or_default()
    }
    fn draws_refusal(&self, key: &str) -> bool {
        match self {
            Widget::Field(Field { name, error, .. })
            | Widget::Switch(Switch { name, error, .. }) => name == key && !error.is_empty(),
            // A list's refusals ride its rows, keyed by row, so it draws the
            // refusal named for the list when any row carries one.
            Widget::List(List { name, errors, .. }) => name == key && !errors.is_empty(),
            Widget::Conditional {
                fields, otherwise, ..
            } => fields.iter().chain(otherwise).any(|w| w.draws_refusal(key)),
            Widget::When { children, .. }
            | Widget::Card { children, .. }
            | Widget::Section(SectionWidget { children, .. })
            | Widget::Stack { children, .. }
            | Widget::Grid(Grid { children, .. })
            | Widget::Disclosure { children, .. }
            | Widget::Empty { children, .. } => children.iter().any(|w| w.draws_refusal(key)),
            _ => false,
        }
    }
    /// card is a container of child widgets, optionally titled.
    pub fn card(title: &str, children: Vec<Widget>) -> Widget {
        Widget::Card {
            style: String::new(),
            title: title.into(),
            subtitle: String::new(),
            children,
        }
    }

    /// section is a titled region of the page.
    pub fn section(title: &str, sub: &str, children: Vec<Widget>) -> Widget {
        Widget::Section(SectionWidget {
            title: title.into(),
            sub: sub.into(),
            children,
            ..Default::default()
        })
    }

    /// ruled draws this section off from whatever precedes it — the boundary a
    /// form's continuation needs when the fields above it carry no rules of
    /// their own.
    pub fn ruled(mut self) -> Widget {
        if let Widget::Section(SectionWidget { hairline, .. }) = &mut self {
            *hairline = true;
        }
        self
    }

    /// addressed_as names this section so a link on the same page can bring it
    /// into view. The name is the page's to choose and has to match what the link
    /// points at, so keep the two in one list rather than spelling each twice.
    pub fn addressed_as(mut self, anchor: &str) -> Widget {
        if let Widget::Section(SectionWidget { anchor: a, .. }) = &mut self {
            *a = anchor.into();
        }
        self
    }

    /// kicker sets this section's title as the small label over a list rather
    /// than a heading over a subject — what a rail's "On this page" is.
    pub fn kicker(mut self) -> Widget {
        if let Widget::Section(SectionWidget { kicker, .. }) = &mut self {
            *kicker = true;
        }
        self
    }

    /// flush drops a section's own air above its heading, for a section that is
    /// the whole of the surface it sits on — a panel's body, where the container
    /// has already set the distance from the edge and a second one would push
    /// the heading off the top of what it heads.
    /// On a stack it means the same thing one step in: add no rhythm at all, for
    /// a run of children that carry their own — form rows, or a list of links that
    /// is one continuous column rather than several separate lines.
    pub fn flush(mut self) -> Widget {
        match &mut self {
            Widget::Section(SectionWidget { flush, .. }) | Widget::Stack { flush, .. } => {
                *flush = true
            }
            _ => {}
        }
        self
    }

    /// explained hangs the answer to "what even is this" off a field's label:
    /// `what` is the sentence, `source` names what reads the option, and the
    /// shell closes the tip with that beside the option's own name. Reach for it
    /// where the label is a term of art the operator did not choose — a DUID, an
    /// interface identifier — and leave it off where the label is already the
    /// plain word for the thing. Only a field carries one; anything else comes
    /// back as it was.
    pub fn explained(mut self, what: &str, source: &str) -> Widget {
        if let Widget::Field(Field { tip, source: s, .. }) = &mut self {
            *tip = what.into();
            *s = source.into();
        }
        self
    }

    /// reshapes makes a `select` the choice that decides which fields its form
    /// has. Changing it posts the form with `_action=reshape` — the shell's
    /// mark, never a save — and the plugin answers with the form in the shape
    /// now chosen, carrying over what still applies; the shell puts it in the
    /// old one's place and stages nothing. A choice that only shows or hides
    /// fields within one shape is a [`Widget::When`], which needs no round trip.
    pub fn reshapes(mut self) -> Widget {
        if let Widget::Field(Field { reshapes, .. }) = &mut self {
            *reshapes = true;
        }
        self
    }

    /// counted_in names what the number in a field is counted in — "Mbit/s",
    /// "seconds" — for the unit the shell sets inside the box. It belongs on a
    /// field whose value is a bare number; a value that carries its own unit
    /// ("12h") states it already.
    pub fn counted_in(mut self, unit: &str) -> Widget {
        if let Widget::Field(Field { unit: u, .. }) = &mut self {
            *u = unit.into();
        }
        self
    }

    /// typed declares the grammar of a field's value by a shell datatype
    /// ("timehhmmss", "dateyyyymmdd", "ipaddr"): the shell sets it as the
    /// machine string it is, at the measure it needs, and refuses a value
    /// that does not read as one.
    pub fn typed(mut self, grammar: &str) -> Widget {
        if let Widget::Field(Field { datatype, .. }) = &mut self {
            *datatype = grammar.into();
        }
        self
    }

    /// locked fixes a field's value where it stands: the shell draws it in the
    /// field's own row, a quiet box with a padlock, the reason it cannot
    /// change raised on its label, and posts nothing.
    pub fn locked(mut self, reason: &str) -> Widget {
        if let Widget::Field(Field { style, help, .. }) = &mut self {
            *style = "locked".into();
            *help = reason.into();
        }
        self
    }

    /// writes names the option this field sets, verbatim, for the mono chip the
    /// shell puts beside the label. Reach for it where the config's own name for
    /// the setting is worth showing to someone who already knows it.
    pub fn writes(mut self, key: &str) -> Widget {
        match &mut self {
            Widget::Field(Field { key: k, .. }) | Widget::List(List { key: k, .. }) => {
                *k = key.into()
            }
            _ => {}
        }
        self
    }

    /// labelled names a form group whose values the shell fuses into one
    /// control: the words covering every part, and optionally what the group
    /// is. Each field keeps its own label as its part's name.
    pub fn labelled(mut self, label: &str, help: &str) -> Widget {
        if let Widget::Grid(Grid {
            label: l, help: h, ..
        }) = &mut self
        {
            *l = label.into();
            *h = help.into();
        }
        self
    }

    /// joined names the word that joins a form group's parts ("to"), for
    /// parts that read as one sentence — a path from one zone to another.
    pub fn joined(mut self, word: &str) -> Widget {
        if let Widget::Grid(Grid { join, .. }) = &mut self {
            *join = word.into();
        }
        self
    }

    /// at says where the options written here live — the uci config and the
    /// section, by the name the snapshot gives it — so the shell can mark a
    /// control whose change waits on the stage, on every visit until it is
    /// applied. On a form or section it holds for every control inside that
    /// names no place of its own; on a field, switch or list it is that
    /// control's own. A section not yet written has no name, and so no
    /// address; anything else comes back as it was.
    pub fn at(mut self, config: &str, section: &str) -> Widget {
        if config.is_empty() || section.is_empty() {
            return self;
        }
        match &mut self {
            Widget::Form { target, .. }
            | Widget::Section(SectionWidget { target, .. })
            | Widget::Field(Field { target, .. })
            | Widget::Switch(Switch { target, .. })
            | Widget::List(List { target, .. }) => *target = format!("{config}.{section}"),
            _ => {}
        }
        self
    }

    /// suggesting offers the values worth offering on a list, shown as it is
    /// typed in. It is a suggestion and not a restriction: whatever the config
    /// accepts can still be typed.
    pub fn suggesting(mut self, options: Vec<SelectOption>) -> Widget {
        if let Widget::List(List { options: o, .. }) = &mut self {
            *o = options;
        }
        self
    }

    /// removable makes this row one someone can take out of the set — the shell
    /// ends it with the glyph that clears it. Every other row in the same group
    /// sets its `remove` to "lane", or their controls sit where the glyph is.
    pub fn removable(mut self) -> Widget {
        match &mut self {
            Widget::Field(Field { remove, .. }) | Widget::List(List { remove, .. }) => {
                *remove = "yes".into()
            }
            _ => {}
        }
        self
    }

    /// segmented draws a `checks` field as one strip of togglable chips instead
    /// of a grid of boxes — for a set short enough to show whole, where which
    /// members are on is a shape rather than a list to read.
    pub fn segmented(mut self) -> Widget {
        if let Widget::Field(Field { style, .. }) = &mut self {
            *style = "segmented".into();
        }
        self
    }

    /// gate is a field-set behind a toggle: `fields` while it is on, `otherwise`
    /// while it is off. Key is the option the toggle writes and help the line
    /// under it, so the gate reads as the form row it is.
    pub fn gate(
        name: &str,
        label: &str,
        key: &str,
        help: &str,
        checked: bool,
        fields: Vec<Widget>,
        otherwise: Vec<Widget>,
    ) -> Widget {
        Widget::Conditional {
            name: name.into(),
            label: label.into(),
            key: key.into(),
            help: help.into(),
            checked,
            fields,
            otherwise,
        }
    }

    /// stack lays children out vertically; the rhythm variants via the variant.
    pub fn stack(children: Vec<Widget>) -> Widget {
        Widget::Stack {
            width: String::new(),
            compact: false,
            inline: false,
            divided: false,
            flush: false,
            children,
        }
    }

    /// grid lays children out across columns.
    pub fn grid(columns: u32, children: Vec<Widget>) -> Widget {
        Widget::Grid(Grid {
            columns,
            children,
            ..Default::default()
        })
    }

    /// form_grid groups related fields in reading order. The shell owns their
    /// spacing and collapses the columns when the containing form is narrow.
    pub fn form_grid(columns: u32, children: Vec<Widget>) -> Widget {
        Widget::Grid(Grid {
            style: "form".into(),
            columns,
            children,
            ..Default::default()
        })
    }

    /// disclosure folds detail away behind a summary line.
    pub fn disclosure(summary: &str, children: Vec<Widget>) -> Widget {
        Widget::Disclosure {
            style: String::new(),
            summary: summary.into(),
            open: false,
            children,
        }
    }

    /// reveal folds optional rows of a form — a condition's exceptions —
    /// behind one quiet act ("Exclude some"). It arrives open when what it
    /// folds already holds something, so nothing the object says is hidden.
    pub fn reveal(summary: &str, open: bool, children: Vec<Widget>) -> Widget {
        Widget::Disclosure {
            style: "reveal".into(),
            summary: summary.into(),
            open,
            children,
        }
    }

    /// link points somewhere; style "" is a link, "button" a call to action.
    pub fn link(label: &str, href: &str, style: &str) -> Widget {
        Widget::Link {
            desc: String::new(),
            code: String::new(),
            label: label.into(),
            icon: String::new(),
            href: href.into(),
            style: style.into(),
            act: String::new(),
            panel: false,
        }
    }

    /// switch is one on/off setting, labelled and in the plain row style.
    pub fn switch(name: &str, label: &str, on: bool) -> Widget {
        Widget::switch_keyed(name, label, "", "", on)
    }

    /// switch_keyed is one on/off setting that says which option it writes and
    /// what the option is for: the same row a keyed field draws, with a switch
    /// where the box would be.
    pub fn switch_keyed(name: &str, label: &str, key: &str, tip: &str, on: bool) -> Widget {
        Widget::Switch(Switch {
            name: name.into(),
            label: label.into(),
            key: key.into(),
            tip: tip.into(),
            on,
            ..Default::default()
        })
    }

    /// form is a submittable set of fields with the given submit label.
    pub fn form(submit: &str, fields: Vec<Widget>) -> Widget {
        Widget::Form {
            style: String::new(),
            submit: submit.into(),
            error: String::new(),
            fields,
            note: String::new(),
            target: String::new(),
        }
    }

    /// field is a single text input carrying a datatype the shell enforces.
    pub fn field(name: &str, label: &str, value: &str, datatype: &str, help: &str) -> Widget {
        Widget::Field(Field {
            name: name.into(),
            label: label.into(),
            kind: "text".into(),
            value: value.into(),
            datatype: datatype.into(),
            help: help.into(),
            ..Default::default()
        })
    }

    /// select is a single-choice field over a closed option set.
    pub fn select(
        name: &str,
        label: &str,
        value: &str,
        options: Vec<SelectOption>,
        error: &str,
    ) -> Widget {
        Widget::Field(Field {
            name: name.into(),
            label: label.into(),
            kind: "select".into(),
            value: value.into(),
            options,
            error: error.into(),
            ..Default::default()
        })
    }

    /// checks is membership in a set: each option is included or not, and the
    /// checked ones post under the one name. State of a thing is a switch.
    pub fn checks(
        name: &str,
        label: &str,
        values: &[String],
        options: Vec<SelectOption>,
    ) -> Widget {
        Widget::Field(Field {
            name: name.into(),
            label: label.into(),
            kind: "checks".into(),
            values: values.to_vec(),
            options,
            ..Default::default()
        })
    }

    /// hidden is the bare value carrier a form posts but a person never edits.
    pub fn hidden(name: &str, value: &str) -> Widget {
        Widget::Field(Field {
            name: name.into(),
            kind: "hidden".into(),
            value: value.into(),
            ..Default::default()
        })
    }

    /// list is a repeatable text input (one value per row) of one datatype.
    pub fn list(name: &str, label: &str, datatype: &str, items: &[String], help: &str) -> Widget {
        Widget::List(List {
            name: name.into(),
            label: label.into(),
            kind: "text".into(),
            datatype: datatype.into(),
            items: items.to_vec(),
            help: help.into(),
            ..Default::default()
        })
    }

    /// tokens is the compact list style: many short values, each removable, with
    /// Prompt hinting what one looks like.
    pub fn tokens(name: &str, label: &str, prompt: &str, items: &[String], help: &str) -> Widget {
        Widget::List(List {
            name: name.into(),
            label: label.into(),
            kind: "text".into(),
            style: "tokens".into(),
            prompt: prompt.into(),
            items: items.to_vec(),
            help: help.into(),
            ..Default::default()
        })
    }

    /// callout is a boxed contextual notice beside content.
    pub fn callout(variant: Tone, title: &str, body: &str) -> Widget {
        Widget::Callout {
            variant,
            title: title.into(),
            body: body.into(),
            compact: false,
            acts: Vec::new(),
        }
    }

    /// code shows a machine value with a copy button.
    pub fn code(label: &str, value: &str) -> Widget {
        Widget::Code {
            label: label.into(),
            value: value.into(),
            copy: true,
            live: false,
            grammar: String::new(),
        }
    }

    /// config shows a uci section as `/etc/config` spells it, read line by
    /// line.
    pub fn config(label: &str, value: &str) -> Widget {
        Widget::Code {
            label: label.into(),
            value: value.into(),
            copy: true,
            live: false,
            grammar: "uci".into(),
        }
    }

    /// config_preview is a code block that previews the uci sections the form
    /// around it writes, read line by line as [`Widget::config`] is: the same
    /// block, declared as the answer to "what will this write" so the shell
    /// keeps it current while the form is edited. Compose it inside the form it
    /// describes — one per form, since the form is what the answer is computed
    /// from.
    pub fn config_preview(label: &str, value: &str) -> Widget {
        Widget::Code {
            label: label.into(),
            value: value.into(),
            copy: true,
            live: true,
            grammar: "uci".into(),
        }
    }

    /// empty is a whole-page state that is not a listing (see [`Widget::Empty`]).
    pub fn empty(icon: &str, title: &str, body: &str, children: Vec<Widget>) -> Widget {
        Widget::Empty {
            icon: icon.into(),
            title: title.into(),
            body: body.into(),
            variant: String::new(),
            children,
        }
    }

    /// properties is a fact sheet of these rows, values on the right edge.
    pub fn properties(items: Vec<Property>) -> Widget {
        Widget::Properties {
            items,
            align: String::new(),
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

    /// button is a direct action. Style is the semantic emphasis — "primary",
    /// "secondary", "danger", or "act" for an act on a part of a section (the
    /// subsection act's dress, as a link's "act") — never a colour. A button built here carries no
    /// `name`, so it submits nothing: it is the control a shell behaviour
    /// drives (a live listing's pause), or a visual reference. Give it a name
    /// through the variant's own fields to make it submit.
    pub fn button(label: &str, style: &str) -> Widget {
        Widget::Button {
            label: label.into(),
            icon: String::new(),
            style: style.into(),
            name: String::new(),
            value: String::new(),
            disabled: false,
            loading: false,
            live: false,
        }
    }

    /// live marks a button as the state of something running: the spinner turns
    /// beside its label, and the button keeps every affordance because pressing
    /// it is how the running thing stops. Anything but a button comes back as
    /// it was.
    pub fn live(mut self) -> Widget {
        if let Widget::Button { live, .. } = &mut self {
            *live = true;
        }
        self
    }
}

/// SelectOption is one choice of a select or checks field.
#[derive(Serialize, Debug)]
pub struct SelectOption {
    pub value: String,
    pub label: String,
}

impl SelectOption {
    /// new is one choice; label it as a person would say it, value it as the
    /// config writes it.
    pub fn new(value: &str, label: &str) -> SelectOption {
        SelectOption {
            value: value.into(),
            label: label.into(),
        }
    }
}

/// ConditionItem is one uniquely keyed optional condition of a
/// [`Widget::Conditions`]. Key identifies it to the shell's picker; Active says
/// the object carries it now, so it renders in the form rather than waiting in
/// the picker; Children are ordinary widgets, so nothing about validation or
/// posting is special.
#[derive(Serialize, Debug)]
pub struct ConditionItem {
    pub key: String,
    pub label: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    pub help: String,
    /// The heading this entry sits under in the picker. A catalogue long enough to
    /// need searching is long enough to need sorting into kinds; entries naming no
    /// group are listed first, ungrouped. Groups appear in the order the catalogue
    /// first names them.
    #[serde(skip_serializing_if = "String::is_empty")]
    pub group: String,
    /// What sort of value this condition takes — "one host or a subnet", "names or
    /// numbers" — at the picker row's trailing edge. An example, not an
    /// explanation: `help` says what the condition is for and belongs to the row
    /// once it has been added, while this says what you would type into it.
    #[serde(skip_serializing_if = "String::is_empty")]
    pub hint: String,
    #[serde(skip_serializing_if = "is_false")]
    pub active: bool,
    pub children: Vec<Widget>,
}

/// Property is one row of a [`Widget::Properties`] fact sheet. `mono` sets a
/// machine value in monospace type; `verbatim` declares a value that is data
/// without the mono treatment — a size, a rate, an identity set in sans — so
/// the shell's localization leaves it exactly as authored. Declare one of them
/// on every value that is data, not words.
#[derive(Serialize, Debug, Default)]
pub struct Property {
    pub label: String,
    pub value: String,
    #[serde(skip_serializing_if = "is_false")]
    pub mono: bool,
    #[serde(skip_serializing_if = "is_false")]
    pub verbatim: bool,
    #[serde(skip_serializing_if = "is_false")]
    pub copy: bool,
    /// A note under the value, in the value's column: what the fact means for
    /// the reader. Set it with [`Property::noted`].
    #[serde(skip_serializing_if = "String::is_empty")]
    pub help: String,
    /// The state mark leading the value, a tone word. Set it with
    /// [`Property::marked`].
    #[serde(skip_serializing_if = "String::is_empty")]
    pub dot: String,
    /// Makes the value a stretch between two ends. Set it with
    /// [`Property::spanning`].
    #[serde(skip_serializing_if = "Option::is_none")]
    pub span: Option<PropertySpan>,
}

/// CollectionItem is one kept thing in a [`Widget::Collection`]: its identity
/// (a machine string, never translated), a line of detail, and its removal.
#[derive(Serialize, Debug, Default)]
pub struct CollectionItem {
    pub title: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    pub detail: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub remove: Option<CollectionRemove>,
}

/// CollectionRemove is an item's removal: the pair it posts, asked first.
#[derive(Serialize, Debug, Default)]
pub struct CollectionRemove {
    pub name: String,
    pub value: String,
    pub confirm: RemoveConfirm,
}

/// RemoveConfirm is the question a removal asks in place: the trigger is the
/// act's short word (drawn as `icon`, the item's first-line act), the title
/// the question, the message what removing costs.
#[derive(Serialize, Debug, Default)]
pub struct RemoveConfirm {
    pub trigger: String,
    pub icon: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    pub title: String,
    pub message: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    pub confirm: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    pub cancel: String,
}

/// CollectionAdd is the slot a [`Widget::Collection`]'s next item is added
/// from: the act's name at rest; open, the box `name` posts, a live
/// [`Widget::Code`] of what was typed, and `submit`. A refused paste is
/// answered with `value` kept and `error` set; the slot comes back open.
#[derive(Serialize, Debug, Default)]
pub struct CollectionAdd {
    pub label: String,
    pub name: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    pub value: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    pub placeholder: String,
    pub submit: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    pub error: String,
    #[serde(skip_serializing_if = "is_false")]
    pub open: bool,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub preview: Option<Box<Widget>>,
}

/// PropertySpan is a stretch between two ends — a certificate's validity, a
/// lease's term — drawn as the meter's track, filled `at` a share of it (0–100).
/// The ends are data and are never translated.
#[derive(Serialize, Debug)]
pub struct PropertySpan {
    pub from: String,
    pub to: String,
    pub at: u8,
    #[serde(skip_serializing_if = "String::is_empty")]
    pub tone: String,
}

impl Property {
    /// noted sets the sentence under the value that says what it means.
    pub fn noted(mut self, help: &str) -> Property {
        self.help = help.into();
        self
    }

    /// marked leads the value with the state mark in this tone.
    pub fn marked(mut self, tone: Tone) -> Property {
        self.dot = tone.as_str().into();
        self
    }

    /// spanning makes the value the stretch from `from` to `to`, drawn as a
    /// track filled `at` a share of it (held within 0–100), in `tone`.
    pub fn spanning(mut self, from: &str, to: &str, at: u8, tone: Tone) -> Property {
        self.span = Some(PropertySpan {
            from: from.into(),
            to: to.into(),
            at: at.min(100),
            tone: tone.as_str().into(),
        });
        self
    }
}

/// TableColumn is one column of a [`Widget::Table`]: its header label and the
/// kind every cell in it renders as. An empty kind reads as "text"; a table
/// whose columns carry no label at all draws no header row.
#[derive(Serialize, Debug, Default)]
pub struct TableColumn {
    #[serde(skip_serializing_if = "String::is_empty")]
    pub label: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    pub kind: String,
    /// Fix the column at a measure the listing decides rather than at whatever
    /// this page's data happens to need, so a column keeps its place when one
    /// row's value is shorter and two boards' listings line up. The columns
    /// left at [`ColumnWidth::Grow`] share what is left.
    #[serde(skip_serializing_if = "ColumnWidth::is_grow")]
    pub width: ColumnWidth,
}

/// ColumnWidth is a table column's fixed width, named by what the column holds
/// — the shell owns the length, so every listing's address column is the same
/// width. The set is closed: the shell refuses any other word.
#[derive(Serialize, Clone, Copy, PartialEq, Eq, Debug, Default)]
#[serde(rename_all = "lowercase")]
pub enum ColumnWidth {
    /// No fixed width: the column shares the table's slack.
    #[default]
    Grow,
    /// An order number, a grip, an icon.
    Mark,
    /// A counter or a flag: hits, packets, yes/no.
    Count,
    /// A short token: port, protocol, PID, size, a verdict, row acts.
    Short,
    /// A state or a chip: status, zone, version.
    Word,
    /// An address: IPv4 with its prefix, a MAC, address:port.
    Address,
    /// A name, or a short list of them: rule, zone, networks.
    Name,
    /// A long identity: a service, a package.
    Long,
}

impl ColumnWidth {
    fn is_grow(&self) -> bool {
        *self == ColumnWidth::Grow
    }
}

/// TableStream turns a listing live: its rows arrive after the render, newest
/// on top, over a connection the shell holds open. `source` is a name from the
/// shell's closed set — never a URL, so a plugin cannot point the shell at an
/// endpoint of its choosing — and a source the shell does not serve simply
/// leaves the listing still. `ring` is how many rows the browser keeps before
/// the oldest fall off the bottom; zero takes the shell's default (200).
///
/// Everything else is the shell's: the transport, the pause, the collapse of
/// consecutive repeats, and the narrowing that clicking a value in a row does.
/// A live listing should also declare a [`Widget::Filter`], which composes with
/// it as the free-text half of the same question.
#[derive(Serialize, Debug, Default)]
pub struct TableStream {
    pub source: String,
    #[serde(skip_serializing_if = "is_zero")]
    pub ring: u32,
}

/// STREAM_FIREWALL_LOG is the firewall's verdict stream: one row per logged
/// firewall decision, resolved by the shell to its zone and its rule.
pub const STREAM_FIREWALL_LOG: &str = "firewall-log";

/// TableRow is one config section. Id is its stable handle (the UCI section
/// name); it does not render. A Group on a row opens a run of rows sharing one
/// evaluation lane, headed above it. A Drawer makes the row an object that
/// opens.
#[derive(Serialize, Debug, Default)]
pub struct TableRow {
    #[serde(skip_serializing_if = "Vec::is_empty")]
    pub expanded: Vec<Widget>,
    #[serde(skip_serializing_if = "is_zero")]
    pub depth: u32,
    #[serde(skip_serializing_if = "String::is_empty")]
    pub id: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    pub key: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub group: Option<TableGroup>,
    /// The row's subject is switched off (a rule, a Wi-Fi network, a route):
    /// the shell marks its name over with a sand marker, reads its words at the
    /// secondary step and drops its hues to sand, and says "off" to a screen
    /// reader. It is a statement about the subject, not about the row's
    /// importance: the values are still exact, and still copyable.
    #[serde(skip_serializing_if = "is_false")]
    pub muted: bool,
    /// The flags an [`Widget::ActionBar`] tab narrows this listing by — the row
    /// survives a cut whose `match` it carries. A listing with no bar above it
    /// sets none.
    #[serde(skip_serializing_if = "Vec::is_empty")]
    pub tags: Vec<String>,
    pub cells: Vec<TableCell>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub drawer: Option<RowDrawer>,
    /// Where this row's panel is fetched from. The row ships an empty frame and
    /// asks for its contents the first time someone opens it, so a listing of
    /// forty rows carries forty frames rather than forty panels for the at-most
    /// one anybody will look at — and the frame, mounted once, slides in once.
    /// A cell whose link points at this same address opens the panel in place
    /// instead of following the link.
    #[serde(skip_serializing_if = "String::is_empty")]
    pub panel: String,
}

/// RowDrawer is a row's edit surface: a slide-in panel carrying the row's own
/// form. Removal belongs to a separate confirmed action on its listing. A small
/// object is edited here — one record, one host; an object with a page's worth of
/// settings gets a page instead. Every panel stands at the one drawer width;
/// size "choices" lays the body out as a chooser, and Open renders the
/// panel already open — which is how a submission the plugin refused comes
/// back with the failed drawer in front of the operator rather than silently
/// closed.
#[derive(Serialize, Debug, Default)]
pub struct RowDrawer {
    pub title: String,
    /// The address this page has when no panel is open. A panel the address
    /// opened is a place, so closing it has to leave that place — the shell
    /// rewrites the address to this one, and a reload then shows the listing
    /// rather than reopening what was just dismissed. Empty leaves the address
    /// alone, which is right for a panel no address opened.
    #[serde(skip_serializing_if = "String::is_empty")]
    pub closed: String,
    /// The questions this object answers, each carrying where the object stands
    /// under it. A panel with one thing to say sends none: a strip of headings
    /// that decides nothing is chrome.
    #[serde(skip_serializing_if = "Vec::is_empty")]
    pub tabs: Vec<DrawerTab>,
    #[serde(skip_serializing_if = "String::is_empty")]
    pub size: String,
    #[serde(skip_serializing_if = "is_false")]
    pub open: bool,
    pub children: Vec<Widget>,
}

/// DrawerTab is one heading in a panel's strip: what it is called and the
/// address that opens it. Its name is the whole tab.
#[derive(Serialize, Debug, Default)]
pub struct DrawerTab {
    pub label: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    pub href: String,
    #[serde(skip_serializing_if = "is_false")]
    pub active: bool,
}

/// TableGroup heads a run of rows that share one evaluation lane. Label is
/// human-facing; Chain keeps the packet filter's exact name visible to experts;
/// Count is how many rows the run holds.
#[derive(Serialize, Debug)]
pub struct TableGroup {
    /// The group's own handle — the config's name for it — which a dragged
    /// listing posts its order by. Not drawn.
    #[serde(skip_serializing_if = "String::is_empty")]
    pub key: String,
    /// What the group is called; for a traffic lane, where the traffic comes
    /// from.
    pub label: String,
    /// Where a lane's traffic goes, drawn after an arrow: "WAN → Router". A
    /// group that is not a path sets none.
    #[serde(skip_serializing_if = "String::is_empty")]
    pub to: String,
    /// The machine-verbatim string the group resolves to, set in mono beside
    /// the label — a network's CIDR. A group whose handle is not for reading
    /// sets none.
    #[serde(skip_serializing_if = "String::is_empty")]
    pub chain: String,
    /// What the group amounts to, already worded: "6 rules", "3 rules · 1
    /// disabled".
    #[serde(skip_serializing_if = "String::is_empty")]
    pub tally: String,
    /// The lane's own add control, which the shell puts on this row hard right
    /// with these words on hover — the head of the run it adds to; the href may
    /// pre-seed the editor with the lane's own path. Empty leaves the row's
    /// right side bare.
    #[serde(skip_serializing_if = "String::is_empty")]
    pub add_label: String,
    /// The words the add control says beside its glyph — "Add rule" — while
    /// add_label keeps naming the lane it adds to. Set it on every add: a plus
    /// on a table head always says what it adds (DESIGN.md, Tables).
    #[serde(skip_serializing_if = "String::is_empty")]
    pub add_text: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    pub add_href: String,
    /// Declares that address a panel rather than a page: the lane's add opens it
    /// where the lane is, seeded with the lane's own path, and the listing stays
    /// put behind it.
    #[serde(skip_serializing_if = "is_false")]
    pub add_panel: bool,
}

/// ActionTab is one coarse cut of a listing, and what taking it would leave.
/// `matches` is the tag a row must carry to survive the cut; empty is the tab
/// that cuts nothing.
#[derive(Serialize, Debug, Default)]
pub struct ActionTab {
    pub label: String,
    /// What taking this cut would leave. A live listing states none: what is on
    /// screen is whatever has arrived this second, so the shell counts it.
    #[serde(skip_serializing_if = "is_zero")]
    pub count: u32,
    #[serde(rename = "match", skip_serializing_if = "String::is_empty")]
    pub matches: String,
    #[serde(skip_serializing_if = "is_false")]
    pub active: bool,
}

/// TableAction is a bar's or a header band's one forward act — a label and
/// where it leads.
#[derive(Serialize, Debug, Default)]
pub struct TableAction {
    pub label: String,
    pub href: String,
    /// The glyph the act wears, by Lucide name. An act that makes a new object
    /// leaves it empty and takes the plus every "add" wears, so the affordance a
    /// listing most often carries reads identically everywhere; an act that does
    /// something else names its own.
    #[serde(skip_serializing_if = "String::is_empty")]
    pub icon: String,
    /// The act's weight: empty is the bar's one forward act, filled with the
    /// action colour; "quiet" draws it as the secondary button beside the bar's
    /// other controls — for an act that takes something away from the page
    /// rather than leading somewhere.
    #[serde(skip_serializing_if = "String::is_empty")]
    pub style: String,
}

/// TableCell carries the value for one cell; which field applies is decided by
/// the column's kind — Text for the text kinds, Text plus Variant for a pill,
/// On and Name for a toggle, Endpoints for an endpoint cell, Button for an
/// in-row action.
#[derive(Serialize, Debug, Default)]
pub struct TableCell {
    #[serde(skip_serializing_if = "Vec::is_empty")]
    pub chips: Vec<TableChip>,
    #[serde(skip_serializing_if = "String::is_empty")]
    pub text: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    pub variant: String,
    /// A meter cell's share, 0–100; `text` is the figure beside the bar and
    /// `variant` its band.
    #[serde(skip_serializing_if = "is_zero")]
    pub fill: u32,
    #[serde(skip_serializing_if = "is_false")]
    pub dot: bool,
    #[serde(skip_serializing_if = "String::is_empty")]
    pub icon: String,
    #[serde(skip_serializing_if = "is_false")]
    pub copy: bool,
    #[serde(skip_serializing_if = "is_false")]
    pub emphasis: bool,
    #[serde(skip_serializing_if = "String::is_empty")]
    pub chip: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    pub chip_icon: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    pub lead_icon: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    pub key: String,
    #[serde(skip_serializing_if = "is_false")]
    pub muted: bool,
    #[serde(skip_serializing_if = "String::is_empty")]
    pub sub: String,
    /// A line of words under the value saying what it is or since when (the
    /// browser behind an address); the cell's tag rides it. Name, reference
    /// and text cells.
    #[serde(skip_serializing_if = "String::is_empty")]
    pub detail: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    pub tag: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    pub tag_variant: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    pub tag_icon: String,
    /// The packet square leading the tag, in its variant. A chip carries one
    /// mark or none; an icon wins.
    #[serde(skip_serializing_if = "is_false")]
    pub tag_dot: bool,
    #[serde(skip_serializing_if = "String::is_empty")]
    pub href: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    pub button: String,
    #[serde(skip_serializing_if = "is_false")]
    pub disabled: bool,
    #[serde(skip_serializing_if = "is_false")]
    pub on: bool,
    #[serde(skip_serializing_if = "String::is_empty")]
    pub name: String,
    #[serde(skip_serializing_if = "Vec::is_empty")]
    pub endpoints: Vec<TableEndpoint>,
    /// The row's own acts, drawn as quiet icon buttons at its trailing edge.
    #[serde(skip_serializing_if = "Vec::is_empty")]
    pub actions: Vec<TableRowAct>,
}

/// An entity reference beside a row's identity. Its label is always verbatim.
#[derive(Serialize, Debug, Default)]
pub struct TableChip {
    #[serde(skip_serializing_if = "String::is_empty")]
    pub title: String,
    pub icon: String,
    pub label: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    pub tone: String,
    /// The packet square leading the chip, in its tone, where it has no icon.
    /// A chip carries one mark or none; an icon wins.
    #[serde(skip_serializing_if = "is_false")]
    pub dot: bool,
}

/// TableRowAct is one act on a row: a Lucide glyph, the sentence a pointer rests
/// on, and where it leads. An act with no href is drawn and does nothing — the
/// shape is decided, what sits behind it is not, and a control that is present
/// and inert is honest where an absent one would hide that the act exists.
#[derive(Serialize, Debug, Default)]
pub struct TableRowAct {
    pub icon: String,
    /// Short action label, without the row's name; localized by the shell.
    pub title: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    pub href: String,
    /// Name and value make the act a submission rather than a destination: it
    /// posts that one pair to the page it sits on, which is how a row's own
    /// state is flipped from the row itself. An act leads somewhere or posts
    /// something, never both.
    #[serde(skip_serializing_if = "String::is_empty")]
    pub name: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    pub value: String,
    /// A posting action can ask for confirmation independently of the other
    /// actions in its cell. In the title, `%s` names the row after localization.
    #[serde(skip_serializing_if = "String::is_empty")]
    pub confirm_title: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    pub confirm: String,
}

impl TableCell {
    /// zone is a traffic endpoint naming a firewall zone.
    pub fn zone(name: &str) -> TableCell {
        TableCell::endpoint("zone", name)
    }

    /// device is a traffic endpoint naming one address on the network.
    pub fn device(address: &str) -> TableCell {
        TableCell::endpoint("device", address)
    }

    /// router is a traffic endpoint naming this device itself.
    pub fn router() -> TableCell {
        TableCell::endpoint("router", "router")
    }

    /// any is the endpoint that matches wherever traffic comes from or goes.
    pub fn any() -> TableCell {
        TableCell::endpoint("any", "any")
    }

    fn endpoint(kind: &str, label: &str) -> TableCell {
        TableCell {
            endpoints: vec![TableEndpoint {
                kind: kind.into(),
                label: label.into(),
            }],
            ..TableCell::default()
        }
    }
}

/// TableEndpoint is one traffic endpoint in an endpoint cell. The kind picks the
/// type icon and treatment: "zone" | "device" | "router" | "any".
#[derive(Serialize, Debug)]
pub struct TableEndpoint {
    pub kind: String,
    pub label: String,
}

/// SettingsItem is one option row of a [`Widget::Settings`]. Exactly one of
/// Toggle, Pills, or Value carries the trailing state; a row with none is
/// informational. A Value with a Name is edited in place.
#[derive(Serialize, Debug, Default)]
pub struct SettingsItem {
    pub title: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    pub desc: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    pub code: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    pub value: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    pub name: String,
    /// Inline makes a Value+Name row a manage-page control that stages its own
    /// change (inline-settings-commit-model): it reads as text with a square-pen,
    /// opens on edit, and commits just this one option on ✓/Enter/blur — the
    /// counterpart of a row switch for a value. A plain input that stages on blur
    /// or Enter — the "done" a person expects, no edit/confirm icons. Place it
    /// outside a page form; a form-driven value clears this.
    #[serde(skip_serializing_if = "is_false")]
    pub inline: bool,
    /// Datatype names the shape an inline value must have ("hostname", …) so the
    /// shell validates it on blur before staging — an invalid value shows its
    /// error beneath the field and does not stage. Only meaningful with `inline`.
    #[serde(skip_serializing_if = "String::is_empty")]
    pub datatype: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub toggle: Option<SettingsToggle>,
    #[serde(skip_serializing_if = "Vec::is_empty")]
    pub pills: Vec<SettingsPill>,
}

/// SettingsSeam folds a block's rarely-touched options behind a collapsed line
/// inside the same block: the everyday rows stay visible, the long tail stays
/// present and honest without carrying the block. Summary is the line that
/// opens it.
#[derive(Serialize, Debug)]
pub struct SettingsSeam {
    pub summary: String,
    pub items: Vec<SettingsItem>,
}

/// SettingsToggle is an option row's switch: its state and the form name it
/// posts under.
#[derive(Serialize, Debug)]
pub struct SettingsToggle {
    #[serde(skip_serializing_if = "String::is_empty")]
    pub name: String,
    #[serde(skip_serializing_if = "is_false")]
    pub on: bool,
}

/// SettingsPill is one value pill on an option row that reads rather than
/// toggles — the tone vocabulary, never a colour.
#[derive(Serialize, Debug)]
pub struct SettingsPill {
    pub variant: Tone,
    pub text: String,
}

/// Notice is the outcome of the action this render answers — shown by the shell
/// in its own flash slot (ADR-006 §4). State the outcome; never compose it.
#[derive(Serialize, Debug)]
pub struct Notice {
    pub level: Tone,
    pub text: String,
}

/// CommitOp is one declarative uci write the shell applies on the plugin's
/// behalf (ADR-007). It takes one of three shapes: a section and values sets
/// those options; a section_type creates a new section (optionally named by section) of
/// that type and sets the values on it; a section with `delete` removes it.
/// Within values, a JSON `null` clears that one option — what an editor writes
/// when a setting it owns is no longer there, since an empty string is a value.
#[derive(Serialize, Debug)]
pub struct CommitOp {
    pub config: String,
    pub section: String,
    #[serde(rename = "type", skip_serializing_if = "String::is_empty")]
    pub section_type: String,
    #[serde(skip_serializing_if = "is_false")]
    pub delete: bool,
    #[serde(skip_serializing_if = "Value::is_null")]
    pub values: Value,
}

/// uci_text spells one section as `uci export` prints it, for a
/// [`Widget::config_preview`]: the `config` line, then an `option` line per
/// value and a `list` line per list item, each quoted the way uci quotes (a `'`
/// becomes `'\''`). A null is an option being removed and prints nothing; uci
/// holds strings, so a number or flag prints as its characters. An empty name is
/// a section not created yet.
pub fn uci_text(typ: &str, name: &str, values: &Map<String, Value>) -> String {
    let quote = |text: &str| format!("'{}'", text.replace('\'', r"'\''"));
    let scalar = |value: &Value| match value {
        Value::String(text) => quote(text),
        other => quote(&other.to_string()),
    };
    let mut lines = vec![if name.is_empty() {
        format!("config {typ}")
    } else {
        format!("config {typ} {}", quote(name))
    }];
    for (option, value) in values {
        match value {
            Value::Null => {}
            Value::Array(items) => lines.extend(
                items
                    .iter()
                    .map(|item| format!("\tlist {option} {}", scalar(item))),
            ),
            other => lines.push(format!("\toption {option} {}", scalar(other))),
        }
    }
    lines.join("\n")
}

/// commit is one declarative uci write for [`Envelope::with_commit`].
pub fn commit(config: &str, section: &str, values: Value) -> CommitOp {
    CommitOp {
        config: config.into(),
        section: section.into(),
        section_type: String::new(),
        delete: false,
        values,
    }
}

/// commit_new creates a new section of `section_type` and sets `values` on it.
pub fn commit_new(config: &str, section_type: &str, values: Value) -> CommitOp {
    CommitOp {
        config: config.into(),
        section: String::new(),
        section_type: section_type.into(),
        delete: false,
        values,
    }
}

/// commit_delete removes a section outright — the whole object, not one of its
/// options. Clearing a single option is a `null` among a [`commit`]'s values.
pub fn commit_delete(config: &str, section: &str) -> CommitOp {
    CommitOp {
        config: config.into(),
        section: section.into(),
        section_type: String::new(),
        delete: true,
        values: Value::Null,
    }
}

/// ApplyAction is one tightly typed non-UCI operation the shell performs after
/// the staged UCI transaction applies — only for a declared rpcd scope.
#[derive(Serialize, Debug)]
pub struct ApplyAction {
    pub name: String,
    pub args: std::collections::BTreeMap<String, String>,
}

/// PageTab is one subpage in the plugin's top bar — the third navigation tier
/// (sidebar → domain, top bar → kind of visit). Path is relative to the plugin's
/// mount, so the bar can only ever point inside the plugin; the shell builds the
/// href and marks the active tab from the request path.
#[derive(Serialize, Debug)]
pub struct PageTab {
    pub label: String,
    pub path: String,
}

/// PageAction is a link the masthead carries — the envelope's `back`. Href is a
/// route through the shell, exactly like a link widget's, so it addresses the
/// plugin's mount in full.
#[derive(Serialize, Debug)]
pub struct PageAction {
    pub label: String,
    pub href: String,
}

/// A configured subject contributed to the shell roster by an entity tab.
#[derive(Serialize, Debug)]
pub struct EntitySummary {
    pub id: String,
    pub name: String,
    /// Configuration label, not a claim about live enforcement.
    pub state: String,
    pub details: Vec<EntitySummaryDetail>,
}

/// A tooltip fact. The shell translates the label and each value separately.
#[derive(Serialize, Debug)]
pub struct EntitySummaryDetail {
    pub kind: String,
    pub label: String,
    pub values: Vec<String>,
}

/// Envelope is the typed top-level reply (ADR-006 §4).
#[derive(Serialize, Debug)]
pub struct Envelope {
    /// Bulk configured subjects for the shell roster. None means unavailable;
    /// Some(empty) means the configuration was read and contains no subjects.
    #[serde(skip_serializing_if = "Option::is_none")]
    pub entities: Option<Vec<EntitySummary>>,
    pub schema_version: u32,
    pub title: String,
    #[serde(skip_serializing_if = "String::is_empty")]
    pub width: String,
    #[serde(skip_serializing_if = "Vec::is_empty")]
    pub pages: Vec<PageTab>,
    /// An edit page's quiet way home: the shell renders it as a "← Cancel"
    /// back-link in the masthead, above the heading, so a page reached to edit
    /// one record can return to the listing it came from. The shell fixes the
    /// arrow-left glyph and defaults the label to "Cancel", so a page sets only
    /// the href and, if it wants other words, the label.
    #[serde(skip_serializing_if = "Option::is_none")]
    pub back: Option<PageAction>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub notice: Option<Notice>,
    /// The commit row's verb when this envelope is one tab of a shell-owned
    /// entity panel ("Reserve address"). It belongs to the plugin because only
    /// the plugin knows what its tab saves. The act says what it does and
    /// carries no explanation beside it. A tab that stages nothing sets none and
    /// the shell draws no commit row. Ignored on an ordinary page render.
    #[serde(skip_serializing_if = "String::is_empty")]
    pub cta: String,
    /// A page made only of direct reads and commands, not staged configuration:
    /// nothing on it stages, though the staged-changes chip still shows what waits.
    /// Pending changes from elsewhere remain visible, and a `commit` still
    /// stages — immediacy is the page's nature, never a write's.
    #[serde(skip_serializing_if = "is_false")]
    pub immediate: bool,
    /// The title is a message about now, not a place-label: the shell tints
    /// the heading by the closed tone vocabulary ("info" | "success" |
    /// "warning" | "danger" | "neutral" — never a colour) and drops the
    /// " — <tab>" navigation suffix. "neutral" drops the suffix without a
    /// tint; the shell ignores anything outside the vocabulary.
    #[serde(skip_serializing_if = "String::is_empty")]
    pub tone: String,
    pub widget: Widget,
    /// The page's one act, on its heading line; see [`HeadingAct`].
    #[serde(skip_serializing_if = "Option::is_none")]
    pub act: Option<HeadingAct>,
    #[serde(skip_serializing_if = "Vec::is_empty")]
    pub commit: Vec<CommitOp>,
    /// Immediate, explicitly brokered commands; never arbitrary shell code.
    #[serde(skip_serializing_if = "Vec::is_empty")]
    pub commands: Vec<ApplyAction>,
}

impl Envelope {
    /// with_commit_row states the verb this envelope's commit row carries when
    /// the shell frames it as one tab of an entity panel. An envelope that sets
    /// none gets no commit row.
    pub fn with_commit_row(mut self, cta: &str) -> Envelope {
        self.cta = cta.into();
        self
    }

    /// page wraps a widget as the reply for one page render.
    pub fn page(title: &str, widget: Widget) -> Envelope {
        Envelope {
            entities: None,
            schema_version: 1,
            title: title.into(),
            width: String::new(),
            pages: Vec::new(),
            back: None,
            notice: None,
            cta: String::new(),
            immediate: false,
            tone: String::new(),
            widget,
            act: None,
            commit: Vec::new(),
            commands: Vec::new(),
        }
    }

    /// immediate declares the page one of direct reads and commands rather
    /// than staged configuration: nothing on it stages.
    pub fn immediate(mut self) -> Envelope {
        self.immediate = true;
        self
    }

    /// with_tone declares the title a message about now: the shell tints the
    /// heading by the closed tone vocabulary and drops the navigation suffix.
    pub fn with_tone(mut self, tone: &str) -> Envelope {
        self.tone = tone.into();
        self
    }

    /// with_width sets the content-column preset ("narrow" | "normal" | "wide").
    pub fn with_width(mut self, width: &str) -> Envelope {
        self.width = width.into();
        self
    }

    /// with_pages declares the plugin's subpages as the shell's top bar. Every
    /// page of a multi-page plugin declares the same list, so the bar stays put
    /// as the visitor moves between them.
    pub fn with_pages(mut self, pages: Vec<PageTab>) -> Envelope {
        self.pages = pages;
        self
    }

    /// with_back gives an editor its return address: where a save that staged,
    /// or a command that ran, sends the person, and where a page form's Cancel
    /// leads. The masthead draws nothing for it, and the shell reads only the
    /// href.
    pub fn with_back(mut self, label: &str, href: &str) -> Envelope {
        self.back = Some(PageAction {
            label: label.into(),
            href: href.into(),
        });
        self
    }

    /// with_act gives the page its one act, which the shell stands on the
    /// heading line beside the title.
    pub fn with_act(mut self, act: HeadingAct) -> Envelope {
        self.act = Some(act);
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
}

// ---- tiny internals (no dependencies) ----

/// parse_head splits the request head into the method, the raw request target
/// (path and query string together, as the shell sent it), and the headers.
fn parse_head(head: &str) -> (String, String, Vec<(String, String)>) {
    let mut lines = head.split("\r\n");
    let mut request_line = lines.next().unwrap_or("").split(' ');
    let method = request_line.next().unwrap_or("GET").to_string();
    let target = request_line.next().unwrap_or("").to_string();
    let headers = lines
        .filter_map(|line| line.split_once(':'))
        .map(|(k, v)| (k.trim().to_ascii_lowercase(), v.trim().to_string()))
        .collect();
    (method, target, headers)
}

/// header reads one parsed request header by its lowercased name, or "" when the
/// shell sent none — the shape both brokered reads decode from.
fn header<'a>(headers: &'a [(String, String)], name: &str) -> &'a str {
    headers
        .iter()
        .find(|(key, _)| key == name)
        .map(|(_, value)| value.as_str())
        .unwrap_or("")
}

/// request_path is the request target's path — what the plugin routes on; the
/// query string is never part of the route. A target that carries no path at all
/// is the mount root.
fn request_path(target: &str) -> String {
    let path = target.split('?').next().unwrap_or("");
    if path.is_empty() {
        return "/".to_string();
    }
    path.to_string()
}

/// request_query is the request target's query string, decoded like a form body:
/// it addresses something within the page the path named.
fn request_query(target: &str) -> Form {
    Form::parse(target.split_once('?').map(|(_, query)| query).unwrap_or(""))
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

#[cfg(test)]
mod tests {
    use super::{
        commit, commit_delete, commit_new, header, parse_head, request_path, request_query, Change,
        ConditionItem, DescribeRequestBody, DescribeResponseBody, Description, Envelope, Field,
        Form, List, PageTab, RowDrawer, SectionWidget, SelectOption, Settings, SettingsItem,
        SettingsPill, SettingsSeam, Snapshot, Switch, TableCell, TableRow, Tone, Ubus, Widget,
    };
    use super::{json, uci_text, Map, Value};

    // A form's refusal is only what no control of it says: a field's refusal
    // rides the field, however deep in the form it stands, and is never said
    // twice. One no control draws is the form's, so it is never lost.
    #[test]
    fn a_form_refuses_only_what_no_control_says() {
        let mut port = Widget::field("https_port", "HTTPS port", "99999", "", "");
        if let Widget::Field(Field { error, .. }) = &mut port {
            *error = "Enter a port from 1 to 65535.".into();
        }
        let mut redirect = Widget::switch_keyed("redirect", "Redirect", "redirect", "", true);
        if let Widget::Switch(Switch { error, .. }) = &mut redirect {
            *error = "Add an HTTPS listener first.".into();
        }
        let fields = vec![
            Widget::section("", "", vec![Widget::stack(vec![port])]),
            redirect,
        ];
        let only_fields: std::collections::BTreeMap<String, String> = [
            (
                "https_port".to_string(),
                "Enter a port from 1 to 65535.".to_string(),
            ),
            (
                "redirect".to_string(),
                "Add an HTTPS listener first.".to_string(),
            ),
        ]
        .into();
        assert_eq!(Widget::refusal(&fields, &only_fields), "");
        let mut with_whole = only_fields.clone();
        with_whole.insert("listeners".into(), "Keep at least one listener.".into());
        assert_eq!(
            Widget::refusal(&fields, &with_whole),
            "Keep at least one listener."
        );
        // A control that names the key but draws no refusal leaves it the form's.
        let bare = vec![Widget::field("https_port", "HTTPS port", "99999", "", "")];
        assert_eq!(
            Widget::refusal(&bare, &only_fields),
            "Enter a port from 1 to 65535."
        );
        // A list carries its refusal on a row, under the list's own name.
        let mut servers = Widget::list("server", "Time servers", "host", &["x!".into()], "");
        if let Widget::List(List { errors, .. }) = &mut servers {
            errors.insert("0".into(), "Enter valid hostnames.".into());
        }
        let listed: std::collections::BTreeMap<String, String> =
            [("server".to_string(), "Enter valid hostnames.".to_string())].into();
        assert_eq!(Widget::refusal(&[servers], &listed), "");
    }

    // A form group names itself for the control the shell fuses it into; the
    // words ride the grid, and nothing else takes them.
    #[test]
    fn a_form_group_is_labelled_on_the_wire() {
        let group = Widget::form_grid(2, vec![]).labelled("Connection rate", "How fast.");
        let wire = serde_json::to_value(&group).unwrap();
        assert_eq!(wire["label"], "Connection rate");
        assert_eq!(wire["help"], "How fast.");
        let bare = serde_json::to_value(Widget::form_grid(2, vec![])).unwrap();
        assert!(bare.get("label").is_none() && bare.get("help").is_none());
        let text = serde_json::to_value(Widget::text("a").labelled("x", "y")).unwrap();
        assert!(text.get("label").is_none());
    }

    // A locked field is a value fixed where it stands, drawn in the field's
    // own row with the reason it cannot change.
    #[test]
    fn a_fixed_value_is_locked_where_it_stands() {
        let name = serde_json::to_value(
            Widget::field("name", "Name", "lan", "", "").locked("Named once."),
        )
        .unwrap();
        assert_eq!(name["style"], "locked");
        assert_eq!(name["help"], "Named once.");
        assert_eq!(name["value"], "lan");
        let text = serde_json::to_value(Widget::text("a").locked("x")).unwrap();
        assert!(text.get("style").is_none());
    }

    // A reveal folds optional rows behind a quiet act, and arrives open when
    // it already holds something; a disclosure starts folded and says nothing
    // about being open.
    #[test]
    fn a_reveal_says_whether_it_arrives_open() {
        let open = serde_json::to_value(Widget::reveal("Exclude some", true, vec![])).unwrap();
        assert_eq!(open["style"], "reveal");
        assert_eq!(open["open"], true);
        let folded = serde_json::to_value(Widget::reveal("Exclude some", false, vec![])).unwrap();
        assert!(folded.get("open").is_none());
        let plain = serde_json::to_value(Widget::disclosure("More", vec![])).unwrap();
        assert!(plain.get("open").is_none() && plain.get("style").is_none());
    }

    // A form group whose parts read as one sentence names the word that joins
    // them; a group without one says nothing about joining.
    #[test]
    fn a_form_group_is_joined_on_the_wire() {
        let path = Widget::form_grid(2, vec![])
            .labelled("Path", "")
            .joined("to");
        assert_eq!(serde_json::to_value(&path).unwrap()["join"], "to");
        let bare = serde_json::to_value(Widget::form_grid(2, vec![])).unwrap();
        assert!(bare.get("join").is_none());
        let text = serde_json::to_value(Widget::text("a").joined("to")).unwrap();
        assert!(text.get("join").is_none());
    }

    // A describe request the shell POSTs decodes to the normalized change vocabulary,
    // and the descriptions we answer with serialize to the shape the shell decodes.
    #[test]
    fn describe_wire_round_trips() {
        let body = r#"{"changes":[
            {"config":"firewall","op":"add-section","section":"cfg99","option":"rule"},
            {"config":"firewall","op":"set","section":"cfg99","option":"name","value":"Allow DNS"}
        ]}"#;
        let req: DescribeRequestBody = serde_json::from_str(body).unwrap();
        assert_eq!(req.changes.len(), 2);
        assert_eq!(req.changes[0].op, "add-section");
        assert_eq!(req.changes[0].option, "rule");
        assert_eq!(req.changes[1].value, "Allow DNS");

        let resp = DescribeResponseBody {
            descriptions: vec![Description::new("Added the rule.", vec![0, 1])],
        };
        assert_eq!(
            serde_json::to_value(&resp).unwrap(),
            serde_json::json!({"descriptions":[{"plain":"Added the rule.","covers":[0,1]}]})
        );
    }

    // A field an op does not use is absent on the wire but reads as empty.
    #[test]
    fn a_change_omitting_optional_fields_decodes_to_empties() {
        let c: Change =
            serde_json::from_str(r#"{"config":"firewall","op":"remove-section","section":"cfg1"}"#)
                .unwrap();
        assert_eq!(c.option, "");
        assert_eq!(c.value, "");
    }

    #[test]
    fn a_snapshot_orders_a_type_and_tells_named_from_anonymous() {
        let snapshot = Snapshot::from_value(serde_json::json!({
            "firewall": {
                "cfg02": {".type": "rule", ".name": "cfg02", ".index": 2, "name": "Allow-Ping"},
                "wanping": {".type": "rule", ".name": "wanping", ".index": 1},
                "cfg00": {".type": "zone", ".name": "cfg00", ".index": 0, ".anonymous": true},
            }
        }));

        let rules = snapshot.sections_of_type("firewall", "rule");
        let names: Vec<String> = rules.iter().map(|rule| rule.name()).collect();
        assert_eq!(names, vec!["wanping", "cfg02"]);
        assert!(!rules[0].anonymous(), "a named section reads as named");

        let zone = snapshot.section("firewall", "cfg00").expect("zone");
        assert!(zone.anonymous());
    }

    #[test]
    fn a_sections_entries_are_its_options_and_not_ucis_meta() {
        let snapshot = Snapshot::from_value(serde_json::json!({
            "dhcp": {
                "lan": {
                    ".type": "dhcp", ".name": "lan", ".index": 1, ".anonymous": false,
                    "interface": "lan", "leasetime": "12h", "ra_flags": ["managed-config"]
                }
            }
        }));
        let section = snapshot.section("dhcp", "lan").expect("section");
        let options: Vec<&str> = section
            .entries()
            .map(|(option, _)| option.as_str())
            .collect();
        assert_eq!(options, vec!["interface", "leasetime", "ra_flags"]);
        let values: Vec<&super::Value> = section.entries().map(|(_, value)| value).collect();
        assert_eq!(values[1], &serde_json::json!("12h"));
        assert_eq!(values[2], &serde_json::json!(["managed-config"]));
    }

    #[test]
    fn request_line_carries_method_and_sub_path() {
        let (method, target, _) = parse_head("GET /zones HTTP/1.1\r\nHost: plugin\r\n\r\n");
        assert_eq!(method, "GET");
        assert_eq!(request_path(&target), "/zones");

        let (method, target, _) = parse_head("POST /port-forwards HTTP/1.1\r\n\r\n");
        assert_eq!(method, "POST");
        assert_eq!(request_path(&target), "/port-forwards");
    }

    #[test]
    fn the_route_is_the_path_and_the_query_is_read_beside_it() {
        let (_, target, _) = parse_head("GET /zones?edit=lan&x=1&x=2 HTTP/1.1\r\n\r\n");
        assert_eq!(request_path(&target), "/zones");
        let query = request_query(&target);
        assert_eq!(query.get("edit"), "lan");
        assert_eq!(query.all("x"), vec!["1", "2"]);

        // A percent-encoded value arrives decoded, the way a form field does.
        let (_, target, _) =
            parse_head("GET /?reserve=42%3Ae6%3Aad%3Aff%3Ab7%3Aaf HTTP/1.1\r\n\r\n");
        assert_eq!(request_path(&target), "/");
        assert_eq!(request_query(&target).get("reserve"), "42:e6:ad:ff:b7:af");
    }

    #[test]
    fn a_request_without_a_query_reads_as_no_values() {
        let (_, target, _) = parse_head("GET /config HTTP/1.1\r\n\r\n");
        assert_eq!(request_query(&target).get("reserve"), "");
    }

    #[test]
    fn a_pathless_request_line_reads_as_the_mount_root() {
        for line in ["GET  HTTP/1.1\r\n\r\n", "GET\r\n\r\n", "\r\n\r\n"] {
            let (_, target, _) = parse_head(line);
            assert_eq!(request_path(&target), "/", "{line:?}");
        }
    }

    #[test]
    fn headers_are_lowercased_and_trimmed() {
        let (_, _, headers) = parse_head("POST / HTTP/1.1\r\nX-Verso-UCI:  e30=  \r\n\r\n");
        assert_eq!(
            headers,
            vec![("x-verso-uci".to_string(), "e30=".to_string())]
        );
    }

    // The shell's X-Verso-Ubus header for
    // {"firewallCounters":{"counters":[{"chain":"input_wan","name":"Allow-Ping",
    //   "packets":12,"bytes":1008}]}}
    const COUNTERS_HEADER: &str = "eyJmaXJld2FsbENvdW50ZXJzIjp7ImNvdW50ZXJzIjpbeyJjaGFpbiI6ImlucHV0X3dhbiIsIm5hbWUiOiJBbGxvdy1QaW5nIiwicGFja2V0cyI6MTIsImJ5dGVzIjoxMDA4fV19fQ==";

    #[test]
    fn brokered_reads_arrive_under_their_function_name() {
        let head =
            format!("POST /port-forwards HTTP/1.1\r\nX-Verso-Ubus: {COUNTERS_HEADER}\r\n\r\n");
        let (_, _, headers) = parse_head(&head);
        let ubus = Ubus::from_b64(header(&headers, "x-verso-ubus"));

        let counters = ubus.get("firewallCounters").expect("brokered read");
        assert_eq!(counters["counters"][0]["name"], "Allow-Ping");
        assert_eq!(counters["counters"][0]["packets"], 12);
        assert!(ubus.get("somethingElse").is_none());
    }

    #[test]
    fn an_unbrokered_read_is_absent_rather_than_an_error() {
        let (_, _, headers) = parse_head("GET / HTTP/1.1\r\nX-Verso-UCI: e30=\r\n\r\n");
        assert!(Ubus::from_b64(header(&headers, "x-verso-ubus"))
            .get("firewallCounters")
            .is_none());
        // A header the shell could not have sent decodes to nothing, never a panic.
        assert!(Ubus::from_b64("not base64 at all!")
            .get("firewallCounters")
            .is_none());
        assert!(Ubus::from_b64("bm90IGpzb24=")
            .get("firewallCounters")
            .is_none());
    }

    #[test]
    fn form_reads_first_and_all_values() {
        let form = Form::parse("server=a&server=b&name=my+router&empty=");
        assert_eq!(form.get("server"), "a");
        assert_eq!(form.all("server"), vec!["a", "b"]);
        assert_eq!(form.get("name"), "my router");
        assert_eq!(form.get("empty"), "");
        assert_eq!(form.get("absent"), "");
    }

    #[test]
    fn pages_serialize_only_once_declared() {
        let bare = Envelope::page("Firewall", Widget::text("body"));
        let json = serde_json::to_value(&bare).unwrap();
        assert!(json.get("pages").is_none());

        let tabbed = Envelope::page("Firewall", Widget::text("body")).with_pages(vec![
            PageTab {
                label: "Rules".into(),
                path: "".into(),
            },
            PageTab {
                label: "Zones".into(),
                path: "zones".into(),
            },
        ]);
        let json = serde_json::to_value(&tabbed).unwrap();
        assert_eq!(
            json["pages"],
            serde_json::json!([
                {"label": "Rules", "path": ""},
                {"label": "Zones", "path": "zones"}
            ])
        );
    }

    /// The masthead back-link rides the envelope rather than the widget tree, so
    /// it has no conformance fixture; this pins its wire shape. The plugin never sends a
    /// glyph — the shell fixes arrow-left — so `back` carries only label and href.
    #[test]
    fn the_back_link_serializes_only_once_declared() {
        let bare = Envelope::page("New reservation", Widget::text("body"));
        let json = serde_json::to_value(&bare).unwrap();
        assert!(json.get("back").is_none());

        let with_back = Envelope::page("New reservation", Widget::text("body"))
            .with_back("Cancel", "/plugins/dnsdhcp/hosts");
        let json = serde_json::to_value(&with_back).unwrap();
        assert_eq!(
            json["back"],
            serde_json::json!({"label": "Cancel", "href": "/plugins/dnsdhcp/hosts"})
        );
        assert!(
            json["back"].get("icon").is_none(),
            "the back-link never carries a glyph; the shell fixes arrow-left"
        );
    }

    #[test]
    fn endpoint_cells_name_their_kind() {
        let cases = [
            (TableCell::zone("guest"), "zone", "guest"),
            (TableCell::device("10.0.0.30"), "device", "10.0.0.30"),
            (TableCell::router(), "router", "router"),
            (TableCell::any(), "any", "any"),
        ];
        for (cell, kind, label) in cases {
            let json = serde_json::to_value(&cell).unwrap();
            assert_eq!(
                json["endpoints"],
                serde_json::json!([{"kind": kind, "label": label}])
            );
        }
    }

    #[test]
    fn a_disabled_row_button_serializes_both_fields() {
        let cell = TableCell {
            button: "Edit".into(),
            disabled: true,
            ..TableCell::default()
        };
        let json = serde_json::to_value(&cell).unwrap();
        assert_eq!(
            json,
            serde_json::json!({"button": "Edit", "disabled": true})
        );
    }

    #[test]
    fn a_switch_states_only_what_it_carries() {
        let plain = Widget::switch("enabled", "Enabled", true);
        assert_eq!(
            serde_json::to_value(&plain).unwrap(),
            serde_json::json!({"type": "switch", "name": "enabled", "label": "Enabled", "on": true})
        );
        let off = Widget::Switch(Switch {
            name: "counter".into(),
            label: "Count matching packets".into(),
            off_label: "Not counted".into(),
            help: "Hit counts come from the kernel.".into(),
            style: "inline".into(),
            ..Default::default()
        });
        assert_eq!(
            serde_json::to_value(&off).unwrap(),
            serde_json::json!({
                "type": "switch", "name": "counter", "label": "Count matching packets",
                "off_label": "Not counted", "help": "Hit counts come from the kernel.",
                "style": "inline"
            })
        );
    }

    #[test]
    fn a_condition_declares_its_key_and_whether_it_is_carried() {
        let widget = Widget::Conditions {
            label: "Conditions".into(),
            help: "Combined with and.".into(),
            items: vec![
                ConditionItem {
                    key: "dest_port".into(),
                    label: "Destination ports".into(),
                    help: String::new(),
                    group: "Endpoints".into(),
                    hint: "space-separated, or a range".into(),
                    active: true,
                    children: vec![Widget::tokens("dest_port", "Include", "Port", &[], "")],
                },
                ConditionItem {
                    key: "src_mac".into(),
                    label: "Source MAC addresses".into(),
                    help: String::new(),
                    group: "Endpoints".into(),
                    hint: "survives a changed lease".into(),
                    active: false,
                    children: vec![],
                },
            ],
        };
        let json = serde_json::to_value(&widget).unwrap();
        assert_eq!(json["type"], "conditions");
        assert_eq!(json["items"][0]["key"], "dest_port");
        assert_eq!(json["items"][0]["active"], true);
        assert!(json["items"][1].get("active").is_none());
        assert_eq!(json["items"][1]["children"], serde_json::json!([]));
    }

    #[test]
    fn a_checks_field_carries_its_members_and_a_token_list_its_style() {
        let checks = Widget::checks(
            "weekdays",
            "Weekdays",
            &["Mon".to_string()],
            vec![
                SelectOption::new("Mon", "Mon"),
                SelectOption::new("Tue", "Tue"),
            ],
        );
        let json = serde_json::to_value(&checks).unwrap();
        assert_eq!(json["kind"], "checks");
        assert_eq!(json["values"], serde_json::json!(["Mon"]));

        let tokens = Widget::tokens("dest_port", "Ports", "Port or range", &["53".into()], "");
        let json = serde_json::to_value(&tokens).unwrap();
        assert_eq!(json["style"], "tokens");
        assert_eq!(json["prompt"], "Port or range");
        assert!(json.get("errors").is_none());
    }

    #[test]
    fn a_section_carries_its_control_beside_the_title() {
        let widget = Widget::Section(SectionWidget {
            title: "Rule".into(),
            flush: true,
            control: Some(Box::new(Widget::switch("enabled", "Enabled", true))),
            ..Default::default()
        });
        let json = serde_json::to_value(&widget).unwrap();
        assert_eq!(json["flush"], true);
        assert_eq!(json["control"]["type"], "switch");
    }

    #[test]
    fn uci_text_spells_a_section_as_uci_export_does() {
        let values = json!({
            "name": "Allow Bob's NAS",
            "proto": ["tcp", "udp"],
            "dest_port": 445,
            "enabled": Value::Null,
        });
        let text = uci_text("rule", "cfg01", values.as_object().unwrap());
        assert_eq!(
            text,
            "config rule 'cfg01'\n\
             \toption dest_port '445'\n\
             \toption name 'Allow Bob'\\''s NAS'\n\
             \tlist proto 'tcp'\n\
             \tlist proto 'udp'"
        );
        // A section not yet created has no name to print.
        assert_eq!(uci_text("forwarding", "", &Map::new()), "config forwarding");
    }

    #[test]
    fn the_three_commit_shapes_are_distinguishable_on_the_wire() {
        let set = commit(
            "firewall",
            "allow_ping",
            serde_json::json!({"enabled": "0"}),
        );
        assert_eq!(
            serde_json::to_value(&set).unwrap(),
            serde_json::json!({
                "config": "firewall", "section": "allow_ping", "values": {"enabled": "0"}
            })
        );

        let created = commit_new("firewall", "rule", serde_json::json!({"target": "ACCEPT"}));
        assert_eq!(
            serde_json::to_value(&created).unwrap(),
            serde_json::json!({
                "config": "firewall", "section": "", "type": "rule",
                "values": {"target": "ACCEPT"}
            })
        );

        let removed = commit_delete("firewall", "allow_ping");
        assert_eq!(
            serde_json::to_value(&removed).unwrap(),
            serde_json::json!({"config": "firewall", "section": "allow_ping", "delete": true})
        );

        // A null among the values clears that one option.
        let cleared = commit(
            "firewall",
            "allow_ping",
            serde_json::json!({"dest_port": null}),
        );
        assert_eq!(
            serde_json::to_value(&cleared).unwrap()["values"],
            serde_json::json!({"dest_port": null})
        );
    }

    #[test]
    fn a_row_drawer_states_only_what_it_carries() {
        let closed = TableRow {
            id: "nas".into(),
            cells: vec![TableCell::default()],
            drawer: Some(RowDrawer {
                title: "Edit reservation — nas".into(),
                children: vec![Widget::text("body")],
                ..RowDrawer::default()
            }),
            ..TableRow::default()
        };
        assert_eq!(
            serde_json::to_value(&closed).unwrap()["drawer"],
            serde_json::json!({
                "title": "Edit reservation — nas",
                "children": [{"type": "text", "markdown": "body"}]
            })
        );

        // A refused submission comes back with its drawer already open.
        let refused = RowDrawer {
            title: "Edit record — nas.lan".into(),
            open: true,
            children: Vec::new(),
            ..RowDrawer::default()
        };
        let json = serde_json::to_value(&refused).unwrap();
        assert_eq!(json["open"], true);

        // A row without one says nothing about drawers at all.
        let bare = TableRow {
            cells: Vec::new(),
            ..TableRow::default()
        };
        assert!(serde_json::to_value(&bare).unwrap().get("drawer").is_none());
    }

    #[test]
    fn a_settings_seam_folds_its_own_rows() {
        let block = Widget::Settings(Settings {
            items: vec![SettingsItem {
                title: "Expand hosts".into(),
                ..SettingsItem::default()
            }],
            seam: Some(SettingsSeam {
                summary: "1 more option".into(),
                items: vec![SettingsItem {
                    title: "Skip /etc/hosts".into(),
                    ..SettingsItem::default()
                }],
            }),
            ..Default::default()
        });
        let json = serde_json::to_value(&block).unwrap();
        assert_eq!(json["seam"]["summary"], "1 more option");
        assert_eq!(json["seam"]["items"][0]["title"], "Skip /etc/hosts");

        let plain = Widget::Settings(Settings::default());
        assert!(serde_json::to_value(&plain).unwrap().get("seam").is_none());
    }

    #[test]
    fn settings_pills_speak_the_tone_vocabulary() {
        let pill = SettingsPill {
            variant: Tone::Success,
            text: "accept".into(),
        };
        let json = serde_json::to_value(&pill).unwrap();
        assert_eq!(
            json,
            serde_json::json!({"variant": "success", "text": "accept"})
        );
    }
}
