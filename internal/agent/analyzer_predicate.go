package agent

import (
	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/types"
)

// analyzer_predicate.go used to host a multi-language verb-cue table
// (predicateVerbMap) that mapped surface verb forms in ZH+EN to a
// PredicateAxis. After schema-v4 the LLM emits the axis directly via
// emit_analysis.predicate_axis (typed enum, every language), and the
// table is gone; the deletion satisfies the multi-language
// generalisation red line in
// memory/feedback_generalization_over_project_success.md.
//
// What's left: a thin reconcile shim that survives only as a
// defense-in-depth identity preserve. If the LLM emits an axis,
// downstream gets it; if not, the field stays AxisUnknown and the
// axis-aware ranker boost simply does not fire (the affinity matrix
// returns neutral weights for unknown combos).

// reconcilePredicateAxis preserves the LLM-emitted axis verbatim.
// Returns (declared, "") so the analyzer.go observability hook does
// not fire an "override" event on a clean pass-through.
//
// Kept as a function (rather than inlining the no-op) so future
// reconciliation rules can hook here without changing the analyzer
// pipeline shape. For example, if a downstream signal needs to
// suppress AxisCondition when the question is structurally a
// definitional lookup, the rule lands in this function.
func reconcilePredicateAxis(declared types.PredicateAxis) (types.PredicateAxis, string) {
	return declared, ""
}

// reconcileSemanticPredicates applies deterministic guardrails to the
// LLM-emitted semantic predicate set when multiple structured signals
// make one predicate internally contradictory.
//
// A single request-mentioned exact target (config key / path / symbol)
// can require nearby context, precedence layers, or override
// explanation without becoming a cross-component / multi-subtopic
// question. Leaving is_cross_component=true in that population
// cascades into complexity inflation and subtopic_coherence retries.
//
// Likewise, a single ordered trace can cross files / packages while
// still remaining one answer topic. That population wants richer trace
// output (steps + diagrams), not a forced multi-topic architecture
// reconcile loop.
//
// The rule stays language-neutral and repository-agnostic:
//   - exactly one exact-resolution target survives the request-grounded lane
//   - no emitted sub-topics
//   - not a relational lookup
//   - not an explicit trace intent
//
// That combination still centers the answer on one named target, so
// nearby layers/anchors are context, not independent components.
func reconcileSemanticPredicates(rm types.RequestModel) (types.SemanticPredicates, string) {
	resolved := rm.Predicates
	if !resolved.IsCrossComponent {
		return resolved, ""
	}
	// Existing rule: single exact-target lookup keeps one answer
	// topic regardless of nearby layers.
	if len(types.ExactResolutionTargets(rm)) == 1 &&
		len(rm.SubTopics) == 0 &&
		!resolved.IsRelationalLookup &&
		rm.Intent != types.IntentTrace {
		resolved.IsCrossComponent = false
		return resolved,
			"single exact-target lookup keeps one answer topic; nearby layers/anchors are context, not independent cross-component sub-topics"
	}
	// Single ordered trace rule: a source-to-sink walkthrough may span
	// multiple files/packages, but it is still one topic when the
	// request does not split into independent sub-topics or set-style
	// relationships. Keep the richer trace/diagram lane while avoiding
	// the multi-topic coherence gate.
	if types.IsSingleTopicStructuralTrace(rm) {
		resolved.IsCrossComponent = false
		return resolved,
			"single ordered structural trace keeps one answer topic even when the chain crosses files/packages; preserve the trace lane without promoting to multi-topic cross-component reasoning"
	}
	// New rule (R1.2 auto-fix): when the LLM emits IsCrossComponent=true
	// but provides ZERO sub-topics, that's an internal contradiction
	// — IsCrossComponent claims the question crosses multiple
	// components, yet no sub-topics were enumerated. Pre-2026-04-30
	// the gate (subtopic_coherence R1.2 in internal/analysis/gate)
	// rejected this and pushed the LLM into a retry loop with a
	// hint, but the LLM frequently couldn't recover within budget
	// and the pipeline aborted. The structural fact (0 sub-topics)
	// is unambiguous; the meta-claim (IsCrossComponent) is the
	// soft signal. Demote IsCrossComponent so the pipeline treats
	// the question as single-topic and proceeds. The gate's R1.1
	// (domain divergence — separate signal from TermGraph) still
	// catches genuine multi-domain questions where the LLM emitted
	// 0 sub-topics by mistake — those need true sub-topic synthesis,
	// not just the predicate flip.
	if len(rm.SubTopics) <= 1 && !resolved.IsRelationalLookup && rm.Intent != types.IntentTrace {
		resolved.IsCrossComponent = false
		return resolved,
			"R1.2 auto-fix: predicate IsCrossComponent=true contradicts SubTopics empty/single — demoting to false rather than failing the coherence gate; downstream behaviour is single-topic, with R1.1 (domain divergence) still active for true multi-domain catches"
	}
	return resolved, ""
}

// logPredicateAxisReconcile mirrors logIntentReconcile: one info
// line when the rule changed the axis, silent otherwise. Today the
// rule never changes the axis (LLM is the sole producer); the helper
// stays so a future reconcile rule has a logging hook ready.
func logPredicateAxisReconcile(before, after types.PredicateAxis, reason string) {
	if before == after || reason == "" {
		return
	}
	logging.Info("[analyzer] predicate_axis reconciled: %q -> %q (%s)", before, after, reason)
}

func logPredicateReconcile(before, after types.SemanticPredicates, reason string) {
	if before == after || reason == "" {
		return
	}
	logging.Warning(
		"[analyzer] predicates reconciled: is_cross_component=%t -> %t (%s)",
		before.IsCrossComponent, after.IsCrossComponent, reason,
	)
}
