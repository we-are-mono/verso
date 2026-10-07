// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package listen

import (
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"syscall"
	"testing"
)

func TestAbsentOptionsMeanBothPortsOnEveryAddressAndARedirect(t *testing.T) {
	c, err := New(nil, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(c.HTTPS, []string{"0.0.0.0:443", "[::]:443"}) || !slices.Equal(c.HTTP, []string{"0.0.0.0:80", "[::]:80"}) || !c.Redirect {
		t.Errorf("defaults = %+v", c)
	}
}

func TestListenersAreTakenAsTheInitScriptHandsThem(t *testing.T) {
	c, err := New([]string{"192.168.1.1:8443", "[fd00::1]:8443", "8444"}, []string{"0.0.0.0:8080"}, "0")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(c.HTTP, []string{"0.0.0.0:8080"}) || !slices.Equal(c.HTTPS, []string{"192.168.1.1:8443", "[fd00::1]:8443", ":8444"}) || c.Redirect {
		t.Errorf("config = %+v", c)
	}
}

func TestWithNothingSetAndThePortsHeldVersoAnswersBesideOn8443(t *testing.T) {
	held := &net.OpError{Op: "listen", Err: &os.SyscallError{Syscall: "bind", Err: syscall.EADDRINUSE}}
	c, _ := New(nil, nil, "")
	beside, ok := c.Beside(held)
	if !ok || !beside.IsBeside || !slices.Equal(beside.HTTPS, []string{"0.0.0.0:8443", "[::]:8443"}) || len(beside.HTTP) != 0 {
		t.Fatalf("Beside = %+v, %v", beside, ok)
	}
	if c.Ports() != PortsOwn || beside.Ports() != PortsBeside {
		t.Errorf("Ports = %q then %q", c.Ports(), beside.Ports())
	}
	// Listeners someone set are bound as written: a port they chose that is
	// held is theirs to see, not one to move away from.
	set, _ := New([]string{"0.0.0.0:443"}, nil, "")
	if _, ok := set.Beside(held); ok || set.Ports() != PortsSet {
		t.Error("set listeners moved beside")
	}
	// Any other failure to bind is no other server's doing.
	denied := &net.OpError{Op: "listen", Err: &os.SyscallError{Syscall: "bind", Err: syscall.EACCES}}
	if _, ok := c.Beside(denied); ok {
		t.Error("a refused bind moved beside")
	}
}

func TestAListenerThatIsNoAddressAndPortIsRefused(t *testing.T) {
	for _, bad := range []string{"router:80", "0.0.0.0:http", "0.0.0.0:70000", "[::1]", "1.2.3.4:"} {
		if _, err := New([]string{bad}, nil, ""); err == nil {
			t.Errorf("HTTPS listener %q accepted", bad)
		}
		if _, err := New(nil, []string{bad}, ""); err == nil {
			t.Errorf("HTTP listener %q accepted", bad)
		}
	}
}

// An IPv4 and an IPv6 wildcard on the same port are two sockets, as uhttpd
// binds them: the IPv6 one must not claim IPv4 too, or the second bind fails.
func TestWildcardsOfBothFamiliesBindTheSamePort(t *testing.T) {
	v4, err := Listen("0.0.0.0:0")
	if err != nil {
		t.Fatal(err)
	}
	defer v4.Close()
	_, port, _ := net.SplitHostPort(v4.Addr().String())
	v6, err := Listen("[::]:" + port)
	if err != nil {
		t.Skipf("no IPv6 here: %v", err)
	}
	v6.Close()
}

func TestHTTPIsSentToTheSamePlaceOverHTTPS(t *testing.T) {
	for _, tc := range []struct {
		https, name, host, path, want string
	}{
		{"0.0.0.0:443", "", "192.168.1.1", "/system/access?x=1", "https://192.168.1.1/system/access?x=1"},
		{"0.0.0.0:443", "", "192.168.1.1:80", "/", "https://192.168.1.1/"},
		{"[::]:8443", "", "[fd00::1]:8080", "/", "https://[fd00::1]:8443/"},
		{"[::]:443", "", "[fd00::1]", "/", "https://[fd00::1]/"},
		{"0.0.0.0:443", "router.example.com", "192.168.1.1", "/login", "https://router.example.com/login"},
	} {
		h := Redirect(tc.https, func() string { return tc.name })
		r := httptest.NewRequest(http.MethodPost, "http://"+tc.host+tc.path, nil)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != http.StatusTemporaryRedirect || w.Header().Get("Location") != tc.want {
			t.Errorf("%s via %s: %d %q, want %q", tc.host+tc.path, tc.https, w.Code, w.Header().Get("Location"), tc.want)
		}
	}
}
