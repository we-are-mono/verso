// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"net/netip"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/we-are-mono/verso/internal/alerts"
	"github.com/we-are-mono/verso/internal/history"
	"github.com/we-are-mono/verso/internal/openwrt"
	"github.com/we-are-mono/verso/internal/probe"
	"github.com/we-are-mono/verso/internal/sensors"
	"github.com/we-are-mono/verso/profiles"
)

// StartHistory records the router's measurements into dir/history.db
// (ADR-019). A file that cannot be opened is reported and the history is kept
// in memory instead, so collection goes on. The uplink is probed at its
// gateway and at each of probes, and alerts go where notify says (ADR-020).
func (s *Server) StartHistory(dir string, probes []netip.Addr, notify alerts.Config) {
	path := filepath.Join(dir, "history.db")
	store, openErr := history.Open(path)
	onDisk := openErr == nil
	if !onDisk {
		log.Printf("verso: history kept in memory only, %s cannot be written: %v", path, openErr)
		var err error
		if store, err = history.Open(":memory:"); err != nil {
			log.Printf("verso: no history: %v", err)
			return
		}
	}
	// The alerter begins before this boot is recorded, so an unclean one
	// alerts.
	host, _ := os.Hostname()
	alerter := alerts.New(store, host)
	alerter.Configure(notify)
	if err := alerter.Begin(context.Background()); err != nil {
		log.Printf("verso: alerts: %v", err)
	}
	s.historyStore, s.changeLog, s.alerter = store, store, alerter
	store.OnHealth(s.storageTurned)
	if !onDisk {
		store.Fail(fmt.Sprintf("%s cannot be written: %v", path, openErr), time.Now())
	}
	if onDisk {
		recordBoot(context.Background(), store, dir)
	}
	prober := probe.New(probes)
	collector := history.NewCollector(store,
		prober.Source,
		interfaceRates(s.telemetry),
		gauges(map[string]func() (float64, error){
			"cpu":       s.stats.CPUPercent,
			"memory":    func() (float64, error) { return readThen("/proc/meminfo", memoryInUse) },
			"conntrack": func() (float64, error) { return readThen("/proc/sys/net/netfilter/nf_conntrack_count", number) },
		}),
		temperatures(),
	)
	// The watcher, the prober and the alerter stop before the collector,
	// which closes the store.
	watchCtx, stopWatch := context.WithCancel(context.Background())
	var watching sync.WaitGroup
	watching.Go(func() { watchWAN(watchCtx, store, openwrt.KernelWAN) })
	watching.Go(func() { s.watchSpool(watchCtx, store, changesSpool) })
	watching.Go(func() { prober.Run(watchCtx) })
	watching.Go(func() { alerter.Run(watchCtx) })
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		collector.Run(ctx)
	}()
	s.historyStop = func() {
		stopWatch()
		watching.Wait()
		cancel()
		<-done
		if onDisk {
			markStopped(dir)
		}
	}
}

// interfaceRates hands over each interface's per-second rates, in bits per
// second, that it has not handed over before. Loopback is not recorded, nor a
// device that is down: its minutes are a gap, not a line of zeros.
func interfaceRates(source telemetrySource) history.Source {
	var lastMS uint64
	return func(time.Time) map[string][]float64 {
		snapshot, err := source.Snapshot(context.Background())
		if err != nil {
			return nil
		}
		out := map[string][]float64{}
		newest := lastMS
		for _, iface := range snapshot.Interfaces {
			if iface.Name == "lo" || normalOperstate(iface.Operstate) == "down" {
				continue
			}
			for _, p := range iface.History {
				if p.TimestampMS <= lastMS {
					continue
				}
				out["iface."+iface.Name+".rx"] = append(out["iface."+iface.Name+".rx"], float64(p.RxBPS))
				out["iface."+iface.Name+".tx"] = append(out["iface."+iface.Name+".tx"], float64(p.TxBPS))
				newest = max(newest, p.TimestampMS)
			}
		}
		lastMS = newest
		return out
	}
}

// gauges reads one value per series each tick; a value that cannot be read is
// left out, so the minute holds a gap rather than a false zero.
func gauges(reads map[string]func() (float64, error)) history.Source {
	return func(time.Time) map[string][]float64 {
		out := make(map[string][]float64, len(reads))
		for series, read := range reads {
			if v, err := read(); err == nil {
				out[series] = []float64{v}
			}
		}
		return out
	}
}

// temperatures reads every working temperature sensor, in °C, named as the
// Hardware page names it.
func temperatures() history.Source {
	board, _ := os.ReadFile("/tmp/sysinfo/board_name")
	profile, _ := sensors.LoadProfile(profiles.FS, strings.TrimSpace(string(board)))
	return func(time.Time) map[string][]float64 {
		return temperatureSeries(sensors.ResolveAll(profile, &sensors.Reader{}).Temps)
	}
}

// temperatureSeries names each working sensor's series by its name, and by its
// chip as well wherever another sensor shares the name.
func temperatureSeries(temps []sensors.TempReading) map[string][]float64 {
	named := map[string]int{}
	for _, t := range temps {
		named[t.Name]++
	}
	out := map[string][]float64{}
	for _, t := range temps {
		if t.Fault {
			continue
		}
		series := "temp." + t.Name
		if named[t.Name] > 1 {
			series += "." + t.Source
		}
		out[series] = []float64{float64(t.MilliC) / 1000}
	}
	return out
}

func readThen(path string, parse func([]byte) (float64, error)) (float64, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	return parse(b)
}

func number(b []byte) (float64, error) {
	return strconv.ParseFloat(string(bytes.TrimSpace(b)), 64)
}

// memoryInUse is MemTotal less MemAvailable, in bytes: what the kernel could
// not hand out at once.
func memoryInUse(meminfo []byte) (float64, error) {
	fields := map[string]float64{}
	for line := range strings.Lines(string(meminfo)) {
		name, rest, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		if kb, err := strconv.ParseFloat(strings.TrimSuffix(strings.TrimSpace(rest), " kB"), 64); err == nil {
			fields[name] = kb * 1024
		}
	}
	total, okTotal := fields["MemTotal"]
	available, okAvailable := fields["MemAvailable"]
	if !okTotal || !okAvailable {
		return 0, fmt.Errorf("meminfo: no MemTotal or MemAvailable")
	}
	return total - available, nil
}
