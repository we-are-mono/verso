// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"errors"
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

// sidebar builds one path's sidebar in the advanced reading, where every section
// renders — the ordering and liveness rules below are about what the sections
// contain, not about which reading shows them. The mode cases state their own.
func sidebar(s *Server, active string) navModel {
	return s.buildSidebar(active, widget.ModeAdvanced, identityTranslator,
		func(string) func(string) string { return identityTranslator })
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

func TestBuildSidebarPromotesSystemAboveAdvanced(t *testing.T) {
	system := manifest("system", nav("System", "General", "/"))
	system.Socket = "/system.sock"
	s := navServer(system, manifest("net", nav("Network", "Interfaces", "/")))
	model := sidebar(s, "/plugins/system/")
	if got := model.Basic[len(model.Basic)-1]; got.Label != "System" || got.Href != "/plugins/system/" || !got.Active {
		t.Fatalf("last basic row = %+v, want active System targeting registered General", got)
	}
	for _, group := range model.Advanced {
		if group.Title == "System" {
			t.Fatal("System must not also appear among the sections")
		}
	}
}

// securityRow returns the sidebar's Security row, which sits directly above
// System among the everyday rows.
func securityRow(t *testing.T, model navModel) navLink {
	t.Helper()
	for _, row := range model.Basic {
		if row.Icon == "shield" {
			return row
		}
	}
	t.Fatal("Security row missing from the sidebar")
	return navLink{}
}

// Security is an everyday row, not an Advanced group: it targets the firewall
// plugin's own page and stays lit anywhere inside the domain, the same way
// System does.
func TestBuildSidebarPromotesSecurityAboveAdvanced(t *testing.T) {
	firewall := manifest("firewall", nav("Security", "Firewall", "/"))
	firewall.Socket = "/firewall.sock"
	s := navServer(firewall, manifest("net", nav("Network", "Interfaces", "/")))

	model := sidebar(s, "/plugins/firewall/zones")
	row := securityRow(t, model)
	if row.Label != "Security" || row.Href != "/plugins/firewall/" || !row.Active {
		t.Fatalf("Security row = %+v, want active Security targeting the registered page", row)
	}
	for _, group := range model.Advanced {
		if group.Title == "Security" {
			t.Fatal("Security must not also appear among the sections")
		}
	}
}

// With no plugin answering under Security the row leads nowhere rather than to a
// URL that only reports the plugin is unavailable.
func TestBuildSidebarSecurityWithoutALivePluginLeadsNowhere(t *testing.T) {
	firewall := manifest("firewall", nav("Security", "Firewall", "/"))
	firewall.Socket = "/dead/firewall.sock"
	s := navServer(firewall)
	s.probe = func(string) bool { return false }

	model := sidebar(s, "/")
	if row := securityRow(t, model); row.Href != "#" || row.Active {
		t.Fatalf("Security row = %+v, want an inert row", row)
	}
	// The stopped plugin's own URL still belongs to the domain, so the row lights
	// up there and the page can explain itself.
	model = sidebar(s, "/plugins/firewall/")
	if row := securityRow(t, model); !row.Active {
		t.Fatalf("Security row = %+v, want Active on a stopped plugin's own URL", row)
	}
}

// basicRow returns the everyday row carrying one icon.
func basicRow(t *testing.T, model navModel, icon string) navLink {
	t.Helper()
	for _, row := range model.Basic {
		if row.Icon == icon {
			return row
		}
	}
	t.Fatalf("no basic row with icon %q", icon)
	return navLink{}
}

// The Devices row leads to the roster page and lights up there, carrying the
// live count of devices on the network as its trailing detail.
func TestBuildSidebarDevicesRowLeadsToTheRoster(t *testing.T) {
	s := navServer()
	s.neighbors = testNeighbors
	model := sidebar(s, devicesPath)
	row := basicRow(t, model, "devices")
	if row.Href != devicesPath || !row.Active || row.Detail != "2" {
		t.Fatalf("Devices row = %+v, want the active roster row counting both kernel-vouched devices", row)
	}
	if elsewhere := basicRow(t, sidebar(s, "/"), "devices"); elsewhere.Active {
		t.Errorf("Devices row = %+v, want inactive away from the roster", elsewhere)
	}
}

// A box that cannot count its devices shows the row without a number rather
// than an invented or stale one.
func TestBuildSidebarDevicesRowDegradesWithoutACount(t *testing.T) {
	model := sidebar(navServer(), "/")
	if row := basicRow(t, model, "devices"); row.Detail != "" {
		t.Fatalf("Devices row = %+v, want no detail when the count is unavailable", row)
	}
}

// Every everyday row leads to a page that exists — the Family placeholder is
// gone, and its icon with it.
func TestBuildSidebarHasNoFamilyRow(t *testing.T) {
	model := sidebar(navServer(), "/")
	for _, row := range model.Basic {
		if row.Label == "Family" || row.Icon == "users" {
			t.Fatalf("the Family placeholder row should be gone: %+v", row)
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
	if len(pages) != 6 || pages[0].Label != "General" || pages[0].Href != "/plugins/system/" || !pages[0].Active {
		t.Fatalf("System pages = %+v, want live General followed by five shell pages", pages)
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
