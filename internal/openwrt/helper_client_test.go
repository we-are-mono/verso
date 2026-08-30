// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package openwrt

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"path/filepath"
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
