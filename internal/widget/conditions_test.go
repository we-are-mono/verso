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
		// The block is headed as every section is: the shared band (4px from
		// the title to the lede, 20px from the lede to what follows) and the
		// shared lede, not a heading of its own spacing.
		`<div class="mb-5 flex flex-wrap items-center justify-between gap-x-6 gap-y-1">`,
		`<div data-verso-section-lede class="verso-prose text-body">Combined with and.</div>`,
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
		`<span class="text-xs font-medium tracking-[.08em] text-meta uppercase">Endpoints</span>`,
		// A condition the rule already carries stays in the list and stays legible,
		// but it is no longer an offer.
		`data-verso-condition-choose="dest_port" disabled`,
		// 576px wide, under the button, over the page.
		`w-[36rem]`, `top-full left-0 z-30 mt-2`,
		// A condition is a form row, and the one thing that is different about
		// it — that it can be taken away — is a glyph at the row's trailing
		// edge, labelled for a reader who cannot see it. It wears the quiet
		// treatment every icon act in a table wears: taking a condition off a
		// rule changes nothing on the device until the whole change is applied,
		// and crimson would promise a severity it does not have.
		`data-verso-condition-remove aria-label="Remove Destination ports"`,
		`border border-transparent text-glyph transition-colors hover:border-sand-5 hover:bg-rule hover:text-ink`,
		// The glyph stands on the condition's own name line, at the shared
		// form measure's edge, so the controls under it keep the full measure;
		// the condition itself runs the content's width, so the hairline between
		// two conditions does too.
		`data-verso-condition="dest_port" data-verso-and="and" role="group" aria-labelledby="condition-dest_port-label" class="verso-condition verso-condition-single"`,
		`class="verso-condition-name flex max-w-form items-center gap-3"`,
		`class="verso-rhythm max-w-form`,
		"hover:border-sand-5 hover:bg-rule",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("conditions missing %q:\n%s", want, got)
		}
	}
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
		`role="group" aria-labelledby="condition-icmp_type-label" class="verso-condition verso-condition-single"`,
		`<span id="condition-icmp_type-label" class="text-sm font-semibold text-ink group/tip relative cursor-help`,
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
		`class="verso-condition">`, // several parts: no single-control class
		`<span id="condition-rate-label"`,
		`>rate</span>`,
		`>Initial burst</label>`,
	} {
		if !strings.Contains(rate, want) {
			t.Errorf("several-part condition missing %q:\n%s", want, rate)
		}
	}
}

// TestConditionsStandInOneCardJoinedByAnd: the conditions a rule carries are
// one block — a card the list wears while it holds any (forms.css) — and
// each condition after the first says how it joins the one before it, in the
// reader's language, on the seam between them.
func TestConditionsStandInOneCardJoinedByAnd(t *testing.T) {
	c := &Conditions{Label: "Conditions", Items: []ConditionItem{
		{Key: "icmp_type", Label: "ICMP types", Active: true, Children: []Widget{&List{Name: "icmp_type", Label: "Types", Style: "tokens"}}},
		{Key: "rate", Label: "Rate limit", Active: true, Children: []Widget{&Field{Name: "limit", Label: "Rate"}}},
	}}
	got := render(t, newRenderer(t), c)
	for want, n := range map[string]int{
		`<div data-verso-condition-list class="verso-condition-list">`: 1,
		`data-verso-and="and"`: 4, // every condition carries it, active and in its template alike
	} {
		if c := strings.Count(got, want); c != n {
			t.Errorf("want %d of %q, got %d:\n%s", n, want, c, got)
		}
	}
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
		`<summary class="verso-reveal-act`,
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
// same compact act a set adds from, 16px under the slot, with no rule of its
// own above it. Once a condition is added the slot goes; the picker is its own.
func TestConditionsSayWhenEmptyAsASetDoes(t *testing.T) {
	c := &Conditions{Label: "Conditions", Items: []ConditionItem{{Key: "rate", Label: "Rate limit"}}}
	got := render(t, newRenderer(t), c)
	for _, want := range []string{
		// mb-0: the block's 8px step would otherwise stand the act 24px under
		// the slot, not the 16px it stands under every set's.
		`<div data-verso-condition-empty data-verso-slot class="flex items-center gap-3 rounded-xs border border-dashed border-rule-strong bg-quiet px-4 py-2.5 max-w-form mb-0">`,
		`<span aria-hidden="true" class="size-1.5 shrink-0 rounded-[1px] border border-faint"></span>`,
		`<p class="min-w-0 flex-1 text-sm leading-6 text-meta">No additional conditions.`,
		`<div data-verso-condition-picker class="relative pt-4">`,
		// The collection's own add act, glyph first.
		`group/act relative inline-flex h-7 shrink-0 cursor-pointer items-center gap-1.5 rounded-xs border border-rule-strong bg-transparent text-sm font-medium whitespace-nowrap text-meta transition-colors hover:border-sand-5 hover:bg-rule hover:text-ink focus-visible:outline-2 focus-visible:-outline-offset-2 focus-visible:outline-denim verso-press pl-2 pr-2.5">`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("want %s in:\n%s", want, got)
		}
	}

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
