// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package plugin

import (
	"context"
	"encoding/base64"
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

// TestSocketTransportDecodesCommitShapes proves the three write shapes survive
// the wire: a set, a section delete, and a null that clears one option (which
// must arrive as a present key with a nil value, not as an absent key).
func TestSocketTransportDecodesCommitShapes(t *testing.T) {
	sock := serveUnix(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		envelopeJSON(w, http.StatusOK, `{"schema_version":1,"title":"Rules",
			"widget":{"type":"card"},
			"commit":[
				{"config":"firewall","section":"allow_ping","values":{"proto":["tcp","udp"],"dest_port":null}},
				{"config":"firewall","section":"block_telnet","delete":true}
			]}`)
	}))

	env, err := NewSocketTransport().Fetch(context.Background(), sock,
		Request{Method: http.MethodPost, Path: "/rules/allow_ping"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(env.Commit) != 2 {
		t.Fatalf("commit ops = %d, want 2", len(env.Commit))
	}
	set := env.Commit[0]
	if set.Delete {
		t.Errorf("a set op must not read as a delete: %+v", set)
	}
	value, present := set.Values["dest_port"]
	if !present || value != nil {
		t.Errorf("dest_port = %v (present=%v), want a present null", value, present)
	}
	if list, ok := set.Values["proto"].([]any); !ok || len(list) != 2 {
		t.Errorf("proto = %#v, want a two-item list", set.Values["proto"])
	}
	removed := env.Commit[1]
	if !removed.Delete || removed.Section != "block_telnet" || len(removed.Values) != 0 {
		t.Errorf("delete op = %+v, want a bare delete of block_telnet", removed)
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

// TestSocketTransportInjectsUCISnapshot proves the shell brokers a read snapshot
// to the plugin (ADR-007): buildRequest carries Request.UCI in the X-Verso-UCI
// header as base64 JSON, and a plugin decodes it the way a real plugin would —
// here reading network.wg0.proto back out of the snapshot.
func TestSocketTransportInjectsUCISnapshot(t *testing.T) {
	sock := serveUnix(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := base64.StdEncoding.DecodeString(r.Header.Get(HeaderUCI))
		if err != nil {
			http.Error(w, "bad header", http.StatusBadRequest)
			return
		}
		var snap UCI
		if err := json.Unmarshal(raw, &snap); err != nil {
			http.Error(w, "bad json", http.StatusBadRequest)
			return
		}
		wg0, _ := snap["network"]["wg0"].(map[string]any)
		proto, _ := wg0["proto"].(string)
		envelopeJSON(w, http.StatusOK,
			fmt.Sprintf(`{"schema_version":1,"title":%q,"widget":{"type":"card"}}`, proto))
	}))

	env, err := NewSocketTransport().Fetch(context.Background(), sock, Request{
		Method: http.MethodGet, Path: "/",
		UCI: UCI{"network": {"wg0": map[string]any{"proto": "wireguard"}}},
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if env.Title != "wireguard" {
		t.Errorf("snapshot did not round-trip: title = %q, want wireguard", env.Title)
	}
}

// TestSocketTransportOmitsSnapshotHeaderWhenEmpty: no declared reads, no header —
// the plugin sees the injection only when the shell actually brokered a read.
func TestSocketTransportOmitsSnapshotHeaderWhenEmpty(t *testing.T) {
	sock := serveUnix(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		absent := "false"
		if r.Header.Get(HeaderUCI) == "" {
			absent = "true"
		}
		envelopeJSON(w, http.StatusOK, fmt.Sprintf(
			`{"schema_version":1,"title":%q,"widget":{"type":"card"}}`, absent))
	}))
	env, err := NewSocketTransport().Fetch(context.Background(), sock,
		Request{Method: http.MethodGet, Path: "/"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if env.Title != "true" {
		t.Errorf("empty snapshot should set no header; title = %q, want true", env.Title)
	}
}

// TestSocketTransportInjectsBrokeredReads proves the second brokered channel
// (ADR-007): buildRequest carries Request.Ubus in the X-Verso-Ubus header as base64
// JSON keyed by helper function, and the results cross verbatim — the shell never
// reshapes what the helper returned.
func TestSocketTransportInjectsBrokeredReads(t *testing.T) {
	sock := serveUnix(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := base64.StdEncoding.DecodeString(r.Header.Get(HeaderUbus))
		if err != nil {
			http.Error(w, "bad header", http.StatusBadRequest)
			return
		}
		var reads Ubus
		if err := json.Unmarshal(raw, &reads); err != nil {
			http.Error(w, "bad json", http.StatusBadRequest)
			return
		}
		var counters struct {
			Counters []struct {
				Name    string `json:"name"`
				Packets int    `json:"packets"`
			} `json:"counters"`
		}
		if err := json.Unmarshal(reads["firewallCounters"], &counters); err != nil {
			http.Error(w, "bad counters", http.StatusBadRequest)
			return
		}
		envelopeJSON(w, http.StatusOK, fmt.Sprintf(
			`{"schema_version":1,"title":"%s=%d","widget":{"type":"card"}}`,
			counters.Counters[0].Name, counters.Counters[0].Packets))
	}))

	env, err := NewSocketTransport().Fetch(context.Background(), sock, Request{
		Method: http.MethodGet, Path: "/",
		Ubus: Ubus{"firewallCounters": json.RawMessage(
			`{"counters":[{"chain":"input_wan","name":"Allow-Ping","packets":12}]}`)},
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if want := "Allow-Ping=12"; env.Title != want {
		t.Errorf("brokered reads did not round-trip: title = %q, want %q", env.Title, want)
	}
}

// TestSocketTransportOmitsBrokeredHeaderWhenEmpty: nothing brokered, no header —
// a plugin sees the channel only when the shell actually performed a read.
func TestSocketTransportOmitsBrokeredHeaderWhenEmpty(t *testing.T) {
	sock := serveUnix(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		absent := "false"
		if r.Header.Get(HeaderUbus) == "" {
			absent = "true"
		}
		envelopeJSON(w, http.StatusOK, fmt.Sprintf(
			`{"schema_version":1,"title":%q,"widget":{"type":"card"}}`, absent))
	}))
	env, err := NewSocketTransport().Fetch(context.Background(), sock, Request{
		Method: http.MethodGet, Path: "/",
		UCI: UCI{"firewall": {"cfg01": map[string]any{".type": "defaults"}}},
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if env.Title != "true" {
		t.Errorf("no brokered reads should set no header; title = %q, want true", env.Title)
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
