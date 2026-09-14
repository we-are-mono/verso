// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

//! The system-firmware answer, read from owut's `check`.
//!
//! owut asks an attended-sysupgrade server whether it can build a newer image for
//! this exact device and package set. The device's `attendedsysupgrade` uci config
//! names that server (owut falls back to OpenWrt's own), so the same call serves a
//! Mono Gateway pointed at sysupgrade.mono.si and an official target pointed at
//! openwrt.org.
//!
//! owut speaks to people, not to programs: `check` prints a config block, the two
//! versions, and a closing verdict line. That text is the whole contract, so this
//! module reads exactly the lines owut commits to and reports which rung of the
//! ladder the device landed on. Not every device can be answered — owut may not be
//! installed, the server may be unreachable, the target may not be buildable — and
//! a rung named plainly is worth more than an error, because the page has something
//! true to say either way.

use serde_json::{json, Value};

/// The states a firmware check can honestly report.
///
/// `update` and `current` are answers; the rest name why there is none.
pub const STATE_UPDATE: &str = "update";
pub const STATE_CURRENT: &str = "current";
pub const STATE_NO_OWUT: &str = "no-owut";
pub const STATE_NO_SERVER: &str = "no-server";
pub const STATE_UNSUPPORTED: &str = "unsupported";

/// The verdict owut prints when the server has nothing newer to build.
const VERDICT_CURRENT: &str = "There are no changes, upgrade not necessary";
/// The verdict owut prints when a build is available and passes its own checks.
const VERDICT_UPDATE: &str = "It is safe to proceed with an upgrade";

/// unavailable reports a rung the check never got to run on, so the caller can
/// answer without an owut invocation at all.
pub fn unavailable(state: &str, message: &str) -> Value {
    json!({
        "state": state,
        "server": "",
        "from": "",
        "to": "",
        "packages": 0,
        "message": message,
    })
}

/// summarize reduces one `owut check` run to the rung it landed on.
///
/// A run that exits cleanly still has to say which of the two answers it reached;
/// a run that fails is classified by what owut complained about, since a server it
/// cannot reach and a device it cannot build for are different facts to a person.
pub fn summarize(stdout: &[u8], stderr: &[u8], success: bool) -> Value {
    let out = String::from_utf8_lossy(stdout);
    let err = String::from_utf8_lossy(stderr);
    let (from, from_code) = version_line(&out, "Version-from");
    let (to, to_code) = version_line(&out, "Version-to");
    let state = if success && out.contains(VERDICT_CURRENT) {
        STATE_CURRENT
    } else if success && out.contains(VERDICT_UPDATE) {
        STATE_UPDATE
    } else if unreachable_server(&err) {
        STATE_NO_SERVER
    } else {
        STATE_UNSUPPORTED
    };
    let message = match state {
        STATE_UPDATE | STATE_CURRENT => String::new(),
        _ => complaint(&err, &out),
    };
    json!({
        "state": state,
        "server": field(&out, "ASU-Server"),
        "from": join_version(&from, &from_code),
        "to": join_version(&to, &to_code),
        "packages": out_of_date(&out),
        "message": message,
    })
}

// owut reports an unreachable server through its uclient layer, and says so in
// those words. Anything else it refuses on is about this device, not the network.
fn unreachable_server(stderr: &str) -> bool {
    stderr.contains("uclient error") || stderr.contains("server being down or inaccessible")
}

// The config block is aligned columns: a label, whitespace, the value.
fn field(stdout: &str, label: &str) -> String {
    stdout
        .lines()
        .find_map(|line| line.strip_prefix(label))
        .map(|rest| rest.trim().to_string())
        .unwrap_or_default()
}

// A version line is "<version> <rev-code> (kernel <k>)"; the kernel is the
// image's, not this device's running one, so it is left out of the answer.
fn version_line(stdout: &str, label: &str) -> (String, String) {
    let value = field(stdout, label);
    let mut parts = value.split_whitespace();
    let version = parts.next().unwrap_or_default().to_string();
    let code = parts
        .next()
        .filter(|code| !code.starts_with('('))
        .unwrap_or_default()
        .to_string();
    (version, code)
}

fn join_version(version: &str, code: &str) -> String {
    match (version.is_empty(), code.is_empty()) {
        (true, _) => String::new(),
        (false, true) => version.to_string(),
        (false, false) => format!("{version} {code}"),
    }
}

// "N packages are out-of-date" is owut's count of what an upgrade would change.
fn out_of_date(stdout: &str) -> u64 {
    stdout
        .lines()
        .find_map(|line| line.trim().strip_suffix(" packages are out-of-date"))
        .and_then(|count| count.trim().parse().ok())
        .unwrap_or(0)
}

/// complaint_of repeats back what a failed owut run said, so the page can quote
/// it verbatim. An upgrade that never got started fails for the same reasons a
/// check does — no server, a build the server refused, a device it cannot answer
/// for — so the reading is the same one `summarize` applies, exposed for the runs
/// that have no rung to land on.
pub fn complaint_of(stdout: &[u8], stderr: &[u8]) -> String {
    complaint(
        &String::from_utf8_lossy(stderr),
        &String::from_utf8_lossy(stdout),
    )
}

// What to repeat back when there is no answer: owut's own complaint, stripped of
// its severity prefix, or its last words if it failed without one.
fn complaint(stderr: &str, stdout: &str) -> String {
    let line = stderr
        .lines()
        .map(str::trim)
        .find(|line| line.starts_with("ERROR:"))
        .or_else(|| {
            stderr
                .lines()
                .map(str::trim)
                .find(|line| !line.is_empty() && !line.starts_with("WARNING:"))
        })
        .or_else(|| stdout.lines().map(str::trim).rev().find(|l| !l.is_empty()))
        .unwrap_or_default();
    line.strip_prefix("ERROR:")
        .unwrap_or(line)
        .trim()
        .to_string()
}

#[cfg(test)]
mod tests {
    use super::*;

    // Captured from `owut check` on the dev container against the official ASU
    // server (the container is a real x86/64 target, so the answer is a real one).
    const AVAILABLE: &[u8] = include_bytes!("../testdata/owut-check.txt");

    #[test]
    fn an_available_build_reports_both_versions_and_the_server() {
        let answer = summarize(
            AVAILABLE,
            b"WARNING: There are 13 missing default packages\n",
            true,
        );
        assert_eq!(answer["state"], STATE_UPDATE);
        assert_eq!(answer["server"], "https://sysupgrade.openwrt.org");
        assert_eq!(answer["from"], "25.12.4 r32933-4ccb782af7");
        assert_eq!(answer["to"], "25.12.5 r33051-f5dae5ece4");
        assert_eq!(answer["packages"], 78);
        assert_eq!(answer["message"], "");
    }

    #[test]
    fn a_device_already_on_the_newest_build_is_current() {
        let stdout = concat!(
            "ASU-Server     https://sysupgrade.mono.si\n",
            "Version-from   25.12.5 r33051-f5dae5ece4 (kernel 6.12.94)\n",
            "Version-to     25.12.5 r33051-f5dae5ece4 (kernel 6.12.94)\n",
            "All packages are up-to-date\n",
            "There are no changes, upgrade not necessary (re-run with '--verbose' for details)\n",
        );
        let answer = summarize(stdout.as_bytes(), b"", true);
        assert_eq!(answer["state"], STATE_CURRENT);
        assert_eq!(answer["server"], "https://sysupgrade.mono.si");
        assert_eq!(answer["to"], "25.12.5 r33051-f5dae5ece4");
        assert_eq!(answer["packages"], 0);
    }

    // The dev container is a Docker rootfs, not a flashable image, so owut cannot
    // determine a filesystem type — the honest answer is that this device cannot
    // be checked, carrying owut's own words.
    #[test]
    fn a_device_owut_cannot_evaluate_is_unsupported() {
        let answer = summarize(
            b"",
            b"ERROR: File system type '(null)' should be one of [ \"ext4\", \"squashfs\", \"targz\" ]\n",
            false,
        );
        assert_eq!(answer["state"], STATE_UNSUPPORTED);
        assert_eq!(
            answer["message"],
            "File system type '(null)' should be one of [ \"ext4\", \"squashfs\", \"targz\" ]"
        );
        assert_eq!(answer["from"], "");
    }

    #[test]
    fn a_server_that_does_not_answer_is_its_own_rung() {
        let answer = summarize(
            b"ASU-Server     https://sysupgrade.mono.si\n",
            b"ERROR: uclient error code=-1\n  This could be due to the server being down or inaccessible, check\n",
            false,
        );
        assert_eq!(answer["state"], STATE_NO_SERVER);
        assert_eq!(answer["server"], "https://sysupgrade.mono.si");
        assert!(answer["message"]
            .as_str()
            .unwrap()
            .contains("uclient error"));
    }

    // A failed run that nevertheless reached a verdict is still a failure: owut
    // says a build is possible only when its own checks pass.
    #[test]
    fn a_failed_run_never_reports_an_available_build() {
        let answer = summarize(
            b"Version-to     25.12.5 r33051-f5dae5ece4 (kernel 6.12.94)\nIt is safe to proceed with an upgrade\n",
            b"ERROR: Checks reveal errors, do not upgrade\n",
            false,
        );
        assert_eq!(answer["state"], STATE_UNSUPPORTED);
        assert_eq!(answer["message"], "Checks reveal errors, do not upgrade");
    }

    // An upgrade that owut refused says why in owut's own words, so the page can
    // quote the tool rather than paraphrase it.
    #[test]
    fn a_refused_upgrade_carries_owuts_own_words() {
        assert_eq!(
            complaint_of(
                b"ASU-Server     https://sysupgrade.mono.si\n",
                b"WARNING: There are 13 missing default packages\nERROR: Update checks reveal errors, can't proceed\n",
            ),
            "Update checks reveal errors, can't proceed"
        );
        // No ERROR line: owut's last words stand in, from stdout if that is all
        // there is.
        assert_eq!(
            complaint_of(b"There are no changes to download (see '--force')\n", b""),
            "There are no changes to download (see '--force')"
        );
        // A run that said nothing has nothing to quote; the caller supplies the
        // plain sentence instead of inventing one here.
        assert_eq!(complaint_of(b"", b""), "");
    }

    #[test]
    fn a_missing_owut_is_reported_without_running_anything() {
        let answer = unavailable(STATE_NO_OWUT, "owut is not installed");
        assert_eq!(answer["state"], STATE_NO_OWUT);
        assert_eq!(answer["message"], "owut is not installed");
        assert_eq!(answer["packages"], 0);
    }
}
