// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"encoding/json"
	"html/template"
	"io"
)

// When presents fields belonging to one value of another form control — or to
// any of several, when Value lists them space-separated ("sae sae-mixed psk2").
// Inactive branches are disabled as well as hidden, so they neither validate nor
// submit.
type When struct {
	Name     string   `json:"name"`
	Value    string   `json:"value"`
	Active   bool     `json:"active"`
	Children []Widget `json:"children"`
}

func (*When) isWidget()                      {}
func (w *When) children() []Widget           { return w.Children }
func (w *When) prune(keep func(Widget) bool) { w.Children = pruneList(w.Children, keep) }
func (w *When) UnmarshalJSON(data []byte) error {
	var raw struct {
		Name, Value string
		Active      bool
		Children    []json.RawMessage
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	w.Name, w.Value, w.Active = raw.Name, raw.Value, raw.Active
	var err error
	w.Children, err = decodeChildren(raw.Children, "when child")
	return err
}
func (w *When) renderInto(r *Renderer, out io.Writer, csrf string) error {
	body, err := r.renderChildren(w.Children, csrf)
	if err != nil {
		return err
	}
	return r.execute(out, "when.html.tmpl", struct {
		Name, Value string
		Active      bool
		Body        []template.HTML
	}{w.Name, w.Value, w.Active, body})
}
