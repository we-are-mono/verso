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
		"hover:bg-mid/50 hover:text-ink",
		// The glyph is inside the row's measure, so the form is 40rem wide
		// whether or not a row ends in one.
		`data-verso-condition="dest_port" class="flex max-w-[40rem] items-start gap-8"`,
		"hover:border-sand-5 hover:bg-rule", "border-rule-strong bg-transparent text-meta",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("conditions missing %q:\n%s", want, got)
		}
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

func TestDecodeConditionsRejectsDuplicateKeys(t *testing.T) {
	_, err := Decode([]byte(`{"type":"conditions","items":[` +
		`{"key":"rate","label":"Rate"},{"key":"rate","label":"Again"}]}`))
	if err == nil {
		t.Fatal("Decode: want duplicate condition key error")
	}
}
