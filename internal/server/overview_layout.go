// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"net/http"

	"github.com/we-are-mono/verso/internal/widget"
)

// overviewLayoutCookie carries the Overview's card layout. The page writes it
// (verso-overview.js) and the shell only reads it.
const overviewLayoutCookie = "verso_overview"

// layoutSource is where the Overview's card layout comes from for a request.
type layoutSource interface {
	Layout(*http.Request) widget.OverviewLayout
}

// cookieLayout keeps the layout per browser, in a cookie (ADR-022). It is the
// layout's interim home until it is kept on the router per user.
type cookieLayout struct{}

// Layout reads the request's layout cookie as untrusted input; with none, or
// one it cannot read, the request gets the default layout.
func (cookieLayout) Layout(r *http.Request) widget.OverviewLayout {
	cookie, err := r.Cookie(overviewLayoutCookie)
	if err != nil {
		return widget.DefaultOverviewLayout()
	}
	return widget.ParseOverviewLayout(cookie.Value)
}
