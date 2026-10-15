// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"bytes"
	"html"
	"regexp"
	"strings"
	"testing"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/parser"
	gmhtml "github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/util"
)

// hostileLinks are destinations a browser reads as javascript:, vbscript: or
// data: once the Markdown is rendered: entity- and escape-encoded schemes,
// mixed case, white space a browser drops, angle-bracket and reference
// destinations, images, and autolinks.
var hostileLinks = map[string]string{
	"named colon entity":         "[x](javascript&colon;alert(1))",
	"decimal colon entity":       "[x](javascript&#58;alert(1))",
	"hex entities":               "[x](&#x6A;avascript&#x3A;alert(1))",
	"backslash-escaped colon":    `[x](javascript\:alert(1))`,
	"mixed case":                 "[x](JaVaScRiPt:alert(1))",
	"tab entity in the scheme":   "[x](java&Tab;script:alert(1))",
	"newline entity in scheme":   "[x](java&NewLine;script:alert(1))",
	"leading control entity":     "[x](&#1;javascript:alert(1))",
	"angle-bracket destination":  "[x](<java script:alert(1)>)",
	"angle-bracket with entity":  "[x](<javascript&colon;alert(1)>)",
	"reference with entity":      "[x][r]\n\n[r]: javascript&colon;alert(1)",
	"vbscript":                   "[x](VBScript&colon;msgbox(1))",
	"data link":                  "[x](data&colon;text/html,<script>alert(1)</script>)",
	"image with entity":          "![x](javascript&colon;alert(1))",
	"image data":                 "![x](data&colon;text/html;base64,PHNjcmlwdD4=)",
	"image mailto":               "![x](mailto:a@example.com)",
	"autolink":                   "<javascript:alert(1)>",
	"autolink mixed case":        "<JaVaScRiPt:alert(1)>",
	"autolink vbscript":          "<vbscript:msgbox(1)>",
	"autolink data":              "<data:text/html,x>",
	"autolink unknown scheme":    "<intent:alert(1)>",
	"double-encoded colon":       "[x](javascript&amp;colon;alert(1))",
	"double-encoded hex":         "[x](javascript&amp;#x3a;alert(1))",
	"entity-encoded whole value": "[x](&#106;&#97;&#118;&#97;&#115;&#99;&#114;&#105;&#112;&#116;&#58;alert(1))",
}

// TestRawMarkdownRendersNoDisallowedScheme: no hostile destination reaches a
// rendered href or src. It is checked twice: through the shell's engine, and
// through one whose renderer passes every URL as given, so the guarantee
// rests on the shell's own link check and not on the Markdown library's.
func TestRawMarkdownRendersNoDisallowedScheme(t *testing.T) {
	for engine, md := range map[string]goldmark.Markdown{
		"shell":           newMarkdown(),
		"safeLinks alone": safeLinksOnly(),
	} {
		for name, src := range hostileLinks {
			t.Run(engine+"/"+name, func(t *testing.T) {
				out := convert(t, md, src)
				for _, attr := range urlAttrs.FindAllStringSubmatch(out, -1) {
					if scheme := browserScheme(attr[2]); !renderedSchemeAllowed(scheme, attr[1] == "src") {
						t.Errorf("%s=%q carries the %q scheme:\n%s", attr[1], attr[2], scheme, out)
					}
				}
			})
		}
	}
}

// TestRawAutolinkWithDisallowedSchemeRendersAsText: an autolink outside the
// allowlist is not a link at all; the reader still sees what was written.
func TestRawAutolinkWithDisallowedSchemeRendersAsText(t *testing.T) {
	out := convert(t, safeLinksOnly(), "see <javascript:alert(1)> here")
	if strings.Contains(out, "<a") {
		t.Errorf("disallowed autolink rendered as a link: %s", out)
	}
	if !strings.Contains(out, "javascript:alert(1)") {
		t.Errorf("disallowed autolink's text was dropped: %s", out)
	}
}

// TestRawKeepsAllowedLinks: http, https, mailto and relative destinations,
// entity-encoded or not, and URL and email autolinks render as written.
func TestRawKeepsAllowedLinks(t *testing.T) {
	for src, want := range map[string]string{
		"[a](https://example.com/?a=1&amp;b=2)": `href="https://example.com/?a=1&amp;b=2"`,
		"[a](https&colon;//example.com)":        `href="https://example.com"`,
		"[a](/relative/path)":                   `href="/relative/path"`,
		"[a](page#a:b)":                         `href="page#a:b"`,
		"[a](mailto:ops@example.com)":           `href="mailto:ops@example.com"`,
		"![i](https://example.com/i.png)":       `src="https://example.com/i.png"`,
		"<https://example.com/x>":               `href="https://example.com/x"`,
		"<HTTP://EXAMPLE.COM>":                  `href="HTTP://EXAMPLE.COM"`,
		"<ops@example.com>":                     `href="mailto:ops@example.com"`,
	} {
		t.Run(src, func(t *testing.T) {
			if out := convert(t, newMarkdown(), src); !strings.Contains(out, want) {
				t.Errorf("want %s in:\n%s", want, out)
			}
		})
	}
}

// urlAttrs finds every href and src the renderer writes; it always quotes
// with double quotes.
var urlAttrs = regexp.MustCompile(`\b(href|src)="([^"]*)"`)

// browserScheme is the scheme a browser reads from an attribute value: the
// HTML entities decoded, leading and trailing controls and spaces trimmed,
// tabs and newlines removed, lower-cased. "" for a relative URL.
func browserScheme(attr string) string {
	s := html.UnescapeString(attr)
	s = strings.TrimFunc(s, func(r rune) bool { return r <= ' ' })
	s = strings.NewReplacer("\t", "", "\n", "", "\r", "").Replace(s)
	end := strings.IndexAny(s, "/?#")
	if end < 0 {
		end = len(s)
	}
	colon := strings.IndexByte(s[:end], ':')
	if colon < 0 {
		return ""
	}
	return strings.ToLower(s[:colon])
}

func renderedSchemeAllowed(scheme string, image bool) bool {
	switch scheme {
	case "", "http", "https":
		return true
	case "mailto":
		return !image
	}
	return false
}

// safeLinksOnly is the shell's Markdown with the library's own URL screening
// switched off, leaving safeLinks as the only check.
func safeLinksOnly() goldmark.Markdown {
	return goldmark.New(
		goldmark.WithParserOptions(parser.WithASTTransformers(util.Prioritized(safeLinks{}, 100))),
		goldmark.WithRendererOptions(gmhtml.WithUnsafe()),
	)
}

func convert(t *testing.T, md goldmark.Markdown, src string) string {
	t.Helper()
	var buf bytes.Buffer
	if err := md.Convert([]byte(src), &buf); err != nil {
		t.Fatalf("Convert(%q): %v", src, err)
	}
	return buf.String()
}
