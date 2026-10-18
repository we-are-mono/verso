// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

//! Argument kinds (ADR-027): the closed set an act's arguments are declared
//! in. The privileged helper checks every argument against its kind before a
//! root command runs; a plugin can check a form value the same way before it
//! names the act. Every value arrives as text, as a form posts it.
//!
//! `testdata/argument-kinds.json` holds accepted and refused samples for every
//! kind. This crate's tests, the helper's and the shell's datatype tests all
//! read it, so the three checks cannot drift apart.

use serde::{Deserialize, Serialize};
use std::net::{Ipv4Addr, Ipv6Addr};

/// The longest text an argument may be declared to take.
pub const MAX_TEXT: usize = 65536;

/// What one argument may hold.
#[derive(Serialize, Deserialize, Debug, Clone, PartialEq)]
#[serde(tag = "kind", rename_all = "kebab-case")]
pub enum ArgKind {
    /// A uci section or option name: 1–64 of letters, digits and `_`.
    UciName,
    /// A host name: up to 253 characters of dot-separated labels.
    Hostname,
    Ipv4,
    /// An IPv6 address, without a zone and not an IPv4-mapped one.
    Ipv6,
    /// An IPv4 or IPv6 address.
    Ip,
    /// 1–65535, in plain digits.
    Port,
    /// A whole number from `min` to `max`, in plain digits with an optional `-`.
    Int { min: i64, max: i64 },
    /// One of `values`, exactly.
    Enum { values: Vec<String> },
    /// `0` or `1`, as uci writes a boolean.
    Bool,
    /// Any text of at most `max` bytes without a NUL.
    Text { max: usize },
}

impl ArgKind {
    /// accepts reports whether value is one this kind allows.
    pub fn accepts(&self, value: &str) -> bool {
        match self {
            ArgKind::UciName => {
                !value.is_empty()
                    && value.len() <= 64
                    && value.bytes().all(|b| b.is_ascii_alphanumeric() || b == b'_')
            }
            ArgKind::Hostname => hostname(value),
            ArgKind::Ipv4 => ipv4(value),
            ArgKind::Ipv6 => ipv6(value),
            ArgKind::Ip => ipv4(value) || ipv6(value),
            ArgKind::Port => digits(value) && value.parse::<u32>().is_ok_and(|n| (1..=65535).contains(&n)),
            ArgKind::Int { min, max } => {
                let unsigned = value.strip_prefix('-').unwrap_or(value);
                digits(unsigned) && value.parse::<i64>().is_ok_and(|n| (*min..=*max).contains(&n))
            }
            ArgKind::Enum { values } => values.iter().any(|v| v == value),
            ArgKind::Bool => value == "0" || value == "1",
            ArgKind::Text { max } => value.len() <= *max && !value.contains('\0'),
        }
    }

    /// check refuses a declaration no value could sensibly satisfy, naming why.
    pub fn check(&self) -> Result<(), String> {
        match self {
            ArgKind::Int { min, max } if min > max => Err("min is above max".into()),
            ArgKind::Enum { values } => {
                let mut seen = std::collections::BTreeSet::new();
                if values.is_empty() {
                    return Err("an enum needs values".into());
                }
                for v in values {
                    if v.is_empty() || v.len() > 64 || v.contains('\0') || !seen.insert(v) {
                        return Err(format!("enum value {v:?} is empty, too long or repeated"));
                    }
                }
                Ok(())
            }
            ArgKind::Text { max } if *max == 0 || *max > MAX_TEXT => {
                Err(format!("text max must be 1–{MAX_TEXT}"))
            }
            _ => Ok(()),
        }
    }
}

fn digits(value: &str) -> bool {
    !value.is_empty() && value.bytes().all(|b| b.is_ascii_digit())
}

fn ipv4(value: &str) -> bool {
    value.parse::<Ipv4Addr>().is_ok()
}

fn ipv6(value: &str) -> bool {
    value
        .parse::<Ipv6Addr>()
        .is_ok_and(|a| a.to_ipv4_mapped().is_none())
}

fn hostname(value: &str) -> bool {
    !value.is_empty() && value.len() <= 253 && value.split('.').all(label)
}

fn label(l: &str) -> bool {
    let b = l.as_bytes();
    !b.is_empty()
        && b.len() <= 63
        && b[0] != b'-'
        && b[b.len() - 1] != b'-'
        && b.iter().all(|c| c.is_ascii_alphanumeric() || *c == b'-')
}
