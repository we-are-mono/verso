// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"encoding/json"
	"fmt"
)

// Card is a titled container that nests other widgets. It is the composition
// primitive: the schema's ability to nest — not merely render a leaf — rests on
// it. Children decode recursively through Decode, so a card may hold any widget,
// itself included, and the closed set is enforced at every level.
type Card struct {
	Title    string
	Children []Widget
}

func (*Card) isWidget() {}

// UnmarshalJSON decodes a card's title and children, recursing through Decode so
// an unknown child type fails here rather than silently vanishing.
func (c *Card) UnmarshalJSON(data []byte) error {
	var raw struct {
		Title    string            `json:"title"`
		Children []json.RawMessage `json:"children"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	c.Title = raw.Title
	c.Children = make([]Widget, 0, len(raw.Children))
	for i, rc := range raw.Children {
		child, err := Decode(rc)
		if err != nil {
			return fmt.Errorf("card child %d: %w", i, err)
		}
		c.Children = append(c.Children, child)
	}
	return nil
}
