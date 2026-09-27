// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

//! The DHCP Configuration face: what each network hands out, and to whom.
//!
//! A pool is surfaced as a card of its own rather than buried inside an
//! interface editor, because "does this network hand out addresses, and which
//! ones" is a question about the network, not about the interface's protocol.
//! The card states the span in real addresses — `start` and `limit` are offsets,
//! and an offset answers nothing — and the IPv6 announcement, which odhcpd
//! serves rather than dnsmasq, folds behind the card's seam.
//!
//! A network whose interface commits to no address of its own is the uplink:
//! this router is the client there, so the card offers the one thing that can
//! honestly be said about it.

use verso_plugin::{Envelope, Form, SettingsItem, Tone, Widget};

use crate::format::{self, Subnet};
use crate::hosts;
use crate::live::Leases;
use crate::model::{self, Dnsdhcp, Options};
use crate::options::{self, inverted, on, read, value, Changes, Check, Row};
use crate::page;

/// HANDOUT is the one thing every pool card says, uplink included.
const HANDOUT: [Row; 1] = [inverted(
    "ignore",
    "Hand out addresses",
    "This router assigns addresses on this network.",
)];

/// LEASE is what a pool with addresses to give also says.
const LEASE: [Row; 1] = [value(
    "leasetime",
    "Lease length",
    "How long an address stays with a device after last contact.",
    Check::Leasetime,
)];

/// POOL_FOLDED is the IPv6 announcement and the rare v4 overrides. The three
/// `ra`/`dhcpv6` values are odhcpd's mode words rather than switches, so the
/// card states them and does not pretend they are on/off.
const POOL_FOLDED: [Row; 6] = [
    read(
        "ra",
        "Announce the network",
        "Router advertisements let devices configure IPv6 themselves.",
    ),
    read(
        "dhcpv6",
        "Managed IPv6 addresses",
        "Hand out DHCPv6 addresses alongside SLAAC.",
    ),
    options::on_by_default(
        "ra_slaac",
        "SLAAC",
        "Devices derive their own address from the announced prefix.",
    ),
    on(
        "force",
        "Force DHCP",
        "Answer even if another DHCP server is detected here.",
    ),
    read(
        "netmask",
        "Netmask override",
        "Blank derives it from the network.",
    ),
    read(
        "dhcp_option",
        "Extra DHCP options",
        "Options handed to clients, e.g. 6,10.0.0.30 for a different DNS.",
    ),
];

/// DAEMON is the handful of daemon-wide switches that change how every network
/// is served.
const DAEMON: [Row; 3] = [
    on(
        "authoritative",
        "Authoritative",
        "This is the only DHCP server here; answer without hesitation.",
    ),
    on(
        "sequential_ip",
        "Sequential addresses",
        "Allocate in order instead of hashing by MAC.",
    ),
    on(
        "logdhcp",
        "Log every lease",
        "Write lease events to the system log.",
    ),
];

const DAEMON_FOLDED: [Row; 3] = [
    value(
        "dhcpleasemax",
        "Max simultaneous leases",
        "Cap across all networks.",
        Check::Number(u32::MAX),
    ),
    on(
        "readethers",
        "Read /etc/ethers",
        "Load classic MAC→IP pairs from the file.",
    ),
    read("leasefile", "Lease file", "Where live leases are recorded."),
];

const NETWORKS_SUB: &str = "One card per network — `config dhcp`. The range is shown as real \
addresses, never start/limit offsets; the IPv6 announcement (served by odhcpd) folds below.";

const EXTRAS_SUB: &str = "The daemon-wide switches — `config dnsmasq` and friends. Everything \
is here; almost nothing needs touching.";

const UPLINK_META: &str = "upstream — this router is the client here";

/// page renders the configuration face.
pub fn page(model: &Dnsdhcp, leases: &Leases) -> Envelope {
    render(model, leases, "")
}

/// render draws the page. A refused page-form value belongs to the form itself,
/// since a settings row is a name, a description and a control with nowhere to
/// put a message; the reservations are edited on pages of their own, so their
/// refusals never reach here.
fn render(model: &Dnsdhcp, leases: &Leases, refused: &str) -> Envelope {
    page::dhcp(Widget::stack(vec![
        page::filter("Filter everything — network, device, IP, option…"),
        page::page_form(
            refused,
            vec![
                Widget::section(
                    "Networks",
                    NETWORKS_SUB,
                    vec![Widget::grid(
                        2,
                        model
                            .networks
                            .iter()
                            .map(|pool| card(model, leases, pool))
                            .collect(),
                    )],
                ),
                hosts::section(model, leases),
                Widget::section("Rarely needed", EXTRAS_SUB, vec![extras(model)]),
            ],
        ),
    ]))
}

/// card is one pool. Everything an operator asks about a network every week is
/// on its face; the rest is behind the seam.
fn card(model: &Dnsdhcp, leases: &Leases, pool: &Options) -> Widget {
    let name = pool_name(pool);
    let prefix = prefix_of(pool);
    let subnet = subnet_of(model, pool);
    let mut items = options::items(&HANDOUT, pool, &prefix);
    let Some(subnet) = subnet else {
        return Widget::Settings {
            condensed: false,
            style: "card".into(),
            title: name.into(),
            meta: UPLINK_META.into(),
            items,
            seam: None,
        };
    };
    items.push(SettingsItem {
        title: "Address range".into(),
        desc: "Devices get an address from this span.".into(),
        code: "start · limit".into(),
        value: format::range_label(Some(subnet), pool.scalar("start"), pool.scalar("limit")),
        ..SettingsItem::default()
    });
    items.extend(options::items(&LEASE, pool, &prefix));
    Widget::Settings {
        condensed: false,
        style: "card".into(),
        title: name.into(),
        meta: format!(
            "{} · {}",
            subnet.cidr(),
            format::devices_label(leases.on(&subnet))
        ),
        items,
        seam: options::fold(
            "IPv6 & advanced — ",
            options::items(&POOL_FOLDED, pool, &prefix),
        ),
    }
}

/// extras is the daemon-wide block. The last two rows are sections rather than
/// options: relaying and network boot are whole features, and stating whether
/// they are configured is all this page claims about them.
fn extras(model: &Dnsdhcp) -> Widget {
    let mut folded = options::items(&DAEMON_FOLDED, &model.daemon, "");
    folded.push(present(
        "Relay to another server",
        "Forward DHCP requests elsewhere instead of answering.",
        "config relay",
        model.relays,
    ));
    folded.push(present(
        "Network boot (PXE/TFTP)",
        "Serve boot files to machines that start from the network.",
        "config boot",
        model.boots,
    ));
    Widget::Settings {
        condensed: false,
        style: String::new(),
        title: String::new(),
        meta: String::new(),
        items: options::items(&DAEMON, &model.daemon, ""),
        seam: options::fold("", folded),
    }
}

/// present states whether a whole feature is configured, without offering to
/// configure it here.
fn present(title: &str, desc: &str, code: &str, count: usize) -> SettingsItem {
    SettingsItem {
        title: title.into(),
        desc: desc.into(),
        code: code.into(),
        value: match count {
            0 => "not configured".into(),
            1 => "1 configured".into(),
            count => format!("{count} configured"),
        },
        ..SettingsItem::default()
    }
}

/// pool_name is the network a pool serves — the interface it names, which is
/// what an operator calls it.
fn pool_name(pool: &Options) -> &str {
    match pool.scalar("interface") {
        "" => pool.section.as_str(),
        interface => interface,
    }
}

/// prefix_of namespaces one pool's form names, so the same catalogue draws every
/// card without one network's switch answering for another's.
fn prefix_of(pool: &Options) -> String {
    format!("{}.", pool.section)
}

/// subnet_of is the address space a pool hands out of. An interface that learns
/// its address at runtime commits to none, and that is the uplink.
fn subnet_of(model: &Dnsdhcp, pool: &Options) -> Option<Subnet> {
    let interface = model.interface(pool_name(pool))?;
    Subnet::read(&interface.ipaddr, &interface.netmask)
}

/// post answers the configuration page's submission. The only thing this page
/// draws that posts is its own form of cards and daemon switches; a reservation
/// posts to its own page, so this page never sees one.
pub fn post(model: &mut Dnsdhcp, leases: &Leases, form: &Form) -> Envelope {
    save(model, leases, form)
}

/// save reads the page form back. A card only reads back the rows it drew: a
/// control the uplink card never rendered must not be read as the operator
/// having emptied it.
fn save(model: &mut Dnsdhcp, leases: &Leases, form: &Form) -> Envelope {
    let mut changes = Changes::default();
    let subnets: Vec<bool> = model
        .networks
        .iter()
        .map(|pool| subnet_of(model, pool).is_some())
        .collect();
    for (pool, has_addresses) in model.networks.iter_mut().zip(subnets) {
        let prefix = prefix_of(pool);
        options::save(&HANDOUT, pool, "dhcp", &prefix, form, &mut changes);
        if has_addresses {
            options::save(&LEASE, pool, "dhcp", &prefix, form, &mut changes);
            options::save(&POOL_FOLDED, pool, "dhcp", &prefix, form, &mut changes);
        }
    }
    for rows in [&DAEMON[..], &DAEMON_FOLDED[..]] {
        options::save(
            rows,
            &mut model.daemon,
            model::DAEMON_TYPE,
            "",
            form,
            &mut changes,
        );
    }

    if let Some(refusal) = changes.refusal() {
        let refused = format!("{refusal}, so nothing was saved.");
        return render(model, leases, &refused).with_notice(Tone::Danger, &refused);
    }
    let answer = render(model, leases, "");
    if changes.is_empty() {
        return answer;
    }
    answer
        .with_notice(Tone::Success, "DHCP settings saved.")
        .with_commit(changes.commit(model::CONFIG))
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::fixture;
    use serde_json::Value as Json;

    fn body() -> Json {
        serde_json::to_value(page(&fixture::dnsdhcp(), &fixture::leases())).expect("serialize")
    }

    fn sections(body: &Json) -> Vec<Json> {
        body["widget"]["children"][1]["fields"]
            .as_array()
            .expect("sections")
            .clone()
    }

    fn cards(body: &Json) -> Vec<Json> {
        sections(body)[0]["children"][0]["children"]
            .as_array()
            .expect("cards")
            .clone()
    }

    fn answer(body: &str) -> Json {
        let mut model = fixture::dnsdhcp();
        serde_json::to_value(post(&mut model, &fixture::leases(), &Form::parse(body)))
            .expect("serialize")
    }

    /// STANDING is the page resubmitted exactly as it renders — the body every
    /// case varies one field of, so a change is the only thing under test.
    const STANDING: &str = "lan.ignore=1&lan.leasetime=12h&lan.ra_slaac=1&\
guest.ignore=1&guest.leasetime=2h&guest.ra_slaac=1&authoritative=1&readethers=1";

    #[test]
    fn the_page_is_one_form_over_every_card() {
        let body = body();
        assert_eq!(body["title"], "DHCP");
        assert!(body.get("pages").is_none(), "DNS & DHCP has no subpages");
        let form = &body["widget"]["children"][1];
        assert_eq!(form["style"], "page");
        assert!(form.get("submit").is_none());
        let sections = sections(&body);
        let titles: Vec<&str> = sections
            .iter()
            .map(|section| section["title"].as_str().unwrap_or(""))
            .collect();
        assert_eq!(titles, vec!["Networks", "Reservations", "Rarely needed"]);
    }

    #[test]
    fn a_pool_states_real_addresses_and_who_is_on_it() {
        let cards = cards(&body());
        assert_eq!(cards[0]["title"], "lan");
        assert_eq!(cards[0]["style"], "card");
        assert_eq!(cards[0]["meta"], "10.0.0.0/24 · 3 devices right now");
        assert_eq!(
            cards[0]["items"][1],
            serde_json::json!({
                "title": "Address range",
                "desc": "Devices get an address from this span.",
                "code": "start · limit",
                "value": "10.0.0.100 – 10.0.0.249"
            })
        );
        assert_eq!(cards[0]["items"][2]["value"], "12h");
        assert_eq!(cards[0]["items"][2]["name"], "lan.leasetime");

        // A pool that states no offsets still shows the span dnsmasq would use.
        assert_eq!(cards[1]["title"], "guest");
        assert_eq!(cards[1]["meta"], "10.0.20.0/24 · 1 device right now");
        assert_eq!(cards[1]["items"][1]["value"], "10.0.20.100 – 10.0.20.249");
    }

    #[test]
    fn handing_out_addresses_is_the_inverse_of_ignoring_the_network() {
        let cards = cards(&body());
        // lan states no `ignore`, so it does hand out addresses.
        assert_eq!(
            cards[0]["items"][0],
            serde_json::json!({
                "title": "Hand out addresses",
                "desc": "This router assigns addresses on this network.",
                "code": "ignore",
                "toggle": {"name": "lan.ignore", "on": true}
            })
        );
        // wan sets `ignore 1`, so its switch is off.
        assert_eq!(
            cards[2]["items"][0]["toggle"],
            serde_json::json!({"name": "wan.ignore"})
        );
    }

    #[test]
    fn the_uplink_card_says_only_what_is_true_of_it() {
        let wan = &cards(&body())[2];
        assert_eq!(wan["title"], "wan");
        assert_eq!(wan["meta"], UPLINK_META);
        assert_eq!(wan["items"].as_array().expect("items").len(), 1);
        assert!(wan.get("seam").is_none());
    }

    #[test]
    fn the_ipv6_announcement_folds_behind_the_cards_seam() {
        let seam = &cards(&body())[0]["seam"];
        assert_eq!(seam["summary"], "IPv6 & advanced — 6 more options");
        assert_eq!(seam["items"][0]["code"], "ra");
        assert_eq!(seam["items"][0]["value"], "server");
        assert!(
            seam["items"][0].get("name").is_none(),
            "a mode word is stated, not toggled"
        );
        assert_eq!(
            seam["items"][2]["toggle"],
            serde_json::json!({"name": "lan.ra_slaac", "on": true})
        );
        assert_eq!(seam["items"][5]["value"], "6,10.0.0.30");
    }

    #[test]
    fn the_daemon_wide_block_states_the_features_it_does_not_edit() {
        let extras = &sections(&body())[2]["children"][0];
        assert_eq!(
            extras["items"][0]["toggle"],
            serde_json::json!({"name": "authoritative", "on": true})
        );
        assert_eq!(extras["seam"]["summary"], "5 more options");
        assert_eq!(extras["seam"]["items"][2]["value"], "/tmp/dhcp.leases");
        assert_eq!(extras["seam"]["items"][3]["code"], "config relay");
        assert_eq!(extras["seam"]["items"][3]["value"], "not configured");
        assert_eq!(extras["seam"]["items"][4]["value"], "1 configured");
    }

    #[test]
    fn the_page_form_writes_only_what_changed_across_every_card() {
        assert!(answer(STANDING).get("commit").is_none());

        let saved = answer(
            "lan.leasetime=24h&lan.ra_slaac=1&guest.ignore=1&guest.leasetime=2h&\
             guest.ra_slaac=1&guest.force=1&authoritative=1&readethers=1&logdhcp=1",
        );
        assert_eq!(
            saved["commit"],
            serde_json::json!([
                {"config": "dhcp", "section": "dnsmasq_main", "values": {"logdhcp": "1"}},
                {"config": "dhcp", "section": "guest", "values": {"force": "1"}},
                // The switch that stopped posting turned the option it inverts on.
                {"config": "dhcp", "section": "lan", "values": {"ignore": "1", "leasetime": "24h"}}
            ])
        );
        assert_eq!(saved["notice"]["level"], "success");
    }

    #[test]
    fn a_control_the_uplink_card_never_drew_is_not_read_as_a_change() {
        // wan carries a lease time the card does not render, and the submission
        // therefore says nothing about it — which must not clear it.
        let saved = answer(&format!("{STANDING}&logdhcp=1"));
        let values = &saved["commit"][0]["values"];
        assert_eq!(values, &serde_json::json!({"logdhcp": "1"}));
        assert!(
            saved["commit"]
                .as_array()
                .expect("commit")
                .iter()
                .all(|op| op["section"] != "wan"),
            "the uplink card drew nothing that changed: {}",
            saved["commit"]
        );
    }

    #[test]
    fn a_lease_length_the_daemon_would_not_read_saves_nothing_and_says_so() {
        let refused = answer("lan.ignore=1&lan.leasetime=forever&lan.ra_slaac=1&guest.ignore=1&guest.leasetime=2h&guest.ra_slaac=1&authoritative=1&readethers=1");
        assert!(refused.get("commit").is_none());
        let said =
            "“Lease length” has to be a length such as 12h, 30m, or infinite, so nothing was saved.";
        assert_eq!(refused["notice"]["text"], said);
        // The refusal rides the form as well as the notice: that is what makes
        // the answer a 422, which keeps the operator on the page.
        assert_eq!(refused["widget"]["children"][1]["error"], said);
        assert_eq!(cards(&refused)[0]["items"][2]["value"], "forever");
    }

    /// The reservations listing sits on this page, but each row leads to a page
    /// of its own — the listing draws no drawer and carries a tail that adds one.
    #[test]
    fn the_reservations_listing_leads_to_pages_of_its_own() {
        let body = body();
        let table = &sections(&body)[1]["children"][0];
        assert_eq!(table["add_label"], "New reservation");
        assert_eq!(
            table["add_href"],
            "/plugins/dnsdhcp/config/reservations/new"
        );
        for row in table["rows"].as_array().expect("rows") {
            assert!(row.get("drawer").is_none(), "{row}");
            let section = row["id"].as_str().expect("id");
            assert_eq!(
                row["cells"]
                    .as_array()
                    .expect("cells")
                    .last()
                    .expect("cell"),
                &serde_json::json!({
                    "text": "Edit",
                    "href": format!("/plugins/dnsdhcp/config/reservations/{section}")
                })
            );
        }
    }
}
