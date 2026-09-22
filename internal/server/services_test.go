// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/we-are-mono/verso/internal/openwrt"
)

type serviceRuntimeBackend struct {
	fakeBackend
	inventoryReads int
	runtimeReads   int
}

func (b *serviceRuntimeBackend) PkgInstalled(ctx context.Context, sid string) ([]openwrt.Package, error) {
	b.inventoryReads++
	return b.fakeBackend.PkgInstalled(ctx, sid)
}

func (b *serviceRuntimeBackend) RCList(ctx context.Context, sid string) (map[string]openwrt.RCState, error) {
	b.runtimeReads++
	return b.fakeBackend.RCList(ctx, sid)
}

func TestServiceActionReadsOnlyRuntimeAndReturnsAffectedCells(t *testing.T) {
	var calls []string
	b := &serviceRuntimeBackend{fakeBackend: fakeBackend{access: true, rcInits: &calls,
		rcStates: map[string]openwrt.RCState{"dnsmasq": {Kind: openwrt.ServiceDaemon, Running: true, PIDs: []int{42}, MemoryBytes: 2048}}}}
	s := newServer(t, b)
	res := postPluginAs(t, s, "/system/services", url.Values{"_service_action": {"restart:dnsmasq"}}, "act")
	if res.Code != 200 || b.inventoryReads != 0 || b.runtimeReads != 1 || len(calls) != 1 {
		t.Fatalf("status %d, reads %d/%d, calls %v", res.Code, b.inventoryReads, b.runtimeReads, calls)
	}
	if strings.Contains(res.Body.String(), "<main") || !strings.Contains(res.Body.String(), `data-verso-row-patch="3,4,5,6"`) {
		t.Fatalf("not a runtime patch: %s", res.Body.String())
	}
	res = postPluginAs(t, s, "/system/services", url.Values{"_service_action": {"stop:firewall"}}, "act")
	if res.Code != http.StatusConflict || len(calls) != 1 || b.inventoryReads != 0 || b.runtimeReads != 1 {
		t.Fatalf("refused act fetched or mutated: %d %v", res.Code, calls)
	}
}

// TestServicesTable: procd's whole table renders flush-edged (not striped), all
// facts as columns, no drawers — service/type, providing package, state,
// runtime, restart and enabled switch; tasks and the keep-list have no switch.
func TestServicesTable(t *testing.T) {
	b := fakeBackend{access: true,
		rcStates: map[string]openwrt.RCState{
			"boot":              {Enabled: true, Kind: openwrt.ServiceTask},
			"dnsmasq":           {Enabled: true, Running: true, Kind: openwrt.ServiceDaemon, PIDs: []int{1842}, MemoryBytes: 2411724, Uptime: 22440},
			"cron":              {Enabled: true, Running: false, Kind: openwrt.ServiceDaemon},
			"firewall":          {Enabled: true, Kind: openwrt.ServiceSubsystem},
			"verso":             {Enabled: true, Running: true, Kind: openwrt.ServiceDaemon},
			"verso-rpcd":        {Enabled: true, Running: false, Kind: openwrt.ServiceDaemon}, // helper round-trip corrects stale rc state
			"verso-plugin-demo": {Enabled: true, Running: true, Kind: openwrt.ServiceDaemon},
		},
		pkgInstalledList: []openwrt.Package{
			{Name: "dnsmasq", Version: "2.91-r1", Feed: "base", Installed: true, Services: []string{"dnsmasq"}},
			{Name: "verso", Version: "0.0.11-r1", Feed: "mono", Installed: true, Services: []string{"verso", "verso-rpcd"}},
		},
	}
	s := pluginsServer(t, b, true, mgmtManifest())

	body := get(t, s, "/system/services").Body.String()
	for _, want := range []string{
		"Find a service",
		`<option value="daemon" data-label="daemon">daemon · `,
		`value="stop:dnsmasq"`,           // live stop leaves boot policy alone
		">dnsmasq</span>",                // …and its providing package in the Package column
		`value="stop:verso-plugin-demo"`, // the plugin's live action
		"6h 14m",                         // process age belongs with live state
		">PID</th>",                      // process facts share one compact column
		"2.3 MiB",                        // runtime keeps process identity and aggregate RSS
		"tabular-nums text-meta",         // runtime matches the landing-page RX/TX ink
		">Memory</th>",                   // immediate restart sits before the switch
		`aria-label="Restart"`,           // the icon-only action remains accessible
		`name="_service_action" value="restart:dnsmasq"`,
		">State</th>",  // the switch names and reflects persistent boot policy
		"startup task", // lifecycle type is carried beside the service name
		"subsystem",
		"runs at boot",                    // one-shot tasks are not misreported as stopped
		"border-rule bg-quiet font-mono",  // type reuses the one chip treatment
		">verso</span>",                   // APK ownership joins verso-rpcd to the verso package
		"font-mono text-base font-medium", // package ownership uses fixed 16px/500 mono type
		"max-w-6xl",                       // runtime fits without forcing a horizontal scroller
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
	if strings.Contains(body, `name="svc:boot"`) {
		t.Error("a completed startup task must not offer a meaningless on/off switch")
	}
	if strings.Contains(body, ">Starts at boot</th>") {
		t.Error("boot policy must not be duplicated beside the Enabled switch")
	}
	if strings.Contains(body, `name="service" value="boot"`) {
		t.Error("a completed startup task must not offer a meaningless restart action")
	}
	for _, service := range []string{"verso", "verso-rpcd"} {
		if strings.Contains(body, `name="service" value="`+service+`"`) {
			t.Errorf("keep-listed service %s must not offer a restart action", service)
		}
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
	firewallAt := strings.Index(body, ">firewall<")
	if firewallAt < 0 {
		t.Fatal("firewall row missing")
	}
	firewallRowAt := strings.LastIndex(body[:firewallAt], "<tr")
	if firewallRowAt < 0 {
		t.Fatal("firewall table row malformed")
	}
	firewallRowEnd := strings.Index(body[firewallRowAt:], "</tr>")
	if firewallRowEnd < 0 {
		t.Fatal("firewall table row malformed")
	}
	firewallRow := body[firewallRowAt : firewallRowAt+firewallRowEnd]
	if strings.Contains(firewallRow, ">Stopped<") {
		t.Errorf("a PID-less subsystem must not be called stopped: %s", firewallRow)
	}
	if !strings.Contains(firewallRow, `text-meta">—</span>`) {
		t.Errorf("an indeterminate State must use the same secondary dash as Runtime: %s", firewallRow)
	}
	if strings.Contains(firewallRow, `name="svc:firewall"`) {
		t.Errorf("firewall must not offer an enabled toggle: %s", firewallRow)
	}
	if !strings.Contains(firewallRow, `aria-label="Cannot be stopped from here"`) {
		t.Errorf("firewall must explain its locked enablement: %s", firewallRow)
	}
	if !strings.Contains(firewallRow, `aria-label="Restart"`) {
		t.Errorf("firewall must remain restartable: %s", firewallRow)
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
	if rowEnd < 0 || !strings.Contains(body[rowAt:rowAt+rowEnd], "Running") || strings.Contains(body[rowAt:rowAt+rowEnd], ">Stopped<") {
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
		{"stopped", map[string]openwrt.RCState{"verso-plugin-demo": {Enabled: false, Running: false}}, true, "Stopped"},
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
	postPlugin(t, s, "/system/services", url.Values{"service": {"dnsmasq"}, "_action": {"restart"}})
	want := []string{"dnsmasq stop", "dnsmasq disable",
		"verso-plugin-demo enable", "verso-plugin-demo start", "dnsmasq restart"}
	if len(inits) != len(want) {
		t.Fatalf("rc actions = %v, want %v", inits, want)
	}
	for i := range want {
		if inits[i] != want[i] {
			t.Fatalf("rc actions = %v, want %v", inits, want)
		}
	}
}

func TestServicesSwitchFetchAvoidsRedirectRender(t *testing.T) {
	var inits []string
	s := pluginsServer(t, fakeBackend{access: true, rcInits: &inits}, true, mgmtManifest())
	token, err := s.sessions.CreateWithMetadata("test-sid", "root", "", "")
	if err != nil {
		t.Fatalf("session: %v", err)
	}
	sess, _ := s.sessions.get(token)
	form := url.Values{"svc:dnsmasq": {"off"}, "_csrf": {sess.csrf}}
	req := httptest.NewRequest(http.MethodPost, "/system/services", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-Verso-Interaction", "switch")
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: token})
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("switch fetch status = %d, want 204: %s", rec.Code, rec.Body.String())
	}
	want := []string{"dnsmasq stop", "dnsmasq disable"}
	if len(inits) != len(want) || inits[0] != want[0] || inits[1] != want[1] {
		t.Fatalf("rc actions = %v, want %v", inits, want)
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
	if rec := postPlugin(t, s, "/system/services", url.Values{"svc:firewall": {"off"}}); rec.Code != http.StatusOK {
		t.Errorf("firewall disable should be refused, got %d", rec.Code)
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
