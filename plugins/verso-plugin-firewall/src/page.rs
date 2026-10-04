// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

//! The frame every firewall page shares, and the cells all three listings speak.
//!
//! Rules, port forwards, and zones are three views of one domain, so they carry
//! one heading, one subpage bar, and one width; only the lede and the content
//! change. The cell constructors live here for the same reason: an endpoint, a
//! hit count, and a switch mean the same thing on every page, and a listing that
//! spelled one of them differently would read as a different kind of thing.

use verso_plugin::{Envelope, PageTab, TableCell, TableEndpoint, TableRowAct, Widget};

use crate::format;
use crate::format::EM_DASH;

/// RULES, PORT_FORWARDS, ZONES and ACTIVITY are the sub-paths below this
/// plugin's mount.
pub const RULES: &str = "";
pub const PORT_FORWARDS: &str = "port-forwards";
pub const ZONES: &str = "zones";
pub const SETTINGS: &str = "settings";
pub const ACTIVITY: &str = "activity";

/// MOUNT is where the shell serves this plugin. A widget's href is a route
/// through the shell, not a plugin sub-path, so the plugin builds it in full —
/// unlike the subpage bar, whose paths the shell resolves against the mount.
pub const MOUNT: &str = "/plugins/firewall";

/// rules_href is the address of the rules listing itself — the door another page
/// opens when the thing it is about is a rule that does not exist yet.
///
/// The trailing slash is the address the shell actually serves this page at.
/// Without it the mux answers with a redirect to the slashed form, which for a
/// panel means the browser navigates twice and the page it was already on is
/// thrown away and rebuilt — visible as a jump. Naming the canonical address
/// costs nothing and removes the round trip.
pub fn rules_href() -> String {
    format!("{MOUNT}/")
}

/// port_forwards_href and zones_href are the addresses of those listings — the
/// door an editor returns to once its change is staged.
pub fn port_forwards_href() -> String {
    format!("{MOUNT}/{PORT_FORWARDS}")
}

pub fn zones_href() -> String {
    format!("{MOUNT}/{ZONES}")
}

// A port forward and a zone have no address of their own: each is read and
// edited in the panel beside its listing, so its door is a query on the
// listing's own address (redirect_drawer::href, zone_drawer::href) rather than a
// path below it.

/// tabs is the subpage bar. Every page declares the same list, so the bar stays
/// put as the visitor moves between them.
pub fn tabs() -> Vec<PageTab> {
    [
        ("Rules", RULES),
        ("Port forwards", PORT_FORWARDS),
        ("Zones", ZONES),
        ("Settings", SETTINGS),
        ("Activity", ACTIVITY),
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
pub fn envelope(heading: &str, subheading: &str, widget: Widget) -> Envelope {
    Envelope::page(heading, widget)
        .with_subheading(subheading)
        .with_width("wide")
        .with_pages(tabs())
        // Each page says what it is about in its own words, so the shell's
        // "<plugin> — <tab>" suffix would restate the tab strip underneath it.
        // The heading is a name, not a message about now, so it takes the tone
        // that drops the suffix and tints nothing.
        .with_tone("neutral")
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
/// A counter that has counted something is a fact about this rule and reads in
/// full ink; one that is still at nothing stays out of the way, so a column of
/// zeroes does not compete with the rules that have actually fired.
pub fn hits_cell(packets: Option<u64>) -> TableCell {
    match packets {
        Some(0) => muted("0"),
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
        // a verdict leads with the packet it decides about: filled in the
        // verdict's hue, hollow for drop, where the packet is simply gone
        dot: true,
        ..TableCell::default()
    }
}

/// acts_cell is the row's trailing acts: turn this section off (or back on),
/// and open the page where it is edited. They are glyphs rather than words
/// because they repeat on every row, and a column of the same word read forty
/// times is a column of noise — the row's own name is the thing to read.
///
/// The power act carries the state it would move to, so the glyph and its
/// sentence always describe what pressing it does rather than what is already
/// true. Off is the resting state made visible: the row reads muted, and the act
/// offers to turn it back on.
pub fn acts_cell(section: &str, enabled: bool, href: String, removal: TableRowAct) -> TableCell {
    let (icon, title, value) = match enabled {
        true => ("power", "Disable", "off"),
        false => ("power-off", "Enable", "on"),
    };
    TableCell {
        actions: vec![
            TableRowAct {
                icon: icon.into(),
                title: title.into(),
                name: section.into(),
                value: value.into(),
                ..TableRowAct::default()
            },
            TableRowAct {
                icon: "square-pen".into(),
                title: "Edit".into(),
                href,
                ..TableRowAct::default()
            },
            removal,
        ],
        ..TableCell::default()
    }
}

/// edit_act_cell is the row's trailing act where the row has no state to flip —
/// a zone is not switched off, it is edited or it is deleted.
pub fn edit_act_cell(href: String, removal: TableRowAct) -> TableCell {
    TableCell {
        actions: vec![
            TableRowAct {
                icon: "square-pen".into(),
                title: "Edit".into(),
                href,
                ..TableRowAct::default()
            },
            removal,
        ],
        ..TableCell::default()
    }
}

/// remove_act asks about exactly this row, independently of its edit and power acts.
pub fn remove_act(section: &str, title: &str, question: &str, consequence: &str) -> TableRowAct {
    TableRowAct {
        icon: "trash-2".into(),
        title: title.into(),
        name: crate::fields::REMOVE_FIELD.into(),
        value: section.into(),
        confirm_title: question.into(),
        confirm: consequence.into(),
        ..TableRowAct::default()
    }
}

/// name_cell is the row's subject: what this section is called, in full ink,
/// naming where it is edited. The shell draws a name as words, never a door;
/// the row's edit glyph is the way in.
pub fn name_cell(name: &str, href: String) -> TableCell {
    TableCell {
        text: name.into(),
        href,
        ..TableCell::default()
    }
}

/// index_cell is a row's place in the order the whole listing is evaluated in.
/// It reads at the secondary step: it is how to refer to the row out loud, not a
/// fact about the traffic.
pub fn index_cell(position: u32) -> TableCell {
    TableCell {
        text: position.to_string(),
        muted: true,
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
