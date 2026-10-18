// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.
//! `pluginCommand` (ADR-027): a plugin's registered root command, run for a
//! session that holds the plugin's grant. The command runs as root, directly
//! (never through a shell), with `PATH` alone in its environment, in `/`, its
//! request on stdin, bounded in time and output. Its outcome is one of three:
//! a result, a refusal the operator reads, or a failure that is only logged.

use crate::registry::{self, Kind, Registry};
use crate::{args, Failure};
use serde_json::{json, Map, Value};
use std::path::PathBuf;
use std::time::Duration;

const PATH: &str = "/usr/sbin:/usr/bin:/sbin:/bin";
pub const MAX_OUTPUT: usize = 1 << 20;
const EXIT_REFUSED: i32 = 3;

/// One run of a command's executable.
#[derive(Debug, Clone, PartialEq)]
pub struct Run {
    pub exec: PathBuf,
    pub argv: Vec<String>,
    pub env: Vec<(String, String)>,
    pub dir: PathBuf,
    pub stdin: Vec<u8>,
    pub timeout: Duration,
}

/// How a run ended: its exit code (none when a signal ended it) and output.
#[derive(Debug, Clone, PartialEq)]
pub struct Ran {
    pub code: Option<i32>,
    pub stdout: Vec<u8>,
    pub stderr: Vec<u8>,
}

/// Runner starts a command and waits for it, within its timeout and the
/// output cap; an Err is a run that did not end on its own.
pub trait Runner {
    fn run(&self, run: &Run) -> Result<Ran, String>;
}

/// The real runner: the helper's bounded child runner.
pub struct System;

impl Runner for System {
    fn run(&self, run: &Run) -> Result<Ran, String> {
        let mut command = std::process::Command::new(&run.exec);
        command.args(&run.argv).current_dir(&run.dir);
        let name = run.exec.display().to_string();
        let mut output =
            crate::run_bounded_with_stdin(command, &name, MAX_OUTPUT, run.timeout, Some(run.stdin.clone()), &run.env)
                .map_err(|f| f.message)?;
        Ok(Ran {
            code: output.status.code(),
            stdout: std::mem::take(&mut output.stdout),
            stderr: std::mem::take(&mut output.stderr),
        })
    }
}

/// run_command runs one already-loaded command with `input` as its request's
/// input, and reads its outcome. Transforms are run this way by the helper
/// itself; sessions reach commands only through [`for_session`].
pub fn run_command(
    runner: &impl Runner,
    plugin: &str,
    command: &registry::Command,
    checked: &std::collections::BTreeMap<String, String>,
    input: Value,
) -> Result<Value, Failure> {
    let mut request = json!({ "command": command.name, "args": checked });
    if !input.is_null() {
        request["input"] = input;
    }
    let run = Run {
        exec: PathBuf::from(&command.exec),
        argv: command.argv.clone(),
        env: vec![("PATH".into(), PATH.into())],
        dir: PathBuf::from("/"),
        stdin: serde_json::to_vec(&request).unwrap_or_default(),
        timeout: Duration::from_secs(u64::from(command.timeout())),
    };
    let at = format!("{plugin}.{}", command.name);
    let ran = runner.run(&run).map_err(|why| {
        eprintln!("verso-rpcd: {at}: {why}");
        Failure::unknown(format!("{at} did not complete"))
    })?;
    outcome(&at, ran)
}

/// outcome reads how a command ended, as the SDK's root dispatcher ends it.
fn outcome(at: &str, ran: Ran) -> Result<Value, Failure> {
    if !ran.stderr.is_empty() {
        eprintln!("verso-rpcd: {at}: {}", String::from_utf8_lossy(&ran.stderr).trim_end());
    }
    let object = || match serde_json::from_slice::<Value>(&ran.stdout) {
        Ok(Value::Object(map)) => Some(map),
        _ => None,
    };
    match ran.code {
        Some(0) => match object() {
            Some(map) => Ok(Value::Object(map)),
            None => {
                eprintln!("verso-rpcd: {at}: the output is not one JSON object");
                Err(Failure::unknown(format!("{at} answered nothing readable")))
            }
        },
        Some(EXIT_REFUSED) => match object().as_ref().and_then(|m| m.get("refusal")).and_then(Value::as_str) {
            Some(words) if !words.is_empty() => Err(Failure::invalid(words)),
            _ => {
                eprintln!("verso-rpcd: {at}: a refusal without words");
                Err(Failure::unknown(format!("{at} did not complete")))
            }
        },
        code => {
            eprintln!("verso-rpcd: {at}: ended with {code:?}");
            Err(Failure::unknown(format!("{at} did not complete")))
        }
    }
}

/// Access asks rpcd whether the session holds a grant.
pub trait Access {
    fn allows(&self, sid: &str, scope: &str, object: &str, function: &str) -> Result<bool, String>;
}

/// rpcd itself.
pub struct Rpcd;

impl Access for Rpcd {
    fn allows(&self, sid: &str, scope: &str, object: &str, function: &str) -> Result<bool, String> {
        crate::ubus::access_to(sid, scope, object, function).map_err(|e| e.to_string())
    }
}

/// for_session runs `plugin`'s command `name` for a session: only under the
/// grant `verso-plugin <plugin> <name>`, only a read or an act, and only with
/// arguments its registry allows.
pub fn for_session(
    access: &impl Access,
    load: impl Fn(&str) -> Result<Registry, registry::Refused>,
    runner: &impl Runner,
    sid: &str,
    request: &Map<String, Value>,
) -> Result<Value, Failure> {
    let text = |key: &str| request.get(key).and_then(Value::as_str).unwrap_or_default();
    let (plugin, name) = (text("plugin"), text("name"));
    if !registry::valid_plugin(plugin) || !registry::valid_name(name) {
        return Err(Failure::invalid("A plugin command needs a plugin and a name."));
    }
    match access.allows(sid, "verso-plugin", plugin, name) {
        Ok(true) => {}
        Ok(false) => return Err(crate::denied()),
        Err(why) => {
            eprintln!("verso-rpcd: authorization failed for {plugin}.{name}: {why}");
            return Err(crate::denied());
        }
    }
    let registry = load(plugin).map_err(|refused| match refused {
        registry::Refused::Absent => not_found(plugin, name),
        registry::Refused::Unusable(why) => {
            eprintln!("verso-rpcd: {plugin}: registry refused: {why}");
            Failure::unknown(format!("{plugin}'s commands are unavailable"))
        }
    })?;
    let command = registry
        .command(name)
        .filter(|c| c.kind != Kind::Transform)
        .ok_or_else(|| not_found(plugin, name))?;
    let checked = args::check(&command.args, request.get("args").unwrap_or(&Value::Null))
        .map_err(Failure::invalid)?;
    run_command(runner, plugin, command, &checked, Value::Null)
}

fn not_found(plugin: &str, name: &str) -> Failure {
    Failure {
        status: crate::STATUS_METHOD_NOT_FOUND,
        message: format!("{plugin} registers no command {name:?}"),
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::cell::RefCell;
    use std::collections::BTreeMap;

    struct Grants(Vec<(&'static str, &'static str)>);

    impl Access for Grants {
        fn allows(&self, _: &str, scope: &str, object: &str, function: &str) -> Result<bool, String> {
            assert_eq!(scope, "verso-plugin");
            Ok(self.0.contains(&(object, function)))
        }
    }

    struct Fake {
        answer: Result<Ran, String>,
        runs: RefCell<Vec<Run>>,
    }

    impl Fake {
        fn answering(code: Option<i32>, stdout: &str) -> Fake {
            Fake {
                answer: Ok(Ran { code, stdout: stdout.as_bytes().to_vec(), stderr: vec![] }),
                runs: RefCell::new(vec![]),
            }
        }
    }

    impl Runner for Fake {
        fn run(&self, run: &Run) -> Result<Ran, String> {
            self.runs.borrow_mut().push(run.clone());
            self.answer.clone()
        }
    }

    fn ddns(plugin: &str) -> Result<Registry, registry::Refused> {
        if plugin != "ddns" {
            return Err(registry::Refused::Absent);
        }
        Ok(serde_json::from_value(json!({"schema_version": 1, "commands": [
            {"name": "state", "kind": "read", "version": 1, "exec": "/usr/bin/verso-plugin-ddns", "argv": ["root"]},
            {"name": "update", "kind": "act", "exec": "/usr/bin/verso-plugin-ddns", "argv": ["root"], "timeout": 30,
             "args": {"section": {"kind": "uci-name"}}},
            {"name": "files", "kind": "transform", "exec": "/usr/bin/verso-plugin-ddns", "argv": ["root"]}
        ]}))
        .unwrap())
    }

    fn ask(grants: &Grants, runner: &Fake, request: Value) -> Result<Value, Failure> {
        for_session(grants, ddns, runner, "sid", request.as_object().unwrap())
    }

    fn all() -> Grants {
        Grants(vec![("ddns", "state"), ("ddns", "update"), ("ddns", "files"), ("vpn", "state")])
    }

    #[test]
    fn the_child_gets_exec_argv_path_root_dir_and_its_request() {
        let runner = Fake::answering(Some(0), r#"{"done":true}"#);
        let out = ask(&all(), &runner, json!({"plugin":"ddns","name":"update","args":{"section":"home"}})).unwrap();
        assert_eq!(out, json!({"done": true}));
        let run = runner.runs.borrow()[0].clone();
        assert_eq!(run.exec, PathBuf::from("/usr/bin/verso-plugin-ddns"));
        assert_eq!(run.argv, ["root"]);
        assert_eq!(run.env, [("PATH".to_string(), PATH.to_string())]);
        assert_eq!(run.dir, PathBuf::from("/"));
        assert_eq!(run.timeout, Duration::from_secs(30));
        let request: Value = serde_json::from_slice(&run.stdin).unwrap();
        assert_eq!(request, json!({"command": "update", "args": {"section": "home"}}));
    }

    #[test]
    fn a_read_runs_on_the_default_timeout() {
        let runner = Fake::answering(Some(0), "{}");
        ask(&all(), &runner, json!({"plugin":"ddns","name":"state"})).unwrap();
        assert_eq!(runner.runs.borrow()[0].timeout, Duration::from_secs(10));
    }

    #[test]
    fn without_the_grant_nothing_runs() {
        let runner = Fake::answering(Some(0), "{}");
        let err = ask(&Grants(vec![("ddns", "state")]), &runner, json!({"plugin":"ddns","name":"update","args":{"section":"home"}})).unwrap_err();
        assert_eq!(err.status, crate::STATUS_PERMISSION_DENIED);
        assert!(runner.runs.borrow().is_empty());
    }

    #[test]
    fn a_transform_is_not_reachable_through_the_verb() {
        let runner = Fake::answering(Some(0), "{}");
        let err = ask(&all(), &runner, json!({"plugin":"ddns","name":"files"})).unwrap_err();
        assert_eq!(err.status, crate::STATUS_METHOD_NOT_FOUND);
        assert!(runner.runs.borrow().is_empty());
    }

    #[test]
    fn an_unknown_plugin_or_command_is_not_found() {
        let runner = Fake::answering(Some(0), "{}");
        assert_eq!(ask(&all(), &runner, json!({"plugin":"vpn","name":"state"})).unwrap_err().status, crate::STATUS_METHOD_NOT_FOUND);
        let grants = Grants(vec![("ddns", "reboot")]);
        assert_eq!(ask(&grants, &runner, json!({"plugin":"ddns","name":"reboot"})).unwrap_err().status, crate::STATUS_METHOD_NOT_FOUND);
        assert!(runner.runs.borrow().is_empty());
    }

    #[test]
    fn bad_arguments_run_nothing() {
        let runner = Fake::answering(Some(0), "{}");
        for args in [json!({"section":"home; reboot"}), json!({}), json!({"section":"home","x":"1"})] {
            let err = ask(&all(), &runner, json!({"plugin":"ddns","name":"update","args":args})).unwrap_err();
            assert_eq!(err.status, crate::STATUS_INVALID_ARGUMENT);
        }
        let err = ask(&all(), &runner, json!({"plugin":"ddns","name":"state","args":{"x":"1"}})).unwrap_err();
        assert_eq!(err.status, crate::STATUS_INVALID_ARGUMENT);
        assert!(runner.runs.borrow().is_empty());
    }

    #[test]
    fn a_refusal_carries_its_words() {
        let runner = Fake::answering(Some(3), r#"{"refusal":"That section does not exist."}"#);
        let err = ask(&all(), &runner, json!({"plugin":"ddns","name":"update","args":{"section":"home"}})).unwrap_err();
        assert_eq!(err.status, crate::STATUS_INVALID_ARGUMENT);
        assert_eq!(err.message, "That section does not exist.");
    }

    #[test]
    fn every_other_end_is_a_failure_that_shows_no_detail() {
        for (code, stdout) in [(Some(1), ""), (Some(101), "panicked at"), (None, ""), (Some(0), "[1,2]"), (Some(0), "nope"), (Some(3), "{}")] {
            let runner = Fake::answering(code, stdout);
            let err = ask(&all(), &runner, json!({"plugin":"ddns","name":"state"})).unwrap_err();
            assert_eq!(err.status, crate::STATUS_UNKNOWN_ERROR, "{code:?} {stdout}");
            assert!(!err.message.contains("panicked"));
        }
    }

    #[test]
    fn a_run_that_did_not_end_is_a_failure() {
        let runner = Fake { answer: Err("timed out".into()), runs: RefCell::new(vec![]) };
        let err = ask(&all(), &runner, json!({"plugin":"ddns","name":"state"})).unwrap_err();
        assert_eq!(err.status, crate::STATUS_UNKNOWN_ERROR);
    }

    #[test]
    fn a_transform_gets_its_input() {
        let runner = Fake::answering(Some(0), "{}");
        let registry = ddns("ddns").unwrap();
        run_command(&runner, "ddns", registry.command("files").unwrap(), &BTreeMap::new(), json!({"op": "list"})).unwrap();
        let request: Value = serde_json::from_slice(&runner.runs.borrow()[0].stdin).unwrap();
        assert_eq!(request["input"], json!({"op": "list"}));
    }

    #[test]
    fn the_real_runner_stops_a_hang_and_its_children() {
        let run = Run {
            exec: PathBuf::from("/bin/sh"),
            argv: vec!["-c".into(), "sleep 30 & sleep 30".into()],
            env: vec![("PATH".into(), PATH.into())],
            dir: PathBuf::from("/"),
            stdin: vec![],
            timeout: Duration::from_millis(300),
        };
        let started = std::time::Instant::now();
        assert!(System.run(&run).is_err());
        assert!(started.elapsed() < Duration::from_secs(5));
    }

    #[test]
    fn the_real_runner_refuses_oversized_output_and_passes_stdin() {
        let big = Run {
            exec: PathBuf::from("/bin/sh"),
            argv: vec!["-c".into(), "head -c 1048577 /dev/zero".into()],
            env: vec![("PATH".into(), PATH.into())],
            dir: PathBuf::from("/"),
            stdin: vec![],
            timeout: Duration::from_secs(10),
        };
        assert!(System.run(&big).is_err());
        let echo = Run { argv: vec!["-c".into(), "cat; echo \"$PATH\" >&2; pwd >&2".into()], stdin: b"{\"a\":1}".to_vec(), ..big };
        let ran = System.run(&echo).unwrap();
        assert_eq!(ran.stdout, b"{\"a\":1}");
        assert_eq!(String::from_utf8_lossy(&ran.stderr), format!("{PATH}\n/\n"));
    }
}
