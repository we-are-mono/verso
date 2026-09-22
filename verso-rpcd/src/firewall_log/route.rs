// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

//! fw4 has no UCI NFLOG group option. Augment its log transport at generation
//! time so a reload never temporarily installs managed kernel log statements.
//! Migrate existing rules in one nft transaction, without changing decisions.

use super::{GROUP, SNAPLEN};
use crate::{command_failure, run_bounded_with_stdin, run_bounded_within, Failure};
use serde_json::{json, Value};
use std::io::Write;
use std::process::{Command, Stdio};

const LOG: &str = "log prefix ";
const NFLOG: &str = "log group 4242 snaplen 256 queue-threshold 1 prefix ";
const TEMPLATES: &[&str] = &[
    "rule.uc",
    "redirect.uc",
    "snat.uc",
    "mangle-rule.uc",
    "ruleset.uc",
    "zone-verdict.uc",
    "zone-drop-invalid.uc",
    "zone-mssfix.uc",
];

pub(super) fn template(source: &str, enable: bool) -> String {
    if enable {
        source.replace(LOG, NFLOG)
    } else {
        source.replace(NFLOG, LOG)
    }
}

// This deliberately changes only the transport of upstream's log expressions.
// Keep no fork of the templates: upstream fixes and local edits are retained.
// Prepare every file before replacing any, and reject unexpected required
// templates so unsupported fw4 changes are reported instead of claimed working.
pub(super) fn configure(enable: bool) -> Result<(), Failure> {
    use std::os::unix::fs::{MetadataExt, OpenOptionsExt};
    if enable {
        super::netlink::ensure_supported()
            .map_err(|e| Failure::unknown(format!("NFLOG support is required: {e}")))?;
    }
    let root = std::path::Path::new("/usr/share/firewall4/templates");
    let mut changes = Vec::new();
    for name in TEMPLATES {
        let path = root.join(name);
        let metadata = match std::fs::symlink_metadata(&path) {
            Ok(metadata) => metadata,
            Err(e)
                if e.kind() == std::io::ErrorKind::NotFound
                    && !matches!(*name, "rule.uc" | "zone-verdict.uc") =>
            {
                continue
            }
            Err(e) => return Err(Failure::unknown(format!("read fw4 template {name}: {e}"))),
        };
        if !metadata.is_file() || metadata.uid() != 0 || metadata.mode() & 0o022 != 0 {
            return Err(Failure::unknown(format!(
                "fw4 template {name} must be a root-owned regular file"
            )));
        }
        let source = std::fs::read_to_string(&path).map_err(|e| Failure::unknown(e.to_string()))?;
        if matches!(*name, "rule.uc" | "zone-verdict.uc")
            && !source.contains(LOG)
            && !source.contains(NFLOG)
        {
            return Err(Failure::unknown(format!(
                "unsupported logging template {name}"
            )));
        }
        let rendered = template(&source, enable);
        if rendered != source {
            changes.push((path, rendered, metadata.mode() & 0o777));
        }
    }
    for (path, source, mode) in changes {
        let temporary = path.with_extension(format!("verso-{}.tmp", std::process::id()));
        let mut file = std::fs::OpenOptions::new()
            .write(true)
            .create_new(true)
            .mode(mode)
            .open(&temporary)
            .map_err(|e| Failure::unknown(e.to_string()))?;
        let result = file
            .write_all(source.as_bytes())
            .and_then(|_| std::fs::rename(&temporary, &path));
        if result.is_err() {
            let _ = std::fs::remove_file(&temporary);
        }
        result.map_err(|e| Failure::unknown(e.to_string()))?;
    }
    Ok(())
}

pub(super) fn plan(ruleset: &Value, enable: bool) -> Result<Value, String> {
    let rules = ruleset["nftables"]
        .as_array()
        .ok_or("nft did not return a ruleset")?;
    let mut changes = Vec::new();
    for item in rules {
        let Some(rule) = item.get("rule") else {
            continue;
        };
        if rule["family"] != "inet" || rule["table"] != "fw4" {
            continue;
        }
        let mut replacement = rule.clone();
        let Some(expr) = replacement["expr"].as_array_mut() else {
            continue;
        };
        let mut changed = false;
        for statement in expr {
            if let Some(log) = statement.get_mut("log") {
                // An explicit group belongs to whoever configured it, including 0.
                if (enable && log.get("group").is_some()) || (!enable && log["group"] != GROUP) {
                    continue;
                }
                let prefix = log["prefix"].as_str().unwrap_or("").to_owned();
                *log = if enable {
                    json!({"prefix":prefix,"group":GROUP,"snaplen":SNAPLEN,"queue-threshold":1})
                } else {
                    json!({"prefix":prefix})
                };
                changed = true;
            }
        }
        if changed {
            if replacement["handle"].as_u64().is_none() || replacement["chain"].as_str().is_none() {
                return Err("nft rule has no handle or chain".into());
            }
            changes.push(json!({"replace":{"rule":replacement}}));
        }
    }
    Ok(json!({"nftables":changes}))
}

pub(super) fn apply(enable: bool) -> Result<(), Failure> {
    let mut list = Command::new("/usr/sbin/nft");
    list.args(["-j", "list", "table", "inet", "fw4"]);
    let output = run_bounded_within(
        list,
        "read firewall logging",
        4 * 1024 * 1024,
        std::time::Duration::from_secs(5),
    )?;
    if !output.status.success() {
        // Removing Verso while fw4 is stopped still restores its templates.
        if !enable && String::from_utf8_lossy(&output.stderr).contains("No such file or directory")
        {
            return Ok(());
        }
        return Err(command_failure("read firewall logging", output));
    }
    let value: Value =
        serde_json::from_slice(&output.stdout).map_err(|e| Failure::unknown(e.to_string()))?;
    let commands = plan(&value, enable).map_err(Failure::unknown)?;
    if commands["nftables"].as_array().is_some_and(Vec::is_empty) {
        return Ok(());
    }
    let input = serde_json::to_vec(&commands).map_err(|e| Failure::unknown(e.to_string()))?;
    // Feed nft through a pipe, not a file a peer could replace in /tmp. A writer
    // thread lets the existing bounded runner enforce its process timeout.
    let (mut writer, reader) =
        std::os::unix::net::UnixStream::pair().map_err(|e| Failure::unknown(e.to_string()))?;
    let writing = std::thread::spawn(move || writer.write_all(&input));
    let mut command = Command::new("/usr/sbin/nft");
    command.args(["-j", "-f", "-"]);
    let result = run_bounded_with_stdin(
        command,
        "route firewall logging",
        64 * 1024,
        std::time::Duration::from_secs(5),
        Stdio::from(std::os::fd::OwnedFd::from(reader)),
    );
    let written = writing.join();
    let output = result?;
    if !output.status.success() {
        return Err(command_failure("route firewall logging", output));
    }
    written
        .map_err(|_| Failure::unknown("firewall log transaction writer stopped"))?
        .map_err(|e| Failure::unknown(format!("write firewall log transaction: {e}")))?;
    Ok(())
}
