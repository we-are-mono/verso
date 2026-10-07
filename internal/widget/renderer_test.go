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
	if !strings.Contains(got, "border-crimson-line bg-crimson-soft") || !strings.Contains(got, "text-crimson-deep") {
		t.Errorf("form-level error not styled as danger: %s", got)
	}
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
// first and last of them as if they stood in it directly (sections.css).
func TestAFlushStackIsARunOfRows(t *testing.T) {
	got := render(t, newRenderer(t), &Stack{Flush: true, Children: []Widget{&Badge{Text: "a"}}})
	if !strings.HasPrefix(got, `<div data-verso-rows class="">`) {
		t.Errorf("a flush stack does not say it is a run of rows: %s", got)
	}
	if plain := render(t, newRenderer(t), &Stack{Children: []Widget{&Badge{Text: "a"}}}); strings.Contains(plain, "data-verso-rows") {
		t.Errorf("a spaced stack is blocks, not a run of rows: %s", plain)
	}
}

func TestRenderStackDivided(t *testing.T) {
	r := newRenderer(t)
	plain := render(t, r, &Stack{Children: []Widget{&Badge{Text: "a"}, &Badge{Text: "b"}}})
	// The default stack sets its blocks a cell of the notebook's grid apart.
	if !strings.Contains(plain, "space-y-5") {
		t.Errorf("default stack should space its children a cell apart:\n%s", plain)
	}
	div := render(t, r, &Stack{Divided: true, Children: []Widget{&Badge{Text: "a"}, &Badge{Text: "b"}}})
	if !strings.Contains(div, "divide-y") {
		t.Errorf("divided stack should draw hairlines:\n%s", div)
	}
	compact := render(t, r, &Stack{Compact: true, Children: []Widget{&Badge{Text: "a"}, &Badge{Text: "b"}}})
	if !strings.Contains(compact, "space-y-3") || strings.Contains(compact, "space-y-5") {
		t.Errorf("compact stack should use tighter spacing: %s", compact)
	}
	// An inline stack is a wrapping row of acts: 12px between them, its
	// wrapped rows two cells apart, and the row centred in two cells, 3px
	// over and under a control's 34px.
	inline := render(t, r, &Stack{Inline: true, Children: []Widget{&Badge{Text: "a"}, &Text{Markdown: "or"}, &Badge{Text: "b"}}})
	if !strings.Contains(inline, "flex flex-wrap items-center gap-x-3 gap-y-1.5 py-0.75") || !strings.Contains(inline, ">or</p>") {
		t.Errorf("inline stack should keep its children in one wrapping row: %s", inline)
	}
	if strings.Contains(div, "space-y-") {
		t.Errorf("divided stack should not also space:\n%s", div)
	}
	bounded := render(t, r, &Stack{Width: "compact", Children: []Widget{&Badge{Text: "a"}}})
	if !strings.Contains(bounded, "max-w-md") {
		t.Errorf("compact-width stack must remain visually bounded: %s", bounded)
	}
}

func TestRenderCardChrome(t *testing.T) {
	r := newRenderer(t)

	got := render(t, r, &Card{Title: "empty"})
	want := `<section><div class="mb-5"><h3 class="text-lg font-semibold tracking-tight leading-5 text-ink">empty</h3></div><div class="space-y-5"></div></section>`
	if got != want {
		t.Errorf("Render mismatch:\n got: %s\nwant: %s", got, want)
	}
}

func TestRenderCardSubtitle(t *testing.T) {
	r := newRenderer(t)
	got := render(t, r, &Card{Title: "Your gateway", Subtitle: "the back of the box"})
	if !strings.Contains(got, `<h3 class="text-lg font-semibold tracking-tight leading-5 text-ink">Your gateway</h3>`) {
		t.Errorf("card title missing:\n%s", got)
	}
	if !strings.Contains(got, `<p class="text-sm leading-5 text-body">the back of the box</p>`) {
		t.Errorf("card subtitle missing or not styled as a subtitle:\n%s", got)
	}
}

func TestRenderCardWithoutTitleOmitsHeader(t *testing.T) {
	r := newRenderer(t)

	got := render(t, r, &Card{})
	want := `<section><div class="space-y-5"></div></section>`
	if got != want {
		t.Errorf("Render mismatch:\n got: %s\nwant: %s", got, want)
	}
}

// TestRenderCardNestsChild proves a container renders its children through the
// same renderer, inside its own body — the composition the schema bet depends on.
func TestRenderCardNestsChild(t *testing.T) {
	r := newRenderer(t)
	card := &Card{
		Title:    "Status",
		Children: []Widget{&Table{Columns: []TableColumn{{Label: "A"}}, Rows: []TableRow{{Cells: []TableCell{{Text: "1"}}}}}},
	}

	got := render(t, r, card)
	body := strings.Index(got, `<div class="space-y-5">`)
	table := strings.Index(got, "<table")
	if body < 0 || table < 0 || table < body {
		t.Errorf("nested table not rendered inside card body: %s", got)
	}
}

func TestRenderCardNestingDepth(t *testing.T) {
	r := newRenderer(t)
	card := &Card{Title: "outer", Children: []Widget{
		&Card{Title: "inner", Children: []Widget{
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

func TestRenderCardEscapesTitle(t *testing.T) {
	r := newRenderer(t)

	got := render(t, r, &Card{Title: `<script>alert(1)</script>`})
	if strings.Contains(got, "<script>") {
		t.Errorf("card title not escaped: %s", got)
	}
	if !strings.Contains(got, "&lt;script&gt;") {
		t.Errorf("escaped title missing: %s", got)
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
		"Connected", "size-1.5 shrink-0 rounded-[1px]",
		"border-green-line bg-green-soft text-green-deep", // light treatment is unchanged
		"rounded-xs border",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("success badge missing %q in: %s", want, got)
		}
	}
	// An unknown/neutral variant falls back to sand, never leaks the variant
	// name. A pill with no hue — the firewall's drop — is the neutral chip, the
	// same box and colours as every other chip without a hue.
	neutral := render(t, r, &Badge{Variant: "neutral", Text: "Offline"})
	if !strings.Contains(neutral, chipMonoBox+"\n  border-rule bg-quiet text-meta") {
		t.Errorf("neutral badge should be the neutral chip: %s", neutral)
	}
	if strings.Contains(neutral, "size-1.5") {
		t.Errorf("badge without Dot should not render a dot: %s", neutral)
	}
}

// TestAFocusedBoxStaysFocusedUnderThePointer: a box whose focus is inside
// it (a field of value chips, the page filter, a file's drop area) turns
// denim while focused whether or not the pointer is over it — its hover
// colour applies only while it is not focused, as a plain field's does.
func TestAFocusedBoxStaysFocusedUnderThePointer(t *testing.T) {
	r := newRenderer(t)
	for name, w := range map[string]Widget{
		"tokens": &List{Name: "proto", Label: "Protocol", Style: "tokens"},
		"filter": &Filter{},
		"file":   &Field{Name: "backup", Kind: "file"},
	} {
		got := render(t, r, w)
		if !strings.Contains(got, "hover:not-focus-within:border-") || !strings.Contains(got, "focus-within:border-denim") {
			t.Errorf("%s: hover must not outrank focus:\n%s", name, got)
		}
	}
}

// TestACompactNoteKeepsItsControlsMeasure: a compact note explains the
// control it sits under, so it stands at that control's measure — the form's
// — rather than running past it; it leads with its tone's small square.
func TestACompactNoteKeepsItsControlsMeasure(t *testing.T) {
	got := render(t, newRenderer(t), &Callout{Variant: "warning", Compact: true, Body: "Named once."})
	if !strings.Contains(got, `flex w-[calc(100%_+_1px)] max-w-[calc(var(--container-3xl)_+_1px)] items-start gap-2 rounded-xs`) {
		t.Errorf("a compact note keeps the form's measure, its frame a pixel out:\n%s", got)
	}
	if !strings.Contains(got, "size-1.5 flex-none") || !strings.Contains(got, "bg-marigold") {
		t.Errorf("a compact note leads with its tone's square:\n%s", got)
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
		`class="inline-flex w-fit max-w-full flex-wrap gap-0.5 self-start rounded-xs border border-rule-strong bg-quiet p-0.5">`,
		`<label class="flex h-7.5 cursor-pointer items-center rounded-xs px-3.5 text-sm font-normal text-body transition-colors hover:text-ink has-checked:bg-choice has-checked:font-semibold has-checked:text-white has-focus-visible:outline-2`,
	} {
		if !strings.Contains(days, want) {
			t.Errorf("segmented field missing %q:\n%s", want, days)
		}
	}
	if strings.Contains(days, "has-checked:bg-body") {
		t.Errorf("a selected chip must use the shared choice colour:\n%s", days)
	}
	if !strings.Contains(days, `type="checkbox" name="days" value="mon" checked`) {
		t.Errorf("a day in the set is a checked box:\n%s", days)
	}
	// Checking a chip thickens its word, and a thicker word is a wider one:
	// every chip holds its semibold width from the start, a hidden twin in the
	// same cell, so checking one never nudges the rest along the strip.
	for _, want := range []string{
		`<span class="grid"><span class="col-start-1 row-start-1">Tue</span><span aria-hidden="true" class="invisible col-start-1 row-start-1 font-semibold">Tue</span></span>`,
	} {
		if !strings.Contains(days, want) {
			t.Errorf("segmented chip does not hold its checked width %q:\n%s", want, days)
		}
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
		"verso-empty-icon",
		"Reach your home from anywhere",
		"Set up a private VPN so your devices can reach home.",
		"Set up home VPN", // the CTA (a modal trigger) rendered as a child
		`x-data="modal"`,  // the CTA is a real composed widget
	} {
		if !strings.Contains(got, want) {
			t.Errorf("empty missing %q in: %s", want, got)
		}
	}
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
		if strings.Contains(got, "verso-live-dot") {
			t.Errorf("a %s chip's packet must not pulse: %s", variant, got)
		}
	}
}

func TestRenderDivider(t *testing.T) {
	r := newRenderer(t)
	plain := render(t, r, &Divider{})
	if !strings.Contains(plain, `<hr class="border-rule my-20">`) {
		t.Errorf("divider should be a bare rule at the page gap: %s", plain)
	}
	// Tight uses compact spacing for inline group separators, not the page-scale gap.
	tight := render(t, r, &Divider{Tight: true})
	if !strings.Contains(tight, "my-6") || strings.Contains(tight, "my-20") {
		t.Errorf("tight divider should use compact spacing: %s", tight)
	}
}

func TestRenderProperties(t *testing.T) {
	r := newRenderer(t)
	got := render(t, r, &Properties{Items: []Property{
		{Label: "Can reach", Value: "My whole home network"},
		{Label: "Tunnel address", Value: "10.7.0.2", Mono: true},
		{Label: "Server key", Value: "KEY==", Copy: true},
	}})
	for _, want := range []string{"<dl", "Can reach", "My whole home network", "Tunnel address", "10.7.0.2", "font-mono"} {
		if !strings.Contains(got, want) {
			t.Errorf("properties missing %q in: %s", want, got)
		}
	}
	// A non-mono value carries no font-mono on its own row.
	if strings.Count(got, "font-mono") != 1 {
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
	// line of the grid; the last row keeps that pixel as air instead.
	const hairlineRow = "border-b border-mid pt-2.5 pb-2.25 leading-5 last:border-b-0 last:pb-2.5"
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
	if !strings.Contains(bare, "space-y-3") || strings.Contains(bare, "divide-y") || strings.Contains(bare, "odd:bg-quiet") {
		t.Errorf("plain properties should have no separators:\n%s", bare)
	}
	identity := render(t, r, &Properties{Style: "identity", Items: []Property{{
		Label: "Username", Value: "root", Mono: true, Emphasis: true, Help: "Main system username cannot be changed.",
	}}})
	for _, want := range []string{"text-base", ">Username:<", "text-lg", "font-semibold", "font-mono", ">root<", "Main system username cannot be changed."} {
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
		// The 44px row: the value's 24px line inset 10px, the label raised to
		// the same line, and the value a flex line so its copy control cannot
		// lift it.
		"space-y-0", "border-t border-mid py-2", "text-sm leading-6 text-meta",
		"flex items-center text-sm font-medium text-ink", "font-mono text-base font-medium",
	} {
		if !strings.Contains(system, want) {
			t.Errorf("system properties missing %q:\n%s", want, system)
		}
	}
	emphasised := render(t, r, &Properties{Items: []Property{
		{Label: "Installed", Value: "Mono OpenWrt 25.12", Emphasis: true},
		{Label: "Target", Value: "aarch64_generic", Mono: true, Emphasis: true},
	}})
	for _, want := range []string{
		`<dt class="shrink-0 text-meta">Installed</dt>`,
		`<dd class="relative flex min-w-0 items-start justify-end text-right text-base font-medium text-ink [&_*]:leading-5">`,
		`class="min-w-0 wrap-anywhere text-balance font-mono text-base font-medium">aarch64_generic</span>`,
	} {
		if !strings.Contains(emphasised, want) {
			t.Errorf("emphasised properties missing %q:\n%s", want, emphasised)
		}
	}
}

// TestRenderPropertyVariantTonesTheValue: a row may declare a semantic tone on
// its value — the badge vocabulary, never a colour (ADR-005) — and the shell
// maps it to the same text treatment a badge of that tone wears, in both
// themes. The tone rides the value, not the row: the label stays quiet ink.
// A tone the vocabulary does not hold renders plainly, mechanically.
func TestRenderPropertyVariantTonesTheValue(t *testing.T) {
	r := newRenderer(t)
	toned := render(t, r, &Properties{Style: "system", Items: []Property{
		{Label: "Installed build", Value: "25.12.4", Mono: true, Emphasis: true, Variant: "warning"},
		{Label: "Available build", Value: "25.12.5", Mono: true, Emphasis: true, Variant: "success"},
	}})
	for _, want := range []string{
		`class="font-mono text-base font-medium text-marigold-deep">25.12.4</span>`,
		`class="font-mono text-base font-medium text-green-deep">25.12.5</span>`,
	} {
		if !strings.Contains(toned, want) {
			t.Errorf("toned system properties missing %q:\n%s", want, toned)
		}
	}
	// The whole vocabulary resolves, in the default row shape too.
	// A tone carries words in the step of its hue that can carry them — the
	// full-chroma value stays a mark.
	for variant, want := range map[string]string{
		"success": "text-green-deep",
		"warning": "text-marigold-deep",
		"danger":  "text-crimson-deep",
		"info":    "text-denim-deep",
	} {
		got := render(t, r, &Properties{Items: []Property{{Label: "State", Value: "x", Variant: variant}}})
		if !strings.Contains(got, want) {
			t.Errorf("the %q tone should render %q:\n%s", variant, want, got)
		}
	}
	// No variant, and an unknown one, both keep the ordinary ink.
	for _, variant := range []string{"", "chartreuse"} {
		got := render(t, r, &Properties{Items: []Property{{Label: "State", Value: "x", Variant: variant}}})
		for _, unwanted := range []string{"text-green-deep", "text-marigold-deep", "text-crimson-deep", "text-denim-deep"} {
			if strings.Contains(got, unwanted) {
				t.Errorf("variant %q should tint nothing, found %q:\n%s", variant, unwanted, got)
			}
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
		"border-marigold-line bg-marigold-soft", "text-marigold-deep", "text-marigold", "hover:border-marigold",
		"border-marigold-deep bg-marigold-deep text-white", "hover:bg-marigold-line/50",
		"Download and install 25.12.5", "Install 25.12.5 now?", "Install firmware", "Not yet",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("caution confirm missing %q in: %s", want, got)
		}
	}
	if strings.Contains(got, "crimson") {
		t.Errorf("a caution confirm must carry no crimson: %s", got)
	}
	if danger := render(t, r, &Confirm{Trigger: "Delete", Message: "Sure?"}); !strings.Contains(danger, `data-verso-confirm-tone="danger"`) {
		t.Errorf("a confirm without a tone is the danger one: %s", danger)
	}
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
		"border-crimson-line bg-crimson-soft", "text-crimson-deep", "size-1.5 flex-none\n    bg-crimson",
		// The question is the compact callout in the act's tone: its frame on
		// the grid's lines, its words half a cell inside.
		`<div data-verso-callout class="-mt-px -ml-px flex w-[calc(100%_+_1px)] max-w-[calc(var(--container-3xl)_+_1px)] items-start gap-2 rounded-xs border px-3 pt-2.5 pb-2.25`,
		// The answers are the band's acts, on the line a cell under the words;
		// cancel keeps the explanation's tone and hovers by a soft wash, not a
		// heavy colour darken.
		`<div data-verso-callout-acts class="mt-5 flex flex-wrap items-center gap-x-3 gap-y-1.5 pb-1.5"><button type="submit" x-ref="first"`, "hover:bg-crimson-line/50",
		"border-crimson bg-crimson text-white",
		"verso-press",
		// The trigger is a control like every other button: h-control holds
		// the border inside the height, where a line plus padding would add 2px,
		// and it is as wide as its words, never the column.
		"flex h-control w-fit cursor-pointer items-center justify-center rounded-xs border px-4",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("confirm missing %q in: %s", want, got)
		}
	}
	// No checkbox pretending to be a button, and no inline script: the
	// behaviour is the shell's confirm component.
	if strings.Contains(got, "<script") || strings.Contains(got, `type="checkbox"`) {
		t.Errorf("confirm must use the shell component, not a checkbox or script: %s", got)
	}
	if strings.Contains(got, `@click="cancel" class="flex h-control shrink-0 cursor-pointer items-center rounded-xs border`) {
		t.Errorf("confirm cancel must not render as a bordered button: %s", got)
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
	if !strings.Contains(titled, `<p class="font-semibold">Install now?</p>`) {
		t.Errorf("a titled confirm should render its heading bold: %s", titled)
	}
	if !strings.Contains(titled, "It will be unavailable for several minutes.") {
		t.Errorf("a titled confirm should still render its message: %s", titled)
	}
	// Caution asks in the warning's marigold band.
	if !strings.Contains(titled, "border-marigold-line bg-marigold-soft text-marigold-deep") || strings.Contains(titled, "bg-crimson-soft") {
		t.Errorf("a caution confirm should ask in the marigold band: %s", titled)
	}
	// The trigger and the confirm button are semibold like every other control;
	// what an untitled confirm must not draw is the heading paragraph.
	if strings.Contains(def, `<p class="font-semibold">`) {
		t.Errorf("an untitled confirm should draw no heading: %s", def)
	}
}

func TestRenderCallout(t *testing.T) {
	r := newRenderer(t)
	// A tone at full chroma is a mark and never sets type: the band is the tone's
	// soft ground with its deep step carrying the words, and the standing form is
	// framed by the tone's own hairline rather than marked.
	ok := render(t, r, &Callout{Variant: "success", Title: "Reachable", Body: "Verified from the internet."})
	for _, want := range []string{
		"border-green-line bg-green-soft text-green-deep",
		"Reachable", "Verified from the internet.",
	} {
		if !strings.Contains(ok, want) {
			t.Errorf("success callout missing %q in: %s", want, ok)
		}
	}
	// No glyph anywhere: the colour and the mark already say what kind of thing
	// this is, and a triangle beside three words of prose only crowds them.
	if strings.Contains(ok, "<svg") {
		t.Errorf("a callout carries no icon: %s", ok)
	}
	// Unknown/default variant falls back to info (denim), never leaks the variant name.
	def := render(t, r, &Callout{Body: "heads up"})
	if !strings.Contains(def, "border-denim-line bg-denim-soft text-denim-deep") {
		t.Errorf("default callout should use the info palette: %s", def)
	}
	warn := render(t, r, &Callout{Variant: "warning", Body: "x"})
	if !strings.Contains(warn, "bg-marigold-soft text-marigold-deep") {
		t.Errorf("warning callout should use marigold: %s", warn)
	}
	neutral := render(t, r, &Callout{Variant: "neutral", Compact: true, Body: "Quiet context."})
	if !strings.Contains(neutral, "bg-quiet text-body") || !strings.Contains(neutral, "bg-glyph") {
		t.Errorf("neutral callout should be the quiet band: %s", neutral)
	}
	// Compact is the note under a control: the mark on the first line's optical
	// centre, framed one step darker than its ground as every band is, without
	// the standing band's padding.
	compact := render(t, r, &Callout{Compact: true, Body: "A short note."})
	for _, want := range []string{"items-start", "gap-2", "px-3", "pt-2.5 pb-2.25", "border-denim-line", "mt-[0.4375rem] size-1.5", "bg-denim", "A short note."} {
		if !strings.Contains(compact, want) {
			t.Errorf("compact callout missing %q in: %s", want, compact)
		}
	}
	if strings.Contains(compact, "px-5") {
		t.Errorf("compact callout should not carry the standing padding: %s", compact)
	}
}

func TestRenderLink(t *testing.T) {
	r := newRenderer(t)
	// A data: URL is never a link's destination.
	if data := render(t, r, &Link{Label: "Config", Href: "data:text/plain,abc"}); !strings.Contains(data, `href="#"`) {
		t.Errorf("a data: link must collapse to #: %s", data)
	}
	primary := render(t, r, &Link{Label: "Download backup", Href: "/backup", Style: "button"})
	if !strings.Contains(primary, "verso-press") {
		t.Errorf("primary button link missing tactile pressed state: %s", primary)
	}
	secondary := render(t, r, &Link{Label: "Restart router", Href: "/restart", Style: "secondary"})
	for _, want := range []string{
		"hover:border-sand-5 hover:bg-rule",
		"border-rule-strong bg-transparent text-meta",
		"verso-press",
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
		`href="#section-speed"`, "data-verso-rail-link", "verso-rail-link",
		// a cell and a half each: two cells is too loose for a run of 14px
		// words and one too tight. The run reaches its padding out at either
		// end, so every second link's words stand in a cell and the ones
		// between straddle a line; the hairline a pixel out onto the column's
		"-ml-px flex items-center gap-1.5 border-l border-rule py-1.25 pl-4 text-sm leading-5",
		// and ends on a line whatever the count
		"first:-mt-1.25 last:-mb-1.25 [&:nth-child(even):last-child]:mb-1.25",
	} {
		if !strings.Contains(rail, want) {
			t.Errorf("rail link missing %q: %s", want, rail)
		}
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
	for _, want := range []string{`type="button"`, "border-denim bg-denim text-white", "hover:border-denim-deep hover:bg-denim-deep", "verso-press", "size-4", "Choose firmware…"} {
		if !strings.Contains(inert, want) {
			t.Errorf("inert button missing %q: %s", want, inert)
		}
	}
	if strings.Contains(inert, "name=") || strings.Contains(inert, "value=") {
		t.Errorf("inert button must not submit anything: %s", inert)
	}
	submit := render(t, r, &Button{Label: "Apply", Style: "secondary", Name: "_action", Value: "apply"})
	for _, want := range []string{`type="submit"`, `name="_action"`, `value="apply"`, "border-rule-strong bg-transparent text-meta", "hover:border-sand-5 hover:bg-rule hover:text-ink"} {
		if !strings.Contains(submit, want) {
			t.Errorf("submitting button missing %q: %s", want, submit)
		}
	}
	loading := render(t, r, &Button{Label: "Fetching sources", Icon: "refresh-cw", Style: "secondary", Loading: true})
	// Waiting is the canvas's one look whatever the style — sand ground, strong
	// hairline, glyph label, no pointer — on the same h-control footprint. It
	// is the one waiting rule the shell's script puts on a pressed button
	// (verso-button-waiting), laid over the style's own dress, so a script
	// ends a wait the server drew by taking the rule off.
	for _, want := range []string{`disabled`, `aria-disabled="true"`, `aria-busy="true"`, "h-control", "verso-button-waiting", `data-verso-wait`, "Fetching sources", "size-1.5 rounded-[1px] bg-sand-5"} {
		if !strings.Contains(loading, want) {
			t.Errorf("loading button missing %q: %s", want, loading)
		}
	}
	for _, never := range []string{lucideIcons["refresh-cw"], "bg-ground", "cursor-wait", "verso-press"} {
		if strings.Contains(loading, never) {
			t.Errorf("loading button must drop its icon and its press, found %q: %s", never, loading)
		}
	}
	css, err := os.ReadFile(filepath.Join("..", "server", "assets", "input.css"))
	if err != nil {
		t.Fatal(err)
	}
	waiting := regexp.MustCompile(`button\.verso-button-waiting:disabled \{\s*pointer-events: none;\s*cursor: default;\s*opacity: 1;\s*border-color: var\(--color-rule-strong\);\s*background-color: var\(--color-quiet\);\s*color: var\(--color-glyph\);`)
	if !waiting.Match(css) {
		t.Error("the waiting rule no longer takes the pointer and lays the sand ground, strong hairline and glyph label over every style")
	}
	// act: an act on a part of a section wears the subsection act's one dress,
	// the same as a Link's act, whether it leads somewhere or submits.
	act := render(t, r, &Button{Label: "Use my computer's time", Icon: "clock", Style: "act", Name: "_action", Value: "clock"})
	for _, want := range []string{`type="submit"`, `name="_action"`, "group/act", "h-7", "border-rule-strong bg-transparent", "hover:bg-rule", "pl-2 pr-2.5", lucideIcons["clock"]} {
		if !strings.Contains(act, want) {
			t.Errorf("act button missing %q: %s", want, act)
		}
	}
	for _, never := range []string{"h-control", "bg-denim", "bg-ground"} {
		if strings.Contains(act, never) {
			t.Errorf("act button wears another dress's %q: %s", never, act)
		}
	}
	disabled := render(t, r, &Button{Label: "Apply", Disabled: true})
	for _, want := range []string{`disabled`, "cursor-not-allowed"} {
		if !strings.Contains(disabled, want) {
			t.Errorf("disabled button missing %q: %s", want, disabled)
		}
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
		`data-verso-wait`, "size-1.5 rounded-[1px] bg-sand-5",
		"<span data-verso-live-label>Pause</span>",
		"hover:border-sand-5", // hover intact
		"cursor-pointer",      // the pointer says "press me"
		"verso-press",         // and it presses
	} {
		if !strings.Contains(live, want) {
			t.Errorf("live button missing %q: %s", want, live)
		}
	}
	for _, unwanted := range []string{"disabled", "cursor-wait", "opacity-70"} {
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
	// its words 20px in on every side, as the conditions card does.
	for _, want := range []string{
		"border-rule bg-quiet", "text-sm font-semibold text-ink",
		"px-5 py-5 text-sm font-semibold text-ink", // the summary
		"border-t border-rule px-5 py-5",           // what it folds
	} {
		if !strings.Contains(condition, want) {
			t.Errorf("condition disclosure missing %q: %s", want, condition)
		}
	}
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
	for _, want := range []string{"<section", "Devices", `data-verso-section="plain"`, "x"} {
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
	for _, want := range []string{"Server public key", "HIgo9xNzJM==", "font-mono", `x-data="copy"`, "Copy", "Copied", "text-green", "whitespace-pre"} {
		if !strings.Contains(got, want) {
			t.Errorf("code missing %q in: %s", want, got)
		}
	}
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
		`<span class="min-w-0 truncate font-mono text-sm font-medium text-meta">/etc/config/network · interface</span>`,
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
	if !strings.Contains(got, `data-verso-code-head class="flex h-10 items-center gap-2 rounded-t-xs`) {
		t.Errorf("the head rounds its own corners: %s", got)
	}
	// The card stands two cells under the rule that sets it off, the rule two
	// cells under what it follows, as every section's rule and title — on a
	// page and in a drawer alike, so the air is the stylesheet's, not the
	// block's.
	sections, err := os.ReadFile(filepath.Join("..", "server", "assets", "sections.css"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, `class="flex flex-col gap-2 border-t border-rule" data-verso-code-divider`) ||
		!strings.Contains(string(sections), "[data-verso-code-divider] {\n    margin-top: calc(var(--spacing) * 10 - 1px);\n    padding-top: calc(var(--spacing) * 10 - 1px);\n  }") {
		t.Errorf("the card stands a cell under its rule: %s", got)
	}
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
	page := &Card{Title: "Zone", Children: []Widget{
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
	if strings.Contains(out, `name="input"`) || strings.Contains(out, "Zone") {
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
	for _, want := range []string{
		refusedBorder,                          // the field names itself as the refused one, focused too
		"bg-crimson-soft", "text-crimson-deep", // and the message sits on crimson's own ground
		"bg-crimson", // with the mark beside it
	} {
		if !strings.Contains(got, want) {
			t.Errorf("errored field missing %q: %s", want, got)
		}
	}
}

func TestRenderFieldFocus(t *testing.T) {
	r := newRenderer(t)
	got := render(t, r, &Field{Name: "hostname", Label: "Hostname"})
	for _, want := range []string{
		// resting, pointed at, and focused: only the border changes — the
		// strong hairline, the faint step under the pointer, the action colour
		// while typing — as the canvas draws every input
		"border-rule-strong hover:border-faint focus:border-denim",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("valid field missing focus treatment %q: %s", want, got)
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
		"border-rule-strong hover:border-faint focus:border-denim",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("list missing %q", want)
		}
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
		// 24px inside a 34px box, with 4px of the box showing all round and
		// between chips, wrapped rows 16px apart so the box stays on the grid's
		// cells — and a 20px remove glyph wearing the hover every icon act
		// wears. The box is a field, so it wears a field's fill: white, with
		// the inset shadow, and a field's height.
		`class="flex min-h-control flex-wrap items-center gap-x-1 gap-y-4 rounded-xs border border-rule-strong bg-white shadow-[inset_0_1px_2px_rgba(27,25,23,.06)] p-1`,
		`h-6 min-w-18 flex-1`, // the value being typed stands at the chips' height
		// A value in the box is a step darker than the quiet ground a chip
		// wears elsewhere, so it stands off the white box it sits in; its
		// remove glyph's hover is a step darker again, so it shows on the chip.
		`inline-flex h-6 items-center gap-1.5 rounded-xs border border-rule-strong bg-mid px-2 font-mono text-base font-medium whitespace-nowrap text-ink`,
		// Its remove glyph takes the hover every icon act takes: the parked
		// border, the hairline wash and Ink.
		`grid size-5 -mr-1 flex-none shrink-0 cursor-pointer place-items-center rounded-xs border border-transparent text-glyph transition-colors hover:border-sand-5 hover:bg-rule hover:text-ink`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("token list missing %q:\n%s", want, got)
		}
	}
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
	for _, want := range []string{
		refusedBorder,
		"text-crimson-deep", // the message reads in the step of the hue that carries words
	} {
		if !strings.Contains(got, want) {
			t.Errorf("errored list item missing focus treatment %q: %s", want, got)
		}
	}
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
	if !strings.Contains(got, "border border-denim bg-denim text-white") {
		t.Errorf("form submit missing the primary-button treatment: %s", got)
	}
	if !strings.Contains(got, "hover:border-denim-deep hover:bg-denim-deep") ||
		!strings.Contains(got, "verso-press") {
		t.Errorf("form submit missing tactile pressed state: %s", got)
	}
}

func TestRenderRawMarkdown(t *testing.T) {
	r := newRenderer(t)

	got := render(t, r, &Raw{Markdown: "**bold** and `code`"})
	for _, want := range []string{"<strong>bold</strong>", "<code>code</code>", "Raw"} {
		if !strings.Contains(got, want) {
			t.Errorf("raw output missing %q: %s", want, got)
		}
	}
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
