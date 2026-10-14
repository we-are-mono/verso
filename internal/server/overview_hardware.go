// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"os"
	"strconv"
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

// hardwareView is the Hardware card's readings: the profile's entries in its
// order, written as the Hardware page writes them; nil when none answered.
func hardwareView(inv sensors.Inventory) *widget.OverviewSensors {
	temps, powers, fans := inv.Profiled()
	if len(temps)+len(powers)+len(fans) == 0 {
		return nil
	}
	v := &widget.OverviewSensors{}
	key := func(kind string, entry int) string { return "hw-" + kind + "-" + strconv.Itoa(entry) }
	for _, t := range temps {
		s := widget.OverviewSensor{Key: key("thermal", t.Entry), Name: t.Name, Value: "—", Tone: tempVariant(t.Level)}
		if !t.Fault {
			s.Value = celsiusWhole(t.MilliC) + " °C"
		}
		if t.CPU {
			v.CPU = s.Value
		}
		v.Thermal = append(v.Thermal, s)
	}
	for _, p := range powers {
		v.Power = append(v.Power, widget.OverviewSensor{Key: key("power", p.Entry), Name: p.Name, Value: powerText(p.MicroW)})
	}
	for _, f := range fans {
		s := widget.OverviewSensor{Key: key("fans", f.Entry), Name: f.Name, Value: "—"}
		switch f.State {
		case "running":
			s.Value = rpmGroup(f.RPM) + " rpm"
		case "fault":
			s.Tone = "warning"
		}
		v.Fans = append(v.Fans, s)
	}
	return v
}

// hardwareRows is the sensors frame: each row's value by its key.
func hardwareRows(v *widget.OverviewSensors) map[string]string {
	rows := map[string]string{}
	if v == nil {
		return rows
	}
	for _, list := range [][]widget.OverviewSensor{v.Thermal, v.Power, v.Fans} {
		for _, s := range list {
			rows[s.Key] = s.Value
		}
	}
	return rows
}
