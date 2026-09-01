// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

//! The rule editor: one rule, on a page of its own.
//!
//! A firewall rule is not one fact. It is a path, a protocol, a verdict, and any
//! number of optional matches, and the operator has to see all of it at once to
//! know what the rule does — so it gets a page rather than a panel beside the
//! listing it came from.
//!
//! The form is the page's, not its own: it carries no Save button, and the
//! shell's staged-changes bar submits it and applies the result. The editor
//! therefore answers a submission the same way it answers a visit — with itself,
//! re-rendered from what was submitted — and states the outcome in the notice.
//! A value firewall4 would refuse is reported on the control that carries it,
//! and nothing is written.
//!
//! Deleting is deliberately not part of that form. It is a second, small form
//! below it, behind a confirmation, so the one action that cannot be undone is
//! never a keystroke away from the one that can.

use verso_plugin::{
    commit, commit_delete, commit_new, Envelope, Form, SelectOption, Snapshot, Tone, Widget,
};

use crate::conditions;
use crate::counters::Counters;
use crate::model::{Firewall, CONFIG};
use crate::page;
use crate::rule_form::{Errors, RuleForm, DSCP_CLASSES, HELPERS, TARGETS};
use crate::rules;

/// DELETE_FIELD marks the submission of the delete form rather than the editor's.
pub const DELETE_FIELD: &str = "_delete";

const NEW_SUBHEADING: &str =
    "Say which traffic this rule is about, and what the router should do with it.";

const CONDITIONS_SUB: &str = "Everything a rule can match beyond its path and protocol. \
A rule with none of these matches all the traffic on the path above.";

const ADVANCED_NOTE: &str = "Only the parameters the chosen action needs are written; \
the rest are left out of the rule.";

/// edit answers a visit to one rule's editor, or nothing when the sub-path names
/// no rule this config holds.
pub fn edit(snapshot: &Snapshot, model: &Firewall, section: &str) -> Option<Envelope> {
    let rule = model.rule(section)?;
    let uci = snapshot.section(CONFIG, section)?;
    let form = RuleForm::read(&uci);
    Some(page(model, Some(section), &form, &Errors::default()).with_subheading(&heading(&rule.name)))
}

/// blank answers a visit to the new-rule editor: firewall4's own defaults, so
/// what the operator starts from is what the packet filter would assume.
pub fn blank(model: &Firewall) -> Envelope {
    page(model, None, &RuleForm::default(), &Errors::default()).with_subheading(NEW_SUBHEADING)
}

/// create answers the new-rule editor's submission. A rule that would not survive
/// firewall4's own parser is refused with the offending controls marked; one that
/// would is stated as the section to add.
pub fn create(model: &Firewall, form: &Form) -> Envelope {
    let rule = RuleForm::submitted(form);
    let errors = rule.validate(&model.zone_names());
    let answer = page(model, None, &rule, &errors).with_subheading(NEW_SUBHEADING);
    if !errors.is_empty() {
        return answer.with_notice(Tone::Danger, REFUSED);
    }
    answer
        .with_notice(Tone::Success, "Rule added.")
        .with_commit(vec![commit_new(CONFIG, "rule", rule.values(false))])
}

/// save answers one rule's editor. A body carrying the delete marker is the
/// second form below the editor, and answers with the listing the rule is
/// leaving; anything else is the editor's own submission.
pub fn save(
    model: &mut Firewall,
    counters: &Counters,
    section: &str,
    form: &Form,
) -> Option<Envelope> {
    let index = model.rules.iter().position(|rule| rule.section == section)?;
    if form.get(DELETE_FIELD) == "1" {
        let removed = model.rules.remove(index);
        return Some(
            rules::page(model, counters)
                .with_notice(Tone::Success, "Rule deleted.")
                .with_commit(vec![commit_delete(CONFIG, &removed.section)]),
        );
    }

    let rule = RuleForm::submitted(form);
    let errors = rule.validate(&model.zone_names());
    let answer =
        page(model, Some(section), &rule, &errors).with_subheading(&heading(&rule.name));
    if !errors.is_empty() {
        return Some(answer.with_notice(Tone::Danger, REFUSED));
    }
    Some(
        answer
            .with_notice(Tone::Success, "Rule saved.")
            .with_commit(vec![commit(CONFIG, section, rule.values(true))]),
    )
}

const REFUSED: &str =
    "Some values aren’t ones the firewall accepts, so nothing was saved. They’re marked below.";

/// heading names the rule the page is about, falling back to its own words when
/// the rule carries no name — every rule has an identity, but not every one has
/// a name an operator wrote.
fn heading(name: &str) -> String {
    match name.is_empty() {
        true => "An unnamed rule.".to_string(),
        false => name.to_string(),
    }
}

/// page composes the editor. `section` is the rule being edited, or None for a
/// new one — which is also what decides whether the delete form is there at all.
fn page(model: &Firewall, section: Option<&str>, rule: &RuleForm, errors: &Errors) -> Envelope {
    let title = match section {
        Some(_) => "Edit rule",
        None => "New rule",
    };
    let mut children = vec![Widget::Form {
        style: "page".into(),
        submit: String::new(),
        error: String::new(),
        fields: vec![
            identity(rule, errors),
            traffic_path(model, rule, errors),
            Widget::section("", CONDITIONS_SUB, vec![conditions::widget(rule, errors, model)]),
            action(rule, errors),
            observability(rule, errors),
        ],
    }];
    if section.is_some() {
        children.push(rule_delete_form(rule));
    }
    Envelope::page(title, Widget::stack(children))
        .with_width("normal")
        .with_pages(page::tabs())
}

/// identity is the rule's name and whether it is live at all. The switch rides
/// the heading rather than the body: it is the state of the whole object, not
/// one of its settings.
fn identity(rule: &RuleForm, errors: &Errors) -> Widget {
    Widget::Section {
        title: "Rule".into(),
        sub: String::new(),
        meta: String::new(),
        meta_icon: String::new(),
        meta_position: String::new(),
        mode: String::new(),
        flush: true,
        control: Some(Box::new(Widget::Switch {
            name: "enabled".into(),
            label: "Enabled".into(),
            off_label: "Disabled".into(),
            help: String::new(),
            style: "inline".into(),
            icon: String::new(),
            meta: String::new(),
            on: rule.enabled,
        })),
        children: vec![form_grid(
            2,
            vec![text_field(
                "name",
                "Name",
                &rule.name,
                "Identifies the rule in listings, hit counts, and the system log.",
                errors,
            )],
        )],
    }
}

/// traffic_path is where the traffic comes from and goes to. Both ends are a
/// closed choice over the zones this config defines, plus the router itself and
/// anywhere at all — a zone name typed by hand and misspelled is a rule that
/// matches nothing and says nothing about it.
fn traffic_path(model: &Firewall, rule: &RuleForm, errors: &Errors) -> Widget {
    let ends = |current: &str| {
        let mut choices = vec![
            SelectOption::new("", "The router itself"),
            SelectOption::new("*", "Anywhere"),
        ];
        choices.extend(conditions::named(&model.zone_names()));
        if !current.is_empty() && current != "*" && !model.zone_names().iter().any(|z| z == current)
        {
            choices.push(SelectOption::new(current, current));
        }
        choices
    };
    Widget::section(
        "Traffic path",
        "",
        vec![
            form_grid(
                2,
                vec![
                    select_field("src", "From", &rule.src, ends(&rule.src), errors),
                    select_field("dest", "To", &rule.dest, ends(&rule.dest), errors),
                ],
            ),
            // Most rules are about a path and a protocol and never name an address
            // family: fw4 then matches both, which is what an operator means. So the
            // control belongs to the advanced reading — but only while it sits at
            // that default. A rule someone narrowed to one family carries live
            // state, and state is visible to every reader whatever mode they are in
            // (ADR-015 §4): the plugin knows the default, so the plugin decides.
            select_field(
                "family",
                "Address family",
                &rule.family,
                conditions::options(&[
                    ("", "IPv4 and IPv6"),
                    ("ipv4", "IPv4 only"),
                    ("ipv6", "IPv6 only"),
                ]),
                errors,
            )
            .advanced_when(rule.family.is_empty()),
            token_list(
                "proto",
                "Protocols",
                "Protocol name or number",
                "Each protocol is an alternative. Names and IP protocol numbers are both accepted.",
                &rule.proto,
                errors,
            ),
        ],
    )
}

/// action is the verdict and the parameters the less common verdicts need. The
/// parameters fold away because most rules accept, reject, or drop, and those
/// need none of them.
fn action(rule: &RuleForm, errors: &Errors) -> Widget {
    let mark_operation = match rule.mark_xor {
        true => "xor",
        false => "set",
    };
    Widget::section(
        "Action",
        "",
        vec![
            select_field(
                "target",
                "When the rule matches",
                &rule.target,
                conditions::options(&TARGETS),
                errors,
            ),
            Widget::Disclosure {
                style: "condition".into(),
                summary: "Parameters for the less common actions".into(),
                children: vec![
                    Widget::Callout {
                        variant: Tone::Neutral,
                        title: String::new(),
                        body: ADVANCED_NOTE.into(),
                        compact: true,
                    },
                    select_field(
                        "set_helper",
                        "Connection helper to assign",
                        &rule.set_helper,
                        conditions::named(&HELPERS.map(String::from)),
                        errors,
                    ),
                    form_grid(
                        3,
                        vec![
                            select_field(
                                "mark_operation",
                                "Mark operation",
                                mark_operation,
                                conditions::options(&[("set", "Set"), ("xor", "XOR")]),
                                errors,
                            ),
                            text_field("set_mark", "Mark value", &rule.set_mark, "", errors),
                            text_field(
                                "set_mark_mask",
                                "Mark mask",
                                &rule.set_mark_mask,
                                "Optional.",
                                errors,
                            ),
                        ],
                    ),
                    select_field(
                        "set_dscp",
                        "DSCP value to apply",
                        &rule.set_dscp,
                        conditions::named(&DSCP_CLASSES.map(String::from)),
                        errors,
                    ),
                ],
            },
        ],
    )
}

/// observability is what the rule leaves behind when it matches: the kernel hit
/// counters the listing reads, and the system-log line.
fn observability(rule: &RuleForm, errors: &Errors) -> Widget {
    Widget::section(
        "Observability",
        "",
        vec![Widget::Stack {
            width: String::new(),
            compact: true,
            inline: false,
            divided: false,
            children: vec![
                Widget::Switch {
                    name: "counter".into(),
                    label: "Count matching packets".into(),
                    off_label: String::new(),
                    help: "Turning counting off empties this rule's Hits column.".into(),
                    style: String::new(),
                    icon: String::new(),
                    meta: String::new(),
                    on: rule.counter,
                },
                Widget::Conditional {
                    name: "log".into(),
                    label: "Log matching packets".into(),
                    checked: rule.log.on,
                    fields: vec![form_grid(
                        2,
                        vec![
                            text_field(
                                "log_prefix",
                                "Log prefix",
                                &rule.log.prefix,
                                "Optional; the rule's name is used when this is blank.",
                                errors,
                            ),
                            text_field(
                                "log_limit",
                                "Log rate limit",
                                &rule.log.limit,
                                "For example 10/minute. Protects the system log from bursts.",
                                errors,
                            ),
                        ],
                    )],
                    otherwise: Vec::new(),
                },
            ],
        }],
    )
}

/// rule_delete_form is the rule editor's one irreversible action.
fn rule_delete_form(rule: &RuleForm) -> Widget {
    delete_form(
        "Delete rule",
        &format!(
            "Delete {}? Traffic it allowed will be decided by whatever rule or zone policy \
             comes next.",
            subject(&rule.name, "this rule")
        ),
    )
}

/// delete_form is an editor's one irreversible action, kept in a form of its own
/// so the page form above it can never carry it by accident. The confirm holds
/// the submit, so this form draws no Save of its own.
pub fn delete_form(action: &str, message: &str) -> Widget {
    Widget::Form {
        style: String::new(),
        submit: String::new(),
        error: String::new(),
        fields: vec![
            Widget::hidden(DELETE_FIELD, "1"),
            Widget::Confirm {
                trigger: action.into(),
                message: message.into(),
                confirm: action.into(),
                cancel: String::new(),
            },
        ],
    }
}

/// subject names the thing an editor is about to delete: what its operator
/// called it, or what kind of thing it is when nobody named it.
pub fn subject(name: &str, unnamed: &str) -> String {
    match name.is_empty() {
        true => unnamed.to_string(),
        false => format!("“{name}”"),
    }
}

// ---- the controls the editor and its conditions share ----

/// form_grid lays a run of controls across columns at the tighter gutter a form
/// takes.
pub fn form_grid(columns: u32, children: Vec<Widget>) -> Widget {
    Widget::Grid {
        style: "form".into(),
        columns,
        children,
    }
}

/// text_field is one typed value, carrying whatever the last submission got
/// wrong about it.
pub fn text_field(name: &str, label: &str, value: &str, help: &str, errors: &Errors) -> Widget {
    Widget::Field {
        name: name.into(),
        label: label.into(),
        kind: "text".into(),
        advanced: false,
        value: value.into(),
        values: Vec::new(),
        placeholder: String::new(),
        datatype: String::new(),
        options: Vec::new(),
        error: errors.get(name).into(),
        help: help.into(),
    }
}

/// select_field is one choice over a closed set.
pub fn select_field(
    name: &str,
    label: &str,
    value: &str,
    options: Vec<SelectOption>,
    errors: &Errors,
) -> Widget {
    Widget::select(name, label, value, options, errors.get(name))
}

/// token_list is many short values under one name, each removable on its own and
/// each able to report its own error.
pub fn token_list(
    name: &str,
    label: &str,
    prompt: &str,
    help: &str,
    items: &[String],
    errors: &Errors,
) -> Widget {
    Widget::List {
        name: name.into(),
        label: label.into(),
        kind: "text".into(),
        style: "tokens".into(),
        prompt: prompt.into(),
        datatype: String::new(),
        items: items.to_vec(),
        errors: errors.list(name),
        help: help.into(),
    }
}

/// missing states that the sub-path names no rule — a stale link, or a rule
/// someone else removed — and answers with the listing, which is somewhere real.
pub fn missing(model: &Firewall, counters: &Counters) -> Envelope {
    rules::page(model, counters).with_notice(
        Tone::Danger,
        "That rule isn’t here any more, so Verso showed you the rules instead.",
    )
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::fixture;
    use serde_json::Value;

    fn open(section: &str) -> Value {
        let snapshot = fixture::editor_snapshot();
        let model = fixture::editor_firewall();
        let envelope = edit(&snapshot, &model, section).expect("the fixture holds this rule");
        serde_json::to_value(&envelope).expect("serialize")
    }

    fn submit(section: &str, fields: &[(&str, &str)]) -> Value {
        let mut model = fixture::editor_firewall();
        let envelope = save(
            &mut model,
            &Counters::default(),
            section,
            &Form::parse(&encode(fields)),
        )
        .expect("the fixture holds this rule");
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

    /// conditions returns the catalogue entries by key.
    fn conditions(body: &Value) -> Value {
        body["widget"]["children"][0]["fields"][2]["children"][0].clone()
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
    fn the_editor_is_a_page_form_the_capsule_saves() {
        let body = open("everything");
        assert_eq!(body["title"], "Edit rule");
        assert_eq!(body["subheading"], "Everything");
        assert_eq!(body["width"], "normal");
        assert_eq!(body["pages"][0]["label"], "Rules");
        let form = &body["widget"]["children"][0];
        assert_eq!(form["type"], "form");
        assert_eq!(form["style"], "page");
        assert!(
            form.get("submit").is_none(),
            "the staged-changes bar owns saving, so the form declares no button"
        );
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
        assert_eq!(control(&body, "dest_port")["items"], serde_json::json!(["53"]));
        assert_eq!(
            control(&body, "dest_port_not")["items"],
            serde_json::json!(["5353"])
        );
        assert_eq!(control(&body, "src_port")["items"], serde_json::json!(["1024-65535"]));
        assert_eq!(
            control(&body, "icmp_type")["items"],
            serde_json::json!(["echo-request"])
        );
        assert_eq!(control(&body, "proto")["items"], serde_json::json!(["tcp", "udp"]));
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
        assert_eq!(control(&body, "start_date")["value"], "2026-01-01");
        assert_eq!(control(&body, "stop_time")["value"], "18:00:00");
        assert_eq!(control(&body, "time_basis")["value"], "utc");
    }

    #[test]
    fn the_action_and_its_parameters_read_as_one_verdict() {
        let body = open("everything");
        assert_eq!(control(&body, "target")["value"], "MARK");
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
        let body = open("everything");
        assert!(!switched_on(&body, "enabled"));
        assert!(!switched_on(&body, "counter"));
        assert_eq!(control(&body, "log")["checked"], true);
        assert_eq!(control(&body, "log_prefix")["value"], "guest-audit: ");
        assert_eq!(control(&body, "log_limit")["value"], "10/minute");

        // A rule stating none of them runs on firewall4's defaults: on, counted,
        // unlogged.
        let plain = open("plain");
        assert!(switched_on(&plain, "enabled"));
        assert!(switched_on(&plain, "counter"));
        assert_eq!(control(&plain, "log")["checked"], false);
    }

    /// The reference case for "mode hides capability, never state" (ADR-015 §4):
    /// the address family is part of the advanced reading while it sits at fw4's
    /// default, and the moment a rule narrows it the tag is gone — so a basic
    /// reader is never shown a rule that hides what it actually does.
    #[test]
    fn the_address_family_hides_only_while_it_is_at_its_default() {
        let untouched = open("plain");
        assert_eq!(
            control(&untouched, "family")["advanced"],
            true,
            "a rule matching both families keeps the control out of the basic reading"
        );

        let narrowed = open("everything");
        assert_eq!(control(&narrowed, "family")["value"], "ipv4");
        assert!(
            control(&narrowed, "family").get("advanced").is_none(),
            "a rule narrowed to one family states that in every reading"
        );

        // A submission that drops the family puts the control back in the advanced
        // reading, since the rule is back at the default it started from.
        let cleared = submit(
            "everything",
            &[("src", "guest"), ("target", "ACCEPT"), ("counter", "1")],
        );
        assert_eq!(control(&cleared, "family")["advanced"], true);

        // Nothing else on the path is hidden: the essentials are the basic reading.
        for essential in ["src", "dest", "proto", "target", "name"] {
            assert!(
                control(&untouched, essential).get("advanced").is_none(),
                "{essential} is one of the fields a rule needs to work"
            );
        }
    }

    #[test]
    fn a_new_rule_starts_where_firewall4s_own_defaults_are() {
        let body = serde_json::to_value(blank(&fixture::editor_firewall())).expect("serialize");
        assert_eq!(body["title"], "New rule");
        assert_eq!(control(&body, "enabled")["on"], true);
        assert_eq!(control(&body, "counter")["on"], true);
        assert_eq!(control(&body, "target")["value"], "ACCEPT");
        assert_eq!(control(&body, "proto")["items"], serde_json::json!(["tcp", "udp"]));
        assert_eq!(control(&body, "name")["value"], "");
        assert_eq!(body["subheading"], NEW_SUBHEADING);
        assert!(
            body["widget"]["children"].as_array().expect("children").len() == 1,
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

    #[test]
    fn creating_a_rule_states_only_the_options_it_sets() {
        let model = fixture::editor_firewall();
        let body = serde_json::to_value(create(
            &model,
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
                "log": null
            })
        );
        // An option firewall4 knows and this editor does not draw is not its to
        // clear.
        let cleared = values(&body);
        assert!(cleared.get("extra").is_none());
        assert!(cleared.get("log_limit").is_none());
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
        assert_eq!(values["weekdays"], serde_json::json!(["Mon"]));
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
        let body = submit(
            "everything",
            &[("src", "guest"), ("target", "ACCEPT")],
        );
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
            assert!(body.get("commit").is_none(), "{case}: nothing may be written");
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
        let envelope = save(
            &mut model,
            &Counters::default(),
            "allow_ping",
            &Form::parse("_delete=1"),
        )
        .expect("the fixture holds this rule");
        let body = serde_json::to_value(&envelope).expect("serialize");

        assert_eq!(body["title"], "Firewall");
        assert_eq!(body["notice"], serde_json::json!({"level": "success", "text": "Rule deleted."}));
        assert_eq!(
            body["commit"],
            serde_json::json!([{
                "config": "firewall",
                "section": "allow_ping",
                "delete": true
            }])
        );
        // The listing answers with the rule already gone.
        let rows = fixture::table(&body, "Traffic rules")["rows"]
            .as_array()
            .expect("rows")
            .clone();
        assert!(rows.iter().all(|row| row["id"] != "allow_ping"));
    }

    #[test]
    fn a_sub_path_naming_no_rule_answers_with_the_listing() {
        let snapshot = fixture::snapshot();
        let model = fixture::firewall();
        assert!(edit(&snapshot, &model, "no_such_rule").is_none());

        let mut model = fixture::firewall();
        assert!(save(
            &mut model,
            &Counters::default(),
            "no_such_rule",
            &Form::parse("target=ACCEPT")
        )
        .is_none());

        // A redirect is a section, but it is not a rule this editor edits.
        let model = fixture::firewall();
        assert!(edit(&snapshot, &model, "https_to_nas").is_none());

        let body = serde_json::to_value(missing(&model, &Counters::default())).expect("serialize");
        assert_eq!(body["notice"]["level"], "danger");
        assert_eq!(fixture::section(&body, "Traffic rules")["title"], "Traffic rules");
    }

    #[test]
    fn only_an_existing_rule_offers_to_delete_itself() {
        let body = open("everything");
        let delete = &body["widget"]["children"][1];
        assert_eq!(delete["type"], "form");
        assert!(
            delete.get("submit").is_none(),
            "the confirm carries the submit, so the form draws no Save"
        );
        assert_eq!(delete["fields"][0]["name"], DELETE_FIELD);
        assert_eq!(delete["fields"][0]["value"], "1");
        assert_eq!(delete["fields"][1]["type"], "confirm");
    }
}
