// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package plugin

import (
	"encoding/json"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"
)

// serviceNameRe is the shape of an init-script name, the only thing that
// reaches procd's rc object.
var serviceNameRe = regexp.MustCompile(`^[a-z0-9][a-zA-Z0-9._-]{0,63}$`)

// ServiceName says whether name is an init script's name.
func ServiceName(name string) bool { return serviceNameRe.MatchString(name) }

// readServices sets the plugin's words for the services it brings, from the
// manifest's own words, one entry at a time: an entry whose key is no
// init-script name, or whose words are not text, is logged and dropped, and a
// services that is not an object is dropped whole. The plugin loads either way.
func (m *Manifest) readServices(data []byte) (problems []error) {
	var raw struct {
		Services json.RawMessage `json:"services"`
	}
	if json.Unmarshal(data, &raw) != nil || raw.Services == nil {
		return nil
	}
	var entries map[string]json.RawMessage
	if err := json.Unmarshal(raw.Services, &entries); err != nil {
		return []error{fmt.Errorf("plugin %q services must map init scripts to words, read as none", m.ID)}
	}
	for _, script := range slices.Sorted(maps.Keys(entries)) {
		var words string
		switch {
		case !ServiceName(script):
			problems = append(problems, fmt.Errorf("plugin %q services: %q is not an init script's name, dropped", m.ID, script))
		case json.Unmarshal(entries[script], &words) != nil || strings.TrimSpace(words) == "":
			problems = append(problems, fmt.Errorf("plugin %q services: %q has no words, dropped", m.ID, script))
		default:
			if m.Services == nil {
				m.Services = map[string]string{}
			}
			m.Services[script] = words
		}
	}
	return problems
}
