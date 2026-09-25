// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

//! The Verso device-limits plugin: what a device may reach, when, and how fast.
//!
//! Limits are edited in the device drawer. A bulk summary supplies configured
//! devices to the shell's roster, keeping offline policies findable there too.
//!
//! The policy is two configs because it is two mechanisms. Refusing a device the
//! route out is a firewall rule, and fw4 is what enforces it — including the
//! schedule, which it reads natively. A speed cap is not a firewall rule in any
//! form, so it is written to this plugin's own config instead of being bent into
//! one. The tab shows both, side by side, and says which is which.
//!
//! Every request is answered from the reads the shell brokers with it (ADR-007),
//! so the plugin holds no state between requests and reaches nothing itself.

use verso_plugin::{serve, Envelope, Form, Request};

mod entity;
mod form;
mod leases;
mod model;
mod summaries;

#[cfg(test)]
mod fixture;

use leases::Leases;
use model::Policy;

fn main() {
    serve("qos", get, post);
}

fn get(request: &Request) -> Envelope {
    let leases = Leases::read(&request.ubus);
    match Route::of(&request.path) {
        Route::Summaries => summaries::read(&request.snapshot, &leases),
        Route::EntityDevice(mac) => entity::tab(
            &Policy::read(&request.snapshot, &mac),
            &leases,
            &form::Errors::default(),
        ),
    }
}

fn post(request: &Request, form: &Form) -> Envelope {
    let leases = Leases::read(&request.ubus);
    match Route::of(&request.path) {
        // A summary request never changes configuration.
        Route::Summaries => summaries::read(&request.snapshot, &leases),
        Route::EntityDevice(mac) => {
            entity::save(&Policy::read(&request.snapshot, &mac), &leases, form)
        }
    }
}

/// A roster summary or one device’s editor; there is no standalone page.
enum Route {
    Summaries,
    /// This plugin's say about one device, for the shell's device panel. It
    /// answers with a tab, not a page: the panel around it is the shell's, and
    /// the other tabs in it belong to plugins this one knows nothing about.
    EntityDevice(String),
}

impl Route {
    fn of(path: &str) -> Route {
        match path.trim_matches('/').strip_prefix("entity/device/") {
            Some(mac) => Route::EntityDevice(mac.to_string()),
            None => Route::Summaries,
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn roster_summaries_include_each_configured_device() {
        let reply = get(&fixture::request("/entity/device/"));
        let summaries = reply.entities.expect("successful read");
        assert_eq!(summaries.len(), 3);
        assert_eq!(summaries[0].name, "kids-ipad");
        assert_eq!(summaries[0].state, "Internet block");
        assert_eq!(summaries[1].state, "Schedule & speed limit");
        assert_eq!(summaries[2].state, "Speed limit");
        assert!(reply.commit.is_empty());
    }

    #[test]
    fn offline_policies_remain_in_roster_without_leases() {
        let mut request = fixture::request("/entity/device/");
        request.ubus = verso_plugin::Ubus::from_value(serde_json::json!({}));
        let summaries = get(&request).entities.unwrap();
        assert_eq!(summaries.len(), 3);
        assert!(summaries.iter().all(|entry| entry.name == entry.id));
    }

    #[test]
    fn unreadable_configuration_is_not_an_empty_roster() {
        assert!(get(&fixture::empty("/entity/device/")).entities.is_none());
        let mut request = fixture::empty("/entity/device/");
        request.snapshot =
            verso_plugin::Snapshot::from_value(serde_json::json!({"firewall": {}, "qos": {}}));
        assert!(get(&request).entities.unwrap().is_empty());
        request.snapshot = verso_plugin::Snapshot::from_value(serde_json::json!({"firewall": {}}));
        assert!(get(&request).entities.is_none());
    }

    #[test]
    fn a_device_still_opens_its_editor() {
        let tab = get(&fixture::empty("/entity/device/00:11:22:33:44:55"));
        assert_eq!(tab.title, "Limits");
        assert_eq!(tab.state, "no limit");
        assert!(tab.entities.is_none());
    }
}
