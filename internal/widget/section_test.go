// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"strings"
	"testing"
)

// Heading actions align to the section's right content edge, including when
// wrapped below the title, and keep the compact act's size wherever they
// stand. A fact on the heading belongs to its act (when it was checked, the
// clock it sets), so it stands right in front of it, with no dot after the
// title; a heading with no act keeps its fact beside the title.
func TestSectionControlAlignsToContentEdge(t *testing.T) {
	got := render(t, newRenderer(t), &Section{
		Title: "Time", Meta: "2026-09-23 12:00", MetaPosition: "inline",
		Control:  &Button{Label: "Use my computer's time", Style: "act", Name: "_action", Value: "clock"},
		Children: []Widget{&Field{Name: "zonename", Label: "Time zone"}},
	})
	column := `<span class="verso-heading__act">`
	at := strings.Index(got, column)
	if at < 0 {
		t.Fatalf("the heading's control must align to the content edge:\n%s", got)
	}
	title := strings.Index(got, "</h2>")
	meta := strings.Index(got, "data-verso-section-meta")
	button := strings.Index(got, "Use my computer")
	if !(title < at && at < meta && meta < button) {
		t.Errorf("the title leads; the fact stands in the act, in front of its button:\n%s", got)
	}
	if strings.Contains(got, "verso-heading__sep") {
		t.Errorf("a fact standing in front of the act keeps no dot after the title:\n%s", got)
	}
	bare := render(t, newRenderer(t), &Section{Title: "Time", Meta: "2026-09-23 12:00", MetaPosition: "inline"})
	if !strings.Contains(bare, "verso-heading__sep") || strings.Contains(bare, "verso-heading__act") {
		t.Errorf("a heading with no act keeps its fact beside the title:\n%s", bare)
	}
	// At the line's end, the full width when it wraps under the title, its
	// own width beside it; on the title's 24px line, the fact 12px in front
	// of the button.
	declares(t, "heading", ".verso-heading__act",
		"justify-content: flex-end;", "gap: calc(var(--spacing) * 3);", "width: 100%;", "height: calc(var(--spacing) * 6);", "margin-left: auto;")
	if rule := cssRuleWithin(blockCSS(t, "heading"), "@media (width >= 40rem)", ".verso-heading__act"); !strings.Contains(rule, "width: auto;") {
		t.Errorf("a heading's act beside the title keeps its own width:\n%s", rule)
	}
	// No rule grows a heading's button past the compact act it is.
	if strings.Contains(blockCSS(t, "button"), "verso-heading__act") {
		t.Error("blocks/button.css resizes a heading act's button")
	}
}

// TestSectionActPostsWithTheSectionsForm: a card that saves itself holds its
// form, and the act on its heading stands outside it; the act still posts
// with that form — its fields and its value — through the form's id. A
// section with no form, or with two, leaves its act as it was.
func TestSectionActPostsWithTheSectionsForm(t *testing.T) {
	act := func() Widget {
		return &Button{Label: "Use my computer's time", Style: "act", Name: "_action", Value: "clock"}
	}
	got := render(t, newRenderer(t), &Section{
		Title: "Time", Control: act(),
		Children: []Widget{&Form{Submit: "Save time settings", Fields: []Widget{&Field{Name: "zonename", Label: "Time zone"}}}},
	})
	id := tagAttr(got, `<form`, `id="`, `"`)
	if id == "" {
		t.Fatalf("the section's form carries no id:\n%s", got)
	}
	if !strings.Contains(got, `name="_action" value="clock" form="`+id+`"`) {
		t.Errorf("the heading's act does not post with the form %q:\n%s", id, got)
	}
	for name, s := range map[string]*Section{
		"no form": {Title: "Time", Control: act(), Children: []Widget{&Field{Name: "zonename", Label: "Time zone"}}},
		"two forms": {Title: "Time", Control: act(), Children: []Widget{
			&Form{Submit: "Save one", Fields: []Widget{&Field{Name: "a", Label: "A"}}},
			&Form{Submit: "Save two", Fields: []Widget{&Field{Name: "b", Label: "B"}}},
		}},
	} {
		if got := render(t, newRenderer(t), s); strings.Contains(got, `form="`) {
			t.Errorf("%s: the act is tied to a form it cannot tell apart:\n%s", name, got)
		}
	}
}

// tagAttr is the text after open, then after start, up to end: an attribute
// of the first element open begins.
func tagAttr(s, open, start, end string) string {
	at := strings.Index(s, open)
	if at < 0 {
		return ""
	}
	tag := s[at:]
	if close := strings.Index(tag, ">"); close >= 0 {
		tag = tag[:close]
	}
	from := strings.Index(tag, start)
	if from < 0 {
		return ""
	}
	rest := tag[from+len(start):]
	to := strings.Index(rest, end)
	if to < 0 {
		return ""
	}
	return rest[:to]
}

// TestHeadlessSectionStartsAtItsFirstBox: a section with no heading has
// nothing for its first row's top padding to stand off from, so it is marked
// headless and its block takes that padding back — the first box starts
// where the section does. A titled section keeps it, as the gap after its
// heading.
func TestHeadlessSectionStartsAtItsFirstBox(t *testing.T) {
	r := newRenderer(t)
	row := func() []Widget { return []Widget{&Field{Name: "hostname", Label: "Router name"}} }
	if got := render(t, r, &Section{Children: row()}); !strings.Contains(got, `<section class="verso-section" data-verso-section="plain" data-verso-headless`) {
		t.Errorf("an untitled section is marked headless:\n%s", got)
	}
	if got := render(t, r, &Section{Title: "Time", Children: row()}); strings.Contains(got, "data-verso-headless") {
		t.Errorf("a titled section keeps its first row's air:\n%s", got)
	}
	// The section hands its first visible box no air, and the row reads it
	// as the air over its name.
	declares(t, "section", `.verso-section[data-verso-headless] > .verso-section__body > :nth-child(1 of :not(input[type="hidden"], [hidden], template))`,
		"--verso-edge-air: 0px;")
	declares(t, "field", ".verso-field", "padding: var(--verso-edge-air, var(--verso-field-lead, var(--spacing-inset)))")
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
	if !strings.Contains(add, `class="verso-button" data-verso-style="secondary"`) || strings.Contains(add, "hover:") {
		t.Errorf("Add hovers as every secondary button: %s", add)
	}
}

// TestListRowsTakeTheirHeightFromPadding: a list's values are rows, and a row's
// height is its line plus its padding, never a fixed floor — the value's 24px
// line 7px after the hairline above every row and 8px over the next, so its
// line fills a band of the grid, the 28px remove riding the line. The first
// row's hairline goes clear and keeps its pixel, on the line under the label,
// and beside the question the first value stands on the name's own line; the
// add box stands 8px after the last row, so a set with nothing in it puts its
// box straight under the label, or beside it on the name's line.
func TestListRowsTakeTheirHeightFromPadding(t *testing.T) {
	got := render(t, newRenderer(t), &List{Name: "server", Label: "Time servers", Style: "rows", Prompt: "Add a server",
		Items: []string{"0.openwrt.pool.ntp.org", "1.openwrt.pool.ntp.org"}})
	const row = `<div data-verso-list-row class="verso-list__row">`
	if strings.Count(got, row) != 3 { // two values and the row the shell clones
		t.Errorf("value rows take their height from padding, want %s three times in:\n%s", row, got)
	}
	declares(t, "list", ".verso-list__row",
		"padding: calc(var(--spacing) * 1.75) calc(var(--spacing) * 3) calc(var(--spacing) * 2) calc(var(--spacing) * 1.5);",
		"border: 0 solid var(--color-rule);", "border-top-width: 1px;")
	declares(t, "list", ".verso-list__row:first-child", "border-top-color: transparent;")
	// Each value leads with the packet in its words' ink, centred on its first
	// line and in the grid's first column, the value starting at the next; the
	// values are divided from each other, never from the label.
	const packet = `<span aria-hidden="true" class="verso-mark mt-2.25" data-verso-tone="meta"></span>`
	const value = `class="verso-list__value" data-verso-verbatim`
	if strings.Count(got, packet) != 3 || strings.Count(got, value) != 3 {
		t.Errorf("each value leads with the packet, its words quiet:\n%s", got)
	}
	// Meta reads thin at 400, so a value is written at the value role's 500.
	declares(t, "list", ".verso-list__value", "color: var(--color-meta);", "@apply type-value;")
	if strings.Contains(got, "min-h-8") || strings.Contains(blockCSS(t, "list"), "height:") {
		t.Errorf("no fixed row floor:\n%s", got)
	}
	if !strings.Contains(got, `<div data-verso-list-items class="verso-list__items">`) || !strings.Contains(got, `<div class="flex items-center gap-2">`) {
		t.Errorf("the set's rows start on the line under the label, and the add box stands 8px under them:\n%s", got)
	}
	declares(t, "list", ".verso-list__items:has(> .verso-list__row)", "padding-bottom: calc(var(--spacing) * 2);")
	// Beside its question an answer is pulled 4px over the name's line and
	// gives 4px back under itself, as a 32px control centred on that line
	// does. A set's first row takes the top pull itself; its add box, 8px
	// under the rows, stands on the rows' lines, so the set gives the 4px
	// back under it too and the row ends on a line. An empty set's add box
	// stands on the name's line as a control does, and keeps the pull.
	if rule := cssRuleWithin(blockCSS(t, "list"), "@container verso-field (width >= 36rem)", ".verso-list:has(.verso-list__row)"); !strings.Contains(rule, "padding-bottom: var(--spacing);") {
		t.Errorf("a set beside its question does not give back the answer's 4px under its add box:\n%s", rule)
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
		// description stays inside the band, on a line of its own
		`<div data-verso-section-lede class="verso-heading__lede verso-prose">`,
		"<strong>Input</strong>", // sub renders Markdown
	} {
		if !strings.Contains(got, want) {
			t.Errorf("section sub missing %q:\n%s", want, got)
		}
	}
	// sub is muted head-matter, not body prose
	declares(t, "heading", ".verso-heading__lede", "flex: 1 0 100%;")
	declares(t, "heading", ".verso-heading__lede:is(.verso-prose)", "color: var(--color-body);", "@apply type-lede;")
	for _, block := range []string{"heading", "card", "section"} {
		if rule := cssRule(blockCSS(t, block), ".verso-heading__lede"); strings.Contains(rule, "margin-bottom") {
			t.Errorf("blocks/%s.css: a band with a lede keeps a margin of its own; the block under it carries the cell", block)
		}
	}
	// The first block under a band stands a cell under it unless it brings
	// that cell itself (a field, a stacked form of fields, a run of rows that
	// opens on one): the surface hands it the lead, and the rhythm reads it.
	const brings = `.verso-field, .verso-form-grid, .verso-conditional, .verso-field-set, .verso-form:not([data-verso-compact-form]):has(.verso-field, .verso-form-grid, .verso-conditional), [data-verso-change-field], .verso-settings, [data-verso-rows]:has(> .verso-field:first-child)`
	declares(t, "section", `.verso-section:has(> .verso-section__head[data-verso-section-band]) > .verso-section__body > :nth-child(1 of :not(input[type="hidden"], [hidden], template)):not(`+brings+`)`,
		"--verso-edge-lead: var(--spacing-inset);")
	// Under a card's head the cell is measured from the seam's line: the
	// body's border is that line's pixel, so the block stands 23px after it.
	declares(t, "card", `.verso-card__body:not(.verso-card[data-verso-headless] > *) > :nth-child(1 of :not(input[type="hidden"], [hidden], template)):not(`+brings+`, .verso-listing:not([data-verso-list]))`,
		"--verso-edge-lead: calc(var(--spacing-inset) - 1px);")
	declares(t, "rhythm", `.verso-rhythm > :nth-child(1 of :not(input[type="hidden"], [hidden], template))`,
		"margin-top: var(--verso-edge-lead, 0px);")

	plain := render(t, r, &Section{Title: "Zones", Children: []Widget{&Text{Markdown: "body"}}})
	if strings.Contains(plain, "data-verso-section-lede") {
		t.Errorf("a section without a sub stands its content straight on under the title:\n%s", plain)
	}
}

func TestRenderSectionMeta(t *testing.T) {
	r := newRenderer(t)
	got := render(t, r, &Section{
		Title:     "Time and region",
		MetaLabel: "Current time",
		Meta:      "2026-08-29 22:14:08",
		Children:  []Widget{&Text{Markdown: "body"}},
	})
	for _, want := range []string{
		`<span class="verso-heading__meta"><span class="verso-heading__meta-label">Current time</span>`,
		`<span data-verso-section-meta class="verso-heading__meta-value">2026-08-29 22:14:08</span>`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("section meta missing %q:\n%s", want, got)
		}
	}
	// The fact stands at the heading's far end, its label in Meta and its
	// value in Body a step heavier, so the value is what the eye lands on.
	declares(t, "heading", ".verso-heading", "justify-content: space-between;")
	declares(t, "heading", ".verso-heading__meta-label", "color: var(--color-meta);")
	declares(t, "heading", ".verso-heading__meta-value", "color: var(--color-body);", "@apply type-subheading;")
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

// A card's chip names which one of its kind it is (a module's cage), as a
// fact list's head carries one: beside the title, verbatim.
func TestRenderSectionChip(t *testing.T) {
	s := &Section{Title: "SFP+ module", Chip: "xfi0"}
	translateSchema(s, func(string) string { return "translated" })
	got := render(t, newRenderer(t), s)
	title := strings.Index(got, ">translated</h2>")
	chip := strings.Index(got, `<span class="verso-chip" data-verso-verbatim>xfi0</span>`)
	if title < 0 || chip < title {
		t.Errorf("the shell's chip should follow the title in the header, verbatim:\n%s", got)
	}
}

func TestRenderSectionControl(t *testing.T) {
	r := newRenderer(t)
	w, err := Decode([]byte(`{"type":"section","title":"Rule","control":{"type":"switch","name":"enabled","label":"Enabled","off_label":"Disabled","style":"inline","on":true},"children":[]}`))
	if err != nil {
		t.Fatalf("decode section control: %v", err)
	}
	got := render(t, r, w)
	for _, want := range []string{"Rule", "Enabled", "Disabled", `name="enabled"`, `class="verso-choice__row" data-verso-kind="inline"`, `class="verso-choice__dot"`, `<span class="verso-heading__act">`} {
		if !strings.Contains(got, want) {
			t.Errorf("section control missing %q:\n%s", want, got)
		}
	}
	if _, err := Decode([]byte(`{"type":"section","control":{"type":"nope"},"children":[]}`)); err == nil {
		t.Error("unknown section control should fail loudly")
	}
}

// A flush section that is none of the page's subjects (no heading, no rule)
// is bare; a titled one on a page is a card all the same.
func TestRenderSectionFlush(t *testing.T) {
	got := render(t, newRenderer(t), &Section{Flush: true, Children: []Widget{&Text{Markdown: "x"}}})
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
	// section with an id (blocks/card.css, blocks/section.css).
	if !strings.Contains(got, `<section class="verso-card" data-verso-section="card" id="section-speed">`) {
		t.Errorf("anchored section missing its id:\n%s", got)
	}
	declares(t, "card", ".verso-card[id]", "scroll-margin-top: calc(var(--spacing) * 16);")
	declares(t, "section", ".verso-section[id]", "scroll-margin-top: calc(var(--spacing) * 16);")
	for _, block := range []string{"card", "section"} {
		sel := ".verso-" + block + "[id]"
		if rule := cssRuleWithin(blockCSS(t, block), "@media (width >= 48rem)", sel); !strings.Contains(rule, "scroll-margin-top: 0;") {
			t.Errorf("blocks/%s.css: on a wider screen the air %s keeps above itself is enough:\n%s", block, sel, rule)
		}
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
	if !strings.Contains(got, `<h2 class="verso-heading__kicker">On this page</h2>`) {
		t.Errorf("kicker is not set as one:\n%s", got)
	}
	declares(t, "heading", ".verso-heading__kicker", "@apply type-label;")
	if strings.Contains(got, "verso-heading__title") || strings.Contains(got, "data-verso-section-band") {
		t.Errorf("a kicker is not a heading:\n%s", got)
	}
	// The list it names stands a cell under it.
	if !strings.Contains(got, `<div class="verso-section__head verso-heading">`) {
		t.Errorf("a kicker keeps a cell of air under it:\n%s", got)
	}
	declares(t, "section", ".verso-section__head:not([data-verso-section-band]):has(.verso-heading__kicker)", "margin-bottom: var(--spacing-inset);")
}

// On a surface that does not stand its subjects on cards (an Overview card's
// body), a section draws no rule by default; hairline=true makes it one of
// the surface's ruled subjects, and its block draws the rule.
func TestRenderSectionHairline(t *testing.T) {
	r := newRenderer(t)
	bare := func(w Widget) string {
		t.Helper()
		pass := &Renderer{tmpl: r.setFor(""), md: r.md, seq: r.seq}
		var b strings.Builder
		if err := pass.render(&b, w, ""); err != nil {
			t.Fatal(err)
		}
		return b.String()
	}
	plain := bare(&Section{Title: "Default", Children: []Widget{&Text{Markdown: "body"}}})
	if !strings.Contains(plain, `<section class="verso-section" data-verso-section="plain">`) {
		t.Errorf("section should omit its hairline by default:\n%s", plain)
	}

	divided := bare(&Section{
		Title:    "Divided",
		Hairline: true,
		Children: []Widget{&Text{Markdown: "body"}},
	})
	if !strings.Contains(divided, `<section class="verso-section" data-verso-section="ruled">`) {
		t.Errorf("section with hairline=true should be a ruled section:\n%s", divided)
	}
	declares(t, "section", `.verso-section[data-verso-section="ruled"]`, "border: 0 solid var(--color-rule);", "border-top-width: 1px;")
	// Titled, its heading band carries that rule.
	if !strings.Contains(divided, `<div class="verso-section__head verso-heading" data-verso-section-band>`) {
		t.Errorf("a titled ruled section's band carries its rule:\n%s", divided)
	}
	declares(t, "section", ".verso-section__head[data-verso-section-band]", "border: 0 solid var(--color-rule);", "border-top-width: 1px;")
	// On a surface of cards the same section is a card: its frame divides it.
	if got := render(t, r, &Section{Title: "Divided", Hairline: true}); !strings.Contains(got, `<section class="verso-card" data-verso-section="card">`) {
		t.Errorf("a ruled subject on a surface of cards is a card:\n%s", got)
	}
}
