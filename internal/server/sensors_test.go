// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"testing"

	"github.com/we-are-mono/verso/internal/sensors"
)

func TestFormatSensors(t *testing.T) {
	// A fully-instrumented board: temperature rounds from milli-°C, power from
	// micro-watts, and the summary counts both kinds.
	v := formatSensors("Mono Gateway Development Kit", sensors.Facts{
		CPUTemp:      &sensors.Temp{MilliC: 75750, Status: "Warm", Level: "warning"},
		Fan:          &sensors.Fan{RPM: 3630},
		Power:        &sensors.Power{MicroW: 12400000},
		PowerCount:   8,
		ThermalCount: 5,
	})
	if v.Model != "Mono Gateway Development Kit" {
		t.Errorf("model: %q", v.Model)
	}
	if v.Temperature != "76 °C · Warm" || v.TempLevel != "warning" { // 75750 rounds to 76
		t.Errorf("temperature: %q level %q", v.Temperature, v.TempLevel)
	}
	if v.Fan != "3630 rpm" {
		t.Errorf("fan: %q", v.Fan)
	}
	if v.Power != "12.4 W" {
		t.Errorf("power: %q", v.Power)
	}
	if v.Summary != "8 power · 5 thermal" {
		t.Errorf("summary: %q", v.Summary)
	}
}

func TestFormatSensorsHidesAbsent(t *testing.T) {
	// An unprofiled PC: temperature only, no fan, no power. Blank fields hide
	// their rows; the summary drops the kind that has no sensors.
	v := formatSensors("", sensors.Facts{
		CPUTemp:      &sensors.Temp{MilliC: 45000, Status: "Normal", Level: "success"},
		ThermalCount: 3,
	})
	if v.Temperature != "45 °C · Normal" {
		t.Errorf("temperature: %q", v.Temperature)
	}
	if v.Fan != "" || v.Power != "" {
		t.Errorf("absent fan/power must be blank: fan=%q power=%q", v.Fan, v.Power)
	}
	if v.Summary != "3 thermal" {
		t.Errorf("summary should drop the empty power kind: %q", v.Summary)
	}
}
