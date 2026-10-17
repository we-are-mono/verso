// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"context"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/we-are-mono/verso/internal/openwrt"
	"github.com/we-are-mono/verso/internal/widget"
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
		`value="stop:dnsmasq"`,           // live stop leaves boot policy alone
		">dnsmasq</span>",                // …and its providing package in the Package column
		`value="stop:verso-plugin-demo"`, // the plugin's live action
		"6\U0000202fh 14\U0000202fmin",   // process age belongs with live state
		">PID</th>",                      // process facts share one compact column
		"2.3\U0000202fMiB",               // runtime keeps process identity and aggregate RSS
		`<td data-verso-figures class="verso-table__cell whitespace-nowrap text-meta">`, // runtime matches the landing-page RX/TX ink, in figures
		">Memory</th>",                 // immediate restart sits before the switch
		`aria-label="Restart dnsmasq"`, // the icon-only action names the service it restarts
		`name="_service_action" value="restart:dnsmasq"`,
		">State</th>",  // the switch names and reflects persistent boot policy
		"startup task", // lifecycle type is carried beside the service name
		"subsystem",
		"runs at boot", // one-shot tasks are not misreported as stopped
		`<span class="verso-chip" data-verso-verbatim>`, // type reuses the mono chip
		">verso</span>", // APK ownership joins verso-rpcd to the verso package
		// package ownership is verbatim: the cell's role in its fixed 16px mono form
		`<td data-verso-verbatim class="verso-table__cell whitespace-nowrap text-ink"><span class="flex items-center"><span>verso</span>`,
		"max-w-column-6xl", // runtime fits without forcing a horizontal scroller
	} {
		if !strings.Contains(body, want) {
			t.Errorf("services missing %q", want)
		}
	}
	// A still listing is scrolled and found in with the browser's own find,
	// so the services stand under the heading with no control band.
	if strings.Contains(body, "<div data-verso-actionbar") {
		t.Error("the services listing should carry no control band")
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
	// No state, no PID, no memory (nor order or package here): each the one
	// absence mark, the faint dash in the sans, never a hollow packet before it
	// or a minus in mono.
	if got := strings.Count(firewallRow, `<td class="verso-table__cell whitespace-nowrap text-faint"><span aria-hidden="true">—</span><span class="sr-only">None</span></td>`); got < 3 {
		t.Errorf("an indeterminate State, PID and Memory must each be the absence mark (got %d): %s", got, firewallRow)
	}
	// The state it does state, applied at boot, is no live state: the empty
	// ring, which never stands before a dash.
	if !regexp.MustCompile(`class="verso-mark" data-verso-tone="[a-z]*"></span>\s*<span class="text-meta">applied at boot<`).MatchString(firewallRow) {
		t.Errorf("applied at boot should wear the empty ring: %s", firewallRow)
	}
	if regexp.MustCompile(`border border-faint"></span>\s*<span[^>]*>—`).MatchString(firewallRow) {
		t.Errorf("a hollow packet stands before an absence: %s", firewallRow)
	}
	if strings.Contains(firewallRow, `name="svc:firewall"`) {
		t.Errorf("firewall must not offer an enabled toggle: %s", firewallRow)
	}
	// A subsystem says what it did: active where its script says so, applied
	// at boot where it is enabled, and disabled otherwise — never a dash.
	for _, tc := range []struct {
		running, enabled bool
		want             string
	}{{true, true, "active"}, {false, true, "applied at boot"}, {false, false, "disabled"}} {
		if got := subsystemStateCell(tc.running, tc.enabled).Text; got != tc.want {
			t.Errorf("subsystem running=%v enabled=%v reads %q, want %q", tc.running, tc.enabled, got, tc.want)
		}
	}
	// Nothing runs, so nothing starts or stops: the firewall offers only
	// reloading its rules, which can drift from the config.
	if !strings.Contains(firewallRow, `aria-label="Reload rules firewall"`) || !strings.Contains(firewallRow, `value="reload:firewall"`) {
		t.Errorf("firewall must offer reloading its rules: %s", firewallRow)
	}
	for _, never := range []string{`aria-label="Restart firewall"`, `aria-label="Cannot be stopped from here"`, `aria-label="Stop firewall"`, `aria-label="Start firewall"`} {
		if strings.Contains(firewallRow, never) {
			t.Errorf("a subsystem offers nothing to start, stop or restart (%s): %s", never, firewallRow)
		}
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

// Only a state asserted now fills its packet. What a task or a subsystem is set
// to do at boot, or that it is off, is no live state: the empty ring and the
// word at the secondary step, as Stopped reads, never Denim, the hue of acts.
func TestServicesBootStatesWearTheRing(t *testing.T) {
	for _, row := range []widget.TableRow{
		serviceRow("boot", openwrt.RCState{Kind: openwrt.ServiceTask, Enabled: true}, nil),
		serviceRow("umount", openwrt.RCState{Kind: openwrt.ServiceTask}, nil),
		serviceRow("system", openwrt.RCState{Kind: openwrt.ServiceSubsystem, Enabled: true}, nil),
		serviceRow("gpio_switch", openwrt.RCState{Kind: openwrt.ServiceSubsystem}, nil),
		pluginServiceRow(pluginState{Service: "verso-plugin-demo", Known: true, Enabled: true, Kind: openwrt.ServiceTask}, nil),
	} {
		if state := row.Cells[5]; state.Variant != "" {
			t.Errorf("%s %q wears %q, want the ring", row.ID, state.Text, state.Variant)
		}
	}
}

// A daemon procd stopped restarting reads failed, in danger, never the calm
// "Stopped" an operator's stop leaves; its row still offers Start.
func TestServicesFailedDaemonReadsFailed(t *testing.T) {
	failed := openwrt.RCState{Enabled: true, Failed: true, Kind: openwrt.ServiceDaemon}
	for _, row := range []widget.TableRow{
		serviceRow("sysntpd", failed, nil),
		pluginServiceRow(pluginState{Service: "verso-plugin-demo", Known: true, Enabled: true, Failed: true, Kind: openwrt.ServiceDaemon}, nil),
	} {
		if state := row.Cells[5]; state.Text != "Failed" || state.Variant != "danger" {
			t.Errorf("%s state = %q/%q, want Failed/danger", row.ID, state.Text, state.Variant)
		}
		// Why it failed is in its log, so the state leads to the log's lines
		// about it.
		if state := row.Cells[5]; state.Href != "/system/logs?q="+row.ID {
			t.Errorf("%s state leads to %q, want its log", row.ID, state.Href)
		}
		if acts := row.Cells[6].Actions; len(acts) == 0 || acts[len(acts)-1].Title != "Start" {
			t.Errorf("%s acts = %+v, want Start last", row.ID, acts)
		}
	}
}

// TestServicesRefusals: the keep-list never reaches procd; unknown verbs and
// bad names are refused.
func TestServicesRefusals(t *testing.T) {
	var inits []string
	s := pluginsServer(t, fakeBackend{access: true, rcInits: &inits}, true, mgmtManifest())

	if rec := postPlugin(t, s, "/system/services", url.Values{"_service_action": {"restart:verso"}}); rec.Code != http.StatusOK {
		t.Errorf("keep-list restart should re-render the refusal, got %d", rec.Code)
	}
	if rec := postPlugin(t, s, "/system/services", url.Values{"_service_action": {"stop:verso-rpcd"}}); rec.Code != http.StatusOK {
		t.Errorf("verso-rpcd stop should be refused, got %d", rec.Code)
	}
	if rec := postPlugin(t, s, "/system/services", url.Values{"_service_action": {"stop:firewall"}}); rec.Code != http.StatusOK {
		t.Errorf("firewall stop should be refused, got %d", rec.Code)
	}
	for _, verb := range []string{"reboot-the-moon", "enable", "disable"} {
		if rec := postPlugin(t, s, "/system/services", url.Values{"_service_action": {verb + ":dnsmasq"}}); rec.Code != http.StatusBadRequest {
			t.Errorf("unknown verb %q: got %d, want 400", verb, rec.Code)
		}
	}
	if rec := postPlugin(t, s, "/system/services", url.Values{"_service_action": {"restart:../etc"}}); rec.Code != http.StatusBadRequest {
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
