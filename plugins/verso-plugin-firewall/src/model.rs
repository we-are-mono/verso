// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

//! The firewall as this plugin understands it: the read snapshot mapped onto the
//! sections firewall4 actually acts on.
//!
//! Two things here are firewall4's semantics rather than UCI's, and the listings
//! depend on both. The **evaluation chain** is where a section's packets are
//! matched — it follows from the section's zones and target, not from any option
//! an operator writes — and it is both the lane a rule is grouped into and half
//! the identity its kernel counters carry. The **identity** is the other half:
//! firewall4 comments every rule it renders with the section's `name` option,
//! defaulting to the section id, which for an unnamed section is its position
//! among the sections of its type (`@rule[2]`).
//!
//! Option defaults are firewall4's, taken from its rule, redirect, zone, and
//! defaults specs: an absent `enabled` is on, an absent `proto` is tcp and udp,
//! an absent `target` on a redirect is dnat, and an absent zone or global policy
//! is drop.

use verso_plugin::files;
use verso_plugin::{Section, Snapshot, Ubus, Value};

/// CONFIG is the uci config this plugin reads and writes.
pub const CONFIG: &str = "firewall";

/// NETWORK_CONFIG holds the interfaces zones cover; a zone's subnets are read
/// from it, never from the firewall config, which names interfaces only.
pub const NETWORK_CONFIG: &str = "network";

/// Firewall is the whole readable state of one render: the firewall config's
/// sections in config order, plus the network interfaces a zone's subnets come
/// from.
pub struct Firewall {
    pub defaults: Defaults,
    pub zones: Vec<Zone>,
    pub forwardings: Vec<Forwarding>,
    pub rules: Vec<Rule>,
    pub redirects: Vec<Redirect>,
    pub nats: Vec<Nat>,
    pub includes: Vec<Include>,
    pub interfaces: Vec<Interface>,
    pub ipsets: Vec<String>,
    /// The network config's own `config device` sections — bridges and the like,
    /// which no interface names as its device but a rule may still match on.
    declared_devices: Vec<String>,
    /// The rule files fw4 reads from `/etc/nftables.d/`, as the helper reports
    /// them — not config, so the snapshot never carries them; the request's
    /// brokered read does (see [`Firewall::with_rule_files`]).
    pub rule_files: Vec<Value>,
}

/// Defaults is `config defaults` — the policy baseline applied before any zone
/// or rule. Section is empty when the config carries no defaults section at all,
/// in which case firewall4's own defaults are what the device runs.
pub struct Defaults {
    pub section: String,
    pub input: String,
    pub output: String,
    pub forward: String,
    pub drop_invalid: bool,
    pub synflood_protect: bool,
    /// What the flood protection allows before it starts limiting: a rate in
    /// firewall4's own `<n>/<unit>` spelling, and how many may arrive at once.
    pub synflood_rate: String,
    pub synflood_burst: String,
    pub flow_offloading: bool,
    pub flow_offloading_hw: bool,
    /// The kernel knobs firewall4 sets on the router's own stack. They are not
    /// about any zone or rule, which is why they sit under the settings rather
    /// than anywhere a packet is decided.
    pub syn_cookies: bool,
    pub tcp_window_scaling: bool,
    pub tcp_ecn: String,
    /// Two kernel behaviours firewall4 turns off for you, and whose on is a
    /// deliberate loosening: obeying ICMP redirects, and honouring a packet that
    /// says which way it should be routed.
    pub accept_redirects: bool,
    pub accept_source_route: bool,
    /// Whether firewall4 assigns connection helpers by itself, and whether it loads
    /// the custom nftables files at all. Both are on unless the config says
    /// otherwise, which is why the page has to state them.
    pub auto_helper: bool,
    pub auto_includes: bool,
    /// What a refusal sends back. firewall4 answers a refused TCP connection with a
    /// reset and everything else with "port unreachable"; a router that should look
    /// closed rather than defended says something quieter here.
    pub tcp_reject_code: String,
    pub any_reject_code: String,
}

/// Include is one `config include`: a snippet fw4 loads alongside the ruleset it
/// generates. Verso does not write these — they are authored over SSH — so what
/// the page can say about one is where it is, what kind it is, when it loads,
/// and whether it loads at all.
pub struct Include {
    pub section: String,
    pub path: String,
    /// "nftables" for a native snippet, "script" for a shell script run after
    /// the ruleset loads. Scripts must be compatible with fw4.
    pub kind: String,
    /// Where in fw4's own run the snippet is loaded.
    pub hook: String,
    pub enabled: bool,
    pub fw4_compatible: bool,
}

/// Zone is one `config zone`: the networks (or raw devices) it claims and the
/// policy they share.
pub struct Zone {
    pub section: String,
    pub name: String,
    pub networks: Vec<String>,
    pub devices: Vec<String>,
    pub input: String,
    /// What happens to traffic the router itself sends into this zone. It is
    /// the third of the three policies firewall4 applies per zone, and the one
    /// a reader most often has to go looking for, because a zone that answers
    /// nothing and can be reached by nothing is decided here.
    pub output: String,
    pub forward: String,
    pub masq: bool,
}

/// Forwarding is one `config forwarding`: traffic from `src` may cross into
/// `dest` without a rule of its own.
pub struct Forwarding {
    /// The uci handle, because a crossing is added and removed as a whole
    /// section rather than as an option on either zone.
    pub section: String,
    pub src: String,
    pub dest: String,
    /// firewall4 skips a crossing whose `enabled` is off — the zone then reaches
    /// nothing through it, whatever the section says. Absence means on.
    pub enabled: bool,
}

/// Rule is one `config rule` — a filtering decision in one evaluation chain.
pub struct Rule {
    pub section: String,
    pub identity: String,
    pub name: String,
    pub enabled: bool,
    pub src: String,
    pub dest: String,
    pub src_ips: Vec<String>,
    pub dest_ips: Vec<String>,
    pub proto: Vec<String>,
    pub family: String,
    pub dest_ports: Vec<String>,
    pub icmp_types: Vec<String>,
    pub limit: String,
    pub target: String,
    pub chain: String,
}

/// Redirect is one `config redirect`. A dnat redirect rewrites where inbound
/// traffic goes — the port forward; a snat redirect rewrites where outbound
/// traffic appears to come from, which the port-forwards listing leaves out.
pub struct Redirect {
    pub section: String,
    pub identity: String,
    pub name: String,
    pub enabled: bool,
    pub src: String,
    /// The zone the rewritten traffic leaves through. A port forward states none;
    /// a source rewrite names one, and either way it is a zone reference that
    /// follows the zone's name.
    pub dest: String,
    pub src_ips: Vec<String>,
    pub dest_ip: String,
    pub proto: Vec<String>,
    pub family: String,
    pub src_dport: String,
    pub dest_port: String,
    pub target: String,
    pub chain: String,
    /// The zones a port forward's NAT reflection rules are generated for. It is
    /// a second, list-shaped way a redirect names zones, and a zone named only
    /// there is still a zone this redirect depends on.
    pub reflection_zones: Vec<String>,
}

/// Nat is one `config nat` — firewall4's source rewrite, which decides how
/// traffic leaving a zone appears to the other side. This plugin does not edit
/// them, but each one names its zone by name, so they belong to what a zone's
/// blast radius counts.
pub struct Nat {
    pub src: String,
}

/// References is what points at one zone by name: the sections that would stop
/// matching if the zone stopped existing. A zone is referenced by name and by
/// nothing else, so this is the whole blast radius of editing or deleting one —
/// as far as the firewall config goes. Every section type firewall4 lets name a
/// zone is counted, the ones this plugin does not edit included: a count that
/// left them out would say a zone is free to go when it is not.
#[derive(Default, PartialEq, Eq, Debug)]
pub struct References {
    pub rules: usize,
    pub redirects: usize,
    pub forwardings: usize,
    pub nats: usize,
}

/// Interface is one `config interface` of the network config, reduced to what a
/// zone's subnet column can honestly state plus the kernel device it runs on —
/// the name a rule tied to one device has to use.
pub struct Interface {
    pub name: String,
    pub device: String,
    pub ipaddr: String,
    pub netmask: String,
    pub ip6assign: String,
}

impl Firewall {
    /// read maps the brokered snapshot onto the model. Every list keeps config
    /// order, which for rules and redirects is evaluation order.
    pub fn read(snapshot: &Snapshot) -> Firewall {
        let defaults = snapshot
            .sections_of_type(CONFIG, "defaults")
            .first()
            .map(Defaults::read)
            .unwrap_or_default();
        Firewall {
            zones: snapshot
                .sections_of_type(CONFIG, "zone")
                .iter()
                .map(|section| Zone::read(section, &defaults))
                .collect(),
            forwardings: snapshot
                .sections_of_type(CONFIG, "forwarding")
                .iter()
                .map(Forwarding::read)
                .collect(),
            rules: read_sections(snapshot, "rule", Rule::read),
            redirects: read_sections(snapshot, "redirect", Redirect::read),
            nats: snapshot
                .sections_of_type(CONFIG, "nat")
                .iter()
                .map(Nat::read)
                .collect(),
            includes: snapshot
                .sections_of_type(CONFIG, "include")
                .iter()
                .map(Include::read)
                .collect(),
            interfaces: snapshot
                .sections_of_type(NETWORK_CONFIG, "interface")
                .iter()
                .map(Interface::read)
                .collect(),
            ipsets: snapshot
                .sections_of_type(CONFIG, "ipset")
                .iter()
                .map(|section| section.scalar("name"))
                .filter(|name| !name.is_empty())
                .collect(),
            declared_devices: snapshot
                .sections_of_type(NETWORK_CONFIG, "device")
                .iter()
                .map(|section| section.scalar("name"))
                .collect(),
            defaults,
            rule_files: Vec::new(),
        }
    }

    /// with_rule_files adds the rule files the request's brokered read carries.
    pub fn with_rule_files(mut self, ubus: &Ubus) -> Firewall {
        self.rule_files = files::files(ubus.get("firewallFiles"));
        self
    }

    /// zone_names lists the zones a rule may name, in config order.
    pub fn zone_names(&self) -> Vec<String> {
        self.zones
            .iter()
            .filter(|zone| !zone.name.is_empty())
            .map(|zone| zone.name.clone())
            .collect()
    }

    /// devices lists the kernel devices the network config names — the interfaces'
    /// own devices and the devices the config declares in their own right. A rule
    /// tied to a device names one of these; the config is the only place this
    /// plugin can learn them, so a device it does not mention is not offered.
    pub fn devices(&self) -> Vec<String> {
        let mut names: Vec<String> = Vec::new();
        for device in self
            .interfaces
            .iter()
            .map(|interface| interface.device.clone())
            .chain(self.declared_devices.iter().cloned())
        {
            if !device.is_empty() && !names.contains(&device) {
                names.push(device);
            }
        }
        names
    }

    /// network_names lists the logical networks a zone may cover, in config
    /// order. The firewall config names interfaces and never defines them, so
    /// the network config is the only place this plugin can learn them.
    pub fn network_names(&self) -> Vec<String> {
        self.interfaces
            .iter()
            .map(|interface| interface.name.clone())
            .filter(|name| !name.is_empty())
            .collect()
    }

    /// references counts what names one zone. Neither of firewall4's two
    /// wildcards is a zone: a section stating no source is one about the router
    /// itself, and one stating `*` is one about anywhere at all. Counting either
    /// against a zone would make it look load-bearing when nothing points at it.
    pub fn references(&self, name: &str) -> References {
        if !named_zone(name) {
            return References::default();
        }
        let names = |src: &str, dest: &str| src == name || dest == name;
        References {
            rules: self
                .rules
                .iter()
                .filter(|rule| names(&rule.src, &rule.dest))
                .count(),
            // A redirect names zones twice over: the path it rewrites, and the
            // zones whose hosts reach the forward by its public address. A
            // section is one reference whichever way it names the zone.
            redirects: self
                .redirects
                .iter()
                .filter(|redirect| {
                    names(&redirect.src, &redirect.dest)
                        || redirect.reflection_zones.iter().any(|zone| zone == name)
                })
                .count(),
            forwardings: self
                .forwardings
                .iter()
                .filter(|forwarding| names(&forwarding.src, &forwarding.dest))
                .count(),
            nats: self.nats.iter().filter(|nat| nat.src == name).count(),
        }
    }

    /// interface returns the network interface a zone names, if the network
    /// config defines one under that name.
    pub fn interface(&self, name: &str) -> Option<&Interface> {
        self.interfaces
            .iter()
            .find(|interface| interface.name == name)
    }

    /// forwards_from lists the zones traffic leaving `zone` may cross into.
    ///
    /// A crossing firewall4 skips is not one: a section switched off forwards
    /// nothing, so listing it would tell a reader this zone reaches somewhere it
    /// does not. Each zone appears once however many sections say so.
    pub fn forwards_from(&self, zone: &str) -> Vec<String> {
        self.crossings(
            |forwarding| forwarding.src == zone,
            |forwarding| &forwarding.dest,
        )
    }

    /// forwards_into is the other direction: the zones whose traffic may cross
    /// into `zone`. Each of those is the other zone's crossing to edit, which is
    /// why the panel states them rather than offering them.
    pub fn forwards_into(&self, zone: &str) -> Vec<String> {
        self.crossings(
            |forwarding| forwarding.dest == zone,
            |forwarding| &forwarding.src,
        )
    }

    /// crossings is the shared half of the two: the crossings in force that
    /// match, named by their far end, once each.
    fn crossings(
        &self,
        matching: impl Fn(&Forwarding) -> bool,
        far_end: impl Fn(&Forwarding) -> &String,
    ) -> Vec<String> {
        let mut out: Vec<String> = Vec::new();
        for forwarding in &self.forwardings {
            if !forwarding.enabled || !matching(forwarding) {
                continue;
            }
            let zone = far_end(forwarding);
            if !out.contains(zone) {
                out.push(zone.clone());
            }
        }
        out
    }

    /// forwardings_from is the sections themselves — what a save has to add to,
    /// switch back on, or remove. The listing reads names; only the editor needs
    /// the handles behind them.
    pub fn forwardings_from(&self, zone: &str) -> Vec<&Forwarding> {
        self.forwardings
            .iter()
            .filter(|forwarding| forwarding.src == zone)
            .collect()
    }
}

/// read_sections maps every section of one type, handing each its position among
/// them — the number firewall4's section id counts to for an unnamed section.
fn read_sections<T>(snapshot: &Snapshot, typ: &str, read: fn(&Section, usize) -> T) -> Vec<T> {
    snapshot
        .sections_of_type(CONFIG, typ)
        .iter()
        .enumerate()
        .map(|(position, section)| read(section, position))
        .collect()
}

impl Default for Defaults {
    fn default() -> Defaults {
        Defaults {
            section: String::new(),
            input: "drop".into(),
            output: "drop".into(),
            forward: "drop".into(),
            drop_invalid: false,
            synflood_protect: false,
            synflood_rate: "25/s".into(),
            synflood_burst: "50".into(),
            flow_offloading: false,
            flow_offloading_hw: false,
            syn_cookies: true,
            tcp_window_scaling: true,
            tcp_ecn: "2".into(),
            accept_redirects: false,
            accept_source_route: false,
            auto_helper: true,
            auto_includes: true,
            tcp_reject_code: "tcp-reset".into(),
            any_reject_code: "port-unreachable".into(),
        }
    }
}

impl Defaults {
    fn read(section: &Section) -> Defaults {
        Defaults {
            section: section.name(),
            input: policy(section, "input", "drop"),
            output: policy(section, "output", "drop"),
            forward: policy(section, "forward", "drop"),
            drop_invalid: flag(section, "drop_invalid", false),
            synflood_protect: flag(section, "synflood_protect", false),
            // firewall4's own defaults, written here rather than left blank so
            // the page states what the device will actually do.
            synflood_rate: scalar_or(section, "synflood_rate", "25/s"),
            synflood_burst: scalar_or(section, "synflood_burst", "50"),
            flow_offloading: flag(section, "flow_offloading", false),
            flow_offloading_hw: flag(section, "flow_offloading_hw", false),
            syn_cookies: flag(section, "tcp_syncookies", true),
            tcp_window_scaling: flag(section, "tcp_window_scaling", true),
            tcp_ecn: scalar_or(section, "tcp_ecn", "2"),
            accept_redirects: flag(section, "accept_redirects", false),
            accept_source_route: flag(section, "accept_source_route", false),
            auto_helper: flag(section, "auto_helper", true),
            auto_includes: flag(section, "auto_includes", true),
            tcp_reject_code: scalar_or(section, "tcp_reject_code", "tcp-reset"),
            any_reject_code: scalar_or(section, "any_reject_code", "port-unreachable"),
        }
    }

    /// state reads one of the switchable options by its uci name.
    /// It answers for every option the settings page can write, because the page,
    /// the save and the config preview all ask it rather than each reaching into the
    /// struct for a field it happens to know the name of. The preview used to do
    /// that, and wrote `syn_cookies` — the field's name, not the option's — into a
    /// block claiming to be the file.
    pub fn state(&self, option: &str) -> bool {
        match option {
            "drop_invalid" => self.drop_invalid,
            "synflood_protect" => self.synflood_protect,
            "flow_offloading" => self.flow_offloading,
            "flow_offloading_hw" => self.flow_offloading_hw,
            "tcp_syncookies" => self.syn_cookies,
            "tcp_window_scaling" => self.tcp_window_scaling,
            "accept_redirects" => self.accept_redirects,
            "accept_source_route" => self.accept_source_route,
            "auto_helper" => self.auto_helper,
            "auto_includes" => self.auto_includes,
            _ => false,
        }
    }

    /// value is the same for the options that hold a value rather than a state.
    pub fn value(&self, option: &str) -> &str {
        match option {
            "input" => &self.input,
            "output" => &self.output,
            "forward" => &self.forward,
            "synflood_rate" => &self.synflood_rate,
            "synflood_burst" => &self.synflood_burst,
            "tcp_ecn" => &self.tcp_ecn,
            "tcp_reject_code" => &self.tcp_reject_code,
            "any_reject_code" => &self.any_reject_code,
            _ => "",
        }
    }

    /// set writes one of the switchable options by its uci name.
    pub fn set(&mut self, option: &str, on: bool) {
        match option {
            "drop_invalid" => self.drop_invalid = on,
            "synflood_protect" => self.synflood_protect = on,
            "flow_offloading" => self.flow_offloading = on,
            _ => {}
        }
    }
}

impl Zone {
    fn read(section: &Section, defaults: &Defaults) -> Zone {
        Zone {
            section: section.name(),
            name: section.scalar("name"),
            networks: values(section, "network"),
            devices: values(section, "device"),
            input: policy(section, "input", &defaults.input),
            output: policy(section, "output", &defaults.output),
            forward: policy(section, "forward", &defaults.forward),
            masq: flag(section, "masq", false),
        }
    }
}

impl Include {
    fn read(section: &Section) -> Include {
        let path = section.scalar("path");
        // fw4 reads `type` as nftables or script; an include that states none is
        // a script. Script includes do not have an nftables insertion position.
        let kind = match section.scalar("type").as_str() {
            "nftables" => "nftables".to_string(),
            _ => "script".to_string(),
        };
        Include {
            section: section.name(),
            hook: if kind == "nftables" {
                scalar_or(section, "position", "table-append")
            } else {
                String::new()
            },
            enabled: flag(section, "enabled", true),
            fw4_compatible: flag(section, "fw4_compatible", path != "/etc/firewall.user"),
            path,
            kind,
        }
    }
}

impl Forwarding {
    fn read(section: &Section) -> Forwarding {
        Forwarding {
            section: section.name(),
            src: section.scalar("src"),
            dest: section.scalar("dest"),
            enabled: flag(section, "enabled", true),
        }
    }
}

impl Rule {
    fn read(section: &Section, position: usize) -> Rule {
        let src = section.scalar("src");
        let dest = section.scalar("dest");
        let target = section.scalar("target").to_lowercase();
        Rule {
            section: section.name(),
            identity: identity(section, "rule", position),
            name: section.scalar("name"),
            enabled: flag(section, "enabled", true),
            chain: rule_chain(&target, &src, &dest),
            src,
            dest,
            src_ips: values(section, "src_ip"),
            dest_ips: values(section, "dest_ip"),
            proto: values(section, "proto"),
            family: section.scalar("family"),
            dest_ports: values(section, "dest_port"),
            icmp_types: values(section, "icmp_type"),
            limit: section.scalar("limit"),
            target,
        }
    }
}

impl Redirect {
    fn read(section: &Section, position: usize) -> Redirect {
        let src = section.scalar("src");
        // firewall4 defaults a redirect's target to dnat and falls back to dnat
        // for anything it does not recognize, so only an explicit snat is not a
        // port forward.
        let target = match section.scalar("target").to_lowercase().as_str() {
            "snat" => "snat".to_string(),
            _ => "dnat".to_string(),
        };
        Redirect {
            section: section.name(),
            identity: identity(section, "redirect", position),
            name: section.scalar("name"),
            enabled: flag(section, "enabled", true),
            chain: redirect_chain(&target, &src),
            src,
            dest: section.scalar("dest"),
            src_ips: values(section, "src_ip"),
            dest_ip: section.scalar("dest_ip"),
            proto: values(section, "proto"),
            family: section.scalar("family"),
            src_dport: section.scalar("src_dport"),
            dest_port: section.scalar("dest_port"),
            reflection_zones: values(section, "reflection_zone"),
            target,
        }
    }

    /// is_port_forward reports whether the redirect belongs on the port-forwards
    /// listing: the dnat direction, inbound traffic sent somewhere else.
    pub fn is_port_forward(&self) -> bool {
        self.target == "dnat"
    }
}

impl Nat {
    fn read(section: &Section) -> Nat {
        Nat {
            src: section.scalar("src"),
        }
    }
}

impl Interface {
    fn read(section: &Section) -> Interface {
        Interface {
            name: section.name(),
            device: section.scalar("device"),
            ipaddr: first_value(section, "ipaddr"),
            netmask: first_value(section, "netmask"),
            ip6assign: section.scalar("ip6assign"),
        }
    }
}

/// identity is the name firewall4 comments a section's kernel rules with: the
/// `name` option, or the section id — the section's own name, or its position
/// among the sections of its type when uci generated the name.
fn identity(section: &Section, typ: &str, position: usize) -> String {
    let name = section.scalar("name");
    if !name.is_empty() {
        return name;
    }
    if section.anonymous() {
        return format!("@{typ}[{position}]");
    }
    section.name()
}

/// rule_chain is the chain firewall4 evaluates a rule in. The filtering targets
/// take the zone pair's chain; notrack and helper take their own per-zone chain;
/// mark and dscp are packet mangling and take a chain named for the hook they
/// run on. A `*` zone is "any", which is a different chain from no zone at all.
pub fn rule_chain(target: &str, src: &str, dest: &str) -> String {
    let src_zone = named_zone(src);
    match target {
        // firewall4 refuses a notrack or helper rule that names no source zone,
        // so such a section renders nothing; the filtering lane is the closest
        // honest place for a row the packet filter never reaches.
        "notrack" | "helper" if src_zone => format!("{target}_{src}"),
        "mark" | "dscp" => mangle_chain(src, dest),
        _ => filter_chain(src, dest),
    }
}

fn filter_chain(src: &str, dest: &str) -> String {
    match (src, dest) {
        ("", "") => "output".into(),
        ("", "*") => "output".into(),
        ("", zone) => format!("output_{zone}"),
        ("*", "") => "input".into(),
        ("*", _) => "forward".into(),
        (zone, "") => format!("input_{zone}"),
        (zone, _) => format!("forward_{zone}"),
    }
}

fn mangle_chain(src: &str, dest: &str) -> String {
    let hook = match (src, dest) {
        ("*", "*") => "forward",
        ("*", "") => "input",
        ("*", _) => "postrouting",
        ("", _) => "output",
        (_, "*") => "prerouting",
        (_, "") => "input",
        (_, _) => "forward",
    };
    format!("mangle_{hook}")
}

/// redirect_chain is the nat chain a redirect evaluates in. Both directions are
/// named for the zone the traffic is seen on; a redirect naming no zone there is
/// one firewall4 skips, and has no chain.
fn redirect_chain(target: &str, src: &str) -> String {
    if target == "dnat" && named_zone(src) {
        return format!("dstnat_{src}");
    }
    String::new()
}

fn named_zone(zone: &str) -> bool {
    !zone.is_empty() && zone != "*"
}

/// values reads an option that may be written either as a uci list or as one
/// whitespace-separated string — firewall4 accepts both for its list options.
pub fn values(section: &Section, option: &str) -> Vec<String> {
    let list = section.list(option);
    if !list.is_empty() {
        return list;
    }
    section
        .scalar(option)
        .split_whitespace()
        .map(String::from)
        .collect()
}

/// first_value reads a scalar option that a config may also have written as a
/// single-entry list (uci allows both for an interface address).
fn first_value(section: &Section, option: &str) -> String {
    let scalar = section.scalar(option);
    if !scalar.is_empty() {
        return scalar;
    }
    section.list(option).into_iter().next().unwrap_or_default()
}

/// policy lowercases a policy option for display; uci writes them upper case.
/// scalar_or reads an option, or states firewall4's own default for it where
/// the config is silent. A page that showed an empty box would be saying the
/// device does nothing, which is not what an unset option means.
fn scalar_or(section: &Section, option: &str, fallback: &str) -> String {
    let value = section.scalar(option);
    match value.is_empty() {
        true => fallback.to_string(),
        false => value,
    }
}

fn policy(section: &Section, option: &str, fallback: &str) -> String {
    let value = section.scalar(option);
    if value.is_empty() {
        return fallback.to_string();
    }
    value.to_lowercase()
}

/// flag reads a boolean option in firewall4's vocabulary; anything it does not
/// recognize leaves the option at its default, which is what firewall4 does.
pub fn flag(section: &Section, option: &str, fallback: bool) -> bool {
    boolean(&section.scalar(option)).unwrap_or(fallback)
}

/// boolean is firewall4's own reading of a written truth value. None means the
/// value is not one — which for most options is a fallback to the default, but
/// for `log` is the difference between "on" and "on, with this prefix".
pub fn boolean(value: &str) -> Option<bool> {
    match value {
        "1" | "on" | "true" | "yes" => Some(true),
        "0" | "off" | "false" | "no" => Some(false),
        _ => None,
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::fixture;

    #[test]
    fn a_rules_chain_follows_its_zone_pair() {
        assert_eq!(rule_chain("accept", "wan", ""), "input_wan");
        assert_eq!(rule_chain("accept", "wan", "lan"), "forward_wan");
        assert_eq!(rule_chain("accept", "wan", "*"), "forward_wan");
        assert_eq!(rule_chain("accept", "*", ""), "input");
        assert_eq!(rule_chain("accept", "*", "lan"), "forward");
        assert_eq!(rule_chain("accept", "", "lan"), "output_lan");
        assert_eq!(rule_chain("accept", "", "*"), "output");
        assert_eq!(rule_chain("accept", "", ""), "output");
    }

    #[test]
    fn a_targets_own_chain_wins_over_the_zone_pair() {
        assert_eq!(rule_chain("notrack", "lan", ""), "notrack_lan");
        assert_eq!(rule_chain("helper", "wan", ""), "helper_wan");
        assert_eq!(rule_chain("mark", "lan", "wan"), "mangle_forward");
        assert_eq!(rule_chain("mark", "*", "*"), "mangle_forward");
        assert_eq!(rule_chain("dscp", "*", "wan"), "mangle_postrouting");
        assert_eq!(rule_chain("dscp", "lan", "*"), "mangle_prerouting");
        assert_eq!(rule_chain("mark", "lan", ""), "mangle_input");
        assert_eq!(rule_chain("mark", "", "wan"), "mangle_output");
        // A source zone is required, so one without falls back to the filter lane.
        assert_eq!(rule_chain("notrack", "*", ""), "input");
    }

    #[test]
    fn a_redirects_chain_is_its_source_zones_dstnat() {
        assert_eq!(redirect_chain("dnat", "wan"), "dstnat_wan");
        assert_eq!(redirect_chain("snat", "wan"), "");
        assert_eq!(redirect_chain("dnat", "*"), "");
        assert_eq!(redirect_chain("dnat", ""), "");
    }

    #[test]
    fn an_unnamed_section_is_identified_by_its_position() {
        let model = fixture::firewall();
        let identities: Vec<&str> = model
            .rules
            .iter()
            .map(|rule| rule.identity.as_str())
            .collect();
        // The fixture's seventh rule carries no name option and was written
        // without a section name, so firewall4 calls it by its position.
        assert!(identities.contains(&"@rule[6]"), "{identities:?}");
        assert_eq!(identities[0], "Allow-DHCP-Renew");
        // A named section with no name option keeps its section name.
        assert!(identities.contains(&"family_to_printer"), "{identities:?}");
    }

    #[test]
    fn sections_keep_config_order_and_firewall4_defaults() {
        let model = fixture::firewall();
        assert_eq!(model.defaults.section, "cfg01e63d");
        assert_eq!(model.defaults.input, "reject");
        assert_eq!(model.defaults.output, "accept");
        assert_eq!(model.defaults.forward, "reject");
        assert!(model.defaults.synflood_protect);
        assert!(!model.defaults.drop_invalid);
        assert!(!model.defaults.flow_offloading);

        let zones: Vec<&str> = model.zones.iter().map(|zone| zone.name.as_str()).collect();
        assert_eq!(zones, vec!["lan", "wan", "guest", "tailscale"]);
        // An absent zone policy takes the defaults section's, not firewall4's
        // built-in drop.
        assert_eq!(model.zones[2].input, "reject");
        assert_eq!(model.zones[2].forward, "reject");
        assert!(model.zones[1].masq);

        assert_eq!(model.forwards_from("lan"), vec!["wan".to_string()]);
        assert!(model.forwards_from("wan").is_empty());
        // A crossing firewall4 skips is not a crossing: the guest zone's section
        // says it reaches the uplink and is switched off, so it reaches nothing.
        assert!(model.forwards_from("guest").is_empty());
        assert_eq!(model.forwards_into("wan"), vec!["lan".to_string()]);
    }

    #[test]
    fn a_zones_blast_radius_is_everything_that_names_it() {
        let model = fixture::firewall();
        // Two rules point at lan as their destination, the source rewrite leaves
        // through it, the port forward reflects into it, and the one forwarding
        // starts there.
        assert_eq!(
            model.references("lan"),
            References {
                rules: 2,
                redirects: 2,
                forwardings: 1,
                nats: 0
            }
        );
        // The uplink carries the stock rules, both redirects, and both crossings
        // from the other end — the one out of lan and the switched-off one out of
        // guest. A blast radius counts sections that name the zone, in force or
        // not: one fw4 is currently skipping still breaks if the zone it names
        // stops existing, and is still a section somebody has to go and fix.
        assert_eq!(
            model.references("wan"),
            References {
                rules: 6,
                redirects: 2,
                forwardings: 2,
                nats: 0
            }
        );
        assert_eq!(
            model.references("guest"),
            References {
                rules: 3,
                redirects: 2,
                forwardings: 1,
                nats: 1
            }
        );
        // A zone nothing points at is free to go.
        assert_eq!(model.references("tailscale"), References::default());
        assert_eq!(model.references("no_such_zone"), References::default());
    }

    /// firewall4 lets a section name a zone in two places this plugin does not
    /// otherwise read: a `config nat`'s source, and the reflection zones a port
    /// forward generates its hairpin rules for. A blast radius that skipped them
    /// would tell an operator a zone is free to delete when it is not.
    #[test]
    fn a_nat_source_and_a_reflection_zone_are_references_too() {
        let model = fixture::firewall();
        // The guest zone is named by exactly one section of each: the source NAT
        // it leaves through, and the forward that reflects into it.
        assert_eq!(model.references("guest").nats, 1);
        let guest_redirects: Vec<&str> = model
            .redirects
            .iter()
            .filter(|redirect| {
                redirect.src == "guest"
                    || redirect.reflection_zones.iter().any(|zone| zone == "guest")
            })
            .map(|redirect| redirect.section.as_str())
            .collect();
        assert_eq!(guest_redirects, vec!["force_dns_guest", "https_to_nas"]);
        // A redirect naming the zone both ways is still one section.
        assert_eq!(model.references("wan").redirects, 2);
    }

    /// A rule that states no source is a rule about the router itself, and a
    /// rule that states `*` is one about anywhere — neither names a zone, so
    /// neither may be counted against one.
    #[test]
    fn the_router_and_anywhere_are_not_zones_anything_references() {
        let model = fixture::firewall();
        assert_eq!(model.references(""), References::default());
        assert_eq!(model.references("*"), References::default());
    }

    #[test]
    fn the_networks_a_zone_may_cover_come_from_the_network_config() {
        let model = fixture::firewall();
        assert_eq!(
            model.network_names(),
            vec!["loopback", "lan", "lan2", "wan", "wan6", "guest"]
        );
    }

    #[test]
    fn a_disabled_section_is_read_as_off_and_everything_else_as_on() {
        let model = fixture::firewall();
        let disabled: Vec<&str> = model
            .rules
            .iter()
            .filter(|rule| !rule.enabled)
            .map(|rule| rule.section.as_str())
            .collect();
        assert_eq!(disabled, vec!["block_telnet"]);
        assert!(model.redirects.iter().all(|redirect| redirect.enabled));
    }

    #[test]
    fn only_a_dnat_redirect_is_a_port_forward() {
        let model = fixture::firewall();
        let forwards: Vec<&str> = model
            .redirects
            .iter()
            .filter(|redirect| redirect.is_port_forward())
            .map(|redirect| redirect.section.as_str())
            .collect();
        assert_eq!(forwards, vec!["force_dns_guest", "https_to_nas"]);
    }

    #[test]
    fn an_option_written_either_way_reads_the_same() {
        let model = fixture::firewall();
        let icmpv6 = model
            .rules
            .iter()
            .find(|rule| rule.name == "Allow-ICMPv6-Input")
            .expect("rule");
        assert_eq!(icmpv6.icmp_types.len(), 11);
        let dns = model
            .rules
            .iter()
            .find(|rule| rule.name == "Allow-DNS-Guest")
            .expect("rule");
        // The same option, written as one whitespace-separated string.
        assert_eq!(dns.dest_ports, vec!["53", "67", "547"]);
    }
}
