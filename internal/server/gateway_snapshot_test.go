// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"testing"

	"github.com/we-are-mono/verso/internal/openwrt"
)

// TestAConfigTheRouterLacksIsReadAsEmptyAndQuietly: a plugin may read a config
// no installed package ships (wireless on a wired router). Its snapshot has no
// such config, as on any failed read, but the shell logs nothing: an absent
// config is a fact about the router, read on every page and every review, and
// saying it as an error every time buried the errors that matter. A real
// failure is still said.
func TestAConfigTheRouterLacksIsReadAsEmptyAndQuietly(t *testing.T) {
	var logged bytes.Buffer
	defer log.SetOutput(log.Writer())
	log.SetOutput(&logged)
	for name, c := range map[string]struct {
		err   error
		quiet bool
	}{
		"absent":  {fmt.Errorf("openwrt: read network: %w", openwrt.ErrConfigNotFound), true},
		"refused": {errors.New("rpcd refused"), false},
	} {
		logged.Reset()
		s := newServer(t, fakeBackend{access: true, uciReadErr: c.err})
		snapshot := s.readSnapshot(context.Background(), demoReadManifest(), "s1")
		if _, ok := snapshot["network"]; ok {
			t.Errorf("%s: a read that did not answer put a config in the snapshot", name)
		}
		if said := strings.Contains(logged.String(), "read of uci"); said == c.quiet {
			t.Errorf("%s: logged %v, want %v:\n%s", name, said, !c.quiet, logged.String())
		}
	}
}
