// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"sync"

	"github.com/we-are-mono/verso/internal/listen"
)

// fakeListeners is the shell's listener set without a socket: what it answers
// on, what an apply staged beside it, and whether that was kept or dropped.
type fakeListeners struct {
	mu       sync.Mutex
	current  listen.Config
	staged   *listen.Config
	stageErr error
	kept     int
	dropped  int
}

func (f *fakeListeners) Current() listen.Config {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.staged != nil {
		return *f.staged
	}
	return f.current
}

func (f *fakeListeners) Stage(c listen.Config) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.stageErr != nil {
		return f.stageErr
	}
	f.staged = &c
	return nil
}

func (f *fakeListeners) Keep() {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.staged != nil {
		f.current, f.staged = *f.staged, nil
	}
	f.kept++
}

func (f *fakeListeners) Drop() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.staged = nil
	f.dropped++
}

// listenersHolding is a set answering as ports says: on the router's own
// ports, beside another server or for want of the capability to bind them, or
// on listeners set in verso.web.
func listenersHolding(ports string) *fakeListeners {
	switch ports {
	case listen.PortsBeside:
		return &fakeListeners{current: listen.Config{HTTPS: []string{"0.0.0.0:8443", "[::]:8443"}, IsBeside: true}}
	case listen.PortsRefused:
		return &fakeListeners{current: listen.Config{HTTPS: []string{"0.0.0.0:8443", "[::]:8443"}, IsBeside: true, Refused: true}}
	case listen.PortsOwn:
		c, _ := listen.New(nil, nil, "")
		return &fakeListeners{current: c}
	}
	c, _ := listen.New([]string{"0.0.0.0:8443"}, []string{"0.0.0.0:8080"}, "0")
	return &fakeListeners{current: c}
}
