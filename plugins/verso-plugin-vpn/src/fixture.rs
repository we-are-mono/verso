// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

//! One router's tunnels, in the shapes the shell hands this plugin: a Proton
//! client connected on tun0, a work profile switched off, the package's own
//! samples left as shipped, and Tailscale running beside them.

use verso_plugin::{Form, Request, Snapshot, Ubus};

const SNAPSHOT: &str = include_str!("../testdata/snapshot.json");
const STATE: &str = include_str!("../testdata/vpn.json");

/// request is a visit to one path, against the fixture router.
pub fn request(path: &str) -> Request {
    let (path, query) = path.split_once('?').unwrap_or((path, ""));
    Request {
        path: path.into(),
        query: Form::parse(query),
        snapshot: Snapshot::from_value(serde_json::from_str(SNAPSHOT).expect("snapshot fixture")),
        ubus: Ubus::from_value(serde_json::from_str(STATE).expect("state fixture")),
    }
}

/// empty is the same visit against a router with no tunnels at all.
pub fn empty(path: &str) -> Request {
    Request {
        path: path.into(),
        query: Form::default(),
        snapshot: Snapshot::from_value(serde_json::json!({})),
        ubus: Ubus::from_value(serde_json::json!({})),
    }
}
