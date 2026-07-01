package types

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// TestProducerPrecedenceNotReimplementedInline is the structural guard for the
// runtime-artifact producer-precedence chokepoint (gap 2/5). The producer
// family — "trace_query" / "perf_trace" / "emit_perf_trace" — is base-normalized
// by RuntimeObservationProducerIsDeterministicQuery / runtimeObservationProducer-
// IsPreTriage so run-suffixed ids like "trace_query:run2" are classified
// correctly. An inline comparison of a Producer field against one of those
// literals silently MISSES run-suffixed ids — exactly the divergence bug that
// had three channels disagree on whether a record was a trace_query observation.
//
// This lint fails the build if any non-test .go file (outside the chokepoint
// home files) compares against a producer-family literal with == / != or
// strings.EqualFold. The only way to extend the family is to add the id to the
// chokepoint switch (observation_ledger.go); never silence a site by name.
func TestProducerPrecedenceNotReimplementedInline(t *testing.T) {
	// Packages that classify observation records. The test working dir is the
	// types package dir, so siblings are reached via "../".
	dirs := []string{".", "../agent", "../tool"}
	fset := token.NewFileSet()
	var violations []string
	for _, dir := range dirs {
		_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			if signalChokepointHomeFiles[filepath.Base(path)] {
				return nil
			}
			f, perr := parser.ParseFile(fset, path, nil, 0)
			if perr != nil {
				return nil
			}
			violations = append(violations, findProducerPrecedenceReimplementations(f, fset, path)...)
			return nil
		})
	}
	if len(violations) > 0 {
		t.Fatalf("inline producer-precedence re-implementation(s) found — these silently miss run-suffixed ids like trace_query:run2; route through RuntimeObservationProducerIsDeterministicQuery / runtimeObservationProducerIsPreTriage instead of comparing a Producer against a family literal:\n  %s", strings.Join(violations, "\n  "))
	}
}

// signalChokepointHomeFiles legitimately define/own the producer-family
// classification (the chokepoint switch and the producer/origin registry).
var signalChokepointHomeFiles = map[string]bool{
	"observation_ledger.go":     true, // RuntimeObservationProducer* chokepoint
	"answer_evidence_origin.go": true, // producer / origin classification registry
}

// producerFamilyLiterals are the runtime-artifact producer ids owned by the
// chokepoint. A bare comparison against any of them is a re-implementation.
var producerFamilyLiterals = map[string]bool{
	"trace_query":     true,
	"perf_trace":      true,
	"emit_perf_trace": true,
}

// findProducerPrecedenceReimplementations reports every == / != comparison and
// strings.EqualFold call that compares an observation Producer FIELD against a
// producer-family literal. It is deliberately keyed on the operand referencing a
// `.Producer` selector — NOT on the literal alone — so legitimate tool-name
// dispatch (CanonicalToolName(x.ToolName) == "trace_query", which is always the
// exact tool name and never run-suffixed), field-sets, case clauses, and slice
// membership are NOT flagged. Only Producer-precedence classification is.
func findProducerPrecedenceReimplementations(f *ast.File, fset *token.FileSet, path string) []string {
	var out []string
	ast.Inspect(f, func(n ast.Node) bool {
		switch e := n.(type) {
		case *ast.BinaryExpr:
			if e.Op != token.EQL && e.Op != token.NEQ {
				return true
			}
			if isProducerFamilyLiteral(e.X) && exprReferencesProducerField(e.Y) ||
				isProducerFamilyLiteral(e.Y) && exprReferencesProducerField(e.X) {
				out = append(out, path+":"+strconv.Itoa(fset.Position(n.Pos()).Line))
			}
		case *ast.CallExpr:
			if !isStringsEqualFold(e) {
				return true
			}
			hasLit, hasProducer := false, false
			for _, arg := range e.Args {
				if isProducerFamilyLiteral(arg) {
					hasLit = true
				}
				if exprReferencesProducerField(arg) {
					hasProducer = true
				}
			}
			if hasLit && hasProducer {
				out = append(out, path+":"+strconv.Itoa(fset.Position(n.Pos()).Line))
			}
		}
		return true
	})
	return out
}

// exprReferencesProducerField reports whether e (an operand subtree) reads an
// observation Producer field — directly (record.Producer) or wrapped
// (strings.TrimSpace(record.Producer)).
func exprReferencesProducerField(e ast.Expr) bool {
	found := false
	ast.Inspect(e, func(n ast.Node) bool {
		if sel, ok := n.(*ast.SelectorExpr); ok && sel.Sel != nil && sel.Sel.Name == "Producer" {
			found = true
			return false
		}
		return true
	})
	return found
}

func isProducerFamilyLiteral(e ast.Expr) bool {
	lit, ok := e.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return false
	}
	v, err := strconv.Unquote(lit.Value)
	if err != nil {
		return false
	}
	return producerFamilyLiterals[v]
}

func isStringsEqualFold(e *ast.CallExpr) bool {
	sel, ok := e.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	return ok && pkg.Name == "strings" && sel.Sel.Name == "EqualFold"
}

// TestProducerPrecedenceLintDetectsViolations self-validates the detector: it
// must fire on a synthetic re-implementation and stay quiet on legitimate uses
// (field-set, case clause, slice membership) so the guard above cannot silently
// become a no-op.
func TestProducerPrecedenceLintDetectsViolations(t *testing.T) {
	bad := `package p
import "strings"
func a(r struct{ Producer string }) bool { return r.Producer == "trace_query" }
func b(r struct{ Producer string }) bool { return r.Producer != "perf_trace" }
func c(r struct{ Producer string }) bool { return strings.EqualFold(r.Producer, "trace_query") }
`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "bad.go", bad, 0)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got := findProducerPrecedenceReimplementations(f, fset, "bad.go"); len(got) != 3 {
		t.Fatalf("detector must flag all 3 synthetic re-implementations, got %d: %v", len(got), got)
	}

	good := `package p
import "strings"
type R struct{ Producer string }
func mk() R { return R{Producer: "trace_query"} }
func sw(s string) bool { switch s { case "trace_query", "perf_trace": return true }; return false }
var set = []string{"trace_query", "perf_trace", "emit_perf_trace"}
func art(k string) bool { return k == "trace" }
func tool(name string) bool { return name == "trace_query" }
func toolField(r struct{ ToolName string }) bool { return r.ToolName == "emit_perf_trace" }
func toolFold(r struct{ ToolName string }) bool { return strings.EqualFold(r.ToolName, "trace_query") }
`
	f2, err := parser.ParseFile(fset, "good.go", good, 0)
	if err != nil {
		t.Fatalf("parse good: %v", err)
	}
	if got := findProducerPrecedenceReimplementations(f2, fset, "good.go"); len(got) != 0 {
		t.Fatalf("detector must NOT flag field-set / case / slice / non-family comparisons, got %v", got)
	}
}

// TestRuntimeSourceAuthorityLegacyHelpersStayInFallbackChokepoints guards the
// runtime/current-source authority cutover. The old RequestModel helpers
// CurrentSourceLaneDecision().RequiresCurrentSource() and
// RequiresCurrentSourceForExternalObservation() are still useful as compatibility
// fallback inside a few named chokepoints, but new production consumers must not
// make hard-gate or scheduling decisions from those helpers directly. They must
// consume RuntimeSourceAnswerAuthoritySnapshot (or a helper built on it) so
// soft obligations downgrade to bounded caveats while precise obligations remain
// load-bearing.
func TestRuntimeSourceAuthorityLegacyHelpersStayInFallbackChokepoints(t *testing.T) {
	dirs := runtimeSourcePolicyProductionDirs(t)
	fset := token.NewFileSet()
	var violations []string
	for _, dir := range dirs {
		_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			f, perr := parser.ParseFile(fset, path, nil, 0)
			if perr != nil {
				return nil
			}
			violations = append(violations, findRuntimeSourceLegacyHelperBypasses(f, fset, path)...)
			return nil
		})
	}
	if len(violations) > 0 {
		t.Fatalf("runtime/source current-source legacy helper used outside an authority fallback chokepoint — route through RuntimeSourceAnswerAuthoritySnapshot or an existing authority helper instead:\n  %s", strings.Join(violations, "\n  "))
	}
}

var runtimeSourceLegacyHelperFallbackChokepoints = map[string]bool{}

func runtimeSourcePolicyProductionDirs(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir("..")
	if err != nil {
		t.Fatalf("read internal dirs: %v", err)
	}
	skip := map[string]bool{
		"thirdparty": true,
		"types":      true,
	}
	var dirs []string
	for _, entry := range entries {
		if !entry.IsDir() || skip[entry.Name()] {
			continue
		}
		dirs = append(dirs, filepath.ToSlash(filepath.Join("..", entry.Name())))
	}
	sort.Strings(dirs)
	return dirs
}

func findRuntimeSourceLegacyHelperBypasses(f *ast.File, fset *token.FileSet, path string) []string {
	var out []string
	ast.Inspect(f, func(n ast.Node) bool {
		fn, ok := n.(*ast.FuncDecl)
		if !ok || fn.Body == nil || fn.Name == nil {
			return true
		}
		ast.Inspect(fn.Body, func(m ast.Node) bool {
			call, ok := m.(*ast.CallExpr)
			if !ok || !isRuntimeSourceLegacyHelperCall(call) {
				return true
			}
			key := filepath.ToSlash(path) + "::" + fn.Name.Name
			if runtimeSourceLegacyHelperFallbackChokepoints[key] {
				return true
			}
			out = append(out, key+":"+strconv.Itoa(fset.Position(call.Pos()).Line))
			return true
		})
		return true
	})
	return out
}

func isRuntimeSourceLegacyHelperCall(call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel == nil {
		return false
	}
	switch sel.Sel.Name {
	case "RequiresCurrentSourceForExternalObservation":
		return true
	case "RequiresCurrentSource":
		return true
	default:
		return false
	}
}

func TestRuntimeSourceAuthorityLegacyHelperLintDetectsViolations(t *testing.T) {
	src := `package p
func bad(r interface{ RequiresCurrentSourceForExternalObservation(any) bool }) bool {
	return r.RequiresCurrentSourceForExternalObservation(nil)
}
func bad2(r interface{ CurrentSourceLaneDecision() interface{ RequiresCurrentSource() bool } }) bool {
	return r.CurrentSourceLaneDecision().RequiresCurrentSource()
}
func bad3(l interface{ RequiresCurrentSource() bool }) bool {
	return l.RequiresCurrentSource()
}
`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "bad.go", src, 0)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got := findRuntimeSourceLegacyHelperBypasses(f, fset, "bad.go"); len(got) != 3 {
		t.Fatalf("detector must flag all legacy helper calls, got %d: %v", len(got), got)
	}

	allowed := `package p
func currentSourceRequirementPrecise(s struct{ CurrentSourceRequirement string }) bool {
	return s.CurrentSourceRequirement == "precise"
}
func carrier(s struct{ CanHardBlockCompletion bool }) bool {
	return s.CanHardBlockCompletion
}
`
	f2, err := parser.ParseFile(fset, "good.go", allowed, 0)
	if err != nil {
		t.Fatalf("parse good: %v", err)
	}
	if got := findRuntimeSourceLegacyHelperBypasses(f2, fset, "good.go"); len(got) != 0 {
		t.Fatalf("detector must not flag authority field consumers, got %v", got)
	}
}

func TestRuntimeSourceAnswerContractBuildersUseAuthorityPrecision(t *testing.T) {
	paths := []string{"exact_lookup.go", "answer_required_anchor.go"}
	fset := token.NewFileSet()
	var violations []string
	for _, path := range paths {
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		violations = append(violations, findRuntimeSourceAnswerContractPolicyBypasses(f, fset, path)...)
	}
	if len(violations) > 0 {
		t.Fatalf("types-level exact/anchor contract builders must consume RuntimeSourceRequestSuppressesCurrentSourceAnswerContract instead of local runtime/source policy:\n  %s", strings.Join(violations, "\n  "))
	}
}

func findRuntimeSourceAnswerContractPolicyBypasses(f *ast.File, fset *token.FileSet, path string) []string {
	targetFns := map[string]bool{
		"BuildExactResolutionContract":    true,
		"requiredMechanismAnchorsEnabled": true,
	}
	var out []string
	ast.Inspect(f, func(n ast.Node) bool {
		fn, ok := n.(*ast.FuncDecl)
		if !ok || fn.Body == nil || fn.Name == nil || !targetFns[fn.Name.Name] {
			return true
		}
		ast.Inspect(fn.Body, func(m ast.Node) bool {
			call, ok := m.(*ast.CallExpr)
			if !ok || !isRuntimeSourceAnswerContractLocalPolicyCall(call) {
				return true
			}
			out = append(out, filepath.ToSlash(path)+"::"+fn.Name.Name+":"+strconv.Itoa(fset.Position(call.Pos()).Line))
			return true
		})
		return true
	})
	return out
}

func isRuntimeSourceAnswerContractLocalPolicyCall(call *ast.CallExpr) bool {
	if isRuntimeSourceLegacyHelperCall(call) {
		return true
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel == nil {
		return false
	}
	switch sel.Sel.Name {
	case "HasRuntimeArtifactWithoutRequiredCurrentSource",
		"HasRuntimeArtifactWithoutRequiredCurrentSourceInArtifactContext",
		"HasRuntimeArtifactWithoutRequiredCurrentSourceInTraceContext":
		return true
	default:
		return false
	}
}

func TestRuntimeSourceAnswerContractBuilderLintDetectsViolations(t *testing.T) {
	src := `package p
func BuildExactResolutionContract(r interface{ HasRuntimeArtifactWithoutRequiredCurrentSource() bool }) bool {
	return r.HasRuntimeArtifactWithoutRequiredCurrentSource()
}
func requiredMechanismAnchorsEnabled(r interface{ CurrentSourceLaneDecision() interface{ RequiresCurrentSource() bool } }) bool {
	return r.CurrentSourceLaneDecision().RequiresCurrentSource()
}
func unrelated(r interface{ HasRuntimeArtifactWithoutRequiredCurrentSource() bool }) bool {
	return r.HasRuntimeArtifactWithoutRequiredCurrentSource()
}
`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "bad.go", src, 0)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got := findRuntimeSourceAnswerContractPolicyBypasses(f, fset, "bad.go"); len(got) != 2 {
		t.Fatalf("detector must flag exact/anchor builders only, got %d: %v", len(got), got)
	}
}

// TestRuntimeSourceStaticMixedShapeStaysInFallbackChokepoints keeps the legacy
// MixedRuntimeCurrentSourceRequiredFileCoverageShape helper from becoming a new
// sibling authority. It is allowed only as compatibility fallback where the
// shared RuntimeSourceAnswerAuthoritySnapshot cannot yet carry enough context
// to cap required-file reads. New mixed runtime/source consumers must use the
// shared authority carrier helpers instead.
func TestRuntimeSourceStaticMixedShapeStaysInFallbackChokepoints(t *testing.T) {
	dirs := runtimeSourcePolicyProductionDirs(t)
	fset := token.NewFileSet()
	var violations []string
	for _, dir := range dirs {
		_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			f, perr := parser.ParseFile(fset, path, nil, 0)
			if perr != nil {
				return nil
			}
			violations = append(violations, findRuntimeSourceStaticMixedShapeBypasses(f, fset, path)...)
			return nil
		})
	}
	if len(violations) > 0 {
		t.Fatalf("static mixed runtime/source shape used outside an authority fallback chokepoint — route through RuntimeSourceAnswerAuthoritySnapshot carrier helpers instead:\n  %s", strings.Join(violations, "\n  "))
	}
}

var runtimeSourceStaticMixedShapeFallbackChokepoints = map[string]bool{}

func findRuntimeSourceStaticMixedShapeBypasses(f *ast.File, fset *token.FileSet, path string) []string {
	var out []string
	ast.Inspect(f, func(n ast.Node) bool {
		fn, ok := n.(*ast.FuncDecl)
		if !ok || fn.Body == nil || fn.Name == nil {
			return true
		}
		ast.Inspect(fn.Body, func(m ast.Node) bool {
			call, ok := m.(*ast.CallExpr)
			if !ok || !isRuntimeSourceStaticMixedShapeCall(call) {
				return true
			}
			key := filepath.ToSlash(path) + "::" + fn.Name.Name
			if runtimeSourceStaticMixedShapeFallbackChokepoints[key] {
				return true
			}
			out = append(out, key+":"+strconv.Itoa(fset.Position(call.Pos()).Line))
			return true
		})
		return true
	})
	return out
}

func isRuntimeSourceStaticMixedShapeCall(call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	return ok && sel.Sel != nil && sel.Sel.Name == "MixedRuntimeCurrentSourceRequiredFileCoverageShape"
}

func TestRuntimeSourceStaticMixedShapeLintDetectsViolations(t *testing.T) {
	src := `package p
func bad(t interface{ MixedRuntimeCurrentSourceRequiredFileCoverageShape(any) bool }, rm any) bool {
	return t.MixedRuntimeCurrentSourceRequiredFileCoverageShape(rm)
}
`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "bad.go", src, 0)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got := findRuntimeSourceStaticMixedShapeBypasses(f, fset, "bad.go"); len(got) != 1 {
		t.Fatalf("detector must flag static mixed-shape calls, got %d: %v", len(got), got)
	}

	good := `package p
func good(s struct{ HasMixedRuntimeCurrentSourceCarrier bool }) bool {
	return s.HasMixedRuntimeCurrentSourceCarrier
}
`
	f2, err := parser.ParseFile(fset, "good.go", good, 0)
	if err != nil {
		t.Fatalf("parse good: %v", err)
	}
	if got := findRuntimeSourceStaticMixedShapeBypasses(f2, fset, "good.go"); len(got) != 0 {
		t.Fatalf("detector must not flag authority-carrier field consumers, got %v", got)
	}
}
