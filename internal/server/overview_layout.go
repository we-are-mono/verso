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
// layout's interim home until it is kept on the router per user. hardware is
// whether this board's profile lists sensors; without, there is no Hardware card.
type cookieLayout struct{ hardware bool }

// Layout reads the request's layout cookie as untrusted input; with none, or
// one it cannot read, the request gets the default layout.
func (c cookieLayout) Layout(r *http.Request) widget.OverviewLayout {
	return c.LayoutWithPlugins(r, nil)
}

func (c cookieLayout) LayoutWithPlugins(r *http.Request, pluginIDs []string) widget.OverviewLayout {
	l := widget.DefaultOverviewLayoutWithPlugins(pluginIDs)
	if cookie, err := r.Cookie(overviewLayoutCookie); err == nil {
		l = widget.ParseOverviewLayoutWithPlugins(cookie.Value, pluginIDs)
	}
	if !c.hardware {
		l = l.Without("hardware")
	}
	return l
}
