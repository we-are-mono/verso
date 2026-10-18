// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"net/http"

	"github.com/we-are-mono/verso/internal/plugin"
	"github.com/we-are-mono/verso/internal/widget"
)

// overviewCards is the Overview's cards slot.
const overviewCards = "shell.overview.cards"

// overviewPlacements are the installed plugins' Overview cards (ADR-026), in
// their slot's order, each where the default layout places it. They name
// every card an installed plugin has, drawn this time or not, so a layout
// keeps their places and drops only those of plugins no longer installed.
func (s *Server) overviewPlacements() []widget.OverviewPlacement {
	var out []widget.OverviewPlacement
	for _, e := range orderSlot(nil, s.slotFills(overviewCards)) {
		p := widget.OverviewPlacement{ID: e.Placement.Name(), Anchor: e.Placement.Contribution.Position.Anchor}
		switch e.Placement.Contribution.Position.Where {
		case plugin.First:
			p.Where = "first"
		case plugin.Before:
			p.Where = "before"
		case plugin.After:
			p.Where = "after"
		}
		out = append(out, p)
	}
	return out
}

// overviewContributions asks every installed plugin for its Overview card at
// once, hidden ones too, since their causes reach the heading: the cards that
// answered, and the ids of those that did not, whose places hold.
func (s *Server) overviewContributions(r *http.Request) (drawn []widget.OverviewContribution, kept []string) {
	var jobs []*slotJob
	for _, e := range orderSlot(nil, s.slotFills(overviewCards)) {
		jobs = append(jobs, &slotJob{slot: openedSlot{Name: overviewCards, Kind: "overview-card"}, placement: *e.Placement})
	}
	s.askAll(r, jobs, pluginCall{page: r.URL.Path})
	for _, job := range jobs {
		if job.card.ID != "" {
			drawn = append(drawn, job.card)
		} else {
			kept = append(kept, job.placement.Name())
		}
	}
	return drawn, kept
}

// cardHeads are the cards' headers alone, which a session keeps for its
// stream.
func cardHeads(cards []widget.OverviewContribution) []widget.OverviewContribution {
	heads := make([]widget.OverviewContribution, 0, len(cards))
	for _, c := range cards {
		c.Body = nil
		heads = append(heads, c)
	}
	return heads
}
