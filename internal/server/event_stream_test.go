// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/we-are-mono/verso/internal/history"
	"github.com/we-are-mono/verso/internal/plugin"
)

// streamFrame is one event the stream sent: its id and its payload.
type streamFrame struct {
	id   string
	html string
}

// openEventStream opens the events stream as the session token, with the id
// the browser last saw, and hands each frame on.
func openEventStream(t *testing.T, ctx context.Context, srv *Server, token, lastID string) (<-chan streamFrame, <-chan string) {
	t.Helper()
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+eventStreamPath, nil)
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: token})
	if lastID != "" {
		req.Header.Set("Last-Event-ID", lastID)
	}
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	res, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	frames, comments := make(chan streamFrame, 16), make(chan string, 16)
	go func() {
		defer res.Body.Close()
		defer close(frames)
		scanner := bufio.NewScanner(res.Body)
		var id string
		for scanner.Scan() {
			line := scanner.Text()
			switch {
			case strings.HasPrefix(line, ":"):
				comments <- line
			case strings.HasPrefix(line, "id: "):
				id = strings.TrimPrefix(line, "id: ")
			case strings.HasPrefix(line, "data: "):
				var payload struct{ HTML string }
				_ = json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &payload)
				frames <- streamFrame{id, payload.HTML}
			}
		}
	}()
	return frames, comments
}

func nextFrame(t *testing.T, frames <-chan streamFrame) streamFrame {
	t.Helper()
	select {
	case f, ok := <-frames:
		if !ok {
			t.Fatal("the stream ended")
		}
		return f
	case <-time.After(3 * time.Second):
		t.Fatal("no frame")
	}
	return streamFrame{}
}

// renewedEvent is acme's renewal as recorded.
func renewedEvent() history.Event {
	return history.Event{Time: time.Now(), Kind: "acme.renewed", Detail: renewedDetail}
}

// TestTheEventStreamSaysWhatTheSessionKeeps: on connect the stream sends the
// session's events newer than the one the browser last saw, each the shell's
// own notification in its tone and the reader's words, its id the event's;
// then each new one as it is kept.
func TestTheEventStreamSaysWhatTheSessionKeeps(t *testing.T) {
	srv := newServerWith(t, fakeBackend{access: true}, &fakeTransport{}, []plugin.Manifest{acmeEvents()})
	token := srv.sessions.CreateWithMetadata("sid", "root", "", "")
	all := func(string) bool { return true }
	srv.sessions.tellAll(saidEvent{ID: 4, Event: renewedEvent()}, all)
	srv.sessions.tellAll(saidEvent{ID: 5, Event: renewedEvent()}, all)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	frames, _ := openEventStream(t, ctx, srv, token, "4")
	f := nextFrame(t, frames)
	if f.id != "5" || !strings.Contains(f.html, "The certificate for router.example.com was renewed.") ||
		!strings.Contains(f.html, `data-verso-flash-variant="success"`) {
		t.Errorf("first frame = %+v, want event 5 as a success notification", f)
	}
	srv.sessions.tellAll(saidEvent{ID: 6, Event: renewedEvent()}, all)
	if f := nextFrame(t, frames); f.id != "6" {
		t.Errorf("then = %+v, want event 6", f)
	}
}

// TestTheEventStreamProvesItIsOpen: an idle stream writes a comment every
// keep-alive, so a broken connection is found.
func TestTheEventStreamProvesItIsOpen(t *testing.T) {
	srv := newServer(t, fakeBackend{access: true})
	srv.eventKeepAlive = 20 * time.Millisecond
	token := srv.sessions.CreateWithMetadata("sid", "root", "", "")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, comments := openEventStream(t, ctx, srv, token, "")
	select {
	case <-comments:
	case <-time.After(3 * time.Second):
		t.Fatal("no keep-alive")
	}
}

// TestTheEventStreamEndsWithItsSession: a stream ends when its session does,
// and holding it open moves no idle clock.
func TestTheEventStreamEndsWithItsSession(t *testing.T) {
	now := time.Now()
	srv := newServer(t, fakeBackend{access: true})
	srv.sessions = newSessionsClock(func() time.Time { return now })
	srv.eventKeepAlive = 20 * time.Millisecond
	token := srv.sessions.CreateWithMetadata("sid", "root", "", "")
	before, _ := srv.sessions.peek(token)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	frames, _ := openEventStream(t, ctx, srv, token, "")
	now = now.Add(time.Minute)
	time.Sleep(60 * time.Millisecond)
	if during, ok := srv.sessions.peek(token); !ok || !during.lastSeen.Equal(before.lastSeen) {
		t.Errorf("the open stream moved the idle clock: %v → %v", before.lastSeen, during.lastSeen)
	}
	srv.sessions.destroy(token)
	select {
	case _, open := <-frames:
		if open {
			t.Error("the stream sent a frame for an ended session")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("the stream outlived its session")
	}
}
