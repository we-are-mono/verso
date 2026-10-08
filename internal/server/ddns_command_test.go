// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"testing"

	"github.com/we-are-mono/verso/internal/plugin"
)

// ddnsBackend records the services it was asked to update now.
type ddnsBackend struct {
	fakeBackend
	updated *[]string
}

func (b ddnsBackend) DDNSUpdate(_ context.Context, _, section string) error {
	*b.updated = append(*b.updated, section)
	return nil
}

// TestDDNSUpdateRunsOnlyAsDeclared: a Dynamic DNS page's "update now" reaches
// the helper for a plugin that declared it, naming one section, and for no
// other plugin or name.
func TestDDNSUpdateRunsOnlyAsDeclared(t *testing.T) {
	var updated []string
	b := ddnsBackend{fakeBackend: fakeBackend{access: true}, updated: &updated}
	run := func(declared bool, section string) int {
		m := demoManifest()
		if declared {
			m.ACL.Write = []plugin.ACLScope{{Scope: "ubus", Object: "verso", Function: "ddnsUpdate"}}
		}
		env := &plugin.Envelope{SchemaVersion: 1, Title: "Dynamic DNS", Status: http.StatusOK,
			Widget:   json.RawMessage(`{"type":"text","markdown":"ddns"}`),
			Commands: []plugin.ApplyAction{{Name: "ddns-update", Args: map[string]string{"section": section}}}}
		s := newServerWith(t, b, &fakeTransport{env: env}, []plugin.Manifest{m})
		return postPlugin(t, s, "/plugins/demo/", url.Values{"update": {section}}).Code
	}

	if code := run(true, "home_example_com"); code >= 400 {
		t.Fatalf("a declared update answered %d", code)
	}
	if code := run(false, "home_example_com"); code < 400 {
		t.Errorf("an undeclared update answered %d, want refused", code)
	}
	if code := run(true, "home;reboot"); code < 400 {
		t.Errorf("an update naming %q answered %d, want refused", "home;reboot", code)
	}
	if len(updated) != 1 || updated[0] != "home_example_com" {
		t.Errorf("updated %v, want only the declared, well-named one", updated)
	}
}
