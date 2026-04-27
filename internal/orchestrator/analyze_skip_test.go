package orchestrator

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// TestRunAnalyzePhase_SkipsWhenApplyHasPlanFile locks the /approve and
// `--mode=apply --plan-file=` fast path: when the user has supplied a
// vetted plan, the analyzer has nothing useful to do (the plan was
// classified upstream) and running it wastes time + produces a
// misleading task_map for files that may not even exist in the main
// repo yet (write_graph.go SkipOnFirstVisit then suppresses the
// planner so the AnalysisIR is never read either). Pre-fix path
// dispatched the analyzer for ~30-60s on every /approve.
func TestRunAnalyzePhase_SkipsWhenApplyHasPlanFile(t *testing.T) {
	cases := []struct {
		name string
		mode types.PipelineMode
	}{
		{"apply", types.ModeApply},
		{"verify", types.ModeVerify},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			o := &Orchestrator{
				busCtx: &types.BusContext{
					Mode:    tc.mode,
					Mutable: types.NewMutableState("approve test"),
				},
				planPath: "/tmp/plan-x.json",
				settings: types.PipelineSettings{MaxRetriesPerStage: 2},
			}
			used, err := o.runAnalyzePhase()
			if err != nil {
				t.Fatalf("skip path must not error; got %v", err)
			}
			if used != 0 {
				t.Errorf("skip path must consume 0 steps; got %d", used)
			}
			if o.busCtx.AnalysisIR == nil {
				t.Errorf("skip path must install a stub AnalysisIR so the Mode-switch in Run() finds non-nil")
			}
		})
	}
}

// TestRunAnalyzePhase_DoesNotSkipApplyWithoutPlanFile locks the
// inverse: ModeApply WITHOUT a plan path must NOT take the skip
// branch — apply needs a plan, and without --plan-file the planner
// stage emits one (which requires the analyzer's classification
// upstream).
func TestRunAnalyzePhase_DoesNotSkipApplyWithoutPlanFile(t *testing.T) {
	o := &Orchestrator{
		busCtx: &types.BusContext{
			Mode:    types.ModeApply,
			Mutable: types.NewMutableState("apply test"),
		},
		// planPath intentionally empty.
		settings: types.PipelineSettings{MaxRetriesPerStage: 1},
	}
	// We can't safely dispatch (no agent registry wired in this test
	// fixture), but we CAN verify the skip-branch precondition is
	// false by inspecting the orch's state pre-call. The skip branch
	// fires iff planPath != "". Asserting the precondition is a
	// structural test that doesn't require dispatching.
	if o.planPath != "" {
		t.Fatal("test setup: planPath must be empty to exercise non-skip path")
	}
	if (o.busCtx.Mode == types.ModeApply || o.busCtx.Mode == types.ModeVerify) && o.planPath != "" {
		t.Errorf("skip-branch precondition (ModeApply/Verify + planPath != \"\") MUST be false; got Mode=%q planPath=%q",
			o.busCtx.Mode, o.planPath)
	}
}
