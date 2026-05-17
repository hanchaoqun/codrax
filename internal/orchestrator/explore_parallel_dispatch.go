package orchestrator

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/hanchaoqun/codrax/internal/agent"
	ctxbuilder "github.com/hanchaoqun/codrax/internal/context"
	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/types"
)

type exploreParallelResult struct {
	window []*types.TaskNode
	output *agent.StageOutput
	fork   *types.MutableState
	err    error
}

func (o *Orchestrator) dispatchExploreWindowsParallel(
	windows [][]*types.TaskNode,
	hints []string,
	parallelism int,
) (*agent.StageOutput, error) {
	if len(windows) == 0 {
		return nil, nil
	}
	if parallelism <= 1 {
		return nil, fmt.Errorf("parallel explorer dispatch called with parallelism=%d", parallelism)
	}
	results := make([]exploreParallelResult, len(windows))
	jobs := make(chan int)
	var wg sync.WaitGroup
	for w := 0; w < parallelism; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				hint := ""
				if i < len(hints) {
					hint = hints[i]
				}
				fork := o.busCtx.Mutable.ForkForExploreDispatch()
				out, err := o.runExploreAgentOnFork(fork, hint, exploreDispatchKeyForWindow(windows[i]))
				results[i] = exploreParallelResult{
					window: windows[i],
					output: out,
					fork:   fork,
					err:    err,
				}
			}
		}()
	}
	for i := range windows {
		jobs <- i
	}
	close(jobs)
	wg.Wait()

	merged := &agent.StageOutput{
		MissingPiece:  types.MissingNone,
		SignalUpdates: &types.ExecutionSignals{HasEnoughFacts: true},
	}
	for i := range results {
		res := results[i]
		if res.err != nil {
			return merged, res.err
		}
		if res.fork != nil {
			o.busCtx.Mutable.MergeExploreFork(res.fork)
		}
		if res.output == nil {
			continue
		}
		o.applyStageOutput(res.output)
		mergeExploreParallelOutput(merged, res.output)
	}
	return merged, nil
}

func (o *Orchestrator) runExploreAgentOnFork(
	mut *types.MutableState,
	hint string,
	dispatchKey string,
) (*agent.StageOutput, error) {
	stage := types.StageExplore
	if err := o.checkCanceled(string(stage), 0); err != nil {
		return nil, err
	}
	info, ok := pipelineTopology[stage]
	if !ok {
		return nil, fmt.Errorf("unknown pipeline stage: %s", stage)
	}
	agentName := info.Agent
	skillName := info.Skill
	ag, err := o.agents.Get(agentName)
	if err != nil {
		return nil, fmt.Errorf("get agent %s: %w", agentName, err)
	}
	sk, err := o.skills.Get(skillName)
	if err != nil {
		return nil, fmt.Errorf("get skill %s: %w", skillName, err)
	}

	workerBus := *o.busCtx
	workerBus.Mutable = mut
	workerBus.ActiveAgent = agentName
	workerBus.PipelineStage = stage
	workerBus.ExploreDispatchKey = dispatchKey
	workerBus.TaskState.Stage = stage
	workerBus.TaskState.RetryHint = hint
	workerBus.TaskState.LastError = ""
	agentCtx := ctxbuilder.BuildAgentContext(&workerBus, agentName, stage)
	if ta, ok := o.thinkAloudMap[agentName]; ok {
		agentCtx.ThinkAloud = ta
	}
	priorVisible := o.orchestratorPriorConvVisibleForStage(
		o.settings.Agent.PriorConvPolicy, stage, agentCtx.Objective)
	agentCtx.PriorConvHidden = !priorVisible
	o.applyExploreIterationScaling(agentCtx)

	logging.Info("[orchestrator] parallel explore dispatch key=%s skill=%s", dispatchKey, skillName)
	output, err := ag.Execute(agentCtx, sk)
	if err != nil {
		return output, fmt.Errorf("agent %s execution: %w", agentName, err)
	}
	if proposal := extractSubAgentProposal(output, agentName); proposal != nil {
		logging.Info("[orchestrator] parallel explore sub-agent proposal: %s (%d sub_tasks)", proposal.Reason, len(proposal.SubTasks))
		merged, runErr := o.subRuntime.Run(&workerBus, proposal)
		if runErr != nil {
			return output, runErr
		}
		output = merged
	}
	return output, nil
}

func (o *Orchestrator) applyExploreIterationScaling(agentCtx *types.AgentContext) {
	if o == nil || agentCtx == nil || o.busCtx == nil || o.busCtx.AnalysisIR == nil {
		return
	}
	nSub := len(o.busCtx.AnalysisIR.RequestModel.SubTopics)
	if nSub <= 1 {
		return
	}
	agentCfg := o.settings.Agent
	base := agentCfg.MaxIterations
	extra := nSub * agentCfg.SubTopicExplorerBudgetExtra
	adjusted := base + extra
	ceil := agentCfg.ExplorerScaledIterMax
	if ceil <= 0 {
		ceil = 35
	}
	if adjusted > ceil {
		adjusted = ceil
	}
	if adjusted > base {
		agentCtx.MaxIterOverride = adjusted
	}
}

func mergeExploreParallelOutput(dst, src *agent.StageOutput) {
	if dst == nil || src == nil {
		return
	}
	if src.MissingPiece == types.MissingFacts {
		dst.MissingPiece = types.MissingFacts
	}
	if strings.TrimSpace(dst.RetryHint) == "" && strings.TrimSpace(src.RetryHint) != "" {
		dst.RetryHint = src.RetryHint
	}
	if src.SignalUpdates == nil || !src.SignalUpdates.HasEnoughFacts {
		dst.SignalUpdates.HasEnoughFacts = false
	}
	if src.Error != "" {
		if dst.Error != "" {
			dst.Error += "; "
		}
		dst.Error += src.Error
	}
}

func exploreDispatchKeyForWindow(window []*types.TaskNode) string {
	if len(window) == 0 {
		return "explore"
	}
	ids := make([]string, 0, len(window))
	for _, n := range window {
		if n == nil || n.ID == "" {
			continue
		}
		ids = append(ids, n.ID)
	}
	if len(ids) == 0 {
		return "explore"
	}
	return strings.Join(ids, "+")
}

func emitParallelExploreStageStart(o *Orchestrator) time.Time {
	stageStart := time.Now()
	o.emit(render.Event{
		Kind:      render.EventStageStart,
		Timestamp: stageStart,
		Stage:     types.StageExplore,
		Agent:     types.AgentExplorer,
		Skill:     "explore-skill",
	})
	o.emit(render.Event{
		Kind:      render.EventSkillBound,
		Timestamp: stageStart,
		Stage:     types.StageExplore,
		Agent:     types.AgentExplorer,
		Skill:     "explore-skill",
	})
	return stageStart
}
