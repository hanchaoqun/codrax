package tool

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// semantic_wording_lint_test.go — RN-16 (§7.9) layer 3: source-scan wording
// lint for the §7.4 demand/supply wording lanes (modeled on
// TestNoInternalTermsInPrompts). It walks every NON-test .go file in
// internal/tool and internal/tracequery and checks each STRING LITERAL
// containing a lane-locked term against a file::function whitelist:
//
//   - "算力"   — compute-DELIVERY wording only: the compute_supply zh label
//     (runtimeTraceRootCauseTypeZHLabel), the compare-overview supply column,
//     the execution/CPU display cells that name the CONSUMING side
//     (user-adjudicated during the RN-15 display sweep), and the
//     compute_supply_balance stanza renderer.
//   - "调度压力" / "需求积压" — demand-lane wording is single-sourced from
//     runtimeTraceSupplyPressureDisplayLabel (§7.5: wire token retained,
//     demand-backlog semantics owned by the display layer).
//
// A hit outside the whitelist means a new surface is wording a runnable/
// demand signal as compute (or minting a second demand label source) —
// re-read §7.4/§7.5 in docs/design/customer_dead_session_audit_20260703.md
// and route through the existing helpers instead of extending the whitelist.
// Stale whitelist entries (no longer matching any literal) also fail, so the
// list cannot rot.

type wordingLaneRule struct {
	term string
	// whitelist keys are "<file base name>::<enclosing func name>";
	// package-level literals use "<file>::(package-level)".
	whitelist map[string]bool
}

var wordingLaneRules = []wordingLaneRule{
	{
		term: "算力",
		whitelist: map[string]bool{
			"answer_document_mutation_runtime_typelabels.go::runtimeTraceRootCauseTypeZHLabel":   true, // compute_supply → 算力供给 (delivery-lane zh label)
			"answer_document_mutation_runtime.go::runtimeTraceProjCompareOverviewBlock":          true, // compare-overview 算力供给(归一化) column header
			"answer_document_mutation_runtime.go::runtimeTraceCausalProjectionActionCell":        true, // 执行/算力 action cell (consuming side, RN-15 sweep kept)
			"answer_document_mutation_runtime.go::runtimeTraceCausalProjectionImpactMeaningCell": true, // 本层运行/算力占用 meaning cell (consuming side)
			"trace_query.go::writeTraceComputeSupplyBalance":                                     true, // compute_supply_balance(算力供给) delivery stanza
		},
	},
	{
		term: "调度压力",
		whitelist: map[string]bool{
			"answer_document_mutation_runtime_typelabels.go::runtimeTraceSupplyPressureDisplayLabel": true,
		},
	},
	{
		term: "需求积压",
		whitelist: map[string]bool{
			"answer_document_mutation_runtime_typelabels.go::runtimeTraceSupplyPressureDisplayLabel": true,
		},
	},
	{
		// VS-2 (§7.10): 供给折算缺口 is ComputeDelivery-lane wording, single-
		// sourced from the four-branch clause helper — every display surface
		// (conclusion line, fence tag, detail table) consumes that one
		// function, so a second literal means a fork of the decision table.
		term: "供给折算缺口",
		whitelist: map[string]bool{
			"answer_document_mutation_runtime_supplyfold.go::runtimeTraceProjSupplyFoldClause": true,
		},
	},
	{
		// §7.30.3 D3 / VS-2 (§7.10 red line 3): 运行折算 is the INVERSION
		// lane's word (consumer-relative discount), single-sourced from the
		// composition-text helper. The supply-fold lane says 供给折算缺口 and
		// never borrows this term — the two folds are different mechanisms.
		term: "运行折算",
		whitelist: map[string]bool{
			"answer_document_mutation_runtime_supplyfold.go::runtimeTraceProjInversionCompositionText": true,
		},
	},
}

// wordingLintDirs: the two packages owning trace causal wording (relative to
// this package's directory).
var wordingLintDirs = []string{".", "../tracequery"}

func TestSemanticWordingLaneLint(t *testing.T) {
	fset := token.NewFileSet()
	var violations []string
	hits := map[string]map[string]bool{} // term → whitelist key → seen

	for _, rule := range wordingLaneRules {
		hits[rule.term] = map[string]bool{}
	}

	for _, dir := range wordingLintDirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatalf("ReadDir(%s): %v", dir, err)
		}
		for _, entry := range entries {
			name := entry.Name()
			if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
				continue
			}
			path := filepath.Join(dir, name)
			file, err := parser.ParseFile(fset, path, nil, 0)
			if err != nil {
				t.Fatalf("parse %s: %v", path, err)
			}
			scanWordingLanes(t, fset, file, name, hits, &violations)
		}
	}

	for _, v := range violations {
		t.Errorf("  %s", v)
	}
	if len(violations) > 0 {
		t.Fatalf("TestSemanticWordingLaneLint: %d wording-lane violation(s) — §7.4: 算力 belongs to the compute-delivery lane only; runnable/demand wording is 调度压力(需求积压) and is single-sourced from runtimeTraceSupplyPressureDisplayLabel. Route through the existing helpers; do not extend the whitelist without re-reading §7.4/§7.5 (docs/design/customer_dead_session_audit_20260703.md)", len(violations))
	}

	// Whitelist rot check: every entry must still match at least one literal.
	for _, rule := range wordingLaneRules {
		for key := range rule.whitelist {
			if !hits[rule.term][key] {
				t.Errorf("stale whitelist entry for term %q: %s no longer contains such a literal — delete the entry", rule.term, key)
			}
		}
	}
}

// scanWordingLanes walks one parsed file: every string literal is attributed
// to its enclosing top-level function (or "(package-level)") and checked
// against each rule.
func scanWordingLanes(t *testing.T, fset *token.FileSet, file *ast.File, fileName string, hits map[string]map[string]bool, violations *[]string) {
	t.Helper()
	check := func(lit *ast.BasicLit, enclosing string) {
		if lit.Kind != token.STRING {
			return
		}
		value, err := strconv.Unquote(lit.Value)
		if err != nil {
			return
		}
		for _, rule := range wordingLaneRules {
			if !strings.Contains(value, rule.term) {
				continue
			}
			key := fileName + "::" + enclosing
			if rule.whitelist[key] {
				hits[rule.term][key] = true
				continue
			}
			pos := fset.Position(lit.Pos())
			*violations = append(*violations, fmt.Sprintf("%s:%d: term %q in string literal inside %s (whitelist key %q)", pos.Filename, pos.Line, rule.term, enclosing, key))
		}
	}

	for _, decl := range file.Decls {
		enclosing := "(package-level)"
		if fn, ok := decl.(*ast.FuncDecl); ok {
			enclosing = fn.Name.Name
		}
		ast.Inspect(decl, func(n ast.Node) bool {
			if lit, ok := n.(*ast.BasicLit); ok {
				check(lit, enclosing)
			}
			return true
		})
	}
}
