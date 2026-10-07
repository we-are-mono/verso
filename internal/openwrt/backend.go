// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

// Package openwrt reads live state from the OpenWrt backend through rpcd's
// ACL-gated ubus objects, carrying the caller's session id (ADR-007).
package openwrt

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/we-are-mono/verso/internal/ubus"
)

// ErrAccessDenied is returned when rpcd's ACLs refuse the session the operation.
var ErrAccessDenied = errors.New("openwrt: access denied by rpcd ACL")

// ErrOptionNotFound reports that an option-level clear found nothing to clear:
// rpcd answers `uci delete` for an option a section does not carry with
// UBUS_STATUS_NOT_FOUND. It is the domain reading of that status — the option is
// already unset — so a caller can tell it from a refusal or a transport failure
// without knowing anything about ubus. Only the option-level shape yields it: a
// section that is not there is a different question, and one this never answers.
var ErrOptionNotFound = errors.New("openwrt: uci option not set")

// ZeroSID is rpcd's local-root convention: an all-zero session id, presented by
// automation that runs as root on the device itself and therefore has no
// operator session to borrow (ADR-014 §3). It authorizes nothing on its own —
// verso-rpcd grants it exactly the two update-check read verbs, and only when the
// calling process is genuinely uid 0. Verso's shell never uses it: an operator's
// action always rides that operator's own session (ADR-007).
const ZeroSID = "00000000000000000000000000000000"

// Backend reads live state from the OpenWrt system on behalf of a session. It is
// the injected seam (ADR-003): callers depend on this interface, tests supply a
// fake. Every method takes the rpcd session id (sid) so rpcd — not Verso —
// authorizes the operation (ADR-007).
type Backend interface {
	SystemInfo(ctx context.Context, sid string) (SystemInfo, error)
	// Board reads the device's identity from `ubus call system board` — the
	// firmware release and kernel version — sid-gated like system info.
	Board(ctx context.Context, sid string) (Board, error)
	// Hostname is the running kernel hostname, also available before sign-in.
	// Configured and staged names belong to the System plugin's UCI snapshot.
	Hostname(ctx context.Context, sid string) (string, error)
	// Access asks rpcd whether the session may call object.function within the
	// given ACL scope. It is the enforcement point the shell uses to gate plugin
	// writes on the operator's rpcd ACLs (ADR-007). A transport error is distinct
	// from a denial, so callers can fail closed on the former.
	Access(ctx context.Context, sid, scope, object, function string) (bool, error)
	// UCISet writes config through rpcd's ACL-gated `uci` object, carrying the
	// operator's sid so rpcd — not Verso — authorizes the write. The shell
	// performs writes on a plugin's behalf (ADR-007), so a plugin holds no
	// write privilege and no session credential of its own.
	UCISet(ctx context.Context, sid, config, section string, values map[string]any) error
	// UCIConfig reads a whole uci config through rpcd's ACL-gated `uci` object,
	// carrying the sid. It returns the `values` map — section name → section table
	// (its `.type`/`.name` meta and options) — so the shell can hand a plugin a read
	// snapshot of a config the plugin declared, without the plugin holding a session
	// or reading /etc/config (ADR-007). rpcd scopes the read to the operator, so an
	// unreadable config yields an empty snapshot rather than an error.
	UCIConfig(ctx context.Context, sid, config string) (map[string]any, error)
	// UCIAdd creates a new section of secType in config through rpcd's `uci`
	// object, carrying the sid, and returns the new section's id. An empty name
	// asks for the anonymous section a uci-backed repeater adds (ADR-005 §7): the
	// shell — not the plugin — performs the structural change, within the plugin's
	// declared write scope. A name asks for the named typed section a config is
	// meant to be read by (`config updates 'updates'`), which is the shape Verso's
	// own settings take (ADR-013 §1).
	UCIAdd(ctx context.Context, sid, config, secType, name string) (string, error)
	// UCIDelete removes a section from config through rpcd, carrying the sid, or
	// one option of that section when option is non-empty — rpcd's `uci delete`
	// takes both shapes. It realizes the "remove" of a uci-backed repeater
	// (ADR-005 §7) and the clearing of a single option, which is how an editor
	// unsets a setting it owns rather than writing it empty.
	UCIDelete(ctx context.Context, sid, config, section, option string) error
	// UCIOrder rewrites the sequence of a config's sections through rpcd's `uci`
	// object, carrying the sid; sections is the config's whole order, not a
	// fragment. It realizes the drag of a reorderable listing — the shell, not
	// the plugin, performs the structural change — and stages like every other
	// write (ADR-010), so the review drawer's apply is what makes the new order live.
	UCIOrder(ctx context.Context, sid, config string, sections []string) error
	// SetPassword asks the persistent root helper to set username's system
	// password, carrying the operator's sid. The helper verifies the sid through
	// rpcd before acting; the shell itself is unprivileged and cannot write
	// /etc/shadow (ADR-007).
	SetPassword(ctx context.Context, sid, username, password string) error
	// SetSystemTime asks the persistent root helper to set the kernel clock from
	// one validated local datetime and its POSIX timezone. It is the privileged,
	// non-UCI tail of System → General's save-then-apply transaction.
	SetSystemTime(ctx context.Context, sid, datetime, timezone string) error
	// RootHasPassword reports whether root has a password set — read from
	// /etc/shadow by the persistent root helper (the shell is unprivileged and
	// cannot read it), carrying the operator's sid. It returns only the boolean,
	// never the hash.
	RootHasPassword(ctx context.Context, sid string) (bool, error)
	// CreateBackup asks OpenWrt's sysupgrade machinery to write its native
	// configuration archive to path. RestoreBackup applies that same format.
	// The path is a shell-created, private temporary file; archive bytes never
	// cross the helper's JSON protocol.
	CreateBackup(ctx context.Context, sid, path string) error
	RestoreBackup(ctx context.Context, sid, path string) error
	// ValidateFirmware runs OpenWrt's own sysupgrade compatibility check against
	// an uploaded image and returns its signed metadata. InstallFirmware repeats
	// that check in the privileged helper immediately before starting sysupgrade.
	ValidateFirmware(ctx context.Context, sid, path string) (FirmwareInfo, error)
	InstallFirmware(ctx context.Context, sid, path string) error
	Restart(ctx context.Context, sid string) error
	FactoryReset(ctx context.Context, sid string) error
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
	// PkgBrowse pages through the local index, including installed packages.
	PkgBrowse(ctx context.Context, sid, query string, offset int) (PackagePage, error)
	PkgFiles(ctx context.Context, sid, name string) ([]string, error)
	PkgUpgradeOne(ctx context.Context, sid, name string) error
	// PkgInstalled lists every installed package (no descriptions).
	PkgInstalled(ctx context.Context, sid string) ([]Package, error)
	PkgInstall(ctx context.Context, sid, name string) error
	PkgRemove(ctx context.Context, sid, name string) error
	// PkgUpgradable lists the installed packages the configured feeds hold a newer
	// copy of, and PkgUpgrade installs every one of them. Both ride the helper's
	// package mutex, so neither overlaps a refresh, a search, or each other.
	PkgUpgradable(ctx context.Context, sid string) ([]PackageUpgrade, error)
	PkgUpgrade(ctx context.Context, sid string) error
	// FirmwareCheck asks the device's attended-sysupgrade server, through owut,
	// whether it can build this device a newer image. It never downloads or
	// installs anything. A device the check cannot answer for is not an error: the
	// result names the rung it landed on instead (see FirmwareUpdate.State).
	FirmwareCheck(ctx context.Context, sid string) (FirmwareUpdate, error)
	// FirmwareUpgrade is the act that check leads to: the helper has owut build,
	// download and verify an image for this device, then starts the install. It
	// carries no arguments at all — owut reads the board, the package set and uci
	// on the device, so there is no version for a caller to name (ADR-007).
	//
	// It returns when the image is on the device and the install has been
	// started, not when the router is running it: the install ends in sysupgrade,
	// which takes the helper and this shell down with the rest of userspace. So
	// nil means the image is on the device, verified, and being written; an
	// error is a failure that happened before anything was written — the router
	// is still running the build it started with, and the message is owut's own.
	FirmwareUpgrade(ctx context.Context, sid string) error
	// WANStatus discovers every live uplink from netifd's active default routes
	// and the kernel FIB. Roles attach to exact L3 devices; logical-interface
	// names and transport ancestry are never used as classifiers.
	WANStatus(ctx context.Context, sid string) (WANState, error)
	// WANConn reads the preferred main-table IPv4 and IPv6 uplinks' connection
	// facts for the overview panel, without assuming wan/wan6 names.
	WANConn(ctx context.Context, sid string) (WANConn, error)
	// IPv6Leases reads odhcpd's DHCPv6 leases (`dhcp ipv6leases`) — the client
	// DUID, hostname, and assigned addresses — sid-gated. DHCPv6 keys on the DUID,
	// not the MAC, so the roster joins a lease to a device by a shared address.
	IPv6Leases(ctx context.Context, sid string) ([]V6Lease, error)
	// DeviceStats reads one network device's byte counters
	// (network.device status), sid-gated likewise. A throughput reading is the
	// delta between two of these.
	DeviceStats(ctx context.Context, sid, device string) (DeviceStats, error)
	// FirewallCounters reads fw4's live nftables hit counters — one entry per
	// (chain, rule name) with packets and bytes — through the privileged helper,
	// which lists the ruleset as root and self-gates on the sid. The result stays
	// raw JSON: the shell brokers it to a declaring plugin (ADR-007) and the plugin,
	// not the shell, owns what a firewall counter means.
	FirewallCounters(ctx context.Context, sid string) (json.RawMessage, error)
	// FirewallStatus reports fw4's loaded kernel state, independently of staged
	// configuration or whether its settings plugin is available.
	FirewallStatus(ctx context.Context, sid string) (FirewallStatus, error)
	// LogRead reads the tail of the device's log ring through logd's `log`
	// object, sid-gated like the other live reads. lines bounds the tail; the
	// records come back oldest-first, each carrying the monotonic id a reader
	// uses as its cursor. Packet events have their own NFLOG buffer.
	LogRead(ctx context.Context, sid string, lines int) ([]LogEntry, error)
	FirewallLogRead(ctx context.Context, sid, generation string, after int64, limit int) (FirewallLogBatch, error)
	// NetworkInterfaces reads netifd's logical interfaces and the kernel device
	// each one currently owns (`network.interface dump`), sid-gated. It is the
	// join a kernel device name needs to become something a person named: a log
	// line says eth0, the firewall config says "wan", and only netifd knows they
	// are the same thing right now.
	NetworkInterfaces(ctx context.Context, sid string) ([]NetIface, error)
	// The helper's raw state reads the shell brokers to a declaring plugin
	// (ADR-007), sid-gated: DHCP and DNS service state, fw4's rule files,
	// netifd's network state, what the radios are doing and what the tunnels
	// are doing. Each stays raw JSON; the plugin, not the shell, owns what it
	// means.
	DHCPState(ctx context.Context, sid string) (json.RawMessage, error)
	DNSState(ctx context.Context, sid string) (json.RawMessage, error)
	FirewallFiles(ctx context.Context, sid string) (json.RawMessage, error)
	NetworkState(ctx context.Context, sid string) (json.RawMessage, error)
	WirelessState(ctx context.Context, sid string) (json.RawMessage, error)
	VPNState(ctx context.Context, sid string) (json.RawMessage, error)
	OpenVPNFiles(ctx context.Context, sid string) (json.RawMessage, error)
	// StageConfigFile stages a hand-edited daemon file (dnsmasq, nftables)
	// through the helper, refusing it unless the file still holds expected.
	StageConfigFile(ctx context.Context, sid, path, expected, content string) error
	// NetworkSetUp brings one logical interface up or down; NetworkRestart
	// restarts it. Both ride netifd's network.interface object, sid-gated.
	NetworkSetUp(ctx context.Context, sid, name string, up bool) error
	NetworkRestart(ctx context.Context, sid, name string) error
	// AccessCredentials reads the SSH authorized keys and the web certificate;
	// SetAuthorizedKeys replaces the keys unless they changed from expected,
	// SetWebCertificate installs a certificate and its key, and
	// MakeWebCertificate has the router make a self-signed one for its own
	// names. All ride the helper.
	AccessCredentials(ctx context.Context, sid string) (AccessCredentials, error)
	SetAuthorizedKeys(ctx context.Context, sid, expected, keys string) error
	SetWebCertificate(ctx context.Context, sid, cert, key string) error
	MakeWebCertificate(ctx context.Context, sid string) error
	// SetWebOwner hands the router's web ports to Verso ("verso") or back to
	// LuCI's uhttpd ("luci"); the helper does it after answering, because it
	// restarts the shell that asked.
	SetWebOwner(ctx context.Context, sid, owner string) error
	// SetWebListeners writes verso.web's listener options and commits them on
	// their own, leaving whatever else the session stages: a nil list, or an
	// empty redirect, clears its option.
	SetWebListeners(ctx context.Context, sid string, https, http []string, redirect string) error
	// DiagStart starts a network diagnostic on the router and names the run;
	// DiagRead reads what it has written past a cursor, and DiagStop ends it.
	// A run is its operator's alone: the helper answers another session's read
	// or stop as if it did not exist.
	DiagStart(ctx context.Context, sid string, run DiagnosticRun) (string, error)
	DiagRead(ctx context.Context, sid, job string, after int) (DiagnosticOutput, error)
	DiagStop(ctx context.Context, sid, job string) error
}

// LogEntry is one record from the device's log ring. ID is logd's monotonic
// message counter — a reader remembers the highest one it has seen and takes
// only what is newer. Source distinguishes the kernel's own messages from
// userspace syslog; Time is milliseconds since the epoch, as logd stamps it.
type LogEntry struct {
	ID       int64
	Priority int64
	Source   int64
	Time     int64
	Msg      string
}

// NetIface is one netifd logical interface (the name a person and the firewall
// config use) and the kernel L3 device it currently owns (the name the kernel
// writes into a log line). The pair is the whole point: neither half alone can
// turn "IN=eth0" into "mgmt".
type NetIface struct {
	Name   string
	Device string
	Up     bool
}

// WANState is the set of live L3 devices that own an active default route.
type WANState struct {
	Devices []WANDevice
}

// WANDevice is one exact kernel L3 device participating in a reachable default
// route. Networks are its netifd logical owners; unmanaged devices have none.
type WANDevice struct {
	Device    string
	Transport string
	Networks  []string
	Routes    []WANRoute
	Uptime    int64
}

// WANRoute retains the routing facts that made a device a WAN candidate.
type WANRoute struct {
	Family int // 4 or 6
	Table  uint32
	Metric uint32
	Main   bool   // in the main table; a route elsewhere is a policy route
	Owner  string // logical netifd interface; empty for an unmanaged route
}

// Up reports whether at least one live default-route device exists.
func (s WANState) Up() bool { return len(s.Devices) != 0 }

// Primary selects one device for legacy single-uplink presentations such as the
// overview graph. Prefer main-table IPv4, then main-table IPv6, then policy-only
// candidates; discovery itself remains plural.
func (s WANState) Primary() (WANDevice, bool) {
	var best WANDevice
	bestRank, bestMetric := 4, ^uint32(0)
	found := false
	for _, device := range s.Devices {
		rank, metric := deviceRank(device)
		if rank == 3 {
			continue
		}
		if rank < bestRank || (rank == bestRank && (metric < bestMetric || (metric == bestMetric && device.Device < best.Device))) {
			best, bestRank, bestMetric, found = device, rank, metric, true
		}
	}
	return best, found
}

// WANConn is the preferred main-table uplinks' connection facts as the overview
// lists them. IPv4 and IPv6 may come from different logical interfaces.
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

// DeviceStats is one network device's byte counters, from network.device
// status.
type DeviceStats struct {
	RxBytes int64
	TxBytes int64
}

// Package is one row of a package search or listing, as the helper reports
// it; the detail fields are filled where the source provides them in bulk.
type Package struct {
	Name        string   `json:"name"`
	Version     string   `json:"version"`
	Feed        string   `json:"feed"`
	Description string   `json:"description"`
	License     string   `json:"license"`
	Webpage     string   `json:"webpage"`
	Size        int64    `json:"size"` // package file size, bytes
	Installed   bool     `json:"installed"`
	Services    []string `json:"services,omitempty"` // init scripts installed by this package
	RequiredBy  []string `json:"required_by,omitempty"`
	Removable   bool     `json:"removable"`
}

type PackagePage struct {
	Packages  []Package `json:"packages"`
	Total     int       `json:"total"`
	Count     int       `json:"count"`
	Installed int       `json:"installed"`
}

// PackageUpgrade is one installed package the feeds hold a newer copy of, named
// with both versions — what the device runs and what it would get.
type PackageUpgrade struct {
	Name      string `json:"name"`
	Installed string `json:"installed"`
	Available string `json:"available"`
}

// FirmwareUpdate is what the attended-sysupgrade server had to say about this
// device. State is the rung the check landed on: "update" and "current" are
// answers, while "no-owut" (the tool is not installed), "no-server" (nothing
// answered) and "unsupported" (the server cannot build this device) each name why
// there is none — Message carries the tool's own words for those. From and To are
// the release plus revision code; Packages is how many packages an upgrade would
// change; Server is the URL the check consulted.
type FirmwareUpdate struct {
	State    string `json:"state"`
	Server   string `json:"server"`
	From     string `json:"from"`
	To       string `json:"to"`
	Packages int    `json:"packages"`
	Message  string `json:"message"`
}

// The firmware-check rungs, mirroring verso-rpcd's vocabulary.
const (
	FirmwareUpdateAvailable = "update"
	FirmwareCurrent         = "current"
	FirmwareNoOwut          = "no-owut"
	FirmwareNoServer        = "no-server"
	FirmwareUnsupported     = "unsupported"
)

// ServiceKind describes an init script's lifecycle shape, not its current
// state. Daemons own a process, subsystems are procd-managed without a resident
// process, and startup tasks run to completion during boot.
type ServiceKind string

const (
	ServiceDaemon    ServiceKind = "daemon"
	ServiceSubsystem ServiceKind = "subsystem"
	ServiceTask      ServiceKind = "startup task"
)

// RCState is one procd service's assembled snapshot: rc supplies boot/running,
// service.list supplies instances and PIDs, and procfs supplies aggregate RSS
// plus the oldest current process's age. Runtime fields stay empty for
// process-less subsystems and completed startup tasks.
type RCState struct {
	Order       *int // init script START priority; nil when rc does not report it
	Enabled     bool
	Running     bool
	Kind        ServiceKind
	PIDs        []int
	MemoryBytes int64
	Uptime      int64
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
	Firmware    string
	Kernel      string
	KernelBuild string
	Target      string
	BoardName   string
	Model       string
}

// FirmwareInfo is the device-side verdict for one uploaded sysupgrade image.
// Invalid images are a normal result (Valid=false), not a transport failure.
type FirmwareInfo struct {
	Valid    bool   `json:"valid"`
	Version  string `json:"version"`
	Revision string `json:"revision"`
	Target   string `json:"target"`
	Board    string `json:"board"`
	Error    string `json:"error"`
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
	hostnameFn     func() (string, error)
	systemInfoFn   func(ctx context.Context, sid string) (map[string]any, error)
	systemBoardFn  func(ctx context.Context, sid string) (map[string]any, error)
	ipv6LeasesFn   func(ctx context.Context, sid string) (map[string]any, error)
	wanConnFn      func(ctx context.Context, sid string) (WANConn, error)
	accessFn       func(ctx context.Context, sid, scope, object, function string) (bool, error)
	uciSetFn       func(ctx context.Context, sid, config, section string, values map[string]any) error
	uciConfigFn    func(ctx context.Context, sid, config string) (map[string]any, error)
	uciAddFn       func(ctx context.Context, sid, config, secType, name string) (string, error)
	uciDeleteFn    func(ctx context.Context, sid, config, section, option string) error
	uciOrderFn     func(ctx context.Context, sid, config string, sections []string) error
	passwdFn       func(ctx context.Context, sid, username, password string) error
	setTimeFn      func(ctx context.Context, sid, datetime, timezone string) error
	rootPasswdFn   func(ctx context.Context, sid string) (bool, error)
	backupFn       func(ctx context.Context, sid, path string) error
	firmwareFn     func(ctx context.Context, sid, path string) (FirmwareInfo, error)
	maintenanceFn  func(ctx context.Context, sid string) error
	uciChangesFn   func(ctx context.Context, sid string) (map[string][][]string, error)
	uciRevertFn    func(ctx context.Context, sid, config string) error
	uciApplyFn     func(ctx context.Context, sid string, timeout int) error
	uciConfirmFn   func(ctx context.Context, sid string) error
	rcListFn       func(ctx context.Context, sid string) (map[string]RCState, error)
	rcInitFn       func(ctx context.Context, sid, name, action string) error
	pkgStatusFn    func(ctx context.Context, sid string) (int64, error)
	pkgUpdateFn    func(ctx context.Context, sid string) error
	pkgSearchFn    func(ctx context.Context, sid, query string) ([]Package, int, error)
	pkgBrowseFn    func(ctx context.Context, sid, query string, offset int) (PackagePage, error)
	pkgFilesFn     func(ctx context.Context, sid, name string) ([]string, error)
	pkgInstalledFn func(ctx context.Context, sid string) ([]Package, error)
	pkgActFn       func(ctx context.Context, sid, name string) error
	pkgUpgradesFn  func(ctx context.Context, sid string) ([]PackageUpgrade, error)
	pkgUpgradeFn   func(ctx context.Context, sid string) error
	firmwareUpdFn  func(ctx context.Context, sid string) (FirmwareUpdate, error)
	wanStatusFn    func(ctx context.Context, sid string) (WANState, error)
	deviceStatsFn  func(ctx context.Context, sid, device string) (DeviceStats, error)
	fwCountersFn   func(ctx context.Context, sid string) (json.RawMessage, error)
	logReadFn      func(ctx context.Context, sid string, lines int) (map[string]any, error)
	netIfacesFn    func(ctx context.Context, sid string) (map[string]any, error)
)

// NativeBackend reads OpenWrt state over the ubus socket, presenting the session
// sid to rpcd's ACL-gated objects. Verso holds no ambient privilege over config:
// rpcd authorizes and executes, so a restricted operator is limited to what
// their ACLs grant (ADR-007).
type NativeBackend struct {
	configFileCall configFileCall
	hostname       hostnameFn
	systemInfo     systemInfoFn
	systemBoard    systemBoardFn
	ipv6Leases     ipv6LeasesFn
	wanConn        wanConnFn
	access         accessFn
	uciSet         uciSetFn
	uciConfig      uciConfigFn
	uciAdd         uciAddFn
	uciDelete      uciDeleteFn
	uciOrder       uciOrderFn
	setPassword    passwdFn
	setTime        setTimeFn
	rootPasswd     rootPasswdFn
	createBackup   backupFn
	restoreBackup  backupFn
	firmwareCheck  firmwareFn
	firmwareFlash  backupFn
	restart        maintenanceFn
	factoryReset   maintenanceFn
	kernelBuild    func() string
	uciChanges     uciChangesFn
	uciRevert      uciRevertFn
	uciApply       uciApplyFn
	uciConfirm     uciConfirmFn
	rcList         rcListFn
	rcInit         rcInitFn
	pkgStatus      pkgStatusFn
	pkgUpdate      pkgUpdateFn
	pkgSearch      pkgSearchFn
	pkgBrowse      pkgBrowseFn
	pkgFiles       pkgFilesFn
	pkgUpgradeOne  pkgActFn
	pkgInstalled   pkgInstalledFn
	pkgInstall     pkgActFn
	pkgRemove      pkgActFn
	pkgUpgrades    pkgUpgradesFn
	pkgUpgrade     pkgUpgradeFn
	firmwareUpd    firmwareUpdFn
	firmwareUpg    maintenanceFn
	wanStatus      wanStatusFn
	deviceStats    deviceStatsFn
	fwCounters     fwCountersFn
	fwStatus       firewallStatusFn
	firewallLog    firewallLogFn
	logRead        logReadFn
	netIfaces      netIfacesFn
	diagStart      diagStartFn
	diagRead       diagReadFn
	diagStop       diagStopFn
}

// NewNativeBackend returns a backend using the default ubus socket.
func NewNativeBackend() *NativeBackend {
	return &NativeBackend{
		hostname:      os.Hostname,
		systemInfo:    dialSystemInfo(""),
		systemBoard:   dialSystemBoard(""),
		ipv6Leases:    dialIPv6Leases(""),
		wanConn:       dialWANConn(""),
		access:        dialAccess(""),
		uciSet:        dialUCISet(""),
		uciConfig:     dialUCIConfig(""),
		uciAdd:        dialUCIAdd(""),
		uciDelete:     dialUCIDelete(""),
		uciOrder:      dialUCIOrder(""),
		setPassword:   dialSetPassword(""),
		setTime:       dialSetSystemTime(""),
		rootPasswd:    dialRootHasPassword(""),
		createBackup:  dialBackupAct("", "createBackup"),
		restoreBackup: dialBackupAct("", "restoreBackup"),
		firmwareCheck: dialFirmwareValidate(""),
		firmwareFlash: dialBackupAct("", "installFirmware"),
		restart:       dialMaintenanceAct("", "restart"),
		factoryReset:  dialMaintenanceAct("", "factoryReset"),
		kernelBuild: func() string {
			data, err := os.ReadFile("/proc/version")
			if err != nil {
				return ""
			}
			return strings.TrimSpace(string(data))
		},
		uciChanges:     dialUCIChanges(""),
		configFileCall: nativeConfigFileCall,
		uciRevert:      dialUCIRevert(""),
		uciApply:       dialUCIApply(""),
		uciConfirm:     dialUCIConfirm(""),
		rcList:         dialRCList(""),
		rcInit:         dialRCInit(""),
		pkgStatus:      dialPkgStatus(""),
		pkgUpdate:      dialPkgUpdate(""),
		pkgSearch:      dialPkgSearch(""),
		pkgBrowse:      dialPkgBrowse(""),
		pkgFiles:       dialPkgFiles(""),
		pkgUpgradeOne:  dialPkgAct("", "pkgUpgradeOne"),
		pkgInstalled:   dialPkgInstalled(""),
		pkgInstall:     dialPkgAct("", "pkgInstall"),
		pkgRemove:      dialPkgAct("", "pkgRemove"),
		pkgUpgrades:    dialPkgUpgradable(""),
		pkgUpgrade:     dialPkgUpgrade(""),
		firmwareUpd:    dialFirmwareCheck(""),
		firmwareUpg:    dialFirmwareUpgrade(""),
		wanStatus:      dialWANStatus(""),
		deviceStats:    dialDeviceStats(""),
		fwCounters:     dialFirewallCounters(""),
		fwStatus:       dialFirewallStatus(""),
		firewallLog:    dialFirewallLog(""),
		logRead:        dialLogRead(""),
		netIfaces:      dialNetworkInterfaces(""),
		diagStart:      dialDiagStart(""),
		diagRead:       dialDiagRead(""),
		diagStop:       dialDiagStop(""),
	}
}

// Hostname reads the running name, just like the public sign-in page and
// OpenWrt's system.board response. This kernel fact needs no rpcd session.
func (b *NativeBackend) Hostname(_ context.Context, _ string) (string, error) {
	return b.hostname()
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
	board := parseBoard(m)
	if board.KernelBuild == "" && board.Kernel != "" {
		board.KernelBuild = board.Kernel
	}
	if b.kernelBuild != nil {
		if full := b.kernelBuild(); full != "" {
			board.KernelBuild = condenseKernelBuild(full, board.Kernel)
		}
	}
	return board, nil
}

// condenseKernelBuild reads /proc/version down to what identifies the kernel: the
// release and the build stamp. The full line carries the builder's address and
// the whole compiler pedigree between them — provenance for a bug report, noise
// on a facts row. A line that is not that shape falls back to the short release
// ubus already stated, never to the unparsed line.
//
//	Linux version 6.12.101+deb13-amd64 (debian-kernel@…) (gcc … (…) …, GNU ld …) #1 SMP PREEMPT_DYNAMIC Debian 6.12.101-1 (2026-08-05)
//	→ 6.12.101+deb13-amd64 #1 SMP PREEMPT_DYNAMIC Debian 6.12.101-1 (2026-08-05)
func condenseKernelBuild(full, release string) string {
	fields := strings.Fields(full)
	if len(fields) < 3 || fields[0] != "Linux" || fields[1] != "version" {
		return release
	}
	out := fields[2]
	// The build stamp is everything from the "#N" word on; the parenthesised
	// groups before it (builder, compiler — with nested parens of their own)
	// are what this row does without.
	if i := strings.Index(full, " #"); i >= 0 {
		out += " " + strings.TrimSpace(full[i+1:])
	}
	return out
}

// Access reports whether rpcd grants the session object.function in scope.
func (b *NativeBackend) Access(ctx context.Context, sid, scope, object, function string) (bool, error) {
	return b.access(ctx, sid, scope, object, function)
}

// UCISet writes option values into a uci section through rpcd, gated by the sid.
func (b *NativeBackend) UCISet(ctx context.Context, sid, config, section string, values map[string]any) error {
	return b.uciSet(ctx, sid, config, section, values)
}

// UCIConfig reads a whole uci config through rpcd, gated by the sid.
func (b *NativeBackend) UCIConfig(ctx context.Context, sid, config string) (map[string]any, error) {
	return b.uciConfig(ctx, sid, config)
}

// UCIAdd creates a new section of secType through rpcd, gated by the sid, and
// returns its id — anonymous when name is empty, named otherwise.
func (b *NativeBackend) UCIAdd(ctx context.Context, sid, config, secType, name string) (string, error) {
	return b.uciAdd(ctx, sid, config, secType, name)
}

// UCIDelete removes a section — or one of its options — through rpcd, gated by
// the sid. An option rpcd cannot find is reported as ErrOptionNotFound: clearing
// an option a section never carried is the state the caller asked for, and the
// ubus status that says so stops here rather than travelling on as a bare
// failure. A section-level delete is passed through as it comes.
//
// Only the *invoke* answers that question. The `uci` object lookup that precedes
// every call answers a missing object with the same UBUS_STATUS_NOT_FOUND, and
// rpcd off the bus is the opposite of "already in the state you asked for" —
// nothing was read, nothing was written, and a save that swallowed it would
// report success over a device it never reached.
func (b *NativeBackend) UCIDelete(ctx context.Context, sid, config, section, option string) error {
	err := b.uciDelete(ctx, sid, config, section, option)
	if err == nil || option == "" {
		return err
	}
	var status *ubus.StatusError
	if errors.As(err, &status) && status.Code == ubus.StatusNotFound && status.Phase == ubus.PhaseInvoke {
		return fmt.Errorf("openwrt: clear %s.%s.%s: %w", config, section, option, ErrOptionNotFound)
	}
	return err
}

// UCIOrder rewrites a config's section sequence through rpcd, gated by the sid.
func (b *NativeBackend) UCIOrder(ctx context.Context, sid, config string, sections []string) error {
	return b.uciOrder(ctx, sid, config, sections)
}

// SetPassword sets username's system password through the privileged `verso-rpcd`
// helper (the setPassword verb), gated by the sid.
func (b *NativeBackend) SetPassword(ctx context.Context, sid, username, password string) error {
	return b.setPassword(ctx, sid, username, password)
}

// SetSystemTime updates the kernel clock through the helper's setSystemTime
// verb, independently authorized against the operator's session.
func (b *NativeBackend) SetSystemTime(ctx context.Context, sid, datetime, timezone string) error {
	return b.setTime(ctx, sid, datetime, timezone)
}

// RootHasPassword reports whether root has a system password, read from
// /etc/shadow by the privileged `verso-rpcd` helper (the shell cannot read it
// itself), gated by the sid.
func (b *NativeBackend) RootHasPassword(ctx context.Context, sid string) (bool, error) {
	return b.rootPasswd(ctx, sid)
}

func (b *NativeBackend) CreateBackup(ctx context.Context, sid, path string) error {
	return b.createBackup(ctx, sid, path)
}

func (b *NativeBackend) RestoreBackup(ctx context.Context, sid, path string) error {
	return b.restoreBackup(ctx, sid, path)
}

func (b *NativeBackend) ValidateFirmware(ctx context.Context, sid, path string) (FirmwareInfo, error) {
	return b.firmwareCheck(ctx, sid, path)
}

func (b *NativeBackend) InstallFirmware(ctx context.Context, sid, path string) error {
	return b.firmwareFlash(ctx, sid, path)
}

func (b *NativeBackend) Restart(ctx context.Context, sid string) error {
	return b.restart(ctx, sid)
}

func (b *NativeBackend) FactoryReset(ctx context.Context, sid string) error {
	return b.factoryReset(ctx, sid)
}

// UCIChanges reads the pending uci changes across all configs through rpcd, gated
// by the sid.
func (b *NativeBackend) UCIChanges(ctx context.Context, sid string) (map[string][][]string, error) {
	changes, err := b.uciChanges(ctx, sid)
	if err != nil {
		return nil, err
	}
	files, err := b.configFileState(ctx, sid)
	if err != nil {
		return nil, err
	}
	if changes == nil {
		changes = make(map[string][][]string)
	}
	for _, f := range files.Files {
		config, ok := fileFamilies[f.Family]
		if f.Pending && ok {
			changes[config] = append(changes[config], []string{"file", f.Path, f.Content})
		}
	}
	return changes, nil
}

// UCIRevert discards a config's staged changes through rpcd, gated by the sid,
// and the staged files of the family that config's daemon reads.
func (b *NativeBackend) UCIRevert(ctx context.Context, sid, config string) error {
	if err := b.uciRevert(ctx, sid, config); err != nil {
		return err
	}
	if family, ok := familyOf(config); ok {
		return b.discardFiles(ctx, sid, family)
	}
	return nil
}

// UCIApply commits all dirty configs with rpcd's rollback armed, gated by the sid.
func (b *NativeBackend) UCIApply(ctx context.Context, sid string, timeout int) error {
	return b.applyWithFiles(ctx, sid, timeout)
}

// UCIConfirm disarms a pending rollback through rpcd, gated by the sid.
func (b *NativeBackend) UCIConfirm(ctx context.Context, sid string) error {
	return b.confirmWithFiles(ctx, sid)
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

func (b *NativeBackend) PkgBrowse(ctx context.Context, sid, query string, offset int) (PackagePage, error) {
	return b.pkgBrowse(ctx, sid, query, offset)
}

func (b *NativeBackend) PkgFiles(ctx context.Context, sid, name string) ([]string, error) {
	return b.pkgFiles(ctx, sid, name)
}

func (b *NativeBackend) PkgUpgradeOne(ctx context.Context, sid, name string) error {
	return b.pkgUpgradeOne(ctx, sid, name)
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

func (b *NativeBackend) PkgUpgradable(ctx context.Context, sid string) ([]PackageUpgrade, error) {
	return b.pkgUpgrades(ctx, sid)
}

func (b *NativeBackend) PkgUpgrade(ctx context.Context, sid string) error {
	return b.pkgUpgrade(ctx, sid)
}

// FirmwareCheck asks the privileged helper to run owut's check against the
// device's configured attended-sysupgrade server, gated by the sid.
func (b *NativeBackend) FirmwareCheck(ctx context.Context, sid string) (FirmwareUpdate, error) {
	return b.firmwareUpd(ctx, sid)
}

// FirmwareUpgrade asks the privileged helper to run owut's download and start
// its install, gated by the sid.
func (b *NativeBackend) FirmwareUpgrade(ctx context.Context, sid string) error {
	return b.firmwareUpg(ctx, sid)
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

// FirewallCounters reads fw4's per-rule nftables counters through the privileged
// `verso-rpcd` helper, gated by the sid.
func (b *NativeBackend) FirewallCounters(ctx context.Context, sid string) (json.RawMessage, error) {
	return b.fwCounters(ctx, sid)
}

// LogRead reads the tail of logd's ring and folds it to typed records.
func (b *NativeBackend) LogRead(ctx context.Context, sid string, lines int) ([]LogEntry, error) {
	m, err := b.logRead(ctx, sid, lines)
	if err != nil {
		return nil, err
	}
	return parseLogEntries(m), nil
}

// NetworkInterfaces reads netifd's dump and folds it to the logical-name →
// kernel-device pairs a log line has to be read through.
func (b *NativeBackend) NetworkInterfaces(ctx context.Context, sid string) ([]NetIface, error) {
	m, err := b.netIfaces(ctx, sid)
	if err != nil {
		return nil, err
	}
	return parseNetIfaces(m), nil
}

// parseLogEntries folds logd's `{"log": [...]}` reply into typed records,
// keeping only entries that carry a message — a malformed element costs its own
// line, never the read.
func parseLogEntries(m map[string]any) []LogEntry {
	list, _ := m["log"].([]any)
	out := make([]LogEntry, 0, len(list))
	for _, e := range list {
		rec, ok := e.(map[string]any)
		if !ok {
			continue
		}
		msg, _ := rec["msg"].(string)
		if msg == "" {
			continue
		}
		out = append(out, LogEntry{
			ID:       asInt64(rec["id"]),
			Priority: asInt64(rec["priority"]),
			Source:   asInt64(rec["source"]),
			Time:     asInt64(rec["time"]),
			Msg:      msg,
		})
	}
	return out
}

// parseNetIfaces folds `network.interface dump` into the logical/device pairs.
// The L3 device is what the kernel writes into a log line, so it wins over the
// configured device when netifd reports both (a protocol whose L3 device is a
// tunnel above its physical carrier).
func parseNetIfaces(dump map[string]any) []NetIface {
	entries, _ := dump["interface"].([]any)
	out := make([]NetIface, 0, len(entries))
	for _, e := range entries {
		rec, ok := e.(map[string]any)
		if !ok {
			continue
		}
		name, _ := rec["interface"].(string)
		if name == "" {
			continue
		}
		device, _ := rec["l3_device"].(string)
		if device == "" {
			device, _ = rec["device"].(string)
		}
		up := false
		switch v := rec["up"].(type) {
		case bool:
			up = v
		case int64:
			up = v != 0
		}
		out = append(out, NetIface{Name: name, Device: device, Up: up})
	}
	return out
}

// dialLogRead returns a logReadFn that reads the tail of logd's ring through the
// `log` object, sid-gated by the same session.access pre-check every non-rpcd
// ubus call uses. stream:false is what makes this a one-shot tail: logd's
// default is a subscription that never returns.
func dialLogRead(socket string) logReadFn {
	return func(_ context.Context, sid string, lines int) (map[string]any, error) {
		return callChecked(socket, sid, "log", "read", map[string]any{
			"lines":   lines,
			"stream":  false,
			"oneshot": true,
		})
	}
}

// dialNetworkInterfaces returns a netIfacesFn reading the same netifd dump the
// uplink discovery reads — one shape, one ACL, one place it is fetched.
func dialNetworkInterfaces(socket string) netIfacesFn {
	return func(_ context.Context, sid string) (map[string]any, error) {
		return fetchNetworkDump(socket, sid)
	}
}

// dialSystemInfo probes rpcd `session.access` for the sid, then invokes
// `system info` directly. system is procd's object (rpcd does not proxy it), so
// the ACL is enforced by the pre-check — the pattern for any non-rpcd ubus call.
func dialSystemInfo(socket string) systemInfoFn {
	return func(_ context.Context, sid string) (map[string]any, error) {
		return callChecked(socket, sid, "system", "info", nil)
	}
}

// dialSystemBoard returns a systemBoardFn that reads `ubus call system board`,
// the device's identity (firmware release, kernel), sid-gated like system info.
func dialSystemBoard(socket string) systemBoardFn {
	return func(_ context.Context, sid string) (map[string]any, error) {
		return callChecked(socket, sid, "system", "board", nil)
	}
}

// dialIPv6Leases returns an ipv6LeasesFn that reads `ubus call dhcp ipv6leases`
// (odhcpd's DHCPv6 lease table), sid-gated like the other reads.
func dialIPv6Leases(socket string) ipv6LeasesFn {
	return func(_ context.Context, sid string) (map[string]any, error) {
		return callChecked(socket, sid, "dhcp", "ipv6leases", nil)
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
		_, err := call(socket, "uci", "set", map[string]any{
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
		res, err := call(socket, "uci", "get", map[string]any{
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

// dialUCIAdd returns a uciAddFn that creates a section of secType via rpcd's
// `uci` object (method `add`), carrying the sid, and returns rpcd's new section
// id. The optional `name` argument is what makes the section named rather than
// anonymous; it is omitted entirely when empty, since an empty name is not a
// name rpcd should try to give a section. The caller commits the config
// afterwards, as with `set`.
func dialUCIAdd(socket string) uciAddFn {
	return func(_ context.Context, sid, config, secType, name string) (string, error) {
		args := map[string]any{
			"ubus_rpc_session": sid,
			"config":           config,
			"type":             secType,
		}
		if name != "" {
			args["name"] = name
		}
		res, err := call(socket, "uci", "add", args)
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

// dialSetSystemTime returns the narrow helper client for setting the kernel
// clock. The helper validates both fields again and invokes no shell.
func dialSetSystemTime(socket string) setTimeFn {
	return func(ctx context.Context, sid, datetime, timezone string) error {
		return callHelper(ctx, socket, "setSystemTime", sid, map[string]string{
			"datetime": datetime,
			"timezone": timezone,
		}, nil)
	}
}

// dialRootHasPassword returns a rootPasswdFn that asks the resident Rust helper
// whether root has a password (the rootHasPassword verb). The helper reads
// /etc/shadow as root and self-gates on session.access, so the shell needs no
// shadow-read capability of its own (ADR-007); only the boolean crosses the
// socket, never the hash.
func dialRootHasPassword(socket string) rootPasswdFn {
	return func(ctx context.Context, sid string) (bool, error) {
		var result struct {
			HasPassword bool `json:"has_password"`
		}
		if err := callHelper(ctx, socket, "rootHasPassword", sid, nil, &result); err != nil {
			return false, err
		}
		return result.HasPassword, nil
	}
}

// dialFirewallCounters returns a fwCountersFn that asks the resident Rust helper
// for fw4's per-rule nftables counters (the firewallCounters verb). Listing an
// nftables table needs CAP_NET_ADMIN, which the shell does not hold (ADR-007), so
// the helper lists it as root and self-gates on session.access. The result crosses
// as raw JSON — the shell forwards it to the plugin that declared the read without
// reading it itself.
func dialFirewallCounters(socket string) fwCountersFn {
	return func(ctx context.Context, sid string) (json.RawMessage, error) {
		var result json.RawMessage
		if err := callHelper(ctx, socket, "firewallCounters", sid, nil, &result); err != nil {
			return nil, err
		}
		return result, nil
	}
}

// dialBackupAct brokers OpenWrt-native backup operations without carrying the
// archive through JSON. The helper accepts only Verso's private temporary-file
// naming convention and invokes sysupgrade directly, never through a shell.
func dialBackupAct(socket, method string) backupFn {
	return func(ctx context.Context, sid, path string) error {
		return callHelper(ctx, socket, method, sid, map[string]string{"path": path}, nil)
	}
}

func dialFirmwareValidate(socket string) firmwareFn {
	return func(ctx context.Context, sid, path string) (FirmwareInfo, error) {
		var result FirmwareInfo
		err := callHelper(ctx, socket, "validateFirmware", sid, map[string]string{"path": path}, &result)
		return result, err
	}
}

func dialMaintenanceAct(socket, method string) maintenanceFn {
	return func(ctx context.Context, sid string) error {
		return callHelper(ctx, socket, method, sid, nil, nil)
	}
}

// dialUCIDelete returns a uciDeleteFn that removes a section via rpcd's `uci`
// object (method `delete`), carrying the sid. Naming an option narrows the
// delete to that option, leaving the section itself; omitting it removes the
// whole section. The caller commits afterwards.
func dialUCIDelete(socket string) uciDeleteFn {
	return func(_ context.Context, sid, config, section, option string) error {
		args := map[string]any{
			"ubus_rpc_session": sid,
			"config":           config,
			"section":          section,
		}
		if option != "" {
			args["option"] = option
		}
		_, err := call(socket, "uci", "delete", args)
		return err
	}
}

// dialUCIOrder returns a uciOrderFn that rewrites a config's section sequence
// via rpcd's `uci` object (method `order`), carrying the sid. sections is the
// whole new order; rpcd stages the move exactly as it stages a set or a delete,
// so the caller's apply is what makes it live.
func dialUCIOrder(socket string) uciOrderFn {
	return func(_ context.Context, sid, config string, sections []string) error {
		_, err := call(socket, "uci", "order", map[string]any{
			"ubus_rpc_session": sid,
			"config":           config,
			"sections":         sections,
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
		res, err := call(socket, "uci", "changes", map[string]any{
			"ubus_rpc_session": sid,
		})
		if err != nil {
			return nil, err
		}
		return parseChanges(res["changes"]), nil
	}
}

// parseChanges maps rpcd's generic changes table onto config → change tuples.
// A tuple element may be a number — an order change carries the section's new
// position (["order","cfg0792bd",5]) — so numeric fields render as their
// decimal string rather than sinking the whole tuple; a staged reorder the
// chip cannot count is one the drawer can neither review nor discard.
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
				switch field := f.(type) {
				case string:
					change = append(change, field)
				case int64:
					change = append(change, strconv.FormatInt(field, 10))
				case float64:
					change = append(change, strconv.FormatFloat(field, 'f', -1, 64))
				default:
					change = nil
				}
				if change == nil {
					break
				}
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
		_, err := call(socket, "uci", "revert", map[string]any{
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
		_, err := call(socket, "uci", "apply", map[string]any{
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
		_, err := call(socket, "uci", "confirm", map[string]any{
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
	res, err := c.InvokeTable(id, "access", map[string]any{
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

// call runs one method on a ubus object over a connection of its own — the
// shape of every single-shot read and write. A nil args table sends the same
// empty table a no-argument Invoke does.
func call(socket, object, method string, args map[string]any) (map[string]any, error) {
	c, err := ubus.Dial(socket)
	if err != nil {
		return nil, err
	}
	defer c.Close()
	return invoke(c, object, method, args)
}

// callChecked is call behind the session.access pre-check for ubus
// object.method — the pattern for any object rpcd does not proxy itself. A
// denial and an unreachable rpcd both fail closed as ErrAccessDenied.
func callChecked(socket, sid, object, method string, args map[string]any) (map[string]any, error) {
	c, err := ubus.Dial(socket)
	if err != nil {
		return nil, err
	}
	defer c.Close()
	if ok, err := probeAccess(c, sid, "ubus", object, method); err != nil || !ok {
		return nil, ErrAccessDenied
	}
	return invoke(c, object, method, args)
}

func invoke(c *ubus.Client, object, method string, args map[string]any) (map[string]any, error) {
	id, err := c.Lookup(object)
	if err != nil {
		return nil, err
	}
	return c.InvokeTable(id, method, args)
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

// parseBoard keeps OpenWrt's complete release description (including revision)
// and target. Short distribution/version fields are only a compatibility
// fallback for older board responses.
func parseBoard(m map[string]any) Board {
	str := func(v any) string { s, _ := v.(string); return s }
	var b Board
	b.BoardName = str(m["board_name"])
	b.Model = str(m["model"])
	if k := str(m["kernel"]); k != "" {
		b.Kernel = "Linux " + k
	}
	if rel, ok := m["release"].(map[string]any); ok {
		b.Target = str(rel["target"])
		b.Firmware = str(rel["description"])
		dist, ver := str(rel["distribution"]), str(rel["version"])
		switch {
		case b.Firmware != "":
		case dist != "" && ver != "":
			b.Firmware = dist + " " + ver
		case dist != "":
			b.Firmware = dist
		default:
			b.Firmware = ver
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

type serviceInstance struct {
	PID         int
	Running     bool
	HasCommand  bool
	Respawn     bool
	HasExitCode bool
	ExitCode    int64
}

type serviceDetail struct {
	Instances []serviceInstance
}

// dialRCList reads the cheap rc inventory first, then enriches it from procd's
// service instances and procfs. The richer read is best-effort: an older ACL or
// procd without service.list still gets the complete, controllable rc table.
//
// rc.list is asked to skip its running checks: rpcd answers each one by
// executing `/etc/init.d/<name> running`, one script after another, which costs
// seconds on a router with fifty procd services — and some packages' scripts
// reload their daemon from that hook. procd's own instance table, read next,
// already says what runs.
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
		res, err := c.InvokeTable(id, "list", map[string]any{"skip_running_check": true})
		if err != nil {
			return nil, err
		}
		out := make(map[string]RCState, len(res))
		for name, v := range res {
			t, ok := v.(map[string]any)
			if !ok {
				continue
			}
			st := RCState{Enabled: asBool(t["enabled"])}
			if value, ok := t["start"]; ok {
				order := int(asInt64(value))
				st.Order = &order
			}
			out[name] = st
		}

		details := map[string]serviceDetail{}
		if ok, _ := probeAccess(c, sid, "ubus", "service", "list"); ok {
			if serviceID, lookupErr := c.Lookup("service"); lookupErr == nil {
				if serviceRes, invokeErr := c.Invoke(serviceID, "list"); invokeErr == nil {
					details = parseServiceDetails(serviceRes)
				}
			}
		}
		enrichRCStates(out, details, os.ReadFile)
		return out, nil
	}
}

func parseServiceDetails(raw map[string]any) map[string]serviceDetail {
	out := make(map[string]serviceDetail, len(raw))
	for name, value := range raw {
		detail := serviceDetail{}
		service, ok := value.(map[string]any)
		if !ok {
			out[name] = detail
			continue
		}
		instances, _ := service["instances"].(map[string]any)
		for _, value := range instances {
			instance, ok := value.(map[string]any)
			if !ok {
				continue
			}
			pid := int(asInt64(instance["pid"]))
			_, hasCommand := instance["command"]
			_, respawn := instance["respawn"]
			exitCode, hasExitCode := instance["exit_code"]
			detail.Instances = append(detail.Instances, serviceInstance{
				PID: pid, Running: asBool(instance["running"]), HasCommand: hasCommand,
				Respawn: respawn, HasExitCode: hasExitCode, ExitCode: asInt64(exitCode),
			})
		}
		out[name] = detail
	}
	return out
}

func enrichRCStates(
	states map[string]RCState,
	details map[string]serviceDetail,
	readFile func(string) ([]byte, error),
) {
	systemUptime := readProcUptime(readFile)
	for name, state := range states {
		detail := details[name]
		var script []byte
		if filepath.Base(name) == name && !strings.ContainsAny(name, `/\\`) {
			script, _ = readFile(filepath.Join("/etc/init.d", name))
		}
		state.Kind = classifyService(usesProcd(script), script, detail)
		for _, instance := range detail.Instances {
			// rc's own check asks the same table: running is any live instance.
			state.Running = state.Running || instance.Running
			if instance.Running && instance.PID > 0 {
				state.PIDs = append(state.PIDs, instance.PID)
			}
		}
		sort.Ints(state.PIDs)
		state.MemoryBytes, state.Uptime = readProcessRuntime(state.PIDs, systemUptime, readFile)
		states[name] = state
	}
}

// usesProcd reports whether an init script declares USE_PROCD with a non-zero
// value at the start of a line — the declaration rpcd's rc reads to decide the
// same.
func usesProcd(script []byte) bool {
	for _, line := range bytes.Split(script, []byte("\n")) {
		if value, ok := bytes.CutPrefix(line, []byte("USE_PROCD=")); ok {
			n, err := strconv.ParseUint(string(bytes.TrimSpace(value)), 0, 64)
			return err == nil && n != 0
		}
	}
	return false
}

func classifyService(managed bool, script []byte, detail serviceDetail) ServiceKind {
	daemonScript := bytes.Contains(script, []byte("procd_set_param command"))
	// A script procd does not run is a boot task.
	if !managed {
		return ServiceTask
	}
	// A procd one-shot leaves a successful, non-respawning instance behind after
	// it completes (urandom_seed is the stock example). That is a task even though
	// its script uses the procd instance API — so this check must precede the
	// daemon-script check below. Accepted edge: a daemon that clean-exits without
	// respawn is indistinguishable from a one-shot and reads as a task; procd drops
	// a daemon's instance on stop and respawns a crashed one, so it does not arise.
	if len(detail.Instances) > 0 {
		completed := true
		for _, instance := range detail.Instances {
			if instance.Running || instance.Respawn || !instance.HasExitCode || instance.ExitCode != 0 {
				completed = false
				break
			}
		}
		if completed {
			return ServiceTask
		}
	}
	if daemonScript {
		return ServiceDaemon
	}
	for _, instance := range detail.Instances {
		if instance.HasCommand {
			return ServiceDaemon
		}
	}
	return ServiceSubsystem
}

func readProcUptime(readFile func(string) ([]byte, error)) float64 {
	data, err := readFile("/proc/uptime")
	if err != nil {
		return 0
	}
	fields := strings.Fields(string(data))
	if len(fields) == 0 {
		return 0
	}
	uptime, _ := strconv.ParseFloat(fields[0], 64)
	return uptime
}

func readProcessRuntime(pids []int, systemUptime float64, readFile func(string) ([]byte, error)) (memory, uptime int64) {
	for _, pid := range pids {
		base := filepath.Join("/proc", strconv.Itoa(pid))
		if status, err := readFile(filepath.Join(base, "status")); err == nil {
			memory += parseProcRSS(status)
		}
		if stat, err := readFile(filepath.Join(base, "stat")); err == nil {
			if age := parseProcAge(stat, systemUptime); age > uptime {
				uptime = age
			}
		}
	}
	return memory, uptime
}

func parseProcRSS(status []byte) int64 {
	for _, line := range strings.Split(string(status), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == "VmRSS:" {
			kib, _ := strconv.ParseInt(fields[1], 10, 64)
			return kib * 1024
		}
	}
	return 0
}

func parseProcAge(stat []byte, systemUptime float64) int64 {
	// comm (field 2) may contain spaces and parentheses. Everything after its
	// final ')' begins at state (field 3); starttime is therefore tail index 19.
	end := bytes.LastIndexByte(stat, ')')
	if end < 0 {
		return 0
	}
	fields := strings.Fields(string(stat[end+1:]))
	if len(fields) <= 19 {
		return 0
	}
	startTicks, err := strconv.ParseUint(fields[19], 10, 64)
	if err != nil || systemUptime <= 0 {
		return 0
	}
	// Linux procfs exposes process starttime in USER_HZ, fixed at 100 for the
	// OpenWrt Linux targets Verso supports (independent of CONFIG_HZ).
	age := systemUptime - float64(startTicks)/100
	if age < 0 {
		return 0
	}
	return int64(age)
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

func dialPkgBrowse(socket string) pkgBrowseFn {
	return func(ctx context.Context, sid, query string, offset int) (PackagePage, error) {
		var result PackagePage
		err := callHelper(ctx, socket, "pkgBrowse", sid, map[string]string{"query": query, "offset": strconv.Itoa(offset)}, &result)
		return result, err
	}
}

func dialPkgFiles(socket string) pkgFilesFn {
	return func(ctx context.Context, sid, name string) ([]string, error) {
		var result struct {
			Files []string `json:"files"`
		}
		err := callHelper(ctx, socket, "pkgFiles", sid, map[string]string{"package": name}, &result)
		return result.Files, err
	}
}

func dialPkgAct(socket, methodName string) pkgActFn {
	return func(ctx context.Context, sid, name string) error {
		return callHelper(ctx, socket, methodName, sid, map[string]string{"package": name}, nil)
	}
}

func dialPkgUpgradable(socket string) pkgUpgradesFn {
	return func(ctx context.Context, sid string) ([]PackageUpgrade, error) {
		var result struct {
			Packages []PackageUpgrade `json:"packages"`
		}
		err := callHelper(ctx, socket, "pkgUpgradable", sid, nil, &result)
		return result.Packages, err
	}
}

func dialPkgUpgrade(socket string) pkgUpgradeFn {
	return func(ctx context.Context, sid string) error {
		return callHelper(ctx, socket, "pkgUpgrade", sid, nil, nil)
	}
}

// dialFirmwareCheck asks the helper to run owut's check. The helper answers with a
// rung rather than an error whenever the check itself cannot conclude, so an error
// here means the helper was unreachable, not that the device has no answer.
func dialFirmwareCheck(socket string) firmwareUpdFn {
	return func(ctx context.Context, sid string) (FirmwareUpdate, error) {
		var result FirmwareUpdate
		err := callHelper(ctx, socket, "firmwareCheck", sid, nil, &result)
		return result, err
	}
}

// dialFirmwareUpgrade asks the helper to run the upgrade. The request carries no
// arguments — the whole point of the verb is that owut decides everything on the
// device — and the wait is the long one, because the build happens on the update
// server before this router has anything to download.
func dialFirmwareUpgrade(socket string) maintenanceFn {
	return func(ctx context.Context, sid string) error {
		return callHelperWithin(ctx, socket, "firmwareUpgrade", sid, nil, nil, firmwareUpgradeTimeout)
	}
}

// dialRCInit drives one rc action on a named init script through procd, after
// probing the session's ubus/rc/init access. Name and action policy is the
// caller's (the shell restricts both); rpcd's session gate is the backstop.
func dialRCInit(socket string) rcInitFn {
	return func(_ context.Context, sid, name, action string) error {
		_, err := callChecked(socket, sid, "rc", "init", map[string]any{"name": name, "action": action})
		return err
	}
}

// dialWANStatus reads one plural netifd dump and supplements it with the kernel
// route/rule view. Kernel discovery is best-effort: stock netifd discovery still
// works if the platform refuses a route-netlink dump.
func dialWANStatus(socket string) wanStatusFn {
	return func(_ context.Context, sid string) (WANState, error) {
		dump, err := fetchNetworkDump(socket, sid)
		if err != nil {
			return WANState{}, err
		}
		routes, _ := kernelDefaultRoutes()
		return discoverWAN(dump, routes), nil
	}
}

// dialWANConn reads the same plural netifd dump and chooses the preferred
// main-table route owner independently for IPv4 and IPv6.
func dialWANConn(socket string) wanConnFn {
	return func(_ context.Context, sid string) (WANConn, error) {
		dump, err := fetchNetworkDump(socket, sid)
		if err != nil {
			return WANConn{}, err
		}
		wan, wan6 := wanStatuses(dump)
		return parseWANConn(wan, wan6), nil
	}
}

func fetchNetworkDump(socket, sid string) (map[string]any, error) {
	return callChecked(socket, sid, "network.interface", "dump", nil)
}

// wanStatuses returns the preferred active main-table default-route owner for
// each family. Names are used only as a deterministic final tie-breaker.
func wanStatuses(dump map[string]any) (map[string]any, map[string]any) {
	entries, _ := dump["interface"].([]any)
	return preferredStatus(entries, 4), preferredStatus(entries, 6)
}

// parseWANConn maps the selected IPv4/IPv6 interface statuses onto WANConn: the first
// address (with its mask), the default route's nexthop as the gateway, the DNS
// list, the delegated IPv6 prefix, and the prefix's valid lifetime as the lease
// time left. Some netifd protocols expose both families on one logical owner,
// so the IPv4-selected status is also the fallback IPv6 source.
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
			if ok && str(e["target"]) == wantTarget && asInt64(e["mask"]) == 0 && routeTable(e["table"]) == mainRouteTable {
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
		res, err := callChecked(socket, sid, "network.device", "status", map[string]any{"name": device})
		if err != nil {
			return DeviceStats{}, err
		}
		return parseDeviceStats(res), nil
	}
}

// parseDeviceStats maps network.device status onto DeviceStats.
func parseDeviceStats(m map[string]any) DeviceStats {
	var ds DeviceStats
	if st, ok := m["statistics"].(map[string]any); ok {
		ds.RxBytes = asInt64(st["rx_bytes"])
		ds.TxBytes = asInt64(st["tx_bytes"])
	}
	return ds
}
