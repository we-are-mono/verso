// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

// Package widget models the Verso widget schema and renders it to theme-token
// HTML.
//
// The widget set is intentionally closed: the Widget interface is sealed by an
// unexported marker method, so only this package can define widget types.
// Authors compose these primitives; they do not add new ones.
package widget

import (
	"encoding/json"
	"fmt"
)

// Widget is one node in a widget schema tree. The unexported marker seals the
// interface, enforcing the closed widget set.
type Widget interface {
	isWidget()
}

// Decode parses one widget from its JSON schema representation, using the
// "type" discriminator to select the concrete widget. This switch is the one
// place a new widget type is wired in; the set is small and closed by design.
func Decode(data []byte) (Widget, error) {
	var head struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(data, &head); err != nil {
		return nil, fmt.Errorf("widget: decode type: %w", err)
	}

	switch head.Type {
	case "":
		return nil, fmt.Errorf("widget: missing \"type\" field")
	case "table":
		var t Table
		if err := json.Unmarshal(data, &t); err != nil {
			return nil, fmt.Errorf("widget: decode table: %w", err)
		}
		return &t, nil
	case "card":
		var c Card
		if err := json.Unmarshal(data, &c); err != nil {
			return nil, fmt.Errorf("widget: decode card: %w", err)
		}
		return &c, nil
	case "form":
		var f Form
		if err := json.Unmarshal(data, &f); err != nil {
			return nil, fmt.Errorf("widget: decode form: %w", err)
		}
		return &f, nil
	case "field":
		var f Field
		if err := json.Unmarshal(data, &f); err != nil {
			return nil, fmt.Errorf("widget: decode field: %w", err)
		}
		return &f, nil
	case "list":
		var l List
		if err := json.Unmarshal(data, &l); err != nil {
			return nil, fmt.Errorf("widget: decode list: %w", err)
		}
		return &l, nil
	case "raw":
		var rw Raw
		if err := json.Unmarshal(data, &rw); err != nil {
			return nil, fmt.Errorf("widget: decode raw: %w", err)
		}
		return &rw, nil
	default:
		return nil, fmt.Errorf("widget: unknown type %q", head.Type)
	}
}
