// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"errors"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/we-are-mono/verso/internal/plugin"
	"github.com/we-are-mono/verso/internal/sysstat"
	"github.com/we-are-mono/verso/internal/widget"
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
	return s.buildSidebar(active, widget.ModeAdvanced, identityTranslator,
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

func sectionTitles(sections []navSection) []string {
	titles := make([]string, len(sections))
	for i, s := range sections {
		titles[i] = s.Title
	}
	return titles
}

func assertTitles(t *testing.T, got []navSection, want ...string) {
	t.Helper()
	titles := sectionTitles(got)
	if len(titles) != len(want) {
		t.Fatalf("section order = %v, want %v", titles, want)
	}
	for i := range want {
		if titles[i] != want[i] {
			t.Fatalf("section order = %v, want %v", titles, want)
		}
	}
}

// Core sections render in the fixed taxonomy order no matter what order the
// plugins are discovered in — the whole point of the shell owning the taxonomy.
func TestBuildNavCoreOrderIsFixed(t *testing.T) {
	s := navServer(
		manifest("sys", nav("System", "General", "/")),
		manifest("fw", nav("Security", "Firewall", "/")),
		manifest("net", nav("Network", "Interfaces", "/")),
	)
	assertTitles(t, s.buildNav("/"), "Status", "Network", "Security", "System")
}

// The System frame exists with zero plugins, but contains only pages the shell
// owns. General must be contributed by a plugin manifest.
func TestBuildNavShellOwnedPagesAreBuiltIn(t *testing.T) {
	sections := navServer().buildNav("/")
	assertTitles(t, sections, "Status", "System")
	if got := sections[0].Links; len(got) != 1 || got[0].Label != "Overview" || got[0].Href != "/" {
		t.Fatalf("Status links = %+v, want single Overview -> /", got)
	}
	if got := sections[1].Links; len(got) != 4 ||
		got[0].Label != "Access" || got[0].Href != "/system/access" ||
		got[1].Label != "Packages" || got[1].Href != "/system/packages" ||
		got[2].Label != "Services" || got[2].Href != "/system/services" ||
		got[3].Label != "Maintenance" || got[3].Href != "/system/maintenance" {
		t.Fatalf("System links = %+v, want [Access, Packages, Services, Maintenance]", got)
	}
}

// Plugin-introduced (non-core) sections sort after every core section, by title.
func TestBuildNavExtensionSectionsAfterCoreByTitle(t *testing.T) {
	s := navServer(
		manifest("vpn", nav("VPN", "WireGuard", "/")),
		manifest("net", nav("Network", "Interfaces", "/")),
		manifest("stats", nav("Statistics", "Graphs", "/")),
	)
	// Core sections (Status, Network, System — System from the built-in Password)
	// precede the extension sections; Statistics before VPN.
	assertTitles(t, s.buildNav("/"), "Status", "Network", "System", "Statistics", "VPN")
}

// Several plugins filing into one section keep their links in discovery
// (id-sorted) order; the stable sort must not disturb within-section order.
func TestBuildNavLinksGroupInDiscoveryOrder(t *testing.T) {
	s := navServer(
		manifest("hostname", nav("System", "General", "/")),
		manifest("time", nav("System", "Time", "/")),
	)
	sections := s.buildNav("/")
	var system *navSection
	for i := range sections {
		if sections[i].Title == "System" {
			system = &sections[i]
		}
	}
	if system == nil {
		t.Fatal("System section missing")
	}
	// Shell-owned entries come first in this grouped navigation model, followed
	// by manifest registrations in discovery order.
	if len(system.Links) != 6 || system.Links[0].Label != "Access" ||
		system.Links[1].Label != "Packages" || system.Links[2].Label != "Services" ||
		system.Links[3].Label != "Maintenance" || system.Links[4].Label != "General" ||
		system.Links[5].Label != "Time" {
		t.Fatalf("System links = %+v, want shell pages followed by plugin registrations", system.Links)
	}
}

// System is one row, not a group: it targets the first page its frame resolves
// to and stays lit anywhere inside the domain.
func TestBuildSidebarSystemIsOneRow(t *testing.T) {
	system := manifest("system", nav("System", "General", "/"))
	system.Socket = "/system.sock"
	s := navServer(system, manifest("net", nav("Network", "Interfaces", "/")))

	model := sidebar(s, "/plugins/system/")
	row := railRow(t, model, "System")
	if row.Href != "/plugins/system/" || !row.Active {
		t.Fatalf("System row = %+v, want active System targeting registered General", row)
	}
	if n := strings.Count(strings.Join(railLabels(model), "\x00"), "System"); n != 1 {
		t.Fatalf("rail = %v, want System exactly once", railLabels(model))
	}
	// The row's own registration must not also stand on its own in the rail.
	for _, label := range railLabels(model) {
		if label == "General" {
			t.Fatal("a System page belongs under the System row, not beside it")
		}
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

// The rail follows the order the design canvas gives it, whatever order the
// plugins were discovered in.
func TestBuildSidebarFollowsTheDesignedOrder(t *testing.T) {
	dns := manifest("dnsdhcp", nav("Network", "DNS & DHCP", "/"))
	dns.Socket = "/dns.sock"
	wireless := manifest("wireless", nav("Network", "Wireless", "/"))
	wireless.Socket = "/wireless.sock"
	firewall := manifest("firewall", nav("Security", "Firewall", "/"))
	firewall.Socket = "/firewall.sock"
	// Discovery is id-sorted, so the manifests arrive dnsdhcp, firewall,
	// wireless — nothing like the order the rail must draw them in.
	s := navServer(dns, firewall, wireless)

	want := []string{"Overview", "Devices", "Wireless", "Firewall", "DNS & DHCP", "System"}
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

// The Devices row leads to the roster page and lights up there, carrying the
// live count of devices on the network as its trailing detail.
func TestBuildSidebarDevicesRowLeadsToTheRoster(t *testing.T) {
	s := navServer()
	s.neighbors = testNeighbors
	row := railRow(t, sidebar(s, devicesPath), "Devices")
	if row.Href != devicesPath || !row.Active || row.Detail != "2" {
		t.Fatalf("Devices row = %+v, want the active roster row counting both kernel-vouched devices", row)
	}
	if elsewhere := railRow(t, sidebar(s, "/"), "Devices"); elsewhere.Active {
		t.Errorf("Devices row = %+v, want inactive away from the roster", elsewhere)
	}
}

// A box that cannot count its devices shows the row without a number rather
// than an invented or stale one.
func TestBuildSidebarDevicesRowDegradesWithoutACount(t *testing.T) {
	if row := railRow(t, sidebar(navServer(), "/"), "Devices"); row.Detail != "" {
		t.Fatalf("Devices row = %+v, want no detail when the count is unavailable", row)
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

	model := s.buildSidebar("/plugins/firewall/", widget.ModeAdvanced, identityTranslator,
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

	pages := s.systemPages("/plugins/system/", widget.ModeAdvanced)
	if len(pages) != 7 || pages[0].Label != "General" || pages[0].Href != "/plugins/system/" || !pages[0].Active {
		t.Fatalf("System pages = %+v, want live General followed by six shell pages", pages)
	}
	for _, page := range pages {
		if page.Label == "VPN" {
			t.Fatal("stopped plugin registration remained in System pages")
		}
	}
}

// A plugin may contribute several entries across sections; the active path marks
// exactly its section open, others stay collapsed.
func TestBuildNavActiveSectionExpands(t *testing.T) {
	s := navServer(
		manifest("net", nav("Network", "Interfaces", "/")),
		manifest("fw", nav("Security", "Firewall", "/")),
	)
	sections := s.buildNav("/plugins/fw/")
	for _, sec := range sections {
		wantOpen := sec.Title == "Security" // only the section holding the active page is open
		if sec.Open != wantOpen {
			t.Fatalf("section %q open = %v, want %v", sec.Title, sec.Open, wantOpen)
		}
		for _, l := range sec.Links {
			if l.Active != (sec.Title == "Security") {
				t.Fatalf("link %q active = %v, want %v", l.Label, l.Active, sec.Title == "Security")
			}
		}
	}
}

// On a deep subpage, only that subpage's link lights up — not the plugin's root
// link, whose href (…/) prefixes every sibling. Regression: the Zones (root) link
// used to stay active on Redirects/Rules because it prefix-matched their URLs.
func TestBuildNavDeepSubpageMarksOnlyItself(t *testing.T) {
	s := navServer(manifest("firewall",
		nav("Security", "Zones", "/"),
		nav("Security", "Redirects", "/redirects"),
		nav("Security", "Traffic rules", "/rules"),
	))
	sections := s.buildNav("/plugins/firewall/redirects")
	var fw navSection
	for _, sec := range sections {
		if sec.Title == "Security" {
			fw = sec
		}
	}
	for _, l := range fw.Links {
		want := l.Label == "Redirects"
		if l.Active != want {
			t.Errorf("link %q active = %v, want %v", l.Label, l.Active, want)
		}
	}
}

// A plugin whose socket does not answer contributes no rows — a menu entry
// that leads to "unavailable" is a dead door; the plugin stays reachable by
// URL and through the management page (ADR-011). Shell-owned rows are
// unaffected.
func TestBuildNavHidesDeadPlugins(t *testing.T) {
	s := navServer(
		manifest("fw", nav("Security", "Firewall", "/")),
		manifest("vpn", nav("VPN", "WireGuard", "/")),
	)
	s.manifests[0].Socket = "/dead/fw.sock"
	s.manifests[1].Socket = "/live/vpn.sock"
	s.probe = func(path string) bool { return path == "/live/vpn.sock" }

	sections := s.buildNav("/")
	assertTitles(t, sections, "Status", "System", "VPN")
	for _, sec := range sections {
		if sec.Title == "Security" {
			t.Fatalf("dead plugin still contributes section %q", sec.Title)
		}
	}
}
