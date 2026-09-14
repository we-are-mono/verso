// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

//! Turning firewall4's vocabulary into the listing's.
//!
//! A firewall listing is read by two people at once: the operator who wants to
//! know which way traffic goes, and the expert who wants the ruleset. So each
//! value is stated plainly and its machine form is kept beside it — a group
//! reads "WAN → Router" and shows `input_wan`; a protocol reads `tcp/udp` and a
//! v4-only rule says so. Nothing here invents a value the config does not carry.

use verso_plugin::Tone;

/// EM_DASH stands for a value the config does not state.
pub const EM_DASH: &str = "—";

/// ACRONYMS are the zone-name words that are read as letters, not as a word. A
/// token outside this set is simply capitalized, so an unknown zone name still
/// reads as a name and never as a mangled acronym.
const ACRONYMS: [(&str, &str); 5] = [
    ("wan", "WAN"),
    ("lan", "LAN"),
    ("dmz", "DMZ"),
    ("vpn", "VPN"),
    ("iot", "IoT"),
];

/// zone_label renders a zone or interface name as a person would say it:
/// `iot_cloud` is "IoT Cloud", `wan6` is "WAN6".
pub fn zone_label(name: &str) -> String {
    let words: Vec<String> = name
        .split(['_', '-'])
        .filter(|word| !word.is_empty())
        .map(word_label)
        .collect();
    if words.is_empty() {
        return name.to_string();
    }
    words.join(" ")
}

fn word_label(word: &str) -> String {
    let lower = word.to_lowercase();
    // A trailing number belongs to the word before it (wan6, lan2), so strip it,
    // read the letters, and put it back.
    let letters = lower.trim_end_matches(|c: char| c.is_ascii_digit());
    let digits = &lower[letters.len()..];
    for (acronym, label) in ACRONYMS {
        if letters == acronym {
            return format!("{label}{digits}");
        }
    }
    let mut chars = word.chars();
    match chars.next() {
        Some(first) => first.to_uppercase().collect::<String>() + chars.as_str(),
        None => String::new(),
    }
}

/// group_label names the traffic path a chain evaluates. A forwarding lane whose
/// rules all point at one zone says so; one whose rules point at several says
/// only that the traffic is forwarded, because no single destination is true of
/// the whole lane.
pub fn group_label(chain: &str, shared_dest: Option<&str>) -> String {
    let destination = || {
        shared_dest
            .map(zone_label)
            .unwrap_or_else(|| "Forwarded traffic".to_string())
    };
    if let Some(zone) = chain.strip_prefix("input_") {
        return format!("{} → Router", zone_label(zone));
    }
    if let Some(zone) = chain.strip_prefix("forward_") {
        return format!("{} → {}", zone_label(zone), destination());
    }
    if let Some(zone) = chain.strip_prefix("output_") {
        return format!("Router → {}", zone_label(zone));
    }
    if let Some(zone) = chain.strip_prefix("notrack_") {
        return format!("{} → Untracked", zone_label(zone));
    }
    if let Some(zone) = chain.strip_prefix("helper_") {
        return format!("{} → Connection helper", zone_label(zone));
    }
    if let Some(zone) = chain.strip_prefix("dstnat_") {
        return format!("{} → Redirected", zone_label(zone));
    }
    match chain {
        "input" => "Any → Router".into(),
        "forward" => format!("Any → {}", destination()),
        "output" => "Router → Any".into(),
        // Packet marking runs on a netfilter hook rather than between two zones;
        // the chain shown beside the label names which hook.
        chain if chain.starts_with("mangle_") => "Packet marking".into(),
        chain => chain.to_string(),
    }
}

/// Family is the address family a section is restricted to.
#[derive(PartialEq, Eq, Clone, Copy)]
pub enum Family {
    Any,
    V4,
    V6,
}

/// family reads firewall4's family option: a value carrying a 4 is IPv4, one
/// carrying a 6 is IPv6, and anything else leaves both families in play.
pub fn family(option: &str) -> Family {
    match option {
        "any" | "all" | "*" | "" => Family::Any,
        value if value.contains('4') || value == "inet" => Family::V4,
        value if value.contains('6') => Family::V6,
        _ => Family::Any,
    }
}

/// protocol renders the protocols a section matches, with the address family
/// beside them when it is restricted to one. An absent protocol option is tcp
/// and udp, which is what firewall4 matches. ICMP over IPv6 is a protocol of its
/// own, so it names the family itself and takes no suffix.
pub fn protocol(protos: &[String], family_option: &str) -> String {
    let family = family(family_option);
    let mut names: Vec<String> = Vec::new();
    for proto in expand(protos) {
        let name = match (proto.as_str(), family) {
            ("icmp", Family::V6) => "icmpv6".to_string(),
            _ => proto,
        };
        if !names.contains(&name) {
            names.push(name);
        }
    }
    let display = names.join("/");
    match family {
        Family::V4 => format!("{display} · v4"),
        // The v6-only ICMP protocol already says which family it is.
        Family::V6 if names.iter().all(|name| name == "icmpv6") => display,
        Family::V6 => format!("{display} · v6"),
        Family::Any => display,
    }
}

/// expand normalizes firewall4's protocol spellings — numbers, aliases, and the
/// tcpudp pair — into the names a listing shows.
fn expand(protos: &[String]) -> Vec<String> {
    let written: Vec<String> = protos.iter().map(|proto| proto.to_lowercase()).collect();
    let written = if written.is_empty() {
        vec!["tcpudp".to_string()]
    } else {
        written
    };
    let mut out = Vec::with_capacity(written.len());
    for proto in written {
        match proto.as_str() {
            "tcpudp" => out.extend(["tcp".to_string(), "udp".to_string()]),
            "1" => out.push("icmp".into()),
            "58" | "ipv6-icmp" => out.push("icmpv6".into()),
            "6" => out.push("tcp".into()),
            "17" => out.push("udp".into()),
            "all" | "any" | "*" => out.push("any".into()),
            _ => out.push(proto),
        }
    }
    out
}

/// match_summary states what a rule matches beyond its zones and protocol: the
/// destination ports it opens, the ICMP types it allows, and the rate it is held
/// to. A long ICMP type set is counted rather than listed — eleven type names is
/// a paragraph, not a cell.
pub fn match_summary(dest_ports: &[String], icmp_types: &[String], limit: &str) -> String {
    let mut parts: Vec<String> = Vec::new();
    if !dest_ports.is_empty() {
        parts.push(dest_ports.join(" "));
    }
    match icmp_types.len() {
        0 => {}
        1..=4 => parts.push(icmp_types.join(" ")),
        count => parts.push(format!("{count} types")),
    }
    if let Some(rate) = limit_label(limit) {
        parts.push(rate);
    }
    parts.join(" · ")
}

/// limit_label renders firewall4's rate limit — `1000/sec` is at most 1000 per
/// second, and an inverted limit matches only the traffic above the rate.
pub fn limit_label(limit: &str) -> Option<String> {
    let limit = limit.trim();
    let (comparison, rate) = match limit.strip_prefix('!') {
        Some(rest) => (">", rest.trim()),
        None => ("≤", limit),
    };
    if rate.is_empty() {
        return None;
    }
    let (count, unit) = match rate.split_once('/') {
        Some((count, unit)) => (count, unit),
        None => (rate, "second"),
    };
    if count.is_empty() || !count.chars().all(|c| c.is_ascii_digit()) {
        return None;
    }
    let unit = match unit.chars().next() {
        Some('s') => "s",
        Some('m') => "min",
        Some('h') => "h",
        Some('d') => "d",
        _ => return None,
    };
    Some(format!("{comparison}{count}/{unit}"))
}

/// hits renders a packet count for a column read at a glance: exact while the
/// number is small enough to mean something, scaled once it is not.
pub fn hits(packets: u64) -> String {
    for (unit, suffix) in [(1_000_000u64, 'M'), (1_000u64, 'k')] {
        if packets >= unit {
            let tenths = (packets as u128 * 10 + u128::from(unit) / 2) / u128::from(unit);
            return format!("{}.{}{}", tenths / 10, tenths % 10, suffix);
        }
    }
    packets.to_string()
}

/// target_tone is the badge tone a rule's verdict carries: what passes is
/// success, what is answered is warning, what disappears is danger. Everything
/// else changes how a packet is handled rather than whether it arrives, and
/// reads as information.
pub fn target_tone(target: &str) -> &'static str {
    match target {
        "accept" => "success",
        "reject" => "warning",
        "drop" => "danger",
        _ => "info",
    }
}

/// tone maps a badge tone's name onto the enum the settings vocabulary takes. A
/// table cell names its tone as a string and a settings pill as this enum, so
/// both read the same verdict from one place.
pub fn tone(name: &str) -> Tone {
    match name {
        "success" => Tone::Success,
        "warning" => Tone::Warning,
        "danger" => Tone::Danger,
        "info" => Tone::Info,
        _ => Tone::Neutral,
    }
}

/// subnet_label states an interface's address range as far as the config commits
/// to one: the network in CIDR when the address and mask give one, the bare
/// address when only that is configured, and nothing at all for an interface
/// that learns its address at runtime.
pub fn subnet_label(ipaddr: &str, netmask: &str, ip6assign: &str) -> Option<String> {
    let v4 = ipv4_network(ipaddr, netmask);
    match (v4, ip6assign.is_empty()) {
        (Some(network), true) => Some(network),
        (Some(network), false) => Some(format!("{network} + v6")),
        (None, true) => None,
        (None, false) => Some("v6".into()),
    }
}

/// ipv4_network masks a configured address down to its network. The prefix comes
/// from the address itself when it is written in CIDR form, otherwise from the
/// netmask; with neither, the address is all the config states.
fn ipv4_network(ipaddr: &str, netmask: &str) -> Option<String> {
    let ipaddr = ipaddr.trim();
    if ipaddr.is_empty() {
        return None;
    }
    let (address, prefix) = match ipaddr.split_once('/') {
        Some((address, prefix)) => (address, Some(prefix.parse::<u32>().ok()?)),
        None => (ipaddr, prefix_of(netmask)),
    };
    let octets = ipv4_octets(address)?;
    let Some(prefix) = prefix.filter(|prefix| *prefix <= 32) else {
        return Some(address.to_string());
    };
    let mask = if prefix == 0 {
        0
    } else {
        u32::MAX << (32 - prefix)
    };
    let network = (u32::from_be_bytes(octets) & mask).to_be_bytes();
    Some(format!(
        "{}.{}.{}.{}/{}",
        network[0], network[1], network[2], network[3], prefix
    ))
}

/// prefix_of counts the leading ones of a dotted-quad netmask. A mask with a gap
/// in it is not a prefix and is refused rather than guessed at.
fn prefix_of(netmask: &str) -> Option<u32> {
    let bits = u32::from_be_bytes(ipv4_octets(netmask.trim())?);
    let ones = bits.leading_ones();
    (bits
        == if ones == 0 {
            0
        } else {
            u32::MAX << (32 - ones)
        })
    .then_some(ones)
}

fn ipv4_octets(address: &str) -> Option<[u8; 4]> {
    let mut octets = [0u8; 4];
    let mut parts = address.split('.');
    for octet in octets.iter_mut() {
        *octet = parts.next()?.parse::<u8>().ok()?;
    }
    parts.next().is_none().then_some(octets)
}

#[cfg(test)]
mod tests {
    use super::*;

    fn strings(values: &[&str]) -> Vec<String> {
        values.iter().map(|value| value.to_string()).collect()
    }

    #[test]
    fn a_zone_name_reads_as_words_and_acronyms() {
        for (name, want) in [
            ("wan", "WAN"),
            ("lan", "LAN"),
            ("wan6", "WAN6"),
            ("lan2", "LAN2"),
            ("iot_cloud", "IoT Cloud"),
            ("iot_local", "IoT Local"),
            ("family", "Family"),
            ("guest-net", "Guest Net"),
            ("mgmt", "Mgmt"),
        ] {
            assert_eq!(zone_label(name), want);
        }
    }

    #[test]
    fn a_group_names_the_path_its_chain_evaluates() {
        assert_eq!(group_label("input_wan", None), "WAN → Router");
        assert_eq!(group_label("input_iot_cloud", None), "IoT Cloud → Router");
        assert_eq!(group_label("input", None), "Any → Router");
        assert_eq!(group_label("forward_wan", None), "WAN → Forwarded traffic");
        assert_eq!(
            group_label("forward_family", Some("iot_local")),
            "Family → IoT Local"
        );
        assert_eq!(group_label("output_lan", None), "Router → LAN");
        assert_eq!(group_label("output", None), "Router → Any");
        assert_eq!(group_label("notrack_lan", None), "LAN → Untracked");
        assert_eq!(group_label("mangle_prerouting", None), "Packet marking");
        assert_eq!(group_label("dstnat_wan", None), "WAN → Redirected");
    }

    #[test]
    fn an_absent_protocol_is_the_pair_firewall4_matches() {
        assert_eq!(protocol(&[], ""), "tcp/udp");
        assert_eq!(protocol(&strings(&["tcp", "udp"]), ""), "tcp/udp");
        assert_eq!(protocol(&strings(&["tcpudp"]), ""), "tcp/udp");
    }

    #[test]
    fn a_protocol_carries_the_family_unless_it_names_one() {
        assert_eq!(protocol(&strings(&["udp"]), "ipv4"), "udp · v4");
        assert_eq!(protocol(&strings(&["udp"]), "ipv6"), "udp · v6");
        assert_eq!(protocol(&strings(&["icmp"]), "ipv4"), "icmp · v4");
        assert_eq!(protocol(&strings(&["icmp"]), "ipv6"), "icmpv6");
        assert_eq!(protocol(&strings(&["ipv6-icmp"]), ""), "icmpv6");
        assert_eq!(protocol(&strings(&["esp"]), ""), "esp");
        assert_eq!(protocol(&strings(&["1"]), "ipv4"), "icmp · v4");
        assert_eq!(protocol(&strings(&["all"]), ""), "any");
    }

    #[test]
    fn a_match_states_ports_types_and_rate() {
        assert_eq!(match_summary(&strings(&["68"]), &[], ""), "68");
        assert_eq!(
            match_summary(&strings(&["53", "67", "547"]), &[], ""),
            "53 67 547"
        );
        assert_eq!(
            match_summary(&[], &strings(&["echo-request"]), ""),
            "echo-request"
        );
        assert_eq!(
            match_summary(&[], &strings(&["130", "131", "132", "143"]), ""),
            "130 131 132 143"
        );
        let eleven = strings(&["a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k"]);
        assert_eq!(
            match_summary(&[], &eleven, "1000/sec"),
            "11 types · ≤1000/s"
        );
        assert_eq!(match_summary(&[], &[], ""), "");
    }

    #[test]
    fn a_rate_limit_states_its_comparison_and_unit() {
        assert_eq!(limit_label("1000/sec"), Some("≤1000/s".into()));
        assert_eq!(limit_label("10/minute"), Some("≤10/min".into()));
        assert_eq!(limit_label("5/hour"), Some("≤5/h".into()));
        assert_eq!(limit_label("2/day"), Some("≤2/d".into()));
        assert_eq!(limit_label("25"), Some("≤25/s".into()));
        assert_eq!(limit_label("!100/sec"), Some(">100/s".into()));
        assert_eq!(limit_label(""), None);
        assert_eq!(limit_label("burst/sec"), None);
        assert_eq!(limit_label("10/fortnight"), None);
    }

    #[test]
    fn a_hit_count_is_exact_while_it_is_small() {
        for (packets, want) in [
            (0u64, "0"),
            (4, "4"),
            (196, "196"),
            (999, "999"),
            (1_400, "1.4k"),
            (29_300, "29.3k"),
            (65_500, "65.5k"),
            (93_450, "93.5k"),
            (1_000_000, "1.0M"),
            (2_450_000, "2.5M"),
        ] {
            assert_eq!(hits(packets), want, "{packets}");
        }
    }

    #[test]
    fn a_verdict_carries_its_tone() {
        assert_eq!(target_tone("accept"), "success");
        assert_eq!(target_tone("reject"), "warning");
        assert_eq!(target_tone("drop"), "danger");
        for target in ["notrack", "helper", "mark", "dscp"] {
            assert_eq!(target_tone(target), "info");
        }
    }

    #[test]
    fn a_subnet_is_derived_only_where_the_config_states_one() {
        assert_eq!(
            subnet_label("192.168.77.1", "255.255.255.0", "60"),
            Some("192.168.77.0/24 + v6".into())
        );
        assert_eq!(
            subnet_label("172.20.0.10", "255.255.0.0", ""),
            Some("172.20.0.0/16".into())
        );
        assert_eq!(
            subnet_label("10.0.31.5/24", "", ""),
            Some("10.0.31.0/24".into())
        );
        // Configured address, no mask: the address is all the config commits to.
        assert_eq!(
            subnet_label("193.77.189.155", "", ""),
            Some("193.77.189.155".into())
        );
        // A dhcp uplink states nothing to derive.
        assert_eq!(subnet_label("", "", ""), None);
        assert_eq!(subnet_label("", "", "60"), Some("v6".into()));
        // A netmask with a gap is not a prefix.
        assert_eq!(
            subnet_label("10.0.0.1", "255.0.255.0", ""),
            Some("10.0.0.1".into())
        );
    }
}
