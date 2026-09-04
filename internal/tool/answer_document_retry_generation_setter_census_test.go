package tool

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// answer_document_retry_generation_setter_census_test.go — V2-1 / V2-2
// structural tripwires (colleague_merge_audit §40.17 ②③ / §40.18 ③ → §40.45;
// template: hard_arm_mutable_carrier_census_test.go / the V4-1 census).
//
// Retry-local generation state of the answer-document patch transaction —
// the pending staged base and the relation-repair lease — may be written only
// from a registered class of function: the patch tool's single staging entry
// point, the evaluator's lease install / mirror sites, and the full-emit
// explicit rollback. The locked success epilogue and the task reset are the
// only field-level clearers in package types. The patch tool's merged base is
// built by exactly one constructor, and the patch-normalizer chain runs
// before any atomic diagram operation inside Execute.
//
// Totality (§40.45 fold-in): the three setters are exported MutableState
// methods, so the writer census walks EVERY non-test Go file under internal/
// and cmd/ recursively (not two package directories); the field census reads
// every file of package types (the two fields are package-visible); the base
// constructor census counts every constructor spelling of the same merged
// base (NewPartialMutation, ApplyAnswerDocumentV2Patch, an
// AnswerDocumentMutation composite literal, and Apply on a mutation value)
// and, per §40.45 fold-in, every ALIASED spelling — function values, method
// values/expressions, alias/defined types, interface lanes, container
// elements, struct fields, conversions, copies, range frames; the
// staged_for_retry flag census covers address-taking, helper writes, and
// aliased references to the staging entry point.
// Every evasion shape has a self-red subtest that injects the shape into a
// copy of the parsed tree.

// ---------------------------------------------------------------------------
// Parsed tree
// ---------------------------------------------------------------------------

// retryGenerationTree is the parsed non-test source tree under internal/ and
// cmd/, keyed by repo-relative slash path ("internal/tool/x.go").
type retryGenerationTree struct {
	fset  *token.FileSet
	files map[string]*ast.File
	srcs  map[string]string
}

// retryGenerationTreeRoots are walked recursively from the repo root.
var retryGenerationTreeRoots = []string{"internal", "cmd"}

// retryGenerationSentinelPackages must each contribute at least one file to
// the walked tree — a scan that silently narrows back to a directory glob
// goes red here before any writer can escape it.
var retryGenerationSentinelPackages = []string{
	"cmd", "internal/agent", "internal/orchestrator", "internal/repl", "internal/tool", "internal/tool/ground", "internal/types",
}

func loadRetryGenerationTree(t *testing.T) *retryGenerationTree {
	t.Helper()
	tree := &retryGenerationTree{fset: token.NewFileSet(), files: map[string]*ast.File{}, srcs: map[string]string{}}
	repoRoot := filepath.Join("..", "..")
	for _, root := range retryGenerationTreeRoots {
		start := filepath.Join(repoRoot, root)
		err := filepath.WalkDir(start, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			name := d.Name()
			if d.IsDir() {
				// go build ignores testdata and "."/"_"-prefixed directories:
				// nothing there can call the guarded API.
				if path != start && (name == "testdata" || strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_")) {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
				return nil
			}
			rel, err := filepath.Rel(repoRoot, path)
			if err != nil {
				return err
			}
			src, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			key := filepath.ToSlash(rel)
			file, err := parser.ParseFile(tree.fset, key, src, 0)
			if err != nil {
				return err
			}
			tree.files[key] = file
			tree.srcs[key] = string(src)
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", start, err)
		}
	}
	if len(tree.files) == 0 {
		t.Fatal("no sources under internal/ and cmd/")
	}
	return tree
}

// with returns a copy of the tree in which path holds src (added or
// replaced). src only has to parse, not compile.
func (tr *retryGenerationTree) with(t *testing.T, path, src string) *retryGenerationTree {
	t.Helper()
	file, err := parser.ParseFile(tr.fset, path, src, 0)
	if err != nil {
		t.Fatalf("parse synthetic %s: %v", path, err)
	}
	out := &retryGenerationTree{fset: tr.fset, files: make(map[string]*ast.File, len(tr.files)+1), srcs: tr.srcs}
	for k, v := range tr.files {
		out.files[k] = v
	}
	out.files[path] = file
	return out
}

// withInjected replaces path by its own source with `old` rewritten to
// `new` (first occurrence); the anchor must exist.
func (tr *retryGenerationTree) withInjected(t *testing.T, path, old, new string) *retryGenerationTree {
	t.Helper()
	src, ok := tr.srcs[path]
	if !ok {
		t.Fatalf("%s is not in the walked tree", path)
	}
	if !strings.Contains(src, old) {
		t.Fatalf("self-red anchor %q not found in %s", old, path)
	}
	return tr.with(t, path, strings.Replace(src, old, new, 1))
}

func (tr *retryGenerationTree) pos(n ast.Node) string {
	return tr.fset.Position(n.Pos()).String()
}

func retryGenerationPkgDir(path string) string {
	return filepath.ToSlash(filepath.Dir(path))
}

func retryGenerationFuncName(fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return fn.Name.Name
	}
	recv := fn.Recv.List[0].Type
	if star, ok := recv.(*ast.StarExpr); ok {
		recv = star.X
	}
	if ident, ok := recv.(*ast.Ident); ok {
		return ident.Name + "." + fn.Name.Name
	}
	return fn.Name.Name
}

// retryGenerationDecls yields every declaration of a file with the name of
// the enclosing function ("<package scope>" for non-function declarations),
// so package-level aliases are censused too.
func retryGenerationDecls(file *ast.File, visit func(fnName string, node ast.Node)) {
	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok {
			if fn.Body == nil {
				continue
			}
			visit(retryGenerationFuncName(fn), fn.Body)
			continue
		}
		visit("<package scope>", decl)
	}
}

func selectorCallee(call *ast.CallExpr) string {
	switch f := call.Fun.(type) {
	case *ast.SelectorExpr:
		return f.Sel.Name
	case *ast.Ident:
		return f.Name
	}
	return ""
}

func isNilIdent(expr ast.Expr) bool {
	ident, ok := expr.(*ast.Ident)
	return ok && ident.Name == "nil"
}

// typeName returns the bare type name of an Ident / pkg.Ident / *T / (T)
// type expression, or "".
func typeName(expr ast.Expr) string {
	switch x := expr.(type) {
	case *ast.Ident:
		return x.Name
	case *ast.SelectorExpr:
		return x.Sel.Name
	case *ast.StarExpr:
		return typeName(x.X)
	case *ast.ParenExpr:
		return typeName(x.X)
	}
	return ""
}

func retryGenerationCheckOffenders(t *testing.T, offenders []string) {
	t.Helper()
	sort.Strings(offenders)
	for _, o := range offenders {
		t.Error(o)
	}
}

func retryGenerationExpectOffender(t *testing.T, offenders []string, want string) {
	t.Helper()
	for _, o := range offenders {
		if strings.Contains(o, want) {
			return
		}
	}
	t.Fatalf("self-red shape was not reported (want an offender containing %q); got:\n  %s", want, strings.Join(offenders, "\n  "))
}

// ---------------------------------------------------------------------------
// ① Setter writers (§40.18 ③)
// ---------------------------------------------------------------------------

// retryGenerationSetterClass says which setters a registered writer may call.
type retryGenerationSetterClass string

const (
	// rollback: explicit abandonment of the delta base — every argument nil.
	retryGenerationRollback retryGenerationSetterClass = "rollback"
	// install: the evaluator minting (or un-minting) a typed lease from a
	// producer-owned delta.
	retryGenerationInstall retryGenerationSetterClass = "install"
	// mirror: copying one live lease across the dual MutableState carriers.
	retryGenerationMirror retryGenerationSetterClass = "mirror"
	// stage: the patch tool installing base + lease as one generation.
	retryGenerationStage retryGenerationSetterClass = "stage"
)

var retryGenerationSetterAllowed = map[retryGenerationSetterClass]map[string]bool{
	retryGenerationRollback: {"SetAnswerDiagramRelationRepairLease": true, "SetPendingAnswerDocumentPatchBase": true},
	retryGenerationInstall:  {"SetAnswerDiagramRelationRepairLease": true},
	retryGenerationMirror:   {"SetAnswerDiagramRelationRepairLease": true},
	retryGenerationStage:    {"StageAnswerDocumentPatchGeneration": true},
}

var retryGenerationSetters = map[string]bool{
	"SetAnswerDiagramRelationRepairLease": true,
	"SetPendingAnswerDocumentPatchBase":   true,
	"StageAnswerDocumentPatchGeneration":  true,
}

// retryGenerationWriterKey addresses a function by package directory
// (repo-relative), file base name and receiver-qualified function name.
type retryGenerationWriterKey struct{ pkg, file, fn string }

// retryGenerationWriters is the single declared table. Any reference to a
// setter (call, method value, method expression, package-level alias) inside
// any other function of any package under internal/ or cmd/ is red; a
// registered function that no longer exists is red.
var retryGenerationWriters = map[retryGenerationWriterKey]retryGenerationSetterClass{
	{"internal/tool", "emit_answer_document.go", "EmitAnswerDocument.Execute"}:                                  retryGenerationRollback,
	{"internal/tool", "emit_answer_document_patch.go", "stageAnswerDocumentPatchGeneration"}:                    retryGenerationStage,
	{"internal/agent", "answer_document_evaluator.go", "installAnswerDocDiagramRelationRepairLease"}:            retryGenerationInstall,
	{"internal/agent", "answer_document_evaluator.go", "installAnswerDocDiagramParticipantBoundaryRepairLease"}: retryGenerationInstall,
	{"internal/agent", "answer_document_evaluator.go", "currentAnswerDocRelationRepairLeaseAcrossMutableState"}: retryGenerationMirror,
}

func retryGenerationSetterCensus(tree *retryGenerationTree, writers map[retryGenerationWriterKey]retryGenerationSetterClass) (offenders []string) {
	seen := map[retryGenerationWriterKey]bool{}
	for path, file := range tree.files {
		pkg, base := retryGenerationPkgDir(path), filepath.Base(path)
		retryGenerationDecls(file, func(fnName string, body ast.Node) {
			key := retryGenerationWriterKey{pkg, base, fnName}
			class, registered := writers[key]
			direct := map[*ast.SelectorExpr]*ast.CallExpr{}
			ast.Inspect(body, func(n ast.Node) bool {
				if call, ok := n.(*ast.CallExpr); ok {
					if sel, ok := call.Fun.(*ast.SelectorExpr); ok && retryGenerationSetters[sel.Sel.Name] {
						direct[sel] = call
					}
				}
				return true
			})
			ast.Inspect(body, func(n ast.Node) bool {
				sel, ok := n.(*ast.SelectorExpr)
				if !ok || !retryGenerationSetters[sel.Sel.Name] {
					return true
				}
				where := fmt.Sprintf("%s:%s (%s)", path, fnName, tree.pos(sel))
				if !registered {
					offenders = append(offenders, where+" references "+sel.Sel.Name+" but is not a registered retry-generation writer (§40.18 ③: setters ⊆ {success epilogue, explicit rollback, staging, evaluator install/mirror})")
					return true
				}
				seen[key] = true
				call, isCall := direct[sel]
				if !isCall {
					offenders = append(offenders, where+" aliases "+sel.Sel.Name+" as a method value; registered writers must call the setter directly")
					return true
				}
				if !retryGenerationSetterAllowed[class][sel.Sel.Name] {
					offenders = append(offenders, fmt.Sprintf("%s (class %s) calls %s, which that class does not allow", where, class, sel.Sel.Name))
				}
				if class == retryGenerationRollback {
					for _, arg := range call.Args {
						if !isNilIdent(arg) {
							offenders = append(offenders, where+" (class rollback) must pass nil to "+sel.Sel.Name)
						}
					}
				}
				return true
			})
		})
	}
	for key := range writers {
		if !seen[key] {
			offenders = append(offenders, fmt.Sprintf("registered writer %s/%s:%s does not exist or no longer calls a setter; update the table", key.pkg, key.file, key.fn))
		}
	}
	return offenders
}

// TestRetryGenerationSetterCensus_TreeReachesEveryPackage: the walk covers
// the packages that already drive MutableState carriers, so the writer
// census cannot regress to a two-directory glob unnoticed.
func TestRetryGenerationSetterCensus_TreeReachesEveryPackage(t *testing.T) {
	tree := loadRetryGenerationTree(t)
	pkgs := map[string]int{}
	for path := range tree.files {
		pkgs[retryGenerationPkgDir(path)]++
	}
	for _, sentinel := range retryGenerationSentinelPackages {
		if pkgs[sentinel] == 0 {
			t.Errorf("walked tree has no non-test Go file under %s; the census must be total over internal/ and cmd/", sentinel)
		}
	}
	if len(pkgs) < 20 {
		t.Errorf("walked tree spans only %d package directories; expected the whole internal/ + cmd/ tree", len(pkgs))
	}
}

// TestRetryGenerationSetterCensus_WritersAreRegistered: every reference to a
// setter under internal/ and cmd/ sits inside a registered writer, is a
// direct call, and uses only the setters its class allows; rollback passes
// nil for every argument.
func TestRetryGenerationSetterCensus_WritersAreRegistered(t *testing.T) {
	tree := loadRetryGenerationTree(t)
	retryGenerationCheckOffenders(t, retryGenerationSetterCensus(tree, retryGenerationWriters))

	probe := func(pkgDecl, body string) string {
		return "package " + pkgDecl + "\n\nimport \"github.com/hanchaoqun/codrax/internal/types\"\n\n" + body
	}
	t.Run("self_red_caller_in_orchestrator", func(t *testing.T) {
		got := retryGenerationSetterCensus(tree.with(t, "internal/orchestrator/zz_probe.go",
			probe("orchestrator", "func probe(m *types.MutableState) { m.SetPendingAnswerDocumentPatchBase(nil) }")), retryGenerationWriters)
		retryGenerationExpectOffender(t, got, "internal/orchestrator/zz_probe.go:probe")
	})
	t.Run("self_red_caller_in_cmd", func(t *testing.T) {
		got := retryGenerationSetterCensus(tree.with(t, "cmd/zz_probe.go",
			probe("cmd", "func probe(m *types.MutableState) { m.StageAnswerDocumentPatchGeneration(nil, nil) }")), retryGenerationWriters)
		retryGenerationExpectOffender(t, got, "cmd/zz_probe.go:probe")
	})
	t.Run("self_red_caller_in_nested_package", func(t *testing.T) {
		got := retryGenerationSetterCensus(tree.with(t, "internal/tool/ground/zz_probe.go",
			probe("ground", "func probe(m *types.MutableState) { m.SetAnswerDiagramRelationRepairLease(nil) }")), retryGenerationWriters)
		retryGenerationExpectOffender(t, got, "internal/tool/ground/zz_probe.go:probe")
	})
	t.Run("self_red_caller_in_types_sibling", func(t *testing.T) {
		got := retryGenerationSetterCensus(tree.with(t, "internal/types/zz_probe.go",
			"package types\n\nfunc (m *MutableState) probe(l *AnswerDiagramRelationRepairLease) { m.SetAnswerDiagramRelationRepairLease(l) }"), retryGenerationWriters)
		retryGenerationExpectOffender(t, got, "internal/types/zz_probe.go:MutableState.probe")
	})
	t.Run("self_red_method_value_alias", func(t *testing.T) {
		got := retryGenerationSetterCensus(tree.with(t, "internal/orchestrator/zz_probe.go",
			probe("orchestrator", "func probe(m *types.MutableState) { f := m.StageAnswerDocumentPatchGeneration; f(nil, nil) }")), retryGenerationWriters)
		retryGenerationExpectOffender(t, got, "internal/orchestrator/zz_probe.go:probe")
	})
	t.Run("self_red_package_level_alias", func(t *testing.T) {
		got := retryGenerationSetterCensus(tree.with(t, "internal/orchestrator/zz_probe.go",
			probe("orchestrator", "var stageAlias = (*types.MutableState).StageAnswerDocumentPatchGeneration")), retryGenerationWriters)
		retryGenerationExpectOffender(t, got, "internal/orchestrator/zz_probe.go:<package scope>")
	})
	t.Run("self_red_registered_writer_aliases_setter", func(t *testing.T) {
		got := retryGenerationSetterCensus(tree.withInjected(t, "internal/tool/emit_answer_document.go",
			"ctx.Mutable.SetPendingAnswerDocumentPatchBase(nil)",
			"clear := ctx.Mutable.SetPendingAnswerDocumentPatchBase; clear(nil)"), retryGenerationWriters)
		retryGenerationExpectOffender(t, got, "aliases SetPendingAnswerDocumentPatchBase as a method value")
	})
	t.Run("self_red_rollback_with_non_nil_argument", func(t *testing.T) {
		got := retryGenerationSetterCensus(tree.withInjected(t, "internal/tool/emit_answer_document.go",
			"ctx.Mutable.SetAnswerDiagramRelationRepairLease(nil)",
			"ctx.Mutable.SetAnswerDiagramRelationRepairLease(ctx.Mutable.AnswerDiagramRelationRepairLease())"), retryGenerationWriters)
		retryGenerationExpectOffender(t, got, "(class rollback) must pass nil to SetAnswerDiagramRelationRepairLease")
	})
	t.Run("self_red_class_disallows_setter", func(t *testing.T) {
		writers := map[retryGenerationWriterKey]retryGenerationSetterClass{}
		for k, v := range retryGenerationWriters {
			writers[k] = v
		}
		writers[retryGenerationWriterKey{"internal/tool", "emit_answer_document.go", "EmitAnswerDocument.Execute"}] = retryGenerationInstall
		got := retryGenerationSetterCensus(tree, writers)
		retryGenerationExpectOffender(t, got, "(class install) calls SetPendingAnswerDocumentPatchBase, which that class does not allow")
	})
	t.Run("self_red_stale_registration", func(t *testing.T) {
		writers := map[retryGenerationWriterKey]retryGenerationSetterClass{}
		for k, v := range retryGenerationWriters {
			writers[k] = v
		}
		writers[retryGenerationWriterKey{"internal/repl", "ghost.go", "ghost"}] = retryGenerationStage
		got := retryGenerationSetterCensus(tree, writers)
		retryGenerationExpectOffender(t, got, "registered writer internal/repl/ghost.go:ghost does not exist")
	})
}

// ---------------------------------------------------------------------------
// ② Field writers in package types (§40.18 ①)
// ---------------------------------------------------------------------------

// retryGenerationFields are the two retry-local carriers on MutableState.
var retryGenerationFields = map[string]bool{"answerDiagramRelationRepairLease": true, "pendingAnswerDocumentPatchBase": true}

// retryGenerationFieldCloners are the only functions a field may be passed
// to: every read hands out a defensive copy.
var retryGenerationFieldCloners = map[string]bool{"cloneAnswerDocumentV2": true, "cloneAnswerDiagramRelationRepairLease": true}

// retryGenerationFieldClearers are the package-types functions allowed to
// assign the two retry-local fields directly (the locked success epilogue,
// the task reset, and the three typed setters that assign clones). Keys are
// receiver-qualified.
var retryGenerationFieldClearers = map[string]bool{
	"MutableState.commitAcceptedAnswerDocumentLocked":  true,
	"MutableState.ResetAnswerDocumentV2":               true,
	"MutableState.SetPendingAnswerDocumentPatchBase":   true,
	"MutableState.SetAnswerDiagramRelationRepairLease": true,
	"MutableState.StageAnswerDocumentPatchGeneration":  true,
}

// retryGenerationFieldRoot walks an lvalue / operand chain (x.f.g[i].h, *p,
// (x)) and returns the retry-local field selector it is rooted in, if any,
// plus whether the field is the chain's outermost node.
func retryGenerationFieldRoot(expr ast.Expr) (field *ast.SelectorExpr, outermost bool) {
	outermost = true
	for expr != nil {
		switch x := expr.(type) {
		case *ast.SelectorExpr:
			if retryGenerationFields[x.Sel.Name] {
				return x, outermost
			}
			expr = x.X
		case *ast.IndexExpr:
			expr = x.X
		case *ast.StarExpr:
			expr = x.X
		case *ast.ParenExpr:
			expr = x.X
		default:
			return nil, false
		}
		outermost = false
	}
	return nil, false
}

func retryGenerationFieldCensus(tree *retryGenerationTree, clearers map[string]bool) (offenders []string) {
	seen := map[string]bool{}
	for path, file := range tree.files {
		if retryGenerationPkgDir(path) != "internal/types" {
			continue
		}
		retryGenerationDecls(file, func(fnName string, body ast.Node) {
			consumed := map[*ast.SelectorExpr]bool{}
			where := func(n ast.Node) string { return fmt.Sprintf("%s:%s (%s)", path, fnName, tree.pos(n)) }
			write := func(lhs ast.Expr, verb string) {
				field, outermost := retryGenerationFieldRoot(lhs)
				if field == nil {
					return
				}
				consumed[field] = true
				switch {
				case !outermost:
					offenders = append(offenders, where(lhs)+" "+verb+" through MutableState."+field.Sel.Name+" in place; the retry-local carriers are replaced whole by their typed setters only (§40.18 ①)")
				case !clearers[fnName]:
					offenders = append(offenders, where(lhs)+" "+verb+" MutableState."+field.Sel.Name+"; only the locked success epilogue, the task reset, and the typed setters may (§40.18 ①)")
				default:
					seen[fnName] = true
				}
			}
			ast.Inspect(body, func(n ast.Node) bool {
				switch node := n.(type) {
				case *ast.AssignStmt:
					for _, lhs := range node.Lhs {
						write(lhs, "assigns")
					}
				case *ast.IncDecStmt:
					write(node.X, "assigns")
				case *ast.RangeStmt:
					write(node.Key, "assigns")
					write(node.Value, "assigns")
				case *ast.UnaryExpr:
					if node.Op == token.AND {
						if field, _ := retryGenerationFieldRoot(node.X); field != nil {
							consumed[field] = true
							offenders = append(offenders, where(node)+" takes the address of MutableState."+field.Sel.Name+"; a pointer helper could clear it outside the registered clearers (§40.18 ①)")
						}
					}
				case *ast.CallExpr:
					if retryGenerationFieldCloners[selectorCallee(node)] && len(node.Args) == 1 {
						if sel, ok := node.Args[0].(*ast.SelectorExpr); ok && retryGenerationFields[sel.Sel.Name] {
							consumed[sel] = true
						}
					}
				case *ast.BinaryExpr:
					if node.Op == token.EQL || node.Op == token.NEQ {
						for _, side := range []ast.Expr{node.X, node.Y} {
							if sel, ok := side.(*ast.SelectorExpr); ok && retryGenerationFields[sel.Sel.Name] && (isNilIdent(node.X) || isNilIdent(node.Y)) {
								consumed[sel] = true
							}
						}
					}
				}
				return true
			})
			ast.Inspect(body, func(n ast.Node) bool {
				sel, ok := n.(*ast.SelectorExpr)
				if !ok || !retryGenerationFields[sel.Sel.Name] || consumed[sel] {
					return true
				}
				offenders = append(offenders, where(sel)+" hands out MutableState."+sel.Sel.Name+" raw (not via a clone helper or a nil comparison); an aliased pointer could be mutated outside the registered clearers (§40.18 ①)")
				return true
			})
		})
	}
	for name := range clearers {
		if !seen[name] {
			offenders = append(offenders, "registered clearer "+name+" no longer assigns a retry-local field; update the table")
		}
	}
	return offenders
}

// TestRetryGenerationSetterCensus_MutableFieldsClearedOnlyInEpilogueOrReset:
// across every file of package types the retry-local fields are assigned
// only by the registered clearers, never mutated in place, never
// address-taken, and read only through the clone helpers or a nil check.
func TestRetryGenerationSetterCensus_MutableFieldsClearedOnlyInEpilogueOrReset(t *testing.T) {
	tree := loadRetryGenerationTree(t)
	retryGenerationCheckOffenders(t, retryGenerationFieldCensus(tree, retryGenerationFieldClearers))

	sibling := func(body string) *retryGenerationTree {
		return tree.with(t, "internal/types/zz_probe.go", "package types\n\n"+body)
	}
	t.Run("self_red_sibling_file_direct_assignment", func(t *testing.T) {
		got := retryGenerationFieldCensus(sibling("func (m *MutableState) probe() { m.mu.Lock(); m.pendingAnswerDocumentPatchBase = nil; m.answerDiagramRelationRepairLease = nil; m.mu.Unlock() }"), retryGenerationFieldClearers)
		retryGenerationExpectOffender(t, got, "internal/types/zz_probe.go:MutableState.probe")
		retryGenerationExpectOffender(t, got, "assigns MutableState.pendingAnswerDocumentPatchBase")
		retryGenerationExpectOffender(t, got, "assigns MutableState.answerDiagramRelationRepairLease")
	})
	t.Run("self_red_free_function_with_pointer_parameter", func(t *testing.T) {
		got := retryGenerationFieldCensus(sibling("func probe(m *MutableState) { m.answerDiagramRelationRepairLease = nil }"), retryGenerationFieldClearers)
		retryGenerationExpectOffender(t, got, "internal/types/zz_probe.go:probe")
	})
	t.Run("self_red_address_taken", func(t *testing.T) {
		got := retryGenerationFieldCensus(sibling("func (m *MutableState) probe() { discharge(&m.answerDiagramRelationRepairLease) }"), retryGenerationFieldClearers)
		retryGenerationExpectOffender(t, got, "takes the address of MutableState.answerDiagramRelationRepairLease")
	})
	t.Run("self_red_in_place_mutation", func(t *testing.T) {
		got := retryGenerationFieldCensus(sibling("func (m *MutableState) probe() { m.answerDiagramRelationRepairLease.Failures = nil; m.pendingAnswerDocumentPatchBase.Blocks[0].ID = \"x\" }"), retryGenerationFieldClearers)
		retryGenerationExpectOffender(t, got, "assigns through MutableState.answerDiagramRelationRepairLease in place")
		retryGenerationExpectOffender(t, got, "assigns through MutableState.pendingAnswerDocumentPatchBase in place")
	})
	t.Run("self_red_raw_pointer_return", func(t *testing.T) {
		got := retryGenerationFieldCensus(sibling("func (m *MutableState) probe() *AnswerDocumentV2 { return m.pendingAnswerDocumentPatchBase }"), retryGenerationFieldClearers)
		retryGenerationExpectOffender(t, got, "hands out MutableState.pendingAnswerDocumentPatchBase raw")
	})
	t.Run("self_red_raw_pointer_argument", func(t *testing.T) {
		got := retryGenerationFieldCensus(sibling("func (m *MutableState) probe() { rewrite(m.answerDiagramRelationRepairLease) }"), retryGenerationFieldClearers)
		retryGenerationExpectOffender(t, got, "hands out MutableState.answerDiagramRelationRepairLease raw")
	})
	t.Run("self_red_registered_clearer_in_wrong_receiver", func(t *testing.T) {
		// A free function reusing a registered clearer's bare name is not the
		// registered method.
		got := retryGenerationFieldCensus(sibling("func ResetAnswerDocumentV2(m *MutableState) { m.pendingAnswerDocumentPatchBase = nil }"), retryGenerationFieldClearers)
		retryGenerationExpectOffender(t, got, "internal/types/zz_probe.go:ResetAnswerDocumentV2 (")
		retryGenerationExpectOffender(t, got, "assigns MutableState.pendingAnswerDocumentPatchBase")
	})
	t.Run("self_red_stale_clearer", func(t *testing.T) {
		clearers := map[string]bool{}
		for k, v := range retryGenerationFieldClearers {
			clearers[k] = v
		}
		clearers["MutableState.ghostClear"] = true
		got := retryGenerationFieldCensus(tree, clearers)
		retryGenerationExpectOffender(t, got, "registered clearer MutableState.ghostClear no longer assigns")
	})
}

// ---------------------------------------------------------------------------
// ③ Single base constructor (§40.17 ②)
// ---------------------------------------------------------------------------

// answerDocumentPatchBaseConstructorSites are the only functions under
// internal/ and cmd/ allowed to construct a merged answer document from a
// mutation, in any spelling: NewPartialMutation / ApplyAnswerDocumentV2Patch
// calls, an AnswerDocumentMutation composite literal, or Apply on a value
// bound to AnswerDocumentMutation.
var answerDocumentPatchBaseConstructorSites = map[retryGenerationWriterKey]bool{
	// definitions of the two mutation kinds (composite literal).
	{"internal/types", "answer_document_v2_mutation.go", "NewPartialMutation"}:    true,
	{"internal/types", "answer_document_v2_mutation.go", "NewReplaceAllMutation"}: true,
	// the mutation's own dispatch to the patch merger.
	{"internal/types", "answer_document_v2_mutation.go", "AnswerDocumentMutation.Apply"}: true,
	// the patch transaction's single base constructor (V2-1 §40.17 ②).
	{"internal/tool", "emit_answer_document_patch.go", "buildAnswerDocumentPatchBase"}: true,
	// the unified persist chokepoint re-applies the same pure mutation
	// (its ...WithFinding twin was retired with the legacy trace_finding
	// lane, §40.44 V1-5, and left this table with it).
	{"internal/tool", "answer_document_mutation_runtime.go", "ApplyAndPersistMutation"}: true,
}

var answerDocumentMutationConstructors = map[string]bool{"NewPartialMutation": true, "NewReplaceAllMutation": true}

// answerDocumentMutationCtorFuncs are the function spellings that mint a
// mutation or merge a patch. §40.45 fold-in (G-patch-txn #0) EVOLUTION
// RECORD: the census used to see them only as direct CallExpr callees, so a
// function-value alias (`f := types.ApplyAnswerDocumentV2Patch`) escaped;
// now any bare reference to these names is red everywhere.
var answerDocumentMutationCtorFuncs = map[string]bool{
	"NewPartialMutation":         true,
	"NewReplaceAllMutation":      true,
	"ApplyAnswerDocumentV2Patch": true,
}

// answerDocumentMutationFacts are the tree-wide static facts the base
// constructor census binds data flow with: every type name that denotes the
// mutation (the type itself plus defined/alias types over it, to a
// fixpoint), every struct field name that can hold one, every package-level
// var name declared as one, and every function whose results include one
// (result slot index + result count, so both `_, m, _ := f()` and
// `m := g()` bind).
type answerDocumentMutationFacts struct {
	typeNames  map[string]bool
	fieldNames map[string]bool
	pkgVars    map[string]bool
	results    map[string]answerDocumentMutationResultSlot
	// constraintNames (§40.43 round-seven #4) are the NAMED interface
	// constraints whose type set carries the mutation — `type mutC interface{
	// ~[]types.AnswerDocumentMutation }`, a scalar `~types.AnswerDocumentMutation`,
	// a union, a map/array of it, or an embedded carrying constraint — to a
	// fixpoint. A named interface is deliberately NOT a typeName (it is a
	// constraint, not a value type); a type parameter constrained by one
	// binds as a mutation carrier instead.
	constraintNames map[string]bool
}

// answerDocumentConstraintCarries (§40.43 round-seven #4) classifies a
// generic constraint expression by its TYPE SET: it reports whether the set
// carries the mutation and which elements it could not classify. Recognized
// element shapes — precise, structural: a type name denoting the mutation
// (typeNames) or a carrying named constraint (constraintNames), `~T`, unions
// `A | B`, arrays/slices, maps, pointers, and inline interfaces over those.
// Every OTHER element shape that mentions the mutation (a chan, a func, a
// generic instantiation, …) is UNRECOGNIZED: the census cannot follow the
// values it admits, so the caller fails loud on it instead of guessing.
// Methods of an inline interface are not type-set elements (the Apply-method
// lane handles them) and are skipped here.
func answerDocumentConstraintCarries(expr ast.Expr, facts *answerDocumentMutationFacts) (carries bool, unrecognized []ast.Node) {
	var term func(e ast.Expr) bool
	term = func(e ast.Expr) bool {
		switch x := e.(type) {
		case *ast.ParenExpr:
			return term(x.X)
		case *ast.UnaryExpr:
			return term(x.X)
		case *ast.BinaryExpr:
			left := term(x.X)
			right := term(x.Y)
			return left || right
		case *ast.Ident:
			return facts.typeNames[x.Name] || facts.constraintNames[x.Name]
		case *ast.SelectorExpr:
			return facts.typeNames[x.Sel.Name] || facts.constraintNames[x.Sel.Name]
		case *ast.ArrayType:
			return term(x.Elt)
		case *ast.MapType:
			key := term(x.Key)
			value := term(x.Value)
			return key || value
		case *ast.StarExpr:
			return term(x.X)
		case *ast.InterfaceType:
			found := false
			if x.Methods != nil {
				for _, field := range x.Methods.List {
					if len(field.Names) > 0 {
						continue // a method, not a type-set element
					}
					if term(field.Type) {
						found = true
					}
				}
			}
			return found
		}
		if answerDocumentMutationMentions(e, facts.typeNames) {
			unrecognized = append(unrecognized, e)
			return true
		}
		return false
	}
	carries = term(expr)
	return carries, unrecognized
}

type answerDocumentMutationResultSlot struct{ idx, count int }

// answerDocumentMutationMentions reports whether any identifier inside node
// (including a SelectorExpr's Sel) carries one of the given names.
func answerDocumentMutationMentions(node ast.Node, names map[string]bool) bool {
	found := false
	ast.Inspect(node, func(n ast.Node) bool {
		if id, ok := n.(*ast.Ident); ok && names[id.Name] {
			found = true
		}
		return !found
	})
	return found
}

// answerDocumentTypeParamConstraintDeclaresApply (§40.43 round-six #13)
// reports whether a generic type-parameter constraint declares an Apply
// method with the mutation's base-constructor signature.
func answerDocumentTypeParamConstraintDeclaresApply(expr ast.Expr, docName map[string]bool) bool {
	iface, ok := expr.(*ast.InterfaceType)
	if !ok || iface.Methods == nil {
		return false
	}
	for _, method := range iface.Methods.List {
		ft, ok := method.Type.(*ast.FuncType)
		if !ok {
			continue
		}
		for _, name := range method.Names {
			if name.Name == "Apply" && answerDocumentMutationMentions(ft, docName) {
				return true
			}
		}
	}
	return false
}

// answerDocumentFuncLitResultSlot (§40.43 round-six #15) mirrors the
// FuncDecl result-slot facts for a func literal: which result slot carries
// the mutation and how many results the literal has.
func answerDocumentFuncLitResultSlot(lit *ast.FuncLit, typeNames map[string]bool) (answerDocumentMutationResultSlot, bool) {
	if lit.Type.Results == nil {
		return answerDocumentMutationResultSlot{}, false
	}
	idx, count := 0, 0
	for _, field := range lit.Type.Results.List {
		n := len(field.Names)
		if n == 0 {
			n = 1
		}
		count += n
	}
	for _, field := range lit.Type.Results.List {
		n := len(field.Names)
		if n == 0 {
			n = 1
		}
		if answerDocumentMutationMentions(field.Type, typeNames) {
			return answerDocumentMutationResultSlot{idx: idx, count: count}, true
		}
		idx += n
	}
	return answerDocumentMutationResultSlot{}, false
}

func collectAnswerDocumentMutationFacts(tree *retryGenerationTree) *answerDocumentMutationFacts {
	facts := &answerDocumentMutationFacts{
		typeNames:       map[string]bool{"AnswerDocumentMutation": true},
		fieldNames:      map[string]bool{},
		pkgVars:         map[string]bool{},
		results:         map[string]answerDocumentMutationResultSlot{},
		constraintNames: map[string]bool{},
	}
	mentions := func(n ast.Node) bool { return answerDocumentMutationMentions(n, facts.typeNames) }
	// defined/alias types over the mutation carry the census to their names
	// (`type mut = types.AnswerDocumentMutation`, `type mut2 mut`, ...);
	// named interface constraints whose type set carries the mutation
	// (§40.43 round-seven #4) are collected as constraintNames, to the same
	// fixpoint (an embedded carrying constraint carries its embedder).
	for changed := true; changed; {
		changed = false
		for _, file := range tree.files {
			ast.Inspect(file, func(n ast.Node) bool {
				spec, ok := n.(*ast.TypeSpec)
				if !ok || facts.typeNames[spec.Name.Name] || facts.constraintNames[spec.Name.Name] {
					return true
				}
				if iface, isInterface := spec.Type.(*ast.InterfaceType); isInterface {
					if spec.TypeParams == nil {
						if carries, _ := answerDocumentConstraintCarries(iface, facts); carries {
							facts.constraintNames[spec.Name.Name] = true
							changed = true
						}
					}
				} else if mentions(spec.Type) {
					facts.typeNames[spec.Name.Name] = true
					changed = true
				}
				return true
			})
		}
	}
	for _, file := range tree.files {
		ast.Inspect(file, func(n ast.Node) bool {
			if st, ok := n.(*ast.StructType); ok && st.Fields != nil {
				for _, field := range st.Fields.List {
					if mentions(field.Type) {
						for _, name := range field.Names {
							facts.fieldNames[name.Name] = true
						}
					}
				}
			}
			return true
		})
		for _, decl := range file.Decls {
			switch d := decl.(type) {
			case *ast.GenDecl:
				for _, spec := range d.Specs {
					vs, ok := spec.(*ast.ValueSpec)
					if !ok {
						continue
					}
					holds := vs.Type != nil && mentions(vs.Type)
					for _, v := range vs.Values {
						if call, isCall := v.(*ast.CallExpr); isCall && answerDocumentMutationConstructors[selectorCallee(call)] {
							holds = true
						}
						if _, isLit := v.(*ast.CompositeLit); isLit && mentions(v) {
							holds = true
						}
					}
					if holds {
						for _, name := range vs.Names {
							facts.pkgVars[name.Name] = true
						}
					}
				}
			case *ast.FuncDecl:
				if d.Type.Results == nil {
					continue
				}
				idx, count := 0, 0
				for _, field := range d.Type.Results.List {
					n := len(field.Names)
					if n == 0 {
						n = 1
					}
					count += n
				}
				for _, field := range d.Type.Results.List {
					n := len(field.Names)
					if n == 0 {
						n = 1
					}
					if mentions(field.Type) {
						facts.results[d.Name.Name] = answerDocumentMutationResultSlot{idx: idx, count: count}
					}
					idx += n
				}
			}
		}
	}
	return facts
}

// answerDocumentPatchBaseConstructorCensus is total over the ALIASED
// spellings too (§40.45 fold-in, G-patch-txn #0). EVOLUTION RECORD: v1
// matched only direct callee names and syntactically bound Ident receivers,
// so function values, method values, method expressions, interface lanes,
// alias/defined types, container elements, struct fields, conversions,
// copies, range frames and package-level vars all escaped (confirmed by
// overlay probes); v2 binds identifiers by data flow to a fixpoint and reds
// every reference shape, each pinned by its own self-red subtest.
func answerDocumentPatchBaseConstructorCensus(tree *retryGenerationTree, sites map[retryGenerationWriterKey]bool) (offenders []string) {
	seen := map[retryGenerationWriterKey]bool{}
	facts := collectAnswerDocumentMutationFacts(tree)
	mentions := func(n ast.Node) bool { return answerDocumentMutationMentions(n, facts.typeNames) }
	docName := map[string]bool{"AnswerDocumentV2": true}
	for path, file := range tree.files {
		pkg, base := retryGenerationPkgDir(path), filepath.Base(path)
		scan := func(key retryGenerationWriterKey, fn *ast.FuncDecl, root ast.Node) {
			report := func(n ast.Node, what string) {
				if sites[key] {
					seen[key] = true
					return
				}
				offenders = append(offenders, fmt.Sprintf("%s:%s (%s) %s outside the registered base constructor sites (§40.17 ②: stage / commit / rollback share one base constructor)", path, key.fn, tree.pos(n), what))
			}
			// identifiers bound to an AnswerDocumentMutation (or a container
			// of them) by receiver, parameter, package-level var, or any
			// assignment / declaration / range frame, to a fixpoint.
			bound := map[string]bool{}
			for name := range facts.pkgVars {
				bound[name] = true
			}
			// §40.43 round-six #13: interface lanes living in the SIGNATURE
			// — a generic type-parameter's inline constraint or an anonymous
			// interface parameter type — are choked at the declaration
			// exactly like a named interface: fn.Type covers TypeParams,
			// Params and Results, none of which the body-rooted scan saw.
			reportSignatureInterfaces := func(root ast.Node) {
				ast.Inspect(root, func(n ast.Node) bool {
					iface, ok := n.(*ast.InterfaceType)
					if !ok || iface.Methods == nil {
						return true
					}
					for _, method := range iface.Methods.List {
						ft, ok := method.Type.(*ast.FuncType)
						if !ok {
							continue
						}
						for _, name := range method.Names {
							if name.Name == "Apply" && answerDocumentMutationMentions(ft, docName) {
								report(method, "declares an Apply method with the mutation's base-constructor signature (an interface lane could launder base construction)")
							}
						}
					}
					return true
				})
			}
			mutationTypeParams := map[string]bool{}
			if fn != nil {
				reportSignatureInterfaces(fn.Type)
				if fn.Type.TypeParams != nil {
					for _, tp := range fn.Type.TypeParams.List {
						// §40.43 round-seven #4: the constraint is classified by
						// its TYPE SET — a named constraint (`[M mutC]`), an
						// inline interface, `~T`, a union, arrays/maps of the
						// mutation — and its type parameters bind as mutation
						// carriers, so Apply through them is a constructor use
						// (offender unless at a registered site). EVOLUTION
						// RECORD: round six recognized only the INLINE interface
						// (Apply-method lane) and failed loud on every other
						// carrying shape; a NAMED constraint — the idiomatic
						// reusable spelling — was neither (the ident is not a
						// typeName), so `ms[0].Apply` / `range ms` through it
						// went unreported (confirmed by overlay probe).
						setCarries, unrecognized := answerDocumentConstraintCarries(tp.Type, facts)
						declaresApply := answerDocumentTypeParamConstraintDeclaresApply(tp.Type, docName)
						if !setCarries && !declaresApply && !mentions(tp.Type) {
							continue
						}
						for _, n := range unrecognized {
							report(n, "uses an unrecognized generic constraint element shape carrying the mutation; the census cannot follow the values it admits — spell the type set with the mutation type, ~T, a union, or a slice/array/map of it, or drop the generic lane")
						}
						if !setCarries && !declaresApply {
							// Mentions the mutation but no recognized type-set
							// element carries it (a generic instantiation, …):
							// FAIL LOUD instead of guessing.
							report(tp, "uses an unrecognized generic constraint shape carrying the mutation; the census cannot classify its instantiations — spell the constraint as an inline interface (choked at the declaration) or drop the generic lane")
						}
						for _, name := range tp.Names {
							mutationTypeParams[name.Name] = true
						}
					}
				}
				fields := append([]*ast.Field{}, fn.Type.Params.List...)
				if fn.Recv != nil {
					fields = append(fields, fn.Recv.List...)
				}
				for _, field := range fields {
					if mentions(field.Type) || answerDocumentMutationMentions(field.Type, mutationTypeParams) {
						for _, name := range field.Names {
							bound[name.Name] = true
						}
					}
				}
			}
			// §40.43 round-six #15: func-literal identity/passthrough lanes —
			// a call through a func-literal-valued variable whose literal
			// RETURNS the mutation type yields the mutation; collect the
			// literals so yields and the multi-value binder can follow them.
			funcVals := map[string]*ast.FuncLit{}
			ast.Inspect(root, func(n ast.Node) bool {
				switch node := n.(type) {
				case *ast.AssignStmt:
					if len(node.Lhs) == len(node.Rhs) {
						for i, rhs := range node.Rhs {
							if lit, ok := rhs.(*ast.FuncLit); ok {
								if id, ok := node.Lhs[i].(*ast.Ident); ok {
									funcVals[id.Name] = lit
								}
							}
						}
					}
				case *ast.ValueSpec:
					for i, v := range node.Values {
						if lit, ok := v.(*ast.FuncLit); ok && i < len(node.Names) {
							funcVals[node.Names[i].Name] = lit
						}
					}
				}
				return true
			})
			var isCtor func(expr ast.Expr) bool
			isCtor = func(expr ast.Expr) bool {
				switch x := expr.(type) {
				case *ast.CallExpr:
					return answerDocumentMutationConstructors[selectorCallee(x)]
				case *ast.CompositeLit:
					return x.Type != nil && mentions(x.Type)
				case *ast.ParenExpr:
					return isCtor(x.X)
				}
				return false
			}
			// yields: the expression's value is (or contains) a mutation.
			var yields func(expr ast.Expr) bool
			yields = func(expr ast.Expr) bool {
				switch x := expr.(type) {
				case *ast.Ident:
					return bound[x.Name]
				case *ast.ParenExpr:
					return yields(x.X)
				case *ast.StarExpr:
					return yields(x.X)
				case *ast.UnaryExpr:
					return yields(x.X)
				case *ast.IndexExpr:
					return yields(x.X)
				case *ast.SliceExpr:
					return yields(x.X)
				case *ast.SelectorExpr:
					return facts.fieldNames[x.Sel.Name]
				case *ast.CallExpr:
					if isCtor(x) {
						return true
					}
					if slot, ok := facts.results[selectorCallee(x)]; ok && slot.count == 1 {
						return true
					}
					// §40.43 round-six #15: a call through a func-literal-
					// valued variable whose literal returns the mutation.
					if id, ok := x.Fun.(*ast.Ident); ok {
						if lit := funcVals[id.Name]; lit != nil {
							if slot, ok := answerDocumentFuncLitResultSlot(lit, facts.typeNames); ok && slot.count == 1 {
								return true
							}
						}
					}
					return mentions(x.Fun) // conversion: AnswerDocumentMutation(v)
				case *ast.CompositeLit:
					return x.Type != nil && mentions(x.Type)
				case *ast.TypeAssertExpr:
					return x.Type != nil && mentions(x.Type)
				}
				return false
			}
			for changed := true; changed; {
				changed = false
				bind := func(e ast.Expr) {
					if id, ok := e.(*ast.Ident); ok && id.Name != "_" && !bound[id.Name] {
						bound[id.Name] = true
						changed = true
					}
				}
				ast.Inspect(root, func(n ast.Node) bool {
					switch node := n.(type) {
					case *ast.AssignStmt:
						if len(node.Rhs) == 1 && len(node.Lhs) > 1 {
							if call, ok := node.Rhs[0].(*ast.CallExpr); ok {
								if slot, ok := facts.results[selectorCallee(call)]; ok && slot.idx < len(node.Lhs) {
									bind(node.Lhs[slot.idx])
								}
								if id, ok := call.Fun.(*ast.Ident); ok {
									if lit := funcVals[id.Name]; lit != nil {
										if slot, ok := answerDocumentFuncLitResultSlot(lit, facts.typeNames); ok && slot.idx < len(node.Lhs) {
											bind(node.Lhs[slot.idx])
										}
									}
								}
							}
						}
						if len(node.Rhs) == len(node.Lhs) {
							for i, rhs := range node.Rhs {
								if yields(rhs) {
									bind(node.Lhs[i])
								}
							}
						}
					case *ast.ValueSpec:
						if node.Type != nil && mentions(node.Type) {
							for _, name := range node.Names {
								bind(name)
							}
						}
						for i, v := range node.Values {
							if i < len(node.Names) && yields(v) {
								bind(node.Names[i])
							}
						}
					case *ast.RangeStmt:
						if yields(node.X) {
							bind(node.Key)
							bind(node.Value)
						}
					}
					return true
				})
			}
			// direct-call classification: which Apply selectors are called,
			// which ctor-func identifiers are call callees.
			applyCalls := map[*ast.SelectorExpr]bool{}
			calleeIdents := map[*ast.Ident]bool{}
			ast.Inspect(root, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				switch f := call.Fun.(type) {
				case *ast.SelectorExpr:
					calleeIdents[f.Sel] = true
					if f.Sel.Name == "Apply" {
						applyCalls[f] = true
					}
				case *ast.Ident:
					calleeIdents[f] = true
				}
				return true
			})
			ast.Inspect(root, func(n ast.Node) bool {
				switch node := n.(type) {
				case *ast.CallExpr:
					switch callee := selectorCallee(node); callee {
					case "NewPartialMutation", "ApplyAnswerDocumentV2Patch":
						report(node, "calls "+callee)
					}
				case *ast.Ident:
					if answerDocumentMutationCtorFuncs[node.Name] && !calleeIdents[node] {
						report(node, "references "+node.Name+" as a function value")
					}
				case *ast.SelectorExpr:
					if node.Sel.Name != "Apply" {
						return true
					}
					if !yields(node.X) && !mentions(node.X) {
						return true
					}
					switch {
					case !applyCalls[node]:
						report(node, "aliases Apply on a mutation-typed value as a method value/expression")
					case isCtor(node.X):
						report(node, "applies a freshly constructed mutation")
					default:
						if id, ok := node.X.(*ast.Ident); ok && bound[id.Name] {
							report(node, "applies the mutation "+id.Name)
						} else {
							report(node, "applies a mutation-typed expression")
						}
					}
				case *ast.CompositeLit:
					if node.Type != nil && mentions(node.Type) {
						report(node, "builds an AnswerDocumentMutation literal")
					}
				case *ast.TypeSpec:
					// §40.43 round-seven #4: a named constraint's type set is
					// classified at the declaration — an element shape the
					// census cannot follow fails loud; and a GENERIC TYPE whose
					// constraint carries the mutation is unrecognized as a
					// whole (its fields and methods would carry the mutation
					// under a type-parameter name the per-function binder
					// never sees): fail loud.
					if iface, ok := node.Type.(*ast.InterfaceType); ok {
						if _, unrecognized := answerDocumentConstraintCarries(iface, facts); len(unrecognized) > 0 {
							for _, n := range unrecognized {
								report(n, "declares a generic constraint with an unrecognized type-set element shape carrying the mutation; the census cannot follow the values it admits")
							}
						}
					}
					if node.TypeParams != nil {
						for _, tp := range node.TypeParams.List {
							setCarries, _ := answerDocumentConstraintCarries(tp.Type, facts)
							if setCarries || mentions(tp.Type) || answerDocumentTypeParamConstraintDeclaresApply(tp.Type, docName) {
								report(tp, "declares a generic type whose constraint carries the mutation; the census cannot follow its fields or methods — spell the carrier as a concrete type or drop the generic lane")
							}
						}
					}
				case *ast.InterfaceType:
					if node.Methods == nil {
						return true
					}
					for _, method := range node.Methods.List {
						ft, ok := method.Type.(*ast.FuncType)
						if !ok {
							continue
						}
						for _, name := range method.Names {
							if name.Name == "Apply" && answerDocumentMutationMentions(ft, docName) {
								report(method, "declares an Apply method with the mutation's base-constructor signature (an interface lane could launder base construction)")
							}
						}
					}
				}
				return true
			})
		}
		for _, decl := range file.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok {
				if fn.Body != nil {
					scan(retryGenerationWriterKey{pkg, base, retryGenerationFuncName(fn)}, fn, fn.Body)
				}
				continue
			}
			scan(retryGenerationWriterKey{pkg, base, "<package scope>"}, nil, decl)
		}
	}
	for key := range sites {
		if !seen[key] {
			offenders = append(offenders, fmt.Sprintf("registered base constructor site %s/%s:%s does not exist or no longer constructs a base; update the table", key.pkg, key.file, key.fn))
		}
	}
	return offenders
}

// TestAnswerDocumentPatchBaseCensus_SingleBaseConstructor (§40.17 ②): under
// internal/ and cmd/, every spelling of "merge a patch onto prev" —
// NewPartialMutation, ApplyAnswerDocumentV2Patch, an AnswerDocumentMutation
// literal, Apply on a mutation value — sits in a registered site, so stage /
// commit / rollback share buildAnswerDocumentPatchBase.
func TestAnswerDocumentPatchBaseCensus_SingleBaseConstructor(t *testing.T) {
	tree := loadRetryGenerationTree(t)
	retryGenerationCheckOffenders(t, answerDocumentPatchBaseConstructorCensus(tree, answerDocumentPatchBaseConstructorSites))

	const patchFile = "internal/tool/emit_answer_document_patch.go"
	const orphanSite = "if staged, _, applyErr := buildAnswerDocumentPatchBase(prev, patch); applyErr == nil && staged != nil {"
	t.Run("self_red_direct_patch_merge", func(t *testing.T) {
		got := answerDocumentPatchBaseConstructorCensus(tree.withInjected(t, patchFile, orphanSite,
			"if staged, applyErr := types.ApplyAnswerDocumentV2Patch(prev, patch); applyErr == nil && staged != nil {"), answerDocumentPatchBaseConstructorSites)
		retryGenerationExpectOffender(t, got, "EmitAnswerDocumentPatch.Execute")
		retryGenerationExpectOffender(t, got, "calls ApplyAnswerDocumentV2Patch")
	})
	t.Run("self_red_composite_literal_apply", func(t *testing.T) {
		got := answerDocumentPatchBaseConstructorCensus(tree.withInjected(t, patchFile, orphanSite,
			"if staged, applyErr := (types.AnswerDocumentMutation{Kind: types.MutationPartial, Patch: patch}).Apply(prev); applyErr == nil && staged != nil {"), answerDocumentPatchBaseConstructorSites)
		retryGenerationExpectOffender(t, got, "builds an AnswerDocumentMutation literal")
		retryGenerationExpectOffender(t, got, "applies a freshly constructed mutation")
	})
	t.Run("self_red_chained_constructor_apply", func(t *testing.T) {
		got := answerDocumentPatchBaseConstructorCensus(tree.withInjected(t, patchFile, orphanSite,
			"if staged, applyErr := types.NewPartialMutation(patch).Apply(prev); applyErr == nil && staged != nil {"), answerDocumentPatchBaseConstructorSites)
		retryGenerationExpectOffender(t, got, "calls NewPartialMutation")
		retryGenerationExpectOffender(t, got, "applies a freshly constructed mutation")
	})
	t.Run("self_red_bound_mutation_reapplied", func(t *testing.T) {
		// The registered constructor's returned mutation is applied again at
		// an unregistered site → a second base.
		got := answerDocumentPatchBaseConstructorCensus(tree.withInjected(t, patchFile,
			"merged, mutation, applyErr := buildAnswerDocumentPatchBase(prev, patch)\n",
			"merged, mutation, applyErr := buildAnswerDocumentPatchBase(prev, patch)\n\tsecond, _ := mutation.Apply(prev)\n\t_ = second\n"), answerDocumentPatchBaseConstructorSites)
		retryGenerationExpectOffender(t, got, "applies the mutation mutation")
	})
	t.Run("self_red_mutation_parameter_applied_elsewhere", func(t *testing.T) {
		got := answerDocumentPatchBaseConstructorCensus(tree.with(t, "internal/orchestrator/zz_probe.go",
			"package orchestrator\n\nimport \"github.com/hanchaoqun/codrax/internal/types\"\n\nfunc probe(prev *types.AnswerDocumentV2, mutation types.AnswerDocumentMutation) { _, _ = mutation.Apply(prev) }"), answerDocumentPatchBaseConstructorSites)
		retryGenerationExpectOffender(t, got, "internal/orchestrator/zz_probe.go:probe")
	})
	t.Run("self_red_var_declared_mutation_applied", func(t *testing.T) {
		got := answerDocumentPatchBaseConstructorCensus(tree.with(t, "internal/agent/zz_probe.go",
			"package agent\n\nimport \"github.com/hanchaoqun/codrax/internal/types\"\n\nfunc probe(prev *types.AnswerDocumentV2) { var m types.AnswerDocumentMutation; _, _ = m.Apply(prev) }"), answerDocumentPatchBaseConstructorSites)
		retryGenerationExpectOffender(t, got, "internal/agent/zz_probe.go:probe")
	})
	t.Run("self_red_stale_site", func(t *testing.T) {
		sites := map[retryGenerationWriterKey]bool{}
		for k, v := range answerDocumentPatchBaseConstructorSites {
			sites[k] = v
		}
		sites[retryGenerationWriterKey{"internal/tool", "ghost.go", "ghost"}] = true
		got := answerDocumentPatchBaseConstructorCensus(tree, sites)
		retryGenerationExpectOffender(t, got, "registered base constructor site internal/tool/ghost.go:ghost does not exist")
	})

	// §40.45 fold-in (G-patch-txn #0): the census is total over ALIASED
	// spellings too — function values, method values, method expressions,
	// alias/defined types, interface lanes, container elements, struct
	// fields, conversions, copies, range frames, package-level vars.
	probe := func(body string) *retryGenerationTree {
		return tree.with(t, "internal/orchestrator/zz_probe.go",
			"package orchestrator\n\nimport \"github.com/hanchaoqun/codrax/internal/types\"\n\n"+body)
	}
	expectProbe := func(t *testing.T, mutated *retryGenerationTree, wants ...string) {
		t.Helper()
		got := answerDocumentPatchBaseConstructorCensus(mutated, answerDocumentPatchBaseConstructorSites)
		for _, want := range wants {
			retryGenerationExpectOffender(t, got, want)
		}
	}
	t.Run("self_red_merge_function_value_alias", func(t *testing.T) {
		expectProbe(t, probe("func probe(prev *types.AnswerDocumentV2, patch *types.AnswerDocumentV2Patch) { f := types.ApplyAnswerDocumentV2Patch; _, _ = f(prev, patch) }"),
			"internal/orchestrator/zz_probe.go:probe", "references ApplyAnswerDocumentV2Patch as a function value")
	})
	t.Run("self_red_constructor_function_value_alias", func(t *testing.T) {
		expectProbe(t, probe("func probe(prev *types.AnswerDocumentV2, patch *types.AnswerDocumentV2Patch) { ctor := types.NewPartialMutation; m := ctor(patch); _, _ = m.Apply(prev) }"),
			"references NewPartialMutation as a function value")
	})
	t.Run("self_red_package_level_function_alias", func(t *testing.T) {
		expectProbe(t, probe("var evadeMerge = types.ApplyAnswerDocumentV2Patch"),
			"internal/orchestrator/zz_probe.go:<package scope>", "references ApplyAnswerDocumentV2Patch as a function value")
	})
	t.Run("self_red_method_value_alias", func(t *testing.T) {
		expectProbe(t, probe("func probe(prev *types.AnswerDocumentV2, m types.AnswerDocumentMutation) { g := m.Apply; _, _ = g(prev) }"),
			"aliases Apply on a mutation-typed value")
	})
	t.Run("self_red_method_expression", func(t *testing.T) {
		expectProbe(t, probe("func probe(prev *types.AnswerDocumentV2, m types.AnswerDocumentMutation) { _, _ = types.AnswerDocumentMutation.Apply(m, prev) }"),
			"applies a mutation-typed expression")
	})
	t.Run("self_red_interface_declaring_apply", func(t *testing.T) {
		expectProbe(t, probe("type applier interface {\n\tApply(prev *types.AnswerDocumentV2) (*types.AnswerDocumentV2, error)\n}\n\nfunc probe(prev *types.AnswerDocumentV2, a applier) { _, _ = a.Apply(prev) }"),
			"declares an Apply method with the mutation's base-constructor signature")
	})
	t.Run("self_red_interface_var_holding_mutation", func(t *testing.T) {
		expectProbe(t, probe("func probe(prev *types.AnswerDocumentV2, m types.AnswerDocumentMutation) {\n\tvar a interface {\n\t\tApply(prev *types.AnswerDocumentV2) (*types.AnswerDocumentV2, error)\n\t} = m\n\t_, _ = a.Apply(prev)\n}"),
			"applies the mutation a")
	})
	t.Run("self_red_slice_element_apply", func(t *testing.T) {
		expectProbe(t, probe("func probe(prev *types.AnswerDocumentV2, ms []types.AnswerDocumentMutation) { _, _ = ms[0].Apply(prev) }"),
			"applies a mutation-typed expression")
	})
	t.Run("self_red_range_element_apply", func(t *testing.T) {
		expectProbe(t, probe("func probe(prev *types.AnswerDocumentV2, ms []types.AnswerDocumentMutation) {\n\tfor _, m := range ms {\n\t\t_, _ = m.Apply(prev)\n\t}\n}"),
			"applies the mutation m")
	})
	t.Run("self_red_type_alias_apply", func(t *testing.T) {
		expectProbe(t, probe("type mut = types.AnswerDocumentMutation\n\nfunc probe(prev *types.AnswerDocumentV2, m mut) { _, _ = m.Apply(prev) }"),
			"applies the mutation m")
	})
	t.Run("self_red_defined_type_conversion_apply", func(t *testing.T) {
		expectProbe(t, probe("type mut types.AnswerDocumentMutation\n\nfunc probe(prev *types.AnswerDocumentV2, m mut) { _, _ = types.AnswerDocumentMutation(m).Apply(prev) }"),
			"applies a mutation-typed expression")
	})
	t.Run("self_red_struct_field_apply", func(t *testing.T) {
		expectProbe(t, probe("type holder struct{ M types.AnswerDocumentMutation }\n\nfunc probe(prev *types.AnswerDocumentV2, h holder) { _, _ = h.M.Apply(prev) }"),
			"applies a mutation-typed expression")
	})
	t.Run("self_red_copy_propagation_apply", func(t *testing.T) {
		expectProbe(t, probe("func probe(prev *types.AnswerDocumentV2, m types.AnswerDocumentMutation) { a := m; _, _ = a.Apply(prev) }"),
			"applies the mutation a")
	})
	t.Run("self_red_single_result_binding_apply", func(t *testing.T) {
		expectProbe(t, probe("func mint(patch *types.AnswerDocumentV2Patch) types.AnswerDocumentMutation {\n\tvar m types.AnswerDocumentMutation\n\t_ = patch\n\treturn m\n}\n\nfunc probe(prev *types.AnswerDocumentV2, patch *types.AnswerDocumentV2Patch) { m := mint(patch); _, _ = m.Apply(prev) }"),
			"internal/orchestrator/zz_probe.go:probe", "applies the mutation m")
	})
	t.Run("self_red_package_level_mutation_var_apply", func(t *testing.T) {
		expectProbe(t, probe("var boot types.AnswerDocumentMutation\n\nfunc probe(prev *types.AnswerDocumentV2) { _, _ = boot.Apply(prev) }"),
			"applies the mutation boot")
	})
	t.Run("self_red_composite_literal_via_alias", func(t *testing.T) {
		expectProbe(t, probe("type mut = types.AnswerDocumentMutation\n\nfunc probe(prev *types.AnswerDocumentV2, patch *types.AnswerDocumentV2Patch) { _, _ = (mut{Kind: types.MutationPartial, Patch: patch}).Apply(prev) }"),
			"builds an AnswerDocumentMutation literal", "applies a freshly constructed mutation")
	})

	// §40.43 round-six #13: interface lanes living in the SIGNATURE — a
	// generic inline constraint and an anonymous interface parameter — used
	// to escape because scan() rooted at fn.Body and never saw fn.Type.
	t.Run("self_red_generic_inline_constraint_apply", func(t *testing.T) {
		expectProbe(t, probe("func launder[M interface {\n\tApply(prev *types.AnswerDocumentV2) (*types.AnswerDocumentV2, error)\n}](prev *types.AnswerDocumentV2, m M) (*types.AnswerDocumentV2, error) {\n\treturn m.Apply(prev)\n}"),
			"declares an Apply method with the mutation's base-constructor signature")
	})
	t.Run("self_red_generic_tilde_constraint_apply", func(t *testing.T) {
		// EVOLUTION RECORD (§40.43 round-seven #4): this subtest was named
		// "self_red_generic_named_constraint_carrying_mutation" and pinned
		// the fail-loud arm on this shape — which is not a named constraint
		// at all but the implicit-interface shorthand `~[]T`. The type set is
		// now classified, the parameter binds, and the Apply is the offense.
		expectProbe(t, probe("func launder[M ~[]types.AnswerDocumentMutation](prev *types.AnswerDocumentV2, ms M) { _, _ = ms[0].Apply(prev) }"),
			"applies a mutation-typed expression")
	})
	// §40.43 round-seven #4: NAMED interface constraints whose type set
	// carries the mutation used to evade the census entirely (excluded from
	// typeNames, and the InterfaceType arm reds only an Apply METHOD, not an
	// embedded `~[]mutation` element) — 0 offenders on 79ca2f98b for each of
	// the carrying shapes below (overlay probe).
	t.Run("self_red_generic_named_slice_constraint_index_apply", func(t *testing.T) {
		expectProbe(t, probe("type mutC interface{ ~[]types.AnswerDocumentMutation }\n\nfunc launder[M mutC](prev *types.AnswerDocumentV2, ms M) { _, _ = ms[0].Apply(prev) }"),
			"internal/orchestrator/zz_probe.go:launder", "applies a mutation-typed expression")
	})
	t.Run("self_red_generic_named_slice_constraint_range_apply", func(t *testing.T) {
		expectProbe(t, probe("type mutC interface{ ~[]types.AnswerDocumentMutation }\n\nfunc applyAll[M mutC](prev *types.AnswerDocumentV2, ms M) {\n\tfor _, m := range ms {\n\t\t_, _ = m.Apply(prev)\n\t}\n}"),
			"applies the mutation m")
	})
	t.Run("self_red_generic_named_scalar_constraint_apply", func(t *testing.T) {
		expectProbe(t, probe("type mutC interface{ ~types.AnswerDocumentMutation }\n\nfunc launder[M mutC](prev *types.AnswerDocumentV2, m M) { _, _ = m.Apply(prev) }"),
			"applies the mutation m")
	})
	t.Run("self_red_generic_union_constraint_apply", func(t *testing.T) {
		expectProbe(t, probe("type mutC interface{ ~[]types.AnswerDocumentMutation | ~[]int }\n\nfunc launder[M mutC](prev *types.AnswerDocumentV2, ms M) { _, _ = ms[0].Apply(prev) }"),
			"applies a mutation-typed expression")
	})
	t.Run("self_red_generic_nested_constraint_apply", func(t *testing.T) {
		expectProbe(t, probe("type inner interface{ ~[]types.AnswerDocumentMutation }\n\ntype outer interface{ inner }\n\nfunc launder[M outer](prev *types.AnswerDocumentV2, ms M) { _, _ = ms[0].Apply(prev) }"),
			"applies a mutation-typed expression")
	})
	t.Run("self_red_generic_named_map_constraint_range_apply", func(t *testing.T) {
		expectProbe(t, probe("type mutC interface{ ~map[string]types.AnswerDocumentMutation }\n\nfunc applyAll[M mutC](prev *types.AnswerDocumentV2, ms M) {\n\tfor _, m := range ms {\n\t\t_, _ = m.Apply(prev)\n\t}\n}"),
			"applies the mutation m")
	})
	t.Run("self_red_generic_named_constraint_slice_parameter_apply", func(t *testing.T) {
		expectProbe(t, probe("type mutC interface{ ~types.AnswerDocumentMutation }\n\nfunc applyAll[M mutC](prev *types.AnswerDocumentV2, ms []M) {\n\tfor _, m := range ms {\n\t\t_, _ = m.Apply(prev)\n\t}\n}"),
			"applies the mutation m")
	})
	t.Run("self_red_generic_unrecognized_constraint_element_fail_loud", func(t *testing.T) {
		expectProbe(t, probe("type mutC interface{ ~chan types.AnswerDocumentMutation }\n\nfunc drain[M mutC](ms M) { _ = ms }"),
			"unrecognized type-set element shape carrying the mutation")
	})
	t.Run("self_red_generic_type_declaration_carrying_mutation_fail_loud", func(t *testing.T) {
		expectProbe(t, probe("type mutC interface{ ~[]types.AnswerDocumentMutation }\n\ntype box[M mutC] struct{ ms M }"),
			"declares a generic type whose constraint carries the mutation")
	})
	t.Run("self_red_generic_instantiated_constraint_fail_loud", func(t *testing.T) {
		// A generic-constraint instantiation is an IndexExpr element the
		// type-set classifier cannot follow: fail loud at the element.
		expectProbe(t, probe("type carrier[T any] interface{ ~[]T }\n\nfunc launder[M carrier[types.AnswerDocumentMutation]](prev *types.AnswerDocumentV2, ms M) { _, _ = ms[0].Apply(prev) }"),
			"unrecognized generic constraint element shape carrying the mutation")
	})
	t.Run("self_green_generic_constraint_without_the_mutation", func(t *testing.T) {
		// Idiomatic generics that do not carry the mutation are not flagged.
		got := answerDocumentPatchBaseConstructorCensus(probe("type numbers interface{ ~int | ~int64 }\n\nfunc sum[N numbers](ns []N) N {\n\tvar total N\n\tfor _, n := range ns {\n\t\ttotal += n\n\t}\n\treturn total\n}\n\ntype ring[T comparable] struct{ items []T }"), answerDocumentPatchBaseConstructorSites)
		for _, o := range got {
			if strings.Contains(o, "zz_probe.go") {
				t.Fatalf("a mutation-free generic must not be reported: %s", o)
			}
		}
	})
	t.Run("self_red_anonymous_interface_parameter_apply", func(t *testing.T) {
		expectProbe(t, probe("func launder(prev *types.AnswerDocumentV2, a interface {\n\tApply(prev *types.AnswerDocumentV2) (*types.AnswerDocumentV2, error)\n}) { _, _ = a.Apply(prev) }"),
			"declares an Apply method with the mutation's base-constructor signature")
	})
	// §40.43 round-six #15: a func-literal identity call used to unbind the
	// mutation (facts.results covers FuncDecls only).
	t.Run("self_red_func_literal_identity_apply", func(t *testing.T) {
		expectProbe(t, probe("func probe(prev *types.AnswerDocumentV2, m types.AnswerDocumentMutation) {\n\tid := func(mm types.AnswerDocumentMutation) types.AnswerDocumentMutation { return mm }\n\ta := id(m)\n\t_, _ = a.Apply(prev)\n}"),
			"applies the mutation a")
	})
	t.Run("self_red_func_literal_multi_result_apply", func(t *testing.T) {
		expectProbe(t, probe("func probe(prev *types.AnswerDocumentV2, m types.AnswerDocumentMutation) {\n\tid := func(mm types.AnswerDocumentMutation) (types.AnswerDocumentMutation, bool) { return mm, true }\n\ta, _ := id(m)\n\t_, _ = a.Apply(prev)\n}"),
			"applies the mutation a")
	})
}

// ---------------------------------------------------------------------------
// ④ Normalization order + staged_for_retry flag (§40.17 ①③ / §40.18 ③)
// ---------------------------------------------------------------------------

const (
	answerDocumentPatchToolFile    = "internal/tool/emit_answer_document_patch.go"
	answerDocumentPatchExecuteName = "EmitAnswerDocumentPatch.Execute"
	stagedByThisCallIdent          = "stagedByThisCall"
	stagingEntryPoint              = "stageAnswerDocumentPatchGeneration"
	failureOutcomeAnnotator        = "annotateAnswerDocumentPatchFailureOutcome"
)

func answerDocumentPatchExecute(t *testing.T, tree *retryGenerationTree) *ast.FuncDecl {
	t.Helper()
	file := tree.files[answerDocumentPatchToolFile]
	if file == nil {
		t.Fatalf("%s not found", answerDocumentPatchToolFile)
	}
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if ok && retryGenerationFuncName(fn) == answerDocumentPatchExecuteName {
			return fn
		}
	}
	t.Fatalf("%s not found", answerDocumentPatchExecuteName)
	return nil
}

// stagedForRetryFlagCensus pins that stagedByThisCall is true exactly when a
// generation was staged: inside Execute the flag is declared once, its
// address is passed only as the fourth argument of stageAnswerDocumentPatchGeneration
// (every staging call passes it — the helper is nil-tolerant, so a nil there
// would stage silently), it is read only by the failure-outcome annotator,
// and no other function in the tree references the staging entry point in
// any spelling (call, function value, wrapper).
func stagedForRetryFlagCensus(tree *retryGenerationTree, execute *ast.FuncDecl) (offenders []string) {
	where := func(n ast.Node) string {
		return fmt.Sprintf("%s:%s (%s)", answerDocumentPatchToolFile, answerDocumentPatchExecuteName, tree.pos(n))
	}
	isFlag := func(e ast.Expr) bool {
		id, ok := e.(*ast.Ident)
		return ok && id.Name == stagedByThisCallIdent
	}
	consumed := map[ast.Node]bool{}
	declarations, stagingCalls, annotatorReads := 0, 0, 0
	ast.Inspect(execute.Body, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.AssignStmt:
			for _, lhs := range node.Lhs {
				if !isFlag(lhs) {
					continue
				}
				consumed[lhs] = true
				if node.Tok == token.DEFINE {
					declarations++
				} else {
					offenders = append(offenders, where(lhs)+" assigns "+stagedByThisCallIdent+" directly; only "+stagingEntryPoint+" may mark a staged generation")
				}
			}
		case *ast.CallExpr:
			switch selectorCallee(node) {
			case stagingEntryPoint:
				switch f := node.Fun.(type) {
				case *ast.Ident:
					consumed[f] = true
				case *ast.SelectorExpr:
					// §40.43 round-six #6: the staging entry point is a
					// package FREE FUNCTION — a selector call spells a
					// same-named METHOD on a foreign receiver, which sets
					// the flag without staging. Bind by receiver, not by
					// name: this is an offender, never THE staging call.
					consumed[f.Sel] = true
					offenders = append(offenders, where(node)+" calls a same-named method "+stagingEntryPoint+" on a foreign receiver; the staging entry point is the package free function, called directly")
					return true
				}
				stagingCalls++
				if len(node.Args) != 4 {
					offenders = append(offenders, where(node)+" calls "+stagingEntryPoint+" with an unexpected arity")
					return true
				}
				addr, ok := node.Args[3].(*ast.UnaryExpr)
				if !ok || addr.Op != token.AND || !isFlag(addr.X) {
					offenders = append(offenders, where(node)+" stages a generation without passing &"+stagedByThisCallIdent+"; the outcome metadata would report not_staged for a staged base")
					return true
				}
				consumed[addr] = true
				consumed[addr.X] = true
			case failureOutcomeAnnotator:
				if len(node.Args) == 2 && isFlag(node.Args[1]) {
					annotatorReads++
					consumed[node.Args[1]] = true
				}
			}
		}
		return true
	})
	ast.Inspect(execute.Body, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.UnaryExpr:
			if node.Op == token.AND && isFlag(node.X) && !consumed[node] {
				consumed[node.X] = true
				offenders = append(offenders, where(node)+" lets &"+stagedByThisCallIdent+" escape to something other than "+stagingEntryPoint+"; a pointer helper could mark staged_for_retry without staging")
			}
		case *ast.Ident:
			if node.Name == stagedByThisCallIdent && !consumed[node] {
				offenders = append(offenders, where(node)+" uses "+stagedByThisCallIdent+" outside its declaration, the staging call, and the failure-outcome annotator")
			}
			// §40.45 fold-in (G-patch-txn #1) EVOLUTION RECORD: staging
			// used to be recognized only as a direct CallExpr callee, so
			// `stageAlias := stageAnswerDocumentPatchGeneration` staged a
			// base whose outcome metadata reported not_staged; now any
			// non-callee reference to the entry point is red.
			if node.Name == stagingEntryPoint && !consumed[node] {
				offenders = append(offenders, where(node)+" aliases "+stagingEntryPoint+" as a function value; every staging call must be a direct call passing &"+stagedByThisCallIdent)
			}
		}
		return true
	})
	if declarations != 1 {
		offenders = append(offenders, fmt.Sprintf("%s declares %s %d times, want exactly once", answerDocumentPatchExecuteName, stagedByThisCallIdent, declarations))
	}
	if stagingCalls == 0 {
		offenders = append(offenders, answerDocumentPatchExecuteName+" never calls "+stagingEntryPoint+"; update the census")
	}
	if annotatorReads != 1 {
		offenders = append(offenders, fmt.Sprintf("%s hands %s to %s %d times, want exactly once (the deferred outcome annotation)", answerDocumentPatchExecuteName, stagedByThisCallIdent, failureOutcomeAnnotator, annotatorReads))
	}
	// §40.43 round-six #6: the staging entry point's NAME is a census
	// subject too — a FuncDecl named stageAnswerDocumentPatchGeneration on
	// another receiver never appears inside any function body, so the
	// reference scan below cannot see it. Exactly one declaration may
	// exist, and it is the package free function.
	stagingDecls := 0
	for path, file := range tree.files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Name.Name != stagingEntryPoint {
				continue
			}
			stagingDecls++
			if fn.Recv != nil {
				offenders = append(offenders, fmt.Sprintf("%s (%s) declares a METHOD named %s on a foreign receiver — a same-named method could satisfy a name-bound staging call while staging nothing", path, tree.pos(fn), stagingEntryPoint))
			}
		}
	}
	if stagingDecls != 1 {
		offenders = append(offenders, fmt.Sprintf("%s must be declared exactly once (the package free function), found %d declarations", stagingEntryPoint, stagingDecls))
	}
	for path, file := range tree.files {
		retryGenerationDecls(file, func(fnName string, body ast.Node) {
			if path == answerDocumentPatchToolFile && fnName == answerDocumentPatchExecuteName {
				return
			}
			// Any REFERENCE to the staging entry point outside Execute —
			// direct call, function value, wrapper — is red (§40.45
			// fold-in, G-patch-txn #1): an alias would stage a base the
			// outcome metadata reports as not_staged.
			ast.Inspect(body, func(n ast.Node) bool {
				id, ok := n.(*ast.Ident)
				if ok && id.Name == stagingEntryPoint {
					offenders = append(offenders, fmt.Sprintf("%s:%s (%s) references %s; only %s stages a generation, by direct call", path, fnName, tree.pos(id), stagingEntryPoint, answerDocumentPatchExecuteName))
				}
				return true
			})
		})
	}
	return offenders
}

// TestAnswerDocumentPatchBaseCensus_NormalizationPrecedesAtomicEdits
// (§40.17 ①③): inside EmitAnswerDocumentPatch.Execute the patch-normalizer
// chain is called exactly once and its position precedes both atomic diagram
// executors.
func TestAnswerDocumentPatchBaseCensus_NormalizationPrecedesAtomicEdits(t *testing.T) {
	tree := loadRetryGenerationTree(t)
	execute := answerDocumentPatchExecute(t, tree)
	positions := map[string][]token.Pos{}
	ast.Inspect(execute.Body, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok {
			switch callee := selectorCallee(call); callee {
			case "normalizeAnswerDocumentPatchForBase",
				"applyModelAuthoredDiagramAtomicEditsWithParticipantsAndBoundaries",
				"applyModelAuthoredDiagramRelationScopeEdits":
				positions[callee] = append(positions[callee], call.Pos())
			}
		}
		return true
	})
	if got := len(positions["normalizeAnswerDocumentPatchForBase"]); got != 1 {
		t.Fatalf("normalizeAnswerDocumentPatchForBase must be called exactly once in Execute, got %d", got)
	}
	normalize := positions["normalizeAnswerDocumentPatchForBase"][0]
	for _, executor := range []string{"applyModelAuthoredDiagramAtomicEditsWithParticipantsAndBoundaries", "applyModelAuthoredDiagramRelationScopeEdits"} {
		if got := len(positions[executor]); got != 1 {
			t.Fatalf("%s must be called exactly once in Execute, got %d", executor, got)
		}
		if positions[executor][0] < normalize {
			t.Errorf("%s runs before the patch-normalizer chain; the chain is positionally aligned to the model's JSON and must precede every system-appended block", executor)
		}
	}
	names := make([]string, 0, len(positions))
	for name := range positions {
		names = append(names, name)
	}
	sort.Strings(names)
	if len(names) != 3 {
		t.Fatalf("census expected three tracked calls, saw %v", names)
	}
}

// TestAnswerDocumentPatchBaseCensus_StagedForRetryFlagIsTruthful (§40.18 ③
// / §40.45): stagedByThisCall can only become true by staging a generation.
func TestAnswerDocumentPatchBaseCensus_StagedForRetryFlagIsTruthful(t *testing.T) {
	tree := loadRetryGenerationTree(t)
	retryGenerationCheckOffenders(t, stagedForRetryFlagCensus(tree, answerDocumentPatchExecute(t, tree)))

	const decl = "stagedByThisCall := false\n"
	run := func(t *testing.T, mutated *retryGenerationTree) []string {
		t.Helper()
		return stagedForRetryFlagCensus(mutated, answerDocumentPatchExecute(t, mutated))
	}
	t.Run("self_red_pointer_helper", func(t *testing.T) {
		got := run(t, tree.withInjected(t, answerDocumentPatchToolFile, decl, decl+"\tprobeMark(&stagedByThisCall)\n"))
		retryGenerationExpectOffender(t, got, "lets &stagedByThisCall escape")
	})
	t.Run("self_red_address_stored", func(t *testing.T) {
		got := run(t, tree.withInjected(t, answerDocumentPatchToolFile, decl, decl+"\tflag := &stagedByThisCall\n\t_ = flag\n"))
		retryGenerationExpectOffender(t, got, "lets &stagedByThisCall escape")
	})
	t.Run("self_red_closure_assignment", func(t *testing.T) {
		got := run(t, tree.withInjected(t, answerDocumentPatchToolFile, decl, decl+"\tfunc() { stagedByThisCall = true }()\n"))
		retryGenerationExpectOffender(t, got, "assigns stagedByThisCall directly")
	})
	t.Run("self_red_staging_without_flag", func(t *testing.T) {
		got := run(t, tree.withInjected(t, answerDocumentPatchToolFile, ", &stagedByThisCall)", ", nil)"))
		retryGenerationExpectOffender(t, got, "stages a generation without passing &stagedByThisCall")
	})
	t.Run("self_red_staging_with_foreign_flag", func(t *testing.T) {
		got := run(t, tree.withInjected(t, answerDocumentPatchToolFile, ", &stagedByThisCall)", ", &otherFlag)"))
		retryGenerationExpectOffender(t, got, "stages a generation without passing &stagedByThisCall")
	})
	t.Run("self_red_flag_read_elsewhere", func(t *testing.T) {
		got := run(t, tree.withInjected(t, answerDocumentPatchToolFile, decl, decl+"\tlogging.Debug(\"%v\", stagedByThisCall)\n"))
		retryGenerationExpectOffender(t, got, "uses stagedByThisCall outside its declaration")
	})
	t.Run("self_red_second_staging_caller", func(t *testing.T) {
		got := run(t, tree.with(t, "internal/tool/zz_probe.go",
			"package tool\n\nimport \"github.com/hanchaoqun/codrax/internal/types\"\n\nfunc probe(mut *types.MutableState, base *types.AnswerDocumentV2) { stageAnswerDocumentPatchGeneration(mut, base, nil, nil) }"))
		retryGenerationExpectOffender(t, got, "internal/tool/zz_probe.go:probe")
	})

	// §40.45 fold-in (G-patch-txn #1): every REFERENCE to the staging entry
	// point — not only direct calls — is censused, so an aliased staging
	// path cannot stage a base while the outcome metadata reports
	// not_staged.
	t.Run("self_red_staging_alias_in_execute", func(t *testing.T) {
		got := run(t, tree.withInjected(t, answerDocumentPatchToolFile, decl,
			decl+"\tstageAlias := stageAnswerDocumentPatchGeneration\n\tstageAlias(ctx.Mutable, nil, nil, nil)\n"))
		retryGenerationExpectOffender(t, got, "aliases stageAnswerDocumentPatchGeneration as a function value")
	})
	t.Run("self_red_package_level_staging_alias", func(t *testing.T) {
		got := run(t, tree.with(t, "internal/tool/zz_probe.go",
			"package tool\n\nvar stageAlias = stageAnswerDocumentPatchGeneration"))
		retryGenerationExpectOffender(t, got, "internal/tool/zz_probe.go:<package scope>")
		retryGenerationExpectOffender(t, got, "references stageAnswerDocumentPatchGeneration")
	})
	t.Run("self_red_staging_alias_in_other_function", func(t *testing.T) {
		got := run(t, tree.with(t, "internal/tool/zz_probe.go",
			"package tool\n\nimport \"github.com/hanchaoqun/codrax/internal/types\"\n\nfunc probe(mut *types.MutableState, base *types.AnswerDocumentV2) { f := stageAnswerDocumentPatchGeneration; f(mut, base, nil, nil) }"))
		retryGenerationExpectOffender(t, got, "internal/tool/zz_probe.go:probe")
		retryGenerationExpectOffender(t, got, "references stageAnswerDocumentPatchGeneration")
	})

	// §40.43 round-six #6: a same-named METHOD on a foreign receiver used to
	// satisfy the staging census by bare callee name — the flag was consumed
	// and stagingCalls counted while nothing was staged (the exact P5
	// staged_for_retry / not_staged drift this census exists to prevent).
	t.Run("self_red_same_named_method_on_foreign_receiver", func(t *testing.T) {
		got := run(t, tree.withInjected(t, answerDocumentPatchToolFile,
			"stageAnswerDocumentPatchGeneration(ctx.Mutable, merged, nil, &stagedByThisCall)",
			"evilHelper.stageAnswerDocumentPatchGeneration(ctx.Mutable, merged, nil, &stagedByThisCall)"))
		retryGenerationExpectOffender(t, got, "same-named method stageAnswerDocumentPatchGeneration on a foreign receiver")
	})
	t.Run("self_red_same_named_method_declaration", func(t *testing.T) {
		got := run(t, tree.with(t, "internal/tool/zz_evil.go",
			"package tool\n\nimport \"github.com/hanchaoqun/codrax/internal/types\"\n\ntype evilStager struct{}\n\nfunc (evilStager) stageAnswerDocumentPatchGeneration(mut *types.MutableState, base *types.AnswerDocumentV2, lease any, flag *bool) {\n\tif flag != nil {\n\t\t*flag = true\n\t}\n}"))
		retryGenerationExpectOffender(t, got, "declares a METHOD named stageAnswerDocumentPatchGeneration on a foreign receiver")
	})
}
