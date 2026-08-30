// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

//! Persistent privileged companion for Verso.
//!
//! The Go shell is deliberately unprivileged. This daemon owns the few root
//! operations it needs and listens on a group-protected Unix socket. Every
//! request still carries the operator's rpcd session and is independently
//! checked through native ubus `session.access` before any action runs.

mod packages;
mod ubus;

use serde_json::{json, Value};
use std::ffi::c_void;
use std::fs;
use std::io::{BufRead, BufReader, Read, Write};
use std::mem;
use std::os::fd::AsRawFd;
use std::os::unix::fs::MetadataExt;
use std::os::unix::fs::PermissionsExt;
use std::os::unix::net::{UnixListener, UnixStream};
use std::path::{Path, PathBuf};
use std::process::{Command, Stdio};
use std::sync::atomic::{AtomicUsize, Ordering};
use std::sync::{Arc, Mutex};
use std::time::Duration;

const DEFAULT_SOCKET: &str = "/var/run/verso/verso-rpcd.sock";
const MAX_REQUEST: u64 = 128 * 1024;
const VERSO_UID: u32 = 6000;
const SOL_SOCKET: i32 = 1;
const SO_PEERCRED: i32 = 17;

const STATUS_OK: i32 = 0;
const STATUS_INVALID_ARGUMENT: i32 = 2;
const STATUS_METHOD_NOT_FOUND: i32 = 3;
const STATUS_PERMISSION_DENIED: i32 = 6;
const STATUS_UNKNOWN_ERROR: i32 = 9;

struct State {
    packages: Mutex<()>,
    maintenance: Mutex<()>,
}

#[repr(C)]
struct UCred {
    pid: i32,
    uid: u32,
    gid: u32,
}

unsafe extern "C" {
    fn getsockopt(
        socket: i32,
        level: i32,
        option: i32,
        value: *mut c_void,
        length: *mut u32,
    ) -> i32;
}

struct Failure {
    status: i32,
    message: String,
}

impl Failure {
    fn invalid(message: impl Into<String>) -> Self {
        Self {
            status: STATUS_INVALID_ARGUMENT,
            message: message.into(),
        }
    }

    fn unknown(message: impl Into<String>) -> Self {
        Self {
            status: STATUS_UNKNOWN_ERROR,
            message: message.into(),
        }
    }
}

fn main() {
    let socket = socket_argument().unwrap_or_else(|message| {
        eprintln!("verso-rpcd: {message}");
        std::process::exit(2);
    });
    if let Err(error) = serve(&socket) {
        eprintln!("verso-rpcd: {error}");
        std::process::exit(1);
    }
}

fn socket_argument() -> Result<PathBuf, String> {
    let mut args = std::env::args_os().skip(1);
    let Some(first) = args.next() else {
        return Ok(DEFAULT_SOCKET.into());
    };
    if first != "--socket" {
        return Err("usage: verso-rpcd [--socket PATH]".into());
    }
    let path = args
        .next()
        .ok_or_else(|| "--socket requires a path".to_string())?;
    if args.next().is_some() {
        return Err("usage: verso-rpcd [--socket PATH]".into());
    }
    Ok(path.into())
}

fn serve(socket: &Path) -> Result<(), String> {
    if let Some(parent) = socket.parent() {
        fs::create_dir_all(parent)
            .map_err(|error| format!("create {}: {error}", parent.display()))?;
    }
    match fs::remove_file(socket) {
        Ok(()) => {}
        Err(error) if error.kind() == std::io::ErrorKind::NotFound => {}
        Err(error) => return Err(format!("remove stale {}: {error}", socket.display())),
    }
    let listener = UnixListener::bind(socket)
        .map_err(|error| format!("bind {}: {error}", socket.display()))?;
    fs::set_permissions(socket, fs::Permissions::from_mode(0o660))
        .map_err(|error| format!("chmod {}: {error}", socket.display()))?;
    eprintln!("verso-rpcd: listening on {}", socket.display());

    let state = Arc::new(State {
        packages: Mutex::new(()),
        maintenance: Mutex::new(()),
    });
    // Bound concurrent requests: each connection pins a thread for up to the read
    // timeout, so an unbounded thread-per-connection would let even an authorized
    // but buggy shell exhaust the root helper (ADR-007 wants a compromised shell
    // bounded). Over the cap we shed the connection — dropping the stream closes
    // it — rather than queue work that also holds a thread.
    const MAX_INFLIGHT: usize = 16;
    let inflight = Arc::new(AtomicUsize::new(0));
    for connection in listener.incoming() {
        match connection {
            Ok(stream) => {
                if inflight.fetch_add(1, Ordering::AcqRel) >= MAX_INFLIGHT {
                    inflight.fetch_sub(1, Ordering::AcqRel);
                    eprintln!("verso-rpcd: at capacity ({MAX_INFLIGHT}), shedding connection");
                    continue;
                }
                let state = Arc::clone(&state);
                let inflight = Arc::clone(&inflight);
                std::thread::spawn(move || {
                    if let Err(error) = handle(stream, &state) {
                        eprintln!("verso-rpcd: request: {error}");
                    }
                    inflight.fetch_sub(1, Ordering::AcqRel);
                });
            }
            Err(error) => eprintln!("verso-rpcd: accept: {error}"),
        }
    }
    Ok(())
}

fn handle(mut stream: UnixStream, state: &State) -> Result<(), String> {
    let uid = peer_uid(&stream)?;
    if uid != 0 && uid != VERSO_UID {
        return Err(format!("refused peer uid {uid}"));
    }
    let timeout = Some(Duration::from_secs(95));
    stream
        .set_read_timeout(timeout)
        .map_err(|error| error.to_string())?;
    stream
        .set_write_timeout(timeout)
        .map_err(|error| error.to_string())?;

    let reader_stream = stream
        .try_clone()
        .map_err(|error| format!("clone socket: {error}"))?;
    let mut line = String::new();
    BufReader::new(reader_stream)
        .take(MAX_REQUEST)
        .read_line(&mut line)
        .map_err(|error| format!("read request: {error}"))?;
    let response = match serde_json::from_str::<Value>(&line) {
        Ok(request) => response(dispatch(&request, state)),
        Err(error) => response(Err(Failure::invalid(format!(
            "invalid request JSON: {error}"
        )))),
    };
    serde_json::to_writer(&mut stream, &response)
        .map_err(|error| format!("encode response: {error}"))?;
    stream
        .write_all(b"\n")
        .map_err(|error| format!("write response: {error}"))?;
    Ok(())
}

fn peer_uid(stream: &UnixStream) -> Result<u32, String> {
    let mut credentials = UCred {
        pid: 0,
        uid: 0,
        gid: 0,
    };
    let mut length = mem::size_of::<UCred>() as u32;
    // SAFETY: `credentials` and `length` point to initialized, writable values
    // of the exact Linux SO_PEERCRED ABI sizes for the duration of the call.
    let result = unsafe {
        getsockopt(
            stream.as_raw_fd(),
            SOL_SOCKET,
            SO_PEERCRED,
            (&mut credentials as *mut UCred).cast(),
            &mut length,
        )
    };
    if result != 0 {
        return Err(format!(
            "read peer credentials: {}",
            std::io::Error::last_os_error()
        ));
    }
    if length < mem::size_of::<UCred>() as u32 {
        return Err("short SO_PEERCRED result".into());
    }
    Ok(credentials.uid)
}

fn response(result: Result<Value, Failure>) -> Value {
    match result {
        Ok(value) => json!({"status": STATUS_OK, "result": value}),
        Err(failure) => json!({"status": failure.status, "error": failure.message}),
    }
}

fn dispatch(request: &Value, state: &State) -> Result<Value, Failure> {
    let method = field(request, "method")?;
    let sid = field(request, "sid")?;
    match ubus::access(sid, method) {
        Ok(true) => {}
        Ok(false) => {
            return Err(Failure {
                status: STATUS_PERMISSION_DENIED,
                message: "permission denied".into(),
            });
        }
        Err(error) => {
            eprintln!("verso-rpcd: authorization failed for {method}: {error}");
            return Err(Failure {
                status: STATUS_PERMISSION_DENIED,
                message: "permission denied".into(),
            });
        }
    }

    match method {
        "setPassword" => {
            let username = argument(request, "username")?;
            let password = argument(request, "password")?;
            set_password(username, password)?;
            Ok(json!({"result": true}))
        }
        "setSystemTime" => {
            let datetime = argument(request, "datetime")?;
            let timezone = argument(request, "timezone")?;
            set_system_time(datetime, timezone)?;
            Ok(json!({"result": true}))
        }
        "pkgStatus" => Ok(json!({"checked_at": packages::checked_at()})),
        "pkgUpdate" => {
            let _guard = package_guard(state)?;
            packages::update().map_err(Failure::unknown)?;
            Ok(json!({"result": true, "checked_at": packages::checked_at()}))
        }
        "pkgInstalled" => {
            let _guard = package_guard(state)?;
            let found = packages::installed().map_err(Failure::unknown)?;
            Ok(json!({"packages": found}))
        }
        "pkgSearch" => {
            let query = argument(request, "query")?;
            if !packages::valid_query(query) {
                return Err(Failure::invalid("query contains unsupported characters"));
            }
            let _guard = package_guard(state)?;
            let (found, total) = packages::search(query).map_err(Failure::unknown)?;
            Ok(json!({"packages": found, "total": total}))
        }
        "pkgInstall" | "pkgRemove" => {
            let name = argument(request, "package")?;
            if !packages::valid_name(name) {
                return Err(Failure::invalid(
                    "package name contains unsupported characters",
                ));
            }
            let _guard = package_guard(state)?;
            if method == "pkgRemove" && packages::protected(name) {
                return Err(Failure::invalid(format!(
                    "{name} is part of the device's base and stays"
                )));
            }
            if method == "pkgRemove" {
                let required_by = packages::required_by(name).map_err(Failure::unknown)?;
                if !required_by.is_empty() {
                    return Err(Failure::invalid(format!(
                        "{name} is required by {}",
                        required_by.join(", ")
                    )));
                }
            }
            let output = if method == "pkgInstall" {
                packages::install(name)
            } else {
                packages::remove(name)
            }
            .map_err(Failure::unknown)?;
            Ok(json!({"result": true, "output": output}))
        }
        "rootHasPassword" => Ok(json!({"has_password": root_has_password()})),
        "createBackup" | "restoreBackup" | "validateFirmware" | "installFirmware" => {
            let path = argument(request, "path")?;
            let _guard = state
                .maintenance
                .lock()
                .map_err(|_| Failure::unknown("maintenance-operation lock poisoned"))?;
            match method {
                "createBackup" => create_backup(path)?,
                "restoreBackup" => restore_backup(path)?,
                "validateFirmware" => return validate_firmware(path),
                "installFirmware" => install_firmware(path)?,
                _ => unreachable!(),
            }
            Ok(json!({"result": true}))
        }
        "restart" => {
            spawn_system_action("/sbin/reboot", &[])?;
            Ok(json!({"result": true}))
        }
        "factoryReset" => {
            spawn_system_action("/sbin/firstboot", &["-r", "-y"])?;
            Ok(json!({"result": true}))
        }
        _ => Err(Failure {
            status: STATUS_METHOD_NOT_FOUND,
            message: format!("unknown method {method}"),
        }),
    }
}

// root_has_password reports whether the root account has a password set — the
// /etc/shadow read the de-privileged shell cannot do itself (ADR-007), brokered
// here instead of widening the shell's capabilities. It returns only this
// boolean, never the hash. Fails safe to true (an unreadable file or a missing
// root line reads as "has one"), so the shell never falsely warns.
fn root_has_password() -> bool {
    match fs::read_to_string("/etc/shadow") {
        Ok(shadow) => shadow_root_has_password(&shadow),
        Err(_) => true, // fail safe: an unreadable /etc/shadow reads as "has one"
    }
}

// Backups stay in OpenWrt's native sysupgrade format. The unprivileged shell
// creates the private temporary file; accepting only that exact shape keeps the
// helper from becoming an arbitrary root file writer/reader.
fn backup_path(path: &str, restore: bool) -> Result<&Path, Failure> {
    let path = Path::new(path);
    if path.parent() != Some(Path::new("/var/run/verso")) {
        return Err(Failure::invalid(
            "backup path is outside Verso's runtime directory",
        ));
    }
    let name = path.file_name().and_then(|v| v.to_str()).unwrap_or("");
    let prefix = if restore {
        "verso-restore-"
    } else {
        "verso-backup-"
    };
    if !name.starts_with(prefix) || !name.ends_with(".tar.gz") {
        return Err(Failure::invalid("invalid backup temporary file"));
    }
    let metadata = fs::symlink_metadata(path)
        .map_err(|error| Failure::invalid(format!("inspect backup file: {error}")))?;
    if !metadata.file_type().is_file() || metadata.uid() != VERSO_UID {
        return Err(Failure::invalid(
            "backup temporary file has invalid ownership or type",
        ));
    }
    Ok(path)
}

fn firmware_path(path: &str) -> Result<&Path, Failure> {
    let path = Path::new(path);
    if path.parent() != Some(Path::new("/var/run/verso")) {
        return Err(Failure::invalid(
            "firmware path is outside Verso's runtime directory",
        ));
    }
    let name = path
        .file_name()
        .and_then(|value| value.to_str())
        .unwrap_or("");
    if !name.starts_with("verso-firmware-") || !name.ends_with(".bin") {
        return Err(Failure::invalid("invalid firmware temporary file"));
    }
    let metadata = fs::symlink_metadata(path)
        .map_err(|error| Failure::invalid(format!("inspect firmware file: {error}")))?;
    if !metadata.file_type().is_file() || metadata.uid() != VERSO_UID {
        return Err(Failure::invalid(
            "firmware temporary file has invalid ownership or type",
        ));
    }
    if metadata.len() == 0 || metadata.len() > 128 * 1024 * 1024 {
        return Err(Failure::invalid(
            "firmware must be between 1 byte and 128 MiB",
        ));
    }
    Ok(path)
}

fn command_failure(name: &str, output: std::process::Output) -> Failure {
    let message = if output.stderr.is_empty() {
        String::from_utf8_lossy(&output.stdout).trim().to_string()
    } else {
        String::from_utf8_lossy(&output.stderr).trim().to_string()
    };
    Failure::unknown(format!("{name} failed: {message}"))
}

fn create_backup(path: &str) -> Result<(), Failure> {
    let path = backup_path(path, false)?;
    let output = Command::new("/sbin/sysupgrade")
        .arg("-b")
        .arg(path)
        .env_clear()
        .stdout(Stdio::piped())
        .stderr(Stdio::piped())
        .output()
        .map_err(|error| Failure::unknown(format!("create backup: {error}")))?;
    if !output.status.success() {
        return Err(command_failure("sysupgrade backup", output));
    }
    Ok(())
}

fn restore_backup(path: &str) -> Result<(), Failure> {
    let path = backup_path(path, true)?;
    let metadata = fs::metadata(path)
        .map_err(|error| Failure::invalid(format!("inspect backup size: {error}")))?;
    if metadata.len() == 0 || metadata.len() > 32 * 1024 * 1024 {
        return Err(Failure::invalid("backup must be between 1 byte and 32 MiB"));
    }

    // Listing first rejects corrupt/non-gzip input before sysupgrade extracts
    // anything. Absolute and parent-traversal members are refused explicitly.
    let mut listing = Command::new("/bin/tar")
        .args(["-tzf"])
        .arg(path)
        .env_clear()
        .stdout(Stdio::piped())
        .stderr(Stdio::null())
        .spawn()
        .map_err(|error| Failure::unknown(format!("inspect backup archive: {error}")))?;
    let mut names = Vec::new();
    listing
        .stdout
        .take()
        .ok_or_else(|| Failure::unknown("backup listing stdout unavailable"))?
        .take(8 * 1024 * 1024 + 1)
        .read_to_end(&mut names)
        .map_err(|error| Failure::unknown(format!("read backup listing: {error}")))?;
    if names.len() > 8 * 1024 * 1024 {
        let _ = listing.kill();
        let _ = listing.wait();
        return Err(Failure::invalid("backup contains too many paths"));
    }
    let status = listing
        .wait()
        .map_err(|error| Failure::unknown(format!("wait for backup listing: {error}")))?;
    if !status.success() {
        return Err(Failure::invalid(
            "the selected file is not a valid OpenWrt backup",
        ));
    }
    let names = String::from_utf8_lossy(&names);
    if names
        .lines()
        .any(|name| name.starts_with('/') || name.split('/').any(|part| part == ".."))
    {
        return Err(Failure::invalid("backup contains an unsafe path"));
    }

    let output = Command::new("/sbin/sysupgrade")
        .arg("-r")
        .arg(path)
        .env_clear()
        .stdout(Stdio::piped())
        .stderr(Stdio::piped())
        .output()
        .map_err(|error| Failure::unknown(format!("restore backup: {error}")))?;
    if !output.status.success() {
        let mountinfo = fs::read_to_string("/proc/self/mountinfo").unwrap_or_default();
        if restore_failed_only_on_container_mounts(&output.stdout, &output.stderr, &mountinfo) {
            eprintln!(
                "verso-rpcd: restore skipped Docker-managed mount: {}",
                command_output(&output)
            );
        } else {
            return Err(command_failure("sysupgrade restore", output));
        }
    }
    spawn_system_action("/sbin/reboot", &[])?;
    Ok(())
}

fn firmware_test(path: &Path) -> Result<std::process::Output, Failure> {
    Command::new("/sbin/sysupgrade")
        .arg("--test")
        .arg(path)
        .env_clear()
        .stdout(Stdio::piped())
        .stderr(Stdio::piped())
        .output()
        .map_err(|error| Failure::unknown(format!("validate firmware: {error}")))
}

fn firmware_metadata(path: &Path) -> Value {
    let Ok(output) = Command::new("/usr/bin/fwtool")
        .args(["-q", "-i", "-"])
        .arg(path)
        .env_clear()
        .stdout(Stdio::piped())
        .stderr(Stdio::null())
        .output()
    else {
        return Value::Null;
    };
    if !output.status.success() {
        return Value::Null;
    }
    serde_json::from_slice(&output.stdout).unwrap_or(Value::Null)
}

fn firmware_version_field<'a>(metadata: &'a Value, field: &str) -> &'a str {
    metadata
        .get("version")
        .and_then(|version| version.get(field))
        .and_then(Value::as_str)
        .unwrap_or("")
}

fn validate_firmware(path: &str) -> Result<Value, Failure> {
    let path = firmware_path(path)?;
    let output = firmware_test(path)?;
    let metadata = firmware_metadata(path);
    Ok(json!({
        "valid": output.status.success(),
        "version": firmware_version_field(&metadata, "version"),
        "revision": firmware_version_field(&metadata, "revision"),
        "target": firmware_version_field(&metadata, "target"),
        "board": firmware_version_field(&metadata, "board"),
        "error": if output.status.success() { String::new() } else { command_output(&output) },
    }))
}

fn install_firmware(path: &str) -> Result<(), Failure> {
    let path = firmware_path(path)?;
    let validation = firmware_test(path)?;
    if !validation.status.success() {
        return Err(Failure::invalid(format!(
            "firmware validation failed: {}",
            command_output(&validation)
        )));
    }

    // The shell removes its pending upload once this method returns. Rename the
    // image within tmpfs first so the detached sysupgrade process owns a stable
    // path for the rest of the upgrade.
    let install_path = Path::new("/tmp/verso-firmware-install.bin");
    match fs::remove_file(install_path) {
        Ok(()) => {}
        Err(error) if error.kind() == std::io::ErrorKind::NotFound => {}
        Err(error) => {
            return Err(Failure::unknown(format!(
                "remove previous firmware image: {error}"
            )))
        }
    }
    fs::rename(path, install_path)
        .map_err(|error| Failure::unknown(format!("prepare firmware install: {error}")))?;
    if let Err(error) = Command::new("/sbin/sysupgrade")
        .arg(install_path)
        .env_clear()
        .stdin(Stdio::null())
        .stdout(Stdio::null())
        .stderr(Stdio::null())
        .spawn()
    {
        if let Err(restore_error) = fs::rename(install_path, path) {
            return Err(Failure::unknown(format!(
                "start firmware install: {error}; restore verified image: {restore_error}"
            )));
        }
        return Err(Failure::unknown(format!("start firmware install: {error}")));
    }
    Ok(())
}

// Docker supplies these three files as immutable bind mounts. A native OpenWrt
// device does not. Accept BusyBox tar's failure only when every reported path is
// one of those files and mountinfo proves it is a mount in this process; any
// other extraction error remains fatal.
fn command_output(output: &std::process::Output) -> String {
    [output.stdout.as_slice(), output.stderr.as_slice()]
        .into_iter()
        .flat_map(|bytes| {
            String::from_utf8_lossy(bytes)
                .lines()
                .map(str::to_owned)
                .collect::<Vec<_>>()
        })
        .filter(|line| !line.trim().is_empty())
        .collect::<Vec<_>>()
        .join("\n")
}

fn restore_failed_only_on_container_mounts(stdout: &[u8], stderr: &[u8], mountinfo: &str) -> bool {
    let mut found = false;
    let output = [stdout, stderr].into_iter().flat_map(|bytes| {
        String::from_utf8_lossy(bytes)
            .lines()
            .map(str::to_owned)
            .collect::<Vec<_>>()
    });
    for line in output.filter(|line| !line.trim().is_empty()) {
        if line.ends_with("upgrade: Restoring config files...") {
            continue;
        }
        let Some(path) = line
            .strip_prefix("tar: can't remove old file ")
            .and_then(|line| line.strip_suffix(": Resource busy"))
        else {
            return false;
        };
        if !matches!(path, "etc/hosts" | "etc/hostname" | "etc/resolv.conf") {
            return false;
        }
        let target = format!("/{path}");
        if !mountinfo
            .lines()
            .any(|mount| mount.split_whitespace().nth(4) == Some(target.as_str()))
        {
            return false;
        }
        found = true;
    }
    found
}

fn spawn_system_action(program: &str, args: &[&str]) -> Result<(), Failure> {
    Command::new(program)
        .args(args)
        .env_clear()
        .stdin(Stdio::null())
        .stdout(Stdio::null())
        .stderr(Stdio::null())
        .spawn()
        .map_err(|error| Failure::unknown(format!("start {program}: {error}")))?;
    Ok(())
}

// shadow_root_has_password parses an /etc/shadow body for root's password field.
// The hash field being non-empty counts as "has a password" (a locked `!`/`*`
// included); a missing root line reads as "has one" — fail safe, never a false
// warning.
fn shadow_root_has_password(shadow: &str) -> bool {
    for line in shadow.lines() {
        if let Some(rest) = line.strip_prefix("root:") {
            return !rest.split(':').next().unwrap_or("").is_empty();
        }
    }
    true
}

fn package_guard(state: &State) -> Result<std::sync::MutexGuard<'_, ()>, Failure> {
    state
        .packages
        .lock()
        .map_err(|_| Failure::unknown("package-operation lock poisoned"))
}

fn field<'a>(request: &'a Value, name: &str) -> Result<&'a str, Failure> {
    request
        .get(name)
        .and_then(Value::as_str)
        .filter(|value| !value.is_empty())
        .ok_or_else(|| Failure::invalid(format!("{name} is required")))
}

fn argument<'a>(request: &'a Value, name: &str) -> Result<&'a str, Failure> {
    request
        .get("args")
        .and_then(|args| args.get(name))
        .and_then(Value::as_str)
        .filter(|value| !value.is_empty())
        .ok_or_else(|| Failure::invalid(format!("{name} is required")))
}

fn set_password(username: &str, password: &str) -> Result<(), Failure> {
    // A control character (newline especially) can't survive passwd's two-line
    // stdin protocol, so reject it up front rather than silently fail to set it.
    if !valid_username(username) || password.len() > 1024 || password.chars().any(char::is_control)
    {
        return Err(Failure::invalid("invalid username or password"));
    }
    let mut child = Command::new("passwd")
        .arg(username)
        .stdin(Stdio::piped())
        .stdout(Stdio::piped())
        .stderr(Stdio::piped())
        .spawn()
        .map_err(|error| Failure::unknown(format!("passwd {username:?}: {error}")))?;
    child
        .stdin
        .as_mut()
        .ok_or_else(|| Failure::unknown("passwd stdin unavailable"))?
        .write_all(format!("{password}\n{password}\n").as_bytes())
        .map_err(|error| Failure::unknown(format!("passwd stdin: {error}")))?;
    let output = child
        .wait_with_output()
        .map_err(|error| Failure::unknown(format!("passwd wait: {error}")))?;
    if !output.status.success() {
        let message = if output.stderr.is_empty() {
            String::from_utf8_lossy(&output.stdout).trim().to_string()
        } else {
            String::from_utf8_lossy(&output.stderr).trim().to_string()
        };
        return Err(Failure::unknown(format!("passwd failed: {message}")));
    }
    Ok(())
}

// set_system_time is deliberately a structured verb, not a command runner. The
// browser supplies a local wall-clock value and the System plugin's selected
// POSIX timezone; both are validated here again at the root boundary. BusyBox
// date performs the one settimeofday operation, with no shell involved.
fn set_system_time(datetime: &str, timezone: &str) -> Result<(), Failure> {
    let normalized = normalize_local_datetime(datetime)
        .ok_or_else(|| Failure::invalid("invalid local date and time"))?;
    if !valid_posix_timezone(timezone) {
        return Err(Failure::invalid("invalid timezone"));
    }
    let output = Command::new("/bin/date")
        .args(["-s", &normalized])
        .env_clear()
        .env("TZ", timezone)
        .stdout(Stdio::piped())
        .stderr(Stdio::piped())
        .output()
        .map_err(|error| Failure::unknown(format!("set system time: {error}")))?;
    if !output.status.success() {
        let message = if output.stderr.is_empty() {
            String::from_utf8_lossy(&output.stdout).trim().to_string()
        } else {
            String::from_utf8_lossy(&output.stderr).trim().to_string()
        };
        return Err(Failure::unknown(format!("date failed: {message}")));
    }
    Ok(())
}

fn normalize_local_datetime(value: &str) -> Option<String> {
    let bytes = value.as_bytes();
    if bytes.len() != 19
        || bytes[4] != b'-'
        || bytes[7] != b'-'
        || bytes[10] != b'T'
        || bytes[13] != b':'
        || bytes[16] != b':'
    {
        return None;
    }
    let number = |from: usize, to: usize| value[from..to].parse::<u32>().ok();
    let (year, month, day, hour, minute, second) = (
        number(0, 4)?,
        number(5, 7)?,
        number(8, 10)?,
        number(11, 13)?,
        number(14, 16)?,
        number(17, 19)?,
    );
    if !(1970..=9999).contains(&year)
        || !(1..=12).contains(&month)
        || hour > 23
        || minute > 59
        || second > 59
    {
        return None;
    }
    let leap = year % 4 == 0 && (year % 100 != 0 || year % 400 == 0);
    let days = match month {
        2 if leap => 29,
        2 => 28,
        4 | 6 | 9 | 11 => 30,
        _ => 31,
    };
    if !(1..=days).contains(&day) {
        return None;
    }
    Some(format!(
        "{year:04}-{month:02}-{day:02} {hour:02}:{minute:02}:{second:02}"
    ))
}

fn valid_posix_timezone(value: &str) -> bool {
    !value.is_empty()
        && value.len() <= 128
        && value.bytes().all(|byte| {
            byte.is_ascii_alphanumeric()
                || matches!(
                    byte,
                    b'+' | b'-' | b':' | b',' | b'.' | b'<' | b'>' | b'/' | b'_'
                )
        })
}

fn valid_username(username: &str) -> bool {
    let bytes = username.as_bytes();
    (1..=32).contains(&bytes.len())
        && (bytes[0].is_ascii_lowercase() || bytes[0] == b'_')
        && bytes.iter().all(|byte| {
            byte.is_ascii_lowercase() || byte.is_ascii_digit() || matches!(byte, b'_' | b'-')
        })
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn root_password_detection() {
        // empty hash field = no password (a fresh boot)
        assert!(!shadow_root_has_password("root::0:99999:7:::\n"));
        // a hashed (or locked) password = has one
        assert!(shadow_root_has_password("root:$1$abc$xyz0:0:99999:7:::\n"));
        assert!(shadow_root_has_password("root:!:0:99999:7:::\n"));
        // root need not be the first line
        assert!(shadow_root_has_password(
            "daemon:*:0:::\nroot:$6$h$h:0:::\n"
        ));
        // no root line and an empty file both fail safe to "has one"
        assert!(shadow_root_has_password("daemon:*:0:::\nnobody:*:0:::\n"));
        assert!(shadow_root_has_password(""));
    }

    #[test]
    fn only_verified_container_mount_conflicts_are_ignorable() {
        let mounts = "123 1 0:1 / / rw - overlay overlay rw\n124 123 0:2 /hosts /etc/hosts rw - tmpfs tmpfs rw\n";
        assert!(restore_failed_only_on_container_mounts(
            b"Sun Aug 30 22:24:37 CEST 2026 upgrade: Restoring config files...\n",
            b"tar: can't remove old file etc/hosts: Resource busy\n",
            mounts
        ));
        assert!(restore_failed_only_on_container_mounts(
            b"Sun Aug 30 22:24:37 CEST 2026 upgrade: Restoring config files...\ntar: can't remove old file etc/hosts: Resource busy\n",
            b"",
            mounts
        ));
        assert!(!restore_failed_only_on_container_mounts(
            b"",
            b"tar: can't remove old file etc/config/system: Resource busy\n",
            mounts
        ));
        assert!(!restore_failed_only_on_container_mounts(
            b"",
            b"tar: short read\n",
            mounts
        ));
        assert!(!restore_failed_only_on_container_mounts(
            b"",
            b"tar: can't remove old file etc/hosts: Resource busy\n",
            ""
        ));
    }

    #[test]
    fn firmware_metadata_fields_are_optional_and_bounded_to_version_object() {
        let metadata = json!({
            "version": {
                "version": "25.12.5",
                "revision": "r123-abc",
                "target": "layerscape/armv8_64b",
                "board": "mono_gateway-dk"
            },
            "board": "wrong-level"
        });
        assert_eq!(firmware_version_field(&metadata, "version"), "25.12.5");
        assert_eq!(
            firmware_version_field(&metadata, "board"),
            "mono_gateway-dk"
        );
        assert_eq!(firmware_version_field(&metadata, "missing"), "");
        assert_eq!(firmware_version_field(&Value::Null, "version"), "");
    }

    #[test]
    fn response_shape_preserves_status() {
        assert_eq!(response(Ok(json!({"x": 1})))["status"], STATUS_OK);
        assert_eq!(
            response(Err(Failure::invalid("bad")))["status"],
            STATUS_INVALID_ARGUMENT
        );
    }

    #[test]
    fn usernames_cannot_be_passwd_options() {
        assert!(valid_username("root"));
        assert!(valid_username("service-user"));
        for username in ["", "-d", "Root", "a b", "../root"] {
            assert!(!valid_username(username), "accepted {username:?}");
        }
    }

    #[test]
    fn local_datetime_is_validated_and_normalized() {
        assert_eq!(
            normalize_local_datetime("2026-08-30T12:34:56").as_deref(),
            Some("2026-08-30 12:34:56")
        );
        assert_eq!(
            normalize_local_datetime("2024-02-29T00:00:00").as_deref(),
            Some("2024-02-29 00:00:00")
        );
        for value in [
            "2026-08-30T12:34",
            "2026-02-29T12:34:56",
            "2026-08-30T24:00:00",
            "2026-13-01T00:00:00",
        ] {
            assert!(
                normalize_local_datetime(value).is_none(),
                "accepted {value}"
            );
        }
    }

    #[test]
    fn timezone_cannot_become_an_environment_or_shell_escape() {
        for value in ["GMT0", "CET-1CEST,M3.5.0,M10.5.0/3", "<+03>-3"] {
            assert!(valid_posix_timezone(value), "rejected {value}");
        }
        for value in ["", "UTC;reboot", "UTC\nPATH=/tmp", "UTC=value"] {
            assert!(!valid_posix_timezone(value), "accepted {value:?}");
        }
    }
}
