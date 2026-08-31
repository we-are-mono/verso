// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"context"
	"fmt"
	"log"
)

// Verso configures everything on the router, including itself: its own settings
// are one uci config like every other owner's (ADR-013). That buys the capsule,
// `uci changes`, `uci show verso`, hand edits over SSH, sysupgrade's config-keep
// and Verso's own backup with no new machinery — and lets a root script read a
// setting with one `uci get` while Verso is not running.
const (
	// versoConfig is that config: /etc/config/verso, shipped empty because every
	// option's default is what its absence means (ADR-013 §2).
	versoConfig = "verso"
	// updatesSectionName is the named typed section holding what this router does
	// about updates. Verso's sections are named after their type, so a consumer
	// addresses one as `verso.updates.<option>` rather than by position.
	updatesSectionName = "updates"
	// autocheckOption gates the unattended daily check (ADR-014 §2). The cron
	// script reads it directly; the Maintenance toggle writes it explicitly.
	autocheckOption = "autocheck"
	// optionOn is the only value that turns an option on. Anything else — an
	// explicit 0, a typo, an absent option — is off, so a bare config never
	// reaches the network on its own.
	optionOn = "1"
	// optionOff is what an owner's "no" is written as. The toggle never clears the
	// option: an absence would be re-seeded as on by the next package install, so
	// an explicit 0 is what makes a decision survive an upgrade (ADR-014 §2).
	optionOff = "0"
)

// shellConfigs is the set of uci configs the shell itself owns. Plugins declare
// theirs in their manifests; the shell declares this one here, so a staged Verso
// setting counts, reviews, and discards through the capsule exactly like a
// firewall rule (ADR-013 §3, ADR-010).
var shellConfigs = []string{versoConfig}

// versoOption reads one option of Verso's own config through the same brokered
// snapshot the shell hands plugins — never by opening the file. ADR-007's posture
// does not soften for the shell's own config, and reading through rpcd is also
// what makes a staged, not-yet-applied value the one the page shows.
//
// The second return reports whether the config could be read at all. A caller
// must not fold an unreadable config into "the option is absent": a switch drawn
// off for a setting that is actually on would state the opposite of what the
// router does, on the one control whose purpose is telling the truth about
// unattended network activity (ADR-014 §2).
func (s *Server) versoOption(ctx context.Context, sid, section, option string) (string, bool) {
	snapshot, err := s.backend.UCIConfig(ctx, sid, versoConfig)
	if err != nil {
		log.Printf("verso: settings: %s config unavailable: %v", versoConfig, err)
		return "", false
	}
	value, _ := asSection(snapshot[section])[option].(string)
	return value, true
}

// stageVersoOption stages one option of Verso's own config through rpcd, gated by
// the operator's sid like every other write (ADR-007). It creates the named typed
// section first when the config has none — a fresh install ships the file empty,
// so the first setting an owner changes is also the section's first appearance.
// Nothing is committed here: the capsule's Save & Apply is what makes it live
// (ADR-010).
func (s *Server) stageVersoOption(ctx context.Context, sid, section, option, value string) error {
	snapshot, err := s.backend.UCIConfig(ctx, sid, versoConfig)
	if err != nil {
		return fmt.Errorf("read %s config: %w", versoConfig, err)
	}
	if _, present := snapshot[section]; !present {
		if _, err := s.backend.UCIAdd(ctx, sid, versoConfig, section, section); err != nil {
			return fmt.Errorf("create %s.%s: %w", versoConfig, section, err)
		}
	}
	if err := s.backend.UCISet(ctx, sid, versoConfig, section, map[string]any{option: value}); err != nil {
		return fmt.Errorf("stage %s.%s.%s: %w", versoConfig, section, option, err)
	}
	return nil
}
