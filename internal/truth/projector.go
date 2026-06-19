package truth

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/loopkernel"
	"github.com/hanchaoqun/codrax/internal/types"
	"github.com/hanchaoqun/codrax/internal/writeflow"
)

type ProjectInput struct {
	View   writeflow.WorkflowExecutionView
	Plan   *types.ChangePlan
	Report *types.ChangeReport
}

func Project(in ProjectInput) types.TruthLedger {
	var signals []types.TruthSignal
	if signal, ok := observationSignal(in.View.Observation); ok {
		signals = append(signals, signal)
	}
	if signal, ok := proofSignal(in.View.Proof); ok {
		signals = append(signals, signal)
	}
	if signal, ok := localizationSignal(in.View.Localization); ok {
		signals = append(signals, signal)
	}
	if signal, ok := navigationSignal(in.View.Navigation); ok {
		signals = append(signals, signal)
	}
	if in.Plan != nil {
		if signal, ok := patchReviewSignal(in.Plan.PatchReview); ok {
			signals = append(signals, signal)
		}
		if signal, ok := impactSignal(in.Plan.ImpactAnalysis); ok {
			signals = append(signals, signal)
		}
	}
	if in.Report != nil && in.View.Observation.State == "" {
		obs := writeflow.DeriveObservationAuthorityFromReport(in.Report, nil)
		if signal, ok := observationSignal(obs); ok {
			signals = append(signals, signal)
		}
	}
	ledger := types.TruthLedger{Signals: signals}
	ledger = types.NormalizeTruthLedger(ledger)
	ledger = applyPrecedence(ledger)
	return types.NormalizeTruthLedger(ledger)
}

func ProjectFromWorkflow(mode types.PipelineMode, run types.WriteWorkflowRun, plan *types.ChangePlan, report *types.ChangeReport) types.TruthLedger {
	view := writeflow.DeriveWorkflowExecutionViewWithReport(mode, run, plan, report)
	return Project(ProjectInput{View: view, Plan: plan, Report: report})
}

func applyPrecedence(ledger types.TruthLedger) types.TruthLedger {
	for _, signal := range ledger.Signals {
		if signal.Kind == types.TruthSignalRuntimeObservation && signal.State == types.TruthLedgerFailed {
			ledger.State = types.TruthLedgerFailed
			ledger.ReasonCode = firstNonEmpty(signal.ReasonCode, "runtime_observation_failed")
			ledger.RecommendedAction = types.TruthActionRepair
			return ledger
		}
	}
	for _, signal := range ledger.Signals {
		if signal.HardBlock || signal.State == types.TruthLedgerFailed {
			ledger.State = types.TruthLedgerFailed
			ledger.ReasonCode = firstNonEmpty(signal.ReasonCode, "truth_failed")
			ledger.RecommendedAction = types.TruthActionRepair
			return ledger
		}
	}
	for _, signal := range ledger.Signals {
		if signal.Kind == types.TruthSignalLocalization && signal.State == types.TruthLedgerWeak {
			ledger.State = types.TruthLedgerWeak
			ledger.ReasonCode = firstNonEmpty(signal.ReasonCode, "localization_requires_context")
			ledger.RecommendedAction = types.TruthActionLocalize
			return ledger
		}
	}
	for _, signal := range ledger.Signals {
		if signal.State == types.TruthLedgerWeak {
			ledger.State = types.TruthLedgerWeak
			ledger.ReasonCode = firstNonEmpty(signal.ReasonCode, "truth_weak")
			ledger.RecommendedAction = firstNonAction(signal.RecommendedAction, types.TruthActionAddProof)
			return ledger
		}
	}
	for _, signal := range ledger.Signals {
		if signal.State == types.TruthLedgerUnverified {
			ledger.State = types.TruthLedgerUnverified
			ledger.ReasonCode = firstNonEmpty(signal.ReasonCode, "truth_unverified")
			ledger.RecommendedAction = types.TruthActionContinue
			return ledger
		}
	}
	if len(ledger.Signals) > 0 {
		ledger.State = types.TruthLedgerCovered
		ledger.ReasonCode = "truth_covered"
		ledger.RecommendedAction = types.TruthActionContinue
	}
	return ledger
}

func observationSignal(obs writeflow.ObservationAuthorityView) (types.TruthSignal, bool) {
	state := types.TruthLedgerUnknown
	action := types.TruthActionVerify
	switch obs.State {
	case writeflow.ObservationAuthorityVerified:
		state = types.TruthLedgerCovered
		action = types.TruthActionContinue
	case writeflow.ObservationAuthorityFailed:
		state = types.TruthLedgerFailed
		action = types.TruthActionRepair
	case writeflow.ObservationAuthorityUnverified:
		state = types.TruthLedgerUnverified
		action = types.TruthActionContinue
	case writeflow.ObservationAuthorityMissing:
		state = types.TruthLedgerWeak
		action = types.TruthActionVerify
	default:
		return types.TruthSignal{}, false
	}
	return types.TruthSignal{
		Kind:              types.TruthSignalRuntimeObservation,
		State:             state,
		ReasonCode:        firstNonEmpty(obs.FailureReasonCode, obs.ReasonCode),
		RecommendedAction: action,
		Source:            "observation_authority",
		EvidenceRefs:      compactRefs(obs.ReportID, obs.ArtifactRef, obs.SurfaceRef),
		RequiresAction:    obs.MustObserve || obs.MustReplan,
	}, true
}

func proofSignal(proof loopkernel.ProofCoverageAuthorityView) (types.TruthSignal, bool) {
	state := types.TruthLedgerUnknown
	action := types.TruthActionVerify
	switch proof.State {
	case loopkernel.ProofCoverageCovered:
		state = types.TruthLedgerCovered
		action = types.TruthActionContinue
	case loopkernel.ProofCoverageFailed:
		state = types.TruthLedgerFailed
		action = types.TruthActionRepair
	case loopkernel.ProofCoverageWeak, loopkernel.ProofCoverageMissing:
		state = types.TruthLedgerWeak
		action = types.TruthActionAddProof
	case loopkernel.ProofCoverageUnavailable:
		state = types.TruthLedgerUnverified
		action = types.TruthActionContinue
	default:
		return types.TruthSignal{}, false
	}
	return types.TruthSignal{
		Kind:              types.TruthSignalProofCoverage,
		State:             state,
		ReasonCode:        proof.ReasonCode,
		RecommendedAction: action,
		Source:            "proof_coverage_authority",
		RequiresAction:    proof.RequiresProof,
	}, true
}

func localizationSignal(localization loopkernel.LocalizationAuthorityView) (types.TruthSignal, bool) {
	state := types.TruthLedgerUnknown
	action := types.TruthActionLocalize
	switch localization.State {
	case loopkernel.LocalizationAuthorityOwnerSupported:
		state = types.TruthLedgerCovered
		action = types.TruthActionContinue
	case loopkernel.LocalizationAuthorityObservedOnly,
		loopkernel.LocalizationAuthorityAuxiliaryOnly,
		loopkernel.LocalizationAuthorityConflicted,
		loopkernel.LocalizationAuthorityMissing,
		loopkernel.LocalizationAuthorityUnknown:
		state = types.TruthLedgerWeak
		action = types.TruthActionLocalize
	default:
		return types.TruthSignal{}, false
	}
	return types.TruthSignal{
		Kind:              types.TruthSignalLocalization,
		State:             state,
		ReasonCode:        localization.ReasonCode,
		RecommendedAction: action,
		Source:            "localization_authority",
		Paths:             compactRefs(append(append([]string(nil), localization.SourcePaths...), localization.OwnerMissingPaths...)...),
		RequiresAction:    localization.RequiresMoreContext,
	}, true
}

func navigationSignal(coverage types.RepoMapNavigationCoverage) (types.TruthSignal, bool) {
	coverage = types.NormalizeRepoMapNavigationCoverage(coverage)
	switch coverage.State {
	case types.RepoMapNavigationCoverageCovered, types.RepoMapNavigationCoverageNotRequired:
		return types.TruthSignal{
			Kind:              types.TruthSignalNavigation,
			State:             types.TruthLedgerCovered,
			ReasonCode:        coverage.ReasonCode,
			RecommendedAction: types.TruthActionContinue,
			Source:            "repo_map_navigation_coverage",
			EvidenceRefs:      compactRefs(coverage.EvidenceRefs...),
		}, true
	case types.RepoMapNavigationCoveragePartial, types.RepoMapNavigationCoverageMissing:
		return types.TruthSignal{
			Kind:              types.TruthSignalNavigation,
			State:             types.TruthLedgerWeak,
			ReasonCode:        coverage.ReasonCode,
			RecommendedAction: types.TruthActionLocalize,
			Source:            "repo_map_navigation_coverage",
			EvidenceRefs:      compactRefs(coverage.EvidenceRefs...),
			RequiresAction:    true,
		}, true
	default:
		return types.TruthSignal{}, false
	}
}

func patchReviewSignal(review *types.PatchReviewRecord) (types.TruthSignal, bool) {
	if review == nil {
		return types.TruthSignal{}, false
	}
	normalized := types.NormalizePatchReviewRecord(*review)
	summary := types.SummarizePatchReviewCoverage(normalized)
	state := types.TruthLedgerCovered
	action := types.TruthActionContinue
	switch {
	case summary.HardBlock || summary.Verdict == types.PatchReviewCoverageVerdictFailed:
		state = types.TruthLedgerFailed
		action = types.TruthActionRepair
	case summary.HasUncoveredSemantic:
		state = types.TruthLedgerWeak
		action = types.TruthActionAddProof
	case summary.Verdict == types.PatchReviewCoverageVerdictUnverified:
		state = types.TruthLedgerWeak
		action = types.TruthActionAddProof
	case summary.Verdict == types.PatchReviewCoverageVerdictAdvisory:
		state = types.TruthLedgerCovered
	}
	return types.TruthSignal{
		Kind:              types.TruthSignalPatchReview,
		State:             state,
		ReasonCode:        firstNonEmpty(summary.BlockReason, firstString(summary.ReasonCodes), string(summary.Verdict)),
		RecommendedAction: action,
		Source:            "patch_review",
		Paths:             compactRefs(append(append([]string(nil), normalized.AppliedPaths...), normalized.TargetPaths...)...),
		EvidenceRefs:      patchReviewEvidenceRefs(normalized),
		HardBlock:         summary.HardBlock,
		RequiresAction:    state != types.TruthLedgerCovered,
	}, true
}

func impactSignal(analysis *types.ImpactAnalysisResult) (types.TruthSignal, bool) {
	if analysis == nil {
		return types.TruthSignal{}, false
	}
	normalized := types.NormalizeImpactAnalysisResult(*analysis)
	if len(normalized.ChangedSurfaces) == 0 && len(normalized.Edges) == 0 && len(normalized.VerificationTargets) == 0 {
		return types.TruthSignal{}, false
	}
	state := types.TruthLedgerCovered
	action := types.TruthActionContinue
	requires := false
	for _, target := range normalized.VerificationTargets {
		status := strings.TrimSpace(target.CoverageStatus)
		if status == "" || status == "unverified" || status == "missing" || status == "unknown" {
			state = types.TruthLedgerWeak
			action = types.TruthActionAddProof
			requires = true
			break
		}
	}
	return types.TruthSignal{
		Kind:              types.TruthSignalImpact,
		State:             state,
		ReasonCode:        impactReason(normalized, state),
		RecommendedAction: action,
		Source:            "impact_analysis",
		Paths:             impactPaths(normalized),
		EvidenceRefs:      impactEvidenceRefs(normalized),
		RequiresAction:    requires,
	}, true
}

func patchReviewEvidenceRefs(review types.PatchReviewRecord) []string {
	var refs []string
	for _, finding := range review.Findings {
		refs = append(refs, finding.EvidenceRef)
	}
	return compactRefs(refs...)
}

func impactPaths(analysis types.ImpactAnalysisResult) []string {
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
	return compactRefs(paths...)
}

func impactEvidenceRefs(analysis types.ImpactAnalysisResult) []string {
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
	return compactRefs(refs...)
}

func impactReason(analysis types.ImpactAnalysisResult, state types.TruthLedgerState) string {
	if state == types.TruthLedgerWeak {
		return "impact_targets_unverified"
	}
	if len(analysis.VerificationTargets) > 0 {
		return "impact_targets_covered"
	}
	return "impact_surfaces_projected"
}

func compactRefs(values ...string) []string {
	seen := map[string]bool{}
	var out []string
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func firstString(values []string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func firstNonAction(values ...types.TruthRecommendedAction) types.TruthRecommendedAction {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
