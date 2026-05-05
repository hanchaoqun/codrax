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
//  3. rm.Intent ∈ {Explain, Trace, RootCause} — narrows the rule
//     to question kinds where multi-subject often indicates the
//     user wants a categorised list. Excludes IntentEnumerate
//     (already marked), IntentConfigQuery / IntentReturnValue
//     (scalar lanes), and IntentUnknown (LLM gave up — don't
//     compound the guess).
//  4. len(types.ExactResolutionTargets(rm)) != 1 — a single exact
//     resolution target is a precise scalar lookup, not an
//     enumeration even when multiple entities exist on the model.
//  5. rm.Predicates.IsScalarAnswer == false — the typed scalar-
//     answer signal stands in for the (non-existent) SubjectScalar
//     mentioned in the design doc; either form means "answer is
//     one literal, not a set", which is incompatible with
//     enumeration.
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
	case types.IntentExplain, types.IntentTrace, types.IntentRootCause:
		// fall through to the structural checks
	default:
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

func init() {
	preCompileRules = append(preCompileRules, r1MultiSubjectPredicate)
}
