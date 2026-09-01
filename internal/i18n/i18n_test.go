// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package i18n

import (
	"strings"
	"testing"
	"testing/fstest"
)

func testBundle(t *testing.T) *Bundle {
	t.Helper()
	b, problems := Load(fstest.MapFS{
		"sl/base.json":     {Data: []byte(`{"Save & Apply":"Shrani in uveljavi","Discard":"Zavrzi","Zones":"cone-base"}`)},
		"sl/firewall.json": {Data: []byte(`{"Zones":"Območja","Firewall":"Požarni zid"}`)},
		"de/base.json":     {Data: []byte(`{"Discard":"Verwerfen"}`)},
		"sl/broken.json":   {Data: []byte(`{not json`)},               // parse error
		"sl/empty.json":    {Data: []byte(`{}`)},                      // empty catalog
		"en/base.json":     {Data: []byte(`{"Discard":"no"}`)},        // English is the source, skipped silently
		"pt-br/base.json":  {Data: []byte(`{"Discard":"Descartar"}`)}, // region variant, rejected
	}, "*/*.json")

	// broken (parse), empty, and pt-br (region) are reported; en is skipped silently.
	if len(problems) != 3 {
		t.Fatalf("problems = %v, want 3 (broken, empty, pt-br)", problems)
	}
	joined := ""
	for _, p := range problems {
		joined += p.Error() + "\n"
	}
	for _, want := range []string{"broken.json", "empty.json", "pt-br"} {
		if !strings.Contains(joined, want) {
			t.Errorf("problems do not mention %q; got:\n%s", want, joined)
		}
	}
	return b
}

func TestLoadDiscoversBaseCodesAndSkipsTheRest(t *testing.T) {
	got := testBundle(t).Codes()
	if len(got) != 2 || got[0] != "de" || got[1] != "sl" {
		t.Fatalf("Codes() = %v, want [de sl] (en/pt-br excluded)", got)
	}
}

func TestLoadReportsDuplicateComponent(t *testing.T) {
	// Two files that fold to the same code+component — one loads, the other is reported.
	b, problems := Load(fstest.MapFS{
		"DE/base.json": {Data: []byte(`{"Discard":"Verwerfen"}`)},
		"de/base.json": {Data: []byte(`{"Discard":"other"}`)},
	}, "*/*.json")
	if got := b.Codes(); len(got) != 1 || got[0] != "de" {
		t.Fatalf("Codes() = %v, want [de] once", got)
	}
	if len(problems) != 1 || !strings.Contains(problems[0].Error(), "duplicate") {
		t.Fatalf("want one duplicate problem, got %v", problems)
	}
}

func TestTranslatorIsBaseOnly(t *testing.T) {
	b := testBundle(t)
	sl := b.Translator("sl")
	if got := sl("Save & Apply"); got != "Shrani in uveljavi" {
		t.Errorf("sl(Save & Apply) = %q", got)
	}
	if got := sl("Untranslated string"); got != "Untranslated string" {
		t.Errorf("missing key must fall back to source, got %q", got)
	}
	// The base translator must not see a plugin's catalog: "Zones" resolves to the
	// base value, never the firewall plugin's.
	if got := sl("Zones"); got != "cone-base" {
		t.Errorf("base translator leaked a plugin catalog: sl(Zones) = %q", got)
	}
	if got := b.Translator("SL")("Discard"); got != "Zavrzi" {
		t.Errorf("Translator must be case-insensitive on the code, got %q", got)
	}
	for _, code := range []string{"", "en", "xx"} {
		if got := b.Translator(code)("Discard"); got != "Discard" {
			t.Errorf("Translator(%q) must be identity, got %q", code, got)
		}
	}
}

func TestPluginTranslatorOverlaysBase(t *testing.T) {
	b := testBundle(t)
	fw := b.PluginTranslator("sl", "firewall")
	// The plugin's own key wins over base.
	if got := fw("Zones"); got != "Območja" {
		t.Errorf("plugin key must win: fw(Zones) = %q, want Območja", got)
	}
	// A key only the plugin has resolves from the plugin.
	if got := fw("Firewall"); got != "Požarni zid" {
		t.Errorf("fw(Firewall) = %q, want Požarni zid", got)
	}
	// A shell-owned key the plugin does not carry falls through to base.
	if got := fw("Save & Apply"); got != "Shrani in uveljavi" {
		t.Errorf("plugin translator must fall through to base: fw(Save & Apply) = %q", got)
	}
	// A key in neither falls back to the source.
	if got := fw("Nothing here"); got != "Nothing here" {
		t.Errorf("missing key must fall back to source, got %q", got)
	}
	// An unknown plugin still resolves base keys (base ⊕ nothing).
	if got := b.PluginTranslator("sl", "no-such-plugin")("Discard"); got != "Zavrzi" {
		t.Errorf("unknown plugin must still resolve base, got %q", got)
	}
	// English (no catalogs) is the identity.
	if got := b.PluginTranslator("en", "firewall")("Discard"); got != "Discard" {
		t.Errorf("English plugin translator must be identity, got %q", got)
	}
}

// Translation values pass through the package verbatim — escaping is the render
// layer's job (values must reach the browser only as html/template data, never as
// template source or template.HTML). This pins that the package neither mangles
// nor sanitizes them, so the invariant lives downstream, deliberately.
func TestTranslatorReturnsValuesVerbatim(t *testing.T) {
	b, _ := Load(fstest.MapFS{
		"sl/base.json": {Data: []byte(`{"x":"<b>{{.}}</b> 100%"}`)},
	}, "*/*.json")
	if got := b.Translator("sl")("x"); got != "<b>{{.}}</b> 100%" {
		t.Errorf("value not returned verbatim: %q", got)
	}
}

func TestNegotiate(t *testing.T) {
	available := []string{"de", "sl"}
	cases := []struct {
		header string
		want   string
	}{
		{"", ""},
		{"sl", "sl"},
		{"sl-SI,en;q=0.8", "sl"}, // base subtag matches, highest q
		{"en-US,en;q=0.9", ""},   // no installed match → English
		{"en-US,en;q=0.4,sl;q=0.9,de;q=0.8", "sl"}, // highest-q installed wins
		{"en-US,en;q=0.9,sl;q=0", ""},              // sl explicitly rejected (q=0)
		{"de;q=0.9,sl;q=abc", "de"},                // B2: malformed q rejects sl, does not promote it
		{"sl;q=5,de", "sl"},                        // B2: q>1 clamps to 1, header order breaks the tie
		{"sl,de", "sl"},                            // equal default q → first in header wins
		{"de,sl", "de"},                            // …and order matters
		{"*", ""},                                  // wildcard is not an explicit pick
		{"fr", ""},
	}
	for _, c := range cases {
		if got := Negotiate(c.header, available); got != c.want {
			t.Errorf("Negotiate(%q) = %q, want %q", c.header, got, c.want)
		}
	}
	if got := Negotiate("sl", nil); got != "" {
		t.Errorf("no available languages must yield English, got %q", got)
	}
}

// lookupRecord is one observed translator lookup, for the recorder tests.
type lookupRecord struct {
	code, component, key string
	translated           bool
}

// TestRecordedReportsEveryLookup pins the audit seam: a Recorded view reports
// each request for an installed language with its outcome — translated, or a
// fallback to English (an untranslated string) — labeled by component.
func TestRecordedReportsEveryLookup(t *testing.T) {
	var got []lookupRecord
	rec := testBundle(t).Recorded(func(code, component, key string, translated bool) {
		got = append(got, lookupRecord{code, component, key, translated})
	})

	tr := rec.Translator("sl")
	tr("Discard") // translated in sl/base
	tr("Reboot")  // absent from sl/base — a miss
	ptr := rec.PluginTranslator("sl", "firewall")
	ptr("Zones")         // overlay hit (plugin catalog wins)
	ptr("Save & Apply")  // base hit through the overlay fallthrough
	ptr("Traffic rules") // absent everywhere — a miss

	want := []lookupRecord{
		{"sl", "base", "Discard", true},
		{"sl", "base", "Reboot", false},
		{"sl", "firewall", "Zones", true},
		{"sl", "firewall", "Save & Apply", true},
		{"sl", "firewall", "Traffic rules", false},
	}
	if len(got) != len(want) {
		t.Fatalf("recorded %d lookups, want %d: %v", len(got), len(want), got)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("lookup %d = %v, want %v", i, got[i], w)
		}
	}
}

// TestRecordedStaysSilentWhereNothingCanMiss: English and uninstalled languages
// have no catalog to miss, the empty string is not a source string, and the
// bundle the view derives from keeps translating without a recorder.
func TestRecordedStaysSilentWhereNothingCanMiss(t *testing.T) {
	b := testBundle(t)
	calls := 0
	rec := b.Recorded(func(string, string, string, bool) { calls++ })

	rec.Translator("")("Discard")   // English: the identity
	rec.Translator("fr")("Discard") // not installed: negotiation can't select it
	rec.Translator("sl")("")        // empty string is not a key
	rec.PluginTranslator("", "firewall")("Zones")
	if calls != 0 {
		t.Errorf("recorded %d lookups, want 0", calls)
	}

	if got := b.Translator("sl")("Discard"); got != "Zavrzi" {
		t.Errorf("original bundle translation = %q, want Zavrzi", got)
	}
	if got := rec.Translator("sl")("Discard"); got != "Zavrzi" {
		t.Errorf("recorded view translation = %q, want Zavrzi", got)
	}
}

// TestLoadPluginsMergesTravellingCatalogs pins the travelling-catalog layout
// (ADR-012 §1): <plugin-dir>/i18n/<code>.json merges as that plugin's
// component, a language only a plugin carries becomes negotiable, and the
// usual resilience applies (English skipped, region variants and broken files
// reported, never fatal).
func TestLoadPluginsMergesTravellingCatalogs(t *testing.T) {
	b := testBundle(t)
	problems := b.LoadPlugins(fstest.MapFS{
		"firewall/i18n/sl.json":  {Data: []byte(`{"Rules":"Pravila"}`)},
		"wireless/i18n/fr.json":  {Data: []byte(`{"Radios":"Radios FR"}`)}, // a language base has none of
		"firewall/i18n/en.json":  {Data: []byte(`{"Rules":"no"}`)},         // English is the source, skipped
		"broken/i18n/pt-br.json": {Data: []byte(`{"x":"y"}`)},              // region variant, rejected
		"broken/i18n/de.json":    {Data: []byte(`{not json`)},              // parse error, reported
	}, "*/i18n/*.json")

	if len(problems) != 2 {
		t.Fatalf("problems = %v, want 2 (pt-br, broken de)", problems)
	}
	if got := b.PluginTranslator("sl", "firewall")("Rules"); got != "Pravila" {
		t.Errorf("travelling catalog not applied: %q", got)
	}
	if got := b.PluginTranslator("sl", "firewall")("Save & Apply"); got != "Shrani in uveljavi" {
		t.Errorf("base fallthrough lost under a travelling catalog: %q", got)
	}
	codes := b.Codes()
	found := false
	for _, c := range codes {
		if c == "fr" {
			found = true
		}
	}
	if !found {
		t.Errorf("a plugin-only language must join negotiation, codes = %v", codes)
	}
	if got := b.PluginTranslator("fr", "wireless")("Radios"); got != "Radios FR" {
		t.Errorf("plugin-only language catalog not applied: %q", got)
	}
}

// TestTravellingCatalogOverridesShellDir: a catalog shipped beside the plugin's
// manifest is version-locked to its binary, so it wins over a same
// code+component file installed into the shell's i18n directory.
func TestTravellingCatalogOverridesShellDir(t *testing.T) {
	b := testBundle(t) // sl/firewall.json: {"Zones":"Območja","Firewall":"Požarni zid"}
	problems := b.LoadPlugins(fstest.MapFS{
		"firewall/i18n/sl.json": {Data: []byte(`{"Zones":"Cone (novo)"}`)},
	}, "*/i18n/*.json")
	if len(problems) != 0 {
		t.Fatalf("problems = %v", problems)
	}
	tr := b.PluginTranslator("sl", "firewall")
	if got := tr("Zones"); got != "Cone (novo)" {
		t.Errorf("travelling catalog must override the shell-dir one, got %q", got)
	}
	// The override replaces the component's catalog wholesale — a key only the
	// stale shell-dir file carried falls back to English, never to the stale
	// translation of a different plugin version.
	if got := tr("Firewall"); got != "Firewall" {
		t.Errorf("stale shell-dir key must fall back to source, got %q", got)
	}
}
