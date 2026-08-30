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
use std::os::unix::fs::PermissionsExt;
use std::os::unix::net::{UnixListener, UnixStream};
use std::path::{Path, PathBuf};
use std::process::{Command, Stdio};
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
    });
    for connection in listener.incoming() {
        match connection {
            Ok(stream) => {
                let state = Arc::clone(&state);
                std::thread::spawn(move || {
                    if let Err(error) = handle(stream, &state) {
                        eprintln!("verso-rpcd: request: {error}");
                    }
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
        _ => Err(Failure {
            status: STATUS_METHOD_NOT_FOUND,
            message: format!("unknown method {method}"),
        }),
    }
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
    if !valid_username(username) || password.len() > 1024 {
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
}
