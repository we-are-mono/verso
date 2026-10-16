// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"testing"

	"github.com/we-are-mono/verso/internal/widget"
)

func TestCookieLayout(t *testing.T) {
	var source layoutSource = cookieLayout{hardware: true}
	bare := httptest.NewRequest(http.MethodGet, "/", nil)
	if got := source.Layout(bare); !reflect.DeepEqual(got, widget.DefaultOverviewLayout()) {
		t.Errorf("no cookie = %+v, want the default layout", got)
	}

	saved := httptest.NewRequest(http.MethodGet, "/", nil)
	value := "1.traffic-internet-hardware.ports-tunnels-resources-system.system."
	saved.AddCookie(&http.Cookie{Name: overviewLayoutCookie, Value: value})
	if got := source.Layout(saved).Encode(); got != value {
		t.Errorf("saved layout = %q, want %q", got, value)
	}

	garbled := httptest.NewRequest(http.MethodGet, "/", nil)
	garbled.Header.Set("Cookie", overviewLayoutCookie+"=1.%%%.;;")
	if got := source.Layout(garbled); !reflect.DeepEqual(got, widget.DefaultOverviewLayout()) {
		t.Errorf("garbled cookie = %+v, want the default layout", got)
	}

	// A board with no profile has no Hardware card, whatever the cookie says.
	unprofiled := cookieLayout{}
	if got := unprofiled.Layout(bare); slices.Contains(got.Cols[1], "hardware") {
		t.Errorf("no profile, default = %+v, want no Hardware", got)
	}
	if got := unprofiled.Layout(saved).Encode(); got != "1.traffic-internet.ports-tunnels-resources-system.system." {
		t.Errorf("no profile, saved = %q, want Hardware dropped and the rest in place", got)
	}
}

func TestCookieLayoutAcceptsOnlyInstalledPluginCards(t *testing.T) {
	id := widget.PluginOverviewCardID("jodu-5174x")
	value := "1.internet-" + id + ".traffic-ports-tunnels-resources-system-hardware.."
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: overviewLayoutCookie, Value: value})
	got := (cookieLayout{hardware: true}).LayoutWithPlugins(req, []string{"jodu-5174x"})
	if got.Encode() != value {
		t.Fatalf("installed card layout = %q, want %q", got.Encode(), value)
	}
	unknown := (cookieLayout{hardware: true}).LayoutWithPlugins(req, nil)
	if slices.Contains(slices.Concat(unknown.Cols[0], unknown.Cols[1]), id) {
		t.Fatalf("unknown plugin card survived: %+v", unknown)
	}
}
