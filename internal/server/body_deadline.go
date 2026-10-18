// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"net/http"
	"time"
)

// The listeners bound a connection's handshake, headers and idle wait (ADR-017
// §1) but not a body, so a firmware upload runs its course. A client that has
// not signed in sends no such body, so its body has a deadline of its own: one
// that trickles a body to /login, or to any other path, cannot hold its
// connection inside the per-source cap until the body ends.

// signInBodyWait is how long a request without a session has to send its
// body, the time a connection has for its headers.
const signInBodyWait = 10 * time.Second

// bodyDeadline is next with the body of every request signedIn does not
// vouch for read by d from its start. The deadline is the connection's read
// deadline, which net/http lifts once the body is read whole, so the handler
// then takes as long as it takes.
func bodyDeadline(d time.Duration, signedIn func(*http.Request) bool, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if d > 0 && r.Body != nil && r.Body != http.NoBody && !signedIn(r) {
			_ = http.NewResponseController(w).SetReadDeadline(time.Now().Add(d))
		}
		next.ServeHTTP(w, r)
	})
}
