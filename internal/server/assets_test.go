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
	token, err := srv.sessions.CreateWithMetadata("test-sid", "root", "", "")
	if err != nil {
		t.Fatalf("session: %v", err)
	}
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
// own inset, 2.5rem — the gap the page keeps from the window's top and left
// edge — and a control band's listing sits flush on it, its column heads 1rem
// under the band. A section on the page takes its standoff from that
// rhythm rather than adding its own on top, and blocks inside a section keep
// the same 2.5rem.
func TestStylesheetKeepsOnePageRhythm(t *testing.T) {
	css, err := os.ReadFile("assets/verso.css")
	if err != nil {
		t.Fatalf("read stylesheet: %v", err)
	}
	for _, want := range []string{
		".verso-page-body>.verso-stack>*+*{margin-top:calc(var(--spacing) * 10)}",
		// the listing sits flush on its band, and a log (and its notice) on its bar
		".verso-page-body>.verso-stack>[data-verso-actionbar]+*,.verso-page-body>.verso-stack>.verso-console,.verso-page-body>.verso-stack>.verso-console-notice{margin-top:0}",
		".verso-page-body>.verso-stack>section[data-verso-section]:not([data-verso-section=ruled],[data-verso-section=part]){padding-top:0}",
		// a page's control band spans the page, its controls in the column
		".verso-page-body>.verso-stack>[data-verso-actionbar],.verso-page-body>[data-verso-packages]>[data-verso-actionbar]{margin-inline:calc(var(--spacing) * -10) calc(100% - 100cqw + var(--spacing) * 10);padding-inline:calc(var(--spacing) * 10) calc(100cqw - 100% - var(--spacing) * 10)}",
		"margin-top:var(--verso-rhythm,calc(var(--spacing) * 10))",
	} {
		if !strings.Contains(string(css), want) {
			t.Errorf("stylesheet is missing %q", want)
		}
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
	// runs 40px out to the rail and none past the column's end; only a form
	// column, which pads its own end, reaches back through it.
	for _, want := range []string{
		"main{--verso-reach-start:calc(var(--spacing) * 10);--verso-reach-end:0px}",
		"[data-verso-form-column]:not([role=dialog] *){--verso-reach-end:calc(var(--spacing) * 10)}",
		"margin-inline:calc(-1 * var(--verso-reach-start)) calc(-1 * var(--verso-reach-end));padding-inline:var(--verso-reach-start) var(--verso-reach-end)",
	} {
		if !strings.Contains(string(css), want) {
			t.Errorf("stylesheet is missing %q", want)
		}
	}
}

// TestARuledSectionStandsByItsRulesAir: a ruled section on the page's stack
// stands 24px after the block before it — the title's own air, which every
// rule keeps on both sides — not the 40px between unruled blocks.
func TestARuledSectionStandsByItsRulesAir(t *testing.T) {
	css, err := os.ReadFile("assets/verso.css")
	if err != nil {
		t.Fatalf("read stylesheet: %v", err)
	}
	if !strings.Contains(string(css), ".verso-page-body>.verso-stack>section:is([data-verso-section=ruled],[data-verso-section=part]){margin-top:calc(var(--spacing) * 6)}") {
		t.Error("a ruled section on the page's stack keeps the 40px block gap")
	}
}

// TestASectionKeepsTheMastheadsAirAndNoMore: a section holds the masthead's
// 20px inside its rule itself, so the rows it starts and ends with give their
// own padding back — an untitled section's first visible row starts where the
// section does, and every section's last visible row ends where it does.
// Hidden carriers ahead of the first row are not a block: the first row after
// them takes no block gap. A branch that is hidden (a `when` whose control
// holds another value) is not a row either, so the row before it can still be
// the last; and a branch that shows and ends the section ends on its own last
// row.
func TestASectionKeepsTheMastheadsAirAndNoMore(t *testing.T) {
	raw, err := os.ReadFile("assets/verso.css")
	if err != nil {
		t.Fatalf("read stylesheet: %v", err)
	}
	css := string(raw)
	for _, want := range []string{
		`[data-verso-headless]>.verso-rhythm>:not(input[type=hidden],[hidden]):not(:not(input[type=hidden],[hidden])~*){padding-top:0}`,
		`[data-verso-section]>.verso-rhythm>:not(input[type=hidden],[hidden]):not(:has(~:not(input[type=hidden],[hidden]))),` +
			`[data-verso-section]>.verso-rhythm>[data-verso-when]:not([hidden]):not(:has(~:not(input[type=hidden],[hidden])))>:not(input[type=hidden],[hidden]):not(:has(~:not(input[type=hidden],[hidden]))){padding-bottom:0}`,
		`.verso-rhythm>:not(input[type=hidden]):not(:not(input[type=hidden])~*){margin-top:0}`,
	} {
		if !strings.Contains(css, want) {
			t.Errorf("stylesheet is missing %s", want)
		}
	}
}

// TestMastheadEndsUnderLogOut: whatever measure a page's column keeps, its
// masthead — the title's line and the page's acts — ends at the right edge
// Log out ends at: the page's 2.5rem gutter in, never past the top bar's
// 72rem.
func TestMastheadEndsUnderLogOut(t *testing.T) {
	css, err := os.ReadFile("assets/verso.css")
	if err != nil {
		t.Fatalf("read stylesheet: %v", err)
	}
	if !strings.Contains(string(css), "main [data-verso-masthead]{margin-right:calc(100% - min(100cqw - var(--spacing) * 20, var(--container-6xl)))}") {
		t.Error("the masthead ends where Log out does")
	}
}

// TestStylesheetKeepsFocusAndStillness: two things no utility on any one
// element can promise, so the stylesheet states them once for all. In forced
// colours (Windows high contrast) a border's colour is overridden, so a control
// that marks focus only by recolouring its border would show none; there,
// whatever holds keyboard focus is outlined in the system's own colour. And a
// reader who asked for less motion gets dialogs and drawers that appear rather
// than slide or scale in.
func TestStylesheetKeepsFocusAndStillness(t *testing.T) {
	css, err := os.ReadFile("assets/verso.css")
	if err != nil {
		t.Fatalf("read stylesheet: %v", err)
	}
	for _, want := range []string{
		"@media (forced-colors:active){:focus-visible{",
		"outline:2px solid canvastext!important",
		`@media (prefers-reduced-motion:reduce){[role=dialog],[role=alertdialog]{transition-duration:0s!important`,
	} {
		if !strings.Contains(string(css), want) {
			t.Errorf("stylesheet is missing %q", want)
		}
	}
}
