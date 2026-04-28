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
	if len(rm.SubTopics) > 0 || resolved.IsRelationalLookup || rm.Intent == types.IntentTrace {
		return resolved, ""
	}
	if len(types.ExactResolutionTargets(rm)) != 1 {
		return resolved, ""
	}
	resolved.IsCrossComponent = false
	return resolved,
		"single exact-target lookup keeps one answer topic; nearby layers/anchors are context, not independent cross-component sub-topics"
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
