// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"fmt"
	"strings"

	"github.com/we-are-mono/verso/internal/sensors"
	"github.com/we-are-mono/verso/internal/widget"
	"github.com/we-are-mono/verso/profiles"
)

// sensorView is the hardware-sensor snapshot the overview shows and the stream
// pushes: the formatted temperature/fan/power/summary and the temperature's
// status colour. An empty field means the box exposes no such reading — its row
// is hidden at page load and skipped by the live updater. Model is a page-load
// fact (the profile's board name), so it never rides the stream.
type sensorView struct {
	Model       string `json:"-"`
	Temperature string `json:"temperature,omitempty"`
	TempLevel   string `json:"tempLevel,omitempty"`
	Fan         string `json:"fan,omitempty"`
	Power       string `json:"power,omitempty"`
	Summary     string `json:"summary,omitempty"`
}

// resolveSensors reads local sysfs and resolves it through the board's profile
// (generic when unprofiled), formatting the home-dashboard sensor facts. Power is
// profile-only, so an unprofiled box leaves it blank. Reads the local kernel, so
// it needs no sid — the same call serves the page render and each stream tick.
func resolveSensors(boardName string) sensorView {
	profile, _ := sensors.LoadProfile(profiles.FS, boardName)
	f := sensors.Resolve(profile, &sensors.Reader{})
	model := ""
	if profile != nil {
		model = profile.Name
	}
	return formatSensors(model, f)
}

// formatSensors renders resolved facts into display strings — °C rounded from
// milli, watts from micro, RPM as-is — leaving a field blank when its reading is
// absent so the row hides. Pure, so the formatting contract is unit-tested.
func formatSensors(model string, f sensors.Facts) sensorView {
	v := sensorView{Model: model}
	if f.CPUTemp != nil {
		v.Temperature = fmt.Sprintf("%d °C · %s", (f.CPUTemp.MilliC+500)/1000, f.CPUTemp.Status)
		v.TempLevel = f.CPUTemp.Level
	}
	if f.Fan != nil {
		v.Fan = fmt.Sprintf("%d rpm", f.Fan.RPM)
	}
	if f.Power != nil {
		v.Power = fmt.Sprintf("%.1f W", float64(f.Power.MicroW)/1e6)
	}
	v.Summary = sensorSummary(f.PowerCount, f.ThermalCount)
	return v
}

// applySensors fills the overview's sensor facts for the initial render. A blank
// reading leaves its field empty so the row drops out (see Overview.sysRight).
func (s *Server) applySensors(ov *widget.Overview, boardName string) {
	v := resolveSensors(boardName)
	if v.Model != "" {
		ov.Model = v.Model
	}
	ov.Temperature, ov.TempDot = v.Temperature, v.TempLevel
	ov.Fan = v.Fan
	ov.Power = v.Power
	ov.SensorSummary = v.Summary
}

// sensorSummary reads like "8 power · 5 thermal", dropping a kind the box has
// none of and returning empty when it has neither.
func sensorSummary(power, thermal int) string {
	parts := make([]string, 0, 2)
	if power > 0 {
		parts = append(parts, fmt.Sprintf("%d power", power))
	}
	if thermal > 0 {
		parts = append(parts, fmt.Sprintf("%d thermal", thermal))
	}
	return strings.Join(parts, " · ")
}
