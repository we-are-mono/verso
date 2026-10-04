// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"fmt"

	"github.com/we-are-mono/verso/internal/sensors"
	"github.com/we-are-mono/verso/internal/widget"
	"github.com/we-are-mono/verso/profiles"
)

// sensorView is the hardware-sensor snapshot the overview shows and the stream
// pushes: the formatted temperature/fan/power and the temperature's status
// colour. Missing readings are explicit N/A values so a live update clears a
// stale reading. Model is a page-load fact and never rides the stream.
type sensorView struct {
	Model       string `json:"-"`
	Temperature string `json:"temperature"`
	TempLevel   string `json:"tempLevel"`
	Fan         string `json:"fan"`
	Power       string `json:"power"`
}

// resolveSensors reads local sysfs through the board's profile — the same
// inventory the Hardware page reads, so the two pages pick and grade the same
// sensors. Reads the local kernel, so it needs no sid — the same call serves the
// page render and each stream tick.
func resolveSensors(tr func(string) string, boardName string) sensorView {
	profile, _ := sensors.LoadProfile(profiles.FS, boardName)
	model := ""
	if profile != nil {
		model = profile.Name
	}
	return formatSensors(tr, model, sensors.ResolveAll(profile, &sensors.Reader{}))
}

// tempWords names a temperature's grade for the overview, in the Hardware page's
// levels; a reading with no limits of its own carries no grade.
var tempWords = map[string]string{"nominal": "Normal", "warn": "Warm", "critical": "Critical"}

// formatSensors renders the inventory's headline readings into display strings —
// °C rounded from milli, watts from micro, RPM as-is — using N/A when a reading
// is absent. The temperature's grade is translated before composing its value so
// the initial page and the live stream share the same wording.
func formatSensors(tr func(string) string, model string, inv sensors.Inventory) sensorView {
	v := sensorView{Model: model, Temperature: "N/A", TempLevel: "neutral", Fan: "N/A", Power: "N/A"}
	if t := inv.CPUTemp(); t != nil {
		v.Temperature = fmt.Sprintf("%d °C", (t.MilliC+500)/1000)
		if word, graded := tempWords[t.Level]; graded {
			v.Temperature += " · " + tr(word)
		}
		v.TempLevel = tempVariant(t.Level)
	}
	if f := inv.MainFan(); f != nil {
		v.Fan = fmt.Sprintf("%d rpm", f.RPM)
	}
	if p := inv.MainPower(); p != nil {
		v.Power = fmt.Sprintf("%.1f W", float64(p.MicroW)/1e6)
	}
	return v
}

// applySensors fills the overview's three sensor facts for the initial render.
func (s *Server) applySensors(ov *widget.Overview, boardName string, tr func(string) string) {
	v := resolveSensors(tr, boardName)
	if v.Model != "" {
		ov.Model = v.Model
	}
	ov.Temperature, ov.TempDot = v.Temperature, v.TempLevel
	ov.Fan = v.Fan
	ov.Power = v.Power
}
