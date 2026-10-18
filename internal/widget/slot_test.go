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

func TestAnOpenedSlotDecodesWithItsKind(t *testing.T) {
	for _, kind := range []string{"block", "act", "notice"} {
		w, err := Decode([]byte(`{"type":"slot","id":"certificate.acts","kind":"` + kind + `"}`))
		if err != nil {
			t.Fatal(err)
		}
		if slot, ok := w.(*Slot); !ok || slot.Kind != kind || !slot.Opened() {
			t.Errorf("decoded %#v; want an opened %s slot", w, kind)
		}
	}
	shell, _ := Decode([]byte(`{"type":"slot","id":"appearance"}`))
	if shell.(*Slot).Opened() {
		t.Error("a slot with no kind is the shell's block, not an opened one")
	}
	odd, _ := Decode([]byte(`{"type":"slot","id":"x","kind":"fact"}`))
	if odd.(*Slot).Opened() {
		t.Error("a slot widget of a kind it cannot hold is no opened slot")
	}
}

func TestAFactsListDecodesTheSlotItOpens(t *testing.T) {
	w, err := Decode([]byte(`{"type":"properties","items":[{"label":"Issued to","value":"a"}],"slot":"certificate.source"}`))
	if err != nil {
		t.Fatal(err)
	}
	if p, ok := w.(*Properties); !ok || p.Slot != "certificate.source" {
		t.Fatalf("decoded %#v; want a facts list opening certificate.source", w)
	}
}

func TestJoinedFactsStandAfterTheListsOwnInTheirOwnWords(t *testing.T) {
	catalog := map[string]string{"Issued to": "Izdano za", "Source": "Vir"}
	tr := func(s string) string {
		if v, ok := catalog[s]; ok {
			return v
		}
		return s
	}
	list := &Properties{Items: []Property{{Label: "Issued to", Value: "router.example.com"}}, Slot: "certificate.source"}
	// A contributed row arrives already in its plugin's words; the host's
	// catalog must not touch it again.
	list.Join([]Property{{Label: "Source", Value: "Your domain"}})
	var b strings.Builder
	if err := newRenderer(t).RenderWithToken(&b, list, "", "sl", tr); err != nil {
		t.Fatal(err)
	}
	got := b.String()
	own, joined := strings.Index(got, "Izdano za"), strings.Index(got, "Your domain")
	if own < 0 || joined < 0 || joined < own {
		t.Fatalf("render = %q; want the joined row after the list's own", got)
	}
	if strings.Contains(got, "Vir") {
		t.Error("the host's catalog translated a joined row")
	}
	if len(list.Items) != 1 {
		t.Errorf("joining changed the plugin's own rows: %v", list.Items)
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
