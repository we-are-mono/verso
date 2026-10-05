// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/we-are-mono/verso/internal/openwrt"
)

func diagBackend() fakeBackend {
	return fakeBackend{
		diag: &fakeDiag{},
		netIfaces: []openwrt.NetIface{
			{Name: "loopback", Device: "lo", Up: true},
			{Name: "lan", Device: "br-lan", Up: true},
			{Name: "wan", Device: "wan0", Up: true},
			{Name: "wan6", Device: "wan0", Up: true},
			{Name: "guest", Device: "", Up: false},
		},
	}
}

// The page is a light masthead over the run's output: the heading, then the
// row that says what to run, and under it the sand the output fills.
func TestDiagnosticsPage(t *testing.T) {
	body := get(t, newServer(t, diagBackend()), "/system/diagnostics").Body.String()
	for _, want := range []string{
		`data-verso-masthead="light"`, ">Diagnostics</h1>",
		`data-verso-diag-form`,
		`<option value="ping" selected>Ping</option>`, `<option value="traceroute">Traceroute</option>`, `<option value="nslookup">DNS lookup</option>`,
		`placeholder="example.com or 1.1.1.1"`,
		// Each device is offered once, under the names netifd gives it; the
		// loopback and an interface with no device are not ways out.
		`<option value="" selected>any interface</option>`, `<option value="br-lan">lan</option>`, `<option value="wan0">wan, wan6</option>`,
		`type="radio" name="family" value="" checked`, `type="radio" name="family" value="4"`, `type="radio" name="family" value="6"`,
		">Run</button>", ">Stop</button>",
		"Nothing run yet.",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("diagnostics page missing %q", want)
		}
	}
	for _, refused := range []string{`value="lo"`, ">guest<", "data-verso-colophon"} {
		if strings.Contains(body, refused) {
			t.Errorf("diagnostics page carries %q", refused)
		}
	}
}

func TestDiagnosticsIsASystemPage(t *testing.T) {
	body := get(t, newServer(t, diagBackend()), "/system/logs").Body.String()
	if !strings.Contains(body, `href="/system/diagnostics"`) {
		t.Error("the rail does not lead to Diagnostics under System")
	}
}

// A run starts in the helper under the operator's own session, with the parts
// the row chose.
func TestDiagnosticsRunStartsTheChosenRun(t *testing.T) {
	backend := diagBackend()
	srv := newServer(t, backend)
	rec := postPlugin(t, srv, "/system/diagnostics/run", url.Values{
		"tool": {"traceroute"}, "target": {" openwrt.org "}, "interface": {"wan0"}, "family": {"6"},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	var answer struct{ Job string }
	if err := json.Unmarshal(rec.Body.Bytes(), &answer); err != nil || answer.Job != "job1" {
		t.Fatalf("answer = %s", rec.Body.String())
	}
	want := []openwrt.DiagnosticRun{{Tool: "traceroute", Target: "openwrt.org", Interface: "wan0", Family: "6"}}
	if !reflect.DeepEqual(backend.diag.starts, want) || backend.diag.sids[0] != "test-sid" {
		t.Fatalf("started %+v as %v", backend.diag.starts, backend.diag.sids)
	}
}

// What cannot run is refused before it reaches the router, in words.
func TestDiagnosticsRunRefusesWhatCannotRun(t *testing.T) {
	for _, tc := range []struct {
		form url.Values
		says string
	}{
		{url.Values{"tool": {"ping"}, "target": {""}}, "Enter a hostname or an IP address."},
		{url.Values{"tool": {"ping"}, "target": {"-f"}}, "Enter a hostname or an IP address."},
		{url.Values{"tool": {"ping"}, "target": {"a;reboot"}}, "Enter a hostname or an IP address."},
		{url.Values{"tool": {"sh"}, "target": {"x.com"}}, "Choose a tool."},
		{url.Values{"tool": {"ping"}, "target": {"x.com"}, "interface": {"eth9"}}, "Choose an interface from the list."},
		{url.Values{"tool": {"ping"}, "target": {"1.1.1.1"}, "family": {"6"}}, "That is an IPv4 address."},
		{url.Values{"tool": {"ping"}, "target": {"::1"}, "family": {"4"}}, "That is an IPv6 address."},
		{url.Values{"tool": {"nslookup"}, "target": {"x.com"}, "interface": {"br-lan"}}, "A DNS lookup is not bound to an interface."},
	} {
		backend := diagBackend()
		rec := postPlugin(t, newServer(t, backend), "/system/diagnostics/run", tc.form)
		if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), tc.says) {
			t.Errorf("%v: %d %s, want 422 %q", tc.form, rec.Code, rec.Body.String(), tc.says)
		}
		if len(backend.diag.starts) != 0 {
			t.Errorf("%v reached the router", tc.form)
		}
	}
}

func TestDiagnosticsStopEndsTheRun(t *testing.T) {
	backend := diagBackend()
	rec := postPlugin(t, newServer(t, backend), "/system/diagnostics/stop", url.Values{"job": {"job1"}})
	if rec.Code != http.StatusNoContent || !reflect.DeepEqual(backend.diag.stops, []string{"job1"}) {
		t.Fatalf("stop = %d, stops %v", rec.Code, backend.diag.stops)
	}
}

// diagFrames reads a run's stream to its end and returns every frame.
func diagFrames(t *testing.T, s *Server, path string) []diagFrame {
	t.Helper()
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)
	token := s.sessions.CreateWithMetadata("test-sid", "root", "", "")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+path, nil)
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: token})
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var frames []diagFrame
	scanner := bufio.NewScanner(res.Body)
	for scanner.Scan() {
		if data, ok := strings.CutPrefix(scanner.Text(), "data: "); ok {
			var frame diagFrame
			if err := json.Unmarshal([]byte(data), &frame); err != nil {
				t.Fatalf("frame %s: %v", data, err)
			}
			frames = append(frames, frame)
		}
	}
	return frames
}

// The stream carries the run's lines as they come, each once, and ends on a
// frame that says how the run ended — then closes rather than poll a finished
// run.
func TestDiagnosticsStreamCarriesTheRunToItsEnd(t *testing.T) {
	zero, one := 0, 1
	backend := diagBackend()
	backend.diag.reads = []openwrt.DiagnosticOutput{
		{Lines: []string{"PING 1.1.1.1 (1.1.1.1): 56 data bytes"}, Next: 1},
		{Lines: []string{"64 bytes from 1.1.1.1: seq=0 ttl=57 time=4.1 ms"}, Next: 2},
		{Lines: nil, Next: 2, Done: true, Code: &zero, Ended: "exited"},
	}
	srv := newServer(t, backend)
	srv.eventInterval = 4 * time.Millisecond
	frames := diagFrames(t, srv, "/system/diagnostics/run?job=job1")
	if len(frames) != 3 {
		t.Fatalf("frames = %+v", frames)
	}
	// a line travels read: its address and its time apart from the words
	want := []diagToken{{"", "64 bytes from "}, {"value", "1.1.1.1"}, {"", ": seq=0 ttl=57 time="}, {"keyword", "4.1 ms"}}
	if !reflect.DeepEqual(frames[1].Lines[0], want) || frames[1].Done {
		t.Errorf("second frame = %+v", frames[1])
	}
	if last := frames[2]; !last.Done || last.Summary != "Done." || last.Tone != "" {
		t.Errorf("last frame = %+v", last)
	}
	if !reflect.DeepEqual(backend.diag.afters, []int{0, 1, 2}) {
		t.Errorf("cursors = %v", backend.diag.afters)
	}

	// A run that ends badly says so in its tone.
	backend = diagBackend()
	backend.diag.reads = []openwrt.DiagnosticOutput{{Done: true, Code: &one, Ended: "exited"}}
	srv = newServer(t, backend)
	frames = diagFrames(t, srv, "/system/diagnostics/run?job=job1")
	if len(frames) != 1 || frames[0].Summary != "Ended with status 1." || frames[0].Tone != "danger" {
		t.Errorf("failed run = %+v", frames)
	}
}

// A run's lines are read in the code block's inks: an address is a value
// (green), a measurement a keyword (denim), and what only frames the reading
// — ping's statistics rule, a hop that did not answer — is meta.
func TestDiagnosticLinesAreReadInTheCodeInks(t *testing.T) {
	for line, want := range map[string][]diagToken{
		"PING example.com (93.184.215.14): 56 data bytes": {{"", "PING example.com ("}, {"value", "93.184.215.14"}, {"", "): 56 data bytes"}},
		" 3  10.0.0.1  0.224 ms":                          {{"", " 3  "}, {"value", "10.0.0.1"}, {"", "  "}, {"keyword", "0.224 ms"}},
		" 5  *":                                           {{"", " 5  "}, {"meta", "*"}},
		"--- 1.1.1.1 ping statistics ---":                 {{"meta", "--- 1.1.1.1 ping statistics ---"}},
		"5 packets transmitted, 5 packets received, 0% packet loss": {{"", "5 packets transmitted, 5 packets received, "}, {"keyword", "0%"}, {"", " packet loss"}},
		"round-trip min/avg/max = 0.048/0.052/0.062 ms":             {{"", "round-trip min/avg/max = "}, {"keyword", "0.048/0.052/0.062 ms"}},
		"Address:\t127.0.0.1:53":                                    {{"", "Address:\t"}, {"value", "127.0.0.1:53"}},
		"Address: 2606:4700::1111":                                  {{"", "Address: "}, {"value", "2606:4700::1111"}},
		"Address: ::1":                                              {{"", "Address: "}, {"value", "::1"}},
		"Name:\tlocalhost.lan":                                      {{"", "Name:\tlocalhost.lan"}},
		"":                                                          {},
	} {
		if got := diagnosticTokens(line); !reflect.DeepEqual(got, want) {
			t.Errorf("%q read as %q, want %q", line, got, want)
		}
	}
}

func TestDiagnosticsStreamSaysHowARunWasCut(t *testing.T) {
	for ended, says := range map[string]string{"stopped": "Stopped.", "timeout": "Stopped after 90 seconds."} {
		backend := diagBackend()
		backend.diag.reads = []openwrt.DiagnosticOutput{{Done: true, Ended: ended}}
		frames := diagFrames(t, newServer(t, backend), "/system/diagnostics/run?job=job1")
		if len(frames) != 1 || frames[0].Summary != says || frames[0].Tone != "warning" {
			t.Errorf("%s: frames = %+v", ended, frames)
		}
	}
	// A run the router no longer holds ends the stream too.
	frames := diagFrames(t, newServer(t, diagBackend()), "/system/diagnostics/run?job=gone")
	if len(frames) != 1 || !frames[0].Done || frames[0].Summary != "This run is no longer on the router." {
		t.Errorf("lost run = %+v", frames)
	}
}
