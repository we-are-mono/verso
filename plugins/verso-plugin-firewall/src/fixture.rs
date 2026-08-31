// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

//! One router's firewall, in the shapes the shell hands this plugin.
//!
//! The snapshot is a uci read as rpcd returns it — sections keyed by their uci
//! handle, carrying their `.type`, `.name`, `.index`, and `.anonymous` meta — and
//! it holds the cases the listings have to get right: named and unnamed sections,
//! a list option written both ways, a disabled rule, a zone with no policy of its
//! own, a zone claiming a raw device, an uplink that learns its address, a snat
//! redirect among the port forwards. The counters are the shape verso-rpcd
//! returns, including a reflection entry a port forward must not count.
//!
//! The editor's snapshot is a different kind of fixture. Its `everything` rule
//! carries every condition and parameter the editor draws at once — no operator
//! would write such a rule, but a round-trip has to survive the whole vocabulary,
//! and the option it also carries that the editor does not draw (`extra`) is the
//! one a save must leave exactly where it found it.
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

/// table finds the listing inside one titled region.
pub fn table(body: &Value, title: &str) -> Value {
    let section = section(body, title);
    find(&section, &|value| value["type"] == "table")
        .unwrap_or_else(|| panic!("no table under {title}"))
}

/// control finds one named control anywhere in a page, which is how a test asks
/// what the operator would see without walking the composition.
pub fn control(body: &Value, name: &str) -> Value {
    find(body, &|value| {
        value.get("type").is_some() && value.get("name") == Some(&Value::from(name))
    })
    .unwrap_or_else(|| panic!("no control named {name}"))
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
