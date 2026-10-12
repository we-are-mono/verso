// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

//! The configs as they are committed, with nothing staged in them: what an
//! apply replaces, so config history can say what each option held before
//! (ADR-021).
//!
//! `uci` given a config by its path loads the file alone and applies no delta
//! from any stage, the CLI's or an rpcd session's. `export` keeps an option
//! told from a list of one, which `show` does not, and `-n` names anonymous
//! sections the way `uci changes` does (`cfg0792bd`), so an old value is found
//! under the section a staged change names.

use serde_json::{Map, Value};
use std::path::Path;
use std::process::Command;

use crate::Failure;

/// Where uci looks for a config that overrides /etc/config (UCI_CONF2DIR).
const OVERRIDE_DIR: &str = "/var/run/uci";
const CONFIG_DIR: &str = "/etc/config";
/// A whole config is a few kilobytes; this bounds a runaway one.
const MAX_EXPORT: usize = 4 << 20;

/// committed answers each config named in configs, space-separated, with its
/// committed sections in the shape rpcd's `uci get` has: section name →
/// {".type", ".name", ".anonymous", ".index", option → string or list of
/// strings}. A config with no file is an empty object.
pub fn committed(configs: &str) -> Result<Value, Failure> {
    let mut out = Map::new();
    for name in configs.split_whitespace() {
        if !config_name(name) {
            return Err(Failure::invalid("configs must be uci config names"));
        }
        out.insert(name.to_owned(), read(name)?);
    }
    Ok(Value::Object(out))
}

pub(crate) fn read(name: &str) -> Result<Value, Failure> {
    let overriding = Path::new(OVERRIDE_DIR).join(name);
    let path = if overriding.is_file() {
        overriding
    } else {
        Path::new(CONFIG_DIR).join(name)
    };
    if !path.is_file() {
        return Ok(Value::Object(Map::new()));
    }
    let mut command = Command::new("/sbin/uci");
    command.arg("-n").arg("export").arg(&path);
    let output = crate::run_bounded(command, "uci export", MAX_EXPORT)?;
    if !output.status.success() {
        return Err(crate::command_failure("uci export", output));
    }
    let text = String::from_utf8(output.stdout)
        .map_err(|_| Failure::unknown("uci export: not UTF-8"))?;
    parse_export(&text).map_err(Failure::unknown)
}

/// config_name is libuci's rule for a package name: letters, digits,
/// underscores and dashes, and not empty.
pub(crate) fn config_name(name: &str) -> bool {
    !name.is_empty()
        && name.len() <= 64
        && name
            .bytes()
            .all(|b| b.is_ascii_alphanumeric() || b == b'_' || b == b'-')
}

/// parse_export reads `uci export` into sections. Every section carries its
/// name, as `-n` has it print one even for an anonymous section.
fn parse_export(text: &str) -> Result<Value, String> {
    let mut sections = Map::new();
    let mut current: Option<String> = None;
    let mut index = 0usize;
    for statement in statements(text)? {
        match statement.as_slice() {
            [keyword, ..] if keyword == "package" => {}
            [keyword, kind, name] if keyword == "config" => {
                let mut section = Map::new();
                section.insert(".type".into(), Value::String(kind.clone()));
                section.insert(".name".into(), Value::String(name.clone()));
                section.insert(".anonymous".into(), Value::Bool(anonymous(name)));
                section.insert(".index".into(), Value::from(index));
                index += 1;
                sections.insert(name.clone(), Value::Object(section));
                current = Some(name.clone());
            }
            [keyword, option, value] if keyword == "option" => {
                section_of(&mut sections, &current)?
                    .insert(option.clone(), Value::String(value.clone()));
            }
            [keyword, option, value] if keyword == "list" => {
                let section = section_of(&mut sections, &current)?;
                match section
                    .entry(option.clone())
                    .or_insert_with(|| Value::Array(Vec::new()))
                {
                    Value::Array(values) => values.push(Value::String(value.clone())),
                    _ => return Err(format!("{option} is both an option and a list")),
                }
            }
            other => return Err(format!("unexpected statement {other:?}")),
        }
    }
    Ok(Value::Object(sections))
}

fn section_of<'a>(
    sections: &'a mut Map<String, Value>,
    current: &Option<String>,
) -> Result<&'a mut Map<String, Value>, String> {
    current
        .as_ref()
        .and_then(|name| sections.get_mut(name))
        .and_then(Value::as_object_mut)
        .ok_or_else(|| "an option outside any section".to_owned())
}

/// anonymous is whether uci made the name up: cfg, then two hex digits of the
/// section's place and four of its type's hash.
fn anonymous(name: &str) -> bool {
    name.len() == 9
        && name.starts_with("cfg")
        && name[3..].bytes().all(|b| b.is_ascii_hexdigit())
}

/// statements splits export text into its statements, each a list of words.
/// A word is bare or single-quoted; export writes a quote inside a value as
/// '\'', which closes the quote, adds an escaped quote and opens it again. A
/// value may hold a newline, so only a newline outside quotes ends a statement.
fn statements(text: &str) -> Result<Vec<Vec<String>>, String> {
    let mut out = Vec::new();
    let mut words: Vec<String> = Vec::new();
    let mut word = String::new();
    let mut in_word = false;
    let mut chars = text.chars();
    while let Some(c) = chars.next() {
        match c {
            '\'' => {
                in_word = true;
                loop {
                    match chars.next() {
                        Some('\'') => break,
                        Some(inner) => word.push(inner),
                        None => return Err("a quote is never closed".into()),
                    }
                }
            }
            '\\' => {
                in_word = true;
                match chars.next() {
                    Some(escaped) => word.push(escaped),
                    None => return Err("a backslash ends the text".into()),
                }
            }
            '\n' | ' ' | '\t' | '\r' => {
                if in_word {
                    words.push(std::mem::take(&mut word));
                    in_word = false;
                }
                if c == '\n' && !words.is_empty() {
                    out.push(std::mem::take(&mut words));
                }
            }
            _ => {
                in_word = true;
                word.push(c);
            }
        }
    }
    if in_word {
        words.push(word);
    }
    if !words.is_empty() {
        out.push(words);
    }
    Ok(out)
}

#[cfg(test)]
mod tests {
    use super::*;
    use serde_json::json;

    /// What `uci -n export` prints for a firewall with an anonymous rule, a
    /// named zone, a list, and values holding a quote, a space and a newline.
    const EXPORT: &str = "package firewall\n\
\n\
config defaults 'cfg01e63d'\n\
\toption input 'REJECT'\n\
\n\
config zone 'lan'\n\
\toption name 'lan'\n\
\tlist network 'lan'\n\
\tlist network 'guest'\n\
\n\
config rule 'cfg0792bd'\n\
\toption name 'Ana'\\''s iPad'\n\
\toption extra 'two words'\n\
\toption note 'line one\n\
line two'\n\
\toption enabled ''\n";

    #[test]
    fn export_reads_into_rpcd_shaped_sections() {
        let got = parse_export(EXPORT).unwrap();
        assert_eq!(
            got,
            json!({
                "cfg01e63d": {".type": "defaults", ".name": "cfg01e63d", ".anonymous": true, ".index": 0, "input": "REJECT"},
                "lan": {".type": "zone", ".name": "lan", ".anonymous": false, ".index": 1, "name": "lan", "network": ["lan", "guest"]},
                "cfg0792bd": {".type": "rule", ".name": "cfg0792bd", ".anonymous": true, ".index": 2,
                    "name": "Ana's iPad", "extra": "two words", "note": "line one\nline two", "enabled": ""}
            })
        );
    }

    #[test]
    fn a_list_of_one_stays_a_list() {
        let got = parse_export("config zone 'wan'\n\tlist network 'wan'\n").unwrap();
        assert_eq!(got["wan"]["network"], json!(["wan"]));
    }

    #[test]
    fn broken_export_is_an_error_not_a_guess() {
        assert!(parse_export("config zone 'wan'\n\toption name 'wan\n").is_err());
        assert!(parse_export("\toption name 'wan'\n").is_err());
        assert!(parse_export("config zone 'wan'\n\toption network 'a'\n\tlist network 'b'\n").is_err());
    }

    #[test]
    fn only_uci_config_names_are_read() {
        assert!(config_name("network"));
        assert!(config_name("https-dns-proxy"));
        for bad in ["", "../shadow", "/etc/shadow", "net work", "a.b"] {
            assert!(!config_name(bad), "{bad}");
        }
        assert!(committed("network ../shadow").is_err());
        assert!(committed("/etc/shadow").is_err());
    }

    #[test]
    fn anonymous_names_are_the_ones_uci_made_up() {
        assert!(anonymous("cfg0792bd"));
        assert!(!anonymous("lan"));
        assert!(!anonymous("cfgx792bd"));
    }
}
