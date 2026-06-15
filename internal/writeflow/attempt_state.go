package writeflow

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// BatchAttemptPhase is the single canonical model-facing phase of a write
// workflow batch, derived from the batch status plus its typed attempt
// records. It removes the two-readings problem where a batch rendered both
// "ready_to_plan" (state) and "verify_failed" (event) at once.
type BatchAttemptPhase string

const (
	BatchPhaseNeedsExploration BatchAttemptPhase = "needs_exploration"
	BatchPhaseReadyToPlan      BatchAttemptPhase = "ready_to_plan"
	BatchPhaseNeedsReplan      BatchAttemptPhase = "needs_replan"
	BatchPhasePlanned          BatchAttemptPhase = "planned"
	BatchPhasePendingApproval  BatchAttemptPhase = "pending_approval"
	BatchPhaseApplying         BatchAttemptPhase = "applying"
	BatchPhaseVerifying        BatchAttemptPhase = "verifying"
	BatchPhaseComplete         BatchAttemptPhase = "complete"
	BatchPhaseBlocked          BatchAttemptPhase = "blocked"
)

// BatchAttemptState is the canonical derived view of one batch: a single
// phase, the typed cause from the deciding attempt, and the artifact refs a
// consumer needs to look the evidence up. All inputs are typed records;
// nothing is parsed from prose.
type BatchAttemptState struct {
	Phase BatchAttemptPhase `json:"phase"`

	// Cause is the typed reason code of the attempt that put the batch in
	// this phase (e.g. "tests_failed", "build_failed"). Empty when the
	// phase is not attempt-driven.
	Cause string `json:"cause,omitempty"`

	PlanID   string `json:"plan_id,omitempty"`
	ReportID string `json:"report_id,omitempty"`

	// LatestVerifyStatus / LatestVerifyArtifactRef / LatestVerifySurfaceRef
	// expose the newest verify attempt's typed status ("passed", "failed",
	// "unverified", ...), its artifact ref (the persisted attempt diff for
	// failures), and the runnable-test-surface artifact for resume hydration
	// and finish caveats.
	LatestVerifyStatus      string `json:"latest_verify_status,omitempty"`
	LatestVerifyArtifactRef string `json:"latest_verify_artifact_ref,omitempty"`
	LatestVerifySurfaceRef  string `json:"latest_verify_surface_ref,omitempty"`

	// FailedVerifyAttempts counts post-apply verify attempts with a failed
	// status. Attempt records are only written by the scheduler's verify
	// branch, so planner dry-run probes never inflate this count.
	FailedVerifyAttempts int `json:"failed_verify_attempts,omitempty"`
}

// latestAttemptOfKind returns the last attempt record of the given kind, or
// nil. Attempt records append in execution order.
func latestAttemptOfKind(batch types.WriteWorkflowBatch, kind string) *types.WriteWorkflowAttempt {
	for i := len(batch.Attempts) - 1; i >= 0; i-- {
		if batch.Attempts[i].Kind == kind {
			return &batch.Attempts[i]
		}
	}
	return nil
}

// DeriveBatchAttemptState computes the canonical phase for one batch.
// The one non-identity derivation: a batch whose status is ready_to_plan
// while its latest post-apply verify attempt failed is in needs_replan with
// the verify attempt's typed reason code as cause — one phase, not two
// contradictory readings.
func DeriveBatchAttemptState(batch types.WriteWorkflowBatch) BatchAttemptState {
	st := BatchAttemptState{
		Phase:  BatchAttemptPhase(batch.Status),
		PlanID: strings.TrimSpace(batch.PlanID),
	}
	for _, a := range batch.Attempts {
		if a.Kind == "verify" && a.Status == "failed" {
			st.FailedVerifyAttempts++
		}
	}
	latestVerify := latestAttemptOfKind(batch, "verify")
	if latestVerify != nil {
		st.ReportID = strings.TrimSpace(latestVerify.ReportID)
		st.LatestVerifyStatus = strings.TrimSpace(latestVerify.Status)
		st.LatestVerifyArtifactRef = strings.TrimSpace(latestVerify.ArtifactRef)
		st.LatestVerifySurfaceRef = strings.TrimSpace(latestVerify.SurfaceRef)
	}
	if batch.Status == types.WriteWorkflowBatchReadyToPlan && latestVerify != nil && latestVerify.Status == "failed" {
		st.Phase = BatchPhaseNeedsReplan
		st.Cause = strings.TrimSpace(latestVerify.ReasonCode)
	}
	return st
}

// ValidateWorkflowRunState checks cross-field invariants that should stay true
// across controller decisions, persisted runs, and resume. It reads only typed
// workflow records and the current ChangePlan approval record; no prose fields
// are used for hard interpretation.
func ValidateWorkflowRunState(run types.WriteWorkflowRun, plan *types.ChangePlan) []string {
	run = types.NormalizeWriteWorkflowRun(run)
	var errs []string
	batches := map[string]types.WriteWorkflowBatch{}
	for _, batch := range run.Batches {
		batches[batch.ID] = batch
		errs = append(errs, validateWorkflowBatchAttemptPlanIDs(batch)...)
	}
	if run.ActiveBatchID != "" {
		if _, ok := batches[run.ActiveBatchID]; !ok {
			errs = append(errs, "active_batch_id "+run.ActiveBatchID+" does not match any batch")
		}
	}
	if plan != nil {
		errs = append(errs, validateWorkflowPlanBatchState(run, plan)...)
	}
	return errs
}

func validateWorkflowBatchAttemptPlanIDs(batch types.WriteWorkflowBatch) []string {
	if strings.TrimSpace(batch.PlanID) == "" {
		return nil
	}
	activePlanStart := -1
	for i, attempt := range batch.Attempts {
		if attempt.Kind == "plan" && strings.TrimSpace(attempt.PlanID) == batch.PlanID {
			activePlanStart = i
		}
	}
	var errs []string
	for i, attempt := range batch.Attempts {
		if attempt.PlanID == "" || attempt.PlanID == batch.PlanID {
			continue
		}
		switch attempt.Kind {
		case "plan", "apply", "verify":
			if activePlanStart >= 0 && i < activePlanStart {
				continue
			}
			errs = append(errs, "batch "+batch.ID+" "+attempt.Kind+" attempt plan_id "+attempt.PlanID+" conflicts with batch plan_id "+batch.PlanID)
		}
	}
	return errs
}

func validateWorkflowPlanBatchState(run types.WriteWorkflowRun, plan *types.ChangePlan) []string {
	planID := strings.TrimSpace(plan.ID)
	if planID == "" || plan.Approval == nil {
		return nil
	}
	var errs []string
	for _, batch := range run.Batches {
		if batch.PlanID != planID {
			continue
		}
		switch strings.TrimSpace(plan.Approval.Action) {
		case string(ApprovalActionAutoExecute):
			if batch.Status == types.WriteWorkflowBatchPendingApproval {
				errs = append(errs, "batch "+batch.ID+" is pending_approval but plan approval action is auto_execute")
			}
		case string(ApprovalActionDeny):
			if batch.Status != types.WriteWorkflowBatchBlocked {
				errs = append(errs, "batch "+batch.ID+" is "+string(batch.Status)+" but plan approval action is deny")
			}
		}
	}
	return errs
}

// Finish disposition values: the typed escape lane for the finish hard gate.
// The system rule is "finish is rejected while a batch's latest post-apply
// verify attempt failed"; a model that judges the residual failure acceptable
// must say so in the typed field, never in prose.
const (
	FinishDispositionAllVerified      = "all_verified"
	FinishDispositionAcceptUnverified = "accept_unverified"
)

// FinishBlockedReason evaluates the typed finish gate against the run. It
// returns a non-empty typed reason string when finish must be rejected:
// some batch's latest post-apply verify attempt failed, the batch never
// completed, and the decision did not declare
// finish_disposition=accept_unverified. All inputs are typed attempt
// records and the schema-validated decision field.
func FinishBlockedReason(run types.WriteWorkflowRun, decision WriteWorkflowDecision) string {
	if strings.TrimSpace(decision.FinishDisposition) == FinishDispositionAcceptUnverified {
		return ""
	}
	var failed []string
	for _, batch := range run.Batches {
		switch batch.Status {
		case types.WriteWorkflowBatchComplete, types.WriteWorkflowBatchBlocked:
			continue
		}
		latestVerify := latestAttemptOfKind(batch, "verify")
		if latestVerify == nil || latestVerify.Status != "failed" {
			continue
		}
		entry := batch.ID
		if rc := strings.TrimSpace(latestVerify.ReasonCode); rc != "" {
			entry += "(" + rc + ")"
		}
		failed = append(failed, entry)
	}
	if len(failed) == 0 {
		return ""
	}
	return "latest post-apply verification failed for " + strings.Join(failed, ", ") +
		"; replan_batch / split_batch / block, or finish with finish_disposition=accept_unverified to accept the residual failure"
}

// UnverifiedBatchSummaries lists batches whose latest verify attempt failed,
// for the typed completion caveat when finish_disposition=accept_unverified
// is used.
func UnverifiedBatchSummaries(run types.WriteWorkflowRun) []string {
	var out []string
	for _, batch := range run.Batches {
		latestVerify := latestAttemptOfKind(batch, "verify")
		if latestVerify == nil || latestVerify.Status != "failed" {
			continue
		}
		entry := batch.ID
		if rc := strings.TrimSpace(latestVerify.ReasonCode); rc != "" {
			entry += " (" + rc + ")"
		}
		out = append(out, entry)
	}
	return out
}

// UnverifiedBatchCaveats lists batches whose latest verify attempt completed
// with the typed "unverified" status (for example no_tests or runner_missing).
// Finish paths append this as a result caveat so a run can never silently read
// "complete" without a validating local assertion.
func UnverifiedBatchCaveats(run types.WriteWorkflowRun) []string {
	var out []string
	for _, batch := range run.Batches {
		latestVerify := latestAttemptOfKind(batch, "verify")
		if latestVerify == nil || latestVerify.Status != "unverified" {
			continue
		}
		out = append(out, batch.ID)
	}
	return out
}
