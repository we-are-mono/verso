// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"strings"
	"testing"
)

// TestRenderSectionSub: the description belongs to the heading — tight beneath
// the title (mb-1 instead of mb-3), with the saved space moved below the sub so
// the title-to-content distance is preserved. Markdown renders; without a sub
// the title keeps its original spacing.
func TestRenderSectionSub(t *testing.T) {
	r := newRenderer(t)
	got := render(t, r, &Section{
		Title:    "Zones",
		Sub:      "**Input** governs traffic to the router.",
		Children: []Widget{&Text{Markdown: "body"}},
	})
	for _, want := range []string{
		"mb-1",                   // title pulled tight to its description
		"mb-6",                   // the gap moves below the sub
		"<strong>Input</strong>", // sub renders Markdown
		"text-slate-500",         // sub is muted head-matter, not body prose
	} {
		if !strings.Contains(got, want) {
			t.Errorf("section sub missing %q:\n%s", want, got)
		}
	}

	plain := render(t, r, &Section{Title: "Zones", Children: []Widget{&Text{Markdown: "body"}}})
	if !strings.Contains(plain, "mb-3") || strings.Contains(plain, "mb-6") {
		t.Errorf("section without sub should keep its original title spacing:\n%s", plain)
	}
}

func TestRenderSectionMeta(t *testing.T) {
	r := newRenderer(t)
	got := render(t, r, &Section{
		Title:     "Time and region",
		MetaLabel: "Current time",
		Meta:      "2026-08-29 22:14:08",
		MetaIcon:  "clock",
		Children:  []Widget{&Text{Markdown: "body"}},
	})
	for _, want := range []string{
		"justify-between",
		"Current time",
		"font-semibold text-slate-700",
		"2026-08-29 22:14:08",
		lucideIcons["clock"],
	} {
		if !strings.Contains(got, want) {
			t.Errorf("section meta missing %q:\n%s", want, got)
		}
	}
}

func TestRenderSectionInlineMeta(t *testing.T) {
	got := render(t, newRenderer(t), &Section{
		Title: "Time and region", MetaLabel: "Current time",
		Meta: "2026-08-30 12:30:00", MetaPosition: "inline",
	})
	for _, want := range []string{"Time and region", "Current time", "2026-08-30 12:30:00", `aria-hidden="true">·</span>`} {
		if !strings.Contains(got, want) {
			t.Errorf("inline section meta missing %q:\n%s", want, got)
		}
	}
}

func TestRenderSectionControl(t *testing.T) {
	r := newRenderer(t)
	w, err := Decode([]byte(`{"type":"section","title":"Rule","control":{"type":"switch","name":"enabled","label":"Enabled","off_label":"Disabled","style":"inline","on":true},"children":[]}`))
	if err != nil {
		t.Fatalf("decode section control: %v", err)
	}
	got := render(t, r, w)
	for _, want := range []string{"Rule", "Enabled", "Disabled", `name="enabled"`, "verso-inline-switch inline-flex", "size-1 rounded-full", "gap-3"} {
		if !strings.Contains(got, want) {
			t.Errorf("section control missing %q:\n%s", want, got)
		}
	}
	if _, err := Decode([]byte(`{"type":"section","control":{"type":"nope"},"children":[]}`)); err == nil {
		t.Error("unknown section control should fail loudly")
	}
}

func TestRenderSectionFlush(t *testing.T) {
	got := render(t, newRenderer(t), &Section{Title: "Rule", Flush: true})
	if strings.Contains(got, `class="pt-6`) {
		t.Errorf("flush section must rely on its parent's inset: %s", got)
	}
}

func TestRenderSectionHairline(t *testing.T) {
	r := newRenderer(t)
	plain := render(t, r, &Section{Title: "Default", Children: []Widget{&Text{Markdown: "body"}}})
	if strings.Contains(plain, "border-slate-200") {
		t.Errorf("section should omit its hairline by default:\n%s", plain)
	}

	divided := render(t, r, &Section{
		Title:    "Divided",
		Hairline: true,
		Children: []Widget{&Text{Markdown: "body"}},
	})
	if !strings.Contains(divided, "border-slate-200") {
		t.Errorf("section with hairline=true should include divider styling:\n%s", divided)
	}
}
