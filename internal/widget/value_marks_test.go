// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"strings"
	"testing"
)

// marked reports whether the text in got stands inside an element carrying
// attr: the nearest such opening tag before it, with no cell, fact or item
// closed between them.
func marked(got, text, attr string) bool {
	at := strings.Index(got, ">"+text+"<")
	if at < 0 {
		return false
	}
	seg := got[max(0, at-900):at]
	open := strings.LastIndex(seg, attr)
	if open < 0 {
		return false
	}
	tail := seg[open:]
	for _, closer := range []string{"</td>", "</dd>", "</li>", "</th>"} {
		if strings.Contains(tail, closer) {
			return false
		}
	}
	return true
}

// regexpChip reports whether the chip whose words are label carries the
// verbatim mark (want) or not.
func regexpChip(got, label string, want bool) bool {
	at := strings.Index(got, label+"</span>")
	if at < 0 {
		return false
	}
	open := strings.LastIndex(got[:at], "<span")
	for open > 0 && !strings.Contains(got[open:at], "verso-chip") {
		open = strings.LastIndex(got[:open], "<span")
	}
	if open < 0 {
		return false
	}
	tag := got[open : open+strings.Index(got[open:], ">")]
	return strings.Contains(tag, "data-verso-verbatim") == want
}

// TestAFactsFaceFollowsWhatItIs: the shell marks a fact's value by what it is
// (ADR-025): a string the machine wrote is verbatim, which its role sets in
// mono; a duration or a rate is figures, set tabular, and never verbatim, in
// every style a fact list takes.
func TestAFactsFaceFollowsWhatItIs(t *testing.T) {
	uptime := int64(8040)
	for _, style := range []struct{ name, style, align string }{
		{"divided", "", ""}, {"plain", "plain", ""}, {"left", "", "left"},
		{"identity", "identity", ""}, {"system", "system", ""},
	} {
		t.Run(style.name, func(t *testing.T) {
			got := render(t, newRenderer(t), &Properties{Style: style.style, Align: style.align, Items: []Property{
				{Label: "Address", Value: "192.0.2.7", Mono: true},
				{Label: "Uptime", Duration: &uptime},
			}})
			if !marked(got, "192.0.2.7", "data-verso-verbatim") {
				t.Errorf("the address is not marked verbatim:\n%s", got)
			}
			duration := strings.Trim(written("2", "h", "14", "min"), "<>")
			if !marked(got, duration, "data-verso-figures") {
				t.Errorf("the uptime is not marked as figures:\n%s", got)
			}
			if marked(got, duration, "data-verso-verbatim") {
				t.Errorf("the uptime is marked verbatim, which a duration never is:\n%s", got)
			}
		})
	}
}

// TestACellsFaceFollowsWhatItIs: a cell is marked by its column's kind — a
// machine string (mono, keyword) verbatim, a count or a time (num, runtime)
// figures — so the cell's role draws it.
func TestACellsFaceFollowsWhatItIs(t *testing.T) {
	lease := int64(600)
	expires := strings.Trim(written("10", "min"), "<>")
	got := render(t, newRenderer(t), &Table{
		Columns: []TableColumn{{Label: "Address", Kind: "mono"}, {Label: "Type", Kind: "keyword"}, {Label: "Expires in", Kind: "runtime"}, {Label: "Hits", Kind: "num"}},
		Rows:    []TableRow{{ID: "a", Cells: []TableCell{{Text: "192.0.2.7"}, {Text: "dhcp"}, {Duration: &lease}, {Text: "42"}}}},
	})
	for _, c := range []struct{ text, attr string }{
		{"192.0.2.7", "data-verso-verbatim"},
		{"dhcp", "data-verso-verbatim"},
		{expires, "data-verso-figures"},
		{"42", "data-verso-figures"},
	} {
		if !marked(got, c.text, c.attr) {
			t.Errorf("%q is not marked %s:\n%s", c.text, c.attr, got)
		}
	}
	chips := render(t, newRenderer(t), &Table{
		Columns: []TableColumn{{Label: "Name", Kind: "name"}, {Label: "Tags", Kind: "entity"}},
		Rows: []TableRow{{ID: "a", Cells: []TableCell{{Text: "laptop"}, {Chips: []TableChip{
			{Label: "lan"}, {Label: "reserved", LocalizeLabel: true},
		}}}}},
	})
	if !strings.Contains(chips, "data-verso-verbatim") || !regexpChip(chips, "lan", true) || !regexpChip(chips, "reserved", false) {
		t.Errorf("a config value's chip is verbatim and a word's is not:\n%s", chips)
	}
	for _, figure := range []string{expires, "42"} {
		if marked(got, figure, "data-verso-verbatim") {
			t.Errorf("%q is marked verbatim, which a figure never is", figure)
		}
	}
}
