// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.
package server

import (
	"context"
	"errors"
	"fmt"
	"github.com/we-are-mono/verso/internal/plugin"
	"strings"
)

// Commands have a closed vocabulary and structured arguments. They run only
// after validation on a CSRF-protected POST, with the operator's own session.
func (s *Server) runPluginCommands(ctx context.Context, m plugin.Manifest, sid string, commands []plugin.ApplyAction) error {
	if len(commands) != 1 {
		return fmt.Errorf("expected one command")
	}
	cmd := commands[0]
	switch cmd.Name {
	case "config-file-stage":
		configAllowed := false
		for _, a := range m.ACL.Write {
			if a.Scope == "uci" && a.Object == "dhcp" && a.Function == "write" {
				configAllowed = true
			}
		}
		allowed := false
		for _, a := range m.ACL.Write {
			if a.Scope == "ubus" && a.Object == "verso" && a.Function == "stageConfigFile" {
				allowed = true
			}
		}
		if !allowed || !configAllowed || len(cmd.Args) != 3 {
			return fmt.Errorf("invalid file staging command")
		}
		backend, ok := s.backend.(interface {
			StageConfigFile(context.Context, string, string, string, string) error
		})
		if !ok {
			return fmt.Errorf("file staging unavailable")
		}
		err := backend.StageConfigFile(ctx, sid, cmd.Args["path"], cmd.Args["expected"], cmd.Args["content"])
		var validation interface{ ValidationMessage() string }
		if errors.As(err, &validation) && validation.ValidationMessage() != "" {
			return commandValidationError(validation.ValidationMessage())
		}
		return err
	case "ssh-key-add", "ssh-key-remove", "certificate-generate", "certificate-install":
		return s.credentialCommand(ctx, m, sid, cmd)
	case "set-system-time":
		if err := validateApplyActions(m, commands); err != nil {
			return err
		}
		return s.backend.SetSystemTime(ctx, sid, cmd.Args["datetime"], cmd.Args["timezone"])
	case "interface-restart", "interface-up", "interface-down":
		method := strings.TrimPrefix(cmd.Name, "interface-")
		allowed := false
		for _, a := range m.ACL.Write {
			if a.Scope == "ubus" && a.Object == "network.interface" && a.Function == method {
				allowed = true
			}
		}
		name := cmd.Args["interface"]
		if !allowed || len(cmd.Args) != 1 || name == "" || name == "loopback" || len(name) > 63 || strings.IndexFunc(name, func(r rune) bool {
			return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_')
		}) >= 0 {
			return fmt.Errorf("invalid interface command")
		}
		if method != "restart" {
			backend, ok := s.backend.(interface {
				NetworkSetUp(context.Context, string, string, bool) error
			})
			if !ok {
				return fmt.Errorf("network control unavailable")
			}
			return backend.NetworkSetUp(ctx, sid, name, method == "up")
		}
		backend, ok := s.backend.(interface {
			NetworkRestart(context.Context, string, string) error
		})
		if !ok {
			return fmt.Errorf("network control unavailable")
		}
		return backend.NetworkRestart(ctx, sid, name)
	default:
		return fmt.Errorf("unknown command")
	}
}
