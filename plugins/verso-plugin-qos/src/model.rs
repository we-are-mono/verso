// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

//! One device's policy, held as the two configs that carry it.
//!
//! Keeping a device off the internet is a firewall rule, because a firewall rule
//! is what stops a packet: fw4 matches on the source MAC and refuses the route
//! out, on a schedule if one is set. Capping a device's speed is not a firewall
//! rule at all — a rate is a queueing discipline on the bridge, and no rule can
//! express one — so the caps live in a config of this plugin's own.
//!
//! Both halves are the same decision to the person making it, which is why they
//! are one tab; they are two files because that is honestly where each belongs,
//! and the tab says so rather than hiding it.

use verso_plugin::{Section, Snapshot};

/// FIREWALL is where a device's access rule lives, and RULE its section type.
pub const FIREWALL: &str = "firewall";
pub const RULE: &str = "rule";

/// CAPS is this plugin's own config, and DEVICE the section type one device's
/// caps are written as.
pub const CAPS: &str = "qos";
pub const DEVICE: &str = "device";

/// WAN is the destination an access rule refuses: the route out, and nothing
/// else. A device kept off the internet still reaches everything on its own
/// network, which is the whole point of refusing this one destination rather
/// than turning the device off.
pub const WAN: &str = "wan";

/// ANY_ZONE is the source an access rule matches from. fw4 reads `*` as any
/// zone, so the rule follows the device: one that moves from the trusted network
/// to the guest network is still the device the operator said to keep off.
pub const ANY_ZONE: &str = "*";

/// DAYS is the week in the order fw4 writes and reads it.
pub const DAYS: [&str; 7] = ["Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"];

/// CURFEW_FROM and CURFEW_TO are where a schedule opens when someone turns one
/// on and states no hours — an evening curfew, the thing schedules are mostly
/// for. They are a starting point in an editable field, never a written default.
pub const CURFEW_FROM: &str = "21:00";
pub const CURFEW_TO: &str = "07:00";

/// Policy is everything this plugin says about one device.
#[derive(Clone, Debug, PartialEq)]
pub struct Policy {
    pub mac: String,
    /// The uci sections the two halves are written in, or "" where the config
    /// holds none yet — the handle a save edits rather than adds.
    pub rule: String,
    pub caps: String,
    /// Whether the device may reach the internet. A device that may not is
    /// refused outright; one that may can still be refused between hours.
    pub allowed: bool,
    pub scheduled: bool,
    pub days: Vec<String>,
    pub from: String,
    pub to: String,
    /// Megabits per second, or "" for no cap in that direction.
    pub down: String,
    pub up: String,
}

impl Policy {
    /// blank is a device nothing has been said about: on the internet, at full
    /// speed, with the hours a curfew would start from waiting in the fields.
    pub fn blank(mac: &str) -> Policy {
        Policy {
            mac: mac.to_string(),
            rule: String::new(),
            caps: String::new(),
            allowed: true,
            scheduled: false,
            days: DAYS.iter().map(|day| day.to_string()).collect(),
            from: CURFEW_FROM.into(),
            to: CURFEW_TO.into(),
            down: String::new(),
            up: String::new(),
        }
    }

    /// read is what the config says about one device right now. An access rule
    /// carrying hours is a curfew; one without is a flat refusal. A rule written
    /// by hand reads exactly as one written here, because it is the same rule —
    /// this plugin owns the shape, not the authorship.
    pub fn read(snapshot: &Snapshot, mac: &str) -> Policy {
        let mut policy = Policy::blank(mac);
        if let Some(rule) = access_rule(snapshot, mac) {
            policy.rule = rule.name();
            let days = option_list(&rule, "weekdays");
            let from = rule.scalar("start_time");
            let to = rule.scalar("stop_time");
            policy.scheduled = !from.is_empty() || !to.is_empty() || !days.is_empty();
            policy.allowed = policy.scheduled;
            if policy.scheduled {
                if !days.is_empty() {
                    policy.days = days;
                }
                if !from.is_empty() {
                    policy.from = from;
                }
                if !to.is_empty() {
                    policy.to = to;
                }
            }
        }
        if let Some(caps) = device_caps(snapshot, mac) {
            policy.caps = caps.name();
            policy.down = caps.scalar("download");
            policy.up = caps.scalar("upload");
        }
        policy
    }

    /// stated reports whether anything has been said about this device — whether
    /// it belongs in the listing of devices under a policy at all.
    pub fn stated(&self) -> bool {
        !self.allowed || self.scheduled || self.capped()
    }

    /// capped reports whether either direction carries a speed limit.
    pub fn capped(&self) -> bool {
        !self.down.is_empty() || !self.up.is_empty()
    }

    /// refuses reports whether the access rule exists at all: a device kept off
    /// the internet, or one kept off it between hours.
    pub fn refuses(&self) -> bool {
        !self.allowed || self.scheduled
    }
}

/// access_rule is the rule that keeps one device off the internet, if the config
/// holds one: a refusal, matched on this device's MAC, aimed at the route out.
/// Those three together are what the rule *is*, so a rule Verso did not write is
/// found by the same test as one it did.
pub fn access_rule<'a>(snapshot: &'a Snapshot, mac: &str) -> Option<Section<'a>> {
    snapshot
        .sections_of_type(FIREWALL, RULE)
        .into_iter()
        .find(|rule| {
            rule.scalar("dest") == WAN
                && refusal(&rule.scalar("target"))
                && option_list(rule, "src_mac")
                    .iter()
                    .any(|listed| same_mac(listed, mac))
        })
}

/// device_caps is the section holding one device's speed limits, if any.
pub fn device_caps<'a>(snapshot: &'a Snapshot, mac: &str) -> Option<Section<'a>> {
    snapshot
        .sections_of_type(CAPS, DEVICE)
        .into_iter()
        .find(|caps| same_mac(&caps.scalar("mac"), mac))
}

/// every_policy is each device the two configs say something about, MAC by MAC,
/// so a listing can read the whole arrangement without being handed a roster.
pub fn every_policy(snapshot: &Snapshot) -> Vec<Policy> {
    let mut macs: Vec<String> = Vec::new();
    let seen = |macs: &mut Vec<String>, mac: String| {
        if !mac.is_empty() && !macs.iter().any(|held| same_mac(held, &mac)) {
            macs.push(mac);
        }
    };
    for rule in snapshot.sections_of_type(FIREWALL, RULE) {
        if rule.scalar("dest") == WAN && refusal(&rule.scalar("target")) {
            for mac in option_list(&rule, "src_mac") {
                seen(&mut macs, mac);
            }
        }
    }
    for caps in snapshot.sections_of_type(CAPS, DEVICE) {
        seen(&mut macs, caps.scalar("mac"));
    }
    macs.iter()
        .map(|mac| Policy::read(snapshot, mac))
        .filter(Policy::stated)
        .collect()
}

/// refusal reports whether a target stops the packet. fw4 spells a refusal two
/// ways — one answers, one does not — and either is a device kept off the
/// internet as far as this plugin is concerned.
fn refusal(target: &str) -> bool {
    let target = target.to_ascii_uppercase();
    target == "REJECT" || target == "DROP"
}

/// option_list reads an option written either as a uci list or as one
/// whitespace-separated string; uci accepts both and configs use both.
pub fn option_list(section: &Section, option: &str) -> Vec<String> {
    let listed = section.list(option);
    if !listed.is_empty() {
        return listed;
    }
    section
        .scalar(option)
        .split_whitespace()
        .map(String::from)
        .collect()
}

/// same_mac compares two MACs as the daemons do: case and the separator are
/// spelling, not identity.
pub fn same_mac(left: &str, right: &str) -> bool {
    let strip = |mac: &str| {
        mac.chars()
            .filter(|c| c.is_ascii_hexdigit())
            .map(|c| c.to_ascii_lowercase())
            .collect::<String>()
    };
    !left.is_empty() && strip(left) == strip(right)
}
