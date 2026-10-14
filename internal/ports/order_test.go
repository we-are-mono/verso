// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package ports

import (
	"slices"
	"testing"

	"github.com/we-are-mono/verso/internal/telemetry"
)

func port(name string) telemetry.Interface {
	return telemetry.Interface{Name: name, Kind: "port", Physical: true}
}

// TestOrder: the ports stand in the profile's left-to-right order when the
// board has one; else in the board description's, with every other port
// after it; else in natural name order. Only wired physical ports count: a
// DSA conduit, a wireless device and anything virtual are not on the case.
func TestOrder(t *testing.T) {
	conduit := port("eth0")
	conduit.Conduit = true
	interfaces := []telemetry.Interface{
		port("eth10"), port("eth2"), port("eth1"), port("lan2"), port("lan1"), port("wan"), conduit,
		{Name: "wlan0", Kind: "wifi"}, {Name: "br-lan", Kind: "bridge"}, {Name: "lo", Kind: "loopback"},
		{Name: "wg0", Kind: "tunnel"},
	}
	for _, tc := range []struct {
		name           string
		profile, board []string
		want           []string
	}{
		{"profile wins", []string{"eth1", "eth2", "eth10"}, []string{"wan"}, []string{"eth1", "eth2", "eth10"}},
		{"board, then the rest", nil, []string{"wan", "lan1", "lan2", "lan3"},
			[]string{"wan", "lan1", "lan2", "eth1", "eth2", "eth10"}},
		{"natural", nil, nil, []string{"eth1", "eth2", "eth10", "lan1", "lan2", "wan"}},
		{"board names a conduit", nil, []string{"eth0", "wan"}, []string{"wan", "eth1", "eth2", "eth10", "lan1", "lan2"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := Order(interfaces, tc.profile, tc.board); !slices.Equal(got, tc.want) {
				t.Errorf("Order = %v, want %v", got, tc.want)
			}
		})
	}
}
