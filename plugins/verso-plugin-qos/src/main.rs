// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

//! The Verso device-limits plugin: what a device may reach, when, and how fast.
//!
//! It owns no page an operator goes looking for. The question it answers — "keep
//! this one off the internet after nine" — is asked *at* a device, so its real
//! surface is one tab of the shell's device panel, which the ban and the sliders
//! on a device's row both lead to. The page it does publish is the other half of
//! that: which devices are under a policy at all, which no listing of devices can
//! show and nobody would otherwise remember.
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
mod page;

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
        Route::Listing => page::page(&model::every_policy(&request.snapshot), &leases),
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
        // The listing edits nothing, so a submission to it is answered with
        // itself rather than refused: nothing was asked for and nothing changed.
        Route::Listing => page::page(&model::every_policy(&request.snapshot), &leases),
        Route::EntityDevice(mac) => {
            entity::save(&Policy::read(&request.snapshot, &mac), &leases, form)
        }
    }
}

/// Route is what a request asks for: the listing, or this plugin's say about one
/// device. Anything else is the listing, so a stale link lands somewhere real.
enum Route {
    Listing,
    /// This plugin's say about one device, for the shell's device panel. It
    /// answers with a tab, not a page: the panel around it is the shell's, and
    /// the other tabs in it belong to plugins this one knows nothing about.
    EntityDevice(String),
}

impl Route {
    fn of(path: &str) -> Route {
        match path.trim_matches('/').strip_prefix("entity/device/") {
            Some(mac) => Route::EntityDevice(mac.to_string()),
            None => Route::Listing,
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use serde_json::Value;

    fn body(env: Envelope) -> Value {
        serde_json::to_value(&env).expect("serialize")
    }

    #[test]
    fn a_sub_path_this_plugin_does_not_publish_lands_on_the_listing() {
        for path in ["", "/", "nonsense", "entity/zone/lan"] {
            assert!(matches!(Route::of(path), Route::Listing), "{path}");
        }
        assert!(matches!(
            Route::of("entity/device/00:11:22:33:44:55"),
            Route::EntityDevice(mac) if mac == "00:11:22:33:44:55"
        ));
    }

    #[test]
    fn the_listing_states_every_device_under_a_policy_and_nothing_else() {
        let request = fixture::request("/");
        let listing = body(get(&request));
        assert_eq!(listing["title"], "Device limits");
        let rows = listing["widget"]["children"][0]["rows"]
            .as_array()
            .expect("rows");
        assert_eq!(rows.len(), 3);
        // The blocked device, the one on a curfew, and the one merely capped.
        assert_eq!(rows[0]["cells"][0]["text"], "kids-ipad");
        assert_eq!(rows[0]["cells"][2]["text"], "blocked");
        assert_eq!(rows[0]["cells"][2]["variant"], "danger");
        assert_eq!(rows[1]["cells"][2]["text"], "on a schedule");
        assert_eq!(rows[1]["cells"][3]["text"], "21:00–07:00 · Mon–Fri");
        assert_eq!(rows[2]["cells"][2]["text"], "allowed");
        assert_eq!(rows[2]["cells"][4]["text"], "50 Mbit/s");
    }

    #[test]
    fn an_empty_config_still_answers_every_route() {
        let request = fixture::empty("/");
        assert_eq!(body(get(&request))["title"], "Device limits");
        let tab = body(get(&fixture::empty("entity/device/00:11:22:33:44:55")));
        assert_eq!(tab["title"], "Limits & schedule");
        assert_eq!(tab["state"], "no limit");
    }
}
