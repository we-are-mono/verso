// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package sysstat

// DeviceTraffic is the presentation shape used by device cards. Per-device
// attribution is currently unavailable, so production values remain zero.
type DeviceTraffic struct {
	TxBytes int64
	RxBytes int64
	Conns   int
}
