// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"strings"
	"testing"
)

func TestDecodeAndRenderConditions(t *testing.T) {
	in := []byte(`{"type":"conditions","label":"Conditions","help":"Combined with and.","items":[` +
		`{"key":"dest_port","label":"Destination ports","group":"Endpoints",` +
		`"hint":"space-separated, or a range","active":true,"children":[` +
		`{"type":"list","name":"dest_port","label":"Included ports","items":["53","67"]}]},` +
		`{"key":"rate","label":"Rate limit","group":"Rate and time",` +
		`"hint":"how often it may match","children":[` +
		`{"type":"field","name":"limit","label":"Rate","value":"1000"}]}` +
		`]}`)
	w, err := Decode(in)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	c, ok := w.(*Conditions)
	if !ok || len(c.Items) != 2 || !c.Items[0].Active {
		t.Fatalf("decoded conditions wrong: %#v", w)
	}
	got := render(t, newRenderer(t), c)
	for _, want := range []string{
		`data-verso-conditions`, `data-verso-condition="dest_port"`,
		// The block is headed as a part of its card is: its name on a ledger
		// line, the Strong Hairline running on from it, and the lede on a line
		// of its own under them.
		`<fieldset data-verso-conditions class="verso-conditions">`,
		`<div class="verso-conditions__head">`,
		`<legend class="verso-conditions__title">`,
		`<span aria-hidden="true" class="verso-conditions__line"></span>`,
		`<div data-verso-section-lede class="verso-conditions__lede verso-prose">Combined with and.</div>`,
		`data-verso-condition-template="rate"`, `name="dest_port"`,
		`Combined with and.`,
		// The picker: one act rather than a closed list beside an Add button. Each
		// line states the condition, the option it writes, and what sort of value
		// it takes; the filter is there because a catalogue this size is faster to
		// type into than to read through.
		`data-verso-condition-open aria-expanded="false"`, `Add a condition`,
		`data-verso-condition-filter`, `placeholder="Filter conditions"`,
		`data-verso-condition-choose="rate"`, `how often it may match`,
		// Grouped, with the heading set as a kicker over its run of entries.
		`data-verso-condition-group-name="Endpoints"`,
		`data-verso-condition-group-name="Rate and time"`,
		`<div class="verso-conditions__group-head">Endpoints</div>`,
		// A condition the rule already carries stays in the list and stays legible,
		// but it is no longer an offer.
		`data-verso-condition-choose="dest_port" disabled`,
		// 576px wide, under the button, over the page (declared below).
		`<div data-verso-condition-menu hidden class="verso-conditions__menu">`,
		// A condition is a form row, and the one thing that is different about
		// it — that it can be taken away — is a glyph at the row's trailing
		// edge, labelled for a reader who cannot see it. It wears the quiet
		// treatment every icon act in a table wears, the button block's icon
		// size: taking a condition off a rule changes nothing on the device
		// until the whole change is applied, and crimson would promise a
		// severity it does not have.
		`data-verso-condition-remove aria-label="Remove Destination ports"`,
		`class="verso-button ml-auto" data-verso-size="icon">`,
		// The glyph stands on the condition's own name line, at the shared
		// form measure's edge, so the controls under it keep the full measure;
		// the condition itself runs the content's width, so the hairline between
		// two conditions does too.
		`data-verso-condition="dest_port" data-verso-and="and" role="group" aria-labelledby="condition-dest_port-label" class="verso-condition" data-verso-single>`,
		`<div class="verso-condition__name">`,
		`<div class="verso-condition__parts verso-rhythm">`,
		`data-verso-condition-open aria-expanded="false" class="verso-button group/act relative" data-verso-size="act" data-verso-glyph>`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("conditions missing %q:\n%s", want, got)
		}
	}
	declares(t, "conditions", ".verso-conditions", "padding-top: calc(var(--spacing) * 4);")
	declares(t, "conditions", ".verso-conditions__head",
		"flex-wrap: wrap;", "align-items: center;", "column-gap: calc(var(--spacing) * 3);", "margin-bottom: var(--spacing-inset);")
	declares(t, "conditions", ".verso-conditions__title", "flex-shrink: 0;", "color: var(--color-ink);", "@apply type-subheading;")
	declares(t, "conditions", ".verso-conditions__line", "height: 1px;", "background-color: var(--color-rule-strong);")
	declares(t, "conditions", ".verso-conditions__lede", "flex: 1 0 100%;")
	declares(t, "conditions", ".verso-conditions__lede:is(.verso-prose)", "color: var(--color-body);", "@apply type-lede;")
	declares(t, "conditions", ".verso-conditions__group-head", "color: var(--color-meta);", "@apply type-label;")
	declares(t, "conditions", ".verso-conditions__menu",
		"position: absolute;", "top: 100%;", "left: 0;", "z-index: 30;", "width: 36rem;", "margin-top: calc(var(--spacing) * 2);")
	declares(t, "condition", ".verso-condition__name",
		"display: flex;", "align-items: center;", "gap: calc(var(--spacing) * 3);", "max-width: var(--verso-form-width, 40rem);")
	declares(t, "condition", ".verso-condition__parts", "max-width: var(--verso-form-width, 40rem);")
}

// TestAConditionIsOneNamedSetting: an added condition reads as the setting it
// is — its name, the key it is known by, what it is for raised on the name, and
// the glyph that takes it away, all on one line, as every row in the form is
// labelled. A condition of one control draws that control straight under the
// name, its own label kept for a screen reader only, and the name wears the
// control's staged mark. A condition of several parts keeps each part's label,
// set quieter than the name it sits under.
func TestAConditionIsOneNamedSetting(t *testing.T) {
	c := &Conditions{Label: "Conditions", Items: []ConditionItem{
		{Key: "icmp_type", Label: "ICMP types", Help: "Narrow an ICMP rule.", Active: true, Children: []Widget{
			&List{Name: "icmp_type", Label: "Types", Style: "tokens", Items: []string{"echo-request"}, Staged: true},
		}},
		{Key: "rate", Label: "Rate limit", Active: true, Children: []Widget{
			&Field{Name: "limit", Label: "Rate", Key: "limit", Value: "1000"},
			&Field{Name: "limit_burst", Label: "Initial burst", Key: "limit_burst"},
		}},
	}}
	got := render(t, newRenderer(t), c)
	icmp := got[strings.Index(got, `data-verso-condition="icmp_type"`):strings.Index(got, `data-verso-condition="rate"`)]
	for _, want := range []string{
		`role="group" aria-labelledby="condition-icmp_type-label" class="verso-condition" data-verso-single>`,
		`<span id="condition-icmp_type-label" class="verso-field__name" data-verso-tip tabindex="0"`,
		`Narrow an ICMP rule.`,
		`>icmp_type</span>`,
		`data-verso-condition-remove aria-label="Remove ICMP types"`,
		"data-verso-staged-row",
	} {
		if !strings.Contains(icmp, want) {
			t.Errorf("single condition missing %q:\n%s", want, icmp)
		}
	}
	// The name line comes first: the glyph stands on it, before any control.
	if strings.Index(icmp, "data-verso-condition-remove") > strings.Index(icmp, "data-verso-token-list") {
		t.Errorf("the remove glyph must stand on the name line, above the control:\n%s", icmp)
	}
	rate := got[strings.Index(got, `data-verso-condition="rate"`):]
	for _, want := range []string{
		`class="verso-condition">`, // several parts: not data-verso-single
		`<span id="condition-rate-label"`,
		`>rate</span>`,
		`>Initial burst</label>`,
	} {
		if !strings.Contains(rate, want) {
			t.Errorf("several-part condition missing %q:\n%s", want, rate)
		}
	}
	// A name that explains itself takes the help cursor; a single control's
	// own label is kept for a screen reader only, and a part's name is set
	// quieter than the condition's.
	declares(t, "field", ".verso-field__name[data-verso-tip]", "position: relative;", "cursor: help;")
	declares(t, "field", ".verso-field__label:where(.verso-condition[data-verso-single] .verso-condition__parts *)",
		"position: absolute;", "width: 1px;", "height: 1px;", "clip-path: inset(50%);")
	declares(t, "field", ".verso-field__name:where(.verso-condition:not([data-verso-single]) .verso-condition__parts *)",
		"color: var(--color-body);", "@apply type-row-label;")
}

// TestConditionsStandInOneCardJoinedByAnd: the conditions a rule carries are
// one block — a card the list wears while it holds any (blocks/conditions.css) — and
// each condition after the first says how it joins the one before it, in the
// reader's language, on the seam between them.
func TestConditionsStandInOneCardJoinedByAnd(t *testing.T) {
	c := &Conditions{Label: "Conditions", Items: []ConditionItem{
		{Key: "icmp_type", Label: "ICMP types", Active: true, Children: []Widget{&List{Name: "icmp_type", Label: "Types", Style: "tokens"}}},
		{Key: "rate", Label: "Rate limit", Active: true, Children: []Widget{&Field{Name: "limit", Label: "Rate"}}},
	}}
	got := render(t, newRenderer(t), c)
	for want, n := range map[string]int{
		`<div data-verso-condition-list class="verso-conditions__list">`: 1,
		`data-verso-and="and"`: 4, // every condition carries it, active and in its template alike
	} {
		if c := strings.Count(got, want); c != n {
			t.Errorf("want %d of %q, got %d:\n%s", n, want, c, got)
		}
	}
	declares(t, "conditions", ".verso-conditions__list:has(> .verso-condition)",
		"border: 1px solid var(--color-rule);", "background: var(--color-quiet);")
	declares(t, "condition", ".verso-condition + .verso-condition::before", "content: attr(data-verso-and);")
}

// TestARevealIsAQuietAct: a reveal folds optional rows (a condition's
// exceptions) behind one quiet act in the link's ink, a plus and the words,
// with no frame of its own; it arrives open when what it folds already holds
// something, so nothing a rule says is ever hidden.
func TestARevealIsAQuietAct(t *testing.T) {
	folded := render(t, newRenderer(t), &Disclosure{Style: "reveal", Summary: "Exclude some", Children: []Widget{
		&List{Name: "src_ip_not", Label: "Exclude", Style: "tokens"},
	}})
	for _, want := range []string{
		`<details class="verso-reveal">`,
		`<summary class="verso-reveal__act">`,
		`Exclude some`,
		`name="src_ip_not"`,
	} {
		if !strings.Contains(folded, want) {
			t.Errorf("reveal missing %q:\n%s", want, folded)
		}
	}
	if strings.Contains(folded, "verso-disclosure") {
		t.Errorf("a reveal wears no disclosure frame:\n%s", folded)
	}
	opened := render(t, newRenderer(t), &Disclosure{Style: "reveal", Summary: "Exclude some", Open: true})
	if !strings.Contains(opened, `<details open class="verso-reveal">`) {
		t.Errorf("a reveal holding something arrives open:\n%s", opened)
	}
}

// The picker's groups keep the catalogue's own order, and so do the entries inside
// them: which condition someone reaches for first is an editorial judgement the
// plugin made, and alphabetising either would throw it away. An entry naming no
// group is listed first, ungrouped, so a short catalogue needs no headings at all.
func TestConditionsGroupsKeepCatalogueOrder(t *testing.T) {
	items := []ConditionItem{
		{Key: "plain", Label: "Plain"},
		{Key: "dest_ip", Label: "To address", Group: "Endpoints"},
		{Key: "rate", Label: "Rate limit", Group: "Rate and time"},
		{Key: "src_ip", Label: "From address", Group: "Endpoints"},
	}
	options := make([]conditionOptionView, len(items))
	for i, item := range items {
		options[i] = conditionOptionView{Key: item.Key, Label: item.Label}
	}
	groups := groupConditions(items, options)
	if len(groups) != 3 {
		t.Fatalf("want three groups, got %d: %#v", len(groups), groups)
	}
	for i, want := range []string{"", "Endpoints", "Rate and time"} {
		if groups[i].Name != want {
			t.Errorf("group %d is %q, want %q", i, groups[i].Name, want)
		}
	}
	// Endpoints holds both of its entries, in the order declared, even though one
	// of them was declared after a different group's.
	got := []string{groups[1].Items[0].Key, groups[1].Items[1].Key}
	if got[0] != "dest_ip" || got[1] != "src_ip" {
		t.Errorf("Endpoints holds %v, want [dest_ip src_ip]", got)
	}
}

// TestConditionsSayWhenEmptyAsASetDoes: a rule carrying no condition draws
// the empty set the way every set does — the dashed Quiet Sand slot marked
// absent, its words in Meta, at the form's measure — and the way in is the
// same compact act a set adds from, a cell under the slot and centred in the
// two cells after, with no rule of its own above it. Once a condition is added
// the slot goes; the picker is its own.
func TestConditionsSayWhenEmptyAsASetDoes(t *testing.T) {
	c := &Conditions{Label: "Conditions", Items: []ConditionItem{{Key: "rate", Label: "Rate limit"}}}
	got := render(t, newRenderer(t), c)
	for _, want := range []string{
		// mb-0: the act stands a cell under the slot, as it does under every
		// set's, and nothing of the block adds to that.
		// The slot gives its bottom border's pixel back itself (the slot
		// block's -1px), so the act lands on the line.
		`<div data-verso-condition-empty data-verso-slot class="verso-slot verso-conditions__empty">`,
		`<span aria-hidden="true" class="verso-mark"></span>`,
		`<p class="verso-slot__words">No additional conditions.`,
		`<div data-verso-condition-picker class="verso-conditions__picker">`,
		// The collection's own add act, glyph first.
		`class="verso-button group/act relative" data-verso-size="act" data-verso-glyph>`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("want %s in:\n%s", want, got)
		}
	}
	declaresTheSlot(t)
	declares(t, "conditions", ".verso-conditions__empty", "max-width: var(--verso-form-width, 40rem);")
	declares(t, "conditions", ".verso-conditions__picker", "padding-block: calc(var(--spacing) * 6) var(--spacing);")

	// With a condition carried, the slot is there for the script to bring back
	// but hidden — by the attribute, which no display utility overrides.
	c.Items[0].Active = true
	got = render(t, newRenderer(t), c)
	if !strings.Contains(got, `<div data-verso-condition-empty data-verso-slot hidden class="`) {
		t.Errorf("a carried condition hides the slot:\n%s", got)
	}
}

func TestDecodeConditionsRejectsDuplicateKeys(t *testing.T) {
	_, err := Decode([]byte(`{"type":"conditions","items":[` +
		`{"key":"rate","label":"Rate"},{"key":"rate","label":"Again"}]}`))
	if err == nil {
		t.Fatal("Decode: want duplicate condition key error")
	}
}
