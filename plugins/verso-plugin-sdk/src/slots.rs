// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

//! Named slots (ADR-026): the places on pages that plugins fill from their
//! manifest's `contributes`. The shell's own are listed here, the catalogue a
//! plugin reads to know where it may stand; a plugin's page opens its own with
//! [`Widget::open_slot`](crate::Widget::open_slot), named under its id.

use serde::Serialize;

/// What a slot takes: the shell checks every contribution's answer against
/// its slot's kind and draws none of another.
#[derive(Serialize, Debug, Clone, Copy, PartialEq, Eq)]
#[serde(rename_all = "kebab-case")]
pub enum SlotKind {
    /// A section, card or table standing in a page's stack.
    Block,
    /// One act in an act row: a link with an icon, which may open the
    /// contributor's page in a drawer.
    Act,
    /// Rows of a facts list, joining the host's own.
    Fact,
    /// One callout.
    Notice,
    /// An Overview card: its header and a body.
    OverviewCard,
}

/// One slot a shell page opens: its name, its kind, and the items in it a
/// contribution may stand before or after, in page order.
#[derive(Serialize, Debug)]
pub struct ShellSlot {
    pub name: &'static str,
    pub kind: SlotKind,
    pub anchors: &'static [&'static str],
}

/// Access's blocks: the password, the signed-in sessions, where the web
/// interface answers, and LuCI's part.
pub const ACCESS_BLOCKS: &str = "shell.access.blocks";

/// The Overview's cards.
pub const OVERVIEW_CARDS: &str = "shell.overview.cards";

/// Every slot the shell's pages open.
pub const SHELL_SLOTS: &[ShellSlot] = &[
    ShellSlot {
        name: ACCESS_BLOCKS,
        kind: SlotKind::Block,
        anchors: &["password", "sessions", "web", "luci"],
    },
    ShellSlot {
        name: OVERVIEW_CARDS,
        kind: SlotKind::OverviewCard,
        anchors: &[
            "internet",
            "ports",
            "tunnels",
            "traffic",
            "resources",
            "system",
            "hardware",
        ],
    },
];
