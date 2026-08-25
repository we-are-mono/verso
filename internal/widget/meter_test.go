// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"strings"
	"testing"
)

func TestRenderMeter(t *testing.T) {
	r := newRenderer(t)
	got := render(t, r, &Meter{Label: "Storage", Value: "23", Unit: "GB", Fill: 72, Detail: "9 GB free"})
	for _, want := range []string{
		"Storage", ">23<", ">GB<", "9 GB free",
		`r="50"`,          // a full ring
		"stroke-dasharray", // filled by Fill
		"tabular-nums",
		"stroke-green-600", // 72% is still healthy (amber starts at 80)
	} {
		if !strings.Contains(got, want) {
			t.Errorf("meter missing %q:\n%s", want, got)
		}
	}
}

// TestMeterBands: the ring colours itself from Fill (not from the displayed Value).
func TestMeterBands(t *testing.T) {
	r := newRenderer(t)
	cases := map[int]string{45: "stroke-green-600", 85: "stroke-amber-500", 95: "stroke-red-600"}
	for fill, want := range cases {
		got := render(t, r, &Meter{Label: "x", Value: "x", Fill: fill})
		if !strings.Contains(got, want) {
			t.Errorf("fill %d: want %q in:\n%s", fill, want, got)
		}
	}
}

// TestMeterInfoVariant: "info" forces the accent and shows the value in its own unit,
// bypassing the fill-based auto-colour (a rate like speed has no "getting full").
func TestMeterInfoVariant(t *testing.T) {
	r := newRenderer(t)
	got := render(t, r, &Meter{Label: "Speed", Value: "300", Unit: "Mbps", Fill: 30, Variant: "info", Detail: "24 Mbps up"})
	for _, want := range []string{">300<", ">Mbps<", "stroke-sky-600", "24 Mbps up"} {
		if !strings.Contains(got, want) {
			t.Errorf("speed meter missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "stroke-green-600") {
		t.Errorf("info variant must not auto-colour:\n%s", got)
	}
}

// TestMeterClampsFill: a Fill over 100 fills the whole ring.
func TestMeterClampsFill(t *testing.T) {
	r := newRenderer(t)
	got := render(t, r, &Meter{Label: "x", Value: "x", Fill: 150})
	if !strings.Contains(got, "314.2") { // 100% of the 314.16 circumference, rounded
		t.Errorf("fill>100 should fill the ring:\n%s", got)
	}
}
