package loopkernel

import (
	"encoding/json"
	"testing"

	"github.com/hanchaoqun/codrax/internal/safety"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestEventsFromWriteWorkflowRunPendingApprovalProjectsAsk(t *testing.T) {
	run := types.WriteWorkflowRun{
		RunID:         "wf-approval",
		Status:        types.WriteWorkflowRunInProgress,
		ActiveBatchID: "batch-1",
		Batches: []types.WriteWorkflowBatch{{
			ID:     "batch-1",
			Status: types.WriteWorkflowBatchPendingApproval,
			PlanID: "plan-1",
		}},
	}
	got := ReduceEvents(EventsFromWriteWorkflowRun(run))
	if got.Status != LoopRunStatusInProgress {
		t.Fatalf("status = %s, want in_progress", got.Status)
	}
	if got.Permission.State != PermissionAuthorityAsk || !got.Permission.RequiresUser {
		t.Fatalf("permission ask not projected: %+v", got.Permission)
	}
	if got.ActiveUnitID != "batch:batch-1" {
		t.Fatalf("active unit = %q", got.ActiveUnitID)
	}
}

func TestEventsFromWriteWorkflowRunPlannedProjectsRuntimeUnitEffect(t *testing.T) {
	run := types.WriteWorkflowRun{
		RunID:         "wf-effect",
		Status:        types.WriteWorkflowRunInProgress,
		ActiveBatchID: "batch-1",
		Batches: []types.WriteWorkflowBatch{{
			ID:            "batch-1",
			Status:        types.WriteWorkflowBatchPlanned,
			PlanID:        "plan-1",
			ActiveSliceID: "slice-1",
			Slices: []types.WriteWorkflowSlice{{
				ID:     "slice-1",
				Status: types.ChangePlanSlicePending,
				Paths:  []string{"pkg/a.go"},
			}},
		}},
	}
	events := EventsFromWriteWorkflowRun(run)
	var payload EffectEventPayload
	for _, event := range events {
		if event.Kind != LoopEventEffectDescribed {
			continue
		}
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			t.Fatalf("effect payload decode: %v", err)
		}
		break
	}
	if payload.Fingerprint == "" {
		t.Fatalf("runtime unit effect fingerprint missing: %+v", payload)
	}
	if payload.Effect.Role != safety.EffectRoleCoder || payload.Effect.Kind != safety.EffectKindApplyPatch {
		t.Fatalf("runtime unit effect mismatch: %+v", payload.Effect)
	}
	if len(payload.Effect.Paths) != 1 || payload.Effect.Paths[0] != "pkg/a.go" {
		t.Fatalf("runtime unit effect paths mismatch: %+v", payload.Effect.Paths)
	}
	state := ReduceEvents(events)
	if state.Permission.State != PermissionAuthorityAllow {
		t.Fatalf("ordinary runtime unit should project allow permission, got %+v", state.Permission)
	}
}

func TestEventsFromWriteWorkflowRunExternalRuntimeUnitEffectDenies(t *testing.T) {
	run := types.WriteWorkflowRun{
		RunID:         "wf-effect-deny",
		Status:        types.WriteWorkflowRunInProgress,
		ActiveBatchID: "batch-1",
		Batches: []types.WriteWorkflowBatch{{
			ID:            "batch-1",
			Status:        types.WriteWorkflowBatchPlanned,
			PlanID:        "plan-1",
			ActiveSliceID: "slice-1",
			Slices: []types.WriteWorkflowSlice{{
				ID:     "slice-1",
				Status: types.ChangePlanSlicePending,
				Paths:  []string{"../outside.py"},
			}},
		}},
	}
	got := ReduceEvents(EventsFromWriteWorkflowRun(run))
	if got.Permission.State != PermissionAuthorityDeny || got.Permission.ReasonCode != "external_directory_write" {
		t.Fatalf("external runtime unit effect should deny, got %+v", got.Permission)
	}
}

func TestEventsFromWriteWorkflowRunProjectsOwnerLocalizationAuthority(t *testing.T) {
	run := types.WriteWorkflowRun{
		RunID:         "wf-localized",
		Status:        types.WriteWorkflowRunInProgress,
		ActiveBatchID: "batch-1",
		Batches: []types.WriteWorkflowBatch{{
			ID:     "batch-1",
			Status: types.WriteWorkflowBatchReadyToPlan,
		}},
		ContextPacks: []types.WriteContextPack{{
			BatchID: "batch-1",
			Items: []types.WriteContextItem{{
				Priority: types.WriteContextP1,
				Kind:     "localization_anchor",
				Text:     "pkg/owner.py",
				LocalizationAnchor: &types.SourceLocalizationAnchor{
					Path:     "pkg/owner.py",
					Role:     types.SourcePathRoleProduction,
					Kind:     types.SourceLocalizationAnchorGroundedEvidence,
					Strength: types.SourceLocalizationAnchorOwner,
				},
			}},
		}},
	}
	got := ReduceEvents(EventsFromWriteWorkflowRun(run))
	if got.Localization.State != LocalizationAuthorityOwnerSupported {
		t.Fatalf("localization = %+v, want owner_supported", got.Localization)
	}
	if got.Localization.RecommendedAction != LoopActionContinue {
		t.Fatalf("owner localization should continue: %+v", got.Localization)
	}
}

func TestEventsFromWriteWorkflowRunObservedLocalizationNeedsLocalizer(t *testing.T) {
	run := types.WriteWorkflowRun{
		RunID:         "wf-observed",
		Status:        types.WriteWorkflowRunInProgress,
		ActiveBatchID: "batch-1",
		Batches: []types.WriteWorkflowBatch{{
			ID:     "batch-1",
			Status: types.WriteWorkflowBatchReadyToPlan,
		}},
		ContextPacks: []types.WriteContextPack{{
			BatchID: "batch-1",
			Items: []types.WriteContextItem{{
				Priority: types.WriteContextP1,
				Kind:     "localization_anchor",
				Text:     "pkg/maybe.py",
				LocalizationAnchor: &types.SourceLocalizationAnchor{
					Path:     "pkg/maybe.py",
					Role:     types.SourcePathRoleProduction,
					Kind:     types.SourceLocalizationAnchorReadFile,
					Strength: types.SourceLocalizationAnchorObserved,
				},
			}},
		}},
	}
	got := ReduceEvents(EventsFromWriteWorkflowRun(run))
	if got.Localization.State != LocalizationAuthorityObservedOnly {
		t.Fatalf("localization = %+v, want observed_only", got.Localization)
	}
	if got.Localization.RecommendedAction != LoopActionLocalize || !got.Localization.RequiresMoreContext {
		t.Fatalf("observed-only localization should request localizer: %+v", got.Localization)
	}
}

func TestEventsFromWriteWorkflowRunExpectedPathsNeedLocalizer(t *testing.T) {
	run := types.WriteWorkflowRun{
		RunID:         "wf-expected-path",
		Status:        types.WriteWorkflowRunInProgress,
		ActiveBatchID: "batch-1",
		Batches: []types.WriteWorkflowBatch{{
			ID:            "batch-1",
			Status:        types.WriteWorkflowBatchReadyToPlan,
			ExpectedPaths: []string{"pkg/maybe.py"},
		}},
	}
	got := ReduceEvents(EventsFromWriteWorkflowRun(run))
	if got.Localization.State != LocalizationAuthorityObservedOnly {
		t.Fatalf("localization = %+v, want observed_only", got.Localization)
	}
	if got.Localization.RecommendedAction != LoopActionLocalize || !got.Localization.RequiresMoreContext {
		t.Fatalf("expected paths should request owner localization: %+v", got.Localization)
	}
	if len(got.Localization.SourcePaths) != 1 || got.Localization.SourcePaths[0] != "pkg/maybe.py" {
		t.Fatalf("expected path not projected as source path: %+v", got.Localization.SourcePaths)
	}
}

func TestEventsFromWriteWorkflowRunNoLocalizationSignalProjectsMissingAuthority(t *testing.T) {
	run := types.WriteWorkflowRun{
		RunID:         "wf-no-localization",
		Status:        types.WriteWorkflowRunInProgress,
		ActiveBatchID: "batch-1",
		Batches: []types.WriteWorkflowBatch{{
			ID:     "batch-1",
			Status: types.WriteWorkflowBatchReadyToPlan,
		}},
	}
	got := ReduceEvents(EventsFromWriteWorkflowRun(run))
	if got.Localization.State != LocalizationAuthorityMissing {
		t.Fatalf("localization = %+v, want missing", got.Localization)
	}
	if got.Localization.RecommendedAction != LoopActionLocalize || !got.Localization.RequiresMoreContext {
		t.Fatalf("missing localization should remain a soft localize recommendation: %+v", got.Localization)
	}
	if len(got.Localization.SourcePaths) != 0 || len(got.Localization.OwnerMissingPaths) != 0 {
		t.Fatalf("no-signal localization must not invent candidate paths: %+v", got.Localization)
	}
}

func TestEventsFromWriteWorkflowRunVerifyingUsesActiveSlice(t *testing.T) {
	run := types.WriteWorkflowRun{
		RunID:         "wf-verify",
		Status:        types.WriteWorkflowRunInProgress,
		ActiveBatchID: "batch-1",
		Batches: []types.WriteWorkflowBatch{{
			ID:            "batch-1",
			Status:        types.WriteWorkflowBatchVerifying,
			PlanID:        "plan-1",
			ActiveSliceID: "slice-2",
			Slices: []types.WriteWorkflowSlice{{
				ID:     "slice-1",
				Status: types.ChangePlanSliceVerified,
			}, {
				ID:     "slice-2",
				Status: types.ChangePlanSliceObserving,
			}},
		}},
	}
	got := ReduceEvents(EventsFromWriteWorkflowRun(run))
	if got.ActiveUnitID != "slice-2" {
		t.Fatalf("active unit = %q, want slice-2", got.ActiveUnitID)
	}
	if len(got.Units) != 1 || got.Units[0].Status != RuntimeUnitObserving {
		t.Fatalf("unit view = %+v, want observing", got.Units)
	}
}

func TestEventsFromWriteWorkflowRunActiveVerifyAttemptProjectsProofAuthority(t *testing.T) {
	run := types.WriteWorkflowRun{
		RunID:         "wf-proof-attempt",
		Status:        types.WriteWorkflowRunInProgress,
		ActiveBatchID: "batch-1",
		Batches: []types.WriteWorkflowBatch{{
			ID:     "batch-1",
			Status: types.WriteWorkflowBatchVerifying,
			Attempts: []types.WriteWorkflowAttempt{{
				Kind:       "verify",
				Status:     "unverified",
				ReasonCode: "runner_missing",
			}},
		}},
	}
	got := ReduceEvents(EventsFromWriteWorkflowRun(run))
	if got.Proof.State != ProofCoverageUnavailable {
		t.Fatalf("proof = %+v, want unavailable", got.Proof)
	}
	if got.Proof.RecommendedAction == LoopActionRepair {
		t.Fatalf("unavailable proof must not request repair: %+v", got.Proof)
	}
}

func TestEventsFromWriteWorkflowRunCompleteProjectsProof(t *testing.T) {
	run := types.WriteWorkflowRun{
		RunID:         "wf-complete",
		Status:        types.WriteWorkflowRunComplete,
		ActiveBatchID: "batch-1",
		Batches: []types.WriteWorkflowBatch{{
			ID:     "batch-1",
			Status: types.WriteWorkflowBatchComplete,
			Completion: &types.WriteWorkflowCompletion{
				Verdict:    types.WriteWorkflowCompletionVerified,
				ReasonCode: "tests_passed",
			},
		}},
		Completion: &types.WriteWorkflowCompletion{
			Verdict:    types.WriteWorkflowCompletionVerified,
			ReasonCode: "tests_passed",
		},
	}
	got := ReduceEvents(EventsFromWriteWorkflowRun(run))
	if got.Status != LoopRunStatusComplete {
		t.Fatalf("status = %s, want complete", got.Status)
	}
	if got.Proof.State != ProofCoverageCovered {
		t.Fatalf("proof = %+v, want covered", got.Proof)
	}
	if len(got.Units) != 1 || got.Units[0].Status != RuntimeUnitComplete {
		t.Fatalf("unit view = %+v, want complete", got.Units)
	}
}

func TestEventsFromWriteWorkflowRunUnverifiedCompletionDoesNotRepair(t *testing.T) {
	run := types.WriteWorkflowRun{
		RunID:         "wf-unverified",
		Status:        types.WriteWorkflowRunComplete,
		ActiveBatchID: "batch-1",
		Batches: []types.WriteWorkflowBatch{{
			ID:     "batch-1",
			Status: types.WriteWorkflowBatchComplete,
			Completion: &types.WriteWorkflowCompletion{
				Verdict:    types.WriteWorkflowCompletionUnverified,
				ReasonCode: "runner_missing",
			},
		}},
		Completion: &types.WriteWorkflowCompletion{
			Verdict:    types.WriteWorkflowCompletionUnverified,
			ReasonCode: "runner_missing",
		},
	}
	got := ReduceEvents(EventsFromWriteWorkflowRun(run))
	if got.Proof.State != ProofCoverageUnavailable {
		t.Fatalf("proof = %+v, want unavailable", got.Proof)
	}
	if got.Proof.RecommendedAction == LoopActionRepair {
		t.Fatalf("unverified workflow must not force repair: %+v", got.Proof)
	}
}

func TestEventsFromWriteWorkflowRunBlockedProjectsBlocked(t *testing.T) {
	run := types.WriteWorkflowRun{
		RunID:         "wf-blocked",
		Status:        types.WriteWorkflowRunBlocked,
		ActiveBatchID: "batch-1",
		Batches: []types.WriteWorkflowBatch{{
			ID:     "batch-1",
			Status: types.WriteWorkflowBatchBlocked,
		}},
	}
	got := ReduceEvents(EventsFromWriteWorkflowRun(run))
	if got.Status != LoopRunStatusBlocked {
		t.Fatalf("status = %s, want blocked", got.Status)
	}
	if len(got.Units) != 1 || got.Units[0].Status != RuntimeUnitBlocked {
		t.Fatalf("unit view = %+v, want blocked", got.Units)
	}
}
