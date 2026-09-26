// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"io"
	"strings"
)

// Code displays a long machine value — a public key, a token, an ID — in a
// full-width monospace box with an optional inline copy. It is the right home for
// the kind of string a properties row would cram: it gets room to breathe and wrap,
// and a one-tap copy sits beside it.
type Code struct {
	Label string `json:"label"` // optional heading above the box
	Value string `json:"value"` // the machine value shown (and copied)
	Copy  bool   `json:"copy"`  // show an inline copy button
	// Live declares the block a preview of the form it sits in: what the form
	// would write, kept current as the form is edited rather than as it was when
	// the page was built. It is intent and not a mechanism (ADR-005 §7) — the
	// plugin says "this previews the form", and the shell realizes it by asking
	// the plugin to re-render from the values on screen. Only the plugin can
	// answer: which option a switch writes, whether an off is a zero or a
	// deletion, and how a mark becomes `!0x10/0xff` are its readings of the
	// daemon's grammar, and a preview the shell computed for itself would be a
	// second copy of them, drifting.
	Live bool `json:"live,omitempty"`
	// Grammar is what the value is written in, when the shell should read it
	// line by line rather than show it whole: "uci" for a section as
	// /etc/config spells it, so a keyword reads apart from the key and value
	// after it. Empty is an opaque value — a key, a token — shown as one
	// string.
	Grammar string `json:"grammar,omitempty"`
}

// CodeLine is one line of a Code block read in its grammar: the indent it
// keeps, the keyword that opens it (config, option, list), the key after the
// keyword, and the quoted value, held apart so each can be set in its own
// ink. A line the grammar does not read keeps its whole text in Raw.
type CodeLine struct {
	Indent  string
	Keyword string
	Key     string
	Value   string // the text between the quotes; Quoted says there were any
	Quoted  bool
	Rest    string // what follows an unquoted key (a bare value, or nothing)
	Raw     string
}

// Lines reads the value in its grammar, one CodeLine a line, or nothing for
// an opaque value, which the template shows whole.
func (c *Code) Lines() []CodeLine {
	if c.Grammar != "uci" {
		return nil
	}
	var lines []CodeLine
	for _, raw := range strings.Split(c.Value, "\n") {
		lines = append(lines, uciLine(raw))
	}
	return lines
}

// uciLine reads one line of a uci section: an indent, a keyword, a key, and
// a value in single quotes (uci quotes every value it writes). A line shaped
// any other way is kept raw.
func uciLine(raw string) CodeLine {
	body := strings.TrimLeft(raw, " \t")
	indent := raw[:len(raw)-len(body)]
	keyword, rest, _ := strings.Cut(body, " ")
	switch keyword {
	case "config", "option", "list":
	default:
		return CodeLine{Raw: raw}
	}
	key, rest, _ := strings.Cut(strings.TrimLeft(rest, " "), " ")
	if key == "" {
		return CodeLine{Raw: raw}
	}
	line := CodeLine{Indent: indent, Keyword: keyword, Key: key, Raw: raw}
	rest = strings.TrimLeft(rest, " ")
	if len(rest) >= 2 && strings.HasPrefix(rest, "'") && strings.HasSuffix(rest, "'") {
		line.Value = rest[1 : len(rest)-1]
		line.Quoted = true
	} else {
		line.Rest = rest
	}
	return line
}

func (*Code) isWidget() {}

func (*Code) children() []Widget { return nil }

func (c *Code) renderInto(r *Renderer, out io.Writer, _ string) error {
	return r.execute(out, "code.html.tmpl", c)
}

// EndsWithCode reports whether a surface ends in a visible configuration card.
// Form actions belong to that card, so they follow it without another rule.
// Hidden form carriers take no space and do not break that relationship.
func EndsWithCode(w Widget) bool {
	var children []Widget
	switch v := w.(type) {
	case *Code:
		return !v.Live || v.Label != "" || v.Value != ""
	case *Form:
		children = v.Fields
	case *Section:
		children = v.Children
	case *Stack:
		children = v.Children
	case *Card:
		children = v.Children
	default:
		return false
	}
	for i := len(children) - 1; i >= 0; i-- {
		if f, ok := children[i].(*Field); ok && f.Kind == "hidden" {
			continue
		}
		return EndsWithCode(children[i])
	}
	return false
}

// RenderLivePreviewWithToken renders just the live preview a tree carries, for
// a request that is asking only what the form would write. The page around it is
// already on screen and is not being replaced — re-rendering it would take the
// operator's cursor out of the field they are typing in — so this takes the one
// block out of the answer and leaves the rest unsent.
//
// It reports false for a tree that declares no live preview, which is a POST
// that arrived marked as one and should be answered as whatever it really is.
func (r *Renderer) RenderLivePreviewWithToken(out io.Writer, w Widget, csrfToken, lang string, t func(string) string) (bool, error) {
	if t != nil {
		translateSchema(w, t)
	}
	preview := livePreview(w)
	if preview == nil {
		return false, nil
	}
	pass := &Renderer{tmpl: r.setFor(lang), t: t, md: r.md, seq: r.seq}
	return true, pass.render(out, preview, csrfToken)
}

// livePreview is the one block a tree declares as its form's preview. A page
// composing two is a page whose forms would overwrite each other's previews, so
// the first is the one that answers and the gauge below names the rest.
func livePreview(w Widget) *Code {
	var found *Code
	Walk(w, func(n Widget) {
		if code, ok := n.(*Code); ok && code.Live && found == nil {
			found = code
		}
	})
	return found
}

// LivePreviewCount is how many live previews a tree declares — the dev gauge's
// question, since only the first of them can ever be kept current.
func LivePreviewCount(w Widget) int {
	count := 0
	Walk(w, func(n Widget) {
		if code, ok := n.(*Code); ok && code.Live {
			count++
		}
	})
	return count
}
