package worker

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/loopkernel"
	"github.com/hanchaoqun/codrax/internal/types"
)

type ImpactAnalyzerInput struct {
	ID        string                      `json:"id,omitempty"`
	RunID     string                      `json:"run_id,omitempty"`
	UnitID    string                      `json:"unit_id,omitempty"`
	Goal      string                      `json:"goal,omitempty"`
	Analysis  *types.ImpactAnalysisResult `json:"analysis,omitempty"`
	InputRefs []loopkernel.ArtifactRef    `json:"input_refs,omitempty"`
	Budget    Budget                      `json:"budget,omitempty"`
}

type ImpactAnalyzerOutput struct {
	Request  Request                    `json:"request,omitempty"`
	Result   Result                     `json:"result,omitempty"`
	Analysis types.ImpactAnalysisResult `json:"analysis,omitempty"`
}

type PatchCriticInput struct {
	ID        string                   `json:"id,omitempty"`
	RunID     string                   `json:"run_id,omitempty"`
	UnitID    string                   `json:"unit_id,omitempty"`
	Goal      string                   `json:"goal,omitempty"`
	Review    *types.PatchReviewRecord `json:"review,omitempty"`
	InputRefs []loopkernel.ArtifactRef `json:"input_refs,omitempty"`
	Budget    Budget                   `json:"budget,omitempty"`
}

type PatchCriticOutput struct {
	Request Request                          `json:"request,omitempty"`
	Result  Result                           `json:"result,omitempty"`
	Review  types.PatchReviewRecord          `json:"review,omitempty"`
	Summary types.PatchReviewCoverageSummary `json:"summary,omitempty"`
}

type ProofAuditorInput struct {
	ID        string                          `json:"id,omitempty"`
	RunID     string                          `json:"run_id,omitempty"`
	UnitID    string                          `json:"unit_id,omitempty"`
	Goal      string                          `json:"goal,omitempty"`
	Plan      *types.ChangePlan               `json:"plan,omitempty"`
	Report    *types.ChangeReport             `json:"report,omitempty"`
	Profile   *types.VerificationProofProfile `json:"profile,omitempty"`
	Ledger    *types.VerificationProofLedger  `json:"ledger,omitempty"`
	InputRefs []loopkernel.ArtifactRef        `json:"input_refs,omitempty"`
	Budget    Budget                          `json:"budget,omitempty"`
}

type ProofAuditorOutput struct {
	Request   Request                               `json:"request,omitempty"`
	Result    Result                                `json:"result,omitempty"`
	Profile   types.VerificationProofProfile        `json:"profile,omitempty"`
	Ledger    types.VerificationProofLedger         `json:"ledger,omitempty"`
	Authority loopkernel.ProofCoverageAuthorityView `json:"authority,omitempty"`
}

type FailureAnalyzerInput struct {
	ID                 string                   `json:"id,omitempty"`
	RunID              string                   `json:"run_id,omitempty"`
	UnitID             string                   `json:"unit_id,omitempty"`
	Goal               string                   `json:"goal,omitempty"`
	BatchID            string                   `json:"batch_id,omitempty"`
	Attempt            int                      `json:"attempt,omitempty"`
	Report             *types.ChangeReport      `json:"report,omitempty"`
	DiffArtifactRef    string                   `json:"diff_artifact_ref,omitempty"`
	SurfaceArtifactRef string                   `json:"surface_artifact_ref,omitempty"`
	InputRefs          []loopkernel.ArtifactRef `json:"input_refs,omitempty"`
	Budget             Budget                   `json:"budget,omitempty"`
}

type FailureAnalyzerOutput struct {
	Request Request                     `json:"request,omitempty"`
	Result  Result                      `json:"result,omitempty"`
	Handoff *types.VerifyFailureHandoff `json:"handoff,omitempty"`
}

func RunImpactAnalyzer(in ImpactAnalyzerInput) ImpactAnalyzerOutput {
	analysis := types.ImpactAnalysisResult{}
	if in.Analysis != nil {
		analysis = types.NormalizeImpactAnalysisResult(*in.Analysis)
	}
	request := buildArtifactWorkerRequest(
		in.ID, in.RunID, in.UnitID, KindImpactAnalyzer,
		firstNonEmpty(in.Goal, "project impact analysis evidence"),
		impactAnalyzerPaths(analysis), in.InputRefs, in.Budget,
	)
	outputRefs := []loopkernel.ArtifactRef{localizerArtifactRef("impact_analysis_result", analysis)}
	result := Result{
		RequestID:    request.ID,
		RunID:        request.RunID,
		UnitID:       request.UnitID,
		Kind:         KindImpactAnalyzer,
		Status:       StatusComplete,
		ReasonCode:   impactAnalyzerReason(analysis),
		OutputRefs:   outputRefs,
		EvidenceRefs: impactAnalyzerEvidenceRefs(analysis),
	}
	return ImpactAnalyzerOutput{Request: request, Result: NormalizeResult(result), Analysis: analysis}
}

func RunPatchCritic(in PatchCriticInput) PatchCriticOutput {
	review := types.PatchReviewRecord{}
	if in.Review != nil {
		review = types.NormalizePatchReviewRecord(*in.Review)
	}
	summary := types.SummarizePatchReviewCoverage(review)
	request := buildArtifactWorkerRequest(
		in.ID, in.RunID, in.UnitID, KindPatchCritic,
		firstNonEmpty(in.Goal, "review actual patch evidence"),
		patchCriticPaths(review), in.InputRefs, in.Budget,
	)
	outputRefs := []loopkernel.ArtifactRef{
		localizerArtifactRef("patch_review_record", review),
		localizerArtifactRef("patch_review_coverage_summary", summary),
	}
	result := Result{
		RequestID:    request.ID,
		RunID:        request.RunID,
		UnitID:       request.UnitID,
		Kind:         KindPatchCritic,
		Status:       StatusComplete,
		ReasonCode:   patchCriticReason(summary),
		OutputRefs:   outputRefs,
		EvidenceRefs: patchCriticEvidenceRefs(review),
	}
	return PatchCriticOutput{Request: request, Result: NormalizeResult(result), Review: review, Summary: summary}
}

func RunProofAuditor(in ProofAuditorInput) ProofAuditorOutput {
	profile := types.VerificationProofProfile{}
	if in.Profile != nil {
		profile = types.NormalizeVerificationProofProfile(*in.Profile)
	} else {
		profile = types.BuildVerificationProofProfile(in.Plan, in.Report)
	}
	ledger := types.VerificationProofLedger{}
	if in.Ledger != nil {
		ledger = types.NormalizeVerificationProofLedger(*in.Ledger)
	} else {
		ledger = types.BuildVerificationProofLedger(in.Plan, in.Report, nil)
	}
	authority := loopkernel.DeriveProofCoverageAuthority(&profile, &ledger)
	request := buildArtifactWorkerRequest(
		in.ID, in.RunID, in.UnitID, KindProofAuditor,
		firstNonEmpty(in.Goal, "audit verification proof coverage"),
		proofAuditorPaths(in.Plan, ledger), in.InputRefs, in.Budget,
	)
	outputRefs := []loopkernel.ArtifactRef{
		localizerArtifactRef("verification_proof_profile", profile),
		localizerArtifactRef("verification_proof_ledger", ledger),
		localizerArtifactRef("proof_coverage_authority", authority),
	}
	result := Result{
		RequestID:    request.ID,
		RunID:        request.RunID,
		UnitID:       request.UnitID,
		Kind:         KindProofAuditor,
		Status:       StatusComplete,
		ReasonCode:   authority.ReasonCode,
		OutputRefs:   outputRefs,
		EvidenceRefs: proofAuditorEvidenceRefs(ledger),
	}
	return ProofAuditorOutput{Request: request, Result: NormalizeResult(result), Profile: profile, Ledger: ledger, Authority: authority}
}

func RunFailureAnalyzer(in FailureAnalyzerInput) FailureAnalyzerOutput {
	handoff := types.BuildVerifyFailureHandoff(in.Report, in.BatchID, in.Attempt, in.DiffArtifactRef, in.SurfaceArtifactRef)
	request := buildArtifactWorkerRequest(
		in.ID, in.RunID, in.UnitID, KindFailureAnalyzer,
		firstNonEmpty(in.Goal, "project verification failure evidence"),
		failureAnalyzerPaths(handoff), in.InputRefs, in.Budget,
	)
	reason := "failure_analysis_not_required"
	outputValue := struct {
		ReportPlanID string                      `json:"report_plan_id,omitempty"`
		Handoff      *types.VerifyFailureHandoff `json:"handoff,omitempty"`
	}{
		Handoff: handoff,
	}
	if in.Report != nil {
		outputValue.ReportPlanID = strings.TrimSpace(in.Report.PlanID)
	}
	if handoff != nil {
		reason = firstNonEmpty(handoff.FailureReasonCode, string(handoff.FailureKind), "verification_failed")
	}
	result := Result{
		RequestID:    request.ID,
		RunID:        request.RunID,
		UnitID:       request.UnitID,
		Kind:         KindFailureAnalyzer,
		Status:       StatusComplete,
		ReasonCode:   reason,
		OutputRefs:   []loopkernel.ArtifactRef{localizerArtifactRef("verify_failure_handoff", outputValue)},
		EvidenceRefs: failureAnalyzerEvidenceRefs(handoff),
	}
	return FailureAnalyzerOutput{Request: request, Result: NormalizeResult(result), Handoff: handoff}
}

func buildArtifactWorkerRequest(id, runID, unitID string, kind Kind, goal string, paths []string, inputRefs []loopkernel.ArtifactRef, budget Budget) Request {
	req := Request{
		ID:        strings.TrimSpace(id),
		RunID:     strings.TrimSpace(runID),
		UnitID:    strings.TrimSpace(unitID),
		Kind:      kind,
		Goal:      goal,
		Scope:     NormalizeScope(Scope{Paths: paths}),
		InputRefs: inputRefs,
		Budget:    budget,
	}
	if req.ID == "" {
		req.ID = string(kind) + "-" + shortHash(req)
	}
	return NormalizeRequest(req)
}

func impactAnalyzerPaths(analysis types.ImpactAnalysisResult) []string {
	var paths []string
	for _, surface := range analysis.ChangedSurfaces {
		paths = append(paths, surface.Path)
	}
	for _, edge := range analysis.Edges {
		paths = append(paths, edge.FromPath, edge.ToPath)
	}
	for _, target := range analysis.VerificationTargets {
		paths = append(paths, target.Path, target.RelatedPath)
	}
	return NormalizeScope(Scope{Paths: paths}).Paths
}

func impactAnalyzerEvidenceRefs(analysis types.ImpactAnalysisResult) []string {
	var refs []string
	for _, surface := range analysis.ChangedSurfaces {
		refs = append(refs, surface.EvidenceRef)
	}
	for _, edge := range analysis.Edges {
		refs = append(refs, edge.EvidenceRef)
	}
	for _, target := range analysis.VerificationTargets {
		refs = append(refs, target.EvidenceRef)
	}
	return normalizeStringSet(refs, strings.TrimSpace)
}

func impactAnalyzerReason(analysis types.ImpactAnalysisResult) string {
	if len(analysis.VerificationTargets) > 0 {
		return "impact_analysis_targets_projected"
	}
	if len(analysis.ChangedSurfaces) > 0 || len(analysis.Edges) > 0 {
		return "impact_analysis_surfaces_projected"
	}
	return "impact_analysis_empty"
}

func patchCriticPaths(review types.PatchReviewRecord) []string {
	paths := append(append([]string(nil), review.TargetPaths...), review.AllowedPaths...)
	paths = append(paths, review.AppliedPaths...)
	for _, finding := range review.Findings {
		paths = append(paths, finding.Path, finding.RelatedPath)
	}
	return NormalizeScope(Scope{Paths: paths}).Paths
}

func patchCriticEvidenceRefs(review types.PatchReviewRecord) []string {
	var refs []string
	for _, finding := range review.Findings {
		refs = append(refs, finding.EvidenceRef)
	}
	return normalizeStringSet(refs, strings.TrimSpace)
}

func patchCriticReason(summary types.PatchReviewCoverageSummary) string {
	if summary.HardBlock {
		return firstNonEmpty(summary.BlockReason, "patch_review_hard_block")
	}
	if summary.Verdict != "" {
		return "patch_review_" + string(summary.Verdict)
	}
	return "patch_review_clean"
}

func proofAuditorPaths(plan *types.ChangePlan, ledger types.VerificationProofLedger) []string {
	var paths []string
	if plan != nil {
		paths = append(paths, plan.TargetPaths...)
		for _, change := range plan.Changes {
			paths = append(paths, change.Path, change.NewPath)
		}
	}
	for _, item := range ledger.Obligations {
		paths = append(paths, item.Path, item.RelatedPath)
	}
	return NormalizeScope(Scope{Paths: paths}).Paths
}

func proofAuditorEvidenceRefs(ledger types.VerificationProofLedger) []string {
	var refs []string
	for _, item := range ledger.Obligations {
		refs = append(refs, item.EvidenceRef)
	}
	for _, item := range ledger.Capabilities {
		refs = append(refs, item.EvidenceRef)
	}
	return normalizeStringSet(refs, strings.TrimSpace)
}

func failureAnalyzerPaths(handoff *types.VerifyFailureHandoff) []string {
	if handoff == nil {
		return nil
	}
	var paths []string
	for _, err := range handoff.BuildErrors {
		paths = append(paths, err.File)
	}
	for _, snapshot := range handoff.RepairSourceSnapshots {
		paths = append(paths, snapshot.Path)
	}
	return NormalizeScope(Scope{Paths: paths}).Paths
}

func failureAnalyzerEvidenceRefs(handoff *types.VerifyFailureHandoff) []string {
	if handoff == nil {
		return nil
	}
	return normalizeStringSet([]string{
		handoff.BlobRef,
		handoff.DiffArtifactRef,
		handoff.SurfaceArtifactRef,
	}, strings.TrimSpace)
}
