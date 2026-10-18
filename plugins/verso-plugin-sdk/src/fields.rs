// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

//! The form shapes a row drawer is built from, and the markers that tell its
//! submission apart from an act on the listing behind it.
//!
//! An object is edited in the panel beside the listing it belongs to, at the
//! listing's own address, so a value typed into one panel and the same value
//! typed into another are the same control, and every panel names its
//! submission the same way.

use crate::{Errors, Field, Form, SelectOption, Widget};

/// REMOVE_FIELD names the subject of a confirmed act from a listing.
pub const REMOVE_FIELD: &str = "_remove";

/// PANEL_FIELD marks a submission as the open panel's own. A panel is edited at
/// its listing's address, and a row's power act posts to that same address while
/// the panel is open, so the address cannot say which of the two arrived: read
/// by the address, the act would come in as the panel's form and write a record
/// stripped of everything but its switch. The panel's form says what it is
/// itself, and a submission without it is never read as a save, whatever the
/// address names.
pub const PANEL_FIELD: &str = "_panel";

/// from_panel reports whether a submission is the open panel's editing form
/// rather than something posted to the panel's address from the listing behind
/// it.
pub fn from_panel(form: &Form) -> bool {
    form.get(PANEL_FIELD) == "1"
}

/// panel_form is a panel's editing form: the controls, the one act that submits
/// them, and — after the controls, where it takes no place in the layout — the
/// marker that names the submission as the panel's own. A panel composes its
/// form here so it cannot post without the marker and be mistaken for a flip on
/// the listing behind it. The commit row carries the verb and nothing about
/// applying: the panel saves into the stage and closes.
pub fn panel_form(submit: &str, mut fields: Vec<Widget>) -> Widget {
    fields.push(Widget::hidden(PANEL_FIELD, "1"));
    Widget::form(submit, fields)
}

/// text_field is one typed value, carrying whatever the last submission got
/// wrong about it.
pub fn text_field(name: &str, label: &str, value: &str, help: &str, errors: &Errors) -> Widget {
    Widget::Field(Field {
        name: name.into(),
        label: label.into(),
        kind: "text".into(),
        value: value.into(),
        error: errors.get(name).into(),
        help: help.into(),
        ..Default::default()
    })
}

/// select_field is one choice over a closed set, carrying whatever the last
/// submission got wrong about it.
pub fn select_field(
    name: &str,
    label: &str,
    value: &str,
    options: Vec<SelectOption>,
    errors: &Errors,
) -> Widget {
    Widget::select(name, label, value, options, errors.get(name))
}

#[cfg(test)]
mod tests {
    use super::*;
    use serde_json::json;

    fn wire(widget: Widget) -> serde_json::Value {
        serde_json::to_value(widget).expect("serialize")
    }

    #[test]
    fn a_panel_form_names_itself_after_its_controls() {
        let form = wire(panel_form("Save", vec![Widget::hidden("a", "b")]));
        let names: Vec<_> = form["fields"]
            .as_array()
            .expect("fields")
            .iter()
            .map(|f| (f["name"].clone(), f["value"].clone()))
            .collect();
        assert_eq!(
            names,
            vec![(json!("a"), json!("b")), (json!("_panel"), json!("1"))]
        );
        assert!(from_panel(&Form::parse("_panel=1&name=x")));
        assert!(!from_panel(&Form::parse("allow_ping=off")));
    }

    #[test]
    fn a_field_carries_the_message_its_value_earned() {
        let mut errors = Errors::default();
        errors.field("port", "Not a port.");
        let field = wire(text_field(
            "port",
            "Port",
            "x",
            "Where it listens.",
            &errors,
        ));
        assert_eq!(
            (&field["value"], &field["error"], &field["help"]),
            (
                &json!("x"),
                &json!("Not a port."),
                &json!("Where it listens.")
            )
        );
        let pick = wire(select_field(
            "port",
            "Port",
            "x",
            vec![SelectOption::new("x", "X")],
            &errors,
        ));
        assert_eq!(
            (&pick["kind"], &pick["error"]),
            (&json!("select"), &json!("Not a port."))
        );
    }
}
