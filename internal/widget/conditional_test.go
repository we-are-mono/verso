// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"strings"
	"testing"
)

func TestConditionalDecodeAndRender(t *testing.T) {
	js := `{"type":"conditional","name":"use_psk","label":"Use a pre-shared key","checked":true,
		"fields":[{"type":"field","name":"psk","label":"Pre-shared key","value":"SECRET"}]}`
	w, err := Decode([]byte(js))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	c, ok := w.(*Conditional)
	if !ok {
		t.Fatalf("Decode returned %T, want *Conditional", w)
	}
	if c.Name != "use_psk" || c.Label != "Use a pre-shared key" || !c.Checked || len(c.Fields) != 1 {
		t.Fatalf("conditional = %+v", c)
	}

	got := render(t, newRenderer(t), c)
	for _, want := range []string{
		"verso-conditional",                              // the shell-owned wrapper the CSS targets
		`type="checkbox"`, `name="use_psk"`, `value="on"`, // the controlling toggle
		"checked", "Use a pre-shared key",
		"verso-conditional-body", // the gated field-set the CSS shows/hides
		`name="psk"`,             // the gated field rendered
	} {
		if !strings.Contains(got, want) {
			t.Errorf("conditional render missing %q", want)
		}
	}
}

// TestConditionalUncheckedOmitsChecked: an off toggle renders no `checked`, so the
// shell's CSS starts the field-set hidden.
func TestConditionalUncheckedOmitsChecked(t *testing.T) {
	w, err := Decode([]byte(`{"type":"conditional","name":"t","label":"L","checked":false,"fields":[]}`))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if got := render(t, newRenderer(t), w); strings.Contains(got, "checked") {
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
