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
		Nav: []plugin.NavEntry{navAt(10, plugin.NavEntry{Section: "System", Label: "General", Path: "/"})},
		ACL: plugin.ACL{
			Read:  []plugin.ACLScope{{Scope: "uci", Object: "system", Function: "read"}},
			Write: []plugin.ACLScope{{Scope: "uci", Object: "system", Function: "write"}},
		},
	}
}

func TestSystemGeneralUsesBundledPluginRegistration(t *testing.T) {
	tr := &fakeTransport{env: &plugin.Envelope{
		SchemaVersion: 1, Title: "General", Status: http.StatusOK,
		Width: "narrow",
		Widget: json.RawMessage(`{
			"type":"form","style":"page","fields":[
				{"type":"field","name":"hostname","label":"Hostname","value":"router"}
			]}`),
	}}
	s := newServerWith(t, fakeBackend{}, tr, []plugin.Manifest{systemManifest()})
	body := get(t, s, "/plugins/system/").Body.String()

	if !strings.Contains(body, `verso-page__heading">General</h1>`) {
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
			Nav: []plugin.NavEntry{navAt(20, plugin.NavEntry{Section: "Network", Label: "DHCP", Path: "/"})}},
		{ID: "interfaces", Name: "Interfaces", Socket: "/var/run/verso/interfaces.sock", SchemaVersion: 1,
			Nav: []plugin.NavEntry{navAt(10, plugin.NavEntry{Section: "Network", Label: "Interfaces", Path: "/"})}},
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
	s.probe = func(plugin.Endpoint) bool { return false }
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
	if !strings.Contains(body, `verso-page__heading">DHCP</h1>`) {
		t.Error("a page in Network keeps its own name as the heading, with no section prefix")
	}
}

func TestStoppedSystemPluginWithdrawsGeneralRegistration(t *testing.T) {
	s := newServerWith(t, fakeBackend{}, &fakeTransport{}, []plugin.Manifest{systemManifest()})
	s.probe = func(plugin.Endpoint) bool { return false }

	rec := get(t, s, "/system")
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/system/hardware" {
		t.Fatalf("redirect with stopped plugin = %d %q", rec.Code, rec.Header().Get("Location"))
	}
	body := get(t, s, "/system/access").Body.String()
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
	// The homepage keeps the air every page keeps: its heading line 24px under
	// the top bar, 40px from the sidebar once there is room for it and 16px on
	// a phone, so the cards' frames land on the grid's lines.
	frame := strings.Index(body, `<div class="verso-page__frame">`)
	main := body[strings.Index(body, "<main "):]
	if !strings.Contains(main[:strings.Index(main, ">")+1], ` data-verso-home>`) || frame < 0 {
		t.Error("the page's air is the frame's, outside the content measure")
	}
	page := blockSheet(t, "page")
	if !strings.Contains(cssBlock(t, page, "\n.verso-page__frame {"), "padding: 0 var(--verso-gutter) calc(var(--spacing) * 10);") ||
		!strings.Contains(cssBlock(t, page, "\n.verso-page__frame:where(.verso-page__main[data-verso-home] *) {"), "padding-block: calc(var(--spacing) * 6) calc(var(--spacing) * 20);") {
		t.Error("the homepage's heading line does not stand 24px under the top bar, in the page's side air")
	}
	// Its h1 is every page's heading, on a control's line, and the cards
	// stand 24px under it as they do under any heading line.
	if !strings.Contains(body, `<h1 class="verso-page__heading text-balance">`) || !strings.Contains(body, `<div class="verso-overview__cols">`) {
		t.Error("the homepage's heading is not every page's heading")
	}
	if !strings.Contains(cssBlock(t, blockSheet(t, "overview"), "\n.verso-overview__cols:not(:first-child) {"), "margin-top: var(--spacing-inset);") {
		t.Error("the cards do not stand 24px under the heading line")
	}
	column := strings.Index(body, `<div data-verso-column class="verso-page__column">`)
	if column < frame {
		t.Error("the content measure must sit inside that air")
	}
	if !strings.Contains(cssBlock(t, page, "\n.verso-page__column {"), "--verso-column-max: calc(var(--container-5xl) + 1px);") {
		t.Error("the content measure is not capped")
	}
}
