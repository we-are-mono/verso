// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package plugin

import (
	"testing"
	"testing/fstest"
)

const testGlob = "plugins/*/manifest.json"

func mapFS(files map[string]string) fstest.MapFS {
	m := fstest.MapFS{}
	for name, body := range files {
		m[name] = &fstest.MapFile{Data: []byte(body)}
	}
	return m
}

func validManifest(id string) string {
	return `{"manifest_version":1,"id":"` + id + `","name":"Name ` + id +
		`","socket":"/run/verso/` + id + `.sock","schema_version":1,` +
		`"nav":[{"section":"System","label":"Label","path":"/"}]}`
}

func TestDiscoverLoadsValidManifests(t *testing.T) {
	fsys := mapFS(map[string]string{
		"plugins/a/manifest.json": validManifest("a"),
		"plugins/b/manifest.json": validManifest("b"),
	})

	got, problems := Discover(fsys, testGlob)
	if len(problems) != 0 {
		t.Fatalf("problems = %v, want none", problems)
	}
	if len(got) != 2 {
		t.Fatalf("manifests = %d, want 2", len(got))
	}
	// Deterministic order (sorted) so nav is stable across boots.
	if got[0].ID != "a" || got[1].ID != "b" {
		t.Errorf("order = %q,%q, want a,b", got[0].ID, got[1].ID)
	}
	if got[0].Socket != "/run/verso/a.sock" || got[0].Nav[0].Section != "System" {
		t.Errorf("fields not parsed: %+v", got[0])
	}
}

// TestDiscoverSkipsMalformedButKeepsRest is the isolation ethos at load time: one
// broken plugin dir must not break the shell's whole navigation.
func TestDiscoverSkipsMalformedButKeepsRest(t *testing.T) {
	fsys := mapFS(map[string]string{
		"plugins/good/manifest.json": validManifest("good"),
		"plugins/bad/manifest.json":  `{ not json`,
	})

	got, problems := Discover(fsys, testGlob)
	if len(got) != 1 || got[0].ID != "good" {
		t.Fatalf("want only 'good' loaded, got %+v", got)
	}
	if len(problems) != 1 {
		t.Errorf("problems = %d, want 1", len(problems))
	}
}

func TestDiscoverSkipsMissingRequiredField(t *testing.T) {
	fsys := mapFS(map[string]string{
		// no name, socket, or nav
		"plugins/x/manifest.json": `{"manifest_version":1,"id":"x","schema_version":1}`,
	})

	got, problems := Discover(fsys, testGlob)
	if len(got) != 0 {
		t.Errorf("want none loaded, got %+v", got)
	}
	if len(problems) != 1 {
		t.Errorf("problems = %d, want 1", len(problems))
	}
}

func TestDiscoverRejectsDuplicateID(t *testing.T) {
	fsys := mapFS(map[string]string{
		"plugins/a1/manifest.json": validManifest("dup"),
		"plugins/a2/manifest.json": validManifest("dup"),
	})

	got, problems := Discover(fsys, testGlob)
	if len(got) != 1 {
		t.Fatalf("want first kept only, got %d", len(got))
	}
	if len(problems) != 1 {
		t.Errorf("problems = %d, want 1 (the duplicate)", len(problems))
	}
}

// TestDiscoverRejectsUnsafeID guards the /plugins/<id>/ mount and the socket
// namespace: an id with path characters could escape either.
func TestDiscoverRejectsUnsafeID(t *testing.T) {
	fsys := mapFS(map[string]string{
		"plugins/x/manifest.json": `{"manifest_version":1,"id":"../etc","name":"N",` +
			`"socket":"/s","schema_version":1,"nav":[{"section":"S","label":"L","path":"/"}]}`,
	})

	got, problems := Discover(fsys, testGlob)
	if len(got) != 0 || len(problems) != 1 {
		t.Errorf("unsafe id must be skipped: got=%d problems=%d", len(got), len(problems))
	}
}

// TestDiscoverRejectsBadSocketPath: the socket is dialed as-is, so a relative or
// traversal path is rejected (VS-09).
func TestDiscoverRejectsBadSocketPath(t *testing.T) {
	fsys := mapFS(map[string]string{
		"plugins/rel/manifest.json": `{"manifest_version":1,"id":"rel","name":"N","socket":"relative.sock",` +
			`"schema_version":1,"nav":[{"section":"S","label":"L","path":"/"}]}`,
		"plugins/dot/manifest.json": `{"manifest_version":1,"id":"dot","name":"N","socket":"/var/run/../etc/x.sock",` +
			`"schema_version":1,"nav":[{"section":"S","label":"L","path":"/"}]}`,
	})

	got, problems := Discover(fsys, testGlob)
	if len(got) != 0 {
		t.Errorf("bad socket paths must be skipped, got %+v", got)
	}
	if len(problems) != 2 {
		t.Errorf("problems = %d, want 2", len(problems))
	}
}

func TestDiscoverRejectsEmptyNav(t *testing.T) {
	fsys := mapFS(map[string]string{
		"plugins/x/manifest.json": `{"manifest_version":1,"id":"x","name":"N",` +
			`"socket":"/s","schema_version":1,"nav":[]}`,
	})

	got, problems := Discover(fsys, testGlob)
	if len(got) != 0 || len(problems) != 1 {
		t.Errorf("a manifest with no nav entries must be skipped: got=%d problems=%d", len(got), len(problems))
	}
}

func TestDiscoverNoPluginsIsClean(t *testing.T) {
	got, problems := Discover(mapFS(nil), testGlob)
	if len(got) != 0 || len(problems) != 0 {
		t.Errorf("empty dir: got=%d problems=%d, want 0/0", len(got), len(problems))
	}
}

// TestDiscoverParsesACL: a plugin declaring the rpcd write scopes it needs parses
// into structured ACL entries the shell gates on (ADR-007).
func TestDiscoverParsesACL(t *testing.T) {
	fsys := mapFS(map[string]string{
		"plugins/w/manifest.json": `{"manifest_version":1,"id":"w","name":"N","socket":"/run/verso/w.sock",` +
			`"schema_version":1,"nav":[{"section":"S","label":"L","path":"/"}],` +
			`"acl":{"write":[{"scope":"uci","object":"system","function":"write"}]}}`,
	})

	got, problems := Discover(fsys, testGlob)
	if len(problems) != 0 || len(got) != 1 {
		t.Fatalf("want one clean manifest, got=%d problems=%v", len(got), problems)
	}
	w := got[0].ACL.Write
	if len(w) != 1 || w[0].Scope != "uci" || w[0].Object != "system" || w[0].Function != "write" {
		t.Errorf("acl.write not parsed: %+v", w)
	}
}

// TestDiscoverRejectsIncompleteACL: a write scope missing a field is a load error,
// not a silently-ignored (and therefore ungated) grant.
func TestDiscoverRejectsIncompleteACL(t *testing.T) {
	fsys := mapFS(map[string]string{
		"plugins/x/manifest.json": `{"manifest_version":1,"id":"x","name":"N","socket":"/run/verso/x.sock",` +
			`"schema_version":1,"nav":[{"section":"S","label":"L","path":"/"}],` +
			`"acl":{"write":[{"scope":"uci","object":"system"}]}}`,
	})

	got, problems := Discover(fsys, testGlob)
	if len(got) != 0 || len(problems) != 1 {
		t.Errorf("an incomplete acl entry must be skipped: got=%d problems=%d", len(got), len(problems))
	}
}
