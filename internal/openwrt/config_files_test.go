// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.
package openwrt

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"reflect"
	"testing"
)

// configFileHelper answers every request at a fake helper socket through
// call, the way the helper's file verbs answer: a configFiles read fills the
// state call is handed, and an error call returns is the helper's refusal.
func configFileHelper(t *testing.T, call func(ctx context.Context, sid, method string, args map[string]string, result any) error) string {
	t.Helper()
	socket := filepath.Join(t.TempDir(), "helper.sock")
	listener := listenAsHelper(t, socket)
	t.Cleanup(func() { _ = listener.Close() })
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			var request helperRequest
			if json.NewDecoder(conn).Decode(&request) == nil {
				var result any
				if request.Method == "configFiles" {
					result = &configFileState{}
				}
				response := map[string]any{"status": 0, "result": result}
				if err := call(context.Background(), request.SID, request.Method, request.Args, result); err != nil {
					response = map[string]any{"status": 2, "error": err.Error()}
				}
				_ = json.NewEncoder(conn).Encode(response)
			}
			_ = conn.Close()
		}
	}()
	return socket
}

func TestConfigFilesJoinReviewDiscardAndApply(t *testing.T) {
	for _, uci := range []bool{false, true} {
		t.Run(map[bool]string{false: "files only", true: "mixed"}[uci], func(t *testing.T) {
			ctx := context.Background()
			var calls []string
			state := configFileState{Files: []ConfigFile{{Path: "/etc/dnsmasq.conf", Family: "dnsmasq", Content: "log-queries\n", Pending: true}}}
			staged := map[string]any{}
			if uci {
				staged["dhcp"] = []any{[]any{"set", "main", "domain", "home"}}
			}
			b := uciStage(t, staged, func(method string) int {
				calls = append(calls, "uci:"+map[string]string{"apply": "apply", "confirm": "confirm", "revert": "discard"}[method])
				return ubusOK
			})
			b.helperSocket = configFileHelper(t, func(_ context.Context, sid, method string, args map[string]string, result any) error {
				if sid != "sid" {
					t.Errorf("sid lost")
				}
				if method == "configFiles" {
					*result.(*configFileState) = state
					return nil
				}
				calls = append(calls, "files:"+args["action"])
				switch args["action"] {
				case "apply":
					state.Active = true
					state.UCI = args["uci"] == "1"
				case "uci-confirmed":
					state.UCIConfirmed = true
				case "confirm", "discard":
					state = configFileState{}
				}
				return nil
			})
			changes, err := b.UCIChanges(ctx, "sid")
			if err != nil || len(changes["dhcp"]) == 0 {
				t.Fatalf("file missing from review: %v %v", changes, err)
			}
			if err := b.UCIApply(ctx, "sid", 30); err != nil {
				t.Fatal(err)
			}
			if err := b.UCIConfirm(ctx, "sid"); err != nil {
				t.Fatal(err)
			}
			want := []string{"files:apply", "files:confirm"}
			if uci {
				want = []string{"files:apply", "uci:apply", "uci:confirm", "files:uci-confirmed", "files:confirm"}
			}
			if !reflect.DeepEqual(calls, want) {
				t.Fatalf("calls %v want %v", calls, want)
			}
			if err := b.UCIRevert(ctx, "sid", "dhcp"); err != nil {
				t.Fatal(err)
			}
			if calls[len(calls)-1] != "files:discard" {
				t.Fatalf("file stage survived discard")
			}
		})
	}
}

// TestAFileWaitsUnderItsOwnConfig: a staged file is a change to the config
// whose daemon reads it — a rule file waits under the firewall, a dnsmasq file
// under dhcp — and discarding one config leaves the other's files staged.
func TestAFileWaitsUnderItsOwnConfig(t *testing.T) {
	ctx := context.Background()
	var discarded []string
	state := configFileState{Files: []ConfigFile{
		{Path: "/etc/dnsmasq.d/10-local.conf", Family: "dnsmasq", Content: "log-queries\n", Pending: true},
		{Path: "/etc/nftables.d/10-custom.nft", Family: "fw4", Content: "chain x {}\n", Pending: true},
	}}
	b := uciStage(t, map[string]any{}, func(string) int { return ubusOK })
	b.helperSocket = configFileHelper(t, func(_ context.Context, _, method string, args map[string]string, result any) error {
		if method == "configFiles" {
			*result.(*configFileState) = state
			return nil
		}
		if args["action"] == "discard" {
			discarded = append(discarded, args["family"])
		}
		return nil
	})
	changes, err := b.UCIChanges(ctx, "sid")
	if err != nil {
		t.Fatal(err)
	}
	want := map[string][][]string{
		"dhcp":     {{"file", "/etc/dnsmasq.d/10-local.conf", "log-queries\n"}},
		"firewall": {{"file", "/etc/nftables.d/10-custom.nft", "chain x {}\n"}},
	}
	if !reflect.DeepEqual(changes, want) {
		t.Fatalf("changes %v want %v", changes, want)
	}
	for config, family := range map[string]string{"firewall": "fw4", "dhcp": "dnsmasq", "openvpn": "openvpn"} {
		discarded = nil
		if err := b.UCIRevert(ctx, "sid", config); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(discarded, []string{family}) {
			t.Errorf("discarding %s discarded files of %v", config, discarded)
		}
	}
	discarded = nil
	if err := b.UCIRevert(ctx, "sid", "network"); err != nil || discarded != nil {
		t.Errorf("a config no file belongs to discards no files: %v %v", discarded, err)
	}
}

func TestFailedUCIApplyRestoresFiles(t *testing.T) {
	var calls []string
	b := uciStage(t, map[string]any{"dhcp": []any{[]any{"set", "main", "domain", "home"}}}, func(method string) int {
		if method == "apply" {
			return ubusUnknownError
		}
		return ubusOK
	})
	b.helperSocket = configFileHelper(t, func(_ context.Context, _, method string, args map[string]string, result any) error {
		if method == "configFiles" {
			*result.(*configFileState) = configFileState{Files: []ConfigFile{{Pending: true}}}
		} else {
			calls = append(calls, args["action"])
		}
		return nil
	})
	if b.UCIApply(context.Background(), "sid", 30) == nil {
		t.Fatal("failure hidden")
	}
	if !reflect.DeepEqual(calls, []string{"apply", "abort"}) {
		t.Fatal(calls)
	}
}

func TestInvalidFileDoesNotStageAnIncludeDirectory(t *testing.T) {
	ubusd := newFakeUbus(t, map[string]ubusObject{"uci": replyWith(map[string]any{
		"values": map[string]any{"main": map[string]any{".type": "dnsmasq"}},
	})})
	b := ubusd.backend()
	b.helperSocket = configFileHelper(t, func(context.Context, string, string, map[string]string, any) error {
		return errors.New("invalid or stale file")
	})
	if b.StageConfigFile(context.Background(), "sid", "/etc/dnsmasq.d/local.conf", "old", "invalid") == nil {
		t.Fatal("accepted")
	}
	for _, call := range ubusd.invoked("uci") {
		if call.Method != "get" {
			t.Fatalf("invalid file staged UCI: %+v", call)
		}
	}
}

// uciStage is a backend whose ubusd's `uci` object holds changes as its stage
// and hands every other method it is asked to run, which answers its status.
func uciStage(t *testing.T, changes map[string]any, run func(method string) int) *NativeBackend {
	t.Helper()
	return newFakeUbus(t, map[string]ubusObject{"uci": func(method string, _ map[string]any) (map[string]any, int) {
		if method == "changes" {
			return map[string]any{"changes": changes}, ubusOK
		}
		return nil, run(method)
	}}).backend()
}
