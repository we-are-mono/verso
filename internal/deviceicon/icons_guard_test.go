// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package deviceicon_test

import (
	"testing"

	"github.com/we-are-mono/verso/internal/deviceicon"
	"github.com/we-are-mono/verso/internal/widget"
)

// TestEveryIconRenders guards the two curated data files against a typo: every
// icon name they (or the fallback) can emit must be a real glyph in the widget
// icon set, so a contribution can never resolve a device to a blank/generic
// stand-in without a test failing first.
func TestEveryIconRenders(t *testing.T) {
	for _, name := range deviceicon.Icons() {
		if !widget.HasIcon(name) {
			t.Errorf("device icon %q is not defined in internal/widget/icons.go", name)
		}
	}
}
