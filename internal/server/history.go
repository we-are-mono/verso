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
)

// StartHistory records the router's measurements into dir/history.db
// (ADR-019), after carrying over the history kept in from, the directory
// chosen before, when that names another. A file that cannot be opened is
// reported and the history is kept in memory instead, so collection goes on.
// The uplink is probed at its gateway and at each of probes, and alerts go
// where notify says (ADR-020).
func (s *Server) StartHistory(dir, from string, probes []netip.Addr, notify alerts.Config) {
	carryHistory(from, dir)
	store, onDisk := s.openHistory(dir, notify)
	if store == nil {
		return
	}
	board := boardProfile()
	keepTemperatureHistory(context.Background(), store, sensors.ResolveAll(board, &sensors.Reader{}).Temps)
	prober := probe.New(probes)
	collector := history.NewCollector(store,
		prober.Source,
		interfaceRates(s.telemetry),
		gauges(map[string]func() (float64, error){
			"cpu":       s.stats.CPUPercent,
			"memory":    func() (float64, error) { return readThen("/proc/meminfo", memoryInUse) },
			"conntrack": func() (float64, error) { return readThen("/proc/sys/net/netfilter/nf_conntrack_count", number) },
		}),
		temperatures(board),
	)
	// The watcher, the prober, the alerter and a rescan's scrub, cut short,
	// stop before the collector, which closes the store.
	watchCtx, stopWatch := context.WithCancel(context.Background())
	var watching sync.WaitGroup
	watching.Go(func() { watchWAN(watchCtx, store, openwrt.KernelWAN) })
	watching.Go(func() { s.watchSpool(watchCtx, store, changesSpool) })
	watching.Go(func() { prober.Run(watchCtx) })
	watching.Go(func() { s.alerter.Run(watchCtx) })
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		collector.Run(ctx)
	}()
	s.historyStop = func() {
		stopWatch()
		watching.Wait()
		s.stopScrubs()
		cancel()
		<-done
		if onDisk {
			markStopped(dir)
		}
	}
}

// carryHistory moves the history kept in from into dir, with the mark of its
// clean stop, so this start reads the boot as the stop left it. A history
// that cannot move stays where it was, and the shell says why in its log.
func carryHistory(from, dir string) {
	moved, err := history.Move(from, dir)
	switch {
	case !moved && err != nil:
		log.Printf("verso: history in %s not moved to %s: %v", from, dir, err)
		return
	case !moved:
		return
	case err != nil:
		log.Printf("verso: history moved from %s to %s, but not all of it left %s: %v", from, dir, from, err)
	default:
		log.Printf("verso: history moved from %s to %s", from, dir)
	}
	mark := filepath.Join(from, stoppedMark)
	if _, err := os.Stat(mark); err == nil {
		markStopped(dir)
		_ = os.Remove(mark)
	}
}

// openHistory opens dir/history.db, or a history in memory where the file
// cannot be written, and keeps it on s with its alerter. A history on disk
// records this boot and is scrubbed of every secret the shell knows. It
// answers the store, nil when there is none, and whether it is on disk.
func (s *Server) openHistory(dir string, notify alerts.Config) (*history.Store, bool) {
	path := filepath.Join(dir, "history.db")
	store, openErr := history.Open(path)
	onDisk := openErr == nil
	if !onDisk {
		log.Printf("verso: history kept in memory only, %s cannot be written: %v", path, openErr)
		var err error
		if store, err = history.Open(history.Memory); err != nil {
			log.Printf("verso: no history: %v", err)
			return nil, false
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
		s.scrubHistory(context.Background())
	}
	return store, onDisk
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
func temperatures(profile *sensors.Profile) history.Source {
	return func(time.Time) map[string][]float64 {
		return temperatureSeries(sensors.ResolveAll(profile, &sensors.Reader{}).Temps)
	}
}

// keepTemperatureHistory moves what the shell recorded under the names it gave
// temperatures before the kernel's (sensors.SeriesRenames) to the names it
// records under now, before the first minute is recorded. Once moved there is
// nothing under the old names, so every start after asks for nothing.
func keepTemperatureHistory(ctx context.Context, store *history.Store, temps []sensors.TempReading) {
	renames := map[string]string{}
	for was, now := range sensors.SeriesRenames(temps) {
		renames["temp."+was] = "temp." + now
	}
	if err := store.RenameSeries(ctx, renames); err != nil {
		log.Printf("verso: history: temperatures recorded under their old names stay there: %v", err)
	}
}

// temperatureSeries records each working sensor under its series, the name the
// kernel gives it (sensors.TempReading.Series), so a profile that renames a
// reading keeps its history.
func temperatureSeries(temps []sensors.TempReading) map[string][]float64 {
	out := map[string][]float64{}
	for _, t := range temps {
		if t.Fault || t.Series == "" {
			continue
		}
		out[t.Series] = []float64{float64(t.MilliC) / 1000}
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
