// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

//! This plugin's say about one device, for the shell's device panel.
//!
//! It answers with a tab, not a page: the panel around it is the shell's, the
//! facts above it are the shell's, and the other tabs in it belong to plugins
//! this one knows nothing about. The Edit limits action asks whether a device reaches the
//! internet, when, and how fast.

use verso_plugin::{Envelope, Form, SelectOption, Tone, Widget};

use crate::form::{self, Errors};
use crate::leases::Leases;
use crate::model::{Policy, DAYS};

/// Saving stages configuration; the shell's review drawer owns Apply.
pub const LABEL: &str = "Limits";
const CTA: &str = "Save";
const NOTE: &str = "Apply pending changes to use these settings.";

const BLOCKED_NOTE: &str =
    "Blocks internet access. Access to devices on the local network is unchanged.";

const SCHEDULE_HELP: &str = "Block internet access during these hours on the selected days.";

const HOURS_HELP: &str = "Times use the router’s time zone.";

const RATE_TIP: &str = "Maximum download speed in megabits per second. Leave blank for no limit.";

/// tab is the whole of this plugin's contribution to a device's panel.
pub fn tab(policy: &Policy, leases: &Leases, errors: &Errors) -> Envelope {
    let name = leases.subject(&policy.mac);
    let body = Widget::stack(vec![
        Widget::Form {
            style: "page".into(),
            submit: String::new(),
            error: String::new(),
            fields: controls(policy, errors),
            note: String::new(),
            target: String::new(),
        },
        // Two files, two blocks — so neither is declared live: only one preview
        // per form can be kept current, and choosing between these two is this
        // plugin's call rather than a side effect of the firewall's.
        Widget::code("/etc/config/firewall", &form::rule_preview(policy, &name)),
        Widget::code("/etc/config/qos", &form::caps_preview(policy, &name)),
    ]);
    Envelope::page(LABEL, body)
        .with_commit_row(CTA, NOTE)
        .with_tab_state(state(policy))
}

/// state is where the device stands, for the chip the shell hangs beside this
/// tab's label — the answer someone opened the panel for, before they read a
/// single control.
pub fn state(policy: &Policy) -> &'static str {
    match (policy.allowed, policy.scheduled, policy.capped()) {
        (false, _, _) => "blocked",
        (true, true, _) => "on a schedule",
        (true, false, true) => "limited",
        (true, false, false) => "no limit",
    }
}

/// controls is the tab's editing surface: access, then — only where access is
/// granted — when it lapses, then how fast. Each gate hides what it decides, so
/// nobody sets a curfew on a device that is blocked outright.
fn controls(policy: &Policy, errors: &Errors) -> Vec<Widget> {
    vec![
        Widget::gate(
            "allowed",
            "Internet access",
            "target",
            "",
            policy.allowed,
            vec![Widget::gate(
                "scheduled",
                "Block internet on a schedule",
                "",
                SCHEDULE_HELP,
                policy.scheduled,
                vec![
                    days(policy, errors),
                    from(policy, errors),
                    to(policy, errors),
                ],
                Vec::new(),
            )],
            vec![Widget::Callout {
                variant: Tone::Warning,
                title: String::new(),
                body: BLOCKED_NOTE.into(),
                compact: true,
            }],
        ),
        cap("download", "Download limit", &policy.down, errors).explained(RATE_TIP, "qos device"),
        cap("upload", "Upload limit", &policy.up, errors).explained(
            "Maximum upload speed in megabits per second. Leave blank for no limit.",
            "qos device",
        ),
    ]
}

/// days is the week as one strip: which days the curfew runs is a shape to
/// recognize, not a list to read.
fn days(policy: &Policy, errors: &Errors) -> Widget {
    let options: Vec<SelectOption> = DAYS.iter().map(|day| SelectOption::new(day, day)).collect();
    field(
        Widget::checks("weekdays", "Days", &policy.days, options).segmented(),
        "weekdays",
        errors,
    )
}

fn from(policy: &Policy, errors: &Errors) -> Widget {
    clock("start_time", "Block from", &policy.from, HOURS_HELP, errors)
}

fn to(policy: &Policy, errors: &Errors) -> Widget {
    clock("stop_time", "Until", &policy.to, "", errors)
}

/// clock is one edge of the curfew, in the native time control.
fn clock(name: &str, label: &str, value: &str, help: &str, errors: &Errors) -> Widget {
    field(
        Widget::Field {
            name: name.into(),
            label: label.into(),
            kind: "time".into(),
            advanced: false,
            value: value.into(),
            values: Vec::new(),
            placeholder: String::new(),
            datatype: String::new(),
            options: Vec::new(),
            error: String::new(),
            help: help.into(),
            key: name.into(),
            tip: String::new(),
            source: String::new(),
            unit: String::new(),
            style: String::new(),
            remove: String::new(),
            pair: None,
            target: String::new(),
        },
        name,
        errors,
    )
}

/// cap is one direction's ceiling: a bare number, with what it counts inside the
/// box and "No limit" standing where a number is not.
fn cap(name: &str, label: &str, value: &str, errors: &Errors) -> Widget {
    field(
        Widget::Field {
            name: name.into(),
            label: label.into(),
            kind: "text".into(),
            advanced: false,
            value: value.into(),
            values: Vec::new(),
            placeholder: "No limit".into(),
            datatype: String::new(),
            options: Vec::new(),
            error: String::new(),
            help: String::new(),
            key: name.into(),
            tip: String::new(),
            source: String::new(),
            unit: "Mbit/s".into(),
            style: String::new(),
            remove: String::new(),
            pair: None,
            target: String::new(),
        },
        name,
        errors,
    )
}

/// field hangs whatever the last submission got wrong on the control carrying
/// it, wherever that control was built.
fn field(widget: Widget, name: &str, errors: &Errors) -> Widget {
    let mut widget = widget;
    if let Widget::Field { error, .. } = &mut widget {
        *error = errors.get(name).into();
    }
    widget
}

/// save answers a submitted tab: the policy as stated, refused on the controls
/// that carry a value the router would not read, or staged as the two writes it
/// comes to.
pub fn save(policy: &Policy, leases: &Leases, form: &Form) -> Envelope {
    let stated = form::submitted(policy, form);
    let errors = form::validate(&stated);
    if !errors.is_empty() {
        return tab(&stated, leases, &errors).with_notice(Tone::Danger, form::REFUSED);
    }
    let name = leases.subject(&stated.mac);
    tab(&stated, leases, &Errors::default())
        .with_notice(Tone::Success, "Limits saved to pending changes.")
        .with_commit(form::commits(&stated, &name))
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::fixture;
    use crate::model::Policy;
    use serde_json::Value as Json;

    fn body(env: Envelope) -> Json {
        serde_json::to_value(&env).expect("serialize")
    }

    /// control digs one named control out of the tab, wherever the gates put it.
    fn control(env: &Json, name: &str) -> Json {
        fn walk(node: &Json, name: &str) -> Option<Json> {
            if node["name"] == name && node["type"] == "field" {
                return Some(node.clone());
            }
            for key in ["fields", "otherwise", "children"] {
                for child in node[key].as_array().into_iter().flatten() {
                    if let Some(found) = walk(child, name) {
                        return Some(found);
                    }
                }
            }
            None
        }
        walk(&env["widget"], name).unwrap_or_else(|| panic!("no control {name}"))
    }

    fn policy(mac: &str) -> Policy {
        Policy::read(&fixture::snapshot(), mac)
    }

    #[test]
    fn the_tab_opens_on_what_the_config_already_says_about_the_device() {
        let leases = Leases::read(&fixture::ubus());

        // Refused outright: the access gate is off, so the hours are not even
        // on the screen — but the schedule inside it is honest about itself.
        let blocked = body(tab(
            &policy("3c:22:fb:8d:44:a1"),
            &leases,
            &Errors::default(),
        ));
        assert_eq!(blocked["state"], "blocked");
        assert_eq!(blocked["cta"], "Save");
        assert_eq!(gate(&blocked, "allowed")["checked"], false);

        // Refused between hours: allowed, on a schedule, and the days as written.
        let curfew = body(tab(
            &policy("e4:5e:1b:9a:2c:63"),
            &leases,
            &Errors::default(),
        ));
        assert_eq!(curfew["state"], "on a schedule");
        assert_eq!(gate(&curfew, "allowed")["checked"], true);
        assert_eq!(gate(&curfew, "scheduled")["checked"], true);
        assert_eq!(control(&curfew, "start_time")["value"], "21:00");
        assert_eq!(control(&curfew, "stop_time")["value"], "07:00");
        assert_eq!(
            control(&curfew, "weekdays")["values"],
            serde_json::json!(["Mon", "Tue", "Wed", "Thu", "Fri"])
        );
        assert_eq!(control(&curfew, "weekdays")["style"], "segmented");
        assert_eq!(control(&curfew, "download")["value"], "25");

        // Capped only: no rule at all, so the preview says there is none.
        let capped = body(tab(
            &policy("30:9c:23:5e:88:01"),
            &leases,
            &Errors::default(),
        ));
        assert_eq!(capped["state"], "limited");
        assert_eq!(control(&capped, "download")["value"], "50");
        assert_eq!(control(&capped, "download")["unit"], "Mbit/s");
        let previews = capped["widget"]["children"].as_array().expect("children");
        assert!(previews[1]["value"]
            .as_str()
            .expect("rule preview")
            .starts_with("# no rule: nas reaches the internet"));
        assert!(previews[2]["value"]
            .as_str()
            .expect("caps preview")
            .contains("option download '50'"));
    }

    /// gate digs out one of the tab's two conditionals.
    fn gate(env: &Json, name: &str) -> Json {
        fn walk(node: &Json, name: &str) -> Option<Json> {
            if node["name"] == name && node["type"] == "conditional" {
                return Some(node.clone());
            }
            for key in ["fields", "otherwise", "children"] {
                for child in node[key].as_array().into_iter().flatten() {
                    if let Some(found) = walk(child, name) {
                        return Some(found);
                    }
                }
            }
            None
        }
        walk(&env["widget"], name).unwrap_or_else(|| panic!("no gate {name}"))
    }

    #[test]
    fn a_value_the_router_would_not_read_is_marked_and_nothing_is_written() {
        let leases = Leases::read(&fixture::ubus());
        let held = policy("e4:5e:1b:9a:2c:63");
        let refused = body(save(
            &held,
            &leases,
            &Form::parse("allowed=1&scheduled=1&start_time=25:00&download=lots"),
        ));
        assert_eq!(refused["notice"]["level"], "danger");
        assert!(refused["commit"].is_null(), "{refused}");
        assert_eq!(
            control(&refused, "start_time")["error"],
            "Times run on the 24-hour clock, as in 21:00."
        );
        assert!(control(&refused, "download")["error"]
            .as_str()
            .expect("error")
            .starts_with("A download limit is a whole number"));

        // A schedule with no day picked is refused on the days, not silently
        // turned into a rule that never matches.
        let no_days = body(save(&held, &leases, &Form::parse("allowed=1&scheduled=1")));
        assert_eq!(
            control(&no_days, "weekdays")["error"],
            "Pick at least one day, or turn the schedule off."
        );
    }
}
