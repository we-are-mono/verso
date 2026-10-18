// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

//! The table cells every listing speaks the same way.
//!
//! A value the config does not state reads as one quiet dash on every page, and
//! a state reads as one pill, so a listing that spelled either differently would
//! read as a different kind of thing.

use crate::TableCell;

/// EM_DASH stands for a value the config or the device does not state.
pub const EM_DASH: &str = "—";

/// text_cell is a plain value, or a quiet dash when there is none.
pub fn text_cell(text: &str) -> TableCell {
    if text.is_empty() {
        return muted_cell(EM_DASH);
    }
    TableCell {
        text: text.into(),
        ..TableCell::default()
    }
}

/// muted_cell is a value that stands back from the row: a dash, a zero, a
/// default the row does not need read first.
pub fn muted_cell(text: &str) -> TableCell {
    TableCell {
        text: text.into(),
        muted: true,
        ..TableCell::default()
    }
}

/// pill_cell is a state or an enum value as a pill led by its square, in the
/// tone `variant` names. An empty value renders as the column's faint dash,
/// which is what keeps the filled pills meaningful.
pub fn pill_cell(text: &str, variant: &str) -> TableCell {
    TableCell {
        text: text.into(),
        variant: variant.into(),
        dot: true,
        ..TableCell::default()
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use serde_json::json;

    fn wire(cell: TableCell) -> serde_json::Value {
        serde_json::to_value(cell).expect("serialize")
    }

    #[test]
    fn a_stated_value_is_plain_and_a_missing_one_a_quiet_dash() {
        assert_eq!(wire(text_cell("tcp")), json!({"text": "tcp"}));
        assert_eq!(wire(text_cell("")), json!({"text": "—", "muted": true}));
    }

    #[test]
    fn a_pill_carries_its_tone_and_square() {
        assert_eq!(
            wire(pill_cell("ACCEPT", "success")),
            json!({"text": "ACCEPT", "variant": "success", "dot": true})
        );
    }
}
