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
// new widget never edits a shared switch. Decode's catalog below stays one table
// on purpose: it is the one place the wire "type" strings map to concrete
// structs, a single visible list of the closed set.
type Widget interface {
	isWidget()
	renderInto(r *Renderer, out io.Writer, csrf string) error
	// children returns every widget nested beneath this one — a section's
	// control, a table row's drawer contents, a repeater item's subtree. It is
	// the one structural seam every tree pass recurses through (Walk), declared
	// beside the widget so no walk can silently miss a container. Leaves return
	// nil; widgets composed only at render time (Overview) are leaves here.
	children() []Widget
}

// Widgets is a container's children. Each element decodes through Decode, so an
// unknown child type fails loudly rather than silently vanishing — and a
// container whose children are Widgets needs no decoder of its own.
type Widgets []Widget

// UnmarshalJSON decodes each element through Decode, naming the failing one.
func (ws *Widgets) UnmarshalJSON(data []byte) error {
	var raw []json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	children := make(Widgets, 0, len(raw))
	for i, rc := range raw {
		w, err := Decode(rc)
		if err != nil {
			return fmt.Errorf("child %d: %w", i, err)
		}
		children = append(children, w)
	}
	*ws = children
	return nil
}

// decodeOptional decodes a lone widget a schema may leave out: absent or null
// is no widget, anything else decodes through Decode.
func decodeOptional(raw json.RawMessage) (Widget, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	return Decode(raw)
}

// catalog maps each wire "type" to a fresh widget of that type. It is the one
// place a new widget type is wired in; the set is small and closed by design.
var catalog = map[string]func() Widget{
	"table":       func() Widget { return new(Table) },
	"card":        func() Widget { return new(Card) },
	"form":        func() Widget { return new(Form) },
	"field":       func() Widget { return new(Field) },
	"list":        func() Widget { return new(List) },
	"raw":         func() Widget { return new(Raw) },
	"repeater":    func() Widget { return new(Repeater) },
	"conditional": func() Widget { return new(Conditional) },
	"when":        func() Widget { return new(When) },
	"conditions":  func() Widget { return new(Conditions) },
	"switch":      func() Widget { return new(Switch) },
	"modal":       func() Widget { return new(Modal) },
	"text":        func() Widget { return new(Text) },
	"stack":       func() Widget { return new(Stack) },
	"empty":       func() Widget { return new(Empty) },
	"divider":     func() Widget { return new(Divider) },
	"properties":  func() Widget { return new(Properties) },
	"collection":  func() Widget { return new(Collection) },
	"confirm":     func() Widget { return new(Confirm) },
	"callout":     func() Widget { return new(Callout) },
	"link":        func() Widget { return new(Link) },
	"button":      func() Widget { return new(Button) },
	"disclosure":  func() Widget { return new(Disclosure) },
	"section":     func() Widget { return new(Section) },
	"code":        func() Widget { return new(Code) },
	"stat":        func() Widget { return new(Stat) },
	"grid":        func() Widget { return new(Grid) },
	"meter":       func() Widget { return new(Meter) },
	"ports":       func() Widget { return new(Ports) },
	"settings":    func() Widget { return new(Settings) },
	"actionbar":   func() Widget { return new(ActionBar) },
	"filter":      func() Widget { return new(Filter) },
}

// Decode parses one widget from its JSON schema representation, using the
// "type" discriminator to select the concrete widget from the catalog.
func Decode(data []byte) (Widget, error) {
	var head struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(data, &head); err != nil {
		return nil, fmt.Errorf("widget: decode type: %w", err)
	}
	if head.Type == "" {
		return nil, fmt.Errorf("widget: missing \"type\" field")
	}
	fresh, ok := catalog[head.Type]
	if !ok {
		return nil, fmt.Errorf("widget: unknown type %q", head.Type)
	}
	w := fresh()
	if err := json.Unmarshal(data, w); err != nil {
		return nil, fmt.Errorf("widget: decode %s: %w", head.Type, err)
	}
	return w, nil
}
