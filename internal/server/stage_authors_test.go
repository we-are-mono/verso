// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"testing"
	"time"

	"github.com/we-are-mono/verso/internal/plugin"
)

// TestAuthorsRecordWhenAndWhere: a change's record says when it was staged
// and which page it was made on, so the Pending changes page can say how long
// ago and lead back to it.
func TestAuthorsRecordWhenAndWhere(t *testing.T) {
	clock := time.Date(2026, 10, 17, 9, 30, 0, 0, time.UTC)
	a := stageAuthors{now: func() time.Time { return clock }}
	m := plugin.Manifest{ID: "dnsdhcp", Name: "DNS & DHCP", Nav: []plugin.NavEntry{
		{Section: "Network", Label: "DHCP", Path: "/"},
		{Section: "Network", Label: "DNS", Path: "dns"},
	}}

	a.note("sid", "dhcp.lan.leasetime", authorAt(m, "/dns/records"))
	who, ok := a.of("sid", "dhcp.lan.leasetime")
	if !ok {
		t.Fatal("no record for the change just noted")
	}
	if !who.At.Equal(clock) {
		t.Errorf("At = %v, want the time it was staged, %v", who.At, clock)
	}
	if who.Href != "/plugins/dnsdhcp/dns" {
		t.Errorf("Href = %q, want the page it was made on, /plugins/dnsdhcp/dns", who.Href)
	}
	if who.Page != "DNS" {
		t.Errorf("Page = %q, want DNS", who.Page)
	}

	// Noted again later, the record keeps the newer time.
	clock = clock.Add(5 * time.Minute)
	a.note("sid", "dhcp.lan.leasetime", authorAt(m, "/"))
	if who, _ := a.of("sid", "dhcp.lan.leasetime"); !who.At.Equal(clock) || who.Href != "/plugins/dnsdhcp/" {
		t.Errorf("after a second save the record is %+v, want the later time and its page", who)
	}

	// A record carried with its own time (an undo's replay) keeps it.
	kept := clock.Add(-time.Hour)
	a.note("sid", "dhcp.lan.start", stageAuthor{Plugin: "dnsdhcp", Page: "DHCP", At: kept})
	if who, _ := a.of("sid", "dhcp.lan.start"); !who.At.Equal(kept) {
		t.Errorf("a replayed record's time = %v, want the one it carried, %v", who.At, kept)
	}

	a.forget("sid")
	if _, ok := a.of("sid", "dhcp.lan.leasetime"); ok {
		t.Error("forget left a record behind")
	}
}

// TestAuthorLeadsToItsSubpage: a change made on one of an entry's subpages,
// or in a drawer on it, leads back to that subpage, not the entry's first.
func TestAuthorLeadsToItsSubpage(t *testing.T) {
	m := plugin.Manifest{ID: "firewall", Name: "Firewall", Nav: []plugin.NavEntry{{
		Section: "Security", Label: "Firewall", Path: "/",
		Pages: plugin.NavPages{List: []plugin.PageTab{{Label: "Rules", Path: ""}, {Label: "Zones", Path: "zones"}}},
	}}}
	for path, want := range map[string]string{
		"/":          "/plugins/firewall/",
		"/zones":     "/plugins/firewall/zones",
		"/zones/lan": "/plugins/firewall/zones",
		"/cfg0992bd": "/plugins/firewall/",
	} {
		if got := authorAt(m, path).Href; got != want {
			t.Errorf("a change made at %q leads to %q, want %q", path, got, want)
		}
	}
}

// TestAccessAuthorLeadsToAccess: a change a plugin stages from its part of
// System › Access is filed under Access and leads back there.
func TestAccessAuthorLeadsToAccess(t *testing.T) {
	m := plugin.Manifest{ID: "system", Name: "System", SystemAccess: "/access"}
	who := authorAt(m, "/access")
	if who.Page != "Access" || who.Href != "/system/access" {
		t.Errorf("author = %+v, want Access at /system/access", who)
	}
}
