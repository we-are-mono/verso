// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

//! What a config can hold, as the daemon that reads it defines.
//!
//! A plugin edits a config file some daemon owns, and that daemon — not the
//! plugin — decides which options exist, what each one takes, and which of them
//! it quietly ignores. A plugin that keeps its own idea of that list is keeping a
//! copy, and a copy drifts: it is how a firewall rule came to be written with a
//! default nobody could see, and how thirteen options firewall4 supports became
//! unreachable without anyone noticing.
//!
//! So the list is an artifact here rather than knowledge spread through the forms.
//! Every option the daemon accepts is named once, with what it takes; every one of
//! them is then in exactly one of three states, and a test refuses a fourth:
//!
//!   - the UI renders it,
//!   - the UI deliberately leaves it out, and says why,
//!   - the daemon itself does not support it.
//!
//! "Nobody thought about it" is not one of the three. That is the whole point.
//!
//! # Where the list comes from
//!
//! [`Provenance`] says, in the artifact, how its contents were obtained — because
//! a hand-written list and a list read out of the daemon's own source are worth
//! different amounts, and nothing should be able to mistake one for the other.
//! A daemon that declares its options can be extracted from; one that only
//! validates can be asked; one that does neither leaves the plugin's own table as
//! the whole of the truth, which is fine as long as it says so.

use std::collections::BTreeSet;

/// Vocabulary is one config file's whole option set, as its daemon defines it.
pub struct Vocabulary {
    /// The config this describes, as uci names it: "firewall", "dhcp", "network".
    pub config: &'static str,
    /// How this list was obtained, and from what.
    pub provenance: Provenance,
    pub sections: &'static [SectionVocabulary],
}

/// Provenance is where a vocabulary's contents came from. It is recorded in the
/// artifact so a reader can tell a checked list from an asserted one.
pub enum Provenance {
    /// Read out of the daemon's own source by a tool, from a file whose hash is
    /// recorded: the strongest kind, because a firmware that moves under us stops
    /// matching and says so.
    Extracted {
        /// The upstream package the options were read from.
        upstream: &'static str,
        /// Its version, as the package declares it.
        version: &'static str,
        /// The SHA-256 of the file the options were read from, so the artifact and
        /// the firmware it describes cannot silently come apart.
        sha256: &'static str,
        /// The tool that read it, and its version — a changed extractor is as
        /// capable of changing this list as a changed daemon.
        extractor: &'static str,
    },
    /// Written by hand, because the daemon publishes no list. Honest, and weaker:
    /// the reason belongs here so nobody reads it as verified.
    Declared { because: &'static str },
}

/// SectionVocabulary is one uci section type and every option it accepts.
pub struct SectionVocabulary {
    /// The section type, as uci writes it: "rule", "zone", "defaults".
    pub kind: &'static str,
    pub options: &'static [OptionSpec],
}

/// OptionSpec is one option, as the daemon parses it.
pub struct OptionSpec {
    /// The option name, verbatim — what appears in the config file.
    pub key: &'static str,
    /// The daemon's own name for what this takes ("bool", "network", "limit").
    /// It is the daemon's vocabulary, not a widget: what to render for it is the
    /// plugin's judgement, and two options of one datatype may read differently.
    pub datatype: &'static str,
    /// What the daemon assumes when the option is absent. A default that is not
    /// `None` and not "0" is the dangerous kind: the config does something the
    /// screen never mentioned unless the UI says so.
    pub default: Option<&'static str>,
    pub cardinality: Cardinality,
    /// Whether the daemon accepts a leading "!" on the value.
    pub invertible: bool,
    pub support: Support,
}

/// Cardinality is whether one option holds one value or a list of them.
#[derive(PartialEq, Eq)]
pub enum Cardinality {
    Scalar,
    List,
}

/// Support is what the daemon does with an option it parses.
#[derive(PartialEq, Eq)]
pub enum Support {
    /// Parsed and acted on.
    Supported,
    /// Parsed and ignored. A UI that offers one of these offers a control that
    /// changes nothing, which is worse than a control that is missing.
    Unsupported,
    /// Parsed, acted on, and on its way out. Not worth a control either.
    Deprecated,
}

impl Vocabulary {
    /// section returns one section type's options.
    pub fn section(&self, kind: &str) -> Option<&SectionVocabulary> {
        self.sections.iter().find(|s| s.kind == kind)
    }

    /// options_with_defaults lists every supported option the daemon acts on in
    /// the absence of any value. These are the ones worth auditing hardest: a
    /// default-on option the UI does not render is a config doing something the
    /// screen does not say.
    pub fn options_with_defaults(&self) -> Vec<(&'static str, &'static str, &'static str)> {
        let mut out = Vec::new();
        for section in self.sections {
            for option in section.options {
                if option.support != Support::Supported {
                    continue;
                }
                match option.default {
                    Some(value) if value != "0" => out.push((section.kind, option.key, value)),
                    _ => {}
                }
            }
        }
        out
    }
}

/// Omission is an option the UI leaves out on purpose, and the reason.
///
/// The reason is the whole value of the type. An option absent from a form is
/// indistinguishable from an option nobody considered; an option listed here is a
/// decision somebody made and signed, and the next person can disagree with it
/// knowing it was one.
pub struct Omission {
    pub section: &'static str,
    /// The option left out, or [`Omission::EVERY`] for a section type this UI does
    /// not edit at all. A whole section is a real answer — a config's sections are
    /// not all one screen's business — and it is one decision with one reason, so
    /// spelling it per-option would bury the reason under ninety copies of itself.
    pub key: &'static str,
    pub because: &'static str,
}

impl Omission {
    /// EVERY stands for every option in a section type, for a section this UI does
    /// not edit.
    pub const EVERY: &'static str = "*";

    /// covers says whether this omission answers for one option.
    fn covers(&self, section: &str, key: &str) -> bool {
        self.section == section && (self.key == key || self.key == Omission::EVERY)
    }
}

/// Composition is one option several form fields write between them.
///
/// Some options hold a small syntax of their own — a mark is `!value/mask`, a set
/// match names the set and the packet fields it applies to — and a form that asked
/// for the whole string would be asking a person to write the daemon's grammar.
/// Such an option is rendered; it is just not rendered under its own name, and
/// without saying so it reads as missing.
///
/// The fields are checked: a composition whose fields are nowhere on screen is a
/// claim that has stopped being true, and is reported like a stale omission.
pub struct Composition {
    pub section: &'static str,
    pub key: &'static str,
    /// The form fields that together write it.
    pub fields: &'static [&'static str],
    pub because: &'static str,
}

impl Composition {
    /// shown says whether every field this composition rests on is actually on
    /// screen. One missing field and the claim is no longer true.
    fn shown(&self, rendered: &BTreeSet<(&str, &str)>) -> bool {
        self.fields
            .iter()
            .all(|field| rendered.contains(&(self.section, *field)))
    }
}

/// Unaccounted is one option in none of the three states: the daemon supports it,
/// the UI does not render it, and nobody wrote down why.
#[derive(Debug, PartialEq, Eq, PartialOrd, Ord)]
pub struct Unaccounted {
    pub section: String,
    pub key: String,
    pub datatype: String,
    /// What the daemon does in its absence, when that is something.
    pub default: Option<String>,
}

/// reckon is the guard. Given what the daemon accepts, what the UI actually
/// renders, and what the UI says it leaves out on purpose, it returns what is in
/// none of those states.
///
/// `rendered` is the set of option names the UI writes — collected from the widget
/// trees the plugin really builds, never from a second hand-kept list, because a
/// list of "options we render" is exactly the copy this whole file exists to
/// abolish.
pub fn reckon(
    vocabulary: &Vocabulary,
    rendered: &BTreeSet<(&str, &str)>,
    omissions: &[Omission],
    compositions: &[Composition],
) -> Vec<Unaccounted> {
    let mut out = Vec::new();
    for section in vocabulary.sections {
        for option in section.options {
            if option.support != Support::Supported {
                continue;
            }
            if rendered.contains(&(section.kind, option.key)) {
                continue;
            }
            if compositions
                .iter()
                .any(|c| c.section == section.kind && c.key == option.key && c.shown(rendered))
            {
                continue;
            }
            if omissions.iter().any(|o| o.covers(section.kind, option.key)) {
                continue;
            }
            out.push(Unaccounted {
                section: section.kind.to_string(),
                key: option.key.to_string(),
                datatype: option.datatype.to_string(),
                default: option.default.map(str::to_string),
            });
        }
    }
    out.sort();
    out
}

/// stale_omissions is the guard's other half: an omission naming an option the
/// daemon no longer has, or one the UI has since grown a control for. Both are
/// notes that have stopped being true, and a table of those rots until nobody
/// trusts any of it.
pub fn stale_omissions<'a>(
    vocabulary: &Vocabulary,
    rendered: &BTreeSet<(&str, &str)>,
    omissions: &'a [Omission],
) -> Vec<(&'a Omission, &'static str)> {
    let mut out: Vec<(&'a Omission, &'static str)> = Vec::new();
    for omission in omissions {
        let Some(section) = vocabulary.section(omission.section) else {
            out.push((omission, "names a section type this config does not have"));
            continue;
        };
        if omission.key == Omission::EVERY {
            // A whole section claimed as out of scope is stale once anything in it
            // is on screen: either the claim is wrong or the control is.
            if section
                .options
                .iter()
                .any(|o| rendered.contains(&(omission.section, o.key)))
            {
                out.push((
                    omission,
                    "claims a whole section nothing edits, but something does",
                ));
            }
            continue;
        }
        if !section.options.iter().any(|o| o.key == omission.key) {
            out.push((omission, "names an option the daemon no longer parses"));
        } else if rendered.contains(&(omission.section, omission.key)) {
            out.push((omission, "is rendered after all"));
        }
    }
    out
}

/// stale_compositions is the same guard for the other table: a composition naming
/// an option the daemon dropped, or resting on a field that is no longer on screen.
/// The second is the dangerous one — it silently turns "rendered, under other
/// names" back into "not rendered", and without this check the claim would keep
/// vouching for an option nothing writes.
pub fn stale_compositions<'a>(
    vocabulary: &Vocabulary,
    rendered: &BTreeSet<(&str, &str)>,
    compositions: &'a [Composition],
) -> Vec<(&'a Composition, String)> {
    let mut out = Vec::new();
    for composition in compositions {
        let Some(section) = vocabulary.section(composition.section) else {
            out.push((
                composition,
                "names a section type this config does not have".to_string(),
            ));
            continue;
        };
        if !section.options.iter().any(|o| o.key == composition.key) {
            out.push((
                composition,
                "names an option the daemon no longer parses".to_string(),
            ));
            continue;
        }
        let missing: Vec<&str> = composition
            .fields
            .iter()
            .copied()
            .filter(|field| !rendered.contains(&(composition.section, *field)))
            .collect();
        if !missing.is_empty() {
            out.push((
                composition,
                format!("rests on field(s) nothing renders: {}", missing.join(", ")),
            ));
        }
    }
    out
}
