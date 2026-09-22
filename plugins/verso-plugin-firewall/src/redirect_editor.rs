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
//!
//! The page used to ask four questions and stop, and a `redirect` carries nearly
//! a rule's whole matching vocabulary besides — twenty-two options with nowhere
//! to go, two of which firewall4 acts on in their absence. They are here now, and
//! the shape is the rule editor's rather than twenty-two more rows: the ones a
//! forward can narrow itself by are an open catalogue the shell renders only
//! where the object carries them (`conditions::for_redirect`), and the few that
//! say what happens rather than what matches sit beside the thing they govern.

use verso_plugin::{
    commit, commit_delete, commit_new, Envelope, Form, SelectOption, Snapshot, Tone, Value, Widget,
};

use crate::conditions;
use crate::counters::Counters;
use crate::fields::{self, DELETE_FIELD};
use crate::model::{Firewall, CONFIG};
use crate::page;
use crate::redirect_form::{RedirectForm, PROTOCOLS};
use crate::redirects;
use crate::rule_form::Errors;

const NEW_SUBHEADING: &str =
    "Say which arriving traffic to catch, and where the router should send it.";

const INCOMING_SUB: &str = "The traffic the router picks up before it decides where it goes.";

const DESTINATION_SUB: &str = "Where the router sends that traffic instead. Leave the address \
blank to keep the traffic on the router itself.";

const REACH_SUB: &str = "Whether your own devices can use the public address too, instead of \
having to know the local one.";

const REFLECTION_HELP: &str = "On, a device at home that asks for your public address still \
reaches this forward. Off, it has to use the local address instead — the forward then works \
only from the internet.";

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
    let errors = redirect.validate(&model.zone_names(), &model.ipsets);
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
    let errors = redirect.validate(&model.zone_names(), &model.ipsets);
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
    let (title, submit) = match section {
        Some(_) => ("Edit port forward", "Save changes"),
        None => ("New port forward", "Add port forward"),
    };
    let mut children = vec![Widget::Form {
        style: "page".into(),
        submit: submit.into(),
        error: String::new(),
        fields: vec![
            identity(redirect, errors),
            incoming(model, redirect, errors),
            destination(model, redirect, errors),
            reach(model, redirect, errors),
            handling(redirect, errors),
            // What the page will write, as the file spells it — the same footnote
            // the rule and zone panels carry, and the same reason: the form asks
            // its questions in plain words, and someone who knows the config
            // reads this to check the plain words said what they meant.
            Widget::preview(
                CONFIG_PATH,
                &uci_block(section.unwrap_or(NEW_SECTION), redirect),
            ),
        ],
        note: String::new(),
    }];
    if section.is_some() {
        children.push(fields::delete_form(
            "Delete port forward",
            &format!(
                "Delete {}? Traffic arriving on that port stops being forwarded, and the device \
                 behind it is no longer reachable from outside.",
                fields::subject(&redirect.name, "this port forward")
            ),
        ));
    }
    Envelope::page(title, Widget::stack(children))
        .with_width("normal")
        .with_pages(page::tabs())
        .with_back("Cancel", &page::port_forwards_href())
}

/// identity is what the forward is called and whether it is live at all. The
/// switch rides the heading rather than the body: it is the state of the whole
/// object, not one of its settings.
fn identity(redirect: &RedirectForm, errors: &Errors) -> Widget {
    Widget::Section {
        title: "Port forward".into(),
        anchor: String::new(),
        kicker: false,
        sub: String::new(),
        meta: String::new(),
        meta_icon: String::new(),
        meta_position: String::new(),
        mode: String::new(),
        flush: true,
        hairline: false,
        control: Some(Box::new(Widget::Switch {
            name: "enabled".into(),
            label: "Enabled".into(),
            off_label: "Disabled".into(),
            help: String::new(),
            style: "inline".into(),
            on: redirect.enabled,
            key: String::new(),
            tip: String::new(),
            source: String::new(),
        })),
        children: vec![fields::row_group(vec![fields::text_field(
            "name",
            "Comment",
            &redirect.name,
            "Identifies the forward in listings, hit counts, and the system log.",
            errors,
        )])],
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
            fields::row_group(vec![
                fields::select_field("src", "Arrives from", &redirect.src, zones, errors),
                fields::text_field(
                    "src_dport",
                    "Incoming port",
                    &redirect.src_dport,
                    "The port the traffic is addressed to, such as 8443.",
                    errors,
                ),
            ]),
            fields::select_field("proto", "Protocol", &redirect.proto, protocols, errors),
            // Everything else a forward can be narrowed by. The catalogue is the
            // shell's: it renders the conditions this forward carries as rows and
            // keeps the rest behind its own picker. A row apiece would have been
            // twenty more rows on a page that asks a four-line question.
            conditions::for_redirect(redirect, errors, model),
        ],
    )
}

/// destination is where the caught traffic goes. A blank address keeps it on the
/// router, which is how a redirect that captures DNS is written.
///
/// The zone is the one thing here that is not an address: firewall4 works out
/// which zone the destination is in and says so in its own log, but a forward it
/// cannot place is a forward that quietly does not reflect. Naming it settles
/// that, and leaving it blank keeps firewall4's own answer.
fn destination(model: &Firewall, redirect: &RedirectForm, errors: &Errors) -> Widget {
    Widget::section(
        "Send it to",
        DESTINATION_SUB,
        vec![
            fields::row_group(vec![
                fields::text_field(
                    "dest_ip",
                    "Destination address",
                    &redirect.dest_ip,
                    "The device that answers, such as 10.0.0.30. Blank targets this router.",
                    errors,
                ),
                fields::text_field(
                    "dest_port",
                    "Destination port",
                    &redirect.dest_port,
                    "Blank keeps the incoming port.",
                    errors,
                ),
            ]),
            fields::select_field(
                "dest",
                "The device is in",
                &redirect.dest,
                zone_options(model, &redirect.dest, "Work it out from the address"),
                errors,
            )
            .writes("dest")
            .advanced_when(redirect.dest.is_empty()),
        ],
    )
}

/// CONFIG_PATH is the file this page previews, named as the operator would type
/// it.
const CONFIG_PATH: &str = "/etc/config/firewall";

/// NEW_SECTION stands in for the handle a forward does not have yet. uci picks
/// the real one when the section is added; the preview says what kind of thing
/// is being added rather than inventing a name for it.
const NEW_SECTION: &str = "new";

/// uci_block is the section this page writes, as the file spells it. An existing
/// forward states its nulls as the options a save clears, and those are left out
/// — the preview is what the file will hold, not the operations that get it
/// there.
fn uci_block(section: &str, redirect: &RedirectForm) -> String {
    let mut out = format!("config redirect '{section}'\n");
    let Value::Object(values) = redirect.values(section != NEW_SECTION) else {
        return out;
    };
    for (option, value) in &values {
        match value {
            Value::Array(items) => {
                for item in items {
                    out.push_str(&format!("\tlist {option} '{}'\n", scalar(item)));
                }
            }
            Value::Null => {}
            other => out.push_str(&format!("\toption {option} '{}'\n", scalar(other))),
        }
    }
    out
}

fn scalar(value: &Value) -> String {
    match value {
        Value::String(text) => text.clone(),
        other => other.to_string(),
    }
}

/// zone_options is a choice over this config's zones, led by what an empty value
/// means for that particular option. A zone the config no longer defines but the
/// forward still names stays on offer, so editing one never silently retargets it.
fn zone_options(model: &Firewall, current: &str, blank: &str) -> Vec<SelectOption> {
    let mut choices = vec![SelectOption::new("", blank)];
    choices.extend(conditions::named(&model.zone_names()));
    if !current.is_empty() && !model.zone_names().iter().any(|zone| zone == current) {
        choices.push(SelectOption::new(current, current));
    }
    choices
}

/// reach is whether the forward also works from inside the network, and what the
/// device sees when it does.
///
/// This is what firewall4 calls reflection and the rest of the world calls NAT
/// loopback, neither of which means anything to the person setting up a camera.
/// What they know is that typing their own public address from the sofa either
/// reaches the thing or does not — so the switch says that, and the phrase
/// firewall4 uses appears only as the option it writes.
///
/// It reads as on by default because firewall4 behaves that way, which is the whole
/// reason it is here: the behaviour existed before the control did.
fn reach(model: &Firewall, redirect: &RedirectForm, errors: &Errors) -> Widget {
    Widget::section(
        "Reaching it from home",
        REACH_SUB,
        vec![Widget::gate(
            "reflection",
            "Also works from inside your network",
            "reflection",
            REFLECTION_HELP,
            redirect.reflection,
            vec![
                fields::select_field(
                    "reflection_src",
                    "Connections appear to come from",
                    &redirect.reflection_src,
                    conditions::options(&[
                        ("internal", "This router, on your network"),
                        ("external", "This router's public address"),
                    ]),
                    errors,
                ),
                // Which inside networks get the hairpin rules. firewall4 writes
                // them for the zone the traffic is forwarded into unless this
                // names others, which is what a router with a second inside
                // network needs and what nothing here could say before.
                fields::token_list(
                    "reflection_zone",
                    "Which of your networks",
                    "Zone name",
                    WHICH_NETWORKS_HELP,
                    &redirect.reflection_zones,
                    errors,
                )
                .writes("reflection_zone")
                .suggesting(conditions::named(&model.zone_names())),
            ],
            Vec::new(),
        )],
    )
}

const WHICH_NETWORKS_HELP: &str = "Blank covers the network the device is on, which is what most \
routers need. Name zones to cover others as well.";

/// handling is what the forward does besides rewriting: whether its matches are
/// counted, and whether they reach the system log.
///
/// Counting is on unless a forward says otherwise — it is what the listing's hit
/// column reads — so the switch states firewall4's own behaviour rather than
/// leaving it to be discovered.
fn handling(redirect: &RedirectForm, errors: &Errors) -> Widget {
    Widget::section(
        "Counting and logging",
        HANDLING_SUB,
        vec![
            Widget::switch_keyed(
                "counter",
                "Count what this forward matches",
                "counter",
                COUNTER_HELP,
                redirect.counter,
            ),
            Widget::gate(
                "log",
                "Log every match",
                "log",
                LOG_HELP,
                redirect.log.on,
                vec![
                    fields::text_field(
                        "log_prefix",
                        "Log prefix",
                        &redirect.log.prefix,
                        "Blank logs under the forward's own name.",
                        errors,
                    ),
                    fields::text_field(
                        "log_limit",
                        "At most",
                        &redirect.log.limit,
                        "How often a line may be written, such as 10/minute.",
                        errors,
                    ),
                ],
                Vec::new(),
            ),
            fields::select_field(
                "family",
                "Address family",
                &redirect.family,
                conditions::options(&[
                    ("", "IPv4 and IPv6"),
                    ("ipv4", "IPv4 only"),
                    ("ipv6", "IPv6 only"),
                ]),
                errors,
            )
            .writes("family")
            .advanced_when(redirect.family.is_empty()),
        ],
    )
}

const HANDLING_SUB: &str = "What the forward does besides sending the traffic on.";

const COUNTER_HELP: &str = "Keeps a packet and byte count on this forward, which is what the \
listing's hit column reads. Off saves a little work per packet and leaves it blank.";

const LOG_HELP: &str = "Writes a system-log line for every connection this forward rewrites. A \
busy forward fills the 64 KB ring in minutes.";

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

    /// conditions is the page's catalogue, keyed the way the shell's picker
    /// keys it — which is also how a test asks what a forward can be narrowed by.
    fn condition_keys(body: &Value) -> Vec<String> {
        fixture::widget(body, "conditions")["items"]
            .as_array()
            .expect("items")
            .iter()
            .map(|item| item["key"].as_str().unwrap_or_default().to_string())
            .collect()
    }

    fn values(body: &Value) -> Value {
        body["commit"][0]["values"].clone()
    }

    #[test]
    fn the_editor_is_a_page_form_that_stages_and_returns() {
        let body = open("https_to_nas");
        assert_eq!(body["title"], "Edit port forward");
        assert_eq!(body["subheading"], "HTTPS-to-NAS");
        assert_eq!(body["width"], "normal");
        assert_eq!(body["pages"][1]["label"], "Port forwards");
        let form = &body["widget"]["children"][0];
        assert_eq!(form["type"], "form");
        assert_eq!(form["style"], "page");
        assert_eq!(
            form["submit"], "Save changes",
            "the editor carries its own submit"
        );
        assert_eq!(body["back"]["href"], "/plugins/firewall/port-forwards");
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
    /// it means, so a stock redirect opens on the choice it already made. What
    /// the two spellings mean is the form's business and is checked there.
    #[test]
    fn the_two_protocol_spellings_read_as_one_choice() {
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
                // The blank form offers these on, as firewall4 behaves, so the
                // browser posts them.
                ("reflection", "1"),
                ("counter", "1"),
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
                "dest_port": "25565",
                "reflection": "1",
                // firewall4 counts what a redirect matches unless told
                // otherwise, and the screen says so now, so the file does too.
                "counter": "1"
            }),
            "a new forward states what it sets and nothing else"
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
                "name": null,
                "dest": null,
                "family": null,
                // A submission that does not say "also from inside" means off, and
                // off is written rather than left to firewall4 — whose own default
                // is on, which is the whole reason this option needed a control.
                "reflection": "0",
                "reflection_src": null,
                "reflection_zone": null,
                // Counting is the same kind of option and answers the same way.
                "counter": "0",
                "log": null,
                "log_limit": null,
                // And every condition the catalogue can add is one the editor can
                // take away again: a submission that carries none clears them all,
                // which is what "this forward no longer says that" has to write.
                "src_ip": null,
                "src_dip": null,
                "src_mac": null,
                "src_port": null,
                "ipset": null,
                "helper": null,
                "mark": null,
                "limit": null,
                "limit_burst": null,
                "weekdays": null,
                "start_date": null,
                "stop_date": null,
                "start_time": null,
                "stop_time": null,
                "utc_time": null
            })
        );
        // The direction is not this editor's to rewrite on an existing section.
        assert!(values(&body).get("target").is_none());
    }

    /// A forward reaches from inside by default because firewall4 does, and the
    /// screen says so instead of letting it happen quietly. This is the option the
    /// reckoning found: every forward Verso had ever written was reflecting, and no
    /// page mentioned it.
    #[test]
    fn a_forward_states_whether_it_reaches_from_inside() {
        let body = open("https_to_nas");
        let gate = control(&body, "reflection");
        assert_eq!(gate["type"], "conditional");
        assert_eq!(gate["key"], "reflection");
        assert_eq!(
            gate["checked"], true,
            "firewall4 reflects unless told otherwise, so the control reads that way"
        );
        // And the choice it governs is inside it, because it means nothing with
        // reflection off.
        let source = &gate["fields"][0];
        assert_eq!(source["name"], "reflection_src");
        assert_eq!(source["value"], "internal");

        // The blank form agrees: a new forward starts reachable from inside.
        let blank = serde_json::to_value(blank(&fixture::firewall())).expect("serialize");
        assert_eq!(control(&blank, "reflection")["checked"], true);
    }

    /// The source only rides along while reflection is on: firewall4 ignores it
    /// otherwise, and writing it would leave a value in the file that does nothing.
    #[test]
    fn the_reflection_source_is_written_only_while_reflection_is_on() {
        let on = submit(
            "https_to_nas",
            &[
                ("src", "wan"),
                ("src_dport", "8443"),
                ("reflection", "1"),
                ("reflection_src", "external"),
            ],
        );
        assert_eq!(values(&on)["reflection"], "1");
        assert_eq!(values(&on)["reflection_src"], "external");

        let off = submit(
            "https_to_nas",
            &[
                ("src", "wan"),
                ("src_dport", "8443"),
                ("reflection_src", "external"),
            ],
        );
        assert_eq!(values(&off)["reflection"], "0");
        assert!(
            values(&off)["reflection_src"].is_null(),
            "a source with nothing to apply to is cleared, not kept"
        );

        // firewall4's own default is not written out: the file stays quiet about a
        // value it would assume anyway.
        let plain = submit(
            "https_to_nas",
            &[
                ("src", "wan"),
                ("src_dport", "8443"),
                ("reflection", "1"),
                ("reflection_src", "internal"),
            ],
        );
        assert!(values(&plain)["reflection_src"].is_null());
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

        assert_eq!(body["title"], crate::redirects::HEADING);
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
        let rows = fixture::listing(&body)["rows"]
            .as_array()
            .expect("rows")
            .clone();
        assert!(rows.iter().all(|row| row["id"] != "https_to_nas"));
    }

    /// The page offers every match a redirect can carry and nothing firewall4
    /// would ignore on one. The catalogue is what makes that checkable: a
    /// condition is an entry rather than a row somebody has to find room for.
    #[test]
    fn the_page_offers_the_matches_a_redirect_can_carry() {
        let keys = condition_keys(&open("https_to_nas"));
        assert_eq!(
            keys,
            vec![
                "src_ip", "src_dip", "src_port", "src_mac", "ipset", "mark", "helper", "rate",
                "schedule"
            ]
        );
        // The rule's own conditions that firewall4 does not read on a redirect
        // are not offered: a control that changes nothing is worse than one that
        // is missing.
        for absent in ["device", "dscp", "icmp_type", "dest_port", "dest_ip"] {
            assert!(!keys.contains(&absent.to_string()), "{absent}");
        }
    }

    /// A forward the config narrows opens with those conditions already on the
    /// page rather than behind the picker, which is what "active" means to the
    /// shell and what the operator needs to see without hunting.
    #[test]
    fn a_forward_that_carries_conditions_opens_with_them_showing() {
        let snapshot = fixture::editor_snapshot();
        let model = fixture::editor_firewall();
        let body = serde_json::to_value(
            edit(&snapshot, &model, "everything_forward").expect("the fixture's forward"),
        )
        .expect("serialize");

        let items = fixture::widget(&body, "conditions")["items"]
            .as_array()
            .expect("items")
            .clone();
        let active: Vec<&str> = items
            .iter()
            .filter(|item| item["active"] == true)
            .map(|item| item["key"].as_str().unwrap_or_default())
            .collect();
        assert_eq!(
            active,
            vec![
                "src_ip", "src_dip", "src_port", "src_mac", "ipset", "mark", "helper", "rate",
                "schedule"
            ],
            "every condition this forward states is on the page"
        );
        // And the values are in the controls that wrote them, including the
        // comparison a negated scalar needs.
        assert_eq!(control(&body, "src_dip")["value"], "198.51.100.7");
        assert_eq!(control(&body, "src_dip_match")["value"], "exclude");
        assert_eq!(control(&body, "helper")["value"], "ftp");
        assert_eq!(
            control(&body, "reflection_zone")["items"],
            serde_json::json!(["lan"])
        );
        // A switch that is off carries no `on` at all, which is how the schema
        // spells false — so this forward's counting reads as off, as its config
        // says.
        assert_ne!(control(&body, "counter")["on"], true);
        assert_eq!(control(&body, "counter")["type"], "switch");
        assert_eq!(control(&body, "log_limit")["value"], "10/minute");
        assert_eq!(control(&body, "dest")["value"], "lan");
    }

    /// firewall4 refuses a leading `!` on a redirect's helper and skips the whole
    /// section for it, so the control has no comparison beside it — unlike the
    /// rule's, which does.
    #[test]
    fn the_helper_condition_offers_no_comparison_on_a_forward() {
        let body = open("https_to_nas");
        let helper = fixture::widget(&body, "conditions")["items"]
            .as_array()
            .expect("items")
            .iter()
            .find(|item| item["key"] == "helper")
            .expect("the helper condition")
            .clone();
        assert!(
            fixture::find_with(&helper, &|value| value["name"] == "helper_match").is_none(),
            "a negated helper is a forward firewall4 drops"
        );
    }

    /// A condition added on the page is written as the option firewall4 reads,
    /// in the spelling its own parser expects.
    #[test]
    fn a_condition_the_page_adds_is_written_as_firewall4_reads_it() {
        let body = submit(
            "https_to_nas",
            &[
                ("src", "wan"),
                ("src_dport", "8443"),
                ("src_ip", "203.0.113.0/24"),
                ("src_ip_match", "exclude"),
                ("limit_match", "over"),
                ("limit", "100"),
                ("limit_unit", "minute"),
                ("weekdays", "Sat"),
                ("time_basis", "utc"),
                ("counter", "1"),
                ("log", "1"),
                ("log_limit", "10/minute"),
            ],
        );
        assert_eq!(body["notice"]["level"], "success");
        let values = values(&body);
        assert_eq!(values["src_ip"], "!203.0.113.0/24");
        assert_eq!(values["limit"], "!100/minute");
        assert_eq!(values["weekdays"], "Sat", "one value, never a uci list");
        assert_eq!(values["utc_time"], "1");
        assert_eq!(values["counter"], "1");
        assert_eq!(values["log"], "1");
        assert_eq!(values["log_limit"], "10/minute");
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
        assert_eq!(body["title"], crate::redirects::HEADING);
    }
}
