// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.
//! Bounded, root-owned file stage with a durable rollback journal. Saving never
//! changes active configuration. Recovery runs even when the shell disappears.
use crate::Failure;
use serde_json::{json, Value};
use std::{
    fs,
    io::{Read, Write},
    os::unix::fs::{OpenOptionsExt, PermissionsExt},
    path::{Path, PathBuf},
    process::{Command, Stdio},
    sync::Mutex,
    time::{SystemTime, UNIX_EPOCH},
};
static LOCK: Mutex<()> = Mutex::new(());
// OpenWrt links /var to /tmp. Use the physical runtime path so symlink
// protection for user-editable configuration does not reject the journal.
const JOURNAL: &str = "/tmp/run/verso-config-files/journal.json";
const LIMIT: usize = 32768;
const TOTAL: usize = 128 * 1024;
fn failure(e: impl std::fmt::Display) -> Failure {
    Failure::unknown(e.to_string())
}
fn now() -> u64 {
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .unwrap_or_default()
        .as_secs()
}
/// A family is one daemon's hand-edited files: where they may live, how the
/// daemon's whole configuration is checked with them in place, and how it is
/// told to read them again. A path belongs to one family or to none, and
/// nothing outside a family is ever read, written or restored.
#[derive(Clone, Copy, Debug, PartialEq, Eq, PartialOrd, Ord)]
pub enum Family {
    Dnsmasq,
    Fw4,
}

impl Family {
    const ALL: [Family; 2] = [Family::Dnsmasq, Family::Fw4];

    pub fn of(path: &str) -> Option<Family> {
        if path == "/etc/dnsmasq.conf" || in_dir(path, "/etc/dnsmasq.d/", ".conf") {
            Some(Family::Dnsmasq)
        } else if in_dir(path, "/etc/nftables.d/", ".nft") {
            Some(Family::Fw4)
        } else {
            None
        }
    }

    pub fn named(name: &str) -> Option<Family> {
        Family::ALL.into_iter().find(|f| f.name() == name)
    }

    pub fn name(self) -> &'static str {
        match self {
            Family::Dnsmasq => "dnsmasq",
            Family::Fw4 => "fw4",
        }
    }

    /// The files the family always has, and the folder its others live in.
    fn places(self) -> (&'static [&'static str], &'static str) {
        match self {
            Family::Dnsmasq => (&["/etc/dnsmasq.conf"], "/etc/dnsmasq.d"),
            Family::Fw4 => (&[], "/etc/nftables.d"),
        }
    }

    /// check_draft checks one file on its own before it is staged. dnsmasq
    /// reads a file of options by itself; an nftables snippet names fw4's own
    /// chains and sets, so it can only be checked inside the ruleset, which
    /// check_live does when the change is applied.
    fn check_draft(self, body: &str) -> Result<(), Failure> {
        match self {
            Family::Dnsmasq => validate_dnsmasq(body),
            Family::Fw4 => Ok(()),
        }
    }

    /// check_live checks the daemon's whole configuration with the written
    /// files in place.
    fn check_live(self) -> Result<(), Failure> {
        let (mut command, refusal) = match self {
            Family::Dnsmasq => {
                let mut command = Command::new("/usr/sbin/dnsmasq");
                command.args(["--test", "--conf-file=/etc/dnsmasq.conf"]);
                if Path::new("/etc/dnsmasq.d").is_dir() {
                    command.arg("--conf-dir=/etc/dnsmasq.d");
                }
                (command, "dnsmasq rejected the custom options")
            }
            Family::Fw4 => {
                let mut command = Command::new("/sbin/fw4");
                command.arg("check");
                (command, "fw4 rejected the rule files")
            }
        };
        let check = command.stdin(Stdio::null()).output().map_err(failure)?;
        if !check.status.success() {
            let said = [check.stderr, check.stdout].concat();
            return Err(Failure::invalid(format!(
                "{refusal}: {}",
                complaint(&String::from_utf8_lossy(&said))
            )));
        }
        Ok(())
    }

    /// The refusals a family's editor shows, each in the words its files are
    /// called by, whole so a plugin's catalog can translate them.
    fn too_large(self) -> &'static str {
        match self {
            Family::Dnsmasq => "Custom option files must be at most 32 KiB.",
            Family::Fw4 => "Rule files must be at most 32 KiB.",
        }
    }
    fn invalid(self) -> &'static str {
        match self {
            Family::Dnsmasq => "Invalid custom options file.",
            Family::Fw4 => "Invalid rule file.",
        }
    }
    fn over_limit(self) -> &'static str {
        match self {
            Family::Dnsmasq => "Custom option files exceed the editor size limit.",
            Family::Fw4 => "Rule files exceed the editor size limit.",
        }
    }

    /// reload has the daemon read its files again, so what runs is exactly
    /// what they say. fw4's own reload only flushes its table, which empties
    /// chains but keeps them: a chain a file no longer declares — a base chain
    /// with a drop policy, say, undone by a rollback — would stay hooked in.
    /// fw4's restart checks the ruleset, then rebuilds the table whole.
    fn reload(self) {
        let (program, args): (&str, &[&str]) = match self {
            Family::Dnsmasq => ("/etc/init.d/dnsmasq", &["reload"]),
            Family::Fw4 => ("/sbin/fw4", &["-q", "restart"]),
        };
        let _ = Command::new(program)
            .args(args)
            .stdin(Stdio::null())
            .stdout(Stdio::null())
            .stderr(Stdio::null())
            .status();
    }
}

/// complaint is what a check said about the files, without the notices it
/// prints on every run (fw4's `[!]` lines: a disabled rule, a package's
/// snippet included), which say nothing about why the check failed.
fn complaint(said: &str) -> String {
    said.lines()
        .filter(|line| !line.trim_start().starts_with("[!]"))
        .collect::<Vec<_>>()
        .join("\n")
        .trim()
        .to_string()
}

/// in_dir reports whether path is a file directly in dir with the suffix,
/// named as a person would name one: lowercase letters, digits, dashes and
/// underscores, starting with a letter or digit.
fn in_dir(path: &str, dir: &str, suffix: &str) -> bool {
    path.strip_prefix(dir)
        .and_then(|s| s.strip_suffix(suffix))
        .is_some_and(|name| {
            !name.is_empty()
                && name.len() <= 64
                && name.as_bytes()[0].is_ascii_alphanumeric()
                && name
                    .bytes()
                    .all(|b| b.is_ascii_lowercase() || b.is_ascii_digit() || b == b'_' || b == b'-')
        })
}

fn allowed(path: &str) -> bool {
    Family::of(path).is_some()
}

/// touched is every family a journal's files belong to, once each, in order.
fn touched(j: &Value) -> Vec<Family> {
    let mut families: Vec<Family> = j["files"]
        .as_object()
        .map(|files| files.keys().filter_map(|p| Family::of(p)).collect())
        .unwrap_or_default();
    families.sort();
    families.dedup();
    families
}

/// drop_family unstages one family's files, or every file when none is named.
fn drop_family(j: &mut Value, family: Option<Family>) {
    if let Some(files) = j["files"].as_object_mut() {
        files.retain(|path, _| family.is_some_and(|f| Family::of(path) != Some(f)));
    }
}

fn reload_all(families: &[Family]) {
    for family in families {
        family.reload();
    }
}
fn check_path(path: &Path) -> Result<(), Failure> {
    for p in path.ancestors() {
        match fs::symlink_metadata(p) {
            Ok(m) if m.file_type().is_symlink() => {
                return Err(Failure::invalid("Symbolic links are not editable."))
            }
            Ok(_) => {}
            Err(e) if e.kind() == std::io::ErrorKind::NotFound => {}
            Err(e) => return Err(failure(e)),
        }
    }
    Ok(())
}
fn read(path: &Path) -> Result<Option<String>, Failure> {
    check_path(path)?;
    let mut file = match fs::File::open(path) {
        Ok(f) => f,
        Err(e) if e.kind() == std::io::ErrorKind::NotFound => return Ok(None),
        Err(e) => return Err(failure(e)),
    };
    if !file.metadata().map_err(failure)?.is_file() {
        return Err(Failure::invalid("Not a regular file."));
    }
    let mut bytes = Vec::new();
    Read::by_ref(&mut file)
        .take((LIMIT + 1) as u64)
        .read_to_end(&mut bytes)
        .map_err(failure)?;
    if bytes.len() > LIMIT {
        let family = path
            .to_str()
            .and_then(Family::of)
            .unwrap_or(Family::Dnsmasq);
        return Err(Failure::invalid(family.too_large()));
    }
    String::from_utf8(bytes)
        .map(Some)
        .map_err(|_| Failure::invalid("The file must contain UTF-8 text."))
}
fn version(body: Option<&str>) -> String {
    let mut h = 0xcbf29ce484222325u64;
    for b in body.unwrap_or("").bytes().chain([u8::from(body.is_some())]) {
        h = (h ^ b as u64).wrapping_mul(0x100000001b3);
    }
    format!("{h:016x}")
}
fn atomic(path: &Path, body: &str, mode: u32) -> Result<(), Failure> {
    check_path(path)?;
    let parent = path
        .parent()
        .ok_or_else(|| Failure::invalid("Missing parent directory."))?;
    fs::create_dir_all(parent).map_err(failure)?;
    let nonce = SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .unwrap_or_default()
        .as_nanos();
    let tmp = parent.join(format!(".verso-{}-{nonce}-tmp", std::process::id()));
    let mut f = fs::OpenOptions::new()
        .write(true)
        .create_new(true)
        .mode(mode)
        .open(&tmp)
        .map_err(failure)?;
    let result = (|| {
        f.write_all(body.as_bytes()).map_err(failure)?;
        f.sync_all().map_err(failure)?;
        fs::rename(&tmp, path).map_err(failure)?;
        fs::File::open(parent)
            .and_then(|d| d.sync_all())
            .map_err(failure)
    })();
    let _ = fs::remove_file(&tmp);
    result
}
fn load_at(path: &Path) -> Result<Value, Failure> {
    match fs::read(path) {
        Ok(bytes) => serde_json::from_slice(&bytes).map_err(failure),
        Err(e) if e.kind() == std::io::ErrorKind::NotFound => {
            Ok(json!({"files":{},"active":false}))
        }
        Err(e) => Err(failure(e)),
    }
}
fn save_at(path: &Path, journal: &Value) -> Result<(), Failure> {
    atomic(path, &journal.to_string(), 0o600)
}
fn restore(journal: &Value) -> Result<(), Failure> {
    for (path, file) in journal["files"]
        .as_object()
        .ok_or_else(|| Failure::unknown("Invalid file journal."))?
    {
        if !allowed(path) {
            return Err(Failure::invalid("Invalid file journal path."));
        }
        restore_file(Path::new(path), file)?;
    }
    Ok(())
}
fn restore_file(path: &Path, file: &Value) -> Result<(), Failure> {
    if let Some(body) = file["before"].as_str() {
        atomic(path, body, file["mode"].as_u64().unwrap_or(0o644) as u32)
    } else {
        check_path(path)?;
        match fs::remove_file(path) {
            Ok(()) => Ok(()),
            Err(e) if e.kind() == std::io::ErrorKind::NotFound => Ok(()),
            Err(e) => Err(failure(e)),
        }
    }
}
/// recover rolls back an apply whose confirmation window has passed, and
/// answers with the families it restored, which the caller reloads.
fn recover(j: &mut Value) -> Result<Vec<Family>, Failure> {
    if j["active"] == true && j["deadline"].as_u64().unwrap_or(0) <= now() {
        restore(j)?;
        j["active"] = json!(false);
        j["uci_confirmed"] = json!(false);
        save_at(Path::new(JOURNAL), j)?;
        return Ok(touched(j));
    }
    Ok(Vec::new())
}
pub fn watchdog() {
    std::thread::spawn(|| loop {
        std::thread::sleep(std::time::Duration::from_secs(1));
        let restored = {
            let Ok(_guard) = LOCK.lock() else {
                continue;
            };
            load_at(Path::new(JOURNAL))
                .and_then(|mut j| recover(&mut j))
                .unwrap_or_else(|e| {
                    eprintln!("verso-rpcd: file rollback failed: {}", e.message);
                    Vec::new()
                })
        };
        reload_all(&restored);
    });
}
/// state is every family's files as they will read once staged changes are
/// applied, each saying which family it belongs to.
pub fn state() -> Result<Value, Failure> {
    let _guard = LOCK.lock().map_err(failure)?;
    let mut j = load_at(Path::new(JOURNAL))?;
    reload_all(&recover(&mut j)?);
    let mut paths = vec![];
    for family in Family::ALL {
        let (fixed, dir) = family.places();
        paths.extend(fixed.iter().map(PathBuf::from));
        check_path(Path::new(dir))?;
        if let Ok(entries) = fs::read_dir(dir) {
            paths.extend(
                entries
                    .filter_map(Result::ok)
                    .map(|e| e.path())
                    .filter(|p| p.to_str().is_some_and(allowed)),
            );
        }
    }
    if let Some(files) = j["files"].as_object() {
        paths.extend(files.keys().map(PathBuf::from));
    }
    paths.sort();
    paths.dedup();
    paths.sort_by_key(|p| {
        !j["files"]
            .as_object()
            .is_some_and(|f| f.contains_key(p.to_string_lossy().as_ref()))
    });
    if paths.len() > 64 {
        return Err(Failure::invalid("Too many custom option files."));
    }
    let mut files = vec![];
    let mut total = 0;
    for path in paths {
        let key = path.to_string_lossy();
        let Some(family) = Family::of(&key) else {
            continue;
        };
        let active = match read(&path) {
            Ok(body) => body,
            Err(e) if j["files"].get(key.as_ref()).is_none() => {
                files.push(json!({"path":key,"family":family.name(),"error":e.message}));
                continue;
            }
            Err(_) => None,
        };
        let body = j["files"][key.as_ref()]["after"]
            .as_str()
            .map(String::from)
            .or(active);
        total += body.as_ref().map(String::len).unwrap_or(0);
        if total > TOTAL {
            files.push(json!({"path":key,"family":family.name(),"error":family.over_limit()}));
            continue;
        }
        files.push(json!({"path":key,"family":family.name(),"content":body.as_deref().unwrap_or(""),"version":version(body.as_deref()),"pending":j["files"].get(key.as_ref()).is_some(),"exists":body.is_some()}));
    }
    Ok(
        json!({"files":files,"active":j["active"],"uci":j["uci"],"uci_confirmed":j["uci_confirmed"]}),
    )
}
fn validate_dnsmasq(body: &str) -> Result<(), Failure> {
    let dir = Path::new(JOURNAL).parent().unwrap();
    check_path(dir)?;
    fs::create_dir_all(dir).map_err(failure)?;
    fs::set_permissions(dir, fs::Permissions::from_mode(0o700)).map_err(failure)?;
    let file = dir.join("validate.conf");
    atomic(&file, body, 0o600)?;
    let result = Command::new("/usr/sbin/dnsmasq")
        .arg("--test")
        .arg(format!("--conf-file={}", file.display()))
        .stdin(Stdio::null())
        .stdout(Stdio::null())
        .stderr(Stdio::null())
        .status();
    let _ = fs::remove_file(file);
    if !result.map_err(failure)?.success() {
        return Err(Failure::invalid(
            "dnsmasq rejected these options. Check the option names and values.",
        ));
    }
    Ok(())
}
pub fn stage(path: &str, expected: &str, body: &str) -> Result<Value, Failure> {
    let Some(family) = Family::of(path) else {
        return Err(Failure::invalid("Invalid custom options file."));
    };
    if body.len() > LIMIT || body.contains('\0') {
        return Err(Failure::invalid(family.invalid()));
    }
    let _guard = LOCK.lock().map_err(failure)?;
    let mut j = load_at(Path::new(JOURNAL))?;
    reload_all(&recover(&mut j)?);
    if j["active"] == true {
        return Err(Failure::invalid("Wait for the current apply to finish."));
    }
    let before = read(Path::new(path))?;
    let previous = j["files"][path]["after"].as_str().or(before.as_deref());
    if version(previous) != expected {
        return Err(Failure::invalid(
            "This file changed. Reopen it before saving.",
        ));
    }
    if j["files"]
        .as_object()
        .is_some_and(|f| f.len() >= 32 && !f.contains_key(path))
    {
        return Err(Failure::invalid("Too many staged files."));
    }
    family.check_draft(body)?;
    let mode = fs::metadata(path)
        .map(|m| m.permissions().mode() & 0o777)
        .unwrap_or(0o644);
    let original = j["files"][path]
        .get("before")
        .cloned()
        .unwrap_or(json!(before));
    if original.as_str() == Some(body) {
        j["files"].as_object_mut().unwrap().remove(path);
    } else {
        j["files"][path] = json!({"before":original,"after":body,"mode":mode});
    }
    let total: usize = j["files"]
        .as_object()
        .unwrap()
        .values()
        .map(|f| f["after"].as_str().unwrap_or("").len())
        .sum();
    if total > TOTAL {
        return Err(Failure::invalid(match family {
            Family::Dnsmasq => "Staged custom options exceed the editor size limit.",
            Family::Fw4 => family.over_limit(),
        }));
    }
    save_at(Path::new(JOURNAL), &j)?;
    Ok(json!({"result":true}))
}
/// lifecycle carries staged files through an apply. On apply it writes every
/// staged file, checks each touched family's whole configuration with them in
/// place, and has each family read them again; a refusal restores the files
/// before anything reloads. Abort and the watchdog restore the same way.
/// A discard names the family whose files it drops — the one whose config was
/// discarded — or none, to drop every staged file.
pub fn lifecycle(
    action: &str,
    uci: bool,
    timeout: u64,
    family: Option<Family>,
) -> Result<Value, Failure> {
    let _guard = LOCK.lock().map_err(failure)?;
    let mut j = load_at(Path::new(JOURNAL))?;
    reload_all(&recover(&mut j)?);
    match action {
        "apply" => {
            if j["active"] == true {
                return Err(Failure::invalid("An apply is already in progress."));
            }
            if j["files"].as_object().is_none_or(|f| f.is_empty()) {
                return Ok(json!({"active":false}));
            }
            for (path, file) in j["files"].as_object().unwrap() {
                if !allowed(path) || read(Path::new(path))?.as_deref() != file["before"].as_str() {
                    return Err(Failure::invalid(
                        "A file changed outside Verso. Discard and reopen it.",
                    ));
                }
            }
            j["active"] = json!(true);
            j["uci"] = json!(uci);
            j["uci_confirmed"] = json!(false);
            j["deadline"] = json!(now() + timeout.clamp(5, 60));
            save_at(Path::new(JOURNAL), &j)?;
            let result = (|| {
                for (path, file) in j["files"].as_object().unwrap() {
                    atomic(
                        Path::new(path),
                        file["after"].as_str().unwrap(),
                        file["mode"].as_u64().unwrap_or(0o644) as u32,
                    )?;
                }
                touched(&j).into_iter().try_for_each(Family::check_live)
            })();
            if let Err(e) = result {
                restore(&j)?;
                j["active"] = json!(false);
                save_at(Path::new(JOURNAL), &j)?;
                return Err(e);
            }
            // Every family the change touched reads its files again, whether
            // or not the uci apply beside it touches that daemon's config.
            reload_all(&touched(&j));
        }
        "uci-confirmed" => {
            j["uci_confirmed"] = json!(true);
            save_at(Path::new(JOURNAL), &j)?;
        }
        "confirm" => {
            if j["active"] != true {
                return Err(Failure::invalid("The file rollback window expired."));
            }
            save_at(Path::new(JOURNAL), &json!({"files":{},"active":false}))?;
        }
        "abort" => {
            if j["active"] == true {
                restore(&j)?;
                reload_all(&touched(&j));
            }
            j["active"] = json!(false);
            save_at(Path::new(JOURNAL), &j)?;
        }
        "discard" => {
            if j["active"] == true {
                return Err(Failure::invalid("An apply is in progress."));
            }
            drop_family(&mut j, family);
            save_at(Path::new(JOURNAL), &j)?;
        }
        _ => return Err(Failure::invalid("Invalid file transaction action.")),
    }
    Ok(json!({"active": j["active"]}))
}
pub fn dns_state() -> Result<Value, Failure> {
    let features = Command::new("/usr/sbin/dnsmasq")
        .arg("--version")
        .stdin(Stdio::null())
        .output()
        .ok();
    let dnssec = features.as_ref().is_some_and(|o| {
        String::from_utf8_lossy(&o.stdout)
            .split_whitespace()
            .any(|w| w == "DNSSEC")
    });
    let mut result = only(state()?, Family::Dnsmasq);
    result["resolver"] = json!(Path::new("/usr/sbin/dnsmasq").is_file());
    result["dnssec"] = json!(dnssec);
    result["doh"] = json!(Path::new("/usr/sbin/https-dns-proxy").is_file());
    result["adblock"] = json!(Path::new("/etc/init.d/adblock").is_file());
    Ok(result)
}
/// firewall_state is the rule files fw4 reads from its own folder, as they
/// will read once staged changes are applied.
pub fn firewall_state() -> Result<Value, Failure> {
    let mut result = only(state()?, Family::Fw4);
    result["fw4"] = json!(Path::new("/sbin/fw4").is_file());
    Ok(result)
}

/// only narrows a state to one family's files, so a page never lists — and
/// so never offers to edit — another daemon's.
fn only(mut state: Value, family: Family) -> Value {
    if let Some(files) = state["files"].as_array_mut() {
        files.retain(|f| f["family"] == family.name());
    }
    state
}

#[cfg(test)]
mod tests {
    use super::*;
    #[test]
    fn a_refusal_keeps_the_error_and_drops_the_notices() {
        let said = "[!] Section @rule[0] (Allow-DHCP-Renew) is disabled, ignoring section\n\
[!] Automatically including '/usr/share/nftables.d/ruleset-post/x.nft'\n\
/etc/nftables.d/20-a.nft:1:27-29: Error: syntax error\nchain a { bad }\n";
        assert_eq!(
            complaint(said),
            "/etc/nftables.d/20-a.nft:1:27-29: Error: syntax error\nchain a { bad }"
        );
    }
    #[test]
    fn discarding_one_family_keeps_the_others_staged() {
        let mut j = json!({"files":{
            "/etc/nftables.d/10-a.nft":{"after":"x"},
            "/etc/dnsmasq.d/x.conf":{"after":"y"}
        },"active":false});
        drop_family(&mut j, Some(Family::Fw4));
        assert_eq!(j["files"], json!({"/etc/dnsmasq.d/x.conf":{"after":"y"}}));
        drop_family(&mut j, None);
        assert_eq!(j["files"], json!({}));
    }
    #[test]
    fn a_page_sees_only_its_own_familys_files() {
        let state = json!({"files":[
            {"path":"/etc/dnsmasq.conf","family":"dnsmasq"},
            {"path":"/etc/nftables.d/10-a.nft","family":"fw4"}
        ],"active":false});
        assert_eq!(
            only(state.clone(), Family::Fw4)["files"],
            json!([{"path":"/etc/nftables.d/10-a.nft","family":"fw4"}])
        );
        assert_eq!(
            only(state, Family::Dnsmasq)["files"],
            json!([{"path":"/etc/dnsmasq.conf","family":"dnsmasq"}])
        );
    }
    #[test]
    fn each_family_owns_its_own_paths() {
        assert_eq!(Family::of("/etc/dnsmasq.conf"), Some(Family::Dnsmasq));
        assert_eq!(
            Family::of("/etc/dnsmasq.d/10-local.conf"),
            Some(Family::Dnsmasq)
        );
        assert_eq!(
            Family::of("/etc/nftables.d/10-custom.nft"),
            Some(Family::Fw4)
        );
        for p in [
            "/etc/nftables.d/10-custom.conf",
            "/etc/dnsmasq.d/10-local.nft",
            "/etc/nftables.d/../passwd.nft",
            "/etc/nftables.d/a/b.nft",
            "/etc/nftables.d/.nft",
            "/etc/nftables.d/Rules.nft",
            "/etc/firewall.user",
            "/usr/share/nftables.d/ruleset-post/x.nft",
        ] {
            assert_eq!(Family::of(p), None, "{p}");
        }
    }
    #[test]
    fn a_change_names_every_family_it_touches_once() {
        let j = json!({"files":{
            "/etc/nftables.d/10-a.nft":{},
            "/etc/nftables.d/20-b.nft":{},
            "/etc/dnsmasq.d/x.conf":{}
        }});
        assert_eq!(touched(&j), vec![Family::Dnsmasq, Family::Fw4]);
        assert!(touched(&json!({"files":{}})).is_empty());
    }
    #[test]
    fn paths_are_bounded() {
        for p in ["/etc/dnsmasq.conf", "/etc/dnsmasq.d/10-local.conf"] {
            assert!(allowed(p));
        }
        for p in [
            "/etc/passwd",
            "/etc/dnsmasq.d/../passwd.conf",
            "/etc/dnsmasq.d/a/b.conf",
            "/etc/dnsmasq.d/.conf",
            "/etc/dnsmasq.d/ABC.conf",
        ] {
            assert!(!allowed(p));
        }
        assert_ne!(version(None), version(Some("")));
    }
    #[test]
    fn journal_round_trip_and_restore() {
        let dir = std::env::temp_dir().join(format!("verso-file-test-{}", std::process::id()));
        fs::create_dir_all(&dir).unwrap();
        let path = dir.join("options.conf");
        let journal = dir.join("journal.json");
        atomic(&path, "original\n", 0o600).unwrap();
        let j = json!({"files":{},"active":true,"deadline":123});
        save_at(&journal, &j).unwrap();
        assert_eq!(load_at(&journal).unwrap(), j);
        atomic(&path, "edited\n", 0o600).unwrap();
        restore_file(&path, &json!({"before":"original\n","mode":384})).unwrap();
        assert_eq!(read(&path).unwrap().as_deref(), Some("original\n"));
        restore_file(&path, &json!({"before":null})).unwrap();
        assert!(!path.exists());
        std::os::unix::fs::symlink("/etc/passwd", &path).unwrap();
        assert!(read(&path).is_err());
        assert!(atomic(&path, "bad", 0o600).is_err());
        fs::remove_dir_all(dir).unwrap();
    }
}
