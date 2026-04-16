// Package hdp implements the Hypothesis-Driven Planning layer of
// the Analyzer v3 refactor. It produces a set of falsifiable
// hypotheses from a RequestModel; the separate binder package
// attaches each hypothesis to the TaskGraph node(s) that will
// verify it.
//
// This package is intentionally narrow:
//
//   - Plan(rm) returns a candidate HypothesisSet. Every hypothesis
//     carries both a RequiredEvidence list (what the extractor must
//     observe before declaring the hypothesis confirmed) and a
//     FalsificationCondition (the mechanical criterion that flips
//     it to rejected). Both are evaluated by the criterion package.
//
//   - Validate(tg) is the invariant check the analyzer quality gate
//     calls before accepting an IR ("every binding-eligible node
//     binds ≥1 hypothesis").
//
// Priority values come from the priority package, not from
// hardcoded constants — see priority.Score.
package hdp

import (
	"fmt"
	"sort"
	"strings"

	"github.com/hanchaoqun/codrax/internal/analysis/priority"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Plan generates a set of candidate hypotheses from Intent,
// TermGraph, Ambiguities, and RiskMatrix. The output is sorted by
// priority desc with stable idx tie-breaking.
func Plan(rm types.RequestModel) []types.Hypothesis {
	var out []types.Hypothesis
	nextID := func() string { return fmt.Sprintf("h%d", len(out)+1) }

	topSymbol := firstSymbol(rm.TermGraph)

	// 1. Intent-driven seed hypothesis.
	switch rm.Intent {
	case types.IntentRootCause:
		out = append(out, types.Hypothesis{
			ID: nextID(),
			Statement: fmt.Sprintf(
				"The observed symptom is caused by %s or a module it depends on.",
				orDefault(topSymbol, "the primary subject of the request")),
			RequiredEvidence: []types.Criterion{
				{Kind: types.CritSymbolPresent, Expr: topSymbol},
				{Kind: types.CritEvidenceCount, Expr: ">=2"},
			},
			FalsificationCondition: types.Criterion{Kind: types.CritNoCallSites, Expr: topSymbol},
			Status:                 types.HypUnknown,
		})
	case types.IntentConfigQuery:
		out = append(out, types.Hypothesis{
			ID: nextID(),
			Statement: "The effective config value is resolved by a single deterministic chain from default to override.",
			RequiredEvidence: []types.Criterion{
				{Kind: types.CritEvidenceCount, Expr: ">=1"},
			},
			FalsificationCondition: types.Criterion{Kind: types.CritMultipleResolutionChains},
			Status:                 types.HypUnknown,
		})
	case types.IntentReturnValue, types.IntentEnumerate:
		out = append(out, types.Hypothesis{
			ID: nextID(),
			Statement: fmt.Sprintf(
				"The answer is a finite set of symbols anchored on %s.",
				orDefault(topSymbol, "the subject")),
			RequiredEvidence: []types.Criterion{
				{Kind: types.CritAnswerSetBounded, Expr: "<=50"},
			},
			FalsificationCondition: types.Criterion{Kind: types.CritAnswerSetUnbounded},
			Status:                 types.HypUnknown,
		})
	default:
		out = append(out, types.Hypothesis{
			ID: nextID(),
			Statement: fmt.Sprintf(
				"The request can be answered using evidence anchored on %s.",
				orDefault(topSymbol, "repo symbols")),
			RequiredEvidence: []types.Criterion{
				{Kind: types.CritEvidenceCount, Expr: ">=1"},
			},
			FalsificationCondition: types.Criterion{Kind: types.CritNoRelevantEvidence},
			Status:                 types.HypUnknown,
		})
	}

	// 2. Ambiguities — each becomes a hypothesis to resolve.
	for _, amb := range rm.Ambiguities {
		if strings.TrimSpace(amb.Clause) == "" {
			continue
		}
		statement := fmt.Sprintf("The user's intent for %q is: %s.",
			amb.Clause,
			firstNonEmpty(amb.Resolution, firstOption(amb.Options), "to be determined"))
		out = append(out, types.Hypothesis{
			ID:        nextID(),
			Statement: statement,
			RequiredEvidence: []types.Criterion{
				{Kind: types.CritEvidenceCount, Expr: ">=1"},
			},
			FalsificationCondition: types.Criterion{Kind: types.CritUserClauseUnresolved, Expr: amb.Clause},
			Status:                 types.HypUnknown,
		})
	}

	// 3. Risk-driven hypotheses — one per elevated risk dimension.
	if rm.RiskMatrix.Security.Level >= 3 {
		out = append(out, types.Hypothesis{
			ID:        nextID(),
			Statement: "The change does not introduce an un-sanitized data path from an untrusted boundary.",
			RequiredEvidence: []types.Criterion{
				{Kind: types.CritEvidenceCount, Expr: ">=1"},
			},
			FalsificationCondition: types.Criterion{Kind: types.CritUntrustedReachesSink},
			Status:                 types.HypUnknown,
		})
	}
	if rm.RiskMatrix.DataIntegrity.Level >= 3 {
		out = append(out, types.Hypothesis{
			ID:        nextID(),
			Statement: "The change preserves all existing data invariants and is safe under concurrent writes.",
			RequiredEvidence: []types.Criterion{
				{Kind: types.CritEvidenceCount, Expr: ">=1"},
			},
			FalsificationCondition: types.Criterion{Kind: types.CritInvariantBroken},
			Status:                 types.HypUnknown,
		})
	}

	// Assign priorities via the priority scorer. idx is the
	// original slice position so ties break deterministically.
	for i := range out {
		out[i].Priority = priority.Score(out[i], rm, i)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Priority > out[j].Priority
	})
	return out
}

// Validate enforces the "non-probe nodes bind ≥1 hypothesis"
// invariant. It returns the list of offending node IDs so the
// analyzer quality gate can emit an actionable reject reason.
func Validate(tg types.TaskGraph) []string {
	var missing []string
	for _, n := range tg.Nodes {
		if !requiresHypothesis(n.Type) {
			continue
		}
		if len(n.Hypotheses) == 0 {
			missing = append(missing, n.ID)
		}
	}
	return missing
}

func requiresHypothesis(t types.TaskNodeType) bool {
	switch t {
	case types.NodeEvidence, types.NodeValidate, types.NodeReconcile, types.NodeFinalize:
		return true
	}
	return false
}

// firstSymbol returns the first TermSymbol's surface in the graph.
func firstSymbol(tg types.TermGraph) string {
	best := ""
	bestConf := float32(-1)
	for _, c := range tg.Canonical {
		if c.Kind != types.TermSymbol {
			continue
		}
		if c.Confidence > bestConf {
			bestConf = c.Confidence
			best = c.Surface
		}
	}
	return best
}

func orDefault(s, fallback string) string {
	if strings.TrimSpace(s) == "" {
		return fallback
	}
	return s
}

func firstOption(xs []string) string {
	if len(xs) == 0 {
		return ""
	}
	return xs[0]
}

func firstNonEmpty(xs ...string) string {
	for _, s := range xs {
		if strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
}
