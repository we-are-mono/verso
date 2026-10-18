// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

//! The command registry (ADR-027): the root work a plugin ships, declared in
//! `/usr/share/verso/plugins/<id>/commands.json` beside its manifest. The
//! privileged helper runs a registered command as root, only for a session
//! holding the plugin's grant, and only from root-owned files.
//!
//! - A **read** takes no arguments and is brokered to pages under
//!   `<plugin>.<name>` with its declared version.
//! - An **act** takes the arguments its schema names, each checked against an
//!   [`ArgKind`] before anything runs.
//! - A **transform** is the helper's own: it shapes a file family's files on
//!   their way out of root and back in, and no session can ask for it.
//!
//! Build a registry with [`Registry::new`], then write it with
//! [`Registry::to_json`], which refuses one the helper would refuse.

use crate::args::ArgKind;
use serde::{Deserialize, Serialize};
use std::collections::BTreeMap;

/// The registry format this SDK writes.
pub const REGISTRY_SCHEMA_VERSION: u32 = 1;

/// The longest timeout a command may declare, in seconds.
pub const MAX_TIMEOUT: u32 = 120;

/// What a command is for.
#[derive(Serialize, Deserialize, Debug, Clone, Copy, PartialEq, Eq)]
#[serde(rename_all = "kebab-case")]
pub enum CommandKind {
    Read,
    Act,
    Transform,
}

/// One argument an act takes: its kind, and whether it may be left out.
#[derive(Serialize, Deserialize, Debug, Clone, PartialEq)]
pub struct ArgSpec {
    #[serde(flatten)]
    pub kind: ArgKind,
    #[serde(default, skip_serializing_if = "std::ops::Not::not")]
    pub optional: bool,
}

impl ArgSpec {
    pub fn required(kind: ArgKind) -> ArgSpec {
        ArgSpec { kind, optional: false }
    }
    pub fn optional(kind: ArgKind) -> ArgSpec {
        ArgSpec { kind, optional: true }
    }
}

/// One root command: what runs (`exec` with `argv`), and what it takes.
#[derive(Serialize, Deserialize, Debug, Clone, PartialEq)]
pub struct Command {
    pub name: String,
    pub kind: CommandKind,
    /// A read's version, raised only when its result changes incompatibly.
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub version: Option<u32>,
    pub exec: String,
    #[serde(default, skip_serializing_if = "Vec::is_empty")]
    pub argv: Vec<String>,
    /// Seconds before the helper stops the command; the helper's default when absent.
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub timeout: Option<u32>,
    #[serde(default, skip_serializing_if = "BTreeMap::is_empty")]
    pub args: BTreeMap<String, ArgSpec>,
}

impl Command {
    /// A read at `version`, run as `exec root`.
    pub fn read(name: &str, version: u32, exec: &str) -> Command {
        Command::new(name, CommandKind::Read, Some(version), exec)
    }
    /// An act, run as `exec root`; add its arguments with [`Command::arg`].
    pub fn act(name: &str, exec: &str) -> Command {
        Command::new(name, CommandKind::Act, None, exec)
    }
    /// A file family's transform, run as `exec root`.
    pub fn transform(name: &str, exec: &str) -> Command {
        Command::new(name, CommandKind::Transform, None, exec)
    }
    fn new(name: &str, kind: CommandKind, version: Option<u32>, exec: &str) -> Command {
        Command {
            name: name.into(),
            kind,
            version,
            exec: exec.into(),
            argv: vec!["root".into()],
            timeout: None,
            args: BTreeMap::new(),
        }
    }
    pub fn arg(mut self, name: &str, spec: ArgSpec) -> Command {
        self.args.insert(name.into(), spec);
        self
    }
    pub fn timeout(mut self, seconds: u32) -> Command {
        self.timeout = Some(seconds);
        self
    }
    pub fn argv(mut self, argv: &[&str]) -> Command {
        self.argv = argv.iter().map(|a| a.to_string()).collect();
        self
    }
}

/// How the names of a family's files in its directory may be spelled.
#[derive(Serialize, Deserialize, Debug, Clone, Copy, PartialEq, Eq)]
#[serde(rename_all = "kebab-case")]
pub enum FileNames {
    /// Up to 64 of lower-case letters, digits, `_` and `-`, starting alphanumeric.
    Plain,
    /// Up to 96 of letters, digits, `_`, `-` and `.`, starting alphanumeric:
    /// a provider's download keeps a name like `nl-free-12.protonvpn.udp.ovpn`.
    Dotted,
}

/// The directory a family's other files live in, directly.
#[derive(Serialize, Deserialize, Debug, Clone, PartialEq)]
pub struct FamilyDir {
    pub path: String,
    pub suffixes: Vec<String>,
    pub names: FileNames,
}

/// The refusals a family's editor shows, in the words its files are called by;
/// the owning plugin's catalogue translates them.
#[derive(Serialize, Deserialize, Debug, Clone, PartialEq)]
pub struct FamilyWords {
    pub too_large: String,
    pub invalid: String,
    pub over_limit: String,
}

/// One daemon's hand-edited files, owned by a uci config.
#[derive(Serialize, Deserialize, Debug, Clone, PartialEq)]
pub struct Family {
    pub name: String,
    pub config: String,
    #[serde(default, skip_serializing_if = "Vec::is_empty")]
    pub files: Vec<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub dir: Option<FamilyDir>,
    /// `"0644"`, or `"0600"` for files that hold keys.
    pub mode: String,
    pub words: FamilyWords,
    /// Checks the daemon's whole configuration with the files in place.
    #[serde(default, skip_serializing_if = "Vec::is_empty")]
    pub check: Vec<String>,
    /// Has the daemon read its files again.
    pub reload: Vec<String>,
    /// A `transform` command of the same registry.
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub transform: Option<String>,
}

/// A plugin's whole registry.
#[derive(Serialize, Deserialize, Debug, Clone, PartialEq)]
pub struct Registry {
    pub schema_version: u32,
    #[serde(default)]
    pub commands: Vec<Command>,
    #[serde(default, skip_serializing_if = "Vec::is_empty")]
    pub families: Vec<Family>,
    /// Per read, the fields of its result that hold a PEM certificate, which
    /// the shell turns into the facts the certificate card shows.
    #[serde(default, skip_serializing_if = "BTreeMap::is_empty")]
    pub certificates: BTreeMap<String, Vec<String>>,
}

impl Default for Registry {
    fn default() -> Registry {
        Registry::new()
    }
}

impl Registry {
    pub fn new() -> Registry {
        Registry {
            schema_version: REGISTRY_SCHEMA_VERSION,
            commands: vec![],
            families: vec![],
            certificates: BTreeMap::new(),
        }
    }
    pub fn command(mut self, command: Command) -> Registry {
        self.commands.push(command);
        self
    }
    pub fn family(mut self, family: Family) -> Registry {
        self.families.push(family);
        self
    }
    /// Names a field of a read's result that holds a PEM certificate.
    pub fn certificate(mut self, read: &str, field: &str) -> Registry {
        self.certificates.entry(read.into()).or_default().push(field.into());
        self
    }

    /// check refuses a registry the helper would refuse, naming the first
    /// reason. The helper's own bounds on where a family may place files are
    /// its alone; this checks the shape.
    pub fn check(&self) -> Result<(), String> {
        if self.schema_version != REGISTRY_SCHEMA_VERSION {
            return Err(format!("schema_version must be {REGISTRY_SCHEMA_VERSION}"));
        }
        let mut names = std::collections::BTreeSet::new();
        for command in &self.commands {
            check_command(command)?;
            if !names.insert(command.name.as_str()) {
                return Err(format!("command {:?} is declared twice", command.name));
            }
        }
        let kind_of = |name: &str| self.commands.iter().find(|c| c.name == name).map(|c| c.kind);
        let mut families = std::collections::BTreeSet::new();
        for family in &self.families {
            check_family(family)?;
            if !families.insert(family.name.as_str()) {
                return Err(format!("family {:?} is declared twice", family.name));
            }
            if let Some(transform) = &family.transform {
                if kind_of(transform) != Some(CommandKind::Transform) {
                    return Err(format!("family {:?}: {transform:?} is not a transform", family.name));
                }
            }
        }
        for (read, fields) in &self.certificates {
            if kind_of(read) != Some(CommandKind::Read) {
                return Err(format!("certificates: {read:?} is not a read"));
            }
            if fields.is_empty() || fields.iter().any(|f| f.is_empty()) {
                return Err(format!("certificates: {read:?} names no field"));
            }
        }
        Ok(())
    }

    /// to_json is the registry as `commands.json` holds it, or the reason the
    /// helper would refuse it.
    pub fn to_json(&self) -> Result<String, String> {
        self.check()?;
        serde_json::to_string_pretty(self).map_err(|e| e.to_string())
    }
}

/// A command or family name: 1–64 of lower-case letters, digits and `-`,
/// starting with a letter — the function part of an rpcd grant.
pub fn valid_name(name: &str) -> bool {
    !name.is_empty()
        && name.len() <= 64
        && name.as_bytes()[0].is_ascii_lowercase()
        && name.bytes().all(|b| b.is_ascii_lowercase() || b.is_ascii_digit() || b == b'-')
}

/// An absolute path with no `.` or `..` part, no empty part and no NUL.
pub fn absolute_path(path: &str) -> bool {
    path.len() > 1
        && path.starts_with('/')
        && !path.contains('\0')
        && path[1..].split('/').all(|part| !part.is_empty() && part != "." && part != "..")
}

fn check_command(c: &Command) -> Result<(), String> {
    let at = |why: &str| format!("command {:?}: {why}", c.name);
    if !valid_name(&c.name) {
        return Err(at("name must be lower-case letters, digits and -, starting with a letter"));
    }
    if !absolute_path(&c.exec) {
        return Err(at("exec must be an absolute path"));
    }
    if c.argv.iter().any(|a| a.contains('\0')) {
        return Err(at("argv holds a NUL"));
    }
    if let Some(t) = c.timeout {
        if t == 0 || t > MAX_TIMEOUT {
            return Err(at(&format!("timeout must be 1–{MAX_TIMEOUT} seconds")));
        }
    }
    match c.kind {
        CommandKind::Read => {
            if !c.args.is_empty() {
                return Err(at("a read takes no arguments"));
            }
            if !matches!(c.version, Some(v) if v > 0) {
                return Err(at("a read needs a version of 1 or more"));
            }
        }
        CommandKind::Act | CommandKind::Transform => {
            if c.version.is_some() {
                return Err(at("only a read has a version"));
            }
            if c.kind == CommandKind::Transform && !c.args.is_empty() {
                return Err(at("a transform takes no arguments"));
            }
        }
    }
    for (name, spec) in &c.args {
        if name.is_empty() || !name.bytes().all(|b| b.is_ascii_alphanumeric() || b == b'_') {
            return Err(at(&format!("argument {name:?} must be letters, digits and _")));
        }
        spec.kind.check().map_err(|why| at(&format!("argument {name:?}: {why}")))?;
    }
    Ok(())
}

fn check_family(f: &Family) -> Result<(), String> {
    let at = |why: &str| format!("family {:?}: {why}", f.name);
    if !valid_name(&f.name) {
        return Err(at("name must be lower-case letters, digits and -, starting with a letter"));
    }
    let config_ok = !f.config.is_empty()
        && f.config.bytes().all(|b| b.is_ascii_alphanumeric() || b == b'_' || b == b'-');
    if !config_ok {
        return Err(at("config must be a uci config name"));
    }
    if f.files.is_empty() && f.dir.is_none() {
        return Err(at("a family needs files or a dir"));
    }
    if let Some(bad) = f.files.iter().find(|p| !absolute_path(p)) {
        return Err(at(&format!("{bad:?} is not an absolute path")));
    }
    if let Some(dir) = &f.dir {
        if !absolute_path(&dir.path) {
            return Err(at("dir must be an absolute path"));
        }
        let suffix_ok = |s: &String| {
            s.len() > 1 && s.starts_with('.') && s[1..].bytes().all(|b| b.is_ascii_alphanumeric())
        };
        if dir.suffixes.is_empty() || !dir.suffixes.iter().all(suffix_ok) {
            return Err(at("dir needs suffixes such as \".conf\""));
        }
    }
    if f.mode != "0644" && f.mode != "0600" {
        return Err(at("mode must be \"0644\" or \"0600\""));
    }
    let w = &f.words;
    if w.too_large.is_empty() || w.invalid.is_empty() || w.over_limit.is_empty() {
        return Err(at("every refusal needs its words"));
    }
    for (what, argv) in [("check", &f.check), ("reload", &f.reload)] {
        if what == "reload" && argv.is_empty() {
            return Err(at("a family needs a reload"));
        }
        if let Some(program) = argv.first() {
            if !absolute_path(program) || argv.iter().any(|a| a.contains('\0')) {
                return Err(at(&format!("{what} must start with an absolute program")));
            }
        }
    }
    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;

    const EXEC: &str = "/usr/bin/verso-plugin-ddns";

    fn words() -> FamilyWords {
        FamilyWords {
            too_large: "Rule files must be at most 32 KiB.".into(),
            invalid: "Invalid rule file.".into(),
            over_limit: "Rule files exceed the editor size limit.".into(),
        }
    }

    fn fw4() -> Family {
        Family {
            name: "fw4".into(),
            config: "firewall".into(),
            files: vec![],
            dir: Some(FamilyDir {
                path: "/etc/nftables.d".into(),
                suffixes: vec![".nft".into()],
                names: FileNames::Plain,
            }),
            mode: "0644".into(),
            words: words(),
            check: vec!["/sbin/fw4".into(), "check".into()],
            reload: vec!["/sbin/fw4".into(), "-q".into(), "restart".into()],
            transform: None,
        }
    }

    #[test]
    fn a_read_with_arguments_is_refused() {
        let r = Registry::new()
            .command(Command::read("state", 1, EXEC).arg("x", ArgSpec::required(ArgKind::Bool)));
        assert!(r.to_json().unwrap_err().contains("a read takes no arguments"));
    }

    #[test]
    fn a_read_needs_a_positive_version() {
        let r = Registry::new().command(Command::read("state", 0, EXEC));
        assert!(r.to_json().unwrap_err().contains("version"));
        let mut missing = Command::read("state", 1, EXEC);
        missing.version = None;
        assert!(Registry::new().command(missing).to_json().is_err());
    }

    #[test]
    fn only_a_read_has_a_version() {
        let mut act = Command::act("update", EXEC);
        act.version = Some(1);
        assert!(Registry::new().command(act).to_json().unwrap_err().contains("only a read"));
    }

    #[test]
    fn exec_must_be_absolute() {
        for exec in ["verso-plugin-ddns", "/usr/bin/../bin/sh", "/usr//bin/x", "/", ""] {
            let r = Registry::new().command(Command::act("update", exec));
            assert!(r.to_json().is_err(), "{exec:?}");
        }
    }

    #[test]
    fn names_are_rpcd_function_words() {
        for name in ["", "State", "1st", "a_b", "a.b"] {
            let r = Registry::new().command(Command::read(name, 1, EXEC));
            assert!(r.to_json().is_err(), "{name:?}");
        }
        assert!(Registry::new().command(Command::read("dhcp-state", 1, EXEC)).to_json().is_ok());
    }

    #[test]
    fn a_command_is_declared_once() {
        let r = Registry::new()
            .command(Command::read("state", 1, EXEC))
            .command(Command::act("state", EXEC));
        assert!(r.to_json().unwrap_err().contains("twice"));
    }

    #[test]
    fn timeouts_are_bounded() {
        assert!(Registry::new().command(Command::act("a", EXEC).timeout(0)).to_json().is_err());
        assert!(Registry::new().command(Command::act("a", EXEC).timeout(121)).to_json().is_err());
        assert!(Registry::new().command(Command::act("a", EXEC).timeout(120)).to_json().is_ok());
    }

    #[test]
    fn a_family_transform_names_a_transform() {
        let mut family = fw4();
        family.transform = Some("state".into());
        let r = Registry::new().command(Command::read("state", 1, EXEC)).family(family.clone());
        assert!(r.to_json().unwrap_err().contains("not a transform"));
        family.transform = Some("files".into());
        let r = Registry::new().command(Command::transform("files", EXEC)).family(family);
        assert!(r.to_json().is_ok());
    }

    #[test]
    fn a_family_needs_its_places_mode_and_reload() {
        let mut f = fw4();
        f.dir = None;
        assert!(Registry::new().family(f).to_json().is_err());
        let mut f = fw4();
        f.mode = "0666".into();
        assert!(Registry::new().family(f).to_json().is_err());
        let mut f = fw4();
        f.reload = vec![];
        assert!(Registry::new().family(f).to_json().is_err());
        let mut f = fw4();
        f.dir.as_mut().unwrap().suffixes = vec!["nft".into()];
        assert!(Registry::new().family(f).to_json().is_err());
        assert!(Registry::new().family(fw4()).to_json().is_ok());
    }

    #[test]
    fn certificates_name_a_read() {
        let r = Registry::new().command(Command::act("update", EXEC)).certificate("update", "certificate");
        assert!(r.to_json().is_err());
        let r = Registry::new().command(Command::read("state", 1, EXEC)).certificate("state", "certificate");
        assert!(r.to_json().is_ok());
    }

    #[test]
    fn the_file_reads_back() {
        let r = Registry::new()
            .command(Command::read("state", 1, EXEC))
            .command(
                Command::act("update", EXEC)
                    .timeout(30)
                    .arg("section", ArgSpec::required(ArgKind::UciName)),
            )
            .family(fw4());
        let back: Registry = serde_json::from_str(&r.to_json().unwrap()).unwrap();
        assert_eq!(back, r);
    }
}
