// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strings"
)

// uciRollbackTimeout is the window rpcd holds an applied configuration before
// reverting it on the device, unless the browser confirms (ADR-010).
const uciRollbackTimeout = 30

// capsuleView is what the staged-changes capsule renders (ADR-010): pending uci
// changes across the configs plugins declare, read from UCI's own stage — the
// browser holds nothing.
type capsuleView struct {
	Count int
	Label string   // "3 staged changes"
	Items []string // one plain line per change tuple
}

// declaredConfigsUnion is the set of uci configs any installed plugin declares —
// the surface the capsule reports and discards. Changes staged outside it (a
// concurrent uci shell on an undeclared config) are not the shell's to manage.
func (s *Server) declaredConfigsUnion() map[string]bool {
	union := make(map[string]bool)
	for _, m := range s.manifestList() {
		for cfg := range declaredUCIConfigs(m) {
			union[cfg] = true
		}
	}
	return union
}

// capsule reads the pending changes for the session. A read failure logs and
// yields an empty capsule — the page renders; the pill simply doesn't.
func (s *Server) capsule(ctx context.Context, sid string) capsuleView {
	if sid == "" {
		return capsuleView{}
	}
	changes, err := s.backend.UCIChanges(ctx, sid)
	if err != nil {
		log.Printf("verso: uci changes unavailable: %v", err)
		return capsuleView{}
	}
	declared := s.declaredConfigsUnion()

	var v capsuleView
	configs := make([]string, 0, len(changes))
	for cfg := range changes {
		if declared[cfg] {
			configs = append(configs, cfg)
		}
	}
	sort.Strings(configs)
	for _, cfg := range configs {
		for _, ch := range changes[cfg] {
			v.Items = append(v.Items, humanizeChange(cfg, ch))
		}
	}
	v.Count = len(v.Items)
	switch v.Count {
	case 0:
		v.Label = "No pending changes"
	case 1:
		v.Label = "1 staged change"
	default:
		v.Label = fmt.Sprintf("%d staged changes", v.Count)
	}
	return v
}

// humanizeChange renders one uci change tuple ([op, section, option?, value?])
// as a plain line. Mechanical on purpose (ADR-010): config, section, option,
// value — no per-plugin interpretation.
func humanizeChange(config string, ch []string) string {
	if len(ch) < 2 {
		return config + ": " + strings.Join(ch, " ")
	}
	op, section := ch[0], ch[1]
	switch {
	case op == "set" && len(ch) == 4:
		return fmt.Sprintf("%s: %s.%s = %s", config, section, ch[2], ch[3])
	case op == "set" && len(ch) == 3:
		return fmt.Sprintf("%s: new %s section %s", config, ch[2], section)
	case op == "add" && len(ch) == 3:
		return fmt.Sprintf("%s: new %s section %s", config, ch[2], section)
	case op == "remove" && len(ch) == 3:
		return fmt.Sprintf("%s: remove %s.%s", config, section, ch[2])
	case op == "remove":
		return fmt.Sprintf("%s: remove %s", config, section)
	case op == "list-add" && len(ch) == 4:
		return fmt.Sprintf("%s: %s.%s += %s", config, section, ch[2], ch[3])
	case op == "list-del" && len(ch) == 4:
		return fmt.Sprintf("%s: %s.%s -= %s", config, section, ch[2], ch[3])
	default:
		return fmt.Sprintf("%s: %s %s", config, op, strings.Join(ch[1:], " "))
	}
}

// handleUCIApply commits every dirty config through rpcd's `uci apply` with the
// device-side rollback armed (ADR-010). The capsule then confirms from the
// browser; no confirm within the window and the router reverts itself.
func (s *Server) handleUCIApply(w http.ResponseWriter, r *http.Request) {
	if err := s.backend.UCIApply(r.Context(), s.sessionSID(r), uciRollbackTimeout); err != nil {
		log.Printf("verso: uci apply failed: %v", err)
		http.Error(w, "apply failed", http.StatusBadGateway)
		return
	}
	writeOK(w)
}

// handleUCIConfirm disarms the pending rollback, keeping the applied
// configuration. The capsule polls this inside the rollback window; an error
// here means "not confirmed yet" to the poller, which retries.
func (s *Server) handleUCIConfirm(w http.ResponseWriter, r *http.Request) {
	if err := s.backend.UCIConfirm(r.Context(), s.sessionSID(r)); err != nil {
		log.Printf("verso: uci confirm failed: %v", err)
		http.Error(w, "confirm failed", http.StatusBadGateway)
		return
	}
	writeOK(w)
}

// handleUCIDiscard reverts every declared config with pending changes (ADR-010:
// the stage is one unit).
func (s *Server) handleUCIDiscard(w http.ResponseWriter, r *http.Request) {
	sid := s.sessionSID(r)
	changes, err := s.backend.UCIChanges(r.Context(), sid)
	if err != nil {
		log.Printf("verso: uci changes unavailable: %v", err)
		http.Error(w, "discard failed", http.StatusBadGateway)
		return
	}
	declared := s.declaredConfigsUnion()
	for cfg := range changes {
		if !declared[cfg] {
			continue
		}
		if err := s.backend.UCIRevert(r.Context(), sid, cfg); err != nil {
			log.Printf("verso: uci revert of %q failed: %v", cfg, err)
			http.Error(w, "discard failed", http.StatusBadGateway)
			return
		}
	}
	writeOK(w)
}

func writeOK(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"ok":true}`))
}
