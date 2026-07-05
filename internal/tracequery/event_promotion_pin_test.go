package tracequery

// Mechanical pin for the P4 side-table promotion ban (types.go: "NEVER read a
// group field through promotion"). The 8 kind-specific side tables are
// anonymous embedded POINTERS, so Go field promotion makes `ev.Symbol`
// compile silently and panic at runtime whenever the group is nil — the
// review proved four such forms compile with zero warnings. A comment-only
// ban is soft guidance; this test is the hard gate: it type-checks every
// non-test file of tracequery and its downstream importers and fails on any
// field selection whose path runs THROUGH an embedded pointer group of
// Event/EventView. The only legal access shape is via the explicit group
// pointer (pf := ev.PerfFields; if pf != nil { pf.Symbol }), which this scan
// never flags because the outer selection's receiver is *PerfFields, not
// Event.
//
// Precision notes:
//   - tracequery itself is type-checked for real (stdlib-only imports), so
//     the scan there is exact and the check fails loud on any type error.
//   - Downstream packages (internal/tool, internal/tool/width,
//     internal/hitraceconv, cmd) are checked with a hybrid importer: stdlib
//     and tracequery are imported for real, every other import is a fake
//     empty package. Type errors from the fakes are tolerated; selections on
//     tracequery types still resolve because that import is real. Residual
//     blind spot: an Event value produced by an expression the checker could
//     not type (a fake-package function return) is invisible here — the
//     promotion ban comment in types.go still governs those.
//   - The rule is structural, not a name list: any embedded pointer-to-
//     struct field of Event is a side-table group, so a future 9th group is
//     covered automatically.

import (
	"go/ast"
	"go/build"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func TestEventSideTablePromotionBan(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	modPath := readModulePath(t, filepath.Join(root, "go.mod"))
	tracequeryPath := modPath + "/internal/tracequery"

	fset := token.NewFileSet()
	src, ok := importer.ForCompiler(fset, "source", nil).(types.ImporterFrom)
	if !ok {
		t.Fatal("source importer must implement types.ImporterFrom")
	}
	imp := &promotionPinImporter{
		real: src,
		// tracequery's module-internal import closure (go list -deps),
		// imported for real so the strict tracequery check stays exact. If
		// tracequery grows a new internal dep, the strict check below fails
		// loud with an "undefined" error — add the dep here.
		realModulePaths: map[string]bool{
			tracequeryPath:                true,
			modPath + "/internal/logging": true,
		},
		fakes: map[string]*types.Package{},
	}

	targets := []struct {
		rel        string
		importPath string
		strict     bool // fail on type errors (real-import universe)
	}{
		{"internal/tracequery", tracequeryPath, true},
		{"internal/tool", modPath + "/internal/tool", false},
		{"internal/tool/width", modPath + "/internal/tool/width", false},
		{"internal/hitraceconv", modPath + "/internal/hitraceconv", false},
		{"cmd", modPath + "/cmd", false},
	}

	var violations []string
	for _, target := range targets {
		violations = append(violations, scanPackageForPromotion(t, fset, imp, filepath.Join(root, target.rel), target.importPath, target.strict)...)
	}
	sort.Strings(violations)
	for _, v := range violations {
		t.Errorf("promoted side-table field read (nil-panic hazard): %s — access the group pointer explicitly and nil-check it (pf := ev.PerfFields; if pf != nil { ... })", v)
	}
}

// scanPackageForPromotion type-checks the non-test, build-tag-selected files
// of one package and returns every field selection on
// tracequery.Event/EventView whose index path traverses an embedded pointer
// group.
func scanPackageForPromotion(t *testing.T, fset *token.FileSet, imp types.ImporterFrom, dir, importPath string, strict bool) []string {
	t.Helper()
	bpkg, err := build.Default.ImportDir(dir, 0)
	if err != nil {
		t.Fatalf("build.ImportDir(%s): %v", dir, err)
	}
	var files []*ast.File
	for _, name := range bpkg.GoFiles {
		f, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, 0)
		if err != nil {
			t.Fatalf("parse %s/%s: %v", dir, name, err)
		}
		files = append(files, f)
	}
	info := &types.Info{Selections: map[*ast.SelectorExpr]*types.Selection{}}
	var typeErrs []error
	conf := types.Config{
		Importer:    imp,
		FakeImportC: true,
		Error:       func(err error) { typeErrs = append(typeErrs, err) },
	}
	_, _ = conf.Check(importPath, fset, files, info)
	if strict && len(typeErrs) > 0 {
		t.Fatalf("type-checking %s must be clean for the promotion pin to be exact; first error: %v", importPath, typeErrs[0])
	}

	var out []string
	for sel, selection := range info.Selections {
		if selection.Kind() != types.FieldVal {
			continue
		}
		named := derefToNamed(selection.Recv())
		if named == nil {
			continue
		}
		obj := named.Obj()
		if obj.Pkg() == nil || !strings.HasSuffix(obj.Pkg().Path(), "internal/tracequery") {
			continue
		}
		if obj.Name() != "Event" && obj.Name() != "EventView" {
			continue
		}
		if selectionTraversesPointerEmbed(named, selection.Index()) {
			out = append(out, fset.Position(sel.Sel.Pos()).String()+": ."+sel.Sel.Name+" on "+obj.Name())
		}
	}
	return out
}

// selectionTraversesPointerEmbed walks the selection's index path through the
// struct layout and reports whether any traversed (non-final) step is an
// embedded pointer-to-struct field — i.e. a P4 side-table group.
func selectionTraversesPointerEmbed(named *types.Named, index []int) bool {
	cur := named.Underlying()
	for depth, idx := range index {
		st, ok := cur.(*types.Struct)
		if !ok {
			return false
		}
		if idx >= st.NumFields() {
			return false
		}
		field := st.Field(idx)
		if depth < len(index)-1 && field.Embedded() {
			if ptr, ok := field.Type().Underlying().(*types.Pointer); ok {
				if _, ok := ptr.Elem().Underlying().(*types.Struct); ok {
					return true
				}
			}
		}
		typ := field.Type()
		if ptr, ok := typ.Underlying().(*types.Pointer); ok {
			typ = ptr.Elem()
		}
		cur = typ.Underlying()
	}
	return false
}

func derefToNamed(typ types.Type) *types.Named {
	if ptr, ok := typ.Underlying().(*types.Pointer); ok {
		typ = ptr.Elem()
	}
	named, _ := typ.(*types.Named)
	return named
}

// promotionPinImporter imports stdlib and tracequery for real (source
// importer) and fakes everything else so downstream packages type-check in
// seconds without dragging the whole module graph through go/types.
type promotionPinImporter struct {
	real            types.ImporterFrom
	realModulePaths map[string]bool
	fakes           map[string]*types.Package
}

func (i *promotionPinImporter) Import(path string) (*types.Package, error) {
	return i.ImportFrom(path, "", 0)
}

func (i *promotionPinImporter) ImportFrom(path, dir string, mode types.ImportMode) (*types.Package, error) {
	if path == "unsafe" {
		return types.Unsafe, nil
	}
	firstSeg := path
	if slash := strings.Index(path, "/"); slash >= 0 {
		firstSeg = path[:slash]
	}
	if !strings.Contains(firstSeg, ".") || i.realModulePaths[path] {
		return i.real.ImportFrom(path, dir, mode)
	}
	if pkg, ok := i.fakes[path]; ok {
		return pkg, nil
	}
	name := path[strings.LastIndex(path, "/")+1:]
	if dot := strings.Index(name, "."); dot > 0 {
		name = name[:dot]
	}
	pkg := types.NewPackage(path, name)
	pkg.MarkComplete()
	i.fakes[path] = pkg
	return pkg, nil
}

func readModulePath(t *testing.T, gomod string) string {
	t.Helper()
	data, err := os.ReadFile(gomod)
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "module "))
		}
	}
	t.Fatalf("no module line in %s", gomod)
	return ""
}
