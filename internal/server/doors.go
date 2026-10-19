// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"cmp"
	"fmt"
	"log"
	"net/url"
	"slices"
	"strings"

	"github.com/we-are-mono/verso/internal/plugin"
)

// Doors: where the shell links to a kind of thing a plugin keeps — a network,
// a tunnel, a firewall zone, rule or port forward. The Overview's cards, the
// exposure notice and the firewall log all point at such things, and which
// plugin opens them is the plugin's to say: it claims the kind's door in its
// manifest (entity_acts, slot "open"), and the shell links through the claim
// in the claiming plugin's own words. With no live claim there is no link.

// doorKinds is the closed vocabulary of kinds a plugin may claim a door to,
// named as a change's subjects are (plugin.SubjectKinds), tunnel beside them.
// A door kind is never a panel kind: entitySlots holds those, and only they
// are entity routes.
var doorKinds = []string{"network", "tunnel", "zone", "rule", "port-forward"}

// doorSlot is the one slot a door kind has.
const doorSlot = "open"

// entityDoor is one plugin's claim on a kind's door, resolved against the
// plugin's pages.
type entityDoor struct {
	pluginID string
	manifest plugin.Manifest
	path     string          // the claim's path, {id} where the subject goes
	entry    plugin.NavEntry // the nav entry holding the claim's page
}

// door is the live claim on a kind's door whose plugin id sorts first, so
// every page agrees on one. A claim whose page sits under none of its
// plugin's nav entries is no door: it has no page to lead to or name.
func (s *Server) door(kind string) (entityDoor, bool) {
	for _, m := range s.manifestsByID() {
		for _, a := range m.EntityActs {
			if a.Entity != kind || a.Slot != doorSlot {
				continue
			}
			entry, ok := navEntryAt(m, claimPage(a.Path))
			if !ok || !s.probe(m.Endpoint()) {
				break
			}
			return entityDoor{pluginID: m.ID, manifest: m, path: a.Path, entry: entry}, true
		}
	}
	return entityDoor{}, false
}

// manifestsByID is the served manifests in plugin id order, the order every
// claim lookup reads them in.
func (s *Server) manifestsByID() []plugin.Manifest {
	manifests := slices.Clone(s.manifestList())
	slices.SortStableFunc(manifests, func(a, b plugin.Manifest) int { return cmp.Compare(a.ID, b.ID) })
	return manifests
}

// claimPage is a claim's page: its path without the query.
func claimPage(path string) string {
	page, _, _ := strings.Cut(path, "?")
	return page
}

// To is the address of one subject: the claim's path under the plugin's mount
// with the subject's identity in {id}'s place, escaped for where it stands.
func (d entityDoor) To(id string) string {
	return pluginHref(d.pluginID, fillID(d.path, id))
}

// fillID puts id where a path's {id} stands, path-escaped before the query
// and query-escaped in it.
func fillID(path, id string) string {
	page, query, hasQuery := strings.Cut(path, "?")
	page = strings.ReplaceAll(page, "{id}", url.PathEscape(id))
	if !hasQuery {
		return page
	}
	return page + "?" + strings.ReplaceAll(query, "{id}", url.QueryEscape(id))
}

// Listing is the page that lists the kind: the most specific of the plugin's
// pages, an entry or one of its subpages, holding the claim's page.
func (d entityDoor) Listing() string {
	return pageHref(d.manifest, claimPage(d.path))
}

// Word is the page's name as the claiming plugin says it, in its catalog's
// language.
func (d entityDoor) Word(pluginTr func(pluginID string) func(string) string) string {
	return pluginTr(d.pluginID)(d.entry.Label)
}

// withKnownClaims is manifests with each claim the shell cannot place dropped
// and logged: an act on a kind or slot it does not have, and a door claim with
// no {id} for the subject. A plugin's other claims, and the plugin, stand.
func withKnownClaims(manifests []plugin.Manifest) []plugin.Manifest {
	out := make([]plugin.Manifest, len(manifests))
	for i, m := range manifests {
		var kept []plugin.EntityAct
		for _, a := range m.EntityActs {
			if err := claimProblem(a); err != nil {
				log.Printf("verso: compatibility: plugin %s: entity act %s/%s ignored: %v", m.ID, a.Entity, a.Slot, err)
				continue
			}
			kept = append(kept, a)
		}
		m.EntityActs = kept
		out[i] = m
	}
	return out
}

// claimProblem is why the shell cannot place one entity act, or nil.
func claimProblem(a plugin.EntityAct) error {
	if slices.Contains(doorKinds, a.Entity) {
		switch {
		case a.Slot != doorSlot:
			return fmt.Errorf("a %s has only the slot %q", a.Entity, doorSlot)
		case !strings.Contains(a.Path, "{id}"):
			return fmt.Errorf("a door's path names no {id}")
		}
		return nil
	}
	slots, known := entitySlots[a.Entity]
	if !known {
		return fmt.Errorf("the shell has no kind %q", a.Entity)
	}
	if !slices.ContainsFunc(slots, func(slot entitySlot) bool { return slot.Name == a.Slot }) {
		return fmt.Errorf("a %s has no slot %q", a.Entity, a.Slot)
	}
	return nil
}
