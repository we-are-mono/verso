// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"strings"
	"testing"
)

func TestConditionalDecodeAndRender(t *testing.T) {
	js := `{"type":"conditional","name":"use_psk","label":"Use a pre-shared key","checked":true,
		"fields":[{"type":"field","name":"psk","label":"Pre-shared key","value":"SECRET"}],
		"otherwise":[{"type":"text","markdown":"No key will be used."}]}`
	w, err := Decode([]byte(js))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	c, ok := w.(*Conditional)
	if !ok {
		t.Fatalf("Decode returned %T, want *Conditional", w)
	}
	if c.Name != "use_psk" || c.Label != "Use a pre-shared key" || !c.Checked || len(c.Fields) != 1 || len(c.Otherwise) != 1 {
		t.Fatalf("conditional = %+v", c)
	}

	got := render(t, newRenderer(t), c)
	for _, want := range []string{
		"verso-conditional",                              // the shell-owned wrapper the CSS targets
		`type="checkbox"`, `value="1"`, `name="use_psk"`, // the controlling switch
		"data-verso-switch", "peer-checked:bg-choice", // same control as table toggle cells
		"checked", "Use a pre-shared key",
		"verso-conditional-body", // the gated field-set the CSS shows/hides
		`name="psk"`,             // the gated field rendered
		"verso-conditional-otherwise", "No key will be used.",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("conditional render missing %q", want)
		}
	}
	// The gate keeps a real label bound to its checkbox by id.
	if !strings.Contains(got, `<label for="use_psk" id="use_psk-label" class="text-sm font-semibold text-ink">Use a pre-shared key</label>`) {
		t.Errorf("conditional gate must label its switch at the form measure: %s", got)
	}
}

// TestConditionalKeepsARowsAir: a gate is a row, and stands off what is around
// it as every row does; trimming a section's first and last rows is the
// section's to do (sections.css), so a gate opening a titled section keeps its
// 16px under the band as a checkbox row there does.
func TestConditionalKeepsARowsAir(t *testing.T) {
	got := render(t, newRenderer(t), &Conditional{Name: "reflection", Label: "Also works from inside your network"})
	if !strings.Contains(got, `<div class="verso-conditional py-4" data-verso-change-field`) {
		t.Errorf("a gate's own padding is a row's, untrimmed: %s", got)
	}
}

func TestConditionalUnknownOtherwiseFieldFails(t *testing.T) {
	js := `{"type":"conditional","name":"t","label":"L","fields":[],"otherwise":[{"type":"bogus"}]}`
	if _, err := Decode([]byte(js)); err == nil {
		t.Fatal("Decode: want error for an unknown alternate field")
	}
}

// TestConditionalUncheckedOmitsChecked: an off toggle renders no `checked`, so the
// shell's CSS starts the field-set hidden.
func TestConditionalUncheckedOmitsChecked(t *testing.T) {
	w, err := Decode([]byte(`{"type":"conditional","name":"t","label":"L","checked":false,"fields":[]}`))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if got := render(t, newRenderer(t), w); strings.Contains(got, " checked") {
		t.Errorf("an off toggle must not render `checked`: %s", got)
	}
}

// TestConditionalUnknownFieldFails: a gated field of an unknown type fails the
// decode loudly, like any other nested widget.
func TestConditionalUnknownFieldFails(t *testing.T) {
	js := `{"type":"conditional","name":"t","label":"L","fields":[{"type":"bogus"}]}`
	if _, err := Decode([]byte(js)); err == nil {
		t.Fatal("Decode: want error for an unknown gated field")
	}
}
