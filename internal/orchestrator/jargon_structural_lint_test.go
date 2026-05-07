package orchestrator

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

// TestNoInternalTermsInOrchestratorStrings is the structural twin of
// internal/agent/glossary_lint_test.go's TestNoInternalTermsInHints —
// ported to the orchestrator package. It catches internal-term leaks
// that the existing reviewer-prompt audit does NOT scan: validator
// Violation.Detail / Repair / SuspectedRoot.Reason fields, retry-hint
// composer feed strings, repair-cluster Reason fields, etc. — all of
// which the orchestrator funnels into the next dispatch's prompt
// surface via the retry-hint composer.
//
// Coverage gap before this test:
//   - skill.TestNoInternalTermsInPrompts scans skill configs only
//   - tool.TestNoInternalTermsInToolSchemas scans emit_* tool schemas
//   - agent.TestNoInternalTermsInHints scans internal/agent/*.go only
//   - orchestrator.TestReviewerPrompts_LLMFacingNoInternalJargon
//     scans 6 hand-curated reviewer system prompts + 1 tool desc
//
// Anything constructed dynamically inside a validator and bound to a
// Violation.Detail / Repair / Reason was completely outside any audit
// surface. This file closes that gap.
//
// Filter discipline (matches the agent test):
//   - excludes string literals that are direct args to logging helpers
//     (logging.Debug/Info/Warning/Error/Trace, log.*, fmt.Fprintf etc)
//   - excludes import paths
//   - excludes literals that are RHS of const declarations (typed enum
//     constant values like FallbackTarget = "finalizer_only" appear
//     in many internal contexts but never reach LLM prompts; their
//     usage is wire-format identifier, not user-facing prose)
//   - excludes literals assigned to identifiers ending in "Error"
//     (TaskState.LastError = "..."): these surface in fail-loud
//     terminal output to the operator, not as input to the next LLM
//     prompt; the operator IS expected to understand internal terms
//     in a fail-loud diagnostic
//
// Anything else — Violation.Detail, Violation.Repair, Reason fields,
// fmt.Sprintf return values, struct literal field strings — is in
// scope. False positives can be silenced by adding the right exclusion
// rule above; do NOT silence by adding the leaked term to a per-test
// allow list, because that defeats the whole point of the lint.
func TestNoInternalTermsInOrchestratorStrings(t *testing.T) {
	files, err := listOrchestratorGoFiles(".")
	if err != nil {
		t.Fatalf("listOrchestratorGoFiles: %v", err)
	}
	if len(files) == 0 {
		t.Fatalf("no non-test .go files found under internal/orchestrator/")
	}

	fset := token.NewFileSet()
	var hits []string
	for _, path := range files {
		file, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if err != nil {
			t.Errorf("parse %s: %v", path, err)
			continue
		}

		excluded := collectOrchestratorExclusionPositions(file)
		for _, imp := range file.Imports {
			if imp.Path != nil {
				excluded[imp.Path.Pos()] = true
			}
		}

		ast.Inspect(file, func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			if excluded[lit.Pos()] {
				return true
			}
			raw, err := strconv.Unquote(lit.Value)
			if err != nil {
				return true
			}
			for _, term := range localInternalTerms {
				if term == "" {
					continue
				}
				if !strings.Contains(raw, term) {
					continue
				}
				pos := fset.Position(lit.Pos())
				hits = append(hits, formatOrchestratorHit(pos.Filename, pos.Line, term, raw))
			}
			return true
		})
	}

	if len(hits) == 0 {
		return
	}

	for _, h := range hits {
		t.Errorf("  %s", h)
	}
	t.Fatalf("TestNoInternalTermsInOrchestratorStrings found %d violation(s); rephrase in user-facing language or add to the exclusion rule above (logger / const / *Error assignment)", len(hits))
}

// collectOrchestratorExclusionPositions walks the AST and marks every
// string literal whose Pos must be skipped: logger arguments, const
// declaration RHS values, and assignments to identifiers ending in
// "Error" (TaskState.LastError, RunError, etc. — fail-loud terminal
// surfaces, not LLM-prompt inputs).
func collectOrchestratorExclusionPositions(file *ast.File) map[token.Pos]bool {
	out := make(map[token.Pos]bool)

	ast.Inspect(file, func(n ast.Node) bool {
		// Logger calls.
		if call, ok := n.(*ast.CallExpr); ok && isOrchestratorLoggingCall(call.Fun) {
			for _, arg := range call.Args {
				markStringLits(arg, out)
			}
		}
		// Const declarations.
		if decl, ok := n.(*ast.GenDecl); ok && decl.Tok == token.CONST {
			for _, spec := range decl.Specs {
				vs, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				for _, val := range vs.Values {
					markStringLits(val, out)
				}
			}
		}
		// Assignments to *Error fields / variables.
		if assign, ok := n.(*ast.AssignStmt); ok {
			for i, lhs := range assign.Lhs {
				if !isErrorTargetExpr(lhs) {
					continue
				}
				if i < len(assign.Rhs) {
					markStringLits(assign.Rhs[i], out)
				}
			}
		}
		return true
	})

	return out
}

// markStringLits flags every STRING literal directly inside expr (and
// inside trivial wrappers like parenthesised expressions / typed
// composite literals at the top level). We deliberately stop at
// function-call boundaries — only direct literal args qualify.
func markStringLits(expr ast.Expr, out map[token.Pos]bool) {
	switch v := expr.(type) {
	case *ast.BasicLit:
		if v.Kind == token.STRING {
			out[v.Pos()] = true
		}
	case *ast.ParenExpr:
		markStringLits(v.X, out)
	case *ast.BinaryExpr:
		// String concatenation: each operand still counts as a direct
		// literal in the same surface.
		markStringLits(v.X, out)
		markStringLits(v.Y, out)
	}
}

// isErrorTargetExpr reports whether the LHS of an assignment names a
// fail-loud terminal target (LastError / RunError / *Error suffix on
// either an identifier or a field selector). Such writes flow to
// CLI output, not into the next LLM prompt.
func isErrorTargetExpr(expr ast.Expr) bool {
	switch v := expr.(type) {
	case *ast.SelectorExpr:
		return strings.HasSuffix(v.Sel.Name, "Error")
	case *ast.Ident:
		return strings.HasSuffix(v.Name, "Error")
	}
	return false
}

func isOrchestratorLoggingCall(fn ast.Expr) bool {
	sel, ok := fn.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	if !ok {
		return false
	}
	name := sel.Sel.Name
	switch pkg.Name {
	case "logging":
		switch name {
		case "Debug", "Info", "Warning", "Warn", "Error", "Trace", "Fatalf", "Fatal", "Panicf":
			return true
		}
	case "log":
		switch name {
		case "Printf", "Println", "Print", "Fatalf", "Fatal", "Panicf", "Panic":
			return true
		}
	case "fmt":
		switch name {
		case "Fprintf", "Fprintln", "Fprint":
			return true
		}
	}
	return false
}

func listOrchestratorGoFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		out = append(out, filepath.Join(dir, name))
	}
	return out, nil
}

func formatOrchestratorHit(file string, line int, term, body string) string {
	const window = 50
	idx := strings.Index(body, term)
	if idx < 0 {
		return file + ":" + strconv.Itoa(line) + " token=" + term + " preview=" + bodyPreview(body, 0, window*2)
	}
	from := idx - window
	if from < 0 {
		from = 0
	}
	to := idx + len(term) + window
	if to > len(body) {
		to = len(body)
	}
	preview := body[from:to]
	if from > 0 {
		preview = "…" + preview
	}
	if to < len(body) {
		preview = preview + "…"
	}
	return file + ":" + itoa(line) + " token=" + term + " preview=" + preview
}

func bodyPreview(s string, from, to int) string {
	if from < 0 {
		from = 0
	}
	if to > len(s) {
		to = len(s)
	}
	if from >= to {
		return ""
	}
	return s[from:to]
}

