// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.
//
// Conformance fixtures: serialize one populated sample of every Widget variant
// into testdata/, where the shell's Go tests decode each through widget.Decode
// (internal/widget/sdk_conformance_test.go). This is the pin that keeps the
// SDK's typed mirror and the shell's vocabulary from drifting apart (ADR-006 §9):
// a renamed field or a wrong tag fails the shell's suite, not a user's page.

use std::collections::BTreeMap;
use std::fs;
use verso_plugin::{
    ConditionItem, Property, RowDrawer, SelectOption, SettingsItem, SettingsPill, SettingsSeam,
    SettingsToggle, TableCell, TableColumn, TableEndpoint, TableGroup, TableRow, Tone, Widget,
};

#[test]
fn write_widget_fixtures() {
    let samples: Vec<(&str, Widget)> = vec![
        (
            "card",
            Widget::card("Tunnel", vec![Widget::text("One tunnel, two peers.")]),
        ),
        (
            "section",
            Widget::Section {
                title: "Rule".into(),
                sub: "Devices that may connect.".into(),
                meta: "3 peers".into(),
                meta_icon: "shield".into(),
                meta_position: "inline".into(),
                flush: true,
                control: Some(Box::new(Widget::switch("enabled", "Enabled", true))),
                children: vec![Widget::text("body")],
            },
        ),
        (
            "stack",
            Widget::Stack {
                width: "compact".into(),
                compact: true,
                inline: true,
                divided: true,
                children: vec![Widget::text("a"), Widget::text("b")],
            },
        ),
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
            "checks",
            Widget::checks(
                "weekdays",
                "Weekdays",
                &["Mon".to_string(), "Tue".to_string()],
                vec![
                    SelectOption::new("Mon", "Mon"),
                    SelectOption::new("Tue", "Tue"),
                    SelectOption::new("Wed", "Wed"),
                ],
            ),
        ),
        (
            "grid",
            Widget::Grid {
                style: "form".into(),
                columns: 2,
                children: vec![Widget::text("a"), Widget::text("b")],
            },
        ),
        (
            "switch",
            Widget::Switch {
                name: "enabled".into(),
                label: "Enabled".into(),
                off_label: "Disabled".into(),
                help: "A disabled rule is kept but never evaluated.".into(),
                style: "inline".into(),
                icon: "shield".into(),
                meta: "Applies at the next save".into(),
                on: true,
            },
        ),
        (
            "conditions",
            Widget::Conditions {
                label: "Conditions".into(),
                help: "Conditions are combined with and.".into(),
                items: vec![
                    ConditionItem {
                        key: "dest_port".into(),
                        label: "Destination ports".into(),
                        help: "TCP/UDP ports or ranges from 0 to 65535.".into(),
                        active: true,
                        children: vec![Widget::tokens(
                            "dest_port",
                            "Include",
                            "Port or range",
                            &["53".to_string(), "1024-65535".to_string()],
                            "For example 53 or 1024-65535.",
                        )],
                    },
                    ConditionItem {
                        key: "src_mac".into(),
                        label: "Source MAC addresses".into(),
                        help: "Match link-layer senders visible on ingress.".into(),
                        active: false,
                        children: vec![Widget::tokens("src_mac", "Include", "MAC address", &[], "")],
                    },
                ],
            },
        ),
        (
            "disclosure",
            Widget::Disclosure {
                style: "condition".into(),
                summary: "Parameters for advanced actions".into(),
                children: vec![Widget::text("Only the selected action's parameters apply.")],
            },
        ),
        (
            "link",
            Widget::Link {
                label: "Add rule".into(),
                icon: "plus".into(),
                href: "/plugins/firewall/rules/new".into(),
                style: "button".into(),
            },
        ),
        (
            "form",
            Widget::Form {
                style: "page".into(),
                submit: "Save".into(),
                error: "Some values aren’t ones the daemon accepts.".into(),
                fields: vec![Widget::field(
                    "hostname",
                    "Hostname",
                    "router.lan",
                    "fqdn",
                    "The device's name.",
                )],
            },
        ),
        (
            "field",
            Widget::Field {
                name: "hostname".into(),
                label: "Hostname".into(),
                kind: "text".into(),
                value: "router.lan".into(),
                values: Vec::new(),
                placeholder: "router.lan".into(),
                datatype: "fqdn".into(),
                options: Vec::new(),
                error: "must be a fully-qualified domain name".into(),
                help: "The device's name.".into(),
            },
        ),
        (
            "list",
            Widget::List {
                name: "server".into(),
                label: "NTP servers".into(),
                kind: "text".into(),
                style: "tokens".into(),
                prompt: "Server address".into(),
                datatype: "host".into(),
                items: vec!["0.openwrt.pool.ntp.org".into(), "not a host".into()],
                errors: BTreeMap::from([("1".to_string(), "must be a hostname".to_string())]),
                help: "One per row.".into(),
            },
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
        ("table", listing()),
        (
            "settings",
            Widget::Settings {
                style: "card".into(),
                title: "Global defaults".into(),
                meta: "firewall.@defaults[0]".into(),
                items: vec![
                    SettingsItem {
                        title: "Default policies".into(),
                        desc: "What happens to traffic that matches no rule.".into(),
                        code: "input · output · forward".into(),
                        pills: vec![
                            SettingsPill { variant: Tone::Warning, text: "in reject".into() },
                            SettingsPill { variant: Tone::Success, text: "out accept".into() },
                            SettingsPill { variant: Tone::Warning, text: "fwd reject".into() },
                        ],
                        ..SettingsItem::default()
                    },
                    SettingsItem {
                        title: "Drop invalid packets".into(),
                        desc: "Discard packets that belong to no known connection.".into(),
                        code: "drop_invalid".into(),
                        toggle: Some(SettingsToggle { name: "drop_invalid".into(), on: true }),
                        ..SettingsItem::default()
                    },
                    SettingsItem {
                        title: "Tracked connections".into(),
                        desc: "How many connections the router follows at once.".into(),
                        code: "nf_conntrack_max".into(),
                        value: "16384".into(),
                        name: "nf_conntrack_max".into(),
                        ..SettingsItem::default()
                    },
                ],
                seam: Some(SettingsSeam {
                    summary: "2 more options".into(),
                    items: vec![
                        SettingsItem {
                            title: "Software flow offloading".into(),
                            desc: "Fast-path established connections.".into(),
                            code: "flow_offloading".into(),
                            toggle: Some(SettingsToggle {
                                name: "flow_offloading".into(),
                                on: false,
                            }),
                            ..SettingsItem::default()
                        },
                        SettingsItem {
                            title: "Custom rules file".into(),
                            desc: "Read by fw4 after the generated ruleset.".into(),
                            code: "/etc/nftables.d".into(),
                            ..SettingsItem::default()
                        },
                    ],
                }),
            },
        ),
        (
            "filter",
            Widget::Filter { placeholder: "Filter rules, zones, and ports…".into() },
        ),
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

// listing is the table fixture: a column set broad enough that every TableCell
// field appears on a cell whose column kind actually reads it, so no field can
// be renamed on either side of the wire without this pin failing.
fn listing() -> Widget {
    Widget::Table {
        style: "flat".into(),
        title: "Zones".into(),
        detail: "4 zones · 2 forwardings".into(),
        condensed: true,
        align: "top".into(),
        reorder_config: "firewall".into(),
        reorder_label: "Rule order".into(),
        columns: vec![
            TableColumn { kind: "reorder".into(), ..TableColumn::default() },
            TableColumn { label: "Zone".into(), kind: "name".into() },
            TableColumn { label: "Address".into(), kind: "addr".into() },
            TableColumn { label: "From".into(), kind: "endpoint".into() },
            TableColumn { label: "Protocol".into(), kind: "keyword".into() },
            TableColumn { label: "Input".into(), kind: "pill".into() },
            TableColumn { label: "Hits".into(), kind: "num".into() },
            TableColumn { kind: "toggle".into(), ..TableColumn::default() },
            TableColumn { kind: "link".into(), ..TableColumn::default() },
            TableColumn { kind: "pill".into(), ..TableColumn::default() },
        ],
        rows: vec![
            TableRow {
                id: "cfg02zone".into(),
                key: "zone:lan".into(),
                group: Some(TableGroup {
                    label: "LAN → Router".into(),
                    chain: "input_lan".into(),
                    count: 2,
                }),
                cells: vec![
                    TableCell::default(),
                    TableCell {
                        text: "lan".into(),
                        chip: "trusted".into(),
                        chip_icon: "zone".into(),
                        lead_icon: "network".into(),
                        tag: "default".into(),
                        tag_variant: "info".into(),
                        tag_icon: "globe".into(),
                        ..TableCell::default()
                    },
                    TableCell {
                        text: "10.0.0.1/24".into(),
                        sub: "fd00:0:0:1::1/64".into(),
                        copy: true,
                        emphasis: true,
                        ..TableCell::default()
                    },
                    TableCell::zone("lan"),
                    TableCell { text: "tcp/udp".into(), ..TableCell::default() },
                    TableCell {
                        text: "accept".into(),
                        variant: "success".into(),
                        dot: true,
                        icon: "check".into(),
                        ..TableCell::default()
                    },
                    TableCell { text: "1284".into(), key: "hits:lan".into(), ..TableCell::default() },
                    TableCell { on: true, name: "cfg02zone".into(), ..TableCell::default() },
                    TableCell { text: "Details".into(), href: "/zones/lan".into(), ..TableCell::default() },
                    TableCell { button: "Edit".into(), disabled: true, ..TableCell::default() },
                ],
                drawer: Some(RowDrawer {
                    title: "Edit zone — lan".into(),
                    size: "wide".into(),
                    hide_title: true,
                    open: true,
                    children: vec![Widget::form(
                        "Save",
                        vec![Widget::field("name", "Name", "lan", "", "")],
                    )],
                }),
            },
            TableRow {
                id: "cfg03zone".into(),
                cells: vec![
                    TableCell::default(),
                    TableCell { text: "wan".into(), ..TableCell::default() },
                    TableCell { text: "203.0.113.7".into(), muted: true, ..TableCell::default() },
                    TableCell {
                        endpoints: vec![
                            TableEndpoint { kind: "device".into(), label: "10.0.0.30".into() },
                            TableEndpoint { kind: "any".into(), label: "any".into() },
                        ],
                        ..TableCell::default()
                    },
                    TableCell { text: "icmp".into(), ..TableCell::default() },
                    TableCell { text: "reject".into(), variant: "warning".into(), ..TableCell::default() },
                    TableCell { text: "0".into(), ..TableCell::default() },
                    TableCell::default(),
                    TableCell::default(),
                    TableCell { button: "Edit".into(), disabled: true, ..TableCell::default() },
                ],
                ..TableRow::default()
            },
        ],
        drawer_label: "Edit".into(),
        drawer_icon: "pencil".into(),
        empty_text: "No zones yet.".into(),
    }
}
