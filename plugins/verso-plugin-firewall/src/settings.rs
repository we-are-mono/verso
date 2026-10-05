// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

//! Firewall settings: the baseline, and the knobs that are about the firewall
//! rather than about any rule in it.
//!
//! Everything here decides traffic no zone and no rule claimed, or changes how
//! the whole ruleset behaves. That is why it is a page of its own rather than a
//! block at the foot of the zones listing: a zone is a thing you have, and these
//! are the conditions every zone is evaluated under.
//!
//! Each row states the option's own uci name beside its label. The prose says
//! what the setting does for someone meeting it for the first time; the chip
//! says which line of `/etc/config/firewall` it is, for someone who already
//! knows the config and wants the line.

use verso_plugin::{
    commit, commit_new, json, CommitOp, Envelope, Field, Form, Grid, Map, SelectOption, Switch,
    Tone, Value, Widget,
};

use verso_plugin::files::FileSet;

use crate::model;
use crate::model::{Defaults, Firewall, Include};
use crate::page;

const HEADING: &str = "Firewall settings";

const ZONE_TITLE: &str = "When no zone applies";

const PROTECTION_TITLE: &str = "Protection";

const SPEED_TITLE: &str = "Speed";

const TCP_TITLE: &str = "TCP and helpers";

const CUSTOM_TITLE: &str = "Custom rules";
const CUSTOM_SUB: &str = "Every file in /etc/nftables.d/ loads with the ruleset and is checked \
with it when you apply.";

const INCLUDES_LABEL: &str = "Files the config includes";

const PACKAGE_FILES_LABEL: &str = "Load rule files that packages install";
const PACKAGE_FILES_HELP: &str = "Packages put them in /usr/share/nftables.d/.";

const INCOMPATIBLE_NOTE: &str = "This script is enabled but is not marked as compatible with fw4. \
Check its contents before setting `fw4_compatible` to `1` in `/etc/config/firewall`.";

/// RULE_FILES is fw4's own folder of rule files, listed and edited exactly as
/// DNS & DHCP lists and edits dnsmasq's option files.
pub const RULE_FILES: FileSet = FileSet {
    page: "/plugins/firewall/settings",
    editors: "/plugins/firewall/settings/files/",
    dir: "/etc/nftables.d/",
    suffix: ".nft",
    main: None,
    line: ("line", "lines"),
    empty: "No rule files",
    too_large: "Rule files must be at most 32 KiB of text.",
    placeholder: "10-custom",
};

/// POLICIES is the verdict a policy row offers, in the order the canvas states
/// them: what a zone lets through, what it answers, what it swallows.
const POLICIES: [&str; 3] = ["accept", "reject", "drop"];

/// ECN is the three settings firewall4 writes for `tcp_ecn`, in the words a
/// person can choose between rather than the kernel's own 0/1/2.
const ECN: [(&str, &str); 3] = [("0", "Off"), ("1", "On"), ("2", "Incoming only")];

/// SWITCHES and VALUES are every control this page renders, split by how a
/// submission carries it: a switch is present only when it is on, a value is
/// present as whatever was typed. Together they are the closed set a post may
/// name, so the page and the save cannot drift apart.
/// SWITCHES is every on/off option this page writes, paired with what firewall4
/// assumes when the option is absent.
///
/// The pairing is the point. A switch whose state matches the daemon's assumption
/// writes nothing — staging the default back would count as a change nobody made —
/// and a switch that differs writes it out. Which way round that goes is
/// firewall4's business, not this page's, so it is recorded beside each option
/// rather than implied by where the option happens to sit in a list.
const SWITCHES: [(&str, bool); 10] = [
    ("synflood_protect", false),
    ("drop_invalid", false),
    ("flow_offloading", false),
    ("flow_offloading_hw", false),
    ("tcp_syncookies", true),
    ("tcp_window_scaling", true),
    ("accept_redirects", false),
    ("accept_source_route", false),
    // The two firewall4 does unless told otherwise: it assigns connection helpers
    // by itself, and it loads the custom rule files this page lists.
    ("auto_helper", true),
    ("auto_includes", true),
];

const VALUES: [&str; 8] = [
    "input",
    "output",
    "forward",
    "synflood_rate",
    "synflood_burst",
    "tcp_ecn",
    "tcp_reject_code",
    "any_reject_code",
];

/// save writes what the form carries and answers with the page as it now reads.
/// Only the options this page draws are looked at: a body naming anything else
/// is not something Verso rendered, and nothing outside the set can be written
/// through it.
///
/// A switch that is off is simply absent from a submission, which is why the
/// set is walked rather than the body: the difference between "turned off" and
/// "not on this page" is the difference between a write and a lie.
pub fn save(model: &mut Firewall, form: &Form) -> (Envelope, Vec<CommitOp>) {
    let mut values = Map::new();
    for (option, default_on) in SWITCHES {
        let on = !form.get(option).is_empty();
        set_switch(model, option, on);
        values.insert(option.to_string(), flag_value(on, default_on));
    }
    for option in VALUES {
        let value = form.get(option);
        set_value(model, option, &value);
        values.insert(option.to_string(), json!(value));
    }
    // A config with no defaults section runs firewall4's own; the first change
    // to it is what makes the section exist on disk.
    let op = match model.defaults.section.is_empty() {
        true => commit_new(model::CONFIG, "defaults", Value::Object(values)),
        false => commit(
            model::CONFIG,
            &model.defaults.section,
            Value::Object(values),
        ),
    };
    let mut ops = vec![op];
    for include in &mut model.includes {
        let enabled = !form.get(&include_field(include)).is_empty();
        if enabled != include.enabled {
            include.enabled = enabled;
            ops.push(commit(
                model::CONFIG,
                &include.section,
                json!({"enabled": if enabled { "1" } else { "0" }}),
            ));
        }
    }
    (page(model), ops)
}

/// flag_value states a switch the way firewall4 reads it, and clears it where
/// the state asked for is the one the daemon would assume: a stage that wrote
/// the default back would count as a change nobody made.
fn flag_value(on: bool, default_on: bool) -> Value {
    match on == default_on {
        true => json!(null),
        false => json!(if on { "1" } else { "0" }),
    }
}

fn set_switch(model: &mut Firewall, option: &str, on: bool) {
    let d = &mut model.defaults;
    match option {
        "synflood_protect" => d.synflood_protect = on,
        "drop_invalid" => d.drop_invalid = on,
        "flow_offloading" => d.flow_offloading = on,
        "flow_offloading_hw" => d.flow_offloading_hw = on,
        "tcp_syncookies" => d.syn_cookies = on,
        "tcp_window_scaling" => d.tcp_window_scaling = on,
        "accept_redirects" => d.accept_redirects = on,
        "accept_source_route" => d.accept_source_route = on,
        "auto_helper" => d.auto_helper = on,
        "auto_includes" => d.auto_includes = on,
        _ => {}
    }
}

fn set_value(model: &mut Firewall, option: &str, value: &str) {
    let d = &mut model.defaults;
    match option {
        "input" => d.input = value.to_string(),
        "output" => d.output = value.to_string(),
        "forward" => d.forward = value.to_string(),
        "synflood_rate" => d.synflood_rate = value.to_string(),
        "synflood_burst" => d.synflood_burst = value.to_string(),
        "tcp_ecn" => d.tcp_ecn = value.to_string(),
        "tcp_reject_code" => d.tcp_reject_code = value.to_string(),
        "any_reject_code" => d.any_reject_code = value.to_string(),
        _ => {}
    }
}

/// page renders the settings. It is one form: everything on it is staged
/// together and applied together, because the baseline and the protections are
/// one answer to "how does this firewall behave".
pub fn page(model: &Firewall) -> Envelope {
    let d = &model.defaults;
    page::envelope(
        HEADING,
        Widget::Grid(Grid {
            style: "rail".into(),
            columns: 2,
            children: vec![
                Widget::Form {
                    style: "page".into(),
                    submit: "Save settings".into(),
                    error: String::new(),
                    // The page header supplies the first section's spacing;
                    // later sections are ruled off from the one above. Each takes its
                    // address from the same list the rail's links are built
                    // from, so a link and the heading it points at cannot come
                    // apart — which is exactly what they had done.
                    fields: SECTIONS
                        .iter()
                        .zip([baseline(d), protection(d), speed(d), tcp(d), custom(model)])
                        .enumerate()
                        .map(|(index, ((anchor, _), section))| {
                            let section = section.addressed_as(anchor);
                            if index == 0 {
                                section.flush()
                            } else {
                                section.ruled()
                            }
                        })
                        .chain(std::iter::once(Widget::config_preview(
                            CONFIG_PATH,
                            &written(model),
                        )))
                        .collect(),
                    note: String::new(),
                    target: String::new(),
                }
                // Everything this form writes is the defaults section's, which
                // is where the shell looks for what waits on the stage.
                .at(model::CONFIG, &d.section),
                rail(),
            ],
            ..Default::default()
        }),
    )
}

/// The side rail only navigates. The configuration preview belongs to the
/// form, immediately before its Save action.
fn rail() -> Widget {
    Widget::stack(vec![Widget::section(
        "On this page",
        "",
        vec![Widget::stack(
            SECTIONS
                .iter()
                .map(|(anchor, title)| Widget::link(title, &format!("#{anchor}"), "rail"))
                .collect(),
        )
        .flush()],
    )
    .kicker()
    .flush()])
}

/// SECTIONS pairs each section of the form with the name it is addressed by, so
/// the rail's list and the headings it points at cannot fall out of step.
const SECTIONS: [(&str, &str); 5] = [
    ("zone", ZONE_TITLE),
    ("protection", PROTECTION_TITLE),
    ("speed", SPEED_TITLE),
    ("tcp", TCP_TITLE),
    ("custom", CUSTOM_TITLE),
];

const CONFIG_PATH: &str = "/etc/config/firewall";

/// written is the `config defaults` block as this page will write it — the same
/// options the form sets, in the file's own spelling, so what is about to happen
/// is readable before it does. The form declares it a live preview, so the shell
/// asks for it again as the form is edited and this answers from the values on
/// screen rather than from the file.
fn written(model: &Firewall) -> String {
    let d = &model.defaults;
    let mut out = String::from("config defaults\n");
    // The same two lists the save walks, read through the same accessors, so the
    // block cannot claim anything the save would not write. It was a third
    // hand-kept list, and it had already drifted in both directions: it named
    // `syn_cookies`, which is the struct's field and not firewall4's option, and it
    // wrote tcp_window_scaling's ON — the state firewall4 assumes — while the save
    // correctly writes only its OFF.
    for option in VALUES {
        let value = d.value(option);
        if !value.is_empty() {
            out.push_str(&format!("\toption {option} '{value}'\n"));
        }
    }
    for (option, default_on) in SWITCHES {
        if let Some(value) = flag_value(d.state(option), default_on).as_str() {
            out.push_str(&format!("\toption {option} '{value}'\n"));
        }
    }
    for include in &model.includes {
        out.push_str(&format!(
            "\nconfig include '{}'\n\toption enabled '{}'\n",
            include.section,
            if include.enabled { "1" } else { "0" },
        ));
    }
    out
}

/// baseline is the three policies firewall4 applies to traffic no zone claimed.
/// They are the same three questions a zone answers, so they are asked in the
/// same words and the same order.
fn baseline(d: &Defaults) -> Widget {
    Widget::section(
        ZONE_TITLE,
        "",
        vec![
            policy("input", "Traffic to this router", INPUT_HELP, &d.input),
            policy("output", "Traffic from this router", OUTPUT_HELP, &d.output),
            policy(
                "forward",
                "Traffic passing through",
                FORWARD_HELP,
                &d.forward,
            ),
            // What a refusal actually sends back. It belongs with the policies
            // because it is the same decision read one level down: "reject" is a
            // policy, and this is what the rejection says.
            choice(
                "tcp_reject_code",
                "When refusing a connection, say",
                TCP_REJECT_HELP,
                &d.tcp_reject_code,
                &REJECT_CODES,
            ),
            choice(
                "any_reject_code",
                "When refusing anything else, say",
                ANY_REJECT_HELP,
                &d.any_reject_code,
                &REJECT_CODES,
            ),
        ],
    )
}

/// REJECT_CODES is what firewall4 can send back when it refuses something, in its
/// own spelling, with what each one tells the sender.
const REJECT_CODES: [(&str, &str); 5] = [
    ("tcp-reset", "Connection refused (TCP reset)"),
    ("port-unreachable", "Nothing is listening"),
    ("admin-prohibited", "Refused deliberately"),
    ("host-unreachable", "No such host"),
    ("no-route", "No route to it"),
];

const TCP_REJECT_HELP: &str = "A reset is what a closed port on any ordinary machine answers \
with, so it tells nobody that a firewall is here. The others are more explicit.";

const ANY_REJECT_HELP: &str = "The same answer for traffic that is not a TCP connection, where a \
reset has no meaning.";

const INPUT_HELP: &str = "Traffic addressed to the router itself — the web interface, SSH, its \
DNS resolver. Not traffic it merely routes.";
const OUTPUT_HELP: &str = "Traffic the router itself originates: firmware checks, NTP, its own \
DNS lookups. Rarely worth restricting.";
const FORWARD_HELP: &str = "Traffic routed between two interfaces without ever touching the \
router's own services.";

/// choice is one pick over a closed set firewall4 names, labelled in the words a
/// person would use. A drop-down rather than a segmented strip: five answers is
/// more than a strip reads at a glance, and only one of them is ever in force.
fn choice(name: &str, label: &str, help: &str, value: &str, options: &[(&str, &str)]) -> Widget {
    Widget::Field(Field {
        name: name.into(),
        label: label.into(),
        kind: "select".into(),
        value: value.into(),
        options: options
            .iter()
            .map(|(value, label)| SelectOption::new(value, label))
            .collect(),
        help: help.into(),
        key: name.into(),
        ..Default::default()
    })
}

/// policy is one verdict choice; the shell chooses its control by option count.
fn policy(name: &str, label: &str, help: &str, value: &str) -> Widget {
    Widget::Field(Field {
        name: name.into(),
        label: label.into(),
        kind: "select".into(),
        value: value.into(),
        options: POLICIES
            .iter()
            .map(|policy| SelectOption::new(policy, policy))
            .collect(),
        help: help.into(),
        key: name.into(),
        ..Default::default()
    })
}

fn protection(d: &Defaults) -> Widget {
    Widget::section(
        PROTECTION_TITLE,
        "",
        vec![
            switch(
                "synflood_protect",
                "Slow down SYN floods",
                SYN_HELP,
                d.synflood_protect,
            ),
            Widget::form_grid(
                2,
                vec![
                    field(
                        "synflood_rate",
                        "Connections per second",
                        RATE_HELP,
                        &d.synflood_rate,
                        "/s",
                    ),
                    field(
                        "synflood_burst",
                        "Burst",
                        BURST_HELP,
                        &d.synflood_burst,
                        "packets",
                    ),
                ],
            )
            .labelled("Connection rate", ""),
            switch(
                "drop_invalid",
                "Drop packets that belong to no connection",
                INVALID_HELP,
                d.drop_invalid,
            ),
            // Two things the kernel would otherwise be told by strangers. Both are
            // off on any sane router, and both read as a loosening when on, which is
            // why they sit among the protections rather than as plain settings.
            switch(
                "accept_redirects",
                "Let the network redirect this router's traffic",
                REDIRECTS_HELP,
                d.accept_redirects,
            ),
            switch(
                "accept_source_route",
                "Let a packet choose its own route",
                SOURCE_ROUTE_HELP,
                d.accept_source_route,
            ),
        ],
    )
}

const SYN_HELP: &str = "Rate-limits half-open TCP connections so a flood cannot exhaust the \
connection table.";
const RATE_HELP: &str = "New connections per second before limiting kicks in. 25/s suits a \
household; raise it for a busy office.";
const BURST_HELP: &str = "How many may arrive at once before the rate limit applies. Absorbs \
ordinary bursts, like a page opening 40 sockets.";
const INVALID_HELP: &str = "Drops packets conntrack cannot match to a known connection. Leave \
off behind asymmetric routing or multi-WAN, where legitimate replies can arrive unmatched.";

const REDIRECTS_HELP: &str = "An ICMP redirect is another machine saying “send that somewhere \
else”. Obeying one lets whoever sent it steer this router's own traffic, so it stays off unless \
something on a trusted network needs it.";

const SOURCE_ROUTE_HELP: &str = "A source-routed packet carries its own list of hops. It is a way \
to reach places routing would not allow, which is why it is refused.";

fn speed(d: &Defaults) -> Widget {
    Widget::section(
        SPEED_TITLE,
        "",
        vec![Widget::Conditional {
            name: "flow_offloading".into(),
            label: "Shortcut established connections".into(),
            key: "flow_offloading".into(),
            help: FLOW_HELP.into(),
            checked: d.flow_offloading,
            // Hardware offload is only a question once software offload is on:
            // it is where the offload happens, not a second thing to turn on.
            fields: vec![switch(
                "flow_offloading_hw",
                "Do it in hardware",
                FLOW_HW_HELP,
                d.flow_offloading_hw,
            )],
            otherwise: Vec::new(),
        }],
    )
}

const FLOW_HELP: &str = "Established flows bypass the kernel's netfilter path. The first packets \
of every connection still pass the rules.";
const FLOW_HW_HELP: &str = "Hands the offload to the board's packet engine. Needs driver support \
on both interfaces.";

fn tcp(d: &Defaults) -> Widget {
    Widget::section(
        TCP_TITLE,
        "",
        vec![
            switch("tcp_syncookies", "SYN cookies", COOKIES_HELP, d.syn_cookies),
            switch(
                "tcp_window_scaling",
                "Window scaling",
                WINSCALE_HELP,
                d.tcp_window_scaling,
            ),
            // Three answers, but not a policy triplet: "Incoming only" is not
            // one short word, and a strip here would read as one more verdict.
            // A box, as every other closed pick on the page that is not accept,
            // reject or drop.
            choice(
                "tcp_ecn",
                "Congestion notification",
                ECN_HELP,
                &d.tcp_ecn,
                &ECN,
            ),
            // The helpers this section is named for. firewall4 assigns them itself
            // unless told not to, so the page states it rather than leaving the
            // behaviour to an assumption.
            switch(
                "auto_helper",
                "Assign connection helpers automatically",
                AUTO_HELPER_HELP,
                d.auto_helper,
            ),
        ],
    )
}

const AUTO_HELPER_HELP: &str = "A helper teaches the firewall to follow protocols that open \
further connections of their own, such as FTP or SIP. Assigning them automatically is convenient \
and widens what the firewall accepts, so a hardened router turns it off and names the helpers it \
wants on the rules that need them.";

const COOKIES_HELP: &str = "Answers a half-open connection without keeping state for it, so a \
flood cannot fill the backlog queue.";
const WINSCALE_HELP: &str = "Lets connections use windows above 64 KB. Off only for a broken \
middlebox on the path.";
const ECN_HELP: &str = "Lets routers signal congestion by marking packets instead of dropping \
them. A few old middleboxes still discard marked packets.";

/// custom is what fw4 loads besides what this interface writes, by where it
/// comes from. Each source is one setting — which of these load — asked of
/// each thing in turn: a label, then a checkbox row per file or folder, its
/// path verbatim and what it holds under it. First the files the config's
/// include sections name, then the folders fw4 reads on its own. Verso does
/// not author either, so nothing here offers a new file or an editor.
fn custom(model: &Firewall) -> Widget {
    let mut children = vec![
        RULE_FILES.listing(&model.rule_files),
        switch(
            "auto_includes",
            PACKAGE_FILES_LABEL,
            PACKAGE_FILES_HELP,
            model.defaults.auto_includes,
        ),
    ];
    let includes = &model.includes;
    if !includes.is_empty() {
        children.push(
            Widget::form_grid(1, includes.iter().map(include_switch).collect())
                .labelled(INCLUDES_LABEL, ""),
        );
    }
    for include in includes {
        if include.enabled && include.kind == "script" && !include.fw4_compatible {
            children.push(Widget::callout(
                Tone::Warning,
                &include.path,
                INCOMPATIBLE_NOTE,
            ));
        }
    }
    Widget::section(CUSTOM_TITLE, CUSTOM_SUB, children)
}

/// open_file is the settings page with a rule file's editor open over it, or
/// with the reason there is none.
pub fn open_file(model: &Firewall, name: &str) -> Envelope {
    match RULE_FILES.open(&model.rule_files, name) {
        Ok(editor) => with_editor(page(model), editor),
        Err(why) => page(model).with_notice(Tone::Danger, &why),
    }
}

/// save_file answers a rule file's editor: the editor again, with the command
/// that stages the file or the reason it cannot be one.
pub fn save_file(model: &Firewall, name: &str, form: &Form) -> Envelope {
    let Some((editor, command)) = RULE_FILES.save(&model.rule_files, name, form) else {
        return page(model);
    };
    let mut result = with_editor(page(model), editor);
    result.commands.extend(command);
    result
}

/// with_editor opens an editor's drawer over the page, outside its form.
fn with_editor(mut envelope: Envelope, editor: Widget) -> Envelope {
    if let Widget::Grid(Grid { children, .. }) = &mut envelope.widget {
        children.push(editor);
    }
    envelope.with_back("Cancel", RULE_FILES.page)
}

/// include_switch is one file an include section names: whether it loads,
/// its path, and what it holds. It writes the section's own `enabled`.
fn include_switch(include: &Include) -> Widget {
    let detail = if include.kind == "script" {
        "Shell script · runs after the ruleset loads".to_string()
    } else {
        format!("nftables · {}", include.hook)
    };
    // The label is the file's path, set verbatim.
    Widget::Switch(Switch {
        name: include_field(include),
        label: include.path.clone(),
        help: detail,
        on: include.enabled,
        key: "enabled".into(),
        verbatim: true,
        ..Default::default()
    })
    .at(model::CONFIG, &include.section)
}

fn include_field(include: &Include) -> String {
    format!("include_enabled_{}", include.section)
}

/// switch is one on/off row of this page, with the uci option beside its label.
/// switch is one on/off setting of this page. It states the option it writes
/// beside its label, as every other row here does — the switch is the control,
/// not a different kind of row.
fn switch(name: &str, label: &str, help: &str, on: bool) -> Widget {
    Widget::Switch(Switch {
        name: name.into(),
        label: label.into(),
        help: help.into(),
        on,
        key: name.into(),
        ..Default::default()
    })
}

/// field is one typed value of this page, with the unit riding inside the box's
/// trailing edge so the number and what it counts read as one thing.
fn field(name: &str, label: &str, help: &str, value: &str, unit: &str) -> Widget {
    Widget::Field(Field {
        name: name.into(),
        label: label.into(),
        value: value.into(),
        help: help.into(),
        key: name.into(),
        ..Default::default()
    })
    .counted_in(unit)
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::fixture;
    use serde_json::Value;

    fn body() -> Value {
        serde_json::to_value(page(&fixture::firewall())).expect("serialize")
    }

    /// The shell marks a control whose option waits on the stage by its full
    /// address, so the form names the defaults section its options live in.
    #[test]
    fn the_settings_form_says_the_defaults_section_holds_its_options() {
        let model = fixture::firewall();
        let want = format!("\"target\":\"firewall.{}\"", model.defaults.section);
        assert!(!model.defaults.section.is_empty());
        assert!(body().to_string().contains(&want), "{want} in {}", body());
    }

    /// The block the page shows and the write the page performs are the same thing
    /// said twice, so they are checked against each other rather than each against a
    /// hand-written expectation. They had drifted: the preview named a struct field
    /// where firewall4 names an option, and stated a default the save knew better
    /// than to write.
    #[test]
    fn the_config_preview_states_exactly_what_a_save_would_write() {
        let model = fixture::firewall();
        let preview = written(&model);
        let shown = preview
            .split("\nconfig include")
            .next()
            .expect("defaults preview");

        // What a save writes, for a submission that asks for the state the page is
        // currently showing.
        let mut asked = Vec::new();
        for (option, _) in SWITCHES {
            if model.defaults.state(option) {
                asked.push(format!("{option}=1"));
            }
        }
        for option in VALUES {
            asked.push(format!("{option}={}", model.defaults.value(option)));
        }
        for include in &model.includes {
            if include.enabled {
                asked.push(format!("{}=1", include_field(include)));
            }
        }
        let mut after = fixture::firewall();
        let (_, ops) = save(&mut after, &Form::parse(&asked.join("&")));
        let written = ops[0].values.as_object().expect("an object of options");

        for (option, value) in written {
            match value.as_str() {
                // An option the save states must appear in the block, with that value.
                Some(value) => assert!(
                    shown.contains(&format!("\toption {option} '{value}'\n")),
                    "the save writes {option} '{value}' and the preview does not show it:\n{shown}"
                ),
                // One the save clears must not: firewall4 assumes it, so the file is
                // quiet about it and so is the block.
                None => assert!(
                    !shown.contains(&format!("\toption {option} ")),
                    "the preview shows {option}, which the save leaves to firewall4:\n{shown}"
                ),
            }
        }
        // And nothing appears in the block that the save would not write at all.
        for line in shown.lines().filter(|line| line.starts_with('\t')) {
            let option = line.split_whitespace().nth(1).expect("an option name");
            assert!(
                written.contains_key(option),
                "the preview shows {option}, which no save writes:\n{shown}"
            );
        }
    }

    #[test]
    fn the_page_carries_the_shared_firewall_frame() {
        let body = body();
        assert_eq!(body["title"], HEADING);
        // Settings is a page of this plugin like the listings, and the bar says
        // so on every one of them.
        assert_eq!(body["pages"][3]["label"], "Settings");
        assert_eq!(body["pages"][3]["path"], "settings");
    }

    // The page is the form beside a rail, at the canvas's split: the reading
    // measure for the controls and a narrower column for navigation.
    // The rail is not a settings column — nothing in it is a
    // control — so it sits outside the form entirely.
    #[test]
    fn the_page_is_the_form_beside_its_rail() {
        let body = body();
        let grid = &body["widget"];
        assert_eq!(grid["type"], "grid");
        assert_eq!(grid["style"], "rail");
        assert_eq!(grid["children"][0]["type"], "form");
        let rail = &grid["children"][1];
        assert_eq!(rail["type"], "stack");
        // Where you are: one entry per section of the form, in the form's order.
        // The label over them is a kicker — it names the list, and a heading here
        // would read as one of the headings the list points at.
        let anchors = &rail["children"][0];
        assert_eq!(anchors["title"], "On this page");
        assert_eq!(anchors["kicker"], true);
        let links = anchors["children"][0]["children"]
            .as_array()
            .expect("anchors");
        let labels: Vec<&str> = links
            .iter()
            .map(|link| link["label"].as_str().unwrap_or_default())
            .collect();
        assert_eq!(
            labels,
            [
                ZONE_TITLE,
                PROTECTION_TITLE,
                SPEED_TITLE,
                TCP_TITLE,
                CUSTOM_TITLE
            ]
        );
        // And every one of them points at a section this page actually gives that
        // address. They were links to nowhere: the rail named five places and no
        // section on the page was addressable, so nothing moved when one was
        // pressed.
        let sections = grid["children"][0]["fields"].as_array().expect("sections");
        let addresses: Vec<&str> = sections
            .iter()
            .map(|s| s["anchor"].as_str().unwrap_or_default())
            .collect();
        for link in links {
            let href = link["href"].as_str().unwrap_or_default();
            assert_eq!(link["style"], "rail");
            let anchor = href.strip_prefix('#').unwrap_or_default();
            assert!(
                addresses.contains(&anchor),
                "{href} points at no section on this page; it has {addresses:?}"
            );
        }
        // And what will be written, in the file's own spelling.
        assert_eq!(rail["children"].as_array().unwrap().len(), 1);
        let preview = sections
            .last()
            .expect("configuration immediately before Save");
        assert_eq!(preview["type"], "code");
        assert_eq!(preview["label"], CONFIG_PATH);
        let value = preview["value"].as_str().expect("preview text");
        assert!(
            value.starts_with("config defaults\n"),
            "the preview names the section it writes: {value}"
        );
        assert!(
            value.contains("\toption input 'reject'"),
            "the preview states the policies the form holds: {value}"
        );
    }

    #[test]
    fn every_section_of_the_canvas_is_on_the_page() {
        let body = body();
        for title in [
            ZONE_TITLE,
            PROTECTION_TITLE,
            SPEED_TITLE,
            TCP_TITLE,
            CUSTOM_TITLE,
        ] {
            assert_eq!(fixture::section(&body, title)["title"], title);
        }
    }

    // Congestion notification has three answers but is no policy triplet: it is
    // a box, like every closed pick on the page that is not accept, reject or
    // drop — a strip there would read as one more verdict.
    #[test]
    fn congestion_notification_is_a_box_not_a_strip() {
        let field = fixture::control(&body(), "tcp_ecn");
        assert_eq!(field["type"], "field");
        assert_eq!(field["kind"], "select");
        assert!(field.get("style").is_none(), "{field}");
        assert_eq!(field["key"], "tcp_ecn");
        let options: Vec<String> = field["options"]
            .as_array()
            .expect("options")
            .iter()
            .map(|o| o["value"].as_str().unwrap_or_default().to_string())
            .collect();
        assert_eq!(options, ["0", "1", "2"]);
    }

    // The baseline is the same three questions a zone answers, asked in the same
    // words — and each one states the uci option it writes.
    #[test]
    fn the_baseline_offers_the_three_verdicts_as_one_pick() {
        let body = body();
        for (name, value) in [
            ("input", "reject"),
            ("output", "accept"),
            ("forward", "reject"),
        ] {
            let field = fixture::control(&body, name);
            assert_eq!(field["type"], "field");
            assert_eq!(field["kind"], "select");
            assert!(field.get("style").is_none());
            assert_eq!(field["key"], name);
            assert_eq!(field["value"], value);
            let options: Vec<String> = field["options"]
                .as_array()
                .expect("options")
                .iter()
                .map(|o| o["value"].as_str().unwrap_or_default().to_string())
                .collect();
            assert_eq!(options, vec!["accept", "reject", "drop"]);
        }
    }

    // Hardware offload is not a second thing to turn on: it is where the offload
    // happens, so it only exists once there is an offload.
    #[test]
    fn hardware_offload_hangs_off_the_offload_itself() {
        let body = body();
        let flow = fixture::control(&body, "flow_offloading");
        assert_eq!(flow["type"], "conditional");
        assert_eq!(flow["fields"][0]["name"], "flow_offloading_hw");
    }

    #[test]
    fn the_rate_and_burst_carry_their_units() {
        let body = body();
        assert_eq!(fixture::control(&body, "synflood_rate")["unit"], "/s");
        assert_eq!(fixture::control(&body, "synflood_burst")["unit"], "packets");
    }

    // Which included files load is one setting asked of each file: a label,
    // then a checkbox per file, its path verbatim and what it holds under it.
    // Each writes its own section's `enabled`, so each is marked on its own
    // while its change waits. Verso does not author an include, so nothing
    // offers a new file or an editor.
    #[test]
    fn the_included_files_are_one_labelled_set_of_checkboxes() {
        let body = body();
        let section = fixture::section(&body, CUSTOM_TITLE);
        let group = &section["children"][2];
        assert_eq!(group["type"], "grid");
        assert_eq!(group["style"], "form");
        assert_eq!(group["label"], INCLUDES_LABEL);
        let files = group["children"].as_array().unwrap();
        assert_eq!(files.len(), 2);

        let first = &files[0];
        assert_eq!(first["type"], "switch");
        assert_eq!(first["name"], "include_enabled_custom_nft");
        assert_eq!(first["label"], "/etc/nftables.d/10-custom.nft");
        assert_eq!(first["verbatim"], true);
        assert_eq!(first["key"], "enabled");
        assert_eq!(first["target"], "firewall.custom_nft");
        assert_eq!(first["help"], "nftables · chain-pre");
        assert_eq!(first["on"], true);

        let script = &files[1];
        assert_ne!(script["on"], true);
        assert_eq!(script["label"], "/etc/firewall.user");
        assert_eq!(
            script["help"],
            "Shell script · runs after the ruleset loads"
        );
    }

    fn with_rule_files() -> Firewall {
        let mut model = fixture::firewall();
        model.rule_files = vec![json!({
            "path": "/etc/nftables.d/10-custom.nft", "family": "fw4",
            "content": "# the office\nchain x {}\n", "version": "v1"
        })];
        model
    }

    // fw4's own folder of rule files is listed and edited exactly as DNS &
    // DHCP lists dnsmasq's option files: one band, New file, a row per file
    // opening its editor — the SDK draws both.
    #[test]
    fn the_rule_files_are_the_same_listing_as_dnsmasqs() {
        let body = serde_json::to_value(page(&with_rule_files())).expect("serialize");
        let section = fixture::section(&body, CUSTOM_TITLE);
        let listing = &section["children"][0];
        assert_eq!(
            listing,
            &serde_json::to_value(RULE_FILES.listing(&with_rule_files().rule_files)).unwrap()
        );
        let row = &listing["rows"][0];
        assert_eq!(row["group"]["label"], "Files");
        assert_eq!(
            row["group"]["add_href"],
            "/plugins/firewall/settings/files/new"
        );
        assert_eq!(
            row["panel"],
            "/plugins/firewall/settings/files/10-custom.nft"
        );
        assert_eq!(row["cells"][1]["text"], "1");
        assert_eq!(row["cells"][1]["sub"], "line");
    }

    // The packages' folder is one setting, said in words, under the files.
    #[test]
    fn loading_package_rule_files_is_one_setting() {
        let mut model = fixture::firewall();
        model.defaults.auto_includes = true;
        let body = serde_json::to_value(page(&model)).expect("serialize");
        let section = fixture::section(&body, CUSTOM_TITLE);
        let packages = &section["children"][1];
        assert_eq!(packages["type"], "switch");
        assert_eq!(packages["name"], "auto_includes");
        assert_eq!(packages["key"], "auto_includes");
        assert_eq!(packages["label"], PACKAGE_FILES_LABEL);
        assert_eq!(packages["on"], true);
    }

    // With no include sections there is no group of them.
    #[test]
    fn a_config_with_no_includes_draws_no_group() {
        let mut model = fixture::firewall();
        model.includes.clear();
        let body = serde_json::to_value(page(&model)).expect("serialize");
        let section = fixture::section(&body, CUSTOM_TITLE);
        assert_eq!(section["children"].as_array().unwrap().len(), 2);
    }

    // A rule file's address opens its editor over the page, outside the
    // settings form, closing back to the page.
    #[test]
    fn a_rule_files_address_opens_its_editor() {
        let body = serde_json::to_value(open_file(&with_rule_files(), "10-custom.nft")).unwrap();
        let editor = body["widget"]["children"]
            .as_array()
            .unwrap()
            .last()
            .unwrap()
            .clone();
        let drawer = &editor["rows"][0]["drawer"];
        assert_eq!(drawer["title"], "Edit file");
        assert_eq!(drawer["open"], true);
        assert_eq!(drawer["closed"], "/plugins/firewall/settings");
        let text = drawer.to_string();
        assert!(text.contains("chain x {}"), "{text}");
        let gone = serde_json::to_value(open_file(&with_rule_files(), "20-gone.nft")).unwrap();
        assert_eq!(gone["notice"]["text"], "The file is no longer available.");
    }

    // Saving a rule file stages it through the shell's file journal, under
    // fw4's folder; nothing is written to the firewall config.
    #[test]
    fn saving_a_rule_file_stages_it() {
        let saved = save_file(
            &with_rule_files(),
            "new",
            &Form::parse("filename=20-guest&content=chain+y+%7B%7D&expected=af63bd4c8601b7df"),
        );
        let body = serde_json::to_value(&saved).unwrap();
        assert_eq!(body["commands"][0]["name"], "config-file-stage");
        assert_eq!(
            body["commands"][0]["args"]["path"],
            "/etc/nftables.d/20-guest.nft"
        );
        assert_eq!(body["commands"][0]["args"]["content"], "chain y {}");
        assert!(body
            .get("commit")
            .is_none_or(|c| c.as_array().is_none_or(Vec::is_empty)));
    }

    // A script that loads without being marked for fw4 is named right under
    // the includes listing, with what to check before marking it.
    #[test]
    fn an_enabled_script_not_marked_for_fw4_is_named_under_the_listing() {
        let mut model = fixture::firewall();
        let script = model
            .includes
            .iter_mut()
            .find(|i| i.section == "legacy_sh")
            .unwrap();
        script.enabled = true;
        script.fw4_compatible = false;
        let body = serde_json::to_value(page(&model)).expect("serialize");
        let section = fixture::section(&body, CUSTOM_TITLE);
        let warning = &section["children"][3];
        assert_eq!(warning["type"], "callout");
        assert_eq!(warning["title"], "/etc/firewall.user");
        assert_eq!(warning["body"], INCOMPATIBLE_NOTE);
    }

    // Every control the page draws is one the save writes, and nothing the save
    // writes is absent from the page. Drift either way is a control that does
    // nothing or a write nobody asked for.
    #[test]
    fn every_control_on_the_page_is_one_the_save_writes() {
        let body = body();
        // tcp_window_scaling used to need naming here: it defaults on, and the save
        // handled it apart from the rest for that reason. It is in SWITCHES now,
        // beside what firewall4 assumes about it, so there is nothing left outside
        // the two lists.
        // A switch may be a listing row's toggle cell, which is named but is
        // not a widget of its own.
        let named = |name: &str| {
            fixture::find_with(&body, &|v| v["name"] == name)
                .unwrap_or_else(|| panic!("no control named {name}"))
        };
        for (name, _) in SWITCHES {
            named(name);
        }
        for name in VALUES {
            named(name);
        }
    }

    // A switch that is off is absent from a submission, so the save walks the
    // set rather than the body — and writes the off rather than assuming it.
    #[test]
    fn a_switch_left_off_is_written_off_and_not_merely_omitted() {
        let mut model = fixture::firewall();
        let (_, ops) = save(&mut model, &Form::parse("input=drop&tcp_window_scaling=1"));
        let values = &ops[0].values;
        assert_eq!(values["input"], "drop");
        assert_eq!(values["synflood_protect"], Value::Null);
        assert!(!model.defaults.synflood_protect);
        // Window scaling is on by firewall4's own default, so leaving it on
        // clears the option rather than writing the default back.
        assert_eq!(values["tcp_window_scaling"], Value::Null);
        assert!(model.defaults.tcp_window_scaling);
    }

    // …and a default the daemon assumes is cleared, not written back, so a
    // round trip that changes nothing stages nothing.
    #[test]
    fn turning_on_what_is_off_by_default_writes_the_one() {
        let mut model = fixture::firewall();
        let (_, ops) = save(&mut model, &Form::parse("drop_invalid=1"));
        assert_eq!(ops[0].values["drop_invalid"], "1");
        assert_eq!(ops[0].config, "firewall");
    }

    #[test]
    fn include_toggles_save_only_known_sections_and_update_the_preview() {
        let mut model = fixture::firewall();
        let (page, ops) = save(
            &mut model,
            &Form::parse("include_enabled_legacy_sh=1&include_enabled_unknown=1"),
        );
        assert_eq!(ops.len(), 3);
        assert_eq!(ops[1].section, "custom_nft");
        assert_eq!(ops[1].values, json!({"enabled": "0"}));
        assert_eq!(ops[2].section, "legacy_sh");
        assert_eq!(ops[2].values, json!({"enabled": "1"}));
        let body = serde_json::to_value(page).unwrap();
        // An include's switch is its row's toggle cell in the files listing.
        let toggle = |name: &str| {
            fixture::find_with(&body, &|v| v["name"] == name)
                .unwrap_or_else(|| panic!("no toggle named {name}"))
        };
        assert_ne!(toggle("include_enabled_custom_nft")["on"], true);
        assert_eq!(toggle("include_enabled_legacy_sh")["on"], true);
        assert!(written(&model).contains("config include 'custom_nft'\n\toption enabled '0'"));
        let (_, repeated) = save(&mut model, &Form::parse("include_enabled_legacy_sh=1"));
        assert_eq!(repeated.len(), 1, "unchanged includes must not stage again");
    }

    #[test]
    fn script_warning_follows_fw4_compatibility_not_script_type() {
        let mut snapshot: Value =
            serde_json::from_str(include_str!("../testdata/snapshot.json")).unwrap();
        let section = &mut snapshot["firewall"]["legacy_sh"];
        section["path"] = json!("/usr/libexec/verso/firewall-logging");
        section["enabled"] = json!("1");
        section["fw4_compatible"] = json!("1");
        let model = Firewall::read(&verso_plugin::Snapshot::from_value(snapshot.clone()));
        assert!(model.includes[1].fw4_compatible);
        assert!(model.includes[1].hook.is_empty());
        assert!(!serde_json::to_value(page(&model))
            .unwrap()
            .to_string()
            .contains(INCOMPATIBLE_NOTE));
        snapshot["firewall"]["legacy_sh"]["fw4_compatible"] = json!("0");
        let model = Firewall::read(&verso_plugin::Snapshot::from_value(snapshot));
        assert!(serde_json::to_value(page(&model))
            .unwrap()
            .to_string()
            .contains(INCOMPATIBLE_NOTE));
    }
}
