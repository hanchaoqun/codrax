package types

import (
	"sort"
	"strings"
)

// EVALFIX-2E (CLASS 5, 2026-07-30,
// docs/design/evalfix2_class_designs_20260730.md CLASS 5) — typed
// degradation ledger.
//
// The repo has a family of deliberate fail-open lanes: the system
// deterministically softens / rewrites / downgrades some promised
// surface, logs one WARN, and the user delivery surface discloses
// nothing. This file is the single accounting point for that family:
//
//   - DegradationLaneRegistry is the CLOSED single source of truth for
//     lane classification. A lane classified ClassAnswerSemantics
//     participates in the user-facing disclosure footer (rendered at
//     the last-mile chokepoint in internal/agent); a lane classified
//     ClassPlumbing is registered for the operator-side ledger line
//     only and NEVER rides a user surface (噪音从源头消除 red line —
//     honest classification includes honestly NOT disclosing pipe
//     shaping that has zero answer-semantics impact).
//   - Self-disclosing lanes (degraded delivery footer, authority
//     hedging, Tier2 aggregate-facts wording, L7 mermaid fence, runtime
//     citation detach, supplement disclosure) are deliberately NOT
//     registered: they already own a user-facing disclosure surface and
//     an entry here would double-disclose.
//   - F9 (budget-capped under-exploration) is deliberately absent per
//     the completion-gate ownership ruling — no lane, no gate.
//
// Adding lane N+1 costs one registry row + either one AppendDegradation
// call at the writer or one projection arm in
// BuildDegradationLedgerView. Footer, grouping, bilingual wording,
// config gating and the test rig are all inherited.
type DegradationLaneID string

// DegradationClass is the closed two-value classification consumed by
// the disclosure footer. Only ClassAnswerSemantics lanes ever render on
// the user surface.
type DegradationClass string

const (
	ClassAnswerSemantics DegradationClass = "answer_semantics"
	ClassPlumbing        DegradationClass = "plumbing"
)

// Lane IDs are internal tokens — they ride WARN logs and the
// operator-side "[degrade] ledger:" line, NEVER a user surface (the
// footer renders the registry ZH/EN words only; word-discipline test
// pins NOT-contains for every token).
const (
	// Answer-semantics lanes disclose via the footer.
	DegradeLaneRichnessFacetSoftened  DegradationLaneID = "richness_facet_softened"
	DegradeLaneCompletenessDowngraded DegradationLaneID = "completeness_downgraded"

	// Plumbing lanes (LLM↔tool interface shaping with zero
	// answer-semantics impact). Citation quote hydration/correction is a
	// source-grounding sidecar operation: it never changes model prose or
	// conclusions, so it remains operator-visible but not user-visible.
	// Registered classification is an explicit reviewed decision and even
	// a future AppendDegradation on these lanes never reaches the footer.
	DegradeLaneCitationQuoteRewrite    DegradationLaneID = "citation_quote_rewrite"
	DegradeLaneToolParamCompat         DegradationLaneID = "tool_param_compat"
	DegradeLaneStructuredPayloadCompat DegradationLaneID = "structured_payload_compat"
	DegradeLaneLLMJSONRepair           DegradationLaneID = "llm_json_repair"
	DegradeLaneREPLParamRepair         DegradationLaneID = "repl_param_repair"
	DegradeLaneClassifierFallback      DegradationLaneID = "classifier_fallback"
	DegradeLaneMultirepoFocusFallback  DegradationLaneID = "multirepo_focus_fallback"
)

// DegradationLaneSpec carries the classification plus the ONLY words
// allowed to represent the lane on a user surface. Both languages must
// be non-empty for every registered lane (closed-set tripwire test).
type DegradationLaneSpec struct {
	Class DegradationClass
	ZH    string
	EN    string
}

// DegradationLaneRegistry is the closed-set single source of truth.
// degradedSectionDisplayNames discipline applies: internal tokens never
// ride a user surface; an unregistered lane observed in the ledger
// fails open to the generic word at render time — never invent a
// specific name.
var DegradationLaneRegistry = map[DegradationLaneID]DegradationLaneSpec{
	DegradeLaneCitationQuoteRewrite:   {ClassPlumbing, "引用摘录回填", "citation quote backfill"},
	DegradeLaneRichnessFacetSoftened:  {ClassAnswerSemantics, "因证据不足改为建议项的原定内容", "requested content treated as advisory because evidence was insufficient"},
	DegradeLaneCompletenessDowngraded: {ClassAnswerSemantics, "只能确认下界、尚不能确认完整性的清单", "lists confirmed only as a lower bound"},

	DegradeLaneToolParamCompat:         {ClassPlumbing, "工具参数归一", "tool parameter compat"},
	DegradeLaneStructuredPayloadCompat: {ClassPlumbing, "结构化载荷修复", "structured payload compat"},
	DegradeLaneLLMJSONRepair:           {ClassPlumbing, "JSON 结构修复", "LLM JSON structure repair"},
	DegradeLaneREPLParamRepair:         {ClassPlumbing, "REPL 参数修复", "REPL tool parameter repair"},
	DegradeLaneClassifierFallback:      {ClassPlumbing, "分类器回退链", "classifier fallback chain"},
	DegradeLaneMultirepoFocusFallback:  {ClassPlumbing, "多仓聚焦回退", "multi-repo focus fallback"},
}

// DegradationLaneRegistryOrder is the declaration order the footer and
// the operator ledger line emit in (stable output, golden-pinnable).
// The closed-set tripwire pins bidirectional equality with the
// registry's key set, so a lane cannot exist in one without the other.
var DegradationLaneRegistryOrder = []DegradationLaneID{
	DegradeLaneCitationQuoteRewrite,
	DegradeLaneRichnessFacetSoftened,
	DegradeLaneCompletenessDowngraded,
	DegradeLaneToolParamCompat,
	DegradeLaneStructuredPayloadCompat,
	DegradeLaneLLMJSONRepair,
	DegradeLaneREPLParamRepair,
	DegradeLaneClassifierFallback,
	DegradeLaneMultirepoFocusFallback,
}

// DegradationEntry is one grouped ledger row: typed lane + precise
// count. Deliberately NO detail prose field — full detail stays in the
// existing WARN logs; keeping prose out of the ledger structurally
// prevents internal wording from leaking onto the user surface.
type DegradationEntry struct {
	Lane  DegradationLaneID
	Count int
}

// AppendDegradation accumulates n occurrences on the per-Run ledger.
// n <= 0 and empty lane are no-ops (fail-open: the ledger never blocks
// or errors). Safe for concurrent use. Lifecycle boundary is pipeline
// start only (S12/S13 ruling) — see ResetDegradationLedger.
func (m *MutableState) AppendDegradation(lane DegradationLaneID, n int) {
	if m == nil || n <= 0 || strings.TrimSpace(string(lane)) == "" {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.degradationLedger == nil {
		m.degradationLedger = make(map[DegradationLaneID]int)
	}
	m.degradationLedger[lane] += n
}

// DegradationLedger returns the directly-appended entries (registry
// declaration order, then unregistered lanes lexically). It does NOT
// include the projected channels — renderers must consume
// BuildDegradationLedgerView, which merges the projection arms.
func (m *MutableState) DegradationLedger() []DegradationEntry {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if len(m.degradationLedger) == 0 {
		return nil
	}
	counts := make(map[DegradationLaneID]int, len(m.degradationLedger))
	for lane, n := range m.degradationLedger {
		counts[lane] = n
	}
	return orderedDegradationEntries(counts)
}

// ResetDegradationLedger clears the ledger. The only reset boundary is
// pipeline start (S12/S13 ruling: pipeline start is THE per-turn
// boundary); MutableState is freshly allocated per Run today, so the
// orchestrator's reset-cluster call is defensive documentation of that
// boundary rather than a behavioral wipe.
//
// R7-1 note: R6-5's per-Run self-disclosure marks used to live here too.
// They are gone — a marker set at materialize time outlived a rejected
// recovery draft and silenced the footer on a later CLEAN answer (total
// disclosure loss). Self-disclosed-lane suppression is now derived at
// render time from the document being rendered
// (agent.degradationLaneSelfDisclosedInDoc); there is no run-level
// suppression state to reset.
func (m *MutableState) ResetDegradationLedger() {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.degradationLedger = nil
}

// BuildDegradationLedgerView is the single view every reader (footer
// renderer + operator "[degrade] ledger:" line) consumes: direct
// AppendDegradation entries merged with one-way projections of the two
// EXISTING typed channels. The projection is deliberately one-way — the
// RichnessTelemetry / AnalyzerDecisions channels keep their unique
// writers and readers untouched (no dual-write; 五表手抄 lesson;
// FileReadCoverageStore lesson forbids merging the channels
// themselves). Adding a lane that already has a typed channel means
// adding one projection arm here, never a second collector.
func BuildDegradationLedgerView(m *MutableState) []DegradationEntry {
	if m == nil {
		return nil
	}
	counts := map[DegradationLaneID]int{}
	for _, entry := range m.DegradationLedger() {
		counts[entry.Lane] += entry.Count
	}
	// Projection arm A1: richness facet hard→soft downgrades. Kind
	// literal matches the writer in facet_plan.go (CompileFacetCoverage)
	// and the WARN reader in orchestrator.emitCGECSummary.
	for _, sig := range m.RichnessTelemetry() {
		if sig.Kind == "facet_softened" {
			counts[DegradeLaneRichnessFacetSoftened]++
		}
	}
	// Projection arm A3: extractor completeness complete→lower_bound
	// downgrade. Kind literal matches the writer in extractor.go.
	for _, sig := range m.AnalyzerDecisions() {
		if sig.Kind == "completeness_downgraded" {
			counts[DegradeLaneCompletenessDowngraded]++
		}
	}
	return orderedDegradationEntries(counts)
}

// orderedDegradationEntries renders a count map in registry declaration
// order, then any unregistered lanes (defensive arm) in lexical order,
// dropping non-positive counts. Deterministic so footer and log output
// are golden-pinnable.
func orderedDegradationEntries(counts map[DegradationLaneID]int) []DegradationEntry {
	if len(counts) == 0 {
		return nil
	}
	out := make([]DegradationEntry, 0, len(counts))
	seen := make(map[DegradationLaneID]bool, len(counts))
	for _, lane := range DegradationLaneRegistryOrder {
		if n := counts[lane]; n > 0 {
			out = append(out, DegradationEntry{Lane: lane, Count: n})
		}
		seen[lane] = true
	}
	var rest []DegradationLaneID
	for lane, n := range counts {
		if !seen[lane] && n > 0 {
			rest = append(rest, lane)
		}
	}
	sort.Slice(rest, func(i, j int) bool { return rest[i] < rest[j] })
	for _, lane := range rest {
		out = append(out, DegradationEntry{Lane: lane, Count: counts[lane]})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
