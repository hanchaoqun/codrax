package types

import "strings"

// VerificationDeliverySnapshot identifies retained applied bytes, not a test
// execution or a new apply by the current plan. Only the controller authors it.
// The original effect owner is preserved across proof-only follow-ups.
type VerificationDeliverySnapshot struct {
	SourcePlanID     string             `json:"source_plan_id"`
	AppliedCommitSHA string             `json:"applied_commit_sha"`
	PatchEffect      *PatchEffectRecord `json:"patch_effect"`
}

// CloneVerificationDeliverySnapshot makes nested hunk/line/event ownership
// independent from the source plan and from later controller restamps.
func CloneVerificationDeliverySnapshot(in VerificationDeliverySnapshot) VerificationDeliverySnapshot {
	out := in
	if in.PatchEffect == nil {
		return out
	}
	effect := *in.PatchEffect
	effect.Files = append([]PatchEffectFile(nil), in.PatchEffect.Files...)
	for i := range effect.Files {
		file := &effect.Files[i]
		file.Events = append([]PatchEffectEvent(nil), file.Events...)
		file.Hunks = append([]PatchEffectHunk(nil), file.Hunks...)
		for j := range file.Hunks {
			hunk := &file.Hunks[j]
			hunk.AddedLineNumbers = append([]int(nil), hunk.AddedLineNumbers...)
			hunk.AddedLineTexts = append([]PatchEffectLine(nil), hunk.AddedLineTexts...)
			hunk.RemovedLineTexts = append([]PatchEffectLine(nil), hunk.RemovedLineTexts...)
		}
	}
	out.PatchEffect = &effect
	return out
}

// VerificationDeliverySnapshotFromAppliedPlan admits only a complete source
// delivery. Inherited metadata is never re-labelled as this plan's own apply.
func VerificationDeliverySnapshotFromAppliedPlan(plan *ChangePlan) (VerificationDeliverySnapshot, bool) {
	if plan == nil {
		return VerificationDeliverySnapshot{}, false
	}
	out := VerificationDeliverySnapshot{SourcePlanID: plan.ID, AppliedCommitSHA: plan.AppliedCommitSHA, PatchEffect: plan.PatchEffect}
	if !out.Valid() {
		return VerificationDeliverySnapshot{}, false
	}
	return CloneVerificationDeliverySnapshot(out), true
}

// Valid verifies the retained identity's shape, not its physical existence.
// The executor must still recheck the commit/diff and current worktree bytes.
func (in VerificationDeliverySnapshot) Valid() bool {
	effect := in.PatchEffect
	if in.SourcePlanID == "" || strings.TrimSpace(in.SourcePlanID) != in.SourcePlanID || !probeTargetCommit(in.AppliedCommitSHA) ||
		effect == nil || effect.PlanID != in.SourcePlanID || strings.TrimSpace(effect.RecordID) == "" ||
		!probeTargetDigest(effect.DiffFingerprint) || strings.TrimSpace(effect.HeadRef) == "" || strings.HasPrefix(effect.HeadRef, "-") {
		return false
	}
	if probeTargetCommit(effect.HeadRef) && effect.HeadRef != in.AppliedCommitSHA {
		return false
	}
	switch effect.Source {
	case "applied_commit":
		return true
	case "workflow_cumulative_owned_diff":
		return strings.TrimSpace(effect.BaseRef) != "" && !strings.HasPrefix(effect.BaseRef, "-") && len(effect.Files) > 0
	default:
		return false
	}
}

// ResolveVerificationDelivery separates the executing plan from the plan that
// applied its source. It grants no execution proof: tools must still validate
// the physical current HEAD, exact diff and source bytes before minting a fresh
// receipt. Existing own-effect consumers retain their stricter field checks.
// Multi-source scope is intentionally unresolved, never a first-item choice.
func ResolveVerificationDelivery(plan *ChangePlan) (VerificationDeliverySnapshot, bool) {
	if plan == nil || plan.ID == "" {
		return VerificationDeliverySnapshot{}, false
	}
	if plan.PatchEffect != nil {
		if plan.PatchEffect.PlanID != plan.ID {
			return VerificationDeliverySnapshot{}, false
		}
		return CloneVerificationDeliverySnapshot(VerificationDeliverySnapshot{SourcePlanID: plan.ID, AppliedCommitSHA: plan.AppliedCommitSHA, PatchEffect: plan.PatchEffect}), true
	}
	// AppliedAt is also stamped by verify-only lifecycle transitions; unlike
	// these concrete apply carriers, its presence does not prove an own apply.
	if plan.AppliedCommitSHA != "" || len(plan.AppliedPaths) != 0 || plan.ApplyCheckpoint != nil ||
		!(IsPersistedProofProbeOnlyPlan(plan) || IsPersistedNativeTestRegistrationPlan(plan)) || plan.CumulativeVerificationScope == nil {
		return VerificationDeliverySnapshot{}, false
	}
	scope := plan.CumulativeVerificationScope
	if len(scope.SourcePlanIDs) != 1 || len(scope.AppliedSources) != 1 {
		return VerificationDeliverySnapshot{}, false
	}
	source := scope.AppliedSources[0]
	if scope.SourcePlanIDs[0] != source.SourcePlanID || source.SourcePlanID == plan.ID || !source.Valid() {
		return VerificationDeliverySnapshot{}, false
	}
	return CloneVerificationDeliverySnapshot(source), true
}

// Legacy receipts without source_plan_id belong only to an own delivery. They
// cannot be copied into a proof-only plan to borrow an older native execution.
func verificationDeliveryReceiptSourceMatches(plan *ChangePlan, delivery VerificationDeliverySnapshot, source string) bool {
	return source == delivery.SourcePlanID || source == "" && delivery.SourcePlanID == plan.ID
}
