// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

// Package usage is per-device usage's arithmetic (ADR-018): live rates from the
// helper's running byte totals, folded from addresses into devices, and the
// calendar periods summed from nlbwmon's days. It reads nothing itself; the
// shell hands it what the helper answered.
package usage

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	// minGap is the shortest interval read twice: inside it the last answer
	// stands, so every open tab shares one read of the helper.
	minGap = 800 * time.Millisecond
	// maxAge is the oldest previous reading a rate is taken against. Past it
	// the rate would be an average over the gap, not what is happening now.
	maxAge = 10 * time.Second
	// window is how far back closed days are kept: this calendar month and the
	// one before it, whole.
	window = 62
	// busyBPS is the rate under which a device reads as idle: background
	// chatter, not use anyone would look for.
	busyBPS = 100_000
)

// Counter is one address's running bytes as the helper keeps them.
type Counter struct{ Sent, Received uint64 }

// Rate is bits per second: down is what the device received, up what it sent.
type Rate struct{ Down, Up float64 }

// Totals is a period's bytes down and up.
type Totals struct{ Down, Up uint64 }

// Day is one day's totals by device MAC.
type Day map[string]Totals

// Read is one reading of the helper's totals for the listed addresses, made
// under the asking operator's session.
type Read func(ctx context.Context, addrs []string) (map[string]Counter, error)

// Sampler turns the helper's growing totals into rates. One serves every page
// and stream, so the helper is read at most once per gap however many look;
// within the gap the last answer is shared with whoever holds the page.
type Sampler struct {
	Now func() time.Time

	mu     sync.Mutex
	prev   map[string]Counter
	prevAt time.Time
	rates  map[string]Rate
}

// Rates answers each listed address's rate, and false while there is no
// reading recent enough to take one against.
func (s *Sampler) Rates(ctx context.Context, addrs []string, read Read) (map[string]Rate, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.Now()
	if !s.prevAt.IsZero() && now.Sub(s.prevAt) < minGap {
		return s.rates, s.rates != nil
	}
	reading, err := read(ctx, addrs)
	if err != nil {
		return nil, false
	}
	var rates map[string]Rate
	if elapsed := now.Sub(s.prevAt).Seconds(); !s.prevAt.IsZero() && now.Sub(s.prevAt) <= maxAge && elapsed > 0 {
		rates = make(map[string]Rate, len(reading))
		for addr, c := range reading {
			p, ok := s.prev[addr]
			if !ok || c.Received < p.Received || c.Sent < p.Sent {
				continue
			}
			rates[addr] = Rate{
				Down: float64(c.Received-p.Received) * 8 / elapsed,
				Up:   float64(c.Sent-p.Sent) * 8 / elapsed,
			}
		}
	}
	s.prev, s.prevAt, s.rates = reading, now, rates
	return rates, rates != nil
}

// Devices sums each owner's addresses: a device's rate is every address it
// holds. Owners are MACs, as the roster keys devices.
func Devices(rates map[string]Rate, owners map[string]string) map[string]Rate {
	out := map[string]Rate{}
	for addr, r := range rates {
		mac, ok := owners[addr]
		if !ok {
			continue
		}
		sum := out[mac]
		sum.Down += r.Down
		sum.Up += r.Up
		out[mac] = sum
	}
	return out
}

// History is nlbwmon's days as the shell holds them. Days before today never
// change, so each is kept once read; today is replaced every read.
type History struct {
	mu    sync.Mutex
	today string
	days  map[string]Day
}

// Absorb takes a read: today's date as nlbwmon dates it, and the days read.
// Days outside the window go.
func (h *History) Absorb(today string, days map[string]Day) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.days == nil {
		h.days = map[string]Day{}
	}
	// The day that was today was last read live, before its final minutes;
	// once it closes, its file is the truth, so it goes until read again.
	if _, read := days[h.today]; h.today != today && !read {
		delete(h.days, h.today)
	}
	h.today = today
	for date, day := range days {
		h.days[date] = day
	}
	for date := range h.days {
		if age(today, date) >= window || age(today, date) < 0 {
			delete(h.days, date)
		}
	}
}

// Known is the closed days held, newest first: what a read need not ask for.
func (h *History) Known() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	var out []string
	for date := range h.days {
		if date != h.today {
			out = append(out, date)
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(out)))
	return out
}

// Periods is what the Devices roster and the device panel say: each device's
// today, last seven days, calendar month so far and the month before it.
type Periods struct {
	Known                              bool
	Today, Last7, ThisMonth, LastMonth map[string]Totals
}

// Periods sums the held days into their periods.
func (h *History) Periods() Periods {
	h.mu.Lock()
	defer h.mu.Unlock()
	p := Periods{Known: h.today != "", Today: map[string]Totals{}, Last7: map[string]Totals{}, ThisMonth: map[string]Totals{}, LastMonth: map[string]Totals{}}
	if !p.Known {
		return p
	}
	month := h.today[:7]
	last := lastMonth(h.today)
	for date, day := range h.days {
		a := age(h.today, date)
		for mac, t := range day {
			if date == h.today {
				add(p.Today, mac, t)
			}
			if a < 7 {
				add(p.Last7, mac, t)
			}
			if strings.HasPrefix(date, month) {
				add(p.ThisMonth, mac, t)
			}
			if strings.HasPrefix(date, last) {
				add(p.LastMonth, mac, t)
			}
		}
	}
	return p
}

func add(into map[string]Totals, mac string, t Totals) {
	sum := into[mac]
	sum.Down += t.Down
	sum.Up += t.Up
	into[mac] = sum
}

// age is how many days date lies before today, by the calendar.
func age(today, date string) int {
	a, errA := time.Parse(time.DateOnly, today)
	b, errB := time.Parse(time.DateOnly, date)
	if errA != nil || errB != nil {
		return math.MaxInt32
	}
	return int(a.Sub(b).Hours() / 24)
}

// lastMonth is the calendar month before today's, as "YYYY-MM".
func lastMonth(today string) string {
	t, err := time.Parse(time.DateOnly, today)
	if err != nil {
		return ""
	}
	first := time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
	return first.AddDate(0, -1, 0).Format("2006-01")
}

// Share is part's whole percentage of whole, at most 100; anything above none
// shows at least one, so a trickle is a sliver rather than nothing.
func Share(part, whole float64) int {
	if whole <= 0 || part <= 0 {
		return 0
	}
	pct := int(math.Round(part / whole * 100))
	return max(1, min(100, pct))
}

// Busy is whether a device moves enough to read as in use.
func Busy(r Rate) bool { return r.Down+r.Up >= busyBPS }

// Mbits is a rate in megabits per second at one decimal, the ".0" dropped.
func Mbits(bps float64) string {
	return strings.TrimSuffix(fmt.Sprintf("%.1f", bps/1_000_000), ".0")
}

// Bytes is a period's total at its scale, in the binary units the rest of the
// shell uses: whole MiB under a GiB, a decimal under ten GiB, whole GiB under a
// TiB. Nothing is the empty string; anything is at least 1 MiB.
func Bytes(b uint64) string {
	const mib, gib, tib = 1 << 20, 1 << 30, 1 << 40
	trim := func(v float64) string { return strings.TrimSuffix(fmt.Sprintf("%.1f", v), ".0") }
	switch {
	case b == 0:
		return ""
	case b < gib:
		return fmt.Sprintf("%d MiB", max(1, int(math.Ceil(float64(b)/mib))))
	case b < 10*gib:
		return trim(float64(b)/gib) + " GiB"
	case b < tib:
		return fmt.Sprintf("%d GiB", int(math.Round(float64(b)/gib)))
	default:
		return trim(float64(b)/tib) + " TiB"
	}
}
