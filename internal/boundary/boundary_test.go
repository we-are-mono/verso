// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package boundary

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/constant"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
)

const (
	module    = "github.com/we-are-mono/verso"
	pluginPkg = module + "/internal/plugin"
	serverPkg = module + "/internal/server"
)

// labelCarriers are the fields that hold a plugin's nav label in core: the
// manifest's own, and the rail's rows and pages that carry it to the page.
var labelCarriers = [][2]string{
	{pluginPkg, "NavEntry"}, {pluginPkg, "PageTab"},
	{serverPkg, "navRow"}, {serverPkg, "pageTab"},
}

// finding is one place core names a plugin.
type finding struct {
	at   token.Position
	what string
}

func (f finding) String() string { return fmt.Sprintf("%s:%d: %s", f.at.Filename, f.at.Line, f.what) }

// listed is a package as `go list -json` describes it.
type listed struct {
	ImportPath, Dir, Export string
	GoFiles                 []string
	Standard                bool
}

// world is the module's own packages under internal/ and cmd/, and the export
// data each import of theirs, and of a fixture's, resolves to.
type world struct {
	root     string
	packages []listed
	exports  map[string]string
}

var (
	loaded    world
	loadErr   error
	loadWorld sync.Once
)

// load lists the packages once, building their export data in the build
// cache the test run shares.
func load(t *testing.T) world {
	t.Helper()
	loadWorld.Do(func() {
		root, err := filepath.Abs(filepath.Join("..", ".."))
		if err != nil {
			loadErr = err
			return
		}
		cmd := exec.Command("go", "list", "-export", "-deps", "-json=ImportPath,Dir,Export,GoFiles,Standard", "./internal/...", "./cmd/...")
		cmd.Dir = root
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		out, err := cmd.Output()
		if err != nil {
			loadErr = fmt.Errorf("go list: %v: %s", err, stderr.String())
			return
		}
		loaded = world{root: root, exports: map[string]string{}}
		for dec := json.NewDecoder(bytes.NewReader(out)); dec.More(); {
			var p listed
			if err := dec.Decode(&p); err != nil {
				loadErr = err
				return
			}
			loaded.exports[p.ImportPath] = p.Export
			if !p.Standard && (strings.HasPrefix(p.ImportPath, module+"/internal/") || strings.HasPrefix(p.ImportPath, module+"/cmd/")) {
				loaded.packages = append(loaded.packages, p)
			}
		}
	})
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	return loaded
}

// check type-checks files as the package at path, every import read from its
// export data.
func (w world) check(fset *token.FileSet, path string, files []*ast.File) (*types.Info, error) {
	lookup := func(imported string) (io.ReadCloser, error) {
		export, ok := w.exports[imported]
		if !ok || export == "" {
			return nil, fmt.Errorf("no export data for %s", imported)
		}
		return os.Open(export)
	}
	conf := types.Config{Importer: importer.ForCompiler(fset, "gc", lookup)}
	info := &types.Info{
		Types:      map[ast.Expr]types.TypeAndValue{},
		Uses:       map[*ast.Ident]types.Object{},
		Selections: map[*ast.SelectorExpr]*types.Selection{},
	}
	_, err := conf.Check(path, fset, files, info)
	return info, err
}

// scanGo is every place in files that names a plugin.
func scanGo(fset *token.FileSet, files []*ast.File, info *types.Info) []finding {
	var found []finding
	report := func(n ast.Node, what string) { found = append(found, finding{fset.Position(n.Pos()), what}) }
	for _, file := range files {
		ast.Inspect(file, func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.BasicLit:
				if text, err := strconv.Unquote(n.Value); n.Kind == token.STRING && err == nil && strings.HasPrefix(text, "/plugins/") && text != "/plugins/" {
					report(n, "an address into one plugin's mount: "+text)
				}
			case *ast.BinaryExpr:
				if n.Op != token.EQL && n.Op != token.NEQ {
					break
				}
				for _, pair := range [][2]ast.Expr{{n.X, n.Y}, {n.Y, n.X}} {
					if !fixedText(info, pair[1]) {
						continue
					}
					if pluginID(info, pair[0]) {
						report(n, "a plugin's id compared with a fixed text")
					}
					if navLabel(info, pair[0]) {
						report(n, "a plugin's nav label compared with a fixed text")
					}
				}
			case *ast.SwitchStmt:
				if n.Tag == nil || !(pluginID(info, n.Tag) || navLabel(info, n.Tag)) {
					break
				}
				for _, stmt := range n.Body.List {
					for _, value := range stmt.(*ast.CaseClause).List {
						if fixedText(info, value) {
							report(value, "a switch on a plugin's id or nav label with a fixed case")
						}
					}
				}
			case *ast.IndexExpr:
				if _, table := typeOf(info, n.X).(*types.Map); table && navLabel(info, n.Index) {
					report(n, "a table looked up by a plugin's nav label")
				}
			case *ast.CallExpr:
				if sig, ok := typeOf(info, n.Fun).(*types.Signature); ok {
					for i, arg := range n.Args {
						if i < sig.Params().Len() && sig.Params().At(i).Name() == "pluginID" && fixedText(info, arg) {
							report(arg, "a fixed text passed where a plugin's id is expected")
						}
					}
				}
				if callsSlices(info, n.Fun, "Index", "Contains") && slices.ContainsFunc(n.Args, func(arg ast.Expr) bool { return navLabel(info, arg) }) {
					report(n, "a list searched for a plugin's nav label")
				}
			}
			return true
		})
	}
	return found
}

func typeOf(info *types.Info, e ast.Expr) types.Type {
	if t := info.TypeOf(e); t != nil {
		return t.Underlying()
	}
	return nil
}

// fixedText is a non-empty string constant other than the shell's own
// reserved id.
func fixedText(info *types.Info, e ast.Expr) bool {
	tv := info.Types[e]
	if tv.Value == nil || tv.Value.Kind() != constant.String || constant.StringVal(tv.Value) == "" {
		return false
	}
	return !refersTo(info, e, pluginPkg, "ShellID")
}

// refersTo says whether e names the package-level object pkg.name.
func refersTo(info *types.Info, e ast.Expr, pkg, name string) bool {
	var ident *ast.Ident
	switch e := ast.Unparen(e).(type) {
	case *ast.Ident:
		ident = e
	case *ast.SelectorExpr:
		ident = e.Sel
	}
	if ident == nil {
		return false
	}
	obj := info.Uses[ident]
	return obj != nil && obj.Pkg() != nil && obj.Pkg().Path() == pkg && obj.Name() == name
}

// pluginID says whether e holds a plugin's id: a manifest's ID, a field
// named PluginID or Plugin holding text, or a variable named pluginID.
func pluginID(info *types.Info, e ast.Expr) bool {
	switch e := ast.Unparen(e).(type) {
	case *ast.Ident:
		v, ok := info.Uses[e].(*types.Var)
		return ok && v.Name() == "pluginID"
	case *ast.SelectorExpr:
		field, owner := fieldOf(info, e)
		if field == nil {
			return false
		}
		switch field.Name() {
		case "ID":
			return named(owner, pluginPkg, "Manifest")
		case "PluginID", "Plugin":
			basic, ok := field.Type().Underlying().(*types.Basic)
			return ok && basic.Kind() == types.String
		}
	}
	return false
}

// navLabel says whether e is the Label of one of the label carriers.
func navLabel(info *types.Info, e ast.Expr) bool {
	sel, ok := ast.Unparen(e).(*ast.SelectorExpr)
	if !ok {
		return false
	}
	field, owner := fieldOf(info, sel)
	if field == nil || field.Name() != "Label" {
		return false
	}
	return slices.ContainsFunc(labelCarriers, func(c [2]string) bool { return named(owner, c[0], c[1]) })
}

// fieldOf is the field a selector reads and the type that declares it,
// through any embedded fields on the way.
func fieldOf(info *types.Info, sel *ast.SelectorExpr) (*types.Var, types.Type) {
	s := info.Selections[sel]
	if s == nil || s.Kind() != types.FieldVal {
		return nil, nil
	}
	owner := s.Recv()
	path := s.Index()
	for i, index := range path {
		if p, ok := owner.(*types.Pointer); ok {
			owner = p.Elem()
		}
		st, ok := owner.Underlying().(*types.Struct)
		if !ok {
			return nil, nil
		}
		if i == len(path)-1 {
			return st.Field(index), owner
		}
		owner = st.Field(index).Type()
	}
	return nil, nil
}

// named says whether t is the named type pkg.name.
func named(t types.Type, pkg, name string) bool {
	n, ok := t.(*types.Named)
	return ok && n.Obj().Pkg() != nil && n.Obj().Pkg().Path() == pkg && n.Obj().Name() == name
}

// callsSlices says whether fun is one of the slices package's functions.
func callsSlices(info *types.Info, fun ast.Expr, names ...string) bool {
	if index, ok := fun.(*ast.IndexExpr); ok {
		fun = index.X
	}
	for _, name := range names {
		if refersTo(info, fun, "slices", name) {
			return true
		}
	}
	return false
}

// rustInclude is a #[path], include!, include_str! or include_bytes! and the
// file it takes in.
var rustInclude = regexp.MustCompile(`(?:#\[path\s*=\s*|include(?:_str|_bytes)?!\s*\(\s*)"([^"]+)"`)

// scanRust is every place a Rust file takes in a plugin's source other than
// the SDK's.
func scanRust(name string, src []byte) []finding {
	var found []finding
	for i, line := range strings.Split(string(src), "\n") {
		for _, m := range rustInclude.FindAllStringSubmatch(line, -1) {
			if strings.Contains(m[1], "plugins/verso-plugin-") && !strings.Contains(m[1], "plugins/verso-plugin-sdk/") {
				found = append(found, finding{token.Position{Filename: name, Line: i + 1}, "the helper takes in a plugin's source: " + m[1]})
			}
		}
	}
	return found
}

// TestTheShellNamesNoPlugin: core's Go outside tests holds no address into a
// plugin's mount, compares no plugin's id or nav label with a fixed text,
// passes no fixed text where a plugin's id is expected, and looks nothing up
// by a nav label; the helper compiles in no plugin's source but the SDK's.
func TestTheShellNamesNoPlugin(t *testing.T) {
	w := load(t)
	if len(w.packages) == 0 {
		t.Fatal("go list found no packages")
	}
	// go test keeps a passing result for as long as the files and
	// directories the test opened are unchanged. Listing every directory the
	// scan covers makes a file added anywhere in them a change.
	for _, tree := range []string{"internal", "cmd"} {
		err := filepath.WalkDir(filepath.Join(w.root, tree), func(_ string, _ fs.DirEntry, err error) error { return err })
		if err != nil {
			t.Fatal(err)
		}
	}
	var found []finding
	for _, p := range w.packages {
		if len(p.GoFiles) == 0 {
			continue
		}
		fset := token.NewFileSet()
		var files []*ast.File
		for _, name := range p.GoFiles {
			file, err := parser.ParseFile(fset, filepath.Join(p.Dir, name), nil, parser.SkipObjectResolution)
			if err != nil {
				t.Fatal(err)
			}
			files = append(files, file)
		}
		info, err := w.check(fset, p.ImportPath, files)
		if err != nil {
			t.Fatalf("%s: %v", p.ImportPath, err)
		}
		found = append(found, scanGo(fset, files, info)...)
	}
	helper := filepath.Join(w.root, "verso-rpcd", "src")
	err := filepath.WalkDir(helper, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Ext(path) != ".rs" {
			return err
		}
		src, err := os.ReadFile(path)
		if err == nil {
			found = append(found, scanRust(path, src)...)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range found {
		if rel, err := filepath.Rel(w.root, f.at.Filename); err == nil {
			f.at.Filename = rel
		}
		t.Errorf("the shell names a plugin at %s", f)
	}
}

// marked is the lines of a fixture that say "violation".
func marked(src []byte) []int {
	var lines []int
	for i, line := range strings.Split(string(src), "\n") {
		if strings.Contains(line, "// violation") {
			lines = append(lines, i+1)
		}
	}
	return lines
}

func lines(found []finding) []int {
	var out []int
	for _, f := range found {
		out = append(out, f.at.Line)
	}
	slices.Sort(out)
	return slices.Compact(out)
}

// TestTheGuardFindsEachWayOfNamingAPlugin: each check reports its fixture's
// violation at its line and nothing else, so a check that silently matches
// nothing fails here; the clean fixture, OpenWrt's names and the shell's
// reserved id among it, reports nothing.
func TestTheGuardFindsEachWayOfNamingAPlugin(t *testing.T) {
	w := load(t)
	for _, name := range []string{"violations.go", "clean.go"} {
		path := filepath.Join("testdata", name)
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, path, src, parser.SkipObjectResolution)
		if err != nil {
			t.Fatal(err)
		}
		info, err := w.check(fset, "fixture", []*ast.File{file})
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		found := scanGo(fset, []*ast.File{file}, info)
		if got, want := lines(found), marked(src); !slices.Equal(got, want) {
			t.Errorf("%s: reported lines %v, want %v: %v", name, got, want, found)
		}
	}
	src, err := os.ReadFile(filepath.Join("testdata", "helper.rs"))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := lines(scanRust("helper.rs", src)), marked(src); !slices.Equal(got, want) {
		t.Errorf("helper.rs: reported lines %v, want %v", got, want)
	}
}
