// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"bufio"
	"context"
	"encoding/json"
	"html"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/we-are-mono/verso/internal/openwrt"
	"github.com/we-are-mono/verso/internal/plugin"
)

// cardTransport is slotTransport with an Overview card's header beside its
// body, as a plugin answers a contribution to the Overview's cards.
type cardTransport struct {
	slotTransport
	card *plugin.OverviewCard
}

func (f *cardTransport) Fetch(ctx context.Context, target plugin.Endpoint, req plugin.Request) (*plugin.Envelope, error) {
	env, err := f.slotTransport.Fetch(ctx, target, req)
	if err == nil && !env.Empty && req.Slot == "shell.overview.cards" {
		env.Card = f.card
	}
	return env, err
}

func cardServer(t *testing.T, card *plugin.OverviewCard, body string) (*Server, *cardTransport) {
	t.Helper()
	tr := &cardTransport{card: card, slotTransport: slotTransport{answers: map[string]answer{"/run/acme.sock /card": {widget: body}}}}
	if body == "" {
		tr.answers["/run/acme.sock /card"] = answer{empty: true}
	}
	// The uplink is up, so the verdict is the plugin card's cause, not
	// "offline".
	backend := metersBackend()
	backend.wan = openwrt.WANState{Devices: []openwrt.WANDevice{{Device: "eth0", Uptime: 60, Routes: []openwrt.WANRoute{{Family: 4, Table: 254, Main: true}}}}}
	s := newServerWith(t, backend, tr, []plugin.Manifest{
		slotPlugin("acme", false, contribution("card", "shell.overview.cards", "after:system")),
	})
	return s, tr
}

func overviewAs(t *testing.T, s *Server, token string) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: token})
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	return rec.Body.String()
}

var renewalFailed = &plugin.OverviewCard{
	Title: "Certificate", Icon: "shield-check", Tone: "warning", Status: "Renewal failed",
	Summary: "expires in 20 d", Cause: "The certificate could not be renewed", Opens: "/",
}

func TestAPluginsOverviewCardStandsWhereItsPositionPutsIt(t *testing.T) {
	s, tr := cardServer(t, renewalFailed, `{"type":"properties","items":[{"label":"Domain","value":"router.example.com"}]}`)
	body := overviewAs(t, s, s.sessions.CreateWithMetadata("test-sid", "root", "", ""))
	system, card := strings.Index(body, `data-overview-card="system"`), strings.Index(body, `data-overview-card="acme.card"`)
	if system < 0 || card < system {
		t.Fatalf("the Certificate card does not follow System:\n%s", body)
	}
	for _, want := range []string{"Certificate", "Renewal failed", "router.example.com", `href="/plugins/acme/"`} {
		if !strings.Contains(body, want) {
			t.Errorf("Overview missing %q", want)
		}
	}
	if asked := tr.asked("/run/acme.sock")["/card"]; asked.Slot != "shell.overview.cards" {
		t.Errorf("acme was asked with slot %q", asked.Slot)
	}
}

func TestAPluginCardWithNothingToShowKeepsItsPlace(t *testing.T) {
	s, _ := cardServer(t, renewalFailed, "")
	body := overviewAs(t, s, s.sessions.CreateWithMetadata("test-sid", "root", "", ""))
	if strings.Contains(body, `data-overview-card="acme.card"`) {
		t.Error("a card with nothing to show is drawn")
	}
	if !strings.Contains(body, `data-overview-kept="acme.card"`) {
		t.Error("a card with nothing to show loses its place in the layout")
	}
}

func TestAPluginCardWithAFormIsNotDrawn(t *testing.T) {
	s, _ := cardServer(t, renewalFailed, `{"type":"form","fields":[]}`)
	body := overviewAs(t, s, s.sessions.CreateWithMetadata("test-sid", "root", "", ""))
	if strings.Contains(body, `data-overview-card="acme.card"`) {
		t.Error("a card whose body asks something is drawn")
	}
}

// The stream says the plugin card's header again from what the page read,
// never asking the plugin, so the verdict keeps its cause every second.
func TestTheStreamSaysAPluginCardWithoutAskingIt(t *testing.T) {
	s, tr := cardServer(t, renewalFailed, `{"type":"properties","items":[]}`)
	token := s.sessions.CreateWithMetadata("test-sid", "root", "", "")
	page := overviewAs(t, s, token)
	lead := regexp.MustCompile(`data-overview-lead class="verso-overview__lead">([^<]*)<`).FindStringSubmatch(page)
	if lead == nil {
		t.Fatal("the page has no verdict")
	}
	tr.mu.Lock()
	asked := len(tr.requests)
	tr.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/overview/events", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: token})
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	scanner := bufio.NewScanner(res.Body)
	scanner.Buffer(make([]byte, 0, 64<<10), 1<<20)
	event := ""
	for scanner.Scan() {
		line := scanner.Text()
		if v, ok := strings.CutPrefix(line, "event: "); ok {
			event = v
		}
		payload, ok := strings.CutPrefix(line, "data: ")
		if !ok || event != "overview" {
			continue
		}
		var status struct {
			Lead  string `json:"lead"`
			Cards []struct {
				ID     string `json:"id"`
				Status string `json:"status"`
			} `json:"cards"`
		}
		if err := json.Unmarshal([]byte(payload), &status); err != nil {
			t.Fatal(err)
		}
		if status.Lead != html.UnescapeString(lead[1]) {
			t.Errorf("the stream's verdict %q is not the page's %q: it lost the plugin card's cause", status.Lead, lead[1])
		}
		found := false
		for _, c := range status.Cards {
			found = found || (c.ID == "acme.card" && c.Status == "Renewal failed")
		}
		if !found {
			t.Errorf("the stream does not carry the plugin card's header: %+v", status.Cards)
		}
		break
	}
	tr.mu.Lock()
	defer tr.mu.Unlock()
	if len(tr.requests) != asked {
		t.Errorf("the stream asked the plugin %d more times", len(tr.requests)-asked)
	}
}
