// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package openwrt

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/we-are-mono/verso/internal/ubus"
)

// wantProbe checks the one access probe a sid-gated read made: the session's
// ubus-scope access to object.function, asked before the read itself.
func wantProbe(t *testing.T, ubusd *fakeUbus, sid, object, function string) {
	t.Helper()
	want := ubusCall{Object: "session", Method: "access", Args: map[string]any{
		"ubus_rpc_session": sid, "scope": "ubus", "object": object, "function": function,
	}}
	if got := ubusd.only(t, "session"); !reflect.DeepEqual(got, want) {
		t.Errorf("probe = %+v, want %+v", got, want)
	}
}

// unreachableUbus is a backend whose ubus socket nothing listens on.
func unreachableUbus(t *testing.T) *NativeBackend {
	return &NativeBackend{ubusSocket: filepath.Join(t.TempDir(), "absent.sock")}
}

func TestSystemInfoMapsFields(t *testing.T) {
	ubusd := newFakeUbus(t, map[string]ubusObject{"system": replyWith(map[string]any{
		"uptime": int64(1287),
		"load":   []any{int64(1566912), int64(465440), int64(189920)},
		"memory": map[string]any{
			"total":     int64(64883740672),
			"free":      int64(22826393600),
			"available": int64(37969338368),
		},
	})})

	si, err := ubusd.backend().SystemInfo(context.Background(), "sid")
	if err != nil {
		t.Fatalf("SystemInfo: %v", err)
	}
	if si.Uptime != 1287 {
		t.Errorf("Uptime = %d, want 1287", si.Uptime)
	}
	if si.Load[0] != 1566912 {
		t.Errorf("Load[0] = %d, want 1566912", si.Load[0])
	}
	if si.Memory.Total != 64883740672 || si.Memory.Available != 37969338368 {
		t.Errorf("Memory = %+v", si.Memory)
	}
	wantProbe(t, ubusd, "sid", "system", "info")
	if got := ubusd.only(t, "system"); got.Method != "info" || len(got.Args) != 0 {
		t.Errorf("read = %+v, want system.info with no arguments", got)
	}
}

// TestBoardCarriesTheProcessorsName: `system` is the processor as the kernel
// names it, the Hardware page's Processor when no profile names the chip.
func TestBoardCarriesTheProcessorsName(t *testing.T) {
	ubusd := newFakeUbus(t, map[string]ubusObject{"system": replyWith(map[string]any{
		"system": "AMD Ryzen 7 7700 8-Core Processor", "model": "Supermicro H13SAE-MF",
	})})
	b := ubusd.backend()
	b.root = fstest.MapFS{}
	got, err := b.Board(context.Background(), "sid")
	if err != nil {
		t.Fatal(err)
	}
	if got.System != "AMD Ryzen 7 7700 8-Core Processor" || got.Model != "Supermicro H13SAE-MF" {
		t.Errorf("board = %+v", got)
	}
	wantProbe(t, ubusd, "sid", "system", "board")
	if got := ubusd.only(t, "system"); got.Method != "board" {
		t.Errorf("read = %+v, want system.board", got)
	}
}

func TestBoardPrefersFullReleaseAndKernelBuild(t *testing.T) {
	ubusd := newFakeUbus(t, map[string]ubusObject{"system": replyWith(map[string]any{
		"kernel": "6.12.101",
		"release": map[string]any{
			"distribution": "OpenWrt", "version": "25.12.4",
			"description": "OpenWrt 25.12.4 r32933-4ccb782af7",
			"target":      "qualcommax/ipq807x",
		},
	})})
	b := ubusd.backend()
	b.root = fstest.MapFS{
		"proc/version": {Data: []byte("Linux version 6.12.101 (builder@host) #1 SMP\n")},
		"etc/apk/arch": {Data: []byte("aarch64_cortex-a53\n")},
	}
	got, err := b.Board(context.Background(), "sid")
	if err != nil {
		t.Fatal(err)
	}
	if got.Firmware != "OpenWrt 25.12.4 r32933-4ccb782af7" {
		t.Errorf("firmware = %q", got.Firmware)
	}
	// The row states the release and the build stamp; the builder address and
	// the compiler pedigree between them stay in /proc/version.
	if got.KernelBuild != "6.12.101 #1 SMP" {
		t.Errorf("kernel build = %q", got.KernelBuild)
	}
	if got.Target != "qualcommax/ipq807x" {
		t.Errorf("target = %q", got.Target)
	}
	if got.Arch != "aarch64_cortex-a53" {
		t.Errorf("arch = %q, want the package architecture", got.Arch)
	}
}

// TestCondenseKernelBuild: the real shapes — a distro kernel's full pedigree
// line, an OpenWrt build, and lines that are not the shape at all, which fall
// back to the short release rather than to the snake.
func TestCondenseKernelBuild(t *testing.T) {
	for _, tc := range []struct{ full, release, want string }{
		{
			"Linux version 6.12.101+deb13-amd64 (debian-kernel@lists.debian.org) (x86_64-linux-gnu-gcc-14 (Debian 14.2.0-19) 14.2.0, GNU ld (GNU Binutils for Debian) 2.44) #1 SMP PREEMPT_DYNAMIC Debian 6.12.101-1 (2026-08-05)",
			"6.12.101+deb13-amd64",
			"6.12.101+deb13-amd64 #1 SMP PREEMPT_DYNAMIC Debian 6.12.101-1 (2026-08-05)",
		},
		{
			"Linux version 6.12.41 (builder@buildhost) (aarch64-openwrt-linux-musl-gcc (OpenWrt GCC 13.3.0) 13.3.0) #0 SMP Mon Aug 18 10:00:00 2026",
			"6.12.41",
			"6.12.41 #0 SMP Mon Aug 18 10:00:00 2026",
		},
		// No build stamp: the release from the line stands alone.
		{"Linux version 6.12.41 (builder@host) (gcc)", "6.12.41", "6.12.41"},
		// Not the shape at all: the short release ubus stated, never the line.
		{"something unexpected entirely", "6.12.41", "6.12.41"},
		{"", "6.12.41", "6.12.41"},
	} {
		if got := condenseKernelBuild(tc.full, tc.release); got != tc.want {
			t.Errorf("condenseKernelBuild(%q) = %q, want %q", tc.full, got, tc.want)
		}
	}
}

// TestSystemInfoError: a read procd refuses, a session rpcd denies and a ubus
// nobody answers are each an error, never a zero reading.
func TestSystemInfoError(t *testing.T) {
	for name, b := range map[string]*NativeBackend{
		"refused":     newFakeUbus(t, map[string]ubusObject{"system": failWith(ubusUnknownError)}).backend(),
		"denied":      newFakeUbus(t, map[string]ubusObject{"session": denyAll, "system": replyWith(map[string]any{"uptime": int64(1)})}).backend(),
		"unreachable": unreachableUbus(t),
	} {
		if _, err := b.SystemInfo(context.Background(), "sid"); err == nil {
			t.Errorf("%s: SystemInfo: want error, got nil", name)
		}
	}
}

// Identity must agree with the sign-in page without requiring rpcd or UCI.
func TestHostnameReadsRunningName(t *testing.T) {
	want, err := os.Hostname()
	if err != nil {
		t.Fatal(err)
	}
	for _, sid := range []string{"", "operator-session"} {
		got, err := NewNativeBackend().Hostname(context.Background(), sid)
		if err != nil || got != want {
			t.Fatalf("Hostname(%q) = %q, %v; want running hostname %q", sid, got, err, want)
		}
	}
}

// TestAccessPassesProbe checks Access threads the full ACL triple through to
// rpcd's session.access and reads its answer. This is the enforcement
// primitive the shell uses to gate plugin writes (ADR-007).
func TestAccessPassesProbe(t *testing.T) {
	for name, c := range map[string]struct {
		session ubusObject
		want    bool
	}{
		"granted": {grantAll, true},
		"denied":  {denyAll, false},
	} {
		ubusd := newFakeUbus(t, map[string]ubusObject{"session": c.session})
		ok, err := ubusd.backend().Access(context.Background(), "s1", "uci", "system", "write")
		if err != nil || ok != c.want {
			t.Fatalf("%s: Access = %v, %v; want %v, nil", name, ok, err, c.want)
		}
		want := map[string]any{"ubus_rpc_session": "s1", "scope": "uci", "object": "system", "function": "write"}
		if got := ubusd.only(t, "session"); got.Method != "access" || !reflect.DeepEqual(got.Args, want) {
			t.Errorf("%s: probe = %+v, want session.access %v", name, got, want)
		}
	}
	// A transport failure is not a denial: the caller fails closed on it.
	if _, err := unreachableUbus(t).Access(context.Background(), "s1", "uci", "system", "write"); err == nil {
		t.Error("Access over an unreachable ubus: want error, got nil")
	}
}

func TestSetSystemTimeThreadsStructuredArgs(t *testing.T) {
	socket, requests := helperOnce(t, `{"status":0,"result":{"result":true}}`)
	b := &NativeBackend{helperSocket: socket}
	if err := b.SetSystemTime(context.Background(), "s1", "2026-08-30T12:34:56", "GMT0"); err != nil {
		t.Fatalf("SetSystemTime: %v", err)
	}
	got := <-requests
	want := helperRequest{Method: "setSystemTime", SID: "s1", Args: map[string]string{"datetime": "2026-08-30T12:34:56", "timezone": "GMT0"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("request = %+v, want %+v", got, want)
	}
}

// wantUCI checks the calls made to rpcd's `uci` object, method and arguments
// as the wire carried them.
func wantUCI(t *testing.T, ubusd *fakeUbus, want ...ubusCall) {
	t.Helper()
	for i := range want {
		want[i].Object = "uci"
	}
	if got := ubusd.invoked("uci"); !reflect.DeepEqual(got, want) {
		t.Errorf("uci calls =\n%+v\nwant\n%+v", got, want)
	}
}

// TestUCISetThreadsArgs checks UCISet hands rpcd's uci.set the sid, config,
// section and values — the write carries the operator's sid (ADR-007: the
// shell writes on the plugin's behalf).
func TestUCISetThreadsArgs(t *testing.T) {
	ubusd := newFakeUbus(t, map[string]ubusObject{"uci": replyWith(nil)})

	err := ubusd.backend().UCISet(context.Background(), "s1", "system", "@system[0]", map[string]any{"hostname": "verso-lab"})
	if err != nil {
		t.Fatalf("UCISet: %v", err)
	}
	wantUCI(t, ubusd, ubusCall{Method: "set", Args: map[string]any{
		"ubus_rpc_session": "s1", "config": "system", "section": "@system[0]",
		"values": map[string]any{"hostname": "verso-lab"},
	}})
}

// TestUCISetClearsAnEmptyList: rpcd refuses a uci.set that carries an empty
// list with INVALID_ARGUMENT and stages nothing of the call, so clearing every
// NTP server (system.ntp.server = []) failed the whole save, its other values
// with it (verified against rpcd: the option stood as it was). An empty list
// is an option with no values, so UCISet stages it as uci.delete of the
// option, an option already absent counting as cleared, and then sets the
// rest.
func TestUCISetClearsAnEmptyList(t *testing.T) {
	clearServer := ubusCall{Method: "delete", Args: map[string]any{
		"ubus_rpc_session": "s1", "config": "system", "section": "ntp", "option": "server",
	}}
	set := func(values map[string]any) ubusCall {
		return ubusCall{Method: "set", Args: map[string]any{
			"ubus_rpc_session": "s1", "config": "system", "section": "ntp", "values": values,
		}}
	}
	for name, c := range map[string]struct {
		values map[string]any
		absent bool // uci.delete answers NOT_FOUND: the option was never set
		want   []ubusCall
	}{
		"an emptied list beside a value": {values: map[string]any{"server": []any{}, "enabled": "1"},
			want: []ubusCall{clearServer, set(map[string]any{"enabled": "1"})}},
		"an emptied list alone": {values: map[string]any{"server": []string{}}, want: []ubusCall{clearServer}},
		"a list already absent": {values: map[string]any{"server": []any{}}, absent: true, want: []ubusCall{clearServer}},
		"a list with its servers": {values: map[string]any{"server": []any{"0.openwrt.pool.ntp.org"}},
			want: []ubusCall{set(map[string]any{"server": []any{"0.openwrt.pool.ntp.org"}})}},
	} {
		t.Run(name, func(t *testing.T) {
			ubusd := newFakeUbus(t, map[string]ubusObject{"uci": func(method string, _ map[string]any) (map[string]any, int) {
				if method == "delete" && c.absent {
					return nil, ubus.StatusNotFound
				}
				return nil, ubusOK
			}})
			if err := ubusd.backend().UCISet(context.Background(), "s1", "system", "ntp", c.values); err != nil {
				t.Fatalf("UCISet: %v", err)
			}
			wantUCI(t, ubusd, c.want...)
		})
	}
	// a clear rpcd refuses for another reason stops the write
	ubusd := newFakeUbus(t, map[string]ubusObject{"uci": failWith(ubusUnknownError)})
	if err := ubusd.backend().UCISet(context.Background(), "s1", "system", "ntp", map[string]any{"server": []any{}, "enabled": "1"}); err == nil || errors.Is(err, ErrOptionNotFound) {
		t.Errorf("UCISet = %v, want the clear's failure", err)
	}
	wantUCI(t, ubusd, clearServer)
}

// TestUCIConfigThreadsArgs checks UCIConfig hands rpcd's uci.get the sid and
// config and returns the whole-config values map the shell hands a plugin as
// its read snapshot (ADR-007).
func TestUCIConfigThreadsArgs(t *testing.T) {
	ubusd := newFakeUbus(t, map[string]ubusObject{"uci": replyWith(map[string]any{
		"values": map[string]any{"wg0": map[string]any{".type": "interface", "proto": "wireguard"}},
	})})

	got, err := ubusd.backend().UCIConfig(context.Background(), "s1", "network")
	if err != nil {
		t.Fatalf("UCIConfig: %v", err)
	}
	wantUCI(t, ubusd, ubusCall{Method: "get", Args: map[string]any{"ubus_rpc_session": "s1", "config": "network"}})
	if _, ok := got["wg0"]; !ok {
		t.Errorf("UCIConfig result = %v, want the wg0 section", got)
	}
}

// TestUCIAddThreadsArgsAndReturnsSection checks UCIAdd hands rpcd's uci.add
// the sid, config and type and returns rpcd's new section id — the shell
// realizes a repeater's "add" (ADR-005 §7). An empty name is the anonymous
// section a repeater adds, and is not sent at all.
func TestUCIAddThreadsArgsAndReturnsSection(t *testing.T) {
	ubusd := newFakeUbus(t, map[string]ubusObject{"uci": replyWith(map[string]any{"section": "cfg123"})})

	sec, err := ubusd.backend().UCIAdd(context.Background(), "s1", "network", "wireguard_wg0", "")
	if err != nil {
		t.Fatalf("UCIAdd: %v", err)
	}
	if sec != "cfg123" {
		t.Errorf("section = %q, want cfg123", sec)
	}
	wantUCI(t, ubusd, ubusCall{Method: "add", Args: map[string]any{
		"ubus_rpc_session": "s1", "config": "network", "type": "wireguard_wg0",
	}})
}

// TestUCIAddNamesASection checks the named shape rpcd's `uci add` also takes: a
// typed section a config refers to by name (`config updates 'updates'`), which is
// how Verso's own settings sections come into being.
func TestUCIAddNamesASection(t *testing.T) {
	ubusd := newFakeUbus(t, map[string]ubusObject{"uci": func(_ string, args map[string]any) (map[string]any, int) {
		return map[string]any{"section": args["name"]}, ubusOK
	}})

	sec, err := ubusd.backend().UCIAdd(context.Background(), "s1", "verso", "updates", "updates")
	if err != nil {
		t.Fatalf("UCIAdd: %v", err)
	}
	if sec != "updates" {
		t.Errorf("section = %q, want updates", sec)
	}
	wantUCI(t, ubusd, ubusCall{Method: "add", Args: map[string]any{
		"ubus_rpc_session": "s1", "config": "verso", "type": "updates", "name": "updates",
	}})
}

// TestUCIDeleteThreadsArgs checks UCIDelete hands rpcd's uci.delete the
// sid, config and section, and the option too — the narrower shape that clears
// one setting rather than the whole section.
func TestUCIDeleteThreadsArgs(t *testing.T) {
	ubusd := newFakeUbus(t, map[string]ubusObject{"uci": replyWith(nil)})
	b := ubusd.backend()

	if err := b.UCIDelete(context.Background(), "s1", "network", "@wireguard_wg0[0]", ""); err != nil {
		t.Fatalf("UCIDelete: %v", err)
	}
	if err := b.UCIDelete(context.Background(), "s1", "firewall", "allow_ping", "dest_port"); err != nil {
		t.Fatalf("UCIDelete option: %v", err)
	}
	wantUCI(t, ubusd,
		ubusCall{Method: "delete", Args: map[string]any{"ubus_rpc_session": "s1", "config": "network", "section": "@wireguard_wg0[0]"}},
		ubusCall{Method: "delete", Args: map[string]any{"ubus_rpc_session": "s1", "config": "firewall", "section": "allow_ping", "option": "dest_port"}},
	)
}

// TestUCIDeleteAbsentOptionIsNamed: rpcd answers `uci delete` for an option a
// section does not carry with UBUS_STATUS_NOT_FOUND. Clearing an option that was
// never set is the state the caller asked for, so the backend names it —
// ErrOptionNotFound — and the ubus status stops at this seam.
func TestUCIDeleteAbsentOptionIsNamed(t *testing.T) {
	b := newFakeUbus(t, map[string]ubusObject{"uci": failWith(ubus.StatusNotFound)}).backend()

	err := b.UCIDelete(context.Background(), "s1", "firewall", "cfg02dc81", "masq")
	if !errors.Is(err, ErrOptionNotFound) {
		t.Fatalf("UCIDelete = %v, want ErrOptionNotFound", err)
	}
	// The section-level shape asks a different question: a section rpcd cannot
	// find is a genuine failure, and stays one.
	if err := b.UCIDelete(context.Background(), "s1", "firewall", "cfg02dc81", ""); err == nil || errors.Is(err, ErrOptionNotFound) {
		t.Errorf("a section delete must not read as a cleared option: %v", err)
	}
}

// TestUCIConfigAbsentIsNamed: rpcd answers a read of a config no installed
// package ships (wireless on a wired router, adblock before it is installed)
// with UBUS_STATUS_NOT_FOUND from the invoke. That is a fact about the router,
// not a failure, so the backend names it — ErrConfigNotFound. The same status
// from the lookup is rpcd off the bus, and stays a failure.
func TestUCIConfigAbsentIsNamed(t *testing.T) {
	for name, c := range map[string]struct {
		backend *NativeBackend
		absent  bool
	}{
		"no such config":        {newFakeUbus(t, map[string]ubusObject{"uci": failWith(ubus.StatusNotFound)}).backend(), true},
		"the uci object is off": {newFakeUbus(t, nil).backend(), false},
		"a transport failure":   {unreachableUbus(t), false},
	} {
		_, err := c.backend.UCIConfig(context.Background(), "s1", "wireless")
		if errors.Is(err, ErrConfigNotFound) != c.absent || err == nil {
			t.Errorf("%s: UCIConfig = %v, want absent %v", name, err, c.absent)
		}
	}
}

// TestUCIDeleteOtherFailuresPassThrough: only NOT_FOUND from the invoke means
// "already clear". Any other ubus status, and any transport failure, reaches the
// caller as it came.
//
// The lookup case is the one that matters most: rpcd off the bus answers the
// `uci` object lookup with the same UBUS_STATUS_NOT_FOUND, and reading that as
// "the option was already absent" would let a whole save report success without
// a single write reaching the device.
func TestUCIDeleteOtherFailuresPassThrough(t *testing.T) {
	unreachable := unreachableUbus(t)
	_, dialErr := ubus.Dial(unreachable.ubusSocket)
	for name, c := range map[string]struct {
		backend *NativeBackend
		cause   error
	}{
		"another ubus status": {
			newFakeUbus(t, map[string]ubusObject{"uci": failWith(ubus.StatusNotFound + 1)}).backend(),
			&ubus.StatusError{Code: ubus.StatusNotFound + 1, Phase: ubus.PhaseInvoke, Call: "delete"},
		},
		"a transport failure": {unreachable, dialErr},
		"the uci object is off": {
			newFakeUbus(t, nil).backend(),
			&ubus.StatusError{Code: ubus.StatusNotFound, Phase: ubus.PhaseLookup, Call: "uci"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			err := c.backend.UCIDelete(context.Background(), "s1", "firewall", "cfg02dc81", "masq")
			if errors.Is(err, ErrOptionNotFound) || err == nil || err.Error() != c.cause.Error() {
				t.Errorf("UCIDelete = %v, want the cause unchanged: %v", err, c.cause)
			}
		})
	}
}

// TestUCIOrderThreadsArgs checks UCIOrder hands rpcd's uci.order the sid and
// config with the whole new section sequence — the shape rpcd's `uci order`
// takes to stage a reordered listing.
func TestUCIOrderThreadsArgs(t *testing.T) {
	ubusd := newFakeUbus(t, map[string]ubusObject{"uci": replyWith(nil)})

	if err := ubusd.backend().UCIOrder(context.Background(), "s1", "firewall", []string{"allow_ping", "allow_dhcp_renew", "cfg0af21e"}); err != nil {
		t.Fatalf("UCIOrder: %v", err)
	}
	wantUCI(t, ubusd, ubusCall{Method: "order", Args: map[string]any{
		"ubus_rpc_session": "s1", "config": "firewall",
		"sections": []any{"allow_ping", "allow_dhcp_renew", "cfg0af21e"},
	}})
}

// TestUCIStageVerbsThreadArgs checks the stage's own verbs hand rpcd's `uci`
// object what it takes: changes across every config, a config's revert, an
// apply with its rollback armed for the timeout, and the confirm that keeps it.
func TestUCIStageVerbsThreadArgs(t *testing.T) {
	ubusd := newFakeUbus(t, map[string]ubusObject{"uci": func(method string, _ map[string]any) (map[string]any, int) {
		if method == "changes" {
			return map[string]any{"changes": map[string]any{"network": []any{[]any{"set", "lan", "proto", "static"}}}}, ubusOK
		}
		return nil, ubusOK
	}})
	b := ubusd.backend()
	b.helperSocket = configFileHelper(t, func(context.Context, string, string, map[string]string, any) error { return nil })
	ctx := context.Background()

	changes, err := b.UCIChanges(ctx, "s1")
	if err != nil || !reflect.DeepEqual(changes, map[string][][]string{"network": {{"set", "lan", "proto", "static"}}}) {
		t.Fatalf("UCIChanges = %v, %v", changes, err)
	}
	if err := b.UCIUnstage(ctx, "s1", "network"); err != nil {
		t.Fatalf("UCIUnstage: %v", err)
	}
	if err := b.UCIApply(ctx, "s1", 30); err != nil {
		t.Fatalf("UCIApply: %v", err)
	}
	if err := b.UCIConfirm(ctx, "s1"); err != nil {
		t.Fatalf("UCIConfirm: %v", err)
	}
	wantUCI(t, ubusd,
		ubusCall{Method: "changes", Args: map[string]any{"ubus_rpc_session": "s1"}},
		ubusCall{Method: "revert", Args: map[string]any{"ubus_rpc_session": "s1", "config": "network"}},
		ubusCall{Method: "apply", Args: map[string]any{"ubus_rpc_session": "s1", "rollback": int64(1), "timeout": int64(30)}},
		ubusCall{Method: "confirm", Args: map[string]any{"ubus_rpc_session": "s1"}},
	)
}

func TestParseWANConnIPv6(t *testing.T) {
	v4 := map[string]any{
		"proto": "dhcp",
		"ipv4-address": []any{
			map[string]any{"address": "10.0.0.138", "mask": int64(24)},
		},
	}
	v6 := map[string]any{
		"proto": "dhcpv6",
		"ipv6-address": []any{
			map[string]any{"address": "2001:db8::f7c", "mask": int64(128)},
		},
		"ipv6-prefix": []any{
			map[string]any{"address": "2001:db8:1::", "mask": int64(56), "valid": int64(690)},
		},
		"route": []any{
			map[string]any{"target": "::", "mask": int64(0), "nexthop": "fe80::1"},
		},
		"dns-server": []any{"2001:4860:4860::8888"},
	}

	got := parseWANConn(v4, v6)
	if got.V6Proto != "DHCPv6 client" || got.V6Addr != "2001:db8::f7c/128" ||
		got.V6Prefix != "2001:db8:1::/56" || got.V6Gateway != "fe80::1" ||
		len(got.V6DNS) != 1 || got.V6DNS[0] != "2001:4860:4860::8888" || got.V6Valid != 690 {
		t.Errorf("wan6 facts = %+v", got)
	}

	// Some netifd protocols expose the IPv6 facts on wan itself instead of
	// creating a separate wan6 object.
	dualStack := map[string]any{}
	for key, value := range v4 {
		dualStack[key] = value
	}
	for key, value := range v6 {
		dualStack[key] = value
	}
	got = parseWANConn(dualStack, nil)
	if got.V6Addr != "2001:db8::f7c/128" || got.V6Prefix != "2001:db8:1::/56" {
		t.Errorf("wan fallback facts = %+v", got)
	}
}

func TestWANStatusesUsesRoutesRatherThanNames(t *testing.T) {
	dump := map[string]any{"interface": []any{
		map[string]any{
			"interface": "lan6", "proto": "dhcpv6", "l3_device": "br-lan",
			"ipv6-address": []any{map[string]any{"address": "fd00::1"}},
		},
		map[string]any{
			"interface": "upstream", "up": true, "proto": "pppoe", "l3_device": "pppoe-upstream",
			"ipv4-address": []any{map[string]any{"address": "192.0.2.2"}},
			"route":        []any{map[string]any{"target": "0.0.0.0", "mask": int64(0), "metric": int64(20)}},
		},
		map[string]any{
			"interface": "upstream_6", "up": true, "proto": "dhcpv6", "dynamic": true,
			"l3_device":   "pppoe-upstream",
			"ipv6-prefix": []any{map[string]any{"address": "2001:db8:1::", "mask": int64(56)}},
			"route":       []any{map[string]any{"target": "::", "mask": int64(0), "metric": int64(20)}},
		},
	}}

	wan, wan6 := wanStatuses(dump)
	if name, _ := wan["interface"].(string); name != "upstream" {
		t.Fatalf("wan interface = %q", name)
	}
	if name, _ := wan6["interface"].(string); name != "upstream_6" {
		t.Fatalf("IPv6 companion = %q, want upstream_6", name)
	}
}

func TestWANStatusesAllowsIPv6DirectlyOnWAN(t *testing.T) {
	wan := map[string]any{
		"interface": "upstream", "up": true, "proto": "pppoe", "l3_device": "pppoe-upstream",
		"ipv6-address": []any{map[string]any{"address": "2001:db8::2"}},
		"route": []any{
			map[string]any{"target": "0.0.0.0", "mask": int64(0)},
			map[string]any{"target": "::", "mask": int64(0)},
		},
	}
	gotWAN, gotV6 := wanStatuses(map[string]any{"interface": []any{wan}})
	if gotWAN == nil || gotV6 == nil || gotWAN["interface"] != gotV6["interface"] {
		t.Fatalf("statuses = wan:%v v6:%v, want one dual-stack owner", gotWAN, gotV6)
	}
	if got := parseWANConn(gotWAN, gotV6).V6Addr; got != "2001:db8::2" {
		t.Errorf("direct WAN IPv6 = %q", got)
	}
}

// TestParseDeviceStats: counters come from the statistics table.
func TestParseDeviceStats(t *testing.T) {
	got := parseDeviceStats(map[string]any{
		"statistics": map[string]any{"rx_bytes": int64(7733), "tx_bytes": int64(13877)},
	})
	if got.RxBytes != 7733 || got.TxBytes != 13877 {
		t.Errorf("device stats = %+v", got)
	}
}

// TestParseV6Leases folds the odhcpd ipv6leases response — per-device, per-lease
// with an ipv6-addr array — to a flat DUID/hostname/addresses list, skipping any
// lease without a DUID.
func TestParseV6Leases(t *testing.T) {
	m := map[string]any{
		"device": map[string]any{
			"br-lan": map[string]any{
				"leases": []any{
					map[string]any{
						"duid":     "00030001a483e72b190c",
						"hostname": "gaming-pc",
						"ipv6-addr": []any{
							map[string]any{"address": "2001:db8::4f", "valid": 43200.0},
							map[string]any{"address": "fd00::4f"},
						},
					},
					map[string]any{"hostname": "no-duid"}, // skipped: no DUID
				},
			},
		},
	}
	got := parseV6Leases(m)
	if len(got) != 1 {
		t.Fatalf("leases = %d, want 1 (the DUID-less one dropped): %+v", len(got), got)
	}
	l := got[0]
	if l.DUID != "00030001a483e72b190c" || l.Hostname != "gaming-pc" {
		t.Errorf("lease id wrong: %+v", l)
	}
	if len(l.Addrs) != 2 || l.Addrs[0] != "2001:db8::4f" || l.Addrs[1] != "fd00::4f" {
		t.Errorf("lease addrs wrong: %+v", l.Addrs)
	}
}

func TestEnrichRCStatesClassifiesServicesAndAggregatesRuntime(t *testing.T) {
	// rc.list is read without its running checks, so no state arrives running:
	// procd's instances say what runs, and the script says whether procd runs it.
	states := map[string]RCState{
		"boot": {}, "dnsmasq": {}, "firewall": {}, "urandom_seed": {}, "uhttpd": {},
	}
	details := map[string]serviceDetail{
		"dnsmasq":      {Instances: []serviceInstance{{PID: 42, Running: true, HasCommand: true, Respawn: true}}},
		"firewall":     {},
		"urandom_seed": {Instances: []serviceInstance{{HasCommand: true, HasExitCode: true, ExitCode: 0}}},
	}
	procFields := append([]string{"S"}, make([]string, 19)...)
	for i := 1; i < 19; i++ {
		procFields[i] = "0"
	}
	procFields[19] = "2500" // 25 seconds after boot at USER_HZ=100
	files := map[string]string{
		"/proc/uptime":             "100.00 20.00\n",
		"/etc/init.d/boot":         "START=10\n",
		"/etc/init.d/dnsmasq":      "USE_PROCD=1\nprocd_set_param command /usr/sbin/dnsmasq\n",
		"/etc/init.d/firewall":     "USE_PROCD=1\nstart_service() { fw4 start; }\n",
		"/etc/init.d/urandom_seed": "USE_PROCD=1\nprocd_set_param command /sbin/urandom_seed\n",
		"/etc/init.d/uhttpd":       "#!/bin/sh /etc/rc.common\nSTART=50\nUSE_PROCD=1\nprocd_set_param command /usr/sbin/uhttpd\n",
		"/proc/42/status":          "Name:\tdnsmasq\nVmRSS:\t2048 kB\n",
		"/proc/42/stat":            "42 (dns masq) " + strings.Join(procFields, " ") + "\n",
	}
	readFile := func(path string) ([]byte, error) {
		if value, ok := files[path]; ok {
			return []byte(value), nil
		}
		return nil, fmt.Errorf("missing %s", path)
	}

	enrichRCStates(states, details, readFile, nil)
	if states["boot"].Kind != ServiceTask || states["urandom_seed"].Kind != ServiceTask {
		t.Errorf("task classification = boot:%q urandom:%q", states["boot"].Kind, states["urandom_seed"].Kind)
	}
	if states["dnsmasq"].Kind != ServiceDaemon || states["firewall"].Kind != ServiceSubsystem {
		t.Errorf("managed classification = dnsmasq:%q firewall:%q", states["dnsmasq"].Kind, states["firewall"].Kind)
	}
	if states["uhttpd"].Kind != ServiceDaemon {
		t.Errorf("a stopped procd daemon must class as daemon, got %q", states["uhttpd"].Kind)
	}
	for name, want := range map[string]bool{"dnsmasq": true, "boot": false, "firewall": false, "urandom_seed": false, "uhttpd": false} {
		if states[name].Running != want {
			t.Errorf("%s running = %v, want %v", name, states[name].Running, want)
		}
	}
	dns := states["dnsmasq"]
	if len(dns.PIDs) != 1 || dns.PIDs[0] != 42 || dns.MemoryBytes != 2*1024*1024 || dns.Uptime != 75 {
		t.Errorf("dnsmasq runtime = %+v", dns)
	}
}

// TestAJailedServiceCountsWhatRunsInItsJail: procd names a jailed instance by
// its ujail wrapper's pid, and the service runs as the wrapper's child, so a
// service's memory is its instance's whole process tree: the wrapper, the
// daemon, and whatever the daemon started; a process outside it is not.
func TestAJailedServiceCountsWhatRunsInItsJail(t *testing.T) {
	stat := func(pid, ppid int) string {
		fields := append([]string{"S", strconv.Itoa(ppid)}, make([]string, 18)...)
		for i := 2; i < 20; i++ {
			fields[i] = "0"
		}
		return strconv.Itoa(pid) + " (x) " + strings.Join(fields, " ") + "\n"
	}
	files := map[string]string{
		"/proc/uptime":     "100.00 20.00\n",
		"/proc/50/status":  "Name:\tujail\nVmRSS:\t1024 kB\n",
		"/proc/50/stat":    stat(50, 1),
		"/proc/51/status":  "Name:\tAdGuardHome\nVmRSS:\t280000 kB\n",
		"/proc/51/stat":    stat(51, 50),
		"/proc/52/status":  "Name:\thelper\nVmRSS:\t1000 kB\n",
		"/proc/52/stat":    stat(52, 51),
		"/proc/60/status":  "Name:\tdnsmasq\nVmRSS:\t2048 kB\n",
		"/proc/60/stat":    stat(60, 1),
		"/etc/init.d/adgh": "USE_PROCD=1\nprocd_set_param command /usr/bin/AdGuardHome\n",
	}
	readFile := func(path string) ([]byte, error) {
		if value, ok := files[path]; ok {
			return []byte(value), nil
		}
		return nil, fmt.Errorf("missing %s", path)
	}
	states := map[string]RCState{"adgh": {}}
	details := map[string]serviceDetail{"adgh": {Instances: []serviceInstance{{PID: 50, Running: true, HasCommand: true}}}}
	enrichRCStates(states, details, readFile, []int{1, 50, 51, 52, 60})
	adgh := states["adgh"]
	if want := int64(1024+280000+1000) * 1024; adgh.MemoryBytes != want {
		t.Errorf("memory = %d, want %d: the wrapper, the daemon in it and its helper", adgh.MemoryBytes, want)
	}
	if !slices.Equal(adgh.PIDs, []int{50}) {
		t.Errorf("pids = %v, want procd's own, [50]", adgh.PIDs)
	}
}

// TestProcdFailedIsAnInstanceProcdStoppedRestarting: procd keeps a daemon's
// instance after it gives up on it (a crash loop clears its respawn), while a
// stop drops the instance. Not running, an error exit and no respawn left is
// failed; an instance waiting out its respawn delay is not failed yet, and a
// one-shot that exited 0 completed.
func TestProcdFailedIsAnInstanceProcdStoppedRestarting(t *testing.T) {
	raw := map[string]any{
		"sysntpd":      map[string]any{"instances": map[string]any{"instance1": map[string]any{"running": false, "command": []any{"/usr/sbin/ntpd"}, "exit_code": int64(1)}}},
		"odhcpd":       map[string]any{"instances": map[string]any{"instance1": map[string]any{"running": false, "command": []any{"/usr/sbin/odhcpd"}, "exit_code": int64(1), "respawn": map[string]any{"retry": int64(5)}}}},
		"urandom_seed": map[string]any{"instances": map[string]any{"instance1": map[string]any{"running": false, "command": []any{"/sbin/urandom_seed"}, "exit_code": int64(0)}}},
		"dnsmasq":      map[string]any{"instances": map[string]any{"cfg01411c": map[string]any{"running": true, "pid": int64(42), "command": []any{"/usr/sbin/dnsmasq"}}}},
		"gave-up":      map[string]any{"instances": map[string]any{"instance1": map[string]any{"running": false, "respawn_failed": true}}},
	}
	if got := failedServices(raw); !slices.Equal(got, []string{"gave-up", "sysntpd"}) {
		t.Errorf("failedServices = %v, want [gave-up sysntpd]", got)
	}
	states := map[string]RCState{"sysntpd": {Enabled: true}, "odhcpd": {Enabled: true}, "dnsmasq": {Enabled: true}}
	enrichRCStates(states, parseServiceDetails(raw), func(string) ([]byte, error) { return nil, fmt.Errorf("missing") }, nil)
	for name, want := range map[string]bool{"sysntpd": true, "odhcpd": false, "dnsmasq": false} {
		if states[name].Failed != want {
			t.Errorf("%s failed = %v, want %v", name, states[name].Failed, want)
		}
	}
}

// TestParseChangesKeepsNumericFields: an order change carries the section's new
// position as a number; the tuple must survive with the number rendered as its
// decimal string, or a staged reorder becomes invisible to the stage's count.
func TestParseChangesKeepsNumericFields(t *testing.T) {
	got := parseChanges(map[string]any{
		"firewall": []any{
			[]any{"order", "cfg0792bd", int64(5)},
			[]any{"order", "cfg0892bd", float64(6)},
			[]any{"set", "cfg0992bd", "enabled", "0"},
			[]any{"bogus", map[string]any{}},
		},
	})
	want := [][]string{
		{"order", "cfg0792bd", "5"},
		{"order", "cfg0892bd", "6"},
		{"set", "cfg0992bd", "enabled", "0"},
	}
	if len(got["firewall"]) != len(want) {
		t.Fatalf("parseChanges kept %d tuples, want %d: %v", len(got["firewall"]), len(want), got)
	}
	for i, tuple := range want {
		for j, field := range tuple {
			if got["firewall"][i][j] != field {
				t.Errorf("tuple %d field %d = %q, want %q", i, j, got["firewall"][i][j], field)
			}
		}
	}
}

// TestLogReadFoldsLogdRecords: logd's reply is a flat list of records; the fold
// keeps the id (a reader's cursor), the source (kernel or syslog), the stamp,
// and the message, and drops an element carrying no message at all rather than
// letting one malformed entry cost the read.
func TestLogReadFoldsLogdRecords(t *testing.T) {
	ubusd := newFakeUbus(t, map[string]ubusObject{"log": replyWith(map[string]any{"log": []any{
		map[string]any{"msg": "Log-WAN-probes: IN=wan0 OUT= SRC=203.0.113.9", "id": int64(4211), "priority": int64(4), "source": int64(0), "time": int64(1788294054648)},
		map[string]any{"id": int64(4212)}, // no message: nothing to read
		map[string]any{"msg": "verso: listening", "id": int64(4213), "priority": int64(27), "source": int64(1)},
		"not a record",
	}})})
	entries, err := ubusd.backend().LogRead(context.Background(), "sid-1", 64)
	if err != nil {
		t.Fatalf("LogRead: %v", err)
	}
	wantProbe(t, ubusd, "sid-1", "log", "read")
	// A one-shot tail of the asked length: logd's default is a subscription
	// that never returns.
	want := ubusCall{Object: "log", Method: "read", Args: map[string]any{"lines": int64(64), "stream": int64(0), "oneshot": int64(1)}}
	if got := ubusd.only(t, "log"); !reflect.DeepEqual(got, want) {
		t.Errorf("read = %+v, want %+v", got, want)
	}
	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2: %+v", len(entries), entries)
	}
	first := entries[0]
	if first.ID != 4211 || first.Source != 0 || first.Priority != 4 || first.Time != 1788294054648 {
		t.Errorf("kernel record not folded: %+v", first)
	}
	if first.Msg != "Log-WAN-probes: IN=wan0 OUT= SRC=203.0.113.9" {
		t.Errorf("message not carried verbatim: %q", first.Msg)
	}
	if entries[1].ID != 4213 || entries[1].Source != 1 {
		t.Errorf("syslog record not folded: %+v", entries[1])
	}
}

// TestLogReadPropagatesError: a session rpcd denies the log reads nothing — the
// denial is the answer, and logd is never asked.
func TestLogReadPropagatesError(t *testing.T) {
	ubusd := newFakeUbus(t, map[string]ubusObject{"session": denyAll, "log": replyWith(map[string]any{"log": []any{}})})
	if _, err := ubusd.backend().LogRead(context.Background(), "sid", 10); !errors.Is(err, ErrAccessDenied) {
		t.Errorf("LogRead error = %v, want ErrAccessDenied", err)
	}
	if calls := ubusd.invoked("log"); len(calls) != 0 {
		t.Errorf("a denied session read the log: %+v", calls)
	}
}

// TestNetworkInterfacesPairsLogicalNamesWithDevices: the L3 device is what the
// kernel writes into a log line, so it wins over the configured device; an
// entry without a logical name is not a join anyone can use.
func TestNetworkInterfacesPairsLogicalNamesWithDevices(t *testing.T) {
	ubusd := newFakeUbus(t, map[string]ubusObject{"network.interface": replyWith(map[string]any{"interface": []any{
		map[string]any{"interface": "lan", "device": "br-lan", "l3_device": "br-lan", "up": true},
		map[string]any{"interface": "wan", "device": "wan0", "l3_device": "pppoe-wan", "up": true},
		map[string]any{"interface": "mgmt", "device": "eth0", "up": int64(0)},
		map[string]any{"device": "eth9"},
	}})})
	got, err := ubusd.backend().NetworkInterfaces(context.Background(), "sid")
	if err != nil {
		t.Fatalf("NetworkInterfaces: %v", err)
	}
	wantProbe(t, ubusd, "sid", "network.interface", "dump")
	if call := ubusd.only(t, "network.interface"); call.Method != "dump" {
		t.Errorf("read = %+v, want network.interface.dump", call)
	}
	want := []NetIface{
		{Name: "lan", Device: "br-lan", Up: true},
		{Name: "wan", Device: "pppoe-wan", Up: true},
		{Name: "mgmt", Device: "eth0"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d interfaces, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if !reflect.DeepEqual(got[i], want[i]) {
			t.Errorf("interface %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}
