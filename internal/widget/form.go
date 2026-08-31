// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"bytes"
	"encoding/json"
	"html/template"
	"io"
)

// Form is an interactive container: it renders its fields inside a POST form that
// submits back to the same page (the shell's /plugins/<id>/ URL). Fields decode
// recursively through Decode, so a form composes the closed set — typically
// fields and lists, but any widget nests.
type Form struct {
	Action     string       `json:"-"`               // shell-owned route; plugin forms always post to their own page
	Multipart  bool         `json:"-"`               // shell-owned file-transfer encoding
	NoSubmit   bool         `json:"-"`               // shell-owned forms may be driven by a child control
	AutoSubmit bool         `json:"-"`               // submit when a file is selected
	Style      string       `json:"style,omitempty"` // "" (stacked) | "inline" | "inline-compact" | "search" (one compound search field + inset submit) | "page"
	Icon       string       `json:"icon,omitempty"`  // optional leading icon on the submit button, by Lucide name
	Note       string       `json:"note,omitempty"`  // quiet annotation beside the buttons (inline) or under them (stacked); Markdown, sanitized like text
	Submit     string       // submit button label (default "Save")
	Success    string       // optional message shown after a successful save
	Error      string       // optional error not tied to a single field, shown above the fields
	Actions    []FormAction // secondary submit buttons besides Save (below)
	Fields     []Widget     // form contents
}

// FormAction is a secondary submit button: it submits the form — all its fields —
// with an `_action` marker the plugin reads, so the plugin can compute on the
// submitted values and re-render. It is the plugin-computed round-trip (ADR-005 §7):
// the shell owns the button and forwards the submission; the plugin owns the
// computation (e.g. generating a keypair) and returns fresh schema, not a save.
type FormAction struct {
	Label  string `json:"label"`
	Action string `json:"action"`         // posted as _action=<Action>
	Icon   string `json:"icon,omitempty"` // optional leading icon, by Lucide name
}

func (*Form) isWidget() {}

func (f *Form) children() []Widget { return f.Fields }

// UnmarshalJSON decodes a form's fields recursively through Decode, so an unknown
// field type fails loudly rather than vanishing.
func (f *Form) UnmarshalJSON(data []byte) error {
	var raw struct {
		Style   string            `json:"style"`
		Submit  string            `json:"submit"`
		Icon    string            `json:"icon"`
		Note    string            `json:"note"`
		Success string            `json:"success"`
		Error   string            `json:"error"`
		Actions []FormAction      `json:"actions"`
		Fields  []json.RawMessage `json:"fields"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	f.Style = raw.Style
	f.Submit = raw.Submit
	f.Icon = raw.Icon
	f.Note = raw.Note
	f.Success = raw.Success
	f.Error = raw.Error
	f.Actions = raw.Actions
	fields, err := decodeChildren(raw.Fields, "form field")
	if err != nil {
		return err
	}
	f.Fields = fields
	return nil
}

// formView is the form template's model: its fields pre-rendered to trusted HTML,
// plus the resolved submit label and the CSRF token threaded in by the renderer.
type formView struct {
	Action     string
	Multipart  bool
	AutoSubmit bool
	Inline     bool
	Compact    bool
	Search     bool
	Page       bool
	Icon       string
	Note       template.HTML
	Submit     string
	Success    string
	Error      string
	CSRFToken  string
	Actions    []FormAction
	Fields     []template.HTML
}

// renderInto renders each field through the renderer, keeping composition in Go and
// the template a dumb shell (as card does).
func (f *Form) renderInto(r *Renderer, out io.Writer, csrf string) error {
	fields, err := r.renderChildren(f.Fields, csrf)
	if err != nil {
		return err
	}
	submit := f.Submit
	if submit == "" && f.Style != "page" && !f.NoSubmit && !f.AutoSubmit {
		submit = r.tr("Save")
	}
	var note template.HTML
	if f.Note != "" {
		var buf bytes.Buffer
		if err := r.md.Convert([]byte(f.Note), &buf); err != nil {
			return err
		}
		note = template.HTML(buf.String())
	}
	return r.execute(out, "form.html.tmpl", formView{
		Action: f.Action, Multipart: f.Multipart, AutoSubmit: f.AutoSubmit,
		Inline: f.Style == "inline" || f.Style == "inline-compact", Compact: f.Style == "inline-compact", Search: f.Style == "search", Page: f.Style == "page", Icon: f.Icon, Note: note,
		Submit: submit, Success: f.Success, Error: f.Error, CSRFToken: csrf,
		Actions: f.Actions, Fields: fields,
	})
}
