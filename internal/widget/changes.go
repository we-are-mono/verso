// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import "io"

// Changes is a compact, human-facing diff. It names the field that changed and
// presents its original and proposed values side by side. The staged-changes
// capsule uses the same anatomy as the styleguide preview, so its review popup
// never falls back to exposing UCI tuples to the user.
type Changes struct {
	Compact bool          `json:"compact,omitempty"`
	Items   []Change      `json:"items"`
	Groups  []ChangeGroup `json:"groups,omitempty"`
}

type Change struct {
	Label    string `json:"label"`
	Previous string `json:"previous"`
	Next     string `json:"next"`
}

// ChangeGroup is one user-level object or action. Summary carries the compact
// effective configuration; Values hold the optional full review shown on
// demand. This keeps one firewall rule equal to one pending change regardless
// of how many backend options describe it.
type ChangeGroup struct {
	Label     string        `json:"label"`
	Operation string        `json:"operation"`
	Summary   string        `json:"summary,omitempty"`
	Expanded  bool          `json:"expanded,omitempty"`
	Values    []ChangeValue `json:"values,omitempty"`
}

type ChangeValue struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

func (*Changes) isWidget() {}

func (c *Changes) renderInto(r *Renderer, out io.Writer, _ string) error {
	return r.execute(out, "changes.html.tmpl", c)
}
