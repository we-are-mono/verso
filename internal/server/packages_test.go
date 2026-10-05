// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/we-are-mono/verso/internal/openwrt"
	"github.com/we-are-mono/verso/internal/plugin"
	"github.com/we-are-mono/verso/internal/updatecheck"
)

// mgmtManifest is an installed plugin with declared powers, as the management
// surfaces present it (ADR-011).
func mgmtManifest() plugin.Manifest {
	m := demoManifest()
	m.ACL = plugin.ACL{
		Write: []plugin.ACLScope{{Scope: "uci", Object: "firewall", Function: "write"}},
		Read:  []plugin.ACLScope{{Scope: "ubus", Object: "network.device", Function: "status"}},
	}
	return m
}

func pluginsServer(t *testing.T, b fakeBackend, alive bool, manifests ...plugin.Manifest) *Server {
	t.Helper()
	s := newServerWith(t, b, &fakeTransport{}, manifests)
	s.probe = func(string) bool { return alive }
	return s
}

// TestPackagesInventory: the Installed face is the inventory — every package
// as a row, its story and Remove in the drawer, and no lifecycle cells (that
// is the Services page's question).
func TestPackagesInventory(t *testing.T) {
	var removes []string
	b := fakeBackend{access: true,
		pkgInstalledList: []openwrt.Package{
			{Name: "htop", Version: "3.5.1-r1", Feed: "packages", Description: "Process viewer", License: "GPL-2.0", Webpage: "https://htop.dev", Installed: true, Removable: true},
		},
		pkgRemoves: &removes,
	}
	s := pluginsServer(t, b, true, mgmtManifest())

	body := get(t, s, "/system/packages").Body.String()
	for _, want := range []string{
		`href="/system/packages" aria-current="page"`,
		`href="/system/packages/discover"`,
		`<select data-package-cut data-verso-listing-cut`, `<option value="upgradable">Upgradable · `,
		`<option value="all" data-href="/system/packages?tab=all">All</option>`, // the index is its own listing
		`data-verso-actionbar class="flex flex-wrap items-center gap-4 rounded-xs border border-rule-strong bg-mid p-3"`,
		"htop", "3.5.1-r1", "packages", // the row
		"font-mono text-base font-medium", // package versions use the fixed 16px/500 mono treatment
		"Process viewer", "GPL-2.0",       // the drawer's story
		">Remove</button>",    // removal is the row's act, asked on the row
		"max-w-6xl",           // package management uses the focused content width
		"verso-prose text-sm", // the description is plain body prose, no heading over it
		`href="https://htop.dev" target="_blank" rel="noopener noreferrer"`,        // project link opens safely outside Verso
		"space-y-0 border-b border-mid not-first:pt-2", "border-t border-mid py-2", // facts are hairline rows 32px under the prose, closed by a rule as a table's are
		`<header class="flex h-13 flex-none items-center gap-4 border-b border-rule bg-quiet px-10">`, // shared title band
	} {
		if !strings.Contains(body, want) {
			t.Errorf("inventory missing %q", want)
		}
	}
	if strings.Contains(body, `name="svc:`) || strings.Contains(body, `name="on:`) {
		t.Error("the inventory carries no lifecycle switches — services own those")
	}

	rec := postPlugin(t, s, "/system/packages", url.Values{"package": {"htop"}, "_primary": {"remove"}})
	if rec.Code != http.StatusSeeOther || len(removes) != 1 || removes[0] != "htop" {
		t.Fatalf("code=%d removes=%v", rec.Code, removes)
	}
}

func TestPackageDependencyHasNoRemoveAction(t *testing.T) {
	b := fakeBackend{access: true, pkgInstalledList: []openwrt.Package{
		{Name: "ca-bundle", Version: "20260601-r1", Feed: "base", Installed: true,
			RequiredBy: []string{"verso"}},
	}}
	body := get(t, pluginsServer(t, b, true, mgmtManifest()), "/system/packages").Body.String()
	if strings.Contains(body, ">Remove</button>") {
		t.Error("a package required by another installed package must not offer Remove")
	}
	if !strings.Contains(body, "Required by") {
		t.Error("the drawer should explain why the package cannot be removed")
	}
}

func TestPackageSearchStaysInItsPanel(t *testing.T) {
	s := pluginsServer(t, fakeBackend{access: true, pkgFound: []openwrt.Package{{Name: "htop", Description: "Process viewer"}}, pkgTotal: 1}, true)
	res, _ := postPluginFromPanel(t, s, "/system/packages/discover", url.Values{"_primary": {"search"}, "q": {"htop"}})
	if res.Code != http.StatusOK || res.Header().Get("Location") != "" || strings.Contains(res.Body.String(), "<main") {
		t.Fatalf("search returned a page/redirect: %d %v", res.Code, res.Header())
	}
	for _, want := range []string{"Process viewer", `hx-post="/system/packages/discover"`, `hx-post="/system/packages"`} {
		if !strings.Contains(res.Body.String(), want) {
			t.Errorf("panel missing %q", want)
		}
	}
}

// TestPackagesIsWhereUpdatesAre: Packages holds the upgradable list — each row
// its available version under the installed one, in the action's colour (news,
// not a warning) — and the act that updates them all, on the heading line beside
// Install. Maintenance only leads here.
func TestPackagesIsWhereUpdatesAre(t *testing.T) {
	idleUpdates(t)
	s := pluginsServer(t, fakeBackend{access: true, pkgInstalledList: []openwrt.Package{
		{Name: "dnsmasq", Version: "2.91-r3", Installed: true},
		{Name: "htop", Version: "3.4.1", Installed: true},
	}}, true)
	knownUpdates(t, s, updatecheck.Truth{CheckedAt: time.Now(), Packages: []openwrt.PackageUpgrade{{Name: "dnsmasq", Installed: "2.91-r3", Available: "2.93-r1"}}})
	body := get(t, s, "/system/packages").Body.String()
	installed := strings.Index(body, ">2.91-r3<")
	next := strings.Index(body, ">2.93-r1<")
	if installed < 0 || next < installed {
		t.Fatalf("an upgradable row states its next version under the installed one:\n%s", body)
	}
	if strings.Contains(body[installed:next], "marigold") || !strings.Contains(body[installed:next], "text-denim") {
		t.Errorf("an available version is news, led in the action's colour, not a warning:\n%s", body[installed:next])
	}
	for _, want := range []string{`action="/system/packages/upgrade"`, "Update 1 package"} {
		if !strings.Contains(body, want) {
			t.Errorf("the heading line should carry the update act, missing %q", want)
		}
	}
}

// TestAPackageIsRemovedFromItsRow: removal is never a drawer's act. A package
// that can go is removed from its row's trash act, which asks first and then
// posts the one pair; the drawer holds no remove at all.
func TestAPackageIsRemovedFromItsRow(t *testing.T) {
	idleUpdates(t)
	var removed []string
	s := pluginsServer(t, fakeBackend{access: true, pkgRemoves: &removed, pkgInstalledList: []openwrt.Package{
		{Name: "htop", Version: "3.4.1", Installed: true, Removable: true},
	}}, true)
	body := get(t, s, "/system/packages").Body.String()
	for _, want := range []string{`name="remove" value="htop"`, "Remove htop?"} {
		if !strings.Contains(body, want) {
			t.Errorf("the row's trash act should remove, asking first; missing %q", want)
		}
	}
	if strings.Contains(body, `name="_primary" value="remove"`) {
		t.Error("the drawer holds no remove")
	}
	postPlugin(t, s, "/system/packages", url.Values{"remove": {"htop"}})
	if len(removed) != 1 || removed[0] != "htop" {
		t.Errorf("the row's removal should remove htop, removed %v", removed)
	}
}

// TestAPackageDrawerReadsAsTodaysDrawers: a package is one flow, no headings
// and no rule — what it is, its site, its facts with the version reading as the
// table does (installed, then what it becomes), the files it installed, and its
// one act, named for what it does to what.
func TestAPackageDrawerReadsAsTodaysDrawers(t *testing.T) {
	idleUpdates(t)
	s := pluginsServer(t, fakeBackend{access: true, pkgInstalledList: []openwrt.Package{
		{Name: "htop", Version: "3.4.1", Installed: true, Removable: true, Description: "Process viewer"},
	}}, true)
	knownUpdates(t, s, updatecheck.Truth{CheckedAt: time.Now(), Packages: []openwrt.PackageUpgrade{{Name: "htop", Installed: "3.4.1", Available: "3.5.0"}}})
	body := get(t, s, "/system/packages").Body.String()
	// Headings, not the table's "What it is" column label.
	for _, gone := range []string{">What it is</h", ">Details</h"} {
		if strings.Contains(body, gone) {
			t.Errorf("the drawer is one flow, but carries the heading %q", gone)
		}
	}
	at := strings.Index(body, `data-verso-prop-next`)
	if at < 0 {
		t.Fatalf("the drawer's version fact should state what it becomes:\n%s", body)
	}
	drawer := body[strings.LastIndex(body[:at], "Process viewer"):]
	for _, want := range []string{"Update htop", `value="upgrade"`, `href="/system/packages/files?package=htop"`, ">Installed files<"} {
		if !strings.Contains(drawer, want) {
			t.Errorf("the drawer is missing %q", want)
		}
	}
	if strings.Contains(drawer, ">Upgrade<") || strings.Contains(drawer, `name="_primary" value="remove"`) {
		t.Error("the drawer names its act for what it does, and holds no remove")
	}
	// The version fact reads as the table's cell: installed, then the arrow
	// into what it becomes.
	block := drawer[strings.Index(drawer, "data-verso-prop-next"):]
	installed, next := strings.Index(block, ">3.4.1<"), strings.Index(block, ">3.5.0<")
	if installed < 0 || next < installed || !strings.Contains(block[installed:next], "m15 10 5 5-5 5") {
		t.Errorf("the version fact should lead its next value with the arrow:\n%s", block[:min(len(block), 900)])
	}
}

func TestInstalledSearchResultOffersItsSinglePackageUpgrade(t *testing.T) {
	s := pluginsServer(t, fakeBackend{access: true, pkgFound: []openwrt.Package{{Name: "htop", Version: "1", Installed: true}}, pkgTotal: 1}, true)
	knownUpdates(t, s, updatecheck.Truth{CheckedAt: time.Now(), Packages: []openwrt.PackageUpgrade{{Name: "htop", Installed: "1", Available: "2"}}})
	res, _ := postPluginFromPanel(t, s, "/system/packages/discover", url.Values{"_primary": {"search"}, "q": {"htop"}})
	if res.Code != 200 || !strings.Contains(res.Body.String(), `value="upgrade"`) {
		t.Fatalf("installed search result lost its upgrade: %s", res.Body.String())
	}
}

func TestPackagePanelActionsReturnOutcomes(t *testing.T) {
	for _, verb := range []string{"install", "remove", "upgrade"} {
		t.Run(verb, func(t *testing.T) {
			var installed, removed, upgraded []string
			var bulk int
			s := pluginsServer(t, fakeBackend{access: true, pkgInstalls: &installed, pkgRemoves: &removed, pkgSingleUpgrades: &upgraded, pkgUpgrades: &bulk}, true)
			res, _ := postPluginFromPanel(t, s, "/system/packages", url.Values{"package": {"htop"}, "_primary": {verb}})
			if res.Code != http.StatusOK || res.Header().Get("HX-Reswap") != "none" || res.Header().Get("X-Verso-Packages") != "changed" || strings.Contains(res.Body.String(), "<main") {
				t.Fatalf("action must close on an outcome: %d %v %s", res.Code, res.Header(), res.Body.String())
			}
			if !strings.Contains(res.Body.String(), "data-package-navigation") {
				t.Fatal("package mutation must refresh plugin navigation with its outcome")
			}
			calls := append(append(installed, removed...), upgraded...)
			if len(calls) != 1 || calls[0] != "htop" || bulk != 0 {
				t.Fatalf("calls %v, bulk %d", calls, bulk)
			}
		})
	}
	s := pluginsServer(t, fakeBackend{access: true, pkgErr: errors.New("dependency refused")}, true)
	res, _ := postPluginFromPanel(t, s, "/system/packages", url.Values{"package": {"htop"}, "_primary": {"remove"}})
	if res.Code != http.StatusBadGateway || res.Header().Get("HX-Reswap") != "" || !strings.Contains(res.Body.String(), "dependency refused") || strings.Contains(res.Body.String(), "<main") {
		t.Fatalf("refusal must keep the form: %d %s", res.Code, res.Body.String())
	}
}

func TestAllPackagesIsAPaginatedListingFragment(t *testing.T) {
	s := pluginsServer(t, fakeBackend{access: true, pkgTotal: 75, pkgFound: []openwrt.Package{{Name: "available-package", Description: "Available"}}}, true)
	req := httptest.NewRequest("GET", "/system/packages?tab=all&offset=30", nil)
	token := s.sessions.CreateWithMetadata("test-sid", "root", "", "")
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: token})
	req.Header.Set("X-Verso-Interaction", "packages")
	res := httptest.NewRecorder()
	s.Handler().ServeHTTP(res, req)
	body := res.Body.String()
	if res.Code != 200 || strings.Contains(body, "<main") {
		t.Fatalf("not a fragment: %d", res.Code)
	}
	for _, want := range []string{`data-package-all="true"`, "available-package", "offset=0", "offset=60"} {
		if !strings.Contains(body, want) {
			t.Errorf("listing missing %q", want)
		}
	}
}

func TestInstalledFilesAreEscapedAndNamesValidated(t *testing.T) {
	s := pluginsServer(t, fakeBackend{access: true, pkgFiles: []string{"/usr/bin/htop", "/etc/<script>alert(1)</script>"}}, true)
	res := get(t, s, "/system/packages/files?package=htop")
	if res.Code != 200 || !strings.Contains(res.Body.String(), "/usr/bin/htop") || strings.Contains(res.Body.String(), "<script>") {
		t.Fatalf("files not escaped: %s", res.Body.String())
	}
	if res := get(t, s, "/system/packages/files?package=../etc/passwd"); res.Code != http.StatusBadRequest {
		t.Fatalf("invalid name accepted: %d", res.Code)
	}
}

// TestFlashConfirmsActions: an action's confirmation survives the redirect,
// shows once on the next render, and is gone after — the PRG flash.
func TestFlashConfirmsActions(t *testing.T) {
	var removes []string
	b := fakeBackend{access: true,
		pkgInstalledList: []openwrt.Package{{Name: "htop", Version: "3.5.1-r1", Feed: "packages", Installed: true, Removable: true}},
		pkgRemoves:       &removes,
	}
	s := pluginsServer(t, b, true, mgmtManifest())

	// One session across POST and the two GETs, unlike the per-call helpers.
	token := s.sessions.CreateWithMetadata("test-sid", "root", "", "")
	sess, _ := s.sessions.get(token)
	do := func(method, path string, form url.Values) *httptest.ResponseRecorder {
		var req *http.Request
		if form != nil {
			form.Set("_csrf", sess.csrf)
			req = httptest.NewRequest(method, path, strings.NewReader(form.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		} else {
			req = httptest.NewRequest(method, path, nil)
		}
		req.AddCookie(&http.Cookie{Name: sessionCookie, Value: token})
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, req)
		return rec
	}

	if rec := do(http.MethodPost, "/system/packages", url.Values{"package": {"htop"}, "_primary": {"remove"}}); rec.Code != http.StatusSeeOther {
		t.Fatalf("remove: got %d", rec.Code)
	}
	if body := do(http.MethodGet, "/system/packages", nil).Body.String(); !strings.Contains(body, "htop removed.") {
		t.Error("the redirect target must show the confirmation")
	} else if !strings.Contains(body, "border-green-line bg-green-soft text-green-deep") {
		t.Error("a success flash wears green's soft ground and its text step")
	}
	if body := do(http.MethodGet, "/system/packages", nil).Body.String(); strings.Contains(body, "htop removed.") {
		t.Error("a flash shows once, not twice")
	}
}

// TestDiscoverSearchRenders: the Available face lists the helper's matches with
// state, freshness, and an install drawer per row.
func TestDiscoverSearchRenders(t *testing.T) {
	s := pluginsServer(t, fakeBackend{access: true,
		pkgCheckedAt: 1, pkgTotal: 2,
		pkgFound: []openwrt.Package{
			{Name: "htop", Version: "3.5.1-r1", Feed: "packages", Description: "Process viewer"},
			{Name: "htop-lang", Version: "3.5.1-r1", Feed: "packages", Installed: true, Removable: true},
		},
	}, true, mgmtManifest())

	body := get(t, s, "/system/packages/discover?q=htop").Body.String()
	if !strings.Contains(body, "max-w-6xl") {
		t.Error("Available must use the narrow package-management width")
	}
	if !strings.Contains(body, "max-w-sm") {
		t.Error("Available's search field should use the compound search width")
	}
	// bg-denim, not a stock ramp: the page inlines the whole stylesheet, so asking
	// for a colour the app does not use still found it — Tailwind had compiled the
	// class because this line named it.
	for _, want := range []string{"[&_input[type=text]]:pr-28", "absolute inset-y-1 right-1", "bg-denim"} {
		if !strings.Contains(body, want) {
			t.Errorf("Available's compound search control missing %q", want)
		}
	}
	for _, want := range []string{
		"htop", "Process viewer", "3.5.1-r1", "packages",
		"font-mono text-base font-medium", // Available versions match Installed
		"installed",                       // the already-present package carries its state
		"htop-lang",                       // package identities label their drawers
		"Index refreshed",                 // freshness honesty
		`href="/system/packages"`,         // the local switch links the faces
		`action="/system/packages/discover"`,
		"Install packages",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("discover missing %q", want)
		}
	}
}

func TestAvailableWaitsForExplicitSearch(t *testing.T) {
	s := pluginsServer(t, fakeBackend{access: true,
		pkgFound: []openwrt.Package{{Name: "must-not-render-before-search"}},
	}, true, mgmtManifest())

	body := get(t, s, "/system/packages/discover").Body.String()
	// Before a search the listing is its one row, saying what goes there.
	for _, want := range []string{
		`text-left text-sm leading-6 text-meta group-last:border-b-0">Enter a package name, then select Search. Matching packages will appear here.</td>`,
		`placeholder="Package name"`,
		"autofocus",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("initial Available state missing %q", want)
		}
	}
	if strings.Contains(body, "must-not-render-before-search") {
		t.Error("Available must not manufacture results before an explicit search")
	}
}

func TestAvailableNoResultsExplainsRecovery(t *testing.T) {
	body := get(t, pluginsServer(t, fakeBackend{access: true}, true, mgmtManifest()), "/system/packages/discover?q=missing").Body.String()
	// No match is the listing's one row, telling a filtered nothing apart.
	for _, want := range []string{
		`text-left text-sm leading-6 text-meta group-last:border-b-0">No available packages match “missing”. Check the spelling or refresh the package feeds.</td>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("no-results state missing %q", want)
		}
	}
}

// TestAvailableNoResultsQuotesTheQueryLiterally: the empty state's body is
// Markdown prose, and what was typed into the search box is not prose — it is
// the operator's own string, quoted back. It reads as itself: no emphasis, no
// link, whatever punctuation Markdown would have claimed.
func TestAvailableNoResultsQuotesTheQueryLiterally(t *testing.T) {
	s := pluginsServer(t, fakeBackend{access: true}, true, mgmtManifest())
	body := get(t, s, "/system/packages/discover?q="+url.QueryEscape("*luci*_x_ http://evil.example")).Body.String()
	if !strings.Contains(body, "*luci*_x_ http://evil.example") {
		t.Errorf("the query must read back exactly as typed: %s", body)
	}
	if strings.Contains(body, "<em>luci</em>") || strings.Contains(body, `<a href="http://evil.example"`) {
		t.Errorf("a search box is not a Markdown editor: %s", body)
	}
}

// TestDiscoverInstallRunsAndRescans: Install posts the exact package to the
// backend and re-reads the manifest set (ADR-011 §7), then redirects back to
// the same search.
func TestDiscoverInstallRunsAndRescans(t *testing.T) {
	var installs []string
	s := pluginsServer(t, fakeBackend{access: true, pkgInstalls: &installs}, true, mgmtManifest())
	rescanned := false
	s.SetRescan(func() []plugin.Manifest { rescanned = true; return []plugin.Manifest{mgmtManifest()} })

	rec := postPlugin(t, s, "/system/packages/discover", url.Values{
		"package": {"htop"}, "_primary": {"install"}, "q": {"htop"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d: %s", rec.Code, rec.Body.String())
	}
	if loc := rec.Header().Get("Location"); loc != "/system/packages/discover?q=htop" {
		t.Fatalf("redirect must keep the search, got %q", loc)
	}
	if len(installs) != 1 || installs[0] != "htop" || !rescanned {
		t.Fatalf("installs=%v rescanned=%v", installs, rescanned)
	}
}

// refreshBackend holds PkgUpdate open, so a test can read the page while a feed
// refresh is still running and count how many runs the shell actually started.
type refreshBackend struct {
	fakeBackend
	calls   *atomic.Int32
	guarded *atomic.Int32 // guarded verbs the shell attempted mid-refresh
	started chan struct{}
	release chan error
	blocked chan struct{} // never delivered: the queue behind the package guard
}

func (b refreshBackend) PkgUpdate(context.Context, string) error {
	b.calls.Add(1)
	b.started <- struct{}{}
	return <-b.release
}

// The guarded verbs model verso-rpcd's package mutex: while a refresh holds it,
// any of them would queue behind apk for as long as the refresh runs. Here they
// never come back, so a shell that calls one freezes the test the way it froze
// the browser.
func (b refreshBackend) PkgInstalled(ctx context.Context, sid string) ([]openwrt.Package, error) {
	b.waitOnTheGuard()
	return b.fakeBackend.PkgInstalled(ctx, sid)
}

func (b refreshBackend) PkgSearch(ctx context.Context, sid, query string) ([]openwrt.Package, int, error) {
	b.waitOnTheGuard()
	return b.fakeBackend.PkgSearch(ctx, sid, query)
}

func (b refreshBackend) PkgInstall(ctx context.Context, sid, name string) error {
	b.waitOnTheGuard()
	return b.fakeBackend.PkgInstall(ctx, sid, name)
}

func (b refreshBackend) PkgRemove(ctx context.Context, sid, name string) error {
	b.waitOnTheGuard()
	return b.fakeBackend.PkgRemove(ctx, sid, name)
}

func (b refreshBackend) waitOnTheGuard() {
	if feedRefresh.running() {
		b.guarded.Add(1)
		<-b.blocked
	}
}

func newRefreshBackend() refreshBackend {
	return refreshBackend{
		fakeBackend: fakeBackend{access: true, pkgCheckedAt: 1},
		calls:       &atomic.Int32{},
		guarded:     &atomic.Int32{},
		started:     make(chan struct{}, 4),
		release:     make(chan error, 4),
		blocked:     make(chan struct{}),
	}
}

// refreshServer serves the Available face over a blocking backend. The refresh
// job is device-wide state, so every test starts it from idle.
func refreshServer(t *testing.T, b openwrt.Backend) *Server {
	t.Helper()
	feedRefresh.mu.Lock()
	feedRefresh.active, feedRefresh.failure = false, nil
	feedRefresh.mu.Unlock()
	s := newServerWith(t, b, &fakeTransport{}, []plugin.Manifest{mgmtManifest()})
	s.probe = func(string) bool { return true }
	return s
}

func waitFeedRefreshIdle(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for feedRefresh.running() {
		if time.Now().After(deadline) {
			t.Fatal("the background refresh never finished")
		}
		time.Sleep(time.Millisecond)
	}
}

// renderWithin fails the test if a page does not answer inside d — the freeze a
// guarded helper call causes while apk belongs to the refresh.
func renderWithin(t *testing.T, s *Server, path string, d time.Duration) string {
	t.Helper()
	rendered := make(chan string, 1)
	go func() { rendered <- get(t, s, path).Body.String() }()
	select {
	case body := <-rendered:
		return body
	case <-time.After(d):
		t.Fatalf("%s did not render while a feed refresh was running", path)
		return ""
	}
}

// TestPackagePagesRenderWhileTheFeedsRefresh: apk belongs to the refresh, so the
// pages ask the helper for nothing and say so. Every guarded verb in this
// backend blocks forever, so a page that still calls one never answers.
func TestPackagePagesRenderWhileTheFeedsRefresh(t *testing.T) {
	b := newRefreshBackend()
	s := refreshServer(t, b)

	postPlugin(t, s, "/system/packages/discover", url.Values{"_action": {"refresh"}})
	<-b.started

	for _, path := range []string{
		"/system/packages",
		"/system/packages/discover",
		"/system/packages/discover?q=htop",
		"/system/services",
	} {
		body := renderWithin(t, s, path, 5*time.Second)
		if path == "/system/services" {
			continue // the roster is procd's; only its package-ownership read is guarded
		}
		if !strings.Contains(body, "Refreshing the package feeds") {
			t.Errorf("%s must say what the device is doing instead of listing", path)
		}
		// The busy button says it is refreshing; the note beside it says
		// nothing until the index has an age again.
		if path == "/system/packages" && !strings.Contains(body, `<span data-package-note class="text-sm text-meta" aria-live="polite"></span>`) {
			t.Errorf("%s repeats the refresh beside its busy button", path)
		}
	}
	if n := b.guarded.Load(); n != 0 {
		t.Fatalf("%d guarded helper calls made while the refresh held apk", n)
	}

	b.release <- nil
	waitFeedRefreshIdle(t)
	if body := get(t, s, "/system/packages").Body.String(); strings.Contains(body, "Refreshing the package feeds") {
		t.Error("the inventory must come back once the refresh is done")
	}
}

// TestInstallWaitsForTheRefresh: installing would queue behind the same guard,
// so the click is refused with a reason rather than holding the browser.
func TestInstallWaitsForTheRefresh(t *testing.T) {
	b := newRefreshBackend()
	s := refreshServer(t, b)

	token := s.sessions.CreateWithMetadata("test-sid", "root", "", "")
	sess, _ := s.sessions.get(token)
	do := func(path string, form url.Values) *httptest.ResponseRecorder {
		var req *http.Request
		if form != nil {
			form.Set("_csrf", sess.csrf)
			req = httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		} else {
			req = httptest.NewRequest(http.MethodGet, path, nil)
		}
		req.AddCookie(&http.Cookie{Name: sessionCookie, Value: token})
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, req)
		return rec
	}

	do("/system/packages/discover", url.Values{"_action": {"refresh"}})
	<-b.started

	if rec := do("/system/packages/discover", url.Values{"package": {"htop"}, "_primary": {"install"}}); rec.Code != http.StatusSeeOther {
		t.Fatalf("a refused install still answers at once, got %d", rec.Code)
	}
	if rec := do("/system/packages", url.Values{"package": {"htop"}, "_primary": {"remove"}}); rec.Code != http.StatusSeeOther {
		t.Fatalf("a refused remove still answers at once, got %d", rec.Code)
	}
	if body := do("/system/packages", nil).Body.String(); !strings.Contains(body, "The feeds are being refreshed — try again in a moment.") {
		t.Error("a refused package action must say why")
	}
	if n := b.guarded.Load(); n != 0 {
		t.Fatalf("%d guarded helper calls made while the refresh held apk", n)
	}

	b.release <- nil
	waitFeedRefreshIdle(t)
}

// TestFeedRefreshRunsInTheBackground: the POST answers while apk is still
// working, the page states the running act instead of offering it again, and the
// next visit after it finishes is back to normal.
func TestFeedRefreshRunsInTheBackground(t *testing.T) {
	b := newRefreshBackend()
	s := refreshServer(t, b)

	rec := postPlugin(t, s, "/system/packages/discover", url.Values{"_action": {"refresh"}, "q": {"htop"}})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("refresh must answer at once, got %d", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/system/packages/discover?q=htop" {
		t.Fatalf("redirect must keep the search, got %q", loc)
	}
	<-b.started // the helper call is still open: the browser was not held for it

	body := get(t, s, "/system/packages/discover").Body.String()
	if !strings.Contains(body, "Refreshing the feeds") {
		t.Error("a running refresh must be stated on the page")
	}
	if strings.Contains(body, ">Refresh index</button>") {
		t.Error("a refresh under way must not offer itself again")
	}

	b.release <- nil
	waitFeedRefreshIdle(t)

	body = get(t, s, "/system/packages/discover").Body.String()
	if !strings.Contains(body, ">Refresh index</button>") {
		t.Error("the finished refresh must return the button")
	}
	if strings.Contains(body, "Refreshing the feeds") {
		t.Error("a finished refresh must not still read as running")
	}
	if calls := b.calls.Load(); calls != 1 {
		t.Fatalf("one click, one apk update; got %d", calls)
	}
}

// TestFeedRefreshRefusesASecondRun: the index is one file set device-wide, so a
// click landing on a running refresh changes nothing and says so.
func TestFeedRefreshRefusesASecondRun(t *testing.T) {
	b := newRefreshBackend()
	s := refreshServer(t, b)

	token := s.sessions.CreateWithMetadata("test-sid", "root", "", "")
	sess, _ := s.sessions.get(token)
	do := func(method string, form url.Values) *httptest.ResponseRecorder {
		var req *http.Request
		if form != nil {
			form.Set("_csrf", sess.csrf)
			req = httptest.NewRequest(method, "/system/packages/discover", strings.NewReader(form.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		} else {
			req = httptest.NewRequest(method, "/system/packages/discover", nil)
		}
		req.AddCookie(&http.Cookie{Name: sessionCookie, Value: token})
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, req)
		return rec
	}

	do(http.MethodPost, url.Values{"_action": {"refresh"}})
	<-b.started
	if rec := do(http.MethodPost, url.Values{"_action": {"refresh"}}); rec.Code != http.StatusSeeOther {
		t.Fatalf("a refused refresh still answers with the page, got %d", rec.Code)
	}
	if body := do(http.MethodGet, nil).Body.String(); !strings.Contains(body, "The feeds are already being refreshed.") {
		t.Error("the second click must be told why nothing happened")
	}

	b.release <- nil
	waitFeedRefreshIdle(t)
	if calls := b.calls.Load(); calls != 1 {
		t.Fatalf("the second click must start no second run; got %d", calls)
	}
}

// TestFeedRefreshFailureIsStatedOnce: nobody is holding the page when the run
// ends, so its failure waits for the next visit — and is gone after it.
func TestFeedRefreshFailureIsStatedOnce(t *testing.T) {
	b := newRefreshBackend()
	s := refreshServer(t, b)

	postPlugin(t, s, "/system/packages/discover", url.Values{"_action": {"refresh"}})
	<-b.started
	b.release <- errors.New("no route to the feed")
	waitFeedRefreshIdle(t)

	body := get(t, s, "/system/packages/discover").Body.String()
	for _, want := range []string{"Feed refresh failed", "no route to the feed"} {
		if !strings.Contains(body, want) {
			t.Errorf("the failed refresh must be stated, missing %q", want)
		}
	}
	if body := get(t, s, "/system/packages/discover").Body.String(); strings.Contains(body, "Feed refresh failed") {
		t.Error("a stated failure is not repeated on every later visit")
	}
}

// TestDiscoverRefusesBadNames: a package name outside the exact alphabet is
// refused before the bus.
func TestDiscoverRefusesBadNames(t *testing.T) {
	var installs []string
	s := pluginsServer(t, fakeBackend{access: true, pkgInstalls: &installs}, true, mgmtManifest())
	rec := postPlugin(t, s, "/system/packages/discover", url.Values{
		"package": {"htop;reboot"}, "_primary": {"install"},
	})
	if rec.Code != http.StatusBadRequest || len(installs) != 0 {
		t.Fatalf("code=%d installs=%v — bad names must be refused", rec.Code, installs)
	}
}
