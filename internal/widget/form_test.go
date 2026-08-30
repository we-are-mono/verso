// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"strings"
	"testing"
)

func TestFormDecodesActions(t *testing.T) {
	w, err := Decode([]byte(`{"type":"form","submit":"Save",` +
		`"actions":[{"label":"Generate keypair","action":"generate-keypair"}],"fields":[]}`))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	f, ok := w.(*Form)
	if !ok {
		t.Fatalf("Decode returned %T, want *Form", w)
	}
	if len(f.Actions) != 1 || f.Actions[0].Action != "generate-keypair" || f.Actions[0].Label != "Generate keypair" {
		t.Errorf("actions not decoded: %+v", f.Actions)
	}
}

// TestFormRendersSecondaryActions: a secondary action renders as a submit button
// carrying its _action marker, so clicking it submits the whole form for the plugin
// to compute on (ADR-005 §7).
func TestFormRendersSecondaryActions(t *testing.T) {
	f := &Form{
		Submit:  "Save",
		Actions: []FormAction{{Label: "Generate keypair", Action: "generate-keypair"}},
		Fields:  []Widget{&Field{Name: "k", Label: "Key"}},
	}
	got := render(t, newRenderer(t), f)
	for _, want := range []string{
		">Save</button>",
		`name="_action" value="generate-keypair"`, ">Generate keypair</button>",
		"active:translate-y-px active:shadow-none motion-reduce:active:translate-y-0",
		"dark:hover:border-gray-600 dark:hover:bg-gray-800 dark:hover:text-gray-200",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("form render missing %q", want)
		}
	}
}
