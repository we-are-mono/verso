// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

//! Factory reset, and with it, unless the operator keeps it, the router's
//! history (ADR-021). `firstboot` clears the overlay and leaves /data, so the
//! history directory (config history, events, charts) and nlbwmon's
//! per-device usage there would outlive a reset: a router handed on would
//! carry its past to its next owner.
//!
//! The directories are the helper's to find, never the shell's to name: a
//! shell that could name one could have root empty any directory. The history
//! directory is found as the shell's init script finds it, and is emptied
//! only when it is a directory the shell's account owns.

use std::fs;
use std::os::unix::fs::MetadataExt;
use std::path::{Path, PathBuf};
use std::process::Command;
use std::time::Duration;

use crate::Failure;

/// Where history is kept without a choice of the operator's, on a router
/// with /data and on one without.
const DATA_HISTORY: &str = "/data/verso";
const OVERLAY_HISTORY: &str = "/usr/share/verso/history";
/// Where usage-setup keeps nlbwmon's database on a router with /data; on one
/// without, it is on the overlay, which firstboot clears.
const DATA_USAGE: &str = "/data/nlbwmon";

/// start resets the router to its defaults, after emptying its history if
/// erase is set. It answers first: the reset stops the shell that asked, and
/// the router restarts.
pub fn start(erase: bool) -> Result<(), Failure> {
    if !erase {
        return crate::spawn_system_action("/sbin/firstboot", &["-r", "-y"]);
    }
    let directories = history_directories();
    std::thread::spawn(move || {
        // Long enough for the answer to reach the shell before it stops.
        std::thread::sleep(Duration::from_secs(1));
        let _ = Command::new("/etc/init.d/verso").arg("stop").status();
        for dir in &directories {
            if let Err(error) = empty(dir, &KEEPERS) {
                eprintln!("verso-rpcd: reset: {} not emptied: {error}", dir.display());
            }
        }
        let _ = Command::new("/sbin/firstboot").args(["-r", "-y"]).status();
    });
    Ok(())
}

/// history_directories is every directory the router's history is kept in
/// that a reset would leave: the history directory, and nlbwmon's on /data.
fn history_directories() -> Vec<PathBuf> {
    let configured = uci_get("verso.history.directory");
    let data = data_mounted();
    let mut dirs = vec![history_directory(configured.as_deref(), data)];
    if uci_get("nlbwmon.@nlbwmon[0].database_directory").as_deref() == Some(DATA_USAGE) {
        dirs.push(PathBuf::from(DATA_USAGE));
    }
    dirs
}

/// history_directory is where the shell keeps history: the operator's
/// choice, else /data/verso where /data is a mount, else on the overlay — the
/// init script's rule.
fn history_directory(configured: Option<&str>, data_mounted: bool) -> PathBuf {
    match configured {
        Some(dir) if !dir.is_empty() => PathBuf::from(dir),
        _ if data_mounted => PathBuf::from(DATA_HISTORY),
        _ => PathBuf::from(OVERLAY_HISTORY),
    }
}

fn uci_get(key: &str) -> Option<String> {
    let output = Command::new("/sbin/uci")
        .args(["-q", "get", key])
        .output()
        .ok()?;
    let value = String::from_utf8(output.stdout).ok()?.trim().to_owned();
    (output.status.success() && !value.is_empty()).then_some(value)
}

fn data_mounted() -> bool {
    fs::read_to_string("/proc/mounts")
        .map(|mounts| {
            mounts
                .lines()
                .any(|line| line.split(' ').nth(1) == Some("/data"))
        })
        .unwrap_or(false)
}

/// The accounts a history directory may belong to: the shell's, which the
/// init script gives it, or root's.
const KEEPERS: [u32; 2] = [crate::VERSO_UID, 0];

/// empty removes everything a history directory holds and keeps the
/// directory. It refuses anything but an absolute directory, not the root,
/// that one of keepers owns and that is no link.
fn empty(dir: &Path, keepers: &[u32]) -> Result<(), String> {
    if !dir.is_absolute() || dir.parent().is_none() {
        return Err("not a directory of its own".into());
    }
    let meta = fs::symlink_metadata(dir).map_err(|e| e.to_string())?;
    if !meta.is_dir() || !keepers.contains(&meta.uid()) {
        return Err("not a directory the shell keeps".into());
    }
    for entry in fs::read_dir(dir).map_err(|e| e.to_string())? {
        let path = entry.map_err(|e| e.to_string())?.path();
        let removed = match fs::symlink_metadata(&path) {
            Ok(m) if m.is_dir() => fs::remove_dir_all(&path),
            _ => fs::remove_file(&path),
        };
        removed.map_err(|e| format!("{}: {e}", path.display()))?;
    }
    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn the_history_directory_is_found_as_the_init_script_finds_it() {
        assert_eq!(
            history_directory(Some("/mnt/usb/verso"), true),
            PathBuf::from("/mnt/usb/verso")
        );
        assert_eq!(
            history_directory(Some(""), true),
            PathBuf::from(DATA_HISTORY)
        );
        assert_eq!(history_directory(None, true), PathBuf::from(DATA_HISTORY));
        assert_eq!(
            history_directory(None, false),
            PathBuf::from(OVERLAY_HISTORY)
        );
    }

    #[test]
    fn emptying_keeps_the_directory_and_nothing_in_it() {
        let dir = std::env::temp_dir().join(format!("verso-erase-{}", std::process::id()));
        fs::create_dir_all(dir.join("usage")).unwrap();
        fs::write(dir.join("history.db"), "x").unwrap();
        fs::write(dir.join("usage/day"), "x").unwrap();
        let outside = std::env::temp_dir().join(format!("verso-outside-{}", std::process::id()));
        fs::write(&outside, "kept").unwrap();
        std::os::unix::fs::symlink(&outside, dir.join("link")).unwrap();

        let me = fs::metadata(&dir).unwrap().uid();
        empty(&dir, &[me]).unwrap();
        assert!(dir.is_dir());
        assert_eq!(fs::read_dir(&dir).unwrap().count(), 0);
        assert_eq!(
            fs::read_to_string(&outside).unwrap(),
            "kept",
            "a link is removed, never followed"
        );
        fs::remove_dir(dir).unwrap();
        fs::remove_file(outside).unwrap();
    }

    #[test]
    fn only_a_directory_of_its_own_is_emptied() {
        assert!(empty(Path::new("/"), &[0]).is_err());
        assert!(empty(Path::new("relative"), &[0]).is_err());
        let target = std::env::temp_dir().join(format!("verso-target-{}", std::process::id()));
        fs::create_dir_all(&target).unwrap();
        fs::write(target.join("keep"), "x").unwrap();
        let link = std::env::temp_dir().join(format!("verso-dirlink-{}", std::process::id()));
        let _ = fs::remove_file(&link);
        std::os::unix::fs::symlink(&target, &link).unwrap();
        let me = fs::metadata(&target).unwrap().uid();
        assert!(empty(&link, &[me]).is_err());
        assert!(target.join("keep").exists());
        fs::remove_file(link).unwrap();
        fs::remove_dir_all(target).unwrap();
    }
}
