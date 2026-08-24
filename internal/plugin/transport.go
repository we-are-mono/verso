// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package plugin

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// HeaderUCI carries the uci read snapshot the shell brokers to a plugin (ADR-007),
// as base64-encoded JSON. base64 keeps arbitrary config values — non-ASCII, or
// punctuation a raw header would mangle — intact across the header.
const HeaderUCI = "X-Verso-UCI"

// maxEnvelopeBytes bounds how much a plugin can return, so a misbehaving plugin
// cannot exhaust shell memory. Widget schema for an admin page is tiny; a
// megabyte is generous.
const maxEnvelopeBytes = 1 << 20

// Request is what the shell forwards to a plugin over its socket: the browser's
// method, sub-path (below the /plugins/<id>/ mount), query, any form body, and the
// uci read snapshot the shell brokered on the plugin's behalf (ADR-007).
type Request struct {
	Method string
	Path   string
	Query  map[string][]string
	Form   map[string][]string
	UCI    UCI
}

// UCI is the read snapshot the shell injects into a plugin request (ADR-007):
// config name → the rpcd `uci get` values for that config (section name → section
// table, carrying its `.type`/`.name` meta and its options). The shell reads each
// config the plugin declared in acl.read with the operator's sid and hands the
// result down, so a session-less plugin renders from config without linking a uci
// library or reading /etc/config. A list option is a JSON array; a scalar is a
// string.
type UCI map[string]map[string]any

// Envelope is a plugin's schema response (ADR-006 §4). Widget is the raw schema
// subtree, decoded by the widget package — the transport stays ignorant of the
// widget vocabulary. Status is the HTTP status the plugin returned (200 or 422),
// set by the transport so the gateway can propagate a validation failure to the
// browser; it is not part of the plugin's JSON.
type Envelope struct {
	SchemaVersion int             `json:"schema_version"`
	Title         string          `json:"title"`
	Width         string          `json:"width"` // page width preset: "narrow" | "normal" (default) | "wide"
	Widget        json.RawMessage `json:"widget"`
	Commit        []CommitOp      `json:"commit"`
	Status        int             `json:"-"`
}

// CommitOp is one declarative uci write a plugin asks the shell to perform on its
// behalf (ADR-007). A de-privileged plugin holds no write access and no
// session; it returns intents, and the shell executes them through rpcd with the
// operator's sid — but only for a config the plugin declared in its manifest acl,
// so a plugin cannot broker a write outside its declared surface.
type CommitOp struct {
	Config  string         `json:"config"`
	Section string         `json:"section"`
	Values  map[string]any `json:"values"` // option → value; a value may be a string or a list of strings (uci list option)
}

// Transport exchanges a request with a plugin and returns its schema envelope.
// It is the injected seam (ADR-003): the shell depends on this interface, tests
// supply a fake, production dials the unix socket. Implementations MUST bound
// the call in time so a hung plugin cannot wedge the shell.
type Transport interface {
	Fetch(ctx context.Context, socket string, req Request) (*Envelope, error)
}

// SocketTransport speaks HTTP/1.1 to a plugin over its unix domain socket. It
// dials per call (no pooling) — simple and correct for admin-UI request rates;
// a pool is a later optimization behind this same interface.
type SocketTransport struct {
	timeout time.Duration
}

// NewSocketTransport returns a transport with the default per-call timeout.
func NewSocketTransport() *SocketTransport { return newSocketTransport(5 * time.Second) }

func newSocketTransport(timeout time.Duration) *SocketTransport {
	return &SocketTransport{timeout: timeout}
}

// Fetch dials socket, issues req as an HTTP request, and decodes the JSON schema
// envelope. A 200 or a 422 both carry a renderable envelope (ADR-006 §5); any
// other status, a dial failure, a timeout, or a non-JSON body is an error — the
// gateway turns that into a contained "plugin unavailable" state, never a 500.
func (t *SocketTransport) Fetch(ctx context.Context, socket string, req Request) (*Envelope, error) {
	client := &http.Client{
		Timeout: t.timeout,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				var d net.Dialer
				return d.DialContext(ctx, "unix", socket)
			},
			DisableKeepAlives: true,
		},
	}

	httpReq, err := t.buildRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("plugin: fetch %s: %w", socket, err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK, http.StatusUnprocessableEntity:
		// Both carry a renderable schema envelope.
	default:
		return nil, fmt.Errorf("plugin: %s returned status %d", socket, resp.StatusCode)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxEnvelopeBytes))
	if err != nil {
		return nil, fmt.Errorf("plugin: read %s: %w", socket, err)
	}
	var env Envelope
	if err := json.Unmarshal(data, &env); err != nil {
		return nil, fmt.Errorf("plugin: decode envelope from %s: %w", socket, err)
	}
	env.Status = resp.StatusCode
	return &env, nil
}

// buildRequest turns a plugin Request into an HTTP request. The host is a fixed
// placeholder — the unix dialer ignores it — but must be a valid URL.
func (t *SocketTransport) buildRequest(ctx context.Context, req Request) (*http.Request, error) {
	method := req.Method
	if method == "" {
		method = http.MethodGet
	}

	u := "http://plugin" + normalizePath(req.Path)
	if len(req.Query) > 0 {
		u += "?" + encodeValues(req.Query)
	}

	var body io.Reader
	var contentType string
	if method != http.MethodGet && method != http.MethodHead && len(req.Form) > 0 {
		body = strings.NewReader(encodeValues(req.Form))
		contentType = "application/x-www-form-urlencoded"
	}

	httpReq, err := http.NewRequestWithContext(ctx, method, u, body)
	if err != nil {
		return nil, fmt.Errorf("plugin: build request: %w", err)
	}
	if contentType != "" {
		httpReq.Header.Set("Content-Type", contentType)
	}
	httpReq.Header.Set("Accept", "application/json")

	// The read snapshot rides a header, orthogonal to method/query/form: it reaches
	// a GET as readily as a POST, and never disturbs the plugin's form parsing.
	if len(req.UCI) > 0 {
		snapshot, err := json.Marshal(req.UCI)
		if err != nil {
			return nil, fmt.Errorf("plugin: encode uci snapshot: %w", err)
		}
		httpReq.Header.Set(HeaderUCI, base64.StdEncoding.EncodeToString(snapshot))
	}
	return httpReq, nil
}

func normalizePath(p string) string {
	if p == "" {
		return "/"
	}
	if !strings.HasPrefix(p, "/") {
		return "/" + p
	}
	return p
}

func encodeValues(v map[string][]string) string {
	return url.Values(v).Encode()
}
