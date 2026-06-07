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

type FieldContractGuardInput struct {
	Action      dataquery.DataAction
	ActionIndex int
	Violation   WorkflowViolation
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

func FieldContractGuardResult(input FieldContractGuardInput) GuardResult {
	violation := input.Violation
	if strings.TrimSpace(violation.Code) == "" ||
		len(violation.MissingFields) == 0 ||
		strings.TrimSpace(violation.InputAlias) == "" {
		return GuardResult{}
	}
	action := input.Action
	actionNumber := input.ActionIndex + 1
	if actionNumber <= 0 {
		actionNumber = 1
	}
	actionLabel := firstNonEmptyGuardText(
		strings.TrimSpace(action.ID),
		strings.TrimSpace(violation.ActionID),
		strings.TrimSpace(string(NormalizeActionKind(action.Kind))),
		strings.TrimSpace(violation.ActionKind),
	)
	candidateHint := ""
	if len(violation.CandidateArtifacts) > 0 {
		candidateHint = " Candidate artifact(s) with relevant field(s): " + strings.Join(violation.CandidateArtifacts, "; ") + "."
	}
	message := fmt.Sprintf("data planning incomplete: action %d (%s) references field(s) [%s] that are not present on input %s fields [%s].%s Use an existing artifact from workflow_state_json.artifact_availability, or first materialize the missing field(s) with derive_fields, extract_fields, group_records, enrich_records, join_records, or a valid prior typed action before consuming them.",
		actionNumber,
		actionLabel,
		strings.Join(violation.MissingFields, ", "),
		violation.InputAlias,
		strings.Join(clampStrings(violation.AvailableFieldSample, 32), ", "),
		candidateHint)
	return NewGuardResult("field_contract_violation", "error", RepairNeedsTypedAction, message, violation)
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
