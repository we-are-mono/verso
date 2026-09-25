// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

//! The form vocabulary this plugin's panels are built out of.
//!
//! A rule, a port forward and a zone are edited in the panel beside the listing
//! they belong to — there is no page for any of them — and all three are made of
//! the same handful of shapes: a labelled value, a closed choice, a list of
//! tokens, a row of them side by side, and the one act that cannot be undone.
//!
//! They live here rather than in any one panel so that a value typed into a rule
//! and the same value typed into a zone are the same control. A panel composes
//! these; it does not spell its own.

use verso_plugin::{Form, SelectOption, Widget};

use crate::rule_form::{Errors, RuleForm};

/// REMOVE_FIELD names the subject of a confirmed act from a listing.
pub const REMOVE_FIELD: &str = "_remove";

/// DELETE_FIELD marks a legacy delete form rather than an editor submission.
pub const DELETE_FIELD: &str = "_delete";

/// PANEL_FIELD marks a submission as the open panel's own. A panel is edited at
/// its listing's address, and a row's power act posts to that same address while
/// the panel is open, so the address cannot say which of the two arrived: read
/// by the address, the act came in as the panel's form and wrote a rule stripped
/// of everything but its switch. The panel's form says what it is itself, the
/// way the delete form does with DELETE_FIELD, and a submission carrying neither
/// is never read as a save, whatever the address names.
pub const PANEL_FIELD: &str = "_panel";

/// from_panel reports whether a submission is the open panel's — its editing
/// form or its delete form — rather than something posted to the panel's address
/// from the listing behind it.
pub fn from_panel(form: &Form) -> bool {
    form.get(PANEL_FIELD) == "1" || form.get(DELETE_FIELD) == "1"
}

/// panel_form is a panel's editing form: the controls, the one act that submits
/// them, and — after the controls, where it takes no place in the layout — the
/// marker that names the submission as the panel's own. Every panel composes
/// its form here so none can post without the marker and be mistaken for a
/// flip on the listing behind it. The commit row carries the verb and nothing
/// about applying: the panel saves into the stage and closes, and what applying
/// costs is the bar's to say.
pub fn panel_form(submit: &str, mut fields: Vec<Widget>) -> Widget {
    fields.push(Widget::hidden(PANEL_FIELD, "1"));
    Widget::Form {
        style: String::new(),
        submit: submit.into(),
        error: String::new(),
        fields,
        note: String::new(),
        target: String::new(),
    }
}

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

/// delete_form is an editor's one irreversible action, kept in a form of its own
/// so the page form above it can never carry it by accident. The confirm holds
/// the submit, so this form draws no Save of its own.
pub fn delete_form(action: &str, message: &str) -> Widget {
    Widget::Form {
        style: String::new(),
        submit: String::new(),
        error: String::new(),
        fields: vec![
            Widget::hidden(DELETE_FIELD, "1"),
            Widget::Confirm {
                trigger: action.into(),
                title: String::new(),
                message: message.into(),
                confirm: action.into(),
                cancel: String::new(),
            },
        ],
        note: String::new(),
        target: String::new(),
    }
}

/// subject names the thing an editor is about to delete: what its operator
/// called it, or what kind of thing it is when nobody named it.
pub fn subject(name: &str, unnamed: &str) -> String {
    match name.is_empty() {
        true => unnamed.to_string(),
        false => format!("“{name}”"),
    }
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

/// text_field is one typed value, carrying whatever the last submission got
/// wrong about it.
pub fn text_field(name: &str, label: &str, value: &str, help: &str, errors: &Errors) -> Widget {
    Widget::Field {
        name: name.into(),
        label: label.into(),
        kind: "text".into(),
        advanced: false,
        value: value.into(),
        values: Vec::new(),
        placeholder: String::new(),
        datatype: String::new(),
        options: Vec::new(),
        error: errors.get(name).into(),
        help: help.into(),
        key: String::new(),
        tip: String::new(),
        source: String::new(),
        unit: String::new(),
        style: String::new(),
        remove: String::new(),
        pair: None,
        target: String::new(),
    }
}

/// select_field is one choice over a closed set.
pub fn select_field(
    name: &str,
    label: &str,
    value: &str,
    options: Vec<SelectOption>,
    errors: &Errors,
) -> Widget {
    Widget::select(name, label, value, options, errors.get(name))
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
    Widget::List {
        name: name.into(),
        label: label.into(),
        kind: "text".into(),
        style: "tokens".into(),
        prompt: prompt.into(),
        datatype: String::new(),
        items: items.to_vec(),
        errors: errors.list(name),
        help: help.into(),
        key: String::new(),
        tip: String::new(),
        options: Vec::new(),
        remove: String::new(),
        target: String::new(),
    }
}
