// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"
)

var scriptTag = regexp.MustCompile(`<script src="(/assets/[^"]+)"`)

// TestEveryScriptThePageAsksForIsServed walks the rendered page for its own script
// tags and fetches each one. The shell's behaviours are several files, one per
// concern (ADR-004), and each is named in three places: the page that loads it,
// the embed directive that ships it, and the stylesheet's source list. A name that
// falls out of step with any of them fails silently — the browser 404s one file,
// the console says so to nobody, and whole interactions are simply dead. This is
// the test that notices.
func TestEveryScriptThePageAsksForIsServed(t *testing.T) {
	srv := newServer(t, fakeBackend{})
	token := srv.sessions.CreateWithMetadata("test-sid", "root", "", "")
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: token})
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /: status = %d, want 200", rec.Code)
	}

	matches := scriptTag.FindAllStringSubmatch(rec.Body.String(), -1)
	if len(matches) < 8 {
		t.Fatalf("the page loads %d scripts; the behaviours alone are more than that", len(matches))
	}
	seen := make(map[string]bool, len(matches))
	for _, m := range matches {
		src := m[1]
		if seen[src] {
			t.Errorf("%s is loaded twice", src)
		}
		seen[src] = true
		asset := httptest.NewRequest(http.MethodGet, src, nil)
		got := httptest.NewRecorder()
		srv.Handler().ServeHTTP(got, asset)
		if got.Code != http.StatusOK {
			t.Errorf("%s: status = %d, want 200 — the page asks for a file nothing serves", src, got.Code)
		}
		if got.Body.Len() == 0 {
			t.Errorf("%s is served empty", src)
		}
	}

	// Order. Deferred scripts run in the order the document names them, and two
	// things depend on that: verso.js defines the T() every behaviour file calls,
	// and Alpine boots on load and fires alpine:init exactly once, so a component
	// registered after Alpine is never registered at all.
	//
	// verso-boot.js is not in this reckoning: it runs in the head, before the page
	// paints, because what it does has to happen before anything is drawn.
	order := make([]string, 0, len(matches))
	for _, m := range matches {
		order = append(order, m[1])
	}
	at := func(src string) int {
		for i, s := range order {
			if s == src {
				return i
			}
		}
		return -1
	}
	core, alpine := at("/assets/verso.js"), at("/assets/alpine.csp.min.js")
	if core < 0 {
		t.Fatal("the page does not load verso.js")
	}
	if alpine < 0 {
		t.Fatal("the page does not load Alpine")
	}
	buttons := at("/assets/verso-buttons.js")
	for _, consumer := range []string{"/assets/verso-commit.js", "/assets/verso-system.js", "/assets/verso-packages.js"} {
		if buttons < 0 || buttons > at(consumer) {
			t.Errorf("the shared button state must load before %s", consumer)
		}
	}
	behaviour := func(src string) bool {
		return strings.HasPrefix(src, "/assets/verso-") &&
			src != "/assets/verso-boot.js" && src != "/assets/verso-dev.js"
	}
	for i, src := range order {
		if behaviour(src) && i < core {
			t.Errorf("%s loads before verso.js, whose T() it calls", src)
		}
		if (behaviour(src) || src == "/assets/verso.js") && i > alpine {
			t.Errorf("%s loads after Alpine; a component it registers would never be registered", src)
		}
	}
	// And the split is real: the concerns are separate files, not one again.
	var count int
	for _, src := range order {
		if behaviour(src) {
			count++
		}
	}
	if count < 5 {
		t.Errorf("the page loads %d behaviour files beside verso.js; the concerns are more separate than that", count)
	}
}

// A package install can open over any page. Its waiting mark must belong to
// the shell, even when the Packages listing has never been rendered.
func TestWaitingMarkAvailableOutsidePackages(t *testing.T) {
	s := passwordServer(t, fakeBackend{})
	for _, route := range []string{"/", "/system/access", "/system/maintenance"} {
		body := get(t, s, route).Body.String()
		_, mark, found := strings.Cut(body, `<template data-verso-button-waiting>`)
		if !found {
			t.Fatalf("%s is missing the shared waiting template", route)
		}
		mark, _, _ = strings.Cut(mark, `</template>`)
		if !strings.Contains(mark, `data-verso-wait aria-hidden="true"`) || strings.Count(mark, "size-1.5") != 4 {
			t.Errorf("%s must supply the four-square waiting mark", route)
		}
	}
}

// TestEveryBehaviourFileIsEmbedded is the other half: a file on disk that nothing
// ships is a behaviour that works in a dev session and is missing from the binary.
func TestEveryBehaviourFileIsEmbedded(t *testing.T) {
	onDisk, err := os.ReadDir("assets")
	if err != nil {
		t.Skipf("asset directory not readable: %v", err)
	}
	for _, entry := range onDisk {
		name := entry.Name()
		if entry.IsDir() || !strings.HasPrefix(name, "verso") || !strings.HasSuffix(name, ".js") {
			continue
		}
		if _, err := scriptFS.ReadFile("assets/" + name); err != nil {
			t.Errorf("%s is on disk but not embedded: %v", name, err)
		}
	}
}

// TestAConsoleLineIsARowOfItsGrid: a firewall log line takes its columns from
// the console's one grid (a subgrid row), so no end of the path carries a
// width of its own that a long IPv6 address could spill out of.
func TestAConsoleLineIsARowOfItsGrid(t *testing.T) {
	src, err := scriptFS.ReadFile("assets/verso-listing.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(src)
	start := strings.Index(js, "var CONSOLE = {")
	if start < 0 {
		t.Fatal("verso-listing.js has no CONSOLE line shape")
	}
	shape := js[start : start+strings.Index(js[start:], "};")]
	if !strings.Contains(shape, `row: "group col-span-full grid grid-cols-subgrid `) {
		t.Errorf("a console line is not a row of the console's grid:\n%s", shape)
	}
	for _, fixed := range []string{"w-66", "w-18 ", "w-22 ", "w-24 ", "flex-1"} {
		if strings.Contains(shape, fixed) {
			t.Errorf("a console column still sizes itself (%q):\n%s", fixed, shape)
		}
	}
	// Nothing on a line is cut short, as nothing on the system log's is: an
	// address shows whole, and a rule's name wraps rather than ending in an
	// ellipsis.
	build := js[strings.Index(js, "function build(ev)"):]
	build = build[:strings.Index(build, "\n    }\n")]
	if strings.Contains(shape, "truncate") || strings.Contains(build, "truncate") {
		t.Errorf("a console line cuts a value short:\n%s\n%s", shape, build)
	}
	if !strings.Contains(build, "wrap-break-word") {
		t.Errorf("a rule's name does not wrap:\n%s", build)
	}
	// An address that the window cannot hold whole wraps at its own
	// separators — a break after each ":" or "." — never mid-group, and a
	// copy of it is still the plain string.
	if !strings.Contains(js, `document.createElement("wbr")`) || strings.Contains(shape, "whitespace-nowrap") {
		t.Errorf("an address cannot wrap at its separators:\n%s", shape)
	}
}

// TestABusyButtonTakesItsLabelsWidth: a button waiting on its act holds the
// spinner and its busy words at their own width, as a busy button the server
// draws does — never pinned to the width its resting label had.
func TestABusyButtonTakesItsLabelsWidth(t *testing.T) {
	src, err := scriptFS.ReadFile("assets/verso-buttons.js")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(src), "minWidth") {
		t.Error("verso-buttons.js pins a busy button to its resting width")
	}
}

// TestAPackageDrawerShowsItsFilesAsItOpens: the files a package installed are
// always visible in its drawer — read in when the drawer opens (the link comes
// into view), not on a click, and never for every row at page load. The link
// stays as the way in without script.
func TestAPackageDrawerShowsItsFilesAsItOpens(t *testing.T) {
	src, err := scriptFS.ReadFile("assets/verso-packages.js")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"IntersectionObserver", `a[href^="/system/packages/files?"]`, "loadFiles("} {
		if !strings.Contains(string(src), want) {
			t.Errorf("verso-packages.js should read a drawer's files in as it opens; missing %q", want)
		}
	}
}

// TestAReshapingChoiceAsksForItsForm: changing a choice marked to reshape its
// form posts the form as a reshape — the mark the gateway answers without
// staging — into the drawer that holds it, and the live preview stands aside
// for it, since the form that comes back brings its own.
func TestAReshapingChoiceAsksForItsForm(t *testing.T) {
	forms, err := scriptFS.ReadFile("assets/verso-forms.js")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`el.closest("[data-verso-reshape]")`,
		`headers: { "X-Verso-Interaction": "reshape" }`,
		`versoPost(url, versoBody(new FormData(form)), "reshape")`,
		"if (el.closest(\"[data-verso-reshape]\")) return;",
		// The waiting mark beside the choice turns while the form is on its way.
		`row.querySelector("[data-verso-reshape-wait]")`,
		`if (mark) mark.hidden = false;`,
		`form.setAttribute("aria-busy", "true");`,
	} {
		if !strings.Contains(string(forms), want) {
			t.Errorf("verso-forms.js is missing %s", want)
		}
	}
}

// TestADevRedeployWaitsForUnsavedWork: the dev reload that follows a redeploy
// asks the page's unsaved-work guard first and holds while it answers yes, so a
// redeploy never throws the browser's leave prompt over a half-made edit; the
// guard answers without changing anything.
func TestADevRedeployWaitsForUnsavedWork(t *testing.T) {
	forms, err := scriptFS.ReadFile("assets/verso-forms.js")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(forms), "dirty: function () {") {
		t.Error("versoDirtyState has no read-only dirty() for the dev reload to ask")
	}
	dev, err := scriptFS.ReadFile("assets/verso-dev.js")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(dev), "window.versoDirtyState.dirty()") {
		t.Error("the dev reload does not ask whether the page holds unsaved work")
	}
}

// TestACardMeasuresOnTheEightPixelGrid: a page's card keeps its words 24px in
// from its frame, stands 24px from the card before it, and its header and its
// column-head row are 48px each, hairlines included (layout-grid). Its frame's
// borders are lines: the bottom air and the gap after it give back a pixel.
func TestACardMeasuresOnTheEightPixelGrid(t *testing.T) {
	raw, err := os.ReadFile("assets/sections.css")
	if err != nil {
		t.Fatal(err)
	}
	css := string(raw)
	for _, want := range []string{
		// the inset: a pixel short of 24px after each side border, so the body
		// is a multiple of 8px and a pixel wide and a box across it has its
		// borders on lines; the reach of its rules matches it
		":root { --verso-side: calc(var(--spacing-inset) - 1px); }",
		"--verso-reach-start: var(--verso-side);\n  --verso-reach-end: var(--verso-side);",
		// (the body ends on a line, so the full 24px under it, and a pixel
		// short after the frame or the header's hairline, so its lines fill
		// whole bands of the grid)
		"padding: 0 var(--verso-side) var(--spacing-inset);",
		"padding-top: calc(var(--spacing-inset) - 1px);",
		"+ .verso-rhythm:not(:has(> [data-verso-listing]:not([data-verso-list]):first-child)) {\n  margin-top: -1px;\n}",
		// the header: 11px after the frame, its 24px line, 12px under, then its
		// hairline, the title centred on the header's middle line
		"padding-block: calc(var(--spacing) * 2.75) calc(var(--spacing) * 3);\n  border-top-width: 0;\n  border-bottom: 1px solid var(--color-rule);",
		// the column-head row: its 16px line 15px after the edge over it, 16px
		// over its own hairline
		"padding-block: calc(var(--spacing) * 4 - 1px) calc(var(--spacing) * 4);",
		// the air between cards, less the frame's pixel so the next frame lands
		// on a line
		"[data-verso-section-band]) {\n  margin-top: calc(var(--spacing-inset) - 1px);\n}",
	} {
		if !strings.Contains(css, want) {
			t.Errorf("a card does not measure on the 8px grid; sections.css is missing %s", want)
		}
	}
}

// TestAPagesPanelStandsCentred: a panel in a page's card (the Hardware page's
// board card) is centred across it, and the rule reaches no further: the
// Overview's Ports card is an article, not a page section, and a drawer is a
// dialog, so their panels keep flush with what stands under them.
func TestAPagesPanelStandsCentred(t *testing.T) {
	raw, err := os.ReadFile("assets/sections.css")
	if err != nil {
		t.Fatal(err)
	}
	css := string(raw)
	const scope = ".verso-page-body section[data-verso-section]:not(dialog *) "
	for _, want := range []string{
		scope + ".verso-panel-face svg {\n  margin-inline: auto;\n}",
		scope + ".verso-panel-art {\n  text-align: center;\n}",
		scope + ".verso-ports-legend {\n  justify-content: center;\n}",
	} {
		if !strings.Contains(css, want) {
			t.Errorf("sections.css is missing %s", want)
		}
	}
	for _, line := range strings.Split(css, "\n") {
		if strings.Contains(line, "verso-panel") && strings.Contains(line, "verso-overview-card") {
			t.Errorf("the centring reaches the Overview's Ports card: %s", line)
		}
		if strings.Contains(line, "verso-panel-face svg") && !strings.Contains(line, scope) {
			t.Errorf("a panel is centred outside a page's card: %s", line)
		}
	}
}

// TestTheThemeMeasuresOnTheEightPixelGrid: every fixed measure of the layout
// is a multiple of 8px (layout-grid), stated once as a theme token: a control,
// a card's inset and air, the page frame, and the page measures.
func TestTheThemeMeasuresOnTheEightPixelGrid(t *testing.T) {
	raw, err := os.ReadFile("assets/input.css")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"--spacing-control: calc(2rem + 1px);",
		"--spacing-inset: 1.5rem;",
		"--spacing-frame: 2.5rem;",
		"--spacing-act: 1.75rem;",
		"--container-3xl: 47.5rem;",
		"--container-4xl: 55rem;",
		"--container-5xl: 63.5rem;",
		"--container-6xl: 71rem;",
	} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("the theme does not measure on the 8px grid: missing %s", want)
		}
	}
}

// TestAPanelMeasuresOnTheEightPixelGrid: a router's panel is drawn at its own
// size where it fits, so every edge it draws stands on a line, and where it
// does not it shrinks to a whole number of 8px steps tall, its width following
// its proportions and its left edge the content's (device-panel).
func TestAPanelMeasuresOnTheEightPixelGrid(t *testing.T) {
	raw, err := os.ReadFile("assets/input.css")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		".verso-panel-art { position: relative; container-type: inline-size; }",
		".verso-panel-face svg { display: block; width: auto; max-width: none; height: min(var(--art-h), round(down, 100cqi * var(--art-aspect), 0.5rem)); }",
	} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("the panel does not measure on the 8px grid: input.css is missing %s", want)
		}
	}
	// No panel rule sizes the art any other way.
	if strings.Contains(string(raw), ".verso-panel-art svg { display: block; width: 100%; height: auto; }") {
		t.Error("the panel's art is still stretched to its column's width, off the grid")
	}
}

// TestReducedMotionStillsTheTrafficLED: under reduced motion a busy port's
// traffic LED stays lit and still (device-panel). The rule that stops it must
// match the rule that starts it selector for selector, or the blink, being
// more specific, wins and keeps blinking.
func TestReducedMotionStillsTheTrafficLED(t *testing.T) {
	raw, err := os.ReadFile("assets/input.css")
	if err != nil {
		t.Fatal(err)
	}
	css := string(raw)
	const blinking = ".verso-panel-art [data-verso-port]:is(.act-1, .act-2, .act-3, .act-4) .led-traffic"
	if !strings.Contains(css, blinking+" {\n    fill:") {
		t.Fatalf("input.css no longer starts the blink with %s", blinking)
	}
	if !strings.Contains(css, "@media (prefers-reduced-motion: reduce) {\n    "+blinking+" { animation: none; }") {
		t.Errorf("reduced motion must stop the blink with the same selector: %s { animation: none; }", blinking)
	}
}

// TestADevSessionCanDrawTheGrid: in a dev session G draws the 8px grid over
// the page, from the top bar's hairline and the rail's border, and over a
// drawer's body from its chrome's hairline (layout-grid). Only a dev session
// loads the script that sets it, so a released build never draws a grid.
func TestADevSessionCanDrawTheGrid(t *testing.T) {
	page := func(srv *Server) string {
		token := srv.sessions.CreateWithMetadata("test-sid", "root", "", "")
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.AddCookie(&http.Cookie{Name: sessionCookie, Value: token})
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)
		return rec.Body.String()
	}
	released := newServer(t, fakeBackend{})
	if strings.Contains(page(released), "/assets/verso-dev.js") {
		t.Error("a released build loads the dev script, which can draw the grid")
	}
	dev := newServer(t, fakeBackend{})
	dev.devCSS = t.TempDir() + "/verso-dev.css"
	if err := os.WriteFile(dev.devCSS, []byte("main{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(page(dev), "/assets/verso-dev.js") {
		t.Error("a dev session does not load the dev script")
	}

	css, err := os.ReadFile("assets/verso.css")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		":root[data-verso-grid] main{position:relative}",
		":root[data-verso-grid] main:after{",
		"pointer-events:none",
		"background-size:calc(var(--spacing) * 2) calc(var(--spacing) * 2)",
		":root[data-verso-grid] [data-verso-drawer-body]{",
		"background-attachment:local",
	} {
		if !strings.Contains(string(css), want) {
			t.Errorf("the dev grid is not drawn; stylesheet is missing %s", want)
		}
	}
}

// TestACardUnderACardDrawsNoSecondDivider: a form that writes more than one
// file shows a card per file, one under the other. The rule above the first
// sets the files off from the form; the next card is more of the same answer,
// so it stands a cell under the one before, its frame on the line, with no
// rule of its own.
func TestACardUnderACardDrawsNoSecondDivider(t *testing.T) {
	css, err := os.ReadFile("assets/verso.css")
	if err != nil {
		t.Fatalf("read stylesheet: %v", err)
	}
	want := "[data-verso-code-divider]+[data-verso-code-divider]{margin-top:calc(var(--spacing-inset) - 1px);border-top-width:0;padding-top:0}"
	if !strings.Contains(string(css), want) {
		t.Errorf("stylesheet is missing %s", want)
	}
}

// TestStylesheetTypeSystem: the two faces are the only faces, and each is
// declared across the weights its file actually carries (both are variable),
// so a weight the design asks for (the top bar's light maker line) is drawn
// rather than clamped to the nearest declared one. The kicker is the canvas's
// one kicker: 12px, 500, uppercase, .08em, in the meta ink.
func TestStylesheetTypeSystem(t *testing.T) {
	raw, err := os.ReadFile("assets/verso.css")
	if err != nil {
		t.Fatalf("read stylesheet: %v", err)
	}
	css := string(raw)
	for _, want := range []string{
		`font-family:Hanken Grotesk;font-style:normal;font-weight:100 900`,
		`font-family:Inconsolata;font-style:normal;font-weight:200 900`,
		// bare code, kbd, samp and pre take the preflight's default mono
		`--default-mono-font-family:var(--font-mono)`,
	} {
		if !strings.Contains(css, want) {
			t.Errorf("stylesheet is missing %q", want)
		}
	}
	kickerAt := strings.Index(css, ".verso-kicker{")
	if kickerAt < 0 {
		t.Fatal("no kicker rule")
	}
	kicker := css[kickerAt : kickerAt+strings.Index(css[kickerAt:], "}")]
	for _, want := range []string{"font-size:.75rem", "font-weight:500", "letter-spacing:.08em", "text-transform:uppercase", "color:var(--color-meta)"} {
		if !strings.Contains(kicker, want) {
			t.Errorf("kicker is missing %q: %s", want, kicker)
		}
	}
	// A rule that names the system's monospace first skips Inconsolata.
	if strings.Contains(css, "font-family:ui-monospace") {
		t.Errorf("a third face leaks in through ui-monospace; mono is Inconsolata")
	}
}

// TestStylesheetKeepsOnePageRhythm: a page's blocks stand apart by the body's
// own inset, 2.5rem (two cells) — the gap the page keeps from the window's top
// and left edge — and a control band's listing stands flush under it, the
// band being its card's top strip. A log stays flush on its bar. A section on the page takes its standoff from that
// rhythm rather than adding its own on top, and blocks inside a section keep
// the same 2.5rem.
func TestStylesheetKeepsOnePageRhythm(t *testing.T) {
	css, err := os.ReadFile("assets/verso.css")
	if err != nil {
		t.Fatalf("read stylesheet: %v", err)
	}
	for _, want := range []string{
		".verso-page-body>.verso-stack>*+*{margin-top:calc(var(--spacing) * 10)}",
		// the listing joins its band, as a log (and its notice) sits flush on its bar
		".verso-page-body>.verso-stack>[data-verso-actionbar]{margin-bottom:0}",
		".verso-page-body>.verso-stack>[data-verso-actionbar]+*,.verso-page-body>.verso-stack>.verso-console,.verso-page-body>.verso-stack>.verso-console-notice{margin-top:0}",
		".verso-page-body>.verso-stack>section[data-verso-section]:not([data-verso-section=ruled],[data-verso-section=part]){padding-top:0}",
		"margin-top:var(--verso-rhythm,calc(var(--spacing) * 10))",
	} {
		if !strings.Contains(string(css), want) {
			t.Errorf("stylesheet is missing %q", want)
		}
	}
	// a page's control band is a box in the page's column, never a bar
	// reaching to the rail and the window's edge
	if strings.Contains(string(css), "[data-verso-actionbar]{margin-inline") {
		t.Error("the control band reaches past the page's column")
	}
}

// TestSectionRulesRunToTheRailOnly: a page's rules between its sections run
// out to the rail on the left and stop where the column ends on the right, so
// a column with a sidebar beside it (Maintenance's "On this page") keeps a gap
// between the rule and the sidebar.
func TestSectionRulesRunToTheRailOnly(t *testing.T) {
	css, err := os.ReadFile("assets/verso.css")
	if err != nil {
		t.Fatalf("read stylesheet: %v", err)
	}
	// Every rule reaches by its surface's two measures (sections.css): a page
	// runs 40px out to the rail and none past the column's end; a page card
	// keeps its rules to its own edges.
	for _, want := range []string{
		"main{--verso-reach-start:calc(var(--spacing) * 10);--verso-reach-end:0px}",
		"margin-inline:calc(-1 * var(--verso-reach-start)) calc(-1 * var(--verso-reach-end));padding-inline:var(--verso-reach-start) var(--verso-reach-end)",
	} {
		if !strings.Contains(string(css), want) {
			t.Errorf("stylesheet is missing %q", want)
		}
	}
}

// TestARuledSectionStandsByItsRulesAir: a ruled section on the page's stack
// puts its rule in the stack's two cells after the block before it, less the
// rule's own pixel so the rule lands on a line of the grid — not the plain
// 40px between unruled blocks, which would set the rule a pixel off it.
func TestARuledSectionStandsByItsRulesAir(t *testing.T) {
	css, err := os.ReadFile("assets/sections.css")
	if err != nil {
		t.Fatalf("read stylesheet: %v", err)
	}
	if !strings.Contains(string(css), `.verso-page-body > .verso-stack > section:is([data-verso-section="ruled"], [data-verso-section="part"]) { margin-top: calc(var(--spacing) * 10 - 1px); }`) {
		t.Error("a ruled section on the page's stack does not land its rule on a line")
	}
}

// TestTheOverviewHeadingSpeaksInItsTone: a heading that names what is wrong
// is a message about now, so it reads in its tone's deep step (the masthead
// tone contract); the live stream retones it by its attribute alone.
func TestTheOverviewHeadingSpeaksInItsTone(t *testing.T) {
	raw, err := os.ReadFile("assets/input.css")
	if err != nil {
		t.Fatalf("read stylesheet: %v", err)
	}
	for _, want := range []string{
		`[data-overview-tone="warning"] [data-overview-lead] { color: var(--color-marigold-deep); }`,
		`[data-overview-tone="danger"] [data-overview-lead] { color: var(--color-crimson-deep); }`,
	} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("stylesheet is missing %s", want)
		}
	}
}

// TestASectionKeepsTheMastheadsAirAndNoMore: an untitled section holds the
// cell of air under its rule itself, so the row it starts with gives its own
// padding back — its first visible row starts where the section's air ends,
// inside a run of rows too, and inside the form a card saves itself with when
// that form is the card's body. Every row ends on a line of its own, so a
// section's last row keeps its padding: no section gives its last row's air
// back. Hidden carriers ahead of the first row are not a block: the first row
// after them takes no block gap.
func TestASectionKeepsTheMastheadsAirAndNoMore(t *testing.T) {
	raw, err := os.ReadFile("assets/verso.css")
	if err != nil {
		t.Fatalf("read stylesheet: %v", err)
	}
	css := string(raw)
	for _, want := range []string{
		`[data-verso-headless]>.verso-rhythm>:not(input[type=hidden],[hidden]):not(:not(input[type=hidden],[hidden])~*),` +
			`[data-verso-headless]>.verso-rhythm>[data-verso-rows]:not(input[type=hidden],[hidden]):not(:not(input[type=hidden],[hidden])~*)>:not(input[type=hidden],[hidden]):not(:not(input[type=hidden],[hidden])~*),` +
			`[data-verso-headless]>.verso-rhythm>form.verso-form:not(:not(input[type=hidden],[hidden])~*)>:not(input[type=hidden],[hidden]):not(:not(input[type=hidden],[hidden])~*){padding-top:0}`,
		`.verso-rhythm>:not(input[type=hidden]):not(:not(input[type=hidden])~*){margin-top:0}`,
	} {
		if !strings.Contains(css, want) {
			t.Errorf("stylesheet is missing %s", want)
		}
	}
	if strings.Contains(css, `[data-verso-section]>.verso-rhythm>:not(input[type=hidden],[hidden]):not(:has(~:not(input[type=hidden],[hidden])))`) {
		t.Error("a section's last row gives its padding back, so it ends off the grid's line")
	}
}

// TestAPageFormOpensUnderTheMastheadsRule: a page form opens right under the
// heading, whose bar already ends on a hairline, so its first section draws no
// rule of its own above it — one line, not two.
func TestAPageFormOpensUnderTheMastheadsRule(t *testing.T) {
	css, err := os.ReadFile("assets/verso.css")
	if err != nil {
		t.Fatalf("read stylesheet: %v", err)
	}
	if strings.Contains(string(css), "[data-verso-page-form]>[data-verso-section=ruled][data-verso-headless]:first-of-type{") {
		t.Error("a page form's first section draws a rule under the masthead's own")
	}
}

// TestAPageOpeningOnAHeadingDrawsOneRule: a page whose first content is a
// section's heading band opens under the masthead's hairline, as a drawer's
// opens under its chrome's: the band draws no rule of its own, and its title
// is the cell under the masthead's own cell of air. An element with nothing in
// it (a notice block with no notice) is not content before the band, so no
// page has to take care not to render one.
func TestAPageOpeningOnAHeadingDrawsOneRule(t *testing.T) {
	css, err := os.ReadFile("assets/verso.css")
	if err != nil {
		t.Fatalf("read stylesheet: %v", err)
	}
	for _, want := range []string{
		".verso-page-body [data-verso-section-band]:not(.verso-page-body :not(input[type=hidden],template,script,[hidden],:empty)~* [data-verso-section-band],.verso-page-body :not(input[type=hidden],template,script,[hidden],:empty)~[data-verso-section-band]){border-top-width:0;padding-top:0}",
	} {
		if !strings.Contains(string(css), want) {
			t.Errorf("stylesheet is missing %s", want)
		}
	}
}

// TestMastheadEndsUnderLogOut: whatever measure a page's column keeps, its
// masthead runs from the rail to the window's right edge, while the title
// keeps to the page's column and the page's acts end at the right edge Log
// out ends at: the page's gutter in (16px on a phone, 40px past it), never
// past the wide measure, rounded down to the 8px grid and a pixel more, as a
// page's column is, so a right border at the end lands on a line. The pixel
// is taken before the cap, as the column's max-w-column-* does, and the
// page's width is read in whole pixels, so no width rounds a step short of
// the column's. The colophon's words end on the same line.
func TestMastheadEndsUnderLogOut(t *testing.T) {
	raw, err := os.ReadFile("assets/verso.css")
	if err != nil {
		t.Fatalf("read stylesheet: %v", err)
	}
	css := string(raw)
	for _, want := range []string{
		":root{--verso-gutter:calc(var(--spacing) * 4)}",
		":root{--verso-gutter:calc(var(--spacing) * 10)}",
		"main [data-verso-masthead]{--verso-measure:calc(round(down, min(round(100cqw, 1px) - var(--verso-gutter) * 2 - 1px, var(--container-6xl)), .5rem) + 1px);margin-inline:calc(var(--verso-gutter) * -1) calc(100% - round(100cqw, 1px) + var(--verso-gutter));padding-inline:var(--verso-gutter) calc(round(100cqw, 1px) - var(--verso-gutter) - var(--verso-measure))}",
		"[data-verso-session]>div{margin-inline:var(--verso-gutter);width:calc(round(down, min(100% - var(--verso-gutter) * 2 - 1px, var(--container-6xl)), .5rem) + 1px);flex:none}",
		"[data-verso-colophon]>div{max-width:none;width:calc(round(down, min(100% - var(--verso-gutter) * 2 - 1px, var(--container-6xl)), .5rem) + 1px + var(--verso-gutter) * 2)}",
		".w-column{width:calc(round(down,100% - 1px,.5rem) + 1px)}",
		".max-w-column-6xl{max-width:calc(var(--container-6xl) + 1px)}",
	} {
		if !strings.Contains(css, want) {
			t.Errorf("the masthead runs from the rail to the window's edge and its acts end where Log out does; stylesheet is missing %s", want)
		}
	}
}

// TestColophonHoldsStillBetweenPages: the colophon is one 56px line on every
// page, short or long, so moving between pages never moves it. A short page's
// ground takes every pixel down to it rather than rounding to the grid and
// handing the colophon the remainder, which would stand its words a few
// pixels higher on a short page than at the foot of a long one.
func TestColophonHoldsStillBetweenPages(t *testing.T) {
	raw, err := os.ReadFile("assets/verso.css")
	if err != nil {
		t.Fatalf("read stylesheet: %v", err)
	}
	css := string(raw)
	if !strings.Contains(css, ".verso-page-ground{flex:1 0 auto}") {
		t.Error("a short page's ground fills the window down to the colophon")
	}
	if !strings.Contains(css, "main>[data-verso-colophon]{flex:none}") {
		t.Error("the colophon keeps its own 56px whatever the window's height")
	}
	if strings.Contains(css, "100vh - var(--spacing) * 28") {
		t.Error("the ground is not rounded to the grid, leaving the colophon the remainder")
	}
}

// TestStylesheetKeepsFocusAndStillness: two things no utility on any one
// element can promise, so the stylesheet states them once for all. In forced
// colours (Windows high contrast) a border's colour is overridden, so a control
// that marks focus only by recolouring its border would show none; there,
// whatever holds keyboard focus is outlined in the system's own colour. And a
// reader who asked for less motion gets dialogs and drawers that appear rather
// than slide or scale in.
// The rail's page change is the stylesheet's alone: the browser carries the
// named rail parts from one page to the next, nothing else on the page moves,
// the carried parts never catch a press, and a reader who asks for stillness,
// or a phone whose rail is a closed drawer, gets the plain page load.
func TestTheRailCarriesItsPlaceAcrossAPageChange(t *testing.T) {
	css, err := os.ReadFile("assets/verso.css")
	if err != nil {
		t.Fatalf("read stylesheet: %v", err)
	}
	s := string(css)
	gate := "@media (min-width:48rem) and (prefers-reduced-motion:no-preference){@view-transition{navigation:auto}"
	if !strings.Contains(s, gate) {
		t.Fatalf("stylesheet must opt into page-change transitions only at rail width and without reduced motion (%q)", gate)
	}
	for _, want := range []string{
		"view-transition-name:var(--verso-vt)",
		"view-transition-class:var(--verso-vt-class,none)",
		":root{view-transition-name:none}",
		"::view-transition{pointer-events:none}",
		"::view-transition-new(.verso-nav-branch):only-child",
		"::view-transition-new(.verso-nav-sub):only-child",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("stylesheet is missing %q", want)
		}
	}
}

// TestTheRuleAfterASectionBringsItsOwnAir: no section keeps closing air;
// whatever draws a rule after one brings the air before it itself, its margin
// a whole number of cells less the rule's own pixel so the rule lands on a
// line: a ruled section two cells under the last row's line, a heading band's
// section the same, the configuration card's divider one. A section that kept
// air of its own while the rule after it zeroed its margin set every rule off
// the grid.
func TestTheRuleAfterASectionBringsItsOwnAir(t *testing.T) {
	raw, err := os.ReadFile("assets/sections.css")
	if err != nil {
		t.Fatal(err)
	}
	css := string(raw)
	// A notice or an empty slot is a box like any other, a multiple of 4px
	// tall with its frame, so it takes the full air before it.
	if strings.Contains(css, ":is([data-verso-slot], [data-verso-callout])") {
		t.Error("a notice or a slot still takes its air less a pixel, as a frame standing a pixel out did")
	}
	for _, want := range []string{
		"section[data-verso-section=\"ruled\"] {\n    margin-top: calc(var(--spacing) * 10 - 1px);\n    border-top: 1px solid var(--color-rule);\n    padding-top: var(--spacing-inset);\n  }",
		":not(input[type=\"hidden\"], template, [hidden]) + section[data-verso-section]:has(> [data-verso-section-band]),\n" +
			"  .verso-page-body > .verso-stack > * + section[data-verso-section]:has(> [data-verso-section-band]) {\n    margin-top: calc(var(--spacing) * 10 - 1px);\n  }",
		// a section with no heading that opens under a notice stands the
		// inset's air under it, rather than flush on it
		"[data-verso-callout] + section[data-verso-section][data-verso-headless] {\n    margin-top: var(--spacing-inset);\n  }",
		// a listing that ends its section gives its last hairline to the rule
		// that closes it, clear but keeping its pixel
		"section[data-verso-section] > .verso-rhythm > :last-child .verso-table > tbody > tr:last-child > td {\n    border-bottom-color: transparent;\n  }",
		// the configuration card's rule is a section's rule: 40px under what it
		// follows, which ends on a line, the card 40px under it, less the
		// rule's own pixel
		"[data-verso-code-divider] {\n    margin-top: calc(var(--spacing) * 10);\n    padding-top: calc(var(--spacing) * 10 - 1px);\n  }",
	} {
		if !strings.Contains(css, want) {
			t.Errorf("sections.css is missing %s", want)
		}
	}
	for _, gone := range []string{
		"[data-verso-section] + [data-verso-code-divider]",
		"padding-block: 0 calc(var(--spacing) * 8)",
	} {
		if strings.Contains(css, gone) {
			t.Errorf("sections.css still trades a section's closing air for the next rule's: %s", gone)
		}
	}
}

// TestASectionTrimsThroughARunOfRows: rows grouped in a flush stack are still
// the section's rows, so an untitled section's first row gives its air back
// from inside the group too. A port forward's panel opened lower than a DHCP
// server's because its first rows stood in one. The last row keeps its
// padding, in a group or not, so it ends on a line.
func TestASectionTrimsThroughARunOfRows(t *testing.T) {
	raw, err := os.ReadFile("assets/sections.css")
	if err != nil {
		t.Fatal(err)
	}
	css := string(raw)
	want := "[data-verso-headless] > .verso-rhythm > [data-verso-rows]:not(input[type=\"hidden\"], [hidden]):not(:not(input[type=\"hidden\"], [hidden]) ~ *) > :not(input[type=\"hidden\"], [hidden]):not(:not(input[type=\"hidden\"], [hidden]) ~ *) { padding-top: 0; }"
	if !strings.Contains(css, want) {
		t.Errorf("sections.css is missing %s", want)
	}
	if strings.Contains(css, "[data-verso-section] > .verso-rhythm > [data-verso-rows]") {
		t.Error("a run of rows gives its last row's air back, so it ends off the grid's line")
	}
}

// TestAnUntitledOpeningKeepsNoClosingAir: a drawer's untitled opening block
// (a forward's state and name, a DHCP server's addresses) ends on its last
// row's line, as every section does, and the band, card or Save after it
// brings its own air (TestTheRuleAfterASectionBringsItsOwnAir); an opening
// that kept closing air of its own would push that rule off the line.
func TestAnUntitledOpeningKeepsNoClosingAir(t *testing.T) {
	raw, err := os.ReadFile("assets/sections.css")
	if err != nil {
		t.Fatal(err)
	}
	css := string(raw)
	if strings.Contains(css, "[data-verso-headless]:not([data-verso-section=\"ruled\"]):has(") {
		t.Error("an untitled opening keeps closing air before the rule after it")
	}
	if strings.Contains(css, "padding-bottom: calc(var(--spacing) * 8)") {
		t.Error("a section keeps closing air of its own")
	}
}

// TestAFittingTableClipsNoTip: a table's box scrolls, and so clips both ways,
// only while its table is wider than it; the shell measures every box and one
// whose table fits neither scrolls, clips nor fades, so a first row's act
// raises its tip over the column heads. While the table is wider, the first
// row's tips open under their act rather than into the clipping edge.
func TestAFittingTableClipsNoTip(t *testing.T) {
	css, err := os.ReadFile("assets/input.css")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"[data-verso-scroller][data-verso-fits] {\n    overflow: visible;\n    mask-image: none;\n    animation: none;\n  }",
		"[data-verso-scroller]:not([data-verso-fits]) .verso-table > tbody > tr:first-child .group\\/act > span.bottom-full {\n    top: 100%;",
	} {
		if !strings.Contains(string(css), want) {
			t.Errorf("input.css missing %q", want)
		}
	}
	js, err := os.ReadFile("assets/verso-tables.js")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"new ResizeObserver(", `box.toggleAttribute("data-verso-fits"`, `"htmx:afterSettle"`} {
		if !strings.Contains(string(js), want) {
			t.Errorf("verso-tables.js does not measure the table boxes (%q)", want)
		}
	}
}

// TestTipsRiseAboveTheirLabel: an explanation opens above the label that
// raises it, so the pointer on its way down to the control never meets it; a
// pointer passing over the label raises nothing, because the tip waits for it
// to settle, while keyboard focus raises it at once. Until it shows it takes no
// pointer and no room: a waiting tip by a chip near the window's edge would
// otherwise widen the page past the window, and a phone would scroll sideways.
// It still fades in and out. Below is only where there is no room above.
func TestTipsRiseAboveTheirLabel(t *testing.T) {
	css, err := os.ReadFile("assets/input.css")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"[data-verso-tip] > [role=\"tooltip\"] {\n    display: none;\n    bottom: 100%;\n    margin-bottom: 0.5rem;\n    visibility: hidden;\n    opacity: 0;\n    pointer-events: none;\n    transition: opacity 150ms ease-out, visibility 0s linear 150ms, display 0s linear 150ms;\n    transition-behavior: allow-discrete;",
		"[data-verso-tip]:hover > [role=\"tooltip\"] {\n    display: block;\n    visibility: visible;\n    opacity: 1;\n    pointer-events: auto;\n    transition-delay: 200ms;",
		"[data-verso-tip]:focus-visible > [role=\"tooltip\"] {\n    display: block;\n    visibility: visible;\n    opacity: 1;\n    pointer-events: auto;\n    transition-delay: 0ms;",
		"@starting-style {\n    [data-verso-tip]:is(:hover, :focus-visible) > [role=\"tooltip\"] {\n      visibility: hidden;\n      opacity: 0;\n    }\n  }",
		"[data-verso-tip-below] > [role=\"tooltip\"] {\n    top: 100%;\n    bottom: auto;",
		"[data-verso-tip-dismissed] > [role=\"tooltip\"] {\n    display: none !important;",
	} {
		if !strings.Contains(string(css), want) {
			t.Errorf("stylesheet is missing %s", want)
		}
	}
	if strings.Contains(string(css), "data-verso-tip-above") {
		t.Error("a tip flips below, never above: above is where it opens")
	}
	for _, tmpl := range []string{"../widget/templates/shared.html.tmpl"} {
		src, err := os.ReadFile(tmpl)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(src), "group-hover/tip:opacity-100") {
			t.Errorf("%s reveals a tip itself; the stylesheet owns the reveal and its pause", tmpl)
		}
		// A display utility on the tip outranks the stylesheet's display: none,
		// and the waiting tip would take its room again.
		for _, tip := range regexp.MustCompile(`<span role="tooltip"[^>]*class="([^"]*)"`).FindAllStringSubmatch(string(src), -1) {
			for _, class := range strings.Fields(tip[1]) {
				if class == "flex" || class == "block" || class == "grid" || class == "inline-flex" {
					t.Errorf("%s sets a tip's display (%s); the stylesheet owns it", tmpl, class)
				}
			}
		}
	}
}

func TestStylesheetKeepsFocusAndStillness(t *testing.T) {
	css, err := os.ReadFile("assets/verso.css")
	if err != nil {
		t.Fatalf("read stylesheet: %v", err)
	}
	for _, want := range []string{
		"@media (forced-colors:active){:focus-visible{",
		"outline:2px solid canvastext!important",
		`@media (prefers-reduced-motion:reduce){dialog{transition-duration:0s!important`,
	} {
		if !strings.Contains(string(css), want) {
			t.Errorf("stylesheet is missing %q", want)
		}
	}
}

// TestNoPageStylesItsOwnListRows: a list's value rows wear the widget's one
// look (list.html.tmpl) on every page; a page restyling its own would drift
// from the rest.
func TestNoPageStylesItsOwnListRows(t *testing.T) {
	css, err := os.ReadFile("assets/forms.css")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(css), "[data-verso-list-row]") || strings.Contains(string(css), "[data-verso-list-value]") {
		t.Error("a page styles its own list rows")
	}
}
