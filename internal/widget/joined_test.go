// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"encoding/json"
	"strings"
	"testing"
)

// path is a firewall rule's two zones: where traffic comes from and where it
// goes, one answer to "which traffic" read as the sentence "lan to wan".
func path() *Grid {
	zones := []Option{{Value: "lan", Label: "lan"}, {Value: "wan", Label: "wan"}}
	return &Grid{Style: "form", Columns: 2, Label: "Path", Join: "to", Children: []Widget{
		&Field{Name: "src", Label: "Coming from", Key: "src", Kind: "select", Value: "lan", Options: zones},
		&Field{Name: "dest", Label: "Going to", Key: "dest", Kind: "select", Value: "wan", Options: zones},
	}}
}

// chartChoice is the Temperatures graph's field: which reading, over which
// window, each a choice of what to look at, not a setting.
func chartChoice() *Grid {
	fetch := &FieldFetch{URL: "/system/hardware/temperature", Target: "#hardware-temperature-graph"}
	return &Grid{Style: "form", Label: "Show chart for", Join: "over", Children: []Widget{
		&Field{Name: "sensor", Label: "Reading", Kind: "select", Value: "temp.cluster-thermal", Fetch: fetch,
			Options: []Option{{Value: "temp.cluster-thermal", Label: "CPU"}, {Value: "temp.ddr-thermal", Label: "Memory"}}},
		&Field{Name: "range", Label: "Window", Kind: "select", Value: "day", Fetch: fetch,
			Options: []Option{{Value: "day", Label: "Last 24 hours"}, {Value: "week", Label: "Last 7 days"}}},
	}}
}

// TestAFetchingChoiceFetchesAViewAndStagesNothing: a choice of what to look at
// fetches the view for the row's choices into its place when changed, and
// stages nothing, so the page's changes never count it; the row reads as the
// sentence it is.
func TestAFetchingChoiceFetchesAViewAndStagesNothing(t *testing.T) {
	got := render(t, newRenderer(t), chartChoice())
	for want, n := range map[string]int{
		`hx-get="/system/hardware/temperature"`:                      2,
		`hx-target="#hardware-temperature-graph"`:                    2,
		`hx-include="closest .verso-joined"`:                         2,
		`hx-trigger="change"`:                                        2,
		`hx-swap="outerHTML"`:                                        2,
		`name="sensor"`:                                              1,
		`name="range"`:                                               1,
		">Show chart for<":                                           1,
		`class="verso-joined-word">over</span>`:                      1,
		"data-verso-change-field":                                    0,
		"data-verso-writes":                                          0,
		`<option value="temp.cluster-thermal" selected>CPU</option>`: 1,
	} {
		if c := strings.Count(got, want); c != n {
			t.Errorf("%q appears %d times, want %d:\n%s", want, c, n, got)
		}
	}
	// A setting's dropdown still stages as it always has.
	if setting := render(t, newRenderer(t), path()); strings.Contains(setting, "hx-get") || strings.Count(setting, "data-verso-change-field") != 2 {
		t.Errorf("a setting's dropdown fetches or does not stage:\n%s", setting)
	}
}

// TestAFetchIsTheShellsOwn: a plugin cannot ask a field to fetch: Fetch never
// travels in the widget schema.
func TestAFetchIsTheShellsOwn(t *testing.T) {
	data, err := json.Marshal(&Field{Name: "sensor", Fetch: &FieldFetch{URL: "/x", Target: "#y"}})
	if err != nil {
		t.Fatal(err)
	}
	if s := strings.ToLower(string(data)); strings.Contains(s, "fetch") || strings.Contains(s, "/x") {
		t.Errorf("Fetch travels in the schema: %s", data)
	}
}

// TestJoinedChoicesAreOneRow: two picks that are one answer are one row — one
// label, one chip naming both options, one mark — with the dropdowns side by
// side and the word that joins them standing between. A dropdown is not
// fused into a box (its chevron and its menu are its own), so the word joins
// them as it would in the sentence. Each dropdown keeps its own name, its own
// change and its own options, and stays a dropdown however few it has.
func TestJoinedChoicesAreOneRow(t *testing.T) {
	got := render(t, newRenderer(t), path())
	if strings.Contains(got, "verso-form-grid") {
		t.Errorf("joined choices are one row, not two columns:\n%s", got)
	}
	for want, n := range map[string]int{
		"verso-field-row":         1,
		`class="verso-joined"`:    1,
		"<select":                 2,
		`type="radio"`:            0, // a joined pick stays a dropdown
		"data-verso-change-field": 2,
		`<span id="dest-join" aria-hidden="true" class="verso-joined-word">to</span>`: 1,
	} {
		if c := strings.Count(got, want); c != n {
			t.Errorf("want %d of %q, got %d:\n%s", n, want, c, got)
		}
	}
	for _, want := range []string{
		`<span id="src-group-label" class="text-sm font-medium text-ink">Path</span>`,
		`>src · dest</span>`,
		`role="group" aria-labelledby="src-group-label"`,
		`<select id="src" name="src" aria-label="Coming from"`,
		`<select id="dest" name="dest" aria-label="Going to"`,
		`data-verso-change-name="dest" data-verso-change-label="Going to" data-verso-change-kind="select" data-verso-writes="dest"`,
		`<option value="wan" selected>wan</option>`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("joined row missing %q:\n%s", want, got)
		}
	}
	if src, word, dest := strings.Index(got, `id="src"`), strings.Index(got, `id="dest-join"`), strings.Index(got, `id="dest"`); !(src < word && word < dest) {
		t.Errorf("the word must stand between the two picks:\n%s", got)
	}
}

// TestJoinedChoicesNameTheRefusedPart: the band under the row says which pick
// was refused, and only that pick is invalid.
func TestJoinedChoicesNameTheRefusedPart(t *testing.T) {
	g := path()
	g.Children[1].(*Field).Error = "Choose a zone."
	got := render(t, newRenderer(t), g)
	if !strings.Contains(got, `<span data-verso-error-text>Going to: Choose a zone.</span>`) {
		t.Errorf("the band must name the refused pick:\n%s", got)
	}
	if n := strings.Count(got, `aria-invalid="true"`); n != 1 {
		t.Errorf("only the refused pick is invalid, got %d:\n%s", n, got)
	}
}

// TestJoinedValuesStandApart: a word between values always splits them —
// typed values joined by a word are not fused into one box but stand as their
// own boxes with the word between, as the Path's dropdowns do. A clock time
// and a date are machine strings: mono, each as wide as what it holds.
func TestJoinedValuesStandApart(t *testing.T) {
	got := render(t, newRenderer(t), &Grid{Style: "form", Label: "Between", Join: "to", Children: []Widget{
		&Field{Name: "start_time", Label: "Starts at", Key: "start_time", Datatype: "timehhmmss", Value: "09:00"},
		&Field{Name: "stop_time", Label: "Ends at", Key: "stop_time", Datatype: "timehhmmss", Value: "17:00", Error: "Write a time of day as HH:MM or HH:MM:SS."},
	}})
	for want, n := range map[string]int{
		"verso-field-row":         1,
		`class="verso-box"`:       0, // never fused
		`class="verso-joined"`:    1,
		"<input":                  2,
		"data-verso-change-field": 2,
		`<span id="stop_time-join" aria-hidden="true" class="verso-joined-word">to</span>`: 1,
		"w-measure-32! max-w-full": 2, // a clock time's own width
		`aria-invalid="true"`:      1,
	} {
		if c := strings.Count(got, want); c != n {
			t.Errorf("want %d of %q, got %d:\n%s", n, want, c, got)
		}
	}
	for _, id := range []string{"start_time", "stop_time"} {
		input := got[strings.Index(got, `<input id="`+id+`"`):]
		if input = input[:strings.Index(input, ">")]; !strings.Contains(input, "font-mono") {
			t.Errorf("a clock time is a machine string, set in mono: %s", input)
		}
	}
	for _, want := range []string{
		`<input id="start_time" name="start_time" value="09:00" type="text" aria-label="Starts at"`,
		`<span data-verso-error-text>Ends at: Write a time of day as HH:MM or HH:MM:SS.</span>`,
		`>start_time · stop_time</span>`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("joined values missing %q:\n%s", want, got)
		}
	}
}

// TestJoinedClockTimesStandApart: a window of native clock controls is split
// by its word as typed values are — each end the platform's own time control,
// at a clock time's width, named on its own.
func TestJoinedClockTimesStandApart(t *testing.T) {
	got := render(t, newRenderer(t), &Grid{Style: "form", Label: "Hours", Join: "to", Children: []Widget{
		&Field{Name: "start_time", Label: "Block from", Key: "start_time", Kind: "time", Value: "21:00"},
		&Field{Name: "stop_time", Label: "Until", Key: "stop_time", Kind: "time", Value: "07:00"},
	}})
	for want, n := range map[string]int{
		`class="verso-joined"`:    1,
		`type="time"`:             2,
		"data-verso-change-field": 2,
		`<span id="stop_time-join" aria-hidden="true" class="verso-joined-word">to</span>`: 1,
	} {
		if c := strings.Count(got, want); c != n {
			t.Errorf("want %d of %q, got %d:\n%s", n, want, c, got)
		}
	}
	if !strings.Contains(got, `<input id="stop_time" name="stop_time" value="07:00" type="time" aria-label="Until"`) {
		t.Errorf("each clock is named on its own:\n%s", got)
	}
	// The clock wears the app's own glyph where the platform's would stand,
	// in the quiet ink, as a dropdown wears its chevron; the platform's own
	// stays underneath, invisible, so a press on the glyph still opens its
	// picker.
	for want, n := range map[string]int{
		`points="12 6 12 12 16 14"`: 2, // Lucide's clock, from the shared set
		"verso-time-input":          2,
	} {
		if c := strings.Count(got, want); c != n {
			t.Errorf("want %d of %q, got %d:\n%s", n, want, c, got)
		}
	}
}

// TestAJoinedRowNamesAnOptionOnce: parts that write one option between them
// (a rate's count and its unit, both `limit`) name it once on the row's
// chip, and a count keyed as a count stands at a number's width.
func TestAJoinedRowNamesAnOptionOnce(t *testing.T) {
	got := render(t, newRenderer(t), &Grid{Style: "form", Label: "Rate", Join: "per", Children: []Widget{
		&Field{Name: "limit", Label: "Packets", Key: "limit", Value: "1000"},
		&Field{Name: "limit_unit", Label: "Per", Key: "limit", Kind: "select", Options: []Option{{Value: "second", Label: "Second"}, {Value: "minute", Label: "Minute"}}},
	}})
	if !strings.Contains(got, `>limit</span>`) || strings.Contains(got, "limit · limit") {
		t.Errorf("the row names its one option once:\n%s", got)
	}
	if !strings.Contains(got, "w-measure-24! max-w-full") {
		t.Errorf("a count stands at a number's width:\n%s", got)
	}
	if (&Field{Key: "limit_burst"}).Measure() != "number" {
		t.Error("a burst is a count, at a number's width")
	}
}

// TestDecodeAJoinedGroup: the joining word travels on the grid and translates
// with the plugin's catalog.
func TestDecodeAJoinedGroup(t *testing.T) {
	w, err := Decode([]byte(`{"type":"grid","style":"form","label":"Path","join":"to",
		"children":[{"type":"field","name":"a","label":"A"},{"type":"field","name":"b","label":"B"}]}`))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	g := w.(*Grid)
	if g.Join != "to" {
		t.Fatalf("join not decoded: %+v", g)
	}
	translateSchema(g, func(s string) string { return "«" + s + "»" })
	if g.Join != "«to»" {
		t.Errorf("join not translated: %q", g.Join)
	}
}
