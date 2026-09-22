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

func TestFirewallLogCarriesSIDAndIndependentCursor(t *testing.T) {
	for _, denied := range []bool{false, true} {
		socket := filepath.Join(t.TempDir(), "helper.sock")
		listener, err := net.Listen("unix", socket)
		if err != nil {
			t.Fatal(err)
		}
		done := make(chan error, 1)
		go func() {
			defer listener.Close()
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
			if request.Method != "firewallLog" || request.SID != "operator" || request.Args["generation"] != "epoch" || request.Args["after"] != "42" || request.Args["limit"] != "50" {
				done <- errors.New("lost operator or packet cursor")
				return
			}
			response := `{"status":0,"result":{"entries":[{"id":43,"time":1000,"msg":"packet"}],"generation":"epoch","available":true,"reset":false,"lost":true,"overruns":2}}`
			if denied {
				response = `{"status":6,"error":"access denied"}`
			}
			_, err = conn.Write([]byte(response + "\n"))
			done <- err
		}()
		result, err := dialFirewallLog(socket)(context.Background(), "operator", "epoch", 42, 50)
		if denied {
			if !errors.Is(err, ErrAccessDenied) {
				t.Fatalf("denial = %v", err)
			}
		} else if err != nil || !result.Available || !result.Lost || result.Overruns != 2 || len(result.Entries) != 1 || result.Entries[0].ID != 43 || result.Entries[0].Msg != "packet" {
			t.Fatalf("packet batch = %+v, %v", result, err)
		}
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	}
}
