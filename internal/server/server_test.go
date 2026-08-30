// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/we-are-mono/verso/internal/openwrt"
	"github.com/we-are-mono/verso/internal/plugin"
	"github.com/we-are-mono/verso/internal/sysstat"
	"github.com/we-are-mono/verso/internal/telemetry"
	"github.com/we-are-mono/verso/internal/widget"
)

// fakeBackend is a Backend seam double: no ubus/uci, no device needed. access and
// accessErr drive the plugin-write authorization gate (ADR-007) in tests.
type fakeBackend struct {
	si             openwrt.SystemInfo
	board          openwrt.Board
	v6Leases       []openwrt.V6Lease
	hn             string
	rootNoPassword bool // RootHasPassword returns !rootNoPassword (default: root has a password)
	err            error
	access         bool
	accessErr      error
	uciErr         error                     // returned by UCISet/UCICommit
	writes         *[]uciWrite               // records UCISet calls (pointer: fakeBackend is used by value)
	uci            map[string]map[string]any // per-config read snapshots UCIConfig returns
	addReturns     string                    // section id UCIAdd returns
	adds           *[]string                 // records "config secType" per UCIAdd (pointer: fakeBackend is by value)
	deletes        *[]string                 // records "config.section" per UCIDelete
	// setPassword backs SetPassword — tests inject it to capture the sid/username/
	// password or return an error. Nil means "succeed silently".
	setPassword   func(ctx context.Context, sid, username, password string) error
	setSystemTime func(ctx context.Context, sid, datetime, timezone string) error
	// The uci two-phase lifecycle (ADR-010): canned pending changes, and records
	// of what the shell committed, applied, confirmed, or reverted.
	changes    map[string][][]string
	changesErr error
	commits    *[]string // records committed configs (pointer: fakeBackend is by value)
	reverts    *[]string // records reverted configs
	applies    *[]int    // records UCIApply rollback timeouts
	confirms   *int      // counts UCIConfirm calls
	// procd's rc view (ADR-011): canned per-service states, and records of the
	// lifecycle actions the shell forwarded.
	rcStates map[string]openwrt.RCState
	rcErr    error
	rcInits  *[]string // records "service action" per RCInit
	// The helper's package verbs (ADR-011 §4): canned search results and
	// records of what the shell installed, removed, or refreshed.
	pkgCheckedAt     int64
	pkgFound         []openwrt.Package
	pkgInstalledList []openwrt.Package
	pkgTotal         int
	pkgErr           error
	pkgUpdates       *int      // counts PkgUpdate calls
	pkgInstalls      *[]string // records installed names
	pkgRemoves       *[]string // records removed names
	// The wan side of the overview meters: canned uplink state and a queue of
	// device-counter snapshots, popped one per DeviceStats call (pointer:
	// fakeBackend is used by value); the last snapshot repeats.
	wan      openwrt.WANState
	wanConn  openwrt.WANConn
	wanErr   error
	wanCalls *int
	devStats *[]openwrt.DeviceStats
	devErr   error
}

func (f fakeBackend) WANStatus(context.Context, string) (openwrt.WANState, error) {
	if f.wanCalls != nil {
		*f.wanCalls++
	}
	return f.wan, f.wanErr
}

func (f fakeBackend) WANConn(context.Context, string) (openwrt.WANConn, error) {
	return f.wanConn, f.wanErr
}

func (f fakeBackend) IPv6Leases(context.Context, string) ([]openwrt.V6Lease, error) {
	return f.v6Leases, nil
}

func (f fakeBackend) DeviceStats(context.Context, string, string) (openwrt.DeviceStats, error) {
	if f.devErr != nil {
		return openwrt.DeviceStats{}, f.devErr
	}
	if f.devStats == nil || len(*f.devStats) == 0 {
		return openwrt.DeviceStats{}, nil
	}
	st := (*f.devStats)[0]
	if len(*f.devStats) > 1 {
		*f.devStats = (*f.devStats)[1:]
	}
	return st, nil
}

func (f fakeBackend) SystemInfo(context.Context, string) (openwrt.SystemInfo, error) {
	return f.si, f.err
}

func (f fakeBackend) Board(context.Context, string) (openwrt.Board, error) {
	return f.board, f.err
}

func (f fakeBackend) Hostname(context.Context, string) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	return f.hn, nil
}

func (f fakeBackend) Access(context.Context, string, string, string, string) (bool, error) {
	return f.access, f.accessErr
}

// uciWrite records a UCISet call so the broker tests can assert what the shell
// wrote on the plugin's behalf (ADR-007).
type uciWrite struct {
	sid, config, section string
	values               map[string]any
}

func (f fakeBackend) UCISet(_ context.Context, sid, config, section string, values map[string]any) error {
	if f.writes != nil {
		*f.writes = append(*f.writes, uciWrite{sid, config, section, values})
	}
	return f.uciErr
}

func (f fakeBackend) UCICommit(_ context.Context, _, config string) error {
	if f.commits != nil {
		*f.commits = append(*f.commits, config)
	}
	return f.uciErr
}

// UCIConfig returns the canned read snapshot for a config (the shell brokers
// plugin reads through rpcd, ADR-007). An absent config yields an empty snapshot,
// mirroring an operator who may not read it.
func (f fakeBackend) UCIConfig(_ context.Context, _, config string) (map[string]any, error) {
	return f.uci[config], nil
}

// UCIAdd/UCIDelete record what the shell asked rpcd to do to realize a repeater's
// add/remove (ADR-005 §7), so the broker tests can assert it.
func (f fakeBackend) UCIAdd(_ context.Context, _, config, secType string) (string, error) {
	if f.adds != nil {
		*f.adds = append(*f.adds, config+" "+secType)
	}
	return f.addReturns, f.uciErr
}

func (f fakeBackend) UCIDelete(_ context.Context, _, config, section string) error {
	if f.deletes != nil {
		*f.deletes = append(*f.deletes, config+"."+section)
	}
	return f.uciErr
}

func (f fakeBackend) UCIChanges(context.Context, string) (map[string][][]string, error) {
	return f.changes, f.changesErr
}

func (f fakeBackend) UCIRevert(_ context.Context, _, config string) error {
	if f.reverts != nil {
		*f.reverts = append(*f.reverts, config)
	}
	return f.uciErr
}

func (f fakeBackend) UCIApply(_ context.Context, _ string, timeout int) error {
	if f.applies != nil {
		*f.applies = append(*f.applies, timeout)
	}
	return f.uciErr
}

func (f fakeBackend) UCIConfirm(context.Context, string) error {
	if f.confirms != nil {
		*f.confirms++
	}
	return f.uciErr
}

func (f fakeBackend) RCList(context.Context, string) (map[string]openwrt.RCState, error) {
	return f.rcStates, f.rcErr
}

func (f fakeBackend) RCInit(_ context.Context, _ string, name, action string) error {
	if f.rcInits != nil {
		*f.rcInits = append(*f.rcInits, name+" "+action)
	}
	return f.rcErr
}

func (f fakeBackend) PkgStatus(context.Context, string) (int64, error) {
	return f.pkgCheckedAt, f.pkgErr
}

func (f fakeBackend) PkgUpdate(context.Context, string) error {
	if f.pkgUpdates != nil {
		*f.pkgUpdates++
	}
	return f.pkgErr
}

func (f fakeBackend) PkgSearch(context.Context, string, string) ([]openwrt.Package, int, error) {
	return f.pkgFound, f.pkgTotal, f.pkgErr
}

func (f fakeBackend) PkgInstalled(context.Context, string) ([]openwrt.Package, error) {
	return f.pkgInstalledList, f.pkgErr
}

func (f fakeBackend) PkgInstall(_ context.Context, _ string, name string) error {
	if f.pkgInstalls != nil {
		*f.pkgInstalls = append(*f.pkgInstalls, name)
	}
	return f.pkgErr
}

func (f fakeBackend) PkgRemove(_ context.Context, _ string, name string) error {
	if f.pkgRemoves != nil {
		*f.pkgRemoves = append(*f.pkgRemoves, name)
	}
	return f.pkgErr
}

func (f fakeBackend) SetPassword(ctx context.Context, sid, username, password string) error {
	if f.setPassword != nil {
		return f.setPassword(ctx, sid, username, password)
	}
	return nil
}

func (f fakeBackend) SetSystemTime(ctx context.Context, sid, datetime, timezone string) error {
	if f.setSystemTime != nil {
		return f.setSystemTime(ctx, sid, datetime, timezone)
	}
	return nil
}

func (f fakeBackend) RootHasPassword(context.Context, string) (bool, error) {
	return !f.rootNoPassword, nil
}

// fakeTransport is the plugin-transport seam double (ADR-003/006): it returns a
// canned envelope or error and records the request the gateway forwarded, so the
// gateway is testable with no plugin process and no socket.
type fakeTransport struct {
	env        *plugin.Envelope
	err        error
	lastSocket string
	lastReq    plugin.Request
}

func (f *fakeTransport) Fetch(_ context.Context, socket string, req plugin.Request) (*plugin.Envelope, error) {
	f.lastSocket = socket
	f.lastReq = req
	return f.env, f.err
}

// fakeAuth is the authenticator seam double (ADR-003): no ubus, no device.
type fakeAuth struct {
	sid       string
	err       error
	verifyErr error
}

func (f fakeAuth) Login(context.Context, string, string) (string, error) { return f.sid, f.err }
func (f fakeAuth) Verify(context.Context, string, string) error          { return f.verifyErr }

func newServerFull(t *testing.T, backend openwrt.Backend, tr plugin.Transport, manifests []plugin.Manifest, auth Authenticator) *Server {
	t.Helper()
	r, err := widget.NewRenderer()
	if err != nil {
		t.Fatalf("widget.NewRenderer: %v", err)
	}
	s, err := New(r, backend, tr, manifests, auth)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(s.Close)
	// Tests have no real plugin sockets; default to alive so nav and pages
	// render fully. Liveness-specific tests override s.probe themselves.
	s.probe = func(string) bool { return true }
	// Tests read no real kernel either: canned box health (meters-specific
	// tests substitute their own), and no roster files (roster tests supply
	// theirs).
	s.stats = fakeStats{cpu: 18, root: sysstat.Storage{Used: 23 << 30, Free: 9 << 30}}
	s.readLeases = func() ([]byte, error) { return nil, errors.New("no leases in tests") }
	s.neighbors = func() ([]sysstat.Neighbor, error) {
		return nil, errors.New("no neighbour table in tests")
	}
	s.bridgePorts = func() (map[string]string, error) { return nil, errors.New("no fdb in tests") }
	return s
}

func newServerWith(t *testing.T, backend openwrt.Backend, tr plugin.Transport, manifests []plugin.Manifest) *Server {
	return newServerFull(t, backend, tr, manifests, fakeAuth{sid: "test-sid"})
}

func newServer(t *testing.T, backend openwrt.Backend) *Server {
	return newServerWith(t, backend, &fakeTransport{}, nil)
}

func demoManifest() plugin.Manifest {
	return plugin.Manifest{
		ManifestVersion: 1, ID: "demo", Name: "Demo Plugin",
		Socket: "/run/verso/demo.sock", SchemaVersion: 1,
		Nav: []plugin.NavEntry{{Section: "Apps", Label: "Demo", Path: "/"}},
	}
}

func get(t *testing.T, srv *Server, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	// Authenticate by default: mint a session and attach its cookie, so the
	// behavior tests exercise the page rather than the login redirect.
	token, err := srv.sessions.CreateWithMetadata("test-sid", "root", "", "")
	if err != nil {
		t.Fatalf("session: %v", err)
	}
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: token})
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	return rec
}

func postForm(t *testing.T, srv *Server, path string, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	return rec
}

// postPlugin issues an authenticated, CSRF-valid POST to a plugin path — the
// path a real browser save takes — so the ACL gate (ADR-007) is exercised end to
// end rather than short-circuited by the auth or CSRF middleware.
func postPlugin(t *testing.T, srv *Server, path string, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	token, err := srv.sessions.CreateWithMetadata("test-sid", "root", "", "")
	if err != nil {
		t.Fatalf("session: %v", err)
	}
	sess, _ := srv.sessions.get(token)
	if form == nil {
		form = url.Values{}
	}
	form.Set("_csrf", sess.csrf)
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: token})
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	return rec
}

// TestPluginSubpageBar: a plugin's declared subpages render as the shell's top
// bar — shell-built hrefs, the active tab marked from the request path.
func TestPluginSubpageBar(t *testing.T) {
	tr := &fakeTransport{env: &plugin.Envelope{
		SchemaVersion: 1, Title: "DNS & DHCP", Status: http.StatusOK,
		Pages: []plugin.PageTab{
			{Label: "Leases", Path: "dnsdhcp"},
			{Label: "Configuration", Path: "dnsdhcp/config"},
		},
		Widget: json.RawMessage(`{"type":"card","children":[]}`),
	}}
	s := newServerWith(t, fakeBackend{}, tr, []plugin.Manifest{demoManifest()})

	body := get(t, s, "/plugins/demo/dnsdhcp").Body.String()
	for _, want := range []string{
		`aria-label="Subpages"`,
		`bg-white px-5 md:sticky`,
		`dark:bg-menu-interaction`,
		`dark:hover:bg-menu-interaction dark:active:bg-menu-interaction`,
		`href="/plugins/demo/dnsdhcp"`,
		`href="/plugins/demo/dnsdhcp/config"`,
		`aria-current="page"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("subpage bar missing %q", want)
		}
	}
	// The active marker sits on the Leases tab (the requested path), not Configuration.
	if !strings.Contains(body, `href="/plugins/demo/dnsdhcp"`+" aria-current") &&
		!strings.Contains(body, `href="/plugins/demo/dnsdhcp" aria-current="page"`) {
		t.Errorf("active tab not marked on the requested path:\n%s", body)
	}
	if strings.Contains(body, `href="/plugins/demo/dnsdhcp/config" aria-current`) {
		t.Error("the inactive tab must not carry aria-current")
	}
}

// TestPluginKickerStatus: a page may mark a styleguide/reference state beside
// its kicker without burying that state in the lede.
func TestPluginKickerStatus(t *testing.T) {
	tr := &fakeTransport{env: &plugin.Envelope{
		SchemaVersion: 1, Title: "System", Kicker: "Styleguide", KickerStatus: "Complete",
		Widget: json.RawMessage(`{"type":"card","children":[]}`),
	}}
	s := newServerWith(t, fakeBackend{}, tr, []plugin.Manifest{demoManifest()})

	body := get(t, s, "/plugins/demo/").Body.String()
	for _, want := range []string{"Styleguide", "· Complete", "text-emerald-600"} {
		if !strings.Contains(body, want) {
			t.Errorf("kicker status missing %q", want)
		}
	}
}

func demoACLManifest() plugin.Manifest {
	m := demoManifest()
	m.ACL = plugin.ACL{Write: []plugin.ACLScope{{Scope: "uci", Object: "system", Function: "write"}}}
	return m
}

func demoReadManifest() plugin.Manifest {
	m := demoManifest()
	m.ACL = plugin.ACL{Read: []plugin.ACLScope{{Scope: "uci", Object: "network", Function: "read"}}}
	return m
}

func demoRepeaterManifest() plugin.Manifest {
	m := demoManifest()
	m.ACL = plugin.ACL{
		Read:  []plugin.ACLScope{{Scope: "uci", Object: "network", Function: "read"}},
		Write: []plugin.ACLScope{{Scope: "uci", Object: "network", Function: "write"}},
	}
	return m
}

func repeaterForm(op string, extra url.Values) url.Values {
	f := url.Values{"_repeater_op": {op}, "_repeater_config": {"network"}}
	for k, v := range extra {
		f[k] = v
	}
	return f
}

func TestHealthzReturnsOK(t *testing.T) {
	rec := get(t, newServer(t, fakeBackend{}), "/healthz")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /healthz: status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got, want := rec.Body.String(), "ok\n"; got != want {
		t.Fatalf("GET /healthz: body = %q, want %q", got, want)
	}
}

// TestIndexRendersOverview: the landing page renders the advanced overview — the
// verdict, the traffic section, the System panel with live firmware/kernel/uptime
// from the backend, and the lease table.
func TestIndexRendersOverview(t *testing.T) {
	wanCalls := 0
	backend := fakeBackend{
		hn:    "verso-lab",
		board: openwrt.Board{Firmware: "OpenWrt 25.12.4", Kernel: "Linux 6.12.101"},
		si:    openwrt.SystemInfo{Uptime: 3661},
		uci: map[string]map[string]any{
			"network": {
				"upstream": map[string]any{
					".type": "interface", ".name": "upstream", "device": "eth4.3900", "proto": "pppoe",
				},
			},
			"firewall": {
				"uplink_zone": map[string]any{".type": "zone", "name": "uplink", "network": []any{"upstream"}},
			},
		},
		wan: openwrt.WANState{Devices: []openwrt.WANDevice{{
			Device: "pppoe-upstream", Transport: "eth4.3900", Networks: []string{"upstream"}, Uptime: 8040,
			Routes: []openwrt.WANRoute{{Family: 4, Table: 254, Metric: 10, Main: true}},
		}}},
		wanCalls: &wanCalls,
	}
	s := newServer(t, backend)
	s.telemetry = fakeTelemetry{snapshot: telemetry.Snapshot{Interfaces: []telemetry.Interface{
		{Name: "eth4", Kind: "port", Physical: true, Operstate: "up"},
		{Name: "eth4.3900", Kind: "vlan", Parent: "eth4", Operstate: "up"},
		{Name: "pppoe-upstream", Kind: "pppoe", Operstate: "unknown"},
	}}}
	rec := get(t, s, "/")

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /: status = %d, want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"ALL GOOD", "healthy", "Internet traffic", "System", "Interfaces", "Connected devices",
		"OpenWrt 25.12.4", "Linux 6.12.101", "1h 1m", // live System facts
		"Connected", "for 2h 14m", "live · pppoe-upstream", // live WAN state, device, and uptime
	} {
		if !strings.Contains(body, want) {
			t.Errorf("GET /: body missing %q", want)
		}
	}
	if wanCalls != 1 {
		t.Errorf("WAN discovery calls = %d, want one consistent page snapshot", wanCalls)
	}
	if got := strings.Count(body, ">WAN</span>"); got != 1 {
		t.Errorf("WAN badges = %d, want only the exact pppoe-upstream L3 row", got)
	}
}

// TestIndexDegradesWhenBackendFails: the overview is hardcoded for now, so the
// page renders (never 500s) even when the backend is unreachable.
func TestIndexDegradesWhenBackendFails(t *testing.T) {
	rec := get(t, newServer(t, fakeBackend{err: errors.New("ubus down")}), "/")

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /: status = %d, want 200 (must degrade, not 500)", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Your network is") {
		t.Errorf("GET /: overview should render regardless of backend")
	}
}

// TestPluginPageRendersSchema is the core question's first yes: a plugin's schema
// (fetched via the transport) renders through the shell's own widget renderer and
// chrome, and the request reached the plugin's socket intact.
func TestPluginPageRendersSchema(t *testing.T) {
	tr := &fakeTransport{env: &plugin.Envelope{
		SchemaVersion: 1, Title: "Demo Page",
		Widget: json.RawMessage(`{"type":"card","title":"Hello","children":[{"type":"table","columns":[{"label":"A"}],"rows":[{"cells":[{"text":"1"}]}]}]}`),
	}}
	s := newServerWith(t, fakeBackend{}, tr, []plugin.Manifest{demoManifest()})

	rec := get(t, s, "/plugins/demo/")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{"Demo Page", "Hello", "<table", "<section"} {
		if !strings.Contains(body, want) {
			t.Errorf("plugin page missing %q", want)
		}
	}
	if tr.lastSocket != "/run/verso/demo.sock" {
		t.Errorf("dialed socket = %q, want the manifest's", tr.lastSocket)
	}
	if tr.lastReq.Method != http.MethodGet || tr.lastReq.Path != "" {
		t.Errorf("forwarded request = %+v, want GET with empty sub-path", tr.lastReq)
	}
}

// TestPluginReadBrokeredAsSnapshot: a plugin declaring acl.read receives the config
// in its request as a snapshot the shell read with the operator's sid (ADR-007) —
// the plugin never reads /etc/config itself.
func TestPluginReadBrokeredAsSnapshot(t *testing.T) {
	tr := &fakeTransport{env: &plugin.Envelope{
		SchemaVersion: 1, Title: "WG", Widget: json.RawMessage(`{"type":"card"}`),
	}}
	backend := fakeBackend{uci: map[string]map[string]any{
		"network": {"wg0": map[string]any{".type": "interface", "proto": "wireguard"}},
	}}
	s := newServerWith(t, backend, tr, []plugin.Manifest{demoReadManifest()})

	if rec := get(t, s, "/plugins/demo/"); rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	network, ok := tr.lastReq.UCI["network"]
	if !ok {
		t.Fatalf("forwarded request carried no network snapshot: %+v", tr.lastReq.UCI)
	}
	wg0, _ := network["wg0"].(map[string]any)
	if wg0["proto"] != "wireguard" {
		t.Errorf("snapshot wg0 = %v, want proto wireguard", wg0)
	}
}

// TestPluginNoReadACLGetsNoSnapshot: a plugin that declares no reads receives no
// snapshot — the shell brokers only what the manifest asked for.
func TestPluginNoReadACLGetsNoSnapshot(t *testing.T) {
	tr := &fakeTransport{env: &plugin.Envelope{
		SchemaVersion: 1, Title: "Demo", Widget: json.RawMessage(`{"type":"card"}`),
	}}
	backend := fakeBackend{uci: map[string]map[string]any{
		"network": {"wg0": map[string]any{"proto": "wireguard"}},
	}}
	s := newServerWith(t, backend, tr, []plugin.Manifest{demoManifest()})

	if rec := get(t, s, "/plugins/demo/"); rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if tr.lastReq.UCI != nil {
		t.Errorf("undeclared plugin got a snapshot: %+v", tr.lastReq.UCI)
	}
}

// repeaterTransport renders a trivial page, standing in for the plugin's re-render
// of the fresh state after a repeater op.
func repeaterTransport() *fakeTransport {
	return &fakeTransport{env: &plugin.Envelope{
		SchemaVersion: 1, Title: "WG", Widget: json.RawMessage(`{"type":"card"}`),
	}}
}

// TestPluginRepeaterAddRealized: the shell — not the plugin — performs a repeater's
// "add" (ADR-005 §7). A repeater add POST creates a uci section of the declared type
// through rpcd and re-renders the fresh state as a read (method downgraded to GET).
func TestPluginRepeaterAddRealized(t *testing.T) {
	adds, dels := []string{}, []string{}
	tr := repeaterTransport()
	backend := fakeBackend{access: true, adds: &adds, deletes: &dels, addReturns: "cfgNEW"}
	s := newServerWith(t, backend, tr, []plugin.Manifest{demoRepeaterManifest()})

	rec := postPlugin(t, s, "/plugins/demo/", repeaterForm("add", url.Values{"_repeater_type": {"wireguard_wg0"}}))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if len(adds) != 1 || adds[0] != "network wireguard_wg0" {
		t.Errorf("UCIAdd calls = %v, want one \"network wireguard_wg0\"", adds)
	}
	if len(dels) != 0 {
		t.Errorf("an add must not delete: %v", dels)
	}
	if tr.lastReq.Method != http.MethodGet {
		t.Errorf("re-render method = %q, want GET (the op was realized, then rendered)", tr.lastReq.Method)
	}
}

// TestPluginRepeaterRemoveRealized: a repeater "remove" POST deletes the named uci
// section through rpcd and re-renders.
func TestPluginRepeaterRemoveRealized(t *testing.T) {
	adds, dels := []string{}, []string{}
	tr := repeaterTransport()
	backend := fakeBackend{access: true, adds: &adds, deletes: &dels}
	s := newServerWith(t, backend, tr, []plugin.Manifest{demoRepeaterManifest()})

	rec := postPlugin(t, s, "/plugins/demo/", repeaterForm("remove", url.Values{"_repeater_section": {"cfg01"}}))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if len(dels) != 1 || dels[0] != "network.cfg01" {
		t.Errorf("UCIDelete calls = %v, want one \"network.cfg01\"", dels)
	}
	if len(adds) != 0 {
		t.Errorf("a remove must not add: %v", adds)
	}
}

// TestPluginRepeaterRefusedForUndeclaredConfig: a repeater op naming a config the
// plugin never declared in acl.write is refused, and nothing is written — the same
// bound brokerStage enforces.
func TestPluginRepeaterRefusedForUndeclaredConfig(t *testing.T) {
	adds, dels := []string{}, []string{}
	tr := repeaterTransport()
	backend := fakeBackend{access: true, adds: &adds, deletes: &dels}
	s := newServerWith(t, backend, tr, []plugin.Manifest{demoRepeaterManifest()})

	form := url.Values{"_repeater_op": {"add"}, "_repeater_config": {"firewall"}, "_repeater_type": {"rule"}}
	rec := postPlugin(t, s, "/plugins/demo/", form)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
	if len(adds) != 0 || len(dels) != 0 {
		t.Errorf("an undeclared-config repeater op must write nothing: adds=%v dels=%v", adds, dels)
	}
}

// TestPluginRepeaterGatedByWriteACL: a repeater op is a state-changing request, so
// the acl.write gate (ADR-007) refuses an operator who may not write the plugin's
// declared configs — before any structural change.
func TestPluginRepeaterGatedByWriteACL(t *testing.T) {
	adds, dels := []string{}, []string{}
	tr := repeaterTransport()
	backend := fakeBackend{access: false, adds: &adds, deletes: &dels}
	s := newServerWith(t, backend, tr, []plugin.Manifest{demoRepeaterManifest()})

	rec := postPlugin(t, s, "/plugins/demo/", repeaterForm("add", url.Values{"_repeater_type": {"wireguard_wg0"}}))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
	if len(adds) != 0 {
		t.Errorf("a denied operator must not add: %v", adds)
	}
}

// TestPluginUnavailableWhenTransportFails is the crash-isolation contract, proven
// hermetically: a dead plugin renders "unavailable" in chrome, and the shell
// returns 200 — never 500, never a crash (ADR-006 §7).
func TestPluginUnavailableWhenTransportFails(t *testing.T) {
	tr := &fakeTransport{err: errors.New("dial unix: connection refused")}
	s := newServerWith(t, fakeBackend{}, tr, []plugin.Manifest{demoManifest()})

	rec := get(t, s, "/plugins/demo/")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (a plugin must never 500 the shell)", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "unavailable") {
		t.Errorf("expected 'plugin unavailable' notice")
	}
}

// TestPluginValidation422Propagates: when a plugin returns 422 (a validation
// failure), the shell renders the error form AND passes 422 to the browser —
// the HTTP semantics stay honest, not flattened to 200 (ADR-006 §5).
func TestPluginValidation422Propagates(t *testing.T) {
	tr := &fakeTransport{env: &plugin.Envelope{
		SchemaVersion: 1, Status: http.StatusUnprocessableEntity,
		Widget: json.RawMessage(`{"type":"card","children":[{"type":"table","columns":[{"label":"A"}],"rows":[]}]}`),
	}}
	s := newServerWith(t, fakeBackend{}, tr, []plugin.Manifest{demoManifest()})

	rec := get(t, s, "/plugins/demo/")
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422 propagated from the plugin", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "<section") {
		t.Errorf("a 422 must still render the (error) schema")
	}
}

// TestPluginBadSchemaIsUnavailable: a plugin returning garbage the widget set
// can't decode is contained exactly like a dead one.
func TestPluginBadSchemaIsUnavailable(t *testing.T) {
	tr := &fakeTransport{env: &plugin.Envelope{SchemaVersion: 1, Widget: json.RawMessage(`{"type":"nope"}`)}}
	s := newServerWith(t, fakeBackend{}, tr, []plugin.Manifest{demoManifest()})

	rec := get(t, s, "/plugins/demo/")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "unavailable") {
		t.Errorf("expected 'unavailable' for an undecodable schema")
	}
}

func TestPluginUnknownIDIs404(t *testing.T) {
	s := newServerWith(t, fakeBackend{}, &fakeTransport{}, nil)

	rec := get(t, s, "/plugins/ghost/")
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404 for an unknown plugin id", rec.Code)
	}
}

// TestPluginVersionMismatchDegrades: an unsupported schema_version is a notice,
// not a fetch and not a crash (ADR-006 §6). The transport must not be dialed.
func TestPluginVersionMismatchDegrades(t *testing.T) {
	m := demoManifest()
	m.SchemaVersion = 99
	tr := &fakeTransport{err: errors.New("must not be called")}
	s := newServerWith(t, fakeBackend{}, tr, []plugin.Manifest{m})

	rec := get(t, s, "/plugins/demo/")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "newer Verso") {
		t.Errorf("expected version-mismatch notice, got: %s", rec.Body.String())
	}
	if tr.lastSocket != "" {
		t.Errorf("transport was dialed on a version mismatch; it must not be")
	}
}

// TestPluginWriteAllowedWhenSessionGranted: a POST whose session holds the
// plugin's declared write grant is dispatched to the plugin and its schema
// renders. The gate sits in front of the existing gateway (ADR-007).
func TestPluginWriteAllowedWhenSessionGranted(t *testing.T) {
	tr := &fakeTransport{env: &plugin.Envelope{
		SchemaVersion: 1, Title: "Saved",
		Widget: json.RawMessage(`{"type":"card","title":"Saved","children":[{"type":"table","columns":[{"label":"A"}],"rows":[{"cells":[{"text":"1"}]}]}]}`),
	}}
	s := newServerWith(t, fakeBackend{access: true}, tr, []plugin.Manifest{demoACLManifest()})

	rec := postPlugin(t, s, "/plugins/demo/", url.Values{"hostname": {"verso-lab"}})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if tr.lastSocket != "/run/verso/demo.sock" {
		t.Errorf("granted write was not dispatched to the plugin (socket %q)", tr.lastSocket)
	}
	if got := url.Values(tr.lastReq.Form).Get("hostname"); got != "verso-lab" {
		t.Errorf("form not forwarded: hostname=%q", got)
	}
	if _, ok := tr.lastReq.Form["_csrf"]; ok {
		t.Errorf("the shell's _csrf token must not be forwarded to the plugin")
	}
}

// TestPluginWriteRefusedWhenSessionDenied: a POST whose session lacks the grant
// is refused by the shell with 403 and NEVER reaches the plugin — the shell is
// the enforcement point (ADR-007).
func TestPluginWriteRefusedWhenSessionDenied(t *testing.T) {
	tr := &fakeTransport{env: &plugin.Envelope{SchemaVersion: 1, Widget: json.RawMessage(`{"type":"card","children":[]}`)}}
	s := newServerWith(t, fakeBackend{access: false}, tr, []plugin.Manifest{demoACLManifest()})

	rec := postPlugin(t, s, "/plugins/demo/", url.Values{"hostname": {"x"}})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 for a denied write", rec.Code)
	}
	if tr.lastSocket != "" {
		t.Errorf("a denied write must not reach the plugin; dialed %q", tr.lastSocket)
	}
	if !strings.Contains(rec.Body.String(), "permitted") {
		t.Errorf("expected a 'not permitted' notice")
	}
}

// TestPluginWriteRefusedWithoutDeclaredACL: fail closed — a plugin that declares
// no write scopes cannot receive a state-changing request, even for a session
// that would pass any check (ADR-007).
func TestPluginWriteRefusedWithoutDeclaredACL(t *testing.T) {
	tr := &fakeTransport{env: &plugin.Envelope{SchemaVersion: 1, Widget: json.RawMessage(`{"type":"card","children":[]}`)}}
	s := newServerWith(t, fakeBackend{access: true}, tr, []plugin.Manifest{demoManifest()}) // no ACL

	rec := postPlugin(t, s, "/plugins/demo/", url.Values{"hostname": {"x"}})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 when the plugin declares no write scopes", rec.Code)
	}
	if tr.lastSocket != "" {
		t.Errorf("must not reach a plugin with no declared write ACL; dialed %q", tr.lastSocket)
	}
}

// TestPluginWriteFailsClosedWhenACLCheckErrors: if rpcd can't be reached to
// authorize, the shell fails closed (503) rather than dispatching the write.
func TestPluginWriteFailsClosedWhenACLCheckErrors(t *testing.T) {
	tr := &fakeTransport{env: &plugin.Envelope{SchemaVersion: 1, Widget: json.RawMessage(`{"type":"card","children":[]}`)}}
	s := newServerWith(t, fakeBackend{accessErr: errors.New("rpcd down")}, tr, []plugin.Manifest{demoACLManifest()})

	rec := postPlugin(t, s, "/plugins/demo/", url.Values{"hostname": {"x"}})
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503 when the ACL check errors", rec.Code)
	}
	if tr.lastSocket != "" {
		t.Errorf("must not dispatch when authorization is unknown; dialed %q", tr.lastSocket)
	}
}

// TestPluginGetNotGated: reads are not gated — a GET renders even with no write
// grant, so a read-only operator can still view the page (ADR-007 gates writes).
func TestPluginGetNotGated(t *testing.T) {
	tr := &fakeTransport{env: &plugin.Envelope{
		SchemaVersion: 1, Title: "Demo",
		Widget: json.RawMessage(`{"type":"card","title":"Hi","children":[{"type":"table","columns":[{"label":"A"}],"rows":[{"cells":[{"text":"1"}]}]}]}`),
	}}
	s := newServerWith(t, fakeBackend{access: false}, tr, []plugin.Manifest{demoACLManifest()})

	rec := get(t, s, "/plugins/demo/")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (GET must not be gated)", rec.Code)
	}
	if tr.lastSocket != "/run/verso/demo.sock" {
		t.Errorf("GET should reach the plugin; dialed %q", tr.lastSocket)
	}
}

// TestPluginCommitBrokered: a plugin returns a declarative write intent and the
// shell executes it through the backend on the operator's behalf — the plugin
// itself performs no write (ADR-007).
func TestPluginCommitBrokered(t *testing.T) {
	calls := []uciWrite{}
	tr := &fakeTransport{env: &plugin.Envelope{
		SchemaVersion: 1, Title: "Saved", Status: http.StatusOK,
		Widget: json.RawMessage(`{"type":"card","title":"Saved","children":[{"type":"table","columns":[{"label":"A"}],"rows":[{"cells":[{"text":"1"}]}]}]}`),
		Commit: []plugin.CommitOp{{Config: "system", Section: "@system[0]", Values: map[string]any{"hostname": "verso-lab"}}},
	}}
	s := newServerWith(t, fakeBackend{access: true, writes: &calls}, tr, []plugin.Manifest{demoACLManifest()})

	rec := postPlugin(t, s, "/plugins/demo/", url.Values{"hostname": {"verso-lab"}})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if len(calls) != 1 {
		t.Fatalf("UCISet calls = %d, want 1 (the shell must broker the write)", len(calls))
	}
	w := calls[0]
	if w.sid != "test-sid" || w.config != "system" || w.section != "@system[0]" || w.values["hostname"] != "verso-lab" {
		t.Errorf("brokered write = %+v; want sid test-sid, system/@system[0], hostname=verso-lab", w)
	}
}

// TestPluginCommitCreatesSection: a commit op with no section and a type
// creates the section through the backend, then sets the values on the id the
// add returned — one staged step, so an Add drawer's Save yields its row.
func TestPluginCommitCreatesSection(t *testing.T) {
	calls := []uciWrite{}
	adds := []string{}
	tr := &fakeTransport{env: &plugin.Envelope{
		SchemaVersion: 1, Status: http.StatusOK,
		Widget: json.RawMessage(`{"type":"card","children":[]}`),
		Commit: []plugin.CommitOp{{Config: "system", Section: "", Type: "led", Values: map[string]any{"name": "disk"}}},
	}}
	be := fakeBackend{access: true, writes: &calls, adds: &adds, addReturns: "cfg99aa"}
	s := newServerWith(t, be, tr, []plugin.Manifest{demoACLManifest()})

	rec := postPlugin(t, s, "/plugins/demo/", url.Values{"name": {"disk"}})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if len(adds) != 1 || adds[0] != "system led" {
		t.Fatalf("UCIAdd calls = %v, want one system/led create", adds)
	}
	if len(calls) != 1 || calls[0].section != "cfg99aa" || calls[0].values["name"] != "disk" {
		t.Fatalf("brokered write = %+v; want the created section's id", calls)
	}
}

// TestPluginCommitListOption: a uci list option (an array value, e.g. the NTP
// server list) is carried through the broker to the backend intact.
func TestPluginCommitListOption(t *testing.T) {
	calls := []uciWrite{}
	tr := &fakeTransport{env: &plugin.Envelope{
		SchemaVersion: 1, Status: http.StatusOK,
		Widget: json.RawMessage(`{"type":"card","children":[]}`),
		Commit: []plugin.CommitOp{{Config: "system", Section: "ntp", Values: map[string]any{"server": []any{"a.pool", "b.pool"}}}},
	}}
	s := newServerWith(t, fakeBackend{access: true, writes: &calls}, tr, []plugin.Manifest{demoACLManifest()})

	rec := postPlugin(t, s, "/plugins/demo/", url.Values{"server": {"a.pool", "b.pool"}})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if len(calls) != 1 {
		t.Fatalf("UCISet calls = %d, want 1", len(calls))
	}
	arr, ok := calls[0].values["server"].([]any)
	if !ok || len(arr) != 2 || arr[0] != "a.pool" {
		t.Errorf("list option not carried through the broker: %#v", calls[0].values["server"])
	}
}

// TestPluginCommitRefusedForUndeclaredConfig: a plugin cannot broker a write to a
// config it did not declare in its manifest — the shell refuses and writes nothing
// (the malicious-plugin defense).
func TestPluginCommitRefusedForUndeclaredConfig(t *testing.T) {
	calls := []uciWrite{}
	tr := &fakeTransport{env: &plugin.Envelope{
		SchemaVersion: 1, Status: http.StatusOK,
		Widget: json.RawMessage(`{"type":"card","children":[]}`),
		Commit: []plugin.CommitOp{{Config: "network", Section: "@interface[0]", Values: map[string]any{"proto": "static"}}},
	}}
	// demoACLManifest declares only uci/system.
	s := newServerWith(t, fakeBackend{access: true, writes: &calls}, tr, []plugin.Manifest{demoACLManifest()})

	rec := postPlugin(t, s, "/plugins/demo/", url.Values{"x": {"1"}})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 for a write outside the declared scope", rec.Code)
	}
	if len(calls) != 0 {
		t.Errorf("an undeclared write must NOT be executed; got %d UCISet calls", len(calls))
	}
}

// TestPluginCommitIgnoredOnGet: a commit intent on a safe (GET) request is never
// executed — reads cannot write.
func TestPluginCommitIgnoredOnGet(t *testing.T) {
	calls := []uciWrite{}
	tr := &fakeTransport{env: &plugin.Envelope{
		SchemaVersion: 1, Title: "Demo",
		Widget: json.RawMessage(`{"type":"card","title":"Hi","children":[{"type":"table","columns":[{"label":"A"}],"rows":[{"cells":[{"text":"1"}]}]}]}`),
		Commit: []plugin.CommitOp{{Config: "system", Section: "@system[0]", Values: map[string]any{"hostname": "x"}}},
	}}
	s := newServerWith(t, fakeBackend{access: false, writes: &calls}, tr, []plugin.Manifest{demoACLManifest()})

	rec := get(t, s, "/plugins/demo/")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if len(calls) != 0 {
		t.Errorf("a commit on a GET must be ignored, not executed; got %d writes", len(calls))
	}
}

// TestPluginCommitBackendErrorContained: if the brokered write fails at the
// backend, the shell reports a contained failure rather than a false success.
func TestPluginCommitBackendErrorContained(t *testing.T) {
	calls := []uciWrite{}
	tr := &fakeTransport{env: &plugin.Envelope{
		SchemaVersion: 1, Status: http.StatusOK,
		Widget: json.RawMessage(`{"type":"card","children":[]}`),
		Commit: []plugin.CommitOp{{Config: "system", Section: "@system[0]", Values: map[string]any{"hostname": "x"}}},
	}}
	s := newServerWith(t, fakeBackend{access: true, writes: &calls, uciErr: errors.New("rpcd denied")}, tr, []plugin.Manifest{demoACLManifest()})

	rec := postPlugin(t, s, "/plugins/demo/", url.Values{"hostname": {"x"}})
	if rec.Code == http.StatusOK {
		t.Fatalf("a failed broker must not report 200")
	}
	if len(calls) != 1 {
		t.Errorf("UCISet should have been attempted once; got %d", len(calls))
	}
}

// TestPluginDatatypeErrorBlocksCommit: the shell enforces a field's declared
// datatype on the returned schema (ADR-008); a failure returns 422 and the commit
// never runs, even though the plugin asked for it.
func TestPluginDatatypeErrorBlocksCommit(t *testing.T) {
	calls := []uciWrite{}
	tr := &fakeTransport{env: &plugin.Envelope{
		SchemaVersion: 1, Status: http.StatusOK,
		Widget: json.RawMessage(`{"type":"card","children":[{"type":"form","fields":[{"type":"field","name":"host","datatype":"fqdn","value":"OpenWrt"}]}]}`),
		Commit: []plugin.CommitOp{{Config: "system", Section: "@system[0]", Values: map[string]any{"hostname": "OpenWrt"}}},
	}}
	s := newServerWith(t, fakeBackend{access: true, writes: &calls}, tr, []plugin.Manifest{demoACLManifest()})

	rec := postPlugin(t, s, "/plugins/demo/", url.Values{"host": {"OpenWrt"}})
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422 for a datatype failure", rec.Code)
	}
	if len(calls) != 0 {
		t.Errorf("a datatype failure must block the commit; got %d writes", len(calls))
	}
	if !strings.Contains(rec.Body.String(), "fully-qualified") {
		t.Errorf("the shell's datatype error is not shown: %s", rec.Body.String())
	}
}

// TestPluginDatatypeValidCommits: a submission that passes every declared datatype
// is brokered normally.
func TestPluginDatatypeValidCommits(t *testing.T) {
	calls := []uciWrite{}
	tr := &fakeTransport{env: &plugin.Envelope{
		SchemaVersion: 1, Status: http.StatusOK,
		Widget: json.RawMessage(`{"type":"card","children":[{"type":"form","fields":[{"type":"field","name":"host","datatype":"fqdn","value":"router.lan"}]}]}`),
		Commit: []plugin.CommitOp{{Config: "system", Section: "@system[0]", Values: map[string]any{"hostname": "router.lan"}}},
	}}
	s := newServerWith(t, fakeBackend{access: true, writes: &calls}, tr, []plugin.Manifest{demoACLManifest()})

	rec := postPlugin(t, s, "/plugins/demo/", url.Values{"host": {"router.lan"}})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 for a valid submission", rec.Code)
	}
	if len(calls) != 1 {
		t.Errorf("a valid submission must broker the commit; got %d writes", len(calls))
	}
}

// TestPluginListDatatypeErrorBlocksCommit: a bad item in a list field is caught
// per-item and blocks the write.
func TestPluginListDatatypeErrorBlocksCommit(t *testing.T) {
	calls := []uciWrite{}
	tr := &fakeTransport{env: &plugin.Envelope{
		SchemaVersion: 1, Status: http.StatusOK,
		Widget: json.RawMessage(`{"type":"card","children":[{"type":"form","fields":[{"type":"list","name":"server","datatype":"host","items":["good.example.com","bad host"]}]}]}`),
		Commit: []plugin.CommitOp{{Config: "system", Section: "ntp", Values: map[string]any{"server": []any{"good.example.com", "bad host"}}}},
	}}
	s := newServerWith(t, fakeBackend{access: true, writes: &calls}, tr, []plugin.Manifest{demoACLManifest()})

	rec := postPlugin(t, s, "/plugins/demo/", url.Values{"server": {"good.example.com", "bad host"}})
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422 for a bad list item", rec.Code)
	}
	if len(calls) != 0 {
		t.Errorf("a list datatype failure must block the commit; got %d writes", len(calls))
	}
}

// TestValidateSchemaAnnotatesAndPreservesPluginErrors covers the walk directly:
// the shell flags an invalid value, leaves a plugin's own error alone, and keys a
// bad list item by its index.
func TestValidateSchemaAnnotatesAndPreservesPluginErrors(t *testing.T) {
	form := &widget.Form{Fields: []widget.Widget{
		&widget.Field{Name: "a", Datatype: "fqdn", Value: "OpenWrt"},           // shell flags
		&widget.Field{Name: "b", Datatype: "fqdn", Value: "x", Error: "taken"}, // plugin error kept
		&widget.List{Name: "c", Datatype: "port", Items: []string{"80", "nope"}},
	}}
	if !validateSchema(&widget.Card{Children: []widget.Widget{form}}) {
		t.Fatal("validateSchema should report errors")
	}
	if form.Fields[0].(*widget.Field).Error == "" {
		t.Error("field a should have a shell datatype error")
	}
	if got := form.Fields[1].(*widget.Field).Error; got != "taken" {
		t.Errorf("plugin error on field b was clobbered: %q", got)
	}
	if l := form.Fields[2].(*widget.List); l.Errors["1"] == "" || l.Errors["0"] != "" {
		t.Errorf("list errors keyed wrong: %v", l.Errors)
	}
}

// TestNavListsPlugins: a discovered plugin appears in the shell nav with no shell
// change — the manifest drives it (ADR-006 §2).
func TestNavListsPlugins(t *testing.T) {
	s := newServerWith(t, fakeBackend{}, &fakeTransport{}, []plugin.Manifest{demoManifest()})

	rec := get(t, s, "/")
	body := rec.Body.String()
	for _, want := range []string{"Apps", "Demo", `href="/plugins/demo/"`} {
		if !strings.Contains(body, want) {
			t.Errorf("nav missing %q", want)
		}
	}
}

// TestNavMultipleEntriesPerPlugin: one plugin can place several pages in the
// menu, each grouped under its section.
func TestNavMultipleEntriesPerPlugin(t *testing.T) {
	m := demoManifest()
	m.Nav = []plugin.NavEntry{
		{Section: "Apps", Label: "General", Path: "/"},
		{Section: "Apps", Label: "Time", Path: "/time"},
	}
	s := newServerWith(t, fakeBackend{}, &fakeTransport{}, []plugin.Manifest{m})

	body := get(t, s, "/").Body.String()
	for _, want := range []string{"General", "Time", `href="/plugins/demo/"`, `href="/plugins/demo/time"`} {
		if !strings.Contains(body, want) {
			t.Errorf("nav missing %q", want)
		}
	}
}

// TestHostGuardRejectsUnknownHost: a request with a Host outside the allowlist
// is refused (VS-01, DNS-rebinding defense).
func TestHostGuardRejectsUnknownHost(t *testing.T) {
	srv := newServer(t, fakeBackend{})
	srv.SetAllowedHosts([]string{"verso.lan", "127.0.0.1"})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = "evil.example.com"
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("unknown host: status = %d, want 400", rec.Code)
	}
}

func TestHostGuardAllowsConfiguredHost(t *testing.T) {
	srv := newServer(t, fakeBackend{})
	srv.SetAllowedHosts([]string{"verso.lan"})
	token, _ := srv.sessions.CreateWithMetadata("sid", "root", "", "")

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = "verso.lan:8080" // port is stripped before the check
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: token})
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("allowed host: status = %d, want 200", rec.Code)
	}
}

// TestSecurityHeaders: defensive headers are set on every response (VS-07).
func TestSecurityHeaders(t *testing.T) {
	rec := get(t, newServer(t, fakeBackend{}), "/")
	h := rec.Header()
	csp := h.Get("Content-Security-Policy")
	// The shell serves its own first-party JS (ADR-004), so scripts are 'self' —
	// but never 'unsafe-inline' or 'unsafe-eval', so injected/plugin markup still
	// cannot execute (Alpine's CSP build needs neither).
	if !strings.Contains(csp, "script-src 'self'") {
		t.Errorf("CSP script-src should be 'self': %q", csp)
	}
	if strings.Contains(csp, "unsafe-eval") {
		t.Errorf("CSP must not allow unsafe-eval: %q", csp)
	}
	if h.Get("X-Frame-Options") != "DENY" {
		t.Errorf("X-Frame-Options = %q, want DENY", h.Get("X-Frame-Options"))
	}
	if h.Get("X-Content-Type-Options") != "nosniff" {
		t.Errorf("X-Content-Type-Options missing")
	}
}

// TestUnauthenticatedRedirectsToLogin: the middleware gates every non-public page.
func TestUnauthenticatedRedirectsToLogin(t *testing.T) {
	srv := newServer(t, fakeBackend{})
	req := httptest.NewRequest(http.MethodGet, "/", nil) // no cookie
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/login" {
		t.Errorf("Location = %q, want /login", loc)
	}
}

func TestLoginPageIsPublic(t *testing.T) {
	srv := newServer(t, fakeBackend{})
	req := httptest.NewRequest(http.MethodGet, "/login", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Sign in") {
		t.Errorf("login page missing the form")
	}
	for _, font := range []string{"hanken.woff2", "fraunces.woff2", "inconsolata-latin.woff2"} {
		if !strings.Contains(rec.Body.String(), `rel="preload" href="/assets/fonts/`+font+`"`) {
			t.Errorf("login page does not preload %s", font)
		}
	}
}

func TestLoginSuccessSetsHttpOnlyCookieAndRedirects(t *testing.T) {
	srv := newServerFull(t, fakeBackend{}, &fakeTransport{}, nil, fakeAuth{sid: "rpcd-sid"})

	rec := postForm(t, srv, "/login", url.Values{"username": {"root"}, "password": {"pw"}})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303", rec.Code)
	}
	cookies := rec.Result().Cookies()
	if len(cookies) == 0 || cookies[0].Name != sessionCookie || cookies[0].Value == "" {
		t.Fatalf("no session cookie set: %v", cookies)
	}
	if !cookies[0].HttpOnly {
		t.Errorf("session cookie must be HttpOnly")
	}
}

func TestLoginFailureShowsErrorAndNoCookie(t *testing.T) {
	srv := newServerFull(t, fakeBackend{}, &fakeTransport{}, nil, fakeAuth{err: errors.New("denied")})

	rec := postForm(t, srv, "/login", url.Values{"username": {"root"}, "password": {"bad"}})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (re-render)", rec.Code)
	}
	if len(rec.Result().Cookies()) != 0 {
		t.Errorf("a failed login must not set a session cookie")
	}
	if !strings.Contains(rec.Body.String(), "Invalid") {
		t.Errorf("expected an error message")
	}
	for _, want := range []string{
		"dark:bg-red-500/10 dark:text-red-300 dark:ring-red-500/20",
		"active:translate-y-px", "active:shadow-none", "motion-reduce:active:translate-y-0",
		"dark:bg-sky-700 dark:text-gray-100 dark:hover:bg-sky-800 dark:active:bg-sky-900",
	} {
		if !strings.Contains(rec.Body.String(), want) {
			t.Errorf("login dark-mode styling missing %q", want)
		}
	}
}

// TestLoginThrottled: repeated failures from one client lock further attempts
// with 429 (VS-06).
func TestLoginThrottled(t *testing.T) {
	srv := newServerFull(t, fakeBackend{}, &fakeTransport{}, nil,
		fakeAuth{err: errors.New("denied")})
	bad := url.Values{"username": {"root"}, "password": {"wrong"}}

	for i := 0; i < loginMaxFailures; i++ {
		if rec := postForm(t, srv, "/login", bad); rec.Code != http.StatusOK {
			t.Fatalf("attempt %d: status = %d, want 200", i, rec.Code)
		}
	}
	if rec := postForm(t, srv, "/login", bad); rec.Code != http.StatusTooManyRequests {
		t.Errorf("after %d failures: status = %d, want 429", loginMaxFailures, rec.Code)
	}
}

func TestLogoutClearsSession(t *testing.T) {
	srv := newServer(t, fakeBackend{})
	token, _ := srv.sessions.CreateWithMetadata("sid", "root", "", "")
	sess, _ := srv.sessions.get(token)

	form := url.Values{"_csrf": {sess.csrf}}
	req := httptest.NewRequest(http.MethodPost, "/logout", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: token})
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303", rec.Code)
	}
	if _, ok := srv.sessions.get(token); ok {
		t.Errorf("session was not destroyed")
	}
}

// TestCSRFRejectsPostWithoutToken: a state-changing request lacking the session
// CSRF token is refused (VS-04).
func TestCSRFRejectsPostWithoutToken(t *testing.T) {
	srv := newServer(t, fakeBackend{})
	token, _ := srv.sessions.CreateWithMetadata("sid", "root", "", "")

	req := httptest.NewRequest(http.MethodPost, "/logout", nil) // no _csrf
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: token})
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("POST without CSRF token: status = %d, want 403", rec.Code)
	}
}

// TestCrossSiteLogin: the pre-session CSRF guard on POST /login refuses only a
// genuine cross-site submit. same-site is allowed — routers reached by bare
// IP/hostname legitimately report it, and blocking it breaks real logins. So is
// Origin: null — our own no-referrer page produces it in Safari, and it is the
// opaque-origin marker, not a named cross-site origin.
func TestCrossSiteLogin(t *testing.T) {
	cases := []struct {
		name    string
		fetch   string
		origin  string
		host    string
		blocked bool
	}{
		{"same-origin fetch", "same-origin", "", "", false},
		{"same-site fetch (routers report this)", "same-site", "", "", false},
		{"none fetch (typed URL / direct nav)", "none", "", "", false},
		{"cross-site fetch (the attack)", "cross-site", "", "", true},
		{"no fetch, matching origin", "", "http://box:8080", "box:8080", false},
		{"no fetch, mismatched origin", "", "http://evil.example", "box:8080", true},
		{"no fetch, Origin null (Safari + no-referrer, our own page)", "", "null", "box:8080", false},
		{"no headers at all (non-browser)", "", "", "box:8080", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "/login", nil)
			if c.host != "" {
				r.Host = c.host
			}
			if c.fetch != "" {
				r.Header.Set("Sec-Fetch-Site", c.fetch)
			}
			if c.origin != "" {
				r.Header.Set("Origin", c.origin)
			}
			if got := crossSiteLogin(r); got != c.blocked {
				t.Errorf("crossSiteLogin = %v, want %v", got, c.blocked)
			}
		})
	}
}

// TestNoPasswordBanner: the full-width security warning shows only when root
// has no password and sits at the navigation seam rather than inside content.
func TestNoPasswordBanner(t *testing.T) {
	warn := newServerFull(t, fakeBackend{rootNoPassword: true}, &fakeTransport{}, nil, fakeAuth{sid: "s"})
	body := get(t, warn, "/").Body.String()
	for _, want := range []string{
		"No administrator password is set.",
		"Anyone who can reach this router can change its settings.",
		"border-red-200 bg-red-50",
		"text-red-600",
		"dark:border-red-500/20 dark:bg-red-500/10 dark:text-red-300",
		"dark:text-red-400",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("no-password warning missing %q", want)
		}
	}
	if strings.Contains(body, "passwd</code> over SSH") {
		t.Error("the warning must point at the Access experience, not require SSH")
	}
	safe := newServer(t, fakeBackend{}) // hasPassword true
	if strings.Contains(get(t, safe, "/").Body.String(), "No administrator password") {
		t.Errorf("must not warn when a password is set")
	}

	tr := &fakeTransport{env: &plugin.Envelope{
		SchemaVersion: 1, Title: "System", Pages: []plugin.PageTab{{Label: "Access", Path: "access"}},
		Widget: json.RawMessage(`{"type":"card","children":[]}`),
	}}
	withPages := newServerFull(t, fakeBackend{rootNoPassword: true}, tr, []plugin.Manifest{demoManifest()}, fakeAuth{sid: "s"})
	body = get(t, withPages, "/plugins/demo/access").Body.String()
	navAt := strings.Index(body, `aria-label="Subpages"`)
	warnAt := strings.Index(body, "No administrator password is set.")
	if navAt < 0 || warnAt < navAt {
		t.Errorf("password warning must sit below the secondary menu: nav=%d warning=%d", navAt, warnAt)
	}
	if !strings.Contains(body, "after:bg-red-600") || !strings.Contains(body, "dark:text-red-400 dark:after:bg-red-500") ||
		!strings.Contains(body, "-mt-px flex h-12") || !strings.Contains(body, "border-b border-red-200 dark:border-red-500/20") {
		t.Error("password warning and active subpage must form one red, menu-height seam")
	}

	// A page may declare another important state at the same seam. The shell's
	// own password warning takes precedence when active, so notices never stack.
	tr = &fakeTransport{env: &plugin.Envelope{
		SchemaVersion: 1,
		Title:         "System",
		Pages:         []plugin.PageTab{{Label: "Access", Path: "access"}},
		Banner: &plugin.Banner{Variant: "danger", Title: "No administrator password is set.",
			Body: "Anyone who can reach this router can change its settings."},
		Widget: json.RawMessage(`{"type":"card","children":[]}`),
	}}
	pageBanner := newServerFull(t, fakeBackend{}, tr, []plugin.Manifest{demoManifest()}, fakeAuth{sid: "s"})
	body = get(t, pageBanner, "/plugins/demo/access").Body.String()
	navAt = strings.Index(body, `aria-label="Subpages"`)
	warnAt = strings.Index(body, "No administrator password is set.")
	if navAt < 0 || warnAt < navAt || !strings.Contains(body, "border-red-200 bg-red-50") || !strings.Contains(body, "after:bg-red-600") {
		t.Errorf("declared danger banner must sit below the secondary menu: nav=%d warning=%d", navAt, warnAt)
	}
}
