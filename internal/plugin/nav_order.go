// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package plugin

import (
	"encoding/json"
	"fmt"
	"math"
)

// FirstPlace is the lowest place a plugin's entry takes: Overview alone
// stands before it, at 0.
const FirstPlace = 1

// readNavOrders sets each nav entry's place from the manifest's own words,
// one entry at a time: a place that is not a whole number is logged and read
// as none, and the plugin loads all the same. A place below FirstPlace reads
// as FirstPlace.
func (m *Manifest) readNavOrders(data []byte) (problems []error) {
	var list struct {
		Nav []struct {
			Order json.RawMessage `json:"order"`
		} `json:"nav"`
	}
	if json.Unmarshal(data, &list) != nil {
		return nil // the main decode has already said what is wrong
	}
	for i, entry := range list.Nav {
		if i >= len(m.Nav) || entry.Order == nil {
			continue
		}
		order, err := wholePlace(entry.Order)
		if err != nil {
			problems = append(problems, fmt.Errorf("plugin %q nav[%d]: order %s %w, read as no place", m.ID, i, entry.Order, err))
			continue
		}
		m.Nav[i].Order = &order
	}
	return problems
}

// wholePlace reads a place: a whole number, at least FirstPlace.
func wholePlace(raw json.RawMessage) (int, error) {
	var n float64
	if err := json.Unmarshal(raw, &n); err != nil {
		return 0, fmt.Errorf("is not a number")
	}
	if n != math.Trunc(n) || math.Abs(n) > math.MaxInt32 {
		return 0, fmt.Errorf("is not a whole number")
	}
	return max(int(n), FirstPlace), nil
}
