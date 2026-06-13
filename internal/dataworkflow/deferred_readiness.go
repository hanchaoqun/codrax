package dataworkflow

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/dataquery"
)

type DeferredActionReadinessInput struct {
	Records           []WorkflowRecord
	Action            dataquery.DataAction
	SchemaProjections []ArtifactSchemaProjection
}

type DeferredActionCandidatesInput struct {
	Records           []WorkflowRecord
	Actions           []dataquery.DataAction
	SchemaProjections []ArtifactSchemaProjection
}

func BuildDeferredActionCandidates(input DeferredActionCandidatesInput) []DeferredActionCandidate {
	out := make([]DeferredActionCandidate, 0, len(input.Actions))
	for i, action := range input.Actions {
		readyAction, ok := DeferredReadyAction(DeferredActionReadinessInput{
			Records:           input.Records,
			Action:            action,
			SchemaProjections: input.SchemaProjections,
		})
		candidate := DeferredActionCandidate{
			Index:  i,
			Action: action,
			Ready:  ok,
		}
		if ok {
			candidate.Action = readyAction
		} else {
			candidate.BlockedCode, candidate.BlockedReason = DeferredActionBlockedStatus(DeferredActionReadinessInput{
				Records:           input.Records,
				Action:            action,
				SchemaProjections: input.SchemaProjections,
			})
		}
		out = append(out, candidate)
	}
	return out
}

func DeferredReadyAction(input DeferredActionReadinessInput) (dataquery.DataAction, bool) {
	if narrowed, ok := NarrowSingleRecordSetActionForExecution(input.Action, deferredSchemaProjections(input)); ok {
		input.Action = narrowed
	}
	if DeferredActionInputsReady(input) {
		return input.Action, true
	}
	rewritten, ok := RewriteDeferredActionFirstInputToCompatibleArtifact(input)
	if !ok {
		return dataquery.DataAction{}, false
	}
	input.Action = rewritten
	if DeferredActionInputsReady(input) {
		return rewritten, true
	}
	return dataquery.DataAction{}, false
}

func DeferredActionInputsReady(input DeferredActionReadinessInput) bool {
	inputs := cleanStrings(input.Action.InputPaths)
	if len(inputs) == 0 {
		return true
	}
	available := DeferredAvailableInputPaths(input)
	for _, actionInput := range inputs {
		if CoveragePathCovered(available, actionInput) {
			continue
		}
		return false
	}
	if !DeferredFieldContractGuardResult(input).Empty() {
		return false
	}
	return true
}

func DeferredActionBlockedStatus(input DeferredActionReadinessInput) (string, string) {
	inputs := cleanStrings(input.Action.InputPaths)
	if len(inputs) > 0 {
		available := DeferredAvailableInputPaths(input)
		var missing []string
		for _, actionInput := range inputs {
			if CoveragePathCovered(available, actionInput) {
				continue
			}
			missing = append(missing, actionInput)
		}
		if len(missing) > 0 {
			return DeferredBlockInputUnavailable, fmt.Sprintf("deferred action %s:%s waits for unavailable input alias/material [%s]",
				firstNonEmpty(strings.TrimSpace(input.Action.ID), strings.TrimSpace(string(input.Action.Kind))),
				NormalizeActionKind(input.Action.Kind),
				strings.Join(cleanStrings(missing), ", "))
		}
	}
	if guard := DeferredFieldContractGuardResult(input); !guard.Empty() {
		return firstNonEmpty(strings.TrimSpace(guard.Code), DeferredBlockFieldContract), guard.ErrorText()
	}
	return DeferredBlockNotReady, ""
}

func DeferredFieldContractGuardResult(input DeferredActionReadinessInput) GuardResult {
	return ActionFieldReferenceGuardResult(ActionFieldReferenceGuardInput{
		Action:            input.Action,
		SchemaProjections: deferredSchemaProjections(input),
	})
}

func DeferredAvailableInputPaths(input DeferredActionReadinessInput) map[string]bool {
	available := AvailableActionInputPaths(input.Records, dataquery.TaskPlan{}, 0)
	for _, projection := range deferredSchemaProjections(input) {
		for _, alias := range artifactSchemaAliases(projection) {
			if key := normalizeCoveragePath(alias); key != "" {
				available[key] = true
			}
		}
	}
	return available
}

func RewriteDeferredActionFirstInputToCompatibleArtifact(input DeferredActionReadinessInput) (dataquery.DataAction, bool) {
	actionInput, fields, explicitSource := DeferredActionFirstInputFieldContract(input.Action)
	if actionInput == "" || len(fields) == 0 {
		return dataquery.DataAction{}, false
	}
	projections := deferredSchemaProjections(input)
	if len(projections) == 0 {
		return dataquery.DataAction{}, false
	}
	missing := MissingFieldsOnArtifactSchema(projections, actionInput, fields)
	if len(missing) == 0 {
		return dataquery.DataAction{}, false
	}
	candidate, ok := BestArtifactAliasWithFields(projections, actionInput, missing)
	if !ok {
		return dataquery.DataAction{}, false
	}
	rewritten := input.Action
	if explicitSource {
		rewritten.Params = cloneStringMap(rewritten.Params)
		rewritten.Params["source_path"] = candidate
		return rewritten, true
	}
	inputs := append([]string(nil), rewritten.InputPaths...)
	if len(inputs) == 0 {
		return dataquery.DataAction{}, false
	}
	inputs[0] = candidate
	rewritten.InputPaths = inputs
	return rewritten, true
}

func DeferredActionFirstInputFieldContract(action dataquery.DataAction) (input string, fields []string, explicitSource bool) {
	switch NormalizeActionKind(action.Kind) {
	case dataquery.DataActionNormalizeEntities:
		input = firstNonEmpty(
			action.Params["source_path"],
			action.Params["base_path"],
			action.Params["record_path"],
		)
		explicitSource = input != ""
		if input == "" {
			input = firstActionInput(action.InputPaths, 0)
		}
		fields = cleanStrings(append(
			parseActionStringList(firstNonEmpty(action.Params["source_fields"], action.Params["source_fields_json"], action.Params["source_names"], action.Params["source_names_json"])),
			parseActionStringList(firstNonEmpty(action.Params["source_field"], action.Params["source_name"]))...,
		))
		fields = cleanStrings(append(fields, FilterActionFieldRefs(action)...))
	default:
		return "", nil, false
	}
	return strings.TrimSpace(input), cleanStrings(fields), explicitSource
}

func NarrowSingleRecordSetActionForExecution(action dataquery.DataAction, projections []ArtifactSchemaProjection) (dataquery.DataAction, bool) {
	if len(projections) == 0 {
		return dataquery.DataAction{}, false
	}
	kind := NormalizeActionKind(action.Kind)
	switch kind {
	case dataquery.DataActionDeriveFields, dataquery.DataActionExtractFields, dataquery.DataActionGroupRecords, dataquery.DataActionExpandRecords, dataquery.DataActionFilterRecords, dataquery.DataActionQualifyRecords, dataquery.DataActionComputeContribs:
	default:
		return dataquery.DataAction{}, false
	}
	inputs := cleanStrings(action.InputPaths)
	if len(inputs) <= 1 {
		return dataquery.DataAction{}, false
	}
	fieldRefs := SingleRecordSetActionFieldRefs(kind, action)
	if len(fieldRefs) == 0 {
		return dataquery.DataAction{}, false
	}
	var matches []string
	for _, input := range inputs {
		if _, ok := ArtifactSchemaByAlias(projections, input); !ok {
			continue
		}
		if len(MissingFieldsOnArtifactSchema(projections, input, fieldRefs)) == 0 {
			matches = append(matches, input)
		}
	}
	if len(matches) != 1 {
		return dataquery.DataAction{}, false
	}
	action.InputPaths = []string{matches[0]}
	if purpose := strings.TrimSpace(action.Purpose); purpose != "" && !strings.Contains(purpose, "input narrowed by field contract") {
		action.Purpose = purpose + " (input narrowed by field contract)"
	}
	return action, true
}

func BestArtifactAliasWithFields(projections []ArtifactSchemaProjection, current string, fields []string) (string, bool) {
	currentKey := normalizeAccessPath(current)
	for _, projection := range projections {
		alias := firstArtifactAlias(projection)
		if alias == "" {
			continue
		}
		if normalizeAccessPath(alias) == currentKey || normalizeAccessPath(projection.ID) == currentKey {
			continue
		}
		if len(MissingFieldsOnArtifactSchema(projections, alias, fields)) == 0 {
			return alias, true
		}
	}
	return "", false
}

func deferredSchemaProjections(input DeferredActionReadinessInput) []ArtifactSchemaProjection {
	if len(input.SchemaProjections) > 0 {
		return input.SchemaProjections
	}
	return ProjectArtifactSchemasNewestFirst(ArtifactsNewestFirst(input.Records))
}

func cloneStringMap(in map[string]string) map[string]string {
	out := make(map[string]string, len(in)+1)
	for key, value := range in {
		out[key] = value
	}
	return out
}
