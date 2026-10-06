// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Heading actions align to the section's right content edge, including when
// wrapped below the title. They keep a field's height, centred on the title's
// 20px line and reaching past it, so the title stays on the grid's line.
func TestSectionControlAlignsToContentEdge(t *testing.T) {
	got := render(t, newRenderer(t), &Section{
		Title: "Time", Meta: "2026-09-23 12:00", MetaPosition: "inline",
		Control:  &Button{Label: "Use my computer's time", Style: "act", Name: "_action", Value: "clock"},
		Children: []Widget{&Field{Name: "zonename", Label: "Time zone"}},
	})
	column := `<span class="ml-auto flex h-5 w-full items-center justify-end sm:w-auto [&>button]:h-control [&>button]:gap-2 [&>button]:px-4">`
	at := strings.Index(got, column)
	if at < 0 {
		t.Fatalf("the heading's control must align to the content edge:\n%s", got)
	}
	title := strings.Index(got, "</h2>")
	meta := strings.Index(got, "data-verso-section-meta")
	if !(title < meta && meta < at) || !strings.Contains(got[at:], "Use my computer") {
		t.Errorf("title and meta lead, followed by the heading action:\n%s", got)
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
	if got := render(t, r, &Section{Children: row()}); !strings.Contains(got, `<section data-verso-section="plain" data-verso-headless`) {
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

// TestListRowsTakeTheirHeightFromPadding: a list's values are rows, and a row's
// height is its line plus its padding, never a fixed floor — the value's 20px
// line with 10px over it and 9px under it plus the hairline above every row, a
// row of two cells, the 28px remove riding the line. The first row's hairline
// is the label's own line, so the set pulls up a pixel and gives it back at
// its foot, and only when it holds something; the add box stands centred in
// the two cells after it, so a set with nothing in it puts its box directly
// under the label.
func TestListRowsTakeTheirHeightFromPadding(t *testing.T) {
	got := render(t, newRenderer(t), &List{Name: "server", Label: "Time servers", Style: "rows", Prompt: "Add a server",
		Items: []string{"0.openwrt.pool.ntp.org", "1.openwrt.pool.ntp.org"}})
	const row = `class="flex items-start gap-2 border-t border-rule pt-2.5 pr-3 pb-2.25 pl-1.5 first:border-t-transparent"`
	if strings.Count(got, row) != 3 { // two values and the row the shell clones
		t.Errorf("value rows take their height from padding, want %s three times in:\n%s", row, got)
	}
	// Each value leads with the packet in its words' ink, centred on its first
	// line and in the grid's first column, the value starting at the next; the
	// values are divided from each other, never from the label.
	const packet = `<span aria-hidden="true" class="mt-1.75 size-1.5 shrink-0 rounded-[1px] bg-meta"></span>`
	const value = `class="min-w-0 flex-1 wrap-anywhere font-mono text-base leading-5 text-meta"`
	if strings.Count(got, packet) != 3 || strings.Count(got, value) != 3 {
		t.Errorf("each value leads with the packet, its words quiet:\n%s", got)
	}
	if strings.Contains(got, "min-h-8") {
		t.Errorf("no fixed row floor:\n%s", got)
	}
	if !strings.Contains(got, `<div data-verso-list-items class="has-[>*]:-mt-px has-[>*]:mb-px">`) || !strings.Contains(got, `<div class="flex items-center gap-2 py-0.75">`) {
		t.Errorf("the set takes the label's line for its first hairline only when it holds something, and the add box is centred in its two cells:\n%s", got)
	}
}

// TestRenderSectionSub: the description belongs to the heading band and keeps
// its rendered Markdown. The band's air is the stylesheet's: the band keeps no
// margin of its own, lede or not, and whatever stands first under it stands a
// cell under it — a field brings that cell itself, so does a form of fields;
// anything else is given it.
func TestRenderSectionSub(t *testing.T) {
	r := newRenderer(t)
	got := render(t, r, &Section{
		Title:    "Zones",
		Sub:      "**Input** governs traffic to the router.",
		Children: []Widget{&Text{Markdown: "body"}},
	})
	for _, want := range []string{
		"data-verso-section-lede", // description stays inside the band
		"<strong>Input</strong>",  // sub renders Markdown
		"text-body",               // sub is muted head-matter, not body prose
	} {
		if !strings.Contains(got, want) {
			t.Errorf("section sub missing %q:\n%s", want, got)
		}
	}
	css, err := os.ReadFile(filepath.Join("..", "server", "assets", "sections.css"))
	if err != nil {
		t.Fatal(err)
	}
	sheet := string(css)
	if strings.Contains(sheet, "[data-verso-section-band]:has([data-verso-section-lede]) { margin-bottom") {
		t.Error("a band with a lede keeps a margin of its own; the block under it carries the cell")
	}
	const first = "section[data-verso-section]:has(> [data-verso-section-band]) > .verso-rhythm > :not(input[type=\"hidden\"], [hidden]):not(:not(input[type=\"hidden\"], [hidden]) ~ *):not(.verso-field-row, .verso-form-grid, .verso-conditional, form:has(.verso-field-row, .verso-form-grid, .verso-conditional), [data-verso-change-field], [data-verso-settings]) {\n    margin-top: calc(var(--spacing) * 5);"
	if !strings.Contains(sheet, first) {
		t.Error("the first block under a band stands a cell under it unless it brings that cell itself (a field, a form of fields)")
	}

	plain := render(t, r, &Section{Title: "Zones", Children: []Widget{&Text{Markdown: "body"}}})
	if band := plain[strings.Index(plain, "data-verso-section-band"):]; strings.Contains(band[:strings.Index(band, ">")], "mb-") {
		t.Errorf("a section without a sub stands its content straight on under the title:\n%s", plain)
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
	if !strings.Contains(got, `data-verso-section="bare"`) {
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
	// Its stop short of the viewport's edge is the stylesheet's, for every
	// section with an id (sections.css).
	if !strings.Contains(got, `data-verso-section="plain" id="section-speed"`) {
		t.Errorf("anchored section missing its id:\n%s", got)
	}
	plain := render(t, r, &Section{Title: "Speed"})
	if strings.Contains(plain, "id=") {
		t.Errorf("a section nobody points at takes no address:\n%s", plain)
	}
}

// A kicker names the list under it. It is set at the meta size in caps, because a
// second display heading beside a rail's list of headings reads as one of them.
func TestRenderSectionKicker(t *testing.T) {
	r := newRenderer(t)
	got := render(t, r, &Section{Title: "On this page", Kicker: true})
	if !strings.Contains(got, `<h2 class="text-xs leading-5 font-medium tracking-[.08em] text-meta uppercase">On this page</h2>`) {
		t.Errorf("kicker is not set as one:\n%s", got)
	}
	if strings.Contains(got, "text-lg") {
		t.Errorf("a kicker is not a heading:\n%s", got)
	}
	// The list it names stands a cell under it.
	if !strings.Contains(got, `<div class="flex flex-wrap items-center justify-between gap-x-6 mb-5">`) {
		t.Errorf("a kicker keeps a cell of air under it:\n%s", got)
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
	if !strings.Contains(divided, `data-verso-section="ruled"`) {
		t.Errorf("section with hairline=true should include divider styling:\n%s", divided)
	}
}
