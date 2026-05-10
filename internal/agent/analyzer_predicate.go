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
	// B.4 audit followup (2026-05-02): reverse-risk guard. Rule 1
	// must not demote IsCrossComponent when the LLM has emitted an
	// explicit multi-axis signal that survives "single exact target".
	// Examples that pass exact-target=1 but still need cross-component
	// reasoning:
	//   - "what kinds of agents subscribe to event X?" (IsCategoryEnumeration)
	//   - "how many tools call helper X?" (IsCountQuestion)
	//   - "when was function X last touched?" (IsHistoryLookup)
	//   - "why does X fail / does the same risk remain?" (IsDiagnosticQuestion)
	// Each carries ONE exact target but the answer aggregates across
	// the repo, which is what IsCrossComponent encodes. Without this
	// guard, rule 1 silently flips IsCrossComponent=false and the
	// downstream complexity / shape selection drift to single-symbol
	// lookup answers.
	if len(types.ExactResolutionTargets(rm)) == 1 &&
		len(rm.SubTopics) == 0 &&
		!resolved.IsRelationalLookup &&
		!signalsExplicitMultiAxis(resolved) &&
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
	// IsCrossComponent vs SubTopics inconsistency is intentionally
	// LEFT to the coherence gate (gate.checkSubtopicCoherence R1.2)
	// rather than auto-demoted here. Reasoning:
	//
	//  - IsCrossComponent is a SCHEMA-REQUIRED field (the LLM must
	//    affirm true/false on every emit_analysis call); SubTopics
	//    is SCHEMA-OPTIONAL (omitted by default). Required > optional
	//    on the soft/hard signal hierarchy: the LLM's affirmative
	//    claim that the question is cross-component is the harder
	//    signal; the absence of sub_topics is the softer one.
	//
	//  - Pre-2026-05-02 this site auto-demoted IsCrossComponent to
	//    match the missing sub_topics, but that's the wrong direction:
	//    it discards the LLM's hard signal to defer to the soft one.
	//    Worse, it short-circuited gate R1.2's retry loop — the gate
	//    can no longer surface "you said cross-component but didn't
	//    enumerate sub-topics, please add sub_topics" because the
	//    predicate has already been flipped before the gate runs.
	//
	//  - The user-intent-over-system-gates red line applies here:
	//    when the LLM judges a question as cross-component (e.g.
	//    "compare module A's behaviour with module B's"), the system
	//    must trust that judgment and let the retry path correct the
	//    softer signal, not silently overwrite the harder signal.
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

// signalsExplicitMultiAxis (B.4 audit followup, 2026-05-02) returns
// true when the predicate set carries any signal that the question is
// multi-axis even if a single exact target was named. These signals
// must be preserved as cross-component-positive even when rule 1 of
// reconcileSemanticPredicates would otherwise demote.
//
// Why each signal counts as multi-axis:
//   - IsCategoryEnumeration: "what KINDS of X" iterates across all
//     subtypes / categorisations of the named target — the answer
//     spans multiple component implementations of X.
//   - IsCountQuestion: by definition aggregates across multiple source
//     units to produce a scalar number; the cross-aggregation IS the
//     cross-component dimension.
//   - IsHistoryLookup: VCS-metadata answer crossing commits / branches
//     / authors; that history-axis is orthogonal to the named target.
//   - IsDiagnosticQuestion: root-cause / regression diagnosis can need
//     falsification and current-risk comparison around one named target.
//
// IsRelationalLookup is intentionally NOT folded into this helper: it
// is checked separately in rule 1's condition list (and the gate
// rules in coherence.go). Rolling it in here would mask which signal
// is doing the demotion-block work in audit logs.
func signalsExplicitMultiAxis(p types.SemanticPredicates) bool {
	return p.IsCategoryEnumeration ||
		p.IsCountQuestion ||
		p.IsHistoryLookup ||
		p.IsDiagnosticQuestion
}
