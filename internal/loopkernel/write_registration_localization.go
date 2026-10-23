package loopkernel

import "github.com/hanchaoqun/codrax/internal/types"

// WorkflowRegistrationReusesVerifiedSource distinguishes verification targets
// from new source-owner obligations. It does not grant execution or owner
// evidence: the registration must match the independent run receipt and an
// already verified, exact applied source. The executor still revalidates the
// physical delivery before each invocation. Missing/conflicting history keeps
// the ordinary conservative localization projection.
func WorkflowRegistrationReusesVerifiedSource(run types.WriteWorkflowRun, plan *types.ChangePlan) bool {
	return workflowRegistrationSourceScopeQualified(run, plan, false)
}

// WorkflowRegistrationHasNoNewSourceOwnerObligation only classifies the active
// read-only registration's localization scope. A missing source terminal is
// not a new source edit when the exact apply is bound and source verification
// has not occurred. A complete successful verify chain also qualifies, even
// when legacy normalization backfilled its terminal. Neither case certifies
// source ownership or creates a source verification. Genuinely absent source
// verdicts remain absent for the existing overall completion aggregator.
// An explicit non-verified terminal, incomplete history or new owner scope
// keeps the ordinary localization requirements.
func WorkflowRegistrationHasNoNewSourceOwnerObligation(run types.WriteWorkflowRun, plan *types.ChangePlan) bool {
	return workflowRegistrationSourceScopeQualified(run, plan, true)
}

func workflowRegistrationSourceScopeQualified(run types.WriteWorkflowRun, plan *types.ChangePlan, allowMissingTerminal bool) bool {
	if run.Identity == nil || run.Identity.IdentitySchema != types.WriteWorkflowRepoIdentitySchemaVersion ||
		run.Identity.CanonicalRepoRoot == "" || !WorkflowLocalizationAppliedHistoryCovered(run, plan) ||
		!types.NativeTestRegistrationMatchesRun(plan, &run, run.Identity.CanonicalRepoRoot) {
		return false
	}
	delivery, ok := types.ResolveVerificationDelivery(plan)
	if !ok || !delivery.Valid() {
		return false
	}
	// These paths are covered by the sealed registration and the independent
	// run receipt, not by a comparison against a caller-supplied root or hint.
	targets := make(map[string]bool, len(plan.TargetPaths))
	for _, path := range plan.TargetPaths {
		targets[path] = true
	}
	if review := plan.LocalizationReview; review != nil &&
		(len(review.SourcePaths) != 0 || len(review.OwnerMissingPaths) != 0 || len(review.MissingPaths) != 0) {
		return false
	}
	sourceCount, activeCount := 0, 0
	for _, batch := range run.Batches {
		if batch.ID == run.ActiveBatchID {
			activeCount++
			if len(batch.Slices) != 0 || batch.ActiveSliceID != "" {
				return false
			}
			for _, path := range batch.ExpectedPaths {
				if !targets[path] {
					return false
				}
			}
		}
		if batch.ApplyRef != "" && (batch.PlanID != delivery.SourcePlanID || batch.ApplyRef != delivery.AppliedCommitSHA) {
			return false
		}
		for _, attempt := range batch.Attempts {
			if attempt.Kind == "apply" && (attempt.Status != "applied" || attempt.PlanID != delivery.SourcePlanID || attempt.ArtifactRef != delivery.AppliedCommitSHA) {
				return false
			}
		}
		for _, slice := range batch.Slices {
			for _, attempt := range slice.Attempts {
				if attempt.Kind == "apply" {
					// Slice-only or interrupted delivery is outside this
					// single, fully verified unsliced source qualification.
					return false
				}
			}
			if slice.ApplyRef != "" {
				return false
			}
		}
		if batch.PlanID != delivery.SourcePlanID {
			continue
		}
		sourceCount++
		if batch.ID == run.ActiveBatchID || batch.Status != types.WriteWorkflowBatchComplete ||
			batch.Purpose == "verification_proof_followup" || batch.Purpose == "impact_and_verification_proof_followup" ||
			batch.ApplyRef != delivery.AppliedCommitSHA || len(batch.Slices) != 0 || batch.ActiveSliceID != "" {
			return false
		}
		if batch.Completion == nil {
			if !allowMissingTerminal {
				return false
			}
		} else if batch.Completion.Verdict != types.WriteWorkflowCompletionVerified ||
			(batch.Completion.Source != "verify_attempt" && !(allowMissingTerminal && batch.Completion.Source == "attempts_backfill")) {
			return false
		}
		applied := false
		var latestVerify *types.WriteWorkflowAttempt
		for i := range batch.Attempts {
			attempt := &batch.Attempts[i]
			if attempt.Kind == "apply" {
				applied = true // exact source/commit/status checked above
			}
			if attempt.Kind == "verify" {
				latestVerify = attempt
			}
		}
		if !applied {
			return false
		}
		if allowMissingTerminal && batch.Completion == nil && batch.VerifyRef == "" && latestVerify == nil {
			// Source verification is genuinely absent, not failed, pending or
			// identified by a dangling report. Its overall verdict stays absent.
			continue
		}
		if batch.VerifyRef == "" || latestVerify == nil || latestVerify.Status != "passed" ||
			latestVerify.PlanID != delivery.SourcePlanID || latestVerify.ReportID != batch.VerifyRef ||
			(latestVerify.ArtifactRef != "" && latestVerify.ArtifactRef != batch.VerifyRef) {
			return false
		}
	}
	// A separately declared delivery domain cannot be erased by the read-only
	// registration. Incidental read anchors stay in the run, unchanged.
	for _, pack := range run.ContextPacks {
		if pack.BatchID != "" && pack.BatchID != run.ActiveBatchID {
			continue
		}
		for _, item := range pack.Items {
			if item.BatchID != "" && item.BatchID != run.ActiveBatchID {
				continue
			}
			if anchor := item.LocalizationAnchor; anchor != nil && anchor.Kind == types.SourceLocalizationAnchorScope && !targets[anchor.Path] {
				return false
			}
		}
	}
	return sourceCount == 1 && activeCount == 1
}
