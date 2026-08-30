// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

// Package i18n localizes Verso's user-facing text with a source-string-as-key
// model (the gettext/LuCI shape): the English source string is the key, a
// translation is a catalog lookup, and a missing key or language falls back to
// the source. So English needs no catalog, plugins keep their English strings,
// and a partial catalog renders the rest in English.
//
// Catalogs are shipped as separate data packages (verso-i18n-base-<code>) that
// drop a <code>.json map onto disk; the shell loads them at runtime the same way
// it discovers plugin manifests, so adding a language needs no shell rebuild.
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

// Catalog maps an English source string to its translation in one language.
type Catalog map[string]string

// Bundle is the set of loaded language catalogs, keyed by base language code
// (e.g. "sl"). English is implicit: it has no catalog and is the fallback.
type Bundle struct {
	catalogs map[string]Catalog
	codes    []string // installed codes, sorted — the negotiation candidate set
}

// Load reads every catalog matched by glob within fsys, keying each by its
// filename base ("sl.json" → "sl", lowercased). Resilient by design, exactly like
// plugin.Discover: a malformed or empty catalog is skipped and reported, never
// fatal — one broken language file cannot break the shell. fsys is injected
// (ADR-003), so loading is unit-tested against an in-memory filesystem.
func Load(fsys fs.FS, glob string) (*Bundle, []error) {
	b := &Bundle{catalogs: make(map[string]Catalog)}
	matches, err := fs.Glob(fsys, glob)
	if err != nil {
		return b, []error{fmt.Errorf("i18n: bad glob %q: %w", glob, err)}
	}
	sort.Strings(matches)

	var problems []error
	for _, p := range matches {
		code := strings.ToLower(strings.TrimSuffix(path.Base(p), path.Ext(p)))
		if code == "" || code == "en" {
			continue // English is the source; a catalog for it is redundant
		}
		if strings.ContainsRune(code, '-') {
			// Base language codes only. Negotiate matches (and returns) the base
			// subtag, so a region/script variant (pt-br, zh-hant) would load into
			// a slot it could never select. Name the base language instead.
			problems = append(problems, fmt.Errorf("i18n: %s: region/script variant %q unsupported; name the base language", p, code))
			continue
		}
		data, err := fs.ReadFile(fsys, p)
		if err != nil {
			problems = append(problems, fmt.Errorf("i18n: read %s: %w", p, err))
			continue
		}
		var cat Catalog
		if err := json.Unmarshal(data, &cat); err != nil {
			problems = append(problems, fmt.Errorf("i18n: parse %s: %w", p, err))
			continue
		}
		if len(cat) == 0 {
			problems = append(problems, fmt.Errorf("i18n: %s: empty catalog, skipped", p))
			continue
		}
		if _, dup := b.catalogs[code]; dup {
			problems = append(problems, fmt.Errorf("i18n: %s: duplicate language %q, skipped", p, code))
			continue
		}
		b.catalogs[code] = cat
		b.codes = append(b.codes, code)
	}
	sort.Strings(b.codes)
	return b, problems
}

// Codes returns the installed language codes, sorted. English is not among them.
func (b *Bundle) Codes() []string {
	return append([]string(nil), b.codes...)
}

// Translator returns the translate function for code: a catalog lookup with
// English fallback. An empty or unknown code, or a missing/blank key, returns the
// source string — so English and partial catalogs both render the source.
func (b *Bundle) Translator(code string) func(string) string {
	cat := b.catalogs[strings.ToLower(code)]
	if cat == nil {
		return func(s string) string { return s }
	}
	return func(s string) string {
		if t, ok := cat[s]; ok && t != "" {
			return t
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
