// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

//! What the drawers share: how a submission says which drawer it came from, how
//! a refusal is reported, and what a value has to look like to be written.
//!
//! Three small objects are edited in drawers here — a name record, an upstream,
//! a reservation — and each posts a form of its own, so every submission names
//! its kind and its subject. A page carries all of them at once, and the page
//! form carries none of them, so the discriminator is what tells a save from a
//! save of something else.
//!
//! Validation is the daemons', not a looser echo: dnsmasq and odhcpd drop a
//! section they cannot parse and say nothing about it, so a value they would
//! refuse is refused here, on the control that carries it, before it is written.

use std::collections::BTreeMap;

use verso_plugin::{Form, RowDrawer, SelectOption, Widget};

/// KIND is the field naming which drawer a submission came from, and SECTION,
/// INDEX and DELETE are what it is about: a uci section, a position in a list,
/// and whether the submission removes it rather than saves it.
pub const KIND: &str = "_form";
pub const SECTION: &str = "_section";
pub const INDEX: &str = "_index";
pub const DELETE: &str = "_delete";

/// RECORD, SERVER and HOST are the three kinds.
pub const RECORD: &str = "record";
pub const SERVER: &str = "server";
pub const HOST: &str = "host";

/// kind is which drawer a submission came from, or None when it came from the
/// page form — which carries no discriminator, because it is the page.
pub fn kind(form: &Form) -> Option<String> {
    let kind = form.get(KIND);
    (!kind.is_empty()).then_some(kind)
}

/// deletes reports whether a submission removes its subject.
pub fn deletes(form: &Form) -> bool {
    form.get(DELETE) == "1"
}

/// Errors is what a submission got wrong, addressed to the controls carrying the
/// offending values. The shell reads the annotations back off the re-rendered
/// tree, so a form that reports one is a 422 and its write is blocked.
#[derive(Default)]
pub struct Errors(BTreeMap<String, String>);

impl Errors {
    /// check records a message against a field when the value is not one the
    /// daemon accepts.
    pub fn check(&mut self, name: &str, ok: bool, message: &str) {
        if !ok {
            self.0
                .entry(name.to_string())
                .or_insert_with(|| message.to_string());
        }
    }

    /// get is the message for one field, or "" — what a field widget carries.
    pub fn get(&self, name: &str) -> &str {
        self.0.get(name).map(String::as_str).unwrap_or("")
    }

    pub fn is_empty(&self) -> bool {
        self.0.is_empty()
    }
}

/// REFUSED is what a drawer says when it wrote nothing. The fields say which
/// values were wrong; the notice says that nothing happened.
pub const REFUSED: &str =
    "Some values aren’t ones the daemon accepts, so nothing was saved. They’re marked in the panel.";

/// UNKNOWN is what a page says about a submission it did not draw.
pub const UNKNOWN: &str = "Verso couldn’t tell what that change was, so nothing was saved.";

/// text_field is one typed value, carrying whatever the last submission got
/// wrong about it.
pub fn text_field(name: &str, label: &str, value: &str, help: &str, errors: &Errors) -> Widget {
    Widget::Field {
        name: name.into(),
        label: label.into(),
        kind: "text".into(),
        advanced: false,
        value: value.into(),
        values: Vec::new(),
        placeholder: String::new(),
        datatype: String::new(),
        options: Vec::new(),
        error: errors.get(name).into(),
        help: help.into(),
    }
}

/// select_field is one choice over a closed set.
pub fn select_field(
    name: &str,
    label: &str,
    value: &str,
    options: Vec<SelectOption>,
    errors: &Errors,
) -> Widget {
    Widget::select(name, label, value, options, errors.get(name))
}

/// choices builds a select's option set from value/label pairs.
pub fn choices(pairs: &[(&str, &str)]) -> Vec<SelectOption> {
    pairs
        .iter()
        .map(|(value, label)| SelectOption::new(value, label))
        .collect()
}

/// drawer is a row's edit surface: the object's own form, and — where the object
/// already exists — the confirm that removes it, in a form of its own so the
/// editor above can never carry the delete by accident. Open puts a refused
/// submission back in front of the operator rather than closing on it.
pub fn drawer(title: &str, open: bool, children: Vec<Widget>) -> Option<RowDrawer> {
    Some(RowDrawer {
        title: title.into(),
        size: String::new(),
        hide_title: false,
        open,
        children,
    })
}

/// delete_form is one drawer's irreversible action: the discriminators that name
/// the subject, and a confirm that carries the submit — so the form draws no
/// button of its own.
pub fn delete_form(kind: &str, subject: (&str, &str), trigger: &str, message: &str) -> Widget {
    Widget::Form {
        style: String::new(),
        submit: String::new(),
        error: String::new(),
        fields: vec![
            Widget::hidden(KIND, kind),
            Widget::hidden(subject.0, subject.1),
            Widget::hidden(DELETE, "1"),
            Widget::Confirm {
                trigger: trigger.into(),
                message: message.into(),
                confirm: trigger.into(),
                cancel: String::new(),
            },
        ],
    }
}

// ---- what the daemons accept ----

/// valid_macs accepts one or more MAC addresses: uci writes several onto one
/// option to follow a device between docks, and dnsmasq reads them all.
pub fn valid_macs(value: &str) -> bool {
    let macs: Vec<&str> = value.split_whitespace().collect();
    !macs.is_empty() && macs.iter().all(|mac| valid_mac(mac))
}

fn valid_mac(value: &str) -> bool {
    let pairs: Vec<&str> = value.split([':', '-']).collect();
    pairs.len() == 6
        && pairs
            .iter()
            .all(|pair| pair.len() == 2 && pair.chars().all(|c| c.is_ascii_hexdigit()))
}

/// valid_ipv4 and valid_ipv6 read an address's shape, which is what the daemons
/// do — the registry it belongs to is not theirs to know either.
pub fn valid_ipv4(value: &str) -> bool {
    let octets: Vec<&str> = value.split('.').collect();
    octets.len() == 4 && octets.iter().all(|octet| octet.parse::<u8>().is_ok())
}

pub fn valid_ipv6(value: &str) -> bool {
    if !value.contains(':') || value.matches("::").count() > 1 {
        return false;
    }
    let (head, tail) = match value.rsplit_once(':') {
        Some((head, tail)) if tail.contains('.') => (head, Some(tail)),
        _ => (value, None),
    };
    if tail.is_some_and(|tail| !valid_ipv4(tail)) {
        return false;
    }
    let groups: Vec<&str> = head.split(':').collect();
    let limit = if tail.is_some() { 6 } else { 8 };
    groups.iter().filter(|group| !group.is_empty()).count() <= limit
        && groups
            .iter()
            .all(|group| group.len() <= 4 && group.chars().all(|c| c.is_ascii_hexdigit()))
}

/// valid_hostname accepts a bare name or a dotted one: letters, digits, hyphens
/// and underscores per label, and a label that is neither empty nor overlong.
pub fn valid_hostname(value: &str) -> bool {
    let labels: Vec<&str> = value.trim_end_matches('.').split('.').collect();
    !value.is_empty()
        && labels.iter().all(|label| {
            !label.is_empty()
                && label.len() <= 63
                && label
                    .chars()
                    .all(|c| c.is_ascii_alphanumeric() || c == '-' || c == '_')
        })
}

/// valid_number accepts a whole number within a bound — a port, a priority, a
/// cache size.
pub fn valid_number(value: &str, max: u32) -> bool {
    value.parse::<u32>().is_ok_and(|number| number <= max)
}

/// valid_hostid accepts the IPv6 interface part a reservation pins, written the
/// way uci writes it: bare hex, with or without the `::` an operator types.
pub fn valid_hostid(value: &str) -> bool {
    let digits = value.trim_start_matches(':');
    !digits.is_empty()
        && digits.len() <= 16
        && digits.chars().all(|c| c.is_ascii_hexdigit() || c == ':')
}

/// valid_leasetime accepts what dnsmasq reads as a lease length: a count of
/// seconds, a count with a unit, or forever.
pub fn valid_leasetime(value: &str) -> bool {
    if value.eq_ignore_ascii_case("infinite") {
        return true;
    }
    let digits = value.trim_end_matches(['s', 'm', 'h', 'd', 'w', 'S', 'M', 'H', 'D', 'W']);
    !digits.is_empty()
        && value.len() - digits.len() <= 1
        && digits.chars().all(|c| c.is_ascii_digit())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn a_submission_names_the_drawer_it_came_from() {
        let record = Form::parse("_form=record&_section=cfg09&fqdn=nas.lan");
        assert_eq!(kind(&record).as_deref(), Some("record"));
        assert!(!deletes(&record));
        assert!(deletes(&Form::parse("_form=host&_section=nas&_delete=1")));
        // The page form carries no discriminator: it is the page.
        assert!(kind(&Form::parse("expandhosts=1&cachesize=1000")).is_none());
    }

    #[test]
    fn a_refusal_is_addressed_to_the_control_that_carries_it() {
        let mut errors = Errors::default();
        assert!(errors.is_empty());
        errors.check("mac", false, "Write a MAC address.");
        errors.check("mac", false, "Something else.");
        errors.check("ip", true, "Write an address.");
        assert_eq!(errors.get("mac"), "Write a MAC address.");
        assert_eq!(errors.get("ip"), "");
        assert!(!errors.is_empty());
    }

    #[test]
    fn the_daemons_own_acceptance_is_what_a_value_is_checked_against() {
        for value in ["00:11:22:33:44:55", "AA-BB-CC-DD-EE-FF", "00:11:22:33:44:55 aa:bb:cc:dd:ee:ff"] {
            assert!(valid_macs(value), "{value}");
        }
        for value in ["", "00:11:22:33:44", "gg:11:22:33:44:55", "001122334455"] {
            assert!(!valid_macs(value), "{value}");
        }

        for value in ["10.0.0.30", "0.0.0.0", "255.255.255.255"] {
            assert!(valid_ipv4(value), "{value}");
        }
        for value in ["", "10.0.0.256", "10.0.0", "2a00::1"] {
            assert!(!valid_ipv4(value), "{value}");
        }

        for value in ["::1", "2a00:ee2:2d00:2e00::30", "fe80::1%0", "::ffff:10.0.0.1"] {
            assert_eq!(valid_ipv6(value), !value.contains('%'), "{value}");
        }
        for value in ["", "10.0.0.1", "2a00::ee2::1", "zzzz::1"] {
            assert!(!valid_ipv6(value), "{value}");
        }

        for value in ["nas", "nas.lan", "_matrix._tcp.lan", "corp.example.com."] {
            assert!(valid_hostname(value), "{value}");
        }
        for value in ["", "nas..lan", "nas lan", "na$"] {
            assert!(!valid_hostname(value), "{value}");
        }

        assert!(valid_number("8448", 65535));
        assert!(!valid_number("70000", 65535));
        assert!(!valid_number("", 65535));
        assert!(!valid_number("-1", 65535));

        for value in ["::30", "30", "a1b2"] {
            assert!(valid_hostid(value), "{value}");
        }
        for value in ["", "::zz", "::30::30::30::30::30"] {
            assert!(!valid_hostid(value), "{value}");
        }

        for value in ["12h", "2h", "30m", "infinite", "3600", "7d"] {
            assert!(valid_leasetime(value), "{value}");
        }
        for value in ["", "12hh", "twelve", "12 h"] {
            assert!(!valid_leasetime(value), "{value}");
        }
    }
}
