// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

//! Configs written straight to /etc/config, for config history (ADR-021).
//!
//! `uci commit` writes a temporary file and renames it over the config
//! (IN_MOVED_TO); an editor or a script writes it in place (IN_CLOSE_WRITE).
//! Either way the watch sees it, whoever wrote it: Verso's own apply, the uci
//! command line over SSH, a script, LuCI or a restored backup. Once a config
//! has had no write for two seconds, its committed sections are read and
//! spooled for the shell, which tells Verso's applies from the rest.
//!
//! No secret leaves the helper (ADR-014): an option named in the shared list
//! or a plugin's manifest is spooled as a fingerprint, a keyed hash that says
//! whether it changed and nothing of what it is. The key never leaves the
//! router's root.
//!
//! The spool holds the latest reading of each config, one file each, written
//! by renaming into place. When the helper starts it spools every config, so
//! an edit made while it was not running is found too.

use serde::de::{self, Deserialize, Deserializer, MapAccess, SeqAccess, Visitor};
use serde_json::{json, Value};
use std::collections::{HashMap, HashSet};
use std::fmt;
use std::fs;
use std::io::{ErrorKind, Read, Write};
use std::os::fd::FromRawFd;
use std::os::raw::c_char;
use std::os::unix::fs::{DirBuilderExt, MetadataExt, OpenOptionsExt, PermissionsExt};
use std::path::{Path, PathBuf};
use std::sync::{Mutex, PoisonError};
use std::time::{Duration, Instant, SystemTime, UNIX_EPOCH};

#[path = "../../plugins/verso-plugin-sdk/src/sha256.rs"]
#[allow(dead_code)]
mod sha256;

use crate::uci_committed;

const WATCHED: &str = "/etc/config";
/// Where the shell takes readings from: a directory its account owns, which
/// the init script makes.
const SPOOL: &str = "/var/run/verso/changes";
/// The helper's key, for fingerprints and file versions: made once, readable
/// by root alone, in the shell's directory.
const KEY: &str = "/etc/verso/fingerprint.key";
const PLUGINS: &str = "/usr/share/verso/plugins";
/// The options OpenWrt's own packages keep a secret in, the shell's list.
const BASE_SECRETS: &str = include_str!("../../internal/secrets/secrets.json");
/// How long a config is left after its last write before it is read: one
/// edit, however many writes it takes.
const SETTLE: Duration = Duration::from_secs(2);
const POLL: Duration = Duration::from_millis(500);

const IN_CLOSE_WRITE: u32 = 0x0000_0008;
const IN_MOVED_TO: u32 = 0x0000_0080;
const IN_NONBLOCK: i32 = 0o4000;
const IN_CLOEXEC: i32 = 0o2_000_000;

unsafe extern "C" {
    fn inotify_init1(flags: i32) -> i32;
    fn inotify_add_watch(fd: i32, path: *const c_char, mask: u32) -> i32;
}

/// How long a mark of the helper's own write waits for the write it names.
const OWN_WITHIN: Duration = Duration::from_secs(30);

/// Configs the helper is about to commit itself, and when it said so.
static OWN: Mutex<Option<HashMap<String, Instant>>> = Mutex::new(None);

/// own_write says the helper is about to commit config itself (the web
/// listeners, a package that just arrived), so its next reading is spooled
/// as the helper's own: Verso's act, not an edit made outside it.
pub fn own_write(config: &str) {
    if let Ok(mut own) = OWN.lock() {
        own.get_or_insert_with(HashMap::new)
            .insert(config.to_owned(), Instant::now());
    }
}

/// owned takes the mark of the helper's own write to config, if one is
/// fresh enough to name the write just seen.
fn owned(config: &str) -> bool {
    OWN.lock()
        .ok()
        .and_then(|mut own| own.as_mut()?.remove(config))
        .is_some_and(|at| at.elapsed() <= OWN_WITHIN)
}

/// watchdog runs the watch for as long as the helper does.
pub fn watchdog() {
    std::thread::spawn(|| {
        if let Err(error) = watch() {
            eprintln!("verso-rpcd: config watch stopped: {error}");
        }
    });
}

fn watch() -> Result<(), String> {
    let spool = Path::new(SPOOL);
    // With no key no secret can be fingerprinted, so nothing is spooled: the
    // config is left to its next write, the reason in the log.
    let spool_config = |name: &str, written: u64, own: bool| match key() {
        Ok(key) => spool_config(spool, name, written, own, key),
        Err(error) => eprintln!("verso-rpcd: config watch: {name} not spooled: {error}"),
    };
    // An inotify descriptor, non-blocking so the loop can settle what it saw.
    let fd = unsafe { inotify_init1(IN_NONBLOCK | IN_CLOEXEC) };
    if fd < 0 {
        return Err(format!("inotify: {}", std::io::Error::last_os_error()));
    }
    let mut events = unsafe { fs::File::from_raw_fd(fd) };
    let watched = std::ffi::CString::new(WATCHED).map_err(|e| e.to_string())?;
    if unsafe { inotify_add_watch(fd, watched.as_ptr(), IN_CLOSE_WRITE | IN_MOVED_TO) } < 0 {
        return Err(format!(
            "watch {WATCHED}: {}",
            std::io::Error::last_os_error()
        ));
    }
    for name in configs_in(Path::new(WATCHED)) {
        spool_config(&name, now_secs(), false);
    }
    let mut pending: HashMap<String, (Instant, u64)> = HashMap::new();
    let mut buf = [0u8; 4096];
    loop {
        loop {
            match events.read(&mut buf) {
                Ok(0) => break,
                Ok(n) => {
                    for name in event_names(&buf[..n]) {
                        if uci_committed::config_name(&name) {
                            pending.insert(name, (Instant::now(), now_secs()));
                        }
                    }
                }
                Err(error) if error.kind() == ErrorKind::WouldBlock => break,
                Err(error) if error.kind() == ErrorKind::Interrupted => {}
                Err(error) => return Err(format!("inotify read: {error}")),
            }
        }
        let settled: Vec<(String, u64)> = pending
            .iter()
            .filter(|(_, (at, _))| at.elapsed() >= SETTLE)
            .map(|(name, (_, written))| (name.clone(), *written))
            .collect();
        for (name, written) in settled {
            pending.remove(&name);
            spool_config(&name, written, owned(&name));
        }
        std::thread::sleep(POLL);
    }
}

/// event_names is the file names the inotify events in buf carry: each event
/// is a wd, a mask, a cookie and a length, then the name, NUL-padded.
fn event_names(buf: &[u8]) -> Vec<String> {
    let mut names = Vec::new();
    let mut at = 0;
    while at + 16 <= buf.len() {
        let len = u32::from_ne_bytes(buf[at + 12..at + 16].try_into().unwrap()) as usize;
        let end = (at + 16 + len).min(buf.len());
        let name = &buf[at + 16..end];
        let name = &name[..name.iter().position(|&b| b == 0).unwrap_or(name.len())];
        if let Ok(name) = std::str::from_utf8(name) {
            if !name.is_empty() {
                names.push(name.to_owned());
            }
        }
        at = end;
    }
    names
}

/// configs_in is every config a directory holds: its files named as uci
/// names a config, leaving uci's own temporary files aside.
fn configs_in(dir: &Path) -> Vec<String> {
    let Ok(entries) = fs::read_dir(dir) else {
        return Vec::new();
    };
    let mut names: Vec<String> = entries
        .filter_map(|e| e.ok())
        .filter(|e| e.file_type().map(|t| t.is_file()).unwrap_or(false))
        .filter_map(|e| e.file_name().into_string().ok())
        .filter(|name| uci_committed::config_name(name))
        .collect();
    names.sort();
    names
}

/// spool_config reads one config as it is committed and spools it, its
/// secrets fingerprinted, and own where the helper wrote it itself. A config
/// that cannot be read is left to its next write; the reason goes to the log.
fn spool_config(spool: &Path, name: &str, written: u64, own: bool, key: &[u8]) {
    let mut sections = match uci_committed::read(name) {
        Ok(sections) => sections,
        Err(failure) => {
            eprintln!(
                "verso-rpcd: config watch: {name} unreadable: {}",
                failure.message
            );
            return;
        }
    };
    fingerprint_secrets(
        name,
        &mut sections,
        &secret_options(Path::new(PLUGINS)),
        key,
    );
    let record = json!({ "time": written, "config": name, "own": own, "sections": sections });
    if let Err(error) = write_spool(spool, name, &record) {
        eprintln!("verso-rpcd: config watch: {name} not spooled: {error}");
    }
}

/// secret_options is every config.option kept secret: the shared list and
/// each installed plugin's manifest's (plugin_secrets).
fn secret_options(plugins: &Path) -> HashSet<String> {
    let mut names: HashSet<String> = serde_json::from_str::<Vec<String>>(BASE_SECRETS)
        .unwrap_or_default()
        .into_iter()
        .collect();
    if let Ok(entries) = fs::read_dir(plugins) {
        for entry in entries.filter_map(|e| e.ok()) {
            let Ok(bytes) = fs::read(entry.path().join("manifest.json")) else {
                continue;
            };
            // As the shell's encoding/json: a byte that is not UTF-8 reads as
            // U+FFFD, and the manifest is read still.
            names.extend(manifest_secrets(&String::from_utf8_lossy(&bytes)).unwrap_or_default());
        }
    }
    names
}

/// manifest_secrets is the secrets a manifest's text names (plugin_secrets),
/// or None for one the shell does not read: not JSON, or spelling a key twice
/// in one object (KeysOnce).
fn manifest_secrets(text: &str) -> Option<Vec<String>> {
    serde_json::from_str::<KeysOnce>(text).ok()?;
    let manifest: Value = serde_json::from_str(text).ok()?;
    Some(plugin_secrets(&manifest))
}

/// KeysOnce reads a JSON value, and fails on one that spells a key twice in
/// one object, as written or in another case (folded). The shell's decoder
/// takes the last of two spellings of one field, and decodes a repeated
/// object into the first, where a Value keeps both spellings, or the last
/// object whole: the shell skips such a manifest, and so does the helper.
struct KeysOnce;

impl<'de> Deserialize<'de> for KeysOnce {
    fn deserialize<D: Deserializer<'de>>(json: D) -> Result<Self, D::Error> {
        json.deserialize_any(KeysOnce)
    }
}

impl<'de> Visitor<'de> for KeysOnce {
    type Value = KeysOnce;

    fn expecting(&self, f: &mut fmt::Formatter) -> fmt::Result {
        f.write_str("a JSON value")
    }
    fn visit_unit<E>(self) -> Result<KeysOnce, E> {
        Ok(KeysOnce)
    }
    fn visit_bool<E>(self, _: bool) -> Result<KeysOnce, E> {
        Ok(KeysOnce)
    }
    fn visit_i64<E>(self, _: i64) -> Result<KeysOnce, E> {
        Ok(KeysOnce)
    }
    fn visit_u64<E>(self, _: u64) -> Result<KeysOnce, E> {
        Ok(KeysOnce)
    }
    fn visit_f64<E>(self, _: f64) -> Result<KeysOnce, E> {
        Ok(KeysOnce)
    }
    fn visit_str<E>(self, _: &str) -> Result<KeysOnce, E> {
        Ok(KeysOnce)
    }
    fn visit_seq<A: SeqAccess<'de>>(self, mut items: A) -> Result<KeysOnce, A::Error> {
        while items.next_element::<KeysOnce>()?.is_some() {}
        Ok(KeysOnce)
    }
    fn visit_map<A: MapAccess<'de>>(self, mut object: A) -> Result<KeysOnce, A::Error> {
        let mut spelled = HashSet::new();
        while let Some(key) = object.next_key::<String>()? {
            if !spelled.insert(folded(&key)) {
                return Err(de::Error::custom(format!("key {key:?} spelled twice")));
            }
            object.next_value::<KeysOnce>()?;
        }
        Ok(KeysOnce)
    }
}

/// plugin_secrets is the secrets a manifest names in a config its acl writes
/// with scope uci, as the shell's pluginSecrets takes them: a plugin says
/// which options of the configs it writes hold a secret, never of another.
/// Its keys are matched as the shell matches them (fields).
fn plugin_secrets(manifest: &Value) -> Vec<String> {
    let own: HashSet<&str> = fields(manifest, "acl")
        .flat_map(|acl| fields(acl, "write"))
        .filter_map(Value::as_array)
        .flatten()
        .filter(|scope| fields(scope, "scope").any(|name| name.as_str() == Some("uci")))
        .flat_map(|scope| fields(scope, "object"))
        .filter_map(Value::as_str)
        .collect();
    fields(manifest, "secrets")
        .filter_map(Value::as_array)
        .flatten()
        .filter_map(Value::as_str)
        .filter(|name| matches!(name.split_once('.'), Some((config, _)) if own.contains(config)))
        .map(str::to_owned)
        .collect()
}

/// fields is the value of each key of object that names field, a lower-case
/// name, as the shell's encoding/json matches a key to a field (folded). A
/// manifest read spells each field once (KeysOnce).
fn fields<'a>(object: &'a Value, field: &'a str) -> impl Iterator<Item = &'a Value> {
    object
        .as_object()
        .into_iter()
        .flatten()
        .filter(move |(key, _)| folded(key) == field)
        .map(|(_, value)| value)
}

/// folded is the field a key names, as the shell's encoding/json matches a
/// key to a field, by Unicode simple case folding. Every field's name is
/// ASCII, so a key names one in any case, ſ for s and the Kelvin sign for k;
/// any other character names itself. The shell folds a key alike.
fn folded(key: &str) -> String {
    key.chars()
        .map(|c| match c {
            'ſ' => 's',
            '\u{212a}' => 'k',
            c => c.to_ascii_lowercase(),
        })
        .collect()
}

/// fingerprint_secrets replaces the value of every secret option in sections
/// with {"secret": fingerprint}: the same value always gives the same
/// fingerprint, a changed one another, and none gives the value back.
fn fingerprint_secrets(config: &str, sections: &mut Value, secrets: &HashSet<String>, key: &[u8]) {
    let Some(sections) = sections.as_object_mut() else {
        return;
    };
    for (section, options) in sections.iter_mut() {
        let Some(options) = options.as_object_mut() else {
            continue;
        };
        for (option, value) in options.iter_mut() {
            if option.starts_with('.') || !secrets.contains(&format!("{config}.{option}")) {
                continue;
            }
            let text = match value {
                Value::String(s) => s.clone(),
                Value::Array(list) => list
                    .iter()
                    .filter_map(Value::as_str)
                    .collect::<Vec<_>>()
                    .join("\n"),
                _ => continue,
            };
            let message = format!("{config}.{section}.{option}\0{text}");
            *value = json!({ "secret": hex(&hmac(key, message.as_bytes())[..16]) });
        }
    }
}

/// hmac is HMAC-SHA-256 (RFC 2104).
pub(crate) fn hmac(key: &[u8], message: &[u8]) -> [u8; 32] {
    let mut block = [0u8; 64];
    if key.len() > 64 {
        block[..32].copy_from_slice(&sha256::sha256(key));
    } else {
        block[..key.len()].copy_from_slice(key);
    }
    let inner: Vec<u8> = block
        .iter()
        .map(|b| b ^ 0x36)
        .chain(message.iter().copied())
        .collect();
    let inner = sha256::sha256(&inner);
    let outer: Vec<u8> = block
        .iter()
        .map(|b| b ^ 0x5c)
        .chain(inner.iter().copied())
        .collect();
    sha256::sha256(&outer)
}

pub(crate) fn hex(bytes: &[u8]) -> String {
    bytes.iter().map(|b| format!("{b:02x}")).collect()
}

/// key is the helper's key, made the first time from the kernel's randomness
/// and kept readable by root alone: it fingerprints secrets here and versions
/// the hand-edited files (config_files). It is read once, so both use the
/// same one. Without a place to keep it the key lives as long as the helper,
/// and a restart reads every secret and every file as changed once. Without
/// the kernel's randomness there is no key, and each use tries again.
pub(crate) fn key() -> Result<&'static [u8], String> {
    static HELD: Mutex<Option<&'static [u8]>> = Mutex::new(None);
    held(&HELD, || key_at(Path::new(KEY), crate::VERSO_GID, urandom))
}

/// held is the key cell holds, made by make the first time it succeeds.
fn held(
    cell: &Mutex<Option<&'static [u8]>>,
    make: impl FnOnce() -> Result<Vec<u8>, String>,
) -> Result<&'static [u8], String> {
    let mut held = cell.lock().unwrap_or_else(PoisonError::into_inner);
    if let Some(key) = *held {
        return Ok(key);
    }
    let key: &'static [u8] = Box::leak(make()?.into_boxed_slice());
    *held = Some(key);
    Ok(key)
}

fn urandom(key: &mut [u8]) -> std::io::Result<()> {
    fs::File::open("/dev/urandom")?.read_exact(key)
}

/// key_at reads or makes the key at path, from random. Its directory is the
/// shell's too, where the shell reads its certificate (ADR-017 §3), so it is
/// made readable by group, the shell's; the key itself is root's alone by its
/// own mode.
pub(crate) fn key_at(
    path: &Path,
    group: u32,
    random: impl FnOnce(&mut [u8]) -> std::io::Result<()>,
) -> Result<Vec<u8>, String> {
    if let Ok(key) = fs::read(path) {
        if key.len() == 32 {
            return Ok(key);
        }
    }
    let mut key = vec![0u8; 32];
    random(&mut key).map_err(|error| format!("no randomness for the key: {error}"))?;
    let kept = path
        .parent()
        .map(|dir| {
            fs::DirBuilder::new()
                .recursive(true)
                .mode(0o750)
                .create(dir)
                .and_then(|_| std::os::unix::fs::chown(dir, None, Some(group)))
                .and_then(|_| fs::set_permissions(dir, fs::Permissions::from_mode(0o750)))
        })
        .transpose()
        .and_then(|_| {
            fs::OpenOptions::new()
                .write(true)
                .create_new(true)
                .mode(0o600)
                .open(path)
                .and_then(|mut f| f.write_all(&key))
        });
    if let Err(error) = kept {
        eprintln!(
            "verso-rpcd: config watch: key not kept at {}: {error}",
            path.display()
        );
    }
    Ok(key)
}

/// write_spool puts one config's reading in the spool, in place of any
/// reading of it not yet taken. The spool must be a directory the shell's
/// account owns, not a link to one: the parent is writable by its group.
fn write_spool(spool: &Path, config: &str, record: &Value) -> Result<(), String> {
    let meta = fs::symlink_metadata(spool).map_err(|e| format!("{}: {e}", spool.display()))?;
    if !meta.is_dir() || meta.uid() != crate::VERSO_UID {
        return Err(format!("{} is not the shell's directory", spool.display()));
    }
    write_reading(spool, config, record)
}

fn write_reading(spool: &Path, config: &str, record: &Value) -> Result<(), String> {
    let nanos = SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .map(|d| d.as_nanos())
        .unwrap_or(0);
    let name = format!("{nanos:020}-{config}.json");
    let temporary = spool.join(format!(".{name}"));
    let mut file = fs::OpenOptions::new()
        .write(true)
        .create_new(true)
        .mode(0o644)
        .open(&temporary)
        .map_err(|e| format!("{}: {e}", temporary.display()))?;
    file.write_all(record.to_string().as_bytes())
        .and_then(|_| file.sync_all())
        .map_err(|e| e.to_string())?;
    fs::rename(&temporary, spool.join(&name)).map_err(|e| e.to_string())?;
    for older in readings_of(spool, config) {
        if older
            .file_name()
            .is_some_and(|n| n.to_string_lossy() != name)
        {
            let _ = fs::remove_file(older);
        }
    }
    Ok(())
}

/// readings_of is every reading of config waiting in the spool.
fn readings_of(spool: &Path, config: &str) -> Vec<PathBuf> {
    let suffix = format!("-{config}.json");
    fs::read_dir(spool)
        .map(|entries| {
            entries
                .filter_map(|e| e.ok())
                .map(|e| e.path())
                .filter(|p| {
                    p.file_name()
                        .and_then(|n| n.to_str())
                        .is_some_and(|n| !n.starts_with('.') && n.ends_with(&suffix))
                })
                .collect()
        })
        .unwrap_or_default()
}

fn now_secs() -> u64 {
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .map(|d| d.as_secs())
        .unwrap_or(0)
}

#[cfg(test)]
mod tests {
    use super::*;

    fn event(name: &str, padded: usize) -> Vec<u8> {
        let mut out = Vec::new();
        out.extend_from_slice(&1i32.to_ne_bytes());
        out.extend_from_slice(&IN_MOVED_TO.to_ne_bytes());
        out.extend_from_slice(&0u32.to_ne_bytes());
        out.extend_from_slice(&(padded as u32).to_ne_bytes());
        let mut bytes = name.as_bytes().to_vec();
        bytes.resize(padded, 0);
        out.extend_from_slice(&bytes);
        out
    }

    #[test]
    fn events_name_the_files_written() {
        let mut buf = event("firewall", 16);
        buf.extend(event(".network.uci-a1b2c3", 32));
        assert_eq!(event_names(&buf), ["firewall", ".network.uci-a1b2c3"]);
        // A temporary file is no config.
        assert!(!uci_committed::config_name(".network.uci-a1b2c3"));
    }

    #[test]
    fn hmac_is_rfc_4231() {
        assert_eq!(
            hex(&hmac(b"Jefe", b"what do ya want for nothing?")),
            "5bdcc146bf60754e6a042426089575c75a003f089d2739839dec58b964ec3843"
        );
    }

    #[test]
    fn a_secret_is_spooled_as_its_fingerprint_alone() {
        let secrets: HashSet<String> =
            ["network.password".to_owned(), "network.key".to_owned()].into();
        let read = |password: &str| {
            let mut sections = json!({
                "wan": { ".type": "interface", "proto": "pppoe", "username": "t2-41877", "password": password },
                "vpn": { ".type": "interface", "key": ["a", "b"] }
            });
            fingerprint_secrets("network", &mut sections, &secrets, b"k");
            sections
        };
        let first = read("hunter22");
        assert_eq!(first["wan"]["username"], "t2-41877");
        assert!(!first.to_string().contains("hunter22"));
        let fingerprint = first["wan"]["password"]["secret"].as_str().unwrap();
        assert_eq!(fingerprint.len(), 32);
        assert!(first["vpn"]["key"]["secret"].is_string());
        assert_eq!(
            read("hunter22")["wan"]["password"],
            first["wan"]["password"]
        );
        assert_ne!(
            read("hunter23")["wan"]["password"],
            first["wan"]["password"]
        );
    }

    // Wi-Fi's RADIUS, 802.11r and WPS secrets and the alert addresses, under
    // each name uci knows them by, leave the helper as fingerprints alone.
    #[test]
    fn the_lists_secrets_are_spooled_as_fingerprints() {
        let secrets = secret_options(Path::new("/nonexistent"));
        let mut wireless = json!({ "corp": {
            ".type": "wifi-iface", "ssid": "Corp",
            "dae_secret": "PLAIN", "radius_das_secret": "PLAIN",
            "auth_server_shared_secret": "PLAIN", "acct_server_shared_secret": "PLAIN",
            "r0kh": ["02:00:00:00:00:01,*,PLAIN"], "r1kh": ["02:00:00:00:00:01,02:00:00:00:00:01,PLAIN"],
            "ap_pin": "PLAIN", "wps_pin": "PLAIN", "multi_ap_backhaul_key": "PLAIN",
            "priv_key2_pwd": "PLAIN", "private_key2_passwd": "PLAIN",
            "hostapd_bss_options": ["wpa_passphrase=PLAIN", "sae_password=PLAIN"]
        }});
        let mut verso = json!({ "alerts": {
            ".type": "alerts", "url": "PLAIN", "heartbeat": "PLAIN", "kind": ["boot"]
        }});
        fingerprint_secrets("wireless", &mut wireless, &secrets, b"k");
        fingerprint_secrets("verso", &mut verso, &secrets, b"k");
        for (config, sections) in [("wireless", &wireless), ("verso", &verso)] {
            assert!(!sections.to_string().contains("PLAIN"), "{config}: {sections}");
        }
        assert_eq!(wireless["corp"]["ssid"], "Corp");
        assert_eq!(verso["alerts"]["kind"], json!(["boot"]));
    }

    #[test]
    fn a_plugins_secrets_join_the_list() {
        let dir = std::env::temp_dir().join(format!("verso-secrets-{}", std::process::id()));
        fs::create_dir_all(dir.join("vpn")).unwrap();
        fs::write(
            dir.join("vpn/manifest.json"),
            r#"{"id":"vpn","acl":{"write":[{"scope":"uci","object":"vpn","function":"write"}]},"secrets":["vpn.auth_pass"]}"#,
        )
        .unwrap();
        let names = secret_options(&dir);
        assert!(names.contains("vpn.auth_pass") && names.contains("wireless.key"));
        fs::remove_dir_all(dir).unwrap();
    }

    // As the shell's pluginSecrets: a secret a manifest names in a config its
    // acl does not write with scope uci, one it only reads included, is not
    // taken for one, so the helper fingerprints exactly the options the shell
    // scrubs.
    #[test]
    fn a_plugin_names_secrets_only_in_the_configs_it_writes() {
        let dir = std::env::temp_dir().join(format!("verso-own-secrets-{}", std::process::id()));
        fs::create_dir_all(dir.join("demo")).unwrap();
        fs::write(
            dir.join("demo/manifest.json"),
            r#"{"id":"demo","acl":{
                "read":[{"scope":"uci","object":"vpn","function":"read"},{"scope":"ubus","object":"network","function":"read"}],
                "write":[{"scope":"uci","object":"system","function":"write"},{"scope":"ubus","object":"dhcp","function":"write"}]},
              "secrets":["system.notes","vpn.auth_pass","network.ipaddr","dhcp.key_file"]}"#,
        )
        .unwrap();
        let names = secret_options(&dir);
        fs::remove_dir_all(dir).unwrap();
        assert!(
            names.contains("system.notes"),
            "system.notes, in a config the plugin writes, is no secret"
        );
        for other in ["vpn.auth_pass", "network.ipaddr", "dhcp.key_file"] {
            assert!(
                !names.contains(other),
                "{other}, in a config the plugin does not write, is a secret"
            );
        }
        assert!(
            names.contains("network.password"),
            "the shared list's secrets are gone"
        );
    }

    // The shell's test reads the same cases: what one takes for a plugin's
    // secrets, the other does, and a manifest one does not read, neither does.
    // A case's text is its manifest as written, where the file cannot hold it
    // as a value: nested deep, escaping half a surrogate pair, or a number
    // past a float64's range.
    #[test]
    fn a_plugins_secrets_are_read_as_the_shell_reads_them() {
        let cases: Vec<Value> = serde_json::from_str(include_str!(
            "../../internal/server/testdata/plugin-secrets.json"
        ))
        .unwrap();
        for case in cases {
            let text = match case["text"].as_str() {
                Some(text) => text.to_owned(),
                None => case["manifest"].to_string(),
            };
            let got = manifest_secrets(&text).map(|mut got| {
                got.sort();
                got
            });
            let want: Option<Vec<String>> = if case["skipped"] == true {
                None
            } else {
                Some(serde_json::from_value(case["secrets"].clone()).unwrap())
            };
            assert_eq!(got, want, "{}", case["case"]);
        }
    }

    // A key spelled twice in one object, as written or in another case, is
    // one field the shell's decoder takes the last of, or merges, so the
    // shell skips the manifest and the helper takes none of its secrets.
    #[test]
    fn a_manifest_spelling_a_key_twice_is_not_read() {
        let head =
            r#"{"id":"z","acl":{"write":[{"scope":"uci","object":"vpn","function":"write"}]},"#;
        for twice in [
            r#""secrets":["vpn.a"],"secrets":["vpn.b"]}"#,
            r#""acl":{"read":[]},"secrets":["vpn.a"]}"#,
            concat!(r#""secrets":["vpn.a"],"\"#, r#"u0053ECRETS":["vpn.b"]}"#),
            r#""secrets":["vpn.a"],"nav":[{"label":"L","LABEL":"M"}]}"#,
            "\"secrets\":[\"vpn.a\"],\"soc\u{212a}et\":1,\"socket\":2}",
        ] {
            assert_eq!(manifest_secrets(&format!("{head}{twice}")), None, "{twice}");
        }
        assert_eq!(
            manifest_secrets(&format!(r#"{head}"secrets":["vpn.a"],"wrıte":1}}"#)),
            Some(vec!["vpn.a".to_owned()]),
            "a dotless i is no i"
        );
        assert_eq!(manifest_secrets("{ not json"), None);
    }

    // The shell reads a manifest that is not all UTF-8 with each byte that is
    // not as U+FFFD, and so does the helper.
    #[test]
    fn a_manifest_not_all_utf8_is_still_read() {
        let dir = std::env::temp_dir().join(format!("verso-lossy-secrets-{}", std::process::id()));
        fs::create_dir_all(dir.join("demo")).unwrap();
        let mut manifest = br#"{"id":"demo","name":"Demo "#.to_vec();
        manifest.push(0xff);
        manifest.extend_from_slice(
            br#"","acl":{"write":[{"scope":"uci","object":"vpn","function":"write"}]},"secrets":["vpn.auth_pass"]}"#,
        );
        fs::write(dir.join("demo/manifest.json"), manifest).unwrap();
        let names = secret_options(&dir);
        fs::remove_dir_all(dir).unwrap();
        assert!(names.contains("vpn.auth_pass"));
    }

    #[test]
    fn the_spool_keeps_one_reading_of_a_config() {
        let dir = std::env::temp_dir().join(format!("verso-spool-{}", std::process::id()));
        fs::create_dir_all(&dir).unwrap();
        write_reading(&dir, "network", &json!({"n": 1})).unwrap();
        write_reading(&dir, "firewall", &json!({"f": 1})).unwrap();
        write_reading(&dir, "network", &json!({"n": 2})).unwrap();
        let network = readings_of(&dir, "network");
        assert_eq!(network.len(), 1);
        assert_eq!(fs::read_to_string(&network[0]).unwrap(), r#"{"n":2}"#);
        assert_eq!(readings_of(&dir, "firewall").len(), 1);
        fs::remove_dir_all(dir).unwrap();
    }

    #[test]
    fn a_spool_the_shell_does_not_own_is_not_written() {
        let dir = std::env::temp_dir().join(format!("verso-foreign-{}", std::process::id()));
        fs::create_dir_all(&dir).unwrap();
        let link = std::env::temp_dir().join(format!("verso-link-{}", std::process::id()));
        let _ = fs::remove_file(&link);
        std::os::unix::fs::symlink(&dir, &link).unwrap();
        assert!(write_spool(&link, "network", &json!({})).is_err());
        assert!(readings_of(&dir, "network").is_empty());
        fs::remove_file(link).unwrap();
        fs::remove_dir_all(dir).unwrap();
    }

    #[test]
    fn the_key_is_root_s_alone_in_a_directory_the_shell_can_read() {
        let root = std::env::temp_dir().join(format!("verso-key-{}", std::process::id()));
        fs::create_dir_all(&root).unwrap();
        let path = root.join("etc/verso/fingerprint.key");
        // A group the test may give a directory to: its own.
        let group = fs::metadata(&root).unwrap().gid();
        let made = key_at(&path, group, urandom).unwrap();
        let dir = fs::metadata(path.parent().unwrap()).unwrap();
        assert_eq!(dir.mode() & 0o777, 0o750, "the shell reads its certificate here");
        assert_eq!(dir.gid(), group);
        assert_eq!(fs::metadata(&path).unwrap().mode() & 0o777, 0o600);
        assert_eq!(key_at(&path, group, urandom).unwrap(), made, "a key made once is kept");
        fs::remove_dir_all(root).unwrap();
    }

    /// Without the kernel's randomness there is no key: none is made or
    /// kept, nothing is keyed under a stand-in, and the next use tries again.
    #[test]
    fn without_randomness_nothing_is_keyed() {
        let root = std::env::temp_dir().join(format!("verso-nokey-{}", std::process::id()));
        let path = root.join("etc/verso/fingerprint.key");
        let starved = |_: &mut [u8]| Err(std::io::Error::other("no randomness"));
        assert!(key_at(&path, 0, starved).is_err());
        assert!(!path.exists(), "no key is kept");
        let cell = Mutex::new(None);
        assert!(held(&cell, || key_at(&path, 0, starved)).is_err());
        assert!(cell.lock().unwrap().is_none(), "a failure is not held");
        assert_eq!(held(&cell, || Ok(vec![5; 32])).unwrap(), [5; 32]);
        assert_eq!(held(&cell, || Err("unused".into())).unwrap(), [5; 32], "a key is made once");
        let _ = fs::remove_dir_all(root);
    }

    #[test]
    fn the_helpers_own_write_is_marked_once() {
        own_write("verso-test-own");
        assert!(owned("verso-test-own"));
        assert!(!owned("verso-test-own"), "a mark names one write");
        assert!(!owned("verso-test-other"));
    }

    #[test]
    fn configs_are_the_files_uci_names() {
        let dir = std::env::temp_dir().join(format!("verso-configs-{}", std::process::id()));
        fs::create_dir_all(dir.join("subdir")).unwrap();
        for name in ["network", "firewall", ".network.uci-x", "bad name"] {
            fs::write(dir.join(name), "").unwrap();
        }
        assert_eq!(configs_in(&dir), ["firewall", "network"]);
        fs::remove_dir_all(dir).unwrap();
    }
}
