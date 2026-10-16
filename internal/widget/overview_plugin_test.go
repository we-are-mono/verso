// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
)

func TestDecodeOverviewBodyAllowsOnlyReadOnlyWidgets(t *testing.T) {
	for _, raw := range []string{
		`{"type":"stack","children":[{"type":"properties","items":[{"label":"Device","value":"ODU"}]},{"type":"callout","variant":"info","body":"Ready"}]}`,
		`{"type":"properties","items":[]}`,
		`{"type":"callout","variant":"warning","title":"Unavailable","body":"No data"}`,
	} {
		if _, err := DecodeOverviewBody(json.RawMessage(raw)); err != nil {
			t.Errorf("valid body rejected: %s: %v", raw, err)
		}
	}
	for _, raw := range []string{
		`{"type":"form","style":"page"}`,
		`{"type":"stack","children":[{"type":"link","label":"go","href":"/outside"}]}`,
		`{"type":"callout","body":"go","link":{"type":"link","label":"go","href":"/plugins/x/"}}`,
		`{"type":"callout","body":"go","acts":[{"label":"run","href":"/plugins/x/"}]}`,
	} {
		if _, err := DecodeOverviewBody([]byte(raw)); err == nil {
			t.Errorf("unsafe body accepted: %s", raw)
		}
	}
}

func TestPluginOverviewLayoutRoundTripsInstalledCards(t *testing.T) {
	id := PluginOverviewCardID("jodu-5174x")
	value := "1.internet-" + id + ".traffic-ports-tunnels-resources-system-hardware.."
	if got := ParseOverviewLayoutWithPlugins(value, []string{"jodu-5174x"}).Encode(); got != value {
		t.Fatalf("layout round-trip = %q, want %q", got, value)
	}
	unknown := ParseOverviewLayoutWithPlugins(value, nil)
	if slices.Contains(slices.Concat(unknown.Cols[0], unknown.Cols[1]), id) {
		t.Fatalf("uninstalled card survived layout: %+v", unknown)
	}
}

func TestPluginOverviewCardJoinsDefaultAndHiddenLayouts(t *testing.T) {
	id := PluginOverviewCardID("jodu-5174x")
	l := DefaultOverviewLayoutWithPlugins([]string{"jodu-5174x"})
	if !slices.Contains(slices.Concat(l.Cols[0], l.Cols[1]), id) {
		t.Fatalf("plugin absent from default layout: %+v", l)
	}
	ids := append(overviewCardIDs(), id)
	encoded := "1...." + strings.Join(ids, "-")
	l = ParseOverviewLayoutWithPlugins(encoded, []string{"jodu-5174x"})
	o := &Overview{Layout: l, PluginCards: []OverviewPluginCard{{PluginID: "jodu-5174x"}}}
	if got := o.layout(); len(got.Cols[0])+len(got.Cols[1]) != 0 || !slices.Equal(got.Hidden, ids) {
		t.Fatalf("all-hidden layout changed: %+v", got)
	}
}
