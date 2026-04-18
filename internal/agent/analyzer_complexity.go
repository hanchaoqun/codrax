package agent

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/types"
)

// analyzer_complexity.go holds deterministic sanity checks that run
// on the LLM-emitted Complexity after emit_analysis. The LLM's
// choice is treated as a prior; these rules only override when
// structural signals (entity count, keyword count, question-shape
// patterns in the raw request) strongly contradict that prior.
//
// The intent is NOT to second-guess every verdict — LLMs are better
// than regex at reading intent. The intent is to stop catastrophic
// misclassifications from drifting downstream: a "compare A and B
// across 3 systems" question judged as simple would deprive the
// adaptive citation gate of its higher floor; a single-file literal
// lookup judged as complex would hold the answer to an unreachable
// citation bar.
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
//   - rawRequest: the current request text (conversation prefix stripped).
//   - entities: emit_analysis.entities (already de-generic-filtered).
//   - keywords: emit_analysis.keywords.
//   - subTopics: len(emit_analysis.sub_topics).
//
// Returns (resolved, reason). When resolved == declared the rule did
// not fire and reason is empty.
func reconcileComplexity(declared types.Complexity, rawRequest string, entities, keywords []string, subTopics int, questionKind string) (types.Complexity, string) {
	entCount := len(entities)
	kwCount := len(keywords)
	lower := strings.ToLower(strings.TrimSpace(rawRequest))

	// Rule 1: sub-topic floor. An existing analyzer.go rule already
	// lifts simple → moderate when sub_topics>1; this rule takes it
	// further — complexSubTopicFloor+ sub-topics structurally imply
	// cross-component reasoning and LOCK the verdict at complex so
	// later lookup-shape downgrade rules cannot pull it back. Fires
	// unconditionally when the count threshold is met so a
	// "declared=complex" verdict with a superficially simple-sounding
	// rephrasing ("what is A, B, C, and D?") does not get downgraded
	// by rule 3.
	if subTopics >= complexSubTopicFloor {
		if declared == types.ComplexityComplex {
			return declared, ""
		}
		return types.ComplexityComplex,
			"subTopics>=complexSubTopicFloor structurally requires cross-component breadth"
	}

	// Rule 2: cross-component language → complex. Independent of the
	// LLM's self-assessment. Phrases are pooled across languages,
	// cross-referenced so a single language does not dominate.
	if declared != types.ComplexityComplex && containsCrossComponentCue(lower) {
		return types.ComplexityComplex,
			"cross-component cue detected in request text (compare / between / 对比 / 之间 / across)"
	}

	// Rule 3: explicit simple downgrade when declared complex but the
	// question has only downgradeMaxEntities entities and
	// downgradeMaxKeywords keywords and fits a shape-lookup cue. An
	// LLM that marked "X 是什么" as complex is probably over-estimating
	// scope.
	if declared == types.ComplexityComplex && entCount <= downgradeMaxEntities && kwCount <= downgradeMaxKeywords && containsSimpleLookupCue(lower) {
		return types.ComplexitySimple,
			"single-entity lookup shape (what is / where / 是什么 / 在哪) contradicts declared complex"
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
	// answer requires 6+ files across 3+ packages. The 2026-04-18
	// "explorer是如何调用subagent的？" failure was caused by this exact
	// gap: 2 entities + mechanism → classified moderate → cap=20,
	// threshold=2 → LLM stopped after 3 files, answer missed the
	// orchestrator dispatch chain.
	if declared != types.ComplexityComplex && entCount >= 2 &&
		(questionKind == "mechanism" || questionKind == "call_chain") {
		return types.ComplexityComplex,
			"mechanism/call_chain question with 2+ entities implies cross-component dispatch chain"
	}

	// Rule 7: enumeration + relational verb upgrade. A question
	// shaped "how many X can do Y" / "有几个 X 可以 Y" reads as a
	// count on the surface but the relational verb forces the
	// answer to trace a relationship across every candidate. The
	// generic-entity blocklist may have stripped X/Y when they hit
	// domain-neutral tokens (agent / handler / module), which is
	// exactly the case where Rule 4's entity floor cannot fire. Left
	// at simple, investigation budget caps at ~2 rounds and the
	// first file answers for the whole family ("SubExplorer" when
	// the real answer is three agents). Raising to moderate gives
	// the explorer 4-6 rounds and keeps citation gates ON so each
	// candidate must be grounded in file:line evidence.
	//
	// Intent is deliberately left untouched — whether the LLM picks
	// enumerate (returns the list) or return_value (returns the
	// scalar) is a downstream shape concern, and both populations
	// benefit equally from the budget lift.
	if declared == types.ComplexitySimple && hasLeadingEnumerationCue(lower) && containsRelationalVerbCue(lower) {
		return types.ComplexityModerate,
			"enumeration cue + relational verb — answer requires tracing a relationship across candidates"
	}

	return declared, ""
}

// containsCrossComponentCue reports whether the request text carries
// a language-neutral cue that the answer must span multiple
// components. The corpus is kept small on purpose: a bigger list
// becomes a stopword-style curation burden and is exactly the
// anti-pattern memory/feedback_overfitting_audit_stopwords.md warns
// against. Add a cue here only when a real run demonstrates a
// systematic miss.
var crossComponentCues = []string{
	" compare ", " versus ", " vs ", " diff ", " between ", " across ",
	"对比", "对照", "之间", "跨越", "跨",
	"differences", "differ ", "trade-offs", "trade offs",
}

func containsCrossComponentCue(lower string) bool {
	padded := " " + lower + " "
	for _, cue := range crossComponentCues {
		if strings.Contains(padded, cue) {
			return true
		}
	}
	return false
}

// containsSimpleLookupCue reports whether the request fits a
// single-entity literal-lookup shape — "what is X", "where is X",
// "X 是什么", "X 在哪". Same curation discipline as above.
var simpleLookupCues = []string{
	"what is ", "what are ", "where is ", "where are ", "does ",
	"是什么", "在哪", "什么是", "哪里",
}

func containsSimpleLookupCue(lower string) bool {
	for _, cue := range simpleLookupCues {
		if strings.Contains(lower, cue) {
			return true
		}
	}
	return false
}

// logComplexityReconcile emits a single warning line when the rule
// overrode the LLM's pick. Keeps the event visible in eval harness
// traces without flooding info-level logs on no-op cases.
func logComplexityReconcile(before, after types.Complexity, reason string) {
	if before == after || reason == "" {
		return
	}
	logging.Warning("[analyzer] complexity reconciled: %s → %s (%s)", before, after, reason)
}
