// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

//! What a submitted policy says, whether the daemons would accept it, and what
//! writing it comes to.
//!
//! Validation is fw4's and the shaper's, not a looser echo: a time fw4 cannot
//! parse is a rule it drops in silence, and a rate that is not a number is a
//! queueing discipline that never loads. Both are refused here, on the control
//! carrying the offending value, before anything is written.

use std::collections::BTreeMap;

use verso_plugin::{commit, commit_delete, commit_new, json, Form, Map, Value};

use crate::model::{Policy, CAPS, DAYS, DEVICE, FIREWALL, RULE, WAN};

/// MAX_MBIT bounds a cap at something a home link could plausibly carry, so a
/// mistyped run of digits is refused rather than written as a rate no interface
/// can offer.
const MAX_MBIT: u32 = 100_000;

/// Errors is what a submission got wrong, addressed to the controls carrying the
/// offending values. The shell reads the annotations back off the re-rendered
/// tree, so a form that reports one is a 422 and its write is blocked.
#[derive(Default)]
pub struct Errors(BTreeMap<String, String>);

impl Errors {
    pub fn check(&mut self, name: &str, ok: bool, message: &str) {
        if !ok {
            self.0
                .entry(name.to_string())
                .or_insert_with(|| message.to_string());
        }
    }

    pub fn get(&self, name: &str) -> &str {
        self.0.get(name).map(String::as_str).unwrap_or("")
    }

    pub fn is_empty(&self) -> bool {
        self.0.is_empty()
    }
}

/// REFUSED is what the tab says when it wrote nothing.
pub const REFUSED: &str =
    "Some values aren’t ones the router accepts, so nothing was saved. They’re marked below.";

/// submitted reads one device's policy back off the form, carrying the sections
/// the config already holds so a save edits what is there rather than adding
/// beside it.
///
/// A branch the operator cannot see is a branch they did not choose: the hours
/// still post while the schedule is folded away, and the schedule still posts
/// while the device is blocked outright, so each is read only where the one
/// above it left it reachable.
pub fn submitted(held: &Policy, form: &Form) -> Policy {
    let allowed = form.get("allowed") == "1";
    let scheduled = allowed && form.get("scheduled") == "1";
    let days: Vec<String> = match scheduled {
        true => form
            .all("weekdays")
            .into_iter()
            .filter(|day| DAYS.contains(&day.as_str()))
            .collect(),
        false => held.days.clone(),
    };
    Policy {
        mac: held.mac.clone(),
        rule: held.rule.clone(),
        caps: held.caps.clone(),
        allowed,
        scheduled,
        days,
        from: value(form, "start_time", &held.from),
        to: value(form, "stop_time", &held.to),
        down: form.get("download").trim().to_string(),
        up: form.get("upload").trim().to_string(),
    }
}

/// value is a submitted field, falling back to what is held where the form did
/// not carry it at all — a control the reading in force never drew.
fn value(form: &Form, name: &str, held: &str) -> String {
    let stated = form.get(name).trim().to_string();
    match stated.is_empty() {
        true => held.to_string(),
        false => stated,
    }
}

/// validate refuses what fw4 or the shaper would not read.
pub fn validate(policy: &Policy) -> Errors {
    let mut errors = Errors::default();
    errors.check(
        "download",
        rate(&policy.down),
        "A download limit is a whole number of Mbit/s, or empty for no limit.",
    );
    errors.check(
        "upload",
        rate(&policy.up),
        "An upload limit is a whole number of Mbit/s, or empty for no limit.",
    );
    if policy.scheduled {
        errors.check(
            "weekdays",
            !policy.days.is_empty(),
            "Pick at least one day, or turn the schedule off.",
        );
        errors.check(
            "start_time",
            clock(&policy.from),
            "Times run on the 24-hour clock, as in 21:00.",
        );
        errors.check(
            "stop_time",
            clock(&policy.to),
            "Times run on the 24-hour clock, as in 21:00.",
        );
    }
    errors
}

/// rate accepts a whole number of Mbit/s, or nothing at all for no limit.
fn rate(value: &str) -> bool {
    value.is_empty()
        || value
            .parse::<u32>()
            .is_ok_and(|mbit| mbit > 0 && mbit <= MAX_MBIT)
}

/// clock accepts a 24-hour time of day, which is what fw4 reads.
fn clock(value: &str) -> bool {
    let Some((hours, minutes)) = value.split_once(':') else {
        return false;
    };
    let whole = |part: &str, max: u32| {
        part.len() == 2
            && part.chars().all(|c| c.is_ascii_digit())
            && part.parse().is_ok_and(|n: u32| n < max)
    };
    whole(hours, 24) && whole(minutes, 60)
}

/// commits is what saving this policy comes to: the access rule fw4 enforces,
/// and the caps in this plugin's own config. Each half is added, rewritten, or
/// removed on its own — a device given a speed limit and left on the internet
/// needs no firewall rule at all, and one that had a rule and no longer needs it
/// has that rule removed rather than left behind saying nothing.
pub fn commits(policy: &Policy, name: &str) -> Vec<verso_plugin::CommitOp> {
    let mut ops = Vec::new();
    match (policy.refuses(), policy.rule.is_empty()) {
        (true, true) => ops.push(commit_new(FIREWALL, RULE, rule_values(policy, name))),
        (true, false) => ops.push(commit(FIREWALL, &policy.rule, rule_values(policy, name))),
        (false, false) => ops.push(commit_delete(FIREWALL, &policy.rule)),
        (false, true) => {}
    }
    match (policy.capped(), policy.caps.is_empty()) {
        (true, true) => ops.push(commit_new(CAPS, DEVICE, caps_values(policy))),
        (true, false) => ops.push(commit(CAPS, &policy.caps, caps_values(policy))),
        (false, false) => ops.push(commit_delete(CAPS, &policy.caps)),
        (false, true) => {}
    }
    ops
}

/// rule_values is the access rule as fw4 holds it. A flat refusal states no
/// hours, and states them as null rather than leaving them out, so a schedule
/// that was there is cleared rather than kept alongside a rule that no longer
/// means it.
fn rule_values(policy: &Policy, name: &str) -> Value {
    let mut values = Map::new();
    values.insert("name".into(), json!(rule_name(name)));
    values.insert("src".into(), json!(crate::model::ANY_ZONE));
    values.insert("src_mac".into(), json!(policy.mac));
    values.insert("dest".into(), json!(WAN));
    values.insert("target".into(), json!("REJECT"));
    let (start, stop, days) = match policy.scheduled {
        true => (
            json!(policy.from),
            json!(policy.to),
            json!(policy.days.join(" ")),
        ),
        false => (Value::Null, Value::Null, Value::Null),
    };
    values.insert("start_time".into(), start);
    values.insert("stop_time".into(), stop);
    values.insert("weekdays".into(), days);
    Value::Object(values)
}

/// caps_values is the speed limits as this plugin's own config holds them. A
/// direction left blank is cleared, because a blank is no cap and the option
/// would otherwise still state one.
fn caps_values(policy: &Policy) -> Value {
    let mut values = Map::new();
    values.insert("mac".into(), json!(policy.mac));
    for (option, value) in [("download", &policy.down), ("upload", &policy.up)] {
        values.insert(
            option.into(),
            match value.is_empty() {
                true => Value::Null,
                false => json!(value),
            },
        );
    }
    Value::Object(values)
}

/// rule_name is what the rule calls itself in the firewall's own listing, so
/// someone reading the rules sees a sentence rather than a section handle.
pub fn rule_name(subject: &str) -> String {
    format!("No internet for {subject}")
}

/// RULE_OPTIONS and CAP_OPTIONS are the order each section reads in: what it is,
/// then who it matches, then what happens to them. uci itself keeps no order, and
/// a preview sorted alphabetically would put the verdict before the subject — so
/// the footnote states the order rather than taking whatever the map hands back.
const RULE_OPTIONS: [&str; 8] = [
    "name",
    "src",
    "src_mac",
    "dest",
    "target",
    "start_time",
    "stop_time",
    "weekdays",
];

const CAP_OPTIONS: [&str; 3] = ["mac", "download", "upload"];

/// preview is a config as it will be written, for the footnote under the form.
/// An option the policy states nothing for is left out, because a blank is a
/// removal and the file simply will not carry that line.
fn preview(
    config: &str,
    section: &str,
    section_type: &str,
    order: &[&str],
    values: &Value,
) -> String {
    let handle = match section.is_empty() {
        true => String::new(),
        false => format!(" '{section}'"),
    };
    let mut out = format!("# /etc/config/{config}\nconfig {section_type}{handle}");
    for option in order {
        if let Some(text) = values.get(option).and_then(Value::as_str) {
            out.push_str(&format!("\n\toption {option} '{text}'"));
        }
    }
    out
}

/// rule_preview and caps_preview are the two halves of the policy as uci will
/// hold them — or a line saying why there is nothing to hold.
pub fn rule_preview(policy: &Policy, name: &str) -> String {
    match policy.refuses() {
        true => preview(
            FIREWALL,
            &policy.rule,
            RULE,
            &RULE_OPTIONS,
            &rule_values(policy, name),
        ),
        false => format!("# no rule: {name} reaches the internet like every other device"),
    }
}

pub fn caps_preview(policy: &Policy, name: &str) -> String {
    match policy.capped() {
        true => preview(
            CAPS,
            &policy.caps,
            DEVICE,
            &CAP_OPTIONS,
            &caps_values(policy),
        ),
        false => format!("# no limit: {name} runs at whatever the link offers"),
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::fixture;
    use crate::model::{every_policy, Policy};

    fn held(mac: &str) -> Policy {
        Policy::read(&fixture::snapshot(), mac)
    }

    fn op<'a>(ops: &'a [verso_plugin::CommitOp], config: &str) -> &'a verso_plugin::CommitOp {
        ops.iter()
            .find(|op| op.config == config)
            .unwrap_or_else(|| panic!("no write to {config}"))
    }

    #[test]
    fn the_config_says_what_it_says_however_it_is_spelled() {
        // One MAC is written upper-case and scalar, one lower-case as a uci list;
        // both are the device they name.
        let blocked = held("3C:22:FB:8D:44:A1");
        assert_eq!(blocked.rule, "cfg0b22bb");
        assert!(!blocked.allowed && !blocked.scheduled);
        let curfew = held("e4:5e:1b:9a:2c:63");
        assert!(curfew.allowed && curfew.scheduled);
        assert_eq!(curfew.days, ["Mon", "Tue", "Wed", "Thu", "Fri"]);
        assert_eq!(curfew.down, "25");
        assert_eq!(curfew.up, "");

        // A rule refusing the wan without naming a MAC is nobody's device policy,
        // and neither is one that accepts.
        let policies = every_policy(&fixture::snapshot());
        assert_eq!(policies.len(), 3);
        assert!(policies.iter().all(|p| !p.mac.is_empty()));
    }

    #[test]
    fn a_saved_policy_writes_the_rule_fw4_enforces_and_the_caps_it_cannot() {
        // Blocked outright: the hours are cleared rather than left beside a rule
        // that no longer means them.
        let ops = commits(
            &submitted(&held("e4:5e:1b:9a:2c:63"), &Form::parse("download=25")),
            "steam-deck",
        );
        let rule = op(&ops, FIREWALL);
        assert_eq!(rule.section, "cfg0c33cc");
        assert_eq!(rule.values["name"], "No internet for steam-deck");
        assert_eq!(rule.values["src"], "*");
        assert_eq!(rule.values["dest"], WAN);
        assert_eq!(rule.values["target"], "REJECT");
        assert!(rule.values["start_time"].is_null());
        assert!(rule.values["weekdays"].is_null());

        // Back on the internet with no cap: both halves are removed rather than
        // left saying nothing.
        let cleared = commits(
            &submitted(&held("e4:5e:1b:9a:2c:63"), &Form::parse("allowed=1")),
            "steam-deck",
        );
        assert!(op(&cleared, FIREWALL).delete);
        assert!(op(&cleared, CAPS).delete);

        // A device nothing was said about before gets sections added, not edited.
        let fresh = commits(
            &submitted(
                &held("aa:bb:cc:dd:ee:ff"),
                &Form::parse("allowed=1&scheduled=1&weekdays=Sat&weekdays=Sun&start_time=22:30&stop_time=06:00&download=10"),
            ),
            "guest-laptop",
        );
        let rule = op(&fresh, FIREWALL);
        assert!(rule.section.is_empty() && rule.section_type == RULE);
        assert_eq!(rule.values["weekdays"], "Sat Sun");
        assert_eq!(rule.values["start_time"], "22:30");
        let caps = op(&fresh, CAPS);
        assert!(caps.section.is_empty() && caps.section_type == DEVICE);
        assert_eq!(caps.values["download"], "10");
        assert!(caps.values["upload"].is_null());
    }

    #[test]
    fn a_branch_the_operator_could_not_see_is_not_one_they_chose() {
        // The schedule's controls still post while the device is blocked outright,
        // and the hours still post while the schedule is folded away. Neither is
        // read where the gate above it was closed.
        let blocked = submitted(
            &held("e4:5e:1b:9a:2c:63"),
            &Form::parse("scheduled=1&weekdays=Mon&start_time=08:00"),
        );
        assert!(!blocked.allowed && !blocked.scheduled);
        assert!(commits(&blocked, "steam-deck")
            .iter()
            .any(|op| { op.config == FIREWALL && op.values["start_time"].is_null() }));
    }

    #[test]
    fn what_the_daemons_would_not_read_is_refused() {
        for (value, ok) in [
            ("", true),
            ("50", true),
            ("0", false),
            ("1e6", false),
            ("999999", false),
        ] {
            assert_eq!(rate(value), ok, "{value}");
        }
        for (value, ok) in [
            ("21:00", true),
            ("00:00", true),
            ("23:59", true),
            ("24:00", false),
            ("9:00", false),
            ("21:60", false),
            ("evening", false),
        ] {
            assert_eq!(clock(value), ok, "{value}");
        }
    }
}
