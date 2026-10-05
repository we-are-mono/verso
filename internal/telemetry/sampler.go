// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package telemetry

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	defaultNetClass = "/sys/class/net"
	defaultInterval = time.Second
	defaultHistory  = 60
)

type counters struct {
	timestampMS uint64
	rxBytes     uint64
	txBytes     uint64
	rxPackets   uint64
	txPackets   uint64
}

type observation struct {
	name      string
	operstate string
	kind      string
	physical  bool
	parent    string
	members   []string
	counters  counters
}

// Sampler owns one process-wide sampling clock and a bounded history for every
// kernel network interface. Snapshot is a read-only copy; callers never cause
// an additional kernel collection.
type Sampler struct {
	root         string
	interval     time.Duration
	historyLimit int
	now          func() time.Time

	mu       sync.RWMutex
	history  map[string][]Point
	previous map[string]counters
	snapshot Snapshot
	lastErr  error

	startOnce sync.Once
	stopOnce  sync.Once
	cancel    context.CancelFunc
	done      chan struct{}
}

func NewSampler() *Sampler {
	return newSampler(defaultNetClass, defaultInterval, defaultHistory, time.Now)
}

func newSampler(root string, interval time.Duration, historyLimit int, now func() time.Time) *Sampler {
	return &Sampler{
		root:         root,
		interval:     interval,
		historyLimit: historyLimit,
		now:          now,
		history:      make(map[string][]Point),
		previous:     make(map[string]counters),
	}
}

// Start takes an immediate baseline sample and then samples once per interval.
func (s *Sampler) Start() {
	s.startOnce.Do(func() {
		ctx, cancel := context.WithCancel(context.Background())
		s.cancel = cancel
		s.done = make(chan struct{})
		s.sample()
		go s.run(ctx)
	})
}

func (s *Sampler) run(ctx context.Context) {
	defer close(s.done)
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.sample()
		}
	}
}

func (s *Sampler) Stop() {
	s.stopOnce.Do(func() {
		if s.cancel == nil {
			return
		}
		s.cancel()
		<-s.done
	})
}

func (s *Sampler) Snapshot(ctx context.Context) (Snapshot, error) {
	select {
	case <-ctx.Done():
		return Snapshot{}, ctx.Err()
	default:
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneSnapshot(s.snapshot), s.lastErr
}

func (s *Sampler) sample() {
	now := s.now()
	timestampMS := uint64(now.UnixMilli())
	observed, err := readInterfaces(s.root, timestampMS)
	if err != nil {
		s.mu.Lock()
		s.lastErr = err
		s.mu.Unlock()
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	nextHistory := make(map[string][]Point, len(observed))
	nextPrevious := make(map[string]counters, len(observed))
	interfaces := make([]Interface, 0, len(observed))
	for _, observedInterface := range observed {
		current := observedInterface.counters
		point := Point{
			TimestampMS: current.timestampMS,
			RxBytes:     current.rxBytes,
			TxBytes:     current.txBytes,
			RxPackets:   current.rxPackets,
			TxPackets:   current.txPackets,
		}
		name := observedInterface.name
		if previous, ok := s.previous[name]; ok && current.timestampMS > previous.timestampMS {
			elapsedMS := current.timestampMS - previous.timestampMS
			point.RxBPS = counterRate(current.rxBytes, previous.rxBytes, elapsedMS, 8)
			point.TxBPS = counterRate(current.txBytes, previous.txBytes, elapsedMS, 8)
			point.RxPPS = counterRate(current.rxPackets, previous.rxPackets, elapsedMS, 1)
			point.TxPPS = counterRate(current.txPackets, previous.txPackets, elapsedMS, 1)
		}
		history := append(s.history[name], point)
		if len(history) > s.historyLimit {
			history = history[len(history)-s.historyLimit:]
		}
		nextHistory[name] = history
		nextPrevious[name] = current
		interfaces = append(interfaces, Interface{
			Name: name, Operstate: observedInterface.operstate,
			Kind: observedInterface.kind, Physical: observedInterface.physical,
			Parent: observedInterface.parent, Members: observedInterface.members,
			History: history,
		})
	}
	s.history = nextHistory
	s.previous = nextPrevious
	s.snapshot = Snapshot{TimestampMS: timestampMS, Interfaces: interfaces}
	s.lastErr = nil
}

// readInterfaces returns counters in os.ReadDir's stable filename order. An
// interface whose counters cannot be read is left out of the sample.
func readInterfaces(root string, timestampMS uint64) ([]observation, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("telemetry: read %s: %w", root, err)
	}
	names := make(map[string]bool, len(entries))
	indexNames := make(map[uint64]string, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		names[name] = true
		if index, indexErr := readCounter(filepath.Join(root, name, "ifindex")); indexErr == nil {
			indexNames[index] = name
		}
	}
	observed := make([]observation, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		current := counters{timestampMS: timestampMS}
		values := []*uint64{&current.rxBytes, &current.txBytes, &current.rxPackets, &current.txPackets}
		files := []string{"rx_bytes", "tx_bytes", "rx_packets", "tx_packets"}
		valid := true
		for index, file := range files {
			value, readErr := readCounter(filepath.Join(root, name, "statistics", file))
			if readErr != nil {
				valid = false
				break
			}
			*values[index] = value
		}
		if valid {
			kind, physical, parent, members := readInterfaceMetadata(root, name, names, indexNames)
			observed = append(observed, observation{
				name: name, operstate: readOperstate(root, name), kind: kind,
				physical: physical, parent: parent, members: members, counters: current,
			})
		}
	}
	return observed, nil
}

// readInterfaceMetadata classifies one kernel netdev from sysfs. A hardware
// device link identifies a physical NIC, except for wireless devices; virtual
// relationships come from lower_* links, iflink, bridge membership, and finally
// the conventional numeric VLAN suffix used by Linux and OpenWrt.
func readInterfaceMetadata(root, name string, names map[string]bool, indexNames map[uint64]string) (string, bool, string, []string) {
	base := filepath.Join(root, name)
	wireless := pathExists(filepath.Join(base, "wireless")) || pathExists(filepath.Join(base, "phy80211"))
	physical := pathExists(filepath.Join(base, "device")) && !wireless
	bridge := pathExists(filepath.Join(base, "bridge"))

	parent := ""
	if entries, err := os.ReadDir(base); err == nil {
		for _, entry := range entries {
			if lower, ok := strings.CutPrefix(entry.Name(), "lower_"); ok && names[lower] {
				parent = lower
				break
			}
		}
	}
	if parent == "" {
		if link, err := readCounter(filepath.Join(base, "iflink")); err == nil {
			if index, indexErr := readCounter(filepath.Join(base, "ifindex")); indexErr == nil && link != index {
				parent = indexNames[link]
			}
		}
	}
	if parent == "" {
		if candidate, vlan := vlanParent(name); vlan && names[candidate] {
			parent = candidate
		}
	}

	var members []string
	if entries, err := os.ReadDir(filepath.Join(base, "brif")); err == nil {
		members = make([]string, 0, len(entries))
		for _, entry := range entries {
			members = append(members, entry.Name())
		}
	}

	switch {
	case physical:
		return "port", true, parent, members
	case wireless:
		return "wifi", false, parent, members
	case bridge:
		return "bridge", false, parent, members
	case name == "lo":
		return "loopback", false, parent, members
	case isVLANName(name):
		return "vlan", false, parent, members
	case strings.HasPrefix(name, "pppoe-"):
		return "pppoe", false, parent, members
	case tunnelDevice(base), strings.HasPrefix(name, "tun"), strings.HasPrefix(name, "tap"),
		strings.HasPrefix(name, "tailscale"), strings.HasPrefix(name, "wg"),
		strings.HasPrefix(name, "ip6tnl"), strings.HasPrefix(name, "sit"),
		strings.HasPrefix(name, "gre"):
		return "tunnel", false, parent, members
	default:
		return "virtual", false, parent, members
	}
}

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func vlanParent(name string) (string, bool) {
	index := strings.LastIndexByte(name, '.')
	if index < 1 || index == len(name)-1 {
		return "", false
	}
	if _, err := strconv.ParseUint(name[index+1:], 10, 16); err != nil {
		return "", false
	}
	return name[:index], true
}

func isVLANName(name string) bool {
	_, ok := vlanParent(name)
	return ok
}

// tunnelDevice is whether the kernel says the device is a tunnel, whatever it
// is called: a tun or tap carries tun_flags, and WireGuard and the IP tunnels
// report a tunnel link type (none, ipip, ip6tnl, sit, gre, ip6gre).
func tunnelDevice(base string) bool {
	if pathExists(filepath.Join(base, "tun_flags")) {
		return true
	}
	linkType, err := readCounter(filepath.Join(base, "type"))
	if err != nil {
		return false
	}
	switch linkType {
	case 65534, 768, 769, 776, 778, 823:
		return true
	}
	return false
}

func readCounter(path string) (uint64, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return 0, fmt.Errorf("telemetry: read %s: %w", path, err)
	}
	value, err := strconv.ParseUint(strings.TrimSpace(string(raw)), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("telemetry: parse %s: %w", path, err)
	}
	return value, nil
}

func readOperstate(root, name string) string {
	raw, err := os.ReadFile(filepath.Join(root, name, "operstate"))
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(raw))
}

func counterRate(current, previous, elapsedMS, multiplier uint64) uint64 {
	if current < previous || elapsedMS == 0 {
		return 0
	}
	delta := current - previous
	factor := multiplier * 1000
	if delta > math.MaxUint64/factor {
		return math.MaxUint64
	}
	return delta * factor / elapsedMS
}

func cloneSnapshot(source Snapshot) Snapshot {
	clone := source
	clone.Interfaces = make([]Interface, len(source.Interfaces))
	for index, iface := range source.Interfaces {
		clone.Interfaces[index] = iface
		clone.Interfaces[index].Members = append([]string(nil), iface.Members...)
		clone.Interfaces[index].History = append([]Point(nil), iface.History...)
	}
	return clone
}
