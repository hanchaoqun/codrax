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
		} {
			if !strings.Contains(got, want) {
				t.Fatalf("%s: guidance missing %q:\n%s", lang, want, got)
			}
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
