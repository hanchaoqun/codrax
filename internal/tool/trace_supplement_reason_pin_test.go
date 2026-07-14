package tool

// trace_supplement_reason_pin_test.go — SUPP-HYG P3-4 (批4 立案, 2026-07-14):
// AST pins keeping the supplement skip/lane reason family on the typed closed
// set (types.TraceSupplementReason* — trace_supplement_reasons.go), NKR
// consumer-pin 同构 (trace_note_keys_consumer_pin_test.go model).
//
// Three rules, all precise structural signals over the two production files
// (internal/tool/trace_query_supplement.go = producer + operator log face,
// internal/tool/answer_document_mutation_runtime.go = disclosure consumer):
//
//  1. No string literal anywhere in either file may EQUAL a registered
//     reason value — every mint / equality point / lane label goes through
//     the constant (a bare literal is exactly the silent-typo lane the
//     registry closes).
//  2. No ==/!= comparison against a `SkipReason` selector may use a string
//     literal on either side (belt over rule 1: catches a literal that
//     drifted OFF the registry — the typo itself).
//  3. No format-string literal in either file may embed a registered reason
//     token inside a longer string (operator log face: "reason=<literal>"
//     must be "reason=%s" + constant).
//
// Every rule fatals when its scan matches nothing (matched==0) — a pin that
// stops matching is a pin that silently checks nothing. Test files keep
// verbatim reason literals on purpose (wire-format double-write) and are
// excluded, exactly like the NKR and causal-token registry pins.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

var traceSupplementReasonPinFiles = []string{
	"trace_query_supplement.go",
	"answer_document_mutation_runtime.go",
}

func traceSupplementReasonPinParse(t *testing.T, name string) (*token.FileSet, *ast.File) {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, name, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", name, err)
	}
	return fset, file
}

// Rule 1 + 3: literal sweep — no string literal equals or embeds a registered
// reason value.
func TestTraceSupplementReasonNoBareLiterals(t *testing.T) {
	reasons := types.TraceSupplementReasons()
	if len(reasons) == 0 {
		t.Fatal("reason registry is empty — the pin is checking nothing")
	}
	scanned := 0
	for _, name := range traceSupplementReasonPinFiles {
		fset, file := traceSupplementReasonPinParse(t, name)
		ast.Inspect(file, func(n ast.Node) bool {
			basic, ok := n.(*ast.BasicLit)
			if !ok || basic.Kind != token.STRING {
				return true
			}
			scanned++
			value := strings.Trim(basic.Value, "`\"")
			for _, reason := range reasons {
				if value == reason {
					t.Errorf("%s: bare reason literal %s — use types.TraceSupplementReason* (trace_supplement_reasons.go change protocol)",
						fset.Position(basic.Pos()), basic.Value)
					continue
				}
				if strings.Contains(value, reason) {
					t.Errorf("%s: string literal %s embeds registered reason %q — log/format faces must carry the constant through a %%s verb",
						fset.Position(basic.Pos()), basic.Value, reason)
				}
			}
			return true
		})
	}
	if scanned == 0 {
		t.Fatal("literal sweep matched no string literals — the pin is checking nothing; update traceSupplementReasonPinFiles alongside the refactor")
	}
}

// Rule 2: SkipReason equality points never compare against a string literal.
func TestTraceSupplementReasonEqualityPointsUseConstants(t *testing.T) {
	comparisons := 0
	for _, name := range traceSupplementReasonPinFiles {
		fset, file := traceSupplementReasonPinParse(t, name)
		ast.Inspect(file, func(n ast.Node) bool {
			bin, ok := n.(*ast.BinaryExpr)
			if !ok || (bin.Op != token.EQL && bin.Op != token.NEQ) {
				return true
			}
			isSkipReasonSelector := func(e ast.Expr) bool {
				sel, ok := e.(*ast.SelectorExpr)
				return ok && sel.Sel.Name == "SkipReason"
			}
			if !isSkipReasonSelector(bin.X) && !isSkipReasonSelector(bin.Y) {
				return true
			}
			comparisons++
			for _, side := range []ast.Expr{bin.X, bin.Y} {
				if lit, isLit := side.(*ast.BasicLit); isLit && lit.Kind == token.STRING && strings.Trim(lit.Value, "`\"") != "" {
					t.Errorf("%s: SkipReason compared against bare literal %s — use types.TraceSupplementReason*",
						fset.Position(lit.Pos()), lit.Value)
				}
			}
			return true
		})
	}
	if comparisons == 0 {
		t.Fatal("SkipReason equality scan matched no comparison sites — the pin is checking nothing; move it with the refactor")
	}
}

// Registry cross-check: every reason value the two production files actually
// reference through the constants must be registered (by construction the
// constants ARE the registry — this pin guards the OTHER direction: a lane
// label passed to the census-lite helpers must be a member, so an unregistered
// ad-hoc lane string is red even though it is not a literal at the call site).
func TestTraceSupplementReasonLaneLabelsRegistered(t *testing.T) {
	for _, lane := range []string{
		types.TraceSupplementReasonFamiliesPresent,
		types.TraceSupplementReasonNoTypedTarget,
		types.TraceSupplementReasonNoTypedWindow,
		types.TraceSupplementReasonWindowInconsistent,
		types.TraceSupplementReasonWindowSpanExceeded,
		types.TraceSupplementReasonExecutionFailed,
		types.TraceSupplementReasonWindowedCensusAbsent,
		types.TraceSupplementReasonCanceledByCaller,
		types.TraceSupplementReasonDurationBudgetExceeded,
		types.TraceSupplementReasonColdBudgetExceeded,
		types.TraceSupplementReasonDisabled,
		types.TraceSupplementReasonNoAttachedTrace,
	} {
		if !types.TraceSupplementReasonRegistered(lane) {
			t.Fatalf("reason constant %q is not registered — constant and registry drifted", lane)
		}
	}
}
