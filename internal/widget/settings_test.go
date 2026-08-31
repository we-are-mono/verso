// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"strings"
	"testing"
)

// firewallDefaults is the "config defaults" block in miniature: a read-only
// policy row carrying value pills, and toggle rows carrying the shared switch
// plus their underlying option name as a code chip.
func firewallDefaults() *Settings {
	return &Settings{Items: []SettingsItem{
		{Title: "Default policies", Desc: "What happens to traffic no zone claims.",
			Pills: []Badge{
				{Variant: "warning", Text: "in: reject"},
				{Variant: "success", Text: "out: accept"},
				{Variant: "warning", Text: "fwd: reject"},
			}},
		{Title: "SYN-flood protection", Desc: "Rate-limit half-open connections.",
			Code: "synflood_protect", Toggle: &SettingsToggle{Name: "synflood_protect", On: true}},
		{Title: "Drop invalid packets", Code: "drop_invalid",
			Toggle: &SettingsToggle{Name: "drop_invalid"}},
	}}
}

// TestRenderSettings: each row shows its plain name, muted description, mono
// code chip, and exactly one kind of trailing state — pills or the shared
// switch, in the state given.
func TestRenderSettings(t *testing.T) {
	r := newRenderer(t)
	got := render(t, r, firewallDefaults())
	for _, want := range []string{
		"Default policies", "What happens to traffic no zone claims.",
		"in: reject", "out: accept", // policy pills render through the badge
		"bg-amber-50",      // reject carries the warning palette
		"synflood_protect", // the underlying option is on the row
		"font-mono",        // …as a mono code chip
		`type="checkbox"`,           // toggle rows carry the shared switch
		"border-b border-slate-200", // bare rows divide with hairlines (stripes retired)
	} {
		if !strings.Contains(got, want) {
			t.Errorf("settings missing %q:\n%s", want, got)
		}
	}
	if strings.Count(got, " checked") != 1 {
		t.Errorf("exactly one switch should be on:\n%s", got)
	}
	if strings.Count(got, `type="checkbox"`) != 2 {
		t.Errorf("only toggle rows should render switches:\n%s", got)
	}
}

// TestRenderSettingsValueAndSeam: a read-out value renders mono; the seam folds
// extra rows behind a details block. The default presentation is bare — rows
// straight on the page; style "card" opts into the one box.
func TestRenderSettingsValueAndSeam(t *testing.T) {
	r := newRenderer(t)
	got := render(t, r, &Settings{
		Style: "card",
		Title: "Resolution",
		Items: []SettingsItem{
			{Title: "Local domain", Code: "domain", Value: "lan"},
			{Title: "Cache size", Code: "cachesize", Value: "1000", Name: "cachesize"},
		},
		Seam: &SettingsSeam{Summary: "2 more options", Items: []SettingsItem{
			{Title: "Wildcard address", Code: "address"},
			{Title: "Skip /etc/hosts", Code: "nohosts", Toggle: &SettingsToggle{Name: "nohosts"}},
		}},
	})
	for _, want := range []string{
		">lan</span>", "font-mono",
		`name="cachesize" value="1000"`, "w-28", // a named value is a fixed-width in-place input
		"<span>Resolution</span>", "uppercase", // the group label renders inside the card
		"<details", "2 more options", "nohosts", "verso-chevron",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("settings missing %q:\n%s", want, got)
		}
	}
	if strings.Count(got, "rounded-2xl") != 1 {
		t.Errorf("the seam must not add a second card:\n%s", got)
	}
}

// TestRenderSettingsTracksItsControls: a row that carries a control declares the
// change-tracking hooks, so a settings block inside a page form is counted,
// reviewed, and submitted by the staged-changes capsule like any other field. A
// row that only reads declares none.
func TestRenderSettingsTracksItsControls(t *testing.T) {
	r := newRenderer(t)
	got := render(t, r, &Settings{Items: []SettingsItem{
		{Title: "Hand out addresses", Code: "ignore",
			Toggle: &SettingsToggle{Name: "lan.ignore", On: true}},
		{Title: "Lease length", Code: "leasetime", Value: "12h", Name: "lan.leasetime"},
		{Title: "Address range", Code: "start · limit", Value: "10.0.0.100 – 10.0.0.249"},
	}})
	for _, want := range []string{
		`data-verso-change-field data-verso-change-name="lan.ignore" data-verso-change-label="Hand out addresses" data-verso-change-kind="toggle"`,
		`data-verso-change-field data-verso-change-name="lan.leasetime" data-verso-change-label="Lease length" data-verso-change-kind="text"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("settings row missing %q:\n%s", want, got)
		}
	}
	if strings.Count(got, "data-verso-change-field") != 2 {
		t.Errorf("a row with nothing to change must declare no hooks:\n%s", got)
	}
}

// TestRenderSettingsBare: without style "card" the rows are bare — no box, no
// card padding, the seam un-inset — so pages stay quiet by default.
func TestRenderSettingsBare(t *testing.T) {
	r := newRenderer(t)
	got := render(t, r, &Settings{
		Items: []SettingsItem{{Title: "Cache size", Code: "cachesize", Value: "1000", Name: "cachesize"}},
		Seam:  &SettingsSeam{Summary: "1 more option", Items: []SettingsItem{{Title: "Minimum TTL", Code: "min_cache_ttl"}}},
	})
	for _, bad := range []string{"rounded-2xl", " shadow-sm", "-mx-5", "px-5"} {
		if strings.Contains(got, bad) {
			t.Errorf("bare settings must not carry card chrome %q:\n%s", bad, got)
		}
	}
}

// TestDecodeSettings: the wire shape round-trips — pills as badges, toggles
// with name and state.
func TestDecodeSettings(t *testing.T) {
	w, err := Decode([]byte(`{
		"type": "settings",
		"items": [
			{"title": "Default policies", "pills": [{"variant":"warning","text":"in: reject"}]},
			{"title": "SYN-flood protection", "code": "synflood_protect",
			 "toggle": {"name": "synflood_protect", "on": true}}
		]
	}`))
	if err != nil {
		t.Fatalf("decode settings: %v", err)
	}
	s, ok := w.(*Settings)
	if !ok {
		t.Fatalf("decoded %T, want *Settings", w)
	}
	if len(s.Items) != 2 || s.Items[0].Pills[0].Variant != "warning" {
		t.Errorf("pills not decoded: %+v", s.Items)
	}
	if s.Items[1].Toggle == nil || !s.Items[1].Toggle.On {
		t.Errorf("toggle not decoded: %+v", s.Items[1])
	}
}
