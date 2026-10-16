// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import "net/http"

// descriptionsCookie keeps whether this browser folded the settings'
// descriptions away (form-fields). The page writes it (verso-page.js) and the
// shell only reads it, so a page arrives folded from its first paint.
const descriptionsCookie = "verso_descriptions"

// descriptionsFolded reads the request's fold as untrusted input: only the
// one word folds, and anything else is the default, descriptions in view.
func descriptionsFolded(r *http.Request) bool {
	cookie, err := r.Cookie(descriptionsCookie)
	return err == nil && cookie.Value == "folded"
}
