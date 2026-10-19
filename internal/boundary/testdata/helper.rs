// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

// Each line marked "violation" compiles a plugin's source into the helper;
// the SDK's is the one plugin tree the helper may take from.

#[path = "../../plugins/verso-plugin-system/src/datetime.rs"] // violation
mod datetime;

#[path = "../../plugins/verso-plugin-sdk/src/sha256.rs"]
mod sha256;

const RULES: &str = include_str!("../../plugins/verso-plugin-firewall/src/ruleset.rs"); // violation

include!("../../plugins/verso-plugin-vpn/src/openvpn.rs"); // violation

const SAMPLES: &str = include_str!("../../plugins/verso-plugin-sdk/testdata/args.json");
