// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

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
		"htop", "3.5.1-r1", "packages", // the row
		"font-mono text-base font-semibold",             // package versions use the fixed 16px/600 mono treatment
		"Process viewer", "GPL-2.0", ">Remove</button>", // the drawer's story and act
		"max-w-4xl", // package management uses the focused content width
		"border-slate-200 bg-slate-50 text-slate-700",                       // description uses the neutral callout
		`href="https://htop.dev" target="_blank" rel="noopener noreferrer"`, // project link stays in the callout and opens safely outside Verso
		"space-y-0", "border-t border-slate-100 py-3", // facts match the Overview System DL
		`<header class="flex shrink-0 items-center justify-between px-5 pt-4 pb-2">`, // modal-like header, no divider
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
	} else if !strings.Contains(body, "dark:border-green-500/15 dark:bg-green-500/10 dark:text-green-300") {
		t.Error("success flashes should match the verified modal's dark success palette")
	}
	if body := do(http.MethodGet, "/system/packages", nil).Body.String(); strings.Contains(body, "htop removed.") {
		t.Error("a flash shows once, not twice")
	}
}

// TestDiscoverSearchRenders: the Discover face lists the helper's matches with
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
		t.Error("Discover must use the narrow package-management width")
	}
	if !strings.Contains(body, "w-52") {
		t.Error("Discover's search field should use the compact inline width")
	}
	for _, want := range []string{
		"htop", "Process viewer", "3.5.1-r1", "packages",
		"font-mono text-base font-semibold", // Discover versions match Installed
		"installed",                         // the already-present package carries its state
		"Install — htop",                    // drawer verb for the absent one
		"Remove — htop-lang",                // drawer verb for the present one
		"Feeds checked",                     // freshness honesty
		`href="/system/packages"`,           // the top bar links the faces
	} {
		if !strings.Contains(body, want) {
			t.Errorf("discover missing %q", want)
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
