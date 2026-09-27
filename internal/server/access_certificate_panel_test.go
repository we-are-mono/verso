// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/we-are-mono/verso/internal/plugin"
)

// certificatePanelServer is the system plugin answering one of the
// certificate's acts: its form, in the drawer it opens in over Access, with
// the command that act runs when it was asked to.
func certificatePanelServer(t *testing.T, b *credentialsFake, commands []plugin.ApplyAction) *Server {
	t.Helper()
	b.access = true
	m := credentialManifest()
	m.Socket, m.SystemAccess, m.Name = "/run/system.sock", "/access", "System"
	m.SchemaVersion = supportedSchemaVersion
	widget := `{"type":"table","columns":[],"rows":[{"drawer":{"title":"Make a new certificate","open":true,"closed":"/system/access",
	  "children":[{"type":"form","submit":"Make certificate","fields":[{"type":"field","name":"hostname","label":"Router name","value":"router.lan"}]}]}}]}`
	env := &plugin.Envelope{SchemaVersion: 1, Title: "Make a new certificate", Widget: json.RawMessage(widget),
		Back: &plugin.PageAction{Label: "Access", Href: "/system/access"}, Commands: commands}
	if len(commands) > 0 {
		env.Notice = &plugin.Notice{Level: "success", Text: "Access credentials updated."}
	}
	return newServerWith(t, b, &fakeTransport{env: env}, []plugin.Manifest{m})
}

// TestAPanelOpenedOverAPagePostsToItsOwnAddress: a drawer opened over a page
// that keeps its address (a certificate's act over Access) is read from its
// own address, and its form posts back there — not to the page under it,
// whose address is still the one in the bar.
func TestAPanelOpenedOverAPagePostsToItsOwnAddress(t *testing.T) {
	s := certificatePanelServer(t, &credentialsFake{}, nil)
	rec := getPanel(t, s, "/plugins/system/access/certificate/new")
	body := rec.Body.String()
	if rec.Code != http.StatusOK || strings.Contains(body, "<html") {
		t.Fatalf("want the panel alone, got %d:\n%s", rec.Code, body)
	}
	for _, want := range []string{`hx-post="/plugins/system/access/certificate/new"`, `hx-target="closest [data-verso-panel]"`, "Make certificate"} {
		if !strings.Contains(body, want) {
			t.Errorf("want %s in:\n%s", want, body)
		}
	}
}

// TestAPanelActThatRanClosesOnThePageReadAgain: an act run from a drawer (a
// new certificate made) changed what the page under it shows, so the drawer
// closes on that page read again, with what happened said once there.
func TestAPanelActThatRanClosesOnThePageReadAgain(t *testing.T) {
	b := &credentialsFake{}
	s := certificatePanelServer(t, b, []plugin.ApplyAction{{Name: "certificate-generate", Args: map[string]string{"hostname": "router.lan"}}})
	rec, token := postPluginFromPanel(t, s, "/plugins/system/access/certificate/new", url.Values{"hostname": {"router.lan"}})
	if rec.Code != http.StatusOK || rec.Header().Get("HX-Redirect") != "/system/access" {
		t.Fatalf("want the frame sent to Access, got %d → %q:\n%s", rec.Code, rec.Header().Get("HX-Redirect"), rec.Body.String())
	}
	if !strings.Contains(b.cert, "BEGIN CERTIFICATE") {
		t.Errorf("the certificate was not made: %q", b.cert)
	}
	if _, message := s.sessions.TakeFlash(token); message != "Access credentials updated." {
		t.Errorf("flash = %q, want what happened, waiting on Access", message)
	}
}

// TestAPanelActTheRouterRefusedStaysInThePanel: an act the router refuses (a
// certificate and a key that do not match) is said in the drawer, over what
// was typed, rather than closing it.
func TestAPanelActTheRouterRefusedStaysInThePanel(t *testing.T) {
	b := &credentialsFake{}
	s := certificatePanelServer(t, b, []plugin.ApplyAction{{Name: "certificate-install",
		Args: map[string]string{"certificate": "-----BEGIN CERTIFICATE-----\nx\n-----END CERTIFICATE-----", "key": "-----BEGIN PRIVATE KEY-----\ny\n-----END PRIVATE KEY-----"}}})
	rec, _ := postPluginFromPanel(t, s, "/plugins/system/access/certificate/new", url.Values{"hostname": {"router.lan"}})
	body := rec.Body.String()
	if rec.Code != http.StatusUnprocessableEntity || rec.Header().Get("HX-Redirect") != "" {
		t.Fatalf("want the refusal in the panel at 422, got %d → %q:\n%s", rec.Code, rec.Header().Get("HX-Redirect"), body)
	}
	if strings.Contains(body, "<html") || !strings.Contains(body, "Make certificate") || strings.Contains(body, "Access credentials updated.") {
		t.Errorf("want the panel with the refusal and no success:\n%s", body)
	}
	if b.cert != "" {
		t.Errorf("a refused pair was installed: %q", b.cert)
	}
}

// TestAnAccessPartAskedForAsAPageIsAccess: the certificate's drawers are acts
// over Access, not pages of their own. Their address visited as a page lands
// on Access.
func TestAnAccessPartAskedForAsAPageIsAccess(t *testing.T) {
	s := certificatePanelServer(t, &credentialsFake{}, nil)
	rec := get(t, s, "/plugins/system/access/certificate/new")
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/system/access" {
		t.Fatalf("want Access, got %d → %q", rec.Code, rec.Header().Get("Location"))
	}
}
