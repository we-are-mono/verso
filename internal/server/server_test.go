// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/we-are-mono/verso/internal/openwrt"
	"github.com/we-are-mono/verso/internal/plugin"
	"github.com/we-are-mono/verso/internal/sysstat"
	"github.com/we-are-mono/verso/internal/telemetry"
	"github.com/we-are-mono/verso/internal/updatecheck"
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
	uciErr         error                     // returned by UCISet
	uciReadErr     error                     // returned by UCIConfig (a read the shell brokers)
	uciReads       *int                      // counts UCIConfig calls (pointer: fakeBackend is by value)
	writes         *[]uciWrite               // records UCISet calls (pointer: fakeBackend is used by value)
	uci            map[string]map[string]any // per-config read snapshots UCIConfig returns
	addReturns     string                    // section id UCIAdd returns
	adds           *[]string                 // records "config secType" per UCIAdd (pointer: fakeBackend is by value)
	deletes        *[]string                 // records "config.section[.option]" per UCIDelete
	orders         *[]string                 // records "config: a,b,c" per UCIOrder
	// deleteErr answers one UCIDelete by what it was asked to remove — how a test
	// stages an option rpcd cannot find (openwrt.ErrOptionNotFound) beside options
	// it can. Nil means "succeed unless uciErr says otherwise".
	deleteErr func(config, section, option string) error
	// setPassword backs SetPassword — tests inject it to capture the sid/username/
	// password or return an error. Nil means "succeed silently".
	setPassword      func(ctx context.Context, sid, username, password string) error
	setSystemTime    func(ctx context.Context, sid, datetime, timezone string) error
	createBackup     func(ctx context.Context, sid, path string) error
	restoreBackup    func(ctx context.Context, sid, path string) error
	validateFirmware func(ctx context.Context, sid, path string) (openwrt.FirmwareInfo, error)
	installFirmware  func(ctx context.Context, sid, path string) error
	restart          func(ctx context.Context, sid string) error
	factoryReset     func(ctx context.Context, sid string) error
	// The uci two-phase lifecycle (ADR-010): canned pending changes, and records
	// of what the shell applied, confirmed, or reverted.
	changes     map[string][][]string
	changesErr  error
	changesRead *int      // counts UCIChanges calls
	reverts     *[]string // records reverted configs (pointer: fakeBackend is by value)
	applies     *[]int    // records UCIApply rollback timeouts
	confirms    *int      // counts UCIConfirm calls
	// procd's rc view (ADR-011): canned per-service states, and records of the
	// lifecycle actions the shell forwarded.
	rcStates map[string]openwrt.RCState
	rcErr    error
	rcInits  *[]string // records "service action" per RCInit
	// The helper's package verbs (ADR-011 §4): canned search results and
	// records of what the shell installed, removed, or refreshed.
	pkgCheckedAt      int64
	pkgFound          []openwrt.Package
	pkgInstalledList  []openwrt.Package
	pkgTotal          int
	pkgErr            error
	pkgUpdates        *int      // counts PkgUpdate calls
	pkgInstalls       *[]string // records installed names
	pkgRemoves        *[]string // records removed names
	pkgSingleUpgrades *[]string
	pkgFiles          []string
	// The update truths (Stage L): the upgradable set, the firmware check's answer,
	// and a count of the upgrades the shell started.
	pkgUpgradable    []openwrt.PackageUpgrade
	pkgUpgradableErr error
	pkgUpgrades      *int
	pkgUpgradeErr    error
	firmware         openwrt.FirmwareUpdate
	firmwareErr      error
	// The firmware act: a count of the upgrades the shell started (pointer:
	// fakeBackend is used by value) and the failure owut is made to report.
	firmwareUpgrades   *int
	firmwareUpgradeErr error
	// The wan side of the overview meters: canned uplink state and a queue of
	// device-counter snapshots, popped one per DeviceStats call (pointer:
	// fakeBackend is used by value); the last snapshot repeats.
	wan      openwrt.WANState
	wanConn  openwrt.WANConn
	wanErr   error
	wanCalls *int
	devStats *[]openwrt.DeviceStats
	devErr   error
	// The helper's brokered firewall read (ADR-007): the canned counters payload the
	// shell forwards to a declaring plugin, and the failure that degrades it away.
	fwCounters  json.RawMessage
	fwErr       error
	fwStatus    openwrt.FirewallStatus
	fwStatusErr error
	// The device's log ring and netifd's logical/device join — what the live
	// activity stream reads. logEntries is served whole, filtered by the caller's
	// cursor; logReads counts the polls (pointer: fakeBackend is used by value).
	firewallEntries    []openwrt.LogEntry
	firewallErr        error
	firewallReads      *int
	firewallGeneration string
	logEntries         []openwrt.LogEntry
	logErr             error
	logReads           *int
	netIfaces          []openwrt.NetIface
	netIfErr           error
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

// UCIConfig returns the canned read snapshot for a config (the shell brokers
// plugin reads through rpcd, ADR-007). An absent config yields an empty snapshot,
// mirroring an operator who may not read it.
func (f fakeBackend) UCIConfig(_ context.Context, _, config string) (map[string]any, error) {
	if f.uciReads != nil {
		*f.uciReads++
	}
	if f.uciReadErr != nil {
		return nil, f.uciReadErr
	}
	return f.uci[config], nil
}

// UCIAdd/UCIDelete record what the shell asked rpcd to do to realize a repeater's
// add/remove (ADR-005 §7), so the broker tests can assert it. A named section
// records its name too, since naming is the whole point of that shape.
func (f fakeBackend) UCIAdd(_ context.Context, _, config, secType, name string) (string, error) {
	record := config + " " + secType
	if name != "" {
		record += " " + name
	}
	if f.adds != nil {
		*f.adds = append(*f.adds, record)
	}
	if name != "" {
		return name, f.uciErr
	}
	return f.addReturns, f.uciErr
}

func (f fakeBackend) UCIDelete(_ context.Context, _, config, section, option string) error {
	if f.deletes != nil {
		record := config + "." + section
		if option != "" {
			record += "." + option
		}
		*f.deletes = append(*f.deletes, record)
	}
	if f.deleteErr != nil {
		return f.deleteErr(config, section, option)
	}
	return f.uciErr
}

// UCIOrder records the whole section sequence the shell staged for a dragged
// listing, so the reorder tests can assert what reached rpcd.
func (f fakeBackend) UCIOrder(_ context.Context, _, config string, sections []string) error {
	if f.orders != nil {
		*f.orders = append(*f.orders, config+": "+strings.Join(sections, ","))
	}
	return f.uciErr
}

func (f fakeBackend) UCIChanges(context.Context, string) (map[string][][]string, error) {
	if f.changesRead != nil {
		*f.changesRead++
	}
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

func (f fakeBackend) PkgBrowse(context.Context, string, string, int) (openwrt.PackagePage, error) {
	return openwrt.PackagePage{Packages: f.pkgFound, Total: f.pkgTotal, Count: f.pkgTotal, Installed: len(f.pkgInstalledList)}, f.pkgErr
}

func (f fakeBackend) PkgFiles(context.Context, string, string) ([]string, error) {
	return f.pkgFiles, f.pkgErr
}

func (f fakeBackend) PkgUpgradeOne(_ context.Context, _ string, name string) error {
	if f.pkgSingleUpgrades != nil {
		*f.pkgSingleUpgrades = append(*f.pkgSingleUpgrades, name)
	}
	return f.pkgUpgradeErr
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

func (f fakeBackend) PkgUpgradable(context.Context, string) ([]openwrt.PackageUpgrade, error) {
	return f.pkgUpgradable, f.pkgUpgradableErr
}

func (f fakeBackend) PkgUpgrade(context.Context, string) error {
	if f.pkgUpgrades != nil {
		*f.pkgUpgrades++
	}
	return f.pkgUpgradeErr
}

func (f fakeBackend) FirmwareCheck(context.Context, string) (openwrt.FirmwareUpdate, error) {
	return f.firmware, f.firmwareErr
}

func (f fakeBackend) FirmwareUpgrade(context.Context, string) error {
	if f.firmwareUpgrades != nil {
		*f.firmwareUpgrades++
	}
	return f.firmwareUpgradeErr
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

func (f fakeBackend) CreateBackup(ctx context.Context, sid, path string) error {
	if f.createBackup != nil {
		return f.createBackup(ctx, sid, path)
	}
	return nil
}

func (f fakeBackend) RestoreBackup(ctx context.Context, sid, path string) error {
	if f.restoreBackup != nil {
		return f.restoreBackup(ctx, sid, path)
	}
	return nil
}

func (f fakeBackend) ValidateFirmware(ctx context.Context, sid, path string) (openwrt.FirmwareInfo, error) {
	if f.validateFirmware != nil {
		return f.validateFirmware(ctx, sid, path)
	}
	return openwrt.FirmwareInfo{}, nil
}

func (f fakeBackend) InstallFirmware(ctx context.Context, sid, path string) error {
	if f.installFirmware != nil {
		return f.installFirmware(ctx, sid, path)
	}
	return nil
}

func (f fakeBackend) Restart(ctx context.Context, sid string) error {
	if f.restart != nil {
		return f.restart(ctx, sid)
	}
	return nil
}

func (f fakeBackend) FirewallCounters(context.Context, string) (json.RawMessage, error) {
	return f.fwCounters, f.fwErr
}

func (f fakeBackend) FirewallStatus(context.Context, string) (openwrt.FirewallStatus, error) {
	return f.fwStatus, f.fwStatusErr
}

func (f fakeBackend) LogRead(context.Context, string, int) ([]openwrt.LogEntry, error) {
	if f.logReads != nil {
		*f.logReads++
	}
	return f.logEntries, f.logErr
}

func (f fakeBackend) NetworkInterfaces(context.Context, string) ([]openwrt.NetIface, error) {
	return f.netIfaces, f.netIfErr
}

func (f fakeBackend) FactoryReset(ctx context.Context, sid string) error {
	if f.factoryReset != nil {
		return f.factoryReset(ctx, sid)
	}
	return nil
}

// errNoHelper answers the helper's raw reads and writes a test does not stand
// in for: a test that needs one embeds fakeBackend and overrides that method.
var errNoHelper = errors.New("fakeBackend: helper call not provided")

func (fakeBackend) DHCPState(context.Context, string) (json.RawMessage, error) {
	return nil, errNoHelper
}

func (fakeBackend) DNSState(context.Context, string) (json.RawMessage, error) {
	return nil, errNoHelper
}

func (fakeBackend) FirewallFiles(context.Context, string) (json.RawMessage, error) {
	return nil, errNoHelper
}

func (fakeBackend) NetworkState(context.Context, string) (json.RawMessage, error) {
	return nil, errNoHelper
}

func (fakeBackend) WirelessState(context.Context, string) (json.RawMessage, error) {
	return nil, errNoHelper
}

func (fakeBackend) StageConfigFile(context.Context, string, string, string, string) error {
	return errNoHelper
}

func (fakeBackend) NetworkSetUp(context.Context, string, string, bool) error {
	return errNoHelper
}

func (fakeBackend) NetworkRestart(context.Context, string, string) error {
	return errNoHelper
}

func (fakeBackend) AccessCredentials(context.Context, string) (openwrt.AccessCredentials, error) {
	return openwrt.AccessCredentials{}, errNoHelper
}

func (fakeBackend) SetAuthorizedKeys(context.Context, string, string, string) error {
	return errNoHelper
}

func (fakeBackend) SetWebCertificate(context.Context, string, string, string) error {
	return errNoHelper
}

// fakeTransport is the plugin-transport seam double (ADR-003/006): it returns a
// canned envelope or error and records the request the gateway forwarded, so the
// gateway is testable with no plugin process and no socket.
type fakeTransport struct {
	env          *plugin.Envelope
	err          error
	lastSocket   string
	lastReq      plugin.Request
	descriptions []plugin.Description // what Describe returns
	describeErr  error
	lastDescribe []plugin.DescribeChange
}

func (f *fakeTransport) Fetch(_ context.Context, socket string, req plugin.Request) (*plugin.Envelope, error) {
	f.lastSocket = socket
	f.lastReq = req
	return f.env, f.err
}

func (f *fakeTransport) Describe(_ context.Context, _ string, changes []plugin.DescribeChange, _ plugin.UCI) ([]plugin.Description, error) {
	f.lastDescribe = changes
	return f.descriptions, f.describeErr
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
	s.maintenanceDir = t.TempDir()
	// The update truth is a file on the device shared with the cron run
	// (ADR-014 §5); each test gets its own, so one test's answer is never
	// another's starting state.
	s.stateDir = t.TempDir()
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
		ID: "demo", Name: "Demo Plugin",
		Socket: "/run/verso/demo.sock", SchemaVersion: 1,
		Nav: []plugin.NavEntry{{Section: "Apps", Label: "Demo", Path: "/"}},
	}
}

// get reads a page as a signed-in browser does.
func get(t *testing.T, srv *Server, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	// Authenticate by default: mint a session and attach its cookie, so the
	// behavior tests exercise the page rather than the login redirect.
	token := srv.sessions.CreateWithMetadata("test-sid", "root", "", "")
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
	rec, _ := postPluginRequest(t, srv, path, form, nil)
	return rec
}

// postPluginAs is the same submission, marked as one kind of interaction — the
// header the shell's own controls send, which is how a preview says it is asking
// rather than saving.
func postPluginAs(t *testing.T, srv *Server, path string, form url.Values, interaction string) *httptest.ResponseRecorder {
	t.Helper()
	rec, _ := postPluginRequest(t, srv, path, form, func(req *http.Request) {
		req.Header.Set("X-Verso-Interaction", interaction)
	})
	return rec
}

// postPluginFromPanel is the same submission made from inside an open panel —
// marked as htmx marks its own requests, which is how the frame asks for its
// contents alone rather than for a page. The session's token comes back too,
// for what an answer leaves waiting in the session.
func postPluginFromPanel(t *testing.T, srv *Server, path string, form url.Values) (*httptest.ResponseRecorder, string) {
	t.Helper()
	return postPluginRequest(t, srv, path, form, func(req *http.Request) {
		req.Header.Set("HX-Request", "true")
	})
}

// postPluginRequest is the submission the three above dress: authenticated,
// CSRF-valid, and marked by mark before it is sent (nil sends it plain).
func postPluginRequest(t *testing.T, srv *Server, path string, form url.Values, mark func(*http.Request)) (*httptest.ResponseRecorder, string) {
	t.Helper()
	token := srv.sessions.CreateWithMetadata("test-sid", "root", "", "")
	sess, _ := srv.sessions.get(token)
	if form == nil {
		form = url.Values{}
	}
	form.Set("_csrf", sess.csrf)
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: token})
	if mark != nil {
		mark(req)
	}
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	return rec, token
}

// TestTheRailStandsOnTheTitlesBaseline: beside the page, the rail's first row
// reads on the h1's baseline — 19px of air above the rows, against the
// masthead's 16px and its 36px heading line — while the phone's drawer keeps
// its own 28px under the close button.
func TestTheRailStandsOnTheTitlesBaseline(t *testing.T) {
	body := get(t, newServer(t, fakeBackend{}), "/").Body.String()
	if !strings.Contains(body, `<nav class="min-h-0 flex-1 overflow-y-auto pt-7 pb-4 md:pt-4.75">`) {
		t.Error("the rail's rows do not stand on the title's baseline")
	}
}

// TestTopBarCarriesTheNameplateAndTheWayOut: the bar across the top holds the
// device's identity on the left — the hostname, the way home — and the way out
// on the right, aligned to the content column rather than to the bar's edge.
// On a phone it also holds the hamburger. The rail under it is a list of
// destinations and carries neither.
func TestTopBarCarriesTheNameplateAndTheWayOut(t *testing.T) {
	body := get(t, newServer(t, fakeBackend{hn: "gdk-edge-01"}), "/").Body.String()
	start, end := strings.Index(body, "<header"), strings.Index(body, "</header>")
	if start < 0 || end < 0 {
		t.Fatalf("no top bar:\n%s", firstLines(body, 0))
	}
	header := body[start:end]
	for _, want := range []string{
		`href="/"`, ">gdk-edge-01</a>",
		`aria-label="Open menu"`,
		`action="/logout"`, ">root<", ">Log out<",
		"md:left-72", "max-w-6xl",
	} {
		if !strings.Contains(header, want) {
			t.Errorf("top bar missing %q:\n%s", want, header)
		}
	}
	// The hamburger says whether the rail it opens is open, and which element
	// that rail is.
	for _, want := range []string{`:aria-expanded="expanded"`, `aria-controls="verso-rail"`} {
		if !strings.Contains(header, want) {
			t.Errorf("menu button missing %q:\n%s", want, header)
		}
	}
	// The rail is found by the id the menu button controls; the bar's own
	// staged-changes drawer is an aside too, and comes first.
	railAt := strings.Index(body, `<aside id="verso-rail"`)
	if railAt < 0 {
		t.Fatalf("the rail must carry the id the menu button controls:\n%s", firstLines(body, 0))
	}
	aside := body[railAt : railAt+strings.Index(body[railAt:], "</aside>")]
	for _, unwanted := range []string{"gdk-edge-01", "/logout", "Log out"} {
		if strings.Contains(aside, unwanted) {
			t.Errorf("the rail must not carry %q:\n%s", unwanted, aside)
		}
	}
}

// TestPluginSubpagesOpenInTheRail: a plugin's declared subpages hang under its
// rail row while that row is the one you are in — shell-built hrefs, the active
// one marked from the request path. There is no bar above the content: the rail
// carries the whole path, so a page's depth is read in one place.
func TestPluginSubpagesOpenInTheRail(t *testing.T) {
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
	if strings.Contains(body, `aria-label="Subpages"`) {
		t.Error("the top subpage bar should be gone; the rail opens them instead")
	}
	nav := body[strings.Index(body, "<nav "):strings.Index(body, "</nav>")]
	for _, want := range []string{
		`href="/plugins/demo/dnsdhcp"`,
		`href="/plugins/demo/dnsdhcp/config"`,
		"border-l border-rule",              // the hairline the subpages hang from
		`text-meta rotate-90 ml-auto"><svg`, // the open row's chevron, turned down
		`<path d="m9 18 6-6-6-6" />`,        // the one chevron every row that opens wears
	} {
		if !strings.Contains(nav, want) {
			t.Errorf("rail missing %q:\n%s", want, nav)
		}
	}
	// The active marker sits on Leases (the requested path), not Configuration,
	// and the open parent hands it down rather than keeping one of its own.
	if !strings.Contains(nav, `href="/plugins/demo/dnsdhcp" aria-current="page"`) {
		t.Errorf("active subpage not marked on the requested path:\n%s", nav)
	}
	if strings.Contains(nav, `href="/plugins/demo/dnsdhcp/config" aria-current`) {
		t.Error("the inactive subpage must not carry aria-current")
	}
	if !strings.Contains(nav, "shadow-[inset_-0.125rem_0_0_var(--color-ink)]") {
		t.Error("the active subpage should wear the rail's marker, on the rail's content edge")
	}
	// The marker is one thing of its own, behind the words, so it can travel
	// from the page you leave to the one you open: once in the rail, inside the
	// place you are on.
	if n := strings.Count(nav, "data-verso-nav-marker"); n != 1 {
		t.Fatalf("rail draws %d markers, want exactly one", n)
	}
	here := nav[strings.Index(nav, `href="/plugins/demo/dnsdhcp" aria-current="page"`):]
	if !strings.Contains(here[:strings.Index(here, "</a>")], `data-verso-nav-marker aria-hidden="true" style="--verso-vt: verso-nav-marker"`) {
		t.Errorf("the marker must sit in the subpage you are on:\n%s", nav)
	}
	// The branch and each subpage's words are named for the page change: the
	// branch unfolds, and its subpages arrive one after another, in order.
	for _, want := range []string{
		// A branch is its parent's: one branch never turns into another's.
		`style="--verso-vt: verso-nav-branch-plugins-demo; --verso-vt-class: verso-nav-branch"`,
		`style="--verso-vt: verso-nav-sub-plugins-demo-dnsdhcp; --verso-vt-class: verso-nav-sub verso-nav-sub-0"`,
		`style="--verso-vt: verso-nav-sub-plugins-demo-dnsdhcp-config; --verso-vt-class: verso-nav-sub verso-nav-sub-1"`,
	} {
		if !strings.Contains(nav, want) {
			t.Errorf("rail missing %q:\n%s", want, nav)
		}
	}
}

// What waits on a rail row is said at its end in the action colour, led by the
// rail's square, with the whole sentence on the row for a pointer to rest on
// and a screen reader to read.
func TestARailRowSaysWhatWaitsThere(t *testing.T) {
	s := newServer(t, fakeBackend{})
	waiting := updatecheck.Truth{Firmware: openwrt.FirmwareUpdate{State: openwrt.FirmwareUpdateAvailable}, CheckedAt: time.Now()}
	if err := updatecheck.Write(s.stateDir, waiting); err != nil {
		t.Fatal(err)
	}
	body := get(t, s, "/").Body.String()
	nav := body[strings.Index(body, "<nav "):strings.Index(body, "</nav>")]
	row := nav[strings.Index(nav, `title="New firmware is ready to install."`):]
	row = row[:strings.Index(row, "</a>")]
	// The row's name never gives way to what waits there: the words do, and
	// the whole sentence stays on the row.
	for _, want := range []string{
		`<span class="min-w-0 shrink-0 truncate">System</span>`,
		`<span class="ml-auto flex min-w-0 items-center gap-2 text-sm font-medium tabular-nums text-denim">`,
		`<span aria-hidden="true" class="size-1.25 shrink-0 rounded-[1px] bg-denim"></span><span class="min-w-0 truncate">New firmware</span></span>`,
	} {
		if !strings.Contains(row, want) {
			t.Errorf("System row missing %q:\n%s", want, row)
		}
	}
}

// A closed row that opens into subpages points the way it opens; the open one
// turns down over its branch. It is one chevron, turned, and named for its row,
// so a page change turns it rather than swapping one picture for another.
func TestARowThatOpensPointsTheWay(t *testing.T) {
	m := demoManifest()
	for i := range m.Nav {
		m.Nav[i].Pages = true
	}
	tr := &fakeTransport{env: &plugin.Envelope{
		SchemaVersion: 1, Title: "DNS & DHCP", Status: http.StatusOK,
		Pages:  []plugin.PageTab{{Label: "Leases", Path: "dnsdhcp"}},
		Widget: json.RawMessage(`{"type":"card","children":[]}`),
	}}
	s := newServerWith(t, fakeBackend{}, tr, []plugin.Manifest{m})
	right := `<path d="m9 18 6-6-6-6" />`
	chevron := `style="--verso-vt: verso-nav-chevron-plugins-demo" class="verso-vt flex shrink-0 text-meta`
	rowOf := func(body, href string) string {
		nav := body[strings.Index(body, "<nav "):strings.Index(body, "</nav>")]
		row := nav[strings.Index(nav, `href="`+href+`"`):]
		return row[:strings.Index(row, "</a>")]
	}
	closed := rowOf(get(t, s, "/").Body.String(), "/plugins/demo/")
	if !strings.Contains(closed, chevron+` ml-auto">`) || !strings.Contains(closed, right) {
		t.Errorf("a closed row with subpages must point right, unturned:\n%s", closed)
	}
	open := rowOf(get(t, s, "/plugins/demo/dnsdhcp").Body.String(), "/plugins/demo/")
	if !strings.Contains(open, chevron+` rotate-90 ml-auto">`) || !strings.Contains(open, right) {
		t.Errorf("the open row must turn the same chevron down over its branch:\n%s", open)
	}
	if home := rowOf(get(t, s, "/").Body.String(), "/"); strings.Contains(home, right) || strings.Contains(home, "verso-nav-chevron") {
		t.Errorf("a row with no subpages carries no chevron:\n%s", home)
	}
}

// A page served over HTTPS names the rules that fetch the next page the moment
// a rail row is pressed, so it is on its way before the press ends. The rules
// come from a file of the shell's own: the page's policy runs no script written
// inline. Over plain HTTP a browser prefetches nothing, so the page names no
// rules there and the router is never asked for a file no one can use.
func TestARailRowIsFetchedAsItIsPressed(t *testing.T) {
	s := newServerWith(t, fakeBackend{}, &fakeTransport{}, nil)
	if got := get(t, s, "/").Header().Get("Speculation-Rules"); got != "" {
		t.Fatalf("a page over plain HTTP names rules %q no browser will act on", got)
	}
	page := get(t, s, "https://example.com/")
	if got := page.Header().Get("Speculation-Rules"); got != `"/assets/speculation-rules.json"` {
		t.Fatalf("Speculation-Rules = %q, want the shell's rules file", got)
	}
	rules := get(t, s, "/assets/speculation-rules.json")
	if ct := rules.Header().Get("Content-Type"); ct != "application/speculationrules+json" {
		t.Fatalf("rules served as %q; a browser takes speculation rules only as application/speculationrules+json", ct)
	}
	var doc struct {
		Prefetch []struct {
			Where     map[string]string `json:"where"`
			Eagerness string            `json:"eagerness"`
		} `json:"prefetch"`
	}
	if err := json.Unmarshal(rules.Body.Bytes(), &doc); err != nil {
		t.Fatalf("rules are not JSON: %v\n%s", err, rules.Body.String())
	}
	if len(doc.Prefetch) != 1 || doc.Prefetch[0].Eagerness != "conservative" ||
		doc.Prefetch[0].Where["selector_matches"] != "[data-verso-nav-rows] a[href]" {
		t.Fatalf("rules = %+v, want one press-time prefetch of the rail's links", doc)
	}
}

// TestEveryPageWearsTheOneMasthead: the shell draws one masthead — the sand
// bar, the heading and what acts on the page — for its own pages and a
// plugin's alike, with no second shape (an eyebrow over the heading, a
// display heading off the bar) for a page to drift into.
func TestEveryPageWearsTheOneMasthead(t *testing.T) {
	src, err := os.ReadFile("templates/page.html.tmpl")
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(src), "data-verso-masthead{{end}}"); n != 1 {
		t.Errorf("page.html.tmpl draws %d mastheads, want one", n)
	}
	tr := &fakeTransport{env: &plugin.Envelope{
		SchemaVersion: 1, Title: "System",
		Widget: json.RawMessage(`{"type":"card","children":[]}`),
	}}
	s := newServerWith(t, fakeBackend{}, tr, []plugin.Manifest{demoManifest()})
	body := get(t, s, "/plugins/demo/").Body.String()
	if !strings.Contains(body, `<div data-verso-masthead class="mb-6 py-4">`) || strings.Contains(body, `class="verso-kicker"`) {
		t.Errorf("a plugin's page does not wear the one masthead:\n%s", body)
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

func demoCountersManifest() plugin.Manifest {
	m := demoManifest()
	m.ACL = plugin.ACL{Read: []plugin.ACLScope{
		{Scope: "uci", Object: "firewall", Function: "read"},
		{Scope: "ubus", Object: "verso", Function: "firewallCounters"},
	}}
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

// TestIndexRendersOverview: the landing page renders the overview — the verdict, the traffic section, the System panel with live firmware/kernel/uptime
// from the backend, and the interface listing. The roster itself lives on its own
// page and is not repeated here.
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
		"Internet is working", "online", "Internet traffic", "System", "Interfaces",
		"OpenWrt 25.12.4", "Linux 6.12.101", "1 h 01 min", // live System facts
		"Connected", "for 2 h 14 min", "pppoe-upstream", // live WAN state, device, and uptime
	} {
		if !strings.Contains(body, want) {
			t.Errorf("GET /: body missing %q", want)
		}
	}
	if wanCalls != 1 {
		t.Errorf("WAN discovery calls = %d, want one consistent page snapshot", wanCalls)
	}
	if strings.Contains(body, `data-verso-row="interface:`) {
		t.Error("the homepage must not include the interfaces table")
	}
}

// TestIndexDegradesWhenBackendFails: the overview is hardcoded for now, so the
// page renders (never 500s) even when the backend is unreachable.
func TestIndexDegradesWhenBackendFails(t *testing.T) {
	rec := get(t, newServer(t, fakeBackend{err: errors.New("ubus down")}), "/")

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /: status = %d, want 200 (must degrade, not 500)", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "text-3xl leading-[1.1] font-semibold tracking-tight") {
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

// TestPluginFilterOnlyOverALiveListing: the page-wide lens is the shell's rule,
// not a constant every plugin carries. A page that lists what it has is read by
// scrolling and found in with the browser's own find, however long it is, so
// the shell removes the widget outright — no dead dock, no shortcut into
// nothing. Only a live listing keeps it: its rows arrive while it is read, and
// the browser's find cannot hold a question across them.
func TestPluginFilterOnlyOverALiveListing(t *testing.T) {
	page := func(rows int, stream string) json.RawMessage {
		var b strings.Builder
		b.WriteString(`{"type":"stack","children":[` +
			`{"type":"filter","placeholder":"Filter — zone, port…"},` +
			`{"type":"table",` + stream + `"columns":[{"label":"Name"}],"rows":[`)
		for i := range rows {
			if i > 0 {
				b.WriteString(",")
			}
			fmt.Fprintf(&b, `{"id":"r%d","cells":[{"text":"row %d"}]}`, i, i)
		}
		b.WriteString(`]}]}`)
		return json.RawMessage(b.String())
	}
	live := fmt.Sprintf(`"stream":{"source":%q},`, widget.StreamSourceFirewallLog)
	for _, tc := range []struct {
		name   string
		widget json.RawMessage
		want   bool
	}{
		{"a long static listing", page(200, ""), false},
		{"a live listing", page(0, live), true},
	} {
		tr := &fakeTransport{env: &plugin.Envelope{
			SchemaVersion: 1, Title: "Firewall", Widget: tc.widget,
		}}
		s := newServerWith(t, fakeBackend{}, tr, []plugin.Manifest{demoManifest()})
		body := get(t, s, "/plugins/demo/").Body.String()
		if got := strings.Contains(body, "data-verso-filter"); got != tc.want {
			t.Errorf("%s: filter rendered = %v, want %v", tc.name, got, tc.want)
		}
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

// TestPluginUbusReadBrokered: a plugin declaring an allowlisted helper read in
// acl.read receives that helper's result in its request, read with the operator's
// sid (ADR-007) — live state the plugin cannot reach itself.
func TestPluginUbusReadBrokered(t *testing.T) {
	counters := json.RawMessage(`{"counters":[{"chain":"input_wan","name":"Allow-Ping","packets":12,"bytes":1008}]}`)
	tr := &fakeTransport{env: &plugin.Envelope{
		SchemaVersion: 1, Title: "Firewall", Widget: json.RawMessage(`{"type":"card"}`),
	}}
	s := newServerWith(t, fakeBackend{fwCounters: counters}, tr, []plugin.Manifest{demoCountersManifest()})

	if rec := get(t, s, "/plugins/demo/"); rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	got, ok := tr.lastReq.Ubus["firewallCounters"]
	if !ok {
		t.Fatalf("forwarded request carried no brokered read: %+v", tr.lastReq.Ubus)
	}
	if string(got) != string(counters) {
		t.Errorf("brokered read = %s, want the helper's JSON verbatim", got)
	}
}

// TestPluginUndeclaredUbusReadNotBrokered: neither a plugin that declares no helper
// read nor one that names a function outside the shell's closed set receives one —
// the declaration alone cannot widen what the shell will read.
func TestPluginUndeclaredUbusReadNotBrokered(t *testing.T) {
	unknown := demoManifest()
	unknown.ACL = plugin.ACL{Read: []plugin.ACLScope{
		{Scope: "ubus", Object: "verso", Function: "installFirmware"},
	}}
	for name, m := range map[string]plugin.Manifest{
		"no read acl":         demoManifest(),
		"uci reads only":      demoReadManifest(),
		"un-allowlisted verb": unknown,
	} {
		tr := &fakeTransport{env: &plugin.Envelope{
			SchemaVersion: 1, Title: "Demo", Widget: json.RawMessage(`{"type":"card"}`),
		}}
		backend := fakeBackend{fwCounters: json.RawMessage(`{"counters":[]}`)}
		s := newServerWith(t, backend, tr, []plugin.Manifest{m})

		if rec := get(t, s, "/plugins/demo/"); rec.Code != http.StatusOK {
			t.Fatalf("%s: status = %d, want 200", name, rec.Code)
		}
		if tr.lastReq.Ubus != nil {
			t.Errorf("%s: got a brokered read: %+v", name, tr.lastReq.Ubus)
		}
	}
}

// TestPluginUbusReadDegradesOnBackendError: a helper read that fails contributes
// nothing and the page still renders — reads are never a 4xx/5xx, the same posture
// the uci snapshot takes.
func TestPluginUbusReadDegradesOnBackendError(t *testing.T) {
	tr := &fakeTransport{env: &plugin.Envelope{
		SchemaVersion: 1, Title: "Firewall", Widget: json.RawMessage(`{"type":"card"}`),
	}}
	backend := fakeBackend{fwErr: errors.New("helper unavailable")}
	s := newServerWith(t, backend, tr, []plugin.Manifest{demoCountersManifest()})

	rec := get(t, s, "/plugins/demo/")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (a failed read must degrade, not fail)", rec.Code)
	}
	if tr.lastReq.Ubus != nil {
		t.Errorf("failed read still reached the plugin: %+v", tr.lastReq.Ubus)
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

// demoReorderManifest declares the firewall config for both reading and writing —
// what a plugin whose listing drags must declare for the shell to stage the order.
func demoReorderManifest() plugin.Manifest {
	m := demoManifest()
	m.ACL = plugin.ACL{
		Read:  []plugin.ACLScope{{Scope: "uci", Object: "firewall", Function: "read"}},
		Write: []plugin.ACLScope{{Scope: "uci", Object: "firewall", Function: "write"}},
	}
	return m
}

// reorderSnapshot is a firewall config whose rule sections are interleaved with
// sections of other types — the shape that proves a reorder moves only the rows
// the listing showed. `.index` arrives as an integer, the way rpcd's blobmsg
// carries it.
func reorderSnapshot() map[string]map[string]any {
	section := func(secType string, index int64) map[string]any {
		return map[string]any{".type": secType, ".index": index}
	}
	return map[string]map[string]any{"firewall": {
		"defaults":         section("defaults", 0),
		"lan":              section("zone", 1),
		"allow_dhcp_renew": section("rule", 2),
		"allow_ping":       section("rule", 3),
		"wan":              section("zone", 4),
		"block_telnet":     section("rule", 5),
	}}
}

// reorderForm is what verso.js posts when a row is dropped: the config the rows
// are sections of, and every reorderable row id in the table's new order.
func reorderForm(ids ...string) url.Values {
	f := url.Values{"_uci_order_config": {"firewall"}}
	f["_uci_order"] = ids
	return f
}

// TestPluginReorderStagesTheWholeSequence: a dropped row posts the listing's new
// id order, and the shell stages a `uci order` over the config's *whole* section
// sequence — the posted ids refilling the slots they already held, so the zone and
// defaults sections interleaved among the rules never move.
func TestPluginReorderStagesTheWholeSequence(t *testing.T) {
	orders := []string{}
	tr := repeaterTransport()
	backend := fakeBackend{access: true, orders: &orders, uci: reorderSnapshot()}
	s := newServerWith(t, backend, tr, []plugin.Manifest{demoReorderManifest()})

	rec := postPlugin(t, s, "/plugins/demo/", reorderForm("allow_ping", "allow_dhcp_renew", "block_telnet"))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	want := "firewall: defaults,lan,allow_ping,allow_dhcp_renew,wan,block_telnet"
	if len(orders) != 1 || orders[0] != want {
		t.Errorf("UCIOrder calls = %v, want one %q", orders, want)
	}
	if tr.lastReq.Method != http.MethodGet {
		t.Errorf("re-render method = %q, want GET (the order was staged, then rendered)", tr.lastReq.Method)
	}
}

// TestPluginReorderThatMovesNothingStagesNothing: an abandoned drag announces the
// sequence it started from, and a page that already reads that way has nothing to
// save — the operator's staged changes stay as they were.
func TestPluginReorderThatMovesNothingStagesNothing(t *testing.T) {
	orders := []string{}
	tr := repeaterTransport()
	backend := fakeBackend{access: true, orders: &orders, uci: reorderSnapshot()}
	s := newServerWith(t, backend, tr, []plugin.Manifest{demoReorderManifest()})

	rec := postPlugin(t, s, "/plugins/demo/", reorderForm("allow_dhcp_renew", "allow_ping", "block_telnet"))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if len(orders) != 0 {
		t.Errorf("an unchanged order must stage nothing: %v", orders)
	}
}

// TestPluginReorderRefusedForUndeclaredConfig: the reorder is bounded by the same
// declared write surface every other staged write is — an undeclared config is
// refused and nothing is staged.
func TestPluginReorderRefusedForUndeclaredConfig(t *testing.T) {
	orders := []string{}
	tr := repeaterTransport()
	backend := fakeBackend{access: true, orders: &orders, uci: reorderSnapshot()}
	s := newServerWith(t, backend, tr, []plugin.Manifest{demoRepeaterManifest()})

	rec := postPlugin(t, s, "/plugins/demo/", reorderForm("allow_ping"))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
	if len(orders) != 0 {
		t.Errorf("an undeclared-config reorder must stage nothing: %v", orders)
	}
}

// TestPluginReorderRefusedForUnknownSection: every posted id must name a section
// the config really holds, so a stale page cannot order the listing into a shape
// the device does not have.
func TestPluginReorderRefusedForUnknownSection(t *testing.T) {
	orders := []string{}
	tr := repeaterTransport()
	backend := fakeBackend{access: true, orders: &orders, uci: reorderSnapshot()}
	s := newServerWith(t, backend, tr, []plugin.Manifest{demoReorderManifest()})

	rec := postPlugin(t, s, "/plugins/demo/", reorderForm("allow_ping", "cfg_gone"))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	if len(orders) != 0 {
		t.Errorf("an unusable order must stage nothing: %v", orders)
	}
}

// TestPluginReorderContainedOnBackendError: rpcd refusing the order is a plain
// "try again" notice, never a crash — the same containment every staged write has.
func TestPluginReorderContainedOnBackendError(t *testing.T) {
	orders := []string{}
	tr := repeaterTransport()
	backend := fakeBackend{
		access: true, orders: &orders, uci: reorderSnapshot(),
		uciErr: errors.New("rpcd said no"),
	}
	s := newServerWith(t, backend, tr, []plugin.Manifest{demoReorderManifest()})

	rec := postPlugin(t, s, "/plugins/demo/", reorderForm("allow_ping", "allow_dhcp_renew", "block_telnet"))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Save failed") {
		t.Errorf("a failed reorder should render the contained notice:\n%s", rec.Body.String())
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
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303: a staged save is read again", rec.Code)
	}
	if len(calls) != 1 {
		t.Fatalf("UCISet calls = %d, want 1 (the shell must broker the write)", len(calls))
	}
	w := calls[0]
	if w.sid != "test-sid" || w.config != "system" || w.section != "@system[0]" || w.values["hostname"] != "verso-lab" {
		t.Errorf("brokered write = %+v; want sid test-sid, system/@system[0], hostname=verso-lab", w)
	}
}

// TestPluginPreviewStagesNothing: a submission marked as a preview asks the
// plugin what the form on screen would write and gets back that block alone.
// The operator is still typing — half a port number is not a change anybody
// asked to make — so nothing is staged, whatever the plugin returns with it.
func TestPluginPreviewStagesNothing(t *testing.T) {
	calls := []uciWrite{}
	tr := &fakeTransport{env: &plugin.Envelope{
		SchemaVersion: 1, Title: "Saved", Status: http.StatusOK,
		Widget: json.RawMessage(`{"type":"card","children":[
			{"type":"field","name":"hostname","label":"Name","kind":"text","value":"verso-lab"},
			{"type":"code","label":"/etc/config/system","value":"config system\n\toption hostname 'verso-lab'\n","live":true}]}`),
		// The plugin answers a POST the way it always does, commit and all. The
		// shell is what decides this one is a question.
		Commit: []plugin.CommitOp{{Config: "system", Section: "@system[0]", Values: map[string]any{"hostname": "verso-lab"}}},
	}}
	s := newServerWith(t, fakeBackend{access: true, writes: &calls}, tr, []plugin.Manifest{demoACLManifest()})

	rec := postPluginAs(t, s, "/plugins/demo/", url.Values{"hostname": {"verso-lab"}}, "preview")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if len(calls) != 0 {
		t.Fatalf("a preview staged %d write(s); it must stage none: %+v", len(calls), calls)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "data-verso-preview") {
		t.Errorf("the answer is not the preview block:\n%s", body)
	}
	if strings.Contains(body, `name="hostname"`) {
		t.Errorf("the answer carries the form as well as the preview — the page is already on screen:\n%s", body)
	}
}

// TestPluginReshapeStagesNothing: a choice that reshapes its form asks the
// plugin for the form again with the values on screen. The shell marks the
// question for the plugin (_action=reshape) and answers it with the panel the
// plugin drew, whatever else came with it: nothing is staged, and nothing on
// screen is refused — a half-typed address is still being typed.
func TestPluginReshapeStagesNothing(t *testing.T) {
	calls := []uciWrite{}
	env := openPanelEnvelope(http.StatusOK, nil, `{"type":"field","name":"ipaddr","label":"Address","datatype":"ip4addr","value":"10.0."}`)
	env.Commit = []plugin.CommitOp{{Config: "system", Section: "@system[0]", Values: map[string]any{"hostname": "x"}}}
	tr := &fakeTransport{env: env}
	s := newServerWith(t, fakeBackend{access: true, writes: &calls}, tr, []plugin.Manifest{demoACLManifest()})

	rec, _ := postPluginRequest(t, s, "/plugins/demo/?open=r1", url.Values{"kind": {"bridge"}, "ipaddr": {"10.0."}}, func(req *http.Request) {
		req.Header.Set("HX-Request", "true")
		req.Header.Set("X-Verso-Interaction", "reshape")
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if len(calls) != 0 {
		t.Fatalf("a reshape staged %d write(s); it must stage none: %+v", len(calls), calls)
	}
	if got := tr.lastReq.Form["_action"]; len(got) != 1 || got[0] != "reshape" {
		t.Errorf("the plugin was not told the form is being reshaped: _action = %q", got)
	}
	if rec.Header().Get("HX-Redirect") != "" || rec.Header().Get("HX-Reswap") != "" {
		t.Errorf("a reshape is answered in place, not sent away: %v", rec.Header())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `name="ipaddr"`) || strings.Contains(body, "<main") {
		t.Errorf("the answer is not the panel alone:\n%s", body)
	}
	if strings.Contains(body, `aria-invalid="true"`) {
		t.Errorf("a reshape refused a value still being typed:\n%s", body)
	}
}

// TestPluginReshapeMarkIsTheShells: only the shell says a submission is a
// reshape. A form that posts _action=reshape itself reaches the plugin without
// it, so a plugin can trust the marker it reads.
func TestPluginReshapeMarkIsTheShells(t *testing.T) {
	tr := &fakeTransport{env: openPanelEnvelope(http.StatusOK, nil, `{"type":"field","name":"h","label":"Name"}`)}
	s := newServerWith(t, fakeBackend{access: true, writes: &[]uciWrite{}}, tr, []plugin.Manifest{demoACLManifest()})
	postPluginFromPanel(t, s, "/plugins/demo/?open=r1", url.Values{"_action": {"reshape"}, "h": {"x"}})
	if got := tr.lastReq.Form["_action"]; len(got) != 0 {
		t.Errorf("a posted reshape marker reached the plugin: _action = %q", got)
	}
}

// openPanelEnvelope is a listing with one row's panel open on a form — the
// shape a rules listing answers with while a rule is being edited beside it,
// and the shape it answers that panel's submission with.
func openPanelEnvelope(status int, notice *plugin.Notice, field string) *plugin.Envelope {
	return &plugin.Envelope{
		SchemaVersion: 1, Title: "Rules", Status: status, Notice: notice,
		Widget: json.RawMessage(`{"type":"table","columns":[{"label":"Name","kind":"name"}],"rows":[
			{"id":"r1","cells":[{"text":"Allow-Ping"}],"panel":"/plugins/demo/?open=r1","drawer":{"title":"Allow-Ping","open":true,
				"tabs":[{"label":"Match","href":"/plugins/demo/?open=r1","active":true}],
				"children":[{"type":"form","submit":"Save & apply","fields":[` + field + `]}]}},
			{"id":"r2","cells":[{"text":"Allow-DHCP"}],"panel":"/plugins/demo/?open=r2"}]}`),
	}
}

// TestPluginPanelSubmissionThatStagedValuesIsDone: a panel's form posts from
// inside the frame that holds it, marked as htmx marks its own requests. The
// submission is a real save — the plugin's write is staged through rpcd exactly
// as a page post's is — and, a row's values having changed and nothing else, the
// panel is done: the answer is the outcome alone, the plugin's half and the
// shell's, and the frame is told to swap nothing, because the panel is closing
// on it and the listing behind it is already on screen.
func TestPluginPanelSubmissionThatStagedValuesIsDone(t *testing.T) {
	calls := []uciWrite{}
	env := openPanelEnvelope(http.StatusOK, &plugin.Notice{Level: "success", Text: "Rule saved."},
		`{"type":"field","name":"src","label":"From","kind":"text","value":"wan"}`)
	env.Commit = []plugin.CommitOp{{Config: "system", Section: "r1", Values: map[string]any{"src": "wan"}}}
	tr := &fakeTransport{env: env}
	s := newServerWith(t, fakeBackend{access: true, writes: &calls}, tr, []plugin.Manifest{demoACLManifest()})

	rec, token := postPluginFromPanel(t, s, "/plugins/demo/?open=r1", url.Values{"src": {"wan"}})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200:\n%s", rec.Code, rec.Body.String())
	}
	if len(calls) != 1 || calls[0].section != "r1" {
		t.Fatalf("staged %+v, want the panel's one write on r1", calls)
	}
	if got := tr.lastReq.Form["src"]; tr.lastReq.Method != http.MethodPost || len(got) != 1 || got[0] != "wan" {
		t.Errorf("the plugin should be asked to save what was submitted, got %s %v", tr.lastReq.Method, tr.lastReq.Form)
	}
	if got := rec.Header().Get("HX-Reswap"); got != "none" {
		t.Errorf("HX-Reswap = %q, want none: the panel is closing, not being replaced", got)
	}
	body := rec.Body.String()
	// The panel closes on a change that is only staged: the chip says it
	// waits, and the row it came from sends it there (verso-commit.js).
	for _, unwanted := range []string{"<form", "<html", "<table", "Allow-DHCP", `<div class="verso-flash`, "Nothing is live"} {
		if strings.Contains(body, unwanted) {
			t.Errorf("the outcome is said alone, not with %q:\n%s", unwanted, body)
		}
	}
	// The outcome went to the frame, so nothing waits in the session for a page
	// that is not going to be drawn.
	if variant, message := s.sessions.TakeFlash(token); message != "" {
		t.Errorf("flash = %q %q, want none", variant, message)
	}
}

// TestPluginPanelSubmissionThatComputedAnswersWithThePanel: a panel's form may
// post to have the plugin compute on it rather than save — a secondary action,
// a generated key — and the plugin answers with the panel and no write. That
// panel swaps in where the form was, marked to be posted again in place.
func TestPluginPanelSubmissionThatComputedAnswersWithThePanel(t *testing.T) {
	env := openPanelEnvelope(http.StatusOK, nil,
		`{"type":"field","name":"src","label":"From","kind":"text","value":"computed"}`)
	s := newServerWith(t, fakeBackend{access: true}, &fakeTransport{env: env}, []plugin.Manifest{demoACLManifest()})

	rec, _ := postPluginFromPanel(t, s, "/plugins/demo/?open=r1", url.Values{"_action": {"compute"}})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200:\n%s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("HX-Reswap") != "" {
		t.Error("a panel that has more to say is swapped in, not dismissed")
	}
	body := rec.Body.String()
	for _, want := range []string{`value="computed"`, `hx-target="closest [data-verso-panel]"`} {
		if !strings.Contains(body, want) {
			t.Errorf("panel answer missing %q:\n%s", want, body)
		}
	}
	// The frame, the listing behind it, and the chrome are already on screen.
	for _, unwanted := range []string{"<html", "<table", "Allow-DHCP", "verso-staged", "x-teleport"} {
		if strings.Contains(body, unwanted) {
			t.Errorf("a panel answer must not carry %q:\n%s", unwanted, body)
		}
	}
}

// TestPluginPanelSubmissionThatAddedARowGoesToThePage: a submission that made a
// section — a rule that did not exist — changes which rows the listing has, and
// only a fresh page can show that. The plugin answered with the blank panel
// still open, but the frame is sent to the page instead, and the outcome waits
// there as the flash.
func TestPluginPanelSubmissionThatAddedARowGoesToThePage(t *testing.T) {
	calls := []uciWrite{}
	env := openPanelEnvelope(http.StatusOK, &plugin.Notice{Level: "success", Text: "Rule added."},
		`{"type":"field","name":"src","label":"From","kind":"text","value":"wan"}`)
	env.Commit = []plugin.CommitOp{{Config: "system", Type: "rule", Values: map[string]any{"src": "wan"}}}
	s := newServerWith(t, fakeBackend{access: true, writes: &calls, addReturns: "cfg0a1b2c"}, &fakeTransport{env: env}, []plugin.Manifest{demoACLManifest()})

	rec, token := postPluginFromPanel(t, s, "/plugins/demo/?open=new", url.Values{"src": {"wan"}})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200:\n%s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("HX-Redirect"); got != "/plugins/demo/" {
		t.Errorf("HX-Redirect = %q, want the listing's own address", got)
	}
	if len(calls) != 1 || calls[0].section != "cfg0a1b2c" {
		t.Errorf("staged %+v, want the new section's values", calls)
	}
	if variant, message := s.sessions.TakeFlash(token); message != "" {
		t.Errorf("flash = %q %q, want none: the chip says a staged change waits", variant, message)
	}
}

// TestAStagedOptionIsMarkedOnEveryVisit: the router holds the stage, so a
// page drawn at any time marks each control whose option waits — a visit
// after navigating away, not only the landing after a save — and only that
// option at that address. The chip and the marks read one stage per request.
func TestAStagedOptionIsMarkedOnEveryVisit(t *testing.T) {
	var reads int
	b := fakeBackend{access: true, changes: map[string][][]string{
		"system": {{"set", "sys", "hostname", "edge"}, {"set", "other", "timezone", "UTC"}},
	}, changesRead: &reads}
	tr := &fakeTransport{env: &plugin.Envelope{
		SchemaVersion: 1, Title: "General", Status: http.StatusOK,
		Widget: json.RawMessage(`{"type":"form","style":"page","target":"system.sys","fields":[
		  {"type":"field","name":"hostname","label":"Router name","key":"hostname","value":"edge"},
		  {"type":"field","name":"timezone","label":"Time zone","key":"timezone","value":"UTC"}]}`),
	}}
	s := newServerWith(t, b, tr, []plugin.Manifest{demoACLManifest()})
	body := get(t, s, "/plugins/demo/").Body.String()
	if n := strings.Count(body, "data-verso-staged-row"); n != 1 {
		t.Errorf("want the one waiting option marked, got %d marks:\n%s", n, body)
	}
	at := strings.Index(body, "data-verso-staged-row")
	if at < 0 || !strings.Contains(body[max(0, at-600):at], "hostname") {
		t.Errorf("the mark stands on the hostname row")
	}
	if reads != 1 {
		t.Errorf("the stage was read %d times for one page, want once", reads)
	}
}

// TestAStagedSaveIsSaidByTheChip: a save that staged something has not
// happened yet, so nothing on the page says it did — no green band. The chip
// in the top bar is what says a change is waiting, and the page (verso-
// commit.js) shows where it went. A plugin's warning about the change is not a
// routine outcome, and still speaks in its own tone.
func TestAStagedSaveIsSaidByTheChip(t *testing.T) {
	calls := []uciWrite{}
	tr := &fakeTransport{env: &plugin.Envelope{
		SchemaVersion: 1, Title: "Rules", Status: http.StatusOK,
		Notice: &plugin.Notice{Level: "success", Text: "Rule saved."},
		Widget: json.RawMessage(`{"type":"text","markdown":"body"}`),
		Commit: []plugin.CommitOp{{Config: "system", Section: "r1", Values: map[string]any{"src": "wan"}}},
	}}
	s := newServerWith(t, fakeBackend{access: true, writes: &calls}, tr, []plugin.Manifest{demoACLManifest()})

	for _, notice := range []*plugin.Notice{{Level: "success", Text: "Rule saved."}, {Level: "info", Text: "Rule saved."}, nil} {
		tr.env.Notice = notice
		rec, token := postPluginRequest(t, s, "/plugins/demo/", url.Values{"src": {"wan"}}, nil)
		if variant, message := s.sessions.TakeFlash(token); message != "" {
			t.Errorf("notice %+v: a staged save must not be said on the page, flashed %q %q", notice, variant, message)
		}
		if strings.Contains(rec.Body.String(), `<div class="verso-flash`) {
			t.Errorf("notice %+v: a staged save must not be said on the page", notice)
		}
	}
	tr.env.Notice = &plugin.Notice{Level: "warning", Text: "Applying this closes the SSH port."}
	_, token := postPluginRequest(t, s, "/plugins/demo/", url.Values{"src": {"wan"}}, nil)
	if variant, message := s.sessions.TakeFlash(token); variant != "warning" || message != "Applying this closes the SSH port." {
		t.Errorf("a plugin's warning about the change still speaks in its tone on the page it lands on, got %q %q", variant, message)
	}
}

// TestAStagedSaveComesBackAsAPlainRead: a save that staged answers with the
// page read again, not with a page drawn as the answer to a post — reloading
// that would post the save again, and a change just discarded from the review
// drawer would be staged once more behind the person's back. A refused save
// is drawn in place, with what was typed.
func TestAStagedSaveComesBackAsAPlainRead(t *testing.T) {
	calls := []uciWrite{}
	tr := &fakeTransport{env: &plugin.Envelope{
		SchemaVersion: 1, Title: "Rules", Status: http.StatusOK,
		Widget: json.RawMessage(`{"type":"text","markdown":"body"}`),
		Commit: []plugin.CommitOp{{Config: "system", Section: "r1", Values: map[string]any{"src": "wan"}}},
	}}
	s := newServerWith(t, fakeBackend{access: true, writes: &calls}, tr, []plugin.Manifest{demoACLManifest()})
	rec := postPlugin(t, s, "/plugins/demo/", url.Values{"src": {"wan"}})
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/plugins/demo/" {
		t.Fatalf("a staged save answers %d → %q, want 303 back to the page", rec.Code, rec.Header().Get("Location"))
	}
	if len(calls) != 1 {
		t.Errorf("the save was staged %d times, want once", len(calls))
	}
	tr.env.Status, tr.env.Commit = http.StatusUnprocessableEntity, nil
	if rec := postPlugin(t, s, "/plugins/demo/", url.Values{"src": {"bad"}}); rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("a refused save is drawn in place, got %d", rec.Code)
	}
}

// TestPluginPanelRefusalSwapsInAt422: a refused save answers with the panel
// re-rendered from what was submitted, the refusal on the controls, and the
// plugin's 422 — so the frame swaps the refusal in rather than showing nothing.
// Nothing is staged, whatever the plugin returned beside its refusal.
func TestPluginPanelRefusalSwapsInAt422(t *testing.T) {
	calls := []uciWrite{}
	env := openPanelEnvelope(http.StatusUnprocessableEntity,
		&plugin.Notice{Level: "danger", Text: "Some values aren’t ones the firewall accepts."},
		`{"type":"field","name":"src","label":"From","kind":"text","value":"nowhere","error":"Not a zone."}`)
	env.Commit = []plugin.CommitOp{{Config: "system", Section: "r1", Values: map[string]any{"src": "nowhere"}}}
	s := newServerWith(t, fakeBackend{access: true, writes: &calls}, &fakeTransport{env: env}, []plugin.Manifest{demoACLManifest()})

	rec, _ := postPluginFromPanel(t, s, "/plugins/demo/?open=r1", url.Values{"src": {"nowhere"}})
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422 propagated from the plugin", rec.Code)
	}
	if len(calls) != 0 {
		t.Fatalf("a refused save staged %d write(s); it must stage none", len(calls))
	}
	body := rec.Body.String()
	for _, want := range []string{`value="nowhere"`, "Not a zone.", "Some values aren’t ones the firewall accepts.", "border-crimson-line"} {
		if !strings.Contains(body, want) {
			t.Errorf("refusal missing %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "<html") {
		t.Errorf("a refusal is the panel, not the page:\n%s", body)
	}
}

// TestPluginPanelSubmissionThePanelDoesNotSurviveGoesToThePage: a submission the
// panel does not survive — a delete — is answered with the listing and no panel.
// The frame cannot hold a page, so it is told where the page went, and the
// outcome waits there as the flash — what a native submit would have landed on.
func TestPluginPanelSubmissionThePanelDoesNotSurviveGoesToThePage(t *testing.T) {
	deletes := []string{}
	tr := &fakeTransport{env: &plugin.Envelope{
		SchemaVersion: 1, Title: "Rules", Status: http.StatusOK,
		Notice: &plugin.Notice{Level: "success", Text: "Rule deleted."},
		Widget: json.RawMessage(`{"type":"table","columns":[{"label":"Name","kind":"name"}],"rows":[
			{"id":"r2","cells":[{"text":"Allow-DHCP"}],"panel":"/plugins/demo/?open=r2"}]}`),
		Commit: []plugin.CommitOp{{Config: "system", Section: "r1", Delete: true}},
	}}
	s := newServerWith(t, fakeBackend{access: true, deletes: &deletes}, tr, []plugin.Manifest{demoACLManifest()})

	rec, token := postPluginFromPanel(t, s, "/plugins/demo/?open=r1", url.Values{"_delete": {"1"}})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200:\n%s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("HX-Redirect"); got != "/plugins/demo/" {
		t.Errorf("HX-Redirect = %q, want the listing's own address", got)
	}
	if rec.Body.Len() != 0 {
		t.Errorf("a redirected frame is sent no body to swap:\n%s", rec.Body.String())
	}
	// The delete was staged all the same.
	if len(deletes) != 1 || deletes[0] != "system.r1" {
		t.Errorf("staged deletes = %v, want the one on r1", deletes)
	}
	// Nothing waits on the page the frame is sent to: the chip says the
	// staged delete waits.
	if variant, message := s.sessions.TakeFlash(token); message != "" {
		t.Errorf("flash = %q %q, want none: the chip says a staged change waits", variant, message)
	}
}

// TestPluginPanelSubmissionThatFailsToStageKeepsItsStatus: a stage that fails is
// a contained notice at its own status even from a panel. The frame is not sent
// away from the values just typed — it is told what happened, where the person
// is, and the notice reaches the shell's script as the error it is.
func TestPluginPanelSubmissionThatFailsToStageKeepsItsStatus(t *testing.T) {
	env := openPanelEnvelope(http.StatusOK, &plugin.Notice{Level: "success", Text: "Rule saved."},
		`{"type":"field","name":"src","label":"From","kind":"text","value":"wan"}`)
	env.Commit = []plugin.CommitOp{{Config: "system", Section: "r1", Values: map[string]any{"src": "wan"}}}
	s := newServerWith(t, fakeBackend{access: true, uciErr: errors.New("rpcd down")}, &fakeTransport{env: env}, []plugin.Manifest{demoACLManifest()})

	rec, _ := postPluginFromPanel(t, s, "/plugins/demo/?open=r1", url.Values{"src": {"wan"}})
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503: nothing was staged, so nothing was saved", rec.Code)
	}
	if rec.Header().Get("HX-Redirect") != "" {
		t.Error("a failed stage must not send the frame away from what was typed")
	}
	if !strings.Contains(rec.Body.String(), "Save failed") {
		t.Errorf("the answer should say the save failed:\n%s", rec.Body.String())
	}
}

// TestPluginPreviewWithoutOneAnswersEmpty: a page that declares no live preview
// has nothing to answer a preview with. It leaves as an empty 204 rather than as
// the page, because falling through would stage a submission nobody made.
func TestPluginPreviewWithoutOneAnswersEmpty(t *testing.T) {
	calls := []uciWrite{}
	tr := &fakeTransport{env: &plugin.Envelope{
		SchemaVersion: 1, Status: http.StatusOK,
		Widget: json.RawMessage(`{"type":"card","children":[{"type":"code","label":"Key","value":"abc","copy":true}]}`),
		Commit: []plugin.CommitOp{{Config: "system", Section: "@system[0]", Values: map[string]any{"hostname": "x"}}},
	}}
	s := newServerWith(t, fakeBackend{access: true, writes: &calls}, tr, []plugin.Manifest{demoACLManifest()})

	rec := postPluginAs(t, s, "/plugins/demo/", url.Values{"hostname": {"x"}}, "preview")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
	if len(calls) != 0 {
		t.Fatalf("a preview staged %d write(s); it must stage none", len(calls))
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
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303: a staged save is read again", rec.Code)
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
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303: a staged save is read again", rec.Code)
	}
	if len(calls) != 1 {
		t.Fatalf("UCISet calls = %d, want 1", len(calls))
	}
	arr, ok := calls[0].values["server"].([]any)
	if !ok || len(arr) != 2 || arr[0] != "a.pool" {
		t.Errorf("list option not carried through the broker: %#v", calls[0].values["server"])
	}
}

// TestPluginCommitDeletesSection: a commit op carrying delete removes the whole
// section through the backend and writes nothing else — the editor's Delete.
func TestPluginCommitDeletesSection(t *testing.T) {
	calls := []uciWrite{}
	dels := []string{}
	tr := &fakeTransport{env: &plugin.Envelope{
		SchemaVersion: 1, Status: http.StatusOK,
		Widget: json.RawMessage(`{"type":"card","children":[]}`),
		Commit: []plugin.CommitOp{{Config: "system", Section: "cfg07led", Delete: true}},
	}}
	s := newServerWith(t, fakeBackend{access: true, writes: &calls, deletes: &dels}, tr, []plugin.Manifest{demoACLManifest()})

	rec := postPlugin(t, s, "/plugins/demo/", url.Values{"_delete": {"1"}})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303: a staged save is read again", rec.Code)
	}
	if len(dels) != 1 || dels[0] != "system.cfg07led" {
		t.Fatalf("UCIDelete calls = %v, want one \"system.cfg07led\"", dels)
	}
	if len(calls) != 0 {
		t.Errorf("a delete must not also write values; got %d UCISet calls", len(calls))
	}
}

// TestPluginCommitDeleteMalformedRefused: a delete describes one operation. An op
// that also names a type, values, or no section at all is refused outright rather
// than guessed at, and nothing reaches the backend.
func TestPluginCommitDeleteMalformedRefused(t *testing.T) {
	for name, op := range map[string]plugin.CommitOp{
		"no section": {Config: "system", Section: "", Delete: true},
		"with type":  {Config: "system", Section: "cfg07led", Type: "led", Delete: true},
		"with values": {Config: "system", Section: "cfg07led", Delete: true,
			Values: map[string]any{"name": "disk"}},
	} {
		t.Run(name, func(t *testing.T) {
			calls, dels, adds := []uciWrite{}, []string{}, []string{}
			tr := &fakeTransport{env: &plugin.Envelope{
				SchemaVersion: 1, Status: http.StatusOK,
				Widget: json.RawMessage(`{"type":"card","children":[]}`),
				Commit: []plugin.CommitOp{op},
			}}
			be := fakeBackend{access: true, writes: &calls, deletes: &dels, adds: &adds}
			s := newServerWith(t, be, tr, []plugin.Manifest{demoACLManifest()})

			rec := postPlugin(t, s, "/plugins/demo/", url.Values{"x": {"1"}})
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400 for a malformed delete", rec.Code)
			}
			if len(dels)+len(calls)+len(adds) != 0 {
				t.Errorf("a malformed delete must reach the backend not at all; got %v %v %v", dels, calls, adds)
			}
		})
	}
}

// TestPluginCommitNullClearsOption: a null among a commit's values clears that
// one option (an option-level delete) while the rest of the op is written — how
// an editor drops a setting it owns instead of writing it empty.
func TestPluginCommitNullClearsOption(t *testing.T) {
	calls := []uciWrite{}
	dels := []string{}
	tr := &fakeTransport{env: &plugin.Envelope{
		SchemaVersion: 1, Status: http.StatusOK,
		Widget: json.RawMessage(`{"type":"card","children":[]}`),
		Commit: []plugin.CommitOp{{Config: "system", Section: "ntp", Values: map[string]any{
			"enabled": "1", "server": nil, "interface": nil,
		}}},
	}}
	s := newServerWith(t, fakeBackend{access: true, writes: &calls, deletes: &dels}, tr, []plugin.Manifest{demoACLManifest()})

	rec := postPlugin(t, s, "/plugins/demo/", url.Values{"enabled": {"1"}})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303: a staged save is read again", rec.Code)
	}
	if len(dels) != 2 || dels[0] != "system.ntp.interface" || dels[1] != "system.ntp.server" {
		t.Fatalf("cleared options = %v, want interface then server (sorted)", dels)
	}
	if len(calls) != 1 || len(calls[0].values) != 1 || calls[0].values["enabled"] != "1" {
		t.Fatalf("brokered write = %+v; want only the non-null value", calls)
	}
}

// TestPluginCommitAllNullsWritesNothing: an op whose every value is a null is
// only clears — it must not follow them with an empty uci set.
func TestPluginCommitAllNullsWritesNothing(t *testing.T) {
	calls := []uciWrite{}
	dels := []string{}
	tr := &fakeTransport{env: &plugin.Envelope{
		SchemaVersion: 1, Status: http.StatusOK,
		Widget: json.RawMessage(`{"type":"card","children":[]}`),
		Commit: []plugin.CommitOp{{Config: "system", Section: "ntp", Values: map[string]any{"server": nil}}},
	}}
	s := newServerWith(t, fakeBackend{access: true, writes: &calls, deletes: &dels}, tr, []plugin.Manifest{demoACLManifest()})

	if rec := postPlugin(t, s, "/plugins/demo/", url.Values{"x": {"1"}}); rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303: a staged save is read again", rec.Code)
	}
	if len(dels) != 1 || len(calls) != 0 {
		t.Errorf("clears = %v, writes = %v; want one clear and no write", dels, calls)
	}
}

// TestPluginCommitClearOfAnAbsentOptionStagesOn: an editor that owns a set of
// options states all of them on every save, and the ones a stock section never
// carried come through as nulls. rpcd answers `uci delete` for an option that is
// not set with NOT_FOUND — already the state the clear asked for — so the stage
// carries on and the rest of the save is written.
func TestPluginCommitClearOfAnAbsentOptionStagesOn(t *testing.T) {
	calls := []uciWrite{}
	dels := []string{}
	tr := &fakeTransport{env: &plugin.Envelope{
		SchemaVersion: 1, Status: http.StatusOK,
		Widget: json.RawMessage(`{"type":"card","children":[]}`),
		Commit: []plugin.CommitOp{{Config: "system", Section: "ntp", Values: map[string]any{
			"enabled": "1", "server": nil, "interface": nil,
		}}},
	}}
	be := fakeBackend{access: true, writes: &calls, deletes: &dels,
		deleteErr: func(_, _, option string) error {
			if option == "interface" {
				return fmt.Errorf("openwrt: clear system.ntp.interface: %w", openwrt.ErrOptionNotFound)
			}
			return nil
		}}
	s := newServerWith(t, be, tr, []plugin.Manifest{demoACLManifest()})

	rec := postPlugin(t, s, "/plugins/demo/", url.Values{"enabled": {"1"}})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303: an option that was never set is already clear", rec.Code)
	}
	if len(dels) != 2 {
		t.Fatalf("clears = %v, want both attempted", dels)
	}
	if len(calls) != 1 || calls[0].values["enabled"] != "1" {
		t.Fatalf("brokered write = %+v; want the save to have gone through", calls)
	}
}

// TestPluginCommitAllClearsAbsentChecksTheSectionIsThere: an op whose every
// option came back "already absent" wrote nothing at all — so nothing in it has
// yet proved the section it addressed exists. A stale id (a section deleted in
// another tab, a plugin holding an old snapshot) would otherwise stage nothing
// and be reported as saved. The backstop is one read of that config.
func TestPluginCommitAllClearsAbsentChecksTheSectionIsThere(t *testing.T) {
	commit := []plugin.CommitOp{{Config: "system", Section: "ntp", Values: map[string]any{
		"server": nil, "interface": nil,
	}}}
	absent := func(_, _, option string) error {
		return fmt.Errorf("openwrt: clear system.ntp.%s: %w", option, openwrt.ErrOptionNotFound)
	}

	t.Run("the section is gone", func(t *testing.T) {
		dels, reads := []string{}, 0
		tr := &fakeTransport{env: &plugin.Envelope{
			SchemaVersion: 1, Status: http.StatusOK,
			Widget: json.RawMessage(`{"type":"card","children":[]}`), Commit: commit,
		}}
		be := fakeBackend{access: true, deletes: &dels, deleteErr: absent, uciReads: &reads,
			uci: map[string]map[string]any{"system": {"cfg07led": map[string]any{".type": "led"}}}}
		s := newServerWith(t, be, tr, []plugin.Manifest{demoACLManifest()})

		rec := postPlugin(t, s, "/plugins/demo/", url.Values{"x": {"1"}})
		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("status = %d, want 503: nothing was staged, so nothing was saved", rec.Code)
		}
		if reads != 1 {
			t.Errorf("config reads = %d, want exactly one backstop read", reads)
		}
	})

	t.Run("the section is there", func(t *testing.T) {
		dels, reads := []string{}, 0
		tr := &fakeTransport{env: &plugin.Envelope{
			SchemaVersion: 1, Status: http.StatusOK,
			Widget: json.RawMessage(`{"type":"card","children":[]}`), Commit: commit,
		}}
		be := fakeBackend{access: true, deletes: &dels, deleteErr: absent, uciReads: &reads,
			uci: map[string]map[string]any{"system": {"ntp": map[string]any{".type": "timeserver"}}}}
		s := newServerWith(t, be, tr, []plugin.Manifest{demoACLManifest()})

		if rec := postPlugin(t, s, "/plugins/demo/", url.Values{"x": {"1"}}); rec.Code != http.StatusSeeOther {
			t.Fatalf("status = %d, want 303: a section that carries none of those options is already as asked", rec.Code)
		}
		if len(dels) != 2 {
			t.Errorf("clears = %v, want both attempted", dels)
		}
	})

	t.Run("a config that cannot be read is not a success", func(t *testing.T) {
		tr := &fakeTransport{env: &plugin.Envelope{
			SchemaVersion: 1, Status: http.StatusOK,
			Widget: json.RawMessage(`{"type":"card","children":[]}`), Commit: commit,
		}}
		be := fakeBackend{access: true, deletes: &[]string{}, deleteErr: absent,
			uciReadErr: errors.New("rpcd denied")}
		s := newServerWith(t, be, tr, []plugin.Manifest{demoACLManifest()})

		if rec := postPlugin(t, s, "/plugins/demo/", url.Values{"x": {"1"}}); rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("status = %d, want 503: a section nobody could confirm is not one that was saved", rec.Code)
		}
	})
}

// TestPluginCommitWithARealWriteNeverReadsBack: the backstop above costs a read
// only where a whole op touched nothing. A save that set even one value has
// already proved the section is there, and pays nothing.
func TestPluginCommitWithARealWriteNeverReadsBack(t *testing.T) {
	calls, dels, reads := []uciWrite{}, []string{}, 0
	tr := &fakeTransport{env: &plugin.Envelope{
		SchemaVersion: 1, Status: http.StatusOK,
		Widget: json.RawMessage(`{"type":"card","children":[]}`),
		Commit: []plugin.CommitOp{{Config: "system", Section: "ntp", Values: map[string]any{
			"enabled": "1", "server": nil,
		}}},
	}}
	be := fakeBackend{access: true, writes: &calls, deletes: &dels, uciReads: &reads,
		deleteErr: func(_, _, option string) error {
			return fmt.Errorf("openwrt: clear system.ntp.%s: %w", option, openwrt.ErrOptionNotFound)
		}}
	s := newServerWith(t, be, tr, []plugin.Manifest{demoACLManifest()})

	if rec := postPlugin(t, s, "/plugins/demo/", url.Values{"enabled": {"1"}}); rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303: a staged save is read again", rec.Code)
	}
	if reads != 0 {
		t.Errorf("config reads = %d, want none: the write itself is the proof", reads)
	}
}

// TestPluginCommitSectionDeleteNotFoundStillFails: the tolerance above is the
// option-level clear's alone. A section the backend cannot act on is a genuine
// failure, and the stage stops on it.
func TestPluginCommitSectionDeleteNotFoundStillFails(t *testing.T) {
	dels := []string{}
	tr := &fakeTransport{env: &plugin.Envelope{
		SchemaVersion: 1, Status: http.StatusOK,
		Widget: json.RawMessage(`{"type":"card","children":[]}`),
		Commit: []plugin.CommitOp{{Config: "system", Section: "cfg07led", Delete: true}},
	}}
	be := fakeBackend{access: true, deletes: &dels,
		deleteErr: func(_, _, _ string) error {
			return fmt.Errorf("openwrt: clear system.cfg07led: %w", openwrt.ErrOptionNotFound)
		}}
	s := newServerWith(t, be, tr, []plugin.Manifest{demoACLManifest()})

	if rec := postPlugin(t, s, "/plugins/demo/", url.Values{"x": {"1"}}); rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503: a failed section delete is never tolerated", rec.Code)
	}
}

// TestPluginCommitDeleteRefusedForUndeclaredConfig: the declared-surface bound
// covers deletes exactly as it covers writes.
func TestPluginCommitDeleteRefusedForUndeclaredConfig(t *testing.T) {
	dels := []string{}
	tr := &fakeTransport{env: &plugin.Envelope{
		SchemaVersion: 1, Status: http.StatusOK,
		Widget: json.RawMessage(`{"type":"card","children":[]}`),
		Commit: []plugin.CommitOp{{Config: "network", Section: "cfg01", Delete: true}},
	}}
	// demoACLManifest declares only uci/system.
	s := newServerWith(t, fakeBackend{access: true, deletes: &dels}, tr, []plugin.Manifest{demoACLManifest()})

	rec := postPlugin(t, s, "/plugins/demo/", url.Values{"x": {"1"}})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 for a delete outside the declared scope", rec.Code)
	}
	if len(dels) != 0 {
		t.Errorf("an undeclared delete must NOT be executed; got %v", dels)
	}
}

// TestPluginCommitDeleteBackendErrorContained: rpcd refusing the delete is a
// contained failure, never a false success.
func TestPluginCommitDeleteBackendErrorContained(t *testing.T) {
	dels := []string{}
	tr := &fakeTransport{env: &plugin.Envelope{
		SchemaVersion: 1, Status: http.StatusOK,
		Widget: json.RawMessage(`{"type":"card","children":[]}`),
		Commit: []plugin.CommitOp{{Config: "system", Section: "cfg07led", Delete: true}},
	}}
	be := fakeBackend{access: true, deletes: &dels, uciErr: errors.New("rpcd denied")}
	s := newServerWith(t, be, tr, []plugin.Manifest{demoACLManifest()})

	if rec := postPlugin(t, s, "/plugins/demo/", url.Values{"x": {"1"}}); rec.Code == http.StatusOK {
		t.Fatalf("a failed delete must not report 200")
	}
	if len(dels) != 1 {
		t.Errorf("UCIDelete should have been attempted once; got %v", dels)
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
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303 for a valid submission: a staged save is read again", rec.Code)
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

// TestAnEmptyOptionalFieldIsUnsetNotMalformed: a datatype says what a value
// must look like, not that there must be one. An optional field left empty is
// an option left unset — a schedule nobody gave a port forward — and blocking
// the write over it refuses every save of a form that offers the option. A
// required field left empty is still refused.
func TestAnEmptyOptionalFieldIsUnsetNotMalformed(t *testing.T) {
	optional := &widget.Field{Name: "start_time", Datatype: "timehhmmss"}
	if validateSchema(&widget.Form{Fields: []widget.Widget{optional}}) || optional.Error != "" {
		t.Errorf("an empty optional field blocked the write: %q", optional.Error)
	}
	required := &widget.Field{Name: "hostname", Datatype: "hostname", Required: true}
	if !validateSchema(&widget.Form{Fields: []widget.Widget{required}}) {
		t.Error("an empty required field must still be refused")
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

// TestPluginNoticeRendersInFlashSlot: the envelope's notice is the outcome
// channel (ADR-006 §4) — the shell shows it in its own flash slot, toned by
// its level, so a plugin's outcome and the shell's are indistinguishable.
func TestPluginNoticeRendersInFlashSlot(t *testing.T) {
	tr := &fakeTransport{env: &plugin.Envelope{
		SchemaVersion: 1, Status: http.StatusOK,
		Notice: &plugin.Notice{Level: "warning", Text: "Takes effect on the next reload."},
		Widget: json.RawMessage(`{"type":"text","markdown":"body"}`),
	}}
	s := newServerWith(t, fakeBackend{}, tr, []plugin.Manifest{demoManifest()})

	body := get(t, s, "/plugins/demo/").Body.String()
	for _, want := range []string{`<div class="verso-flash`, "Takes effect on the next reload.", "border-marigold-line"} {
		if !strings.Contains(body, want) {
			t.Errorf("notice flash missing %q", want)
		}
	}
}

// TestEveryMastheadStandsOnAHairline: every page's title stands in a sand bar,
// the colophon's ground, alone on its line — no lede under it — with 16px
// above and below the line, so an act on it sits in the bar's middle; the bar
// ends on a hairline, with 24px before what follows. A page that opens on a
// control band runs the bar on into the band: the band's own top edge is the
// seam, at the same 16px.
func TestEveryMastheadStandsOnAHairline(t *testing.T) {
	tr := &fakeTransport{env: &plugin.Envelope{
		SchemaVersion: 1, Status: http.StatusOK,
		Title:  "Firewall settings",
		Widget: json.RawMessage(`{"type":"text","markdown":"body"}`),
	}}
	s := newServerWith(t, fakeBackend{}, tr, []plugin.Manifest{demoManifest()})
	body := get(t, s, "/plugins/demo/").Body.String()
	if !strings.Contains(body, `<div class="px-10 pb-10">`) {
		t.Errorf("the bar meets the top bar, with no air of the frame's above it:\n%s", body)
	}
	if !strings.Contains(body, `<div data-verso-masthead class="mb-6 py-4">`) {
		t.Errorf("the title's line stands 16px inside the bar at both edges, and the hairline 24px over the page:\n%s", body)
	}
	if strings.Contains(body, `class="verso-lede`) {
		t.Error("a page's title carries no lede")
	}
	if strings.Contains(body, `<div class="mb-5">`) {
		t.Error("no title keeps the 20px standoff")
	}
	css, err := os.ReadFile("assets/input.css")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"main [data-verso-masthead] {\n    margin-inline: calc(var(--spacing) * -10) calc(100% - 100cqw + var(--spacing) * 10);\n    padding-inline: calc(var(--spacing) * 10) calc(100cqw - var(--spacing) * 10 - min(100cqw - var(--spacing) * 20, var(--container-6xl)));\n    border-bottom: 1px solid var(--color-rule);\n    background-color: var(--color-quiet);",
		"main [data-verso-masthead=\"light\"] {\n    background-color: var(--color-ground);",
	} {
		if !strings.Contains(string(css), want) {
			t.Errorf("stylesheet is missing %s", want)
		}
	}
	// the masthead keeps its hairline over a control band: the band is a box
	// in the page, not the bar's continuation
	if strings.Contains(string(css), "main:has([data-verso-actionbar]) [data-verso-masthead]") {
		t.Error("the masthead runs on into the control band")
	}
}

// TestALiveLogsMastheadIsLight: a live log's heading line holds what narrows
// and holds the stream, and its bar is the light one, on the page's own
// ground, so the log under it is the page's one darker surface.
func TestALiveLogsMastheadIsLight(t *testing.T) {
	tr := &fakeTransport{env: &plugin.Envelope{
		SchemaVersion: 1, Status: http.StatusOK, Title: "Activity",
		Widget: json.RawMessage(`{"type":"stack","children":[
			{"type":"actionbar","filter":"Find an address","live":"Live"},
			{"type":"table","style":"console","stream":{"source":"firewall-log"},"columns":[{"label":"Verdict"}]}]}`),
	}}
	s := newServerWith(t, fakeBackend{}, tr, []plugin.Manifest{demoManifest()})
	body := get(t, s, "/plugins/demo/").Body.String()
	if !strings.Contains(body, `<div data-verso-masthead="light" class="mb-6 py-4">`) {
		t.Errorf("a live log's masthead is not the light one:\n%s", body)
	}
	plain := get(t, newServerWith(t, fakeBackend{}, &fakeTransport{env: &plugin.Envelope{
		SchemaVersion: 1, Status: http.StatusOK, Title: "Settings",
		Widget: json.RawMessage(`{"type":"text","markdown":"body"}`),
	}}, []plugin.Manifest{demoManifest()}), "/plugins/demo/").Body.String()
	if strings.Contains(plain, `data-verso-masthead="light"`) {
		t.Error("a page without a live log keeps the sand bar")
	}
	// the log sits flush on the masthead's hairline: one line, no air above it
	css, err := os.ReadFile("assets/input.css")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(css), "main:has([data-verso-actionbar=\"log\"]) [data-verso-masthead] {\n    margin-bottom: 0;") {
		t.Error("a live log's masthead keeps air between its hairline and the log")
	}
}

// TestReturnAddressDrawsNoMastheadLink: an editor's return address is where its
// save and its form's Cancel lead, never a back-link over the title.
func TestReturnAddressDrawsNoMastheadLink(t *testing.T) {
	tr := &fakeTransport{env: &plugin.Envelope{
		SchemaVersion: 1, Status: http.StatusOK, Title: "Interfaces",
		Back:   &plugin.PageAction{Label: "Interfaces", Href: "/plugins/demo/"},
		Widget: json.RawMessage(`{"type":"form","style":"page","fields":[{"type":"field","name":"h","label":"Hostname"}]}`),
	}}
	s := newServerWith(t, fakeBackend{}, tr, []plugin.Manifest{demoManifest()})
	body := get(t, s, "/plugins/demo/").Body.String()
	start := strings.Index(body, "<div data-verso-masthead")
	end := strings.Index(body, "</h1>")
	if start < 0 || end < start {
		t.Fatalf("no masthead:\n%s", body)
	}
	if masthead := body[start:end]; strings.Contains(masthead, "<a ") {
		t.Errorf("the masthead draws a back-link:\n%s", masthead)
	}
	if !strings.Contains(body, `href="/plugins/demo/"`) {
		t.Error("the page form's Cancel still leads to the return address")
	}
}

// TestPluginWithoutActionRendersNoButton: no action, no doorway.
func TestPluginWithoutActionRendersNoButton(t *testing.T) {
	tr := &fakeTransport{env: &plugin.Envelope{
		SchemaVersion: 1, Status: http.StatusOK, Title: "Firewall",
		Widget: json.RawMessage(`{"type":"text","markdown":"body"}`),
	}}
	s := newServerWith(t, fakeBackend{}, tr, []plugin.Manifest{demoManifest()})

	if body := get(t, s, "/plugins/demo/").Body.String(); strings.Contains(body, "rules/new") {
		t.Error("a plugin page declaring no action must render no heading button")
	}
}

// TestPluginWithoutNoticeShowsNoFlash: no notice, no flash strip.
func TestPluginWithoutNoticeShowsNoFlash(t *testing.T) {
	tr := &fakeTransport{env: &plugin.Envelope{
		SchemaVersion: 1, Status: http.StatusOK,
		Widget: json.RawMessage(`{"type":"text","markdown":"body"}`),
	}}
	s := newServerWith(t, fakeBackend{}, tr, []plugin.Manifest{demoManifest()})

	if body := get(t, s, "/plugins/demo/").Body.String(); strings.Contains(body, `<div class="verso-flash`) {
		t.Error("a plugin page without a notice must carry no flash strip")
	}
}

// TestValidateSchemaCoversNestedContainers: the datatype gate reaches a field
// no matter which container it nests in — a modal, a conditions item, a
// drawer, or a table row's drawer. A container outside the walk would let an
// invalid value through to the commit.
func TestValidateSchemaCoversNestedContainers(t *testing.T) {
	bad := func() *widget.Field { return &widget.Field{Name: "p", Datatype: "port", Value: "nope"} }
	cases := []struct {
		name string
		tree widget.Widget
	}{
		{"modal", &widget.Modal{Children: []widget.Widget{bad()}}},
		{"conditions", &widget.Conditions{Items: []widget.ConditionItem{{Key: "k", Children: []widget.Widget{bad()}}}}},
		{"table row drawer", &widget.Table{Rows: []widget.TableRow{{Drawer: &widget.RowDrawer{Children: []widget.Widget{bad()}}}}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if !validateSchema(tc.tree) {
				t.Fatalf("an invalid field inside a %s must block the write", tc.name)
			}
		})
	}
}

// TestNavListsPlugins: a discovered plugin appears in the shell nav with no shell
// change — the manifest drives it (ADR-006 §2).
func TestNavListsPlugins(t *testing.T) {
	s := newServerWith(t, fakeBackend{}, &fakeTransport{}, []plugin.Manifest{demoManifest()})

	rec := get(t, s, "/")
	body := rec.Body.String()
	for _, want := range []string{"Demo", `href="/plugins/demo/"`} {
		if !strings.Contains(body, want) {
			t.Errorf("nav missing %q", want)
		}
	}
	// The rail names the destination, never the section a manifest filed it under.
	if strings.Contains(body, ">Apps<") {
		t.Error("the rail should not print a section title")
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
	token := srv.sessions.CreateWithMetadata("sid", "root", "", "")

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

// TestExpiredSessionRedirectsAnnotated: a request presenting a session cookie
// that no longer names a live session lands on the login page carrying the
// inactivity marker — POST as much as GET, since the work is lost either way
// once the session is.
func TestExpiredSessionRedirectsAnnotated(t *testing.T) {
	cases := []struct {
		name   string
		method string
		cookie func(srv *Server, clk *fakeClock) string
	}{
		{"expired GET", http.MethodGet, func(srv *Server, clk *fakeClock) string {
			token := srv.sessions.CreateWithMetadata("sid", "root", "", "")
			clk.advance(sessionIdleTimeout + time.Minute)
			return token
		}},
		{"expired POST", http.MethodPost, func(srv *Server, clk *fakeClock) string {
			token := srv.sessions.CreateWithMetadata("sid", "root", "", "")
			clk.advance(sessionIdleTimeout + time.Minute)
			return token
		}},
		{"unknown token", http.MethodGet, func(*Server, *fakeClock) string {
			return "a-token-this-shell-never-issued"
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := newServer(t, fakeBackend{})
			clk := &fakeClock{t: time.Unix(1_000_000, 0)}
			srv.sessions = newSessionsClock(clk.now)

			req := httptest.NewRequest(tc.method, "/", nil)
			req.AddCookie(&http.Cookie{Name: sessionCookie, Value: tc.cookie(srv, clk)})
			rec := httptest.NewRecorder()
			srv.Handler().ServeHTTP(rec, req)

			if rec.Code != http.StatusSeeOther {
				t.Fatalf("status = %d, want 303", rec.Code)
			}
			if loc := rec.Header().Get("Location"); loc != "/login?expired=1" {
				t.Errorf("Location = %q, want /login?expired=1", loc)
			}
		})
	}
}

// TestLoginNoticeOnExpiredMarker: the marker turns into one line in the
// login page's marigold notice slot. Without the marker the slot stays empty, so a
// deliberate sign-out and a first visit say nothing.
func TestLoginNoticeOnExpiredMarker(t *testing.T) {
	const notice = "You were signed out after a period of inactivity."
	srv := newServer(t, fakeBackend{})

	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/login?expired=1", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, notice) {
		t.Errorf("expired login page is missing the notice")
	}
	_, status, _ := strings.Cut(body, `<div role="status"`)
	noticeMarkup, _, _ := strings.Cut(status, "</div>")
	if !strings.Contains(noticeMarkup, "bg-marigold-soft") || !strings.Contains(noticeMarkup, "text-marigold-deep") {
		t.Errorf("the notice should carry the marigold warning tone")
	}

	plain := httptest.NewRecorder()
	srv.Handler().ServeHTTP(plain, httptest.NewRequest(http.MethodGet, "/login", nil))
	if strings.Contains(plain.Body.String(), notice) {
		t.Errorf("a plain login page must not claim the visitor was signed out")
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
	// The preloaded names must be files the embedded fonts directory actually
	// holds — a stale name 404s silently and the FOUT fix quietly stops working.
	for _, font := range []string{"hanken-latin.woff2", "inconsolata-latin.woff2"} {
		if !strings.Contains(rec.Body.String(), `rel="preload" href="/assets/fonts/`+font+`"`) {
			t.Errorf("login page does not preload %s", font)
		}
		if _, err := scriptFS.ReadFile("assets/fonts/" + font); err != nil {
			t.Errorf("preloaded font %s is not in the embedded assets: %v", font, err)
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
	// A rejected attempt is explained beside the password, with an accessible
	// association and a cleared password. The submitted username stays editable.
	for _, want := range []string{
		`aria-invalid="true"`, `aria-describedby="login-error"`, `id="login-error" role="alert"`,
		`name="username" type="text" value="root"`,
	} {
		if !strings.Contains(rec.Body.String(), want) {
			t.Errorf("login error missing %q", want)
		}
	}
	if strings.Contains(rec.Body.String(), `value="bad"`) {
		t.Error("a rejected password must never be rendered back into the page")
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
	token := srv.sessions.CreateWithMetadata("sid", "root", "", "")
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
	// Signing out on purpose is not an expiry: the login page it lands on says
	// nothing about inactivity.
	if loc := rec.Header().Get("Location"); loc != "/login" {
		t.Errorf("Location = %q, want /login", loc)
	}
}

// TestSessionExpiryMeta: every rendered page stamps the moment its session ends,
// so a page left open can follow its own session out instead of waiting for the
// next click to discover it.
func TestSessionExpiryMeta(t *testing.T) {
	srv := newServer(t, fakeBackend{})
	clk := &fakeClock{t: time.Unix(1_000_000, 0)}
	srv.sessions = newSessionsClock(clk.now)
	token := srv.sessions.CreateWithMetadata("test-sid", "root", "", "")

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: token})
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	want := `<meta name="verso-session-expiry" content="` +
		clk.t.Add(sessionIdleTimeout).UTC().Format(time.RFC3339) + `">`
	if !strings.Contains(rec.Body.String(), want) {
		t.Errorf("page is missing %s", want)
	}
}

// TestCSRFRejectsPostWithoutToken: a state-changing request lacking the session
// CSRF token is refused (VS-04).
func TestCSRFRejectsPostWithoutToken(t *testing.T) {
	srv := newServer(t, fakeBackend{})
	token := srv.sessions.CreateWithMetadata("sid", "root", "", "")

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
		"border-crimson bg-crimson-soft",
		"text-crimson-deep",
		"text-crimson", // the glyph at full chroma — a mark, beside words at the step that carries them
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
	// Index within <main> only: the compiled stylesheet is inlined above it and
	// carries every class name the page could mention.
	main := body[strings.Index(body, "<main "):]
	warnAt := strings.Index(main, "No administrator password is set.")
	bodyAt := strings.Index(main, `class="verso-page-body"`)
	if warnAt < 0 || bodyAt < 0 || warnAt > bodyAt {
		t.Errorf("the warning belongs at the head of the content area, above the page: warning=%d body=%d", warnAt, bodyAt)
	}
	// A band, arrived at rather than stated: a 28px line inset 12px, so a
	// sentence that wraps on a phone grows the band instead of spilling out.
	if !strings.Contains(body, "-mt-px flex items-start gap-2.5 border-y border-crimson-line bg-crimson-soft px-8 py-3 text-sm leading-7") {
		t.Error("the warning is a 52px band from its padding")
	}
	// its tone is the crimson square on the first line, not a glyph
	if !strings.Contains(body, `<span class="flex shrink-0 pt-1"><span aria-hidden="true" class="mt-1.75 size-1.5 shrink-0 rounded-[1px] bg-crimson"></span></span>`) {
		t.Error("the warning is marked with the crimson square")
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
	main = body[strings.Index(body, "<main "):]
	warnAt = strings.Index(main, "No administrator password is set.")
	bodyAt = strings.Index(main, `class="verso-page-body"`)
	if warnAt < 0 || bodyAt < 0 || warnAt > bodyAt || !strings.Contains(body, "border-crimson-line bg-crimson-soft") {
		t.Errorf("a declared danger banner takes the same seam: warning=%d body=%d", warnAt, bodyAt)
	}
}

// The two fake rings are independent, just like logd and the NFLOG collector.
func (f fakeBackend) FirewallLogRead(_ context.Context, _ string, generation string, after int64, limit int) (openwrt.FirewallLogBatch, error) {
	if f.firewallReads != nil {
		*f.firewallReads++
	}
	epoch := f.firewallGeneration
	if epoch == "" {
		epoch = "test"
	}
	batch := openwrt.FirewallLogBatch{Generation: epoch, Reset: generation != epoch, Available: f.firewallErr == nil}
	if batch.Reset {
		after = -1
	}
	for _, entry := range f.firewallEntries {
		if entry.ID > after {
			batch.Entries = append(batch.Entries, entry)
		}
	}
	if len(batch.Entries) > limit {
		batch.Entries = batch.Entries[len(batch.Entries)-limit:]
	}
	return batch, f.firewallErr
}
