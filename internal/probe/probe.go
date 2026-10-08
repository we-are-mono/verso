// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

// Package probe measures the uplink's quality (ADR-019): one ICMP echo to each
// target every ten seconds — the WAN's gateway, and any address the operator
// adds — over the kernel's unprivileged ping sockets, so the shell sends them
// itself. The package grants the verso group those sockets through
// net.ipv4.ping_group_range, and nothing more.
package probe

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"log"
	"net/netip"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/sys/unix"
)

const (
	every   = 10 * time.Second
	timeout = 2 * time.Second
)

var (
	// ErrNotPermitted is a ping socket the kernel refuses this group: the
	// probe cannot run, which is not the same as the target not answering.
	ErrNotPermitted = errors.New("probe: ping sockets are not permitted for this group (net.ipv4.ping_group_range)")
	// ErrTimeout is an echo that got no answer in time: a loss.
	ErrTimeout = errors.New("probe: no answer")
)

var sequence atomic.Uint32

// Ping sends one ICMP echo to addr and waits up to wait for its answer,
// returning the round trip.
func Ping(addr netip.Addr, wait time.Duration) (time.Duration, error) {
	addr = addr.Unmap()
	family, proto, request, reply := unix.AF_INET6, unix.IPPROTO_ICMPV6, byte(128), byte(129)
	var to unix.Sockaddr = &unix.SockaddrInet6{Addr: addr.As16()}
	if addr.Is4() {
		family, proto, request, reply = unix.AF_INET, unix.IPPROTO_ICMP, 8, 0
		to = &unix.SockaddrInet4{Addr: addr.As4()}
	}
	fd, err := unix.Socket(family, unix.SOCK_DGRAM|unix.SOCK_CLOEXEC, proto)
	if errors.Is(err, unix.EACCES) || errors.Is(err, unix.EPERM) {
		return 0, ErrNotPermitted
	}
	if err != nil {
		return 0, err
	}
	defer func() { _ = unix.Close(fd) }()

	// The kernel sets the identifier to the socket's and fills the checksum;
	// the sequence tells this echo's answer from a late one.
	seq := uint16(sequence.Add(1))
	msg := []byte{request, 0, 0, 0, 0, 0, byte(seq >> 8), byte(seq), 'v', 'e', 'r', 's', 'o', 0, 0, 0}
	start := time.Now()
	if err := unix.Sendto(fd, msg, 0, to); err != nil {
		return 0, err
	}
	buf := make([]byte, 1500)
	for {
		left := wait - time.Since(start)
		if left <= 0 {
			return 0, ErrTimeout
		}
		tv := unix.NsecToTimeval(left.Nanoseconds())
		if err := unix.SetsockoptTimeval(fd, unix.SOL_SOCKET, unix.SO_RCVTIMEO, &tv); err != nil {
			return 0, err
		}
		n, _, err := unix.Recvfrom(fd, buf, 0)
		if errors.Is(err, unix.EAGAIN) || errors.Is(err, unix.EINTR) {
			continue
		}
		if err != nil {
			return 0, err
		}
		if n >= 8 && buf[0] == reply && binary.BigEndian.Uint16(buf[6:8]) == seq {
			return time.Since(start), nil
		}
	}
}

// Gateway is the IPv4 next hop of the main table's default route, if it has
// one; a point-to-point uplink (PPPoE) does not.
func Gateway() (netip.Addr, bool) {
	route, err := os.ReadFile("/proc/net/route")
	if err != nil {
		return netip.Addr{}, false
	}
	return gateway(route)
}

func gateway(route []byte) (netip.Addr, bool) {
	const rtfGateway = 0x2
	for line := range bytes.Lines(route) {
		f := bytes.Fields(line)
		if len(f) < 8 || string(f[1]) != "00000000" || string(f[7]) != "00000000" {
			continue
		}
		flags, err := hex.DecodeString(string(f[3]))
		if err != nil || len(flags) != 2 || flags[1]&rtfGateway == 0 {
			continue
		}
		raw, err := hex.DecodeString(string(f[2]))
		if err != nil || len(raw) != 4 {
			continue
		}
		// Host byte order on the little-endian boards Verso builds for.
		return netip.AddrFrom4([4]byte{raw[3], raw[2], raw[1], raw[0]}), true
	}
	return netip.Addr{}, false
}

// Target is one address probed each round, named for its series. Addr says
// where it is this round; a target with none is skipped.
type Target struct {
	Name string
	Addr func() (netip.Addr, bool)
}

// Fixed is a target that is always at addr.
func fixed(addr netip.Addr) func() (netip.Addr, bool) {
	return func() (netip.Addr, bool) { return addr, true }
}

// Prober probes its targets and keeps the readings until they are handed
// over. Safe for concurrent use.
type Prober struct {
	ping    func(netip.Addr, time.Duration) (time.Duration, error)
	targets []Target

	mu       sync.Mutex
	readings map[string][]float64
	refused  bool
}

// New probes the WAN's gateway and each extra address.
func New(extra []netip.Addr) *Prober {
	targets := []Target{{Name: "gateway", Addr: Gateway}}
	for _, addr := range extra {
		targets = append(targets, Target{Name: addr.String(), Addr: fixed(addr)})
	}
	return &Prober{ping: Ping, targets: targets}
}

// Round sends one echo to each target at once and records what came back: an
// answer's round trip in milliseconds, and the loss as 0 or 100, so a
// minute's average is its loss in percent.
func (p *Prober) Round() {
	var wg sync.WaitGroup
	for _, target := range p.targets {
		addr, ok := target.Addr()
		if !ok {
			continue
		}
		wg.Go(func() {
			rtt, err := p.ping(addr, timeout)
			p.mu.Lock()
			defer p.mu.Unlock()
			if p.readings == nil {
				p.readings = map[string][]float64{}
			}
			prefix := "wan." + target.Name + "."
			switch {
			case err == nil:
				p.readings[prefix+"latency"] = append(p.readings[prefix+"latency"], float64(rtt.Microseconds())/1000)
				p.readings[prefix+"loss"] = append(p.readings[prefix+"loss"], 0)
			case errors.Is(err, ErrNotPermitted):
				if !p.refused {
					log.Printf("verso: the uplink is not probed: %v", err)
					p.refused = true
				}
			default:
				p.readings[prefix+"loss"] = append(p.readings[prefix+"loss"], 100)
			}
		})
	}
	wg.Wait()
}

// Run probes every ten seconds until ctx ends.
func (p *Prober) Run(ctx context.Context) {
	tick := time.NewTicker(every)
	defer tick.Stop()
	for {
		p.Round()
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

// Source hands over the readings taken since it was last asked; it is a
// history.Source.
func (p *Prober) Source(time.Time) map[string][]float64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := p.readings
	p.readings = nil
	return out
}
