// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"encoding/json"
	"testing"

	"github.com/we-are-mono/verso/internal/sensors"
)

func TestFormatSensors(t *testing.T) {
	// A fully-instrumented board: temperature rounds from milli-°C, power from
	// micro-watts.
	v := formatSensors(identityTranslator, "Mono Gateway Development Kit", sensors.Facts{
		CPUTemp: &sensors.Temp{MilliC: 75750, Status: "Warm", Level: "warning"},
		Fan:     &sensors.Fan{RPM: 3630},
		Power:   &sensors.Power{MicroW: 12400000},
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
}

func TestFormatSensorsAbsent(t *testing.T) {
	// An unprofiled PC keeps its system temperature and shows N/A for the rest.
	v := formatSensors(identityTranslator, "", sensors.Facts{
		CPUTemp: &sensors.Temp{MilliC: 45000, Status: "Normal", Level: "success"},
	})
	if v.Temperature != "45 °C · Normal" {
		t.Errorf("temperature: %q", v.Temperature)
	}
	if v.Fan != "N/A" || v.Power != "N/A" {
		t.Errorf("absent fan/power: fan=%q power=%q", v.Fan, v.Power)
	}
	// A missing tick must send all three values and a neutral temperature tone,
	// so a browser clears readings that were present on the preceding tick.
	payload, err := json.Marshal(formatSensors(identityTranslator, "", sensors.Facts{}))
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]string
	if err := json.Unmarshal(payload, &fields); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"temperature", "fan", "power"} {
		if fields[key] != "N/A" {
			t.Errorf("absent %s omitted or not N/A: %s", key, payload)
		}
	}
	if fields["tempLevel"] != "neutral" {
		t.Errorf("absent temperature must clear its status: %s", payload)
	}
}
