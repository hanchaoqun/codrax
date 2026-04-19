package agent

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// TestReconcileComplexity covers every rule in the catalogue + the
// identity case (no rule fires, declared returned unchanged). After
// the schema-v4 rewrite the prose-cue tables are gone — Rules 2/3/7
// dispatch on RequestModel.Predicates fields the LLM emits in any
// language.
func TestReconcileComplexity(t *testing.T) {
	type row struct {
		name         string
		declared     types.Complexity
		entities     []string
		keywords     []string
		subTopics    int
		questionKind string
		preds        types.SemanticPredicates
		wantResult   types.Complexity
		wantReason   bool // true when a reason string is expected (non-empty)
	}
	cases := []row{
		// Identity — no rule applies.
		{"simple passes through on short lookup", types.ComplexitySimple,
			[]string{"login"}, []string{"login", "function"}, 0, "",
			types.SemanticPredicates{}, types.ComplexitySimple, false},
		{"moderate passes through on mechanism question", types.ComplexityModerate,
			[]string{"auth"}, []string{"auth", "module", "work"}, 0, "",
			types.SemanticPredicates{}, types.ComplexityModerate, false},

		// Rule 1: subTopics>=3 → complex (structural, no predicates).
		{"3 subtopics forces complex", types.ComplexityModerate,
			[]string{"A", "B"}, []string{"A", "B", "C"}, 3, "",
			types.SemanticPredicates{}, types.ComplexityComplex, true},
		{"2 subtopics does not trigger rule 1", types.ComplexityModerate,
			[]string{"X", "Y"}, []string{"X", "Y"}, 2, "",
			types.SemanticPredicates{}, types.ComplexityModerate, false},

		// Rule 2: predicates.IsCrossComponent → complex (no prose).
		{"is_cross_component upgrades to complex", types.ComplexityModerate,
			[]string{"logger", "tracer"}, []string{"logger", "tracer"}, 0, "",
			types.SemanticPredicates{IsCrossComponent: true},
			types.ComplexityComplex, true},
		{"is_cross_component upgrades simple to complex", types.ComplexitySimple,
			[]string{"A", "B"}, []string{"A", "B"}, 0, "",
			types.SemanticPredicates{IsCrossComponent: true},
			types.ComplexityComplex, true},

		// Rule 3: declared complex + scalar predicate + thin prompt → simple.
		{"complex + is_scalar_answer + 1 entity downgrades to simple", types.ComplexityComplex,
			[]string{"repomap"}, []string{"repomap"}, 0, "",
			types.SemanticPredicates{IsScalarAnswer: true},
			types.ComplexitySimple, true},
		{"complex + is_scalar_answer but >1 entity stays complex (rule 3 entity ceiling)", types.ComplexityComplex,
			[]string{"a", "b"}, []string{"a", "b"}, 0, "",
			types.SemanticPredicates{IsScalarAnswer: true},
			types.ComplexityComplex, false},

		// Rule 4: 5+ entities and 10+ keywords → complex regardless of LLM pick.
		{"five entities + rich keywords upgrade", types.ComplexityModerate,
			[]string{"A", "B", "C", "D", "E"},
			[]string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j"}, 0, "",
			types.SemanticPredicates{}, types.ComplexityComplex, true},

		// Rule 5: zero entities + tiny keyword pool → simple floor.
		{"empty entities + 3 keywords downgrades from complex", types.ComplexityComplex,
			nil, []string{"lang", "?", "question"}, 0, "",
			types.SemanticPredicates{}, types.ComplexitySimple, true},

		// Rule 6 (mechanism/call_chain + 2+ entities) — structural.
		{"mechanism + 2 entities upgrades moderate→complex", types.ComplexityModerate,
			[]string{"explorer", "subagent"},
			[]string{"explorer", "subagent", "call"}, 0, "mechanism",
			types.SemanticPredicates{}, types.ComplexityComplex, true},
		{"call_chain + 2 entities upgrades simple→complex", types.ComplexitySimple,
			[]string{"A", "B"},
			[]string{"A", "B", "call"}, 0, "call_chain",
			types.SemanticPredicates{}, types.ComplexityComplex, true},
		{"mechanism + 1 entity stays moderate (single component)", types.ComplexityModerate,
			[]string{"X"},
			[]string{"X", "work"}, 0, "mechanism",
			types.SemanticPredicates{}, types.ComplexityModerate, false},

		// Rule 7: enumeration / category cue + relational lookup → moderate.
		// Replaces the old "leading enumeration cue + relational verb"
		// prose check; signal now comes from LLM predicates.
		{"is_count_question + is_relational_lookup upgrades simple→moderate",
			types.ComplexitySimple, nil, []string{"agent", "subagent"}, 0, "",
			types.SemanticPredicates{IsCountQuestion: true, IsRelationalLookup: true},
			types.ComplexityModerate, true},
		{"is_category_enumeration + is_relational_lookup upgrades simple→moderate",
			types.ComplexitySimple, nil, []string{"agent", "executor"}, 0, "",
			types.SemanticPredicates{IsCategoryEnumeration: true, IsRelationalLookup: true},
			types.ComplexityModerate, true},
		// Negative: count without relational lookup stays simple
		// (measurement-scalar carve-out applies downstream).
		{"is_count_question alone stays simple (no relational lookup)",
			types.ComplexitySimple, nil, []string{"python", "files"}, 0, "",
			types.SemanticPredicates{IsCountQuestion: true},
			types.ComplexitySimple, false},
		// Negative: relational lookup alone stays simple.
		{"is_relational_lookup alone stays simple (no count cue)",
			types.ComplexitySimple, []string{"router", "handler"},
			[]string{"router", "call", "handler"}, 0, "",
			types.SemanticPredicates{IsRelationalLookup: true},
			types.ComplexitySimple, false},
		// Rule 7 only lifts simple, not moderate/complex.
		{"moderate enumeration+relational is left alone",
			types.ComplexityModerate, nil, []string{"agent", "subagent"}, 0, "",
			types.SemanticPredicates{IsCountQuestion: true, IsRelationalLookup: true},
			types.ComplexityModerate, false},

		// Conflict ordering — Rule 1 fires before Rule 3.
		{"subTopics>=3 beats scalar downgrade", types.ComplexityComplex,
			[]string{"X"}, []string{"X"}, 4, "",
			types.SemanticPredicates{IsScalarAnswer: true},
			types.ComplexityComplex, false}, // already complex, rule 1 is no-op because declared==complex
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, reason := reconcileComplexity(c.declared, c.entities, c.keywords, c.subTopics, c.questionKind, c.preds)
			if got != c.wantResult {
				t.Errorf("result = %q, want %q (reason=%q)", got, c.wantResult, reason)
			}
			if c.wantReason && reason == "" {
				t.Errorf("expected a reason when result changed; got empty")
			}
			if !c.wantReason && reason != "" {
				t.Errorf("expected no reason when result unchanged; got %q", reason)
			}
		})
	}
}
