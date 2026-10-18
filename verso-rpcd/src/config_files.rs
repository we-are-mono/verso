// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.
//! Bounded, root-owned file stage with a durable rollback journal. Saving never
//! changes active configuration. Recovery runs even when the shell disappears.
//!
//! Which files may be staged is the family table's (`families`): each family is
//! declared by the plugin that owns its daemon (ADR-027), and nothing outside a
//! family is ever read, written or restored. A family's transform, its plugin's
//! root command, shapes what leaves the helper and what a save puts back.
use crate::families::{Family, Table};
use crate::plugin_command::{self, Runner};
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

/// check_live checks the daemon's whole configuration with the written files in
/// place, by the program its family names. A family that names none (OpenVPN:
/// a profile it cannot read shows as a tunnel that does not come up) is not
/// checked. The family's directory exists while it is checked, so a check that
/// names it reads an empty one rather than failing on a missing one.
fn check_live(family: &Family) -> Result<(), Failure> {
    let Some((program, args)) = family.declared.check.split_first() else {
        return Ok(());
    };
    if let (_, Some(dir)) = family.places() {
        check_path(&dir)?;
        fs::create_dir_all(&dir).map_err(failure)?;
    }
    let check = Command::new(program)
        .args(args)
        .stdin(Stdio::null())
        .output()
        .map_err(failure)?;
    if !check.status.success() {
        let said = [check.stderr, check.stdout].concat();
        return Err(Failure::invalid(format!(
            "{} rejected the files: {}",
            family.name(),
            complaint(&String::from_utf8_lossy(&said))
        )));
    }
    Ok(())
}

/// reload has the daemon read its files again, by the program its family
/// names, so what runs is exactly what they say.
fn reload(family: &Family) {
    let Some((program, args)) = family.declared.reload.split_first() else {
        return;
    };
    let _ = Command::new(program)
        .args(args)
        .stdin(Stdio::null())
        .stdout(Stdio::null())
        .stderr(Stdio::null())
        .status();
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

/// touched is every family a journal's files belong to, once each, by name.
fn touched<'t>(table: &'t Table, j: &Value) -> Vec<&'t Family> {
    let mut families: Vec<&Family> = j["files"]
        .as_object()
        .map(|files| files.keys().filter_map(|p| table.of(p)).collect())
        .unwrap_or_default();
    families.sort_by(|a, b| a.name().cmp(b.name()));
    families.dedup_by(|a, b| a.name() == b.name());
    families
}

/// drop_config unstages the files of the families config owns, or every file
/// when no config is named.
fn drop_config(table: &Table, j: &mut Value, config: Option<&str>) {
    if let Some(files) = j["files"].as_object_mut() {
        files.retain(|path, _| {
            config.is_some_and(|c| table.of(path).map(|f| f.declared.config.as_str()) != Some(c))
        });
    }
}

fn reload_all(families: &[&Family]) {
    for family in families {
        reload(family);
    }
}
fn check_path(path: &Path) -> Result<(), Failure> {
    crate::paths::no_links(&crate::paths::System, path).map_err(|refused| match refused {
        crate::paths::Refused::Link => Failure::invalid("Symbolic links are not editable."),
        crate::paths::Refused::Unreadable(why) => failure(why),
    })
}
fn read(path: &Path, too_large: &str) -> Result<Option<String>, Failure> {
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
        return Err(Failure::invalid(too_large));
    }
    String::from_utf8(bytes)
        .map(Some)
        .map_err(|_| Failure::invalid("The file must contain UTF-8 text."))
}
/// NEW_FILE is the version of a file that does not exist: fixed, public, and
/// what the plugin SDK's NEW_FILE_VERSION sends to create one. No present
/// file's version is this long, so a save meant to create a file never
/// replaces one.
const NEW_FILE: &str = "af63bd4c8601b7df";
/// version is what a save is checked against: the file at path changed since
/// its editor read it, or not. It is keyed under the helper's root-only key
/// and names its path, so a profile's version gives nothing of its folded
/// blocks to whoever holds it, nor checks a guess at another path's file; the
/// key is kept, so an open editor outlives a helper restart. Without the key
/// there is no version, and no file is read or saved.
fn version(path: &str, body: Option<&str>) -> Result<String, Failure> {
    version_from(crate::config_watch::key, path, body)
}
/// version_from is version under the key that key gives; its refusal is the
/// operator's to read.
fn version_from<K: AsRef<[u8]>>(
    key: impl FnOnce() -> Result<K, String>,
    path: &str,
    body: Option<&str>,
) -> Result<String, Failure> {
    let Some(body) = body else {
        return Ok(NEW_FILE.to_string());
    };
    let key = key().map_err(|error| {
        eprintln!("verso-rpcd: config files: {error}");
        Failure::invalid("The router has no key to check its files against.")
    })?;
    Ok(version_under(key.as_ref(), path, body))
}
fn version_under(key: &[u8], path: &str, body: &str) -> String {
    let mut message = b"verso config-file version\0".to_vec();
    message.extend_from_slice(&(path.len() as u64).to_be_bytes());
    message.extend_from_slice(path.as_bytes());
    message.extend_from_slice(body.as_bytes());
    crate::config_watch::hex(&crate::config_watch::hmac(key, &message)[..16])
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
fn restore(table: &Table, journal: &Value) -> Result<(), Failure> {
    for (path, file) in journal["files"]
        .as_object()
        .ok_or_else(|| Failure::unknown("Invalid file journal."))?
    {
        if table.of(path).is_none() {
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
/// recover rolls back an apply whose confirmation window has passed and reloads
/// the families it restored. The family table is built only then.
fn recover(j: &mut Value, table: impl FnOnce() -> Table) -> Result<(), Failure> {
    if j["active"] == true && j["deadline"].as_u64().unwrap_or(0) <= now() {
        let table = table();
        restore(&table, j)?;
        j["active"] = json!(false);
        j["uci_confirmed"] = json!(false);
        save_at(Path::new(JOURNAL), j)?;
        reload_all(&touched(&table, j));
    }
    Ok(())
}
pub fn watchdog() {
    std::thread::spawn(|| loop {
        std::thread::sleep(std::time::Duration::from_secs(1));
        let Ok(_guard) = LOCK.lock() else {
            continue;
        };
        if let Err(e) = load_at(Path::new(JOURNAL)).and_then(|mut j| recover(&mut j, Table::installed)) {
            eprintln!("verso-rpcd: file rollback failed: {}", e.message);
        }
    });
}

/// One file as a listing reads it before its family's transform: its path,
/// what it will hold once staged changes apply, and whether it is staged.
struct Draft {
    path: String,
    body: Option<String>,
    pending: bool,
}

/// What a family's transform made of one listed file.
#[derive(Debug, Clone, PartialEq)]
enum Shown {
    Content(String),
    Withheld(String),
}

/// shown runs a family's transform over its listed files (op `list`): what
/// each shows, folded, or a reason to list it by path alone. A family without
/// a transform shows its files as they are; one whose transform fails shows
/// nothing of them.
fn shown(runner: &impl Runner, family: &Family, drafts: &[Draft]) -> Vec<Shown> {
    let Some(command) = &family.transform else {
        return drafts.iter().map(|d| Shown::Content(d.body.clone().unwrap_or_default())).collect();
    };
    let failed = || drafts.iter().map(|_| Shown::Withheld("The file could not be read.".into())).collect();
    let input = json!({
        "op": "list",
        "files": drafts.iter().map(|d| json!({"path": d.path, "content": d.body, "pending": d.pending})).collect::<Vec<_>>(),
    });
    let Ok(answer) = plugin_command::run_command(runner, &family.plugin, command, &Default::default(), input) else {
        return failed();
    };
    let Some(files) = answer["files"].as_array().filter(|f| f.len() == drafts.len()) else {
        eprintln!("verso-rpcd: {}: its transform listed another number of files", family.plugin);
        return failed();
    };
    files
        .iter()
        .zip(drafts)
        .map(|(file, draft)| match (file["path"].as_str(), file["content"].as_str(), file["withheld"].as_str()) {
            (Some(path), Some(content), None) if path == draft.path => Shown::Content(content.to_string()),
            (Some(path), None, Some(why)) if path == draft.path => Shown::Withheld(why.to_string()),
            _ => Shown::Withheld("The file could not be read.".into()),
        })
        .collect()
}

/// restored runs a family's transform over a save (op `stage`): the file as it
/// will be written, with whatever the transform held back put in again, or the
/// transform's refusal. A family without a transform writes the save as it is.
fn restored(
    runner: &impl Runner,
    family: &Family,
    path: &str,
    edited: &str,
    previous: Option<&str>,
) -> Result<String, Failure> {
    let Some(command) = &family.transform else {
        return Ok(edited.to_string());
    };
    let input = json!({"op": "stage", "path": path, "edited": edited, "previous": previous});
    let answer = plugin_command::run_command(runner, &family.plugin, command, &Default::default(), input)?;
    answer["content"]
        .as_str()
        .map(String::from)
        .ok_or_else(|| Failure::unknown(format!("{}: its transform answered no content", family.plugin)))
}

/// state is every family's files as they will read once staged changes are
/// applied, each saying which family it belongs to, and the families: their
/// names, the uci config that owns each and the plugin that declared it.
pub fn state() -> Result<Value, Failure> {
    state_with(&Table::installed(), &plugin_command::System)
}

fn state_with(table: &Table, runner: &impl Runner) -> Result<Value, Failure> {
    let _guard = LOCK.lock().map_err(failure)?;
    let mut j = load_at(Path::new(JOURNAL))?;
    recover(&mut j, || table.clone())?;
    let mut paths = vec![];
    for family in &table.families {
        let (fixed, dir) = family.places();
        paths.extend(fixed);
        if let Some(dir) = dir {
            check_path(&dir)?;
            if let Ok(entries) = fs::read_dir(&dir) {
                paths.extend(
                    entries
                        .filter_map(Result::ok)
                        .map(|e| e.path())
                        .filter(|p| p.to_str().is_some_and(|p| table.of(p).is_some())),
                );
            }
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
        return Err(Failure::invalid("Too many files to list."));
    }
    // Each family's files go to its transform together, in one run.
    let mut files: Vec<Option<Value>> = vec![];
    let mut drafts: Vec<(usize, &Family, Draft)> = vec![];
    let mut total = 0;
    for path in paths {
        let key = path.to_string_lossy().to_string();
        let Some(family) = table.of(&key) else {
            continue;
        };
        let pending = j["files"].get(&key).is_some();
        let active = match read(&path, &family.words().too_large) {
            Ok(body) => body,
            Err(e) if !pending => {
                files.push(Some(json!({"path":key,"family":family.name(),"error":e.message})));
                continue;
            }
            Err(_) => None,
        };
        let body = j["files"][&key]["after"].as_str().map(String::from).or(active);
        total += body.as_ref().map(String::len).unwrap_or(0);
        if total > TOTAL {
            files.push(Some(json!({"path":key,"family":family.name(),"error":family.words().over_limit})));
            continue;
        }
        drafts.push((files.len(), family, Draft { path: key, body, pending }));
        files.push(None);
    }
    for family in &table.families {
        let mine: Vec<&(usize, &Family, Draft)> = drafts.iter().filter(|(_, f, _)| f.name() == family.name()).collect();
        if mine.is_empty() {
            continue;
        }
        let owned: Vec<Draft> = mine
            .iter()
            .map(|(_, _, d)| Draft { path: d.path.clone(), body: d.body.clone(), pending: d.pending })
            .collect();
        for ((slot, family, draft), shown) in mine.into_iter().zip(shown(runner, family, &owned)) {
            files[*slot] = Some(listed(&draft.path, family, draft.body.as_deref(), draft.pending, shown, version));
        }
    }
    let families: Vec<Value> = table.families.iter().map(Family::description).collect();
    Ok(json!({
        "files": files.into_iter().flatten().collect::<Vec<_>>(),
        "families": families,
        "active": j["active"],
        "uci": j["uci"],
        "uci_confirmed": j["uci_confirmed"],
    }))
}

/// listed is how state reports one file: what its transform shows of it, and
/// its version, the whole file's, keyed, which is what a save is checked
/// against. A withheld file is reported by its path alone, with no content and
/// no version, so its name stays taken and no save can be checked against it.
/// A file with no version has the reason in its place, so it fails no other
/// file's listing.
fn listed(
    path: &str,
    family: &Family,
    body: Option<&str>,
    pending: bool,
    shown: Shown,
    version: impl FnOnce(&str, Option<&str>) -> Result<String, Failure>,
) -> Value {
    let content = match shown {
        Shown::Content(content) => content,
        Shown::Withheld(why) => return json!({"path":path,"family":family.name(),"error":why}),
    };
    match version(path, body) {
        Ok(version) => {
            json!({"path":path,"family":family.name(),"content":content,"version":version,"pending":pending,"exists":body.is_some()})
        }
        Err(e) => {
            json!({"path":path,"family":family.name(),"content":content,"error":e.message,"pending":pending,"exists":body.is_some()})
        }
    }
}

/// stage stages one file of a family whose config is one of configs: the
/// configs the plugin asking may write, as the shell reads them from its
/// manifest.
pub fn stage(path: &str, expected: &str, body: &str, configs: &str) -> Result<Value, Failure> {
    stage_with(&Table::installed(), &plugin_command::System, path, expected, body, configs)
}

fn stage_with(
    table: &Table,
    runner: &impl Runner,
    path: &str,
    expected: &str,
    body: &str,
    configs: &str,
) -> Result<Value, Failure> {
    let family = family_for(table, path, configs)?;
    if body.len() > LIMIT || body.contains('\0') {
        return Err(Failure::invalid(family.words().invalid.as_str()));
    }
    let _guard = LOCK.lock().map_err(failure)?;
    let mut j = load_at(Path::new(JOURNAL))?;
    recover(&mut j, || table.clone())?;
    if j["active"] == true {
        return Err(Failure::invalid("Wait for the current apply to finish."));
    }
    let before = read(Path::new(path), &family.words().too_large)?;
    let previous = j["files"][path]["after"].as_str().or(before.as_deref());
    if version(path, previous)? != expected {
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
    let body = restored(runner, family, path, body, previous)?;
    if body.len() > LIMIT || body.contains('\0') {
        return Err(Failure::invalid(family.words().invalid.as_str()));
    }
    let mode = fs::metadata(path)
        .map(|m| m.permissions().mode() & 0o777)
        .unwrap_or(family.mode());
    let original = j["files"][path]
        .get("before")
        .cloned()
        .unwrap_or(json!(before));
    if original.as_str() == Some(body.as_str()) {
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
        return Err(Failure::invalid(family.words().over_limit.as_str()));
    }
    save_at(Path::new(JOURNAL), &j)?;
    Ok(json!({"result":true}))
}
/// family_for is the family that holds path, when its config is one of
/// configs (comma-separated); otherwise the save is refused.
fn family_for<'t>(table: &'t Table, path: &str, configs: &str) -> Result<&'t Family, Failure> {
    let family = table
        .of(path)
        .ok_or_else(|| Failure::invalid("This file is not one the router edits."))?;
    if !configs.split(',').any(|c| c == family.declared.config) {
        return Err(Failure::invalid("This page may not change that file."));
    }
    Ok(family)
}

/// lifecycle carries staged files through an apply. On apply it writes every
/// staged file, checks each touched family's whole configuration with them in
/// place, and has each family read them again; a refusal restores the files
/// before anything reloads. Abort and the watchdog restore the same way.
/// A discard names the config that was discarded, and drops the files of the
/// families that config owns — none, for a config no family belongs to — or
/// names none, to drop every staged file.
pub fn lifecycle(
    action: &str,
    uci: bool,
    timeout: u64,
    config: Option<&str>,
) -> Result<Value, Failure> {
    let table = Table::installed();
    let _guard = LOCK.lock().map_err(failure)?;
    let mut j = load_at(Path::new(JOURNAL))?;
    recover(&mut j, || table.clone())?;
    match action {
        "apply" => {
            if j["active"] == true {
                return Err(Failure::invalid("An apply is already in progress."));
            }
            if j["files"].as_object().is_none_or(|f| f.is_empty()) {
                return Ok(json!({"active":false}));
            }
            for (path, file) in j["files"].as_object().unwrap() {
                let Some(owner) = table.of(path) else {
                    return Err(Failure::invalid("A staged file belongs to no family. Discard it."));
                };
                if read(Path::new(path), &owner.words().too_large)?.as_deref() != file["before"].as_str() {
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
                touched(&table, &j).into_iter().try_for_each(check_live)
            })();
            if let Err(e) = result {
                restore(&table, &j)?;
                j["active"] = json!(false);
                save_at(Path::new(JOURNAL), &j)?;
                return Err(e);
            }
            // Every family the change touched reads its files again, whether
            // or not the uci apply beside it touches that daemon's config.
            reload_all(&touched(&table, &j));
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
                restore(&table, &j)?;
                reload_all(&touched(&table, &j));
            }
            j["active"] = json!(false);
            save_at(Path::new(JOURNAL), &j)?;
        }
        "discard" => {
            if j["active"] == true {
                return Err(Failure::invalid("An apply is in progress."));
            }
            drop_config(&table, &mut j, config);
            save_at(Path::new(JOURNAL), &j)?;
        }
        _ => return Err(Failure::invalid("Invalid file transaction action.")),
    }
    Ok(json!({"active": j["active"]}))
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::families::fixtures;
    use crate::plugin_command::{Ran, Run};
    use std::cell::RefCell;

    /// A transform that answers with a fixed reply and records its requests.
    struct Transform {
        reply: Result<Ran, String>,
        requests: RefCell<Vec<Value>>,
    }

    impl Transform {
        fn answering(code: i32, stdout: Value) -> Transform {
            Transform {
                reply: Ok(Ran { code: Some(code), stdout: stdout.to_string().into_bytes(), stderr: vec![] }),
                requests: RefCell::new(vec![]),
            }
        }
    }

    impl Runner for Transform {
        fn run(&self, run: &Run) -> Result<Ran, String> {
            self.requests.borrow_mut().push(serde_json::from_slice(&run.stdin).unwrap());
            self.reply.clone()
        }
    }

    fn vpn() -> Family {
        let mut f = fixtures::openvpn();
        f.transform = Some(serde_json::from_value(json!(
            {"name": "files", "kind": "transform", "exec": "/usr/bin/verso-plugin-vpn", "argv": ["root"]}
        )).unwrap());
        f
    }

    fn draft(path: &str, body: &str) -> Draft {
        Draft { path: path.into(), body: Some(body.into()), pending: false }
    }

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
    fn discarding_one_config_keeps_the_others_files_staged() {
        let table = fixtures::table();
        let mut j = json!({"files":{
            "/etc/nftables.d/10-a.nft":{"after":"x"},
            "/etc/dnsmasq.d/x.conf":{"after":"y"}
        },"active":false});
        drop_config(&table, &mut j, Some("network"));
        assert_eq!(j["files"].as_object().unwrap().len(), 2, "a config no family belongs to drops nothing");
        drop_config(&table, &mut j, Some("firewall"));
        assert_eq!(j["files"], json!({"/etc/dnsmasq.d/x.conf":{"after":"y"}}));
        drop_config(&table, &mut j, None);
        assert_eq!(j["files"], json!({}));
    }

    /// A file is staged for a plugin only when its family's config is one the
    /// plugin may write; a path in no family is staged for nobody.
    #[test]
    fn a_file_is_staged_only_for_its_familys_writer() {
        let table = fixtures::table();
        for (path, configs) in [
            ("/etc/dnsmasq.d/10-local.conf", "dhcp"),
            ("/etc/dnsmasq.conf", "network,dhcp"),
            ("/etc/nftables.d/10-custom.nft", "firewall"),
            ("/etc/openvpn/proton.ovpn", "openvpn"),
        ] {
            assert!(family_for(&table, path, configs).is_ok(), "{path} for {configs}");
        }
        for (path, configs) in [
            ("/etc/dnsmasq.d/10-local.conf", "firewall"),
            ("/etc/nftables.d/10-custom.nft", "dhcp"),
            ("/etc/firewall.user", "firewall"),
            ("/etc/passwd", "dhcp"),
            ("/etc/openvpn/proton.ovpn", "firewall"),
            ("/etc/dnsmasq.conf", ""),
            ("/etc/dnsmasq.conf", "dhcpx"),
        ] {
            assert!(family_for(&table, path, configs).is_err(), "{path} for {configs}");
        }
    }
    #[test]
    fn a_change_names_every_family_it_touches_once() {
        let table = fixtures::table();
        let j = json!({"files":{
            "/etc/nftables.d/10-a.nft":{},
            "/etc/nftables.d/20-b.nft":{},
            "/etc/dnsmasq.d/x.conf":{}
        }});
        let names: Vec<&str> = touched(&table, &j).into_iter().map(Family::name).collect();
        assert_eq!(names, ["dnsmasq", "fw4"]);
        assert!(touched(&table, &json!({"files":{}})).is_empty());
    }

    /// A journal written before families came from the registries names its
    /// files by path, under the same families: it still lists, applies and
    /// discards under the table the registries build.
    #[test]
    fn a_journal_from_before_the_registries_still_resolves() {
        let table = fixtures::table();
        let j = json!({"files":{
            "/etc/dnsmasq.conf":{"before":"a","after":"b","mode":420},
            "/etc/nftables.d/10-custom.nft":{"before":null,"after":"x","mode":420},
            "/etc/openvpn/proton.ovpn":{"before":"c","after":"d","mode":384}
        },"active":false});
        for path in j["files"].as_object().unwrap().keys() {
            assert!(table.of(path).is_some(), "{path}");
        }
        let names: Vec<&str> = touched(&table, &j).into_iter().map(Family::name).collect();
        assert_eq!(names, ["dnsmasq", "fw4", "openvpn"]);
        let mut discard = j.clone();
        drop_config(&table, &mut discard, Some("openvpn"));
        assert_eq!(discard["files"].as_object().unwrap().len(), 2);
    }

    #[test]
    fn a_family_without_a_transform_shows_its_files_as_they_are() {
        let t = Transform::answering(0, json!({}));
        let shown = shown(&t, &fixtures::fw4(), &[draft("/etc/nftables.d/a.nft", "rule")]);
        assert_eq!(shown, [Shown::Content("rule".into())]);
        assert!(t.requests.borrow().is_empty());
    }

    #[test]
    fn a_transform_folds_and_withholds() {
        let t = Transform::answering(0, json!({"files": [
            {"path": "/etc/openvpn/proton.ovpn", "content": "client\n<key> … 3 lines … </key>"},
            {"path": "/etc/openvpn/proton.conf", "withheld": "No OpenVPN instance reads this file as its profile."}
        ]}));
        let drafts = [draft("/etc/openvpn/proton.ovpn", "client\n<key>\nSECRET\n</key>"), draft("/etc/openvpn/proton.conf", "user\npass")];
        let got = shown(&t, &vpn(), &drafts);
        assert_eq!(got[0], Shown::Content("client\n<key> … 3 lines … </key>".into()));
        assert_eq!(got[1], Shown::Withheld("No OpenVPN instance reads this file as its profile.".into()));
        let request = &t.requests.borrow()[0];
        assert_eq!(request["command"], "files");
        assert_eq!(request["input"]["op"], "list");
        assert_eq!(request["input"]["files"][0]["content"], "client\n<key>\nSECRET\n</key>");

        let listed_withheld = listed("/etc/openvpn/proton.conf", &vpn(), Some("user\npass"), false, got[1].clone(), |_, _| Ok("v".into()));
        assert_eq!(listed_withheld, json!({"path":"/etc/openvpn/proton.conf","family":"openvpn","error":"No OpenVPN instance reads this file as its profile."}));
        let folded = listed("/etc/openvpn/proton.ovpn", &vpn(), Some("client\n<key>\nSECRET\n</key>"), true, got[0].clone(), |_, _| Ok("v".into()));
        assert!(!folded.to_string().contains("SECRET"), "{folded}");
    }

    #[test]
    fn a_failing_transform_lists_its_files_by_path_alone() {
        for t in [
            Transform { reply: Err("timed out".into()), requests: RefCell::new(vec![]) },
            Transform::answering(1, json!({})),
            Transform::answering(0, json!({"files": []})),
            Transform::answering(0, json!({"files": [{"path": "/etc/openvpn/other.ovpn", "content": "x"}]})),
        ] {
            let got = shown(&t, &vpn(), &[draft("/etc/openvpn/proton.ovpn", "client\n<key>\nSECRET\n</key>")]);
            assert!(matches!(&got[0], Shown::Withheld(_)), "{got:?}");
        }
    }

    #[test]
    fn a_transform_restores_a_save_or_refuses_it() {
        let t = Transform::answering(0, json!({"content": "client\n<key>\nSECRET\n</key>\nverb 3"}));
        let saved = restored(&t, &vpn(), "/etc/openvpn/proton.ovpn", "client\n<key> … 3 lines … </key>\nverb 3", Some("client\n<key>\nSECRET\n</key>")).unwrap();
        assert_eq!(saved, "client\n<key>\nSECRET\n</key>\nverb 3");
        let request = &t.requests.borrow()[0];
        assert_eq!(request["input"]["op"], "stage");
        assert_eq!(request["input"]["previous"], "client\n<key>\nSECRET\n</key>");

        let refused = Transform::answering(3, json!({"refusal": "A folded block was moved."}));
        let err = restored(&refused, &vpn(), "/etc/openvpn/proton.ovpn", "x", None).unwrap_err();
        assert_eq!((err.status, err.message.as_str()), (crate::STATUS_INVALID_ARGUMENT, "A folded block was moved."));

        let none = Transform::answering(0, json!({}));
        assert_eq!(restored(&none, &fixtures::fw4(), "/etc/nftables.d/a.nft", "rule", None).unwrap(), "rule");
        assert!(none.requests.borrow().is_empty());
    }

    const KEY: [u8; 32] = [7; 32];
    const PATH: &str = "/etc/openvpn/proton.ovpn";
    /// A file that does not exist has the version the plugin SDK sends to
    /// create one; an empty file has another.
    #[test]
    fn a_new_file_has_the_sdks_version() {
        let sdk = include_str!("../../plugins/verso-plugin-sdk/src/files.rs");
        let new = version(PATH, None).unwrap();
        assert!(sdk.contains(&format!("NEW_FILE_VERSION: &str = \"{new}\"")), "{new}");
        assert_eq!(new, version("/etc/openvpn/other.ovpn", None).unwrap());
        assert_ne!(new, version_under(&KEY, PATH, ""));
    }
    /// The same file at two paths has two versions, so one path's version
    /// checks no guess at another's.
    #[test]
    fn a_version_is_its_paths_alone() {
        let body = "client\n<key>\nSECRET\n</key>\n";
        assert_ne!(
            version_under(&KEY, PATH, body),
            version_under(&KEY, "/etc/openvpn/other.ovpn", body)
        );
        // the path's length is part of the message, so no split of path and
        // body reads as another
        assert_ne!(
            version_under(&KEY, "/etc/openvpn/a", "b\n"),
            version_under(&KEY, "/etc/openvpn/ab", "\n")
        );
    }
    /// A file's version tells a save the file changed and nothing of what it
    /// holds: without the key no guess at a folded block can be checked
    /// against it.
    #[test]
    fn a_files_version_gives_nothing_of_what_it_holds() {
        let profile = |password: &str| {
            format!("client\nremote vpn.test.invalid 1194\n<auth-user-pass>\noperator7\n{password}\n</auth-user-pass>\nverb 3\n")
        };
        let (held, other) = (profile("m4x9p"), profile("m4x9q"));
        let reported = version_under(&KEY, PATH, &held);
        assert_ne!(reported, version_under(&KEY, PATH, &other), "a changed block changes the version");
        assert_eq!(reported, version_under(&KEY, PATH, &held), "the same file keeps its version");
        assert_ne!(reported, version_under(&[8; 32], PATH, &held), "the version is the key's");
        // an unkeyed FNV-1a, which a reader could run over every guess at
        // the password
        let fnv = |body: &str| {
            let mut h = 0xcbf29ce484222325u64;
            for b in body.bytes().chain([1]) {
                h = (h ^ b as u64).wrapping_mul(0x100000001b3);
            }
            format!("{h:016x}")
        };
        assert!(["m4x9p", "m4x9q"].iter().all(|guess| fnv(&profile(guess)) != reported));
        assert_ne!(Some(reported), version(PATH, None).ok(), "a file never reads as new");
    }
    /// Without the kernel's randomness there is no key and so no version: a
    /// present file is listed with the reason in its place, as one that
    /// cannot be read is, so the other files still list and a staged one
    /// still reaches the review and its apply. The reason is the operator's
    /// to read, and a save is refused with it.
    #[test]
    fn a_file_with_no_version_is_listed_with_the_reason() {
        let dir = std::env::temp_dir().join(format!("verso-file-nokey-{}", std::process::id()));
        let starved = || {
            crate::config_watch::key_at(&dir.join("fingerprint.key"), 0, |_: &mut [u8]| {
                Err(std::io::Error::other("no randomness"))
            })
        };
        let unkeyed = |path: &str, body: Option<&str>| version_from(starved, path, body);
        let body = "client\n<key>\nSECRET\n</key>\n";
        let folded = Shown::Content("client\n<key> … 3 lines … </key>\n".into());
        let file = listed(PATH, &vpn(), Some(body), true, folded, unkeyed);
        assert_eq!(file["error"], "The router has no key to check its files against.");
        assert!(file.get("version").is_none(), "{file}");
        assert_eq!((&file["path"], &file["family"], &file["pending"]), (&json!(PATH), &json!("openvpn"), &json!(true)));
        assert!(!file.to_string().contains("SECRET"), "{file}");
        let refused = version_from(starved, PATH, Some(body)).unwrap_err();
        assert_eq!(refused.status, crate::STATUS_INVALID_ARGUMENT, "the operator reads why");
        // a file that does not exist needs no key
        let new = listed(PATH, &vpn(), None, true, Shown::Content(String::new()), unkeyed);
        assert_eq!((&new["version"], &new["exists"]), (&json!(NEW_FILE), &json!(false)));
        assert!(new.get("error").is_none());
        assert!(!dir.exists(), "no key is kept");
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
        assert_eq!(read(&path, "too large").unwrap().as_deref(), Some("original\n"));
        restore_file(&path, &json!({"before":null})).unwrap();
        assert!(!path.exists());
        std::os::unix::fs::symlink("/etc/passwd", &path).unwrap();
        assert!(read(&path, "too large").is_err());
        assert!(atomic(&path, "bad", 0o600).is_err());
        fs::remove_dir_all(dir).unwrap();
    }
}
