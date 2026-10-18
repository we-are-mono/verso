// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.
package openwrt

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

type ConfigFile struct {
	Path    string `json:"path"`
	Family  string `json:"family"`
	Content string `json:"content"`
	Version string `json:"version"`
	Pending bool   `json:"pending"`
}

// FileFamily is one family of hand-edited files as the helper's table holds it
// (ADR-027): its name, the uci config whose daemon reads it — a staged file
// waits under that config, and discarding the config discards the family's
// files with it — and the plugin that declared it.
type FileFamily struct {
	Name   string `json:"name"`
	Config string `json:"config"`
	Plugin string `json:"plugin"`
}

type configFileState struct {
	Files        []ConfigFile `json:"files"`
	Families     []FileFamily `json:"families"`
	Active       bool         `json:"active"`
	UCI          bool         `json:"uci"`
	UCIConfirmed bool         `json:"uci_confirmed"`
}

// config is the uci config that owns the family named name.
func (s configFileState) config(name string) (string, bool) {
	for _, f := range s.Families {
		if f.Name == name {
			return f.Config, true
		}
	}
	return "", false
}

func (b *NativeBackend) DNSState(ctx context.Context, sid string) (json.RawMessage, error) {
	var state json.RawMessage
	err := callHelper(ctx, b.helperSocket, "dnsState", sid, nil, &state)
	return state, err
}

// DDNSState is what ddns-scripts last did for each service: the address its
// name resolves to, when it last sent an update, and whether its updater runs.
func (b *NativeBackend) DDNSState(ctx context.Context, sid string) (json.RawMessage, error) {
	var state json.RawMessage
	err := callHelper(ctx, b.helperSocket, "ddnsState", sid, nil, &state)
	return state, err
}

// DDNSUpdate has the helper restart one ddns-scripts service's updater with no
// record of a last update, so it sends its address at once.
func (b *NativeBackend) DDNSUpdate(ctx context.Context, sid, section string) error {
	return callHelper(ctx, b.helperSocket, "ddnsUpdate", sid, map[string]string{"section": section}, nil)
}

// PluginCommand runs a plugin's registered root command through the helper
// (ADR-027), which checks the session's grant `verso-plugin <plugin> <name>`
// and the arguments against the registry before anything runs.
func (b *NativeBackend) PluginCommand(ctx context.Context, sid, plugin, name string, args map[string]string) (json.RawMessage, error) {
	request := pluginCommandRequest{
		Method: "pluginCommand",
		SID:    sid,
		Args:   pluginCommandArgs{Plugin: plugin, Name: name, Args: args},
	}
	var result json.RawMessage
	err := callHelperWith(ctx, b.helperSocket, "pluginCommand", request, &result, helperCallTimeout)
	return result, err
}

// RefreshGrants has the helper grant the session what rpcd's login would grant
// it from the access lists on disk now.
func (b *NativeBackend) RefreshGrants(ctx context.Context, sid string) error {
	return callHelper(ctx, b.helperSocket, "refreshGrants", sid, nil, nil)
}

// FirewallFiles is the rule files fw4 reads from its own folder, as they will
// read once staged changes are applied.
func (b *NativeBackend) FirewallFiles(ctx context.Context, sid string) (json.RawMessage, error) {
	var state json.RawMessage
	err := callHelper(ctx, b.helperSocket, "firewallFiles", sid, nil, &state)
	return state, err
}
func (b *NativeBackend) configFileState(ctx context.Context, sid string) (configFileState, error) {
	var state configFileState
	err := callHelper(ctx, b.helperSocket, "configFiles", sid, nil, &state)
	return state, err
}

// ConfigFiles is every family's files as they will read once staged changes
// apply, and the families, as the helper answers them: what a plugin that
// declares the files read is handed, narrowed to its own families.
func (b *NativeBackend) ConfigFiles(ctx context.Context, sid string) (json.RawMessage, error) {
	var state json.RawMessage
	err := callHelper(ctx, b.helperSocket, "configFiles", sid, nil, &state)
	return state, err
}
func (b *NativeBackend) fileAction(ctx context.Context, sid, action string, uci bool, timeout int) error {
	enabled := "0"
	if uci {
		enabled = "1"
	}
	return callHelper(ctx, b.helperSocket, "applyConfigFiles", sid, map[string]string{"action": action, "uci": enabled, "timeout": fmt.Sprint(timeout)}, nil)
}

// discardFiles drops the staged files of the families config owns, leaving the
// others staged.
func (b *NativeBackend) discardFiles(ctx context.Context, sid, config string) error {
	return callHelper(ctx, b.helperSocket, "applyConfigFiles", sid, map[string]string{"action": "discard", "config": config, "uci": "0", "timeout": "30"}, nil)
}

// StageConfigFile stages a hand-edited file through the helper, which refuses
// it unless its family's config is one of configs: the configs the plugin
// asking may write.
func (b *NativeBackend) StageConfigFile(ctx context.Context, sid, path, expected, content string, configs []string) error {
	var directories []string
	// dnsmasq's jail only mounts its configured include directory. Stage that
	// setting with the first include file so it also works on bare metal.
	if strings.HasPrefix(path, "/etc/dnsmasq.d/") {
		cfg, err := b.UCIConfig(ctx, sid, "dhcp")
		if err != nil {
			return err
		}
		found := false
		for section, raw := range cfg {
			values, ok := raw.(map[string]any)
			if !ok || values[".type"] != "dnsmasq" {
				continue
			}
			found = true
			dir, _ := values["confdir"].(string)
			if dir != "" && dir != "/etc/dnsmasq.d" {
				return fmt.Errorf("the resolver uses a different custom options directory")
			}
			if dir == "" {
				directories = append(directories, section)
			}
		}
		if !found {
			return fmt.Errorf("local resolver configuration unavailable")
		}
	}
	// Validate and stage the file before changing UCI. A stale editor, invalid
	// filename, or dnsmasq syntax error must not leave a confdir change behind.
	args := map[string]string{"path": path, "expected": expected, "content": content, "configs": strings.Join(configs, ",")}
	if err := callHelper(ctx, b.helperSocket, "stageConfigFile", sid, args, nil); err != nil {
		return err
	}
	for _, section := range directories {
		if err := b.UCISet(ctx, sid, "dhcp", section, map[string]any{"confdir": "/etc/dnsmasq.d"}); err != nil {
			return err
		}
	}
	return nil
}
func (b *NativeBackend) applyWithFiles(ctx context.Context, sid string, timeout int) error {
	files, err := b.configFileState(ctx, sid)
	if err != nil {
		return err
	}
	pending := false
	for _, f := range files.Files {
		pending = pending || f.Pending
	}
	if !pending {
		return b.uciApply(sid, timeout)
	}
	changes, err := b.uciChanges(sid)
	if err != nil {
		return err
	}
	uci := false
	for _, list := range changes {
		uci = uci || len(list) > 0
	}
	if err := b.fileAction(ctx, sid, "apply", uci, timeout); err != nil {
		return err
	}
	if uci {
		if err := b.uciApply(sid, timeout); err != nil {
			// A cancelled HTTP request must not cancel restoration of files already written.
			restoreCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), helperCallTimeout)
			defer cancel()
			if restoreErr := b.fileAction(restoreCtx, sid, "abort", false, timeout); restoreErr != nil {
				return fmt.Errorf("uci apply: %w; file rollback: %v", err, restoreErr)
			}
			return err
		}
	}
	return nil
}
func (b *NativeBackend) confirmWithFiles(ctx context.Context, sid string) error {
	files, err := b.configFileState(ctx, sid)
	if err != nil {
		return err
	}
	if !files.Active {
		return b.uciConfirm(sid)
	}
	if files.UCI && !files.UCIConfirmed {
		if err := b.uciConfirm(sid); err != nil {
			return err
		}
		if err := b.fileAction(ctx, sid, "uci-confirmed", true, 30); err != nil {
			return err
		}
	}
	return b.fileAction(ctx, sid, "confirm", files.UCI, 30)
}
