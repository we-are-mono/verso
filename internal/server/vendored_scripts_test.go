// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"os"
	"path"
	"strings"
	"testing"
)

// vendoredJSManifest is assets/vendored-js.json: where each third-party script
// the shell serves came from, at which version, and its SHA-256. The file is
// in osv-scanner's custom-lockfile shape, so `make vuln` checks the same
// package versions against the advisory database that this test pins.
const vendoredJSManifest = "assets/vendored-js.json"

type vendoredScript struct {
	Package struct {
		Name      string `json:"name"`
		Version   string `json:"version"`
		Ecosystem string `json:"ecosystem"`
	} `json:"package"`
	File   string `json:"file"`
	URL    string `json:"url"`
	SHA256 string `json:"sha256"`
}

func readVendoredJS(t *testing.T) []vendoredScript {
	t.Helper()
	raw, err := os.ReadFile(vendoredJSManifest)
	if err != nil {
		t.Fatalf("read %s: %v", vendoredJSManifest, err)
	}
	var m struct {
		Results []struct {
			Packages []vendoredScript `json:"packages"`
		} `json:"results"`
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("parse %s: %v", vendoredJSManifest, err)
	}
	var out []vendoredScript
	for _, r := range m.Results {
		out = append(out, r.Packages...)
	}
	if len(out) == 0 {
		t.Fatalf("%s records no scripts", vendoredJSManifest)
	}
	return out
}

// TestVendoredScriptsMatchTheirRecord hashes every third-party script the
// binary embeds against the SHA-256 the manifest records. A file swapped,
// edited or upgraded without its record fails here, so the version the
// advisory check reads is the version that ships.
func TestVendoredScriptsMatchTheirRecord(t *testing.T) {
	for _, s := range readVendoredJS(t) {
		t.Run(s.File, func(t *testing.T) {
			if s.Package.Ecosystem != "npm" || s.Package.Name == "" || s.Package.Version == "" {
				t.Fatalf("record names no npm package and version: %+v", s.Package)
			}
			if !strings.Contains(s.URL, s.Package.Name+"@"+s.Package.Version+"/") {
				t.Errorf("url %s is not the release of %s@%s", s.URL, s.Package.Name, s.Package.Version)
			}
			body, err := scriptFS.ReadFile(path.Join("assets", s.File))
			if err != nil {
				t.Fatalf("%s is recorded but not embedded: %v", s.File, err)
			}
			sum := sha256.Sum256(body)
			if got := hex.EncodeToString(sum[:]); got != s.SHA256 {
				t.Errorf("sha256 = %s, the record says %s", got, s.SHA256)
			}
		})
	}
}

// TestEveryVendoredScriptIsRecorded: every embedded script that is not the
// shell's own (verso*.js) is third-party, and ships only with a record of
// where it came from.
func TestEveryVendoredScriptIsRecorded(t *testing.T) {
	recorded := map[string]bool{}
	for _, s := range readVendoredJS(t) {
		recorded[s.File] = true
	}
	scripts, err := fs.Glob(scriptFS, "assets/*.js")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range scripts {
		name := path.Base(p)
		if strings.HasPrefix(name, "verso") {
			continue
		}
		if !recorded[name] {
			t.Errorf("%s is embedded with no record in %s", name, vendoredJSManifest)
		}
	}
}
