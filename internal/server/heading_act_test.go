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

// TestPluginActWithNothingToNarrowSitsOnTheHeadingLine: a listing with nothing
// to search declares its bar with only the act, and the page puts that act on
// the heading line — the canvas's rule — instead of a toolbar row holding one
// button.
func TestPluginActWithNothingToNarrowSitsOnTheHeadingLine(t *testing.T) {
	tr := &fakeTransport{env: &plugin.Envelope{
		SchemaVersion: 1, Title: "Wireless",
		Widget: json.RawMessage(`{"type":"stack","children":[
			{"type":"actionbar","action":{"label":"Add network","href":"/plugins/demo/?open=new","icon":"plus"},"opens_panel":true},
			{"type":"table","columns":[{"label":"Network"}],"rows":[],"empty_text":"No wireless networks yet."}
		]}`),
	}}
	s := newServerWith(t, fakeBackend{}, tr, []plugin.Manifest{demoManifest()})

	rec := get(t, s, "/plugins/demo/")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
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
		t.Error("a bar with nothing to narrow still drew a toolbar row")
	}
	if !strings.Contains(body[heading:listing], `verso-page-body`) {
		t.Error("the act should render in the heading row, ahead of the page body")
	}
}
