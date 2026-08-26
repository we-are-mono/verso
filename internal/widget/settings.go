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
}

// SettingsItem is one option row. Exactly one of Toggle or Pills should carry
// the trailing state; a row with neither is informational.
type SettingsItem struct {
	Title  string          `json:"title"`
	Desc   string          `json:"desc,omitempty"`
	Code   string          `json:"code,omitempty"` // the underlying option name, e.g. "synflood_protect"
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
	Plain bool
	Items []settingsItemView
}

type settingsItemView struct {
	SettingsItem
	PillsHTML []template.HTML
}

func (s *Settings) renderInto(r *Renderer, out io.Writer, _ string) error {
	v := settingsView{Plain: s.Style == "plain", Items: make([]settingsItemView, 0, len(s.Items))}
	for _, it := range s.Items {
		iv := settingsItemView{SettingsItem: it}
		for i := range it.Pills {
			var b strings.Builder
			if err := r.execute(&b, "badge.html.tmpl", &it.Pills[i]); err != nil {
				return err
			}
			iv.PillsHTML = append(iv.PillsHTML, template.HTML(b.String()))
		}
		v.Items = append(v.Items, iv)
	}
	return r.execute(out, "settings.html.tmpl", v)
}
