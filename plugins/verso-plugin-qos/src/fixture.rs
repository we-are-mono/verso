// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

//! One router's device policies, in the shapes the shell hands this plugin.
//!
//! The snapshot holds the three arrangements the tab has to get right and the
//! two it must leave alone: a device refused outright, one refused between hours,
//! one merely capped, and — beside them — firewall rules that are nobody's device
//! policy, including one that refuses a destination on the wan without naming a
//! MAC at all. It also writes one `src_mac` as a scalar and one as a uci list,
//! and one MAC in upper case, because configs in the field do all three.

use verso_plugin::{Form, Request, Snapshot, Ubus};

const SNAPSHOT: &str = include_str!("../testdata/snapshot.json");
const LEASES: &str = include_str!("../testdata/leases.json");

pub fn snapshot() -> Snapshot {
    Snapshot::from_value(serde_json::from_str(SNAPSHOT).expect("snapshot fixture"))
}

pub fn ubus() -> Ubus {
    Ubus::from_value(serde_json::from_str(LEASES).expect("leases fixture"))
}

/// request is a visit to one path, against the fixture router.
pub fn request(path: &str) -> Request {
    Request {
        path: path.into(),
        query: Form::default(),
        snapshot: snapshot(),
        ubus: ubus(),
    }
}

/// empty is the same visit against a router whose configs say nothing — the
/// first boot, and the shape every page must still answer in.
pub fn empty(path: &str) -> Request {
    Request {
        path: path.into(),
        query: Form::default(),
        snapshot: Snapshot::from_value(serde_json::json!({})),
        ubus: Ubus::from_value(serde_json::json!({})),
    }
}
