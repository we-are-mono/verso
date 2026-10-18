// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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
		return strings.HasPrefix(src, "/assets/verso-") && src != "/assets/verso-dev.js"
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
		if !strings.Contains(mark, `class="verso-waiting" data-verso-wait aria-hidden="true"`) || strings.Count(mark, "verso-waiting__square") != 4 {
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
	build := js[strings.Index(js, "function build(ev)"):]
	build = build[:strings.Index(build, "\n    }\n")]
	if !strings.Contains(build, `el("div", "verso-console__event")`) {
		t.Errorf("a console line is not the console block's event:\n%s", build)
	}
	raw, err := os.ReadFile("assets/blocks/console.css")
	if err != nil {
		t.Fatal(err)
	}
	css := string(raw)
	event := css[strings.Index(css, ".verso-console__event {"):]
	event = event[:strings.Index(event, "}")]
	if !strings.Contains(event, "grid-template-columns: subgrid") || !strings.Contains(event, "grid-column: 1 / -1") {
		t.Errorf("a console line is not a row of the console's grid:\n%s", event)
	}
	// No column of an event sizes itself; the grid gives each its measure.
	for _, part := range []string{"__time", "__verdict", "__path", "__proto", "__rule"} {
		at := strings.Index(css, ".verso-console"+part+" {")
		if at < 0 {
			continue
		}
		rule := css[at : at+strings.Index(css[at:], "}")]
		if strings.Contains(rule, "width:") && !strings.Contains(rule, "min-width") {
			t.Errorf("a console column still sizes itself:\n%s", rule)
		}
	}
	// Nothing on a line is cut short, as nothing on the system log's is: an
	// address shows whole, and a rule's name wraps rather than ending in an
	// ellipsis.
	for _, part := range []string{"__event", "__path", "__addr", "__rule", "__rule-name"} {
		at := strings.Index(css, ".verso-console"+part+" {")
		if at < 0 {
			continue
		}
		if rule := css[at : at+strings.Index(css[at:], "}")]; strings.Contains(rule, "text-overflow") || strings.Contains(rule, "nowrap") {
			t.Errorf("a console line cuts a value short:\n%s", rule)
		}
	}
	if name := css[strings.Index(css, ".verso-console__rule-name {"):]; !strings.Contains(name[:strings.Index(name, "}")], "overflow-wrap: break-word") {
		t.Errorf("a rule's name does not wrap:\n%s", name)
	}
	// An address that the window cannot hold whole wraps at its own
	// separators — a break after each ":" or "." — never mid-group, and a
	// copy of it is still the plain string.
	if !strings.Contains(js, `document.createElement("wbr")`) {
		t.Errorf("an address cannot wrap at its separators:\n%s", build)
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
	// the inset: a pixel short of 24px after each side border, so the body
	// is a multiple of 8px and a pixel wide and a box across it has its
	// borders on lines; it is the theme's one token
	if !strings.Contains(cssBlock(t, readAsset(t, "assets/input.css"), "@theme"), "--spacing-side: calc(1.5rem - 1px);") {
		t.Error("the theme does not name a frame's side inset: missing --spacing-side: calc(1.5rem - 1px);")
	}
	css := blockSheet(t, "card")
	for head, wants := range map[string][]string{
		// the frame alone: the reach of its rules matches the inset, and it
		// is the size container what it holds measures against
		"\n.verso-card {": {
			"--verso-reach-start: var(--spacing-side);",
			"--verso-reach-end: var(--spacing-side);",
			"container: card / inline-size;",
			"border: 1px solid var(--color-rule-strong);",
		},
		// the header: 11px after the frame, its 24px line, 12px under, then
		// the body's seam in the frame's own ink, the title centred on the
		// header's middle line
		"\n.verso-card__head {": {
			"padding: calc(var(--spacing) * 2.75) var(--verso-head-end, var(--spacing-side)) calc(var(--spacing) * 3) var(--verso-head-start, var(--spacing-side));",
		},
		// the body ends on a line, so the full 24px under it, its words the
		// inset in from each side; it draws the seam under the head
		"\n.verso-card__body {": {
			"padding: 0 var(--spacing-side) var(--spacing-inset);",
			"border: 0 solid var(--color-rule-strong);",
			"border-top-width: 1px;",
		},
		// with no head it stands straight in the frame, a pixel short of
		// 24px after it, so its lines fill whole bands of the grid
		"\n.verso-card__body:is(.verso-card[data-verso-headless] > *) {": {
			"padding-top: calc(var(--spacing-inset) - 1px);",
			"border-top-width: 0;",
		},
		// the air between cards, less the frame's pixel so the next frame
		// lands on a line
		"\n.verso-card:is(:not(input[type=\"hidden\"], template, script, [hidden]) ~ *) {": {"margin-top: calc(var(--spacing-inset) - 1px);"},
	} {
		block := cssBlock(t, css, head)
		for _, want := range wants {
			if !strings.Contains(block, want) {
				t.Errorf("a card does not measure on the 8px grid; blocks/card.css %s lacks %s", strings.TrimSpace(head), want)
			}
		}
	}
	// The frame is the frame only: the head and the body keep the air, and
	// the head draws no seam of its own beside the body's.
	frame := cssBlock(t, css, "\n.verso-card {")
	if strings.Contains(frame, "padding") {
		t.Errorf("the card's frame keeps air of its own: %s", frame)
	}
	if head := cssBlock(t, css, "\n.verso-card__head {"); strings.Contains(head, "border") || strings.Contains(head, "margin") {
		t.Errorf("the card's head draws a seam or reaches past the frame: %s", head)
	}
	// the column-head row: its 16px line 15px after the edge over it, 16px
	// over its own hairline
	if !strings.Contains(cssBlock(t, blockSheet(t, "table"), ".verso-table__head:where(.verso-listing[data-verso-place] > .verso-listing__scroller > .verso-table > thead > tr > *) {"),
		"padding-block: calc(var(--spacing) * 4 - 1px) calc(var(--spacing) * 4);") {
		t.Error("a card's column-head row does not measure 48px with its hairlines")
	}
	// No sheet keeps a side inset of its own beside the theme's.
	sheets, err := filepath.Glob("assets/blocks/*.css")
	if err != nil {
		t.Fatal(err)
	}
	for _, sheet := range append(sheets, "assets/input.css") {
		if strings.Contains(readAsset(t, sheet), "--verso-side") {
			t.Errorf("%s keeps a side inset of its own (--verso-side)", sheet)
		}
	}
}

// readAsset is the source of the asset at path.
func readAsset(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// TestACardPullsItsLastRowsAir: a card's last row gives back what the card
// asks under itself, so the card closes 24px under the row's answer as it
// opens; no row ends on a sentence under its answer.
func TestACardPullsItsLastRowsAir(t *testing.T) {
	// The card hands its last block the 16px pull, and a row takes it as
	// what it gives back under itself.
	trail := cssBlock(t, blockSheet(t, "card"), "\n.verso-card__body > :nth-last-child(1 of :not(input[type=\"hidden\"], [hidden], template)) {")
	if !strings.Contains(trail, "--verso-edge-trail: calc(var(--spacing) * -2);") {
		t.Errorf("a card does not pull its last row's air: %s", trail)
	}
	field := blockSheet(t, "field")
	if !strings.Contains(cssBlock(t, field, "\n.verso-field {"), "margin-bottom: var(--verso-edge-trail, 0px);") {
		t.Error("a row does not give back what its surface asks under itself")
	}
	// A row's sentences stand under its name, so no row ends on one under
	// its answer and none is spared the pull.
	if strings.Contains(field, "data-verso-field-note") {
		t.Error("the field block still styles a sentence under the answer")
	}
}

// TestNoRuleAimsAtThePanel: where a panel stands is the row that holds it to
// say (flush left over the Overview's port listing, the figure's column on
// the Hardware page), never a card rule aimed at the panel's own classes.
func TestNoRuleAimsAtThePanel(t *testing.T) {
	for _, block := range []string{"card", "section", "heading"} {
		for _, line := range strings.Split(blockSheet(t, block), "\n") {
			if strings.Contains(line, "verso-panel") || strings.Contains(line, "verso-ports") {
				t.Errorf("blocks/%s.css places a panel: %s", block, line)
			}
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

// TestAFieldRowMeasuresOnTheEightPixelGrid: a form's answers stand in one
// 272px column (form-fields), and a field row writes its name on a 24px band
// 15px after the hairline above it and 16px before the next edge, so a row of
// one line is 56px with its hairline (layout-grid).
func TestAFieldRowMeasuresOnTheEightPixelGrid(t *testing.T) {
	raw, err := os.ReadFile("assets/input.css")
	if err != nil {
		t.Fatal(err)
	}
	css := string(raw)
	if !strings.Contains(css, "--spacing-answer: 17rem;") {
		t.Error("the theme does not name the answer column: missing --spacing-answer: 17rem;")
	}
	field := blockSheet(t, "field")
	for _, want := range []string{
		"--verso-field-lead: calc(var(--spacing) * 3.75);",
		"--verso-field-trail: calc(var(--spacing) * 4);",
	} {
		if !strings.Contains(field, want) {
			t.Errorf("a field row does not put its name on its band: blocks/field.css is missing %s", want)
		}
	}
}

// TestAFieldRowIsALedger: a row is its question and its answer side by side,
// the answer in the form's one column, a lane after it where a row can be
// taken out (form-fields). Rows that follow each other meet on a hairline, 16px
// under the one's last line and 15px over the next one's name, and the answer
// centres a 32px control on the name's band, reaching 4px past it either side.
func TestAFieldRowIsALedger(t *testing.T) {
	css := blockSheet(t, "field")
	ledger := cssBlock(t, css, "\n.verso-field__ledger {")
	for _, want := range []string{
		"display: grid;",
		"grid-template-columns: minmax(0, 1fr) var(--spacing-answer);",
		"column-gap: var(--spacing-inset);",
	} {
		if !strings.Contains(ledger, want) {
			t.Errorf("the ledger lacks %s: %s", want, ledger)
		}
	}
	removable := cssBlock(t, css, "\n.verso-field__ledger:has(> .verso-field__remove) {")
	if !strings.Contains(removable, "grid-template-columns: minmax(0, 1fr) var(--spacing-answer) var(--spacing-act);") {
		t.Errorf("a removable row's remove is not the ledger's third column: %s", removable)
	}
	// A row's air and hairline are its knobs, read once by the row itself.
	row := cssBlock(t, css, "\n.verso-field {")
	for _, want := range []string{
		"padding: var(--verso-edge-air, var(--verso-field-lead, var(--spacing-inset))) max(",
		"var(--verso-field-trail, 0px) var(--verso-field-bleed, 0px);",
		"border-top-width: var(--verso-field-rule, 0px);",
	} {
		if !strings.Contains(row, want) {
			t.Errorf("a row does not read its air from its knobs: missing %s in %s", want, row)
		}
	}
	between := cssBlock(t, css, "\n.verso-field:is(.verso-field + *):not(:where(.verso-form[data-verso-questions=\"short\"] *)),")
	for _, want := range []string{"--verso-field-rule: 1px;", "--verso-field-lead: calc(var(--spacing) * 3.75);"} {
		if !strings.Contains(between, want) {
			t.Errorf("rows do not meet on a hairline 15px over the name: missing %s in %s", want, between)
		}
	}
	before := cssBlock(t, css, "\n.verso-field:has(+ :is(.verso-field, .verso-conditional, .verso-field-set, [data-verso-rows])):not(:where(.verso-form[data-verso-questions=\"short\"] *)) {")
	if !strings.Contains(before, "--verso-field-trail: calc(var(--spacing) * 4);") {
		t.Errorf("a row followed by a row keeps 16px under its last line: %s", before)
	}
	if answer := cssBlock(t, css, "\n.verso-field__answer {"); !strings.Contains(answer, "grid-column: 2;") {
		t.Errorf("the answer is not the ledger's second column: %s", answer)
	}
	// The answer reaches 4px past the name's band only beside it; stacked,
	// it stands straight under the question with nothing pulled.
	beside := strings.Index(css, "@container verso-field (width >= 36rem) {")
	stacked := strings.Index(css, "@container verso-field (width < 36rem) {")
	if beside < 0 || stacked < beside {
		t.Fatal("the answer's pull over the name's band is not kept to a row standing side by side")
	}
	for _, want := range []string{"margin-top: calc(var(--spacing) * -1);", "margin-bottom: calc(var(--spacing) * -1);"} {
		if !strings.Contains(css[beside:stacked], want) {
			t.Errorf("the answer does not centre on the name's band beside it: missing %s", want)
		}
		if strings.Contains(cssBlock(t, css, "\n.verso-field__answer {"), want) {
			t.Errorf("a stacked answer is still pulled (%s)", want)
		}
	}
	for _, gone := range []string{".verso-field-toggle", ".verso-field-with-remove"} {
		if strings.Contains(css, gone) {
			t.Errorf("the field block still lays out %s", gone)
		}
	}
}

// TestAMeasureIsAStackedRows: a control's own measure (a secret's 384px, an
// endpoint's 512px, a prefix's, a dropdown sized to its longest option) holds
// only where a row stacks and the control has the row to itself; in the
// answer column every control is the column's (form-fields). Nothing measures
// a field as half its row any more.
func TestAMeasureIsAStackedRows(t *testing.T) {
	measured := map[string]bool{}
	// The field block holds the box measures; the input and control blocks
	// let a dropdown measure its longest option.
	for _, name := range []string{"field", "input", "control"} {
		css := blockSheet(t, name)
		stacked := strings.Index(css, "@container verso-field (width < 36rem)")
		for _, measure := range []string{`"secret"`, `"prefix"`, `"endpoint"`, `"choice"`} {
			for at := strings.Index(css, "[data-verso-control-measure="+measure+"]"); at >= 0; {
				measured[measure] = true
				if stacked < 0 || at < stacked {
					t.Errorf("blocks/%s.css: the %s measure applies outside a stacked row", name, measure)
				}
				next := strings.Index(css[at+1:], "[data-verso-control-measure="+measure+"]")
				if next < 0 {
					break
				}
				at += next + 1
			}
		}
		if strings.Contains(css, "--verso-field-width-host") || strings.Contains(css, `[data-verso-control-measure="host"]`) {
			t.Errorf("blocks/%s.css: a field is still measured as half its row", name)
		}
	}
	for _, measure := range []string{`"secret"`, `"prefix"`, `"endpoint"`, `"choice"`} {
		if !measured[measure] {
			t.Errorf("no block measures a stacked %s control", measure)
		}
	}
}

// TestARevealedFieldIsARowLikeAnyOther: what a switch reveals is rows like
// any other: the first meets the gate on a hairline 15px over its name, a gate
// inside the reveal among them, and no question indents, so every question
// keeps the form's one left edge as every answer keeps its one column
// (form-fields).
func TestARevealedFieldIsARowLikeAnyOther(t *testing.T) {
	css := blockSheet(t, "field")
	revealed := cssBlock(t, css, `.verso-field:is(:is(.verso-conditional__body, .verso-conditional__otherwise) > :nth-child(1 of :not(input[type="hidden"], [hidden], template))),`)
	for _, want := range []string{"--verso-field-rule: 1px;", "--verso-field-lead: calc(var(--spacing) * 3.75);"} {
		if !strings.Contains(revealed, want) {
			t.Errorf("a revealed row does not meet its gate on a hairline (%s): %s", want, revealed)
		}
	}
	for _, name := range []string{"field", "conditional"} {
		if regexp.MustCompile(`verso-conditional__(body|otherwise)[^{]*verso-field__question`).MatchString(blockSheet(t, name)) {
			t.Errorf("blocks/%s.css: a revealed field's question still indents off the form's left edge", name)
		}
	}
}

// TestAFactsRowsKeepTheLedgersStep: a settings list's facts are ledger rows,
// so none keeps a rule of its own; the row before a fold keeps a row's 16px
// over the fold's hairline.
func TestAFactsRowsKeepTheLedgersStep(t *testing.T) {
	sheets, err := filepath.Glob("assets/blocks/*.css")
	if err != nil {
		t.Fatal(err)
	}
	for _, sheet := range sheets {
		raw, err := os.ReadFile(sheet)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(raw), "data-verso-settings-row") {
			t.Errorf("%s: a fact's row keeps a rule of its own", sheet)
		}
	}
	css := blockSheet(t, "field")
	for head, want := range map[string]string{
		".verso-field:is(.verso-settings > *):has(+ .verso-settings__fold) {": "--verso-field-trail: calc(var(--spacing) * 4);",
	} {
		if !strings.Contains(cssBlock(t, css, head), want) {
			t.Errorf("%s wants %s", head, want)
		}
	}
}

// TestATableAmongFieldsKeepsItsActOnTheLine: a set among a form's fields is a
// row whose act stands on the question's line at the row's end at every
// width, the row's two columns the question and the act; its rows start on a
// line 8px under the question's words, and the last draws no line of its own
// over the hairline between questions (form-fields).
func TestATableAmongFieldsKeepsItsActOnTheLine(t *testing.T) {
	css := blockSheet(t, "field")
	ledger := ".verso-field__ledger:has(> .verso-field__act) {"
	if n := strings.Count(css, ledger); n != 2 {
		t.Errorf("an act's ledger must hold its columns wide and stacked, got %d rules", n)
	}
	if !strings.Contains(cssBlock(t, css, ledger), "grid-template-columns: minmax(0, 1fr) auto;") {
		t.Error("an act's ledger is not the question and the act")
	}
	if !strings.Contains(cssBlock(t, css, "\n.verso-field__act {"), "grid-row: 1;") {
		t.Error("an act does not stand on the question's line")
	}
	set := cssBlock(t, blockSheet(t, "listing"), ".verso-listing[data-verso-in-form] {")
	if !strings.Contains(set, "border-top-width: 1px;") {
		t.Errorf("a set among the fields does not open on its own hairline: %s", set)
	}
	if !strings.Contains(set, "margin-top: calc(var(--spacing) * 1.75);") {
		t.Error("a set's first line does not stand 8px under its question's words")
	}
	if !strings.Contains(set, "margin-bottom: calc(var(--spacing) * -2.25);") {
		t.Error("a set's last row keeps the 9px under its words, so the next question leaves the grid")
	}
	if !strings.Contains(cssBlock(t, blockSheet(t, "table"), ":where([data-verso-in-form] tbody > tr:last-child > *) {"), "border-bottom-color: transparent;") {
		t.Error("a set's last row draws a line of its own over the hairline between questions")
	}
	if !strings.Contains(cssBlock(t, blockSheet(t, "slot"), ".verso-slot:is(.verso-field__act ~ .verso-field__answer > *) {"), "margin-top: calc(var(--spacing) * 2);") {
		t.Error("an empty set's slot does not stand 8px under its question")
	}
}

// TestAChooserKeepsItsOwnLayout: a choice of what a page shows reads as a
// sentence, its name then its controls ("Show chart for [CPU ▾] over …"),
// wrapping under the name where the line runs out, not a question and an
// answer; an inline or search form's field stands as it always has, a label
// over its box.
func TestAChooserKeepsItsOwnLayout(t *testing.T) {
	css := blockSheet(t, "field")
	sentence := cssBlock(t, css, ".verso-field__ledger:is(.verso-field[data-verso-plain] > *) {")
	for _, want := range []string{"display: flex;", "flex-wrap: wrap;", "align-items: center;", "gap: calc(var(--spacing) * 2) calc(var(--spacing) * 3);"} {
		if !strings.Contains(sentence, want) {
			t.Errorf("a chooser does not read as a sentence: lacks %s in %s", want, sentence)
		}
	}
	if !strings.Contains(cssBlock(t, css, ".verso-field__answer:is(.verso-field[data-verso-plain] > * > *) {"), "width: auto;") {
		t.Error("a chooser's controls take the row's width instead of standing after its name")
	}
	// An inline or search form is a toolbar, not a list of settings.
	if !strings.Contains(cssBlock(t, css, ".verso-field__ledger:is([data-verso-compact-form] *) {"), "display: block;") {
		t.Error("a compact form's field is laid out as a ledger")
	}
	if !strings.Contains(cssBlock(t, css, ".verso-field__answer:is(.verso-field[data-verso-plain] > * > *, [data-verso-compact-form] *) {"), "margin-block: 0;") {
		t.Error("a chooser's or a compact form's controls are pulled over its name's band")
	}
	// A compact form's field keeps no row's air or hairline.
	compact := cssBlock(t, css, ".verso-field:is([data-verso-compact-form] *) {")
	for _, want := range []string{"--verso-field-rule: 0px;", "--verso-field-lead: 0px;", "--verso-field-trail: 0px;"} {
		if !strings.Contains(compact, want) {
			t.Errorf("a compact form's field is a ledger row: lacks %s in %s", want, compact)
		}
	}
}

// TestHelpOpensOnRequest: every description stands folded to nothing until
// the root shows help, then opens over 200ms, its one grid track going from
// none to its words' height as it fades in, and at once where motion is
// reduced. The help act says the words for what pressing it does,
// and stands only where there is help to show, in its page's body or its
// drawer, on the heading line ending where the form's cards do
// (form-fields).
func TestHelpOpensOnRequest(t *testing.T) {
	css := blockSheet(t, "field")
	for head, wants := range map[string][]string{
		"\n.verso-field__desc {":                                  {"display: grid;", "grid-template-rows: 0fr;", "opacity: 0;", "visibility: hidden;", "200ms", "var(--verso-help-delay, 0ms)"},
		"\n.verso-field__desc > span {":                           {"min-height: 0;", "overflow: hidden;"},
		`.verso-field__desc:where([data-verso-help="shown"] *) {`: {"grid-template-rows: 1fr;", "opacity: 1;", "visibility: visible;"},
	} {
		block := cssBlock(t, css, head)
		for _, want := range wants {
			if !strings.Contains(block, want) {
				t.Errorf("%s lacks %s: %s", head, want, block)
			}
		}
	}
	help := blockSheet(t, "help")
	for head, wants := range map[string][]string{
		".verso-help {": {"display: none;"},
		".verso-help:is(.verso-page__column:has(> .verso-page__body [data-verso-field-desc]) > .verso-page__masthead *, dialog:has([data-verso-field-desc]) *) {": {"display: flex;"},
		`.verso-help__show:where([data-verso-help="shown"] *) {`:                                                               {"display: none;"},
		`.verso-help__hide:where([data-verso-help="shown"] *) {`:                                                               {"display: inline;"},
		".verso-help:where(.verso-page__column:has(> .verso-page__body [data-verso-form-column]) > .verso-page__masthead *) {": {"min(var(--verso-column), var(--verso-form-width))"},
	} {
		block := cssBlock(t, help, head)
		for _, want := range wants {
			if !strings.Contains(block, want) {
				t.Errorf("%s lacks %s: %s", head, want, block)
			}
		}
	}
	motion := strings.Index(css, "@media (prefers-reduced-motion: reduce)")
	if motion < 0 || !strings.Contains(css[motion:], "{\n  .verso-field__desc {\n    transition: none;\n  }") {
		t.Error("descriptions still glide where motion is reduced")
	}
}

// TestACardsRowsKeepARowsAirAtItsEdges: a card whose body opens on a row
// stands that row's name 15px after the header's seam (or the headless
// frame), as a name stands after the hairline between rows, and a card whose
// body ends on a row closes 16px under its last words, as a row stands over
// the next one's hairline: the card's own 24px is a block's (form-fields).
func TestACardsRowsKeepARowsAirAtItsEdges(t *testing.T) {
	css := blockSheet(t, "card")
	// A body that opens on a row, or on a block whose first visible child is
	// one.
	const opensOnARow = ` > :nth-child(1 of :not(input[type="hidden"], [hidden], template)):is(.verso-field, :is(.verso-field-set, .verso-conditional, .verso-settings, [data-verso-rows], .verso-form:not([data-verso-compact-form])):has(> :nth-child(1 of :not(input[type="hidden"], [hidden], template)).verso-field)) {`
	for head, want := range map[string]string{
		// under a header, straight on the seam, the row's own 24px of air
		// pulled 9px back stands its name 15px after the seam's pixel
		".verso-card__body:not(.verso-card[data-verso-headless] > *)" + opensOnARow: "--verso-edge-lead: calc(var(--spacing) * -2 - 1px);",
		// with no head the row's air is the body's own 23px after the frame,
		// pulled 8px back
		".verso-card__body:is(.verso-card[data-verso-headless] > *)" + opensOnARow: "--verso-edge-lead: calc(var(--spacing) * -2);",
		// a body that ends on a row closes 16px under its last words
		".verso-card__body > :nth-last-child(1 of :not(input[type=\"hidden\"], [hidden], template)) {": "--verso-edge-trail: calc(var(--spacing) * -2);",
	} {
		if !strings.Contains(cssBlock(t, css, head), want) {
			t.Errorf("blocks/card.css %s lacks %q", head, want)
		}
	}
	// The rhythm stands its first block by the lead its surface hands it,
	// and a row gives back the trail under itself.
	if !strings.Contains(cssBlock(t, blockSheet(t, "rhythm"), `.verso-rhythm > :nth-child(1 of :not(input[type="hidden"], [hidden], template)) {`), "margin-top: var(--verso-edge-lead, 0px);") {
		t.Error("the rhythm does not read the lead its surface hands its first block")
	}
	if !strings.Contains(cssBlock(t, blockSheet(t, "field"), "\n.verso-field {"), "margin-bottom: var(--verso-edge-trail, 0px);") {
		t.Error("a row does not give back the trail its surface hands it")
	}
	// The blocks a row stands in forward the edges to it.
	for _, block := range []string{"form", "settings"} {
		sheet := blockSheet(t, block)
		if !strings.Contains(sheet, "--verso-edge-trail: inherit;") {
			t.Errorf("blocks/%s.css does not forward the trail to its last row", block)
		}
	}
}

// TestAStagedRowIsMarkedDownItsEdge: a row whose change waits wears a 3px
// bar in the stage's marigold down its left edge, from line to line: on its
// card's border, where rows run to it, on a page and in a drawer alike.
func TestAStagedRowIsMarkedDownItsEdge(t *testing.T) {
	raw, err := os.ReadFile("assets/blocks/staged.css")
	if err != nil {
		t.Fatal(err)
	}
	css := string(raw)
	bar := cssBlock(t, css, "\n.verso-staged__bar {")
	for _, want := range []string{"position: absolute;", "inset-block: 0;", "left: 0;", "width: calc(var(--spacing) * 0.75);", "background: var(--color-marigold);"} {
		if !strings.Contains(bar, want) {
			t.Errorf("the staged bar lacks %s: %s", want, bar)
		}
	}
	// One bar wherever the row stands: no sheet places a drawer's apart.
	for _, sheet := range []string{"assets/blocks/staged.css", "assets/blocks/field.css", "assets/input.css"} {
		raw, err := os.ReadFile(sheet)
		if err != nil {
			t.Fatal(err)
		}
		if regexp.MustCompile(`dialog[^{]*\.verso-staged`).Match(raw) {
			t.Errorf("%s: a drawer's staged bar stands off its card's border", sheet)
		}
	}
}

// TestWhatFollowsAControlStandsOnALine: a refusal stands 8px under its
// control's box wherever the answer stands on the grid (stacked, wide, a bare
// control), and 4px under it only beside the question, where the answer is
// pulled 4px over the name's band, so it always starts on a line.
func TestWhatFollowsAControlStandsOnALine(t *testing.T) {
	css := blockSheet(t, "field")
	after := ":is([data-verso-error], [data-verso-inline-error]) {"
	if !strings.Contains(cssBlock(t, css, "\n.verso-field__answer > "+after), "margin-top: calc(var(--spacing) * 2);") {
		t.Error("what follows a control does not stand 8px under it")
	}
	beside := strings.Index(css, "@container verso-field (width >= 36rem) {")
	pulled := ".verso-field__answer:is(.verso-field:not([data-verso-wide], [data-verso-plain]) > .verso-field__ledger:has(> .verso-field__question > *) > *) > " + after
	at := strings.Index(css, pulled)
	if beside < 0 || at < beside || !strings.Contains(cssBlock(t, css, pulled), "margin-top: var(--spacing);") {
		t.Error("beside its question, what follows a pulled control does not give the pull back")
	}
}

// TestASwitchShowsTheWordForItsState: a form switch holds both words and the
// stylesheet shows the one its box is in, with no script (form-fields).
func TestASwitchShowsTheWordForItsState(t *testing.T) {
	want := ".verso-choice__on:where(.verso-choice__row:not(:has(.verso-choice__input:checked)) *),\n.verso-choice__off:where(.verso-choice__row:has(.verso-choice__input:checked) *) {\n  display: none;\n}"
	if !strings.Contains(blockSheet(t, "choice"), want) {
		t.Errorf("the choice block does not show a switch's word for its state:\n%s", want)
	}
}

// TestAChipVariesOnlyInItsColour: a chip is one box wherever it stands, so a
// reader never reads two sizes as two kinds of thing. A variant chooses the
// chip's colour (its tone's edge, ground, words and glyph); its face comes
// from the type role's verbatim form and its glyph from the glyph element.
// No variant writes the box: no margin, padding, height or border width.
func TestAChipVariesOnlyInItsColour(t *testing.T) {
	css := blockSheet(t, "chip")
	for _, rule := range strings.Split(css, "}") {
		open := strings.Index(rule, "{")
		if open < 0 {
			continue
		}
		sel, body := rule[:open], rule[open+1:]
		if !strings.Contains(sel, "[data-verso-") {
			continue
		}
		for _, decl := range strings.Split(body, ";") {
			prop, _, ok := strings.Cut(strings.TrimSpace(decl), ":")
			if !ok {
				continue
			}
			prop = strings.TrimSpace(prop)
			if !strings.HasPrefix(prop, "--verso-chip-") && prop != "color" {
				t.Errorf("a chip variant writes %q (%s): a variant chooses only the chip's colour", prop, strings.TrimSpace(sel))
			}
		}
	}
}

// TestFieldRowsRunToACardsBorders: in a page card a field row runs from the
// card's left border to its right, its words still the card's 23px in, so the
// hairlines between rows touch both borders (form-fields), on a page and in a
// drawer alike. A row inside another row or a field set's cell stays in its
// column, and a choice of what the card shows is no row and keeps to the
// inset.
func TestFieldRowsRunToACardsBorders(t *testing.T) {
	block := cssBlock(t, blockSheet(t, "card"), ".verso-card__body .verso-field:not(.verso-field .verso-field, .verso-form-grid .verso-field, [data-verso-plain]),\n.verso-card__body .verso-settings__fold,\n.verso-card__body .verso-form__acts {")
	if !strings.Contains(block, "--verso-field-bleed: var(--spacing-side);") {
		t.Errorf("a card's field row does not run to its borders: %s", block)
	}
	// A form's acts under its rows are the ledger's last row: their hairline
	// runs by the same bleed, and the buttons stand 24px under it as the
	// card's edge stands 24px under them, so ruled air holds them alike on
	// both sides. The row over them keeps a row's 16px.
	acts := cssBlock(t, blockSheet(t, "form"), `.verso-form__acts[data-verso-close="flush"]:is(:is(.verso-field, .verso-conditional, .verso-field-set, [data-verso-rows]) + *):not(:where(.verso-form[data-verso-questions="short"] *)) {`)
	for _, want := range []string{
		"margin-inline: calc(var(--verso-field-bleed, 0px) * -1);",
		"padding: calc(var(--spacing) * 5.75) var(--verso-field-bleed, 0px) 0;",
		"border-top-width: 1px;",
	} {
		if !strings.Contains(acts, want) {
			t.Errorf("a form's acts under its rows do not meet them on a hairline: missing %s in %s", want, acts)
		}
	}
	if !strings.Contains(blockSheet(t, "field"), `.verso-field:has(+ .verso-form__acts[data-verso-close="flush"]):not(:where(.verso-form[data-verso-questions="short"] *)),`) {
		t.Error("the row over a form's acts does not keep a row's 16px over their hairline")
	}
	// The row reads the bleed: out past its column by it, its words back in.
	// It spans what it stands in, so its hairline reaches the far border of
	// a card wider than the form's measure; the padding at its end takes up
	// what the measure leaves, so its words keep it.
	row := cssBlock(t, blockSheet(t, "field"), "\n.verso-field {")
	for _, want := range []string{
		"margin-inline: calc(var(--verso-field-bleed, 0px) * -1);",
		"max(var(--verso-field-bleed, 0px), calc(100% - var(--verso-form-width, 40rem) + var(--verso-field-bleed, 0px) * 3)) var(--verso-field-trail, 0px) var(--verso-field-bleed, 0px);",
	} {
		if !strings.Contains(row, want) {
			t.Errorf("a row does not run by its bleed: missing %s in %s", want, row)
		}
	}
	if strings.Contains(row, "max-width") {
		t.Errorf("a row is capped, so its hairline stops short of a wide card's border: %s", row)
	}
}

// cssBlock is the body of the first rule that opens with head, up to its
// first closing brace: enough for a rule that nests nothing.
func cssBlock(t *testing.T, css, head string) string {
	t.Helper()
	at := strings.Index(css, head)
	if at < 0 {
		t.Fatalf("no rule %q", head)
	}
	end := strings.Index(css[at:], "}")
	if end < 0 {
		t.Fatalf("rule %q does not close", head)
	}
	return css[at : at+end]
}

// TestAPanelMeasuresOnTheEightPixelGrid: a router's panel is drawn at its own
// size where it fits, so every edge it draws stands on a line, and where it
// does not it shrinks to a whole number of 8px steps tall, its width following
// its proportions and its left edge the content's (device-panel).
func TestAPanelMeasuresOnTheEightPixelGrid(t *testing.T) {
	raw, err := os.ReadFile("assets/blocks/panel.css")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		".verso-panel {\n  position: relative;\n  container-type: inline-size;\n}",
		".verso-panel__face svg {\n  display: block;\n  width: auto;\n  max-width: none;\n  height: min(var(--art-h), round(down, 100cqi * var(--art-aspect), 0.5rem));\n}",
	} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("the panel does not measure on the 8px grid: blocks/panel.css is missing %s", want)
		}
	}
	// No panel rule sizes the art any other way.
	if strings.Contains(string(raw), "width: 100%") || strings.Contains(string(raw), "height: auto") {
		t.Error("the panel's art is still stretched to its column's width, off the grid")
	}
}

// TestReducedMotionStillsTheTrafficLED: under reduced motion a busy port's
// traffic LED stays lit and still (device-panel). The rule that stops it must
// match the rule that starts it selector for selector, or the blink, being
// more specific, wins and keeps blinking.
func TestReducedMotionStillsTheTrafficLED(t *testing.T) {
	raw, err := os.ReadFile("assets/blocks/panel.css")
	if err != nil {
		t.Fatal(err)
	}
	css := string(raw)
	const blinking = `.verso-panel [data-verso-port][data-verso-activity]:not([data-verso-activity="0"]) .led-traffic`
	if !strings.Contains(css, "\n"+blinking+" {\n  fill:") {
		t.Fatalf("blocks/panel.css no longer starts the blink with %s", blinking)
	}
	if !strings.Contains(css, "@media (prefers-reduced-motion: reduce) {\n  "+blinking+" {\n    animation: none;\n  }") {
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
	// Over a drawer's body, the modal block draws it, scrolling with what the
	// drawer holds.
	if !strings.Contains(string(css), ".verso-modal__ground:where([data-verso-grid] *){") {
		t.Error("the dev grid is not drawn; the compiled stylesheet does not carry the drawer's grid")
	}
	drawer := cssBlock(t, blockSheet(t, "modal"), "\n.verso-modal__ground:where([data-verso-grid] *) {")
	for _, want := range []string{
		"background-size: calc(var(--spacing) * 2) calc(var(--spacing) * 2);",
		"background-attachment: local;",
	} {
		if !strings.Contains(drawer, want) {
			t.Errorf("the dev grid is not drawn; blocks/modal.css's drawer grid is missing %s", want)
		}
	}
	// Over the page, the page block draws it from the bar's hairline and the
	// rail's border, never taking a press.
	sheet := blockSheet(t, "page")
	if !strings.Contains(cssBlock(t, sheet, "\n.verso-page__main:where([data-verso-grid] *) {"), "position: relative;") {
		t.Error("the dev grid is not drawn; the page's room does not hold it")
	}
	grid := cssBlock(t, sheet, "\n.verso-page__main:where([data-verso-grid] *)::after {")
	for _, want := range []string{
		"pointer-events: none;",
		"inset: 0;",
		"background-size: calc(var(--spacing) * 2) calc(var(--spacing) * 2);",
	} {
		if !strings.Contains(grid, want) {
			t.Errorf("the dev grid is not drawn; blocks/page.css's grid is missing %s", want)
		}
	}
	if !strings.Contains(string(css), ".verso-page__main:where([data-verso-grid] *)") {
		t.Error("the dev grid is not drawn; the compiled stylesheet does not carry the page block's grid")
	}
}

// TestACardUnderACardDrawsNoSecondDivider: a form that writes more than one
// file shows a card per file, one under the other. Each is a card of its own:
// it stands a cell under the card before it, less that card's frame pixel,
// its own frame on the line, with no rule of its own.
func TestACardUnderACardDrawsNoSecondDivider(t *testing.T) {
	css, err := os.ReadFile("assets/verso.css")
	if err != nil {
		t.Fatalf("read stylesheet: %v", err)
	}
	want := ".verso-code[data-verso-place=card]{--verso-step:calc(var(--spacing-inset) - 1px);margin-top:calc(var(--spacing-inset) - 1px)}"
	if !strings.Contains(string(css), want) {
		t.Errorf("stylesheet is missing %s", want)
	}
	if rule := cssBlock(t, blockSheet(t, "code"), `.verso-code[data-verso-place="card"] {`); strings.Contains(rule, "border") {
		t.Errorf("a configuration card draws a rule over itself: %s", rule)
	}
}

// TestStylesheetTypeSystem: the two faces are the only faces, and each is
// declared across the weights its file actually carries (both are variable),
// so a weight the design asks for (the top bar's light maker line) is drawn
// rather than clamped to the nearest declared one. A kicker is the canvas's
// one kicker wherever it stands (a section's, a takeover's): the label role,
// 12px, 500, uppercase, .08em, in the meta ink.
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
	for _, sel := range []string{".verso-heading__kicker{", ".verso-takeover__kicker{"} {
		kickerAt := strings.Index(css, sel)
		if kickerAt < 0 {
			t.Errorf("no kicker rule %s", sel)
			continue
		}
		kicker := css[kickerAt : kickerAt+strings.Index(css[kickerAt:], "}")]
		for _, want := range []string{"font-size:.75rem", "font-weight:500", "letter-spacing:.08em", "text-transform:uppercase", "color:var(--color-meta)"} {
			if !strings.Contains(kicker, want) {
				t.Errorf("kicker is missing %q: %s", want, kicker)
			}
		}
	}
	if strings.Contains(css, ".verso-kicker{") {
		t.Error("a kicker of its own stands beside the label role's")
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
		".verso-section:is(.verso-page__body>.verso-stack>*):not([data-verso-section=ruled],[data-verso-section=part]){padding-top:0}",
		"margin-top:var(--verso-step,var(--verso-rhythm,calc(var(--spacing) * 10)))",
	} {
		if !strings.Contains(string(css), want) {
			t.Errorf("stylesheet is missing %q", want)
		}
	}
	// The page's own stack stands its blocks 40px apart: the page sets the
	// gap, and the stack steps every block after the first by it.
	if !strings.Contains(cssBlock(t, blockSheet(t, "page"), "\n.verso-page__body > .verso-stack {"), "--verso-stack-gap: calc(var(--spacing) * 10);") {
		t.Error("the page's root stack does not stand its blocks 40px apart")
	}
	stack := blockSheet(t, "stack")
	step := cssBlock(t, stack, "\n.verso-stack > * + * {")
	own := cssBlock(t, stack, "\n.verso-stack {")
	if !strings.Contains(step, "margin-top: var(--verso-step, var(--verso-stack-step));") ||
		!strings.Contains(own, "--verso-stack-step: var(--verso-stack-gap, var(--spacing-inset));") {
		t.Errorf("a stack does not step its blocks by the gap its page sets:\n%s\n%s", own, step)
	}
	// Only the air over a block: none under the band, so its listing joins it.
	if strings.Contains(stack, "margin-bottom") {
		t.Error("a stack sets air under its blocks, so a listing no longer joins its band")
	}
	// the listing joins its band, as a log (and its notice) sits flush on its bar
	if !strings.Contains(cssBlock(t, stack, "\n.verso-stack > .verso-actionbar + *,\n.verso-stack > .verso-console {"), "--verso-step: 0;") {
		t.Error("a listing does not stand flush under its band, or a log on its masthead")
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
	// Every rule reaches by its surface's two measures (blocks/section.css): a
	// page runs 40px out to the rail and none past the column's end; a page
	// card keeps its rules to its own edges; an overlay keeps them to its
	// column.
	for _, want := range []string{
		".verso-page__main{--verso-reach-start:calc(var(--spacing) * 10);--verso-reach-end:0px;",
		".verso-card{--verso-reach-start:var(--spacing-side);--verso-reach-end:var(--spacing-side);",
		".verso-modal{--verso-reach-start:0px;--verso-reach-end:0px;",
		".verso-section[data-verso-section=ruled]{margin-inline:calc(-1 * var(--verso-reach-start,0px)) calc(-1 * var(--verso-reach-end,0px));",
		"padding-inline:var(--verso-reach-start,0px) var(--verso-reach-end,0px);",
		".verso-section__head[data-verso-section-band]{margin-inline:calc(-1 * var(--verso-reach-start,0px)) calc(-1 * var(--verso-reach-end,0px));padding:calc(var(--spacing) * 10) var(--verso-reach-end,0px) 0 var(--verso-reach-start,0px);",
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
	rule := cssBlock(t, blockSheet(t, "section"), `.verso-section:is(.verso-page__body > .verso-stack > *):is([data-verso-section="ruled"], [data-verso-section="part"]) {`)
	if !strings.Contains(rule, "margin-top: calc(var(--spacing) * 10 - 1px);") {
		t.Error("a ruled section on the page's stack does not land its rule on a line")
	}
}

// TestTheOverviewHeadingSpeaksInItsTone: a heading that names what is wrong
// is a message about now, so it reads in its tone's deep step (the masthead
// tone contract); the live stream retones it by its attribute alone.
func TestTheOverviewHeadingSpeaksInItsTone(t *testing.T) {
	css := blockSheet(t, "overview")
	for head, want := range map[string]string{
		`.verso-overview__lead:where(.verso-overview[data-overview-tone="warning"] *) {`: "color: var(--color-marigold-deep);",
		`.verso-overview__lead:where(.verso-overview[data-overview-tone="danger"] *) {`:  "color: var(--color-crimson-deep);",
	} {
		if !strings.Contains(cssBlock(t, css, head), want) {
			t.Errorf("blocks/overview.css %s lacks %s", head, want)
		}
	}
	// The stream retones the root's attribute, which the rules read.
	if !strings.Contains(readAsset(t, "assets/verso-stream.js"), "root.dataset.overviewTone = d.tone;") {
		t.Error("the live stream does not retone the Overview's heading by its attribute")
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
		// the headless section hands its first visible box no air …
		`.verso-section[data-verso-headless]>.verso-section__body>:nth-child(1 of :not(input[type=hidden],[hidden],template)){--verso-edge-air:0px}`,
		`.verso-card__body:is(.verso-card[data-verso-headless]>*)>:nth-child(1 of :not(input[type=hidden],[hidden],template)){--verso-edge-air:0px}`,
		// … which a run of rows, a reveal and a form forward to their own
		// first row …
		`.verso-stack[data-verso-rows]>:nth-child(1 of :not(input[type=hidden],[hidden],template)){--verso-edge-air:inherit}`,
		`.verso-conditional__gate{--verso-edge-air:inherit}`,
		`.verso-form>:nth-child(1 of :not(input[type=hidden],[hidden],template)){--verso-edge-air:inherit}`,
		// … and the row takes as the air over its name
		`padding:var(--verso-edge-air,var(--verso-field-lead,var(--spacing-inset)))`,
		// hidden carriers ahead of the first block take no block gap
		`.verso-rhythm>:nth-child(1 of :not(input[type=hidden],[hidden],template)){margin-top:var(--verso-edge-lead,0px)}`,
	} {
		if !strings.Contains(css, want) {
			t.Errorf("stylesheet is missing %s", want)
		}
	}
	if strings.Contains(blockSheet(t, "section"), "--verso-edge-trail:") {
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
		".verso-section__head[data-verso-section-band]:is(.verso-page__body *):not(.verso-page__body :not(input[type=hidden],template,script,[hidden],:empty)~* *){border-top-width:0;padding-top:0}",
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
// is taken before the cap, as the page block's column does, and the
// page's width is read in whole pixels, so no width rounds a step short of
// the column's. The colophon's words end on the same line.
func TestMastheadEndsUnderLogOut(t *testing.T) {
	page := blockSheet(t, "page")
	for head, wants := range map[string][]string{
		// the page's gutter: 16px on a phone, 40px past it
		"\n.verso-page {": {"--verso-gutter: calc(var(--spacing) * 4);"},
		"@media (width >= 48rem) {\n  .verso-page {": {"--verso-gutter: calc(var(--spacing) * 10);"},
		"\n.verso-page__masthead {": {
			"--verso-measure: calc(round(down, min(round(100cqw, 1px) - var(--verso-gutter) * 2 - 1px, var(--container-6xl)), 0.5rem) + 1px);",
			"margin-inline: calc(var(--verso-gutter) * -1) calc(100% - round(100cqw, 1px) + var(--verso-gutter));",
			"padding: calc(var(--spacing) * 6) calc(round(100cqw, 1px) - var(--verso-gutter) - var(--verso-measure)) calc(var(--spacing) * 6) var(--verso-gutter);",
		},
		// the page's room is the size container the masthead measures against
		"\n.verso-page__main {":   {"container-type: inline-size;"},
		"\n.verso-page__column {": {"width: calc(round(down, 100% - 1px, 0.5rem) + 1px);", "max-width: var(--verso-column-max);"},
		"\n.verso-page__column:where(.verso-page__main[data-verso-width=\"wide\"] *) {": {"--verso-column-max: calc(var(--container-6xl) + 1px);"},
	} {
		rule := cssBlock(t, page, head)
		for _, want := range wants {
			if !strings.Contains(rule, want) {
				t.Errorf("the masthead runs from the rail to the window's edge and its acts end where Log out does; blocks/page.css %q is missing %s", strings.TrimSpace(head), want)
			}
		}
	}
	// Log out's group and the colophon's words are measured as the column is.
	for block, want := range map[string][]string{
		".verso-bar__end {": {
			"flex: none;",
			"margin-inline: var(--verso-gutter);",
			"width: calc(round(down, min(100% - var(--verso-gutter) * 2 - 1px, var(--container-6xl)), 0.5rem) + 1px);",
		},
		".verso-colophon__line {": {
			"width: calc(round(down, min(100% - var(--verso-gutter) * 2 - 1px, var(--container-6xl)), 0.5rem) + 1px + var(--verso-gutter) * 2);",
		},
	} {
		sheet := blockSheet(t, strings.TrimPrefix(strings.SplitN(block, "__", 2)[0], ".verso-"))
		rule := cssBlock(t, sheet, block)
		for _, w := range want {
			if !strings.Contains(rule, w) {
				t.Errorf("the masthead's acts and the colophon's words end where Log out does; %s is missing %s", block, w)
			}
		}
		if strings.Contains(rule, "max-width") {
			t.Errorf("%s is measured as the column is, never capped short of it:\n%s", block, rule)
		}
	}
	// The page is what the rail leaves; the bar's group spans it.
	if !strings.Contains(blockSheet(t, "bar"), "@media (width >= 48rem) {\n  .verso-bar__session {\n    left: calc(var(--spacing) * 72);") {
		t.Error("beside the rail the bar's group starts where the page does")
	}
}

// blockSheet is the source of blocks/<name>.css.
func blockSheet(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile("assets/blocks/" + name + ".css")
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
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
	if !strings.Contains(cssBlock(t, blockSheet(t, "page"), "\n.verso-page__ground {"), "flex: 1 0 auto;") {
		t.Error("a short page's ground fills the window down to the colophon")
	}
	if !strings.Contains(cssBlock(t, blockSheet(t, "colophon"), ".verso-colophon {"), "flex: none;") {
		t.Error("the colophon keeps its own 56px whatever the window's height")
	}
	if body := get(t, newServer(t, fakeBackend{}), "/").Body.String(); !strings.Contains(body, `<footer data-verso-colophon x-data="copy" :data-copied="done" class="verso-colophon">`) {
		t.Error("the page's colophon is not the colophon block")
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
	// The carried parts are the nav block's own: the marker, each row's face,
	// its chevron and the open branch.
	nav := blockSheet(t, "nav")
	if !strings.Contains(nav, ":is(.verso-nav__face, .verso-nav__marker, .verso-nav__chevron, .verso-nav__branch) {\n    view-transition-name: var(--verso-vt);") {
		t.Error("blocks/nav.css does not name the rail's parts for the page change")
	}
	// How the carried parts move is the nav block's too: the rail's one
	// easing on each of its kinds, and the marker under the words it passes.
	for _, want := range []string{
		"::view-transition-group(.verso-nav-row),\n  ::view-transition-group(.verso-nav-sub),\n  ::view-transition-group(.verso-nav-branch),\n  ::view-transition-group(.verso-nav-marker) {\n    animation-duration: 0.36s;",
		"::view-transition-group(.verso-nav-marker) {\n    z-index: 0;",
		"::view-transition-new(.verso-nav-marker) {\n    block-size: 100%;",
	} {
		if !strings.Contains(nav, want) {
			t.Errorf("blocks/nav.css is missing %q", want)
		}
	}
	entry, err := os.ReadFile("assets/input.css")
	if err != nil {
		t.Fatalf("read entry stylesheet: %v", err)
	}
	if strings.Contains(string(entry), "::view-transition-group") {
		t.Error("the entry stylesheet moves the rail's parts; the nav block does")
	}
}

// TestTheRuleAfterASectionBringsItsOwnAir: no section keeps closing air;
// whatever draws a rule after one brings the air before it itself, its margin
// a whole number of cells less the rule's own pixel so the rule lands on a
// line: a ruled section two cells under the last row's line, a heading band's
// section the same, the configuration card one cell under the card before
// it. A section that kept air of its own while the rule after it zeroed its
// margin set every rule off the grid.
func TestTheRuleAfterASectionBringsItsOwnAir(t *testing.T) {
	css := blockSheet(t, "section")
	// A notice or an empty slot is a box like any other, a multiple of 4px
	// tall with its frame, so it takes the full air before it.
	if strings.Contains(css, ":is([data-verso-slot], [data-verso-callout])") {
		t.Error("a notice or a slot still takes its air less a pixel, as a frame standing a pixel out did")
	}
	for head, wants := range map[string][]string{
		`.verso-section[data-verso-section="ruled"] {`: {
			"margin-top: var(--verso-step, calc(var(--spacing) * 10 - 1px));",
			"padding-top: var(--spacing-inset);",
			"border: 0 solid var(--color-rule);",
			"border-top-width: 1px;",
		},
		`.verso-section:has(> .verso-section__head[data-verso-section-band]):is(:not(input[type="hidden"], template, [hidden]) + *) {`: {
			"margin-top: calc(var(--spacing) * 10 - 1px);",
		},
		// a section with no heading that opens under a notice stands the
		// inset's air under it, rather than flush on it
		`.verso-section[data-verso-headless]:is([data-verso-callout] + *) {`: {"margin-top: var(--spacing-inset);"},
		// a listing that ends its section gives its last hairline to the rule
		// that closes it, clear but keeping its pixel
		"\n.verso-section__body > :last-child {": {"--verso-last-rule-color: transparent;"},
	} {
		block := cssBlock(t, css, head)
		for _, want := range wants {
			if !strings.Contains(block, want) {
				t.Errorf("blocks/section.css %s lacks %s", strings.TrimSpace(head), want)
			}
		}
	}
	// The table's last row reads the colour its surface hands it, keeping
	// its pixel either way.
	if !strings.Contains(blockSheet(t, "table"), "border-bottom-width: var(--verso-last-rule-width, 1px);\n  border-bottom-color: var(--verso-last-rule-color, var(--color-rule));") {
		t.Error("a table's last row does not read the hairline its surface hands it")
	}
	// the configuration card stands as a card does: a cell under the one
	// before it, less that card's frame pixel
	if !strings.Contains(cssBlock(t, blockSheet(t, "code"), `.verso-code[data-verso-place="card"] {`), "margin-top: calc(var(--spacing-inset) - 1px);") {
		t.Error("the configuration card does not stand a cell under the card before it")
	}
	for _, gone := range []string{"padding-block: 0 calc(var(--spacing) * 8)", "padding-bottom:"} {
		if strings.Contains(css, gone) {
			t.Errorf("blocks/section.css still trades a section's closing air for the next rule's: %s", gone)
		}
	}
}

// TestASectionTrimsThroughARunOfRows: rows grouped in a flush stack are still
// the section's rows, so an untitled section's first row gives its air back
// from inside the group too, as a gate's row that opens it does. A port
// forward's panel opened lower than a DHCP server's because its first rows
// stood in one. The last row keeps its padding, in a group or not, so it ends
// on a line.
func TestASectionTrimsThroughARunOfRows(t *testing.T) {
	// The section hands its first visible block no air, and a run of rows
	// and a reveal pass it on to the row they open with.
	if !strings.Contains(cssBlock(t, blockSheet(t, "section"), `.verso-section[data-verso-headless] > .verso-section__body > :nth-child(1 of :not(input[type="hidden"], [hidden], template)) {`), "--verso-edge-air: 0px;") {
		t.Error("an untitled section does not take its first row's air back")
	}
	if !strings.Contains(cssBlock(t, blockSheet(t, "stack"), `.verso-stack[data-verso-rows] > :nth-child(1 of :not(input[type="hidden"], [hidden], template)) {`), "--verso-edge-air: inherit;") {
		t.Error("a run of rows does not pass the section's air on to its first row")
	}
	if !strings.Contains(cssBlock(t, blockSheet(t, "conditional"), ".verso-conditional__gate {"), "--verso-edge-air: inherit;") {
		t.Error("a reveal does not pass the section's air on to its gate's row")
	}
	// The section hands its last row nothing: it keeps its padding.
	if strings.Contains(blockSheet(t, "section"), "--verso-edge-trail:") {
		t.Error("a run of rows gives its last row's air back, so it ends off the grid's line")
	}
}

// TestAnUntitledOpeningKeepsNoClosingAir: a drawer's untitled opening block
// (a forward's state and name, a DHCP server's addresses) ends on its last
// row's line, as every section does, and the band, card or Save after it
// brings its own air (TestTheRuleAfterASectionBringsItsOwnAir); an opening
// that kept closing air of its own would push that rule off the line.
func TestAnUntitledOpeningKeepsNoClosingAir(t *testing.T) {
	css := blockSheet(t, "section")
	if strings.Contains(css, "[data-verso-headless]:has(") {
		t.Error("an untitled opening keeps closing air before the rule after it")
	}
	if strings.Contains(css, "padding-bottom") || strings.Contains(css, "margin-bottom: calc(var(--spacing) * 8)") {
		t.Error("a section keeps closing air of its own")
	}
}

// TestAFittingTableClipsNoTip: a table's box scrolls, and so clips both ways,
// only while its table is wider than it; the shell measures every box and one
// whose table fits neither scrolls, clips nor fades, so a first row's act
// raises its tip over the column heads. While the table is wider, the first
// row's tips open under their act rather than into the clipping edge.
func TestAFittingTableClipsNoTip(t *testing.T) {
	for file, want := range map[string]string{
		"assets/blocks/listing.css": ".verso-listing__scroller[data-verso-fits] {\n  overflow: visible;\n  mask-image: none;\n  animation: none;\n}",
		"assets/blocks/tip.css":     ".verso-tip:not([data-verso-tip-kind]):is([data-verso-scroller]:not([data-verso-fits]) tbody > tr:first-child *) {\n  top: 100%;",
	} {
		css, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(css), want) {
			t.Errorf("%s missing %q", file, want)
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

// TestTheScriptDrawsRowsAsTheTableDoes: a live listing's rows are the
// server's own, swapped in from the page it fetches, so they carry the table
// block's classes and type attributes as rendered; the only rows the script
// draws itself are a drag's — the lifted copy, a table block of its own at
// its source's density and edges, and the gap it leaves, the reorder block's.
func TestTheScriptDrawsRowsAsTheTableDoes(t *testing.T) {
	raw, err := os.ReadFile("assets/verso-tables.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(raw)
	for _, want := range []string{
		`doc.querySelectorAll("[data-verso-inventory], [data-verso-live-table]")`,
		`table.className = "verso-table table-fixed"`,
		`table.setAttribute("data-verso-lifted", "")`,
		`["data-verso-dense", "data-verso-in-form"]`,
		`"--verso-table-start"`,
		`copy.className = row.className`,
		`shell.className = "verso-reorder__float"`,
		`td.className = "verso-reorder__gap"`,
		`document.body.classList.add("verso-reorder")`,
	} {
		if !strings.Contains(js, want) {
			t.Errorf("verso-tables.js lacks %s", want)
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
	css, err := os.ReadFile("assets/blocks/tip.css")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		".verso-tip[data-verso-tip-kind] {\n  left: 0;\n  right: auto;\n  display: none;\n  visibility: hidden;",
		"transition: opacity 150ms ease-out, visibility 0s linear 150ms, display 0s linear 150ms;\n  transition-behavior: allow-discrete;",
		".verso-tip[data-verso-tip-kind]:is([data-verso-tip]:hover > *) {\n  display: block;\n  visibility: visible;\n  opacity: 1;\n  pointer-events: auto;\n  transition-delay: 200ms;",
		".verso-tip[data-verso-tip-kind]:is([data-verso-tip]:focus-visible > *) {\n  display: block;\n  visibility: visible;\n  opacity: 1;\n  pointer-events: auto;\n  transition-delay: 0ms;",
		"@starting-style {\n  .verso-tip[data-verso-tip-kind]:is([data-verso-tip]:is(:hover, :focus-visible) > *) {\n    visibility: hidden;\n    opacity: 0;\n  }\n}",
		".verso-tip[data-verso-tip-kind]:is([data-verso-tip-below] > *) {\n  top: 100%;\n  bottom: auto;",
		".verso-tip:is([data-verso-tip-dismissed] > *) {\n  display: none !important;",
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
	sheets, err := filepath.Glob("assets/blocks/*.css")
	if err != nil {
		t.Fatal(err)
	}
	for _, sheet := range append(sheets, "assets/input.css") {
		css, err := os.ReadFile(sheet)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(css), "[data-verso-list-row]") || strings.Contains(string(css), "[data-verso-list-value]") {
			t.Errorf("%s: a page styles its own list rows", sheet)
		}
	}
}
