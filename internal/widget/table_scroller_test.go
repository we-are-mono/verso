// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"strconv"
	"strings"
	"testing"
)

// TestTableScrollerContainsItsHiddenWords: a cell's screen-reader words (the
// dash's "None") are absolutely placed, so the scroller that clips a wide table
// has to be their containing block — otherwise a word in a column scrolled out
// of view widens the whole page on a phone.
func TestTableScrollerContainsItsHiddenWords(t *testing.T) {
	got := render(t, newRenderer(t), &Table{
		Columns: []TableColumn{{Label: "Airtime busy", Kind: "meter"}},
		Rows:    []TableRow{{ID: "a", Cells: []TableCell{{}}}},
	})
	if !strings.Contains(got, `data-verso-scroller class="verso-listing__scroller verso-scroll-edge"`) {
		t.Errorf("the table's scroller must contain its absolutely placed words:\n%s", got)
	}
	declares(t, "listing", ".verso-listing__scroller", "position: relative", "overflow-x: auto", "overflow-y: hidden")
}

// TestTableScrollerSaysThereIsMore: a table wider than its box (a phone) keeps
// its trailing columns — a row's act among them — out of sight, so the scroller
// wears the edge that fades while more waits to the right.
func TestTableScrollerSaysThereIsMore(t *testing.T) {
	got := render(t, newRenderer(t), &Table{Columns: []TableColumn{{Label: "Source"}}, Rows: []TableRow{{ID: "a", Cells: []TableCell{{Text: "wan"}}}}})
	if !strings.Contains(got, "verso-scroll-edge") {
		t.Errorf("the table's scroller must mark what waits past its edge:\n%s", got)
	}
}

// TestAListingHoldsItsScrollerInAFrameOfItsOwn: a page that is one listing is
// one card, and the frame stands outside the scroller, so the edge
// that fades while more of a wide table waits never fades the card's border.
func TestAListingHoldsItsScrollerInAFrameOfItsOwn(t *testing.T) {
	r := newRenderer(t)
	for name, w := range map[string]Widget{
		"table":      &Table{Columns: []TableColumn{{Label: "Source"}}, Rows: []TableRow{{ID: "a", Cells: []TableCell{{Text: "wan"}}}}},
		"interfaces": &Table{Style: "interfaces", Columns: []TableColumn{{Label: "Device"}}},
	} {
		got := render(t, r, w)
		const frame = `<div class="verso-listing" data-verso-listing data-verso-place="card">`
		if !strings.Contains(got, frame) || strings.Index(got, frame) > strings.Index(got, "data-verso-scroller") {
			t.Errorf("%s: the scroller does not stand in its listing's frame:\n%s", name, got)
		}
	}
	// The frame is the listing's, Paper in the Strong Hairline a card wears;
	// the scroller draws none of it.
	declares(t, "listing", `.verso-listing[data-verso-place="card"]`,
		"border: 1px solid var(--color-rule-strong);", "background-color: var(--color-ground);")
	if rule := cssRule(blockCSS(t, "listing"), ".verso-listing__scroller"); strings.Contains(rule, "border") {
		t.Errorf("the scroller draws a border its fading edge would fade:\n%s", rule)
	}
}

// TestPageSpacingIsThePagesNotTheBlocks: how far a listing stands from what
// sits above it is the page's rhythm (the stack's), not the listing's own —
// a titled listing carries no margin of its own, and the page stack names
// itself so the rhythm can find it.
func TestPageSpacingIsThePagesNotTheBlocks(t *testing.T) {
	r := newRenderer(t)
	titled := render(t, r, &Table{Title: "Radios", Columns: []TableColumn{{Label: "Radio"}}})
	if strings.Contains(titled, "mt-11") || strings.Contains(titled, "mt-10") {
		t.Errorf("a listing carries no page gap of its own:\n%s", titled)
	}
	stack := render(t, r, &Stack{Children: []Widget{&Text{Markdown: "a"}, &Text{Markdown: "b"}}})
	if !strings.HasPrefix(stack, `<div class="verso-stack">`) {
		t.Errorf("a plain stack names itself for the page rhythm:\n%s", stack)
	}
	// The stack's step reads the gap the page sets on its root stack.
	declares(t, "stack", ".verso-stack", "var(--verso-stack-gap,")
	// The band adds no gap of its own: it is its listing card's top strip.
	bar := render(t, r, &ActionBar{Tabs: []ActionTab{{Label: "All", Count: 1, Active: true}}})
	const open = `data-verso-actionbar class="`
	at := strings.Index(bar, open)
	if at < 0 {
		t.Errorf("no control band:\n%s", bar)
	} else {
		class := bar[at+len(open):]
		for _, c := range strings.Fields(class[:strings.Index(class, `"`)]) {
			if strings.HasPrefix(c, "mt-") || strings.HasPrefix(c, "mb-") || strings.HasPrefix(c, "my-") {
				t.Errorf("a control band carries no margin of its own; it sits on its listing (%s):\n%s", c, bar)
			}
		}
	}
}

// TestRuledSectionStandsFortyAboveAndBelowItsRule: a hairline between two of
// a surface's subjects has the page's gap on both sides of it — 2.5rem above
// the rule, 2.5rem from the rule to the section's heading — and the section a
// surface opens with draws neither rule nor gap. On a surface of cards the
// same section is a card, and its frame divides it.
func TestRuledSectionStandsFortyAboveAndBelowItsRule(t *testing.T) {
	got := render(t, newRenderer(t), &Section{Title: "Time", Hairline: true, Children: []Widget{&Text{Markdown: "x"}}})
	want := `<section class="verso-card" data-verso-section="card">`
	if !strings.Contains(got, want) {
		t.Errorf("ruled section should carry %q:\n%s", want, got)
	}
	// The numbers are blocks/section.css's: every rule's margin is its air
	// less the rule's own pixel.
	declares(t, "section", `.verso-section[data-verso-section="ruled"]`,
		"margin-top: var(--verso-step, calc(var(--spacing) * 10 - 1px));", "border-top-width: 1px;")
	declares(t, "section", `.verso-section:has(> .verso-section__head[data-verso-section-band]):is(:not(input[type="hidden"], template, [hidden]) + *)`,
		"margin-top: calc(var(--spacing) * 10 - 1px);")
	declares(t, "section", ".verso-section__head[data-verso-section-band]",
		"padding: calc(var(--spacing) * 10) var(--verso-reach-end, 0px) 0 var(--verso-reach-start, 0px);", "border-top-width: 1px;")
	declares(t, "section", `.verso-section:is([data-verso-section="ruled"], [data-verso-section="part"]):first-of-type`,
		"margin-top: 0;", "padding-top: 0;", "border-top-width: 0;")
}

// A phone's width leaves out the columns a listing names secondary — facts
// the row's panel still holds — so the ones that decide (the name, the state,
// the acts) stay on screen. The table names them by position
// (data-verso-narrow); the table block hides each one's head, cells and width
// under the phone's breakpoint.
func TestANarrowListingLeavesOutItsSecondaryColumns(t *testing.T) {
	r := newRenderer(t)
	tbl := &Table{
		Columns:   []TableColumn{{Label: "Device", Kind: "name"}, {Label: "Port", Kind: "mono"}, {Label: "MAC", Kind: "mono"}, {Label: "Status", Kind: "status"}},
		Rows:      []TableRow{{ID: "a", Cells: []TableCell{{Text: "laptop"}, {Text: "lan1"}, {Text: "aa:bb"}, {Text: "Online", Variant: "success"}}}},
		Secondary: []int{1, 2},
	}
	got := render(t, r, tbl)
	if !strings.Contains(got, `<table class="verso-table" data-verso-narrow="2 3">`) {
		t.Errorf("the table does not name its secondary columns: %s", got)
	}
	tbl.Secondary = []int{0, 9, -1}
	if got := render(t, r, tbl); strings.Contains(got, "data-verso-narrow") {
		t.Errorf("the identity column or a column the table lacks is left out: %s", got)
	}
	// The table block hides every position the table can name — its head,
	// its cells and its width — under the phone's breakpoint.
	css := blockCSS(t, "table")
	phone := css[strings.LastIndex(css, "@media (width < 40rem) {"):]
	for i := 2; i <= narrowMax; i++ {
		n := strconv.Itoa(i)
		for _, want := range []string{
			`.verso-table[data-verso-narrow~="` + n + `"] > :is(thead, tbody) > tr > :nth-child(` + n + `)`,
			`.verso-table[data-verso-narrow~="` + n + `"] > colgroup > col:nth-child(` + n + `)`,
		} {
			if !strings.Contains(phone, want) {
				t.Errorf("the phone does not leave out position %s: want %s", n, want)
			}
		}
	}
	if !strings.Contains(phone, ":nth-child(16) {\n    display: none;\n  }") {
		t.Errorf("the phone's narrow positions are not hidden:\n%s", phone)
	}
}

// A listing whose every row opens a panel needs no word from its plugin to
// fit a phone: the shell keeps the columns that decide — the first, the
// row's name, its state and its acts — and leaves out the facts the panel
// restates. A later identity column (a network a lease cites) is one of
// those facts.
func TestAPanelListingOnAPhoneKeepsItsNameStateAndActs(t *testing.T) {
	r := newRenderer(t)
	tbl := &Table{
		Columns: []TableColumn{{Label: "Device", Kind: "name"}, {Label: "Network", Kind: "reference"}, {Label: "Address", Kind: "mono"}, {Label: "Proto", Kind: "keyword"}, {Label: "State", Kind: "status"}, {Kind: "actions"}},
		Rows: []TableRow{
			{ID: "a", Panel: "/devices/a", Cells: []TableCell{{Text: "laptop"}, {Text: "lan"}, {Text: "192.0.2.4"}, {Text: "tcp"}, {Text: "Online", Variant: "success"}, {}}},
			{ID: "b", Entity: &EntityRef{Kind: "device", ID: "b"}, Cells: []TableCell{{Text: "phone"}, {Text: "lan"}, {Text: "192.0.2.5"}, {Text: "udp"}, {Text: "Offline"}, {}}},
		},
	}
	if got := render(t, r, tbl); !strings.Contains(got, `<table class="verso-table" data-verso-narrow="2 3 4">`) {
		t.Errorf("a phone should keep the name, the state and the acts: %s", got)
	}
}

// A row with no panel has nowhere else to say its facts, so a listing
// holding one keeps every column on a phone and scrolls instead.
func TestAListingWithARowThatOpensNothingKeepsEveryColumn(t *testing.T) {
	r := newRenderer(t)
	tbl := &Table{
		Columns: []TableColumn{{Label: "Target", Kind: "reference"}, {Label: "Gateway", Kind: "mono"}},
		Rows: []TableRow{
			{ID: "a", Panel: "/routes/a", Cells: []TableCell{{Text: "default"}, {Text: "192.0.2.1"}}},
			{ID: "b", Cells: []TableCell{{Text: "192.0.2.0/24"}, {Text: "—"}}},
		},
	}
	if got := render(t, r, tbl); strings.Contains(got, "data-verso-narrow") {
		t.Errorf("a row with no panel should keep its every column: %s", got)
	}
}

// What a listing names secondary itself is what a phone leaves out, even
// where the shell would have kept a column the listing knows to be
// secondary, or left out one it knows decides.
func TestAListingsOwnSecondaryColumnsOutrankTheShells(t *testing.T) {
	r := newRenderer(t)
	tbl := &Table{
		Columns:   []TableColumn{{Label: "Name", Kind: "name"}, {Label: "Action", Kind: "pill"}, {Label: "Hits", Kind: "num"}},
		Rows:      []TableRow{{ID: "a", Panel: "/rules/a", Cells: []TableCell{{Text: "Allow-Ping"}, {Text: "accept"}, {Text: "4"}}}},
		Secondary: []int{2},
	}
	if got := render(t, r, tbl); !strings.Contains(got, `<table class="verso-table" data-verso-narrow="3">`) {
		t.Errorf("the listing's own secondary columns should stand: %s", got)
	}
}

// A list — a few facts about the page's subject, such as who is signed in —
// stands in its card's inset as a fact list does, its hairlines stopping at
// the card's words rather than running to the frame. The listing says so, and
// the card runs only a listing that is no list to its edges (blocks/card.css).
func TestAListStandsInItsCardsInset(t *testing.T) {
	r := newRenderer(t)
	tbl := &Table{Style: "list", Columns: []TableColumn{{Label: "Source", Kind: "reference"}}, Rows: []TableRow{{ID: "a", Cells: []TableCell{{Text: "10.0.0.2"}}}}}
	if got := render(t, r, &Section{Title: "Signed in", Children: []Widget{tbl}}); !strings.Contains(got, `<div class="verso-listing" data-verso-listing data-verso-place="in-card" data-verso-list>`) {
		t.Errorf("a list should say it is one: %s", got)
	}
	declares(t, "card", ".verso-card__body > .verso-listing:not([data-verso-list])",
		"--verso-listing-bleed: var(--spacing-side);", "--verso-edge-start: var(--spacing-side);", "--verso-edge-end: var(--spacing-side);")
	tbl.Style = ""
	if got := render(t, r, &Section{Title: "Signed in", Children: []Widget{tbl}}); strings.Contains(got, "data-verso-list>") || !strings.Contains(got, `data-verso-place="in-card">`) {
		t.Errorf("a listing is not a list: %s", got)
	}
}

// A list's detail rides its value's line where the column has room for both
// and wraps under it where it has not — a long IPv6 address, a phone. The
// detail says it is one, so the sheet can flow it.
func TestAListsDetailIsMarkedToFlowBesideItsValue(t *testing.T) {
	r := newRenderer(t)
	tbl := &Table{Style: "list", Columns: []TableColumn{{Label: "Source", Kind: "reference"}, {Label: "Last seen"}},
		Rows: []TableRow{{ID: "a", Cells: []TableCell{{Text: "10.0.0.2", Detail: "Firefox · Linux"}, {Text: "Now", Detail: "since today, 16:43"}}}}}
	if got := render(t, r, tbl); strings.Count(got, "data-verso-detail") != 2 {
		t.Errorf("each detail should say it is one: %s", got)
	}
}
