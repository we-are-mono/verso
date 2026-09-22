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
