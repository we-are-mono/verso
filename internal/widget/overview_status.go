// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"fmt"
	"slices"
	"strings"
)

// overviewCount translates the entire phrase, including Slovenian's dual and
// paucal forms. Values remain arguments; no sentence is assembled from suffixes.
func overviewCount(tr func(string) string, n int, one, two, few, many string) string {
	key := many
	switch n % 100 {
	case 1:
		key = one
	case 2:
		key = two
	case 3, 4:
		key = few
	}
	pattern := tr(key)
	// Contextual plural keys have an ordinary English fallback; their context
	// markers are never displayed when a catalog is missing.
	if pattern == key {
		pattern = many
		if n == 1 {
			pattern = one
		}
	}
	return fmt.Sprintf(pattern, n)
}

func tunnelCount(tr func(string) string, n int) string {
	return overviewCount(tr, n, "%d tunnel", "%d tunnels (two)", "%d tunnels (few)", "%d tunnels")
}

func deviceCount(tr func(string) string, n int) string {
	return overviewCount(tr, n, "%d device connected", "%d devices connected (two)", "%d devices connected (few)", "%d devices connected")
}

// issues counts what needs attention: a firewall not fully loaded, a port or
// tunnel someone asked for without its link, a resource out of band. Hidden
// cards count, so hiding one never makes the verdict read "online".
func (o *Overview) issues() int {
	n := 0
	if o.FirewallState == "inactive" || o.FirewallState == "partial" {
		n++
	}
	for _, i := range o.Interfaces {
		if ((i.port() && i.inUse()) || i.tunnel()) && (i.State == "down" || i.State == "lowerlayerdown") {
			n++
		}
	}
	for _, meter := range o.SysMetrics {
		if meter.Band == "warning" || meter.Band == "danger" {
			n++
		}
	}
	return n
}

// verdict is the page's one sentence, tinted by tone: online, needs
// attention, offline, or unknown.
func (o *Overview) verdict(tr func(string) string) overviewVerdictView {
	v := overviewVerdictView{Tone: "neutral", Lead: tr("Your network at a glance")}
	switch {
	case !o.WANKnown:
	case !o.WANUp:
		v.Tone, v.Lead = "danger", tr("Your network is offline")
	case o.issues() > 0:
		v.Tone, v.Lead = "warning", tr("Your network needs attention")
	default:
		v.Tone = "success"
		// Keep a full sentence so translations can move the accent with grammar.
		before, after, found := strings.Cut(tr("Your network is %s"), "%s")
		v.Lead = before
		if found {
			v.Accent, v.Tail = tr("online"), after
		}
	}
	return v
}

// OverviewStatus is the live part of the page: the verdict and every card's
// header, hidden cards included so the Arrange shelf can wear their tone. The
// first paint and the SSE stream use the same reading; the browser changes
// text and tone, not policy.
type OverviewStatus struct {
	Lead   string               `json:"lead"`
	Accent string               `json:"accent"`
	Tail   string               `json:"tail"`
	Tone   string               `json:"tone"`
	Cards  []overviewCardStatus `json:"cards"`
}

type overviewCardStatus struct {
	ID string `json:"id"`
	cardHead
}

func (o *Overview) LiveStatus(tr func(string) string) OverviewStatus {
	v := o.verdict(tr)
	status := OverviewStatus{Lead: v.Lead, Accent: v.Accent, Tail: v.Tail, Tone: v.Tone}
	layout := o.layout()
	for _, spec := range overviewCardSpecs() {
		// A card this router does not have is neither in a column nor hidden.
		if layout.IsHidden(spec.ID) || slices.Contains(layout.Cols[0], spec.ID) || slices.Contains(layout.Cols[1], spec.ID) {
			status.Cards = append(status.Cards, overviewCardStatus{ID: spec.ID, cardHead: spec.head(o, tr)})
		}
	}
	return status
}
