package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/types"
)

// retry_render_invariant_test.go locks the contract:
// "EventTaskNodeEnd must NEVER fire on a node that the scheduler is
// about to requeue for retry."
//
// Regression bug: the user reported the stage row reading "已设计改动方案"
// (the localized DONE phrase) while the activity icon was still ⟳
// (running) during a plan retry. Tracing showed write_scheduler.go
// emitted EventTaskNodeEnd on transient retry + verify→plan SC retry,
// which set row.endTime != 0. The renderer's stagePhrase resolution
// reads endTime as "stage finished" and flips the label to the DONE
// triple even though the node is going to re-dispatch. The read
// scheduler intentionally avoids this — it never emits an end event
// for a requeue. This test pins the same invariant for write mode.

// TestWriteScheduler_TransientRetry_DoesNotEmitNodeEndBeforeRequeue
// reproduces the buggy sequence: plan dispatch fails with a
// transient-retryable stream stall, scheduler decides to retry. The
// renderer must NOT see any EventTaskNodeEnd between the failed
// dispatch and the next dispatch's EventTaskNodeStart.
func TestWriteScheduler_TransientRetry_DoesNotEmitNodeEndBeforeRequeue(t *testing.T) {
	expectedPlan := &types.ChangePlan{
		ID:          "plan-render-retry",
		TargetPaths: []string{"main.py"},
		Changes:     []types.FileChange{{Path: "main.py", Kind: "modify"}},
	}
	planCalls := 0
	agentFns := map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentAnalyzer: dagAnalyzerFn(dagIR(types.AnswerContract{Language: "en"})),
		types.AgentPlanner: func(ctx *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
			planCalls++
			if planCalls == 1 {
				return nil, &llm.StreamStalledError{
					IdleFor: 61 * time.Second,
					Cause:   context.Canceled,
				}
			}
			ctx.Mutable.SetChangePlan(expectedPlan)
			return &agent.StageOutput{MissingPiece: types.MissingNone}, nil
		},
	}

	// Capture the full event stream so we can replay the renderer's
	// view of node lifecycle and assert the invariant.
	var events []render.Event
	sink := func(e render.Event) {
		events = append(events, e)
	}

	ar, sr, sar := buildRegistries(agentFns)
	o := New(types.PipelineSettings{}, ar, sr, sar)
	o.SetMaxSteps(20)
	o.SetMode(types.ModePlan)
	o.SetAutoInitRepo(true)
	o.SetScaffoldEnabled(true)
	o.SetTransientRetryBudget(1)
	o.SetEmitter(sink)

	busCtx, _ := o.Run("retry render invariant", t.TempDir(), "main")

	// Sanity: retry actually happened (else the invariant is
	// vacuously satisfied and the test gives a false positive).
	if planCalls != 2 {
		t.Fatalf("setup: planner should have retried once; got %d calls", planCalls)
	}
	if busCtx.Mutable.ChangePlan() == nil {
		t.Fatal("setup: ChangePlan should be installed after successful retry")
	}

	// Walk the event stream. Between two adjacent EventTaskNodeStart
	// for the same nodeID, there must be NO EventTaskNodeEnd. Such
	// an end-then-start sequence is exactly the lying-state-row bug:
	// the renderer would freeze on the DONE label between the two.
	type seen struct {
		idx int
		ev  render.Event
	}
	starts := map[string][]seen{}
	ends := map[string][]seen{}
	for i, e := range events {
		switch e.Kind {
		case render.EventTaskNodeStart:
			starts[e.NodeID] = append(starts[e.NodeID], seen{i, e})
		case render.EventTaskNodeEnd:
			ends[e.NodeID] = append(ends[e.NodeID], seen{i, e})
		}
	}
	for nodeID, ss := range starts {
		if len(ss) < 2 {
			continue // node only dispatched once; nothing to check
		}
		// For every pair of consecutive starts (i.e. a retry), no
		// end event may appear between them.
		for i := 1; i < len(ss); i++ {
			prevStart, nextStart := ss[i-1], ss[i]
			for _, e := range ends[nodeID] {
				if e.idx > prevStart.idx && e.idx < nextStart.idx {
					t.Errorf(
						"node %s: EventTaskNodeEnd at event index %d "+
							"appeared between two consecutive "+
							"EventTaskNodeStart events (idx=%d → idx=%d). "+
							"This sets row.endTime != 0 in the renderer, "+
							"which makes stagePhrase resolve to the DONE "+
							"label while the node is actually about to "+
							"re-dispatch — the lying-state-row bug. "+
							"The retry path must NOT emit EventTaskNodeEnd; "+
							"the next EventTaskNodeStart owns the row's "+
							"transition back to the running state.",
						nodeID, e.idx, prevStart.idx, nextStart.idx,
					)
				}
			}
		}
	}
}
