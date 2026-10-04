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
	v := formatSensors(identityTranslator, "Mono Gateway Development Kit", sensors.Inventory{
		Temps:  []sensors.TempReading{{MilliC: 75750, Warn: 75000, Crit: 95000, Level: "warn", CPU: true}},
		Fans:   []sensors.FanReading{{RPM: 3630, State: "running", Main: true}},
		Powers: []sensors.PowerReading{{MicroW: 12400000, Main: true}},
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

// The overview grades the CPU temperature exactly as the Hardware page does:
// against the sensor's own limits, and not at all when it has none — never
// against a guessed threshold the Hardware page would not show.
func TestFormatSensorsGradesLikeTheHardwarePage(t *testing.T) {
	for _, tc := range []struct {
		name  string
		temp  sensors.TempReading
		value string
		tone  string
	}{
		{"no limits", sensors.TempReading{MilliC: 80000, CPU: true}, "80 °C", "neutral"},
		{"under its warn trip", sensors.TempReading{MilliC: 80000, Warn: 85000, Crit: 95000, Level: "nominal", CPU: true}, "80 °C · Normal", "success"},
		{"past its warn trip", sensors.TempReading{MilliC: 88000, Warn: 85000, Crit: 95000, Level: "warn", CPU: true}, "88 °C · Warm", "warning"},
		{"past critical", sensors.TempReading{MilliC: 96000, Warn: 85000, Crit: 95000, Level: "critical", CPU: true}, "96 °C · Critical", "danger"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v := formatSensors(identityTranslator, "", sensors.Inventory{Temps: []sensors.TempReading{tc.temp}})
			if v.Temperature != tc.value || v.TempLevel != tc.tone {
				t.Errorf("got %q / %q, want %q / %q", v.Temperature, v.TempLevel, tc.value, tc.tone)
			}
			if hw := tempVariant(tc.temp.Level); hw != tc.tone {
				t.Errorf("the Hardware page tones it %q, the overview %q", hw, tc.tone)
			}
		})
	}
}

func TestFormatSensorsAbsent(t *testing.T) {
	// An unprofiled PC keeps its system temperature and shows N/A for the rest.
	v := formatSensors(identityTranslator, "", sensors.Inventory{
		Temps: []sensors.TempReading{{MilliC: 45000, Warn: 80000, Level: "nominal", CPU: true}},
	})
	if v.Temperature != "45 °C · Normal" {
		t.Errorf("temperature: %q", v.Temperature)
	}
	if v.Fan != "N/A" || v.Power != "N/A" {
		t.Errorf("absent fan/power: fan=%q power=%q", v.Fan, v.Power)
	}
	// A missing tick must send all three values and a neutral temperature tone,
	// so a browser clears readings that were present on the preceding tick.
	payload, err := json.Marshal(formatSensors(identityTranslator, "", sensors.Inventory{}))
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
		t.Errorf("absent temperature tone: %s", payload)
	}
}
