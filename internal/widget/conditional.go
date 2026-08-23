// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"encoding/json"
	"fmt"
)

// Conditional is a behavioural widget: a field-set shown only when its controlling
// toggle is on (ADR-005 §7). The plugin declares the intent — this toggle gates
// these fields — and the shell realizes it in pure CSS (no JavaScript, no
// round-trip), because show/hide has no state to persist. The toggle posts its own
// value, so the plugin can read it when interpreting a save.
type Conditional struct {
	Name    string   // form field name of the controlling toggle
	Label   string   // the toggle's label
	Checked bool     // whether the toggle starts on (the field-set starts visible)
	Fields  []Widget // the field-set revealed when the toggle is on
}

func (*Conditional) isWidget() {}

// UnmarshalJSON decodes the gated fields recursively through Decode, so an unknown
// field type fails loudly rather than vanishing.
func (c *Conditional) UnmarshalJSON(data []byte) error {
	var raw struct {
		Name    string            `json:"name"`
		Label   string            `json:"label"`
		Checked bool              `json:"checked"`
		Fields  []json.RawMessage `json:"fields"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	c.Name = raw.Name
	c.Label = raw.Label
	c.Checked = raw.Checked
	c.Fields = make([]Widget, 0, len(raw.Fields))
	for i, rf := range raw.Fields {
		w, err := Decode(rf)
		if err != nil {
			return fmt.Errorf("conditional field %d: %w", i, err)
		}
		c.Fields = append(c.Fields, w)
	}
	return nil
}
