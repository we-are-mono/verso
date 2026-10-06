// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestAnEmbedSetsAContributionIntoTheStack: a plugin's contribution to a shell
// page arrives drawn, and the shell sets it into the page's own stack as one of
// its blocks, so the stack spaces it as it spaces its sections.
func TestAnEmbedSetsAContributionIntoTheStack(t *testing.T) {
	got := render(t, newRenderer(t), &Stack{Children: []Widget{
		&Section{Title: "Router password"},
		&Embed{HTML: `<section data-verso-section="ruled"><h2>SSH</h2></section>`},
	}})
	stack := strings.Index(got, "verso-stack")
	ssh := strings.Index(got, `<section data-verso-section="ruled"><h2>SSH</h2></section>`)
	if stack < 0 || ssh < stack {
		t.Fatalf("the contribution is not a block of the page's stack:\n%s", got)
	}
}

// TestAPluginCannotSendAnEmbed: the embed carries HTML the shell drew, so it is
// the shell's alone — a plugin's schema cannot name one.
func TestAPluginCannotSendAnEmbed(t *testing.T) {
	if _, err := Decode(json.RawMessage(`{"type":"embed","html":"<script>alert(1)</script>"}`)); err == nil {
		t.Fatal("a plugin schema decoded into an embed")
	}
}
