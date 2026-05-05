package amplifier

import (
	"fmt"
	"sort"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// topSymbolsLikeHDP mirrors internal/analysis/hdp/planner.go::topSymbols.
// We duplicate the algorithm rather than importing hdp because hdp.Plan
// has side effects (hypothesis generation) and we only want the symbol
// shortlist. The two implementations must stay in sync — a structural
// test below re-derives the contract from CanonicalTerm shape so a
// drift in either copy fails its package's test.
//
// Algorithm:
//   - keep only Canonical entries with Kind == TermSymbol and a
//     non-blank Surface
//   - sort by Confidence desc, ties broken by original index
//   - if there is a sharp confidence gap (top - second > 0.4),
//     truncate to the single top match (the runner-up is too weak
//     to anchor a multi-subject claim — rarity-penalised kindEnWord
//     hits land at ~0.3 while direct CamelCase hits land at 1.0)
//   - cap at k entries
func topSymbolsLikeHDP(tg types.TermGraph, k int) []string {
	if k <= 0 {
		return nil
	}
	type pair struct {
		surface string
		conf    float32
		idx     int
	}
	var ps []pair
	for i, c := range tg.Canonical {
		if c.Kind != types.TermSymbol {
			continue
		}
		if strings.TrimSpace(c.Surface) == "" {
			continue
		}
		ps = append(ps, pair{surface: c.Surface, conf: c.Confidence, idx: i})
	}
	sort.SliceStable(ps, func(i, j int) bool {
		if ps[i].conf != ps[j].conf {
			return ps[i].conf > ps[j].conf
		}
		return ps[i].idx < ps[j].idx
	})
	if len(ps) >= 2 && ps[0].conf-ps[1].conf > 0.4 {
		ps = ps[:1]
	}
	if len(ps) > k {
		ps = ps[:k]
	}
	out := make([]string, 0, len(ps))
	for _, p := range ps {
		out = append(out, p.surface)
	}
	return out
}

// r1MultiSubjectPredicate fires when typed signals point at a
// multi-subject enumeration the LLM left unmarked. See
// docs/design/analyzer_amplifier_layer.md §5 R1.
//
// Gates (all must hold for the rule to fire):
//
//  1. topSymbolsLikeHDP returns ≥ 2 surfaces — the typed-confidence
//     pass already encodes the "multi-subject signal with no sharp
//     gap" structural test.
//  2. rm.Predicates.IsCategoryEnumeration == false — red line #2
//     forbids overriding the LLM's explicit positive.
//  3. rm.Intent ∈ {Explain, Trace, RootCause} — narrows the rule
//     to question kinds where multi-subject often indicates the user
//     wants a categorised list. Excludes IntentEnumerate (already
//     marked), IntentConfigQuery / IntentReturnValue (scalar lanes),
//     and IntentUnknown (LLM gave up — don't compound the guess).
//  4. len(types.ExactResolutionTargets(rm)) != 1 — a single exact
//     resolution target is a precise scalar lookup, not an
//     enumeration even when multiple symbols exist in the graph.
//  5. rm.Predicates.IsScalarAnswer == false — the typed scalar-answer
//     signal stands in for the (non-existent) SubjectScalar mentioned
//     in the design doc; either form means "answer is one literal,
//     not a set", which is incompatible with enumeration.
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
	syms := topSymbolsLikeHDP(out.TermGraph, 3)
	if len(syms) < 2 {
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
		Reason: fmt.Sprintf("HDP topSymbols=%d (no sharp gap) with intent=%s and non-scalar answer", len(syms), out.Intent),
	}
}

func init() {
	preCompileRules = append(preCompileRules, r1MultiSubjectPredicate)
}
