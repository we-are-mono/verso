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
	NoteVerbatim bool   `json:"-"` // shell-composed annotation already localized with its live values
	CancelHref   string `json:"-"`
	Action       string `json:"-"` // shell-owned route; plugin forms always post to their own page
	// Frame is the panel this form posts back into rather than navigating, set
	// by the shell when it draws the form inside one: the drawer stays open over
	// the listing that opened it, and the answer — saved, or refused with the
	// offending controls marked — swaps in where the form was. FrameEntity is
	// the shell's entity panel, whose route Panel carries; FramePanel is a
	// plugin's own open panel, which posts to the page's own address — the one
	// the address bar already names, because an open panel is a place. Empty
	// everywhere else, which is an ordinary form that posts its page.
	Frame      string       `json:"-"`
	Panel      string       `json:"-"`
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

// The frames a form can post back into. Which one holds a form is the shell's
// to know — the plugin declared a form, and where its answer lands is the
// shell's realization of that (ADR-005 §7).
const (
	FrameEntity = "entity" // the shell's entity panel body, at the tab's own route
	FramePanel  = "panel"  // a plugin's own open panel, at the page's own address
)

// PostIntoFrame makes every form in the tree post back into the frame that holds
// it, at route — empty for a frame whose address is the page's own. A form
// already framed keeps its frame: a tree may be framed at more than one
// altitude, and the first call to reach a form is the one closest to it.
func PostIntoFrame(w Widget, frame, route string) {
	Walk(w, func(n Widget) {
		if form, ok := n.(*Form); ok && form.Frame == "" {
			form.Frame, form.Panel = frame, route
		}
	})
}

func (*Form) isWidget() {}

func (f *Form) children() []Widget { return f.Fields }

func (f *Form) prune(keep func(Widget) bool) { f.Fields = pruneList(f.Fields, keep) }

// confirmDriven reports whether the form's own contents already carry its submit.
// A confirm renders a submit button of its own, so a generated Save beside it
// would offer the same action twice — once guarded, once not. A form that wants
// both states its Save label explicitly.
func (f *Form) ConfirmDriven() bool {
	found := false
	for _, field := range f.Fields {
		Walk(field, func(n Widget) {
			if _, ok := n.(*Confirm); ok {
				found = true
			}
		})
	}
	return found
}

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
	CancelHref string
	Action     string
	Frame      string
	Panel      string
	Multipart  bool
	AutoSubmit bool
	Inline     bool
	Compact    bool
	Search     bool
	Page       bool
	Dirty      bool
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
	// A page form's submit is what stages what it holds. A plugin names it in
	// the object's own verb ("Add rule"); a page form with no label is left
	// buttonless here, because the label is the gateway's to supply — one
	// default, stated once. A non-page form defaults to "Save".
	if submit == "" && f.Style != "page" && !f.NoSubmit && !f.AutoSubmit && !f.ConfirmDriven() {
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
		CancelHref: f.CancelHref, Action: f.Action, Frame: f.Frame, Panel: f.Panel, Multipart: f.Multipart, AutoSubmit: f.AutoSubmit,
		Inline: f.Style == "inline" || f.Style == "inline-compact", Compact: f.Style == "inline-compact", Search: f.Style == "search", Page: f.Style == "page" || f.Style == "settings", Dirty: f.Style == "settings", Icon: f.Icon, Note: note,
		Submit: submit, Success: f.Success, Error: f.Error, CSRFToken: csrf,
		Actions: f.Actions, Fields: fields,
	})
}
