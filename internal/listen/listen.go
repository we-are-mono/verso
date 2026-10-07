// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

// Package listen is where Verso answers (ADR-017 §1): the HTTPS and HTTP
// listeners the init script hands over from `verso.web` on the command line,
// what an absent option means, and the redirect an HTTP listener answers with.
package listen

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"syscall"
)

// Config is the shell's listeners, each address:port with the address an IP
// literal or empty for every address of both families.
type Config struct {
	HTTPS    []string
	HTTP     []string
	Redirect bool
	// Defaulted is set when no listener was given: the router's own ports,
	// which another web server may already hold.
	Defaulted bool
	// IsBeside is set on the listeners Verso answers on beside that server.
	IsBeside bool
}

// BesidePort is where Verso answers beside another web server (ADR-017 §2).
const BesidePort = "8443"

// How Verso holds the router's web ports, as it bound them: its own defaults,
// beside another web server that holds them, or listeners set in verso.web.
const (
	PortsOwn    = "own"
	PortsBeside = "beside"
	PortsSet    = "set"
)

// Ports says how these listeners hold the router's web ports.
func (c Config) Ports() string {
	switch {
	case c.IsBeside:
		return PortsBeside
	case c.Defaulted:
		return PortsOwn
	}
	return PortsSet
}

// Beside is where Verso answers when it was given no listeners and could not
// bind the router's ports — another web server, LuCI's uhttpd, holds them, or
// the shell was not given the capability to bind ports under 1024: HTTPS
// alone, on 8443. Listeners someone set are bound as written, and any other
// failure moves nothing.
func (c Config) Beside(err error) (Config, bool) {
	if !c.Defaulted || !(errors.Is(err, syscall.EADDRINUSE) || errors.Is(err, syscall.EACCES)) {
		return c, false
	}
	return Config{HTTPS: []string{"0.0.0.0:" + BesidePort, "[::]:" + BesidePort}, IsBeside: true}, true
}

// New is the listeners the init script hands over from `verso.web`, one
// argument per list entry. An option that is absent arrives empty and means its
// default; redirect is a uci boolean, on unless "0".
func New(https, http []string, redirect string) (Config, error) {
	var c Config
	var err error
	if c.HTTPS, err = listeners(https, "443"); err != nil {
		return c, err
	}
	if c.HTTP, err = listeners(http, "80"); err != nil {
		return c, err
	}
	c.Redirect = redirect != "0"
	c.Defaulted = len(https) == 0 && len(http) == 0
	return c, nil
}

func listeners(given []string, port string) ([]string, error) {
	if len(given) == 0 {
		return []string{"0.0.0.0:" + port, "[::]:" + port}, nil
	}
	addrs := make([]string, len(given))
	for i, a := range given {
		// A bare port is every address, as uhttpd reads one.
		if !strings.Contains(a, ":") {
			a = ":" + a
		}
		host, p, err := net.SplitHostPort(a)
		if n, perr := strconv.Atoi(p); err != nil || perr != nil || n < 0 || n > 65535 || (host != "" && net.ParseIP(host) == nil) {
			return nil, fmt.Errorf("listener %q is not address:port", given[i])
		}
		addrs[i] = a
	}
	return addrs, nil
}

// Listen binds addr in its own family alone: an IPv6 wildcard does not take
// the IPv4 port too, so `0.0.0.0:443` and `[::]:443` bind side by side. An
// empty address is both families on one socket.
func Listen(addr string) (net.Listener, error) {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	network := "tcp"
	if ip := net.ParseIP(host); ip != nil && ip.To4() != nil {
		network = "tcp4"
	} else if ip != nil {
		network = "tcp6"
	}
	return net.Listen(network, addr)
}

// Redirect answers every request with the same one over HTTPS on httpsAddr's
// port: at the name a trusted certificate is issued for when name gives one,
// else at the host the browser asked for. The redirect is temporary and keeps
// the method, so a browser remembers nothing a changed certificate or port
// would make wrong.
func Redirect(httpsAddr string, name func() string) http.Handler {
	_, port, _ := net.SplitHostPort(httpsAddr)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := name()
		if host == "" {
			host = r.Host
			if h, _, err := net.SplitHostPort(r.Host); err == nil {
				host = h
			}
			host = strings.TrimSuffix(strings.TrimPrefix(host, "["), "]")
		}
		if port != "443" {
			host = net.JoinHostPort(host, port)
		} else if strings.Contains(host, ":") {
			host = "[" + host + "]"
		}
		http.Redirect(w, r, "https://"+host+r.URL.RequestURI(), http.StatusTemporaryRedirect)
	})
}
