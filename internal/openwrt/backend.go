// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

// Package openwrt reads live state from the OpenWrt backend through rpcd's
// ACL-gated ubus objects, carrying the caller's session id (ADR-007).
package openwrt

import (
	"context"
	"errors"
	"fmt"

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
	// Board reads the device's identity from `ubus call system board` — the
	// firmware release and kernel version — sid-gated like system info.
	Board(ctx context.Context, sid string) (Board, error)
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
	// SetPassword asks the persistent root helper to set username's system
	// password, carrying the operator's sid. The helper verifies the sid through
	// rpcd before acting; the shell itself is unprivileged and cannot write
	// /etc/shadow (ADR-007).
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
	// The package verbs (ADR-011 §4) ride the persistent privileged helper's Unix
	// socket. It self-gates on the sid and validates names and queries again on
	// its side. PkgStatus reports when the feed indexes were
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
	// WANConn reads the uplink's connection facts (address, gateway, DNS,
	// protocol) from the wan and wan6 interfaces — the overview's IPv4/IPv6
	// panel — sid-gated like WANStatus.
	WANConn(ctx context.Context, sid string) (WANConn, error)
	// IPv6Leases reads odhcpd's DHCPv6 leases (`dhcp ipv6leases`) — the client
	// DUID, hostname, and assigned addresses — sid-gated. DHCPv6 keys on the DUID,
	// not the MAC, so the roster joins a lease to a device by a shared address.
	IPv6Leases(ctx context.Context, sid string) ([]V6Lease, error)
	// DeviceStats reads one network device's link state and byte counters
	// (network.device status), sid-gated likewise. A throughput reading is the
	// delta between two of these.
	DeviceStats(ctx context.Context, sid, device string) (DeviceStats, error)
}

// WANState is the uplink's live condition, from network.interface.wan status.
type WANState struct {
	Up     bool
	Device string // the l3 device carrying the uplink, e.g. "wan0"
	Addr   string // the uplink's IPv4 address, "" until the protocol is up
	Uptime int64  // seconds since netifd brought this WAN interface up
}

// WANConn is the uplink's connection facts as the overview lists them: the IPv4
// side from the wan interface, the IPv6 side from wan6. Every string is empty
// when the protocol doesn't carry it (a v4-only uplink leaves the v6 fields bare).
type WANConn struct {
	V4Proto   string // "DHCP" | "PPPoE" | "Static" | …
	V4Addr    string // "172.30.1.171/24"
	V4Gateway string
	V4DNS     []string
	V6Proto   string // "DHCPv6 client" | …
	V6Prefix  string // the delegated prefix, "fd42:7ea:aa00::/56"
	V6Addr    string
	V6Gateway string
	V6DNS     []string
	V6Valid   int64 // seconds the prefix stays valid (the lease's time left); 0 = none
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
	Name        string `json:"name"`
	Version     string `json:"version"`
	Feed        string `json:"feed"`
	Description string `json:"description"`
	License     string `json:"license"`
	Webpage     string `json:"webpage"`
	Size        int64  `json:"size"` // package file size, bytes
	Installed   bool   `json:"installed"`
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

// Board is the subset of `ubus call system board` Verso renders: the firmware
// release ("OpenWrt 25.12.4"), the kernel version ("Linux 6.12.101"), the
// board_name that selects a hardware profile, and the human model string.
type Board struct {
	Firmware  string
	Kernel    string
	BoardName string
	Model     string
}

// V6Lease is one odhcpd DHCPv6 lease: the client DUID (its stable DHCPv6
// identity — there is no MAC in DHCPv6), its hostname, and the IPv6 addresses
// assigned to it.
type V6Lease struct {
	DUID     string
	Hostname string
	Addrs    []string
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
	systemBoardFn  func(ctx context.Context, sid string) (map[string]any, error)
	ipv6LeasesFn   func(ctx context.Context, sid string) (map[string]any, error)
	wanConnFn      func(ctx context.Context, sid string) (WANConn, error)
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
	systemBoard  systemBoardFn
	ipv6Leases   ipv6LeasesFn
	wanConn      wanConnFn
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
		systemBoard:  dialSystemBoard(""),
		ipv6Leases:   dialIPv6Leases(""),
		wanConn:      dialWANConn(""),
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

// Board reads `ubus call system board` and folds it to the firmware release and
// kernel version.
func (b *NativeBackend) Board(ctx context.Context, sid string) (Board, error) {
	m, err := b.systemBoard(ctx, sid)
	if err != nil {
		return Board{}, err
	}
	return parseBoard(m), nil
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

// SetPassword sets username's system password through the privileged `verso-rpcd`
// helper (the setPassword verb), gated by the sid.
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

func (b *NativeBackend) WANConn(ctx context.Context, sid string) (WANConn, error) {
	return b.wanConn(ctx, sid)
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

// dialSystemBoard returns a systemBoardFn that reads `ubus call system board`,
// the device's identity (firmware release, kernel), sid-gated like system info.
func dialSystemBoard(socket string) systemBoardFn {
	return func(_ context.Context, sid string) (map[string]any, error) {
		c, err := ubus.Dial(socket)
		if err != nil {
			return nil, err
		}
		defer c.Close()

		if ok, err := probeAccess(c, sid, "ubus", "system", "board"); err != nil || !ok {
			return nil, ErrAccessDenied
		}
		id, err := c.Lookup("system")
		if err != nil {
			return nil, err
		}
		return c.Invoke(id, "board")
	}
}

// dialIPv6Leases returns an ipv6LeasesFn that reads `ubus call dhcp ipv6leases`
// (odhcpd's DHCPv6 lease table), sid-gated like the other reads.
func dialIPv6Leases(socket string) ipv6LeasesFn {
	return func(_ context.Context, sid string) (map[string]any, error) {
		c, err := ubus.Dial(socket)
		if err != nil {
			return nil, err
		}
		defer c.Close()

		if ok, err := probeAccess(c, sid, "ubus", "dhcp", "ipv6leases"); err != nil || !ok {
			return nil, ErrAccessDenied
		}
		id, err := c.Lookup("dhcp")
		if err != nil {
			return nil, err
		}
		return c.Invoke(id, "ipv6leases")
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

// dialSetPassword returns a passwdFn that asks the resident Rust helper to set a
// user's password. The helper runs as root and self-gates on session.access for
// verso.setPassword, so the shell keeps no ambient privilege. The password
// travels only over the protected local socket, never in a process argument.
func dialSetPassword(socket string) passwdFn {
	return func(ctx context.Context, sid, username, password string) error {
		return callHelper(ctx, socket, "setPassword", sid, map[string]string{
			"username": username,
			"password": password,
		}, nil)
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

// IPv6Leases reads odhcpd's DHCPv6 lease table and folds it to a flat list of
// DUID + hostname + assigned addresses per client.
func (b *NativeBackend) IPv6Leases(ctx context.Context, sid string) ([]V6Lease, error) {
	m, err := b.ipv6Leases(ctx, sid)
	if err != nil {
		return nil, err
	}
	return parseV6Leases(m), nil
}

// parseV6Leases folds the `dhcp ipv6leases` response — {device: {<iface>:
// {leases: [...]}}} — into a flat list, keeping only leases that carry a DUID.
func parseV6Leases(m map[string]any) []V6Lease {
	var out []V6Lease
	devs, _ := m["device"].(map[string]any)
	for _, d := range devs {
		iface, _ := d.(map[string]any)
		leases, _ := iface["leases"].([]any)
		for _, l := range leases {
			lease, _ := l.(map[string]any)
			duid, _ := lease["duid"].(string)
			if duid == "" {
				continue
			}
			host, _ := lease["hostname"].(string)
			out = append(out, V6Lease{DUID: duid, Hostname: host, Addrs: leaseV6Addrs(lease)})
		}
	}
	return out
}

// leaseV6Addrs collects a lease's assigned addresses from its ipv6-addr array.
func leaseV6Addrs(lease map[string]any) []string {
	arr, _ := lease["ipv6-addr"].([]any)
	out := make([]string, 0, len(arr))
	for _, a := range arr {
		if am, ok := a.(map[string]any); ok {
			if s, _ := am["address"].(string); s != "" {
				out = append(out, s)
			}
		}
	}
	return out
}

// parseBoard folds `system board` to the firmware release (distribution +
// version, e.g. "OpenWrt 25.12.4") and the kernel version ("Linux 6.12.101").
func parseBoard(m map[string]any) Board {
	str := func(v any) string { s, _ := v.(string); return s }
	var b Board
	b.BoardName = str(m["board_name"])
	b.Model = str(m["model"])
	if k := str(m["kernel"]); k != "" {
		b.Kernel = "Linux " + k
	}
	if rel, ok := m["release"].(map[string]any); ok {
		dist, ver := str(rel["distribution"]), str(rel["version"])
		switch {
		case dist != "" && ver != "":
			b.Firmware = dist + " " + ver
		case dist != "":
			b.Firmware = dist
		default:
			b.Firmware = ver
		}
		if b.Firmware == "" {
			b.Firmware = str(rel["description"])
		}
	}
	return b
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

func dialPkgStatus(socket string) pkgStatusFn {
	return func(ctx context.Context, sid string) (int64, error) {
		var result struct {
			CheckedAt int64 `json:"checked_at"`
		}
		err := callHelper(ctx, socket, "pkgStatus", sid, nil, &result)
		return result.CheckedAt, err
	}
}

func dialPkgUpdate(socket string) pkgUpdateFn {
	return func(ctx context.Context, sid string) error {
		return callHelper(ctx, socket, "pkgUpdate", sid, nil, nil)
	}
}

func dialPkgSearch(socket string) pkgSearchFn {
	return func(ctx context.Context, sid, query string) ([]Package, int, error) {
		var result struct {
			Packages []Package `json:"packages"`
			Total    int       `json:"total"`
		}
		err := callHelper(ctx, socket, "pkgSearch", sid, map[string]string{"query": query}, &result)
		return result.Packages, result.Total, err
	}
}

func dialPkgInstalled(socket string) pkgInstalledFn {
	return func(ctx context.Context, sid string) ([]Package, error) {
		var result struct {
			Packages []Package `json:"packages"`
		}
		err := callHelper(ctx, socket, "pkgInstalled", sid, nil, &result)
		return result.Packages, err
	}
}

func dialPkgAct(socket, methodName string) pkgActFn {
	return func(ctx context.Context, sid, name string) error {
		return callHelper(ctx, socket, methodName, sid, map[string]string{"package": name}, nil)
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
// l3_device once the protocol is up; device is the configured fallback. The
// address is the first entry of ipv4-address, present only while up.
func parseWANState(m map[string]any) WANState {
	ws := WANState{Up: asBool(m["up"]), Uptime: asInt64(m["uptime"])}
	if d, ok := m["l3_device"].(string); ok && d != "" {
		ws.Device = d
	} else if d, ok := m["device"].(string); ok {
		ws.Device = d
	}
	if addrs, ok := m["ipv4-address"].([]any); ok && len(addrs) > 0 {
		if entry, ok := addrs[0].(map[string]any); ok {
			ws.Addr, _ = entry["address"].(string)
		}
	}
	return ws
}

// dialWANConn reads netifd's interface dump and discovers the IPv6 side attached
// to wan. This covers explicit wan6, automatically-created wan_6, custom names
// sharing wan's L3 device, and protocols that put both families directly on wan.
func dialWANConn(socket string) wanConnFn {
	return func(_ context.Context, sid string) (WANConn, error) {
		c, err := ubus.Dial(socket)
		if err != nil {
			return WANConn{}, err
		}
		defer c.Close()
		if ok, err := probeAccess(c, sid, "ubus", "network.interface", "dump"); err != nil || !ok {
			return WANConn{}, ErrAccessDenied
		}
		id, err := c.Lookup("network.interface")
		if err != nil {
			return WANConn{}, err
		}
		dump, err := c.Invoke(id, "dump")
		if err != nil {
			return WANConn{}, err
		}
		wan, wan6 := wanStatuses(dump)
		if wan == nil {
			return WANConn{}, ErrAccessDenied
		}
		return parseWANConn(wan, wan6), nil
	}
}

// wanStatuses returns the configured wan status and the best IPv6 companion in
// a netifd dump. A matching L3 device is the authoritative relationship; the
// conventional names only rank otherwise-equivalent candidates.
func wanStatuses(dump map[string]any) (map[string]any, map[string]any) {
	entries, _ := dump["interface"].([]any)
	var wan map[string]any
	for _, value := range entries {
		entry, _ := value.(map[string]any)
		if name, _ := entry["interface"].(string); name == "wan" {
			wan = entry
			break
		}
	}
	if wan == nil {
		return nil, nil
	}

	wanDevice := interfaceDevice(wan)
	var best map[string]any
	bestScore := 0
	for _, value := range entries {
		entry, _ := value.(map[string]any)
		name, _ := entry["interface"].(string)
		if name == "" || name == "wan" || !interfaceCarriesIPv6(entry) {
			continue
		}
		score := 0
		if wanDevice != "" && interfaceDevice(entry) == wanDevice {
			score = 100
		}
		switch name {
		case "wan6":
			score += 20
		case "wan_6":
			score += 10
		}
		if score > bestScore {
			best, bestScore = entry, score
		}
	}
	return wan, best
}

func interfaceDevice(status map[string]any) string {
	if device, _ := status["l3_device"].(string); device != "" {
		return device
	}
	device, _ := status["device"].(string)
	return device
}

func interfaceCarriesIPv6(status map[string]any) bool {
	if proto, _ := status["proto"].(string); proto == "dhcpv6" {
		return true
	}
	for _, key := range []string{"ipv6-address", "ipv6-prefix"} {
		if values, ok := status[key].([]any); ok && len(values) != 0 {
			return true
		}
	}
	return false
}

// parseWANConn maps the wan/wan6 interface status onto WANConn: the first
// address (with its mask), the default route's nexthop as the gateway, the DNS
// list, the delegated IPv6 prefix, and the prefix's valid lifetime as the lease
// time left. Some netifd protocols expose both families directly on wan, so wan
// is also the fallback source for IPv6 fields when there is no separate wan6.
func parseWANConn(v4, v6 map[string]any) WANConn {
	var wc WANConn
	str := func(v any) string { s, _ := v.(string); return s }
	// addr formats ipv{4,6}-address[0] as "address/mask".
	addr := func(m map[string]any, key string) string {
		list, ok := m[key].([]any)
		if !ok || len(list) == 0 {
			return ""
		}
		e, ok := list[0].(map[string]any)
		if !ok {
			return ""
		}
		a := str(e["address"])
		if a == "" {
			return ""
		}
		if mask := asInt64(e["mask"]); mask > 0 {
			return fmt.Sprintf("%s/%d", a, mask)
		}
		return a
	}
	// gateway is the nexthop of the default route (target all-zeros, mask 0).
	gateway := func(m map[string]any, wantTarget string) string {
		routes, ok := m["route"].([]any)
		if !ok {
			return ""
		}
		for _, r := range routes {
			e, ok := r.(map[string]any)
			if ok && str(e["target"]) == wantTarget && asInt64(e["mask"]) == 0 {
				return str(e["nexthop"])
			}
		}
		return ""
	}
	dns := func(m map[string]any) []string {
		list, ok := m["dns-server"].([]any)
		if !ok {
			return nil
		}
		out := make([]string, 0, len(list))
		for _, d := range list {
			if s := str(d); s != "" {
				out = append(out, s)
			}
		}
		return out
	}
	proto := func(p string) string {
		switch p {
		case "dhcp":
			return "DHCP"
		case "pppoe":
			return "PPPoE"
		case "static":
			return "Static"
		case "dhcpv6":
			return "DHCPv6 client"
		}
		return p
	}

	wc.V4Proto = proto(str(v4["proto"]))
	wc.V4Addr = addr(v4, "ipv4-address")
	wc.V4Gateway = gateway(v4, "0.0.0.0")
	wc.V4DNS = dns(v4)

	v6Source := v6
	if v6Source == nil {
		v6Source = v4
	}
	if v6Source != nil {
		wc.V6Proto = proto(str(v6Source["proto"]))
		wc.V6Addr = addr(v6Source, "ipv6-address")
		wc.V6Gateway = gateway(v6Source, "::")
		wc.V6DNS = dns(v6Source)
		if pfx, ok := v6Source["ipv6-prefix"].([]any); ok && len(pfx) > 0 {
			if e, ok := pfx[0].(map[string]any); ok {
				if a := str(e["address"]); a != "" {
					wc.V6Prefix = fmt.Sprintf("%s/%d", a, asInt64(e["mask"]))
				}
				wc.V6Valid = asInt64(e["valid"])
			}
		}
	}
	return wc
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
