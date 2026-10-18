// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"strings"
	"testing"
)

// TestFactSheetRowCarriesItsMarkAndNote: on the ordinary fact sheet a row's
// state mark hangs in the gutter before its value — centred on the value's
// first line, so a marked value starts where every other row's does — and its
// note sits beneath the value it explains, in the value's column.
func TestFactSheetRowCarriesItsMarkAndNote(t *testing.T) {
	got := render(t, newRenderer(t), &Properties{Align: "left", Items: []Property{
		{Label: "Signed by", Value: "The router itself", Dot: "warning", Help: "Browsers warn until you trust it."},
	}})
	for _, want := range []string{
		`<dl class="verso-facts" data-verso-style="divided" data-verso-align="left">`,
		`<span class="verso-mark verso-facts__mark" data-verso-tone="warning" aria-hidden="true"></span>`,
		`<dd class="verso-facts__value">`,
		`<p class="verso-facts__help">Browsers warn until you trust it.</p>`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("want %s in:\n%s", want, got)
		}
	}
	// centred on the value's 24px first line: 9px over the 6px mark, 9px
	// under, in the gutter 15px before the value
	declares(t, "facts", `.verso-facts__mark:where(.verso-facts[data-verso-align="left"] *)`,
		"position: absolute;", "top: calc(var(--spacing) * 2.25);", "left: calc(var(--spacing) * -3.75);")
	declares(t, "facts", ".verso-facts__value", "display: flex;", "@apply type-value;")
	// the note writes on the row's next 24px line, in the value's column
	declares(t, "facts", ".verso-facts__help", "flex-basis: 100%;", "color: var(--color-body);", "@apply type-body;")
	declares(t, "facts", `.verso-facts__help:where(.verso-facts[data-verso-align="left"] *)`, "padding-left: calc(var(--spacing) * 30);")
}

// TestLeftFactsStackOnAPhone: a document's facts read beside a fixed label
// column where there is room for one; on a phone the label sits over its value,
// which then takes the card's whole width — a fingerprint in a 150px column is
// seven lines of hex — straight on, the label's line then the value's, so the
// stack stays on the grid's lines. A long value balances its lines rather than
// leaving one short tail.
func TestLeftFactsStackOnAPhone(t *testing.T) {
	got := render(t, newRenderer(t), &Properties{Align: "left", Items: []Property{
		{Label: "Fingerprint", Value: "F5 3C B1", Mono: true},
	}})
	for _, want := range []string{
		`data-verso-align="left"`,
		`<dt class="verso-facts__label">`,
		"text-balance",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("want %s in:\n%s", want, got)
		}
	}
	css := blockCSS(t, "facts")
	stack := `.verso-facts__row:where(.verso-facts[data-verso-align="left"] *)`
	if rule := cssRule(css, stack); !strings.Contains(rule, "flex-direction: column;") {
		t.Errorf("a left-set fact does not stack on a phone: %q", rule)
	}
	if !strings.Contains(css, "@media (width >= 40rem) {\n  "+stack+" {\n    flex-direction: row;") {
		t.Error("a left-set fact does not stand beside its label where there is room")
	}
	if !strings.Contains(css, "@media (width >= 40rem) {\n  .verso-facts__label:where(.verso-facts[data-verso-align=\"left\"] *) {\n    width: calc(var(--spacing) * 24);") {
		t.Error("a left-set label column is not 96px wide where there is room")
	}
	declares(t, "facts", `.verso-facts__value:where(.verso-facts[data-verso-align="left"] *)`, "align-self: stretch;")
	right := render(t, newRenderer(t), &Properties{Items: []Property{{Label: "Uptime", Value: "3 d"}}})
	if strings.Contains(right, "data-verso-align") {
		t.Errorf("a right-edge fact sheet keeps its row on every width:\n%s", right)
	}
}

// TestRowsHangFromTheirFirstLine: a value that wraps keeps its label, its copy
// control and its pill on its first line, not floating at the middle of the
// block. Every part is a 20px line, so top-aligned is line-aligned.
func TestRowsHangFromTheirFirstLine(t *testing.T) {
	got := render(t, newRenderer(t), &Properties{Items: []Property{
		{Label: "Fingerprint", Value: "F5 3C B1", Mono: true, Copy: true, Status: &Badge{Variant: "success", Text: "ok"}},
	}})
	for _, want := range []string{
		`<div class="verso-facts__row">`,
		`<dd class="verso-facts__value">`,
		`inline-flex h-6 shrink-0 items-center`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("want %s in:\n%s", want, got)
		}
	}
	declares(t, "facts", ".verso-facts__row", "align-items: flex-start;", "justify-content: space-between;", "column-gap: calc(var(--spacing) * 6);")
	declares(t, "facts", `.verso-facts__value:where(.verso-facts:is([data-verso-style="divided"], [data-verso-style="plain"]) *)`,
		"position: relative;", "min-width: 0;", "align-items: flex-start;")
}

// TestSpanRowDrawsTheStretchAndWhereNowIs: a fact that is a stretch between two
// ends — a certificate's validity — draws the stretch as the meter's track,
// filled in its state's hue up to where now is. The ends are written under the
// track's ends; a screen reader hears them.
func TestSpanRowDrawsTheStretchAndWhereNowIs(t *testing.T) {
	got := render(t, newRenderer(t), &Properties{Align: "left", Items: []Property{
		{Label: "Valid", Span: &PropertySpan{From: "2026-08-31", To: "2027-10-02", At: 6, Tone: "success"}},
	}})
	for _, want := range []string{
		`<span class="sr-only">2026-08-31 – 2027-10-02</span>`,
		`<span class="verso-facts__track"><span class="verso-facts__groove"></span>`,
		`<span class="verso-facts__fill" data-verso-tone="success" style="width: 6%"></span>`,
		`<span class="verso-facts__ends" data-verso-figures><span>2026-08-31</span><span>2027-10-02</span></span>`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("want %s in:\n%s", want, got)
		}
	}
	// the ruler stands on the row's first line, level with its label
	declares(t, "facts", ".verso-facts__track", "height: calc(var(--spacing) * 6);")
	// the meter's own track, never a hairline: a list of hairline rows must
	// not have one more line in it that means something else
	declares(t, "facts", ".verso-facts__groove,\n.verso-facts__fill", "height: calc(var(--spacing) * 1.5);", "border-radius: calc(infinity * 1px);")
	declares(t, "facts", `.verso-facts__fill[data-verso-tone="success"]`, "background-color: var(--color-green);")
	declares(t, "facts", ".verso-facts__ends", "justify-content: space-between;", "white-space: nowrap;")
	over := render(t, newRenderer(t), &Properties{Items: []Property{
		{Label: "Valid", Span: &PropertySpan{From: "a", To: "b", At: 140, Tone: "danger"}},
	}})
	if !strings.Contains(over, `data-verso-tone="danger" style="width: 100%"`) {
		t.Errorf("the fill never leaves the stretch:\n%s", over)
	}
}

// TestAHeadNamesAGroupOrItsColumns: a list's head stands on the strong
// hairline a column head does, its first fact 7px after it. Over a group of
// facts it is the group's title, a heading, with its protocol as a chip
// beside it (an address family); over two columns it names them, the value's
// name standing over the values (a ledger's Part and Current).
func TestAHeadNamesAGroupOrItsColumns(t *testing.T) {
	group := render(t, newRenderer(t), &Properties{Align: "left", Head: &PropertiesHead{Label: "IPv4", Chip: "dhcp"}, Items: []Property{{Label: "Address", Value: "10.0.0.2", Mono: true}}})
	for _, want := range []string{
		`<section aria-label="IPv4" class="min-w-0">`,
		`<div data-verso-properties-head class="verso-facts-head">`,
		`<h3>IPv4</h3>`,
		">dhcp</span>",
		"data-verso-headed",
	} {
		if !strings.Contains(group, want) {
			t.Errorf("want %s in:\n%s", want, group)
		}
	}
	declares(t, "facts-head", ".verso-facts-head", "padding-block: calc(var(--spacing) * 2);", "border: 0 solid var(--color-rule-strong);", "@apply type-subheading;")
	// A card the list opens may take that air back (--verso-facts-lead).
	declares(t, "facts", `.verso-facts__row:where(.verso-facts[data-verso-style="divided"][data-verso-headed] *):first-child`, "padding-top: var(--verso-facts-lead, calc(var(--spacing) * 1.75));")
	declares(t, "facts", `.verso-facts__row:where(.verso-facts[data-verso-style="divided"] *):first-child`, "padding-top: var(--verso-facts-lead, calc(var(--spacing) * 2));")
	columns := render(t, newRenderer(t), &Properties{Align: "left", Head: &PropertiesHead{Label: "Part", Value: "Current"}, Items: []Property{{Label: "Kernel", Value: "6.12", Mono: true}}})
	for _, want := range []string{
		`<span class="shrink-0 sm:w-24">Part</span><span>Current</span>`,
	} {
		if !strings.Contains(columns, want) {
			t.Errorf("want %s in:\n%s", want, columns)
		}
	}
	if strings.Contains(columns, "<h3") {
		t.Errorf("column names are not a heading:\n%s", columns)
	}
	right := render(t, newRenderer(t), &Properties{Head: &PropertiesHead{Label: "Sensor", Value: "Reading"}, Items: []Property{{Label: "Fan", Value: "0 rpm"}}})
	if !strings.Contains(right, `<span class="ml-auto">Reading</span>`) {
		t.Errorf("over values on the right edge the value's name holds that edge too:\n%s", right)
	}
	bare := render(t, newRenderer(t), &Properties{Items: []Property{{Label: "Fan", Value: "0 rpm"}}})
	if strings.Contains(bare, "data-verso-properties-head") || strings.Contains(bare, "<section") || strings.Contains(bare, "data-verso-headed") {
		t.Errorf("a list with no head is the rows alone, its first keeping 8px:\n%s", bare)
	}
}

// TestAListWithNoFactsSaysSoInTheSlot: with nothing to list, the empty set's
// slot stands where the first fact would, under the head, its frame 7px after
// the head's hairline.
func TestAListWithNoFactsSaysSoInTheSlot(t *testing.T) {
	got := render(t, newRenderer(t), &Properties{Head: &PropertiesHead{Label: "IPv6"}, Empty: "No prefix was delegated."})
	for _, want := range []string{
		`<div class="pt-1.75"><div data-verso-slot`,
		"No prefix was delegated.",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("want %s in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "<dl") {
		t.Errorf("an empty list draws no rows:\n%s", got)
	}
}

// TestALiveFactCarriesItsKey: a row the stream keeps current carries its key
// on its value and on its mark, so the stream rewrites both in place; on the
// right edge the mark leads the value on its line, centred 9px down it.
func TestALiveFactCarriesItsKey(t *testing.T) {
	got := render(t, newRenderer(t), &Properties{Items: []Property{
		{Label: "Link", Value: "Up", Verbatim: true, Dot: "success", Key: "link"},
	}})
	for _, want := range []string{
		`data-verso-prop="link"`,
		`<span class="verso-mark verso-facts__mark" data-verso-tone="success" data-verso-prop-dot="link" aria-hidden="true">`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("want %s in:\n%s", want, got)
		}
	}
	declares(t, "facts", ".verso-facts__mark", "margin-top: calc(var(--spacing) * 2.25);", "margin-right: calc(var(--spacing) * 2);")
}
