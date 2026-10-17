// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"bytes"
	"fmt"
	"html"
	"html/template"
	"io"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// Raw is the governed bridge (ADR-005 §4): display-only Markdown for when no
// widget fits. It is never interactive, and is rendered through Verso's tokens
// with a visible "raw" affordance — the demand signal for the next widget.
type Raw struct {
	Markdown string `json:"markdown"`
}

func (*Raw) isWidget() {}

func (*Raw) children() []Widget { return nil }

type rawView struct{ HTML template.HTML }

// renderInto converts the plugin's Markdown through the sanitising engine and
// wraps it in the raw affordance (ADR-005 §5).
func (w *Raw) renderInto(r *Renderer, out io.Writer, _ string) error {
	var buf bytes.Buffer
	if err := r.md.Convert([]byte(w.Markdown), &buf); err != nil {
		return fmt.Errorf("widget: render raw: %w", err)
	}
	return r.execute(out, "raw.html.tmpl", rawView{HTML: template.HTML(buf.String())})
}

// newMarkdown builds the goldmark instance for the raw bridge. Raw-HTML
// passthrough is deliberately OFF (WithUnsafe unset), so HTML in the source is
// neutralised rather than injected into the shell's origin; a link/image scheme
// allowlist then strips javascript:/data: destinations. Together these are the
// sanitizer ADR-005 requires.
func newMarkdown() goldmark.Markdown {
	return goldmark.New(
		goldmark.WithParserOptions(
			parser.WithASTTransformers(util.Prioritized(safeLinks{}, 100), util.Prioritized(verbatimCode{}, 200)),
		),
	)
}

// verbatimCode marks inline code a machine string (data-verso-verbatim), so
// the prose block sets it in its role's verbatim form, as every machine
// string in the shell is set.
type verbatimCode struct{}

func (verbatimCode) Transform(doc *ast.Document, _ text.Reader, _ parser.Context) {
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if code, ok := n.(*ast.CodeSpan); ok && entering {
			code.SetAttributeString("data-verso-verbatim", []byte(""))
		}
		return ast.WalkContinue, nil
	})
}

// safeLinks holds every link, image and autolink to the scheme allowlist on
// its own terms, whatever the renderer would screen: a destination fails if
// any decoding of it names a scheme off the list.
type safeLinks struct{}

func (safeLinks) Transform(doc *ast.Document, reader text.Reader, _ parser.Context) {
	source := reader.Source()
	var unlinked []*ast.AutoLink
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch v := n.(type) {
		case *ast.Link:
			if !destinationAllowed(v.Destination, false) {
				v.Destination = []byte("#")
			}
		case *ast.Image:
			if !destinationAllowed(v.Destination, true) {
				v.Destination = []byte("")
			}
		case *ast.AutoLink:
			// An email autolink always renders as mailto:.
			if v.AutoLinkType != ast.AutoLinkEmail && !destinationAllowed(v.URL(source), false) {
				unlinked = append(unlinked, v)
			}
		}
		return ast.WalkContinue, nil
	})
	// A refused autolink becomes its own text, HTML-escaped as its label would
	// be. Swapped after the walk, so the walk never steps off a detached node.
	for _, link := range unlinked {
		label := ast.NewString(link.Label(source))
		label.SetRaw(true)
		link.Parent().ReplaceChild(link.Parent(), link, label)
	}
}

// maxDecodes bounds how many layers of encoding destinationAllowed peels; a
// destination still changing past it is refused.
const maxDecodes = 8

// destinationAllowed reports whether a destination's scheme is on the
// allowlist as written and after every decoding of it: the renderer resolves
// backslash escapes and character references before writing the URL, and a
// browser decodes the attribute's entities again, so each layer is checked.
func destinationAllowed(dest []byte, image bool) bool {
	for range maxDecodes {
		if !schemeAllowed(dest, image) {
			return false
		}
		next := decodeDestination(dest)
		if bytes.Equal(next, dest) {
			return true
		}
		dest = next
	}
	return false
}

// decodeDestination peels one layer of encoding, the way goldmark's renderer
// does (backslash escapes, numeric then named references) and then as HTML
// does.
func decodeDestination(dest []byte) []byte {
	d := util.UnescapePunctuations(dest)
	d = util.ResolveNumericReferences(d)
	d = util.ResolveEntityNames(d)
	return []byte(html.UnescapeString(string(d)))
}

// schemeAllowed reports whether a link/image destination's URL scheme is safe.
// Relative destinations (no scheme) are allowed; http/https always; mailto for
// links only. Everything else (javascript:, data:, vbscript:, …) is rejected.
// The scheme is read as a browser reads it: leading and trailing controls and
// spaces trimmed, tabs and newlines dropped anywhere, case ignored.
func schemeAllowed(dest []byte, image bool) bool {
	s := strings.TrimFunc(string(dest), func(r rune) bool { return r <= ' ' })
	s = strings.Map(func(r rune) rune {
		switch {
		case r == '\t' || r == '\n' || r == '\r':
			return -1
		case 'A' <= r && r <= 'Z':
			return r + ('a' - 'A')
		}
		return r
	}, s)
	if s == "" {
		return false
	}
	// A colon before any slash, query or fragment marks an explicit scheme;
	// otherwise it's relative.
	end := strings.IndexAny(s, "/?#")
	if end < 0 {
		end = len(s)
	}
	colon := strings.IndexByte(s[:end], ':')
	if colon < 0 {
		return true // relative
	}
	switch s[:colon] {
	case "http", "https":
		return true
	case "mailto":
		return !image
	}
	return false
}
