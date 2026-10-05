// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"strings"
	"testing"
)

// TestACardsSubtitleMayBeAMachineString: an artifact card's subtitle is
// usually a sentence of what the thing is for, but it may instead name the
// thing as the machine does (an SFP module's cage, "xfi0") — then it is set
// in mono, as every verbatim string is, and never put through a catalog.
func TestACardsSubtitleMayBeAMachineString(t *testing.T) {
	card := &Card{Style: "artifact", Title: "SFP+ module", Subtitle: "xfi0", SubtitleMono: true}
	translateSchema(card, func(s string) string { return "translated " + s })
	if card.Subtitle != "xfi0" {
		t.Errorf("a machine-string subtitle went through the catalog: %q", card.Subtitle)
	}
	got := render(t, newRenderer(t), card)
	if !strings.Contains(got, `<p class="font-mono text-base leading-6 font-medium text-body">xfi0</p>`) {
		t.Errorf("a machine-string subtitle is not set in mono:\n%s", got)
	}
	sentence := render(t, newRenderer(t), &Card{Style: "artifact", Title: "HTTPS certificate", Subtitle: "What this router shows browsers."})
	if !strings.Contains(sentence, `<p class="text-sm leading-6 text-body">What this router shows browsers.</p>`) {
		t.Errorf("a sentence subtitle lost its sans:\n%s", sentence)
	}
}
