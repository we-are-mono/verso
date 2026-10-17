// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"strings"
	"testing"
)

// TestAButtonSaysWhatItDoesWhilePressed: a button whose press starts work the
// page waits on (an update check) names that work while it runs, from the
// label it carries for the moment of the press.
func TestAButtonSaysWhatItDoesWhilePressed(t *testing.T) {
	got := render(t, newRenderer(t), &Button{Label: "Check again", Busy: "Checking…", Style: "act", Name: "action", Value: "check"})
	if !strings.Contains(got, `data-busy-label="Checking…"`) {
		t.Errorf("the button does not carry what it says while pressed:\n%s", got)
	}
	if plain := render(t, newRenderer(t), &Button{Label: "Save"}); strings.Contains(plain, "data-busy-label") {
		t.Errorf("a button with nothing to say while pressed carries a busy label:\n%s", plain)
	}
}

// TestACalloutCarriesTheActsThatResolveIt: a notice that something is missing
// carries the act that resolves it, inside the band: a row of buttons 24px
// under the words, so the buttons stand on the band's lines, in the band's own
// ink. Under the band's buttons the row keeps 8px more of its own, so they
// stand 16px over the frame rather than the words' 8px, at either width.
func TestACalloutCarriesTheActsThatResolveIt(t *testing.T) {
	for _, compact := range []bool{true, false} {
		got := render(t, newRenderer(t), &Callout{Variant: "info", Compact: compact, Title: "Nothing is blocked", Body: "Installing adds a blocklist.",
			Acts: []Link{{Style: "secondary", Label: "Install adblock", Icon: "download", Href: "/system/packages/package?name=adblock", Panel: true}}})
		row := strings.Index(got, `<div data-verso-callout-acts class="verso-notice__acts">`)
		words := strings.Index(got, "Installing adds a blocklist.")
		band := strings.LastIndex(got, "</div></div>")
		if row < 0 || words < 0 || row < words || band < row {
			t.Errorf("compact=%v: the acts row stands inside the band, under its words:\n%s", compact, got)
			continue
		}
		if !strings.Contains(got[row:], ">Install adblock</a>") || !strings.Contains(got[row:], `@click.prevent="showPanel"`) {
			t.Errorf("compact=%v: the act keeps its button and its panel:\n%s", compact, got)
		}
	}
	// 24px under the words, a wrapping row, and 8px more under its buttons.
	declares(t, "notice", ".verso-notice__acts",
		"display: flex;", "flex-wrap: wrap;", "align-items: center;",
		"gap: calc(var(--spacing) * 2) calc(var(--spacing) * 3);",
		"margin-top: calc(var(--spacing) * 6);", "padding-bottom: calc(var(--spacing) * 2);")
	// In the band's ink: the stylesheet colours an act inside a band from it.
	declares(t, "notice", ".verso-notice__acts a.verso-button",
		"--verso-button-ink: currentColor;",
		"--verso-button-edge: color-mix(in srgb, currentColor 40%, transparent);",
		"--verso-button-ground: transparent;")
	// A door in the band's words is the words' own ink, told apart only by its
	// underline: denim inside a marigold band would be a second voice. The
	// shell marks it a door, and the link block keeps its colour off it.
	got := render(t, newRenderer(t), &Callout{Variant: "warning", Body: "Reachable.", Link: &Link{Label: "Open Firewall", Href: "/x"}})
	if !strings.Contains(got, `class="verso-link" data-verso-door`) {
		t.Errorf("a link in a band's words is not its door:\n%s", got)
	}
}

// TestACompactCalloutIsFramedOneStepDarker: a band, compact or not, wears a
// hairline of its tone one step darker than its ground, its frame a pixel out
// so it lands on the notebook's lines, its words half a cell inside it above
// and below.
func TestACompactCalloutIsFramedOneStepDarker(t *testing.T) {
	for variant, tone := range map[string]struct{ line, wash string }{
		"info":    {"var(--color-denim-line)", "var(--color-denim-soft)"},
		"warning": {"var(--color-marigold-line)", "var(--color-marigold-soft)"},
		"danger":  {"var(--color-crimson-line)", "var(--color-crimson-soft)"},
		"success": {"var(--color-green-line)", "var(--color-green-soft)"},
		"neutral": {"var(--color-rule)", "var(--color-quiet)"},
	} {
		got := render(t, newRenderer(t), &Callout{Variant: variant, Compact: true, Body: "x"})
		if !strings.Contains(got, `<div data-verso-callout class="verso-notice" data-verso-tone="`+variant+`" data-verso-compact>`) {
			t.Errorf("%s: a compact band is the notice block in its tone, kept to the form's measure:\n%s", variant, got)
		}
		declares(t, "notice", noticeTone(variant), "--verso-notice-line: "+tone.line+";", "--verso-notice-wash: "+tone.wash+";")
	}
	declares(t, "notice", ".verso-notice",
		"position: relative;", "display: flex;", "align-items: flex-start;", "width: 100%;",
		"padding: calc(var(--spacing) * 1.75) calc(var(--spacing) * 3) calc(var(--spacing) * 2) calc(var(--spacing) * 5.75);",
		"border: 1px solid var(--verso-notice-line);", "border-radius: var(--radius-xs);",
		"background-color: var(--verso-notice-wash);", "margin-bottom: -1px;")
	declares(t, "notice", ".verso-notice[data-verso-compact]", "max-width: var(--container-3xl);")
}

// noticeTone is the notice block's rule that sets a tone's context: info is
// the block's own, and warning shares its rule with caution.
func noticeTone(tone string) string {
	switch tone {
	case "info":
		return ".verso-notice"
	case "warning", "caution":
		return `.verso-notice:is([data-verso-tone="warning"], [data-verso-tone="caution"])`
	}
	return `.verso-notice[data-verso-tone="` + tone + `"]`
}

// TestACalloutCarriesTheMachinesWords: what a tool reported rides inside the
// band, under a hairline of the band's tone, verbatim in mono, with a copy
// control beside it.
func TestACalloutCarriesTheMachinesWords(t *testing.T) {
	got := render(t, newRenderer(t), &Callout{Variant: "warning", Title: "The check could not finish", Body: "The update tool stopped.", Verbatim: "owut: no route to host"})
	for _, want := range []string{`data-verso-tone="warning"`, `<div class="verso-notice__verbatim">`,
		`<p class="verso-notice__said" data-verso-verbatim>owut: no route to host</p>`, `x-data="copy"`} {
		if !strings.Contains(got, want) {
			t.Errorf("the band does not carry the machine's words (%q):\n%s", want, got)
		}
	}
	// Under a hairline of the band's tone, in the verbatim type.
	declares(t, "notice", ".verso-notice__verbatim", "border: 0 solid var(--verso-notice-line);", "border-top-width: 1px;")
	declares(t, "notice", ".verso-notice__said", "@apply type-value;")
	if plain := render(t, newRenderer(t), &Callout{Variant: "warning", Body: "Nothing verbatim."}); strings.Contains(plain, "verso-notice__said") || strings.Contains(plain, "data-verso-verbatim") {
		t.Errorf("a band with no machine words draws a mono line:\n%s", plain)
	}
}

// TestAFormsActCanBeTheDangerAct: a form whose act destroys something (erasing
// the router) wears the danger act rather than the primary.
func TestAFormsActCanBeTheDangerAct(t *testing.T) {
	got := render(t, newRenderer(t), &Form{Action: "/system/maintenance/factory-reset", Submit: "Erase and start over", Tone: ToneDanger})
	if !strings.Contains(got, `class="verso-button" data-verso-style="danger"`) || strings.Contains(got, `class="verso-button">`) {
		t.Errorf("the destroying act is not the danger act:\n%s", got)
	}
}
