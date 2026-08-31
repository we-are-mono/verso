// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

//! The frame every firewall page shares, and the cells all three listings speak.
//!
//! Rules, port forwards, and zones are three views of one domain, so they carry
//! one heading, one subpage bar, and one width; only the lede and the content
//! change. The cell constructors live here for the same reason: an endpoint, a
//! hit count, and a switch mean the same thing on every page, and a listing that
//! spelled one of them differently would read as a different kind of thing.

use verso_plugin::{Envelope, PageTab, TableCell, TableEndpoint, Widget};

use crate::format;
use crate::format::EM_DASH;

/// RULES, PORT_FORWARDS and ZONES are the sub-paths below this plugin's mount.
pub const RULES: &str = "";
pub const PORT_FORWARDS: &str = "port-forwards";
pub const ZONES: &str = "zones";

/// RULE_EDITOR is the sub-path a rule's own page lives under; a redirect's own
/// page lives directly under the listing it belongs to. NEW is the one name
/// below either that is not a section: `<listing>/new` opens a blank editor,
/// `<listing>/<section>` opens that section's.
pub const RULE_EDITOR: &str = "rules";
pub const NEW: &str = "new";

/// MOUNT is where the shell serves this plugin. A widget's href is a route
/// through the shell, not a plugin sub-path, so the plugin builds it in full —
/// unlike the subpage bar, whose paths the shell resolves against the mount.
pub const MOUNT: &str = "/plugins/firewall";

/// filter is a listing's search field. Whether it renders is the shell's call —
/// it counts what the page really lists and drops a lens over a page short
/// enough to read whole — so a page states the one it would like and says
/// nothing about when it is worth having.
pub fn filter(placeholder: &str) -> Widget {
    Widget::Filter {
        placeholder: placeholder.into(),
    }
}

/// rule_href is the address of one rule's editor.
pub fn rule_href(section: &str) -> String {
    format!("{MOUNT}/{RULE_EDITOR}/{section}")
}

/// new_rule_href is the address of the blank rule editor.
pub fn new_rule_href() -> String {
    format!("{MOUNT}/{RULE_EDITOR}/{NEW}")
}

/// redirect_href is the address of one port forward's editor.
pub fn redirect_href(section: &str) -> String {
    format!("{MOUNT}/{PORT_FORWARDS}/{section}")
}

/// new_redirect_href is the address of the blank port-forward editor.
pub fn new_redirect_href() -> String {
    format!("{MOUNT}/{PORT_FORWARDS}/{NEW}")
}

/// tabs is the subpage bar. Every page declares the same list, so the bar stays
/// put as the visitor moves between them.
pub fn tabs() -> Vec<PageTab> {
    [
        ("Rules", RULES),
        ("Port forwards", PORT_FORWARDS),
        ("Zones", ZONES),
    ]
    .into_iter()
    .map(|(label, path)| PageTab {
        label: label.into(),
        path: path.into(),
    })
    .collect()
}

/// envelope wraps one page's content in the shared firewall frame. The listings
/// are wide: they carry a full traffic path per row and nothing about them reads
/// better in a reading column.
pub fn envelope(subheading: &str, widget: Widget) -> Envelope {
    Envelope::page("Firewall", widget)
        .with_subheading(subheading)
        .with_width("wide")
        .with_pages(tabs())
}

/// endpoint_cell names one side of a traffic path. Explicit addresses are what
/// the rule really matches, so they take precedence over the zone the rule also
/// names; `*` matches wherever traffic comes from; and a side that names neither
/// a zone nor an address is the router itself.
pub fn endpoint_cell(addresses: &[String], zone: &str) -> TableCell {
    if !addresses.is_empty() {
        return TableCell {
            endpoints: addresses
                .iter()
                .map(|address| TableEndpoint {
                    kind: "device".into(),
                    label: address.clone(),
                })
                .collect(),
            ..TableCell::default()
        };
    }
    match zone {
        "*" => TableCell::any(),
        "" => TableCell::router(),
        zone => TableCell::zone(zone),
    }
}

/// hits_cell states a section's kernel hit count, or that none was read for it.
pub fn hits_cell(packets: Option<u64>) -> TableCell {
    match packets {
        Some(packets) => TableCell {
            text: format::hits(packets),
            ..TableCell::default()
        },
        None => muted(EM_DASH),
    }
}

/// mono_cell carries a verbatim machine value, promoted a step because it is the
/// detail the row is scanned for; nothing to state reads as a quiet dash.
pub fn mono_cell(text: &str) -> TableCell {
    if text.is_empty() {
        return muted(EM_DASH);
    }
    TableCell {
        text: text.into(),
        emphasis: true,
        ..TableCell::default()
    }
}

/// text_cell is a plain value, or a quiet dash when there is none.
pub fn text_cell(text: &str) -> TableCell {
    if text.is_empty() {
        return muted(EM_DASH);
    }
    TableCell {
        text: text.into(),
        ..TableCell::default()
    }
}

/// pill_cell is an enum value as a status pill. An empty value renders as the
/// column's faint dash, which is what keeps the filled pills meaningful.
pub fn pill_cell(text: &str, variant: &str) -> TableCell {
    TableCell {
        text: text.into(),
        variant: variant.into(),
        ..TableCell::default()
    }
}

/// toggle_cell is a section's enabled state. The name is the uci section, which
/// is what a flip posts back under.
pub fn toggle_cell(section: &str, on: bool) -> TableCell {
    TableCell {
        name: section.into(),
        on,
        ..TableCell::default()
    }
}

/// edit_cell is the row's trailing Edit button on a listing whose rows have no
/// editor: present, so the column reads the same on every listing, and plainly
/// unavailable.
pub fn edit_cell() -> TableCell {
    TableCell {
        button: "Edit".into(),
        disabled: true,
        ..TableCell::default()
    }
}

/// edit_link_cell is the row's trailing Edit on a listing whose rows do have an
/// editor: a link to the page that edits this one section.
pub fn edit_link_cell(href: String) -> TableCell {
    TableCell {
        text: "Edit".into(),
        href,
        ..TableCell::default()
    }
}

fn muted(text: &str) -> TableCell {
    TableCell {
        text: text.into(),
        muted: true,
        ..TableCell::default()
    }
}
