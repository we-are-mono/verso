// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.
//! File families (ADR-027): each daemon's hand-edited files, declared by the
//! plugin that owns the daemon in its registry. The helper builds the table of
//! families from every installed registry for each operation, and drops — with
//! a logged reason — any family whose places reach outside `/etc`, into a
//! protected path, or into another family's.

use crate::registry::{self, Kind};
use serde::Deserialize;
use serde_json::{json, Value};
use std::path::{Path, PathBuf};

/// Paths under the base no family may name or reach into: the router's own
/// configuration, credentials, init and package machinery.
const PROTECTED: &[&str] = &[
    "config", "verso", "passwd", "shadow", "group", "rpcd", "dropbear", "ssl", "init.d",
    "rc.d", "hotplug.d", "crontabs", "uci-defaults",
];

#[derive(Deserialize, Debug, Clone, Copy, PartialEq, Eq)]
#[serde(rename_all = "kebab-case")]
pub enum Names {
    Plain,
    Dotted,
}

#[derive(Deserialize, Debug, Clone, PartialEq)]
pub struct Dir {
    pub path: String,
    pub suffixes: Vec<String>,
    pub names: Names,
}

#[derive(Deserialize, Debug, Clone, PartialEq)]
pub struct Words {
    pub too_large: String,
    pub invalid: String,
    pub over_limit: String,
}

/// A family as its registry declares it.
#[derive(Deserialize, Debug, Clone, PartialEq)]
pub struct Declared {
    pub name: String,
    pub config: String,
    #[serde(default)]
    pub files: Vec<String>,
    #[serde(default)]
    pub dir: Option<Dir>,
    pub mode: String,
    pub words: Words,
    #[serde(default)]
    pub check: Vec<String>,
    pub reload: Vec<String>,
    #[serde(default)]
    pub transform: Option<String>,
}

/// A family the helper uses: its declaration, the plugin that owns it, and
/// its transform command, if it names one.
#[derive(Debug, Clone, PartialEq)]
pub struct Family {
    pub plugin: String,
    pub declared: Declared,
    pub transform: Option<registry::Command>,
}

impl Family {
    pub fn name(&self) -> &str {
        &self.declared.name
    }

    pub fn words(&self) -> &Words {
        &self.declared.words
    }

    /// mode is the file mode a new file of the family is written with.
    pub fn mode(&self) -> u32 {
        if self.declared.mode == "0600" {
            0o600
        } else {
            0o644
        }
    }

    /// holds reports whether path is one of the family's fixed files, or a
    /// file directly in its directory named as the family names its files.
    pub fn holds(&self, path: &str) -> bool {
        if self.declared.files.iter().any(|f| f == path) {
            return true;
        }
        let Some(dir) = &self.declared.dir else {
            return false;
        };
        let Some(file) = path.strip_prefix(dir.path.as_str()).and_then(|p| p.strip_prefix('/')) else {
            return false;
        };
        dir.suffixes.iter().any(|suffix| {
            file.strip_suffix(suffix.as_str())
                .is_some_and(|name| named_as(dir.names, name))
        })
    }

    /// places is the family's fixed files and its directory.
    pub fn places(&self) -> (Vec<PathBuf>, Option<PathBuf>) {
        let files = self.declared.files.iter().map(PathBuf::from).collect();
        (files, self.declared.dir.as_ref().map(|d| PathBuf::from(&d.path)))
    }

    /// description is how the shell learns a family: its name, the uci config
    /// that owns it and the plugin that declared it.
    pub fn description(&self) -> Value {
        json!({"name": self.name(), "config": self.declared.config, "plugin": self.plugin})
    }
}

fn named_as(names: Names, name: &str) -> bool {
    let b = name.as_bytes();
    if b.is_empty() || !b[0].is_ascii_alphanumeric() {
        return false;
    }
    match names {
        Names::Plain => {
            b.len() <= 64 && b.iter().all(|c| c.is_ascii_lowercase() || c.is_ascii_digit() || matches!(c, b'_' | b'-'))
        }
        Names::Dotted => {
            b.len() <= 96 && b.iter().all(|c| c.is_ascii_alphanumeric() || matches!(c, b'_' | b'-' | b'.'))
        }
    }
}

/// Table is every family in use.
#[derive(Debug, Default, Clone, PartialEq)]
pub struct Table {
    pub families: Vec<Family>,
}

impl Table {
    /// installed builds the table from every installed plugin's registry.
    pub fn installed() -> Table {
        let mut found = vec![];
        for plugin in installed_plugins(Path::new(registry::PLUGINS)) {
            if let Ok(r) = registry::load_installed(&plugin) {
                found.extend(declared(&plugin, &r));
            }
        }
        let (table, refusals) = Table::build(Path::new("/etc"), found);
        for why in refusals {
            eprintln!("verso-rpcd: file family refused: {why}");
        }
        table
    }

    /// build keeps every family whose places lie under base, outside the
    /// protected paths and apart from every other family's, and answers the
    /// reasons the others were dropped.
    pub fn build(base: &Path, found: Vec<Family>) -> (Table, Vec<String>) {
        let mut refusals = vec![];
        let mut kept: Vec<Family> = vec![];
        for family in &found {
            if let Err(why) = bounded(base, family) {
                refusals.push(format!("{} ({}): {why}", family.name(), family.plugin));
            }
        }
        for (i, family) in found.iter().enumerate() {
            if bounded(base, family).is_err() {
                continue;
            }
            let clash = found.iter().enumerate().find(|(j, other)| {
                *j != i && bounded(base, other).is_ok() && (other.name() == family.name() || overlaps(family, other))
            });
            if let Some((_, other)) = clash {
                refusals.push(format!(
                    "{} ({}) shares a name or a place with {} ({})",
                    family.name(),
                    family.plugin,
                    other.name(),
                    other.plugin
                ));
                continue;
            }
            kept.push(family.clone());
        }
        kept.sort_by(|a, b| a.name().cmp(b.name()));
        (Table { families: kept }, refusals)
    }

    /// of is the family that holds path, if any.
    pub fn of(&self, path: &str) -> Option<&Family> {
        self.families.iter().find(|f| f.holds(path))
    }
}

/// declared is a registry's families, each with the transform it names.
pub fn declared(plugin: &str, r: &registry::Registry) -> Vec<Family> {
    r.families
        .iter()
        .filter_map(|value| match serde_json::from_value::<Declared>(value.clone()) {
            Ok(d) => Some(d),
            Err(e) => {
                eprintln!("verso-rpcd: {plugin}: a file family does not parse: {e}");
                None
            }
        })
        .filter_map(|d| {
            let transform = match &d.transform {
                None => None,
                Some(name) => match r.command(name).filter(|c| c.kind == Kind::Transform) {
                    Some(c) => Some(c.clone()),
                    None => {
                        eprintln!("verso-rpcd: {plugin}: family {:?} names no transform {name:?}", d.name);
                        return None;
                    }
                },
            };
            Some(Family { plugin: plugin.to_string(), declared: d, transform })
        })
        .collect()
}

fn installed_plugins(root: &Path) -> Vec<String> {
    let mut ids: Vec<String> = std::fs::read_dir(root)
        .map(|dir| {
            dir.filter_map(|e| e.ok()?.file_name().into_string().ok())
                .filter(|id| registry::valid_plugin(id))
                .collect()
        })
        .unwrap_or_default();
    ids.sort();
    ids
}

fn clean(path: &str) -> bool {
    path.len() > 1
        && path.starts_with('/')
        && !path.contains('\0')
        && path[1..].split('/').all(|p| !p.is_empty() && p != "." && p != "..")
}

/// bounded refuses a family whose places do not lie under base, or lie in or
/// over a protected path, or that has a malformed declaration.
fn bounded(base: &Path, family: &Family) -> Result<(), String> {
    let d = &family.declared;
    if !registry::valid_name(&d.name) {
        return Err("the name is not a family word".into());
    }
    if d.config.is_empty() || !d.config.bytes().all(|b| b.is_ascii_alphanumeric() || b == b'_' || b == b'-') {
        return Err("config is not a uci config name".into());
    }
    if d.mode != "0644" && d.mode != "0600" {
        return Err("mode must be 0644 or 0600".into());
    }
    if d.reload.is_empty() || d.reload.iter().chain(&d.check).any(|a| a.contains('\0')) {
        return Err("reload is missing, or a program's argument holds a NUL".into());
    }
    for program in [d.reload.first(), d.check.first()].into_iter().flatten() {
        if !clean(program) {
            return Err(format!("{program:?} is not an absolute program"));
        }
    }
    let mut places: Vec<&str> = d.files.iter().map(String::as_str).collect();
    if let Some(dir) = &d.dir {
        let suffix_ok = |s: &String| s.len() > 1 && s.starts_with('.') && s[1..].bytes().all(|b| b.is_ascii_alphanumeric());
        if dir.suffixes.is_empty() || !dir.suffixes.iter().all(suffix_ok) {
            return Err("the directory's suffixes are malformed".into());
        }
        places.push(&dir.path);
    }
    if places.is_empty() {
        return Err("a family needs files or a directory".into());
    }
    for place in places {
        if !clean(place) {
            return Err(format!("{place:?} is not an absolute path"));
        }
        let Ok(rest) = Path::new(place).strip_prefix(base) else {
            return Err(format!("{place} is not under {}", base.display()));
        };
        let Some(first) = rest.components().next() else {
            return Err(format!("{place} is {} itself", base.display()));
        };
        let first = first.as_os_str().to_string_lossy();
        if PROTECTED.contains(&first.as_ref()) {
            return Err(format!("{place} reaches into {}", base.join(first.as_ref()).display()));
        }
    }
    Ok(())
}

/// overlaps reports whether two families name a place in common: the same
/// fixed file, the same directory, or a fixed file of one in the other's
/// directory.
fn overlaps(a: &Family, b: &Family) -> bool {
    let (a_files, a_dir) = a.places();
    let (b_files, b_dir) = b.places();
    let inside = |file: &Path, dir: &Option<PathBuf>| dir.as_ref().is_some_and(|d| file.starts_with(d));
    a_files.iter().any(|f| b_files.contains(f) || inside(f, &b_dir))
        || b_files.iter().any(|f| inside(f, &a_dir))
        || matches!((&a_dir, &b_dir), (Some(x), Some(y)) if x.starts_with(y) || y.starts_with(x))
}

#[cfg(test)]
pub mod fixtures {
    use super::*;

    pub fn family(plugin: &str, name: &str, config: &str, files: &[&str], dir: Option<(&str, &[&str], Names)>) -> Family {
        Family {
            plugin: plugin.into(),
            declared: Declared {
                name: name.into(),
                config: config.into(),
                files: files.iter().map(|s| s.to_string()).collect(),
                dir: dir.map(|(path, suffixes, names)| Dir {
                    path: path.into(),
                    suffixes: suffixes.iter().map(|s| s.to_string()).collect(),
                    names,
                }),
                mode: "0644".into(),
                words: Words {
                    too_large: format!("{name} files must be at most 32 KiB."),
                    invalid: format!("Invalid {name} file."),
                    over_limit: format!("{name} files exceed the editor size limit."),
                },
                check: vec![],
                reload: vec!["/etc/init.d/x".into(), "reload".into()],
                transform: None,
            },
            transform: None,
        }
    }

    pub fn dnsmasq() -> Family {
        family("dnsdhcp", "dnsmasq", "dhcp", &["/etc/dnsmasq.conf"], Some(("/etc/dnsmasq.d", &[".conf"], Names::Plain)))
    }

    pub fn fw4() -> Family {
        family("firewall", "fw4", "firewall", &[], Some(("/etc/nftables.d", &[".nft"], Names::Plain)))
    }

    pub fn openvpn() -> Family {
        let mut f = family("vpn", "openvpn", "openvpn", &[], Some(("/etc/openvpn", &[".ovpn", ".conf"], Names::Dotted)));
        f.declared.mode = "0600".into();
        f
    }

    pub fn table() -> Table {
        Table::build(Path::new("/etc"), vec![dnsmasq(), fw4(), openvpn()]).0
    }
}

#[cfg(test)]
mod tests {
    use super::fixtures::*;
    use super::*;

    #[test]
    fn each_family_owns_its_own_paths() {
        let t = table();
        let of = |p: &str| t.of(p).map(|f| f.name().to_string());
        assert_eq!(of("/etc/dnsmasq.conf").as_deref(), Some("dnsmasq"));
        assert_eq!(of("/etc/dnsmasq.d/10-local.conf").as_deref(), Some("dnsmasq"));
        assert_eq!(of("/etc/nftables.d/10-custom.nft").as_deref(), Some("fw4"));
        for p in [
            "/etc/nftables.d/10-custom.conf",
            "/etc/dnsmasq.d/10-local.nft",
            "/etc/nftables.d/../passwd.nft",
            "/etc/nftables.d/a/b.nft",
            "/etc/nftables.d/.nft",
            "/etc/nftables.d/Rules.nft",
            "/etc/firewall.user",
            "/usr/share/nftables.d/ruleset-post/x.nft",
            "/etc/passwd",
            "/etc/dnsmasq.d/ABC.conf",
        ] {
            assert_eq!(of(p), None, "{p}");
        }
    }

    #[test]
    fn a_dotted_family_takes_a_providers_name() {
        let t = table();
        for p in ["/etc/openvpn/proton.ovpn", "/etc/openvpn/nl-free-12.protonvpn.udp.ovpn", "/etc/openvpn/Work.conf"] {
            assert_eq!(t.of(p).map(Family::name), Some("openvpn"), "{p}");
        }
        for p in [
            "/etc/openvpn/proton.auth",
            "/etc/openvpn/keys/a.ovpn",
            "/etc/openvpn/../shadow.conf",
            "/etc/openvpn/.ovpn",
            "/etc/openvpn/.hidden.ovpn",
            "/etc/openvpnx/a.ovpn",
        ] {
            assert!(t.of(p).is_none(), "{p}");
        }
    }

    #[test]
    fn a_family_in_a_protected_place_is_dropped() {
        for (files, dir) in [
            (vec!["/etc/shadow"], None),
            (vec!["/etc/config/firewall"], None),
            (vec![], Some(("/etc/init.d", &[".sh"][..], Names::Plain))),
            (vec![], Some(("/etc/verso", &[".key"][..], Names::Plain))),
            (vec!["/usr/bin/x"], None),
            (vec!["/etc"], None),
        ] {
            let bad = family("evil", "evil", "evil", &files, dir);
            let (t, refusals) = Table::build(Path::new("/etc"), vec![bad, fw4()]);
            assert_eq!(t.families.len(), 1, "{files:?} {dir:?}");
            assert_eq!(t.families[0].name(), "fw4");
            assert!(refusals[0].contains("evil"), "{refusals:?}");
        }
    }

    #[test]
    fn two_families_in_one_place_are_both_dropped() {
        let other = family("other", "profiles", "other", &[], Some(("/etc/openvpn", &[".ovpn"], Names::Dotted)));
        let (t, refusals) = Table::build(Path::new("/etc"), vec![openvpn(), other, fw4()]);
        assert_eq!(t.families.iter().map(Family::name).collect::<Vec<_>>(), ["fw4"]);
        assert_eq!(refusals.len(), 2);
        assert!(refusals.iter().all(|r| r.contains("vpn") || r.contains("other")), "{refusals:?}");

        let twin = family("twin", "fw4", "firewall", &[], Some(("/etc/nft2.d", &[".nft"], Names::Plain)));
        let (t, _) = Table::build(Path::new("/etc"), vec![fw4(), twin, dnsmasq()]);
        assert_eq!(t.families.iter().map(Family::name).collect::<Vec<_>>(), ["dnsmasq"]);

        let inside = family("in", "inside", "x", &["/etc/dnsmasq.d/a.conf"], None);
        let (t, _) = Table::build(Path::new("/etc"), vec![dnsmasq(), inside]);
        assert!(t.families.is_empty());
    }

    #[test]
    fn a_family_names_its_transform() {
        let r: registry::Registry = serde_json::from_value(json!({
            "schema_version": 1,
            "commands": [{"name": "files", "kind": "transform", "exec": "/usr/bin/verso-plugin-vpn", "argv": ["root"]},
                         {"name": "state", "kind": "read", "version": 1, "exec": "/usr/bin/verso-plugin-vpn"}],
            "families": [
                {"name": "openvpn", "config": "openvpn", "dir": {"path": "/etc/openvpn", "suffixes": [".ovpn"], "names": "dotted"},
                 "mode": "0600", "words": {"too_large": "a", "invalid": "b", "over_limit": "c"},
                 "reload": ["/etc/init.d/openvpn", "reload"], "transform": "files"},
                {"name": "bad", "config": "x", "files": ["/etc/x"], "mode": "0644",
                 "words": {"too_large": "a", "invalid": "b", "over_limit": "c"},
                 "reload": ["/x"], "transform": "state"}
            ]
        }))
        .unwrap();
        let found = declared("vpn", &r);
        assert_eq!(found.len(), 1, "a family whose transform is not one is dropped");
        assert_eq!(found[0].transform.as_ref().unwrap().name, "files");
        assert_eq!(found[0].mode(), 0o600);
        assert_eq!(found[0].description(), json!({"name": "openvpn", "config": "openvpn", "plugin": "vpn"}));
    }
}
