// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package i18n

import "fmt"

// Count says n of something as a whole translated phrase, in the plural form
// n takes: one, two (Slovenian's dual), few (its paucal, 3 and 4) or many, by
// n's last two digits. The value stays an argument; no sentence is assembled
// from suffixes. The two and few keys carry a context marker ("%d tunnels
// (two)") a catalog translates; missing, they fall back to the English one or
// many, and the marker is never shown.
func Count(tr func(string) string, n int, one, two, few, many string) string {
	key := many
	switch n % 100 {
	case 1:
		key = one
	case 2:
		key = two
	case 3, 4:
		key = few
	}
	pattern := tr(key)
	if pattern == key {
		pattern = many
		if n == 1 {
			pattern = one
		}
	}
	return fmt.Sprintf(pattern, n)
}
