package orchestrator

import (
	"bytes"
	"encoding/json"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// verificationAppliedSources preserves source ownership independently from
// the active execution plan. Historical path/contract metadata has a wider
// restore closure; delivery authority must additionally still be live in the
// apply ledger. Nothing here reads model prose or lends an old test result.
func (o *Orchestrator) verificationAppliedSources(run *types.WriteWorkflowRun, sourceIDs []string, priorScopes []*types.CumulativeVerificationScope, candidates []*types.ChangePlan) []types.VerificationDeliverySnapshot {
	live := make(map[string]bool)
	for _, id := range o.writeFinalReportAppliedPlanIDs(run) {
		live[id] = true
	}
	wanted := make(map[string]bool)
	for _, id := range sourceIDs {
		if id != "" && id == strings.TrimSpace(id) && live[id] {
			wanted[id] = true
		}
	}
	byID := make(map[string]types.VerificationDeliverySnapshot)
	conflicts := make(map[string]bool)
	add := func(source types.VerificationDeliverySnapshot) {
		id := source.SourcePlanID
		if !wanted[id] {
			return
		}
		if !source.Valid() {
			conflicts[id] = true
			return
		}
		if prior, ok := byID[id]; ok && !verificationDeliveriesEqual(prior, source) {
			conflicts[id] = true
			return
		}
		byID[id] = types.CloneVerificationDeliverySnapshot(source)
	}
	for _, scope := range priorScopes {
		if scope == nil {
			continue
		}
		for _, source := range scope.AppliedSources {
			if cumulativeScopeContainsSourcePlanID(scope.SourcePlanIDs, source.SourcePlanID) {
				add(source)
			}
		}
	}
	// Do not use a last-writer-wins map for live plans: equal IDs with different
	// delivery identities are ambiguous, including one incomplete identity.
	seenCandidate := make(map[string]bool)
	for _, candidate := range candidates {
		if candidate == nil || !wanted[candidate.ID] {
			continue
		}
		seenCandidate[candidate.ID] = true
		if source, ok := verificationSourcePlanDelivery(candidate); ok {
			add(source)
		} else {
			conflicts[candidate.ID] = true
		}
	}
	for _, id := range sourceIDs {
		if !wanted[id] || seenCandidate[id] || o == nil || o.busCtx == nil {
			continue
		}
		// A previous controller snapshot is sufficient if the old artifact is
		// no longer in this blob root. If it is available, it must agree.
		if retained := o.loadDurablePlanArtifact(id); retained != nil {
			if source, ok := verificationSourcePlanDelivery(retained); ok {
				add(source)
			} else {
				conflicts[id] = true
			}
		}
	}
	var out []types.VerificationDeliverySnapshot
	seen := make(map[string]bool)
	for _, id := range sourceIDs {
		if source, ok := byID[id]; ok && !conflicts[id] && !seen[id] {
			out = append(out, types.CloneVerificationDeliverySnapshot(source))
			seen[id] = true
		}
	}
	return out
}

// Compare the same durable representation used for recovery. time.Time's
// monotonic/location internals are not source identity and do not survive JSON;
// every persisted identity, diff, line and timestamp value still participates.
func verificationDeliveriesEqual(a, b types.VerificationDeliverySnapshot) bool {
	left, leftErr := json.Marshal(a)
	right, rightErr := json.Marshal(b)
	return leftErr == nil && rightErr == nil && bytes.Equal(left, right)
}

func verificationSourcePlanDelivery(plan *types.ChangePlan) (types.VerificationDeliverySnapshot, bool) {
	if plan == nil || len(plan.Changes) == 0 {
		return types.VerificationDeliverySnapshot{}, false
	}
	switch plan.Status {
	case types.PlanStatusApplied, types.PlanStatusAppliedPendingVerify, types.PlanStatusVerifyFailed, types.PlanStatusUnverified, types.PlanStatusMerged:
		return types.VerificationDeliverySnapshotFromAppliedPlan(plan)
	default:
		return types.VerificationDeliverySnapshot{}, false
	}
}
