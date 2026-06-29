package orchestrator

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// TestNoKeywordMatchOfUserIntentInGates is the structural guard for gap 2: a
// hard gate / contract check must read TYPED signals, never classify USER INTENT
// by keyword-matching the raw request against a hardcoded string literal.
//
// The distinction the guard enforces is exactly the project red line: matching
// the request against a TYPED token (an identifier/symbol/expected value bound
// from the analyzer IR) via verbatim substring is a PRECISE signal and is
// allowed; matching it against a STRING LITERAL is keyword-classification of
// intent and is banned. The guard is fully decidable — it only flags a
// strings.{Contains,EqualFold,HasPrefix,HasSuffix,Index,Count} call whose
// haystack is RawRequest-derived AND whose other argument is a string literal.
//
// Sanctioned typed-token sinks (MentionedEntitiesFromRawRequest, etc.) are not
// strings.* calls, so routing a typed candidate through them is never flagged.
// To extend, add a typed sink — never silence a site by name.
func TestNoKeywordMatchOfUserIntentInGates(t *testing.T) {
	// The precise hard-gate / contract-check files. The analyzer
	// (emit_analysis) and prompt builders (plan_critic) legitimately consume
	// RawRequest and are intentionally NOT in this set.
	gateFiles := preciseSignalLintGateFiles(t)
	fset := token.NewFileSet()
	var violations []string
	for _, path := range gateFiles {
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		violations = append(violations, findRawRequestKeywordMatches(f, fset, path)...)
	}
	if len(violations) > 0 {
		t.Fatalf("keyword-match of user intent against raw_request in a hard gate — match a TYPED token (identifier/symbol/expected from the IR) via a sanctioned sink instead of a string literal:\n  %s", strings.Join(violations, "\n  "))
	}
}

func preciseSignalLintGateFiles(t *testing.T) []string {
	t.Helper()
	patterns := []string{
		"contract_check*.go",
		"answer_exclusion_policy_check.go",
		"write_analysis_quality.go",
		"../analysis/gate/coherence.go",
		"../tool/answer_document*.go",
		"../tool/emit_answer_document*.go",
		"../tool/emit_evidence.go",
		"../tool/emit_investigation_complete*.go",
		"../tool/pre_emit*.go",
		"../tool/source_inventory*.go",
	}
	seen := map[string]bool{}
	var files []string
	for _, pattern := range patterns {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			t.Fatalf("bad precise-signal lint glob %q: %v", pattern, err)
		}
		if len(matches) == 0 {
			t.Fatalf("precise-signal lint glob %q matched no files", pattern)
		}
		for _, match := range matches {
			if seen[match] || strings.HasSuffix(match, "_test.go") {
				continue
			}
			seen[match] = true
			files = append(files, match)
		}
	}
	sort.Strings(files)
	return files
}

var rawRequestStringOps = map[string]bool{
	"Contains": true, "EqualFold": true, "HasPrefix": true,
	"HasSuffix": true, "Index": true, "Count": true, "ContainsAny": true,
}

// findRawRequestKeywordMatches flags every strings.<op> call in f whose haystack
// is RawRequest-derived (the .RawRequest selector directly, or a local assigned
// from it) AND whose matched operand is a string literal.
func findRawRequestKeywordMatches(f *ast.File, fset *token.FileSet, path string) []string {
	var out []string
	ast.Inspect(f, func(n ast.Node) bool {
		fn, ok := n.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			return true
		}
		rawLocals := rawRequestDerivedLocals(fn.Body)
		ast.Inspect(fn.Body, func(m ast.Node) bool {
			call, ok := m.(*ast.CallExpr)
			if !ok || !isStringsStringOp(call) || len(call.Args) < 2 {
				return true
			}
			if !exprIsRawRequestDerived(call.Args[0], rawLocals) {
				return true
			}
			for _, arg := range call.Args[1:] {
				if isStringLiteral(arg) {
					out = append(out, path+":"+strconv.Itoa(fset.Position(call.Pos()).Line))
					break
				}
			}
			return true
		})
		return true
	})
	return out
}

// rawRequestDerivedLocals collects the names of locals assigned (:= or =) from an
// expression that reads a .RawRequest field.
func rawRequestDerivedLocals(body *ast.BlockStmt) map[string]bool {
	locals := map[string]bool{}
	ast.Inspect(body, func(n ast.Node) bool {
		assign, ok := n.(*ast.AssignStmt)
		if !ok {
			return true
		}
		for i, rhs := range assign.Rhs {
			if i >= len(assign.Lhs) {
				break
			}
			if !exprReadsRawRequest(rhs) {
				continue
			}
			if id, ok := assign.Lhs[i].(*ast.Ident); ok {
				locals[id.Name] = true
			}
		}
		return true
	})
	return locals
}

func exprReadsRawRequest(e ast.Expr) bool {
	found := false
	ast.Inspect(e, func(n ast.Node) bool {
		if sel, ok := n.(*ast.SelectorExpr); ok && sel.Sel != nil && sel.Sel.Name == "RawRequest" {
			found = true
			return false
		}
		return true
	})
	return found
}

func exprIsRawRequestDerived(e ast.Expr, rawLocals map[string]bool) bool {
	if exprReadsRawRequest(e) {
		return true
	}
	found := false
	ast.Inspect(e, func(n ast.Node) bool {
		if id, ok := n.(*ast.Ident); ok && rawLocals[id.Name] {
			found = true
			return false
		}
		return true
	})
	return found
}

func isStringsStringOp(call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	return ok && pkg.Name == "strings" && sel.Sel != nil && rawRequestStringOps[sel.Sel.Name]
}

func isStringLiteral(e ast.Expr) bool {
	lit, ok := e.(*ast.BasicLit)
	return ok && lit.Kind == token.STRING
}

// TestPreciseSignalLintDetectsKeywordMatch self-validates the detector against a
// synthetic violation and legitimate typed-token usage.
func TestPreciseSignalLintDetectsKeywordMatch(t *testing.T) {
	bad := `package p
import "strings"
func g(rm struct{ RawRequest string }, ident string) bool {
	raw := strings.TrimSpace(rm.RawRequest)
	if strings.Contains(raw, "fix the bug") { return true }   // literal keyword match -> BAN
	return strings.EqualFold(rm.RawRequest, "list members")    // direct literal match -> BAN
}`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "bad.go", bad, 0)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got := findRawRequestKeywordMatches(f, fset, "bad.go"); len(got) != 2 {
		t.Fatalf("detector must flag both literal keyword matches, got %d: %v", len(got), got)
	}

	good := `package p
import "strings"
func g(rm struct{ RawRequest string }, ident, expected string) bool {
	raw := strings.TrimSpace(rm.RawRequest)
	if strings.Contains(raw, ident) { return true }        // typed token -> OK
	if strings.Contains(raw, expected) { return true }     // typed token -> OK
	return mentions(raw, "trace_query")                    // sanctioned sink (not strings.*) -> OK
}
func mentions(raw, token string) bool { return false }`
	f2, err := parser.ParseFile(fset, "good.go", good, 0)
	if err != nil {
		t.Fatalf("parse good: %v", err)
	}
	if got := findRawRequestKeywordMatches(f2, fset, "good.go"); len(got) != 0 {
		t.Fatalf("detector must NOT flag typed-token matches or sanctioned sinks, got %v", got)
	}
}
