// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/we-are-mono/verso/internal/widget"
)

// statusTable builds the system-status table from live backend data. It degrades
// to "unavailable" rows rather than failing when the backend can't be reached,
// so the shell never 500s on a backend hiccup; the underlying error is logged.
func (s *Server) statusTable(ctx context.Context) *widget.Table {
	rows := make([][]string, 0, 4)

	if hn, err := s.backend.Hostname(ctx); err == nil {
		rows = append(rows, []string{"Hostname", hn})
	} else {
		log.Printf("verso: hostname unavailable: %v", err)
		rows = append(rows, []string{"Hostname", "unavailable"})
	}

	if si, err := s.backend.SystemInfo(ctx); err == nil {
		rows = append(rows,
			[]string{"Uptime", formatUptime(si.Uptime)},
			[]string{"Load (1m)", formatLoad(si.Load[0])},
			[]string{"Memory", fmt.Sprintf("%s free of %s",
				formatBytes(si.Memory.Available), formatBytes(si.Memory.Total))},
		)
	} else {
		log.Printf("verso: system info unavailable: %v", err)
		rows = append(rows, []string{"System", "unavailable"})
	}

	return &widget.Table{Columns: []string{"Field", "Value"}, Rows: rows}
}

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

func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(b)/float64(div), "KMGTPE"[exp])
}
