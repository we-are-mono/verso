// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"strings"
	"sync"

	"github.com/we-are-mono/verso/internal/plugin"
)

// stageAuthor is who staged a change: the plugin, and the page it was made on.
type stageAuthor struct {
	Plugin string // the plugin's manifest id
	Page   string // the page's name, untranslated
	Shell  bool   // Page is the shell's own word (Access), not the plugin's
}

// authorAt is the author of a change a plugin stages from one of its pages.
func authorAt(m plugin.Manifest, pluginPath string) stageAuthor {
	return stageAuthor{Plugin: m.ID, Page: pageLabel(m, pluginPath), Shell: accessPart(m, pluginPath)}
}

// pageLabel names the page a plugin path is: the plugin's part of the shell's
// Access page is Access; anything else is the label of the navigation entry it
// sits under (the longest that holds it), or the plugin's name where it has
// none.
func pageLabel(m plugin.Manifest, pluginPath string) string {
	if accessPart(m, pluginPath) {
		return "Access"
	}
	current := strings.Trim(pluginPath, "/")
	label, best := m.Name, -1
	for _, entry := range m.Nav {
		path := strings.Trim(entry.Path, "/")
		matches := path == "" || current == path || strings.HasPrefix(current, path+"/")
		if matches && len(path) > best {
			label, best = entry.Label, len(path)
		}
	}
	return label
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
}

// note records who staged the change at address in sid's stage.
func (a *stageAuthors) note(sid, address string, who stageAuthor) {
	a.mu.Lock()
	defer a.mu.Unlock()
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
