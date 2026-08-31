// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

//! The Leases face: who holds an address right now.
//!
//! This is the page you visit to look, not to edit, so it carries no page form
//! and offers exactly one action — keeping an address a device already has. That
//! action is where a reservation comes from: the config is not the place an
//! operator discovers a device, the lease table is.
//!
//! The trailing column carries the state and the action in one place: a device
//! that is already reserved reads as reserved, and one that is not offers to
//! become so. Only the second opens a panel, because only it has anything to
//! ask.

use verso_plugin::{Envelope, Form, TableRow, Tone, Widget};

use crate::form::{self, Errors};
use crate::format::{self, Subnet};
use crate::hosts::{self, Host};
use crate::live::{Lease, Leases};
use crate::model::{same_mac, Dnsdhcp};
use crate::page;

const SUB: &str = "The addresses handed out right now — `/tmp/dhcp.leases`. Open a device to \
reserve its address permanently.";

/// RESERVE is the query parameter that names the device a visitor arrived to
/// reserve: the Devices page links here with a MAC, and the panel that would
/// have taken a click opens on arrival instead.
pub const RESERVE: &str = "reserve";

/// page renders the leases listing. `reserve` is the MAC the visitor came for,
/// or "" — a MAC no lease answers to changes nothing.
pub fn page(model: &Dnsdhcp, leases: &Leases, reserve: &str) -> Envelope {
    render(model, leases, None, reserve)
}

fn render(
    model: &Dnsdhcp,
    leases: &Leases,
    refusal: Option<&hosts::Refusal>,
    reserve: &str,
) -> Envelope {
    page::dhcp(Widget::stack(vec![
        page::filter("Filter — device, MAC, IP…"),
        Widget::section(
            "Active leases",
            SUB,
            vec![listing(model, leases, refusal, reserve)],
        ),
    ]))
}

fn listing(
    model: &Dnsdhcp,
    leases: &Leases,
    refusal: Option<&hosts::Refusal>,
    reserve: &str,
) -> Widget {
    if leases.all().is_empty() {
        return nothing_here(leases);
    }
    let rows = leases
        .all()
        .iter()
        .map(|lease| row(model, lease, refusal, reserve))
        .collect();
    page::table(
        page::columns(&[
            ("Device", "name"),
            ("IPv4", "mono"),
            ("IPv6", "mono"),
            ("MAC", "mono"),
            ("Expires", "keyword"),
            ("Static lease", "pill"),
        ]),
        rows,
        // This listing is the whole page, and its two silences are told apart
        // above by an empty state of their own.
        "",
    )
}

/// nothing_here tells the two silences apart: a router handing out nothing, and
/// a router this plugin could not ask.
fn nothing_here(leases: &Leases) -> Widget {
    match leases.known() {
        true => Widget::empty(
            "wifi",
            "No device holds an address",
            "Nothing has asked this router for an address yet. Devices appear here as they join.",
            Vec::new(),
        ),
        false => Widget::empty(
            "wifi",
            "Live leases aren’t available",
            "Verso could not read the lease table, so it can't say who holds an address. \
             The networks and reservations under Configuration are unaffected.",
            Vec::new(),
        ),
    }
}

fn row(
    model: &Dnsdhcp,
    lease: &Lease,
    refusal: Option<&hosts::Refusal>,
    reserve: &str,
) -> TableRow {
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
        row.cells.push(page::pill_cell("reserved", "info"));
        return row;
    }
    row.cells.push(page::button_cell("Reserve IP"));
    let refused = refusal.filter(|refused| same_mac(&refused.host.mac, &lease.mac));
    row.drawer = match refused {
        Some(refused) => hosts::reserve_drawer(&refused.host, &refused.errors, true),
        None => hosts::reserve_drawer(
            &Host::of_lease(lease),
            &Errors::default(),
            same_mac(reserve, &lease.mac),
        ),
    };
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

/// post answers a submission to this page. The only thing it draws is the
/// reserve panel, so that is the only submission it accepts. A refusal reopens
/// the panel it came from, which outranks the panel the visitor arrived on.
pub fn post(model: &mut Dnsdhcp, leases: &Leases, form: &Form, reserve: &str) -> Envelope {
    if form::kind(form).as_deref() != Some(form::HOST) || form::deletes(form) {
        return render(model, leases, None, reserve).with_notice(Tone::Danger, form::UNKNOWN);
    }
    match hosts::save(model, form) {
        hosts::Saved::Ops(ops, said) => render(model, leases, None, reserve)
            .with_notice(Tone::Success, said)
            .with_commit(ops),
        hosts::Saved::Refused(refusal) => render(model, leases, Some(&refusal), reserve)
            .with_notice(Tone::Danger, form::REFUSED),
        hosts::Saved::Unknown => {
            render(model, leases, None, reserve).with_notice(Tone::Danger, form::UNKNOWN)
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::fixture;
    use crate::live::Leases;
    use serde_json::Value as Json;
    use verso_plugin::{Ubus, Value};

    fn body(leases: &Leases) -> Json {
        opened(leases, "")
    }

    /// opened renders the page as a visitor who arrived to reserve one device
    /// sees it.
    fn opened(leases: &Leases, reserve: &str) -> Json {
        serde_json::to_value(page(&fixture::dnsdhcp(), leases, reserve)).expect("serialize")
    }

    fn rows(body: &Json) -> Vec<Json> {
        body["widget"]["children"][1]["children"][0]["rows"]
            .as_array()
            .expect("rows")
            .clone()
    }

    fn answer(body: &str) -> Json {
        let mut model = fixture::dnsdhcp();
        serde_json::to_value(post(&mut model, &fixture::leases(), &Form::parse(body), ""))
            .expect("serialize")
    }

    #[test]
    fn the_page_watches_rather_than_edits() {
        let body = body(&fixture::leases());
        assert_eq!(body["title"], "DHCP");
        assert_eq!(body["pages"], serde_json::json!([
            {"label": "Leases", "path": ""},
            {"label": "Configuration", "path": "config"}
        ]));
        assert_eq!(body["widget"]["children"][0]["type"], "filter");
        assert_eq!(body["widget"]["children"][1]["title"], "Active leases");
        // No page form: there is nothing on this page to stage.
        assert!(
            !body["widget"]["children"]
                .as_array()
                .expect("children")
                .iter()
                .any(|child| child["type"] == "form")
        );
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

        // A device that offered no name is still called something.
        assert_eq!(rows[1]["cells"][0]["text"], "Device 0e:57");
        // A device with no IPv6 says so rather than showing an empty cell.
        assert_eq!(rows[1]["cells"][2], serde_json::json!({"text": "—", "muted": true}));
        // A lease on another network carries that network's chip.
        assert_eq!(rows[3]["cells"][0]["chip"], "guest");
    }

    #[test]
    fn a_reserved_device_reads_as_reserved_and_offers_nothing_to_open() {
        let rows = rows(&body(&fixture::leases()));
        let nas = &rows[2];
        assert_eq!(nas["cells"][5], serde_json::json!({"text": "reserved", "variant": "info"}));
        assert!(nas.get("drawer").is_none());

        // A dynamic lease offers to become one, prefilled from the device.
        let iphone = &rows[0];
        assert_eq!(iphone["cells"][5], serde_json::json!({"button": "Reserve IP"}));
        let fields = &iphone["drawer"]["children"][0]["fields"];
        assert_eq!(iphone["drawer"]["title"], "Reserve address — toms-iphone");
        assert_eq!(iphone["drawer"]["children"][0]["submit"], "Reserve");
        assert_eq!(
            fields[1],
            serde_json::json!({"type": "field", "name": "_section", "kind": "hidden", "value": ""}),
            "reserving names no section, which is what makes the save create one"
        );
        assert_eq!(fields[2]["value"], "toms-iphone");
        assert_eq!(fields[3]["value"], "42:e6:ad:ff:b7:af");
        assert_eq!(fields[4]["value"], "10.0.0.142");
        // Reserving is not deleting: the panel carries no confirm.
        assert_eq!(iphone["drawer"]["children"].as_array().expect("children").len(), 1);
    }

    #[test]
    fn reserving_writes_the_section_and_the_answer_already_reads_it_as_reserved() {
        let saved = answer("_form=host&_section=&name=toms-iphone&mac=42:e6:ad:ff:b7:af&ip=10.0.0.142");
        assert_eq!(saved["notice"], serde_json::json!({"level": "success", "text": "Address reserved."}));
        assert_eq!(saved["commit"][0]["type"], "host");
        assert_eq!(rows(&saved)[0]["cells"][5], serde_json::json!({"text": "reserved", "variant": "info"}));
    }

    // Arriving with a device named opens that device's panel and no other; a MAC
    // no lease answers to is simply not about anything here.
    #[test]
    fn the_device_the_visitor_came_for_is_already_open() {
        let named = rows(&opened(&fixture::leases(), "42:e6:ad:ff:b7:af"));
        assert_eq!(named[0]["drawer"]["open"], true);
        assert!(named[1]["drawer"].get("open").is_none());

        for reserve in ["", "aa:bb:cc:dd:ee:ff", "nonsense"] {
            for row in rows(&opened(&fixture::leases(), reserve)) {
                assert!(row["drawer"].get("open").is_none(), "{reserve}: {row}");
            }
        }
    }

    // A device whose address is already reserved has no panel to open, so naming
    // it changes nothing.
    #[test]
    fn a_reserved_device_named_in_the_query_still_opens_nothing() {
        for row in rows(&opened(&fixture::leases(), "30:9c:23:5e:88:01")) {
            assert!(row["drawer"]["open"] != true, "{row}");
        }
    }

    #[test]
    fn a_refused_reservation_comes_back_open_on_the_device_it_was_about() {
        let refused = answer("_form=host&_section=&name=toms-iphone&mac=42:e6:ad:ff:b7:af&ip=10.0.0.999");
        assert!(refused.get("commit").is_none());
        assert_eq!(refused["notice"]["level"], "danger");
        let rows = rows(&refused);
        assert_eq!(rows[0]["drawer"]["open"], true);
        assert_eq!(rows[0]["drawer"]["children"][0]["fields"][4]["value"], "10.0.0.999");
        assert!(rows[1]["drawer"].get("open").is_none());
    }

    #[test]
    fn a_body_this_page_did_not_draw_changes_nothing() {
        for body in [
            "_form=record&_section=cname_photos&kind=CNAME&name=a.lan&target=b.lan",
            "_form=host&_section=host_nas&_delete=1",
            "expandhosts=1",
            "",
        ] {
            let answer = answer(body);
            assert!(answer.get("commit").is_none(), "{body}");
            assert_eq!(answer["notice"]["text"], form::UNKNOWN, "{body}");
        }
    }

    #[test]
    fn the_two_silences_are_told_apart() {
        let unread = body(&Leases::read(&Ubus::from_value(Value::Null)));
        let empty = &unread["widget"]["children"][1]["children"][0];
        assert_eq!(empty["type"], "empty");
        assert_eq!(empty["title"], "Live leases aren’t available");

        let served = body(&Leases::read(&Ubus::from_value(
            serde_json::json!({"dhcpLeases": {"leases": []}}),
        )));
        assert_eq!(
            served["widget"]["children"][1]["children"][0]["title"],
            "No device holds an address"
        );
    }
}
