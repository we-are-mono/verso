// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

//! The frame the three pages share, and the cells their listings speak.
//!
//! DNS and DHCP are two domains in one config file, so they are two headings
//! rather than one: DNS has a single kind of visit and therefore no top bar,
//! while DHCP has two — the leases you watch and the configuration you edit —
//! and every DHCP page declares the same bar so it stays put underneath the
//! visitor.
//!
//! The cells live here because an address, a name, and a MAC mean the same thing
//! on every listing; a page that spelled one of them differently would read as a
//! different kind of thing.

use verso_plugin::{Envelope, PageTab, TableCell, TableColumn, TableRow, Widget};

use crate::format::EM_DASH;

/// LEASES, CONFIG and DNS are the sub-paths below this plugin's mount.
pub const LEASES: &str = "";
pub const CONFIG: &str = "config";
pub const DNS: &str = "dns";

const DHCP_SUB: &str = "Who gets an address on each of your networks, and the addresses they \
hold right now. Changes rewrite /etc/config/dhcp.";

const DNS_SUB: &str = "How names are answered on your network — the ones you add, where the \
rest go, and what filters them. Changes rewrite /etc/config/dhcp.";

/// tabs is the DHCP top bar. Both DHCP pages declare the same list.
pub fn tabs() -> Vec<PageTab> {
    [("Leases", LEASES), ("Configuration", CONFIG)]
        .into_iter()
        .map(|(label, path)| PageTab {
            label: label.into(),
            path: path.into(),
        })
        .collect()
}

/// dhcp wraps one DHCP page's content. The listings are wide: a lease carries
/// two address families, a MAC and an expiry, and none of that reads better in a
/// reading column.
pub fn dhcp(widget: Widget) -> Envelope {
    Envelope::page("DHCP", widget)
        .with_subheading(DHCP_SUB)
        .with_width("wide")
        .with_pages(tabs())
}

/// dns wraps the DNS page's content. It declares no subpages: DNS has no live
/// state to watch, so a second face would be a tab that is not about anything.
pub fn dns(widget: Widget) -> Envelope {
    Envelope::page("DNS", widget)
        .with_subheading(DNS_SUB)
        .with_width("wide")
}

/// filter is the page-wide lens every page carries. Whether it renders is the
/// shell's call — it counts what the page really lists and drops a lens over a
/// page short enough to read whole.
pub fn filter(placeholder: &str) -> Widget {
    Widget::Filter {
        placeholder: placeholder.into(),
    }
}

/// page_form is the whole editing surface of a configuration page: one form the
/// staged-changes bar submits, so a toggle flipped in one card and a value typed
/// in another are saved and applied together. A refusal belongs to the form
/// rather than to one row — a settings row is a name, a description and a
/// control, with nowhere to put a message — and it is also what makes the answer
/// a 422, which is how the capsule knows to keep the operator here.
pub fn page_form(refusal: &str, fields: Vec<Widget>) -> Widget {
    Widget::Form {
        style: "page".into(),
        submit: String::new(),
        error: refusal.into(),
        fields,
    }
}

/// form_grid lays a run of controls across columns at the tighter gutter a form
/// takes.
pub fn form_grid(columns: u32, children: Vec<Widget>) -> Widget {
    Widget::Grid {
        style: "form".into(),
        columns,
        children,
    }
}

/// columns builds a column set from label/kind pairs.
pub fn columns(spec: &[(&str, &str)]) -> Vec<TableColumn> {
    spec.iter()
        .map(|(label, kind)| TableColumn {
            label: (*label).into(),
            kind: (*kind).into(),
        })
        .collect()
}

/// table is a listing in the one table style: bare hairline rows, no header
/// band, and no order to persist — nothing here is evaluated in sequence. Every
/// listing here is one section among several, so an empty one keeps its place on
/// the page and says what the absence means; the shell draws that as one row.
pub fn table(columns: Vec<TableColumn>, rows: Vec<TableRow>, empty: &str) -> Widget {
    Widget::Table {
        style: String::new(),
        title: String::new(),
        detail: String::new(),
        condensed: false,
        align: String::new(),
        reorder_config: String::new(),
        reorder_label: String::new(),
        columns,
        rows,
        drawer_label: String::new(),
        drawer_icon: String::new(),
        empty_text: empty.into(),
    }
}

/// name_cell is the row's identity, with the network it sits on riding inline as
/// a chip — a device is a name and where it is, never a name in one column and a
/// network in another.
pub fn name_cell(text: &str, chip: &str) -> TableCell {
    TableCell {
        text: text.into(),
        chip: chip.into(),
        chip_icon: match chip.is_empty() {
            true => String::new(),
            false => "network".into(),
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

/// pill_cell is a state as a status pill; an empty state renders as the column's
/// faint dash, which is what keeps the filled pills meaningful.
pub fn pill_cell(text: &str, variant: &str) -> TableCell {
    TableCell {
        text: text.into(),
        variant: variant.into(),
        ..TableCell::default()
    }
}

/// button_cell is an in-row action that opens the row's drawer.
pub fn button_cell(label: &str) -> TableCell {
    TableCell {
        button: label.into(),
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
