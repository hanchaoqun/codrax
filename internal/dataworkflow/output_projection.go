package dataworkflow

import "github.com/hanchaoqun/codrax/internal/dataquery"

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
