package orchestrator

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestPublishBlockedRunGuidance_SurfacesArtifactsAndVerbs(t *testing.T) {
	for _, lang := range []string{"zh", "en"} {
		mu := types.NewMutableState("guidance")
		o := &Orchestrator{busCtx: &types.BusContext{Mutable: mu, Language: lang}, reportDir: "/tmp/plans"}
		run := &types.WriteWorkflowRun{
			RunID:         "wf-guid-1",
			ActiveBatchID: "batch-1",
			Batches: []types.WriteWorkflowBatch{{
				ID: "batch-1", Status: types.WriteWorkflowBatchBlocked, PlanID: "plan-g1",
				Attempts: []types.WriteWorkflowAttempt{
					{Kind: "plan", Status: "complete", PlanID: "plan-g1"},
					{Kind: "apply", Status: "applied", PlanID: "plan-g1"},
					{Kind: "verify", Status: "failed", ReasonCode: "tests_failed", PlanID: "plan-g1",
						ReportID: "plan-g1.report.json", ArtifactRef: "plan-g1.attempt-1.diff"},
				},
			}},
		}
		o.publishBlockedRunGuidance(run, "verify_retry_budget_exhausted")
		got := mu.Result()
		for _, want := range []string{
			"plan-g1.report.json",
			"plan-g1.attempt-1.diff",
			"refs/codrax/applied/plan-g1",
			"git cherry-pick",
			"/workflow show wf-guid-1",
			"pipeline_write_retry_budget",
			"Proof authority: state=`failed` reason=`tests_failed` action=`repair`",
		} {
			if !strings.Contains(got, want) {
				t.Fatalf("%s: guidance missing %q:\n%s", lang, want, got)
			}
		}
	}
}

func TestPublishBlockedRunGuidance_SurfacesSliceTruthAuthority(t *testing.T) {
	mu := types.NewMutableState("truth")
	o := &Orchestrator{busCtx: &types.BusContext{Mutable: mu, Language: "en"}}
	o.publishBlockedRunGuidance(&types.WriteWorkflowRun{
		RunID:         "wf-truth",
		Status:        types.WriteWorkflowRunBlocked,
		ActiveBatchID: "batch-1",
		Batches: []types.WriteWorkflowBatch{{
			ID:            "batch-1",
			Status:        types.WriteWorkflowBatchBlocked,
			ActiveSliceID: "slice-failed",
			Slices: []types.WriteWorkflowSlice{{
				ID:     "slice-ok",
				Status: types.ChangePlanSliceVerified,
			}, {
				ID:     "slice-failed",
				Status: types.ChangePlanSliceFailed,
				Attempts: []types.WriteWorkflowAttempt{{
					Kind:       "verify",
					Status:     "failed",
					ReasonCode: "tests_failed",
				}},
			}},
		}},
	}, "verify_retry_budget_exhausted")

	got := mu.Result()
	for _, want := range []string{
		"Workflow stopped",
		"Truth authority: state=`failed` reason=`tests_failed` action=`repair`",
		"/workflow show wf-truth",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("slice truth guidance missing %q:\n%s", want, got)
		}
	}
}

func TestPublishBlockedRunGuidance_AppendsToExistingResult(t *testing.T) {
	mu := types.NewMutableState("append")
	mu.SetResultPlain("existing block message")
	o := &Orchestrator{busCtx: &types.BusContext{Mutable: mu, Language: "en"}}
	o.publishBlockedRunGuidance(&types.WriteWorkflowRun{RunID: "wf-2"}, "budget_exhausted")
	got := mu.Result()
	if !strings.HasPrefix(got, "existing block message") || !strings.Contains(got, "pipeline_max_steps") {
		t.Fatalf("guidance must append after existing result, got %q", got)
	}
}

func TestPublishBlockedRunGuidance_PendingApprovalRendersPausedNotStopped(t *testing.T) {
	mu := types.NewMutableState("pending")
	o := &Orchestrator{busCtx: &types.BusContext{Mutable: mu, Language: "en"}}
	o.publishBlockedRunGuidance(&types.WriteWorkflowRun{
		RunID:         "wf-approval",
		Status:        types.WriteWorkflowRunInProgress,
		ActiveBatchID: "batch-1",
		Batches: []types.WriteWorkflowBatch{{
			ID:     "batch-1",
			Status: types.WriteWorkflowBatchPendingApproval,
			PlanID: "plan-approval",
			Attempts: []types.WriteWorkflowAttempt{{
				Kind:   "plan",
				Status: "complete",
				PlanID: "plan-approval",
			}},
		}},
	}, "pending_approval")
	got := mu.Result()
	for _, want := range []string{
		"Workflow paused — approval required",
		"Next: review the current batch risk",
		"plan `plan-approval`",
		"/workflow show wf-approval",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("pending approval guidance missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "Workflow stopped") {
		t.Fatalf("pending approval must not render as stopped:\n%s", got)
	}
}
