// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"bytes"
	"fmt"
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
			parser.WithASTTransformers(util.Prioritized(safeLinks{}, 100)),
		),
	)
}

type safeLinks struct{}

func (safeLinks) Transform(doc *ast.Document, _ text.Reader, _ parser.Context) {
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch v := n.(type) {
		case *ast.Link:
			if !schemeAllowed(v.Destination, false) {
				v.Destination = []byte("#")
			}
		case *ast.Image:
			if !schemeAllowed(v.Destination, true) {
				v.Destination = []byte("")
			}
		}
		return ast.WalkContinue, nil
	})
}

// schemeAllowed reports whether a link/image destination's URL scheme is safe.
// Relative destinations (no scheme) are allowed; http/https always; mailto for
// links only. Everything else (javascript:, data:, vbscript:, …) is rejected.
func schemeAllowed(dest []byte, image bool) bool {
	s := strings.ToLower(strings.TrimSpace(string(dest)))
	if s == "" {
		return false
	}
	colon := strings.IndexByte(s, ':')
	slash := strings.IndexByte(s, '/')
	// A colon before any slash marks an explicit scheme; otherwise it's relative.
	if colon >= 0 && (slash == -1 || colon < slash) {
		switch s[:colon] {
		case "http", "https":
			return true
		case "mailto":
			return !image
		default:
			return false
		}
	}
	return true // relative
}
