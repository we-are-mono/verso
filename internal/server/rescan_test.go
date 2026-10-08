// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"testing"

	"github.com/we-are-mono/verso/internal/plugin"
)

// TestRescanServesAPluginInstalledBesideTheShell: a plugin package installed or
// removed outside the shell (apk over SSH) is served, or no longer served, once
// the shell is asked to rescan — what its package scripts ask with SIGHUP.
func TestRescanServesAPluginInstalledBesideTheShell(t *testing.T) {
	s := newServerWith(t, fakeBackend{}, &fakeTransport{}, nil)
	var onDisk []plugin.Manifest
	s.SetRescan(func() []plugin.Manifest { return onDisk })
	m := demoManifest()

	onDisk = []plugin.Manifest{m}
	if _, ok := s.manifestByID(m.ID); ok {
		t.Fatal("a plugin installed beside the shell was served before any rescan")
	}
	s.Rescan()
	if _, ok := s.manifestByID(m.ID); !ok {
		t.Fatal("a rescan did not serve the plugin installed beside the shell")
	}

	onDisk = nil
	s.Rescan()
	if _, ok := s.manifestByID(m.ID); ok {
		t.Error("a rescan kept serving a plugin removed beside the shell")
	}
}
