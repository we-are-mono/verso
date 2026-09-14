// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

// Package sysstat reads local machine health — CPU busy share and filesystem
// fullness — straight from the kernel. Neither fact exists on ubus, and Verso
// runs on the box it reports on, so /proc and statfs are the source of truth;
// the shell's session gate still fronts every request that reaches them.
package sysstat

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Storage is one filesystem's fullness in bytes: Used counts allocated blocks,
// Free counts blocks available to new files (df's view — on a reserved-blocks
// filesystem Used+Free is less than the raw size, and the fraction full is
// Used/(Used+Free)).
type Storage struct {
	Used int64
	Free int64
}

// Sampler reads the local kernel. CPU busy share is a delta between two
// /proc/stat snapshots, so the Sampler is stateful. Nearby callers share the
// latest reading instead of shortening one another's sampling interval.
// Safe for concurrent use.
type Sampler struct {
	readStat func() ([]byte, error)             // /proc/stat, behind a seam for tests
	statfs   func(path string) (Storage, error) // statfs(2), likewise
	wait     func()                             // the beat between the cold-start double sample
	now      func() time.Time

	mu          sync.Mutex
	lastBusy    int64
	lastTotal   int64
	sampled     bool
	lastPercent float64
	lastAt      time.Time
}

const cpuSampleInterval = 150 * time.Millisecond

// New returns a Sampler on the real kernel interfaces.
func New() *Sampler {
	return &Sampler{
		readStat: func() ([]byte, error) { return os.ReadFile("/proc/stat") },
		statfs:   statfsPath,
		wait:     func() { time.Sleep(cpuSampleInterval) },
		now:      time.Now,
	}
}

// CPUPercent is the whole-box busy share (0–100) over the interval since the
// previous sample, including fractional percentages on a lightly loaded box.
// The first call takes two samples a beat apart. Calls within that same short
// interval reuse the reading, so a page render or another tab cannot consume
// the counters just before the live stream samples them.
func (s *Sampler) CPUPercent() (float64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.lastAt.IsZero() && s.now().Sub(s.lastAt) < cpuSampleInterval {
		return s.lastPercent, nil
	}
	if !s.sampled {
		if err := s.sample(); err != nil {
			return 0, err
		}
		s.wait()
	}
	prevBusy, prevTotal := s.lastBusy, s.lastTotal
	if err := s.sample(); err != nil {
		return 0, err
	}
	dTotal := s.lastTotal - prevTotal
	if dTotal <= 0 {
		return s.lastPercent, nil
	}
	pct := float64(s.lastBusy-prevBusy) * 100 / float64(dTotal)
	if pct < 0 {
		pct = 0
	} else if pct > 100 {
		pct = 100
	}
	s.lastPercent, s.lastAt = pct, s.now()
	return pct, nil
}

// sample reads the aggregate cpu line of /proc/stat. Busy is everything but
// idle and iowait — the kernel's own accounting of time not spent waiting.
func (s *Sampler) sample() error {
	b, err := s.readStat()
	if err != nil {
		return err
	}
	line, _, _ := strings.Cut(string(b), "\n")
	fields := strings.Fields(line)
	if len(fields) < 5 || fields[0] != "cpu" {
		return fmt.Errorf("sysstat: unexpected /proc/stat line %q", line)
	}
	var total, idle int64
	for i, f := range fields[1:] {
		if i >= 8 {
			break // guest and guest_nice are already included in user and nice
		}
		v, err := strconv.ParseInt(f, 10, 64)
		if err != nil {
			return fmt.Errorf("sysstat: /proc/stat field %q: %w", f, err)
		}
		total += v
		if i == 3 || i == 4 { // idle, iowait
			idle += v
		}
	}
	s.lastBusy, s.lastTotal, s.sampled = total-idle, total, true
	return nil
}

// Root is the fullness of the writable root. OpenWrt mounts the writable upper
// at /overlay; where that mount is absent (a dev container, a plain distro)
// the root itself is the writable filesystem.
func (s *Sampler) Root() (Storage, error) {
	if st, err := s.statfs("/overlay"); err == nil {
		return st, nil
	}
	return s.statfs("/")
}

func statfsPath(path string) (Storage, error) {
	var fs syscall.Statfs_t
	if err := syscall.Statfs(path, &fs); err != nil {
		return Storage{}, err
	}
	bsize := int64(fs.Bsize)
	return Storage{
		Used: (int64(fs.Blocks) - int64(fs.Bfree)) * bsize,
		Free: int64(fs.Bavail) * bsize,
	}, nil
}
