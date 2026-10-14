// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package ports

import (
	"slices"
	"testing"
)

// TestBoardOrder: OpenWrt's board description names the WAN device and the
// LAN ports, in either of its two spellings (one device, or a list of ports).
// Their order is the order its ports are drawn in: the WAN first, then the LAN
// ports as listed. A description with no network section, or one that does
// not parse, gives no order and no error: the ports then fall back to names.
func TestBoardOrder(t *testing.T) {
	for _, tc := range []struct {
		name, data string
		want       []string
	}{
		{"DSA router", `{"network":{"lan":{"ports":["lan1","lan2","lan3","lan4"],"protocol":"static"},"wan":{"device":"wan","protocol":"dhcp"}}}`,
			[]string{"wan", "lan1", "lan2", "lan3", "lan4"}},
		{"device spelling", `{"network":{"lan":{"device":"eth0","protocol":"static"},"wan":{"ports":["eth1","eth2"],"protocol":"dhcp"}}}`,
			[]string{"eth1", "eth2", "eth0"}},
		{"LAN only", `{"network":{"lan":{"device":"eth0"}}}`, []string{"eth0"}},
		{"no network", `{"model":{"id":"qemu-standard-pc-q35-ich9-2009","name":"QEMU Standard PC"}}`, nil},
		{"malformed", `{"network":`, nil},
		{"a port named twice", `{"network":{"lan":{"ports":["eth0","eth1"]},"wan":{"device":"eth1"}}}`, []string{"eth1", "eth0"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := BoardOrder([]byte(tc.data)); !slices.Equal(got, tc.want) {
				t.Errorf("BoardOrder = %v, want %v", got, tc.want)
			}
		})
	}
}
