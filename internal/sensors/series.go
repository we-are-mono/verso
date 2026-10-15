// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package sensors

// SeriesNames names each reading in temps for the history, in order: what the
// kernel calls the sensor (a zone's type, a chip channel's label or tempN),
// never the name a profile gives it, so renaming a reading keeps its history.
// A name more than one reading shares is told apart by its chip: the chip's
// device-tree node, which the board fixes, else its locator (a PCI address,
// an SFP cage), which is all a box without a device tree has.
func SeriesNames(temps []TempReading) []string {
	shared := map[string]int{}
	for _, t := range temps {
		shared[t.Kernel]++
	}
	out := make([]string, len(temps))
	for i, t := range temps {
		out[i] = t.Kernel
		if shared[t.Kernel] > 1 {
			chip := t.ofNode
			if chip == "" {
				chip = t.Source
			}
			out[i] += "." + chip
		}
	}
	return out
}

// SeriesRenames pairs each name a reading was recorded under before the
// kernel's names (its display name, told apart by its bus locator where
// another reading shared it) with its name now, so a history keeps what it
// recorded. A name unchanged is no pair, and neither is an old name another
// reading now records under: that series is the other reading's, so asking
// the renames at every start never takes it.
func SeriesRenames(temps []TempReading) map[string]string {
	now := SeriesNames(temps)
	current := map[string]bool{}
	for _, name := range now {
		current[name] = true
	}
	shared := map[string]int{}
	for _, t := range temps {
		shared[t.Name]++
	}
	out := map[string]string{}
	for i, t := range temps {
		was := t.Name
		if shared[t.Name] > 1 {
			was += "." + t.Source
		}
		if was != now[i] && !current[was] {
			out[was] = now[i]
		}
	}
	return out
}
