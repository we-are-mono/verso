// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

use serde_json::{json, Value};
use std::collections::{HashMap, HashSet};
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
    let packages = installed_packages()?;
    let required_by = dependency_graph(&packages);
    Ok(Value::Array(
        packages
            .iter()
            .map(|package| normalize_installed(package, &required_by))
            .collect(),
    ))
}

pub fn required_by(name: &str) -> Result<Vec<String>, String> {
    let packages = installed_packages()?;
    Ok(dependency_graph(&packages).remove(name).unwrap_or_default())
}

fn installed_packages() -> Result<Vec<Value>, String> {
    let output = Command::new("apk")
        .args([
            "query",
            "--installed",
            "--fields",
            "name,version,origin,description,license,url,file-size,contents,depends,provides",
            "--format",
            "json",
            "*",
        ])
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
    Ok(raw
        .as_array()
        .ok_or_else(|| "apk query --installed returned a non-array".to_string())?
        .clone())
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
    if packages
        .iter()
        .any(|package| package.get("installed").and_then(Value::as_bool) == Some(true))
    {
        let inventory = installed()?;
        let installed = inventory
            .as_array()
            .into_iter()
            .flatten()
            .filter_map(|package| {
                package
                    .get("name")
                    .and_then(Value::as_str)
                    .map(|name| (name.to_string(), package))
            })
            .collect::<HashMap<_, _>>();
        for package in &mut packages {
            let Some(object) = package.as_object_mut() else {
                continue;
            };
            let Some(found) = object
                .get("name")
                .and_then(Value::as_str)
                .and_then(|name| installed.get(name))
            else {
                continue;
            };
            object.insert("removable".into(), found["removable"].clone());
            object.insert("required_by".into(), found["required_by"].clone());
        }
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

fn dependency_graph(packages: &[Value]) -> HashMap<String, Vec<String>> {
    let mut providers: HashMap<String, Vec<String>> = HashMap::new();
    for package in packages {
        let Some(name) = package.get("name").and_then(Value::as_str) else {
            continue;
        };
        providers
            .entry(name.to_string())
            .or_default()
            .push(name.to_string());
        for provided in string_array(package, "provides") {
            providers
                .entry(constraint_name(provided).to_string())
                .or_default()
                .push(name.to_string());
        }
    }

    let mut graph: HashMap<String, HashSet<String>> = HashMap::new();
    for package in packages {
        let Some(dependent) = package.get("name").and_then(Value::as_str) else {
            continue;
        };
        for dependency in string_array(package, "depends") {
            if dependency.starts_with('!') {
                continue;
            }
            if let Some(matches) = providers.get(constraint_name(dependency)) {
                for provider in matches {
                    if provider != dependent {
                        graph
                            .entry(provider.clone())
                            .or_default()
                            .insert(dependent.to_string());
                    }
                }
            }
        }
    }

    graph
        .into_iter()
        .map(|(name, dependents)| {
            let mut dependents = dependents.into_iter().collect::<Vec<_>>();
            dependents.sort();
            (name, dependents)
        })
        .collect()
}

fn string_array<'a>(value: &'a Value, key: &str) -> impl Iterator<Item = &'a str> {
    value
        .get(key)
        .and_then(Value::as_array)
        .into_iter()
        .flatten()
        .filter_map(Value::as_str)
}

fn constraint_name(value: &str) -> &str {
    value.split(['<', '>', '=', '~']).next().unwrap_or(value)
}

fn normalize_installed(package: &Value, required_by: &HashMap<String, Vec<String>>) -> Value {
    let string = |key: &str| {
        package
            .get(key)
            .and_then(Value::as_str)
            .unwrap_or_default()
            .to_string()
    };
    let origin = string("origin");
    let name = string("name");
    let dependents = required_by.get(&name).cloned().unwrap_or_default();
    let removable = !protected(&name) && dependents.is_empty();
    let services = package
        .get("contents")
        .and_then(Value::as_array)
        .into_iter()
        .flatten()
        .filter_map(Value::as_str)
        .filter_map(|path| path.strip_prefix("etc/init.d/"))
        .filter(|name| !name.is_empty() && !name.contains('/'))
        .collect::<Vec<_>>();
    json!({
        "name": name,
        "version": string("version"),
        "feed": feed_of(&origin),
        "description": string("description"),
        "license": string("license"),
        "webpage": string("url"),
        "size": package.get("file-size").and_then(Value::as_i64).unwrap_or(0),
        "installed": true,
        "services": services,
        "required_by": dependents,
        "removable": removable,
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
    fn installed_package_exposes_only_its_init_scripts() {
        let value = normalize_installed(
            &json!({
                "name": "verso",
                "contents": [
                    "etc/init.d/verso",
                    "etc/init.d/verso-rpcd",
                    "usr/bin/verso",
                    "etc/init.d/nested/not-a-service"
                ]
            }),
            &HashMap::new(),
        );
        assert_eq!(value["services"], json!(["verso", "verso-rpcd"]));
    }

    #[test]
    fn installed_dependencies_are_not_removable() {
        let packages = vec![
            json!({"name": "verso", "depends": ["ca-bundle>=20260601"]}),
            json!({"name": "ca-bundle", "depends": ["libc"], "provides": ["ca-certificates-any"]}),
            json!({"name": "libc"}),
            json!({"name": "htop"}),
        ];
        let graph = dependency_graph(&packages);
        assert_eq!(graph["ca-bundle"], ["verso"]);
        assert_eq!(graph["libc"], ["ca-bundle"]);
        assert!(!normalize_installed(&packages[1], &graph)["removable"]
            .as_bool()
            .unwrap());
        assert!(normalize_installed(&packages[3], &graph)["removable"]
            .as_bool()
            .unwrap());
    }

    #[test]
    fn virtual_dependencies_protect_the_installed_provider() {
        let packages = vec![
            json!({"name": "downloader", "depends": ["ca-certificates-any"]}),
            json!({"name": "ca-bundle", "provides": ["ca-certificates-any"]}),
        ];
        assert_eq!(dependency_graph(&packages)["ca-bundle"], ["downloader"]);
    }

    #[test]
    fn keeps_the_control_plane_installed() {
        for package in ["busybox", "apk", "rpcd", "verso"] {
            assert!(protected(package));
        }
        assert!(!protected("htop"));
    }
}
