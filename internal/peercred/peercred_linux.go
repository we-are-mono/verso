// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

//go:build linux

package peercred

import (
	"net"
	"syscall"
)

// UID is the uid the kernel recorded for the process that listens behind a
// connected Unix socket (SO_PEERCRED): fixed when that process called
// listen(2), so no later write, rename or file mode can alter it.
func UID(conn net.Conn) (uint32, error) {
	unixConn, ok := conn.(*net.UnixConn)
	if !ok {
		return 0, errNotUnix
	}
	raw, err := unixConn.SyscallConn()
	if err != nil {
		return 0, err
	}
	var credentials *syscall.Ucred
	var credentialsErr error
	if err := raw.Control(func(fd uintptr) {
		credentials, credentialsErr = syscall.GetsockoptUcred(int(fd), syscall.SOL_SOCKET, syscall.SO_PEERCRED)
	}); err != nil {
		return 0, err
	}
	if credentialsErr != nil {
		return 0, credentialsErr
	}
	return credentials.Uid, nil
}
