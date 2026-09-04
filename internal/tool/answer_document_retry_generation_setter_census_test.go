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
// elements, struct fields, conversions, copies, range frames — and, per
// §40.45 round-eight #2–#5, classifies carriers by TYPE-SET CLOSURE rather
// than spelling: alias/defined chains over carrying constraints, generic
// Apply interfaces instantiated with the document type, carrying type
// parameters in parameter/receiver/field/result position (a generic call's
// result binds, outright or inferred from a mutation-bound argument),
// func-typed values whose result is the mutation (a call on them yields it),
// with a fail-loud default for every mutation-mentioning type expression
// none of those lanes classifies and for an Apply through the result of a
// call the census cannot classify (chained, or bound to an identifier —
// §40.45 round-nine #7); round-nine #5/#6/#8 add inference from func-typed
// and instance arguments, make/new classification, and methods/fields of
// generic types through inferred instances; the staged_for_retry flag census covers
// address-taking, helper writes, and aliased references to the staging
// entry point.
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
	// constraintNames (§40.45 round-seven #4) are the NAMED interface
	// constraints whose type set carries the mutation — `type mutC interface{
	// ~[]types.AnswerDocumentMutation }`, a scalar `~types.AnswerDocumentMutation`,
	// a union, a map/array of it, or an embedded carrying constraint — to a
	// fixpoint. A named interface is deliberately NOT a typeName (it is a
	// constraint, not a value type); a type parameter constrained by one
	// binds as a mutation carrier instead. §40.45 round-eight #2: an alias or
	// defined type over a carrying constraint (`type mutC2 = mutC`, `type
	// mutC2 mutC`) carries too — computed to the same fixpoint.
	constraintNames map[string]bool
	// §40.45 round-eight #5: func-typed carriers. A func type whose RESULT
	// slot is the mutation is not a mutation value — a CALL on it yields one.
	// funcTypeNames are the named func types (`type mkFn func() …`),
	// funcFields the struct fields, funcPkgVars the package-level vars, each
	// with the yielding result slot.
	funcTypeNames map[string]answerDocumentMutationResultSlot
	funcFields    map[string]answerDocumentMutationResultSlot
	funcPkgVars   map[string]answerDocumentMutationResultSlot
	// §40.45 round-eight #4: generic FuncDecls by name — which type params
	// are bound by a carrying constraint, which type params each parameter
	// slot mentions, and which each result slot mentions — so a call whose
	// result type is a carrier (directly, or inferred from a mutation-bound
	// argument) binds its result.
	generics map[string]*answerDocumentGenericFacts
	// §40.45 round-eight #3: generic interfaces declaring an Apply method
	// (`type applier[D any] interface{ Apply(prev *D) (*D, error) }`) by
	// name: an INSTANTIATION is classified by its method set after
	// substitution — with the document type it has the base-constructor
	// signature and carries.
	genericAppliers map[string]*answerDocumentGenericApplier
	// §40.45 round-nine #8: generic TYPES by name — their declared type
	// parameters, their struct fields' shapes in terms of those parameters,
	// and their methods' result shapes in terms of the receiver's type
	// parameters — so a value bound to an INFERRED instantiation
	// (`b := newBag(ms)`) yields through `b.first()` / `b.items` exactly as
	// the explicit spelling fails loud.
	genericTypes      map[string][]string
	genericTypeFields map[string]map[string]answerDocumentGenericResultSlot
	genericMethods    map[string]map[string]*answerDocumentGenericMethodFacts
	// funcDecls are every FuncDecl name in the tree (functions and methods):
	// a call to one that yields nothing is CLASSIFIED as non-yielding, so the
	// fail-loud on unclassifiable call receivers keys on foreign callees only.
	funcDecls map[string]bool
	// unrecognized are the type expressions that mention the mutation (or a
	// carrier name) in a shape the census cannot classify: FAIL LOUD.
	unrecognized []answerDocumentUnrecognizedShape
}

type answerDocumentUnrecognizedShape struct {
	node ast.Node
	why  string
}

// answerDocumentGenericFacts describes one generic FuncDecl.
type answerDocumentGenericFacts struct {
	carrying   map[string]bool   // type params bound by a carrying constraint
	paramSlots []map[string]bool // per argument slot: the type params its declared type mentions (variadic = last slot)
	// §40.45 round-nine #5: per argument slot whose declared type is a func
	// type, the type params each of ITS result slots mentions — a func
	// carrier argument instantiates them through its yielding result slot.
	paramFuncResults [][]map[string]bool
	// §40.45 round-nine #8: per argument slot whose declared type is an
	// instantiation of a tree generic type by this function's type params —
	// an instance argument instantiates them.
	paramInstances []*answerDocumentGenericInstanceShape
	variadic       bool
	results        []answerDocumentGenericResultSlot
}

// answerDocumentGenericResultSlot describes one result slot (or struct field)
// typed in terms of type params: whether it is the mutation outright, which
// type params it mentions, and how it classifies once those are
// instantiated with the mutation.
type answerDocumentGenericResultSlot struct {
	mutation   bool
	typeParams map[string]bool
	shape      answerDocumentGenericShape
}

// answerDocumentGenericShape (§40.45 round-nine #5/#8) is the classification
// of a type expression mentioning type params ONCE they are instantiated
// with the mutation: a plain mutation value, a func carrier (`func() T`,
// with its yielding result slot), an instantiation of a tree generic type
// by those params, or an unrecognized shape (fail loud at the call).
type answerDocumentGenericShape struct {
	kind     answerDocumentValueKind
	slot     answerDocumentMutationResultSlot
	why      string
	instance *answerDocumentGenericInstanceShape
}

// answerDocumentGenericInstanceShape is `G[A, B]` over a tree generic type
// G: per declared parameter of G (declared), the enclosing type param it is
// instantiated with (args; "" when instantiated with anything else).
type answerDocumentGenericInstanceShape struct {
	typeName string
	declared []string
	args     []string
}

// answerDocumentInstanceBinding is a VALUE bound to an instantiation of a
// tree generic type whose listed declared params are instantiated with the
// mutation (`b := newBag(ms)` → bag, {T}).
type answerDocumentInstanceBinding struct {
	typeName string
	params   map[string]bool
}

// answerDocumentYieldKind is what one result slot of a classified call
// yields.
type answerDocumentYieldKind int

const (
	answerDocumentYieldNone         answerDocumentYieldKind = iota
	answerDocumentYieldMutation                             // a mutation (or a container of them): binds as a carrier
	answerDocumentYieldFuncCarrier                          // a func carrier: a call on it yields at slot
	answerDocumentYieldInstance                             // an instance of a tree generic type: inst
	answerDocumentYieldUnclassified                         // a shape the census cannot follow (reported at the call): binds as unclassified
)

type answerDocumentYield struct {
	kind answerDocumentYieldKind
	slot answerDocumentMutationResultSlot
	inst *answerDocumentInstanceBinding
}

// answerDocumentYieldOfSlot classifies a result slot / field once the type
// params in `instantiated` are known to be the mutation.
func answerDocumentYieldOfSlot(res answerDocumentGenericResultSlot, instantiated map[string]bool) answerDocumentYield {
	if res.mutation {
		return answerDocumentYield{kind: answerDocumentYieldMutation}
	}
	hit := false
	for name := range res.typeParams {
		if instantiated[name] {
			hit = true
		}
	}
	if !hit {
		return answerDocumentYield{}
	}
	if inst := res.shape.instance; inst != nil {
		binding := &answerDocumentInstanceBinding{typeName: inst.typeName, params: map[string]bool{}}
		for j, arg := range inst.args {
			if arg != "" && instantiated[arg] && j < len(inst.declared) {
				binding.params[inst.declared[j]] = true
			}
		}
		if len(binding.params) == 0 {
			return answerDocumentYield{}
		}
		return answerDocumentYield{kind: answerDocumentYieldInstance, inst: binding}
	}
	switch res.shape.kind {
	case answerDocumentValueMutation:
		return answerDocumentYield{kind: answerDocumentYieldMutation}
	case answerDocumentValueFunc:
		return answerDocumentYield{kind: answerDocumentYieldFuncCarrier, slot: res.shape.slot}
	case answerDocumentValueUnrecognized:
		return answerDocumentYield{kind: answerDocumentYieldUnclassified}
	}
	return answerDocumentYield{}
}

// answerDocumentGenericMethodFacts describes a method on a generic type:
// the receiver's type-param names as spelled (positional against the type's
// declaration) and the result shapes in terms of them.
type answerDocumentGenericMethodFacts struct {
	recvParams []string
	results    []answerDocumentGenericResultSlot
}

// answerDocumentGenericShapeOf classifies a type expression in terms of the
// given type params (see answerDocumentGenericShape).
func answerDocumentGenericShapeOf(typ ast.Expr, typeParams map[string]bool, facts *answerDocumentMutationFacts) answerDocumentGenericShape {
	if !answerDocumentMutationMentions(typ, typeParams) {
		return answerDocumentGenericShape{kind: answerDocumentValueNone}
	}
	bare := typ
	for {
		switch x := bare.(type) {
		case *ast.ParenExpr:
			bare = x.X
			continue
		case *ast.StarExpr:
			bare = x.X
			continue
		}
		break
	}
	if name, args, ok := answerDocumentInstantiationParts(bare); ok {
		if declared := facts.genericTypes[name]; declared != nil && len(declared) == len(args) {
			inst := &answerDocumentGenericInstanceShape{typeName: name, declared: declared, args: make([]string, len(args))}
			resolvable := true
			for i, arg := range args {
				if id, isIdent := arg.(*ast.Ident); isIdent && typeParams[id.Name] {
					inst.args[i] = id.Name
				} else if answerDocumentMutationMentions(arg, typeParams) {
					resolvable = false
				}
			}
			if resolvable {
				return answerDocumentGenericShape{kind: answerDocumentValueMutation, instance: inst}
			}
		}
	}
	kind, slot, why := answerDocumentClassifyValueType(typ, facts, typeParams)
	return answerDocumentGenericShape{kind: kind, slot: slot, why: why}
}

// answerDocumentGenericSlotsOf builds the per-slot facts of a result list or
// field in terms of type params.
func answerDocumentGenericSlotOf(typ ast.Expr, typeParams map[string]bool, mutation bool, facts *answerDocumentMutationFacts) answerDocumentGenericResultSlot {
	mentioned := map[string]bool{}
	for name := range typeParams {
		if answerDocumentMutationMentions(typ, map[string]bool{name: true}) {
			mentioned[name] = true
		}
	}
	return answerDocumentGenericResultSlot{mutation: mutation, typeParams: mentioned, shape: answerDocumentGenericShapeOf(typ, typeParams, facts)}
}

// answerDocumentGenericApplier describes a generic interface declaring Apply.
type answerDocumentGenericApplier struct {
	params  []string
	applies []*ast.FuncType
}

// answerDocumentCarrierNames is every name whose mention makes a type
// expression census-relevant: mutation type names, carrying constraint
// names, func-carrier type names.
func (facts *answerDocumentMutationFacts) carrierNames() map[string]bool {
	out := map[string]bool{}
	for name := range facts.typeNames {
		out[name] = true
	}
	for name := range facts.constraintNames {
		out[name] = true
	}
	for name := range facts.funcTypeNames {
		out[name] = true
	}
	return out
}

// answerDocumentConstraintCarries (§40.45 round-seven #4) classifies a
// generic constraint expression by its TYPE SET: it reports whether the set
// carries the mutation and which elements it could not classify. Recognized
// element shapes — precise, structural: a type name denoting the mutation
// (typeNames) or a carrying named constraint (constraintNames), `~T`, unions
// `A | B`, arrays/slices, maps, pointers, and inline interfaces over those.
// Every OTHER element shape that mentions the mutation or a carrier name (a
// chan, a func, a generic instantiation, …) is UNRECOGNIZED: the census
// cannot follow the values it admits, so the caller fails loud on it instead
// of guessing. Methods of an inline interface are not type-set elements (the
// Apply-method lane handles them) and are skipped here.
func answerDocumentConstraintCarries(expr ast.Expr, facts *answerDocumentMutationFacts) (carries bool, unrecognized []ast.Node) {
	names := facts.carrierNames()
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
		if answerDocumentMutationMentions(e, names) {
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

// answerDocumentValueKind classifies a VALUE type expression (a parameter,
// receiver, field, var, result or TypeSpec type) that mentions the mutation.
type answerDocumentValueKind int

const (
	answerDocumentValueNone         answerDocumentValueKind = iota // mentions nothing census-relevant
	answerDocumentValueMutation                                    // a mutation, or a pointer/array/slice/map/struct holding one: binds as a carrier
	answerDocumentValueFunc                                        // a func type whose result slot is the mutation: a CALL on it yields one
	answerDocumentValueUnrecognized                                // mentions the mutation in a shape the census cannot classify: fail loud
)

// answerDocumentClassifyValueType (§40.45 round-eight #3/#4/#5, the
// type-set-closure ruling) classifies a value type expression precisely:
//
//   - a func type: its RESULT slot must be a plain mutation value (then a
//     call on the value yields the mutation); a func type that mentions the
//     mutation in a PARAMETER — a callback receiving the mutation — or whose
//     result is itself a func/chan/instantiation is unrecognized;
//   - a name: a mutation type name (or a type parameter bound by a carrying
//     constraint) is a mutation value; a named func carrier is a func; a
//     constraint name used as a value type is unrecognized;
//   - a generic instantiation `G[…]`: a mutation-holding generic type
//     instantiated WITHOUT the mutation as a type argument is a mutation
//     value (its mutation fields are concrete); any instantiation that takes
//     the mutation (or a carrier) as a type ARGUMENT is unrecognized (its
//     fields are typed by a parameter the binder never sees);
//   - chans and interfaces mentioning the mutation are unrecognized;
//   - pointers, arrays, slices, maps, structs, variadics over those recurse:
//     a nested func/chan/instantiation/interface mentioning the mutation
//     makes the whole type unrecognized, otherwise it is a mutation value.
func answerDocumentClassifyValueType(expr ast.Expr, facts *answerDocumentMutationFacts, typeParams map[string]bool) (kind answerDocumentValueKind, slot answerDocumentMutationResultSlot, why string) {
	names := facts.carrierNames()
	for name := range typeParams {
		names[name] = true
	}
	var classify func(e ast.Expr) (answerDocumentValueKind, answerDocumentMutationResultSlot, string)
	classify = func(e ast.Expr) (answerDocumentValueKind, answerDocumentMutationResultSlot, string) {
		for {
			p, ok := e.(*ast.ParenExpr)
			if !ok {
				break
			}
			e = p.X
		}
		if !answerDocumentMutationMentions(e, names) {
			return answerDocumentValueNone, answerDocumentMutationResultSlot{}, ""
		}
		switch x := e.(type) {
		case *ast.FuncType:
			if x.Params != nil {
				for _, field := range x.Params.List {
					if answerDocumentMutationMentions(field.Type, names) {
						return answerDocumentValueUnrecognized, answerDocumentMutationResultSlot{}, "a func type that receives the mutation as a parameter (a callback lane the census cannot follow — bind the mutation in a func literal or a named function's parameter instead)"
					}
				}
			}
			if x.Results == nil {
				return answerDocumentValueUnrecognized, answerDocumentMutationResultSlot{}, "a func type mentioning the mutation without a result slot"
			}
			idx, count := 0, 0
			for _, field := range x.Results.List {
				n := len(field.Names)
				if n == 0 {
					n = 1
				}
				count += n
			}
			for _, field := range x.Results.List {
				n := len(field.Names)
				if n == 0 {
					n = 1
				}
				if answerDocumentMutationMentions(field.Type, names) {
					k, _, why := classify(field.Type)
					if k != answerDocumentValueMutation {
						return answerDocumentValueUnrecognized, answerDocumentMutationResultSlot{}, "a func type whose mutation result is not a plain mutation value (" + why + ")"
					}
					return answerDocumentValueFunc, answerDocumentMutationResultSlot{idx: idx, count: count}, ""
				}
				idx += n
			}
			return answerDocumentValueUnrecognized, answerDocumentMutationResultSlot{}, "a func type mentioning the mutation outside its parameters and results"
		case *ast.Ident, *ast.SelectorExpr:
			name := typeName(x)
			if slot, ok := facts.funcTypeNames[name]; ok {
				return answerDocumentValueFunc, slot, ""
			}
			if facts.constraintNames[name] {
				return answerDocumentValueUnrecognized, answerDocumentMutationResultSlot{}, "a constraint interface used as a value type"
			}
			return answerDocumentValueMutation, answerDocumentMutationResultSlot{}, ""
		case *ast.ChanType:
			return answerDocumentValueUnrecognized, answerDocumentMutationResultSlot{}, "a chan carrying the mutation"
		case *ast.InterfaceType:
			return answerDocumentValueUnrecognized, answerDocumentMutationResultSlot{}, "an interface type mentioning the mutation used as a value type"
		case *ast.IndexExpr, *ast.IndexListExpr:
			var base ast.Expr
			var args []ast.Expr
			if ix, ok := x.(*ast.IndexExpr); ok {
				base, args = ix.X, []ast.Expr{ix.Index}
			} else {
				il := x.(*ast.IndexListExpr)
				base, args = il.X, il.Indices
			}
			for _, arg := range args {
				if answerDocumentMutationMentions(arg, names) {
					return answerDocumentValueUnrecognized, answerDocumentMutationResultSlot{}, "a generic type instantiated with the mutation as a type argument (its fields are typed by a parameter the binder never sees)"
				}
			}
			if k, _, why := classify(base); k != answerDocumentValueMutation {
				return answerDocumentValueUnrecognized, answerDocumentMutationResultSlot{}, "an instantiation of a non-value carrier (" + why + ")"
			}
			return answerDocumentValueMutation, answerDocumentMutationResultSlot{}, ""
		case *ast.StarExpr:
			return classify(x.X)
		case *ast.Ellipsis:
			return classify(x.Elt)
		case *ast.ArrayType:
			k, _, w := classify(x.Elt)
			if k == answerDocumentValueFunc {
				return answerDocumentValueUnrecognized, answerDocumentMutationResultSlot{}, "a container of func carriers (an indexed call the census cannot follow)"
			}
			return k, answerDocumentMutationResultSlot{}, w
		case *ast.MapType:
			kind := answerDocumentValueNone
			for _, part := range []ast.Expr{x.Key, x.Value} {
				k, _, w := classify(part)
				switch k {
				case answerDocumentValueUnrecognized:
					return k, answerDocumentMutationResultSlot{}, w
				case answerDocumentValueFunc:
					return answerDocumentValueUnrecognized, answerDocumentMutationResultSlot{}, "a map of func carriers (an indexed call the census cannot follow)"
				case answerDocumentValueMutation:
					kind = k
				}
			}
			return kind, answerDocumentMutationResultSlot{}, ""
		case *ast.StructType:
			// Fields classify one by one (a func field is a func carrier
			// followed by name); the struct is a mutation value when a field
			// holds one outright.
			kind := answerDocumentValueNone
			if x.Fields != nil {
				for _, field := range x.Fields.List {
					k, _, w := classify(field.Type)
					switch k {
					case answerDocumentValueUnrecognized:
						return k, answerDocumentMutationResultSlot{}, "a struct field of " + w
					case answerDocumentValueMutation:
						kind = k
					}
				}
			}
			return kind, answerDocumentMutationResultSlot{}, ""
		}
		return answerDocumentValueUnrecognized, answerDocumentMutationResultSlot{}, fmt.Sprintf("a %T type expression", e)
	}
	return classify(expr)
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

// answerDocumentInstantiationParts splits `G[A]` / `G[A, B]` into the generic
// name and its type arguments.
func answerDocumentInstantiationParts(expr ast.Expr) (name string, args []ast.Expr, ok bool) {
	switch x := expr.(type) {
	case *ast.IndexExpr:
		return typeName(x.X), []ast.Expr{x.Index}, true
	case *ast.IndexListExpr:
		return typeName(x.X), x.Indices, true
	}
	return "", nil, false
}

// answerDocumentApplierInstantiation (§40.45 round-eight #3) classifies an
// instantiation of a generic Apply interface by its method set AFTER
// substitution: it carries when an Apply signature mentions the document
// type directly or through a type parameter instantiated with the document
// type. A substitution the census cannot resolve — a mismatched argument
// count, or an argument that is itself an enclosing type parameter — is
// reported as `why` (fail loud).
func answerDocumentApplierInstantiation(expr ast.Expr, facts *answerDocumentMutationFacts, docName, enclosing map[string]bool) (isApplier, carries bool, why string) {
	name, args, ok := answerDocumentInstantiationParts(expr)
	if !ok {
		return false, false, ""
	}
	ga := facts.genericAppliers[name]
	if ga == nil {
		return false, false, ""
	}
	if len(args) != len(ga.params) {
		return true, false, fmt.Sprintf("%d type argument(s) for %d parameter(s); the substitution cannot be resolved", len(args), len(ga.params))
	}
	for _, ft := range ga.applies {
		if answerDocumentMutationMentions(ft, docName) {
			carries = true
		}
		for i, param := range ga.params {
			if !answerDocumentMutationMentions(ft, map[string]bool{param: true}) {
				continue
			}
			if answerDocumentMutationMentions(args[i], enclosing) {
				return true, false, "instantiated with an enclosing type parameter; the substitution cannot be resolved"
			}
			if answerDocumentMutationMentions(args[i], docName) {
				carries = true
			}
		}
	}
	return true, carries, ""
}

// answerDocumentApplierCarries reports whether any generic-Apply
// instantiation inside expr carries after substitution.
func answerDocumentApplierCarries(expr ast.Expr, facts *answerDocumentMutationFacts, docName, enclosing map[string]bool) bool {
	found := false
	ast.Inspect(expr, func(n ast.Node) bool {
		if e, ok := n.(ast.Expr); ok {
			if _, carries, _ := answerDocumentApplierInstantiation(e, facts, docName, enclosing); carries {
				found = true
			}
		}
		return !found
	})
	return found
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

// answerDocumentTypeParamNames lists the names declared by a type-parameter
// list (nil-safe).
func answerDocumentTypeParamNames(list *ast.FieldList) []string {
	var out []string
	if list == nil {
		return out
	}
	for _, field := range list.List {
		for _, name := range field.Names {
			out = append(out, name.Name)
		}
	}
	return out
}

// answerDocumentCarryingTypeParams classifies a declaration's type-parameter
// list: the names bound by a carrying constraint (type set, Apply-method or
// instantiated-applier lane).
func answerDocumentCarryingTypeParams(list *ast.FieldList, facts *answerDocumentMutationFacts, docName map[string]bool) map[string]bool {
	out := map[string]bool{}
	if list == nil {
		return out
	}
	enclosing := map[string]bool{}
	for _, name := range answerDocumentTypeParamNames(list) {
		enclosing[name] = true
	}
	for _, tp := range list.List {
		setCarries, _ := answerDocumentConstraintCarries(tp.Type, facts)
		if setCarries || answerDocumentTypeParamConstraintDeclaresApply(tp.Type, docName) || answerDocumentApplierCarries(tp.Type, facts, docName, enclosing) {
			for _, name := range tp.Names {
				out[name.Name] = true
			}
		}
	}
	return out
}

func collectAnswerDocumentMutationFacts(tree *retryGenerationTree) *answerDocumentMutationFacts {
	facts := &answerDocumentMutationFacts{
		typeNames:         map[string]bool{"AnswerDocumentMutation": true},
		fieldNames:        map[string]bool{},
		pkgVars:           map[string]bool{},
		results:           map[string]answerDocumentMutationResultSlot{},
		constraintNames:   map[string]bool{},
		funcTypeNames:     map[string]answerDocumentMutationResultSlot{},
		funcFields:        map[string]answerDocumentMutationResultSlot{},
		funcPkgVars:       map[string]answerDocumentMutationResultSlot{},
		generics:          map[string]*answerDocumentGenericFacts{},
		genericAppliers:   map[string]*answerDocumentGenericApplier{},
		genericTypes:      map[string][]string{},
		genericTypeFields: map[string]map[string]answerDocumentGenericResultSlot{},
		genericMethods:    map[string]map[string]*answerDocumentGenericMethodFacts{},
		funcDecls:         map[string]bool{},
	}
	docName := map[string]bool{"AnswerDocumentV2": true}
	unrecognized := func(n ast.Node, why string) {
		facts.unrecognized = append(facts.unrecognized, answerDocumentUnrecognizedShape{node: n, why: why})
	}
	// §40.45 round-eight #3: generic interfaces declaring Apply (anywhere in
	// their body, embedded inline interfaces included) are classified at
	// their instantiations; a generic TypeSpec that re-exports one (embeds
	// or aliases it under new type parameters) cannot be followed → fail
	// loud below.
	for _, file := range tree.files {
		ast.Inspect(file, func(n ast.Node) bool {
			spec, ok := n.(*ast.TypeSpec)
			if !ok || spec.TypeParams == nil {
				return true
			}
			iface, ok := spec.Type.(*ast.InterfaceType)
			if !ok {
				return true
			}
			var applies []*ast.FuncType
			ast.Inspect(iface, func(m ast.Node) bool {
				field, ok := m.(*ast.Field)
				if !ok {
					return true
				}
				ft, ok := field.Type.(*ast.FuncType)
				if !ok {
					return true
				}
				for _, name := range field.Names {
					if name.Name == "Apply" {
						applies = append(applies, ft)
					}
				}
				return true
			})
			if len(applies) > 0 {
				facts.genericAppliers[spec.Name.Name] = &answerDocumentGenericApplier{params: answerDocumentTypeParamNames(spec.TypeParams), applies: applies}
			}
			return true
		})
	}
	// defined/alias types over the mutation carry the census to their names
	// (`type mut = types.AnswerDocumentMutation`, `type mut2 mut`, ...);
	// named interface constraints whose type set carries the mutation
	// (§40.45 round-seven #4) are collected as constraintNames, to the same
	// fixpoint (an embedded carrying constraint carries its embedder, and —
	// round-eight #2 — so does an alias/defined type over one); named func
	// types whose result is the mutation are func carriers (round-eight #5).
	// Every other TypeSpec shape mentioning a carrier name fails loud.
	classified := map[*ast.TypeSpec]bool{}
	for changed := true; changed; {
		changed = false
		for _, file := range tree.files {
			ast.Inspect(file, func(n ast.Node) bool {
				spec, ok := n.(*ast.TypeSpec)
				if !ok || classified[spec] {
					return true
				}
				if iface, isInterface := spec.Type.(*ast.InterfaceType); isInterface {
					if spec.TypeParams == nil {
						if carries, _ := answerDocumentConstraintCarries(iface, facts); carries {
							facts.constraintNames[spec.Name.Name] = true
							classified[spec] = true
							changed = true
						}
					}
					return true
				}
				if named := typeName(spec.Type); named != "" && facts.constraintNames[named] {
					facts.constraintNames[spec.Name.Name] = true
					classified[spec] = true
					changed = true
					return true
				}
				kind, slot, _ := answerDocumentClassifyValueType(spec.Type, facts, nil)
				switch kind {
				case answerDocumentValueMutation:
					facts.typeNames[spec.Name.Name] = true
					classified[spec] = true
					changed = true
				case answerDocumentValueFunc:
					facts.funcTypeNames[spec.Name.Name] = slot
					classified[spec] = true
					changed = true
				}
				return true
			})
		}
	}
	genericApplierNames := map[string]bool{}
	for name := range facts.genericAppliers {
		genericApplierNames[name] = true
	}
	// §40.45 round-nine #8: generic TYPES — declared params and struct-field
	// shapes in terms of them (classified once the params are instantiated
	// with the mutation at a call that infers them). EVOLUTION RECORD: the
	// round-eight facts never consulted a generic type's fields or methods
	// (type params on Recv), so `newBag(ms).first()` / `.items[0]` yielded
	// nothing while `var b bag[types.AnswerDocumentMutation]` failed loud.
	for _, file := range tree.files {
		ast.Inspect(file, func(n ast.Node) bool {
			spec, ok := n.(*ast.TypeSpec)
			if !ok || spec.TypeParams == nil {
				return true
			}
			params := answerDocumentTypeParamNames(spec.TypeParams)
			facts.genericTypes[spec.Name.Name] = params
			own := map[string]bool{}
			for _, name := range params {
				own[name] = true
			}
			if st, isStruct := spec.Type.(*ast.StructType); isStruct && st.Fields != nil {
				fields := map[string]answerDocumentGenericResultSlot{}
				for _, field := range st.Fields.List {
					slot := answerDocumentGenericSlotOf(field.Type, own, false, facts)
					for _, name := range field.Names {
						fields[name.Name] = slot
					}
				}
				facts.genericTypeFields[spec.Name.Name] = fields
			}
			return true
		})
	}
	for _, file := range tree.files {
		ast.Inspect(file, func(n ast.Node) bool {
			switch node := n.(type) {
			case *ast.TypeSpec:
				if classified[node] {
					return true
				}
				if node.TypeParams != nil && answerDocumentMutationMentions(node.Type, genericApplierNames) {
					unrecognized(node, "a generic type re-exporting a generic Apply interface under its own type parameters; the census cannot resolve its instantiations")
					return true
				}
				if _, isInterface := node.Type.(*ast.InterfaceType); isInterface {
					return true // the type-set / Apply-method lanes classify interfaces
				}
				if kind, _, why := answerDocumentClassifyValueType(node.Type, facts, nil); kind == answerDocumentValueUnrecognized {
					unrecognized(node, "declares a type over "+why)
				}
			case *ast.StructType:
				if node.Fields == nil {
					return true
				}
				for _, field := range node.Fields.List {
					kind, slot, why := answerDocumentClassifyValueType(field.Type, facts, nil)
					switch kind {
					case answerDocumentValueMutation:
						for _, name := range field.Names {
							facts.fieldNames[name.Name] = true
						}
					case answerDocumentValueFunc:
						for _, name := range field.Names {
							facts.funcFields[name.Name] = slot
						}
					case answerDocumentValueUnrecognized:
						unrecognized(field, "declares a struct field of "+why)
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
					holds := false
					var funcSlot *answerDocumentMutationResultSlot
					if vs.Type != nil {
						kind, slot, why := answerDocumentClassifyValueType(vs.Type, facts, nil)
						switch kind {
						case answerDocumentValueMutation:
							holds = true
						case answerDocumentValueFunc:
							s := slot
							funcSlot = &s
						case answerDocumentValueUnrecognized:
							unrecognized(vs, "declares a package-level var of "+why)
						}
					}
					for _, v := range vs.Values {
						if call, isCall := v.(*ast.CallExpr); isCall && answerDocumentMutationConstructors[selectorCallee(call)] {
							holds = true
						}
						if _, isLit := v.(*ast.CompositeLit); isLit && answerDocumentMutationMentions(v, facts.typeNames) {
							holds = true
						}
					}
					for _, name := range vs.Names {
						if holds {
							facts.pkgVars[name.Name] = true
						}
						if funcSlot != nil {
							facts.funcPkgVars[name.Name] = *funcSlot
						}
					}
				}
			case *ast.FuncDecl:
				facts.funcDecls[d.Name.Name] = true
				if d.Type.Results == nil {
					continue
				}
				carrying := answerDocumentCarryingTypeParams(d.Type.TypeParams, facts, docName)
				typeParams := map[string]bool{}
				for _, name := range answerDocumentTypeParamNames(d.Type.TypeParams) {
					typeParams[name] = true
				}
				// §40.45 round-nine #8: a method on a generic type carries the
				// receiver's type params (positional against the type's
				// declaration); its results are shaped in terms of them.
				var recvParams []string
				recvType := ""
				if d.Recv != nil && len(d.Recv.List) == 1 {
					if name, args, ok := answerDocumentInstantiationParts(exprUnstar(d.Recv.List[0].Type)); ok && facts.genericTypes[name] != nil {
						recvType = name
						for _, arg := range args {
							if id, isIdent := arg.(*ast.Ident); isIdent {
								recvParams = append(recvParams, id.Name)
							} else {
								recvParams = append(recvParams, "")
							}
						}
					}
				}
				idx, count := 0, 0
				for _, field := range d.Type.Results.List {
					n := len(field.Names)
					if n == 0 {
						n = 1
					}
					count += n
				}
				var generic *answerDocumentGenericFacts
				if len(typeParams) > 0 {
					generic = &answerDocumentGenericFacts{carrying: carrying}
					if d.Type.Params != nil {
						for _, field := range d.Type.Params.List {
							mentioned := map[string]bool{}
							for name := range typeParams {
								if answerDocumentMutationMentions(field.Type, map[string]bool{name: true}) {
									mentioned[name] = true
								}
							}
							// §40.45 round-nine #5: a func-typed parameter's
							// result slots, in terms of the type params, so a
							// func-carrier argument instantiates them.
							var funcResults []map[string]bool
							paramType := field.Type
							if ell, variadic := paramType.(*ast.Ellipsis); variadic {
								paramType = ell.Elt
							}
							if ft, isFunc := paramType.(*ast.FuncType); isFunc && ft.Results != nil {
								for _, res := range ft.Results.List {
									m := map[string]bool{}
									for name := range typeParams {
										if answerDocumentMutationMentions(res.Type, map[string]bool{name: true}) {
											m[name] = true
										}
									}
									k := len(res.Names)
									if k == 0 {
										k = 1
									}
									for i := 0; i < k; i++ {
										funcResults = append(funcResults, m)
									}
								}
							}
							// §40.45 round-nine #8: an instance-typed parameter
							// (`b bag[T]`) is instantiated by an instance argument.
							var paramInstance *answerDocumentGenericInstanceShape
							if shape := answerDocumentGenericShapeOf(paramType, typeParams, facts); shape.instance != nil {
								paramInstance = shape.instance
							}
							n := len(field.Names)
							if n == 0 {
								n = 1
							}
							for i := 0; i < n; i++ {
								generic.paramSlots = append(generic.paramSlots, mentioned)
								generic.paramFuncResults = append(generic.paramFuncResults, funcResults)
								generic.paramInstances = append(generic.paramInstances, paramInstance)
							}
							if _, variadic := field.Type.(*ast.Ellipsis); variadic {
								generic.variadic = true
							}
						}
					}
					facts.generics[d.Name.Name] = generic
				}
				var method *answerDocumentGenericMethodFacts
				recvParamSet := map[string]bool{}
				if recvType != "" {
					method = &answerDocumentGenericMethodFacts{recvParams: recvParams}
					for _, name := range recvParams {
						if name != "" {
							recvParamSet[name] = true
						}
					}
					if facts.genericMethods[recvType] == nil {
						facts.genericMethods[recvType] = map[string]*answerDocumentGenericMethodFacts{}
					}
					facts.genericMethods[recvType][d.Name.Name] = method
				}
				recorded := false
				for _, field := range d.Type.Results.List {
					n := len(field.Names)
					if n == 0 {
						n = 1
					}
					kind, _, why := answerDocumentClassifyValueType(field.Type, facts, carrying)
					switch kind {
					case answerDocumentValueMutation:
						if !recorded {
							facts.results[d.Name.Name] = answerDocumentMutationResultSlot{idx: idx, count: count}
							recorded = true
						}
					case answerDocumentValueFunc:
						unrecognized(field, "returns a func carrier (a call on a call result the census cannot follow)")
					case answerDocumentValueUnrecognized:
						unrecognized(field, "returns "+why)
					}
					if generic != nil {
						slot := answerDocumentGenericSlotOf(field.Type, typeParams, kind == answerDocumentValueMutation, facts)
						for i := 0; i < n; i++ {
							generic.results = append(generic.results, slot)
						}
					}
					if method != nil {
						slot := answerDocumentGenericSlotOf(field.Type, recvParamSet, kind == answerDocumentValueMutation, facts)
						for i := 0; i < n; i++ {
							method.results = append(method.results, slot)
						}
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
// every reference shape, each pinned by its own self-red subtest. v3 (§40.45
// round-eight #2–#5) classifies by TYPE-SET CLOSURE instead of spelling:
// alias/defined chains over carrying constraints carry; generic Apply
// interfaces carry at their instantiation with the document type; carrying
// type parameters bind in parameter, receiver, field AND result position
// (a generic call's result binds — outright, or inferred from a
// mutation-bound argument); func-typed values whose result is the mutation
// yield it when called; and every type expression mentioning the mutation
// in a shape none of those lanes classifies fails loud — including an Apply
// whose receiver is a call the census cannot classify. v4 (§40.45 round-nine
// #5–#8) closes the inference gaps of v3: a generic's type params are also
// inferred from a func-typed argument's yielding result slot and from an
// instance argument, a generic returning `func() T` yields a func carrier,
// make/new classify by their type argument, methods and fields of generic
// TYPES yield through an inferred instance binding, and the result of an
// unclassifiable call fed a mutation-bound value binds as UNCLASSIFIED so
// an Apply through it — chained or after binding — fails loud.
func answerDocumentPatchBaseConstructorCensus(tree *retryGenerationTree, sites map[retryGenerationWriterKey]bool) (offenders []string) {
	seen := map[retryGenerationWriterKey]bool{}
	facts := collectAnswerDocumentMutationFacts(tree)
	mentions := func(n ast.Node) bool { return answerDocumentMutationMentions(n, facts.typeNames) }
	docName := map[string]bool{"AnswerDocumentV2": true}
	for _, u := range facts.unrecognized {
		offenders = append(offenders, fmt.Sprintf("%s %s carrying the mutation; the census cannot classify it — spell the carrier as a plain mutation value, a slice/array/map/pointer of it, or a func returning it (§40.45 round-eight: type-set closure, fail-loud on unrecognized shapes)", tree.pos(u.node), u.why))
	}
	for path, file := range tree.files {
		pkg, base := retryGenerationPkgDir(path), filepath.Base(path)
		// foreignImports are this file's import names outside the module: a
		// call qualified by one is a FOREIGN callee the tree-wide facts (keyed
		// by bare name) say nothing about — unclassifiable, never "a tree
		// function that yields nothing" (`slices.Clone` vs a tree `Clone`).
		foreignImports := map[string]bool{}
		for _, spec := range file.Imports {
			importPath := strings.Trim(spec.Path.Value, "\"`")
			if strings.HasPrefix(importPath, "github.com/hanchaoqun/codrax/") {
				continue
			}
			local := importPath
			if i := strings.LastIndex(local, "/"); i >= 0 {
				local = local[i+1:]
			}
			if spec.Name != nil && spec.Name.Name != "" {
				local = spec.Name.Name
			}
			foreignImports[local] = true
		}
		scan := func(key retryGenerationWriterKey, fn *ast.FuncDecl, root ast.Node, enclosing map[string]bool) {
			// report records one offense; the call classifiers run to a
			// fixpoint and re-classify the same call, so an offense is
			// recorded once per (node, reason).
			reported := map[string]bool{}
			report := func(n ast.Node, what string) {
				if sites[key] {
					seen[key] = true
					return
				}
				line := fmt.Sprintf("%s:%s (%s) %s outside the registered base constructor sites (§40.17 ②: stage / commit / rollback share one base constructor)", path, key.fn, tree.pos(n), what)
				if reported[line] {
					return
				}
				reported[line] = true
				offenders = append(offenders, line)
			}
			// identifiers bound to an AnswerDocumentMutation (or a container
			// of them) by receiver, parameter, package-level var, or any
			// assignment / declaration / range frame, to a fixpoint.
			bound := map[string]bool{}
			for name := range facts.pkgVars {
				bound[name] = true
			}
			// §40.45 round-eight #5: identifiers bound to a FUNC CARRIER — a
			// call on them yields the mutation at the recorded result slot.
			boundFuncs := map[string]answerDocumentMutationResultSlot{}
			for name, slot := range facts.funcPkgVars {
				boundFuncs[name] = slot
			}
			// §40.43 round-six #13: interface lanes living in the SIGNATURE
			// — a generic type-parameter's inline constraint or an anonymous
			// interface parameter type — are choked at the declaration
			// exactly like a named interface: fn.Type covers TypeParams,
			// Params and Results, none of which the body-rooted scan saw.
			// §40.45 round-eight #3: an Apply method abstracted over an
			// ENCLOSING type parameter (`[D any, M interface{ Apply(*D) (*D,
			// error) }]`) has no instantiation site to classify — fail loud.
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
							if name.Name != "Apply" {
								continue
							}
							if answerDocumentMutationMentions(ft, docName) {
								report(method, "declares an Apply method with the mutation's base-constructor signature (an interface lane could launder base construction)")
							} else if answerDocumentMutationMentions(ft, enclosing) {
								report(method, "declares an Apply method abstracted over an enclosing type parameter; the census cannot resolve its instantiations — spell the document type concretely or drop the generic lane")
							}
						}
					}
					return true
				})
			}
			// §40.45 round-eight #3: every instantiation of a generic Apply
			// interface is classified by its method set after substitution.
			reportApplierInstantiations := func(root ast.Node) {
				ast.Inspect(root, func(n ast.Node) bool {
					e, ok := n.(ast.Expr)
					if !ok {
						return true
					}
					isApplier, carries, why := answerDocumentApplierInstantiation(e, facts, docName, enclosing)
					switch {
					case !isApplier:
					case why != "":
						report(n, "instantiates a generic Apply interface the census cannot resolve ("+why+")")
					case carries:
						report(n, "instantiates a generic interface whose Apply method set, after substitution, has the mutation's base-constructor signature (an interface lane could launder base construction)")
					}
					return true
				})
			}
			mutationTypeParams := map[string]bool{}
			// bindValue classifies one declared value (parameter, receiver,
			// func-literal parameter, local var) and binds its names.
			bindValue := func(field *ast.Field) {
				kind, slot, why := answerDocumentClassifyValueType(field.Type, facts, mutationTypeParams)
				switch kind {
				case answerDocumentValueMutation:
					for _, name := range field.Names {
						bound[name.Name] = true
					}
				case answerDocumentValueFunc:
					for _, name := range field.Names {
						boundFuncs[name.Name] = slot
					}
				case answerDocumentValueUnrecognized:
					report(field, "declares a value of "+why+"; the census cannot classify it")
				}
			}
			if fn != nil {
				reportSignatureInterfaces(fn.Type)
				reportApplierInstantiations(fn.Type)
				if fn.Type.TypeParams != nil {
					for _, tp := range fn.Type.TypeParams.List {
						// §40.45 round-seven #4: the constraint is classified by
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
						// Round-eight #2/#3: alias/defined chains over a carrying
						// constraint and instantiated generic Apply interfaces
						// carry too.
						setCarries, unrecognized := answerDocumentConstraintCarries(tp.Type, facts)
						declaresApply := answerDocumentTypeParamConstraintDeclaresApply(tp.Type, docName)
						applierCarries := answerDocumentApplierCarries(tp.Type, facts, docName, enclosing)
						if !setCarries && !declaresApply && !applierCarries && !mentions(tp.Type) {
							continue
						}
						for _, n := range unrecognized {
							report(n, "uses an unrecognized generic constraint element shape carrying the mutation; the census cannot follow the values it admits — spell the type set with the mutation type, ~T, a union, or a slice/array/map of it, or drop the generic lane")
						}
						if !setCarries && !declaresApply && !applierCarries {
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
					bindValue(field)
				}
			}
			// §40.45 round-eight #5: func-literal parameters bind like
			// FuncDecl parameters (a callback body applying its mutation
			// parameter is a constructor use).
			ast.Inspect(root, func(n ast.Node) bool {
				lit, ok := n.(*ast.FuncLit)
				if !ok || lit.Type.Params == nil {
					return true
				}
				for _, field := range lit.Type.Params.List {
					bindValue(field)
				}
				return true
			})
			reportApplierInstantiations(root)
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
			// §40.45 round-nine #8: identifiers bound to an INFERRED
			// instantiation of a tree generic type (`b := newBag(ms)`), with
			// the type's declared params that the mutation instantiates.
			boundInstances := map[string]*answerDocumentInstanceBinding{}
			// §40.45 round-nine #7: identifiers bound to the result of a call
			// the census cannot classify that was fed a mutation-bound value
			// (a foreign / builtin / unknown callee, a generic fed one of
			// these): an Apply through them fails loud. EVOLUTION RECORD: the
			// round-eight binder left such an LHS silently unbound, so only the
			// chained `slices.Clone(ms)[0].Apply` form failed loud while
			// `sorted := slices.Clone(ms); sorted[0].Apply` escaped.
			boundUnclassified := map[string]string{}
			var yields func(expr ast.Expr) bool
			var callYields func(x *ast.CallExpr) []answerDocumentYield
			var instanceOf func(expr ast.Expr) (*answerDocumentInstanceBinding, bool)
			var unclassifiedOf func(expr ast.Expr) (string, bool)
			// funcCarrierSlot resolves a func-carrier reference expression
			// (a bound func value, a func literal, a func field, a
			// mutation-returning FuncDecl or method value, a call yielding a
			// func carrier) to its yielding result slot.
			var funcCarrierSlot func(expr ast.Expr) (answerDocumentMutationResultSlot, bool)
			funcCarrierSlot = func(expr ast.Expr) (answerDocumentMutationResultSlot, bool) {
				switch x := expr.(type) {
				case *ast.ParenExpr:
					return funcCarrierSlot(x.X)
				case *ast.FuncLit:
					return answerDocumentFuncLitResultSlot(x, facts.typeNames)
				case *ast.Ident:
					if slot, ok := boundFuncs[x.Name]; ok {
						return slot, true
					}
					if lit := funcVals[x.Name]; lit != nil {
						return answerDocumentFuncLitResultSlot(lit, facts.typeNames)
					}
					if slot, ok := facts.results[x.Name]; ok && facts.generics[x.Name] == nil {
						return slot, true
					}
				case *ast.SelectorExpr:
					if slot, ok := facts.funcFields[x.Sel.Name]; ok {
						return slot, true
					}
					if slot, ok := facts.results[x.Sel.Name]; ok && facts.generics[x.Sel.Name] == nil {
						return slot, true
					}
					if inst, ok := instanceOf(x.X); ok {
						if field, ok := facts.genericTypeFields[inst.typeName][x.Sel.Name]; ok {
							if y := answerDocumentYieldOfSlot(field, inst.params); y.kind == answerDocumentYieldFuncCarrier {
								return y.slot, true
							}
						}
					}
				case *ast.CallExpr:
					if out := callYields(x); len(out) == 1 && out[0].kind == answerDocumentYieldFuncCarrier {
						return out[0].slot, true
					}
				}
				return answerDocumentMutationResultSlot{}, false
			}
			// calleeName names a call's callee, looking through an explicit
			// instantiation `f[X](…)`.
			calleeName := func(x *ast.CallExpr) string {
				fun := x.Fun
				switch f := fun.(type) {
				case *ast.IndexExpr:
					fun = f.X
				case *ast.IndexListExpr:
					fun = f.X
				}
				return selectorCallee(&ast.CallExpr{Fun: fun})
			}
			// fed: the call is fed a mutation-bound, mutation-mentioning,
			// func-carrier, instance or unclassified value — through an
			// argument, the receiver, or the callee expression itself.
			fed := func(x *ast.CallExpr) bool {
				if _, ok := unclassifiedOf(x.Fun); ok {
					return true
				}
				if sel, ok := x.Fun.(*ast.SelectorExpr); ok {
					if yields(sel.X) {
						return true
					}
					if _, ok := instanceOf(sel.X); ok {
						return true
					}
				}
				for _, arg := range x.Args {
					if yields(arg) || mentions(arg) {
						return true
					}
					if _, ok := unclassifiedOf(arg); ok {
						return true
					}
					if _, ok := funcCarrierSlot(arg); ok {
						return true
					}
					if _, ok := instanceOf(arg); ok {
						return true
					}
				}
				return false
			}
			// resolveSlots classifies result slots once the instantiated type
			// params are known (§40.45 round-nine #5/#8): a plain mutation
			// shape yields the mutation, a func shape a func carrier, an
			// instantiation of a tree generic type an instance, and an
			// unrecognized shape fails loud (the slot binds as unclassified).
			resolveSlots := func(at ast.Node, results []answerDocumentGenericResultSlot, instantiated map[string]bool) []answerDocumentYield {
				out := make([]answerDocumentYield, len(results))
				for i, res := range results {
					y := answerDocumentYieldOfSlot(res, instantiated)
					if y.kind == answerDocumentYieldUnclassified {
						report(at, "a generic call whose result, with its type parameter instantiated by the mutation, is "+res.shape.why+"; the census cannot follow it")
					}
					out[i] = y
				}
				return out
			}
			// callYields classifies a call: per result slot, what it yields.
			// nil means the callee is not one the census can classify (its
			// result binds as UNCLASSIFIED when a mutation-bound value flows
			// in; an Apply through it fails loud). Classifications are
			// memoized per fixpoint iteration (the binder clears the memo
			// before each pass; a pass that changes nothing sees a memo
			// consistent with its final bindings).
			yieldMemo := map[*ast.CallExpr][]answerDocumentYield{}
			var classifyCall func(x *ast.CallExpr) []answerDocumentYield
			callYields = func(x *ast.CallExpr) []answerDocumentYield {
				if out, ok := yieldMemo[x]; ok {
					return out
				}
				out := classifyCall(x)
				yieldMemo[x] = out
				return out
			}
			classifyCall = func(x *ast.CallExpr) []answerDocumentYield {
				mutation := answerDocumentYield{kind: answerDocumentYieldMutation}
				none := []answerDocumentYield{{}}
				slots := func(slot answerDocumentMutationResultSlot) []answerDocumentYield {
					out := make([]answerDocumentYield, slot.count)
					if slot.idx < slot.count {
						out[slot.idx] = mutation
					}
					return out
				}
				if isCtor(x) {
					return []answerDocumentYield{mutation}
				}
				switch f := x.Fun.(type) {
				case *ast.Ident, *ast.SelectorExpr:
					if mentions(f) { // conversion: AnswerDocumentMutation(v)
						return []answerDocumentYield{mutation}
					}
				case *ast.IndexExpr, *ast.IndexListExpr:
					if mentions(f) {
						// §40.45 round-nine #8: the explicit spelling of a
						// mutation type argument, like its value-type twin,
						// is not followed — fail loud.
						report(x, "calls a generic function explicitly instantiated with the mutation as a type argument; the census cannot follow the instantiation — let the type argument be inferred from a mutation-bound argument")
						return nil
					}
				}
				if sel, ok := x.Fun.(*ast.SelectorExpr); ok {
					if qual, ok := sel.X.(*ast.Ident); ok && foreignImports[qual.Name] {
						return nil // foreign package: unclassifiable
					}
					// §40.45 round-nine #8: a method on an instance of a
					// generic type yields by its result shape in terms of the
					// receiver's type params.
					if inst, ok := instanceOf(sel.X); ok {
						if method := facts.genericMethods[inst.typeName][sel.Sel.Name]; method != nil {
							instantiated := map[string]bool{}
							declared := facts.genericTypes[inst.typeName]
							for i, name := range method.recvParams {
								if name != "" && i < len(declared) && inst.params[declared[i]] {
									instantiated[name] = true
								}
							}
							return resolveSlots(x, method.results, instantiated)
						}
						return nil // a call on an instance the facts do not know: unclassifiable
					}
				}
				callee := calleeName(x)
				if generic := facts.generics[callee]; generic != nil {
					// §40.45 round-eight #4: a carrying type param carries in
					// RESULT position; a non-carrying one is inferred from a
					// mutation-bound argument in a parameter it types — and,
					// round-nine #5/#8, from a func-carrier argument's yielding
					// result slot or an instance argument's instantiated params.
					// An unclassified argument feeding a type param a result
					// mentions makes the call unclassifiable.
					instantiated := map[string]bool{}
					for name := range generic.carrying {
						instantiated[name] = true
					}
					unclassifiedParams := map[string]bool{}
					for i, arg := range x.Args {
						slot := i
						if slot >= len(generic.paramSlots) {
							if !generic.variadic || len(generic.paramSlots) == 0 {
								break
							}
							slot = len(generic.paramSlots) - 1
						}
						if yields(arg) {
							for name := range generic.paramSlots[slot] {
								instantiated[name] = true
							}
						}
						if carrier, ok := funcCarrierSlot(arg); ok {
							if funcResults := generic.paramFuncResults[slot]; carrier.idx < len(funcResults) {
								for name := range funcResults[carrier.idx] {
									instantiated[name] = true
								}
							}
						}
						if inst, ok := instanceOf(arg); ok {
							if pi := generic.paramInstances[slot]; pi != nil && pi.typeName == inst.typeName {
								declared := facts.genericTypes[inst.typeName]
								for j, name := range pi.args {
									if name != "" && j < len(declared) && inst.params[declared[j]] {
										instantiated[name] = true
									}
								}
							}
						}
						if _, ok := unclassifiedOf(arg); ok {
							for name := range generic.paramSlots[slot] {
								unclassifiedParams[name] = true
							}
						}
					}
					for _, res := range generic.results {
						for name := range res.typeParams {
							if unclassifiedParams[name] && !instantiated[name] {
								return nil
							}
						}
					}
					return resolveSlots(x, generic.results, instantiated)
				}
				if _, isIdent := x.Fun.(*ast.Ident); isIdent {
					switch callee {
					case "append":
						// append's result holds its first argument AND every
						// appended value.
						for _, arg := range x.Args {
							if yields(arg) {
								return []answerDocumentYield{mutation}
							}
						}
						return none
					case "make", "new":
						// §40.45 round-nine #6: make/new classify by their type
						// argument exactly like a parameter-position type.
						// EVOLUTION RECORD: the round-eight binder classified
						// them as nothing, so `ms := make([]Mutation, 0, n)` +
						// append + `ms[0].Apply`, make-map, new, make-chan and
						// make-map-of-func all escaped.
						if len(x.Args) == 0 {
							return nil
						}
						kind, _, why := answerDocumentClassifyValueType(x.Args[0], facts, mutationTypeParams)
						switch kind {
						case answerDocumentValueNone:
							return none
						case answerDocumentValueMutation:
							return []answerDocumentYield{mutation}
						case answerDocumentValueFunc:
							report(x, callee+" of a func carrier (a call through a pointer or element the census cannot follow)")
							return nil
						default:
							report(x, callee+" of "+why+"; the census cannot classify it")
							return nil
						}
					}
				}
				if slot, ok := funcCarrierSlot(x.Fun); ok {
					return slots(slot)
				}
				if slot, ok := facts.results[callee]; ok {
					return slots(slot)
				}
				if facts.funcDecls[callee] {
					return none // a tree function/method with no mutation result
				}
				return nil
			}
			// yields: the expression's value is (or contains) a mutation.
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
					if facts.fieldNames[x.Sel.Name] {
						return true
					}
					// §40.45 round-nine #8: a field of a generic type instance
					// typed by an instantiated param.
					if inst, ok := instanceOf(x.X); ok {
						if field, ok := facts.genericTypeFields[inst.typeName][x.Sel.Name]; ok {
							return answerDocumentYieldOfSlot(field, inst.params).kind == answerDocumentYieldMutation
						}
					}
					return false
				case *ast.CallExpr:
					out := callYields(x)
					return len(out) == 1 && out[0].kind == answerDocumentYieldMutation
				case *ast.CompositeLit:
					return x.Type != nil && mentions(x.Type)
				case *ast.TypeAssertExpr:
					return x.Type != nil && mentions(x.Type)
				}
				return false
			}
			// instanceOf resolves an expression to a generic type instance
			// binding (a bound identifier, or a call yielding one).
			instanceOf = func(expr ast.Expr) (*answerDocumentInstanceBinding, bool) {
				switch x := expr.(type) {
				case *ast.Ident:
					inst, ok := boundInstances[x.Name]
					return inst, ok
				case *ast.ParenExpr:
					return instanceOf(x.X)
				case *ast.StarExpr:
					return instanceOf(x.X)
				case *ast.UnaryExpr:
					return instanceOf(x.X)
				case *ast.CallExpr:
					if out := callYields(x); len(out) == 1 && out[0].kind == answerDocumentYieldInstance {
						return out[0].inst, true
					}
				}
				return nil, false
			}
			// unclassifiedOf resolves an expression to the callee of the
			// unclassifiable call it was bound from (§40.45 round-nine #7).
			unclassifiedOf = func(expr ast.Expr) (string, bool) {
				switch x := expr.(type) {
				case *ast.Ident:
					callee, ok := boundUnclassified[x.Name]
					return callee, ok
				case *ast.ParenExpr:
					return unclassifiedOf(x.X)
				case *ast.StarExpr:
					return unclassifiedOf(x.X)
				case *ast.UnaryExpr:
					return unclassifiedOf(x.X)
				case *ast.IndexExpr:
					return unclassifiedOf(x.X)
				case *ast.SliceExpr:
					return unclassifiedOf(x.X)
				case *ast.SelectorExpr:
					return unclassifiedOf(x.X)
				case *ast.CallExpr:
					if callYields(x) == nil && fed(x) {
						if name := calleeName(x); name != "" {
							return name, true
						}
						return fmt.Sprintf("%T", x.Fun), true
					}
				}
				return "", false
			}
			for changed := true; changed; {
				changed = false
				yieldMemo = map[*ast.CallExpr][]answerDocumentYield{}
				bind := func(e ast.Expr) {
					if id, ok := e.(*ast.Ident); ok && id.Name != "_" && !bound[id.Name] {
						bound[id.Name] = true
						changed = true
					}
				}
				bindFunc := func(e ast.Expr, slot answerDocumentMutationResultSlot) {
					if id, ok := e.(*ast.Ident); ok && id.Name != "_" {
						if _, already := boundFuncs[id.Name]; !already {
							boundFuncs[id.Name] = slot
							changed = true
						}
					}
				}
				bindInstance := func(e ast.Expr, inst *answerDocumentInstanceBinding) {
					if id, ok := e.(*ast.Ident); ok && id.Name != "_" {
						if _, already := boundInstances[id.Name]; !already {
							boundInstances[id.Name] = inst
							changed = true
						}
					}
				}
				bindUnclassified := func(e ast.Expr, callee string) {
					if id, ok := e.(*ast.Ident); ok && id.Name != "_" {
						if _, already := boundUnclassified[id.Name]; !already {
							boundUnclassified[id.Name] = callee
							changed = true
						}
					}
				}
				bindYield := func(e ast.Expr, y answerDocumentYield, callee string) {
					switch y.kind {
					case answerDocumentYieldMutation:
						bind(e)
					case answerDocumentYieldFuncCarrier:
						bindFunc(e, y.slot)
					case answerDocumentYieldInstance:
						bindInstance(e, y.inst)
					case answerDocumentYieldUnclassified:
						bindUnclassified(e, callee)
					}
				}
				// bindExpr binds one LHS from one single-value RHS.
				bindExpr := func(lhs, rhs ast.Expr) {
					if yields(rhs) {
						bind(lhs)
					}
					if slot, ok := funcCarrierSlot(rhs); ok {
						bindFunc(lhs, slot)
					}
					if inst, ok := instanceOf(rhs); ok {
						bindInstance(lhs, inst)
					}
					if callee, ok := unclassifiedOf(rhs); ok {
						bindUnclassified(lhs, callee)
					}
				}
				// bindCall binds a multi-value LHS list from one call.
				bindCall := func(lhs []ast.Expr, call *ast.CallExpr) {
					out := callYields(call)
					if out == nil {
						if callee, ok := unclassifiedOf(call); ok {
							for _, l := range lhs {
								bindUnclassified(l, callee)
							}
						}
						return
					}
					for i, y := range out {
						if i < len(lhs) {
							bindYield(lhs[i], y, calleeName(call))
						}
					}
				}
				ast.Inspect(root, func(n ast.Node) bool {
					switch node := n.(type) {
					case *ast.AssignStmt:
						if len(node.Rhs) == 1 && len(node.Lhs) > 1 {
							if call, ok := node.Rhs[0].(*ast.CallExpr); ok {
								bindCall(node.Lhs, call)
							}
						}
						if len(node.Rhs) == len(node.Lhs) {
							for i, rhs := range node.Rhs {
								bindExpr(node.Lhs[i], rhs)
							}
						}
					case *ast.ValueSpec:
						if node.Type != nil {
							kind, slot, why := answerDocumentClassifyValueType(node.Type, facts, mutationTypeParams)
							switch kind {
							case answerDocumentValueMutation:
								for _, name := range node.Names {
									bind(name)
								}
							case answerDocumentValueFunc:
								for _, name := range node.Names {
									bindFunc(name, slot)
								}
							case answerDocumentValueUnrecognized:
								report(node, "declares a value of "+why+"; the census cannot classify it")
							}
						}
						if len(node.Values) == 1 && len(node.Names) > 1 {
							if call, ok := node.Values[0].(*ast.CallExpr); ok {
								lhs := make([]ast.Expr, len(node.Names))
								for i, name := range node.Names {
									lhs[i] = name
								}
								bindCall(lhs, call)
							}
						}
						for i, v := range node.Values {
							if i >= len(node.Names) {
								break
							}
							bindExpr(node.Names[i], v)
						}
					case *ast.RangeStmt:
						if yields(node.X) {
							bind(node.Key)
							bind(node.Value)
						}
						if callee, ok := unclassifiedOf(node.X); ok {
							bindUnclassified(node.Key, callee)
							bindUnclassified(node.Value, callee)
						}
					}
					return true
				})
			}
			yieldMemo = map[*ast.CallExpr][]answerDocumentYield{}
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
						// §40.45 round-eight #4/#5, round-nine #7 fail-loud: an
						// Apply on the result of a call the census cannot
						// classify, fed a mutation-bound (or mutation-mentioning)
						// value — chained, or through an identifier bound to
						// it — is a laundered constructor use until proven
						// otherwise.
						if callee, ok := unclassifiedOf(node.X); ok {
							through := ""
							if id, isIdent := node.X.(*ast.Ident); isIdent {
								through = " — bound to " + id.Name
							}
							report(node, "applies the result of a call the census cannot classify ("+callee+") fed a mutation-bound argument"+through)
						}
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
					// §40.45 round-seven #4: a named constraint's type set is
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
						own := map[string]bool{}
						for _, name := range answerDocumentTypeParamNames(node.TypeParams) {
							own[name] = true
						}
						for _, tp := range node.TypeParams.List {
							setCarries, _ := answerDocumentConstraintCarries(tp.Type, facts)
							if setCarries || mentions(tp.Type) || answerDocumentTypeParamConstraintDeclaresApply(tp.Type, docName) || answerDocumentApplierCarries(tp.Type, facts, docName, own) {
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
					enclosing := map[string]bool{}
					for _, name := range answerDocumentTypeParamNames(fn.Type.TypeParams) {
						enclosing[name] = true
					}
					if fn.Recv != nil {
						for _, field := range fn.Recv.List {
							if _, args, ok := answerDocumentInstantiationParts(exprUnstar(field.Type)); ok {
								for _, arg := range args {
									if id, isIdent := arg.(*ast.Ident); isIdent {
										enclosing[id.Name] = true
									}
								}
							}
						}
					}
					scan(retryGenerationWriterKey{pkg, base, retryGenerationFuncName(fn)}, fn, fn.Body, enclosing)
				}
				continue
			}
			enclosing := map[string]bool{}
			if gd, ok := decl.(*ast.GenDecl); ok {
				for _, spec := range gd.Specs {
					if ts, ok := spec.(*ast.TypeSpec); ok {
						for _, name := range answerDocumentTypeParamNames(ts.TypeParams) {
							enclosing[name] = true
						}
					}
				}
			}
			scan(retryGenerationWriterKey{pkg, base, "<package scope>"}, nil, decl, enclosing)
		}
	}
	for key := range sites {
		if !seen[key] {
			offenders = append(offenders, fmt.Sprintf("registered base constructor site %s/%s:%s does not exist or no longer constructs a base; update the table", key.pkg, key.file, key.fn))
		}
	}
	return offenders
}

// exprUnstar strips a leading pointer from a receiver type expression.
func exprUnstar(expr ast.Expr) ast.Expr {
	if star, ok := expr.(*ast.StarExpr); ok {
		return star.X
	}
	return expr
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

	// §40.45 round-eight #2–#5 (type-set closure). EVOLUTION RECORD: on
	// 154b1a5c5 every shape below reported 0 offenders (overlay probe) —
	// the round-seven classifier keyed on SPELLING (a constraint's own
	// declaration, the mutation type name in a signature) and the binder
	// followed calls only to FuncDecls/func literals whose declared result
	// named the mutation. The census now closes over alias/defined chains
	// (#2), generic Apply interfaces after substitution (#3), carrying and
	// inferred type parameters in result position (#4), func-typed carriers
	// (#5), and fails loud on every mutation-mentioning type expression or
	// call-receiver Apply it cannot classify.
	t.Run("self_red_alias_of_carrying_named_constraint", func(t *testing.T) {
		expectProbe(t, probe("type mutC interface{ ~[]types.AnswerDocumentMutation }\n\ntype mutC2 = mutC\n\nfunc launder[M mutC2](prev *types.AnswerDocumentV2, ms M) { _, _ = ms[0].Apply(prev) }"),
			"internal/orchestrator/zz_probe.go:launder", "applies a mutation-typed expression")
	})
	t.Run("self_red_defined_type_over_carrying_named_constraint", func(t *testing.T) {
		expectProbe(t, probe("type mutC interface{ ~[]types.AnswerDocumentMutation }\n\ntype mutC2 mutC\n\nfunc launder[M mutC2](prev *types.AnswerDocumentV2, ms M) { _, _ = ms[0].Apply(prev) }"),
			"internal/orchestrator/zz_probe.go:launder", "applies a mutation-typed expression")
	})
	t.Run("self_red_generic_type_over_aliased_constraint_fail_loud", func(t *testing.T) {
		expectProbe(t, probe("type mutC interface{ ~[]types.AnswerDocumentMutation }\n\ntype mutC2 = mutC\n\ntype box[M mutC2] struct{ ms M }"),
			"declares a generic type whose constraint carries the mutation")
	})
	t.Run("self_red_generic_applier_instantiated_with_document_in_constraint", func(t *testing.T) {
		expectProbe(t, probe("type applier[D any] interface{ Apply(prev *D) (*D, error) }\n\nfunc launder[M applier[types.AnswerDocumentV2]](prev *types.AnswerDocumentV2, m M) (*types.AnswerDocumentV2, error) { return m.Apply(prev) }"),
			"internal/orchestrator/zz_probe.go:launder", "instantiates a generic interface whose Apply method set, after substitution", "applies the mutation m")
	})
	t.Run("self_red_generic_applier_instantiated_as_parameter_type", func(t *testing.T) {
		expectProbe(t, probe("type applier[D any] interface{ Apply(prev *D) (*D, error) }\n\nfunc launder(prev *types.AnswerDocumentV2, a applier[types.AnswerDocumentV2]) { _, _ = a.Apply(prev) }"),
			"internal/orchestrator/zz_probe.go:launder", "instantiates a generic interface whose Apply method set, after substitution")
	})
	t.Run("self_red_generic_applier_instantiation_aliased", func(t *testing.T) {
		expectProbe(t, probe("type applier[D any] interface{ Apply(prev *D) (*D, error) }\n\ntype docApplier = applier[types.AnswerDocumentV2]"),
			"internal/orchestrator/zz_probe.go:<package scope>", "instantiates a generic interface whose Apply method set, after substitution")
	})
	t.Run("self_red_generic_applier_instantiated_with_enclosing_type_param_fail_loud", func(t *testing.T) {
		expectProbe(t, probe("type applier[D any] interface{ Apply(prev *D) (*D, error) }\n\nfunc launder[D any, M applier[D]](prev *D, m M) { _, _ = m.Apply(prev) }"),
			"instantiates a generic Apply interface the census cannot resolve")
	})
	t.Run("self_red_generic_applier_reexported_under_new_type_params_fail_loud", func(t *testing.T) {
		expectProbe(t, probe("type applier[D any] interface{ Apply(prev *D) (*D, error) }\n\ntype reexport[E any] interface{ applier[E] }"),
			"a generic type re-exporting a generic Apply interface")
	})
	t.Run("self_red_inline_apply_over_enclosing_type_param_fail_loud", func(t *testing.T) {
		expectProbe(t, probe("func launder[D any, M interface{ Apply(*D) (*D, error) }](prev *D, m M) { _, _ = m.Apply(prev) }"),
			"declares an Apply method abstracted over an enclosing type parameter")
	})
	t.Run("self_green_generic_applier_instantiated_without_the_document", func(t *testing.T) {
		got := answerDocumentPatchBaseConstructorCensus(probe("type applier[D any] interface{ Apply(prev *D) (*D, error) }\n\ntype config struct{ n int }\n\nfunc tune(a applier[config], c *config) { _, _ = a.Apply(c) }"), answerDocumentPatchBaseConstructorSites)
		for _, o := range got {
			if strings.Contains(o, "zz_probe.go") {
				t.Fatalf("an Apply interface instantiated without the document must not be reported: %s", o)
			}
		}
	})
	t.Run("self_red_carrying_type_param_in_result_position_chained_apply", func(t *testing.T) {
		expectProbe(t, probe("type mutC interface{ types.AnswerDocumentMutation }\n\nfunc pick[M mutC](ms []M) M { return ms[0] }\n\nfunc probe(prev *types.AnswerDocumentV2, ms []types.AnswerDocumentMutation) { _, _ = pick(ms).Apply(prev) }"),
			"internal/orchestrator/zz_probe.go:probe", "applies a mutation-typed expression")
	})
	t.Run("self_red_carrying_type_param_in_result_position_bound_apply", func(t *testing.T) {
		expectProbe(t, probe("type mutC interface{ ~types.AnswerDocumentMutation }\n\nfunc pick[M mutC](ms []M) M { return ms[0] }\n\nfunc probe(prev *types.AnswerDocumentV2, ms []types.AnswerDocumentMutation) { m := pick(ms); _, _ = m.Apply(prev) }"),
			"internal/orchestrator/zz_probe.go:probe", "applies the mutation m")
	})
	t.Run("self_red_any_type_param_result_inferred_from_bound_argument", func(t *testing.T) {
		expectProbe(t, probe("func first[T any](xs []T) T { return xs[0] }\n\nfunc probe(prev *types.AnswerDocumentV2, ms []types.AnswerDocumentMutation) { _, _ = first(ms).Apply(prev) }"),
			"internal/orchestrator/zz_probe.go:probe", "applies a mutation-typed expression")
	})
	t.Run("self_red_any_type_param_multi_result_inferred", func(t *testing.T) {
		expectProbe(t, probe("func pop[T any](xs []T) (T, []T) { return xs[0], xs[1:] }\n\nfunc probe(prev *types.AnswerDocumentV2, ms []types.AnswerDocumentMutation) { m, rest := pop(ms); _, _ = m.Apply(prev); _, _ = rest[0].Apply(prev) }"),
			"applies the mutation m", "applies a mutation-typed expression")
	})
	t.Run("self_green_any_type_param_result_without_a_bound_argument", func(t *testing.T) {
		got := answerDocumentPatchBaseConstructorCensus(probe("type policy struct{ n int }\n\nfunc (p policy) Apply(x int) int { return x + p.n }\n\nfunc first[T any](xs []T) T { return xs[0] }\n\nfunc probe(ps []policy) int { return first(ps).Apply(1) }"), answerDocumentPatchBaseConstructorSites)
		for _, o := range got {
			if strings.Contains(o, "zz_probe.go") {
				t.Fatalf("a generic result inferred from a mutation-free argument must not be reported: %s", o)
			}
		}
	})
	t.Run("self_red_foreign_call_result_applied_fail_loud", func(t *testing.T) {
		expectProbe(t, probe("import \"slices\"\n\nfunc probe(prev *types.AnswerDocumentV2, ms []types.AnswerDocumentMutation) { _, _ = slices.Clone(ms)[0].Apply(prev) }"),
			"internal/orchestrator/zz_probe.go:probe", "applies the result of a call the census cannot classify (Clone)")
	})
	t.Run("self_red_builtin_append_result_applied", func(t *testing.T) {
		expectProbe(t, probe("func probe(prev *types.AnswerDocumentV2, ms []types.AnswerDocumentMutation, m types.AnswerDocumentMutation) { _, _ = append(ms, m)[0].Apply(prev) }"),
			"applies a mutation-typed expression")
	})
	t.Run("self_green_tree_function_result_applied_without_the_mutation", func(t *testing.T) {
		got := answerDocumentPatchBaseConstructorCensus(probe("type policy struct{ n int }\n\nfunc (p policy) Apply(x int) int { return x + p.n }\n\nfunc newPolicy(ms []types.AnswerDocumentMutation) policy { return policy{n: len(ms)} }\n\nfunc probe(ms []types.AnswerDocumentMutation) int { return newPolicy(ms).Apply(1) }"), answerDocumentPatchBaseConstructorSites)
		for _, o := range got {
			if strings.Contains(o, "zz_probe.go") {
				t.Fatalf("a tree function with no mutation result is classified, not failed loud: %s", o)
			}
		}
	})
	t.Run("self_red_func_typed_parameter_called_and_applied", func(t *testing.T) {
		expectProbe(t, probe("func launder(prev *types.AnswerDocumentV2, mk func() types.AnswerDocumentMutation) { _, _ = mk().Apply(prev) }"),
			"internal/orchestrator/zz_probe.go:launder", "applies a mutation-typed expression")
	})
	t.Run("self_red_func_typed_struct_field_called_and_applied", func(t *testing.T) {
		expectProbe(t, probe("type holder struct{ mk func() types.AnswerDocumentMutation }\n\nfunc probe(prev *types.AnswerDocumentV2, h holder) { _, _ = h.mk().Apply(prev) }"),
			"internal/orchestrator/zz_probe.go:probe", "applies a mutation-typed expression")
	})
	t.Run("self_red_func_typed_closure_variable_copy", func(t *testing.T) {
		expectProbe(t, probe("func probe(prev *types.AnswerDocumentV2, mk func() types.AnswerDocumentMutation) { f := mk; _, _ = f().Apply(prev) }"),
			"internal/orchestrator/zz_probe.go:probe", "applies a mutation-typed expression")
	})
	t.Run("self_red_func_typed_call_result_bound", func(t *testing.T) {
		expectProbe(t, probe("func probe(prev *types.AnswerDocumentV2, mk func() (types.AnswerDocumentMutation, error)) { m, _ := mk(); _, _ = m.Apply(prev) }"),
			"applies the mutation m")
	})
	t.Run("self_red_func_typed_package_level_var", func(t *testing.T) {
		expectProbe(t, probe("var mk func() types.AnswerDocumentMutation\n\nfunc probe(prev *types.AnswerDocumentV2) { _, _ = mk().Apply(prev) }"),
			"internal/orchestrator/zz_probe.go:probe", "applies a mutation-typed expression")
	})
	t.Run("self_red_named_func_type_carrier", func(t *testing.T) {
		expectProbe(t, probe("type mkFn func() types.AnswerDocumentMutation\n\nfunc probe(prev *types.AnswerDocumentV2, mk mkFn) { _, _ = mk().Apply(prev) }"),
			"internal/orchestrator/zz_probe.go:probe", "applies a mutation-typed expression")
	})
	t.Run("self_red_mutation_returning_funcdecl_as_value", func(t *testing.T) {
		expectProbe(t, probe("func mint() types.AnswerDocumentMutation {\n\tvar m types.AnswerDocumentMutation\n\treturn m\n}\n\nfunc probe(prev *types.AnswerDocumentV2) { f := mint; _, _ = f().Apply(prev) }"),
			"internal/orchestrator/zz_probe.go:probe", "applies a mutation-typed expression")
	})
	t.Run("self_red_func_typed_parameter_position_fail_loud", func(t *testing.T) {
		expectProbe(t, probe("func probe(visit func(types.AnswerDocumentMutation)) { _ = visit }"),
			"internal/orchestrator/zz_probe.go:probe", "a func type that receives the mutation as a parameter")
	})
	t.Run("self_red_func_literal_parameter_bound", func(t *testing.T) {
		expectProbe(t, probe("func probe(prev *types.AnswerDocumentV2, ms []types.AnswerDocumentMutation) {\n\teach := func(m types.AnswerDocumentMutation) { _, _ = m.Apply(prev) }\n\teach(ms[0])\n}"),
			"applies the mutation m")
	})
	t.Run("self_red_container_of_func_carriers_fail_loud", func(t *testing.T) {
		expectProbe(t, probe("func probe(prev *types.AnswerDocumentV2, mks []func() types.AnswerDocumentMutation) { _, _ = mks[0]().Apply(prev) }"),
			"a container of func carriers")
	})
	t.Run("self_red_chan_of_mutation_fail_loud", func(t *testing.T) {
		expectProbe(t, probe("type feed struct{ ch chan types.AnswerDocumentMutation }"),
			"a chan carrying the mutation")
	})
	t.Run("self_red_generic_instantiated_with_mutation_as_value_type_fail_loud", func(t *testing.T) {
		expectProbe(t, probe("type bag[T any] struct{ items []T }\n\nfunc probe(prev *types.AnswerDocumentV2, b bag[types.AnswerDocumentMutation]) { _ = b }"),
			"a generic type instantiated with the mutation as a type argument")
	})
	t.Run("self_red_factory_closure_second_base", func(t *testing.T) {
		// The end-to-end launder: the registered site hands its bound
		// mutation to a factory closure, a sibling helper applies the
		// closure's result — a second base outside the registered sites.
		mutated := tree.withInjected(t, patchFile,
			"merged, mutation, applyErr := buildAnswerDocumentPatchBase(prev, patch)\n",
			"merged, mutation, applyErr := buildAnswerDocumentPatchBase(prev, patch)\n\tzzLaunder(prev, func() types.AnswerDocumentMutation { return mutation })\n").
			with(t, "internal/tool/zz_probe.go", "package tool\n\nimport \"github.com/hanchaoqun/codrax/internal/types\"\n\nfunc zzLaunder(prev *types.AnswerDocumentV2, mk func() types.AnswerDocumentMutation) { _, _ = mk().Apply(prev) }")
		got := answerDocumentPatchBaseConstructorCensus(mutated, answerDocumentPatchBaseConstructorSites)
		retryGenerationExpectOffender(t, got, "internal/tool/zz_probe.go:zzLaunder")
	})

	// §40.45 round-nine #5–#8. EVOLUTION RECORD: on 49efc4a2e every shape
	// below reported 0 offenders (overlay probes) — the round-eight binder
	// instantiated a generic's type parameters only from mutation VALUE
	// arguments (#5: a func-typed argument's result slot never bound, and a
	// generic returning `func() T` bound as a mutation), classified make/new
	// as nothing and left an unclassifiable RHS silently unbound (#6),
	// failed loud on an unclassifiable call only in the chained-receiver form
	// (#7: `sorted := slices.Clone(ms); sorted[0].Apply` escaped), and never
	// consulted methods or fields of generic TYPES (#8: an inferred
	// `newBag(ms).first()` escaped while the explicit spelling failed loud).
	expectGreenProbe := func(t *testing.T, mutated *retryGenerationTree, why string) {
		t.Helper()
		for _, o := range answerDocumentPatchBaseConstructorCensus(mutated, answerDocumentPatchBaseConstructorSites) {
			if strings.Contains(o, "zz_probe.go") {
				t.Fatalf("%s: %s", why, o)
			}
		}
	}
	const parseMutationDecl = "func parseMutation(raw []byte) (types.AnswerDocumentMutation, error) {\n\tvar m types.AnswerDocumentMutation\n\t_ = raw\n\treturn m, nil\n}\n\n"
	const repairDecl = "func repair[T any](raw []byte, parse func([]byte) (T, error)) (T, error) { return parse(raw) }\n\n"
	const mintDecl = "func mint() types.AnswerDocumentMutation {\n\tvar m types.AnswerDocumentMutation\n\treturn m\n}\n\n"
	const lazyDecl = "func lazy[T any](x T) func() T { return func() T { return x } }\n\n"
	t.Run("self_red_generic_inferred_from_funcdecl_value_argument", func(t *testing.T) {
		expectProbe(t, probe(parseMutationDecl+repairDecl+"func probe(prev *types.AnswerDocumentV2, raw []byte) { m, _ := repair(raw, parseMutation); _, _ = m.Apply(prev) }"),
			"internal/orchestrator/zz_probe.go:probe", "applies the mutation m")
	})
	t.Run("self_red_generic_inferred_from_func_literal_argument", func(t *testing.T) {
		expectProbe(t, probe(repairDecl+"func probe(prev *types.AnswerDocumentV2, raw []byte) {\n\tm, _ := repair(raw, func(b []byte) (types.AnswerDocumentMutation, error) {\n\t\tvar mm types.AnswerDocumentMutation\n\t\treturn mm, nil\n\t})\n\t_, _ = m.Apply(prev)\n}"),
			"internal/orchestrator/zz_probe.go:probe", "applies the mutation m")
	})
	t.Run("self_red_generic_inferred_from_bound_func_carrier_parameter", func(t *testing.T) {
		expectProbe(t, probe(repairDecl+"func probe(prev *types.AnswerDocumentV2, raw []byte, parse func([]byte) (types.AnswerDocumentMutation, error)) { m, _ := repair(raw, parse); _, _ = m.Apply(prev) }"),
			"internal/orchestrator/zz_probe.go:probe", "applies the mutation m")
	})
	t.Run("self_red_generic_inferred_from_local_func_value", func(t *testing.T) {
		expectProbe(t, probe(parseMutationDecl+repairDecl+"func probe(prev *types.AnswerDocumentV2, raw []byte) { p := parseMutation; m, _ := repair(raw, p); _, _ = m.Apply(prev) }"),
			"internal/orchestrator/zz_probe.go:probe", "applies the mutation m")
	})
	t.Run("self_red_generic_single_result_inferred_from_func_argument", func(t *testing.T) {
		expectProbe(t, probe(mintDecl+"func call1[T any](mk func() T) T { return mk() }\n\nfunc probe(prev *types.AnswerDocumentV2) { _, _ = call1(mint).Apply(prev) }"),
			"internal/orchestrator/zz_probe.go:probe", "applies a mutation-typed expression")
	})
	t.Run("self_red_generic_returning_func_carrier_chained", func(t *testing.T) {
		expectProbe(t, probe(lazyDecl+"func probe(prev *types.AnswerDocumentV2, m types.AnswerDocumentMutation) { _, _ = lazy(m)().Apply(prev) }"),
			"internal/orchestrator/zz_probe.go:probe", "applies a mutation-typed expression")
	})
	t.Run("self_red_generic_returning_func_carrier_bound", func(t *testing.T) {
		expectProbe(t, probe(lazyDecl+"func probe(prev *types.AnswerDocumentV2, m types.AnswerDocumentMutation) { f := lazy(m); _, _ = f().Apply(prev) }"),
			"internal/orchestrator/zz_probe.go:probe", "applies a mutation-typed expression")
	})
	t.Run("self_red_generic_result_unrecognized_after_inference_fail_loud", func(t *testing.T) {
		expectProbe(t, probe("func chanOf[T any](x T) chan T {\n\tch := make(chan T, 1)\n\tch <- x\n\treturn ch\n}\n\nfunc probe(m types.AnswerDocumentMutation) { ch := chanOf(m); _ = ch }"),
			"internal/orchestrator/zz_probe.go:probe", "a generic call whose result, with its type parameter instantiated by the mutation, is a chan carrying the mutation")
	})
	t.Run("self_red_make_slice_append_index_apply", func(t *testing.T) {
		expectProbe(t, probe("func probe(prev *types.AnswerDocumentV2, m types.AnswerDocumentMutation) {\n\tms := make([]types.AnswerDocumentMutation, 0, 1)\n\tms = append(ms, m)\n\t_, _ = ms[0].Apply(prev)\n}"),
			"internal/orchestrator/zz_probe.go:probe", "applies a mutation-typed expression")
	})
	t.Run("self_red_make_map_store_index_apply", func(t *testing.T) {
		expectProbe(t, probe("func probe(prev *types.AnswerDocumentV2, m types.AnswerDocumentMutation) {\n\tmm := make(map[string]types.AnswerDocumentMutation)\n\tmm[\"a\"] = m\n\t_, _ = mm[\"a\"].Apply(prev)\n}"),
			"internal/orchestrator/zz_probe.go:probe", "applies a mutation-typed expression")
	})
	t.Run("self_red_new_mutation_pointer_apply", func(t *testing.T) {
		expectProbe(t, probe("func probe(prev *types.AnswerDocumentV2, m types.AnswerDocumentMutation) {\n\tp := new(types.AnswerDocumentMutation)\n\t*p = m\n\t_, _ = p.Apply(prev)\n}"),
			"internal/orchestrator/zz_probe.go:probe", "applies the mutation p")
	})
	t.Run("self_red_make_chan_of_mutation_fail_loud", func(t *testing.T) {
		expectProbe(t, probe("func probe(prev *types.AnswerDocumentV2, m types.AnswerDocumentMutation) {\n\tch := make(chan types.AnswerDocumentMutation, 1)\n\tch <- m\n\tx := <-ch\n\t_, _ = x.Apply(prev)\n}"),
			"internal/orchestrator/zz_probe.go:probe", "make of a chan carrying the mutation", "applies the result of a call the census cannot classify (make)")
	})
	t.Run("self_red_make_map_of_func_carriers_fail_loud", func(t *testing.T) {
		expectProbe(t, probe(mintDecl+"func probe(prev *types.AnswerDocumentV2) {\n\tfactories := make(map[string]func() types.AnswerDocumentMutation)\n\tfactories[\"p\"] = mint\n\t_, _ = factories[\"p\"]().Apply(prev)\n}"),
			"internal/orchestrator/zz_probe.go:probe", "make of a map of func carriers")
	})
	t.Run("self_red_foreign_call_result_bound_then_applied", func(t *testing.T) {
		expectProbe(t, probe("import \"slices\"\n\nfunc probe(prev *types.AnswerDocumentV2, ms []types.AnswerDocumentMutation) { sorted := slices.Clone(ms); _, _ = sorted[0].Apply(prev) }"),
			"internal/orchestrator/zz_probe.go:probe", "applies the result of a call the census cannot classify (Clone)")
	})
	t.Run("self_red_foreign_call_result_ranged_and_applied", func(t *testing.T) {
		expectProbe(t, probe("import \"slices\"\n\nfunc probe(prev *types.AnswerDocumentV2, ms []types.AnswerDocumentMutation) {\n\tfor _, m := range slices.Clone(ms) {\n\t\t_, _ = m.Apply(prev)\n\t}\n}"),
			"internal/orchestrator/zz_probe.go:probe", "applies the result of a call the census cannot classify (Clone)")
	})
	t.Run("self_red_unclassified_result_copied_then_applied", func(t *testing.T) {
		expectProbe(t, probe("import \"slices\"\n\nfunc probe(prev *types.AnswerDocumentV2, ms []types.AnswerDocumentMutation) { sorted := slices.Clone(ms); again := sorted; _, _ = again[0].Apply(prev) }"),
			"internal/orchestrator/zz_probe.go:probe", "applies the result of a call the census cannot classify (Clone)")
	})
	t.Run("self_red_unclassified_argument_to_generic_result_applied", func(t *testing.T) {
		expectProbe(t, probe("import \"slices\"\n\nfunc first[T any](xs []T) T { return xs[0] }\n\nfunc probe(prev *types.AnswerDocumentV2, ms []types.AnswerDocumentMutation) { sorted := slices.Clone(ms); _, _ = first(sorted).Apply(prev) }"),
			"internal/orchestrator/zz_probe.go:probe", "applies the result of a call the census cannot classify (first)")
	})
	t.Run("self_red_undefined_method_on_bound_receiver_applied", func(t *testing.T) {
		expectProbe(t, probe("func probe(prev *types.AnswerDocumentV2, m types.AnswerDocumentMutation) { _, _ = m.zzUndefined().Apply(prev) }"),
			"internal/orchestrator/zz_probe.go:probe", "applies the result of a call the census cannot classify (zzUndefined)")
	})
	t.Run("self_green_foreign_call_result_never_applied", func(t *testing.T) {
		expectGreenProbe(t, probe("import \"encoding/json\"\n\nfunc probe(m types.AnswerDocumentMutation) []byte { b, _ := json.Marshal(m); return b }"),
			"an unclassifiable call result that is never applied must not be reported")
	})
	const bagDecl = "type bag[T any] struct{ items []T }\n\nfunc newBag[T any](xs []T) bag[T] { return bag[T]{items: xs} }\n\n"
	t.Run("self_red_generic_container_method_result_chained_apply", func(t *testing.T) {
		expectProbe(t, probe(bagDecl+"func (b bag[T]) first() T { return b.items[0] }\n\nfunc probe(prev *types.AnswerDocumentV2, ms []types.AnswerDocumentMutation) { _, _ = newBag(ms).first().Apply(prev) }"),
			"internal/orchestrator/zz_probe.go:probe", "applies a mutation-typed expression")
	})
	t.Run("self_red_generic_container_method_result_bound_apply", func(t *testing.T) {
		expectProbe(t, probe(bagDecl+"func (b bag[T]) first() T { return b.items[0] }\n\nfunc probe(prev *types.AnswerDocumentV2, ms []types.AnswerDocumentMutation) { b := newBag(ms); m := b.first(); _, _ = m.Apply(prev) }"),
			"internal/orchestrator/zz_probe.go:probe", "applies the mutation m")
	})
	t.Run("self_red_generic_container_pointer_receiver_method_apply", func(t *testing.T) {
		expectProbe(t, probe(bagDecl+"func (b *bag[T]) first() T { return b.items[0] }\n\nfunc probe(prev *types.AnswerDocumentV2, ms []types.AnswerDocumentMutation) { b := newBag(ms); _, _ = b.first().Apply(prev) }"),
			"internal/orchestrator/zz_probe.go:probe", "applies a mutation-typed expression")
	})
	t.Run("self_red_generic_container_field_apply", func(t *testing.T) {
		expectProbe(t, probe(bagDecl+"func probe(prev *types.AnswerDocumentV2, ms []types.AnswerDocumentMutation) { b := newBag(ms); _, _ = b.items[0].Apply(prev) }"),
			"internal/orchestrator/zz_probe.go:probe", "applies a mutation-typed expression")
	})
	t.Run("self_red_generic_container_passed_to_generic_function_apply", func(t *testing.T) {
		expectProbe(t, probe(bagDecl+"func (b bag[T]) first() T { return b.items[0] }\n\nfunc drain[T any](b bag[T]) T { return b.first() }\n\nfunc probe(prev *types.AnswerDocumentV2, ms []types.AnswerDocumentMutation) { _, _ = drain(newBag(ms)).Apply(prev) }"),
			"internal/orchestrator/zz_probe.go:probe", "applies a mutation-typed expression")
	})
	t.Run("self_red_generic_container_undefined_method_fail_loud", func(t *testing.T) {
		expectProbe(t, probe(bagDecl+"func probe(prev *types.AnswerDocumentV2, ms []types.AnswerDocumentMutation) { b := newBag(ms); _, _ = b.zzUndefined().Apply(prev) }"),
			"internal/orchestrator/zz_probe.go:probe", "applies the result of a call the census cannot classify (zzUndefined)")
	})
	t.Run("self_red_explicit_instantiation_with_the_mutation_fail_loud", func(t *testing.T) {
		expectProbe(t, probe(bagDecl+"func probe(ms []types.AnswerDocumentMutation) { b := newBag[types.AnswerDocumentMutation](ms); _ = b }"),
			"internal/orchestrator/zz_probe.go:probe", "calls a generic function explicitly instantiated with the mutation as a type argument")
	})
	t.Run("self_green_generic_container_without_the_mutation", func(t *testing.T) {
		expectGreenProbe(t, probe("type policy struct{ n int }\n\nfunc (p policy) Apply(x int) int { return x + p.n }\n\n"+bagDecl+"func (b bag[T]) first() T { return b.items[0] }\n\nfunc probe(ps []policy) int { return newBag(ps).first().Apply(1) }"),
			"a generic container instantiated without the mutation must not be reported")
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
