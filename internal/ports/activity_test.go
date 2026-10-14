// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package ports

import "testing"

// TestActivityLevels: a port's traffic LED shows one of five levels from the
// packets it moves each second, both directions together: dark at none, a
// blip below 10, a steady blink below 1,000, a fast flicker below 100,000 and
// a near-solid flicker from there.
func TestActivityLevels(t *testing.T) {
	for _, tc := range []struct {
		pps  uint64
		want int
	}{
		{0, 0}, {1, 1}, {9, 1}, {10, 2}, {999, 2}, {1000, 3}, {99999, 3}, {100000, 4}, {5000000, 4},
	} {
		if got := Activity(tc.pps); got != tc.want {
			t.Errorf("Activity(%d) = %d, want %d", tc.pps, got, tc.want)
		}
	}
}

// TestPhaseIsAStableEighth: busy ports must not blink in step, so each port
// starts its blink a fraction of a period late. The fraction comes from its
// name, so a port keeps it across reloads, and it is one of eight steps.
func TestPhaseIsAStableEighth(t *testing.T) {
	seen := map[float64]bool{}
	for _, name := range []string{"eth0", "eth1", "eth2", "eth3", "eth4", "lan1", "lan2", "lan3", "lan4", "wan"} {
		phase := Phase(name)
		if phase != Phase(name) {
			t.Errorf("Phase(%q) changed between calls", name)
		}
		if phase < 0 || phase >= 1 || phase*8 != float64(int(phase*8)) {
			t.Errorf("Phase(%q) = %v, want one of 0, 1/8 … 7/8", name, phase)
		}
		seen[phase] = true
	}
	if len(seen) < 3 {
		t.Errorf("ten ports share only %d phases, want them spread", len(seen))
	}
}

// TestSpeedLabel: a port's negotiated speed in the units its label carries;
// a port without link shows a dash.
func TestSpeedLabel(t *testing.T) {
	for mbps, want := range map[int]string{
		10: "10M", 100: "100M", 1000: "1G", 2500: "2.5G", 5000: "5G", 10000: "10G", 25000: "25G", -1: "—", 0: "—",
	} {
		if got := SpeedLabel(mbps); got != want {
			t.Errorf("SpeedLabel(%d) = %q, want %q", mbps, got, want)
		}
	}
}
