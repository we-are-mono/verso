// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

//! The optional matches an object may carry, as its editor offers them.
//!
//! A rule's zones, protocols and verdict are always there; everything else is a
//! condition it either has or has not. Declaring the whole catalogue and marking
//! which entries the object carries is what keeps an editor complete without
//! presenting a wall of empty controls: the shell renders the carried ones and
//! keeps the rest behind its Add-condition picker.
//!
//! A port forward is the same shape. firewall4's `redirect` section carries
//! nearly a rule's whole matching vocabulary, and the editor that asked only
//! where traffic arrives and where it goes left most of it unreachable. It offers
//! the same catalogue now — which is why the builders below take the values a
//! condition is made of rather than a whole rule: the two objects differ in
//! which conditions they can carry, never in what a mark or a schedule means.
//!
//! Where firewall4 reads the same option differently, they differ here too, and
//! the difference is the daemon's:
//!
//! - `src_ip`, `src_dip` and `src_port` are **scalars** on a redirect and lists
//!   on a rule, so one is a value with a comparison and the other a pair of
//!   token lists. fw4 skips a whole section that writes a list where it expects
//!   one value.
//! - `helper` **must not be negated** on a redirect, so that one is a plain
//!   choice there and a comparison here.
//!
//! Only firewall4's own options appear. A condition it cannot express would be a
//! control that changes nothing, which is worse than a control that is missing.

use verso_plugin::{ConditionItem, List, SelectOption, Widget};

use crate::fields::{select_field, text_field, token_list};
use crate::model::Firewall;
use crate::redirect_form::RedirectForm;
use crate::rule_form::{
    Errors, Inverted, MarkMatch, RateLimit, RuleForm, Schedule, SetMatch, Tokens, DSCP_CLASSES,
    EXCLUDE, HELPERS, INCLUDE, RATE_UNITS, SET_FIELDS, WEEKDAYS,
};

/// HELP says the one thing the block does not show: the seams between
/// conditions say "and", but the values inside one are alternatives.
const HELP: &str = "Several values inside one condition are alternatives.";

/// SCHEDULE_FIELDS are the form fields a window is made of. The clock is a
/// choice between two names here and a truth value in the file, which is why
/// this list is the form's and not firewall4's.
pub const SCHEDULE_FIELDS: [&str; 6] = [
    "weekdays",
    "start_date",
    "stop_date",
    "start_time",
    "stop_time",
    "time_basis",
];

/// FIELDS is every form field the rule catalogue renders, whether or not a given
/// rule carries the condition it belongs to — which is the question a panel asks
/// when it decides what to carry for the readings that do not show the
/// catalogue at all.
///
/// A condition the rule does not carry is rendered into a template and posts
/// nothing, so naming its fields here says only "this reading is where they
/// would be", which is what the carrier needs to know. The test beside it reads
/// the catalogue's own tree and fails if the two come apart.
pub const FIELDS: &[&str] = &[
    "direction",
    "device",
    "src_ip",
    "src_ip_not",
    "src_port",
    "src_port_not",
    "dest_ip",
    "dest_ip_not",
    "dest_port",
    "dest_port_not",
    "src_mac",
    "src_mac_not",
    "icmp_type",
    "ipset",
    "ipset_match",
    "ipset_field_1",
    "ipset_field_2",
    "ipset_field_3",
    "mark",
    "mark_match",
    "mark_value",
    "mark_mask",
    "dscp",
    "dscp_match",
    "helper",
    "helper_match",
    "limit",
    "limit_match",
    "limit_unit",
    "limit_burst",
    "weekdays",
    "start_date",
    "stop_date",
    "start_time",
    "stop_time",
    "time_basis",
];

/// for_rule is the condition catalogue for one rule. An entry the config cannot
/// support — a named set on a router that declares none — is left out entirely
/// rather than offered empty.
pub fn for_rule(rule: &RuleForm, errors: &Errors, model: &Firewall) -> Widget {
    // The order is the order someone reaches for them: where the traffic is from
    // and going, then what it is, then the wire, then what another rule has
    // already said about it, then how often and when.
    let mut items = vec![
        listed(
            addresses(
                "src_ip",
                "Source addresses",
                "Match where the traffic comes from.",
                &rule.src_ip,
                rule.active("src_ip"),
                errors,
            ),
            ENDPOINTS,
            "one host or a subnet",
        ),
        listed(
            ports(
                "src_port",
                "Source ports",
                "Match the port the traffic was sent from.",
                &rule.src_port,
                rule.active("src_port"),
                errors,
            ),
            ENDPOINTS,
            "rarely useful",
        ),
        listed(
            addresses(
                "dest_ip",
                "Destination addresses",
                "Match where the traffic is going.",
                &rule.dest_ip,
                rule.active("dest_ip"),
                errors,
            ),
            ENDPOINTS,
            "one host or a subnet",
        ),
        listed(
            ports(
                "dest_port",
                "Destination ports",
                "Match the port the traffic is going to.",
                &rule.dest_port,
                rule.active("dest_port"),
                errors,
            ),
            ENDPOINTS,
            "space-separated, or a range",
        ),
        listed(
            macs(&rule.src_mac, rule.active("src_mac"), errors),
            ENDPOINTS,
            "survives a changed lease",
        ),
        listed(
            icmp_types(&rule.icmp_type, rule.active("icmp_type"), errors),
            PROTOCOL,
            "names or numbers",
        ),
        listed(device(rule, errors, model), INTERFACE, "bypasses the zone"),
    ];
    if let Some(item) = named_set(&rule.ipset, rule.active("ipset"), errors, model) {
        items.push(listed(item, MARKS, "a named set, kept elsewhere"));
    }
    items.extend([
        listed(
            mark(&rule.mark, rule.active("mark"), errors),
            MARKS,
            "set by another rule",
        ),
        listed(dscp(rule, errors), MARKS, "a traffic class"),
        listed(
            helper_match(&rule.helper, rule.active("helper"), errors),
            MARKS,
            "matches a tracked protocol",
        ),
        listed(
            rate(&rule.rate, rule.active("rate"), errors),
            WHEN,
            "how often it may match",
        ),
        listed(
            schedule(&rule.schedule, rule.active("schedule"), errors),
            WHEN,
            "days and hours",
        ),
    ]);
    catalogue(items)
}

/// for_redirect is the same catalogue for one port forward. It is shorter than a
/// rule's by what firewall4 does not read on a redirect — no device, no DSCP, no
/// destination-port match beyond the one the forward rewrites — and the entries
/// it shares are the same controls writing the same options.
pub fn for_redirect(redirect: &RedirectForm, errors: &Errors, model: &Firewall) -> Widget {
    let mut items = vec![
        listed(
            address(
                "src_ip",
                "Source address",
                "Match only traffic that came from one address or network.",
                &redirect.src_ip,
                redirect.active("src_ip"),
                errors,
            ),
            ENDPOINTS,
            "one host or a subnet",
        ),
        listed(
            address(
                "src_dip",
                "Public address it arrived on",
                "Match only traffic addressed to one of this router's own addresses — for a \
                 connection with more than one.",
                &redirect.src_dip,
                redirect.active("src_dip"),
                errors,
            ),
            ENDPOINTS,
            "one of this router's addresses",
        ),
        listed(
            port(
                "src_port",
                "Source port",
                "Match the port the traffic was sent from.",
                &redirect.src_port,
                redirect.active("src_port"),
                errors,
            ),
            ENDPOINTS,
            "rarely useful",
        ),
        listed(
            macs(&redirect.src_mac, redirect.active("src_mac"), errors),
            ENDPOINTS,
            "survives a changed lease",
        ),
    ];
    if let Some(item) = named_set(&redirect.ipset, redirect.active("ipset"), errors, model) {
        items.push(listed(item, MARKS, "a named set, kept elsewhere"));
    }
    items.extend([
        listed(
            mark(&redirect.mark, redirect.active("mark"), errors),
            MARKS,
            "set by another rule",
        ),
        listed(
            helper_choice(&redirect.helper, redirect.active("helper"), errors),
            MARKS,
            "matches a tracked protocol",
        ),
        listed(
            rate(&redirect.rate, redirect.active("rate"), errors),
            WHEN,
            "how often it may match",
        ),
        listed(
            schedule(&redirect.schedule, redirect.active("schedule"), errors),
            WHEN,
            "days and hours",
        ),
    ]);
    catalogue(items)
}

fn catalogue(items: Vec<ConditionItem>) -> Widget {
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
        group: String::new(),
        hint: String::new(),
        active,
        children,
    }
}

/// The kinds a condition belongs to in the picker. Four headings over a dozen
/// entries is the difference between a list someone reads and one they scan past:
/// where the traffic is going, what it is, which wire it came in on, and what
/// something else has already decided about it.
const ENDPOINTS: &str = "Endpoints";
const PROTOCOL: &str = "Protocol";
const INTERFACE: &str = "Interface";
const MARKS: &str = "Sets and marks";
const WHEN: &str = "Rate and time";

/// listed places a condition in the picker: under which heading, and with what a
/// person would type into it. Both belong to the catalogue rather than to the
/// condition's own builder — they are how this list is laid out for reading, which
/// is one editorial decision about the whole of it and is easiest to get right
/// with all of it in front of you.
fn listed(mut item: ConditionItem, group: &str, hint: &str) -> ConditionItem {
    item.group = group.into();
    item.hint = hint.into();
    item
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
    )
}

fn addresses(
    key: &str,
    label: &str,
    help: &str,
    tokens: &Tokens,
    active: bool,
    errors: &Errors,
) -> ConditionItem {
    item(
        key,
        label,
        help,
        active,
        vec![
            token_list(
                key,
                "Include",
                "Address or network",
                "IP addresses, or networks in CIDR form.",
                &tokens.include,
                errors,
            ),
            exceptions(
                token_list(
                    &format!("{key}_not"),
                    "Exclude",
                    "Address or network",
                    "Everything else in the include list still matches.",
                    &tokens.exclude,
                    errors,
                ),
                tokens,
            ),
        ],
    )
}

fn ports(
    key: &str,
    label: &str,
    help: &str,
    tokens: &Tokens,
    active: bool,
    errors: &Errors,
) -> ConditionItem {
    item(
        key,
        label,
        help,
        active,
        vec![
            token_list(
                key,
                "Include",
                "Port or range",
                "For example 53, or 1024-65535.",
                &tokens.include,
                errors,
            ),
            exceptions(
                token_list(
                    &format!("{key}_not"),
                    "Exclude",
                    "Port or range",
                    "",
                    &tokens.exclude,
                    errors,
                ),
                tokens,
            ),
        ],
    )
}

/// exceptions folds a condition's exclude list behind a quiet "Exclude some":
/// an exception is the rare half of an include/exclude condition, and an empty
/// second box beside the first reads as a second thing to fill in. It arrives
/// open whenever the condition already excludes something — or whenever the
/// last submission refused an exclusion, so the refusal is never folded away.
fn exceptions(exclude: Widget, tokens: &Tokens) -> Widget {
    let refused = matches!(&exclude, Widget::List(List { errors, .. }) if !errors.is_empty());
    Widget::reveal(
        "Exclude some",
        !tokens.exclude.is_empty() || refused,
        vec![exclude],
    )
}

/// address is the same condition where firewall4 reads one value rather than a
/// list — a redirect's `src_ip` and `src_dip`. One value with a comparison
/// beside it, because a list here is a section fw4 skips entirely.
fn address(
    key: &str,
    label: &str,
    help: &str,
    value: &Inverted,
    active: bool,
    errors: &Errors,
) -> ConditionItem {
    item(
        key,
        label,
        help,
        active,
        vec![
            comparison(
                &format!("{key}_match"),
                value,
                "Is this address",
                "Is anything but this",
            ),
            text_field(
                key,
                "Address",
                &value.value,
                "One IP address, or a network in CIDR form.",
                errors,
            ),
        ],
    )
}

/// port is the scalar reading of a port match, for the same reason.
fn port(
    key: &str,
    label: &str,
    help: &str,
    value: &Inverted,
    active: bool,
    errors: &Errors,
) -> ConditionItem {
    item(
        key,
        label,
        help,
        active,
        vec![
            comparison(
                &format!("{key}_match"),
                value,
                "Is this port",
                "Is anything but this",
            ),
            text_field(
                key,
                "Port",
                &value.value,
                "For example 53, or 1024-65535.",
                errors,
            ),
        ],
    )
}

fn macs(tokens: &Tokens, active: bool, errors: &Errors) -> ConditionItem {
    item(
        "src_mac",
        "Source MAC addresses",
        "Match link-layer senders visible on the way in.",
        active,
        vec![
            token_list(
                "src_mac",
                "Include",
                "MAC address",
                "",
                &tokens.include,
                errors,
            ),
            exceptions(
                token_list(
                    "src_mac_not",
                    "Exclude",
                    "MAC address",
                    "",
                    &tokens.exclude,
                    errors,
                ),
                tokens,
            ),
        ],
    )
}

fn icmp_types(values: &[String], active: bool, errors: &Errors) -> ConditionItem {
    item(
        "icmp_type",
        "ICMP types",
        "Narrow an ICMP rule to the kinds of message it is about.",
        active,
        vec![token_list(
            "icmp_type",
            "Types",
            "Type name or type/code",
            "Examples: echo-request, neighbour-solicitation, or 130/0.",
            values,
            errors,
        )],
    )
}

/// named_set matches one of the firewall config's own sets. A router with no
/// `config ipset` has nothing to match against, so the condition is not offered
/// there — matching a set that does not exist is a section firewall4 skips.
fn named_set(
    ipset: &SetMatch,
    active: bool,
    errors: &Errors,
    model: &Firewall,
) -> Option<ConditionItem> {
    let mut sets = model.ipsets.clone();
    if !ipset.set.value.is_empty() && !sets.contains(&ipset.set.value) {
        sets.insert(0, ipset.set.value.clone());
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
            &ipset.fields[slot],
            choices,
            errors,
        )
    };
    Some(item(
        "ipset",
        "Named firewall set",
        "Match a set declared in this firewall config, and say which packet fields its entries describe.",
        active,
        vec![
            select_field("ipset", "Set", &ipset.set.value, named(&sets), errors),
            comparison("ipset_match", &ipset.set, "Is in the set", "Is not in the set"),
            field(0, "First field", false),
            field(1, "Second field", true),
            field(2, "Third field", true),
        ],
    ))
}

/// helper_match is the rule's reading: firewall4 lets a rule ask for traffic a
/// helper is *not* tracking, so the comparison is part of the condition.
fn helper_match(value: &Inverted, active: bool, errors: &Errors) -> ConditionItem {
    item(
        "helper",
        "Connection helper",
        "Match traffic a connection helper is already tracking.",
        active,
        vec![
            comparison(
                "helper_match",
                value,
                "Uses this helper",
                "Does not use this helper",
            ),
            select_field(
                "helper",
                "Helper",
                &value.value,
                named(&HELPERS.map(String::from)),
                errors,
            ),
        ],
    )
}

/// helper_choice is the redirect's reading of the same option. firewall4 marks
/// it NO_INVERT there and refuses the whole section for a leading `!`, so there
/// is no comparison to offer — a control that could write one would be a control
/// that takes the forward out of the ruleset.
fn helper_choice(value: &str, active: bool, errors: &Errors) -> ConditionItem {
    item(
        "helper",
        "Connection helper",
        "Match traffic a connection helper is already tracking.",
        active,
        vec![select_field(
            "helper",
            "Helper",
            value,
            named(&HELPERS.map(String::from)),
            errors,
        )],
    )
}

fn mark(mark: &MarkMatch, active: bool, errors: &Errors) -> ConditionItem {
    item(
        "mark",
        "Firewall mark",
        "Match a mark another rule already set on the packet.",
        active,
        vec![
            comparison("mark_match", &mark.mark, "Equals", "Does not equal"),
            text_field(
                "mark_value",
                "Value",
                &mark.mark.value,
                "Decimal, or hexadecimal with 0x.",
                errors,
            ),
            text_field("mark_mask", "Mask", &mark.mask, "Optional.", errors),
        ],
    )
}

fn dscp(rule: &RuleForm, errors: &Errors) -> ConditionItem {
    item(
        "dscp",
        "DSCP value",
        "Match the traffic class another device or rule already marked the packet with.",
        rule.active("dscp"),
        vec![
            comparison("dscp_match", &rule.dscp, "Equals", "Does not equal"),
            select_field("dscp", "DSCP", &rule.dscp.value, dscp_options(), errors),
        ],
    )
}

fn rate(limit: &RateLimit, active: bool, errors: &Errors) -> ConditionItem {
    let matching = match limit.over {
        true => "over",
        false => "below",
    };
    // Read as the sentence it writes: the rate first, "1000 per second", then
    // which side of it matches, then the allowance before it applies. The count
    // and its unit are one option, `limit`, so both name it.
    item(
        "rate",
        "Rate limit",
        "Match only while the traffic stays under a rate — or only once it goes over.",
        active,
        vec![
            Widget::form_grid(
                2,
                vec![
                    text_field("limit", "Packets", &limit.count, "", errors).writes("limit"),
                    select_field(
                        "limit_unit",
                        "Per",
                        &limit.unit,
                        options(&RATE_UNITS),
                        errors,
                    )
                    .writes("limit"),
                ],
            )
            .labelled("Rate", "")
            .joined("per"),
            select_field(
                "limit_match",
                "Matches",
                matching,
                options(&[("below", "At or below the rate"), ("over", "Over the rate")]),
                errors,
            ),
            text_field(
                "limit_burst",
                "Initial burst",
                &limit.burst,
                "Optional packet allowance before the rate applies.",
                errors,
            )
            .writes("limit_burst"),
        ],
    )
}

fn schedule(schedule: &Schedule, active: bool, errors: &Errors) -> ConditionItem {
    item(
        "schedule",
        "Schedule",
        "Restrict it to certain days, dates, and hours.",
        active,
        schedule_fields(schedule, errors),
    )
}

/// schedule_fields is when the object applies. On the page it is one condition
/// among the rest, because a rule that names no window is the common one; the
/// drawer asks it under a tab of its own, so the controls are here rather than
/// inside the condition that folds them away.
pub fn schedule_fields(schedule: &Schedule, errors: &Errors) -> Vec<Widget> {
    let basis = match schedule.utc {
        true => "utc",
        false => "local",
    };
    vec![
        Widget::checks(
            "weekdays",
            "Days",
            &schedule.weekdays,
            WEEKDAYS
                .iter()
                .map(|day| SelectOption::new(day, day))
                .collect(),
        )
        .writes("weekdays")
        .segmented(),
        window(
            "Between",
            "timehhmmss",
            [
                ("start_time", "Starts at", &schedule.start_time),
                ("stop_time", "Ends at", &schedule.stop_time),
            ],
            errors,
        ),
        window(
            "Only between dates",
            "dateyyyymmdd",
            [
                ("start_date", "Starts on", &schedule.start_date),
                ("stop_date", "Ends on", &schedule.stop_date),
            ],
            errors,
        ),
        // The clock the window is read against: a choice between two names
        // here, a truth value in the file (`utc_time`).
        select_field(
            "time_basis",
            "Clock",
            basis,
            options(&[("local", "Router local time"), ("utc", "UTC")]),
            errors,
        )
        .writes("utc_time"),
    ]
}

/// window is a span the object keeps, read as the sentence it is — "09:00 to
/// 17:00": one row under its label, each end its own field posting its own
/// option in the grammar firewall4 reads, the ends joined by their word.
fn window(label: &str, grammar: &str, ends: [(&str, &str, &str); 2], errors: &Errors) -> Widget {
    Widget::form_grid(
        2,
        ends.iter()
            .map(|(name, end, value)| {
                text_field(name, end, value, "", errors)
                    .writes(name)
                    .typed(grammar)
            })
            .collect(),
    )
    .labelled(label, "")
    .joined("to")
}

/// comparison is the include/exclude select every negatable condition carries.
/// The labels say what each side means for that condition, because "exclude"
/// alone reads as removing the condition rather than inverting it.
fn comparison(name: &str, value: &Inverted, include: &str, exclude: &str) -> Widget {
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
