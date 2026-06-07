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

type ActionInputContractGuardInput struct {
	Code              string
	Action            dataquery.DataAction
	InputAlias        string
	MissingFields     []string
	Message           string
	RepairActionHints []string
}

type IntraBatchDependencyGuardInput struct {
	Action        dataquery.DataAction
	ActionIndex   int
	ConsumedInput string
	Producer      dataquery.DataAction
}

type UnavailableActionInputGuardInput struct {
	Action      dataquery.DataAction
	ActionIndex int
	Missing     []string
}

type InputPathContractGuardInput struct {
	Action      dataquery.DataAction
	ActionIndex int
	Contract    ActionInputPathContract
}

type MissingActionSpecGuardInput struct {
	Action      dataquery.DataAction
	ActionIndex int
}

type MissingUpstreamLedgerGuardInput struct {
	Action      dataquery.DataAction
	ActionIndex int
	Ledger      LedgerKind
}

type ZeroMatchFilterGuardInput struct {
	Action            dataquery.DataAction
	ActionIndex       int
	InputAlias        string
	ArtifactID        string
	InputRows         int
	OutputRows        int
	SourcePath        string
	FilterFields      []string
	Reason            string
	RepairActionHints []string
}

type UnmatchedResolutionGuardInput struct {
	Action            dataquery.DataAction
	ActionIndex       int
	InputAlias        string
	ArtifactID        string
	BasePath          string
	BaseRows          int
	TargetFields      []string
	Reason            string
	RepairActionHints []string
}

type ZeroEligibleGuardInput struct {
	Action            dataquery.DataAction
	ActionIndex       int
	InputAlias        string
	ArtifactID        string
	InputRows         int
	EligibleRows      int
	Reason            string
	RepairActionHints []string
}

type ApplyResolutionDiagnosticInputGuardInput struct {
	Action         dataquery.DataAction
	ActionIndex    int
	ResolutionPath string
}

type ApplyResolutionLineageGuardInput struct {
	Action                  dataquery.DataAction
	ActionIndex             int
	ResolutionPath          string
	BasePath                string
	ResolutionSourceLineage string
	BaseLineage             string
}

type ApplyResolutionNoProgressGuardInput struct {
	Action         dataquery.DataAction
	ActionIndex    int
	ResolutionPath string
	BasePath       string
	TargetFields   []string
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

func ActionInputContractGuardResult(input ActionInputContractGuardInput) GuardResult {
	message := strings.TrimSpace(input.Message)
	code := strings.TrimSpace(input.Code)
	if message == "" || code == "" {
		return GuardResult{}
	}
	violation := NewActionInputViolation(
		code,
		"error",
		RepairNeedsTypedAction,
		input.Action,
		input.InputAlias,
		input.MissingFields,
		message,
		input.RepairActionHints,
	)
	return NewGuardResult(code, "error", RepairNeedsTypedAction, message, violation)
}

func IntraBatchDependencyGuardResult(input IntraBatchDependencyGuardInput) GuardResult {
	consumedInput := strings.TrimSpace(input.ConsumedInput)
	if consumedInput == "" {
		return GuardResult{}
	}
	producerLabel := firstNonEmptyGuardText(
		strings.TrimSpace(input.Producer.ID),
		strings.TrimSpace(input.Producer.OutputArtifact),
		strings.TrimSpace(string(NormalizeActionKind(input.Producer.Kind))),
		strings.TrimSpace(string(input.Producer.Kind)),
	)
	message := fmt.Sprintf("data planning incomplete: action %d (%s) consumes prior action output %s from action %s in the same batch. Execute the producer action rank first, let it materialize a real artifact and field contract, then continue with dependent actions from the deferred queue.",
		guardActionNumber(input.ActionIndex),
		guardActionLabel(input.Action),
		consumedInput,
		producerLabel)
	return ActionInputContractGuardResult(ActionInputContractGuardInput{
		Code:       "intra_batch_dependency",
		Action:     input.Action,
		InputAlias: consumedInput,
		Message:    message,
	})
}

func UnavailableActionInputGuardResult(input UnavailableActionInputGuardInput) GuardResult {
	missing := cleanStrings(input.Missing)
	if len(missing) == 0 {
		return GuardResult{}
	}
	message := fmt.Sprintf("data planning incomplete: action %d (%s) references unavailable input_path(s) [%s]. Use covered source materials, prior generated artifact aliases from workflow_state_json.artifact_availability, or add earlier typed actions in this batch that output these aliases before consuming them; do not assume deferred or future action outputs already exist.",
		guardActionNumber(input.ActionIndex),
		guardActionLabel(input.Action),
		strings.Join(missing, ", "))
	return ActionInputContractGuardResult(ActionInputContractGuardInput{
		Code:          "unavailable_action_input",
		Action:        input.Action,
		InputAlias:    missing[0],
		MissingFields: nil,
		Message:       message,
	})
}

func InputPathContractGuardResult(input InputPathContractGuardInput) GuardResult {
	action := input.Action
	kind := NormalizeActionKind(action.Kind)
	inputs := cleanActionAliases(action.InputPaths)
	if len(inputs) < input.Contract.Min {
		var message string
		if kind == dataquery.DataActionComputeContribs {
			message = fmt.Sprintf("data planning incomplete: action %d (%s) requires input_paths containing existing records or generated artifact aliases before contribution computation.",
				guardActionNumber(input.ActionIndex), guardActionLabel(action))
		} else {
			message = fmt.Sprintf("data planning incomplete: action %d (%s) requires input_paths. Choose concrete candidate material paths or prior artifact aliases; do not emit an empty %s action.",
				guardActionNumber(input.ActionIndex), guardActionLabel(action), strings.TrimSpace(string(kind)))
		}
		return ActionInputContractGuardResult(ActionInputContractGuardInput{
			Code:    "missing_action_inputs",
			Action:  action,
			Message: message,
		})
	}
	if input.Contract.Max > 0 && len(inputs) > input.Contract.Max {
		var message string
		if input.Contract.SingleRecordSet {
			message = fmt.Sprintf("data planning incomplete: action %d (%s) is %s with %d input_paths. %s is a single-record-set action. Do not use it for lookup/reference-table mapping. Split different schemas into separate actions, or first use normalize_entities, enrich_records, or join_records to create one joined/generated artifact.",
				guardActionNumber(input.ActionIndex), guardActionLabel(action), kind, len(inputs), kind)
		} else {
			message = fmt.Sprintf("data planning incomplete: action %d (%s) is %s with %d input_paths, but this action accepts at most %d. Split the graph into atomic DAG ranks; for joins, combine exactly two record sets first, let the output artifact materialize, then join that artifact with the next record set.",
				guardActionNumber(input.ActionIndex), guardActionLabel(action), kind, len(inputs), input.Contract.Max)
		}
		return ActionInputContractGuardResult(ActionInputContractGuardInput{
			Code:    "too_many_action_inputs",
			Action:  action,
			Message: message,
		})
	}
	return GuardResult{}
}

func MissingActionSpecGuardResult(input MissingActionSpecGuardInput) GuardResult {
	action := input.Action
	kind := NormalizeActionKind(action.Kind)
	var message string
	switch kind {
	case dataquery.DataActionDeriveFields:
		message = fmt.Sprintf("data planning incomplete: action %d (%s) is derive_fields but has no field specification. Add params.field_specs_json (array of source_field/target_field/operation specs) or a single source_field+target_field+operation spec; if this batch only needs to materialize rows without deriving fields, use extract_records instead.",
			guardActionNumber(input.ActionIndex), guardActionLabel(action))
	case dataquery.DataActionExtractFields:
		message = fmt.Sprintf("data planning incomplete: action %d (%s) is extract_fields but has no field specification. Add params.field_specs or params.extract_specs as an array of source_field/target_field/operation/pattern specs; use it to materialize structured fields from one or more same-schema record/text artifacts.",
			guardActionNumber(input.ActionIndex), guardActionLabel(action))
	case dataquery.DataActionGroupRecords:
		message = fmt.Sprintf("data planning incomplete: action %d (%s) is group_records but has no grouping/text specification. Add params.group_field or params.group_fields, params.text_fields/source_fields, and params.target_field; use group_records to combine multiple rows/spans from one artifact into one grouped record before extract_fields, join_records, filtering, or contribution calculation.",
			guardActionNumber(input.ActionIndex), guardActionLabel(action))
	case dataquery.DataActionExpandRecords:
		message = fmt.Sprintf("data planning incomplete: action %d (%s) is expand_records but has no source_field. Add params.source_field and optionally params.target_field plus delimiter/separator or split_pattern; use expand_records only to turn one multi-value field into multiple records.",
			guardActionNumber(input.ActionIndex), guardActionLabel(action))
	case dataquery.DataActionFilterRecords:
		message = fmt.Sprintf("data planning incomplete: action %d (%s) is filter_records but has no filters. Add params.filters_json (array of field/op/value filters) or filter_field/filter_op/filter_value; use filter_records only when the filter fields already exist on the input artifact.",
			guardActionNumber(input.ActionIndex), guardActionLabel(action))
	default:
		return GuardResult{}
	}
	return ActionInputContractGuardResult(ActionInputContractGuardInput{
		Code:    "missing_action_spec",
		Action:  action,
		Message: message,
	})
}

func MissingApplyResolutionInputGuardResult(action dataquery.DataAction, actionIndex int) GuardResult {
	message := fmt.Sprintf("data planning incomplete: action %d (%s) is apply_entity_resolutions but has no resolution input. Provide input_paths=[base_record_artifact, entity_resolution_artifact...] or params.resolution_specs with resolution_path.",
		guardActionNumber(actionIndex), guardActionLabel(action))
	return ActionInputContractGuardResult(ActionInputContractGuardInput{
		Code:    "missing_action_inputs",
		Action:  action,
		Message: message,
	})
}

func MissingUpstreamLedgerGuardResult(input MissingUpstreamLedgerGuardInput) GuardResult {
	var message string
	switch input.Ledger {
	case LedgerContributions:
		message = fmt.Sprintf("data planning incomplete: action %d (%s) requires contribution records, but no prior compute_contributions result or earlier compute_contributions action is available. Add a bounded compute_contributions batch first, let it execute, then reconcile in a later batch or after a previous contribution-producing action.",
			guardActionNumber(input.ActionIndex), guardActionLabel(input.Action))
	case LedgerReconcile:
		message = fmt.Sprintf("data planning incomplete: action %d (%s) requires a prior reconcile report. Add reconcile_artifacts first, let it execute, then assemble the final output in a later batch.",
			guardActionNumber(input.ActionIndex), guardActionLabel(input.Action))
	default:
		return GuardResult{}
	}
	return ActionInputContractGuardResult(ActionInputContractGuardInput{
		Code:    "missing_upstream_ledger",
		Action:  input.Action,
		Message: message,
	})
}

func ZeroMatchFilterGuardResult(input ZeroMatchFilterGuardInput) GuardResult {
	inputAlias := strings.TrimSpace(input.InputAlias)
	artifactID := firstNonEmptyGuardText(input.ArtifactID, inputAlias)
	if inputAlias == "" || artifactID == "" {
		return GuardResult{}
	}
	message := fmt.Sprintf("data planning incomplete: action %d (%s) consumes zero-match filter artifact %s (%d/%d rows) while contribution/reconcile is still required. Inspect actual filter field values or rerun filter_records against %s with corrected filters before join_records or compute_contributions; do not continue from an empty candidate set.",
		guardActionNumber(input.ActionIndex),
		guardActionLabel(input.Action),
		artifactID,
		input.OutputRows,
		input.InputRows,
		firstNonEmptyGuardText(input.SourcePath, "the non-empty source artifact"))
	reason := firstNonEmptyGuardText(
		input.Reason,
		fmt.Sprintf("zero-match filter artifact %s produced %d rows from %d input rows", artifactID, input.OutputRows, input.InputRows),
	)
	violation := NewActionInputViolation(
		"zero_match_filter",
		"error",
		RepairNeedsTypedAction,
		input.Action,
		inputAlias,
		input.FilterFields,
		reason,
		input.RepairActionHints,
	)
	return NewGuardResult("zero_match_filter", "error", RepairNeedsTypedAction, message, violation)
}

func UnmatchedResolutionGuardResult(input UnmatchedResolutionGuardInput) GuardResult {
	inputAlias := strings.TrimSpace(input.InputAlias)
	artifactID := firstNonEmptyGuardText(input.ArtifactID, inputAlias)
	targetFields := cleanStrings(input.TargetFields)
	if inputAlias == "" || artifactID == "" {
		return GuardResult{}
	}
	message := fmt.Sprintf("data planning incomplete: action %d (%s) consumes all-unmatched resolution artifact %s (base_rows=%d target_fields=[%s]) while contribution/reconcile is still required. Repair apply_entity_resolutions key/role matching against %s before filtering, qualification, join, or contribution calculation.",
		guardActionNumber(input.ActionIndex),
		guardActionLabel(input.Action),
		artifactID,
		input.BaseRows,
		strings.Join(targetFields, ", "),
		firstNonEmptyGuardText(input.BasePath, "the base record artifact"))
	reason := firstNonEmptyGuardText(
		input.Reason,
		fmt.Sprintf("all-unmatched resolution artifact %s has %d base rows and target fields [%s]", artifactID, input.BaseRows, strings.Join(targetFields, ", ")),
	)
	violation := NewActionInputViolation(
		"unmatched_resolution",
		"error",
		RepairNeedsTypedAction,
		input.Action,
		inputAlias,
		targetFields,
		reason,
		input.RepairActionHints,
	)
	return NewGuardResult("unmatched_resolution", "error", RepairNeedsTypedAction, message, violation)
}

func ZeroEligibleGuardResult(input ZeroEligibleGuardInput) GuardResult {
	inputAlias := strings.TrimSpace(input.InputAlias)
	artifactID := firstNonEmptyGuardText(input.ArtifactID, inputAlias)
	if inputAlias == "" || artifactID == "" {
		return GuardResult{}
	}
	message := fmt.Sprintf("data planning incomplete: action %d (%s) consumes zero-eligible qualification artifact %s (%d/%d eligible rows) while contribution/reconcile is still required. Repair the upstream field materialization, resolution application, or qualify_records filters before join_records or compute_contributions.",
		guardActionNumber(input.ActionIndex),
		guardActionLabel(input.Action),
		artifactID,
		input.EligibleRows,
		input.InputRows)
	reason := firstNonEmptyGuardText(
		input.Reason,
		fmt.Sprintf("zero-eligible qualification artifact %s has %d eligible rows from %d input rows", artifactID, input.EligibleRows, input.InputRows),
	)
	violation := NewActionInputViolation(
		"zero_eligible_records",
		"error",
		RepairNeedsTypedAction,
		input.Action,
		inputAlias,
		nil,
		reason,
		input.RepairActionHints,
	)
	return NewGuardResult("zero_eligible_records", "error", RepairNeedsTypedAction, message, violation)
}

func ApplyResolutionDiagnosticInputGuardResult(input ApplyResolutionDiagnosticInputGuardInput) GuardResult {
	resolutionPath := strings.TrimSpace(input.ResolutionPath)
	if resolutionPath == "" {
		return GuardResult{}
	}
	message := fmt.Sprintf("data planning incomplete: action %d (%s) apply_entity_resolutions uses diagnostic artifact %s as a resolution input. Diagnostic/source child artifacts can be inspected for schema evidence, but executable resolution application must use a mapping/entity-resolution ledger or an already materialized canonical field on the base record artifact.",
		guardActionNumber(input.ActionIndex),
		guardActionLabel(input.Action),
		resolutionPath)
	return ActionInputContractGuardResult(ActionInputContractGuardInput{
		Code:       "apply_resolution_diagnostic_input",
		Action:     input.Action,
		InputAlias: resolutionPath,
		Message:    message,
		RepairActionHints: []string{
			string(dataquery.DataActionInspectMaterial),
			string(dataquery.DataActionDeriveFields),
			string(dataquery.DataActionFilterRecords),
			string(dataquery.DataActionQualifyRecords),
			string(dataquery.DataActionComputeContribs),
		},
	})
}

func ApplyResolutionLineageGuardResult(input ApplyResolutionLineageGuardInput) GuardResult {
	resolutionPath := strings.TrimSpace(input.ResolutionPath)
	basePath := strings.TrimSpace(input.BasePath)
	if resolutionPath == "" || basePath == "" {
		return GuardResult{}
	}
	message := fmt.Sprintf("data planning incomplete: action %d (%s) applies resolution input %s to base artifact %s, but the resolution source lineage [%s] is not compatible with the base lineage [%s]. Apply entity resolutions only to the source record lineage they were produced from, or first materialize a compatible joined/enriched artifact.",
		guardActionNumber(input.ActionIndex),
		guardActionLabel(input.Action),
		resolutionPath,
		basePath,
		strings.TrimSpace(input.ResolutionSourceLineage),
		strings.TrimSpace(input.BaseLineage))
	return ActionInputContractGuardResult(ActionInputContractGuardInput{
		Code:       "apply_resolution_lineage_contract",
		Action:     input.Action,
		InputAlias: resolutionPath,
		Message:    message,
	})
}

func ApplyResolutionNoProgressGuardResult(input ApplyResolutionNoProgressGuardInput) GuardResult {
	resolutionPath := strings.TrimSpace(input.ResolutionPath)
	basePath := strings.TrimSpace(input.BasePath)
	targetFields := cleanStrings(input.TargetFields)
	if resolutionPath == "" || basePath == "" || len(targetFields) == 0 {
		return GuardResult{}
	}
	message := fmt.Sprintf("data planning incomplete: action %d (%s) repeats apply_entity_resolutions for resolution input %s on base artifact %s; base already carries target field(s) [%s] from that resolution lineage. This graph edge is idempotent and would not create contribution/reconcile progress. Use the existing resolved artifact for derive_fields, filter_records, qualify_records, join_records, compute_contributions, or inspect a concrete field gap instead.",
		guardActionNumber(input.ActionIndex),
		guardActionLabel(input.Action),
		resolutionPath,
		basePath,
		strings.Join(targetFields, ", "))
	return ActionInputContractGuardResult(ActionInputContractGuardInput{
		Code:       "apply_resolution_no_progress",
		Action:     input.Action,
		InputAlias: resolutionPath,
		Message:    message,
	})
}

func guardActionNumber(actionIndex int) int {
	actionNumber := actionIndex + 1
	if actionNumber <= 0 {
		return 1
	}
	return actionNumber
}

func guardActionLabel(action dataquery.DataAction) string {
	return firstNonEmptyGuardText(
		strings.TrimSpace(action.ID),
		strings.TrimSpace(string(NormalizeActionKind(action.Kind))),
		strings.TrimSpace(string(action.Kind)),
	)
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
