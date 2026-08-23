// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"testing"
	"time"
)

// serveUnix starts an HTTP server on a fresh unix socket and returns its path.
// It is the in-test stand-in for a plugin process — no device, fully hermetic.
func serveUnix(t *testing.T, h http.Handler) string {
	t.Helper()
	sock := filepath.Join(t.TempDir(), "p.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("listen unix: %v", err)
	}
	srv := &http.Server{Handler: h}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })
	return sock
}

func envelopeJSON(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = fmt.Fprint(w, body)
}

func TestSocketTransportFetchGET(t *testing.T) {
	sock := serveUnix(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		envelopeJSON(w, http.StatusOK,
			`{"schema_version":1,"title":"General","widget":{"type":"card","title":"Hostname"}}`)
	}))

	env, err := NewSocketTransport().Fetch(context.Background(), sock,
		Request{Method: http.MethodGet, Path: "/"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if env.SchemaVersion != 1 || env.Title != "General" {
		t.Errorf("envelope = %+v, want schema 1 / title General", env)
	}
	var head struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(env.Widget, &head); err != nil || head.Type != "card" {
		t.Errorf("widget = %s, want a card (err=%v)", env.Widget, err)
	}
}

// TestSocketTransportForwardsMethodPathForm proves the gateway's request reaches
// the plugin intact: method, sub-path, query, and form body.
func TestSocketTransportForwardsMethodPathForm(t *testing.T) {
	sock := serveUnix(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		title := fmt.Sprintf("%s %s hostname=%s q=%s",
			r.Method, r.URL.Path, r.PostForm.Get("hostname"), r.URL.Query().Get("q"))
		envelopeJSON(w, http.StatusOK,
			fmt.Sprintf(`{"schema_version":1,"title":%q,"widget":{"type":"card"}}`, title))
	}))

	env, err := NewSocketTransport().Fetch(context.Background(), sock, Request{
		Method: http.MethodPost,
		Path:   "/apply",
		Query:  url.Values{"q": {"1"}},
		Form:   url.Values{"hostname": {"router"}},
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if want := "POST /apply hostname=router q=1"; env.Title != want {
		t.Errorf("forwarding wrong: title = %q, want %q", env.Title, want)
	}
}

// TestSocketTransportDialRefusedErrors is the crash-isolation signal at the
// transport layer: a dead plugin surfaces as an error, which the gateway turns
// into "plugin unavailable" (asserted in the server package).
func TestSocketTransportDialRefusedErrors(t *testing.T) {
	_, err := NewSocketTransport().Fetch(context.Background(),
		filepath.Join(t.TempDir(), "nope.sock"), Request{Method: http.MethodGet, Path: "/"})
	if err == nil {
		t.Fatal("Fetch: want error dialing a nonexistent socket")
	}
}

func TestSocketTransportNonJSONErrors(t *testing.T) {
	sock := serveUnix(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, "<html>not a schema</html>")
	}))
	if _, err := NewSocketTransport().Fetch(context.Background(), sock,
		Request{Method: http.MethodGet, Path: "/"}); err == nil {
		t.Fatal("Fetch: want error for a non-JSON body")
	}
}

func TestSocketTransportServerErrorIsError(t *testing.T) {
	sock := serveUnix(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	if _, err := NewSocketTransport().Fetch(context.Background(), sock,
		Request{Method: http.MethodGet, Path: "/"}); err == nil {
		t.Fatal("Fetch: want error for a 5xx response")
	}
}

// TestSocketTransport422ReturnsEnvelope: a validation failure is a renderable
// form, not a transport error (ADR-006 §5).
func TestSocketTransport422ReturnsEnvelope(t *testing.T) {
	sock := serveUnix(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		envelopeJSON(w, http.StatusUnprocessableEntity,
			`{"schema_version":1,"title":"General","widget":{"type":"card"}}`)
	}))
	env, err := NewSocketTransport().Fetch(context.Background(), sock,
		Request{Method: http.MethodPost, Path: "/"})
	if err != nil {
		t.Fatalf("422 must return an envelope, got err: %v", err)
	}
	if env.Title != "General" {
		t.Errorf("envelope not parsed from 422: %+v", env)
	}
}

func TestSocketTransportTimeout(t *testing.T) {
	block := make(chan struct{})
	t.Cleanup(func() { close(block) })
	sock := serveUnix(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-block // hang past the client timeout
	}))

	start := time.Now()
	if _, err := newSocketTransport(50*time.Millisecond).Fetch(context.Background(), sock,
		Request{Method: http.MethodGet, Path: "/"}); err == nil {
		t.Fatal("Fetch: want a timeout error")
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("Fetch did not time out promptly: %v", elapsed)
	}
}
