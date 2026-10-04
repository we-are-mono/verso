// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

//! The leases on the DHCP page: who holds an address right now.
//!
//! A lease is a reading, not an object, so its row opens nothing of its own and
//! offers one act: keeping the address a device already has. That act opens the
//! reservation drawer filled from the lease — the reservation is the object, and
//! it lives in its drawer (ADR-005 §8). A device already pinned says so instead.

use verso_plugin::{ColumnWidth, Table, TableCell, TableColumn, TableRow, TableRowAct, Widget};

use crate::format;
use crate::live::{Lease, Leases};
use crate::model::Dnsdhcp;
use crate::page;

/// RESERVE is the query that names the device a blank reservation drawer opens
/// filled from: a lease's Reserve carries its MAC in it.
pub const RESERVE: &str = "reserve";

/// NONE_YET and UNREAD are the listing's two silences, told apart: a router
/// handing out nothing, and a router this plugin could not ask.
const NONE_YET: &str = "No device holds an address yet. Devices appear here as they join.";
const UNREAD: &str = "The lease table couldn’t be read. The servers and reservations above are \
unaffected.";

/// table is the live leases.
pub fn table(model: &Dnsdhcp, leases: &Leases) -> Widget {
    Widget::Table(Table {
        columns: [
            ("Device", "name", ColumnWidth::Name),
            ("Network", "entity", ColumnWidth::Short),
            ("Address", "mono", ColumnWidth::Address),
            ("MAC", "mono", ColumnWidth::Address),
            ("Expires in", "runtime", ColumnWidth::Grow),
            ("", "actions", ColumnWidth::Short),
        ]
        .into_iter()
        .map(|(label, kind, width)| TableColumn {
            label: label.into(),
            kind: kind.into(),
            width,
        })
        .collect(),
        rows: leases.all().iter().map(|lease| row(model, lease)).collect(),
        empty_text: match leases.known() {
            true => NONE_YET,
            false => UNREAD,
        }
        .into(),
        ..Default::default()
    })
}

/// reserve_href is the blank reservation drawer, filled from one lease.
pub fn reserve_href(mac: &str) -> String {
    format!("{}&{RESERVE}={mac}", page::open_href(page::NEW))
}

fn row(model: &Dnsdhcp, lease: &Lease) -> TableRow {
    let network = model.network_of(&lease.ipv4);
    let reserved = model.reserved(&lease.mac);
    let mut name = TableCell {
        text: format::device_name(&lease.hostname, &lease.mac),
        muted: lease.hostname.is_empty(),
        ..TableCell::default()
    };
    // Each row offers only the act that applies to it: a pinned device wears
    // the pin; one that is not offers to pin it, which opens the reservation
    // drawer where the row stands.
    let mut acts = vec![];
    let mut panel = String::new();
    if reserved {
        name.tag = "reserved".into();
        name.tag_variant = "success".into();
        name.tag_icon = "pin".into();
    } else {
        panel = reserve_href(&lease.mac);
        acts.push(TableRowAct {
            icon: "pin".into(),
            title: "Reserve".into(),
            href: panel.clone(),
            ..TableRowAct::default()
        });
    }
    TableRow {
        id: lease.mac.clone(),
        cells: vec![
            name,
            page::network_cell(network),
            page::address_cell(&lease.ipv4),
            page::mono_cell(&lease.mac),
            page::text_cell(&format::expires_in(lease.expires_at)),
            TableCell {
                actions: acts,
                ..TableCell::default()
            },
        ],
        panel,
        ..TableRow::default()
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::fixture;
    use serde_json::Value as Json;
    use verso_plugin::{Ubus, Value};

    fn table_of(leases: &Leases) -> Json {
        serde_json::to_value(table(&fixture::dnsdhcp(), leases)).expect("serialize")
    }

    #[test]
    fn a_lease_states_the_device_its_network_and_when_it_runs_out() {
        let rows = table_of(&fixture::leases())["rows"].clone();
        assert_eq!(rows[0]["cells"][0]["text"], "toms-iphone");
        assert_eq!(rows[0]["cells"][1]["chips"][0]["label"], "lan");
        assert_eq!(rows[0]["cells"][2]["text"], "10.0.0.142");
        assert_eq!(rows[0]["cells"][3]["text"], "42:e6:ad:ff:b7:af");
        // A device that offered no name is called by its MAC, at the quieter step.
        assert_eq!(rows[1]["cells"][0]["text"], "Device 0e:57");
        assert_eq!(rows[1]["cells"][0]["muted"], true);
        assert_eq!(rows[3]["cells"][1]["chips"][0]["label"], "guest");
    }

    #[test]
    fn an_unreserved_device_offers_to_pin_its_address_in_a_drawer_where_it_stands() {
        let rows = table_of(&fixture::leases())["rows"].clone();
        let iphone = &rows[0];
        let door = "/plugins/dnsdhcp/?open=new&reserve=42:e6:ad:ff:b7:af";
        assert_eq!(iphone["panel"], door);
        let act = &iphone["cells"][5]["actions"][0];
        assert_eq!(act["icon"], "pin");
        assert_eq!(act["title"], "Reserve");
        assert_eq!(act["href"], door);
    }

    #[test]
    fn a_reserved_device_wears_the_pin_and_offers_nothing() {
        let rows = table_of(&fixture::leases())["rows"].clone();
        let nas = &rows[2];
        assert_eq!(nas["cells"][0]["tag"], "reserved");
        assert_eq!(nas["cells"][0]["tag_icon"], "pin");
        assert!(nas["cells"][5].get("actions").is_none());
        assert!(nas.get("panel").is_none());
    }

    #[test]
    fn the_two_silences_are_told_apart() {
        let unread = table_of(&Leases::read(&Ubus::from_value(Value::Null)));
        assert_eq!(unread["empty_text"], UNREAD);
        let served = table_of(&Leases::read(&Ubus::from_value(
            serde_json::json!({"dhcpLeases": {"leases": []}}),
        )));
        assert_eq!(served["empty_text"], NONE_YET);
    }
}
