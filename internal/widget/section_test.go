// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"strings"
	"testing"
)

// TestSectionControlStandsInTheFieldColumn: the control on a section's heading
// line acts on what the section holds, so it stands in the column its fields'
// controls do — the same width, hard right — rather than trailing the title.
// A button there stands where a field would, so it takes a field's size (a
// primary's footprint) while keeping its own dress.
func TestSectionControlStandsInTheFieldColumn(t *testing.T) {
	got := render(t, newRenderer(t), &Section{
		Title: "Time", Meta: "2026-09-23 12:00", MetaPosition: "inline",
		Control:  &Button{Label: "Use my computer's time", Style: "act", Name: "_action", Value: "clock"},
		Children: []Widget{&Field{Name: "zonename", Label: "Time zone"}},
	})
	column := `<span class="flex w-full items-center sm:w-64 [&>button]:h-9 [&>button]:gap-2 [&>button]:px-4">`
	at := strings.Index(got, column)
	if at < 0 {
		t.Fatalf("the heading's control is not in the field column:\n%s", got)
	}
	title := strings.Index(got, "</h2>")
	meta := strings.Index(got, "data-verso-section-meta")
	if !(title < meta && meta < at) || !strings.Contains(got[at:], "Use my computer") {
		t.Errorf("title and meta lead, the control stands in the field column after them:\n%s", got)
	}
}

// TestHeadlessSectionStartsAtItsFirstBox: a section with no heading has
// nothing for its first row's top padding to stand off from, so it is marked
// headless and the stylesheet takes that padding back — the first box starts
// where the section does. A titled section keeps it, as the gap after its
// heading.
func TestHeadlessSectionStartsAtItsFirstBox(t *testing.T) {
	r := newRenderer(t)
	row := func() []Widget { return []Widget{&Field{Name: "hostname", Label: "Router name"}} }
	if got := render(t, r, &Section{Children: row()}); !strings.Contains(got, "<section data-verso-headless") {
		t.Errorf("an untitled section is marked headless:\n%s", got)
	}
	if got := render(t, r, &Section{Title: "Time", Children: row()}); strings.Contains(got, "data-verso-headless") {
		t.Errorf("a titled section keeps its first row's air:\n%s", got)
	}
}

// TestListAddHoversAsEverySecondaryButton: a list's Add is a secondary button
// and answers the pointer as every other does — the subsection act's hover.
func TestListAddHoversAsEverySecondaryButton(t *testing.T) {
	got := render(t, newRenderer(t), &List{Name: "server", Label: "Time servers", Style: "rows", Prompt: "Add a server"})
	at := strings.Index(got, "data-verso-list-add")
	if at < 0 {
		t.Fatalf("the list lost its Add:\n%s", got)
	}
	add := got[at:]
	add = add[:strings.Index(add, ">")]
	if !strings.Contains(add, "hover:border-sand-5 hover:bg-rule") || strings.Contains(add, "hover:bg-quiet") {
		t.Errorf("Add hovers as every secondary button: %s", add)
	}
}

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
		"mb-2",                   // title pulled tight to its description
		"mb-5",                   // the gap moves below the sub
		"<strong>Input</strong>", // sub renders Markdown
		"text-body",              // sub is muted head-matter, not body prose
	} {
		if !strings.Contains(got, want) {
			t.Errorf("section sub missing %q:\n%s", want, got)
		}
	}

	plain := render(t, r, &Section{Title: "Zones", Children: []Widget{&Text{Markdown: "body"}}})
	if !strings.Contains(plain, "mb-5") {
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
		"font-semibold text-body",
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

// A section a link elsewhere on the page points at has to be addressable, and has
// to stop short of the viewport's top edge when someone is sent to it. Without the
// id the rail beside a long page is a list of links to nowhere, which is what the
// firewall's settings rail was.
func TestRenderSectionAnchor(t *testing.T) {
	r := newRenderer(t)
	// The id is namespaced: a plugin's anchor is often the name of the setting
	// the section holds, and a section and its field sharing an id leave the
	// field's label pointing at the section.
	got := render(t, r, &Section{Title: "Speed", Anchor: "speed"})
	for _, want := range []string{`id="section-speed"`, "scroll-mt-16 md:scroll-mt-0"} {
		if !strings.Contains(got, want) {
			t.Errorf("anchored section missing %q:\n%s", want, got)
		}
	}
	plain := render(t, r, &Section{Title: "Speed"})
	if strings.Contains(plain, "id=") || strings.Contains(plain, "scroll-mt") {
		t.Errorf("a section nobody points at takes no address:\n%s", plain)
	}
}

// A kicker names the list under it. It is set at the meta size in caps, because a
// second display heading beside a rail's list of headings reads as one of them.
func TestRenderSectionKicker(t *testing.T) {
	r := newRenderer(t)
	got := render(t, r, &Section{Title: "On this page", Kicker: true})
	if !strings.Contains(got, `<h2 class="text-xs font-medium tracking-[.08em] text-meta uppercase">On this page</h2>`) {
		t.Errorf("kicker is not set as one:\n%s", got)
	}
	if strings.Contains(got, "text-lg") {
		t.Errorf("a kicker is not a heading:\n%s", got)
	}
}

func TestRenderSectionHairline(t *testing.T) {
	r := newRenderer(t)
	plain := render(t, r, &Section{Title: "Default", Children: []Widget{&Text{Markdown: "body"}}})
	if strings.Contains(plain, "border-t border-rule") {
		t.Errorf("section should omit its hairline by default:\n%s", plain)
	}

	divided := render(t, r, &Section{
		Title:    "Divided",
		Hairline: true,
		Children: []Widget{&Text{Markdown: "body"}},
	})
	if !strings.Contains(divided, "border-rule") {
		t.Errorf("section with hairline=true should include divider styling:\n%s", divided)
	}
}
