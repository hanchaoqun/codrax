package dataworkflow

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/dataquery"
)

type InvalidRecordMaterializationFallbackInput struct {
	Current      dataquery.TaskPlan
	State        WorkflowStateView
	ExtractLimit int
}

func BuildInvalidRecordMaterializationFallbackPlan(input InvalidRecordMaterializationFallbackInput) (dataquery.TaskPlan, bool) {
	for _, action := range input.Current.Actions {
		if !ActionNeedsRecordMaterialization(action) {
			continue
		}
		inputs := cleanStrings(action.InputPaths)
		if len(inputs) == 0 {
			continue
		}
		if input.State.MaterialCoverageSufficient &&
			input.State.NextStage != StageCoverRequiredMaterials &&
			input.State.NextStage != StageDeriveRules {
			return dataquery.TaskPlan{}, false
		}
		limit := input.ExtractLimit
		if limit <= 0 {
			limit = DefaultRecordMaterializationLimit
		}
		out := input.Current
		out.Actions = []dataquery.DataAction{{
			ID:             firstNonEmpty(strings.TrimSpace(action.ID), "extract_records") + "_records",
			Kind:           dataquery.DataActionExtractRecords,
			Purpose:        "materialize record samples for later single-record-set derivation, enrichment, join, or contribution steps",
			InputPaths:     inputs,
			OutputArtifact: firstNonEmpty(strings.TrimSpace(action.OutputArtifact), strings.TrimSpace(action.ID), "records"),
			Params: map[string]string{
				"limit":       fmt.Sprintf("%d", limit),
				"max_records": fmt.Sprintf("%d", limit),
			},
		}}
		out.Script = ""
		out.Status = "ready"
		out.ContinueAfter = true
		if strings.TrimSpace(out.WhyThisBatch) == "" {
			out.WhyThisBatch = fmt.Sprintf("%s was not executable as one single-record-set action; first materialize the declared records as a bounded atomic batch", NormalizeActionKind(action.Kind))
		}
		if strings.TrimSpace(out.NextBatch) == "" {
			out.NextBatch = "continue with derive_fields, extract_fields, group_records, enrich_records, join_records, or compute_contributions after record artifacts materialize"
		}
		return out, true
	}
	return dataquery.TaskPlan{}, false
}

func ActionNeedsRecordMaterialization(action dataquery.DataAction) bool {
	kind := NormalizeActionKind(action.Kind)
	if kind != dataquery.DataActionDeriveFields && kind != dataquery.DataActionExtractFields {
		return false
	}
	inputs := cleanStrings(action.InputPaths)
	if len(inputs) == 0 {
		return false
	}
	if len(inputs) <= 1 && DeriveOrExtractFieldsActionHasSpec(action) {
		return false
	}
	return true
}

func DeriveOrExtractFieldsActionHasSpec(action dataquery.DataAction) bool {
	if len(action.Params) == 0 {
		return false
	}
	for _, key := range []string{
		"field_specs_json", "extract_specs_json", "derive_specs_json", "transforms_json",
		"field_specs", "extract_specs", "derive_specs", "transforms",
		"source_field", "input_field", "field",
		"target_field", "output_field", "derived_field",
		"operation", "op", "transform",
	} {
		if strings.TrimSpace(action.Params[key]) != "" {
			return true
		}
	}
	return false
}
