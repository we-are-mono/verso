// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"testing"

	"github.com/we-are-mono/verso/internal/plugin"
)

// peek is sid's last undo, left in place.
func (u *undoRecords) peek(sid string) (undoRecord, bool) {
	u.mu.Lock()
	defer u.mu.Unlock()
	record, ok := u.by[sid]
	return record, ok
}

func dhcpStage() stageFake {
	return newStageFake(map[string][][]string{"dhcp": {
		{"set", "lan", "leasetime", "6h"},
		{"set", "cfg01", "cachesize", "151"},
	}}, nil)
}

// TestUndoRouteTakesARowOut: an undo is a form the page posts, with its
// token; it takes the row out and leads back to the Pending changes page,
// keeping what it took to offer back.
func TestUndoRouteTakesARowOut(t *testing.T) {
	f := dhcpStage()
	s := newServerWith(t, f, &fakeTransport{}, []plugin.Manifest{dhcpManifest()})
	id := rowIDs(t, s)["dhcp: lan.leasetime = 6h"]

	rec := postPlugin(t, s, "/pending/undo", url.Values{"row": {id}})
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != pendingPath {
		t.Fatalf("undo answered %d to %q, want 303 to %s", rec.Code, rec.Header().Get("Location"), pendingPath)
	}
	if want := [][]string{{"set", "cfg01", "cachesize", "151"}}; !reflect.DeepEqual(f.j.uci["dhcp"], want) {
		t.Errorf("dhcp's stage = %v, want %v", f.j.uci["dhcp"], want)
	}
	undo, ok := s.undos.peek("test-sid")
	if !ok || undo.token == "" || len(undo.ops) != 1 || !undo.restorable {
		t.Fatalf("the undo kept %+v, want the one change, restorable, with a token", undo)
	}
	if undo.words == "" {
		t.Error("the undo keeps no words to say what it took")
	}
}

// TestThePageOffersAnUndoBackOnce: the page the undo leads to says what it
// took and offers it back, in the notification; the next visit does not.
func TestThePageOffersAnUndoBackOnce(t *testing.T) {
	f := dhcpStage()
	s := newServerWith(t, f, &fakeTransport{}, []plugin.Manifest{dhcpManifest()})
	id := rowIDs(t, s)["dhcp: lan.leasetime = 6h"]
	postPlugin(t, s, "/pending/undo", url.Values{"row": {id}})

	body := get(t, s, pendingPath).Body.String()
	for _, want := range []string{"data-verso-arrival", "Discarded: dhcp.lan changed.", `action="/pending/restore"`, "Put it back"} {
		if !strings.Contains(body, want) {
			t.Errorf("the page after an undo is missing %q", want)
		}
	}
	if strings.Contains(get(t, s, pendingPath).Body.String(), "Put it back") {
		t.Error("the undo is offered back a second time")
	}
}

// TestUndoRouteTakesAPagesRows: Undo these posts every row of a card.
func TestUndoRouteTakesAPagesRows(t *testing.T) {
	f := dhcpStage()
	s := newServerWith(t, f, &fakeTransport{}, []plugin.Manifest{dhcpManifest()})
	ids := rowIDs(t, s)
	rec := postPlugin(t, s, "/pending/undo", url.Values{"row": {ids["dhcp: lan.leasetime = 6h"], ids["dhcp: cfg01.cachesize = 151"]}})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("undo answered %d", rec.Code)
	}
	if len(f.j.uci["dhcp"]) != 0 {
		t.Errorf("dhcp's stage = %v, want nothing left", f.j.uci["dhcp"])
	}
}

// TestPutBackRoute: the put-back form, carrying the undo's token, stages what
// the undo took out again; a second post, or a stale token, does nothing.
func TestPutBackRoute(t *testing.T) {
	f := dhcpStage()
	s := newServerWith(t, f, &fakeTransport{}, []plugin.Manifest{dhcpManifest()})
	id := rowIDs(t, s)["dhcp: lan.leasetime = 6h"]
	postPlugin(t, s, "/pending/undo", url.Values{"row": {id}})
	undo, _ := s.undos.peek("test-sid")

	if rec := postPlugin(t, s, "/pending/restore", url.Values{"undo": {"stale"}}); rec.Code != http.StatusSeeOther || len(f.j.uci["dhcp"]) != 1 {
		t.Fatalf("a stale token answered %d and left %v", rec.Code, f.j.uci["dhcp"])
	}
	if rec := postPlugin(t, s, "/pending/restore", url.Values{"undo": {undo.token}}); rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != pendingPath {
		t.Fatalf("put back answered %d to %q", rec.Code, rec.Header().Get("Location"))
	}
	want := [][]string{{"set", "cfg01", "cachesize", "151"}, {"set", "lan", "leasetime", "6h"}}
	if !reflect.DeepEqual(f.j.uci["dhcp"], want) {
		t.Errorf("dhcp's stage = %v, want %v", f.j.uci["dhcp"], want)
	}
	postPlugin(t, s, "/pending/restore", url.Values{"undo": {undo.token}})
	if len(f.j.uci["dhcp"]) != 2 {
		t.Errorf("a second put back staged again: %v", f.j.uci["dhcp"])
	}
}

// TestUndoRoutesRequireTheToken: without the session's token neither undo
// nor put back touches the stage.
func TestUndoRoutesRequireTheToken(t *testing.T) {
	f := dhcpStage()
	s := newServerWith(t, f, &fakeTransport{}, []plugin.Manifest{dhcpManifest()})
	id := rowIDs(t, s)["dhcp: lan.leasetime = 6h"]
	for _, path := range []string{"/pending/undo", "/pending/restore"} {
		if rec := postForm(t, s, path, url.Values{"row": {id}}); rec.Code == http.StatusSeeOther && rec.Header().Get("Location") == pendingPath {
			t.Errorf("%s without a token went through", path)
		}
	}
	if len(f.j.uci["dhcp"]) != 2 {
		t.Errorf("dhcp's stage = %v, want it untouched", f.j.uci["dhcp"])
	}
}

// TestUndoThatFailsSaysSo: an undo the router refused part way leaves the
// stage as it was and says so in the danger tone on the way back.
func TestUndoThatFailsSaysSo(t *testing.T) {
	f := dhcpStage()
	s := newServerWith(t, f, &fakeTransport{}, []plugin.Manifest{dhcpManifest()})
	id := rowIDs(t, s)["dhcp: lan.leasetime = 6h"]
	f.j.failOn = "set cfg01 cachesize"
	rec, token := postPluginRequest(t, s, "/pending/undo", url.Values{"row": {id}}, nil)
	if rec.Code != http.StatusSeeOther || len(f.j.uci["dhcp"]) != 2 {
		t.Fatalf("a failed undo answered %d and left %v", rec.Code, f.j.uci["dhcp"])
	}
	if variant, _ := s.sessions.TakeFlash(token); variant != "danger" {
		t.Errorf("the way back says %q, want the danger tone", variant)
	}
}

// TestUndoOfAGoneRowSaysSo: a row another tab took first leaves the stage as
// it is and says so on the way back.
func TestUndoOfAGoneRowSaysSo(t *testing.T) {
	f := dhcpStage()
	s := newServerWith(t, f, &fakeTransport{}, []plugin.Manifest{dhcpManifest()})
	rec := postPlugin(t, s, "/pending/undo", url.Values{"row": {"0000000000000000"}})
	if rec.Code != http.StatusSeeOther || len(f.j.uci["dhcp"]) != 2 {
		t.Fatalf("a gone row answered %d and left %v", rec.Code, f.j.uci["dhcp"])
	}
	if _, ok := s.undos.peek("test-sid"); ok {
		t.Error("an undo that took nothing keeps something to put back")
	}
}
