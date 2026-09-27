// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"fmt"
	"html/template"
	"io"
)

// Properties is a label/value detail list (a description list): each row shows a
// name on the left and its value on the right. It is the read-only companion to a
// form — the way to lay out the facts about one thing (a device's address, data
// used, when it was added) so they line up and scan cleanly.
//
// Style sets how rows are separated: a hairline between them ("divided", the
// default), nothing ("plain"), a larger inline identity with its explanation
// beneath ("identity"), or the overview's strong System fact rows ("system").
// The shell owns the chrome; the template maps the style to classes, and unknown
// values fall back to "divided".
type Properties struct {
	Style string     `json:"style"` // "divided" (default) | "plain" | "identity" | "system"
	Items []Property `json:"items"`
	// Align: "" keeps values on the right edge (the default fact sheet);
	// "left" sets them beside a fixed-width label column — the reading order
	// for values a person compares line by line (addresses), with any status
	// pill still holding the right edge.
	Align string `json:"align,omitempty"`
}

// Property is one row: a label, its value, whether the value is monospaced (for
// addresses, keys, and other machine text), and whether to offer an inline copy
// button beside the value (for values a person needs to paste elsewhere).
type Property struct {
	HelpVerbatim bool   `json:"-"` // shell-composed sensor status already localized for the live stream
	Label        string `json:"label"`
	Value        string `json:"value"`
	// Next is what the value becomes — a package's available version — drawn
	// under it as a table's cell draws it: the value recedes, the next leads
	// with the arrow. Machine data, so it is never translated.
	Next string `json:"next,omitempty"`
	Help string `json:"help,omitempty"` // optional explanation immediately beneath this fact
	Mono bool   `json:"mono"`
	// Verbatim declares the value a machine string without the mono type
	// treatment — a size, a rate, an identity set in sans. The localization
	// walk leaves it exactly as authored (as it does Mono and Chip values);
	// the look does not change. Declare it on every value that is data, not
	// words.
	Verbatim bool `json:"verbatim,omitempty"`
	Emphasis bool `json:"emphasis,omitempty"` // promote an important value one size; monospaced values also gain one weight step
	Copy     bool `json:"copy"`
	// Chip renders the value as the small category chip — the same treatment
	// a zone gets everywhere else, so one fact never wears two dresses.
	Chip bool `json:"chip,omitempty"`
	// Variant tones the value with the badge vocabulary — "success",
	// "warning", "danger", "info" — for a fact whose reading is also a
	// verdict: the build a router runs beside the build it could run. It
	// names a meaning, never a colour (ADR-005); the shell maps it to the
	// same text treatment a badge of that tone wears, in both themes. Empty
	// — and any word the vocabulary does not hold — keeps the ordinary ink.
	Variant string `json:"variant,omitempty"`
	// Status hangs a trailing state pill after the value — the same badge
	// vocabulary rows and pill cells speak.
	Status *Badge `json:"status,omitempty"`
	// Dot marks the value with a leading state dot (the tone vocabulary), and
	// Key tags a live row: the rendered markup carries it so the shell's client
	// script can refresh the value — and the dot's tone — in place.
	Dot string `json:"dot,omitempty"`
	Key string `json:"key,omitempty"`
	// Span makes the value a stretch between two ends — a certificate's
	// validity, a lease's term — drawn as the meter's track, filled in its
	// tone up to where now is. The ends are data, written under the track's
	// ends and never translated.
	Span *PropertySpan `json:"span,omitempty"`
}

// PropertySpan is a stretch between From and To, filled At a share of it
// (0–100) in Tone's hue (the tone vocabulary; "" is a hollow track).
type PropertySpan struct {
	From string `json:"from"`
	To   string `json:"to"`
	At   int    `json:"at"`
	Tone string `json:"tone,omitempty"`
}

// Pos is where the mark stands, as a CSS length along the ruler: At, held
// within the stretch.
func (s *PropertySpan) Pos() template.CSS {
	return template.CSS(fmt.Sprintf("%d%%", min(max(s.At, 0), 100))) //nolint:gosec // built here from a clamped integer, never from input
}

func (*Properties) isWidget() {}

func (p *Properties) children() []Widget {
	var out []Widget
	for i := range p.Items {
		if p.Items[i].Status != nil {
			out = append(out, p.Items[i].Status)
		}
	}
	return out
}

func (p *Properties) renderInto(r *Renderer, out io.Writer, _ string) error {
	return r.execute(out, "properties.html.tmpl", p)
}
