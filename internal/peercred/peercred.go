// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

// Package peercred reads which account is on the other end of a connected Unix
// socket, as the kernel recorded it. A socket's path proves nothing about who
// answers there: whoever binds the path while it is free does. The kernel's
// record does, so a client checks it before it writes a byte.
package peercred

import "errors"

// errNotUnix marks a connection the kernel keeps no peer credentials for.
var errNotUnix = errors.New("peercred: not a unix socket")
