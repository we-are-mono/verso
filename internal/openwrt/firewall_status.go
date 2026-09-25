// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package openwrt

import (
	"context"
	"fmt"
	"time"
)

// FirewallStatus is the loaded inet fw4 table. Rules counts kernel rules,
// including generated rules, rather than configured UCI sections.
type FirewallStatus struct {
	State string `json:"state"` // active, inactive, partial
	Rules int    `json:"rules"`
}

type firewallStatusFn func(context.Context, string) (FirewallStatus, error)

func (b *NativeBackend) FirewallStatus(ctx context.Context, sid string) (FirewallStatus, error) {
	return b.fwStatus(ctx, sid)
}

func dialFirewallStatus(socket string) firewallStatusFn {
	return func(ctx context.Context, sid string) (FirewallStatus, error) {
		var result struct {
			State string `json:"state"`
			Rules *int   `json:"rules"`
		}
		// A status read must not hold the homepage or its live stream behind the
		// helper's much longer timeout for administrative operations.
		if err := callHelperWithin(ctx, socket, "firewallStatus", sid, nil, &result, 2*time.Second); err != nil {
			return FirewallStatus{}, err
		}
		if (result.State != "active" && result.State != "inactive" && result.State != "partial") || result.Rules == nil || *result.Rules < 0 {
			return FirewallStatus{}, fmt.Errorf("verso-rpcd: invalid firewall status")
		}
		return FirewallStatus{State: result.State, Rules: *result.Rules}, nil
	}
}
