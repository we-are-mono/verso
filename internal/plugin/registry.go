// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package plugin

import (
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// Registry is what the shell reads of a plugin's commands.json (ADR-027): the
// root commands the privileged helper runs for it, the version of each read,
// the fields of a read's result that hold a certificate, and the events its
// root half announces (ADR-028). The helper re-reads and checks the file
// itself before it runs anything; the shell reads it only to know what to
// broker and how to hand it on, and what an announced event says.
type Registry struct {
	Commands     []RegisteredCommand `json:"commands"`
	Certificates map[string][]string `json:"certificates,omitempty"`
	Events       []EventDeclaration  `json:"events,omitempty"`
}

// RegisteredCommand is one root command: a read (run for a render, versioned),
// an act (run on a POST) or a transform (the helper's own).
type RegisteredCommand struct {
	Name    string `json:"name"`
	Kind    string `json:"kind"`
	Version int    `json:"version,omitempty"`
}

// EventDeclaration is one event a plugin's root half announces (ADR-028): its
// name, the English sentence it reads as, with %s for each argument and a key
// of the plugin's catalog, its tone, the registered read whose grant lets a
// session see it, whether it is said to the operators looking (off unless
// declared), whether it alerts, and the plugin's events it ends. Its name is
// its contract: a changed meaning is a new name.
type EventDeclaration struct {
	Name     string   `json:"name"`
	Sentence string   `json:"sentence"`
	Tone     string   `json:"tone"`
	Read     string   `json:"read"`
	Notify   bool     `json:"notify,omitempty"`
	Alert    bool     `json:"alert,omitempty"`
	Ends     []string `json:"ends,omitempty"`
}

// EventTones are the tones an event may say itself in: the shell's tone
// vocabulary, less neutral, since an event is news.
var EventTones = []string{"success", "info", "warning", "danger"}

// eventName is what an event may be called.
var eventName = regexp.MustCompile(`^[a-z0-9-]{1,64}$`)

// sentenceLimit is the longest sentence an event declares.
const sentenceLimit = 512

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

// Event is the declared event named name, if the registry declares one.
func (r *Registry) Event(name string) (EventDeclaration, bool) {
	if r == nil {
		return EventDeclaration{}, false
	}
	for _, e := range r.Events {
		if e.Name == name {
			return e, true
		}
	}
	return EventDeclaration{}, false
}

// ParseRegistry decodes a commands.json, refusing one whose reads carry no
// version: a read handed on without one could not be checked by its reader.
// An event declaration that breaks a rule is dropped alone, its reason among
// dropped, and the registry kept.
func ParseRegistry(data []byte) (r *Registry, dropped []error, err error) {
	var parsed Registry
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil, nil, err
	}
	for _, c := range parsed.Commands {
		if c.Kind == "read" && c.Version < 1 {
			return nil, nil, fmt.Errorf("read %q has no version", c.Name)
		}
	}
	parsed.Events, dropped = keptEvents(&parsed)
	return &parsed, dropped, nil
}

// keptEvents is the registry's event declarations that keep every rule, and
// why each other one was dropped. An end is checked against the declarations
// kept, until none is dropped for its ends, so an event cannot end one that
// was dropped.
func keptEvents(r *Registry) (kept []EventDeclaration, dropped []error) {
	seen := map[string]bool{}
	for _, e := range r.Events {
		var why string
		switch _, read := r.Read(e.Read); {
		case !eventName.MatchString(e.Name):
			why = "its name must be lowercase letters, digits and dashes, at most 64"
		case seen[e.Name]:
			why = "its name is declared twice"
		case e.Sentence == "" || len(e.Sentence) > sentenceLimit || strings.ContainsAny(e.Sentence, "\r\n"):
			why = "its sentence must be one line of at most 512 bytes"
		case !slices.Contains(EventTones, e.Tone):
			why = fmt.Sprintf("its tone %q is not one of %s", e.Tone, strings.Join(EventTones, ", "))
		case !read:
			why = fmt.Sprintf("its read %q is not registered", e.Read)
		}
		if why != "" {
			dropped = append(dropped, fmt.Errorf("event %q dropped: %s", e.Name, why))
			if eventName.MatchString(e.Name) {
				seen[e.Name] = true
			}
			continue
		}
		seen[e.Name] = true
		kept = append(kept, e)
	}
	for {
		declared := map[string]bool{}
		for _, e := range kept {
			declared[e.Name] = true
		}
		var ended []EventDeclaration
		for _, e := range kept {
			if i := slices.IndexFunc(e.Ends, func(end string) bool { return end == e.Name || !declared[end] }); i >= 0 {
				dropped = append(dropped, fmt.Errorf("event %q dropped: its ends name %q, which is not another event it declares", e.Name, e.Ends[i]))
				continue
			}
			ended = append(ended, e)
		}
		if len(ended) == len(kept) {
			return kept, dropped
		}
		kept = ended
	}
}
