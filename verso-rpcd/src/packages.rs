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

// Read the cached index in one command, rather than spawning apk info for every
// result. Installed copies win the merge, including their removal protection.
// Pagination bounds the helper response even for an unfiltered All listing.
pub fn browse(query: &str, offset: usize) -> Result<Value, String> {
    let output = Command::new("apk")
        .args([
            "query",
            "--network=no",
            "--fields",
            "name,version,origin,description,license,url,file-size",
            "--format",
            "json",
            "*",
        ])
        .output()
        .map_err(|error| format!("apk query: {error}"))?;
    if !output.status.success() {
        return Err(format!("apk query: {}", output_tail(&output.stderr)));
    }
    let available: Vec<Value> = serde_json::from_slice(&output.stdout)
        .map_err(|error| format!("apk query returned invalid JSON: {error}"))?;
    let inventory = installed()?;
    Ok(browse_page(
        &available,
        inventory.as_array().unwrap(),
        query,
        offset,
    ))
}

fn browse_page(available: &[Value], inventory: &[Value], query: &str, offset: usize) -> Value {
    let mut by_name = std::collections::BTreeMap::new();
    for raw in available {
        let mut package = normalize_installed(raw, &HashMap::new());
        package["installed"] = json!(false);
        package["removable"] = json!(false);
        by_name.insert(
            package["name"].as_str().unwrap_or_default().to_string(),
            package,
        );
    }
    for package in inventory {
        by_name.insert(
            package["name"].as_str().unwrap_or_default().to_string(),
            package.clone(),
        );
    }
    let count = by_name.len();
    let query = query.to_lowercase();
    let matches = by_name
        .into_values()
        .filter(|p| {
            ["name", "description"].iter().any(|key| {
                p[key]
                    .as_str()
                    .unwrap_or_default()
                    .to_lowercase()
                    .contains(&query)
            })
        })
        .collect::<Vec<_>>();
    let total = matches.len();
    json!({"packages": matches.into_iter().skip(offset).take(SEARCH_LIMIT).collect::<Vec<_>>(),
        "total": total, "count": count, "installed": inventory.len()})
}

pub fn files(name: &str) -> Result<Value, String> {
    let output = Command::new("apk")
        .args([
            "query",
            "--network=no",
            "--installed",
            "--fields",
            "name,contents",
            "--format",
            "json",
            name,
        ])
        .output()
        .map_err(|error| format!("apk query --installed: {error}"))?;
    if !output.status.success() {
        return Err(format!(
            "apk query --installed: {}",
            output_tail(&output.stderr)
        ));
    }
    let packages: Vec<Value> = serde_json::from_slice(&output.stdout)
        .map_err(|error| format!("apk query returned invalid JSON: {error}"))?;
    let package = packages
        .iter()
        .find(|p| p["name"] == name)
        .ok_or_else(|| format!("{name} is not installed"))?;
    let mut paths = string_array(package, "contents")
        .map(|path| format!("/{}", path.trim_start_matches('/')))
        .collect::<Vec<_>>();
    paths.sort();
    paths.dedup();
    Ok(json!(paths))
}

pub fn upgrade_one(name: &str) -> Result<String, String> {
    // Require the exact installed package before invoking apk's selective upgrade.
    files(name)?;
    command_ok("apk upgrade", Command::new("apk").args(["upgrade", name]))
}

pub fn required_by(name: &str) -> Result<Vec<String>, String> {
    let packages = installed_packages()?;
    Ok(dependency_graph(&packages).remove(name).unwrap_or_default())
}

fn installed_packages() -> Result<Vec<Value>, String> {
    let output = Command::new("apk")
        .args([
            "query",
            "--network=no",
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
    let page = browse(query, 0)?;
    Ok((
        page["packages"].clone(),
        page["total"].as_u64().unwrap_or(0) as usize,
    ))
}

// merge_by_name reduces an `apk list` listing to one entry per package name. apk
// prints a line per package object it knows, and the repository index and the
// installed database each hold their own copy of a package — the same name lands
// twice, only one line carrying `[installed]`. A search result names a package,
// not a source, so the copies collapse into one entry and the installed one wins:
// it carries the state the row shows, and a row that says "installed" has to name
// the version the device actually holds. Entries keep apk's order of first
// appearance.
#[cfg(test)]
fn merge_by_name(listing: &str) -> Vec<Value> {
    let mut packages: Vec<Value> = Vec::new();
    let mut position: HashMap<String, usize> = HashMap::new();
    for line in listing.lines() {
        let Some(package) = parse_list_line(line) else {
            continue;
        };
        let name = package["name"].as_str().unwrap_or_default().to_string();
        match position.get(&name) {
            Some(&index) => {
                if package["installed"] == true {
                    packages[index] = package;
                }
            }
            None => {
                position.insert(name, packages.len());
                packages.push(package);
            }
        }
    }
    packages
}

// upgradable is what the configured feeds hold newer copies of. apk states both
// versions on one line — the repository's in the package token, the device's in the
// trailing `[upgradable from: …]` note — so the whole answer comes from one listing
// rather than from joining two queries whose sets could disagree.
pub fn upgradable() -> Result<Value, String> {
    let output = Command::new("apk")
        .args(["list", "--network=no", "--upgradable"])
        .output()
        .map_err(|error| format!("apk list --upgradable: {error}"))?;
    if !output.status.success() {
        return Err(format!(
            "apk list --upgradable: {}",
            output_tail(&output.stderr)
        ));
    }
    Ok(Value::Array(upgrades(&String::from_utf8_lossy(
        &output.stdout,
    ))))
}

pub fn upgrade() -> Result<String, String> {
    command_ok("apk upgrade", Command::new("apk").arg("upgrade"))
}

fn upgrades(listing: &str) -> Vec<Value> {
    listing.lines().filter_map(parse_upgrade_line).collect()
}

// A line is "<name>-<new> <arch> {origin} (license) [upgradable from: <name>-<old>]".
// Both halves split at the same place — the last dash before a digit — so the
// entry names one package with the two versions that make it upgradable.
fn parse_upgrade_line(line: &str) -> Option<Value> {
    let available = parse_list_line(line)?;
    let start = line.find("[upgradable from: ")? + "[upgradable from: ".len();
    let end = line[start..].find(']')? + start;
    let installed = split_version(line[start..end].trim())?;
    Some(json!({
        "name": available["name"],
        "installed": installed.1,
        "available": available["version"],
    }))
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

// apk names a package object "<name>-<version>". A name may itself hold dashes, so
// the split is the last dash that a digit follows — the point where the version
// begins.
fn split_version(token: &str) -> Option<(&str, &str)> {
    let at = token
        .char_indices()
        .rev()
        .find(|(index, ch)| {
            *ch == '-'
                && token
                    .as_bytes()
                    .get(index + 1)
                    .is_some_and(u8::is_ascii_digit)
        })?
        .0;
    Some((&token[..at], &token[at + 1..]))
}

fn parse_list_line(line: &str) -> Option<Value> {
    let (name, version) = split_version(line.split_whitespace().next()?)?;
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
    fn browsing_merges_inventory_searches_descriptions_and_pages_without_losing_matches() {
        let available = (0..65).map(|i| json!({"name":format!("pkg{i:02}"), "version":"2", "description":"Network tool"})).collect::<Vec<_>>();
        let installed = vec![
            json!({"name":"pkg01", "version":"1", "description":"Installed tool", "installed":true, "removable":false, "required_by":["verso"]}),
        ];
        let first = browse_page(&available, &installed, "", 0);
        assert_eq!(first["count"], 65);
        assert_eq!(first["total"], 65);
        assert_eq!(first["installed"], 1);
        assert_eq!(first["packages"].as_array().unwrap().len(), 30);
        assert_eq!(first["packages"][1], installed[0]);
        let last = browse_page(&available, &installed, "", 60);
        assert_eq!(last["packages"].as_array().unwrap().len(), 5);
        assert_eq!(last["packages"][0]["name"], "pkg60");
        let found = browse_page(&available, &installed, "INSTALLED TOOL", 0);
        assert_eq!(found["total"], 1);
        assert_eq!(found["count"], 65);
        assert_eq!(found["packages"][0]["name"], "pkg01");
        assert!(browse_page(&available, &installed, "", 90)["packages"]
            .as_array()
            .unwrap()
            .is_empty());
    }

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

    // apk lists the index copy and the installed copy of one package as two
    // lines; the search result is a package, so both collapse into the installed
    // entry — the row the person sees says "installed" exactly once.
    #[test]
    fn a_package_held_by_both_sources_lists_once_as_installed() {
        let listing = concat!(
            "busybox-1.37.0-r6 x86_64 {feeds/base/utils/busybox} (GPL-2.0)\n",
            "busybox-1.37.0-r6 x86_64 {feeds/base/utils/busybox} (GPL-2.0) [installed]\n",
            "busybox-selinux-1.37.0-r6 x86_64 {feeds/base/utils/busybox} (GPL-2.0)\n",
        );
        let packages = merge_by_name(listing);
        assert_eq!(packages.len(), 2);
        assert_eq!(packages[0]["name"], "busybox");
        assert_eq!(packages[0]["installed"], true);
        assert_eq!(packages[1]["name"], "busybox-selinux");
        assert_eq!(packages[1]["installed"], false);
    }

    // A newer version in the index is still the same package: one row, named at
    // the version the device holds, because that is what "installed" describes.
    #[test]
    fn the_installed_version_wins_over_a_newer_index_copy() {
        let listing = concat!(
            "dnsmasq-2.91-r3 x86_64 {feeds/base/network/services/dnsmasq} (GPL-2.0) [installed]\n",
            "dnsmasq-2.93-r1 x86_64 {feeds/base/network/services/dnsmasq} (GPL-2.0) [upgradable from: dnsmasq-2.91-r3]\n",
            "dnsmasq-full-2.93-r1 x86_64 {feeds/base/network/services/dnsmasq} (GPL-2.0)\n",
        );
        let packages = merge_by_name(listing);
        assert_eq!(packages.len(), 2);
        assert_eq!(packages[0]["name"], "dnsmasq");
        assert_eq!(packages[0]["version"], "2.91-r3");
        assert_eq!(packages[0]["installed"], true);
        assert_eq!(packages[1]["name"], "dnsmasq-full");
    }

    // Captured from `apk list --upgradable` on the dev container.
    const UPGRADABLE: &str = include_str!("../testdata/apk-upgradable.txt");

    #[test]
    fn an_upgradable_listing_names_both_versions_of_each_package() {
        let entries = upgrades(UPGRADABLE);
        assert_eq!(entries.len(), 47);
        assert_eq!(
            entries[2],
            json!({"name": "dnsmasq", "installed": "2.91-r3", "available": "2.93-r1"})
        );
        // A name holding dashes and a version holding a tilde both split correctly.
        let luci = entries
            .iter()
            .find(|entry| entry["name"] == "luci-app-firewall")
            .expect("luci-app-firewall");
        assert_eq!(luci["installed"], "26.133.20346~e9ebca7");
        assert_eq!(luci["available"], "26.239.42882~e60322b");
    }

    #[test]
    fn a_line_without_an_upgrade_note_is_not_an_upgrade() {
        let listing = concat!(
            "htop-3.4.1-r1 x86_64 {feeds/packages/utils/htop} (GPL-2.0) [installed]\n",
            "\n",
            "dnsmasq-2.93-r1 x86_64 {feeds/base/network/services/dnsmasq} (GPL-2.0) [upgradable from: dnsmasq-2.91-r3]\n",
        );
        let entries = upgrades(listing);
        assert_eq!(entries.len(), 1);
        assert_eq!(entries[0]["name"], "dnsmasq");
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
