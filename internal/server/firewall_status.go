// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"context"

	"github.com/we-are-mono/verso/internal/widget"
)

// applyFirewallStatus reads whether fw4's ruleset is loaded in the kernel, the
// one firewall fact the Overview's verdict counts. A failed read clears the
// previous one, so an old "active" never outlives the router's answer.
func (s *Server) applyFirewallStatus(ctx context.Context, sid string, ov *widget.Overview) {
	ov.FirewallState = ""
	if status, err := s.backend.FirewallStatus(ctx, sid); err == nil {
		ov.FirewallState = status.State
	}
}
