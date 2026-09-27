// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

//! A zone's new name, carried to everything in the firewall config that names it.
//!
//! A zone is referenced by name and by nothing else, so renaming one is not one
//! write but as many as there are sections pointing at it: rules and forwards by
//! `src` and `dest`, a forward's NAT reflection by its `reflection_zone` list,
//! crossings by both ends, and source NAT by `src`. They are staged together with
//! the zone's own save, so they apply — and roll back — as one change.
//!
//! What lies outside the firewall config cannot follow: an nftables include that
//! names a chain firewall4 built from the old name, or another package's config
//! naming the zone. The panel says so where the name is typed.

use verso_plugin::{commit, json, CommitOp, Map, Value};

use crate::model::{named_zone, redirect_chain, rule_chain, Firewall, CONFIG};

/// rename rewrites every section of the firewall config that names zone `from`
/// so it names `to`, in the model and as the writes that stage it. The model is
/// changed too, so whatever is computed from it afterwards — the crossings a
/// save writes, the panel it answers with — already reads the new name.
///
/// Neither of firewall4's wildcards is a zone, so a section stating no zone or
/// `*` is never rewritten, and renaming one of them is no rename at all.
pub fn rename(model: &mut Firewall, from: &str, to: &str) -> Vec<CommitOp> {
    if !named_zone(from) || from == to {
        return Vec::new();
    }
    let mut writes = Writes::default();

    for zone in model.zones.iter_mut().filter(|zone| zone.name == from) {
        zone.name = to.to_string();
    }
    for rule in &mut model.rules {
        writes.follow(&rule.section, "src", &mut rule.src, from, to);
        writes.follow(&rule.section, "dest", &mut rule.dest, from, to);
        rule.chain = rule_chain(&rule.target, &rule.src, &rule.dest);
    }
    for redirect in &mut model.redirects {
        writes.follow(&redirect.section, "src", &mut redirect.src, from, to);
        writes.follow(&redirect.section, "dest", &mut redirect.dest, from, to);
        // The reflection list is one option, so it is written whole: the other
        // zones it names stay where they were.
        if redirect.reflection_zones.iter().any(|zone| zone == from) {
            for zone in redirect
                .reflection_zones
                .iter_mut()
                .filter(|zone| *zone == from)
            {
                *zone = to.to_string();
            }
            writes.set(
                &redirect.section,
                "reflection_zone",
                json!(redirect.reflection_zones.clone()),
            );
        }
        redirect.chain = redirect_chain(&redirect.target, &redirect.src);
    }
    for forwarding in &mut model.forwardings {
        writes.follow(&forwarding.section, "src", &mut forwarding.src, from, to);
        writes.follow(&forwarding.section, "dest", &mut forwarding.dest, from, to);
    }
    for nat in &mut model.nats {
        writes.follow(&nat.section, "src", &mut nat.src, from, to);
    }
    writes.ops()
}

/// Writes gathers the options a rename changes, section by section in the order
/// they are met, so a section naming the zone twice is one write.
#[derive(Default)]
struct Writes {
    sections: Vec<(String, Map<String, Value>)>,
}

impl Writes {
    /// follow points one option at the new name when it names the old one.
    fn follow(&mut self, section: &str, option: &str, value: &mut String, from: &str, to: &str) {
        if value == from {
            *value = to.to_string();
            self.set(section, option, json!(to));
        }
    }

    fn set(&mut self, section: &str, option: &str, value: Value) {
        let index = match self.sections.iter().position(|(name, _)| name == section) {
            Some(index) => index,
            None => {
                self.sections.push((section.to_string(), Map::new()));
                self.sections.len() - 1
            }
        };
        self.sections[index].1.insert(option.to_string(), value);
    }

    fn ops(self) -> Vec<CommitOp> {
        self.sections
            .into_iter()
            .map(|(section, values)| commit(CONFIG, &section, Value::Object(values)))
            .collect()
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::fixture;
    use crate::model::References;
    use serde_json::{json, Value};

    fn staged(ops: &[CommitOp]) -> Vec<Value> {
        ops.iter()
            .map(|op| serde_json::to_value(op).expect("serialize"))
            .collect()
    }

    /// The guest zone is named by every kind of section that can name a zone:
    /// rules by their source, a forward by its source and another by its
    /// reflection list, a source NAT, and a crossing. Each is rewritten once,
    /// stating only the options that named it.
    #[test]
    fn every_section_naming_the_zone_follows_its_new_name() {
        let mut model = fixture::firewall();
        let before = model.references("guest");
        let ops = rename(&mut model, "guest", "visitors");
        assert_eq!(
            staged(&ops),
            vec![
                json!({"config": "firewall", "section": "allow_dns_guest", "values": {"src": "visitors"}}),
                json!({"config": "firewall", "section": "block_telnet", "values": {"src": "visitors"}}),
                json!({"config": "firewall", "section": "family_to_printer", "values": {"src": "visitors"}}),
                json!({"config": "firewall", "section": "force_dns_guest", "values": {"src": "visitors"}}),
                // The list is written whole, in its order, with the other zone
                // it names left where it was.
                json!({"config": "firewall", "section": "https_to_nas", "values": {"reflection_zone": ["lan", "visitors"]}}),
                json!({"config": "firewall", "section": "guest_to_wan_off", "values": {"src": "visitors"}}),
                json!({"config": "firewall", "section": "guest_source_nat", "values": {"src": "visitors"}}),
            ]
        );
        assert_eq!(model.references("visitors"), before);
        assert_eq!(model.references("guest"), References::default());
        assert!(model.zone_names().contains(&"visitors".to_string()));
        assert!(!model.zone_names().contains(&"guest".to_string()));
    }

    /// A section naming the zone at both ends, or by its path and its reflection
    /// list, is one section and one write carrying every option that changed.
    #[test]
    fn a_section_naming_the_zone_twice_is_written_once() {
        let mut model = fixture::firewall();
        let ops = staged(&rename(&mut model, "lan", "home"));
        let sections: Vec<&str> = ops
            .iter()
            .map(|op| op["section"].as_str().unwrap_or_default())
            .collect();
        assert_eq!(
            sections,
            vec![
                "allow_isakmp",
                "family_to_printer",
                "nas_snat",
                "https_to_nas",
                "cfg05ad58"
            ]
        );
        assert_eq!(ops[1]["values"], json!({"dest": "home"}));
        assert_eq!(
            ops[3]["values"],
            json!({"reflection_zone": ["home", "guest"]})
        );
        // The crossing out of lan now leaves from home, which is what the
        // crossings' own save has to find to leave it alone.
        assert_eq!(model.forwards_from("home"), vec!["wan".to_string()]);
    }

    /// Keeping the name is no rename, and neither of firewall4's wildcards is a
    /// zone: a section stating no zone, or `*`, is not about one to rewrite.
    #[test]
    fn nothing_is_written_when_nothing_is_renamed() {
        let mut model = fixture::firewall();
        assert!(rename(&mut model, "lan", "lan").is_empty());
        assert!(rename(&mut model, "", "somewhere").is_empty());
        assert!(rename(&mut model, "*", "somewhere").is_empty());
        assert_eq!(model.references("somewhere"), References::default());
    }
}
