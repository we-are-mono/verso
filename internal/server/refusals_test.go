// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.
package server

import (
	"strings"
	"testing"

	"github.com/we-are-mono/verso/internal/plugin"
)

// TestEveryPageCarriesTheRefusalNavigator: the navigator is chrome, drawn by
// the server on every page and hidden until the page's own fields say
// something was refused, with its words from the catalog and its script's
// words in the page's string blob.
func TestEveryPageCarriesTheRefusalNavigator(t *testing.T) {
	s := newServerWith(t, fakeBackend{access: true}, &fakeTransport{}, []plugin.Manifest{demoACLManifest()})
	body := get(t, s, "/").Body.String()
	for _, want := range []string{
		`<div id="verso-refusals" hidden role="status"`,
		`data-verso-refusals-count`,
		`data-verso-refusals-next`,
		`"%d settings refused"`, `"Ready to save"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("page missing %q", want)
		}
	}
}
