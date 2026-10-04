// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"bytes"
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
	Frame      string `json:"-"`
	Panel      string `json:"-"`
	Multipart  bool   `json:"-"`               // shell-owned file-transfer encoding
	NoSubmit   bool   `json:"-"`               // shell-owned forms may be driven by a child control
	AutoSubmit bool   `json:"-"`               // submit when a file is selected
	Style      string `json:"style,omitempty"` // "" (stacked) | "inline" | "inline-compact" | "search" (one compound search field + inset submit) | "page"
	Icon       string `json:"icon,omitempty"`  // optional leading icon on the submit button, by Lucide name
	Note       string `json:"note,omitempty"`  // quiet annotation beside the buttons (inline) or under them (stacked); Markdown, sanitized like text
	// Target is where the options this form's controls write live,
	// "config.section", when the form writes one uci section — said once here
	// rather than on each control (MarkStaged).
	Target  string       `json:"target,omitempty"`
	Submit  string       `json:"submit"`  // submit button label: the act and its object, "Save rule" (default "Save changes")
	Success string       `json:"success"` // optional message shown after a successful save
	Error   string       `json:"error"`   // optional error not tied to a single field, shown above the fields
	Actions []FormAction `json:"actions"` // secondary submit buttons besides Save (below)
	Fields  Widgets      `json:"fields"`  // form contents
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
	// Sectioned is a form that commits one section of a page rather than the
	// page: its Save stands under its fields with no rule, because the next
	// section's rule is the line that closes it.
	Sectioned bool
	JoinsCode bool // actions finish the configuration card above them
	// ClosesPage is the page's own form, its sections inside it: its Save
	// commits the whole page, so the rule above it is a section rule of the
	// page's.
	ClosesPage bool
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
	// default, stated once. A non-page form that names no act defaults to
	// what its button does, "Save changes" — never a bare "Save"; a plugin
	// names the object ("Save rule") wherever it knows it.
	if submit == "" && f.Style != "page" && !f.NoSubmit && !f.AutoSubmit && !f.ConfirmDriven() {
		submit = r.tr("Save changes")
	}
	var note template.HTML
	if f.Note != "" {
		var buf bytes.Buffer
		if err := r.md.Convert([]byte(f.Note), &buf); err != nil {
			return err
		}
		note = template.HTML(buf.String())
	}
	page := f.Style == "page" || f.Style == "settings"
	return r.execute(out, "form.html.tmpl", formView{
		CancelHref: f.CancelHref, Action: f.Action, Frame: f.Frame, Panel: f.Panel, Multipart: f.Multipart, AutoSubmit: f.AutoSubmit,
		Inline: f.Style == "inline" || f.Style == "inline-compact", Compact: f.Style == "inline-compact", Search: f.Style == "search", Page: page, Dirty: f.Style == "settings",
		Sectioned: r.depth > 0 && f.Frame == "", ClosesPage: page && r.depth == 0 && f.Frame == "", Icon: f.Icon, Note: note,
		JoinsCode: EndsWithCode(f),
		Submit:    submit, Success: f.Success, Error: f.Error, CSRFToken: csrf,
		Actions: f.Actions, Fields: fields,
	})
}
