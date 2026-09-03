package agent

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/analysis/tracefinding"
	"github.com/hanchaoqun/codrax/internal/logging"
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
	// SIDECAR-Q1 (user ruling 2026-09-02, §40.28 ②): the contract consumes the
	// SEAT-LEVEL evidence-ID authority index — the same one the Markdown crown
	// face consults — never the session-wide ANY aggregate (advisory-only; see
	// TestSessionAnyCausalSignalFeedsAdvisoryLanesOnly). Built from the same
	// ledger input the compiled ledger reads (limit 64, answerDocObservationLedger).
	seatInput := types.ObservationLedgerInputFromAgentContext(ctx, 64)
	seatAuthority := tracefinding.BuildSeatFrameCausalityAuthority(seatInput)
	logging.Debug("[trace_finding] seat authority: frame_question=%v tool_results=%d supplements=%d unproven_keys=%d",
		seatAuthority.Applicable, len(seatInput.ToolResults), len(seatInput.SystemTraceSupplementResults), len(seatAuthority.Index))
	contract, err := tracefinding.CompileCandidateContract(ledger, set, seatAuthority)
	if err != nil {
		return fmt.Errorf("compile trace finding contract: %w", err)
	}
	for _, candidate := range contract.Candidates {
		logging.Debug("[trace_finding] candidate %s subject=%q evidence=%v qualifier=%s",
			candidate.Decision.CandidateID, candidate.Decision.SubjectName, candidate.Decision.EvidenceRefs, candidate.Decision.CausalQualifier)
	}
	// The legacy TraceFindingV1 stays out of the model call. The optional report
	// below accepts only exact ids from the typed on-chain roster; model order
	// owns the conclusion while the runtime owns every copied fact.
	contract.Required = false
	contract.RootCauseReportEnabled = len(tracefinding.SelectableRootCauseCandidates(contract)) > 0
	ctx.Mutable.SetTraceFindingContract(contract)
	return nil
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
	selectable := tracefinding.SelectableRootCauseCandidates(contract)
	if contract.RootCauseReportEnabled && len(selectable) > 0 {
		type candidateView struct {
			CandidateID      string   `json:"candidate_id"`
			CauseKind        string   `json:"cause_kind"`
			Subject          string   `json:"subject,omitempty"`
			Resource         string   `json:"resource,omitempty"`
			Phase            string   `json:"phase,omitempty"`
			Rank             int      `json:"rank,omitempty"`
			ImpactMS         float64  `json:"impact_ms"`
			ImpactCaliber    string   `json:"impact_caliber"`
			CausalQualifier  string   `json:"causal_qualifier"`
			ValueDescription string   `json:"value_description,omitempty"`
			EvidenceRefs     []string `json:"evidence_refs"`
		}
		roster := make([]candidateView, 0, len(selectable))
		for _, candidate := range selectable {
			decision := candidate.Decision
			roster = append(roster, candidateView{
				CandidateID: decision.CandidateID, CauseKind: decision.Token.Token,
				Subject: decision.SubjectName, Resource: decision.ResourceName,
				Phase: decision.PhaseName, Rank: decision.Rank,
				ImpactMS: decision.Magnitude.Value, EvidenceRefs: decision.EvidenceRefs,
				ImpactCaliber: decision.Magnitude.Caliber, CausalQualifier: decision.CausalQualifier,
				ValueDescription: tracefinding.RootCauseValueDescription(decision),
			})
		}
		b, err := json.MarshalIndent(roster, "", "  ")
		if err == nil {
			out.WriteString("\n\n## Optional Trace Root Cause JSON\n\n")
			out.WriteString("- The full answer is the primary deliverable. `trace_root_causes` is optional and never replaces or blocks it. Omitting it on a later re-emit keeps the previously accepted selection; send `\"root_causes\": []` to withdraw it.\n")
			out.WriteString("- If useful, choose zero or more exact `candidate_id` values from the typed on-chain roster below and order them strongest to weakest. The runtime binds category, identity, impact, summary, and evidence from those receipts; do not author those fields.\n")
			out.WriteString("- Use `trace_root_causes` in `emit_answer_document`; use `replace_trace_root_causes` in `emit_answer_document_patch`. Both fields take the same native object: `{\"schema_version\":2,\"root_causes\":[{\"candidate_id\":\"<exact id from roster>\"}]}`. Keep `schema_version` inside that object, not at the document top level. Do not quote the object or the number.\n")
			out.WriteString("- A patch that only revises answer blocks should omit `replace_trace_root_causes`; the previously accepted report is retained. A replacement lists the complete ordered selection, not only the added entries.\n")
			out.WriteString("- Omit the field when no candidate should be selected. Background and adjacent observations are intentionally absent.\n")
			out.WriteString("- Optionally add `description` to each selection: one or two plain-language sentences a reader can follow — which thread or resource did what, for how long, and why that delayed the target (e.g. 「同进程 GC 线程 HeapTaskDaemon 执行并发标记约 12 ms，UIThread 在此期间等待堆锁」). Reuse the roster's impact_ms / value_description; never quote candidate ids, file paths or evidence ids. It is published beside the runtime's typed evidence sentences and never replaces them.\n")
			out.WriteString("- Each candidate carries its own `impact_caliber` (effective_attribution vs window_projection) and `causal_qualifier`, a closed set copied verbatim onto the published sidecar: `proven` (this candidate's own trace evidence was checked for frame evidence and none withholds it), `frame_unproven` (checked and absent or unproven; seat-level — the same qualifier the answer headline wears), or `not_applicable` (this request is not a frame/jank question, so frame causality is not a claim the report makes and no headline qualifier appears). Selecting a frame_unproven candidate is allowed and stays disclosed as such.\n\n")
			out.WriteString("```json\n")
			out.Write(b)
			out.WriteString("\n```\n")
		}
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
