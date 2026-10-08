// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"context"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/we-are-mono/verso/internal/usage"
	"github.com/we-are-mono/verso/internal/widget"
)

// Per-device usage (ADR-018): what each device on the roster moves now and has
// moved this calendar month. The helper reads conntrack and nlbwmon; the usage
// package does the arithmetic; this file reads it for a roster and says it.

// usageMonthRefresh is how often the live stream re-reads the month. nlbwmon
// refreshes its counters every 30 seconds, so reading it faster finds nothing.
const usageMonthRefresh = 30 * time.Second

// rosterUsage is one read of usage for a roster.
type rosterUsage struct {
	on      bool                  // nlbwmon is enabled, so usage is drawn at all
	periods usage.Periods         // the calendar periods by MAC
	rates   map[string]usage.Rate // live rates by MAC; nil while there is no rate yet
	wan     usage.Rate            // the uplink's rate now, the meters' whole
}

// readUsage reads the roster's usage under the operator's session: the
// history first, since it says whether usage is on, then the live rates.
func (s *Server) readUsage(ctx context.Context, sid string, roster []widget.Device) rosterUsage {
	u := rosterUsage{on: s.readUsageHistory(ctx, sid)}
	if !u.on {
		return u
	}
	u.periods = s.usageHistory.Periods()
	u.rates = s.readUsageRates(ctx, sid, roster)
	u.wan = s.wanRate(ctx, sid)
	return u
}

// readUsageHistory asks nlbwmon for today and the days not yet held, and
// answers whether usage is on. A failed read leaves the held days as they were
// and keeps usage on: the month then reads from what is known.
func (s *Server) readUsageHistory(ctx context.Context, sid string) bool {
	read, err := s.backend.UsageDays(ctx, sid, s.usageHistory.Known())
	if err != nil {
		log.Printf("verso: usage history unavailable: %v", err)
		return s.usageHistory.Periods().Known
	}
	if !read.Enabled {
		return false
	}
	// A read that cannot say what today is has read nothing; the days held
	// stand rather than going blank.
	if read.Today == "" {
		log.Printf("verso: usage history: the helper named no today")
		return s.usageHistory.Periods().Known
	}
	days := make(map[string]usage.Day, len(read.Days))
	for date, macs := range read.Days {
		day := make(usage.Day, len(macs))
		for mac, c := range macs {
			day[strings.ToLower(mac)] = usage.Totals{Down: c.Received, Up: c.Sent}
		}
		days[date] = day
	}
	s.usageHistory.Absorb(read.Today, days)
	return true
}

// readUsageRates is each device's live rate, its addresses folded into it.
func (s *Server) readUsageRates(ctx context.Context, sid string, roster []widget.Device) map[string]usage.Rate {
	owners := addressOwners(roster)
	addrs := make([]string, 0, len(owners))
	for a := range owners {
		addrs = append(addrs, a)
	}
	read := func(ctx context.Context, addrs []string) (map[string]usage.Counter, error) {
		live, err := s.backend.UsageLive(ctx, sid, addrs)
		if err != nil {
			return nil, err
		}
		out := make(map[string]usage.Counter, len(live.Addresses))
		for a, c := range live.Addresses {
			out[a] = usage.Counter{Sent: c.Sent, Received: c.Received}
		}
		return out, nil
	}
	rates, ok := s.usageSampler.Rates(ctx, addrs, read)
	if !ok {
		return nil
	}
	return usage.Devices(rates, owners)
}

// addressOwners maps every address the roster knows to its device's MAC.
func addressOwners(roster []widget.Device) map[string]string {
	owners := map[string]string{}
	for _, d := range roster {
		mac := strings.ToLower(d.MAC)
		for _, a := range d.Addresses {
			owners[a.Addr] = mac
		}
		for _, a := range []string{d.V4, d.V6} {
			if a != "" {
				owners[a] = mac
			}
		}
	}
	return owners
}

// wanRate is the uplink's rate now, from the telemetry sampler's newest
// point; zero when there is no uplink to sample, which leaves meters empty.
func (s *Server) wanRate(ctx context.Context, sid string) usage.Rate {
	ws, err := s.backend.WANStatus(ctx, sid)
	if err != nil {
		return usage.Rate{}
	}
	primary, found := ws.Primary()
	if !found || primary.Device == "" {
		return usage.Rate{}
	}
	snapshot, ok := s.telemetrySnapshot(ctx)
	if !ok {
		return usage.Rate{}
	}
	device, found := snapshot.Interface(primary.Device)
	if !found || len(device.History) == 0 {
		return usage.Rate{}
	}
	p := device.History[len(device.History)-1]
	return usage.Rate{Down: float64(p.RxBPS), Up: float64(p.TxBPS)}
}

// deviceUsage is one device's usage as the roster says it.
func (u rosterUsage) deviceUsage(mac string) *widget.DeviceUsage {
	mac = strings.ToLower(mac)
	out := &widget.DeviceUsage{}
	if r, ok := u.rates[mac]; ok && usage.Busy(r) {
		out.Busy = true
		out.Down, out.Up = usage.Mbits(r.Down), usage.Mbits(r.Up)
		out.DownFill, out.UpFill = usage.Share(r.Down, u.wan.Down), usage.Share(r.Up, u.wan.Up)
	}
	month := u.periods.ThisMonth[mac]
	out.Month = usage.Bytes(month.Down + month.Up)
	return out
}

// applyUsage gives every device on the roster its usage.
func (u rosterUsage) apply(roster []widget.Device) {
	for i := range roster {
		roster[i].Usage = u.deviceUsage(roster[i].MAC)
	}
}

// usageFrame is one live frame for the roster: each device's Now, and its
// month when the frame carries the month too.
type usageFrame struct {
	MAC      string `json:"mac"`
	Busy     bool   `json:"busy"`
	Down     string `json:"down"`
	Up       string `json:"up"`
	DownFill int    `json:"down_fill"`
	UpFill   int    `json:"up_fill"`
	Month    string `json:"month,omitempty"`
	// DownMbps and UpMbps are the rate as numbers, idle or not, for the
	// device's traffic graph when its panel is open.
	DownMbps float64 `json:"down_mbps"`
	UpMbps   float64 `json:"up_mbps"`
}

// handleDevicesUsage streams the roster's usage while the Devices page is
// open: each device's rate every tick, its month every usageMonthRefresh. The
// roster's addresses are read again with the month, so a device that joins is
// counted within half a minute.
func (s *Server) handleDevicesUsage(w http.ResponseWriter, r *http.Request) {
	sid := s.sessionSID(r)
	var roster []widget.Device
	var monthAt time.Time
	s.serveSSE(w, r, func() bool {
		withMonth := time.Since(monthAt) >= usageMonthRefresh
		u := rosterUsage{on: true}
		if withMonth {
			roster = s.connectedDevices(r.Context(), sid, clientIP(r))
			u.on = s.readUsageHistory(r.Context(), sid)
			u.periods = s.usageHistory.Periods()
			monthAt = time.Now()
		}
		if !u.on {
			return writeEvent(w, "", "usage", map[string]any{"devices": []usageFrame{}}) == nil
		}
		u.rates = s.readUsageRates(r.Context(), sid, roster)
		u.wan = s.wanRate(r.Context(), sid)
		frames := make([]usageFrame, 0, len(roster))
		for _, d := range roster {
			du := u.deviceUsage(d.MAC)
			mac := strings.ToLower(d.MAC)
			r := u.rates[mac]
			f := usageFrame{MAC: mac, Busy: du.Busy, Down: du.Down, Up: du.Up, DownFill: du.DownFill, UpFill: du.UpFill, DownMbps: r.Down / 1_000_000, UpMbps: r.Up / 1_000_000}
			if withMonth {
				f.Month = du.Month
			}
			frames = append(frames, f)
		}
		return writeEvent(w, "", "usage", map[string]any{"devices": frames, "month": withMonth}) == nil
	})
}
