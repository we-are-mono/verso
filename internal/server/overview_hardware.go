// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"context"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/we-are-mono/verso/internal/machine"
	"github.com/we-are-mono/verso/internal/openwrt"
	"github.com/we-are-mono/verso/internal/sensors"
	"github.com/we-are-mono/verso/internal/units"
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
// however many pages and streams ask. A read takes the router a good part of
// a second, so once there is a reading no caller waits for one: a caller that
// finds it stale has it at once and starts one refresh in the background,
// which the callers after it share.
type sensorSampler struct {
	read func() sensors.Inventory
	now  func() time.Time

	mu         sync.Mutex
	at         time.Time
	inv        sensors.Inventory
	refreshing bool
}

func newSensorSampler(p *sensors.Profile) *sensorSampler {
	return &sensorSampler{read: func() sensors.Inventory { return sensors.ResolveAll(p, &sensors.Reader{}) }, now: time.Now}
}

// Read is the latest inventory. The first is waited for, there being nothing
// else to give; one sensorFreshness old, it is read anew behind its callers.
func (s *sensorSampler) Read() sensors.Inventory {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	if s.at.IsZero() {
		s.inv, s.at = s.read(), now
		return s.inv
	}
	if now.Sub(s.at) >= sensorFreshness && !s.refreshing {
		s.refreshing = true
		go s.refresh()
	}
	return s.inv
}

// refresh reads the sensors outside the lock, so every caller meanwhile has
// the last reading, and hands the fresh one over.
func (s *sensorSampler) refresh() {
	inv := s.read()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.inv, s.at, s.refreshing = inv, s.now(), false
}

// hardwareView is the Hardware card's readings: the CPU, the main rail's
// draw with its voltage and current, and the main fan, written as the
// Hardware page writes them, with what needs attention beyond them; nil when
// none answered and nothing does.
func hardwareView(inv sensors.Inventory, alerts []widget.OverviewAlert) *widget.OverviewSensors {
	v := &widget.OverviewSensors{Alerts: alerts}
	if t := inv.CPUTemp(); t != nil {
		v.CPU = &widget.OverviewSensor{Key: "hw-cpu", Name: t.Name, Value: "—"}
		if !t.Fault {
			v.CPU.Value = celsiusWhole(t.MilliC) + units.Space + "°C"
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
			v.Fan.Value = rpmGroup(f.RPM) + units.Space + "rpm"
		case "fault":
			v.Fan.Tone = "warning"
		}
	}
	if v.CPU == nil && v.Power == nil && v.Fan == nil && len(alerts) == 0 {
		return nil
	}
	return v
}

// hardwareAlerts are what needs attention in the box beyond its readings: an
// eMMC wearing out, a clock battery running down.
func hardwareAlerts(facts machine.Facts, hw openwrt.HardwareState) []widget.OverviewAlert {
	var out []widget.OverviewAlert
	if d := facts.Disk; d != nil && d.Wear != nil {
		switch d.Wear.Level() {
		case "warn":
			out = append(out, widget.OverviewAlert{Tone: "warning", Words: "Storage is wearing out"})
		case "critical":
			out = append(out, widget.OverviewAlert{Tone: "danger", Words: "Storage is worn out"})
		}
	}
	if c := hw.Clock; c != nil {
		switch c.Battery {
		case "low":
			out = append(out, widget.OverviewAlert{Tone: "warning", Words: "Clock battery is low"})
		case "empty":
			out = append(out, widget.OverviewAlert{Tone: "warning", Words: "Clock battery is empty"})
		}
	}
	return out
}

// hardwareState is what the root helper reads of the board for the session;
// a helper that cannot answer leaves the clock and the slot unsaid.
func (s *Server) hardwareState(ctx context.Context, sid string) openwrt.HardwareState {
	hw, err := s.backend.HardwareState(ctx, sid)
	if err != nil {
		log.Printf("verso: hardware: state unavailable: %v", err)
	}
	return hw
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
