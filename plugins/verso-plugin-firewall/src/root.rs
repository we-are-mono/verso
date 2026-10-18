// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

//! The plugin's root half (ADR-027): `verso-plugin-firewall root`, run by the
//! privileged helper as root for the `state` read its registry declares — the
//! kernel's hit counters for each rule fw4 rendered.

use crate::ruleset;
use std::io::Read;
use std::process::{Command, Stdio};
use verso_plugin::root::{RootError, RootRequest};
use verso_plugin::{Map, Value};

/// The version of the `state` read: raised only when its result changes
/// incompatibly (ADR-027 §4).
pub const STATE_VERSION: u32 = 1;

/// The most of fw4's table read, so a vast ruleset cannot exhaust memory.
const MAX_RULESET: u64 = 4 * 1024 * 1024;

pub fn run() -> ! {
    verso_plugin::root::run(&[("state", &state)])
}

fn state(_: &RootRequest) -> Result<Map<String, Value>, RootError> {
    let listed = list_fw4()?;
    match ruleset::counters(&listed).map_err(RootError::failure)? {
        Value::Object(map) => Ok(map),
        _ => Err(RootError::failure("the counters are not an object")),
    }
}

/// list_fw4 is `nft -j list table inet fw4`, bounded; the helper stops the
/// whole run at the command's timeout.
fn list_fw4() -> Result<Vec<u8>, RootError> {
    let mut child = Command::new("/usr/sbin/nft")
        .args(["-j", "list", "table", "inet", "fw4"])
        .stdin(Stdio::null())
        .stdout(Stdio::piped())
        .stderr(Stdio::null())
        .spawn()
        .map_err(RootError::failure)?;
    let mut listed = Vec::new();
    if let Some(out) = child.stdout.take() {
        out.take(MAX_RULESET + 1).read_to_end(&mut listed).map_err(RootError::failure)?;
    }
    if listed.len() as u64 > MAX_RULESET {
        let _ = child.kill();
        let _ = child.wait();
        return Err(RootError::failure("fw4's table is larger than the counters read takes"));
    }
    let status = child.wait().map_err(RootError::failure)?;
    if !status.success() {
        return Err(RootError::failure(format!("nft ended with {status}")));
    }
    Ok(listed)
}

#[cfg(test)]
mod tests {
    use super::*;
    use verso_plugin::Registry;

    /// The registry the package installs registers what this half answers.
    #[test]
    fn the_registry_names_what_the_root_half_answers() {
        let text = std::fs::read_to_string("rootfs/usr/share/verso/plugins/firewall/commands.json").unwrap();
        let registry: Registry = serde_json::from_str(&text).unwrap();
        registry.check().unwrap();
        assert_eq!(registry.commands.len(), 1);
        assert_eq!(registry.commands[0].name, "state");
        assert_eq!(registry.commands[0].version, Some(STATE_VERSION));
        assert_eq!(registry.commands[0].exec, "/usr/bin/verso-plugin-firewall");
        let family = &registry.families[0];
        assert_eq!((family.name.as_str(), family.config.as_str()), ("fw4", "firewall"));
        assert_eq!(family.check, ["/sbin/fw4", "check"]);
        assert_eq!(family.reload, ["/sbin/fw4", "-q", "restart"]);
    }
}
