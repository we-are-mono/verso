// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

//! Every device under a policy, on one page.
//!
//! The place a policy is *made* is the device's own panel, on the Devices page,
//! because that is where the device is. This page is the other question — which
//! devices are under one at all — and no listing of devices can answer it: a
//! curfew set months ago is invisible until something gathers them. So the rows
//! state and do not edit, and each ends at the device it is about.

use verso_plugin::{Envelope, TableCell, TableColumn, TableRow, Widget};

use crate::leases::Leases;
use crate::model::{Policy, DAYS};

const TITLE: &str = "Device limits";

const SUB: &str = "Devices kept off the internet, held to a curfew, or capped. Everything else \
runs unrestricted, which is most of them.";

const EMPTY: &str = "No device is limited. Open one on the Devices page and its limits are the \
second tab.";

const EM_DASH: &str = "—";

/// page is the listing. It carries no editor of its own: a policy is a thing
/// about a device, and the device's panel is where it is edited.
pub fn page(policies: &[Policy], leases: &Leases) -> Envelope {
    let rows: Vec<TableRow> = policies.iter().map(|p| row(p, leases)).collect();
    Envelope::page(
        TITLE,
        Widget::stack(vec![Widget::Table {
            style: String::new(),
            title: String::new(),
            detail: String::new(),
            dense: false,
            align: String::new(),
            reorder_config: String::new(),
            reorder_label: String::new(),
            columns: columns(),
            rows,
            drawer_label: String::new(),
            drawer_icon: String::new(),
            empty_text: EMPTY.into(),
            add_label: String::new(),
            add_href: String::new(),
            note: String::new(),
            stream: None,
        }]),
    )
    .with_subheading(SUB)
    .with_width("wide")
}

fn columns() -> Vec<TableColumn> {
    [
        ("Device", "name"),
        ("MAC", "mono"),
        ("Internet", "pill"),
        ("When", "text"),
        ("Down", "mono"),
        ("Up", "mono"),
    ]
    .iter()
    .map(|(label, kind)| TableColumn {
        label: (*label).into(),
        kind: (*kind).into(),
        ..TableColumn::default()
    })
    .collect()
}

fn row(policy: &Policy, leases: &Leases) -> TableRow {
    TableRow {
        id: policy.mac.clone(),
        cells: vec![
            text(&leases.subject(&policy.mac)),
            TableCell {
                text: policy.mac.clone(),
                copy: true,
                emphasis: true,
                ..TableCell::default()
            },
            verdict(policy),
            text(&when(policy)),
            text(&rate(&policy.down)),
            text(&rate(&policy.up)),
        ],
        ..TableRow::default()
    }
}

/// verdict is what happens to this device's traffic, in the pill vocabulary:
/// refused outright is the danger tone, a curfew the warning, and a device that
/// is simply capped is not refused at all.
fn verdict(policy: &Policy) -> TableCell {
    let (text, variant) = match (policy.allowed, policy.scheduled) {
        (false, _) => ("blocked", "danger"),
        (true, true) => ("on a schedule", "warning"),
        (true, false) => ("allowed", "success"),
    };
    TableCell {
        text: text.into(),
        variant: variant.into(),
        ..TableCell::default()
    }
}

/// when is the curfew in words: the hours, and the days as a run rather than a
/// list of seven abbreviations nobody reads.
pub fn when(policy: &Policy) -> String {
    if !policy.scheduled {
        return String::new();
    }
    format!("{}–{} · {}", policy.from, policy.to, days(&policy.days))
}

/// days folds consecutive days into a run — "Mon–Fri" rather than five names —
/// and says "every day" for the whole week, because that is what it means.
fn days(on: &[String]) -> String {
    let mut indexes: Vec<usize> = DAYS
        .iter()
        .enumerate()
        .filter(|(_, day)| on.iter().any(|held| held == *day))
        .map(|(index, _)| index)
        .collect();
    indexes.sort_unstable();
    if indexes.len() == DAYS.len() {
        return "every day".into();
    }
    let mut runs: Vec<String> = Vec::new();
    let mut start = 0;
    while start < indexes.len() {
        let mut end = start;
        while end + 1 < indexes.len() && indexes[end + 1] == indexes[end] + 1 {
            end += 1;
        }
        runs.push(match end - start >= 2 {
            true => format!("{}–{}", DAYS[indexes[start]], DAYS[indexes[end]]),
            false => indexes[start..=end]
                .iter()
                .map(|index| DAYS[*index])
                .collect::<Vec<_>>()
                .join(", "),
        });
        start = end + 1;
    }
    runs.join(", ")
}

/// rate states a cap the way the field takes it, with its unit.
fn rate(mbit: &str) -> String {
    match mbit.is_empty() {
        true => String::new(),
        false => format!("{mbit} Mbit/s"),
    }
}

fn text(value: &str) -> TableCell {
    if value.is_empty() {
        return TableCell {
            text: EM_DASH.into(),
            muted: true,
            ..TableCell::default()
        };
    }
    TableCell {
        text: value.into(),
        ..TableCell::default()
    }
}
