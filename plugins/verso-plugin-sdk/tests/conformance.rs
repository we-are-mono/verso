// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.
//
// Conformance fixtures: serialize one populated sample of every Widget variant
// into testdata/, where the shell's Go tests decode each through widget.Decode
// (internal/widget/sdk_conformance_test.go). This is the pin that keeps the
// SDK's typed mirror and the shell's vocabulary from drifting apart (ADR-006 §9):
// a renamed field or a wrong tag fails the shell's suite, not a user's page.

use std::fs;
use verso_plugin::{Property, Tone, Widget};

#[test]
fn write_widget_fixtures() {
    let samples: Vec<(&str, Widget)> = vec![
        (
            "card",
            Widget::card("Tunnel", vec![Widget::text("One tunnel, two peers.")]),
        ),
        (
            "section",
            Widget::section("Peers", "Devices that may connect.", vec![Widget::text("body")]),
        ),
        ("stack", Widget::stack(vec![Widget::text("a"), Widget::text("b")])),
        (
            "conditional",
            Widget::Conditional {
                name: "ntp_enabled".into(),
                label: "Set the time automatically".into(),
                checked: true,
                fields: vec![Widget::text("on")],
                otherwise: vec![Widget::text("off")],
            },
        ),
        (
            "select",
            Widget::select(
                "zonename",
                "Timezone",
                "UTC",
                vec![verso_plugin::SelectOption { value: "UTC".into(), label: "UTC".into() }],
                "",
            ),
        ),
        ("hidden", Widget::hidden("ntp_section", "cfg1")),
        (
            "grid",
            Widget::Grid { columns: 2, children: vec![Widget::text("a"), Widget::text("b")] },
        ),
        (
            "form",
            Widget::form(
                "Save",
                vec![Widget::field("hostname", "Hostname", "router.lan", "fqdn", "The device's name.")],
            ),
        ),
        (
            "field",
            Widget::field("hostname", "Hostname", "router.lan", "fqdn", "The device's name."),
        ),
        (
            "list",
            Widget::list(
                "server",
                "NTP servers",
                "host",
                &["0.openwrt.pool.ntp.org".to_string()],
                "One per row.",
            ),
        ),
        (
            "callout",
            Widget::callout(Tone::Warning, "Not reachable", "The tunnel has no endpoint yet."),
        ),
        ("code", Widget::code("Public key", "hLIgo9xNzJM=")),
        (
            "empty",
            Widget::empty("shield", "No tunnels yet", "Create one to reach home from anywhere.", vec![]),
        ),
        (
            "properties",
            Widget::Properties {
                items: vec![Property {
                    label: "Endpoint".into(),
                    value: "203.0.113.7:51820".into(),
                    mono: true,
                    copy: true,
                }],
            },
        ),
        (
            "badge",
            Widget::Badge { variant: Tone::Success, text: "up".into(), dot: true },
        ),
        ("text", Widget::text("A short explanatory line.")),
        (
            "confirm",
            Widget::Confirm {
                trigger: "Remove peer".into(),
                message: "The device loses access immediately.".into(),
                confirm: String::new(),
                cancel: String::new(),
            },
        ),
        ("raw", Widget::raw("WireGuard keeps a silent tunnel silent — an idle peer can be perfectly healthy.")),
    ];

    let dir = concat!(env!("CARGO_MANIFEST_DIR"), "/testdata");
    fs::create_dir_all(dir).unwrap();
    for (name, w) in samples {
        let json = serde_json::to_vec_pretty(&w).unwrap();
        // Every widget must carry its wire discriminator.
        let v: serde_json::Value = serde_json::from_slice(&json).unwrap();
        assert!(v.get("type").is_some(), "{name}: missing type tag");
        fs::write(format!("{dir}/widget-{name}.json"), json).unwrap();
    }
}
