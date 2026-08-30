// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/we-are-mono/verso/internal/openwrt"
)

// TestServicesTable: procd's whole table renders flush-edged (not striped), all
// facts as columns, no drawers — service, providing package, state, boot as
// a checkmark, switch; the keep-list shows state but no switch.
func TestServicesTable(t *testing.T) {
	b := fakeBackend{access: true,
		rcStates: map[string]openwrt.RCState{
			"dnsmasq":           {Enabled: true, Running: true},
			"cron":              {Enabled: true, Running: false},
			"verso":             {Enabled: true, Running: true},
			"verso-rpcd":        {Enabled: true, Running: false}, // helper round-trip corrects stale rc state
			"verso-plugin-demo": {Enabled: true, Running: true},
		},
		pkgInstalledList: []openwrt.Package{
			{Name: "dnsmasq", Version: "2.91-r1", Feed: "base", Installed: true, Services: []string{"dnsmasq"}},
			{Name: "verso", Version: "0.0.11-r1", Feed: "mono", Installed: true, Services: []string{"verso", "verso-rpcd"}},
		},
	}
	s := pluginsServer(t, b, true, mgmtManifest())

	body := get(t, s, "/system/services").Body.String()
	for _, want := range []string{
		`name="svc:dnsmasq"`,                // a plain service's switch
		">dnsmasq</td>",                     // …and its providing package in the Package column
		`name="on:demo"`,                    // the plugin's switch, manifest-addressed
		"text-green-600",                    // the boot checkmark
		"running",                           // the state pill
		">Enabled</th>",                     // the switch column names the action
		">verso</td>",                       // APK ownership joins verso-rpcd to the verso package
		"font-mono text-base font-semibold", // package ownership uses fixed 16px/600 mono type
		"max-w-4xl",                         // the service inventory uses the focused content width
	} {
		if !strings.Contains(body, want) {
			t.Errorf("services missing %q", want)
		}
	}
	if strings.Contains(body, `name="svc:verso"`) {
		t.Error("the keep-list service must not offer its own off switch")
	}
	if strings.Contains(body, `name="svc:verso-rpcd"`) {
		t.Error("Verso's required privileged companion must not offer an off switch")
	}
	if strings.Contains(body, "odd:bg-slate-50") {
		t.Error("the services table is flat, never striped")
	}
	if strings.Contains(body, "[&_td:first-child]:pl-3") || strings.Contains(body, "[&_td:last-child]:pr-3") {
		t.Error("the services table must align its outside columns with the content edges")
	}
	if strings.Contains(body, "Monitor — ") || strings.Contains(body, "Service — ") {
		t.Error("the services table carries no drawers")
	}
	nameAt := strings.Index(body, ">verso-rpcd<")
	if nameAt < 0 {
		t.Fatal("verso-rpcd row missing")
	}
	rowAt := strings.LastIndex(body[:nameAt], "<tr")
	if rowAt < 0 {
		t.Fatal("verso-rpcd table row start missing")
	}
	rowEnd := strings.Index(body[rowAt:], "</tr>")
	if rowEnd < 0 || !strings.Contains(body[rowAt:rowAt+rowEnd], "running") || strings.Contains(body[rowAt:rowAt+rowEnd], "stopped") {
		t.Errorf("successful helper call must show verso-rpcd running: %s", body[rowAt:])
	}
}

func TestPackageOfUsesAPKOwnership(t *testing.T) {
	owners := map[string]string{"sysntpd": "busybox", "verso-rpcd": "verso"}
	for service, want := range map[string]string{
		"sysntpd": "busybox", "verso-rpcd": "verso", "verso-plugin-demo": "verso-plugin-demo", "unmanaged": "—",
	} {
		if got := packageOf(service, owners); got != want {
			t.Errorf("packageOf(%q) = %q, want %q", service, got, want)
		}
	}
}

// TestServicesStatePills: a plugin service that procd runs but whose socket
// is dead reads "not responding"; unmanaged reads "not managed".
func TestServicesStatePills(t *testing.T) {
	cases := []struct {
		name  string
		rc    map[string]openwrt.RCState
		alive bool
		want  string
	}{
		{"dead socket", map[string]openwrt.RCState{"verso-plugin-demo": {Enabled: true, Running: true}}, false, "not responding"},
		{"stopped", map[string]openwrt.RCState{"verso-plugin-demo": {Enabled: false, Running: false}}, true, "stopped"},
		{"unmanaged", map[string]openwrt.RCState{}, true, "not managed"},
	}
	for _, tc := range cases {
		s := pluginsServer(t, fakeBackend{access: true, rcStates: tc.rc}, tc.alive, mgmtManifest())
		if body := get(t, s, "/system/services").Body.String(); !strings.Contains(body, tc.want) {
			t.Errorf("%s: missing %q", tc.name, tc.want)
		}
	}
}

// TestServicesSwitchCouplesBothFacts: off is stop+disable, on is enable+start
// — one human concept, both procd facts — for plain and plugin services alike.
func TestServicesSwitchCouplesBothFacts(t *testing.T) {
	var inits []string
	s := pluginsServer(t, fakeBackend{access: true, rcInits: &inits}, true, mgmtManifest())

	rec := postPlugin(t, s, "/system/services", url.Values{"svc:dnsmasq": {"off"}})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d: %s", rec.Code, rec.Body.String())
	}
	postPlugin(t, s, "/system/services", url.Values{"on:demo": {"on"}})
	want := []string{"dnsmasq stop", "dnsmasq disable",
		"verso-plugin-demo enable", "verso-plugin-demo start"}
	if len(inits) != len(want) {
		t.Fatalf("rc actions = %v, want %v", inits, want)
	}
	for i := range want {
		if inits[i] != want[i] {
			t.Fatalf("rc actions = %v, want %v", inits, want)
		}
	}
}

// TestServicesRefusals: the keep-list never reaches procd; unknown verbs and
// bad names are refused.
func TestServicesRefusals(t *testing.T) {
	var inits []string
	s := pluginsServer(t, fakeBackend{access: true, rcInits: &inits}, true, mgmtManifest())

	if rec := postPlugin(t, s, "/system/services", url.Values{"service": {"verso"}, "_primary": {"restart"}}); rec.Code != http.StatusOK {
		t.Errorf("keep-list restart should re-render the refusal, got %d", rec.Code)
	}
	if rec := postPlugin(t, s, "/system/services", url.Values{"service": {"verso-rpcd"}, "_primary": {"stop"}}); rec.Code != http.StatusOK {
		t.Errorf("verso-rpcd stop should be refused, got %d", rec.Code)
	}
	if rec := postPlugin(t, s, "/system/services", url.Values{"service": {"dnsmasq"}, "_action": {"reboot-the-moon"}}); rec.Code != http.StatusBadRequest {
		t.Errorf("unknown verb: got %d, want 400", rec.Code)
	}
	if rec := postPlugin(t, s, "/system/services", url.Values{"service": {"../etc"}, "_primary": {"restart"}}); rec.Code != http.StatusBadRequest {
		t.Errorf("bad name: got %d, want 400", rec.Code)
	}
	if len(inits) != 0 {
		t.Fatalf("refused actions must not reach procd: %v", inits)
	}
}

// TestServicesRCFailureDegrades: an rc read failure names itself instead of a
// blank page.
func TestServicesRCFailureDegrades(t *testing.T) {
	s := pluginsServer(t, fakeBackend{access: true, rcErr: openwrt.ErrAccessDenied}, true, mgmtManifest())
	if body := get(t, s, "/system/services").Body.String(); !strings.Contains(body, "Service table unavailable") {
		t.Error("rc failure should surface as a callout")
	}
}
