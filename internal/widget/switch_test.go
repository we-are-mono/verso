// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"strings"
	"testing"
)

func TestDecodeAndRenderSwitch(t *testing.T) {
	w, err := Decode([]byte(`{"type":"switch","name":"enabled","label":"Enabled","help":"Applies immediately.","on":true}`))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	got := render(t, newRenderer(t), w)
	for _, want := range []string{
		`type="checkbox"`, `data-verso-switch`, `name="enabled"`, " checked",
		"Enabled", "Applies immediately.", "peer-checked:bg-emerald-500",
		"items-center gap-2", "pl-9 text-sm text-slate-500",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("switch missing %q:\n%s", want, got)
		}
	}
	if control, label := strings.Index(got, "data-verso-switch"), strings.Index(got, ">Enabled</span>"); control < 0 || label < 0 || control > label {
		t.Errorf("default switch must precede its static label: %s", got)
	}
	inline := render(t, newRenderer(t), &Switch{Name: "enabled", Label: "Enabled", OffLabel: "Disabled", Style: "inline", On: true})
	for _, want := range []string{"verso-inline-switch inline-flex cursor-pointer", "size-1 rounded-full bg-slate-300", "ml-3 inline-flex translate-y-px", "verso-inline-switch-on", "Enabled", "verso-inline-switch-off", "Disabled", "text-slate-900"} {
		if !strings.Contains(inline, want) {
			t.Errorf("inline switch missing %q: %s", want, inline)
		}
	}
}
