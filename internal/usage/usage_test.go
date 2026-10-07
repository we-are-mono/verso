// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package usage

import (
	"context"
	"testing"
	"time"
)

// TestSamplerTurnsTwoReadingsIntoRates: the helper's totals only grow, so a
// rate is two readings apart; one reading alone is no rate, a reading asked for
// again within the gap is the last answer (many tabs, one read), and a previous
// reading gone stale starts over rather than averaging an hour.
func TestSamplerTurnsTwoReadingsIntoRates(t *testing.T) {
	now := time.Unix(1000, 0)
	var calls int
	totals := map[string]Counter{"192.168.77.20": {Sent: 1000, Received: 10000}}
	read := func(_ context.Context, addrs []string) (map[string]Counter, error) {
		calls++
		out := map[string]Counter{}
		for _, a := range addrs {
			out[a] = totals[a]
		}
		return out, nil
	}
	s := &Sampler{Now: func() time.Time { return now }}
	addrs := []string{"192.168.77.20"}
	if _, ok := s.Rates(context.Background(), addrs, read); ok {
		t.Fatal("one reading is not a rate")
	}

	now = now.Add(2 * time.Second)
	totals["192.168.77.20"] = Counter{Sent: 1000 + 250_000, Received: 10000 + 2_500_000}
	rates, ok := s.Rates(context.Background(), addrs, read)
	got := rates["192.168.77.20"]
	if !ok || got.Down != 10_000_000 || got.Up != 1_000_000 {
		t.Fatalf("rates = %+v ok=%v, want 10 Mbit/s down and 1 up", got, ok)
	}

	now = now.Add(200 * time.Millisecond)
	if again, ok := s.Rates(context.Background(), addrs, read); !ok || again["192.168.77.20"] != got || calls != 2 {
		t.Fatalf("within the gap: rates=%+v ok=%v calls=%d, want the last answer and no read", again, ok, calls)
	}

	now = now.Add(time.Minute)
	if _, ok := s.Rates(context.Background(), addrs, read); ok {
		t.Error("a reading a minute old is no basis for a rate now")
	}
}

// TestDevicesFoldAddressesByOwner: a device is every address it holds, so its
// rate is theirs summed; an address nobody owns is not a device.
func TestDevicesFoldAddressesByOwner(t *testing.T) {
	rates := map[string]Rate{
		"192.168.77.20":  {Down: 8, Up: 1},
		"fd42:7ea:1::20": {Down: 2, Up: 1},
		"8.8.8.8":        {Down: 99, Up: 99},
	}
	owners := map[string]string{"192.168.77.20": "02:aa", "fd42:7ea:1::20": "02:aa"}
	got := Devices(rates, owners)
	if len(got) != 1 || got["02:aa"] != (Rate{Down: 10, Up: 2}) {
		t.Fatalf("devices = %+v", got)
	}
}

// TestHistoryKeepsClosedDaysAndSumsCalendarPeriods: days before today never
// change, so they are kept once read and listed as known; today is replaced on
// every read. The periods are today, the last seven days with today, the
// calendar month so far and the one before it.
func TestHistoryKeepsClosedDaysAndSumsCalendarPeriods(t *testing.T) {
	var h History
	h.Absorb("2026-10-07", map[string]Day{
		"2026-10-07": {"02:aa": {Down: 100, Up: 10}},
		"2026-10-01": {"02:aa": {Down: 1000, Up: 100}},
		"2026-09-30": {"02:aa": {Down: 5, Up: 1}},
		"2026-09-02": {"02:aa": {Down: 7, Up: 1}, "02:bb": {Down: 3}},
		"2026-07-01": {"02:aa": {Down: 1 << 40}},
	})
	if known := h.Known(); len(known) != 3 || known[0] != "2026-10-01" || known[2] != "2026-09-02" {
		t.Fatalf("known = %v, want the closed days in the window, newest first, today excluded", known)
	}
	h.Absorb("2026-10-07", map[string]Day{"2026-10-07": {"02:aa": {Down: 300, Up: 30}}})

	p := h.Periods()
	if p.Today["02:aa"] != (Totals{Down: 300, Up: 30}) {
		t.Errorf("today = %+v, want the newest reading of today", p.Today["02:aa"])
	}
	if p.Last7["02:aa"] != (Totals{Down: 1300, Up: 130}) {
		t.Errorf("last 7 days = %+v", p.Last7["02:aa"])
	}
	if p.ThisMonth["02:aa"] != (Totals{Down: 1300, Up: 130}) {
		t.Errorf("this month = %+v", p.ThisMonth["02:aa"])
	}
	if p.LastMonth["02:aa"] != (Totals{Down: 12, Up: 2}) || p.LastMonth["02:bb"] != (Totals{Down: 3}) {
		t.Errorf("last month = %+v", p.LastMonth)
	}
	if !p.Known {
		t.Error("a history that has read today is known")
	}

	// Past midnight, the day that was today was last read live, before its
	// final minutes: it is not known, so the next read takes its file.
	h.Absorb("2026-10-08", map[string]Day{"2026-10-08": {"02:aa": {Down: 1}}})
	for _, d := range h.Known() {
		if d == "2026-10-07" {
			t.Error("the day that just closed must be read again from its file")
		}
	}
	if new(History).Periods().Known {
		t.Error("a history that never read is not known")
	}
}

func TestShareIsAWholePercentOfTheWhole(t *testing.T) {
	for _, tc := range []struct {
		part, whole float64
		want        int
	}{{38, 50, 76}, {0, 50, 0}, {5, 0, 0}, {80, 50, 100}, {0.2, 1000, 1}} {
		if got := Share(tc.part, tc.whole); got != tc.want {
			t.Errorf("Share(%v, %v) = %d, want %d", tc.part, tc.whole, got, tc.want)
		}
	}
}

func TestFiguresReadAtTheirScale(t *testing.T) {
	for bps, want := range map[float64]string{38_240_000: "38.2", 6_000_000: "6", 120_000: "0.1", 980_000_000: "980"} {
		if got := Mbits(bps); got != want {
			t.Errorf("Mbits(%v) = %q, want %q", bps, got, want)
		}
	}
	for b, want := range map[uint64]string{0: "", 512: "1 MiB", 300 << 20: "300 MiB", 3 << 29: "1.5 GiB", 212 << 30: "212 GiB", 3 << 39: "1.5 TiB"} {
		if got := Bytes(b); got != want {
			t.Errorf("Bytes(%d) = %q, want %q", b, got, want)
		}
	}
	if !Busy(Rate{Down: 100_000}) || Busy(Rate{Down: 40_000, Up: 40_000}) {
		t.Error("busy is a tenth of a megabit in either direction, together")
	}
}
