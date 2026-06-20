package types

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
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
