// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"encoding/base64"
	"encoding/json"
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
// another shape reads as no layout at all. Version 2 is base64url JSON, which
// names a plugin's card by its contribution's full id (ADR-026); version 1,
// shell cards alone joined by "-", is still read.
const (
	overviewLayoutVersion   = "2"
	overviewLayoutVersionV1 = "1"
)

// maxOverviewLayout bounds the encoded layout a browser may send.
const maxOverviewLayout = 3072

// OverviewPlacement is a plugin's Overview card (ADR-026): its id,
// <plugin id>.<contribution id>, and where it stands in the default layout,
// "first", "last" (or empty), "before" or "after" its Anchor, a card's id.
type OverviewPlacement struct {
	ID, Where, Anchor string
}

// overviewDefault is the layout a browser starts with: what the uplink is
// doing on the left, what the router is doing on the right.
var overviewDefault = [2][]string{
	{"internet", "ports", "tunnels"},
	{"traffic", "resources", "system", "hardware"},
}

// overviewMerged names the card each retired card lives on in, so a layout a
// browser saved before the merge keeps its card where it stood.
var overviewMerged = map[string]string{"firmware": "system"}

// overviewCardIDs are every card the Overview has, in default order.
func overviewCardIDs() []string {
	return slices.Concat(overviewDefault[0], overviewDefault[1])
}

// DefaultOverviewLayout is the layout with every card shown and open: the
// shell's in their default places, and each plugin card where its placement
// puts it, at the end of the column with fewer cards when its anchor is not
// there.
func DefaultOverviewLayout(plugins ...OverviewPlacement) OverviewLayout {
	l := OverviewLayout{Cols: [2][]string{slices.Clone(overviewDefault[0]), slices.Clone(overviewDefault[1])}}
	for _, p := range plugins {
		l.place(p)
	}
	return l
}

// place puts a plugin card where its placement says.
func (l *OverviewLayout) place(p OverviewPlacement) {
	if p.Where == "first" {
		l.Cols[0] = slices.Insert(l.Cols[0], 0, p.ID)
		return
	}
	if p.Where == "before" || p.Where == "after" {
		for c := range l.Cols {
			if i := slices.Index(l.Cols[c], p.Anchor); i >= 0 {
				if p.Where == "after" {
					i++
				}
				l.Cols[c] = slices.Insert(l.Cols[c], i, p.ID)
				return
			}
		}
	}
	l.Cols[l.shorter()] = append(l.Cols[l.shorter()], p.ID)
}

// ParseOverviewLayout reads an encoded layout (Encode) from untrusted input,
// knowing the shell's cards and the installed plugins' (plugins). A value it
// cannot read is the default layout. A merged card reads as the card it
// lives on in (overviewMerged), unknown cards are dropped, a card named twice
// keeps its first place, a closed card no column holds is forgotten, and
// every card the value leaves out comes back open at the end of the column
// with fewer cards.
func ParseOverviewLayout(value string, plugins ...OverviewPlacement) OverviewLayout {
	lists, ok := decodeOverviewLayout(value)
	if !ok {
		return DefaultOverviewLayout(plugins...)
	}
	known := overviewCardIDs()
	for _, p := range plugins {
		known = append(known, p.ID)
	}
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

// encodedLayout is version 2's JSON.
type encodedLayout struct {
	Cols   [][]string `json:"cols"`
	Closed []string   `json:"closed"`
	Hidden []string   `json:"hidden"`
}

// decodeOverviewLayout reads either version into its four lists: left,
// right, closed, hidden. Any id that is no card's spoils the whole value.
func decodeOverviewLayout(value string) ([][]string, bool) {
	version, rest, _ := strings.Cut(value, ".")
	switch {
	case len(value) > maxOverviewLayout:
		return nil, false
	case version == overviewLayoutVersionV1:
		fields := strings.Split(rest, ".")
		if len(fields) != 4 {
			return nil, false
		}
		lists := make([][]string, 4)
		for i, field := range fields {
			if field != "" {
				lists[i] = strings.Split(field, "-")
			}
		}
		return mergedIDs(lists, func(id string) bool {
			return strings.IndexFunc(id, func(r rune) bool { return r < 'a' || r > 'z' }) < 0
		})
	case version == overviewLayoutVersion:
		data, err := base64.RawURLEncoding.DecodeString(rest)
		var e encodedLayout
		if err != nil || json.Unmarshal(data, &e) != nil || len(e.Cols) != 2 {
			return nil, false
		}
		return mergedIDs([][]string{e.Cols[0], e.Cols[1], e.Closed, e.Hidden}, overviewCardID)
	}
	return nil, false
}

// mergedIDs checks every id against valid and reads a merged card as the
// card it lives on in.
func mergedIDs(lists [][]string, valid func(string) bool) ([][]string, bool) {
	for _, ids := range lists {
		for i, id := range ids {
			if id == "" || !valid(id) {
				return nil, false
			}
			if merged, ok := overviewMerged[id]; ok {
				ids[i] = merged
			}
		}
	}
	return lists, true
}

// overviewCardID is a shell card's id (a-z) or a plugin card's,
// <plugin id>.<contribution id>.
func overviewCardID(id string) bool {
	letters := func(s string) bool { return strings.IndexFunc(s, func(r rune) bool { return r < 'a' || r > 'z' }) < 0 }
	word := func(s string) bool {
		return s != "" && strings.IndexFunc(s, func(r rune) bool {
			return (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' && r != '_'
		}) < 0
	}
	owner, card, plugin := strings.Cut(id, ".")
	if !plugin {
		return letters(id)
	}
	return word(owner) && word(card)
}

// shorter is the column holding fewer cards, the left on a tie.
func (l OverviewLayout) shorter() int {
	if len(l.Cols[1]) < len(l.Cols[0]) {
		return 1
	}
	return 0
}

// Encode writes the layout as the cookie carries it: 2.<base64url JSON of
// {cols, closed, hidden}>, as the page writes it.
func (l OverviewLayout) Encode() string {
	data, _ := json.Marshal(encodedLayout{
		Cols:   [][]string{nonNil(l.Cols[0]), nonNil(l.Cols[1])},
		Closed: nonNil(l.Closed), Hidden: nonNil(l.Hidden),
	})
	return overviewLayoutVersion + "." + base64.RawURLEncoding.EncodeToString(data)
}

// nonNil is ids, an empty list where there are none.
func nonNil(ids []string) []string {
	if ids == nil {
		return []string{}
	}
	return ids
}

// IsClosed is whether the card shows only its header.
func (l OverviewLayout) IsClosed(id string) bool { return slices.Contains(l.Closed, id) }

// IsHidden is whether the card is off the page.
func (l OverviewLayout) IsHidden(id string) bool { return slices.Contains(l.Hidden, id) }

// Without is the layout with a card this router does not have taken out of
// its column, the closed cards and the shelf; every other card keeps its place.
func (l OverviewLayout) Without(id string) OverviewLayout {
	drop := func(ids []string) []string {
		return slices.DeleteFunc(slices.Clone(ids), func(s string) bool { return s == id })
	}
	return OverviewLayout{Cols: [2][]string{drop(l.Cols[0]), drop(l.Cols[1])}, Closed: drop(l.Closed), Hidden: drop(l.Hidden)}
}

// Shows is whether the card is on the page, open or closed.
func (l OverviewLayout) Shows(id string) bool { return !l.IsHidden(id) }
