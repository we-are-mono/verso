// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

//! One zone, opened beside the listing it came from.
//!
//! The listing answers "which zones are there and what do they let through"; the
//! panel answers "and what exactly does this one say". Opening it never leaves the
//! page, so the other zones stay on screen while one of them is read — which
//! matters more here than on the rules listing, because a zone is only meaningful
//! against the zones it can reach.
//!
//! The panel asks a zone three questions, one per tab: what it allows, where its
//! traffic may go, and the handful of things most zones never touch. Each tab is a
//! link, so the reading is in the address and the panel survives a reload, a back
//! button and a shared URL without a line of client state.
//!
//! A zone lives in its drawer, like a rule (ADR-005 §8): a panel that fetches its
//! own body is a page in every respect that matters, and keeping the object beside
//! the list it belongs to is worth more than a page of its own. A zone is the
//! stronger case of the two, since it reads against the zones it can reach.

use verso_plugin::{DrawerTab, Property, RowDrawer, SelectOption, Value, Widget};

use crate::crossings::{self, Crossings};
use crate::fields;
use crate::format;
use crate::model::{Firewall, Zone};
use crate::page;
use crate::rule_form::{Errors, HELPERS};
use crate::zone_form::{self, ZoneForm, FAMILIES, POLICIES};

/// TRAFFIC, REACHES and ADVANCED are the three readings of a zone, as they appear
/// in the address. TRAFFIC is what a panel opens on: the first question about a
/// zone is always what it lets through.
pub const TRAFFIC: &str = "traffic";
pub const REACHES: &str = "reaches";
pub const ADVANCED: &str = "advanced";

/// OPEN and TAB are the query keys the listing reads: which zone is open, and
/// which of its readings. They address something within the page rather than
/// which page to render, which is what the query is for.
pub const OPEN: &str = "open";
pub const TAB: &str = "tab";

/// NEW is what the query names instead of a section when the panel is to open on
/// a zone that does not exist yet. Making one and editing one are the same job,
/// so they are the same panel at the same kind of address.
pub const NEW: &str = "new";

const TRAFFIC_TITLE: &str = "What this zone allows";
const TRAFFIC_SUB: &str = "These are the defaults for the zone. A rule can narrow them; nothing \
can widen them.";

const REACHES_TITLE: &str = "Where it may go";
const REACHES_SUB: &str = "Each crossing is a `config forwarding` section. Without one, traffic \
from this zone reaches no other zone at all — whatever the policies say.";

const ADVANCED_TITLE: &str = "Advanced";
const ADVANCED_SUB: &str = "Leave these unless something specific asks for them.";

/// CONFIG_PATH is the file the panel previews, named as the operator would type
/// it.
const CONFIG_PATH: &str = "/etc/config/firewall";

const NEW_NAME_HELP: &str = "Rules, forwards and crossings will point at the zone by this name.";

/// RENAME_HELP says what a new name reaches and what it cannot: everything in
/// the firewall config follows it, while an nftables include naming a chain
/// firewall4 built from the old name, or another package's settings naming the
/// zone, are outside what a save here writes.
const RENAME_HELP: &str = "Rules, forwards, crossings and source NAT that name this zone follow \
a new name. Custom nftables includes and other packages' settings keep the old one.";

const NAMED_ELSEWHERE: &str = "Named elsewhere";

const NAMED_ELSEWHERE_SUB: &str = "What else in the firewall config names this zone. A new name \
carries to each of them; deleting the zone leaves them matching nothing.";

const ENABLED_HELP: &str = "Off, firewall4 skips this zone entirely — every rule that names it \
stops applying, and the traffic it covered falls through to the global defaults. The section stays \
in the file, so nothing is lost.";

const DEVICES_HELP: &str = "Kernel devices this zone claims directly, such as tun0 — for \
something the network config does not declare as an interface.";

const SUBNET_FIELD_HELP: &str = "Address ranges this zone covers whether or not an interface \
carries them.";

const MASQ_SRC_HELP: &str = "Blank rewrites everything leaving the zone. Name a range to rewrite \
only traffic that came from it.";

const MASQ_DEST_HELP: &str = "Blank rewrites whatever the destination. Name a range to rewrite \
only traffic heading there.";

const MASQ_INVALID_HELP: &str = "Rewrites packets conntrack cannot match to a known connection. \
Off unless something specific needs it.";

const COUNTER_HELP: &str = "Keeps a packet and byte count on this zone's chains, which is what \
the listings read. Off saves a little work per packet and leaves them blank.";

const AUTO_HELPER_HELP: &str = "A helper follows protocols that open further connections of their \
own, such as FTP. Automatic assignment is convenient and widens what the zone accepts.";

const INPUT_HELP: &str = "Traffic from this zone to the router itself.";
const OUTPUT_HELP: &str = "Traffic the router itself sends into this zone.";
const FORWARD_HELP: &str = "Traffic leaving this zone for another one.";

const MASQ_LABEL: &str = "Masquerading (NAT)";
const MASQ_HELP: &str = "Rewrites traffic leaving this zone so it appears to come from the \
router. Turn it on for the zone facing your internet connection, and leave it off everywhere else.";

const MTU_LABEL: &str = "Clamp MSS to the path MTU";
const MTU_HELP: &str = "Trims outgoing segment sizes so connections over a link with a smaller \
MTU — PPPoE, a tunnel — do not stall.";

const LOG_LABEL: &str = "Log refused traffic";
const LOG_HELP: &str = "Writes a system-log line for every packet this zone rejects or drops.";

pub const DELETE_TRIGGER: &str = "Delete zone";

pub const DELETE_MESSAGE: &str = "When applied, its networks fall to the global defaults, and \
every rule, forward and forwarding that names it stops matching until it is pointed somewhere \
else.";

/// href addresses one zone's panel on the listing: the listing itself, with the
/// zone and the reading in the query. A panel opened on its default reading
/// carries no tab, so the common address stays the short one.
pub fn href(section: &str, tab: &str) -> String {
    match tab {
        TRAFFIC => format!("{}?{OPEN}={section}", page::zones_href()),
        _ => format!("{}?{OPEN}={section}&{TAB}={tab}", page::zones_href()),
    }
}

/// new_href opens the blank panel.
pub fn new_href() -> String {
    format!("{}?{OPEN}={NEW}", page::zones_href())
}

/// reading is the tab a query asks for, falling back to the one a panel opens
/// on. A tab this panel does not have is not an error: it is a stale or hand-
/// typed address, and the honest answer is the zone's first reading.
pub fn reading(asked: &str) -> &str {
    match asked {
        REACHES => REACHES,
        ADVANCED => ADVANCED,
        _ => TRAFFIC,
    }
}

/// blank builds the panel for a zone that does not exist yet: the same three
/// readings, the same rows, and a commit that adds rather than saves.
pub fn blank(
    model: &Firewall,
    form: &ZoneForm,
    reaches: &Crossings,
    errors: &Errors,
    tab: &str,
) -> RowDrawer {
    let (title, sub, mut fields) = tab_body(model, None, form, reaches, errors, tab);
    fields.extend(carried(form, reaches, tab));
    fields.push(Widget::config_preview(
        CONFIG_PATH,
        &uci_block(NEW, form, reaches, false),
    ));
    RowDrawer {
        title: "New zone".into(),
        tabs: tabs(None, form, reaches, tab),
        closed: page::zones_href(),
        open: true,
        children: vec![fields::panel_form(
            "Add zone",
            vec![Widget::section(title, sub, fields).flush()],
        )],
        ..RowDrawer::default()
    }
}

/// drawer builds one zone's panel, on the reading asked for. Its title names the
/// zone; the selected tab holds its settings and dependencies.
pub fn drawer(
    model: &Firewall,
    zone: &Zone,
    form: &ZoneForm,
    reaches: &Crossings,
    errors: &Errors,
    tab: &str,
) -> RowDrawer {
    RowDrawer {
        title: title(&zone.name),
        tabs: tabs(Some(zone), form, reaches, tab),
        closed: page::zones_href(),
        open: true,
        children: body(model, zone, form, reaches, errors, tab),
        ..RowDrawer::default()
    }
}

/// title names the zone, or says plainly that it has none. Every zone should have
/// a name — firewall4 builds its chain names from it — but a config that lost one
/// still has to render.
fn title(name: &str) -> String {
    match name.is_empty() {
        true => "An unnamed zone".to_string(),
        false => name.to_string(),
    }
}

/// tabs are the zone's three readings, each priced with where the zone stands
/// under it — so the strip answers before a tab is chosen.
fn tabs(zone: Option<&Zone>, form: &ZoneForm, reaches: &Crossings, active: &str) -> Vec<DrawerTab> {
    [
        (
            TRAFFIC,
            "Traffic",
            format!("{} in", policy_state(&form.input)),
        ),
        (REACHES, "Reaches", reaches_state(reaches.reaches.len())),
        (ADVANCED, "Advanced", advanced_state(form)),
    ]
    .into_iter()
    .map(|(key, label, state)| DrawerTab {
        label: label.into(),
        state,
        href: match zone {
            Some(zone) => href(&zone.section, key),
            None => match key {
                TRAFFIC => new_href(),
                _ => format!("{}&{TAB}={key}", new_href()),
            },
        },
        active: key == active,
    })
    .collect()
}

/// policy_state is the verdict a zone takes on arriving traffic, as the strip
/// spells it: a zone that states none follows the baseline, which is what the
/// listing's own pill says too.
///
/// Lowercased, because fw4 reads ACCEPT and accept alike and a config may hold
/// either — the strip is repeating the answer the row gave rather than quoting the
/// file, and the row's pill is lowercase.
fn policy_state(policy: &str) -> String {
    match policy.is_empty() {
        true => "default".to_string(),
        false => policy.to_lowercase(),
    }
}

fn reaches_state(count: usize) -> String {
    match count {
        0 => "nowhere".to_string(),
        1 => "1 zone".to_string(),
        n => format!("{n} zones"),
    }
}

/// advanced_state says whether anything under Advanced is in force. A zone that
/// logs its refusals or narrows its family carries live state, and the strip is
/// where a reader learns that without opening the tab.
fn advanced_state(form: &ZoneForm) -> String {
    match (form.log, form.family.is_empty()) {
        (true, _) => "logging".to_string(),
        (false, false) => form.family.clone(),
        (false, true) => "default".to_string(),
    }
}

/// body is the reading itself: its heading and lede, its controls, and — on every
/// reading — the config this panel writes and the row that saves it. The preview
/// and the commit row repeat per tab because they are about the whole zone, and a
/// tab that hid them would be a tab you could save from without seeing what you
/// saved. Save puts the zone in the stage and closes the panel; applying is the
/// bar's, and so is saying what it costs.
fn body(
    model: &Firewall,
    zone: &Zone,
    form: &ZoneForm,
    reaches: &Crossings,
    errors: &Errors,
    tab: &str,
) -> Vec<Widget> {
    let (title, sub, mut fields) = tab_body(model, Some(zone), form, reaches, errors, tab);
    fields.extend(carried(form, reaches, tab));
    fields.push(Widget::config_preview(
        CONFIG_PATH,
        &uci_block(&zone.section, form, reaches, true),
    ));
    vec![fields::panel_form(
        "Save zone",
        vec![Widget::section(title, sub, fields).flush()],
    )]
}

fn tab_body(
    model: &Firewall,
    zone: Option<&Zone>,
    form: &ZoneForm,
    reaches: &Crossings,
    errors: &Errors,
    tab: &str,
) -> (&'static str, &'static str, Vec<Widget>) {
    match tab {
        REACHES => (
            REACHES_TITLE,
            REACHES_SUB,
            reaches_fields(model, zone, reaches, errors),
        ),
        ADVANCED => (ADVANCED_TITLE, ADVANCED_SUB, advanced_fields(form, errors)),
        _ => (
            TRAFFIC_TITLE,
            TRAFFIC_SUB,
            traffic_fields(model, zone, form, errors),
        ),
    }
}

/// traffic_fields is what the zone is and what it allows: its name, the networks
/// it claims, the three verdicts firewall4 evaluates separately, and the two
/// things a zone does to traffic rather than with it.
///
/// The name's help differs by whether the zone exists: a new zone's name is one
/// things will point at, and an existing zone's is one things already point at —
/// and a new name follows to them, as far as the firewall config reaches.
fn traffic_fields(
    model: &Firewall,
    zone: Option<&Zone>,
    form: &ZoneForm,
    errors: &Errors,
) -> Vec<Widget> {
    let allowed = match zone {
        Some(zone) => allowed_networks(model, zone),
        None => model.network_names(),
    };
    let mut out = Vec::new();
    // Whether the zone is in force at all comes first, because nothing below it
    // means anything when it is off: firewall4 skips the whole section, and every
    // rule naming this zone goes with it. It is on unless a zone says otherwise,
    // which is exactly why it needs saying.
    out.push(Widget::switch_keyed(
        "enabled",
        "Zone is in force",
        "enabled",
        ENABLED_HELP,
        form.enabled,
    ));
    let name_help = match zone {
        None => NEW_NAME_HELP,
        Some(_) => RENAME_HELP,
    };
    out.push(fields::text_field("name", "Name", &form.name, name_help, errors).writes("name"));
    out.push(Widget::Field {
        name: "network".into(),
        label: "Networks".into(),
        kind: "checks".into(),
        value: String::new(),
        values: form.networks.clone(),
        placeholder: String::new(),
        datatype: String::new(),
        options: network_options(model, &allowed, &form.networks),
        error: errors.get("network").into(),
        help: String::new(),
        key: "network".into(),
        tip: String::new(),
        source: String::new(),
        unit: String::new(),
        style: String::new(),
        remove: String::new(),
        target: String::new(),
    });
    // The two other ways a zone covers something. A network is what the rest of the
    // config names; these are for what it does not — a kernel device the network
    // config never declared, and an address range that is not an interface at all.
    // The device list used to be a fact with a note saying it could not be edited
    // here, which left a zone claiming a raw device readable and unwritable.
    out.push(
        fields::token_list(
            "device",
            "Devices",
            "Device name",
            DEVICES_HELP,
            &form.devices,
            errors,
        )
        .writes("device"),
    );
    out.push(
        fields::token_list(
            "subnet",
            "Address ranges",
            "Address or network",
            SUBNET_FIELD_HELP,
            &form.subnets,
            errors,
        )
        .writes("subnet"),
    );
    let choices = || zone_form::options(&POLICIES);
    out.extend([
        policy_field(
            "input",
            "Traffic to the router",
            INPUT_HELP,
            &form.input,
            choices(),
            errors,
        ),
        policy_field(
            "output",
            "Traffic from the router",
            OUTPUT_HELP,
            &form.output,
            choices(),
            errors,
        ),
        policy_field(
            "forward",
            "Between its own networks",
            FORWARD_HELP,
            &form.forward,
            choices(),
            errors,
        ),
        // Masquerading and the three things that narrow it. They are inside the
        // switch because each is meaningless without it: a masq_src on a zone that
        // does not masquerade is a line firewall4 reads and does nothing with.
        Widget::gate(
            "masq",
            MASQ_LABEL,
            "masq",
            MASQ_HELP,
            form.masq,
            vec![
                fields::token_list(
                    "masq_src",
                    "Only rewrite traffic from",
                    "Address or network",
                    MASQ_SRC_HELP,
                    &form.masq_src,
                    errors,
                )
                .writes("masq_src"),
                fields::token_list(
                    "masq_dest",
                    "Only when it is going to",
                    "Address or network",
                    MASQ_DEST_HELP,
                    &form.masq_dest,
                    errors,
                )
                .writes("masq_dest"),
                Widget::switch_keyed(
                    "masq_allow_invalid",
                    "Rewrite packets conntrack cannot place",
                    "masq_allow_invalid",
                    MASQ_INVALID_HELP,
                    form.masq_allow_invalid,
                ),
            ],
            Vec::new(),
        ),
        Widget::switch_keyed("mtu_fix", MTU_LABEL, "mtu_fix", MTU_HELP, form.mtu_fix),
    ]);
    out
}

/// reaches_fields is where this zone's traffic may go, the zones pointing back
/// at it, and what else in the config would stop matching without it.
///
/// Only the first is a control. A crossing into this zone belongs to the zone it
/// leaves, and the rest of the blast radius is other objects entirely — this is
/// the one reading where both are worth stating, because it is where a zone's
/// place among the others is the question.
pub fn reaches_fields(
    model: &Firewall,
    zone: Option<&Zone>,
    reaches: &Crossings,
    errors: &Errors,
) -> Vec<Widget> {
    let mut out = crossings::fields(model, zone, reaches, errors);
    let Some(zone) = zone else {
        return out;
    };
    // What else names this zone. The crossings above are the answer to "where
    // may it go"; these are what goes with it if the zone is renamed or removed,
    // which is the other thing a reader comes to this tab for.
    // A titled part of its own, so the counts say what they count: the
    // objects that name this zone and would stop matching without it.
    let references = model.references(&zone.name);
    out.push(Widget::section(
        NAMED_ELSEWHERE,
        NAMED_ELSEWHERE_SUB,
        vec![Widget::properties(vec![
            count("Traffic rules", references.rules),
            count("Port forwards and redirects", references.redirects),
            count("Source NAT rules", references.nats),
        ])],
    ));
    out
}

/// advanced_fields is the block most zones never touch.
fn advanced_fields(form: &ZoneForm, errors: &Errors) -> Vec<Widget> {
    vec![
        Widget::select(
            "family",
            "Address family",
            &form.family,
            zone_form::options(&FAMILIES),
            errors.get("family"),
        )
        .writes("family"),
        // Logging and how often it may write. The limit is inside the switch because
        // a rate on a zone that logs nothing governs nothing — and a zone that logs
        // every refusal on a busy uplink with no limit fills a flash chip.
        Widget::gate(
            "log",
            LOG_LABEL,
            "log",
            LOG_HELP,
            form.log,
            vec![fields::text_field(
                "log_limit",
                "At most",
                &form.log_limit,
                zone_form::LOG_LIMIT_HELP,
                errors,
            )
            .writes("log_limit")],
            Vec::new(),
        ),
        // The two firewall4 does unless told not to, and the helper to name instead.
        Widget::switch_keyed(
            "counter",
            "Count what this zone matches",
            "counter",
            COUNTER_HELP,
            form.counter,
        ),
        Widget::gate(
            "auto_helper",
            "Assign connection helpers automatically",
            "auto_helper",
            AUTO_HELPER_HELP,
            form.auto_helper,
            Vec::new(),
            // Off is where naming one yourself becomes the question, so the choice
            // appears there rather than beside a switch that overrides it.
            vec![Widget::select(
                "helper",
                "Use this helper instead",
                &form.helper,
                helper_options(),
                errors.get("helper"),
            )
            .writes("helper")],
        ),
    ]
}

/// helper_options is firewall4's shipped helper catalogue, with the empty choice
/// that means "none of them".
fn helper_options() -> Vec<SelectOption> {
    let mut out = vec![SelectOption::new("", "None")];
    out.extend(
        HELPERS
            .iter()
            .map(|helper| SelectOption::new(helper, helper)),
    );
    out
}

/// The options each reading shows. A submission carries the reading it was on, so
/// everything outside it rides along as an inert carrier: a zone is one object and
/// Save means "save the zone", not "save the third of it I can currently see".
/// Without the carriers the options on the other two readings would arrive absent,
/// and absent is how a form says "cleared".
const TRAFFIC_OPTIONS: [&str; 13] = [
    "name",
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
    "enabled",
];
const ADVANCED_OPTIONS: [&str; 6] = [
    "family",
    "log",
    "log_limit",
    "counter",
    "auto_helper",
    "helper",
];

/// shown is the set of options the reading in force is asking about.
fn shown(tab: &str) -> &'static [&'static str] {
    match tab {
        ADVANCED => &ADVANCED_OPTIONS,
        // Reaches shows none of the zone's own options — what it edits is a
        // different section type — so every one of them rides as a carrier there.
        REACHES => &[],
        _ => &TRAFFIC_OPTIONS,
    }
}

/// carried is every value this reading does not show, as the hidden fields that
/// post it back unchanged.
fn carried(form: &ZoneForm, reaches: &Crossings, tab: &str) -> Vec<Widget> {
    let visible = shown(tab);
    let mut out = Vec::new();
    // The crossings are the one thing the Reaches reading owns, so they ride
    // along on the other two. Without them a save from Traffic would arrive
    // stating no crossings at all, and this zone would stop reaching anywhere.
    if tab != REACHES {
        out.extend(reaches.carried());
    }
    for (option, value) in carried_values(form) {
        if visible.contains(&option) {
            continue;
        }
        out.push(Widget::hidden(option, &value));
    }
    // Every list option carries one hidden field per value: a single space-joined
    // value would arrive as one entry with a space in its name.
    for (option, values) in [
        ("network", &form.networks),
        ("device", &form.devices),
        ("subnet", &form.subnets),
        ("masq_src", &form.masq_src),
        ("masq_dest", &form.masq_dest),
    ] {
        if visible.contains(&option) {
            continue;
        }
        for value in values {
            out.push(Widget::hidden(option, value));
        }
    }
    out
}

/// carried_values is every scalar option a panel posts, paired with what the zone
/// says it is now.
fn carried_values(form: &ZoneForm) -> Vec<(&'static str, String)> {
    vec![
        ("name", form.name.clone()),
        ("input", form.input.clone()),
        ("output", form.output.clone()),
        ("forward", form.forward.clone()),
        ("masq", flag(form.masq)),
        ("masq_allow_invalid", flag(form.masq_allow_invalid)),
        ("mtu_fix", flag(form.mtu_fix)),
        ("family", form.family.clone()),
        ("log", flag(form.log)),
        ("log_limit", form.log_limit.clone()),
        ("enabled", flag(form.enabled)),
        ("counter", flag(form.counter)),
        ("auto_helper", flag(form.auto_helper)),
        ("helper", form.helper.clone()),
    ]
}

/// flag is a switch as its own control posts it: present and "1" when on, absent
/// when off. A carrier for an off switch is therefore an empty value, which is the
/// same thing a form says by leaving the box unticked.
fn flag(on: bool) -> String {
    match on {
        true => "1".to_string(),
        false => String::new(),
    }
}

/// uci_block is what this panel writes, as the file spells it: the zone's own
/// section, and the crossings out of it — which are sections of their own and
/// are shown as such, because that is the fact about them worth knowing.
fn uci_block(section: &str, form: &ZoneForm, reaches: &Crossings, existing: bool) -> String {
    let mut out = format!("config zone '{section}'\n");
    let Value::Object(values) = form.values(existing) else {
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
    out.push_str(&reaches.uci_block(&form.name));
    out
}

fn scalar(value: &Value) -> String {
    match value {
        Value::String(text) => text.clone(),
        other => other.to_string(),
    }
}

/// allowed_networks is the set a zone may cover: every network this router
/// defines, plus whatever the zone already covers that it no longer does — a
/// network the config lost is still the zone's business, and a save must not drop
/// it just because the checks field could not offer it.
pub fn allowed_networks(model: &Firewall, zone: &Zone) -> Vec<String> {
    let mut allowed = model.network_names();
    for network in &zone.networks {
        if !allowed.contains(network) {
            allowed.push(network.clone());
        }
    }
    allowed
}

/// network_options is the closed set the checks field offers: every network the
/// config defines, plus anything this zone already covers that it does not — a
/// network the operator is looking at must be visible even when the network config
/// no longer defines it.
fn network_options(model: &Firewall, allowed: &[String], selected: &[String]) -> Vec<SelectOption> {
    let mut names: Vec<&String> = allowed.iter().collect();
    for network in selected {
        if !names.contains(&network) {
            names.push(network);
        }
    }
    names
        .into_iter()
        .map(|name| SelectOption::new(name, &network_label(model, name)))
        .collect()
}

/// network_label names a network and, where the config commits to one, the address
/// range that means. Deciding which networks a zone covers is the one moment the
/// range matters — two names tell a reader nothing about whether they overlap, and
/// the range tells them at once.
fn network_label(model: &Firewall, name: &str) -> String {
    let subnet = model.interface(name).and_then(|interface| {
        format::subnet_label(&interface.ipaddr, &interface.netmask, &interface.ip6assign)
    });
    match subnet {
        Some(subnet) => format!("{name} · {subnet}"),
        None => name.to_string(),
    }
}

fn policy_field(
    name: &str,
    label: &str,
    help: &str,
    value: &str,
    options: Vec<SelectOption>,
    errors: &Errors,
) -> Widget {
    Widget::Field {
        name: name.into(),
        label: label.into(),
        kind: "select".into(),
        value: value.into(),
        values: Vec::new(),
        placeholder: String::new(),
        datatype: String::new(),
        options,
        error: errors.get(name).into(),
        help: help.into(),
        key: name.into(),
        tip: String::new(),
        source: String::new(),
        unit: String::new(),
        style: String::new(),
        remove: String::new(),
        target: String::new(),
    }
}

/// count is a fact whose value is a number: data set in sans, so the shell's
/// localization leaves it exactly as counted.
fn count(label: &str, value: usize) -> Property {
    Property {
        label: label.into(),
        value: value.to_string(),
        mono: false,
        verbatim: true,
        copy: false,
        ..Property::default()
    }
}
