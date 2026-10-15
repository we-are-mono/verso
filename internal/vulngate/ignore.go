// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package vulngate

import (
	"bufio"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"
	"unicode"
)

const dateLayout = "2006-01-02"

// MaxTriageDays is how far past the day the list is read an entry may lapse,
// so every triage is revisited and none silences an advisory for good.
const MaxTriageDays = 180

// advisoryID is the shape of every identifier the scanners report: GO-,
// RUSTSEC-, GHSA-, CVE- and the like, upper-case prefix and all.
var advisoryID = regexp.MustCompile(`^[A-Z][A-Z0-9]*-[0-9A-Za-z-]+$`)

// Ignore is one triaged advisory: which one, until when the triage holds, and
// why it does not stop a release.
type Ignore struct {
	ID     string
	Until  time.Time // the last day the entry holds, at midnight UTC
	Reason string
	Line   int
}

// Active reports whether the entry still holds at now. It holds through the
// whole of its date.
func (ig Ignore) Active(now time.Time) bool {
	return now.UTC().Before(ig.Until.AddDate(0, 0, 1))
}

// ParseIgnores reads the ignore list: one entry per line, the advisory, the
// date the triage lapses (YYYY-MM-DD), and the reason; '#' starts a comment
// line. An entry without a date or a reason is refused, as is a repeated one,
// and one that lapses more than MaxTriageDays after now.
func ParseIgnores(r io.Reader, now time.Time) ([]Ignore, error) {
	y, m, d := now.UTC().Date()
	latest := time.Date(y, m, d+MaxTriageDays, 0, 0, 0, 0, time.UTC)
	var out []Ignore
	seen := map[string]int{}
	sc := bufio.NewScanner(r)
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		id, rest := cutField(line)
		date, reason := cutField(rest)
		if !advisoryID.MatchString(id) {
			return nil, fmt.Errorf("line %d: %q is not an advisory identifier", n, id)
		}
		until, err := time.Parse(dateLayout, date)
		if err != nil {
			return nil, fmt.Errorf("line %d: %s needs the date its triage lapses, as YYYY-MM-DD", n, id)
		}
		if until.After(latest) {
			return nil, fmt.Errorf("line %d: %s lapses %s, past %s; a triage holds at most %d days",
				n, id, date, latest.Format(dateLayout), MaxTriageDays)
		}
		if reason == "" {
			return nil, fmt.Errorf("line %d: %s needs the reason it does not stop a release", n, id)
		}
		if first, dup := seen[id]; dup {
			return nil, fmt.Errorf("line %d: %s is already listed on line %d", n, id, first)
		}
		seen[id] = n
		out = append(out, Ignore{ID: id, Until: until, Reason: reason, Line: n})
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// cutField splits s at its first run of white space: the leading field, and
// the rest with its own spacing kept.
func cutField(s string) (field, rest string) {
	s = strings.TrimSpace(s)
	i := strings.IndexFunc(s, unicode.IsSpace)
	if i < 0 {
		return s, ""
	}
	return s[:i], strings.TrimSpace(s[i:])
}
