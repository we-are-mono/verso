// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/we-are-mono/verso/internal/plugin"
	"github.com/we-are-mono/verso/internal/widget"
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
	commits := []string{}
	tr := &fakeTransport{env: &plugin.Envelope{
		SchemaVersion: 1, Title: "Saved", Status: http.StatusOK,
		Widget: json.RawMessage(`{"type":"card","children":[]}`),
		Commit: []plugin.CommitOp{{Config: "system", Section: "@system[0]", Values: map[string]any{"hostname": "verso-lab"}}},
	}}
	s := newServerWith(t, fakeBackend{access: true, writes: &writes, commits: &commits}, tr, []plugin.Manifest{demoACLManifest()})

	rec := postPlugin(t, s, "/plugins/demo/", url.Values{"hostname": {"verso-lab"}})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if len(writes) != 1 {
		t.Fatalf("UCISet calls = %d, want 1 (the write must stage)", len(writes))
	}
	if len(commits) != 0 {
		t.Fatalf("UCICommit calls = %v, want none — only the apply commits (ADR-010)", commits)
	}
}

// stagedChipHidden is the chip as a clean stage renders it: present, so the
// client can turn it on in place, and hidden.
const stagedChipHidden = `data-count="0" title="Nothing is live yet — review, then apply" hidden`

// getPanel reads a page the way an open drawer frame asks for its contents —
// the same authenticated visit as get, marked as htmx's own request.
func getPanel(t *testing.T, srv *Server, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	token, err := srv.sessions.CreateWithMetadata("test-sid", "root", "", "")
	if err != nil {
		t.Fatalf("session: %v", err)
	}
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: token})
	req.Header.Set("HX-Request", "true")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	return rec
}

// TestStagedChipRendersPendingCount: the chip is server-rendered from UCI's
// stage on every page — on screen when changes are pending, scoped to the
// configs plugins declare, counting what a person did — and it is the way into
// the review drawer.
func TestStagedChipRendersPendingCount(t *testing.T) {
	s := newServerWith(t, fakeBackend{
		access: true,
		changes: map[string][][]string{
			"system":  {{"set", "@system[0]", "hostname", "verso-lab"}},
			"network": {{"set", "lan", "ipaddr", "10.0.0.2"}}, // undeclared: not the shell's to show
		},
	}, &fakeTransport{}, []plugin.Manifest{demoACLManifest()})

	body := get(t, s, "/").Body.String()
	for _, want := range []string{
		`<a id="verso-staged" href="/uci/review" @click.prevent="showPanel" data-count="1"`,
		">1 staged change</span>", // the undeclared config is filtered out
		// The caveat colour: a fact about the device, not an alarm; the mark
		// and the way in beside the count.
		"bg-marigold-soft px-3 text-sm font-semibold whitespace-nowrap text-marigold-deep",
		`class="size-1.5 shrink-0 rounded-[1px] bg-marigold"`, "· review</span>",
		// The drawer's frame is chrome: the browser's address stays the page's
		// while it is open, and every open fetches the stage afresh (verso.js).
		`<div data-verso-panel data-verso-panel-chrome class="contents"></div>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("staged chip missing %q", want)
		}
	}
	if strings.Contains(body, stagedChipHidden) {
		t.Error("a dirty stage must show the chip")
	}
	// The raw line belongs to the drawer, not the page.
	if strings.Contains(body, "system: @system[0].hostname = verso-lab") {
		t.Error("the page carries the count; the lines wait in the drawer")
	}
}

// TestReviewDrawerPlainOverRaw: the owning plugin describes its changes in
// plain words and the drawer renders each sentence as a row that opens on the
// raw uci line(s) it covers; a change no sentence covers keeps its raw line as
// its row.
func TestReviewDrawerPlainOverRaw(t *testing.T) {
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

	rec := getPanel(t, s, "/uci/review")
	if rec.Code != http.StatusOK {
		t.Fatalf("review status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{
		`id="verso-staged-review" data-count="2"`, ">2 staged changes</h2>",
		// The sentence is the row, at the words' weight; its verb leads it.
		`text-sm font-semibold text-ink">Renamed the router.</span>`, ">changed</span>",
		// Its raw line waits under the row, led by the glyph of a change.
		`text-marigold-deep"><span class="w-2.5 shrink-0">~</span><span>system: @system[0].hostname = verso-lab</span>`,
		// The undescribed change is its raw line, in mono.
		`font-mono text-base font-medium text-ink">system: @system[0].timezone = UTC</span>`,
		// The two acts that end the stage, and the promise that makes the first
		// safe to press — stating the window the apply actually arms.
		`id="verso-staged-apply"`, ">Apply 2</button>", `id="verso-staged-discard"`, ">Discard all</button>",
		"Usually takes a couple of seconds, longer if many services are affected. If the router stops answering within 30 seconds, the changes are rolled back.",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("drawer missing %q:\n%s", want, body)
		}
	}
	// The plain sentence renders before the raw line it covers.
	if i, j := strings.Index(body, "Renamed the router."), strings.Index(body, "hostname = verso-lab"); i < 0 || j < 0 || i > j {
		t.Errorf("plain sentence should precede its raw line (plain=%d raw=%d)", i, j)
	}
	// A frame's request gets the contents alone.
	if strings.Contains(body, "<html") {
		t.Error("the drawer's contents must not come wrapped in a page")
	}
	// Describe fired, with the normalized op vocabulary.
	if len(tr.lastDescribe) != 2 || tr.lastDescribe[0].Op != "set" || tr.lastDescribe[0].Option != "hostname" {
		t.Errorf("describe received = %+v", tr.lastDescribe)
	}
}

// TestReviewDrawerGroupsByPage: rows gather under the page they belong to — a
// plugin's config under that plugin's name, the shell's own under System —
// each group a band with its tally, and a change's verb reads from what its
// writes amount to.
func TestReviewDrawerGroupsByPage(t *testing.T) {
	s := newServerWith(t, fakeBackend{
		access: true,
		changes: map[string][][]string{
			"system": {{"set", "cfg99", "rule"}, {"set", "cfg99", "name", "Allow DNS"}},
			"verso":  {{"remove", "updates"}},
		},
	}, &fakeTransport{descriptions: []plugin.Description{
		{Plain: "Added the rule “Allow DNS”.", Covers: []int{0, 1}},
	}}, []plugin.Manifest{demoACLManifest()})

	body := getPanel(t, s, "/uci/review").Body.String()
	for _, want := range []string{
		// The plugin's page, then the shell's, each band with its tally.
		`text-body">Demo Plugin</span>`, `text-body">System</span>`, ">1 change</span>",
		// A new section is added, and every one of its lines is new with it
		// (the plus reaches the page as its entity, which html/template writes
		// for text it cannot know is safe).
		">added</span>", `class="size-1.5 shrink-0 rounded-[1px] bg-green"`,
		`text-green-deep"><span class="w-2.5 shrink-0">&#43;</span><span>system: new rule section cfg99</span>`,
		`text-green-deep"><span class="w-2.5 shrink-0">&#43;</span><span>system: cfg99.name = Allow DNS</span>`,
		// A section going is removed, its line going with it.
		">removed</span>", `class="size-1.5 shrink-0 rounded-[1px] bg-crimson"`,
		`text-crimson-deep"><span class="w-2.5 shrink-0">-</span><span>verso: remove updates</span>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("drawer missing %q:\n%s", want, body)
		}
	}
	if i, j := strings.Index(body, ">Demo Plugin<"), strings.Index(body, ">System<"); i < 0 || j < 0 || i > j {
		t.Errorf("pages follow the order their changes first appear in (plugin=%d shell=%d)", i, j)
	}
}

// TestReviewDrawerEmpty: a clean stage still answers the drawer, saying so, with
// nothing to apply and only the way out.
func TestReviewDrawerEmpty(t *testing.T) {
	s := newServer(t, fakeBackend{access: true})
	body := getPanel(t, s, "/uci/review").Body.String()
	for _, want := range []string{
		`id="verso-staged-review" data-count="0"`, ">No staged changes</h2>",
		"Nothing staged. Every save lands here until you apply.", ">Close</button>",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("empty drawer missing %q:\n%s", want, body)
		}
	}
	for _, absent := range []string{`id="verso-staged-apply"`, `id="verso-staged-discard"`, "Takes effect"} {
		if strings.Contains(body, absent) {
			t.Errorf("an empty drawer has nothing to act on; found %q", absent)
		}
	}
}

// TestReviewPageForABrowserWithNoScript: the chip is a link, and a browser that
// cannot open the drawer follows it to the same contents on a page of their own.
func TestReviewPageForABrowserWithNoScript(t *testing.T) {
	s := newServerWith(t, fakeBackend{
		access:  true,
		changes: map[string][][]string{"system": {{"set", "@system[0]", "hostname", "verso-lab"}}},
	}, &fakeTransport{}, []plugin.Manifest{demoACLManifest()})
	body := get(t, s, "/uci/review").Body.String()
	for _, want := range []string{"<html", ">Staged changes<", `id="verso-staged-review" data-count="1"`, ">Apply 1</button>"} {
		if !strings.Contains(body, want) {
			t.Errorf("review page missing %q", want)
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
	if body := get(t, s, "/").Body.String(); !strings.Contains(body, ">1 staged change</span>") {
		t.Error("three writes folded into one item must read as 1 staged change")
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
	if body := get(t, s2, "/").Body.String(); !strings.Contains(body, ">3 staged changes</span>") {
		t.Error("three ungrouped settings must each count")
	}
}

// TestStagedCountIsTheSameInEveryReading: the chip's count is the described
// count — what a person did — in the basic reading as in the advanced one, so
// the shell describes whenever the stage holds something. A clean stage makes
// no round-trip.
func TestStagedCountIsTheSameInEveryReading(t *testing.T) {
	tr := &fakeTransport{descriptions: []plugin.Description{{Plain: "Renamed the router.", Covers: []int{0, 1}}}}
	s := newServerWith(t, fakeBackend{
		access: true,
		changes: map[string][][]string{"system": {
			{"set", "@system[0]", "hostname", "verso-lab"},
			{"set", "@system[0]", "timezone", "UTC"},
		}},
	}, tr, []plugin.Manifest{demoACLManifest()})

	if body := getMode(t, s, "/", widget.ModeBasic).Body.String(); !strings.Contains(body, ">1 staged change</span>") {
		t.Error("the basic reading counts what a person did, not the writes it took")
	}
	if len(tr.lastDescribe) != 2 {
		t.Errorf("the basic reading must describe too; got %+v", tr.lastDescribe)
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

// TestRecordEditorKeepsItsSubmitAndTheChip: a page carrying a Back link (a
// record editor) has its own submit, and the chip stays in the chrome above it
// like on any page — the stage is the whole device's, not the page's.
func TestRecordEditorKeepsItsSubmitAndTheChip(t *testing.T) {
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
	if !strings.Contains(body, ">1 staged change</span>") {
		t.Error("the chip is on every page the stage is dirty behind")
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

// TestStagedChipHiddenWhenClean: every page carries the chip, hidden while the
// stage is clean — so an act that stages something can turn it on in place —
// and nothing of the bar that once sat at the foot of the page remains.
func TestStagedChipHiddenWhenClean(t *testing.T) {
	tr := &fakeTransport{env: &plugin.Envelope{
		SchemaVersion: 1, Title: "Demo", Status: http.StatusOK,
		Widget: json.RawMessage(`{"type":"card","children":[]}`),
	}}
	s := newServerWith(t, fakeBackend{}, tr, []plugin.Manifest{demoManifest()})

	for _, path := range []string{"/plugins/demo/", "/"} {
		body := get(t, s, path).Body.String()
		if !strings.Contains(body, stagedChipHidden) {
			t.Errorf("%s: a clean stage renders the chip hidden:\n%s", path, body)
		}
		for _, absent := range []string{"verso-capsule", "Save & Apply", "No unsaved changes", "Applies with a"} {
			if strings.Contains(body, absent) {
				t.Errorf("%s: the bar is gone; found %q", path, absent)
			}
		}
	}
}

// TestStagedChipShowsWhenTheStageHoldsChanges: a page that arrives with changes
// already staged renders the chip on, whatever the page is — an immediate-command
// page included, since the stage is shared state that follows the operator.
func TestStagedChipShowsWhenTheStageHoldsChanges(t *testing.T) {
	env := &plugin.Envelope{
		SchemaVersion: 1, Title: "Access", Immediate: true, Status: http.StatusOK,
		Widget: json.RawMessage(`{"type":"card","children":[]}`),
	}
	s := newServerWith(t, fakeBackend{changes: map[string][][]string{
		"system": {{"set", "@system[0]", "hostname", "pending"}},
	}}, &fakeTransport{env: env}, []plugin.Manifest{demoACLManifest()})

	body := get(t, s, "/plugins/demo/").Body.String()
	if strings.Contains(body, stagedChipHidden) || !strings.Contains(body, ">1 staged change</span>") {
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

func TestPluginApplyActionRunsAfterUCIApply(t *testing.T) {
	var sequence []string
	manifest := demoACLManifest()
	manifest.ACL.Write = append(manifest.ACL.Write, plugin.ACLScope{
		Scope: "ubus", Object: "verso", Function: "setSystemTime",
	})
	tr := &fakeTransport{env: &plugin.Envelope{
		SchemaVersion: 1, Status: http.StatusOK,
		Widget: json.RawMessage(`{"type":"card","children":[]}`),
		Commit: []plugin.CommitOp{{Config: "system", Section: "@system[0]", Values: map[string]any{"hostname": "router"}}},
		Apply: []plugin.ApplyAction{{Name: "set-system-time", Args: map[string]string{
			"datetime": "2026-08-30T12:34:56", "timezone": "CET-1CEST,M3.5.0,M10.5.0/3",
		}}},
	}}
	s := newServerWith(t, fakeBackend{
		access:  true,
		applies: &[]int{},
		setSystemTime: func(_ context.Context, sid, datetime, timezone string) error {
			sequence = append(sequence, "time "+sid+" "+datetime+" "+timezone)
			return nil
		},
	}, tr, []plugin.Manifest{manifest})

	if rec := postPlugin(t, s, "/plugins/demo/", url.Values{"hostname": {"router"}}); rec.Code != http.StatusOK {
		t.Fatalf("prepare status = %d, want 200", rec.Code)
	}
	if rec := postPlugin(t, s, "/uci/apply", nil); rec.Code != http.StatusOK {
		t.Fatalf("apply status = %d, want 200", rec.Code)
	}
	if len(sequence) != 1 || sequence[0] != "time test-sid 2026-08-30T12:34:56 CET-1CEST,M3.5.0,M10.5.0/3" {
		t.Fatalf("apply action calls = %v", sequence)
	}
	if len(s.takePendingApply("test-sid")) != 0 {
		t.Fatal("successful apply must clear its one-shot action")
	}
}

// TestPendingApplyIsSessionScoped: one session's armed apply tail is invisible to
// another session's apply, an empty save never wipes it, and draining clears it
// atomically.
func TestPendingApplyIsSessionScoped(t *testing.T) {
	s := newServer(t, fakeBackend{})
	action := plugin.ApplyAction{Name: "set-system-time", Args: map[string]string{
		"datetime": "2026-08-30T12:34:56", "timezone": "GMT0",
	}}
	s.setPendingApply("alice", []plugin.ApplyAction{action})

	// Bob's apply drains only Bob's (empty) tail — never Alice's.
	if got := s.takePendingApply("bob"); len(got) != 0 {
		t.Fatalf("bob's apply drained %d actions from alice's tail", len(got))
	}
	s.setPendingApply("alice", nil) // an unrelated save with no tail must not wipe it
	// Alice's own apply drains exactly her one action, then clears it atomically.
	if got := s.takePendingApply("alice"); len(got) != 1 {
		t.Fatalf("alice drained %d actions, want 1 (survived bob + empty save)", len(got))
	}
	if got := s.takePendingApply("alice"); len(got) != 0 {
		t.Fatal("a drained tail must be cleared")
	}
}

func TestPluginApplyActionRequiresDeclaredScope(t *testing.T) {
	tr := &fakeTransport{env: &plugin.Envelope{
		SchemaVersion: 1, Status: http.StatusOK,
		Widget: json.RawMessage(`{"type":"card","children":[]}`),
		Apply: []plugin.ApplyAction{{Name: "set-system-time", Args: map[string]string{
			"datetime": "2026-08-30T12:34:56", "timezone": "GMT0",
		}}},
	}}
	s := newServerWith(t, fakeBackend{access: true}, tr, []plugin.Manifest{demoACLManifest()})
	if rec := postPlugin(t, s, "/plugins/demo/", nil); rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}

func TestNestedAlternateFieldErrorBlocksTransaction(t *testing.T) {
	called := false
	tr := &fakeTransport{env: &plugin.Envelope{
		SchemaVersion: 1, Status: http.StatusOK,
		Widget: json.RawMessage(`{"type":"form","style":"page","fields":[
			{"type":"section","children":[{"type":"conditional","name":"enabled","checked":false,
			"otherwise":[{"type":"stack","children":[{"type":"field","name":"datetime","error":"Invalid time"}]}]}]}
		]}`),
		Apply: []plugin.ApplyAction{{Name: "set-system-time", Args: map[string]string{
			"datetime": "bad", "timezone": "GMT0",
		}}},
	}}
	manifest := demoACLManifest()
	manifest.ACL.Write = append(manifest.ACL.Write, plugin.ACLScope{Scope: "ubus", Object: "verso", Function: "setSystemTime"})
	s := newServerWith(t, fakeBackend{
		access:        true,
		setSystemTime: func(context.Context, string, string, string) error { called = true; return nil },
	}, tr, []plugin.Manifest{manifest})
	if rec := postPlugin(t, s, "/plugins/demo/", nil); rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", rec.Code)
	}
	if called || len(s.takePendingApply("test-sid")) != 0 {
		t.Fatal("invalid alternate field must not prepare or execute an apply action")
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
	}
	for _, c := range cases {
		if got := humanizeChange("network", c.ch); got != c.want {
			t.Errorf("humanizeChange(%v) = %q, want %q", c.ch, got, c.want)
		}
	}
}

func TestCustomFileReviewShowsThePathAndProposedContents(t *testing.T) {
	tuple := []string{"file", "/etc/dnsmasq.conf", "log-queries\n"}
	raw := humanizeChange("dhcp", tuple)
	if raw != "/etc/dnsmasq.conf" {
		t.Fatal(raw)
	}
	change := stagedChange{config: "dhcp", tuple: tuple, raw: raw}
	row := stagedItemOf(raw, true, []stagedChange{change}, func(s string) string { return s })
	if len(row.Lines) != 1 || row.Lines[0].Text != "log-queries\n" {
		t.Fatalf("%+v", row)
	}
}
