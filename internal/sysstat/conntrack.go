// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package sysstat

import (
	"os"
	"strconv"
	"strings"
)

// Conntrack: per-connection truth from the kernel's flow table. With
// accounting on (net.netfilter.nf_conntrack_acct=1) every tracked connection
// carries byte counters both directions — aggregated by a device's address
// this is per-device traffic, the thing interface counters can never split.
// The router only tracks what transits it, so LAN-to-LAN chatter between two
// devices on the same bridge is invisible — per-device *internet* usage is
// exactly what this measures.

const conntrackPath = "/proc/net/nf_conntrack"

// ConnEntry is one tracked connection, reduced to what a device view needs:
// who started it, to where, and the bytes each direction has carried.
type ConnEntry struct {
	Proto   string
	Src     string // the origin direction's source — the initiator
	Dst     string
	TxBytes int64 // bytes the initiator sent (origin direction)
	RxBytes int64 // bytes that came back (reply direction)
}

// ConntrackDump reads the kernel's conntrack table text.
func ConntrackDump() ([]byte, error) { return os.ReadFile(conntrackPath) }

// ParseConntrack reads /proc/net/nf_conntrack lines. Tokens are positional
// only at the front (family, protocol); the rest is key=value, where the
// first src/dst/bytes belong to the origin direction and the second set to
// the reply. Absent bytes (accounting off) parse as zero.
func ParseConntrack(raw []byte) []ConnEntry {
	var entries []ConnEntry
	for _, line := range strings.Split(string(raw), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		e := ConnEntry{Proto: fields[2]}
		srcSeen, bytesSeen := 0, 0
		for _, f := range fields[3:] {
			key, value, ok := strings.Cut(f, "=")
			if !ok {
				continue
			}
			switch key {
			case "src":
				srcSeen++
				if srcSeen == 1 {
					e.Src = value
				}
			case "dst":
				if srcSeen == 1 {
					e.Dst = value
				}
			case "bytes":
				bytesSeen++
				n, _ := strconv.ParseInt(value, 10, 64)
				if bytesSeen == 1 {
					e.TxBytes = n
				} else {
					e.RxBytes = n
				}
			}
		}
		if e.Src != "" {
			entries = append(entries, e)
		}
	}
	return entries
}

// DeviceTraffic is one device's aggregate across its tracked connections.
type DeviceTraffic struct {
	TxBytes int64 // sent by the device
	RxBytes int64 // received by the device
	Conns   int
}

// AggregateTraffic folds the entries down to per-address device totals for
// the given addresses (a device's v4 and v6 addresses together). A device
// usually initiates (src of the origin direction); a connection *to* it
// counts with the directions mirrored.
func AggregateTraffic(entries []ConnEntry, addrs map[string]string) map[string]DeviceTraffic {
	totals := make(map[string]DeviceTraffic)
	for _, e := range entries {
		if key, ok := addrs[e.Src]; ok {
			t := totals[key]
			t.TxBytes += e.TxBytes
			t.RxBytes += e.RxBytes
			t.Conns++
			totals[key] = t
			continue
		}
		if key, ok := addrs[e.Dst]; ok {
			t := totals[key]
			t.TxBytes += e.RxBytes
			t.RxBytes += e.TxBytes
			t.Conns++
			totals[key] = t
		}
	}
	return totals
}

// AggregateByAddress folds the entries to totals per initiating address —
// the shape the privileged helper hands the shell, which then groups
// addresses into devices. The reply side is credited mirrored, so a
// connection *toward* an address still counts for it.
func AggregateByAddress(entries []ConnEntry) map[string]DeviceTraffic {
	totals := make(map[string]DeviceTraffic)
	for _, e := range entries {
		src := totals[e.Src]
		src.TxBytes += e.TxBytes
		src.RxBytes += e.RxBytes
		src.Conns++
		totals[e.Src] = src
		if e.Dst == "" || e.Dst == e.Src {
			continue
		}
		dst := totals[e.Dst]
		dst.TxBytes += e.RxBytes
		dst.RxBytes += e.TxBytes
		dst.Conns++
		totals[e.Dst] = dst
	}
	return totals
}
