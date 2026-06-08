package dataworkflow

import (
	"sort"
	"strings"

	"github.com/hanchaoqun/codrax/internal/dataquery"
)

type ViolationRepairability string

const (
	RepairSafePatch          ViolationRepairability = "safe_patch"
	RepairNeedsTypedAction   ViolationRepairability = "needs_typed_action"
	RepairNeedsRecompute     ViolationRepairability = "needs_recompute"
	RepairNeedsClarification ViolationRepairability = "needs_clarification"
)

type WorkflowViolation struct {
	Code                 string                 `json:"code,omitempty"`
	Severity             string                 `json:"severity,omitempty"`
	Repairability        ViolationRepairability `json:"repairability,omitempty"`
	ActionID             string                 `json:"action_id,omitempty"`
	ActionKind           string                 `json:"action_kind,omitempty"`
	Field                string                 `json:"field,omitempty"`
	Operation            string                 `json:"operation,omitempty"`
	Param                string                 `json:"param,omitempty"`
	Role                 string                 `json:"role,omitempty"`
	InputAlias           string                 `json:"input_alias,omitempty"`
	InputAliases         []string               `json:"input_aliases,omitempty"`
	OutputAlias          string                 `json:"output_alias,omitempty"`
	IdempotencyKey       string                 `json:"idempotency_key,omitempty"`
	DependencyRank       int                    `json:"dependency_rank,omitempty"`
	MissingFields        []string               `json:"missing_fields,omitempty"`
	AvailableFieldSample []string               `json:"available_field_sample,omitempty"`
	CandidateArtifacts   []string               `json:"candidate_artifacts,omitempty"`
	RepairActionHints    []string               `json:"repair_action_hints,omitempty"`
	Limit                int                    `json:"limit,omitempty"`
	Observed             int                    `json:"observed,omitempty"`
	Reason               string                 `json:"reason,omitempty"`
}

type WorkflowViolationSummary struct {
	Total                  int                          `json:"total,omitempty"`
	ErrorCount             int                          `json:"error_count,omitempty"`
	WarningCount           int                          `json:"warning_count,omitempty"`
	ByCode                 []WorkflowViolationCodeCount `json:"by_code,omitempty"`
	FirstCode              string                       `json:"first_code,omitempty"`
	FirstSeverity          string                       `json:"first_severity,omitempty"`
	FirstRepairability     ViolationRepairability       `json:"first_repairability,omitempty"`
	FirstActionID          string                       `json:"first_action_id,omitempty"`
	FirstActionKind        string                       `json:"first_action_kind,omitempty"`
	FirstInputAlias        string                       `json:"first_input_alias,omitempty"`
	FirstOutputAlias       string                       `json:"first_output_alias,omitempty"`
	FirstField             string                       `json:"first_field,omitempty"`
	FirstOperation         string                       `json:"first_operation,omitempty"`
	FirstParam             string                       `json:"first_param,omitempty"`
	FirstRole              string                       `json:"first_role,omitempty"`
	FirstMissingFields     []string                     `json:"first_missing_fields,omitempty"`
	FirstRepairActionHints []string                     `json:"first_repair_action_hints,omitempty"`
	FirstLimit             int                          `json:"first_limit,omitempty"`
	FirstObserved          int                          `json:"first_observed,omitempty"`
	FirstReason            string                       `json:"first_reason,omitempty"`
}

type WorkflowViolationCodeCount struct {
	Code  string `json:"code,omitempty"`
	Count int    `json:"count,omitempty"`
}

type WorkflowViolationInput struct {
	Facts               StageFacts
	Records             []WorkflowRecord
	ProgressEvents      []ProgressEvent
	NoProgressThreshold int
	GuardViolations     []WorkflowViolation
	Additional          []WorkflowViolation
}

type WorkflowStateViolationInput struct {
	Records             []WorkflowRecord
	State               WorkflowStateView
	GuardViolations     []WorkflowViolation
	NoProgressThreshold int
}

// ActiveGuardViolationsFromRecords returns admission guard blockers that still
// belong to the live workflow suffix. Older rejected-plan guards remain in
// journal/audit events, but once a later record produces successful typed
// progress they should not keep steering the current reducer decision.
func ActiveGuardViolationsFromRecords(records []WorkflowRecord) []WorkflowViolation {
	start := 0
	for i, rec := range records {
		if workflowRecordHasSuccessfulProgress(rec) {
			start = i + 1
		}
	}
	var out []WorkflowViolation
	for _, rec := range records[start:] {
		guard := activeAdmissionGuard(rec.Admission)
		if guard.Empty() {
			continue
		}
		out = append(out, guard.Violations...)
	}
	return cloneWorkflowViolations(out)
}

func workflowRecordHasSuccessfulProgress(rec WorkflowRecord) bool {
	return rec.Result != nil && strings.TrimSpace(rec.Err) == ""
}

func activeAdmissionGuard(admission *ActionDAGAdmissionDecision) GuardResult {
	if admission == nil {
		return GuardResult{}
	}
	if !admission.FinalGuard.Empty() {
		return admission.FinalGuard
	}
	return admission.Guard
}

func BuildWorkflowStateViolations(input WorkflowStateViolationInput) []WorkflowViolation {
	state := input.State
	additional := make([]WorkflowViolation, 0,
		len(input.Records)+
			len(state.FieldContractViolations)+
			len(state.ZeroMatchFilterViolations)+
			len(state.UnmatchedResolutionViolations)+
			len(state.ZeroEligibleViolations))
	additional = append(additional, WorkflowViolationsFromRecordExecution(input.Records)...)
	for _, issue := range state.FieldContractViolations {
		additional = append(additional, WorkflowViolation{
			Code:                 "field_contract_violation",
			Severity:             "error",
			Repairability:        RepairNeedsTypedAction,
			ActionID:             strings.TrimSpace(issue.ActionID),
			ActionKind:           strings.TrimSpace(issue.ActionKind),
			InputAlias:           strings.TrimSpace(issue.InputAlias),
			InputAliases:         cleanStrings(issue.InputAliases),
			OutputAlias:          strings.TrimSpace(issue.OutputAlias),
			IdempotencyKey:       strings.TrimSpace(issue.IdempotencyKey),
			DependencyRank:       issue.DependencyRank,
			MissingFields:        append([]string(nil), issue.MissingFields...),
			AvailableFieldSample: append([]string(nil), issue.AvailableFieldSample...),
			CandidateArtifacts:   append([]string(nil), issue.CandidateArtifacts...),
			RepairActionHints:    append([]string(nil), issue.RepairActionHints...),
			Reason:               strings.TrimSpace(issue.Reason),
		})
	}
	for _, issue := range state.ZeroMatchFilterViolations {
		aliases := diagnosticIssueAliases(issue.Aliases, issue.ArtifactID)
		inputAlias := strings.TrimSpace(issue.InputPath)
		if inputAlias == "" && len(aliases) > 0 {
			inputAlias = aliases[0]
		}
		additional = append(additional, NewActionInputViolation(
			"zero_match_filter",
			"warning",
			RepairNeedsTypedAction,
			dataquery.DataAction{
				Kind:           dataquery.DataActionFilterRecords,
				InputPaths:     cleanStrings([]string{inputAlias}),
				OutputArtifact: firstNonEmpty(issue.ArtifactID, strings.Join(aliases, ",")),
			},
			inputAlias,
			issue.FilterFields,
			issue.Reason,
			issue.RepairActionHints,
		))
	}
	for _, issue := range state.UnmatchedResolutionViolations {
		aliases := diagnosticIssueAliases(issue.Aliases, issue.ArtifactID)
		inputAlias := strings.TrimSpace(issue.BasePath)
		if inputAlias == "" && len(aliases) > 0 {
			inputAlias = aliases[0]
		}
		additional = append(additional, NewActionInputViolation(
			"unmatched_resolution",
			"warning",
			RepairNeedsTypedAction,
			dataquery.DataAction{
				Kind:           dataquery.DataActionApplyResolutions,
				InputPaths:     cleanStrings([]string{inputAlias}),
				OutputArtifact: firstNonEmpty(issue.ArtifactID, strings.Join(aliases, ",")),
			},
			inputAlias,
			issue.TargetFields,
			issue.Reason,
			issue.RepairActionHints,
		))
	}
	for _, issue := range state.ZeroEligibleViolations {
		aliases := diagnosticIssueAliases(issue.Aliases, issue.ArtifactID)
		inputAlias := strings.TrimSpace(issue.InputPath)
		if inputAlias == "" && len(aliases) > 0 {
			inputAlias = aliases[0]
		}
		additional = append(additional, NewActionInputViolation(
			"zero_eligible_records",
			"warning",
			RepairNeedsTypedAction,
			dataquery.DataAction{
				Kind:           dataquery.DataActionQualifyRecords,
				InputPaths:     cleanStrings([]string{inputAlias}),
				OutputArtifact: firstNonEmpty(issue.ArtifactID, strings.Join(aliases, ",")),
			},
			inputAlias,
			nil,
			issue.Reason,
			issue.RepairActionHints,
		))
	}
	return BuildWorkflowViolations(WorkflowViolationInput{
		Facts:               state.Facts(),
		Records:             input.Records,
		NoProgressThreshold: input.NoProgressThreshold,
		GuardViolations:     input.GuardViolations,
		Additional:          additional,
	})
}

func WorkflowViolationsFromRecordExecution(records []WorkflowRecord) []WorkflowViolation {
	var out []WorkflowViolation
	for _, rec := range records {
		for _, violation := range rec.Violations {
			projected := WorkflowViolationFromDataTaskViolation(violation, matchingViolationAction(rec.Plan, violation))
			if strings.TrimSpace(projected.Code) == "" {
				continue
			}
			out = append(out, projected)
		}
	}
	return out
}

func WorkflowViolationFromDataTaskViolation(violation dataquery.DataTaskViolation, action dataquery.DataAction) WorkflowViolation {
	code := strings.TrimSpace(violation.Code)
	if code == "" {
		return WorkflowViolation{}
	}
	severity := "error"
	if strings.TrimSpace(violation.Source) == dataquery.DataViolationSourceErrorText && code == "runtime_failure" {
		severity = "warning"
	}
	inputAliases := cleanActionAliases(violation.InputAliases)
	projected := NewGenericViolation(GenericViolationInput{
		Code:               code,
		Severity:           severity,
		Repairability:      workflowRepairabilityForDataViolation(violation),
		Action:             action,
		InputAlias:         strings.TrimSpace(violation.InputAlias),
		InputAliases:       inputAliases,
		MissingFields:      append([]string(nil), violation.MissingFields...),
		AvailableFields:    append([]string(nil), violation.AvailableFieldSample...),
		RepairActionHints:  cleanStrings([]string{violation.RepairHint}),
		Reason:             firstNonEmptyViolationText(violation.Summary, violation.ActualSnippet, code),
		CandidateArtifacts: nil,
	})
	if strings.TrimSpace(projected.Field) == "" {
		projected.Field = strings.TrimSpace(violation.Field)
	}
	if strings.TrimSpace(projected.Operation) == "" {
		projected.Operation = strings.TrimSpace(violation.Operation)
	}
	if strings.TrimSpace(projected.Param) == "" {
		projected.Param = strings.TrimSpace(violation.Param)
	}
	if strings.TrimSpace(projected.Role) == "" {
		projected.Role = strings.TrimSpace(violation.Role)
	}
	if strings.TrimSpace(projected.ActionID) == "" {
		projected.ActionID = strings.TrimSpace(violation.ActionID)
	}
	if strings.TrimSpace(projected.ActionKind) == "" {
		projected.ActionKind = strings.TrimSpace(violation.ActionKind)
	}
	if strings.TrimSpace(projected.InputAlias) == "" && len(inputAliases) > 0 {
		projected.InputAlias = inputAliases[0]
	}
	if len(projected.InputAliases) == 0 {
		projected.InputAliases = cleanActionAliases(append([]string{violation.InputAlias}, violation.InputAliases...))
	}
	if len(projected.MissingFields) == 0 {
		projected.MissingFields = cleanStrings(violation.MissingFields)
	}
	if len(projected.AvailableFieldSample) == 0 {
		projected.AvailableFieldSample = cleanStrings(violation.AvailableFieldSample)
	}
	if strings.TrimSpace(projected.Reason) == "" {
		projected.Reason = code
	}
	if projected.Limit == 0 {
		projected.Limit = violation.Limit
	}
	if projected.Observed == 0 {
		projected.Observed = violation.Observed
	}
	return projected
}

func matchingViolationAction(plan dataquery.TaskPlan, violation dataquery.DataTaskViolation) dataquery.DataAction {
	actionID := strings.TrimSpace(violation.ActionID)
	actionKind := strings.TrimSpace(violation.ActionKind)
	for _, action := range plan.Actions {
		if actionID != "" && strings.TrimSpace(action.ID) == actionID {
			return action
		}
	}
	for _, action := range plan.Actions {
		kind := strings.TrimSpace(string(NormalizeActionKind(action.Kind)))
		if actionKind != "" && kind == actionKind {
			return action
		}
	}
	return dataquery.DataAction{
		ID:         actionID,
		Kind:       dataquery.DataActionKind(actionKind),
		InputPaths: cleanActionAliases(append([]string{violation.InputAlias}, violation.InputAliases...)),
	}
}

func workflowRepairabilityForDataViolation(violation dataquery.DataTaskViolation) ViolationRepairability {
	switch strings.TrimSpace(violation.Code) {
	case "field_contract_violation", "material_contract_violation", "action_dependency_violation":
		return RepairNeedsTypedAction
	}
	switch violation.Repairability {
	case dataquery.RepairabilitySafePatch:
		return RepairSafePatch
	case dataquery.RepairabilityNeedsClarification:
		return RepairNeedsClarification
	case dataquery.RepairabilityNeedsRecompute:
		return RepairNeedsRecompute
	default:
		return RepairNeedsTypedAction
	}
}

func firstNonEmptyViolationText(values ...string) string {
	for _, value := range values {
		if text := strings.TrimSpace(value); text != "" {
			return text
		}
	}
	return ""
}

func BuildWorkflowViolations(input WorkflowViolationInput) []WorkflowViolation {
	out := append([]WorkflowViolation(nil), input.GuardViolations...)
	events := input.ProgressEvents
	if len(events) == 0 {
		events = ProgressEventsFromRecords(input.Records)
	}
	if violation, ok := RelationNoProgressViolation(input.Facts, events, input.NoProgressThreshold); ok {
		out = append(out, violation)
	}
	out = append(out, input.Additional...)
	return out
}

func BuildWorkflowViolationSummary(violations []WorkflowViolation) WorkflowViolationSummary {
	if len(violations) == 0 {
		return WorkflowViolationSummary{}
	}
	counts := map[string]int{}
	summary := WorkflowViolationSummary{}
	for _, violation := range violations {
		code := strings.TrimSpace(violation.Code)
		if code == "" {
			continue
		}
		summary.Total++
		counts[code]++
		switch strings.ToLower(strings.TrimSpace(violation.Severity)) {
		case "warning", "warn":
			summary.WarningCount++
		default:
			summary.ErrorCount++
		}
		if summary.FirstCode == "" {
			summary.FirstCode = code
			summary.FirstSeverity = strings.TrimSpace(violation.Severity)
			summary.FirstRepairability = violation.Repairability
			summary.FirstActionID = strings.TrimSpace(violation.ActionID)
			summary.FirstActionKind = strings.TrimSpace(violation.ActionKind)
			summary.FirstInputAlias = strings.TrimSpace(violation.InputAlias)
			summary.FirstOutputAlias = strings.TrimSpace(violation.OutputAlias)
			summary.FirstField = strings.TrimSpace(violation.Field)
			summary.FirstOperation = strings.TrimSpace(violation.Operation)
			summary.FirstParam = strings.TrimSpace(violation.Param)
			summary.FirstRole = strings.TrimSpace(violation.Role)
			summary.FirstMissingFields = cleanStrings(violation.MissingFields)
			summary.FirstRepairActionHints = cleanStrings(violation.RepairActionHints)
			summary.FirstLimit = violation.Limit
			summary.FirstObserved = violation.Observed
			summary.FirstReason = strings.TrimSpace(violation.Reason)
		}
	}
	if summary.Total == 0 {
		return WorkflowViolationSummary{}
	}
	codes := make([]string, 0, len(counts))
	for code := range counts {
		codes = append(codes, code)
	}
	sort.Strings(codes)
	for _, code := range codes {
		summary.ByCode = append(summary.ByCode, WorkflowViolationCodeCount{Code: code, Count: counts[code]})
	}
	return summary
}

func CloneWorkflowViolationSummary(in WorkflowViolationSummary) WorkflowViolationSummary {
	out := in
	out.ByCode = append([]WorkflowViolationCodeCount(nil), in.ByCode...)
	out.FirstMissingFields = append([]string(nil), in.FirstMissingFields...)
	out.FirstRepairActionHints = append([]string(nil), in.FirstRepairActionHints...)
	return out
}

func firstNonEmptyWorkflowViolationSummary(values ...WorkflowViolationSummary) WorkflowViolationSummary {
	for _, value := range values {
		if value.Total > 0 {
			return CloneWorkflowViolationSummary(value)
		}
	}
	return WorkflowViolationSummary{}
}

func diagnosticIssueAliases(aliases []string, artifactID string) []string {
	out := append([]string(nil), aliases...)
	if strings.TrimSpace(artifactID) != "" {
		out = append(out, artifactID)
	}
	return cleanStrings(out)
}
