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
    commit, commit_delete, commit_new, ActionTab, ColumnWidth, Envelope, Form, RowDrawer, Snapshot,
    TableAction, TableCell, TableColumn, TableGroup, TableRow, Tone, Widget,
};

use crate::counters::Counters;
use crate::fields;
use crate::format;
use crate::model::{Firewall, Rule, CONFIG};
use crate::page;
use crate::rule_drawer;
use crate::rule_form::{Errors, RuleForm};

/// HEADING is what this page is about, in the words the canvas uses: each of
/// this plugin's pages names its own subject rather than wearing the plugin's
/// name and the tab it is on.
pub const HEADING: &str = "Firewall rules";

/// frame wraps the listing in the firewall's page frame. The heading is the
/// page's whole introduction: the toolbar sits straight under it, and a lede
/// between them would say in prose what the listing says in rows.
fn frame(widget: Widget) -> Envelope {
    page::envelope(HEADING, "", widget)
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
    let children = vec![bar(&model.rules, None), table(model, counters, None)];
    frame(Widget::stack(children))
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
    let children = vec![
        bar(&model.rules, blank),
        table(model, counters, open.as_ref()),
    ];
    frame(Widget::stack(children))
}

/// seeded is the blank rule as the address asked for it. A lane's own New rule
/// button knows the chain it adds to, so the path arrives in the query; a zone
/// this config does not name is ignored rather than trusted into a select.
fn seeded(model: &Firewall, query: &Form) -> RuleForm {
    let mut form = RuleForm::default();
    let zones = model.zone_names();
    for (field, value) in [("src", &mut form.src), ("dest", &mut form.dest)] {
        let asked = query.get(field).trim().to_string();
        if zones.contains(&asked) {
            *value = asked;
        }
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
    if !removal.is_empty() || form.get(fields::DELETE_FIELD) == "1" {
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
    let children = vec![
        bar(&model.rules, None),
        table(model, counters, open.as_ref()),
    ];
    frame(Widget::stack(children))
}

/// opened_blank is the same for a rule that does not exist yet.
fn opened_blank(
    model: &Firewall,
    counters: &Counters,
    form: &RuleForm,
    errors: &Errors,
    tab: &str,
) -> Envelope {
    let children = vec![
        bar(
            &model.rules,
            Some(rule_drawer::blank(model, form, errors, tab)),
        ),
        table(model, counters, None),
    ];
    frame(Widget::stack(children))
}

/// bar is the listing's controls: the address-family cut, the lens, and the one
/// forward act. The family tabs are priced with what taking them would leave,
/// because a firewall is usually only wrong on one family at a time and the
/// count is what says which.
fn bar(rules: &[Rule], blank: Option<RowDrawer>) -> Widget {
    let ipv4 = rules.iter().filter(|rule| families(rule).0).count() as u32;
    let ipv6 = rules.iter().filter(|rule| families(rule).1).count() as u32;
    Widget::ActionBar {
        style: String::new(),
        tabs: vec![
            ActionTab {
                label: "All families".into(),
                count: rules.len() as u32,
                active: true,
                ..ActionTab::default()
            },
            ActionTab {
                label: "IPv4".into(),
                count: ipv4,
                matches: TAG_IPV4.into(),
                ..ActionTab::default()
            },
            ActionTab {
                label: "IPv6".into(),
                count: ipv6,
                matches: TAG_IPV6.into(),
                ..ActionTab::default()
            },
        ],
        filter: "Find a rule".into(),
        live: String::new(),
        // Making a rule is editing one that does not exist yet, so the act opens
        // the same panel a row's name opens rather than a page of its own.
        action: Some(TableAction {
            label: "Add rule".into(),
            href: rule_drawer::new_href("", ""),
            ..TableAction::default()
        }),
        opens_panel: true,
        drawer: blank,
    }
}

/// The tags a family tab cuts by. A rule that names no family covers both, and
/// so survives either cut.
const TAG_IPV4: &str = "ipv4";
const TAG_IPV6: &str = "ipv6";

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
                    tally: tally(&lane),
                    add_label: format!(
                        "Add rule to {}",
                        format::group_label(&lane.chain, lane.shared_dest.as_deref())
                    ),
                    add_text: "Add rule".into(),
                    add_href: add_href(&lane),
                    add_panel: true,
                });
            }
            rows.push(row);
        }
    }
    Widget::Table {
        style: String::new(),
        title: String::new(),
        detail: String::new(),
        dense: true,
        reorder_config: "firewall".into(),
        reorder_label: "Rule order".into(),
        columns: columns(),
        rows,
        drawer_label: String::new(),
        drawer_icon: String::new(),
        empty_text: EMPTY.into(),
        add_label: String::new(),
        add_href: String::new(),
        note: NOTE.into(),
        stream: None,
    }
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
        tags: tags(rule),
        cells: vec![
            TableCell::default(),
            page::index_cell(position),
            // The name is the row's subject and its door: what the log calls
            // this rule, and where it is edited.
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

/// families is the pair of address families this rule's traffic can be on.
fn families(rule: &Rule) -> (bool, bool) {
    format::families(&rule.proto, &rule.family)
}

/// tags are what the bar's family tabs cut this row by. A rule that names no
/// family covers both, so it carries both tags and survives either cut.
fn tags(rule: &Rule) -> Vec<String> {
    let (ipv4, ipv6) = families(rule);
    let mut tags = Vec::with_capacity(2);
    if ipv4 {
        tags.push(TAG_IPV4.to_string());
    }
    if ipv6 {
        tags.push(TAG_IPV6.to_string());
    }
    tags
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

/// tally is what a lane amounts to, in words: how many rules it evaluates, and
/// how many of those are switched off and so decide nothing.
fn tally(lane: &Lane) -> String {
    let off = lane.rules.iter().filter(|rule| !rule.enabled).count();
    let mut tally = match lane.rules.len() {
        1 => "1 rule".to_string(),
        n => format!("{n} rules"),
    };
    if off > 0 {
        tally.push_str(&format!(" · {off} disabled"));
    }
    tally
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
    rule_drawer::new_href("", "")
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

    #[test]
    fn the_page_carries_the_shared_firewall_frame() {
        let body = body(&Counters::default());
        assert_eq!(body["title"], HEADING);
        assert_eq!(body["width"], "wide");
        // No lede under the heading: the listing's own controls follow it.
        assert!(body.get("subheading").is_none());
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
        // The listing's own controls sit between the heading and the rows: the
        // family cut priced with what taking it leaves, the lens, and the one
        // act that adds to the listing.
        let bar = &body["widget"]["children"][0];
        assert_eq!(bar["type"], "actionbar");
        assert_eq!(bar["filter"], "Find a rule");
        assert_eq!(
            bar["action"],
            // Making a rule is editing one that does not exist yet, so the act
            // opens the same panel a row's name opens — the href is the
            // fallback a browser with no script follows.
            serde_json::json!({"label": "Add rule", "href": "/plugins/firewall/?open=new"})
        );
        assert_eq!(
            bar["tabs"],
            serde_json::json!([
                {"label": "All families", "count": 9, "active": true},
                {"label": "IPv4", "count": 6, "match": "ipv4"},
                {"label": "IPv6", "count": 7, "match": "ipv6"}
            ])
        );
        // The grid is the page's own content, not a titled region inside it.
        assert_eq!(body["widget"]["children"][1]["type"], "table");
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
        assert_eq!(drawer["size"], "wide");
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

    // The strip prices each reading before it is chosen, and the address says
    // which one is in force.
    #[test]
    fn the_panel_offers_three_readings_and_states_where_the_rule_stands() {
        let drawer = drawer_of(&opened("allow_ping", rule_drawer::ACTION));
        assert_eq!(
            drawer["tabs"],
            serde_json::json!([
                {
                    "label": "Match",
                    "state": "1 condition",
                    "href": "/plugins/firewall/?open=allow_ping"
                },
                {
                    "label": "Action",
                    "state": "accept",
                    "href": "/plugins/firewall/?open=allow_ping&tab=action",
                    "active": true
                },
                {
                    "label": "When",
                    "state": "always",
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
        // and invisible by design; the rows are the fields someone can see.
        let shapes: Vec<(&str, &str, &str)> = rows
            .iter()
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
        assert_eq!(drawer["children"][0]["submit"], "Save");
        assert!(drawer["children"][0].get("note").is_none());
    }

    // Making a rule is editing one that does not exist yet, so it opens the same
    // panel — from the bar, because there is no row for it to open from.
    #[test]
    fn the_bar_opens_a_blank_rule_seeded_with_the_lane_it_was_asked_from() {
        let body = opened(rule_drawer::NEW, "");
        let bar = &body["widget"]["children"][0];
        let drawer = &bar["drawer"];
        assert_eq!(drawer["title"], "New rule");
        assert_eq!(drawer["open"], true);
        assert_eq!(drawer["size"], "wide");
        // Closing it leaves the address it opened from, so a reload shows the
        // listing rather than reopening what was just dismissed.
        assert_eq!(drawer["closed"], "/plugins/firewall/");
        // No row carries a panel: the blank one belongs to the bar's own act.
        assert!(
            table_of(&body)["rows"]
                .as_array()
                .expect("rows")
                .iter()
                .all(|row| row.get("drawer").is_none()),
            "a blank rule opens from the bar, never from a row"
        );
        // A blank rule uses the same title-only header as an existing rule.
        assert!(drawer.get("verdict").is_none());
        assert!(drawer.get("lede").is_none());
        assert_eq!(drawer["children"][0]["submit"], "Add rule");
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
        let fields = &body["widget"]["children"][0]["drawer"]["children"][0]["fields"][0];
        let src = fields["children"]
            .as_array()
            .expect("rows")
            .iter()
            .find(|w| w["name"] == "src")
            .expect("the source row");
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
        let groups: Vec<(String, String, String, String)> = rows
            .iter()
            .filter_map(|row| row.get("group"))
            .map(|group| {
                (
                    group["label"].as_str().unwrap_or_default().to_string(),
                    group["to"].as_str().unwrap_or_default().to_string(),
                    group["key"].as_str().unwrap_or_default().to_string(),
                    group["tally"].as_str().unwrap_or_default().to_string(),
                )
            })
            .collect();
        // The band names the lane by its two ends and what it amounts to; the
        // chain is the drag's handle, not a word on the band. A lane counts the
        // rules it holds and says how many of them are off.
        assert_eq!(
            groups,
            vec![
                (
                    "WAN".to_string(),
                    "Router".to_string(),
                    "input_wan".to_string(),
                    "4 rules".to_string()
                ),
                (
                    "Guest".to_string(),
                    "Router".to_string(),
                    "input_guest".to_string(),
                    "2 rules · 1 disabled".to_string()
                ),
                (
                    "WAN".to_string(),
                    "Forwarded traffic".to_string(),
                    "forward_wan".to_string(),
                    "2 rules".to_string()
                ),
                (
                    "Guest".to_string(),
                    "LAN".to_string(),
                    "forward_guest".to_string(),
                    "1 rule".to_string()
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
                "tags": ["ipv4"],
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
                            "confirm": "When applied, traffic it allowed will be decided by whatever rule or zone policy comes next."
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
            // Both doors lead to the same place: the rule's own name, and the
            // glyph at the row's trailing edge. That place is this listing with
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
        // ICMPv6 exists only on IPv6, so the family cut knows it without the
        // rule having to state a family.
        assert_eq!(icmpv6["tags"], serde_json::json!(["ipv6"]));
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
