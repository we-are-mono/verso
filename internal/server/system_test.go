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
		ManifestVersion: 1, ID: "system", Name: "System",
		Socket: "/var/run/verso/system.sock", SchemaVersion: 1,
		Nav: []plugin.NavEntry{{Section: "System", Label: "General", Path: "/"}},
		ACL: plugin.ACL{
			Read:  []plugin.ACLScope{{Scope: "uci", Object: "system", Function: "read"}},
			Write: []plugin.ACLScope{{Scope: "uci", Object: "system", Function: "write"}},
		},
	}
}

func TestSystemGeneralUsesBundledPluginBehindStableRoute(t *testing.T) {
	tr := &fakeTransport{env: &plugin.Envelope{
		SchemaVersion: 1, Title: "ignored plugin title", Status: http.StatusOK,
		Subheading: "The name, place, and clock shared by everything on this router.",
		Width:      "narrow",
		Widget: json.RawMessage(`{
			"type":"form","style":"page","fields":[
				{"type":"field","name":"hostname","label":"Hostname","value":"router"}
			]}`),
	}}
	s := newServerWith(t, fakeBackend{}, tr, []plugin.Manifest{systemManifest()})
	body := get(t, s, "/system/general").Body.String()

	for _, want := range []string{
		"System", "— General", `href="/system/access"`, `href="/system/packages"`,
		`href="/system/services"`, `href="/system/maintenance"`,
		`data-verso-page-form`, `name="hostname"`, `id="verso-capsule-close"`,
		`aria-label="Close change review"`, `verso-capsule-review`, `aria-hidden="true"`, `bottom-full`, `px-8`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("System General missing %q", want)
		}
	}
	if tr.lastSocket != "/var/run/verso/system.sock" || tr.lastReq.Path != "" {
		t.Errorf("plugin request = socket %q path %q", tr.lastSocket, tr.lastReq.Path)
	}
	if !strings.Contains(body, `id="verso-capsule-apply" disabled`) {
		t.Error("General's Save & Apply must remain disabled until the form differs from its rendered state")
	}
	if strings.Contains(body, `-bottom-2 left-1/2`) {
		t.Error("General must use the attached review tray, not a tooltip pointer")
	}
}

func TestSystemRootRedirectsToGeneral(t *testing.T) {
	s := newServer(t, fakeBackend{})
	rec := get(t, s, "/system")
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/system/general" {
		t.Fatalf("redirect = %d %q", rec.Code, rec.Header().Get("Location"))
	}
}

func TestWholePageContentCarriesEntranceAnimation(t *testing.T) {
	s := newServer(t, fakeBackend{})
	body := get(t, s, "/").Body.String()
	if !strings.Contains(body, `class="verso-page-enter max-w-`) {
		t.Error("title, content, and capsule must share the page entrance wrapper")
	}
	if strings.Contains(body, `<div class="verso-page-enter">`) {
		t.Error("the body alone must not carry the entrance animation")
	}
}
