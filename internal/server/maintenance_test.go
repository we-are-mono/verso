// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"io"
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

func maintenanceSession(t *testing.T, srv *Server) (string, session) {
	t.Helper()
	token, err := srv.sessions.CreateWithMetadata("maintenance-sid", "root", "", "")
	if err != nil {
		t.Fatal(err)
	}
	sess, ok := srv.sessions.get(token)
	if !ok {
		t.Fatal("created session disappeared")
	}
	return token, sess
}

func TestMaintenanceShowsFullBuildFacts(t *testing.T) {
	srv := newServer(t, fakeBackend{
		board: openwrt.Board{
			Firmware: "OpenWrt 25.12.4 r32933-4ccb782af7",
			// The condensed shape Board() returns: release + build stamp, no
			// builder address, no compiler pedigree.
			KernelBuild: "6.12.101 #1 SMP PREEMPT_DYNAMIC",
			Target:      "qualcommax/ipq807x",
		},
	})
	rr := get(t, srv, "/system/maintenance")
	// The ledger states each part once, trimmed to what identifies it: the
	// OpenWrt version apart from its revision, the kernel by its release.
	if strings.Contains(rr.Body.String(), "OpenWrt OpenWrt") || strings.Contains(rr.Body.String(), "PREEMPT_DYNAMIC") {
		t.Errorf("the ledger should not repeat the distribution name or carry the kernel's build flags:\n%s", rr.Body.String())
	}
	for _, want := range []string{
		">25.12.4<", "r32933-4ccb782af7",
		">6.12.101<",
		"qualcommax/ipq807x", "Download backup", "Restore a backup",
		"Drop an OpenWrt backup here", `x-show="idle"`, `data-verso-autosubmit`,
		`accept=".tar.gz,.tgz,.gz,application/gzip,application/x-gzip,application/x-compressed-tar"`,
	} {
		if !strings.Contains(rr.Body.String(), want) {
			t.Errorf("Maintenance page missing %q", want)
		}
	}
	// Maintenance is a page of immediate acts — each button names its own verb.
	// The automatic-check setting stages through its self-posting switch, so no
	// Save button belongs anywhere; one appearing means an immediate action
	// grew a stray generated submit.
	if n := strings.Count(rr.Body.String(), ">Save</button>"); n != 0 {
		t.Errorf("Maintenance rendered %d Save buttons, want none", n)
	}
	if strings.Contains(rr.Body.String(), ">Check backup</button>") {
		t.Error("Restore should submit on file selection, not require a second button")
	}
}

func TestBackupDownloadStreamsSysupgradeArchive(t *testing.T) {
	want := openWrtBackup(t)
	srv := newServer(t, fakeBackend{
		hn: "Mono Gateway",
		createBackup: func(_ context.Context, sid, filename string) error {
			if sid != "maintenance-sid" {
				t.Errorf("sid = %q", sid)
			}
			return os.WriteFile(filename, want, 0o600)
		},
	})
	token, _ := maintenanceSession(t, srv)
	req := httptest.NewRequest(http.MethodGet, "/system/maintenance/backup", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: token})
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", rr.Code, rr.Body.String())
	}
	if !bytes.Equal(rr.Body.Bytes(), want) {
		t.Error("download did not contain the helper-produced archive bytes")
	}
	if got := rr.Header().Get("Content-Disposition"); !strings.Contains(got, "mono-gateway-backup-") || !strings.Contains(got, ".tar.gz") {
		t.Errorf("Content-Disposition = %q", got)
	}
}

func TestRestoreIsVerifiedThenAppliedThroughBackend(t *testing.T) {
	var restored string
	srv := newServerFull(t, fakeBackend{
		restoreBackup: func(_ context.Context, sid, filename string) error {
			if sid != "maintenance-sid" {
				t.Errorf("sid = %q", sid)
			}
			if _, err := os.Stat(filename); err != nil {
				t.Errorf("restore file unavailable to backend: %v", err)
			}
			restored = filename
			return nil
		},
	}, &fakeTransport{}, nil, fakeAuth{sid: "maintenance-sid"})
	token, sess := maintenanceSession(t, srv)

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	if err := mw.WriteField("_csrf", sess.csrf); err != nil {
		t.Fatal(err)
	}
	part, err := mw.CreateFormFile("backup", "router-backup.tar.gz")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(openWrtBackup(t)); err != nil {
		t.Fatal(err)
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/system/maintenance/restore", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: token})
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("inspect status = %d; body=%s", rr.Code, rr.Body.String())
	}
	match := regexp.MustCompile(`name="restore_token" value="([0-9a-f]+)"`).FindStringSubmatch(rr.Body.String())
	if len(match) != 2 {
		t.Fatalf("verified response has no opaque restore token")
	}
	if !strings.Contains(rr.Body.String(), "Backup is ready") {
		t.Error("verified state was not rendered")
	}

	form := url.Values{"_csrf": {sess.csrf}, "restore_token": {match[1]}, "password": {"secret"}}
	apply := httptest.NewRequest(http.MethodPost, "/system/maintenance/restore/apply", strings.NewReader(form.Encode()))
	apply.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	apply.AddCookie(&http.Cookie{Name: sessionCookie, Value: token})
	applied := httptest.NewRecorder()
	srv.Handler().ServeHTTP(applied, apply)
	if applied.Code != http.StatusOK {
		t.Fatalf("apply status = %d; body=%s", applied.Code, applied.Body.String())
	}
	if restored == "" {
		t.Fatal("restore backend was not called")
	}
	assertRestartingTakeover(t, applied.Body.String(), "Restoring your backup", `data-verso-restarting-budget="300"`)
	if _, err := os.Stat(restored); !os.IsNotExist(err) {
		t.Errorf("consumed upload still exists: %v", err)
	}

	// The verified upload is one-shot: replaying the same token must not run the
	// privileged restore a second time.
	restored = ""
	replay := httptest.NewRequest(http.MethodPost, "/system/maintenance/restore/apply", strings.NewReader(form.Encode()))
	replay.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	replay.AddCookie(&http.Cookie{Name: sessionCookie, Value: token})
	replayed := httptest.NewRecorder()
	srv.Handler().ServeHTTP(replayed, replay)
	if replayed.Code == http.StatusOK || restored != "" {
		t.Errorf("replaying a used restore token ran the backend again (code=%d, restored=%q)", replayed.Code, restored)
	}
}

func TestRestoreBackendFailureIsAModalErrorNotAFieldError(t *testing.T) {
	srv := newServer(t, fakeBackend{})
	var body strings.Builder
	err := srv.widgets.RenderWithToken(&body, srv.restoreModal(restoreState{
		open: true, verified: true, token: "opaque", name: "backup.tar.gz", entries: 12,
		restoreError: "OpenWrt could not complete the restore.",
	}), "csrf", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	got := body.String()
	for _, want := range []string{"Restore did not complete", "OpenWrt could not complete the restore.", "backup.tar.gz"} {
		if !strings.Contains(got, want) {
			t.Errorf("restore error modal missing %q", want)
		}
	}
	if strings.Contains(got, `focus:ring-red-600/20`) {
		t.Error("backend restore failure was incorrectly attached to the password field")
	}
}

func openWrtBackup(t *testing.T) []byte {
	t.Helper()
	var body bytes.Buffer
	gz := gzip.NewWriter(&body)
	tw := tar.NewWriter(gz)
	data := []byte("config system 'system'\n")
	if err := tw.WriteHeader(&tar.Header{Name: "etc/config/system", Mode: 0o600, Size: int64(len(data))}); err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(tw, bytes.NewReader(data)); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return body.Bytes()
}
