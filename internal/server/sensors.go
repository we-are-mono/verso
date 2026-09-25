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

// resolveSensors reads local sysfs and resolves it through the board's profile
// (generic temperature when unprofiled), formatting the home-dashboard sensor
// facts. Fan and power are profile-only. Reads the local kernel, so
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
// milli, watts from micro, RPM as-is — using N/A when a reading is absent.
// The temperature's status is translated before composing its value so the
// initial page and the live stream share the same wording.
func formatSensors(tr func(string) string, model string, f sensors.Facts) sensorView {
	v := sensorView{Model: model, Temperature: "N/A", TempLevel: "neutral", Fan: "N/A", Power: "N/A"}
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
