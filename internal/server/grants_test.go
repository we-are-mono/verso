// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"maps"
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
// groups beside them. Config history names whatever config an apply or a
// direct edit touched, one no group declares among them, and root reads and
// acts on it whole.
func TestRootIsGrantedEveryShellProbe(t *testing.T) {
	root := loginGrants(t, []string{"*"}, []string{"*"})
	probes := slices.Clone(shellProbes)
	for _, config := range []string{"network", "dhcp", "wireless", "nothing-declares-this"} {
		probes = append(probes, configGrant(config, "read"), configGrant(config, "write"))
	}
	for _, p := range probes {
		if len(grantedBy(root, p)) == 0 {
			t.Errorf("no shipped ACL group grants root %s %s %s", p.scope, p.object, p.function)
		}
	}
}

// TestEveryShellProbeIsListed: TestRootIsGrantedEveryShellProbe holds root to
// shellProbes, so a probe the shell asks must be listed there to be held. Each
// is a package var whose value is an aclProbe literal or a configGrant, and
// shellProbes names every such var; an aclProbe written anywhere else, past
// that list, is configGrant's own, which config history asks of whatever
// config it names.
func TestEveryShellProbeIsListed(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	var parsed []*ast.File
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		parsed = append(parsed, f)
	}
	probeVars, listed, escaped := scanProbes(fset, parsed)
	for _, problem := range escaped {
		t.Error(problem)
	}
	if len(probeVars) == 0 || len(listed) == 0 {
		t.Fatalf("found probe vars %v and shellProbes %v; the scan no longer sees them", probeVars, listed)
	}
	for name, at := range probeVars {
		if !slices.Contains(listed, name) {
			t.Errorf("%s: probe %s is not in shellProbes", at, name)
		}
	}
}

// TestTheProbeScanSeesEveryProbeWritten: an aclProbe written with its type
// left out, as an element of a slice, array or map of them, escapes
// shellProbes as one written whole does; a list of named probes writes none.
func TestTheProbeScanSeesEveryProbeWritten(t *testing.T) {
	const src = `package server
var shellProbes = []aclProbe{listedGrant, configured}
var listedGrant = aclProbe{"ubus", "a", "b"}
var configured = configGrant("network", "read")
var unlisted = aclProbe{"ubus", "c", "d"}
var named = map[string]*aclProbe{"k": &listedGrant, "n": nil}
func asks() { _ = []aclProbe{listedGrant} }
func inline() { _ = aclProbe{"ubus", "e", "f"} }
func sliced() { _ = []aclProbe{{"ubus", "g", "h"}} }
var arrayed = [1]aclProbe{{"ubus", "i", "j"}}
var indexed = [2]aclProbe{1: {"ubus", "k", "l"}}
var mapped = map[string]aclProbe{"k": {"ubus", "m", "n"}}
var keyed = map[aclProbe]bool{{"ubus", "o", "p"}: true}
var pointed = []*aclProbe{{"ubus", "q", "r"}}
`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "probes.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}
	probeVars, listed, escaped := scanProbes(fset, []*ast.File{f})
	if got := slices.Sorted(maps.Keys(probeVars)); !slices.Equal(got, []string{"configured", "listedGrant", "unlisted"}) {
		t.Errorf("probe vars %v, want configured, listedGrant and unlisted", got)
	}
	if !slices.Equal(listed, []string{"listedGrant", "configured"}) {
		t.Errorf("shellProbes lists %v, want listedGrant and configured", listed)
	}
	want := []string{"inline", "sliced", "arrayed", "indexed", "mapped", "keyed", "pointed"}
	if len(escaped) != len(want) || slices.ContainsFunc(want, func(name string) bool {
		return !slices.ContainsFunc(escaped, func(problem string) bool { return strings.Contains(problem, " inside "+name+" ") })
	}) {
		t.Errorf("escaped %q, want one each inside %v", escaped, want)
	}
}

// scanProbes reads files as TestEveryShellProbeIsListed holds them: it
// answers each package var whose value is an aclProbe literal or a
// configGrant, with where it is declared; the vars shellProbes names; and,
// ending in the func or var it sits in, each aclProbe written anywhere else.
func scanProbes(fset *token.FileSet, files []*ast.File) (probeVars map[string]string, listed, escaped []string) {
	probeVars = map[string]string{}
	isProbe := func(typ ast.Expr) bool {
		if star, ok := typ.(*ast.StarExpr); ok {
			typ = star.X
		}
		id, ok := typ.(*ast.Ident)
		return ok && id.Name == "aclProbe"
	}
	isWholeProbe := func(n ast.Node) bool {
		lit, ok := n.(*ast.CompositeLit)
		if !ok {
			return false
		}
		id, ok := lit.Type.(*ast.Ident)
		return ok && id.Name == "aclProbe"
	}
	// isProbeLiteral: an aclProbe literal, or a slice, array or map literal
	// of aclProbes holding one with its type left out.
	isProbeLiteral := func(n ast.Node) bool {
		if isWholeProbe(n) {
			return true
		}
		lit, ok := n.(*ast.CompositeLit)
		if !ok {
			return false
		}
		var elements []ast.Expr
		switch typ := lit.Type.(type) {
		case *ast.ArrayType:
			if isProbe(typ.Elt) {
				for _, elt := range lit.Elts {
					if kv, ok := elt.(*ast.KeyValueExpr); ok {
						elt = kv.Value
					}
					elements = append(elements, elt)
				}
			}
		case *ast.MapType:
			for _, elt := range lit.Elts {
				if kv, ok := elt.(*ast.KeyValueExpr); ok {
					if isProbe(typ.Key) {
						elements = append(elements, kv.Key)
					}
					if isProbe(typ.Value) {
						elements = append(elements, kv.Value)
					}
				}
			}
		}
		return slices.ContainsFunc(elements, func(e ast.Expr) bool {
			elided, ok := e.(*ast.CompositeLit)
			return ok && elided.Type == nil
		})
	}
	isConfigGrant := func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return false
		}
		id, ok := call.Fun.(*ast.Ident)
		return ok && id.Name == "configGrant"
	}
	for _, f := range files {
		for _, decl := range f.Decls {
			switch d := decl.(type) {
			case *ast.FuncDecl:
				if d.Recv == nil && d.Name.Name == "configGrant" {
					continue
				}
				ast.Inspect(d, func(n ast.Node) bool {
					if isProbeLiteral(n) {
						escaped = append(escaped, fmt.Sprintf("%s: an aclProbe written inside %s escapes shellProbes; declare it as a package var and list it", fset.Position(n.Pos()), d.Name.Name))
					}
					return true
				})
			case *ast.GenDecl:
				for _, spec := range d.Specs {
					vs, ok := spec.(*ast.ValueSpec)
					if !ok {
						continue
					}
					for i, value := range vs.Values {
						name := vs.Names[i].Name
						switch {
						case name == "shellProbes":
							lit, ok := value.(*ast.CompositeLit)
							if !ok {
								escaped = append(escaped, fmt.Sprintf("%s: shellProbes is no longer a list the scan reads", fset.Position(value.Pos())))
								continue
							}
							for _, elt := range lit.Elts {
								if id, ok := elt.(*ast.Ident); ok {
									listed = append(listed, id.Name)
								} else {
									escaped = append(escaped, fmt.Sprintf("%s: shellProbes lists a probe by its value; name its var", fset.Position(elt.Pos())))
								}
							}
						case isWholeProbe(value) || isConfigGrant(value):
							probeVars[name] = fset.Position(value.Pos()).String()
						default:
							ast.Inspect(value, func(n ast.Node) bool {
								if isProbeLiteral(n) {
									escaped = append(escaped, fmt.Sprintf("%s: an aclProbe written inside %s escapes shellProbes; declare it as its own var and list it", fset.Position(n.Pos()), name))
								}
								return true
							})
						}
					}
				}
			}
		}
	}
	return probeVars, listed, escaped
}

// TestARestrictedLoginLeavesTheAdminGroupOut: verso-admin's reach is for a
// login whose write list names '*'. One that lists the shell's other groups by
// name reads and writes only the configs those declare, and neither lists nor
// ends another account's sessions. One that reads '*' and writes nothing
// reads what every other group declares and takes nothing of verso-admin's:
// rpcd's session list answers every session's sid, which LuCI's /ubus beside
// Verso would let it call as that session.
func TestARestrictedLoginLeavesTheAdminGroupOut(t *testing.T) {
	admin := []aclProbe{
		configGrant("nothing-declares-this", "read"), configGrant("nothing-declares-this", "write"),
		sessionListGrant, sessionDestroyGrant,
	}
	logins := map[string][]aclGrant{
		"a restricted login": loginGrants(t, []string{"verso-shell", "verso-helper", "verso-plugin-*"}, []string{"verso-shell"}),
		"a read-only login":  loginGrants(t, []string{"*"}, nil),
	}
	root := loginGrants(t, []string{"*"}, []string{"*"})
	for _, p := range admin {
		for login, grants := range logins {
			if got := grantedBy(grants, p); len(got) != 0 {
				t.Errorf("%s is granted %s %s %s by %v", login, p.scope, p.object, p.function, got)
			}
		}
		if got := grantedBy(root, p); !slices.Equal(got, []string{"verso-admin"}) {
			t.Errorf("%s %s %s is granted by %v, want verso-admin alone", p.scope, p.object, p.function, got)
		}
	}
	for _, p := range []aclProbe{configGrant("network", "read"), configGrant("verso", "read"), leaseGrant} {
		want := slices.DeleteFunc(grantedBy(root, p), func(group string) bool { return group == "verso-admin" })
		if got := grantedBy(logins["a read-only login"], p); len(want) == 0 || !slices.Equal(got, want) {
			t.Errorf("a read-only login is granted %s %s %s by %v, want root's groups but verso-admin, %v",
				p.scope, p.object, p.function, got, want)
		}
	}
}
