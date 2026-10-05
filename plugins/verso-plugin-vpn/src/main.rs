// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

//! The Verso VPN plugin: every tunnel on the router on one page, whatever
//! runs it, and each OpenVPN instance opened to its own panel.
//!
//! An OpenVPN client is usually a profile file a provider handed out, named by
//! a uci section. The file stays the truth — the provider will hand out a new
//! one — so the page reads it out and changes only what the section says.
//!
//! Every request is answered from the reads the shell brokers with it
//! (ADR-007): the uci configs, and the helper's `vpnState`, which reads the
//! profiles, the processes and their logs, so the plugin reaches nothing
//! itself.

use std::time::{SystemTime, UNIX_EPOCH};

use verso_plugin::{serve, Envelope, Errors, Form, Request, Tone};

mod drawer;
mod listing;
mod model;

#[cfg(test)]
mod fixture;

use drawer::Stated;
use model::Vpn;

fn main() {
    serve("vpn", get, post);
}

fn get(request: &Request) -> Envelope {
    let vpn = Vpn::read(request);
    let open = request.query.get(listing::OPEN);
    let panel = vpn.instance(&open).map(|instance| {
        let stated = Stated::of(instance);
        (
            open.as_str(),
            drawer::drawer(instance, now(), &stated, &Errors::default()),
        )
    });
    listing::page(&vpn, panel)
}

/// post answers the panel's own form — saved, or given back with what is
/// missing, and in either case drawn again from what was typed, which is what
/// keeps its preview current while it is edited — or a row's start or stop.
fn post(request: &Request, form: &Form) -> Envelope {
    let vpn = Vpn::read(request);
    let open = request.query.get(listing::OPEN);
    if let (Some(instance), false) = (vpn.instance(&open), form.get(drawer::PANEL).is_empty()) {
        let stated = Stated::submitted(form);
        return match drawer::save(instance, form) {
            Ok(op) => {
                let panel = drawer::drawer(instance, now(), &stated, &Errors::default());
                listing::page(&vpn, Some((&open, panel))).with_commit(vec![op])
            }
            Err((stated, errors)) => {
                let panel = drawer::drawer(instance, now(), &stated, &errors);
                listing::page(&vpn, Some((&open, panel))).with_notice(Tone::Danger, drawer::REFUSED)
            }
        };
    }
    match listing::switch(&vpn, form) {
        Some(op) => listing::page(&vpn, None).with_commit(vec![op]),
        None => listing::page(&vpn, None),
    }
}

fn now() -> u64 {
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .map(|d| d.as_secs())
        .unwrap_or(0)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn an_address_naming_an_instance_opens_its_panel_on_the_listing() {
        let body = serde_json::to_value(get(&fixture::request("/?open=proton"))).unwrap();
        let rows = body["widget"]["rows"].as_array().unwrap();
        assert_eq!(rows[0]["drawer"]["title"], "proton");
        assert!(rows[1].get("drawer").is_none());
        let none = serde_json::to_value(get(&fixture::request("/?open=nobody"))).unwrap();
        assert!(none["widget"]["rows"][0].get("drawer").is_none());
    }

    #[test]
    fn the_panel_saves_its_section_and_a_row_flips_its_own() {
        let saved = serde_json::to_value(post(
            &fixture::request("/?open=proton"),
            &Form::parse("panel=1"),
        ))
        .unwrap();
        assert_eq!(saved["commit"][0]["section"], "proton");
        assert_eq!(saved["commit"][0]["values"]["enabled"], "0");
        let flipped =
            serde_json::to_value(post(&fixture::request("/"), &Form::parse("work=on"))).unwrap();
        assert_eq!(flipped["commit"][0]["section"], "work");
        let nothing =
            serde_json::to_value(post(&fixture::request("/"), &Form::parse("x=1"))).unwrap();
        assert!(nothing.get("commit").is_none());
    }
}
