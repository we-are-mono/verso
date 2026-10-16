// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

//! The Rules listing: every filtering rule under the traffic path it decides.
//!
//! Rules are grouped by the chain firewall4 evaluates them in, because that is
//! the only order that explains the outcome: a packet enters exactly one chain
//! and the first matching rule there decides it. Within a group, config order is
//! evaluation order and is shown untouched; the groups themselves follow the
//! order their first rule appears in, so the page still reads as the file.
//!
//! Because position is meaning here, the rows drag: the listing declares the uci
//! config its rows are sections of, and the shell stages the dropped sequence as
//! a `uci order` on it. A drag stays inside one group, since moving a rule
//! between chains would change which packets it sees rather than when.

use verso_plugin::{
    commit, commit_delete, commit_new, ColumnWidth, Envelope, Form, HeadingAct, RowDrawer,
    Snapshot, Table, TableCell, TableChip, TableColumn, TableGroup, TablePart, TableRow, Tone,
    Widget, RESHAPE,
};

use crate::counters::Counters;
use crate::fields;
use crate::format;
use crate::model::{Firewall, Rule, CONFIG};
use crate::page;
use crate::rule_drawer;
use crate::rule_form::{Errors, RuleForm, UNCHOSEN};
use crate::zone_drawer;

/// HEADING is what this page is about, in the words the canvas uses: each of
/// this plugin's pages names its own subject rather than wearing the plugin's
/// name and the tab it is on.
pub const HEADING: &str = "Firewall rules";

/// frame wraps the listing in the firewall's page frame. The heading is the
/// page's whole introduction: the toolbar sits straight under it, and a lede
/// between them would say in prose what the listing says in rows.
/// frame is the Rules page: the listing, and the act that adds to it on the
/// heading line, carrying the blank rule's panel when an address asks for one.
fn frame(listing: Widget, blank: Option<RowDrawer>) -> Envelope {
    page::envelope(HEADING, listing).with_act(act(blank))
}

/// NOTE is the one sentence the listing owes its reader, under the last row
/// rather than above the first: what the order means, and the two ways to
/// change it. Above the grid it would be a paragraph to get past; below it, it
/// is a caption for something already understood.
const NOTE: &str = "Within a chain the first matching rule wins — drag the grip to reorder, or \
add to a chain from its header.";

const EMPTY: &str = "No rules yet — every packet is decided by its zone's default policy.";

/// page renders the Rules listing: the bar that narrows it, the grid itself,
/// and the sentence about evaluation order under the last row. A rule can be
/// added from the bar, which makes one anywhere, or from a chain's own header,
/// which makes one already in that chain.
pub fn page(model: &Firewall, counters: &Counters) -> Envelope {
    frame(table(model, counters, None), None)
}

/// page_open is the listing with one rule's panel already in front of the
/// operator. A rule is opened by address rather than by a click the page
/// remembers, so the panel survives a reload and travels in a link — and the
/// listing behind it is the same listing either way. A section this config does
/// not hold opens nothing: a stale link should land on the listing rather than
/// on an error about a rule that is gone.
pub fn page_open(
    snapshot: &Snapshot,
    model: &Firewall,
    counters: &Counters,
    query: &Form,
) -> Envelope {
    let section = query.get(rule_drawer::OPEN).trim().to_string();
    let tab = rule_drawer::reading(query.get(rule_drawer::TAB).trim()).to_string();
    // A rule that does not exist yet opens from the bar's own act, because there
    // is no row for it to open from; one that does opens from its row.
    let blank = match section == rule_drawer::NEW {
        true => Some(rule_drawer::blank(
            model,
            &seeded(model, query),
            &Errors::default(),
            &tab,
        )),
        false => None,
    };
    let open = match blank.is_some() {
        true => None,
        false => snapshot.section(CONFIG, &section).map(|uci| Open {
            section,
            tab,
            form: RuleForm::read(&uci),
            errors: Errors::default(),
        }),
    };
    frame(table(model, counters, open.as_ref()), blank)
}

/// seeded is the blank rule as the address asked for it. A lane's own New rule
/// button knows the chain it adds to, so the path arrives in the query; a zone
/// this config does not name is ignored rather than trusted into a select.
fn seeded(model: &Firewall, query: &Form) -> RuleForm {
    let mut form = RuleForm::default();
    let zones = model.zone_names();
    for (field, value) in [("src", &mut form.src), ("dest", &mut form.dest)] {
        let asked = query.get(field).trim().to_string();
        if asked == "*" || zones.contains(&asked) {
            *value = asked;
        }
    }
    // Nothing said where the traffic comes from: the page's own act, which
    // knows no lane. It asks rather than starting on the router. A lane that
    // names only where traffic goes is the router's own output, so it keeps
    // the router as its source.
    if form.src.is_empty() && form.dest.is_empty() {
        form.src = UNCHOSEN.to_string();
    }
    form
}

/// Open is the rule whose panel the address asks for: which one, on which
/// reading, the values its controls start from, and whatever the last submission
/// got wrong about them.
pub struct Open {
    section: String,
    tab: String,
    form: RuleForm,
    errors: Errors,
}

/// save answers an editing panel or a confirmed removal from the listing.
/// A refusal stays with its controls; removal returns the listing without
/// opening the editor or depending on which panel the address happens to name.
pub fn save(
    snapshot: &Snapshot,
    model: &mut Firewall,
    counters: &Counters,
    query: &Form,
    form: &Form,
) -> Envelope {
    let removal = form.get(fields::REMOVE_FIELD);
    let section = if removal.is_empty() {
        query.get(rule_drawer::OPEN).trim().to_string()
    } else {
        removal.trim().to_string()
    };
    let tab = rule_drawer::reading(query.get(rule_drawer::TAB).trim()).to_string();
    let stated = RuleForm::submitted(form);

    // A tab press is the shell's reshape: the panel again, on the tab asked
    // for, drawn from what is on screen. Nothing is saved, and nothing is
    // refused — half a rule is still being written.
    if form.get("_action") == RESHAPE && removal.is_empty() {
        return match section == rule_drawer::NEW {
            true => opened_blank(model, counters, &stated, &Errors::default(), &tab),
            false => opened(
                snapshot,
                model,
                counters,
                &section,
                &stated,
                &Errors::default(),
                &tab,
            ),
        };
    }

    let errors = stated.validate(&model.zone_names());

    if section == rule_drawer::NEW && removal.is_empty() {
        let answer = opened_blank(model, counters, &stated, &errors, &tab);
        if !errors.is_empty() {
            return answer.with_notice(Tone::Danger, REFUSED);
        }
        return answer
            .with_notice(Tone::Success, "Rule added.")
            .with_commit(vec![commit_new(CONFIG, "rule", stated.values(false))]);
    }

    let Some(index) = model.rules.iter().position(|rule| rule.section == section) else {
        return page(model, counters).with_notice(Tone::Danger, GONE);
    };
    // Removing is the one act here that cannot be undone, which is why it is
    // asked for separately and answers with the listing the rule is leaving
    // rather than with a panel about something that no longer exists.
    if !removal.is_empty() {
        let removed = model.rules.remove(index);
        return page(model, counters)
            .with_notice(Tone::Success, "Rule deleted.")
            .with_commit(vec![commit_delete(CONFIG, &removed.section)]);
    }
    let answer = opened(snapshot, model, counters, &section, &stated, &errors, &tab);
    if !errors.is_empty() {
        return answer.with_notice(Tone::Danger, REFUSED);
    }
    answer
        .with_notice(Tone::Success, "Rule saved.")
        .with_commit(vec![commit(CONFIG, &section, stated.values(true))])
}

const REFUSED: &str =
    "Some values aren’t ones the firewall accepts, so nothing was saved. They’re marked below.";

const GONE: &str = "That rule is no longer in the config, so there was nothing to save.";

/// opened is the listing with one rule's panel in front of it, stated from the
/// values given rather than from the config — so a refused submission comes back
/// as what was typed, with the refusals on it.
fn opened(
    snapshot: &Snapshot,
    model: &Firewall,
    counters: &Counters,
    section: &str,
    form: &RuleForm,
    errors: &Errors,
    tab: &str,
) -> Envelope {
    let open = snapshot.section(CONFIG, section).map(|_| Open {
        section: section.to_string(),
        tab: tab.to_string(),
        form: form.clone(),
        errors: errors.clone(),
    });
    frame(table(model, counters, open.as_ref()), None)
}

/// opened_blank is the same for a rule that does not exist yet.
fn opened_blank(
    model: &Firewall,
    counters: &Counters,
    form: &RuleForm,
    errors: &Errors,
    tab: &str,
) -> Envelope {
    frame(
        table(model, counters, None),
        Some(rule_drawer::blank(model, form, errors, tab)),
    )
}

/// act is the page's one forward act. Making a rule is editing one that does
/// not exist yet, so it opens the same panel a row's edit glyph opens rather
/// than a page of its own. A still listing is scrolled and found in with the
/// browser's own find, so nothing narrows it.
fn act(blank: Option<RowDrawer>) -> HeadingAct {
    HeadingAct {
        label: "Add rule".into(),
        href: rule_drawer::new_href("", ""),
        opens_panel: true,
        drawer: blank,
        ..Default::default()
    }
}

fn columns() -> Vec<TableColumn> {
    [
        ("", "reorder", ColumnWidth::Grow),
        ("#", "text", ColumnWidth::Mark),
        ("Name", "name", ColumnWidth::Name),
        ("From", "endpoint", ColumnWidth::Word),
        ("To", "endpoint", ColumnWidth::Word),
        ("Proto", "keyword", ColumnWidth::Short),
        ("Match", "mono", ColumnWidth::Grow),
        ("Action", "pill", ColumnWidth::Short),
        ("Hits", "num", ColumnWidth::Count),
        ("", "actions", ColumnWidth::Short),
    ]
    .into_iter()
    .map(|(label, kind, width)| TableColumn {
        label: label.into(),
        kind: kind.into(),
        width,
    })
    .collect()
}

fn table(model: &Firewall, counters: &Counters, open: Option<&Open>) -> Widget {
    let mut rows: Vec<TableRow> = Vec::with_capacity(model.rules.len());
    // The number in the leading column is the rule's place in the whole
    // evaluation, not in its lane: a packet crosses the chains in order, so the
    // count that explains an outcome is the one that keeps running.
    let mut position = 0;
    for lane in lanes(&model.rules) {
        let (from, to) = format::group_path(&lane.chain, lane.shared_dest.as_deref());
        for (index, rule) in lane.rules.iter().enumerate() {
            position += 1;
            let mut row = row(model, rule, counters, position, open);
            if index == 0 {
                row.group = Some(TableGroup {
                    // The chain is the lane's handle for the drag, never its
                    // name on the band: the band says where the traffic comes
                    // from and where it goes, which is what the chain means.
                    key: lane.chain.clone(),
                    label: from.clone(),
                    to: to.clone(),
                    chain: String::new(),
                    add_label: format!(
                        "Add rule to {}",
                        format::group_label(&lane.chain, lane.shared_dest.as_deref())
                    ),
                    add_text: "Add rule".into(),
                    add_href: add_href(&lane),
                    add_panel: true,
                    close: close(model, &lane),
                });
            }
            rows.push(row);
        }
    }
    Widget::Table(Table {
        dense: true,
        reorder_config: "firewall".into(),
        reorder_label: "Rule order".into(),
        columns: columns(),
        rows,
        empty_text: EMPTY.into(),
        note: NOTE.into(),
        ..Default::default()
    })
}

/// row is one rule. Its name and its edit act both lead to the rule's own panel
/// on this listing rather than to a page elsewhere: the evaluation order is the
/// context a rule is read in, and opening it beside the order keeps it there.
fn row(
    model: &Firewall,
    rule: &Rule,
    counters: &Counters,
    position: u32,
    open: Option<&Open>,
) -> TableRow {
    let door = rule_drawer::href(&rule.section, rule_drawer::MATCH);
    let drawer = open
        .filter(|open| open.section == rule.section)
        .map(|open| rule_drawer::drawer(model, rule, &open.form, &open.errors, &open.tab));
    TableRow {
        depth: 0,
        expanded: Vec::new(),
        id: rule.section.clone(),
        key: String::new(),
        group: None,
        // A rule that is switched off still decides nothing, so it reads at the
        // secondary step — present, exact, and plainly not in force.
        muted: !rule.enabled,
        tags: Vec::new(),
        cells: vec![
            TableCell::default(),
            page::index_cell(position),
            // The name is the row's subject: what the log calls this rule,
            // and where it is edited.
            page::name_cell(&rule.name, door.clone()),
            page::endpoint_cell(&rule.src_ips, &rule.src),
            page::endpoint_cell(&rule.dest_ips, &rule.dest),
            page::text_cell(&format::protocol(&rule.proto, &rule.family)),
            page::mono_cell(&format::match_summary(
                &rule.dest_ports,
                &rule.icmp_types,
                &rule.limit,
            )),
            page::pill_cell(&rule.target, format::target_tone(&rule.target)),
            page::hits_cell(counters.packets(&rule.chain, &rule.identity)),
            page::acts_cell(
                &rule.section,
                rule.enabled,
                door.clone(),
                page::remove_act(
                    &rule.section, "Delete rule", "Delete rule “%s”?",
                    "When applied, traffic it allowed will be decided by whatever rule or zone policy comes next.",
                    "Keep rule",
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

/// Lane is one evaluation chain's run of rules, plus the destination zone the
/// whole run shares — the label says where forwarded traffic goes only when
/// every rule in the lane agrees on it.
struct Lane<'a> {
    chain: String,
    shared_dest: Option<String>,
    rules: Vec<&'a Rule>,
}

/// lanes buckets rules by evaluation chain, keeping the chains in the order
/// their first rule appears and the rules within a chain in config order.
fn lanes(rules: &[Rule]) -> Vec<Lane<'_>> {
    let mut lanes: Vec<Lane> = Vec::new();
    for rule in rules {
        match lanes.iter_mut().find(|lane| lane.chain == rule.chain) {
            Some(lane) => lane.rules.push(rule),
            None => lanes.push(Lane {
                chain: rule.chain.clone(),
                shared_dest: None,
                rules: vec![rule],
            }),
        }
    }
    for lane in lanes.iter_mut() {
        lane.shared_dest = shared_dest(&lane.rules);
    }
    lanes
}

/// add_href aims the lane's tail affordance at the editor with the lane's own
/// path pre-chosen: an input lane seeds the source zone, a forward lane both
/// ends (when every rule shares the destination), an output lane the
/// destination. Zone names are uci section names — url-safe by construction.
fn add_href(lane: &Lane) -> String {
    if let Some(zone) = lane.chain.strip_prefix("input_") {
        return rule_drawer::new_href(zone, "");
    }
    if let Some(zone) = lane.chain.strip_prefix("forward_") {
        return match &lane.shared_dest {
            Some(dest) => rule_drawer::new_href(zone, dest),
            None => rule_drawer::new_href(zone, ""),
        };
    }
    if let Some(zone) = lane.chain.strip_prefix("output_") {
        return rule_drawer::new_href("", zone);
    }
    // The zone-less lanes are traffic from or to any zone.
    match lane.chain.as_str() {
        "input" => rule_drawer::new_href("*", ""),
        "forward" => rule_drawer::new_href("*", lane.shared_dest.as_deref().unwrap_or("")),
        "output" => rule_drawer::new_href("", "*"),
        _ => rule_drawer::new_href("", ""),
    }
}

/// close is a lane's last word: what firewall4 does with traffic none of the
/// lane's rules caught, as it ends the chain. A zone's input and output chains
/// end on that zone's policy for the direction. Its forward chain accepts into
/// the zones it forwards to, and ends on its forward policy only for traffic
/// leaving into the zone itself (`jump reject_to_wan` matches the zone's own
/// devices); traffic into any other zone falls back to the forward hook,
/// which ends on the global forward policy. A rule for any zone runs in every
/// zone's chain, so each zone's own policy follows it. A lane that decides
/// nothing (marking, helpers, untracked traffic) and a zone the config does
/// not have close on nothing.
fn close(model: &Firewall, lane: &Lane) -> Vec<TablePart> {
    let otherwise = TablePart::Words("Otherwise".into());
    if matches!(lane.chain.as_str(), "input" | "output" | "forward") {
        return vec![
            otherwise,
            TablePart::Door {
                words: "each zone’s own policy".into(),
                href: page::zones_href(),
            },
        ];
    }
    let Some((direction, name)) = lane.chain.split_once('_') else {
        return Vec::new();
    };
    let Some(zone) = model.zones.iter().find(|zone| zone.name == name) else {
        return Vec::new();
    };
    let door = |verdict: &str, words: &str, href: String| {
        [
            verdict_chip(verdict),
            TablePart::Door {
                words: words.into(),
                href,
            },
        ]
    };
    let zone_href = || zone_drawer::href(&zone.section, zone_drawer::TRAFFIC);
    let global = &model.defaults.forward;
    let crossings: Vec<&str> = model
        .forwardings
        .iter()
        .filter(|f| f.enabled && f.src == zone.name)
        .map(|f| f.dest.as_str())
        .collect();
    let mut parts = vec![otherwise];
    match direction {
        "input" => parts.extend(door(&zone.input, "the zone’s input policy", zone_href())),
        "output" => parts.extend(door(&zone.output, "the zone’s output policy", zone_href())),
        "forward" => match lane.shared_dest.as_deref() {
            Some(dest) if dest == zone.name => {
                parts.extend(door(
                    &zone.forward,
                    "the zone’s forward policy",
                    zone_href(),
                ));
            }
            Some(dest) if crossings.contains(&dest) => parts.extend(door(
                "accept",
                "the zone’s forwarding",
                zone_drawer::href(&zone.section, zone_drawer::REACHES),
            )),
            Some(_) => parts.extend(door(
                global,
                "the global forward policy",
                page::settings_href(),
            )),
            None => {
                if !crossings.is_empty() {
                    parts.extend([
                        verdict_chip("accept"),
                        TablePart::Words("where the zone forwards".into()),
                    ]);
                }
                parts.extend(door(&zone.forward, "within the zone", zone_href()));
                parts.extend(door(global, "anywhere else", page::settings_href()));
            }
        },
        _ => return Vec::new(),
    }
    parts
}

/// verdict_chip is a verdict as every rule's Action column states it: the
/// word, lowercase as firewall4's targets read, in its tone, led by its square.
fn verdict_chip(verdict: &str) -> TablePart {
    let verdict = verdict.to_lowercase();
    TablePart::Chip(TableChip {
        tone: format::target_tone(&verdict).into(),
        label: verdict,
        dot: true,
        ..TableChip::default()
    })
}

fn shared_dest(rules: &[&Rule]) -> Option<String> {
    let first = rules.first()?;
    if first.dest.is_empty() || first.dest == "*" {
        return None;
    }
    rules
        .iter()
        .all(|rule| rule.dest == first.dest)
        .then(|| first.dest.clone())
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::fixture;
    use serde_json::Value;

    fn body(counters: &Counters) -> Value {
        let envelope = page(&fixture::firewall(), counters);
        serde_json::to_value(&envelope).expect("serialize")
    }

    fn table_of(body: &Value) -> Value {
        fixture::listing(body)
    }

    fn lane(chain: &str, shared_dest: Option<&str>) -> Lane<'static> {
        Lane {
            chain: chain.into(),
            shared_dest: shared_dest.map(str::to_string),
            rules: Vec::new(),
        }
    }

    fn closing(model: &Firewall, chain: &str, shared_dest: Option<&str>) -> Value {
        serde_json::to_value(close(model, &lane(chain, shared_dest))).expect("serialize")
    }

    fn verdict(label: &str, tone: &str) -> Value {
        serde_json::json!({"chip": {"icon": "", "label": label, "tone": tone, "dot": true}})
    }

    // A lane ends where firewall4 ends the chain: traffic none of its rules
    // caught meets the zone's own policy for that direction, and the line says
    // which and leads to where it is set.
    #[test]
    fn a_zone_lane_closes_on_the_zones_policy() {
        let model = fixture::firewall();
        assert_eq!(
            closing(&model, "input_wan", None),
            serde_json::json!([
                {"words": "Otherwise"},
                verdict("reject", "danger"),
                {"words": "the zone’s input policy", "href": "/plugins/firewall/zones?open=cfg03dc81"}
            ])
        );
        assert_eq!(
            closing(&model, "output_lan", None),
            serde_json::json!([
                {"words": "Otherwise"},
                verdict("accept", "success"),
                {"words": "the zone’s output policy", "href": "/plugins/firewall/zones?open=cfg02dc81"}
            ])
        );
    }

    // firewall4 ends a zone's forward chain on its forward policy only for
    // traffic leaving into the same zone (`jump reject_to_wan` matches the
    // zone's own devices). Traffic into a zone the zone forwards to is
    // accepted by the forwarding; into any other zone it falls back to the
    // forward hook, which ends on the global forward policy. A forwarding that
    // is switched off crosses nothing, so guest → wan (off) does not count.
    #[test]
    fn a_forward_lane_closes_on_where_its_traffic_goes() {
        let model = fixture::firewall();
        assert_eq!(
            closing(&model, "forward_wan", Some("wan")),
            serde_json::json!([
                {"words": "Otherwise"},
                verdict("reject", "danger"),
                {"words": "the zone’s forward policy", "href": "/plugins/firewall/zones?open=cfg03dc81"}
            ])
        );
        assert_eq!(
            closing(&model, "forward_lan", Some("wan")),
            serde_json::json!([
                {"words": "Otherwise"},
                verdict("accept", "success"),
                {"words": "the zone’s forwarding", "href": "/plugins/firewall/zones?open=cfg02dc81&tab=reaches"}
            ])
        );
        for dest in ["lan", "wan"] {
            assert_eq!(
                closing(&model, "forward_guest", Some(dest)),
                serde_json::json!([
                    {"words": "Otherwise"},
                    verdict("reject", "danger"),
                    {"words": "the global forward policy", "href": "/plugins/firewall/settings"}
                ]),
                "guest → {dest}"
            );
        }
        // A lane going several places says each: where the zone forwards,
        // within the zone, and anywhere else.
        assert_eq!(
            closing(&model, "forward_lan", None),
            serde_json::json!([
                {"words": "Otherwise"},
                verdict("accept", "success"),
                {"words": "where the zone forwards"},
                verdict("accept", "success"),
                {"words": "within the zone", "href": "/plugins/firewall/zones?open=cfg02dc81"},
                verdict("reject", "danger"),
                {"words": "anywhere else", "href": "/plugins/firewall/settings"}
            ])
        );
        assert_eq!(
            closing(&model, "forward_wan", None),
            serde_json::json!([
                {"words": "Otherwise"},
                verdict("reject", "danger"),
                {"words": "within the zone", "href": "/plugins/firewall/zones?open=cfg03dc81"},
                verdict("reject", "danger"),
                {"words": "anywhere else", "href": "/plugins/firewall/settings"}
            ])
        );
    }

    // A rule for any zone runs in every zone's chain, so what follows it is
    // each zone's own policy; a lane that decides nothing (marking, helpers,
    // untracked) and a zone the config does not have close on nothing.
    #[test]
    fn other_lanes_close_on_what_is_true_of_them() {
        let model = fixture::firewall();
        assert_eq!(
            closing(&model, "input", None),
            serde_json::json!([
                {"words": "Otherwise"},
                {"words": "each zone’s own policy", "href": "/plugins/firewall/zones"}
            ])
        );
        for chain in [
            "mangle_prerouting",
            "notrack_wan",
            "helper_lan",
            "input_nowhere",
        ] {
            assert_eq!(
                closing(&model, chain, None),
                serde_json::json!([]),
                "{chain}"
            );
        }
    }

    #[test]
    fn every_lane_on_the_page_closes() {
        let body = body(&Counters::default());
        let rows = table_of(&body)["rows"].as_array().expect("rows").clone();
        let wan = rows
            .iter()
            .find(|row| row["group"]["key"] == "input_wan")
            .expect("the wan → router lane");
        assert_eq!(wan["group"]["close"][0]["words"], "Otherwise");
    }

    #[test]
    fn the_page_carries_the_shared_firewall_frame() {
        let body = body(&Counters::default());
        assert_eq!(body["title"], HEADING);
        assert_eq!(body["width"], "wide");
        assert_eq!(
            body["pages"],
            serde_json::json!([
                {"label": "Rules", "path": ""},
                {"label": "Port forwards", "path": "port-forwards"},
                {"label": "Zones", "path": "zones"},
                {"label": "Settings", "path": "settings"},
                {"label": "Activity", "path": "activity"}
            ])
        );
        // The page's one act stands on its heading line; a still listing is
        // scrolled and found in with the browser's own find, so nothing
        // narrows it.
        assert_eq!(
            body["act"],
            // Making a rule is editing one that does not exist yet, so the act
            // opens the same panel a row's edit glyph opens — the href is the
            // fallback a browser with no script follows.
            serde_json::json!({
                "label": "Add rule",
                "href": "/plugins/firewall/?open=new",
                "opens_panel": true
            })
        );
        // The grid is the page itself, not a titled region inside it.
        assert_eq!(body["widget"]["type"], "table");
        // What evaluation order means is a caption under the last row, not a
        // paragraph to get past before the first.
        assert_eq!(table_of(&body)["note"], NOTE);
        assert!(body.get("commit").is_none());
        assert!(body.get("notice").is_none());
    }

    /// opened renders the listing with one rule's panel in front of it, the way
    /// a visit carrying ?open= does.
    fn opened(section: &str, tab: &str) -> Value {
        let query = Form::parse(&format!("open={section}&tab={tab}"));
        let envelope = page_open(
            &fixture::snapshot(),
            &fixture::firewall(),
            &Counters::default(),
            &query,
        );
        serde_json::to_value(&envelope).expect("serialize")
    }

    /// drawer_of finds the one row carrying a panel.
    fn drawer_of(body: &Value) -> Value {
        table_of(body)["rows"]
            .as_array()
            .expect("rows")
            .iter()
            .find_map(|row| row.get("drawer").cloned())
            .expect("an open panel")
    }

    // An address that names a rule opens that rule beside the listing — exactly
    // one panel, on the rule it names, with the listing behind it unchanged.
    #[test]
    fn an_address_opens_the_rule_it_names() {
        let body = opened("allow_ping", "");
        let rows = table_of(&body)["rows"].as_array().expect("rows").clone();
        let open: Vec<&str> = rows
            .iter()
            .filter(|row| row.get("drawer").is_some())
            .map(|row| row["id"].as_str().unwrap_or_default())
            .collect();
        assert_eq!(open, ["allow_ping"], "exactly the named rule opens");
        // The listing itself is the same listing: every rule still on screen.
        assert_eq!(rows.len(), 9);
        let drawer = drawer_of(&body);
        assert_eq!(drawer["title"], "Allow-Ping");
        assert_eq!(drawer["open"], true);
        // The header is the rule's name and nothing else. The chain, the verdict,
        // the rule's place in the evaluation and its hit count are all on the row
        // this panel opened from, a few pixels to the left and among the rules
        // that give them their meaning — restated here they were four true things
        // to read past before reaching a control.
        for furniture in ["chain", "verdict", "lede"] {
            assert!(
                drawer.get(furniture).is_none(),
                "the panel's header still carries {furniture}: {drawer}"
            );
        }
    }

    // The strip names each reading, and the address says which one is in
    // force.
    #[test]
    fn the_panel_offers_three_readings_by_name() {
        let drawer = drawer_of(&opened("allow_ping", rule_drawer::ACTION));
        assert_eq!(
            drawer["tabs"],
            serde_json::json!([
                {
                    "label": "Match",
                    "href": "/plugins/firewall/?open=allow_ping"
                },
                {
                    "label": "Action",
                    "href": "/plugins/firewall/?open=allow_ping&tab=action",
                    "active": true
                },
                {
                    "label": "When",
                    "href": "/plugins/firewall/?open=allow_ping&tab=when"
                }
            ])
        );
    }

    // A rule this config does not hold opens nothing. A stale link lands on the
    // listing rather than on an error about a rule that is gone.
    #[test]
    fn an_address_naming_no_rule_opens_nothing() {
        let body = opened("a_rule_that_left", "");
        assert!(
            table_of(&body)["rows"]
                .as_array()
                .expect("rows")
                .iter()
                .all(|row| row.get("drawer").is_none()),
            "a section this config does not hold opens no panel"
        );
    }

    // The Match reading is one flat set of conditions, not a path section and a
    // catalogue of extras: every row is the same shape. None of the path's own
    // rows ends in the glyph that clears it — they are the shape every rule has,
    // and a zone is widened by choosing Anywhere, not by taking the row away. The
    // glyph belongs to a condition somebody added, and the catalogue draws it.
    #[test]
    fn the_match_reading_is_one_flat_set_of_conditions() {
        let drawer = drawer_of(&opened("allow_ping", ""));
        let section = &drawer["children"][0]["fields"][0];
        let rows = section["children"].as_array().expect("rows");
        // The carriers for the readings this one does not show are fields too,
        // and invisible by design; the rows are the fields someone can see. The
        // path's two zones stand in one row, joined by their word, and are read
        // through it.
        let shapes: Vec<(&str, &str, &str)> = rows
            .iter()
            .flat_map(|w| match w["type"].as_str() {
                Some("grid") => w["children"].as_array().unwrap().iter().collect(),
                _ => vec![w],
            })
            .filter(|w| w["type"] == "field" && w["kind"] != "hidden")
            .map(|w| {
                (
                    w["name"].as_str().unwrap_or_default(),
                    w["key"].as_str().unwrap_or_default(),
                    w["remove"].as_str().unwrap_or_default(),
                )
            })
            .collect();
        assert_eq!(
            shapes,
            [
                ("name", "name", ""),
                ("src", "src", ""),
                ("dest", "dest", ""),
                ("family", "family", ""),
            ],
            "the path is rows of its own, none of them removable; everything else a \
             rule matches on comes from the catalogue below them"
        );
        // The set carries the one control that adds to it, ruled off from the
        // rows above — and the catalogue widget is gone from this reading.
        // Everything else a rule can match on is the shell's catalogue: it shows
        // the conditions this rule carries and keeps the rest behind its own
        // picker, which is what gives "Add a condition" something to open.
        let catalogue = rows
            .iter()
            .find(|w| w["type"] == "conditions")
            .expect("the Match reading carries the condition catalogue");
        // Every entry says where it sits in the picker and what it takes. The
        // picker sorts the catalogue into its headings and states a value example
        // at each line's trailing edge, so an entry missing either is a line with
        // a hole in it or, worse, one heading's group of exactly one.
        let items = catalogue["items"].as_array().expect("catalogue entries");
        assert!(items.len() > 8, "the catalogue is the whole of fw4's reach");
        for item in items {
            let key = item["key"].as_str().unwrap_or_default();
            assert!(
                item["group"].as_str().is_some_and(|g| !g.is_empty()),
                "{key} sits under no heading in the picker"
            );
            assert!(
                item["hint"].as_str().is_some_and(|h| !h.is_empty()),
                "{key} says nothing about the value it takes"
            );
        }
        // The config this panel writes closes the reading, so nothing is saved
        // without the file it becomes being on screen.
        assert_eq!(rows.last().expect("a tail")["type"], "code");
        // The commit row carries the one verb and nothing about applying: the
        // panel saves into the stage and closes, and the bar behind it is where
        // applying happens and where its cost is stated.
        assert_eq!(drawer["children"][0]["submit"], "Save rule");
        assert!(drawer["children"][0].get("note").is_none());
    }

    // Making a rule is editing one that does not exist yet, so it opens the same
    // panel — from the page's act, because there is no row for it to open from.
    #[test]
    fn the_act_opens_a_blank_rule_seeded_with_the_lane_it_was_asked_from() {
        let body = opened(rule_drawer::NEW, "");
        let drawer = &body["act"]["drawer"];
        assert_eq!(drawer["title"], "New rule");
        assert_eq!(drawer["open"], true);
        // Closing it leaves the address it opened from, so a reload shows the
        // listing rather than reopening what was just dismissed.
        assert_eq!(drawer["closed"], "/plugins/firewall/");
        // No row carries a panel: the blank one belongs to the page's act.
        assert!(
            table_of(&body)["rows"]
                .as_array()
                .expect("rows")
                .iter()
                .all(|row| row.get("drawer").is_none()),
            "a blank rule opens from the page's act, never from a row"
        );
        // A blank rule uses the same title-only header as an existing rule.
        assert!(drawer.get("verdict").is_none());
        assert!(drawer.get("lede").is_none());
        assert_eq!(drawer["children"][0]["submit"], "Add rule");
    }

    // A tab press posts what is on screen as a reshape: the panel comes back on
    // the tab asked for, drawn from those values, and nothing is saved, refused
    // or announced — a rule is one object across its tabs, new or not.
    #[test]
    fn a_tab_press_carries_what_was_typed_and_saves_nothing() {
        let typed = "_action=reshape&name=Allow-Test&src=wan&proto=tcp&target=ACCEPT&enabled=1";
        let pressed = |query: &str| {
            let mut model = fixture::firewall();
            serde_json::to_value(save(
                &fixture::snapshot(),
                &mut model,
                &Counters::default(),
                &Form::parse(query),
                &Form::parse(typed),
            ))
            .expect("serialize")
        };
        let name_in = |drawer: &Value| {
            fixture::find_with(&drawer["children"], &|w| w["name"] == "name")
                .map(|w| w["value"].clone())
                .expect("the name rides along")
        };
        for (query, drawer_at) in [
            ("open=new&tab=action", "act"),
            ("open=new", "act"),
            ("open=allow_ping&tab=when", "row"),
        ] {
            let body = pressed(query);
            assert!(body.get("commit").is_none(), "{query}: a tab press staged");
            assert!(body.get("notice").is_none(), "{query}: a tab press spoke");
            let drawer = match drawer_at {
                "act" => body["act"]["drawer"].clone(),
                _ => drawer_of(&body),
            };
            assert_eq!(drawer["tabs_keep_edits"], true, "{query}");
            assert_eq!(
                name_in(&drawer),
                "Allow-Test",
                "{query}: the typed name was dropped"
            );
        }
    }

    // A rule made from the page's own act knows nothing of where its traffic
    // comes from, so it asks rather than starting on the router: Add rule is
    // refused, with the reason on the pick, until one is chosen. A lane that
    // knows its path seeds it, the router included.
    #[test]
    fn a_rule_from_nowhere_asks_where_its_traffic_comes_from() {
        let seeded_src = |query: &str| seeded(&fixture::firewall(), &Form::parse(query)).src;
        assert_eq!(seeded_src("open=new"), UNCHOSEN);
        assert_eq!(seeded_src("open=new&src=wan"), "wan");
        assert_eq!(seeded_src("open=new&src=*"), "*");
        assert_eq!(
            seeded_src("open=new&dest=wan"),
            "",
            "an output lane starts at the router"
        );

        let mut model = fixture::firewall();
        let body = serde_json::to_value(save(
            &fixture::snapshot(),
            &mut model,
            &Counters::default(),
            &Form::parse("open=new"),
            &Form::parse("name=Open-SSH&src=%3F&proto=tcp&target=ACCEPT&enabled=1"),
        ))
        .expect("serialize");
        assert!(
            body.get("commit").is_none(),
            "a rule from nowhere was staged"
        );
        let src = fixture::find_with(&body["act"]["drawer"], &|w| {
            w["type"] == "field" && w["name"] == "src"
        })
        .expect("the source pick");
        // The shell sets the pick's label before it: "Coming from: Choose one first."
        assert_eq!(src["error"], "Choose one first.");
    }

    // The lane's own New rule button knows the chain it adds to, so the blank
    // rule starts on that path rather than empty.
    #[test]
    fn a_lane_seeds_the_blank_rule_with_its_own_path() {
        let query = Form::parse("open=new&src=wan");
        let body = serde_json::to_value(page_open(
            &fixture::snapshot(),
            &fixture::firewall(),
            &Counters::default(),
            &query,
        ))
        .expect("serialize");
        let fields = &body["act"]["drawer"]["children"][0]["fields"][0];
        let src = fixture::find_with(fields, &|w| w["type"] == "field" && w["name"] == "src")
            .expect("the source pick");
        assert_eq!(src["value"], "wan");
    }

    // A tab nobody published is a hand-typed or stale address, not an error:    // A tab nobody published is a hand-typed or stale address, not an error:
    // the panel answers with the rule's first reading.
    #[test]
    fn an_unknown_reading_falls_back_to_the_first_one() {
        let drawer = drawer_of(&opened("allow_ping", "whatever"));
        assert_eq!(drawer["tabs"][0]["active"], true);
        assert_eq!(drawer["tabs"][0]["label"], "Match");
    }

    #[test]
    fn rules_are_grouped_into_the_chains_that_evaluate_them() {
        let body = body(&Counters::default());
        let rows = table_of(&body)["rows"].as_array().expect("rows").clone();
        let groups: Vec<(String, String, String)> = rows
            .iter()
            .filter_map(|row| row.get("group"))
            .map(|group| {
                (
                    group["label"].as_str().unwrap_or_default().to_string(),
                    group["to"].as_str().unwrap_or_default().to_string(),
                    group["key"].as_str().unwrap_or_default().to_string(),
                )
            })
            .collect();
        // The band names the lane by its two ends; the chain is the drag's
        // handle, not a word on the band.
        assert_eq!(
            groups,
            vec![
                (
                    "WAN".to_string(),
                    "Router".to_string(),
                    "input_wan".to_string()
                ),
                (
                    "Guest".to_string(),
                    "Router".to_string(),
                    "input_guest".to_string()
                ),
                (
                    "WAN".to_string(),
                    "Forwarded traffic".to_string(),
                    "forward_wan".to_string()
                ),
                (
                    "Guest".to_string(),
                    "LAN".to_string(),
                    "forward_guest".to_string()
                ),
            ]
        );
        for row in &rows {
            if let Some(group) = row.get("group") {
                assert!(
                    group.get("chain").is_none(),
                    "the chain is not drawn on the band"
                );
            }
        }
        // Every rule is in exactly one lane, and a lane's run is contiguous.
        assert_eq!(rows.len(), 9);
    }

    #[test]
    fn a_row_states_the_path_the_match_and_the_verdict() {
        let body = body(&Counters::default());
        let rows = table_of(&body)["rows"].as_array().expect("rows").clone();
        assert_eq!(
            rows[1],
            serde_json::json!({
                "id": "allow_ping",
                // The row ships an empty frame and fetches its panel from here
                // when someone opens it — the same address both its doors point
                // at, so either opens it where it stands.
                "panel": "/plugins/firewall/?open=allow_ping",
                "cells": [
                    {},
                    // Its place in the whole evaluation, not in its lane.
                    {"text": "2", "muted": true},
                    {"text": "Allow-Ping", "href": "/plugins/firewall/?open=allow_ping"},
                    {"endpoints": [{"kind": "zone", "label": "wan"}]},
                    {"endpoints": [{"kind": "router", "label": "router"}]},
                    {"text": "icmp · v4"},
                    {"text": "echo-request", "emphasis": true},
                    {"text": "accept", "variant": "success", "dot": true},
                    {"text": "—", "muted": true},
                    // The row's own acts: turn it off, and open it. The power
                    // act's words name the state it would move to; its glyph is
                    // the plain power mark on a rule that is on, and the
                    // struck one on a rule that is off.
                    {"actions": [
                        {
                            "icon": "power",
                            "title": "Disable",
                            "name": "allow_ping",
                            "value": "off"
                        },
                        {
                            "icon": "square-pen",
                            "title": "Edit",
                            "href": "/plugins/firewall/?open=allow_ping"
                        },
                        // Only the delete action asks for confirmation.
                        {
                            "icon": "trash-2",
                            "title": "Delete rule",
                            "name": "_remove",
                            "value": "allow_ping",
                            "confirm_title": "Delete rule “%s”?",
                            "confirm": "When applied, traffic it allowed will be decided by whatever rule or zone policy comes next.",
                            "confirm_cancel": "Keep rule"
                        }
                    ]}
                ]
            })
        );
    }

    // A firewall with nothing in it is not an empty grid of column headings: the
    // listing says what the absence means, and the shell draws it as one row.
    #[test]
    fn a_config_with_no_rules_says_what_that_means() {
        let mut model = fixture::firewall();
        model.rules.clear();
        let body = serde_json::to_value(page(&model, &Counters::default())).expect("serialize");
        let table = table_of(&body);
        assert_eq!(table["empty_text"], EMPTY);
        assert!(table["rows"].as_array().expect("rows").is_empty());
    }

    #[test]
    fn every_row_leads_to_its_own_editor_and_each_lane_tail_leads_to_a_seeded_blank() {
        let body = body(&Counters::default());
        assert!(
            body.get("action").is_none(),
            "the listing's own bar carries the act, not the page masthead"
        );
        // Every lane's band carries its own add, aimed at the editor with the
        // lane's path pre-chosen — one rule made anywhere from the bar, one made
        // already in a chain from that chain's head.
        let table = table_of(&body);
        let groups: Vec<&serde_json::Value> = table["rows"]
            .as_array()
            .expect("rows")
            .iter()
            .filter_map(|row| row.get("group").filter(|group| !group.is_null()))
            .collect();
        assert!(!groups.is_empty(), "the fixture rules form lanes");
        for group in &groups {
            // The add is a glyph, and its words name the lane it adds to.
            let label = group["add_label"].as_str().expect("add_label");
            assert!(label.starts_with("Add rule to "), "{label}");
            let href = group["add_href"].as_str().expect("add_href");
            assert!(
                href.starts_with("/plugins/firewall/?open=new"),
                "tail affordance must open the blank panel: {href}"
            );
        }
        let input_lane = groups
            .iter()
            .find(|group| group["key"] == "input_wan")
            .expect("the WAN input lane");
        assert_eq!(input_lane["add_label"], "Add rule to WAN → Router");
        assert_eq!(
            input_lane["add_href"], "/plugins/firewall/?open=new&src=wan",
            "an input lane seeds the editor's source zone"
        );
        let table = table_of(&body);
        assert_eq!(
            table["columns"][9],
            serde_json::json!({"kind": "actions", "width": "short"})
        );
        for row in table["rows"].as_array().expect("rows") {
            let section = row["id"].as_str().expect("id");
            // The name and the glyph at the row's trailing edge name the same
            // place, and the glyph is the way in. That place is this listing with
            // the rule's panel open beside it — a rule is read in the order it
            // sits in, so opening one never leaves the order.
            let href = format!("/plugins/firewall/?open={section}");
            assert_eq!(row["cells"][2]["href"], href);
            assert_eq!(row["cells"][9]["actions"][1]["href"], href);
            // And the act before it flips the rule where it stands, posting the
            // section it belongs to.
            assert_eq!(row["cells"][9]["actions"][0]["name"], section);
        }
    }

    // Only the rules drag. A port forward and a zone are matched by their own
    // fields, not by where they sit, so a grip on those listings would offer a
    // move that means nothing.
    #[test]
    fn only_the_rules_listing_carries_the_drag() {
        let model = fixture::firewall();
        let counters = Counters::default();
        let rules = table_of(&body(&counters));
        assert_eq!(rules["reorder_config"], "firewall");
        assert_eq!(rules["columns"][0], serde_json::json!({"kind": "reorder"}));
        for row in rules["rows"].as_array().expect("rows") {
            assert_eq!(row["cells"][0], serde_json::json!({}));
            assert!(!row["id"].as_str().unwrap_or_default().is_empty());
        }

        for (envelope, listing) in [
            (crate::redirects::page(&model, &counters), "port forwards"),
            (crate::zones::page(&model), "zones"),
        ] {
            let page = serde_json::to_value(&envelope).expect("serialize");
            let table = fixture::listing(&page);
            assert!(table.get("reorder_config").is_none(), "{listing}");
            assert_ne!(table["columns"][0]["kind"], "reorder", "{listing}");
        }
    }

    #[test]
    fn an_address_match_shows_the_addresses_not_the_zone() {
        let body = body(&Counters::default());
        let rows = table_of(&body)["rows"].as_array().expect("rows").clone();
        let printer = rows
            .iter()
            .find(|row| row["id"] == "family_to_printer")
            .expect("row");
        assert_eq!(
            printer["cells"][4]["endpoints"],
            serde_json::json!([{"kind": "device", "label": "10.0.31.213"}])
        );
        assert_eq!(printer["cells"][5]["text"], "tcp/udp");
        assert_eq!(
            printer["cells"][6],
            serde_json::json!({"text": "—", "muted": true})
        );
    }

    #[test]
    fn a_disabled_rule_keeps_its_row_and_offers_to_come_back() {
        let body = body(&Counters::default());
        let rows = table_of(&body)["rows"].as_array().expect("rows").clone();
        let telnet = rows
            .iter()
            .find(|row| row["id"] == "block_telnet")
            .expect("row");
        // Drop says nothing at all, so it wears no hue: the pill is the plain
        // word for what happened, led by the packet — hollow, for it is gone.
        assert_eq!(
            telnet["cells"][7],
            serde_json::json!({"text": "drop", "variant": "neutral", "dot": true})
        );
        // A rule that is off offers to come back on, and says so.
        assert_eq!(
            telnet["cells"][9]["actions"][0],
            serde_json::json!({
                "icon": "power-off",
                "title": "Enable",
                "name": "block_telnet",
                "value": "on"
            })
        );
        // A rule switched off decides nothing, so the whole row reads at the
        // secondary step — present, exact, and plainly not in force.
        assert_eq!(telnet["muted"], true);
    }

    #[test]
    fn a_long_icmp_set_and_a_rate_limit_read_as_a_summary() {
        let body = body(&Counters::default());
        let rows = table_of(&body)["rows"].as_array().expect("rows").clone();
        let icmpv6 = rows
            .iter()
            .find(|row| row["cells"][2]["text"] == "Allow-ICMPv6-Input")
            .expect("row");
        assert_eq!(icmpv6["cells"][5]["text"], "icmpv6");
        assert_eq!(icmpv6["cells"][6]["text"], "11 types · ≤1000/s");
    }

    #[test]
    fn brokered_counters_fill_the_hits_column_by_chain_and_identity() {
        let body = body(&fixture::counters());
        let rows = table_of(&body)["rows"].as_array().expect("rows").clone();
        let hits = |id: &str| {
            rows.iter()
                .find(|row| row["id"] == id)
                .map(|row| row["cells"][8].clone())
                .expect("row")
        };
        assert_eq!(hits("allow_ping"), serde_json::json!({"text": "1.4k"}));
        // A counter that has counted nothing yet stays out of the way.
        assert_eq!(
            hits("allow_dhcp_renew"),
            serde_json::json!({"text": "0", "muted": true})
        );
        // The unnamed section is counted under firewall4's positional identity.
        assert_eq!(hits("cfg0af21e"), serde_json::json!({"text": "296"}));
        // A rule the ruleset carries no counter for states nothing.
        assert_eq!(
            hits("block_telnet"),
            serde_json::json!({"text": "—", "muted": true})
        );
    }
}
