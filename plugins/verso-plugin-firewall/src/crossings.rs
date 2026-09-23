// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

//! Where a zone's traffic may go: the `config forwarding` sections, edited on
//! the zone they leave.
//!
//! A crossing is a section of its own, not an option on either zone, and it is
//! the single thing about an OpenWrt firewall that most often surprises: without
//! one, a zone reaches no other zone at all, whatever its policies say. So it is
//! edited where that question gets asked — on the zone — rather than on a
//! listing of sections nobody goes looking for.
//!
//! The panel is open on one zone, so it edits the crossings **out of** that zone
//! and states the ones pointing at it as facts. A crossing into this zone is the
//! other zone's to edit; offering it at both ends would be one fact in two places
//! with two ways to disagree.
//!
//! The box holds the crossings in force. firewall4 skips a section whose
//! `enabled` is off, so such a crossing reaches nothing and has no business in a
//! list of where traffic goes — and Verso does not delete what it does not show:
//! naming that zone again switches the section back on rather than writing a
//! second one beside it.

use verso_plugin::{
    commit, commit_delete, commit_new, json, CommitOp, Form, Property, SelectOption, Widget,
};

use crate::fields;
use crate::model::{Firewall, Zone, CONFIG};
use crate::rule_form::{cleaned, Errors};

/// DEST is the field each crossing posts under — the option it writes on the
/// section it stands for. The source is the zone whose panel is open, so it is
/// not asked for.
pub const DEST: &str = "dest";

/// ANYWHERE is firewall4's wildcard zone reference. A crossing written with it
/// reaches every zone at once; the picker does not offer it, because choosing it
/// by accident is choosing to forward everything, but a config that states it is
/// still a config this panel has to be able to save.
const ANYWHERE: &str = "*";

const LABEL: &str = "Reaches";

/// KEY is what rides in the mono chip beside the label. Every other row names the
/// option it writes; this one writes whole sections, and `forwarding` is what
/// they are called in the file.
const KEY: &str = "forwarding";

const PROMPT: &str = "Zone name";

const HELP: &str = "Traffic may leave this zone for the zones named here, and for no others. \
Removing one deletes its section.";

const REACHED_BY: &str = "Reached by";

const REACHED_BY_NOTE: &str = "Set on those zones, not here — a crossing belongs to the zone its \
traffic leaves.";

const UNNAMED_NOTE: &str = "A crossing names its two zones by name, and this zone has none. Give \
it a name in the config first.";

const NEW_NOTE: &str = "A zone reaches nowhere until a crossing says so. Name the zones this one \
may reach and they are written with it.";

const UNKNOWN_ZONE: &str = "Name a zone this router defines.";

/// Crossings is where one zone's traffic may go, as the panel holds it: the far
/// end of each crossing out of this zone, once each.
#[derive(Default, Clone)]
pub struct Crossings {
    pub reaches: Vec<String>,
}

impl Crossings {
    /// read is the crossings in force out of one zone.
    pub fn read(model: &Firewall, zone: &str) -> Crossings {
        Crossings {
            reaches: model.forwards_from(zone),
        }
    }

    /// submitted reads what the box posted. The same zone named twice is one
    /// crossing — the config can hold two sections saying it, and the box says
    /// it once.
    pub fn submitted(form: &Form) -> Crossings {
        let mut reaches: Vec<String> = Vec::new();
        for zone in cleaned(form.all(DEST)) {
            if !reaches.contains(&zone) {
                reaches.push(zone);
            }
        }
        Crossings { reaches }
    }

    /// validate answers firewall4's question before the write: `src` and `dest`
    /// are both mandatory on a forwarding, and a name no zone answers to leaves
    /// the section skipped with nothing said to the operator.
    pub fn validate(&self, zones: &[String]) -> Errors {
        let mut errors = Errors::default();
        errors.items(DEST, &self.reaches, |zone| {
            match zone == ANYWHERE || zones.iter().any(|known| known == zone) {
                true => Ok(()),
                false => Err(UNKNOWN_ZONE),
            }
        });
        errors
    }

    /// commits is what the save writes beside the zone's own: one section added
    /// per crossing this zone did not have, one deleted per crossing it no
    /// longer states, and — for a crossing the file holds switched off — the one
    /// option that puts it back in force.
    ///
    /// A zone with no name writes nothing: firewall4 needs both ends of a
    /// crossing named, and a section stating an empty zone is one it skips.
    pub fn commits(&self, model: &Firewall, zone: &str) -> Vec<CommitOp> {
        let mut ops = Vec::new();
        if zone.is_empty() {
            return ops;
        }
        let existing = model.forwardings_from(zone);
        for forwarding in existing.iter().filter(|forwarding| forwarding.enabled) {
            if !self.reaches.contains(&forwarding.dest) {
                ops.push(commit_delete(CONFIG, &forwarding.section));
            }
        }
        for dest in &self.reaches {
            if existing
                .iter()
                .any(|forwarding| forwarding.enabled && &forwarding.dest == dest)
            {
                continue;
            }
            match existing.iter().find(|forwarding| &forwarding.dest == dest) {
                Some(forwarding) => {
                    ops.push(commit(CONFIG, &forwarding.section, json!({"enabled": "1"})))
                }
                None => ops.push(commit_new(
                    CONFIG,
                    "forwarding",
                    json!({"src": zone, "dest": dest}),
                )),
            }
        }
        ops
    }

    /// carried is the crossings as the hidden fields that post them back
    /// unchanged from the readings that do not show them. One field per value:
    /// a single space-joined one would arrive as one zone with a space in its
    /// name, and the save would delete every crossing to write that.
    pub fn carried(&self) -> Vec<Widget> {
        self.reaches
            .iter()
            .map(|zone| Widget::hidden(DEST, zone))
            .collect()
    }

    /// uci_block is the sections this panel writes for the crossings, as the
    /// file spells them — beneath the zone's own block, because a save from this
    /// reading writes both.
    pub fn uci_block(&self, zone: &str) -> String {
        self.reaches
            .iter()
            .map(|dest| {
                format!("\nconfig forwarding\n\toption src '{zone}'\n\toption dest '{dest}'\n")
            })
            .collect()
    }
}

/// fields is the reading itself: the crossings out of this zone as a control,
/// and the ones pointing at it as a fact.
pub fn fields(
    model: &Firewall,
    zone: Option<&Zone>,
    crossings: &Crossings,
    errors: &Errors,
) -> Vec<Widget> {
    let mut out = Vec::new();
    let name = zone.map(|zone| zone.name.as_str()).unwrap_or_default();
    // A zone whose section carries no name cannot be either end of a crossing,
    // so there is nothing to offer: firewall4 would skip whatever was written.
    if zone.is_some() && name.is_empty() {
        return vec![Widget::text(UNNAMED_NOTE)];
    }
    out.push(
        fields::token_list(DEST, LABEL, PROMPT, HELP, &crossings.reaches, errors)
            .writes(KEY)
            .suggesting(options(model, name, &crossings.reaches)),
    );
    if zone.is_none() {
        out.push(Widget::text(NEW_NOTE));
        return out;
    }
    out.push(Widget::properties(vec![Property {
        label: REACHED_BY.into(),
        value: list_or_none(&model.forwards_into(name)),
        mono: true,
        ..Property::default()
    }]));
    out.push(Widget::text(REACHED_BY_NOTE));
    out
}

/// options is what the picker offers: every other zone this config defines. A
/// zone cannot usefully cross into itself — traffic between its own networks is
/// its `forward` policy, one tab away — so it is not among them, and neither is
/// firewall4's "anywhere", which is a decision to forward everything and not one
/// to make by picking the first entry in a list.
///
/// A zone the crossings already name that the config no longer defines stays on
/// offer, so a stale crossing reads as what it says rather than disappearing
/// from the box that holds it.
fn options(model: &Firewall, zone: &str, reaches: &[String]) -> Vec<SelectOption> {
    let mut names: Vec<String> = model
        .zone_names()
        .into_iter()
        .filter(|name| name != zone)
        .collect();
    for named in reaches {
        if named != ANYWHERE && !names.iter().any(|name| name == named) {
            names.push(named.clone());
        }
    }
    names
        .iter()
        .map(|name| SelectOption::new(name, name))
        .collect()
}

fn list_or_none(zones: &[String]) -> String {
    match zones.is_empty() {
        true => "—".to_string(),
        false => zones.join(" · "),
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::fixture;
    use serde_json::Value;

    fn ops(zone: &str, reaches: &[&str]) -> Value {
        let model = fixture::firewall();
        let crossings = Crossings {
            reaches: reaches.iter().map(|zone| zone.to_string()).collect(),
        };
        serde_json::to_value(crossings.commits(&model, zone)).expect("serialize")
    }

    /// The box is where a zone's traffic may go, which is not the same as which
    /// sections the file holds: firewall4 skips a crossing that is switched off,
    /// so the guest zone reaches nothing through the one it has.
    #[test]
    fn the_box_holds_the_crossings_in_force() {
        let model = fixture::firewall();
        assert_eq!(
            Crossings::read(&model, "lan").reaches,
            vec!["wan".to_string()]
        );
        assert!(Crossings::read(&model, "guest").reaches.is_empty());
    }

    /// A zone named here and nowhere in the file is a section of its own, stating
    /// the two zones it joins and nothing else.
    #[test]
    fn naming_a_zone_writes_a_crossing_of_its_own() {
        assert_eq!(
            ops("lan", &["wan", "guest"]),
            serde_json::json!([{
                "config": "firewall",
                "section": "",
                "type": "forwarding",
                "values": {"src": "lan", "dest": "guest"}
            }]),
            "the crossing lan already has is left alone; only the new one is written"
        );
    }

    /// And taking one out removes the section. Switching it off would leave the
    /// zone reaching somewhere on paper, which is the state this box exists to
    /// make unambiguous.
    #[test]
    fn taking_a_zone_out_deletes_the_section() {
        assert_eq!(
            ops("lan", &[]),
            serde_json::json!([{
                "config": "firewall",
                "section": "cfg05ad58",
                "delete": true
            }])
        );
    }

    /// A crossing the file holds switched off is the one case where naming a zone
    /// writes an option rather than a section. A second section saying the same
    /// thing would leave firewall4 rendering the crossing twice and the operator
    /// with two places to turn it off again.
    #[test]
    fn a_crossing_the_file_holds_switched_off_is_switched_back_on() {
        assert_eq!(
            ops("guest", &["wan"]),
            serde_json::json!([{
                "config": "firewall",
                "section": "guest_to_wan_off",
                "values": {"enabled": "1"}
            }])
        );
    }

    /// A zone with no name cannot be either end of a crossing — firewall4 needs
    /// both, and skips the section that states neither — so nothing is written
    /// rather than a section that would be ignored.
    #[test]
    fn a_zone_with_no_name_writes_no_crossings() {
        assert_eq!(ops("", &["wan"]), serde_json::json!([]));
    }

    /// The same zone twice is one crossing. A config may hold two sections saying
    /// it; the box says it once, and a save that echoed the box back would
    /// otherwise add a third.
    #[test]
    fn the_same_zone_named_twice_is_one_crossing() {
        let form = Form::parse("dest=wan&dest=guest&dest=wan&dest=+");
        assert_eq!(
            Crossings::submitted(&form).reaches,
            vec!["wan".to_string(), "guest".to_string()]
        );
    }

    /// firewall4 needs both ends of a crossing to name a zone it knows: one that
    /// does not is a section skipped with nothing said to the operator, so it is
    /// refused here instead — against the value that caused it, which is how a
    /// box of values says which one is wrong.
    #[test]
    fn a_zone_this_config_does_not_define_is_refused() {
        let model = fixture::firewall();
        let crossings = Crossings {
            reaches: vec!["wan".into(), "nowhere".into()],
        };
        let errors = crossings.validate(&model.zone_names());
        assert!(!errors.is_empty());
        assert_eq!(
            errors.list(DEST).get("1").map(String::as_str),
            Some(UNKNOWN_ZONE)
        );
        assert!(!errors.list(DEST).contains_key("0"));

        // firewall4's own wildcard is a zone reference like any other: a config
        // that states it is one this panel can still save.
        let anywhere = Crossings {
            reaches: vec![ANYWHERE.into()],
        };
        assert!(anywhere.validate(&model.zone_names()).is_empty());
    }

    /// The picker offers the other zones and not this one: traffic between a
    /// zone's own networks is its forward policy, which is a control one tab
    /// away and would be the same fact twice.
    #[test]
    fn the_picker_offers_every_zone_but_this_one() {
        let model = fixture::firewall();
        let offered: Vec<String> = options(&model, "lan", &[])
            .into_iter()
            .map(|option| option.value)
            .collect();
        assert_eq!(offered, vec!["wan", "guest", "tailscale"]);
    }
}
