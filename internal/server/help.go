// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import "net/http"

// helpCookie keeps whether this browser shows the settings' help
// (form-fields). The page writes it (verso-page.js) and the shell only reads
// it, so a page arrives with help shown from its first paint.
const helpCookie = "verso_help"

// helpShown reads the request's help as untrusted input: only the one word
// shows it, and anything else is the default, help hidden.
func helpShown(r *http.Request) bool {
	cookie, err := r.Cookie(helpCookie)
	return err == nil && cookie.Value == "shown"
}
