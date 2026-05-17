package orchestrator

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestDispatchExploreWindowsParallel_CancelsSiblingAfterConvergence(t *testing.T) {
	var slowStarted sync.Once
	slowStartedCh := make(chan struct{})
	var slowCanceled int32

	ar, sr, sar := buildRegistries(map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentExplorer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			switch ctx.ExploreDispatchKey {
			case "done":
				<-slowStartedCh
				ctx.Mutable.SetInvestigationComplete("parallel branch reached terminal evidence")
				return &agent.StageOutput{
					MissingPiece:  types.MissingNone,
					SignalUpdates: &types.ExecutionSignals{HasEnoughFacts: true},
				}, nil
			case "slow":
				slowStarted.Do(func() { close(slowStartedCh) })
				<-ctx.Context().Done()
				atomic.StoreInt32(&slowCanceled, 1)
				return nil, ctx.Context().Err()
			default:
				t.Fatalf("unexpected dispatch key %q", ctx.ExploreDispatchKey)
				return nil, nil
			}
		},
	})
	o := New(types.PipelineSettings{MaxParallelism: 2}, ar, sr, sar)
	o.busCtx = &types.BusContext{
		Mutable: types.NewMutableState("parallel explore cancellation"),
		Signals: types.ExecutionSignals{},
	}

	out, err := o.dispatchExploreWindowsParallel([][]*types.TaskNode{
		{{ID: "done"}},
		{{ID: "slow"}},
	}, nil, 2)
	if err != nil {
		t.Fatalf("terminal branch should absorb canceled sibling, got error: %v", err)
	}
	if out == nil || out.SignalUpdates == nil || !out.SignalUpdates.HasEnoughFacts {
		t.Fatalf("merged output should preserve terminal enough-facts signal, got %+v", out)
	}
	if !o.busCtx.Mutable.IsInvestigationComplete() {
		t.Fatal("terminal fork state was not merged into parent mutable state")
	}
	if atomic.LoadInt32(&slowCanceled) != 1 {
		t.Fatal("running sibling explorer did not observe cancellation after convergence")
	}
}

func TestDispatchExploreWindowsParallel_PropagatesErrorWithoutConvergence(t *testing.T) {
	ar, sr, sar := buildRegistries(map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentExplorer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			return nil, context.Canceled
		},
	})
	o := New(types.PipelineSettings{MaxParallelism: 2}, ar, sr, sar)
	o.busCtx = &types.BusContext{
		Mutable: types.NewMutableState("parallel explore cancellation"),
		Signals: types.ExecutionSignals{},
	}

	if _, err := o.dispatchExploreWindowsParallel([][]*types.TaskNode{
		{{ID: "a"}},
		{{ID: "b"}},
	}, nil, 2); err == nil {
		t.Fatal("non-converged parallel dispatch should still propagate worker errors")
	}
}
