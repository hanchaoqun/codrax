package dataworkflow

import (
	"path"
	"sort"
	"strings"

	"github.com/hanchaoqun/codrax/internal/dataquery"
)

type ActionInputAvailabilityGuardInput struct {
	Records     []WorkflowRecord
	Plan        dataquery.TaskPlan
	Action      dataquery.DataAction
	ActionIndex int
}

func ActionInputAvailabilityGuardResult(input ActionInputAvailabilityGuardInput) GuardResult {
	if !WorkflowHasSuccessfulResult(input.Records) {
		return GuardResult{}
	}
	kind := NormalizeActionKind(input.Action.Kind)
	switch kind {
	case dataquery.DataActionMaterialInventory,
		dataquery.DataActionInspectMaterial,
		dataquery.DataActionExtractRecords,
		dataquery.DataActionDeriveRules:
		return GuardResult{}
	}
	inputs := cleanStrings(input.Action.InputPaths)
	if len(inputs) == 0 {
		return GuardResult{}
	}
	available := AvailableActionInputPaths(input.Records, input.Plan, input.ActionIndex)
	var missing []string
	for _, actionInput := range inputs {
		if CoveragePathCovered(available, actionInput) {
			continue
		}
		missing = append(missing, actionInput)
	}
	if len(missing) == 0 {
		return GuardResult{}
	}
	sort.Strings(missing)
	return UnavailableActionInputGuardResult(UnavailableActionInputGuardInput{
		Action:      input.Action,
		ActionIndex: input.ActionIndex,
		Missing:     cleanStrings(missing),
	})
}

func WorkflowHasSuccessfulResult(records []WorkflowRecord) bool {
	for _, rec := range records {
		if rec.Result != nil && strings.TrimSpace(rec.Err) == "" {
			return true
		}
	}
	return false
}

func AvailableActionInputPaths(records []WorkflowRecord, plan dataquery.TaskPlan, beforeActionIndex int) map[string]bool {
	available := map[string]bool{}
	mark := func(values []string) {
		for _, value := range cleanStrings(values) {
			if normalized := normalizeCoveragePath(value); normalized != "" {
				available[normalized] = true
			}
		}
	}
	var markArtifact func(dataquery.DataArtifact)
	markArtifact = func(artifact dataquery.DataArtifact) {
		mark(artifact.SourcePaths)
		mark(ArtifactAliases(artifact))
		for _, child := range artifact.Children {
			markArtifact(child)
		}
	}
	for _, rec := range records {
		if rec.Result == nil {
			continue
		}
		mark(rec.Result.ConsumedPaths)
		for _, artifact := range rec.Result.Artifacts {
			markArtifact(artifact)
		}
	}
	mark(topLevelExternalMaterialInputs(plan.InputPaths))
	if beforeActionIndex > len(plan.Actions) {
		beforeActionIndex = len(plan.Actions)
	}
	for i := 0; i < beforeActionIndex; i++ {
		action := plan.Actions[i]
		switch NormalizeActionKind(action.Kind) {
		case dataquery.DataActionCustomTransform, dataquery.DataActionMaterialInventory:
			continue
		default:
			mark(action.InputPaths)
			mark(ActionOutputAliases(action))
		}
	}
	return available
}

func ActionOutputAliases(action dataquery.DataAction) []string {
	var out []string
	if id := strings.TrimSpace(action.ID); id != "" {
		out = append(out, id)
	}
	if artifact := strings.TrimSpace(action.OutputArtifact); artifact != "" {
		out = append(out, artifact, path.Base(artifact))
	}
	return cleanStrings(out)
}

func topLevelExternalMaterialInputs(values []string) []string {
	var out []string
	for _, value := range cleanStrings(values) {
		if pathLooksLikeExternalMaterial(value) {
			out = append(out, value)
		}
	}
	return out
}

func pathLooksLikeExternalMaterial(value string) bool {
	value = strings.TrimSpace(strings.ReplaceAll(value, "\\", "/"))
	if value == "" {
		return false
	}
	if strings.HasPrefix(value, "/") || strings.HasPrefix(value, "./") || strings.HasPrefix(value, "../") {
		return true
	}
	if strings.Contains(value, "/") {
		return true
	}
	return strings.TrimSpace(path.Ext(value)) != ""
}
