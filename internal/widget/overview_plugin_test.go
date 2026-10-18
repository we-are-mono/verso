// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"encoding/base64"
	"regexp"
	"slices"
	"strings"
	"testing"
)

func certificateCard() OverviewContribution {
	return OverviewContribution{
		ID: "acme.card", Title: "Certificate", Icon: "shield-check",
		Tone: "success", Summary: "expires in 52 d",
		Href: "/plugins/acme/", Body: &Properties{Items: []Property{{Label: "Domain", Value: "router.example.com", Verbatim: true}}},
	}
}

func allCardOrder(page string) []string {
	var ids []string
	for _, m := range regexp.MustCompile(`data-overview-(?:card|kept)="([a-z0-9._-]+)"`).FindAllStringSubmatch(page, -1) {
		ids = append(ids, m[1])
	}
	return ids
}

func TestAPluginCardHasTheShellCardsAnatomy(t *testing.T) {
	o := fullOverview()
	card := certificateCard()
	o.Contributed = []OverviewContribution{card}
	o.Layout = DefaultOverviewLayout(OverviewPlacement{ID: "acme.card", Where: "after", Anchor: "system"}).Without("hardware")
	o.Layout.Closed = []string{"acme.card"}
	got := render(t, newRenderer(t), o)
	start := strings.Index(got, `data-overview-card="acme.card"`)
	if start < 0 {
		t.Fatalf("no Certificate card:\n%s", got)
	}
	article := got[start:]
	article = article[:strings.Index(article, "</article>")]
	for _, want := range []string{
		"Certificate", "expires in 52 d", "data-closed", `href="/plugins/acme/"`, `aria-label="Open Certificate"`,
		"data-overview-grip", "data-overview-hide", "router.example.com",
	} {
		if !strings.Contains(article, want) {
			t.Errorf("plugin card missing %q", want)
		}
	}
	if !strings.Contains(strings.Join(allCardOrder(got), " "), "system acme.card") {
		t.Errorf("order = %v, want the Certificate card after System", allCardOrder(got))
	}
}

func TestAPluginCardsCauseIsTheVerdict(t *testing.T) {
	for _, hidden := range []bool{false, true} {
		o := fullOverview()
		card := certificateCard()
		card.Tone, card.Status, card.Cause = "warning", "Renewal failed", "The certificate could not be renewed"
		o.Contributed = []OverviewContribution{card}
		o.Layout = DefaultOverviewLayout(OverviewPlacement{ID: "acme.card"}).Without("hardware")
		if hidden {
			o.Layout = o.Layout.Without("acme.card")
			o.Layout.Hidden = []string{"acme.card"}
		}
		v := o.verdict(func(s string) string { return s })
		if v.Lead != "The certificate could not be renewed." || v.Tone != "warning" {
			t.Errorf("hidden=%v: verdict = %+v, want the plugin card's cause", hidden, v)
		}
		status := o.LiveStatus(func(s string) string { return s })
		if !slices.ContainsFunc(status.Cards, func(c overviewCardStatus) bool { return c.ID == "acme.card" && c.Status == "Renewal failed" }) {
			t.Errorf("hidden=%v: the live status does not carry the plugin card's header", hidden)
		}
	}
}

func TestAPluginCardThatDidNotDrawKeepsItsPlace(t *testing.T) {
	o := fullOverview()
	o.Kept = []string{"acme.card"}
	o.Layout = ParseOverviewLayout("1.internet-ports-tunnels.traffic-resources-system", OverviewPlacement{ID: "acme.card"})
	o.Layout = o.Layout.Without("hardware")
	o.Layout = o.Layout.Without("acme.card")
	o.Layout.Cols[0] = append([]string{"acme.card"}, o.Layout.Cols[0]...)
	got := render(t, newRenderer(t), o)
	if order := allCardOrder(got); len(order) == 0 || order[0] != "acme.card" {
		t.Fatalf("order = %v, want the kept card first", order)
	}
	if strings.Contains(got, `data-overview-card="acme.card"`) {
		t.Error("a card that did not draw is drawn")
	}
}

func TestADefaultLayoutPlacesPluginCards(t *testing.T) {
	l := DefaultOverviewLayout(
		OverviewPlacement{ID: "acme.card", Where: "after", Anchor: "system"},
		OverviewPlacement{ID: "a.top", Where: "first"},
		OverviewPlacement{ID: "b.gone", Where: "before", Anchor: "nowhere"},
	)
	if want := []string{"a.top", "internet", "ports", "tunnels"}; !slices.Equal(l.Cols[0][:4], want) {
		t.Errorf("left = %v, want %v", l.Cols[0], want)
	}
	if want := []string{"traffic", "resources", "system", "acme.card", "hardware"}; !slices.Equal(l.Cols[1][:5], want) {
		t.Errorf("right = %v, want %v then b.gone at a column's end", l.Cols[1], want)
	}
	if !slices.Contains(slices.Concat(l.Cols[0], l.Cols[1]), "b.gone") {
		t.Error("a card whose anchor is missing is not placed")
	}
}

func TestOverviewLayoutVersionTwoNamesPluginCards(t *testing.T) {
	l := DefaultOverviewLayout(OverviewPlacement{ID: "acme.card"})
	l = l.Without("acme.card")
	l.Cols[0] = append([]string{"acme.card"}, l.Cols[0]...)
	l.Closed = []string{"acme.card", "ports"}
	value := l.Encode()
	if !strings.HasPrefix(value, "2.") {
		t.Fatalf("encoded %q, want version 2", value)
	}
	back := ParseOverviewLayout(value, OverviewPlacement{ID: "acme.card"})
	if !slices.Equal(back.Cols[0], l.Cols[0]) || !slices.Equal(back.Cols[1], l.Cols[1]) || !slices.Equal(back.Closed, l.Closed) {
		t.Errorf("round trip = %+v, want %+v", back, l)
	}
}

func TestOverviewLayoutVersionTwoIsUntrusted(t *testing.T) {
	enc := func(s string) string { return "2." + base64.RawURLEncoding.EncodeToString([]byte(s)) }
	for name, value := range map[string]string{
		"not base64":   "2.@@@",
		"not json":     enc("{nope"),
		"too long":     "2." + strings.Repeat("A", 4000),
		"odd id":       enc(`{"cols":[["internet","<script>"],[]],"closed":[],"hidden":[]}`),
		"three cols":   enc(`{"cols":[[],[],[]],"closed":[],"hidden":[]}`),
		"wrong shapes": enc(`{"cols":"x"}`),
	} {
		t.Run(name, func(t *testing.T) {
			got := ParseOverviewLayout(value, OverviewPlacement{ID: "acme.card"})
			want := DefaultOverviewLayout(OverviewPlacement{ID: "acme.card"})
			if !slices.Equal(got.Cols[0], want.Cols[0]) || !slices.Equal(got.Cols[1], want.Cols[1]) {
				t.Errorf("layout = %+v, want the default", got)
			}
		})
	}
}

func TestAnUninstalledPluginsCardLeavesTheLayout(t *testing.T) {
	l := DefaultOverviewLayout(OverviewPlacement{ID: "acme.card"})
	value := l.Encode()
	if got := ParseOverviewLayout(value); slices.Contains(slices.Concat(got.Cols[0], got.Cols[1]), "acme.card") {
		t.Errorf("layout %v keeps a card no installed plugin has", got.Cols)
	}
}
