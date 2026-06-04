package orchestrator

import (
	"time"

	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/tool/repomap/multigraph"
	"github.com/hanchaoqun/codrax/internal/types"
)

func (o *Orchestrator) trySelectMultiRepoFocus(mg *multigraph.MultiGraph) *types.MultiRepoFocusDecision {
	if o == nil || o.busCtx == nil || o.busCtx.Mutable == nil || mg == nil || mg.IsSingle() || mg.Topology() == nil || len(mg.Topology().Repos) < 2 {
		return nil
	}
	if o.busCtx.Mode.Normalize() != types.ModeRead {
		return nil
	}
	prevStage := o.busCtx.PipelineStage
	prevTaskStage := o.busCtx.TaskState.Stage
	prevAgent := o.busCtx.ActiveAgent
	o.busCtx.Mutable.SetMultiRepoFocusDecision(nil)
	start := time.Now()
	logging.Info("multi-repo: selecting focus from compact topology before analyzer")
	out, err := o.dispatchStage(types.StageMultiRepoFocus)
	o.busCtx.PipelineStage = prevStage
	o.busCtx.TaskState.Stage = prevTaskStage
	o.busCtx.ActiveAgent = prevAgent
	if err != nil {
		logging.Warning("multi-repo: focus selector failed; using fallback preview: %v", err)
		return nil
	}
	if out != nil && out.Error != "" {
		logging.Warning("multi-repo: focus selector produced no usable recommendation; using fallback preview: %s", out.Error)
		return nil
	}
	decision := o.busCtx.Mutable.MultiRepoFocusDecision()
	if decision == nil {
		logging.Warning("multi-repo: focus selector completed without a decision; using fallback preview")
		return nil
	}
	logging.Info("multi-repo: focus selector selected %d sub-repo(s) in %s source=%s confidence=%.2f",
		len(decision.Candidates), time.Since(start).Round(time.Millisecond), decision.Source, decision.Confidence)
	return decision
}
