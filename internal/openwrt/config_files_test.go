// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.
package openwrt

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestConfigFilesJoinReviewDiscardAndApply(t *testing.T) {
	for _, uci := range []bool{false, true} {
		t.Run(map[bool]string{false: "files only", true: "mixed"}[uci], func(t *testing.T) {
			ctx := context.Background()
			var calls []string
			state := configFileState{Files: []ConfigFile{{Path: "/etc/dnsmasq.conf", Content: "log-queries\n", Pending: true}}}
			b := &NativeBackend{configFileCall: func(_ context.Context, sid, method string, args map[string]string, result any) error {
				if sid != "sid" {
					t.Fatalf("sid lost")
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
			}, uciChanges: func(context.Context, string) (map[string][][]string, error) {
				if uci {
					return map[string][][]string{"dhcp": {{"set", "main", "domain", "home"}}}, nil
				}
				return nil, nil
			}, uciApply: func(context.Context, string, int) error { calls = append(calls, "uci:apply"); return nil }, uciConfirm: func(context.Context, string) error { calls = append(calls, "uci:confirm"); return nil }, uciRevert: func(context.Context, string, string) error { calls = append(calls, "uci:discard"); return nil }}
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
func TestFailedUCIApplyRestoresFiles(t *testing.T) {
	var calls []string
	b := &NativeBackend{configFileCall: func(_ context.Context, _, method string, args map[string]string, result any) error {
		if method == "configFiles" {
			*result.(*configFileState) = configFileState{Files: []ConfigFile{{Pending: true}}}
		} else {
			calls = append(calls, args["action"])
		}
		return nil
	}, uciChanges: func(context.Context, string) (map[string][][]string, error) {
		return map[string][][]string{"dhcp": {{"set", "main", "domain", "home"}}}, nil
	}, uciApply: func(context.Context, string, int) error { return errors.New("failed") }}
	if b.UCIApply(context.Background(), "sid", 30) == nil {
		t.Fatal("failure hidden")
	}
	if !reflect.DeepEqual(calls, []string{"apply", "abort"}) {
		t.Fatal(calls)
	}
}

func TestInvalidFileDoesNotStageAnIncludeDirectory(t *testing.T) {
	wrote := false
	b := &NativeBackend{uciConfig: func(context.Context, string, string) (map[string]any, error) {
		return map[string]any{"main": map[string]any{".type": "dnsmasq"}}, nil
	}, uciSet: func(context.Context, string, string, string, map[string]any) error { wrote = true; return nil }, configFileCall: func(context.Context, string, string, map[string]string, any) error {
		return errors.New("invalid or stale file")
	}}
	if b.StageConfigFile(context.Background(), "sid", "/etc/dnsmasq.d/local.conf", "old", "invalid") == nil {
		t.Fatal("accepted")
	}
	if wrote {
		t.Fatal("invalid file staged UCI")
	}
}
