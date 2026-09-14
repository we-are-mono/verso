// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

//! The Verso DNS and DHCP plugin: three pages over `/etc/config/dhcp`.
//!
//! One file, two daemons, two domains. dnsmasq answers names and hands out IPv4
//! addresses; odhcpd announces the network and hands out IPv6. They read the
//! same config, so one plugin owns it — but an operator does not visit "the
//! dhcp config", they visit DNS or they visit DHCP, so the nav offers both and
//! each is a heading of its own. DHCP splits again into the leases you watch and
//! the configuration you edit; DNS has no live state to watch, so it stays one
//! face rather than growing a tab that is not about anything.
//!
//! Every request is answered from the reads the shell brokers with it — the uci
//! snapshot and the live lease table — so the plugin holds no state between
//! requests and reaches nothing itself (ADR-007). A submission is answered the
//! same way: the model is built from the snapshot, the accepted change is
//! applied to the model, and the page renders from the model, so what the
//! operator did is what they see beside the commit intent the shell stages.

use verso_plugin::{serve, Envelope, Form, Request};

mod config;
mod dns;
mod form;
mod format;
mod hosts;
mod leases;
mod live;
mod model;
mod options;
mod page;
mod records;
mod upstreams;

#[cfg(test)]
mod fixture;

use live::Leases;
use model::Dnsdhcp;

fn main() {
    serve("dnsdhcp", get, post);
}

fn get(request: &Request) -> Envelope {
    let model = Dnsdhcp::read(&request.snapshot);
    let leases = Leases::read(&request.ubus);
    match Route::of(&request.path) {
        Route::Dns => dns::page(&model),
        Route::Config => config::page(&model, &leases),
        Route::Leases => leases::page(&model, &leases, &reserve(request)),
    }
}

fn post(request: &Request, form: &Form) -> Envelope {
    let mut model = Dnsdhcp::read(&request.snapshot);
    let leases = Leases::read(&request.ubus);
    match Route::of(&request.path) {
        Route::Dns => dns::post(&mut model, form),
        Route::Config => config::post(&mut model, &leases, form),
        Route::Leases => leases::post(&mut model, &leases, form, &reserve(request)),
    }
}

/// reserve is the device a visitor arrived to reserve — the Devices page sends
/// a MAC in the query so the panel opens on arrival rather than after a hunt.
fn reserve(request: &Request) -> String {
    request.query.get(leases::RESERVE)
}

/// Route is what a request's sub-path asks for. A sub-path this plugin does not
/// publish answers with the page it leads with, so a stale link lands somewhere
/// real.
enum Route {
    Leases,
    Config,
    Dns,
}

impl Route {
    fn of(path: &str) -> Route {
        match path.trim_matches('/') {
            page::DNS => Route::Dns,
            page::CONFIG => Route::Config,
            _ => Route::Leases,
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use serde_json::Value;
    use verso_plugin::{Snapshot, Ubus};

    fn request(path: &str) -> Request {
        Request {
            path: path.into(),
            query: Form::default(),
            snapshot: fixture::snapshot(),
            ubus: fixture::ubus(),
        }
    }

    fn read(path: &str) -> Value {
        serde_json::to_value(get(&request(path))).expect("serialize")
    }

    fn answer(path: &str, body: &str) -> Value {
        serde_json::to_value(post(&request(path), &Form::parse(body))).expect("serialize")
    }

    #[test]
    fn each_sub_path_answers_with_its_own_page() {
        for (path, title, section) in [
            ("/", "DHCP", "Active leases"),
            ("/config", "DHCP", "Networks"),
            ("/dns", "DNS", "Names on your network"),
        ] {
            let body = read(path);
            assert_eq!(body["title"], title, "{path}");
            let content = &body["widget"]["children"][1];
            let first = match content["type"] == "form" {
                true => content["fields"][0]["title"].clone(),
                false => content["title"].clone(),
            };
            assert_eq!(first, section, "{path}");
        }
    }

    #[test]
    fn an_unpublished_sub_path_answers_with_the_leases_page() {
        for path in ["/nowhere", "/dns/records", "", "/config/lan"] {
            let body = read(path);
            assert_eq!(body["title"], "DHCP", "{path}");
            assert_eq!(
                body["widget"]["children"][1]["title"], "Active leases",
                "{path}"
            );
        }
    }

    #[test]
    fn a_read_without_brokered_leases_still_renders_every_page() {
        for path in ["/", "/config", "/dns"] {
            let request = Request {
                path: path.into(),
                query: Form::default(),
                snapshot: fixture::snapshot(),
                ubus: Ubus::from_value(Value::Null),
            };
            let body = serde_json::to_value(get(&request)).expect("serialize");
            assert_eq!(body["schema_version"], 1, "{path}");
        }
        // Without the lease table a network card states its span but nobody on it.
        let request = Request {
            path: page::CONFIG.into(),
            query: Form::default(),
            snapshot: fixture::snapshot(),
            ubus: Ubus::from_value(Value::Null),
        };
        let body = serde_json::to_value(get(&request)).expect("serialize");
        let card = &body["widget"]["children"][1]["fields"][0]["children"][0]["children"][0];
        assert_eq!(card["meta"], "10.0.0.0/24 · nobody right now");
    }

    // The Devices page links here with a MAC in the query; that device's panel is
    // open when the page arrives, so the visitor lands on the form they came for.
    #[test]
    fn a_reserve_query_opens_that_devices_panel() {
        let request = Request {
            path: "/".into(),
            query: Form::parse("reserve=42:e6:ad:ff:b7:af"),
            snapshot: fixture::snapshot(),
            ubus: fixture::ubus(),
        };
        let body = serde_json::to_value(get(&request)).expect("serialize");
        let rows = &body["widget"]["children"][1]["children"][0]["rows"];
        assert_eq!(rows[0]["drawer"]["open"], true);
        assert!(rows[1]["drawer"].get("open").is_none());
    }

    #[test]
    fn a_page_answers_only_the_submissions_it_drew() {
        // The record drawer belongs to the DNS page.
        let record = "_form=record&_section=cname_photos&kind=CNAME&name=photos.lan&target=nas.lan";
        assert_eq!(answer(page::DNS, record)["notice"]["level"], "success");
        for path in ["/", "/config"] {
            let elsewhere = answer(path, record);
            assert!(elsewhere.get("commit").is_none(), "{path}");
            assert_eq!(elsewhere["notice"]["level"], "danger", "{path}");
        }

        // The reservation drawer belongs to both DHCP faces, and to neither on DNS.
        let reserve = "_form=host&_section=&name=iphone&mac=42:e6:ad:ff:b7:af&ip=10.0.0.142";
        for path in ["/", "/config"] {
            assert_eq!(
                answer(path, reserve)["notice"]["level"],
                "success",
                "{path}"
            );
        }
        assert_eq!(answer(page::DNS, reserve)["notice"]["level"], "danger");
    }

    #[test]
    fn an_empty_config_still_answers_every_page() {
        let request = Request {
            path: page::CONFIG.into(),
            query: Form::default(),
            snapshot: Snapshot::from_value(serde_json::json!({"dhcp": {}, "network": {}})),
            ubus: fixture::ubus(),
        };
        let body = serde_json::to_value(get(&request)).expect("serialize");
        assert_eq!(body["title"], "DHCP");

        // A config with no daemon section runs on dnsmasq's own defaults, so
        // writing one of its options is what creates the section.
        let body =
            serde_json::to_value(post(&request, &Form::parse("logdhcp=1"))).expect("serialize");
        assert_eq!(
            body["commit"],
            serde_json::json!([{
                "config": "dhcp", "section": "", "type": "dnsmasq",
                "values": {"logdhcp": "1"}
            }])
        );
    }
}
