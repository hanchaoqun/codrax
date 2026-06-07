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

type GenericViolationInput struct {
	Code               string
	Severity           string
	Repairability      ViolationRepairability
	Action             dataquery.DataAction
	InputAlias         string
	InputAliases       []string
	OutputAlias        string
	MissingFields      []string
	AvailableFields    []string
	CandidateArtifacts []string
	RepairActionHints  []string
	Reason             string
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

func NewGenericViolation(input GenericViolationInput) WorkflowViolation {
	violation := WorkflowViolation{
		Code:                 strings.TrimSpace(input.Code),
		Severity:             strings.TrimSpace(input.Severity),
		Repairability:        input.Repairability,
		InputAlias:           strings.TrimSpace(input.InputAlias),
		InputAliases:         cleanActionAliases(input.InputAliases),
		OutputAlias:          strings.TrimSpace(input.OutputAlias),
		MissingFields:        cleanStrings(input.MissingFields),
		AvailableFieldSample: clampStrings(input.AvailableFields, 32),
		CandidateArtifacts:   cleanStrings(input.CandidateArtifacts),
		RepairActionHints:    cleanStrings(input.RepairActionHints),
		Reason:               strings.TrimSpace(input.Reason),
	}
	if violation.Severity == "" {
		violation.Severity = "error"
	}
	if violation.Repairability == "" {
		violation.Repairability = RepairNeedsTypedAction
	}
	action := input.Action
	if !actionHasStructuralShape(action) {
		return violation
	}
	action, _ = NormalizeRolePathAction(action)
	kind := NormalizeActionKind(action.Kind)
	capability, _ := Capability(kind)
	violation.ActionID = strings.TrimSpace(action.ID)
	violation.ActionKind = string(kind)
	if len(violation.InputAliases) == 0 {
		violation.InputAliases = cleanActionAliases(action.InputPaths)
	}
	if violation.InputAlias == "" && len(violation.InputAliases) > 0 {
		violation.InputAlias = violation.InputAliases[0]
	}
	if violation.OutputAlias == "" {
		violation.OutputAlias = strings.TrimSpace(action.OutputArtifact)
	}
	violation.IdempotencyKey = ActionIdempotencyKey(action)
	violation.DependencyRank = capability.DependencyRank
	return violation
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

func actionHasStructuralShape(action dataquery.DataAction) bool {
	return strings.TrimSpace(action.ID) != "" ||
		strings.TrimSpace(string(action.Kind)) != "" ||
		strings.TrimSpace(action.OutputArtifact) != "" ||
		len(cleanActionAliases(action.InputPaths)) > 0 ||
		strings.TrimSpace(action.Script) != "" ||
		len(action.Params) > 0
}

func clampStrings(values []string, limit int) []string {
	values = cleanStrings(values)
	if limit <= 0 || len(values) <= limit {
		return values
	}
	return append([]string(nil), values[:limit]...)
}
