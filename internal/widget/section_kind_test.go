// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// edgeTrim is a utility by which an element gives back its own air at the
// edge of whatever holds it: first:pt-0, last:pb-0, only:py-0, first:mt-0. A
// not-first: utility adds to the rows after the first and trims nothing.
var edgeTrim = regexp.MustCompile(`(?:^|[\s"'])((?:first|last|only)(?:-of-type)?:[pm][tby]-[0-9.]+)`)

// TestRowsNeverTrimTheirOwnEdges: a row keeps its air whatever stands around
// it; giving back a first or last row's air is the section's to do
// (sections.css), which knows whether its band sits above. A row that trims
// itself does it in titled sections too, and opens one 16px tighter than its
// neighbours — as the forward drawer's reflection gate did under "Reaching it
// from home". Only a read-only listing, which is no form row, keeps its own.
func TestRowsNeverTrimTheirOwnEdges(t *testing.T) {
	allowed := map[string]string{
		"settings.html.tmpl last:pb-0": "a read-only facts listing closes on its last line, inside no section's rows",
		// A last row that draws no hairline keeps that pixel as air instead —
		// it adds to its edge rather than trimming it, so the row stays a cell
		// more than its lines and ends on one.
		"meter.html.tmpl last:pb-2.5":      "a last reading keeps its hairline's pixel as air",
		"properties.html.tmpl last:pb-2.5": "a last fact keeps its hairline's pixel as air",
		// Radios stand a cell and a half apart, so an odd run adds half a
		// padding at its end to finish on a line.
		"field.html.tmpl last-of-type:mb-1.25": "an odd run of radios ends on a line",
	}
	files, err := filepath.Glob("templates/*.tmpl")
	if err != nil {
		t.Fatal(err)
	}
	shell, err := filepath.Glob("../server/templates/*.tmpl")
	if err != nil {
		t.Fatal(err)
	}
	files = append(files, shell...)
	if len(files) == 0 {
		t.Fatal("no templates found")
	}
	for _, path := range files {
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, match := range edgeTrim.FindAllStringSubmatch(string(src), -1) {
			trim := match[1]
			if _, ok := allowed[filepath.Base(path)+" "+trim]; ok {
				continue
			}
			t.Errorf("%s trims its own edge with %s; the section gives back a row's air", path, trim)
		}
	}
}

// TestASectionSaysWhatItIsNotHowItStands: a section states its kind and the
// stylesheet (sections.css) owns its air and its rule, so a page and a drawer
// draw the same section the same way, each by its own reach. No spacing or
// rule utility rides the element.
func TestASectionSaysWhatItIsNotHowItStands(t *testing.T) {
	r := newRenderer(t)
	child := []Widget{&Text{Markdown: "x"}}
	for kind, s := range map[string]*Section{
		"ruled":     {Title: "Time", Hairline: true, Children: child},
		"continued": {Hairline: true, Flush: true, Children: child},
		"plain":     {Title: "Time", Children: child},
		"bare":      {Title: "Time", Flush: true, Children: child},
	} {
		got := render(t, r, s)
		if !strings.Contains(got, `data-verso-section="`+kind+`"`) {
			t.Errorf("%s: section must say its kind:\n%s", kind, got)
		}
		open := got[:strings.Index(got, ">")]
		for _, utility := range []string{"mt-", "pt-", "border-t", "scroll-mt"} {
			if strings.Contains(open, utility) {
				t.Errorf("%s: the section carries no spacing of its own (%q):\n%s", kind, utility, open)
			}
		}
	}
	// A section inside a section is a part: ruled the same way, kept to the
	// content's inset.
	part := render(t, r, &Section{Title: "Outer", Children: []Widget{&Section{Title: "Inner", Hairline: true, Children: child}}})
	if !strings.Contains(part, `data-verso-section="part"`) {
		t.Errorf("a ruled section inside a section is a part:\n%s", part)
	}
}
