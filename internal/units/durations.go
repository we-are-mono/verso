// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package units

import (
	"fmt"
	"math"
	"strconv"
)

// durationUnits are the units a stretch of time counts in, largest first. The
// symbols (s, min, h, d) read the same in every language the shell speaks, as
// Mb/s and MiB do.
var durationUnits = []struct {
	symbol  string
	seconds int64
}{{"d", 86400}, {"h", 3600}, {"min", 60}, {"s", 1}}

// Duration writes how long something lasted or has lasted: seconds under a
// minute, minutes under an hour, and from there its two largest units
// ("2 h 14 min", "52 d 8 h"), the second left out when it is nothing. A
// negative stretch reads as none.
func Duration(seconds int64) string { return span(seconds, 2, math.MaxInt64) }

// DurationWithin is Duration counted in no unit larger than the largest a step
// of step seconds reaches, so an axis stepping in hours reads "24 h", not
// "1 d", and one stepping in seconds "60 s".
func DurationWithin(seconds, step int64) string { return span(seconds, 2, step) }

// Since writes how long ago something was in its largest unit alone ("2 h"),
// as a time ago is said.
func Since(seconds int64) string { return span(seconds, 1, math.MaxInt64) }

func span(seconds int64, units int, step int64) string {
	seconds = max(seconds, 0)
	for i, u := range durationUnits {
		if (seconds < u.seconds || u.seconds > step) && i < len(durationUnits)-1 {
			continue
		}
		out := strconv.FormatInt(seconds/u.seconds, 10) + Space + u.symbol
		// Seconds never trail minutes: a span under an hour reads in minutes,
		// so a live one does not change every second.
		if units > 1 && i < len(durationUnits)-2 {
			next := durationUnits[i+1]
			if rest := seconds % u.seconds / next.seconds; rest > 0 {
				out += " " + strconv.FormatInt(rest, 10) + Space + next.symbol
			}
		}
		return out
	}
	return ""
}

// Ago writes how long ago something was, in the language's pattern for it
// ("2 h ago", "pred 2 h"); under a minute ago, and any time ahead, is "just
// now".
func (n Numbers) Ago(seconds int64) string {
	if seconds < 60 {
		return n.tr("just now")
	}
	return fmt.Sprintf(n.tr("%s ago"), Since(seconds))
}
