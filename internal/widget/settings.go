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
	Style string         `json:"style,omitempty"` // "" (card, default) | "plain" — rows only, for embedding in a drawer or form
	Items []SettingsItem `json:"items"`
	Seam  *SettingsSeam  `json:"seam,omitempty"`
}

// SettingsSeam folds a card's long tail of options behind a collapsed block
// inside the same card — the everyday options stay visible, the rare ones are
// present and honest but do not carry the card.
type SettingsSeam struct {
	Summary string         `json:"summary"`
	Items   []SettingsItem `json:"items"`
}

// SettingsItem is one option row. Exactly one of Toggle, Pills, or Value should
// carry the trailing state; a row with none is informational.
type SettingsItem struct {
	Title  string          `json:"title"`
	Desc   string          `json:"desc,omitempty"`
	Code   string          `json:"code,omitempty"`  // the underlying option name, e.g. "synflood_protect"
	Value  string          `json:"value,omitempty"` // a read-out value (mono), e.g. "lan", "1000"
	Toggle *SettingsToggle `json:"toggle,omitempty"`
	Pills  []Badge         `json:"pills,omitempty"`
}

// SettingsToggle is the row's switch: its current state and the form name it
// posts under.
type SettingsToggle struct {
	Name string `json:"name,omitempty"`
	On   bool   `json:"on,omitempty"`
}

func (*Settings) isWidget() {}

// settingsView carries the card style and its rows; each item's pills are
// pre-rendered to trusted HTML, the rest is plain text the template escapes.
type settingsView struct {
	Plain       bool
	Items       []settingsItemView
	SeamSummary string
	SeamItems   []settingsItemView
}

type settingsItemView struct {
	SettingsItem
	PillsHTML []template.HTML
}

func (s *Settings) itemViews(r *Renderer, items []SettingsItem) ([]settingsItemView, error) {
	out := make([]settingsItemView, 0, len(items))
	for _, it := range items {
		iv := settingsItemView{SettingsItem: it}
		for i := range it.Pills {
			var b strings.Builder
			if err := r.execute(&b, "badge.html.tmpl", &it.Pills[i]); err != nil {
				return nil, err
			}
			iv.PillsHTML = append(iv.PillsHTML, template.HTML(b.String()))
		}
		out = append(out, iv)
	}
	return out, nil
}

func (s *Settings) renderInto(r *Renderer, out io.Writer, _ string) error {
	v := settingsView{Plain: s.Style == "plain"}
	var err error
	if v.Items, err = s.itemViews(r, s.Items); err != nil {
		return err
	}
	if s.Seam != nil {
		v.SeamSummary = s.Seam.Summary
		if v.SeamItems, err = s.itemViews(r, s.Seam.Items); err != nil {
			return err
		}
	}
	return r.execute(out, "settings.html.tmpl", v)
}
