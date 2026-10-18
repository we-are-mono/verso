// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"net/http"
	"strings"
	"testing"
)

// TestSystemPanelsStandOnCards: a System page's settings drawn as the drawer
// over the page stand on cards on the drawer's sand, as every drawer's body
// does, never loose on a Paper ground.
func TestSystemPanelsStandOnCards(t *testing.T) {
	s := newServer(t, fakeBackend{access: true, uci: map[string]map[string]any{"system": {"main": map[string]any{".type": "system", "hostname": "router"}}}})
	withAlerter(t, s)
	for _, path := range []string{"/system/logs/settings", "/system/events/alerts", "/system/packages/discover"} {
		res := getPanel(t, s, path)
		body := res.Body.String()
		if res.Code != http.StatusOK {
			t.Errorf("%s: status %d", path, res.Code)
			continue
		}
		if strings.Contains(body, "data-verso-ground") {
			t.Errorf("%s: the drawer's body stands on Paper:\n%s", path, body)
		}
		ground := strings.Index(body, "verso-modal__ground")
		card := strings.Index(body, `class="verso-card`)
		if ground < 0 || card < ground {
			t.Errorf("%s: the drawer's body holds no card:\n%s", path, body)
		}
	}
}

// TestLogSettingsReadingIsACardOfItsOwn: how the log is read stands in a card
// above the settings' card, outside their form.
func TestLogSettingsReadingIsACardOfItsOwn(t *testing.T) {
	s := newServer(t, fakeBackend{access: true, uci: map[string]map[string]any{"system": {"main": map[string]any{".type": "system", "hostname": "router"}}}})
	body := getPanel(t, s, "/system/logs/settings").Body.String()
	first := strings.Index(body, `class="verso-card`)
	toggle := strings.Index(body, "data-log-include-firewall")
	form := strings.Index(body, "<form")
	if first < 0 || toggle < first || form < toggle || !strings.Contains(body[form:], `class="verso-card`) {
		t.Errorf("the reading should be a card above the settings form's card:\n%s", body)
	}
}
