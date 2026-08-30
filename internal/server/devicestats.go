// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"context"
	"log"

	"github.com/we-are-mono/verso/internal/openwrt"
)

// deviceStats reads one network device's live state (carrier, negotiated speed,
// byte counters), degrading to zero state rather than failing — the port exists
// even when its stats cannot be read. Used by the overview stream (events.go)
// to feed the per-interface WAN readings.
func (s *Server) deviceStats(ctx context.Context, sid, dev string) openwrt.DeviceStats {
	st, err := s.backend.DeviceStats(ctx, sid, dev)
	if err != nil {
		log.Printf("verso: device %s stats unavailable: %v", dev, err)
		return openwrt.DeviceStats{}
	}
	return st
}
