// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package openwrt

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"net"
	"path/filepath"
	"strings"
	"testing"
)

func TestCallHelperProtocol(t *testing.T) {
	socket := filepath.Join(t.TempDir(), "helper.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	done := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			done <- err
			return
		}
		defer conn.Close()
		var request helperRequest
		if err := json.NewDecoder(conn).Decode(&request); err != nil {
			done <- err
			return
		}
		if request.Method != "pkgSearch" || request.SID != "good-sid" || request.Args["query"] != "htop" {
			done <- errors.New("unexpected helper request")
			return
		}
		done <- json.NewEncoder(conn).Encode(map[string]any{
			"status": 0,
			"result": map[string]any{"packages": []any{map[string]any{
				"name": "htop", "version": "3.5.1-r1", "feed": "packages", "installed": false,
			}}, "total": 1},
		})
	}()

	packages, total, err := dialPkgSearch(socket)(context.Background(), "good-sid", "htop")
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(packages) != 1 || packages[0].Name != "htop" || packages[0].Version != "3.5.1-r1" {
		t.Fatalf("packages=%+v total=%d", packages, total)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestPackageDetailAndBrowseVerbsCarrySessionAndExactArguments(t *testing.T) {
	for _, method := range []string{"pkgBrowse", "pkgFiles", "pkgUpgradeOne"} {
		t.Run(method, func(t *testing.T) {
			socket := filepath.Join(t.TempDir(), "helper.sock")
			listener, err := net.Listen("unix", socket)
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			done := make(chan error, 1)
			go func() {
				conn, err := listener.Accept()
				if err != nil {
					done <- err
					return
				}
				defer conn.Close()
				var request helperRequest
				if err := json.NewDecoder(conn).Decode(&request); err != nil {
					done <- err
					return
				}
				valid := request.Method == method && request.SID == "operator-sid"
				if method == "pkgBrowse" {
					valid = valid && request.Args["query"] == "" && request.Args["offset"] == "30"
				} else {
					valid = valid && request.Args["package"] == "htop"
				}
				if !valid {
					done <- errors.New("unexpected helper arguments or session")
					return
				}
				done <- json.NewEncoder(conn).Encode(map[string]any{"status": 0, "result": map[string]any{
					"packages": []Package{{Name: "htop"}}, "total": 64, "count": 99, "installed": 20, "files": []string{"/usr/bin/htop"},
				}})
			}()
			switch method {
			case "pkgBrowse":
				page, err := dialPkgBrowse(socket)(context.Background(), "operator-sid", "", 30)
				if err != nil || page.Total != 64 || page.Count != 99 || page.Installed != 20 || len(page.Packages) != 1 {
					t.Fatalf("page %+v, error %v", page, err)
				}
			case "pkgFiles":
				files, err := dialPkgFiles(socket)(context.Background(), "operator-sid", "htop")
				if err != nil || len(files) != 1 || files[0] != "/usr/bin/htop" {
					t.Fatalf("files %v, error %v", files, err)
				}
			case "pkgUpgradeOne":
				if err := dialPkgAct(socket, method)(context.Background(), "operator-sid", "htop"); err != nil {
					t.Fatal(err)
				}
			}
			if err := <-done; err != nil {
				t.Fatal(err)
			}
		})
	}
}

// TestFirewallCountersPassesResultThrough: the counters verb carries the operator's
// sid and no arguments, and the helper's result reaches the caller as raw JSON —
// the shell brokers firewall state without interpreting it (ADR-007).
func TestFirewallCountersPassesResultThrough(t *testing.T) {
	socket := filepath.Join(t.TempDir(), "helper.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	done := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			done <- err
			return
		}
		defer conn.Close()
		var request helperRequest
		if err := json.NewDecoder(conn).Decode(&request); err != nil {
			done <- err
			return
		}
		if request.Method != "firewallCounters" || request.SID != "good-sid" || len(request.Args) != 0 {
			done <- errors.New("unexpected helper request")
			return
		}
		_, err = conn.Write([]byte(
			`{"status":0,"result":{"counters":[{"chain":"input_wan","name":"Allow-Ping","packets":12,"bytes":1008}]}}` + "\n"))
		done <- err
	}()

	result, err := dialFirewallCounters(socket)(context.Background(), "good-sid")
	if err != nil {
		t.Fatal(err)
	}
	want := `{"counters":[{"chain":"input_wan","name":"Allow-Ping","packets":12,"bytes":1008}]}`
	if string(result) != want {
		t.Errorf("result = %s, want the helper's JSON verbatim %s", result, want)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

// helperReplying answers one request with a canned body, after checking that the
// verb arrived under the operator's sid with no arguments — the shape every read
// verb shares.
func helperReplying(t *testing.T, method, body string) string {
	t.Helper()
	return helperReplyingTo(t, method, nil, body)
}

// helperReplyingTo is helperReplying for a verb that carries arguments: the
// request must arrive with exactly these.
func helperReplyingTo(t *testing.T, method string, args map[string]string, body string) string {
	t.Helper()
	socket := filepath.Join(t.TempDir(), "helper.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		var request helperRequest
		if err := json.NewDecoder(conn).Decode(&request); err != nil {
			return
		}
		if request.Method != method || request.SID != "good-sid" || !maps.Equal(request.Args, args) {
			_ = json.NewEncoder(conn).Encode(map[string]any{"status": 2, "error": "unexpected request"})
			return
		}
		_, _ = conn.Write([]byte(body + "\n"))
	}()
	return socket
}

// TestPkgInfoAnswersOnePackageOrNone: a package's panel asks for it by name,
// and the helper's null — neither installed nor in the feeds — is an answer,
// not an error.
func TestPkgInfoAnswersOnePackageOrNone(t *testing.T) {
	socket := helperReplyingTo(t, "pkgInfo", map[string]string{"package": "htop"},
		`{"status":0,"result":{"package":{"name":"htop","version":"3.5.1-r1","installed":true,"removable":true}}}`)
	got, ok, err := dialPkgInfo(socket)(context.Background(), "good-sid", "htop")
	if err != nil || !ok || got.Name != "htop" || got.Version != "3.5.1-r1" || !got.Installed || !got.Removable {
		t.Fatalf("package=%+v ok=%v err=%v", got, ok, err)
	}
	socket = helperReplyingTo(t, "pkgInfo", map[string]string{"package": "nosuch"}, `{"status":0,"result":{"package":null}}`)
	if got, ok, err := dialPkgInfo(socket)(context.Background(), "good-sid", "nosuch"); err != nil || ok {
		t.Fatalf("an unknown package: package=%+v ok=%v err=%v", got, ok, err)
	}
	socket = helperReplyingTo(t, "pkgInfo", map[string]string{"package": "htop"}, `{"status":6,"error":"permission denied"}`)
	if _, _, err := dialPkgInfo(socket)(context.Background(), "good-sid", "htop"); !errors.Is(err, ErrAccessDenied) {
		t.Fatalf("a refused read: err=%v", err)
	}
}

// TestPkgUpgradableDecodesBothVersions: the upgradable set reaches the caller as
// typed entries naming what the device runs and what the feeds hold.
func TestPkgUpgradableDecodesBothVersions(t *testing.T) {
	socket := helperReplying(t, "pkgUpgradable",
		`{"status":0,"result":{"packages":[{"name":"dnsmasq","installed":"2.91-r3","available":"2.93-r1"}]}}`)
	packages, err := dialPkgUpgradable(socket)(context.Background(), "good-sid")
	if err != nil {
		t.Fatal(err)
	}
	if len(packages) != 1 || packages[0] != (PackageUpgrade{Name: "dnsmasq", Installed: "2.91-r3", Available: "2.93-r1"}) {
		t.Fatalf("packages = %+v", packages)
	}
}

// TestFirmwareCheckDecodesTheRung: a device the check cannot answer for comes back
// as a named rung with the tool's own words, not as an error.
func TestFirmwareCheckDecodesTheRung(t *testing.T) {
	socket := helperReplying(t, "firmwareCheck",
		`{"status":0,"result":{"state":"unsupported","server":"","from":"","to":"","packages":0,"message":"File system type '(null)'"}}`)
	update, err := dialFirmwareCheck(socket)(context.Background(), "good-sid")
	if err != nil {
		t.Fatal(err)
	}
	if update.State != FirmwareUnsupported || update.Message != "File system type '(null)'" {
		t.Fatalf("update = %+v", update)
	}
}

// TestFirmwareUpgradeCarriesNothingButTheSession: the upgrade verb is
// argument-free by design. owut decides the version, the package set and the
// image on the device, so the request holds nothing a caller could steer
// (ADR-007) — the helper's own argv has no room for one either.
func TestFirmwareUpgradeCarriesNothingButTheSession(t *testing.T) {
	socket := helperReplying(t, "firmwareUpgrade", `{"status":0,"result":{"result":true}}`)
	if err := dialFirmwareUpgrade(socket)(context.Background(), "good-sid"); err != nil {
		t.Fatalf("firmware upgrade: %v", err)
	}
}

// TestFirmwareUpgradeReportsTheToolsOwnWords: an upgrade the update server
// refused comes back with owut's complaint intact, so the page can quote the
// tool rather than paraphrase it.
func TestFirmwareUpgradeReportsTheToolsOwnWords(t *testing.T) {
	socket := helperReplying(t, "firmwareUpgrade",
		`{"status":9,"error":"Update checks reveal errors, can't proceed"}`)
	err := dialFirmwareUpgrade(socket)(context.Background(), "good-sid")
	if err == nil || !strings.Contains(err.Error(), "Update checks reveal errors, can't proceed") {
		t.Fatalf("error = %v, want owut's own complaint", err)
	}
}

func TestCallHelperMapsPermissionDenied(t *testing.T) {
	socket := filepath.Join(t.TempDir(), "helper.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go func() {
		conn, acceptErr := listener.Accept()
		if acceptErr != nil {
			return
		}
		defer conn.Close()
		var request helperRequest
		_ = json.NewDecoder(conn).Decode(&request)
		_ = json.NewEncoder(conn).Encode(map[string]any{"status": 6, "error": "permission denied"})
	}()

	err = callHelper(context.Background(), socket, "pkgStatus", "bad-sid", nil, nil)
	if !errors.Is(err, ErrAccessDenied) {
		t.Fatalf("error = %v, want ErrAccessDenied", err)
	}
}
