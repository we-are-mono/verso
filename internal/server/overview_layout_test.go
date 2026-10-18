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
	if got, want := source.Layout(saved), widget.ParseOverviewLayout(value); !reflect.DeepEqual(got, want) {
		t.Errorf("saved layout = %+v, want %+v", got, want)
	}

	garbled := httptest.NewRequest(http.MethodGet, "/", nil)
	garbled.Header.Set("Cookie", overviewLayoutCookie+"=1.%%%.;;")
	if got := source.Layout(garbled); !reflect.DeepEqual(got, widget.DefaultOverviewLayout()) {
		t.Errorf("garbled cookie = %+v, want the default layout", got)
	}

	// A browser still carrying the layout under its former name keeps it until
	// the page writes the current one, which then wins.
	former := httptest.NewRequest(http.MethodGet, "/", nil)
	former.AddCookie(&http.Cookie{Name: formerOverviewLayoutCookie, Value: value})
	if got, want := source.Layout(former), widget.ParseOverviewLayout(value); !reflect.DeepEqual(got, want) {
		t.Errorf("former cookie = %+v, want %+v", got, want)
	}
	both := httptest.NewRequest(http.MethodGet, "/", nil)
	both.AddCookie(&http.Cookie{Name: formerOverviewLayoutCookie, Value: value})
	current := "1.internet-ports-tunnels.traffic-resources-system-hardware..resources"
	both.AddCookie(&http.Cookie{Name: overviewLayoutCookie, Value: current})
	if got, want := source.Layout(both), widget.ParseOverviewLayout(current); !reflect.DeepEqual(got, want) {
		t.Errorf("both cookies = %+v, want the current cookie's layout %+v", got, want)
	}

	// A board with no profile has no Hardware card, whatever the cookie says.
	unprofiled := cookieLayout{}
	if got := unprofiled.Layout(bare); slices.Contains(got.Cols[1], "hardware") {
		t.Errorf("no profile, default = %+v, want no Hardware", got)
	}
	if got, want := unprofiled.Layout(saved), widget.ParseOverviewLayout("1.traffic-internet.ports-tunnels-resources-system.system.").Without("hardware"); !reflect.DeepEqual(got, want) {
		t.Errorf("no profile, saved = %+v, want Hardware dropped and the rest in place", got)
	}

	// An installed plugin's card is known to the layout: a cookie naming it
	// keeps it where it put it.
	acme := widget.OverviewPlacement{ID: "acme.card"}
	placed := widget.DefaultOverviewLayout(acme).Without("acme.card")
	placed.Cols[0] = append([]string{"acme.card"}, placed.Cols[0]...)
	withCard := httptest.NewRequest(http.MethodGet, "/", nil)
	withCard.AddCookie(&http.Cookie{Name: overviewLayoutCookie, Value: placed.Encode()})
	if got := source.Layout(withCard, acme); got.Cols[0][0] != "acme.card" {
		t.Errorf("plugin card = %+v, want it first on the left", got)
	}
}
