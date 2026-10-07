// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/we-are-mono/verso/internal/listen"
)

// webForm is Access on a router whose verso config holds web and whose shell
// answers as set says, with what Apply writes recorded.
type webForm struct {
	t       *testing.T
	s       *Server
	set     *fakeListeners
	written []string
	window  func()
}

func newWebForm(t *testing.T, set *fakeListeners, web map[string]any) *webForm {
	t.Helper()
	w := &webForm{t: t, set: set}
	verso := map[string]any{}
	if web != nil {
		verso["web"] = web
	}
	w.s = newServer(t, fakeBackend{
		uci: map[string]map[string]any{"verso": verso},
		webListeners: func(https, http []string, redirect string) error {
			w.written = append(w.written, fmt.Sprintf("%v %v %q", https, http, redirect))
			return nil
		},
	})
	w.s.SetListeners(set)
	w.s.afterRollback = func(_ time.Duration, f func()) { w.window = f }
	return w
}

// at sets the address a request came in on, over HTTPS when the port is one
// of these tests' HTTPS ports.
func at(local string) func(*http.Request) {
	return func(req *http.Request) {
		addr, _ := net.ResolveTCPAddr("tcp", local)
		*req = *req.WithContext(context.WithValue(req.Context(), http.LocalAddrContextKey, net.Addr(addr)))
		req.TLS = nil
		if _, port, _ := net.SplitHostPort(local); port == "443" || port == "8443" || port == "9443" {
			req.TLS = &tls.ConnectionState{}
		}
	}
}

// apply posts the section's Apply as reached on local.
func (w *webForm) apply(local, https, plain string, redirect bool) *http.Response {
	w.t.Helper()
	form := url.Values{"web_https": {https}, "web_http": {plain}}
	if redirect {
		form.Set("web_redirect", "1")
	}
	rec, _ := postPluginRequest(w.t, w.s, "/system/access", form, at(local))
	return rec.Result()
}

// arrive is the browser reaching Access on local.
func (w *webForm) arrive(local string) {
	req, _ := http.NewRequest(http.MethodGet, "/system/access", nil)
	at(local)(req)
	w.s.webArrived(req)
}

func devListeners() *fakeListeners {
	c, _ := listen.New([]string{"0.0.0.0:8443"}, []string{"0.0.0.0:8080"}, "0")
	return &fakeListeners{current: c}
}

func TestAccessShowsWhereTheWebInterfaceAnswersInAFormToApply(t *testing.T) {
	body := get(t, newWebForm(t, listenersHolding(listen.PortsOwn), nil).s, "/system/access").Body.String()
	for _, want := range []string{"HTTPS port", `name="web_https"`, `value="443"`, "HTTP port", `value="80"`, "Redirect HTTP to HTTPS", ">Apply<"} {
		if !strings.Contains(body, want) {
			t.Errorf("the web interface's form is missing %q", want)
		}
	}
	if body := get(t, newWebForm(t, listenersHolding(listen.PortsBeside), nil).s, "/system/access").Body.String(); strings.Contains(body, `name="web_https"`) {
		t.Error("beside LuCI, the ports are offered for editing")
	}
}

func TestApplyingANewPortSendsThePageThereAndClosesTheOldOnArrival(t *testing.T) {
	w := newWebForm(t, devListeners(), map[string]any{"listen_https": []any{"0.0.0.0:8443"}, "listen_http": []any{"0.0.0.0:8080"}, "redirect_https": "0"})
	resp := w.apply("192.0.2.1:8443", "9443", "8080", false)
	if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "https://example.com:9443/system/access" {
		t.Fatalf("%d → %q", resp.StatusCode, resp.Header.Get("Location"))
	}
	if w.set.staged == nil || w.set.staged.HTTPS[0] != "0.0.0.0:9443" || w.set.kept != 0 || len(w.written) != 0 {
		t.Fatalf("the new listener must answer beside the old, nothing written, until the page arrives: %+v, %v", w.set, w.written)
	}
	// Arriving somewhere else keeps nothing; arriving on the new port writes
	// the settings and closes the old.
	w.arrive("192.0.2.1:8443")
	if w.set.kept != 0 || len(w.written) != 0 {
		t.Error("a request on the old port kept the move")
	}
	w.arrive("192.0.2.1:9443")
	if w.set.kept != 1 || len(w.written) != 1 || w.written[0] != `[0.0.0.0:9443] [0.0.0.0:8080] "0"` {
		t.Errorf("the browser arrived: kept %d, written %v", w.set.kept, w.written)
	}
	w.window()
	if w.set.dropped != 0 || len(w.written) != 1 {
		t.Error("the window passing after the arrival changed something")
	}
}

func TestABrowserThatNeverArrivesLeavesTheSettingsAsTheyWere(t *testing.T) {
	w := newWebForm(t, devListeners(), map[string]any{"listen_https": []any{"0.0.0.0:8443"}, "listen_http": []any{"0.0.0.0:8080"}, "redirect_https": "0"})
	w.apply("192.0.2.1:8443", "9443", "8080", false)
	w.window()
	if w.set.dropped != 1 || len(w.written) != 0 {
		t.Errorf("dropped %d, written %v", w.set.dropped, w.written)
	}
}

func TestAChangeThatLeavesThePageWhereItIsAppliesAtOnce(t *testing.T) {
	w := newWebForm(t, devListeners(), map[string]any{"listen_https": []any{"0.0.0.0:8443"}, "listen_http": []any{"0.0.0.0:8080"}, "redirect_https": "0"})
	resp := w.apply("192.0.2.1:8443", "8443", "8081", false)
	if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/system/access" || w.set.kept != 1 {
		t.Errorf("%d → %q, kept %d", resp.StatusCode, resp.Header.Get("Location"), w.set.kept)
	}
}

func TestTheRoutersOwnPortsAreWrittenAsNothing(t *testing.T) {
	w := newWebForm(t, devListeners(), map[string]any{"listen_https": []any{"0.0.0.0:8443", "[::]:8443"}})
	w.apply("192.0.2.1:8080", "443", "80", true)
	w.arrive("192.0.2.1:443")
	if len(w.written) != 1 || w.written[0] != `[] [] ""` {
		t.Errorf("written %v", w.written)
	}
}

func TestAPortItWasBoundToByHandKeepsItsAddress(t *testing.T) {
	w := newWebForm(t, devListeners(), map[string]any{"listen_https": []any{"192.168.1.1:8443"}, "listen_http": []any{"0.0.0.0:8080"}})
	w.apply("192.0.2.1:8080", "9443", "8080", false)
	w.arrive("192.168.1.1:9443")
	if len(w.written) != 1 || !strings.HasPrefix(w.written[0], "[192.168.1.1:9443]") {
		t.Errorf("written %v", w.written)
	}
}

func TestAPortThatCannotBeIsSaidUnderItsField(t *testing.T) {
	for _, tc := range []struct{ https, plain, field string }{
		{"abc", "8080", "web_https"}, {"0", "8080", "web_https"}, {"8443", "70000", "web_http"}, {"8443", "8443", "web_http"},
	} {
		w := newWebForm(t, devListeners(), nil)
		resp := w.apply("192.0.2.1:8443", tc.https, tc.plain, false)
		if resp.StatusCode != http.StatusUnprocessableEntity || len(w.written) != 0 || w.set.staged != nil {
			t.Errorf("%s/%s: %d, written %v", tc.https, tc.plain, resp.StatusCode, w.written)
		}
	}
	held := devListeners()
	held.stageErr = fmt.Errorf("https listener 0.0.0.0:9443: %w", &net.OpError{Op: "listen", Err: &osSyscallError{syscall.EADDRINUSE}})
	w := newWebForm(t, held, nil)
	rec, _ := postPluginRequest(t, w.s, "/system/access", url.Values{"web_https": {"9443"}, "web_http": {"8080"}}, at("192.0.2.1:8443"))
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "Port 9443 is in use on this router.") || len(w.written) != 0 {
		t.Errorf("a port in use: %d, written %v", rec.Code, w.written)
	}
}

// osSyscallError is a bind refusal as the kernel words it.
type osSyscallError struct{ errno syscall.Errno }

func (e *osSyscallError) Error() string { return "bind: " + e.errno.Error() }
func (e *osSyscallError) Unwrap() error { return e.errno }
