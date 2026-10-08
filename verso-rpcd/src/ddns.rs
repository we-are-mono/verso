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
use std::process::{Command, Stdio};

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

/// update has one service's updater send its address now. Without a record of
/// a last update the updater sends at once, whether the address changed or
/// not, so the record goes before its instance alone restarts; the others keep
/// running. Only a service switched on in the applied config is started.
pub fn update(section: &str) -> Result<Value, Failure> {
    if !section_name(section) {
        return Err(Failure::invalid("Invalid service name."));
    }
    let get = |option: &str| {
        let key = format!("ddns.{section}{option}");
        Command::new("/sbin/uci")
            .args(["-q", "get", &key])
            .output()
            .map(|o| String::from_utf8_lossy(&o.stdout).trim().to_string())
            .unwrap_or_default()
    };
    if get("") != "service" || get(".enabled") != "1" {
        return Err(Failure::invalid("This name is not switched on."));
    }
    let _ = fs::remove_file(Path::new(RUN).join(format!("{section}.update")));
    for action in ["stop", "start"] {
        let done = Command::new("/etc/init.d/ddns")
            .args([action, section])
            .stdin(Stdio::null())
            .status()
            .is_ok_and(|s| s.success());
        if !done {
            return Err(Failure::unknown("ddns-scripts did not restart the updater."));
        }
    }
    Ok(json!({}))
}

/// section_name is a uci section name the updater's files can be named by.
fn section_name(name: &str) -> bool {
    !name.is_empty()
        && name.len() <= 64
        && name.bytes().all(|b| b.is_ascii_alphanumeric() || b == b'_')
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
    fn an_update_names_a_section_and_nothing_else() {
        assert!(section_name("home_example_com_v6"));
        for bad in ["", "../etc", "a.b", "a b", "a;reboot", &"x".repeat(65)] {
            assert!(!section_name(bad), "{bad}");
            assert!(update(bad).is_err(), "{bad}");
        }
    }

    #[test]
    fn a_pid_runs_only_as_an_updater() {
        assert!(!updater_runs("1/../self"));
        assert!(!updater_runs(&std::process::id().to_string()));
    }
}
