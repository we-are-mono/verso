// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"fmt"
	"time"
)

func formatUptime(sec int64) string {
	if sec < 0 {
		sec = 0
	}
	d := time.Duration(sec) * time.Second
	days := int64(d / (24 * time.Hour))
	h := int64(d/time.Hour) % 24
	m := int64(d/time.Minute) % 60
	switch {
	case days > 0:
		return fmt.Sprintf("%dd %dh %dm", days, h, m)
	case h > 0:
		return fmt.Sprintf("%dh %dm", h, m)
	default:
		return fmt.Sprintf("%dm", m)
	}
}

// formatLoad converts a kernel load value (fixed-point, scaled by 65536) to a
// human decimal.
func formatLoad(raw int64) string {
	return fmt.Sprintf("%.2f", float64(raw)/65536.0)
}
