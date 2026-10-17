// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"strings"
	"testing"
)

// TestAStateIsAMark: every 6px state square is the mark block (ADR-025), its
// tone named rather than painted, so one rule in blocks/mark.css draws every
// state wherever it stands; a live success breathes, a chip's packet never
// does.
func TestAStateIsAMark(t *testing.T) {
	r := newRenderer(t)
	for _, c := range []struct {
		name string
		w    Widget
		want string
	}{
		{"a resting status", &Badge{Plain: true, Dot: true, Variant: "success", Text: "Connected"}, `class="verso-mark" data-verso-tone="success" data-verso-live`},
		{"a resting status of another word", &Badge{Plain: true, Dot: true, Variant: "neutral", Text: "Idle"}, `class="verso-mark" data-verso-tone="muted"`},
		{"a chip's packet", &Badge{Dot: true, Variant: "warning", Text: "reject"}, `class="verso-mark" data-verso-tone="warning"`},
		{"a fact's mark", &Properties{Items: []Property{{Label: "Signed by", Value: "The router", Dot: "warning"}}}, `class="verso-mark mt-2.25 mr-2" data-verso-tone="warning"`},
		{"a notice's mark", &Callout{Variant: "danger", Title: "Down"}, `class="verso-mark absolute top-4 left-2.25" data-verso-tone="danger"`},
	} {
		if got := render(t, r, c.w); !strings.Contains(got, c.want) {
			t.Errorf("%s: want %s in:\n%s", c.name, c.want, got)
		}
	}
	if chip := render(t, r, &Badge{Dot: true, Variant: "success", Text: "accept"}); strings.Contains(chip, "data-verso-live") {
		t.Errorf("a chip's packet breathes:\n%s", chip)
	}
}
