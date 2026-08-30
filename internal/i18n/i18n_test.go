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
		"sl.json":     {Data: []byte(`{"Save & Apply":"Shrani in uveljavi","Discard":"Zavrzi"}`)},
		"de.json":     {Data: []byte(`{"Discard":"Verwerfen"}`)},
		"broken.json": {Data: []byte(`{not json`)},               // parse error
		"empty.json":  {Data: []byte(`{}`)},                      // empty catalog
		"en.json":     {Data: []byte(`{"Discard":"no"}`)},        // English is the source, skipped silently
		"pt-br.json":  {Data: []byte(`{"Discard":"Descartar"}`)}, // region variant, rejected
	}, "*.json")

	// broken (parse), empty, and pt-br (region) are reported; en is skipped silently.
	if len(problems) != 3 {
		t.Fatalf("problems = %v, want 3 (broken, empty, pt-br)", problems)
	}
	joined := ""
	for _, p := range problems {
		joined += p.Error() + "\n"
	}
	for _, want := range []string{"broken.json", "empty.json", "pt-br.json"} {
		if !strings.Contains(joined, want) {
			t.Errorf("problems do not mention %q; got:\n%s", want, joined)
		}
	}
	return b
}

func TestLoadDiscoversBaseCodesAndSkipsTheRest(t *testing.T) {
	got := testBundle(t).Codes()
	if len(got) != 2 || got[0] != "de" || got[1] != "sl" {
		t.Fatalf("Codes() = %v, want [de sl] (en/broken/empty/pt-br excluded)", got)
	}
}

func TestLoadReportsDuplicateLanguage(t *testing.T) {
	// Two files that fold to the same code — one loads, the other is reported.
	b, problems := Load(fstest.MapFS{
		"DE.json": {Data: []byte(`{"Discard":"Verwerfen"}`)},
		"de.json": {Data: []byte(`{"Discard":"other"}`)},
	}, "*.json")
	if got := b.Codes(); len(got) != 1 || got[0] != "de" {
		t.Fatalf("Codes() = %v, want [de] once", got)
	}
	if len(problems) != 1 || !strings.Contains(problems[0].Error(), "duplicate") {
		t.Fatalf("want one duplicate problem, got %v", problems)
	}
}

func TestTranslatorFallsBackToSource(t *testing.T) {
	b := testBundle(t)
	sl := b.Translator("sl")
	if got := sl("Save & Apply"); got != "Shrani in uveljavi" {
		t.Errorf("sl(Save & Apply) = %q", got)
	}
	if got := sl("Untranslated string"); got != "Untranslated string" {
		t.Errorf("missing key must fall back to source, got %q", got)
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

// Translation values pass through the package verbatim — escaping is the render
// layer's job (values must reach the browser only as html/template data, never as
// template source or template.HTML). This pins that the package neither mangles
// nor sanitizes them, so the invariant lives downstream, deliberately.
func TestTranslatorReturnsValuesVerbatim(t *testing.T) {
	b, _ := Load(fstest.MapFS{
		"sl.json": {Data: []byte(`{"x":"<b>{{.}}</b> 100%"}`)},
	}, "*.json")
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
