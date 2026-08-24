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
	"io"
)

// Widget is one node in a widget schema tree. The unexported markers seal the
// interface, enforcing the closed widget set: only this package can define a
// widget, and every widget knows how to render itself.
//
// renderInto is the render dispatch — polymorphism in place of a type switch
// (Open/Closed): the engine calls it, each widget's own file implements it, so a
// new widget never edits a shared switch. Decode below stays a switch on purpose:
// it is the one place the wire "type" strings map to concrete structs, a single
// visible catalog of the closed set.
type Widget interface {
	isWidget()
	renderInto(r *Renderer, out io.Writer, csrf string) error
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
	case "repeater":
		var rp Repeater
		if err := json.Unmarshal(data, &rp); err != nil {
			return nil, fmt.Errorf("widget: decode repeater: %w", err)
		}
		return &rp, nil
	case "conditional":
		var c Conditional
		if err := json.Unmarshal(data, &c); err != nil {
			return nil, fmt.Errorf("widget: decode conditional: %w", err)
		}
		return &c, nil
	case "modal":
		var m Modal
		if err := json.Unmarshal(data, &m); err != nil {
			return nil, fmt.Errorf("widget: decode modal: %w", err)
		}
		return &m, nil
	case "badge":
		var b Badge
		if err := json.Unmarshal(data, &b); err != nil {
			return nil, fmt.Errorf("widget: decode badge: %w", err)
		}
		return &b, nil
	case "text":
		var t Text
		if err := json.Unmarshal(data, &t); err != nil {
			return nil, fmt.Errorf("widget: decode text: %w", err)
		}
		return &t, nil
	case "row":
		var rw Row
		if err := json.Unmarshal(data, &rw); err != nil {
			return nil, fmt.Errorf("widget: decode row: %w", err)
		}
		return &rw, nil
	case "stack":
		var st Stack
		if err := json.Unmarshal(data, &st); err != nil {
			return nil, fmt.Errorf("widget: decode stack: %w", err)
		}
		return &st, nil
	case "toggle":
		var t Toggle
		if err := json.Unmarshal(data, &t); err != nil {
			return nil, fmt.Errorf("widget: decode toggle: %w", err)
		}
		return &t, nil
	case "tabs":
		var t Tabs
		if err := json.Unmarshal(data, &t); err != nil {
			return nil, fmt.Errorf("widget: decode tabs: %w", err)
		}
		return &t, nil
	case "qr":
		var q Qr
		if err := json.Unmarshal(data, &q); err != nil {
			return nil, fmt.Errorf("widget: decode qr: %w", err)
		}
		return &q, nil
	case "choice":
		var c Choice
		if err := json.Unmarshal(data, &c); err != nil {
			return nil, fmt.Errorf("widget: decode choice: %w", err)
		}
		return &c, nil
	case "wizard":
		var wz Wizard
		if err := json.Unmarshal(data, &wz); err != nil {
			return nil, fmt.Errorf("widget: decode wizard: %w", err)
		}
		return &wz, nil
	case "drawer":
		var d Drawer
		if err := json.Unmarshal(data, &d); err != nil {
			return nil, fmt.Errorf("widget: decode drawer: %w", err)
		}
		return &d, nil
	case "empty":
		var e Empty
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, fmt.Errorf("widget: decode empty: %w", err)
		}
		return &e, nil
	case "divider":
		var d Divider
		if err := json.Unmarshal(data, &d); err != nil {
			return nil, fmt.Errorf("widget: decode divider: %w", err)
		}
		return &d, nil
	case "properties":
		var p Properties
		if err := json.Unmarshal(data, &p); err != nil {
			return nil, fmt.Errorf("widget: decode properties: %w", err)
		}
		return &p, nil
	case "confirm":
		var c Confirm
		if err := json.Unmarshal(data, &c); err != nil {
			return nil, fmt.Errorf("widget: decode confirm: %w", err)
		}
		return &c, nil
	case "callout":
		var c Callout
		if err := json.Unmarshal(data, &c); err != nil {
			return nil, fmt.Errorf("widget: decode callout: %w", err)
		}
		return &c, nil
	case "link":
		var l Link
		if err := json.Unmarshal(data, &l); err != nil {
			return nil, fmt.Errorf("widget: decode link: %w", err)
		}
		return &l, nil
	case "copy":
		var c Copy
		if err := json.Unmarshal(data, &c); err != nil {
			return nil, fmt.Errorf("widget: decode copy: %w", err)
		}
		return &c, nil
	case "disclosure":
		var d Disclosure
		if err := json.Unmarshal(data, &d); err != nil {
			return nil, fmt.Errorf("widget: decode disclosure: %w", err)
		}
		return &d, nil
	case "section":
		var s Section
		if err := json.Unmarshal(data, &s); err != nil {
			return nil, fmt.Errorf("widget: decode section: %w", err)
		}
		return &s, nil
	case "code":
		var c Code
		if err := json.Unmarshal(data, &c); err != nil {
			return nil, fmt.Errorf("widget: decode code: %w", err)
		}
		return &c, nil
	default:
		return nil, fmt.Errorf("widget: unknown type %q", head.Type)
	}
}
