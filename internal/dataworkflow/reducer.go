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
	ProgressEvents []ProgressEvent
	NoProgressStop int
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
		if WouldRepeatRelationNoProgress(input.Facts, input.ProgressEvents, action.Kind, input.NoProgressStop) {
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

type NextStageFallbackPlanInput struct {
	Current            dataquery.TaskPlan
	Coverage           dataquery.CoverageContract
	Output             dataquery.OutputContract
	Facts              StageFacts
	AllowedNextActions []string
	Artifacts          []ArtifactSchemaProjection
	ExtraScaffolds     []ActionScaffold
	ReasonPrefix       string
	SeenActionKeys     map[string]bool
	ProgressEvents     []ProgressEvent
	NoProgressStop     int
}

func BuildNextStageConcreteFallbackPlan(input NextStageFallbackPlanInput) (dataquery.TaskPlan, string, bool) {
	allowed := allowedActionSet(input.AllowedNextActions)
	if len(allowed) == 0 {
		return dataquery.TaskPlan{}, "", false
	}
	scaffolds := nextStageFallbackScaffolds(input.Facts, input.Artifacts, input.ExtraScaffolds, allowed)
	if len(scaffolds) == 0 {
		return dataquery.TaskPlan{}, "", false
	}
	return BuildConcreteFallbackPlan(ConcreteFallbackPlanInput{
		Current:        input.Current,
		Coverage:       input.Coverage,
		Output:         input.Output,
		Scaffolds:      scaffolds,
		Facts:          input.Facts,
		ReasonPrefix:   input.ReasonPrefix,
		SeenActionKeys: input.SeenActionKeys,
		ProgressEvents: input.ProgressEvents,
		NoProgressStop: input.NoProgressStop,
	})
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

func nextStageFallbackScaffolds(facts StageFacts, artifacts []ArtifactSchemaProjection, extra []ActionScaffold, allowed map[string]bool) []ActionScaffold {
	var scaffolds []ActionScaffold
	stage := facts.NextStage()
	switch stage {
	case StageNormalizeOrEnrichEntities:
		scaffolds = append(scaffolds, RelationActionScaffolds(artifacts, allowedActionList(allowed), 20)...)
	case StagePrepareContributionInputs, StageComputeContributions:
		scaffolds = append(scaffolds, RelationActionScaffolds(artifacts, allowedActionList(allowed), 24)...)
		scaffolds = append(scaffolds, extra...)
	default:
		scaffolds = append(scaffolds, extra...)
	}
	return filterAllowedScaffolds(scaffolds, allowed)
}

func filterAllowedScaffolds(scaffolds []ActionScaffold, allowed map[string]bool) []ActionScaffold {
	out := make([]ActionScaffold, 0, len(scaffolds))
	for _, scaffold := range scaffolds {
		kind := string(NormalizeActionKind(dataquery.DataActionKind(scaffold.Kind)))
		if kind != "" && allowed[kind] {
			out = append(out, scaffold)
		}
	}
	return out
}

func allowedActionSet(values []string) map[string]bool {
	out := map[string]bool{}
	for _, value := range cleanStrings(values) {
		kind := string(NormalizeActionKind(dataquery.DataActionKind(value)))
		if kind != "" {
			out[kind] = true
		}
	}
	return out
}

func allowedActionList(allowed map[string]bool) []string {
	out := make([]string, 0, len(allowed))
	for value := range allowed {
		if strings.TrimSpace(value) != "" {
			out = append(out, value)
		}
	}
	return out
}
