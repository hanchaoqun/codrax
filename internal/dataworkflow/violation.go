package dataworkflow

import (
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
	InputAlias           string                 `json:"input_alias,omitempty"`
	InputAliases         []string               `json:"input_aliases,omitempty"`
	OutputAlias          string                 `json:"output_alias,omitempty"`
	IdempotencyKey       string                 `json:"idempotency_key,omitempty"`
	DependencyRank       int                    `json:"dependency_rank,omitempty"`
	MissingFields        []string               `json:"missing_fields,omitempty"`
	AvailableFieldSample []string               `json:"available_field_sample,omitempty"`
	CandidateArtifacts   []string               `json:"candidate_artifacts,omitempty"`
	RepairActionHints    []string               `json:"repair_action_hints,omitempty"`
	Reason               string                 `json:"reason,omitempty"`
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

func BuildWorkflowStateViolations(input WorkflowStateViolationInput) []WorkflowViolation {
	state := input.State
	additional := make([]WorkflowViolation, 0,
		len(state.FieldContractViolations)+
			len(state.ZeroMatchFilterViolations)+
			len(state.UnmatchedResolutionViolations)+
			len(state.ZeroEligibleViolations))
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

func diagnosticIssueAliases(aliases []string, artifactID string) []string {
	out := append([]string(nil), aliases...)
	if strings.TrimSpace(artifactID) != "" {
		out = append(out, artifactID)
	}
	return cleanStrings(out)
}
