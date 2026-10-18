// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.
//! The announcement of a certificate installed or made by hand (ADR-017 §6,
//! ADR-027): `hotplug-call verso-certificate` with `ACTION=installed`, run as
//! root once the pair is written. Whatever owns another source of the
//! certificate answers it from its own hotplug script, turning that source off
//! so no renewal replaces the operator's choice. Core names no such source.

use crate::plugin_command::{Run, Runner};
use std::path::PathBuf;
use std::time::Duration;

const HOTPLUG_CALL: &str = "/sbin/hotplug-call";
const SUBSYSTEM: &str = "verso-certificate";
/// The scripts answering it commit a config; this bounds a stalled one.
const WAIT: Duration = Duration::from_secs(30);

/// installed announces a hand-installed certificate and waits for the
/// scripts answering it, so they have acted before the install answers. The
/// pair is already served, so a script that fails is logged and the install
/// stands.
pub fn installed(runner: &impl Runner) {
    let run = Run {
        exec: PathBuf::from(HOTPLUG_CALL),
        argv: vec![SUBSYSTEM.into()],
        env: vec![
            ("PATH".into(), "/usr/sbin:/usr/bin:/sbin:/bin".into()),
            ("ACTION".into(), "installed".into()),
        ],
        dir: PathBuf::from("/"),
        stdin: Vec::new(),
        timeout: WAIT,
    };
    match runner.run(&run) {
        Ok(ran) if ran.code == Some(0) => {}
        Ok(ran) => eprintln!(
            "verso-rpcd: {SUBSYSTEM} installed ended with {:?}: {}",
            ran.code,
            String::from_utf8_lossy(&ran.stderr).trim_end()
        ),
        Err(why) => eprintln!("verso-rpcd: {SUBSYSTEM} installed: {why}"),
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::plugin_command::Ran;
    use std::cell::RefCell;

    struct Recorder {
        answer: Result<Ran, String>,
        runs: RefCell<Vec<Run>>,
    }

    impl Runner for Recorder {
        fn run(&self, run: &Run) -> Result<Ran, String> {
            self.runs.borrow_mut().push(run.clone());
            self.answer.clone()
        }
    }

    #[test]
    fn a_hand_install_is_announced_and_nothing_else_runs() {
        let r = Recorder { answer: Ok(Ran { code: Some(0), stdout: vec![], stderr: vec![] }), runs: RefCell::new(vec![]) };
        installed(&r);
        let runs = r.runs.borrow();
        assert_eq!(runs.len(), 1, "one announcement, and no uci call of core's own");
        assert_eq!(runs[0].exec, PathBuf::from("/sbin/hotplug-call"));
        assert_eq!(runs[0].argv, ["verso-certificate"]);
        assert!(runs[0].env.contains(&("ACTION".into(), "installed".into())));
    }

    #[test]
    fn a_failed_announcement_leaves_the_install_standing() {
        for answer in [Err("timed out".to_string()), Ok(Ran { code: Some(1), stdout: vec![], stderr: b"uci: denied".to_vec() })] {
            let r = Recorder { answer, runs: RefCell::new(vec![]) };
            installed(&r);
            assert_eq!(r.runs.borrow().len(), 1);
        }
    }
}
