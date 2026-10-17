// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package openwrt

import (
	"context"
	"reflect"
	"testing"
)

func TestSetThemeAsksTheHelperWithTheOperatorsSession(t *testing.T) {
	socket, got := helperOnce(t, `{"status":0,"result":{"result":true}}`)
	if err := (&NativeBackend{helperSocket: socket}).SetTheme(context.Background(), "operator", "catppuccin-mocha"); err != nil {
		t.Fatal(err)
	}
	request := <-got
	want := helperRequest{Method: "setTheme", SID: "operator", Args: map[string]string{"theme": "catppuccin-mocha"}}
	if !reflect.DeepEqual(request, want) {
		t.Errorf("request = %+v; want %+v", request, want)
	}
}

func TestSetThemeSaysWhenTheHelperRefuses(t *testing.T) {
	socket, _ := helperOnce(t, `{"status":2,"error":"invalid theme"}`)
	if err := (&NativeBackend{helperSocket: socket}).SetTheme(context.Background(), "operator", "night"); err == nil {
		t.Error("a refused theme reads as written")
	}
}
