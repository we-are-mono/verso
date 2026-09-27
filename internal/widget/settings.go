// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"html/template"
	"io"
	"strings"
)

// Settings is a card of option rows — the "config defaults" pattern: each row a
// plainly-named option with a one-line description, the underlying option name
// as a mono code chip, and its state on the right — a switch for an on/off
// option, or value pills for a row that reads rather than toggles (a policy
// triplet). Generic by design: any options block (firewall defaults, Wi-Fi
// advanced, DHCP options) is this shape.
type Settings struct {
	Style string `json:"style,omitempty"` // "" (bare rows on the page, default; "plain" is its legacy alias) | "card" — boxed, for the select places that earn a card
	Title string `json:"title,omitempty"` // optional group label above the rows
	Meta  string `json:"meta,omitempty"`  // optional quiet detail on the title row's right, e.g. a subnet · live count
	// Condensed lowers the row padding only — same anatomy, tighter, for a list
	// of many short facts.
	Condensed bool           `json:"condensed,omitempty"`
	Items     []SettingsItem `json:"items"`
	Seam      *SettingsSeam  `json:"seam,omitempty"`
}

// SettingsSeam folds a card's long tail of options behind a collapsed block
// inside the same card — the everyday options stay visible, the rare ones are
// present and honest but do not carry the card.
type SettingsSeam struct {
	Summary string         `json:"summary"`
	Items   []SettingsItem `json:"items"`
}

// seamMinimum is how many rows a seam has to hold before folding them is worth
// what it costs. The fold's own summary line occupies the space one hidden row
// would, so under three rows the block saves no height and charges a click for
// it: those rows render in place and the card draws no fold. The rule is a
// render decision — the declared schema keeps its seam, because what a plugin
// considers its long tail stays true whatever the card does with it.
const seamMinimum = 3

// SettingsItem is one option row. Exactly one of Toggle, Pills, or Value should
// carry the trailing state; a row with none is informational. A Value with a
// Name is edited in place: the read-out renders as a borderless input posting
// under that name — the value on screen is the control.
type SettingsItem struct {
	Title string `json:"title"`
	Desc  string `json:"desc,omitempty"`
	Code  string `json:"code,omitempty"`  // the underlying option name, e.g. "synflood_protect"
	Value string `json:"value,omitempty"` // a read-out value (mono), e.g. "lan", "1000"
	Name  string `json:"name,omitempty"`  // form name; makes Value an in-place input
	// Inline makes the in-place value a manage-page control that stages its own
	// change ([[inline-settings-commit-model]]): the
	// value shows read-only (as text, a square-pen to edit) and, on commit —
	// ✓/Enter/blur — posts just this one option, the counterpart of a row switch
	// for a value. A free-text field has no natural "done", so the ✓ is it: the
	// commit both stages and validates. Without Inline, Value+Name is an
	// always-open input a surrounding form submits. Only meaningful with Name.
	Inline bool `json:"inline,omitempty"`
	// Datatype is the shape an inline value must have ("hostname", …); the shell
	// validates it on blur before staging (inline-settings-commit-model). Only
	// meaningful with Inline.
	Datatype string          `json:"datatype,omitempty"`
	Toggle   *SettingsToggle `json:"toggle,omitempty"`
	Pills    []Badge         `json:"pills,omitempty"`
	// Staged is the shell's word that this row's option (Code, in the form or
	// section around the block) waits to be applied.
	Staged bool `json:"-"`
}

// edits reports whether the row is a setting someone changes here — a value
// posted under a name, or a switch — rather than a fact that only reads.
func (it *SettingsItem) edits() bool {
	return it.Name != "" || it.Toggle != nil
}

// SettingsToggle is the row's switch: its current state and the form name it
// posts under.
type SettingsToggle struct {
	Name string `json:"name,omitempty"`
	On   bool   `json:"on,omitempty"`
}

func (*Settings) isWidget() {}

// children surfaces the rows' pill badges, so the walk covers their text the
// same way it covers a property's status badge.
func (s *Settings) children() []Widget {
	var out []Widget
	collect := func(items []SettingsItem) {
		for i := range items {
			for p := range items[i].Pills {
				out = append(out, &items[i].Pills[p])
			}
		}
	}
	collect(s.Items)
	if s.Seam != nil {
		collect(s.Seam.Items)
	}
	return out
}

// settingsView carries the presentation and the rows; each item's pills are
// pre-rendered to trusted HTML, the rest is plain text the template escapes.
type settingsView struct {
	Card        bool
	Title       string
	Meta        string
	Items       []settingsItemView
	SeamSummary string
	SeamItems   []settingsItemView
}

// settingsItemView is one row as the template draws it: an editing row is a
// setting's row (Frame, drawn by verso-field); a row that reads keeps the
// listing's compact geometry and its pills.
type settingsItemView struct {
	SettingsItem
	Frame     template.HTML
	PillsHTML []template.HTML
	Condensed bool
}

// control is the row's checkbox. A switch with no posted name has no id for
// the row's label to point at, so it carries the row's title itself.
func (it *SettingsItem) control() switchControl {
	c := switchControl{Name: it.Toggle.Name, On: it.Toggle.On}
	if c.Name == "" {
		c.Label = it.Title
	} else if it.Desc != "" {
		c.Described = c.Name + "-desc"
	}
	return c
}

// DescribedBy is what an in-place value is described by: the description kept
// under it, then the refusal slot its own check writes into.
func (it *SettingsItem) DescribedBy() string {
	ids := make([]string, 0, 2)
	if it.Desc != "" {
		ids = append(ids, it.Name+"-desc")
	}
	if it.Inline {
		ids = append(ids, it.Name+"-error")
	}
	return strings.Join(ids, " ")
}

// frame is an editing row as one setting's row: its title is the label, its
// option the key chip, and its description stays in view under it. A value
// that stages itself tracks no form change; its own commit does the staging.
func (it *SettingsItem) frame() fieldFrame {
	f := fieldFrame{
		Label: fieldLabel{For: it.Name, Label: it.Title, Key: it.Code, Staged: it.Staged},
		Desc:  it.Desc, Class: "last:pb-0",
	}
	if it.Name != "" {
		f.Change = fieldChange{Track: !it.Inline, Name: it.Name, Label: it.Title, Kind: "text"}
		f.Measure = (&Field{Key: it.Code, Datatype: it.Datatype}).ControlMeasure()
		f.Inline = it.Inline
		return f
	}
	f.Label.For, f.Label.Group = it.Toggle.Name, it.Toggle.Name == ""
	f.Change = fieldChange{Track: it.Toggle.Name != "", Name: it.Toggle.Name, Label: it.Title, Kind: "toggle"}
	f.Toggle = true
	return f
}

func (s *Settings) itemViews(r *Renderer, items []SettingsItem) ([]settingsItemView, error) {
	out := make([]settingsItemView, 0, len(items))
	for _, it := range items {
		iv := settingsItemView{SettingsItem: it, Condensed: s.Condensed}
		if it.edits() {
			var b strings.Builder
			var err error
			if it.Name != "" {
				err = r.renderFrame(&b, "settings.value", &it, it.frame())
			} else {
				err = r.renderFrame(&b, "switch.row", it.control(), it.frame())
			}
			if err != nil {
				return nil, err
			}
			iv.Frame = template.HTML(b.String())
		}
		for p := range it.Pills {
			var b strings.Builder
			if err := r.execute(&b, "badge.html.tmpl", &it.Pills[p]); err != nil {
				return nil, err
			}
			iv.PillsHTML = append(iv.PillsHTML, template.HTML(b.String()))
		}
		out = append(out, iv)
	}
	return out, nil
}

func (s *Settings) renderInto(r *Renderer, out io.Writer, _ string) error {
	v := settingsView{Card: s.Style == "card", Title: s.Title, Meta: s.Meta}
	items, folded := s.rows()
	var err error
	if v.Items, err = s.itemViews(r, items); err != nil {
		return err
	}
	if len(folded) > 0 {
		v.SeamSummary = s.Seam.Summary
		if v.SeamItems, err = s.itemViews(r, folded); err != nil {
			return err
		}
	}
	return r.execute(out, "settings.html.tmpl", v)
}

// rows splits the card into what it shows and what it folds. A seam too short to
// earn its fold (seamMinimum) gives its rows back to the card, where they render
// as ordinary rows — carrying their controls and change-tracking hooks like every
// other row, since they render through the same one.
func (s *Settings) rows() (items, folded []SettingsItem) {
	if s.Seam == nil {
		return s.Items, nil
	}
	if len(s.Seam.Items) >= seamMinimum {
		return s.Items, s.Seam.Items
	}
	return append(append(make([]SettingsItem, 0, len(s.Items)+len(s.Seam.Items)), s.Items...), s.Seam.Items...), nil
}
