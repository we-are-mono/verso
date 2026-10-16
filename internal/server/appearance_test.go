// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
	"testing"

	"github.com/we-are-mono/verso/internal/plugin"
	"github.com/we-are-mono/verso/internal/theme"
)

// appearancePage is a plugin page that leaves the Appearance slot between
// its two sections, as the system plugin's General does, and a slot the shell
// has nothing for.
func appearancePage(t *testing.T, choice string) *Server {
	t.Helper()
	tr := &fakeTransport{env: &plugin.Envelope{SchemaVersion: 1, Title: "General",
		Widget: json.RawMessage(`{"type":"form","style":"settings","submit":"Save settings","fields":[
			{"type":"section","title":"","children":[{"type":"field","name":"hostname","label":"Router name","value":"verso-lab"}]},
			{"type":"slot","id":"appearance"},
			{"type":"slot","id":"nothing-here"},
			{"type":"section","title":"Time","children":[]}]}`)}}
	s := newServerWith(t, fakeBackend{}, tr, []plugin.Manifest{demoManifest()})
	s.useThemes(theme.Set{testMocha.ID: testMocha, testSandDark.ID: testSandDark}, choice)
	return s
}

func TestTheAppearanceCardStandsWhereThePluginLeftIt(t *testing.T) {
	rec := get(t, appearancePage(t, theme.Sand), "/plugins/demo/")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	body := rec.Body.String()
	name, appearance, timeCard := strings.Index(body, "Router name"), strings.Index(body, ">Appearance<"), strings.Index(body, ">Time<")
	if appearance < 0 {
		t.Fatalf("the page carries no Appearance card")
	}
	if name >= appearance || appearance >= timeCard {
		t.Errorf("the Appearance card does not stand between the router's settings and Time (at %d, %d, %d)", name, appearance, timeCard)
	}
	if strings.Count(body, ">Appearance<") != 1 {
		t.Error("the Appearance card is drawn more than once; the slot with no block should draw nothing")
	}
}

func TestTheAppearanceCardOffersEveryTheme(t *testing.T) {
	body := get(t, appearancePage(t, testMocha.ID), "/plugins/demo/").Body.String()
	for _, want := range []string{
		`<option value="sand">Sand, follows the browser</option>`,
		`<option value="sand-light">Sand, always light</option>`,
		`<option value="sand-dark">Sand, always dark</option>`,
		`<option value="catppuccin-mocha" selected>Catppuccin Mocha</option>`,
		"Takes effect immediately, for everyone who signs in to this router.",
		">theme<", // the option's key, as its chip
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the Appearance card lacks %s", want)
		}
	}
	// Saying it takes effect at once is the pick's own note, kept under the
	// select and read with it, not a paragraph standing on its own.
	if !strings.Contains(body, `id="theme-desc"`) || !strings.Contains(body, `aria-describedby="theme-desc"`) {
		t.Error("the Appearance card's sentence is not the theme pick's note")
	}
	// Sand dark is offered once, as Sand's own, not again as a theme.
	if strings.Count(body, `value="sand-dark"`) != 1 {
		t.Error("Sand dark is offered twice")
	}
}

func TestTheThemeSelectIsNoPartOfThePluginsForm(t *testing.T) {
	body := get(t, appearancePage(t, theme.Sand), "/plugins/demo/").Body.String()
	sel := regexp.MustCompile(`<select[^>]*data-verso-theme[^>]*>`).FindString(body)
	if sel == "" {
		t.Fatal("the theme select carries no data-verso-theme")
	}
	if strings.Contains(sel, "name=") {
		t.Errorf("the theme select has a name, so the plugin's form would submit it: %s", sel)
	}
	if !strings.Contains(body, `<option value="sand" selected>`) {
		t.Error("with no theme chosen the card does not name Sand following the browser")
	}
}
