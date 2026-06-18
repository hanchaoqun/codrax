package types

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const WriteFinalReportSchemaVersion = 1

// WriteFinalReport is the durable typed delivery summary for a write workflow.
// It is a projection from existing typed artifacts, not a new source of routing
// authority. Consumers must not infer hard state from terminal logs or model
// prose when this artifact is available.
type WriteFinalReport struct {
	SchemaVersion int             `json:"schema_version"`
	Kind          WriteOutputKind `json:"kind"`
	GeneratedAt   time.Time       `json:"generated_at,omitempty"`

	RunID         string                        `json:"run_id,omitempty"`
	RunStatus     WriteWorkflowRunStatus        `json:"run_status,omitempty"`
	Goal          string                        `json:"goal,omitempty"`
	Completion    *WriteWorkflowCompletion      `json:"completion,omitempty"`
	ActiveBatchID string                        `json:"active_batch_id,omitempty"`
	Batches       []WriteFinalBatchSummary      `json:"batches,omitempty"`
	Artifacts     WriteFinalArtifactRefs        `json:"artifacts,omitempty"`
	Plan          WriteFinalPlanSummary         `json:"plan,omitempty"`
	Patch         WriteFinalPatchSummary        `json:"patch,omitempty"`
	Verification  WriteFinalVerificationSummary `json:"verification,omitempty"`
	Proof         VerificationProofProfile      `json:"proof,omitempty"`
	Delivery      WriteFinalDeliverySummary     `json:"delivery,omitempty"`
	PatchReview   WriteFinalPatchReviewSummary  `json:"patch_review,omitempty"`
	Impact        WriteFinalImpactSummary       `json:"impact,omitempty"`
	Handoff       WriteFinalHandoffSummary      `json:"handoff,omitempty"`
	ResidualRisks []WriteFinalResidualRisk      `json:"residual_risks,omitempty"`
}

type WriteFinalArtifactRefs struct {
	PlanPath     string `json:"plan_path,omitempty"`
	ReportPath   string `json:"report_path,omitempty"`
	WorkflowPath string `json:"workflow_path,omitempty"`
}

type WriteFinalBatchSummary struct {
	ID                  string                   `json:"id,omitempty"`
	Status              WriteWorkflowBatchStatus `json:"status,omitempty"`
	PlanID              string                   `json:"plan_id,omitempty"`
	ApplyRef            string                   `json:"apply_ref,omitempty"`
	VerifyRef           string                   `json:"verify_ref,omitempty"`
	Completion          *WriteWorkflowCompletion `json:"completion,omitempty"`
	LatestAttemptKind   string                   `json:"latest_attempt_kind,omitempty"`
	LatestAttemptStatus string                   `json:"latest_attempt_status,omitempty"`
	LatestReasonCode    string                   `json:"latest_reason_code,omitempty"`
	ActiveSliceID       string                   `json:"active_slice_id,omitempty"`
	ActiveSliceStatus   ChangePlanSliceStatus    `json:"active_slice_status,omitempty"`
}

type WriteFinalPlanSummary struct {
	ID                 string                    `json:"id,omitempty"`
	Status             string                    `json:"status,omitempty"`
	PlanFingerprint    string                    `json:"plan_fingerprint,omitempty"`
	TargetPaths        []string                  `json:"target_paths,omitempty"`
	AppliedPaths       []string                  `json:"applied_paths,omitempty"`
	SourcePaths        []string                  `json:"source_paths,omitempty"`
	TestPaths          []string                  `json:"test_paths,omitempty"`
	ApprovalAction     string                    `json:"approval_action,omitempty"`
	ApprovalRiskLevel  string                    `json:"approval_risk_level,omitempty"`
	ApprovalReasonCode string                    `json:"approval_reason_code,omitempty"`
	AppliedCommitSHA   string                    `json:"applied_commit_sha,omitempty"`
	Localization       *SourceLocalizationReview `json:"localization,omitempty"`
	OwnerAnchors       []OwnerAnchorViewItem     `json:"owner_anchors,omitempty"`
	OwnerAnchorGaps    []WriteFinalOwnerGap      `json:"owner_anchor_gaps,omitempty"`
}

// WriteFinalOwnerGap is a typed final-report audit row for a production source
// path that the plan edited after only file/scope localization, without a
// strong owner/evidence anchor in the prior context. It is advisory confidence
// metadata, not an apply/verify gate.
type WriteFinalOwnerGap struct {
	Path             string `json:"path,omitempty"`
	ReasonCode       string `json:"reason_code,omitempty"`
	RequiredEvidence string `json:"required_evidence,omitempty"`
	Source           string `json:"source,omitempty"`
}

type WriteFinalPatchSummary struct {
	PatchEffectID    string   `json:"patch_effect_id,omitempty"`
	DiffFingerprint  string   `json:"diff_fingerprint,omitempty"`
	DiffBytes        int      `json:"diff_bytes,omitempty"`
	ChangedFiles     []string `json:"changed_files,omitempty"`
	SourceFiles      []string `json:"source_files,omitempty"`
	TestFiles        []string `json:"test_files,omitempty"`
	EffectEventCodes []string `json:"effect_event_codes,omitempty"`
}

type WriteFinalVerificationSummary struct {
	ReportID              string             `json:"report_id,omitempty"`
	Status                VerificationStatus `json:"status,omitempty"`
	Passed                bool               `json:"passed,omitempty"`
	FailureKind           FailureKind        `json:"failure_kind,omitempty"`
	FailureReasonCode     string             `json:"failure_reason_code,omitempty"`
	TestCount             int                `json:"test_count,omitempty"`
	PassedCount           int                `json:"passed_count,omitempty"`
	FailedCount           int                `json:"failed_count,omitempty"`
	CommandCount          int                `json:"command_count,omitempty"`
	ConfidenceReasonCodes []string           `json:"confidence_reason_codes,omitempty"`
	NoTestsRunners        []string           `json:"no_tests_runners,omitempty"`
}

type WriteFinalDeliverySummary struct {
	Status                  string   `json:"status,omitempty"`
	Relation                string   `json:"relation,omitempty"`
	ReasonCode              string   `json:"reason_code,omitempty"`
	FinalPlanID             string   `json:"final_plan_id,omitempty"`
	ReportPlanID            string   `json:"report_plan_id,omitempty"`
	PrimarySourcePlanID     string   `json:"primary_source_plan_id,omitempty"`
	SourceOwnerPlanIDs      []string `json:"source_owner_plan_ids,omitempty"`
	SourcePaths             []string `json:"source_paths,omitempty"`
	TestPaths               []string `json:"test_paths,omitempty"`
	FinalPlanTestOnly       bool     `json:"final_plan_test_only,omitempty"`
	FinalPlanValidationOnly bool     `json:"final_plan_validation_only,omitempty"`
}

type WriteFinalPatchReviewSummary struct {
	Status          string                     `json:"status,omitempty"`
	Verdict         PatchReviewCoverageVerdict `json:"verdict,omitempty"`
	HardBlock       bool                       `json:"hard_block,omitempty"`
	ReasonCodes     []string                   `json:"reason_codes,omitempty"`
	BlockReason     string                     `json:"block_reason,omitempty"`
	TotalFindings   int                        `json:"total_findings,omitempty"`
	UnverifiedKinds []string                   `json:"unverified_kinds,omitempty"`
}

type WriteFinalImpactSummary struct {
	ResultID            string                     `json:"result_id,omitempty"`
	ChangedSurfaceCount int                        `json:"changed_surface_count,omitempty"`
	VerificationTargets int                        `json:"verification_targets,omitempty"`
	CoverageCounts      map[string]int             `json:"coverage_counts,omitempty"`
	UncoveredTargets    []ImpactVerificationTarget `json:"uncovered_targets,omitempty"`
}

type WriteFinalHandoffSummary struct {
	TotalItems   int                     `json:"total_items,omitempty"`
	TopItems     []WriteFinalHandoffItem `json:"top_items,omitempty"`
	OwnerAnchors []OwnerAnchorViewItem   `json:"owner_anchors,omitempty"`
}

type WriteFinalHandoffItem struct {
	Priority    WriteContextPriority `json:"priority,omitempty"`
	Kind        string               `json:"kind,omitempty"`
	SourceStage string               `json:"source_stage,omitempty"`
	SourceID    string               `json:"source_id,omitempty"`
	EvidenceRef string               `json:"evidence_ref,omitempty"`
	Fingerprint string               `json:"fingerprint,omitempty"`
}

type WriteFinalResidualRisk struct {
	Code     string `json:"code"`
	Source   string `json:"source,omitempty"`
	Severity string `json:"severity,omitempty"`
	Detail   string `json:"detail,omitempty"`
}

type WriteFinalReportInput struct {
	Run            *WriteWorkflowRun
	Plan           *ChangePlan
	Report         *ChangeReport
	ProofArtifacts []VerificationProofArtifact
	Delivery       WriteFinalDeliverySummary
	PlanPath       string
	ReportPath     string
	WorkflowPath   string
	GeneratedAt    time.Time
}

func BuildWriteFinalReport(input WriteFinalReportInput) WriteFinalReport {
	generatedAt := input.GeneratedAt
	if generatedAt.IsZero() {
		generatedAt = time.Now()
	}
	out := WriteFinalReport{
		SchemaVersion: WriteFinalReportSchemaVersion,
		Kind:          WriteOutputFinalReport,
		GeneratedAt:   generatedAt,
		Artifacts: WriteFinalArtifactRefs{
			PlanPath:     strings.TrimSpace(input.PlanPath),
			ReportPath:   strings.TrimSpace(input.ReportPath),
			WorkflowPath: strings.TrimSpace(input.WorkflowPath),
		},
	}
	var runContextPacks []WriteContextPack
	if input.Run != nil {
		run := NormalizeWriteWorkflowRun(*input.Run)
		out.RunID = strings.TrimSpace(run.RunID)
		out.RunStatus = run.Status
		out.Goal = strings.TrimSpace(run.Goal)
		out.ActiveBatchID = strings.TrimSpace(run.ActiveBatchID)
		if run.Completion != nil {
			completion := *run.Completion
			out.Completion = &completion
		}
		out.Batches = writeFinalBatchSummaries(run)
		out.Handoff = writeFinalHandoffSummary(run.ContextPacks, 16)
		runContextPacks = run.ContextPacks
	}
	if input.Plan != nil {
		out.Plan = writeFinalPlanSummary(input.Plan, runContextPacks)
		out.Patch = writeFinalPatchSummary(input.Plan.PatchEffect)
		out.PatchReview = writeFinalPatchReviewSummary(input.Plan.PatchReview)
		out.Impact = writeFinalImpactSummary(input.Plan.ImpactAnalysis)
	}
	if input.Report != nil {
		out.Verification = writeFinalVerificationSummary(input.Report)
	}
	out.Proof = BuildCumulativeVerificationProofProfile(input.Plan, input.Report, input.ProofArtifacts)
	out.Delivery = NormalizeWriteFinalDeliverySummary(input.Delivery)
	out.ResidualRisks = writeFinalResidualRisks(out)
	return NormalizeWriteFinalReport(out)
}

func NormalizeWriteFinalReport(in WriteFinalReport) WriteFinalReport {
	if in.SchemaVersion == 0 {
		in.SchemaVersion = WriteFinalReportSchemaVersion
	}
	if in.Kind == "" {
		in.Kind = WriteOutputFinalReport
	}
	in.RunID = strings.TrimSpace(in.RunID)
	in.Goal = strings.TrimSpace(in.Goal)
	in.ActiveBatchID = strings.TrimSpace(in.ActiveBatchID)
	in.Artifacts.PlanPath = strings.TrimSpace(in.Artifacts.PlanPath)
	in.Artifacts.ReportPath = strings.TrimSpace(in.Artifacts.ReportPath)
	in.Artifacts.WorkflowPath = strings.TrimSpace(in.Artifacts.WorkflowPath)
	in.Plan.TargetPaths = dedupTrimWriteWorkflowRunStrings(in.Plan.TargetPaths)
	in.Plan.AppliedPaths = dedupTrimWriteWorkflowRunStrings(in.Plan.AppliedPaths)
	in.Plan.SourcePaths = dedupTrimWriteWorkflowRunStrings(in.Plan.SourcePaths)
	in.Plan.TestPaths = dedupTrimWriteWorkflowRunStrings(in.Plan.TestPaths)
	in.Plan.Localization = CloneSourceLocalizationReviewPtr(in.Plan.Localization)
	in.Plan.OwnerAnchors = NormalizeOwnerAnchorView(OwnerAnchorView{Items: in.Plan.OwnerAnchors}, 12).Items
	in.Plan.OwnerAnchorGaps = normalizeWriteFinalOwnerGaps(in.Plan.OwnerAnchorGaps)
	in.Patch.ChangedFiles = dedupTrimWriteWorkflowRunStrings(in.Patch.ChangedFiles)
	in.Patch.SourceFiles = dedupTrimWriteWorkflowRunStrings(in.Patch.SourceFiles)
	in.Patch.TestFiles = dedupTrimWriteWorkflowRunStrings(in.Patch.TestFiles)
	in.Patch.EffectEventCodes = dedupTrimWriteWorkflowRunStrings(in.Patch.EffectEventCodes)
	in.Verification.ConfidenceReasonCodes = dedupTrimWriteWorkflowRunStrings(in.Verification.ConfidenceReasonCodes)
	in.Verification.NoTestsRunners = dedupTrimWriteWorkflowRunStrings(in.Verification.NoTestsRunners)
	in.Proof = NormalizeVerificationProofProfile(in.Proof)
	in.Delivery = NormalizeWriteFinalDeliverySummary(in.Delivery)
	in.PatchReview.ReasonCodes = dedupTrimWriteWorkflowRunStrings(in.PatchReview.ReasonCodes)
	in.PatchReview.UnverifiedKinds = dedupTrimWriteWorkflowRunStrings(in.PatchReview.UnverifiedKinds)
	in.Impact.UncoveredTargets = writeFinalBoundedImpactTargets(in.Impact.UncoveredTargets, 12)
	in.Handoff.OwnerAnchors = NormalizeOwnerAnchorView(OwnerAnchorView{Items: in.Handoff.OwnerAnchors}, 12).Items
	in.ResidualRisks = writeFinalNormalizeRisks(in.ResidualRisks)
	return in
}

func LoadWriteFinalReportFromFile(path string) (*WriteFinalReport, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var report WriteFinalReport
	if err := json.Unmarshal(data, &report); err != nil {
		return nil, fmt.Errorf("parse write final report %s: %w", path, err)
	}
	report = NormalizeWriteFinalReport(report)
	return &report, nil
}

func WriteFinalReportToFile(report *WriteFinalReport, path string) error {
	if report == nil {
		return fmt.Errorf("WriteFinalReportToFile: nil report")
	}
	path = strings.TrimSpace(path)
	if path == "" {
		return fmt.Errorf("WriteFinalReportToFile: empty path")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("WriteFinalReportToFile: mkdir %s: %w", filepath.Dir(path), err)
	}
	normalized := NormalizeWriteFinalReport(*report)
	data, err := json.MarshalIndent(normalized, "", "  ")
	if err != nil {
		return fmt.Errorf("WriteFinalReportToFile: marshal: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("WriteFinalReportToFile: write %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("WriteFinalReportToFile: rename %s -> %s: %w", tmp, path, err)
	}
	return nil
}

func NormalizeWriteFinalDeliverySummary(in WriteFinalDeliverySummary) WriteFinalDeliverySummary {
	in.Status = strings.TrimSpace(in.Status)
	in.Relation = strings.TrimSpace(in.Relation)
	in.ReasonCode = strings.TrimSpace(in.ReasonCode)
	in.FinalPlanID = strings.TrimSpace(in.FinalPlanID)
	in.ReportPlanID = strings.TrimSpace(in.ReportPlanID)
	in.PrimarySourcePlanID = strings.TrimSpace(in.PrimarySourcePlanID)
	in.SourceOwnerPlanIDs = dedupTrimWriteWorkflowRunStrings(in.SourceOwnerPlanIDs)
	in.SourcePaths = dedupTrimWriteWorkflowRunStrings(in.SourcePaths)
	in.TestPaths = dedupTrimWriteWorkflowRunStrings(in.TestPaths)
	return in
}

func writeFinalBatchSummaries(run WriteWorkflowRun) []WriteFinalBatchSummary {
	out := make([]WriteFinalBatchSummary, 0, len(run.Batches))
	for _, batch := range run.Batches {
		item := WriteFinalBatchSummary{
			ID:                strings.TrimSpace(batch.ID),
			Status:            batch.Status,
			PlanID:            strings.TrimSpace(batch.PlanID),
			ApplyRef:          strings.TrimSpace(batch.ApplyRef),
			VerifyRef:         strings.TrimSpace(batch.VerifyRef),
			ActiveSliceID:     strings.TrimSpace(batch.ActiveSliceID),
			ActiveSliceStatus: batchActiveSliceStatus(batch),
		}
		if batch.Completion != nil {
			completion := *batch.Completion
			item.Completion = &completion
		}
		if len(batch.Attempts) > 0 {
			attempt := batch.Attempts[len(batch.Attempts)-1]
			item.LatestAttemptKind = strings.TrimSpace(attempt.Kind)
			item.LatestAttemptStatus = strings.TrimSpace(attempt.Status)
			item.LatestReasonCode = strings.TrimSpace(attempt.ReasonCode)
		}
		out = append(out, item)
	}
	return out
}

func batchActiveSliceStatus(batch WriteWorkflowBatch) ChangePlanSliceStatus {
	active := strings.TrimSpace(batch.ActiveSliceID)
	if active == "" {
		return ""
	}
	for _, slice := range batch.Slices {
		if strings.TrimSpace(slice.ID) == active {
			return slice.Status
		}
	}
	return ""
}

func writeFinalPlanSummary(plan *ChangePlan, prior []WriteContextPack) WriteFinalPlanSummary {
	out := WriteFinalPlanSummary{
		ID:               strings.TrimSpace(plan.ID),
		Status:           strings.TrimSpace(plan.Status),
		PlanFingerprint:  PlanFingerprint(plan),
		TargetPaths:      append([]string(nil), plan.TargetPaths...),
		AppliedPaths:     append([]string(nil), plan.AppliedPaths...),
		AppliedCommitSHA: strings.TrimSpace(plan.AppliedCommitSHA),
	}
	for _, path := range plan.TargetPaths {
		if writeFinalLooksLikeTestPath(path) {
			out.TestPaths = append(out.TestPaths, path)
		} else {
			out.SourcePaths = append(out.SourcePaths, path)
		}
	}
	if plan.Approval != nil {
		out.ApprovalAction = strings.TrimSpace(plan.Approval.Action)
		out.ApprovalRiskLevel = strings.TrimSpace(plan.Approval.RiskLevel)
		out.ApprovalReasonCode = strings.TrimSpace(plan.Approval.ReasonCode)
	}
	out.Localization = CloneSourceLocalizationReviewPtr(plan.LocalizationReview)
	out.OwnerAnchors = OwnerAnchorViewFromChangePlan(plan, 12).Items
	out.OwnerAnchorGaps = writeFinalOwnerGaps(prior, plan)
	return out
}

func writeFinalPatchSummary(effect *PatchEffectRecord) WriteFinalPatchSummary {
	if effect == nil {
		return WriteFinalPatchSummary{}
	}
	out := WriteFinalPatchSummary{
		PatchEffectID:   strings.TrimSpace(effect.RecordID),
		DiffFingerprint: strings.TrimSpace(effect.DiffFingerprint),
		DiffBytes:       effect.DiffBytes,
	}
	for _, file := range effect.Files {
		path := strings.TrimSpace(file.Path)
		if path == "" {
			path = strings.TrimSpace(file.OldPath)
		}
		if path != "" {
			out.ChangedFiles = append(out.ChangedFiles, path)
			if file.PathRole == SourcePathRoleTest || writeFinalLooksLikeTestPath(path) {
				out.TestFiles = append(out.TestFiles, path)
			} else {
				out.SourceFiles = append(out.SourceFiles, path)
			}
		}
		for _, event := range file.Events {
			if code := strings.TrimSpace(event.Code); code != "" {
				out.EffectEventCodes = append(out.EffectEventCodes, code)
			}
		}
	}
	return out
}

func writeFinalVerificationSummary(report *ChangeReport) WriteFinalVerificationSummary {
	if report == nil {
		return WriteFinalVerificationSummary{}
	}
	status := report.NormalizeVerificationStatus()
	passed, total := report.Score()
	failed := 0
	if total >= 0 && passed >= 0 {
		failed = total - passed
	}
	reportID := ""
	if planID := strings.TrimSpace(report.PlanID); planID != "" {
		reportID = planID + ".report.json"
	}
	out := WriteFinalVerificationSummary{
		ReportID:          reportID,
		Status:            status,
		Passed:            status == VerificationStatusPassed,
		FailureKind:       report.FailureKind,
		FailureReasonCode: strings.TrimSpace(report.FailureReasonCode),
		TestCount:         total,
		PassedCount:       passed,
		FailedCount:       failed,
		CommandCount:      len(report.ExecutedCommands),
		NoTestsRunners:    append([]string(nil), report.NoTestsRunners...),
	}
	for _, rec := range report.VerificationConfidence {
		if code := strings.TrimSpace(rec.ReasonCode); code != "" {
			out.ConfidenceReasonCodes = append(out.ConfidenceReasonCodes, code)
		}
	}
	return out
}

func writeFinalPatchReviewSummary(review *PatchReviewRecord) WriteFinalPatchReviewSummary {
	if review == nil {
		return WriteFinalPatchReviewSummary{}
	}
	normalized := NormalizePatchReviewRecord(*review)
	coverage := SummarizePatchReviewCoverage(normalized)
	out := WriteFinalPatchReviewSummary{
		Status:        strings.TrimSpace(normalized.Status),
		Verdict:       coverage.Verdict,
		HardBlock:     normalized.HardBlock || coverage.HardBlock,
		ReasonCodes:   append([]string(nil), coverage.ReasonCodes...),
		BlockReason:   strings.TrimSpace(coverage.BlockReason),
		TotalFindings: coverage.TotalFindings,
	}
	for _, item := range coverage.ImpactKindCoverage {
		if item.Unverified > 0 || item.Unavailable > 0 || item.Unknown > 0 {
			out.UnverifiedKinds = append(out.UnverifiedKinds, item.Kind)
		}
	}
	return out
}

func writeFinalImpactSummary(analysis *ImpactAnalysisResult) WriteFinalImpactSummary {
	if analysis == nil {
		return WriteFinalImpactSummary{}
	}
	normalized := NormalizeImpactAnalysisResult(*analysis)
	out := WriteFinalImpactSummary{
		ResultID:            strings.TrimSpace(normalized.ResultID),
		ChangedSurfaceCount: len(normalized.ChangedSurfaces),
		VerificationTargets: len(normalized.VerificationTargets),
		CoverageCounts:      map[string]int{},
	}
	for _, target := range normalized.VerificationTargets {
		status := strings.TrimSpace(target.CoverageStatus)
		if status == "" {
			status = "unknown"
		}
		out.CoverageCounts[status]++
		if status != "verified" {
			out.UncoveredTargets = append(out.UncoveredTargets, target)
		}
	}
	if len(out.CoverageCounts) == 0 {
		out.CoverageCounts = nil
	}
	return out
}

func writeFinalHandoffSummary(packs []WriteContextPack, limit int) WriteFinalHandoffSummary {
	var merged WriteContextPack
	for _, pack := range packs {
		merged.Items = append(merged.Items, NormalizeWriteContextPack(pack).Items...)
	}
	merged = NormalizeWriteContextPack(merged)
	view := merged.View(WriteConsumerController, limit)
	out := WriteFinalHandoffSummary{
		TotalItems:   len(merged.Items),
		OwnerAnchors: OwnerAnchorViewFromWriteContextPack(merged, WriteConsumerController, "", "", 12).Items,
	}
	for _, item := range view.Items {
		out.TopItems = append(out.TopItems, WriteFinalHandoffItem{
			Priority:    item.Priority,
			Kind:        strings.TrimSpace(item.Kind),
			SourceStage: strings.TrimSpace(item.SourceStage),
			SourceID:    strings.TrimSpace(item.SourceID),
			Fingerprint: strings.TrimSpace(item.Fingerprint),
			EvidenceRef: writeFinalEvidenceRefID(item.EvidenceRef),
		})
	}
	return out
}

func writeFinalResidualRisks(report WriteFinalReport) []WriteFinalResidualRisk {
	var risks []WriteFinalResidualRisk
	add := func(code, source, severity, detail string) {
		code = strings.TrimSpace(code)
		if code == "" {
			return
		}
		risks = append(risks, WriteFinalResidualRisk{
			Code:     code,
			Source:   strings.TrimSpace(source),
			Severity: strings.TrimSpace(severity),
			Detail:   strings.TrimSpace(detail),
		})
	}
	if report.Completion != nil && report.Completion.Verdict != WriteWorkflowCompletionVerified {
		add(string(report.Completion.Verdict), "workflow_completion", "warning", report.Completion.ReasonCode)
	}
	if report.RunStatus != "" && report.RunStatus != WriteWorkflowRunComplete && report.RunStatus != WriteWorkflowRunBlocked {
		add("workflow_nonterminal", "workflow_run", "warning", string(report.RunStatus))
	}
	if report.Verification.Status == VerificationStatusUnavailable {
		add("verification_unavailable", "change_report", "warning", report.Verification.FailureReasonCode)
	} else if report.Verification.Status == VerificationStatusFailed {
		add("verification_failed", "change_report", "error", string(report.Verification.FailureKind))
	}
	switch report.Proof.Status {
	case VerificationProofFailed:
		add("verification_proof_failed", "verification_proof", "error", strings.Join(report.Proof.ReasonCodes, ","))
	case VerificationProofUnavailable:
		add("verification_proof_unavailable", "verification_proof", "warning", strings.Join(report.Proof.ReasonCodes, ","))
	case VerificationProofWeak:
		add("verification_proof_weak", "verification_proof", "warning", strings.Join(report.Proof.ReasonCodes, ","))
	}
	if report.PatchReview.HardBlock {
		add("patch_review_hard_block", "patch_review", "error", report.PatchReview.BlockReason)
	}
	if report.PatchReview.Verdict == PatchReviewCoverageVerdictUnverified {
		add("patch_review_semantic_unverified", "patch_review", "warning", strings.Join(report.PatchReview.UnverifiedKinds, ","))
	}
	if report.Plan.Localization != nil {
		localization := NormalizeSourceLocalizationReview(*report.Plan.Localization)
		switch localization.Status {
		case SourceLocalizationMissing:
			add("source_localization_missing", "source_localization", "warning", strings.Join(localization.MissingPaths, ","))
		case SourceLocalizationWeak:
			add("source_localization_weak", "source_localization", "warning", strings.Join(localization.ReasonCodes, ","))
		}
	}
	if len(report.Plan.OwnerAnchorGaps) > 0 {
		add("source_owner_anchor_missing", "source_localization", "warning", strings.Join(writeFinalOwnerGapPaths(report.Plan.OwnerAnchorGaps), ","))
	}
	for status, count := range report.Impact.CoverageCounts {
		if count <= 0 || status == "verified" {
			continue
		}
		add("impact_"+status, "impact_analysis", "warning", fmt.Sprintf("%d target(s)", count))
	}
	return risks
}

func writeFinalNormalizeRisks(in []WriteFinalResidualRisk) []WriteFinalResidualRisk {
	seen := map[string]int{}
	var out []WriteFinalResidualRisk
	for _, risk := range in {
		risk.Code = strings.TrimSpace(risk.Code)
		risk.Source = strings.TrimSpace(risk.Source)
		risk.Severity = strings.TrimSpace(risk.Severity)
		risk.Detail = strings.TrimSpace(risk.Detail)
		if risk.Code == "" {
			continue
		}
		key := risk.Code + "|" + risk.Source + "|" + risk.Detail
		if idx, ok := seen[key]; ok {
			if out[idx].Severity == "" {
				out[idx].Severity = risk.Severity
			}
			continue
		}
		seen[key] = len(out)
		out = append(out, risk)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Severity != out[j].Severity {
			return writeFinalSeverityRank(out[i].Severity) < writeFinalSeverityRank(out[j].Severity)
		}
		if out[i].Source != out[j].Source {
			return out[i].Source < out[j].Source
		}
		return out[i].Code < out[j].Code
	})
	return out
}

func writeFinalOwnerGaps(prior []WriteContextPack, plan *ChangePlan) []WriteFinalOwnerGap {
	paths := WritePlanSourcePathsWithoutOwnerAnchor(prior, plan)
	if len(paths) == 0 {
		return nil
	}
	out := make([]WriteFinalOwnerGap, 0, len(paths))
	for _, path := range paths {
		out = append(out, WriteFinalOwnerGap{
			Path:             path,
			ReasonCode:       "plan_source_path_without_owner_anchor",
			RequiredEvidence: "typed_owner_or_supporting_localization_anchor",
			Source:           "prior_context",
		})
	}
	return normalizeWriteFinalOwnerGaps(out)
}

func normalizeWriteFinalOwnerGaps(in []WriteFinalOwnerGap) []WriteFinalOwnerGap {
	seen := map[string]int{}
	var out []WriteFinalOwnerGap
	for _, gap := range in {
		gap.Path = strings.TrimSpace(filepath.ToSlash(gap.Path))
		gap.ReasonCode = strings.TrimSpace(gap.ReasonCode)
		gap.RequiredEvidence = strings.TrimSpace(gap.RequiredEvidence)
		gap.Source = strings.TrimSpace(gap.Source)
		if gap.Path == "" {
			continue
		}
		if gap.ReasonCode == "" {
			gap.ReasonCode = "plan_source_path_without_owner_anchor"
		}
		if gap.RequiredEvidence == "" {
			gap.RequiredEvidence = "typed_owner_or_supporting_localization_anchor"
		}
		if gap.Source == "" {
			gap.Source = "prior_context"
		}
		key := gap.Path + "|" + gap.ReasonCode + "|" + gap.Source
		if idx, ok := seen[key]; ok {
			if out[idx].RequiredEvidence == "" {
				out[idx].RequiredEvidence = gap.RequiredEvidence
			}
			continue
		}
		seen[key] = len(out)
		out = append(out, gap)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Path != out[j].Path {
			return out[i].Path < out[j].Path
		}
		if out[i].ReasonCode != out[j].ReasonCode {
			return out[i].ReasonCode < out[j].ReasonCode
		}
		return out[i].Source < out[j].Source
	})
	return out
}

func writeFinalOwnerGapPaths(gaps []WriteFinalOwnerGap) []string {
	var paths []string
	for _, gap := range normalizeWriteFinalOwnerGaps(gaps) {
		paths = append(paths, gap.Path)
	}
	return paths
}

func writeFinalSeverityRank(severity string) int {
	switch strings.TrimSpace(severity) {
	case "error":
		return 0
	case "warning":
		return 1
	case "info":
		return 2
	default:
		return 3
	}
}

func writeFinalBoundedImpactTargets(in []ImpactVerificationTarget, limit int) []ImpactVerificationTarget {
	if limit <= 0 || len(in) <= limit {
		return in
	}
	sort.SliceStable(in, func(i, j int) bool {
		if in[i].Priority != in[j].Priority {
			return in[i].Priority < in[j].Priority
		}
		if in[i].Kind != in[j].Kind {
			return in[i].Kind < in[j].Kind
		}
		return in[i].Path < in[j].Path
	})
	return append([]ImpactVerificationTarget(nil), in[:limit]...)
}

func writeFinalEvidenceRefID(ref *WriteExplorationEvidenceRef) string {
	if ref == nil {
		return ""
	}
	for _, value := range []string{ref.ID, ref.Source, ref.OwnerSymbol, ref.Subject, ref.AnchorSymbol} {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func writeFinalLooksLikeTestPath(path string) bool {
	path = strings.ToLower(strings.TrimSpace(filepath.ToSlash(path)))
	if path == "" {
		return false
	}
	parts := strings.Split(path, "/")
	base := parts[len(parts)-1]
	for _, part := range parts {
		switch part {
		case "test", "tests", "testing", "__tests__", "spec", "specs":
			return true
		}
	}
	return strings.HasPrefix(base, "test_") ||
		strings.Contains(base, "_test.") ||
		strings.Contains(base, ".test.") ||
		strings.Contains(base, ".spec.")
}
