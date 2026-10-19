// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"bytes"
	"log"
	"net/http"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/we-are-mono/verso/internal/plugin"
)

// doorFirewall is a firewall plugin as it ships: one nav entry whose subpages
// list its rules, port forwards and zones, claiming the door to each.
func doorFirewall(id string) plugin.Manifest {
	m := manifest(id, plugin.NavEntry{Section: "Security", Label: "Firewall", Path: "/", Pages: plugin.NavPages{Opens: true, List: []plugin.PageTab{
		{Label: "Rules", Path: ""}, {Label: "Port forwards", Path: "port-forwards"}, {Label: "Zones", Path: "zones"},
	}}})
	m.Socket = "/" + id + ".sock"
	m.EntityActs = []plugin.EntityAct{
		{Entity: "rule", Slot: "open", Path: "/?open={id}"},
		{Entity: "port-forward", Slot: "open", Path: "/port-forwards?open={id}"},
		{Entity: "zone", Slot: "open", Path: "/zones?open={id}"},
	}
	return m
}

// doorPlugin is a plugin with one nav entry claiming one kind's door.
func doorPlugin(id, label, kind, path string) plugin.Manifest {
	m := manifest(id, nav("Network", label, "/"))
	m.Socket = "/" + id + ".sock"
	m.EntityActs = []plugin.EntityAct{{Entity: kind, Slot: "open", Path: path}}
	return m
}

// TestADoorKindHasTheOpenSlotAndNoPanel: the shell links to a network, a
// tunnel, a zone, a rule and a port forward through a plugin's claim, and none
// of them is a subject with a panel of its own, so its entity route is not one.
func TestADoorKindHasTheOpenSlotAndNoPanel(t *testing.T) {
	want := []string{"network", "tunnel", "zone", "rule", "port-forward"}
	if !slices.Equal(doorKinds, want) {
		t.Fatalf("door kinds = %v, want %v", doorKinds, want)
	}
	for _, kind := range doorKinds {
		if _, ok := entitySlots[kind]; ok {
			t.Errorf("door kind %q is a panel kind", kind)
		}
	}
	if rec := get(t, newServer(t, fakeBackend{}), "/entity/zone/lan"); rec.Code != http.StatusNotFound {
		t.Errorf("/entity/zone/lan = %d, want 404", rec.Code)
	}
}

// TestADoorIsTheFirstLiveClaimantByID: two plugins claiming one kind's door
// give every page the same one, the plugin whose id sorts first, whatever
// order they were found in; a stopped claimant gives way to the next.
func TestADoorIsTheFirstLiveClaimantByID(t *testing.T) {
	zerotier := doorPlugin("zerotier", "ZeroTier", "tunnel", "/?open={id}")
	vpn := doorPlugin("vpn", "VPN", "tunnel", "/?open={id}")
	s := navServer(zerotier, vpn)
	door, ok := s.door("tunnel")
	if !ok || door.pluginID != "vpn" {
		t.Fatalf("door = %+v, %v; want vpn's", door, ok)
	}
	s.probe = func(at plugin.Endpoint) bool { return at.Socket != vpn.Socket }
	if door, ok = s.door("tunnel"); !ok || door.pluginID != "zerotier" {
		t.Errorf("with vpn stopped, door = %+v, %v; want zerotier's", door, ok)
	}
	s.probe = func(plugin.Endpoint) bool { return false }
	if _, ok = s.door("tunnel"); ok {
		t.Error("a door with no claimant answering")
	}
}

// TestAClaimUnderNoNavEntryGivesNoDoor: a door names its page by the nav entry
// it sits under; a plugin with no page for it has nothing to open.
func TestAClaimUnderNoNavEntryGivesNoDoor(t *testing.T) {
	m := plugin.Manifest{ID: "tunnels", Socket: "/tunnels.sock", EntityActs: []plugin.EntityAct{{Entity: "tunnel", Slot: "open", Path: "/?open={id}"}}}
	if _, ok := navServer(m).door("tunnel"); ok {
		t.Error("a claim under no nav entry gave a door")
	}
}

// TestADoorToOneThingNamesItInTheAddress: the subject's identity takes {id}'s
// place, escaped for where it stands.
func TestADoorToOneThingNamesItInTheAddress(t *testing.T) {
	door, ok := navServer(doorFirewall("firewall")).door("rule")
	if !ok {
		t.Fatal("no rule door")
	}
	if got := door.To("cfg0c92bd"); got != "/plugins/firewall/?open=cfg0c92bd" {
		t.Errorf("To = %q", got)
	}
	if got := door.To("a b&c"); got != "/plugins/firewall/?open=a+b%26c" {
		t.Errorf("To in a query = %q", got)
	}
	door, _ = navServer(doorPlugin("vpn", "VPN", "tunnel", "/tunnel/{id}")).door("tunnel")
	if got := door.To("a b"); got != "/plugins/vpn/tunnel/a%20b" {
		t.Errorf("To in a path = %q", got)
	}
}

// TestADoorToAKindLeadsToThePageThatListsIt: the most specific of the
// plugin's pages holding the claim's path, its query left out.
func TestADoorToAKindLeadsToThePageThatListsIt(t *testing.T) {
	s := navServer(doorFirewall("firewall"))
	for kind, want := range map[string]string{
		"zone":         "/plugins/firewall/zones",
		"port-forward": "/plugins/firewall/port-forwards",
		"rule":         "/plugins/firewall/",
	} {
		door, ok := s.door(kind)
		if !ok || door.Listing() != want {
			t.Errorf("%s listing = %q, %v; want %q", kind, door.Listing(), ok, want)
		}
	}
	interfaces := manifest("interfaces", nav("Network", "Interfaces", "/"), nav("Network", "Routes", "/routes"))
	interfaces.Socket = "/interfaces.sock"
	interfaces.EntityActs = []plugin.EntityAct{{Entity: "network", Slot: "open", Path: "/edit?network={id}"}}
	door, ok := navServer(interfaces).door("network")
	if !ok || door.Listing() != "/plugins/interfaces/" {
		t.Errorf("network listing = %q, %v; want /plugins/interfaces/", door.Listing(), ok)
	}
}

// TestADoorSaysTheClaimingPluginsWord: the label of the nav entry holding the
// claim, translated by the claiming plugin's own catalog.
func TestADoorSaysTheClaimingPluginsWord(t *testing.T) {
	door, _ := navServer(doorFirewall("firewall")).door("zone")
	slovenian := func(pluginID string) func(string) string {
		return func(s string) string {
			if pluginID == "firewall" && s == "Firewall" {
				return "Požarni zid"
			}
			return s
		}
	}
	if got := door.Word(slovenian); got != "Požarni zid" {
		t.Errorf("Word = %q", got)
	}
	if got := door.Word(func(string) func(string) string { return identityTranslator }); got != "Firewall" {
		t.Errorf("English word = %q", got)
	}
}

// TestClaimsTheShellDoesNotHaveAreDroppedAtLoad: a claim on a kind or slot the
// shell does not have, and a door claim with no place for the subject, are
// logged and dropped alone; the plugin's other claims stand, a listing's
// doorway with no {id} among them.
func TestClaimsTheShellDoesNotHaveAreDroppedAtLoad(t *testing.T) {
	var logged bytes.Buffer
	log.SetOutput(&logged)
	defer log.SetOutput(os.Stderr)
	m := manifest("dnsdhcp", nav("Network", "DHCP", "/"))
	m.EntityActs = []plugin.EntityAct{
		{Entity: "device", Slot: "add", Path: "/?open=new"},
		{Entity: "device", Slot: "unreserve", Path: "/?open={id}"},
		{Entity: "zone", Slot: "open", Path: "/zones"},
		{Entity: "printer", Slot: "open", Path: "/?open={id}"},
		{Entity: "device", Slot: "frobnicate", Path: "/?open={id}"},
		{Entity: "network", Slot: "open", Path: "/?open={id}"},
	}
	got := withKnownClaims([]plugin.Manifest{m})[0].EntityActs
	want := []plugin.EntityAct{m.EntityActs[0], m.EntityActs[1], m.EntityActs[5]}
	if !slices.Equal(got, want) {
		t.Errorf("claims = %+v, want %+v", got, want)
	}
	for _, dropped := range []string{"zone/open", "printer/open", "device/frobnicate"} {
		if !strings.Contains(logged.String(), dropped) || !strings.Contains(logged.String(), "dnsdhcp") {
			t.Errorf("log does not name %s for dnsdhcp: %s", dropped, logged.String())
		}
	}
	if len(m.EntityActs) != 6 {
		t.Error("the caller's manifest was changed")
	}
}

// overviewGo is one Overview card's Go link as the page draws it.
func overviewGo(t *testing.T, page, id string) string {
	t.Helper()
	at := strings.Index(page, `data-overview-card="`+id+`"`)
	if at < 0 {
		t.Fatalf("no %s card", id)
	}
	card := page[at+1:]
	if next := strings.Index(card, `data-overview-card="`); next >= 0 {
		card = card[:next]
	}
	return regexp.MustCompile(`href="[^"]*" data-overview-go aria-label="[^"]*"`).FindString(card)
}

// TestTheOverviewOpensThroughThePluginsClaims: the Internet, Ports and
// Tunnels cards lead where the running plugins' claims on networks and
// tunnels lead, in those plugins' words; with the VPN plugin stopped, Tunnels
// opens the networks' page, and with no claim at all no card opens a plugin.
func TestTheOverviewOpensThroughThePluginsClaims(t *testing.T) {
	interfaces := manifest("interfaces", nav("Network", "Interfaces", "/"), nav("Network", "Routes", "/routes"))
	interfaces.Socket = "/interfaces.sock"
	interfaces.EntityActs = []plugin.EntityAct{{Entity: "network", Slot: "open", Path: "/edit?network={id}"}}
	vpn := doorPlugin("vpn", "VPN", "tunnel", "/?open={id}")
	s := newServerWith(t, fakeBackend{}, &fakeTransport{}, []plugin.Manifest{interfaces, vpn})
	s.probe = func(plugin.Endpoint) bool { return true }

	page := get(t, s, "/").Body.String()
	for id, want := range map[string]string{
		"internet": `href="/plugins/interfaces/" data-overview-go aria-label="Open Interfaces"`,
		"ports":    `href="/plugins/interfaces/" data-overview-go aria-label="Open Interfaces"`,
		"tunnels":  `href="/plugins/vpn/" data-overview-go aria-label="Open VPN"`,
	} {
		if got := overviewGo(t, page, id); !strings.Contains(got, want) {
			t.Errorf("%s card's door = %q, want %s", id, got, want)
		}
	}

	s.probe = func(at plugin.Endpoint) bool { return at.Socket != vpn.Socket }
	page = get(t, s, "/").Body.String()
	if got := overviewGo(t, page, "tunnels"); !strings.Contains(got, `href="/plugins/interfaces/" data-overview-go aria-label="Open Interfaces"`) {
		t.Errorf("with VPN stopped, Tunnels' door = %q", got)
	}

	s.probe = func(plugin.Endpoint) bool { return false }
	page = get(t, s, "/").Body.String()
	for _, id := range []string{"internet", "ports", "tunnels"} {
		if got := overviewGo(t, page, id); got != "" {
			t.Errorf("with no claim answering, %s opens %q", id, got)
		}
	}
}

// TestEntityActsTakeTheFirstClaimantByID: one slot two plugins claim leads
// where the plugin whose id sorts first leads, as a door does.
func TestEntityActsTakeTheFirstClaimantByID(t *testing.T) {
	b := manifest("b-reservations")
	b.Socket = "/b.sock"
	b.EntityActs = []plugin.EntityAct{{Entity: "device", Slot: "unreserve", Path: "/b?open={id}"}}
	a := manifest("a-reservations")
	a.Socket = "/a.sock"
	a.EntityActs = []plugin.EntityAct{{Entity: "device", Slot: "unreserve", Path: "/a?open={id}"}}
	if got := navServer(b, a).entityActs("device", "x")["unreserve"]; got != "/plugins/a-reservations/a?open=x" {
		t.Errorf("unreserve = %q, want a-reservations'", got)
	}
}
