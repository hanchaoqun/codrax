package tracefinding

import (
	"fmt"
	"strings"

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
	item := &types.TraceRootCauseItemV2{
		CandidateID:   strings.TrimSpace(decision.CandidateID),
		Category:      category,
		ImpactSeconds: &impactSeconds,
		Evidence:      []string{boundRootCauseEvidence(decision)},
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
	switch token {
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

func boundRootCauseEvidence(decision types.TraceCauseDecision) string {
	subject := strings.TrimSpace(decision.SubjectName)
	if subject == "" {
		subject = "目标链路"
	}
	refs := strings.Join(decision.EvidenceRefs, ",")
	if refs == "" {
		refs = "typed-trace-row"
	}
	return fmt.Sprintf("%s 在目标窗口内的链上有效影响为 %.3f ms（证据 %s）", subject, decision.Magnitude.Value, refs)
}
