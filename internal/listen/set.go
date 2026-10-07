// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package listen

import (
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync"
)

// Set is the listeners the shell answers on, changed while it serves
// (ADR-017 §1): the listeners an apply stages are bound beside the open ones,
// then kept once the apply is confirmed or dropped once it is rolled back. No
// listener moves by restarting, so no session ends with one.
type Set struct {
	mu       sync.Mutex
	shell    http.Handler
	secure   func() *tls.Config
	name     func() string
	announce func(net.Addr, string)
	open     map[string]*served
	current  Config
	staged   *Config
	failed   chan error
}

// served is one bound listener and the server answering on it.
type served struct {
	listener net.Listener
	server   *http.Server
}

// NewSet answers with shell over HTTPS, under the TLS config secure makes,
// and over HTTP with shell or the redirect to HTTPS — at the name a trusted
// certificate is issued for when name gives one. announce, when given, hears
// each listener as it opens.
func NewSet(shell http.Handler, secure func() *tls.Config, name func() string, announce func(net.Addr, string)) *Set {
	return &Set{shell: shell, secure: secure, name: name, announce: announce, open: map[string]*served{}, failed: make(chan error, 1)}
}

func key(scheme, addr string) string { return scheme + " " + addr }

// Open binds c and serves it. A listener that cannot be bound closes the ones
// before it, and nothing is open.
func (s *Set) Open(c Config) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.bind(c); err != nil {
		return err
	}
	s.current = c
	return nil
}

// Stage binds the listeners c names that are not open, beside the ones that
// are, and has HTTP answer as c says. A listener that cannot be bound closes
// the ones this call opened, and nothing changes.
func (s *Set) Stage(c Config) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.bind(c); err != nil {
		return err
	}
	s.staged = &c
	return nil
}

// Keep makes the staged listeners the shell's own and closes those they do
// not name.
func (s *Set) Keep() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.staged == nil {
		return
	}
	s.current, s.staged = *s.staged, nil
	s.closeAllBut(s.current)
}

// Drop gives the staged listeners up and closes those the shell's own do not
// name.
func (s *Set) Drop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.staged = nil
	s.closeAllBut(s.current)
}

// Current is how the shell answers now: the staged listeners while a stage
// waits, else its own.
func (s *Set) Current() Config {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.staged != nil {
		return *s.staged
	}
	return s.current
}

// Failed hears a listener that stopped serving for any reason but a close.
func (s *Set) Failed() <-chan error { return s.failed }

// Close closes every listener and every connection on it at once.
func (s *Set) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for k, e := range s.open {
		_ = e.server.Close()
		delete(s.open, k)
	}
}

// bind opens what c names and is not open, answering it from the start. On a
// failure it closes what it opened and says which listener could not be bound.
func (s *Set) bind(c Config) error {
	var opened []string
	want := func(scheme string, addrs []string) error {
		for _, addr := range addrs {
			k := key(scheme, addr)
			if s.open[k] != nil {
				continue
			}
			l, err := Listen(addr)
			if err != nil {
				return fmt.Errorf("%s listener %s: %w", scheme, addr, err)
			}
			server := &http.Server{Handler: s.shell}
			if scheme == "https" {
				server.TLSConfig = s.secure()
			} else {
				server.Handler = http.HandlerFunc(s.plain)
			}
			s.open[k] = &served{l, server}
			opened = append(opened, k)
			if s.announce != nil {
				s.announce(l.Addr(), scheme)
			}
			go s.serve(server, l)
		}
		return nil
	}
	err := want("https", c.HTTPS)
	if err == nil {
		err = want("http", c.HTTP)
	}
	if err != nil {
		for _, k := range opened {
			_ = s.open[k].server.Close()
			delete(s.open, k)
		}
	}
	return err
}

func (s *Set) serve(server *http.Server, l net.Listener) {
	var err error
	if server.TLSConfig != nil {
		err = server.ServeTLS(l, "", "")
	} else {
		err = server.Serve(l)
	}
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		select {
		case s.failed <- err:
		default:
		}
	}
}

// closeAllBut closes every open listener c does not name.
func (s *Set) closeAllBut(c Config) {
	keep := map[string]bool{}
	for _, addr := range c.HTTPS {
		keep[key("https", addr)] = true
	}
	for _, addr := range c.HTTP {
		keep[key("http", addr)] = true
	}
	for k, e := range s.open {
		if !keep[k] {
			_ = e.server.Close()
			delete(s.open, k)
		}
	}
}

// plain answers HTTP as the shell answers now: the redirect to its first
// HTTPS listener as bound, or the shell itself with the redirect off.
func (s *Set) plain(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	c := s.current
	if s.staged != nil {
		c = *s.staged
	}
	var https string
	if c.Redirect && len(c.HTTPS) > 0 {
		if e := s.open[key("https", c.HTTPS[0])]; e != nil {
			https = e.listener.Addr().String()
		}
	}
	s.mu.Unlock()
	if https == "" {
		s.shell.ServeHTTP(w, r)
		return
	}
	Redirect(https, s.name).ServeHTTP(w, r)
}
