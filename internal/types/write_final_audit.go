package types

import "strings"

const WriteFinalAuditSchemaVersion = 1

// WriteFinalAuditSummary is the offline replay/audit view for a completed or
// auditable write workflow. It is projected only from WriteFinalReport typed
// fields so audit consumers do not need to re-run tools, re-call LLMs, or parse
// terminal prose.
type WriteFinalAuditSummary struct {
	SchemaVersion int             `json:"schema_version"`
	Kind          WriteOutputKind `json:"kind"`
	SourcePath    string          `json:"source_path,omitempty"`
	ReportPath    string          `json:"report_path,omitempty"`

	RunID         string                   `json:"run_id,omitempty"`
	RunStatus     WriteWorkflowRunStatus   `json:"run_status,omitempty"`
	Goal          string                   `json:"goal,omitempty"`
	Completion    *WriteWorkflowCompletion `json:"completion,omitempty"`
	ActiveBatchID string                   `json:"active_batch_id,omitempty"`
	BatchCount    int                      `json:"batch_count,omitempty"`
	Batches       []WriteFinalBatchSummary `json:"batches,omitempty"`

	Artifacts       WriteFinalArtifactRefs           `json:"artifacts,omitempty"`
	Plan            WriteFinalPlanSummary            `json:"plan,omitempty"`
	Patch           WriteFinalPatchSummary           `json:"patch,omitempty"`
	Verification    WriteFinalVerificationSummary    `json:"verification,omitempty"`
	Proof           VerificationProofProfile         `json:"proof,omitempty"`
	ProofLedger     VerificationProofLedger          `json:"proof_ledger,omitempty"`
	Delivery        WriteFinalDeliverySummary        `json:"delivery,omitempty"`
	SourceAuthority WriteFinalSourceAuthoritySummary `json:"source_authority,omitempty"`
	PatchReview     WriteFinalPatchReviewSummary     `json:"patch_review,omitempty"`
	Impact          WriteFinalImpactSummary          `json:"impact,omitempty"`
	Handoff         WriteFinalHandoffSummary         `json:"handoff,omitempty"`
	Loop            *WriteFinalLoopSummary           `json:"loop,omitempty"`
	ReasoningGraph  *WriteFinalReasoningGraphSummary `json:"reasoning_graph,omitempty"`
	GraphAudit      *ReasoningGraphAuditSummary      `json:"graph_audit,omitempty"`
	ResidualRisks   []WriteFinalResidualRisk         `json:"residual_risks,omitempty"`
}

func BuildWriteFinalAuditSummary(report WriteFinalReport, sourcePath string) WriteFinalAuditSummary {
	normalized := NormalizeWriteFinalReport(report)
	sourcePath = strings.TrimSpace(sourcePath)
	reportPath := strings.TrimSpace(normalized.Artifacts.ReportPath)
	if reportPath == "" {
		reportPath = sourcePath
	}
	out := WriteFinalAuditSummary{
		SchemaVersion:   WriteFinalAuditSchemaVersion,
		Kind:            WriteOutputFinalAudit,
		SourcePath:      sourcePath,
		ReportPath:      reportPath,
		RunID:           normalized.RunID,
		RunStatus:       normalized.RunStatus,
		Goal:            normalized.Goal,
		ActiveBatchID:   normalized.ActiveBatchID,
		BatchCount:      len(normalized.Batches),
		Batches:         append([]WriteFinalBatchSummary(nil), normalized.Batches...),
		Artifacts:       normalized.Artifacts,
		Plan:            normalized.Plan,
		Patch:           normalized.Patch,
		Verification:    normalized.Verification,
		Proof:           normalized.Proof,
		ProofLedger:     normalized.ProofLedger,
		Delivery:        normalized.Delivery,
		SourceAuthority: normalized.SourceAuthority,
		PatchReview:     normalized.PatchReview,
		Impact:          normalized.Impact,
		Handoff:         normalized.Handoff,
		ResidualRisks:   append([]WriteFinalResidualRisk(nil), normalized.ResidualRisks...),
	}
	if normalized.Completion != nil {
		completion := *normalized.Completion
		out.Completion = &completion
	}
	if normalized.Loop != nil {
		loop := *normalized.Loop
		loop.EventRefs = append([]string(nil), normalized.Loop.EventRefs...)
		loop.RuntimeUnits = append([]WriteFinalRuntimeUnitSummary(nil), normalized.Loop.RuntimeUnits...)
		out.Loop = &loop
	}
	if normalized.ReasoningGraph != nil {
		graph := *normalized.ReasoningGraph
		graph.EventRefs = append([]string(nil), normalized.ReasoningGraph.EventRefs...)
		out.ReasoningGraph = &graph
		out.GraphAudit = ReasoningGraphAuditFromWriteSummary(normalized.ReasoningGraph)
	}
	if out.Artifacts.ReportPath == "" {
		out.Artifacts.ReportPath = reportPath
	}
	return out
}
