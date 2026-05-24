package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"sort"
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
	index    int
	window   []*types.TaskNode
	lanePlan types.ExploreLanePlan
	output   *agent.StageOutput
	fork     *types.MutableState
	err      error
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
	o.busCtx.PipelineStage = types.StageExplore
	o.busCtx.ActiveAgent = types.AgentExplorer
	o.busCtx.TaskState.Stage = types.StageExplore
	groupID := fmt.Sprintf("explore:%d", time.Now().UnixNano())
	unitIDs := make([]string, len(windows))
	for i, w := range windows {
		unitIDs[i] = exploreDispatchKeyForWindow(w)
	}
	laneLabels := o.busCtx.ExploreLanePlan.Labels()
	lanePlans := scopeExploreLanePlansForWindows(o.busCtx.ExploreLanePlan, windows)
	o.emit(render.Event{
		Kind:               render.EventParallelDispatchStart,
		Timestamp:          time.Now(),
		Stage:              types.StageExplore,
		Agent:              types.AgentExplorer,
		ParallelGroupID:    groupID,
		ParallelTotal:      len(windows),
		Parallelism:        parallelism,
		ParallelUnitIDs:    unitIDs,
		ParallelLaneLabels: laneLabels,
	})
	defer o.emit(render.Event{
		Kind:            render.EventParallelDispatchEnd,
		Timestamp:       time.Now(),
		Stage:           types.StageExplore,
		Agent:           types.AgentExplorer,
		ParallelGroupID: groupID,
		ParallelTotal:   len(windows),
		Parallelism:     parallelism,
	})

	runCtx, cancel := context.WithCancel(o.CancelContext())
	defer cancel()
	allowEarlyConvergence := o.parallelExploreAllowsEarlyConvergence()

	jobs := make(chan int)
	resultCh := make(chan exploreParallelResult, len(windows))
	var wg sync.WaitGroup
	for w := 0; w < parallelism; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				var i int
				select {
				case <-runCtx.Done():
					return
				case next, ok := <-jobs:
					if !ok {
						return
					}
					i = next
				}
				hint := ""
				if i < len(hints) {
					hint = hints[i]
				}
				fork := o.busCtx.Mutable.ForkForExploreDispatch()
				unitID := unitIDs[i]
				var lanePlan types.ExploreLanePlan
				if i < len(lanePlans) {
					lanePlan = lanePlans[i]
				}
				o.emit(render.Event{
					Kind:               render.EventParallelDispatchUnitStart,
					Timestamp:          time.Now(),
					Stage:              types.StageExplore,
					Agent:              types.AgentExplorer,
					ParallelGroupID:    groupID,
					ParallelUnitID:     unitID,
					ParallelTotal:      len(windows),
					Parallelism:        parallelism,
					ParallelLaneLabels: lanePlan.Labels(),
				})
				out, err := o.runExploreAgentOnFork(runCtx, fork, hint, unitID, groupID, exploreDispatchKindForWindow(windows[i]), lanePlan)
				unitErr := ""
				if err != nil {
					unitErr = err.Error()
				} else if out != nil && out.Error != "" {
					unitErr = out.Error
				}
				o.emit(render.Event{
					Kind:            render.EventParallelDispatchUnitEnd,
					Timestamp:       time.Now(),
					Stage:           types.StageExplore,
					Agent:           types.AgentExplorer,
					ParallelGroupID: groupID,
					ParallelUnitID:  unitID,
					ParallelTotal:   len(windows),
					Parallelism:     parallelism,
					Error:           unitErr,
				})
				resultCh <- exploreParallelResult{
					index:    i,
					window:   windows[i],
					lanePlan: lanePlan,
					output:   out,
					fork:     fork,
					err:      err,
				}
			}
		}()
	}
	go func() {
		defer close(jobs)
		for i := range windows {
			select {
			case <-runCtx.Done():
				return
			case jobs <- i:
			}
		}
	}()
	go func() {
		wg.Wait()
		close(resultCh)
	}()

	results := make([]exploreParallelResult, 0, len(windows))
	earlyConverged := false
	winningConvergedIndex := -1
	collectiveConverged := false
	collectiveConvergedIndexes := map[int]bool{}
	handoffTracker := newExploreParallelHandoffTracker(lanePlans)
	for res := range resultCh {
		if allowEarlyConvergence && exploreParallelResultConverged(res) {
			if !earlyConverged {
				winningConvergedIndex = res.index
			}
			earlyConverged = true
			cancel()
		} else if !allowEarlyConvergence && handoffTracker.active() && exploreParallelResultConverged(res) {
			if handoffTracker.mark(res.index, res.lanePlan) {
				collectiveConverged = true
				for _, idx := range handoffTracker.convergedIndexes() {
					collectiveConvergedIndexes[idx] = true
				}
				cancel()
			}
		}
		results = append(results, res)
	}
	sort.SliceStable(results, func(i, j int) bool { return results[i].index < results[j].index })

	merged := &agent.StageOutput{
		MissingPiece:  types.MissingNone,
		SignalUpdates: &types.ExecutionSignals{HasEnoughFacts: true},
	}
	var firstErr error
	for i := range results {
		res := results[i]
		if earlyConverged && winningConvergedIndex >= 0 && res.index != winningConvergedIndex {
			if res.err != nil {
				if errors.Is(res.err, context.Canceled) {
					continue
				}
				logging.Warning("[orchestrator] parallel explore sibling ended after convergence: %v", res.err)
				continue
			}
			logging.Info("[orchestrator] skipping non-winning parallel explore sibling after accepted closure key=%s winner=%s",
				exploreDispatchKeyForWindow(res.window), unitIDs[winningConvergedIndex])
			continue
		}
		if collectiveConverged && !collectiveConvergedIndexes[res.index] {
			if res.err != nil {
				if errors.Is(res.err, context.Canceled) {
					continue
				}
				logging.Warning("[orchestrator] parallel explore sibling ended after typed-lane convergence: %v", res.err)
				continue
			}
			logging.Info("[orchestrator] skipping non-required parallel explore sibling after typed-lane convergence key=%s",
				exploreDispatchKeyForWindow(res.window))
			continue
		}
		if res.fork != nil {
			o.busCtx.Mutable.MergeExploreFork(res.fork)
		}
		if res.output != nil {
			o.applyStageOutput(res.output)
			mergeExploreParallelOutput(merged, res.output)
		}
		if res.err != nil {
			if earlyConverged && errors.Is(res.err, context.Canceled) {
				continue
			}
			if firstErr == nil {
				firstErr = res.err
			}
		}
	}
	if earlyConverged && merged.SignalUpdates != nil {
		merged.SignalUpdates.HasEnoughFacts = true
		merged.MissingPiece = types.MissingNone
	}
	if collectiveConverged && merged.SignalUpdates != nil {
		merged.SignalUpdates.HasEnoughFacts = true
		merged.MissingPiece = types.MissingNone
	}
	if firstErr != nil {
		return merged, firstErr
	}
	return merged, nil
}

func (o *Orchestrator) runExploreAgentOnFork(
	runCtx context.Context,
	mut *types.MutableState,
	hint string,
	dispatchKey string,
	parallelGroupID string,
	dispatchKind types.TaskNodeType,
	lanePlan types.ExploreLanePlan,
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
	workerBus.Ctx = runCtx
	workerBus.ActiveAgent = agentName
	workerBus.PipelineStage = stage
	workerBus.ExploreDispatchKey = dispatchKey
	workerBus.ExploreDispatchKind = dispatchKind
	if !lanePlan.Empty() {
		workerBus.ExploreLanePlan = lanePlan
	}
	workerBus.TaskState.Stage = stage
	workerBus.TaskState.RetryHint = hint
	workerBus.TaskState.LastError = ""
	agentCtx := ctxbuilder.BuildAgentContext(&workerBus, agentName, stage)
	agentCtx.ParallelGroupID = parallelGroupID
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

func scopeExploreLanePlansForWindows(plan types.ExploreLanePlan, windows [][]*types.TaskNode) []types.ExploreLanePlan {
	out := make([]types.ExploreLanePlan, len(windows))
	if plan.Empty() {
		return out
	}
	seenPrincipal := map[string]struct{}{}
	for i, window := range windows {
		scoped := scopeExploreLanePlanForWindow(plan, window)
		for j := range scoped.Lanes {
			key := scoped.Lanes[j].OwnershipKey()
			if key == "" {
				continue
			}
			if _, exists := seenPrincipal[key]; exists {
				scoped.Lanes[j].Role = types.ExploreLaneRoleSupport
				scoped.Lanes[j].HandoffPolicy = types.ExploreLaneHandoffSupport
				continue
			}
			seenPrincipal[key] = struct{}{}
		}
		out[i] = scoped
	}
	return out
}

func scopeExploreLanePlanForWindow(plan types.ExploreLanePlan, window []*types.TaskNode) types.ExploreLanePlan {
	if plan.Empty() || len(window) == 0 {
		return cloneExploreLanePlan(plan)
	}
	unitIDs := exploreLaneUnitIDsForWindow(window)
	if len(unitIDs) == 0 {
		return cloneExploreLanePlan(plan)
	}
	filtered := make([]types.ExploreLane, 0, len(plan.Lanes))
	for _, lane := range plan.Lanes {
		if unitIDs[strings.TrimSpace(lane.InvestigationUnitID)] {
			filtered = append(filtered, cloneExploreLane(lane))
		}
	}
	if len(filtered) == 0 {
		return cloneExploreLanePlan(plan)
	}
	return types.ExploreLanePlan{Lanes: filtered}
}

func cloneExploreLanePlan(plan types.ExploreLanePlan) types.ExploreLanePlan {
	if plan.Empty() {
		return types.ExploreLanePlan{}
	}
	out := make([]types.ExploreLane, 0, len(plan.Lanes))
	for _, lane := range plan.Lanes {
		out = append(out, cloneExploreLane(lane))
	}
	return types.ExploreLanePlan{Lanes: out}
}

func cloneExploreLane(lane types.ExploreLane) types.ExploreLane {
	lane.FacetIDs = append([]string(nil), lane.FacetIDs...)
	lane.DimensionLabels = append([]string(nil), lane.DimensionLabels...)
	return lane
}

func exploreLaneUnitIDsForWindow(window []*types.TaskNode) map[string]bool {
	out := map[string]bool{}
	for _, n := range window {
		if n == nil {
			continue
		}
		if id, ok := exploreLaneUnitIDFromEvidenceNodeID(n.ID); ok {
			out[id] = true
		}
	}
	return out
}

func exploreLaneUnitIDFromEvidenceNodeID(id string) (string, bool) {
	id = strings.TrimSpace(id)
	if id == "" {
		return "", false
	}
	idx := strings.LastIndex(id, "_t")
	if idx < 0 || idx+2 >= len(id) {
		return "", false
	}
	n := 0
	for _, r := range id[idx+2:] {
		if r < '0' || r > '9' {
			return "", false
		}
		n = n*10 + int(r-'0')
	}
	return fmt.Sprintf("subtopic-%d", n+1), true
}

type exploreParallelHandoffTracker struct {
	required map[string]struct{}
	covered  map[string]struct{}
	indexes  map[int]struct{}
}

func newExploreParallelHandoffTracker(plans []types.ExploreLanePlan) *exploreParallelHandoffTracker {
	required := map[string]struct{}{}
	for _, plan := range plans {
		for _, key := range exploreParallelRequiredLaneKeys(plan) {
			required[key] = struct{}{}
		}
	}
	if len(required) == 0 {
		return &exploreParallelHandoffTracker{}
	}
	return &exploreParallelHandoffTracker{
		required: required,
		covered:  map[string]struct{}{},
		indexes:  map[int]struct{}{},
	}
}

func (t *exploreParallelHandoffTracker) active() bool {
	return t != nil && len(t.required) > 0
}

func (t *exploreParallelHandoffTracker) mark(index int, plan types.ExploreLanePlan) bool {
	if !t.active() {
		return false
	}
	var matched bool
	for _, key := range exploreParallelRequiredLaneKeys(plan) {
		if _, ok := t.required[key]; !ok {
			continue
		}
		t.covered[key] = struct{}{}
		matched = true
	}
	if matched {
		t.indexes[index] = struct{}{}
	}
	return len(t.covered) >= len(t.required)
}

func (t *exploreParallelHandoffTracker) convergedIndexes() []int {
	if t == nil || len(t.indexes) == 0 {
		return nil
	}
	out := make([]int, 0, len(t.indexes))
	for idx := range t.indexes {
		out = append(out, idx)
	}
	sort.Ints(out)
	return out
}

func exploreParallelRequiredLaneKeys(plan types.ExploreLanePlan) []string {
	if plan.Empty() {
		return nil
	}
	out := make([]string, 0, len(plan.Lanes))
	seen := map[string]struct{}{}
	for _, lane := range plan.Lanes {
		if lane.HandoffPolicy != types.ExploreLaneHandoffOwn && lane.Role != types.ExploreLaneRolePrincipal {
			continue
		}
		key := strings.TrimSpace(lane.OwnershipKey())
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

func exploreParallelResultConverged(res exploreParallelResult) bool {
	if res.fork != nil && res.fork.IsInvestigationComplete() {
		return true
	}
	return false
}

func (o *Orchestrator) parallelExploreAllowsEarlyConvergence() bool {
	if o == nil || o.busCtx == nil || o.busCtx.AnalysisIR == nil {
		return true
	}
	rm := o.busCtx.AnalysisIR.RequestModel
	if types.HistoryLookupPrefersVCSNarrativePrincipal(rm, &o.busCtx.AnalysisIR.AnswerContract) {
		return true
	}
	if parallelExploreMustWaitForSiblingHandoffs(rm, &o.busCtx.AnalysisIR.AnswerContract) {
		return false
	}
	return true
}

func parallelExploreMustWaitForSiblingHandoffs(rm types.RequestModel, contract *types.AnswerContract) bool {
	if parallelExploreMixedOriginNeedsSiblingHandoffs(rm, contract) {
		return true
	}
	if hasExplicitQuestionStructureObligation(rm) {
		return true
	}
	if types.RequiresExhaustiveEnumerationMemberSetHandoff(rm) ||
		types.RequiresRelationMemberSetHandoff(rm) ||
		rm.Intent == types.IntentEnumerate {
		return true
	}
	if contract != nil && contract.Diagram != nil && contract.Diagram.Required {
		return true
	}
	if rm.ChangeImpactProfile != nil && rm.ChangeImpactProfile.Active() {
		return true
	}
	if rm.FieldValueProfile != nil && rm.FieldValueProfile.Active() {
		return true
	}
	// Diagnostic / current-status flags describe answer semantics, not a
	// principal sibling-handoff shape by themselves. The accepted
	// emit_investigation_complete pre-complete gates already protect precise
	// obligations such as current-source required files, evidence grounding,
	// and runtime-artifact boundaries. Treating a broad diagnostic flag as a
	// hard wait made parallel exploration duplicate accepted closures and widen
	// into adjacent details. Explicit buckets, exhaustive sets, relation sets,
	// field/value, and change-impact contracts above remain the typed blockers.
	return false
}

func parallelExploreMixedOriginNeedsSiblingHandoffs(rm types.RequestModel, contract *types.AnswerContract) bool {
	intentContract := types.CompileAnswerIntentContract(rm, contract)
	if !intentContract.HasOrigin(types.AnswerEvidenceOriginCurrentSource) {
		return false
	}
	hasNonSource := false
	for _, origin := range intentContract.Origins {
		if origin != types.AnswerEvidenceOriginUnknown && origin != types.AnswerEvidenceOriginCurrentSource {
			hasNonSource = true
			break
		}
	}
	if !hasNonSource {
		return false
	}
	for _, output := range intentContract.RequestedOutputs {
		switch output {
		case types.AnswerRequestedOutputMechanism,
			types.AnswerRequestedOutputTrace,
			types.AnswerRequestedOutputDiagram,
			types.AnswerRequestedOutputDiagnostic,
			types.AnswerRequestedOutputChangeImpact,
			types.AnswerRequestedOutputComparison,
			types.AnswerRequestedOutputEnumeration,
			types.AnswerRequestedOutputAbsence:
			return true
		}
	}
	return false
}

func hasExplicitQuestionStructureObligation(rm types.RequestModel) bool {
	if rm.EnumerationBoundary != nil && rm.EnumerationBoundary.DeclaredCount > 0 {
		return true
	}
	if rm.CompletenessObligation.IsActive() {
		return true
	}
	return len(rm.Buckets) >= 2
}

func (o *Orchestrator) applyExploreIterationScaling(agentCtx *types.AgentContext) {
	if o == nil || agentCtx == nil || o.busCtx == nil || o.busCtx.AnalysisIR == nil {
		return
	}
	nSub := len(o.busCtx.AnalysisIR.RequestModel.SubTopics)
	if nSub <= 1 {
		applyExploreLaneHandoffIterationCap(agentCtx)
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
	applyExploreLaneHandoffIterationCap(agentCtx)
}

func applyExploreLaneHandoffIterationCap(agentCtx *types.AgentContext) {
	if agentCtx == nil || agentCtx.ExploreLanePlan.Empty() {
		return
	}
	hasPrincipalOwner := false
	hasHandoffOnlyLane := false
	for _, lane := range agentCtx.ExploreLanePlan.Lanes {
		if lane.HandoffPolicy == types.ExploreLaneHandoffOwn || lane.Role == types.ExploreLaneRolePrincipal {
			hasPrincipalOwner = true
			break
		}
		switch lane.HandoffPolicy {
		case types.ExploreLaneHandoffSupport, types.ExploreLaneHandoffVerify, types.ExploreLaneHandoffDelay:
			hasHandoffOnlyLane = true
		}
	}
	if hasPrincipalOwner || !hasHandoffOnlyLane {
		return
	}
	const supportHandoffIterCap = 8
	if agentCtx.MaxIterOverride <= 0 || agentCtx.MaxIterOverride > supportHandoffIterCap {
		agentCtx.MaxIterOverride = supportHandoffIterCap
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

func exploreDispatchKindForWindow(window []*types.TaskNode) types.TaskNodeType {
	var kind types.TaskNodeType
	for _, n := range window {
		if n == nil || n.Type == "" {
			continue
		}
		if kind == "" {
			kind = n.Type
			continue
		}
		if kind != n.Type {
			return ""
		}
	}
	return kind
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
