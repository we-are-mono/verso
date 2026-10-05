// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package openwrt

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"path/filepath"
	"reflect"
	"testing"
)

// helperOnce answers one helper request with response and hands the request
// back for the test to read.
func helperOnce(t *testing.T, response string) (string, <-chan helperRequest) {
	t.Helper()
	socket := filepath.Join(t.TempDir(), "helper.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	got := make(chan helperRequest, 1)
	go func() {
		defer listener.Close()
		conn, err := listener.Accept()
		if err != nil {
			close(got)
			return
		}
		defer conn.Close()
		var request helperRequest
		_ = json.NewDecoder(conn).Decode(&request)
		got <- request
		_, _ = conn.Write([]byte(response + "\n"))
	}()
	return socket, got
}

func TestDiagnosticStartSendsTheRunsPartsAndItsOperator(t *testing.T) {
	socket, got := helperOnce(t, `{"status":0,"result":{"job":"abc123"}}`)
	job, err := dialDiagStart(socket)(context.Background(), "operator", DiagnosticRun{Tool: "ping", Target: "example.com", Interface: "br-lan", Family: "6"})
	if err != nil || job != "abc123" {
		t.Fatalf("start = %q, %v", job, err)
	}
	request := <-got
	want := map[string]string{"tool": "ping", "target": "example.com", "interface": "br-lan", "family": "6"}
	if request.Method != "diagStart" || request.SID != "operator" || !reflect.DeepEqual(request.Args, want) {
		t.Fatalf("request = %+v", request)
	}
}

func TestDiagnosticReadCarriesTheCursorAndTheEnding(t *testing.T) {
	socket, got := helperOnce(t, `{"status":0,"result":{"lines":["64 bytes from 1.1.1.1"],"next":3,"done":true,"code":0,"ended":"exited"}}`)
	out, err := dialDiagRead(socket)(context.Background(), "operator", "abc123", 2)
	if err != nil || !reflect.DeepEqual(out.Lines, []string{"64 bytes from 1.1.1.1"}) || out.Next != 3 || !out.Done || out.Code == nil || *out.Code != 0 || out.Ended != "exited" {
		t.Fatalf("read = %+v, %v", out, err)
	}
	request := <-got
	if request.Method != "diagRead" || request.Args["job"] != "abc123" || request.Args["after"] != "2" {
		t.Fatalf("request = %+v", request)
	}
}

func TestDiagnosticStopIsTheOperatorsAndADenialSaysSo(t *testing.T) {
	socket, got := helperOnce(t, `{"status":6,"error":"permission denied"}`)
	if err := dialDiagStop(socket)(context.Background(), "operator", "abc123"); !errors.Is(err, ErrAccessDenied) {
		t.Fatalf("stop = %v", err)
	}
	if request := <-got; request.Method != "diagStop" || request.SID != "operator" || request.Args["job"] != "abc123" {
		t.Fatalf("request = %+v", request)
	}
}

func TestDiagnosticRefusalKeepsTheHelpersWords(t *testing.T) {
	socket, _ := helperOnce(t, `{"status":2,"error":"no such interface"}`)
	_, err := dialDiagStart(socket)(context.Background(), "operator", DiagnosticRun{Tool: "ping", Target: "x", Interface: "eth9"})
	var refused interface{ ValidationMessage() string }
	if !errors.As(err, &refused) || refused.ValidationMessage() != "no such interface" {
		t.Fatalf("refusal = %v", err)
	}
}
