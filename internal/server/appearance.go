// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"sort"
	"strings"

	"github.com/we-are-mono/verso/internal/theme"
	"github.com/we-are-mono/verso/internal/widget"
)

// appearanceSlot is the slot the system plugin leaves on General for the
// Appearance card.
const appearanceSlot = "appearance"

// fillSlots puts the shell's block in each slot a plugin's page leaves for
// one, in the shell's own words (tr); a slot the shell has no block for draws
// nothing.
func (s *Server) fillSlots(page widget.Widget, tr func(string) string) {
	for _, slot := range widget.Slots(page) {
		if slot.ID == appearanceSlot {
			slot.Fill(s.appearanceCard(tr))
		}
	}
}

// appearanceCard is the Appearance card (ADR-024): the theme in force, picked
// from Sand's three and every other theme the router has, and the one fact to
// know before picking. The card is the shell's, so it stands in a plugin's
// page already in the reader's language.
func (s *Server) appearanceCard(tr func(string) string) widget.Widget {
	now := s.themeNow()
	choice := now.choice
	if choice == "" {
		choice = theme.Sand
	}
	options := []widget.Option{
		{Value: theme.Sand, Label: tr("Sand, follows the browser")},
		{Value: theme.SandLight, Label: tr("Sand, always light")},
		{Value: theme.SandDark, Label: tr("Sand, always dark")},
	}
	var others []theme.Theme
	for id, t := range now.set {
		if id != theme.SandDark {
			others = append(others, t)
		}
	}
	sort.Slice(others, func(i, j int) bool { return label(others[i]) < label(others[j]) })
	for _, t := range others {
		options = append(options, widget.Option{Value: t.ID, Label: label(t)})
	}
	return &widget.Section{
		Title:    tr("Appearance"),
		Hairline: true,
		Children: widget.Widgets{
			&widget.Field{Name: "theme", Label: tr("Theme"), Kind: "select", Options: options, Value: choice, Key: "theme", Theme: true},
			&widget.Text{Markdown: tr("Takes effect immediately, for everyone who signs in to this router."), Translated: true},
		},
	}
}

// label is how the Appearance card names a theme: its family, then its name.
func label(t theme.Theme) string {
	return strings.TrimSpace(t.Family + " " + t.Name)
}
