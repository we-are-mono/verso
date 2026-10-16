// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"strings"
	"sync"
	"time"

	"github.com/we-are-mono/verso/internal/plugin"
)

// stageAuthor is who staged a change: the plugin, the page it was made on and
// where that page is, and when.
type stageAuthor struct {
	Plugin string // the plugin's manifest id
	Page   string // the page's name, untranslated
	Shell  bool   // Page is the shell's own word (Access), not the plugin's
	// Href is the page's address, where the Pending changes page leads back
	// to; empty for a change no page is on record for.
	Href string
	// Heading is the page's own title as its plugin wrote it ("Firewall
	// rules"), what the Pending changes page names its card by; empty where
	// the save did not render the page.
	Heading string
	// At is when the change was staged; zero where it is not known.
	At time.Time
}

// authorAt is the author of a change a plugin stages from one of its pages.
func authorAt(m plugin.Manifest, pluginPath string) stageAuthor {
	who := stageAuthor{Plugin: m.ID, Page: pageLabel(m, pluginPath), Shell: accessPart(m, pluginPath)}
	switch {
	case who.Shell:
		who.Href = "/system/access"
	case m.ID != "":
		who.Href = pageHref(m, pluginPath)
	}
	return who
}

// pageHref is the address of the page a plugin path is on: the deepest of
// its nav entry's subpages that holds it, else the entry's own page, else the
// plugin's root. A drawer's path ("zones/lan") is on the page its listing is.
func pageHref(m plugin.Manifest, pluginPath string) string {
	entry, ok := navEntryAt(m, pluginPath)
	if !ok {
		return pluginHref(m.ID, "/")
	}
	current := strings.Trim(pluginPath, "/")
	best, href := -1, pluginHref(m.ID, entry.Path)
	for _, page := range entry.Pages.List {
		path := strings.Trim(page.Path, "/")
		holds := path == "" || current == path || strings.HasPrefix(current, path+"/")
		if holds && len(path) > best {
			best, href = len(path), pluginHref(m.ID, page.Path)
		}
	}
	return href
}

// pageLabel names the page a plugin path is: the plugin's part of the shell's
// Access page is Access; anything else is the label of the navigation entry it
// sits under (the longest that holds it), or the plugin's name where it has
// none.
func pageLabel(m plugin.Manifest, pluginPath string) string {
	if accessPart(m, pluginPath) {
		return "Access"
	}
	if entry, ok := navEntryAt(m, pluginPath); ok {
		return entry.Label
	}
	return m.Name
}

// accessPart reports whether a plugin path is the plugin's part of Access.
func accessPart(m plugin.Manifest, pluginPath string) bool {
	access := strings.Trim(m.SystemAccess, "/")
	current := strings.Trim(pluginPath, "/")
	return access != "" && (current == access || strings.HasPrefix(current, access+"/"))
}

// stageAuthors remembers, for each session's stage, who staged each change:
// the plugin and the page it was made on. UCI keeps what changed but not who
// changed it, and a config is often shared — network is written by
// Interfaces, by DNS & DHCP and by General — so the first plugin to declare a
// config is not the page a change came from. The review drawer files a change
// under its page, and asks its plugin to describe it. A change with no author
// on record (one staged before the shell last started) falls back to the
// config's first declarer. The record is the stage's own: it is forgotten when
// the stage is applied or discarded.
type stageAuthors struct {
	mu sync.Mutex
	by map[string]map[string]stageAuthor // sid → change address → author
	// now is the clock a record is stamped by; nil is the wall clock.
	now func() time.Time
}

// note records who staged the change at address in sid's stage, and when: now,
// unless the record carries its own time (a change staged again as it was).
func (a *stageAuthors) note(sid, address string, who stageAuthor) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if who.At.IsZero() {
		who.At = time.Now()
		if a.now != nil {
			who.At = a.now()
		}
	}
	if a.by == nil {
		a.by = map[string]map[string]stageAuthor{}
	}
	if a.by[sid] == nil {
		a.by[sid] = map[string]stageAuthor{}
	}
	a.by[sid][address] = who
}

// of names who staged the change at address, if it is recorded.
func (a *stageAuthors) of(sid, address string) (stageAuthor, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	who, ok := a.by[sid][address]
	return who, ok
}

// forget drops sid's record: its stage has been applied or discarded.
func (a *stageAuthors) forget(sid string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.by, sid)
}

// optionAddress and sectionAddress name what a change touches, the keys the
// record is kept by.
func optionAddress(config, section, option string) string {
	return config + "." + section + "." + option
}

func sectionAddress(config, section string) string { return config + "." + section }
