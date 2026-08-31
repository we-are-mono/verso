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
		">Installed</a>", ">Available</a>",
		"htop", "3.5.1-r1", "packages", // the row
		"font-mono text-base font-semibold",             // package versions use the fixed 16px/600 mono treatment
		"Process viewer", "GPL-2.0", ">Remove</button>", // the drawer's story and act
		"max-w-4xl", // package management uses the focused content width
		"border-slate-200 bg-slate-50 text-slate-700",                       // description uses the neutral callout
		`href="https://htop.dev" target="_blank" rel="noopener noreferrer"`, // project link stays in the callout and opens safely outside Verso
		"space-y-0", "border-t border-slate-100 py-3", // facts match the Overview System DL
		`<header class="flex shrink-0 items-center justify-between px-6 py-4">`, // the shared drawer panel's header, no divider
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
	token, err := s.sessions.CreateWithMetadata("test-sid", "root", "", "")
	if err != nil {
		t.Fatalf("session: %v", err)
	}
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
	} else if !strings.Contains(body, "dark:border-emerald-500/15 dark:bg-emerald-500/10 dark:text-emerald-300") {
		t.Error("success flashes should match the verified modal's dark success palette")
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
	if !strings.Contains(body, "max-w-4xl") {
		t.Error("Available must use the narrow package-management width")
	}
	if !strings.Contains(body, "max-w-sm") {
		t.Error("Available's search field should use the compound search width")
	}
	for _, want := range []string{"[&_input[type=text]]:pr-28", "absolute inset-y-1 right-1", "bg-sky-600"} {
		if !strings.Contains(body, want) {
			t.Errorf("Available's compound search control missing %q", want)
		}
	}
	for _, want := range []string{
		"htop", "Process viewer", "3.5.1-r1", "packages",
		"font-mono text-base font-semibold", // Available versions match Installed
		"installed",                         // the already-present package carries its state
		"Install — htop",                    // drawer verb for the absent one
		"Remove — htop-lang",                // drawer verb for the present one
		"Feeds checked",                     // freshness honesty
		`href="/system/packages"`,           // the local switch links the faces
		`href="/system/packages/discover" aria-current="page"`,
		">Installed</a>", ">Available</a>",
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
	for _, want := range []string{
		"Search available packages",
		"Enter a package name, then select Search. Matching packages will appear here.",
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
	for _, want := range []string{
		"No packages found",
		"No available packages match “missing”. Check the spelling or refresh the package feeds.",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("no-results state missing %q", want)
		}
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
	started chan struct{}
	release chan error
}

func (b refreshBackend) PkgUpdate(context.Context, string) error {
	b.calls.Add(1)
	b.started <- struct{}{}
	return <-b.release
}

func newRefreshBackend() refreshBackend {
	return refreshBackend{
		fakeBackend: fakeBackend{access: true, pkgCheckedAt: 1},
		calls:       &atomic.Int32{},
		started:     make(chan struct{}, 4),
		release:     make(chan error, 4),
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
	if strings.Contains(body, ">Refresh feeds</button>") {
		t.Error("a refresh under way must not offer itself again")
	}

	b.release <- nil
	waitFeedRefreshIdle(t)

	body = get(t, s, "/system/packages/discover").Body.String()
	if !strings.Contains(body, ">Refresh feeds</button>") {
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

	token, err := s.sessions.CreateWithMetadata("test-sid", "root", "", "")
	if err != nil {
		t.Fatalf("session: %v", err)
	}
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
