// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"context"
	"errors"
	"log"
	"net/http"
	"sync"
)

// What a session may read, as rpcd answers it (ADR-007). Some of what the
// shell shows it reads with its own privilege — the lease file, the kernel's
// neighbour table, its own cache of a helper's answer, config history — so no
// rpcd call stands between that read and the page. Such a reading reaches a
// session only when rpcd grants that session the read guarding the same fact
// through rpcd: the shell asks session.access with the session's sid, and
// rpcd decides.

// aclProbe is one session.access question: object.function within scope.
type aclProbe struct{ scope, object, function string }

// leaseGrant guards the live lease and neighbour tables: the DHCPv6 lease
// read's grant, which the shell's ACL group declares for connected devices.
var leaseGrant = aclProbe{"ubus", "dhcp", "ipv6leases"}

// shellProbes is every probe the shell asks before a reading it makes with
// its own privilege. The ACL groups Verso ships grant each of them to a login
// whose lists name '*', as root's do, so root sees every such reading whatever
// else is installed.
var shellProbes = []aclProbe{leaseGrant}

// permissionsUnchecked is what a page says in place of a reading rpcd did not
// answer the grant of: the reading may be the session's to see.
const permissionsUnchecked = "Your permissions couldn’t be checked just now. Try again in a moment."

// errGrantsUnchecked is a reading withheld because rpcd did not answer its
// grant, rather than because it refused it.
var errGrantsUnchecked = errors.New("verso: rpcd did not answer a grant")

// grantAnswer is rpcd's answer to one probe.
type grantAnswer int

const (
	granted grantAnswer = iota
	refused
	unchecked // rpcd could not be asked, or did not answer
)

// sessionGrants answers one request's probes, each asked of rpcd once. A
// denial and an unreachable rpcd both answer no: what rpcd cannot confirm is
// not shown. answer tells the two apart, so a page can say which.
type sessionGrants struct {
	access func(ctx context.Context, sid, scope, object, function string) (bool, error)
	sid    string
	mu     sync.Mutex
	asked  map[aclProbe]grantAnswer
}

// grantsFor is the session's grants, unasked as yet.
func (s *Server) grantsFor(sid string) *sessionGrants {
	return &sessionGrants{access: s.backend.Access, sid: sid, asked: map[aclProbe]grantAnswer{}}
}

type grantsKey struct{}

// withGrants carries g on the request, so each reading the request makes asks
// through it: a device's panel, whose roster and whose plugins' brokered lease
// reads stand under the same grant, asks rpcd for it once.
func withGrants(r *http.Request, g *sessionGrants) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), grantsKey{}, g))
}

// grantsIn is the grants for sid that the request ctx belongs to carries, or
// fresh ones where it carries none.
func (s *Server) grantsIn(ctx context.Context, sid string) *sessionGrants {
	if g, ok := ctx.Value(grantsKey{}).(*sessionGrants); ok && g.sid == sid {
		return g
	}
	return s.grantsFor(sid)
}

// answer is rpcd's answer to the probe for the session.
func (g *sessionGrants) answer(ctx context.Context, p aclProbe) grantAnswer {
	g.mu.Lock()
	defer g.mu.Unlock()
	if a, seen := g.asked[p]; seen {
		return a
	}
	a := refused
	switch ok, err := g.access(ctx, g.sid, p.scope, p.object, p.function); {
	case err != nil:
		log.Printf("verso: access %s %s.%s unanswered: %v", p.scope, p.object, p.function, err)
		a = unchecked
	case ok:
		a = granted
	}
	g.asked[p] = a
	return a
}

// may reports whether rpcd grants the session the probe.
func (g *sessionGrants) may(ctx context.Context, p aclProbe) bool {
	return g.answer(ctx, p) == granted
}
