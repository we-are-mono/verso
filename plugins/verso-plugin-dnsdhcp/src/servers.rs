// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.
use verso_plugin::{dhcp, Request, Table, TableCell, TableRow, TableRowAct, Widget};

pub fn listing(r: &Request) -> Widget {
    let rows = dhcp::servers(&r.snapshot, &r.ubus)
        .iter()
        .map(|server| TableRow {
            id: server.network.clone(),
            cells: vec![
                TableCell {
                    text: server.network.clone(),
                    href: server.href(),
                    ..Default::default()
                },
                TableCell {
                    text: server.label().into(),
                    variant: server.tone().into(),
                    dot: true,
                    ..Default::default()
                },
                TableCell {
                    text: server
                        .leases
                        .map(|n| n.to_string())
                        .unwrap_or_else(|| "—".into()),
                    ..Default::default()
                },
                TableCell {
                    actions: vec![TableRowAct {
                        icon: "square-pen".into(),
                        title: "Edit".into(),
                        href: server.href(),
                        ..Default::default()
                    }],
                    ..Default::default()
                },
            ],
            ..Default::default()
        })
        .collect();
    let mut table = crate::page::table(
        crate::page::columns(&[
            ("Network", "reference"),
            ("DHCP server", "status"),
            ("Active leases", "count"),
            ("", "actions"),
        ]),
        rows,
        "No networks configured. Add a network in Interfaces to configure its DHCP server.",
    );
    if let Widget::Table(Table { style, .. }) = &mut table {
        *style = "live".into();
    }
    table
}
