// Package hdp implements the Hypothesis-Driven Planning layer of the
// Analyzer v3 refactor. It produces a set of falsifiable hypotheses
// from a RequestModel and binds each hypothesis to the TaskGraph
// nodes that must verify or reject it. The explorer/validator stages
// later write HypothesisStatus back onto the IR so the DAG scheduler
// can drive transitions on evidence rather than on opaque booleans.
//
// This package is intentionally narrow:
//
//   - Plan(rm, tg) returns a candidate HypothesisSet that the
//     analyzer agent (b5) may replace or refine with LLM-generated
//     statements. When the LLM provides nothing, the rule-based
//     output is still a valid starting point.
//
//   - Bind(tg, hs) attaches hypothesis IDs to every NodeEvidence and
//     NodeValidate node so Validate(tg) can later enforce the
//     "every non-probe node binds ≥1 hypothesis" invariant.
//
//   - Validate(tg) is the invariant check the analyzer quality gate
//     (b4 gate package, next) calls before accepting an IR.
package hdp

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// Plan generates a small set of candidate hypotheses derived from
// the Intent + TermGraph + Ambiguities of the request. It never
// returns an empty set — even a fully generic request gets one
// baseline hypothesis so downstream binding has something to attach.
//
// The output order is deterministic: priority desc, then ID asc, so
// tests and diff-based evaluations stay stable.
func Plan(rm types.RequestModel) []types.Hypothesis {
	var out []types.Hypothesis
	nextID := func() string { return fmt.Sprintf("h%d", len(out)+1) }

	topSymbol := firstSymbol(rm.TermGraph)

	// 1. Intent-driven seed hypothesis.
	switch rm.Intent {
	case types.IntentRootCause:
		out = append(out, types.Hypothesis{
			ID:        nextID(),
			Statement: fmt.Sprintf("The observed symptom is caused by %s or a module it depends on.", orDefault(topSymbol, "the primary subject of the request")),
			RequiredEvidence: []types.Criterion{
				{Kind: "symbol_present", Expr: orDefault(topSymbol, "")},
			},
			FalsificationCondition: types.Criterion{Kind: "no_call_sites", Expr: orDefault(topSymbol, "")},
			Priority:               90,
			Status:                 types.HypUnknown,
		})
	case types.IntentConfigQuery:
		out = append(out, types.Hypothesis{
			ID:                     nextID(),
			Statement:              "The effective config value is resolved by a single deterministic chain from default to override.",
			FalsificationCondition: types.Criterion{Kind: "multiple_resolution_chains"},
			Priority:               80,
			Status:                 types.HypUnknown,
		})
	case types.IntentReturnValue, types.IntentEnumerate:
		out = append(out, types.Hypothesis{
			ID:                     nextID(),
			Statement:              fmt.Sprintf("The answer is a finite set of symbols anchored on %s.", orDefault(topSymbol, "the subject")),
			FalsificationCondition: types.Criterion{Kind: "answer_set_unbounded"},
			Priority:               80,
			Status:                 types.HypUnknown,
		})
	default:
		out = append(out, types.Hypothesis{
			ID:                     nextID(),
			Statement:              fmt.Sprintf("The request can be answered using evidence anchored on %s.", orDefault(topSymbol, "repo symbols")),
			FalsificationCondition: types.Criterion{Kind: "no_relevant_evidence"},
			Priority:               60,
			Status:                 types.HypUnknown,
		})
	}

	// 2. Ambiguities — each becomes a hypothesis to resolve.
	for _, amb := range rm.Ambiguities {
		if strings.TrimSpace(amb.Clause) == "" {
			continue
		}
		statement := fmt.Sprintf("The user's intent for %q is: %s.",
			amb.Clause, firstNonEmpty(amb.Resolution, firstOption(amb.Options), "to be determined"))
		out = append(out, types.Hypothesis{
			ID:                     nextID(),
			Statement:              statement,
			FalsificationCondition: types.Criterion{Kind: "user_clause_unresolved", Expr: amb.Clause},
			Priority:               70,
			Status:                 types.HypUnknown,
		})
	}

	// 3. Risk-driven hypotheses — one per high-risk dimension.
	if rm.RiskMatrix.Security.Level >= 3 {
		out = append(out, types.Hypothesis{
			ID:                     nextID(),
			Statement:              "The change does not introduce an un-sanitized data path from an untrusted boundary.",
			FalsificationCondition: types.Criterion{Kind: "untrusted_reaches_sink"},
			Priority:               85,
			Status:                 types.HypUnknown,
		})
	}
	if rm.RiskMatrix.DataIntegrity.Level >= 3 {
		out = append(out, types.Hypothesis{
			ID:                     nextID(),
			Statement:              "The change preserves all existing data invariants and is safe under concurrent writes.",
			FalsificationCondition: types.Criterion{Kind: "invariant_broken"},
			Priority:               85,
			Status:                 types.HypUnknown,
		})
	}

	return out
}

// Bind attaches hypothesis IDs to every NodeEvidence, NodeValidate,
// NodeReconcile, and NodeFinalize node in the task graph. Probe and
// writing-stage nodes (design/implement/review/verify) are left
// untouched — probes are baseline exploration and writing stages
// inherit their targets from the parent validation node.
//
// Binding strategy is round-robin: the i-th binding-eligible node
// gets hs[i % len(hs)], so a single hypothesis graph still spreads
// pressure across evidence nodes, and multiple hypotheses get
// distributed evenly.
//
// Bind is additive — nodes that already have hypotheses keep them.
func Bind(tg *types.TaskGraph, hs []types.Hypothesis) {
	if tg == nil || len(hs) == 0 {
		return
	}
	i := 0
	for idx := range tg.Nodes {
		if !requiresHypothesis(tg.Nodes[idx].Type) {
			continue
		}
		if len(tg.Nodes[idx].Hypotheses) > 0 {
			continue
		}
		tg.Nodes[idx].Hypotheses = []string{hs[i%len(hs)].ID}
		i++
	}
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

// firstSymbol returns the first TermSymbol's surface in the graph,
// preferring repo-grounded terms over pattern-derived ones by
// confidence.
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
