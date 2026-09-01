// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

// The two readings of Verso (ADR-015). A person is in one of them app-wide, and
// content declares which reading it belongs to: a Section carries a Mode, a Field
// an Advanced flag, and everything untagged belongs to both.
const (
	ModeBasic    = "basic"
	ModeAdvanced = "advanced"
)

// FilterMode removes from a tree everything the other reading declares and returns
// the root — nil when the root itself belongs to the other mode. Three rules, all
// mechanical: a section tagged for the other mode goes, an advanced field goes in
// basic mode, and a container the filter emptied goes with its contents, so no
// heading outlives what it introduced.
//
// What a tag *means* is the declarer's obligation, not the shell's (ADR-015 §4):
// the shell does not know a plugin's defaults, so it never second-guesses a tag it
// is handed. A tree that mentions neither declaration comes back unchanged.
func FilterMode(w Widget, mode string) Widget {
	if w == nil {
		return nil
	}
	// decided memoizes each widget's answer. prune recurses into the survivors of
	// every slice it rewrites, so a subtree is visited once per ancestor; the memo
	// makes each of those visits a lookup rather than a second filtering pass.
	decided := make(map[Widget]bool)
	var keep func(Widget) bool
	keep = func(n Widget) bool {
		if answer, done := decided[n]; done {
			return answer
		}
		answer := visibleInMode(n, mode)
		if answer {
			if p, ok := n.(pruner); ok {
				// Filter the children first, then ask what is left of the widget:
				// a container that held something and now holds nothing was emptied
				// by the filter, while one authored empty is left exactly as it was.
				held := len(n.children()) > 0
				p.prune(keep)
				answer = !held || len(n.children()) > 0
			}
		}
		decided[n] = answer
		return answer
	}
	if !keep(w) {
		return nil
	}
	return w
}

// visibleInMode reports whether one widget belongs to the reading being rendered.
// Only the declared tags filter: a section tagged for the other mode, and an
// advanced field in basic mode. Anything else renders in both — including a mode
// name this shell does not know, so a typo shows up as content that never hides
// rather than content that never appears.
func visibleInMode(w Widget, mode string) bool {
	switch w := w.(type) {
	case *Section:
		switch w.Mode {
		case ModeBasic:
			return mode != ModeAdvanced
		case ModeAdvanced:
			return mode == ModeAdvanced
		}
	case *Field:
		if w.Advanced {
			return mode == ModeAdvanced
		}
	}
	return true
}
