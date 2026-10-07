// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package listen

import (
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/we-are-mono/verso/internal/tlscert"
)

// freePort is a port nothing listens on, found by binding one and letting go.
func freePort(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	_, port, _ := net.SplitHostPort(l.Addr().String())
	return port
}

func testSet(t *testing.T) *Set {
	t.Helper()
	cert, key, err := tlscert.Generate([]string{"router"}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	pair, err := tls.X509KeyPair(cert, key)
	if err != nil {
		t.Fatal(err)
	}
	shell := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "shell") })
	set := NewSet(shell, func() *tls.Config { return &tls.Config{Certificates: []tls.Certificate{pair}} }, func() string { return "" }, nil)
	t.Cleanup(set.Close)
	return set
}

// answers reports what scheme://addr answers, "" when nothing does.
func answers(scheme, addr string) string {
	client := &http.Client{
		Timeout:       2 * time.Second,
		Transport:     &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, DisableKeepAlives: true},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	resp, err := client.Get(scheme + "://" + addr + "/")
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if loc := resp.Header.Get("Location"); loc != "" {
		return "→ " + loc
	}
	body, _ := io.ReadAll(resp.Body)
	return string(body)
}

func TestTheOpenSetAnswersOnEachListener(t *testing.T) {
	set := testSet(t)
	https, http := "127.0.0.1:"+freePort(t), "127.0.0.1:"+freePort(t)
	if err := set.Open(Config{HTTPS: []string{https}, HTTP: []string{http}, Redirect: false}); err != nil {
		t.Fatal(err)
	}
	if got := answers("https", https); got != "shell" {
		t.Errorf("HTTPS answers %q", got)
	}
	if got := answers("http", http); got != "shell" {
		t.Errorf("HTTP answers %q with the redirect off", got)
	}
}

func TestStagedListenersAnswerBesideTheOpenOnesUntilKeptOrDropped(t *testing.T) {
	set := testSet(t)
	old, moved := "127.0.0.1:"+freePort(t), "127.0.0.1:"+freePort(t)
	if err := set.Open(Config{HTTPS: []string{old}}); err != nil {
		t.Fatal(err)
	}
	if err := set.Stage(Config{HTTPS: []string{moved}}); err != nil {
		t.Fatal(err)
	}
	if answers("https", old) != "shell" || answers("https", moved) != "shell" {
		t.Fatal("a staged listener must answer beside the open one")
	}
	if set.Current().HTTPS[0] != moved {
		t.Errorf("Current = %+v, want the staged listeners", set.Current())
	}
	set.Drop()
	if answers("https", moved) != "" || answers("https", old) != "shell" {
		t.Error("Drop must close what the stage added and keep what was open")
	}
	if set.Current().HTTPS[0] != old {
		t.Errorf("after Drop Current = %+v", set.Current())
	}

	if err := set.Stage(Config{HTTPS: []string{moved}}); err != nil {
		t.Fatal(err)
	}
	set.Keep()
	if answers("https", old) != "" || answers("https", moved) != "shell" {
		t.Error("Keep must close what the stage dropped and keep what it added")
	}
}

func TestAStageThatCannotBindChangesNothing(t *testing.T) {
	set := testSet(t)
	open, extra := "127.0.0.1:"+freePort(t), "127.0.0.1:"+freePort(t)
	if err := set.Open(Config{HTTPS: []string{open}}); err != nil {
		t.Fatal(err)
	}
	held, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer held.Close()
	err = set.Stage(Config{HTTPS: []string{open, extra}, HTTP: []string{held.Addr().String()}})
	if err == nil || !strings.Contains(err.Error(), held.Addr().String()) {
		t.Fatalf("a held port must refuse the stage and name it: %v", err)
	}
	if answers("https", extra) != "" {
		t.Error("a refused stage left a listener it opened")
	}
	if answers("https", open) != "shell" || set.Current().HTTPS[0] != open || len(set.Current().HTTPS) != 1 {
		t.Error("a refused stage changed what was open")
	}
}

func TestHTTPRedirectsToTheHTTPSListenerAsItIsNow(t *testing.T) {
	set := testSet(t)
	https, http := "127.0.0.1:"+freePort(t), "127.0.0.1:"+freePort(t)
	if err := set.Open(Config{HTTPS: []string{https}, HTTP: []string{http}, Redirect: true}); err != nil {
		t.Fatal(err)
	}
	_, port := hostPort(https)
	if got := answers("http", http); got != "→ https://127.0.0.1:"+port+"/" {
		t.Errorf("HTTP answers %q", got)
	}
	moved := "127.0.0.1:" + freePort(t)
	if err := set.Stage(Config{HTTPS: []string{moved}, HTTP: []string{http}, Redirect: true}); err != nil {
		t.Fatal(err)
	}
	_, port = hostPort(moved)
	if got := answers("http", http); got != "→ https://127.0.0.1:"+port+"/" {
		t.Errorf("HTTP answers %q while the move is staged", got)
	}
	if err := set.Stage(Config{HTTPS: []string{moved}, HTTP: []string{http}, Redirect: false}); err != nil {
		t.Fatal(err)
	}
	if got := answers("http", http); got != "shell" {
		t.Errorf("HTTP answers %q with the redirect turned off", got)
	}
}

func hostPort(addr string) (string, string) {
	h, p, _ := net.SplitHostPort(addr)
	return h, p
}
