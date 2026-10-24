// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

//! What stands between a figure and its unit, every unit, the percent and the
//! degree included: a narrow no-break space, so "80 MHz" reads as one word and
//! never breaks at a line's end (DESIGN.md, the Narrow Space Rule). It is the
//! shell's `units.Space`, for the figures a plugin writes itself. A duration
//! or a rate is not one of them: a plugin hands it to the shell as a number
//! (a cell's or a fact's `duration`, a cell's `rate`), and the shell writes it.

/// SPACE is the narrow no-break space, U+202F.
pub const SPACE: &str = "\u{202f}";

/// figure writes value and its unit with SPACE between them.
pub fn figure(value: impl std::fmt::Display, unit: &str) -> String {
    format!("{value}{SPACE}{unit}")
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn a_unit_stands_a_narrow_no_break_space_after_its_figure() {
        assert_eq!(SPACE, "\u{202f}");
        assert_eq!(figure(80, "MHz"), "80\u{202f}MHz");
        assert_eq!(figure("2.4", "GHz"), "2.4\u{202f}GHz");
        assert_eq!(figure(61, "%"), "61\u{202f}%");
    }
}
