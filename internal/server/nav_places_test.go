// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"slices"
	"testing"

	"github.com/we-are-mono/verso/internal/plugin"
)

// at is a nav entry with its declared place.
func navAt(order int, entry plugin.NavEntry) plugin.NavEntry {
	entry.Order = &order
	return entry
}

// pageLabels is a collapsed section's pages, in the order the rail lists them.
func pageLabels(s *Server, section string) []string {
	var labels []string
	for _, page := range s.sectionPages(section, "") {
		labels = append(labels, page.Label)
	}
	return labels
}

// bundled is the first-party plugins at the versions that declare their
// places.
func bundled() []plugin.Manifest {
	return []plugin.Manifest{
		manifest("ddns", navAt(50, nav("Network", "Dynamic DNS", "/"))),
		manifest("dnsdhcp", navAt(20, nav("Network", "DHCP", "/")), navAt(30, nav("Network", "DNS", "/dns"))),
		manifest("firewall", navAt(50, nav("Security", "Firewall", "/"))),
		manifest("interfaces", navAt(10, nav("Network", "Interfaces", "/")), navAt(40, nav("Network", "Routes", "/routes"))),
		manifest("system", navAt(10, nav("System", "General", "/"))),
		manifest("vpn", navAt(40, nav("VPN", "VPN", "/"))),
		manifest("wireless", navAt(30, nav("Wireless", "Wireless", "/"))),
	}
}

var systemShellPages = []string{"Hardware", "Access", "Packages", "Services", "Maintenance", "Logs", "Events", "Changes", "Diagnostics"}

// TestTheBundledPluginsKeepTheDesignedRail: with every first-party plugin
// declaring its place, the rail and both collapsed sections read as designed.
func TestTheBundledPluginsKeepTheDesignedRail(t *testing.T) {
	s := navServer(bundled()...)
	if got, want := railLabels(sidebar(s, "/")), []string{"Overview", "Devices", "Network", "Wireless", "VPN", "Firewall", "System"}; !slices.Equal(got, want) {
		t.Errorf("rail = %v, want %v", got, want)
	}
	if got, want := pageLabels(s, "Network"), []string{"Interfaces", "DHCP", "DNS", "Routes", "Dynamic DNS"}; !slices.Equal(got, want) {
		t.Errorf("Network = %v, want %v", got, want)
	}
	if got, want := pageLabels(s, "System"), append([]string{"General"}, systemShellPages...); !slices.Equal(got, want) {
		t.Errorf("System = %v, want %v", got, want)
	}
}

// TestAPluginPlacesItselfAmongOthers: a row at 45 stands between Wireless at
// 30 and Firewall at 50, and a page at 35 between DNS and Routes.
func TestAPluginPlacesItselfAmongOthers(t *testing.T) {
	s := navServer(
		manifest("wireless", navAt(30, nav("Wireless", "Wireless", "/"))),
		manifest("firewall", navAt(50, nav("Security", "Firewall", "/"))),
		manifest("mesh", navAt(45, nav("Mesh", "Mesh", "/"))),
		manifest("interfaces", navAt(10, nav("Network", "Interfaces", "/")), navAt(40, nav("Network", "Routes", "/routes"))),
		manifest("dnsdhcp", navAt(20, nav("Network", "DHCP", "/")), navAt(30, nav("Network", "DNS", "/dns"))),
		manifest("multicast", navAt(35, nav("Network", "Multicast", "/"))),
	)
	if got, want := railLabels(sidebar(s, "/")), []string{"Overview", "Devices", "Network", "Wireless", "Mesh", "Firewall", "System"}; !slices.Equal(got, want) {
		t.Errorf("rail = %v, want %v", got, want)
	}
	if got, want := pageLabels(s, "Network"), []string{"Interfaces", "DHCP", "DNS", "Multicast", "Routes"}; !slices.Equal(got, want) {
		t.Errorf("Network = %v, want %v", got, want)
	}
}

// TestPlacesTieByTheirEnglishLabel: two rows at one place stand by their
// English label, then plugin id, so a translation never reorders the rail.
func TestPlacesTieByTheirEnglishLabel(t *testing.T) {
	s := navServer(
		manifest("a-storage", navAt(60, nav("Storage", "Storage", "/"))),
		manifest("b-printers", navAt(60, nav("Printers", "Printers", "/"))),
	)
	slovenian := map[string]string{"Printers": "Tiskalniki", "Storage": "Shramba"}
	pluginTr := func(string) func(string) string {
		return func(s string) string {
			if word, ok := slovenian[s]; ok {
				return word
			}
			return s
		}
	}
	english := railLabels(sidebar(s, "/"))
	translated := railLabels(s.buildSidebar("/", identityTranslator, pluginTr, nil))
	if slices.Index(english, "Printers") > slices.Index(english, "Storage") {
		t.Errorf("rail = %v, want Printers before Storage", english)
	}
	if slices.Index(translated, "Tiskalniki") > slices.Index(translated, "Shramba") {
		t.Errorf("Slovenian rail = %v, want Tiskalniki before Shramba as in English", translated)
	}
}

// TestOverviewStandsFirst: Overview is place 0, whatever a plugin declares; a
// plugin may stand between it and Devices, at 10.
func TestOverviewStandsFirst(t *testing.T) {
	for _, place := range []int{-5, 1, 5} {
		s := navServer(manifest("top", navAt(place, nav("Top", "Top", "/"))))
		if got, want := railLabels(sidebar(s, "/"))[:3], []string{"Overview", "Top", "Devices"}; !slices.Equal(got, want) {
			t.Errorf("at %d the rail starts %v, want %v", place, got, want)
		}
	}
}

// TestAPagePlacesItselfAmongSystemsOwn: the shell's System pages stand at
// their published places, so a plugin's page at 65 stands after Maintenance.
func TestAPagePlacesItselfAmongSystemsOwn(t *testing.T) {
	s := navServer(
		manifest("system", navAt(10, nav("System", "General", "/"))),
		manifest("backup", navAt(65, nav("System", "Backup", "/"))),
	)
	want := []string{"General", "Hardware", "Access", "Packages", "Services", "Maintenance", "Backup", "Logs", "Events", "Changes", "Diagnostics"}
	if got := pageLabels(s, "System"); !slices.Equal(got, want) {
		t.Errorf("System = %v, want %v", got, want)
	}
}

// TestAnUnplacedEntryFollowsThePlacedOnes: an older plugin that declares no
// place stands after every sibling that does, by section and then label.
func TestAnUnplacedEntryFollowsThePlacedOnes(t *testing.T) {
	s := navServer(
		manifest("vpn", nav("VPN", "VPN", "/")),
		manifest("firewall", navAt(50, nav("Security", "Firewall", "/"))),
		manifest("guard", nav("Security", "Guard", "/")),
		manifest("interfaces", navAt(10, nav("Network", "Interfaces", "/")), navAt(40, nav("Network", "Routes", "/routes"))),
		manifest("dnsdhcp", nav("Network", "DHCP", "/"), nav("Network", "DNS", "/dns")),
	)
	if got, want := railLabels(sidebar(s, "/")), []string{"Overview", "Devices", "Network", "Firewall", "System", "Guard", "VPN"}; !slices.Equal(got, want) {
		t.Errorf("rail = %v, want %v", got, want)
	}
	if got, want := pageLabels(s, "Network"), []string{"Interfaces", "Routes", "DHCP", "DNS"}; !slices.Equal(got, want) {
		t.Errorf("Network = %v, want %v", got, want)
	}
}

// TestARowsGlyphComesFromItsEntryOrItsSection: the shell picks no glyph by a
// plugin's label; an entry under Status still has no row of its own.
func TestARowsGlyphComesFromItsEntryOrItsSection(t *testing.T) {
	vpn := nav("VPN", "VPN", "/")
	vpn.Icon = "lock"
	s := navServer(
		manifest("vpn", vpn),
		manifest("firewall", nav("Security", "Firewall", "/")),
		manifest("radio", nav("Wireless", "Wireless", "/")),
		manifest("speed", nav("Status", "Speed", "/")),
	)
	model := sidebar(s, "/")
	for label, want := range map[string]string{"VPN": "lock", "Firewall": "zone", "Wireless": "menu", "Overview": "house", "Devices": "phone"} {
		if got := railRow(t, model, label).Icon; got != want {
			t.Errorf("%s glyph = %q, want %q", label, got, want)
		}
	}
	if slices.Contains(railLabels(model), "Speed") {
		t.Error("an entry under Status has a row of its own")
	}
}
