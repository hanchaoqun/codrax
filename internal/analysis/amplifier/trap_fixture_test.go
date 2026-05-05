package amplifier

// This file is a TRAP-FIXTURE TEST: every new rule landing in the
// amplifier package MUST pass these scenarios. They exist to defend
// against the failure mode that bit Phase 2.1 (commit 8dfd27b) and
// took two follow-up commits (7bf5ce5 + bc340a4) to chase down.
//
// The trap: rm.TermGraph.Canonical LOOKS like "the place to read
// LLM-emitted entities" — it has typed Kind / Confidence / Surface
// fields. But TermGraph is built by normalizer.Normalize from
// SURFACES EXTRACTED FROM rm.RawRequest TEXT. LLM-emitted entity
// names that the user did NOT type verbatim into the question text
// (a) do not appear in TermGraph.Canonical and (b) do not get
// promoted to TermKind=TermSymbol. Chinese / natural-language
// questions almost always fit this shape: the user describes a
// concept ("what stages does the read-mode pipeline have"), and
// the LLM names the symbols (StageAnalyze / StageExplore / ...)
// in its analyzer emit.
//
// The right entity slot for "subjects the LLM identified" is
// rm.AnalyzerHints.Entities. That field is populated by
// emit_analysis directly and is independent of whether the
// surfaces appear in raw text.
//
// Trap-fixture invariant: a RequestModel with EMPTY TermGraph but
// POPULATED AnalyzerHints.Entities must still drive every rule
// to the same firing decision it would make on a fully-populated
// model. If your rule cares about "did the LLM identify multiple
// subjects?" or "are there parallel-named entities?", read
// AnalyzerHints.Entities — never TermGraph.Canonical.
//
// When you add a new rule:
//  1. Add a case below for whatever your rule's gate cares about.
//  2. The new case MUST use TermGraph: types.TermGraph{} (zero) so
//     the test pins the no-TermGraph behaviour.
//  3. Run `go test ./internal/analysis/amplifier/`. If your rule
//     compiles but mis-uses TermGraph, this test will catch it.

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// TestAmplifier_TrapFixture_EmptyTermGraphMultiEntity asserts that
// the canonical empirical case (Chinese qf_arch-style question,
// four stage entities only present in AnalyzerHints.Entities,
// empty TermGraph because none of those names appear in the raw
// question) still drives R1 + R2 to fire correctly when the
// question is cross-component.
//
// Failure mode this defends: a rule reads rm.TermGraph.Canonical
// to count subjects. With empty TermGraph the rule no-ops despite
// the LLM having emitted 4 obvious subjects.
//
// We use IsCrossComponent=true so both R1 and R2 reach their
// firing path past the axis-collapse alignment gate. The pure
// single-component enumeration case is covered by
// TestAmplifier_AxisCollapseFixture_SingleComponentEnumeration in
// axis_collapse_fixture_test.go.
func TestAmplifier_TrapFixture_EmptyTermGraphMultiEntity(t *testing.T) {
	rm := types.RequestModel{
		Intent: types.IntentExplain,
		// TermGraph is intentionally zero — this is the empirical
		// shape that broke pre-bc340a4 R1.
		TermGraph: types.TermGraph{},
		AnalyzerHints: types.AnalyzerHints{
			Entities: []string{"StageAnalyze", "StageExplore", "StageExtract", "StageFinalize"},
		},
		Predicates: types.SemanticPredicates{
			IsCrossComponent: true, // pass axis-collapse alignment gates
		},
	}
	got, obs := Amplify(rm)

	if !got.Predicates.IsCategoryEnumeration {
		t.Errorf("R1 must fire on empty-TermGraph + multi-entity input — see trap-fixture doc above")
	}
	if len(got.SubTopics) == 0 {
		t.Errorf("R2 must fire on empty-TermGraph + parallel-named entities — see trap-fixture doc above")
	}

	// At least one R1 and one R2 observation expected.
	hasR1, hasR2 := false, false
	for _, o := range obs {
		switch o.Rule {
		case "R1_multi_subject_predicate":
			hasR1 = true
		case "R2_typed_name_parity_subtopics":
			hasR2 = true
		}
	}
	if !hasR1 {
		t.Errorf("expected R1 observation; got %+v", obs)
	}
	if !hasR2 {
		t.Errorf("expected R2 observation; got %+v", obs)
	}
}

// TestAmplifier_TrapFixture_EmptyTermGraphSingleEntity is the
// negative pair: empty TermGraph + single entity must NOT trigger
// any rule that gates on multi-subject. Defends against a rule
// that "fixes" the trap above by always firing — R1 / R2 must
// still respect their cardinality gates.
func TestAmplifier_TrapFixture_EmptyTermGraphSingleEntity(t *testing.T) {
	rm := types.RequestModel{
		Intent:        types.IntentExplain,
		TermGraph:     types.TermGraph{},
		AnalyzerHints: types.AnalyzerHints{Entities: []string{"PipelineStage"}},
	}
	got, obs := Amplify(rm)

	if got.Predicates.IsCategoryEnumeration {
		t.Errorf("R1 must NOT fire on single-entity input regardless of TermGraph state")
	}
	if len(got.SubTopics) != 0 {
		t.Errorf("R2 must NOT fire on single-entity input (no parallel-naming family possible), got %+v", got.SubTopics)
	}
	for _, o := range obs {
		if o.Rule == "R1_multi_subject_predicate" || o.Rule == "R2_typed_name_parity_subtopics" {
			t.Errorf("unexpected firing on single-entity input: %+v", o)
		}
	}
}

// TestAmplifier_TrapFixture_EmptyTermGraphLLMAlreadyTrue asserts
// that even on the canonical empty-TermGraph shape, red line #2
// (never override LLM positive) holds. Defends against a rule
// that conditions on `len(TermGraph.Canonical) > 0` to gate the
// red-line check, accidentally letting it fire under empty
// TermGraph regardless of LLM state.
func TestAmplifier_TrapFixture_EmptyTermGraphLLMAlreadyTrue(t *testing.T) {
	rm := types.RequestModel{
		Intent:    types.IntentExplain,
		TermGraph: types.TermGraph{},
		AnalyzerHints: types.AnalyzerHints{
			Entities: []string{"StageAnalyze", "StageExplore", "StageExtract"},
		},
		Predicates: types.SemanticPredicates{
			IsCategoryEnumeration: true, // LLM already filled
		},
		SubTopics: []types.SubTopic{
			{Summary: "user-supplied", Entities: []string{"X"}},
		},
	}
	got, obs := Amplify(rm)

	if !got.Predicates.IsCategoryEnumeration {
		t.Errorf("LLM-set IsCategoryEnumeration must not be cleared")
	}
	if len(got.SubTopics) != 1 || got.SubTopics[0].Summary != "user-supplied" {
		t.Errorf("LLM-set SubTopics must not be replaced, got %+v", got.SubTopics)
	}
	for _, o := range obs {
		if o.Rule == "R1_multi_subject_predicate" || o.Rule == "R2_typed_name_parity_subtopics" {
			t.Errorf("rule fired despite LLM already filling its slot — red line #2 breach: %+v", o)
		}
	}
}
