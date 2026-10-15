// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/we-are-mono/verso/internal/palette"
)

// A takeover declares the browser's bar colour as its own ground, and takes the
// value from the palette rather than keeping a copy: given a palette whose
// Canvas and Paper are unlike any shipped value, each takeover declares those.
func TestTakeoversTakeTheirBarColourFromThePalette(t *testing.T) {
	s := newServer(t, fakeBackend{})
	s.palette = palette.Palette{"canvas": "#123456", "ground": "#abcdef"}
	meta := func(v string) string { return `<meta name="theme-color" content="` + v + `">` }

	started := httptest.NewRecorder()
	s.renderFirmwareStarted(started, httptest.NewRequest("POST", "/system/maintenance/firmware", nil))
	if !strings.Contains(started.Body.String(), meta("#123456")) {
		t.Error("the firmware-started takeover does not declare the palette's Canvas as its bar colour")
	}

	upgrading := httptest.NewRecorder()
	s.renderUpgrading(upgrading, httptest.NewRequest("GET", "/", nil))
	if !strings.Contains(upgrading.Body.String(), meta("#123456")) {
		t.Error("the upgrading takeover does not declare the palette's Canvas as its bar colour")
	}

	restarting, err := s.restartingPage(httptest.NewRequest("POST", "/", nil), restartPlan{Title: "Restarting"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(restarting, meta("#abcdef")) {
		t.Error("the restarting takeover does not declare the palette's Paper as its bar colour")
	}
}
