// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package openwrt

import (
	"context"
	"strconv"
	"time"
)

// FirewallLogBatch is a reader's slice of the helper's bounded NFLOG history.
// Generation changes on helper restart; reading never consumes another reader's
// events. Available describes the collector, independently of an empty history.
type FirewallLogBatch struct {
	Entries    []LogEntry `json:"entries"`
	Generation string     `json:"generation"`
	Available  bool       `json:"available"`
	Reset      bool       `json:"reset"`
	Lost       bool       `json:"lost"`
	Overruns   uint64     `json:"overruns"`
}

type firewallLogFn func(context.Context, string, string, int64, int) (FirewallLogBatch, error)

func (b *NativeBackend) FirewallLogRead(ctx context.Context, sid, generation string, after int64, limit int) (FirewallLogBatch, error) {
	return b.firewallLog(ctx, sid, generation, after, limit)
}

func dialFirewallLog(socket string) firewallLogFn {
	return func(ctx context.Context, sid, generation string, after int64, limit int) (FirewallLogBatch, error) {
		var result FirewallLogBatch
		err := callHelperWithin(ctx, socket, "firewallLog", sid, map[string]string{
			"generation": generation, "after": strconv.FormatInt(after, 10), "limit": strconv.Itoa(limit),
		}, &result, 5*time.Second)
		return result, err
	}
}
