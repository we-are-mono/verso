// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package listen

import (
	"errors"
	"io"
	"net"
	"net/http"
	"sync"
	"time"
)

// Limits bound what any client that reaches a listener holds before it has
// signed in, so none can take the file descriptors the shell answers the
// operator and dials ubus, the helper and the plugins with. A client that
// connects from enough addresses can still hold every connection Conns allows,
// and the operator's own is refused until one frees. A zero field is no bound.
type Limits struct {
	// Header is how long a connection has for its TLS handshake and each
	// request's headers. A body and a response are not bounded, so a firmware
	// upload and a live stream run as long as they run.
	Header time.Duration
	// Idle is how long a connection waits for its next request.
	Idle time.Duration
	// HeaderBytes is the most a request's headers may take.
	HeaderBytes int
	// Conns is how many connections every listener of a Set holds at once.
	Conns int
	// ConnsPerSource is how many of them one client address holds, IPv4 or
	// IPv6. Every host on a LAN shares the router's IPv6 /64, so a prefix
	// would let one host lock out the rest.
	ConnsPerSource int
}

// DefaultLimits are the shell's: a handshake and headers in 10 seconds, two
// minutes idle, and connections far below the descriptors a router's process
// is given (Go lifts its soft limit to one under the hard limit, typically
// 4095), with room left for many browsers on one host.
var DefaultLimits = Limits{
	Header:         10 * time.Second,
	Idle:           2 * time.Minute,
	HeaderBytes:    64 << 10,
	Conns:          256,
	ConnsPerSource: 32,
}

// server is an http.Server answering with h within l.
func (l Limits) server(h http.Handler) *http.Server {
	// ReadHeaderTimeout bounds the TLS handshake too. ReadTimeout and
	// WriteTimeout stay unset: they would cut an upload or a stream.
	return &http.Server{Handler: h, ReadHeaderTimeout: l.Header, IdleTimeout: l.Idle, MaxHeaderBytes: l.HeaderBytes}
}

// gate counts the connections every listener of a Set holds, all of them and
// from each source, and admits one only under both caps.
type gate struct {
	mu     sync.Mutex
	limits Limits
	open   int
	from   map[string]int
}

func newGate(l Limits) *gate { return &gate{limits: l, from: map[string]int{}} }

func (g *gate) enter(src string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if (g.limits.Conns > 0 && g.open >= g.limits.Conns) || (g.limits.ConnsPerSource > 0 && g.from[src] >= g.limits.ConnsPerSource) {
		return false
	}
	g.open++
	g.from[src]++
	return true
}

func (g *gate) leave(src string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.open--
	if g.from[src]--; g.from[src] == 0 {
		delete(g.from, src)
	}
}

// gated is a listener whose connections pass a gate. One the gate does not
// admit is closed as it is accepted, so its client hears at once, and the
// listener goes on accepting.
type gated struct {
	net.Listener
	gate *gate
}

func (l gated) Accept() (net.Conn, error) {
	for {
		c, err := l.Listener.Accept()
		if err != nil {
			return nil, err
		}
		src := source(c.RemoteAddr())
		if l.gate.enter(src) {
			return &gatedConn{Conn: c, leave: func() { l.gate.leave(src) }}, nil
		}
		_ = c.Close()
	}
}

// gatedConn gives its place back to the gate once, on its first close. It
// half-closes and copies through the connection it wraps, so net/http still
// sends a plain answer's FIN and serves a file with sendfile.
type gatedConn struct {
	net.Conn
	once  sync.Once
	leave func()
}

func (c *gatedConn) Close() error {
	err := c.Conn.Close()
	c.once.Do(c.leave)
	return err
}

func (c *gatedConn) CloseWrite() error {
	if cw, ok := c.Conn.(interface{ CloseWrite() error }); ok {
		return cw.CloseWrite()
	}
	return errors.ErrUnsupported
}

func (c *gatedConn) ReadFrom(r io.Reader) (int64, error) {
	if rf, ok := c.Conn.(io.ReaderFrom); ok {
		return rf.ReadFrom(r)
	}
	return io.Copy(c.Conn, r)
}

// source is the client a connection comes from, as ConnsPerSource counts it:
// its address, an IPv4-mapped one as IPv4 and a link-local one with its link.
func source(a net.Addr) string {
	t, ok := a.(*net.TCPAddr)
	if !ok {
		return a.String()
	}
	if ip := t.IP.To4(); ip != nil {
		return ip.String()
	}
	return (&net.IPAddr{IP: t.IP, Zone: t.Zone}).String()
}
