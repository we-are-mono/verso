// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// normalizeHTML collapses whitespace between and around tags, so render tests
// assert structure rather than template indentation.
var (
	wsAfterOpen   = regexp.MustCompile(`>\s+`)
	wsBeforeClose = regexp.MustCompile(`\s+<`)
)

func normalizeHTML(s string) string {
	s = wsAfterOpen.ReplaceAllString(s, ">")
	s = wsBeforeClose.ReplaceAllString(s, "<")
	return strings.TrimSpace(s)
}

func newRenderer(t *testing.T) *Renderer {
	t.Helper()
	r, err := NewRenderer()
	if err != nil {
		t.Fatalf("NewRenderer: %v", err)
	}
	return r
}

func render(t *testing.T, r *Renderer, w Widget) string {
	t.Helper()
	var b strings.Builder
	if err := r.RenderWithToken(&b, w, "", "", nil); err != nil {
		t.Fatalf("RenderWithToken: %v", err)
	}
	return normalizeHTML(b.String())
}

// TestRenderFormError renders the form-level error — a message not tied to any
// field (e.g. a cross-field rule) — and confirms it is escaped like all
// shell-rendered text.
func TestRenderFormError(t *testing.T) {
	r := newRenderer(t)
	got := render(t, r, &Form{Error: "start port must be <= end port", Submit: "Save"})
	if !strings.Contains(got, "start port must be") {
		t.Errorf("form-level error not rendered: %s", got)
	}
	if !strings.Contains(got, `<div class="verso-notice mb-6" data-verso-tone="danger">`) {
		t.Errorf("form-level error not styled as danger: %s", got)
	}
	declares(t, "notice", `.verso-notice[data-verso-tone="danger"]`,
		"--verso-notice-line: var(--color-crimson-line);", "--verso-notice-wash: var(--color-crimson-soft);", "--verso-notice-ink: var(--color-crimson-deep);")
	if strings.Contains(got, "must be <= end") {
		t.Errorf("form error not HTML-escaped by the shell: %s", got)
	}
}

func TestRenderTableEscapesCells(t *testing.T) {
	r := newRenderer(t)
	tbl := &Table{
		Columns: []TableColumn{{Label: "x"}},
		Rows:    []TableRow{{Cells: []TableCell{{Text: `<script>alert(1)</script>`}}}},
	}

	got := render(t, r, tbl)
	if strings.Contains(got, "<script>") {
		t.Errorf("cell content not escaped: %s", got)
	}
	if !strings.Contains(got, "&lt;script&gt;") {
		t.Errorf("escaped cell content missing: %s", got)
	}
}

// TestAFlushStackIsARunOfRows: a flush stack adds no rhythm of its own, so it
// says it holds rows and nothing else, and a section gives back the air of the
// first and last of them as if they stood in it directly (blocks/stack.css
// forwards the section's edges to them).
func TestAFlushStackIsARunOfRows(t *testing.T) {
	got := render(t, newRenderer(t), &Stack{Flush: true, Children: []Widget{&Badge{Text: "a"}}})
	if !strings.HasPrefix(got, `<div class="verso-stack" data-verso-rows data-verso-gap="none">`) {
		t.Errorf("a flush stack does not say it is a run of rows: %s", got)
	}
	declares(t, "stack", `.verso-stack[data-verso-gap="none"]`,
		"--verso-stack-step: var(--verso-stack-gap, 0px)")
	if plain := render(t, newRenderer(t), &Stack{Children: []Widget{&Badge{Text: "a"}}}); strings.Contains(plain, "data-verso-rows") {
		t.Errorf("a spaced stack is blocks, not a run of rows: %s", plain)
	}
}

func TestRenderStackDivided(t *testing.T) {
	r := newRenderer(t)
	plain := render(t, r, &Stack{Children: []Widget{&Badge{Text: "a"}, &Badge{Text: "b"}}})
	// The default stack sets its blocks 24px apart, the grid's inset.
	if !strings.HasPrefix(plain, `<div class="verso-stack">`) {
		t.Errorf("default stack should space its children 24px apart:\n%s", plain)
	}
	declares(t, "stack", ".verso-stack > * + *", "margin-top: var(--verso-step, var(--verso-stack-step))")
	declares(t, "stack", ".verso-stack", "--verso-stack-step: var(--verso-stack-gap, var(--spacing-inset))")
	div := render(t, r, &Stack{Divided: true, Children: []Widget{&Badge{Text: "a"}, &Badge{Text: "b"}}})
	if !strings.HasPrefix(div, `<div class="verso-stack" data-verso-divided`) {
		t.Errorf("divided stack should draw hairlines:\n%s", div)
	}
	declares(t, "stack", ".verso-stack[data-verso-divided] > :not(:last-child)",
		"border-bottom: 1px solid var(--color-rule)")
	compact := render(t, r, &Stack{Compact: true, Children: []Widget{&Badge{Text: "a"}, &Badge{Text: "b"}}})
	if !strings.HasPrefix(compact, `<div class="verso-stack" data-verso-gap="compact">`) {
		t.Errorf("compact stack should use tighter spacing: %s", compact)
	}
	declares(t, "stack", `.verso-stack[data-verso-gap="compact"]`,
		"--verso-stack-step: var(--verso-stack-gap, calc(var(--spacing) * 3))")
	// An inline stack is a wrapping row of acts: 12px between them, its
	// wrapped rows 40px apart, and the row a control's 32px, so its controls'
	// edges are the row's own and land on the grid's lines.
	inline := render(t, r, &Stack{Inline: true, Children: []Widget{&Badge{Text: "a"}, &Text{Markdown: "or"}, &Badge{Text: "b"}}})
	if !strings.HasPrefix(inline, `<div class="verso-cluster">`) || !strings.Contains(inline, ">or</p>") {
		t.Errorf("inline stack should keep its children in one wrapping row: %s", inline)
	}
	declares(t, "cluster", ".verso-cluster",
		"display: flex", "flex-wrap: wrap", "align-items: center",
		"gap: calc(var(--spacing) * 2) calc(var(--spacing) * 3)")
	// A divided list is ruled, not spaced: its gap is none.
	if !strings.Contains(div, `data-verso-gap="none"`) {
		t.Errorf("divided stack should not also space:\n%s", div)
	}
	bounded := render(t, r, &Stack{Width: "compact", Children: []Widget{&Badge{Text: "a"}}})
	if !strings.HasPrefix(bounded, `<div class="verso-stack" data-verso-width="compact">`) {
		t.Errorf("compact-width stack must remain visually bounded: %s", bounded)
	}
	declares(t, "stack", `.verso-stack[data-verso-width="compact"]`, "max-width: var(--container-md)")
}

func TestRenderCardWithoutTitleOmitsHeader(t *testing.T) {
	r := newRenderer(t)

	got := render(t, r, &Card{})
	want := `<section><div class="space-y-6"></div></section>`
	if got != want {
		t.Errorf("Render mismatch:\n got: %s\nwant: %s", got, want)
	}
}

// TestRenderCardNestsChild proves a container renders its children through the
// same renderer, inside its own body — the composition the schema bet depends on.
func TestRenderCardNestsChild(t *testing.T) {
	r := newRenderer(t)
	card := &Card{
		Children: []Widget{&Table{Columns: []TableColumn{{Label: "A"}}, Rows: []TableRow{{Cells: []TableCell{{Text: "1"}}}}}},
	}

	got := render(t, r, card)
	body := strings.Index(got, `<div class="space-y-6">`)
	table := strings.Index(got, "<table")
	if body < 0 || table < 0 || table < body {
		t.Errorf("nested table not rendered inside card body: %s", got)
	}
}

func TestRenderCardNestingDepth(t *testing.T) {
	r := newRenderer(t)
	card := &Card{Children: []Widget{
		&Card{Children: []Widget{
			&Table{Columns: []TableColumn{{Label: "A"}}, Rows: []TableRow{{Cells: []TableCell{{Text: "1"}}}}},
		}},
	}}

	got := render(t, r, card)
	if strings.Count(got, "<section") != 2 {
		t.Errorf("want 2 nested card sections, got: %s", got)
	}
	if !strings.Contains(got, "<table") {
		t.Errorf("innermost table missing: %s", got)
	}
}

func TestRenderFieldText(t *testing.T) {
	r := newRenderer(t)

	got := render(t, r, &Field{Name: "hostname", Label: "Hostname", Value: "OpenWrt", Datatype: "hostname"})
	for _, want := range []string{`name="hostname"`, `value="OpenWrt"`, "Hostname", "<input", `data-datatype="hostname"`} {
		if !strings.Contains(got, want) {
			t.Errorf("field missing %q: %s", want, got)
		}
	}
}

func TestRenderFieldSelectMarksSelected(t *testing.T) {
	r := newRenderer(t)

	got := render(t, r, &Field{
		Name: "tz", Label: "Timezone", Kind: "select", Value: "0",
		Options: choiceOptions(RadioOptionLimit + 1),
	})
	if !strings.Contains(got, "<select") {
		t.Errorf("no <select>: %s", got)
	}
	if !strings.Contains(got, `value="0" selected`) {
		t.Errorf("selected option not marked: %s", got)
	}
	if strings.Count(got, " selected") != 1 {
		t.Errorf("wrong option marked selected: %s", got)
	}
}

func TestRenderBadge(t *testing.T) {
	r := newRenderer(t)
	got := render(t, r, &Badge{Variant: "success", Text: "Connected", Dot: true})
	for _, want := range []string{
		// the state mark is the app's one mark: a 6px square, the packet
		"Connected", `class="verso-mark" data-verso-tone="success"`,
		`<span class="verso-chip" data-verso-verbatim data-verso-tone="success">`, // the chip in its tone
	} {
		if !strings.Contains(got, want) {
			t.Errorf("success badge missing %q in: %s", want, got)
		}
	}
	// An unknown/neutral variant falls back to sand, never leaks the variant
	// name. A pill with no hue — the firewall's drop — is the neutral chip, the
	// same box and colours as every other chip without a hue.
	neutral := render(t, r, &Badge{Variant: "neutral", Text: "Offline"})
	if !strings.Contains(neutral, `<span class="verso-chip" data-verso-verbatim data-verso-tone="neutral">Offline`) {
		t.Errorf("neutral badge should be the neutral chip: %s", neutral)
	}
	if strings.Contains(neutral, "verso-mark") {
		t.Errorf("badge without Dot should not render a dot: %s", neutral)
	}
}

// TestAFocusedBoxStaysFocusedUnderThePointer: a box whose focus is inside
// it (a field of value chips, a file's drop area) turns
// denim while focused whether or not the pointer is over it — its hover
// colour applies only while it is not focused, as a plain field's does.
func TestAFocusedBoxStaysFocusedUnderThePointer(t *testing.T) {
	r := newRenderer(t)
	file := render(t, r, &Field{Name: "backup", Kind: "file"})
	if !strings.Contains(file, `class="verso-drop__area"`) {
		t.Errorf("file: the area is not the drop block's:\n%s", file)
	}
	declares(t, "drop", ".verso-drop__area:hover:not(:focus-within)", "border-color: var(--color-sand-5);")
	declares(t, "drop", ".verso-drop__area:focus-within", "border-color: var(--color-denim);")
	if cssRule(blockCSS(t, "drop"), ".verso-drop__area:hover") != "" {
		t.Errorf("file: a bare hover rule would outrank focus")
	}
	tokens := render(t, r, &List{Name: "proto", Label: "Protocol", Style: "tokens"})
	if !strings.Contains(tokens, `class="verso-tokens"`) {
		t.Errorf("tokens: the box is not the tokens block:\n%s", tokens)
	}
	declares(t, "tokens", ".verso-tokens:hover:not(:focus-within)", "border-color: var(--color-faint);")
	declares(t, "tokens", ".verso-tokens:focus-within", "border-color: var(--color-denim);")
	if cssRule(blockCSS(t, "tokens"), ".verso-tokens:hover") != "" {
		t.Errorf("tokens: a bare hover rule would outrank focus")
	}
}

// TestACompactNoteKeepsItsControlsMeasure: a compact note explains the
// control it sits under, so it stands at that control's measure — the form's
// — rather than running past it; it leads with its tone's small square.
func TestACompactNoteKeepsItsControlsMeasure(t *testing.T) {
	got := render(t, newRenderer(t), &Callout{Variant: "warning", Compact: true, Body: "Named once."})
	if !strings.Contains(got, `<div data-verso-callout class="verso-notice" data-verso-tone="warning" data-verso-compact>`) {
		t.Errorf("a compact note keeps the form's measure:\n%s", got)
	}
	declares(t, "notice", ".verso-notice[data-verso-compact]", "max-width: var(--container-3xl);")
	if !strings.Contains(got, `class="verso-mark verso-notice__mark" data-verso-tone="warning"`) {
		t.Errorf("a compact note leads with its tone's square:\n%s", got)
	}
}

// TestEveryNoticeIsOneBand: a notice that leads a page or a card is the band a
// compact note is — the tone's square on the first line, the same air, the
// door closing the words' line — and differs only in reaching its container's
// width instead of keeping a control's measure.
func TestEveryNoticeIsOneBand(t *testing.T) {
	standing := render(t, newRenderer(t), &Callout{Variant: "warning", Title: "The internet can reach this web interface",
		Body: "The firewall zone “mgmt” accepts every connection from the internet.", Link: &Link{Label: "Open Firewall", Href: "/plugins/firewall/"}})
	for _, want := range []string{`class="verso-mark verso-notice__mark" data-verso-tone="warning"`, `<div data-verso-callout class="verso-notice" data-verso-tone="warning">`} {
		if !strings.Contains(standing, want) {
			t.Errorf("a standing notice is not the one band, missing %q:\n%s", want, standing)
		}
	}
	// The one band's air and its tone are the block's, not the variant's.
	declares(t, "notice", ".verso-notice",
		"padding: calc(var(--spacing) * 1.75) calc(var(--spacing) * 3) calc(var(--spacing) * 2) calc(var(--spacing) * 5.75);")
	declares(t, "notice", noticeTone("warning"),
		"--verso-notice-line: var(--color-marigold-line);", "--verso-notice-wash: var(--color-marigold-soft);", "--verso-notice-ink: var(--color-marigold-deep);")
	if strings.Contains(standing, "data-verso-compact") {
		t.Errorf("a standing notice reaches its container's width:\n%s", standing)
	}
	if band := cssRule(blockCSS(t, "notice"), ".verso-notice"); strings.Contains(band, "max-width") {
		t.Errorf("only a compact band keeps a measure; the band itself reaches its container's width:\n%s", band)
	}
	body := strings.Index(standing, "accepts every connection")
	closes := strings.Index(standing[body:], "</p>")
	if link := strings.Index(standing, "Open Firewall"); body < 0 || link < 0 || closes < 0 || link > body+closes {
		t.Errorf("the door closes the notice's own line:\n%s", standing)
	}
}

// TestACompactNoteKeepsItsLinkOnItsLine: a compact note is one line of words,
// and its door is part of that line — the sentence, then where to go about it —
// not a second line under it.
func TestACompactNoteKeepsItsLinkOnItsLine(t *testing.T) {
	got := render(t, newRenderer(t), &Callout{Variant: "info", Compact: true, Body: "15 packages have newer versions.",
		Link: &Link{Label: "Review in Packages", Href: "/system/packages?tab=upgradable"}})
	sentence := strings.Index(got, "15 packages have newer versions.")
	link := strings.Index(got, "Review in Packages")
	closes := strings.Index(got[sentence:], "</p>")
	if sentence < 0 || link < 0 || closes < 0 || link > sentence+closes {
		t.Errorf("the link should close the note's own line:\n%s", got)
	}
}

// Multi-select chips retain their checkbox semantics and selected treatment.
func TestRenderMultipleChoiceChips(t *testing.T) {
	r := newRenderer(t)
	days := render(t, r, &Field{Name: "days", Label: "Days", Kind: "checks", Style: "segmented", Values: []string{"mon"},
		Options: []Option{{Value: "mon", Label: "Mon"}, {Value: "tue", Label: "Tue"}}})
	for _, want := range []string{
		`class="verso-choice__group" data-verso-kind="segments">`,
		`<label class="verso-choice__segment">`,
	} {
		if !strings.Contains(days, want) {
			t.Errorf("segmented field missing %q:\n%s", want, days)
		}
	}
	declares(t, "choice", `.verso-choice__group[data-verso-kind="segments"]`, "flex-wrap: wrap", "width: 100%",
		"border: 1px solid var(--color-rule-strong)", "background-color: var(--color-quiet)")
	// a selected chip uses the shared choice colour
	declares(t, "choice", ".verso-choice__segment:has(:checked)", "background-color: var(--color-choice)",
		"color: var(--color-on-fill)", "@apply type-button")
	if !strings.Contains(days, `type="checkbox" name="days" value="mon" checked`) {
		t.Errorf("a day in the set is a checked box:\n%s", days)
	}
	// Checking a chip thickens its word, and a thicker word is a wider one:
	// every chip holds its semibold width from the start, a hidden twin in the
	// same cell, so checking one never nudges the rest along the strip.
	want := `<span class="verso-choice__stack"><span class="verso-choice__words">Tue</span><span aria-hidden="true" class="verso-choice__twin">Tue</span></span>`
	if !strings.Contains(days, want) {
		t.Errorf("segmented chip does not hold its checked width %q:\n%s", want, days)
	}
	declares(t, "choice", ".verso-choice__stack", "display: grid")
	declares(t, "choice", ".verso-choice__words,\n.verso-choice__twin", "grid-area: 1 / 1")
	if twin := ".verso-choice__twin {\n  visibility: hidden;\n  @apply type-button;\n}"; !strings.Contains(blockCSS(t, "choice"), twin) {
		t.Errorf("the twin is not the hidden semibold width:\n%s", twin)
	}
}

func TestRenderEmpty(t *testing.T) {
	r := newRenderer(t)
	got := render(t, r, &Empty{
		Icon: "shield", Title: "Reach your home from anywhere",
		Body:     "Set up a private VPN so your devices can reach home.",
		Children: []Widget{&Modal{Trigger: "Set up home VPN", Title: "Add a device"}},
	})

	for _, want := range []string{
		`<div class="verso-empty">`,
		`<span class="verso-empty__icon">`,
		`<h2 class="verso-empty__title">Reach your home from anywhere</h2>`,
		`<div class="verso-prose verso-empty__body">`,
		`<div class="verso-empty__acts">`,
		"Reach your home from anywhere",
		"Set up a private VPN so your devices can reach home.",
		"Set up home VPN", // the CTA (a modal trigger) rendered as a child
		`x-data="modal"`,  // the CTA is a real composed widget
	} {
		if !strings.Contains(got, want) {
			t.Errorf("empty missing %q in: %s", want, got)
		}
	}
	// The pulsing accent icon is the block's: 80px, 24px over the title.
	declares(t, "empty", ".verso-empty__icon",
		"width: calc(var(--spacing) * 20)", "height: calc(var(--spacing) * 20)",
		"margin-bottom: calc(var(--spacing) * 6)", "border-radius: var(--radius-3xl)")
	declares(t, "empty", ".verso-empty__icon::after", "animation: verso-pulse-lg")
}

// TestRenderEmptyBodyIsMarkdown: an empty state's one line of reassurance is
// prose, and prose in this schema is Markdown — the same pass a section's sub
// gets. It is where a first-run state names the control that ends it, and a
// literal `**Log matching packets**` on the screen is the schema showing through.
// The sanitising engine is the same one the raw bridge uses, so an empty state
// cannot become a way to inject markup.
func TestRenderEmptyBodyIsMarkdown(t *testing.T) {
	r := newRenderer(t)
	got := render(t, r, &Empty{
		Icon: "activity", Title: "Nothing is being logged",
		Body: "Turn on **Log matching packets** on a rule to see verdicts here.",
	})
	if !strings.Contains(got, "<strong>Log matching packets</strong>") {
		t.Errorf("empty body not rendered as Markdown: %s", got)
	}
	if strings.Contains(got, "**") {
		t.Errorf("empty body still shows its own markup: %s", got)
	}
	// Markdown emits its own <p>; nesting that inside one would be invalid.
	if strings.Contains(got, "<p") && strings.Contains(got, "<p><p") {
		t.Errorf("empty body must not nest paragraphs: %s", got)
	}
	unsafe := render(t, r, &Empty{Title: "t", Body: `<script>alert(1)</script>`})
	if strings.Contains(unsafe, "<script") {
		t.Errorf("an empty body must never carry markup through: %s", unsafe)
	}
}

// TestRenderBadgeDotIsStill: a pill is a chip, and a chip's packet never
// pulses, whatever it states — the page's live mark is where liveness shows.
func TestRenderBadgeDotIsStill(t *testing.T) {
	r := newRenderer(t)
	for _, variant := range []string{"success", "neutral"} {
		got := render(t, r, &Badge{Variant: variant, Text: "Connected", Dot: true})
		if strings.Contains(got, "data-verso-live") {
			t.Errorf("a %s chip's packet must not pulse: %s", variant, got)
		}
	}
}

func TestRenderDivider(t *testing.T) {
	r := newRenderer(t)
	// A divider separates groups: a bare hairline that keeps no air of its
	// own, spaced by the stack or rhythm it stands in.
	got := render(t, r, &Divider{})
	if !strings.Contains(got, `<hr class="verso-divider">`) {
		t.Errorf("divider should be a bare rule spaced by what it stands in: %s", got)
	}
	declares(t, "divider", ".verso-divider",
		"margin: 0", "border: 0 solid var(--color-rule)", "border-top-width: 1px")
}

func TestRenderProperties(t *testing.T) {
	r := newRenderer(t)
	got := render(t, r, &Properties{Items: []Property{
		{Label: "Can reach", Value: "My whole home network"},
		{Label: "Tunnel address", Value: "10.7.0.2", Mono: true},
		{Label: "Server key", Value: "KEY==", Copy: true},
	}})
	for _, want := range []string{"<dl", "Can reach", "My whole home network", "Tunnel address", `<span data-verso-verbatim class="verso-facts__text">10.7.0.2</span>`} {
		if !strings.Contains(got, want) {
			t.Errorf("properties missing %q in: %s", want, got)
		}
	}
	// A verbatim value is the value role in its verbatim form.
	declares(t, "facts", ".verso-facts__text:is([data-verso-verbatim], [data-verso-figures]),\n.verso-facts__next[data-verso-verbatim]", "@apply type-value;")
	// A non-mono value is not verbatim on its own row.
	if strings.Count(got, "data-verso-verbatim") != 1 {
		t.Errorf("only the mono value should be monospaced: %s", got)
	}
	// A Copy row grows an inline copy button (shell-owned) carrying the value.
	if !strings.Contains(got, `x-data="copy"`) || !strings.Contains(got, "KEY==") {
		t.Errorf("copyable property missing its inline copy button: %s", got)
	}
	// Non-copy rows don't.
	plain := render(t, r, &Properties{Items: []Property{{Label: "A", Value: "b"}}})
	if strings.Contains(plain, "x-data") {
		t.Errorf("non-copy property should have no copy button: %s", plain)
	}
}

// TestRenderPropertiesStyles: the row style resolves to hairlines (default),
// no separators, or the larger single-identity treatment.
func TestRenderPropertiesStyles(t *testing.T) {
	r := newRenderer(t)
	items := []Property{{Label: "A", Value: "1"}, {Label: "B", Value: "2"}}

	// Each ruled row draws its own hairline as its last pixel, landing it on a
	// line of the grid; the first, with no hairline over it, keeps 8px over its
	// line, and the last draws none, so the run starts and ends on lines.
	const hairlineRow = `<dl class="verso-facts" data-verso-style="divided">`
	declares(t, "facts", `.verso-facts__row:where(.verso-facts[data-verso-style="divided"] *)`,
		"padding-block: calc(var(--spacing) * 1.75) calc(var(--spacing) * 2);", "border: 0 solid var(--color-mid);", "border-bottom-width: 1px;")
	declares(t, "facts", `.verso-facts__row:where(.verso-facts[data-verso-style="divided"] *):last-child`, "border-bottom-width: 0;")
	divided := render(t, r, &Properties{Items: items}) // default
	if !strings.Contains(divided, hairlineRow) {
		t.Errorf("default properties should use hairlines:\n%s", divided)
	}
	// Striping is retired: a "striped" request falls through to the hairline
	// default and never zebra-shades.
	striped := render(t, r, &Properties{Style: "striped", Items: items})
	if strings.Contains(striped, "odd:bg-quiet") {
		t.Errorf("striped is retired; must not zebra-shade:\n%s", striped)
	}
	if !strings.Contains(striped, hairlineRow) {
		t.Errorf("retired striped should render as the hairline default:\n%s", striped)
	}
	bare := render(t, r, &Properties{Style: "plain", Items: items})
	if !strings.Contains(bare, `data-verso-style="plain"`) || strings.Contains(bare, "divide-y") || strings.Contains(bare, "odd:bg-quiet") {
		t.Errorf("plain properties should have no separators:\n%s", bare)
	}
	declares(t, "facts", `.verso-facts:is([data-verso-style="plain"], [data-verso-style="identity"]) > :not(:last-child)`, "margin-bottom: calc(var(--spacing) * 3);")
	identity := render(t, r, &Properties{Style: "identity", Items: []Property{{
		Label: "Username", Value: "root", Mono: true, Emphasis: true, Help: "Main system username cannot be changed.",
	}}})
	// the larger single-identity treatment: an emphasised verbatim value in
	// the title's verbatim form
	declares(t, "facts", ".verso-facts__value[data-verso-emphasis][data-verso-verbatim]", "@apply type-title;")
	for _, want := range []string{`data-verso-style="identity"`, ">Username:<", "data-verso-verbatim", "data-verso-emphasis", ">root<", "Main system username cannot be changed."} {
		if !strings.Contains(identity, want) {
			t.Errorf("identity properties missing %q:\n%s", want, identity)
		}
	}
	if strings.Contains(identity, "divide-y") {
		t.Errorf("identity properties must not look like a table:\n%s", identity)
	}
	system := render(t, r, &Properties{Style: "system", Items: []Property{
		{Label: "Firmware", Value: "Verso 1.3.2"},
		{Label: "Kernel", Value: "Linux 6.12", Mono: true},
	}})
	for _, want := range []string{
		`data-verso-style="system"`, `<dt class="verso-facts__label">`, `<dd class="verso-facts__value">`,
		`<span data-verso-verbatim class="verso-facts__text">Linux 6.12</span>`,
	} {
		if !strings.Contains(system, want) {
			t.Errorf("system properties missing %q:\n%s", want, system)
		}
	}
	// The 44px row: the value's 24px line inset 10px, the label raised to the
	// same line, and the value a flex line so its copy control cannot lift it.
	declares(t, "facts", `.verso-facts__row:where(.verso-facts[data-verso-style="system"] *)`,
		"align-items: center;", "padding-block: calc(var(--spacing) * 2.5);", "border-top-width: 1px;")
	declares(t, "facts", ".verso-facts__label", "@apply type-row-label;", "color: var(--color-meta);")
	declares(t, "facts", ".verso-facts__value", "display: flex;", "align-items: center;", "@apply type-value;")
	emphasised := render(t, r, &Properties{Items: []Property{
		{Label: "Installed", Value: "Mono OpenWrt 25.12", Emphasis: true},
		{Label: "Target", Value: "aarch64_generic", Mono: true, Emphasis: true},
	}})
	declares(t, "facts", ".verso-facts__value[data-verso-emphasis]", "@apply type-card-title;")
	for _, want := range []string{
		`<dt class="verso-facts__label">Installed</dt>`,
		`<dd class="verso-facts__value" data-verso-emphasis>`,
		`<span data-verso-verbatim class="verso-facts__text">aarch64_generic</span>`,
	} {
		if !strings.Contains(emphasised, want) {
			t.Errorf("emphasised properties missing %q:\n%s", want, emphasised)
		}
	}
}

// TestRenderConfirmCaution: an act that is disruptive but wanted asks in the same
// one-hue shape, in marigold. White on full marigold fails the 4.5 floor, so the
// act is white on the deep step; no crimson appears anywhere in it.
func TestRenderConfirmCaution(t *testing.T) {
	r := newRenderer(t)
	got := render(t, r, &Confirm{Trigger: "Download and install 25.12.5", Title: "Install 25.12.5 now?", Message: "It restarts on its own.", Confirm: "Install firmware", Cancel: "Not yet", Tone: "caution"})
	for _, want := range []string{
		`data-verso-confirm-tone="caution"`,
		// The question is the marigold band, marked by marigold's square.
		`<div data-verso-callout class="verso-notice" data-verso-tone="warning" data-verso-compact>`,
		`class="verso-mark verso-notice__mark" data-verso-tone="warning"`,
		// The trigger is the button block's caution wash, the act its caution
		// dress, the way back the band's own words.
		`class="verso-button w-fit" data-verso-style="caution-wash">`,
		`class="verso-button shrink-0" data-verso-style="caution">`,
		`<button type="button" @click="cancel" class="verso-notice__cancel">Not yet</button>`,
		"Download and install 25.12.5", "Install 25.12.5 now?", "Install firmware", "Not yet",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("caution confirm missing %q in: %s", want, got)
		}
	}
	declares(t, "notice", noticeTone("warning"),
		"--verso-notice-line: var(--color-marigold-line);", "--verso-notice-wash: var(--color-marigold-soft);", "--verso-notice-ink: var(--color-marigold-deep);")
	if strings.Contains(got, "crimson") || strings.Contains(got, `data-verso-tone="danger"`) || strings.Contains(got, `data-verso-style="danger`) {
		t.Errorf("a caution confirm must carry no crimson: %s", got)
	}
	if danger := render(t, r, &Confirm{Trigger: "Delete", Message: "Sure?"}); !strings.Contains(danger, `data-verso-confirm-tone="danger"`) {
		t.Errorf("a confirm without a tone is the danger one: %s", danger)
	}
}

// An icon confirm standing in its item's row lays itself into that row, so its
// question drops full-width under the row rather than squeezing in beside the
// acts it shares the line with.
func TestRenderConfirmInARowOfActs(t *testing.T) {
	r := newRenderer(t)
	got := render(t, r, &Confirm{Trigger: "Remove", Icon: "trash-2", Message: "Remove this key?", Confirm: "Remove", Cancel: "Keep it"})
	for _, want := range []string{`<div class="verso-confirm" data-verso-in-row x-data="confirm"`, `x-cloak class="verso-confirm__question"`, `aria-label="Remove"`} {
		if !strings.Contains(got, want) {
			t.Errorf("in-row confirm missing %q in: %s", want, got)
		}
	}
	declares(t, "confirm", ".verso-confirm[data-verso-in-row]", "display: contents;")
	declares(t, "confirm", ".verso-confirm__question:where(.verso-confirm[data-verso-in-row] > *)", "flex-basis: 100%;", "margin-top: calc(var(--spacing) * 3);")
	if alone := render(t, r, &Confirm{Trigger: "Reboot now", Message: "Reboot?"}); strings.Contains(alone, "data-verso-in-row") || !strings.Contains(alone, `<div class="verso-confirm" x-data="confirm"`) {
		t.Errorf("a confirm standing alone keeps its own block: %s", alone)
	}
	declares(t, "confirm", ".verso-confirm", "display: flow-root;")
}

func TestRenderConfirm(t *testing.T) {
	r := newRenderer(t)
	got := render(t, r, &Confirm{Trigger: "Remove device", Message: "Remove this device?", Confirm: "Remove", Cancel: "Keep it"})
	for _, want := range []string{
		"verso-confirm", `x-data="confirm"`,
		// The trigger and the way back are real buttons: reached by Tab, pressed
		// by Enter or Space, and the trigger says whether it has the question open.
		`<button type="button" x-ref="trigger" x-show="idle" @click="ask" :aria-expanded="expanded"`,
		`<button type="button" @click="cancel"`,
		`x-show="asking" x-cloak`,
		"Remove device", "Remove this device?", "Remove", "Keep it",
		// Both states stand on crimson's own soft ground inside its hairline,
		// worded in the step of the hue that can carry words; the full-chroma
		// value stays a mark, on the square.
		`class="verso-mark verso-notice__mark" data-verso-tone="danger"`,
		// The question is the compact callout in the act's tone: a row's
		// height for a line, its words 24px in from the frame's edge.
		`<div data-verso-callout class="verso-notice" data-verso-tone="danger" data-verso-compact>`,
		// The answers are the band's acts, on the line a cell under the words;
		// cancel keeps the explanation's tone and hovers by a soft wash, not a
		// heavy colour darken.
		`<div data-verso-callout-acts class="verso-notice__acts"><button type="submit" x-ref="first"`,
		`<button type="button" @click="cancel" class="verso-notice__cancel">Keep it</button>`,
		`x-ref="first" class="verso-button shrink-0" data-verso-style="danger">`,
		// The trigger is a control like every other button, the button block
		// in its danger wash, and it is as wide as its words, never the column.
		`class="verso-button w-fit" data-verso-style="danger-wash">`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("confirm missing %q in: %s", want, got)
		}
	}
	// Crimson's own soft ground inside its hairline, its words the deep step.
	declares(t, "notice", noticeTone("danger"),
		"--verso-notice-line: var(--color-crimson-line);", "--verso-notice-wash: var(--color-crimson-soft);", "--verso-notice-ink: var(--color-crimson-deep);")
	// The way back is the band's words, washed by the band's hairline under
	// the pointer.
	declares(t, "notice", ".verso-notice__cancel", "color: var(--verso-notice-ink);", "height: var(--spacing-control);", "@apply type-button;")
	declares(t, "notice", ".verso-notice__cancel:hover", "background-color: color-mix(in srgb, var(--verso-notice-line) 50%, transparent);")
	// No checkbox pretending to be a button, and no inline script: the
	// behaviour is the shell's confirm component.
	if strings.Contains(got, "<script") || strings.Contains(got, `type="checkbox"`) {
		t.Errorf("confirm must use the shell component, not a checkbox or script: %s", got)
	}
	if cancel := cssRule(blockCSS(t, "notice"), ".verso-notice__cancel"); strings.Contains(cancel, "border:") || strings.Contains(cancel, "border-width") ||
		strings.Contains(got, `@click="cancel" class="verso-button`) {
		t.Errorf("confirm cancel must not render as a bordered button:\n%s\n%s", cancel, got)
	}
	// Unset, the confirm button repeats the act it confirms — the question
	// and its answer name the same act, never a bare "Confirm" — and the way
	// back is Cancel.
	def := render(t, r, &Confirm{Trigger: "Delete rule", Message: "Sure?"})
	if strings.Count(def, ">Delete rule<") < 2 || strings.Contains(def, ">Confirm<") || !strings.Contains(def, ">Cancel<") {
		t.Errorf("confirm should default to its act's own name: %s", def)
	}
	// Two confirms on a page get distinct panel ids, the trigger's controls.
	if !strings.Contains(got, `aria-controls="verso-confirm-1"`) || !strings.Contains(def, `id="verso-confirm-2"`) {
		t.Errorf("each confirm's trigger controls its own panel:\n%s\n%s", got, def)
	}
	// A Title renders as the band's bold title on the 20px line above the
	// message; without one the message stands alone (no stray heading element).
	titled := render(t, r, &Confirm{Trigger: "Download and install", Title: "Install now?", Message: "It will be unavailable for several minutes.", Tone: ToneCaution})
	if !strings.Contains(titled, `<p class="verso-notice__title">Install now?</p>`) {
		t.Errorf("a titled confirm should render its heading bold: %s", titled)
	}
	declares(t, "notice", ".verso-notice__title", "@apply type-subheading;")
	if !strings.Contains(titled, "It will be unavailable for several minutes.") {
		t.Errorf("a titled confirm should still render its message: %s", titled)
	}
	// Caution asks in the warning's marigold band.
	if !strings.Contains(titled, `class="verso-notice" data-verso-tone="warning"`) || strings.Contains(titled, `data-verso-tone="danger"`) {
		t.Errorf("a caution confirm should ask in the marigold band: %s", titled)
	}
	// The trigger and the confirm button are semibold like every other control;
	// what an untitled confirm must not draw is the heading paragraph.
	if strings.Contains(def, `<p class="verso-notice__title">`) {
		t.Errorf("an untitled confirm should draw no heading: %s", def)
	}
}

func TestRenderCallout(t *testing.T) {
	r := newRenderer(t)
	// A tone at full chroma is a mark and never sets type: the band is the tone's
	// soft ground with its deep step carrying the words, framed by the tone's
	// own hairline and marked by its square.
	ok := render(t, r, &Callout{Variant: "success", Title: "Reachable", Body: "Verified from the internet."})
	for _, want := range []string{
		`<div data-verso-callout class="verso-notice" data-verso-tone="success">`,
		`class="verso-mark verso-notice__mark" data-verso-tone="success"`,
		"Reachable", "Verified from the internet.",
	} {
		if !strings.Contains(ok, want) {
			t.Errorf("success callout missing %q in: %s", want, ok)
		}
	}
	declares(t, "notice", noticeTone("success"),
		"--verso-notice-line: var(--color-green-line);", "--verso-notice-wash: var(--color-green-soft);", "--verso-notice-ink: var(--color-green-deep);")
	declares(t, "notice", ".verso-notice",
		"border: 1px solid var(--verso-notice-line);", "background-color: var(--verso-notice-wash);", "color: var(--verso-notice-ink);")
	// No glyph anywhere: the colour and the mark already say what kind of thing
	// this is, and a triangle beside three words of prose only crowds them.
	if strings.Contains(ok, "<svg") {
		t.Errorf("a callout carries no icon: %s", ok)
	}
	// Unknown/default variant falls back to info (denim), never leaks the variant name.
	def := render(t, r, &Callout{Body: "heads up"})
	if !strings.Contains(def, `class="verso-notice" data-verso-tone="info"`) {
		t.Errorf("default callout should use the info palette: %s", def)
	}
	declares(t, "notice", noticeTone("info"),
		"--verso-notice-line: var(--color-denim-line);", "--verso-notice-wash: var(--color-denim-soft);", "--verso-notice-ink: var(--color-denim-deep);")
	if odd := render(t, r, &Callout{Variant: "loud", Body: "x"}); !strings.Contains(odd, `data-verso-tone="info"`) || strings.Contains(odd, "loud") {
		t.Errorf("an unknown variant should fall back to info without leaking its name: %s", odd)
	}
	warn := render(t, r, &Callout{Variant: "warning", Body: "x"})
	if !strings.Contains(warn, `class="verso-notice" data-verso-tone="warning"`) {
		t.Errorf("warning callout should use marigold: %s", warn)
	}
	declares(t, "notice", noticeTone("warning"), "--verso-notice-wash: var(--color-marigold-soft);", "--verso-notice-ink: var(--color-marigold-deep);")
	neutral := render(t, r, &Callout{Variant: "neutral", Compact: true, Body: "Quiet context."})
	if !strings.Contains(neutral, `class="verso-notice" data-verso-tone="neutral"`) || !strings.Contains(neutral, `data-verso-tone="muted"`) {
		t.Errorf("neutral callout should be the quiet band: %s", neutral)
	}
	declares(t, "notice", noticeTone("neutral"), "--verso-notice-wash: var(--color-quiet);", "--verso-notice-ink: var(--color-body);")
	// Compact is the note under a control: its words 24px in from the frame's
	// edge, the mark in the middle of that 24px on the first line's optical
	// centre, framed one step darker than its ground as every band is, without
	// the standing band's padding.
	compact := render(t, r, &Callout{Compact: true, Body: "A short note."})
	for _, want := range []string{`<div data-verso-callout class="verso-notice" data-verso-tone="info" data-verso-compact>`,
		`<span aria-hidden="true" class="verso-mark verso-notice__mark" data-verso-tone="info"></span>`, "A short note."} {
		if !strings.Contains(compact, want) {
			t.Errorf("compact callout missing %q in: %s", want, compact)
		}
	}
	declares(t, "notice", ".verso-notice", "align-items: flex-start;",
		"padding: calc(var(--spacing) * 1.75) calc(var(--spacing) * 3) calc(var(--spacing) * 2) calc(var(--spacing) * 5.75);",
		"border: 1px solid var(--verso-notice-line);")
	declares(t, "notice", ".verso-notice__mark", "position: absolute;", "top: calc(var(--spacing) * 4);", "left: calc(var(--spacing) * 2.25);")
	if rule := cssRule(blockCSS(t, "notice"), ".verso-notice[data-verso-compact]"); rule == "" || strings.Contains(rule, "padding") {
		t.Errorf("compact callout should not carry padding of its own:\n%s", rule)
	}
}

func TestRenderLink(t *testing.T) {
	r := newRenderer(t)
	// A data: URL is never a link's destination.
	if data := render(t, r, &Link{Label: "Config", Href: "data:text/plain,abc"}); !strings.Contains(data, `href="#"`) {
		t.Errorf("a data: link must collapse to #: %s", data)
	}
	primary := render(t, r, &Link{Label: "Download backup", Href: "/backup", Style: "button"})
	// The button block carries the tactile pressed state.
	if !strings.Contains(primary, `class="verso-button">`) {
		t.Errorf("primary button link missing tactile pressed state: %s", primary)
	}
	secondary := render(t, r, &Link{Label: "Restart router", Href: "/restart", Style: "secondary"})
	for _, want := range []string{
		`class="verso-button" data-verso-style="secondary">`,
	} {
		if !strings.Contains(secondary, want) {
			t.Errorf("secondary link missing %q: %s", want, secondary)
		}
	}
	// One place on this page in the list of them: a block, so a run of them is a
	// column; its own left hairline, so the run draws one continuous line; and the
	// hook the shell marks when this is the section being read.
	// A link to a place on this page reaches the section's namespaced id.
	rail := render(t, r, &Link{Label: "Speed", Href: "#speed", Style: "rail"})
	for _, want := range []string{
		`href="#section-speed"`, "data-verso-rail-link", `class="verso-rail"`,
	} {
		if !strings.Contains(rail, want) {
			t.Errorf("rail link missing %q: %s", want, rail)
		}
	}
	// 32px each, its 24px line and 4px over and under, so a run of any count
	// is a multiple of 8px; each its own left hairline; the current one Ink
	// and weighted with a 2px bar drawn inside, so nothing shifts as it moves.
	declares(t, "rail", ".verso-rail",
		"display: flex", "align-items: center", "gap: calc(var(--spacing) * 1.5)",
		"padding: var(--spacing) 0 var(--spacing) calc(var(--spacing) * 4)",
		"border: 0 solid var(--color-rule)", "border-left-width: 1px", "@apply type-body")
	declares(t, "rail", `.verso-rail[aria-current="true"]`,
		"color: var(--color-ink)", "box-shadow: inset 2px 0 0 var(--color-ink)", "@apply type-subheading")
	// Beside a form the hairline steps back a pixel onto the column's line.
	if !strings.Contains(blockCSS(t, "rail"), "@container (width >= 62.5rem) {\n  .verso-rail {\n    margin-left: -1px;") {
		t.Error("blocks/rail.css: beside a form the rail's hairline should step back 1px")
	}
	if strings.Contains(rail, "underline") || strings.Contains(rail, "text-denim") {
		t.Errorf("a rail link is not an ordinary link: %s", rail)
	}
	// So does a link to a place on a page it opens: the act into an editor's
	// DHCP section reaches that section's namespaced id, and a link with no
	// place named is left alone.
	into := render(t, r, &Link{Label: "Configure DHCP", Href: "/plugins/interfaces/edit?network=lan#dhcp-server", Style: "act", Panel: true})
	if !strings.Contains(into, `href="/plugins/interfaces/edit?network=lan#section-dhcp-server"`) {
		t.Errorf("a link into a page's section must reach its id: %s", into)
	}
	bare := render(t, r, &Link{Label: "Interfaces", Href: "/plugins/interfaces/", Style: "act"})
	if !strings.Contains(bare, `href="/plugins/interfaces/"`) {
		t.Errorf("a link with no place named is left alone: %s", bare)
	}
	// Its three colour states are the named rule's. A utility for any one of them
	// would beat the rule that draws the other two — this is the cascade bug that
	// left the current section marked everywhere except in its colour.
	for _, never := range []string{"text-body", "hover:text-ink", "font-semibold"} {
		if strings.Contains(rail, never) {
			t.Errorf("rail link must not carry %q as a utility: %s", never, rail)
		}
	}
	// A plain link carries no download attribute.
	plain := render(t, r, &Link{Label: "Docs", Href: "/help"})
	if strings.Contains(plain, "download=") {
		t.Errorf("non-download link must not carry a download attribute: %s", plain)
	}
	external := render(t, r, &Link{Label: "Project website", Href: "https://example.com", NewTab: true})
	if !strings.Contains(external, `target="_blank" rel="noopener noreferrer"`) {
		t.Errorf("new-tab links must isolate the opener: %s", external)
	}
	// A link that leaves the router says so before its words, to the eye and to
	// a screen reader alike.
	glyph := strings.Index(external, `<path d="M15 3h6v6" />`)
	if glyph < 0 || glyph > strings.Index(external, "Project website") {
		t.Errorf("new-tab link must lead with the external-link glyph: %s", external)
	}
	if !strings.Contains(external, `<span class="sr-only">(opens in a new tab)</span>`) {
		t.Errorf("new-tab link must tell a screen reader where it goes: %s", external)
	}
	if strings.Contains(plain, `M15 3h6v6`) || strings.Contains(plain, "new tab") {
		t.Errorf("a link that stays must not claim to leave: %s", plain)
	}
}

func TestRenderButton(t *testing.T) {
	r := newRenderer(t)
	inert := render(t, r, &Button{Label: "Choose firmware…", Icon: "upload"})
	// The button block in no other dress is the pressing primary.
	for _, want := range []string{`type="button"`, `class="verso-button">`, "size-4", "Choose firmware…"} {
		if !strings.Contains(inert, want) {
			t.Errorf("inert button missing %q: %s", want, inert)
		}
	}
	if strings.Contains(inert, "name=") || strings.Contains(inert, "value=") {
		t.Errorf("inert button must not submit anything: %s", inert)
	}
	submit := render(t, r, &Button{Label: "Apply", Style: "secondary", Name: "_action", Value: "apply"})
	for _, want := range []string{`type="submit"`, `name="_action"`, `value="apply"`, `class="verso-button" data-verso-style="secondary">`} {
		if !strings.Contains(submit, want) {
			t.Errorf("submitting button missing %q: %s", want, submit)
		}
	}
	loading := render(t, r, &Button{Label: "Fetching sources", Icon: "refresh-cw", Style: "secondary", Loading: true})
	// Waiting is the canvas's one look whatever the style — sand ground, strong
	// hairline, glyph label, no pointer — on the button block's own footprint.
	// It is the one busy state the shell's script puts on a pressed button
	// (data-verso-busy), laid over the style's own dress, so a script ends a
	// wait the server drew by taking the attribute off.
	for _, want := range []string{`disabled`, `aria-disabled="true"`, `aria-busy="true"`, `data-verso-busy class="verso-button" data-verso-style="secondary">`, `data-verso-wait`, "Fetching sources", "verso-waiting__square"} {
		if !strings.Contains(loading, want) {
			t.Errorf("loading button missing %q: %s", want, loading)
		}
	}
	for _, never := range []string{lucideIcons["refresh-cw"], "bg-ground", "cursor-wait"} {
		if strings.Contains(loading, never) {
			t.Errorf("loading button must drop its icon and its press, found %q: %s", never, loading)
		}
	}
	css, err := os.ReadFile(filepath.Join("..", "server", "assets", "blocks", "button.css"))
	if err != nil {
		t.Fatal(err)
	}
	waiting := regexp.MustCompile(`\.verso-button\[data-verso-busy\] \{\s*pointer-events: none;\s*cursor: default;[^}]*--verso-button-edge: var\(--color-rule-strong\);\s*--verso-button-ground: var\(--color-quiet\);\s*--verso-button-ink: var\(--color-glyph\);`)
	if !waiting.Match(css) {
		t.Error("the busy rule does not take the pointer and lay the sand ground, strong hairline and glyph label over every style")
	}
	// The press stands down while the button is busy.
	if !strings.Contains(string(css), `.verso-button:not(:disabled, [aria-disabled="true"], [aria-busy="true"]):active {`) {
		t.Error("the button block presses a busy button")
	}
	// act: an act on a part of a section wears the subsection act's one dress,
	// the same as a Link's act, whether it leads somewhere or submits: the
	// button block's act size, the glyph's tighter inset.
	act := render(t, r, &Button{Label: "Use my computer's time", Icon: "clock", Style: "act", Name: "_action", Value: "clock"})
	for _, want := range []string{`type="submit"`, `name="_action"`, `class="verso-button group/act relative" data-verso-size="act" data-verso-glyph>`, lucideIcons["clock"]} {
		if !strings.Contains(act, want) {
			t.Errorf("act button missing %q: %s", want, act)
		}
	}
	for _, never := range []string{"data-verso-style="} {
		if strings.Contains(act, never) {
			t.Errorf("act button wears another dress's %q: %s", never, act)
		}
	}
	disabled := render(t, r, &Button{Label: "Apply", Disabled: true})
	for _, want := range []string{`disabled aria-disabled="true" class="verso-button">`} {
		if !strings.Contains(disabled, want) {
			t.Errorf("disabled button missing %q: %s", want, disabled)
		}
	}
	// A disabled button keeps the plain arrow, as a native one does: its look
	// already says it cannot be pressed, and a forbidding pointer would scold
	// someone who has only moved the mouse.
	declares(t, "button", `.verso-button:is(:disabled, [aria-disabled="true"])`, "cursor: default;")
	if strings.Contains(string(css), "not-allowed") {
		t.Error("a disabled button takes the forbidding pointer")
	}
	// Whether a button can be pressed reads at a glance: one that can wears
	// an edge, a secondary's words in Ink; one that cannot loses its edge
	// and rests as a slab of Quiet Sand, its words in Meta, whatever its
	// dress; on a ground that is itself Quiet Sand (a page's, a drawer's) the
	// slab is Mid Sand, so a form's Save under its last card still reads as a
	// button. Faint is never text, so it is the edge, not a paler word, that
	// tells them apart. An act keeps its Meta words, and a glyph or a quiet
	// act, which never had an edge to lose, only fades.
	declares(t, "button", `.verso-button[data-verso-style="secondary"]:not([data-verso-size])`, "--verso-button-ink: var(--color-ink);")
	declares(t, "button", `.verso-button:is(:disabled, [aria-disabled="true"]):not([data-verso-busy], [data-verso-size="icon"], [data-verso-size="glyph"], [data-verso-style="quiet"])`,
		"--verso-button-edge: var(--verso-button-slab, var(--color-quiet));", "--verso-button-ground: var(--verso-button-slab, var(--color-quiet));", "--verso-button-ink: var(--color-meta);")
	declares(t, "page", ".verso-page__ground", "--verso-button-slab: var(--color-mid);")
	declares(t, "modal", ".verso-modal__ground", "--verso-button-slab: var(--color-mid);")
	declares(t, "card", ".verso-card", "--verso-button-slab: var(--color-quiet);")
	declares(t, "button", `.verso-button:disabled:is([data-verso-size="icon"], [data-verso-size="glyph"], [data-verso-style="quiet"])`, "opacity: 0.7;")
	if rule := cssRule(blockCSS(t, "button"), `.verso-button:disabled:not([data-verso-busy], [data-verso-standby])`); strings.Contains(rule, "opacity") {
		t.Errorf("a disabled button with an edge to lose fades as well:\n%s", rule)
	}
}

// TestRenderLiveButton: the live state is the spinner without the surrender —
// something the button governs is running and saying so, and the button is
// still the way to stop it. Loading is the in-flight submit and keeps disabling;
// Live must not disable, must keep its hover and pointer, and puts its label in
// a hook the shell's stream client can rewrite (Pause ⇄ Resume · N new).
func TestRenderLiveButton(t *testing.T) {
	r := newRenderer(t)
	live := render(t, r, &Button{Label: "Pause", Style: "secondary", Live: true})
	for _, want := range []string{
		"data-verso-live",
		`data-verso-wait`, "verso-waiting__square",
		"<span data-verso-live-label>Pause</span>",
		// The button block in its secondary dress: hover intact, the pointer
		// saying "press me", and it presses.
		`data-verso-live class="verso-button" data-verso-style="secondary">`,
	} {
		if !strings.Contains(live, want) {
			t.Errorf("live button missing %q: %s", want, live)
		}
	}
	for _, unwanted := range []string{"disabled", "cursor-wait", "opacity-70", "data-verso-busy", "aria-busy"} {
		if strings.Contains(live, unwanted) {
			t.Errorf("live button must not take anything away (%q): %s", unwanted, live)
		}
	}
}

func TestRenderDisclosure(t *testing.T) {
	r := newRenderer(t)
	got := render(t, r, &Disclosure{Summary: "Server settings", Children: []Widget{&Field{Name: "port", Label: "Listen port"}}})
	for _, want := range []string{"<details", "<summary", "Server settings", "verso-chevron", `name="port"`} {
		if !strings.Contains(got, want) {
			t.Errorf("disclosure missing %q in: %s", want, got)
		}
	}
	if strings.Contains(got, "<script") || strings.Contains(got, "x-data") {
		t.Errorf("disclosure must be pure HTML/CSS: %s", got)
	}
	condition := render(t, r, &Disclosure{Style: "condition", Summary: "Advanced"})
	// The condition style sits a step in from the page's ground — on the quiet
	// band, because what it folds is part of the form around it — and holds
	// its words 24px in on every side, as the conditions card does.
	for _, want := range []string{
		`<details class="verso-disclosure" data-verso-style="condition">`,
		`<summary class="verso-disclosure__summary">`,
		`<div class="verso-disclosure__body verso-rhythm">`,
	} {
		if !strings.Contains(condition, want) {
			t.Errorf("condition disclosure missing %q: %s", want, condition)
		}
	}
	declares(t, "disclosure", ".verso-disclosure", "border: 1px solid var(--color-rule);")
	declares(t, "disclosure", `.verso-disclosure[data-verso-style="condition"]`, "background-color: var(--color-quiet);")
	declares(t, "disclosure", `.verso-disclosure__summary:where(.verso-disclosure[data-verso-style="condition"] *)`,
		"padding: calc(var(--spacing) * 6);", "color: var(--color-ink);")
	declares(t, "disclosure", ".verso-disclosure__body", "border-top-width: 1px;")
	declares(t, "disclosure", `.verso-disclosure__body:where(.verso-disclosure[data-verso-style="condition"] *)`,
		"padding: calc(var(--spacing) * 6);")
	if strings.Contains(got, "<details open") {
		t.Errorf("a disclosure not asked to open must start folded: %s", got)
	}
	open := render(t, r, &Disclosure{Open: true, Summary: "What changes"})
	if !strings.Contains(open, "<details open") {
		t.Errorf("an open disclosure must render expanded: %s", open)
	}
}

func TestRenderSection(t *testing.T) {
	r := newRenderer(t)
	got := render(t, r, &Section{Title: "Devices", Children: []Widget{&Badge{Variant: "success", Text: "x"}}})
	for _, want := range []string{"<section", "Devices", `data-verso-section="card"`, "x"} {
		if !strings.Contains(got, want) {
			t.Errorf("section missing %q in: %s", want, got)
		}
	}
	// A titleless section is valid — it still groups and spaces, with no heading.
	notitle := render(t, r, &Section{Children: []Widget{&Badge{Text: "y"}}})
	if strings.Contains(notitle, "<h3") {
		t.Errorf("titleless section should render no heading: %s", notitle)
	}
}

func TestRenderCode(t *testing.T) {
	r := newRenderer(t)
	got := render(t, r, &Code{Label: "Server public key", Value: "HIgo9xNzJM==", Copy: true})
	for _, want := range []string{
		`<span class="verso-code__path" data-verso-verbatim>Server public key</span>`,
		`<div class="verso-code__value verso-drawer-scrollbar">HIgo9xNzJM==</div>`,
		`x-data="copy"`, "Copy", "Copied", "text-green",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("code missing %q in: %s", want, got)
		}
	}
	// The value is a machine string, set as code and kept as written.
	declares(t, "code", ".verso-code__value", "white-space: pre", "@apply type-code")
	// Without copy, no copy button.
	nocopy := render(t, r, &Code{Value: "abc"})
	if strings.Contains(nocopy, "x-data") {
		t.Errorf("code without copy should have no copy button: %s", nocopy)
	}
	// A block that is not a live preview carries no marker, so the form watcher
	// leaves it alone: a public key is not a reading of the form beside it.
	if strings.Contains(got, "data-verso-preview") {
		t.Errorf("a plain code block must not read as a live preview: %s", got)
	}
}

// TestRenderCodeInUciGrammar: a block in the uci grammar is read a line at a
// time, the keyword and quotes set apart from the key and value, with the
// text's own whitespace kept, while the copy control still takes it whole.
func TestRenderCodeInUciGrammar(t *testing.T) {
	r := newRenderer(t)
	text := "config interface 'lan'\n\toption device 'br-lan'\n\tlist dns '9.9.9.9'\n\toption proto 'static'"
	block := &Code{Label: "/etc/config/network · interface", Value: text, Copy: true, Grammar: "uci"}
	// The box keeps whitespace as written, so the indent, the spaces between
	// the tokens and the line breaks are in the markup itself.
	var raw strings.Builder
	if err := r.RenderWithToken(&raw, block, "", "", nil); err != nil {
		t.Fatalf("RenderWithToken: %v", err)
	}
	got := raw.String()
	for _, want := range []string{
		// Read as an editor reads a file: the keyword in denim, a section's
		// type in amethyst, an option's name in Ink, its value in green, the
		// quotes in Meta; each line its own element, the whitespace its own.
		`<span data-verso-line><span class="text-denim-deep">config</span> <span class="text-amethyst-deep">interface</span> <span class="text-meta">'</span><span data-verso-value class="text-green-deep">lan</span><span class="text-meta">'</span></span>` + "\n" +
			`<span data-verso-line>` + "\t" + `<span class="text-denim-deep">option</span> device <span class="text-meta">'</span><span data-verso-value class="text-green-deep">br-lan</span><span class="text-meta">'</span></span>`,
		"\n" + `<span data-verso-line>` + "\t" + `<span class="text-denim-deep">list</span> dns `,
		// The file it goes into heads it, as an editor's tab does, with the
		// copy control on that line and the whole text behind it.
		`data-verso-code-head`,
		`<span class="verso-code__path" data-verso-verbatim>/etc/config/network · interface</span>`,
		`<span x-ref="src" class="hidden">config interface &#39;lan&#39;`,
		// A gutter numbers the lines, apart from the text, so selecting and
		// copying the text takes none of it.
		`data-verso-preview-gutter aria-hidden="true"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("uci code block missing %q in: %s", want, got)
		}
	}
	if n := strings.Count(got, "data-verso-gutter-line"); n != 4 {
		t.Errorf("one number a line, got %d: %s", n, got)
	}
	// Each line and its number stand on a 24px line, so a file of any length
	// closes its card on a line of the 8px grid.
	if !strings.Contains(got, `class="verso-code__gutter" data-verso-figures>`) ||
		!strings.Contains(got, `class="verso-code__text verso-drawer-scrollbar">`) {
		t.Errorf("the gutter and the text are not the code block's: %s", got)
	}
	declares(t, "code", ".verso-code__gutter", "@apply type-code")
	declares(t, "code", ".verso-code__text", "@apply type-code", "white-space: pre")
	typeCSS, err := os.ReadFile(filepath.Join("..", "server", "assets", "type.css"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(cssRule(string(typeCSS), "@utility type-code"), "line-height: 1.5rem;") {
		t.Errorf("the file's lines are not 24px: %s", cssRule(string(typeCSS), "@utility type-code"))
	}
	// Past 99 lines the gutter widens for a third figure.
	var long strings.Builder
	for i := 0; i < 100; i++ {
		long.WriteString("config interface 'lan'\n")
	}
	if wide := render(t, r, &Code{Value: long.String(), Grammar: "uci"}); !strings.Contains(wide, `class="verso-code__gutter" data-verso-figures data-verso-wide>`) {
		t.Errorf("a gutter past 99 lines is not the wide one: %s", wide)
	}
	declares(t, "code", ".verso-code__gutter[data-verso-wide]", "width: calc(var(--spacing) * 16)")
	// A file ends in a newline; the line it closes is not a line of its own.
	ended := &Code{Value: text + "\n", Grammar: "uci"}
	if n := len(ended.Lines()); n != 4 {
		t.Errorf("a trailing newline is not a line, got %d lines", n)
	}
	// The copy control's tip hangs above the head, outside the card, so the
	// card must not clip what stands outside it; its pieces round their own
	// corners instead.
	frame := got[strings.Index(got, "data-verso-code-editor"):]
	frame = frame[:strings.Index(frame, ">")]
	if strings.Contains(frame, "overflow-hidden") {
		t.Errorf("the card must not clip its copy control's tip: %s", frame)
	}
	if !strings.Contains(got, `data-verso-code-head class="verso-code__head"`) {
		t.Errorf("the head is not the code block's: %s", got)
	}
	declares(t, "code", ".verso-code__editor", "border-radius: var(--radius-xs)")
	if strings.Contains(cssRule(blockCSS(t, "code"), ".verso-code__editor"), "overflow") {
		t.Error("the card must not clip its copy control's tip")
	}
	declares(t, "code", ".verso-code__head", "height: calc(var(--spacing) * 10)",
		"border-radius: var(--radius-xs) var(--radius-xs) 0 0", "background-color: var(--color-quiet)")
	// The gutter rounds the corner it stands in: the bottom left under a head,
	// the top left as well with none.
	declares(t, "code", ".verso-code__gutter", "border-radius: 0 0 0 var(--radius-xs)")
	declares(t, "code", ".verso-code__gutter:where(.verso-code__editor:not(:has(> .verso-code__head)) *)",
		"border-top-left-radius: var(--radius-xs)")
	// The preview is a card of its own, on a page and in a drawer alike: it
	// stands 23px under the card before it, as one card stands under
	// another, its editor in the Strong Hairline frame a card wears.
	if !strings.Contains(got, `class="verso-code" data-verso-place="card"`) {
		t.Errorf("the preview does not stand as a card: %s", got)
	}
	declares(t, "code", `.verso-code[data-verso-place="card"]`,
		"--verso-step: calc(var(--spacing-inset) - 1px);", "margin-top: calc(var(--spacing-inset) - 1px);")
	declares(t, "code", `.verso-code__editor:where(.verso-code[data-verso-place="card"] > *)`, "border-color: var(--color-rule-strong);")
	// The path alone names the file: no glyph before it.
	if head := got[strings.Index(got, "data-verso-code-head"):strings.Index(got, "/etc/config/network")]; strings.Contains(head, "<svg") {
		t.Errorf("the head names the file by its path alone: %s", head)
	}
	if strings.Count(got, `text-denim-deep">option`) != 2 || strings.Count(got, `text-denim-deep">list`) != 1 {
		t.Errorf("every keyword is set apart: %s", got)
	}
	// An opaque value is shown whole, as before: no editor, no gutter.
	plain := render(t, r, &Code{Value: "config interface 'lan'"})
	if strings.Contains(plain, "text-denim-deep") || strings.Contains(plain, "data-verso-preview-gutter") {
		t.Errorf("a block without a grammar is one string: %s", plain)
	}
}

// TestUciLine: a uci line splits into its keyword, key and quoted value; a
// line shaped any other way stays raw.
func TestUciLine(t *testing.T) {
	for _, c := range []struct {
		raw  string
		want CodeLine
	}{
		{"\toption proto 'static'", CodeLine{Indent: "\t", Keyword: "option", Key: "proto", Value: "static", Quoted: true, Raw: "\toption proto 'static'"}},
		{"config device", CodeLine{Keyword: "config", Key: "device", Raw: "config device"}},
		{"\tlist ports 'lan 0'", CodeLine{Indent: "\t", Keyword: "list", Key: "ports", Value: "lan 0", Quoted: true, Raw: "\tlist ports 'lan 0'"}},
		{"\toption mtu 1500", CodeLine{Indent: "\t", Keyword: "option", Key: "mtu", Rest: "1500", Raw: "\toption mtu 1500"}},
		{"# a comment", CodeLine{Comment: true, Raw: "# a comment"}},
		{"\t# an indented comment", CodeLine{Comment: true, Raw: "\t# an indented comment"}},
		{"", CodeLine{Raw: ""}},
		{"option", CodeLine{Raw: "option"}},
	} {
		if got := uciLine(c.raw); got != c.want {
			t.Errorf("uciLine(%q) = %+v, want %+v", c.raw, got, c.want)
		}
	}
}

// TestRenderLivePreview: a code block declared live is the same block plus the
// marker the form watcher finds — and the renderer can answer with that one
// block alone, which is what a preview request gets back.
// A live preview may say in words what its lines do. The sentence stands
// inside the block, under the lines, so it is answered and replaced with them
// as the form is edited, and it is the last thing over the form's act.
func TestALivePreviewSaysWhatItDoes(t *testing.T) {
	r := newRenderer(t)
	says := "Accepts tcp/udp traffic from WAN to the router itself."
	got := render(t, r, &Code{Label: "/etc/config/firewall", Value: "config rule 'new'\n", Grammar: "uci", Live: true, Says: says})
	preview := got[strings.Index(got, "data-verso-preview"):]
	at := strings.Index(preview, says)
	if at < 0 || !strings.Contains(preview[:at], "config") {
		t.Fatalf("the sentence does not stand in the preview under its lines: %s", got)
	}
	if !strings.Contains(got, `data-verso-preview-says`) {
		t.Errorf("the sentence carries no hook: %s", got)
	}
	if plain := render(t, r, &Code{Value: "abc"}); strings.Contains(plain, "data-verso-preview-says") {
		t.Errorf("a block with nothing to say draws a sentence: %s", plain)
	}
}

func TestRenderLivePreview(t *testing.T) {
	r := newRenderer(t)
	block := &Code{Label: "/etc/config/firewall", Value: "config zone 'lan'\n", Copy: true, Live: true}
	got := render(t, r, block)
	for _, want := range []string{"data-verso-preview", "/etc/config/firewall", "config zone"} {
		if !strings.Contains(got, want) {
			t.Errorf("live preview missing %q in: %s", want, got)
		}
	}

	// The page around it is already on screen, so a preview request is answered
	// with the block and nothing else — not the form it describes.
	page := &Card{Children: []Widget{
		&Field{Name: "input", Label: "Traffic to the router", Kind: "text", Value: "ACCEPT"},
		block,
	}}
	var b strings.Builder
	found, err := r.RenderLivePreviewWithToken(&b, page, "tok", "", nil)
	if err != nil {
		t.Fatalf("RenderLivePreviewWithToken: %v", err)
	}
	if !found {
		t.Fatal("the page declares a live preview and none was found")
	}
	out := b.String()
	if !strings.Contains(out, "config zone") {
		t.Errorf("the answer is not the preview: %s", out)
	}
	if strings.Contains(out, `name="input"`) || strings.Contains(out, "Traffic to the router") {
		t.Errorf("the answer carries the page as well as the preview: %s", out)
	}

	// A page with no live preview has nothing to answer with, which the caller
	// needs to tell apart from an empty one.
	plain := &Card{Children: []Widget{&Code{Value: "abc"}}}
	var none strings.Builder
	switch found, err = r.RenderLivePreviewWithToken(&none, plain, "tok", "", nil); {
	case err != nil:
		t.Fatalf("RenderLivePreviewWithToken: %v", err)
	case found:
		t.Errorf("a page with no live preview reported one: %s", none.String())
	}

	// Only the first of several can be kept current, so the gauge counts them.
	two := &Card{Children: []Widget{block, &Code{Value: "x", Live: true}}}
	if n := LivePreviewCount(two); n != 2 {
		t.Errorf("LivePreviewCount = %d, want 2", n)
	}
	if n := LivePreviewCount(plain); n != 0 {
		t.Errorf("LivePreviewCount = %d, want 0", n)
	}

	// The body is marked apart from the block, because the eager patch writes
	// into it: a value typed into a control lands on its line before the
	// plugin's answer arrives, and it has to find the text to do that.
	if !strings.Contains(got, "data-verso-preview-body") {
		t.Errorf("a live preview offers nothing for the eager patch to write into: %s", got)
	}
	still := render(t, r, &Code{Value: "abc", Copy: true})
	if strings.Contains(still, "data-verso-preview-body") {
		t.Errorf("a block nothing keeps current must not be written into: %s", still)
	}
}

// TestFieldDeclaresTheOptionItWrites: the uci option a control writes travels to
// the browser as data, not only as the chip a reader sees. The live preview's
// eager patch is what needs it — it puts a typed value on that option's line
// without waiting for the round-trip — and it can only do that for a control
// that says which line is its own.
func TestFieldDeclaresTheOptionItWrites(t *testing.T) {
	r := newRenderer(t)
	got := render(t, r, &Field{Name: "dest_port", Label: "To port", Kind: "text", Key: "dest_port"})
	if !strings.Contains(got, `data-verso-writes="dest_port"`) {
		t.Errorf("field does not declare the option it writes: %s", got)
	}
	// A control that writes no single option claims none: the patch must not
	// guess a line for it.
	bare := render(t, r, &Field{Name: "filter", Label: "Find", Kind: "text"})
	if strings.Contains(bare, "data-verso-writes") {
		t.Errorf("a field with no option declared one anyway: %s", bare)
	}
}

func TestRenderModal(t *testing.T) {
	r := newRenderer(t)
	got := render(t, r, &Modal{
		Trigger:  "Add a device",
		Title:    "Add a device",
		Children: []Widget{&Field{Name: "device_name", Label: "Device name"}},
	})

	for _, want := range []string{
		`x-data="modal"`,         // the shell-owned Alpine component
		`@click="show"`,          // trigger opens it
		`x-teleport="body"`,      // dialog escapes the content flow
		`<dialog x-ref="dialog"`, // a native dialog: top layer, inert page, Escape
		`@cancel="dismiss"`,      // Escape closes it through the component
		"Add a device",           // trigger + title
		`name="device_name"`,     // the child widget is rendered inside
		"verso-modal",            // the canvas scrim, as its ::backdrop
	} {
		if !strings.Contains(got, want) {
			t.Errorf("modal missing %q in: %s", want, got)
		}
	}
	// The plugin ships no JS: the markup carries only directives, never a script.
	if strings.Contains(got, "<script") {
		t.Errorf("modal must not emit a script tag: %s", got)
	}
	if strings.Contains(got, `<header class="flex items-center justify-between border-b`) {
		t.Errorf("modal title and content should not be divided by a hairline: %s", got)
	}
}

func TestRenderPasswordField(t *testing.T) {
	r := newRenderer(t)
	got := render(t, r, &Field{Name: "password", Label: "New password", Kind: "password"})

	for _, want := range []string{
		`type="password"`, `name="password"`, `id="password"`,
		`autocomplete="new-password"`, `New password`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("password field missing %q in: %s", want, got)
		}
	}
	// The password must never be reflected: no value attribute on the input.
	if strings.Contains(got, "value=") {
		t.Errorf("password input must not carry a value attribute: %s", got)
	}
}

func TestRenderFieldError(t *testing.T) {
	r := newRenderer(t)

	got := render(t, r, &Field{Name: "h", Label: "H", Value: "bad host", Error: "must be a valid hostname"})
	if !strings.Contains(got, "must be a valid hostname") {
		t.Errorf("inline error not shown: %s", got)
	}
	// the field names itself as the refused one, focused too
	refusedBorder(t)
	if box := refusedBox(got); !strings.Contains(box, `class="verso-input"`) {
		t.Errorf("errored field's box is not the input block: %q", box)
	}
	for _, want := range []string{
		`data-verso-error class="verso-refusal">`,                                       // and the message sits on crimson's own ground
		`<span aria-hidden="true" class="verso-mark mt-2.25" data-verso-tone="danger">`, // with the mark beside it
	} {
		if !strings.Contains(got, want) {
			t.Errorf("errored field missing %q: %s", want, got)
		}
	}
	declares(t, "refusal", ".verso-refusal", "background-color: var(--color-crimson-soft);", "color: var(--color-crimson-deep);")
}

func TestRenderFieldFocus(t *testing.T) {
	r := newRenderer(t)
	got := render(t, r, &Field{Name: "hostname", Label: "Hostname"})
	if !strings.Contains(got, `class="verso-input"`) {
		t.Errorf("valid field is not the input block: %s", got)
	}
	// resting, pointed at, and focused: only the border changes — the strong
	// hairline, the faint step under the pointer, the action colour while
	// typing — as the canvas draws every input
	declares(t, "input", ".verso-input", "border: 1px solid var(--color-rule-strong);")
	declares(t, "input", ".verso-input:hover", "border-color: var(--color-faint);")
	declares(t, "input", ".verso-input:focus", "border-color: var(--color-denim);")
	for _, sel := range []string{".verso-input:hover", ".verso-input:focus"} {
		if rule := cssRule(blockCSS(t, "input"), sel); strings.Contains(rule, "background") || strings.Contains(rule, "shadow") {
			t.Errorf("%s changes more than the border:\n%s", sel, rule)
		}
	}
}

func TestRenderFieldEscapesValue(t *testing.T) {
	r := newRenderer(t)

	got := render(t, r, &Field{Name: "h", Label: "H", Value: `"><script>alert(1)</script>`})
	if strings.Contains(got, "<script>") {
		t.Errorf("field value not escaped: %s", got)
	}
}

// TestRenderListItemsPlusBlank proves the repeating widget: N item inputs plus a
// trailing blank add-slot, all sharing the name so they post as a multi-value
// field.
func TestRenderListItemsPlusBlank(t *testing.T) {
	r := newRenderer(t)

	got := render(t, r, &List{Name: "server", Label: "NTP servers", Datatype: "host",
		Items: []string{"0.pool.ntp.org", "1.pool.ntp.org"}})
	if n := strings.Count(got, ` name="server" value=`); n != 3 {
		t.Errorf("want 3 inputs (2 items + 1 blank), got %d: %s", n, got)
	}
	for _, want := range []string{
		`data-verso-change-name="server"`, `data-verso-change-kind="list"`,
		"NTP servers", "0.pool.ntp.org", "1.pool.ntp.org", "Add",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("list missing %q", want)
		}
	}
	// every item, the blank one too, is the input block's box
	if n := strings.Count(got, `class="verso-input" data-verso-verbatim`); n != 3 {
		t.Errorf("want 3 input boxes, got %d: %s", n, got)
	}
}

func TestRenderTokenListKeepsRepeatedFieldContract(t *testing.T) {
	got := render(t, newRenderer(t), &List{
		Name: "dest_port", Label: "Ports", Style: "tokens", Prompt: "Port or range",
		Items: []string{"53", "67", "547"},
	})
	// Three values and the empty chip the shell clones for a fourth, which is
	// inert template content and posts nothing until it is filled and placed.
	if n := strings.Count(got, `type="hidden" name="dest_port"`); n != 4 {
		t.Errorf("token values must share the field name: got %d\n%s", n, got)
	}
	if n := strings.Count(got, `type="hidden" name="dest_port" value=""`); n != 1 {
		t.Errorf("want exactly one empty chip to clone, got %d\n%s", n, got)
	}
	for _, want := range []string{
		`data-verso-token-list`, `data-verso-token-input`, `placeholder="Port or range"`, `value="547"`,
		// The chip the script clones is the chip the server draws, to the class:
		// one source for a token's shape, not one here and one in the script.
		`<template data-verso-token-chip>`, `data-verso-token-label`,
		`<div data-verso-token-list class="verso-tokens">`,
		`class="verso-tokens__input" data-verso-verbatim`,
		`<span data-verso-token class="verso-tokens__chip" data-verso-verbatim>`,
		// Its remove glyph takes the hover every icon act takes: the parked
		// border, the hairline wash and Ink, the button block at its glyph
		// size.
		`class="verso-button verso-tokens__remove" data-verso-size="glyph">`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("token list missing %q:\n%s", want, got)
		}
	}
	// 24px inside a 34px box, with 4px of the box showing all round and
	// between chips, wrapped rows 16px apart so the box stays on the grid's
	// cells — and a 20px remove glyph wearing the hover every icon act wears.
	// The box is a field, so it wears a field's fill: white, with the inset
	// shadow, and a field's height.
	declares(t, "tokens", ".verso-tokens", "display: flex;", "flex-wrap: wrap;", "align-items: center;",
		"gap: calc(var(--spacing) * 4) var(--spacing);", "min-height: var(--spacing-control);",
		"padding: calc(var(--spacing) * 0.75);", "border: 1px solid var(--color-rule-strong);",
		"border-radius: var(--radius-xs);", "background-color: var(--color-canvas);", "box-shadow: var(--shadow-field);")
	// the value being typed stands at the chips' height
	declares(t, "tokens", ".verso-tokens__input", "height: calc(var(--spacing) * 6);",
		"min-width: calc(var(--spacing) * 18);", "flex: 1 1 0%;")
	// A value in the box is a step darker than the quiet ground a chip wears
	// elsewhere, so it stands off the white box it sits in; its remove glyph's
	// hover is a step darker again, so it shows on the chip. The value is set
	// as it was typed: the displayed value's role, verbatim.
	declares(t, "tokens", ".verso-tokens__chip", "display: inline-flex;", "height: calc(var(--spacing) * 6);",
		"gap: calc(var(--spacing) * 1.5);", "padding-inline: calc(var(--spacing) * 2);",
		"border: 1px solid var(--color-rule-strong);", "background-color: var(--color-mid);",
		"color: var(--color-ink);", "white-space: nowrap;", "@apply type-value;")
	declares(t, "tokens", ".verso-tokens__remove[data-verso-size]", "width: calc(var(--spacing) * 5);",
		"height: calc(var(--spacing) * 5);", "margin-right: calc(var(--spacing) * -1);")
}

// TestRenderListPerItemError shows how a repeating widget reports which row
// failed: an error keyed by the item index.
func TestRenderListPerItemError(t *testing.T) {
	r := newRenderer(t)

	got := render(t, r, &List{Name: "server", Label: "NTP", Items: []string{"ok.example.com", "bad host"},
		Errors: map[string]string{"1": "must be a hostname or IP address"}})
	if !strings.Contains(got, "must be a hostname or IP address") {
		t.Errorf("per-item error missing: %s", got)
	}
	refusedBorder(t)
	if box := refusedBox(got); !strings.Contains(box, `value="bad host"`) || !strings.Contains(box, `class="verso-input"`) {
		t.Errorf("the refused item is not the input block's box: %q", box)
	}
	if !strings.Contains(got, `data-verso-error class="verso-refusal`) {
		t.Errorf("errored list item's message is not the refusal: %s", got)
	}
	// the message reads in the step of the hue that carries words
	declares(t, "refusal", ".verso-refusal", "color: var(--color-crimson-deep);")
}

func TestRenderFormDefaultsSubmitLabel(t *testing.T) {
	r := newRenderer(t)

	got := render(t, r, &Form{Fields: []Widget{&Field{Name: "h", Label: "H"}}})
	if !strings.Contains(got, `<form method="post"`) {
		t.Errorf("no post form: %s", got)
	}
	// A form that names no act still says what its button does: it saves the
	// changes typed into it. No button says a bare "Save".
	if !strings.Contains(got, ">Save changes<") || strings.Contains(got, ">Save<") {
		t.Errorf("default submit label 'Save changes' missing: %s", got)
	}
	if !strings.Contains(got, `name="h"`) {
		t.Errorf("nested field not rendered inside form: %s", got)
	}
}

func TestRenderFormSubmit(t *testing.T) {
	r := newRenderer(t)

	got := render(t, r, &Form{Submit: "Apply"})
	if !strings.Contains(got, ">Apply<") {
		t.Errorf("custom submit label missing: %s", got)
	}
	// The button block in no other dress is the primary, and it presses.
	if !strings.Contains(got, `<button type="submit" class="verso-button">Apply</button>`) {
		t.Errorf("form submit missing the primary-button treatment: %s", got)
	}
}

func TestRenderRawMarkdown(t *testing.T) {
	r := newRenderer(t)

	got := render(t, r, &Raw{Markdown: "**bold** and `code`"})
	for _, want := range []string{
		`<div class="verso-raw"><div class="verso-raw__kicker">Raw</div><div class="verso-prose text-ink">`,
		"<strong>bold</strong>",
		// Inline code is a machine string, set in the prose's verbatim form.
		`<code data-verso-verbatim="">code</code>`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("raw output missing %q: %s", want, got)
		}
	}
	declares(t, "prose", ".verso-prose :not(pre) > code", "@apply type-body")
	declares(t, "raw", ".verso-raw", "border: 1px dashed var(--color-rule-strong)")
	declares(t, "raw", ".verso-raw__kicker", "@apply type-label")
}

// TestRenderRawSanitizes is the bridge's safety net: source HTML is neutralised
// and dangerous URL schemes are stripped, while a safe link survives.
func TestRenderRawSanitizes(t *testing.T) {
	r := newRenderer(t)

	got := render(t, r, &Raw{Markdown: "<script>alert(1)</script>\n\n[x](javascript:alert(2)) and [ok](https://example.com)"})
	if strings.Contains(got, "<script>") {
		t.Errorf("raw HTML not neutralised: %s", got)
	}
	if strings.Contains(got, "javascript:") {
		t.Errorf("javascript: scheme not stripped: %s", got)
	}
	if !strings.Contains(got, `href="https://example.com"`) {
		t.Errorf("safe link was dropped: %s", got)
	}
}

// TestRenderWithTokenInjectsCSRF: a form rendered with a token carries a hidden
// _csrf field; plain Render omits it (VS-04).
func TestRenderWithTokenInjectsCSRF(t *testing.T) {
	r := newRenderer(t)
	form := &Form{Fields: []Widget{&Field{Name: "h", Label: "H"}}}

	var withTok strings.Builder
	if err := r.RenderWithToken(&withTok, form, "tok123", "", nil); err != nil {
		t.Fatalf("RenderWithToken: %v", err)
	}
	if !strings.Contains(withTok.String(), `name="_csrf" value="tok123"`) {
		t.Errorf("csrf hidden field missing: %s", withTok.String())
	}

	var plain strings.Builder
	if err := r.RenderWithToken(&plain, form, "", "", nil); err != nil {
		t.Fatalf("RenderWithToken: %v", err)
	}
	if strings.Contains(plain.String(), "_csrf") {
		t.Errorf("empty-token render must not inject a csrf field: %s", plain.String())
	}
}

// Note: there is no "unknown widget type" render test any more. Dispatch is
// polymorphic (each widget implements renderInto), so an unhandled type can't be
// constructed — the compiler enforces it. The failure mode the old switch's default
// branch guarded no longer exists.

// A part's heading is one level under the nearest titled section over it, so
// the page's outline never skips: a part under a titled section is h3, and a
// part whose section carries no title (an expanded row's, under the page's h1)
// is h2.
func TestAPartsHeadingNeverSkipsALevel(t *testing.T) {
	r := newRenderer(t)
	part := &Section{Title: "Network", Children: []Widget{&Text{Markdown: "lan"}}}
	under := render(t, r, &Section{Title: "Interfaces", Children: []Widget{part}})
	if !strings.Contains(under, `<h3 class="verso-section__part-title">Network</h3>`) {
		t.Errorf("a part under a titled section is not h3: %s", under)
	}
	bare := render(t, r, &Section{Children: []Widget{part}})
	if !strings.Contains(bare, `<h2 class="verso-section__part-title">Network</h2>`) {
		t.Errorf("a part under an untitled section skips to h3: %s", bare)
	}
}
