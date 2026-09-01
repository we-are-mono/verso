// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"encoding/json"
	"testing"
)

func countSections(w Widget) int {
	return countVisits(w, func(n Widget) bool { _, ok := n.(*Section); return ok })
}

func countFields(w Widget) int {
	return countVisits(w, func(n Widget) bool { _, ok := n.(*Field); return ok })
}

// TestFilterModeKeepsUntaggedTrees: a tree that declares no reading renders
// identically in both modes — a plugin adopts progressive disclosure by tagging,
// never by rewriting (ADR-015 §7).
func TestFilterModeKeepsUntaggedTrees(t *testing.T) {
	tree := func() Widget {
		return &Stack{Children: []Widget{
			&Section{Title: "Identity", Children: []Widget{&Field{Name: "hostname"}}},
			&Section{Title: "Clock", Children: []Widget{&Field{Name: "zone"}}},
		}}
	}
	for _, mode := range []string{ModeBasic, ModeAdvanced} {
		got := FilterMode(tree(), mode)
		if countSections(got) != 2 || countFields(got) != 2 {
			t.Errorf("%s mode changed an untagged tree: %d sections, %d fields", mode, countSections(got), countFields(got))
		}
	}
}

// TestFilterModeSelectsOneFace: the two tagged sections are alternatives the mode
// picks between, so exactly one face of the same fact renders (ADR-015 §2).
func TestFilterModeSelectsOneFace(t *testing.T) {
	tree := func() Widget {
		return &Stack{Children: []Widget{
			&Section{Title: "Simple", Mode: ModeBasic, Children: []Widget{&Text{Markdown: "plain"}}},
			&Section{Title: "Full", Mode: ModeAdvanced, Children: []Widget{&Text{Markdown: "detail"}}},
			&Section{Title: "Both", Children: []Widget{&Text{Markdown: "always"}}},
		}}
	}
	for _, tc := range []struct{ mode, want string }{
		{ModeBasic, "Simple"},
		{ModeAdvanced, "Full"},
	} {
		stack, ok := FilterMode(tree(), tc.mode).(*Stack)
		if !ok || len(stack.Children) != 2 {
			t.Fatalf("%s mode: want the matching face beside the untagged one, got %#v", tc.mode, stack)
		}
		if got := stack.Children[0].(*Section).Title; got != tc.want {
			t.Errorf("%s mode kept %q, want %q", tc.mode, got, tc.want)
		}
		if got := stack.Children[1].(*Section).Title; got != "Both" {
			t.Errorf("%s mode dropped the untagged section, kept %q", tc.mode, got)
		}
	}
}

// TestFilterModeHidesAdvancedFieldsInBasic: an advanced field is absent from the
// basic reading and present in the advanced one — and the fields beside it are
// untouched either way.
func TestFilterModeHidesAdvancedFieldsInBasic(t *testing.T) {
	tree := func() Widget {
		return &Form{Fields: []Widget{
			&Field{Name: "name"},
			&Field{Name: "family", Advanced: true},
		}}
	}
	basic, ok := FilterMode(tree(), ModeBasic).(*Form)
	if !ok || len(basic.Fields) != 1 || basic.Fields[0].(*Field).Name != "name" {
		t.Fatalf("basic mode form = %#v, want only the essential field", basic)
	}
	if advanced := FilterMode(tree(), ModeAdvanced).(*Form); len(advanced.Fields) != 2 {
		t.Errorf("advanced mode dropped a field: %#v", advanced.Fields)
	}
}

// TestFilterModeCollapsesWhatItEmpties: a heading must not outlive its contents,
// and a form left with no fields must not offer a submission. A container that was
// authored empty is left alone — the filter removes what it hid, nothing else.
func TestFilterModeCollapsesWhatItEmpties(t *testing.T) {
	page := &Stack{Children: []Widget{
		&Card{Title: "Tuning", Children: []Widget{
			&Section{Title: "Timers", Mode: ModeAdvanced, Children: []Widget{&Field{Name: "timeout"}}},
		}},
		&Section{Title: "Match", Children: []Widget{
			&Form{Fields: []Widget{&Field{Name: "mark", Advanced: true}}},
		}},
		&Empty{Title: "Nothing here yet"},
		&Section{Title: "Name", Children: []Widget{&Field{Name: "name"}}},
	}}
	stack, ok := FilterMode(page, ModeBasic).(*Stack)
	if !ok {
		t.Fatalf("filtered root is %T, want *Stack", stack)
	}
	if len(stack.Children) != 2 {
		t.Fatalf("basic page = %#v, want the authored-empty widget and the untagged section", stack.Children)
	}
	if _, isEmpty := stack.Children[0].(*Empty); !isEmpty {
		t.Errorf("a widget authored without children was collapsed: %#v", stack.Children[0])
	}
	if got := stack.Children[1].(*Section).Title; got != "Name" {
		t.Errorf("surviving section = %q, want Name", got)
	}
}

// TestFilterModeDropsATaggedRoot: a page whose whole body belongs to the other
// reading filters to nothing, which the renderer treats as an empty body rather
// than a failure.
func TestFilterModeDropsATaggedRoot(t *testing.T) {
	root := &Section{Title: "Diagnostics", Mode: ModeAdvanced, Children: []Widget{&Text{Markdown: "detail"}}}
	if got := FilterMode(root, ModeBasic); got != nil {
		t.Errorf("basic mode kept an advanced root: %#v", got)
	}
	if got := FilterMode(root, ModeAdvanced); got != Widget(root) {
		t.Errorf("advanced mode dropped its own root: %#v", got)
	}
	if got := FilterMode(nil, ModeBasic); got != nil {
		t.Errorf("filtering nothing returned %#v", got)
	}
}

// TestFilterModeIgnoresAModeItDoesNotKnow: only the two declared readings filter,
// so a mistyped tag shows up as a section that never hides rather than one that
// never appears.
func TestFilterModeIgnoresAModeItDoesNotKnow(t *testing.T) {
	tree := func() Widget {
		return &Stack{Children: []Widget{&Section{Title: "Expert", Mode: "expert", Children: []Widget{&Text{Markdown: "x"}}}}}
	}
	for _, mode := range []string{ModeBasic, ModeAdvanced} {
		if got := countSections(FilterMode(tree(), mode)); got != 1 {
			t.Errorf("%s mode dropped a section tagged with an unknown reading", mode)
		}
	}
}

// TestFilterModeReachesEveryContainer: the filter has to reach wherever a page can
// compose, so every nesting position the walk covers is pinned again here — a
// container the filter cannot rewrite would ship advanced markup to a basic reader.
func TestFilterModeReachesEveryContainer(t *testing.T) {
	hidden := func() Widget { return &Field{Name: "tuning", Advanced: true} }
	cases := []struct {
		name string
		tree Widget
	}{
		{"card", &Card{Children: []Widget{hidden(), &Text{Markdown: "keep"}}}},
		{"section children", &Section{Children: []Widget{hidden(), &Text{Markdown: "keep"}}}},
		{"section control", &Section{Control: hidden(), Children: []Widget{&Text{Markdown: "keep"}}}},
		{"grid", &Grid{Children: []Widget{hidden(), &Text{Markdown: "keep"}}}},
		{"stack", &Stack{Children: []Widget{hidden(), &Text{Markdown: "keep"}}}},
		{"canvas", &Canvas{Children: []Widget{hidden(), &Text{Markdown: "keep"}}}},
		{"disclosure", &Disclosure{Children: []Widget{hidden(), &Text{Markdown: "keep"}}}},
		{"empty", &Empty{Children: []Widget{hidden(), &Text{Markdown: "keep"}}}},
		{"capsule preview", &CapsulePreview{Children: []Widget{hidden(), &Text{Markdown: "keep"}}}},
		{"hero", &Hero{Children: []Widget{hidden(), &Text{Markdown: "keep"}}}},
		{"modal", &Modal{Children: []Widget{hidden(), &Text{Markdown: "keep"}}}},
		{"drawer", &Drawer{Trigger: []Widget{hidden(), &Text{Markdown: "keep"}}, Children: []Widget{hidden(), &Text{Markdown: "keep"}}}},
		{"conditional", &Conditional{Fields: []Widget{hidden(), &Text{Markdown: "keep"}}, Otherwise: []Widget{hidden(), &Text{Markdown: "keep"}}}},
		{"conditions", &Conditions{Items: []ConditionItem{{Key: "k", Children: []Widget{hidden(), &Text{Markdown: "keep"}}}}}},
		{"form", &Form{Fields: []Widget{hidden(), &Text{Markdown: "keep"}}}},
		{"tabs", &Tabs{Tabs: []Tab{{Children: []Widget{hidden(), &Text{Markdown: "keep"}}}}}},
		{"wizard", &Wizard{Steps: []WizardStep{{Children: []Widget{hidden(), &Text{Markdown: "keep"}}}}}},
		{"repeater", &Repeater{Items: []RepeaterItem{{Section: "s", Widget: hidden()}, {Section: "t", Widget: &Text{Markdown: "keep"}}}}},
		{"table row drawer", &Table{Rows: []TableRow{{Drawer: &RowDrawer{Children: []Widget{hidden(), &Text{Markdown: "keep"}}}}}}},
		{"table seam row drawer", &Table{Seam: &TableSeam{Rows: []TableRow{{Drawer: &RowDrawer{Children: []Widget{hidden(), &Text{Markdown: "keep"}}}}}}}},
		{"deep nesting", &Modal{Children: []Widget{
			&Tabs{Tabs: []Tab{{Children: []Widget{&Form{Fields: []Widget{hidden(), &Text{Markdown: "keep"}}}}}}},
		}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := countFields(tc.tree); got == 0 {
				t.Fatalf("the case places no advanced field to remove")
			}
			if got := countFields(FilterMode(tc.tree, ModeBasic)); got != 0 {
				t.Errorf("%d advanced fields survived the basic reading", got)
			}
			if got := countVisits(tc.tree, isText); got == 0 {
				t.Errorf("the filter removed what it was not asked to")
			}
		})
	}
}

// TestModeDeclarationsDecode: both declarations arrive over the wire from a
// plugin, so the decoder has to carry them (ADR-005 §1).
func TestModeDeclarationsDecode(t *testing.T) {
	w, err := Decode(json.RawMessage(`{
		"type": "section", "title": "Tuning", "mode": "advanced",
		"children": [{"type": "field", "name": "family", "advanced": true}]
	}`))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	section, ok := w.(*Section)
	if !ok {
		t.Fatalf("decoded %T, want *Section", w)
	}
	if section.Mode != ModeAdvanced {
		t.Errorf("section mode = %q, want %q", section.Mode, ModeAdvanced)
	}
	if field := section.Children[0].(*Field); !field.Advanced {
		t.Errorf("field advanced = false, want true")
	}
}
