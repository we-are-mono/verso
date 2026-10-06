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
		// centred on the value's 20px first line, 10px down the row
		`<span class="absolute -left-3.75 top-1.75 size-1.5 rounded-[1px] bg-marigold" aria-hidden="true"></span>`,
		`<dd class="relative flex min-w-0 items-start`,
		// the note writes on the row's next 20px line
		`<p class="basis-full sm:pl-30 text-sm leading-5 font-normal text-body">Browsers warn until you trust it.</p>`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("want %s in:\n%s", want, got)
		}
	}
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
		"flex flex-col items-start sm:flex-row gap-x-6",
		`<dt class="shrink-0 text-meta sm:w-24">`,
		"self-stretch sm:flex-1 sm:self-auto",
		"text-balance",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("want %s in:\n%s", want, got)
		}
	}
	right := render(t, newRenderer(t), &Properties{Items: []Property{{Label: "Uptime", Value: "3 d"}}})
	if strings.Contains(right, "flex-col") {
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
		`<div class="flex items-start justify-between gap-x-6`,
		`<dd class="relative flex min-w-0 items-start`,
		`inline-flex h-5 shrink-0 items-center`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("want %s in:\n%s", want, got)
		}
	}
}

// TestArtifactFrameNamesWhatItHolds: a stored document says what it is before
// its facts do — a header on the frame's own inset, the name over a sentence of
// what it is for, closed off from the facts by the strong hairline a column
// head sits on. It is the card's part, one step under the section it is in.
func TestArtifactFrameNamesWhatItHolds(t *testing.T) {
	got := render(t, newRenderer(t), &Card{Style: "artifact", Title: "HTTPS certificate", Subtitle: "What this router shows browsers.", Children: []Widget{&Properties{}}})
	for _, want := range []string{
		// a cell of air over the name, the name's 20px line and the
		// sentence's, a cell under them, and the hairline on the cell's line
		`<div class="border-b border-rule-strong pt-5 pb-4.75"><h3 class="text-base leading-5 font-semibold text-ink">HTTPS certificate</h3><p class="text-sm leading-5 text-body">What this router shows browsers.</p></div>`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("want %s in:\n%s", want, got)
		}
	}
}

// TestArtifactFrameKeepsOneInset: inside the frame around a stored document,
// every word writes on the notebook's 20px lines, and the frame's own edges
// are lines like any other — a pixel out at the top and the left onto the
// grid's own, closing 19px under its last row so its bottom edge is the next
// line. At the sides the frame holds the words 20px in.
func TestArtifactFrameKeepsOneInset(t *testing.T) {
	got := render(t, newRenderer(t), &Card{Style: "artifact", Children: []Widget{&Properties{}}})
	if !strings.Contains(got, `class="-mt-px -ml-px w-[calc(100%_+_1px)] rounded-xs border border-rule bg-quiet px-5 pb-4.75"`) {
		t.Errorf("want one inset on every side:\n%s", got)
	}
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
		`<span class="relative block h-5">`, // the ruler stands on the row's first line, level with its label
		// the meter's own track, never a hairline: a list of hairline rows
		// must not have one more line in it that means something else
		`<span class="absolute inset-x-0 top-1/2 h-1.5 -translate-y-1/2 rounded-full bg-rule"></span>`,
		`<span class="absolute left-0 top-1/2 h-1.5 -translate-y-1/2 rounded-full bg-green" style="width: 6%"></span>`,
		`<span class="whitespace-nowrap">2026-08-31</span><span class="whitespace-nowrap">2027-10-02</span>`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("want %s in:\n%s", want, got)
		}
	}
	over := render(t, newRenderer(t), &Properties{Items: []Property{
		{Label: "Valid", Span: &PropertySpan{From: "a", To: "b", At: 140, Tone: "danger"}},
	}})
	if !strings.Contains(over, `rounded-full bg-crimson" style="width: 100%"`) {
		t.Errorf("the fill never leaves the stretch:\n%s", over)
	}
}
