package dataworkflow

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/dataquery"
)

type ConcreteFallbackPlanInput struct {
	Current        dataquery.TaskPlan
	Coverage       dataquery.CoverageContract
	Output         dataquery.OutputContract
	Scaffolds      []ActionScaffold
	Facts          StageFacts
	ReasonPrefix   string
	SeenActionKeys map[string]bool
}

func BuildConcreteFallbackPlan(input ConcreteFallbackPlanInput) (dataquery.TaskPlan, string, bool) {
	if len(input.Scaffolds) == 0 {
		return dataquery.TaskPlan{}, "", false
	}
	reasonPrefix := strings.TrimSpace(input.ReasonPrefix)
	if reasonPrefix == "" {
		reasonPrefix = "workflow state requested a concrete typed action"
	}
	base := dataquery.TaskPlan{
		Status:           "ready",
		OutputContract:   input.Output,
		CoverageContract: input.Coverage,
		Goal:             strings.TrimSpace(input.Current.Goal),
		SuccessCriteria:  append([]string(nil), input.Current.SuccessCriteria...),
		ContinueAfter:    true,
	}
	if strings.TrimSpace(base.Goal) == "" {
		base.Goal = "continue the typed data workflow toward the user's requested output"
	}
	for _, scaffold := range ConcreteFallbackScaffolds(input.Scaffolds, input.Facts) {
		action, ok := ConcreteActionFromScaffold(scaffold)
		if !ok {
			continue
		}
		if input.SeenActionKeys[ActionIdempotencyKey(action)] {
			continue
		}
		plan := base
		plan.Actions = []dataquery.DataAction{action}
		plan.InputPaths = mergeActionInputPaths(plan.InputPaths, action.InputPaths)
		plan.WhyThisBatch = reasonPrefix + "; execute the next concrete typed action scaffold instead of another script or schema-only inspection"
		plan.NextBatch = "evaluate this materialized typed artifact, then continue with the next legal DAG stage"
		if purpose := strings.TrimSpace(action.Purpose); purpose != "" {
			plan.WhyThisBatch = reasonPrefix + "; " + purpose
		}
		return plan, reasonPrefix + "; converted to concrete typed action scaffold", true
	}
	return dataquery.TaskPlan{}, "", false
}

func ActionIdempotencyKeys(actions []dataquery.DataAction) map[string]bool {
	out := map[string]bool{}
	for _, action := range actions {
		key := ActionIdempotencyKey(action)
		if strings.TrimSpace(key) != "" {
			out[key] = true
		}
	}
	return out
}
