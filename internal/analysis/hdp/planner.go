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
//
// Multi-subject dispatch: topSymbols picks up to 3 TermSymbols
// ordered by Confidence desc; a sharp confidence gap (> 0.4) collapses
// back to the single top subject to keep low-quality symbols from
// diluting the hypothesis set. Each intent branch emits one hypothesis
// per subject — so "blob 和 log 的关系" produces two anchored
// hypotheses, not one merged statement. For explain/trace with
// SemanticPredicates.IsRelationalLookup AND exactly two subjects, an
// extra relation hypothesis is appended whose falsification is the
// new CritRelationAbsent kind.
func Plan(rm types.RequestModel) []types.Hypothesis {
	var out []types.Hypothesis
	nextID := func() string { return fmt.Sprintf("h%d", len(out)+1) }

	syms := topSymbols(rm.TermGraph, 3)

	// 1. Intent-driven seed hypothesis.
	switch rm.Intent {
	case types.IntentRootCause:
		if len(syms) == 0 {
			out = append(out, rootCauseHypothesis(nextID(), "the primary subject of the request"))
		} else {
			for _, sym := range syms {
				out = append(out, rootCauseHypothesis(nextID(), sym))
			}
		}
	case types.IntentConfigQuery:
		if len(syms) <= 1 {
			out = append(out, configQueryHypothesis(nextID(), orDefault(firstOf(syms), "")))
		} else {
			for _, sym := range syms {
				out = append(out, configQueryHypothesis(nextID(), sym))
			}
		}
	case types.IntentReturnValue, types.IntentEnumerate:
		if len(syms) == 0 {
			out = append(out, enumerateHypothesis(nextID(), "the subject"))
		} else {
			for _, sym := range syms {
				out = append(out, enumerateHypothesis(nextID(), sym))
			}
		}
	default:
		if len(syms) <= 1 {
			out = append(out, explainHypothesis(nextID(), orDefault(firstOf(syms), "repo symbols")))
		} else {
			out = append(out, explainSetHypothesis(nextID(), syms))
			// Pairwise relation hypothesis is only emitted when the
			// LLM's IsRelationalLookup predicate fires AND there are
			// exactly two subjects — N-ary relations would need an
			// order-insensitive pairing scheme we don't have.
			if rm.Predicates.IsRelationalLookup && len(syms) == 2 {
				out = append(out, relationHypothesis(nextID(), syms[0], syms[1]))
			}
		}
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

// topSymbols returns up to k TermSymbol surfaces from the graph in
// Confidence-desc order. A sharp confidence gap between the top match
// and the runner-up (> 0.4) collapses the return to just the top one —
// rarity-penalised kindEnWord matches can land at Confidence ≈ 0.3
// while a direct CamelCase hit lands at 1.0, and we do not want the
// weaker match to spawn a full hypothesis branch. Ties break on slice
// index so the output is deterministic.
func topSymbols(tg types.TermGraph, k int) []string {
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

// ── per-intent hypothesis constructors ──────────────────────────

func rootCauseHypothesis(id, sym string) types.Hypothesis {
	return types.Hypothesis{
		ID: id,
		Statement: fmt.Sprintf(
			"The observed symptom is caused by %s or a module it depends on.",
			sym),
		RequiredEvidence: []types.Criterion{
			{Kind: types.CritSymbolPresent, Expr: sym},
			{Kind: types.CritEvidenceCount, Expr: ">=2"},
		},
		FalsificationCondition: types.Criterion{Kind: types.CritNoCallSites, Expr: sym},
		Status:                 types.HypUnknown,
	}
}

func configQueryHypothesis(id, sym string) types.Hypothesis {
	stmt := "The effective config value is resolved by a single deterministic chain from default to override."
	if sym != "" {
		stmt = fmt.Sprintf(
			"The effective config for %s is resolved by a single deterministic chain from default to override.",
			sym)
	}
	return types.Hypothesis{
		ID:        id,
		Statement: stmt,
		RequiredEvidence: []types.Criterion{
			{Kind: types.CritEvidenceCount, Expr: ">=1"},
		},
		FalsificationCondition: types.Criterion{Kind: types.CritMultipleResolutionChains},
		Status:                 types.HypUnknown,
	}
}

func enumerateHypothesis(id, sym string) types.Hypothesis {
	return types.Hypothesis{
		ID: id,
		Statement: fmt.Sprintf(
			"The answer is a finite set of symbols anchored on %s.",
			sym),
		RequiredEvidence: []types.Criterion{
			{Kind: types.CritAnswerSetBounded, Expr: "<=50"},
		},
		FalsificationCondition: types.Criterion{Kind: types.CritAnswerSetUnbounded},
		Status:                 types.HypUnknown,
	}
}

func explainHypothesis(id, sym string) types.Hypothesis {
	return types.Hypothesis{
		ID: id,
		Statement: fmt.Sprintf(
			"The request can be answered using evidence anchored on %s.",
			sym),
		RequiredEvidence: []types.Criterion{
			{Kind: types.CritEvidenceCount, Expr: ">=1"},
		},
		FalsificationCondition: types.Criterion{Kind: types.CritNoRelevantEvidence},
		Status:                 types.HypUnknown,
	}
}

// explainSetHypothesis is the multi-subject counterpart of
// explainHypothesis. The statement lists every subject in the set so
// downstream priority / binder scoring picks up the full vocabulary,
// but the falsification condition stays scalar — an empty evidence
// list rejects regardless of which subject was meant.
func explainSetHypothesis(id string, syms []string) types.Hypothesis {
	return types.Hypothesis{
		ID: id,
		Statement: fmt.Sprintf(
			"The request can be answered using evidence anchored on the set {%s}.",
			strings.Join(syms, ", ")),
		RequiredEvidence: []types.Criterion{
			{Kind: types.CritEvidenceCount, Expr: ">=1"},
		},
		FalsificationCondition: types.Criterion{Kind: types.CritNoRelevantEvidence},
		Status:                 types.HypUnknown,
	}
}

// relationHypothesis asserts the two subjects are actually linked in
// the code base. RequiredEvidence demands both symbols plus ≥2
// evidence rows so a single mention does not confirm the relation;
// falsification is the new CritRelationAbsent kind.
func relationHypothesis(id, a, b string) types.Hypothesis {
	return types.Hypothesis{
		ID: id,
		Statement: fmt.Sprintf(
			"A direct relation exists between %s and %s (shared call site, type hierarchy, or resolution chain).",
			a, b),
		RequiredEvidence: []types.Criterion{
			{Kind: types.CritSymbolPresent, Expr: a},
			{Kind: types.CritSymbolPresent, Expr: b},
			{Kind: types.CritEvidenceCount, Expr: ">=2"},
		},
		FalsificationCondition: types.Criterion{
			Kind: types.CritRelationAbsent,
			Expr: a + "," + b,
		},
		Status: types.HypUnknown,
	}
}

// ── misc helpers ───────────────────────────────────────────────

func orDefault(s, fallback string) string {
	if strings.TrimSpace(s) == "" {
		return fallback
	}
	return s
}

func firstOf(xs []string) string {
	if len(xs) == 0 {
		return ""
	}
	return xs[0]
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
