// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import "io"

// List is a repeating text field: several values under one name, each validated
// against the same datatype. It is the schema's answer to LuCI's DynamicList — a
// stress test for the static-schema bet: repetition and per-item validation, not a
// single leaf.
//
// It renders each item as an input plus one trailing blank slot; all share Name,
// so they post as a multi-value form field. The plugin reads them, drops blanks,
// validates each, and rewrites the list. Per-item errors come back keyed by the
// item's index (as a string), which is how a repeating widget reports *which*
// row failed.
type List struct {
	Name     string            `json:"name"`
	Label    string            `json:"label"`
	Kind     string            `json:"kind"`     // item kind; "text" for now
	Style    string            `json:"style"`    // "" repeated inputs | "tokens" compact removable values
	Prompt   string            `json:"prompt"`   // token input placeholder
	Datatype string            `json:"datatype"` // tier-1 datatype for each item
	Items    []string          `json:"items"`
	Errors   map[string]string `json:"errors"` // index (string) -> error
	Help     string            `json:"help"`
}

func (*List) isWidget() {}

func (l *List) renderInto(r *Renderer, out io.Writer, _ string) error {
	return r.execute(out, "list.html.tmpl", l)
}
