// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/we-are-mono/verso/internal/openwrt"
	"github.com/we-are-mono/verso/internal/plugin"
	"github.com/we-are-mono/verso/internal/sysstat"
	"github.com/we-are-mono/verso/internal/updatecheck"
)

// nav builds a Server holding manifests and the one device reading the sidebar
// takes, so the sidebar ordering (ADR-009 §2, §5) is unit-testable with no
// transport, backend, or device.
func navServer(manifests ...plugin.Manifest) *Server {
	// Sockets read as alive so every manifest contributes rows; the
	// liveness-hiding test overrides probe itself. The neighbour table answers
	// nothing until a test supplies one, which is the sidebar's own
	// degrade-without-a-count case.
	return &Server{
		manifests: manifests,
		probe:     func(string) bool { return true },
		neighbors: func() ([]sysstat.Neighbor, error) { return nil, errors.New("no neighbour table in tests") },
	}
}

func manifest(id string, entries ...plugin.NavEntry) plugin.Manifest {
	return plugin.Manifest{ID: id, Nav: entries}
}

// sidebar builds one path's rail with no subpages; the rules below describe
// which rows the rail holds and where they lead.
func sidebar(s *Server, active string) navModel {
	return s.buildSidebar(active, identityTranslator,
		func(string) func(string) string { return identityTranslator }, nil)
}

// railRow returns the rail row carrying one label.
func railRow(t *testing.T, model navModel, label string) navRow {
	t.Helper()
	for _, row := range model.Rows {
		if row.Label == label {
			return row
		}
	}
	t.Fatalf("no rail row labelled %q", label)
	return navRow{}
}

// railLabels is the rail's rows in the order it draws them.
func railLabels(model navModel) []string {
	labels := make([]string, len(model.Rows))
	for i, row := range model.Rows {
		labels[i] = row.Label
	}
	return labels
}

func nav(section, label, path string) plugin.NavEntry {
	return plugin.NavEntry{Section: section, Label: label, Path: path}
}

// Destinations the canvas does not name follow the core taxonomy's order of the
// sections they are filed under, no matter what order the plugins are
// discovered in — the whole point of the shell owning the taxonomy — and
// plugin-introduced sections come after every core one, by title. A page filed
// under Network is no row of its own: it is one of the Network row's pages.
func TestBuildSidebarUnnamedRowsFollowSectionOrder(t *testing.T) {
	s := navServer(
		manifest("vpn", nav("VPN", "WireGuard", "/")),
		manifest("guard", nav("Security", "Guard", "/")),
		manifest("links", nav("Network", "Links", "/")),
		manifest("stats", nav("Statistics", "Graphs", "/")),
	)
	want := []string{"Overview", "Devices", "Network", "System", "Guard", "Graphs", "WireGuard"}
	if got := railLabels(sidebar(s, "/")); !slices.Equal(got, want) {
		t.Fatalf("rail = %v, want %v", got, want)
	}
}

// The rail exists with zero plugins: Overview, Devices and the System row,
// which leads into the pages the shell owns.
func TestBuildSidebarShellOwnedRowsAreBuiltIn(t *testing.T) {
	model := sidebar(navServer(), "/")
	if got, want := railLabels(model), []string{"Overview", "Devices", "System"}; !slices.Equal(got, want) {
		t.Fatalf("rail = %v, want %v", got, want)
	}
	if row := railRow(t, model, "Overview"); row.Href != "/" || !row.Active {
		t.Errorf("Overview row = %+v, want the active row leading home", row)
	}
	if row := railRow(t, model, "System"); row.Href != "/system/hardware" || !row.Opens || row.Active {
		t.Errorf("System row = %+v, want an inactive row opening into the shell's first System page", row)
	}
}

// Several plugins filing into one section keep their rows in discovery
// (id-sorted) order; the stable sorts must not disturb within-section order.
func TestBuildSidebarRowsKeepDiscoveryOrder(t *testing.T) {
	s := navServer(
		manifest("alpha", nav("Extra", "Zulu", "/")),
		manifest("beta", nav("Extra", "Alpha", "/")),
	)
	want := []string{"Overview", "Devices", "System", "Zulu", "Alpha"}
	if got := railLabels(sidebar(s, "/")); !slices.Equal(got, want) {
		t.Fatalf("rail = %v, want %v", got, want)
	}
}

// Network and System are each one row, not a group: the row targets the first
// page its section resolves to, opens into its pages, and stays lit anywhere
// inside the section. The pages filed there never stand in the rail themselves.
func TestBuildSidebarCollapsedSectionIsOneRow(t *testing.T) {
	s := navServer(
		manifest("system", nav("System", "General", "/")),
		manifest("dnsdhcp", nav("Network", "DHCP", "/dhcp"), nav("Network", "DNS", "/dns")),
		manifest("interfaces", nav("Network", "Interfaces", "/")),
	)
	for _, tc := range []struct {
		section, at, first string
		members            []string
	}{
		{"System", "/plugins/system/", "/plugins/system/", []string{"General"}},
		{"Network", "/plugins/dnsdhcp/dns", "/plugins/interfaces/", []string{"Interfaces", "DHCP", "DNS"}},
	} {
		model := sidebar(s, tc.at)
		row := railRow(t, model, tc.section)
		if row.Href != tc.first || !row.Active || !row.Opens {
			t.Errorf("%s row = %+v, want the active row opening at %s", tc.section, row, tc.first)
		}
		labels := railLabels(model)
		if n := strings.Count(strings.Join(labels, "\x00"), tc.section); n != 1 {
			t.Errorf("rail = %v, want %s exactly once", labels, tc.section)
		}
		for _, member := range tc.members {
			if slices.Contains(labels, member) {
				t.Errorf("rail = %v: %s belongs under the %s row, not beside it", labels, member, tc.section)
			}
		}
	}
}

// A section with no live page has no row: nothing filed under Network answers,
// so Network leads nowhere and stays out of the rail.
func TestBuildSidebarEmptyCollapsedSectionHasNoRow(t *testing.T) {
	s := navServer(manifest("interfaces", nav("Network", "Interfaces", "/")))
	s.probe = func(string) bool { return false }
	if labels := railLabels(sidebar(s, "/")); slices.Contains(labels, "Network") {
		t.Fatalf("rail = %v, want no Network row without a live Network page", labels)
	}
}

// A row is the same place on every page: its transition name comes from where
// it leads, never from where it stands, so the browser carries the row from the
// page you leave to the page you open (the marker's glide, a branch pushing the
// rows under it down). Names are unique in a rail — one clash and the browser
// drops the whole transition — and plain idents CSS can name.
func TestARailRowIsTheSamePlaceOnEveryPage(t *testing.T) {
	firewall := manifest("firewall", nav("Security", "Firewall", "/"))
	firewall.Socket = "/firewall.sock"
	system := manifest("system", nav("System", "General", "/"))
	system.Socket = "/system.sock"
	s := navServer(firewall, system)

	home, zones := sidebar(s, "/"), sidebar(s, "/plugins/firewall/zones")
	if a, b := railRow(t, home, "Firewall").Transition(), railRow(t, zones, "Firewall").Transition(); a != b || a == "" {
		t.Fatalf("Firewall is %q on one page and %q on another; a place keeps its name", a, b)
	}
	ident := regexp.MustCompile(`^verso-nav-row-[a-z0-9]+(-[a-z0-9]+)*$`)
	seen := map[string]bool{}
	for _, row := range zones.Rows {
		name := row.Transition()
		if !ident.MatchString(name) {
			t.Errorf("%s names itself %q, not a rail ident", row.Label, name)
		}
		if seen[name] {
			t.Errorf("two rows share %q", name)
		}
		seen[name] = true
	}
	// A subpage is named apart from any row: System leads to its first page,
	// and the row and that page must not share a name.
	if sub := (pageTab{Href: railRow(t, home, "System").Href}).Transition(); seen[sub] || !strings.HasPrefix(sub, "verso-nav-sub-") {
		t.Errorf("subpage name %q collides with a row or is not a subpage name", sub)
	}
}

// A row that opens into subpages says so on every page, not only once you are
// in it: Network and System, whose pages the shell gathers, and a plugin's
// destination that declares its pages in its manifest. A row with none says
// nothing.
func TestARowThatOpensSaysSoEverywhere(t *testing.T) {
	firewall := manifest("firewall", plugin.NavEntry{Section: "Security", Label: "Firewall", Path: "/", Pages: true})
	firewall.Socket = "/firewall.sock"
	interfaces := manifest("net", nav("Network", "Interfaces", "/"))
	interfaces.Socket = "/net.sock"
	wireless := manifest("wireless", nav("Wireless", "Wireless", "/"))
	wireless.Socket = "/wireless.sock"
	s := navServer(firewall, interfaces, wireless)

	for _, at := range []string{"/", "/plugins/net/"} {
		model := sidebar(s, at)
		for label, opens := range map[string]bool{"Firewall": true, "System": true, "Network": true, "Wireless": false, "Overview": false} {
			if got := railRow(t, model, label).Opens; got != opens {
				t.Errorf("at %s, %s row Opens = %v, want %v", at, label, got, opens)
			}
		}
	}
}

// A plugin is its own row, at the label the design gives it: no "Security"
// domain row stands in front of the firewall.
func TestBuildSidebarPluginIsItsOwnRow(t *testing.T) {
	firewall := manifest("firewall", nav("Security", "Firewall", "/"))
	firewall.Socket = "/firewall.sock"
	s := navServer(firewall, manifest("net", nav("Network", "Interfaces", "/")))

	model := sidebar(s, "/plugins/firewall/zones")
	row := railRow(t, model, "Firewall")
	if row.Href != "/plugins/firewall/" || !row.Active {
		t.Fatalf("Firewall row = %+v, want the active row targeting the registered page", row)
	}
	for _, label := range railLabels(model) {
		if label == "Security" {
			t.Fatal("the rail names destinations, not the sections they were filed under")
		}
	}
}

// A plugin whose socket does not answer contributes no row — the same rule
// every other registration follows. A dead door is worse than a missing one,
// and the plugin's own URL still answers and explains itself.
func TestBuildSidebarDropsARowWithoutALivePlugin(t *testing.T) {
	firewall := manifest("firewall", nav("Security", "Firewall", "/"))
	firewall.Socket = "/dead/firewall.sock"
	s := navServer(firewall)
	s.probe = func(string) bool { return false }

	for _, label := range railLabels(sidebar(s, "/")) {
		if label == "Firewall" {
			t.Fatal("a plugin that is not answering must not hold a row in the rail")
		}
	}
}

// The rail follows its designed order, whatever order the plugins were
// discovered in. Wi-Fi files under a section of its own, so it keeps its row
// beside Network rather than becoming one of Network's pages.
func TestBuildSidebarFollowsTheDesignedOrder(t *testing.T) {
	dns := manifest("dnsdhcp", nav("Network", "DHCP", "/"))
	dns.Socket = "/dns.sock"
	wireless := manifest("wireless", nav("Wireless", "Wireless", "/"))
	wireless.Socket = "/wireless.sock"
	firewall := manifest("firewall", nav("Security", "Firewall", "/"))
	firewall.Socket = "/firewall.sock"
	// Discovery is id-sorted, so the manifests arrive dnsdhcp, firewall,
	// wireless — nothing like the order the rail must draw them in.
	s := navServer(dns, firewall, wireless)

	want := []string{"Overview", "Devices", "Network", "Wireless", "Firewall", "System"}
	if got := railLabels(sidebar(s, "/")); !slices.Equal(got, want) {
		t.Fatalf("rail = %v, want %v", got, want)
	}
}

// A destination the canvas does not name still appears — after the ones it does,
// so a plugin lands in the rail with no shell change.
func TestBuildSidebarKeepsUnnamedDestinationsLast(t *testing.T) {
	extra := manifest("extra", nav("Extra", "Landing", "/"))
	extra.Socket = "/extra.sock"
	s := navServer(extra)

	labels := railLabels(sidebar(s, "/"))
	if labels[len(labels)-1] != "Landing" {
		t.Fatalf("rail = %v, want the unnamed destination last", labels)
	}
}

// The Devices row leads to the roster page and lights up there. It says nothing
// more: how many devices are about is a reading the homepage and the roster
// give, and a rail word that moves without asking anything teaches the eye to
// pass over the place a word that matters would stand.
func TestBuildSidebarDevicesRowLeadsToTheRoster(t *testing.T) {
	s := navServer()
	s.neighbors = testNeighbors
	row := railRow(t, sidebar(s, devicesPath), "Devices")
	if row.Href != devicesPath || !row.Active {
		t.Fatalf("Devices row = %+v, want the active roster row", row)
	}
	if row.Detail != "" || row.Dot || row.Hint != "" {
		t.Errorf("Devices row = %+v, want no reading in the rail", row)
	}
	if elsewhere := railRow(t, sidebar(s, "/"), "Devices"); elsewhere.Active {
		t.Errorf("Devices row = %+v, want inactive away from the roster", elsewhere)
	}
}

// A rail row speaks only of what waits for you there. System says what is
// ready to install, in words short enough for the rail and whole in its hint:
// new firmware before any package, since a firmware upgrade brings its own
// package versions; nothing at all when nothing waits, or when no check has
// answered.
func TestSystemRowSaysWhatWaitsToInstall(t *testing.T) {
	upgrades := func(n int) []openwrt.PackageUpgrade {
		out := make([]openwrt.PackageUpgrade, n)
		for i := range out {
			out[i] = openwrt.PackageUpgrade{Name: fmt.Sprintf("pkg%d", i), Installed: "1", Available: "2"}
		}
		return out
	}
	firmware := openwrt.FirmwareUpdate{State: openwrt.FirmwareUpdateAvailable}
	for name, tc := range map[string]struct {
		truth        *updatecheck.Truth
		detail, hint string
	}{
		"never checked": {nil, "", ""},
		"up to date":    {&updatecheck.Truth{}, "", ""},
		"one package":   {&updatecheck.Truth{Packages: upgrades(1)}, "1 update", "1 package update is ready to install."},
		"packages":      {&updatecheck.Truth{Packages: upgrades(15)}, "15 updates", "15 package updates are ready to install."},
		"firmware":      {&updatecheck.Truth{Firmware: firmware}, "New firmware", "New firmware is ready to install."},
		"both":          {&updatecheck.Truth{Packages: upgrades(15), Firmware: firmware}, "New firmware", "New firmware is ready to install."},
	} {
		t.Run(name, func(t *testing.T) {
			system := manifest("system", nav("System", "General", "/"))
			system.Socket = "/system.sock"
			s := navServer(system)
			s.stateDir = t.TempDir()
			if tc.truth != nil {
				tc.truth.CheckedAt = time.Now()
				if err := updatecheck.Write(s.stateDir, *tc.truth); err != nil {
					t.Fatal(err)
				}
			}
			row := railRow(t, sidebar(s, "/"), "System")
			if row.Detail != tc.detail || row.Hint != tc.hint {
				t.Fatalf("System row says %q (%q), want %q (%q)", row.Detail, row.Hint, tc.detail, tc.hint)
			}
			if waiting := tc.detail != ""; row.Dot != waiting || (waiting && row.Variant != "info") {
				t.Errorf("System row mark = %v %q; update news wears the info square", row.Dot, row.Variant)
			}
		})
	}
}

// Every rail row leads to a page that exists — no placeholders.
func TestBuildSidebarHasNoPlaceholderRows(t *testing.T) {
	model := sidebar(navServer(), "/")
	for _, row := range model.Rows {
		if row.Label == "Family" || row.Href == "#" || row.Href == "" {
			t.Fatalf("a rail row leads nowhere: %+v", row)
		}
	}
}

// Only the row you are in opens its subpages: the shell learns a plugin's pages
// from the envelope it just rendered, so it can speak for that page and no other.
func TestBuildSidebarOpensOnlyTheActiveRow(t *testing.T) {
	firewall := manifest("firewall", nav("Security", "Firewall", "/"))
	firewall.Socket = "/firewall.sock"
	s := navServer(firewall)
	pages := []pageTab{{Label: "Rules", Href: "/plugins/firewall/", Active: true}, {Label: "Zones", Href: "/plugins/firewall/zones"}}

	model := s.buildSidebar("/plugins/firewall/", identityTranslator,
		func(string) func(string) string { return identityTranslator }, pages)
	for _, row := range model.Rows {
		switch {
		case row.Label == "Firewall" && len(row.Children) != 2:
			t.Fatalf("the active row should open its subpages, got %+v", row.Children)
		case row.Label != "Firewall" && len(row.Children) != 0:
			t.Fatalf("row %q opened subpages that are not its own: %+v", row.Label, row.Children)
		}
	}
}

func TestSystemPagesUseOnlyLivePluginRegistrations(t *testing.T) {
	system := manifest("system", nav("System", "General", "/"))
	system.Socket = "/live/system.sock"
	vpn := manifest("vpn", nav("System", "VPN", "/"))
	vpn.Socket = "/dead/vpn.sock"
	s := navServer(system, vpn)
	s.probe = func(path string) bool { return path == "/live/system.sock" }

	pages := s.sectionPages("System", "/plugins/system/")
	if len(pages) != 7 || pages[0].Label != "General" || pages[0].Href != "/plugins/system/" || !pages[0].Active {
		t.Fatalf("System pages = %+v, want live General followed by six shell pages", pages)
	}
	for _, page := range pages {
		if page.Label == "VPN" {
			t.Fatal("stopped plugin registration remained in System pages")
		}
	}
}

// Network's pages are every live registration filed under it, whichever plugin
// files them, in the designed order — Interfaces, DHCP, DNS — and anything else
// after those by label. The page in hand is the one marked.
func TestNetworkPagesFollowTheDesignedOrder(t *testing.T) {
	s := navServer(
		manifest("dnsdhcp", nav("Network", "DNS", "/dns"), nav("Network", "DHCP", "/")),
		manifest("interfaces", nav("Network", "Interfaces", "/")),
		manifest("vpn", nav("Network", "WireGuard", "/")),
		manifest("lab", nav("Network", "Bridges", "/")),
	)
	pages := s.sectionPages("Network", "/plugins/dnsdhcp/dns")
	var got []string
	for _, page := range pages {
		got = append(got, page.Label)
		if page.Active != (page.Label == "DNS") {
			t.Errorf("%s active = %v, want only DNS marked", page.Label, page.Active)
		}
	}
	if want := []string{"Interfaces", "DHCP", "DNS", "Bridges", "WireGuard"}; !slices.Equal(got, want) {
		t.Fatalf("Network pages = %v, want %v", got, want)
	}
}

// The active path lights exactly the row it belongs to; the others stay dark.
func TestBuildSidebarLightsOnlyTheActiveRow(t *testing.T) {
	s := navServer(
		manifest("net", nav("Network", "Interfaces", "/")),
		manifest("fw", nav("Security", "Firewall", "/")),
	)
	for _, row := range sidebar(s, "/plugins/fw/").Rows {
		if row.Active != (row.Label == "Firewall") {
			t.Fatalf("row %q active = %v, want only Firewall lit", row.Label, row.Active)
		}
	}
}

// On a deep subpage, only that subpage's row lights up — not the plugin's root
// row, whose href (…/) prefixes every sibling. Regression: the Zones (root) link
// used to stay active on Redirects/Rules because it prefix-matched their URLs.
func TestBuildSidebarDeepSubpageMarksOnlyItself(t *testing.T) {
	s := navServer(manifest("firewall",
		nav("Security", "Zones", "/"),
		nav("Security", "Redirects", "/redirects"),
		nav("Security", "Traffic rules", "/rules"),
	))
	for _, row := range sidebar(s, "/plugins/firewall/redirects").Rows {
		if row.Active != (row.Label == "Redirects") {
			t.Errorf("row %q active = %v, want only Redirects lit", row.Label, row.Active)
		}
	}
}

// A page lights the row of the section its most specific registration is filed
// under, and no other, even when a sibling registration's shorter path in
// another section also holds it.
func TestBuildSidebarSectionPageLightsOnlyItsSection(t *testing.T) {
	s := navServer(manifest("tools",
		nav("Network", "Tools", "/"),
		nav("System", "Schedule", "/schedule"),
	))
	for at, lit := range map[string]string{"/plugins/tools/schedule": "System", "/plugins/tools/": "Network"} {
		for _, row := range sidebar(s, at).Rows {
			if row.Active != (row.Label == lit) {
				t.Errorf("at %s, row %q active = %v, want only %s lit", at, row.Label, row.Active, lit)
			}
		}
	}
}

// A plugin whose socket does not answer contributes no rows — a menu entry
// that leads to "unavailable" is a dead door; the plugin stays reachable by
// URL and through the management page (ADR-011). Shell-owned rows are
// unaffected.
func TestBuildSidebarHidesDeadPlugins(t *testing.T) {
	s := navServer(
		manifest("fw", nav("Security", "Firewall", "/")),
		manifest("vpn", nav("VPN", "WireGuard", "/")),
	)
	s.manifests[0].Socket = "/dead/fw.sock"
	s.manifests[1].Socket = "/live/vpn.sock"
	s.probe = func(path string) bool { return path == "/live/vpn.sock" }

	want := []string{"Overview", "Devices", "System", "WireGuard"}
	if got := railLabels(sidebar(s, "/")); !slices.Equal(got, want) {
		t.Fatalf("rail = %v, want %v", got, want)
	}
}
