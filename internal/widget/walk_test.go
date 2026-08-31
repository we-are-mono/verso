// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import "testing"

// countVisits walks w and counts the visited widgets match reports true for.
func countVisits(w Widget, match func(Widget) bool) int {
	n := 0
	Walk(w, func(v Widget) {
		if match(v) {
			n++
		}
	})
	return n
}

func isText(w Widget) bool { _, ok := w.(*Text); return ok }

// TestWalkReachesNestedWidgets pins the children seam of every container that
// nests a widget slice: a marker leaf placed in each nesting position must be
// visited. A container missing here, or a container whose children() forgets a
// slice, is exactly the gap that once let fields inside a modal or a wizard
// skip datatype validation.
func TestWalkReachesNestedWidgets(t *testing.T) {
	leaf := func() Widget { return &Text{Markdown: "leaf"} }
	cases := []struct {
		name string
		tree Widget
		want int
	}{
		{"card", &Card{Children: []Widget{leaf()}}, 1},
		{"section children", &Section{Children: []Widget{leaf()}}, 1},
		{"grid", &Grid{Children: []Widget{leaf()}}, 1},
		{"stack", &Stack{Children: []Widget{leaf()}}, 1},
		{"canvas", &Canvas{Children: []Widget{leaf()}}, 1},
		{"disclosure", &Disclosure{Children: []Widget{leaf()}}, 1},
		{"empty", &Empty{Children: []Widget{leaf()}}, 1},
		{"capsule preview", &CapsulePreview{Children: []Widget{leaf()}}, 1},
		{"hero", &Hero{Children: []Widget{leaf()}}, 1},
		{"modal", &Modal{Children: []Widget{leaf()}}, 1},
		{"drawer", &Drawer{Trigger: []Widget{leaf()}, Children: []Widget{leaf()}}, 2},
		{"conditional", &Conditional{Fields: []Widget{leaf()}, Otherwise: []Widget{leaf()}}, 2},
		{"conditions", &Conditions{Items: []ConditionItem{{Key: "k", Children: []Widget{leaf()}}}}, 1},
		{"form", &Form{Fields: []Widget{leaf()}}, 1},
		{"tabs", &Tabs{Tabs: []Tab{{Children: []Widget{leaf()}}, {Children: []Widget{leaf()}}}}, 2},
		{"wizard", &Wizard{Steps: []WizardStep{{Children: []Widget{leaf()}}}}, 1},
		{"repeater", &Repeater{Items: []RepeaterItem{{Section: "s", Widget: leaf()}}}, 1},
		{"table row drawer", &Table{Rows: []TableRow{{Drawer: &RowDrawer{Children: []Widget{leaf()}}}}}, 1},
		{"table seam row drawer", &Table{Seam: &TableSeam{Rows: []TableRow{{Drawer: &RowDrawer{Children: []Widget{leaf()}}}}}}, 1},
		{"deep nesting", &Modal{Children: []Widget{
			&Tabs{Tabs: []Tab{{Children: []Widget{&Form{Fields: []Widget{leaf()}}}}}},
		}}, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := countVisits(tc.tree, isText); got != tc.want {
				t.Errorf("walk visited %d marker leaves, want %d", got, tc.want)
			}
		})
	}
}

// TestWalkReachesAttachedWidgets pins the single-widget attachments: a
// section's control, a callout's link, a row's or property's status badge, and
// a settings row's pills are all part of the tree.
func TestWalkReachesAttachedWidgets(t *testing.T) {
	cases := []struct {
		name  string
		tree  Widget
		match func(Widget) bool
		want  int
	}{
		{"section control", &Section{Control: &Switch{Name: "on"}},
			func(w Widget) bool { _, ok := w.(*Switch); return ok }, 1},
		{"callout link", &Callout{Link: &Link{Label: "More"}},
			func(w Widget) bool { _, ok := w.(*Link); return ok }, 1},
		{"row status", &Row{Status: &Badge{Text: "up"}},
			func(w Widget) bool { _, ok := w.(*Badge); return ok }, 1},
		{"property status", &Properties{Items: []Property{{Status: &Badge{Text: "up"}}}},
			func(w Widget) bool { _, ok := w.(*Badge); return ok }, 1},
		{"settings pills", &Settings{
			Items: []SettingsItem{{Pills: []Badge{{Text: "a"}, {Text: "b"}}}},
			Seam:  &SettingsSeam{Items: []SettingsItem{{Pills: []Badge{{Text: "c"}}}}},
		}, func(w Widget) bool { _, ok := w.(*Badge); return ok }, 3},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := countVisits(tc.tree, tc.match); got != tc.want {
				t.Errorf("walk visited %d attached widgets, want %d", got, tc.want)
			}
		})
	}
}
