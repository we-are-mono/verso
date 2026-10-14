// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package listen

import (
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

// shortLimits bound a test's listeners in fractions of a second.
var shortLimits = Limits{Header: 200 * time.Millisecond, Idle: 200 * time.Millisecond, HeaderBytes: 64 << 10, Conns: 64, ConnsPerSource: 64}

// closedWithin reports whether the server closed c before d passed, reading
// and dropping whatever it sends first.
func closedWithin(c net.Conn, d time.Duration) bool {
	_ = c.SetReadDeadline(time.Now().Add(d))
	_, err := io.Copy(io.Discard, c)
	var timeout net.Error
	return !(errors.As(err, &timeout) && timeout.Timeout())
}

func dial(t *testing.T, addr string) net.Conn {
	t.Helper()
	c, err := net.Dial("tcp4", addr)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

// held is a connection the HTTPS listener at addr took: its handshake is done,
// and it asks for nothing.
func held(t *testing.T, addr string) net.Conn {
	t.Helper()
	c, err := tls.Dial("tcp4", addr, &tls.Config{InsecureSkipVerify: true})
	if err != nil {
		t.Fatalf("the listener refused a connection under its limits: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

// eventually waits for ok, failing the test after two seconds.
func eventually(t *testing.T, what string, ok func() bool) {
	t.Helper()
	for deadline := time.Now().Add(2 * time.Second); !ok(); time.Sleep(10 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatal(what)
		}
	}
}

func openLimited(t *testing.T, shell http.Handler, limits Limits) (set *Set, https, plain string) {
	t.Helper()
	if shell == nil {
		shell = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "shell") })
	}
	set = limitedSet(t, shell, limits)
	https, plain = "127.0.0.1:"+freePort(t), "127.0.0.1:"+freePort(t)
	if err := set.Open(Config{HTTPS: []string{https}, HTTP: []string{plain}}); err != nil {
		t.Fatal(err)
	}
	return set, https, plain
}

// holding is how many connections g holds.
func (g *gate) holding() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.open
}

func TestAConnectionThatSendsNothingIsClosedByTheHeaderDeadline(t *testing.T) {
	set, https, plain := openLimited(t, nil, shortLimits)
	conns := map[string]net.Conn{
		"HTTPS before its handshake": dial(t, https),
		"HTTPS after its handshake":  held(t, https),
		"HTTP":                       dial(t, plain),
		"HTTP with headers unfinished": func() net.Conn {
			c := dial(t, plain)
			_, _ = io.WriteString(c, "GET /login HTTP/1.1\r\nHost: router\r\n")
			return c
		}(),
	}
	for name, c := range conns {
		if !closedWithin(c, 2*time.Second) {
			t.Errorf("%s: still held past the header deadline", name)
		}
	}
	eventually(t, "a connection the header deadline closed kept its place", func() bool { return set.gate.holding() == 0 })
}

func TestAKeptAliveConnectionIsClosedOnceIdle(t *testing.T) {
	limits := shortLimits
	limits.Header = 5 * time.Second
	set, _, plain := openLimited(t, nil, limits)
	c := dial(t, plain)
	_, _ = io.WriteString(c, "GET / HTTP/1.1\r\nHost: router\r\n\r\n")
	_ = c.SetReadDeadline(time.Now().Add(time.Second))
	buf := make([]byte, 512)
	if n, _ := c.Read(buf); !strings.Contains(string(buf[:n]), "shell") {
		t.Fatalf("the request was not answered: %q", buf[:n])
	}
	if !closedWithin(c, 2*time.Second) {
		t.Error("an idle kept-alive connection is held past the idle deadline")
	}
	eventually(t, "a connection the idle deadline closed kept its place", func() bool { return set.gate.holding() == 0 })
}

func TestAnUploadAndAStreamOutlastTheDeadlines(t *testing.T) {
	stream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		_, _ = w.Write(body)
		for range 3 {
			time.Sleep(150 * time.Millisecond)
			_, _ = io.WriteString(w, ".")
			w.(http.Flusher).Flush()
		}
	})
	_, https, _ := openLimited(t, stream, shortLimits)
	upload, sending := io.Pipe()
	go func() {
		for _, part := range []string{"firm", "ware"} {
			time.Sleep(150 * time.Millisecond)
			_, _ = io.WriteString(sending, part)
		}
		_ = sending.Close()
	}()
	client := &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}}
	resp, err := client.Post("https://"+https+"/", "application/octet-stream", upload)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	got, err := io.ReadAll(resp.Body)
	if err != nil || string(got) != "firmware..." {
		t.Errorf("a slow upload and a long answer were cut: %q, %v", got, err)
	}
}

func TestConnectionsPastTheSetsCapAreClosedUntilOneFrees(t *testing.T) {
	limits := shortLimits
	limits.Header, limits.Idle = 10*time.Second, 10*time.Second
	limits.Conns = 2
	_, https, plain := openLimited(t, nil, limits)
	first, _ := held(t, https), held(t, https)
	// The cap is the Set's, so it holds on the HTTP listener too.
	if !closedWithin(dial(t, plain), time.Second) {
		t.Fatal("a connection past the cap was held")
	}
	if answers("https", https) != "" {
		t.Fatal("a request past the cap was answered")
	}
	_ = first.Close()
	eventually(t, "the operator was not answered once a connection freed", func() bool { return answers("https", https) == "shell" })
}

func TestOneSourceCannotTakeEveryConnection(t *testing.T) {
	limits := shortLimits
	limits.Header, limits.Idle = 10*time.Second, 10*time.Second
	limits.ConnsPerSource = 2
	_, https, plain := openLimited(t, nil, limits)
	first, _ := held(t, https), held(t, https)
	if !closedWithin(dial(t, plain), time.Second) {
		t.Fatal("a connection past its source's cap was held")
	}
	operator := &net.TCPAddr{IP: net.IPv4(127, 0, 0, 2)}
	if got := answersFrom(operator, "https", https); got != "shell" {
		t.Errorf("another source was refused while one held its cap: %q", got)
	}
	_ = first.Close()
	eventually(t, "the source was not answered once its connection freed", func() bool { return answers("https", https) == "shell" })
}

// tcpAddr is the TCP address of host, an IP with an optional %zone.
func tcpAddr(host string) *net.TCPAddr {
	ip, zone, _ := strings.Cut(host, "%")
	return &net.TCPAddr{IP: net.ParseIP(ip), Zone: zone}
}

func TestASourceIsOneIPv4OrIPv6Address(t *testing.T) {
	for _, tc := range []struct{ a, b string }{
		{"192.0.2.1", "192.0.2.1"},
		{"::ffff:192.0.2.1", "192.0.2.1"},
		{"2001:db8:1:2::1", "2001:db8:1:2:0:0:0:1"},
		{"fe80::1%br-lan", "fe80::1%br-lan"},
	} {
		if source(tcpAddr(tc.a)) != source(tcpAddr(tc.b)) {
			t.Errorf("%s and %s are one source", tc.a, tc.b)
		}
	}
	// Every host on a LAN shares the router's /64, and a link-local address
	// names a host only on its own link.
	for _, tc := range []struct{ a, b string }{
		{"192.0.2.1", "192.0.2.2"},
		{"2001:db8:1:2::1", "2001:db8:1:2:ffff::9"},
		{"fd00:1:2:3::10", "fd00:1:2:3::11"},
		{"fe80::1%br-lan", "fe80::2%br-lan"},
		{"fe80::1%br-lan", "fe80::1%br-guest"},
	} {
		if source(tcpAddr(tc.a)) == source(tcpAddr(tc.b)) {
			t.Errorf("%s and %s are two sources", tc.a, tc.b)
		}
	}
}

// halfCloser is a connection that says whether it was half-closed and copied
// into.
type halfCloser struct {
	net.Conn
	closedWrite, readFrom bool
}

func (c *halfCloser) CloseWrite() error { c.closedWrite = true; return nil }

func (c *halfCloser) ReadFrom(r io.Reader) (int64, error) {
	c.readFrom = true
	return io.Copy(io.Discard, r)
}

func TestAGatedConnectionHalfClosesAndCopiesThroughTheConnectionItWraps(t *testing.T) {
	inner := &halfCloser{}
	var c net.Conn = &gatedConn{Conn: inner, leave: func() {}}
	cw, ok := c.(interface{ CloseWrite() error })
	if !ok {
		t.Fatal("a gated connection hides CloseWrite, so a plain HTTP answer ends without its FIN")
	}
	rf, ok := c.(io.ReaderFrom)
	if !ok {
		t.Fatal("a gated connection hides ReadFrom, so a plain HTTP answer loses sendfile")
	}
	_ = cw.CloseWrite()
	_, _ = rf.ReadFrom(strings.NewReader("file"))
	if !inner.closedWrite || !inner.readFrom {
		t.Errorf("CloseWrite reached the wrapped connection: %v, ReadFrom: %v", inner.closedWrite, inner.readFrom)
	}
}

func TestAGatedConnectionWithoutHalfCloseStillCopies(t *testing.T) {
	server, client := net.Pipe()
	defer client.Close()
	c := &gatedConn{Conn: server, leave: func() {}}
	defer c.Close()
	if err := c.CloseWrite(); !errors.Is(err, errors.ErrUnsupported) {
		t.Errorf("half-closing a connection that cannot: %v", err)
	}
	go func() { _, _ = c.ReadFrom(strings.NewReader("file")) }()
	buf := make([]byte, 4)
	if _, err := io.ReadFull(client, buf); err != nil || string(buf) != "file" {
		t.Errorf("ReadFrom did not write through: %q, %v", buf, err)
	}
}
