// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

//! The form vocabulary this plugin's panels are built out of, beyond the
//! labelled value, the closed choice and the panel form the SDK gives every
//! plugin.
//!
//! A rule, a port forward and a zone are edited in the panel beside the listing
//! they belong to — there is no page for any of them — and all three are made of
//! the same handful of shapes: a labelled value, a closed choice, a list of
//! tokens, a row of them side by side, and the one act that cannot be undone.
//!
//! They live here rather than in any one panel so that a value typed into a rule
//! and the same value typed into a zone are the same control. A panel composes
//! these; it does not spell its own.

use verso_plugin::{text_field, List, Widget};

use crate::rule_form::{Errors, RuleForm};

/// name_field is what the rule is called — the string every listing, hit count
/// and log line identifies it by.
pub fn name_field(rule: &RuleForm, errors: &Errors) -> Widget {
    text_field(
        "name",
        "Name",
        &rule.name,
        "Identifies the rule in listings, hit counts, and the system log.",
        errors,
    )
}

// ---- the controls the editor and its conditions share ----

/// row_group is a run of rows that belong together — a range's two ends, a
/// condition's include and exclude. It adds nothing: every row already owns the
/// form's full measure and its own air, so the group exists to say the rows are
/// one thought, not to lay them out. It used to be a grid, which put two
/// full-measure rows side by side in half the width each and broke both.
pub fn row_group(children: Vec<Widget>) -> Widget {
    Widget::Stack {
        width: String::new(),
        compact: false,
        inline: false,
        divided: false,
        flush: true,
        children,
    }
}

/// token_list is many short values under one name, each removable on its own and
/// each able to report its own error.
pub fn token_list(
    name: &str,
    label: &str,
    prompt: &str,
    help: &str,
    items: &[String],
    errors: &Errors,
) -> Widget {
    Widget::List(List {
        name: name.into(),
        label: label.into(),
        kind: "text".into(),
        style: "tokens".into(),
        prompt: prompt.into(),
        items: items.to_vec(),
        errors: errors.list(name),
        help: help.into(),
        ..Default::default()
    })
}
