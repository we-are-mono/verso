// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

//! One zone as its editor holds it, between the config and the form.
//!
//! The editor owns a fixed set of firewall4's zone options — [`OWNED`] — and
//! nothing else. That set is the contract in both directions: reading a section
//! fills this struct from those options, saving writes them back, and every one
//! of them the submission no longer carries is cleared rather than left behind.
//!
//! The set used to be eight options, and the raw devices a zone claimed, its
//! masquerade filters and its connection helper were among the things it did not
//! draw — carried through a save untouched, which kept them safe and kept them
//! unreachable. They are drawn now. What firewall4 accepts on a zone and what this
//! editor offers are the same list, and the reckoning beside it fails if they ever
//! come apart again.
//!
//! The `name` option is outside that set. A zone's name is the handle every rule,
//! forward and forwarding points at, so it is written only when it changes — when
//! the zone is created, or renamed — and a rename carries every one of those
//! references with it ([`crate::rename`]).
//!
//! A policy is the other deliberate absence. firewall4 falls a zone's input,
//! output and forward back to the `config defaults` section, so a zone that
//! states none is a zone that follows the baseline — and writing the baseline's
//! current value onto it would silently sever that. The empty choice is
//! therefore a real choice, and it clears the option.
//!
//! Two of firewall4's own readings are the reason this file exists rather than
//! a field-per-option mapping. A **policy** is matched by case-insensitive
//! prefix, so `acc` is accept and an editor that compared whole words would
//! offer to clear a verdict the zone is enforcing. And **`log`** is a bitfield,
//! not a truth value: bit 1 logs refused traffic, bit 2 logs MSS clamps. One
//! switch owns the first bit; the second has no control here and rides through
//! a save untouched.
//!
//! That bitfield has a third state worth naming. fw4 parses it with ucode's
//! `+val` and skips the entire section when the answer is not a number — so
//! `log 'on'`, which uci reads as true everywhere else, is a zone that is not in
//! the packet filter at all. The switch reads off for it, because such a zone
//! logs nothing, and the value itself rides through a save untouched under the
//! same contract as every other option this editor does not draw.

use verso_plugin::{json, Form, Map, Section, SelectOption, Value};

use crate::model;
use crate::rule_form::{cleaned, family_value, valid_address, valid_limit, Errors, HELPERS};

/// SUBNET_HELP is what a zone's own address has to look like, and it is also the
/// error: an address firewall4 cannot parse makes it skip the whole zone, and every
/// rule naming that zone with it.
pub const SUBNET_HELP: &str = "An address or a network in CIDR form, such as 10.0.20.0/24.";

pub const LOG_LIMIT_HELP: &str = "How often a line may be written, such as 10/minute.";

/// OWNED is every option this editor writes. A save states all of them: the ones
/// the submission carries as values, the rest as nulls that clear them.
pub const OWNED: [&str; 18] = [
    "network",
    "device",
    "subnet",
    "input",
    "output",
    "forward",
    "masq",
    "masq_src",
    "masq_dest",
    "masq_allow_invalid",
    "mtu_fix",
    "log",
    "log_limit",
    "family",
    "enabled",
    "counter",
    "auto_helper",
    "helper",
];

/// POLICIES are the verdicts a zone's traffic can take. The empty value is not a
/// missing choice: it is the zone stating no policy of its own and following the
/// global defaults, which is how most zones on a stock install are written.
pub const POLICIES: [(&str, &str); 4] = [
    ("", "Use the global default"),
    ("ACCEPT", "Accept"),
    ("REJECT", "Reject"),
    ("DROP", "Drop"),
];

/// FAMILIES are the address families a zone may narrow itself to, spelled the
/// way the rule editor spells them so one vocabulary covers the whole plugin.
pub const FAMILIES: [(&str, &str); 3] = [
    ("", "IPv4 and IPv6"),
    ("ipv4", "IPv4 only"),
    ("ipv6", "IPv6 only"),
];

/// NAME_HELP states what a zone name may be — firewall4's own identifier rule,
/// in the words an operator can act on: a leading letter, dot or underscore,
/// then letters, digits, dots, dashes, slashes and underscores.
pub const NAME_HELP: &str = "Start with a letter, a dot or an underscore, then use letters, \
digits, dots, dashes, slashes and underscores.";

/// NAME_LIMIT is how long a zone name may be. firewall4 accepts any length, but
/// it builds every one of its chain names out of this one — `accept_from_<name>`
/// is the longest — and nftables refuses a chain name past 255 characters, which
/// is a firewall that stops reloading. The cap is well inside that, and
/// NAME_TOO_LONG states it in the same number.
pub const NAME_LIMIT: usize = 200;

const NAME_TOO_LONG: &str =
    "Use at most 200 characters: the firewall builds longer names of its own out of this one.";

/// ZoneForm is one zone as the editor holds it: every option it owns, in the
/// shape its controls take rather than the shape uci writes.
#[derive(Clone)]
pub struct ZoneForm {
    pub name: String,
    /// The name the section carries, which is what every reference to the zone
    /// points at. A blank zone has none; a name that differs from it is a rename.
    pub named: String,
    pub networks: Vec<String>,
    /// A zone covers what its networks cover, and may also claim a kernel device
    /// directly or a subnet of its own. Both are how a zone covers something the
    /// network config does not name as an interface.
    pub devices: Vec<String>,
    pub subnets: Vec<String>,
    pub input: String,
    pub output: String,
    pub forward: String,
    pub masq: bool,
    /// Masquerading rewrites the source of what leaves, and these three narrow it:
    /// which sources it applies to, which destinations, and whether a packet
    /// conntrack cannot place is rewritten too.
    pub masq_src: Vec<String>,
    pub masq_dest: Vec<String>,
    pub masq_allow_invalid: bool,
    pub mtu_fix: bool,
    /// firewall4 keeps these on unless a zone says otherwise: the zone is in force,
    /// its chains count what they match, and connection helpers are assigned
    /// automatically. Each off has to be written, because absence means on.
    pub enabled: bool,
    pub counter: bool,
    pub auto_helper: bool,
    /// One named connection helper for this zone, where the automatic assignment is
    /// not what is wanted.
    pub helper: String,
    /// How often the logging below may write a line. A zone that logs every refusal
    /// on a busy uplink fills a flash chip.
    pub log_limit: String,
    /// Bit 1 of firewall4's `log`: a line for every packet this zone refuses.
    pub log: bool,
    /// Bit 2 of the same option: a line for every MSS clamp. The editor draws no
    /// control for it — clamping is already its own switch — so it is carried
    /// from the section rather than typed, and written back untouched.
    pub log_mss: bool,
    /// The `log` option verbatim where firewall4 cannot read it as a number.
    /// Such a zone is skipped by fw4 outright, so neither bit is set and nothing
    /// here claims it logs; the value is carried so a save that never touched
    /// the switch writes back exactly what an expert wrote.
    pub log_unreadable: String,
    pub family: String,
}

impl Default for ZoneForm {
    /// A blank zone starts where firewall4's own defaults are. Three of them are on,
    /// and deriving this would have started them off — so the first zone created
    /// through the panel would have been written disabled, uncounted, and with
    /// helper assignment turned off, none of which anybody asked for.
    fn default() -> ZoneForm {
        ZoneForm {
            name: String::new(),
            named: String::new(),
            networks: Vec::new(),
            devices: Vec::new(),
            subnets: Vec::new(),
            input: String::new(),
            output: String::new(),
            forward: String::new(),
            masq: false,
            masq_src: Vec::new(),
            masq_dest: Vec::new(),
            masq_allow_invalid: false,
            mtu_fix: false,
            enabled: true,
            counter: true,
            auto_helper: true,
            helper: String::new(),
            log_limit: String::new(),
            log: false,
            log_mss: false,
            log_unreadable: String::new(),
            family: String::new(),
        }
    }
}

impl ZoneForm {
    /// read fills the form from a zone section. Unlike the listing, it reads
    /// what the section states and never what the section inherits: a policy the
    /// zone does not carry has to come back as the empty choice, or saving would
    /// write the baseline onto the zone and cut it loose from the baseline.
    pub fn read(section: &Section) -> ZoneForm {
        let log = log_bits(&section.scalar("log"));
        ZoneForm {
            name: section.scalar("name"),
            named: section.scalar("name"),
            networks: model::values(section, "network"),
            devices: model::values(section, "device"),
            subnets: model::values(section, "subnet"),
            input: policy_value(&section.scalar("input")),
            output: policy_value(&section.scalar("output")),
            forward: policy_value(&section.scalar("forward")),
            masq: model::flag(section, "masq", false),
            masq_src: model::values(section, "masq_src"),
            masq_dest: model::values(section, "masq_dest"),
            masq_allow_invalid: model::flag(section, "masq_allow_invalid", false),
            mtu_fix: model::flag(section, "mtu_fix", false),
            // firewall4's own defaults, stated rather than assumed.
            enabled: model::flag(section, "enabled", true),
            counter: model::flag(section, "counter", true),
            auto_helper: model::flag(section, "auto_helper", true),
            helper: section.scalar("helper"),
            log_limit: section.scalar("log_limit"),
            log: log.refused,
            log_mss: log.mss,
            log_unreadable: log.unreadable,
            family: family_value(&section.scalar("family")),
        }
    }

    /// submitted reads the form the browser posted. A switch posts only while it
    /// is on, and a checks field posts only its checked members, so an absent
    /// field is the operator saying that option is gone.
    pub fn submitted(form: &Form) -> ZoneForm {
        ZoneForm {
            name: form.get("name").trim().to_string(),
            named: String::new(),
            networks: cleaned(form.all("network")),
            devices: cleaned(form.all("device")),
            subnets: cleaned(form.all("subnet")),
            input: form.get("input").trim().to_uppercase(),
            output: form.get("output").trim().to_uppercase(),
            forward: form.get("forward").trim().to_uppercase(),
            masq: !form.get("masq").is_empty(),
            masq_src: cleaned(form.all("masq_src")),
            masq_dest: cleaned(form.all("masq_dest")),
            masq_allow_invalid: !form.get("masq_allow_invalid").is_empty(),
            mtu_fix: !form.get("mtu_fix").is_empty(),
            enabled: !form.get("enabled").is_empty(),
            counter: !form.get("counter").is_empty(),
            auto_helper: !form.get("auto_helper").is_empty(),
            helper: form.get("helper").trim().to_string(),
            log_limit: form.get("log_limit").trim().to_string(),
            log: !form.get("log").is_empty(),
            log_mss: false,
            log_unreadable: String::new(),
            family: family_value(&form.get("family")),
        }
    }

    /// carrying takes back the parts of a zone the form does not draw: the name
    /// the section carries, which is what tells a rename from a save, and the two
    /// halves of the `log` option the switch does not own — the MSS-clamp bit,
    /// which a submission that dropped it would turn off on a zone nobody asked
    /// about, and a value firewall4 cannot read at all, which is not this editor's
    /// to discard.
    pub fn carrying(mut self, stated: &ZoneForm) -> ZoneForm {
        self.named = stated.named.clone();
        self.log_mss = stated.log_mss;
        self.log_unreadable = stated.log_unreadable.clone();
        self
    }

    /// renamed reports whether the name typed is not the one the section
    /// carries — a zone being created is not a rename, it has nothing to follow.
    pub fn renamed(&self) -> bool {
        !self.named.is_empty() && self.name != self.named
    }

    /// values is what the save writes: every option the zone states, plus — when
    /// editing an existing section — a null for each owned option it no longer
    /// states. The name is written when it is not the one the section carries:
    /// a new zone's, or a renamed one's.
    pub fn values(&self, existing: bool) -> Value {
        let mut values = Map::new();
        let mut set = |option: &str, value: Value| {
            values.insert(option.to_string(), value);
        };

        if !self.name.is_empty() && self.name != self.named {
            set("name", json!(self.name.clone()));
        }
        for (option, values_of) in [
            ("network", &self.networks),
            ("device", &self.devices),
            ("subnet", &self.subnets),
            ("masq_src", &self.masq_src),
            ("masq_dest", &self.masq_dest),
        ] {
            if !values_of.is_empty() {
                set(option, json!(values_of.clone()));
            }
        }
        for (option, value) in [
            ("input", &self.input),
            ("output", &self.output),
            ("forward", &self.forward),
            ("family", &self.family),
            ("helper", &self.helper),
            ("log_limit", &self.log_limit),
        ] {
            if !value.is_empty() {
                set(option, json!(value.clone()));
            }
        }
        // The three firewall4 keeps on unless told otherwise. Their off is what has
        // to be written: clearing the option would restore the behaviour the
        // operator just turned off, which is the mistake a default-on option invites
        // and the reason each of these now has a control at all.
        for (option, on) in [
            ("enabled", self.enabled),
            ("counter", self.counter),
            ("auto_helper", self.auto_helper),
        ] {
            if !on {
                set(option, json!("0"));
            }
        }
        // These two are off unless the section says otherwise, so turning one
        // off is clearing the option rather than writing a zero — which is what
        // the config would look like if it had never been on.
        for (option, on) in [
            ("masq", self.masq),
            ("mtu_fix", self.mtu_fix),
            ("masq_allow_invalid", self.masq_allow_invalid),
        ] {
            if on {
                set(option, json!("1"));
            }
        }
        // `log` is not a truth value: firewall4 reads it as a bitfield, where 1
        // logs refused traffic and 2 logs MSS clamps. The switch owns the first
        // bit; the second is carried from the section, so a zone that logs its
        // clamps still does after a save that never mentioned them.
        //
        // A value fw4 cannot read as a number at all is a third case. The switch
        // rendered off for it (nothing is being logged — fw4 skipped the whole
        // zone), so a save that leaves it off is a save that did not touch this
        // option, and the value goes back exactly as it was found. Turning the
        // switch on is the operator stating what they want instead, and that
        // replaces it.
        if !self.log_unreadable.is_empty() && !self.log {
            set("log", json!(self.log_unreadable.clone()));
        } else if let Some(bits) = log_option(self.log, self.log_mss) {
            set("log", json!(bits));
        }

        if existing {
            for option in OWNED {
                values.entry(option.to_string()).or_insert(Value::Null);
            }
        } else {
            values.retain(|_, value| !value.is_null());
        }
        Value::Object(values)
    }

    /// validate answers firewall4's question — would the packet filter accept
    /// this zone — before the write, because a zone fw4 refuses is skipped with
    /// nothing said to the operator, taking every rule that names it with it.
    /// `networks` is the set a zone may cover: what the network config defines,
    /// plus whatever this zone already covers.
    pub fn validate(&self, networks: &[String]) -> Errors {
        let mut errors = Errors::default();
        for (field, value) in [
            ("input", &self.input),
            ("output", &self.output),
            ("forward", &self.forward),
        ] {
            if !POLICIES.iter().any(|(policy, _)| policy == value) {
                errors.field(field, "Choose one of the offered policies.");
            }
        }
        if self
            .networks
            .iter()
            .any(|network| !networks.iter().any(|known| known == network))
        {
            errors.field("network", "Choose from the networks this router defines.");
        }
        // A subnet, and a masquerade narrowed to one, are addresses in CIDR form.
        // firewall4 skips a whole zone whose address it cannot parse, taking every
        // rule that names the zone with it, so a value it would refuse is stopped
        // here instead.
        for (field, values) in [
            ("subnet", &self.subnets),
            ("masq_src", &self.masq_src),
            ("masq_dest", &self.masq_dest),
        ] {
            if values.iter().any(|value| !valid_address(value)) {
                errors.field(field, SUBNET_HELP);
            }
        }
        if !self.log_limit.is_empty() && !valid_limit(&self.log_limit) {
            errors.field("log_limit", LOG_LIMIT_HELP);
        }
        if !self.helper.is_empty() && !HELPERS.contains(&self.helper.as_str()) {
            errors.field("helper", "Choose one of the helpers this firewall ships.");
        }
        errors
    }

    /// validate_named adds what a name being written has to answer for, on a zone
    /// being created or renamed: it needs one, firewall4 has to be able to parse
    /// it, and it has to be free — two zones with one name are two zones every
    /// reference is ambiguous between. A zone keeping its name asks none of it.
    pub fn validate_named(&self, networks: &[String], taken: &[String]) -> Errors {
        let mut errors = self.validate(networks);
        if !self.name.is_empty() && self.name == self.named {
            return errors;
        }
        if self.name.is_empty() {
            errors.field("name", "Give the zone a name.");
        } else if !valid_identifier(&self.name) {
            errors.field("name", NAME_HELP);
        } else if self.name.chars().count() > NAME_LIMIT {
            errors.field("name", NAME_TOO_LONG);
        } else if taken.iter().any(|name| name == &self.name) {
            errors.field("name", "A zone with this name already exists.");
        }
        errors
    }
}

/// options builds a select's option set from value/label pairs.
pub fn options(pairs: &[(&str, &str)]) -> Vec<SelectOption> {
    pairs
        .iter()
        .map(|(value, label)| SelectOption::new(value, label))
        .collect()
}

/// policy_value normalizes a written policy onto the choices the editor offers,
/// reading it the way firewall4 does: a case-insensitive prefix of one of the
/// verdicts, so `acc`, `A` and `rej` are all policies the packet filter acts on.
/// A value fw4 would not read as a policy is not one this editor can show, so it
/// reads as the empty choice and the next save clears it.
///
/// The empty option is the one deliberate departure from firewall4. fw4's
/// prefix matcher answers `''` with the *first* choice — every choice starts
/// with the empty string, so `input ''` is enforced as accept — which is an
/// accident of the matcher rather than anything the option means. A zone stating
/// no policy is a zone following the baseline, so that is what the editor shows,
/// and clearing the option (which is what the empty choice saves) is the state
/// fw4 reads that way for certain. A hand-written `input ''` therefore reads
/// here as "no policy stated" while fw4 is enforcing accept, and the first save
/// resolves the disagreement in favour of the baseline.
fn policy_value(written: &str) -> String {
    let written = written.trim().to_lowercase();
    if written.is_empty() {
        return String::new();
    }
    POLICIES
        .iter()
        .map(|(policy, _)| *policy)
        .filter(|policy| !policy.is_empty())
        .find(|policy| policy.to_lowercase().starts_with(&written))
        .unwrap_or_default()
        .to_string()
}

/// LogBits is a zone's `log` option read the way firewall4 reads it.
#[derive(Clone, Debug, Default, PartialEq)]
pub struct LogBits {
    /// Bit 1: a line for every packet the zone refuses.
    pub refused: bool,
    /// Bit 2: a line for every MSS clamp.
    pub mss: bool,
    /// The value verbatim where fw4 cannot read it as a number at all. Both bits
    /// are then false — because such a zone is not logging, it is not in the
    /// packet filter — and this is what a save writes back untouched.
    pub unreadable: String,
}

/// log_bits reads firewall4's `log` bitfield: bit 1 logs refused traffic, bit 2
/// logs MSS clamps.
///
/// The option is declared `int`, and fw4's `parse_int` is ucode's `+val`: it
/// answers null for anything that is not a number, and `parse_options` turns
/// that null into "skip this section". uci's written truth values are *not* an
/// exception — `log 'on'` is NaN, and a zone carrying it is dropped from the
/// ruleset entirely, taking every rule that names the zone with it. So the one
/// thing this reading must never do is report such a zone as logging.
pub fn log_bits(written: &str) -> LogBits {
    let Some(number) = ucode_number(written) else {
        return LogBits {
            unreadable: written.to_string(),
            ..LogBits::default()
        };
    };
    // ucode's `&` truncates toward zero and reads the two's-complement pattern,
    // so a fraction loses its fraction and a negative value carries every bit.
    let bits = number as i64;
    LogBits {
        refused: bits & 1 != 0,
        mss: bits & 2 != 0,
        unreadable: String::new(),
    }
}

/// ucode_number is ucode's `+val`, which firewall4's `parse_int` is a thin skin
/// over. It is not Rust's number parser: `0x`, `0b` and `0o` are bases, `017` is
/// seventeen rather than octal, the empty string is zero, and a leading `.5`,
/// the words `inf`/`nan`, and a digit separator are all NaN. None here is that
/// NaN — the answer that makes fw4 skip the section.
fn ucode_number(written: &str) -> Option<f64> {
    let text = written.trim();
    if text.is_empty() {
        return Some(0.0);
    }
    let (sign, magnitude) = match text.strip_prefix('-') {
        Some(rest) => (-1.0, rest),
        None => (1.0, text.strip_prefix('+').unwrap_or(text)),
    };
    for (prefix, radix) in [
        ("0x", 16),
        ("0X", 16),
        ("0b", 2),
        ("0B", 2),
        ("0o", 8),
        ("0O", 8),
    ] {
        let Some(digits) = magnitude.strip_prefix(prefix) else {
            continue;
        };
        if digits.is_empty() || !digits.chars().all(|c| c.is_digit(radix)) {
            return None;
        }
        // A magnitude past what the width holds saturates, exactly as the cast
        // ucode performs does; the low bits this reading wants are all ones.
        return Some(sign * u64::from_str_radix(digits, radix).map_or(f64::INFINITY, |v| v as f64));
    }
    // A decimal has to start with a digit. Rust's own parser would take ".5",
    // "inf" and "nan"; ucode takes none of them.
    if !magnitude.starts_with(|c: char| c.is_ascii_digit()) {
        return None;
    }
    magnitude.parse::<f64>().ok().map(|value| sign * value)
}

/// log_option is the value those two bits are written back as, or None when
/// neither is set — an option nobody asked for is cleared, not written zero.
fn log_option(refused: bool, mss: bool) -> Option<&'static str> {
    match (refused, mss) {
        (false, false) => None,
        (true, false) => Some("1"),
        (false, true) => Some("2"),
        (true, true) => Some("3"),
    }
}

/// valid_identifier is firewall4's own `identifier` parser: a name it cannot
/// match is a zone it skips entirely.
pub fn valid_identifier(name: &str) -> bool {
    let mut chars = name.chars();
    let leading = |c: char| c.is_ascii_alphabetic() || c == '_' || c == '.';
    let trailing = |c: char| c.is_ascii_alphanumeric() || "/_.-".contains(c);
    chars.next().is_some_and(leading) && chars.all(trailing)
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::fixture;
    use crate::model::CONFIG;
    use verso_plugin::Snapshot;

    fn read(section: &str) -> ZoneForm {
        let snapshot = fixture::snapshot();
        let uci = snapshot.section(CONFIG, section).expect("the zone section");
        ZoneForm::read(&uci)
    }

    /// stated reads a zone written exactly as these options — how a test puts one
    /// option's odd spelling in front of the editor without adding a zone to the
    /// fixture every listing renders.
    fn stated(options: Value) -> ZoneForm {
        let snapshot = Snapshot::from_value(json!({ CONFIG: { "z": options } }));
        ZoneForm::read(&snapshot.section(CONFIG, "z").expect("the zone section"))
    }

    fn networks() -> Vec<String> {
        fixture::firewall().network_names()
    }

    #[test]
    fn a_zone_survives_the_round_trip_through_the_editor() {
        let wan = read("cfg03dc81");
        assert_eq!(wan.name, "wan");
        assert_eq!(wan.networks, vec!["wan", "wan6"]);
        assert_eq!(wan.input, "REJECT");
        assert_eq!(wan.output, "ACCEPT");
        assert_eq!(wan.forward, "REJECT");
        assert!(wan.masq);
        assert!(wan.mtu_fix);
        assert!(!wan.log);
        assert!(wan.family.is_empty());

        let written = wan.values(true);
        assert_eq!(
            written,
            json!({
                "network": ["wan", "wan6"],
                "input": "REJECT",
                "output": "ACCEPT",
                "forward": "REJECT",
                "masq": "1",
                "mtu_fix": "1",
                "log": null,
                "family": null,
                // Every other option this editor now owns, cleared because the wan
                // zone states none of them. The three firewall4 keeps on are cleared
                // rather than written "1": absence is already on, and writing the
                // default back would stage a change nobody made.
                "device": null,
                "subnet": null,
                "masq_src": null,
                "masq_dest": null,
                "masq_allow_invalid": null,
                "log_limit": null,
                "enabled": null,
                "counter": null,
                "auto_helper": null,
                "helper": null
            })
        );
        assert!(wan.validate(&networks()).is_empty());
        // The name is the one the section carries, so there is nothing to write.
        assert!(written.get("name").is_none());
    }

    /// A stock guest zone states only its output policy; the other two follow
    /// the defaults section. The editor has to come back with that — an inherited
    /// policy read as a stated one would be written onto the zone on the next
    /// save, quietly cutting it loose from the baseline it was following.
    #[test]
    fn an_inherited_policy_reads_as_the_empty_choice_and_stays_that_way() {
        let guest = read("guest_zone");
        assert_eq!(guest.input, "");
        assert_eq!(guest.forward, "");
        assert_eq!(guest.output, "ACCEPT");

        let written = guest.values(true);
        assert_eq!(written["input"], Value::Null);
        assert_eq!(written["forward"], Value::Null);
        assert_eq!(written["output"], "ACCEPT");
        assert!(guest.validate(&networks()).is_empty());
    }

    #[test]
    fn a_new_zone_writes_its_name_and_nothing_it_has_nothing_to_clear() {
        let zone = ZoneForm {
            name: "iot".into(),
            networks: vec!["guest".into()],
            input: "REJECT".into(),
            forward: "REJECT".into(),
            ..ZoneForm::default()
        };
        let written = zone.values(false);
        assert_eq!(
            written,
            json!({
                "name": "iot",
                "network": ["guest"],
                "input": "REJECT",
                "forward": "REJECT"
            })
        );
        let object = written.as_object().expect("values");
        assert!(object.values().all(|value| !value.is_null()), "{written}");
    }

    #[test]
    fn a_switch_the_submission_left_out_clears_the_option_it_writes() {
        let off = ZoneForm::submitted(&Form::parse("input=ACCEPT"));
        assert!(!off.masq);
        assert!(!off.mtu_fix);
        assert!(!off.log);
        let written = off.values(true);
        for option in ["masq", "mtu_fix", "log"] {
            assert_eq!(written[option], Value::Null, "{option}");
        }

        let on = ZoneForm::submitted(&Form::parse("masq=on&mtu_fix=on&log=on"));
        let written = on.values(true);
        for option in ["masq", "mtu_fix", "log"] {
            assert_eq!(written[option], "1", "{option}");
        }
    }

    #[test]
    fn a_checks_field_posts_the_networks_it_checked_and_nothing_else() {
        let some = ZoneForm::submitted(&Form::parse("network=lan&network=&network=lan2"));
        assert_eq!(some.networks, vec!["lan", "lan2"]);
        assert_eq!(some.values(true)["network"], json!(["lan", "lan2"]));

        // A zone that covers nothing clears the option rather than writing an
        // empty list, which is not a thing uci holds.
        let none = ZoneForm::submitted(&Form::parse("input=ACCEPT"));
        assert!(none.networks.is_empty());
        assert_eq!(none.values(true)["network"], Value::Null);
    }

    /// The rarely-touched settings are read off the section the same way as the
    /// rest, which is what the panel's Advanced tab states on the strip. The test
    /// that used to stand here asked at_defaults() — the page's mode gate — and went
    /// with it; what is worth pinning is that the values themselves arrive.
    #[test]
    fn the_rarely_touched_settings_are_read_off_the_section() {
        let fresh = ZoneForm::default();
        assert!(!fresh.mtu_fix);
        assert!(!fresh.log);
        assert!(fresh.family.is_empty());
        // The uplink clamps its MTU, so the panel has live state to state.
        assert!(read("cfg03dc81").mtu_fix);
        assert!(!read("guest_zone").mtu_fix);
    }

    #[test]
    fn a_value_firewall4_would_refuse_is_marked_and_nothing_is_written() {
        let bad_policy = ZoneForm {
            input: "MAYBE".into(),
            ..ZoneForm::default()
        };
        assert_eq!(
            bad_policy.validate(&networks()).get("input"),
            "Choose one of the offered policies."
        );

        let bad_network = ZoneForm {
            networks: vec!["lan".into(), "nowhere".into()],
            ..ZoneForm::default()
        };
        assert_eq!(
            bad_network.validate(&networks()).get("network"),
            "Choose from the networks this router defines."
        );
        assert!(ZoneForm {
            networks: vec!["lan".into(), "guest".into()],
            ..ZoneForm::default()
        }
        .validate(&networks())
        .is_empty());
    }

    #[test]
    fn a_new_zone_needs_a_free_name_firewall4_can_parse() {
        let taken = fixture::firewall().zone_names();
        let named = |name: &str| ZoneForm {
            name: name.into(),
            ..ZoneForm::default()
        };
        assert_eq!(
            named("").validate_named(&networks(), &taken).get("name"),
            "Give the zone a name."
        );
        assert_eq!(
            named("2fast")
                .validate_named(&networks(), &taken)
                .get("name"),
            NAME_HELP
        );
        assert_eq!(
            named("lan").validate_named(&networks(), &taken).get("name"),
            "A zone with this name already exists."
        );
        assert!(named("iot").validate_named(&networks(), &taken).is_empty());
    }

    #[test]
    fn firewall4s_own_identifier_rule_is_what_a_name_is_checked_against() {
        for name in [
            "lan",
            "iot_cloud",
            "guest-net",
            "_private",
            ".hidden",
            "a.b/c",
        ] {
            assert!(valid_identifier(name), "{name}");
        }
        for name in ["", "2fast", "-lead", "has space", "has*star", "wan!"] {
            assert!(!valid_identifier(name), "{name}");
        }
    }

    /// firewall4 matches a policy by case-insensitive prefix, so `acc` and `A`
    /// are both accept and the config an operator wrote by hand still means what
    /// they meant. An editor that matched the whole word would show them "use
    /// the global default" and clear a policy the zone was actually enforcing.
    #[test]
    fn a_policy_the_config_wrote_any_way_at_all_reads_as_one_choice() {
        assert_eq!(policy_value("ACCEPT"), "ACCEPT");
        assert_eq!(policy_value("accept"), "ACCEPT");
        assert_eq!(policy_value(" Reject "), "REJECT");
        assert_eq!(policy_value("DROP"), "DROP");
        assert_eq!(policy_value("acc"), "ACCEPT");
        assert_eq!(policy_value("A"), "ACCEPT");
        assert_eq!(policy_value("rej"), "REJECT");
        assert_eq!(policy_value("d"), "DROP");
        // Nothing stated is the zone following the baseline, not a verdict.
        assert_eq!(policy_value(""), "");
        assert_eq!(policy_value("   "), "");
        // A value fw4 matches no verdict to is one it refuses the zone over.
        assert_eq!(policy_value("maybe"), "");
        assert_eq!(policy_value("accepted"), "");
    }

    /// A zone's `log` is firewall4's bitfield, not a truth value: bit 1 logs
    /// refused traffic, bit 2 logs MSS clamps. The switch is bit 1 alone, so a
    /// zone that only logs its clamps reads as off — and a save has to leave that
    /// second bit exactly where it found it.
    ///
    /// The number is ucode's `+val`, which is not Rust's: `0x` is hex, a
    /// fraction is truncated toward zero by the bit test, and a negative value
    /// carries every bit its two's complement has.
    #[test]
    fn a_zones_log_is_a_bitfield_read_the_way_ucode_reads_a_number() {
        for (written, refused, mss) in [
            ("", false, false),
            ("0", false, false),
            ("1", true, false),
            ("2", false, true),
            ("3", true, true),
            (" 3 ", true, true),
            ("+5", true, false),
            // 0x, 0b and 0o are bases to ucode, and 017 is seventeen.
            ("0x3", true, true),
            ("0b101", true, false),
            ("0o17", true, true),
            ("017", true, false),
            // The bit test truncates toward zero; the exponent is a number too.
            ("2.9", false, true),
            ("1e2", false, false),
            // Every bit of a negative value's two's complement is set.
            ("-1", true, true),
            ("-2", false, true),
        ] {
            let read = log_bits(written);
            assert_eq!((read.refused, read.mss), (refused, mss), "{written:?}");
            assert!(read.unreadable.is_empty(), "{written:?}");
        }
    }

    /// firewall4 parses `log` as an integer and *skips the whole zone* when the
    /// value is not one — 'on' and 'yes' included, which uci elsewhere reads as
    /// true. Such a zone is not in the packet filter at all, so the one thing
    /// this editor must never do is claim it is logging. It must also not throw
    /// the value away: an operator who did not touch the switch did not ask for
    /// it to be rewritten.
    #[test]
    fn a_log_firewall4_cannot_read_never_reads_as_logging_and_survives_a_save() {
        for written in [
            "on", "yes", "true", "off", "no", "false", "maybe", "3abc", ".5", "1_0",
        ] {
            let read = log_bits(written);
            assert!(
                !read.refused && !read.mss,
                "{written:?} must not read as logging"
            );
            assert_eq!(
                read.unreadable, written,
                "{written:?} must be kept verbatim"
            );
        }

        let odd = stated(json!({".type": "zone", "name": "odd", "log": "on"}));
        assert!(!odd.log && !odd.log_mss);

        // The switch untouched: the value the config carries is written back as
        // it was found, not cleared and not reinterpreted.
        let untouched = ZoneForm::submitted(&Form::parse("input=ACCEPT")).carrying(&odd);
        assert_eq!(untouched.values(true)["log"], "on");

        // The switch turned on is the operator saying what they want instead.
        let turned_on = ZoneForm::submitted(&Form::parse("log=on")).carrying(&odd);
        assert_eq!(turned_on.values(true)["log"], "1");
    }

    #[test]
    fn a_save_writes_the_first_log_bit_and_carries_the_second() {
        let clamp_logger = stated(json!({".type": "zone", "name": "mss", "log": "2"}));
        // The zone reads as one that does not log refusals: that is what the
        // switch is about, and bit 2 is not it.
        assert!(!clamp_logger.log);
        assert_eq!(clamp_logger.values(true)["log"], "2");

        let on = ZoneForm::submitted(&Form::parse("log=on")).carrying(&clamp_logger);
        assert_eq!(on.values(true)["log"], "3");

        // The switch off, the clamp log still on: the option keeps the bit the
        // editor never drew rather than being cleared.
        let off = ZoneForm::submitted(&Form::parse("input=ACCEPT")).carrying(&clamp_logger);
        assert!(!off.log);
        assert_eq!(off.values(true)["log"], "2");

        // A zone with nothing of the sort clears the option in both directions.
        assert_eq!(
            ZoneForm::submitted(&Form::parse("log=on")).values(true)["log"],
            "1"
        );
        assert_eq!(
            ZoneForm::submitted(&Form::parse("input=ACCEPT")).values(true)["log"],
            Value::Null
        );
    }

    /// A policy fw4 reads by prefix survives the round trip as the verdict it
    /// really is: the select shows it, and the save writes the canonical word
    /// rather than clearing the option the zone was enforcing.
    #[test]
    fn a_prefix_written_policy_survives_the_round_trip() {
        let terse =
            stated(json!({".type": "zone", "name": "iot", "input": "acc", "forward": "rej"}));
        assert_eq!(terse.input, "ACCEPT");
        assert_eq!(terse.forward, "REJECT");
        let written = terse.values(true);
        assert_eq!(written["input"], "ACCEPT");
        assert_eq!(written["forward"], "REJECT");
        assert!(terse.validate(&networks()).is_empty());
    }

    /// A name long enough to wedge the firewall is refused before it is written:
    /// nftables caps a chain name at 255 characters, and fw4 builds every one of
    /// a zone's chains out of its name.
    #[test]
    fn a_name_too_long_for_the_chains_it_becomes_is_refused() {
        let taken = fixture::firewall().zone_names();
        let long = |n: usize| ZoneForm {
            name: "z".repeat(n),
            ..ZoneForm::default()
        };
        assert!(long(NAME_LIMIT)
            .validate_named(&networks(), &taken)
            .is_empty());
        assert_eq!(
            long(NAME_LIMIT + 1)
                .validate_named(&networks(), &taken)
                .get("name"),
            NAME_TOO_LONG
        );
        assert!(NAME_TOO_LONG.contains(&NAME_LIMIT.to_string()));
    }

    /// A zone keeps the name its section carries until the operator types another,
    /// and only then is `name` written: an untouched save states nothing about it.
    #[test]
    fn a_renamed_zone_writes_its_new_name_and_an_untouched_one_does_not() {
        let lan = read("cfg02dc81");
        assert_eq!(lan.named, "lan");
        assert!(lan.values(true).get("name").is_none());

        let renamed = ZoneForm::submitted(&Form::parse("name=home")).carrying(&lan);
        assert_eq!(renamed.named, "lan");
        assert_eq!(renamed.values(true)["name"], "home");
        assert!(renamed.renamed());
        assert!(!lan.renamed());
    }

    /// A new name answers the same questions a new zone's does, against every
    /// other zone; keeping the name asks none of them, so a zone is never refused
    /// for being called what it is already called.
    #[test]
    fn a_new_name_has_to_be_free_and_one_firewall4_can_parse() {
        let taken = fixture::firewall().zone_names();
        let lan = read("cfg02dc81");
        let named =
            |name: &str| ZoneForm::submitted(&Form::parse(&format!("name={name}"))).carrying(&lan);
        assert!(named("lan").validate_named(&networks(), &taken).is_empty());
        assert!(named("home").validate_named(&networks(), &taken).is_empty());
        assert_eq!(
            named("wan").validate_named(&networks(), &taken).get("name"),
            "A zone with this name already exists."
        );
        assert_eq!(
            named("").validate_named(&networks(), &taken).get("name"),
            "Give the zone a name."
        );
        assert_eq!(
            named("2fast")
                .validate_named(&networks(), &taken)
                .get("name"),
            NAME_HELP
        );
        assert_eq!(
            named(&"z".repeat(NAME_LIMIT + 1))
                .validate_named(&networks(), &taken)
                .get("name"),
            NAME_TOO_LONG
        );
    }

    /// A new zone starts where firewall4's own defaults are, and three of them are
    /// ON. Deriving Default would have started them off, so the first zone anybody
    /// created through the panel would have been written disabled and uncounted.
    #[test]
    fn a_new_zone_starts_where_firewall4_leaves_one() {
        let blank = ZoneForm::default();
        assert!(blank.enabled, "a zone nobody disabled is in force");
        assert!(blank.counter, "firewall4 counts unless told not to");
        assert!(blank.auto_helper, "and assigns helpers unless told not to");

        // And none of the three is written out, because absence already means on.
        let written = blank.values(false);
        for option in ["enabled", "counter", "auto_helper"] {
            assert!(
                written.get(option).is_none(),
                "{option} is firewall4's default; writing it would stage a change nobody made"
            );
        }
    }

    /// Turning one of the three off writes the off. This is the whole reason they
    /// needed controls: clearing the option would restore the behaviour the operator
    /// just turned off.
    #[test]
    fn turning_off_what_firewall4_keeps_on_writes_the_off() {
        let off = ZoneForm {
            enabled: false,
            counter: false,
            auto_helper: false,
            ..ZoneForm::default()
        };
        let written = off.values(true);
        for option in ["enabled", "counter", "auto_helper"] {
            assert_eq!(
                written[option], "0",
                "{option} turned off has to be written, not cleared"
            );
        }
    }

    /// The options a zone covers itself with, and the ones that narrow its
    /// masquerading, are written as lists — one uci `list` line each, not a single
    /// value with spaces in it.
    #[test]
    fn a_zones_own_addresses_are_written_as_lists() {
        let zone = ZoneForm {
            devices: vec!["tun0".into(), "wg0".into()],
            subnets: vec!["10.0.20.0/24".into()],
            masq: true,
            masq_src: vec!["10.0.20.0/24".into()],
            masq_dest: vec!["0.0.0.0/0".into()],
            masq_allow_invalid: true,
            ..ZoneForm::default()
        };
        let written = zone.values(true);
        assert_eq!(written["device"], json!(["tun0", "wg0"]));
        assert_eq!(written["subnet"], json!(["10.0.20.0/24"]));
        assert_eq!(written["masq_src"], json!(["10.0.20.0/24"]));
        assert_eq!(written["masq_dest"], json!(["0.0.0.0/0"]));
        assert_eq!(written["masq_allow_invalid"], "1");
    }

    /// An address firewall4 cannot parse is refused here, because fw4 answers it by
    /// skipping the entire zone — and every rule that names the zone goes with it.
    #[test]
    fn an_address_the_firewall_would_refuse_is_refused_first() {
        for field in ["subnet", "masq_src", "masq_dest"] {
            let mut zone = ZoneForm::default();
            let bad = vec!["10.0.20.0/33".into()];
            match field {
                "subnet" => zone.subnets = bad,
                "masq_src" => zone.masq_src = bad,
                _ => zone.masq_dest = bad,
            }
            let errors = zone.validate(&networks());
            assert!(
                !errors.get(field).is_empty(),
                "{field} accepted a prefix no address has"
            );
        }
        // And a log rate in a grammar fw4 does not read.
        let noisy = ZoneForm {
            log: true,
            log_limit: "often".into(),
            ..ZoneForm::default()
        };
        assert!(!noisy.validate(&networks()).get("log_limit").is_empty());

        // A helper it does not ship.
        let odd = ZoneForm {
            helper: "not-a-helper".into(),
            ..ZoneForm::default()
        };
        assert!(!odd.validate(&networks()).get("helper").is_empty());
    }
}
