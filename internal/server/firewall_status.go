// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"context"

	"github.com/we-are-mono/verso/internal/widget"
)

func (s *Server) applyFirewallStatus(ctx context.Context, sid string, ov *widget.Overview) {
	// Clear the previous frame before reading so failures never leave an old
	// green status or rule count on screen.
	ov.FirewallState, ov.FirewallRules, ov.FirewallRulesKnown = "", 0, false
	if status, err := s.backend.FirewallStatus(ctx, sid); err == nil {
		ov.FirewallState = status.State
	}
	// Read through the same session-scoped UCI snapshot as the plugin listing.
	// Kernel rule counts include generated plumbing and cannot match its rows.
	if cfg, err := s.backend.UCIConfig(ctx, sid, "firewall"); err == nil && cfg != nil {
		ov.FirewallRulesKnown = true
		for _, raw := range cfg {
			if section, ok := raw.(map[string]any); ok && section[".type"] == "rule" {
				ov.FirewallRules++
			}
		}
	}
}
