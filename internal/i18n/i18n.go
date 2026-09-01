// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

// Package i18n localizes Verso's user-facing text with a source-string-as-key
// model (the gettext/LuCI shape): the English source string is the key, a
// translation is a catalog lookup, and a missing key or language falls back to
// the source. So English needs no catalog, components keep their English strings,
// and a partial catalog renders the rest in English.
//
// Catalogs are per-component (ADR-012): the shell owns "base", and each plugin
// owns its own. They ship as separate data packages (verso-i18n-base-<code>,
// verso-i18n-<plugin>-<code>) that drop <component>.json into a per-language
// directory (<code>/<component>.json) on disk; the shell loads them at runtime the
// same way it discovers plugin manifests, so adding a language — for the shell or
// for one plugin — needs no rebuild. A plugin's catalog is applied by the shell
// but never curated into the shell's own; it travels with the plugin.
package i18n

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"math"
	"path"
	"sort"
	"strconv"
	"strings"
)

// BaseComponent is the shell's own catalog within a language. Every other
// component key is a plugin id.
const BaseComponent = "base"

// Catalog maps an English source string to its translation in one language.
type Catalog map[string]string

// Bundle is the set of loaded catalogs, keyed by base language code (e.g. "sl")
// and then by component ("base" or a plugin id). English is implicit: it has no
// catalog and is the fallback.
type Bundle struct {
	catalogs map[string]map[string]Catalog
	codes    []string // installed codes, sorted — the negotiation candidate set
	record   func(code, component, key string, translated bool)
}

// Recorded returns a view of b whose translators report every lookup for an
// installed language to sink: the requested source string, the component it was
// requested through, and whether a translation existed (false is a fallback to
// English — an untranslated string). English and uninstalled codes report
// nothing; there is nothing to miss. The view shares b's catalogs; b itself
// stays silent. This is the audit seam: the translators are the one place that
// knows a key fell back, so an audit records there instead of guessing from
// source text (the i18n-pot extractor is partial by design).
func (b *Bundle) Recorded(sink func(code, component, key string, translated bool)) *Bundle {
	view := *b
	view.record = sink
	return &view
}

// sink returns the per-lookup callback for one translator, or nil when nothing
// should be recorded: no recorder installed, English (code ""), or a language
// with no catalogs (negotiation could never select it).
func (b *Bundle) sink(code, component string) func(key string, translated bool) {
	if b.record == nil || code == "" || b.catalogs[code] == nil {
		return nil
	}
	return func(key string, translated bool) { b.record(code, component, key, translated) }
}

// Load reads every catalog matched by glob within fsys. Each file is one
// component of one language, laid out as "<code>/<component>.json" ("sl/base.json",
// "sl/firewall.json"): the parent directory is the language code, the filename base
// is the component. Resilient by design, exactly like plugin.Discover: a malformed
// or empty catalog is skipped and reported, never fatal — one broken file cannot
// break the shell. fsys is injected (ADR-003), so loading is unit-tested against an
// in-memory filesystem.
func Load(fsys fs.FS, glob string) (*Bundle, []error) {
	b := &Bundle{catalogs: make(map[string]map[string]Catalog)}
	matches, err := fs.Glob(fsys, glob)
	if err != nil {
		return b, []error{fmt.Errorf("i18n: bad glob %q: %w", glob, err)}
	}
	sort.Strings(matches)

	var problems []error
	for _, p := range matches {
		code := strings.ToLower(path.Base(path.Dir(p)))
		component := strings.ToLower(strings.TrimSuffix(path.Base(p), path.Ext(p)))
		if code == "" || code == "." || code == "en" {
			continue // no language dir, or English (the source needs no catalog)
		}
		if component == "" {
			continue
		}
		if strings.ContainsRune(code, '-') {
			// Base language codes only. Negotiate matches (and returns) the base
			// subtag, so a region/script variant (pt-br, zh-hant) would load into
			// a slot it could never select. Name the base language instead.
			problems = append(problems, fmt.Errorf("i18n: %s: region/script variant %q unsupported; name the base language", p, code))
			continue
		}
		cat, err := readCatalog(fsys, p)
		if err != nil {
			problems = append(problems, err)
			continue
		}
		if b.catalogs[code] == nil {
			b.catalogs[code] = make(map[string]Catalog)
		}
		if _, dup := b.catalogs[code][component]; dup {
			problems = append(problems, fmt.Errorf("i18n: %s: duplicate catalog for %q/%q, skipped", p, code, component))
			continue
		}
		b.catalogs[code][component] = cat
	}
	b.refreshCodes()
	return b, problems
}

// LoadPlugins merges each plugin's travelling catalog into b (ADR-012 §1):
// files matched by glob within fsys (the plugins directory), laid out as
// "<plugin-dir>/i18n/<code>.json" — the language is the file's basename and
// the component is the plugin directory's name (the plugin id, the same
// convention that keys the mount). Resilient exactly like Load: a malformed
// file is skipped and reported, never fatal. A travelling catalog overrides a
// same code+component catalog already loaded from the shell's i18n directory —
// the file that ships with the plugin binary is version-locked to it.
func (b *Bundle) LoadPlugins(fsys fs.FS, glob string) []error {
	matches, err := fs.Glob(fsys, glob)
	if err != nil {
		return []error{fmt.Errorf("i18n: bad plugin glob %q: %w", glob, err)}
	}
	sort.Strings(matches)

	var problems []error
	for _, p := range matches {
		component := strings.ToLower(topDir(p))
		code := strings.ToLower(strings.TrimSuffix(path.Base(p), path.Ext(p)))
		if component == "" || code == "" || code == "en" {
			continue // no plugin dir, or English (the source needs no catalog)
		}
		if strings.ContainsRune(code, '-') {
			problems = append(problems, fmt.Errorf("i18n: %s: region/script variant %q unsupported; name the base language", p, code))
			continue
		}
		cat, err := readCatalog(fsys, p)
		if err != nil {
			problems = append(problems, err)
			continue
		}
		if b.catalogs[code] == nil {
			b.catalogs[code] = make(map[string]Catalog)
		}
		b.catalogs[code][component] = cat
	}
	b.refreshCodes()
	return problems
}

// topDir returns the first segment of a slash path — the plugin directory a
// travelling catalog belongs to.
func topDir(p string) string {
	dir, _, _ := strings.Cut(p, "/")
	if dir == p {
		return ""
	}
	return dir
}

// readCatalog reads and parses one catalog file, treating unreadable, invalid,
// and empty files as reportable problems.
func readCatalog(fsys fs.FS, p string) (Catalog, error) {
	data, err := fs.ReadFile(fsys, p)
	if err != nil {
		return nil, fmt.Errorf("i18n: read %s: %w", p, err)
	}
	var cat Catalog
	if err := json.Unmarshal(data, &cat); err != nil {
		return nil, fmt.Errorf("i18n: parse %s: %w", p, err)
	}
	if len(cat) == 0 {
		return nil, fmt.Errorf("i18n: %s: empty catalog, skipped", p)
	}
	return cat, nil
}

// refreshCodes rebuilds the sorted negotiation candidate set from the loaded
// catalogs — a travelling catalog may introduce a language base has none of,
// and per-key English fallback covers the shell's own chrome there.
func (b *Bundle) refreshCodes() {
	b.codes = b.codes[:0]
	for code := range b.catalogs {
		b.codes = append(b.codes, code)
	}
	sort.Strings(b.codes)
}

// Codes returns the installed language codes, sorted. English is not among them.
func (b *Bundle) Codes() []string {
	return append([]string(nil), b.codes...)
}

// Translator returns the translate function for the shell's own strings in code:
// a lookup of the base catalog with English fallback. An empty or unknown code, or
// a missing/blank key, returns the source string.
func (b *Bundle) Translator(code string) func(string) string {
	code = strings.ToLower(code)
	return lookup(b.catalogs[code][BaseComponent], nil, b.sink(code, BaseComponent))
}

// PluginTranslator returns the translate function for a plugin's page and manifest
// labels: the plugin's catalog overlaid on the shell's base, so the plugin's own
// text resolves from its catalog while shell-owned widget defaults (Save, Details)
// still resolve from base. A key present in both resolves to the plugin's value.
// English fallback throughout.
func (b *Bundle) PluginTranslator(code, plugin string) func(string) string {
	code = strings.ToLower(code)
	plugin = strings.ToLower(plugin)
	comps := b.catalogs[code]
	return lookup(comps[BaseComponent], comps[plugin], b.sink(code, plugin))
}

// lookup builds a source→translation function that tries the overlay first, then
// the base, then the source itself. A nil catalog contributes nothing; two nil
// catalogs yield the identity, so English (no catalogs) is a no-op. A non-nil
// onLookup observes every non-empty request and whether it resolved.
func lookup(base, overlay Catalog, onLookup func(key string, translated bool)) func(string) string {
	if base == nil && overlay == nil && onLookup == nil {
		return func(s string) string { return s }
	}
	return func(s string) string {
		if overlay != nil {
			if t, ok := overlay[s]; ok && t != "" {
				if onLookup != nil && s != "" {
					onLookup(s, true)
				}
				return t
			}
		}
		if t, ok := base[s]; ok && t != "" {
			if onLookup != nil && s != "" {
				onLookup(s, true)
			}
			return t
		}
		if onLookup != nil && s != "" {
			onLookup(s, false)
		}
		return s
	}
}

// Negotiate picks the best installed code for an Accept-Language header, or ""
// (English) when none is acceptable. It honors quality values and matches on the
// base subtag (so "sl-SI" selects an installed "sl"); q=0 explicitly rejects.
func Negotiate(header string, available []string) string {
	header = strings.TrimSpace(header)
	if header == "" || len(available) == 0 {
		return ""
	}
	acceptable := make(map[string]bool, len(available))
	for _, a := range available {
		acceptable[strings.ToLower(a)] = true
	}

	best, bestQ := "", 0.0
	for _, part := range strings.Split(header, ",") {
		tag, q := parseLanguageRange(part)
		if tag == "" || tag == "*" || q <= 0 {
			continue
		}
		if base, _, _ := strings.Cut(tag, "-"); acceptable[base] && q > bestQ {
			best, bestQ = base, q
		}
	}
	return best
}

// parseLanguageRange splits one Accept-Language entry ("sl-SI;q=0.8") into its
// lowercased tag and quality (default 1.0).
func parseLanguageRange(part string) (tag string, q float64) {
	tag, params, _ := strings.Cut(strings.TrimSpace(part), ";")
	tag = strings.ToLower(strings.TrimSpace(tag))
	q = 1.0
	for params != "" {
		var field string
		field, params, _ = strings.Cut(params, ";")
		value, ok := strings.CutPrefix(strings.TrimSpace(field), "q=")
		if !ok {
			continue
		}
		// A present weight governs the entry. An unparseable one rejects it
		// (q=0) rather than keeping the default 1.0 and silently promoting a
		// client-downranked language to the top (RFC 9110 §12.4.2); a parseable
		// one is clamped to [0,1].
		parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
		if err != nil {
			return tag, 0
		}
		q = math.Max(0, math.Min(1, parsed))
	}
	return tag, q
}
