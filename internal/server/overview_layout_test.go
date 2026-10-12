// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/we-are-mono/verso/internal/widget"
)

func TestCookieLayout(t *testing.T) {
	var source layoutSource = cookieLayout{}
	bare := httptest.NewRequest(http.MethodGet, "/", nil)
	if got := source.Layout(bare); !reflect.DeepEqual(got, widget.DefaultOverviewLayout()) {
		t.Errorf("no cookie = %+v, want the default layout", got)
	}

	saved := httptest.NewRequest(http.MethodGet, "/", nil)
	value := "1.traffic-internet.ports-tunnels-resources-hardware-firmware.firmware."
	saved.AddCookie(&http.Cookie{Name: overviewLayoutCookie, Value: value})
	if got := source.Layout(saved).Encode(); got != value {
		t.Errorf("saved layout = %q, want %q", got, value)
	}

	garbled := httptest.NewRequest(http.MethodGet, "/", nil)
	garbled.Header.Set("Cookie", overviewLayoutCookie+"=1.%%%.;;")
	if got := source.Layout(garbled); !reflect.DeepEqual(got, widget.DefaultOverviewLayout()) {
		t.Errorf("garbled cookie = %+v, want the default layout", got)
	}
}
