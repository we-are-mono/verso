// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package plugin

import (
	"slices"
	"strings"
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
	return `{"id":"` + id + `","name":"Name ` + id +
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
		"plugins/x/manifest.json": `{"id":"x","schema_version":1}`,
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
		"plugins/x/manifest.json": `{"id":"../etc","name":"N",` +
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
		"plugins/rel/manifest.json": `{"id":"rel","name":"N","socket":"relative.sock",` +
			`"schema_version":1,"nav":[{"section":"S","label":"L","path":"/"}]}`,
		"plugins/dot/manifest.json": `{"id":"dot","name":"N","socket":"/var/run/../etc/x.sock",` +
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
		"plugins/x/manifest.json": `{"id":"x","name":"N",` +
			`"socket":"/s","schema_version":1,"nav":[]}`,
	})

	got, problems := Discover(fsys, testGlob)
	if len(got) != 0 || len(problems) != 1 {
		t.Errorf("a manifest with no nav entries must be skipped: got=%d problems=%d", len(got), len(problems))
	}
}

func TestDiscoverOverviewCardAndEmptyNav(t *testing.T) {
	fsys := mapFS(map[string]string{
		"plugins/odu/manifest.json": `{"id":"odu","name":"5G ODU","socket":"/run/odu.sock","schema_version":1,"nav":[],"overview":{"path":"/overview","icon":"radio"}}`,
	})
	got, problems := Discover(fsys, testGlob)
	if len(problems) != 0 || len(got) != 1 || got[0].Overview == nil || got[0].Overview.Path != "/overview" || got[0].Overview.Icon != "radio" {
		t.Fatalf("overview manifest = %+v, problems=%v", got, problems)
	}
}

func TestDiscoverRejectsInvalidOverview(t *testing.T) {
	for _, overview := range []string{`{"path":"/","icon":"radio"}`, `{"path":"/overview","icon":""}`} {
		fsys := mapFS(map[string]string{
			"plugins/x/manifest.json": `{"id":"x","name":"X","socket":"/run/x.sock","schema_version":1,"nav":[],"overview":` + overview + `}`,
		})
		if got, problems := Discover(fsys, testGlob); len(got) != 0 || len(problems) != 1 {
			t.Errorf("overview %s: got=%d problems=%v", overview, len(got), problems)
		}
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
		"plugins/w/manifest.json": `{"id":"w","name":"N","socket":"/run/verso/w.sock",` +
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
		"plugins/x/manifest.json": `{"id":"x","name":"N","socket":"/run/verso/x.sock",` +
			`"schema_version":1,"nav":[{"section":"S","label":"L","path":"/"}],` +
			`"acl":{"write":[{"scope":"uci","object":"system"}]}}`,
	})

	got, problems := Discover(fsys, testGlob)
	if len(got) != 0 || len(problems) != 1 {
		t.Errorf("an incomplete acl entry must be skipped: got=%d problems=%d", len(got), len(problems))
	}
}

// TestDiscoverParsesReadACL: a plugin declaring the configs it reads parses into
// structured acl.read entries the shell pre-reads and brokers as a snapshot (ADR-007).
func TestDiscoverParsesReadACL(t *testing.T) {
	fsys := mapFS(map[string]string{
		"plugins/r/manifest.json": `{"id":"r","name":"N","socket":"/run/verso/r.sock",` +
			`"schema_version":1,"nav":[{"section":"S","label":"L","path":"/"}],` +
			`"acl":{"read":[{"scope":"uci","object":"network","function":"read"}]}}`,
	})

	got, problems := Discover(fsys, testGlob)
	if len(problems) != 0 || len(got) != 1 {
		t.Fatalf("want one clean manifest, got=%d problems=%v", len(got), problems)
	}
	r := got[0].ACL.Read
	if len(r) != 1 || r[0].Scope != "uci" || r[0].Object != "network" || r[0].Function != "read" {
		t.Errorf("acl.read not parsed: %+v", r)
	}
}

// TestDiscoverRejectsIncompleteReadACL: a read scope missing a field names no
// config to fetch, so it is a load error rather than a silently-carried grant.
func TestDiscoverRejectsIncompleteReadACL(t *testing.T) {
	fsys := mapFS(map[string]string{
		"plugins/y/manifest.json": `{"id":"y","name":"N","socket":"/run/verso/y.sock",` +
			`"schema_version":1,"nav":[{"section":"S","label":"L","path":"/"}],` +
			`"acl":{"read":[{"scope":"uci","object":"network"}]}}`,
	})

	got, problems := Discover(fsys, testGlob)
	if len(got) != 0 || len(problems) != 1 {
		t.Errorf("an incomplete acl.read entry must be skipped: got=%d problems=%d", len(got), len(problems))
	}
}

// TestDiscoverParsesSecrets: a plugin names the options that hold secrets, as
// config.option, so config history keeps that they changed and never their
// values (ADR-021).
func TestDiscoverParsesSecrets(t *testing.T) {
	fsys := mapFS(map[string]string{
		"plugins/s/manifest.json": `{"id":"s","name":"N","socket":"/run/verso/s.sock",` +
			`"schema_version":1,"nav":[{"section":"S","label":"L","path":"/"}],` +
			`"secrets":["network.password","ddns.password"]}`,
	})

	got, problems := Discover(fsys, testGlob)
	if len(problems) != 0 || len(got) != 1 {
		t.Fatalf("want one clean manifest, got=%d problems=%v", len(got), problems)
	}
	if s := got[0].Secrets; len(s) != 2 || s[0] != "network.password" || s[1] != "ddns.password" {
		t.Errorf("secrets not parsed: %+v", s)
	}
}

// TestDiscoverRejectsAMalformedSecret: a secret that names no config and
// option would protect nothing, so it is a load error rather than a value
// that quietly lands in the history.
func TestDiscoverRejectsAMalformedSecret(t *testing.T) {
	for _, secret := range []string{"password", "network.", ".password", "network.wan.password", "net work.password"} {
		fsys := mapFS(map[string]string{
			"plugins/z/manifest.json": `{"id":"z","name":"N","socket":"/run/verso/z.sock",` +
				`"schema_version":1,"nav":[{"section":"S","label":"L","path":"/"}],` +
				`"secrets":["` + secret + `"]}`,
		})
		if got, problems := Discover(fsys, testGlob); len(got) != 0 || len(problems) != 1 {
			t.Errorf("secret %q: got=%d problems=%d, want it skipped", secret, len(got), len(problems))
		}
	}
}

// TestDiscoverRejectsAKeySpelledTwice: encoding/json takes the last of two
// spellings of one field, merging two objects, where verso-rpcd's reading
// would keep both or the last whole, so the two would take different secrets
// from one manifest (ADR-021). A manifest spelling any key twice in one
// object, in the same case or another, is not read.
func TestDiscoverRejectsAKeySpelledTwice(t *testing.T) {
	const head = `{"id":"z","name":"N","socket":"/run/verso/z.sock","schema_version":1,`
	const nav = `"nav":[{"section":"S","label":"L","path":"/"}]`
	const write = `"acl":{"write":[{"scope":"uci","object":"vpn","function":"write"}]}`
	for name, body := range map[string]string{
		"secrets twice":        head + nav + `,` + write + `,"secrets":["vpn.a"],"secrets":["vpn.b"]}`,
		"acl twice":            head + nav + `,` + write + `,"acl":{"read":[]},"secrets":["vpn.a"]}`,
		"acl in another case":  head + nav + `,` + write + `,"ACL":{"read":[]},"secrets":["vpn.a"]}`,
		"an escaped spelling":  head + nav + `,` + write + `,"secrets":["vpn.a"],"\` + `u0073ecrets":["vpn.b"]}`,
		"a Kelvin sign":        head + nav + `,` + write + `,"socKet":"/run/verso/y.sock"}`,
		"a nav entry's pages":  head + `"nav":[{"section":"S","label":"L","path":"/","pages":true,"pages":false}],` + write + `}`,
		"a nav entry's label":  head + `"nav":[{"section":"S","label":"L","path":"/","LABEL":"M"}],` + write + `}`,
		"a scope's object":     head + nav + `,"acl":{"write":[{"scope":"uci","object":"vpn","objecT":"x","function":"write"}]}}`,
		"a key no field takes": head + nav + `,"x-note":1,"X-Note":2}`,
	} {
		fsys := mapFS(map[string]string{"plugins/z/manifest.json": body})
		if got, problems := Discover(fsys, testGlob); len(got) != 0 || len(problems) != 1 {
			t.Errorf("%s: got=%d problems=%v, want it skipped", name, len(got), problems)
		}
	}

	// A dotless i is no i: two keys, not one spelled twice.
	fsys := mapFS(map[string]string{"plugins/z/manifest.json": head + nav + `,` + write + `,"wrıte":1}`})
	if got, problems := Discover(fsys, testGlob); len(got) != 1 || len(problems) != 0 {
		t.Errorf("a dotless i: got=%d problems=%v, want it read", len(got), problems)
	}
}

// TestDiscoverRejectsWhatTheHelperCannotRead: verso-rpcd's JSON reader
// refuses a value nested deeper than 127 and a \u escape of half a surrogate
// pair, where encoding/json reads the one and takes the other for U+FFFD, so
// the helper would take none of a manifest's secrets the shell scrubs
// (ADR-021). Neither is read; a pair, and an escaped backslash before u, are.
func TestDiscoverRejectsWhatTheHelperCannotRead(t *testing.T) {
	const head = `{"id":"z","name":"N","socket":"/run/verso/z.sock","schema_version":1,` +
		`"nav":[{"section":"S","label":"L","path":"/"}],` +
		`"acl":{"write":[{"scope":"uci","object":"vpn","function":"write"}]},"secrets":["vpn.a"],`
	nested := func(depth int) string {
		// The manifest is one level; the rest are arrays.
		return head + `"x":` + strings.Repeat("[", depth-1) + strings.Repeat("]", depth-1) + `}`
	}
	for name, c := range map[string]struct {
		body string
		read bool
	}{
		"nested 127 deep":                  {nested(127), true},
		"nested 128 deep":                  {nested(128), false},
		"a leading surrogate alone":        {head + `"x":"\ud800"}`, false},
		"a leading surrogate at the end":   {head + `"x":"a\udbff"}`, false},
		"a trailing surrogate alone":       {head + `"x":"\udc00"}`, false},
		"a leading surrogate, then a char": {head + `"x":"\ud800A"}`, false},
		"a leading surrogate, then \\n":    {head + `"x":"\ud800\n"}`, false},
		"a surrogate in a key":             {head + `"\ud800":1}`, false},
		"a surrogate pair":                 {head + `"x":"😀"}`, true},
		"an escaped surrogate pair":        {head + `"x":"\ud83d` + `\ude00"}`, true},
		"an escaped backslash before u":    {head + `"x":"\\ud800"}`, true},
		"an escaped quote before it":       {head + `"x":"\"\\ud800"}`, true},
	} {
		fsys := mapFS(map[string]string{"plugins/z/manifest.json": c.body})
		got, problems := Discover(fsys, testGlob)
		if read := len(got) == 1 && len(problems) == 0; read != c.read {
			t.Errorf("%s: got=%d problems=%v, want read %v", name, len(got), problems, c.read)
		}
	}
}

// TestDiscoverParsesNavIconAndPages: a nav entry may name the shell glyph its row
// draws (ADR-005) and say it opens into subpages. Both are optional — an entry
// declaring neither is the ordinary case and parses to empty.
func TestDiscoverParsesNavIconAndPages(t *testing.T) {
	fsys := mapFS(map[string]string{
		"plugins/n/manifest.json": `{"id":"n","name":"N","socket":"/run/verso/n.sock",` +
			`"schema_version":1,"nav":[` +
			`{"section":"Network","label":"DNS","path":"/dns","icon":"globe","pages":true},` +
			`{"section":"Network","label":"DHCP","path":"/"},` +
			`{"section":"Security","label":"Firewall","path":"/","pages":[{"label":"Rules","path":"/"},{"label":"Zones","path":"zones"}]}]}`,
	})

	got, problems := Discover(fsys, testGlob)
	if len(problems) != 0 || len(got) != 1 {
		t.Fatalf("want one clean manifest, got=%d problems=%v", len(got), problems)
	}
	nav := got[0].Nav
	if nav[0].Icon != "globe" || !nav[0].Pages.Opens || nav[0].Pages.List != nil {
		t.Errorf("nav[0] icon/pages not parsed: %+v", nav[0])
	}
	if nav[1].Icon != "" || nav[1].Pages.Opens {
		t.Errorf("nav[1] should declare none of them: %+v", nav[1])
	}
	if want := []PageTab{{Label: "Rules", Path: "/"}, {Label: "Zones", Path: "zones"}}; !nav[2].Pages.Opens || !slices.Equal(nav[2].Pages.List, want) {
		t.Errorf("nav[2] subpages = %+v, want %+v", nav[2].Pages, want)
	}
}

// TestDiscoverRejectsASubpageOutsideTheMount: a declared subpage needs a label
// and a clean path under the plugin's mount, so the rail cannot be aimed
// elsewhere.
func TestDiscoverRejectsASubpageOutsideTheMount(t *testing.T) {
	for name, page := range map[string]string{
		"no label":  `{"label":"","path":"zones"}`,
		"traversal": `{"label":"L","path":"../system"}`,
		"query":     `{"label":"L","path":"zones?x=1"}`,
		"double":    `{"label":"L","path":"a//b"}`,
		"backslash": `{"label":"L","path":"a\\b"}`,
	} {
		fsys := mapFS(map[string]string{
			"plugins/f/manifest.json": `{"id":"f","name":"F","socket":"/run/verso/f.sock","schema_version":1,` +
				`"nav":[{"section":"Security","label":"Firewall","path":"/","pages":[` + page + `]}]}`,
		})
		got, problems := Discover(fsys, testGlob)
		if len(got) != 0 || len(problems) != 1 || !strings.Contains(problems[0].Error(), `"f" nav[0]`) {
			t.Errorf("%s: got=%d problems=%v", name, len(got), problems)
		}
	}
}

func TestDiscoverEntityOnlyPlugin(t *testing.T) {
	fsys := mapFS(map[string]string{
		"plugins/qos/manifest.json": `{"id":"qos","name":"Device limits","socket":"/s","schema_version":1,"nav":[],"entity_tabs":[{"entity":"device","slot":"shape","label":"Limits","summaries":true}]}`,
	})
	got, problems := Discover(fsys, testGlob)
	if len(got) != 1 || len(problems) != 0 {
		t.Fatalf("entity-only plugin: got=%v problems=%v", got, problems)
	}
	if !got[0].EntityTabs[0].Summaries {
		t.Error("bulk summaries capability was lost")
	}
}
