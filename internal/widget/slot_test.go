// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"strings"
	"testing"
)

func TestASlotDecodesWithItsID(t *testing.T) {
	w, err := Decode([]byte(`{"type":"slot","id":"appearance"}`))
	if err != nil {
		t.Fatal(err)
	}
	slot, ok := w.(*Slot)
	if !ok || slot.ID != "appearance" {
		t.Fatalf("decoded %#v; want a slot named appearance", w)
	}
}

func TestASlotRendersWhatTheShellFillsItWith(t *testing.T) {
	r := newRenderer(t)
	slot := &Slot{ID: "appearance"}
	if got := strings.TrimSpace(render(t, r, slot)); got != "" {
		t.Errorf("an unfilled slot renders %q; want nothing", got)
	}
	slot.Fill(&Text{Markdown: "Theme goes here"})
	if got := render(t, r, slot); !strings.Contains(got, "Theme goes here") {
		t.Errorf("a filled slot renders %q", got)
	}
}

func TestSlotsFindsEverySlotInAPage(t *testing.T) {
	page, err := Decode([]byte(`{"type":"form","submit":"Save settings","fields":[
		{"type":"section","title":"","children":[{"type":"field","name":"hostname","label":"Router name"}]},
		{"type":"slot","id":"appearance"},
		{"type":"section","title":"Time","children":[{"type":"slot","id":"other"}]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, s := range Slots(page) {
		ids = append(ids, s.ID)
	}
	if strings.Join(ids, " ") != "appearance other" {
		t.Errorf("Slots = %v; want appearance and other, in page order", ids)
	}
}
