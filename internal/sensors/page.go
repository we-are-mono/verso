// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package sensors

import (
	"slices"
	"sort"
)

// The Hardware page's readings, one list per kind. A profile chooses them: an
// entry left out of its array stays off the page (profiles/README.md). A board
// with no profile, or one whose profile names nothing of a kind the box
// reports, lists every reading of that kind under the kernel's names, so a
// stale profile never hides what the hardware says.

// PageTemps is the Temperatures card: the CPU first, then the board's own
// temperatures in curation order, then (with no profile) every secondary
// channel. An SFP module's temperature stands on its fibre card instead.
func (inv Inventory) PageTemps() []TempReading {
	profiled := slices.ContainsFunc(inv.Temps, func(t TempReading) bool { return t.Named })
	var cpu, curated, rest []TempReading
	for _, t := range inv.Temps {
		switch {
		case t.Fiber:
		case t.CPU:
			cpu = append(cpu, t)
		case profiled && !t.Named:
		case t.Curated:
			curated = append(curated, t)
		default:
			rest = append(rest, t)
		}
	}
	sort.SliceStable(curated, func(i, j int) bool { return curated[i].Rank < curated[j].Rank })
	return slices.Concat(cpu, curated, rest)
}

// PageRails is the Power card: the input rail first, then every other rail by
// draw, highest first.
func (inv Inventory) PageRails() []PowerReading {
	profiled := slices.ContainsFunc(inv.Powers, func(p PowerReading) bool { return p.Named })
	var main, rest []PowerReading
	for _, p := range inv.Powers {
		switch {
		case profiled && !p.Named:
		case p.Main:
			main = append(main, p)
		default:
			rest = append(rest, p)
		}
	}
	sort.SliceStable(rest, func(i, j int) bool { return rest[i].MicroW > rest[j].MicroW })
	return slices.Concat(main, rest)
}

// PageFans is the Cooling card: the main fan first, then every other fan in
// channel order.
func (inv Inventory) PageFans() []FanReading {
	profiled := slices.ContainsFunc(inv.Fans, func(f FanReading) bool { return f.Named })
	var main, rest []FanReading
	for _, f := range inv.Fans {
		switch {
		case profiled && !f.Named:
		case f.Main:
			main = append(main, f)
		default:
			rest = append(rest, f)
		}
	}
	return slices.Concat(main, rest)
}
