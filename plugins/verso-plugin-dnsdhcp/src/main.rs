// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

//! DNS settings, with device reservation contributions and legacy editor URLs.
//! Reads and mutations are brokered by the shell; the plugin has no privileges.

use verso_plugin::{serve_described, Envelope, Form, Request};

mod config;
mod describe;
mod dns;
mod files;
mod form;
mod format;
mod hosts;
mod leases;
mod live;
mod model;
mod options;
mod page;
mod records;
mod servers;
mod settings;
mod upstreams;

#[cfg(test)]
mod fixture;

use live::Leases;
use model::Dnsdhcp;

fn main() {
    serve_described("dnsdhcp", get, post, describe::describe);
}

fn get(request: &Request) -> Envelope {
    if request.path.trim_matches('/').starts_with("files/") {
        return files::get(request);
    }
    if matches!(
        request.path.trim_matches('/'),
        "" | "config" | "dns" | "leases"
    ) {
        return settings::page(request);
    }
    let model = Dnsdhcp::read(&request.snapshot);
    let leases = Leases::read(&request.ubus);
    let answer = match Route::of(&request.path) {
        Route::Dns => dns::page(&model),
        Route::Config => config::page(&model, &leases),
        Route::Leases => leases::page(&model, &leases),
        Route::NewRecord => records::blank(),
        Route::EditRecord(section) => {
            records::edit(&model, &section).unwrap_or_else(|| records::missing(&model))
        }
        Route::NewServer => upstreams::blank(),
        Route::EditServer(index) => {
            upstreams::edit(&model, &index).unwrap_or_else(|| upstreams::missing(&model))
        }
        Route::NewReservation => hosts::blank(&leases, &request.query),
        Route::EntityDevice(mac) => hosts::entity_tab(&model, &leases, &mac),
        Route::EditReservation(section) => {
            hosts::edit(&model, &section).unwrap_or_else(|| hosts::missing(&model, &leases))
        }
    };
    current_page(request, answer)
}

fn post(request: &Request, form: &Form) -> Envelope {
    if request.path.trim_matches('/').starts_with("files/") {
        return files::post(request, form);
    }
    if matches!(
        request.path.trim_matches('/'),
        "" | "config" | "dns" | "leases"
    ) {
        return settings::post(request, form);
    }
    let mut model = Dnsdhcp::read(&request.snapshot);
    let leases = Leases::read(&request.ubus);
    let answer =
        match Route::of(&request.path) {
            Route::Dns => dns::post(&mut model, form),
            Route::Config => config::post(&mut model, &leases, form),
            Route::Leases => leases::post(&model, &leases),
            Route::NewRecord => records::create(&mut model, form),
            Route::EditRecord(section) => records::save(&mut model, &section, form)
                .unwrap_or_else(|| records::missing(&model)),
            Route::NewServer => upstreams::create(&mut model, form),
            Route::EditServer(index) => upstreams::save(&mut model, &index, form)
                .unwrap_or_else(|| upstreams::missing(&model)),
            Route::NewReservation => hosts::create(&mut model, &leases, form),
            Route::EditReservation(section) => hosts::save(&mut model, &leases, &section, form)
                .unwrap_or_else(|| hosts::missing(&model, &leases)),
            // A submitted tab saves into the reservation the device already has, or
            // creates the one it does not — the same form either way, so the panel
            // never asks which it is.
            Route::EntityDevice(mac) => match hosts::entity_section(&model, &mac) {
                Some(section) => hosts::save(&mut model, &leases, &section, form)
                    .unwrap_or_else(|| hosts::missing(&model, &leases)),
                None => hosts::create(&mut model, &leases, form),
            },
        };
    current_page(request, answer)
}

// Existing deep links remain usable; every completed edit returns to the new
// settings page, while reservations return to the Devices page that owns them.
fn current_page(request: &Request, mut answer: Envelope) -> Envelope {
    answer.pages.clear();
    if matches!(answer.title.as_str(), "DNS" | "DHCP") {
        let mut current = settings::page(request);
        current.commit = answer.commit;
        current.notice = answer.notice;
        if request.path.contains("reservations/") {
            current = current.with_back("Devices", "/devices");
        }
        return current;
    }
    if request.path.contains("reservations/") {
        answer = answer.with_back("Devices", "/devices");
    }
    answer
}

/// Route is what a request's sub-path asks for: one of the three pages, or one of
/// the three editors below two of them. A sub-path this plugin does not publish
/// answers with the page it leads with, so a stale link lands somewhere real; an
/// editor's own root (`dns/records` with no section) is the listing it belongs
/// to, and a sub-path naming no object there is that editor's to answer.
enum Route {
    Leases,
    Config,
    Dns,
    NewRecord,
    EditRecord(String),
    NewServer,
    EditServer(String),
    NewReservation,
    EditReservation(String),
    /// This plugin's say about one device, for the shell's device panel. It
    /// answers with a tab, not a page: the panel around it is the shell's, and
    /// the other tabs in it belong to plugins this one knows nothing about.
    EntityDevice(String),
}

impl Route {
    fn of(path: &str) -> Route {
        let path = path.trim_matches('/');
        if let Some(rest) = sub_path(path, page::RECORDS) {
            return match rest {
                "" => Route::Dns,
                page::NEW => Route::NewRecord,
                section => Route::EditRecord(section.to_string()),
            };
        }
        if let Some(rest) = sub_path(path, page::SERVERS) {
            return match rest {
                "" => Route::Dns,
                page::NEW => Route::NewServer,
                index => Route::EditServer(index.to_string()),
            };
        }
        if let Some(rest) = sub_path(path, page::RESERVATIONS) {
            return match rest {
                "" => Route::Config,
                page::NEW => Route::NewReservation,
                section => Route::EditReservation(section.to_string()),
            };
        }
        if let Some(mac) = path.strip_prefix("entity/device/") {
            return Route::EntityDevice(mac.to_string());
        }
        match path {
            page::DNS => Route::Dns,
            page::CONFIG => Route::Config,
            _ => Route::Leases,
        }
    }
}

/// sub_path returns the tail below an editor's prefix, or None when the path is
/// not under it. The prefix ends at a slash or at the end of the path, so `dns`
/// never matches the `dns/records` prefix and a section is never glued onto it.
fn sub_path<'a>(path: &'a str, prefix: &str) -> Option<&'a str> {
    let rest = path.strip_prefix(prefix)?;
    match rest.strip_prefix('/') {
        Some(section) => Some(section),
        None if rest.is_empty() => Some(""),
        None => None,
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
    fn every_former_listing_lands_on_the_single_settings_page() {
        for path in ["/", "/dns", "/config", "/leases", "/nowhere", "/config/lan"] {
            let body = read(path);
            assert_eq!(body["title"], "DNS & DHCP");
            assert!(body.get("pages").is_none());
            assert_eq!(body["widget"]["children"][0]["submit"], "Save");
        }
    }

    #[test]
    fn each_editor_sub_path_opens_its_own_page() {
        for (path, title) in [
            ("/dns/records/new", "New record"),
            ("/dns/records/domain_backup_v4", "Edit record"),
            ("/dns/servers/new", "New server"),
            ("/dns/servers/0", "Edit server"),
            ("/config/reservations/new", "New reservation"),
            ("/config/reservations/host_nas", "Edit reservation"),
        ] {
            assert_eq!(read(path)["title"], title, "{path}");
        }

        // An editor's own root is the listing it belongs to.
        assert_eq!(read("/dns/records")["title"], "DNS & DHCP");
        assert_eq!(read("/dns/servers")["title"], "DNS & DHCP");
        assert_eq!(read("/config/reservations")["title"], "DNS & DHCP");
    }

    #[test]
    fn a_stale_editor_sub_path_lands_on_the_listing_it_belongs_to() {
        for (path, title) in [
            ("/dns/records/no_such_record", "DNS & DHCP"),
            ("/dns/servers/9", "DNS & DHCP"),
            ("/config/reservations/no_such_host", "DNS & DHCP"),
        ] {
            let body = read(path);
            assert_eq!(body["title"], title, "{path}");
            assert_eq!(body["notice"]["level"], "danger", "{path}");
            assert!(body.get("commit").is_none(), "{path}");
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
    }

    #[test]
    fn an_editor_submission_is_answered_by_its_own_route() {
        // A record posts to its own page, which answers with the DNS listing.
        let record = answer(
            "/dns/records/cname_photos",
            "kind=CNAME&name=photos.lan&target=nas.lan",
        );
        assert_eq!(record["title"], "DNS & DHCP");
        assert_eq!(record["notice"]["level"], "success");
        assert_eq!(record["commit"][0]["section"], "cname_photos");

        // A reservation posts to its own page, which answers with the DHCP
        // configuration.
        let reservation = answer(
            "/config/reservations/new",
            "name=iphone&mac=42:e6:ad:ff:b7:af&ip=10.0.0.142",
        );
        assert_eq!(reservation["title"], "DNS & DHCP");
        assert_eq!(reservation["notice"]["text"], "Address reserved.");
        assert_eq!(reservation["commit"][0]["type"], "host");
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
        assert_eq!(body["title"], "DNS & DHCP");
        let body = serde_json::to_value(post(&request, &Form::parse("logdhcp=1"))).unwrap();
        assert!(body.get("commit").is_none());
    }
}
