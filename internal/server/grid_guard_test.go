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
	offGridAir = regexp.MustCompile(`(?:^|[\s"'{:!(])(-?(?:m|mx|my|mt|mb|ml|mr|ms|me|gap|gap-x|gap-y|space-x|space-y)-\d+\.\d+)(?:[^\w.]|$)`)
	cellAsAir  = regexp.MustCompile(`(?:^|[\s"'{:!(])(-?(?:p|px|py|pt|pb|pl|pr|ps|pe|m|mx|my|mt|mb|ml|mr|ms|me|gap|gap-x|gap-y|space-x|space-y)-5)(?:[^\w.]|$)`)
)

// gridExceptions are the utilities that keep an off-grid value on purpose,
// each with the reason it does.
var gridExceptions = map[string]string{}

// gridPending are the sources the 8px sweep has not reached yet. A file leaves
// this list the moment it is clean, and the test says so when it is.
var gridPending = map[string]bool{
	"assets/verso-commit.js":                         true,
	"assets/verso-forms.js":                          true,
	"assets/verso.js":                                true,
	"assets/verso-listing.js":                        true,
	"assets/verso-stream.js":                         true,
	"templates/diagnostics.html.tmpl":                true,
	"templates/entity-panel.html.tmpl":               true,
	"templates/login.html.tmpl":                      true,
	"templates/nav-row.html.tmpl":                    true,
	"templates/package-navigation.html.tmpl":         true,
	"templates/page.html.tmpl":                       true,
	"templates/restarting.html.tmpl":                 true,
	"templates/staged.html.tmpl":                     true,
	"../widget/templates/badge.html.tmpl":            true,
	"../widget/templates/callout.html.tmpl":          true,
	"../widget/templates/card.html.tmpl":             true,
	"../widget/templates/code.html.tmpl":             true,
	"../widget/templates/conditions.html.tmpl":       true,
	"../widget/templates/confirm.html.tmpl":          true,
	"../widget/templates/device_limit_tip.html.tmpl": true,
	"../widget/templates/disclosure.html.tmpl":       true,
	"../widget/templates/empty.html.tmpl":            true,
	"../widget/templates/field.html.tmpl":            true,
	"../widget/templates/filter.html.tmpl":           true,
	"../widget/templates/form.html.tmpl":             true,
	"../widget/templates/grid.html.tmpl":             true,
	"../widget/templates/link.html.tmpl":             true,
	"../widget/templates/list.html.tmpl":             true,
	"../widget/templates/meter.html.tmpl":            true,
	"../widget/templates/modal.html.tmpl":            true,
	"../widget/templates/overview.html.tmpl":         true,
	"../widget/templates/ports.html.tmpl":            true,
	"../widget/templates/properties.html.tmpl":       true,
	"../widget/templates/section.html.tmpl":          true,
	"../widget/templates/settings.html.tmpl":         true,
	"../widget/templates/shared.html.tmpl":           true,
	"../widget/templates/stack.html.tmpl":            true,
	"../widget/templates/stat.html.tmpl":             true,
	"../widget/templates/table.html.tmpl":            true,
	"../widget/templates/table-interfaces.html.tmpl": true,
	"../widget/templates/text.html.tmpl":             true,
	"../widget/templates/traffic.html.tmpl":          true,
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
