// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/we-are-mono/verso/internal/plugin"
	"github.com/we-are-mono/verso/internal/sysstat"
	"github.com/we-are-mono/verso/internal/widget"
)

const testLeases = "1787825263 42:e6:ad:ff:b7:af 192.168.77.102 toms-iphone 01:42:e6:ad:ff:b7:af\n" +
	"1787825264 a2:8c:d7:4a:0e:57 192.168.77.120 * *\n" +
	"1787825265 06:11:22:33:44:55 192.168.77.130 old-printer *\n"

// testNeighbors: .102 is confirmed (reachable) and carries v6 entries on the
// same MAC, .120 has a lapsed entry (stale), .130 has none at all.
func testNeighbors() ([]sysstat.Neighbor, error) {
	return []sysstat.Neighbor{
		{Addr: "192.168.77.102", MAC: "42:e6:ad:ff:b7:af", Interface: "br-lan", State: 0x02},        // reachable
		{Addr: "fd42:7ea:aa00:0:1::66", MAC: "42:e6:ad:ff:b7:af", Interface: "br-lan", State: 0x04}, // its v6, stale
		{Addr: "fe80::44:11", MAC: "42:e6:ad:ff:b7:af", Interface: "br-lan", State: 0x04},           // its link-local
		{Addr: "192.168.77.120", MAC: "a2:8c:d7:4a:0e:57", Interface: "br-lan", State: 0x04},        // stale
		{Addr: "fe80::dead", MAC: "", Interface: "br-lan", State: 0x20},                             // failed, unresolved
	}, nil
}

// rosterBackend carries the config the roster reads: the lan interface's subnet,
// the firewall zone that covers it, and one DHCP reservation.
func rosterBackend() fakeBackend {
	return fakeBackend{uci: map[string]map[string]any{
		"network": {
			"lan": map[string]any{".type": "interface", ".name": "lan",
				"device": "br-lan", "ipaddr": "192.168.77.1", "netmask": "255.255.255.0"},
		},
		"firewall": {
			"z1": map[string]any{".type": "zone", "name": "lan", "network": []any{"lan"}},
		},
		"dhcp": {
			"printer": map[string]any{".type": "host", ".name": "printer",
				"name": "old-printer", "mac": "06:11:22:33:44:55", "ip": "192.168.77.130"},
		},
	}}
}

// dnsdhcpManifest registers the plugin that owns reservations, so the roster's
// reserve door has somewhere to lead.
func dnsdhcpManifest() plugin.Manifest {
	return plugin.Manifest{
		ManifestVersion: 1, ID: "dnsdhcp", Name: "DNS and DHCP",
		Socket: "/run/verso/dnsdhcp.sock", SchemaVersion: 1,
		Nav: []plugin.NavEntry{{Section: "Network", Label: "DHCP", Path: "/"}},
	}
}

func rosterServer(t *testing.T) *Server {
	t.Helper()
	s := newServerWith(t, rosterBackend(), &fakeTransport{}, []plugin.Manifest{dnsdhcpManifest()})
	s.readLeases = func() ([]byte, error) { return []byte(testLeases), nil }
	s.neighbors = testNeighbors
	s.bridgePorts = func() (map[string]string, error) {
		return map[string]string{"42:e6:ad:ff:b7:af": "lan0"}, nil
	}
	return s
}

func roster(t *testing.T, s *Server) map[string]widget.Device {
	t.Helper()
	byName := map[string]widget.Device{}
	for _, device := range s.connectedDevices(context.Background(), "test-sid") {
		byName[device.Name] = device
	}
	return byName
}

func TestConnectedDevicesUseKernelInterfaceWithUCIFallback(t *testing.T) {
	byName := roster(t, rosterServer(t))
	if got := byName["toms-iphone"]; got.Interface != "br-lan" || got.Zone != "lan" {
		t.Errorf("kernel-backed device = %+v", got)
	}
	if got := byName["old-printer"]; got.Interface != "br-lan" || got.Zone != "lan" {
		t.Errorf("lease-only UCI fallback = %+v", got)
	}
}

// TestDevicesPageRendersTheRoster: /devices is the roster's own page — heading,
// the listing, and every row's drawer, from the same reads the overview used.
func TestDevicesPageRendersTheRoster(t *testing.T) {
	rec := get(t, rosterServer(t), "/devices")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /devices: status = %d, want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	for _, want := range []string{
		">Devices<",                      // the page heading
		"Connected devices", "3 devices", // the listing and its count
		"toms-iphone", "42:e6:ad:ff:b7:af", // a leaseholder, by name and MAC
		"192.168.77.102", "Online", "Offline", // its address and the presence words
		"Details", "fd42:7ea:aa00:0:1::66", // the drawer and the addresses only it shows
	} {
		if !strings.Contains(body, want) {
			t.Errorf("GET /devices: body missing %q", want)
		}
	}
}

// TestDevicesDrawerOffersReservationOnlyWhereItMeans: the door to the DHCP page
// opens for a device holding an address it does not own; a reserved device
// states the fact instead, and a device with no lease has nothing to keep.
func TestDevicesDrawerOffersReservationOnlyWhereItMeans(t *testing.T) {
	byName := roster(t, rosterServer(t))
	iphone := byName["toms-iphone"]
	if !iphone.Leased || iphone.Reserved {
		t.Fatalf("toms-iphone = %+v, want a lease and no reservation", iphone)
	}
	if want := "/plugins/dnsdhcp/?reserve=42%3Ae6%3Aad%3Aff%3Ab7%3Aaf"; iphone.ReserveHref != want {
		t.Errorf("reserve door = %q, want %q", iphone.ReserveHref, want)
	}
	if printer := byName["old-printer"]; !printer.Reserved {
		t.Errorf("a MAC named by a dhcp host section should read as reserved: %+v", printer)
	}

	body := get(t, rosterServer(t), "/devices").Body.String()
	if strings.Count(body, "Reserve its address") != 2 {
		t.Errorf("only the two unreserved leaseholders should be offered a reservation")
	}
	if !strings.Contains(body, "Address reserved for this device") {
		t.Errorf("the reserved device should state its reservation")
	}
}

// TestDevicesReserveDoorClosesWithoutThePlugin: with nothing serving the DHCP
// page, the drawer offers no door rather than one that lands on "unavailable".
func TestDevicesReserveDoorClosesWithoutThePlugin(t *testing.T) {
	s := rosterServer(t)
	s.probe = func(string) bool { return false }
	if got := roster(t, s)["toms-iphone"].ReserveHref; got != "" {
		t.Errorf("reserve door = %q, want none while no plugin answers", got)
	}
}

// TestOnlineDevicesCountsFromTheNeighbourTable: the sidebar's number is the
// kernel's own confirmation — and an unreadable table reports no count at all
// rather than claiming nobody is here.
func TestOnlineDevicesCountsFromTheNeighbourTable(t *testing.T) {
	s := rosterServer(t)
	if online, ok := s.onlineDevices(); !ok || online != 1 {
		t.Errorf("online devices = %d (known %v), want the one reachable device", online, ok)
	}
	s.neighbors = func() ([]sysstat.Neighbor, error) { return nil, errors.New("no netlink") }
	if online, ok := s.onlineDevices(); ok || online != 0 {
		t.Errorf("online devices = %d (known %v), want no count", online, ok)
	}
}

// TestLeaseIn: lease expiry reads the way a person says it.
func TestLeaseIn(t *testing.T) {
	now := time.Unix(1000, 0)
	cases := map[int64]string{
		1000 + 11*3600 + 32*60: "in 11h 32m",
		1000 + 40*60:           "in 40 min",
		900:                    "expired",
	}
	for expiry, want := range cases {
		if got := leaseIn(expiry, now); got != want {
			t.Errorf("leaseIn(%d) = %q, want %q", expiry, got, want)
		}
	}
}
