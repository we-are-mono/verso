// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"encoding/json"
	"fmt"
)

// Repeater is a behavioural widget: a repeatable group of widgets backed by a set
// of uci sections (ADR-005 §7). A plugin declares the *intent* — these items
// repeat, and they are `SectionType` sections of `Config` — and renders the items
// it reads. The shell owns the add/remove affordances and realizes the structural
// change (uci add/delete) itself; the plugin ships no behaviour. The realization
// is a re-render round-trip today, swappable for client-side later behind this same
// declaration.
type Repeater struct {
	Config      string         // uci config backing the items (e.g. "network")
	SectionType string         // uci section type of each item (e.g. "wireguard_wg0")
	AddLabel    string         // label for the add affordance
	Items       []RepeaterItem // the current items, in order
}

// RepeaterItem is one repeated element: the uci section it maps to (so remove
// knows what to delete) and the widget subtree that renders it.
type RepeaterItem struct {
	Section string
	Widget  Widget
}

func (*Repeater) isWidget() {}

// The hidden form fields the shell's repeater affordances post, and that the
// gateway reads to realize the structural change. They live here because this
// package renders the affordances; the gateway imports these same constants, so
// the two never drift.
const (
	RepeaterOpField      = "_repeater_op"
	RepeaterConfigField  = "_repeater_config"
	RepeaterTypeField    = "_repeater_type"
	RepeaterSectionField = "_repeater_section"
	RepeaterOpAdd        = "add"
	RepeaterOpRemove     = "remove"
)

// UnmarshalJSON decodes a repeater's items recursively through Decode, so an
// unknown item widget fails loudly rather than vanishing.
func (rp *Repeater) UnmarshalJSON(data []byte) error {
	var raw struct {
		Config      string `json:"config"`
		SectionType string `json:"section_type"`
		AddLabel    string `json:"add_label"`
		Items       []struct {
			Section string          `json:"section"`
			Widget  json.RawMessage `json:"widget"`
		} `json:"items"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	rp.Config = raw.Config
	rp.SectionType = raw.SectionType
	rp.AddLabel = raw.AddLabel
	rp.Items = make([]RepeaterItem, 0, len(raw.Items))
	for i, it := range raw.Items {
		w, err := Decode(it.Widget)
		if err != nil {
			return fmt.Errorf("repeater item %d: %w", i, err)
		}
		rp.Items = append(rp.Items, RepeaterItem{Section: it.Section, Widget: w})
	}
	return nil
}
