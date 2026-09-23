// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"embed"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"strings"
	"sync/atomic"

	"github.com/yuin/goldmark"
)

//go:embed templates/*.tmpl
var templateFS embed.FS

// Partials exposes the widget templates so the shell's own pages can reuse the
// few partials that are shared vocabulary rather than widget internals — the
// copy control, the chip, the tooltip. A page that renders a fact the same way a
// table cell does should render it with the same markup, not a second copy of
// it that drifts.
func Partials() fs.FS { return templateFS }

// Renderer renders widgets to HTML. Values flow through html/template, so
// plugin-supplied text is contextually escaped and cannot inject markup into
// the shell's origin. Not to be copied after first use (it holds counters).
//
// Dispatch is polymorphic, not a switch: each widget renders itself via renderInto
// (below), so adding a widget touches only that widget's file — never this engine.
// The per-widget view models and render logic live beside their structs.
//
// Localization (ADR-012) has two seams here. The {{ t }} template function is
// bound at parse time, so the template set is parsed once per installed language
// and cached (sets); the request selects its set. And the request's translator is
// carried on the render handle (t) so a widget's renderInto can localize the
// string defaults it injects (Form "Save", Confirm labels, …), which have no
// struct field for translateSchema to reach.
type Renderer struct {
	// sets is the per-language template cache on the root renderer: language code
	// → parsed set, with "" the English (identity) set. Swapped atomically on a
	// catalog rescan, so a concurrent render keeps rendering its own snapshot.
	sets atomic.Pointer[map[string]*template.Template]
	// tmpl and t are set on a per-render handle derived by RenderWithToken: the
	// selected language's template set, and its translator (nil/identity for
	// English). The root renderer leaves them nil and never renders directly.
	tmpl *template.Template
	t    func(string) string
	md   goldmark.Markdown
	// seq holds the process-wide id counters, shared by pointer so a per-render
	// handle keeps minting ids that never collide with any other render's.
	seq *renderSeqs
	// depth is how many sections enclose what this handle renders, so a
	// section inside another is titled as its part (h3), not as the page's.
	depth int
}

// nested is the handle a section renders its contents with: the same pass,
// one section deeper. Built field by field, as RenderWithToken's is — the root
// renderer's template cache is never copied.
func (r *Renderer) nested() *Renderer {
	return &Renderer{tmpl: r.tmpl, t: r.t, md: r.md, seq: r.seq, depth: r.depth + 1}
}

// renderSeqs are the monotonic id counters that keep generated element ids
// unique — confirm checkboxes, chart gradients — across every render (they are
// shared by pointer, never copied, so concurrency is safe).
type renderSeqs struct {
	cfm   atomic.Int64
	chart atomic.Int64
}

// NewRenderer parses the embedded widget templates and builds the sanitising
// Markdown engine for the raw bridge. It starts with English only; SetLanguages
// adds the installed catalogs' template sets once the bundle is known.
func NewRenderer() (*Renderer, error) {
	base, err := parseWidgetTemplates(identityTranslator)
	if err != nil {
		return nil, fmt.Errorf("widget: parse templates: %w", err)
	}
	r := &Renderer{md: newMarkdown(), seq: &renderSeqs{}}
	r.sets.Store(&map[string]*template.Template{"": base})
	return r, nil
}

// identityTranslator is the English translator: every source string is its own
// translation. It backs the "" template set and any nil per-request translator.
func identityTranslator(s string) string { return s }

// parseWidgetTemplates parses the embedded widget templates with t bound as the
// {{ t }} function, so each installed language gets its own set (html/template
// binds funcs at parse time).
func parseWidgetTemplates(t func(string) string) (*template.Template, error) {
	return template.New("widget").
		Funcs(template.FuncMap{"icon": Icon, "t": t, "breakable": Breakable}).
		ParseFS(templateFS, "templates/*.tmpl")
}

// SetLanguages rebuilds the per-language template cache: the English identity set
// plus one parsed set per installed code, each with its {{ t }} closing over that
// code's translator. translatorFor returns the lookup for a code (i18n.Bundle's
// Translator). The fresh cache is swapped in atomically. Called at startup and on
// every catalog rescan (ADR-012); safe to call while requests render.
func (r *Renderer) SetLanguages(codes []string, translatorFor func(code string) func(string) string) error {
	sets := make(map[string]*template.Template, len(codes)+1)
	base, err := parseWidgetTemplates(identityTranslator)
	if err != nil {
		return fmt.Errorf("widget: parse templates: %w", err)
	}
	sets[""] = base
	for _, code := range codes {
		set, err := parseWidgetTemplates(translatorFor(code))
		if err != nil {
			return fmt.Errorf("widget: parse templates for %q: %w", code, err)
		}
		sets[strings.ToLower(code)] = set
	}
	r.sets.Store(&sets)
	return nil
}

// setFor returns the parsed template set for a language code, falling back to the
// English set for an empty or uninstalled code.
func (r *Renderer) setFor(lang string) *template.Template {
	sets := *r.sets.Load()
	if set, ok := sets[strings.ToLower(lang)]; ok {
		return set
	}
	return sets[""]
}

// RenderWithToken renders w for the request's language, injecting csrfToken as a
// hidden field into any form it produces so state-changing plugin submissions
// carry the caller's CSRF token (VS-04). An empty token omits it. lang selects the
// template set and t is the request's translator (nil = English): when t is
// non-nil the widget tree is translated in place first (translateSchema), and t
// rides the render handle so injected string defaults are localized too. The
// widget set is closed, so an unknown type is a programming error, not an
// extension point.
func (r *Renderer) RenderWithToken(out io.Writer, w Widget, csrfToken, lang string, t func(string) string) error {
	// The one place a page's tree becomes markup: dock the page-wide lens here so
	// every page — shell or plugin — renders it below the masthead wherever the page
	// declared it, without each render site remembering to place it first. Whether a
	// page carries a lens at all is the page's own decision, made before this.
	w = HoistFilter(w)
	if t != nil {
		translateSchema(w, t)
	}
	pass := &Renderer{tmpl: r.setFor(lang), t: t, md: r.md, seq: r.seq}
	return pass.render(out, w, csrfToken)
}

// tr localizes a string the renderer itself injects at render time (a widget's
// default label, e.g. Form's "Save"). translateSchema already localized every
// authored struct field, so this is only ever called on renderer-owned literals —
// never re-translating an authored value. English (nil t) is the identity.
func (r *Renderer) tr(s string) string {
	if r.t == nil {
		return s
	}
	return r.t(s)
}

// translate runs the localization walk over a widget subtree the renderer composes
// itself — the overview's child tables/chart/meters, which it renders directly and
// so never reach RenderWithToken's walk. A no-op for English (nil t).
func (r *Renderer) translate(w Widget) {
	if r.t != nil {
		translateSchema(w, r.t)
	}
}

// render dispatches to the widget itself — polymorphism replaces the old type
// switch (Open/Closed). Containers call it back on their children for recursion.
func (r *Renderer) render(out io.Writer, w Widget, csrf string) error {
	// A pruned tree can come back empty — a page whose whole body was a lens it
	// did not earn. Nothing to render is not a failure to render.
	if w == nil {
		return nil
	}
	return w.renderInto(r, out, csrf)
}

// renderChildren renders a slice of widgets to trusted HTML fragments (each produced
// by this renderer, so html/template's escaping guarantees still hold) — the shared
// helper every container widget uses to render its subtree.
func (r *Renderer) renderChildren(children []Widget, csrf string) ([]template.HTML, error) {
	out := make([]template.HTML, 0, len(children))
	for _, c := range children {
		var b strings.Builder
		if err := r.render(&b, c, csrf); err != nil {
			return nil, err
		}
		out = append(out, template.HTML(b.String()))
	}
	return out, nil
}

// joinHTML concatenates rendered fragments into one — for containers whose template
// takes a single body blob (a drawer's trigger) rather than ranging a slice.
func joinHTML(parts []template.HTML) template.HTML {
	var b strings.Builder
	for _, p := range parts {
		b.WriteString(string(p))
	}
	return template.HTML(b.String())
}

func (r *Renderer) execute(out io.Writer, name string, data any) error {
	if err := r.tmpl.ExecuteTemplate(out, name, data); err != nil {
		return fmt.Errorf("widget: render %s: %w", name, err)
	}
	return nil
}
