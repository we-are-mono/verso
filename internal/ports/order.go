// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package ports

import (
	"slices"
	"strconv"

	"github.com/we-are-mono/verso/internal/telemetry"
)

// IsPort is whether an interface is a socket on the case: a wired physical
// NIC that is not a DSA conduit (the switch's link to the SoC, whose ports
// are the switch's own netdevs). Wireless and virtual devices are no ports.
func IsPort(iface telemetry.Interface) bool {
	return iface.Physical && !iface.Conduit
}

// Order is the left-to-right order of the board's ports. A profile's list is
// the order outright, since it is the board's own. Without one, the board
// description's order comes first, keeping only the ports there are, and every
// other port follows in natural name order (eth2 before eth10).
func Order(interfaces []telemetry.Interface, profile, board []string) []string {
	if len(profile) > 0 {
		return slices.Clone(profile)
	}
	present := map[string]bool{}
	var rest []string
	for _, iface := range interfaces {
		if IsPort(iface) {
			present[iface.Name] = true
			rest = append(rest, iface.Name)
		}
	}
	var order []string
	placed := map[string]bool{}
	for _, name := range board {
		if present[name] && !placed[name] {
			placed[name] = true
			order = append(order, name)
		}
	}
	rest = slices.DeleteFunc(rest, func(name string) bool { return placed[name] })
	slices.SortFunc(rest, naturalCompare)
	return append(order, rest...)
}

// naturalCompare orders names as people count: runs of digits compare by
// value, so eth2 comes before eth10.
func naturalCompare(a, b string) int {
	for a != "" && b != "" {
		da, db := digitRun(a), digitRun(b)
		if da > 0 && db > 0 {
			na, _ := strconv.ParseUint(a[:da], 10, 64)
			nb, _ := strconv.ParseUint(b[:db], 10, 64)
			if na != nb {
				if na < nb {
					return -1
				}
				return 1
			}
			a, b = a[da:], b[db:]
			continue
		}
		if a[0] != b[0] {
			if a[0] < b[0] {
				return -1
			}
			return 1
		}
		a, b = a[1:], b[1:]
	}
	return len(a) - len(b)
}

// digitRun is the length of the run of ASCII digits s starts with.
func digitRun(s string) int {
	n := 0
	for n < len(s) && s[n] >= '0' && s[n] <= '9' {
		n++
	}
	return n
}
