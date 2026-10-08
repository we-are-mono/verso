// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.
//! What ddns-scripts last did for each service, for the Dynamic DNS page: the
//! address the name resolves to, how long ago an update was last sent, how soon
//! it checks again, and whether its updater runs. Read from the files the
//! updater keeps in its run folder, never from its log.
use crate::Failure;
use serde_json::{json, Map, Value};
use std::fs;
use std::path::Path;

/// The updater's default `ddns_rundir`; Verso never moves it.
const RUN: &str = "/var/run/ddns";

pub fn state() -> Result<Value, Failure> {
    let uptime = fs::read_to_string("/proc/uptime")
        .ok()
        .and_then(|u| u.split('.').next().and_then(|s| s.parse().ok()))
        .unwrap_or(0);
    Ok(json!({ "services": services(Path::new(RUN), uptime, updater_runs) }))
}

/// services is each section the run folder holds files for. The updater writes
/// its times as the router's uptime, so they are given back as seconds since
/// the last update and seconds until the next check.
fn services(dir: &Path, uptime: u64, runs: impl Fn(&str) -> bool) -> Map<String, Value> {
    let mut out = Map::new();
    let names = fs::read_dir(dir)
        .into_iter()
        .flatten()
        .flatten()
        .filter_map(|e| e.file_name().to_str()?.split_once('.').map(|(n, _)| n.to_string()))
        .filter(|n| !n.is_empty() && n.bytes().all(|b| b.is_ascii_alphanumeric() || b == b'_'));
    for name in names {
        if out.contains_key(&name) {
            continue;
        }
        let read = |ext: &str| {
            let path = dir.join(format!("{name}.{ext}"));
            fs::symlink_metadata(&path).ok().filter(|m| m.is_file() && m.len() < 256)?;
            Some(fs::read_to_string(path).ok()?.trim().to_string())
        };
        let number = |ext: &str| read(ext).and_then(|t| t.parse::<u64>().ok());
        let mut service = Map::new();
        service.insert("address".into(), json!(read("ip").unwrap_or_default()));
        service.insert("running".into(), json!(read("pid").is_some_and(|p| runs(&p))));
        if let Some(at) = number("update").filter(|at| *at > 0 && *at <= uptime) {
            service.insert("updated".into(), json!(uptime - at));
        }
        if let Some(at) = number("nextcheck") {
            service.insert("next".into(), json!(at.saturating_sub(uptime)));
        }
        out.insert(name, Value::Object(service));
    }
    out
}

/// updater_runs is whether a pid file still names a live updater, not a
/// process that took its number after it exited.
fn updater_runs(pid: &str) -> bool {
    pid.bytes().all(|b| b.is_ascii_digit())
        && fs::read(format!("/proc/{pid}/cmdline"))
            .is_ok_and(|c| String::from_utf8_lossy(&c).contains("dynamic_dns_updater"))
}

#[cfg(test)]
mod tests {
    use super::*;

    fn dir(name: &str, files: &[(&str, &str)]) -> std::path::PathBuf {
        let dir = std::env::temp_dir().join(format!("verso-ddns-{name}-{}", std::process::id()));
        let _ = fs::remove_dir_all(&dir);
        fs::create_dir_all(&dir).expect("dir");
        for (file, text) in files {
            fs::write(dir.join(file), text).expect("file");
        }
        dir
    }

    #[test]
    fn a_service_reads_out_in_seconds_from_now() {
        let dir = dir(
            "times",
            &[
                ("home.pid", "812\n"),
                ("home.ip", "203.0.113.7\n"),
                ("home.update", "1000\n"),
                ("home.nextcheck", "1300\n"),
                ("home.dat", "good 203.0.113.7"),
            ],
        );
        let got = services(&dir, 1200, |pid| pid == "812");
        assert_eq!(
            Value::Object(got),
            json!({"home": {"address": "203.0.113.7", "running": true, "updated": 200, "next": 100}})
        );
    }

    #[test]
    fn a_service_never_updated_or_stopped_says_only_that() {
        let dir = dir("fresh", &[("work.ip", "\n"), ("work.pid", "9\n"), ("work.update", "0\n")]);
        let got = services(&dir, 50, |_| false);
        assert_eq!(Value::Object(got), json!({"work": {"address": "", "running": false}}));
    }

    #[test]
    fn a_missing_folder_is_no_services() {
        let got = services(Path::new("/nonexistent/verso-ddns"), 10, |_| true);
        assert!(got.is_empty());
    }

    #[test]
    fn a_pid_runs_only_as_an_updater() {
        assert!(!updater_runs("1/../self"));
        assert!(!updater_runs(&std::process::id().to_string()));
    }
}
