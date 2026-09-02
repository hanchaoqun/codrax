package tracefinding

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/hanchaoqun/codrax/internal/types"
)

// BindRootCauseReportSelection turns a model-owned ordered candidate-id
// selection into the public root-cause sidecar. All semantic fields and
// magnitudes come from the frozen typed contract; model prose is not an
// authority input.
func BindRootCauseReportSelection(in *types.TraceRootCauseReportV2, contract *types.TraceFindingContract) (*types.TraceRootCauseReportV2, error) {
	if in == nil {
		return nil, nil
	}
	if contract == nil || !contract.RootCauseReportEnabled {
		return nil, fmt.Errorf("trace_root_causes is not enabled for this request")
	}
	if in.SchemaVersion != types.TraceRootCauseReportSchemaVersion {
		return nil, fmt.Errorf("trace_root_causes schema_version=%d, want %d", in.SchemaVersion, types.TraceRootCauseReportSchemaVersion)
	}
	byID := make(map[string]types.TraceFindingCandidateV1, len(contract.Candidates))
	for _, candidate := range contract.Candidates {
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
			return nil, fmt.Errorf("trace_root_causes.root_causes[%d] is null", index)
		}
		candidateID := strings.TrimSpace(selection.CandidateID)
		if candidateID == "" {
			return nil, fmt.Errorf("trace_root_causes.root_causes[%d].candidate_id is required", index)
		}
		if seen[candidateID] {
			return nil, fmt.Errorf("trace_root_causes.root_causes[%d].candidate_id duplicates an earlier selection", index)
		}
		candidate, ok := byID[candidateID]
		if !ok {
			return nil, fmt.Errorf("trace_root_causes.root_causes[%d].candidate_id is outside the selectable typed on-chain roster", index)
		}
		item, ok := boundRootCauseItem(candidate)
		if !ok {
			return nil, fmt.Errorf("trace_root_causes.root_causes[%d].candidate_id is not representable in the public report", index)
		}
		seen[candidateID] = true
		bound.RootCauses = append(bound.RootCauses, item)
	}
	report, err := types.NormalizeAndValidateTraceRootCauseReport(bound)
	if err != nil {
		return nil, err
	}
	// Candidate identity owns selection uniqueness, but remains private to the
	// binding transaction. The public v2 sidecar keeps its existing wire shape.
	for _, item := range report.RootCauses {
		item.CandidateID = ""
	}
	return report, nil
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

func boundRootCauseEvidence(decision types.TraceCauseDecision) []string {
	subject := strings.TrimSpace(decision.SubjectName)
	if subject == "" {
		subject = "目标链路"
	}
	refs := decision.EvidenceRefs
	if len(refs) == 0 {
		refs = []string{"typed-trace-row"}
	}
	// SIDECAR-Q1: the sentence speaks the magnitude's own caliber — a raw
	// window projection is never called 有效 (CROWNCAL discipline).
	statement := fmt.Sprintf("%s 在目标窗口内的链上有效归因为 %.3f ms", subject, decision.Magnitude.Value)
	if strings.TrimSpace(decision.Magnitude.Caliber) == types.TraceImpactCaliberWindowProjection {
		statement = fmt.Sprintf("%s 在目标窗口内的窗内投影占用为 %.3f ms（未发布有效归因）", subject, decision.Magnitude.Value)
	}
	evidence := statement + "（证据 " + strings.Join(refs, ",") + "）"
	description := RootCauseValueDescription(decision)
	if description != "" {
		evidence += "；" + description
	}
	if utf8.RuneCountInString(evidence) <= types.TraceRootCauseEvidenceMaxRunes {
		return []string{evidence}
	}
	// Long source handles plus a value-caliber note can exceed one entry even
	// though the existing four-entry schema can represent every fact. Pack at
	// semantic boundaries; never split/truncate a reference or drop the note.
	// Truly oversized atoms/sets remain intact for the strict validator to
	// reject; this formatter cannot silently reduce evidence to make it fit.
	parts := []string{statement}
	if description != "" {
		parts = append(parts, description)
	}
	for _, ref := range refs {
		parts = append(parts, "证据 "+ref)
	}
	var entries []string
	for _, part := range parts {
		last := len(entries) - 1
		if last >= 0 && utf8.RuneCountInString(entries[last])+1+utf8.RuneCountInString(part) <= types.TraceRootCauseEvidenceMaxRunes {
			entries[last] += "；" + part
		} else {
			entries = append(entries, part)
		}
	}
	return entries
}
