// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

//! Configured devices for the shell's Devices roster, including offline ones.

use verso_plugin::{EntitySummary, EntitySummaryDetail, Envelope, Snapshot, Widget};

use crate::leases::Leases;
use crate::model::{self, Policy, CAPS, FIREWALL};

pub fn read(snapshot: &Snapshot, leases: &Leases) -> Envelope {
    let mut reply = Envelope::page("Limits", Widget::stack(Vec::new()));
    if snapshot.has_config(FIREWALL) && snapshot.has_config(CAPS) {
        reply.entities = Some(
            model::every_policy(snapshot)
                .iter()
                .map(|policy| EntitySummary {
                    id: policy.mac.clone(),
                    name: leases.subject(&policy.mac),
                    // These labels describe saved configuration, never whether
                    // a schedule or speed limit is being enforced right now.
                    state: match (policy.allowed, policy.scheduled) {
                        (false, _) => "Internet block",
                        (true, true) if policy.capped() => "Schedule & speed limit",
                        (true, true) => "Internet schedule",
                        (true, false) => "Speed limit",
                    }
                    .into(),
                    details: details(snapshot, policy),
                })
                .collect(),
        );
    }
    reply
}

fn fact(kind: &str, label: &str, values: Vec<String>) -> EntitySummaryDetail {
    EntitySummaryDetail {
        kind: kind.into(),
        label: label.into(),
        values,
    }
}

fn details(snapshot: &Snapshot, policy: &Policy) -> Vec<EntitySummaryDetail> {
    let mut facts = Vec::new();
    if !policy.allowed {
        facts.push(fact("block", "Internet block", vec!["Always".into()]));
    } else if policy.scheduled {
        facts.push(fact("weekdays", "Blocked days", policy.days.clone()));
        // Read the stored bounds rather than the editor's suggested defaults:
        // a weekdays-only rule blocks all day, and either bound may be absent.
        if let Some(rule) = snapshot.section(FIREWALL, &policy.rule) {
            let from = rule.scalar("start_time");
            let to = rule.scalar("stop_time");
            let (kind, label, hours) = match (from.is_empty(), to.is_empty()) {
                (true, true) => ("hours", "Hours (router time)", "All day".into()),
                (false, true) => ("from", "From (router time)", from),
                (true, false) => ("until", "Until (router time)", to),
                (false, false) => ("hours", "Hours (router time)", format!("{from}–{to}")),
            };
            facts.push(fact(kind, label, vec![hours]));
        }
    }
    if policy.capped() {
        let rate = |value: &str| {
            if value.is_empty() {
                "No limit".into()
            } else {
                format!("{value} Mbit/s")
            }
        };
        facts.push(fact("download", "Download", vec![rate(&policy.down)]));
        facts.push(fact("upload", "Upload", vec![rate(&policy.up)]));
    }
    facts
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::fixture;
    use crate::model::DAYS;
    use serde_json::json;

    #[test]
    fn tooltip_states_the_saved_days_hours_and_both_speed_directions() {
        let reply = read(&fixture::snapshot(), &Leases::read(&fixture::ubus()));
        let summaries = reply.entities.unwrap();
        assert_eq!(
            serde_json::to_value(&summaries[0].details).unwrap(),
            json!([
                {"kind":"block", "label":"Internet block", "values":["Always"]}
            ])
        );
        assert_eq!(
            serde_json::to_value(&summaries[1].details).unwrap(),
            json!([
                {"kind":"weekdays", "label":"Blocked days", "values":["Mon", "Tue", "Wed", "Thu", "Fri"]},
                {"kind":"hours", "label":"Hours (router time)", "values":["21:00–07:00"]},
                {"kind":"download", "label":"Download", "values":["25 Mbit/s"]},
                {"kind":"upload", "label":"Upload", "values":["No limit"]}
            ])
        );
        assert_eq!(
            serde_json::to_value(&summaries[2].details).unwrap(),
            json!([
                {"kind":"download", "label":"Download", "values":["50 Mbit/s"]},
                {"kind":"upload", "label":"Upload", "values":["10 Mbit/s"]}
            ])
        );
    }

    #[test]
    fn missing_hours_are_not_replaced_with_editor_defaults() {
        for (start, stop, label, value) in [
            ("", "", "Hours (router time)", "All day"),
            ("18:00", "", "From (router time)", "18:00"),
            ("", "08:00", "Until (router time)", "08:00"),
        ] {
            let snapshot = Snapshot::from_value(json!({"firewall": {"rule1": {
                ".type":"rule", ".name":"rule1", "src_mac":"02:11:22:33:44:55",
                "dest":"wan", "target":"REJECT", "weekdays":DAYS,
                "start_time":start, "stop_time":stop
            }}, "qos": {}}));
            let summaries = read(&snapshot, &Leases::default()).entities.unwrap();
            assert_eq!(summaries[0].details[0].values, DAYS);
            assert_eq!(summaries[0].details[1].label, label);
            assert_eq!(summaries[0].details[1].values, [value]);
        }
    }
}
