// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/we-are-mono/verso/internal/openwrt"
	"github.com/we-are-mono/verso/internal/plugin"
)

// pendingServer is a server whose stage holds three changes on two pages of
// the DNS & DHCP plugin: two DHCP settings and a firewall rule.
func pendingServer(t *testing.T, b fakeBackend) *Server {
	t.Helper()
	b.access = true
	if b.changes == nil {
		b.changes = map[string][][]string{
			"dhcp":     {{"set", "lan", "leasetime", "6h"}, {"set", "cfg01", "cachesize", "151"}},
			"firewall": {{"set", "wan", "input", "DROP"}},
		}
	}
	m := dhcpManifest()
	m.Nav = []plugin.NavEntry{{Section: "Network", Label: "DHCP", Path: "/", Pages: plugin.NavPages{List: []plugin.PageTab{
		{Label: "DHCP", Path: ""}, {Label: "Rules", Path: "rules"},
	}}}}
	s := newServerWith(t, b, &fakeTransport{}, []plugin.Manifest{m})
	clock := time.Now()
	s.authors.now = func() time.Time { return clock }
	dhcp := authorAt(m, "/")
	dhcp.Heading = "DHCP"
	rules := authorAt(m, "/rules")
	rules.Heading = "Firewall rules"
	clock = clock.Add(-9 * time.Minute)
	s.authors.note("test-sid", optionAddress("dhcp", "lan", "leasetime"), dhcp)
	s.authors.note("test-sid", optionAddress("dhcp", "cfg01", "cachesize"), dhcp)
	clock = clock.Add(5 * time.Minute)
	s.authors.note("test-sid", optionAddress("firewall", "wan", "input"), rules)
	clock = time.Now()
	return s
}

func mustContain(t *testing.T, body string, wants ...string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(body, want) {
			t.Errorf("page missing %q", want)
		}
	}
}

// TestPendingPageHeadsTheStage: the heading line counts the stage and holds
// its two acts, with nothing under the heading; the When you apply card
// comes first, then one card per page the changes were made on.
func TestPendingPageHeadsTheStage(t *testing.T) {
	s := pendingServer(t, fakeBackend{triggers: map[string][]string{"dhcp": {"dnsmasq"}, "firewall": {"firewall"}}})
	rec := get(t, s, pendingPath)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET %s = %d", pendingPath, rec.Code)
	}
	body := rec.Body.String()
	mustContain(t, body,
		">3 pending changes<", "Apply 3 changes", "Discard all",
		"When you apply", "Saves 3 changes.", "Reloads DNS &amp; DHCP and Firewall.",
		"Keeps them if this page checks in within 30 s.",
		`name="note"`,
		`text-ink">DHCP</h2>`, `text-ink">Firewall rules</h2>`,
		`href="/plugins/dnsdhcp/"`, `href="/plugins/dnsdhcp/rules"`, "Open page", "Discard these",
	)
	// The note's question stands over its box, which takes the row's width.
	note := strings.Index(body, `data-verso-change-name="note"`)
	if note < 0 {
		t.Fatal("the note is not a field row")
	}
	row := body[strings.LastIndex(body[:note], "<div"):]
	if row = row[:strings.Index(row, ">")]; !strings.HasPrefix(row, `<div class="verso-field"`) || !strings.Contains(row, "data-verso-wide") {
		t.Errorf("the note's label does not stand over its box: %s", row)
	}
	apply, dhcp, rules := strings.Index(body, "When you apply"), strings.Index(body, `text-ink">DHCP</h2>`), strings.Index(body, `text-ink">Firewall rules</h2>`)
	if apply < 0 || dhcp < apply || rules < apply {
		t.Errorf("When you apply stands at %d, the cards at %d and %d: it comes first", apply, dhcp, rules)
	}
	// Apply and Discard all stand under the changes, read before them, where
	// a page form's Save stands (24px under the last card, as cards stand);
	// the heading line holds none.
	mustContain(t, body, "data-verso-pending-acts data-verso-form-actions data-verso-rule")
	acts := strings.Index(body, "data-verso-pending-acts")
	if acts < rules || acts < dhcp {
		t.Errorf("the acts stand at %d, before the last card at %d", acts, max(dhcp, rules))
	}
	// Discard all lets the stage go as a Cancel does: a secondary act, no
	// crimson, and nothing to answer first.
	block := body[acts:]
	block = block[:strings.Index(block, "<noscript>")]
	if !strings.Contains(block, `action="/pending/discard"`) || strings.Contains(block, "crimson") {
		t.Errorf("Discard all is not a plain secondary act:\n%s", block)
	}
	// Firewall rules has one row, so no Discard these of its own.
	if strings.Count(body, "Discard these") != 1 {
		t.Errorf("Discard these appears %d times, want once, on the card of two rows", strings.Count(body, "Discard these"))
	}
	// Letting changes go is one act at three reaches, so one verb names it:
	// a row's, a card's, the page's.
	mustContain(t, body, `aria-label="Discard this change`, ">Discard all</button>")
	if strings.Contains(body, "Undo") {
		t.Error("the page names letting a change go two ways, Undo and Discard")
	}
}

// TestPendingPageTakesTheListingsColumn: the page lists changes as System ›
// Changes does, so its cards stand on the wide column every page of listings
// takes, with or without changes, never a width of their own.
func TestPendingPageTakesTheListingsColumn(t *testing.T) {
	for name, s := range map[string]*Server{
		"pending": pendingServer(t, fakeBackend{}),
		"clean":   pendingServer(t, fakeBackend{changes: map[string][][]string{}}),
	} {
		if body := get(t, s, pendingPath).Body.String(); !strings.Contains(body, `class="verso-page__main" data-verso-width="wide"`) {
			t.Errorf("%s: the page does not stand on the listings' wide column", name)
		}
	}
}

// TestPendingRowsSayWhat: each row reads as the Changes page's rows do, says
// what happened, and offers its three acts: its page at its anchor, its undo,
// and its config lines. It says no age: a pending change is minutes old, and
// the card already says where it came from.
func TestPendingRowsSayWhat(t *testing.T) {
	s := pendingServer(t, fakeBackend{committed: map[string]map[string]any{"dhcp": {"lan": map[string]any{"leasetime": "12h"}}}})
	body := get(t, s, pendingPath).Body.String()
	for _, absent := range []string{">Staged<", "min ago"} {
		if strings.Contains(body, absent) {
			t.Errorf("the rows still say when they were staged: %q", absent)
		}
	}
	// The column of what changed is headed by a noun its rows' status words
	// do not repeat ("Changed" over a row whose status is Changed).
	head := body[strings.Index(body, "<thead>"):]
	head = head[:strings.Index(head, "</thead>")]
	if !strings.Contains(head, "Item") || strings.Contains(head, "Changed") {
		t.Errorf("the first column's head repeats a status word:\n%s", head)
	}
	mustContain(t, body,
		"leasetime", "12h", "6h", "Changed",
		`href="/plugins/dnsdhcp/#staged-dhcp.lan.leasetime"`,
		`action="/pending/undo"`, `name="row"`,
		"~ dhcp: lan.leasetime = 6h",
		// An undo changes what the page is made of, so its answer is the page
		// read whole, not its rows reconciled in place.
		`<form method="post" action="/pending/undo" class="contents">`,
	)
}

// TestPendingPageWarnsOfADisconnect: a change to the interface this browser
// reached the router through opens the When you apply card with the warning,
// naming both addresses; a read of the triggers that fails leaves the card
// without a reload step and the page renders.
func TestPendingPageWarnsOfADisconnect(t *testing.T) {
	m := demoManifest()
	m.ID, m.Name = "interfaces", "Interfaces"
	m.ACL = plugin.ACL{Write: []plugin.ACLScope{{Scope: "uci", Object: "network", Function: "write"}}}
	s := newServerWith(t, fakeBackend{
		access:      true,
		changes:     map[string][][]string{"network": {{"set", "lan", "ipaddr", "10.0.0.1"}}},
		netIfaces:   []openwrt.NetIface{{Name: "lan", Addresses: []string{"192.168.1.1"}}},
		triggersErr: errors.New("access denied"),
	}, &fakeTransport{}, []plugin.Manifest{m})
	req := httptest.NewRequest(http.MethodGet, pendingPath, nil)
	req = req.WithContext(context.WithValue(req.Context(), http.LocalAddrContextKey, &net.TCPAddr{IP: net.ParseIP("192.168.1.1"), Port: 80}))
	rec := serve(t, s, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET %s = %d", pendingPath, rec.Code)
	}
	body := rec.Body.String()
	mustContain(t, body, "May disconnect you.", "192.168.1.1", "10.0.0.1")
	if strings.Contains(body, "Reloads") {
		t.Error("a trigger read that failed still lists reloads")
	}
	if i, j := strings.Index(body, "May disconnect you."), strings.Index(body, "Saves 1 change."); i < 0 || j < i {
		t.Error("the warning opens the card, before its steps")
	}
}

// TestPendingPageOfOneChange: the heading and the act say the count's own
// form.
func TestPendingPageOfOneChange(t *testing.T) {
	s := pendingServer(t, fakeBackend{changes: map[string][][]string{"dhcp": {{"set", "lan", "leasetime", "6h"}}}})
	mustContain(t, get(t, s, pendingPath).Body.String(), ">1 pending change<", "Apply 1 change")
}

// TestPendingPageAfterTheStageEnded: nothing pending reads as how the stage
// ended, leading back to the page of its latest change; opened clean, with no
// end on record, it offers no way back.
func TestPendingPageAfterTheStageEnded(t *testing.T) {
	s := pendingServer(t, fakeBackend{changes: map[string][][]string{}})
	body := get(t, s, pendingPath).Body.String()
	mustContain(t, body, ">Nothing pending<")
	if strings.Contains(body, "Back to") {
		t.Error("a clean stage with no end on record offers a way back")
	}

	back := stageAuthor{Plugin: "dnsdhcp", Page: "DHCP", Heading: "Firewall rules", Href: "/plugins/dnsdhcp/rules"}
	s.stageEnds.set("test-sid", stageEnd{Kind: stageApplied, At: time.Date(2026, 10, 17, 15, 42, 0, 0, time.Local), Count: 12, Back: back})
	body = get(t, s, pendingPath).Body.String()
	mustContain(t, body, ">Changes applied<", "The router kept 12 changes at 15:42.", "Back to Firewall rules", `href="/plugins/dnsdhcp/rules"`)

	s.stageEnds.set("test-sid", stageEnd{Kind: stageDiscarded, At: time.Now(), Count: 2, Back: back})
	mustContain(t, get(t, s, pendingPath).Body.String(), ">Nothing pending<", "Back to Firewall rules")
}

// TestPendingPageAfterARollback: changes pending again after the router put
// its settings back open with the callout saying when and why; the
// notification says the outcome once, in words of its own.
func TestPendingPageAfterARollback(t *testing.T) {
	s := pendingServer(t, fakeBackend{})
	s.stageEnds.set("test-sid", stageEnd{Kind: stageRolledBack, At: time.Date(2026, 10, 17, 15, 42, 0, 0, time.Local), Count: 3})
	body := get(t, s, pendingPath).Body.String()
	mustContain(t, body, "The last apply was put back at 15:42.", "This page could not check in", "The apply was put back. Your changes are still pending.")
	if strings.Count(body, "could not check in") != 1 {
		t.Error("the notification repeats the callout's reason")
	}
	if strings.Contains(get(t, s, pendingPath).Body.String(), "The apply was put back.") {
		t.Error("the notification says the outcome more than once")
	}
}

// TestPendingPageWithoutScripting: undo and discard work without a script,
// as forms; Apply waits for one, and says so where there is none.
func TestPendingPageWithoutScripting(t *testing.T) {
	s := pendingServer(t, fakeBackend{})
	body := get(t, s, pendingPath).Body.String()
	mustContain(t, body,
		`action="/pending/undo"`, `action="/pending/discard"`,
		`<noscript>`, "Applying needs scripting",
		`data-verso-pending-apply hidden`,
	)
}

// TestPendingDiscardRoute: the page's Discard all lets the stage go and
// leads back to the page, which then has nothing pending.
func TestPendingDiscardRoute(t *testing.T) {
	reverts := []string{}
	s := pendingServer(t, fakeBackend{reverts: &reverts})
	rec := postPlugin(t, s, pendingPath+"/discard", url.Values{})
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != pendingPath {
		t.Fatalf("discard answered %d to %q", rec.Code, rec.Header().Get("Location"))
	}
	if len(reverts) != 2 {
		t.Errorf("reverted %v, want dhcp and firewall", reverts)
	}
	if end, ok := s.stageEnds.of("test-sid"); !ok || end.Kind != stageDiscarded || end.Count != 3 {
		t.Errorf("the page is told %+v, want three changes discarded", end)
	}
}

// TestTheOldReviewAddressLeadsToThePage: a bookmark of the review drawer's
// address reaches the page.
func TestTheOldReviewAddressLeadsToThePage(t *testing.T) {
	s := pendingServer(t, fakeBackend{})
	rec := get(t, s, "/uci/review")
	if rec.Code != http.StatusPermanentRedirect || rec.Header().Get("Location") != pendingPath {
		t.Errorf("/uci/review answered %d to %q, want 308 to %s", rec.Code, rec.Header().Get("Location"), pendingPath)
	}
}
