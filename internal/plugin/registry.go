// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package plugin

import (
	"encoding/json"
	"fmt"
)

// Registry is what the shell reads of a plugin's commands.json (ADR-027): the
// root commands the privileged helper runs for it, the version of each read,
// and the fields of a read's result that hold a certificate. The helper
// re-reads and checks the file itself before it runs anything; the shell reads
// it only to know what to broker and how to hand it on.
type Registry struct {
	Commands     []RegisteredCommand `json:"commands"`
	Certificates map[string][]string `json:"certificates,omitempty"`
}

// RegisteredCommand is one root command: a read (run for a render, versioned),
// an act (run on a POST) or a transform (the helper's own).
type RegisteredCommand struct {
	Name    string `json:"name"`
	Kind    string `json:"kind"`
	Version int    `json:"version,omitempty"`
}

// Read is the registered read named name, if the registry has one.
func (r *Registry) Read(name string) (RegisteredCommand, bool) {
	if r == nil {
		return RegisteredCommand{}, false
	}
	for _, c := range r.Commands {
		if c.Name == name && c.Kind == "read" {
			return c, true
		}
	}
	return RegisteredCommand{}, false
}

// parseRegistry decodes a commands.json, refusing one whose reads carry no
// version: a read handed on without one could not be checked by its reader.
func parseRegistry(data []byte) (*Registry, error) {
	var r Registry
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, err
	}
	for _, c := range r.Commands {
		if c.Kind == "read" && c.Version < 1 {
			return nil, fmt.Errorf("read %q has no version", c.Name)
		}
	}
	return &r, nil
}
