// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

//! Every option firewall4 supports is accounted for: this page renders it, or we
//! left it out on purpose and said why.
//!
//! The third state — nobody thought about it — is what this file abolishes. It had
//! gone unnoticed for thirteen options, two of which firewall4 defaults to ON, so
//! a port forward was doing something no screen mentioned and a settings page
//! listed the include files above the switch that decides whether they run.
//!
//! What the UI renders is read from the widget trees the plugin actually builds,
//! never from a list kept beside them. A list of "options we render" would be one
//! more copy to drift, and drift is the whole disease.

use std::collections::BTreeSet;

use verso_plugin::vocabulary::{Composition, Omission};

/// COMPOSITIONS are the options several controls write between them.
///
/// firewall4 gives a few options a small grammar of their own. A form that asked
/// for the whole string would be asking a person to write `!0x10/0xff`, so the
/// editor takes it apart and puts it back together on save. Each of these IS
/// rendered; it is simply not rendered under its own name, and the fields named
/// here are checked against the screen so the claim cannot outlive the controls.
pub const COMPOSITIONS: &[Composition] = &[
    Composition {
        section: "rule",
        key: "mark",
        fields: &["mark_match", "mark_value", "mark_mask"],
        because: "a mark match is a comparison, a value and an optional mask, which \
                  firewall4 spells as one '!value/mask' string.",
    },
    Composition {
        section: "rule",
        key: "set_xmark",
        fields: &["mark_operation", "set_mark", "set_mark_mask"],
        because: "firewall4 has two ways to write a mark and they differ only in the \
                  operation, so the editor asks which operation and writes whichever \
                  option means it.",
    },
    Composition {
        section: "rule",
        key: "utc_time",
        fields: &["time_basis"],
        because: "the window is read against one clock or the other, which is a choice \
                  between two things rather than a switch with an off — and firewall4 \
                  spells the second of them as a truth value.",
    },
    // A port forward matches on a mark and reads a clock the same way a rule
    // does, out of the same catalogue — so it takes the same option apart into
    // the same fields, under its own section type.
    Composition {
        section: "redirect",
        key: "mark",
        fields: &["mark_match", "mark_value", "mark_mask"],
        because: "a mark match is a comparison, a value and an optional mask, which \
                  firewall4 spells as one '!value/mask' string.",
    },
    Composition {
        section: "redirect",
        key: "utc_time",
        fields: &["time_basis"],
        because: "the window is read against one clock or the other, which is a choice \
                  between two things rather than a switch with an off — and firewall4 \
                  spells the second of them as a truth value.",
    },
];

/// OMISSIONS is every supported option this UI does not render, and why.
///
/// A line here is a decision somebody made and signed. The reason is the point:
/// the next person can disagree with it, but they cannot mistake it for an
/// oversight. [`verso_plugin::vocabulary::stale_omissions`] deletes the lines that
/// stop being true — an option firewall4 drops, or one we later grow a control
/// for — so the table cannot rot into noise nobody trusts.
pub const OMISSIONS: &[Omission] = &[
    // ---- aliases firewall4 keeps for older configs ----
    Omission {
        section: "defaults",
        key: "syn_flood",
        because: "firewall4's legacy spelling of synflood_protect: it copies the value \
                  across and deletes this one. Rendering both would put one fact on \
                  screen twice.",
    },
    // ---- the section types this plugin does not edit ----
    // A config's options are not all one UI's business. These sections are read
    // (the listings show what they say) but edited nowhere, so every option in
    // them is out of scope rather than missing — and the reason is the same one,
    // which is why they are grouped rather than repeated.
    Omission {
        section: "nat",
        key: Omission::EVERY,
        because: "source NAT is not edited by this plugin: the port-forward listing \
                  shows the dnat direction, and a snat rule answers a different \
                  question that has no page yet.",
    },
    Omission {
        section: "ipset",
        key: Omission::EVERY,
        because: "a named set is not edited here: rules match against sets someone \
                  else maintains, and the rule editor offers the sets the config \
                  already declares.",
    },
    Omission {
        section: "include",
        key: Omission::EVERY,
        because: "an include names a file of hand-written nftables: the settings page \
                  states which files are loaded, and editing their contents is a file \
                  editor's job, not a form's.",
    },
    // ---- the crossings, which are edited as membership and nothing else ----
    // A `config forwarding` is one direction between two zones, and the zone
    // panel edits which zones this one reaches. What that leaves out is one
    // decision taken four times, because each of the four has its own reason for
    // being absent from a box that holds zone names.
    Omission {
        section: "forwarding",
        key: "src",
        because: "a crossing is edited on the zone its traffic leaves, so the source is \
                  the zone whose panel is open. Asking for it again would let somebody \
                  write a crossing out of a zone they are not looking at.",
    },
    Omission {
        section: "forwarding",
        key: "enabled",
        because: "a crossing is in force or it is not there. The box holds the ones in \
                  force: naming a zone the file holds switched off switches that section \
                  back on rather than writing a second one beside it, and taking a zone \
                  out deletes the section.",
    },
    Omission {
        section: "forwarding",
        key: "name",
        because: "firewall4 identifies a crossing by the two zones it joins, which is \
                  what the box shows. A second label for the same fact is one more thing \
                  to keep true.",
    },
    Omission {
        section: "forwarding",
        key: "family",
        because: "the box edits which zones this one reaches and rewrites nothing else \
                  about a crossing, so one an expert narrowed to a single address family \
                  keeps saying so.",
    },
    // ---- the direction this plugin does not write ----
    Omission {
        section: "redirect",
        key: "target",
        because: "firewall4 covers both directions with one section type and this editor \
                  writes only dnat — the port forward. A source rewrite answers a \
                  different question, is not what the listing shows, and would be a \
                  second page rather than a choice on this one; a new section states \
                  dnat and an existing one keeps whatever direction it has.",
    },
];

/// NOT_BUILT is what firewall4 supports and this UI has not got to yet.
///
/// It is deliberately not part of OMISSIONS. An omission is a decision somebody
/// made; these are a debt somebody owes, and conflating the two would let "we never
/// thought about it" hide inside a table of reasons. The list only goes down: a new
/// unaccounted option fails the reckoning, and an entry here that becomes reachable
/// fails it too, so the debt can neither grow quietly nor be paid off without the
/// line going away.
///
/// Reading it: the list is empty. Every option firewall4 supports in this config
/// is rendered, composed, or left out for a reason somebody wrote down.
///
/// It held twenty-two a moment ago, all of them the port forward's, and what
/// emptied it is the same thing that kept the rule editor off it from the start:
/// an open catalogue. The three editors that were written as a closed list of
/// fields each grew a tail of options nobody had a place to put; the one built
/// around a catalogue never did, because adding an option to it is adding an
/// entry rather than finding room on a page.
///
/// The list stays here rather than being deleted with its last entry. It is a
/// ratchet, and a ratchet with nothing in it is the state worth keeping: a new
/// unaccounted option has somewhere to be written down, and the test that guards
/// it says the same thing whether the list is empty or not.
const NOT_BUILT: &[&str] = &[];

/// rendered collects every field name the UI writes, by section type, out of the
/// widget trees themselves.
///
/// A name here is whatever the form posts, which for this config is the uci option
/// it sets — the carriers are named for the options they carry, which is what makes
/// this readable as coverage at all.
pub fn rendered(trees: &[(&'static str, &serde_json::Value)]) -> BTreeSet<(&'static str, String)> {
    let mut out = BTreeSet::new();
    for (kind, tree) in trees {
        let mut names = Vec::new();
        collect(tree, &mut names);
        for name in names {
            out.insert((*kind, name));
        }
    }
    out
}

/// collect walks a widget tree for every `name` a control declares. It does not
/// care which widget carries it: a field, a token list, a switch, a hidden carrier
/// and a condition's children all name the option they write, and all of them count
/// as rendering it.
fn collect(node: &serde_json::Value, out: &mut Vec<String>) {
    match node {
        serde_json::Value::Object(map) => {
            if let Some(serde_json::Value::String(name)) = map.get("name") {
                if !name.is_empty() {
                    out.push(name.clone());
                }
            }
            for value in map.values() {
                collect(value, out);
            }
        }
        serde_json::Value::Array(items) => {
            for item in items {
                collect(item, out);
            }
        }
        _ => {}
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::redirect_form::RedirectForm;
    use crate::rule_form::{Errors, RuleForm};
    use crate::vocabulary::FIREWALL;
    use crate::zone_form::ZoneForm;
    use crate::{crossings, fixture, redirects, rules, settings, zone_drawer, zones};
    use verso_plugin::vocabulary::{
        reckon, stale_compositions, stale_omissions, Cardinality, Provenance,
    };
    use verso_plugin::Form;

    /// trees builds every screen that edits this config and hands back the widget
    /// trees, paired with the section type each one writes. This is the whole of
    /// what "the UI renders" means here: if a screen is added and not listed, its
    /// options read as unaccounted, which is the right way round — a screen nobody
    /// declared is a screen nobody is checking.
    fn trees() -> Vec<(&'static str, serde_json::Value)> {
        let snapshot = fixture::snapshot();
        let model = fixture::firewall();
        let counters = fixture::counters();
        let mut out = Vec::new();

        // The rule panel, once per reading: each shows a third of the rule and
        // carries the rest as hidden fields, so all three together are the rule.
        for tab in ["match", "action", "when"] {
            let query = Form::parse(&format!("open=allow_ping&tab={tab}"));
            let envelope = rules::page_open(&snapshot, &model, &counters, &query);
            out.push((
                "rule",
                serde_json::to_value(&envelope).expect("serialize the rule panel"),
            ));
        }
        // And the blank one, which is where a new rule's defaults are decided.
        let query = Form::parse("open=new&tab=match");
        let envelope = rules::page_open(&snapshot, &model, &counters, &query);
        out.push((
            "rule",
            serde_json::to_value(&envelope).expect("serialize the new-rule panel"),
        ));

        out.push((
            "defaults",
            serde_json::to_value(settings::page(&model)).expect("serialize the settings page"),
        ));
        // The zone panel, once per reading, and the blank one. Making a zone and
        // changing one are different screens and they do not offer the same things —
        // a zone's name is settled when it is created and immutable after, because
        // every rule naming it would follow it — so reading only one path
        // under-reports what the UI can write.
        for tab in ["traffic", "reaches", "advanced"] {
            let query = Form::parse(&format!("open=cfg02dc81&tab={tab}"));
            out.push((
                "zone",
                serde_json::to_value(zones::page_open(&snapshot, &model, &query))
                    .expect("serialize the zone panel"),
            ));
        }
        let query = Form::parse("open=new&tab=traffic");
        out.push((
            "zone",
            serde_json::to_value(zones::page_open(&snapshot, &model, &query))
                .expect("serialize the new-zone panel"),
        ));
        // The zone panel's Reaches reading writes `config forwarding` sections
        // rather than the zone's own options, so it is read as the section type
        // it writes. It is the controls themselves rather than the whole panel:
        // the panel around them carries the zone's every option as a hidden
        // carrier, and three of those share a name with a forwarding option —
        // paired whole, the panel would vouch for options nothing on it writes.
        let zone = model
            .zones
            .iter()
            .find(|zone| zone.section == "cfg02dc81")
            .expect("the fixture's lan zone");
        let reaches = crossings::Crossings::read(&model, &zone.name);
        out.push((
            "forwarding",
            serde_json::to_value(zone_drawer::reaches_fields(
                &model,
                Some(zone),
                &reaches,
                &Errors::default(),
            ))
            .expect("serialize the crossings editor"),
        ));
        // The port-forward panel, open on a forward and blank.
        for open in ["https_to_nas", "new"] {
            let query = Form::parse(&format!("open={open}"));
            out.push((
                "redirect",
                serde_json::to_value(redirects::page_open(&snapshot, &model, &counters, &query))
                    .expect("serialize the port-forward panel"),
            ));
        }
        out
    }

    /// Every option firewall4 supports is in one of the three states. An option in
    /// none of them fails here with its name, its datatype and what firewall4 does
    /// in its absence — which is the decision to make: render it, or write down why
    /// not.
    #[test]
    fn every_option_firewall4_supports_is_accounted_for() {
        let trees = trees();
        let pairs: Vec<(&'static str, &serde_json::Value)> =
            trees.iter().map(|(kind, tree)| (*kind, tree)).collect();
        let rendered = rendered(&pairs);
        let borrowed: std::collections::BTreeSet<(&str, &str)> = rendered
            .iter()
            .map(|(kind, name)| (*kind, name.as_str()))
            .collect();

        let unaccounted = reckon(&FIREWALL, &borrowed, OMISSIONS, COMPOSITIONS);
        let found: Vec<String> = unaccounted
            .iter()
            .map(|o| format!("{}.{}", o.section, o.key))
            .collect();
        let baseline: Vec<String> = NOT_BUILT.iter().map(|s| s.to_string()).collect();

        let fresh: Vec<&String> = found.iter().filter(|k| !baseline.contains(k)).collect();
        let fixed: Vec<&String> = baseline.iter().filter(|k| !found.contains(k)).collect();

        if !fresh.is_empty() {
            let mut report = String::from(
                "firewall4 supports option(s) that nothing on screen writes and nothing \
                 accounts for.\n\nGive each a control, or a line in OMISSIONS saying why \
                 not. If it is simply not built yet, add it to NOT_BUILT — but read what \
                 firewall4 assumes in its absence first, because a default it acts on is \
                 the config doing something no screen says:\n\n",
            );
            for option in unaccounted
                .iter()
                .filter(|o| fresh.contains(&&format!("{}.{}", o.section, o.key)))
            {
                let fallback = match &option.default {
                    Some(value) => format!("  firewall4 assumes '{value}' when it is absent"),
                    None => String::new(),
                };
                report.push_str(&format!(
                    "  {:<10} {:<22} {}{}\n",
                    option.section, option.key, option.datatype, fallback
                ));
            }
            panic!("{report}");
        }
        assert!(
            fixed.is_empty(),
            "NOT_BUILT names option(s) that are now accounted for. Delete these lines — \
             the list only goes down:\n  {}",
            fixed
                .iter()
                .map(|k| k.as_str())
                .collect::<Vec<_>>()
                .join("\n  ")
        );
    }

    /// And the table cannot rot: an omission naming an option firewall4 has dropped,
    /// or one the UI has since grown a control for, is a note that has stopped being
    /// true and has to go.
    #[test]
    fn no_omission_has_stopped_being_true() {
        let trees = trees();
        let pairs: Vec<(&'static str, &serde_json::Value)> =
            trees.iter().map(|(kind, tree)| (*kind, tree)).collect();
        let rendered = rendered(&pairs);
        let borrowed: std::collections::BTreeSet<(&str, &str)> = rendered
            .iter()
            .map(|(kind, name)| (*kind, name.as_str()))
            .collect();

        let stale = stale_omissions(&FIREWALL, &borrowed, OMISSIONS);
        let report: Vec<String> = stale
            .iter()
            .map(|(omission, why)| format!("  {}.{} {why}", omission.section, omission.key))
            .collect();
        assert!(
            stale.is_empty(),
            "OMISSIONS holds {} entry/entries that are no longer true:\n{}",
            stale.len(),
            report.join("\n")
        );

        // And the compositions, which rot the more dangerous way: one vouches for an
        // option nothing writes under its own name, so a field quietly renamed turns
        // "rendered" back into "missing" while the claim keeps saying otherwise.
        let broken = stale_compositions(&FIREWALL, &borrowed, COMPOSITIONS);
        let report: Vec<String> = broken
            .iter()
            .map(|(composition, why)| {
                format!("  {}.{} {why}", composition.section, composition.key)
            })
            .collect();
        assert!(
            broken.is_empty(),
            "COMPOSITIONS holds {} claim(s) that are no longer true:\n{}",
            broken.len(),
            report.join("\n")
        );
    }

    /// The artifact says where it came from, and says it in a way a reader can
    /// check. A vocabulary with no provenance is an assertion wearing the clothes of
    /// a measurement.
    #[test]
    fn the_vocabulary_states_what_it_was_read_from() {
        match FIREWALL.provenance {
            Provenance::Extracted {
                upstream,
                version,
                sha256,
                extractor,
            } => {
                assert_eq!(upstream, "firewall4");
                assert!(!version.is_empty(), "the upstream version is unrecorded");
                assert_eq!(
                    sha256.len(),
                    64,
                    "the source hash is not a sha256: {sha256}"
                );
                assert!(sha256.chars().all(|c| c.is_ascii_hexdigit()));
                assert!(!extractor.is_empty(), "the extractor is unrecorded");
            }
            Provenance::Declared { .. } => {
                panic!("firewall4 publishes its own option table; this should be extracted")
            }
        }
    }

    /// The artifact describes one exact file, and this is where that claim is
    /// checked: point VERSO_FW4 at the fw4.uc the firmware actually ships and the
    /// recorded hash must still match it. A firmware that moved while the artifact
    /// stood still fails here.
    ///
    /// It is opt-in because the only machine that has an OpenWrt build tree is the
    /// one that built the firmware; CI has the artifact and no tree, and a test that
    /// silently passed for want of its input would be worse than none. When the
    /// variable is unset it says so rather than pretending.
    #[test]
    fn the_recorded_hash_still_matches_the_firmware() {
        let Ok(path) = std::env::var("VERSO_FW4") else {
            eprintln!(
                "VERSO_FW4 unset: the recorded fw4.uc hash was not checked against any \
                 firmware. Point it at <build_dir>/firewall4-*/root/usr/share/ucode/fw4.uc \
                 to check it."
            );
            return;
        };
        let source = std::fs::read(&path).expect("read the fw4.uc named by VERSO_FW4");
        let Provenance::Extracted {
            sha256, version, ..
        } = FIREWALL.provenance
        else {
            panic!("the firewall vocabulary is not an extracted one");
        };
        assert_eq!(
            sha256,
            crate::reckoning::tests::digest(&source),
            "the vocabulary was read from firewall4 {version}, and {path} is not that \
             file. Re-run the extractor, read the diff, and commit it."
        );
    }

    /// digest is the same SHA-256 the extractor records, so the check above compares
    /// like with like. It is small enough to state twice and not worth a dependency
    /// in either place.
    pub(super) fn digest(data: &[u8]) -> String {
        const K: [u32; 64] = [
            0x428a2f98, 0x71374491, 0xb5c0fbcf, 0xe9b5dba5, 0x3956c25b, 0x59f111f1, 0x923f82a4,
            0xab1c5ed5, 0xd807aa98, 0x12835b01, 0x243185be, 0x550c7dc3, 0x72be5d74, 0x80deb1fe,
            0x9bdc06a7, 0xc19bf174, 0xe49b69c1, 0xefbe4786, 0x0fc19dc6, 0x240ca1cc, 0x2de92c6f,
            0x4a7484aa, 0x5cb0a9dc, 0x76f988da, 0x983e5152, 0xa831c66d, 0xb00327c8, 0xbf597fc7,
            0xc6e00bf3, 0xd5a79147, 0x06ca6351, 0x14292967, 0x27b70a85, 0x2e1b2138, 0x4d2c6dfc,
            0x53380d13, 0x650a7354, 0x766a0abb, 0x81c2c92e, 0x92722c85, 0xa2bfe8a1, 0xa81a664b,
            0xc24b8b70, 0xc76c51a3, 0xd192e819, 0xd6990624, 0xf40e3585, 0x106aa070, 0x19a4c116,
            0x1e376c08, 0x2748774c, 0x34b0bcb5, 0x391c0cb3, 0x4ed8aa4a, 0x5b9cca4f, 0x682e6ff3,
            0x748f82ee, 0x78a5636f, 0x84c87814, 0x8cc70208, 0x90befffa, 0xa4506ceb, 0xbef9a3f7,
            0xc67178f2,
        ];
        let mut h: [u32; 8] = [
            0x6a09e667, 0xbb67ae85, 0x3c6ef372, 0xa54ff53a, 0x510e527f, 0x9b05688c, 0x1f83d9ab,
            0x5be0cd19,
        ];
        let mut message = data.to_vec();
        let bits = (data.len() as u64) * 8;
        message.push(0x80);
        while message.len() % 64 != 56 {
            message.push(0);
        }
        message.extend_from_slice(&bits.to_be_bytes());
        for block in message.chunks(64) {
            let mut w = [0u32; 64];
            for (i, word) in block.chunks(4).enumerate() {
                w[i] = u32::from_be_bytes([word[0], word[1], word[2], word[3]]);
            }
            for i in 16..64 {
                let s0 = w[i - 15].rotate_right(7) ^ w[i - 15].rotate_right(18) ^ (w[i - 15] >> 3);
                let s1 = w[i - 2].rotate_right(17) ^ w[i - 2].rotate_right(19) ^ (w[i - 2] >> 10);
                w[i] = w[i - 16]
                    .wrapping_add(s0)
                    .wrapping_add(w[i - 7])
                    .wrapping_add(s1);
            }
            let [mut a, mut b, mut c, mut d, mut e, mut f, mut g, mut hh] = h;
            for i in 0..64 {
                let s1 = e.rotate_right(6) ^ e.rotate_right(11) ^ e.rotate_right(25);
                let ch = (e & f) ^ ((!e) & g);
                let t1 = hh
                    .wrapping_add(s1)
                    .wrapping_add(ch)
                    .wrapping_add(K[i])
                    .wrapping_add(w[i]);
                let s0 = a.rotate_right(2) ^ a.rotate_right(13) ^ a.rotate_right(22);
                let maj = (a & b) ^ (a & c) ^ (b & c);
                let t2 = s0.wrapping_add(maj);
                hh = g;
                g = f;
                f = e;
                e = d.wrapping_add(t1);
                d = c;
                c = b;
                b = a;
                a = t1.wrapping_add(t2);
            }
            for (slot, value) in h.iter_mut().zip([a, b, c, d, e, f, g, hh]) {
                *slot = slot.wrapping_add(value);
            }
        }
        h.iter().map(|word| format!("{word:08x}")).collect()
    }

    /// No editor writes a uci list where firewall4 reads one value.
    ///
    /// This is the other half of the vocabulary's worth, and it cost a shipped
    /// bug to notice: `weekdays` is a scalar on both a rule and a redirect —
    /// fw4 splits the string itself — and the rule editor wrote it as a list, so
    /// `option 'weekdays' must not be a list` took the whole section out of the
    /// ruleset. A rule somebody wrote, saved and watched apply was never in the
    /// firewall, and nothing said so.
    ///
    /// The artifact knows each option's cardinality, so the check is simply to
    /// ask it. The reverse — a scalar where fw4 accepts a list — is fine and not
    /// checked: uci reads a single value into a one-entry list, which is what
    /// `parse_list` does with it.
    #[test]
    fn no_editor_writes_a_list_where_firewall4_reads_one_value() {
        let mut wrong: Vec<String> = Vec::new();
        for (kind, values) in written() {
            let Some(section) = FIREWALL.section(kind) else {
                panic!("nothing in this config is a {kind}");
            };
            let serde_json::Value::Object(values) = values else {
                panic!("{kind} did not write an object");
            };
            for (option, value) in values {
                if !value.is_array() {
                    continue;
                }
                let Some(spec) = section.options.iter().find(|spec| spec.key == option) else {
                    continue;
                };
                if spec.cardinality == Cardinality::Scalar {
                    wrong.push(format!(
                        "  {kind}.{option} is a {} and was written as a list of {}",
                        spec.datatype,
                        value.as_array().map(Vec::len).unwrap_or_default()
                    ));
                }
            }
        }
        assert!(
            wrong.is_empty(),
            "firewall4 refuses a list on these options and skips the whole section — \
             the object is saved and is not in the ruleset:\n{}",
            wrong.join("\n")
        );
    }

    /// written is what this plugin puts in the file, per section type it writes:
    /// every one of the five, so a list on the wrong option cannot hide in the
    /// two that have no list options at all.
    ///
    /// The three editors' forms come from the fixtures that carry the whole
    /// vocabulary, so this reads what they really write rather than a hand-made
    /// object that might miss the one option that matters. The other two write
    /// through a commit rather than a form, so their ops are read directly.
    fn written() -> Vec<(&'static str, serde_json::Value)> {
        let editor = fixture::editor_snapshot();
        let snapshot = fixture::snapshot();
        let rule = RuleForm::read(&editor.section("firewall", "everything").expect("the rule"));
        let redirect = RedirectForm::read(
            &editor
                .section("firewall", "everything_forward")
                .expect("the forward"),
        );
        let zone = ZoneForm::read(&snapshot.section("firewall", "cfg02dc81").expect("the zone"));
        let mut model = fixture::firewall();
        let (_, defaults) = settings::save(&mut model, &Form::parse("drop_invalid=1"));
        let crossings = crossings::Crossings {
            reaches: vec!["guest".to_string()],
        }
        .commits(&fixture::firewall(), "lan");
        let mut out = vec![
            ("rule", rule.values(true)),
            ("redirect", redirect.values(true)),
            ("zone", zone.values(true)),
        ];
        for (kind, ops) in [("defaults", defaults), ("forwarding", crossings)] {
            for op in ops {
                // A delete carries no values at all, which is nothing to check.
                if let Some(values) = serde_json::to_value(&op)
                    .expect("serialize the op")
                    .get("values")
                {
                    let kind = if kind == "defaults"
                        && model
                            .includes
                            .iter()
                            .any(|include| include.section == op.section)
                    {
                        "include"
                    } else {
                        kind
                    };
                    out.push((kind, values.clone()));
                }
            }
        }
        out
    }

    /// Every screen that edits this config shows what it will write, and shows it
    /// live.
    ///
    /// The block is a promise: it says "this is the config this form is about to
    /// put in the file", and a promise made against the values the page was built
    /// from goes stale the moment anybody types. Declaring it live is what makes
    /// it true — the shell asks this plugin to re-render from the values on
    /// screen. Exactly one per screen, because only the first can be kept current.
    #[test]
    fn every_editing_screen_previews_what_it_will_write_and_keeps_it_live() {
        for (screen, tree) in trees() {
            // A whole screen is an envelope. The crossings editor is in the list
            // as its controls alone — it is read there as the section type it
            // writes — and the preview it belongs to is the zone panel's, one
            // entry along.
            if tree.get("widget").is_none() {
                continue;
            }
            let mut live = 0;
            let mut stale = 0;
            count_previews(&tree, &mut live, &mut stale);
            assert_eq!(
                live, 1,
                "the {screen} screen declares {live} live previews; it needs exactly one"
            );
            assert_eq!(
                stale, 0,
                "the {screen} screen shows {stale} config block(s) that are not kept \
                 current — a preview of values nobody is typing into any more"
            );
        }
    }

    /// count_previews walks a tree for its config blocks, apart: the ones the
    /// shell keeps current, and the ones that would go stale under an edit.
    fn count_previews(node: &serde_json::Value, live: &mut usize, stale: &mut usize) {
        match node {
            serde_json::Value::Object(map) => {
                if map.get("type") == Some(&serde_json::Value::from("code")) {
                    match map.get("live") == Some(&serde_json::Value::Bool(true)) {
                        true => *live += 1,
                        false => *stale += 1,
                    }
                }
                for value in map.values() {
                    count_previews(value, live, stale);
                }
            }
            serde_json::Value::Array(items) => {
                for item in items {
                    count_previews(item, live, stale);
                }
            }
            _ => {}
        }
    }

    /// The options firewall4 acts on in the absence of any value are the ones worth
    /// naming out loud: each is something the config does that no screen had to
    /// mention. This states the list, so a firmware that adds one shows up here
    /// rather than in a support question.
    #[test]
    fn firewall4s_standing_assumptions_are_stated() {
        let defaults = FIREWALL.options_with_defaults();
        assert!(
            defaults.len() > 10,
            "firewall4 assumes more than this: {defaults:?}"
        );
        // The two that caught us: a forward reflects by default, and the custom
        // includes load by default.
        assert!(defaults.contains(&("redirect", "reflection", "1")));
        assert!(defaults.contains(&("defaults", "auto_includes", "1")));
    }
}
