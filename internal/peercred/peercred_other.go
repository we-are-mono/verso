// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

//go:build !linux

package peercred

import (
	"errors"
	"net"
)

// UID fails closed where the kernel offers no SO_PEERCRED: no account is
// reported, so no caller takes a listener for the one it expects.
func UID(net.Conn) (uint32, error) {
	return 0, errors.New("peercred: socket peer credentials are only available on Linux")
}
