package gate

import (
	"fmt"
	"sort"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// Cross-signal coherence gates run inside Run's read-mode-only
// branch. They are upstream root-cause guards for two recurring
// mis-classification patterns the downstream explorer / extractor /
// finalizer layers historically cleaned up after the fact:
//
//  1. SubTopics under-count — the LLM emits 1 sub-topic for a
//     question that structurally spans multiple repomap domains or
//     that the LLM itself flagged as cross-component. Triggers cascade
//     ExplanationAnchorBackbone failures, retry hints, and downstream
//     completion downgrades, all of which only treat symptoms.
//
//  2. Shape-vs-subject mismatch — the LLM emits a scalar AnswerSubject
//     under an Explanation shape, or an IsScalarAnswer predicate with
//     2+ sub-topics. These contradictions silently slip through today
//     because each downstream stage only looks at one side.
//
// Both gates compare LLM-emitted IR fields against each other and
// against the repomap-verified TermGraph domains. No keyword tables,
// no language-specific cue lists — every input is either a typed
// enum (AnswerSubject.Kind, EffectiveRequiredAnswerShape), an LLM
// self-judged bool (SemanticPredicates), or a structural signal
// (TermGraph.Canonical, PrimaryEntities, SubTopics). When a gate
// fires the GateReport.Retryable bit re-enters the analyzer's emit
// loop with the IR-field-level detail rendered into the next
// dispatch's retry hint.

// Tunables for the coherence checks. These are deliberately *not*
// yaml-exposed: they are correctness boundaries, not budget knobs,
// and lifting them weakens the gate without addressing the LLM's
// underlying mis-classification.
const (
	// coherenceTermSymbolMinConfidence excludes low-confidence
	// TermSymbol entries from the Domain divergence check so a
	// fuzzy-match dictionary surface (kindEnWord with rarity-
	// scaled Confidence) does not single-handedly escalate a
	// well-formed single-topic IR. 0.7 mirrors the threshold the
	// normalizer uses for resolver-verified TermSymbols.
	coherenceTermSymbolMinConfidence = float32(0.7)

	// coherenceSubjectConfidenceFloor is the AnswerSubject.Confidence
	// floor below which the explanation-vs-scalar-subject rule (R2.2)
	// is silenced. Subject inference at confidence < 0.6 carries too
	// much noise to justify a retry.
	coherenceSubjectConfidenceFloor = float32(0.6)

	// coherenceMinPrimaryEntitiesForOrphan: SubTopic-vs-PrimaryEntities
	// orphan rule (R1.3) only fires when the LLM emitted ≥2 primary
	// entities. A single-entity question can legitimately have a
	// sub-topic whose entities[] expands into different identifiers.
	coherenceMinPrimaryEntitiesForOrphan = 2
)

// checkSubtopicCoherence enforces three structural invariants that
// catch SubTopics under-count without resorting to keyword tables.
//
// R1.1 Domain divergence: TermGraph carries multiple repomap-verified
//      domains (FileInfo.Package values seen by normalizer's resolver
//      pass) but the LLM emitted ≤1 sub-topic — strong evidence the
//      question covers independent code regions.
//
// R1.2 Predicate self-contradiction: LLM emitted IsCrossComponent=true
//      but ≤1 sub-topic. The LLM is contradicting its own structured
//      self-assessment.
//
// R1.3 Sub-topic entity orphan: SubTopics declare entities[] that share
//      no element with PrimaryEntities. Either the SubTopics were
//      improvised after-the-fact or PrimaryEntities is incomplete.
//      Either way the ChainGraph cannot trace the answer.
//
// All three routes return the SAME check name so the retry hint
// renderer can ladder a single "subtopic_coherence" follow-up across
// any of the underlying triggers — the downstream LLM does not need
// to know which specific rule fired, only that its sub-topic count
// is structurally wrong.
func checkSubtopicCoherence(ir *types.AnalysisIR) types.GateCheck {
	rm := ir.RequestModel
	nSub := len(rm.SubTopics)

	// R1.1 — Domain divergence.
	domains := extractDistinctTermDomains(rm.TermGraph)
	if len(domains) >= 2 && nSub <= 1 {
		return types.GateCheck{
			Name:   "subtopic_coherence",
			Passed: false,
			Detail: fmt.Sprintf(
				"R1.1 domain_divergence: TermGraph spans %d distinct repomap-verified domains %s but only %d sub-topic emitted",
				len(domains), formatDomainList(domains), nSub),
		}
	}

	// R1.2 — Predicate self-contradiction.
	if rm.Predicates.IsCrossComponent && nSub <= 1 {
		return types.GateCheck{
			Name:   "subtopic_coherence",
			Passed: false,
			Detail: fmt.Sprintf(
				"R1.2 predicate_contradiction: Predicates.IsCrossComponent=true but only %d sub-topic emitted",
				nSub),
		}
	}

	// R1.3 — Sub-topic entity orphan.
	if nSub >= 1 && len(rm.AnalyzerHints.PrimaryEntities) >= coherenceMinPrimaryEntitiesForOrphan {
		subEnts := flattenSubTopicEntities(rm.SubTopics)
		if len(subEnts) > 0 {
			if !anyOverlap(subEnts, rm.AnalyzerHints.PrimaryEntities) {
				return types.GateCheck{
					Name:   "subtopic_coherence",
					Passed: false,
					Detail: fmt.Sprintf(
						"R1.3 entity_orphan: sub-topic entities %s share no element with primary entities %s",
						formatStringList(subEnts), formatStringList(rm.AnalyzerHints.PrimaryEntities)),
				}
			}
		}
	}

	return types.GateCheck{
		Name:   "subtopic_coherence",
		Passed: true,
		Score:  1.0,
		Detail: fmt.Sprintf("sub-topics=%d domains=%d primary_entities=%d",
			nSub, len(domains), len(rm.AnalyzerHints.PrimaryEntities)),
	}
}

// checkShapeSubjectCoherence enforces two cross-signal invariants on
// the AnswerShape ↔ AnswerSubject ↔ Predicates triangle. Both routes
// catch genuine LLM contradictions rather than judgement calls.
//
// R2.1 Scalar vs multi-topic: Predicates.IsScalarAnswer is true but
//      the LLM emitted ≥2 sub-topics. A scalar answer cannot be the
//      union of multiple independently-answerable sub-topics.
//
// R2.2 Explanation vs scalar subject: the resolved required answer
//      shape is Explanation but AnswerSubject.Kind is one of the
//      single-value subject kinds (Numeric, StringLiteral,
//      ReturnValue) at confidence ≥ floor. Explanation prose cannot
//      "be" a single literal value.
func checkShapeSubjectCoherence(ir *types.AnalysisIR) types.GateCheck {
	rm := ir.RequestModel
	nSub := len(rm.SubTopics)

	// R2.1 — IsScalarAnswer with ≥2 sub-topics.
	if rm.Predicates.IsScalarAnswer && nSub >= 2 {
		return types.GateCheck{
			Name:   "shape_subject_coherence",
			Passed: false,
			Detail: fmt.Sprintf(
				"R2.1 scalar_multi_topic: Predicates.IsScalarAnswer=true but %d sub-topics emitted",
				nSub),
		}
	}

	// R2.2 — Explanation shape with high-confidence scalar subject.
	resolvedShape := types.EffectiveRequiredAnswerShape(ir, nil)
	declaredShape := ir.AnswerContract.RequiredAnswerShape
	isExplanation := resolvedShape == types.ShapeExplanation || declaredShape == types.ShapeExplanation
	if isExplanation && isScalarSubjectKind(rm.AnswerSubject.Kind) &&
		float32(rm.AnswerSubject.Confidence) >= coherenceSubjectConfidenceFloor {
		return types.GateCheck{
			Name:   "shape_subject_coherence",
			Passed: false,
			Detail: fmt.Sprintf(
				"R2.2 explanation_scalar_subject: resolved shape=%s but AnswerSubject.Kind=%s at confidence %.2f (>= %.2f)",
				resolvedShape, rm.AnswerSubject.Kind, rm.AnswerSubject.Confidence, coherenceSubjectConfidenceFloor),
		}
	}

	return types.GateCheck{
		Name:   "shape_subject_coherence",
		Passed: true,
		Score:  1.0,
		Detail: fmt.Sprintf("shape=%s subject_kind=%s scalar_pred=%t sub_topics=%d",
			resolvedShape, rm.AnswerSubject.Kind, rm.Predicates.IsScalarAnswer, nSub),
	}
}

// extractDistinctTermDomains returns the set of distinct repomap
// domains carried by TermGraph.Canonical entries that are
// resolver-verified TermSymbols at confidence ≥ floor. Empty
// Domain values and non-symbol kinds are excluded so the count
// reflects "code regions the user named" rather than every term
// surface.
func extractDistinctTermDomains(tg types.TermGraph) []string {
	if len(tg.Canonical) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(tg.Canonical))
	for _, c := range tg.Canonical {
		if c.Kind != types.TermSymbol {
			continue
		}
		if c.Confidence < coherenceTermSymbolMinConfidence {
			continue
		}
		domain := strings.TrimSpace(c.Domain)
		if domain == "" {
			continue
		}
		seen[domain] = true
	}
	if len(seen) == 0 {
		return nil
	}
	out := make([]string, 0, len(seen))
	for d := range seen {
		out = append(out, d)
	}
	sort.Strings(out)
	return out
}

// flattenSubTopicEntities collects every entity surface mentioned
// in any SubTopic's Entities slice into a single dedup'd list. Used
// by R1.3 to compare against PrimaryEntities.
func flattenSubTopicEntities(subs []types.SubTopic) []string {
	if len(subs) == 0 {
		return nil
	}
	seen := make(map[string]bool)
	out := make([]string, 0, len(subs))
	for _, st := range subs {
		for _, e := range st.Entities {
			trimmed := strings.TrimSpace(e)
			if trimmed == "" || seen[trimmed] {
				continue
			}
			seen[trimmed] = true
			out = append(out, trimmed)
		}
	}
	return out
}

// anyOverlap returns true when the two slices share at least one
// element (case-sensitive). Both inputs are de-duplicated by the
// callers so a linear scan is fine.
func anyOverlap(a, b []string) bool {
	if len(a) == 0 || len(b) == 0 {
		return false
	}
	bset := make(map[string]bool, len(b))
	for _, s := range b {
		trimmed := strings.TrimSpace(s)
		if trimmed != "" {
			bset[trimmed] = true
		}
	}
	for _, s := range a {
		if bset[strings.TrimSpace(s)] {
			return true
		}
	}
	return false
}

// isScalarSubjectKind groups the AnswerSubjectKind values whose
// answer is, by definition, a single literal value rather than a
// narrative. Used by R2.2 to detect Explanation-shape contradictions.
func isScalarSubjectKind(k types.AnswerSubjectKind) bool {
	switch k {
	case types.SubjectNumeric, types.SubjectStringLiteral, types.SubjectReturnValue:
		return true
	}
	return false
}

// formatDomainList renders a sorted, bracketed list of domains for
// the GateCheck.Detail string. Pure presentation; the underlying
// slice already came from a sorted source in extractDistinctTermDomains.
func formatDomainList(domains []string) string {
	if len(domains) == 0 {
		return "[]"
	}
	return "[" + strings.Join(domains, ", ") + "]"
}

// formatStringList trims and bounds a string slice for inclusion in
// a GateCheck.Detail message. Detail strings ride the retry-hint
// path, so a 100-character cap keeps the prompt snippet readable
// when the LLM emitted a long entity list.
func formatStringList(items []string) string {
	if len(items) == 0 {
		return "[]"
	}
	const maxItems = 8
	const maxBytes = 100
	out := items
	if len(out) > maxItems {
		out = append([]string(nil), out[:maxItems]...)
		out = append(out, fmt.Sprintf("… +%d more", len(items)-maxItems))
	}
	joined := "[" + strings.Join(out, ", ") + "]"
	if len(joined) > maxBytes {
		joined = joined[:maxBytes-1] + "…]"
	}
	return joined
}
