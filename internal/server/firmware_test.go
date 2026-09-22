// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"bytes"
	"context"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/we-are-mono/verso/internal/openwrt"
)

func TestMaintenanceFirmwarePickerMatchesSysupgradeImages(t *testing.T) {
	srv := newServer(t, fakeBackend{board: openwrt.Board{Model: "Mono Gateway DK"}})
	rr := get(t, srv, "/system/maintenance")
	for _, want := range []string{
		"Upload a custom image…", "Install firmware", "Drop a sysupgrade image here",
		`accept=".bin,application/octet-stream"`, `action="/system/maintenance/firmware"`,
		`data-verso-autosubmit`,
	} {
		if !strings.Contains(rr.Body.String(), want) {
			t.Errorf("Maintenance firmware picker missing %q", want)
		}
	}
}

func TestFirmwareIsDeviceVerifiedThenInstalled(t *testing.T) {
	var installed string
	info := openwrt.FirmwareInfo{
		Valid: true, Version: "25.12.5", Revision: "r123-abc",
		Target: "layerscape/armv8_64b", Board: "mono_gateway-dk",
	}
	srv := newServerFull(t, fakeBackend{
		board: openwrt.Board{Model: "Mono Gateway DK"},
		validateFirmware: func(_ context.Context, sid, filename string) (openwrt.FirmwareInfo, error) {
			if sid != "maintenance-sid" {
				t.Errorf("validate sid = %q", sid)
			}
			if _, err := os.Stat(filename); err != nil {
				t.Errorf("firmware unavailable during validation: %v", err)
			}
			return info, nil
		},
		installFirmware: func(_ context.Context, sid, filename string) error {
			if sid != "maintenance-sid" {
				t.Errorf("install sid = %q", sid)
			}
			if _, err := os.Stat(filename); err != nil {
				t.Errorf("firmware unavailable during install: %v", err)
			}
			installed = filename
			return nil
		},
	}, &fakeTransport{}, nil, fakeAuth{sid: "maintenance-sid"})
	token, sess := maintenanceSession(t, srv)

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	if err := mw.WriteField("_csrf", sess.csrf); err != nil {
		t.Fatal(err)
	}
	part, err := mw.CreateFormFile("firmware_image", "mono-gateway-sysupgrade.bin")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte("test sysupgrade image")); err != nil {
		t.Fatal(err)
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/system/maintenance/firmware", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: token})
	verified := httptest.NewRecorder()
	srv.Handler().ServeHTTP(verified, req)
	if verified.Code != http.StatusOK {
		t.Fatalf("verify status = %d; body=%s", verified.Code, verified.Body.String())
	}
	for _, want := range []string{
		"Firmware verified", "Mono Gateway DK", "25.12.5", "r123-abc",
		"Keep current settings", "Install and restart", "Do not disconnect its power",
	} {
		if !strings.Contains(verified.Body.String(), want) {
			t.Errorf("verified firmware modal missing %q", want)
		}
	}
	match := regexp.MustCompile(`name="firmware_token" value="([0-9a-f]+)"`).FindStringSubmatch(verified.Body.String())
	if len(match) != 2 {
		t.Fatal("verified response has no opaque firmware token")
	}

	form := url.Values{"_csrf": {sess.csrf}, "firmware_token": {match[1]}, "password": {"secret"}}
	apply := httptest.NewRequest(http.MethodPost, "/system/maintenance/firmware/apply", strings.NewReader(form.Encode()))
	apply.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	apply.AddCookie(&http.Cookie{Name: sessionCookie, Value: token})
	applied := httptest.NewRecorder()
	srv.Handler().ServeHTTP(applied, apply)
	if applied.Code != http.StatusOK {
		t.Fatalf("install status = %d; body=%s", applied.Code, applied.Body.String())
	}
	// The way back is the page's 36px primary, not a button sized by its padding.
	for _, want := range []string{"FIRMWARE INSTALLING", "Return to OpenWrt in about 5 minutes", "inline-flex h-9"} {
		if !strings.Contains(applied.Body.String(), want) {
			t.Errorf("firmware started page missing %q", want)
		}
	}
	if got := applied.Header().Get("Retry-After"); got != "300" {
		t.Errorf("Retry-After = %q, want 300", got)
	}
	if installed == "" {
		t.Fatal("firmware backend was not called")
	}
	if _, err := os.Stat(installed); !os.IsNotExist(err) {
		t.Errorf("consumed firmware upload still exists: %v", err)
	}
}

func TestRejectedFirmwareStaysInTheModalAndIsRemoved(t *testing.T) {
	var uploaded string
	srv := newServer(t, fakeBackend{
		board: openwrt.Board{Model: "Mono Gateway DK"},
		validateFirmware: func(_ context.Context, _ string, filename string) (openwrt.FirmwareInfo, error) {
			uploaded = filename
			return openwrt.FirmwareInfo{Valid: false, Error: "Device x86/64 not supported"}, nil
		},
	})
	token, sess := maintenanceSession(t, srv)
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	_ = mw.WriteField("_csrf", sess.csrf)
	part, _ := mw.CreateFormFile("firmware_image", "wrong-device.bin")
	_, _ = part.Write([]byte("not for this device"))
	_ = mw.Close()
	req := httptest.NewRequest(http.MethodPost, "/system/maintenance/firmware", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: token})
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d; body=%s", rr.Code, rr.Body.String())
	}
	for _, want := range []string{"Firmware could not be verified", "different device", "Drop a sysupgrade image here"} {
		if !strings.Contains(rr.Body.String(), want) {
			t.Errorf("rejected modal missing %q", want)
		}
	}
	if uploaded == "" {
		t.Fatal("validator did not receive uploaded firmware")
	}
	if _, err := os.Stat(uploaded); !os.IsNotExist(err) {
		t.Errorf("rejected firmware still exists: %v", err)
	}
}
