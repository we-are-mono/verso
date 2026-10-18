// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

//! A plugin's root half (ADR-027): the same binary, started by root as
//! `verso-plugin-<id> root`. The privileged helper starts it for a registered
//! command, with one JSON request on stdin; the plugin's hotplug scripts and
//! init start it for unattended work under subcommands of the plugin's own.
//!
//! [`run`] answers the helper: it refuses unless the effective uid is root,
//! runs the handler the request names, and ends the process the way the
//! helper reads an outcome:
//!
//! | Exit | Stdout | The helper reads |
//! |---|---|---|
//! | 0 | the result, one JSON object | success |
//! | 3 | `{"refusal": "<words>"}` | a refusal the operator reads |
//! | anything else | — | a failure, logged and never shown |

use serde::Deserialize;
use serde_json::{json, Map, Value};
use std::collections::BTreeMap;
use std::fs;
use std::io::{Read, Write};
use std::os::unix::fs::{DirBuilderExt, OpenOptionsExt, PermissionsExt};
use std::path::{Path, PathBuf};

pub mod uci;

/// The exit code of a refusal.
pub const EXIT_REFUSED: i32 = 3;
/// The exit code of a failure: anything but success or a refusal.
pub const EXIT_FAILED: i32 = 1;

/// The longest request the helper sends.
const MAX_REQUEST: u64 = 1 << 20;

/// What the helper asks: the command's name, an act's checked arguments, and
/// a transform's input.
#[derive(Deserialize, Debug, Default, Clone, PartialEq)]
pub struct RootRequest {
    pub command: String,
    #[serde(default)]
    pub args: BTreeMap<String, String>,
    #[serde(default)]
    pub input: Value,
}

/// How a handler did not succeed.
#[derive(Debug, Clone, PartialEq)]
pub enum RootError {
    /// Words the operator reads, translated by the plugin's catalogue.
    Refusal(String),
    /// A fault, logged to stderr and never shown.
    Failure(String),
}

impl RootError {
    pub fn refusal(words: impl Into<String>) -> RootError {
        RootError::Refusal(words.into())
    }
    pub fn failure(detail: impl std::fmt::Display) -> RootError {
        RootError::Failure(detail.to_string())
    }
}

/// One command's handler.
pub type Handler<'a> = &'a dyn Fn(&RootRequest) -> Result<Map<String, Value>, RootError>;

/// What one dispatch came to: the exit code, stdout and stderr.
#[derive(Debug, PartialEq)]
pub struct Outcome {
    pub code: i32,
    pub stdout: Vec<u8>,
    pub stderr: String,
}

impl Outcome {
    fn failed(why: impl Into<String>) -> Outcome {
        Outcome { code: EXIT_FAILED, stdout: vec![], stderr: why.into() }
    }
}

/// dispatch answers one request for the effective uid `euid`, without touching
/// the process: [`run`] is this with the real uid and stdio.
pub fn dispatch(euid: u32, input: &[u8], handlers: &[(&str, Handler)]) -> Outcome {
    if euid != 0 {
        return Outcome::failed("the root half runs as root only");
    }
    let request: RootRequest = match serde_json::from_slice(input) {
        Ok(request) => request,
        Err(e) => return Outcome::failed(format!("unreadable request: {e}")),
    };
    let Some((_, handler)) = handlers.iter().find(|(name, _)| *name == request.command) else {
        return Outcome::failed(format!("unknown command {:?}", request.command));
    };
    let answer = std::panic::catch_unwind(std::panic::AssertUnwindSafe(|| handler(&request)));
    match answer {
        Ok(Ok(result)) => Outcome {
            code: 0,
            stdout: serde_json::to_vec(&Value::Object(result)).unwrap_or_default(),
            stderr: String::new(),
        },
        Ok(Err(RootError::Refusal(words))) => Outcome {
            code: EXIT_REFUSED,
            stdout: serde_json::to_vec(&json!({ "refusal": words })).unwrap_or_default(),
            stderr: String::new(),
        },
        Ok(Err(RootError::Failure(detail))) => Outcome::failed(detail),
        Err(_) => Outcome::failed(format!("{:?} panicked", request.command)),
    }
}

/// effective_uid is the process's effective uid, read from the kernel's own
/// account of it; None when that cannot be read.
pub fn effective_uid() -> Option<u32> {
    let status = std::fs::read_to_string("/proc/self/status").ok()?;
    let line = status.lines().find(|l| l.starts_with("Uid:"))?;
    line.split_whitespace().nth(2)?.parse().ok()
}

/// is_root reports whether the process runs as root: a plugin's hotplug and
/// init subcommands check it before they act.
pub fn is_root() -> bool {
    effective_uid() == Some(0)
}

/// Where a root half marks a commit it is about to make as the router's own
/// act, so config history records it as the router's, with its cause, rather
/// than as edited directly (ADR-021, ADR-027). The helper reads and removes
/// the mark.
pub const OWN_MARKS: &str = "/tmp/run/verso-own";

/// The longest cause a mark carries.
const CAUSE_LIMIT: usize = 512;

/// own_write marks config as about to be committed by this root half: call it
/// just before `uci commit`. A mark counts for one commit, within 30 seconds.
/// cause is one English sentence saying what made the commit ("A certificate
/// was installed by hand."); config history shows it translated from plugin's
/// own catalog (ADR-012), so it is written as a key of that catalog.
///
/// ```no_run
/// // ACME's hand-over turns its renewal off, as the router's own act.
/// verso_plugin::root::own_write("acme", "acme", "A certificate was installed by hand.")?;
/// std::process::Command::new("/sbin/uci").args(["commit", "acme"]).status().map_err(|e| e.to_string())?;
/// # Ok::<(), String>(())
/// ```
pub fn own_write(plugin: &str, config: &str, cause: &str) -> Result<(), String> {
    own_write_in(Path::new(OWN_MARKS), plugin, config, cause)
}

fn own_write_in(dir: &Path, plugin: &str, config: &str, cause: &str) -> Result<(), String> {
    if !uci::config_name(config) {
        return Err(format!("{config:?} is not a uci config name"));
    }
    if plugin.is_empty()
        || plugin.len() > 64
        || !plugin.bytes().all(|b| b.is_ascii_lowercase() || b.is_ascii_digit() || b == b'-' || b == b'_')
    {
        return Err(format!("{plugin:?} is not a plugin id"));
    }
    if cause.is_empty() || cause.len() > CAUSE_LIMIT || cause.contains(['\n', '\r']) {
        return Err("a cause is one line of at most 512 bytes".into());
    }
    let body = serde_json::to_vec(&json!({"plugin": plugin, "cause": cause}))
        .map_err(|e| e.to_string())?;
    match fs::DirBuilder::new().mode(0o700).create(dir) {
        Ok(()) => {}
        Err(e) if e.kind() == std::io::ErrorKind::AlreadyExists => {}
        Err(e) => return Err(format!("{}: {e}", dir.display())),
    }
    fs::set_permissions(dir, fs::Permissions::from_mode(0o700)).map_err(|e| e.to_string())?;
    let mark = dir.join(config);
    let _ = fs::remove_file(&mark);
    fs::OpenOptions::new()
        .write(true)
        .create_new(true)
        .mode(0o600)
        .open(&mark)
        .and_then(|mut f| f.write_all(&body))
        .map_err(|e| format!("{}: {e}", mark.display()))
}

/// signal_shell tells the running shell, through procd, to read what changed
/// again — a new certificate pair on disk, for one (ADR-017 §4).
pub fn signal_shell() -> Result<(), String> {
    let status = std::process::Command::new("/bin/ubus")
        .args(["call", "service", "signal", r#"{"name":"verso","signal":1}"#])
        .stdin(std::process::Stdio::null())
        .status()
        .map_err(|e| e.to_string())?;
    if !status.success() {
        return Err("the web interface could not be told".into());
    }
    Ok(())
}

/// write_new writes value beside path under a private name, its mode set
/// before a byte is written, and answers that name for the caller to rename
/// into place: the file is never readable by anyone it is not meant for.
pub fn write_new(path: &Path, value: &[u8], mode: u32) -> Result<PathBuf, String> {
    let mut name = path.file_name().ok_or("no file name")?.to_os_string();
    name.push(format!(".verso-{}", std::process::id()));
    let tmp = path.with_file_name(name);
    let mut f = fs::OpenOptions::new()
        .write(true)
        .create_new(true)
        .mode(mode & 0o600)
        .open(&tmp)
        .map_err(|e| format!("{}: {e}", tmp.display()))?;
    let written = f
        .set_permissions(fs::Permissions::from_mode(mode))
        .and_then(|_| f.write_all(value))
        .and_then(|_| f.sync_all());
    if let Err(e) = written {
        let _ = fs::remove_file(&tmp);
        return Err(e.to_string());
    }
    Ok(tmp)
}

/// run answers the helper's request on stdin and ends the process.
pub fn run(handlers: &[(&str, Handler)]) -> ! {
    let mut input = Vec::new();
    let read = std::io::stdin().take(MAX_REQUEST).read_to_end(&mut input);
    let outcome = match (read, effective_uid()) {
        (Err(e), _) => Outcome::failed(format!("reading the request: {e}")),
        (_, None) => Outcome::failed("the effective uid could not be read"),
        (Ok(_), Some(euid)) => dispatch(euid, &input, handlers),
    };
    if !outcome.stderr.is_empty() {
        eprintln!("{}", outcome.stderr);
    }
    let mut out = std::io::stdout();
    let _ = out.write_all(&outcome.stdout);
    let _ = out.flush();
    std::process::exit(outcome.code)
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::cell::Cell;

    fn state(_: &RootRequest) -> Result<Map<String, Value>, RootError> {
        let mut m = Map::new();
        m.insert("up".into(), json!(true));
        Ok(m)
    }

    #[test]
    fn a_non_root_caller_runs_nothing() {
        let called = Cell::new(false);
        let handler = |_: &RootRequest| {
            called.set(true);
            Ok(Map::new())
        };
        let out = dispatch(6013, br#"{"command":"state"}"#, &[("state", &handler)]);
        assert_eq!(out.code, EXIT_FAILED);
        assert!(out.stdout.is_empty());
        assert!(!called.get());
    }

    #[test]
    fn success_answers_the_object() {
        let out = dispatch(0, br#"{"command":"state"}"#, &[("state", &state)]);
        assert_eq!(out.code, 0);
        assert_eq!(serde_json::from_slice::<Value>(&out.stdout).unwrap(), json!({"up": true}));
    }

    #[test]
    fn a_refusal_exits_three_with_its_words() {
        let refuse = |_: &RootRequest| Err(RootError::refusal("That section does not exist."));
        let out = dispatch(0, br#"{"command":"update","args":{"section":"x"}}"#, &[("update", &refuse)]);
        assert_eq!(out.code, EXIT_REFUSED);
        assert_eq!(
            serde_json::from_slice::<Value>(&out.stdout).unwrap(),
            json!({"refusal": "That section does not exist."})
        );
    }

    #[test]
    fn a_failure_is_logged_not_answered() {
        let fail = |_: &RootRequest| Err(RootError::failure("ubus timed out"));
        let out = dispatch(0, br#"{"command":"state"}"#, &[("state", &fail)]);
        assert_eq!(out.code, EXIT_FAILED);
        assert!(out.stdout.is_empty());
        assert_eq!(out.stderr, "ubus timed out");
    }

    #[test]
    fn a_panic_is_a_failure() {
        let panics = |_: &RootRequest| -> Result<Map<String, Value>, RootError> { panic!("boom") };
        let out = dispatch(0, br#"{"command":"state"}"#, &[("state", &panics)]);
        assert_eq!(out.code, EXIT_FAILED);
        assert!(out.stdout.is_empty());
    }

    #[test]
    fn an_unknown_command_is_a_failure() {
        let out = dispatch(0, br#"{"command":"reboot"}"#, &[("state", &state)]);
        assert_eq!(out.code, EXIT_FAILED);
        assert!(out.stderr.contains("reboot"));
    }

    #[test]
    fn an_unreadable_request_is_a_failure() {
        assert_eq!(dispatch(0, b"state", &[("state", &state)]).code, EXIT_FAILED);
    }

    #[test]
    fn the_handler_gets_args_and_input() {
        let echo = |r: &RootRequest| {
            let mut m = Map::new();
            m.insert("section".into(), json!(r.args["section"]));
            m.insert("op".into(), r.input["op"].clone());
            Ok(m)
        };
        let out = dispatch(
            0,
            br#"{"command":"files","args":{"section":"home"},"input":{"op":"list"}}"#,
            &[("files", &echo)],
        );
        assert_eq!(
            serde_json::from_slice::<Value>(&out.stdout).unwrap(),
            json!({"section": "home", "op": "list"})
        );
    }

    #[test]
    fn the_effective_uid_is_read() {
        assert!(effective_uid().is_some());
    }

    // A mark is one private file per config, saying whose words its cause is
    // and the cause itself, for config history to show (ADR-021).
    #[test]
    fn a_mark_is_one_private_file_per_config_with_its_cause() {
        let dir = std::env::temp_dir().join(format!("verso-sdk-marks-{}", std::process::id()));
        let _ = fs::remove_dir_all(&dir);
        let cause = "A certificate was installed by hand.";
        own_write_in(&dir, "acme", "acme", cause).unwrap();
        own_write_in(&dir, "acme", "acme", cause).unwrap();
        assert_eq!(fs::metadata(&dir).unwrap().permissions().mode() & 0o777, 0o700);
        let mark = dir.join("acme");
        assert_eq!(fs::metadata(&mark).unwrap().permissions().mode() & 0o777, 0o600);
        let written: Value = serde_json::from_slice(&fs::read(&mark).unwrap()).unwrap();
        assert_eq!(written, json!({"plugin": "acme", "cause": cause}));
        assert!(own_write_in(&dir, "acme", "../shadow", cause).is_err(), "a config that is no uci name");
        assert!(own_write_in(&dir, "Acme!", "acme", cause).is_err(), "a plugin that is no plugin id");
        assert!(own_write_in(&dir, "acme", "acme", "two\nlines").is_err(), "a cause of more than one line");
        assert!(own_write_in(&dir, "acme", "acme", &"x".repeat(513)).is_err(), "a cause over 512 bytes");
        assert!(own_write_in(&dir, "acme", "acme", "").is_err(), "no cause");
        let _ = fs::remove_dir_all(&dir);
    }

    #[test]
    fn a_new_file_is_private_until_it_is_whole() {
        let dir = std::env::temp_dir().join(format!("verso-sdk-write-{}", std::process::id()));
        fs::create_dir_all(&dir).unwrap();
        let tmp = write_new(&dir.join("record"), b"event=renewed\n", 0o644).unwrap();
        assert_eq!(fs::read(&tmp).unwrap(), b"event=renewed\n");
        assert_eq!(fs::metadata(&tmp).unwrap().permissions().mode() & 0o777, 0o644);
        assert!(write_new(&dir.join("record"), b"again", 0o644).is_err(), "a name in use is never reused");
        let _ = fs::remove_dir_all(&dir);
    }
}
