// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.
package server

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/we-are-mono/verso/internal/plugin"
)

// writableConfigs is every uci config the plugin declares it writes.
func writableConfigs(m plugin.Manifest) []string {
	var out []string
	for _, a := range m.ACL.Write {
		if a.Scope == "uci" && a.Function == "write" && !slices.Contains(out, a.Object) {
			out = append(out, a.Object)
		}
	}
	return out
}

// Commands have a closed vocabulary and structured arguments. They run only
// after validation on a CSRF-protected POST, with the operator's own session.
func (s *Server) runPluginCommands(ctx context.Context, m plugin.Manifest, sid string, commands []plugin.ApplyAction) error {
	if len(commands) != 1 {
		return fmt.Errorf("expected one command")
	}
	cmd := commands[0]
	var err error // a helper refusal whose message the operator reads
	if owner, name, registered := strings.Cut(cmd.Name, "."); registered {
		err = s.registeredAct(ctx, m, sid, owner, name, cmd.Args)
	} else {
		err = s.coreAct(ctx, m, sid, cmd)
	}
	var validation interface{ ValidationMessage() string }
	if errors.As(err, &validation) && validation.ValidationMessage() != "" {
		return commandValidationError(validation.ValidationMessage())
	}
	return err
}

// registeredAct runs a plugin's registered act, `<plugin>.<name>` (ADR-027),
// only when the posting plugin declares it in acl.write. The helper checks the
// session's grant and the arguments against the registry; the shell adds no
// check of its own that could disagree with it.
func (s *Server) registeredAct(ctx context.Context, m plugin.Manifest, sid, pluginID, name string, args map[string]string) error {
	if !slices.Contains(m.ACL.Write, plugin.ACLScope{Scope: "verso-plugin", Object: pluginID, Function: name}) {
		return fmt.Errorf("act %s.%s lacks declared scope", pluginID, name)
	}
	_, err := s.backend.PluginCommand(ctx, sid, pluginID, name, args)
	return err
}

// coreAct runs one of the shell's own acts.
func (s *Server) coreAct(ctx context.Context, m plugin.Manifest, sid string, cmd plugin.ApplyAction) error {
	var err error
	switch cmd.Name {
	case "config-file-stage":
		// A file belongs to the family of the daemon that reads it, and is
		// staged only by a plugin that may write that family's config: the
		// helper, which holds the family table, holds the file to the configs
		// the plugin may write.
		allowed := slices.Contains(m.ACL.Write, plugin.ACLScope{Scope: "ubus", Object: "verso", Function: "stageConfigFile"})
		configs := writableConfigs(m)
		if !allowed || len(configs) == 0 || len(cmd.Args) != 3 {
			return fmt.Errorf("invalid file staging command")
		}
		err = s.backend.StageConfigFile(ctx, sid, cmd.Args["path"], cmd.Args["expected"], cmd.Args["content"], configs)
	case "ssh-key-add", "ssh-key-remove", "certificate-generate", "certificate-install":
		return s.credentialCommand(ctx, m, sid, cmd)
	case "set-system-time":
		// A plugin opts into the exact helper permission; a helper method
		// existing is not enough.
		if cmd.Args["datetime"] == "" || cmd.Args["timezone"] == "" {
			return fmt.Errorf("set-system-time needs datetime and timezone")
		}
		if !slices.Contains(m.ACL.Write, plugin.ACLScope{Scope: "ubus", Object: "verso", Function: "setSystemTime"}) {
			return fmt.Errorf("action %q lacks declared scope", cmd.Name)
		}
		return s.backend.SetSystemTime(ctx, sid, cmd.Args["datetime"], cmd.Args["timezone"])
	case "interface-restart", "interface-up", "interface-down":
		method := strings.TrimPrefix(cmd.Name, "interface-")
		allowed := slices.Contains(m.ACL.Write, plugin.ACLScope{Scope: "ubus", Object: "network.interface", Function: method})
		name := cmd.Args["interface"]
		if !allowed || len(cmd.Args) != 1 || name == "loopback" || len(name) > 63 || !plugin.UCIName(name, false) {
			return fmt.Errorf("invalid interface command")
		}
		if method != "restart" {
			return s.backend.NetworkSetUp(ctx, sid, name, method == "up")
		}
		return s.backend.NetworkRestart(ctx, sid, name)
	default:
		return fmt.Errorf("unknown command")
	}
	return err
}
