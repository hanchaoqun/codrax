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
// AnswerDocumentMutation composite literal, and Apply on a mutation value);
// the staged_for_retry flag census covers address-taking and helper writes.
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

// answerDocumentMutationResultIndex maps every function in the tree whose
// results include an AnswerDocumentMutation to the index of that result, so
// `_, mutation, _ := f(...)` binds `mutation` for the Apply check.
func answerDocumentMutationResultIndex(tree *retryGenerationTree) map[string]int {
	out := map[string]int{}
	for _, file := range tree.files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Type.Results == nil {
				continue
			}
			idx := 0
			for _, field := range fn.Type.Results.List {
				n := len(field.Names)
				if n == 0 {
					n = 1
				}
				if typeName(field.Type) == "AnswerDocumentMutation" {
					out[fn.Name.Name] = idx
				}
				idx += n
			}
		}
	}
	return out
}

func answerDocumentPatchBaseConstructorCensus(tree *retryGenerationTree, sites map[retryGenerationWriterKey]bool) (offenders []string) {
	seen := map[retryGenerationWriterKey]bool{}
	resultIndex := answerDocumentMutationResultIndex(tree)
	for path, file := range tree.files {
		pkg, base := retryGenerationPkgDir(path), filepath.Base(path)
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			key := retryGenerationWriterKey{pkg, base, retryGenerationFuncName(fn)}
			report := func(n ast.Node, what string) {
				if sites[key] {
					seen[key] = true
					return
				}
				offenders = append(offenders, fmt.Sprintf("%s:%s (%s) %s outside the registered base constructor sites (§40.17 ②: stage / commit / rollback share one base constructor)", path, key.fn, tree.pos(n), what))
			}
			// identifiers bound to an AnswerDocumentMutation inside fn.
			bound := map[string]bool{}
			for _, field := range fn.Type.Params.List {
				if typeName(field.Type) == "AnswerDocumentMutation" {
					for _, name := range field.Names {
						bound[name.Name] = true
					}
				}
			}
			var isCtor func(expr ast.Expr) bool
			isCtor = func(expr ast.Expr) bool {
				switch x := expr.(type) {
				case *ast.CallExpr:
					return answerDocumentMutationConstructors[selectorCallee(x)]
				case *ast.CompositeLit:
					return typeName(x.Type) == "AnswerDocumentMutation"
				case *ast.ParenExpr:
					return isCtor(x.X)
				}
				return false
			}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				switch node := n.(type) {
				case *ast.AssignStmt:
					if len(node.Rhs) == 1 && len(node.Lhs) > 1 {
						if call, ok := node.Rhs[0].(*ast.CallExpr); ok {
							if idx, ok := resultIndex[selectorCallee(call)]; ok && idx < len(node.Lhs) {
								if id, ok := node.Lhs[idx].(*ast.Ident); ok {
									bound[id.Name] = true
								}
							}
						}
					}
					if len(node.Rhs) == len(node.Lhs) {
						for i, rhs := range node.Rhs {
							if id, ok := node.Lhs[i].(*ast.Ident); ok && isCtor(rhs) {
								bound[id.Name] = true
							}
						}
					}
				case *ast.ValueSpec:
					if typeName(node.Type) == "AnswerDocumentMutation" {
						for _, name := range node.Names {
							bound[name.Name] = true
						}
					}
					for i, v := range node.Values {
						if i < len(node.Names) && isCtor(v) {
							bound[node.Names[i].Name] = true
						}
					}
				}
				return true
			})
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				switch node := n.(type) {
				case *ast.CallExpr:
					switch callee := selectorCallee(node); callee {
					case "NewPartialMutation", "ApplyAnswerDocumentV2Patch":
						report(node, "calls "+callee)
					case "Apply":
						sel, ok := node.Fun.(*ast.SelectorExpr)
						if !ok {
							return true
						}
						switch recv := sel.X.(type) {
						case *ast.Ident:
							if bound[recv.Name] {
								report(node, "applies the mutation "+recv.Name)
							}
						default:
							if isCtor(recv) {
								report(node, "applies a freshly constructed mutation")
							}
						}
					}
				case *ast.CompositeLit:
					if typeName(node.Type) == "AnswerDocumentMutation" {
						report(node, "builds an AnswerDocumentMutation literal")
					}
				}
				return true
			})
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
// and no other function in the tree calls the staging entry point.
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
	for path, file := range tree.files {
		retryGenerationDecls(file, func(fnName string, body ast.Node) {
			if path == answerDocumentPatchToolFile && fnName == answerDocumentPatchExecuteName {
				return
			}
			ast.Inspect(body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if ok && selectorCallee(call) == stagingEntryPoint {
					offenders = append(offenders, fmt.Sprintf("%s:%s (%s) calls %s; only %s stages a generation", path, fnName, tree.pos(call), stagingEntryPoint, answerDocumentPatchExecuteName))
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
}
