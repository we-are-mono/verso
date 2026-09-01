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
func resolveSensors(tr func(string) string, boardName string) sensorView {
	profile, _ := sensors.LoadProfile(profiles.FS, boardName)
	f := sensors.Resolve(profile, &sensors.Reader{})
	model := ""
	if profile != nil {
		model = profile.Name
	}
	return formatSensors(tr, model, f)
}

// formatSensors renders resolved facts into display strings — °C rounded from
// milli, watts from micro, RPM as-is — leaving a field blank when its reading is
// absent so the row hides. Composed strings translate their format here, at the
// point of composition: the schema walk can only match whole catalog keys, and
// "8 power · 5 thermal" is not one. Pure, so the formatting contract is
// unit-tested.
func formatSensors(tr func(string) string, model string, f sensors.Facts) sensorView {
	v := sensorView{Model: model}
	if f.CPUTemp != nil {
		v.Temperature = fmt.Sprintf("%d °C · %s", (f.CPUTemp.MilliC+500)/1000, tr(f.CPUTemp.Status))
		v.TempLevel = f.CPUTemp.Level
	}
	if f.Fan != nil {
		v.Fan = fmt.Sprintf("%d rpm", f.Fan.RPM)
	}
	if f.Power != nil {
		v.Power = fmt.Sprintf("%.1f W", float64(f.Power.MicroW)/1e6)
	}
	v.Summary = sensorSummary(tr, f.PowerCount, f.ThermalCount)
	return v
}

// applySensors fills the overview's sensor facts for the initial render. A blank
// reading leaves its field empty so the row drops out (see Overview.sysRight).
func (s *Server) applySensors(ov *widget.Overview, boardName string, tr func(string) string) {
	v := resolveSensors(tr, boardName)
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
func sensorSummary(tr func(string) string, power, thermal int) string {
	parts := make([]string, 0, 2)
	if power > 0 {
		parts = append(parts, fmt.Sprintf(tr("%d power"), power))
	}
	if thermal > 0 {
		parts = append(parts, fmt.Sprintf(tr("%d thermal"), thermal))
	}
	return strings.Join(parts, " · ")
}
