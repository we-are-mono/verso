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
	if err := r.Render(&b, w); err != nil {
		t.Fatalf("Render: %v", err)
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
	if !strings.Contains(got, "red-600") {
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
	if strings.Contains(div, "space-y-4") {
		t.Errorf("divided stack should not also space:\n%s", div)
	}
}

func TestRenderCardChrome(t *testing.T) {
	r := newRenderer(t)

	got := render(t, r, &Card{Title: "empty"})
	want := `<section class="rounded-md border border-slate-300 bg-white p-8 shadow-md"><div class="mb-4"><h3 class="text-base font-medium text-slate-900">empty</h3></div><div class="space-y-4"></div></section>`
	if got != want {
		t.Errorf("Render mismatch:\n got: %s\nwant: %s", got, want)
	}
}

func TestRenderCardSubtitle(t *testing.T) {
	r := newRenderer(t)
	got := render(t, r, &Card{Title: "Your gateway", Subtitle: "the back of the box"})
	if !strings.Contains(got, `<h3 class="text-base font-medium text-slate-900">Your gateway</h3>`) {
		t.Errorf("card title missing:\n%s", got)
	}
	if !strings.Contains(got, `<p class="mt-1 text-sm text-slate-500">the back of the box</p>`) {
		t.Errorf("card subtitle missing or not styled as a subtitle:\n%s", got)
	}
}

func TestRenderCardWithoutTitleOmitsHeader(t *testing.T) {
	r := newRenderer(t)

	got := render(t, r, &Card{})
	want := `<section class="rounded-md border border-slate-300 bg-white p-8 shadow-md"><div class="space-y-4"></div></section>`
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
	body := strings.Index(got, `<div class="space-y-4">`)
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
	for _, want := range []string{"Connected", "green", "rounded-full"} {
		if !strings.Contains(got, want) {
			t.Errorf("success badge missing %q in: %s", want, got)
		}
	}
	// An unknown/neutral variant falls back to slate, never leaks the variant name.
	neutral := render(t, r, &Badge{Variant: "neutral", Text: "Offline"})
	if !strings.Contains(neutral, "slate") {
		t.Errorf("neutral badge should use slate: %s", neutral)
	}
	if strings.Contains(neutral, "size-1.5") {
		t.Errorf("badge without Dot should not render a dot: %s", neutral)
	}
}

func TestRenderToggle(t *testing.T) {
	r := newRenderer(t)
	got := render(t, r, &Toggle{
		Icon: "shield", Name: "vpn_on", Checked: true,
		Label: "Your home VPN is on", OffLabel: "Your home VPN is off",
		Meta: "2 of 3 devices connected",
	})

	for _, want := range []string{
		"verso-toggle",                 // the pure-CSS state scope
		`type="checkbox"`, `name="vpn_on"`,
		"Your home VPN is on",          // on headline
		"Your home VPN is off",         // off headline (CSS hides it while checked)
		"2 of 3 devices connected",     // meta
		"peer-checked:bg-emerald-500",  // switch reflects state without JS
	} {
		if !strings.Contains(got, want) {
			t.Errorf("toggle missing %q in: %s", want, got)
		}
	}
	// The checked attribute is present on the input when Checked is true.
	if !strings.Contains(got, `name="vpn_on" checked`) {
		t.Errorf("checked toggle missing the checked attribute: %s", got)
	}
	// The switch is pure CSS: the markup carries no script and no Alpine directive.
	if strings.Contains(got, "<script") || strings.Contains(got, "x-data") {
		t.Errorf("toggle must be pure CSS, found script/alpine: %s", got)
	}
	// Unchecked renders without the checked attribute (peer-checked utility aside).
	off := render(t, r, &Toggle{Name: "n", Label: "On", OffLabel: "Off"})
	if strings.Contains(off, `name="n" checked`) {
		t.Errorf("unchecked toggle must not carry the checked attribute: %s", off)
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
	if !strings.Contains(got, `-1" class="inline-flex cursor-pointer items-center justify-center rounded-md bg-sky-600`) {
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
		"translate-x-full",   // slides in from the right
		"My Phone",           // trigger + title
		"<svg",               // qr child rendered in the body
		"Added",              // text child rendered in the body
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
		`x-data="modal"`,   // the CTA is a real composed widget
	} {
		if !strings.Contains(got, want) {
			t.Errorf("empty missing %q in: %s", want, got)
		}
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
	for _, want := range []string{"First-run / empty state", "bg-slate-200", "my-20"} {
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

// TestRenderPropertiesStyles: the row style resolves to hairlines (default), zebra
// shading, or no separators.
func TestRenderPropertiesStyles(t *testing.T) {
	r := newRenderer(t)
	items := []Property{{Label: "A", Value: "1"}, {Label: "B", Value: "2"}}

	divided := render(t, r, &Properties{Items: items}) // default
	if !strings.Contains(divided, "divide-y divide-slate-200") {
		t.Errorf("default properties should use hairlines:\n%s", divided)
	}
	striped := render(t, r, &Properties{Style: "striped", Items: items})
	if !strings.Contains(striped, "odd:bg-slate-50") {
		t.Errorf("striped properties should zebra-shade rows:\n%s", striped)
	}
	if strings.Contains(striped, "divide-y") {
		t.Errorf("striped properties should not also draw hairlines:\n%s", striped)
	}
	bare := render(t, r, &Properties{Style: "plain", Items: items})
	if !strings.Contains(bare, "space-y-3") || strings.Contains(bare, "divide-y") || strings.Contains(bare, "odd:bg-slate-50") {
		t.Errorf("plain properties should have no separators:\n%s", bare)
	}
}

func TestRenderConfirm(t *testing.T) {
	r := newRenderer(t)
	got := render(t, r, &Confirm{Trigger: "Remove device", Message: "Remove this device?", Confirm: "Remove", Cancel: "Keep it"})
	for _, want := range []string{
		"verso-confirm", "verso-confirm-toggle", "verso-confirm-panel", "verso-confirm-trigger",
		"Remove device", "Remove this device?", "Remove", "Keep it", `type="checkbox"`,
		"text-red-700", "<svg", // the prompt is red with a warning icon
	} {
		if !strings.Contains(got, want) {
			t.Errorf("confirm missing %q in: %s", want, got)
		}
	}
	// Pure CSS: no script, no Alpine.
	if strings.Contains(got, "<script") || strings.Contains(got, "x-data") {
		t.Errorf("confirm must be pure CSS: %s", got)
	}
	// Labels default when unset.
	def := render(t, r, &Confirm{Trigger: "Delete", Message: "Sure?"})
	if !strings.Contains(def, ">Confirm<") || !strings.Contains(def, ">Cancel<") {
		t.Errorf("confirm should default its labels: %s", def)
	}
	// Two confirms on a page get distinct checkbox ids.
	if strings.Count(got+def, `id="verso-confirm-`) == 2 && strings.Contains(def, `id="verso-confirm-1"`) && strings.Contains(got, `id="verso-confirm-1"`) {
		t.Errorf("two confirms shared an id")
	}
}

func TestRenderCallout(t *testing.T) {
	r := newRenderer(t)
	ok := render(t, r, &Callout{Variant: "success", Title: "Reachable", Body: "Verified from the internet."})
	for _, want := range []string{"green", "Reachable", "Verified from the internet.", "<svg"} {
		if !strings.Contains(ok, want) {
			t.Errorf("success callout missing %q in: %s", want, ok)
		}
	}
	// Unknown/default variant falls back to info (sky), never leaks the variant name.
	def := render(t, r, &Callout{Body: "heads up"})
	if !strings.Contains(def, "sky") {
		t.Errorf("default callout should use the info palette: %s", def)
	}
	warn := render(t, r, &Callout{Variant: "warning", Body: "x"})
	if !strings.Contains(warn, "amber") {
		t.Errorf("warning callout should use amber: %s", warn)
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
	// A plain link carries no download attribute.
	plain := render(t, r, &Link{Label: "Docs", Href: "/help"})
	if strings.Contains(plain, "download=") {
		t.Errorf("non-download link must not carry a download attribute: %s", plain)
	}
}

func TestRenderCopy(t *testing.T) {
	r := newRenderer(t)
	got := render(t, r, &Copy{Label: "Copy key", Text: "SECRETKEY=="})
	for _, want := range []string{`x-data="copy"`, `@click="run"`, "Copy key", "Copied!", "SECRETKEY=="} {
		if !strings.Contains(got, want) {
			t.Errorf("copy missing %q in: %s", want, got)
		}
	}
	if strings.Contains(got, "<script") {
		t.Errorf("copy must not emit a script tag: %s", got)
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
	for _, want := range []string{"Server public key", "HIgo9xNzJM==", "<code", "font-mono", `x-data="copy"`, "Copy", "Copied!", "text-green-600"} {
		if !strings.Contains(got, want) {
			t.Errorf("code missing %q in: %s", want, got)
		}
	}
	// Without copy, no copy button.
	nocopy := render(t, r, &Code{Value: "abc"})
	if strings.Contains(nocopy, "x-data") {
		t.Errorf("code without copy should have no copy button: %s", nocopy)
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
		`x-data="modal"`,   // the shell-owned Alpine component
		`@click="show"`,    // trigger opens it
		`x-teleport="body"`, // dialog escapes the content flow
		`role="dialog"`,
		"Add a device",       // trigger + title
		`name="device_name"`, // the child widget is rendered inside
	} {
		if !strings.Contains(got, want) {
			t.Errorf("modal missing %q in: %s", want, got)
		}
	}
	// The plugin ships no JS: the markup carries only directives, never a script.
	if strings.Contains(got, "<script") {
		t.Errorf("modal must not emit a script tag: %s", got)
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
	if !strings.Contains(got, "border-red-600") {
		t.Errorf("errored field should carry the danger token: %s", got)
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
	if n := strings.Count(got, `name="server"`); n != 3 {
		t.Errorf("want 3 inputs (2 items + 1 blank), got %d: %s", n, got)
	}
	for _, want := range []string{"NTP servers", "0.pool.ntp.org", "1.pool.ntp.org", "Add"} {
		if !strings.Contains(got, want) {
			t.Errorf("list missing %q", want)
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

// TestRawUsageInstrumented proves the demand-signal counter (ADR-005 §5).
func TestRawUsageInstrumented(t *testing.T) {
	r := newRenderer(t)
	if r.RawUsage() != 0 {
		t.Fatalf("initial RawUsage = %d, want 0", r.RawUsage())
	}
	_ = render(t, r, &Raw{Markdown: "a"})
	_ = render(t, r, &Raw{Markdown: "b"})
	if r.RawUsage() != 2 {
		t.Errorf("RawUsage = %d, want 2", r.RawUsage())
	}
}

// TestRenderWithTokenInjectsCSRF: a form rendered with a token carries a hidden
// _csrf field; plain Render omits it (VS-04).
func TestRenderWithTokenInjectsCSRF(t *testing.T) {
	r := newRenderer(t)
	form := &Form{Fields: []Widget{&Field{Name: "h", Label: "H"}}}

	var withTok strings.Builder
	if err := r.RenderWithToken(&withTok, form, "tok123"); err != nil {
		t.Fatalf("RenderWithToken: %v", err)
	}
	if !strings.Contains(withTok.String(), `name="_csrf" value="tok123"`) {
		t.Errorf("csrf hidden field missing: %s", withTok.String())
	}

	var plain strings.Builder
	if err := r.Render(&plain, form); err != nil {
		t.Fatalf("Render: %v", err)
	}
	if strings.Contains(plain.String(), "_csrf") {
		t.Errorf("plain Render must not inject a csrf field: %s", plain.String())
	}
}

// Note: there is no "unknown widget type" render test any more. Dispatch is
// polymorphic (each widget implements renderInto), so an unhandled type can't be
// constructed — the compiler enforces it. The failure mode the old switch's default
// branch guarded no longer exists.
