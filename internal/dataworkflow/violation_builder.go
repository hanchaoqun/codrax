package dataworkflow

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/dataquery"
)

type FieldContractViolationInput struct {
	Action             dataquery.DataAction
	InputAlias         string
	MissingFields      []string
	AvailableFields    []string
	SchemaProjections  []ArtifactSchemaProjection
	AllowedNextActions []string
}

func NewFieldContractViolation(input FieldContractViolationInput) WorkflowViolation {
	action, _ := NormalizeRolePathAction(input.Action)
	kind := NormalizeActionKind(action.Kind)
	capability, _ := Capability(kind)
	inputs := cleanActionAliases(action.InputPaths)
	if len(inputs) == 0 && strings.TrimSpace(input.InputAlias) != "" {
		inputs = cleanActionAliases([]string{input.InputAlias})
	}
	candidates := FieldContractCandidateArtifacts(input.SchemaProjections, input.InputAlias, input.MissingFields, 4)
	missing := cleanStrings(input.MissingFields)
	available := cleanStrings(input.AvailableFields)
	reason := fmt.Sprintf("input %s is missing field(s) [%s]", strings.TrimSpace(input.InputAlias), strings.Join(missing, ", "))
	if len(available) > 0 {
		reason += fmt.Sprintf("; available fields [%s]", strings.Join(clampStrings(available, 32), ", "))
	}
	return WorkflowViolation{
		Code:                 "field_contract_violation",
		Severity:             "error",
		Repairability:        RepairNeedsTypedAction,
		ActionID:             strings.TrimSpace(action.ID),
		ActionKind:           string(kind),
		InputAlias:           strings.TrimSpace(input.InputAlias),
		InputAliases:         inputs,
		OutputAlias:          strings.TrimSpace(action.OutputArtifact),
		IdempotencyKey:       ActionIdempotencyKey(action),
		DependencyRank:       capability.DependencyRank,
		MissingFields:        missing,
		AvailableFieldSample: clampStrings(available, 32),
		CandidateArtifacts:   FieldContractCandidateLabels(candidates),
		RepairActionHints:    FieldContractRepairHints(input.AllowedNextActions),
		Reason:               reason,
	}
}

func NewActionInputViolation(code, severity string, repairability ViolationRepairability, action dataquery.DataAction, inputAlias string, missingFields []string, reason string, hints []string) WorkflowViolation {
	action, _ = NormalizeRolePathAction(action)
	kind := NormalizeActionKind(action.Kind)
	capability, _ := Capability(kind)
	inputs := cleanActionAliases(action.InputPaths)
	if len(inputs) == 0 && strings.TrimSpace(inputAlias) != "" {
		inputs = cleanActionAliases([]string{inputAlias})
	}
	return WorkflowViolation{
		Code:              strings.TrimSpace(code),
		Severity:          strings.TrimSpace(severity),
		Repairability:     repairability,
		ActionID:          strings.TrimSpace(action.ID),
		ActionKind:        string(kind),
		InputAlias:        strings.TrimSpace(inputAlias),
		InputAliases:      inputs,
		OutputAlias:       strings.TrimSpace(action.OutputArtifact),
		IdempotencyKey:    ActionIdempotencyKey(action),
		DependencyRank:    capability.DependencyRank,
		MissingFields:     cleanStrings(missingFields),
		RepairActionHints: cleanStrings(hints),
		Reason:            strings.TrimSpace(reason),
	}
}

func clampStrings(values []string, limit int) []string {
	values = cleanStrings(values)
	if limit <= 0 || len(values) <= limit {
		return values
	}
	return append([]string(nil), values[:limit]...)
}
