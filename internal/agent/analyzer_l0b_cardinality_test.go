package agent

import (
	"os"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// TestDistinctNamedEntities covers the helper's contract (case-fold
// + trim + drop blanks). Used by both amplifier's distinctEntityCount
// and the L0-B enumeration cardinality gate; we test it locally
// here to defend against drift in this copy.
func TestDistinctNamedEntities(t *testing.T) {
	cases := []struct {
		name string
		in   []string
		want int
	}{
		{"empty", nil, 0},
		{"empty_strings", []string{"", "  ", "\t"}, 0},
		{"single", []string{"Foo"}, 1},
		{"three_distinct", []string{"Foo", "Bar", "Baz"}, 3},
		{"case_fold_dup", []string{"Foo", "FOO", "foo"}, 1},
		{"trim_dup", []string{"Foo", "  Foo  ", "\tFoo\n"}, 1},
		{"mixed_blanks_and_distinct", []string{"Foo", "", "Bar", "  "}, 2},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := distinctNamedEntities(c.in)
			if got != c.want {
				t.Errorf("distinctNamedEntities(%v) = %d, want %d", c.in, got, c.want)
			}
		})
	}
}

// TestL0B_EnumerationCardinalityGate is a structural assertion of
// the L0-B reject contract. We exercise the full buildAnalysisIR
// path via a synthetic RequestModel so the gate is hit at the real
// runtime location, not in isolation.
//
// L0-B contract:
//
//   - cat=true && distinct entities ≤ 1 → REJECT (error returned)
//   - cat=true && distinct entities ≥ 2 → PASS
//   - cat=false && any entity count → PASS
//
// Because buildAnalysisIR depends on a fully-populated AgentContext
// (AgentContext.Mutable, BusContext.Objective, repomap, ...) that
// is expensive to fake, we test the gate at the source level: the
// reject branch must contain the precise predicate combination and
// the error message must name both halves of the contradiction so
// the LLM retry sees an actionable fix.
func TestL0B_EnumerationCardinalityGate_PresentInSource(t *testing.T) {
	const wantPredicateExpr = "rm.Predicates.IsCategoryEnumeration &&"
	const wantHelperCall = "distinctNamedEntities(rm.AnalyzerHints.Entities) <= 1"
	// 2026-05-10 R6 audit: the reject message used to read
	// "ENUMERATED VALUES (e.g. StageAnalyze, StageExplore, ...)" —
	// the parenthetical leaked Go const names from
	// internal/types/context.go into an LLM-facing retry hint.
	// Fix-β realigned the wording to "ENUMERATED MEMBERS (e.g.
	// each implementer name, each enum case name, ...)" matching
	// internal/skill/analysis_contract.go:365 exactly. Either token
	// keeps the cardinality intent legible to the LLM, so the
	// assertion below accepts both spellings.
	const wantErrorMsgFix2 = "is_category_enumeration=false"
	// 2026-05-08 carve-out: gate must NOT fire on
	// IsCategoryEnumeration=true + IsRelationalLookup=true + 1 entity.
	const wantRelationalCarveOut = "!rm.Predicates.IsRelationalLookup"
	const wantHistoryCarveOut = "!rm.Predicates.IsHistoryLookup"

	src := readAnalyzerSource(t)

	if !strings.Contains(src, wantPredicateExpr) {
		t.Errorf("L0-B gate predicate missing: expected substring %q in analyzer.go", wantPredicateExpr)
	}
	if !strings.Contains(src, wantHelperCall) {
		t.Errorf("L0-B gate helper call missing: expected substring %q in analyzer.go", wantHelperCall)
	}
	if !strings.Contains(src, "ENUMERATED MEMBERS") && !strings.Contains(src, "ENUMERATED VALUES") {
		t.Errorf("L0-B reject message must explicitly name 'ENUMERATED MEMBERS' (or legacy 'ENUMERATED VALUES') so the LLM retry knows the fix")
	}
	if !strings.Contains(src, wantErrorMsgFix2) {
		t.Errorf("L0-B reject message must offer the cat=false escape hatch for legitimate type-name lookups")
	}
	if !strings.Contains(src, wantRelationalCarveOut) {
		t.Errorf("L0-B gate must carry the IsRelationalLookup carve-out (expected substring %q) — without it the gate over-fires on \"filter set X by relation to Y\" questions like \"which packages import X\"", wantRelationalCarveOut)
	}
	if !strings.Contains(src, wantHistoryCarveOut) {
		t.Errorf("L0-B gate must carry the IsHistoryLookup carve-out (expected substring %q) — pure VCS history enumerations list commits discovered by git tools, not analyzer-time code entities", wantHistoryCarveOut)
	}
}

// TestL0B_GateClassification_TableDriven exercises the classification
// logic directly via a stub: any RequestModel passed to a function
// that mirrors the L0-B condition should classify correctly. This
// pins the contract behaviour so a future refactor that changes the
// condition (e.g. swapping AND for OR) breaks a test.
func TestL0B_GateClassification_TableDriven(t *testing.T) {
	cases := []struct {
		name       string
		cat        bool
		rel        bool
		entities   []string
		configure  func(*types.RequestModel)
		wantReject bool
	}{
		// Original L0-B contract — relational lookup unset.
		{"cat_true_zero_entities", true, false, nil, nil, true},
		{"cat_true_one_entity", true, false, []string{"PipelineStage"}, nil, true},
		{"cat_true_one_entity_with_blanks", true, false, []string{"PipelineStage", ""}, nil, true},
		{"cat_true_one_entity_dup", true, false, []string{"Foo", "FOO"}, nil, true},
		{"cat_true_two_distinct", true, false, []string{"StageAnalyze", "StageExplore"}, nil, false},
		{"cat_true_four_distinct", true, false, []string{"a", "b", "c", "d"}, nil, false},
		{"cat_false_one_entity", false, false, []string{"PipelineStage"}, nil, false},
		{"cat_false_zero_entities", false, false, nil, nil, false},
		{"cat_false_two_distinct", false, false, []string{"a", "b"}, nil, false},

		// 2026-05-08 carve-out — IsRelationalLookup=true + cat=true +
		// single entity is the structurally correct shape for "filter
		// set X by relation to Y" questions. Gate must NOT fire.
		{"relational_lookup_single_entity_pkg_importers",
			true, true, []string{"internal/analysis/criterion"}, nil, false}, // u4a shape
		{"relational_lookup_single_entity_pkg_dependents",
			true, true, []string{"internal/tool/ground"}, nil, false}, // u4b shape
		{"relational_lookup_single_entity_pkg_exports",
			true, true, []string{"criterion"}, nil, false}, // u8a shape
		{"relational_lookup_zero_entities",
			true, true, nil, nil, false}, // pre-classification, allow through
		// Edge case: relational lookup with multi-entity emit is also
		// fine (the LLM resolved more than just the target — bonus).
		{"relational_lookup_multi_entity",
			true, true, []string{"X", "Y"}, nil, false},
		// Edge case: cat=false + rel=true single-entity is unaffected
		// (gate already passes when cat=false).
		{"cat_false_rel_true_single", false, true, []string{"X"}, nil, false},
		{"history_lookup_zero_entities",
			true, false, nil, func(rm *types.RequestModel) {
				rm.Predicates.IsHistoryLookup = true
			}, false},
		{"history_lookup_one_entity",
			true, false, []string{"commit"}, func(rm *types.RequestModel) {
				rm.Predicates.IsHistoryLookup = true
			}, false},
		{"scoped_inventory_single_entity_with_required_files",
			true, false, []string{"criterion"}, func(rm *types.RequestModel) {
				rm.SourceScopeProfile = &types.SourceScopeProfile{RequestedScope: types.SourceScopeProduction, Confidence: 0.9}
				rm.AnalyzerHints.RequiredFileHints = []types.RequiredFileHint{{Path: "internal/analysis/criterion/grammar.go", Confidence: 0.95}}
			}, false},
		{"scoped_inventory_single_entity_with_subtopics",
			true, false, []string{"criterion"}, func(rm *types.RequestModel) {
				rm.SubTopics = []types.SubTopic{{Summary: "types"}, {Summary: "functions"}}
			}, false},
		{"required_files_without_scope_still_rejects",
			true, false, []string{"criterion"}, func(rm *types.RequestModel) {
				rm.AnalyzerHints.RequiredFileHints = []types.RequiredFileHint{{Path: "internal/analysis/criterion/grammar.go", Confidence: 0.95}}
			}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rm := types.RequestModel{
				Predicates: types.SemanticPredicates{
					IsCategoryEnumeration: c.cat,
					IsRelationalLookup:    c.rel,
				},
				AnalyzerHints: types.AnalyzerHints{Entities: c.entities},
			}
			if c.configure != nil {
				c.configure(&rm)
			}
			gotReject := shouldRejectEnumerationCardinality(&rm)
			if gotReject != c.wantReject {
				t.Errorf("classification: cat=%v rel=%v entities=%v → got reject=%v, want %v",
					c.cat, c.rel, c.entities, gotReject, c.wantReject)
			}
		})
	}
}

// readAnalyzerSource returns the analyzer.go source contents.
// Helper for source-level structural assertions (insertion points,
// gate presence) — same pattern as analyzer_amplifier_wiring_test.go.
func readAnalyzerSource(t *testing.T) string {
	t.Helper()
	src, err := os.ReadFile("analyzer.go")
	if err != nil {
		t.Fatalf("read analyzer.go: %v", err)
	}
	return string(src)
}
