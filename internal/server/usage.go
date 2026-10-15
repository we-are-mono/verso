// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/we-are-mono/verso/internal/openwrt"
	"github.com/we-are-mono/verso/internal/units"
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
	nums    units.Numbers         // how the reader's language writes the figures
}

// The shell holds one copy of what the helper's usage reads answered, shared
// by every page (the days, the last rates and the minute of them a device's
// graph opens on), so a session sees it only while rpcd grants that session
// the read it came from (ADR-007): the days under usageDays, the rates under
// usageLive.
var (
	usageDaysGrant = aclProbe{"ubus", "verso", "usageDays"}
	usageLiveGrant = aclProbe{"ubus", "verso", "usageLive"}
)

// readUsage reads the roster's usage under the operator's session: the
// history first, since it says whether usage is on, then the live rates.
func (s *Server) readUsage(ctx context.Context, sid string, roster []widget.Device, nums units.Numbers) rosterUsage {
	g := s.grantsFor(sid)
	u := rosterUsage{on: s.readUsageHistory(ctx, g), nums: nums}
	if !u.on {
		return u
	}
	u.periods = s.usageHistory.Periods()
	u.rates = s.readUsageRates(ctx, g, roster)
	return u
}

// readUsageHistory asks nlbwmon for today and the days not yet held, and
// answers whether usage is on for the session. A read that fails on the way
// leaves the held days as they were and keeps usage on: the month then reads
// from what is known. A session refused the read, by its grants or by the
// helper, has usage off: what is held is another session's reading.
func (s *Server) readUsageHistory(ctx context.Context, g *sessionGrants) bool {
	if !g.may(ctx, usageDaysGrant) {
		return false
	}
	read, err := s.backend.UsageDays(ctx, g.sid, s.usageHistory.Known())
	if err != nil {
		log.Printf("verso: usage history unavailable: %v", err)
		if errors.Is(err, openwrt.ErrAccessDenied) {
			return false
		}
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

// readUsageRates is each device's live rate, its addresses folded into it,
// for a session granted the live read; none for any other.
func (s *Server) readUsageRates(ctx context.Context, g *sessionGrants, roster []widget.Device) map[string]usage.Rate {
	if !g.may(ctx, usageLiveGrant) {
		return nil
	}
	owners := addressOwners(roster)
	addrs := make([]string, 0, len(owners))
	for a := range owners {
		addrs = append(addrs, a)
	}
	read := func(ctx context.Context, addrs []string) (map[string]usage.Counter, error) {
		live, err := s.backend.UsageLive(ctx, g.sid, addrs)
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

// loadNames is each load as the roster names it.
var loadNames = map[usage.Load]string{usage.Light: "light", usage.Medium: "medium", usage.Heavy: "heavy"}

// deviceUsage is one device's usage as the roster says it.
func (u rosterUsage) deviceUsage(mac string) *widget.DeviceUsage {
	mac = strings.ToLower(mac)
	out := &widget.DeviceUsage{}
	if r, ok := u.rates[mac]; ok && usage.Busy(r) {
		out.Busy = true
		out.Down, out.Up = u.nums.Rate(r.Down).String(), u.nums.Rate(r.Up).String()
		out.Load = loadNames[usage.LoadOf(r)]
	}
	month := u.periods.ThisMonth[mac]
	out.Month = u.nums.Bytes(month.Down + month.Up)
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
	MAC   string `json:"mac"`
	Busy  bool   `json:"busy"`
	Down  string `json:"down"`
	Up    string `json:"up"`
	Ink   string `json:"ink"` // the Now cell's tone for the device's load, none while idle
	Month string `json:"month,omitempty"`
	// DownMbps and UpMbps are the rate as numbers, idle or not, for the
	// device's traffic graph when its panel is open.
	DownMbps float64 `json:"down_mbps"`
	UpMbps   float64 `json:"up_mbps"`
}

// handleDevicesUsage streams the roster's usage while the Devices page is
// open: each device's rate every tick, its month every usageMonthRefresh. The
// roster's addresses are read again with the month, so a device that joins is
// counted within half a minute, and the session's grants asked again with it;
// usage off for the session at one month's read stays off until the next.
func (s *Server) handleDevicesUsage(w http.ResponseWriter, r *http.Request) {
	sid := s.sessionSID(r)
	_, t := s.localize(r)
	nums := units.For(t)
	var roster []widget.Device
	var monthAt time.Time
	var g *sessionGrants
	var on bool
	s.serveSSE(w, r, func() bool {
		withMonth := time.Since(monthAt) >= usageMonthRefresh
		u := rosterUsage{nums: nums}
		if withMonth {
			g = s.grantsFor(sid)
			roster, _ = s.connectedDevices(r.Context(), sid, clientIP(r))
			on = s.readUsageHistory(r.Context(), g)
			u.periods = s.usageHistory.Periods()
			monthAt = time.Now()
		}
		u.on = on
		if !u.on {
			return writeEvent(w, "", "usage", map[string]any{"devices": []usageFrame{}}) == nil
		}
		u.rates = s.readUsageRates(r.Context(), g, roster)
		frames := make([]usageFrame, 0, len(roster))
		for _, d := range roster {
			du := u.deviceUsage(d.MAC)
			mac := strings.ToLower(d.MAC)
			r := u.rates[mac]
			f := usageFrame{MAC: mac, Busy: du.Busy, Down: du.Down, Up: du.Up, Ink: widget.LoadInk(du.Load), DownMbps: r.Down / 1_000_000, UpMbps: r.Up / 1_000_000}
			if withMonth {
				f.Month = du.Month
			}
			frames = append(frames, f)
		}
		return writeEvent(w, "", "usage", map[string]any{"devices": frames, "month": withMonth}) == nil
	})
}
