// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"net/http"
	"strings"
	"testing"

	"github.com/we-are-mono/verso/internal/openwrt"
)

// A package opened from another page (DNS & DHCP offering https-dns-proxy)
// answers with its own panel — the same drawer Packages opens from its row:
// what it is, its facts, and Install — and nothing of the Packages page.
func TestPackagePanelIsThePackagesOwnDrawer(t *testing.T) {
	s := pluginsServer(t, fakeBackend{access: true, pkgFound: []openwrt.Package{
		{Name: "https-dns-proxy-luci", Description: "LuCI support"},
		{Name: "https-dns-proxy", Description: "DNS over HTTPS proxy", Version: "2025.1", Feed: "packages", Size: 18432},
	}}, true)
	res := getPanel(t, s, "/system/packages/package?name=https-dns-proxy")
	body := res.Body.String()
	if res.Code != http.StatusOK || strings.Contains(body, "<main") {
		t.Fatalf("want the panel alone, got %d:\n%s", res.Code, body)
	}
	for _, want := range []string{"https-dns-proxy", "DNS over HTTPS proxy", "2025.1", `name="package" value="https-dns-proxy"`, `value="install"`, ">Install<"} {
		if !strings.Contains(body, want) {
			t.Errorf("panel missing %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "LuCI support") {
		t.Errorf("the panel is the named package only, not every search match:\n%s", body)
	}
}

// Asked for as a page, the package is found on Packages instead; a name the
// feeds do not carry says so in the panel; a malformed name is refused.
func TestPackagePanelOutsideAPanel(t *testing.T) {
	s := pluginsServer(t, fakeBackend{access: true}, true)
	if res := get(t, s, "/system/packages/package?name=adblock"); res.Code != http.StatusSeeOther || res.Header().Get("Location") != "/system/packages?tab=all&q=adblock" {
		t.Errorf("as a page: %d → %q, want 303 → Packages with the package found", res.Code, res.Header().Get("Location"))
	}
	if res := getPanel(t, s, "/system/packages/package?name=adblock"); res.Code != http.StatusNotFound || !strings.Contains(res.Body.String(), "adblock") {
		t.Errorf("a package not in the feeds: %d\n%s", res.Code, res.Body.String())
	}
	if res := getPanel(t, s, "/system/packages/package?name=../etc"); res.Code != http.StatusBadRequest {
		t.Errorf("a malformed name: %d", res.Code)
	}
}

// A search that arrives without a view searches everything: a link to a
// package that is not installed yet must find it, not an empty Installed list.
func TestPackagesSearchArrivingWithoutAViewSearchesAll(t *testing.T) {
	s := pluginsServer(t, fakeBackend{access: true, pkgTotal: 1, pkgFound: []openwrt.Package{{Name: "adblock", Description: "Blocklist"}}}, true)
	body := get(t, s, "/system/packages?q=adblock").Body.String()
	if !strings.Contains(body, `data-package-all="true"`) || !strings.Contains(body, "adblock") {
		t.Errorf("a search without a view lists all packages:\n%s", body)
	}
}
