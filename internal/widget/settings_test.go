// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"strings"
	"testing"
)

// TestAFactIsALedgerRow: a fact that only reads is a row of the ledger as a
// setting is (form-fields): its title and description in the question, its
// value or its pills in the answer column, where a setting's control stands,
// with the run's hairlines and air rather than its own.
func TestAFactIsALedgerRow(t *testing.T) {
	got := render(t, newRenderer(t), &Settings{Items: []SettingsItem{
		{Title: "Local domain", Desc: "Names here resolve locally.", Code: "domain", Value: "lan"},
		{Title: "Default policies", Pills: []Badge{{Variant: "warning", Text: "in: reject"}}},
		{Title: "Check for updates automatically", Desc: "Once a day."},
	}})
	if n := strings.Count(got, `class="verso-field-row`); n != 3 {
		t.Fatalf("every fact is a row of the ledger, got %d:\n%s", n, got)
	}
	question, answer := strings.Index(got, "verso-field-question"), strings.Index(got, "verso-field-answer")
	title, desc, value := strings.Index(got, ">Local domain<"), strings.Index(got, `data-verso-field-desc`), strings.Index(got, ">lan</span>")
	if question < 0 || title < question || desc < title || answer < desc || value < answer {
		t.Errorf("a fact's title and description are its question, its value its answer:\n%s", got)
	}
	if pills := strings.Index(got, ">in: reject<"); pills < strings.LastIndex(got[:pills], "verso-field-answer") || strings.LastIndex(got[:pills], "verso-field-answer") < strings.Index(got, ">Default policies<") {
		t.Errorf("a fact's pills are its answer:\n%s", got)
	}
	for _, bad := range []string{"data-verso-settings-row", "border-b border-rule", "data-verso-change-field"} {
		if strings.Contains(got, bad) {
			t.Errorf("a fact draws the ledger's rows, not its own (%q), and tracks no change:\n%s", bad, got)
		}
	}
}

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

// TestInlineValueField: a Value+Name row marked Inline renders the manage-page
// control — a plain input (no read-mode, no edit/confirm icons) that stages on
// blur. Verticality: label + option chip on one line, the field below at a
// compact width, help under that; the datatype rides for client validation and an
// error slot sits beneath the field.
// TestAnInlinePortIsAsWideAsAPort: a number set in place stands at a number's
// measure, the width a port field has in a form, and its description keeps
// the row's.
func TestAnInlinePortIsAsWideAsAPort(t *testing.T) {
	got := render(t, newRenderer(t), &Settings{Items: []SettingsItem{
		{Title: "HTTPS port", Desc: "Where this interface answers.", Code: "web.listen_https", Value: "443", Name: "web_https", Inline: true, Datatype: "port"},
	}})
	input := got[strings.Index(got, `<input id="web_https"`):]
	input = input[:strings.Index(input, ">")]
	if !strings.Contains(input, `class="verso-input"`) || !strings.Contains(input, `data-verso-measure="number"`) || !strings.Contains(input, `inputmode="numeric"`) {
		t.Errorf("an inline port takes the row's width: %s", input)
	}
	if strings.Contains(got, "data-verso-control-measure") {
		t.Errorf("the port's measure narrows its description too:\n%s", got)
	}
}

func TestInlineValueField(t *testing.T) {
	s := &Settings{Items: []SettingsItem{
		{Title: "Hostname", Desc: "Used on the network.", Code: "hostname", Value: "mono-gateway",
			Name: "hostname", Inline: true, Datatype: "hostname"},
	}}
	got := render(t, newRenderer(t), s)
	for _, want := range []string{
		"data-verso-inline-field",
		`<label for="hostname" id="hostname-label" class="text-sm font-medium text-ink">`, // the ordinary field label
		`id="hostname" type="text" name="hostname" value="mono-gateway"`,
		`aria-describedby="hostname-desc hostname-error"`, // read with its description and its refusal
		"data-verso-inline-input",
		`data-verso-datatype="hostname"`,          // the client validates by this on blur
		"data-verso-inline-error",                 // the error slot lives beneath the field
		`class="verso-input" data-verso-verbatim`, // an ordinary field's box: its error treatment ready, its words the same size
		`data-verso-control-measure="host"`,       // same host measure as form fields
		`<p id="hostname-desc" data-verso-field-desc class="text-sm leading-6 text-pretty text-body"><span>Used on the network.</span></p>`, // the description stands under the name, on 24px lines
	} {
		if !strings.Contains(got, want) {
			t.Errorf("inline field missing %q\n%s", want, got)
		}
	}
	// The read-mode + edit/confirm-icon markup is retired: a plain field stages on
	// blur, so none of it should render.
	for _, gone := range []string{"data-verso-inline-input readonly", "data-verso-inline-edit",
		"data-verso-inline-save", "read-only:border-transparent"} {
		if strings.Contains(got, gone) {
			t.Errorf("inline field must not carry retired read-mode markup %q", gone)
		}
	}
	// A plain Value+Name (no Inline) stays the always-open form field, unchanged.
	plain := render(t, newRenderer(t), &Settings{Items: []SettingsItem{
		{Title: "Lease", Value: "12h", Name: "leasetime"},
	}})
	if strings.Contains(plain, "data-verso-inline-field") {
		t.Error("a non-inline Value+Name must not become an inline-commit field")
	}
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
		`data-verso-tone="warning"`,   // reject carries the warning tone
		"synflood_protect",            // the underlying option is on the row
		"data-verso-verbatim",         // …as a mono code chip
		`type="checkbox"`,             // toggle rows carry the shared switch
		`class="verso-field-row`,      // every row is a row of the ledger
		`class="verso-field-question`, // …its title the question
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
		Seam: &SettingsSeam{Summary: "3 more options", Items: []SettingsItem{
			{Title: "Wildcard address", Code: "address"},
			{Title: "Minimum TTL", Code: "min_cache_ttl"},
			{Title: "Skip /etc/hosts", Code: "nohosts", Toggle: &SettingsToggle{Name: "nohosts"}},
		}},
	})
	for _, want := range []string{
		">lan</span>", "font-mono",
		`name="cachesize" value="1000"`, "verso-field-control", // a named value uses shared field geometry
		"<span>Resolution</span>", "uppercase", // the group label renders inside the card
		"<details", "3 more options", "nohosts", "verso-chevron",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("settings missing %q:\n%s", want, got)
		}
	}
	if strings.Count(got, "rounded-xs border border-rule bg-ground") != 0 {
		t.Errorf("settings must not be wrapped in artifact cards:\n%s", got)
	}
}

// TestRenderSettingsTracksItsControls: a row that carries a control declares the
// change-tracking hooks, so a settings block inside a page form is submitted
// with the form and staged like any other field. A row that only reads
// declares none.
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
	for _, bad := range []string{"rounded-xs border border-rule bg-ground", " shadow-sm", "-mx-5", "px-5"} {
		if strings.Contains(got, bad) {
			t.Errorf("bare settings must not carry card chrome %q:\n%s", bad, got)
		}
	}
}

// TestRenderSettingsSeamMinimum: a fold has to hide more than its own summary
// line to be worth a click, so a seam of one or two rows dissolves — the rows
// render in place, controls and change hooks intact — while three rows fold.
func TestRenderSettingsSeamMinimum(t *testing.T) {
	r := newRenderer(t)
	card := func(folded ...SettingsItem) string {
		return render(t, r, &Settings{
			Items: []SettingsItem{{Title: "Local domain", Code: "domain", Value: "lan"}},
			Seam:  &SettingsSeam{Summary: "the long tail", Items: folded},
		})
	}
	short := card(
		SettingsItem{Title: "Skip /etc/hosts", Code: "nohosts", Toggle: &SettingsToggle{Name: "nohosts"}},
		SettingsItem{Title: "Minimum TTL", Code: "min_cache_ttl", Value: "60", Name: "min_cache_ttl"},
	)
	for _, want := range []string{
		"Skip /etc/hosts", "Minimum TTL", // the rows are on the card, not behind it
		`data-verso-change-name="nohosts"`,       // …and still post with the page form
		`data-verso-change-name="min_cache_ttl"`, // …in-place fields included
	} {
		if !strings.Contains(short, want) {
			t.Errorf("dissolved seam missing %q:\n%s", want, short)
		}
	}
	for _, absent := range []string{"<details", "the long tail", "verso-chevron"} {
		if strings.Contains(short, absent) {
			t.Errorf("a seam under the minimum must draw no fold, found %q:\n%s", absent, short)
		}
	}

	long := card(
		SettingsItem{Title: "Skip /etc/hosts", Code: "nohosts"},
		SettingsItem{Title: "Minimum TTL", Code: "min_cache_ttl"},
		SettingsItem{Title: "Maximum TTL", Code: "max_cache_ttl"},
	)
	for _, want := range []string{"<details", "the long tail", "Maximum TTL"} {
		if !strings.Contains(long, want) {
			t.Errorf("seam at the minimum missing %q:\n%s", want, long)
		}
	}

	// A seam declared with nothing in it is not a fold over nothing.
	if none := card(); strings.Contains(none, "<details") {
		t.Errorf("an empty seam must draw no fold:\n%s", none)
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
