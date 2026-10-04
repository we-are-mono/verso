// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

//! One rule as the editor holds it, between the config and the form.
//!
//! The editor owns a fixed set of firewall4's rule options — [`OWNED`] — and
//! nothing else. That set is the contract in both directions: reading a section
//! fills this struct from those options, saving writes them back, and every one
//! of them the submission no longer carries is cleared rather than left behind.
//! A rule may also carry options this editor does not draw; those are never
//! touched, so editing a rule cannot quietly discard what an expert wrote by
//! hand.
//!
//! Absence is the vocabulary for "no condition". firewall4 reads an empty option
//! as a value, not as nothing — an empty destination port matches port zero, not
//! every port — so a condition that is gone leaves as a `null` the shell turns
//! into a uci delete, never as an empty string.
//!
//! Validation is firewall4's, not a looser echo of it: a value this accepts is
//! one fw4's own parser accepts, because a rule fw4 refuses is silently absent
//! from the ruleset and the operator is never told.
//!
//! The grammars beneath the rule live here too — [`Tokens`], [`Inverted`],
//! [`SetMatch`], [`MarkMatch`], [`RateLimit`], [`Schedule`], [`Log`] and the
//! checks each of them answers to. They belong to firewall4 rather than to the
//! rule: a redirect carries most of the same vocabulary, and reading `!10/minute`
//! twice in two places is two chances to disagree with the parser that decides.

use std::collections::BTreeMap;
use std::net::{Ipv4Addr, Ipv6Addr};

use verso_plugin::{json, Form, Map, Section, Value};

use crate::model;

/// OWNED is every option this editor writes. A save states all of them: the ones
/// the submission carries as values, the rest as nulls that clear them — which is
/// why an option written only while something else is on (`log_limit`, under the
/// log switch) belongs here as much as one with a control of its own. Left out,
/// it could be written once and never taken away again.
pub const OWNED: [&str; 34] = [
    "name",
    "enabled",
    "src",
    "dest",
    "family",
    "proto",
    "device",
    "direction",
    "src_ip",
    "src_mac",
    "src_port",
    "dest_ip",
    "dest_port",
    "icmp_type",
    "ipset",
    "helper",
    "mark",
    "dscp",
    "limit",
    "limit_burst",
    "weekdays",
    "start_date",
    "stop_date",
    "start_time",
    "stop_time",
    "utc_time",
    "target",
    "set_helper",
    "set_mark",
    "set_xmark",
    "set_dscp",
    "counter",
    "log",
    "log_limit",
];

/// TARGETS are the verdicts firewall4 accepts on a rule, in the order the editor
/// offers them. Setting a mark and XOR-ing one are the same target with a
/// different parameter, so they are one choice here and a switch in the
/// advanced parameters, not two verdicts.
pub const TARGETS: [(&str, &str); 7] = [
    ("ACCEPT", "Accept"),
    ("REJECT", "Reject"),
    ("DROP", "Drop"),
    ("NOTRACK", "Do not track"),
    ("HELPER", "Assign a connection helper"),
    ("MARK", "Set a firewall mark"),
    ("DSCP", "Set a DSCP value"),
];

/// HELPERS is firewall4's shipped connection-helper catalogue
/// (/usr/share/firewall4/helpers). A helper outside it is one the packet filter
/// cannot load, so offering it would be offering a rule that never applies.
pub const HELPERS: [&str; 12] = [
    "amanda",
    "ftp",
    "RAS",
    "Q.931",
    "irc",
    "netbios-ns",
    "pptp",
    "sane",
    "sip",
    "snmp",
    "tftp",
    "rtsp",
];

/// DSCP_CLASSES are firewall4's named differentiated-services classes, the
/// vocabulary its dscp parser accepts beside a bare 0–63.
pub const DSCP_CLASSES: [&str; 23] = [
    "CS0", "CS1", "CS2", "CS3", "CS4", "CS5", "CS6", "CS7", "BE", "LE", "AF11", "AF12", "AF13",
    "AF21", "AF22", "AF23", "AF31", "AF32", "AF33", "AF41", "AF42", "AF43", "EF",
];

/// WEEKDAYS are the days a schedule may name, abbreviated the way uci writes
/// them; firewall4 matches a day name by its prefix.
pub const WEEKDAYS: [&str; 7] = ["Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"];

/// RATE_UNITS are the periods a rate limit may be expressed over.
pub const RATE_UNITS: [(&str, &str); 4] = [
    ("second", "Second"),
    ("minute", "Minute"),
    ("hour", "Hour"),
    ("day", "Day"),
];

/// SET_FIELDS are the packet fields a named-set match maps its dimensions onto.
pub const SET_FIELDS: [(&str, &str); 4] = [
    ("src", "Source address"),
    ("dst", "Destination address"),
    ("src_port", "Source port"),
    ("dest_port", "Destination port"),
];

/// Tokens is one condition written as two lists that share a uci option: the
/// values that match it, and the values that must not. firewall4 inverts a
/// single value with a leading `!`, so the two lists are one option on disk.
#[derive(Default, Clone)]
pub struct Tokens {
    pub include: Vec<String>,
    pub exclude: Vec<String>,
}

impl Tokens {
    /// read splits a uci list into the values that match and the values that are
    /// excluded.
    pub fn read(section: &Section, option: &str) -> Tokens {
        let mut tokens = Tokens::default();
        for value in model::values(section, option) {
            match value.strip_prefix('!') {
                Some(rest) => tokens.exclude.push(rest.trim().to_string()),
                None => tokens.include.push(value),
            }
        }
        tokens
    }

    pub fn from_form(form: &Form, option: &str) -> Tokens {
        Tokens {
            include: cleaned(form.all(option)),
            exclude: cleaned(form.all(&format!("{option}_not"))),
        }
    }

    /// is_set reports whether the condition says anything. A condition rendered
    /// with no values in it states nothing about the traffic, which is what
    /// carrying no option at all states, so the two are the same rule.
    pub fn is_set(&self) -> bool {
        !self.include.is_empty() || !self.exclude.is_empty()
    }

    /// option is the uci list: the matching values, then the excluded ones with
    /// firewall4's inversion mark on each.
    pub fn option(&self) -> Vec<String> {
        let mut values = self.include.clone();
        values.extend(self.exclude.iter().map(|value| format!("!{value}")));
        values
    }
}

/// Inverted is a condition whose whole value may be negated — firewall4's `!`
/// prefix on a scalar option.
#[derive(Default, Clone)]
pub struct Inverted {
    pub negated: bool,
    pub value: String,
}

impl Inverted {
    pub fn read(section: &Section, option: &str) -> Inverted {
        Inverted::parse(&section.scalar(option))
    }

    fn parse(raw: &str) -> Inverted {
        match raw.trim().strip_prefix('!') {
            Some(rest) => Inverted {
                negated: true,
                value: rest.trim().to_string(),
            },
            None => Inverted {
                negated: false,
                value: raw.trim().to_string(),
            },
        }
    }

    pub fn from_form(form: &Form, value_field: &str, comparison_field: &str) -> Inverted {
        Inverted {
            negated: form.get(comparison_field) == EXCLUDE,
            value: form.get(value_field).trim().to_string(),
        }
    }

    /// comparison is the form value of the include/exclude select.
    pub fn comparison(&self) -> &'static str {
        if self.negated {
            EXCLUDE
        } else {
            INCLUDE
        }
    }

    pub fn option(&self, body: &str) -> String {
        match self.negated {
            true => format!("!{body}"),
            false => body.to_string(),
        }
    }
}

/// INCLUDE and EXCLUDE are the two sides of every comparison select.
pub const INCLUDE: &str = "include";
pub const EXCLUDE: &str = "exclude";

/// SetMatch is a named-set condition: the set, and which packet fields its
/// dimensions are compared against.
#[derive(Default, Clone)]
pub struct SetMatch {
    pub set: Inverted,
    pub fields: [String; 3],
}

/// MarkMatch is a packet-mark condition: the value, and the mask it is compared
/// through.
#[derive(Default, Clone)]
pub struct MarkMatch {
    pub mark: Inverted,
    pub mask: String,
}

/// RateLimit is firewall4's `limit`: how many packets over what period, whether
/// the rule matches below the rate or only above it, and the initial allowance.
#[derive(Clone)]
pub struct RateLimit {
    pub over: bool,
    pub count: String,
    pub unit: String,
    pub burst: String,
}

impl Default for RateLimit {
    fn default() -> RateLimit {
        RateLimit {
            over: false,
            count: String::new(),
            unit: "second".into(),
            burst: String::new(),
        }
    }
}

/// Schedule restricts a rule to weekdays, a date range, and a daily window,
/// read on one clock or the other.
#[derive(Default, Clone)]
pub struct Schedule {
    pub weekdays: Vec<String>,
    pub start_date: String,
    pub stop_date: String,
    pub start_time: String,
    pub stop_time: String,
    pub utc: bool,
}

impl Schedule {
    pub fn is_set(&self) -> bool {
        !self.weekdays.is_empty()
            || ![
                &self.start_date,
                &self.stop_date,
                &self.start_time,
                &self.stop_time,
            ]
            .iter()
            .all(|value| value.is_empty())
    }
}

/// Device ties a rule to one kernel device, on the way in or on the way out.
#[derive(Clone)]
pub struct Device {
    pub direction: String,
    pub name: String,
}

impl Default for Device {
    fn default() -> Device {
        Device {
            direction: "in".into(),
            name: String::new(),
        }
    }
}

/// Log says whether matching packets reach the system log, under what prefix,
/// and how fast they may arrive there.
#[derive(Default, Clone)]
pub struct Log {
    pub on: bool,
    pub prefix: String,
    pub limit: String,
}

/// RuleForm is one rule as the editor holds it: every option it owns, in the
/// shape its controls take rather than the shape uci writes.
#[derive(Clone)]
pub struct RuleForm {
    pub name: String,
    pub enabled: bool,
    pub src: String,
    pub dest: String,
    pub family: String,
    pub proto: Vec<String>,
    pub target: String,
    pub set_helper: String,
    pub mark_xor: bool,
    pub set_mark: String,
    pub set_mark_mask: String,
    pub set_dscp: String,
    pub counter: bool,
    pub log: Log,
    pub device: Device,
    pub src_ip: Tokens,
    pub src_mac: Tokens,
    pub src_port: Tokens,
    pub dest_ip: Tokens,
    pub dest_port: Tokens,
    pub icmp_type: Vec<String>,
    pub ipset: SetMatch,
    pub helper: Inverted,
    pub mark: MarkMatch,
    pub dscp: Inverted,
    pub rate: RateLimit,
    pub schedule: Schedule,
}

impl Default for RuleForm {
    /// A blank rule starts where firewall4's own defaults are: on, accepting,
    /// over TCP and UDP, counted.
    fn default() -> RuleForm {
        RuleForm {
            name: String::new(),
            enabled: true,
            src: String::new(),
            dest: String::new(),
            family: String::new(),
            proto: vec!["tcp".into(), "udp".into()],
            target: "ACCEPT".into(),
            set_helper: String::new(),
            mark_xor: false,
            set_mark: String::new(),
            set_mark_mask: String::new(),
            set_dscp: String::new(),
            counter: true,
            log: Log::default(),
            device: Device::default(),
            src_ip: Tokens::default(),
            src_mac: Tokens::default(),
            src_port: Tokens::default(),
            dest_ip: Tokens::default(),
            dest_port: Tokens::default(),
            icmp_type: Vec::new(),
            ipset: SetMatch::default(),
            helper: Inverted::default(),
            mark: MarkMatch::default(),
            dscp: Inverted::default(),
            rate: RateLimit::default(),
            schedule: Schedule::default(),
        }
    }
}

impl RuleForm {
    /// read fills the form from a rule section, with firewall4's defaults where
    /// an option is absent.
    pub fn read(section: &Section) -> RuleForm {
        let mark = Inverted::read(section, "mark");
        let (mark_value, mark_mask) = split_mask(&mark.value);
        let set_mark = section.scalar("set_mark");
        let set_xmark = section.scalar("set_xmark");
        let mark_xor = set_mark.is_empty() && !set_xmark.is_empty();
        let (set_mark_value, set_mark_mask) =
            split_mask(if mark_xor { &set_xmark } else { &set_mark });
        RuleForm {
            name: section.scalar("name"),
            enabled: model::flag(section, "enabled", true),
            src: section.scalar("src"),
            dest: section.scalar("dest"),
            family: family_value(&section.scalar("family")),
            proto: model::values(section, "proto"),
            target: section.scalar("target").to_uppercase(),
            set_helper: section.scalar("set_helper"),
            mark_xor,
            set_mark: set_mark_value,
            set_mark_mask,
            set_dscp: section.scalar("set_dscp"),
            counter: model::flag(section, "counter", true),
            log: read_log(section),
            device: Device {
                direction: match section.scalar("direction").as_str() {
                    "out" | "egress" => "out".into(),
                    _ => "in".into(),
                },
                name: section.scalar("device"),
            },
            src_ip: Tokens::read(section, "src_ip"),
            src_mac: Tokens::read(section, "src_mac"),
            src_port: Tokens::read(section, "src_port"),
            dest_ip: Tokens::read(section, "dest_ip"),
            dest_port: Tokens::read(section, "dest_port"),
            icmp_type: model::values(section, "icmp_type"),
            ipset: read_ipset(section),
            helper: Inverted::read(section, "helper"),
            mark: MarkMatch {
                mark: Inverted {
                    negated: mark.negated,
                    value: mark_value,
                },
                mask: mark_mask,
            },
            dscp: Inverted::read(section, "dscp"),
            rate: read_limit(&section.scalar("limit"), &section.scalar("limit_burst")),
            schedule: Schedule {
                weekdays: model::values(section, "weekdays"),
                start_date: section.scalar("start_date"),
                stop_date: section.scalar("stop_date"),
                start_time: section.scalar("start_time"),
                stop_time: section.scalar("stop_time"),
                utc: model::flag(section, "utc_time", false),
            },
        }
    }

    /// submitted reads the form the browser posted. A control the shell did not
    /// render posts nothing, and a condition removed from the editor takes its
    /// controls with it, so an absent field is the operator saying that
    /// condition is gone.
    pub fn submitted(form: &Form) -> RuleForm {
        let rate = RateLimit {
            over: form.get("limit_match") == "over",
            count: form.get("limit").trim().to_string(),
            unit: rate_unit(&form.get("limit_unit")),
            burst: form.get("limit_burst").trim().to_string(),
        };
        RuleForm {
            name: form.get("name").trim().to_string(),
            enabled: !form.get("enabled").is_empty(),
            src: form.get("src").trim().to_string(),
            dest: form.get("dest").trim().to_string(),
            family: family_value(&form.get("family")),
            // A protocol list arrives either as a token apiece or as one choice
            // naming a pair ("tcp udp") — the panel offers the pairs the canvas
            // does, and uci holds a list either way. Splitting on whitespace
            // reads both without the control having to know which it is.
            proto: cleaned(split_words(form.all("proto"))),
            target: form.get("target").trim().to_uppercase(),
            set_helper: form.get("set_helper").trim().to_string(),
            mark_xor: form.get("mark_operation") == "xor",
            set_mark: form.get("set_mark").trim().to_string(),
            set_mark_mask: form.get("set_mark_mask").trim().to_string(),
            set_dscp: form.get("set_dscp").trim().to_string(),
            counter: !form.get("counter").is_empty(),
            log: Log {
                on: !form.get("log").is_empty(),
                // Not trimmed, alone among the form's text: firewall4 writes this
                // string in front of the packet it logs, so the space somebody
                // put at the end of "guest-audit: " is the space between the
                // prefix and the line. Trimming it silently reformatted every
                // log line a rule wrote.
                prefix: form.get("log_prefix"),
                limit: form.get("log_limit").trim().to_string(),
            },
            device: Device {
                direction: match form.get("direction").as_str() {
                    "out" => "out".into(),
                    _ => "in".into(),
                },
                name: form.get("device").trim().to_string(),
            },
            src_ip: Tokens::from_form(form, "src_ip"),
            src_mac: Tokens::from_form(form, "src_mac"),
            src_port: Tokens::from_form(form, "src_port"),
            dest_ip: Tokens::from_form(form, "dest_ip"),
            dest_port: Tokens::from_form(form, "dest_port"),
            icmp_type: cleaned(form.all("icmp_type")),
            ipset: SetMatch {
                set: Inverted::from_form(form, "ipset", "ipset_match"),
                fields: [
                    form.get("ipset_field_1"),
                    form.get("ipset_field_2"),
                    form.get("ipset_field_3"),
                ],
            },
            helper: Inverted::from_form(form, "helper", "helper_match"),
            mark: MarkMatch {
                mark: Inverted::from_form(form, "mark_value", "mark_match"),
                mask: form.get("mark_mask").trim().to_string(),
            },
            dscp: Inverted::from_form(form, "dscp", "dscp_match"),
            rate,
            schedule: Schedule {
                weekdays: cleaned(form.all("weekdays")),
                start_date: form.get("start_date").trim().to_string(),
                stop_date: form.get("stop_date").trim().to_string(),
                start_time: form.get("start_time").trim().to_string(),
                stop_time: form.get("stop_time").trim().to_string(),
                utc: form.get("time_basis") == "utc",
            },
        }
    }

    /// active reports whether a condition is part of the rule — the same
    /// question for the editor's picker and for the save: a condition states
    /// something or it is not there.
    pub fn active(&self, key: &str) -> bool {
        match key {
            "device" => !self.device.name.is_empty(),
            "src_ip" => self.src_ip.is_set(),
            "src_mac" => self.src_mac.is_set(),
            "src_port" => self.src_port.is_set(),
            "dest_ip" => self.dest_ip.is_set(),
            "dest_port" => self.dest_port.is_set(),
            "icmp_type" => !self.icmp_type.is_empty(),
            "ipset" => !self.ipset.set.value.is_empty(),
            "helper" => !self.helper.value.is_empty(),
            "mark" => !self.mark.mark.value.is_empty(),
            "dscp" => !self.dscp.value.is_empty(),
            "rate" => !self.rate.count.is_empty(),
            "schedule" => self.schedule.is_set(),
            _ => false,
        }
    }

    /// values is what the save writes: every option the rule states, plus — when
    /// editing an existing section — a null for each owned option it no longer
    /// states. A new section has nothing to clear.
    pub fn values(&self, existing: bool) -> Value {
        let mut values = Map::new();
        let mut set = |option: &str, value: Value| {
            values.insert(option.to_string(), value);
        };

        set("enabled", json!(bit(self.enabled)));
        set("target", json!(self.target.clone()));
        set("counter", json!(bit(self.counter)));
        for (option, value) in [
            ("name", &self.name),
            ("src", &self.src),
            ("dest", &self.dest),
            ("family", &self.family),
        ] {
            if !value.is_empty() {
                set(option, json!(value.clone()));
            }
        }
        if !self.proto.is_empty() {
            set("proto", json!(self.proto.clone()));
        }
        self.action_values(&mut set);
        self.condition_values(&mut set);
        self.log_values(&mut set);

        if existing {
            for option in OWNED {
                values.entry(option.to_string()).or_insert(Value::Null);
            }
        } else {
            values.retain(|_, value| !value.is_null());
        }
        Value::Object(values)
    }

    /// action_values writes the parameters of the chosen verdict and only those:
    /// firewall4 refuses a mark rule with no mark, and ignores a mark set beside
    /// an accept, so carrying a parameter for an action the rule does not take
    /// would be config that lies about what the rule does.
    fn action_values(&self, set: &mut impl FnMut(&str, Value)) {
        match self.target.as_str() {
            "HELPER" => set("set_helper", json!(self.set_helper.clone())),
            "MARK" => {
                let mark = join_mask(&self.set_mark, &self.set_mark_mask);
                set(
                    if self.mark_xor {
                        "set_xmark"
                    } else {
                        "set_mark"
                    },
                    json!(mark),
                );
            }
            "DSCP" => set("set_dscp", json!(self.set_dscp.clone())),
            _ => {}
        }
    }

    fn condition_values(&self, set: &mut impl FnMut(&str, Value)) {
        if self.active("device") {
            set("device", json!(self.device.name.clone()));
            set("direction", json!(self.device.direction.clone()));
        }
        for (key, option, tokens) in [
            ("src_ip", "src_ip", &self.src_ip),
            ("src_mac", "src_mac", &self.src_mac),
            ("src_port", "src_port", &self.src_port),
            ("dest_ip", "dest_ip", &self.dest_ip),
            ("dest_port", "dest_port", &self.dest_port),
        ] {
            if self.active(key) {
                set(option, json!(tokens.option()));
            }
        }
        if self.active("icmp_type") {
            set("icmp_type", json!(self.icmp_type.clone()));
        }
        if self.active("ipset") {
            let fields: Vec<&str> = self
                .ipset
                .fields
                .iter()
                .map(String::as_str)
                .filter(|field| !field.is_empty())
                .collect();
            let mut body = self.ipset.set.value.clone();
            if !fields.is_empty() {
                body = format!("{body} {}", fields.join(" "));
            }
            set("ipset", json!(self.ipset.set.option(&body)));
        }
        if self.active("helper") {
            let value = self.helper.value.clone();
            set("helper", json!(self.helper.option(&value)));
        }
        if self.active("mark") {
            let body = join_mask(&self.mark.mark.value, &self.mark.mask);
            set("mark", json!(self.mark.mark.option(&body)));
        }
        if self.active("dscp") {
            let value = self.dscp.value.clone();
            set("dscp", json!(self.dscp.option(&value)));
        }
        if self.active("rate") {
            let rate = format!("{}/{}", self.rate.count, self.rate.unit);
            let limit = match self.rate.over {
                true => format!("!{rate}"),
                false => rate,
            };
            set("limit", json!(limit));
            if !self.rate.burst.is_empty() {
                set("limit_burst", json!(self.rate.burst.clone()));
            }
        }
        if self.active("schedule") {
            let schedule = &self.schedule;
            if !schedule.weekdays.is_empty() {
                // One value, not a uci list. firewall4 parses this option with
                // parse_opt and splits the string itself; a list here is
                // "option 'weekdays' must not be a list" and the whole section
                // is skipped — a rule that was written, saved, and silently
                // never in the ruleset.
                set("weekdays", json!(schedule.weekdays.join(" ")));
            }
            for (option, value) in [
                ("start_date", &schedule.start_date),
                ("stop_date", &schedule.stop_date),
                ("start_time", &schedule.start_time),
                ("stop_time", &schedule.stop_time),
            ] {
                if !value.is_empty() {
                    set(option, json!(value.clone()));
                }
            }
            if schedule.utc {
                set("utc_time", json!("1"));
            }
        }
    }

    /// log_values writes firewall4's `log`, which is one option doing two jobs:
    /// a truth value turns logging on under the rule's own name, any other
    /// string turns it on under that prefix.
    fn log_values(&self, set: &mut impl FnMut(&str, Value)) {
        if !self.log.on {
            return;
        }
        let value = match self.log.prefix.is_empty() {
            true => "1".to_string(),
            false => self.log.prefix.clone(),
        };
        set("log", json!(value));
        if !self.log.limit.is_empty() {
            set("log_limit", json!(self.log.limit.clone()));
        }
    }

    /// validate answers firewall4's question — would the packet filter accept
    /// this rule — before the write, because a rule fw4 refuses is dropped from
    /// the ruleset with nothing said to the operator.
    pub fn validate(&self, zones: &[String]) -> Errors {
        let mut errors = Errors::default();
        self.validate_path(zones, &mut errors);
        self.validate_action(&mut errors);
        self.validate_conditions(&mut errors);
        self.validate_schedule(&mut errors);
        if self.log.on && !self.log.limit.is_empty() && !valid_limit(&self.log.limit) {
            errors.field("log_limit", "Write a rate like 10/minute.");
        }
        errors
    }

    fn validate_path(&self, zones: &[String], errors: &mut Errors) {
        for (field, value) in [("src", &self.src), ("dest", &self.dest)] {
            if !value.is_empty() && value != "*" && !zones.iter().any(|zone| zone == value) {
                errors.field(field, &format!("There is no zone called “{value}”."));
            }
        }
        if !matches!(self.family.as_str(), "" | "ipv4" | "ipv6") {
            errors.field("family", "Choose one of the offered address families.");
        }
        if !TARGETS.iter().any(|(value, _)| *value == self.target) {
            errors.field("target", "Choose one of the offered actions.");
        }
        errors.items("proto", &self.proto, |value| {
            valid_protocol(value).then_some(()).ok_or(PROTOCOL_HELP)
        });
    }

    fn validate_action(&self, errors: &mut Errors) {
        match self.target.as_str() {
            "HELPER" if !HELPERS.contains(&self.set_helper.as_str()) => {
                errors.field("set_helper", "Choose the helper this rule assigns.");
            }
            "MARK" => {
                if self.set_mark.is_empty() {
                    errors.field("set_mark", "A mark rule needs a value to set.");
                } else if !valid_mark(&self.set_mark) {
                    errors.field("set_mark", MARK_HELP);
                }
                if !self.set_mark_mask.is_empty() && !valid_mark(&self.set_mark_mask) {
                    errors.field("set_mark_mask", MARK_HELP);
                }
            }
            "DSCP" if !valid_dscp(&self.set_dscp) => {
                errors.field("set_dscp", "Choose the DSCP value this rule applies.");
            }
            _ => {}
        }
    }

    fn validate_conditions(&self, errors: &mut Errors) {
        for (key, option, tokens) in [
            ("src_ip", "src_ip", &self.src_ip),
            ("dest_ip", "dest_ip", &self.dest_ip),
        ] {
            if self.active(key) {
                tokens.check(option, errors, valid_address, ADDRESS_HELP);
            }
        }
        for (key, option, tokens) in [
            ("src_port", "src_port", &self.src_port),
            ("dest_port", "dest_port", &self.dest_port),
        ] {
            if self.active(key) {
                tokens.check(option, errors, valid_port, PORT_HELP);
            }
        }
        if self.active("src_mac") {
            self.src_mac.check("src_mac", errors, valid_mac, MAC_HELP);
        }
        if self.active("icmp_type") {
            errors.items("icmp_type", &self.icmp_type, |value| {
                valid_icmp_type(value).then_some(()).ok_or(ICMP_HELP)
            });
        }
        if self.active("mark") {
            if !valid_mark(&self.mark.mark.value) {
                errors.field("mark_value", MARK_HELP);
            }
            if !self.mark.mask.is_empty() && !valid_mark(&self.mark.mask) {
                errors.field("mark_mask", MARK_HELP);
            }
        }
        if self.active("dscp") && !valid_dscp(&self.dscp.value) {
            errors.field("dscp", "Choose one of the offered DSCP values.");
        }
        if self.active("helper") && !HELPERS.contains(&self.helper.value.as_str()) {
            errors.field("helper", "Choose one of the offered connection helpers.");
        }
        if self.active("rate") {
            if !valid_count(&self.rate.count) {
                errors.field("limit", "Write how many packets, as a whole number.");
            }
            if !self.rate.burst.is_empty() && !valid_count(&self.rate.burst) {
                errors.field(
                    "limit_burst",
                    "Write the burst allowance as a whole number.",
                );
            }
        }
    }

    fn validate_schedule(&self, errors: &mut Errors) {
        if !self.active("schedule") {
            return;
        }
        for (field, value) in [
            ("start_date", &self.schedule.start_date),
            ("stop_date", &self.schedule.stop_date),
        ] {
            if !value.is_empty() && !valid_date(value) {
                errors.field(field, "Write a date as YYYY-MM-DD.");
            }
        }
        for (field, value) in [
            ("start_time", &self.schedule.start_time),
            ("stop_time", &self.schedule.stop_time),
        ] {
            if !value.is_empty() && !valid_time(value) {
                errors.field(field, "Write a time of day as HH:MM or HH:MM:SS.");
            }
        }
        for day in &self.schedule.weekdays {
            if !WEEKDAYS.contains(&day.as_str()) {
                errors.field("weekdays", "Choose from the days offered.");
            }
        }
    }
}

impl Tokens {
    /// check validates both sides of the condition against the same rule, and
    /// reports a failure against the list the value was written in.
    pub fn check(
        &self,
        option: &str,
        errors: &mut Errors,
        valid: fn(&str) -> bool,
        help: &'static str,
    ) {
        errors.items(option, &self.include, |value| {
            valid(value).then_some(()).ok_or(help)
        });
        errors.items(&format!("{option}_not"), &self.exclude, |value| {
            valid(value).then_some(()).ok_or(help)
        });
    }
}

pub const ADDRESS_HELP: &str = "Write an IP address or a network in CIDR form.";
pub const PORT_HELP: &str = "Write a port from 0 to 65535, or a range such as 1024-65535.";
pub const PROTOCOL_HELP: &str =
    "Write a protocol name such as tcp, or an IP protocol number from 0 to 255.";
pub const MAC_HELP: &str = "Write a MAC address as six hex pairs, such as 00:11:22:33:44:55.";
const ICMP_HELP: &str = "Write an ICMP type name, or a numeric type or type/code.";
pub const MARK_HELP: &str = "Write a mark as a decimal or hexadecimal number.";

/// Errors is what a submission got wrong, addressed to the controls that carry
/// the offending values: a message per field, and a message per item of a list.
/// The shell reads the annotations back off the re-rendered tree, so a form that
/// reports one is a 422 whether or not the plugin says so as well.
#[derive(Default, Clone)]
pub struct Errors {
    fields: BTreeMap<String, String>,
    lists: BTreeMap<String, BTreeMap<String, String>>,
}

impl Errors {
    pub fn field(&mut self, name: &str, message: &str) {
        self.fields
            .entry(name.to_string())
            .or_insert_with(|| message.to_string());
    }

    /// items validates one list and records a message under the index of each
    /// item that failed — which is how a repeating control says which row is
    /// wrong rather than reddening the whole list.
    pub fn items<F>(&mut self, name: &str, values: &[String], check: F)
    where
        F: Fn(&str) -> Result<(), &'static str>,
    {
        for (index, value) in values.iter().enumerate() {
            if let Err(message) = check(value) {
                self.lists
                    .entry(name.to_string())
                    .or_default()
                    .insert(index.to_string(), message.to_string());
            }
        }
    }

    /// get is the message for one field, or "" — what a field widget carries.
    pub fn get(&self, name: &str) -> &str {
        self.fields.get(name).map(String::as_str).unwrap_or("")
    }

    /// list is the per-item messages for one list, keyed by index as a string.
    pub fn list(&self, name: &str) -> BTreeMap<String, String> {
        self.lists.get(name).cloned().unwrap_or_default()
    }

    pub fn is_empty(&self) -> bool {
        self.fields.is_empty() && self.lists.is_empty()
    }

    /// merge folds another set of refusals in. One submission may be validated
    /// by more than one owner — a zone panel writes its zone and the crossings
    /// out of it, which are different section types with different rules — and
    /// the operator is owed every refusal at once rather than one per attempt.
    /// The first message on a field wins, as it does within one set.
    pub fn merge(&mut self, other: Errors) {
        for (name, message) in other.fields {
            self.fields.entry(name).or_insert(message);
        }
        for (name, items) in other.lists {
            let list = self.lists.entry(name).or_default();
            for (index, message) in items {
                list.entry(index).or_insert(message);
            }
        }
    }
}

// ---- reading firewall4's written forms ----

/// read_log reads the option that says both whether to log and under what
/// prefix: a truth value is the plain on/off, anything else is a prefix and
/// means on.
pub fn read_log(section: &Section) -> Log {
    let written = section.scalar("log");
    let (on, prefix) = match model::boolean(&written) {
        Some(on) => (on, String::new()),
        None if written.is_empty() => (false, String::new()),
        None => (true, written),
    };
    Log {
        on,
        prefix,
        limit: section.scalar("log_limit"),
    }
}

pub fn read_ipset(section: &Section) -> SetMatch {
    let written = Inverted::read(section, "ipset");
    let mut parts = written
        .value
        .split([' ', '\t', ','])
        .filter(|p| !p.is_empty());
    let name = parts.next().unwrap_or_default().to_string();
    let mut fields = [String::new(), String::new(), String::new()];
    for (slot, field) in fields.iter_mut().zip(parts) {
        *slot = match field {
            "dest" => "dst".to_string(),
            field => field.to_string(),
        };
    }
    SetMatch {
        set: Inverted {
            negated: written.negated,
            value: name,
        },
        fields,
    }
}

pub fn read_limit(limit: &str, burst: &str) -> RateLimit {
    let written = Inverted::parse(limit);
    let (count, unit) = match written.value.split_once('/') {
        Some((count, unit)) => (count.trim().to_string(), unit.trim()),
        None => (written.value.clone(), "second"),
    };
    RateLimit {
        over: written.negated,
        count,
        unit: rate_unit(unit),
        burst: burst.trim().to_string(),
    }
}

/// family_value normalizes firewall4's family spellings onto the two the editor
/// offers; anything else leaves both families in play, as fw4 does. A zone
/// narrows its family through the same option, so both editors read it here.
pub fn family_value(written: &str) -> String {
    match written {
        value if value.contains('4') || value == "inet" => "ipv4".into(),
        value if value.contains('6') => "ipv6".into(),
        _ => String::new(),
    }
}

/// rate_unit maps a written period onto the closed set the way firewall4 does —
/// by prefix, case-insensitively — falling back to seconds, fw4's own default.
pub fn rate_unit(written: &str) -> String {
    let written = written.trim().to_lowercase();
    if !written.is_empty() {
        for (unit, _) in RATE_UNITS {
            if unit.starts_with(&written) {
                return unit.to_string();
            }
        }
    }
    "second".to_string()
}

pub fn split_mask(written: &str) -> (String, String) {
    match written.trim().split_once('/') {
        Some((value, mask)) => (value.trim().to_string(), mask.trim().to_string()),
        None => (written.trim().to_string(), String::new()),
    }
}

pub fn join_mask(value: &str, mask: &str) -> String {
    match mask.is_empty() {
        true => value.to_string(),
        false => format!("{value}/{mask}"),
    }
}

pub fn bit(on: bool) -> &'static str {
    match on {
        true => "1",
        false => "0",
    }
}

/// split_words flattens values that carry more than one word into the words
/// themselves: a choice naming a protocol pair states it as one value, and uci
/// holds it as two.
pub fn split_words(values: Vec<String>) -> Vec<String> {
    values
        .iter()
        .flat_map(|value| value.split_whitespace().map(str::to_string))
        .collect()
}

/// cleaned drops the blank slots a repeating control posts and trims what is
/// left — the multi-value contract every list and checks field shares.
pub fn cleaned(values: Vec<String>) -> Vec<String> {
    values
        .into_iter()
        .map(|value| value.trim().to_string())
        .filter(|value| !value.is_empty())
        .collect()
}

// ---- firewall4's own acceptance tests ----

pub fn valid_protocol(value: &str) -> bool {
    let value = value.strip_prefix('!').unwrap_or(value);
    if value.is_empty() {
        return false;
    }
    if value.chars().all(|c| c.is_ascii_digit()) {
        return value.parse::<u16>().is_ok_and(|number| number <= 255);
    }
    value
        .chars()
        .all(|c| c.is_ascii_alphanumeric() || c == '-' || c == '.')
}

pub fn valid_port(value: &str) -> bool {
    let value = value.strip_prefix('!').unwrap_or(value).trim();
    let number = |part: &str| part.parse::<u32>().ok().filter(|port| *port <= 65535);
    match value.split_once(['-', ':']) {
        Some((low, high)) => match (number(low), number(high)) {
            (Some(low), Some(high)) => low <= high,
            _ => false,
        },
        None => number(value).is_some(),
    }
}

/// valid_address accepts what firewall4's network parser does: an IPv4 address
/// with an optional prefix or dotted mask, or an IPv6 address with an optional
/// prefix. It reads the shape of the value, not the registry it belongs to.
pub fn valid_address(value: &str) -> bool {
    let value = value.strip_prefix('!').unwrap_or(value).trim();
    let (address, suffix) = match value.split_once('/') {
        Some((address, suffix)) => (address, Some(suffix)),
        None => (value, None),
    };
    if address.contains(':') {
        return valid_ipv6(address) && suffix.is_none_or(|s| prefix_within(s, 128));
    }
    if !valid_ipv4(address) {
        return false;
    }
    match suffix {
        None => true,
        Some(suffix) if suffix.contains('.') => valid_ipv4(suffix),
        Some(suffix) => prefix_within(suffix, 32),
    }
}

fn valid_ipv4(address: &str) -> bool {
    address.parse::<Ipv4Addr>().is_ok()
}

fn valid_ipv6(address: &str) -> bool {
    address.parse::<Ipv6Addr>().is_ok()
}

fn prefix_within(value: &str, limit: u32) -> bool {
    value.parse::<u32>().is_ok_and(|prefix| prefix <= limit)
}

pub fn valid_mac(value: &str) -> bool {
    let value = value.strip_prefix('!').unwrap_or(value).trim();
    let pairs: Vec<&str> = value.split([':', '-']).collect();
    pairs.len() == 6
        && pairs
            .iter()
            .all(|pair| pair.len() == 2 && pair.chars().all(|c| c.is_ascii_hexdigit()))
}

fn valid_icmp_type(value: &str) -> bool {
    let value = value.strip_prefix('!').unwrap_or(value).trim();
    if value.is_empty() {
        return false;
    }
    let numeric = |part: &str| part.parse::<u16>().is_ok_and(|number| number <= 255);
    match value.split_once('/') {
        Some((typ, code)) => numeric(typ) && numeric(code),
        None if value.chars().next().is_some_and(|c| c.is_ascii_digit()) => numeric(value),
        None => value
            .chars()
            .all(|c| c.is_ascii_alphanumeric() || c == '-' || c == '_'),
    }
}

pub fn valid_mark(value: &str) -> bool {
    let value = value.trim();
    let (digits, radix) = match value
        .strip_prefix("0x")
        .or_else(|| value.strip_prefix("0X"))
    {
        Some(digits) => (digits, 16),
        None => (value, 10),
    };
    !digits.is_empty() && u32::from_str_radix(digits, radix).is_ok()
}

fn valid_dscp(value: &str) -> bool {
    DSCP_CLASSES.contains(&value.to_uppercase().as_str())
        || value.parse::<u8>().is_ok_and(|class| class <= 0x3F)
}

pub fn valid_count(value: &str) -> bool {
    !value.is_empty() && value.chars().all(|c| c.is_ascii_digit())
}

/// valid_date accepts firewall4's date shapes: a year, a year and month, or a
/// full date, within the range its parser bounds.
pub fn valid_date(value: &str) -> bool {
    let parts: Vec<&str> = value.split('-').collect();
    if parts.is_empty() || parts.len() > 3 {
        return false;
    }
    let Ok(year) = parts[0].parse::<u32>() else {
        return false;
    };
    if parts[0].len() != 4 || !(1970..=2038).contains(&year) {
        return false;
    }
    let bounded = |part: &str, max: u32| {
        (1..=2).contains(&part.len()) && part.parse::<u32>().is_ok_and(|n| n >= 1 && n <= max)
    };
    parts.get(1).is_none_or(|month| bounded(month, 12))
        && parts.get(2).is_none_or(|day| bounded(day, 31))
}

/// valid_time accepts firewall4's time shapes: an hour, an hour and minute, or
/// a full time of day.
pub fn valid_time(value: &str) -> bool {
    let parts: Vec<&str> = value.split(':').collect();
    if parts.is_empty() || parts.len() > 3 {
        return false;
    }
    let bounded = |part: &str, max: u32| {
        (1..=2).contains(&part.len()) && part.parse::<u32>().is_ok_and(|n| n <= max)
    };
    bounded(parts[0], 23)
        && parts.get(1).is_none_or(|minute| bounded(minute, 59))
        && parts.get(2).is_none_or(|second| bounded(second, 59))
}

/// valid_limit reports whether a rate reads as firewall4's limit: a count, and
/// optionally the period it is counted over.
/// It is public because a zone's log limit is the same option in the same grammar:
/// firewall4 reads `log_limit` on a zone exactly as it reads `limit` on a rule, and
/// two validators for one grammar would be two chances to disagree with fw4.
pub fn valid_limit(value: &str) -> bool {
    let value = value.strip_prefix('!').unwrap_or(value).trim();
    let (count, unit) = match value.split_once('/') {
        Some((count, unit)) => (count, Some(unit.trim().to_lowercase())),
        None => (value, None),
    };
    if !valid_count(count.trim()) {
        return false;
    }
    match unit {
        None => true,
        Some(unit) => {
            !unit.is_empty() && RATE_UNITS.iter().any(|(name, _)| name.starts_with(&unit))
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::fixture;
    use verso_plugin::Snapshot;

    fn everything() -> RuleForm {
        let snapshot = fixture::editor_snapshot();
        let section = snapshot
            .section(crate::model::CONFIG, "everything")
            .expect("the fixture's loaded rule");
        RuleForm::read(&section)
    }

    fn zones() -> Vec<String> {
        vec!["lan".to_string(), "guest".to_string()]
    }

    #[test]
    fn a_rule_survives_the_round_trip_through_the_editor() {
        let read = everything();
        let written = read.values(true);
        // Reading a section and writing it straight back states the same rule:
        // every option comes back in the form firewall4 wrote it, and nothing
        // the editor owns is left behind as a null.
        for (option, want) in [
            ("name", json!("Everything")),
            ("enabled", json!("0")),
            ("src", json!("guest")),
            ("dest", json!("lan")),
            ("family", json!("ipv4")),
            ("proto", json!(["tcp", "udp"])),
            ("device", json!("br-guest")),
            ("direction", json!("out")),
            ("src_ip", json!(["10.0.20.0/24", "!10.0.20.5"])),
            ("src_mac", json!(["00:11:22:33:44:55"])),
            ("src_port", json!(["1024-65535"])),
            ("dest_ip", json!(["10.0.0.0/24"])),
            ("dest_port", json!(["53", "!5353"])),
            ("icmp_type", json!(["echo-request"])),
            ("ipset", json!("!blocked_hosts src dst")),
            ("helper", json!("!ftp")),
            ("mark", json!("!0x10/0xff")),
            ("dscp", json!("EF")),
            ("limit", json!("!1000/minute")),
            ("limit_burst", json!("5")),
            ("weekdays", json!("Mon Tue")),
            ("start_date", json!("2026-01-01")),
            ("stop_date", json!("2026-12-31")),
            ("start_time", json!("08:00:00")),
            ("stop_time", json!("18:00:00")),
            ("utc_time", json!("1")),
            ("target", json!("MARK")),
            ("set_xmark", json!("0x20/0xff")),
            ("counter", json!("0")),
            ("log", json!("guest-audit: ")),
            ("log_limit", json!("10/minute")),
        ] {
            assert_eq!(written[option], want, "{option}");
        }
        // The verdict's other parameters are not this rule's, so they are cleared.
        for option in ["set_mark", "set_helper", "set_dscp"] {
            assert_eq!(written[option], Value::Null, "{option}");
        }
        assert!(read.validate(&zones()).is_empty());
    }

    #[test]
    fn a_new_rule_writes_no_nulls_because_it_has_nothing_to_clear() {
        let written = RuleForm::default().values(false);
        let object = written.as_object().expect("values");
        assert!(object.values().all(|value| !value.is_null()), "{written}");
        assert_eq!(written["enabled"], "1");
        assert_eq!(written["counter"], "1");
        assert_eq!(written["target"], "ACCEPT");
        assert_eq!(written["proto"], json!(["tcp", "udp"]));
        // A blank rule states no path: it is written when the operator chooses one.
        assert!(object.get("src").is_none());
        assert!(object.get("name").is_none());
    }

    #[test]
    fn logging_reads_and_writes_the_one_option_that_does_two_jobs() {
        // fw4's `log` is a truth value or a prefix; a prefix means on.
        for (written, on, prefix) in [
            ("", false, ""),
            ("0", false, ""),
            ("1", true, ""),
            ("yes", true, ""),
            ("guest-audit: ", true, "guest-audit: "),
        ] {
            let log = with_option("log", json!(written), read_log);
            assert_eq!((log.on, log.prefix.as_str()), (on, prefix), "{written:?}");
        }

        let mut rule = RuleForm {
            log: Log {
                on: true,
                ..Log::default()
            },
            ..RuleForm::default()
        };
        assert_eq!(rule.values(false)["log"], "1");
        rule.log.prefix = "audit: ".into();
        assert_eq!(rule.values(false)["log"], "audit: ");
        rule.log.on = false;
        assert!(rule.values(false).get("log").is_none());
    }

    #[test]
    fn a_rate_reads_and_writes_the_way_firewall4_spells_it() {
        for (written, over, count, unit) in [
            ("1000/sec", false, "1000", "second"),
            ("!10/minute", true, "10", "minute"),
            ("25", false, "25", "second"),
            ("5/h", false, "5", "hour"),
            ("", false, "", "second"),
        ] {
            let rate = read_limit(written, "");
            assert_eq!(
                (rate.over, rate.count.as_str(), rate.unit.as_str()),
                (over, count, unit),
                "{written}"
            );
        }
        let rule = RuleForm {
            rate: RateLimit {
                over: true,
                count: "10".into(),
                unit: "minute".into(),
                burst: "3".into(),
            },
            ..RuleForm::default()
        };
        let written = rule.values(false);
        assert_eq!(written["limit"], "!10/minute");
        assert_eq!(written["limit_burst"], "3");
    }

    #[test]
    fn an_exclusion_is_the_same_option_with_each_value_inverted() {
        let tokens = Tokens {
            include: vec!["53".into(), "67".into()],
            exclude: vec!["5353".into()],
        };
        assert_eq!(tokens.option(), vec!["53", "67", "!5353"]);
        let read = with_option("dest_port", json!(["53", "!5353"]), |section| {
            Tokens::read(section, "dest_port")
        });
        assert_eq!(read.include, vec!["53"]);
        assert_eq!(read.exclude, vec!["5353"]);
    }

    #[test]
    fn a_condition_with_no_values_is_a_condition_that_is_not_there() {
        let mut rule = RuleForm::default();
        assert!(!rule.active("dest_port"));
        rule.dest_port.include.push("53".into());
        assert!(rule.active("dest_port"));
        // An exclusion alone is still a condition.
        rule.dest_port.include.clear();
        rule.dest_port.exclude.push("53".into());
        assert!(rule.active("dest_port"));
    }

    #[test]
    fn firewall4s_own_acceptance_is_what_a_value_is_checked_against() {
        for value in ["0", "53", "65535", "1024-65535", "1024:65535", "!22"] {
            assert!(valid_port(value), "{value}");
        }
        for value in ["", "65536", "99-22", "http", "1-2-3"] {
            assert!(!valid_port(value), "{value}");
        }

        for value in [
            "10.0.0.1",
            "10.0.0.0/24",
            "10.0.0.0/255.255.255.0",
            "fe80::/10",
            "::1",
            "!192.168.1.1",
        ] {
            assert!(valid_address(value), "{value}");
        }
        for value in [
            "",
            "10.0.0.256",
            "10.0.0.1/33",
            "fe80::/129",
            "not-an-address",
            "10.0.0",
        ] {
            assert!(!valid_address(value), "{value}");
        }

        for value in ["2026-01-01", "2026-01", "2026"] {
            assert!(valid_date(value), "{value}");
        }
        for value in [
            "",
            "2026-13-01",
            "2026-01-32",
            "1969",
            "2039",
            "26-01-01",
            "today",
        ] {
            assert!(!valid_date(value), "{value}");
        }

        for value in ["8", "08:00", "23:59:59"] {
            assert!(valid_time(value), "{value}");
        }
        for value in ["", "24:00", "08:60", "08:00:60", "noon"] {
            assert!(!valid_time(value), "{value}");
        }

        for value in ["0x1", "0xff", "16", "4294967295"] {
            assert!(valid_mark(value), "{value}");
        }
        for value in ["", "0x", "0xzz", "4294967296"] {
            assert!(!valid_mark(value), "{value}");
        }

        for value in [
            "00:11:22:33:44:55",
            "00-11-22-33-44-55",
            "!aa:bb:cc:dd:ee:ff",
        ] {
            assert!(valid_mac(value), "{value}");
        }
        for value in ["", "00:11:22:33:44", "gg:11:22:33:44:55"] {
            assert!(!valid_mac(value), "{value}");
        }

        for value in ["tcp", "icmpv6", "6", "255", "!esp"] {
            assert!(valid_protocol(value), "{value}");
        }
        for value in ["", "256", "tcp udp"] {
            assert!(!valid_protocol(value), "{value}");
        }

        for value in ["echo-request", "130", "3/1", "neighbour-solicitation"] {
            assert!(valid_icmp_type(value), "{value}");
        }
        for value in ["", "300", "3/300", "echo request"] {
            assert!(!valid_icmp_type(value), "{value}");
        }
    }

    #[test]
    fn a_rule_a_zone_does_not_exist_for_is_refused_before_it_is_written() {
        let rule = RuleForm {
            src: "nowhere".into(),
            ..RuleForm::default()
        };
        let errors = rule.validate(&zones());
        assert_eq!(errors.get("src"), "There is no zone called “nowhere”.");
        assert!(errors.get("dest").is_empty());
        assert!(!errors.is_empty());

        // The router itself and anywhere at all are not zones, and are accepted.
        for value in ["", "*", "guest"] {
            let rule = RuleForm {
                src: value.into(),
                ..RuleForm::default()
            };
            assert!(rule.validate(&zones()).get("src").is_empty(), "{value}");
        }
    }

    #[test]
    fn a_verdict_that_needs_a_parameter_is_refused_without_one() {
        let mut rule = RuleForm {
            target: "MARK".into(),
            ..RuleForm::default()
        };
        assert_eq!(
            rule.validate(&zones()).get("set_mark"),
            "A mark rule needs a value to set."
        );

        rule.set_mark = "0x1".into();
        assert!(rule.validate(&zones()).is_empty());

        let rule = RuleForm {
            target: "HELPER".into(),
            set_helper: "not-a-helper".into(),
            ..RuleForm::default()
        };
        assert!(!rule.validate(&zones()).get("set_helper").is_empty());
    }

    #[test]
    fn a_failing_item_is_named_by_its_position_in_the_list() {
        let rule = RuleForm {
            dest_port: Tokens {
                include: vec!["53".into(), "99-22".into(), "443".into()],
                exclude: Vec::new(),
            },
            ..RuleForm::default()
        };
        let errors = rule.validate(&zones());
        assert_eq!(
            errors.list("dest_port").keys().collect::<Vec<&String>>(),
            vec!["1"]
        );
        assert_eq!(errors.list("dest_port")["1"], PORT_HELP);
        assert!(errors.list("dest_port_not").is_empty());
    }

    #[test]
    fn a_named_set_keeps_its_dimensions_in_the_order_they_were_written() {
        let read = with_option("ipset", json!("!blocked src dest src_port"), read_ipset);
        assert!(read.set.negated);
        assert_eq!(read.set.value, "blocked");
        assert_eq!(read.fields, ["src", "dst", "src_port"]);

        let rule = RuleForm {
            ipset: read,
            ..RuleForm::default()
        };
        assert_eq!(rule.values(false)["ipset"], "!blocked src dst src_port");
    }

    /// with_option hands a mapping helper the one-option section it is about, in
    /// the shape rpcd's read gives a plugin.
    fn with_option<T>(option: &str, value: Value, read: impl FnOnce(&Section) -> T) -> T {
        let snapshot = Snapshot::from_value(json!({
            "firewall": { "test": { ".type": "rule", ".name": "test", option: value } }
        }));
        let section = snapshot.section("firewall", "test").expect("section");
        read(&section)
    }
}
