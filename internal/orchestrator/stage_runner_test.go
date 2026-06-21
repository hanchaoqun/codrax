package orchestrator

import (
	"errors"
	"sync"
	"testing"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestNewExploreStageExecutionRequestUsesTypedWindowHelpers(t *testing.T) {
	window := []*types.TaskNode{{
		ID:       "lens",
		Type:     types.NodeProbe,
		Optional: true,
		EntryConditions: []types.Criterion{{
			Kind: types.CritSourceInventoryLensMissing,
		}},
	}}
	req := newExploreStageExecutionRequest(window)
	if req.Stage != types.StageExplore ||
		req.ExploreDispatchKey != "lens" ||
		req.ExploreDispatchKind != types.NodeProbe ||
		req.ExploreToolSurface != types.ExploreToolSurfaceSourceInventoryLens ||
		req.ReasonCode != "read_dag_explore_window" {
		t.Fatalf("unexpected explore request: %+v", req)
	}
	if len(req.Window) != 1 || req.Window[0] != window[0] {
		t.Fatalf("request should carry the same window node refs: %+v", req.Window)
	}
}

func TestNewParallelExploreStageExecutionRequestCarriesRetryHint(t *testing.T) {
	window := []*types.TaskNode{{ID: "node-a", Type: types.NodeEvidence}}
	req := newParallelExploreStageExecutionRequest(window, "retry this slice")
	if req.Stage != types.StageExplore ||
		req.ExploreDispatchKey != "node-a" ||
		req.ExploreDispatchKind != types.NodeEvidence ||
		req.RetryHint != "retry this slice" ||
		req.ReasonCode != "read_dag_parallel_explore_window" {
		t.Fatalf("unexpected parallel explore request: %+v", req)
	}
}

func TestStageExecutionRequestInstallsAndRestoresExploreDispatchFields(t *testing.T) {
	var seenKey string
	var seenKind types.TaskNodeType
	var seenSurface types.ExploreToolSurface
	ar, sr, sar := buildRegistries(map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentExplorer: func(ctx *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
			seenKey = ctx.ExploreDispatchKey
			seenKind = ctx.ExploreDispatchKind
			seenSurface = ctx.ExploreToolSurface
			return &agent.StageOutput{MissingPiece: types.MissingNone}, nil
		},
	})
	o := New(types.PipelineSettings{}, ar, sr, sar)
	o.busCtx = &types.BusContext{
		Mutable:             types.NewMutableState("stage runner seam"),
		ExploreDispatchKey:  "prev-key",
		ExploreDispatchKind: types.NodeValidate,
		ExploreToolSurface:  types.ExploreToolSurfaceSourceInventoryLens,
		TaskState:           types.TaskState{Stage: types.StageExplore},
	}
	window := []*types.TaskNode{{ID: "evidence-node", Type: types.NodeEvidence}}

	out, err := o.dispatchExploreWindow(window)
	if err != nil {
		t.Fatalf("dispatchExploreWindow: %v", err)
	}
	if out == nil || out.MissingPiece != types.MissingNone {
		t.Fatalf("unexpected output: %+v", out)
	}
	if seenKey != "evidence-node" || seenKind != types.NodeEvidence || seenSurface != types.ExploreToolSurfaceDefault {
		t.Fatalf("agent saw wrong dispatch fields: key=%q kind=%q surface=%q", seenKey, seenKind, seenSurface)
	}
	if o.busCtx.ExploreDispatchKey != "prev-key" ||
		o.busCtx.ExploreDispatchKind != types.NodeValidate ||
		o.busCtx.ExploreToolSurface != types.ExploreToolSurfaceSourceInventoryLens {
		t.Fatalf("dispatch fields not restored: key=%q kind=%q surface=%q",
			o.busCtx.ExploreDispatchKey, o.busCtx.ExploreDispatchKind, o.busCtx.ExploreToolSurface)
	}
}

func TestStageExecutionRequestPreservesOutputAndError(t *testing.T) {
	wantErr := errors.New("stage failed")
	ar, sr, sar := buildRegistries(map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentExplorer: func(ctx *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
			return &agent.StageOutput{
				Error:        "partial output",
				MissingPiece: types.MissingFacts,
			}, wantErr
		},
	})
	o := New(types.PipelineSettings{}, ar, sr, sar)
	o.busCtx = &types.BusContext{
		Mutable:   types.NewMutableState("stage runner error"),
		TaskState: types.TaskState{Stage: types.StageExplore},
	}
	result := o.executeStageRequest(newExploreStageExecutionRequest([]*types.TaskNode{{ID: "e", Type: types.NodeEvidence}}))
	if !errors.Is(result.Err, wantErr) {
		t.Fatalf("result error = %v, want %v", result.Err, wantErr)
	}
	if result.Output == nil || result.Output.Error != "partial output" {
		t.Fatalf("partial output not preserved: %+v", result.Output)
	}
	if result.Stage != types.StageExplore || result.Elapsed <= 0 {
		t.Fatalf("result metadata not populated: %+v", result)
	}
}

func TestParallelStageExecutionRequestProjectsWorkerFields(t *testing.T) {
	type seenWorker struct {
		kind          types.TaskNodeType
		surface       types.ExploreToolSurface
		retryHint     string
		parallelGroup string
		laneLabels    []string
	}
	var mu sync.Mutex
	seen := map[string]seenWorker{}
	ar, sr, sar := buildRegistries(map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentExplorer: func(ctx *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
			mu.Lock()
			seen[ctx.ExploreDispatchKey] = seenWorker{
				kind:          ctx.ExploreDispatchKind,
				surface:       ctx.ExploreToolSurface,
				retryHint:     ctx.RetryHint,
				parallelGroup: ctx.ParallelGroupID,
				laneLabels:    ctx.ExploreLanePlan.Labels(),
			}
			mu.Unlock()
			return &agent.StageOutput{
				MissingPiece:  types.MissingFacts,
				SignalUpdates: &types.ExecutionSignals{HasEnoughFacts: false},
			}, nil
		},
	})
	o := New(types.PipelineSettings{MaxParallelism: 2}, ar, sr, sar)
	o.busCtx = &types.BusContext{
		Mutable:   types.NewMutableState("parallel stage runner seam"),
		TaskState: types.TaskState{Stage: types.StageExplore},
		ExploreLanePlan: types.ExploreLanePlan{Lanes: []types.ExploreLane{{
			ID:                  "lane-source",
			InvestigationUnitID: "subtopic-1",
			Origin:              types.AnswerEvidenceOriginCurrentSource,
			Role:                types.ExploreLaneRolePrincipal,
			HandoffPolicy:       types.ExploreLaneHandoffOwn,
		}}},
	}

	windows := [][]*types.TaskNode{
		{{
			ID:       "n_t0",
			Type:     types.NodeProbe,
			Optional: true,
			EntryConditions: []types.Criterion{{
				Kind: types.CritSourceInventoryLensMissing,
			}},
		}},
		{{ID: "plain", Type: types.NodeEvidence}},
	}
	if _, err := o.dispatchExploreWindowsParallel(windows, []string{"hint-a", "hint-b"}, 2); err != nil {
		t.Fatalf("dispatchExploreWindowsParallel: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	lens := seen["n_t0"]
	if lens.kind != types.NodeProbe || lens.surface != types.ExploreToolSurfaceSourceInventoryLens || lens.retryHint != "hint-a" {
		t.Fatalf("lens worker saw wrong request projection: %+v", lens)
	}
	if lens.parallelGroup == "" || len(lens.laneLabels) == 0 {
		t.Fatalf("lens worker should receive parallel group and scoped lane plan: %+v", lens)
	}
	plain := seen["plain"]
	if plain.kind != types.NodeEvidence || plain.surface != types.ExploreToolSurfaceDefault || plain.retryHint != "hint-b" {
		t.Fatalf("plain worker saw wrong request projection: %+v", plain)
	}
	if plain.parallelGroup == "" {
		t.Fatalf("plain worker should receive parallel group: %+v", plain)
	}
}
