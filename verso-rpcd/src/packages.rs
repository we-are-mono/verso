// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

use serde_json::{json, Value};
use std::fs;
use std::process::Command;
use std::time::UNIX_EPOCH;

const APK_INDEX_DIR: &str = "/var/cache/apk";
const SEARCH_LIMIT: usize = 30;

pub fn checked_at() -> i64 {
    let Ok(entries) = fs::read_dir(APK_INDEX_DIR) else {
        return 0;
    };
    entries
        .flatten()
        .filter_map(|entry| entry.metadata().ok())
        .filter_map(|metadata| metadata.modified().ok())
        .filter_map(|modified| modified.duration_since(UNIX_EPOCH).ok())
        .map(|duration| duration.as_secs() as i64)
        .max()
        .unwrap_or(0)
}

pub fn update() -> Result<(), String> {
    command_ok("apk update", Command::new("apk").arg("update"))?;
    Ok(())
}

pub fn installed() -> Result<Value, String> {
    let output = Command::new("apk")
        .args(["query", "--installed", "--format", "json", "*"])
        .output()
        .map_err(|error| format!("apk query --installed: {error}"))?;
    if !output.status.success() {
        return Err(format!(
            "apk query --installed: {}",
            output_tail(&output.stderr)
        ));
    }
    let raw: Value = serde_json::from_slice(&output.stdout)
        .map_err(|error| format!("apk query --installed returned invalid JSON: {error}"))?;
    let packages = raw
        .as_array()
        .ok_or_else(|| "apk query --installed returned a non-array".to_string())?
        .iter()
        .map(normalize_installed)
        .collect::<Vec<_>>();
    Ok(Value::Array(packages))
}

pub fn search(query: &str) -> Result<(Value, usize), String> {
    let pattern = format!("*{query}*");
    let output = Command::new("apk")
        .args(["list", &pattern])
        .output()
        .map_err(|error| format!("apk list: {error}"))?;
    if !output.status.success() {
        return Err(format!("apk list: {}", output_tail(&output.stderr)));
    }

    let text = String::from_utf8_lossy(&output.stdout);
    let mut total = 0usize;
    let mut packages = Vec::new();
    for line in text.lines() {
        let Some(mut package) = parse_list_line(line) else {
            continue;
        };
        total += 1;
        if packages.len() >= SEARCH_LIMIT {
            continue;
        }
        if let Some(object) = package.as_object_mut() {
            object.insert(
                "description".into(),
                Value::String(description(object["name"].as_str().unwrap_or_default())),
            );
        }
        packages.push(package);
    }
    Ok((Value::Array(packages), total))
}

pub fn install(name: &str) -> Result<String, String> {
    command_ok("apk add", Command::new("apk").args(["add", name]))
}

pub fn remove(name: &str) -> Result<String, String> {
    command_ok("apk del", Command::new("apk").args(["del", name]))
}

pub fn valid_name(name: &str) -> bool {
    let bytes = name.as_bytes();
    (1..=64).contains(&bytes.len())
        && (bytes[0].is_ascii_lowercase() || bytes[0].is_ascii_digit())
        && bytes
            .iter()
            .all(|byte| byte.is_ascii_alphanumeric() || matches!(byte, b'.' | b'_' | b'+' | b'-'))
}

pub fn valid_query(query: &str) -> bool {
    let bytes = query.as_bytes();
    (1..=64).contains(&bytes.len())
        && bytes
            .iter()
            .all(|byte| byte.is_ascii_alphanumeric() || matches!(byte, b'.' | b'_' | b'+' | b'-'))
}

pub fn protected(name: &str) -> bool {
    matches!(
        name,
        "busybox"
            | "apk"
            | "procd"
            | "ubusd"
            | "ubus"
            | "rpcd"
            | "uhttpd"
            | "netifd"
            | "libc"
            | "musl"
            | "verso"
    )
}

fn normalize_installed(package: &Value) -> Value {
    let string = |key: &str| {
        package
            .get(key)
            .and_then(Value::as_str)
            .unwrap_or_default()
            .to_string()
    };
    let origin = string("origin");
    json!({
        "name": string("name"),
        "version": string("version"),
        "feed": feed_of(&origin),
        "description": string("description"),
        "license": string("license"),
        "webpage": string("url"),
        "size": package.get("file-size").and_then(Value::as_i64).unwrap_or(0),
        "installed": true,
    })
}

fn parse_list_line(line: &str) -> Option<Value> {
    let first = line.split_whitespace().next()?;
    let split = first
        .char_indices()
        .rev()
        .find(|(index, ch)| {
            *ch == '-'
                && first
                    .as_bytes()
                    .get(index + 1)
                    .is_some_and(u8::is_ascii_digit)
        })?
        .0;
    let name = &first[..split];
    let version = &first[split + 1..];
    let origin_start = line.find('{')? + 1;
    let origin_end = line[origin_start..].find('}')? + origin_start;
    let origin = &line[origin_start..origin_end];
    Some(json!({
        "name": name,
        "version": version,
        "feed": feed_of(origin),
        "description": "",
        "installed": line.trim_end().ends_with("[installed]"),
    }))
}

fn feed_of(origin: &str) -> &str {
    let mut parts = origin.split('/');
    if parts.next() == Some("feeds") {
        return parts.next().unwrap_or("base");
    }
    "base"
}

fn description(name: &str) -> String {
    let Ok(output) = Command::new("apk").args(["info", "-d", name]).output() else {
        return String::new();
    };
    if !output.status.success() {
        return String::new();
    }
    let text = String::from_utf8_lossy(&output.stdout);
    text.trim()
        .split_once('\n')
        .map(|(_, description)| description.trim().to_string())
        .unwrap_or_default()
}

fn command_ok(label: &str, command: &mut Command) -> Result<String, String> {
    let output = command
        .output()
        .map_err(|error| format!("{label}: {error}"))?;
    let combined = if output.stderr.is_empty() {
        output.stdout
    } else {
        [output.stdout, output.stderr].concat()
    };
    if !output.status.success() {
        return Err(format!("{label}: {}", output_tail(&combined)));
    }
    Ok(output_tail(&combined))
}

fn output_tail(output: &[u8]) -> String {
    let text = String::from_utf8_lossy(output).trim().to_string();
    if text.chars().count() <= 400 {
        return text;
    }
    let tail = text.chars().rev().take(400).collect::<String>();
    format!("…{}", tail.chars().rev().collect::<String>())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn package_names_are_tightly_bounded() {
        assert!(valid_name("htop"));
        assert!(valid_name("kmod-nft-core"));
        for invalid in ["", "-rf", "a b", "../etc", "htop*", "UPPER"] {
            assert!(!valid_name(invalid), "accepted {invalid:?}");
        }
    }

    #[test]
    fn parses_apk_list_rows() {
        let value = parse_list_line(
            "busybox-selinux-1.37.0-r6 aarch64_generic {feeds/base/utils/busybox} (GPL-2.0) [installed]",
        )
        .expect("row");
        assert_eq!(value["name"], "busybox-selinux");
        assert_eq!(value["version"], "1.37.0-r6");
        assert_eq!(value["feed"], "base");
        assert_eq!(value["installed"], true);
    }

    #[test]
    fn keeps_the_control_plane_installed() {
        for package in ["busybox", "apk", "rpcd", "verso"] {
            assert!(protected(package));
        }
        assert!(!protected("htop"));
    }
}
