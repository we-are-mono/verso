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
use std::io::{Read, Write};

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
}
