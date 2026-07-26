package dataworkflow

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"strings"
	"testing"
)

// ledger_completion_tripwire_test.go — LEDGER-TRIPWIRE (GAP-EVAL-D1 class
// guard, 2026-07-26): the D1 hole was "machinery present, dispatch case
// missing" — the decisions ledger's producer closed set already named
// compute_contributions, but buildRequiredLedgerCompletionPlanFromGraph's
// switch had no decisions case, so the deterministic backfill silently fell
// through and the workflow terminal-failed with the correct answer in hand.
// This pin makes that hole class STATICALLY red: every ledger in the
// LedgerKind universe must either appear as a case in the deterministic
// completion dispatch or carry an explicit exemption with a rationale.

// ledgerCompletionExemptions — deliberate non-deterministic-completion
// ledgers. Adding a ledger kind without either a dispatch case or an entry
// here is a red test, exactly like the thread-state fall-through ledger.
var ledgerCompletionExemptions = map[string]string{
	string(LedgerFinalProjection):   "owned by BuildRequiredOutputProjectionPlan — the output-projection lane runs BEFORE the ledger dispatch in BuildRequiredWorkflowCompletionPlan, so the switch never needs a final_projection case",
	string(LedgerEntityResolutions): "entity resolution requires semantic mapping decisions (which source entity maps to which canonical entity) — no conservative deterministic backfill exists; the LLM plans normalize_entities/apply_entity_resolutions with the full-roster disclosure naming the gap",
}

func TestEveryRequiredLedgerHasDeterministicCompletionOrExemption(t *testing.T) {
	universe := ledgerKindUniverseFromSource(t)
	if len(universe) < 6 {
		t.Fatalf("ledger universe scan looks broken (got %v) — the tripwire would be vacuous", universe)
	}
	cases := ledgerCompletionDispatchCasesFromSource(t)
	if len(cases) == 0 {
		t.Fatal("no dispatch cases found — the tripwire is checking nothing")
	}
	for _, ledger := range universe {
		if cases[ledger] {
			continue
		}
		if rationale, ok := ledgerCompletionExemptions[ledger]; ok {
			if strings.TrimSpace(rationale) == "" {
				t.Errorf("ledger %q exemption has no rationale", ledger)
			}
			continue
		}
		t.Errorf("ledger %q has NEITHER a deterministic completion dispatch case NOR an explicit exemption — the GAP-EVAL-D1 hole class (machinery present, dispatch missing); add the case in buildRequiredLedgerCompletionPlanFromGraph or declare the exemption with a rationale", ledger)
	}
	// The reverse direction: exemptions must not go stale (an exempted
	// ledger that GAINS a dispatch case should drop its entry).
	for ledger := range ledgerCompletionExemptions {
		if cases[ledger] {
			t.Errorf("ledger %q is exempted AND dispatched — remove the stale exemption", ledger)
		}
	}
}

// ledgerKindUniverseFromSource extracts every LedgerKind constant's wire
// value from action_capability.go (the const block IS the universe — never
// hand-copied).
func ledgerKindUniverseFromSource(t *testing.T) []string {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "action_capability.go", nil, 0)
	if err != nil {
		t.Fatalf("parse action_capability.go: %v", err)
	}
	var out []string
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST {
			continue
		}
		for _, spec := range gen.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			ident, ok := vs.Type.(*ast.Ident)
			if !ok || ident.Name != "LedgerKind" {
				continue
			}
			for _, value := range vs.Values {
				lit, ok := value.(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					continue
				}
				raw, err := strconv.Unquote(lit.Value)
				if err != nil {
					t.Fatalf("unquote LedgerKind value %s: %v", lit.Value, err)
				}
				out = append(out, raw)
			}
		}
	}
	return out
}

// ledgerCompletionDispatchCasesFromSource scans fallback_plan.go for the
// buildRequiredLedgerCompletionPlanFromGraph switch and collects every
// `case string(LedgerX)` ledger name (resolved through the same const
// block, so a renamed constant cannot desynchronize the scan).
func ledgerCompletionDispatchCasesFromSource(t *testing.T) map[string]bool {
	t.Helper()
	constants := map[string]string{}
	for _, name := range ledgerKindUniverseFromSource(t) {
		constants[name] = name
	}
	identValues := ledgerKindIdentValuesFromSource(t)
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "fallback_plan.go", nil, 0)
	if err != nil {
		t.Fatalf("parse fallback_plan.go: %v", err)
	}
	out := map[string]bool{}
	ast.Inspect(file, func(n ast.Node) bool {
		fn, ok := n.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "buildRequiredLedgerCompletionPlanFromGraph" {
			return true
		}
		ast.Inspect(fn, func(m ast.Node) bool {
			clause, ok := m.(*ast.CaseClause)
			if !ok {
				return true
			}
			for _, expr := range clause.List {
				call, ok := expr.(*ast.CallExpr)
				if !ok || len(call.Args) != 1 {
					continue
				}
				ident, ok := call.Args[0].(*ast.Ident)
				if !ok {
					continue
				}
				if value, known := identValues[ident.Name]; known {
					out[value] = true
				}
			}
			return true
		})
		return false
	})
	return out
}

// ledgerKindIdentValuesFromSource maps LedgerKind constant IDENT names to
// their wire values (LedgerDecisions → "decisions").
func ledgerKindIdentValuesFromSource(t *testing.T) map[string]string {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "action_capability.go", nil, 0)
	if err != nil {
		t.Fatalf("parse action_capability.go: %v", err)
	}
	out := map[string]string{}
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST {
			continue
		}
		for _, spec := range gen.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			ident, ok := vs.Type.(*ast.Ident)
			if !ok || ident.Name != "LedgerKind" {
				continue
			}
			for i, name := range vs.Names {
				if i >= len(vs.Values) {
					continue
				}
				lit, ok := vs.Values[i].(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					continue
				}
				raw, err := strconv.Unquote(lit.Value)
				if err != nil {
					t.Fatalf("unquote LedgerKind value %s: %v", lit.Value, err)
				}
				out[name.Name] = raw
			}
		}
	}
	return out
}
