// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

// Package openwrt reads live state from the OpenWrt backend through rpcd's
// ACL-gated ubus objects, carrying the caller's session id (ADR-007).
package openwrt

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/we-are-mono/verso/internal/ubus"
)

// ErrAccessDenied is returned when rpcd's ACLs refuse the session the operation.
var ErrAccessDenied = errors.New("openwrt: access denied by rpcd ACL")

// Backend reads live state from the OpenWrt system on behalf of a session. It is
// the injected seam (ADR-003): callers depend on this interface, tests supply a
// fake. Every method takes the rpcd session id (sid) so rpcd — not Verso —
// authorizes the operation (ADR-007).
type Backend interface {
	SystemInfo(ctx context.Context, sid string) (SystemInfo, error)
	Hostname(ctx context.Context, sid string) (string, error)
	// Access asks rpcd whether the session may call object.function within the
	// given ACL scope. It is the enforcement point the shell uses to gate plugin
	// writes on the operator's rpcd ACLs (ADR-007). A transport error is distinct
	// from a denial, so callers can fail closed on the former.
	Access(ctx context.Context, sid, scope, object, function string) (bool, error)
	// UCISet and UCICommit write config through rpcd's ACL-gated `uci` object,
	// carrying the operator's sid so rpcd — not Verso — authorizes the write. The
	// shell performs writes on a plugin's behalf (ADR-007), so a plugin
	// holds no write privilege and no session credential of its own.
	UCISet(ctx context.Context, sid, config, section string, values map[string]any) error
	UCICommit(ctx context.Context, sid, config string) error
	// UCIConfig reads a whole uci config through rpcd's ACL-gated `uci` object,
	// carrying the sid. It returns the `values` map — section name → section table
	// (its `.type`/`.name` meta and options) — so the shell can hand a plugin a read
	// snapshot of a config the plugin declared, without the plugin holding a session
	// or reading /etc/config (ADR-007). rpcd scopes the read to the operator, so an
	// unreadable config yields an empty snapshot rather than an error.
	UCIConfig(ctx context.Context, sid, config string) (map[string]any, error)
	// UCIAdd creates a new anonymous section of secType in config through rpcd's
	// `uci` object, carrying the sid, and returns the new section's id. It realizes
	// the "add" of a uci-backed repeater (ADR-005 §7): the shell — not the plugin —
	// performs the structural change, within the plugin's declared write scope.
	UCIAdd(ctx context.Context, sid, config, secType string) (string, error)
	// UCIDelete removes a section from config through rpcd, carrying the sid. It
	// realizes the "remove" of a uci-backed repeater (ADR-005 §7).
	UCIDelete(ctx context.Context, sid, config, section string) error
	// SetPassword sets username's system password through rpcd's ACL-gated `luci`
	// object (method setPassword), carrying the operator's sid. rpcd authorizes and
	// executes the change as root; the shell itself is unprivileged and cannot
	// write /etc/shadow (ADR-007). It backs the shell-owned password page (ADR-009
	// §3) — the shell owns the credential surface, but the privileged write, like
	// every other, goes through rpcd with the session.
	SetPassword(ctx context.Context, sid, username, password string) error
	// The rest of the uci two-phase lifecycle (ADR-010). Staged edits live in
	// UCI's own stage; these four let the shell read it, discard it, and apply it
	// with rpcd's native device-side rollback — all sid-gated like every write.
	//
	// UCIChanges returns the pending (staged, uncommitted) changes across all
	// configs the session may see: config name → change records, each record the
	// uci tuple [op, section, option?, value?].
	UCIChanges(ctx context.Context, sid string) (map[string][][]string, error)
	// UCIRevert discards a config's staged changes.
	UCIRevert(ctx context.Context, sid, config string) error
	// UCIApply commits every dirty config, lets procd's config triggers reload the
	// affected services, and arms rpcd's rollback: unless UCIConfirm lands within
	// timeout seconds, the device reverts itself.
	UCIApply(ctx context.Context, sid string, timeout int) error
	// UCIConfirm disarms a pending rollback, keeping the applied configuration.
	UCIConfirm(ctx context.Context, sid string) error
	// RCList reads procd's rc snapshot — per init script, whether it starts at
	// boot and whether procd reports it running — gated on the session's
	// ubus/rc/list access. The plugin-management surface (ADR-011) reads plugin
	// service state through it.
	RCList(ctx context.Context, sid string) (map[string]RCState, error)
	// RCInit drives one lifecycle action (start, stop, restart, enable,
	// disable) on a named init script through procd's rc object, gated on the
	// session's ubus/rc/init access. Callers constrain the name to services
	// they own; the backend adds no policy of its own beyond the session gate.
	RCInit(ctx context.Context, sid, name, action string) error
	// The package verbs (ADR-011 §4) ride the privileged helper's ubus object
	// ("verso"), which self-gates on the sid; the helper validates names and
	// queries again on its side. PkgStatus reports when the feed indexes were
	// last refreshed (unix seconds; 0 = never); PkgUpdate refreshes them;
	// PkgSearch lists matching packages plus the uncapped total; PkgInstall
	// and PkgRemove act on one exact package name.
	PkgStatus(ctx context.Context, sid string) (int64, error)
	PkgUpdate(ctx context.Context, sid string) error
	PkgSearch(ctx context.Context, sid, query string) ([]Package, int, error)
	// PkgInstalled lists every installed package (no descriptions).
	PkgInstalled(ctx context.Context, sid string) ([]Package, error)
	PkgInstall(ctx context.Context, sid, name string) error
	PkgRemove(ctx context.Context, sid, name string) error
	// WANStatus reads the uplink's live condition from netifd
	// (network.interface.wan status) — whether it is up and which l3 device
	// carries it — gated on the session's access to that object.
	WANStatus(ctx context.Context, sid string) (WANState, error)
	// DeviceStats reads one network device's link state and byte counters
	// (network.device status), sid-gated likewise. A throughput reading is the
	// delta between two of these.
	DeviceStats(ctx context.Context, sid, device string) (DeviceStats, error)
}

// WANState is the uplink's live condition, from network.interface.wan status.
type WANState struct {
	Up     bool
	Device string // the l3 device carrying the uplink, e.g. "wan0"
}

// DeviceStats is one network device's link state and byte counters, from
// network.device status.
type DeviceStats struct {
	Carrier   bool
	SpeedMbps int // negotiated link speed; 0 when the driver reports none
	RxBytes   int64
	TxBytes   int64
}

// Package is one row of a package search or listing, as the helper reports
// it; the detail fields are filled where the source provides them in bulk.
type Package struct {
	Name        string
	Version     string
	Feed        string
	Description string
	License     string
	Webpage     string
	Size        int64 // package file size, bytes
	Installed   bool
}

// RCState is one procd service's rc snapshot: enabled is the boot symlink,
// running is procd's live view. procd omits `running` for scripts it has never
// managed; that decodes as false, which is the honest reading.
type RCState struct {
	Enabled bool
	Running bool
}

// SystemInfo is the subset of `ubus call system info` that Verso renders.
type SystemInfo struct {
	Uptime int64
	Load   [3]int64
	Memory Memory
}

// Memory holds byte counts reported by system info.
type Memory struct {
	Total     int64
	Free      int64
	Available int64
}

// The side-effecting operations sit behind func seams so the backend is
// unit-testable with fakes; the real implementations dial the ubus socket and
// go through rpcd (verified live, like the ubus client itself).
type (
	hostnameFn     func(ctx context.Context, sid string) (string, error)
	systemInfoFn   func(ctx context.Context, sid string) (map[string]any, error)
	accessFn       func(ctx context.Context, sid, scope, object, function string) (bool, error)
	uciSetFn       func(ctx context.Context, sid, config, section string, values map[string]any) error
	uciCommitFn    func(ctx context.Context, sid, config string) error
	uciConfigFn    func(ctx context.Context, sid, config string) (map[string]any, error)
	uciAddFn       func(ctx context.Context, sid, config, secType string) (string, error)
	uciDeleteFn    func(ctx context.Context, sid, config, section string) error
	passwdFn       func(ctx context.Context, sid, username, password string) error
	uciChangesFn   func(ctx context.Context, sid string) (map[string][][]string, error)
	uciRevertFn    func(ctx context.Context, sid, config string) error
	uciApplyFn     func(ctx context.Context, sid string, timeout int) error
	uciConfirmFn   func(ctx context.Context, sid string) error
	rcListFn       func(ctx context.Context, sid string) (map[string]RCState, error)
	rcInitFn       func(ctx context.Context, sid, name, action string) error
	pkgStatusFn    func(ctx context.Context, sid string) (int64, error)
	pkgUpdateFn    func(ctx context.Context, sid string) error
	pkgSearchFn    func(ctx context.Context, sid, query string) ([]Package, int, error)
	pkgInstalledFn func(ctx context.Context, sid string) ([]Package, error)
	pkgActFn       func(ctx context.Context, sid, name string) error
	wanStatusFn    func(ctx context.Context, sid string) (WANState, error)
	deviceStatsFn  func(ctx context.Context, sid, device string) (DeviceStats, error)
)

// NativeBackend reads OpenWrt state over the ubus socket, presenting the session
// sid to rpcd's ACL-gated objects. Verso holds no ambient privilege over config:
// rpcd authorizes and executes, so a restricted operator is limited to what
// their ACLs grant (ADR-007).
type NativeBackend struct {
	hostname     hostnameFn
	systemInfo   systemInfoFn
	access       accessFn
	uciSet       uciSetFn
	uciCommit    uciCommitFn
	uciConfig    uciConfigFn
	uciAdd       uciAddFn
	uciDelete    uciDeleteFn
	setPassword  passwdFn
	uciChanges   uciChangesFn
	uciRevert    uciRevertFn
	uciApply     uciApplyFn
	uciConfirm   uciConfirmFn
	rcList       rcListFn
	rcInit       rcInitFn
	pkgStatus    pkgStatusFn
	pkgUpdate    pkgUpdateFn
	pkgSearch    pkgSearchFn
	pkgInstalled pkgInstalledFn
	pkgInstall   pkgActFn
	pkgRemove    pkgActFn
	wanStatus    wanStatusFn
	deviceStats  deviceStatsFn
}

// NewNativeBackend returns a backend using the default ubus socket.
func NewNativeBackend() *NativeBackend {
	return &NativeBackend{
		hostname:     dialHostname(""),
		systemInfo:   dialSystemInfo(""),
		access:       dialAccess(""),
		uciSet:       dialUCISet(""),
		uciCommit:    dialUCICommit(""),
		uciConfig:    dialUCIConfig(""),
		uciAdd:       dialUCIAdd(""),
		uciDelete:    dialUCIDelete(""),
		setPassword:  dialSetPassword(""),
		uciChanges:   dialUCIChanges(""),
		uciRevert:    dialUCIRevert(""),
		uciApply:     dialUCIApply(""),
		uciConfirm:   dialUCIConfirm(""),
		rcList:       dialRCList(""),
		rcInit:       dialRCInit(""),
		pkgStatus:    dialPkgStatus(""),
		pkgUpdate:    dialPkgUpdate(""),
		pkgSearch:    dialPkgSearch(""),
		pkgInstalled: dialPkgInstalled(""),
		pkgInstall:   dialPkgAct("", "pkgInstall"),
		pkgRemove:    dialPkgAct("", "pkgRemove"),
		wanStatus:    dialWANStatus(""),
		deviceStats:  dialDeviceStats(""),
	}
}

// Hostname reads the configured hostname through rpcd's ACL-gated `uci` object.
func (b *NativeBackend) Hostname(ctx context.Context, sid string) (string, error) {
	return b.hostname(ctx, sid)
}

// SystemInfo fetches and maps live system info, gated by an rpcd ACL check.
func (b *NativeBackend) SystemInfo(ctx context.Context, sid string) (SystemInfo, error) {
	m, err := b.systemInfo(ctx, sid)
	if err != nil {
		return SystemInfo{}, err
	}
	return parseSystemInfo(m), nil
}

// Access reports whether rpcd grants the session object.function in scope.
func (b *NativeBackend) Access(ctx context.Context, sid, scope, object, function string) (bool, error) {
	return b.access(ctx, sid, scope, object, function)
}

// UCISet writes option values into a uci section through rpcd, gated by the sid.
func (b *NativeBackend) UCISet(ctx context.Context, sid, config, section string, values map[string]any) error {
	return b.uciSet(ctx, sid, config, section, values)
}

// UCICommit persists staged changes to a uci config through rpcd, gated by the sid.
func (b *NativeBackend) UCICommit(ctx context.Context, sid, config string) error {
	return b.uciCommit(ctx, sid, config)
}

// UCIConfig reads a whole uci config through rpcd, gated by the sid.
func (b *NativeBackend) UCIConfig(ctx context.Context, sid, config string) (map[string]any, error) {
	return b.uciConfig(ctx, sid, config)
}

// UCIAdd creates a new anonymous section of secType through rpcd, gated by the sid,
// and returns its id.
func (b *NativeBackend) UCIAdd(ctx context.Context, sid, config, secType string) (string, error) {
	return b.uciAdd(ctx, sid, config, secType)
}

// UCIDelete removes a section through rpcd, gated by the sid.
func (b *NativeBackend) UCIDelete(ctx context.Context, sid, config, section string) error {
	return b.uciDelete(ctx, sid, config, section)
}

// SetPassword sets username's system password through rpcd's `luci` object, gated
// by the sid.
func (b *NativeBackend) SetPassword(ctx context.Context, sid, username, password string) error {
	return b.setPassword(ctx, sid, username, password)
}

// UCIChanges reads the pending uci changes across all configs through rpcd, gated
// by the sid.
func (b *NativeBackend) UCIChanges(ctx context.Context, sid string) (map[string][][]string, error) {
	return b.uciChanges(ctx, sid)
}

// UCIRevert discards a config's staged changes through rpcd, gated by the sid.
func (b *NativeBackend) UCIRevert(ctx context.Context, sid, config string) error {
	return b.uciRevert(ctx, sid, config)
}

// UCIApply commits all dirty configs with rpcd's rollback armed, gated by the sid.
func (b *NativeBackend) UCIApply(ctx context.Context, sid string, timeout int) error {
	return b.uciApply(ctx, sid, timeout)
}

// UCIConfirm disarms a pending rollback through rpcd, gated by the sid.
func (b *NativeBackend) UCIConfirm(ctx context.Context, sid string) error {
	return b.uciConfirm(ctx, sid)
}

func (b *NativeBackend) RCList(ctx context.Context, sid string) (map[string]RCState, error) {
	return b.rcList(ctx, sid)
}

func (b *NativeBackend) RCInit(ctx context.Context, sid, name, action string) error {
	return b.rcInit(ctx, sid, name, action)
}

func (b *NativeBackend) PkgStatus(ctx context.Context, sid string) (int64, error) {
	return b.pkgStatus(ctx, sid)
}

func (b *NativeBackend) PkgUpdate(ctx context.Context, sid string) error {
	return b.pkgUpdate(ctx, sid)
}

func (b *NativeBackend) PkgSearch(ctx context.Context, sid, query string) ([]Package, int, error) {
	return b.pkgSearch(ctx, sid, query)
}

func (b *NativeBackend) PkgInstalled(ctx context.Context, sid string) ([]Package, error) {
	return b.pkgInstalled(ctx, sid)
}

func (b *NativeBackend) PkgInstall(ctx context.Context, sid, name string) error {
	return b.pkgInstall(ctx, sid, name)
}

func (b *NativeBackend) PkgRemove(ctx context.Context, sid, name string) error {
	return b.pkgRemove(ctx, sid, name)
}

func (b *NativeBackend) WANStatus(ctx context.Context, sid string) (WANState, error) {
	return b.wanStatus(ctx, sid)
}

func (b *NativeBackend) DeviceStats(ctx context.Context, sid, device string) (DeviceStats, error) {
	return b.deviceStats(ctx, sid, device)
}

// dialHostname reads system.@system[0].hostname via rpcd's `uci get`, carrying
// the sid so rpcd applies the session's ACLs.
func dialHostname(socket string) hostnameFn {
	return func(_ context.Context, sid string) (string, error) {
		c, err := ubus.Dial(socket)
		if err != nil {
			return "", err
		}
		defer c.Close()

		id, err := c.Lookup("uci")
		if err != nil {
			return "", err
		}
		res, err := c.InvokeArgs(id, "get", map[string]string{
			"ubus_rpc_session": sid,
			"config":           "system",
			"section":          "@system[0]",
			"option":           "hostname",
		})
		if err != nil {
			return "", err
		}
		val, _ := res["value"].(string)
		if val == "" {
			return "", fmt.Errorf("openwrt: hostname not set in uci system config")
		}
		return val, nil
	}
}

// dialSystemInfo probes rpcd `session.access` for the sid, then invokes
// `system info` directly. system is procd's object (rpcd does not proxy it), so
// the ACL is enforced by the pre-check — the pattern for any non-rpcd ubus call.
func dialSystemInfo(socket string) systemInfoFn {
	return func(_ context.Context, sid string) (map[string]any, error) {
		c, err := ubus.Dial(socket)
		if err != nil {
			return nil, err
		}
		defer c.Close()

		if ok, err := probeAccess(c, sid, "ubus", "system", "info"); err != nil || !ok {
			return nil, ErrAccessDenied
		}
		id, err := c.Lookup("system")
		if err != nil {
			return nil, err
		}
		return c.Invoke(id, "info")
	}
}

// dialAccess returns an accessFn that dials the socket per call and probes rpcd's
// session.access for the session — the shell's ACL enforcement point (ADR-007).
func dialAccess(socket string) accessFn {
	return func(_ context.Context, sid, scope, object, function string) (bool, error) {
		c, err := ubus.Dial(socket)
		if err != nil {
			return false, err
		}
		defer c.Close()
		return probeAccess(c, sid, scope, object, function)
	}
}

// dialUCISet returns a uciSetFn that writes option values via rpcd's `uci` object
// (method `set`), carrying the sid so rpcd applies the operator's ACLs. values is
// encoded as the nested `values:{}` table uci.set expects.
func dialUCISet(socket string) uciSetFn {
	return func(_ context.Context, sid, config, section string, values map[string]any) error {
		c, err := ubus.Dial(socket)
		if err != nil {
			return err
		}
		defer c.Close()
		id, err := c.Lookup("uci")
		if err != nil {
			return err
		}
		_, err = c.InvokeTable(id, "set", map[string]any{
			"ubus_rpc_session": sid,
			"config":           config,
			"section":          section,
			"values":           values,
		})
		return err
	}
}

// dialUCIConfig returns a uciConfigFn that reads a whole config via rpcd's `uci`
// object (method `get` with no section), carrying the sid. rpcd returns the
// config's sections under `values` — section name → section table — which the
// shell hands to a plugin as its read snapshot (ADR-007). A config the operator
// may not read comes back with no values, which surfaces as an empty snapshot.
func dialUCIConfig(socket string) uciConfigFn {
	return func(_ context.Context, sid, config string) (map[string]any, error) {
		c, err := ubus.Dial(socket)
		if err != nil {
			return nil, err
		}
		defer c.Close()
		id, err := c.Lookup("uci")
		if err != nil {
			return nil, err
		}
		res, err := c.InvokeArgs(id, "get", map[string]string{
			"ubus_rpc_session": sid,
			"config":           config,
		})
		if err != nil {
			return nil, err
		}
		values, _ := res["values"].(map[string]any)
		return values, nil
	}
}

// dialUCIAdd returns a uciAddFn that creates an anonymous section of secType via
// rpcd's `uci` object (method `add`), carrying the sid, and returns rpcd's new
// section id. The caller commits the config afterwards, as with `set`.
func dialUCIAdd(socket string) uciAddFn {
	return func(_ context.Context, sid, config, secType string) (string, error) {
		c, err := ubus.Dial(socket)
		if err != nil {
			return "", err
		}
		defer c.Close()
		id, err := c.Lookup("uci")
		if err != nil {
			return "", err
		}
		res, err := c.InvokeArgs(id, "add", map[string]string{
			"ubus_rpc_session": sid,
			"config":           config,
			"type":             secType,
		})
		if err != nil {
			return "", err
		}
		section, _ := res["section"].(string)
		return section, nil
	}
}

// dialSetPassword returns a passwdFn that sets a user's password via Verso's own
// `verso` rpcd helper (method `setPassword`), carrying the sid. The helper runs
// as root under rpcd and self-gates on the session's ACL (it verifies
// session.access for verso.setPassword before acting), so no LuCI dependency and
// no ambient privilege in the shell (ADR-007). The password travels only in the
// ubus payload over the local socket, never as a process argument.
func dialSetPassword(socket string) passwdFn {
	return func(_ context.Context, sid, username, password string) error {
		c, err := ubus.Dial(socket)
		if err != nil {
			return err
		}
		defer c.Close()
		id, err := c.Lookup("verso")
		if err != nil {
			return err
		}
		_, err = c.InvokeArgs(id, "setPassword", map[string]string{
			"ubus_rpc_session": sid,
			"username":         username,
			"password":         password,
		})
		return err
	}
}

// dialUCIDelete returns a uciDeleteFn that removes a section via rpcd's `uci`
// object (method `delete`), carrying the sid. The caller commits afterwards.
func dialUCIDelete(socket string) uciDeleteFn {
	return func(_ context.Context, sid, config, section string) error {
		c, err := ubus.Dial(socket)
		if err != nil {
			return err
		}
		defer c.Close()
		id, err := c.Lookup("uci")
		if err != nil {
			return err
		}
		_, err = c.InvokeArgs(id, "delete", map[string]string{
			"ubus_rpc_session": sid,
			"config":           config,
			"section":          section,
		})
		return err
	}
}

// dialUCICommit returns a uciCommitFn that persists a config's staged changes via
// rpcd's `uci` object (method `commit`), carrying the sid.
func dialUCICommit(socket string) uciCommitFn {
	return func(_ context.Context, sid, config string) error {
		c, err := ubus.Dial(socket)
		if err != nil {
			return err
		}
		defer c.Close()
		id, err := c.Lookup("uci")
		if err != nil {
			return err
		}
		_, err = c.InvokeArgs(id, "commit", map[string]string{
			"ubus_rpc_session": sid,
			"config":           config,
		})
		return err
	}
}

// dialUCIChanges returns a uciChangesFn that reads all pending changes via rpcd's
// `uci` object (method `changes`, no config), carrying the sid. rpcd answers
// {changes: {config: [[op, section, option?, value?], …]}}; configs the operator
// may not read simply do not appear.
func dialUCIChanges(socket string) uciChangesFn {
	return func(_ context.Context, sid string) (map[string][][]string, error) {
		c, err := ubus.Dial(socket)
		if err != nil {
			return nil, err
		}
		defer c.Close()
		id, err := c.Lookup("uci")
		if err != nil {
			return nil, err
		}
		res, err := c.InvokeArgs(id, "changes", map[string]string{
			"ubus_rpc_session": sid,
		})
		if err != nil {
			return nil, err
		}
		return parseChanges(res["changes"]), nil
	}
}

// parseChanges maps rpcd's generic changes table onto config → change tuples,
// dropping anything that is not a list of strings.
func parseChanges(v any) map[string][][]string {
	out := map[string][][]string{}
	byConfig, ok := v.(map[string]any)
	if !ok {
		return out
	}
	for config, recs := range byConfig {
		list, ok := recs.([]any)
		if !ok {
			continue
		}
		for _, rec := range list {
			tuple, ok := rec.([]any)
			if !ok {
				continue
			}
			change := make([]string, 0, len(tuple))
			for _, f := range tuple {
				s, ok := f.(string)
				if !ok {
					change = nil
					break
				}
				change = append(change, s)
			}
			if change != nil {
				out[config] = append(out[config], change)
			}
		}
	}
	return out
}

// dialUCIRevert returns a uciRevertFn that discards a config's staged changes via
// rpcd's `uci` object (method `revert`), carrying the sid.
func dialUCIRevert(socket string) uciRevertFn {
	return func(_ context.Context, sid, config string) error {
		c, err := ubus.Dial(socket)
		if err != nil {
			return err
		}
		defer c.Close()
		id, err := c.Lookup("uci")
		if err != nil {
			return err
		}
		_, err = c.InvokeArgs(id, "revert", map[string]string{
			"ubus_rpc_session": sid,
			"config":           config,
		})
		return err
	}
}

// dialUCIApply returns a uciApplyFn that commits all dirty configs via rpcd's
// `uci` object (method `apply` with rollback), carrying the sid. rpcd checkpoints
// the configs, commits, lets procd's config triggers reload services, and reverts
// on the device unless `confirm` lands within timeout seconds — the safety net
// runs on the router, the only place it works when the operator cuts their own
// connectivity (ADR-010).
func dialUCIApply(socket string) uciApplyFn {
	return func(_ context.Context, sid string, timeout int) error {
		c, err := ubus.Dial(socket)
		if err != nil {
			return err
		}
		defer c.Close()
		id, err := c.Lookup("uci")
		if err != nil {
			return err
		}
		_, err = c.InvokeTable(id, "apply", map[string]any{
			"ubus_rpc_session": sid,
			"rollback":         true,
			"timeout":          timeout,
		})
		return err
	}
}

// dialUCIConfirm returns a uciConfirmFn that disarms a pending rollback via rpcd's
// `uci` object (method `confirm`), carrying the sid.
func dialUCIConfirm(socket string) uciConfirmFn {
	return func(_ context.Context, sid string) error {
		c, err := ubus.Dial(socket)
		if err != nil {
			return err
		}
		defer c.Close()
		id, err := c.Lookup("uci")
		if err != nil {
			return err
		}
		_, err = c.InvokeArgs(id, "confirm", map[string]string{
			"ubus_rpc_session": sid,
		})
		return err
	}
}

// probeAccess asks rpcd whether the session may call object.function within the
// ubus ACL scope. It returns the decision and any transport error separately, so
// a caller can tell a denial (false, nil) from an unreachable rpcd (_, err) and
// fail closed. It is Verso's replacement for the check the HTTP→ubus bridge does.
func probeAccess(c *ubus.Client, sid, scope, object, function string) (bool, error) {
	id, err := c.Lookup("session")
	if err != nil {
		return false, err
	}
	res, err := c.InvokeArgs(id, "access", map[string]string{
		"ubus_rpc_session": sid,
		"scope":            scope,
		"object":           object,
		"function":         function,
	})
	if err != nil {
		return false, err
	}
	// ubus encodes booleans as the blobmsg INT8/BOOL type, which the client
	// decodes to int64 (1/0), not a Go bool.
	switch v := res["access"].(type) {
	case bool:
		return v, nil
	case int64:
		return v != 0, nil
	}
	return false, nil
}

// parseSystemInfo maps the generic ubus result table onto SystemInfo.
func parseSystemInfo(m map[string]any) SystemInfo {
	var si SystemInfo
	si.Uptime = asInt64(m["uptime"])
	if load, ok := m["load"].([]any); ok {
		for i := 0; i < len(si.Load) && i < len(load); i++ {
			si.Load[i] = asInt64(load[i])
		}
	}
	if mem, ok := m["memory"].(map[string]any); ok {
		si.Memory.Total = asInt64(mem["total"])
		si.Memory.Free = asInt64(mem["free"])
		si.Memory.Available = asInt64(mem["available"])
	}
	return si
}

func asInt64(v any) int64 {
	switch n := v.(type) {
	case int64:
		return n
	case float64:
		return int64(n)
	}
	return 0
}

// asBool reads a blobmsg boolean, which the ubus client may decode as a Go
// bool or as an int8-carried int64 (1/0) depending on the sender's type tag.
func asBool(v any) bool {
	switch b := v.(type) {
	case bool:
		return b
	case int64:
		return b != 0
	case float64:
		return b != 0
	}
	return false
}

// dialRCList reads procd's rc table — every init script with its boot-enabled
// flag and procd's live running view — after probing the session's
// ubus/rc/list access (ADR-011: plugin service state).
func dialRCList(socket string) rcListFn {
	return func(_ context.Context, sid string) (map[string]RCState, error) {
		c, err := ubus.Dial(socket)
		if err != nil {
			return nil, err
		}
		defer c.Close()

		if ok, err := probeAccess(c, sid, "ubus", "rc", "list"); err != nil || !ok {
			return nil, ErrAccessDenied
		}
		id, err := c.Lookup("rc")
		if err != nil {
			return nil, err
		}
		res, err := c.Invoke(id, "list")
		if err != nil {
			return nil, err
		}
		out := make(map[string]RCState, len(res))
		for name, v := range res {
			t, ok := v.(map[string]any)
			if !ok {
				continue
			}
			out[name] = RCState{Enabled: asBool(t["enabled"]), Running: asBool(t["running"])}
		}
		return out, nil
	}
}

// pkgCallTimeout is the widened per-message deadline for helper package
// calls: an index refresh or an install does real network and disk work on
// the far side.
const pkgCallTimeout = 90 * time.Second

// dialVerso opens a ubus client aimed at the privileged helper's object with
// the package-call deadline. The helper self-gates on the sid it is handed.
func dialVerso(socket string) (*ubus.Client, uint32, error) {
	c, err := ubus.Dial(socket)
	if err != nil {
		return nil, 0, err
	}
	c.SetTimeout(pkgCallTimeout)
	id, err := c.Lookup("verso")
	if err != nil {
		c.Close()
		return nil, 0, err
	}
	return c, id, nil
}

func dialPkgStatus(socket string) pkgStatusFn {
	return func(_ context.Context, sid string) (int64, error) {
		c, id, err := dialVerso(socket)
		if err != nil {
			return 0, err
		}
		defer c.Close()
		res, err := c.InvokeArgs(id, "pkgStatus", map[string]string{"ubus_rpc_session": sid})
		if err != nil {
			return 0, err
		}
		return asInt64(res["checked_at"]), nil
	}
}

func dialPkgUpdate(socket string) pkgUpdateFn {
	return func(_ context.Context, sid string) error {
		c, id, err := dialVerso(socket)
		if err != nil {
			return err
		}
		defer c.Close()
		_, err = c.InvokeArgs(id, "pkgUpdate", map[string]string{"ubus_rpc_session": sid})
		return err
	}
}

func dialPkgSearch(socket string) pkgSearchFn {
	return func(_ context.Context, sid, query string) ([]Package, int, error) {
		c, id, err := dialVerso(socket)
		if err != nil {
			return nil, 0, err
		}
		defer c.Close()
		res, err := c.InvokeArgs(id, "pkgSearch", map[string]string{
			"ubus_rpc_session": sid,
			"query":            query,
		})
		if err != nil {
			return nil, 0, err
		}
		raw, _ := res["packages"].([]any)
		pkgs := make([]Package, 0, len(raw))
		for _, r := range raw {
			t, ok := r.(map[string]any)
			if !ok {
				continue
			}
			name, _ := t["name"].(string)
			version, _ := t["version"].(string)
			feed, _ := t["feed"].(string)
			desc, _ := t["description"].(string)
			pkgs = append(pkgs, Package{
				Name: name, Version: version, Feed: feed, Description: desc,
				Installed: asBool(t["installed"]),
			})
		}
		return pkgs, int(asInt64(res["total"])), nil
	}
}

func dialPkgInstalled(socket string) pkgInstalledFn {
	return func(_ context.Context, sid string) ([]Package, error) {
		c, id, err := dialVerso(socket)
		if err != nil {
			return nil, err
		}
		defer c.Close()
		res, err := c.InvokeArgs(id, "pkgInstalled", map[string]string{"ubus_rpc_session": sid})
		if err != nil {
			return nil, err
		}
		raw, _ := res["packages"].([]any)
		pkgs := make([]Package, 0, len(raw))
		for _, r := range raw {
			t, ok := r.(map[string]any)
			if !ok {
				continue
			}
			name, _ := t["name"].(string)
			version, _ := t["version"].(string)
			feed, _ := t["feed"].(string)
			desc, _ := t["description"].(string)
			license, _ := t["license"].(string)
			webpage, _ := t["webpage"].(string)
			pkgs = append(pkgs, Package{
				Name: name, Version: version, Feed: feed,
				Description: desc, License: license, Webpage: webpage,
				Size: asInt64(t["size"]), Installed: true,
			})
		}
		return pkgs, nil
	}
}

func dialPkgAct(socket, methodName string) pkgActFn {
	return func(_ context.Context, sid, name string) error {
		c, id, err := dialVerso(socket)
		if err != nil {
			return err
		}
		defer c.Close()
		_, err = c.InvokeArgs(id, methodName, map[string]string{
			"ubus_rpc_session": sid,
			"package":          name,
		})
		return err
	}
}

// dialRCInit drives one rc action on a named init script through procd, after
// probing the session's ubus/rc/init access. Name and action policy is the
// caller's (the shell restricts both); rpcd's session gate is the backstop.
func dialRCInit(socket string) rcInitFn {
	return func(_ context.Context, sid, name, action string) error {
		c, err := ubus.Dial(socket)
		if err != nil {
			return err
		}
		defer c.Close()

		if ok, err := probeAccess(c, sid, "ubus", "rc", "init"); err != nil || !ok {
			return ErrAccessDenied
		}
		id, err := c.Lookup("rc")
		if err != nil {
			return err
		}
		_, err = c.InvokeArgs(id, "init", map[string]string{"name": name, "action": action})
		return err
	}
}

// dialWANStatus reads network.interface.wan status through netifd, after
// probing the session's access to the object. Only the shell-rendered facts
// are mapped: up, and the l3 device carrying the uplink.
func dialWANStatus(socket string) wanStatusFn {
	return func(_ context.Context, sid string) (WANState, error) {
		c, err := ubus.Dial(socket)
		if err != nil {
			return WANState{}, err
		}
		defer c.Close()

		if ok, err := probeAccess(c, sid, "ubus", "network.interface.wan", "status"); err != nil || !ok {
			return WANState{}, ErrAccessDenied
		}
		id, err := c.Lookup("network.interface.wan")
		if err != nil {
			return WANState{}, err
		}
		res, err := c.Invoke(id, "status")
		if err != nil {
			return WANState{}, err
		}
		return parseWANState(res), nil
	}
}

// parseWANState maps netifd's interface status onto WANState. netifd reports
// l3_device once the protocol is up; device is the configured fallback.
func parseWANState(m map[string]any) WANState {
	ws := WANState{Up: asBool(m["up"])}
	if d, ok := m["l3_device"].(string); ok && d != "" {
		ws.Device = d
	} else if d, ok := m["device"].(string); ok {
		ws.Device = d
	}
	return ws
}

// dialDeviceStats reads network.device status for one named device, after
// probing the session's access to the object.
func dialDeviceStats(socket string) deviceStatsFn {
	return func(_ context.Context, sid, device string) (DeviceStats, error) {
		c, err := ubus.Dial(socket)
		if err != nil {
			return DeviceStats{}, err
		}
		defer c.Close()

		if ok, err := probeAccess(c, sid, "ubus", "network.device", "status"); err != nil || !ok {
			return DeviceStats{}, ErrAccessDenied
		}
		id, err := c.Lookup("network.device")
		if err != nil {
			return DeviceStats{}, err
		}
		res, err := c.InvokeTable(id, "status", map[string]any{"name": device})
		if err != nil {
			return DeviceStats{}, err
		}
		return parseDeviceStats(res), nil
	}
}

// parseDeviceStats maps network.device status onto DeviceStats. netifd
// reports speed as a string like "1000F" (Mbps plus duplex) — the leading
// digits are the number; an unknown speed ("-1", absent) maps to 0.
func parseDeviceStats(m map[string]any) DeviceStats {
	ds := DeviceStats{Carrier: asBool(m["carrier"])}
	if s, ok := m["speed"].(string); ok {
		n := 0
		for _, r := range s {
			if r < '0' || r > '9' {
				break
			}
			n = n*10 + int(r-'0')
		}
		ds.SpeedMbps = n
	}
	if st, ok := m["statistics"].(map[string]any); ok {
		ds.RxBytes = asInt64(st["rx_bytes"])
		ds.TxBytes = asInt64(st["tx_bytes"])
	}
	return ds
}
