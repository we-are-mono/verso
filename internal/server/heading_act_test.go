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

// TestHeadingLineIsAnActsHeight: the heading line is an act's height whether
// it holds one or not, so moving from a page with an act (Packages) to one
// without (Services) never shifts the heading or the band under it.
func TestHeadingLineIsAnActsHeight(t *testing.T) {
	s := newServer(t, fakeBackend{access: true})
	for _, path := range []string{"/system/services", "/system/packages"} {
		whole := get(t, s, path).Body.String()
		if !strings.Contains(whole[strings.LastIndex(whole, "</style>"):], `<div class="flex min-h-9 flex-wrap items-center justify-between gap-5">`) {
			t.Errorf("%s: the heading line should hold an act's height", path)
		}
	}
}

// TestPluginActSitsOnTheHeadingLine: a page's act is the envelope's own, and
// the page puts it on the heading line — the canvas's rule — with no toolbar
// row under the heading.
func TestPluginActSitsOnTheHeadingLine(t *testing.T) {
	tr := &fakeTransport{env: &plugin.Envelope{
		SchemaVersion: 1, Title: "Wireless",
		Act:    json.RawMessage(`{"label":"Add network","href":"/plugins/demo/?open=new","icon":"plus","opens_panel":true}`),
		Widget: json.RawMessage(`{"type":"table","columns":[{"label":"Network"}],"rows":[],"empty_text":"No wireless networks yet."}`),
	}}
	s := newServerWith(t, fakeBackend{}, tr, []plugin.Manifest{demoManifest()})

	rec := get(t, s, "/plugins/demo/")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	// The markup after the inlined stylesheet, whose selectors name the hooks
	// the markup would carry.
	whole := rec.Body.String()
	body := whole[strings.LastIndex(whole, "</style>"):]
	heading := strings.Index(body, ">Wireless</h1>")
	act := strings.Index(body, ">Add network<")
	listing := strings.Index(body, "No wireless networks yet.")
	if heading < 0 || act < 0 || listing < 0 {
		t.Fatalf("page is missing its heading, act or listing:\n%s", body)
	}
	if !(heading < act && act < listing) {
		t.Errorf("the act should sit between the heading and the listing (h1 %d, act %d, listing %d)", heading, act, listing)
	}
	if strings.Contains(body, "data-verso-actionbar") {
		t.Error("a page with nothing to narrow drew a toolbar row")
	}
	if !strings.Contains(body[heading:listing], `verso-page-body`) {
		t.Error("the act should render in the heading row, ahead of the page body")
	}
}

// TestTheActsBlankPanelAnswersAPanelRequest: making one is editing one that
// does not exist yet, so an act carries the blank object's panel, and the
// frame that asks for the act's address gets that panel alone — as it would a
// row's.
func TestTheActsBlankPanelAnswersAPanelRequest(t *testing.T) {
	tr := &fakeTransport{env: &plugin.Envelope{
		SchemaVersion: 1, Title: "Wireless",
		Act: json.RawMessage(`{"label":"Add network","href":"/plugins/demo/?open=new",
			"drawer":{"title":"New network","open":true,"closed":"/plugins/demo/","children":[{"type":"text","markdown":"A blank network."}]}}`),
		Widget: json.RawMessage(`{"type":"table","columns":[{"label":"Network"}],"rows":[]}`),
	}}
	s := newServerWith(t, fakeBackend{}, tr, []plugin.Manifest{demoManifest()})
	body := getPanel(t, s, "/plugins/demo/?open=new").Body.String()
	if !strings.Contains(body, "A blank network.") {
		t.Errorf("the act's blank panel should answer the panel request:\n%s", body)
	}
	if strings.Contains(body, "verso-page-heading") {
		t.Errorf("a panel request is answered with the panel alone:\n%s", body)
	}
}
