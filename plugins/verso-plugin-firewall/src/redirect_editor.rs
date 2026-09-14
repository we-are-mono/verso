// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

//! The port-forward editor: one redirect, on a page of its own.
//!
//! A port forward is a short thought — catch this arriving traffic, send it
//! there — but it is still three separate decisions, and it is the one firewall
//! object that deliberately opens a way in from the internet. It gets the same
//! page treatment as a rule so the whole of it is visible while it is written.
//!
//! The form is the page's, not its own: the shell's staged-changes bar submits it
//! and applies the result, so the editor answers a submission the way it answers
//! a visit — with itself, re-rendered from what was submitted. Deleting is a
//! second, small form below it, behind a confirmation.
//!
//! Only the dnat direction is edited here. firewall4's redirect spec covers both
//! directions with one section type, but a snat redirect answers a different
//! question and is not what this listing shows, so a sub-path naming one is a
//! sub-path naming nothing.

use verso_plugin::{
    commit, commit_delete, commit_new, json, Envelope, Form, Map, Section, SelectOption, Snapshot,
    Tone, Value, Widget,
};

use crate::counters::Counters;
use crate::editor::{self, DELETE_FIELD};
use crate::model::{self, Firewall, CONFIG};
use crate::page;
use crate::redirects;
use crate::rule_form::{
    valid_address, valid_port, valid_protocol, Errors, ADDRESS_HELP, PORT_HELP, PROTOCOL_HELP,
};

/// OWNED is every option this editor writes. A save states all of them: the ones
/// the submission carries as values, the rest as nulls that clear them. A
/// redirect may also carry options this editor does not draw — firewall4's spec
/// is far wider than a port forward needs — and those are never touched.
const OWNED: [&str; 7] = [
    "name",
    "enabled",
    "src",
    "proto",
    "src_dport",
    "dest_ip",
    "dest_port",
];

/// PROTOCOLS are the choices a port forward is written with. firewall4 accepts
/// any protocol here, but a forward of anything but TCP or UDP has no ports to
/// rewrite, which is the whole point of the form.
const PROTOCOLS: [(&str, &str); 3] = [("tcp udp", "tcp/udp"), ("tcp", "tcp"), ("udp", "udp")];

/// DEFAULT_PROTOCOL is firewall4's own `tcpudp`, spelled the way this editor
/// offers it.
const DEFAULT_PROTOCOL: &str = "tcp udp";

const NEW_SUBHEADING: &str =
    "Say which arriving traffic to catch, and where the router should send it.";

const INCOMING_SUB: &str = "The traffic the router picks up before it decides where it goes.";

const DESTINATION_SUB: &str = "Where the router sends that traffic instead. Leave the address \
blank to keep the traffic on the router itself.";

const REFUSED: &str =
    "Some values aren’t ones the firewall accepts, so nothing was saved. They’re marked below.";

/// edit answers a visit to one port forward's editor, or nothing when the
/// sub-path names no forward this config holds.
pub fn edit(snapshot: &Snapshot, model: &Firewall, section: &str) -> Option<Envelope> {
    let redirect = model
        .redirects
        .iter()
        .find(|redirect| redirect.section == section && redirect.is_port_forward())?;
    let uci = snapshot.section(CONFIG, section)?;
    let form = RedirectForm::read(&uci);
    Some(
        page(model, Some(section), &form, &Errors::default())
            .with_subheading(&heading(&redirect.name)),
    )
}

/// blank answers a visit to the new-forward editor: firewall4's own defaults, so
/// what the operator starts from is what the packet filter would assume.
pub fn blank(model: &Firewall) -> Envelope {
    page(model, None, &RedirectForm::default(), &Errors::default()).with_subheading(NEW_SUBHEADING)
}

/// create answers the new-forward editor's submission.
pub fn create(model: &Firewall, form: &Form) -> Envelope {
    let redirect = RedirectForm::submitted(form);
    let errors = redirect.validate(&model.zone_names());
    let answer = page(model, None, &redirect, &errors).with_subheading(NEW_SUBHEADING);
    if !errors.is_empty() {
        return answer.with_notice(Tone::Danger, REFUSED);
    }
    answer
        .with_notice(Tone::Success, "Port forward added.")
        .with_commit(vec![commit_new(CONFIG, "redirect", redirect.values(false))])
}

/// save answers one port forward's editor. A body carrying the delete marker is
/// the second form below the editor, and answers with the listing the forward is
/// leaving; anything else is the editor's own submission.
pub fn save(
    model: &mut Firewall,
    counters: &Counters,
    section: &str,
    form: &Form,
) -> Option<Envelope> {
    let index = model
        .redirects
        .iter()
        .position(|redirect| redirect.section == section && redirect.is_port_forward())?;
    if form.get(DELETE_FIELD) == "1" {
        let removed = model.redirects.remove(index);
        return Some(
            redirects::page(model, counters)
                .with_notice(Tone::Success, "Port forward deleted.")
                .with_commit(vec![commit_delete(CONFIG, &removed.section)]),
        );
    }

    let redirect = RedirectForm::submitted(form);
    let errors = redirect.validate(&model.zone_names());
    let answer =
        page(model, Some(section), &redirect, &errors).with_subheading(&heading(&redirect.name));
    if !errors.is_empty() {
        return Some(answer.with_notice(Tone::Danger, REFUSED));
    }
    Some(
        answer
            .with_notice(Tone::Success, "Port forward saved.")
            .with_commit(vec![commit(CONFIG, section, redirect.values(true))]),
    )
}

/// missing states that the sub-path names no port forward — a stale link, or one
/// someone else removed — and answers with the listing, which is somewhere real.
pub fn missing(model: &Firewall, counters: &Counters) -> Envelope {
    redirects::page(model, counters).with_notice(
        Tone::Danger,
        "That port forward isn’t here any more, so Verso showed you the forwards instead.",
    )
}

/// heading names the forward the page is about, falling back to its own words
/// when the section carries no comment.
fn heading(name: &str) -> String {
    match name.is_empty() {
        true => "An unnamed port forward.".to_string(),
        false => name.to_string(),
    }
}

/// page composes the editor. `section` is the forward being edited, or None for
/// a new one — which is also what decides whether the delete form is there.
fn page(
    model: &Firewall,
    section: Option<&str>,
    redirect: &RedirectForm,
    errors: &Errors,
) -> Envelope {
    let title = match section {
        Some(_) => "Edit port forward",
        None => "New port forward",
    };
    let mut children = vec![Widget::Form {
        note: Default::default(),

        style: "page".into(),
        submit: String::new(),
        error: String::new(),
        fields: vec![
            identity(redirect, errors),
            incoming(model, redirect, errors),
            destination(redirect, errors),
        ],
    }];
    if section.is_some() {
        children.push(editor::delete_form(
            "Delete port forward",
            &format!(
                "Delete {}? Traffic arriving on that port stops being forwarded, and the device \
                 behind it is no longer reachable from outside.",
                editor::subject(&redirect.name, "this port forward")
            ),
        ));
    }
    Envelope::page(title, Widget::stack(children))
        .with_width("normal")
        .with_pages(page::tabs())
}

/// identity is what the forward is called and whether it is live at all. The
/// switch rides the heading rather than the body: it is the state of the whole
/// object, not one of its settings.
fn identity(redirect: &RedirectForm, errors: &Errors) -> Widget {
    Widget::Section {
        anchor: Default::default(),
        kicker: Default::default(),
        hairline: Default::default(),

        title: "Port forward".into(),
        sub: String::new(),
        meta: String::new(),
        meta_icon: String::new(),
        meta_position: String::new(),
        mode: String::new(),
        flush: true,
        control: Some(Box::new(Widget::Switch {
            key: Default::default(),
            tip: Default::default(),
            source: Default::default(),

            name: "enabled".into(),
            label: "Enabled".into(),
            off_label: "Disabled".into(),
            help: String::new(),
            style: "inline".into(),
            icon: String::new(),
            meta: String::new(),
            on: redirect.enabled,
        })),
        children: vec![editor::form_grid(
            2,
            vec![editor::text_field(
                "name",
                "Comment",
                &redirect.name,
                "Identifies the forward in listings, hit counts, and the system log.",
                errors,
            )],
        )],
    }
}

/// incoming is the traffic the forward catches. The zone is a closed choice over
/// the zones this config defines: firewall4 skips a port forward whose source is
/// missing or "anywhere", so neither is on offer.
fn incoming(model: &Firewall, redirect: &RedirectForm, errors: &Errors) -> Widget {
    let mut zones = crate::conditions::named(&model.zone_names());
    if !redirect.src.is_empty() && !model.zone_names().contains(&redirect.src) {
        zones.push(SelectOption::new(&redirect.src, &redirect.src));
    }
    let mut protocols = crate::conditions::options(&PROTOCOLS);
    if !PROTOCOLS.iter().any(|(value, _)| *value == redirect.proto) {
        protocols.push(SelectOption::new(&redirect.proto, &redirect.proto));
    }
    Widget::section(
        "Incoming traffic",
        INCOMING_SUB,
        vec![
            editor::form_grid(
                2,
                vec![
                    editor::select_field("src", "Arrives from", &redirect.src, zones, errors),
                    editor::text_field(
                        "src_dport",
                        "Incoming port",
                        &redirect.src_dport,
                        "The port the traffic is addressed to, such as 8443.",
                        errors,
                    ),
                ],
            ),
            editor::select_field("proto", "Protocol", &redirect.proto, protocols, errors),
        ],
    )
}

/// destination is where the caught traffic goes. A blank address keeps it on the
/// router, which is how a redirect that captures DNS is written.
fn destination(redirect: &RedirectForm, errors: &Errors) -> Widget {
    Widget::section(
        "Send it to",
        DESTINATION_SUB,
        vec![editor::form_grid(
            2,
            vec![
                editor::text_field(
                    "dest_ip",
                    "Destination address",
                    &redirect.dest_ip,
                    "The device that answers, such as 10.0.0.30. Blank targets this router.",
                    errors,
                ),
                editor::text_field(
                    "dest_port",
                    "Destination port",
                    &redirect.dest_port,
                    "Blank keeps the incoming port.",
                    errors,
                ),
            ],
        )],
    )
}

/// RedirectForm is one port forward as the editor holds it: every option it
/// owns, in the shape its controls take rather than the shape uci writes.
#[derive(Clone)]
struct RedirectForm {
    name: String,
    enabled: bool,
    src: String,
    src_dport: String,
    proto: String,
    dest_ip: String,
    dest_port: String,
}

impl Default for RedirectForm {
    /// A blank forward starts where firewall4's own defaults are: on, over TCP
    /// and UDP.
    fn default() -> RedirectForm {
        RedirectForm {
            name: String::new(),
            enabled: true,
            src: String::new(),
            src_dport: String::new(),
            proto: DEFAULT_PROTOCOL.into(),
            dest_ip: String::new(),
            dest_port: String::new(),
        }
    }
}

impl RedirectForm {
    /// read fills the form from a redirect section, with firewall4's defaults
    /// where an option is absent.
    fn read(section: &Section) -> RedirectForm {
        RedirectForm {
            name: section.scalar("name"),
            enabled: model::flag(section, "enabled", true),
            src: section.scalar("src"),
            src_dport: section.scalar("src_dport"),
            proto: protocol_value(&model::values(section, "proto")),
            dest_ip: section.scalar("dest_ip"),
            dest_port: section.scalar("dest_port"),
        }
    }

    /// submitted reads the form the browser posted.
    fn submitted(form: &Form) -> RedirectForm {
        RedirectForm {
            name: form.get("name").trim().to_string(),
            enabled: !form.get("enabled").is_empty(),
            src: form.get("src").trim().to_string(),
            src_dport: form.get("src_dport").trim().to_string(),
            proto: form.get("proto").trim().to_string(),
            dest_ip: form.get("dest_ip").trim().to_string(),
            dest_port: form.get("dest_port").trim().to_string(),
        }
    }

    /// protocols is the select's value as the list firewall4 reads.
    fn protocols(&self) -> Vec<String> {
        self.proto.split_whitespace().map(String::from).collect()
    }

    /// values is what the save writes: every option the forward states, plus —
    /// when editing an existing section — a null for each owned option it no
    /// longer states. A new section states its direction as well, because that
    /// is what makes the section a port forward rather than a source rewrite;
    /// an existing one already has a direction this editor does not offer to
    /// change.
    fn values(&self, existing: bool) -> Value {
        let mut values = Map::new();
        let mut set = |option: &str, value: Value| {
            values.insert(option.to_string(), value);
        };

        set("enabled", json!(if self.enabled { "1" } else { "0" }));
        if !existing {
            set("target", json!("dnat"));
        }
        for (option, value) in [
            ("name", &self.name),
            ("src", &self.src),
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
    /// this redirect — before the write, because one fw4 refuses is dropped from
    /// the ruleset with nothing said to the operator.
    fn validate(&self, zones: &[String]) -> Errors {
        let mut errors = Errors::default();
        match self.src.as_str() {
            "" | "*" => errors.field("src", "Choose the zone this traffic arrives on."),
            zone if !zones.iter().any(|known| known == zone) => {
                errors.field("src", &format!("There is no zone called “{zone}”."));
            }
            _ => {}
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
        errors
    }
}

/// protocol_value normalizes what a config wrote onto the choices the editor
/// offers: firewall4 spells "both" as one token, and the editor spells it as the
/// two protocols it means. An empty option is fw4's own tcpudp default.
fn protocol_value(values: &[String]) -> String {
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
    use serde_json::Value;

    fn open(section: &str) -> Value {
        let snapshot = fixture::snapshot();
        let model = fixture::firewall();
        let envelope = edit(&snapshot, &model, section).expect("the fixture holds this forward");
        serde_json::to_value(&envelope).expect("serialize")
    }

    fn submit(section: &str, fields: &[(&str, &str)]) -> Value {
        let mut model = fixture::firewall();
        let envelope = save(
            &mut model,
            &Counters::default(),
            section,
            &Form::parse(&encode(fields)),
        )
        .expect("the fixture holds this forward");
        serde_json::to_value(&envelope).expect("serialize")
    }

    fn encode(fields: &[(&str, &str)]) -> String {
        fields
            .iter()
            .map(|(name, value)| format!("{name}={}", value.replace(' ', "+")))
            .collect::<Vec<String>>()
            .join("&")
    }

    fn control(body: &Value, name: &str) -> Value {
        fixture::control(body, name)
    }

    fn values(body: &Value) -> Value {
        body["commit"][0]["values"].clone()
    }

    #[test]
    fn the_editor_is_a_page_form_the_capsule_saves() {
        let body = open("https_to_nas");
        assert_eq!(body["title"], "Edit port forward");
        assert_eq!(body["subheading"], "HTTPS-to-NAS");
        assert_eq!(body["width"], "normal");
        assert_eq!(body["pages"][1]["label"], "Port forwards");
        let form = &body["widget"]["children"][0];
        assert_eq!(form["type"], "form");
        assert_eq!(form["style"], "page");
        assert!(
            form.get("submit").is_none(),
            "the staged-changes bar owns saving, so the form declares no button"
        );
    }

    #[test]
    fn an_existing_forward_comes_back_in_the_controls_that_wrote_it() {
        let body = open("https_to_nas");
        assert_eq!(control(&body, "name")["value"], "HTTPS-to-NAS");
        assert_eq!(control(&body, "enabled")["on"], true);
        assert_eq!(control(&body, "src")["value"], "wan");
        assert_eq!(control(&body, "src_dport")["value"], "8443");
        assert_eq!(control(&body, "proto")["value"], "tcp");
        assert_eq!(control(&body, "dest_ip")["value"], "10.0.0.30");
        assert_eq!(control(&body, "dest_port")["value"], "443");
    }

    /// firewall4 writes "both protocols" as one token; the editor offers the two
    /// it means, so a stock redirect opens on the choice it already made.
    #[test]
    fn the_two_protocol_spellings_read_as_one_choice() {
        assert_eq!(protocol_value(&[]), "tcp udp");
        assert_eq!(protocol_value(&["tcpudp".into()]), "tcp udp");
        assert_eq!(protocol_value(&["tcp".into(), "udp".into()]), "tcp udp");
        assert_eq!(protocol_value(&["icmp".into()]), "icmp");

        let body = open("force_dns_guest");
        assert_eq!(control(&body, "proto")["value"], "tcp udp");
        assert_eq!(
            control(&body, "proto")["options"],
            serde_json::json!([
                {"value": "tcp udp", "label": "tcp/udp"},
                {"value": "tcp", "label": "tcp"},
                {"value": "udp", "label": "udp"}
            ])
        );
    }

    /// A port forward has to name the zone the traffic arrives on, so the zone
    /// choice is the config's own zones and nothing else.
    #[test]
    fn the_source_is_a_closed_choice_over_this_configs_zones() {
        let body = open("https_to_nas");
        assert_eq!(
            control(&body, "src")["options"],
            serde_json::json!([
                {"value": "lan", "label": "lan"},
                {"value": "wan", "label": "wan"},
                {"value": "guest", "label": "guest"},
                {"value": "tailscale", "label": "tailscale"}
            ])
        );
    }

    #[test]
    fn a_new_forward_starts_where_firewall4s_own_defaults_are() {
        let body = serde_json::to_value(blank(&fixture::firewall())).expect("serialize");
        assert_eq!(body["title"], "New port forward");
        assert_eq!(body["subheading"], NEW_SUBHEADING);
        assert_eq!(control(&body, "enabled")["on"], true);
        assert_eq!(control(&body, "proto")["value"], "tcp udp");
        assert_eq!(control(&body, "src")["value"], "");
        assert!(
            body["widget"]["children"]
                .as_array()
                .expect("children")
                .len()
                == 1,
            "a forward that does not exist yet cannot be deleted"
        );
    }

    #[test]
    fn creating_a_forward_states_its_direction_and_only_the_options_it_sets() {
        let model = fixture::firewall();
        let body = serde_json::to_value(create(
            &model,
            &Form::parse(&encode(&[
                ("name", "Minecraft"),
                ("enabled", "1"),
                ("src", "wan"),
                ("src_dport", "25565"),
                ("proto", "tcp"),
                ("dest_ip", "10.0.0.44"),
                ("dest_port", "25565"),
            ])),
        ))
        .expect("serialize");

        assert_eq!(body["notice"]["level"], "success");
        assert_eq!(body["commit"][0]["config"], "firewall");
        assert_eq!(body["commit"][0]["section"], "");
        assert_eq!(body["commit"][0]["type"], "redirect");
        assert_eq!(
            values(&body),
            serde_json::json!({
                "enabled": "1",
                "target": "dnat",
                "name": "Minecraft",
                "src": "wan",
                "src_dport": "25565",
                "proto": ["tcp"],
                "dest_ip": "10.0.0.44",
                "dest_port": "25565"
            })
        );
    }

    #[test]
    fn saving_a_forward_clears_every_option_it_owns_that_the_submission_dropped() {
        let body = submit(
            "https_to_nas",
            &[
                ("enabled", "1"),
                ("src", "wan"),
                ("src_dport", "8443"),
                ("proto", "tcp udp"),
            ],
        );
        assert_eq!(body["notice"]["level"], "success");
        assert_eq!(body["commit"][0]["section"], "https_to_nas");
        assert_eq!(
            values(&body),
            serde_json::json!({
                "enabled": "1",
                "src": "wan",
                "src_dport": "8443",
                "proto": ["tcp", "udp"],
                // The forward now targets the router itself, so the address and
                // the rewritten port are cleared rather than written empty.
                "dest_ip": null,
                "dest_port": null,
                "name": null
            })
        );
        // The direction is not this editor's to rewrite on an existing section.
        assert!(values(&body).get("target").is_none());
    }

    #[test]
    fn a_switch_the_submission_left_out_turns_its_option_off() {
        let body = submit("https_to_nas", &[("src", "wan"), ("src_dport", "8443")]);
        assert_eq!(values(&body)["enabled"], "0");
    }

    #[test]
    fn a_value_firewall4_would_refuse_is_marked_and_nothing_is_written() {
        let cases = [
            (
                "a port range that runs backwards",
                ("src_dport", "99-22"),
                "src_dport",
            ),
            (
                "a destination port that is not one",
                ("dest_port", "howdy"),
                "dest_port",
            ),
            (
                "an address that is not one",
                ("dest_ip", "10.0.0.300"),
                "dest_ip",
            ),
            (
                "a zone this config does not define",
                ("src", "nowhere"),
                "src",
            ),
        ];
        for (case, field, control_name) in cases {
            // The offending value leads, so a case that overrides one of the
            // forward's own fields is the value the form reads.
            let body = submit("https_to_nas", &[field, ("src", "wan")]);
            assert!(
                body.get("commit").is_none(),
                "{case}: nothing may be written"
            );
            assert_eq!(body["notice"]["level"], "danger", "{case}");
            let marked = control(&body, control_name);
            assert!(
                marked.get("error").is_some(),
                "{case}: {control_name} carries no error: {marked}"
            );
        }
    }

    /// firewall4 skips a port forward with no source zone, and refuses one whose
    /// source is "anywhere" — so the editor refuses both before the write.
    #[test]
    fn a_forward_that_names_no_arrival_zone_is_refused() {
        for src in ["", "*"] {
            let model = fixture::firewall();
            let body = serde_json::to_value(create(&model, &Form::parse(&encode(&[("src", src)]))))
                .expect("serialize");
            assert!(body.get("commit").is_none(), "{src:?}");
            assert_eq!(
                control(&body, "src")["error"],
                "Choose the zone this traffic arrives on.",
                "{src:?}"
            );
        }
    }

    // A dnat section carrying only a source zone makes fw4 render `dnat` with no
    // address and no port — a statement nft's parser rejects, taking the whole
    // ruleset load down with it. fw4 accepts the section, so the editor is the
    // last honest gate.
    #[test]
    fn a_forward_that_rewrites_nothing_is_refused() {
        let model = fixture::firewall();
        let body = serde_json::to_value(create(&model, &Form::parse("src=wan&proto=tcp")))
            .expect("serialize");
        assert!(body.get("commit").is_none());
        assert_eq!(
            control(&body, "src_dport")["error"],
            "Give the forward something to rewrite: an incoming port, a destination address, or a destination port."
        );
        for accepted in [
            "src=wan&src_dport=8443",
            "src=wan&dest_ip=10.0.0.30",
            "src=wan&dest_port=443",
        ] {
            let body =
                serde_json::to_value(create(&model, &Form::parse(accepted))).expect("serialize");
            assert!(body.get("commit").is_some(), "{accepted}");
        }
    }

    #[test]
    fn deleting_a_forward_answers_with_the_listing_it_is_leaving() {
        let mut model = fixture::firewall();
        let envelope = save(
            &mut model,
            &Counters::default(),
            "https_to_nas",
            &Form::parse("_delete=1"),
        )
        .expect("the fixture holds this forward");
        let body = serde_json::to_value(&envelope).expect("serialize");

        assert_eq!(body["title"], "Firewall");
        assert_eq!(
            body["notice"],
            serde_json::json!({"level": "success", "text": "Port forward deleted."})
        );
        assert_eq!(
            body["commit"],
            serde_json::json!([{
                "config": "firewall",
                "section": "https_to_nas",
                "delete": true
            }])
        );
        let rows = fixture::table(&body, "Port forwards and redirects")["rows"]
            .as_array()
            .expect("rows")
            .clone();
        assert!(rows.iter().all(|row| row["id"] != "https_to_nas"));
    }

    #[test]
    fn only_an_existing_forward_offers_to_delete_itself() {
        let body = open("https_to_nas");
        let delete = &body["widget"]["children"][1];
        assert_eq!(delete["type"], "form");
        assert_eq!(delete["fields"][0]["name"], DELETE_FIELD);
        assert_eq!(delete["fields"][1]["type"], "confirm");
        assert_eq!(delete["fields"][1]["trigger"], "Delete port forward");
    }

    #[test]
    fn a_sub_path_naming_no_port_forward_answers_with_the_listing() {
        let snapshot = fixture::snapshot();
        let model = fixture::firewall();
        assert!(edit(&snapshot, &model, "no_such_forward").is_none());
        // A source rewrite is a redirect section, but it is not a port forward.
        assert!(edit(&snapshot, &model, "nas_snat").is_none());
        // Neither is a rule.
        assert!(edit(&snapshot, &model, "allow_ping").is_none());

        let mut model = fixture::firewall();
        assert!(save(
            &mut model,
            &Counters::default(),
            "nas_snat",
            &Form::parse("src=wan")
        )
        .is_none());

        let body = serde_json::to_value(missing(&model, &Counters::default())).expect("serialize");
        assert_eq!(body["notice"]["level"], "danger");
        assert_eq!(body["title"], "Firewall");
    }
}
