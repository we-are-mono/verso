// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/we-are-mono/verso/internal/i18n"
	"github.com/we-are-mono/verso/internal/widget"
)

// TestI18nAudit is the deterministic untranslated-string report (`make
// i18n-audit`), not a pass/fail gate: it loads the repo's real catalogs,
// installs a recording bundle (the translators are the one seam that knows a
// key fell back to English — the i18n-pot extractor is partial by design), and
// crawls every page reachable from / and /login for each installed language.
// The crawl reads in the advanced mode: the basic reading prunes sections
// before their strings reach the translator (ADR-015), so auditing basic would
// hide exactly the strings most likely to be missing.
//
// The report is two lists per language: source strings that fell back
// (untranslated), and catalog keys no render requested (stale, or waiting on a
// page state the fake world does not reach — error paths and flashes join as
// fixtures for them exist). Plugin prose is out of reach by construction: the
// fake transport serves no real envelopes, so a plugin's own catalog is
// audited beside its renderer, not here.
//
// Blind spot, by construction: a string that never passes a translator is
// invisible — a label a template prints raw, or a literal in shell client JS.
// The contract that keeps the blind spot empty: every user-facing string a
// handler composes goes through tr at composition, and every string verso.js
// writes into the page goes through its T (fed by renderPage's jsCatalog).
func TestI18nAudit(t *testing.T) {
	if os.Getenv("VERSO_I18N_AUDIT") == "" {
		t.Skip("audit report only; run via make i18n-audit")
	}

	const i18nDir = "../../i18n"
	bundle, problems := i18n.Load(os.DirFS(i18nDir), "*/*.json")
	for _, p := range problems {
		t.Errorf("catalog problem: %v", p)
	}
	if len(bundle.Codes()) == 0 {
		t.Fatal("no catalogs installed; nothing to audit")
	}

	// seen collects every lookup per code; missing the fallbacks among them.
	// SetBundle records too: {{ t }} literals translate at template parse, once
	// per installed language.
	seen := map[string]map[string]bool{}
	missing := map[string]map[string]bool{}
	recorded := bundle.Recorded(func(code, component, key string, translated bool) {
		if seen[code] == nil {
			seen[code] = map[string]bool{}
			missing[code] = map[string]bool{}
		}
		seen[code][component+"\x00"+key] = true
		if !translated {
			missing[code][component+": "+key] = true
		}
	})

	srv := newServer(t, fakeBackend{})
	srv.SetBundle(recorded)

	token, err := srv.sessions.CreateWithMetadata("audit-sid", "root", "", "")
	if err != nil {
		t.Fatalf("session: %v", err)
	}

	href := regexp.MustCompile(`href="(/[^"#]*)`)
	for _, code := range bundle.Codes() {
		visited := map[string]bool{}
		queue := []string{"/", "/login"}
		for len(queue) > 0 {
			path := queue[0]
			queue = queue[1:]
			if visited[path] || strings.HasPrefix(path, "/assets/") {
				continue
			}
			visited[path] = true

			req := httptest.NewRequest(http.MethodGet, path, nil)
			req.Header.Set("Accept-Language", code)
			req.AddCookie(&http.Cookie{Name: sessionCookie, Value: token})
			req.AddCookie(&http.Cookie{Name: modeCookie, Value: widget.ModeAdvanced})
			rec := httptest.NewRecorder()
			srv.Handler().ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				t.Logf("[%s] %s → %d (not crawled into)", code, path, rec.Code)
				continue
			}
			for _, m := range href.FindAllStringSubmatch(rec.Body.String(), -1) {
				if next := m[1]; !visited[next] {
					queue = append(queue, next)
				}
			}
		}
		t.Logf("[%s] crawled %d pages", code, len(visited))
	}

	for _, code := range bundle.Codes() {
		miss := make([]string, 0, len(missing[code]))
		for key := range missing[code] {
			miss = append(miss, key)
		}
		sort.Strings(miss)
		t.Logf("[%s] %d untranslated strings reached a render:", code, len(miss))
		for _, key := range miss {
			t.Logf("[%s]   MISSING  %s", code, key)
		}

		stale := staleKeys(t, filepath.Join(i18nDir, code), seen[code])
		t.Logf("[%s] %d catalog keys no render requested:", code, len(stale))
		for _, key := range stale {
			t.Logf("[%s]   UNUSED   %s", code, key)
		}
	}
}

// staleKeys diffs one language's catalog files against the recorded lookups:
// a key nothing requested is either stale or waiting on an unreached state.
func staleKeys(t *testing.T, dir string, seen map[string]bool) []string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		t.Fatalf("glob %s: %v", dir, err)
	}
	var stale []string
	for _, f := range files {
		component := strings.TrimSuffix(filepath.Base(f), ".json")
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		var cat map[string]string
		if err := json.Unmarshal(data, &cat); err != nil {
			t.Fatalf("parse %s: %v", f, err)
		}
		for key := range cat {
			if !seen[component+"\x00"+key] {
				stale = append(stale, component+": "+key)
			}
		}
	}
	sort.Strings(stale)
	return stale
}
