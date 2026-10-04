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
    ActionTab, CollectionAdd, CollectionItem, CollectionRemove, ColumnWidth, ConditionItem,
    Property, RemoveConfirm, RowDrawer, SelectOption, SettingsItem, SettingsPill, SettingsSeam,
    SettingsToggle, TableAction, TableCell, TableColumn, TableEndpoint, TableGroup, TableRow,
    TableRowAct, TableStream, Tone, Widget, STREAM_FIREWALL_LOG,
};

#[test]
fn write_widget_fixtures() {
    let samples: Vec<(&str, Widget)> = vec![
        (
            "when",
            Widget::When {
                name: "proto".into(),
                value: "static".into(),
                active: true,
                children: vec![Widget::field(
                    "ipaddr",
                    "Address",
                    "192.168.1.1",
                    "ipaddr",
                    "",
                )],
            },
        ),
        (
            "card",
            Widget::card("Tunnel", vec![Widget::text("One tunnel, two peers.")]),
        ),
        (
            "section",
            Widget::Section {
                title: "Rule".into(),
                icon: String::new(),
                anchor: "rule".into(),
                kicker: false,
                sub: "Devices that may connect.".into(),
                meta: "3 peers".into(),
                meta_icon: "shield".into(),
                meta_position: "inline".into(),
                flush: true,
                hairline: false,
                target: "firewall.cfg0a1b2c".into(),
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
                flush: true,
                children: vec![Widget::text("a"), Widget::text("b")],
            },
        ),
        (
            "conditional",
            Widget::Conditional {
                name: "ntp_enabled".into(),
                label: "Set the time automatically".into(),
                key: "ntp.enabled".into(),
                help: "Off means the clock is set by hand.".into(),
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
                vec![verso_plugin::SelectOption {
                    value: "UTC".into(),
                    label: "UTC".into(),
                }],
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
                label: String::new(),
                help: String::new(),
                join: String::new(),
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
                on: true,
                key: String::new(),
                tip: String::new(),
                source: String::new(),
                verbatim: false,
                error: String::new(),
                target: String::new(),
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
                        group: "Endpoints".into(),
                        hint: "space-separated, or a range".into(),
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
                        group: "Endpoints".into(),
                        hint: "survives a changed lease".into(),
                        active: false,
                        children: vec![Widget::tokens(
                            "src_mac",
                            "Include",
                            "MAC address",
                            &[],
                            "",
                        )],
                    },
                ],
            },
        ),
        (
            "disclosure",
            Widget::Disclosure {
                style: "condition".into(),
                summary: "Parameters for advanced actions".into(),
                open: false,
                children: vec![Widget::text("Only the selected action's parameters apply.")],
            },
        ),
        (
            "link",
            Widget::Link {
                desc: String::new(),
                code: String::new(),
                label: "Add rule".into(),
                icon: "plus".into(),
                href: "/plugins/firewall/rules/new".into(),
                style: "button".into(),
                act: String::new(),
                panel: false,
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
                note: String::new(),
                target: "system.cfg01e48a".into(),
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
                key: "hostname".into(),
                tip: "The name this router answers to on the local network.".into(),
                source: "system system".into(),
                unit: String::new(),
                style: "segmented".into(),
                remove: String::new(),
                target: String::new(),
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
                key: "system.ntp.server".into(),
                tip: "The clocks this router asks for the time.".into(),
                options: vec![SelectOption::new("0.openwrt.pool.ntp.org", "OpenWrt pool")],
                remove: "yes".into(),
                target: String::new(),
            },
        ),
        (
            "callout",
            Widget::callout(
                Tone::Warning,
                "Not reachable",
                "The tunnel has no endpoint yet.",
            ),
        ),
        ("code", Widget::code("Public key", "hLIgo9xNzJM=")),
        // The same block declared as its form's preview: the shell keeps it
        // current as the form is edited, so the flag has to survive the decoder.
        (
            "code-live",
            Widget::preview(
                "/etc/config/firewall",
                "config zone 'lan'\n\toption input 'ACCEPT'\n",
            ),
        ),
        (
            "empty",
            Widget::empty(
                "shield",
                "No tunnels yet",
                "Create one to reach home from anywhere.",
                vec![],
            ),
        ),
        (
            "properties",
            Widget::Properties {
                items: vec![
                    Property {
                        label: "Endpoint".into(),
                        value: "203.0.113.7:51820".into(),
                        mono: true,
                        verbatim: false,
                        copy: true,
                        ..Property::default()
                    }
                    .toned(Tone::Success)
                    .marked(Tone::Success)
                    .noted("Reached over the internet."),
                    Property {
                        label: "Valid".into(),
                        ..Property::default()
                    }
                    .spanning("2026-08-31", "2027-10-02", 6, Tone::Success),
                ],
                align: "left".into(),
            },
        ),
        (
            "badge",
            Widget::Badge {
                variant: Tone::Success,
                text: "up".into(),
                dot: true,
            },
        ),
        ("text", Widget::text("A short explanatory line.")),
        (
            "confirm",
            Widget::Confirm {
                trigger: "Remove peer".into(),
                title: String::new(),
                message: "The device loses access immediately.".into(),
                confirm: String::new(),
                cancel: String::new(),
            },
        ),
        (
            "collection",
            Widget::Collection {
                items: vec![CollectionItem {
                    title: "me@laptop".into(),
                    detail: "SHA256:UPedI7axeQlxL8dlMkeSROduLmrflVzNxJkm5UNiHxw".into(),
                    remove: Some(CollectionRemove {
                        name: "_key_remove".into(),
                        value: "SHA256:UPedI7axeQlxL8dlMkeSROduLmrflVzNxJkm5UNiHxw".into(),
                        confirm: RemoveConfirm {
                            trigger: "Remove".into(),
                            icon: "trash-2".into(),
                            title: "Remove this key?".into(),
                            message: "It can no longer sign in over SSH.".into(),
                            ..RemoveConfirm::default()
                        },
                    }),
                }],
                empty: "No keys are authorized.".into(),
                add: Some(CollectionAdd {
                    label: "Add a key".into(),
                    name: "authorized_key".into(),
                    submit: "Add key".into(),
                    preview: Some(Box::new(Widget::preview("Reads as", ""))),
                    ..CollectionAdd::default()
                }),
            },
        ),
        (
            "raw",
            Widget::raw(
                "WireGuard keeps a silent tunnel silent — an idle peer can be perfectly healthy.",
            ),
        ),
        ("table", listing()),
        (
            "settings",
            Widget::Settings {
                condensed: false,
                style: "card".into(),
                title: "Global defaults".into(),
                meta: "firewall.@defaults[0]".into(),
                items: vec![
                    SettingsItem {
                        title: "Default policies".into(),
                        desc: "What happens to traffic that matches no rule.".into(),
                        code: "input · output · forward".into(),
                        pills: vec![
                            SettingsPill {
                                variant: Tone::Warning,
                                text: "in reject".into(),
                            },
                            SettingsPill {
                                variant: Tone::Success,
                                text: "out accept".into(),
                            },
                            SettingsPill {
                                variant: Tone::Warning,
                                text: "fwd reject".into(),
                            },
                        ],
                        ..SettingsItem::default()
                    },
                    SettingsItem {
                        title: "Drop invalid packets".into(),
                        desc: "Discard packets that belong to no known connection.".into(),
                        code: "drop_invalid".into(),
                        toggle: Some(SettingsToggle {
                            name: "drop_invalid".into(),
                            on: true,
                        }),
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
            Widget::Filter {
                placeholder: "Filter rules, zones, and ports…".into(),
            },
        ),
        (
            "actionbar",
            Widget::ActionBar {
                style: String::new(),
                tabs: vec![
                    ActionTab {
                        label: "All".into(),
                        count: 22,
                        active: true,
                        ..ActionTab::default()
                    },
                    ActionTab {
                        label: "IPv6".into(),
                        count: 19,
                        matches: "ipv6".into(),
                        active: false,
                    },
                ],
                filter: "Find a rule".into(),
                live: "Pause".into(),
                action: Some(TableAction {
                    label: "Add rule".into(),
                    href: "/plugins/firewall/rules/new".into(),
                    ..TableAction::default()
                }),
                opens_panel: false,
                drawer: None,
            },
        ),
        ("stream-table", live_listing()),
        (
            "button",
            Widget::Button {
                label: "Pause".into(),
                icon: "pause".into(),
                style: "secondary".into(),
                name: "_action".into(),
                value: "pause".into(),
                disabled: false,
                loading: false,
                live: true,
            },
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
        dense: true,
        reorder_config: "firewall".into(),
        reorder_label: "Rule order".into(),
        columns: vec![
            TableColumn {
                kind: "reorder".into(),
                ..TableColumn::default()
            },
            TableColumn {
                label: "Zone".into(),
                kind: "name".into(),
                width: ColumnWidth::Name,
            },
            TableColumn {
                label: "Address".into(),
                kind: "addr".into(),
                ..TableColumn::default()
            },
            TableColumn {
                label: "From".into(),
                kind: "endpoint".into(),
                ..TableColumn::default()
            },
            TableColumn {
                label: "Protocol".into(),
                kind: "keyword".into(),
                ..TableColumn::default()
            },
            TableColumn {
                label: "Input".into(),
                kind: "pill".into(),
                ..TableColumn::default()
            },
            TableColumn {
                label: "Hits".into(),
                kind: "num".into(),
                ..TableColumn::default()
            },
            TableColumn {
                label: "Airtime busy".into(),
                kind: "meter".into(),
                width: ColumnWidth::Name,
            },
            TableColumn {
                kind: "toggle".into(),
                ..TableColumn::default()
            },
            TableColumn {
                kind: "link".into(),
                ..TableColumn::default()
            },
            TableColumn {
                kind: "pill".into(),
                ..TableColumn::default()
            },
            TableColumn {
                kind: "actions".into(),
                ..TableColumn::default()
            },
        ],
        rows: vec![
            TableRow {
                depth: 0,
                expanded: Vec::new(),
                id: "cfg02zone".into(),
                key: "zone:lan".into(),
                group: Some(TableGroup {
                    key: "input_lan".into(),
                    label: "LAN".into(),
                    to: "Router".into(),
                    chain: String::new(),
                    tally: "2 rules".into(),
                    add_label: "Add rule to LAN → Router".into(),
                    add_text: "Add rule".into(),
                    add_href: "/firewall/rules?open=new&src=lan".into(),
                    add_panel: true,
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
                    TableCell {
                        text: "tcp/udp".into(),
                        ..TableCell::default()
                    },
                    TableCell {
                        text: "accept".into(),
                        variant: "success".into(),
                        dot: true,
                        icon: "check".into(),
                        ..TableCell::default()
                    },
                    TableCell {
                        text: "1284".into(),
                        key: "hits:lan".into(),
                        ..TableCell::default()
                    },
                    TableCell {
                        text: "61 %".into(),
                        fill: 61,
                        variant: "warning".into(),
                        ..TableCell::default()
                    },
                    TableCell {
                        on: true,
                        name: "cfg02zone".into(),
                        ..TableCell::default()
                    },
                    TableCell {
                        text: "Details".into(),
                        href: "/zones/lan".into(),
                        ..TableCell::default()
                    },
                    TableCell {
                        button: "Edit".into(),
                        disabled: true,
                        ..TableCell::default()
                    },
                    TableCell {
                        actions: vec![
                            TableRowAct {
                                icon: "square-pen".into(),
                                title: "Edit".into(),
                                href: "/zones/lan/edit".into(),
                                ..TableRowAct::default()
                            },
                            // An act that posts rather than leads: the row's own
                            // state, flipped where it stands.
                            TableRowAct {
                                icon: "power-off".into(),
                                title: "Disable lan".into(),
                                name: "cfg02zone".into(),
                                value: "off".into(),
                                ..TableRowAct::default()
                            },
                            TableRowAct {
                                icon: "trash-2".into(),
                                title: "Delete zone".into(),
                                name: "_remove".into(),
                                value: "cfg02zone".into(),
                                confirm_title: "Delete zone “%s”?".into(),
                                confirm: "Its networks will use the global defaults when applied."
                                    .into(),
                                ..TableRowAct::default()
                            },
                        ],
                        ..TableCell::default()
                    },
                ],
                muted: false,
                tags: vec!["ipv4".into(), "ipv6".into()],
                drawer: Some(RowDrawer {
                    title: "Edit zone — lan".into(),
                    open: true,
                    children: vec![Widget::form(
                        "Save",
                        vec![Widget::field("name", "Name", "lan", "", "")],
                    )],
                    ..RowDrawer::default()
                }),
                panel: String::new(),
            },
            TableRow {
                depth: 0,
                expanded: Vec::new(),
                id: "cfg03zone".into(),
                cells: vec![
                    TableCell::default(),
                    TableCell {
                        text: "wan".into(),
                        ..TableCell::default()
                    },
                    TableCell {
                        text: "203.0.113.7".into(),
                        muted: true,
                        ..TableCell::default()
                    },
                    TableCell {
                        endpoints: vec![
                            TableEndpoint {
                                kind: "device".into(),
                                label: "10.0.0.30".into(),
                            },
                            TableEndpoint {
                                kind: "any".into(),
                                label: "any".into(),
                            },
                        ],
                        ..TableCell::default()
                    },
                    TableCell {
                        text: "icmp".into(),
                        ..TableCell::default()
                    },
                    TableCell {
                        text: "reject".into(),
                        variant: "warning".into(),
                        ..TableCell::default()
                    },
                    TableCell {
                        text: "0".into(),
                        ..TableCell::default()
                    },
                    TableCell::default(),
                    TableCell::default(),
                    TableCell::default(),
                    TableCell {
                        button: "Edit".into(),
                        disabled: true,
                        ..TableCell::default()
                    },
                    TableCell::default(),
                ],
                muted: true,
                ..TableRow::default()
            },
        ],
        drawer_label: "Edit".into(),
        drawer_icon: "pencil".into(),
        empty_text: "No zones yet.".into(),
        add_label: String::new(),
        add_href: String::new(),
        note: "Source rewrites are not listed here.".into(),
        stream: None,
    }
}

// live_listing is the streaming table fixture: the same vocabulary declared as
// a run of events rather than a state of the config, so the stream's own wire
// shape is pinned to the shell's decoder like every other field.
fn live_listing() -> Widget {
    Widget::Table {
        style: "lined".into(),
        title: String::new(),
        detail: String::new(),
        dense: true,
        reorder_config: String::new(),
        reorder_label: String::new(),
        columns: vec![
            TableColumn {
                label: "When".into(),
                kind: "runtime".into(),
                ..TableColumn::default()
            },
            TableColumn {
                label: "Verdict".into(),
                kind: "pill".into(),
                ..TableColumn::default()
            },
            TableColumn {
                label: "Source".into(),
                kind: "mono".into(),
                ..TableColumn::default()
            },
        ],
        rows: vec![],
        drawer_label: String::new(),
        drawer_icon: String::new(),
        empty_text: "Waiting for the first logged event…".into(),
        add_label: String::new(),
        add_href: String::new(),
        note: String::new(),
        stream: Some(TableStream {
            source: STREAM_FIREWALL_LOG.into(),
            ring: 200,
        }),
    }
}
