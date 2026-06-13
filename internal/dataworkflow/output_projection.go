package dataworkflow

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/dataquery"
)

const (
	OutputProjectionStatusSatisfied           = "satisfied"
	OutputProjectionStatusMissingAnswer       = "missing_answer"
	OutputProjectionStatusMissingProjection   = "missing_projection"
	OutputProjectionStatusIncompleteReference = "incomplete_reference"

	OutputProjectionReasonSatisfied           = "satisfied"
	OutputProjectionReasonNoAnswer            = "no_answer"
	OutputProjectionReasonStrictContract      = "strict_contract_projection_required"
	OutputProjectionReasonReferenceIncomplete = "reference_projection_incomplete"
)

type OutputProjectionGraphInput struct {
	Output                    dataquery.OutputContract
	Coverage                  dataquery.CoverageContract
	AnswerPresent             bool
	ProjectionArtifactPresent bool
	ReconcilePresent          bool
	ReconcileGroups           int
	PlanHasCustomTransform    bool
	ReferenceGapPresent       bool
	ReferenceKeyCount         int
	AnswerItemCount           int
}

func BuildOutputProjectionGraph(input OutputProjectionGraphInput) OutputProjectionGraph {
	output := input.Output.Normalize()
	strict := OutputContractNeedsFinalProjection(output)
	answerPresent := input.AnswerPresent
	projectionArtifactPresent := input.ProjectionArtifactPresent
	reconcilePresent := input.ReconcilePresent || input.ReconcileGroups > 0
	referenceRequired := input.ReferenceGapPresent
	needsProjection := outputProjectionNeedsAssembly(input, strict, reconcilePresent)
	required := referenceRequired || needsProjection || !answerPresent
	status := OutputProjectionStatusSatisfied
	reason := OutputProjectionReasonSatisfied
	var missing []string
	switch {
	case referenceRequired:
		status = OutputProjectionStatusIncompleteReference
		reason = OutputProjectionReasonReferenceIncomplete
		missing = append(missing, "reference_complete_projection")
	case needsProjection:
		status = OutputProjectionStatusMissingProjection
		reason = OutputProjectionReasonStrictContract
		missing = append(missing, "assemble_answer")
	case !answerPresent:
		status = OutputProjectionStatusMissingAnswer
		reason = OutputProjectionReasonNoAnswer
		missing = append(missing, "answer")
	}
	return OutputProjectionGraph{
		Required:                  required,
		StrictContract:            strict,
		AnswerPresent:             answerPresent,
		ProjectionArtifactPresent: projectionArtifactPresent,
		ReconcilePresent:          reconcilePresent,
		ReconcileGroups:           input.ReconcileGroups,
		ReferenceCompleteRequired: referenceRequired,
		ReferenceComplete:         !referenceRequired,
		ReferenceKeyCount:         input.ReferenceKeyCount,
		AnswerItemCount:           input.AnswerItemCount,
		Status:                    status,
		ReasonCode:                reason,
		ProducesActions:           cleanStrings([]string{string(dataquery.DataActionAssembleAnswer)}),
		MissingPrerequisites:      cleanStrings(missing),
	}
}

func ResultAnswerPresent(result dataquery.Result) bool {
	return strings.TrimSpace(result.Answer) != "" && !dataquery.AnswerLooksLikeArtifactSummary(result.Answer)
}

func ResultIsFinalAnswerCandidate(plan dataquery.TaskPlan, result dataquery.Result, contract dataquery.CoverageContract, expected dataquery.OutputContract) bool {
	if !ResultAnswerPresent(result) {
		return false
	}
	if plan.ContinueAfter && !ResultHasAssembleAnswerArtifact(result) {
		return false
	}
	if !PlanMayProduceFinalAnswer(plan, result) {
		return false
	}
	if expected.Format != "" {
		actual := result.OutputContract.Normalize()
		want := expected.Normalize()
		if actual.Format == dataquery.OutputFreeform && want.Format != dataquery.OutputFreeform {
			return false
		}
	}
	if contract.RuleCoverageRequired && len(result.RuleCoverage) == 0 {
		return false
	}
	if contract.DecisionRecordsRequired && len(result.Rows) == 0 {
		return false
	}
	if contract.EntityResolutionRequired && len(result.EntityResolutions) == 0 {
		return false
	}
	if contract.ContributionLedgerRequired && len(result.Contributions) == 0 {
		return false
	}
	if contract.ReconcileRequired && result.Reconcile == nil {
		return false
	}
	return true
}

func PlanMayProduceFinalAnswer(plan dataquery.TaskPlan, result dataquery.Result) bool {
	if len(plan.Actions) == 0 {
		return true
	}
	if result.Reconcile != nil && (strings.TrimSpace(result.Reconcile.ActualAnswer.String()) != "" || strings.TrimSpace(result.Reconcile.ExpectedAnswer.String()) != "") {
		return true
	}
	for _, action := range plan.Actions {
		switch NormalizeActionKind(action.Kind) {
		case dataquery.DataActionCustomTransform:
			if strings.TrimSpace(action.Script) != "" {
				return true
			}
		case dataquery.DataActionReconcile:
			return true
		}
	}
	return false
}

func outputProjectionNeedsAssembly(input OutputProjectionGraphInput, strict, reconcilePresent bool) bool {
	if input.ReferenceGapPresent {
		return true
	}
	if !reconcilePresent {
		return false
	}
	if strict && !input.ProjectionArtifactPresent && !input.PlanHasCustomTransform {
		return true
	}
	if input.AnswerPresent {
		return false
	}
	if input.Output.Normalize().Format != "" && input.Output.Normalize().Format != dataquery.OutputFreeform {
		return true
	}
	if !input.Output.Normalize().ExplanationAllowed && input.Output.Normalize().Format != "" {
		return true
	}
	return input.Coverage.ReconcileRequired
}
