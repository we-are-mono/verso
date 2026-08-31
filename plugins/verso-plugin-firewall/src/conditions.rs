// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

//! The optional matches a rule may carry, as the editor offers them.
//!
//! A rule's zones, protocols and verdict are always there; everything else is a
//! condition it either has or has not. Declaring the whole catalogue and marking
//! which entries the rule carries is what keeps the editor complete without
//! presenting a wall of empty controls: the shell renders the carried ones and
//! keeps the rest behind its Add-condition picker.
//!
//! Only firewall4's own rule options appear here. A condition it cannot express
//! would be a control that changes nothing, which is worse than a control that
//! is missing.

use verso_plugin::{ConditionItem, SelectOption, Widget};

use crate::editor::{form_grid, select_field, text_field, token_list};
use crate::model::Firewall;
use crate::rule_form::{
    Errors, RuleForm, DSCP_CLASSES, EXCLUDE, HELPERS, INCLUDE, RATE_UNITS, SET_FIELDS, WEEKDAYS,
};

const HELP: &str =
    "Conditions are combined with and. Multiple values inside one condition are alternatives.";

/// widget is the condition catalogue for one rule. An entry the config cannot
/// support — a named set on a router that declares none — is left out entirely
/// rather than offered empty.
pub fn widget(rule: &RuleForm, errors: &Errors, model: &Firewall) -> Widget {
    let mut items = vec![
        device(rule, errors, model),
        addresses(
            "src_ip",
            "Source addresses",
            "Match where the traffic comes from.",
            &rule.src_ip.include,
            &rule.src_ip.exclude,
            rule.active("src_ip"),
            errors,
        ),
        macs(rule, errors),
        ports(
            "src_port",
            "Source ports",
            "Match the port the traffic was sent from.",
            &rule.src_port.include,
            &rule.src_port.exclude,
            rule.active("src_port"),
            errors,
        ),
        addresses(
            "dest_ip",
            "Destination addresses",
            "Match where the traffic is going.",
            &rule.dest_ip.include,
            &rule.dest_ip.exclude,
            rule.active("dest_ip"),
            errors,
        ),
        ports(
            "dest_port",
            "Destination ports",
            "Match the port the traffic is going to.",
            &rule.dest_port.include,
            &rule.dest_port.exclude,
            rule.active("dest_port"),
            errors,
        ),
        icmp_types(rule, errors),
    ];
    if let Some(item) = named_set(rule, errors, model) {
        items.push(item);
    }
    items.extend([
        helper(rule, errors),
        mark(rule, errors),
        dscp(rule, errors),
        rate(rule, errors),
        schedule(rule, errors),
    ]);
    Widget::Conditions {
        label: "Conditions".into(),
        help: HELP.into(),
        items,
    }
}

fn item(key: &str, label: &str, help: &str, active: bool, children: Vec<Widget>) -> ConditionItem {
    ConditionItem {
        key: key.into(),
        label: label.into(),
        help: help.into(),
        active,
        children,
    }
}

/// device ties the rule to one kernel device. The choices are the devices the
/// network config names — the only ones this plugin can know — plus whatever the
/// rule already names, so editing a rule never silently retargets it.
fn device(rule: &RuleForm, errors: &Errors, model: &Firewall) -> ConditionItem {
    let mut names = model.devices();
    if !rule.device.name.is_empty() && !names.contains(&rule.device.name) {
        names.insert(0, rule.device.name.clone());
    }
    item(
        "device",
        "Network device",
        "Tie the rule to one incoming or outgoing kernel device.",
        rule.active("device"),
        vec![form_grid(
            2,
            vec![
                select_field(
                    "direction",
                    "Direction",
                    &rule.device.direction,
                    options(&[("in", "Incoming"), ("out", "Outgoing")]),
                    errors,
                ),
                select_field("device", "Device", &rule.device.name, named(&names), errors),
            ],
        )],
    )
}

fn addresses(
    key: &str,
    label: &str,
    help: &str,
    include: &[String],
    exclude: &[String],
    active: bool,
    errors: &Errors,
) -> ConditionItem {
    item(
        key,
        label,
        help,
        active,
        vec![form_grid(
            2,
            vec![
                token_list(
                    key,
                    "Include",
                    "Address or network",
                    "IP addresses, or networks in CIDR form.",
                    include,
                    errors,
                ),
                token_list(
                    &format!("{key}_not"),
                    "Exclude",
                    "Address or network",
                    "Everything else in the include list still matches.",
                    exclude,
                    errors,
                ),
            ],
        )],
    )
}

fn ports(
    key: &str,
    label: &str,
    help: &str,
    include: &[String],
    exclude: &[String],
    active: bool,
    errors: &Errors,
) -> ConditionItem {
    item(
        key,
        label,
        help,
        active,
        vec![form_grid(
            2,
            vec![
                token_list(
                    key,
                    "Include",
                    "Port or range",
                    "For example 53, or 1024-65535.",
                    include,
                    errors,
                ),
                token_list(
                    &format!("{key}_not"),
                    "Exclude",
                    "Port or range",
                    "",
                    exclude,
                    errors,
                ),
            ],
        )],
    )
}

fn macs(rule: &RuleForm, errors: &Errors) -> ConditionItem {
    item(
        "src_mac",
        "Source MAC addresses",
        "Match link-layer senders visible on the way in.",
        rule.active("src_mac"),
        vec![form_grid(
            2,
            vec![
                token_list(
                    "src_mac",
                    "Include",
                    "MAC address",
                    "",
                    &rule.src_mac.include,
                    errors,
                ),
                token_list(
                    "src_mac_not",
                    "Exclude",
                    "MAC address",
                    "",
                    &rule.src_mac.exclude,
                    errors,
                ),
            ],
        )],
    )
}

fn icmp_types(rule: &RuleForm, errors: &Errors) -> ConditionItem {
    item(
        "icmp_type",
        "ICMP types",
        "Narrow an ICMP rule to the kinds of message it is about.",
        rule.active("icmp_type"),
        vec![token_list(
            "icmp_type",
            "Types",
            "Type name or type/code",
            "Examples: echo-request, neighbour-solicitation, or 130/0.",
            &rule.icmp_type,
            errors,
        )],
    )
}

/// named_set matches one of the firewall config's own sets. A router with no
/// `config ipset` has nothing to match against, so the condition is not offered
/// there — matching a set that does not exist is a rule firewall4 skips.
fn named_set(rule: &RuleForm, errors: &Errors, model: &Firewall) -> Option<ConditionItem> {
    let mut sets = model.ipsets.clone();
    if !rule.ipset.set.value.is_empty() && !sets.contains(&rule.ipset.set.value) {
        sets.insert(0, rule.ipset.set.value.clone());
    }
    if sets.is_empty() {
        return None;
    }
    let field = |slot: usize, label: &str, optional: bool| {
        let mut choices = options(&SET_FIELDS);
        if optional {
            choices.insert(0, SelectOption::new("", "Not used"));
        }
        select_field(
            &format!("ipset_field_{}", slot + 1),
            label,
            &rule.ipset.fields[slot],
            choices,
            errors,
        )
    };
    Some(item(
        "ipset",
        "Named firewall set",
        "Match a set declared in this firewall config, and say which packet fields its entries describe.",
        rule.active("ipset"),
        vec![
            form_grid(
                2,
                vec![
                    select_field("ipset", "Set", &rule.ipset.set.value, named(&sets), errors),
                    comparison("ipset_match", &rule.ipset.set, "Is in the set", "Is not in the set"),
                ],
            ),
            form_grid(
                3,
                vec![
                    field(0, "First field", false),
                    field(1, "Second field", true),
                    field(2, "Third field", true),
                ],
            ),
        ],
    ))
}

fn helper(rule: &RuleForm, errors: &Errors) -> ConditionItem {
    item(
        "helper",
        "Connection helper",
        "Match traffic a connection helper is already tracking.",
        rule.active("helper"),
        vec![form_grid(
            2,
            vec![
                comparison(
                    "helper_match",
                    &rule.helper,
                    "Uses this helper",
                    "Does not use this helper",
                ),
                select_field(
                    "helper",
                    "Helper",
                    &rule.helper.value,
                    named(&HELPERS.map(String::from)),
                    errors,
                ),
            ],
        )],
    )
}

fn mark(rule: &RuleForm, errors: &Errors) -> ConditionItem {
    item(
        "mark",
        "Firewall mark",
        "Match a mark another rule already set on the packet.",
        rule.active("mark"),
        vec![form_grid(
            3,
            vec![
                comparison("mark_match", &rule.mark.mark, "Equals", "Does not equal"),
                text_field(
                    "mark_value",
                    "Value",
                    &rule.mark.mark.value,
                    "Decimal, or hexadecimal with 0x.",
                    errors,
                ),
                text_field("mark_mask", "Mask", &rule.mark.mask, "Optional.", errors),
            ],
        )],
    )
}

fn dscp(rule: &RuleForm, errors: &Errors) -> ConditionItem {
    item(
        "dscp",
        "DSCP value",
        "Match the traffic class another device or rule already marked the packet with.",
        rule.active("dscp"),
        vec![form_grid(
            2,
            vec![
                comparison("dscp_match", &rule.dscp, "Equals", "Does not equal"),
                select_field(
                    "dscp",
                    "DSCP",
                    &rule.dscp.value,
                    dscp_options(),
                    errors,
                ),
            ],
        )],
    )
}

fn rate(rule: &RuleForm, errors: &Errors) -> ConditionItem {
    let matching = match rule.rate.over {
        true => "over",
        false => "below",
    };
    item(
        "rate",
        "Rate limit",
        "Match only while the traffic stays under a rate — or only once it goes over.",
        rule.active("rate"),
        vec![
            form_grid(
                2,
                vec![
                    select_field(
                        "limit_match",
                        "Match",
                        matching,
                        options(&[
                            ("below", "At or below the rate"),
                            ("over", "Over the rate"),
                        ]),
                        errors,
                    ),
                    text_field("limit", "Packets", &rule.rate.count, "", errors),
                ],
            ),
            form_grid(
                2,
                vec![
                    select_field("limit_unit", "Per", &rule.rate.unit, options(&RATE_UNITS), errors),
                    text_field(
                        "limit_burst",
                        "Initial burst",
                        &rule.rate.burst,
                        "Optional packet allowance before the rate applies.",
                        errors,
                    ),
                ],
            ),
        ],
    )
}

fn schedule(rule: &RuleForm, errors: &Errors) -> ConditionItem {
    let basis = match rule.schedule.utc {
        true => "utc",
        false => "local",
    };
    item(
        "schedule",
        "Schedule",
        "Restrict the rule to certain days, dates, and hours.",
        rule.active("schedule"),
        vec![
            Widget::checks(
                "weekdays",
                "Weekdays",
                &rule.schedule.weekdays,
                named(&WEEKDAYS.map(String::from)),
            ),
            form_grid(
                2,
                vec![
                    text_field(
                        "start_date",
                        "Starting date",
                        &rule.schedule.start_date,
                        "YYYY-MM-DD; blank means immediately.",
                        errors,
                    ),
                    text_field(
                        "stop_date",
                        "Ending date",
                        &rule.schedule.stop_date,
                        "YYYY-MM-DD; blank means indefinitely.",
                        errors,
                    ),
                ],
            ),
            form_grid(
                3,
                vec![
                    text_field("start_time", "From", &rule.schedule.start_time, "", errors),
                    text_field("stop_time", "Until", &rule.schedule.stop_time, "", errors),
                    select_field(
                        "time_basis",
                        "Clock",
                        basis,
                        options(&[("local", "Router local time"), ("utc", "UTC")]),
                        errors,
                    ),
                ],
            ),
        ],
    )
}

/// comparison is the include/exclude select every negatable condition carries.
/// The labels say what each side means for that condition, because "exclude"
/// alone reads as removing the condition rather than inverting it.
fn comparison(
    name: &str,
    value: &crate::rule_form::Inverted,
    include: &str,
    exclude: &str,
) -> Widget {
    Widget::select(
        name,
        "Comparison",
        value.comparison(),
        vec![
            SelectOption::new(INCLUDE, include),
            SelectOption::new(EXCLUDE, exclude),
        ],
        "",
    )
}

fn dscp_options() -> Vec<SelectOption> {
    DSCP_CLASSES
        .iter()
        .map(|class| {
            let label = match *class {
                "CS0" => "CS0 — best effort",
                "BE" => "BE — best effort",
                "LE" => "LE — lower effort",
                "EF" => "EF — expedited forwarding",
                class => class,
            };
            SelectOption::new(class, label)
        })
        .collect()
}

/// options turns a value/label table into the select's choices.
pub fn options(pairs: &[(&str, &str)]) -> Vec<SelectOption> {
    pairs
        .iter()
        .map(|(value, label)| SelectOption::new(value, label))
        .collect()
}

/// named turns a list of config names into choices labelled as themselves — the
/// right treatment for a value an operator wrote and recognizes.
pub fn named(values: &[String]) -> Vec<SelectOption> {
    values
        .iter()
        .map(|value| SelectOption::new(value, value))
        .collect()
}
