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

/// LEASES, CONFIG and DNS are the sub-paths below this plugin's mount that name a
/// page; the three editors live below two of them.
pub const LEASES: &str = "";
pub const CONFIG: &str = "config";
pub const DNS: &str = "dns";

/// RECORDS, SERVERS and RESERVATIONS are the sub-paths an object's own page lives
/// under — `<listing>/<section>` edits one, `<listing>/new` adds one. A record
/// and an upstream are things a name is answered by, so both sit under DNS; a
/// reservation is a thing on a network, so it sits under the DHCP configuration.
pub const RECORDS: &str = "dns/records";
pub const SERVERS: &str = "dns/servers";
pub const RESERVATIONS: &str = "config/reservations";

/// NEW is the one name below an editor's sub-path that is not a section.
pub const NEW: &str = "new";

/// MOUNT is where the shell serves this plugin. A widget's href is a route
/// through the shell, not a plugin sub-path, so the plugin builds it in full —
/// unlike the subpage bar, whose paths the shell resolves against the mount.
pub const MOUNT: &str = "/plugins/dnsdhcp";

/// record_href and new_record_href address one name record's page and the blank
/// one.
pub fn record_href(section: &str) -> String {
    format!("{MOUNT}/{RECORDS}/{section}")
}

pub fn new_record_href() -> String {
    format!("{MOUNT}/{RECORDS}/{NEW}")
}

/// server_href and new_server_href address one upstream's page and the blank
/// one. An upstream is addressed by its position in `list server`, not a uci
/// section name — that is all the list gives it to be found by.
pub fn server_href(index: usize) -> String {
    format!("{MOUNT}/{SERVERS}/{index}")
}

pub fn new_server_href() -> String {
    format!("{MOUNT}/{SERVERS}/{NEW}")
}

/// reservation_href and new_reservation_href address one reservation's page and
/// the blank one.
pub fn reservation_href(section: &str) -> String {
    format!("{MOUNT}/{RESERVATIONS}/{section}")
}

pub fn new_reservation_href() -> String {
    format!("{MOUNT}/{RESERVATIONS}/{NEW}")
}

const DHCP_SUB: &str = "Who gets an address on each of your networks, and the addresses they \
hold right now. Changes rewrite /etc/config/dhcp.";

const DNS_SUB: &str = "How names are answered on your network — the ones you add, where the \
rest go, and what filters them. Changes rewrite /etc/config/dhcp.";

/// tabs is the DHCP top bar. Both DHCP pages declare the same list.
pub fn tabs() -> Vec<PageTab> {
    [("Leases", LEASES), ("Configuration", CONFIG), ("DNS", DNS)]
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

/// dns_editor frames one DNS object's page. The title is a fixed label — "Edit
/// record", "New upstream" — so it stays translatable, and the object's own name
/// rides the subheading rather than being composed into the title. DNS has no
/// top bar, so the editor declares none either; the column is the form's
/// 640px, because one object is a form — its fields, its Advanced disclosure
/// and the rule its Save stands under all end where the fields do.
pub fn dns_editor(title: &str, subheading: &str, widget: Widget) -> Envelope {
    Envelope::page(title, widget)
        .with_subheading(subheading)
        .with_width("form")
        // Cancel and a completed save both return to the DNS page, where the
        // record now waits in the stage.
        .with_back("Cancel", &format!("{MOUNT}/{DNS}"))
}

/// dhcp_editor frames one DHCP object's page. Like dns_editor, the form's
/// measure, but it keeps the DHCP top bar so the Leases/Configuration tabs stay
/// put underneath an operator editing a reservation.
pub fn dhcp_editor(title: &str, subheading: &str, widget: Widget) -> Envelope {
    Envelope::page(title, widget)
        .with_subheading(subheading)
        .with_width("form")
        .with_pages(tabs())
        // The reservation lives on the DHCP configuration page; that is where
        // Cancel and a completed save return.
        .with_back("Cancel", &format!("{MOUNT}/{CONFIG}"))
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
/// a 422, which is how the shell knows to keep the operator here.
pub fn page_form(refusal: &str, fields: Vec<Widget>) -> Widget {
    Widget::Form {
        style: "page".into(),
        submit: String::new(),
        error: refusal.into(),
        fields,
        note: String::new(),
        target: String::new(),
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
            ..TableColumn::default()
        })
        .collect()
}

/// table is a listing in the one table style: bare hairline rows, no header
/// band, and no order to persist — nothing here is evaluated in sequence. Every
/// listing here is one section among several, so an empty one keeps its place on
/// the page and says what the absence means; the shell draws that as one row.
pub fn table(columns: Vec<TableColumn>, rows: Vec<TableRow>, empty: &str) -> Widget {
    listing(columns, rows, empty, "", "")
}

/// listing is a table that also carries the tail affordance a page of editable
/// objects wants — the quiet "add another" row the shell draws after the last,
/// pointing at the blank editor where the next one lands. A listing with no
/// editor of its own (the live leases) uses [`table`] and adds nothing.
pub fn listing(
    columns: Vec<TableColumn>,
    rows: Vec<TableRow>,
    empty: &str,
    add_label: &str,
    add_href: &str,
) -> Widget {
    Widget::Table {
        style: String::new(),
        title: String::new(),
        detail: String::new(),
        dense: false,
        reorder_config: String::new(),
        reorder_label: String::new(),
        columns,
        rows,
        drawer_label: String::new(),
        drawer_icon: String::new(),
        empty_text: empty.into(),
        add_label: add_label.into(),
        add_href: add_href.into(),
        note: String::new(),
        stream: None,
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

/// link_cell is a listing row's trailing action link — the word that opens or
/// creates the object this row is about, hard right where the eye ends.
pub fn link_cell(label: &str, href: String) -> TableCell {
    TableCell {
        text: label.into(),
        href,
        ..TableCell::default()
    }
}

/// edit_link_cell is a listing row's trailing link to the page that edits this
/// one object — the same word opens an object everywhere in this plugin.
pub fn edit_link_cell(href: String) -> TableCell {
    link_cell("Edit", href)
}

/// empty_cell holds a column's place on a row with nothing to put there, so the
/// column renders its faint dash rather than the cells after it sliding left.
pub fn empty_cell() -> TableCell {
    TableCell::default()
}

fn muted(text: &str) -> TableCell {
    TableCell {
        text: text.into(),
        muted: true,
        ..TableCell::default()
    }
}
