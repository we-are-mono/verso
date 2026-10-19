// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"context"
	"encoding/json"

	"github.com/we-are-mono/verso/internal/events"
	"github.com/we-are-mono/verso/internal/history"
	"github.com/we-are-mono/verso/internal/widget"
)

// Config history as one session may read it (ADR-007, ADR-021). history.db is
// the shell's own store, read with no rpcd call, so each config in it reaches
// a session only while rpcd grants that session the config's read: an op on
// any other config is left out, a row left with none goes, and an entry left
// with none is not there for the session, who applied it and from where with
// it. An act on an entry stages only what the session sees whole, and only
// into configs rpcd lets it write.

// uncheckedNotice says, on a listing kept to what the session may read, that
// rpcd did not answer for some of it: in place of the empty listing's words,
// or as a notice above the rows kept.
func uncheckedNotice(g *sessionGrants, table *widget.Table) []widget.Widget {
	switch {
	case !g.anyUnchecked():
		return nil
	case len(table.Rows) == 0:
		table.EmptyText = permissionsUnchecked
		return nil
	}
	return []widget.Widget{&widget.Callout{Variant: "warning", Compact: true, Body: permissionsUnchecked}}
}

// visibleChanges is the entries as the session may read them, each config
// asked of rpcd once. A row with no op of its own stands or goes with its
// entry.
func visibleChanges(ctx context.Context, g *sessionGrants, all []history.Change) []history.Change {
	var out []history.Change
	for _, c := range all {
		var items []history.ChangeItem
		readable := false
		for _, item := range c.Items {
			var ops []history.ChangeOp
			for _, op := range item.Ops {
				if g.mayReadConfig(ctx, op.Config) {
					ops = append(ops, op)
				}
			}
			if len(ops) == 0 && len(item.Ops) > 0 {
				continue
			}
			readable = readable || len(ops) > 0
			item.Ops = ops
			items = append(items, item)
		}
		if readable {
			c.Items = items
			out = append(out, c)
		}
	}
	return out
}

// readableWhole reports whether the session may read every config entries
// touch: what an act across them would stage is then what the session sees.
func readableWhole(ctx context.Context, g *sessionGrants, entries []history.Change) bool {
	for _, c := range entries {
		for _, item := range c.Items {
			for _, op := range item.Ops {
				if !g.mayReadConfig(ctx, op.Config) {
					return false
				}
			}
		}
	}
	return true
}

// readableConfigs is configs, by name, less those the session may not read;
// whole says none was left out.
func readableConfigs[T any](ctx context.Context, g *sessionGrants, configs map[string]T) (out map[string]T, whole bool) {
	out, whole = make(map[string]T, len(configs)), true
	for config, v := range configs {
		if g.mayReadConfig(ctx, config) {
			out[config] = v
		} else {
			whole = false
		}
	}
	return out, whole
}

// planReadable reports whether every config plan stages into is one the
// session may read.
func planReadable(ctx context.Context, g *sessionGrants, plan actPlan) bool {
	for _, step := range plan.Steps {
		if !g.mayReadConfig(ctx, step.op.Config) {
			return false
		}
	}
	return true
}

// mayStage reports whether rpcd lets the session write every config plan
// stages into. rpcd checks each write again as it is made; asking first
// refuses the act before any of it is staged.
func mayStage(ctx context.Context, g *sessionGrants, plan actPlan) bool {
	for _, step := range plan.Steps {
		if !g.may(ctx, configGrant(step.op.Config, "write")) {
			return false
		}
	}
	return true
}

// readsAllHistory answers, once asked and then from memory, whether the
// session may read every config config history names: in its entries, its
// starting point and its last readings. A store that cannot say answers no.
func (s *Server) readsAllHistory(ctx context.Context, g *sessionGrants, entries []history.Change) func() bool {
	var answered, all bool
	return func() bool {
		if answered {
			return all
		}
		answered = true
		if !readableWhole(ctx, g, entries) {
			return false
		}
		start, err := s.historyStore.StartingPoint(ctx)
		if err != nil {
			return false
		}
		states, err := s.historyStore.ConfigStates(ctx)
		if err != nil {
			return false
		}
		_, startWhole := readableConfigs(ctx, g, start.Configs)
		_, statesWhole := readableConfigs(ctx, g, states)
		all = startWhole && statesWhole
		return all
	}
}

// The grants the router's own kinds of event name (internal/events): the read
// the state they record comes from, the internet's outages interfaceGrant,
// the router starting systemGrant, and the web certificate's replacement
// accessCredentialsGrant. The shell's history storage is its own and needs
// none.
var (
	interfaceGrant         = aclProbe{"ubus", "network.interface", "dump"}
	systemGrant            = aclProbe{"ubus", "system", "info"}
	accessCredentialsGrant = aclProbe{"ubus", "verso", "accessCredentials"}
	coreEventGrants        = map[events.Grant]*aclProbe{
		events.InterfaceGrant:         &interfaceGrant,
		events.SystemGrant:            &systemGrant,
		events.AccessCredentialsGrant: &accessCredentialsGrant,
	}
)

// pluginReadGrant is the grant of a plugin's registered read (ADR-027), which
// its events show under (ADR-028). A session holds it through the plugin's
// own ACL group, root's among them, never through one Verso ships, so it is
// asked of whatever read a declaration names and is not one of shellProbes.
func pluginReadGrant(pluginID, read string) aclProbe { return aclProbe{"verso-plugin", pluginID, read} }

// kindProbe is the probe a session passes to see an event of k, nil when every
// session may; known is false for a grant the shell does not ask, which no
// session passes.
func kindProbe(k events.Kind) (probe *aclProbe, known bool) {
	switch {
	case k.Grant == nil:
		return nil, true
	case k.Plugin != "" && k.Grant.Scope == "verso-plugin" && k.Grant.Object == k.Plugin:
		p := pluginReadGrant(k.Plugin, k.Grant.Function)
		return &p, true
	}
	probe, known = coreEventGrants[*k.Grant]
	return probe, known
}

// grantedEvents is the events of a known kind, apart from an apply's, whose
// grant rpcd gives the session, in the order given: what a chart may mark.
func grantedEvents(ctx context.Context, g *sessionGrants, kinds *events.Table, recorded []history.Event) []history.Event {
	var out []history.Event
	for _, e := range recorded {
		k := kinds.Of(e)
		if k.Unknown || k.ByChange {
			continue
		}
		if probe, known := kindProbe(k); known && (probe == nil || g.may(ctx, *probe)) {
			out = append(out, e)
		}
	}
	return out
}

// eventsView is what the events page may show a session: each event under
// its kind's grant, and an apply's under its entry in config history.
type eventsView struct {
	g       *sessionGrants
	kept    map[int64]bool           // every entry config history keeps
	visible map[int64]history.Change // the entries the session may read, as it may
	partial map[int64]bool           // the visible entries it may read only in part
	// whole reports whether the session may read every config history
	// names: the bar for an event nothing else places.
	whole func() bool
}

// newEventsView is the events as the session may see them, beside config
// history's entries.
func newEventsView(ctx context.Context, g *sessionGrants, entries []history.Change, whole func() bool) eventsView {
	v := eventsView{g: g, kept: map[int64]bool{}, visible: map[int64]history.Change{}, partial: map[int64]bool{}, whole: whole}
	for _, c := range entries {
		v.kept[c.ID] = true
		v.partial[c.ID] = !readableWhole(ctx, g, []history.Change{c})
	}
	for _, c := range visibleChanges(ctx, g, entries) {
		v.visible[c.ID] = c
	}
	return v
}

// show reports whether the session may see e, of kind k, and e as it may see
// it: an apply whose entry the session reads only in part names only its
// pages; an event of no known kind shows to a session that may read all of
// config history.
func (v eventsView) show(ctx context.Context, k events.Kind, e history.Event) (history.Event, bool) {
	if k.ByChange {
		var d changesDetail
		if json.Unmarshal([]byte(e.Detail), &d) != nil || !v.kept[d.Change] {
			return e, v.whole()
		}
		c, ok := v.visible[d.Change]
		if !ok {
			return e, false
		}
		if v.partial[d.Change] {
			d.Pages = changePages(c)
			detail, _ := json.Marshal(d)
			e.Detail = string(detail)
		}
		return e, true
	}
	if k.Unknown {
		return e, v.whole()
	}
	probe, known := kindProbe(k)
	switch {
	case !known:
		return e, false
	case probe == nil:
		return e, true
	}
	return e, v.g.may(ctx, *probe)
}
