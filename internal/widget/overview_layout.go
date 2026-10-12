// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"slices"
	"strings"
)

// OverviewLayout is where the Overview's cards stand: two columns of card ids
// in order, the cards closed among them, and the cards hidden from both. A
// hidden card holds no place in a column.
type OverviewLayout struct {
	Cols   [2][]string
	Closed []string
	Hidden []string
}

// overviewLayoutVersion leads the encoded layout, so a value written in
// another shape reads as no layout at all.
const overviewLayoutVersion = "1"

// overviewDefault is the layout a browser starts with: what the uplink is
// doing on the left, what the router is doing on the right.
var overviewDefault = [2][]string{
	{"internet", "firewall", "ports", "tunnels"},
	{"traffic", "resources", "hardware", "firmware"},
}

// overviewCardIDs are every card the Overview has, in default order.
func overviewCardIDs() []string {
	return slices.Concat(overviewDefault[0], overviewDefault[1])
}

// DefaultOverviewLayout is the layout with every card shown and open.
func DefaultOverviewLayout() OverviewLayout {
	return OverviewLayout{Cols: [2][]string{slices.Clone(overviewDefault[0]), slices.Clone(overviewDefault[1])}}
}

// ParseOverviewLayout reads an encoded layout (Encode) from untrusted input.
// A value it cannot read is the default layout. Unknown cards are dropped, a
// card named twice keeps its first place, a closed card no column holds is
// forgotten, and every card the value leaves out comes back open at the end of
// the column with fewer cards.
func ParseOverviewLayout(value string) OverviewLayout {
	fields := strings.Split(value, ".")
	if len(fields) != 5 || fields[0] != overviewLayoutVersion {
		return DefaultOverviewLayout()
	}
	lists := make([][]string, 4)
	for i, field := range fields[1:] {
		ids, ok := layoutIDs(field)
		if !ok {
			return DefaultOverviewLayout()
		}
		lists[i] = ids
	}
	known := overviewCardIDs()
	seen := map[string]bool{}
	take := func(ids []string) []string {
		out := []string{}
		for _, id := range ids {
			if slices.Contains(known, id) && !seen[id] {
				seen[id] = true
				out = append(out, id)
			}
		}
		return out
	}
	var l OverviewLayout
	l.Hidden = take(lists[3])
	l.Cols = [2][]string{take(lists[0]), take(lists[1])}
	placed := slices.Concat(l.Cols[0], l.Cols[1])
	for _, id := range lists[2] {
		if slices.Contains(placed, id) && !slices.Contains(l.Closed, id) {
			l.Closed = append(l.Closed, id)
		}
	}
	for _, id := range known {
		if !seen[id] {
			l.Cols[l.shorter()] = append(l.Cols[l.shorter()], id)
		}
	}
	return l
}

// layoutIDs splits one field into card ids; any id outside a-z spoils it.
func layoutIDs(field string) ([]string, bool) {
	if field == "" {
		return nil, true
	}
	ids := strings.Split(field, "-")
	for _, id := range ids {
		if id == "" || strings.IndexFunc(id, func(r rune) bool { return r < 'a' || r > 'z' }) >= 0 {
			return nil, false
		}
	}
	return ids, true
}

// shorter is the column holding fewer cards, the left on a tie.
func (l OverviewLayout) shorter() int {
	if len(l.Cols[1]) < len(l.Cols[0]) {
		return 1
	}
	return 0
}

// Encode writes the layout as the cookie carries it:
// 1.<left>.<right>.<closed>.<hidden>, each field card ids joined by "-".
func (l OverviewLayout) Encode() string {
	return strings.Join([]string{overviewLayoutVersion,
		strings.Join(l.Cols[0], "-"), strings.Join(l.Cols[1], "-"),
		strings.Join(l.Closed, "-"), strings.Join(l.Hidden, "-")}, ".")
}

// IsClosed is whether the card shows only its header.
func (l OverviewLayout) IsClosed(id string) bool { return slices.Contains(l.Closed, id) }

// IsHidden is whether the card is off the page.
func (l OverviewLayout) IsHidden(id string) bool { return slices.Contains(l.Hidden, id) }

// Shows is whether the card is on the page, open or closed.
func (l OverviewLayout) Shows(id string) bool { return !l.IsHidden(id) }
