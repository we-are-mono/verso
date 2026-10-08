// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

// Package clock is the router's own time zone. OpenWrt keeps it as a POSIX
// rule in /etc/TZ ("CET-1CEST,M3.5.0,M10.5.0/3") and ships no zone database,
// so Go's time.Local is UTC there. The rule is the trailing rule of a TZif
// file (RFC 8536 §3.3), so a file of no transitions and that rule is a
// Location Go follows exactly, daylight saving included, with no zone data
// carried in the binary.
package clock

import (
	"bytes"
	"encoding/binary"
	"errors"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
)

// posixTZ is a POSIX TZ rule: a name and offset, and optionally a summer
// name, offset and the two dates it runs between.
var posixTZ = regexp.MustCompile(`^` +
	`(?:<[+-]?[A-Za-z0-9]+>|[A-Za-z]{3,})[+-]?\d{1,2}(?::\d{2}){0,2}` +
	`(?:(?:<[+-]?[A-Za-z0-9]+>|[A-Za-z]{3,})(?:[+-]?\d{1,2}(?::\d{2}){0,2})?` +
	`(?:,(?:M\d{1,2}\.\d\.\d|J?\d{1,3})(?:/[+-]?\d{1,3}(?::\d{2}){0,2})?` +
	`,(?:M\d{1,2}\.\d\.\d|J?\d{1,3})(?:/[+-]?\d{1,3}(?::\d{2}){0,2})?)?)?$`)

var bareName = regexp.MustCompile(`^[A-Za-z]{3,}$`)

// Location is the zone a POSIX TZ rule describes.
func Location(rule string) (*time.Location, error) {
	rule = strings.TrimSpace(rule)
	if bareName.MatchString(rule) {
		rule += "0" // "UTC" as some images write it: the zero offset it means
	}
	if !posixTZ.MatchString(rule) {
		return nil, errors.New("clock: not a POSIX TZ rule")
	}
	var tzif bytes.Buffer
	// A version 2 file: an empty version 1 block, the same with one
	// placeholder type in the 64-bit block, and the rule as its footer.
	header := func(typecnt, charcnt uint32) {
		tzif.WriteString("TZif2")
		tzif.Write(make([]byte, 15))
		for _, n := range []uint32{0, 0, 0, 0, typecnt, charcnt} {
			_ = binary.Write(&tzif, binary.BigEndian, n)
		}
	}
	block := func() {
		tzif.Write([]byte{0, 0, 0, 0, 0, 0}) // utoff 0, not DST, abbreviation 0
		tzif.WriteByte(0)                    // the empty abbreviation
	}
	header(1, 1)
	block()
	header(1, 1)
	block()
	tzif.WriteString("\n" + rule + "\n")
	return time.LoadLocationFromTZData(rule, tzif.Bytes())
}

// zone is the zone as /etc/TZ last said it, read again once its file
// changes.
type zone struct {
	path  string
	every time.Duration

	mu      sync.Mutex
	checked time.Time
	rule    string
	loc     *time.Location
}

var router = &zone{path: "/etc/TZ", every: 10 * time.Second}

// get is the zone, its file read again at most every so often. A file that is
// missing or holds no rule leaves the last good zone, or the host's own.
func (z *zone) get() *time.Location {
	z.mu.Lock()
	defer z.mu.Unlock()
	if z.loc != nil && time.Since(z.checked) < z.every {
		return z.loc
	}
	z.checked = time.Now()
	if rule, err := os.ReadFile(z.path); err == nil && string(rule) != z.rule {
		if loc, err := Location(string(rule)); err == nil {
			z.rule, z.loc = string(rule), loc
		}
	}
	if z.loc == nil {
		z.loc = time.Local // a host that is not an OpenWrt router
	}
	return z.loc
}

// Zone is the router's time zone.
func Zone() *time.Location { return router.get() }

// Local is t in the router's time zone.
func Local(t time.Time) time.Time { return t.In(Zone()) }

// Now is the router's local time.
func Now() time.Time { return Local(time.Now()) }
