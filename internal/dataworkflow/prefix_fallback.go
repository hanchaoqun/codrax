package dataworkflow

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/dataquery"
)

type InitialRankPrefixFallbackInput struct {
	Plan                dataquery.TaskPlan
	HasSuccessfulResult bool
}

type StagePrefixFallbackInput struct {
	Plan                dataquery.TaskPlan
	State               WorkflowStateView
	Guard               GuardResult
	HasSuccessfulResult bool
}

type ExecutablePrefixFallbackInput struct {
	Plan         dataquery.TaskPlan
	Guard        GuardResult
	StagingGuard func(dataquery.TaskPlan) GuardResult
}

func InitialRankPrefixFallback(input InitialRankPrefixFallbackInput) (dataquery.TaskPlan, dataquery.TaskPlan, bool) {
	plan := input.Plan
	if len(plan.Actions) < 2 || input.HasSuccessfulResult {
		return dataquery.TaskPlan{}, dataquery.TaskPlan{}, false
	}
	prefix, rest, split := SplitInitialDiscoveryDependencyRank(plan.Actions)
	if !split || len(prefix) == 0 || len(rest) == 0 {
		return dataquery.TaskPlan{}, dataquery.TaskPlan{}, false
	}
	out := plan
	out.Actions = prefix
	out.Script = ""
	out.ContinueAfter = true
	if strings.TrimSpace(out.WhyThisBatch) == "" {
		out.WhyThisBatch = "execute the first typed data action rank before downstream graph stages"
	}
	if strings.TrimSpace(out.NextBatch) == "" {
		out.NextBatch = "continue with the deferred typed data action ranks after this batch materializes artifacts"
	}
	remainder := plan
	remainder.Actions = rest
	remainder.Script = ""
	remainder.ContinueAfter = true
	return out, remainder, true
}

func IntraBatchDependencyPrefixFallback(plan dataquery.TaskPlan) (dataquery.TaskPlan, dataquery.TaskPlan, bool) {
	dependencyIndex, _, _, ok := FirstIntraBatchDependency(plan.Actions)
	if !ok || dependencyIndex <= 0 || dependencyIndex >= len(plan.Actions) {
		return dataquery.TaskPlan{}, dataquery.TaskPlan{}, false
	}
	out := plan
	out.Actions = append([]dataquery.DataAction(nil), plan.Actions[:dependencyIndex]...)
	out.Script = ""
	out.ContinueAfter = true
	if strings.TrimSpace(out.WhyThisBatch) == "" {
		out.WhyThisBatch = "execute the producer action rank before consuming its generated artifact"
	}
	if strings.TrimSpace(out.NextBatch) == "" {
		out.NextBatch = "continue with the deferred dependent action after real artifact fields are available"
	}
	remainder := plan
	remainder.Actions = append([]dataquery.DataAction(nil), plan.Actions[dependencyIndex:]...)
	remainder.Script = ""
	remainder.ContinueAfter = true
	return out, remainder, true
}

func FirstIntraBatchDependency(actions []dataquery.DataAction) (int, string, dataquery.DataAction, bool) {
	produced := map[string]dataquery.DataAction{}
	for i, action := range actions {
		for _, input := range cleanStrings(action.InputPaths) {
			key := normalizeAccessPath(input)
			if key == "" {
				continue
			}
			if producer, ok := produced[key]; ok {
				return i, input, producer, true
			}
		}
		for _, alias := range ActionOutputAliases(action) {
			key := normalizeAccessPath(alias)
			if key != "" {
				produced[key] = action
			}
		}
	}
	return 0, "", dataquery.DataAction{}, false
}

func StagePrefixFallbackWithRemainder(input StagePrefixFallbackInput) (dataquery.TaskPlan, dataquery.TaskPlan, bool) {
	plan := input.Plan
	if len(plan.Actions) < 2 || input.Guard.Empty() || !input.HasSuccessfulResult {
		return dataquery.TaskPlan{}, dataquery.TaskPlan{}, false
	}
	state := input.State
	if len(state.ComputedAllowedNextActions()) == 0 || len(state.MissingValidationStages()) <= 1 {
		return dataquery.TaskPlan{}, dataquery.TaskPlan{}, false
	}
	keep, rest := SplitActionRankForState(plan.Actions, state)
	if len(keep) == 0 || len(keep) == len(plan.Actions) {
		return dataquery.TaskPlan{}, dataquery.TaskPlan{}, false
	}
	out := plan
	out.Actions = keep
	out.Script = ""
	out.ContinueAfter = true
	if strings.TrimSpace(out.WhyThisBatch) == "" {
		out.WhyThisBatch = "execute the typed action prefix for the current data workflow stage"
	}
	if strings.TrimSpace(out.NextBatch) == "" {
		out.NextBatch = "continue with the remaining data workflow stages after this prefix produces reusable artifacts"
	}
	remainder := plan
	remainder.Actions = rest
	remainder.Script = ""
	remainder.ContinueAfter = true
	return out, remainder, true
}

func ExecutablePrefixFallback(input ExecutablePrefixFallbackInput) (dataquery.TaskPlan, bool) {
	plan := input.Plan
	if len(plan.Actions) < 2 || input.Guard.Empty() || input.StagingGuard == nil {
		return dataquery.TaskPlan{}, false
	}
	if input.StagingGuard(plan).Empty() {
		return dataquery.TaskPlan{}, false
	}
	for prefixLen := len(plan.Actions) - 1; prefixLen >= 1; prefixLen-- {
		out := plan
		out.Actions = append([]dataquery.DataAction(nil), plan.Actions[:prefixLen]...)
		out.Script = ""
		out.ContinueAfter = true
		if strings.TrimSpace(out.WhyThisBatch) == "" {
			out.WhyThisBatch = "execute the valid typed data action prefix before repairing the rejected suffix"
		}
		if strings.TrimSpace(out.NextBatch) == "" {
			out.NextBatch = "continue from the prefix result and replan the invalid suffix against real artifact fields"
		}
		if input.StagingGuard(out).Empty() {
			return out, true
		}
	}
	return dataquery.TaskPlan{}, false
}

func SplitActionRankForState(actions []dataquery.DataAction, state WorkflowStateView) ([]dataquery.DataAction, []dataquery.DataAction) {
	keep := make([]dataquery.DataAction, 0, len(actions))
	firstRank := 0
	for _, action := range actions {
		kind := NormalizeActionKind(action.Kind)
		if !WorkflowActionKindAllowed(kind, state) {
			break
		}
		if kind == dataquery.DataActionCustomTransform && strings.TrimSpace(action.Script) != "" {
			break
		}
		rank := DependencyRank(kind)
		if rank > 0 {
			if firstRank == 0 {
				firstRank = rank
			} else if rank != firstRank {
				break
			}
		}
		keep = append(keep, action)
	}
	if len(keep) == 0 {
		return nil, append([]dataquery.DataAction(nil), actions...)
	}
	rest := append([]dataquery.DataAction(nil), actions[len(keep):]...)
	return keep, rest
}

func WorkflowActionKindAllowed(kind dataquery.DataActionKind, state WorkflowStateView) bool {
	normalized := NormalizeActionKind(kind)
	for _, action := range state.ComputedAllowedNextActions() {
		if normalized == dataquery.DataActionKind(strings.TrimSpace(action)) {
			return true
		}
	}
	return false
}
