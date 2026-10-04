// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

//! What the two pages share: where they live, the cells their listings speak,
//! and the frame a drawer's form stands in.
//!
//! The cells live here because an address, a network and a MAC mean the same
//! thing on every listing; a page that spelled one of them differently would
//! read as a different kind of thing.

use verso_plugin::{TableCell, TableChip, Value, Widget};

use crate::format::EM_DASH;

/// MOUNT is where the shell serves this plugin. A widget's href is a route
/// through the shell, not a plugin sub-path, so the plugin builds it in full.
pub const MOUNT: &str = "/plugins/dnsdhcp";

/// DNS is the DNS page's sub-path; the DHCP page is the plugin's root, so a
/// stale link to any older path lands on it.
pub const DNS: &str = "dns";

/// RECORDS and SERVERS are the sub-paths a DNS object's own page lives under —
/// `<listing>/<section>` edits one, `<listing>/new` adds one.
pub const RECORDS: &str = "dns/records";
pub const SERVERS: &str = "dns/servers";

/// NEW is the one name below an editor's sub-path that is not a section, and
/// the one `open` names a reservation not made yet by.
pub const NEW: &str = "new";

/// OPEN is the query a DHCP page address names its open drawer by: a server's
/// network, a reservation's section or its device's MAC, or `new`.
pub const OPEN: &str = "open";

/// PANEL marks a submission as a drawer's own, told apart from the page's
/// settings form that posts to the same page.
pub const PANEL: &str = "_panel";

/// root_href is the DHCP page with nothing open.
pub fn root_href() -> String {
    format!("{MOUNT}/")
}

/// open_href is the DHCP page with one drawer open.
pub fn open_href(key: &str) -> String {
    format!("{MOUNT}/?{OPEN}={}", encode(key))
}

/// dns_href is the DNS page.
pub fn dns_href() -> String {
    format!("{MOUNT}/{DNS}")
}

fn encode(value: &str) -> String {
    value
        .bytes()
        .map(|b| match b.is_ascii_alphanumeric() || b"-_.~:".contains(&b) {
            true => (b as char).to_string(),
            false => format!("%{b:02X}"),
        })
        .collect()
}

/// dns_editor frames one DNS object's page. The title is a fixed label — "Edit
/// record", "New upstream" — so it stays translatable; the object's own name is
/// the form's first value. The column is the form's 640px, because one object is
/// a form.
pub fn dns_editor(title: &str, widget: Widget) -> verso_plugin::Envelope {
    verso_plugin::Envelope::page(title, widget)
        .with_width("form")
        // Cancel and a completed save both return to the DNS page.
        .with_back("Cancel", &dns_href())
}

/// panel_form is a drawer's one form: its fields, its save, and the marker that
/// says the submission is the drawer's.
pub fn panel_form(submit: &str, mut fields: Vec<Widget>) -> Widget {
    fields.push(Widget::hidden(PANEL, "1"));
    Widget::Form {
        style: String::new(),
        submit: submit.into(),
        error: String::new(),
        fields,
        note: String::new(),
        target: String::new(),
    }
}

/// uci_block is one section as `/etc/config` spells it: the section line, then
/// every option it states, lists as one line per value. A section the shell
/// has not named yet is spelled with no name.
pub fn uci_block(kind: &str, name: &str, options: &[(&str, Value)]) -> String {
    let mut out = match name.is_empty() {
        true => format!("config {kind}"),
        false => format!("config {kind} '{name}'"),
    };
    for (option, value) in options {
        match value {
            Value::Array(items) => {
                for item in items.iter().filter_map(Value::as_str) {
                    out.push_str(&format!("\n\tlist {option} '{item}'"));
                }
            }
            Value::String(text) if !text.is_empty() => {
                out.push_str(&format!("\n\toption {option} '{text}'"));
            }
            _ => {}
        }
    }
    out
}

/// network_cell cites the network a row sits on, as the entity chip every
/// listing cites a network with.
pub fn network_cell(network: &str) -> TableCell {
    TableCell {
        chips: match network.is_empty() {
            true => vec![],
            false => vec![TableChip {
                icon: "network".into(),
                label: network.into(),
                tone: "accent".into(),
                ..TableChip::default()
            }],
        },
        ..TableCell::default()
    }
}

/// address_cell is a verbatim machine value the row is scanned for: promoted a
/// step and copyable, because it is what gets typed somewhere else.
pub fn address_cell(text: &str) -> TableCell {
    if text.is_empty() {
        return muted(EM_DASH);
    }
    TableCell {
        text: text.into(),
        copy: true,
        emphasis: true,
        ..TableCell::default()
    }
}

/// mono_cell is a machine value that is read rather than copied.
pub fn mono_cell(text: &str) -> TableCell {
    if text.is_empty() {
        return muted(EM_DASH);
    }
    TableCell {
        text: text.into(),
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

fn muted(text: &str) -> TableCell {
    TableCell {
        text: text.into(),
        muted: true,
        ..TableCell::default()
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use verso_plugin::json;

    #[test]
    fn a_section_is_spelled_as_the_file_spells_it() {
        let block = uci_block(
            "dhcp",
            "lan",
            &[
                ("interface", json!("lan")),
                ("ignore", Value::Null),
                ("dhcp_option", json!(["3,10.0.0.1", "6,10.0.0.30"])),
            ],
        );
        assert_eq!(
            block,
            "config dhcp 'lan'\n\toption interface 'lan'\n\tlist dhcp_option '3,10.0.0.1'\n\tlist dhcp_option '6,10.0.0.30'"
        );
        assert_eq!(uci_block("host", "", &[]), "config host");
    }

    #[test]
    fn an_open_address_survives_any_key() {
        assert_eq!(open_href("lan"), "/plugins/dnsdhcp/?open=lan");
        assert_eq!(
            open_href("30:9c:23:5e:88:01"),
            "/plugins/dnsdhcp/?open=30:9c:23:5e:88:01"
        );
        assert_eq!(open_href("a b"), "/plugins/dnsdhcp/?open=a%20b");
    }
}
