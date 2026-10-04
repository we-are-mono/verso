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
// slice, is exactly the gap that would let fields inside a modal skip
// datatype validation.
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
		{"disclosure", &Disclosure{Children: []Widget{leaf()}}, 1},
		{"empty", &Empty{Children: []Widget{leaf()}}, 1},
		{"modal", &Modal{Children: []Widget{leaf()}}, 1},
		{"conditional", &Conditional{Fields: []Widget{leaf()}, Otherwise: []Widget{leaf()}}, 2},
		{"conditions", &Conditions{Items: []ConditionItem{{Key: "k", Children: []Widget{leaf()}}}}, 1},
		{"form", &Form{Fields: []Widget{leaf()}}, 1},
		{"repeater", &Repeater{Items: []RepeaterItem{{Section: "s", Widget: leaf()}}}, 1},
		{"table row drawer", &Table{Rows: []TableRow{{Drawer: &RowDrawer{Children: []Widget{leaf()}}}}}, 1},
		{"table seam row drawer", &Table{Seam: &TableSeam{Rows: []TableRow{{Drawer: &RowDrawer{Children: []Widget{leaf()}}}}}}, 1},
		{"deep nesting", &Modal{Children: []Widget{
			&Card{Children: []Widget{&Form{Fields: []Widget{leaf()}}}},
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

// TestStripFiltersReachesEveryContainer: removal has to reach wherever a page
// can compose, so the same nesting positions the walk covers are pinned again
// here — a container that visits its children but cannot rewrite them would
// leave a lens on a page that did not earn one.
func TestStripFiltersReachesEveryContainer(t *testing.T) {
	lens := func() Widget { return &Filter{Placeholder: "Filter…"} }
	isFilter := func(w Widget) bool { _, ok := w.(*Filter); return ok }
	cases := []struct {
		name string
		tree Widget
	}{
		{"card", &Card{Children: []Widget{lens()}}},
		{"section children", &Section{Children: []Widget{lens()}}},
		{"section control", &Section{Control: lens()}},
		{"grid", &Grid{Children: []Widget{lens()}}},
		{"stack", &Stack{Children: []Widget{lens()}}},
		{"disclosure", &Disclosure{Children: []Widget{lens()}}},
		{"empty", &Empty{Children: []Widget{lens()}}},
		{"modal", &Modal{Children: []Widget{lens()}}},
		{"conditional", &Conditional{Fields: []Widget{lens()}, Otherwise: []Widget{lens()}}},
		{"conditions", &Conditions{Items: []ConditionItem{{Key: "k", Children: []Widget{lens()}}}}},
		{"form", &Form{Fields: []Widget{lens()}}},
		{"repeater", &Repeater{Items: []RepeaterItem{{Section: "s", Widget: lens()}}}},
		{"table row drawer", &Table{Rows: []TableRow{{Drawer: &RowDrawer{Children: []Widget{lens()}}}}}},
		{"table seam row drawer", &Table{Seam: &TableSeam{Rows: []TableRow{{Drawer: &RowDrawer{Children: []Widget{lens()}}}}}}},
		{"deep nesting", &Modal{Children: []Widget{
			&Card{Children: []Widget{&Form{Fields: []Widget{lens()}}}},
		}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := countVisits(tc.tree, isFilter); got == 0 {
				t.Fatalf("the case places no lens to remove")
			}
			if got := countVisits(StripFilters(tc.tree), isFilter); got != 0 {
				t.Errorf("%d lenses survived the strip", got)
			}
		})
	}

	// Everything else stays where it was, in order.
	kept := StripFilters(&Stack{Children: []Widget{
		&Text{Markdown: "a"}, lens(), &Text{Markdown: "b"},
	}})
	stack, ok := kept.(*Stack)
	if !ok {
		t.Fatalf("stripped root is %T, want *Stack", kept)
	}
	if len(stack.Children) != 2 || stack.Children[0].(*Text).Markdown != "a" || stack.Children[1].(*Text).Markdown != "b" {
		t.Errorf("the strip disturbed the composition: %+v", stack.Children)
	}
	// A page that is nothing but its lens has nothing left.
	if got := StripFilters(lens()); got != nil {
		t.Errorf("a root filter should strip to nothing, got %T", got)
	}
}

// TestFilterableCount: the lens sifts rows and option rows, folded ones included,
// wherever they sit; 20 is the boundary at which a page still reads whole.
func TestFilterableCount(t *testing.T) {
	rows := func(n int) []TableRow {
		out := make([]TableRow, n)
		return out
	}
	items := func(n int) []SettingsItem {
		out := make([]SettingsItem, n)
		return out
	}
	cases := []struct {
		name string
		tree Widget
		want int
	}{
		{"nothing to sift", &Stack{Children: []Widget{&Text{Markdown: "prose"}}}, 0},
		{"rows and folded rows", &Table{Rows: rows(3), Seam: &TableSeam{Rows: rows(2)}}, 5},
		{"option rows and folded ones", &Settings{Items: items(4), Seam: &SettingsSeam{Items: items(1)}}, 5},
		{"across the page", &Stack{Children: []Widget{
			&Section{Children: []Widget{&Table{Rows: rows(18)}}},
			&Section{Children: []Widget{&Settings{Items: items(2)}}},
		}}, 20},
		{"one past the threshold", &Stack{Children: []Widget{
			&Section{Children: []Widget{&Table{Rows: rows(21)}}},
		}}, 21},
		// A live listing renders with nothing in it and fills afterwards. What
		// the lens will sift is the ring it keeps, so the page earns its lens
		// on the strength of the stream, not of the empty table it starts as.
		{"a live listing counts its ring", &Table{Stream: &TableStream{Source: StreamSourceFirewallLog}}, StreamRingDefault},
		{"a declared ring is what it keeps", &Table{Stream: &TableStream{Source: StreamSourceFirewallLog, Ring: 40}}, 40},
		{"a source nobody serves sifts nothing", &Table{Stream: &TableStream{Source: "syslog"}}, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := FilterableCount(c.tree); got != c.want {
				t.Errorf("FilterableCount = %d, want %d", got, c.want)
			}
		})
	}
	if FilterThreshold != 20 {
		t.Errorf("FilterThreshold = %d, want 20", FilterThreshold)
	}
}

// TestHoistFilter: the page-wide lens docks from the body's first child, so a
// filter a page nested anywhere in its tree is lifted to the front — the fix that
// stops a filter floating mid-page. A tree with no filter is untouched.
func TestHoistFilter(t *testing.T) {
	filter := &Filter{Placeholder: "find"}

	// Nested inside a section, mid-tree: hoisted to the root's first child.
	nested := &Stack{Children: []Widget{
		&Section{Title: "Panel"},
		&Section{Title: "Readings", Children: []Widget{filter, &Table{}}},
	}}
	got := HoistFilter(nested)
	s, ok := got.(*Stack)
	if !ok || len(s.Children) == 0 {
		t.Fatalf("hoisted root is not a populated stack: %#v", got)
	}
	if _, first := s.Children[0].(*Filter); !first {
		t.Errorf("filter was not hoisted to the first child: %#v", s.Children[0])
	}
	if n := countFilters(got); n != 1 {
		t.Errorf("hoist should keep exactly one filter, found %d", n)
	}

	// Already first: order is unchanged.
	front := &Stack{Children: []Widget{&Filter{}, &Section{}}}
	if s := HoistFilter(front).(*Stack); s.Children[0] != front.Children[0] {
		t.Error("a filter already first should stay first")
	}

	// No filter: the tree is returned as-is.
	plain := &Stack{Children: []Widget{&Section{Title: "only"}}}
	if HoistFilter(plain) != Widget(plain) {
		t.Error("a tree with no filter should be returned unchanged")
	}
}

func countFilters(w Widget) int {
	n := 0
	Walk(w, func(x Widget) {
		if _, ok := x.(*Filter); ok {
			n++
		}
	})
	return n
}
