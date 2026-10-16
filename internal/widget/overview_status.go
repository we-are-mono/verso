// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"slices"
	"strings"

	"github.com/we-are-mono/verso/internal/i18n"
)

// overviewCount translates the entire phrase, including Slovenian's dual and
// paucal forms (i18n.Count).
func overviewCount(tr func(string) string, n int, one, two, few, many string) string {
	return i18n.Count(tr, n, one, two, few, many)
}

func tunnelCount(tr func(string) string, n int) string {
	return overviewCount(tr, n, "%d tunnel", "%d tunnels (two)", "%d tunnels (few)", "%d tunnels")
}

func deviceCount(tr func(string) string, n int) string {
	return overviewCount(tr, n, "%d device connected", "%d devices connected (two)", "%d devices connected (few)", "%d devices connected")
}

// cause is one thing wrong, as the heading would say it alone, in its tone.
type cause struct {
	Text, Tone string
}

// causes is everything that needs attention: a firewall not fully loaded and
// a web interface the internet can reach, which no card shows, then each
// card's own cause — a port or tunnel someone
// asked for without its link, a reading out of band, a board past its own
// limits. Every card counts, hidden or not, so hiding one never quiets the
// heading. The uplink is the verdict's own first question, not a cause.
func (o *Overview) causes(tr func(string) string) []cause {
	var out []cause
	switch o.FirewallState {
	case "inactive":
		out = append(out, cause{tr("The firewall is not running"), "danger"})
	case "partial":
		out = append(out, cause{tr("The firewall is only partly loaded"), "warning"})
	}
	if o.Exposed {
		out = append(out, cause{tr("The internet can reach this web interface"), "warning"})
	}
	for _, spec := range overviewCardSpecs() {
		if h := spec.head(o, tr); h.Cause != "" {
			out = append(out, cause{h.Cause, h.Tone})
		}
	}
	return out
}

// verdict is the page's one sentence, tinted by tone: the most important
// true sentence on the page. Offline outranks everything; one cause is said
// in its own words; several are counted, in danger when any of them is;
// nothing wrong on a working uplink is online; nothing known says so.
func (o *Overview) verdict(tr func(string) string) overviewVerdictView {
	v := overviewVerdictView{Tone: "neutral", Lead: tr("Your network at a glance")}
	causes := o.causes(tr)
	switch {
	case o.WANKnown && !o.WANUp:
		v.Tone, v.Lead = "danger", tr("Your network is offline")
	case len(causes) == 1:
		v.Tone, v.Lead = causes[0].Tone, causes[0].Text
	case len(causes) > 1:
		v.Tone = "warning"
		for _, c := range causes {
			if c.Tone == "danger" {
				v.Tone = "danger"
			}
		}
		v.Lead = overviewCount(tr, len(causes), "%d thing needs attention", "%d things need attention (two)", "%d things need attention (few)", "%d things need attention")
	case !o.WANKnown:
	default:
		v.Tone = "success"
		// Keep a full sentence so translations can move the accent with grammar.
		before, after, found := strings.Cut(tr("Your network is %s"), "%s")
		v.Lead = before
		if found {
			v.Accent, v.Tail = tr("online"), after
		}
	}
	// The verdict is a sentence, and ends as one: a full stop, or an
	// exclamation where it is in danger.
	mark := "."
	if v.Tone == "danger" {
		mark = "!"
	}
	if v.Accent != "" {
		v.Tail += mark
	} else {
		v.Lead += mark
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
