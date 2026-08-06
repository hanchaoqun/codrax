package orchestrator

import (
	"encoding/json"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// renderWriteAnalysisRetryPayload gives a quality-retry dispatch the prior
// typed emit in the same field shape accepted by emit_write_analysis. The
// retry remains model-owned: this is a preservation scaffold, not an automatic
// repair. It prevents one rejected contract field from forcing the model to
// reconstruct unrelated task semantics, constraints, outcomes, and contracts
// from scratch.
func renderWriteAnalysisRetryPayload(ir *types.WriteAnalysisIR) string {
	if ir == nil {
		return ""
	}
	payload := struct {
		RawRequest         string                        `json:"raw_request"`
		Task               types.WriteTask               `json:"task"`
		Risk               types.WriteRiskProfile        `json:"risk"`
		ScopeAnchors       []string                      `json:"scope_anchors,omitempty"`
		Constraints        []types.WriteConstraint       `json:"constraints,omitempty"`
		ExpectedOutcomes   []string                      `json:"expected_outcomes,omitempty"`
		BehaviorContracts  []types.WriteBehaviorContract `json:"behavior_contracts,omitempty"`
		PhaseProposal      types.PhaseProposal           `json:"phase_proposal,omitempty"`
		ApplicablePitfalls []string                      `json:"applicable_pitfalls,omitempty"`
	}{
		RawRequest:         ir.Request.RawRequest,
		Task:               ir.Request.Task,
		Risk:               ir.Request.Risk,
		ScopeAnchors:       ir.Request.ScopeAnchors,
		Constraints:        ir.Request.Constraints,
		ExpectedOutcomes:   ir.Request.ExpectedOutcomes,
		BehaviorContracts:  ir.Request.BehaviorContracts,
		PhaseProposal:      ir.PhaseProposal,
		ApplicablePitfalls: ir.PitfallsApplied,
	}
	raw, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n\n## Previous structured payload (repair base)\n\n")
	b.WriteString("The previous payload passed the tool schema and failed only the named quality check above. Re-emit the full payload from this base: preserve task, risk, scope anchors, constraints, expected outcomes, unrelated behavior contracts, phase proposal, and applicable pitfalls exactly; change only the rejected contract field(s) needed to satisfy the named check. Do not reclassify or reinterpret unrelated source behavior during this local repair.\n\n```json\n")
	b.Write(raw)
	b.WriteString("\n```")
	return b.String()
}
