// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.
//! The command registry (ADR-027): each plugin's root commands, read from
//! `/usr/share/verso/plugins/<id>/commands.json` each time one is asked for, so
//! an install or a removal takes effect at once. A registry is read only from
//! root-owned files, and only a well-formed one is used; a refused registry
//! affects its own plugin alone.

use crate::args;
use crate::paths::{self, Owner};
use serde::Deserialize;
use std::collections::BTreeMap;
use std::path::{Path, PathBuf};

pub const PLUGINS: &str = "/usr/share/verso/plugins";
const SCHEMA_VERSION: u32 = 1;
pub const DEFAULT_TIMEOUT: u32 = 10;
pub const MAX_TIMEOUT: u32 = 120;
const MAX_FILE: u64 = 256 * 1024;

#[derive(Deserialize, Debug, Clone, Copy, PartialEq, Eq)]
#[serde(rename_all = "kebab-case")]
pub enum Kind {
    Read,
    Act,
    Transform,
}

#[derive(Deserialize, Debug, Clone, PartialEq)]
pub struct Command {
    pub name: String,
    pub kind: Kind,
    #[serde(default)]
    pub version: Option<u32>,
    pub exec: String,
    #[serde(default)]
    pub argv: Vec<String>,
    #[serde(default)]
    pub timeout: Option<u32>,
    #[serde(default)]
    pub args: BTreeMap<String, args::Spec>,
}

impl Command {
    pub fn timeout(&self) -> u32 {
        self.timeout.unwrap_or(DEFAULT_TIMEOUT)
    }
}

#[derive(Deserialize, Debug, Clone, PartialEq)]
pub struct Registry {
    pub schema_version: u32,
    #[serde(default)]
    pub commands: Vec<Command>,
    #[serde(default)]
    pub families: Vec<serde_json::Value>,
    #[serde(default)]
    pub certificates: BTreeMap<String, Vec<String>>,
}

impl Registry {
    pub fn command(&self, name: &str) -> Option<&Command> {
        self.commands.iter().find(|c| c.name == name)
    }
}

/// Why a registry or a command in it was not used.
#[derive(Debug, PartialEq, Eq)]
pub enum Refused {
    /// The plugin ships no registry: it is not installed, or has no root work.
    Absent,
    /// The registry is there but not to be used, for the logged reason.
    Unusable(String),
}

/// A plugin id: what the manifest names it, and the directory its files live in.
pub fn valid_plugin(id: &str) -> bool {
    !id.is_empty()
        && id.len() <= 64
        && id.as_bytes()[0].is_ascii_lowercase()
        && id.bytes().all(|b| b.is_ascii_lowercase() || b.is_ascii_digit() || b == b'-')
}

/// A command name: the function of the plugin's rpcd grant.
pub fn valid_name(name: &str) -> bool {
    valid_plugin(name)
}

fn absolute(path: &str) -> bool {
    path.len() > 1
        && path.starts_with('/')
        && !path.contains('\0')
        && path[1..].split('/').all(|p| !p.is_empty() && p != "." && p != "..")
}

/// path is where a plugin's registry lives under `root`.
pub fn path(root: &Path, plugin: &str) -> PathBuf {
    root.join(plugin).join("commands.json")
}

/// load reads one plugin's registry under `root`, from root-owned files only,
/// and checks every command in it, so a registry is used whole or not at all.
pub fn load(owner: &impl Owner, root: &Path, plugin: &str) -> Result<Registry, Refused> {
    if !valid_plugin(plugin) {
        return Err(Refused::Absent);
    }
    let file = path(root, plugin);
    match owner.meta(&file) {
        Err(e) if e.kind() == std::io::ErrorKind::NotFound => return Err(Refused::Absent),
        _ => {}
    }
    paths::root_only(owner, &file).map_err(Refused::Unusable)?;
    let text = read_capped(&file).map_err(Refused::Unusable)?;
    let registry: Registry = serde_json::from_str(&text)
        .map_err(|e| Refused::Unusable(format!("{}: {e}", file.display())))?;
    check(&registry).map_err(|why| Refused::Unusable(format!("{}: {why}", file.display())))?;
    for command in &registry.commands {
        paths::root_only(owner, Path::new(&command.exec))
            .map_err(|why| Refused::Unusable(format!("{} ({}): {why}", file.display(), command.name)))?;
    }
    Ok(registry)
}

fn read_capped(file: &Path) -> Result<String, String> {
    use std::io::Read;
    let mut text = String::new();
    std::fs::File::open(file)
        .and_then(|f| f.take(MAX_FILE + 1).read_to_string(&mut text))
        .map_err(|e| format!("{}: {e}", file.display()))?;
    if text.len() as u64 > MAX_FILE {
        return Err(format!("{} is larger than {MAX_FILE} bytes", file.display()));
    }
    Ok(text)
}

/// check refuses a registry the SDK's `Registry::check` would refuse: the
/// helper is the boundary, so it never trusts that a registry was built with it.
pub fn check(r: &Registry) -> Result<(), String> {
    if r.schema_version != SCHEMA_VERSION {
        return Err(format!("schema_version {} is not {SCHEMA_VERSION}", r.schema_version));
    }
    let mut seen = std::collections::BTreeSet::new();
    for c in &r.commands {
        let at = |why: &str| format!("command {:?}: {why}", c.name);
        if !valid_name(&c.name) || !seen.insert(c.name.as_str()) {
            return Err(at("the name is not a function word, or is declared twice"));
        }
        if !absolute(&c.exec) || c.argv.iter().any(|a| a.contains('\0')) {
            return Err(at("exec must be an absolute path and argv hold no NUL"));
        }
        if c.timeout.is_some_and(|t| t == 0 || t > MAX_TIMEOUT) {
            return Err(at("the timeout is out of bounds"));
        }
        match c.kind {
            Kind::Read if !c.args.is_empty() => return Err(at("a read takes no arguments")),
            Kind::Read if !c.version.is_some_and(|v| v > 0) => return Err(at("a read needs a version")),
            Kind::Act | Kind::Transform if c.version.is_some() => return Err(at("only a read has a version")),
            Kind::Transform if !c.args.is_empty() => return Err(at("a transform takes no arguments")),
            _ => {}
        }
        for (name, spec) in &c.args {
            spec.kind.declared().map_err(|why| at(&format!("argument {name:?}: {why}")))?;
        }
    }
    for read in r.certificates.keys() {
        if r.command(read).map(|c| c.kind) != Some(Kind::Read) {
            return Err(format!("certificates: {read:?} is not a read"));
        }
    }
    Ok(())
}

/// load_installed reads a plugin's registry from the real filesystem, under
/// [`PLUGINS`].
pub fn load_installed(plugin: &str) -> Result<Registry, Refused> {
    load(&paths::System, Path::new(PLUGINS), plugin)
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::paths::fake::Table;
    use std::fs;

    /// A registry written to a temp dir, with an ownership table that says the
    /// written file and the executable are root's (the tests do not run as root).
    struct Fixture {
        dir: PathBuf,
    }

    impl Fixture {
        fn new(name: &str, json: &str) -> Fixture {
            let dir = std::env::temp_dir().join(format!("verso-rpcd-registry-{name}-{}", std::process::id()));
            let _ = fs::remove_dir_all(&dir);
            fs::create_dir_all(dir.join("ddns")).unwrap();
            fs::write(dir.join("ddns/commands.json"), json).unwrap();
            Fixture { dir }
        }
        fn table(&self) -> Table {
            Table::tree(&[path(&self.dir, "ddns").to_str().unwrap(), EXEC, "/x", "/usr/bin/verso-plugin-example"])
        }
    }

    impl Drop for Fixture {
        fn drop(&mut self) {
            let _ = fs::remove_dir_all(&self.dir);
        }
    }

    const EXEC: &str = "/usr/bin/verso-plugin-ddns";

    #[test]
    fn a_group_writable_executable_is_refused() {
        let f = Fixture::new("exec", GOOD);
        let t = f.table().set(EXEC, |m| m.mode = 0o100777);
        match load(&t, &f.dir, "ddns") {
            Err(Refused::Unusable(why)) => assert!(why.contains(EXEC), "{why}"),
            other => panic!("{other:?}"),
        }
        let t = f.table().set(EXEC, |m| m.link = true);
        assert!(matches!(load(&t, &f.dir, "ddns"), Err(Refused::Unusable(_))));
    }

    const GOOD: &str = r#"{"schema_version": 1, "commands": [
        {"name": "state", "kind": "read", "version": 1, "exec": "/usr/bin/verso-plugin-ddns", "argv": ["root"]},
        {"name": "update", "kind": "act", "exec": "/usr/bin/verso-plugin-ddns", "argv": ["root"], "timeout": 30,
         "args": {"section": {"kind": "uci-name"}}}
    ]}"#;

    #[test]
    fn a_root_owned_registry_loads() {
        let f = Fixture::new("good", GOOD);
        let r = load(&f.table(), &f.dir, "ddns").unwrap();
        assert_eq!(r.command("update").unwrap().timeout(), 30);
        assert_eq!(r.command("state").unwrap().timeout(), DEFAULT_TIMEOUT);
    }

    #[test]
    fn a_plugin_without_a_registry_is_absent() {
        let f = Fixture::new("absent", GOOD);
        assert_eq!(load(&f.table(), &f.dir, "vpn"), Err(Refused::Absent));
        assert_eq!(load(&f.table(), &f.dir, "../ddns"), Err(Refused::Absent));
    }

    #[test]
    fn a_registry_others_may_write_is_refused() {
        let f = Fixture::new("writable", GOOD);
        let file = path(&f.dir, "ddns");
        let t = f.table().set(file.to_str().unwrap(), |m| {
            m.mode = 0o100664;
            m.gid = 6000;
        });
        match load(&t, &f.dir, "ddns") {
            Err(Refused::Unusable(why)) => assert!(why.contains("commands.json"), "{why}"),
            other => panic!("{other:?}"),
        }
    }

    #[test]
    fn a_malformed_registry_is_refused() {
        for (name, json) in [
            ("unparsable", "{"),
            ("noexec", r#"{"schema_version":1,"commands":[{"name":"state","kind":"read","version":1}]}"#),
            ("kind", r#"{"schema_version":1,"commands":[{"name":"state","kind":"run","exec":"/x"}]}"#),
            ("relative", r#"{"schema_version":1,"commands":[{"name":"a","kind":"act","exec":"x"}]}"#),
            ("readargs", r#"{"schema_version":1,"commands":[{"name":"s","kind":"read","version":1,"exec":"/x","args":{"a":{"kind":"bool"}}}]}"#),
            ("noversion", r#"{"schema_version":1,"commands":[{"name":"s","kind":"read","exec":"/x"}]}"#),
            ("timeout", r#"{"schema_version":1,"commands":[{"name":"a","kind":"act","exec":"/x","timeout":121}]}"#),
            ("twice", r#"{"schema_version":1,"commands":[{"name":"a","kind":"act","exec":"/x"},{"name":"a","kind":"act","exec":"/x"}]}"#),
            ("schema", r#"{"schema_version":2,"commands":[]}"#),
            ("certs", r#"{"schema_version":1,"commands":[{"name":"a","kind":"act","exec":"/x"}],"certificates":{"a":["c"]}}"#),
        ] {
            let f = Fixture::new(name, json);
            assert!(matches!(load(&f.table(), &f.dir, "ddns"), Err(Refused::Unusable(_))), "{name}");
        }
    }

    #[test]
    fn the_sdk_fixture_loads() {
        let fixture = concat!(env!("CARGO_MANIFEST_DIR"), "/../plugins/verso-plugin-sdk/testdata/commands.json");
        let f = Fixture::new("sdk", &fs::read_to_string(fixture).unwrap());
        let r = load(&f.table(), &f.dir, "ddns").unwrap();
        assert_eq!(r.command("state").unwrap().kind, Kind::Read);
        assert_eq!(r.command("update").unwrap().args.len(), 8);
        assert_eq!(r.command("files").unwrap().kind, Kind::Transform);
        assert_eq!(r.certificates["state"], ["certificate"]);
    }
}
