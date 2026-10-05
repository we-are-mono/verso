// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

//! The Zones listing and the policy baseline underneath it.
//!
//! A zone is a name for a set of networks and the policy they share, so the row
//! shows both: what the zone claims, the address ranges that means in practice,
//! and what happens to traffic the rules did not decide. The subnets are derived
//! from the network config rather than the firewall config, which names
//! interfaces and never addresses.
//!
//! Each row opens the zone's own panel beside the listing, the way a rule's row
//! does. A zone is only meaningful against the other zones — what it reaches, what
//! reaches it, which of them a network could have gone to instead — so reading one
//! without losing sight of the rest is worth more than a page of its own.
//!
//! The global defaults sit on the same page because they are the other half of
//! the same answer: they decide traffic no zone claims, and they are what a zone
//! policy falls back to when the zone states none. They are the only things this
//! page posts on their own, one switch at a time.

use verso_plugin::{
    commit, commit_delete, commit_new, ColumnWidth, Envelope, Form, HeadingAct, RowDrawer,
    Settings, SettingsItem, SettingsPill, SettingsToggle, Snapshot, Table, TableCell, TableColumn,
    TableRow, Tone, Widget,
};

use crate::crossings::Crossings;
use crate::fields;
use crate::format;
use crate::model::{Firewall, Zone, CONFIG};
use crate::page;
use crate::rename;
use crate::rule_form::Errors;
use crate::zone_drawer;
use crate::zone_form::ZoneForm;

pub const HEADING: &str = "Zones";

/// NOTE is the one sentence the grid owes its reader, under the last row:
/// which policy answers which question, so the three verdict columns can be
/// read without a legend above them.
const NOTE: &str = "Input decides traffic to the router itself, Output what the router sends \
into the zone, and Forward traffic between the zone's own networks. Reaching another zone is \
decided by the forwardings under Reaches.";

const EMPTY: &str = "No zones yet — every network is governed by the defaults below.";

const REFUSED: &str =
    "Some values aren’t ones the firewall accepts, so nothing was saved. They’re marked below.";

const GONE: &str = "That zone is no longer in the config, so there was nothing to save.";

/// TOGGLE_ROWS name the defaults options plainly and say what each one does.
/// The order is the order they render; the uci name is the switch's form name.
const TOGGLE_ROWS: [(&str, &str, &str); 3] = [
    (
        "drop_invalid",
        "Drop invalid packets",
        "Discard packets that don't match any known connection.",
    ),
    (
        "synflood_protect",
        "SYN-flood protection",
        "Rate-limit half-open connections to blunt basic floods.",
    ),
    (
        "flow_offloading",
        "Software flow offloading",
        "Fast-path established connections for higher throughput.",
    ),
];

/// options names the defaults switches this page renders — the closed set a post
/// to it may name, so the page and its POST handler cannot drift apart.
pub fn options() -> impl Iterator<Item = &'static str> {
    TOGGLE_ROWS.iter().map(|(option, _, _)| *option)
}

/// page renders the Zones listing and the global defaults. Everything both are
/// built from is already in the model — a zone's subnets included, which come
/// from the network config the firewall config only names.
pub fn page(model: &Firewall) -> Envelope {
    frame(model, None, None)
}

/// frame is the Zones page: the listing with the zone `open` names in front of
/// it, the global defaults under it, and the act that adds a zone on the
/// heading line, carrying the blank zone's panel when an address asks for one.
fn frame(model: &Firewall, open: Option<&Open>, blank: Option<RowDrawer>) -> Envelope {
    let children = vec![
        table(model, open),
        Widget::section("Global defaults", "", vec![defaults(model)]),
    ];
    page::envelope(HEADING, Widget::stack(children)).with_act(act(blank))
}

/// page_open is the listing with one zone's panel already in front of the
/// operator. A zone is opened by address rather than by a click the page
/// remembers, so the panel survives a reload and travels in a link — and the
/// listing behind it is the same listing either way. A section this config does
/// not hold opens nothing: a stale link should land on the listing rather than on
/// an error about a zone that is gone.
pub fn page_open(snapshot: &Snapshot, model: &Firewall, query: &Form) -> Envelope {
    let section = query.get(zone_drawer::OPEN).trim().to_string();
    let tab = zone_drawer::reading(query.get(zone_drawer::TAB).trim()).to_string();
    let blank = match section == zone_drawer::NEW {
        true => Some(zone_drawer::blank(
            model,
            &ZoneForm::default(),
            &Crossings::default(),
            &Errors::default(),
            &tab,
        )),
        false => None,
    };
    let open = match blank.is_some() {
        true => None,
        false => snapshot.section(CONFIG, &section).map(|uci| {
            let form = ZoneForm::read(&uci);
            Open {
                reaches: Crossings::read(model, &form.name),
                section,
                tab,
                form,
                errors: Errors::default(),
            }
        }),
    };
    frame(model, open.as_ref(), blank)
}

/// Open is the zone whose panel the address asks for: which one, on which
/// reading, the values its controls start from, and whatever the last submission
/// got wrong about them.
pub struct Open {
    section: String,
    tab: String,
    form: ZoneForm,
    /// Where this zone's traffic may go. They are `config forwarding` sections
    /// rather than options on the zone, so the panel holds them beside the form
    /// rather than inside it — but it posts both at once, and a refused
    /// submission has to come back showing what was typed into either.
    reaches: Crossings,
    errors: Errors,
}

/// save answers an editing panel or a confirmed removal from the listing.
/// A refusal stays with its controls; removal returns the listing without
/// opening the editor or depending on which panel the address happens to name.
pub fn save(snapshot: &Snapshot, model: &mut Firewall, query: &Form, form: &Form) -> Envelope {
    let removal = form.get(fields::REMOVE_FIELD);
    let section = if removal.is_empty() {
        query.get(zone_drawer::OPEN).trim().to_string()
    } else {
        removal.trim().to_string()
    };
    let tab = zone_drawer::reading(query.get(zone_drawer::TAB).trim()).to_string();

    let reaches = Crossings::submitted(form);

    if section == zone_drawer::NEW && removal.is_empty() {
        let stated = ZoneForm::submitted(form);
        let mut errors = stated.validate_named(&model.network_names(), &model.zone_names());
        errors.merge(reaches.validate(&model.zone_names()));
        let answer = opened_blank(model, &stated, &reaches, &errors, &tab);
        if !errors.is_empty() {
            return answer.with_notice(Tone::Danger, REFUSED);
        }
        // The zone and the crossings out of it are written together: a crossing
        // names its zones by name, and the name is settled the moment the zone is
        // created, so there is nothing to wait for.
        let mut ops = vec![commit_new(CONFIG, "zone", stated.values(false))];
        ops.extend(reaches.commits(model, &stated.name));
        return answer
            .with_notice(Tone::Success, "Zone added.")
            .with_commit(ops);
    }

    let Some(index) = model.zones.iter().position(|zone| zone.section == section) else {
        return page(model).with_notice(Tone::Danger, GONE);
    };
    // Removing is the one act here that cannot be undone, which is why it is
    // asked for separately and answers with the listing the zone is leaving
    // rather than with a panel about something that no longer exists.
    if !removal.is_empty() {
        let removed = model.zones.remove(index);
        return page(model)
            .with_notice(Tone::Success, "Zone deleted.")
            .with_commit(vec![commit_delete(CONFIG, &removed.section)]);
    }

    // The submission carries the controls; the section carries what this panel
    // does not draw, and a save has to keep both.
    let stated = snapshot
        .section(CONFIG, &section)
        .map(|uci| ZoneForm::read(&uci))
        .unwrap_or_default();
    let zone = ZoneForm::submitted(form).carrying(&stated);
    let mut errors = zone.validate_named(
        &zone_drawer::allowed_networks(model, &model.zones[index]),
        &model.zone_names(),
    );
    errors.merge(reaches.validate(&model.zone_names()));
    if !errors.is_empty() {
        return opened(model, &section, &zone, &reaches, &errors, &tab)
            .with_notice(Tone::Danger, REFUSED);
    }
    // A new name is carried to every section that names the zone before the
    // crossings are worked out, so they are matched — and any new one written —
    // under the name the zone is about to have.
    let mut ops = vec![commit(CONFIG, &section, zone.values(true))];
    if zone.renamed() {
        ops.extend(rename::rename(model, &zone.named, &zone.name));
    }
    ops.extend(reaches.commits(model, &zone.name));
    opened(model, &section, &zone, &reaches, &errors, &tab)
        .with_notice(Tone::Success, "Zone saved.")
        .with_commit(ops)
}

/// opened is the listing with one zone's panel in front of it, stated from the
/// values given rather than from the config — so a refused submission comes back
/// as what was typed, with the refusals on it.
fn opened(
    model: &Firewall,
    section: &str,
    form: &ZoneForm,
    reaches: &Crossings,
    errors: &Errors,
    tab: &str,
) -> Envelope {
    let open = model
        .zones
        .iter()
        .find(|zone| zone.section == section)
        .map(|_| Open {
            section: section.to_string(),
            tab: tab.to_string(),
            form: form.clone(),
            reaches: reaches.clone(),
            errors: errors.clone(),
        });
    frame(model, open.as_ref(), None)
}

/// opened_blank is the same for a zone that does not exist yet.
fn opened_blank(
    model: &Firewall,
    form: &ZoneForm,
    reaches: &Crossings,
    errors: &Errors,
    tab: &str,
) -> Envelope {
    frame(
        model,
        None,
        Some(zone_drawer::blank(model, form, reaches, errors, tab)),
    )
}

/// act is the page's one forward act. Making a zone is editing one that does
/// not exist yet, so it opens the same panel a row's edit glyph opens rather
/// than a page of its own.
fn act(blank: Option<RowDrawer>) -> HeadingAct {
    HeadingAct {
        label: "Add zone".into(),
        href: zone_drawer::new_href(),
        opens_panel: true,
        drawer: blank,
        ..Default::default()
    }
}

fn columns() -> Vec<TableColumn> {
    [
        ("Zone", "name", ColumnWidth::Name),
        ("Networks", "mono", ColumnWidth::Name),
        ("Reaches", "mono", ColumnWidth::Grow),
        ("Input", "pill", ColumnWidth::Short),
        ("Output", "pill", ColumnWidth::Short),
        ("Forward", "pill", ColumnWidth::Short),
        ("NAT", "text", ColumnWidth::Count),
        ("", "actions", ColumnWidth::Grow),
    ]
    .into_iter()
    .map(|(label, kind, width)| TableColumn {
        label: label.into(),
        kind: kind.into(),
        width,
    })
    .collect()
}

fn table(model: &Firewall, open: Option<&Open>) -> Widget {
    Widget::Table(Table {
        dense: true,
        columns: columns(),
        rows: model
            .zones
            .iter()
            .map(|zone| row(model, zone, open))
            .collect(),
        empty_text: EMPTY.into(),
        note: NOTE.into(),
        ..Default::default()
    })
}

/// row is one zone. Its name and its edit act both lead to the zone's own panel
/// on this listing rather than to a page elsewhere.
fn row(model: &Firewall, zone: &Zone, open: Option<&Open>) -> TableRow {
    // Masquerading is a yes or a nothing, not a verdict, so it is the word for
    // it rather than a pill — the pills in this row all answer the same kind of
    // question, and a fourth one that did not would read as a fourth policy.
    let nat = match zone.masq {
        true => page::text_cell("on"),
        false => TableCell::default(),
    };
    let reaches = model.forwards_from(&zone.name).join(" · ");
    let door = zone_drawer::href(&zone.section, zone_drawer::TRAFFIC);
    let drawer = open
        .filter(|open| open.section == zone.section)
        .map(|open| {
            zone_drawer::drawer(
                model,
                zone,
                &open.form,
                &open.reaches,
                &open.errors,
                &open.tab,
            )
        });
    TableRow {
        depth: 0,
        expanded: Vec::new(),
        id: zone.section.clone(),
        key: String::new(),
        group: None,
        muted: false,
        tags: Vec::new(),
        cells: vec![
            page::name_cell(&zone.name, door.clone()),
            page::mono_cell(&membership(zone)),
            page::mono_cell(&reaches),
            page::pill_cell(&zone.input, format::target_tone(&zone.input)),
            page::pill_cell(&zone.output, format::target_tone(&zone.output)),
            page::pill_cell(&zone.forward, format::target_tone(&zone.forward)),
            nat,
            // The row's trailing affordance reads the way the other two listings'
            // do, so one glyph opens an object everywhere in this plugin.
            page::edit_act_cell(
                door.clone(),
                page::remove_act(
                    &zone.section,
                    zone_drawer::DELETE_TRIGGER,
                    "Delete zone “%s”?",
                    zone_drawer::DELETE_MESSAGE,
                ),
            ),
        ],
        drawer,
        // The row ships an empty frame and fetches this address when it opens;
        // both of the row's doors point at the same place, so either of them
        // opens the panel where it stands rather than reloading the listing.
        panel: door,
    }
}

/// membership is what the zone claims: the logical networks it covers, and any
/// kernel device it claims directly — marked as such, because a raw device is
/// not a network the rest of the config knows by name.
fn membership(zone: &Zone) -> String {
    let mut parts: Vec<String> = zone.networks.clone();
    parts.extend(
        zone.devices
            .iter()
            .map(|device| format!("{device} (device)")),
    );
    parts.join(", ")
}

fn defaults(model: &Firewall) -> Widget {
    let defaults = &model.defaults;
    let mut items = vec![SettingsItem {
        title: "Default policies".into(),
        desc: "What happens to traffic no zone claims.".into(),
        pills: [
            ("in", &defaults.input),
            ("out", &defaults.output),
            ("fwd", &defaults.forward),
        ]
        .into_iter()
        .map(|(direction, policy)| SettingsPill {
            variant: format::tone(format::target_tone(policy)),
            text: format!("{direction}: {policy}"),
        })
        .collect(),
        ..SettingsItem::default()
    }];
    items.extend(
        TOGGLE_ROWS
            .iter()
            .map(|(option, title, desc)| SettingsItem {
                title: (*title).into(),
                desc: (*desc).into(),
                code: (*option).into(),
                toggle: Some(SettingsToggle {
                    name: (*option).into(),
                    on: defaults.state(option),
                }),
                ..SettingsItem::default()
            }),
    );
    Widget::Settings(Settings {
        items,
        ..Default::default()
    })
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::fixture;
    use serde_json::Value;

    fn body() -> Value {
        serde_json::to_value(page(&fixture::firewall())).expect("serialize")
    }

    /// opened is the listing with one zone's panel on it, as the address asks.
    fn opened(section: &str, tab: &str) -> Value {
        let query = Form::parse(&format!("open={section}&tab={tab}"));
        let envelope = page_open(&fixture::snapshot(), &fixture::firewall(), &query);
        serde_json::to_value(&envelope).expect("serialize")
    }

    fn rows(body: &Value) -> Vec<Value> {
        fixture::listing(body)["rows"]
            .as_array()
            .expect("rows")
            .clone()
    }

    fn cells(row: &Value) -> Value {
        row["cells"].clone()
    }

    /// panel_of is the drawer the named row carries, which is the only row that
    /// carries one.
    fn panel_of(body: &Value, section: &str) -> Value {
        rows(body)
            .into_iter()
            .find(|row| row["id"] == section)
            .expect("the row")["drawer"]
            .clone()
    }

    /// reading is the rows of one of the zone's readings, as its section holds
    /// them.
    fn reading(tab: &str) -> Vec<Value> {
        let panel = panel_of(&opened("cfg02dc81", tab), "cfg02dc81");
        let section = fixture::find_with(&panel, &|v| v["type"] == "section").expect("the reading");
        section["children"].as_array().unwrap().clone()
    }

    /// A zone's name is a field like any other, and its help says what a new
    /// name carries to and what it cannot reach.
    #[test]
    fn a_zones_name_is_a_field_that_says_what_follows_it() {
        let rows = reading(zone_drawer::TRAFFIC);
        let at = rows
            .iter()
            .position(|w| w["name"] == "name")
            .expect("the name row");
        assert_eq!(rows[at]["type"], "field");
        assert_ne!(rows[at]["style"], "locked");
        assert_eq!(rows[at]["value"], "lan");
        assert_eq!(rows[at]["key"], "name");
        let help = rows[at]["help"].as_str().unwrap_or_default();
        assert!(help.contains("nftables"), "{help}");
        // The interface is not the actor: the help says what holds, never what
        // "Verso" does.
        assert!(!help.contains("Verso"), "{help}");
        assert_ne!(rows[at + 1]["type"], "callout", "no band under the name");
        let stray = |w: &Value| w["type"] == "properties" || w["type"] == "callout";
        assert!(!rows.iter().any(stray), "no fact rows or bands in a form");
    }

    /// Renaming a zone stages its new name and every section that names it, as
    /// one change — and the crossings the panel posts are matched under the new
    /// name, so the crossing it already has is left alone rather than replaced.
    #[test]
    fn renaming_a_zone_stages_every_reference_with_it() {
        let body = saved(
            "cfg02dc81",
            zone_drawer::TRAFFIC,
            &format!("{}&dest=wan", LAN_TRAFFIC.replace("name=lan", "name=home")),
        );
        assert_eq!(body["notice"]["level"], "success");
        let commit = body["commit"].as_array().expect("commit");
        assert_eq!(commit[0]["section"], "cfg02dc81");
        assert_eq!(commit[0]["values"]["name"], "home");
        let followed: Vec<&str> = commit[1..]
            .iter()
            .map(|op| op["section"].as_str().unwrap_or_default())
            .collect();
        assert_eq!(
            followed,
            vec![
                "allow_isakmp",
                "family_to_printer",
                "nas_snat",
                "https_to_nas",
                "cfg05ad58"
            ],
            "every reference, and no crossing added or removed"
        );
        let panel = panel_of(&body, "cfg02dc81");
        assert_eq!(
            panel["title"], "home",
            "the panel answers under the new name"
        );
    }

    /// A new name answers the questions a new zone's does, and a refused one
    /// writes nothing at all — not the zone, and none of its references.
    #[test]
    fn a_rename_to_a_name_already_taken_is_refused() {
        let body = saved(
            "cfg02dc81",
            zone_drawer::TRAFFIC,
            &LAN_TRAFFIC.replace("name=lan", "name=wan"),
        );
        assert!(body.get("commit").is_none(), "nothing may be written");
        assert_eq!(body["notice"]["level"], "danger");
        let panel = panel_of(&body, "cfg02dc81");
        assert_eq!(
            fixture::control(&panel, "name")["error"],
            "A zone with this name already exists."
        );
    }

    // Reaches reads as two fields and what else names the zone: the crossings
    // it makes, the crossings into it (read-only, the same anatomy, the reason
    // raised on its label), and a titled part saying what would stop matching
    // without it.
    #[test]
    fn reaches_reads_as_fields_and_a_titled_part() {
        let rows = reading(zone_drawer::REACHES);
        let reached = rows
            .iter()
            .find(|w| w["label"] == "Reached by")
            .expect("reached by");
        assert_eq!(reached["type"], "field");
        assert_eq!(reached["style"], "locked");
        assert!(!reached["help"].as_str().unwrap_or_default().is_empty());
        assert!(
            !rows.iter().any(|w| w["type"] == "text"),
            "no loose sentences"
        );
        let named = rows
            .iter()
            .find(|w| w["type"] == "section")
            .expect("the named-elsewhere part");
        assert_eq!(named["title"], "Named elsewhere");
        assert!(!named["sub"].as_str().unwrap_or_default().is_empty());
    }

    #[test]
    fn the_page_carries_the_shared_firewall_frame_and_both_sections() {
        let body = body();
        assert_eq!(body["title"], HEADING);
        assert_eq!(body["width"], "wide");
        assert_eq!(body["pages"][2]["path"], "zones");
        // Making a zone opens the panel that edits one, at the listing's own
        // address: the page's act, on its heading line.
        assert_eq!(body["act"]["label"], "Add zone");
        assert_eq!(body["act"]["href"], "/plugins/firewall/zones?open=new");
        assert_eq!(body["act"]["opens_panel"], true);
        // The grid, then the baseline the grid falls back to — which is a
        // region of its own because it is a second subject, not a second view
        // of this one.
        assert_eq!(body["widget"]["children"][0]["type"], "table");
        assert_eq!(body["widget"]["children"][1]["title"], "Global defaults");
    }

    // A firewall that groups nothing still has a baseline, and the listing says
    // so rather than drawing a grid of column headings over no rows.
    #[test]
    fn a_config_with_no_zones_says_what_that_means() {
        let mut model = fixture::firewall();
        model.zones.clear();
        let body = serde_json::to_value(page(&model)).expect("serialize");
        let table = fixture::listing(&body);
        assert_eq!(table["empty_text"], EMPTY);
        assert!(table["rows"].as_array().expect("rows").is_empty());
        // The way to a first zone stays, on the heading line above the empty
        // grid.
        assert_eq!(body["act"]["href"], "/plugins/firewall/zones?open=new");
    }

    #[test]
    fn a_zone_row_states_membership_policy_and_forwardings() {
        let rows = rows(&body());
        assert_eq!(rows[0]["id"], "cfg02dc81");
        assert_eq!(
            cells(&rows[0]),
            serde_json::json!([
                {"text": "lan", "href": "/plugins/firewall/zones?open=cfg02dc81"},
                {"text": "lan, lan2", "emphasis": true},
                {"text": "wan", "emphasis": true},
                // The three policies firewall4 evaluates separately, in the
                // order it evaluates them.
                {"text": "accept", "variant": "success", "dot": true},
                {"text": "accept", "variant": "success", "dot": true},
                {"text": "accept", "variant": "success", "dot": true},
                // Masquerading is a yes or a nothing, not a fourth verdict.
                {},
                {"actions": [{
                    "icon": "square-pen",
                    "title": "Edit",
                    "href": "/plugins/firewall/zones?open=cfg02dc81"
                }, {
                    "icon": "trash-2",
                    "title": "Delete zone",
                    "name": "_remove",
                    "value": "cfg02dc81",
                    "confirm_title": "Delete zone “%s”?",
                    "confirm": zone_drawer::DELETE_MESSAGE
                }]}
            ])
        );
    }

    /// Every row opens that zone's own panel beside the listing, from the glyph
    /// at the row's trailing edge. The zone's name points at the same address,
    /// and the row ships the empty frame that fetches it.
    #[test]
    fn every_row_opens_its_own_panel_in_place() {
        let body = body();
        let table = fixture::listing(&body);
        assert_eq!(table["columns"][7], serde_json::json!({"kind": "actions"}));
        for row in rows(&body) {
            let section = row["id"].as_str().expect("id");
            let href = format!("/plugins/firewall/zones?open={section}");
            assert_eq!(row["cells"][0]["href"], href);
            assert_eq!(row["cells"][7]["actions"][0]["href"], href);
            assert_eq!(row["panel"], href, "{section} fetches its own panel");
            // A listing with nothing open carries no rendered panel: the frames
            // are empty until one is asked for.
            assert!(row.get("drawer").is_none(), "{section}");
            for cell in row["cells"].as_array().expect("cells") {
                assert!(cell.get("button").is_none(), "{cell}");
            }
        }
    }

    #[test]
    fn an_uplink_states_its_policies_and_shows_its_nat() {
        let wan = &rows(&body())[1];
        assert_eq!(wan["cells"][1]["text"], "wan, wan6");
        // Nothing crosses out of the uplink by a forwarding of its own.
        assert_eq!(
            wan["cells"][2],
            serde_json::json!({"text": "—", "muted": true})
        );
        assert_eq!(
            wan["cells"][3],
            serde_json::json!({"text": "reject", "variant": "danger", "dot": true})
        );
        assert_eq!(wan["cells"][6], serde_json::json!({"text": "on"}));
    }

    #[test]
    fn a_zone_takes_the_defaults_policy_when_it_states_none() {
        let guest = &rows(&body())[2];
        assert_eq!(guest["cells"][0]["text"], "guest");
        assert_eq!(guest["cells"][3]["text"], "reject");
        assert_eq!(guest["cells"][5]["text"], "reject");
    }

    #[test]
    fn a_zone_claiming_a_raw_device_says_so() {
        let tailscale = &rows(&body())[3];
        assert_eq!(tailscale["cells"][1]["text"], "tailscale0 (device)");
    }

    /// Only the row the address names carries a rendered panel. Every other row
    /// ships the empty frame and nothing else, which is what keeps a listing of
    /// forty zones from shipping forty panels nobody asked for.
    #[test]
    fn only_the_zone_the_address_names_is_open() {
        let body = opened("cfg02dc81", zone_drawer::TRAFFIC);
        let panel = panel_of(&body, "cfg02dc81");
        assert_eq!(panel["title"], "lan");
        assert_eq!(panel["open"], true);
        // Its button names what it saves, never a bare "Save".
        assert_eq!(panel["children"][0]["submit"], "Save zone");
        // Closing it leaves the listing, so a reload shows the zones rather than
        // reopening what was just dismissed.
        assert_eq!(panel["closed"], "/plugins/firewall/zones");
        for row in rows(&body) {
            if row["id"] == "cfg02dc81" {
                continue;
            }
            assert!(
                row.get("drawer").is_none(),
                "{} should ship an empty frame",
                row["id"]
            );
        }
    }

    /// A zone the config does not hold opens nothing: a stale link lands on the
    /// listing rather than on an error about a zone that is gone.
    #[test]
    fn a_stale_address_opens_nothing_and_shows_the_listing() {
        let body = opened("no_such_zone", zone_drawer::TRAFFIC);
        assert_eq!(body["title"], HEADING);
        for row in rows(&body) {
            assert!(row.get("drawer").is_none(), "{}", row["id"]);
        }
    }

    /// The page's act opens a blank panel rather than a page. It belongs to the
    /// act because there is no row for a zone that does not exist yet.
    #[test]
    fn the_act_opens_a_blank_zone() {
        let body = opened(zone_drawer::NEW, zone_drawer::TRAFFIC);
        let panel = &body["act"]["drawer"];
        assert_eq!(panel["title"], "New zone");
        assert_eq!(panel["open"], true);
        assert_eq!(panel["closed"], "/plugins/firewall/zones");
        // No row carries a panel: the blank one belongs to the page's act.
        for row in rows(&body) {
            assert!(row.get("drawer").is_none(), "{}", row["id"]);
        }
    }

    /// A reading shows a third of the zone and carries the rest as hidden fields,
    /// so saving from one tab keeps what the other two hold. Without the carriers
    /// every save from Advanced would clear the zone's networks and policies —
    /// absent is how a form says "cleared".
    #[test]
    fn saving_from_one_reading_keeps_what_the_others_hold() {
        let body = opened("cfg02dc81", zone_drawer::ADVANCED);
        let panel = panel_of(&body, "cfg02dc81");
        // Collect every name the panel posts, visible control or carrier alike.
        let mut posted: Vec<String> = Vec::new();
        collect_names(&panel, &mut posted);
        for option in [
            "name", "network", "input", "output", "forward", "masq", "mtu_fix",
        ] {
            assert!(
                posted.iter().any(|name| name == option),
                "the Advanced reading drops {option}, so a save from it would clear it"
            );
        }

        // And the save proves it: a submission carrying what that panel posts comes
        // back as the whole zone, not as the two options the tab showed.
        let mut model = fixture::firewall();
        let form = Form::parse(
            "family=ipv4&log=1&name=lan&input=ACCEPT&output=ACCEPT&forward=ACCEPT\
             &network=lan&network=lan2",
        );
        let query = Form::parse("open=cfg02dc81&tab=advanced");
        let envelope = save(&fixture::snapshot(), &mut model, &query, &form);
        let body = serde_json::to_value(&envelope).expect("serialize");
        let values = &body["commit"][0]["values"];
        assert_eq!(body["notice"]["level"], "success");
        assert_eq!(values["family"], "ipv4");
        assert_eq!(values["log"], "1");
        assert_eq!(
            values["network"],
            serde_json::json!(["lan", "lan2"]),
            "the networks the reading never showed survive the save"
        );
        assert_eq!(values["input"], "ACCEPT");
    }

    /// collect_names walks a widget tree for every form field name it posts.
    fn collect_names(node: &Value, out: &mut Vec<String>) {
        match node {
            Value::Object(map) => {
                if let Some(Value::String(name)) = map.get("name") {
                    if !name.is_empty() {
                        out.push(name.clone());
                    }
                }
                for value in map.values() {
                    collect_names(value, out);
                }
            }
            Value::Array(items) => {
                for item in items {
                    collect_names(item, out);
                }
            }
            _ => {}
        }
    }

    /// saved answers one panel's submission and hands back the envelope as JSON,
    /// which is how every assertion below reads both what was written and what
    /// came back on screen.
    fn saved(section: &str, tab: &str, body: &str) -> Value {
        let mut model = fixture::firewall();
        let query = Form::parse(&format!("open={section}&tab={tab}"));
        let envelope = save(&fixture::snapshot(), &mut model, &query, &Form::parse(body));
        serde_json::to_value(&envelope).expect("serialize")
    }

    /// The whole of the lan zone as its Traffic reading posts it, so a test about
    /// one thing does not accidentally clear another.
    const LAN_TRAFFIC: &str = "enabled=1&name=lan&network=lan&network=lan2&input=ACCEPT\
                               &output=ACCEPT&forward=ACCEPT";

    /// Where a zone's traffic may go is a control now, holding the zones it
    /// reaches and offering the others.
    #[test]
    fn the_reaches_reading_edits_the_crossings_out_of_this_zone() {
        let panel = panel_of(&opened("cfg02dc81", zone_drawer::REACHES), "cfg02dc81");
        let box_ = fixture::control(&panel, "dest");
        assert_eq!(box_["type"], "list");
        assert_eq!(box_["style"], "tokens");
        assert_eq!(box_["label"], "Reaches");
        assert_eq!(
            box_["key"], "forwarding",
            "the chip beside the label names the section each value stands for"
        );
        assert_eq!(box_["items"], serde_json::json!(["wan"]));
        assert_eq!(
            box_["options"],
            serde_json::json!([
                {"value": "wan", "label": "wan"},
                {"value": "guest", "label": "guest"},
                {"value": "tailscale", "label": "tailscale"}
            ])
        );
    }

    /// A save from that reading writes the zone and the crossings together: one
    /// section added for a zone newly named, one removed for a zone taken out.
    #[test]
    fn saving_the_reaches_reading_adds_and_removes_whole_sections() {
        let body = saved(
            "cfg02dc81",
            zone_drawer::REACHES,
            &format!("{LAN_TRAFFIC}&dest=guest"),
        );
        assert_eq!(body["notice"]["level"], "success");
        let commit = body["commit"].as_array().expect("commit");
        assert_eq!(commit[0]["section"], "cfg02dc81", "the zone's own save");
        assert_eq!(
            commit[1],
            serde_json::json!({
                "config": "firewall",
                "section": "cfg05ad58",
                "delete": true
            }),
            "the crossing it no longer names is gone"
        );
        assert_eq!(
            commit[2],
            serde_json::json!({
                "config": "firewall",
                "section": "",
                "type": "forwarding",
                "values": {"src": "lan", "dest": "guest"}
            })
        );
    }

    /// The crossings ride along on the readings that do not show them. Absent is
    /// how a form says "cleared", so a save from Traffic that did not carry them
    /// would delete every crossing out of the zone.
    #[test]
    fn a_save_from_another_reading_keeps_the_crossings() {
        let panel = panel_of(&opened("cfg02dc81", zone_drawer::TRAFFIC), "cfg02dc81");
        let carriers = carried_crossings(&panel);
        assert_eq!(
            carriers,
            vec!["wan".to_string()],
            "the crossing rides as one hidden field per zone"
        );

        let body = saved(
            "cfg02dc81",
            zone_drawer::TRAFFIC,
            &format!("{LAN_TRAFFIC}&dest=wan"),
        );
        let commit = body["commit"].as_array().expect("commit");
        assert_eq!(
            commit.len(),
            1,
            "nothing about the crossings changed: {commit:?}"
        );
    }

    /// And a reading that shows them does not carry them twice: the box is the
    /// control, so there is no hidden copy to disagree with it.
    #[test]
    fn the_reaches_reading_carries_no_hidden_copy_of_its_own_box() {
        let panel = panel_of(&opened("cfg02dc81", zone_drawer::REACHES), "cfg02dc81");
        assert!(carried_crossings(&panel).is_empty());
    }

    /// carried_crossings is every crossing a panel posts as a hidden field, in
    /// the order it posts them. A list option carries one field per value —
    /// space-joined, they would arrive as one zone with a space in its name.
    fn carried_crossings(panel: &Value) -> Vec<String> {
        fn walk(node: &Value, out: &mut Vec<String>) {
            match node {
                Value::Object(map) => {
                    if map.get("kind") == Some(&Value::from("hidden"))
                        && map.get("name") == Some(&Value::from(crate::crossings::DEST))
                    {
                        out.push(map["value"].as_str().unwrap_or_default().to_string());
                    }
                    for value in map.values() {
                        walk(value, out);
                    }
                }
                Value::Array(items) => items.iter().for_each(|item| walk(item, out)),
                _ => {}
            }
        }
        let mut out = Vec::new();
        walk(panel, &mut out);
        out
    }

    /// A zone is created with its crossings: a crossing names its zones by name,
    /// and a new zone's name is settled the moment it is written.
    #[test]
    fn a_new_zone_is_written_with_the_crossings_it_states() {
        let body = saved(
            zone_drawer::NEW,
            zone_drawer::REACHES,
            "name=iot&network=guest&input=REJECT&output=ACCEPT&forward=REJECT&dest=wan",
        );
        assert_eq!(body["notice"]["level"], "success");
        let commit = body["commit"].as_array().expect("commit");
        assert_eq!(commit[0]["type"], "zone");
        assert_eq!(
            commit[1],
            serde_json::json!({
                "config": "firewall",
                "section": "",
                "type": "forwarding",
                "values": {"src": "iot", "dest": "wan"}
            })
        );
    }

    /// A crossing naming a zone this config does not define is one firewall4
    /// skips outright, so it is refused before the write — and the refusal is on
    /// the value that caused it, beside the zone's own if it has one.
    #[test]
    fn a_crossing_to_a_zone_that_does_not_exist_is_refused() {
        let body = saved(
            "cfg02dc81",
            zone_drawer::REACHES,
            &format!("{LAN_TRAFFIC}&dest=nowhere"),
        );
        assert!(body.get("commit").is_none(), "nothing may be written");
        assert_eq!(body["notice"]["level"], "danger");
        let panel = panel_of(&body, "cfg02dc81");
        assert_eq!(
            fixture::control(&panel, "dest")["errors"]["0"],
            "Name a zone this router defines."
        );
    }

    /// Saving from one reading writes the whole zone and all its crossings, not
    /// the third of it that reading shows.
    ///
    /// The same contract the rule panel broke, checked the same way: submit
    /// exactly what each reading posts and the write has to be the write the
    /// zone already was. Reading the carrier lists instead is what let the rule
    /// panel delete every condition on a rule for a year.
    #[test]
    fn saving_from_any_reading_writes_the_whole_zone() {
        let snapshot = fixture::snapshot();
        let model = fixture::firewall();
        let zone = &model.zones[0];
        let intact = ZoneForm::read(
            &snapshot
                .section(CONFIG, &zone.section)
                .expect("the fixture's lan zone"),
        )
        .values(true);
        let crossings = Crossings::read(&model, &zone.name).reaches;

        for tab in [
            zone_drawer::TRAFFIC,
            zone_drawer::REACHES,
            zone_drawer::ADVANCED,
        ] {
            let body = opened(&zone.section, tab);
            let panel = panel_of(&body, &zone.section);
            let mut model = fixture::firewall();
            let query = Form::parse(&format!("open={}&tab={tab}", zone.section));
            let answer = serde_json::to_value(save(
                &snapshot,
                &mut model,
                &query,
                &Form::parse(&fixture::submission(&panel)),
            ))
            .expect("serialize");

            assert_eq!(
                answer["notice"]["level"], "success",
                "the {tab} reading posts something the editor refuses"
            );
            assert_eq!(
                answer["commit"][0]["values"], intact,
                "saving from the {tab} reading rewrites the zone",
            );
            // And the crossings are still exactly the ones it had: a reading that
            // dropped them would delete a `config forwarding` section apiece.
            let ops = answer["commit"].as_array().expect("commit");
            assert_eq!(
                ops.len(),
                1,
                "saving from the {tab} reading changes {} crossing(s) nobody touched: {:?}",
                ops.len() - 1,
                &ops[1..]
            );
            assert_eq!(crossings, vec!["wan".to_string()]);
        }
    }

    /// The three readings are the zone's, each priced with where it stands under
    /// that reading — so the strip answers before a tab is chosen.
    #[test]
    fn the_readings_are_priced_on_the_strip() {
        let panel = panel_of(&opened("cfg02dc81", zone_drawer::TRAFFIC), "cfg02dc81");
        let tabs = panel["tabs"].as_array().expect("tabs");
        assert_eq!(tabs.len(), 3);
        // The verdict as the listing spells it, not as the config happens to: fw4
        // reads ACCEPT and accept alike.
        assert_eq!(tabs[0]["label"], "Traffic");
        assert_eq!(tabs[0]["state"], "accept in");
        assert_eq!(tabs[0]["active"], true);
        assert_eq!(tabs[0]["href"], "/plugins/firewall/zones?open=cfg02dc81");
        assert_eq!(tabs[1]["label"], "Reaches");
        assert_eq!(tabs[1]["state"], "1 zone");
        assert_eq!(
            tabs[1]["href"],
            "/plugins/firewall/zones?open=cfg02dc81&tab=reaches"
        );
        assert_eq!(tabs[2]["state"], "default");

        // A zone that logs its refusals says so on the strip, which is how a reader
        // learns of live state without opening the tab. No fixture zone
        // logs, so the form states it: the strip reads the zone, not the config.
        let logging = serde_json::to_value(zone_drawer::blank(
            &fixture::firewall(),
            &ZoneForm {
                log: true,
                ..ZoneForm::default()
            },
            &Crossings::default(),
            &Errors::default(),
            zone_drawer::TRAFFIC,
        ))
        .expect("serialize");
        assert_eq!(logging["tabs"][2]["state"], "logging");

        // A zone that narrows its family says which one.
        let narrowed = serde_json::to_value(zone_drawer::blank(
            &fixture::firewall(),
            &ZoneForm {
                family: "ipv4".into(),
                ..ZoneForm::default()
            },
            &Crossings::default(),
            &Errors::default(),
            zone_drawer::TRAFFIC,
        ))
        .expect("serialize");
        assert_eq!(narrowed["tabs"][2]["state"], "ipv4");
    }

    /// A panel's heading names the zone; the form carries its settings.
    #[test]
    fn the_panel_has_no_header_subtitle_or_recap() {
        let panel = panel_of(&opened("cfg02dc81", zone_drawer::TRAFFIC), "cfg02dc81");
        assert_eq!(panel["title"], "lan");
        for field in ["chain", "lede", "verdict"] {
            assert!(panel.get(field).is_none(), "obsolete header field: {field}");
        }
        assert_eq!(panel["tabs"].as_array().expect("tabs").len(), 3);
        assert!(!panel["children"].as_array().expect("body").is_empty());
    }

    #[test]
    fn the_defaults_card_reads_the_real_baseline() {
        let body = body();
        let settings = &fixture::section(&body, "Global defaults")["children"][0];
        assert_eq!(settings["type"], "settings");
        assert_eq!(
            settings["items"][0],
            serde_json::json!({
                "title": "Default policies",
                "desc": "What happens to traffic no zone claims.",
                "pills": [
                    {"variant": "danger", "text": "in: reject"},
                    {"variant": "success", "text": "out: accept"},
                    {"variant": "danger", "text": "fwd: reject"}
                ]
            })
        );
        assert_eq!(
            settings["items"][1],
            serde_json::json!({
                "title": "Drop invalid packets",
                "desc": "Discard packets that don't match any known connection.",
                "code": "drop_invalid",
                "toggle": {"name": "drop_invalid"}
            })
        );
        assert_eq!(
            settings["items"][2]["toggle"],
            serde_json::json!({"name": "synflood_protect", "on": true})
        );
        assert_eq!(
            settings["items"][3]["toggle"],
            serde_json::json!({"name": "flow_offloading"})
        );
    }
}
