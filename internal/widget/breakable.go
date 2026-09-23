// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"html/template"
	"strings"
)

// breakSeparators are the characters a machine string divides itself by, in
// the order they outrank each other: a forwarding rule's slashes before the
// dots of the domain inside it, a hostname's dots, an IPv6 address's colons.
const breakSeparators = "/.:"

// Breakable sets a machine string so that, when it has to wrap, it wraps where
// it divides itself: after each occurrence of the first of its separators it
// has (see breakSeparators), and nowhere else unless one part alone is wider
// than its box (the caller's wrap-anywhere). The string is escaped; only the
// <wbr> break points are markup. verso-forms.js versoBreakable is the same
// rule for a value added in the browser.
func Breakable(s string) template.HTML {
	mark := ""
	for _, sep := range breakSeparators {
		if strings.ContainsRune(s, sep) {
			mark = string(sep)
			break
		}
	}
	if mark == "" {
		return template.HTML(template.HTMLEscapeString(s))
	}
	parts := strings.SplitAfter(s, mark)
	var b strings.Builder
	for i, part := range parts {
		if i > 0 && part != "" {
			b.WriteString("<wbr>")
		}
		b.WriteString(template.HTMLEscapeString(part))
	}
	return template.HTML(b.String())
}
