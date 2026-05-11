package amplifier

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// distinctEntityCount counts unique entries in AnalyzerHints.Entities
// after trimming whitespace and case-folding for the comparison key.
// The original surface forms are preserved in the slice; this helper
// only answers the cardinality question for R1's gate.
//
// We use AnalyzerHints.Entities (the analyzer LLM's typed entity
// emit) rather than TermGraph.Canonical (which only contains
// surfaces extracted from RawRequest text) because Chinese /
// natural-language questions almost never put symbol names verbatim
// in the question — they describe the concept and the LLM names the
// symbols in its analysis output. The empirical evidence is the
// 2026-05-05 qf_arch run-2: LLM emitted four stage entities
// (StageAnalyze / StageExplore / StageExtract / StageFinalize) but
// TermGraph.Canonical was empty for symbol kinds because none of
// those identifiers appeared in the user's Chinese question text.
func distinctEntityCount(entities []string) int {
	if len(entities) == 0 {
		return 0
	}
	seen := make(map[string]struct{}, len(entities))
	for _, e := range entities {
		key := strings.ToLower(strings.TrimSpace(e))
		if key == "" {
			continue
		}
		seen[key] = struct{}{}
	}
	return len(seen)
}

// r1MultiSubjectPredicate fires when typed signals point at a
// multi-subject enumeration the LLM left unmarked. See
// docs/design/analyzer_amplifier_layer.md §5 R1.
//
// Gates (all must hold for the rule to fire):
//
//  1. distinctEntityCount(rm.AnalyzerHints.Entities) ≥ 2 — the LLM
//     emitted at least two distinct named subjects. The original
//     design read TermGraph.Canonical with a confidence-gap guard;
//     2026-05-05 real eval surfaced that TermGraph is populated
//     from RawRequest surfaces only, so Chinese / natural-language
//     questions whose entities are LLM-named (not user-typed) never
//     cleared the TermGraph gate. AnalyzerHints.Entities is the
//     direct typed signal for "subjects the LLM identified".
//  2. rm.Predicates.IsCategoryEnumeration == false — red line #2
//     forbids overriding the LLM's explicit positive.
//  3. rm.Intent ∈ {Explain, Trace} — narrows the rule
//     to question kinds where multi-subject often indicates the
//     user wants a categorised list. Excludes IntentEnumerate
//     (already marked), IntentRootCause (multi-entity diagnostics
//     usually name observed frames/components, not enumerated answer
//     members), IntentConfigQuery / IntentReturnValue (scalar lanes),
//     and IntentUnknown (LLM gave up — don't compound the guess).
//  4. NOT structural endpoint trace — a source→sink / call-chain
//     question may legitimately carry two exact targets plus nearby
//     context entities, but the principal answer is still one ordered
//     trace, not a category enumeration. This guard reads only typed
//     analyzer fields (Intent, PredicateAxis, AnalyzerHints.Kind,
//     raw-request-validated exact_targets, predicates) so it stays
//     language-neutral and avoids raw-request keyword matching.
//  5. len(types.ExactResolutionTargets(rm)) != 1 — a single exact
//     resolution target is a precise scalar lookup, not an
//     enumeration even when multiple entities exist on the model.
//  6. rm.Predicates.IsScalarAnswer == false — the typed scalar-
//     answer signal stands in for the (non-existent) SubjectScalar
//     mentioned in the design doc; either form means "answer is
//     one literal, not a set", which is incompatible with
//     enumeration.
//  7. NOT (len(rm.SubTopics) >= 2 AND !rm.Predicates.IsCrossComponent).
//     Axis-collapse alignment: when the LLM has emitted ≥2
//     SubTopics in a single-component question, flipping
//     IsCategoryEnumeration to true creates exactly the four
//     conditions axis_collapse rejects (nSub≥2 + !IsCrossComponent
//     + IsCategoryEnumeration + ≤1 distinct domain). Trust the
//     LLM's structured SubTopics emit and skip R1 in this scenario.
//     If LLM marked IsCrossComponent=true, the SubTopics
//     legitimately span ≥2 subsystems and axis_collapse will not
//     trigger — R1 may fire safely. Empirical: 2026-05-05 m1a runs
//     all had IsCrossComponent=true so this gate did not block any
//     m1a R1 firing. See internal/analysis/gate/coherence.go::R1.4.
//
// The action sets IsCategoryEnumeration=true and emits one
// Observation. No other slot is touched: SubTopics derivation is
// R2's job (Phase 3); MustInclude pinning is R3's job (Phase 4).
func r1MultiSubjectPredicate(in types.RequestModel, out *types.RequestModel) *Observation {
	if out.Predicates.IsCategoryEnumeration {
		return nil
	}
	if out.Predicates.IsScalarAnswer {
		return nil
	}
	switch out.Intent {
	case types.IntentExplain, types.IntentTrace:
		// fall through to the structural checks
	default:
		return nil
	}
	// Axis-collapse alignment gate: see #6 in the doc above.
	if len(out.SubTopics) >= 2 && !out.Predicates.IsCrossComponent {
		return nil
	}
	if isStructuralEndpointTraceQuestion(*out) {
		return nil
	}
	count := distinctEntityCount(out.AnalyzerHints.Entities)
	if count < 2 {
		return nil
	}
	if len(types.ExactResolutionTargets(*out)) == 1 {
		return nil
	}
	out.Predicates.IsCategoryEnumeration = true
	return &Observation{
		Rule:   "R1_multi_subject_predicate",
		Field:  "predicates.is_category_enumeration",
		Before: "false",
		After:  "true",
		Reason: fmt.Sprintf("AnalyzerHints.Entities has %d distinct named subjects with intent=%s and non-scalar answer", count, out.Intent),
	}
}

func isStructuralEndpointTraceQuestion(rm types.RequestModel) bool {
	if rm.Intent != types.IntentTrace {
		return false
	}
	if rm.Predicates.IsScalarAnswer ||
		rm.Predicates.IsRelationalLookup ||
		rm.Predicates.IsCategoryEnumeration ||
		rm.Predicates.IsCountQuestion ||
		rm.Predicates.IsHistoryLookup ||
		rm.Predicates.IsDiagnosticQuestion {
		return false
	}
	if len(structuralEndpointTraceTargets(rm)) < 2 {
		return false
	}
	switch rm.PredicateAxis {
	case types.AxisCall, types.AxisCondition, types.AxisRegister:
		return true
	}
	switch types.NormalizeRequirementKind(rm.AnalyzerHints.Kind) {
	case types.ReqCallChain, types.ReqConditional, types.ReqMechanism, types.ReqRegistration:
		return true
	}
	return false
}

func structuralEndpointTraceTargets(rm types.RequestModel) []string {
	if targets := types.ExactResolutionTargets(rm); len(targets) > 0 {
		return targets
	}
	return types.MentionedEntitiesFromRawRequest(rm.RawRequest, rm.AnalyzerHints.ExactTargets)
}

func init() {
	preCompileRules = append(preCompileRules, r1MultiSubjectPredicate)
}
