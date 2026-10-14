// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// The layout grid is 8px (layout-grid spec): every fixed measure lands on an
// 8px line and a component's own box on a 4px step. Margins and gaps stand
// between boxes, so they are layout: one off the 4px step (a fractional
// margin or gap) lands a box between the lines. The 20px cell is retired as
// air: what a cell of air was is a role token now (inset, frame).
//
// Padding is left free, since inside a component it centres text on its line
// (a row's 10px over and 9px under), and so are sizes: a 6px state square, a
// 14px icon and a 20px text line are marks and lines, not layout.
var (
	offGridAir = regexp.MustCompile(`(?:^|[\s"'{}:!(])(-?(?:m|mx|my|mt|mb|ml|mr|ms|me|gap|gap-x|gap-y|space-x|space-y)-\d+\.\d+)(?:[^\w.]|$)`)
	cellAsAir  = regexp.MustCompile(`(?:^|[\s"'{}:!(])(-?(?:p|px|py|pt|pb|pl|pr|ps|pe|m|mx|my|mt|mb|ml|mr|ms|me|gap|gap-x|gap-y|space-x|space-y)-5)(?:[^\w.]|$)`)
)

// gridExceptions are the utilities that keep an off-grid value on purpose,
// each with the reason it does.
var gridExceptions = map[string]string{
	"gap-1.5":  "the 6px between a glyph and its words inside a chip, an act or a link is the component's own",
	"gap-0.5":  "the 2px between the segments of one segmented control is the control's own",
	"-mx-1.5":  "a middot between two words pulls in its own side bearings",
	"mt-1.75":  "a 6px state mark centred on its 20px line hangs 7px down it",
	"-my-0.5":  "a 28px act rides a 24px line, reaching 2px past it either side",
	"-my-1.5":  "a 32px control rides a 20px line, reaching 6px past it either side",
	"my-1.75":  "a 6px bar centred on its 20px line stands 7px off either side",
	"mt-9.75":  "a rule 40px under the content, less its own pixel, lands on the line",
	"mt-5.75":  "a rule 24px under the content, less its own pixel, lands on the line",
	"-mt-1.75": "a 6px state mark centred on its 20px line hangs 7px down it",
}

// gridPending are the sources the 8px sweep has not reached yet. A file leaves
// this list the moment it is clean, and the test says so when it is.
var gridPending = map[string]bool{
	"assets/verso-forms.js":               true,
	"assets/verso-listing.js":             true,
	"assets/verso-stream.js":              true,
	"templates/diagnostics.html.tmpl":     true,
	"templates/entity-panel.html.tmpl":    true,
	"templates/login.html.tmpl":           true,
	"templates/restarting.html.tmpl":      true,
	"templates/staged.html.tmpl":          true,
	"../widget/templates/modal.html.tmpl": true,
}

func gridSources(t *testing.T) []string {
	t.Helper()
	var files []string
	for _, pattern := range []string{"templates/*.tmpl", "../widget/templates/*.tmpl", "assets/verso*.js"} {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, matches...)
	}
	if len(files) < 40 {
		t.Fatalf("found %d template and script sources; the glob no longer reaches them", len(files))
	}
	sort.Strings(files)
	return files
}

func TestLayoutAirStaysOnTheGrid(t *testing.T) {
	for _, file := range gridSources(t) {
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		var offences []string
		for _, re := range []*regexp.Regexp{offGridAir, cellAsAir} {
			for _, m := range re.FindAllStringSubmatch(string(raw), -1) {
				if _, ok := gridExceptions[m[1]]; !ok {
					offences = append(offences, m[1])
				}
			}
		}
		name := filepath.ToSlash(file)
		switch {
		case gridPending[name] && len(offences) == 0:
			t.Errorf("%s is on the grid now; take it off gridPending", name)
		case !gridPending[name] && len(offences) > 0:
			t.Errorf("%s puts air off the 8px grid: %s", name, strings.Join(offences, " "))
		}
	}
	for name := range gridPending {
		if _, err := os.Stat(name); err != nil {
			t.Errorf("gridPending names %s, which no longer exists", name)
		}
	}
}
