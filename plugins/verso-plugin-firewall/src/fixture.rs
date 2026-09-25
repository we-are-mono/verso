// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

//! One router's firewall, in the shapes the shell hands this plugin.
//!
//! The snapshot is a uci read as rpcd returns it — sections keyed by their uci
//! handle, carrying their `.type`, `.name`, `.index`, and `.anonymous` meta — and
//! it holds the cases the listings have to get right: named and unnamed sections,
//! a list option written both ways, a disabled rule, a zone with no policy of its
//! own, a zone claiming a raw device, an uplink that learns its address, a snat
//! redirect among the port forwards, and a crossing switched off — which fw4
//! skips, so the zone it leaves reaches nowhere through it. The counters are the
//! shape verso-rpcd returns, including a reflection entry a port forward must
//! not count.
//!
//! The editor's snapshot is a different kind of fixture. Its `everything` rule
//! and `everything_forward` redirect carry every condition and parameter their
//! editors draw at once — no operator would write either, but a round-trip has to
//! survive the whole vocabulary, and the options they also carry that no editor
//! draws (`extra`, and the forward's unsupported `monthdays`) are the ones a save
//! must leave exactly where it found them.
//!
//! The probes below go with them: a page composes its filter only when the
//! listing is long enough to need one, so a test asks for the region or control
//! it means rather than counting a stack's children.

use serde_json::Value;
use verso_plugin::{Snapshot, Ubus};

use crate::counters::Counters;
use crate::model::Firewall;

const SNAPSHOT: &str = include_str!("../testdata/snapshot.json");
const COUNTERS: &str = include_str!("../testdata/counters.json");
const EDITOR: &str = include_str!("../testdata/editor.json");

pub fn snapshot() -> Snapshot {
    Snapshot::from_value(serde_json::from_str(SNAPSHOT).expect("snapshot fixture"))
}

pub fn firewall() -> Firewall {
    Firewall::read(&snapshot())
}

pub fn counters() -> Counters {
    Counters::read(&Ubus::from_value(
        serde_json::from_str(COUNTERS).expect("counters fixture"),
    ))
}

pub fn editor_snapshot() -> Snapshot {
    Snapshot::from_value(serde_json::from_str(EDITOR).expect("editor fixture"))
}

pub fn editor_firewall() -> Firewall {
    Firewall::read(&editor_snapshot())
}

/// section finds a page's titled region by its heading.
pub fn section(body: &Value, title: &str) -> Value {
    find(body, &|value| {
        value["type"] == "section" && value["title"] == *title
    })
    .unwrap_or_else(|| panic!("no section titled {title}"))
}

/// listing finds the page's own grid — the one a listing page IS, rather than
/// one of several inside titled regions. The heading already names what the
/// page lists, so the grid carries no title of its own to look it up by.
pub fn listing(body: &Value) -> Value {
    widget(body, "table")
}

/// widget finds the first widget of one kind anywhere in a page — for the
/// pieces that are not a titled region and carry no name of their own (the
/// page-wide lens, a nothing-here state).
pub fn widget(body: &Value, kind: &str) -> Value {
    find(body, &|value| value["type"] == kind)
        .unwrap_or_else(|| panic!("no {kind} widget on the page"))
}

/// control finds one named control anywhere in a page, which is how a test asks
/// what the operator would see without walking the composition.
pub fn control(body: &Value, name: &str) -> Value {
    find(body, &|value| {
        value.get("type").is_some() && value.get("name") == Some(&Value::from(name))
    })
    .unwrap_or_else(|| panic!("no control named {name}"))
}

/// find_with is the same search a test can steer itself, for a widget named by
/// what it carries rather than by a type or a name.
pub fn find_with(value: &Value, wanted: &dyn Fn(&Value) -> bool) -> Option<Value> {
    find(value, wanted)
}

fn find(value: &Value, wanted: &dyn Fn(&Value) -> bool) -> Option<Value> {
    match value {
        Value::Object(map) => {
            if wanted(value) {
                return Some(value.clone());
            }
            map.values().find_map(|child| find(child, wanted))
        }
        Value::Array(items) => items.iter().find_map(|item| find(item, wanted)),
        _ => None,
    }
}

/// submission is what a browser would post for a panel: every control on it,
/// in the encoding the form uses. It reads the widget tree the way the shell
/// renders it — a switch posts only while it is on, a condition the rule does
/// not carry is a template and posts nothing at all — so what this builds is
/// what would really arrive.
pub fn submission(body: &Value) -> String {
    let mut pairs: Vec<(String, String)> = Vec::new();
    // Gather only the editor's form. The surrounding listing carries its own
    // toggle and removal actions, which a Save never submits.
    gather(&editing_form(body).expect("the panel's form"), &mut pairs);
    pairs
        .iter()
        .map(|(name, value)| {
            format!(
                "{}={}",
                name,
                value
                    .replace('%', "%25")
                    .replace('&', "%26")
                    .replace(' ', "+")
            )
        })
        .collect::<Vec<String>>()
        .join("&")
}

fn gather(node: &Value, out: &mut Vec<(String, String)>) {
    match node {
        Value::Object(map) => {
            let name = map
                .get("name")
                .and_then(Value::as_str)
                .unwrap_or_default()
                .to_string();
            match map.get("type").and_then(Value::as_str).unwrap_or_default() {
                "field" if !name.is_empty() => {
                    match map.get("values").and_then(Value::as_array) {
                        // A checks field posts one entry per box ticked.
                        Some(values) => values.iter().for_each(|value| {
                            out.push((name.clone(), string(value)));
                        }),
                        None => out.push((name, string(map.get("value").unwrap_or(&Value::Null)))),
                    }
                    // A paired row is two controls in one: the second half
                    // posts under its own name and would otherwise be missed.
                    if let Some(pair) = map.get("pair") {
                        let field = string(pair.get("name").unwrap_or(&Value::Null));
                        if !field.is_empty() {
                            out.push((field, string(pair.get("value").unwrap_or(&Value::Null))));
                        }
                    }
                    return;
                }
                "list" if !name.is_empty() => {
                    if let Some(items) = map.get("items").and_then(Value::as_array) {
                        items
                            .iter()
                            .for_each(|item| out.push((name.clone(), string(item))));
                    }
                    return;
                }
                // A switch and a gate post only while they are on; a gate's
                // fields are in the DOM either way, so they post either way.
                "switch" => {
                    if map.get("on") == Some(&Value::Bool(true)) && !name.is_empty() {
                        out.push((name, "1".into()));
                    }
                    return;
                }
                "conditional" => {
                    if map.get("checked") == Some(&Value::Bool(true)) && !name.is_empty() {
                        out.push((name, "1".into()));
                    }
                }
                // The catalogue renders what the rule carries as controls and
                // keeps the rest in templates, which post nothing.
                "conditions" => {
                    if let Some(items) = map.get("items").and_then(Value::as_array) {
                        for item in items {
                            if item["active"] == Value::Bool(true) {
                                gather(&item["children"], out);
                            }
                        }
                    }
                    return;
                }
                _ => {}
            }
            for value in map.values() {
                gather(value, out);
            }
        }
        Value::Array(items) => items.iter().for_each(|item| gather(item, out)),
        _ => {}
    }
}

pub fn errors_in(node: &Value, out: &mut Vec<(String, Value)>) {
    match node {
        Value::Object(map) => {
            if let Some(error) = map.get("error") {
                if error.as_str().unwrap_or_default() != "" {
                    out.push((
                        map.get("name")
                            .and_then(Value::as_str)
                            .unwrap_or("?")
                            .into(),
                        error.clone(),
                    ));
                }
            }
            if let Some(errors) = map.get("errors") {
                if errors.as_object().map(|m| !m.is_empty()).unwrap_or(false) {
                    out.push((
                        map.get("name")
                            .and_then(Value::as_str)
                            .unwrap_or("?")
                            .into(),
                        errors.clone(),
                    ));
                }
            }
            map.values().for_each(|v| errors_in(v, out));
        }
        Value::Array(items) => items.iter().for_each(|i| errors_in(i, out)),
        _ => {}
    }
}

fn string(value: &Value) -> String {
    value.as_str().unwrap_or_default().to_string()
}

/// editing_form is the first form in a panel: the one its controls are in.
fn editing_form(node: &Value) -> Option<Value> {
    match node {
        Value::Object(map) => {
            if map.get("type") == Some(&Value::from("form"))
                && map
                    .get("submit")
                    .and_then(Value::as_str)
                    .unwrap_or_default()
                    != ""
            {
                return Some(node.clone());
            }
            map.values().find_map(editing_form)
        }
        Value::Array(items) => items.iter().find_map(editing_form),
        _ => None,
    }
}
