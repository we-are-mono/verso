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

use verso_plugin::{Section, Snapshot};

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
    pub interfaces: Vec<Interface>,
    pub ipsets: Vec<String>,
    /// The network config's own `config device` sections — bridges and the like,
    /// which no interface names as its device but a rule may still match on.
    declared_devices: Vec<String>,
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
    pub flow_offloading: bool,
}

/// Zone is one `config zone`: the networks (or raw devices) it claims and the
/// policy they share.
pub struct Zone {
    pub section: String,
    pub name: String,
    pub networks: Vec<String>,
    pub devices: Vec<String>,
    pub input: String,
    pub forward: String,
    pub masq: bool,
}

/// Forwarding is one `config forwarding`: traffic from `src` may cross into
/// `dest` without a rule of its own.
pub struct Forwarding {
    pub src: String,
    pub dest: String,
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
    pub src_ips: Vec<String>,
    pub dest_ip: String,
    pub proto: Vec<String>,
    pub family: String,
    pub src_dport: String,
    pub dest_port: String,
    pub target: String,
    pub chain: String,
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
        }
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

    /// rule returns the rule section by its uci name.
    pub fn rule(&self, section: &str) -> Option<&Rule> {
        self.rules.iter().find(|rule| rule.section == section)
    }

    /// interface returns the network interface a zone names, if the network
    /// config defines one under that name.
    pub fn interface(&self, name: &str) -> Option<&Interface> {
        self.interfaces
            .iter()
            .find(|interface| interface.name == name)
    }

    /// forwards_from lists the zones traffic leaving `zone` may cross into.
    pub fn forwards_from(&self, zone: &str) -> Vec<String> {
        self.forwardings
            .iter()
            .filter(|forwarding| forwarding.src == zone)
            .map(|forwarding| forwarding.dest.clone())
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
            flow_offloading: false,
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
            flow_offloading: flag(section, "flow_offloading", false),
        }
    }

    /// state reads one of the switchable options by its uci name.
    pub fn state(&self, option: &str) -> bool {
        match option {
            "drop_invalid" => self.drop_invalid,
            "synflood_protect" => self.synflood_protect,
            "flow_offloading" => self.flow_offloading,
            _ => false,
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
            forward: policy(section, "forward", &defaults.forward),
            masq: flag(section, "masq", false),
        }
    }
}

impl Forwarding {
    fn read(section: &Section) -> Forwarding {
        Forwarding {
            src: section.scalar("src"),
            dest: section.scalar("dest"),
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
            src_ips: values(section, "src_ip"),
            dest_ip: section.scalar("dest_ip"),
            proto: values(section, "proto"),
            family: section.scalar("family"),
            src_dport: section.scalar("src_dport"),
            dest_port: section.scalar("dest_port"),
            target,
        }
    }

    /// is_port_forward reports whether the redirect belongs on the port-forwards
    /// listing: the dnat direction, inbound traffic sent somewhere else.
    pub fn is_port_forward(&self) -> bool {
        self.target == "dnat"
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
