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

// TestConditionalKeepsARowsAir: a gate is a field like any other, and brings
// its own cell of air over its label's line as every field does (forms.css);
// the block around it adds no air of its own, and a field-set it reveals is
// more rows of the form, each with its own cell of air.
func TestConditionalKeepsARowsAir(t *testing.T) {
	got := render(t, newRenderer(t), &Conditional{
		Name: "reflection", Label: "Also works from inside your network",
		Fields: Widgets{&Field{Name: "port", Label: "Port"}},
	})
	if !strings.Contains(got, `<div class="verso-conditional" data-verso-change-field`) {
		t.Errorf("the block adds no air of its own: %s", got)
	}
	if !strings.Contains(got, `<div class="verso-conditional-body verso-rhythm">`) {
		t.Errorf("a revealed field-set stands on its rows' own air, not a margin: %s", got)
	}
	gate := got[strings.Index(got, "verso-conditional-gate"):]
	gate = gate[:strings.Index(gate, `"`)]
	if strings.Contains(gate, "py-0") || strings.Contains(gate, "pt-0") {
		t.Errorf("the gate gives up its field's cell of air (%q), so nothing stands it off what is above: %s", gate, got)
	}
}

// TestAGateWithNothingToRevealIsJustARow: a gate whose switch reveals no
// fields draws no reveal under it, which would hang its 20px standoff under
// the row when switched on and push the section's end away (a zone's
// Advanced reading, ending on automatic helpers).
func TestAGateWithNothingToRevealIsJustARow(t *testing.T) {
	got := render(t, newRenderer(t), &Conditional{Name: "auto_helper", Label: "Assign connection helpers automatically", Checked: true})
	if strings.Contains(got, "verso-conditional-body") {
		t.Errorf("a gate with nothing to reveal draws an empty reveal: %s", got)
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
