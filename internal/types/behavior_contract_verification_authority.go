package types

import (
	"reflect"
	"strings"
)

// BehaviorContractRefIsPlanningOnly resolves only the current typed contract
// scope, including retained contracts and retirement. A missing id is unknown,
// not evidence that a declaration was planning-only.
func BehaviorContractRefIsPlanningOnly(plan *ChangePlan, ref string) bool {
	ref = strings.TrimSpace(ref)
	for _, contract := range ChangePlanVerificationBehaviorContracts(plan) {
		if strings.TrimSpace(contract.ID) == ref && ref != "" {
			return IsPlanningOnlyWriteBehaviorContract(contract)
		}
	}
	return false
}

// BehaviorContractRefHasVerificationWitness requires the current report's
// exact contract-ref observation and the existing contract/witness matrix.
// Path execution, suite success, declared refs and persisted coverage labels
// are not per-contract witnesses. Planning-only declarations never gain this
// authority, even when a compatible test happens to mention their id.
func BehaviorContractRefHasVerificationWitness(plan *ChangePlan, report *ChangeReport, ref string) bool {
	return newBehaviorContractVerificationAuthority(plan, report).witnessed[strings.TrimSpace(ref)]
}

// BehaviorContractVerificationStateConflict preserves a failed verifier attempt
// whose retained runner report itself passed (for example an external verify
// error). The report's partial results remain facts, not permission to undo the
// persisted failed plan on reload. A genuinely failed report is a different
// axis and can still carry independent passed assertion receipts.
func BehaviorContractVerificationStateConflict(plan *ChangePlan, report *ChangeReport) bool {
	return plan != nil && report != nil && strings.TrimSpace(plan.ID) != "" &&
		strings.TrimSpace(plan.ID) == strings.TrimSpace(report.PlanID) &&
		plan.Status == PlanStatusVerifyFailed && report.NormalizeVerificationStatus() == VerificationStatusPassed
}

type behaviorContractVerificationAuthority struct {
	planning  map[string]bool
	witnessed map[string]bool
}

func newBehaviorContractVerificationAuthority(plan *ChangePlan, report *ChangeReport) behaviorContractVerificationAuthority {
	out := behaviorContractVerificationAuthority{planning: map[string]bool{}, witnessed: map[string]bool{}}
	contracts := ChangePlanVerificationBehaviorContracts(plan)
	for _, contract := range contracts {
		if id := strings.TrimSpace(contract.ID); id != "" && IsPlanningOnlyWriteBehaviorContract(contract) {
			out.planning[id] = true
		}
	}
	if plan == nil || report == nil || strings.TrimSpace(plan.ID) == "" || strings.TrimSpace(report.PlanID) != strings.TrimSpace(plan.ID) || BehaviorContractVerificationStateConflict(plan, report) {
		return out
	}
	records := EffectiveVerificationConfidence(plan, report)
	for _, contract := range contracts {
		id := strings.TrimSpace(contract.ID)
		if id == "" || out.planning[id] {
			continue
		}
		for _, rec := range records {
			if VerificationConfidenceRecordCoversContract(rec, contract) {
				out.witnessed[id] = true
				break
			}
		}
	}
	return out
}

func (a behaviorContractVerificationAuthority) coverage(ref, previous string) string {
	ref = strings.TrimSpace(ref)
	// Independent failures retain their meaning. This projection does not
	// upgrade the overall report, review status, severity or hard-block flag.
	switch strings.TrimSpace(previous) {
	case "failed", "error":
		return previous
	}
	if ref != "" && a.planning[ref] {
		return "advisory"
	}
	if ref != "" && a.witnessed[ref] {
		return "verified"
	}
	switch strings.TrimSpace(previous) {
	case "verified", "covered", "passed":
		return "unverified"
	default:
		return previous
	}
}

// EffectiveBehaviorContractVerificationPlan is a read-only authority view of
// derived impact/review states, including persisted legacy plans. It never
// rewrites model declarations, report bytes or source content. Only behavioral
// coverage states and the review's derived summary are projected; unrelated
// impact kinds and hard findings retain their existing semantics.
func EffectiveBehaviorContractVerificationPlan(plan *ChangePlan, report *ChangeReport) *ChangePlan {
	if plan == nil {
		return nil
	}
	out := *plan
	authority := newBehaviorContractVerificationAuthority(plan, report)
	forCarrier := func(id string) behaviorContractVerificationAuthority {
		if id = strings.TrimSpace(id); id != "" && id != strings.TrimSpace(plan.ID) {
			return behaviorContractVerificationAuthority{}
		}
		return authority
	}
	if plan.ImpactAnalysis != nil {
		analysis := *plan.ImpactAnalysis
		analysis.VerificationTargets = append([]ImpactVerificationTarget(nil), analysis.VerificationTargets...)
		bound := forCarrier(analysis.PlanID)
		for i := range analysis.VerificationTargets {
			target := &analysis.VerificationTargets[i]
			if strings.TrimSpace(target.Kind) == "behavior_contract" {
				target.CoverageStatus = bound.coverage(firstNonEmptyVerificationProof(target.ContractRef, target.EvidenceRef), target.CoverageStatus)
			}
		}
		out.ImpactAnalysis = &analysis
	}
	if plan.PatchReview != nil {
		review := *plan.PatchReview
		review.Findings = append([]PatchReviewFinding(nil), review.Findings...)
		bound := forCarrier(review.PlanID)
		changed := false
		for i := range review.Findings {
			finding := &review.Findings[i]
			if finding.Category != PatchReviewCategorySemanticCoverage || patchReviewImpactKindForFinding(*finding) != PatchReviewImpactKindBehaviorContract {
				continue
			}
			status := PatchReviewCoverageStatus(bound.coverage(finding.EvidenceRef, string(finding.CoverageStatus)))
			changed = changed || status != finding.CoverageStatus
			finding.CoverageStatus = status
		}
		if changed {
			summary := SummarizePatchReviewCoverage(review)
			review.CoverageSummary = &summary
		}
		out.PatchReview = &review
	}
	return &out
}

const derivedBehaviorContractWitnessMissing = "derived_behavior_contract_witness_missing"

// The cumulative lane replaces an old derived debt with advisory history only
// when a separately bound report witnesses the same complete contract value.
// Equal refs alone are insufficient: Source, expected value, subject, placement
// and every future field participate in this deliberately conservative match.
func effectiveCumulativeBehaviorContractArtifacts(in []VerificationProofArtifact) []VerificationProofArtifact {
	var witnessed []WriteBehaviorContract
	for _, artifact := range in {
		authority := newBehaviorContractVerificationAuthority(artifact.Plan, artifact.Report)
		for _, contract := range ChangePlanVerificationBehaviorContracts(artifact.Plan) {
			if authority.witnessed[strings.TrimSpace(contract.ID)] {
				witnessed = append(witnessed, contract)
			}
		}
	}
	out := append([]VerificationProofArtifact(nil), in...)
	for i := range out {
		plan := EffectiveBehaviorContractVerificationPlan(out[i].Plan, out[i].Report)
		out[i].Plan = plan
		if plan == nil {
			continue
		}
		contracts := ChangePlanVerificationBehaviorContracts(plan)
		covered := func(ref string) bool {
			for _, contract := range contracts {
				if strings.TrimSpace(contract.ID) != strings.TrimSpace(ref) || IsPlanningOnlyWriteBehaviorContract(contract) {
					continue
				}
				for _, receiptContract := range witnessed {
					if reflect.DeepEqual(contract, receiptContract) {
						return true
					}
				}
			}
			return false
		}
		if analysis := plan.ImpactAnalysis; analysis != nil && (analysis.PlanID == "" || strings.TrimSpace(analysis.PlanID) == strings.TrimSpace(plan.ID)) {
			for j := range analysis.VerificationTargets {
				target := &analysis.VerificationTargets[j]
				if target.Kind == "behavior_contract" && target.CoverageStatus == "unverified" && covered(firstNonEmptyVerificationProof(target.ContractRef, target.EvidenceRef)) {
					target.CoverageStatus = "advisory"
				}
			}
		}
		if review := plan.PatchReview; review != nil && (review.PlanID == "" || strings.TrimSpace(review.PlanID) == strings.TrimSpace(plan.ID)) {
			changed := false
			for j := range review.Findings {
				finding := &review.Findings[j]
				if finding.Category == PatchReviewCategorySemanticCoverage && patchReviewImpactKindForFinding(*finding) == PatchReviewImpactKindBehaviorContract && finding.CoverageStatus == PatchReviewCoverageUnverified && covered(finding.EvidenceRef) {
					finding.CoverageStatus = PatchReviewCoverageAdvisory
					changed = true
				}
			}
			if changed {
				summary := SummarizePatchReviewCoverage(*review)
				review.CoverageSummary = &summary
			}
		}
	}
	return out
}

func cumulativeBehaviorContractDebtProfile(profile VerificationProofProfile, artifacts []VerificationProofArtifact) VerificationProofProfile {
	add := func(reason string) {
		profile.ReasonCodes = append(profile.ReasonCodes, reason)
		if profile.Status == VerificationProofAdequate || profile.Status == VerificationProofStrong {
			profile.Status = VerificationProofWeak
		}
	}
	for _, artifact := range artifacts {
		if BehaviorContractVerificationStateConflict(artifact.Plan, artifact.Report) {
			add("verification_plan_report_state_conflict")
		}
		if artifact.Plan == nil {
			continue
		}
		if analysis := artifact.Plan.ImpactAnalysis; analysis != nil {
			for _, target := range analysis.VerificationTargets {
				if target.Kind == "behavior_contract" && target.CoverageStatus == "unverified" {
					add("impact_targets_unverified")
					break
				}
			}
		}
		if review := artifact.Plan.PatchReview; review != nil {
			for _, finding := range review.Findings {
				if finding.Category == PatchReviewCategorySemanticCoverage && patchReviewImpactKindForFinding(finding) == PatchReviewImpactKindBehaviorContract && finding.CoverageStatus == PatchReviewCoverageUnverified {
					add("patch_review_semantic_unverified")
					break
				}
			}
		}
	}
	return NormalizeVerificationProofProfile(profile)
}
