package agent

import (
	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/types"
)

// analyzer_complexity.go holds deterministic sanity checks that run
// on the LLM-emitted Complexity after emit_analysis. The LLM's
// choice is treated as a prior; these rules only override when
// either structural signals (entity count, keyword count, sub-topic
// count) or the LLM-emitted SemanticPredicates strongly contradict
// that prior.
//
// Schema-v4 rewrite: every prose-cue table that this file used to
// host (crossComponentCues, simpleLookupCues, hasLeadingEnumerationCue,
// containsRelationalVerbCue) is gone. The corresponding signals now
// come from RequestModel.Predicates which the LLM produces in any
// language, satisfying the multi-language generalisation red line in
// memory/feedback_generalization_over_project_success.md.
//
// Rules fire only on strong signals. A borderline misclassification
// (moderate vs. complex on a 4-entity question) is left to the LLM.

// Rule thresholds. Named so one grep surfaces every magic number and
// tuning a rule only requires editing its constant, not the
// reconciler body. Each threshold has a one-line comment stating why
// it is set where it is.
const (
	// complexSubTopicFloor is the sub-topic count at which complexity
	// is locked at "complex" regardless of LLM verdict. At 3+
	// sub-topics the investigation is structurally multi-component
	// (each sub-topic typically maps to a separate pipeline, module,
	// or decision axis).
	complexSubTopicFloor = 3

	// downgradeMaxEntities / downgradeMaxKeywords bound the
	// "declared complex but looks like a lookup" guard. Raising
	// either past these ceilings means the question already has
	// enough surface area to justify complex; only thin prompts
	// should fall through to simple.
	downgradeMaxEntities = 1
	downgradeMaxKeywords = 6

	// upgradeMinEntities / upgradeMinKeywords are the twin floors
	// that lift declared non-complex to complex. Chosen so that the
	// typical "what does X do?" (1 entity, 4-8 keywords) does NOT
	// trip the upgrade — only genuinely broad prompts do.
	upgradeMinEntities = 5
	upgradeMinKeywords = 10

	// sparsePromptMaxKeywords is the ceiling for the "zero entities
	// + very few keywords" sanity fallback. If the LLM declared
	// complex on a prompt this sparse, something went wrong upstream
	// and the verdict cannot be trusted.
	sparsePromptMaxKeywords = 4
)

// reconcileComplexity returns the complexity that should travel
// downstream. Inputs:
//   - declared: the Complexity the LLM picked via emit_analysis.
//   - entities: emit_analysis.entities (already de-generic-filtered).
//   - keywords: emit_analysis.keywords.
//   - subTopics: len(emit_analysis.sub_topics).
//   - questionKind: emit_analysis.question_kind (legacy string form).
//   - preds: LLM-emitted SemanticPredicates.
//
// Returns (resolved, reason). When resolved == declared the rule did
// not fire and reason is empty.
// complexityEscalationConfidenceCeiling is the §1.6 typed-escape
// floor for NOISY escalation rules: when the model declared simple
// with at least this ComplexityConfidence, heuristic count-based
// upgrades (Rule 6) yield to the declaration. Precise structural
// rules (sub-topic floor, is_cross_component) are unaffected — they
// read typed signals, not counts.
const complexityEscalationConfidenceCeiling = 0.9

func reconcileComplexity(
	declared types.Complexity,
	declaredConfidence float64,
	entities, keywords []string,
	subTopics int,
	questionKind string,
	preds types.SemanticPredicates,
) (types.Complexity, string) {
	entCount := len(entities)
	kwCount := len(keywords)

	// Rule 1: sub-topic floor. An existing analyzer.go rule already
	// lifts simple → moderate when sub_topics>1; this rule takes it
	// further — complexSubTopicFloor+ sub-topics structurally imply
	// cross-component reasoning and LOCK the verdict at complex so
	// later lookup-shape downgrade rules cannot pull it back.
	if subTopics >= complexSubTopicFloor {
		if declared == types.ComplexityComplex {
			return declared, ""
		}
		return types.ComplexityComplex,
			"subTopics>=complexSubTopicFloor structurally requires cross-component breadth"
	}

	// Rule 2: cross-component → complex. The LLM judges
	// is_cross_component for any language; no prose tables involved.
	if declared != types.ComplexityComplex && preds.IsCrossComponent {
		return types.ComplexityComplex,
			"predicates.is_cross_component=true — answer must span multiple components"
	}

	// Rule 3: explicit simple downgrade when declared complex but the
	// question is structurally a single-scalar lookup (predicates say
	// scalar, very few entities/keywords). The simpleLookup prose cue
	// table is gone; the LLM's is_scalar_answer predicate replaces it.
	if declared == types.ComplexityComplex && entCount <= downgradeMaxEntities && kwCount <= downgradeMaxKeywords && preds.IsScalarAnswer {
		return types.ComplexitySimple,
			"single-entity scalar lookup (predicates.is_scalar_answer=true, ≤1 entity, ≤6 keywords) contradicts declared complex"
	}

	// Rule 4: multi-entity upgrade. upgradeMinEntities or more
	// entities combined with upgradeMinKeywords or more keywords
	// strongly suggest complex investigation breadth.
	if declared != types.ComplexityComplex && entCount >= upgradeMinEntities && kwCount >= upgradeMinKeywords {
		return types.ComplexityComplex,
			"entity and keyword counts reach the upgrade floor — cross-component breadth"
	}

	// Rule 5: zero-entity + tiny prompt → simple floor. A question
	// like "lang" with no entities and almost no keywords cannot be
	// complex regardless of the LLM's pick.
	if declared == types.ComplexityComplex && entCount == 0 && kwCount <= sparsePromptMaxKeywords {
		return types.ComplexitySimple,
			"zero entities and very few keywords — declared complex cannot be justified"
	}

	// Rule 6: mechanism/call_chain + multi-entity → complex. A
	// "how does X call/invoke Y" question with 2+ entities
	// structurally implies a cross-component dispatch chain (X is in
	// package A, Y is in package B, the dispatch path runs through
	// package C). The LLM frequently picks moderate for these because
	// the question READS like a simple "how does X work" — but the
	// answer requires 6+ files across 3+ packages.
	// §1.6 typed escape (batch-6 E2): the entity count is a NOISY
	// signal for this rule — a rename/typo request structurally
	// carries both the wrong and the right token as entities, so the
	// 2+ floor is always met and a one-line single-file request was
	// escalated to complex (inflating read budgets and the write-lane
	// planner soft cap) against the model's simple declaration at
	// 0.98 confidence. A high-confidence typed simple declaration
	// wins over the count heuristic; precise rules above (sub-topic
	// floor, is_cross_component) still escalate regardless.
	if declared != types.ComplexityComplex && entCount >= 2 &&
		(questionKind == "mechanism" || questionKind == "call_chain") {
		if declared == types.ComplexitySimple && declaredConfidence >= complexityEscalationConfidenceCeiling {
			return declared, ""
		}
		return types.ComplexityComplex,
			"mechanism/call_chain question with 2+ entities implies cross-component dispatch chain"
	}

	// Rule 7: enumeration + relational lookup upgrade. A question
	// shaped "how many X can do Y" reads as a count on the surface
	// but the relational lookup forces the answer to trace a
	// relationship across every candidate. Generic-entity blocklist
	// often strips X/Y when they hit domain-neutral tokens (agent /
	// handler / module), which is exactly the case where Rule 4's
	// entity floor cannot fire. Left at simple, investigation budget
	// caps too low. Raising to moderate gives the explorer enough
	// rounds to ground each candidate in file:line evidence.
	//
	// Both the count/category cue AND the relational lookup must
	// fire — without the relational half a raw count question stays
	// simple (it's just a tool query).
	if declared == types.ComplexitySimple &&
		(preds.IsCountQuestion || preds.IsCategoryEnumeration) &&
		preds.IsRelationalLookup {
		return types.ComplexityModerate,
			"enumeration cue + relational lookup — answer requires tracing a relationship across candidates"
	}

	return declared, ""
}

// logComplexityReconcile emits a single info line when the rule
// overrode the LLM's pick. Reconciliation is the analyzer's
// designed structural-override path, not an exception, so it logs
// at INFO so real WARN entries stay diagnostic. Suppressed when no
// override fired (zero noise on no-op cases).
func logComplexityReconcile(before, after types.Complexity, reason string) {
	if before == after || reason == "" {
		return
	}
	logging.Info("[analyzer] complexity reconciled: %s → %s (%s)", before, after, reason)
}
