// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

//! One port forward as its editor holds it, between the config and the form.
//!
//! The editor owns a fixed set of firewall4's redirect options — [`OWNED`] — and
//! nothing else. That set is the contract in both directions: reading a section
//! fills this struct from those options, saving writes them back, and every one
//! of them the submission no longer carries is cleared rather than left behind.
//!
//! It used to own nine, and a redirect carries nearly a rule's whole matching
//! vocabulary besides — which meant a forward could be restricted to one source
//! address, one weekday, or one packet mark, and the page that edited it would
//! neither show that nor let anyone write it. Those are conditions now, out of
//! the same catalogue the rule editor offers, because they are the same options
//! with the same grammars: [`crate::rule_form`] reads and checks them, and this
//! file only says which of them a redirect can carry.
//!
//! Two of firewall4's readings differ from the rule's and both matter:
//!
//! - `src_ip`, `src_dip` and `src_port` are **scalars** here where a rule takes
//!   lists. fw4 refuses a list on a scalar option and skips the whole section, so
//!   each is one value, negatable, rather than an include/exclude pair.
//! - `helper` **must not be negated** on a redirect. fw4 refuses the section
//!   outright, so the control is a plain choice with no comparison beside it.
//!
//! Only the dnat direction is written. `target` and the snat-only reading of
//! `src_dip` are the reckoning's business, not a control's; see the omissions
//! beside it.

use verso_plugin::{json, Form, Map, Section, Value};

use crate::model;
use crate::rule_form::{
    bit, cleaned, family_value, join_mask, rate_unit, read_ipset, read_limit, read_log, split_mask,
    split_words, valid_address, valid_count, valid_date, valid_limit, valid_mac, valid_mark,
    valid_port, valid_protocol, valid_time, Errors, Inverted, Log, MarkMatch, RateLimit, Schedule,
    SetMatch, Tokens, ADDRESS_HELP, HELPERS, MAC_HELP, MARK_HELP, PORT_HELP, PROTOCOL_HELP,
    WEEKDAYS,
};

/// OWNED is every option this editor writes. A save states all of them: the ones
/// the submission carries as values, the rest as nulls that clear them. A
/// redirect may still carry options this editor does not draw — `_name`, the
/// unsupported `extra` and `monthdays` — and those are never touched.
///
/// The options written only while the thing above them is on — a log's rate, a
/// reflection's source and zones — belong here as much as the ones with a
/// control of their own. Left out, an option could be written once and never
/// taken away again.
pub const OWNED: [&str; 30] = [
    "name",
    "enabled",
    "src",
    "dest",
    "family",
    "proto",
    "src_ip",
    "src_dip",
    "src_mac",
    "src_port",
    "src_dport",
    "dest_ip",
    "dest_port",
    "ipset",
    "helper",
    "mark",
    "limit",
    "limit_burst",
    "weekdays",
    "start_date",
    "stop_date",
    "start_time",
    "stop_time",
    "utc_time",
    "counter",
    "log",
    "log_limit",
    "reflection",
    "reflection_src",
    "reflection_zone",
];

/// PROTOCOLS are the choices a port forward is written with. firewall4 accepts
/// any protocol here, but a forward of anything but TCP or UDP has no ports to
/// rewrite, which is the whole point of the form.
pub const PROTOCOLS: [(&str, &str); 3] = [("tcp udp", "tcp/udp"), ("tcp", "tcp"), ("udp", "udp")];

/// DEFAULT_PROTOCOL is firewall4's own `tcpudp`, spelled the way this editor
/// offers it.
pub const DEFAULT_PROTOCOL: &str = "tcp udp";

/// DEFAULT_REFLECTION_SRC is firewall4's own default for which of the router's
/// addresses a reflected connection appears to come from.
pub const DEFAULT_REFLECTION_SRC: &str = "internal";

/// RedirectForm is one port forward as the editor holds it: every option it
/// owns, in the shape its controls take rather than the shape uci writes.
#[derive(Clone)]
pub struct RedirectForm {
    pub name: String,
    pub enabled: bool,
    pub src: String,
    /// The zone the rewritten traffic arrives in. firewall4 works it out from the
    /// destination address when the section states none — and says so in its own
    /// log — but it is the zone a forward's reflection rules are generated for,
    /// so a forward that reflects and cannot be placed silently does not.
    pub dest: String,
    pub family: String,
    pub src_dport: String,
    pub proto: String,
    pub dest_ip: String,
    pub dest_port: String,
    /// Whether the forward also works from inside the network. firewall4 turns this
    /// on unless told otherwise, so a forward Verso wrote was always reflecting and
    /// no screen said so.
    pub reflection: bool,
    /// Which of this router's addresses a reflected connection appears to come
    /// from. Only meaningful while reflection is on.
    pub reflection_src: String,
    /// Which zones get the reflection rules. firewall4 generates them for the
    /// zone the traffic is forwarded into unless this names others, which is what
    /// a second inside network needs.
    pub reflection_zones: Vec<String>,
    /// firewall4 counts what a redirect matches unless told otherwise, which is
    /// what the listing's hit column reads.
    pub counter: bool,
    pub log: Log,
    // ---- the optional matches, in the order the catalogue offers them ----
    pub src_ip: Inverted,
    pub src_dip: Inverted,
    pub src_port: Inverted,
    pub src_mac: Tokens,
    pub ipset: SetMatch,
    /// Not an [`Inverted`]: fw4 marks this NO_INVERT on a redirect and refuses
    /// the whole section for a leading `!`.
    pub helper: String,
    pub mark: MarkMatch,
    pub rate: RateLimit,
    pub schedule: Schedule,
}

impl Default for RedirectForm {
    /// A blank forward starts where firewall4's own defaults are: on, over TCP
    /// and UDP, counted, reachable from inside as well as out.
    fn default() -> RedirectForm {
        RedirectForm {
            name: String::new(),
            enabled: true,
            src: String::new(),
            dest: String::new(),
            family: String::new(),
            src_dport: String::new(),
            proto: DEFAULT_PROTOCOL.into(),
            dest_ip: String::new(),
            dest_port: String::new(),
            reflection: true,
            reflection_src: DEFAULT_REFLECTION_SRC.into(),
            reflection_zones: Vec::new(),
            counter: true,
            log: Log::default(),
            src_ip: Inverted::default(),
            src_dip: Inverted::default(),
            src_port: Inverted::default(),
            src_mac: Tokens::default(),
            ipset: SetMatch::default(),
            helper: String::new(),
            mark: MarkMatch::default(),
            rate: RateLimit::default(),
            schedule: Schedule::default(),
        }
    }
}

impl RedirectForm {
    /// read fills the form from a redirect section, with firewall4's defaults
    /// where an option is absent.
    pub fn read(section: &Section) -> RedirectForm {
        let mark = Inverted::read(section, "mark");
        let (mark_value, mark_mask) = split_mask(&mark.value);
        RedirectForm {
            name: section.scalar("name"),
            enabled: model::flag(section, "enabled", true),
            src: section.scalar("src"),
            dest: section.scalar("dest"),
            family: family_value(&section.scalar("family")),
            src_dport: section.scalar("src_dport"),
            proto: protocol_value(&model::values(section, "proto")),
            dest_ip: section.scalar("dest_ip"),
            dest_port: section.scalar("dest_port"),
            // firewall4's own defaults, stated rather than assumed: a forward with
            // no reflection option reflects, and one with no counter is counted.
            reflection: model::flag(section, "reflection", true),
            reflection_src: match section.scalar("reflection_src") {
                value if value.is_empty() => DEFAULT_REFLECTION_SRC.to_string(),
                value => value,
            },
            reflection_zones: model::values(section, "reflection_zone"),
            counter: model::flag(section, "counter", true),
            log: read_log(section),
            src_ip: Inverted::read(section, "src_ip"),
            src_dip: Inverted::read(section, "src_dip"),
            src_port: Inverted::read(section, "src_port"),
            src_mac: Tokens::read(section, "src_mac"),
            ipset: read_ipset(section),
            helper: section.scalar("helper"),
            mark: MarkMatch {
                mark: Inverted {
                    negated: mark.negated,
                    value: mark_value,
                },
                mask: mark_mask,
            },
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
    /// controls with it, so an absent field is the operator saying that condition
    /// is gone.
    pub fn submitted(form: &Form) -> RedirectForm {
        RedirectForm {
            name: form.get("name").trim().to_string(),
            enabled: !form.get("enabled").is_empty(),
            src: form.get("src").trim().to_string(),
            dest: form.get("dest").trim().to_string(),
            family: family_value(&form.get("family")),
            src_dport: form.get("src_dport").trim().to_string(),
            proto: form.get("proto").trim().to_string(),
            dest_ip: form.get("dest_ip").trim().to_string(),
            dest_port: form.get("dest_port").trim().to_string(),
            reflection: !form.get("reflection").is_empty(),
            reflection_src: match form.get("reflection_src").trim() {
                "" => DEFAULT_REFLECTION_SRC.to_string(),
                value => value.to_string(),
            },
            reflection_zones: cleaned(form.all("reflection_zone")),
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
            src_ip: Inverted::from_form(form, "src_ip", "src_ip_match"),
            src_dip: Inverted::from_form(form, "src_dip", "src_dip_match"),
            src_port: Inverted::from_form(form, "src_port", "src_port_match"),
            src_mac: Tokens::from_form(form, "src_mac"),
            ipset: SetMatch {
                set: Inverted::from_form(form, "ipset", "ipset_match"),
                fields: [
                    form.get("ipset_field_1"),
                    form.get("ipset_field_2"),
                    form.get("ipset_field_3"),
                ],
            },
            helper: form.get("helper").trim().to_string(),
            mark: MarkMatch {
                mark: Inverted::from_form(form, "mark_value", "mark_match"),
                mask: form.get("mark_mask").trim().to_string(),
            },
            rate: RateLimit {
                over: form.get("limit_match") == "over",
                count: form.get("limit").trim().to_string(),
                unit: rate_unit(&form.get("limit_unit")),
                burst: form.get("limit_burst").trim().to_string(),
            },
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

    /// active reports whether a condition is part of the forward — the same
    /// question for the editor's picker and for the save: a condition states
    /// something or it is not there.
    pub fn active(&self, key: &str) -> bool {
        match key {
            "src_ip" => !self.src_ip.value.is_empty(),
            "src_dip" => !self.src_dip.value.is_empty(),
            "src_port" => !self.src_port.value.is_empty(),
            "src_mac" => self.src_mac.is_set(),
            "ipset" => !self.ipset.set.value.is_empty(),
            "helper" => !self.helper.is_empty(),
            "mark" => !self.mark.mark.value.is_empty(),
            "rate" => !self.rate.count.is_empty(),
            "schedule" => self.schedule.is_set(),
            _ => false,
        }
    }

    /// protocols is the select's value as the list firewall4 reads.
    pub fn protocols(&self) -> Vec<String> {
        split_words(vec![self.proto.clone()])
    }

    /// values is what the save writes: every option the forward states, plus —
    /// when editing an existing section — a null for each owned option it no
    /// longer states. A new section states its direction as well, because that
    /// is what makes the section a port forward rather than a source rewrite;
    /// an existing one already has a direction this editor does not offer to
    /// change.
    pub fn values(&self, existing: bool) -> Value {
        let mut values = Map::new();
        let mut set = |option: &str, value: Value| {
            values.insert(option.to_string(), value);
        };

        // The two firewall4 acts on in the absence of any value. Off has to be
        // written: clearing either would restore the behaviour just turned off.
        set("enabled", json!(bit(self.enabled)));
        set("counter", json!(bit(self.counter)));
        if !existing {
            set("target", json!("dnat"));
        }
        for (option, value) in [
            ("name", &self.name),
            ("src", &self.src),
            ("dest", &self.dest),
            ("family", &self.family),
            ("src_dport", &self.src_dport),
            ("dest_ip", &self.dest_ip),
            ("dest_port", &self.dest_port),
        ] {
            if !value.is_empty() {
                set(option, json!(value.clone()));
            }
        }
        let protocols = self.protocols();
        if !protocols.is_empty() {
            set("proto", json!(protocols));
        }
        self.reflection_values(&mut set);
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

    /// reflection_values writes whether the forward answers from inside the
    /// network, and the two settings that only mean anything while it does.
    ///
    /// Reflection is written either way rather than left to firewall4's default,
    /// because the screen now states it: a forward that says "reachable from
    /// inside too" and writes nothing would be relying on an assumption a later
    /// firmware is free to change.
    fn reflection_values(&self, set: &mut impl FnMut(&str, Value)) {
        set("reflection", json!(bit(self.reflection)));
        if !self.reflection {
            return;
        }
        if self.reflection_src != DEFAULT_REFLECTION_SRC {
            set("reflection_src", json!(self.reflection_src.clone()));
        }
        if !self.reflection_zones.is_empty() {
            set("reflection_zone", json!(self.reflection_zones.clone()));
        }
    }

    fn condition_values(&self, set: &mut impl FnMut(&str, Value)) {
        // The three fw4 reads as single values on a redirect where a rule may
        // hold lists. One of these written as a uci list is a section fw4 skips.
        for (key, option, value) in [
            ("src_ip", "src_ip", &self.src_ip),
            ("src_dip", "src_dip", &self.src_dip),
            ("src_port", "src_port", &self.src_port),
        ] {
            if self.active(key) {
                set(option, json!(value.option(&value.value)));
            }
        }
        if self.active("src_mac") {
            set("src_mac", json!(self.src_mac.option()));
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
            set("helper", json!(self.helper.clone()));
        }
        if self.active("mark") {
            let body = join_mask(&self.mark.mark.value, &self.mark.mask);
            set("mark", json!(self.mark.mark.option(&body)));
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
                // One value, not a uci list — see the same write on a rule.
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

    /// log_values writes firewall4's `log`, which is one option doing two jobs: a
    /// truth value turns logging on under the forward's own name, any other
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
    /// this redirect — before the write, because one fw4 refuses is dropped from
    /// the ruleset with nothing said to the operator.
    pub fn validate(&self, zones: &[String], sets: &[String]) -> Errors {
        let mut errors = Errors::default();
        self.validate_path(zones, &mut errors);
        self.validate_conditions(sets, &mut errors);
        self.validate_schedule(&mut errors);
        if self.log.on && !self.log.limit.is_empty() && !valid_limit(&self.log.limit) {
            errors.field("log_limit", "Write a rate like 10/minute.");
        }
        errors
    }

    fn validate_path(&self, zones: &[String], errors: &mut Errors) {
        match self.src.as_str() {
            "" | "*" => errors.field("src", "Choose the zone this traffic arrives on."),
            zone if !zones.iter().any(|known| known == zone) => {
                errors.field("src", &format!("There is no zone called “{zone}”."));
            }
            _ => {}
        }
        if !self.dest.is_empty()
            && self.dest != "*"
            && !zones.iter().any(|known| known == &self.dest)
        {
            errors.field("dest", &format!("There is no zone called “{}”.", self.dest));
        }
        for zone in &self.reflection_zones {
            if zone != "*" && !zones.iter().any(|known| known == zone) {
                errors.field(
                    "reflection_zone",
                    "Name zones this router defines, or leave it blank.",
                );
            }
        }
        for (field, value) in [
            ("src_dport", &self.src_dport),
            ("dest_port", &self.dest_port),
        ] {
            if !value.is_empty() && !valid_port(value) {
                errors.field(field, PORT_HELP);
            }
        }
        if !self.dest_ip.is_empty() && !valid_address(&self.dest_ip) {
            errors.field("dest_ip", ADDRESS_HELP);
        }
        if self.protocols().iter().any(|proto| !valid_protocol(proto)) {
            errors.field("proto", PROTOCOL_HELP);
        }
        // A dnat redirect with no port and no destination renders a bare `dnat`
        // statement nft refuses to parse — fw4 itself accepts the section, and
        // the whole ruleset then fails to load. At least one of the three gives
        // the rewrite something to say.
        if self.src_dport.is_empty() && self.dest_ip.is_empty() && self.dest_port.is_empty() {
            errors.field(
                "src_dport",
                "Give the forward something to rewrite: an incoming port, a destination address, or a destination port.",
            );
        }
    }

    fn validate_conditions(&self, sets: &[String], errors: &mut Errors) {
        for (key, field, value) in [
            ("src_ip", "src_ip", &self.src_ip),
            ("src_dip", "src_dip", &self.src_dip),
        ] {
            if self.active(key) && !valid_address(&value.value) {
                errors.field(field, ADDRESS_HELP);
            }
        }
        if self.active("src_port") && !valid_port(&self.src_port.value) {
            errors.field("src_port", PORT_HELP);
        }
        if self.active("src_mac") {
            self.src_mac.check("src_mac", errors, valid_mac, MAC_HELP);
        }
        if self.active("ipset") && !sets.iter().any(|set| set == &self.ipset.set.value) {
            errors.field("ipset", "Choose one of the sets this config declares.");
        }
        if self.active("helper") && !HELPERS.contains(&self.helper.as_str()) {
            errors.field("helper", "Choose one of the offered connection helpers.");
        }
        if self.active("mark") {
            if !valid_mark(&self.mark.mark.value) {
                errors.field("mark_value", MARK_HELP);
            }
            if !self.mark.mask.is_empty() && !valid_mark(&self.mark.mask) {
                errors.field("mark_mask", MARK_HELP);
            }
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

/// protocol_value normalizes what a config wrote onto the choices the editor
/// offers: firewall4 spells "both" as one token, and the editor spells it as the
/// two protocols it means. An empty option is fw4's own tcpudp default.
pub fn protocol_value(values: &[String]) -> String {
    let mut tokens: Vec<String> = Vec::new();
    for value in values {
        match value.as_str() {
            "tcpudp" => tokens.extend(["tcp".to_string(), "udp".to_string()]),
            other => tokens.push(other.to_string()),
        }
    }
    if tokens.is_empty() {
        return DEFAULT_PROTOCOL.to_string();
    }
    tokens.join(" ")
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::fixture;

    fn read(section: &str) -> RedirectForm {
        let snapshot = fixture::snapshot();
        RedirectForm::read(&snapshot.section("firewall", section).expect("the section"))
    }

    /// firewall4 writes "both protocols" as one token; the editor offers the two
    /// it means, so a stock redirect opens on the choice it already made.
    #[test]
    fn the_two_protocol_spellings_read_as_one_choice() {
        assert_eq!(protocol_value(&[]), "tcp udp");
        assert_eq!(protocol_value(&["tcpudp".into()]), "tcp udp");
        assert_eq!(protocol_value(&["tcp".into(), "udp".into()]), "tcp udp");
        assert_eq!(protocol_value(&["icmp".into()]), "icmp");
    }

    /// The three firewall4 reads as one value on a redirect are written as one
    /// value. A uci list on any of them is a section fw4 skips outright, taking
    /// the forward out of the ruleset with nothing said to the operator.
    #[test]
    fn the_scalar_matches_are_never_written_as_lists() {
        let mut form = RedirectForm {
            src: "wan".into(),
            src_dport: "8443".into(),
            src_ip: Inverted {
                negated: false,
                value: "203.0.113.7".into(),
            },
            src_dip: Inverted {
                negated: true,
                value: "198.51.100.1".into(),
            },
            src_port: Inverted {
                negated: false,
                value: "1024-65535".into(),
            },
            ..RedirectForm::default()
        };
        let values = form.values(false);
        assert_eq!(values["src_ip"], "203.0.113.7");
        assert_eq!(values["src_dip"], "!198.51.100.1", "fw4 negates with a !");
        assert_eq!(values["src_port"], "1024-65535");

        // And the one that IS a list on a redirect still is.
        form.src_mac = Tokens {
            include: vec!["00:11:22:33:44:55".into()],
            exclude: vec!["66:77:88:99:aa:bb".into()],
        };
        assert_eq!(
            form.values(false)["src_mac"],
            json!(["00:11:22:33:44:55", "!66:77:88:99:aa:bb"])
        );
    }

    /// firewall4 marks `helper` NO_INVERT on a redirect and refuses the whole
    /// section for a leading `!`, so the form holds a name and not a comparison.
    #[test]
    fn the_helper_is_written_without_a_comparison() {
        let form = RedirectForm {
            src: "wan".into(),
            src_dport: "21".into(),
            helper: "ftp".into(),
            ..RedirectForm::default()
        };
        assert_eq!(form.values(false)["helper"], "ftp");
        assert!(!form.values(false)["helper"]
            .as_str()
            .expect("a string")
            .starts_with('!'));
    }

    /// The two firewall4 acts on in the absence of any value are written either
    /// way. Clearing one restores the behaviour the operator just turned off,
    /// which is the mistake a default-on option invites.
    #[test]
    fn the_options_firewall4_assumes_are_always_written() {
        let off = RedirectForm {
            src: "wan".into(),
            src_dport: "8443".into(),
            enabled: false,
            counter: false,
            reflection: false,
            ..RedirectForm::default()
        };
        let values = off.values(false);
        for option in ["enabled", "counter", "reflection"] {
            assert_eq!(values[option], "0", "{option}: off is written, not cleared");
        }
        // And on is written too, so the file states what the screen says rather
        // than relying on a default a later firmware is free to change.
        let on = RedirectForm {
            src: "wan".into(),
            src_dport: "8443".into(),
            ..RedirectForm::default()
        };
        let values = on.values(false);
        for option in ["enabled", "counter", "reflection"] {
            assert_eq!(values[option], "1", "{option}");
        }
    }

    /// firewall4's own defaults are what a forward that states nothing reads as:
    /// on, counted, and reflecting. Each of the three is something the config
    /// does that no screen had to mention until it did.
    #[test]
    fn a_forward_that_states_nothing_reads_as_what_firewall4_assumes() {
        let form = read("https_to_nas");
        assert_eq!(form.src, "wan");
        assert_eq!(form.src_dport, "8443");
        assert_eq!(form.dest_ip, "10.0.0.30");
        assert!(form.enabled);
        assert!(form.counter, "firewall4 counts unless told otherwise");
        assert!(form.reflection, "and reflects unless told otherwise");
    }

    /// A forward carrying the whole of firewall4's redirect vocabulary comes back
    /// stating the whole of it. No operator writes this forward; a round trip has
    /// to survive it anyway, because every option in it is one somebody's config
    /// may hold and this editor now owns.
    #[test]
    fn a_forward_carrying_the_whole_vocabulary_survives_the_round_trip() {
        let snapshot = fixture::editor_snapshot();
        let section = snapshot
            .section("firewall", "everything_forward")
            .expect("the fixture's forward");
        let form = RedirectForm::read(&section);

        assert_eq!(form.name, "Everything forward");
        assert!(!form.enabled);
        assert_eq!(form.src, "guest");
        assert_eq!(form.dest, "lan");
        assert_eq!(form.family, "ipv4");
        assert_eq!(form.proto, "tcp");
        assert_eq!(form.src_dport, "8443");
        assert_eq!(form.dest_ip, "10.0.0.30");
        assert_eq!(form.dest_port, "443");
        // The three firewall4 reads as one value apiece, one of them negated.
        assert_eq!(form.src_ip.value, "10.0.20.0/24");
        assert!(!form.src_ip.negated);
        assert_eq!(form.src_dip.value, "198.51.100.7");
        assert!(form.src_dip.negated);
        assert_eq!(form.src_port.value, "1024-65535");
        // And the one that is a list, split into what matches and what does not.
        assert_eq!(form.src_mac.include, vec!["00:11:22:33:44:55".to_string()]);
        assert_eq!(form.src_mac.exclude, vec!["66:77:88:99:aa:bb".to_string()]);
        assert_eq!(form.ipset.set.value, "blocked_hosts");
        assert!(form.ipset.set.negated);
        assert_eq!(form.ipset.fields[0], "src");
        assert_eq!(form.helper, "ftp");
        assert_eq!(form.mark.mark.value, "0x10");
        assert_eq!(form.mark.mask, "0xff");
        assert!(form.mark.mark.negated);
        assert!(form.rate.over);
        assert_eq!(form.rate.count, "1000");
        assert_eq!(form.rate.unit, "minute");
        assert_eq!(form.rate.burst, "5");
        assert_eq!(
            form.schedule.weekdays,
            vec!["Mon".to_string(), "Tue".to_string()]
        );
        assert_eq!(form.schedule.start_time, "08:00:00");
        assert!(form.schedule.utc);
        assert!(form.reflection);
        assert_eq!(form.reflection_src, "external");
        assert_eq!(form.reflection_zones, vec!["lan".to_string()]);
        assert!(!form.counter);
        assert!(form.log.on);
        assert_eq!(form.log.prefix, "forward-audit: ");
        assert_eq!(form.log.limit, "10/minute");

        // And writing it back states every one of them as the file spelled it.
        let values = form.values(true);
        for (option, written) in [
            ("name", json!("Everything forward")),
            ("enabled", json!("0")),
            ("src", json!("guest")),
            ("dest", json!("lan")),
            ("family", json!("ipv4")),
            ("proto", json!(["tcp"])),
            ("src_ip", json!("10.0.20.0/24")),
            ("src_dip", json!("!198.51.100.7")),
            (
                "src_mac",
                json!(["00:11:22:33:44:55", "!66:77:88:99:aa:bb"]),
            ),
            ("src_port", json!("1024-65535")),
            ("src_dport", json!("8443")),
            ("dest_ip", json!("10.0.0.30")),
            ("dest_port", json!("443")),
            ("ipset", json!("!blocked_hosts src dst")),
            ("helper", json!("ftp")),
            ("mark", json!("!0x10/0xff")),
            ("limit", json!("!1000/minute")),
            ("limit_burst", json!("5")),
            ("weekdays", json!("Mon Tue")),
            ("start_date", json!("2026-01-01")),
            ("stop_date", json!("2026-12-31")),
            ("start_time", json!("08:00:00")),
            ("stop_time", json!("18:00:00")),
            ("utc_time", json!("1")),
            ("reflection", json!("1")),
            ("reflection_src", json!("external")),
            ("reflection_zone", json!(["lan"])),
            ("counter", json!("0")),
            ("log", json!("forward-audit: ")),
            ("log_limit", json!("10/minute")),
        ] {
            assert_eq!(values[option], written, "{option}");
        }
        // What no control draws is not this editor's to discard: the direction an
        // existing section already has, the hand-written `extra`, and an option
        // firewall4 parses and ignores all stay exactly where they were found.
        for untouched in ["target", "extra", "monthdays"] {
            assert!(
                values.get(untouched).is_none(),
                "{untouched} is not this editor's to rewrite"
            );
        }
    }

    /// A value firewall4 would refuse is refused first, because a section it
    /// refuses is dropped from the ruleset with nothing said to the operator.
    #[test]
    fn a_condition_firewall4_would_refuse_is_marked() {
        let zones = vec!["wan".to_string(), "lan".to_string()];
        let sets = vec!["blocklist".to_string()];
        let form = RedirectForm {
            src: "wan".into(),
            src_dport: "8443".into(),
            src_ip: Inverted {
                negated: false,
                value: "10.0.0.300".into(),
            },
            src_port: Inverted {
                negated: false,
                value: "howdy".into(),
            },
            helper: "nosuchhelper".into(),
            dest: "nowhere".into(),
            ..RedirectForm::default()
        };
        let errors = form.validate(&zones, &sets);
        assert_eq!(errors.get("src_ip"), ADDRESS_HELP);
        assert_eq!(errors.get("src_port"), PORT_HELP);
        assert_eq!(
            errors.get("helper"),
            "Choose one of the offered connection helpers."
        );
        assert_eq!(errors.get("dest"), "There is no zone called “nowhere”.");

        // And a forward that states none of them passes.
        let plain = RedirectForm {
            src: "wan".into(),
            src_dport: "8443".into(),
            ..RedirectForm::default()
        };
        assert!(plain.validate(&zones, &sets).is_empty());
    }
}
