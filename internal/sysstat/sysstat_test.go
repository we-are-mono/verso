// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package sysstat

import (
	"errors"
	"testing"
)

// statSampler returns a Sampler fed from canned /proc/stat contents, consumed
// one per sample, with the cold-start beat stubbed out.
func statSampler(lines ...string) *Sampler {
	i := 0
	return &Sampler{
		readStat: func() ([]byte, error) {
			if i >= len(lines) {
				return nil, errors.New("out of samples")
			}
			line := lines[i]
			i++
			return []byte(line + "\ncpu0 0 0 0 0 0 0 0 0\n"), nil
		},
		wait: func() {},
	}
}

// TestCPUPercentColdStart: with no previous sample, the first call takes two a
// beat apart and reports the share between them. Busy excludes idle (field 4)
// and iowait (field 5): 200→300 busy of 1000→1200 total is 50%.
func TestCPUPercentColdStart(t *testing.T) {
	s := statSampler(
		"cpu 100 0 100 700 100 0 0 0",
		"cpu 150 0 150 800 100 0 0 0",
	)
	pct, err := s.CPUPercent()
	if err != nil {
		t.Fatalf("CPUPercent: %v", err)
	}
	if pct != 50 {
		t.Fatalf("CPUPercent = %d, want 50", pct)
	}
}

// TestCPUPercentSpansCalls: a warm call takes one sample and spans the gap
// since the previous call — 100% busy over the second interval.
func TestCPUPercentSpansCalls(t *testing.T) {
	s := statSampler(
		"cpu 100 0 0 900 0 0 0 0",
		"cpu 100 0 0 900 0 0 0 0",
		"cpu 300 0 0 900 0 0 0 0",
	)
	if _, err := s.CPUPercent(); err != nil {
		t.Fatalf("cold call: %v", err)
	}
	pct, err := s.CPUPercent()
	if err != nil {
		t.Fatalf("warm call: %v", err)
	}
	if pct != 100 {
		t.Fatalf("CPUPercent = %d, want 100", pct)
	}
}

// TestCPUPercentIdleBox: no counter movement between samples reads as 0, not a
// division by zero.
func TestCPUPercentIdleBox(t *testing.T) {
	s := statSampler(
		"cpu 100 0 0 900 0 0 0 0",
		"cpu 100 0 0 900 0 0 0 0",
	)
	pct, err := s.CPUPercent()
	if err != nil {
		t.Fatalf("CPUPercent: %v", err)
	}
	if pct != 0 {
		t.Fatalf("CPUPercent = %d, want 0", pct)
	}
}

// TestCPUPercentBadStat: a malformed /proc/stat is an error, not a zero
// dressed as truth.
func TestCPUPercentBadStat(t *testing.T) {
	s := statSampler("intr 12 34")
	if _, err := s.CPUPercent(); err == nil {
		t.Fatal("CPUPercent on a malformed line: want error")
	}
}

// TestRootPrefersOverlay: the OpenWrt writable upper wins when statfs answers
// for it; a box without one falls back to /.
func TestRootPrefersOverlay(t *testing.T) {
	s := &Sampler{statfs: func(path string) (Storage, error) {
		if path == "/overlay" {
			return Storage{Used: 10, Free: 90}, nil
		}
		return Storage{Used: 1, Free: 1}, nil
	}}
	st, err := s.Root()
	if err != nil {
		t.Fatalf("Root: %v", err)
	}
	if st.Used != 10 || st.Free != 90 {
		t.Fatalf("Root = %+v, want the /overlay reading", st)
	}
}

func TestRootFallsBackToSlash(t *testing.T) {
	s := &Sampler{statfs: func(path string) (Storage, error) {
		if path == "/overlay" {
			return Storage{}, errors.New("no such mount")
		}
		return Storage{Used: 5, Free: 15}, nil
	}}
	st, err := s.Root()
	if err != nil {
		t.Fatalf("Root: %v", err)
	}
	if st.Used != 5 || st.Free != 15 {
		t.Fatalf("Root = %+v, want the / reading", st)
	}
}
