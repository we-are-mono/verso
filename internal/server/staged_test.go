// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/we-are-mono/verso/internal/plugin"
)

// TestPluginSaveStagesWithoutCommit: a plugin's commit intent stages through
// rpcd and stops there (ADR-010) — the review drawer owns apply; the broker
// must never commit.
// TestCoalesceChangesNetsRepeatedWrites: rpcd's change journal records every
// write, so an inline toggle flipped repeatedly must not pile up. The net effect
// per target is what counts — the last write wins, a value added then removed in
// the same session cancels, and a real removal or a distinct option stands.
func TestCoalesceChangesNetsRepeatedWrites(t *testing.T) {
	cases := []struct {
		name string
		in   [][]string
		want int
	}{
		{"one write", [][]string{{"set", "r1", "enabled", "0"}}, 1},
		{"same option written twenty times", func() [][]string {
			var j [][]string
			for i := 0; i < 20; i++ {
				j = append(j, []string{"set", "r1", "enabled", "0"})
			}
			return j
		}(), 1},
		{"added then removed cancels (toggle off then on)", [][]string{
			{"set", "r1", "enabled", "0"}, {"remove", "r1", "enabled"},
		}, 0},
		{"flipped back and forth ten times, ending on", [][]string{
			{"set", "r1", "enabled", "0"}, {"remove", "r1", "enabled"},
			{"set", "r1", "enabled", "0"}, {"remove", "r1", "enabled"},
		}, 0},
		{"flipped, ending off", [][]string{
			{"set", "r1", "enabled", "0"}, {"remove", "r1", "enabled"},
			{"set", "r1", "enabled", "0"},
		}, 1},
		{"lone removal of a committed option stands", [][]string{
			{"remove", "r1", "log"},
		}, 1},
		{"distinct options each count", [][]string{
			{"set", "r1", "enabled", "0"}, {"set", "r2", "target", "DROP"},
		}, 2},
	}
	for _, tc := range cases {
		if got := len(coalesceChanges(tc.in)); got != tc.want {
			t.Errorf("%s: coalesced to %d changes, want %d", tc.name, got, tc.want)
		}
	}
}

func TestPluginSaveStagesWithoutCommit(t *testing.T) {
	writes := []uciWrite{}
	tr := &fakeTransport{env: &plugin.Envelope{
		SchemaVersion: 1, Title: "Saved", Status: http.StatusOK,
		Widget: json.RawMessage(`{"type":"card","children":[]}`),
		Commit: []plugin.CommitOp{{Config: "system", Section: "@system[0]", Values: map[string]any{"hostname": "verso-lab"}}},
	}}
	s := newServerWith(t, fakeBackend{access: true, writes: &writes}, tr, []plugin.Manifest{demoACLManifest()})

	rec := postPlugin(t, s, "/plugins/demo/", url.Values{"hostname": {"verso-lab"}})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303: a staged save is read again", rec.Code)
	}
	if len(writes) != 1 {
		t.Fatalf("UCISet calls = %d, want 1 (the write must stage)", len(writes))
	}
}

// pendingRowHidden is the rail's Pending changes row as a clean stage draws
// it: there, hidden, for an act that stages something to turn on in place.
const pendingRowHidden = `id="verso-pending-row" data-count="0" hidden`

// pendingRowSays is the rail's Pending changes row's count as a page draws
// it with n changes waiting.
func pendingRowSays(n int) string {
	return fmt.Sprintf(`id="verso-pending-count" class="ml-auto tabular-nums">%d</span>`, n)
}

// getPanel reads a page the way an open drawer frame asks for its contents —
// the same authenticated visit as get, marked as htmx's own request.
func getPanel(t *testing.T, srv *Server, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	token := srv.sessions.CreateWithMetadata("test-sid", "root", "", "")
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: token})
	req.Header.Set("HX-Request", "true")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	return rec
}

// TestThePendingRowCountsTheStage: the rail's row is server-rendered from
// UCI's stage on every page — on screen when changes are pending, scoped to
// the configs plugins declare, counting what a person did — and it leads to
// the Pending changes page.
func TestThePendingRowCountsTheStage(t *testing.T) {
	s := newServerWith(t, fakeBackend{
		access: true,
		changes: map[string][][]string{
			"system":  {{"set", "@system[0]", "hostname", "verso-lab"}},
			"network": {{"set", "lan", "ipaddr", "10.0.0.2"}}, // undeclared: not the shell's to show
		},
	}, &fakeTransport{}, []plugin.Manifest{demoACLManifest()})

	body := get(t, s, "/").Body.String()
	for _, want := range []string{
		`id="verso-pending-row" data-count="1"`,
		pendingRowSays(1), // the undeclared config is filtered out
		`<a href="/pending"`, ">Pending changes</span>",
		`class="verso-nav__row" data-verso-stage>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the Pending changes row is missing %q", want)
		}
	}
	// The caveat colour: a fact about the device, not an alarm.
	wash := cssBlock(t, blockSheet(t, "nav"), ".verso-nav__row[data-verso-stage],\n.verso-nav__row[data-verso-stage]:hover {")
	for _, want := range []string{"background-color: var(--color-marigold-soft);", "color: var(--color-marigold-deep);"} {
		if !strings.Contains(wash, want) {
			t.Errorf("the Pending changes row's wash lacks %s", want)
		}
	}
	if strings.Contains(body, pendingRowHidden) {
		t.Error("a dirty stage must show the row")
	}
	// The raw line belongs to the Pending changes page, not every page.
	if strings.Contains(body, "system: @system[0].hostname = verso-lab") {
		t.Error("the page carries the count; the lines wait on the Pending changes page")
	}
}

// TestThePendingRowStandsFirstInTheRail: what waits on the stage stands
// above every destination, in no group; the top bar holds no chip, and on a
// phone the menu button says the count.
func TestThePendingRowStandsFirstInTheRail(t *testing.T) {
	s := newServerWith(t, fakeBackend{
		access:  true,
		changes: map[string][][]string{"system": {{"set", "@system[0]", "hostname", "verso-lab"}}},
	}, &fakeTransport{}, []plugin.Manifest{demoACLManifest()})
	body := get(t, s, "/").Body.String()
	body = body[strings.LastIndex(body, "</style>")+len("</style>"):]
	rail := strings.Index(body, "data-verso-nav-rows")
	row := strings.Index(body, `id="verso-pending-row"`)
	overview := strings.Index(body, `<a href="/" aria-current="page"`)
	if rail < 0 || row < rail || overview < row {
		t.Errorf("the row stands first in the rail (rail %d, row %d, Overview %d)", rail, row, overview)
	}
	if strings.Contains(body, `id="verso-staged"`) {
		t.Error("the top bar still holds the chip")
	}
	if !strings.Contains(body, `id="verso-menu-count" class="verso-bar__count" data-verso-figures>1</span>`) {
		t.Error("on a phone the menu button says the count")
	}
	if !strings.Contains(body, `data-count="1" class="verso-bar__menu"`) {
		t.Error("on a phone the menu button carries the stage's count for its wash")
	}
	if !strings.Contains(body, `<a href="/pending" class="verso-nav__row" data-verso-stage>`) {
		t.Error("the Pending changes row is a rail row on the stage's wash")
	}
	if !strings.Contains(get(t, s, pendingPath).Body.String(), `<a href="/pending" aria-current="page" class="verso-nav__row" data-verso-stage>`) {
		t.Error("on the Pending changes page the row carries the marker")
	}
	nav := blockSheet(t, "nav")
	for _, want := range []string{
		".verso-nav__row[data-verso-stage],\n.verso-nav__row[data-verso-stage]:hover {\n  background-color: var(--color-marigold-soft);\n  color: var(--color-marigold-deep);",
		".verso-nav__row[data-verso-stage][aria-current=\"page\"] {\n  box-shadow: inset -0.125rem 0 0 var(--color-marigold);\n}",
	} {
		if !strings.Contains(nav, want) {
			t.Errorf("blocks/nav.css is missing the stage's row: %s", want)
		}
	}
}

// TestPendingPagePlainOverRaw: the owning plugin describes its changes in
// plain words and the Pending changes page renders each sentence as a row
// whose config lines are the raw uci line(s) it covers; a change no sentence
// covers keeps its raw line as its row.
func TestPendingPagePlainOverRaw(t *testing.T) {
	tr := &fakeTransport{descriptions: []plugin.Description{
		{Plain: "Renamed the router.", Covers: []int{0}}, // covers only the first change
	}}
	s := newServerWith(t, fakeBackend{
		access: true,
		changes: map[string][][]string{
			"system": {
				{"set", "@system[0]", "hostname", "verso-lab"},
				{"set", "@system[0]", "timezone", "UTC"},
			},
		},
	}, tr, []plugin.Manifest{demoACLManifest()})

	rec := get(t, s, pendingPath)
	if rec.Code != http.StatusOK {
		t.Fatalf("page status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{
		">2 pending changes<",
		// The sentence is the row; its verb is its status.
		"Renamed the router.", ">Changed<",
		// Its raw line waits in the row's config lines, led by the glyph of a
		// change.
		"~ system: @system[0].hostname = verso-lab",
		// The undescribed change is its raw line.
		"system: @system[0].timezone = UTC",
		// The two acts that end the stage.
		"Apply 2 changes", `action="/pending/discard"`, ">Discard all</button>",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("page missing %q", want)
		}
	}
	// Discard all asks nothing first: the router runs as it is either way.
	if strings.Contains(body, "Discard all pending changes?") {
		t.Error("Discard all asks before letting the stage go")
	}
	// The plain sentence renders before the raw line it covers.
	if i, j := strings.Index(body, "Renamed the router."), strings.Index(body, "hostname = verso-lab"); i < 0 || j < 0 || i > j {
		t.Errorf("plain sentence should precede its raw line (plain=%d raw=%d)", i, j)
	}
	// Describe fired, with the normalized op vocabulary.
	if len(tr.lastDescribe) != 2 || tr.lastDescribe[0].Op != "set" || tr.lastDescribe[0].Option != "hostname" {
		t.Errorf("describe received = %+v", tr.lastDescribe)
	}
}

// TestPendingPageSaysASettingByItsLabel: a change a plugin describes as one
// of its settings reads the way the settings row did — the setting's name,
// the option's key chip, its new value — never the section's handle.
func TestPendingPageSaysASettingByItsLabel(t *testing.T) {
	tr := &fakeTransport{descriptions: []plugin.Description{
		{Plain: "Log every lease: on", Covers: []int{0}, Setting: &plugin.Setting{Label: "Log every lease", Value: "on"}},
		{Plain: "Renamed the router.", Covers: []int{1}},
		{Plain: "Added the rule “Allow DNS”.", Covers: []int{2, 3}},
	}}
	s := newServerWith(t, fakeBackend{
		access: true,
		changes: map[string][][]string{
			"system": {
				{"set", "cfg01411c", "logdhcp", "1"},
				{"set", "@system[0]", "hostname", "verso-lab"},
				{"set", "cfg99", "rule"},
				{"set", "cfg99", "name", "Allow DNS"},
			},
		},
	}, tr, []plugin.Manifest{demoACLManifest()})

	body := get(t, s, pendingPath).Body.String()
	// The rows' cells, past the note whose placeholder names them too and the
	// row that carries its name.
	body = body[strings.Index(body, "<td"):]
	cell := func(text string) string {
		at := strings.Index(body, text)
		if at < 0 {
			t.Fatalf("no row says %q", text)
		}
		start := strings.LastIndex(body[:at], "<td")
		end := strings.Index(body[at:], "</td>")
		return body[start : at+end]
	}
	setting := cell("Log every lease")
	for _, want := range []string{">logdhcp<", ">on<"} {
		if !strings.Contains(setting, want) {
			t.Errorf("the setting's row is missing %q:\n%s", want, setting)
		}
	}
	if strings.Contains(setting, "cfg01411c") {
		t.Errorf("the setting's row names the section's handle:\n%s", setting)
	}
	for _, sentence := range []string{"Renamed the router.", "Allow DNS"} {
		if !strings.Contains(body, sentence) {
			t.Errorf("no row says %q", sentence)
		}
	}
}

// TestASettingNamesItsOwnKey: a setting whose writes fold another option in
// (a time zone written with its rule) names its own option for its chip.
func TestASettingNamesItsOwnKey(t *testing.T) {
	tr := &fakeTransport{descriptions: []plugin.Description{
		{Plain: "Time zone: Europe/Ljubljana", Covers: []int{0, 1},
			Setting: &plugin.Setting{Label: "Time zone", Value: "Europe/Ljubljana", Verbatim: true, Key: "zonename"}},
	}}
	s := newServerWith(t, fakeBackend{
		access: true,
		changes: map[string][][]string{"system": {
			{"set", "@system[0]", "zonename", "Europe/Ljubljana"},
			{"set", "@system[0]", "timezone", "CET-1CEST,M3.5.0,M10.5.0/3"},
		}},
	}, tr, []plugin.Manifest{demoACLManifest()})
	body := get(t, s, pendingPath).Body.String()
	at := strings.Index(body, "Time zone")
	if at < 0 {
		t.Fatal("no row says Time zone")
	}
	row := body[at : at+strings.Index(body[at:], "</td>")]
	for _, want := range []string{">zonename<", "Europe/Ljubljana"} {
		if !strings.Contains(row, want) {
			t.Errorf("the time zone's row is missing %q:\n%s", want, row)
		}
	}
	if strings.Contains(row, ">timezone<") {
		t.Errorf("the time zone's row names the option folded into it:\n%s", row)
	}
}

// TestPendingPageGroupsByPage: rows gather under the page they belong to — a
// plugin's config under that plugin's name, the shell's own under System —
// each a card, and a change's verb reads from what its writes amount to.
func TestPendingPageGroupsByPage(t *testing.T) {
	s := newServerWith(t, fakeBackend{
		access: true,
		changes: map[string][][]string{
			"system": {{"set", "cfg99", "rule"}, {"set", "cfg99", "name", "Allow DNS"}},
			"verso":  {{"remove", "updates"}},
		},
	}, &fakeTransport{descriptions: []plugin.Description{
		{Plain: "Added the rule “Allow DNS”.", Covers: []int{0, 1}},
	}}, []plugin.Manifest{demoACLManifest()})

	body := get(t, s, pendingPath).Body.String()
	for _, want := range []string{
		// The plugin's page, then the shell's, each a card.
		`<h2 class="verso-card__title">Demo Plugin</h2>`, `<h2 class="verso-card__title">System</h2>`,
		// A new section is added, and every one of its lines is new with it;
		// a section going is removed, its line going with it.
		// (The plus reaches the page as its entity, which html/template writes
		// for text it cannot know is safe.)
		">Added<", "&#43; system: new rule section cfg99", "&#43; system: cfg99.name = Allow DNS",
		">Removed<", "- verso: remove updates",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("page missing %q", want)
		}
	}
	// What happened is marked in its tone: an addition green, a removal
	// crimson.
	for word, tone := range map[string]string{"Added": "success", "Removed": "danger"} {
		if !regexp.MustCompile(`<span class="verso-mark" data-verso-tone="` + tone + `"></span>\s*<span[^>]*>` + word + `</span>`).MatchString(body) {
			t.Errorf("%s is not marked in the %s tone", word, tone)
		}
	}
	if i, j := strings.Index(body, `<h2 class="verso-card__title">Demo Plugin</h2>`), strings.Index(body, `<h2 class="verso-card__title">System</h2>`); i < 0 || j < 0 || i > j {
		t.Errorf("pages follow the order their changes first appear in (plugin=%d shell=%d)", i, j)
	}
}

// TestPendingPageFilesAChangeUnderThePageThatMadeIt: a config is often
// shared — network is written by Interfaces and by General alike — so a change
// is filed under the plugin that staged it, not the first to declare its
// config. What the shell staged itself it remembers until the stage is applied
// or discarded.
func TestPendingPageFilesAChangeUnderThePageThatMadeIt(t *testing.T) {
	first := demoManifest()
	first.ID, first.Name, first.Socket = "interfaces", "Interfaces", "/run/verso/interfaces.sock"
	first.ACL = plugin.ACL{Write: []plugin.ACLScope{{Scope: "uci", Object: "network", Function: "write"}}}
	// One plugin, two pages: its own General, and its part of the shell's
	// Access. The drawer names the page, not the plugin.
	general := demoManifest()
	general.ID, general.Name, general.Socket = "general", "System", "/run/verso/general.sock"
	general.Nav = []plugin.NavEntry{{Section: "System", Label: "General", Path: "/"}}
	general.ACL = first.ACL
	tr := &fakeTransport{env: &plugin.Envelope{
		SchemaVersion: 1, Title: "General", Status: http.StatusOK,
		Widget: json.RawMessage(`{"type":"text","markdown":"body"}`),
		Commit: []plugin.CommitOp{{Config: "network", Section: "globals", Values: map[string]any{"packet_steering": "0"}}},
	}}
	calls := []uciWrite{}
	s := newServerWith(t, fakeBackend{access: true, writes: &calls, changes: map[string][][]string{
		"network": {{"set", "globals", "packet_steering", "0"}},
	}}, tr, []plugin.Manifest{first, general})

	if rec := postPlugin(t, s, "/plugins/general/", url.Values{"packet_steering": {""}}); rec.Code != http.StatusSeeOther {
		t.Fatalf("stage answered %d", rec.Code)
	}
	body := get(t, s, pendingPath).Body.String()
	for _, unwanted := range []string{`<h2 class="verso-card__title">Interfaces</h2>`, `<h2 class="verso-card__title">System</h2>`} {
		if strings.Contains(body, unwanted) {
			t.Errorf("filed under %s, not the page that made it", unwanted)
		}
	}
	if !strings.Contains(body, `<h2 class="verso-card__title">General</h2>`) {
		t.Error("the change is filed under the page that made it, General")
	}
	if got := pageLabel(general, "/"); got != "General" {
		t.Errorf("the plugin's root page is %q, want General", got)
	}
	general.SystemAccess = "/access"
	if got := pageLabel(general, "/access"); got != "Access" {
		t.Errorf("the plugin's part of Access is %q, want Access", got)
	}
	// Discarded, the record goes with the stage.
	postPlugin(t, s, "/uci/discard", url.Values{})
	if _, ok := s.authors.of("test-sid", optionAddress("network", "globals", "packet_steering")); ok {
		t.Error("the record of who staged what outlives the stage it describes")
	}
}

// TestPendingPageEmpty: a clean stage still answers the page, saying so, with
// nothing to apply or discard.
func TestPendingPageEmpty(t *testing.T) {
	s := newServer(t, fakeBackend{access: true})
	body := get(t, s, pendingPath).Body.String()
	if !strings.Contains(body, ">Nothing pending<") {
		t.Error("a clean stage's page does not say Nothing pending")
	}
	for _, absent := range []string{"data-verso-pending-apply", `action="/pending/discard"`} {
		if strings.Contains(body, absent) {
			t.Errorf("an empty page has nothing to act on; found %q", absent)
		}
	}
}

// TestStagedCountsItemsNotWrites: the count is what a person did, not how many
// uci writes it took. A plugin that folds all of one item's writes into a single
// described row makes that item count once; writes it leaves undescribed (a
// settings page's options) each count on their own.
func TestStagedCountsItemsNotWrites(t *testing.T) {
	// An item: three writes to one new rule, folded into one sentence → one change.
	item := &fakeTransport{descriptions: []plugin.Description{
		{Plain: "Added the rule “Allow DNS”.", Covers: []int{0, 1, 2}},
	}}
	s := newServerWith(t, fakeBackend{
		access: true,
		changes: map[string][][]string{"system": {
			{"set", "cfg99", "rule"},
			{"set", "cfg99", "name", "Allow DNS"},
			{"set", "cfg99", "dest_port", "53"},
		}},
	}, item, []plugin.Manifest{demoACLManifest()})
	if body := get(t, s, "/").Body.String(); !strings.Contains(body, pendingRowSays(1)) {
		t.Error("three writes folded into one item must count as 1 change")
	}

	// Settings: three option writes the plugin does not group → three changes.
	settings := &fakeTransport{} // describes nothing
	s2 := newServerWith(t, fakeBackend{
		access: true,
		changes: map[string][][]string{"system": {
			{"set", "@system[0]", "hostname", "verso-lab"},
			{"set", "@system[0]", "timezone", "UTC"},
			{"set", "@system[0]", "log_size", "128"},
		}},
	}, settings, []plugin.Manifest{demoACLManifest()})
	if body := get(t, s2, "/").Body.String(); !strings.Contains(body, pendingRowSays(3)) {
		t.Error("three ungrouped settings must each count")
	}
}

// TestStagedCountIsTheDescribedCount: the chip's count is the described count —
// what a person did — so the shell describes whenever the stage holds
// something. A clean stage makes no round-trip.
func TestStagedCountIsTheDescribedCount(t *testing.T) {
	tr := &fakeTransport{descriptions: []plugin.Description{{Plain: "Renamed the router.", Covers: []int{0, 1}}}}
	s := newServerWith(t, fakeBackend{
		access: true,
		changes: map[string][][]string{"system": {
			{"set", "@system[0]", "hostname", "verso-lab"},
			{"set", "@system[0]", "timezone", "UTC"},
		}},
	}, tr, []plugin.Manifest{demoACLManifest()})

	if body := get(t, s, "/").Body.String(); !strings.Contains(body, pendingRowSays(1)) {
		t.Error("the row counts what a person did, not the writes it took")
	}
	if len(tr.lastDescribe) != 2 {
		t.Errorf("the page must describe the stage; got %+v", tr.lastDescribe)
	}
	clean := &fakeTransport{}
	_ = get(t, newServerWith(t, fakeBackend{access: true}, clean, []plugin.Manifest{demoACLManifest()}), "/")
	if clean.lastDescribe != nil {
		t.Errorf("a clean stage must not describe; got %+v", clean.lastDescribe)
	}
}

// TestRecordEditorSubmitReturnsToListing: a record editor's submit stages its one
// change and 303-redirects to the listing it came from (its Back), rather than
// re-rendering the form — the person applies from the review drawer, where the
// change now waits.
func TestRecordEditorSubmitReturnsToListing(t *testing.T) {
	writes := []uciWrite{}
	tr := &fakeTransport{env: &plugin.Envelope{
		SchemaVersion: 1, Title: "New rule", Status: http.StatusOK,
		Back:   &plugin.PageAction{Label: "Cancel", Href: "/plugins/demo/"},
		Commit: []plugin.CommitOp{{Config: "system", Section: "@system[0]", Values: map[string]any{"hostname": "verso-lab"}}},
		Widget: json.RawMessage(`{"type":"card","children":[]}`),
	}}
	s := newServerWith(t, fakeBackend{access: true, writes: &writes}, tr, []plugin.Manifest{demoACLManifest()})

	rec := postPlugin(t, s, "/plugins/demo/rules/new", url.Values{"hostname": {"verso-lab"}})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want a 303 back to the listing", rec.Code)
	}
	if got := rec.Header().Get("Location"); got != "/plugins/demo/" {
		t.Errorf("Location = %q, want the editor's Back listing", got)
	}
	if len(writes) != 1 {
		t.Errorf("the editor submit must stage its change (writes = %d)", len(writes))
	}
}

// TestRecordEditorKeepsItsSubmitAndTheRow: a page carrying a Back link (a
// record editor) has its own submit, and the rail's Pending changes row
// stays beside it like on any page — the stage is the whole device's, not
// the page's.
func TestRecordEditorKeepsItsSubmitAndTheRow(t *testing.T) {
	tr := &fakeTransport{env: &plugin.Envelope{
		SchemaVersion: 1, Title: "Edit rule", Status: http.StatusOK,
		Back:   &plugin.PageAction{Label: "Cancel", Href: "/plugins/demo/"},
		Widget: json.RawMessage(`{"type":"form","style":"page","submit":"Save changes","fields":[]}`),
	}}
	s := newServerWith(t, fakeBackend{
		access:  true,
		changes: map[string][][]string{"system": {{"set", "@system[0]", "hostname", "x"}}},
	}, tr, []plugin.Manifest{demoACLManifest()})

	body := get(t, s, "/plugins/demo/rules/edit").Body.String()
	if !strings.Contains(body, pendingRowSays(1)) {
		t.Error("the row is on every page the stage is dirty behind")
	}
	if !strings.Contains(body, "Save changes") {
		t.Error("the editor must carry its own submit button")
	}
}

// TestPageFormGetsTheShellsSubmit: a page form the plugin left label-less has
// nothing else to submit it, so the shell gives it the button — pressing it
// stages, and the drawer applies.
func TestPageFormGetsTheShellsSubmit(t *testing.T) {
	tr := &fakeTransport{env: &plugin.Envelope{
		SchemaVersion: 1, Title: "General", Status: http.StatusOK,
		Widget: json.RawMessage(`{"type":"form","style":"page","fields":[{"type":"field","name":"hostname","label":"Hostname"}]}`),
	}}
	s := newServerWith(t, fakeBackend{access: true}, tr, []plugin.Manifest{demoACLManifest()})
	if body := get(t, s, "/plugins/demo/").Body.String(); !strings.Contains(body, ">Save changes</button>") {
		t.Errorf("a label-less page form must get the shell's submit:\n%s", body)
	}
}

// TestThePendingRowHiddenWhenClean: every page carries the row, hidden while
// the stage is clean — so an act that stages something can turn it on in
// place — and nothing of the bar that once sat at the foot of the page
// remains.
func TestThePendingRowHiddenWhenClean(t *testing.T) {
	tr := &fakeTransport{env: &plugin.Envelope{
		SchemaVersion: 1, Title: "Demo", Status: http.StatusOK,
		Widget: json.RawMessage(`{"type":"card","children":[]}`),
	}}
	s := newServerWith(t, fakeBackend{}, tr, []plugin.Manifest{demoManifest()})

	for _, path := range []string{"/plugins/demo/", "/"} {
		body := get(t, s, path).Body.String()
		if !strings.Contains(body, pendingRowHidden) {
			t.Errorf("%s: a clean stage renders the row hidden", path)
		}
		for _, absent := range []string{"verso-capsule", "Save & Apply", "No unsaved changes", "Applies with a"} {
			if strings.Contains(body, absent) {
				t.Errorf("%s: the bar is gone; found %q", path, absent)
			}
		}
	}
}

// TestThePendingRowShowsWhenTheStageHoldsChanges: a page that arrives with
// changes already staged renders the row on, whatever the page is — an
// immediate-command page included, since the stage is shared state that
// follows the operator.
func TestThePendingRowShowsWhenTheStageHoldsChanges(t *testing.T) {
	env := &plugin.Envelope{
		SchemaVersion: 1, Title: "Access", Immediate: true, Status: http.StatusOK,
		Widget: json.RawMessage(`{"type":"card","children":[]}`),
	}
	s := newServerWith(t, fakeBackend{changes: map[string][][]string{
		"system": {{"set", "@system[0]", "hostname", "pending"}},
	}}, &fakeTransport{env: env}, []plugin.Manifest{demoACLManifest()})

	body := get(t, s, "/plugins/demo/").Body.String()
	if strings.Contains(body, pendingRowHidden) || !strings.Contains(body, pendingRowSays(1)) {
		t.Error("an immediate-command page must still reveal pending changes from elsewhere")
	}
}

// TestUCIApplyRoute: Apply calls rpcd's uci apply with the rollback window.
func TestUCIApplyRoute(t *testing.T) {
	applies := []int{}
	s := newServer(t, fakeBackend{applies: &applies})
	rec := postPlugin(t, s, "/uci/apply", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if len(applies) != 1 || applies[0] != uciRollbackTimeout {
		t.Errorf("UCIApply calls = %v, want one with the %ds rollback window", applies, uciRollbackTimeout)
	}
}

func TestNestedAlternateFieldErrorBlocksTransaction(t *testing.T) {
	var writes []uciWrite
	tr := &fakeTransport{env: &plugin.Envelope{
		SchemaVersion: 1, Status: http.StatusOK,
		Widget: json.RawMessage(`{"type":"form","style":"page","fields":[
			{"type":"section","children":[{"type":"conditional","name":"enabled","checked":false,
			"otherwise":[{"type":"stack","children":[{"type":"field","name":"datetime","error":"Invalid time"}]}]}]}
		]}`),
		Commit: []plugin.CommitOp{{Config: "system", Section: "@system[0]", Values: map[string]any{"hostname": "router"}}},
	}}
	s := newServerWith(t, fakeBackend{access: true, writes: &writes}, tr, []plugin.Manifest{demoACLManifest()})
	if rec := postPlugin(t, s, "/plugins/demo/", nil); rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", rec.Code)
	}
	if len(writes) != 0 {
		t.Fatalf("invalid alternate field must not stage the transaction: %v", writes)
	}
}

// TestUCIConfirmRoute: Confirm disarms the pending rollback.
func TestUCIConfirmRoute(t *testing.T) {
	confirms := 0
	s := newServer(t, fakeBackend{confirms: &confirms})
	rec := postPlugin(t, s, "/uci/confirm", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if confirms != 1 {
		t.Errorf("UCIConfirm calls = %d, want 1", confirms)
	}
}

// TestUCIDiscardRevertsDeclaredConfigs: Discard reverts every dirty config a
// plugin declares — and only those; a stage on an undeclared config is not the
// shell's to manage.
func TestUCIDiscardRevertsDeclaredConfigs(t *testing.T) {
	reverts := []string{}
	s := newServerWith(t, fakeBackend{
		changes: map[string][][]string{
			"system":  {{"set", "@system[0]", "hostname", "x"}},
			"network": {{"set", "lan", "ipaddr", "y"}},
		},
		reverts: &reverts,
	}, &fakeTransport{}, []plugin.Manifest{demoACLManifest()})

	rec := postPlugin(t, s, "/uci/discard", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if len(reverts) != 1 || reverts[0] != "system" {
		t.Errorf("reverts = %v, want [system] only", reverts)
	}
}

// TestUCIApplyRequiresCSRF: the drawer's posts sit behind the same CSRF gate as
// every state-changing request (VS-04).
func TestUCIApplyRequiresCSRF(t *testing.T) {
	s := newServer(t, fakeBackend{})
	rec := postForm(t, s, "/uci/apply", url.Values{})
	if rec.Code == http.StatusOK {
		t.Fatalf("status = %d; an unauthenticated, tokenless apply must not succeed", rec.Code)
	}
}

// TestHumanizeChange: the mechanical rendering of uci change tuples.
func TestHumanizeChange(t *testing.T) {
	cases := []struct {
		ch   []string
		want string
	}{
		{[]string{"set", "lan", "ipaddr", "10.0.0.2"}, "network: lan.ipaddr = 10.0.0.2"},
		{[]string{"set", "guest", "zone"}, "network: new zone section guest"},
		{[]string{"add", "cfg0b", "rule"}, "network: new rule section cfg0b"},
		{[]string{"remove", "lan", "ipaddr"}, "network: remove lan.ipaddr"},
		{[]string{"remove", "lan"}, "network: remove lan"},
		{[]string{"list-add", "ntp", "server", "a.pool"}, "network: ntp.server += a.pool"},
		{[]string{"rename", "lan", "trusted"}, "network: rename lan trusted"},
		// A secret is staged like any value but never read back onto a screen.
		{[]string{"set", "wan", "password", "hunter2"}, "network: wan.password = ••••••••"},
		{[]string{"set", "wg0", "private_key", "aGVsbG8="}, "network: wg0.private_key = ••••••••"},
		{[]string{"set", "proton", "cert_password", "x"}, "network: proton.cert_password = ••••••••"},
	}
	for _, c := range cases {
		if got := humanizeChange("network", c.ch, nil); got != c.want {
			t.Errorf("humanizeChange(%v) = %q, want %q", c.ch, got, c.want)
		}
	}
}

func TestCustomFileReviewShowsThePathAndProposedContents(t *testing.T) {
	tuple := []string{"file", "/etc/dnsmasq.conf", "log-queries\n"}
	raw := humanizeChange("dhcp", tuple, nil)
	if raw != "/etc/dnsmasq.conf" {
		t.Fatal(raw)
	}
	change := stagedChange{config: "dhcp", tuple: tuple, raw: raw}
	row := stagedItemOf(raw, true, []stagedChange{change}, func(s string) string { return s })
	if len(row.Lines) != 1 || row.Lines[0].Text != "log-queries\n" {
		t.Fatalf("%+v", row)
	}
}

// A setting's row says what the option held before the stage, where the two
// values are said the same way: the committed value when the row shows the
// value as written, "not set" when there was none, and nothing at all for a
// secret or for a value the plugin put in words ("on") the old one is not in.
func TestASettingRowSaysWhatItReplaces(t *testing.T) {
	tr := func(s string) string { return s }
	setting := func(option, value, shown string, verbatim bool) stagedItem {
		return stagedItem{
			Setting: &plugin.Setting{Label: option, Value: shown, Verbatim: verbatim},
			Key:     option,
			changes: []stagedChange{{config: "dhcp", tuple: []string{"set", "cfg01411c", option, value}}},
		}
	}
	committed := map[string]map[string]any{"dhcp": {"cfg01411c": map[string]any{"cachesize": "150", "logdhcp": "0"}}}
	secrets := map[string]bool{"dhcp.key": true}
	for _, tc := range []struct {
		name      string
		item      stagedItem
		committed map[string]map[string]any
		want      *stagedWas
	}{
		{"a value as written", setting("cachesize", "151", "151", true), committed, &stagedWas{Text: "150", Verbatim: true}},
		{"nothing before", setting("cachesize", "151", "151", true), map[string]map[string]any{"dhcp": {"cfg01411c": map[string]any{}}}, &stagedWas{Text: "not set"}},
		{"words for a flag", setting("logdhcp", "1", "on", false), committed, nil},
		{"words, nothing before", setting("logdhcp", "1", "on", false), map[string]map[string]any{"dhcp": {"cfg01411c": map[string]any{}}}, &stagedWas{Text: "not set"}},
		{"a secret", setting("key", "hunter2", "••••", false), committed, nil},
		{"no setting", stagedItem{Label: "Added a host"}, committed, nil},
	} {
		got := settingWas(tc.item, tc.committed, secrets, tr)
		if (got == nil) != (tc.want == nil) || (got != nil && *got != *tc.want) {
			t.Errorf("%s: was = %+v, want %+v", tc.name, got, tc.want)
		}
	}
}

// The drawer's act names what it applies, in the count's own form: "Apply 1
// change", never a bare "Apply 1"; Slovenian's dual and paucal come from the
// catalog.
func TestTheApplyActNamesWhatItApplies(t *testing.T) {
	plain := func(s string) string { return s }
	sl := map[string]string{"Apply %d change": "Uveljavi %d spremembo", "Apply %d changes (two)": "Uveljavi %d spremembi", "Apply %d changes (few)": "Uveljavi %d spremembe", "Apply %d changes": "Uveljavi %d sprememb"}
	slTr := func(s string) string {
		if v, ok := sl[s]; ok {
			return v
		}
		return s
	}
	for n, want := range map[int][2]string{
		1: {"Apply 1 change", "Uveljavi 1 spremembo"},
		2: {"Apply 2 changes", "Uveljavi 2 spremembi"},
		3: {"Apply 3 changes", "Uveljavi 3 spremembe"},
		7: {"Apply 7 changes", "Uveljavi 7 sprememb"},
	} {
		if got := stagedApplyLabel(n, plain); got != want[0] {
			t.Errorf("en %d = %q, want %q", n, got, want[0])
		}
		if got := stagedApplyLabel(n, slTr); got != want[1] {
			t.Errorf("sl %d = %q, want %q", n, got, want[1])
		}
	}
}

// TestStagedViewSaysWhereAndWhen: each page's run of changes names the page
// by the heading its plugin gave it and leads back to it; each row says how
// long ago it was staged and carries the anchor its page marks it by; and the
// view says which change was staged last, in the row's own words.
func TestStagedViewSaysWhereAndWhen(t *testing.T) {
	m := demoACLManifest()
	m.Nav = []plugin.NavEntry{{Section: "Apps", Label: "Demo", Path: "/", Pages: plugin.NavPages{List: []plugin.PageTab{
		{Label: "Rules", Path: ""}, {Label: "Zones", Path: "zones"},
	}}}}
	s := newServerWith(t, fakeBackend{access: true, changes: map[string][][]string{
		"system": {
			{"set", "cfg99", "rule"}, {"set", "cfg99", "name", "Allow DNS"},
			{"set", "lan", "input", "REJECT"},
		},
	}}, &fakeTransport{descriptions: []plugin.Description{
		{Plain: "Added the rule “Allow DNS”.", Covers: []int{0, 1}},
		{Plain: "LAN input is reject", Covers: []int{2}, Setting: &plugin.Setting{Label: "Input", Value: "REJECT", Verbatim: true, Key: "input"}},
	}}, []plugin.Manifest{m})
	start := time.Now()
	clock := start
	s.authors.now = func() time.Time { return clock }

	rules := authorAt(m, "/")
	rules.Heading = "Demo rules"
	clock = start.Add(-10 * time.Minute)
	s.authors.note("test-sid", sectionAddress("system", "cfg99"), rules)
	zones := authorAt(m, "/zones/lan")
	clock = start.Add(-4 * time.Minute)
	s.authors.note("test-sid", optionAddress("system", "lan", "input"), zones)
	clock = start

	identity := func(s string) string { return s }
	view := s.staged(context.Background(), "test-sid", identity, func(string) func(string) string { return identity })
	if len(view.Groups) != 2 {
		t.Fatalf("groups = %+v, want one per page the changes were made on", view.Groups)
	}
	first, second := view.Groups[0], view.Groups[1]
	if first.Heading != "Demo rules" || first.Href != "/plugins/demo/" {
		t.Errorf("first card = %q at %q, want the page's heading, Demo rules, at /plugins/demo/", first.Heading, first.Href)
	}
	if second.Heading != "Demo" || second.Href != "/plugins/demo/zones" {
		t.Errorf("second card = %q at %q, want the page named as its entry, Demo, at /plugins/demo/zones", second.Heading, second.Href)
	}
	rule, input := first.Items[0], second.Items[0]
	if rule.Anchor != "staged-system.cfg99" || input.Anchor != "staged-system.lan.input" {
		t.Errorf("anchors = %q, %q, want the section's and the option's", rule.Anchor, input.Anchor)
	}
	if rule.ID == "" || rule.ID == input.ID {
		t.Errorf("row ids = %q, %q, want each row its own", rule.ID, input.ID)
	}
	if view.Latest != "Input → REJECT" {
		t.Errorf("Latest = %q, want the last staged row's words, Input → REJECT", view.Latest)
	}
	if view.Count != 2 {
		t.Errorf("Count = %d, want rows, not writes: 2", view.Count)
	}
}

// TestASavedPageNamesItsCardByItsHeading: a save renders its page again, and
// the title that render carries is what the page's card is named by.
func TestASavedPageNamesItsCardByItsHeading(t *testing.T) {
	tr := &fakeTransport{env: &plugin.Envelope{
		SchemaVersion: 1, Title: "Demo rules", Status: http.StatusOK,
		Widget: json.RawMessage(`{"type":"text","markdown":"body"}`),
		Commit: []plugin.CommitOp{{Config: "system", Section: "lan", Values: map[string]any{"input": "REJECT"}}},
	}}
	s := newServerWith(t, fakeBackend{access: true, writes: &[]uciWrite{}, changes: map[string][][]string{
		"system": {{"set", "lan", "input", "REJECT"}},
	}}, tr, []plugin.Manifest{demoACLManifest()})
	if rec := postPlugin(t, s, "/plugins/demo/", url.Values{"input": {"REJECT"}}); rec.Code != http.StatusSeeOther {
		t.Fatalf("stage answered %d", rec.Code)
	}
	identity := func(s string) string { return s }
	view := s.staged(context.Background(), "test-sid", identity, func(string) func(string) string { return identity })
	if len(view.Groups) != 1 || view.Groups[0].Heading != "Demo rules" || view.Groups[0].Href != "/plugins/demo/" {
		t.Errorf("groups = %+v, want one card, Demo rules, at /plugins/demo/", view.Groups)
	}
}

// TestStagedRowWithNoTimeOnRecordIsNoLatest: a change staged before the shell
// last started has no time on record, so it is no stage's latest change.
func TestStagedRowWithNoTimeOnRecordIsNoLatest(t *testing.T) {
	s := newServerWith(t, fakeBackend{access: true, changes: map[string][][]string{
		"system": {{"set", "lan", "input", "REJECT"}},
	}}, &fakeTransport{}, []plugin.Manifest{demoACLManifest()})
	identity := func(s string) string { return s }
	view := s.staged(context.Background(), "test-sid", identity, func(string) func(string) string { return identity })
	if view.Latest != "" {
		t.Errorf("Latest = %q, want none", view.Latest)
	}
}
