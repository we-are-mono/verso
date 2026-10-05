// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

//! The port-forward panel: one redirect, beside the listing.
//!
//! A port forward is a short thought — catch this arriving traffic, send it
//! there — but it is still three separate decisions, and it is the one firewall
//! object that deliberately opens a way in from the internet. It lives where the
//! other firewall objects do, in the panel its row opens, and a new one is the
//! same panel opened blank: making one and editing one are the same job. It is
//! one reading rather than tabs, because the parts are one thought read in
//! order — what arrives, where it goes, who else reaches it — and the whole of
//! it should be visible while it is written.
//!
//! Only the dnat direction is edited here. firewall4's redirect spec covers both
//! directions with one section type, but a snat redirect answers a different
//! question and is not what the listing shows, so an address naming one opens
//! nothing.
//!
//! A `redirect` carries nearly a rule's whole matching vocabulary — twenty-two
//! options, two of which firewall4 acts on in their absence. The shape is the
//! rule panel's rather than twenty-two rows: the ones a forward can narrow
//! itself by are an open catalogue the shell renders only where the object
//! carries them (`conditions::for_redirect`), and the few that say what happens
//! rather than what matches sit beside the thing they govern.

use verso_plugin::{uci_text, Map, RowDrawer, SectionWidget, SelectOption, Widget};

use crate::conditions;
use crate::fields;
use crate::model::{Firewall, Redirect};
use crate::page;
use crate::redirect_form::{RedirectForm, PROTOCOLS};
use crate::rule_form::Errors;

/// OPEN names the forward whose panel is open in the listing's query, and NEW
/// stands in for one that does not exist yet.
pub const OPEN: &str = "open";
pub const NEW: &str = "new";

/// The row's trash act asks before it removes a forward, in these words; the
/// shell puts the forward's name where the question says %s.
pub const DELETE_TRIGGER: &str = "Delete port forward";
pub const DELETE_QUESTION: &str = "Delete port forward “%s”?";
pub const DELETE_MESSAGE: &str = "Traffic arriving on that port stops being forwarded, and the \
device behind it is no longer reachable from outside.";

const REFLECTION_HELP: &str = "On, a device at home that asks for your public address still \
reaches this forward. Off, it has to use the local address instead — the forward then works \
only from the internet.";

/// href opens one forward's panel on the listing; new_href opens it blank.
pub fn href(section: &str) -> String {
    format!("{}?{OPEN}={section}", page::port_forwards_href())
}

pub fn new_href() -> String {
    format!("{}?{OPEN}={NEW}", page::port_forwards_href())
}

/// drawer is the panel for an existing forward (`redirect`), or a blank one for
/// a forward that does not exist yet, stated from `form` — the config's values
/// on a visit, what was typed on a refused submission.
pub fn drawer(
    model: &Firewall,
    redirect: Option<&Redirect>,
    form: &RedirectForm,
    errors: &Errors,
) -> RowDrawer {
    let (title, submit, section) = match redirect {
        Some(redirect) => (
            title(&redirect.name),
            "Save port forward",
            redirect.section.as_str(),
        ),
        None => (
            "New port forward".to_string(),
            "Add port forward",
            NEW_SECTION,
        ),
    };
    // The parts read in order under one another: the first heads the panel under
    // its title bar, and a rule sets off each of the rest.
    let fields = vec![
        identity(form, errors),
        incoming(model, form, errors).ruled(),
        destination(model, form, errors).ruled(),
        reach(model, form, errors).ruled(),
        handling(form, errors).ruled(),
        // What the panel will write, as the file spells it — the same footnote
        // the rule and zone panels carry, and the same reason: the form asks its
        // questions in plain words, and someone who knows the config reads this
        // to check the plain words said what they meant.
        Widget::config_preview(CONFIG_PATH, &uci_block(section, form)),
    ];
    RowDrawer {
        title,
        closed: page::port_forwards_href(),
        open: true,
        children: vec![fields::panel_form(submit, fields)],
        ..RowDrawer::default()
    }
}

/// title names the forward, or says plainly that it has no name: a redirect's
/// comment is optional, and one without it still has to open.
fn title(name: &str) -> String {
    match name.is_empty() {
        true => "An unnamed port forward".to_string(),
        false => name.to_string(),
    }
}

/// identity is whether the forward is live at all, then what it is called.
/// Whether it is in force comes first, as a zone's does, because nothing below
/// it means anything when it is off. The two are the forward itself, which the
/// panel's title already names, so they head the panel under no heading of
/// their own.
fn identity(redirect: &RedirectForm, errors: &Errors) -> Widget {
    Widget::Section(SectionWidget {
        flush: true,
        children: vec![fields::row_group(vec![
            Widget::switch_keyed(
                "enabled",
                "Forward is active",
                "enabled",
                ENABLED_TIP,
                redirect.enabled,
            ),
            fields::text_field(
                "name",
                "Name",
                &redirect.name,
                "Identifies the forward in listings, hit counts, and the system log.",
                errors,
            )
            .writes("name"),
        ])],
        ..Default::default()
    })
}

const ENABLED_TIP: &str = "A forward that is off stays in the config and reflects nothing — \
traffic to its port meets the zone's own policy.";

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
        "",
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
        "",
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
            .writes("dest"),
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
    let values = redirect.values(section != NEW_SECTION);
    uci_text(
        "redirect",
        section,
        values.as_object().unwrap_or(&Map::new()),
    )
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
        "",
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
        "",
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
            .writes("family"),
        ],
    )
}

const COUNTER_HELP: &str = "Keeps a packet and byte count on this forward, which is what the \
listing's hit column reads. Off saves a little work per packet and leaves it blank.";

const LOG_HELP: &str = "Writes a system-log line for every connection this forward rewrites. A \
busy forward fills the 64 KB ring in minutes.";

#[cfg(test)]
mod tests {
    use super::*;
    use crate::counters::Counters;
    use crate::fixture;
    use crate::redirects;
    use serde_json::Value;
    use verso_plugin::{Form, Snapshot};

    /// visit is the listing at an address that names a panel.
    fn visit(snapshot: &Snapshot, model: &Firewall, query: &str) -> Value {
        let envelope =
            redirects::page_open(snapshot, model, &Counters::default(), &Form::parse(query));
        serde_json::to_value(&envelope).expect("serialize")
    }

    fn open(section: &str) -> Value {
        visit(
            &fixture::snapshot(),
            &fixture::firewall(),
            &format!("{OPEN}={section}"),
        )
    }

    fn blank() -> Value {
        open(NEW)
    }

    /// submit posts the panel open on `section` the way its form posts: the
    /// fields, and the marker that says the body is the panel's own.
    fn submit(section: &str, fields: &[(&str, &str)]) -> Value {
        let mut model = fixture::firewall();
        let body = format!("{}&{}=1", encode(fields), fields::PANEL_FIELD);
        let envelope = redirects::save(
            &mut model,
            &Counters::default(),
            &Form::parse(&format!("{OPEN}={section}")),
            &Form::parse(&body),
        );
        serde_json::to_value(&envelope).expect("serialize")
    }

    fn create(fields: &[(&str, &str)]) -> Value {
        submit(NEW, fields)
    }

    /// panel is the drawer the answer holds open: a row's, or the bar's for a
    /// forward that does not exist yet.
    fn panel(body: &Value) -> Value {
        fixture::find_with(body, &|v| v.get("drawer").is_some_and(|d| d.is_object()))
            .map(|holder| holder["drawer"].clone())
            .expect("a panel is open")
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

    /// The panel opens on whether the forward is in force, a row like any
    /// other rather than a switch on the heading, then on its name, labelled
    /// as the config spells the option. The two are the forward itself, which
    /// the panel's title already names, so they stand under no heading.
    #[test]
    fn a_forward_opens_on_its_state_then_its_name() {
        let body = panel(&open("https_to_nas"));
        let first = fixture::find_with(&body, &|v| v["type"] == "section").expect("a section");
        assert!(
            first.get("control").is_none_or(Value::is_null),
            "nothing rides the heading: {first}"
        );
        assert!(
            first.get("title").is_none_or(|t| t.as_str() == Some("")),
            "the forward's own name and state carry no heading: {first}"
        );
        let rows = serde_json::to_string(&first["children"]).expect("serialize");
        let (enabled, name) = (
            rows.find(r#""name":"enabled""#).expect("enabled row"),
            rows.find(r#""name":"name""#).expect("name row"),
        );
        assert!(enabled < name, "state first, then the name: {rows}");
        let state = control(&body, "enabled");
        assert_eq!(state["label"], "Forward is active");
        assert_eq!(state["key"], "enabled");
        let named = control(&body, "name");
        assert_eq!(named["label"], "Name");
        assert_eq!(named["key"], "name");
    }

    /// A forward opens in its row's panel on the listing, not on a page of its
    /// own: the listing stays behind it, and the panel saves the forward by name.
    #[test]
    fn a_forward_opens_in_its_rows_panel() {
        let body = open("https_to_nas");
        assert_eq!(
            body["title"],
            redirects::HEADING,
            "the listing stays the page"
        );
        let row = fixture::listing(&body)["rows"]
            .as_array()
            .expect("rows")
            .iter()
            .find(|row| row["id"] == "https_to_nas")
            .expect("the forward's row")
            .clone();
        let drawer = &row["drawer"];
        assert_eq!(drawer["title"], "HTTPS-to-NAS");
        assert_eq!(drawer["open"], true);
        assert_eq!(drawer["closed"], "/plugins/firewall/port-forwards");
        let form = &drawer["children"][0];
        assert_eq!(form["type"], "form");
        assert_eq!(form["submit"], "Save port forward");
        assert!(
            fixture::find_with(form, &|v| v["name"] == fields::PANEL_FIELD).is_some(),
            "the form says it is the panel's own"
        );
        // The parts read in order: the first heads the panel, a rule sets off the
        // rest, and the preview closes it.
        let parts: Vec<Value> = form["fields"]
            .as_array()
            .expect("fields")
            .iter()
            .filter(|w| w["type"] == "section")
            .cloned()
            .collect();
        assert_eq!(parts.len(), 5);
        assert_eq!(parts[0]["flush"], true);
        assert!(parts[1..].iter().all(|part| part["hairline"] == true));
        // Its row's doors both lead here.
        assert_eq!(row["panel"], href("https_to_nas"));
        assert_eq!(row["cells"][0]["href"], href("https_to_nas"));
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

    /// Add opens the same panel blank, from the page's act, at firewall4's own
    /// defaults — what the operator starts from is what the packet filter would
    /// assume.
    #[test]
    fn a_new_forward_starts_where_firewall4s_own_defaults_are() {
        let body = blank();
        let drawer = body["act"]["drawer"].clone();
        assert_eq!(drawer["title"], "New port forward");
        assert_eq!(drawer["children"][0]["submit"], "Add port forward");
        assert_eq!(control(&drawer, "enabled")["on"], true);
        assert_eq!(control(&drawer, "proto")["value"], "tcp udp");
        assert_eq!(control(&drawer, "src")["value"], "");
        assert_eq!(
            body["act"]["href"],
            new_href(),
            "the page's Add opens this panel"
        );
    }

    #[test]
    fn creating_a_forward_states_its_direction_and_only_the_options_it_sets() {
        let body = create(&[
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
        ]);

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
        assert_eq!(control(&blank(), "reflection")["checked"], true);
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
            let body = create(&[("src", src)]);
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
        let body = create(&[("src", "wan"), ("proto", "tcp")]);
        assert!(body.get("commit").is_none());
        assert_eq!(
            control(&body, "src_dport")["error"],
            "Give the forward something to rewrite: an incoming port, a destination address, or a destination port."
        );
        for accepted in [
            ("src_dport", "8443"),
            ("dest_ip", "10.0.0.30"),
            ("dest_port", "443"),
        ] {
            let body = create(&[("src", "wan"), accepted]);
            assert!(body.get("commit").is_some(), "{accepted:?}");
        }
    }

    /// A forward is removed from its row, behind the row's own question, and the
    /// answer is the listing it is leaving.
    #[test]
    fn deleting_a_forward_answers_with_the_listing_it_is_leaving() {
        let mut model = fixture::firewall();
        let envelope = redirects::save(
            &mut model,
            &Counters::default(),
            &Form::parse(""),
            &Form::parse(&format!("{}=https_to_nas", fields::REMOVE_FIELD)),
        );
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
        let body = visit(
            &fixture::editor_snapshot(),
            &fixture::editor_firewall(),
            &format!("{OPEN}=everything_forward"),
        );

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

    /// Removal is the row's act, asked about on the row: the panel is for
    /// reading and changing a forward, and holds no delete form of its own.
    #[test]
    fn a_forward_is_deleted_from_its_row() {
        let body = open("https_to_nas");
        let row = fixture::listing(&body)["rows"]
            .as_array()
            .expect("rows")
            .iter()
            .find(|row| row["id"] == "https_to_nas")
            .expect("the forward's row")
            .clone();
        let trash = fixture::find_with(&row["cells"], &|v| v["icon"] == "trash-2")
            .expect("the row's trash act");
        assert_eq!(trash["name"], fields::REMOVE_FIELD);
        assert_eq!(trash["value"], "https_to_nas");
        assert_eq!(trash["title"], DELETE_TRIGGER);
        assert_eq!(trash["confirm_title"], DELETE_QUESTION);
        assert_eq!(trash["confirm"], DELETE_MESSAGE);
        assert!(
            fixture::find_with(&panel(&body), &|v| v["name"] == fields::REMOVE_FIELD).is_none(),
            "the panel carries no removal; that belongs to its row"
        );
    }

    /// An address naming no port forward opens nothing: a stale link lands on the
    /// listing rather than on an error about a forward that is gone. A source
    /// rewrite is a redirect section but not a port forward, and a rule is
    /// neither.
    #[test]
    fn an_address_naming_no_port_forward_opens_nothing() {
        for section in ["no_such_forward", "nas_snat", "allow_ping"] {
            let body = open(section);
            assert_eq!(body["title"], redirects::HEADING, "{section}");
            assert!(
                fixture::find_with(&body, &|v| v.get("drawer").is_some_and(|d| d.is_object()))
                    .is_none(),
                "{section} opened a panel"
            );
        }
        // And a save addressed to one writes nothing and says why.
        let body = submit("nas_snat", &[("src", "wan"), ("src_dport", "80")]);
        assert!(body.get("commit").is_none());
        assert_eq!(body["notice"]["level"], "danger");
        assert_eq!(body["title"], redirects::HEADING);
    }
}
