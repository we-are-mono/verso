// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

//! Persistent privileged companion for Verso.
//!
//! The Go shell is deliberately unprivileged. This daemon owns the few root
//! operations it needs and listens on a group-protected Unix socket. Every
//! request still carries the operator's rpcd session and is independently
//! checked through native ubus `session.access` before any action runs.

mod access;
mod arrival;
mod config_files;
mod dhcp;
mod firewall;
mod firewall_log;
mod firmware;
mod packages;
mod ubus;

use serde_json::{json, Value};
use std::ffi::c_void;
use std::fs;
use std::io::{BufRead, BufReader, Read, Write};
use std::mem;
use std::os::fd::AsRawFd;
use std::os::unix::fs::MetadataExt;
use std::os::unix::fs::OpenOptionsExt;
use std::os::unix::fs::PermissionsExt;
use std::os::unix::net::{UnixListener, UnixStream};
use std::path::{Path, PathBuf};
use std::process::{Command, Stdio};
use std::sync::atomic::{AtomicBool, AtomicUsize, Ordering};
use std::sync::{Arc, Mutex};
use std::time::Duration;

const DEFAULT_SOCKET: &str = "/var/run/verso/verso-rpcd.sock";
const MAX_REQUEST: u64 = 256 * 1024;
const VERSO_UID: u32 = 6000;

/// rpcd's local-root convention: automation running as root on the device itself
/// has no operator session to borrow and presents an all-zero session id instead.
/// rpcd's own `session.access` answers `false` for it on every verb, so honoring
/// it is this helper's decision to make, and the bound below is where it is made.
const ZERO_SID: &str = "00000000000000000000000000000000";
const SOL_SOCKET: i32 = 1;
const SO_PEERCRED: i32 = 17;

const STATUS_OK: i32 = 0;
const STATUS_INVALID_ARGUMENT: i32 = 2;
const STATUS_METHOD_NOT_FOUND: i32 = 3;
const STATUS_PERMISSION_DENIED: i32 = 6;
const STATUS_UNKNOWN_ERROR: i32 = 9;

struct State {
    firewall_log: firewall_log::Collector,
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

#[derive(Debug)]
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
    if firewall_log::command_line() {
        return;
    }
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
    println!("verso-rpcd: listening on {}", socket.display());

    config_files::watchdog();
    let collector = firewall_log::Collector::new();
    collector.start();
    let state = Arc::new(State {
        firewall_log: collector,
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
        Ok(request) => response(dispatch(&request, state, uid)),
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

/// The zero session's whole grant: the two verbs that only read what this device
/// could install, and only for a caller the kernel says is root. Unattended work
/// runs under this and nothing else, so the blast radius of the daily update check
/// is "the router found out" (ADR-014 §3). The shell's own requests never take
/// this path — an operator's action always carries that operator's session — so
/// uid 6000 gets nothing here however it asks.
fn zero_session_grants(uid: u32, method: &str) -> bool {
    uid == 0 && matches!(method, "pkgUpgradable" | "firmwareCheck")
}

fn dispatch(request: &Value, state: &State, uid: u32) -> Result<Value, Failure> {
    let method = field(request, "method")?;
    let sid = field(request, "sid")?;
    let denied = || Failure {
        status: STATUS_PERMISSION_DENIED,
        message: "permission denied".into(),
    };
    if sid == ZERO_SID {
        // rpcd holds no session to consult for the zero sid — asking it would deny
        // every verb, including the two this path exists for. The peer's kernel-
        // reported uid and the grant above are the whole decision.
        if !zero_session_grants(uid, method) {
            return Err(denied());
        }
    } else {
        match ubus::access(sid, method) {
            Ok(true) => {}
            Ok(false) => return Err(denied()),
            Err(error) => {
                eprintln!("verso-rpcd: authorization failed for {method}: {error}");
                return Err(denied());
            }
        }
    }

    match method {
        "dhcpState" => dhcp::state(),
        "dnsState" => config_files::dns_state(),
        "firewallFiles" => config_files::firewall_state(),
        "configFiles" => config_files::state(),
        "stageConfigFile" => config_files::stage(
            argument(request, "path")?,
            argument(request, "expected")?,
            argument(request, "content")?,
        ),
        "applyConfigFiles" => config_files::lifecycle(
            argument(request, "action")?,
            argument(request, "uci")? == "1",
            argument(request, "timeout")?.parse().unwrap_or(30),
            match argument(request, "family") {
                Ok(name) => Some(
                    config_files::Family::named(name)
                        .ok_or_else(|| Failure::invalid("Unknown file family."))?,
                ),
                Err(_) => None,
            },
        ),
        "accessCredentials" => access::state(),
        "setAuthorizedKeys" => {
            let _guard = state
                .maintenance
                .lock()
                .map_err(|_| Failure::unknown("credential-operation lock poisoned"))?;
            access::keys(
                text_argument(request, "expected")?,
                text_argument(request, "keys")?,
            )
        }
        "setWebCertificate" => {
            let _guard = state
                .maintenance
                .lock()
                .map_err(|_| Failure::unknown("credential-operation lock poisoned"))?;
            access::certificate(argument(request, "certificate")?, argument(request, "key")?)
        }
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
        "pkgUpgradable" => {
            let _guard = package_guard(state)?;
            let found = packages::upgradable().map_err(Failure::unknown)?;
            Ok(json!({"packages": found}))
        }
        "pkgUpgrade" => {
            let _guard = package_guard(state)?;
            let output = packages::upgrade().map_err(Failure::unknown)?;
            Ok(json!({"result": true, "output": output}))
        }
        "firmwareCheck" => Ok(firmware_check()),
        // The one act that takes both guards, and the only place two are held at
        // once — so this order is the whole lock order and no inversion exists to
        // deadlock against. It earns both: owut builds its request from the
        // installed package set and then flashes, so an apk transaction running
        // underneath it would be reading and writing the very thing being
        // replaced, and a manual image install landing beside it would be a
        // second sysupgrade. What the guards do not cover is the moment after
        // the detached install starts — by then sysupgrade owns the device and
        // is taking every other process down with it.
        "firmwareUpgrade" => {
            let _packages = package_guard(state)?;
            let _maintenance = state
                .maintenance
                .lock()
                .map_err(|_| Failure::unknown("maintenance-operation lock poisoned"))?;
            firmware_upgrade()?;
            Ok(json!({"result": true}))
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
        "pkgBrowse" => {
            let query = request
                .get("args")
                .and_then(|args| args.get("query"))
                .and_then(Value::as_str)
                .ok_or_else(|| Failure::invalid("query is required"))?;
            if query.len() > 128 || query.chars().any(char::is_control) {
                return Err(Failure::invalid("invalid package query"));
            }
            let offset = argument(request, "offset")?
                .parse::<usize>()
                .ok()
                .filter(|offset| *offset <= 100_000)
                .ok_or_else(|| Failure::invalid("invalid package offset"))?;
            let _guard = package_guard(state)?;
            packages::browse(query, offset).map_err(Failure::unknown)
        }
        "pkgFiles" => {
            let name = argument(request, "package")?;
            if !packages::valid_name(name) {
                return Err(Failure::invalid("invalid package name"));
            }
            let _guard = package_guard(state)?;
            Ok(json!({"files": packages::files(name).map_err(Failure::unknown)?}))
        }
        "pkgInstall" | "pkgRemove" | "pkgUpgradeOne" => {
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
            } else if method == "pkgUpgradeOne" {
                packages::upgrade_one(name)
            } else {
                packages::remove(name)
            }
            .map_err(Failure::unknown)?;
            Ok(json!({"result": true, "output": output}))
        }
        "rootHasPassword" => Ok(json!({"has_password": root_has_password()})),
        "firewallCounters" => firewall_counters(),
        "firewallStatus" => firewall_status(),
        "firewallLog" => state.firewall_log.read(&request["args"]),
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

// firewall_counters reads fw4's live nftables ruleset and reduces it to per-rule
// hit counters. Listing a table needs CAP_NET_ADMIN, which the de-privileged shell
// does not hold (ADR-007), so the read is brokered here; nft is executed directly,
// never through a shell, and the listing is a read with no arguments to validate.
fn firewall_counters() -> Result<Value, Failure> {
    let mut command = Command::new("/usr/sbin/nft");
    command.args(["-j", "list", "table", "inet", "fw4"]);
    let output = run_bounded(command, "list firewall counters", 4 * 1024 * 1024)?;
    if !output.status.success() {
        return Err(command_failure("list firewall counters", output));
    }
    firewall::counters(&output.stdout).map_err(Failure::unknown)
}

fn firewall_status() -> Result<Value, Failure> {
    let mut command = Command::new("/usr/sbin/nft");
    // Omit set elements: the overview needs the loaded chains and rule count,
    // not potentially large blocklists. A missing fw4 table is a valid reading.
    command.args(["-j", "-t", "list", "ruleset", "inet"]);
    let output = run_bounded(command, "read firewall status", 4 * 1024 * 1024)?;
    if !output.status.success() {
        return Err(command_failure("read firewall status", output));
    }
    firewall::status(&output.stdout).map_err(Failure::unknown)
}

// firmware_check asks owut whether the device's attended-sysupgrade server can
// build it a newer image. owut reads the whole system — board, installed packages,
// uci — and talks to the network, which the de-privileged shell cannot do (ADR-007),
// so the run is brokered here; the binary is executed directly and only ever with
// `check`, which builds, downloads and flashes nothing.
//
// A device owut cannot answer for is not an error: the method reports the rung
// instead, so the page states a plain fact rather than a failure.
const OWUT: &str = "/usr/bin/owut";

fn firmware_check() -> Value {
    if !Path::new(OWUT).exists() {
        return firmware::unavailable(
            firmware::STATE_NO_OWUT,
            "owut is not installed on this device",
        );
    }
    let mut command = Command::new(OWUT);
    command.arg("check");
    match run_bounded(command, "check for firmware updates", 1 << 20) {
        Ok(output) => firmware::summarize(&output.stdout, &output.stderr, output.status.success()),
        Err(failure) => firmware::unavailable(firmware::STATE_UNSUPPORTED, &failure.message),
    }
}

// firmware_upgrade is the act the check leads to: owut asks the device's
// attended-sysupgrade server to build this exact device an image, downloads it,
// verifies it, and then hands the device to sysupgrade.
//
// The argv is fixed — `owut download`, then `owut install` — and nothing from the
// request reaches it. owut reads the board, the installed package set and uci
// itself, so there is no version, package name or path for a caller to steer
// (ADR-007); no shell is involved; and owut reads no stdin, so the run is
// non-interactive by construction rather than by a confirmation flag.
//
// The two phases are what make a failure reportable. The download is where an
// upgrade actually fails — no server, a build the server refused, a package set
// it cannot resolve — and it is bounded and waited on, so owut's own words come
// back. The install is detached: it ends in sysupgrade, which tears this daemon
// down with everything else, so there is no completion left to wait for.
const OWUT_IMAGE: &str = "/tmp/firmware.bin";
const OWUT_SUMS: &str = "/tmp/firmware.sha256sums";
const OWUT_MAX_IMAGE: u64 = 128 * 1024 * 1024;
const OWUT_MAX_SUMS: u64 = 64 * 1024;
// An attended-sysupgrade build is a queue plus a compile on someone else's
// machine, then a download of the image it produced.
const OWUT_DOWNLOAD_TIMEOUT: Duration = Duration::from_secs(20 * 60);

fn firmware_upgrade() -> Result<(), Failure> {
    if !Path::new(OWUT).exists() {
        return Err(Failure::invalid("owut is not installed on this device"));
    }
    let mut download = Command::new(OWUT);
    download.arg("download");
    let output = run_bounded_within(
        download,
        "download the new firmware",
        1 << 20,
        OWUT_DOWNLOAD_TIMEOUT,
    )?;
    if !output.status.success() {
        let complaint = firmware::complaint_of(&output.stdout, &output.stderr);
        return Err(Failure::unknown(if complaint.is_empty() {
            "the upgrade tool failed without saying why".to_string()
        } else {
            complaint
        }));
    }
    // owut writes both artifacts as root. A file planted at either path before
    // the run would be written through — root ignores the mode — and left owned
    // by whoever planted it, so ownership is what proves these are owut's own
    // and not a compromised shell's substitution (ADR-007). It is the same bound
    // firmware_path applies to an uploaded image.
    owut_artifact(OWUT_IMAGE, OWUT_MAX_IMAGE)?;
    owut_artifact(OWUT_SUMS, OWUT_MAX_SUMS)?;
    spawn_system_action(OWUT, &["install"])
}

fn owut_artifact(path: &str, max: u64) -> Result<(), Failure> {
    let metadata = fs::symlink_metadata(path)
        .map_err(|error| Failure::unknown(format!("inspect {path}: {error}")))?;
    if !plausible_artifact(
        metadata.file_type().is_file(),
        metadata.uid(),
        metadata.len(),
        max,
    ) {
        return Err(Failure::unknown(format!(
            "{path} is not the file the upgrade tool wrote"
        )));
    }
    Ok(())
}

// plausible_artifact is the whole test: a regular file (never a symlink or a
// device), owned by root, and of a size the thing it claims to be could have.
fn plausible_artifact(is_file: bool, uid: u32, len: u64, max: u64) -> bool {
    is_file && uid == 0 && len > 0 && len <= max
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

// Maintenance uploads live in a group-writable runtime dir, so under ADR-007's
// compromised-shell model the staged file could be swapped between a check and
// its use. Each op first copies the upload into a root-owned working file in
// sticky /tmp — root clears any pre-planted entry, then O_EXCL|O_NOFOLLOW create
// so a racing plant loses rather than redirects — and then validates and acts on
// that immutable copy. No other account can rename, unlink, or follow it. The
// maintenance mutex serializes callers, so fixed working names are safe.
const O_NOFOLLOW: i32 = 0o400000;

fn root_open_new(work: &str) -> Result<(fs::File, PathBuf), Failure> {
    let dest = Path::new("/tmp").join(work);
    match fs::remove_file(&dest) {
        Ok(()) => {}
        Err(error) if error.kind() == std::io::ErrorKind::NotFound => {}
        Err(error) => return Err(Failure::unknown(format!("clear staging {work}: {error}"))),
    }
    let file = fs::OpenOptions::new()
        .write(true)
        .create_new(true)
        .custom_flags(O_NOFOLLOW)
        .mode(0o600)
        .open(&dest)
        .map_err(|error| Failure::unknown(format!("create staging {work}: {error}")))?;
    Ok((file, dest))
}

fn root_stage(src: &Path, work: &str) -> Result<PathBuf, Failure> {
    let (mut out, dest) = root_open_new(work)?;
    let mut input =
        fs::File::open(src).map_err(|error| Failure::unknown(format!("open upload: {error}")))?;
    std::io::copy(&mut input, &mut out)
        .map_err(|error| Failure::unknown(format!("stage upload: {error}")))?;
    Ok(dest)
}

// write_back hands a root-produced archive to the shell's file without following
// a symlink swapped in at that path.
fn write_back(work: &Path, dest: &Path) -> Result<(), Failure> {
    let mut input = fs::File::open(work)
        .map_err(|error| Failure::unknown(format!("open staged backup: {error}")))?;
    let mut out = fs::OpenOptions::new()
        .write(true)
        .truncate(true)
        .custom_flags(O_NOFOLLOW)
        .open(dest)
        .map_err(|error| Failure::unknown(format!("write backup to upload path: {error}")))?;
    std::io::copy(&mut input, &mut out)
        .map_err(|error| Failure::unknown(format!("copy backup to upload path: {error}")))?;
    Ok(())
}

fn remove_quietly(path: &Path) {
    let _ = fs::remove_file(path);
}

// run_bounded runs a child under a wall-clock cap so a stalled tool cannot pin its
// worker thread — and any mutex that thread holds — indefinitely, and caps captured
// stdout so a hostile archive listing or a vast ruleset cannot exhaust memory. On
// the deadline a kill ends the child and the call reports a timeout.
//
// Two minutes is the cap for a tool working on this device. A run that waits on
// someone else's machine says its own cap through run_bounded_within: an
// attended-sysupgrade build is minutes of a remote compile, and a two-minute
// deadline would report that as a failure of this router.
const CHILD_TIMEOUT: Duration = Duration::from_secs(120);

fn run_bounded(
    command: Command,
    name: &str,
    max_stdout: usize,
) -> Result<std::process::Output, Failure> {
    run_bounded_within(command, name, max_stdout, CHILD_TIMEOUT)
}

fn run_bounded_within(
    command: Command,
    name: &str,
    max_stdout: usize,
    timeout: Duration,
) -> Result<std::process::Output, Failure> {
    run_bounded_with_stdin(command, name, max_stdout, timeout, Stdio::null())
}

fn run_bounded_with_stdin(
    mut command: Command,
    name: &str,
    max_stdout: usize,
    timeout: Duration,
    stdin: Stdio,
) -> Result<std::process::Output, Failure> {
    command
        .env_clear()
        .stdin(stdin)
        .stdout(Stdio::piped())
        .stderr(Stdio::piped());
    let mut child = command
        .spawn()
        .map_err(|error| Failure::unknown(format!("{name}: {error}")))?;
    let mut child_stdout = child.stdout.take();
    let mut child_stderr = child.stderr.take();
    let over = Arc::new(AtomicBool::new(false));
    let stdout_reader = {
        let over = Arc::clone(&over);
        std::thread::spawn(move || {
            let mut buffer = Vec::new();
            if let Some(stream) = child_stdout.as_mut() {
                let _ = stream
                    .take((max_stdout as u64) + 1)
                    .read_to_end(&mut buffer);
                if buffer.len() > max_stdout {
                    over.store(true, Ordering::Release);
                }
            }
            buffer
        })
    };
    let stderr_reader = std::thread::spawn(move || {
        let mut buffer = Vec::new();
        if let Some(stream) = child_stderr.as_mut() {
            let _ = stream.take(64 * 1024).read_to_end(&mut buffer);
        }
        buffer
    });
    let deadline = std::time::Instant::now() + timeout;
    let mut timed_out = false;
    let status = loop {
        if over.load(Ordering::Acquire) {
            let _ = child.kill();
            break child
                .wait()
                .map_err(|error| Failure::unknown(format!("{name}: {error}")))?;
        }
        match child.try_wait() {
            Ok(Some(status)) => break status,
            Ok(None) => {
                if std::time::Instant::now() >= deadline {
                    let _ = child.kill();
                    timed_out = true;
                    break child
                        .wait()
                        .map_err(|error| Failure::unknown(format!("{name}: {error}")))?;
                }
                std::thread::sleep(Duration::from_millis(50));
            }
            Err(error) => return Err(Failure::unknown(format!("{name}: {error}"))),
        }
    };
    let stdout = stdout_reader.join().unwrap_or_default();
    let stderr = stderr_reader.join().unwrap_or_default();
    if timed_out {
        return Err(Failure::unknown(format!("{name} timed out")));
    }
    if over.load(Ordering::Acquire) {
        return Err(Failure::invalid(format!("{name} produced too much output")));
    }
    Ok(std::process::Output {
        status,
        stdout,
        stderr,
    })
}

fn create_backup(path: &str) -> Result<(), Failure> {
    let path = backup_path(path, false)?;
    // sysupgrade writes into a root-owned working file, so a symlink swapped in at
    // the shell's path cannot redirect the archive write; then hand it back.
    let (_file, work) = root_open_new("verso-backup-work.tar.gz")?;
    let mut command = Command::new("/sbin/sysupgrade");
    command.arg("-b").arg(&work);
    let output = run_bounded(command, "sysupgrade backup", 1 << 20)?;
    if !output.status.success() {
        remove_quietly(&work);
        return Err(command_failure("sysupgrade backup", output));
    }
    let result = write_back(&work, path);
    remove_quietly(&work);
    result
}

fn restore_backup(path: &str) -> Result<(), Failure> {
    let path = backup_path(path, true)?;
    let metadata = fs::metadata(path)
        .map_err(|error| Failure::invalid(format!("inspect backup size: {error}")))?;
    if metadata.len() == 0 || metadata.len() > 32 * 1024 * 1024 {
        return Err(Failure::invalid("backup must be between 1 byte and 32 MiB"));
    }
    // Copy into a root-owned working file, then list members and extract from that
    // same immutable copy: a swap between the safety scan and the restore cannot
    // substitute a malicious archive.
    let work = root_stage(path, "verso-restore-work.tar.gz")?;
    let result = restore_staged(&work);
    remove_quietly(&work);
    result
}

fn restore_staged(work: &Path) -> Result<(), Failure> {
    // List first, rejecting corrupt/non-gzip input and absolute/parent-traversal
    // members before sysupgrade extracts anything.
    let mut listing = Command::new("/bin/tar");
    listing.arg("-tzf").arg(work);
    let listed = run_bounded(listing, "inspect backup archive", 8 * 1024 * 1024)?;
    if !listed.status.success() {
        return Err(Failure::invalid(
            "the selected file is not a valid OpenWrt backup",
        ));
    }
    let names = String::from_utf8_lossy(&listed.stdout);
    if names
        .lines()
        .any(|name| name.starts_with('/') || name.split('/').any(|part| part == ".."))
    {
        return Err(Failure::invalid("backup contains an unsafe path"));
    }

    let mut restore = Command::new("/sbin/sysupgrade");
    restore.arg("-r").arg(work);
    let output = run_bounded(restore, "sysupgrade restore", 1 << 20)?;
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
    let mut command = Command::new("/sbin/sysupgrade");
    command.arg("--test").arg(path);
    run_bounded(command, "validate firmware", 1 << 20)
}

fn firmware_metadata(path: &Path) -> Value {
    let mut command = Command::new("/usr/bin/fwtool");
    command.args(["-q", "-i", "-"]).arg(path);
    let Ok(output) = run_bounded(command, "firmware metadata", 1 << 20) else {
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
    let work = root_stage(path, "verso-firmware-verify.bin")?;
    let output = firmware_test(&work)?;
    let metadata = firmware_metadata(&work);
    remove_quietly(&work);
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
    // Copy into a root-owned image the shell cannot swap, then validate and flash
    // that same copy. It also gives the detached sysupgrade a stable path after
    // the shell removes its pending upload.
    let install_path = root_stage(path, "verso-firmware-install.bin")?;
    let validation = firmware_test(&install_path)?;
    if !validation.status.success() {
        remove_quietly(&install_path);
        return Err(Failure::invalid(format!(
            "firmware validation failed: {}",
            command_output(&validation)
        )));
    }
    if let Err(error) = Command::new("/sbin/sysupgrade")
        .arg(&install_path)
        .env_clear()
        .stdin(Stdio::null())
        .stdout(Stdio::null())
        .stderr(Stdio::null())
        .spawn()
    {
        remove_quietly(&install_path);
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

// text_argument is a file's whole body, which may be empty: the authorized
// keys once the last one is removed, or the file the first one is added to.
// It must still be sent, as a string.
fn text_argument<'a>(request: &'a Value, name: &str) -> Result<&'a str, Failure> {
    request
        .get("args")
        .and_then(|args| args.get(name))
        .and_then(Value::as_str)
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

    /// Every verb dispatch answers, so the grant below is checked against the
    /// whole surface rather than a remembered subset of it.
    const EVERY_METHOD: &[&str] = &[
        "accessCredentials",
        "setAuthorizedKeys",
        "setWebCertificate",
        "setPassword",
        "setSystemTime",
        "pkgStatus",
        "pkgUpdate",
        "pkgInstalled",
        "pkgUpgradable",
        "pkgUpgrade",
        "firmwareCheck",
        "firmwareUpgrade",
        "pkgSearch",
        "pkgBrowse",
        "pkgFiles",
        "pkgUpgradeOne",
        "pkgInstall",
        "pkgRemove",
        "rootHasPassword",
        "firewallCounters",
        "firewallStatus",
        "firewallLog",
        "createBackup",
        "restoreBackup",
        "validateFirmware",
        "installFirmware",
        "restart",
        "factoryReset",
    ];

    #[test]
    fn a_file_body_may_be_empty_but_must_be_sent() {
        // Removing the last authorized key writes an empty file, and adding the
        // first expects one: an empty body is a value, not a missing argument.
        let empty = json!({"args": {"keys": "", "expected": ""}});
        assert_eq!(text_argument(&empty, "keys").unwrap(), "");
        assert_eq!(text_argument(&empty, "expected").unwrap(), "");
        let absent = json!({"args": {}});
        assert!(text_argument(&absent, "keys").is_err());
        let wrong = json!({"args": {"keys": 1}});
        assert!(text_argument(&wrong, "keys").is_err());
        // every other argument still has to say something
        assert!(argument(&empty, "keys").is_err());
    }

    #[test]
    fn the_zero_session_grants_root_exactly_the_two_read_verbs() {
        for method in EVERY_METHOD {
            let reading = matches!(*method, "pkgUpgradable" | "firmwareCheck");
            assert_eq!(
                zero_session_grants(0, method),
                reading,
                "root's zero-session grant for {method}"
            );
            // The shell keeps using operator sessions; the zero sid buys uid 6000
            // nothing, not even the reads.
            assert!(
                !zero_session_grants(VERSO_UID, method),
                "the shell's account must not ride the zero session for {method}"
            );
        }
    }

    #[test]
    fn the_zero_session_refuses_a_write_verb_from_root() {
        // No ubus socket is involved: the zero sid short-circuits before any
        // session lookup, which is exactly what makes this bound the helper's own.
        let state = State {
            firewall_log: firewall_log::Collector::new(),
            packages: Mutex::new(()),
            maintenance: Mutex::new(()),
        };
        let request = json!({
            "method": "setPassword",
            "sid": ZERO_SID,
            "args": {"username": "root", "password": "hunter2"},
        });
        let failure = dispatch(&request, &state, 0).expect_err("denied");
        assert_eq!(failure.status, STATUS_PERMISSION_DENIED);
    }

    #[test]
    fn the_zero_session_refuses_a_read_verb_from_the_shell() {
        let state = State {
            firewall_log: firewall_log::Collector::new(),
            packages: Mutex::new(()),
            maintenance: Mutex::new(()),
        };
        let request = json!({"method": "pkgUpgradable", "sid": ZERO_SID});
        let failure = dispatch(&request, &state, VERSO_UID).expect_err("denied");
        assert_eq!(failure.status, STATUS_PERMISSION_DENIED);
    }

    // The upgrade tool's own files are the only ones this helper will flash from:
    // a regular file, owned by root, of a size the artifact could plausibly be.
    // Anything a compromised shell could have planted at those paths fails on
    // ownership (ADR-007).
    #[test]
    fn only_a_root_owned_regular_file_passes_as_the_upgrade_tools_output() {
        let image = |is_file, uid, len| plausible_artifact(is_file, uid, len, OWUT_MAX_IMAGE);
        assert!(image(true, 0, 42 * 1024 * 1024));
        // The shell's own account planted a file at that path: not owut's.
        assert!(!image(true, VERSO_UID, 42 * 1024 * 1024));
        // A symlink or a device node is not a file to flash from.
        assert!(!image(false, 0, 1024));
        // A truncated download and an image larger than the device could take
        // are both refused before sysupgrade ever sees them.
        assert!(!image(true, 0, 0));
        assert!(!image(true, 0, OWUT_MAX_IMAGE + 1));
    }

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
