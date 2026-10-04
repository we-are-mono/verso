// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/we-are-mono/verso/internal/plugin"
)

func systemManifest() plugin.Manifest {
	return plugin.Manifest{
		ID: "system", Name: "System",
		Socket: "/var/run/verso/system.sock", SchemaVersion: 1,
		Nav: []plugin.NavEntry{{Section: "System", Label: "General", Path: "/"}},
		ACL: plugin.ACL{
			Read:  []plugin.ACLScope{{Scope: "uci", Object: "system", Function: "read"}},
			Write: []plugin.ACLScope{{Scope: "uci", Object: "system", Function: "write"}},
		},
	}
}

func TestSystemGeneralUsesBundledPluginRegistration(t *testing.T) {
	tr := &fakeTransport{env: &plugin.Envelope{
		SchemaVersion: 1, Title: "General", Status: http.StatusOK,
		Subheading: "The name, place, and clock shared by everything on this router.",
		Width:      "narrow",
		Widget: json.RawMessage(`{
			"type":"form","style":"page","fields":[
				{"type":"field","name":"hostname","label":"Hostname","value":"router"}
			]}`),
	}}
	s := newServerWith(t, fakeBackend{}, tr, []plugin.Manifest{systemManifest()})
	body := get(t, s, "/plugins/system/").Body.String()

	if !strings.Contains(body, `verso-page-heading">General</h1>`) {
		t.Error("a page in System keeps its own name as the heading, with no section prefix")
	}
	for _, want := range []string{
		`href="/system/access"`, `href="/system/packages"`,
		`href="/system/services"`, `href="/system/maintenance"`,
		`data-verso-page-form`, `name="hostname"`,
		// The form carries its own submit: the shell's, since General declares
		// none — pressing it stages, and the drawer applies.
		">Save changes</button>",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("System General missing %q", want)
		}
	}
	if tr.lastSocket != "/var/run/verso/system.sock" || tr.lastReq.Path != "" {
		t.Errorf("plugin request = socket %q path %q", tr.lastSocket, tr.lastReq.Path)
	}
}

func TestSystemRootRedirectsToFirstLiveRegisteredPage(t *testing.T) {
	s := newServerWith(t, fakeBackend{}, &fakeTransport{}, []plugin.Manifest{systemManifest()})
	rec := get(t, s, "/system")
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/plugins/system/" {
		t.Fatalf("redirect = %d %q", rec.Code, rec.Header().Get("Location"))
	}
}

func networkManifests() []plugin.Manifest {
	return []plugin.Manifest{
		{ID: "dnsdhcp", Name: "DNS and DHCP", Socket: "/var/run/verso/dnsdhcp.sock", SchemaVersion: 1,
			Nav: []plugin.NavEntry{{Section: "Network", Label: "DHCP", Path: "/"}}},
		{ID: "interfaces", Name: "Interfaces", Socket: "/var/run/verso/interfaces.sock", SchemaVersion: 1,
			Nav: []plugin.NavEntry{{Section: "Network", Label: "Interfaces", Path: "/"}}},
	}
}

// Network's root leads to its first live page, as System's does.
func TestNetworkRootRedirectsToFirstLiveRegisteredPage(t *testing.T) {
	s := newServerWith(t, fakeBackend{}, &fakeTransport{}, networkManifests())
	for _, path := range []string{"/network", "/network/"} {
		rec := get(t, s, path)
		if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/plugins/interfaces/" {
			t.Errorf("GET %s = %d %q, want a redirect to Interfaces", path, rec.Code, rec.Header().Get("Location"))
		}
	}
	s.probe = func(string) bool { return false }
	if rec := get(t, s, "/network"); rec.Code != http.StatusNotFound {
		t.Errorf("GET /network with nothing live = %d, want 404", rec.Code)
	}
}

// A plugin page filed under Network joins the Network frame: the page keeps its
// own name as the heading, as System's pages do, and the rail's Network row
// opens into every live Network page, whichever plugin files it.
func TestNetworkFiledPluginPageOpensTheNetworkRow(t *testing.T) {
	tr := &fakeTransport{env: &plugin.Envelope{
		SchemaVersion: 1, Title: "DHCP", Status: http.StatusOK,
		Widget: json.RawMessage(`{"type":"card","children":[]}`),
	}}
	s := newServerWith(t, fakeBackend{}, tr, networkManifests())
	body := get(t, s, "/plugins/dnsdhcp/").Body.String()
	body = body[strings.LastIndex(body, "</style>"):]
	nav := body[strings.Index(body, "<nav "):strings.Index(body, "</nav>")]
	for _, want := range []string{`href="/plugins/interfaces/"`, `href="/plugins/dnsdhcp/" aria-current="page"`, ">Network</span>"} {
		if !strings.Contains(nav, want) {
			t.Errorf("rail missing %q", want)
		}
	}
	if !strings.Contains(body, `verso-page-heading">DHCP</h1>`) {
		t.Error("a page in Network keeps its own name as the heading, with no section prefix")
	}
}

func TestStoppedSystemPluginWithdrawsGeneralRegistration(t *testing.T) {
	s := newServerWith(t, fakeBackend{}, &fakeTransport{}, []plugin.Manifest{systemManifest()})
	s.probe = func(string) bool { return false }

	rec := get(t, s, "/system")
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/system/hardware" {
		t.Fatalf("redirect with stopped plugin = %d %q", rec.Code, rec.Header().Get("Location"))
	}
	body := get(t, s, "/system/access").Body.String()
	// Route names can appear in the inlined stylesheet's selectors too; only
	// the rendered markup can leave a stopped plugin in the navigation.
	body = body[strings.LastIndex(body, "</style>")+len("</style>"):]
	if strings.Contains(body, `href="/plugins/system/"`) || strings.Contains(body, `>General</a>`) {
		t.Error("stopped System plugin must not leave its General registration in navigation")
	}
}

// The page appears without an entrance animation. Its content measure still
// sits inside the frame's padding.
func TestPageContentAppearsWithoutEntranceAnimation(t *testing.T) {
	s := newServer(t, fakeBackend{})
	whole := get(t, s, "/").Body.String()
	if strings.Contains(whole, "verso-page-enter") {
		t.Fatal("page loads must not carry an entrance animation")
	}
	body := whole[strings.LastIndex(whole, "</style>"):]
	// The homepage keeps the page inset every page keeps: 2.5rem from the top,
	// and from the sidebar once there is room for it.
	if !strings.Contains(body, `<div class="px-4 pt-10 pb-16 sm:px-6 md:px-10">`) {
		t.Error("the page's air is the frame's, outside the content measure")
	}
	if !strings.Contains(body, `<div class="max-w-`) {
		t.Error("the content measure must sit inside that air")
	}
}
