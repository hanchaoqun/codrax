package amplifier

// This file is the second TRAP-FIXTURE TEST suite for the amplifier
// package. It defends against the failure mode that bit Phase 3.1
// (commit e4ab1ea) and was caught the same day at Phase 3.2 real
// eval (qf_arch run-1, 4 retries exhausted, eval FAIL).
//
// # The axis-collapse trap
//
// The downstream gate internal/analysis/gate/coherence.go::R1.4
// (axis_collapse) is a HARD reject. It rejects analyzer emit when
// ALL of these conditions hold:
//
//   1. len(rm.SubTopics) >= 2
//   2. !rm.Predicates.IsCrossComponent
//   3. rm.Predicates.IsCategoryEnumeration OR rm.Intent==IntentEnumerate
//   4. SubTopic entities all resolve to ≤1 distinct repomap Domain
//
// When axis_collapse rejects, the analyzer retries; after the
// budget is exhausted the entire Run terminates with no answer.
//
// Two amplifier rules can independently steer a RequestModel into
// this combination:
//
//  - R1 flips IsCategoryEnumeration true. If the LLM had already
//    emitted ≥2 SubTopics in a single-component question (and
//    those SubTopics happen to share a domain), R1 has just
//    activated condition #3 — axis_collapse triggers on the next
//    gate run.
//  - R2 derives SubTopics from a same-affix family. If
//    IsCategoryEnumeration is already true (LLM-emit or R1-flip)
//    and IsCrossComponent is false, R2 has just activated
//    condition #1 with same-domain content — axis_collapse
//    triggers.
//
// Both rules now have explicit alignment gates; this fixture pins
// them. New rules that touch IsCategoryEnumeration or SubTopics
// MUST add cases here proving they don't recreate the trap.
//
// # Authoring rule for IsCategoryEnumeration / SubTopics writers
//
// Before flipping IsCategoryEnumeration to true OR appending to
// SubTopics, ask: "would the resulting RequestModel satisfy the
// four axis_collapse conditions?" If yes, your rule needs an
// alignment gate similar to:
//
//   if out.Predicates.IsCategoryEnumeration && !out.Predicates.IsCrossComponent {
//       return nil  // axis_collapse alignment skip
//   }
//
// Or for rules that flip the predicate itself:
//
//   if len(out.SubTopics) >= 2 && !out.Predicates.IsCrossComponent {
//       return nil  // would invalidate LLM's existing SubTopics
//   }

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// TestAmplifier_AxisCollapseFixture_SingleComponentEnumeration
// pins the qf_arch-shape regression: a single-component
// enumeration question with a same-prefix entity family. R2 must
// not derive SubTopics here. R1 may set IsCategoryEnumeration
// either way (no SubTopics yet → R1 gate #6 doesn't apply), but
// the resulting model must not satisfy all four axis_collapse
// conditions.
func TestAmplifier_AxisCollapseFixture_SingleComponentEnumeration(t *testing.T) {
	rm := types.RequestModel{
		Intent: types.IntentExplain,
		AnalyzerHints: types.AnalyzerHints{
			Entities: []string{"StageAnalyze", "StageExplore", "StageExtract", "StageFinalize"},
		},
		Predicates: types.SemanticPredicates{
			IsCrossComponent: false, // single-component
		},
	}
	got, _ := Amplify(rm)

	// After amplifier, axis_collapse must not be activatable.
	wouldTriggerAxisCollapse := len(got.SubTopics) >= 2 &&
		!got.Predicates.IsCrossComponent &&
		(got.Predicates.IsCategoryEnumeration || got.Intent == types.IntentEnumerate)
	if wouldTriggerAxisCollapse {
		t.Errorf("amplifier produced a RequestModel that satisfies axis_collapse trigger "+
			"conditions (subt=%d cross=%v cat=%v intent=%v) — see axis_collapse_fixture_test.go doc",
			len(got.SubTopics), got.Predicates.IsCrossComponent,
			got.Predicates.IsCategoryEnumeration, got.Intent)
	}
}

// TestAmplifier_AxisCollapseFixture_LLMEmittedSameDomainSubTopics
// pins R1's gate #6: when the LLM has already emitted ≥2 SubTopics
// in a single-component question, R1 must not flip
// IsCategoryEnumeration to true (which would weaponise those
// SubTopics through axis_collapse).
func TestAmplifier_AxisCollapseFixture_LLMEmittedSameDomainSubTopics(t *testing.T) {
	rm := types.RequestModel{
		Intent: types.IntentExplain,
		AnalyzerHints: types.AnalyzerHints{
			Entities: []string{"AlphaSetup", "AlphaTeardown"},
		},
		SubTopics: []types.SubTopic{
			{Summary: "alpha-setup-axis", Entities: []string{"AlphaSetup"}},
			{Summary: "alpha-teardown-axis", Entities: []string{"AlphaTeardown"}},
		},
		Predicates: types.SemanticPredicates{
			IsCrossComponent: false, // single-component
			// LLM did NOT mark this as enumeration.
			IsCategoryEnumeration: false,
		},
	}
	got, _ := Amplify(rm)

	if got.Predicates.IsCategoryEnumeration {
		t.Errorf("R1 flipped IsCategoryEnumeration on a single-component question with " +
			"≥2 LLM-emitted SubTopics — would activate axis_collapse downstream")
	}

	wouldTriggerAxisCollapse := len(got.SubTopics) >= 2 &&
		!got.Predicates.IsCrossComponent &&
		(got.Predicates.IsCategoryEnumeration || got.Intent == types.IntentEnumerate)
	if wouldTriggerAxisCollapse {
		t.Errorf("amplifier output satisfies axis_collapse trigger conditions: " +
			"subt≥2, !IsCrossComponent, IsCategoryEnumeration=true")
	}
}

// TestAmplifier_AxisCollapseFixture_CrossComponentEscape verifies
// the escape clause: when the LLM marks the question as cross-
// component, axis_collapse cannot trigger regardless of SubTopic
// domain count, so amplifier rules may fire freely. This is the
// m1a-shape case (multi-axis cross-package question).
func TestAmplifier_AxisCollapseFixture_CrossComponentEscape(t *testing.T) {
	rm := types.RequestModel{
		Intent: types.IntentExplain,
		AnalyzerHints: types.AnalyzerHints{
			Entities: []string{"AgentExplorer", "AgentExtractor"},
		},
		Predicates: types.SemanticPredicates{
			IsCrossComponent: true, // legitimately spans subsystems
		},
	}
	got, obs := Amplify(rm)

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
		t.Errorf("R1 should fire on cross-component multi-entity input — IsCrossComponent " +
			"escape clause expected")
	}
	if !hasR2 {
		t.Errorf("R2 should fire on cross-component multi-entity input — IsCrossComponent " +
			"escape clause expected")
	}

	// Even with R1+R2 firing, the cross-component escape means
	// axis_collapse condition #2 (!IsCrossComponent) fails, so the
	// resulting model is safe.
	if !got.Predicates.IsCrossComponent {
		t.Errorf("amplifier dropped IsCrossComponent=true; would enable axis_collapse")
	}
}
