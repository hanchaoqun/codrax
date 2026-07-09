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
			"answer_document_mutation_runtime_typelabels.go::runtimeTraceRootCauseTypeZHLabel": true, // compute_supply → 算力供给 (delivery-lane zh label)
			"answer_document_mutation_runtime_rcr.go::runtimeTraceProjImpactFormSpecs":         true, // PTV8-RCR-A §24.1/§24.3: 算力供给候选 category family word (delivery lane, user-ruled 类别词族; single-source form table)
			"answer_document_mutation_runtime.go::runtimeTraceProjCompareOverviewBlocks":       true, // compare-overview 算力供给(归一化) column header (PTV8-LAD L6: builder renamed to the plural form, same delivery-lane strings)
			"trace_query.go::writeTraceComputeSupplyBalance":                                   true, // compute_supply_balance(算力供给) delivery stanza
			// CAP (§26 C3, real_trace_campaign_20260705.md, 2026-07-08 —
			// §7.4/§7.5 re-read): the capability disclosure words (按默认算力比
			// 粗算 / 核类算力差…) price the DELIVERY-side running folds — the
			// VS-2 supply fold and the R5d discounted running component
			// (supply_fold_deficit is a ComputeDelivery-lane token), squarely
			// the lane §7.4 reserves 算力 for. Single sources: the clause
			// helper + its legend seats in the catalog.
			// CAP-2 (§28.4/§28.5, 2026-07-09): the clause helper grew the
			// topology-aware single source (Clause delegates to ClauseTopo);
			// the wording home is unchanged, only the enclosing func renamed.
			"answer_document_mutation_runtime_supplyfold.go::runtimeTraceProjCapabilityCaliberClauseTopo": true,
			"answer_document_mutation_runtime_tree.go::runtimeTraceProjLegendCatalog":                 true,
			// …and the wrap-atom registration of the same word (LAD L5 table —
			// display-wrap boundary only, no new wording surface).
			"answer_document_mutation_runtime_tree.go::(package-level)": true,
		},
	},
	{
		term: "调度压力",
		whitelist: map[string]bool{
			"answer_document_mutation_runtime_typelabels.go::runtimeTraceSupplyPressureDisplayLabel": true,
			// EVOLUTION RECORD (SYM-2 §24.17 R2, 2026-07-08, §7.4/§7.5 re-read):
			// the runnable family's 行2 category word is 调度压力候选 (user
			// ruling verbatim "自身 runnable → 调度压力候选" — the §7.4
			// demand-side vocabulary, replacing 就绪排队候选). Same precedent
			// as the 算力供给候选 entry above: the single-source form table IS
			// the category-word home; the supply_pressure token label keeps
			// its own single source in the typelabels helper.
			"answer_document_mutation_runtime_rcr.go::runtimeTraceProjImpactFormSpecs": true,
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
			// CAP-2 (2026-07-09): the four-branch body moved into the Core
			// helper so the THERM sentence could append once at the wrapper —
			// still ONE decision-table home, only the enclosing func renamed.
			"answer_document_mutation_runtime_supplyfold.go::runtimeTraceProjSupplyFoldClauseCore": true,
			// PTV8-RCR-A §24 ②/§24.1 (2026-07-08): the inversion node's
			// detail-block 供给折算 line — the retired Triple sentence's data
			// half in the unified sub-row grammar (供给折算缺口 句式并入口径
			// 括注, §24.1). Still delivery-lane wording; the demand side stays
			// on runtimeTraceSupplyPressureDisplayLabel.
			"answer_document_mutation_runtime_rcr.go::runtimeTraceProjInversionSupplyFoldDetailLine": true,
		},
	},
	{
		// §7.30.3 D3 / VS-2 (§7.10 red line 3): the inversion lane's discount
		// word (PTV7 face: running 折算; formerly 运行折算) is single-sourced
		// from the composition-text helper. The supply-fold lane says
		// 供给折算缺口 and never borrows this term — different mechanisms.
		term: "running 折算",
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
