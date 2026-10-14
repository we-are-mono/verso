// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package ports

import "encoding/json"

// BoardOrder is the port order OpenWrt's board description (/etc/board.json)
// gives: the WAN's device or ports first, then the LAN's, each as listed and
// each name once. The description records roles, not places on the case, so
// WAN first is the convention, not a measurement. A description without a
// network section, or one that does not parse, gives none.
func BoardOrder(data []byte) []string {
	type role struct {
		Device string   `json:"device"`
		Ports  []string `json:"ports"`
	}
	var board struct {
		Network map[string]role `json:"network"`
	}
	if err := json.Unmarshal(data, &board); err != nil {
		return nil
	}
	var order []string
	seen := map[string]bool{}
	for _, name := range []string{"wan", "lan"} {
		r, ok := board.Network[name]
		if !ok {
			continue
		}
		names := r.Ports
		if r.Device != "" {
			names = append([]string{r.Device}, names...)
		}
		for _, port := range names {
			if port != "" && !seen[port] {
				seen[port] = true
				order = append(order, port)
			}
		}
	}
	return order
}
