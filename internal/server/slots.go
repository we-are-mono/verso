// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"cmp"
	"fmt"
	"log"
	"slices"

	"github.com/we-are-mono/verso/internal/plugin"
	"github.com/we-are-mono/verso/internal/widget"
)

// shellSlot is one slot a shell page opens (ADR-026): its name, the kind it
// takes, and its own items a contribution may stand before or after, in page
// order. The list is the shell's promise to plugins, held to the SDK's
// catalogue (testdata/slots.json) by a test.
type shellSlot struct {
	Name    string   `json:"name"`
	Kind    string   `json:"kind"`
	Anchors []string `json:"anchors"`
}

// shellSlots are every slot the shell's pages open.
var shellSlots = []shellSlot{
	{Name: plugin.AccessBlocks, Kind: "block", Anchors: []string{"password", "sessions", "web", "luci"}},
	{Name: "shell.overview.cards", Kind: "overview-card", Anchors: []string{"internet", "ports", "tunnels", "traffic", "resources", "system", "hardware"}},
}

// shellSlotNamed is the shell slot of that name.
func shellSlotNamed(name string) (shellSlot, bool) {
	i := slices.IndexFunc(shellSlots, func(s shellSlot) bool { return s.Name == name })
	if i < 0 {
		return shellSlot{}, false
	}
	return shellSlots[i], true
}

// slotPlacement is one plugin's contribution, bound for the slot it fills.
type slotPlacement struct {
	Plugin       string
	Contribution plugin.Contribution
}

// Name is the contribution's full id, its anchor for others.
func (p slotPlacement) Name() string { return p.Contribution.Name(p.Plugin) }

// slotIndex is every contribution by the slot it fills, built once each time
// the plugins load.
type slotIndex map[string][]slotPlacement

// indexSlots files every installed plugin's contributions by slot, and says,
// once, what each cannot find: a shell slot the shell does not open, a
// plugin slot whose opener is not installed, an anchor not in its slot.
// Whether an installed opener's page opens its slot is only known when the
// page is drawn, and a slot it leaves out simply draws nothing.
func indexSlots(manifests []plugin.Manifest) (slotIndex, []error) {
	installed := map[string]bool{}
	for _, m := range manifests {
		installed[m.ID] = true
	}
	index := slotIndex{}
	for _, m := range manifests {
		for _, c := range m.Contributions() {
			index[c.Slot] = append(index[c.Slot], slotPlacement{Plugin: m.ID, Contribution: c})
		}
	}
	var problems []error
	for name, placements := range index {
		owner, _ := plugin.SlotOwner(name)
		shell, known := shellSlotNamed(name)
		for _, p := range placements {
			switch {
			case owner == plugin.ShellID && !known:
				problems = append(problems, fmt.Errorf("plugin %q contribution %q fills %s, which no shell page opens", p.Plugin, p.Contribution.ID, name))
			case owner != plugin.ShellID && !installed[owner]:
				problems = append(problems, fmt.Errorf("plugin %q contribution %q fills %s, whose opener %q is not installed", p.Plugin, p.Contribution.ID, name, owner))
			default:
				anchor := p.Contribution.Position.Anchor
				switch {
				case anchor == "" || slices.Contains(shell.Anchors, anchor):
				case !slices.ContainsFunc(placements, func(o slotPlacement) bool { return o.Name() == anchor }):
					problems = append(problems, fmt.Errorf("plugin %q contribution %q stands by %s, which %s does not hold; it stands last", p.Plugin, p.Contribution.ID, anchor, name))
				case inCycle(p, placements):
					problems = append(problems, fmt.Errorf("plugin %q contribution %q stands by %s, which leads back to itself in %s; it stands last", p.Plugin, p.Contribution.ID, anchor, name))
				}
			}
		}
	}
	slices.SortFunc(problems, func(a, b error) int { return cmp.Compare(a.Error(), b.Error()) })
	return index, problems
}

// inCycle reports whether following p's anchors from contribution to
// contribution comes back to p.
func inCycle(p slotPlacement, placements []slotPlacement) bool {
	byName := make(map[string]slotPlacement, len(placements))
	for _, o := range placements {
		byName[o.Name()] = o
	}
	seen := map[string]bool{}
	for at := p; ; {
		next, ok := byName[at.Contribution.Position.Anchor]
		if !ok || seen[next.Name()] {
			return false
		}
		if next.Name() == p.Name() {
			return true
		}
		seen[next.Name()] = true
		at = next
	}
}

// loadSlots indexes the plugins' contributions and logs, once for this load
// of them, each one that cannot find its slot or its anchor.
func loadSlots(manifests []plugin.Manifest) slotIndex {
	index, problems := indexSlots(manifests)
	for _, p := range problems {
		log.Printf("verso: compatibility: %v", p)
	}
	return index
}

// slotEntry is one item of a slot in the order it is drawn: one of the
// slot's own items (Anchor) or a contribution (Placement).
type slotEntry struct {
	Anchor    string
	Placement *slotPlacement
}

// orderSlot is a slot's items and its contributions in the order they are
// drawn: the first contributions, then each own item with what stands
// before and after it, then the last, then any whose anchor is not there
// or leads back to itself. Each group at one place stands by order, then
// plugin id, then contribution id, so every render draws them alike.
func orderSlot(own []string, placed []slotPlacement) []slotEntry {
	sorted := slices.Clone(placed)
	slices.SortStableFunc(sorted, func(a, b slotPlacement) int {
		return cmp.Or(
			cmp.Compare(a.Contribution.Order, b.Contribution.Order),
			cmp.Compare(a.Plugin, b.Plugin),
			cmp.Compare(a.Contribution.ID, b.Contribution.ID),
		)
	})
	before, after := map[string][]*slotPlacement{}, map[string][]*slotPlacement{}
	var first, last []*slotPlacement
	for i := range sorted {
		p := &sorted[i]
		switch pos := p.Contribution.Position; pos.Where {
		case plugin.First:
			first = append(first, p)
		case plugin.Before:
			before[pos.Anchor] = append(before[pos.Anchor], p)
		case plugin.After:
			after[pos.Anchor] = append(after[pos.Anchor], p)
		default:
			last = append(last, p)
		}
	}
	var out []slotEntry
	placedDone := map[*slotPlacement]bool{}
	var emit func(anchor string, p *slotPlacement)
	emit = func(anchor string, p *slotPlacement) {
		if p != nil {
			if placedDone[p] {
				return
			}
			placedDone[p] = true
			anchor = p.Name()
		}
		for _, b := range before[anchor] {
			emit("", b)
		}
		out = append(out, slotEntry{Anchor: anchor, Placement: p})
		for _, a := range after[anchor] {
			emit("", a)
		}
	}
	for _, p := range first {
		emit("", p)
	}
	for _, a := range own {
		emit(a, nil)
	}
	for _, p := range last {
		emit("", p)
	}
	for i := range sorted {
		emit("", &sorted[i])
	}
	return out
}

// openedSlot is a slot a plugin's page opens for others to fill, named
// under its opener: a facts list's (Facts) or a slot widget's (Slot).
type openedSlot struct {
	Name, Kind string
	Slot       *widget.Slot
	Facts      *widget.Properties
}

// openedSlots are the slots the opener's page opens, in page order. What a
// contribution answered stands in the page already drawn, so no slot of its
// is among them.
func openedSlots(opener string, page widget.Widget) []openedSlot {
	var out []openedSlot
	widget.Walk(page, func(n widget.Widget) {
		switch n := n.(type) {
		case *widget.Properties:
			if n.Slot != "" {
				out = append(out, openedSlot{Name: opener + "." + n.Slot, Kind: "fact", Facts: n})
			}
		case *widget.Slot:
			if n.Opened() {
				out = append(out, openedSlot{Name: opener + "." + n.ID, Kind: n.Kind, Slot: n})
			}
		}
	})
	return out
}
