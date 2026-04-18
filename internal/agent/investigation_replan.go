package agent

import (
	"fmt"

	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/types"
)

// investigation_replan.go — P4 dynamic mid-investigation replanning.
//
// MaybeRefinePlan checks whether the current investigation step has
// stalled (RetryCount exceeds threshold) and, if so, refines the
// plan by splitting the stalled step into more granular sub-steps.
//
// This is the "if the current approach isn't working, try a
// different decomposition" mechanism that prevents infinite retry
// loops on the same unachievable objective.

// replanRetryThreshold is the number of soft-stop retries on a
// single step before MaybeRefinePlan kicks in. Set to 3 so the LLM
// gets 3 attempts at each step before the system intervenes.
const replanRetryThreshold = 3

// MaybeRefinePlan checks if the current step has stalled and, if
// so, attempts to refine it. Returns true if the plan was modified
// (caller should re-read the current step). Returns false if no
// refinement was needed or possible.
func MaybeRefinePlan(plan *types.InvestigationPlan) bool {
	if plan == nil {
		return false
	}
	current := plan.CurrentStep()
	if current == nil || current.RetryCount < replanRetryThreshold {
		return false
	}

	switch current.Strategy {
	case types.StrategyTraceChain:
		return refineTraceChainStep(plan, current)
	case types.StrategyVerifyCondition:
		return refineVerifyConditionStep(plan, current)
	default:
		return false
	}
}

// refineTraceChainStep splits a stalled trace_chain step into:
//   sub-step A: locate ALL intermediate components (broaden search)
//   sub-step B: trace each hop individually (narrow + serial)
func refineTraceChainStep(plan *types.InvestigationPlan, step *types.InvestigationStep) bool {
	src := "source"
	tgt := "target"
	if len(step.Entities) >= 1 {
		src = step.Entities[0]
	}
	if len(step.Entities) >= 2 {
		tgt = step.Entities[1]
	}

	newSteps := []types.InvestigationStep{
		{
			ID: step.ID + "-a",
			Objective: fmt.Sprintf("扩大搜索：grep 所有提到 %s 和 %s 的文件，包括中间组件（orchestrator、runtime、dispatch）",
				src, tgt),
			Strategy:  types.StrategyLocate,
			Entities:  step.Entities,
			DependsOn: step.DependsOn,
			Status:    types.StepPending,
		},
		{
			ID:        step.ID + "-b",
			Objective: fmt.Sprintf("逐跳追踪：读取每个中间文件，找到从 %s 到 %s 的每一个调用/委托步骤", src, tgt),
			Strategy:  types.StrategyTraceChain,
			Entities:  step.Entities,
			DependsOn: []string{step.ID + "-a"},
			Status:    types.StepPending,
		},
	}

	// Mark the stalled step as satisfied (it produced partial
	// evidence that the refined sub-steps will build on) and
	// insert the new sub-steps after it.
	plan.MarkSatisfied(step.ID, "refined into sub-steps")
	plan.InsertAfter(step.ID, newSteps...)

	logging.Info("[planner] refined stalled step %s into %d sub-steps: %s",
		step.ID, len(newSteps), plan.Summary())
	return true
}

// refineVerifyConditionStep splits a stalled verify_condition step
// by adding an explicit locate step for the condition's definition.
func refineVerifyConditionStep(plan *types.InvestigationPlan, step *types.InvestigationStep) bool {
	newSteps := []types.InvestigationStep{
		{
			ID:        step.ID + "-a",
			Objective: "定位条件的判断逻辑所在文件和函数",
			Strategy:  types.StrategyLocate,
			Entities:  step.Entities,
			DependsOn: step.DependsOn,
			Status:    types.StepPending,
		},
		{
			ID:        step.ID + "-b",
			Objective: step.Objective + "（重试：先读条件的定义，再逐个验证）",
			Strategy:  types.StrategyVerifyCondition,
			Entities:  step.Entities,
			DependsOn: []string{step.ID + "-a"},
			Status:    types.StepPending,
		},
	}

	plan.MarkSatisfied(step.ID, "refined into sub-steps")
	plan.InsertAfter(step.ID, newSteps...)

	logging.Info("[planner] refined stalled verify_condition step %s into sub-steps: %s",
		step.ID, plan.Summary())
	return true
}
