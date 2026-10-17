// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"sort"
	"strings"
	"sync"
	"testing"
)

// The stylesheet guards hold the block contract (ADR-025): every look a named
// block defined once in its own file, every text a type role, the markup naming
// both instead of repeating them. Each guard is a pure function over the
// shell's style sources, so its fixture test shows it catching what it names;
// the test that runs it over the real tree compares what it finds with the
// ratchet (testdata/stylesheet-ratchet.txt), which lists what the tree has not
// yet converted: a finding not on the list fails, and so does a listed line
// that no longer occurs, so the list only shrinks.
//
// STYLESHEET_RATCHET=write rewrites the list from what the guards find.

// styleSources is everything that styles the shell or writes its classes:
// stylesheets keyed by their path under internal/server/assets, markup
// (templates, client scripts, the Go that writes classes) keyed by its path
// under internal, and DESIGN.md.
type styleSources struct {
	css    map[string]string
	markup map[string]string
	design string
}

// violation is one finding: the guard that found it, the key the ratchet
// lists it by (content, never a line number, so an edit elsewhere in a file
// does not move it), and where it is, for a person reading the failure.
type violation struct {
	guard, key, where string
}

// ── reading the sources ────────────────────────────────────────────────────

// loadStyleSources reads the real tree once.
var loadStyleSources = sync.OnceValues(func() (styleSources, error) {
	_, here, _, _ := runtime.Caller(0)
	internal := filepath.Join(filepath.Dir(here), "..")
	src := styleSources{css: map[string]string{}, markup: map[string]string{}}
	read := func(dst map[string]string, root, rel string) error {
		b, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			return err
		}
		dst[filepath.ToSlash(rel)] = string(b)
		return nil
	}
	assets := filepath.Join(internal, "server", "assets")
	for _, pattern := range []string{"*.css", "blocks/*.css"} {
		files, _ := filepath.Glob(filepath.Join(assets, pattern))
		for _, f := range files {
			rel, _ := filepath.Rel(assets, f)
			if rel == "verso.css" { // compiled
				continue
			}
			if err := read(src.css, assets, rel); err != nil {
				return src, err
			}
		}
	}
	for _, pattern := range []string{
		"widget/templates/*.tmpl", "server/templates/*.tmpl", "server/assets/verso*.js",
		"widget/*.go", "server/*.go",
	} {
		files, _ := filepath.Glob(filepath.Join(internal, pattern))
		for _, f := range files {
			if strings.HasSuffix(f, "_test.go") {
				continue
			}
			rel, _ := filepath.Rel(internal, f)
			if err := read(src.markup, internal, rel); err != nil {
				return src, err
			}
		}
	}
	design, err := os.ReadFile(filepath.Join(internal, "..", "DESIGN.md"))
	src.design = string(design)
	return src, err
})

// ── markup: the class lists the shell writes ───────────────────────────────

// classList is one element's class attribute (or the classes a script sets on
// one element): its utilities, its verso-* classes, and how many template
// actions or script interpolations stand in it (each counts as one class, since
// it writes at least one).
type classList struct {
	file    string
	line    int
	raw     string
	utils   []string
	blocks  []string
	actions int
}

var (
	goTemplateAction = regexp.MustCompile(`\{\{.*?\}\}`)
	jsInterpolation  = regexp.MustCompile(`\$\{[^}]*\}`)
	jsClassName      = regexp.MustCompile(`className\s*=\s*(["'])([^"']*)["']`)
	jsClassList      = regexp.MustCompile(`classList\.(?:add|toggle)\(\s*(["'])([^"']*)["']`)
	goClassField     = regexp.MustCompile(`\bClass:\s*"([^"]*)"`)
)

// classAttrs finds every class="…" in s, reading past template actions and
// script interpolations, which may hold quotes of their own.
func classAttrs(s string) [][2]int {
	var spans [][2]int
	for i := 0; ; {
		at := strings.Index(s[i:], `class=`)
		if at < 0 {
			return spans
		}
		start := i + at + len(`class=`)
		i = start
		if start >= len(s) {
			return spans
		}
		quote := s[start]
		if quote == '\\' && start+1 < len(s) && s[start+1] == '"' { // class=\"…\" in a Go string
			start++
			quote = '"'
		}
		if quote != '"' && quote != '\'' {
			continue
		}
		j := start + 1
		for j < len(s) {
			switch {
			case strings.HasPrefix(s[j:], "{{"):
				end := strings.Index(s[j:], "}}")
				if end < 0 {
					return spans
				}
				j += end + 2
				continue
			case strings.HasPrefix(s[j:], "${"):
				end := strings.Index(s[j:], "}")
				if end < 0 {
					return spans
				}
				j += end + 1
				continue
			case s[j] == '\\' && j+1 < len(s) && s[j+1] == '"':
				// the escaped closing quote of a Go string
			case s[j] != quote:
				j++
				continue
			}
			break
		}
		spans = append(spans, [2]int{start + 1, j})
		i = j
	}
}

func newClassList(file string, line int, raw string) classList {
	cl := classList{file: file, line: line, raw: strings.Join(strings.Fields(raw), " ")}
	stripped := goTemplateAction.ReplaceAllStringFunc(raw, func(string) string { cl.actions++; return " " })
	stripped = jsInterpolation.ReplaceAllStringFunc(stripped, func(string) string { cl.actions++; return " " })
	for _, tok := range strings.Fields(stripped) {
		if strings.HasPrefix(tok, "verso-") {
			cl.blocks = append(cl.blocks, tok)
		} else {
			cl.utils = append(cl.utils, tok)
		}
	}
	return cl
}

var (
	jsStringLiteral = regexp.MustCompile(`"([^"\\\n]*)"|'([^'\\\n]*)'`)
	// utilityShaped is a token that can only be a class: a utility with a
	// value or a variant (text-sm, md:px-10, [&>p]:inline) or one of the bare
	// utilities.
	utilityShaped = regexp.MustCompile(`^!?(?:[a-z0-9@:\[\]()./_%&>~*+=,#!-]*-[a-z0-9@:\[\]()./_%&>~*+=,#!-]*|flex|grid|block|inline|hidden|truncate|italic|uppercase|lowercase|relative|absolute|fixed|sticky|contents|invisible|underline|grow|shrink|border|rounded|shadow|outline|transition|static|isolate)!?$`)
)

// classish is a script's string literal that can only be a class list: two
// or more tokens, every one shaped like a utility or a block class. Scripts
// build elements through helpers (versoEl(tag, classes, text)) and keep maps of
// class strings, which no class="…" attribute shows.
func classish(s string) bool {
	toks := strings.Fields(s)
	if len(toks) < 2 {
		return false
	}
	for _, t := range toks {
		if !utilityShaped.MatchString(t) {
			return false
		}
	}
	return true
}

// classLists is every element's class list across the markup, in file order.
func (src styleSources) classLists() []classList {
	var out []classList
	files := keys(src.markup)
	for _, file := range files {
		body := src.markup[file]
		lineAt := func(i int) int { return strings.Count(body[:i], "\n") + 1 }
		seen := map[int]bool{}
		add := func(from, to int) {
			if !seen[from] {
				seen[from] = true
				out = append(out, newClassList(file, lineAt(from), body[from:to]))
			}
		}
		if strings.HasSuffix(file, ".go") || strings.HasSuffix(file, ".tmpl") || strings.HasSuffix(file, ".js") {
			for _, sp := range classAttrs(body) {
				add(sp[0], sp[1])
			}
		}
		if strings.HasSuffix(file, ".js") {
			for _, re := range []*regexp.Regexp{jsClassName, jsClassList} {
				for _, m := range re.FindAllStringSubmatchIndex(body, -1) {
					add(m[4], m[5])
				}
			}
			for _, m := range jsStringLiteral.FindAllStringSubmatchIndex(body, -1) {
				from, to := m[2], m[3]
				if from < 0 {
					from, to = m[4], m[5]
				}
				if classish(body[from:to]) {
					add(from, to)
				}
			}
		}
		if strings.HasSuffix(file, ".go") {
			for _, m := range goClassField.FindAllStringSubmatchIndex(body, -1) {
				out = append(out, newClassList(file, lineAt(m[0]), body[m[2]:m[3]]))
			}
		}
	}
	return out
}

// ── stylesheets: rules ─────────────────────────────────────────────────────

// cssRule is one rule: the file it is in, its selector list (nested selectors
// as written, & and all), its declarations, and the at-rules around it.
type cssRule struct {
	file      string
	selectors []string
	decls     []cssDecl
	within    []string
}

type cssDecl struct{ prop, value string }

var cssComment = regexp.MustCompile(`(?s)/\*.*?\*/`)

// rules parses every stylesheet into its rules. It knows only what the shell's
// stylesheets write: nested blocks, at-rules with and without a body, and
// declarations; strings and brackets are read past whole.
func (src styleSources) rules() []cssRule {
	var out []cssRule
	for _, file := range keys(src.css) {
		parseCSSBlock(file, cssComment.ReplaceAllString(src.css[file], ""), nil, &out)
	}
	return out
}

func parseCSSBlock(file, body string, within []string, out *[]cssRule) []cssDecl {
	var decls []cssDecl
	for i := 0; i < len(body); {
		end, stop := scanCSS(body, i)
		chunk := strings.TrimSpace(body[i:end])
		switch stop {
		case '{':
			close := matchBrace(body, end)
			inner := body[end+1 : close]
			if strings.HasPrefix(chunk, "@") {
				name := strings.Fields(chunk)[0]
				if name == "@font-face" || name == "@property" || name == "@keyframes" || name == "@theme" {
					i = close + 1
					continue
				}
				parseCSSBlock(file, inner, append(slices.Clone(within), chunk), out)
			} else {
				rule := cssRule{file: file, within: within}
				for _, sel := range splitTop(chunk, ',') {
					rule.selectors = append(rule.selectors, strings.Join(strings.Fields(sel), " "))
				}
				idx := len(*out)
				*out = append(*out, rule)
				(*out)[idx].decls = parseCSSBlock(file, inner, append(slices.Clone(within), chunk), out)
			}
			i = close + 1
		default: // ';' or end
			if k := strings.Index(chunk, ":"); k > 0 && !strings.HasPrefix(chunk, "@") {
				decls = append(decls, cssDecl{strings.TrimSpace(chunk[:k]), strings.TrimSpace(chunk[k+1:])})
			}
			i = end + 1
		}
	}
	return decls
}

// scanCSS reads from i to the next ';', '{' or the end, past strings and
// brackets, and says which it stopped at.
func scanCSS(s string, i int) (int, byte) {
	depth := 0
	for j := i; j < len(s); j++ {
		switch c := s[j]; c {
		case '"', '\'':
			if k := strings.IndexByte(s[j+1:], c); k >= 0 {
				j += k + 1
			}
		case '(', '[':
			depth++
		case ')', ']':
			depth--
		case ';', '{':
			if depth == 0 {
				return j, c
			}
		case '}':
			if depth == 0 {
				return j, ';'
			}
		}
	}
	return len(s), 0
}

func matchBrace(s string, open int) int {
	depth := 0
	for j := open; j < len(s); j++ {
		switch s[j] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return j
			}
		}
	}
	return len(s) - 1
}

// splitTop splits s on sep where it stands outside brackets and parentheses.
func splitTop(s string, sep byte) []string {
	var parts []string
	depth, from := 0, 0
	for j := 0; j < len(s); j++ {
		switch s[j] {
		case '(', '[':
			depth++
		case ')', ']':
			depth--
		case sep:
			if depth == 0 {
				parts = append(parts, strings.TrimSpace(s[from:j]))
				from = j + 1
			}
		}
	}
	return append(parts, strings.TrimSpace(s[from:]))
}

// compounds splits a selector into its compound selectors, dropping the
// combinators between them.
func compounds(sel string) []string {
	var out []string
	depth, from := 0, 0
	flush := func(j int) {
		if c := strings.TrimSpace(sel[from:j]); c != "" {
			out = append(out, c)
		}
		from = j + 1
	}
	for j := 0; j < len(sel); j++ {
		switch sel[j] {
		case '(', '[':
			depth++
		case ')', ']':
			depth--
		case ' ', '>', '+', '~', '\n', '\t':
			if depth == 0 {
				flush(j)
			}
		}
	}
	flush(len(sel))
	return out
}

var (
	cssClass      = regexp.MustCompile(`\.(-?[_a-zA-Z][\w-]*)`)
	cssVersoAttr  = regexp.MustCompile(`\[data-verso-[\w-]+`)
	functionalArg = regexp.MustCompile(`\([^()]*\)`)
)

// ownClasses is the classes a compound itself carries, not those named inside
// its :is(), :not() or :has().
func ownClasses(compound string) []string {
	for functionalArg.MatchString(compound) {
		compound = functionalArg.ReplaceAllString(compound, "")
	}
	var out []string
	for _, m := range cssClass.FindAllStringSubmatch(compound, -1) {
		out = append(out, m[1])
	}
	return out
}

// blockOf is the block a verso-* class belongs to: what stands before its
// "__", without the prefix.
func blockOf(class string) string {
	name := strings.TrimPrefix(class, "verso-")
	if i := strings.Index(name, "__"); i >= 0 {
		return name[:i]
	}
	return name
}

// blockFiles is the blocks the tree defines: blocks/<name>.css.
func (src styleSources) blockFiles() map[string]bool {
	out := map[string]bool{}
	for file := range src.css {
		if name, ok := strings.CutPrefix(file, "blocks/"); ok {
			out[strings.TrimSuffix(name, ".css")] = true
		}
	}
	return out
}

func keys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// ── the guards ─────────────────────────────────────────────────────────────

// contractNames are verso-* classes a published data contract carries, kept
// under their names so what was published keeps working.
var contractNames = map[string]string{
	"verso-port-role": "the board art's role slot (profiles/README.md)",
}

// viewTransitionName is a view transition's class, named in a style and
// selected through ::view-transition-*(), never an element's class.
var viewTransitionName = regexp.MustCompile(`^verso-nav-sub-\d+$`)

const typeFile = "type.css"

// rulesLiveInBlocks: only a block file, or the type roles' file for the two
// type marks, selects a verso-* class or a data-verso-* attribute.
func rulesLiveInBlocks(src styleSources) []violation {
	var out []violation
	for _, r := range src.rules() {
		if strings.HasPrefix(r.file, "blocks/") {
			continue
		}
		for _, sel := range r.selectors {
			names := strings.Contains(sel, ".verso-") || cssVersoAttr.MatchString(sel)
			if r.file == typeFile {
				rest := strings.NewReplacer("[data-verso-verbatim]", "", "[data-verso-figures]", "").Replace(sel)
				names = strings.Contains(rest, ".verso-") || cssVersoAttr.MatchString(rest)
			}
			if names {
				out = append(out, violation{"rules-live-in-blocks", r.file + "\t" + sel, r.file})
			}
		}
	}
	return out
}

var viewTransitionSelector = regexp.MustCompile(`^::view-transition-[a-z-]+\(\.([\w-]+)\)`)

// aBlockStartsFromItself: a block file's selectors start at the block.
func aBlockStartsFromItself(src styleSources) []violation {
	var out []violation
	for _, r := range src.rules() {
		name, ok := strings.CutPrefix(r.file, "blocks/")
		if !ok {
			continue
		}
		block := strings.TrimSuffix(name, ".css")
		for _, sel := range r.selectors {
			first := compounds(sel)
			if len(first) == 0 || strings.HasPrefix(first[0], "&") {
				continue
			}
			if m := viewTransitionSelector.FindStringSubmatch(first[0]); m != nil && strings.HasPrefix(m[1], "verso-"+block) {
				continue
			}
			if !slices.ContainsFunc(ownClasses(first[0]), func(c string) bool {
				return strings.HasPrefix(c, "verso-") && blockOf(c) == block
			}) {
				out = append(out, violation{"a-block-starts-from-itself", r.file + "\t" + sel, r.file})
			}
		}
	}
	return out
}

// crossBlockRulesSetOnlyProperties: a rule whose subject is another block's
// element, and not this block's, declares custom properties and nothing else.
func crossBlockRulesSetOnlyProperties(src styleSources) []violation {
	var out []violation
	for _, r := range src.rules() {
		name, ok := strings.CutPrefix(r.file, "blocks/")
		if !ok {
			continue
		}
		block := strings.TrimSuffix(name, ".css")
		for _, sel := range r.selectors {
			parts := compounds(sel)
			if len(parts) < 2 {
				continue
			}
			own, other := false, false
			for _, c := range ownClasses(parts[len(parts)-1]) {
				if strings.HasPrefix(c, "verso-") {
					if blockOf(c) == block {
						own = true
					} else {
						other = true
					}
				}
			}
			if !other || own {
				continue
			}
			for _, d := range r.decls {
				if !strings.HasPrefix(d.prop, "--") {
					out = append(out, violation{"cross-block-rules-set-only-properties", r.file + "\t" + sel + "\t" + d.prop, r.file})
				}
			}
		}
	}
	return out
}

// classesAreBlocks: every verso-* class the markup writes or a stylesheet
// selects is a block's root or element, of a block that has its file; and
// every class a block file selects is written somewhere.
func classesAreBlocks(src styleSources) []violation {
	blocks := src.blockFiles()
	written := map[string]string{}
	for _, cl := range src.classLists() {
		for _, c := range cl.blocks {
			if _, ok := written[c]; !ok {
				written[c] = fmt.Sprintf("%s:%d", cl.file, cl.line)
			}
		}
	}
	defined := map[string]string{}
	for _, r := range src.rules() {
		for _, sel := range r.selectors {
			for _, m := range cssClass.FindAllStringSubmatch(sel, -1) {
				if strings.HasPrefix(m[1], "verso-") {
					if _, ok := defined[m[1]]; !ok {
						defined[m[1]] = r.file
					}
				}
			}
		}
	}
	valid := func(c string) bool {
		if _, ok := contractNames[c]; ok || viewTransitionName.MatchString(c) {
			return true
		}
		name := strings.TrimPrefix(c, "verso-")
		if strings.Contains(name, "--") || strings.Count(name, "__") > 1 {
			return false
		}
		return blocks[blockOf(c)]
	}
	var out []violation
	for _, c := range keys(written) {
		if !valid(c) {
			out = append(out, violation{"classes-are-blocks", c, written[c]})
		}
	}
	for _, c := range keys(defined) {
		if !valid(c) {
			if _, also := written[c]; !also {
				out = append(out, violation{"classes-are-blocks", c, defined[c]})
			}
			continue
		}
		if _, ok := written[c]; !ok && strings.HasPrefix(defined[c], "blocks/") && !viewTransitionName.MatchString(c) {
			out = append(out, violation{"classes-are-blocks", c + "\tunused", defined[c]})
		}
	}
	return out
}

// noRepeatedBundles: one look is not drawn twice from utilities.
func noRepeatedBundles(src styleSources) []violation {
	sites := map[string][]string{}
	for _, cl := range src.classLists() {
		set := slices.Clone(cl.utils)
		slices.Sort(set)
		set = slices.Compact(set)
		if len(set) < 4 {
			continue
		}
		k := strings.Join(set, " ")
		sites[k] = append(sites[k], fmt.Sprintf("%s:%d", cl.file, cl.line))
	}
	var out []violation
	for _, k := range keys(sites) {
		if len(sites[k]) > 1 {
			out = append(out, violation{"no-repeated-bundles", k, strings.Join(sites[k], ", ")})
		}
	}
	return out
}

// shortClassLists: an element carries at most eight utilities.
func shortClassLists(src styleSources) []violation {
	var out []violation
	for _, cl := range src.classLists() {
		if n := len(cl.utils) + cl.actions; n > 8 {
			out = append(out, violation{"short-class-lists", cl.file + "\t" + cl.raw, fmt.Sprintf("%s:%d (%d)", cl.file, cl.line, n)})
		}
	}
	return out
}

// noRulesInMarkup: an arbitrary variant or an important utility is a rule
// written into a class attribute.
func noRulesInMarkup(src styleSources) []violation {
	var out []violation
	for _, cl := range src.classLists() {
		for _, u := range cl.utils {
			if strings.Contains(u, "[&") || strings.HasPrefix(u, "!") || strings.HasSuffix(u, "!") {
				out = append(out, violation{"no-rules-in-markup", cl.file + "\t" + u, fmt.Sprintf("%s:%d", cl.file, cl.line)})
			}
		}
	}
	return out
}

var templateDefine = regexp.MustCompile(`(?s)\{\{-?\s*define\s+"([^"]+)"\s*-?\}\}(.*?)\{\{-?\s*end\s*-?\}\}`)

// noClassStringTemplates: no template exists only to print classes.
func noClassStringTemplates(src styleSources) []violation {
	var out []violation
	for _, file := range keys(src.markup) {
		if !strings.HasSuffix(file, ".tmpl") {
			continue
		}
		for _, m := range templateDefine.FindAllStringSubmatch(src.markup[file], -1) {
			body := strings.TrimSpace(goTemplateAction.ReplaceAllString(m[2], " "))
			if body != "" && !strings.ContainsAny(body, "<>") && !strings.Contains(m[2], "{{define") {
				out = append(out, violation{"no-class-string-templates", file + "\t" + m[1], file})
			}
		}
	}
	return out
}

var (
	typeUtility = regexp.MustCompile(`^(?:text-(?:xs|sm|base|lg|[0-9]?xl|\[[0-9.]+(?:px|rem|em)\])|font-(?:thin|extralight|light|normal|medium|semibold|bold|extrabold|black|mono|sans|serif|\[.+\])|leading-.+|tracking-.+)$`)
	typeProps   = map[string]bool{"font-size": true, "font-weight": true, "font-family": true, "line-height": true, "letter-spacing": true, "font": true}
)

// typeComesFromRoles: no markup or stylesheet outside the roles' file writes
// a size, weight, line height, tracking or family.
func typeComesFromRoles(src styleSources) []violation {
	var out []violation
	for _, cl := range src.classLists() {
		for _, u := range cl.utils {
			bare := u[strings.LastIndex(u, ":")+1:]
			if typeUtility.MatchString(strings.Trim(bare, "!")) {
				out = append(out, violation{"type-comes-from-roles", cl.file + "\t" + u, fmt.Sprintf("%s:%d", cl.file, cl.line)})
			}
		}
	}
	for _, r := range src.rules() {
		if r.file == typeFile {
			continue
		}
		for _, d := range r.decls {
			if typeProps[d.prop] {
				out = append(out, violation{"type-comes-from-roles", r.file + "\t" + strings.Join(r.selectors, ", ") + "\t" + d.prop, r.file})
			}
		}
	}
	return out
}

var (
	roleUtility  = regexp.MustCompile(`@utility\s+(type-[\w-]+)`)
	roleInDesign = regexp.MustCompile("\\(`(type-[\\w-]+)`[;)]")
)

// typeRolesMatchDesign: the roles type.css defines are the roles DESIGN.md
// lists, by class, and no others.
func typeRolesMatchDesign(src styleSources) []violation {
	defined := map[string]bool{}
	for _, m := range roleUtility.FindAllStringSubmatch(src.css[typeFile], -1) {
		defined[m[1]] = true
	}
	listed := map[string]bool{}
	if at := strings.Index(src.design, "## Typography"); at >= 0 {
		section := src.design[at:]
		if next := strings.Index(section[3:], "\n## "); next >= 0 {
			section = section[:next+3]
		}
		for _, m := range roleInDesign.FindAllStringSubmatch(section, -1) {
			listed[m[1]] = true
		}
	}
	var out []violation
	for _, r := range keys(defined) {
		if !listed[r] {
			out = append(out, violation{"type-roles-match-design", r + "\tnot in DESIGN.md", typeFile})
		}
	}
	for _, r := range keys(listed) {
		if !defined[r] {
			out = append(out, violation{"type-roles-match-design", r + "\tnot in type.css", "DESIGN.md"})
		}
	}
	return out
}

// everyBlockIsImported: the entry imports each block file into the components
// layer, since a CSS import takes no glob and a file left off is a look gone.
func everyBlockIsImported(src styleSources) []violation {
	var out []violation
	for _, block := range keys(src.blockFiles()) {
		want := `@import "./blocks/` + block + `.css" layer(components);`
		if !strings.Contains(src.css["input.css"], want) {
			out = append(out, violation{"every-block-is-imported", block, "input.css"})
		}
	}
	return out
}

var roleBlock = regexp.MustCompile(`(?s)@utility\s+(type-[\w-]+)\s*\{(.*?)\n\}`)

// TestEveryRoleIsWhole: a role sets its face, size, line, weight, tracking and
// figures itself, so nothing of the role around it leaks into a text that takes
// it (ADR-025).
func TestEveryRoleIsWhole(t *testing.T) {
	src, err := loadStyleSources()
	if err != nil {
		t.Fatal(err)
	}
	roles := roleBlock.FindAllStringSubmatch(src.css[typeFile], -1)
	if len(roles) == 0 {
		t.Fatal("type.css defines no role")
	}
	for _, m := range roles {
		own := regexp.MustCompile(`(?m)^  ([a-z-]+):`).FindAllStringSubmatch(m[2], -1)
		set := map[string]bool{}
		for _, p := range own {
			set[p[1]] = true
		}
		for _, want := range []string{"font-family", "font-size", "line-height", "font-weight", "letter-spacing", "font-variant-numeric"} {
			if !set[want] {
				t.Errorf("%s does not set %s", m[1], want)
			}
		}
	}
}

// ── the ratchet ────────────────────────────────────────────────────────────

const ratchetFile = "testdata/stylesheet-ratchet.txt"

var ratchetMu sync.Mutex

// readRatchet is the listed findings of one guard, as a multiset of keys.
func readRatchet(t *testing.T, guard string) map[string]int {
	t.Helper()
	f, err := os.Open(ratchetFile)
	if os.IsNotExist(err) {
		return map[string]int{}
	}
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	out := map[string]int{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		if g, key, ok := strings.Cut(sc.Text(), "\t"); ok && g == guard {
			out[key]++
		}
	}
	return out
}

// writeRatchet replaces one guard's lines in the list with what it found.
func writeRatchet(t *testing.T, guard string, found []violation) {
	t.Helper()
	ratchetMu.Lock()
	defer ratchetMu.Unlock()
	var lines []string
	if b, err := os.ReadFile(ratchetFile); err == nil {
		for _, l := range strings.Split(strings.TrimRight(string(b), "\n"), "\n") {
			if g, _, ok := strings.Cut(l, "\t"); ok && g != guard {
				lines = append(lines, l)
			}
		}
	}
	for _, v := range found {
		lines = append(lines, guard+"\t"+v.key)
	}
	sort.Strings(lines)
	if err := os.WriteFile(ratchetFile, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// holdTheRatchet runs one guard over the real tree and compares its findings
// with the list.
func holdTheRatchet(t *testing.T, guard string, check func(styleSources) []violation) {
	t.Helper()
	src, err := loadStyleSources()
	if err != nil {
		t.Fatal(err)
	}
	found := check(src)
	if os.Getenv("STYLESHEET_RATCHET") == "write" {
		writeRatchet(t, guard, found)
		return
	}
	listed := readRatchet(t, guard)
	for _, v := range found {
		if listed[v.key] > 0 {
			listed[v.key]--
			continue
		}
		t.Errorf("%s: %s (at %s) breaks the block contract (ADR-025)", guard, strings.ReplaceAll(v.key, "\t", " · "), v.where)
	}
	for _, k := range keys(listed) {
		if listed[k] > 0 {
			t.Errorf("%s: the ratchet lists %q, which no longer occurs; delete its line from %s", guard, strings.ReplaceAll(k, "\t", " · "), ratchetFile)
		}
	}
}

func TestRulesLiveInBlocks(t *testing.T) {
	holdTheRatchet(t, "rules-live-in-blocks", rulesLiveInBlocks)
}

func TestABlockStartsFromItself(t *testing.T) {
	holdTheRatchet(t, "a-block-starts-from-itself", aBlockStartsFromItself)
}

func TestCrossBlockRulesSetOnlyProperties(t *testing.T) {
	holdTheRatchet(t, "cross-block-rules-set-only-properties", crossBlockRulesSetOnlyProperties)
}

func TestClassesAreBlocks(t *testing.T) {
	holdTheRatchet(t, "classes-are-blocks", classesAreBlocks)
}

func TestNoRepeatedBundles(t *testing.T) {
	holdTheRatchet(t, "no-repeated-bundles", noRepeatedBundles)
}

func TestShortClassLists(t *testing.T) {
	holdTheRatchet(t, "short-class-lists", shortClassLists)
}

func TestNoRulesInMarkup(t *testing.T) {
	holdTheRatchet(t, "no-rules-in-markup", noRulesInMarkup)
}

func TestNoClassStringTemplates(t *testing.T) {
	holdTheRatchet(t, "no-class-string-templates", noClassStringTemplates)
}

func TestTypeComesFromRoles(t *testing.T) {
	holdTheRatchet(t, "type-comes-from-roles", typeComesFromRoles)
}

func TestEveryBlockIsImported(t *testing.T) {
	holdTheRatchet(t, "every-block-is-imported", everyBlockIsImported)
}

func TestTypeRolesMatchDesign(t *testing.T) {
	holdTheRatchet(t, "type-roles-match-design", typeRolesMatchDesign)
}
