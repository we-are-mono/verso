// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

//! A UCI truth value as OpenWrt's init scripts read one: `config_get_bool` and
//! ubox's `validate_data` take the same ten spellings. A daemon that parses its
//! own config with a narrower set (firewall4, netifd) reads it its own way.

use crate::Section;

/// uci_bool reads a written truth value: `1`, `on`, `true`, `yes`, `enabled`
/// and `0`, `off`, `false`, `no`, `disabled`. None means the value is not one,
/// which those readers take as the option's default.
pub fn uci_bool(value: &str) -> Option<bool> {
    match value {
        "1" | "on" | "true" | "yes" | "enabled" => Some(true),
        "0" | "off" | "false" | "no" | "disabled" => Some(false),
        _ => None,
    }
}

impl Section<'_> {
    /// flag reads a boolean option as [`uci_bool`] does, `default` where the
    /// option is absent or not a truth value.
    pub fn flag(&self, option: &str, default: bool) -> bool {
        uci_bool(&self.scalar(option)).unwrap_or(default)
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::{json, Snapshot};

    #[test]
    fn every_spelling_init_scripts_accept_reads_as_its_truth() {
        for on in ["1", "on", "true", "yes", "enabled"] {
            assert_eq!(uci_bool(on), Some(true), "{on}");
        }
        for off in ["0", "off", "false", "no", "disabled"] {
            assert_eq!(uci_bool(off), Some(false), "{off}");
        }
        for neither in ["", "2", "YES", " 1"] {
            assert_eq!(uci_bool(neither), None, "{neither:?}");
        }
    }

    #[test]
    fn an_absent_or_unreadable_option_is_its_default() {
        let snapshot = Snapshot::from_value(json!({"system": {"ntp": {
            ".name": "ntp", ".type": "timeserver", "enabled": "no", "enable_server": "maybe"
        }}}));
        let ntp = snapshot.section("system", "ntp").expect("the section");
        assert!(!ntp.flag("enabled", true));
        assert!(ntp.flag("enable_server", true));
        assert!(!ntp.flag("enable_server", false));
        assert!(ntp.flag("use_dhcp", true));
    }
}
