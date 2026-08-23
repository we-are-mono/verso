// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package plugin

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"sort"
)

// Discover parses every manifest matched by glob within fsys. It is resilient by
// design (ADR-006's isolation ethos, applied at load time): a malformed,
// invalid, or duplicate manifest is skipped and reported in problems, never
// fatal — one broken plugin cannot break the shell's navigation. Manifests are
// returned sorted by discovery path so the nav is stable across boots. No
// matches yields empty results and no problems.
//
// fsys is injected (ADR-003), so discovery is unit-tested against an in-memory
// filesystem with no real plugin directory.
func Discover(fsys fs.FS, glob string) (manifests []Manifest, problems []error) {
	matches, err := fs.Glob(fsys, glob)
	if err != nil {
		return nil, []error{fmt.Errorf("plugin: bad glob %q: %w", glob, err)}
	}
	sort.Strings(matches)

	seen := make(map[string]bool)
	for _, path := range matches {
		data, err := fs.ReadFile(fsys, path)
		if err != nil {
			problems = append(problems, fmt.Errorf("plugin: read %s: %w", path, err))
			continue
		}
		var m Manifest
		if err := json.Unmarshal(data, &m); err != nil {
			problems = append(problems, fmt.Errorf("plugin: parse %s: %w", path, err))
			continue
		}
		if err := m.validate(); err != nil {
			problems = append(problems, fmt.Errorf("plugin: %s: %w", path, err))
			continue
		}
		if seen[m.ID] {
			problems = append(problems, fmt.Errorf("plugin: %s: duplicate id %q, skipped", path, m.ID))
			continue
		}
		seen[m.ID] = true
		manifests = append(manifests, m)
	}
	return manifests, problems
}
