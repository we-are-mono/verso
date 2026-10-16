// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"context"

	"github.com/we-are-mono/verso/internal/widget"
)

// serviceStatusEvery is how many live Overview frames pass between two reads
// of procd's service table: a service fails on the scale of seconds, and the
// read is the router's to pay on every frame of every open Overview.
const serviceStatusEvery = 5

// applyFailedServices reads the services procd stopped restarting, the cause
// the Overview's System card owns. A failed read clears the previous one, so
// an old failure never outlives the router's answer.
func (s *Server) applyFailedServices(ctx context.Context, sid string, ov *widget.Overview) {
	ov.FailedServices = nil
	if failed, err := s.backend.FailedServices(ctx, sid); err == nil {
		ov.FailedServices = failed
	}
}
