// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/we-are-mono/verso/internal/openwrt"
	"github.com/we-are-mono/verso/internal/plugin"
)

// dhcpLeasesFile: three DHCPv4 leases out of address order, one from a device
// that offered no hostname.
const dhcpLeasesFile = "1787825265 30:9C:23:5E:88:01 10.0.10.30 nas 01:30:9c:23:5e:88:01\n" +
	"1787825263 42:e6:ad:ff:b7:af 10.0.0.142 toms-iphone 01:42:e6:ad:ff:b7:af\n" +
	"1787825264 a2:8c:d7:4a:0e:57 10.0.0.120 * *\n"

func dhcpLeasesManifest() plugin.Manifest {
	m := demoManifest()
	m.ACL = plugin.ACL{Read: []plugin.ACLScope{
		{Scope: "uci", Object: "dhcp", Function: "read"},
		{Scope: "ubus", Object: "verso", Function: "dhcpLeases"},
	}}
	return m
}

// leasesServer is a shell whose plugin declares the lease read, with the two
// sources the read joins already answered.
func leasesServer(t *testing.T, v6 []openwrt.V6Lease, leases func() ([]byte, error)) (*Server, *fakeTransport) {
	t.Helper()
	tr := &fakeTransport{env: &plugin.Envelope{
		SchemaVersion: 1, Title: "DHCP", Widget: json.RawMessage(`{"type":"card"}`),
	}}
	s := newServerWith(t, fakeBackend{v6Leases: v6}, tr, []plugin.Manifest{dhcpLeasesManifest()})
	s.readLeases = leases
	return s, tr
}

// TestDHCPLeasesBrokered: a plugin declaring dhcpLeases receives the live lease
// table — the DHCPv4 file in address order, each device's DHCPv6 addresses
// merged onto it, and dnsmasq's "*" stand-in dropped rather than passed off as
// a name.
func TestDHCPLeasesBrokered(t *testing.T) {
	v6 := []openwrt.V6Lease{
		// DUID-LLT: the MAC is its tail, which is how a v6 lease finds its device.
		{DUID: "00010001299d1e4b42e6adffb7af", Hostname: "", Addrs: []string{"2a00:ee2::142"}},
		// No link-layer DUID, but both tables recorded the same name.
		{DUID: "0002000000091122334455", Hostname: "nas", Addrs: []string{"2a00:ee2::30"}},
		// A lease belonging to no v4 lease carries no MAC, so it is left out.
		{DUID: "000100011122334455667788", Hostname: "unknown", Addrs: []string{"2a00:ee2::99"}},
	}
	s, tr := leasesServer(t, v6, func() ([]byte, error) { return []byte(dhcpLeasesFile), nil })

	if rec := get(t, s, "/plugins/demo/"); rec.Code != 200 {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	raw, ok := tr.lastReq.Ubus["dhcpLeases"]
	if !ok {
		t.Fatalf("forwarded request carried no lease read: %+v", tr.lastReq.Ubus)
	}
	var got struct {
		Leases []dhcpLease `json:"leases"`
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal %s: %v", raw, err)
	}

	want := []dhcpLease{
		{Hostname: "", MAC: "a2:8c:d7:4a:0e:57", IPv4: "10.0.0.120", ExpiresAt: 1787825264},
		{Hostname: "toms-iphone", MAC: "42:e6:ad:ff:b7:af", IPv4: "10.0.0.142",
			IPv6s: []string{"2a00:ee2::142"}, ExpiresAt: 1787825263},
		{Hostname: "nas", MAC: "30:9c:23:5e:88:01", IPv4: "10.0.10.30",
			IPv6s: []string{"2a00:ee2::30"}, ExpiresAt: 1787825265},
	}
	if len(got.Leases) != len(want) {
		t.Fatalf("leases = %+v, want %d entries", got.Leases, len(want))
	}
	for i, w := range want {
		g := got.Leases[i]
		if g.Hostname != w.Hostname || g.MAC != w.MAC || g.IPv4 != w.IPv4 || g.ExpiresAt != w.ExpiresAt {
			t.Errorf("lease %d = %+v, want %+v", i, g, w)
		}
		if len(g.IPv6s) != len(w.IPv6s) || (len(w.IPv6s) == 1 && g.IPv6s[0] != w.IPv6s[0]) {
			t.Errorf("lease %d addresses = %v, want %v", i, g.IPv6s, w.IPv6s)
		}
	}
}

// TestDHCPLeasesDegradeWithoutTheLeaseFile: a device that hands out no addresses
// (or a lease file the shell cannot read) leaves the plugin with no lease data at
// all, and the page still renders — reads never fail a page.
func TestDHCPLeasesDegradeWithoutTheLeaseFile(t *testing.T) {
	s, tr := leasesServer(t, nil, func() ([]byte, error) { return nil, errors.New("no lease file") })

	if rec := get(t, s, "/plugins/demo/"); rec.Code != 200 {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if tr.lastReq.Ubus != nil {
		t.Errorf("a failed read must contribute nothing: %+v", tr.lastReq.Ubus)
	}
}

// TestDHCPLeasesEmptyFileIsAnEmptyList: no leases is an answer, not an absence —
// the plugin renders its empty state rather than the "no data" one.
func TestDHCPLeasesEmptyFileIsAnEmptyList(t *testing.T) {
	s, tr := leasesServer(t, nil, func() ([]byte, error) { return []byte("\n"), nil })

	if rec := get(t, s, "/plugins/demo/"); rec.Code != 200 {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := string(tr.lastReq.Ubus["dhcpLeases"]); got != `{"leases":[]}` {
		t.Errorf("lease read = %s, want an empty list", got)
	}
}
