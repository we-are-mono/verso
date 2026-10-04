// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import "fmt"

// formatLoad converts a kernel load value (fixed-point, scaled by 65536) to a
// human decimal.
func formatLoad(raw int64) string {
	return fmt.Sprintf("%.2f", float64(raw)/65536.0)
}
