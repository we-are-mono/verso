// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

//! The Leases face: who holds an address right now.
//!
//! This is the page you visit to look, not to edit, so it carries no form. Its
//! one offering is keeping an address a device already has — and a reservation is
//! a page's worth of settings, so this page does not try to be that page. It
//! offers the door directly: a device that is not reserved carries a trailing
//! "Reserve IP" link straight onto the reservation page, prefilled from the
//! device. No panel in between — a reservation is edited on a page of its own
//! (ADR-005 §8), and the lease's own row already shows what that page prefills.
//!
//! The state column reads reserved for a device already pinned; the action column
//! offers the door for one that is not. Only one of the two is ever filled.

use verso_plugin::{Envelope, TableRow, Tone, Widget};

use crate::form;
use crate::format::{self, Subnet};
use crate::live::{Lease, Leases};
use crate::model::Dnsdhcp;
use crate::page;

const SUB: &str = "The addresses handed out right now — `/tmp/dhcp.leases`. Reserve a device's \
address to keep it permanently.";

/// RESERVE is the query parameter that names the device a visitor arrived to
/// reserve: the Leases "Reserve IP" link and the Devices page both carry a MAC in
/// it, and the reservation page reads it to open prefilled from that device.
pub const RESERVE: &str = "reserve";

/// page renders the leases listing — a look, not an edit.
pub fn page(model: &Dnsdhcp, leases: &Leases) -> Envelope {
    page::dhcp(Widget::stack(vec![
        page::filter("Filter — device, MAC, IP…"),
        Widget::section("Active leases", SUB, vec![listing(model, leases)]),
    ]))
}

/// post answers a submission to this page. The page draws nothing that posts —
/// reserving is a link onto the reservation page — so any submission here is one
/// this page did not draw.
pub fn post(model: &Dnsdhcp, leases: &Leases) -> Envelope {
    page(model, leases).with_notice(Tone::Danger, form::UNKNOWN)
}

/// NONE_YET and UNREAD are the listing's two silences, told apart: a router
/// handing out nothing, and a router this plugin could not ask.
const NONE_YET: &str = "No device holds an address yet — devices appear here as they join.";
const UNREAD: &str = "The lease table couldn’t be read. The networks and reservations \
under Configuration are unaffected.";

fn listing(model: &Dnsdhcp, leases: &Leases) -> Widget {
    let rows = leases.all().iter().map(|lease| row(model, lease)).collect();
    let empty = match leases.known() {
        true => NONE_YET,
        false => UNREAD,
    };
    page::table(
        page::columns(&[
            ("Device", "name"),
            ("IPv4", "mono"),
            ("IPv6", "mono"),
            ("MAC", "mono"),
            ("Expires", "keyword"),
            ("Static lease", "pill"),
            ("", "link"),
        ]),
        rows,
        empty,
    )
}

fn row(model: &Dnsdhcp, lease: &Lease) -> TableRow {
    let mut row = TableRow {
        id: lease.mac.clone(),
        cells: vec![
            page::name_cell(
                &format::device_name(&lease.hostname, &lease.mac),
                network_of(model, &lease.ipv4),
            ),
            page::address_cell(&lease.ipv4),
            page::address_cell(lease.ipv6s.first().map(String::as_str).unwrap_or("")),
            page::address_cell(&lease.mac),
            page::text_cell(&format::expires_in(lease.expires_at)),
        ],
        ..TableRow::default()
    };
    if model.reserved(&lease.mac) {
        // Already pinned: the state reads reserved, and there is nothing to do
        // here — its settings live on its own page under Configuration.
        row.cells.push(page::pill_cell("reserved", "info"));
        return row;
    }
    // Not reserved: no state to show, and the door straight onto the reservation
    // page prefilled from this lease — one click, no panel between.
    row.cells.push(page::empty_cell());
    row.cells.push(page::link_cell(
        "Reserve IP",
        format!("{}?{}={}", page::new_reservation_href(), RESERVE, lease.mac),
    ));
    row
}

/// network_of names the network a lease sits on, matched by address against the
/// pools' own interfaces — the same derivation the network cards state.
fn network_of<'a>(model: &'a Dnsdhcp, ipv4: &str) -> &'a str {
    model
        .interfaces
        .iter()
        .find(|interface| {
            Subnet::read(&interface.ipaddr, &interface.netmask)
                .is_some_and(|subnet| subnet.holds(ipv4))
        })
        .map(|interface| interface.name.as_str())
        .unwrap_or("")
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::fixture;
    use crate::live::Leases;
    use serde_json::Value as Json;
    use verso_plugin::{Ubus, Value};

    fn body(leases: &Leases) -> Json {
        serde_json::to_value(page(&fixture::dnsdhcp(), leases)).expect("serialize")
    }

    fn rows(body: &Json) -> Vec<Json> {
        body["widget"]["children"][1]["children"][0]["rows"]
            .as_array()
            .expect("rows")
            .clone()
    }

    #[test]
    fn the_page_watches_rather_than_edits() {
        let body = body(&fixture::leases());
        assert_eq!(body["title"], "DHCP");
        assert!(body.get("pages").is_none(), "DNS & DHCP has no subpages");
        assert_eq!(body["widget"]["children"][0]["type"], "filter");
        assert_eq!(body["widget"]["children"][1]["title"], "Active leases");
        // No form anywhere: there is nothing on this page to stage.
        assert!(!body["widget"]["children"]
            .as_array()
            .expect("children")
            .iter()
            .any(|child| child["type"] == "form"));
    }

    #[test]
    fn a_lease_states_the_device_its_addresses_and_when_it_runs_out() {
        let rows = rows(&body(&fixture::leases()));
        assert_eq!(
            rows[0]["cells"][0],
            serde_json::json!({"text": "toms-iphone", "chip": "lan", "chip_icon": "network"})
        );
        assert_eq!(rows[0]["cells"][1]["text"], "10.0.0.142");
        assert_eq!(rows[0]["cells"][2]["text"], "2a00:ee2:2d00:2e00::142");
        assert_eq!(rows[0]["cells"][3]["text"], "42:e6:ad:ff:b7:af");
        assert_eq!(rows[1]["cells"][0]["text"], "Device 0e:57");
        assert_eq!(rows[3]["cells"][0]["chip"], "guest");
    }

    #[test]
    fn a_dynamic_device_links_straight_to_the_reservation_page_no_panel() {
        let rows = rows(&body(&fixture::leases()));
        let iphone = &rows[0];
        // No state pill, and the trailing action is a plain link — one click onto
        // the reservation page prefilled from this lease. No drawer between.
        assert_eq!(iphone["cells"][5], serde_json::json!({}));
        assert_eq!(
            iphone["cells"][6],
            serde_json::json!({
                "text": "Reserve IP",
                "href": "/plugins/dnsdhcp/config/reservations/new?reserve=42:e6:ad:ff:b7:af"
            })
        );
        assert!(iphone.get("drawer").is_none(), "no panel in the way");
    }

    #[test]
    fn a_reserved_device_reads_as_reserved_and_offers_no_door() {
        let rows = rows(&body(&fixture::leases()));
        let nas = &rows[2];
        assert_eq!(
            nas["cells"][5],
            serde_json::json!({"text": "reserved", "variant": "info"})
        );
        // The action column is empty for a device already pinned, and nothing opens.
        assert!(nas["cells"].as_array().expect("cells").get(6).is_none());
        assert!(nas.get("drawer").is_none());
    }

    #[test]
    fn a_submission_to_this_page_changes_nothing() {
        let body =
            serde_json::to_value(post(&fixture::dnsdhcp(), &fixture::leases())).expect("serialize");
        assert!(body.get("commit").is_none());
        assert_eq!(body["notice"]["text"], form::UNKNOWN);
    }

    #[test]
    fn the_two_silences_are_told_apart() {
        // Each silence is the listing's one row, where the first lease would sit.
        let unread = body(&Leases::read(&Ubus::from_value(Value::Null)));
        let empty = &unread["widget"]["children"][1]["children"][0];
        assert_eq!(empty["type"], "table");
        assert_eq!(empty["empty_text"], UNREAD);

        let served = body(&Leases::read(&Ubus::from_value(
            serde_json::json!({"dhcpLeases": {"leases": []}}),
        )));
        let empty = &served["widget"]["children"][1]["children"][0];
        assert_eq!(empty["type"], "table");
        assert_eq!(empty["empty_text"], NONE_YET);
    }
}
