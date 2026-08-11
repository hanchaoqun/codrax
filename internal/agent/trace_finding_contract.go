package agent

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/analysis/tracefinding"
	"github.com/hanchaoqun/codrax/internal/types"
)

// prepareTraceFindingContract activates typed findings only for an actual
// runtime-trace finalizer dispatch. Ordinary source-code, log-only, and narrow
// runtime fact answers retain their historical answer-document schema.
func prepareTraceFindingContract(ctx *types.AgentContext) error {
	if ctx == nil || ctx.Mutable == nil {
		return nil
	}
	if current := ctx.Mutable.TraceFindingContract(); current != nil && strings.TrimSpace(current.CandidateSetID) != "" {
		return nil
	}
	authority := answerDocRuntimeTraceGuidanceView(ctx)
	// The guidance view is evidence-derived and therefore may remain false when
	// trace parsing/querying produced no publication-grade row. The typed
	// preflight/direct attachment is still sufficient to require an unresolved
	// finding for a causal trace analysis; otherwise the visible short-root-
	// cause section disappears exactly when the trace evidence is incomplete.
	if !authority.RuntimeTrace && !traceFindingHasTraceCarrier(ctx) {
		return nil
	}
	ledger := answerDocObservationLedger(ctx)
	set := types.CompileTraceCausalProjectionSet(ledger)
	var requestModel *types.RequestModel
	if ctx.AnalysisIR != nil {
		requestModel = &ctx.AnalysisIR.RequestModel
	}
	// An explicit bounded-fact request (for example "list this TID's state")
	// must not be widened into a root-cause conclusion. Undecided trace shapes
	// are allowed here only because the carrier check above proves that this is
	// an actual trace run; an empty candidate set then fails closed to
	// trace_finding.unresolved.
	if decided, allowed := types.RuntimeTraceReportShapeAuthority(requestModel); decided && !allowed {
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
	// The legacy TraceFindingV1 remains an internal deterministic snapshot.
	// Required=false keeps its candidate-id schema out of the model call; the
	// independent root-cause report below is the only new finalizer field and
	// is never rendered into the long answer.
	contract.Required = false
	contract.RootCauseReportRequired = true
	ctx.Mutable.SetTraceFindingContract(contract)
	return nil
}

func finalizeDeterministicTraceFinding(ctx *types.AgentContext) {
	if ctx == nil || ctx.Mutable == nil {
		return
	}
	contract := ctx.Mutable.TraceFindingContract()
	if contract == nil || strings.TrimSpace(contract.CandidateSetID) == "" {
		return
	}
	ctx.Mutable.SetTraceFinding(tracefinding.BuildDeterministicFinding(contract))
}

func traceFindingHasTraceCarrier(ctx *types.AgentContext) bool {
	if ctx == nil {
		return false
	}
	if ctx.RuntimeArtifactPreflight.HasTraceArtifact() ||
		strings.TrimSpace(ctx.AttachedHitrace) != "" ||
		strings.TrimSpace(ctx.AttachedHitraceSource) != "" ||
		ctx.PerfTrace != nil {
		return true
	}
	if ctx.Mutable != nil && ctx.Mutable.PerfTrace() != nil {
		return true
	}
	return ctx.AnalysisIR != nil && ctx.AnalysisIR.RequestModel.PerfTrace != nil
}

func renderTraceFindingContract(ctx *types.AgentContext) string {
	if ctx == nil || ctx.Mutable == nil {
		return ""
	}
	contract := ctx.Mutable.TraceFindingContract()
	if contract == nil || strings.TrimSpace(contract.CandidateSetID) == "" {
		return ""
	}
	var out strings.Builder
	if contract.RootCauseReportRequired {
		out.WriteString("\n\n## Trace Root Cause JSON (Required)\n\n")
		out.WriteString("- Submit `trace_root_causes` in the same `emit_answer_document` call as the full answer. It is always required for this trace root-cause analysis; no prompt switch is used.\n")
		out.WriteString("- `root_cause_1` is the strongest supported diagnosis and must agree with the full answer. `root_cause_2` is the second independently useful diagnosis, or JSON null when the trace supports only one. Never invent a runner-up.\n")
		out.WriteString("- Choose each category only from the schema enum. Supply the relevant exact thread_name, resource_name, or phase_name. The runtime normalizes `summary` to the fixed Chinese format.\n")
		out.WriteString("- Put 1 to 4 short, trace-specific facts in each cause's `evidence` array. Evidence is concise free text, not a fixed phrase and not an internal evidence id. Include useful measurements or event relationships when available.\n")
		out.WriteString("- This object is written to a separate `.root-causes.json` file. It is not inserted into or substituted for the full answer.\n")
	}
	if !contract.Required {
		return strings.TrimSpace(out.String())
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
