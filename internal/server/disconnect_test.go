// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"testing"

	"github.com/we-are-mono/verso/internal/openwrt"
)

func viewOf(changes map[string][][]string) stagedView {
	var items []stagedItem
	for config, list := range changes {
		for _, ch := range list {
			items = append(items, stagedItem{changes: []stagedChange{{config: config, tuple: ch}}})
		}
	}
	return stagedView{Count: len(items), Groups: []stagedGroup{{Items: items}}}
}

var labIfaces = []openwrt.NetIface{
	{Name: "lan", Device: "br-lan", Up: true, Addresses: []string{"192.168.1.1"}},
	{Name: "wan", Device: "wan0", Up: true, Addresses: []string{"172.30.1.171"}},
}

// TestALANMoveMayDisconnect: the browser reaches the router at 192.168.1.1,
// LAN's address; a pending change that moves LAN's address may cut it off,
// and the warning names both addresses.
func TestALANMoveMayDisconnect(t *testing.T) {
	view := viewOf(map[string][][]string{"network": {{"set", "lan", "ipaddr", "10.0.0.1"}}})
	w := disconnectOf(view, "192.168.1.1", labIfaces)
	if w == nil {
		t.Fatal("a move of the browser's own address is not flagged")
	}
	if w.From != "192.168.1.1" || w.To != "10.0.0.1" || w.Interface != "lan" {
		t.Errorf("warning = %+v, want lan moving from 192.168.1.1 to 10.0.0.1", w)
	}
	// A prefix written with the address is the address.
	view = viewOf(map[string][][]string{"network": {{"set", "lan", "ipaddr", "10.0.0.1/24"}}})
	if w := disconnectOf(view, "192.168.1.1", labIfaces); w == nil || w.To != "10.0.0.1" {
		t.Errorf("warning = %+v, want the address without its prefix", w)
	}
}

// TestAnyChangeToTheBrowsersInterfaceMayDisconnect: its protocol, or the
// interface going, is as much a risk as its address; the warning then names
// no new address.
func TestAnyChangeToTheBrowsersInterfaceMayDisconnect(t *testing.T) {
	for name, changes := range map[string][][]string{
		"protocol": {{"set", "lan", "proto", "dhcp"}},
		"removed":  {{"remove", "lan"}},
	} {
		w := disconnectOf(viewOf(map[string][][]string{"network": changes}), "192.168.1.1", labIfaces)
		if w == nil || w.From != "192.168.1.1" || w.To != "" {
			t.Errorf("%s: warning = %+v, want lan flagged with no new address", name, w)
		}
	}
}

// TestOtherChangesDoNotDisconnect: a change to another interface, to another
// config, or a browser the shell cannot place, flags nothing.
func TestOtherChangesDoNotDisconnect(t *testing.T) {
	for name, c := range map[string]struct {
		changes map[string][][]string
		local   string
	}{
		"another interface":  {map[string][][]string{"network": {{"set", "wan", "mtu", "1492"}}}, "192.168.1.1"},
		"another config":     {map[string][][]string{"dhcp": {{"set", "lan", "leasetime", "6h"}}}, "192.168.1.1"},
		"an unknown address": {map[string][][]string{"network": {{"set", "lan", "ipaddr", "10.0.0.1"}}}, "10.9.9.9"},
		"no address":         {map[string][][]string{"network": {{"set", "lan", "ipaddr", "10.0.0.1"}}}, ""},
	} {
		if w := disconnectOf(viewOf(c.changes), c.local, labIfaces); w != nil {
			t.Errorf("%s: flagged %+v", name, w)
		}
	}
}
