// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/we-are-mono/verso/internal/listen"
)

// webStaged is a router whose stage sets verso.web to web, its listeners
// answering as set says, the page reached on local.
type webStaged struct {
	t        *testing.T
	s        *Server
	set      *fakeListeners
	applies  []int
	confirms int
	rollback func()
}

func newWebStaged(t *testing.T, set *fakeListeners, web map[string]any) *webStaged {
	t.Helper()
	w := &webStaged{t: t, set: set}
	backend := fakeBackend{
		applies:  &w.applies,
		confirms: &w.confirms,
		changes:  map[string][][]string{"verso": {{"set", "web", "listen_https", "x"}}},
		uci:      map[string]map[string]any{"verso": {"web": web}},
	}
	if web == nil {
		backend.changes = map[string][][]string{"firewall": {{"set", "wan", "input", "DROP"}}}
	}
	w.s = newServer(t, backend)
	w.s.SetListeners(set)
	w.s.afterRollback = func(_ time.Duration, f func()) { w.rollback = f }
	return w
}

// apply posts the review drawer's Apply as reached on local.
func (w *webStaged) apply(local string) (int, applyAnswer) {
	w.t.Helper()
	rec, _ := postPluginRequest(w.t, w.s, "/uci/apply", nil, func(req *http.Request) {
		addr, _ := net.ResolveTCPAddr("tcp", local)
		*req = *req.WithContext(context.WithValue(req.Context(), http.LocalAddrContextKey, net.Addr(addr)))
	})
	var answer applyAnswer
	_ = json.Unmarshal(rec.Body.Bytes(), &answer)
	return rec.Code, answer
}

func dev() *fakeListeners {
	c, _ := listen.New([]string{"0.0.0.0:8443"}, []string{"0.0.0.0:8080"}, "0")
	return &fakeListeners{current: c}
}

func TestAStageWithoutListenersAppliesAsEver(t *testing.T) {
	w := newWebStaged(t, dev(), nil)
	code, answer := w.apply("192.0.2.1:8080")
	if code != http.StatusOK || answer.Window != uciRollbackTimeout || answer.Move != "" {
		t.Fatalf("%d %+v", code, answer)
	}
	if w.set.staged != nil || len(w.applies) != 1 || w.applies[0] != uciRollbackTimeout {
		t.Errorf("staged %v, applies %v", w.set.staged, w.applies)
	}
}

func TestNewListenersAnswerBeforeTheApplyAndHoldItLonger(t *testing.T) {
	web := map[string]any{"listen_https": []any{"0.0.0.0:9443"}, "listen_http": []any{"0.0.0.0:8080"}, "redirect_https": "0"}
	w := newWebStaged(t, dev(), web)
	code, answer := w.apply("192.0.2.1:8080")
	if code != http.StatusOK || answer.Window != uciListenerRollbackTimeout {
		t.Fatalf("%d %+v", code, answer)
	}
	if w.set.staged == nil || w.set.staged.HTTPS[0] != "0.0.0.0:9443" {
		t.Fatalf("staged = %+v", w.set.staged)
	}
	if len(w.applies) != 1 || w.applies[0] != uciListenerRollbackTimeout {
		t.Errorf("applies = %v", w.applies)
	}
	if answer.Move != "" {
		t.Errorf("the page is still served where it is, yet moves to %q", answer.Move)
	}
}

func TestAPageWhoseListenerGoesMovesToWhereTheShellWillAnswer(t *testing.T) {
	web := map[string]any{"listen_https": []any{"0.0.0.0:9443", "[::]:9443"}, "listen_http": []any{"0.0.0.0:8080"}, "redirect_https": "1"}
	w := newWebStaged(t, dev(), web)
	// On HTTP with the redirect on, the page's own requests would be sent away.
	if _, answer := w.apply("192.0.2.1:8080"); answer.Move != "https://example.com:9443" {
		t.Errorf("move = %q", answer.Move)
	}
	w = newWebStaged(t, dev(), map[string]any{"listen_https": []any{"0.0.0.0:443"}, "listen_http": []any{"0.0.0.0:8080"}, "redirect_https": "0"})
	if _, answer := w.apply("192.0.2.1:7000"); answer.Move != "https://example.com" {
		t.Errorf("move to the default port = %q", answer.Move)
	}
}

func TestListenersThatCannotAnswerRefuseTheApply(t *testing.T) {
	held := dev()
	held.stageErr = errors.New("https listener 0.0.0.0:9443: listen tcp4 0.0.0.0:9443: bind: address already in use")
	w := newWebStaged(t, held, map[string]any{"listen_https": []any{"0.0.0.0:9443"}})
	code, answer := w.apply("192.0.2.1:8080")
	if code != http.StatusUnprocessableEntity || !strings.Contains(answer.Error, "9443") || len(w.applies) != 0 {
		t.Errorf("%d %+v, applies %v", code, answer, w.applies)
	}
	w = newWebStaged(t, dev(), map[string]any{"listen_https": []any{"router:443"}})
	code, answer = w.apply("192.0.2.1:8080")
	if code != http.StatusUnprocessableEntity || answer.Error == "" || w.set.staged != nil || len(w.applies) != 0 {
		t.Errorf("a listener that is no address: %d %+v", code, answer)
	}
}

func TestConfirmKeepsTheNewListenersAndRollbackDropsThem(t *testing.T) {
	web := map[string]any{"listen_https": []any{"0.0.0.0:9443"}, "listen_http": []any{"0.0.0.0:8080"}, "redirect_https": "0"}
	w := newWebStaged(t, dev(), web)
	w.apply("192.0.2.1:8080")
	if rec := postPlugin(t, w.s, "/uci/confirm", nil); rec.Code != http.StatusOK {
		t.Fatalf("confirm = %d", rec.Code)
	}
	if w.set.kept != 1 || w.set.current.HTTPS[0] != "0.0.0.0:9443" {
		t.Errorf("confirm did not keep the staged listeners: %+v", w.set)
	}
	w.rollback() // the window passing after the confirm changes nothing
	if w.set.dropped != 0 {
		t.Error("a kept apply's window dropped the listeners")
	}

	w = newWebStaged(t, dev(), web)
	w.apply("192.0.2.1:8080")
	w.rollback()
	if w.set.dropped != 1 || w.set.staged != nil {
		t.Errorf("an unconfirmed apply kept its listeners: %+v", w.set)
	}
}

func TestBesideLuCIAStageOfOtherSettingsLeavesTheListenersBe(t *testing.T) {
	w := newWebStaged(t, listenersHolding(listen.PortsBeside), map[string]any{"autocheck": "1"})
	if code, _ := w.apply("192.0.2.1:8443"); code != http.StatusOK || w.set.staged != nil {
		t.Errorf("%d, staged %+v", code, w.set.staged)
	}
}
