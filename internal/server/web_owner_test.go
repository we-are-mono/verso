// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/we-are-mono/verso/internal/listen"
	"github.com/we-are-mono/verso/internal/openwrt"
)

// webOwnerServer is Access on a router with uhttpd installed when withLuCI,
// with Verso holding its ports as ports says.
func webOwnerServer(t *testing.T, ports string, withLuCI bool, owner func(string) error) *Server {
	t.Helper()
	rc := map[string]openwrt.RCState{"dropbear": {}}
	if withLuCI {
		rc["uhttpd"] = openwrt.RCState{}
	}
	s := newServerFull(t, fakeBackend{rcStates: rc, webOwner: owner}, &fakeTransport{}, nil, fakeAuth{sid: "s"})
	s.SetWebPorts(ports)
	return s
}

func TestAccessOffersTheRoutersWebAddressAsVersoHoldsIt(t *testing.T) {
	for _, tc := range []struct {
		name, ports string
		luci        bool
		want        string
	}{
		{"beside LuCI", listen.PortsBeside, true, "Make this the router’s web interface"},
		{"holding the router's ports", listen.PortsOwn, true, "Hand the web interface back to LuCI"},
	} {
		body := get(t, webOwnerServer(t, tc.ports, tc.luci, nil), "/system/access").Body.String()
		if !strings.Contains(body, tc.want) || !strings.Contains(body, `action="/system/access/web-owner"`) {
			t.Errorf("%s: Access does not offer %q", tc.name, tc.want)
		}
	}
	for _, tc := range []struct {
		name, ports string
		luci        bool
	}{
		{"no LuCI installed", listen.PortsOwn, false},
		{"listeners set in verso.web", listen.PortsSet, true},
	} {
		if body := get(t, webOwnerServer(t, tc.ports, tc.luci, nil), "/system/access").Body.String(); strings.Contains(body, "/system/access/web-owner") {
			t.Errorf("%s: Access offers to move the web interface", tc.name)
		}
	}
}

func TestHandingTheRoutersPortsOverMovesToWhereVersoWillAnswer(t *testing.T) {
	for _, tc := range []struct {
		ports, owner, target string
	}{
		{listen.PortsBeside, "verso", "https://example.com/"},
		{listen.PortsOwn, "luci", "https://example.com:8443/"},
	} {
		var asked string
		s := webOwnerServer(t, tc.ports, true, func(owner string) error { asked = owner; return nil })
		rec := postPlugin(t, s, "/system/access/web-owner", url.Values{"owner": {tc.owner}})
		if rec.Code != http.StatusOK || asked != tc.owner {
			t.Fatalf("%s: %d, the router asked for %q", tc.owner, rec.Code, asked)
		}
		body := rec.Body.String()
		if !strings.Contains(body, `data-verso-restarting-target="`+tc.target+`"`) {
			t.Errorf("%s: the takeover does not move to %s:\n%s", tc.owner, tc.target, body)
		}
	}
}

func TestTheRoutersPortsMoveOnlyTheWayTheyCan(t *testing.T) {
	for _, tc := range []struct {
		name, ports, owner string
		luci               bool
	}{
		{"an owner that is neither", listen.PortsBeside, "nginx", true},
		{"to Verso while it holds them", listen.PortsOwn, "verso", true},
		{"to LuCI while it holds them", listen.PortsBeside, "luci", true},
		{"with listeners set", listen.PortsSet, "luci", true},
		{"with no LuCI to take them", listen.PortsOwn, "luci", false},
	} {
		asked := false
		s := webOwnerServer(t, tc.ports, tc.luci, func(string) error { asked = true; return nil })
		rec := postPlugin(t, s, "/system/access/web-owner", url.Values{"owner": {tc.owner}})
		if rec.Code < http.StatusBadRequest || asked {
			t.Errorf("%s: %d, router asked %v", tc.name, rec.Code, asked)
		}
	}
}

func TestARouterThatRefusesTheMoveKeepsThePersonWhereTheyAre(t *testing.T) {
	s := webOwnerServer(t, listen.PortsBeside, true, func(string) error { return errors.New("helper down") })
	rec := postPlugin(t, s, "/system/access/web-owner", url.Values{"owner": {"verso"}})
	if rec.Code != http.StatusBadGateway || strings.Contains(rec.Body.String(), "data-verso-restarting") {
		t.Fatalf("a refused move is not said as one: %d\n%s", rec.Code, rec.Body.String())
	}
}
