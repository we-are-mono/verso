// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.
//! An act's arguments, checked against the kinds its registry declares before
//! a root command runs (ADR-027). The kinds are a closed set; the SDK's
//! `testdata/argument-kinds.json` holds the samples this check, the SDK's and
//! the shell's datatypes all answer the same way.

use serde::Deserialize;
use serde_json::Value;
use std::collections::BTreeMap;
use std::net::{Ipv4Addr, Ipv6Addr};

pub const MAX_TEXT: usize = 65536;

#[derive(Deserialize, Debug, Clone, PartialEq)]
#[serde(tag = "kind", rename_all = "kebab-case")]
pub enum Kind {
    UciName,
    Hostname,
    Ipv4,
    Ipv6,
    Ip,
    Port,
    Int { min: i64, max: i64 },
    Enum { values: Vec<String> },
    Bool,
    Text { max: usize },
}

#[derive(Deserialize, Debug, Clone, PartialEq)]
pub struct Spec {
    #[serde(flatten)]
    pub kind: Kind,
    #[serde(default)]
    pub optional: bool,
}

impl Kind {
    pub fn accepts(&self, value: &str) -> bool {
        match self {
            Kind::UciName => {
                !value.is_empty()
                    && value.len() <= 64
                    && value.bytes().all(|b| b.is_ascii_alphanumeric() || b == b'_')
            }
            Kind::Hostname => hostname(value),
            Kind::Ipv4 => ipv4(value),
            Kind::Ipv6 => ipv6(value),
            Kind::Ip => ipv4(value) || ipv6(value),
            Kind::Port => digits(value) && value.parse::<u32>().is_ok_and(|n| (1..=65535).contains(&n)),
            Kind::Int { min, max } => {
                digits(value.strip_prefix('-').unwrap_or(value))
                    && value.parse::<i64>().is_ok_and(|n| (*min..=*max).contains(&n))
            }
            Kind::Enum { values } => values.iter().any(|v| v == value),
            Kind::Bool => value == "0" || value == "1",
            Kind::Text { max } => value.len() <= *max && !value.contains('\0'),
        }
    }

    /// declared refuses a declaration no value could sensibly satisfy.
    pub fn declared(&self) -> Result<(), String> {
        match self {
            Kind::Int { min, max } if min > max => Err("min is above max".into()),
            Kind::Enum { values } if values.is_empty() => Err("an enum needs values".into()),
            Kind::Text { max } if *max == 0 || *max > MAX_TEXT => Err(format!("text max must be 1–{MAX_TEXT}")),
            _ => Ok(()),
        }
    }
}

/// check returns the act's arguments as text once each is present, known and
/// of its kind, or the reason they are refused. Nothing runs on a refusal.
pub fn check(specs: &BTreeMap<String, Spec>, given: &Value) -> Result<BTreeMap<String, String>, String> {
    let empty = serde_json::Map::new();
    let given = match given {
        Value::Null => &empty,
        Value::Object(map) => map,
        _ => return Err("the arguments are not an object".into()),
    };
    let mut out = BTreeMap::new();
    for (name, value) in given {
        let Some(spec) = specs.get(name) else {
            return Err(format!("unknown argument {name:?}"));
        };
        let Some(text) = value.as_str() else {
            return Err(format!("argument {name:?} is not text"));
        };
        if !spec.kind.accepts(text) {
            return Err(format!("argument {name:?} is not a valid {}", kind_name(&spec.kind)));
        }
        out.insert(name.clone(), text.to_string());
    }
    if let Some((name, _)) = specs.iter().find(|(n, s)| !s.optional && !out.contains_key(*n)) {
        return Err(format!("missing argument {name:?}"));
    }
    Ok(out)
}

fn kind_name(kind: &Kind) -> &'static str {
    match kind {
        Kind::UciName => "uci name",
        Kind::Hostname => "host name",
        Kind::Ipv4 => "IPv4 address",
        Kind::Ipv6 => "IPv6 address",
        Kind::Ip => "IP address",
        Kind::Port => "port",
        Kind::Int { .. } => "number in range",
        Kind::Enum { .. } => "choice",
        Kind::Bool => "0 or 1",
        Kind::Text { .. } => "text",
    }
}

fn digits(value: &str) -> bool {
    !value.is_empty() && value.bytes().all(|b| b.is_ascii_digit())
}

fn ipv4(value: &str) -> bool {
    value.parse::<Ipv4Addr>().is_ok()
}

fn ipv6(value: &str) -> bool {
    value.parse::<Ipv6Addr>().is_ok_and(|a| a.to_ipv4_mapped().is_none())
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

#[cfg(test)]
mod tests {
    use super::*;
    use serde_json::json;

    #[test]
    fn every_sample_is_answered_as_the_sdk_answers_it() {
        let path = concat!(env!("CARGO_MANIFEST_DIR"), "/../plugins/verso-plugin-sdk/testdata/argument-kinds.json");
        let doc: Value = serde_json::from_slice(&std::fs::read(path).unwrap()).unwrap();
        let kinds = doc["kinds"].as_array().unwrap();
        assert_eq!(kinds.len(), 10, "every kind has samples");
        for entry in kinds {
            let kind: Kind = serde_json::from_value(entry["kind"].clone()).unwrap();
            kind.declared().unwrap();
            for v in entry["accept"].as_array().unwrap() {
                assert!(kind.accepts(v.as_str().unwrap()), "{kind:?} refused {v}");
            }
            for r in entry["refuse"].as_array().unwrap() {
                assert!(!kind.accepts(r["value"].as_str().unwrap()), "{kind:?} accepted {}: {}", r["value"], r["why"]);
            }
        }
    }

    fn specs() -> BTreeMap<String, Spec> {
        serde_json::from_value(json!({
            "section": {"kind": "uci-name"},
            "note": {"kind": "text", "max": 16, "optional": true}
        }))
        .unwrap()
    }

    #[test]
    fn known_arguments_of_their_kind_pass() {
        let out = check(&specs(), &json!({"section": "home"})).unwrap();
        assert_eq!(out["section"], "home");
        assert!(check(&specs(), &json!({"section": "home", "note": "hi"})).is_ok());
    }

    #[test]
    fn an_argument_outside_its_kind_is_refused() {
        let why = check(&specs(), &json!({"section": "home; reboot"})).unwrap_err();
        assert!(why.contains("section"), "{why}");
    }

    #[test]
    fn an_unknown_argument_is_refused() {
        assert!(check(&specs(), &json!({"section": "home", "x": "1"})).unwrap_err().contains("unknown"));
    }

    #[test]
    fn a_missing_required_argument_is_refused() {
        assert!(check(&specs(), &json!({"note": "hi"})).unwrap_err().contains("missing"));
    }

    #[test]
    fn a_non_text_argument_is_refused() {
        assert!(check(&specs(), &json!({"section": 1})).is_err());
        assert!(check(&specs(), &json!(["home"])).is_err());
    }

    #[test]
    fn a_read_takes_no_arguments() {
        let none = BTreeMap::new();
        assert!(check(&none, &Value::Null).is_ok());
        assert!(check(&none, &json!({})).is_ok());
        assert!(check(&none, &json!({"x": "1"})).is_err());
    }
}
