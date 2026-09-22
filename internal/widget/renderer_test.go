// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
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

func TestRenderStackDivided(t *testing.T) {
	r := newRenderer(t)
	plain := render(t, r, &Stack{Children: []Widget{&Badge{Text: "a"}, &Badge{Text: "b"}}})
	if !strings.Contains(plain, "space-y-4") {
		t.Errorf("default stack should space its children:\n%s", plain)
	}
	div := render(t, r, &Stack{Divided: true, Children: []Widget{&Badge{Text: "a"}, &Badge{Text: "b"}}})
	if !strings.Contains(div, "divide-y") {
		t.Errorf("divided stack should draw hairlines:\n%s", div)
	}
	compact := render(t, r, &Stack{Compact: true, Children: []Widget{&Badge{Text: "a"}, &Badge{Text: "b"}}})
	if !strings.Contains(compact, "space-y-3") || strings.Contains(compact, "space-y-4") {
		t.Errorf("compact stack should use tighter spacing: %s", compact)
	}
	inline := render(t, r, &Stack{Inline: true, Children: []Widget{&Badge{Text: "a"}, &Text{Markdown: "or"}, &Badge{Text: "b"}}})
	if !strings.Contains(inline, "flex flex-wrap items-center gap-3") || !strings.Contains(inline, ">or</p>") {
		t.Errorf("inline stack should keep its children in one wrapping row: %s", inline)
	}
	if strings.Contains(div, "space-y-4") {
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
	want := `<section><div class="mb-5"><h3 class="text-lg font-semibold tracking-tight text-ink">empty</h3></div><div class="space-y-5"></div></section>`
	if got != want {
		t.Errorf("Render mismatch:\n got: %s\nwant: %s", got, want)
	}
}

func TestRenderCardSubtitle(t *testing.T) {
	r := newRenderer(t)
	got := render(t, r, &Card{Title: "Your gateway", Subtitle: "the back of the box"})
	if !strings.Contains(got, `<h3 class="text-lg font-semibold tracking-tight text-ink">Your gateway</h3>`) {
		t.Errorf("card title missing:\n%s", got)
	}
	if !strings.Contains(got, `<p class="mt-1 text-sm leading-snug text-body">the back of the box</p>`) {
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
		Name: "tz", Label: "Timezone", Kind: "select", Value: "CET",
		Options: []Option{{Value: "UTC", Label: "UTC"}, {Value: "CET", Label: "Central"}},
	})
	if !strings.Contains(got, "<select") {
		t.Errorf("no <select>: %s", got)
	}
	if !strings.Contains(got, `value="CET" selected`) {
		t.Errorf("selected option not marked: %s", got)
	}
	if strings.Contains(got, `value="UTC" selected`) {
		t.Errorf("wrong option marked selected: %s", got)
	}
}

func TestRenderBadge(t *testing.T) {
	r := newRenderer(t)
	got := render(t, r, &Badge{Variant: "success", Text: "Connected", Dot: true})
	for _, want := range []string{
		"Connected", "rounded-full",
		"border-green-line bg-green-soft text-green-deep", // light treatment is unchanged
		"rounded-xs border",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("success badge missing %q in: %s", want, got)
		}
	}
	// An unknown/neutral variant falls back to sand, never leaks the variant
	// name. A pill with no hue is still a verdict — the firewall's drop — so it
	// stands a step above the reference chip's quiet ground and keeps its word
	// in full ink.
	neutral := render(t, r, &Badge{Variant: "neutral", Text: "Offline"})
	if !strings.Contains(neutral, "border-rule-strong bg-mid text-ink") {
		t.Errorf("neutral badge should use the sand verdict step: %s", neutral)
	}
	if strings.Contains(neutral, "size-1.5") {
		t.Errorf("badge without Dot should not render a dot: %s", neutral)
	}
}

func TestRenderHeroSwitch(t *testing.T) {
	r := newRenderer(t)
	got := render(t, r, &Switch{
		Style: "hero", Icon: "shield", Name: "vpn_on", On: true,
		Label: "Your home VPN is on", OffLabel: "Your home VPN is off",
		Meta: "2 of 3 devices connected",
	})

	for _, want := range []string{
		"verso-toggle", // the pure-CSS state scope
		`type="checkbox"`, `name="vpn_on"`,
		"Your home VPN is on",      // on headline
		"Your home VPN is off",     // off headline (CSS hides it while checked)
		"2 of 3 devices connected", // meta
		// The switch reflects state without JS, and it is on in the same colour
		// every switch in the app is on in — the action colour, not a second
		// green that would read as a verdict.
		"peer-checked:bg-body",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("hero switch missing %q in: %s", want, got)
		}
	}
	// The checked attribute is present on the input when On is true.
	if !strings.Contains(got, `name="vpn_on" checked`) {
		t.Errorf("checked hero switch missing the checked attribute: %s", got)
	}
	// The switch is pure CSS: the markup carries no script and no Alpine directive.
	if strings.Contains(got, "<script") || strings.Contains(got, "x-data") {
		t.Errorf("hero switch must be pure CSS, found script/alpine: %s", got)
	}
	// Unchecked renders without the checked attribute (peer-checked utility aside).
	off := render(t, r, &Switch{Style: "hero", Name: "n", Label: "On", OffLabel: "Off"})
	if strings.Contains(off, `name="n" checked`) {
		t.Errorf("unchecked hero switch must not carry the checked attribute: %s", off)
	}
}

func TestRenderTabs(t *testing.T) {
	r := newRenderer(t)
	got := render(t, r, &Tabs{Tabs: []Tab{
		{Label: "My devices", Icon: "device", Children: []Widget{&Badge{Variant: "success", Text: "here"}}},
		{Label: "Route through a provider", Icon: "globe", Children: []Widget{&Field{Name: "cfg", Label: "Config"}}},
	}})

	for _, want := range []string{
		"verso-tabs",
		`type="radio"`,
		"My devices", "Route through a provider", // both labels
		"here",       // first tab's child rendered
		`name="cfg"`, // second tab's child rendered
		"verso-tab-panel",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("tabs missing %q in: %s", want, got)
		}
	}
	// Exactly one radio starts checked (the first tab).
	if n := strings.Count(got, "checked"); n != 1 {
		t.Errorf("want exactly one checked radio, got %d: %s", n, got)
	}
	// Pure CSS: no script, no Alpine.
	if strings.Contains(got, "<script") || strings.Contains(got, "x-data") {
		t.Errorf("tabs must be pure CSS, found script/alpine: %s", got)
	}
	// Two tab groups on one page get distinct radio names, so they never collide.
	two := render(t, r, &Tabs{Tabs: []Tab{{Label: "A"}, {Label: "B"}}})
	if strings.Contains(got, `name="verso-tabs-1"`) && strings.Contains(two, `name="verso-tabs-1"`) {
		t.Errorf("two tab groups shared a radio group name: %s", two)
	}
}

// TestRenderSegmentedFieldsWearTheOneTray: a segmented pick — one of three
// verdicts, or the days of a schedule — wears the tray every switch in the app
// wears: the segment in force filled in the body ink, the rest plain words,
// never the action colour. Radios and checkboxes underneath, so it posts as
// the box it replaces.
func TestRenderSegmentedFieldsWearTheOneTray(t *testing.T) {
	r := newRenderer(t)
	pick := render(t, r, &Field{Name: "input", Label: "Traffic to this router", Kind: "select", Style: "segmented", Value: "reject",
		Options: []Option{{Value: "accept", Label: "accept"}, {Value: "reject", Label: "reject"}, {Value: "drop", Label: "drop"}}})
	days := render(t, r, &Field{Name: "days", Label: "Days", Kind: "checks", Style: "segmented", Values: []string{"mon"},
		Options: []Option{{Value: "mon", Label: "Mon"}, {Value: "tue", Label: "Tue"}}})
	for _, got := range []string{pick, days} {
		for _, want := range []string{
			`class="inline-flex w-fit gap-0.5 self-start rounded-xs border border-rule-strong bg-quiet p-0.5">`,
			`<label class="flex h-7.5 cursor-pointer items-center rounded-xs px-3.5 text-sm font-normal text-body transition-colors hover:text-ink has-checked:bg-body has-checked:font-semibold has-checked:text-white has-focus-visible:outline-2`,
		} {
			if !strings.Contains(got, want) {
				t.Errorf("segmented field missing %q:\n%s", want, got)
			}
		}
		if strings.Contains(got, "bg-denim") {
			t.Errorf("a selected segment is never the action colour:\n%s", got)
		}
	}
	if !strings.Contains(pick, `type="radio" name="input" value="reject" checked`) {
		t.Errorf("the verdict in force is the checked radio:\n%s", pick)
	}
	if !strings.Contains(days, `type="checkbox" name="days" value="mon" checked`) {
		t.Errorf("a day in the set is a checked box:\n%s", days)
	}
}

func TestRenderChoice(t *testing.T) {
	r := newRenderer(t)
	got := render(t, r, &Choice{Name: "reach", Label: "What can it reach?", Options: []ChoiceOption{
		{Value: "home", Label: "My whole home network", Desc: "Everything on your LAN.", Checked: true},
		{Value: "device", Label: "Just this router", Desc: "Nothing else."},
	}})

	for _, want := range []string{
		"verso-choice", "verso-choice-card", "verso-choice-tick",
		`type="radio"`, `name="reach"`, `value="home"`, `value="device"`,
		"My whole home network", "Everything on your LAN.", "What can it reach?",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("choice missing %q in: %s", want, got)
		}
	}
	if !strings.Contains(got, `value="home" checked`) {
		t.Errorf("checked option not marked: %s", got)
	}
	if strings.Contains(got, "<script") || strings.Contains(got, "x-data") {
		t.Errorf("choice must be pure CSS: %s", got)
	}
}

func TestRenderQr(t *testing.T) {
	r := newRenderer(t)
	got := render(t, r, &Qr{Data: "wg://join?token=abc", Caption: "Scan with the app"})

	for _, want := range []string{"<svg", "viewBox", "<path", "Scan with the app"} {
		if !strings.Contains(got, want) {
			t.Errorf("qr missing %q in: %s", want, got)
		}
	}
	// It is a real, self-contained code: inline SVG, no external fetch (no <img>,
	// no src/href pulling a remote resource). The SVG xmlns is a namespace, not a fetch.
	if strings.Contains(got, "<img") || strings.Contains(got, "src=") || strings.Contains(got, "href=") {
		t.Errorf("qr must be inline SVG with no external fetch: %s", got)
	}
	// Different payloads produce different codes (proves it encodes the data).
	other := render(t, r, &Qr{Data: "wg://join?token=xyz"})
	if got == other {
		t.Errorf("qr did not vary with its payload")
	}
	// With a download, a quiet link rides beneath the code (a data: URL survives the
	// link URL policy).
	dl := render(t, r, &Qr{Data: "x", DownloadHref: "data:text/plain,abc", DownloadName: "home.conf", DownloadLabel: "Download config file"})
	for _, want := range []string{`href="data:text/plain,abc"`, `download="home.conf"`, "Download config file"} {
		if !strings.Contains(dl, want) {
			t.Errorf("qr download missing %q in: %s", want, dl)
		}
	}
}

func TestRenderWizard(t *testing.T) {
	r := newRenderer(t)
	got := render(t, r, &Wizard{Steps: []WizardStep{
		{Children: []Widget{&Field{Name: "device_name", Label: "Name"}}},
		{Children: []Widget{&Qr{Data: "x"}}},
	}})

	for _, want := range []string{
		"verso-wizard", "verso-wizard-step", "verso-wizard-dot",
		`name="device_name"`, // step 1 child
		"<svg",               // step 2 child (qr)
		"Continue", "Back", "Done",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("wizard missing %q in: %s", want, got)
		}
	}
	// Exactly the first step's radio starts checked.
	if n := strings.Count(got, "checked"); n != 1 {
		t.Errorf("want exactly one checked step radio, got %d: %s", n, got)
	}
	// The Continue label on step 1 targets step 2's radio (id ...-1).
	if !strings.Contains(got, `-1" class="inline-flex cursor-pointer items-center justify-center rounded-md px-4 py-2 text-sm font-medium border border-denim bg-denim`) {
		t.Errorf("Continue label does not target the next step's radio: %s", got)
	}
	if strings.Contains(got, "<script") || strings.Contains(got, "x-data") {
		t.Errorf("wizard must be pure CSS: %s", got)
	}
}

func TestRenderDrawer(t *testing.T) {
	r := newRenderer(t)
	got := render(t, r, &Drawer{
		Title:    "My Phone",
		Trigger:  []Widget{&Row{Icon: "phone", Title: "My Phone", Meta: "My whole home network"}},
		Children: []Widget{&Qr{Data: "x"}, &Text{Markdown: "**Added** · 2 weeks ago"}},
	})

	for _, want := range []string{
		`x-data="modal"`,    // reuses the shell-owned modal component
		`@click="show"`,     // the trigger opens it
		`x-teleport="body"`, // panel escapes the content flow
		`role="dialog"`,
		"translate-x-full", // slides in from the right
		"bg-ink/18",        // the canvas scrim: ink at 18% over a 2px blur
		"My Phone",         // trigger + title
		"<svg",             // qr child rendered in the body
		"Added",            // text child rendered in the body
	} {
		if !strings.Contains(got, want) {
			t.Errorf("drawer missing %q in: %s", want, got)
		}
	}
	// Plugins ship no JS: the markup carries only directives, never a script.
	if strings.Contains(got, "<script") {
		t.Errorf("drawer must not emit a script tag: %s", got)
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

// TestRenderBadgeLiveDot proves a connected (success) dot gets the live pulse hook,
// while other dots stay calm.
func TestRenderBadgeLiveDot(t *testing.T) {
	r := newRenderer(t)
	live := render(t, r, &Badge{Variant: "success", Text: "Connected", Dot: true})
	if !strings.Contains(live, "verso-live-dot") {
		t.Errorf("connected dot missing the live pulse hook: %s", live)
	}
	calm := render(t, r, &Badge{Variant: "neutral", Text: "Offline", Dot: true})
	if strings.Contains(calm, "verso-live-dot") {
		t.Errorf("offline dot must not pulse: %s", calm)
	}
}

func TestRenderDivider(t *testing.T) {
	r := newRenderer(t)
	got := render(t, r, &Divider{Label: "First-run / empty state"})
	for _, want := range []string{"First-run / empty state", "bg-rule", "my-20"} {
		if !strings.Contains(got, want) {
			t.Errorf("divider missing %q in: %s", want, got)
		}
	}
	// No label → a bare rule, no label text.
	plain := render(t, r, &Divider{})
	if strings.Contains(plain, "<span class=\"relative") {
		t.Errorf("labelless divider should not render a label span: %s", plain)
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

	divided := render(t, r, &Properties{Items: items}) // default
	if !strings.Contains(divided, "divide-y divide-mid") {
		t.Errorf("default properties should use hairlines:\n%s", divided)
	}
	// Striping is retired: a "striped" request falls through to the hairline
	// default and never zebra-shades.
	striped := render(t, r, &Properties{Style: "striped", Items: items})
	if strings.Contains(striped, "odd:bg-quiet") {
		t.Errorf("striped is retired; must not zebra-shade:\n%s", striped)
	}
	if !strings.Contains(striped, "divide-y divide-mid") {
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
		`<dd class="flex min-w-0 items-center justify-end text-right text-base font-medium text-ink">`,
		`class="min-w-0 wrap-anywhere font-mono text-base font-medium">aarch64_generic</span>`,
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

func TestRenderConfirm(t *testing.T) {
	r := newRenderer(t)
	got := render(t, r, &Confirm{Trigger: "Remove device", Message: "Remove this device?", Confirm: "Remove", Cancel: "Keep it", RequirePassword: true})
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
		// value stays a mark, on the icon.
		"border-crimson-line bg-crimson-soft", "text-crimson-deep", "text-crimson", "<svg",
		"mt-6 ml-6 flex items-center justify-start", "hover:bg-crimson-line/50", // actions align with the message; cancel keeps the explanation's tone and hovers by a soft wash, not a heavy colour darken
		`type="password"`, `autocomplete="current-password"`, "w-1/3", // sensitive actions can require re-authentication
		"border-crimson bg-crimson text-white",
		"active:translate-y-px active:shadow-none motion-reduce:active:translate-y-0",
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
	if strings.Contains(got, `@click="cancel" class="flex h-9 shrink-0 cursor-pointer items-center rounded-xs border`) {
		t.Errorf("confirm cancel must not render as a bordered button: %s", got)
	}
	// Labels default when unset.
	def := render(t, r, &Confirm{Trigger: "Delete", Message: "Sure?"})
	if !strings.Contains(def, ">Confirm<") || !strings.Contains(def, ">Cancel<") {
		t.Errorf("confirm should default its labels: %s", def)
	}
	// Two confirms on a page get distinct panel ids, the trigger's controls.
	if !strings.Contains(got, `aria-controls="verso-confirm-1"`) || !strings.Contains(def, `id="verso-confirm-2"`) {
		t.Errorf("each confirm's trigger controls its own panel:\n%s\n%s", got, def)
	}
	// A Title renders as a bold heading above the message; without one the
	// message stands alone (no stray heading element).
	titled := render(t, r, &Confirm{Trigger: "Download and install", Title: "Install now?", Message: "It will be unavailable for several minutes."})
	if !strings.Contains(titled, `<p class="text-base font-semibold">Install now?</p>`) {
		t.Errorf("a titled confirm should render its heading bold: %s", titled)
	}
	if !strings.Contains(titled, "It will be unavailable for several minutes.") {
		t.Errorf("a titled confirm should still render its message: %s", titled)
	}
	// The trigger and the confirm button are semibold like every other control;
	// what an untitled confirm must not draw is the heading paragraph.
	if strings.Contains(def, `<p class="text-base font-semibold">`) {
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
	// centre, and no frame.
	compact := render(t, r, &Callout{Compact: true, Body: "A short note."})
	for _, want := range []string{"items-start", "gap-2", "px-3", "py-2", "mt-[0.4375rem] size-1.5", "bg-denim", "A short note."} {
		if !strings.Contains(compact, want) {
			t.Errorf("compact callout missing %q in: %s", want, compact)
		}
	}
	if strings.Contains(compact, "px-5 py-4") || strings.Contains(compact, "border ") {
		t.Errorf("compact callout should carry neither the standing padding nor a frame: %s", compact)
	}
}

func TestRenderLink(t *testing.T) {
	r := newRenderer(t)
	dl := render(t, r, &Link{Label: "Download config", Href: "data:text/plain,abc", Download: "phone.conf", Style: "ghost"})
	for _, want := range []string{`href="data:text/plain,abc"`, `download="phone.conf"`, "Download config"} {
		if !strings.Contains(dl, want) {
			t.Errorf("link missing %q in: %s", want, dl)
		}
	}
	if !strings.Contains(dl, "border border-rule-strong bg-transparent text-ink") || !strings.Contains(dl, "hover:border-faint hover:bg-quiet") {
		t.Errorf("ghost link missing the secondary-button treatment: %s", dl)
	}
	if !strings.Contains(dl, "active:translate-y-px active:shadow-none motion-reduce:active:translate-y-0") {
		t.Errorf("button-styled link missing tactile pressed state: %s", dl)
	}
	primary := render(t, r, &Link{Label: "Download backup", Href: "/backup", Style: "button"})
	if !strings.Contains(primary, "active:translate-y-px active:shadow-none motion-reduce:active:translate-y-0") {
		t.Errorf("primary button link missing tactile pressed state: %s", primary)
	}
	secondary := render(t, r, &Link{Label: "Restart router", Href: "/restart", Style: "secondary"})
	for _, want := range []string{
		"hover:border-faint hover:bg-quiet",
		"border-rule-strong bg-transparent text-ink",
		"active:translate-y-px",
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
		"flex items-center gap-1.5 border-l border-rule py-1.5 pl-4 text-sm",
	} {
		if !strings.Contains(rail, want) {
			t.Errorf("rail link missing %q: %s", want, rail)
		}
	}
	if strings.Contains(rail, "underline") || strings.Contains(rail, "text-denim") {
		t.Errorf("a rail link is not an ordinary link: %s", rail)
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
}

func TestRenderButton(t *testing.T) {
	r := newRenderer(t)
	inert := render(t, r, &Button{Label: "Choose firmware…", Icon: "upload"})
	for _, want := range []string{`type="button"`, "border-denim bg-denim text-white", "hover:border-denim-deep hover:bg-denim-deep", "active:translate-y-px active:shadow-none motion-reduce:active:translate-y-0", "size-4", "Choose firmware…"} {
		if !strings.Contains(inert, want) {
			t.Errorf("inert button missing %q: %s", want, inert)
		}
	}
	if strings.Contains(inert, "name=") || strings.Contains(inert, "value=") {
		t.Errorf("inert button must not submit anything: %s", inert)
	}
	submit := render(t, r, &Button{Label: "Apply", Style: "secondary", Name: "_action", Value: "apply"})
	for _, want := range []string{`type="submit"`, `name="_action"`, `value="apply"`, "border-rule-strong bg-ground text-ink"} {
		if !strings.Contains(submit, want) {
			t.Errorf("submitting button missing %q: %s", want, submit)
		}
	}
	loading := render(t, r, &Button{Label: "Fetching sources", Icon: "refresh-cw", Style: "secondary", Loading: true})
	for _, want := range []string{`disabled`, `aria-busy="true"`, "cursor-wait opacity-70", `data-verso-wait`, "Fetching sources", "size-1.5 rounded-[1px] bg-sand-5"} {
		if !strings.Contains(loading, want) {
			t.Errorf("loading button missing %q: %s", want, loading)
		}
	}
	if strings.Contains(loading, lucideIcons["refresh-cw"]) || strings.Contains(loading, "hover:border-denim") {
		t.Errorf("loading button must replace its action icon and have no hover response: %s", loading)
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
		"hover:border-faint",    // hover intact
		"cursor-pointer",        // the pointer says "press me"
		"active:translate-y-px", // and it presses
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
	// band, because what it folds is part of the form around it.
	for _, want := range []string{"border-rule bg-quiet", "text-sm font-semibold text-ink"} {
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
	for _, want := range []string{"<section", "Devices", "pt-6", "x"} {
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
		`x-data="modal"`,    // the shell-owned Alpine component
		`@click="show"`,     // trigger opens it
		`x-teleport="body"`, // dialog escapes the content flow
		`role="dialog"`,
		"Add a device",       // trigger + title
		`name="device_name"`, // the child widget is rendered inside
		"bg-ink/18",          // the canvas scrim, shared with every other overlay
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

// TestRenderModalAddTrigger: the "add" trigger style renders a full-width dashed
// button (a create affordance), while the default stays a solid button.
func TestRenderModalAddTrigger(t *testing.T) {
	r := newRenderer(t)
	add := render(t, r, &Modal{Trigger: "Add a device", TriggerStyle: "add", Title: "Add a device"})
	for _, want := range []string{"border-dashed", "w-full", "Add a device"} {
		if !strings.Contains(add, want) {
			t.Errorf("add-style modal trigger missing %q in: %s", want, add)
		}
	}
	solid := render(t, r, &Modal{Trigger: "Open", Title: "T"})
	if strings.Contains(solid, "border-dashed") {
		t.Errorf("default modal trigger should be solid, not dashed: %s", solid)
	}
}

func TestRenderProgress(t *testing.T) {
	r := newRenderer(t)
	got := render(t, r, &Progress{Title: "Verifying firmware", Body: "Checking the image signature."})
	for _, want := range []string{
		`role="status"`, `aria-live="polite"`, `data-verso-wait`,
		"Verifying firmware", "Checking the image signature.",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("progress state missing %q: %s", want, got)
		}
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

func TestRenderFileField(t *testing.T) {
	r := newRenderer(t)
	got := render(t, r, &Field{
		Name: "firmware_image", Kind: "file", Accept: ".bin,.img",
		Prompt: "Drop a firmware image here, or",
	})
	for _, want := range []string{
		`type="file"`, `accept=".bin,.img"`, "border-dotted", "rounded-xs", "px-8", "py-12",
		"size-10", "Drop a firmware image here", "mt-1 block", "whitespace-nowrap", "choose one from your computer",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("file field missing %q: %s", want, got)
		}
	}
	if !strings.Contains(got, `for="firmware_image"`) || !strings.Contains(got, `id="firmware_image"`) {
		t.Errorf("file picker phrase must label the native input: %s", got)
	}
}

func TestRenderFieldError(t *testing.T) {
	r := newRenderer(t)

	got := render(t, r, &Field{Name: "h", Label: "H", Value: "bad host", Error: "must be a valid hostname"})
	if !strings.Contains(got, "must be a valid hostname") {
		t.Errorf("inline error not shown: %s", got)
	}
	for _, want := range []string{
		"border-crimson hover:border-crimson-deep focus:border-crimson-deep", // the field names itself as the refused one
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
		"border-rule-strong hover:border-denim focus:border-denim-deep", // resting, pointed at, and focused
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
		"border-rule-strong hover:border-denim focus:border-denim-deep",
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
		// 30px inside a 36px box, with 2px of the box showing all round — and a
		// 20px remove glyph wearing the hover every icon act wears.
		`class="flex min-h-9 flex-wrap items-center gap-1 rounded-xs border border-rule-strong bg-ground p-0.5`,
		`inline-flex h-7.5 items-center gap-1.5 rounded-xs border border-rule bg-quiet px-2 font-mono text-base font-medium whitespace-nowrap text-ink`,
		`grid size-5 -mr-1 flex-none cursor-pointer place-items-center rounded-xs text-glyph transition-colors hover:bg-mid/50 hover:text-ink`,
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
		"border-crimson hover:border-crimson-deep focus:border-crimson-deep",
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
	if !strings.Contains(got, ">Save<") {
		t.Errorf("default submit label 'Save' missing: %s", got)
	}
	if !strings.Contains(got, `name="h"`) {
		t.Errorf("nested field not rendered inside form: %s", got)
	}
}

func TestRenderFormSuccess(t *testing.T) {
	r := newRenderer(t)

	got := render(t, r, &Form{Submit: "Apply", Success: "Saved."})
	if !strings.Contains(got, "Saved.") {
		t.Errorf("success message missing: %s", got)
	}
	if !strings.Contains(got, ">Apply<") {
		t.Errorf("custom submit label missing: %s", got)
	}
	if !strings.Contains(got, "border border-denim bg-denim text-white") {
		t.Errorf("form submit missing the primary-button treatment: %s", got)
	}
	if !strings.Contains(got, "hover:border-denim-deep hover:bg-denim-deep") ||
		!strings.Contains(got, "active:translate-y-px active:shadow-none motion-reduce:active:translate-y-0") {
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
