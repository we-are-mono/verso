// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"os"
	"strings"
	"sync"
	"time"

	"github.com/we-are-mono/verso/internal/sensors"
	"github.com/we-are-mono/verso/internal/widget"
	"github.com/we-are-mono/verso/profiles"
)

// sensorFreshness is how long one read of the board's sensors serves every
// page and stream: most of them are i2c transactions behind a mux.
const sensorFreshness = 5 * time.Second

// boardProfile is the profile of the board the router runs on, read once:
// board_name does not change while it runs. nil where no profile matches.
func boardProfile() *sensors.Profile {
	name, _ := os.ReadFile("/tmp/sysinfo/board_name")
	p, _ := sensors.LoadProfile(profiles.FS, strings.TrimSpace(string(name)))
	return p
}

// listsSensors is whether a profile gives the Overview a Hardware card.
func listsSensors(p *sensors.Profile) bool {
	return p != nil && len(p.Thermal)+len(p.Power)+len(p.Fans) > 0
}

// sensorSampler reads the board's sensors at most once per sensorFreshness,
// however many pages and streams ask; a caller that finds the read stale
// while another refreshes it waits for that one read.
type sensorSampler struct {
	read func() sensors.Inventory
	now  func() time.Time

	mu  sync.Mutex
	at  time.Time
	inv sensors.Inventory
}

func newSensorSampler(p *sensors.Profile) *sensorSampler {
	return &sensorSampler{read: func() sensors.Inventory { return sensors.ResolveAll(p, &sensors.Reader{}) }, now: time.Now}
}

// Read is the latest inventory, read anew once it is sensorFreshness old.
func (s *sensorSampler) Read() sensors.Inventory {
	s.mu.Lock()
	defer s.mu.Unlock()
	if now := s.now(); s.at.IsZero() || now.Sub(s.at) >= sensorFreshness {
		s.inv, s.at = s.read(), now
	}
	return s.inv
}

// hardwareView is the Hardware card's readings: the CPU, the main rail's
// draw with its voltage and current, and the main fan, written as the
// Hardware page writes them; nil when none answered.
func hardwareView(inv sensors.Inventory) *widget.OverviewSensors {
	v := &widget.OverviewSensors{}
	if t := inv.CPUTemp(); t != nil {
		v.CPU = &widget.OverviewSensor{Key: "hw-cpu", Name: t.Name, Value: "—"}
		if !t.Fault {
			v.CPU.Value = celsiusWhole(t.MilliC) + " °C"
		}
		if tone := tempVariant(t.Level); tone == "warning" || tone == "danger" {
			v.CPU.Tone = tone
		}
	}
	if p := inv.MainPower(); p != nil {
		value := powerText(p.MicroW)
		for _, part := range []string{voltage(*p), current(*p)} {
			if part != "" {
				value += " · " + part
			}
		}
		v.Power = &widget.OverviewSensor{Key: "hw-power", Name: p.Name, Value: value}
	}
	if f := inv.MainFan(); f != nil {
		v.Fan = &widget.OverviewSensor{Key: "hw-fan", Name: f.Name, Value: "—"}
		switch f.State {
		case "running":
			v.Fan.Value = rpmGroup(f.RPM) + " rpm"
		case "fault":
			v.Fan.Tone = "warning"
		}
	}
	if v.CPU == nil && v.Power == nil && v.Fan == nil {
		return nil
	}
	return v
}

// hardwareRows is the sensors frame: each row's value by its key.
func hardwareRows(v *widget.OverviewSensors) map[string]string {
	rows := map[string]string{}
	if v == nil {
		return rows
	}
	for _, s := range []*widget.OverviewSensor{v.CPU, v.Power, v.Fan} {
		if s != nil {
			rows[s.Key] = s.Value
		}
	}
	return rows
}
