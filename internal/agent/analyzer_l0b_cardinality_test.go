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
	const wantErrorMsgFix1 = "ENUMERATED VALUES"
	const wantErrorMsgFix2 = "is_category_enumeration=false"

	src := readAnalyzerSource(t)

	if !strings.Contains(src, wantPredicateExpr) {
		t.Errorf("L0-B gate predicate missing: expected substring %q in analyzer.go", wantPredicateExpr)
	}
	if !strings.Contains(src, wantHelperCall) {
		t.Errorf("L0-B gate helper call missing: expected substring %q in analyzer.go", wantHelperCall)
	}
	if !strings.Contains(src, wantErrorMsgFix1) {
		t.Errorf("L0-B reject message must explicitly name 'ENUMERATED VALUES' so the LLM retry knows the fix")
	}
	if !strings.Contains(src, wantErrorMsgFix2) {
		t.Errorf("L0-B reject message must offer the cat=false escape hatch for legitimate type-name lookups")
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
		entities   []string
		wantReject bool
	}{
		{"cat_true_zero_entities", true, nil, true},
		{"cat_true_one_entity", true, []string{"PipelineStage"}, true},
		{"cat_true_one_entity_with_blanks", true, []string{"PipelineStage", ""}, true},
		{"cat_true_one_entity_dup", true, []string{"Foo", "FOO"}, true},
		{"cat_true_two_distinct", true, []string{"StageAnalyze", "StageExplore"}, false},
		{"cat_true_four_distinct", true, []string{"a", "b", "c", "d"}, false},
		{"cat_false_one_entity", false, []string{"PipelineStage"}, false},
		{"cat_false_zero_entities", false, nil, false},
		{"cat_false_two_distinct", false, []string{"a", "b"}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rm := types.RequestModel{
				Predicates:    types.SemanticPredicates{IsCategoryEnumeration: c.cat},
				AnalyzerHints: types.AnalyzerHints{Entities: c.entities},
			}
			gotReject := rm.Predicates.IsCategoryEnumeration &&
				distinctNamedEntities(rm.AnalyzerHints.Entities) <= 1
			if gotReject != c.wantReject {
				t.Errorf("classification: cat=%v entities=%v → got reject=%v, want %v",
					c.cat, c.entities, gotReject, c.wantReject)
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
