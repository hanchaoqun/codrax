package agent

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/analysis/tracefinding"
	"github.com/hanchaoqun/codrax/internal/types"
)

// prepareTraceFindingContract activates typed findings only for an actual
// runtime-trace finalizer dispatch. Ordinary source-code and log-only answers
// retain their historical answer-document schema.
func prepareTraceFindingContract(ctx *types.AgentContext) error {
	if ctx == nil || ctx.Mutable == nil {
		return nil
	}
	if current := ctx.Mutable.TraceFindingContract(); current != nil && current.Required && strings.TrimSpace(current.CandidateSetID) != "" {
		return nil
	}
	authority := answerDocRuntimeTraceGuidanceView(ctx)
	if !authority.RuntimeTrace {
		return nil
	}
	ledger := answerDocObservationLedger(ctx)
	set := types.CompileTraceCausalProjectionSet(ledger)
	var requestModel *types.RequestModel
	if ctx.AnalysisIR != nil {
		requestModel = &ctx.AnalysisIR.RequestModel
	}
	if !types.RuntimeTraceReportMaterializationAllowed(requestModel, set) {
		return nil
	}
	ceiling := "proven"
	if authority.CausalUnproven || authority.FrameFlowUnproven {
		ceiling = "unproven"
	}
	contract, err := tracefinding.CompileCandidateContract(ledger, set, ceiling)
	if err != nil {
		return fmt.Errorf("compile trace finding contract: %w", err)
	}
	ctx.Mutable.SetTraceFindingContract(contract)
	return nil
}

func renderTraceFindingContract(ctx *types.AgentContext) string {
	if ctx == nil || ctx.Mutable == nil {
		return ""
	}
	contract := ctx.Mutable.TraceFindingContract()
	if contract == nil || !contract.Required || strings.TrimSpace(contract.CandidateSetID) == "" {
		return ""
	}
	view := struct {
		CandidateSetID      string                          `json:"candidate_set_id"`
		FindingID           string                          `json:"finding_id"`
		AnalysisKey         string                          `json:"analysis_key"`
		ContractHash        string                          `json:"contract_hash"`
		CausalCeiling       string                          `json:"causal_ceiling"`
		Artifact            types.TraceFindingArtifact      `json:"artifact"`
		Scope               types.TraceFindingScope         `json:"scope"`
		Symptom             types.TraceSymptomSummary       `json:"symptom"`
		Candidates          []types.TraceFindingCandidateV1 `json:"candidates"`
		AcceptedEvidenceIDs []string                        `json:"accepted_evidence_ids"`
	}{
		contract.CandidateSetID, contract.FindingID, contract.AnalysisKey,
		contract.ContractHash, contract.CausalCeiling, contract.Artifact,
		contract.Scope, contract.Symptom, contract.Candidates, contract.AcceptedEvidenceIDs,
	}
	b, err := json.MarshalIndent(view, "", "  ")
	if err != nil {
		return ""
	}
	var out strings.Builder
	out.WriteString("\n\n## Trace Finding Contract (Required Typed Sidecar)\n\n")
	out.WriteString("- Submit `trace_finding` in the same `emit_answer_document` transaction. It is required for this trace analysis.\n")
	out.WriteString("- The system owns candidate fields. Select at most one primary candidate and copy its `decision` fields verbatim; you may choose only `status` within the causal ceiling and `confidence`. Do not invent or rewrite token, roles, phase, magnitude, rank, or evidence IDs.\n")
	out.WriteString("- A selected primary cause and `unresolved` are mutually exclusive. If no candidate is sufficiently supported, omit `primary_cause`, emit `unresolved.reason`, and do not turn a context/data-boundary row into a cause.\n")
	out.WriteString("- Copy the fixed finding/artifact/scope/symptom/revision values below. Set `revision.contract_hash` to `contract_hash`; use the listed `finding_id` and `analysis_key`.\n")
	out.WriteString("- This sidecar is the clustering truth. It must agree with the diagnosis in the visible answer and must never be reconstructed from Markdown.\n\n")
	out.WriteString("```json\n")
	out.Write(b)
	out.WriteString("\n```")
	return out.String()
}
