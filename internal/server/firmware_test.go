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
		// the dialog says the trigger's words, where it is, and what fits
		"Upload a custom image…", ">Upload a custom image</h2>", "Drop a sysupgrade image here",
		`data-state="current" aria-current="step"`, ">Choose</span>", ">Verify</span>", ">Install</span>",
		"A sysupgrade image built for Mono Gateway DK · .bin, up to 128 MiB",
		`accept=".bin,application/octet-stream"`, `action="/system/maintenance/firmware"`,
		`data-verso-autosubmit`,
	} {
		if !strings.Contains(rr.Body.String(), want) {
			t.Errorf("Maintenance firmware picker missing %q", want)
		}
	}
}

// An image uploaded from the dialog is answered with the dialog's contents
// alone — the page around it is already on screen — on the step it reached:
// refused, it is back at Choose with the reason; verified, it is at Install.
func TestFirmwareUploadedFromTheDialogAnswersTheDialog(t *testing.T) {
	for name, tc := range map[string]struct {
		valid  bool
		status int
		want   []string
	}{
		"refused":  {false, http.StatusUnprocessableEntity, []string{"Firmware could not be verified", `aria-current="step" class="verso-step flex items-center gap-2"><span aria-hidden="true" class="verso-step-mark size-1.5 shrink-0 rounded-[1px]"></span><span>Choose</span>`}},
		"verified": {true, http.StatusOK, []string{"Firmware verified", "Install and restart", `aria-current="step" class="verso-step flex items-center gap-2"><span aria-hidden="true" class="verso-step-mark size-1.5 shrink-0 rounded-[1px]"></span><span>Install</span>`}},
	} {
		t.Run(name, func(t *testing.T) {
			srv := newServer(t, fakeBackend{
				board: openwrt.Board{Model: "Mono Gateway DK"},
				validateFirmware: func(context.Context, string, string) (openwrt.FirmwareInfo, error) {
					return openwrt.FirmwareInfo{Valid: tc.valid, Version: "25.12.5"}, nil
				},
			})
			token, sess := maintenanceSession(t, srv)
			var body bytes.Buffer
			mw := multipart.NewWriter(&body)
			_ = mw.WriteField("_csrf", sess.csrf)
			part, _ := mw.CreateFormFile("firmware_image", "image.bin")
			_, _ = part.Write([]byte("image"))
			_ = mw.Close()
			req := httptest.NewRequest(http.MethodPost, "/system/maintenance/firmware", &body)
			req.Header.Set("Content-Type", mw.FormDataContentType())
			req.Header.Set("X-Verso-Upload", "dialog")
			req.AddCookie(&http.Cookie{Name: sessionCookie, Value: token})
			rr := httptest.NewRecorder()
			srv.Handler().ServeHTTP(rr, req)
			got := rr.Body.String()
			if rr.Code != tc.status || strings.Contains(got, "<main") || strings.Contains(got, "x-teleport") {
				t.Fatalf("want the dialog's contents alone with %d, got %d:\n%s", tc.status, rr.Code, got)
			}
			for _, want := range append(tc.want, ">Upload a custom image</h2>") {
				if !strings.Contains(got, want) {
					t.Errorf("missing %q in:\n%s", want, got)
				}
			}
		})
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
	// The way back is the page's control-height primary, not a button sized by its padding.
	for _, want := range []string{"FIRMWARE INSTALLING", "Return to OpenWrt in about 5 minutes", "inline-flex h-control"} {
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
