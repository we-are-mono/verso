// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

//! Turning the config's arithmetic into what a person came to read.
//!
//! Two things here are the point of the DHCP page. A pool is written as an
//! offset and a count — `start 100`, `limit 150` — which says nothing about
//! which addresses a device will get; the page states the span itself, computed
//! exactly the way dnsmasq's init script computes it (`ipcalc.sh`), so what is
//! shown is what will be handed out. And a lease is written as the epoch second
//! it runs out at; the page says how long that is from now.

use std::time::{SystemTime, UNIX_EPOCH};

/// EM_DASH stands for a value the config or the device does not state.
pub const EM_DASH: &str = "—";

/// DEFAULT_START and DEFAULT_LIMIT are dnsmasq's own pool defaults, applied when
/// the section states neither — the same numbers its init script reads.
pub const DEFAULT_START: u32 = 100;
pub const DEFAULT_LIMIT: u32 = 150;

/// Subnet is one interface's IPv4 network: the address the router holds on it,
/// the network itself, and the prefix — everything a pool is measured against.
#[derive(Clone, Copy)]
pub struct Subnet {
    pub address: u32,
    pub network: u32,
    pub prefix: u32,
}

impl Subnet {
    /// read derives the subnet an interface commits to. An interface that learns
    /// its address at runtime commits to none, and nothing is guessed for it.
    pub fn read(ipaddr: &str, netmask: &str) -> Option<Subnet> {
        let ipaddr = ipaddr.trim();
        let (address, prefix) = match ipaddr.split_once('/') {
            Some((address, prefix)) => (address, prefix.parse::<u32>().ok()?),
            None => (ipaddr, prefix_of(netmask)?),
        };
        if prefix > 32 {
            return None;
        }
        let address = u32::from_be_bytes(octets(address)?);
        let mask = mask_of(prefix);
        Some(Subnet {
            address,
            network: address & mask,
            prefix,
        })
    }

    /// cidr states the network the way an operator writes it.
    pub fn cidr(&self) -> String {
        format!("{}/{}", address_label(self.network), self.prefix)
    }

    /// holds reports whether an address sits on this network — how a lease finds
    /// the pool it came from.
    pub fn holds(&self, addr: &str) -> bool {
        octets(addr.trim())
            .map(|octets| u32::from_be_bytes(octets) & mask_of(self.prefix) == self.network)
            .unwrap_or(false)
    }

    /// range is the span of addresses this pool hands out, computed the way
    /// dnsmasq's init script does: the offset is masked into the network, held
    /// inside the usable range, and never lands on the router's own address.
    /// None means the network is too small to hand anything out.
    pub fn range(&self, start: u32, limit: u32) -> Option<(String, String)> {
        let hostmask = !mask_of(self.prefix);
        // A network large enough to have them keeps its own address and its
        // broadcast out of the pool; a /31 has neither, and a /32 is one address.
        let (lower, upper) = match self.prefix {
            0..=30 => (
                self.network.saturating_add(1),
                (self.network | hostmask).saturating_sub(1),
            ),
            31 => (self.network, self.network | hostmask),
            _ => (self.network, self.network),
        };
        let mut first = (self.network | (start & hostmask)).max(lower);
        if first == self.address {
            first = first.saturating_add(1);
        }
        let mut last = first.saturating_add(limit.saturating_sub(1)).min(upper);
        if last == self.address {
            last = last.saturating_sub(1);
        }
        (first <= last).then(|| (address_label(first), address_label(last)))
    }
}

/// offset reads a pool's `start` or `limit`. dnsmasq accepts a plain count or a
/// whole address for `start`, folding the address to its numeric value, so both
/// are read here the same way it reads them.
pub fn offset(written: &str, fallback: u32) -> u32 {
    let written = written.trim();
    if written.is_empty() {
        return fallback;
    }
    if let Some(octets) = octets(written) {
        return u32::from_be_bytes(octets);
    }
    written.parse::<u32>().unwrap_or(fallback)
}

/// range_label is the pool's span as one line, or a plain statement when the
/// network the pool sits on states no addresses to hand out.
pub fn range_label(subnet: Option<Subnet>, start: &str, limit: &str) -> String {
    let Some(subnet) = subnet else {
        return EM_DASH.to_string();
    };
    match subnet.range(offset(start, DEFAULT_START), offset(limit, DEFAULT_LIMIT)) {
        Some((first, last)) => format!("{first} – {last}"),
        None => EM_DASH.to_string(),
    }
}

/// expires_in says how long a lease has left, the way a person would. A lease
/// whose moment has passed is stated as expired rather than as a negative time.
pub fn expires_in(expires_at: i64) -> String {
    expires_from(expires_at, now())
}

/// expires_from is expires_in against a stated clock, which is what makes the
/// wording testable.
pub fn expires_from(expires_at: i64, now: i64) -> String {
    // dnsmasq writes 0 for a lease that never runs out (a static one it serves).
    if expires_at == 0 {
        return "never".to_string();
    }
    let left = expires_at - now;
    if left <= 0 {
        return "expired".to_string();
    }
    let (hours, minutes) = (left / 3600, (left % 3600) / 60);
    match hours {
        0 => format!("{minutes}m"),
        _ => format!("{hours}h {minutes}m"),
    }
}

/// now is the device's clock in epoch seconds; a clock before the epoch is not
/// one a lease can be measured against, and reads as zero.
fn now() -> i64 {
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .map(|since| since.as_secs() as i64)
        .unwrap_or(0)
}

/// device_name is what a lease is called. A device that offered no name gets a
/// quiet stand-in from its MAC tail rather than a full MAC in the name column.
pub fn device_name(hostname: &str, mac: &str) -> String {
    if !hostname.is_empty() {
        return hostname.to_string();
    }
    let pairs: Vec<&str> = mac.split(':').collect();
    match pairs.len() >= 2 {
        true => format!("Device {}", pairs[pairs.len() - 2..].join(":")),
        false => "Device".to_string(),
    }
}

/// devices_label counts what is on a network right now, in words rather than in
/// a bare number — a card's meta line is read, not scanned.
pub fn devices_label(count: usize) -> String {
    match count {
        0 => "nobody right now".to_string(),
        1 => "1 device right now".to_string(),
        count => format!("{count} devices right now"),
    }
}

/// points_to is what a record answers with, as one line: the address or name it
/// resolves to, and the number that qualifies it where the kind has one.
pub fn points_to(target: &str, qualifier: &str) -> String {
    match (target.is_empty(), qualifier.is_empty()) {
        (true, _) => EM_DASH.to_string(),
        (false, true) => target.to_string(),
        (false, false) => format!("{target} · {qualifier}"),
    }
}

fn address_label(value: u32) -> String {
    let octets = value.to_be_bytes();
    format!("{}.{}.{}.{}", octets[0], octets[1], octets[2], octets[3])
}

fn mask_of(prefix: u32) -> u32 {
    match prefix {
        0 => 0,
        prefix => u32::MAX << (32 - prefix),
    }
}

/// prefix_of counts the leading ones of a dotted-quad netmask. A mask with a gap
/// in it is not a prefix and is refused rather than guessed at.
fn prefix_of(netmask: &str) -> Option<u32> {
    let bits = u32::from_be_bytes(octets(netmask.trim())?);
    let ones = bits.leading_ones();
    (bits == mask_of(ones)).then_some(ones)
}

fn octets(address: &str) -> Option<[u8; 4]> {
    Some(address.parse::<std::net::Ipv4Addr>().ok()?.octets())
}

#[cfg(test)]
mod tests {
    use super::*;

    fn subnet(ipaddr: &str, netmask: &str) -> Subnet {
        Subnet::read(ipaddr, netmask).expect("subnet")
    }

    #[test]
    fn a_subnet_is_derived_only_where_the_config_states_one() {
        assert_eq!(subnet("10.0.0.1", "255.255.255.0").cidr(), "10.0.0.0/24");
        assert_eq!(subnet("192.168.77.1/24", "").cidr(), "192.168.77.0/24");
        assert_eq!(subnet("172.20.0.10", "255.255.0.0").cidr(), "172.20.0.0/16");
        // An uplink that learns its address states nothing to derive.
        assert!(Subnet::read("", "").is_none());
        assert!(Subnet::read("10.0.0.1", "").is_none());
        // A netmask with a gap in it is not a prefix.
        assert!(Subnet::read("10.0.0.1", "255.0.255.0").is_none());
    }

    #[test]
    fn a_pool_states_the_addresses_it_hands_out() {
        let lan = subnet("10.0.0.1", "255.255.255.0");
        assert_eq!(
            lan.range(100, 150),
            Some(("10.0.0.100".into(), "10.0.0.249".into()))
        );
        // The count is clamped to what the network holds, never past its
        // broadcast address.
        assert_eq!(
            lan.range(100, 500),
            Some(("10.0.0.100".into(), "10.0.0.254".into()))
        );
        // An offset below the network's first usable address is lifted to it,
        // and never lands on the router itself.
        assert_eq!(
            lan.range(0, 10),
            Some(("10.0.0.2".into(), "10.0.0.11".into()))
        );
        // A start written as a whole address is folded into this network.
        assert_eq!(offset("10.0.0.50", DEFAULT_START), 167772210);
        assert_eq!(
            lan.range(offset("10.0.0.50", DEFAULT_START), 10),
            Some(("10.0.0.50".into(), "10.0.0.59".into()))
        );
        // A /30 holds one address the router is not already using, a /31 its one
        // peer, and a /32 nothing at all.
        assert_eq!(
            subnet("10.9.9.1", "255.255.255.252").range(1, 150),
            Some(("10.9.9.2".into(), "10.9.9.2".into()))
        );
        assert_eq!(
            subnet("10.9.9.0", "255.255.255.254").range(1, 150),
            Some(("10.9.9.1".into(), "10.9.9.1".into()))
        );
        assert_eq!(subnet("10.9.9.1", "255.255.255.255").range(1, 150), None);
        // The far end of the address space computes rather than panics.
        assert_eq!(
            subnet("255.255.255.255", "255.255.255.255").range(u32::MAX, u32::MAX),
            None
        );
    }

    #[test]
    fn a_pools_defaults_are_dnsmasqs_own() {
        assert_eq!(offset("", DEFAULT_START), 100);
        assert_eq!(offset("", DEFAULT_LIMIT), 150);
        assert_eq!(offset("not a number", DEFAULT_LIMIT), 150);
        assert_eq!(
            range_label(Subnet::read("10.0.10.1", "255.255.255.0"), "", ""),
            "10.0.10.100 – 10.0.10.249"
        );
        assert_eq!(range_label(None, "100", "150"), EM_DASH);
    }

    #[test]
    fn a_subnet_knows_which_addresses_are_on_it() {
        let lan = subnet("10.0.0.1", "255.255.255.0");
        assert!(lan.holds("10.0.0.142"));
        assert!(!lan.holds("10.0.10.142"));
        assert!(!lan.holds("2a00:ee2::1"));
        assert!(!lan.holds(""));
    }

    #[test]
    fn a_lease_says_how_long_it_has_left() {
        let now = 1_787_800_000;
        assert_eq!(expires_from(now + 9 * 3600 + 14 * 60, now), "9h 14m");
        assert_eq!(expires_from(now + 42 * 60, now), "42m");
        assert_eq!(expires_from(now, now), "expired");
        assert_eq!(expires_from(now - 60, now), "expired");
        assert_eq!(expires_from(0, now), "never");
    }

    #[test]
    fn a_nameless_device_is_still_called_something() {
        assert_eq!(device_name("nas", "30:9c:23:5e:88:01"), "nas");
        assert_eq!(device_name("", "30:9c:23:5e:88:01"), "Device 88:01");
        assert_eq!(device_name("", ""), "Device");
    }

    #[test]
    fn a_count_reads_as_words() {
        assert_eq!(devices_label(0), "nobody right now");
        assert_eq!(devices_label(1), "1 device right now");
        assert_eq!(devices_label(4), "4 devices right now");
    }

    #[test]
    fn a_record_states_what_qualifies_its_answer() {
        assert_eq!(points_to("nas.lan", "8448"), "nas.lan · 8448");
        assert_eq!(points_to("10.0.0.31", ""), "10.0.0.31");
        assert_eq!(points_to("", "8448"), EM_DASH);
    }
}
