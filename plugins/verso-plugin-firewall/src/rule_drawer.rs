// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

//! One rule, opened beside the listing it came from.
//!
//! The listing answers "which rule decides this traffic"; the panel answers
//! "and what exactly does it say". Opening it never leaves the page, so the
//! evaluation order stays on screen while the rule that sits in it is read —
//! which is the whole reason position is meaning on this listing.
//!
//! The panel asks a rule three questions, one per tab: what it matches, what it
//! then does, and when it applies at all. Each tab is a link, so the reading is
//! in the address and the panel survives a reload, a back button and a shared
//! URL without a line of client state.
//!
//! The controls are the editor page's own, not copies of them: both compose the
//! same field builders, so a rule edited here and a rule edited there post
//! identical forms and can never drift into two spellings of one setting.

use verso_plugin::{DrawerTab, RowDrawer, SelectOption, Tone, Value, Widget};

use crate::conditions;
use crate::fields;
use crate::model::{Firewall, Rule};
use crate::page;
use crate::rule_form::{Errors, RuleForm, DSCP_CLASSES, HELPERS};

/// MATCH, ACTION and WHEN are the three readings of a rule, as they appear in
/// the address. MATCH is what a panel opens on: the first question about a rule
/// is always which traffic it is about.
pub const MATCH: &str = "match";
pub const ACTION: &str = "action";
pub const WHEN: &str = "when";

/// OPEN and TAB are the query keys the listing reads: which rule is open, and
/// which of its readings. They address something within the page rather than
/// which page to render, which is what the query is for.
pub const OPEN: &str = "open";
pub const TAB: &str = "tab";

const MATCH_TITLE: &str = "What this rule matches";
const MATCH_SUB: &str = "Traffic has to satisfy everything below.";

const ACTION_TITLE: &str = "What happens to it";
const ACTION_SUB: &str =
    "The first rule in the chain that matches decides, and nothing after it is consulted.";

const WHEN_TITLE: &str = "When it applies";
const WHEN_SUB: &str =
    "Outside this window the rule is skipped entirely, as though it were not in the chain.";

/// CONFIG_PATH is the file the panel previews, named as the operator would type
/// it.
const CONFIG_PATH: &str = "/etc/config/firewall";

/// NEW is what the query names instead of a section when the panel is to open on
/// a rule that does not exist yet. Making one and editing one are the same job,
/// so they are the same panel at the same kind of address.
pub const NEW: &str = "new";

/// href addresses one rule's panel on the listing: the listing itself, with the
/// rule and the reading in the query. A panel opened on its default reading
/// carries no tab, so the common address stays the short one.
pub fn href(section: &str, tab: &str) -> String {
    match tab {
        MATCH => format!("{}?{OPEN}={section}", page::rules_href()),
        _ => format!("{}?{OPEN}={section}&{TAB}={tab}", page::rules_href()),
    }
}

/// new_href opens the blank panel, seeded with whatever path the caller already
/// knows — a lane's own New rule button knows the chain it adds to, so the rule
/// starts where it was asked for rather than empty.
pub fn new_href(src: &str, dest: &str) -> String {
    let mut out = format!("{}?{OPEN}={NEW}", page::rules_href());
    if !src.is_empty() {
        out.push_str(&format!("&src={src}"));
    }
    if !dest.is_empty() {
        out.push_str(&format!("&dest={dest}"));
    }
    out
}

/// blank builds the panel for a rule that does not exist yet: the same three
/// readings, the same rows, and a commit that adds rather than saves.
pub fn blank(model: &Firewall, form: &RuleForm, errors: &Errors, tab: &str) -> RowDrawer {
    let (title, sub, mut fields) = tab_body(model, form, errors, tab);
    // A rule that does not exist yet has no config to preview and no hits to
    // count: the panel states what it will be, not what it is.
    fields.extend(carried(form, tab));
    fields.push(Widget::config_preview(CONFIG_PATH, &uci_block(NEW, form)));
    RowDrawer {
        title: "New rule".into(),
        tabs: new_tabs(form, model, tab),
        closed: page::rules_href(),
        open: true,
        children: vec![fields::panel_form(
            "Add rule",
            vec![Widget::section(title, sub, fields).flush()],
        )],
        ..RowDrawer::default()
    }
}

/// new_tabs are the blank rule's readings. They address the same panel, so
/// switching one before the rule exists keeps the blank panel open.
fn new_tabs(form: &RuleForm, model: &Firewall, active: &str) -> Vec<DrawerTab> {
    [
        (MATCH, "Match", conditions_state(form, model)),
        // fw4 reads ACCEPT and accept alike and its own default is the shouted
        // one; the strip states the verdict the way every listing spells it.
        (ACTION, "Action", form.target.to_lowercase()),
        (WHEN, "When", schedule_state(form)),
    ]
    .into_iter()
    .map(|(key, label, state)| DrawerTab {
        label: label.into(),
        state,
        href: match key {
            MATCH => new_href(&form.src, &form.dest),
            _ => format!("{}&{TAB}={key}", new_href(&form.src, &form.dest)),
        },
        active: key == active,
    })
    .collect()
}

/// reading is the tab a query asks for, falling back to the one a panel opens
/// on. A tab this panel does not have is not an error: it is a stale or hand-
/// typed address, and the honest answer is the rule's first reading.
pub fn reading(asked: &str) -> &str {
    match asked {
        ACTION => ACTION,
        WHEN => WHEN,
        _ => MATCH,
    }
}

/// drawer builds one rule's panel, on the reading asked for.
///
/// The header is the rule's name and nothing else. It used to carry the chain,
/// the verdict, the rule's position in the evaluation and its hit count as well
/// — every one of them true, and none of them the reason anybody opened the
/// panel. The listing behind it already states all four, on the row the panel
/// came out of and next to the other rules that give them their meaning; repeated
/// here they were four facts to read past on the way to the controls.
pub fn drawer(
    model: &Firewall,
    rule: &Rule,
    form: &RuleForm,
    errors: &Errors,
    tab: &str,
) -> RowDrawer {
    RowDrawer {
        title: title(&rule.name),
        tabs: tabs(rule, form, model, tab),
        closed: page::rules_href(),
        open: true,
        children: body(model, rule, form, errors, tab),
        ..RowDrawer::default()
    }
}

/// title names the rule, or says plainly that it has no name. Every rule has an
/// identity; not every one has a name someone wrote.
fn title(name: &str) -> String {
    match name.is_empty() {
        true => "An unnamed rule".to_string(),
        false => name.to_string(),
    }
}

/// tabs are the rule's three readings, each priced with where the rule stands
/// under it — so the strip answers before a tab is chosen.
fn tabs(rule: &Rule, form: &RuleForm, model: &Firewall, active: &str) -> Vec<DrawerTab> {
    [
        (MATCH, "Match", conditions_state(form, model)),
        // The verdict as the listing spells it, not as the config happens to:
        // fw4 reads ACCEPT and accept alike, and the strip is repeating the
        // answer the row gave rather than quoting the file.
        (ACTION, "Action", rule.target.clone()),
        (WHEN, "When", schedule_state(form)),
    ]
    .into_iter()
    .map(|(key, label, state)| DrawerTab {
        label: label.into(),
        state,
        href: href(&rule.section, key),
        active: key == active,
    })
    .collect()
}

/// conditions_state is how many narrowing conditions the rule carries beyond
/// its path — the number that says whether Match holds a sentence or a page.
fn conditions_state(form: &RuleForm, model: &Firewall) -> String {
    let count = match conditions::for_rule(form, &Errors::default(), model) {
        Widget::Conditions { items, .. } => items.iter().filter(|item| item.active).count(),
        _ => 0,
    };
    match count {
        1 => "1 condition".to_string(),
        n => format!("{n} conditions"),
    }
}

/// schedule_state is the window the rule keeps, as a word: a rule that names no
/// window is in force always, which is what most rules are.
fn schedule_state(form: &RuleForm) -> String {
    match form.active("schedule") {
        true => "restricted".to_string(),
        false => "always".to_string(),
    }
}

/// body is the reading itself: its heading and lede, its controls, and — on
/// every reading — the config this panel writes and the row that saves it.
/// The preview and the commit row repeat per tab because they are about the
/// whole rule, and a tab that hid them would be a tab you could save from
/// without seeing what you saved.
fn body(model: &Firewall, rule: &Rule, form: &RuleForm, errors: &Errors, tab: &str) -> Vec<Widget> {
    let (title, sub, mut fields) = tab_body(model, form, errors, tab);
    fields.extend(carried(form, tab));
    fields.push(Widget::config_preview(
        CONFIG_PATH,
        &uci_block(&rule.section, form),
    ));
    // The panel saves for itself rather than through the page's staging bar:
    // the bar belongs to the listing behind it, and a Save that is down there
    // while the form is up here is a Save nobody finds. Save puts the rule in
    // the stage and closes the panel; applying is the bar's, and so is saying
    // what it costs.
    vec![fields::panel_form(
        "Save rule",
        vec![Widget::section(title, sub, fields).flush()],
    )]
}

/// match_fields is what the rule is about, as one flat set of conditions rather
/// than as a path section and a catalogue of extras. Everything a rule matches
/// on is the same kind of statement — where from, where to, over what, on which
/// port — so every one of them is the same row, and each can be taken out the
/// same way. The name is not a condition, so it reserves the remove lane instead
/// of carrying the glyph, and the set ends with the one control that adds to it.
fn match_fields(model: &Firewall, form: &RuleForm, errors: &Errors) -> Vec<Widget> {
    let zones = |current: &str, router: &str| {
        let mut choices = vec![
            SelectOption::new("", router),
            SelectOption::new("*", "Anywhere"),
        ];
        choices.extend(conditions::named(&model.zone_names()));
        // A zone the config no longer names is still what this rule says, so it
        // stays in the list rather than being silently retargeted.
        if !current.is_empty() && current != "*" && !model.zone_names().iter().any(|z| z == current)
        {
            choices.push(SelectOption::new(current, current));
        }
        choices
    };
    vec![
        // None of these five ends in a glyph that clears it. They are the shape
        // every rule has — a rule without a path is not a rule — so there is
        // nothing to take away: a zone is widened by choosing Anywhere, a
        // protocol by emptying its box. The one thing a rule can be rid of is a
        // condition it was given, and the catalogue below ends each of those with
        // the X that removes it.
        fields::name_field(form, errors).writes("name"),
        // Where traffic comes from and where it goes are one answer, read as
        // the sentence it is: "lan to wan". Each pick keeps its own name.
        Widget::form_grid(
            2,
            vec![
                fields::select_field(
                    "src",
                    "Coming from",
                    &form.src,
                    zones(&form.src, "The router itself"),
                    errors,
                )
                .writes("src"),
                fields::select_field(
                    "dest",
                    "Going to",
                    &form.dest,
                    zones(&form.dest, "The router itself"),
                    errors,
                )
                .writes("dest"),
            ],
        )
        .labelled("Path", "")
        .joined("to"),
        // A list rather than a pick: fw4 reads protocol names and IP protocol
        // numbers alike, and a closed choice would make everything it does not
        // happen to list unwritable. The canvas draws a select over the common
        // pairs; that is a narrower control than the config allows.
        // No prompt: the label says Protocol and the suggestions say what one
        // looks like, so spelling it out again only gave the box a line of text
        // too long to fit beside the values it holds.
        fields::token_list("proto", "Protocol", "", "", &form.proto, errors)
            .writes("proto")
            .suggesting(conditions::named(&PROTOCOLS.map(String::from))),
        fields::select_field(
            "family",
            "Address family",
            &form.family,
            conditions::options(&[
                ("", "IPv4 and IPv6"),
                ("ipv4", "IPv4 only"),
                ("ipv6", "IPv6 only"),
            ]),
            errors,
        )
        .writes("family"),
        // Everything else a rule can match on, and the control that adds one.
        // The catalogue is the shell's: it renders the conditions this rule
        // carries as rows and keeps the rest behind its own picker, which is
        // exactly what the canvas draws. A hardcoded row per condition would
        // have been a shorter path to the same screen and would have quietly
        // made every condition outside it unreachable.
        conditions::for_rule(form, errors, model),
    ]
}

/// PROTOCOLS are the protocols worth offering by name. They are suggestions, not
/// the whole set: fw4 reads any protocol name its own tables know and any IP
/// protocol number, so the control takes whatever is typed and simply saves
/// anyone who wants one of these from having to remember how it is spelled.
const PROTOCOLS: [&str; 8] = ["tcp", "udp", "icmp", "icmpv6", "esp", "ah", "igmp", "gre"];

/// The fields each reading renders. A submission carries the reading it was on,
/// so everything outside it rides along as an inert carrier: a rule is one
/// object and Save means "save the rule", not "save the third of it I can
/// currently see". Without the carriers the fields on the other two readings
/// would arrive absent, and absent is how a form says "cleared".
///
/// These name **form fields**, not uci options, because that is what a
/// submission is made of and what [`RuleForm::submitted`] reads. They used to
/// name options, and the two vocabularies do not line up: a rate is one option
/// and four fields, a schedule's clock is `time_basis` on the form and
/// `utc_time` in the file, a list is one option and one field per value. Every
/// place they disagreed was a value that arrived wrong or did not arrive at all
/// — which is how saving a rule from its Action reading came to delete every
/// condition it matched on.
const MATCH_FIELDS: [&str; 5] = ["name", "src", "dest", "proto", "family"];
const ACTION_FIELDS: [&str; 11] = [
    "target",
    "enabled",
    "log",
    "log_prefix",
    "log_limit",
    "counter",
    "set_helper",
    "mark_operation",
    "set_mark",
    "set_mark_mask",
    "set_dscp",
];

/// shown is the set of fields the reading in force is asking about.
///
/// The Match reading shows the catalogue as well as its own five rows, and the
/// catalogue is every optional match a rule can carry — so a condition the rule
/// does not have posts nothing there, which is exactly what "it is gone" means.
/// The When reading shows the schedule, which the catalogue also holds, so the
/// two overlap by design.
fn shown(tab: &str) -> Vec<&'static str> {
    match tab {
        ACTION => ACTION_FIELDS.to_vec(),
        WHEN => conditions::SCHEDULE_FIELDS.to_vec(),
        _ => {
            let mut out = MATCH_FIELDS.to_vec();
            out.extend_from_slice(conditions::FIELDS);
            out
        }
    }
}

/// carried is every value this reading does not show, as the hidden fields that
/// post it back unchanged. A field holding several values carries one hidden
/// apiece: joined into one they would arrive as a single value with spaces in
/// it, which is a different rule.
fn carried(form: &RuleForm, tab: &str) -> Vec<Widget> {
    let visible = shown(tab);
    let mut out = Vec::new();
    for (field, values) in posted(form) {
        if visible.contains(&field) {
            continue;
        }
        for value in values {
            out.push(Widget::hidden(field, &value));
        }
    }
    out
}

/// posted is every form field a rule's panel submits, with what the rule says it
/// is now — the other half of [`RuleForm::submitted`], which names the fields it
/// reads. A field in one and not the other is a value that vanishes on save, and
/// the round trip through every reading is what holds the two together.
///
/// A condition the rule does not carry posts nothing, including its comparison:
/// absent is how a form says a condition is gone. One it does carry posts all of
/// its parts, the comparison included — carried without it, a negated match
/// would come back as a positive one.
fn posted(form: &RuleForm) -> Vec<(&'static str, Vec<String>)> {
    let mut out = Posted::default();
    out.one("name", &form.name);
    out.flag("enabled", form.enabled);
    out.one("src", &form.src);
    out.one("dest", &form.dest);
    out.one("family", &form.family);
    out.many("proto", &form.proto);

    // The verdict and the parameters of whichever verdict it is.
    out.one("target", &form.target);
    out.one("set_helper", &form.set_helper);
    out.one("mark_operation", mark_operation(form));
    out.one("set_mark", &form.set_mark);
    out.one("set_mark_mask", &form.set_mark_mask);
    out.one("set_dscp", &form.set_dscp);
    out.flag("counter", form.counter);
    out.flag("log", form.log.on);
    out.one("log_prefix", &form.log.prefix);
    out.one("log_limit", &form.log.limit);

    if form.active("device") {
        out.one("direction", &form.device.direction);
        out.one("device", &form.device.name);
    }
    for (key, field, excluded, tokens) in [
        ("src_ip", "src_ip", "src_ip_not", &form.src_ip),
        ("src_mac", "src_mac", "src_mac_not", &form.src_mac),
        ("src_port", "src_port", "src_port_not", &form.src_port),
        ("dest_ip", "dest_ip", "dest_ip_not", &form.dest_ip),
        ("dest_port", "dest_port", "dest_port_not", &form.dest_port),
    ] {
        if form.active(key) {
            out.many(field, &tokens.include);
            out.many(excluded, &tokens.exclude);
        }
    }
    if form.active("icmp_type") {
        out.many("icmp_type", &form.icmp_type);
    }
    if form.active("ipset") {
        out.one("ipset", &form.ipset.set.value);
        out.one("ipset_match", form.ipset.set.comparison());
        for (slot, field) in ["ipset_field_1", "ipset_field_2", "ipset_field_3"]
            .iter()
            .enumerate()
        {
            out.one(field, &form.ipset.fields[slot]);
        }
    }
    if form.active("helper") {
        out.one("helper", &form.helper.value);
        out.one("helper_match", form.helper.comparison());
    }
    if form.active("mark") {
        out.one("mark_value", &form.mark.mark.value);
        out.one("mark_match", form.mark.mark.comparison());
        out.one("mark_mask", &form.mark.mask);
    }
    if form.active("dscp") {
        out.one("dscp", &form.dscp.value);
        out.one("dscp_match", form.dscp.comparison());
    }
    if form.active("rate") {
        out.one("limit", &form.rate.count);
        out.one("limit_unit", &form.rate.unit);
        out.one("limit_match", rate_comparison(form));
        out.one("limit_burst", &form.rate.burst);
    }
    if form.active("schedule") {
        out.many("weekdays", &form.schedule.weekdays);
        out.one("start_date", &form.schedule.start_date);
        out.one("stop_date", &form.schedule.stop_date);
        out.one("start_time", &form.schedule.start_time);
        out.one("stop_time", &form.schedule.stop_time);
        out.one("time_basis", clock(form));
    }
    out.0
}

/// Posted collects the fields a form submits. A blank scalar is left out
/// entirely: a form posting an empty box and a form not posting it say the same
/// thing to the reader, and the shorter one is fewer hidden inputs on a page.
#[derive(Default)]
struct Posted(Vec<(&'static str, Vec<String>)>);

impl Posted {
    fn one(&mut self, field: &'static str, value: &str) {
        if !value.is_empty() {
            self.0.push((field, vec![value.to_string()]));
        }
    }

    fn many(&mut self, field: &'static str, values: &[String]) {
        if !values.is_empty() {
            self.0.push((field, values.to_vec()));
        }
    }

    /// A switch posts while it is on and not at all while it is off, which is
    /// how a browser posts a checkbox and therefore how the form reads one.
    fn flag(&mut self, field: &'static str, on: bool) {
        if on {
            self.0.push((field, vec!["1".to_string()]));
        }
    }
}

fn rate_comparison(form: &RuleForm) -> &'static str {
    match form.rate.over {
        true => "over",
        false => "below",
    }
}

fn clock(form: &RuleForm) -> &'static str {
    match form.schedule.utc {
        true => "utc",
        false => "local",
    }
}

/// tab_body is one tab's heading, lede and rows. All three readings are the same
/// flat set of rows — the panel asks a rule three questions and asks each of them
/// the same way.
fn tab_body(
    model: &Firewall,
    form: &RuleForm,
    errors: &Errors,
    tab: &str,
) -> (&'static str, &'static str, Vec<Widget>) {
    match tab {
        ACTION => (ACTION_TITLE, ACTION_SUB, action_fields(form, errors)),
        WHEN => (WHEN_TITLE, WHEN_SUB, when_fields(form, errors)),
        _ => (MATCH_TITLE, MATCH_SUB, match_fields(model, form, errors)),
    }
}

/// action_fields is what the rule does when it matches: the verdict it takes,
/// whether it is in the chain at all, and the trace it leaves behind. Each is one
/// row, as everything in this panel is.
fn action_fields(form: &RuleForm, errors: &Errors) -> Vec<Widget> {
    vec![
        fields::select_field(
            "target",
            "Verdict",
            &form.target.to_lowercase(),
            conditions::options(&TARGET_CHOICES),
            errors,
        )
        .writes("target"),
        // The page hangs this off its heading as an inline switch; here it is one
        // row among the rule's other answers, so it is the row every other
        // setting is.
        Widget::switch_keyed(
            "enabled",
            "Rule is active",
            "enabled",
            ENABLED_TIP,
            form.enabled,
        ),
        // Logging is not one option but three: whether to log, under what
        // prefix, and how often. The gate is the row; the two it governs appear
        // under it when it is on, which is the only place they mean anything.
        Widget::gate(
            "log",
            "Log every match",
            "log",
            LOG_TIP,
            form.log.on,
            vec![
                fields::text_field("log_prefix", "Log prefix", &form.log.prefix, "", errors),
                fields::text_field("log_limit", "Log rate limit", &form.log.limit, "", errors),
            ],
            Vec::new(),
        ),
        Widget::switch_keyed(
            "counter",
            "Count matches",
            "counter",
            COUNTER_TIP,
            form.counter,
        ),
        // The verdicts that are not accept/reject/drop need a parameter apiece —
        // which mark to set, which DSCP class, which connection helper. They fold
        // away because most rules need none of them, but they have to be here:
        // without them a rule can be told to mark traffic and never told what
        // with.
        Widget::Disclosure {
            style: "condition".into(),
            summary: "Parameters for the less common actions".into(),
            open: false,
            children: vec![
                Widget::callout(Tone::Warning, "", ADVANCED_NOTE),
                fields::select_field(
                    "set_helper",
                    "Connection helper to assign",
                    &form.set_helper,
                    conditions::named(&HELPERS.map(String::from)),
                    errors,
                ),
                fields::select_field(
                    "mark_operation",
                    "Mark operation",
                    mark_operation(form),
                    conditions::options(&[("set", "Set"), ("xor", "XOR")]),
                    errors,
                ),
                fields::text_field("set_mark", "Mark value", &form.set_mark, "", errors),
                fields::text_field(
                    "set_mark_mask",
                    "Mark mask",
                    &form.set_mark_mask,
                    "Optional.",
                    errors,
                ),
                fields::select_field(
                    "set_dscp",
                    "DSCP value to apply",
                    &form.set_dscp,
                    conditions::named(&DSCP_CLASSES.map(String::from)),
                    errors,
                ),
            ],
        },
    ]
}

const ADVANCED_NOTE: &str = "Only the parameters the chosen action needs are written; \
the rest are left out of the rule.";

/// mark_operation is which of fw4's two mark writes this rule uses: set_xmark is
/// the same verdict as set_mark with the operation flipped.
fn mark_operation(form: &RuleForm) -> &'static str {
    match form.mark_xor {
        true => "xor",
        false => "set",
    }
}

/// TARGET_CHOICES is the verdict a rule can take, in the canvas's order: the
/// three that decide whether traffic arrives, then the ones that change how it is
/// handled on the way.
const TARGET_CHOICES: [(&str, &str); 7] = [
    ("accept", "accept"),
    ("reject", "reject"),
    ("drop", "drop"),
    ("mark", "mark"),
    ("dscp", "dscp"),
    ("notrack", "notrack"),
    ("helper", "helper"),
];

const ENABLED_TIP: &str = "A rule that is off stays in the config and decides nothing — the \
traffic falls through to whatever comes next.";
const LOG_TIP: &str = "Every match writes a line to the 64 KB ring, which a busy rule fills in \
minutes.";
const COUNTER_TIP: &str = "Turning counting off empties this rule's Hits column.";

/// when_fields is the window the rule keeps. A window is a pair of ends, so each
/// pair is one row rather than two: as two rows a reader is invited to set the
/// start and forget the end.
fn when_fields(form: &RuleForm, errors: &Errors) -> Vec<Widget> {
    // One set of controls wherever a schedule is asked: the same the schedule
    // condition offers, posting the same fields, because it is the same fact.
    conditions::schedule_fields(&form.schedule, errors)
}

/// uci_block renders the rule as the config file will hold it: the section it
/// writes, then one line per option in the order uci itself lists them. A list
/// option writes one line per value, the way the file does.
fn uci_block(section: &str, form: &RuleForm) -> String {
    let mut out = format!("config rule '{section}'\n");
    let Value::Object(values) = form.values(true) else {
        return out;
    };
    for (option, value) in &values {
        match value {
            Value::Array(items) => {
                for item in items {
                    out.push_str(&format!("\tlist {option} '{}'\n", scalar(item)));
                }
            }
            Value::Null => {}
            other => out.push_str(&format!("\toption {option} '{}'\n", scalar(other))),
        }
    }
    out
}

/// scalar is one uci value as the file spells it — uci holds strings, so a
/// number or a flag is written as the characters it is stored as rather than as
/// JSON would quote it.
fn scalar(value: &Value) -> String {
    match value {
        Value::String(text) => text.clone(),
        other => other.to_string(),
    }
}
#[cfg(test)]
mod tests {
    use super::*;
    use crate::counters::Counters;
    use crate::fields::DELETE_FIELD;
    use crate::fixture;
    use serde_json::Value;
    use verso_plugin::Form;

    /// Saving from one reading writes the whole rule, not the third of it that
    /// reading shows.
    ///
    /// This is the contract the carriers exist for, and it was broken in both
    /// directions: the Action reading claimed to show a rate limit it did not
    /// draw, and carried none of the rule's conditions at all — so opening a
    /// rule, going to Action and pressing Save deleted every address, port, MAC,
    /// set, helper, mark, DSCP and ICMP type it matched on, silently, and the
    /// operator saw a rule they had not changed become a rule that matched
    /// everything.
    ///
    /// It is checked as a round trip rather than by reading the lists: take a
    /// rule carrying the whole vocabulary, submit exactly what each reading
    /// posts, and the write has to be the write the rule already was.
    #[test]
    fn saving_from_any_reading_writes_the_whole_rule() {
        let snapshot = fixture::editor_snapshot();
        let intact = RuleForm::read(
            &snapshot
                .section("firewall", "everything")
                .expect("the fixture's rule"),
        )
        .values(true);

        for tab in [MATCH, ACTION, WHEN] {
            let body = serde_json::to_value(crate::rules::page_open(
                &snapshot,
                &fixture::editor_firewall(),
                &Counters::default(),
                &Form::parse(&format!("open=everything&tab={tab}")),
            ))
            .expect("serialize");

            let mut model = fixture::editor_firewall();
            let answer = serde_json::to_value(crate::rules::save(
                &snapshot,
                &mut model,
                &Counters::default(),
                &Form::parse(&format!("open=everything&tab={tab}")),
                &Form::parse(&fixture::submission(&body)),
            ))
            .expect("serialize");

            if answer["notice"]["level"] != "success" {
                let mut marked = Vec::new();
                fixture::errors_in(&answer, &mut marked);
                panic!("the {tab} reading posts what the editor refuses: {marked:?}");
            }
            assert_eq!(
                answer["commit"][0]["values"], intact,
                "saving from the {tab} reading rewrites the rule",
            );
        }
    }

    /// open is one rule's panel as a visit to its address renders it. A rule is
    /// read and written in the panel beside the listing and nowhere else, so this
    /// is the whole of what an operator can see and submit about one.
    fn open(section: &str) -> Value {
        let envelope = crate::rules::page_open(
            &fixture::editor_snapshot(),
            &fixture::editor_firewall(),
            &Counters::default(),
            &Form::parse(&format!("open={section}")),
        );
        serde_json::to_value(&envelope).expect("serialize")
    }

    /// opened_new is the blank panel the bar's own act opens.
    fn opened_new() -> Value {
        opened_new_on(MATCH)
    }

    /// opened_new_on is the blank panel on a named reading.
    fn opened_new_on(tab: &str) -> Value {
        let envelope = crate::rules::page_open(
            &fixture::editor_snapshot(),
            &fixture::editor_firewall(),
            &Counters::default(),
            &Form::parse(&format!("open=new&tab={tab}")),
        );
        serde_json::to_value(&envelope).expect("serialize")
    }

    /// opened_on is one rule's panel on a named reading — a control lives on the
    /// reading that asks about it, so a test looking for the log asks for Action.
    fn opened_on(section: &str, tab: &str) -> Value {
        let envelope = crate::rules::page_open(
            &fixture::editor_snapshot(),
            &fixture::editor_firewall(),
            &Counters::default(),
            &Form::parse(&format!("open={section}&tab={tab}")),
        );
        serde_json::to_value(&envelope).expect("serialize")
    }

    fn submit(section: &str, fields: &[(&str, &str)]) -> Value {
        let mut model = fixture::editor_firewall();
        let envelope = crate::rules::save(
            &fixture::editor_snapshot(),
            &mut model,
            &Counters::default(),
            &Form::parse(&format!("open={section}")),
            &Form::parse(&encode(fields)),
        );
        serde_json::to_value(&envelope).expect("serialize")
    }

    fn encode(fields: &[(&str, &str)]) -> String {
        fields
            .iter()
            .map(|(name, value)| format!("{name}={}", value.replace(' ', "+")))
            .collect::<Vec<String>>()
            .join("&")
    }

    /// control finds one named control anywhere in the editor, which is how a
    /// test asks what the operator would see without walking the composition.
    fn control(body: &Value, name: &str) -> Value {
        fixture::control(body, name)
    }

    /// conditions returns the catalogue entries by key. The block's own lede is
    /// the callout above it, so the catalogue is the section's second child.
    fn conditions(body: &Value) -> Value {
        fixture::widget(body, "conditions")
    }

    fn condition(body: &Value, key: &str) -> Value {
        conditions(body)["items"]
            .as_array()
            .expect("items")
            .iter()
            .find(|item| item["key"] == key)
            .cloned()
            .unwrap_or_else(|| panic!("no condition {key}"))
    }

    fn values(body: &Value) -> Value {
        body["commit"][0]["values"].clone()
    }

    #[test]
    fn a_carried_condition_is_active_and_an_uncarried_one_waits_in_the_picker() {
        let body = open("everything");
        for key in [
            "device",
            "src_ip",
            "src_mac",
            "src_port",
            "dest_ip",
            "dest_port",
            "icmp_type",
            "ipset",
            "helper",
            "mark",
            "dscp",
            "rate",
            "schedule",
        ] {
            assert_eq!(condition(&body, key)["active"], true, "{key}");
        }

        let plain = open("plain");
        for item in conditions(&plain)["items"].as_array().expect("items") {
            assert!(
                item.get("active").is_none(),
                "{} is active on a rule that carries nothing",
                item["key"]
            );
        }
    }

    #[test]
    fn a_conditions_values_come_back_split_the_way_they_were_written() {
        let body = open("everything");
        assert_eq!(
            control(&body, "src_ip")["items"],
            serde_json::json!(["10.0.20.0/24"])
        );
        assert_eq!(
            control(&body, "src_ip_not")["items"],
            serde_json::json!(["10.0.20.5"]),
            "an inverted value belongs in the exclude list, without its mark"
        );
        assert_eq!(
            control(&body, "dest_port")["items"],
            serde_json::json!(["53"])
        );
        assert_eq!(
            control(&body, "dest_port_not")["items"],
            serde_json::json!(["5353"])
        );
        assert_eq!(
            control(&body, "src_port")["items"],
            serde_json::json!(["1024-65535"])
        );
        assert_eq!(
            control(&body, "icmp_type")["items"],
            serde_json::json!(["echo-request"])
        );
        assert_eq!(
            control(&body, "proto")["items"],
            serde_json::json!(["tcp", "udp"])
        );
    }

    #[test]
    fn a_negated_condition_reads_as_the_exclude_side_of_its_comparison() {
        let body = open("everything");
        assert_eq!(control(&body, "ipset")["value"], "blocked_hosts");
        assert_eq!(control(&body, "ipset_match")["value"], "exclude");
        assert_eq!(control(&body, "ipset_field_1")["value"], "src");
        assert_eq!(
            control(&body, "ipset_field_2")["value"],
            "dst",
            "fw4 spells the destination dimension both ways; the editor offers one"
        );
        assert_eq!(control(&body, "helper")["value"], "ftp");
        assert_eq!(control(&body, "helper_match")["value"], "exclude");
        assert_eq!(control(&body, "mark_value")["value"], "0x10");
        assert_eq!(control(&body, "mark_mask")["value"], "0xff");
        assert_eq!(control(&body, "mark_match")["value"], "exclude");
        assert_eq!(control(&body, "dscp")["value"], "EF");
        assert_eq!(control(&body, "dscp_match")["value"], "include");
    }

    #[test]
    fn a_rate_and_a_schedule_come_apart_into_the_controls_that_wrote_them() {
        let body = open("everything");
        assert_eq!(control(&body, "limit")["value"], "1000");
        assert_eq!(control(&body, "limit_unit")["value"], "minute");
        assert_eq!(control(&body, "limit_match")["value"], "over");
        assert_eq!(control(&body, "limit_burst")["value"], "5");
        assert_eq!(
            control(&body, "weekdays")["values"],
            serde_json::json!(["Mon", "Tue"])
        );
        // Each day wears the three letters uci writes it in, as QoS's strip
        // does: the drawer has the width, so no day is cut to two.
        let days = fixture::find_with(&opened_on("everything", WHEN), &|value| {
            value["name"] == "weekdays" && value["style"] == "segmented"
        })
        .expect("the days strip");
        let labels: Vec<&str> = days["options"]
            .as_array()
            .unwrap()
            .iter()
            .map(|option| option["label"].as_str().unwrap())
            .collect();
        assert_eq!(labels, ["Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"]);
        assert_eq!(control(&body, "start_date")["value"], "2026-01-01");
        assert_eq!(control(&body, "stop_time")["value"], "18:00:00");
        assert_eq!(control(&body, "time_basis")["value"], "utc");
    }

    #[test]
    fn the_action_and_its_parameters_read_as_one_verdict() {
        let body = opened_on("everything", ACTION);
        // The verdict reads the way every listing spells it: fw4 accepts MARK and
        // mark alike, and the panel offers the one vocabulary.
        assert_eq!(control(&body, "target")["value"], "mark");
        assert_eq!(
            control(&body, "mark_operation")["value"],
            "xor",
            "set_xmark is the same verdict as set_mark with the operation flipped"
        );
        assert_eq!(control(&body, "set_mark")["value"], "0x20");
        assert_eq!(control(&body, "set_mark_mask")["value"], "0xff");
    }

    /// A switch states only that it is on: an off one carries no state at all,
    /// which is the wire shape the shell reads.
    fn switched_on(body: &Value, name: &str) -> bool {
        control(body, name)["on"] == Value::Bool(true)
    }

    #[test]
    fn the_state_and_the_log_read_the_options_that_carry_them() {
        let body = opened_on("everything", ACTION);
        assert!(!switched_on(&body, "enabled"));
        assert!(!switched_on(&body, "counter"));
        assert_eq!(control(&body, "log")["checked"], true);
        assert_eq!(control(&body, "log_prefix")["value"], "guest-audit: ");
        assert_eq!(control(&body, "log_limit")["value"], "10/minute");

        // A rule stating none of them runs on firewall4's defaults: on, counted,
        // unlogged.
        let plain = opened_on("plain", ACTION);
        assert!(switched_on(&plain, "enabled"));
        assert!(switched_on(&plain, "counter"));
        assert_eq!(control(&plain, "log")["checked"], false);
    }

    #[test]
    fn a_new_rule_starts_where_firewall4s_own_defaults_are() {
        let body = opened_new();
        assert_eq!(
            body["widget"]["children"][0]["drawer"]["title"], "New rule",
            "the bar's own act opens the blank panel"
        );
        let action = opened_new_on(ACTION);
        assert_eq!(control(&action, "enabled")["on"], true);
        assert_eq!(control(&action, "counter")["on"], true);
        assert_eq!(control(&action, "target")["value"], "accept");
        assert_eq!(
            control(&body, "proto")["items"],
            serde_json::json!(["tcp", "udp"])
        );
        assert_eq!(control(&body, "name")["value"], "");
        // The panel opens over the listing page as it is: no lede under its heading.
        assert!(body.get("subheading").is_none());
        assert!(
            fixture::find_with(&body, &|value| value["type"] == "form"
                && value["fields"][0]["name"] == DELETE_FIELD)
            .is_none(),
            "a rule that does not exist yet cannot be deleted"
        );
    }

    #[test]
    fn the_traffic_path_offers_the_router_anywhere_and_this_configs_zones() {
        let body = open("plain");
        assert_eq!(
            control(&body, "src")["options"],
            serde_json::json!([
                {"value": "", "label": "The router itself"},
                {"value": "*", "label": "Anywhere"},
                {"value": "lan", "label": "lan"},
                {"value": "guest", "label": "guest"}
            ])
        );
    }

    // The card at the foot of every reading is the file the rule is written
    // into, declared in its grammar so the shell reads it as an editor would.
    #[test]
    fn the_rule_is_written_in_the_uci_grammar() {
        for body in [
            open("everything"),
            opened_new(),
            opened_on("everything", WHEN),
        ] {
            let card = fixture::find_with(&body, &|value| value["type"] == "code")
                .expect("the configuration card");
            assert_eq!(card["grammar"], "uci");
            assert_eq!(card["live"], true);
        }
    }

    // An exclusion is the rare half of an include/exclude condition, so it
    // waits behind a quiet "Exclude some" until it is wanted — and arrives
    // open whenever the rule already excludes something, so nothing the rule
    // says is hidden.
    #[test]
    fn exclusions_wait_until_wanted() {
        let reveal_of = |body: &Value, list: &str| {
            fixture::find_with(body, &|value| {
                value["type"] == "disclosure"
                    && value["children"]
                        .as_array()
                        .is_some_and(|c| c.iter().any(|w| w["name"] == list))
            })
            .unwrap_or_else(|| panic!("a reveal around {list}"))
        };
        // The fixture's rule excludes one host from its source network.
        let body = open("everything");
        let src = reveal_of(&body, "src_ip_not");
        assert_eq!(src["style"], "reveal");
        assert_eq!(src["summary"], "Exclude some");
        assert_eq!(src["open"], true);
        // A blank rule excludes nothing, so every exclusion waits folded.
        let blank = opened_new();
        for list in [
            "src_ip_not",
            "dest_ip_not",
            "src_port_not",
            "dest_port_not",
            "src_mac_not",
        ] {
            assert!(reveal_of(&blank, list).get("open").is_none(), "{list}");
        }
    }

    // A condition is itself the group its parts belong to, so its parts are
    // its own rows: a wrapper around them would only hide them from the
    // condition's rhythm.
    #[test]
    fn a_conditions_parts_are_its_own_rows() {
        let body = open("everything");
        let catalogue = fixture::find_with(&body, &|value| value["type"] == "conditions")
            .expect("the conditions");
        for condition in catalogue["items"].as_array().unwrap() {
            for part in condition["children"].as_array().into_iter().flatten() {
                assert_ne!(part["type"], "stack", "{}: {part}", condition["key"]);
            }
        }
    }

    // A schedule is one set of controls wherever it is asked: the condition in
    // the catalogue (a port forward's only way to it) asks exactly what the
    // When tab asks — the days as one strip, each window as its two ends
    // joined by their word, the clock they are read against.
    #[test]
    fn a_schedule_is_asked_one_way() {
        let shape = |widgets: &Value| -> Vec<String> {
            widgets
                .as_array()
                .unwrap()
                .iter()
                .map(|w| {
                    format!(
                        "{}:{}:{}:{}",
                        w["type"].as_str().unwrap_or_default(),
                        w["name"]
                            .as_str()
                            .unwrap_or(w["label"].as_str().unwrap_or_default()),
                        w["style"].as_str().unwrap_or_default(),
                        w["join"].as_str().unwrap_or_default()
                    )
                })
                .collect()
        };
        let body = open("everything");
        let condition = fixture::find_with(&body, &|value| {
            value["key"] == "schedule" && value.get("children").is_some()
        })
        .expect("the schedule condition");
        let when = fixture::find_with(&opened_on("everything", WHEN), &|value| {
            value["type"] == "section" && value["children"].is_array()
        })
        .expect("the When reading");
        // The reading's own rows, without the carriers for the other readings.
        let asked: Vec<String> = shape(&when["children"])
            .into_iter()
            .filter(|s| !s.contains("hidden"))
            .take(shape(&condition["children"]).len())
            .collect();
        assert_eq!(shape(&condition["children"]), asked);
        assert!(shape(&condition["children"]).contains(&"grid:Between:form:to".to_string()));
        assert!(shape(&condition["children"]).contains(&"field:weekdays:segmented:".to_string()));
    }

    // A rate limit reads as the sentence it writes, "1000 per second": the
    // count and its unit one row joined by their word, then which side of the
    // rate matches, then the burst — each part naming the option it writes, so
    // the count and the burst stand at a number's width.
    #[test]
    fn a_rate_limit_reads_as_a_rate() {
        let body = open("everything");
        let rate = fixture::find_with(&body, &|value| {
            value["key"] == "rate" && value.get("children").is_some()
        })
        .expect("the rate condition");
        let parts = rate["children"].as_array().unwrap();
        assert_eq!(parts[0]["type"], "grid");
        assert_eq!(parts[0]["label"], "Rate");
        assert_eq!(parts[0]["join"], "per");
        let ends: Vec<(&str, &str)> = parts[0]["children"]
            .as_array()
            .unwrap()
            .iter()
            .map(|end| (end["name"].as_str().unwrap(), end["key"].as_str().unwrap()))
            .collect();
        assert_eq!(ends, [("limit", "limit"), ("limit_unit", "limit")]);
        assert_eq!(parts[1]["name"], "limit_match");
        assert_eq!(parts[2]["name"], "limit_burst");
        assert_eq!(parts[2]["key"], "limit_burst");
    }

    // A window is its two ends read as one sentence, "09:00 to 17:00": one row
    // each for the clock and the calendar, the ends joined by their word, each
    // end its own field posting its own option under the grammar fw4 reads.
    #[test]
    fn a_window_is_one_row_from_an_end_to_an_end() {
        let body = opened_on("everything", WHEN);
        for (label, ends, grammar) in [
            ("Between", ["start_time", "stop_time"], "timehhmmss"),
            (
                "Only between dates",
                ["start_date", "stop_date"],
                "dateyyyymmdd",
            ),
        ] {
            let window = fixture::find_with(&body, &|value| {
                value["type"] == "grid" && value["label"] == label
            })
            .unwrap_or_else(|| panic!("the {label} row"));
            assert_eq!(window["style"], "form");
            assert_eq!(window["join"], "to");
            let children = window["children"].as_array().unwrap();
            let names: Vec<&str> = children
                .iter()
                .map(|c| c["name"].as_str().unwrap())
                .collect();
            assert_eq!(names, ends, "{label}");
            for end in children {
                assert_eq!(end["datatype"], grammar, "{label}");
                assert_eq!(end["key"], end["name"], "{label}");
            }
        }
        assert!(
            fixture::find_with(&body, &|value| value.get("pair").is_some()).is_none(),
            "no field carries a second value of its own"
        );
    }

    // Where traffic comes from and where it goes are one answer, read as the
    // sentence "lan to wan": one row named Path, the two zone picks joined by
    // their word, each still posting its own option.
    #[test]
    fn the_path_is_one_row_from_a_zone_to_a_zone() {
        let body = open("everything");
        let path = fixture::find_with(&body, &|value| {
            value["type"] == "grid" && value["join"] == "to"
        })
        .expect("the path row");
        assert_eq!(path["style"], "form");
        assert_eq!(path["label"], "Path");
        let names: Vec<&str> = path["children"]
            .as_array()
            .unwrap()
            .iter()
            .map(|child| child["name"].as_str().unwrap())
            .collect();
        assert_eq!(names, ["src", "dest"]);
    }

    #[test]
    fn creating_a_rule_states_only_the_options_it_sets() {
        let mut model = fixture::editor_firewall();
        let body = serde_json::to_value(crate::rules::save(
            &fixture::editor_snapshot(),
            &mut model,
            &Counters::default(),
            &Form::parse("open=new"),
            &Form::parse(&encode(&[
                ("name", "Block+guest+SSH"),
                ("enabled", "1"),
                ("src", "guest"),
                ("dest", "lan"),
                ("proto", "tcp"),
                ("dest_port", "22"),
                ("target", "DROP"),
                ("counter", "1"),
            ])),
        ))
        .expect("serialize");

        assert_eq!(body["notice"]["level"], "success");
        assert_eq!(body["commit"][0]["config"], "firewall");
        assert_eq!(body["commit"][0]["section"], "");
        assert_eq!(body["commit"][0]["type"], "rule");
        assert_eq!(
            values(&body),
            serde_json::json!({
                "name": "Block guest SSH",
                "enabled": "1",
                "counter": "1",
                "src": "guest",
                "dest": "lan",
                "proto": ["tcp"],
                "dest_port": ["22"],
                "target": "DROP"
            })
        );
    }

    #[test]
    fn saving_a_rule_clears_every_option_it_owns_that_the_submission_dropped() {
        let body = submit(
            "everything",
            &[
                ("name", "Everything"),
                ("enabled", "1"),
                ("src", "guest"),
                ("dest", "lan"),
                ("proto", "tcp"),
                ("target", "ACCEPT"),
                ("counter", "1"),
            ],
        );
        assert_eq!(body["notice"]["level"], "success");
        assert_eq!(body["commit"][0]["section"], "everything");
        assert_eq!(
            values(&body),
            serde_json::json!({
                "name": "Everything",
                "enabled": "1",
                "counter": "1",
                "src": "guest",
                "dest": "lan",
                "proto": ["tcp"],
                "target": "ACCEPT",
                // Every condition the submission no longer carries is cleared,
                // never written empty.
                "family": null,
                "device": null,
                "direction": null,
                "src_ip": null,
                "src_mac": null,
                "src_port": null,
                "dest_ip": null,
                "dest_port": null,
                "icmp_type": null,
                "ipset": null,
                "helper": null,
                "mark": null,
                "dscp": null,
                "limit": null,
                "limit_burst": null,
                "weekdays": null,
                "start_date": null,
                "stop_date": null,
                "start_time": null,
                "stop_time": null,
                "utc_time": null,
                "set_helper": null,
                "set_mark": null,
                "set_xmark": null,
                "set_dscp": null,
                // Logging and the rate it may write at: the rate is written only
                // while the switch is on, so it is cleared with it. Left out of
                // OWNED it could be written once and never taken away.
                "log": null,
                "log_limit": null
            })
        );
        // An option firewall4 knows and this editor does not draw is not its to
        // clear. `log_limit` used to be on this line, which was the bug: the log
        // gate draws it, so a rate once written could never be taken away again.
        let cleared = values(&body);
        assert!(cleared.get("extra").is_none());
    }

    #[test]
    fn a_saved_condition_is_written_the_way_firewall4_reads_it() {
        let body = submit(
            "everything",
            &[
                ("enabled", "1"),
                ("src", "guest"),
                ("target", "ACCEPT"),
                ("counter", "1"),
                ("src_ip", "10.0.20.0/24"),
                ("src_ip_not", "10.0.20.5"),
                ("src_ip_not", "10.0.20.6"),
                ("dest_port", "53"),
                ("limit_match", "over"),
                ("limit", "1000"),
                ("limit_unit", "minute"),
                ("limit_burst", "5"),
                ("mark_match", "exclude"),
                ("mark_value", "0x10"),
                ("mark_mask", "0xff"),
                ("weekdays", "Mon"),
                ("start_time", "08:00"),
                ("time_basis", "utc"),
                ("log", "1"),
                ("log_prefix", "guest-audit"),
            ],
        );
        let values = values(&body);
        assert_eq!(
            values["src_ip"],
            serde_json::json!(["10.0.20.0/24", "!10.0.20.5", "!10.0.20.6"]),
            "an exclusion is the same list option with fw4's inversion on each value"
        );
        assert_eq!(values["dest_port"], serde_json::json!(["53"]));
        assert_eq!(values["limit"], "!1000/minute");
        assert_eq!(values["limit_burst"], "5");
        assert_eq!(values["mark"], "!0x10/0xff");
        // One value, never a uci list: firewall4 splits the string itself and
        // refuses a list outright, skipping the whole rule.
        assert_eq!(values["weekdays"], "Mon");
        assert_eq!(values["start_time"], "08:00");
        assert_eq!(values["utc_time"], "1");
        assert_eq!(values["log"], "guest-audit");
    }

    #[test]
    fn only_the_chosen_actions_parameters_are_written() {
        let mark = submit(
            "everything",
            &[
                ("enabled", "1"),
                ("src", "guest"),
                ("target", "MARK"),
                ("mark_operation", "set"),
                ("set_mark", "0x20"),
                ("set_dscp", "EF"),
                ("set_helper", "ftp"),
                ("counter", "1"),
            ],
        );
        assert_eq!(values(&mark)["set_mark"], "0x20");
        assert_eq!(values(&mark)["set_xmark"], Value::Null);
        assert_eq!(values(&mark)["set_dscp"], Value::Null);
        assert_eq!(values(&mark)["set_helper"], Value::Null);

        let helper = submit(
            "everything",
            &[
                ("enabled", "1"),
                ("src", "guest"),
                ("target", "HELPER"),
                ("set_helper", "ftp"),
                ("set_mark", "0x20"),
                ("counter", "1"),
            ],
        );
        assert_eq!(values(&helper)["set_helper"], "ftp");
        assert_eq!(values(&helper)["set_mark"], Value::Null);
    }

    #[test]
    fn a_switch_the_submission_left_out_turns_its_option_off() {
        let body = submit("everything", &[("src", "guest"), ("target", "ACCEPT")]);
        assert_eq!(values(&body)["enabled"], "0");
        assert_eq!(values(&body)["counter"], "0");
    }

    /// Refusal is one case of a submission firewall4 would not accept: the field
    /// that carries the offending value, and where that control reports it — a
    /// single control on its own `error`, a repeating one per item in `errors`.
    struct Refusal {
        case: &'static str,
        field: (&'static str, &'static str),
        control: &'static str,
        reported_in: &'static str,
    }

    #[test]
    fn a_value_firewall4_would_refuse_is_marked_and_nothing_is_written() {
        let cases = [
            Refusal {
                case: "a port range that runs backwards",
                field: ("dest_port", "99-22"),
                control: "dest_port",
                reported_in: "errors",
            },
            Refusal {
                case: "a date that is not one",
                field: ("start_date", "2026-13-40"),
                control: "start_date",
                reported_in: "error",
            },
            Refusal {
                case: "a zone this config does not define",
                field: ("src", "nowhere"),
                control: "src",
                reported_in: "error",
            },
        ];
        for refusal in cases {
            // The offending value leads, so a case that overrides one of the
            // rule's own fields is the value the form reads.
            let body = submit(
                "everything",
                &[refusal.field, ("src", "guest"), ("target", "ACCEPT")],
            );
            let case = refusal.case;
            assert!(
                body.get("commit").is_none(),
                "{case}: nothing may be written"
            );
            assert_eq!(body["notice"]["level"], "danger", "{case}");
            let marked = control(&body, refusal.control);
            assert!(
                marked.get(refusal.reported_in).is_some(),
                "{case}: {} carries no error: {marked}",
                refusal.control
            );
        }
    }

    #[test]
    fn a_refused_port_is_marked_on_the_item_that_is_wrong() {
        let body = submit(
            "everything",
            &[
                ("src", "guest"),
                ("target", "ACCEPT"),
                ("dest_port", "53"),
                ("dest_port", "99-22"),
            ],
        );
        assert_eq!(
            control(&body, "dest_port")["errors"],
            serde_json::json!({
                "1": "Write a port from 0 to 65535, or a range such as 1024-65535."
            }),
            "the second item is the wrong one, and only it is marked"
        );
        // The submitted values come back, so nothing an operator typed is lost.
        assert_eq!(
            control(&body, "dest_port")["items"],
            serde_json::json!(["53", "99-22"])
        );
    }

    #[test]
    fn deleting_a_rule_answers_with_the_listing_it_is_leaving() {
        let mut model = fixture::firewall();
        let envelope = crate::rules::save(
            &fixture::snapshot(),
            &mut model,
            &Counters::default(),
            &Form::parse("open=allow_ping"),
            &Form::parse("_delete=1"),
        );
        let body = serde_json::to_value(&envelope).expect("serialize");

        assert_eq!(body["title"], crate::rules::HEADING);
        assert_eq!(
            body["notice"],
            serde_json::json!({"level": "success", "text": "Rule deleted."})
        );
        assert_eq!(
            body["commit"],
            serde_json::json!([{
                "config": "firewall",
                "section": "allow_ping",
                "delete": true
            }])
        );
        // The listing answers with the rule already gone.
        let rows = fixture::listing(&body)["rows"]
            .as_array()
            .expect("rows")
            .clone();
        assert!(rows.iter().all(|row| row["id"] != "allow_ping"));
    }

    #[test]
    fn editing_a_rule_keeps_removal_on_the_listing() {
        for body in [open("everything"), opened_new()] {
            let drawer = fixture::find_with(&body, &|value| {
                value["open"] == true && value["children"].is_array()
            })
            .expect("the opened drawer");
            assert!(
                fixture::find_with(&drawer, &|value| value["type"] == "confirm"
                    || value["name"] == DELETE_FIELD
                    || value["name"] == fields::REMOVE_FIELD)
                .is_none(),
                "an edit drawer only edits; removal belongs to its row action"
            );
        }
    }
}
