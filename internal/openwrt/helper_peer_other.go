// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

//go:build !linux

package openwrt

import (
	"errors"
	"net"
)

func socketPeerUID(net.Conn) (uint32, error) {
	return 0, errors.New("openwrt: socket peer credentials are only available on Linux")
}
