// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// trickled is a request to ts whose headers promise a body of ten bytes and
// whose body sends one, carrying cookie where one is given.
func trickled(t *testing.T, ts *httptest.Server, path, cookie string) net.Conn {
	t.Helper()
	c, err := net.Dial("tcp", ts.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	head := fmt.Sprintf("POST %s HTTP/1.1\r\nHost: %s\r\nContent-Type: application/x-www-form-urlencoded\r\nContent-Length: 10\r\n", path, ts.Listener.Addr())
	if cookie != "" {
		head += "Cookie: " + sessionCookie + "=" + cookie + "\r\n"
	}
	if _, err := io.WriteString(c, head+"\r\nu"); err != nil {
		t.Fatal(err)
	}
	return c
}

// endsWithin reports whether the server closed c, answering or not, before d
// passed.
func endsWithin(c net.Conn, d time.Duration) bool {
	_ = c.SetReadDeadline(time.Now().Add(d))
	_, err := io.Copy(io.Discard, c)
	var timeout net.Error
	return !(errors.As(err, &timeout) && timeout.Timeout())
}

// TestABodyBeforeSignInHasADeadline: a client that has not signed in and
// trickles a body, to /login or to any other path, has its connection closed
// once the body's time is up (ADR-017 §1), while a signed-in session's body,
// a firmware upload's, runs as long as it runs.
func TestABodyBeforeSignInHasADeadline(t *testing.T) {
	s := newServer(t, fakeBackend{})
	s.bodyWait = 200 * time.Millisecond
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)

	for _, path := range []string{"/login", "/system/access"} {
		if c := trickled(t, ts, path, ""); !endsWithin(c, 3*time.Second) {
			t.Errorf("a body trickled to %s before sign-in holds its connection", path)
		}
	}
	token := s.sessions.CreateWithMetadata("test-sid", "root", "", "")
	if c := trickled(t, ts, "/system/access", token); endsWithin(c, 600*time.Millisecond) {
		t.Error("a signed-in session's slow body is cut")
	}
}

// TestABodyReadInTimeOutlivesItsDeadline: the deadline bounds the body alone,
// so a request that sent its body in time is answered however long its
// handler takes, its context not cancelled, and the connection serves the
// next request.
func TestABodyReadInTimeOutlivesItsDeadline(t *testing.T) {
	var cancelled error
	handler := bodyDeadline(100*time.Millisecond, func(*http.Request) bool { return false },
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = io.Copy(io.Discard, r.Body)
			time.Sleep(300 * time.Millisecond)
			cancelled = r.Context().Err()
			_, _ = io.WriteString(w, "answered")
		}))
	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)
	c, err := net.Dial("tcp", ts.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	r := bufio.NewReader(c)
	for i := range 2 {
		if _, err := io.WriteString(c, "POST / HTTP/1.1\r\nHost: x\r\nContent-Length: 2\r\n\r\nok"); err != nil {
			t.Fatal(err)
		}
		res, err := http.ReadResponse(r, nil)
		if err != nil {
			t.Fatalf("request %d: %v", i, err)
		}
		body, _ := io.ReadAll(res.Body)
		_ = res.Body.Close()
		if !strings.Contains(string(body), "answered") || cancelled != nil {
			t.Errorf("request %d answered %q, its context %v", i, body, cancelled)
		}
	}
}
