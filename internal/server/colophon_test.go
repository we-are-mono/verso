// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"strings"
	"testing"
)

const monoRelease = "DISTRIB_ID='OpenWrt'\nDISTRIB_RELEASE='25.12.4'\nDISTRIB_REVISION='r32933-4ccb782af7'\nDISTRIB_TARGET='layerscape/armv8_64b'\nDISTRIB_ARCH='aarch64_generic'\nDISTRIB_DESCRIPTION='OpenWrt 25.12.4 r32933-4ccb782af7'\n"

var monoBoard = boardFacts{Maker: "Mono", Model: "Gateway Development Kit", Name: "mono,gdk"}

// The line names what runs the router, the revision and target beside the
// release rather than folded into it, so each is its own fact on the line.
func TestColophonLine(t *testing.T) {
	c := newColophon([]byte(monoRelease), []byte("6.12.101\n"), monoBoard, "0.0.41")
	if c.Release != "OpenWrt 25.12.4" || c.Revision != "r32933-4ccb782af7" || c.Target != "layerscape/armv8_64b" || c.Verso != "0.0.41" {
		t.Errorf("colophon = %+v", c)
	}
}

// The copy is what a bug report asks for, one fact a line, labelled in English
// whatever language the page reads in: it is written for whoever fixes the bug.
func TestColophonReport(t *testing.T) {
	c := newColophon([]byte(monoRelease), []byte("6.12.101\n"), monoBoard, "0.0.41")
	want := "OpenWrt 25.12.4 r32933-4ccb782af7\n" +
		"Target: layerscape/armv8_64b\n" +
		"Board: mono,gdk (Mono Gateway Development Kit)\n" +
		"Kernel: 6.12.101\n" +
		"Verso: 0.0.41"
	if c.Report != want {
		t.Errorf("report =\n%s\nwant\n%s", c.Report, want)
	}
}

// A fact the router does not state is left out, never written as a blank; a
// router that names no release at all is still an OpenWrt.
func TestColophonOmitsWhatIsNotStated(t *testing.T) {
	c := newColophon(nil, nil, boardFacts{}, "dev")
	if c.Release != "OpenWrt" || c.Revision != "" || c.Target != "" {
		t.Errorf("colophon = %+v", c)
	}
	if c.Report != "OpenWrt\nVerso: dev" {
		t.Errorf("report = %q", c.Report)
	}
	c = newColophon(nil, nil, boardFacts{Name: "x86_64"}, "dev")
	if !strings.Contains(c.Report, "\nBoard: x86_64\n") {
		t.Errorf("a board with no name keeps its id: %q", c.Report)
	}
}

// Every page ends on the colophon but a log: a log runs on as long as the
// router writes, so a footer would only ever be pushed away under it.
func TestColophonClosesEveryPageButALog(t *testing.T) {
	srv := newServer(t, fakeBackend{})
	for path, want := range map[string]bool{
		"/":            true,
		"/system/logs": false,
	} {
		body := get(t, srv, path).Body.String()
		if got := strings.Contains(body, "<footer data-verso-colophon"); got != want {
			t.Errorf("%s: colophon shown = %v, want %v", path, got, want)
		}
	}
}

// The colophon is 56px as the top bar is: its 28px copy and 14px of air
// either side, the hairline drawn inside the band, so no fixed height or
// border adds to it. It keeps to one line on a phone, the page's 16px
// gutter at its sides: its facts end in an ellipsis rather than wrap.
func TestColophonIsItsLineAndItsAir(t *testing.T) {
	body := get(t, newServer(t, fakeBackend{}), "/").Body.String()
	for _, want := range []string{
		`data-verso-colophon x-data="copy" :data-copied="done" class="verso-colophon"`,
		// The copy's verbatim form: mono 16px on the 24px line.
		`<div class="verso-colophon__line" data-verso-verbatim>`,
		`<p class="verso-colophon__facts">`,
		`<span class="verso-colophon__fact type-value" data-verso-verbatim>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("colophon missing %q", want)
		}
	}
	sheet := blockSheet(t, "colophon")
	for head, want := range map[string][]string{
		// The hairline drawn inside the band: no border, no fixed height.
		".verso-colophon {": {"background-color: var(--color-mid);", "box-shadow: inset 0 1px 0 var(--color-rule);"},
		".verso-colophon__line {": {
			"display: flex;", "align-items: center;", "gap: calc(var(--spacing) * 4);",
			"padding: calc(var(--spacing) * 3.5) calc(var(--spacing) * 4);",
			"color: var(--color-meta);", "@apply type-body;",
		},
		".verso-colophon__facts {": {"flex: 1 1 auto;", "min-width: 0;", "column-gap: calc(var(--spacing) * 3);", "white-space: nowrap;"},
		".verso-colophon__fact {":  {"min-width: 0;", "overflow: hidden;", "text-overflow: ellipsis;", "white-space: nowrap;"},
	} {
		rule := cssBlock(t, sheet, head)
		for _, w := range want {
			if !strings.Contains(rule, w) {
				t.Errorf("blocks/colophon.css %s is missing %s", head, w)
			}
		}
		for _, never := range []string{"height:", "border:", "border-top", "border-bottom", "border-width"} {
			if strings.Contains(rule, never) {
				t.Errorf("blocks/colophon.css %s adds %s to the line and its air:\n%s", head, never, rule)
			}
		}
	}
	if !strings.Contains(sheet, "@media (width >= 48rem) {\n  .verso-colophon__line {\n    padding-inline: calc(var(--spacing) * 10);") {
		t.Error("past a phone the colophon's words keep the page's 40px gutter")
	}
}
