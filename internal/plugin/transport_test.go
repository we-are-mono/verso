// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package plugin

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
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

// ownAccount is the Accounts double for plugins this test process serves:
// every plugin's account is the uid the test runs as, which is what the
// kernel reports behind serveUnix's sockets.
type ownAccount struct{}

func (ownAccount) UID(string) (uint32, error) { return uint32(os.Getuid()), nil }

// otherAccount names an account that is not the one listening: the uid beside
// the test's own, as a sibling plugin holding the path would be.
type otherAccount struct{}

func (otherAccount) UID(string) (uint32, error) { return uint32(os.Getuid()) + 1, nil }

// noAccount is a plugin with no account on the system.
type noAccount struct{}

func (noAccount) UID(id string) (uint32, error) { return 0, fmt.Errorf("no account for %q", id) }

func testTransport() *SocketTransport { return NewSocketTransport(ownAccount{}) }

// at is the endpoint of a test plugin listening on sock.
func at(sock string) Endpoint { return Endpoint{ID: "test", Socket: sock} }

// listenRecording stands in for whoever holds a plugin's path without being the
// plugin: it accepts every connection, never answers, and hands back each
// connection's bytes once the dialler has closed it.
func listenRecording(t *testing.T) (string, <-chan []byte) {
	t.Helper()
	sock := filepath.Join(t.TempDir(), "p.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("listen unix: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	received := make(chan []byte, 8)
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer func() { _ = conn.Close() }()
				_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
				data, _ := io.ReadAll(conn)
				received <- data
			}()
		}
	}()
	return sock, received
}

// TestSocketTransportRefusesAnotherAccountsListener: a listener at the
// manifest's path under any account but the plugin's own is sent nothing — no
// request line, no X-Verso-UCI or X-Verso-Ubus, no form — and nothing it could
// answer is taken for the plugin's. The kernel's word on who listens is the
// test; the path proves nothing, since whoever binds it while it is free
// answers there.
func TestSocketTransportRefusesAnotherAccountsListener(t *testing.T) {
	calls := map[string]func(*SocketTransport, Endpoint) error{
		"fetch": func(tr *SocketTransport, ep Endpoint) error {
			_, err := tr.Fetch(context.Background(), ep, Request{
				Method: http.MethodPost, Path: "/access",
				Form: url.Values{"authorized_key": {"what the operator typed"}},
				UCI:  UCI{"wireless": {"default_radio0": map[string]any{".type": "wifi-iface", "key": "not-a-real-psk"}}},
				Ubus: Ubus{"firewall.state": json.RawMessage(`{"version":1,"result":{"counters":[]}}`)},
			})
			return err
		},
		"describe": func(tr *SocketTransport, ep Endpoint) error {
			_, err := tr.Describe(context.Background(), ep,
				[]DescribeChange{{Config: "wireless", Op: "set", Section: "default_radio0", Option: "ssid", Value: "home"}},
				UCI{"wireless": {"default_radio0": map[string]any{".type": "wifi-iface", "key": "not-a-real-psk"}}})
			return err
		},
	}
	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			sock, received := listenRecording(t)
			err := call(NewSocketTransport(otherAccount{}), at(sock))
			if !errors.Is(err, errUntrustedPlugin) {
				t.Fatalf("err = %v, want the listener refused as another account's", err)
			}
			select {
			case data := <-received:
				if len(data) != 0 {
					t.Fatalf("another account's listener was sent %d bytes:\n%s", len(data), data)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("the dial never reached the listener")
			}
		})
	}
}

// TestSocketTransportRefusesAPluginWithNoAccount: a plugin whose account the
// system does not know has no one the shell could be talking to, so its socket
// is not even dialled.
func TestSocketTransportRefusesAPluginWithNoAccount(t *testing.T) {
	sock, received := listenRecording(t)
	if _, err := NewSocketTransport(noAccount{}).Fetch(context.Background(), at(sock),
		Request{Method: http.MethodGet, Path: "/"}); err == nil {
		t.Fatal("Fetch: want an error for a plugin with no account")
	}
	select {
	case data := <-received:
		t.Fatalf("a plugin with no account was dialled and sent %q", data)
	default:
	}
}

// TestSocketTransportAliveOnlyAsThePluginsAccount: a plugin is alive when its
// own account accepts a connection at its socket. Another account holding the
// path is not the plugin, so a squatter cannot make a plugin that is down look
// running; nor is a plugin with no account, or a path nothing listens on.
func TestSocketTransportAliveOnlyAsThePluginsAccount(t *testing.T) {
	sock, received := listenRecording(t)
	for name, tc := range map[string]struct {
		accounts Accounts
		socket   string
		want     bool
	}{
		"its own account":   {ownAccount{}, sock, true},
		"another account":   {otherAccount{}, sock, false},
		"no account":        {noAccount{}, sock, false},
		"nothing listening": {ownAccount{}, filepath.Join(t.TempDir(), "absent.sock"), false},
	} {
		t.Run(name, func(t *testing.T) {
			if got := NewSocketTransport(tc.accounts).Alive(at(tc.socket)); got != tc.want {
				t.Errorf("Alive = %v, want %v", got, tc.want)
			}
		})
	}
	// The probe only asks who listens: no listener, the plugin's own included,
	// is sent a byte.
	for range 2 {
		select {
		case data := <-received:
			if len(data) != 0 {
				t.Errorf("the probe sent %d bytes:\n%s", len(data), data)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("the probe never reached the listener")
		}
	}
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

	env, err := testTransport().Fetch(context.Background(), at(sock),
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

// TestSocketTransportDescribe proves a describe call reaches the plugin marked as
// one, carries the change list and the uci snapshot, and decodes the plain-language
// answer — including a sentence that folds several changes together.
func TestSocketTransportDescribe(t *testing.T) {
	var gotDescribe, gotSnapshot bool
	var gotBody describeRequest
	sock := serveUnix(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotDescribe = r.Header.Get(HeaderDescribe) != ""
		if raw := r.Header.Get(HeaderUCI); raw != "" {
			if dec, err := base64.StdEncoding.DecodeString(raw); err == nil {
				var snap UCI
				if json.Unmarshal(dec, &snap) == nil {
					_, gotSnapshot = snap["firewall"]
				}
			}
		}
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		envelopeJSON(w, http.StatusOK,
			`{"descriptions":[{"plain":"Added the rule “Allow DNS”.","covers":[0,1],"subject":{"kind":"rule","name":"Allow DNS"}}]}`)
	}))

	changes := []DescribeChange{
		{Config: "firewall", Op: "add-section", Section: "cfg99", Option: "rule"},
		{Config: "firewall", Op: "set", Section: "cfg99", Option: "name", Value: "Allow DNS"},
	}
	snap := UCI{"firewall": {"cfg99": map[string]any{".type": "rule"}}}
	descs, err := testTransport().Describe(context.Background(), at(sock), changes, snap)
	if err != nil {
		t.Fatalf("Describe: %v", err)
	}
	if !gotDescribe {
		t.Error("plugin did not see the X-Verso-Describe marker")
	}
	if !gotSnapshot {
		t.Error("plugin did not receive the uci snapshot")
	}
	if len(gotBody.Changes) != 2 || gotBody.Changes[0].Op != "add-section" {
		t.Errorf("plugin received changes = %+v", gotBody.Changes)
	}
	if len(descs) != 1 || descs[0].Plain != "Added the rule “Allow DNS”." {
		t.Fatalf("descriptions = %+v", descs)
	}
	if want := []int{0, 1}; len(descs[0].Covers) != 2 || descs[0].Covers[0] != want[0] || descs[0].Covers[1] != want[1] {
		t.Errorf("covers = %v, want %v", descs[0].Covers, want)
	}
	if got := descs[0].Subject; got == nil || *got != (Subject{Kind: "rule", Name: "Allow DNS"}) {
		t.Errorf("subject = %+v, want the rule Allow DNS", got)
	}
}

// TestSocketTransportDescribeBadStatus turns a non-200 into an error so the caller
// falls back to raw lines rather than rendering a broken drawer.
func TestSocketTransportDescribeBadStatus(t *testing.T) {
	sock := serveUnix(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	if _, err := testTransport().Describe(context.Background(), at(sock), nil, nil); err == nil {
		t.Fatal("Describe: want error on non-200, got nil")
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

	env, err := testTransport().Fetch(context.Background(), at(sock),
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

	env, err := testTransport().Fetch(context.Background(), at(sock), Request{
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

	env, err := testTransport().Fetch(context.Background(), at(sock), Request{
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
	env, err := testTransport().Fetch(context.Background(), at(sock),
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
		var state struct {
			Result struct {
				Counters []struct {
					Name    string `json:"name"`
					Packets int    `json:"packets"`
				} `json:"counters"`
			} `json:"result"`
		}
		if err := json.Unmarshal(reads["firewall.state"], &state); err != nil || len(state.Result.Counters) == 0 {
			http.Error(w, "bad counters", http.StatusBadRequest)
			return
		}
		counters := state.Result.Counters
		envelopeJSON(w, http.StatusOK, fmt.Sprintf(
			`{"schema_version":1,"title":"%s=%d","widget":{"type":"card"}}`,
			counters[0].Name, counters[0].Packets))
	}))

	env, err := testTransport().Fetch(context.Background(), at(sock), Request{
		Method: http.MethodGet, Path: "/",
		Ubus: Ubus{"firewall.state": json.RawMessage(
			`{"version":1,"result":{"counters":[{"chain":"input_wan","name":"Allow-Ping","packets":12}]}}`)},
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if want := "Allow-Ping=12"; env.Title != want {
		t.Errorf("brokered reads did not round-trip: title = %q, want %q", env.Title, want)
	}
}

// TestSocketTransportNamesTheInstalledPlugins: the ids of the plugins installed
// beside this one ride X-Verso-Plugins, comma-separated, so a plugin offers an
// act another plugin carries only where that plugin is there to carry it.
func TestSocketTransportNamesTheInstalledPlugins(t *testing.T) {
	sock := serveUnix(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		envelopeJSON(w, http.StatusOK, fmt.Sprintf(
			`{"schema_version":1,"title":%q,"widget":{"type":"card"}}`, r.Header.Get(HeaderPlugins)))
	}))
	env, err := testTransport().Fetch(context.Background(), at(sock), Request{
		Method: http.MethodGet, Path: "/", Plugins: []string{"acme", "dnsdhcp", "system"},
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if env.Title != "acme,dnsdhcp,system" {
		t.Errorf("X-Verso-Plugins = %q", env.Title)
	}
}

// Every request says which contract features the shell speaks, so a plugin
// knows whether the slots its page opens will be filled.
func TestSocketTransportNamesTheFeaturesTheShellSpeaks(t *testing.T) {
	sock := serveUnix(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		envelopeJSON(w, http.StatusOK, fmt.Sprintf(
			`{"schema_version":1,"title":%q,"widget":{"type":"card"}}`, r.Header.Get(HeaderFeatures)))
	}))
	env, err := testTransport().Fetch(context.Background(), at(sock), Request{Method: http.MethodGet, Path: "/"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if env.Title != strings.Join(Features, ",") || !strings.Contains(env.Title, "slots") {
		t.Errorf("X-Verso-Features = %q, want %q", env.Title, strings.Join(Features, ","))
	}
}

// A contribution is asked for with the slot it fills, so one endpoint can
// answer several slots.
func TestSocketTransportNamesTheSlotAContributionFills(t *testing.T) {
	sock := serveUnix(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		envelopeJSON(w, http.StatusOK, fmt.Sprintf(
			`{"schema_version":1,"title":%q,"widget":{"type":"card"}}`, r.Header.Get(HeaderSlot)))
	}))
	env, err := testTransport().Fetch(context.Background(), at(sock), Request{Method: http.MethodGet, Path: "/slots/use", Slot: "system.certificate.acts"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if env.Title != "system.certificate.acts" {
		t.Errorf("X-Verso-Slot = %q", env.Title)
	}
}

// A contribution with nothing to show answers 204: no envelope, no error.
func TestSocketTransportReadsNoContentAsNothingToShow(t *testing.T) {
	sock := serveUnix(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	env, err := testTransport().Fetch(context.Background(), at(sock), Request{Method: http.MethodGet, Path: "/slots/source", Slot: "system.certificate.source"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if !env.Empty {
		t.Errorf("env = %+v, want Empty", env)
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
	env, err := testTransport().Fetch(context.Background(), at(sock), Request{
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
	_, err := testTransport().Fetch(context.Background(),
		at(filepath.Join(t.TempDir(), "nope.sock")), Request{Method: http.MethodGet, Path: "/"})
	if err == nil {
		t.Fatal("Fetch: want error dialing a nonexistent socket")
	}
}

func TestSocketTransportNonJSONErrors(t *testing.T) {
	sock := serveUnix(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, "<html>not a schema</html>")
	}))
	if _, err := testTransport().Fetch(context.Background(), at(sock),
		Request{Method: http.MethodGet, Path: "/"}); err == nil {
		t.Fatal("Fetch: want error for a non-JSON body")
	}
}

func TestSocketTransportServerErrorIsError(t *testing.T) {
	sock := serveUnix(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	if _, err := testTransport().Fetch(context.Background(), at(sock),
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
	env, err := testTransport().Fetch(context.Background(), at(sock),
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
	if _, err := newSocketTransport(50*time.Millisecond, ownAccount{}).Fetch(context.Background(), at(sock),
		Request{Method: http.MethodGet, Path: "/"}); err == nil {
		t.Fatal("Fetch: want a timeout error")
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("Fetch did not time out promptly: %v", elapsed)
	}
}
