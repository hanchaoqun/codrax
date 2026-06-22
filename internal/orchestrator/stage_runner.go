package orchestrator

import (
	"fmt"
	"time"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/types"
)

// StageExecutionRequest is the lightweight seam between the read DAG
// scheduler and a concrete stage dispatch. It carries typed dispatch
// metadata only; prompt text, model prose, and rendered retry hints are
// deliberately outside this request.
type StageExecutionRequest struct {
	Stage               types.PipelineStage
	Window              []*types.TaskNode
	ExploreDispatchKey  string
	ExploreDispatchKind types.TaskNodeType
	ExploreToolSurface  types.ExploreToolSurface
	RetryHint           string
	ReasonCode          string
}

type StageExecutionResult struct {
	Stage   types.PipelineStage
	Output  *agent.StageOutput
	Err     error
	Elapsed time.Duration
}

func newExploreStageExecutionRequest(window []*types.TaskNode) StageExecutionRequest {
	return StageExecutionRequest{
		Stage:               types.StageExplore,
		Window:              append([]*types.TaskNode(nil), window...),
		ExploreDispatchKey:  exploreDispatchKeyForWindow(window),
		ExploreDispatchKind: exploreDispatchKindForWindow(window),
		ExploreToolSurface:  exploreToolSurfaceForWindow(window),
		ReasonCode:          "read_dag_explore_window",
	}
}

func newParallelExploreStageExecutionRequest(window []*types.TaskNode, retryHint string) StageExecutionRequest {
	req := newExploreStageExecutionRequest(window)
	req.RetryHint = retryHint
	req.ReasonCode = "read_dag_parallel_explore_window"
	return req
}

func newExtractStageExecutionRequest(node *types.TaskNode, reasonCode string) StageExecutionRequest {
	return newNodeStageExecutionRequest(types.StageExtract, node, reasonCode, "read_dag_extract")
}

func newFinalizeStageExecutionRequest(node *types.TaskNode, reasonCode string) StageExecutionRequest {
	return newNodeStageExecutionRequest(types.StageFinalize, node, reasonCode, "read_dag_finalize")
}

func newNodeStageExecutionRequest(stage types.PipelineStage, node *types.TaskNode, reasonCode, fallbackReasonCode string) StageExecutionRequest {
	if reasonCode == "" {
		reasonCode = fallbackReasonCode
	}
	req := StageExecutionRequest{
		Stage:      stage,
		ReasonCode: reasonCode,
	}
	if node != nil {
		req.Window = []*types.TaskNode{node}
	}
	return req
}

func (o *Orchestrator) executeStageRequest(req StageExecutionRequest) StageExecutionResult {
	start := time.Now()
	if o == nil || o.busCtx == nil {
		return StageExecutionResult{
			Stage:   req.Stage,
			Err:     fmt.Errorf("stage execution request requires orchestrator bus context"),
			Elapsed: time.Since(start),
		}
	}
	switch req.Stage {
	case types.StageExplore:
		return o.executeExploreStageRequest(req, start)
	default:
		out, err := o.dispatchStage(req.Stage)
		return StageExecutionResult{Stage: req.Stage, Output: out, Err: err, Elapsed: time.Since(start)}
	}
}

func (o *Orchestrator) executeExploreStageRequest(req StageExecutionRequest, start time.Time) StageExecutionResult {
	prevDispatchKey := o.busCtx.ExploreDispatchKey
	prevDispatchKind := o.busCtx.ExploreDispatchKind
	prevToolSurface := o.busCtx.ExploreToolSurface
	o.busCtx.ExploreDispatchKey = req.ExploreDispatchKey
	o.busCtx.ExploreDispatchKind = req.ExploreDispatchKind
	o.busCtx.ExploreToolSurface = req.ExploreToolSurface
	out, err := o.dispatchStage(types.StageExplore)
	o.busCtx.ExploreDispatchKey = prevDispatchKey
	o.busCtx.ExploreDispatchKind = prevDispatchKind
	o.busCtx.ExploreToolSurface = prevToolSurface
	return StageExecutionResult{Stage: types.StageExplore, Output: out, Err: err, Elapsed: time.Since(start)}
}
