// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"encoding/json"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// shippedACLs is every rpcd access list the repository ships: the shell's own
// and each plugin's, as rpcd reads them from /usr/share/rpcd/acl.d.
func shippedACLs(t *testing.T) []map[string]json.RawMessage {
	t.Helper()
	var files []string
	for _, pattern := range []string{
		"../../docker/rootfs/usr/share/rpcd/acl.d/*.json",
		"../../plugins/*/rootfs/usr/share/rpcd/acl.d/*.json",
	} {
		found, err := filepath.Glob(pattern)
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, found...)
	}
	if len(files) == 0 {
		t.Fatal("no rpcd access lists found")
	}
	var out []map[string]json.RawMessage
	for _, name := range files {
		raw, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		var groups map[string]json.RawMessage
		if err := json.Unmarshal(raw, &groups); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		out = append(out, groups)
	}
	return out
}

// aclGrant is one grant a login gives a session, from the group it came from.
type aclGrant struct {
	group string
	aclProbe
}

// loginAllows is rpcd's rpc_login_test_permission: a matching '!' pattern
// denies, any other matching pattern allows, and write implies read.
func loginAllows(read, write []string, perm, group string) bool {
	list := write
	if perm == "read" {
		list = read
	}
	for _, p := range list {
		if denied, ok := strings.CutPrefix(p, "!"); ok {
			if denied = strings.TrimLeft(denied, " \t"); denied != "" && fnmatch(denied, group) {
				return false
			}
		}
	}
	for _, p := range list {
		if p != "" && !strings.HasPrefix(p, "!") && fnmatch(p, group) {
			return true
		}
	}
	return perm == "read" && loginAllows(read, write, "write", group)
}

// fnmatch is the C library's fnmatch with no flags, as rpcd matches group
// names and session grants, for the names Verso's lists hold: none carries a
// '/' or a '\', where path.Match differs.
func fnmatch(pattern, name string) bool {
	ok, err := path.Match(pattern, name)
	return err == nil && ok
}

// loginGrants is every grant rpcd's login gives a login whose lists are read
// and write, from the shipped access lists, as rpc_login_setup_acls derives
// them: a table scope grants each object's functions, a list scope each object
// with the permission as its function.
func loginGrants(t *testing.T, read, write []string) []aclGrant {
	t.Helper()
	var out []aclGrant
	for _, file := range shippedACLs(t) {
		for group, raw := range file {
			var perms map[string]json.RawMessage
			if json.Unmarshal(raw, &perms) != nil {
				continue
			}
			for _, perm := range []string{"read", "write"} {
				var scopes map[string]json.RawMessage
				if json.Unmarshal(perms[perm], &scopes) != nil || !loginAllows(read, write, perm, group) {
					continue
				}
				for scope, entries := range scopes {
					var table map[string][]string
					var list []string
					switch {
					case json.Unmarshal(entries, &table) == nil:
						for object, functions := range table {
							for _, function := range functions {
								out = append(out, aclGrant{group, aclProbe{scope, object, function}})
							}
						}
					case json.Unmarshal(entries, &list) == nil:
						for _, object := range list {
							out = append(out, aclGrant{group, aclProbe{scope, object, perm}})
						}
					}
				}
			}
		}
	}
	return out
}

// grantedBy is the groups whose grants answer the probe yes, as rpcd's
// session.access matches a grant's object and function against it.
func grantedBy(grants []aclGrant, p aclProbe) []string {
	var groups []string
	for _, g := range grants {
		if g.scope == p.scope && fnmatch(g.object, p.object) && fnmatch(g.function, p.function) && !slices.Contains(groups, g.group) {
			groups = append(groups, g.group)
		}
	}
	return groups
}

// TestRootIsGrantedEveryShellProbe: root's login lists '*', so rpcd gives its
// session every group on disk; the groups Verso ships alone must grant every
// probe the shell asks, or root loses a reading on a router without LuCI's
// groups beside them.
func TestRootIsGrantedEveryShellProbe(t *testing.T) {
	root := loginGrants(t, []string{"*"}, []string{"*"})
	for _, p := range shellProbes {
		if len(grantedBy(root, p)) == 0 {
			t.Errorf("no shipped ACL group grants root %s %s %s", p.scope, p.object, p.function)
		}
	}
}
