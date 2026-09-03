package tracefinding

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/hanchaoqun/codrax/internal/tracefence"
	"github.com/hanchaoqun/codrax/internal/types"
)

// BindRootCauseReportSelection turns a model-owned ordered candidate-id
// selection into the public root-cause sidecar. All semantic fields and
// magnitudes come from the frozen typed contract; model prose is not an
// authority input.
func BindRootCauseReportSelection(in *types.TraceRootCauseReportV2, contract *types.TraceFindingContract) (*types.TraceRootCauseReportV2, error) {
	report, _, err := BindRootCauseReportSelectionWithAdvisories(in, contract)
	return report, err
}

// BindRootCauseReportSelectionWithAdvisories is the binder with the
// SIDECAR-NARR-1 disclosure lane: a description that cites an internal
// reference or exceeds the cap is dropped from its item (the typed selection
// stands) and the reason is returned so the tool result can tell the model
// what to repair instead of silently losing the whole selector.
func BindRootCauseReportSelectionWithAdvisories(in *types.TraceRootCauseReportV2, contract *types.TraceFindingContract) (*types.TraceRootCauseReportV2, []string, error) {
	var advisories []string
	if in == nil {
		return nil, nil, nil
	}
	if contract == nil || !contract.RootCauseReportEnabled {
		return nil, nil, fmt.Errorf("trace_root_causes is not enabled for this request")
	}
	if in.SchemaVersion != types.TraceRootCauseReportSchemaVersion {
		return nil, nil, fmt.Errorf("trace_root_causes schema_version=%d, want %d", in.SchemaVersion, types.TraceRootCauseReportSchemaVersion)
	}
	byID := make(map[string]types.TraceFindingCandidateV1, len(contract.Candidates))
	rosterIDs := make([]string, 0, len(contract.Candidates))
	for _, candidate := range contract.Candidates {
		rosterIDs = append(rosterIDs, candidate.Decision.CandidateID)
		if _, ok := boundRootCauseItem(candidate); ok {
			byID[candidate.Decision.CandidateID] = candidate
		}
	}
	bound := &types.TraceRootCauseReportV2{
		SchemaVersion: types.TraceRootCauseReportSchemaVersion,
		RootCauses:    make([]*types.TraceRootCauseItemV2, 0, len(in.RootCauses)),
	}
	seen := make(map[string]bool, len(in.RootCauses))
	for index, selection := range in.RootCauses {
		if selection == nil {
			return nil, nil, fmt.Errorf("trace_root_causes.root_causes[%d] is null", index)
		}
		candidateID := strings.TrimSpace(selection.CandidateID)
		if candidateID == "" {
			return nil, nil, fmt.Errorf("trace_root_causes.root_causes[%d].candidate_id is required", index)
		}
		if seen[candidateID] {
			return nil, nil, fmt.Errorf("trace_root_causes.root_causes[%d].candidate_id duplicates an earlier selection", index)
		}
		candidate, ok := byID[candidateID]
		if !ok {
			return nil, nil, fmt.Errorf("trace_root_causes.root_causes[%d].candidate_id is outside the selectable typed on-chain roster", index)
		}
		item, ok := boundRootCauseItem(candidate)
		if !ok {
			return nil, nil, fmt.Errorf("trace_root_causes.root_causes[%d].candidate_id is not representable in the public report", index)
		}
		// SIDECAR-NARR-1: the model's plain-language account rides beside the
		// typed facts. It is advisory, so a description that cites an internal
		// reference (checked against the WHOLE roster's ids) is dropped from
		// this item and disclosed; the typed selection is never lost for it.
		description, derr := types.ValidateTraceRootCauseDescription(selection.Description, rosterIDs, fmt.Sprintf("root_causes[%d]", index))
		if derr != nil {
			advisories = append(advisories, fmt.Sprintf("description for root_causes[%d] dropped: %v", index, derr))
			description = ""
		}
		item.Description = description
		seen[candidateID] = true
		bound.RootCauses = append(bound.RootCauses, item)
	}
	report, err := types.NormalizeAndValidateTraceRootCauseReport(bound)
	if err != nil {
		return nil, advisories, err
	}
	// Candidate identity owns selection uniqueness, but remains private to the
	// binding transaction. The public v2 sidecar keeps its existing wire shape.
	for _, item := range report.RootCauses {
		item.CandidateID = ""
	}
	return report, advisories, nil
}

// SelectableRootCauseCandidates returns only exact typed on-chain candidates
// whose duration and category can be represented without guessing.
func SelectableRootCauseCandidates(contract *types.TraceFindingContract) []types.TraceFindingCandidateV1 {
	if contract == nil {
		return nil
	}
	out := make([]types.TraceFindingCandidateV1, 0, len(contract.Candidates))
	for _, candidate := range contract.Candidates {
		if _, ok := boundRootCauseItem(candidate); ok {
			out = append(out, candidate)
		}
	}
	return out
}

func boundRootCauseItem(candidate types.TraceFindingCandidateV1) (*types.TraceRootCauseItemV2, bool) {
	decision := candidate.Decision
	if !candidate.PrimaryEligible || strings.TrimSpace(decision.CandidateID) == "" || decision.Magnitude == nil || decision.Magnitude.Value <= 0 {
		return nil, false
	}
	// impact_seconds is wall-clock. Count, score, and cross-thread CPU-ms
	// candidates stay available to the long answer but cannot be re-labeled as
	// elapsed seconds in this sidecar.
	if !strings.EqualFold(strings.TrimSpace(decision.Magnitude.Unit), "ms") ||
		!strings.EqualFold(strings.TrimSpace(decision.Magnitude.Additivity), "wall_clock_per_thread") {
		return nil, false
	}
	category, ok := rootCauseCategory(decision)
	if !ok {
		return nil, false
	}
	impactSeconds := decision.Magnitude.Value / 1000
	// SIDECAR-Q1 (§40.28 ②): both public qualifiers are bound from the frozen
	// typed contract — caliber from the magnitude, causality from the seat-level
	// index — and are ALWAYS explicit on the wire.
	caliber := strings.TrimSpace(decision.Magnitude.Caliber)
	if !types.ValidTraceImpactCaliber(caliber) {
		return nil, false
	}
	// Both fields are closed-set and REQUIRED on the candidate: a candidate
	// that reaches the binder without an explicit qualifier is dropped, never
	// published as an affirmative "proven" claim minted from a missing field
	// (fail-closed, symmetric with the caliber arm above).
	qualifier := strings.TrimSpace(decision.CausalQualifier)
	if !types.ValidTraceCausalQualifier(qualifier) {
		return nil, false
	}
	item := &types.TraceRootCauseItemV2{
		CandidateID:     strings.TrimSpace(decision.CandidateID),
		Category:        category,
		ImpactSeconds:   &impactSeconds,
		ImpactCaliber:   caliber,
		CausalQualifier: qualifier,
		Evidence:        boundRootCauseEvidence(decision),
	}
	switch category {
	case types.TraceRootCauseGCLongPause, types.TraceRootCauseComputeSupplyShortage:
		// These categories permit an unnamed scope, but retain a supplied
		// subject instead of leaving multiple named causes indistinguishable.
		item.ThreadName = strings.TrimSpace(decision.SubjectName)
	case types.TraceRootCauseIOBlocking,
		types.TraceRootCauseSynchronousBinder,
		types.TraceRootCausePriorityInversion,
		types.TraceRootCauseCPUSchedulingDelay,
		types.TraceRootCauseJITCompilation,
		types.TraceRootCauseShaderCompilation,
		types.TraceRootCauseSleepBlocking:
		item.ThreadName = strings.TrimSpace(decision.SubjectName)
		if item.ThreadName == "" {
			return nil, false
		}
	case types.TraceRootCauseLockContention:
		item.ResourceName = strings.TrimSpace(decision.ResourceName)
		if item.ResourceName == "" {
			return nil, false
		}
	case types.TraceRootCausePhaseHighLoad:
		item.PhaseName = firstNonEmpty(decision.PhaseName, decision.ResourceName, decision.Token.Token)
		if item.PhaseName == "" {
			return nil, false
		}
	}
	return item, true
}

func rootCauseCategory(decision types.TraceCauseDecision) (types.TraceRootCauseCategory, bool) {
	token := strings.ToLower(strings.TrimSpace(decision.Token.Token))
	if rootCauseUsesRunningSupplyDeficit(decision) {
		return types.TraceRootCauseComputeSupplyShortage, true
	}
	switch token {
	case "d_state_or_io_wait", "fragmented_d_state_or_io_wait":
		// The combined family does not establish that all its waiting is IO.
		// Keep the existing broad thread-blocking category unless the typed
		// split proves a pure IO amount; never relabel non-IO D as IO.
		if decision.Magnitude != nil && decision.Magnitude.Components != nil {
			parts := decision.Magnitude.Components
			if !parts.DStateRefinedNonIO && parts.DStateMS == 0 && parts.IOWaitMS > 0 {
				return types.TraceRootCauseIOBlocking, true
			}
		}
		return types.TraceRootCauseSleepBlocking, true
	case "priority_inversion_candidate", "priority_inversion_runnable_wait":
		return types.TraceRootCausePriorityInversion, true
	case "binder_wait":
		return types.TraceRootCauseSynchronousBinder, true
	case "jit_compile":
		return types.TraceRootCauseJITCompilation, true
	case "shader_compile":
		return types.TraceRootCauseShaderCompilation, true
	case "gc_pause":
		return types.TraceRootCauseGCLongPause, true
	}
	switch strings.ToLower(strings.TrimSpace(decision.Token.Lane)) {
	case "scheduling_demand":
		return types.TraceRootCauseCPUSchedulingDelay, true
	case "compute_delivery":
		return types.TraceRootCauseComputeSupplyShortage, true
	case "wakeup_chain":
		return types.TraceRootCauseSleepBlocking, true
	case "io_blocking":
		return types.TraceRootCauseIOBlocking, true
	case "lock_contention":
		return types.TraceRootCauseLockContention, true
	case "cpu_work":
		return types.TraceRootCausePhaseHighLoad, true
	default:
		return "", false
	}
}

func rootCauseUsesRunningSupplyDeficit(decision types.TraceCauseDecision) bool {
	// For these exact producer families, published effective attribution is
	// the supply deficit (RootCauseRankItemEffectiveImpactMs), not RunningMs.
	// A fold beside a raw window projection or a semantic JIT/GC row is NOT
	// permission to relabel that row. Do not alter the registry lane or value.
	switch decision.Token.Token {
	case "running", "fragmented_running":
		return decision.Magnitude != nil && decision.Magnitude.Caliber == "effective_attribution" &&
			decision.Magnitude.Components != nil && decision.Magnitude.Components.SupplyFoldComputed
	default:
		return false
	}
}

// RootCauseValueDescription is shared by the selector context and public
// evidence so the model is told the same precise value meaning we publish.
// It does not select/rank causes or rewrite the answer document.
func RootCauseValueDescription(decision types.TraceCauseDecision) string {
	if decision.Magnitude == nil {
		return ""
	}
	parts := decision.Magnitude.Components
	if rootCauseUsesRunningSupplyDeficit(decision) {
		description := fmt.Sprintf("供给折算缺口（估算下界，非全部运行耗时）；频率已知 %.3f ms，未知 %.3f ms", parts.SupplyFoldKnownMS, parts.SupplyFoldUnknownMS)
		switch parts.SupplyFoldCapabilitySource {
		case "default_table":
			description += "；采用默认算力比"
		case "freq_only":
			description += "；仅按频率比折算"
		case "evidence_table":
			description += "；采用证据支持的算力比"
		}
		return description
	}
	switch decision.Token.Token {
	case "d_state_or_io_wait", "fragmented_d_state_or_io_wait":
		if parts != nil {
			if parts.DStateRefinedNonIO && parts.IOWaitMS == 0 {
				return "D 状态等待，已有非 I/O 证据"
			}
			if parts.DStateMS > 0 || parts.IOWaitMS > 0 {
				return fmt.Sprintf("等待组成：D 状态 %.3f ms，I/O 等待 %.3f ms；不是可直接消除的承诺", parts.DStateMS, parts.IOWaitMS)
			}
		}
		return "D 状态与 I/O 等待的合并口径，不能全部视为 I/O"
	}
	return ""
}

// boundRootCauseEvidence renders the public sidecar evidence (SIDECAR-EVID-1,
// customer report 2026-09-02 → §40.32): up to four customer-readable
// sentences — 量化 / 链路关系与凭证 / 机理与边界 / trace 定位 — from the
// system-owned typed facts. Internal artifact paths and trace_query result
// ids are NEVER published (the customer cannot open them); the attachment
// line range and timestamps are the locators a reader can follow in their
// own trace. Legacy candidates without facts keep the quantified sentence.
func boundRootCauseEvidence(decision types.TraceCauseDecision) []string {
	subject := strings.TrimSpace(decision.SubjectName)
	if subject == "" {
		subject = "目标链路"
	}
	// SIDECAR-Q1: the sentence speaks the magnitude's own caliber — a raw
	// window projection is never called 有效 (CROWNCAL discipline).
	statement := fmt.Sprintf("%s 在目标窗口内的链上有效归因为 %.3f ms", subject, decision.Magnitude.Value)
	if strings.TrimSpace(decision.Magnitude.Caliber) == types.TraceImpactCaliberWindowProjection {
		statement = fmt.Sprintf("%s 在目标窗口内的窗内投影占用为 %.3f ms（未发布有效归因）", subject, decision.Magnitude.Value)
	}
	if description := RootCauseValueDescription(decision); description != "" {
		statement += "；" + description
	}
	entries := []string{statement}
	if facts := decision.EvidenceFacts; facts != nil {
		if relation := rootCauseEvidenceRelationSentence(subject, facts); relation != "" {
			entries = append(entries, relation)
		}
		if mechanism := rootCauseEvidenceMechanismSentence(decision, facts); mechanism != "" {
			entries = append(entries, mechanism)
		}
		if locator := rootCauseEvidenceLocatorSentence(facts); locator != "" {
			entries = append(entries, locator)
		}
	}
	for i := range entries {
		entries[i] = rootCauseEvidenceFit(entries[i])
	}
	if len(entries) > types.TraceRootCauseEvidenceMaxEntries {
		entries = entries[:types.TraceRootCauseEvidenceMaxEntries]
	}
	return entries
}

// rootCauseEvidenceRelationSentence — 链路关系与凭证.
func rootCauseEvidenceRelationSentence(subject string, facts *types.TraceCauseEvidenceFacts) string {
	var parts []string
	target := strings.TrimSpace(facts.TargetSubject)
	switch {
	case target != "" && strings.EqualFold(subject, target):
		parts = append(parts, "该线程即分析目标自身")
	case facts.ChainDepth > 0 && target != "":
		parts = append(parts, fmt.Sprintf("位于目标 %s 唤醒依赖链第 %d 级（分支 %d）", target, facts.ChainDepth, facts.ChainBranch))
	case facts.ChainDepth > 0:
		parts = append(parts, fmt.Sprintf("位于目标唤醒依赖链第 %d 级（分支 %d）", facts.ChainDepth, facts.ChainBranch))
	case facts.ChainRelevance == "on_chain":
		parts = append(parts, "位于目标唤醒链上")
	case facts.ChainRelevance == "adjacent":
		parts = append(parts, "位于目标唤醒链邻近（无链上凭证）")
	}
	switch facts.OnChainBasis {
	case types.TraceCausalOnChainBasisHostWakeupEdgeSpan, "host_wakeup_edge_pre_state":
		via := "唤醒边"
		switch facts.HostWakeupEdgeVia {
		case "direct":
			via = "直接唤醒边"
		case "chain_hop":
			via = "链跳唤醒边"
		case "direct+chain_hop":
			via = "直接唤醒边与链跳边"
		}
		if facts.HostWakeupEdgeAnchorTs > 0 {
			parts = append(parts, fmt.Sprintf("凭证=唤醒锚定：该线程于 %.6f s 通过%s唤醒目标，边前份按边=凭证/边前=有效/边后=解除计入", facts.HostWakeupEdgeAnchorTs, via))
		} else {
			parts = append(parts, "凭证=唤醒锚定：该线程持有对目标的窗内唤醒边，边前份计入")
		}
	case types.TraceCausalOnChainBasisSemanticChainIntervalRelation:
		parts = append(parts, "凭证=交集证明：该确定性语义工作与目标唤醒链的 typed 区间精确相交，相交份计入")
	case types.TraceCausalOnChainBasisSelfDeterministicSpan, "self_wall_clock_interval":
		parts = append(parts, "凭证=目标自身：目标线程窗内自身的状态/工作")
	default:
		if facts.ChainRelevance == "on_chain" && facts.OnChainBasis == "" {
			parts = append(parts, "凭证=唤醒链成员：其等待/运行段落落在目标的唤醒依赖窗内")
		}
	}
	if len(facts.WakeupPath) > 0 {
		hops := facts.WakeupPath
		const maxHops = 6
		suffix := ""
		if len(hops) > maxHops {
			suffix = fmt.Sprintf(" … 等 %d 级", len(hops))
			hops = hops[:maxHops]
		}
		parts = append(parts, "唤醒链："+strings.Join(hops, " → ")+suffix)
	}
	if len(parts) == 0 {
		return ""
	}
	return "链路关系：" + strings.Join(parts, "；")
}

// rootCauseEvidenceMechanismSentence — 机理与边界.
func rootCauseEvidenceMechanismSentence(decision types.TraceCauseDecision, facts *types.TraceCauseEvidenceFacts) string {
	var parts []string
	if facts.StateKind != "" {
		parts = append(parts, "状态="+facts.StateKind)
	}
	if facts.SemanticClass != "" && facts.SpanName != "" {
		parts = append(parts, fmt.Sprintf("确定性语义工作 %s（%s）", facts.SpanName, facts.SemanticClass))
	}
	if facts.BlockedReasonCaller != "" {
		parts = append(parts, "阻塞记录调用者="+facts.BlockedReasonCaller)
	}
	if word, ok := tracefence.FixDirectionWord(facts.FixDirection, true); ok {
		parts = append(parts, "修向="+word)
	}
	switch facts.OnChainBasis {
	case types.TraceCausalOnChainBasisHostWakeupEdgeSpan, types.TraceCausalOnChainBasisSemanticChainIntervalRelation:
		parts = append(parts, "语义完成机理未证（仅披露，边前份/相交份仍按凭证规则计价）")
	}
	if decision.CausalQualifier == types.TraceCausalQualifierFrameUnproven {
		parts = append(parts, "帧因果未证：本席位引用的 trace 证据中没有帧证据，该限定不改变有效归因与排序")
	}
	if len(parts) == 0 {
		return ""
	}
	return "机理与边界：" + strings.Join(parts, "；")
}

// rootCauseEvidenceLocatorSentence — trace 定位 (customer-accessible: the
// attachment's line range and timestamps; never an internal path).
func rootCauseEvidenceLocatorSentence(facts *types.TraceCauseEvidenceFacts) string {
	var parts []string
	label := "附件 trace"
	if facts.ArtifactLabel != "" {
		label = facts.ArtifactLabel
	}
	if facts.LineStart > 0 {
		if facts.LineEnd > facts.LineStart {
			parts = append(parts, fmt.Sprintf("%s 第 %d–%d 行", label, facts.LineStart, facts.LineEnd))
		} else {
			parts = append(parts, fmt.Sprintf("%s 第 %d 行", label, facts.LineStart))
		}
	}
	if facts.SeatEndTs > facts.SeatStartTs {
		parts = append(parts, fmt.Sprintf("发生 %.6f–%.6f s", facts.SeatStartTs, facts.SeatEndTs))
	}
	if facts.WindowEndTs > facts.WindowStartTs {
		parts = append(parts, fmt.Sprintf("分析窗 %.6f–%.6f s", facts.WindowStartTs, facts.WindowEndTs))
	}
	if len(parts) == 0 {
		return ""
	}
	return "trace 定位：" + strings.Join(parts, "，")
}

// rootCauseEvidenceFit keeps one entry inside the wire cap at a semantic
// boundary (the last "；" part is dropped first; a single oversized atom is
// cut on a rune boundary with an ellipsis so the strict validator never
// rejects a system-rendered sentence).
func rootCauseEvidenceFit(entry string) string {
	for utf8.RuneCountInString(entry) > types.TraceRootCauseEvidenceMaxRunes {
		if cut := strings.LastIndex(entry, "；"); cut > 0 {
			entry = entry[:cut]
			continue
		}
		runes := []rune(entry)
		return string(runes[:types.TraceRootCauseEvidenceMaxRunes-1]) + "…"
	}
	return entry
}
