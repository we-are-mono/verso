// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"testing"

	"github.com/we-are-mono/verso/internal/plugin"
)

// nav builds a Server holding only manifests — buildNav reads nothing else — so
// the sidebar ordering (ADR-009 §2, §5) is unit-testable with no transport,
// backend, or device.
func navServer(manifests ...plugin.Manifest) *Server {
	// Sockets read as alive so every manifest contributes rows; the
	// liveness-hiding test overrides probe itself.
	return &Server{manifests: manifests, probe: func(string) bool { return true }}
}

func manifest(id string, entries ...plugin.NavEntry) plugin.Manifest {
	return plugin.Manifest{ID: id, Nav: entries}
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
		manifest("fw", nav("Firewall", "Zones", "/")),
		manifest("net", nav("Network", "Interfaces", "/")),
	)
	assertTitles(t, s.buildNav("/"), "Status", "Network", "Firewall", "System")
}

// The shell's own pages exist with zero plugins (ADR-009 §3): Status carries the
// Overview baseline, System the Password auth surface and the plugin-management
// surface (ADR-011).
func TestBuildNavShellOwnedPagesAreBuiltIn(t *testing.T) {
	sections := navServer().buildNav("/")
	assertTitles(t, sections, "Status", "System")
	if got := sections[0].Links; len(got) != 1 || got[0].Label != "Overview" || got[0].Href != "/" {
		t.Fatalf("Status links = %+v, want single Overview -> /", got)
	}
	if got := sections[1].Links; len(got) != 3 || got[0].Label != "Password" || got[0].Href != "/system/password" ||
		got[1].Label != "Packages" || got[1].Href != "/system/packages" ||
		got[2].Label != "Services" || got[2].Href != "/system/services" {
		t.Fatalf("System links = %+v, want [Password, Packages, Services]", got)
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
	// Built-in Password and Plugins first, then plugin links in discovery
	// (id-sorted) order.
	if len(system.Links) != 5 || system.Links[0].Label != "Password" || system.Links[1].Label != "Packages" ||
		system.Links[2].Label != "Services" ||
		system.Links[3].Label != "General" || system.Links[4].Label != "Time" {
		t.Fatalf("System links = %+v, want [Password, Packages, Services, General, Time]", system.Links)
	}
}

// A plugin may contribute several entries across sections; the active path marks
// exactly its section open, others stay collapsed.
func TestBuildNavActiveSectionExpands(t *testing.T) {
	s := navServer(
		manifest("net", nav("Network", "Interfaces", "/")),
		manifest("fw", nav("Firewall", "Zones", "/")),
	)
	sections := s.buildNav("/plugins/fw/")
	for _, sec := range sections {
		wantOpen := sec.Title == "Firewall" // only the section holding the active page is open
		if sec.Open != wantOpen {
			t.Fatalf("section %q open = %v, want %v", sec.Title, sec.Open, wantOpen)
		}
		for _, l := range sec.Links {
			if l.Active != (sec.Title == "Firewall") {
				t.Fatalf("link %q active = %v, want %v", l.Label, l.Active, sec.Title == "Firewall")
			}
		}
	}
}

// A plugin whose socket does not answer contributes no rows — a menu entry
// that leads to "unavailable" is a dead door; the plugin stays reachable by
// URL and through the management page (ADR-011). Shell-owned rows are
// unaffected.
func TestBuildNavHidesDeadPlugins(t *testing.T) {
	s := navServer(
		manifest("fw", nav("Firewall", "Zones", "/")),
		manifest("vpn", nav("VPN", "WireGuard", "/")),
	)
	s.manifests[0].Socket = "/dead/fw.sock"
	s.manifests[1].Socket = "/live/vpn.sock"
	s.probe = func(path string) bool { return path == "/live/vpn.sock" }

	sections := s.buildNav("/")
	assertTitles(t, sections, "Status", "System", "VPN")
	for _, sec := range sections {
		if sec.Title == "Firewall" {
			t.Fatalf("dead plugin still contributes section %q", sec.Title)
		}
	}
}
