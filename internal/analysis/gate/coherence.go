package gate

import (
	"fmt"
	"sort"
	"strings"

	"github.com/hanchaoqun/codrax/internal/analysis/normalizer"
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
// enum (AnswerSubject.Kind, AnswerSemanticView.Family), an LLM
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
func checkSubtopicCoherence(ir *types.AnalysisIR, resolver normalizer.SymbolResolver) types.GateCheck {
	rm := ir.RequestModel
	nSub := len(rm.SubTopics)

	// B.5 audit followup (2026-05-02): instead of returning on the
	// first failing rule (which hid R1.4 behind R1.5 when both fired),
	// collect every triggering detail and surface them together. The
	// LLM then sees both diagnostics on one retry and can repair the
	// IR in one emit instead of bouncing between rules.
	var (
		details          []string // hard-fail reasons (precise typed contradictions)
		softAdvisories   []string // soft hints (noisy heuristics — never hard-fail alone)
	)
	domains := extractDistinctTermDomains(rm.TermGraph)

	// R1.1 — Domain divergence (SOFT advisory, NOT hard gate).
	//
	// 2026-05-08 R3 audit: extractDistinctTermDomains derives "code
	// area" from FileInfo.Package on resolver-matched TermSymbols —
	// a NOISY heuristic that fires on legitimate single-control-flow
	// questions whose identifiers happen to span multiple packages.
	// Examples that historically tripped this rule:
	//   - "executeTool 调用 validateAnalyzerPrescanToolCall 怎么处理 X"
	//     → spans agent + analysis but is single-control-flow
	//   - "emit_evidence 接收 batch 缺少 anchor_kind 时怎么处理"
	//     → spans tool + types but single behaviour question
	//   - "analyze stage retry 怎么决定" → spans dataflow+tool+orch
	//     but single mechanism question
	// Per R3 red line ("noisy signals only as soft guidance"), R1.1
	// produces an advisory, not a hard fail. R1.2 (precise predicate
	// contradiction) and R1.3-R1.5 (typed entity orphan / asymmetry /
	// axis-collapse) remain hard rules — they are typed-precise.
	if len(domains) >= 2 && nSub <= 1 {
		softAdvisories = append(softAdvisories, fmt.Sprintf(
			"R1.1 domain_divergence (advisory): the question's identifiers span %d distinct code areas %s but only %d sub-topic emitted — if the question genuinely spans independent topics, consider re-emitting with sub_topics; if it is a single cross-area control-flow / behaviour question, the current emit is fine",
			len(domains), formatDomainList(domains), nSub))
	}

	// R1.2 — Predicate self-contradiction.
	// Repair guidance is LLM-actionable: name the structural fix
	// (emit sub_topics) rather than restating the contradiction.
	// Pre-2026-05-02 analyzer_predicate.go used to auto-demote
	// IsCrossComponent here as a workaround; that path was removed
	// because it discarded the LLM's hard signal — the retry hint
	// is the correct repair surface.
	if rm.Predicates.IsCrossComponent && nSub <= 1 {
		details = append(details, fmt.Sprintf(
			"R1.2 predicate_contradiction: predicates.is_cross_component=true but only %d sub-topic emitted — re-emit emit_analysis with at least 2 sub_topics (\"cross-component\" implies >=2 components by definition). Each sub_topic.summary names one component, and sub_topic.entities lists identifiers visible in the repo overview that anchor that component. OR if the question is actually single-topic after all, set is_cross_component=false explicitly",
			nSub))
	}

	// R1.3 — Sub-topic entity orphan.
	if nSub >= 1 && len(rm.AnalyzerHints.PrimaryEntities) >= coherenceMinPrimaryEntitiesForOrphan {
		subEnts := flattenSubTopicEntities(rm.SubTopics)
		if len(subEnts) > 0 {
			if !anyOverlap(subEnts, rm.AnalyzerHints.PrimaryEntities) {
				details = append(details, fmt.Sprintf(
					"R1.3 entity_orphan: sub-topic entities %s share no element with primary entities %s",
					formatStringList(subEnts), formatStringList(rm.AnalyzerHints.PrimaryEntities)))
			}
		}
	}

	// R1.5 — Sub-topic entity asymmetry (resolver-backed precision of
	// R1.3). The signal we're catching is INTRA-IR asymmetry: the
	// original 2026-05-02 failure had nSub=3 where two sub-topics had
	// concrete entities (PipelineMode, analyze, plan) but one had a
	// hallucinated `mode_dispatch` token the repo never defined. The
	// downstream multi-topic anchor backbone then strict-substring-
	// matched against the absent identifier forever.
	//
	// Pre-2026-05-02 (initial commit) the rule fired when ANY sub-
	// topic had zero resolved entities, which is too strict: a
	// uniformly-conceptual cross-component question ("compare the
	// explorer stage with the verify stage") legitimately emits
	// sub-topics with stage/phase names that aren't Tier 1-2 symbols
	// even though the underlying components are real. Audit run
	// 2026-05-02 07:06 surfaced exactly this regression.
	//
	// Refined rule: fire only when resolved/unresolved is MIXED —
	// some sub-topics resolve, some don't. A single uniformly-
	// unresolved IR is treated as "concept-only question, gate not
	// applicable here" and passes. R1.4 (axis_collapse) and R1.3
	// still cover their respective failure modes.
	//
	// Evaluates BEFORE R1.4 because the unresolvable-entity repair
	// (drop / rename) is more specific than the axis-collapse repair
	// (collapse to list_of_symbols). Disabled when resolver is nil.
	if resolver != nil && nSub >= 2 {
		type subTopicState struct {
			index int
			topic types.SubTopic
			hit   bool
		}
		states := make([]subTopicState, 0, nSub)
		anyHit := false
		anyMiss := false
		for i, st := range rm.SubTopics {
			if len(st.Entities) == 0 {
				// No entities to validate — neutral, neither hit
				// nor miss. Excluded from the asymmetry calculation.
				continue
			}
			s := subTopicState{index: i, topic: st}
			for _, ent := range st.Entities {
				trimmed := strings.TrimSpace(ent)
				if hits := resolver.LookupSymbol(trimmed); len(hits) > 0 {
					s.hit = true
					break
				}
				// Fix J (2026-05-07 m1b r1 forensic): when exact +
				// flat lookup misses, try stem-aware fallback for
				// user concept-form (`AnalyzerAgent`) ↔ repo
				// implementation-form (`analyzerEvaluator`)
				// translation. Optional protocol — not every
				// resolver implementation supports stem matching;
				// type-assert and skip when absent.
				if stemr, ok := resolver.(interface {
					LookupSymbolStem(string) []normalizer.SymbolHit
				}); ok {
					if hits := stemr.LookupSymbolStem(trimmed); len(hits) > 0 {
						s.hit = true
						break
					}
				}
			}
			states = append(states, s)
			if s.hit {
				anyHit = true
			} else {
				anyMiss = true
			}
		}
		if anyHit && anyMiss {
			var unresolved []string
			for _, s := range states {
				if s.hit {
					continue
				}
				unresolved = append(unresolved, fmt.Sprintf(
					"sub-topic %d %q (entities=%s)",
					s.index+1, summaryShort(s.topic.Summary, 40),
					formatStringList(s.topic.Entities)))
			}
			details = append(details, "R1.5 entity_unresolvable: "+strings.Join(unresolved, "; ")+
				" — these sub-topics' entities don't resolve to any repo symbol, while sibling sub-topics did; the asymmetry suggests one sub-topic was hallucinated. Rename the entities to identifiers visible in the repo overview, or drop the unresolvable sub-topic and merge its content into a sibling")
		}
	}

	// R1.4 — Sub-topic axis collapse.
	// When the LLM emits ≥2 sub-topics whose resolver-validated
	// entities all settle into ≤1 distinct repomap Domain, AND it
	// self-judged IsCrossComponent=false, AND the question is
	// enumeration-shaped (IsCategoryEnumeration OR Intent=enumerate),
	// the LLM has decomposed a single enumeration axis into per-value
	// sub-topics — a structural over-decomposition the multi-topic
	// anchor backbone cannot satisfy.
	//
	// CONTRACT for upstream writers (analyzer reconcilers / amplifier
	// rules): if you ever flip IsCategoryEnumeration to true, OR
	// derive new SubTopics, you MUST not steer rm into the four-
	// condition combo below, or the analyzer enters a retry storm
	// that exhausts the budget. The amplifier package encodes its
	// alignment defense as gate #2 in r2TypedNameParitySubTopics
	// and gate #6 in r1MultiSubjectPredicate; new amplifier rules
	// are pinned by axis_collapse_fixture_test.go.
	//
	// Each predicate is load-bearing:
	//  - nSub >= 2: complementary to R1.1/R1.2 which guard the
	//    under-count direction; R1.4 guards the over-decompose direction.
	//  - ≤1 distinct Domain: same Domain semantic R1.1 uses (FileInfo
	//    .Package + parent-dir fallback from symbol_resolver.go).
	//  - !IsCrossComponent: an LLM-self-judged cross-component question
	//    legitimately spans ≥2 subsystems — never collapse, the user
	//    intent is the source of truth.
	//  - IsCategoryEnumeration OR Intent=IntentEnumerate: enumeration is
	//    the structural marker the LLM itself emits; without this guard
	//    R1.4 would collide with multi-aspect explanations of one
	//    component.
	if resolver != nil && nSub >= 2 && !rm.Predicates.IsCrossComponent &&
		(rm.Predicates.IsCategoryEnumeration || rm.Intent == types.IntentEnumerate) {
		seen := make(map[string]bool)
		for _, st := range rm.SubTopics {
			for _, ent := range st.Entities {
				for _, hit := range resolver.LookupSymbol(strings.TrimSpace(ent)) {
					if d := strings.TrimSpace(hit.Domain); d != "" {
						seen[d] = true
					}
				}
			}
		}
		if len(seen) <= 1 {
			collapsed := make([]string, 0, len(seen))
			for d := range seen {
				collapsed = append(collapsed, d)
			}
			sort.Strings(collapsed)
			details = append(details, fmt.Sprintf(
				"R1.4 axis_collapse: %d sub-topics all sit in the same code area %s under enumeration intent — your sub-topics enumerate values of one axis (e.g. one mode value per sub_topic) rather than independently-answerable questions. Repair by re-emitting emit_analysis ONCE with EITHER (a) sub_topics omitted entirely (one ordered_list item per enumerated value will be rendered downstream — this is the natural form for \"what kinds of X exist\" questions), OR (b) sub_topics retained but each sub-topic's entities now span >=2 distinct code areas so the question really is multi-axis",
				nSub, formatDomainList(collapsed)))
		}
	}

	// R1.6 design note (Plan E, 2026-05-02): we considered an
	// auto-firing rule "IsCategoryEnumeration=true + no
	// completeness_obligation + no enumeration_boundary → flag" but
	// the rule false-positives on every enumeration question whose
	// user did NOT use a universal-quantifier ("what kinds of agents
	// exist?" — legitimate enumeration without "all" cue). Without a
	// keyword table (red line) the gate cannot distinguish "user
	// asked for all" from "user asked for examples". Forcing the LLM
	// to emit an obligation on every enumeration would spam
	// false-positive retries on the majority of legit list questions.
	//
	// Like R1.7, R1.6 is delegated to the skill prompt (E-7): the
	// emit_analysis schema description for completeness_obligation
	// teaches the Decision rule. When the LLM correctly emits, G1/G2
	// enforce. When the LLM misses, downstream behaves like pre-E.

	// R1.7 design note (Plan E, 2026-05-02): we considered an
	// auto-firing rule "≥2 sub-topics + 0 buckets → flag" but every
	// shape-based heuristic to distinguish "user-named partition
	// labels" from legitimate investigation-only sub-topics
	// (summary-length, summary-shape, entity-disjointness) had
	// unacceptable false-positive rates on existing fixtures. Without
	// a keyword table (red line) the gate cannot reliably classify
	// "the user named these groups" from prose alone.
	//
	// Bucket emission is instead taught entirely at the skill-prompt
	// level (E-7): the analyzer's emit_analysis schema description
	// + the analysis-skill workflow rule include the Decision rule
	// for buckets. When the LLM correctly emits buckets[], G3
	// (bucket-aligned shape gate) enforces them downstream. When the
	// LLM misses, G3 simply doesn't fire — which is the same
	// behaviour as pre-Plan-E.
	//
	// Future option: an LLM-driven post-pass could classify whether
	// the question carried a partition; that's a sub-agent dispatch,
	// not a structural gate, and is out of scope for this commit.

	if len(details) > 0 {
		// B.5 multi-rule merge: when ≥2 rules fire, surface every detail
		// joined by " | " so the LLM sees all diagnostics on one retry.
		// plainCoherenceDetail's multi-prefix strip (analyzer.go) handles
		// translating each prefix to plain prose for the LLM hint.
		//
		// Soft advisories (R1.1 domain_divergence in 2026-05-08 R3
		// audit) tag along when a hard rule is also firing — they
		// give the LLM more context for the repair, but never fail
		// the gate alone. Joined with the same " | " separator so
		// retry-hint composer's multi-prefix strip handles them
		// uniformly.
		merged := append([]string{}, details...)
		merged = append(merged, softAdvisories...)
		return types.GateCheck{
			Name:   "subtopic_coherence",
			Passed: false,
			Detail: strings.Join(merged, " | "),
		}
	}

	if len(softAdvisories) > 0 {
		// Pass with advisory — Score < 1.0 so retry-budget heuristics
		// can prefer iter where every check is fully clean, but the
		// gate does NOT block the IR. R3 second-axis rule: noisy
		// signals (R1.1 domain_divergence is system-inferred from
		// TermGraph package distribution, not an LLM self-
		// contradiction) only steer with soft hints; hard gates
		// stay reserved for precise typed contradictions (R1.2 +
		// R1.3 + R1.4 + R1.5).
		return types.GateCheck{
			Name:    "subtopic_coherence",
			Passed:  true,
			Score:   0.7,
			Detail:  strings.Join(softAdvisories, " | "),
		}
	}

	return types.GateCheck{
		Name:   "subtopic_coherence",
		Passed: true,
		Score:  1.0,
		Detail: fmt.Sprintf("sub-topics=%d domains=%d primary_entities=%d buckets=%d",
			nSub, len(domains), len(rm.AnalyzerHints.PrimaryEntities), len(rm.Buckets)),
	}
}

// summaryShort truncates a sub-topic summary for inclusion in a gate
// detail message. Cap at runeMax runes (not bytes) so multi-byte CJK
// characters truncate cleanly. Trailing ellipsis indicates truncation.
func summaryShort(s string, runeMax int) string {
	s = strings.TrimSpace(s)
	if runeMax <= 0 {
		return s
	}
	r := []rune(s)
	if len(r) <= runeMax {
		return s
	}
	return string(r[:runeMax]) + "…"
}

// checkShapeSubjectCoherence enforces two cross-signal invariants on
// the AnswerSemanticView ↔ AnswerSubject ↔ Predicates triangle. Both
// routes catch genuine LLM contradictions rather than judgement calls.
//
// R2.1 Scalar vs multi-topic: Predicates.IsScalarAnswer is true but
//      the LLM emitted ≥2 sub-topics. A scalar answer cannot be the
//      union of multiple independently-answerable sub-topics.
//
// R2.2 Long-form view with scalar subject: the compiled
//      AnswerSemanticView's principal payload is NOT a scalar
//      (i.e. an explanation / call-chain / enumeration prose answer)
//      but AnswerSubject.Kind is one of the single-value subject
//      kinds (Numeric, StringLiteral, ReturnValue) at confidence ≥
//      floor. Long-form answers cannot "be" a single literal value.
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

	// R2.2 — Long-form view with high-confidence scalar subject.
	view := types.BuildAnswerSemanticView(ir, nil)
	family := types.QuestionFamily("")
	isLongForm := false
	if view != nil {
		family = view.Family
		// Principal payload is NOT a scalar literal — i.e. the
		// answer is prose / ordered list / enumeration slate.
		isLongForm = !view.NeedsPrincipalScalar()
	}
	if isLongForm && isScalarSubjectKind(rm.AnswerSubject.Kind) &&
		float32(rm.AnswerSubject.Confidence) >= coherenceSubjectConfidenceFloor {
		return types.GateCheck{
			Name:   "shape_subject_coherence",
			Passed: false,
			Detail: fmt.Sprintf(
				"R2.2 longform_scalar_subject: family=%s expects non-scalar payload but AnswerSubject.Kind=%s at confidence %.2f (>= %.2f)",
				family, rm.AnswerSubject.Kind, rm.AnswerSubject.Confidence, coherenceSubjectConfidenceFloor),
		}
	}

	return types.GateCheck{
		Name:   "shape_subject_coherence",
		Passed: true,
		Score:  1.0,
		Detail: fmt.Sprintf("family=%s subject_kind=%s scalar_pred=%t sub_topics=%d",
			family, rm.AnswerSubject.Kind, rm.Predicates.IsScalarAnswer, nSub),
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
