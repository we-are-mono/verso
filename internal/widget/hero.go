// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"encoding/json"
	"html/template"
	"io"
)

// Hero is the mood-setting verdict banner — a large, coloured panel that states one
// plain thing at the top of a page ("Everything's good", "Your internet is down"),
// with a glyph, a headline, a supporting line, and an optional call-to-action. It is
// generic: any page can open with a verdict. The children are the CTA slot, composed
// from ordinary widgets (typically one button-styled link), so the hero adds no
// bespoke "action" concept — it just arranges what it's given (ADR-005).
//
// The semantic variant chooses the palette and a default glyph; Icon overrides the
// glyph when a calmer or louder cue fits (e.g. a check on an amber "all good, one
// thing to check"). The title is set in the serif display face so the verdict reads
// as a human line, not chrome.
type Hero struct {
	Variant  string
	Icon     string
	Title    string
	Body     string
	Children []Widget
}

func (*Hero) isWidget() {}

func (h *Hero) children() []Widget { return h.Children }

func (h *Hero) UnmarshalJSON(data []byte) error {
	var raw struct {
		Variant  string            `json:"variant"`
		Icon     string            `json:"icon"`
		Title    string            `json:"title"`
		Body     string            `json:"body"`
		Children []json.RawMessage `json:"children"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	h.Variant, h.Icon, h.Title, h.Body = raw.Variant, raw.Icon, raw.Title, raw.Body
	children, err := decodeChildren(raw.Children, "hero child")
	if err != nil {
		return err
	}
	h.Children = children
	return nil
}

// heroView is the template model: the variant (drives palette classes), the resolved
// glyph name, the text, and the pre-rendered CTA children.
type heroView struct {
	Variant  string
	Icon     string
	Title    string
	Body     string
	Children []template.HTML
}

func (h *Hero) renderInto(r *Renderer, out io.Writer, csrf string) error {
	children, err := r.renderChildren(h.Children, csrf)
	if err != nil {
		return err
	}
	return r.execute(out, "hero.html.tmpl", heroView{
		Variant:  h.Variant,
		Icon:     heroIcon(h.Variant, h.Icon),
		Title:    h.Title,
		Body:     h.Body,
		Children: children,
	})
}

// heroIcon resolves the glyph: an explicit Icon wins, otherwise a sensible default
// per variant. The shell owns this mapping (semantic intent → glyph, ADR-005).
func heroIcon(variant, override string) string {
	if override != "" {
		return override
	}
	switch variant {
	case "good":
		return "check"
	case "danger", "warning":
		return "triangle-alert"
	default:
		return "info"
	}
}
