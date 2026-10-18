// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package plugin

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
)

// ShellID is the owner of the slots the shell's own pages open. No plugin may
// take it as its id, so a slot named shell.… is always the shell's.
const ShellID = "shell"

// AccessBlocks is the slot of Access's blocks, where a plugin's part of the
// page stands.
const AccessBlocks = "shell.access.blocks"

// DefaultOrder is a contribution's order when its manifest names none.
const DefaultOrder = 100

// Contribution is one thing a plugin puts in a named slot (ADR-026): what the
// shell fetches from Path on the plugin and where in the slot it stands. The
// slot may be a shell page's or another plugin's, which is how plugins extend
// each other without knowing each other.
type Contribution struct {
	// ID is unique within the plugin; with the plugin's id it names the
	// contribution everywhere else, as an anchor and as an Overview layout id.
	ID       string
	Slot     string
	Path     string
	Position Position
	// Order breaks ties among contributions at one place; then plugin id,
	// then contribution id.
	Order int
}

// Name is the contribution's full id, <plugin id>.<contribution id>.
func (c Contribution) Name(plugin string) string { return plugin + "." + c.ID }

// Where is the place in a slot a position names.
type Where int

const (
	Last Where = iota
	First
	Before
	After
)

// Position is where a contribution stands in its slot: first, last, or
// directly before or after an anchor, one of the slot's own items or another
// contribution's full id.
type Position struct {
	Where  Where
	Anchor string
}

// ParsePosition reads a manifest's position: "first", "last" (also the empty
// string), "before:<anchor>" or "after:<anchor>".
func ParsePosition(s string) (Position, error) {
	switch s {
	case "", "last":
		return Position{Where: Last}, nil
	case "first":
		return Position{Where: First}, nil
	}
	where, anchor, ok := strings.Cut(s, ":")
	if !ok || !anchorName(anchor) {
		return Position{}, fmt.Errorf("position %q must be first, last, before:<anchor> or after:<anchor>", s)
	}
	switch where {
	case "before":
		return Position{Where: Before, Anchor: anchor}, nil
	case "after":
		return Position{Where: After, Anchor: anchor}, nil
	}
	return Position{}, fmt.Errorf("position %q must be first, last, before:<anchor> or after:<anchor>", s)
}

// SlotOwner is the id of whoever opened the named slot, the part before its
// first dot: "shell" for a shell page's, a plugin's id for one its page opens.
// A name with no owner, or with a part that is not lowercase letters, digits,
// hyphens and underscores, is no slot's.
func SlotOwner(name string) (string, bool) {
	parts := strings.Split(name, ".")
	if len(parts) < 2 {
		return "", false
	}
	for _, p := range parts {
		if !nameWord(p, true) {
			return "", false
		}
	}
	return parts[0], true
}

// anchorName is an anchor: a slot's own item (a word) or a contribution's
// full id (two words).
func anchorName(s string) bool {
	parts := strings.Split(s, ".")
	if len(parts) > 2 {
		return false
	}
	for _, p := range parts {
		if !nameWord(p, true) {
			return false
		}
	}
	return true
}

// nameWord is one part of a slot or contribution name: lowercase letters,
// digits and hyphens, and with under also underscores, which plugin ids may
// carry.
func nameWord(s string, under bool) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		ok := r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' || under && r == '_'
		if !ok {
			return false
		}
	}
	return true
}

// localPath is a clean path on the plugin, as the shell sends it: rooted, with
// no query, fragment, backslash or step out of the mount.
func localPath(p string) bool {
	return strings.HasPrefix(p, "/") && !strings.HasPrefix(p, "//") && !strings.ContainsAny(p, "?#\\") && p == filepath.Clean(p)
}

// rawContribution is a contributes entry as the manifest spells it, read one
// entry at a time so a bad one is skipped and the rest kept.
type rawContribution struct {
	ID       string `json:"id"`
	Slot     string `json:"slot"`
	Path     string `json:"path"`
	Position string `json:"position"`
	Order    *int   `json:"order"`
}

// readContributes fills m.Contributes from the manifest's contributes list,
// skipping each entry that is not one with its reason, then reads a
// system_access part as the contribution to Access it is.
func (m *Manifest) readContributes(data []byte) (problems []error) {
	var list struct {
		Contributes []json.RawMessage `json:"contributes"`
	}
	if err := json.Unmarshal(data, &list); err != nil {
		return []error{fmt.Errorf("plugin %q contributes must be a list: %w", m.ID, err)}
	}
	seen := map[string]bool{}
	for i, raw := range list.Contributes {
		c, err := readContribution(raw)
		if err == nil && seen[c.ID] {
			err = fmt.Errorf("id %q repeats another entry's", c.ID)
		}
		if err != nil {
			problems = append(problems, fmt.Errorf("plugin %q contributes[%d]: %w, skipped", m.ID, i, err))
			continue
		}
		seen[c.ID] = true
		m.Contributes = append(m.Contributes, c)
	}
	m.readSystemAccess()
	return problems
}

func readContribution(raw json.RawMessage) (Contribution, error) {
	var r rawContribution
	if err := json.Unmarshal(raw, &r); err != nil {
		return Contribution{}, err
	}
	if !nameWord(r.ID, false) {
		return Contribution{}, fmt.Errorf("id %q must be lowercase letters, digits and hyphens", r.ID)
	}
	if _, ok := SlotOwner(r.Slot); !ok {
		return Contribution{}, fmt.Errorf("slot %q must be <owner>.<name>", r.Slot)
	}
	if !localPath(r.Path) {
		return Contribution{}, fmt.Errorf("path %q must be a clean local path", r.Path)
	}
	pos, err := ParsePosition(r.Position)
	if err != nil {
		return Contribution{}, err
	}
	order := DefaultOrder
	if r.Order != nil {
		order = *r.Order
	}
	return Contribution{ID: r.ID, Slot: r.Slot, Path: r.Path, Position: pos, Order: order}, nil
}

// readSystemAccess reads a manifest's system_access as the contribution to
// Access's blocks it is, standing where plugin parts of Access stand: before
// the web interface's section. A manifest that also contributes to Access
// directly is read from contributes alone.
func (m *Manifest) readSystemAccess() {
	if m.SystemAccess == "" || !localPath(m.SystemAccess) {
		return
	}
	for _, c := range m.Contributes {
		if c.Slot == AccessBlocks {
			return
		}
	}
	m.Contributes = append(m.Contributes, Contribution{
		ID: "access", Slot: AccessBlocks, Path: m.SystemAccess,
		Position: Position{Where: Before, Anchor: "web"}, Order: DefaultOrder,
	})
}
