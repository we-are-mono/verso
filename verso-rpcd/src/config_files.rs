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
fn allowed(path: &str) -> bool {
    if path == "/etc/dnsmasq.conf" {
        return true;
    }
    path.strip_prefix("/etc/dnsmasq.d/")
        .and_then(|s| s.strip_suffix(".conf"))
        .is_some_and(|name| {
            !name.is_empty()
                && name.len() <= 64
                && name.as_bytes()[0].is_ascii_alphanumeric()
                && name
                    .bytes()
                    .all(|b| b.is_ascii_lowercase() || b.is_ascii_digit() || b == b'_' || b == b'-')
        })
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
        return Err(Failure::invalid(
            "Custom option files must be at most 32 KiB.",
        ));
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
fn reload() {
    let _ = Command::new("/etc/init.d/dnsmasq")
        .arg("reload")
        .stdin(Stdio::null())
        .stdout(Stdio::null())
        .stderr(Stdio::null())
        .status();
}
fn recover(j: &mut Value) -> Result<bool, Failure> {
    if j["active"] == true && j["deadline"].as_u64().unwrap_or(0) <= now() {
        restore(j)?;
        j["active"] = json!(false);
        j["uci_confirmed"] = json!(false);
        save_at(Path::new(JOURNAL), j)?;
        return Ok(true);
    }
    Ok(false)
}
pub fn watchdog() {
    std::thread::spawn(|| loop {
        std::thread::sleep(std::time::Duration::from_secs(1));
        let reload_needed = {
            let Ok(_guard) = LOCK.lock() else {
                continue;
            };
            load_at(Path::new(JOURNAL))
                .and_then(|mut j| recover(&mut j))
                .unwrap_or_else(|e| {
                    eprintln!("verso-rpcd: file rollback failed: {}", e.message);
                    false
                })
        };
        if reload_needed {
            reload();
        }
    });
}
pub fn state() -> Result<Value, Failure> {
    let _guard = LOCK.lock().map_err(failure)?;
    let mut j = load_at(Path::new(JOURNAL))?;
    if recover(&mut j)? {
        reload();
    }
    let mut paths = vec![PathBuf::from("/etc/dnsmasq.conf")];
    check_path(Path::new("/etc/dnsmasq.d"))?;
    if let Ok(entries) = fs::read_dir("/etc/dnsmasq.d") {
        paths.extend(
            entries
                .filter_map(Result::ok)
                .map(|e| e.path())
                .filter(|p| p.to_str().is_some_and(allowed)),
        );
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
        let active = match read(&path) {
            Ok(body) => body,
            Err(e) if j["files"].get(key.as_ref()).is_none() => {
                files.push(json!({"path":key,"error":e.message}));
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
            files.push(
                json!({"path":key,"error":"Custom option files exceed the editor size limit."}),
            );
            continue;
        }
        files.push(json!({"path":key,"content":body.as_deref().unwrap_or(""),"version":version(body.as_deref()),"pending":j["files"].get(key.as_ref()).is_some(),"exists":body.is_some()}));
    }
    Ok(
        json!({"files":files,"active":j["active"],"uci":j["uci"],"uci_confirmed":j["uci_confirmed"]}),
    )
}
fn validate_text(body: &str) -> Result<(), Failure> {
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
    if !allowed(path) || body.len() > LIMIT || body.contains('\0') {
        return Err(Failure::invalid("Invalid custom options file."));
    }
    let _guard = LOCK.lock().map_err(failure)?;
    let mut j = load_at(Path::new(JOURNAL))?;
    if recover(&mut j)? {
        reload();
    }
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
    validate_text(body)?;
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
        return Err(Failure::invalid(
            "Staged custom options exceed the editor size limit.",
        ));
    }
    save_at(Path::new(JOURNAL), &j)?;
    Ok(json!({"result":true}))
}
pub fn lifecycle(action: &str, uci: bool, timeout: u64) -> Result<Value, Failure> {
    let _guard = LOCK.lock().map_err(failure)?;
    let mut j = load_at(Path::new(JOURNAL))?;
    if recover(&mut j)? {
        reload();
    }
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
                let mut command = Command::new("/usr/sbin/dnsmasq");
                command.args(["--test", "--conf-file=/etc/dnsmasq.conf"]);
                if Path::new("/etc/dnsmasq.d").is_dir() {
                    command.arg("--conf-dir=/etc/dnsmasq.d");
                }
                let check = command.stdin(Stdio::null()).output().map_err(failure)?;
                if !check.status.success() {
                    return Err(Failure::invalid(format!(
                        "dnsmasq rejected the custom options: {}",
                        String::from_utf8_lossy(&check.stderr)
                    )));
                }
                Ok(())
            })();
            if let Err(e) = result {
                restore(&j)?;
                j["active"] = json!(false);
                save_at(Path::new(JOURNAL), &j)?;
                return Err(e);
            }
            if !uci {
                reload();
            }
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
                reload();
            }
            j["active"] = json!(false);
            save_at(Path::new(JOURNAL), &j)?;
        }
        "discard" => {
            if j["active"] == true {
                return Err(Failure::invalid("An apply is in progress."));
            }
            save_at(Path::new(JOURNAL), &json!({"files":{},"active":false}))?;
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
    let mut result = state()?;
    result["resolver"] = json!(Path::new("/usr/sbin/dnsmasq").is_file());
    result["dnssec"] = json!(dnssec);
    result["doh"] = json!(Path::new("/usr/sbin/https-dns-proxy").is_file());
    result["adblock"] = json!(Path::new("/etc/init.d/adblock").is_file());
    Ok(result)
}
#[cfg(test)]
mod tests {
    use super::*;
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
