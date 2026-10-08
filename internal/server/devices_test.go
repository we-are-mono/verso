// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/we-are-mono/verso/internal/i18n"
	"github.com/we-are-mono/verso/internal/plugin"
	"github.com/we-are-mono/verso/internal/sysstat"
	"github.com/we-are-mono/verso/internal/widget"
)

const testLeases = "1787825263 42:e6:ad:ff:b7:af 192.168.77.102 toms-iphone 01:42:e6:ad:ff:b7:af\n" +
	"1787825264 a2:8c:d7:4a:0e:57 192.168.77.120 * *\n" +
	"1787825265 06:11:22:33:44:55 192.168.77.130 old-printer *\n"

// testNeighbors: .102 is confirmed (reachable) and carries v6 entries on the
// same MAC, .120 has a lapsed entry (stale), .130 has none at all.
func testNeighbors() ([]sysstat.Neighbor, error) {
	return []sysstat.Neighbor{
		{Addr: "192.168.77.102", MAC: "42:e6:ad:ff:b7:af", Interface: "br-lan", State: 0x02},        // reachable
		{Addr: "fd42:7ea:aa00:0:1::66", MAC: "42:e6:ad:ff:b7:af", Interface: "br-lan", State: 0x04}, // its v6, stale
		{Addr: "fe80::44:11", MAC: "42:e6:ad:ff:b7:af", Interface: "br-lan", State: 0x04},           // its link-local
		{Addr: "192.168.77.120", MAC: "a2:8c:d7:4a:0e:57", Interface: "br-lan", State: 0x04},        // stale
		{Addr: "fe80::dead", MAC: "", Interface: "br-lan", State: 0x20},                             // failed, unresolved
	}, nil
}

// rosterBackend carries the config the roster reads: the lan interface's subnet,
// the firewall zone that covers it, and one DHCP reservation.
func rosterBackend() fakeBackend {
	return fakeBackend{uci: map[string]map[string]any{
		"network": {
			"lan": map[string]any{".type": "interface", ".name": "lan",
				"device": "br-lan", "ipaddr": "192.168.77.1", "netmask": "255.255.255.0"},
		},
		"firewall": {
			"z1": map[string]any{".type": "zone", "name": "lan", "network": []any{"lan"}},
		},
		"dhcp": {
			"printer": map[string]any{".type": "host", ".name": "printer",
				"name": "old-printer", "mac": "06:11:22:33:44:55", "ip": "192.168.77.130"},
		},
	}}
}

// dnsdhcpManifest registers the plugin that owns reservations, so the roster's
// reserve door has somewhere to lead.
func dnsdhcpManifest() plugin.Manifest {
	return plugin.Manifest{
		ID: "dnsdhcp", Name: "DNS and DHCP",
		Socket: "/run/verso/dnsdhcp.sock", SchemaVersion: 1,
		Nav: []plugin.NavEntry{{Section: "Network", Label: "DHCP", Path: "/"}},
	}
}

func rosterServer(t *testing.T) *Server {
	t.Helper()
	s := newServerWith(t, rosterBackend(), &fakeTransport{}, []plugin.Manifest{dnsdhcpManifest()})
	s.readLeases = func() ([]byte, error) { return []byte(testLeases), nil }
	s.neighbors = testNeighbors
	s.bridgePorts = func() (map[string]string, error) {
		return map[string]string{"42:e6:ad:ff:b7:af": "lan0"}, nil
	}
	return s
}

func roster(t *testing.T, s *Server) map[string]widget.Device {
	t.Helper()
	byName := map[string]widget.Device{}
	for _, device := range s.connectedDevices(context.Background(), "test-sid", "") {
		byName[device.Name] = device
	}
	return byName
}

func TestConnectedDevicesUseKernelInterfaceWithUCIFallback(t *testing.T) {
	byName := roster(t, rosterServer(t))
	if got := byName["toms-iphone"]; got.Interface != "br-lan" || got.Zone != "lan" {
		t.Errorf("kernel-backed device = %+v", got)
	}
	if got := byName["old-printer"]; got.Interface != "br-lan" || got.Zone != "lan" {
		t.Errorf("lease-only UCI fallback = %+v", got)
	}
}

// TestDevicesPageRendersTheRoster: /devices is the roster's own page — heading,
// the listing, and every row's drawer, from the same reads the overview used.
func TestDevicesPageRendersTheRoster(t *testing.T) {
	rec := get(t, rosterServer(t), "/devices")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /devices: status = %d, want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	for _, want := range []string{
		">Devices<",                        // the page heading
		"toms-iphone", "42:e6:ad:ff:b7:af", // a leaseholder, by name and MAC
		"holding a lease now",                 // the legend under the listing
		"192.168.77.102", "Online", "Offline", // its address and the presence words
		`data-verso-entity-url="/entity/device/42:e6:ad:ff:b7:af"`, // the row points at its panel
	} {
		if !strings.Contains(body, want) {
			t.Errorf("GET /devices: body missing %q", want)
		}
	}
	// The panel is fetched when it opens, so a listing of three devices ships no
	// panel bodies at all — that is what keeps a page of thirty from asking every
	// contributing plugin thirty times before anyone has clicked anything.
	if strings.Contains(body, "fd42:7ea:aa00:0:1::66") {
		t.Error("a device's panel facts must not ride along with the listing")
	}
	// The roster is read whole, banded by network: no bar cuts or narrows it.
	markup := body[strings.LastIndex(body, "</style>"):]
	for _, never := range []string{"data-verso-actionbar", "data-verso-listing-cut", "data-verso-listing-select", "data-verso-tags"} {
		if strings.Contains(markup, never) {
			t.Errorf("GET /devices: the roster carries a filter bar's %q", never)
		}
	}
}

// TestEntityPanelDetailsAreATabNotARecap: a device's machine facts are the
// panel's first tab, Details, holding what the roster does not show — the MAC
// once usage takes its column, every address, the lease. They are a tab of their
// own, never a recap above a plugin's form.
func TestEntityPanelDetailsAreATabNotARecap(t *testing.T) {
	rec := get(t, rosterServer(t), "/entity/device/42:e6:ad:ff:b7:af")
	if rec.Code != http.StatusOK {
		t.Fatalf("panel status = %d", rec.Code)
	}
	body := rec.Body.String()
	// A lone tab draws no strip; the facts are the panel's body.
	for _, want := range []string{"toms-iphone", ">MAC address<", "42:e6:ad:ff:b7:af", "fd42:7ea:aa00:0:1::66", ">Lease<"} {
		if !strings.Contains(body, want) {
			t.Errorf("the Details tab missing %q", want)
		}
	}
	if strings.Contains(body, "Device details") {
		t.Error("the facts are a tab, not a titled recap")
	}

	shape := getLang(t, shapingServer(t, twoTabs()), "/entity/device/42:e6:ad:ff:b7:af?tab=shape", "")
	if strings.Contains(shape, ">MAC address<") {
		t.Error("a plugin's tab carries its own form and no device recap")
	}
}

func TestEntityPanelLocalizesFormWithoutDeviceDetails(t *testing.T) {
	srv := shapingServer(t, twoTabs())
	bundle, problems := i18n.Load(os.DirFS("../../i18n"), "*/*.json")
	if len(problems) != 0 {
		t.Fatal(problems)
	}
	srv.SetBundle(bundle)
	body := getLang(t, srv, "/entity/device/42:e6:ad:ff:b7:af?tab=shape", "sl")
	// The shell's own tab is named from the shell's catalog, as each plugin's
	// is from its own.
	for _, want := range []string{`aria-label="Zapri"`, "toms-iphone", "bg-quiet px-10", ">Podrobnosti<"} {
		if !strings.Contains(body, want) {
			t.Errorf("localized device drawer missing %q", want)
		}
	}
	if !strings.Contains(body, "<form") || strings.Contains(body, "Podrobnosti naprave") {
		t.Error("the plugin form must remain, without the removed device details section")
	}
	header := strings.Split(body, "</header>")[0]
	if strings.Contains(header, "<dl") || strings.Contains(header, "verso-chip") {
		t.Error("the device heading must contain only its title and close button")
	}
}

// TestEntityPanelRefusesAnUnknownSubject: a MAC in a URL is a claim, so the panel
// reads the roster rather than trusting it.
func TestEntityPanelRefusesAnUnknownSubject(t *testing.T) {
	if rec := get(t, rosterServer(t), "/entity/device/00:00:00:00:00:00"); rec.Code != http.StatusNotFound {
		t.Errorf("a device the box cannot see = %d, want 404", rec.Code)
	}
	if rec := get(t, rosterServer(t), "/entity/nonsense/x"); rec.Code != http.StatusNotFound {
		t.Errorf("an entity kind the shell has no vocabulary for = %d, want 404", rec.Code)
	}
}

// TestDevicesMarksAReservedDevice: whether a device's address is pinned is a
// fact about the row; what the reservation says is dnsdhcp's tab in the panel.
func TestDevicesMarksAReservedDevice(t *testing.T) {
	byName := roster(t, rosterServer(t))
	if iphone := byName["toms-iphone"]; !iphone.Leased || iphone.Reserved {
		t.Fatalf("toms-iphone = %+v, want a lease and no reservation", iphone)
	}
	if printer := byName["old-printer"]; !printer.Reserved {
		t.Errorf("a MAC named by a dhcp host section should read as reserved: %+v", printer)
	}

	body := get(t, rosterServer(t), "/devices").Body.String()
	if n := strings.Count(body, ">reserved</span>"); n != 1 {
		t.Errorf("exactly the reserved device should wear the chip, got %d", n)
	}
}

func TestDevicesReservationActionsFollowConfiguredState(t *testing.T) {
	for name, macs := range map[string]any{
		"single MAC":           "06:11:22:33:44:55",
		"space-separated MACs": "06:11:22:33:44:55 AA:BB:CC:DD:EE:FF",
		"UCI list":             []any{"06:11:22:33:44:55", "AA:BB:CC:DD:EE:FF"},
	} {
		t.Run(name, func(t *testing.T) {
			backend := rosterBackend()
			backend.uci["dhcp"]["printer"].(map[string]any)["mac"] = macs
			m := reservingManifest()
			m.EntityActs = []plugin.EntityAct{{Entity: "device", Slot: "unreserve", Path: "/config/reservations/{id}/delete"}}
			s := newServerWith(t, backend, twoTabs(), []plugin.Manifest{m})
			s.readLeases = func() ([]byte, error) { return []byte(testLeases), nil }
			s.neighbors = testNeighbors

			body := get(t, s, "/devices").Body.String()
			for _, mac := range []string{"42:e6:ad:ff:b7:af", "a2:8c:d7:4a:0e:57", "06:11:22:33:44:55"} {
				marker := `data-verso-entity-url="/entity/device/` + mac + `"`
				at := strings.Index(body, marker)
				if at < 0 {
					t.Fatalf("device %s missing", mac)
				}
				row := body[strings.LastIndex(body[:at], "<tr "):]
				row, _, _ = strings.Cut(row, "</tr>")
				reserved := mac == "06:11:22:33:44:55"
				if got := strings.Contains(row, ">reserved</span>"); got != reserved {
					t.Errorf("device %s: reserved chip = %v, want %v", mac, got, reserved)
				}
				if got := strings.Contains(row, "Remove reservation"); got != reserved {
					t.Errorf("device %s: remove action = %v, want %v", mac, got, reserved)
				}
				if got := strings.Contains(row, "Reserve an address"); got == reserved {
					t.Errorf("device %s: reserve action = %v, want %v", mac, got, !reserved)
				}
				if reserved && !strings.Contains(row, "/config/reservations/"+mac+"/delete") {
					t.Error("reserved row must link to its own reservation removal")
				}
			}

			delete(backend.uci["dhcp"], "printer")
			body = get(t, s, "/devices").Body.String()
			if strings.Contains(body, ">reserved</span>") || strings.Contains(body, "Remove reservation") {
				t.Error("after removing the reservation, its chip and remove action must disappear")
			}
			if n := strings.Count(body, `data-verso-entity-tab="reserve"`); n != 3 {
				t.Errorf("after removing the reservation, all three devices should offer Reserve, got %d", n)
			}
		})
	}
}

// TestDeviceActsFollowTheSlotVocabulary: a row draws its applicable slots in
// order. A slot no live plugin claims is inert rather than absent.
func TestDeviceActsFollowTheSlotVocabulary(t *testing.T) {
	s := rosterServer(t)
	acts := s.EntityRowActs("device", "06:11:22:33:44:55", "old-printer", widget.DeviceActTitles(widget.Device{Reserved: true}))
	if len(acts) != 2 {
		t.Fatalf("a reserved device row draws %d acts, want removal and limits", len(acts))
	}
	// No plugin answers in this fixture, so nothing is claimed.
	for _, act := range acts {
		if act.Href != "" || act.Opens {
			t.Errorf("act %+v should be inert while no plugin claims its slot", act)
		}
	}
	if s.EntityListingAct("device") != "" {
		t.Error("with no plugin claiming the listing slot there is nothing to add")
	}
}

// TestOnlineDevicesCountsFromTheNeighbourTable: the sidebar's number is every
// device the kernel still vouches for — a stale entry is a connected device
// that has been quiet toward the router, not an absent one — and an unreadable
// table reports no count at all rather than claiming nobody is here.
func TestOnlineDevicesCountsFromTheNeighbourTable(t *testing.T) {
	s := rosterServer(t)
	if online, ok := s.onlineDevices(); !ok || online != 2 {
		t.Errorf("online devices = %d (known %v), want the reachable and the stale device", online, ok)
	}
	s.neighbors = func() ([]sysstat.Neighbor, error) { return nil, errors.New("no netlink") }
	if online, ok := s.onlineDevices(); ok || online != 0 {
		t.Errorf("online devices = %d (known %v), want no count", online, ok)
	}
}

// TestNeighboursSkipAddressesThatAreNoDevice: a group MAC (IPv6 and IPv4
// multicast, broadcast) and the all-zero MAC a tunnel's entry carries name no
// machine, so they never become a row.
func TestNeighboursSkipAddressesThatAreNoDevice(t *testing.T) {
	agg := aggregateNeighbors([]sysstat.Neighbor{
		{Addr: "ff02::1", MAC: "33:33:00:00:00:01"},
		{Addr: "224.0.0.251", MAC: "01:00:5e:00:00:fb"},
		{Addr: "192.168.1.255", MAC: "ff:ff:ff:ff:ff:ff"},
		{Addr: "0.0.0.0", MAC: "00:00:00:00:00:00"},
		{Addr: "192.168.1.20", MAC: "3C:22:FB:00:00:01"},
	})
	if len(agg) != 1 || agg["3c:22:fb:00:00:01"] == nil {
		t.Errorf("devices = %v, want only 3c:22:fb:00:00:01", agg)
	}
}

// TestLeaseIn: lease expiry reads the way a person says it.
func TestLeaseIn(t *testing.T) {
	now := time.Unix(1000, 0)
	cases := map[int64]string{
		1000 + 11*3600 + 32*60: "in 11h 32m",
		1000 + 40*60:           "in 40 min",
		900:                    "expired",
	}
	for expiry, want := range cases {
		if got := leaseIn(expiry, now); got != want {
			t.Errorf("leaseIn(%d) = %q, want %q", expiry, got, want)
		}
	}
}

// shapingManifest registers a plugin that claims a device's shape slot, which is
// what lights both the ban and the sliders on a device's row and puts a second
// tab in its panel.
func shapingManifest() plugin.Manifest {
	return plugin.Manifest{
		ID: "qos", Name: "Device limits",
		Socket: "/run/verso/qos.sock", SchemaVersion: 1,
		Nav:        []plugin.NavEntry{{Section: "Security", Label: "Device limits", Path: "/"}},
		EntityTabs: []plugin.EntityTab{{Entity: "device", Slot: "shape", Label: "Limits & schedule"}},
		ACL:        plugin.ACL{Write: []plugin.ACLScope{{Scope: "uci", Object: "firewall", Function: "write"}}},
	}
}

// reservingManifest is dnsdhcp as it really ships: claiming a device's reserve
// slot, so a panel beside the limits plugin has the two tabs the design gives it.
func reservingManifest() plugin.Manifest {
	m := dnsdhcpManifest()
	m.EntityTabs = []plugin.EntityTab{{Entity: "device", Slot: "reserve", Label: "Reserved address"}}
	return m
}

func shapingServer(t *testing.T, tr plugin.Transport) *Server {
	t.Helper()
	backend := rosterBackend()
	backend.access = true // the operator holds the plugin's declared write scope
	s := newServerWith(t, backend, tr, []plugin.Manifest{reservingManifest(), shapingManifest()})
	s.readLeases = func() ([]byte, error) { return []byte(testLeases), nil }
	s.neighbors = testNeighbors
	return s
}

// tabTransport answers each plugin with its own envelope, so a panel assembled
// from two contributors can be read the way the browser gets it.
type tabTransport struct {
	fakeTransport
	bySocket map[string]*plugin.Envelope
	// A panel render asks every contributor, so the save has to be told apart
	// from the reads that follow it.
	lastPost plugin.Request
}

func (t *tabTransport) Fetch(_ context.Context, socket string, req plugin.Request) (*plugin.Envelope, error) {
	t.lastSocket, t.lastReq = socket, req
	if req.Method == http.MethodPost {
		t.lastPost = req
	}
	if env, ok := t.bySocket[socket]; ok {
		return env, nil
	}
	return t.env, t.err
}

// twoTabs is a device both plugins have something to say about: dnsdhcp's
// reservation and the limits plugin's policy.
func twoTabs() *tabTransport {
	body := json.RawMessage(`{"type":"form","style":"page","fields":[]}`)
	return &tabTransport{bySocket: map[string]*plugin.Envelope{
		"/run/verso/dnsdhcp.sock": {
			SchemaVersion: 1, Title: "Reserved address",
			Status: http.StatusOK, CTA: "Save reservation", Widget: body,
		},
		"/run/verso/qos.sock": {
			SchemaVersion: 1, Title: "Limits & schedule", Status: http.StatusOK,
			CTA: "Save limits", Widget: body,
		},
	}}
}

// TestEntityPanelTabsAreTheirNames: a panel with more than one tab names each
// reading and nothing more — set at the drawer's 14px label scale, as a row
// panel's strip is, with no chip of where the subject stands beside it.
func TestEntityPanelTabsAreTheirNames(t *testing.T) {
	rec := get(t, shapingServer(t, twoTabs()), "/entity/device/42:e6:ad:ff:b7:af?tab=shape")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET the panel: status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"Reserved address",      // the other tab
		"Limits &amp; schedule", // the tab in force
		">Save limits<",         // its commit row's verb
		"items-center py-3.5 text-sm leading-7 whitespace-nowrap transition",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("entity panel missing %q:\n%s", want, body)
		}
	}
	nav := body[strings.Index(body, "<nav"):strings.Index(body, "</nav>")]
	if strings.Contains(nav, "<span") || strings.Contains(nav, "text-base") {
		t.Errorf("a tab is more than its name at the label scale: %s", nav)
	}
	// The act says what it does and carries no explanation beside it.
	if strings.Contains(body, "ml-auto min-w-0 text-sm leading-snug text-meta") {
		t.Errorf("the commit row carries a note beside its acts:\n%s", body)
	}
	// The tab's form posts back into the panel, so the drawer stays open over
	// the listing rather than navigating to a fragment.
	if !strings.Contains(body, `hx-post="/entity/device/42:e6:ad:ff:b7:af?tab=shape"`) {
		t.Errorf("the tab's form should post back into the panel:\n%s", body)
	}
}

// TestEntityPanelMarksWhatWaits: a tab in a subject's panel is a form like a
// page's, so a field whose option waits on the stage wears the mark there
// too, on every opening, until the change is applied or discarded.
func TestEntityPanelMarksWhatWaits(t *testing.T) {
	tabs := twoTabs()
	tabs.bySocket["/run/verso/qos.sock"].Widget = json.RawMessage(`{"type":"form","style":"page","target":"firewall.shape_42e6","fields":[
	  {"type":"field","name":"rate","label":"Download limit","key":"rate","value":"10"},
	  {"type":"field","name":"burst","label":"Burst","key":"burst","value":"5"}]}`)
	backend := rosterBackend()
	backend.access = true
	backend.changes = map[string][][]string{"firewall": {{"set", "shape_42e6", "rate", "10"}}}
	s := newServerWith(t, backend, tabs, []plugin.Manifest{reservingManifest(), shapingManifest()})
	s.readLeases = func() ([]byte, error) { return []byte(testLeases), nil }
	s.neighbors = testNeighbors
	body := get(t, s, "/entity/device/42:e6:ad:ff:b7:af?tab=shape").Body.String()
	if n := strings.Count(body, "data-verso-staged-row"); n != 1 {
		t.Fatalf("want the one waiting option marked, got %d marks:\n%s", n, body)
	}
	at := strings.Index(body, "data-verso-staged-row")
	if !strings.Contains(body[max(0, at-600):at], "Download limit") {
		t.Errorf("the mark stands on the download limit's row:\n%s", body)
	}
}

// TestReserveSitsOnTheHeadingLine: the roster's forward act stands on the
// heading line, as every listing's does, and the roster runs under it.
func TestReserveSitsOnTheHeadingLine(t *testing.T) {
	m := reservingManifest()
	m.EntityActs = []plugin.EntityAct{{Entity: "device", Slot: "add", Path: "/reserve/{id}"}}
	backend := rosterBackend()
	backend.access = true
	s := newServerWith(t, backend, twoTabs(), []plugin.Manifest{m, shapingManifest()})
	s.readLeases = func() ([]byte, error) { return []byte(testLeases), nil }
	s.neighbors = testNeighbors
	whole := get(t, s, "/devices").Body.String()
	body := whole[strings.LastIndex(whole, "</style>"):]
	heading, act := strings.Index(body, `verso-page-heading">Devices`), strings.Index(body, "Reserve an address")
	roster := strings.Index(body, "<table")
	if heading < 0 || act < 0 || roster < 0 {
		t.Fatalf("roster is missing its heading, act or table:\n%s", body)
	}
	if !(heading < act && act < roster) {
		t.Errorf("reserve should sit on the heading line, over the roster (h1 %d, act %d, table %d)", heading, act, roster)
	}
}

// TestEntityRowShortcutsOpenTheirTab: one clear action opens the limits tab.
func TestEntityRowShortcutsOpenTheirTab(t *testing.T) {
	body := get(t, shapingServer(t, twoTabs()), "/devices").Body.String()
	if n := strings.Count(body, `data-verso-entity-tab="shape"`); n != 3 {
		t.Errorf("one limits action on each of three rows should open the shape tab, got %d", n)
	}
	// With nothing claiming the slot the icons stay drawn and inert, so the
	// column's width is the listing's and not the install's.
	plain := get(t, rosterServer(t), "/devices").Body.String()
	if strings.Contains(plain, `data-verso-entity-tab="shape"`) {
		t.Error("an unclaimed slot should open nothing")
	}
}

// TestEntityPanelSavesThroughTheWriteGate: a tab's submission is its plugin's
// write and passes the same gate any other does (ADR-007) — and a refusal
// answers 422 with what the operator typed still in the controls.
func TestEntityPanelSavesThroughTheWriteGate(t *testing.T) {
	tr := twoTabs()
	s := shapingServer(t, tr)
	rec := postPlugin(t, s, "/entity/device/42:e6:ad:ff:b7:af?tab=shape", url.Values{"allowed": {"1"}})
	if rec.Code != http.StatusOK {
		t.Fatalf("saving a tab: status = %d, want %d", rec.Code, http.StatusOK)
	}
	if tr.lastPost.Path != "/entity/device/42:e6:ad:ff:b7:af" {
		t.Errorf("the plugin should be asked to save on its own entity path, got %q", tr.lastPost.Path)
	}
	if got := tr.lastPost.Form["allowed"]; len(got) != 1 || got[0] != "1" {
		t.Errorf("the submitted form should reach the plugin, got %v", tr.lastPost.Form)
	}
	if _, carried := tr.lastPost.Form["_csrf"]; carried {
		t.Error("the shell's CSRF token is not the plugin's business")
	}

	// A plugin that refused its own submission answers 422, so nothing is
	// written and the browser reads the save as failed.
	tr.bySocket["/run/verso/qos.sock"].Status = http.StatusUnprocessableEntity
	if rec := postPlugin(t, s, "/entity/device/42:e6:ad:ff:b7:af?tab=shape", nil); rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("a refused submission = %d, want 422", rec.Code)
	}

	// An operator whose session lacks the plugin's declared write scope never
	// reaches the plugin at all.
	denied := newServerWith(t, fakeBackend{access: false, uci: rosterBackend().uci}, tr, []plugin.Manifest{reservingManifest(), shapingManifest()})
	denied.readLeases = func() ([]byte, error) { return []byte(testLeases), nil }
	denied.neighbors = testNeighbors
	if rec := postPlugin(t, denied, "/entity/device/42:e6:ad:ff:b7:af?tab=shape", nil); rec.Code != http.StatusForbidden {
		t.Errorf("a save without the grant = %d, want 403", rec.Code)
	}
}
