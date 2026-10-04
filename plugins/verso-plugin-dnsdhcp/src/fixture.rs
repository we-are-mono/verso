// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

//! One router's DNS and DHCP, in the shapes the shell hands this plugin.
//!
//! The snapshot is a uci read as rpcd returns it — sections keyed by their uci
//! handle, carrying their `.type`, `.name` and `.index` meta — and it holds the
//! cases the pages have to get right: all five record kinds across their four
//! section types, a domain-limited upstream beside plain ones, a list option
//! written as a whitespace string, a pool that states its offsets and one that
//! states none, an uplink pool whose interface commits to no address and which
//! nonetheless carries an option its card does not draw, a reservation whose
//! device is here and one whose device is not.
//!
//! The leases are the shape the shell brokers: the DHCPv4 table with the IPv6
//! addresses of the same devices merged in, including one device that offered no
//! name and one on another network.

use verso_plugin::{Snapshot, Ubus};

use crate::live::Leases;
use crate::model::Dnsdhcp;

const SNAPSHOT: &str = include_str!("../testdata/snapshot.json");
const LEASES: &str = include_str!("../testdata/leases.json");

pub fn snapshot() -> Snapshot {
    Snapshot::from_value(serde_json::from_str(SNAPSHOT).expect("snapshot fixture"))
}

pub fn dnsdhcp() -> Dnsdhcp {
    Dnsdhcp::read(&snapshot())
}

pub fn ubus() -> Ubus {
    Ubus::from_value(serde_json::from_str(LEASES).expect("leases fixture"))
}

pub fn leases() -> Leases {
    Leases::read(&ubus())
}

/// request is a visit to one path of this router's pages.
pub fn request(path: &str) -> verso_plugin::Request {
    verso_plugin::Request {
        path: path.into(),
        query: verso_plugin::Form::default(),
        snapshot: snapshot(),
        ubus: ubus(),
    }
}
