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
	OutputProjectionStatusGroundingMismatch   = "reference_grounding_mismatch"

	OutputProjectionReasonSatisfied           = "satisfied"
	OutputProjectionReasonNoAnswer            = "no_answer"
	OutputProjectionReasonStrictContract      = "strict_contract_projection_required"
	OutputProjectionReasonReferenceIncomplete = "reference_projection_incomplete"
	OutputProjectionReasonGroundingMismatch   = "reference_grounding_mismatch"
)

type OutputProjectionGraphInput struct {
	Output                        dataquery.OutputContract
	Coverage                      dataquery.CoverageContract
	AnswerPresent                 bool
	ProjectionArtifactPresent     bool
	ReconcilePresent              bool
	ReconcileGroups               int
	PlanHasCustomTransform        bool
	ReferenceGapPresent           bool
	ReferenceCandidatePresent     bool
	ReferenceCandidateDeclared    bool
	ReferenceCandidatePath        string
	ReferenceCandidateField       string
	ReferenceGroundingEvaluated   bool
	ReferenceKeyCount             int
	AnswerItemCount               int
	ReferenceGroundingMismatch    bool
	ReferenceCardinalityMismatch  bool
	ReferenceLedgerDomainMismatch bool
	ReferenceMismatchCount        int
}

func BuildOutputProjectionGraph(input OutputProjectionGraphInput) OutputProjectionGraph {
	output := input.Output.Normalize()
	strict := OutputContractNeedsFinalProjection(output)
	answerPresent := input.AnswerPresent
	projectionArtifactPresent := input.ProjectionArtifactPresent
	reconcilePresent := input.ReconcilePresent || input.ReconcileGroups > 0
	referenceRequired := input.ReferenceGapPresent || input.ReferenceGroundingMismatch
	referenceDeclared := input.ReferenceCandidateDeclared || output.CompleteReference
	referenceCompleteRequired := referenceRequired || referenceDeclared
	referenceComplete := referenceDeclared &&
		input.ReferenceGroundingEvaluated &&
		input.AnswerPresent &&
		!referenceRequired
	needsProjection := outputProjectionNeedsAssembly(input, strict, reconcilePresent)
	required := referenceRequired || needsProjection || !answerPresent
	status := OutputProjectionStatusSatisfied
	reason := OutputProjectionReasonSatisfied
	var missing []string
	switch {
	case input.ReferenceLedgerDomainMismatch:
		status = OutputProjectionStatusGroundingMismatch
		reason = OutputProjectionReasonGroundingMismatch
		missing = append(missing, "reference_domain_alignment", "reconciled_reference_projection")
	case input.ReferenceGroundingMismatch && !input.ReferenceCardinalityMismatch:
		status = OutputProjectionStatusGroundingMismatch
		reason = OutputProjectionReasonGroundingMismatch
		missing = append(missing, "reference_grounded_projection")
	case input.ReferenceGapPresent:
		status = OutputProjectionStatusIncompleteReference
		reason = OutputProjectionReasonReferenceIncomplete
		missing = append(missing, "reference_complete_projection")
	case input.ReferenceGroundingMismatch:
		status = OutputProjectionStatusGroundingMismatch
		reason = OutputProjectionReasonGroundingMismatch
		missing = append(missing, "reference_grounded_projection")
	case needsProjection:
		status = OutputProjectionStatusMissingProjection
		reason = OutputProjectionReasonStrictContract
		missing = append(missing, "assemble_answer")
	case !answerPresent:
		status = OutputProjectionStatusMissingAnswer
		reason = OutputProjectionReasonNoAnswer
		missing = append(missing, "answer")
	}
	produces := []string{string(dataquery.DataActionAssembleAnswer)}
	if input.ReferenceLedgerDomainMismatch {
		produces = []string{
			string(dataquery.DataActionComputeContribs),
			string(dataquery.DataActionReconcile),
			string(dataquery.DataActionAssembleAnswer),
		}
	}
	return OutputProjectionGraph{
		Required:                      required,
		StrictContract:                strict,
		AnswerPresent:                 answerPresent,
		ProjectionArtifactPresent:     projectionArtifactPresent,
		ReconcilePresent:              reconcilePresent,
		ReconcileGroups:               input.ReconcileGroups,
		ReferenceCompleteRequired:     referenceCompleteRequired,
		ReferenceComplete:             referenceComplete,
		ReferenceCandidatePresent:     input.ReferenceCandidatePresent,
		ReferenceCandidateDeclared:    referenceDeclared,
		ReferenceCandidatePath:        strings.TrimSpace(input.ReferenceCandidatePath),
		ReferenceCandidateField:       strings.TrimSpace(input.ReferenceCandidateField),
		ReferenceGroundingEvaluated:   input.ReferenceGroundingEvaluated,
		ReferenceKeyCount:             input.ReferenceKeyCount,
		AnswerItemCount:               input.AnswerItemCount,
		ReferenceGroundingMismatch:    input.ReferenceGroundingMismatch,
		ReferenceCardinalityMismatch:  input.ReferenceCardinalityMismatch,
		ReferenceLedgerDomainMismatch: input.ReferenceLedgerDomainMismatch,
		ReferenceMismatchCount:        input.ReferenceMismatchCount,
		Status:                        status,
		ReasonCode:                    reason,
		ProducesActions:               cleanStrings(produces),
		MissingPrerequisites:          cleanStrings(missing),
	}
}

// NextStageWithOutputProjection gives a precise incomplete output projection
// authority over the otherwise terminal ledger stage. Business-ledger facts
// remain the primary stage machine; this only reopens the final projection
// lane after those facts would report complete.
func NextStageWithOutputProjection(facts StageFacts, graph OutputProjectionGraph) string {
	stage := NextStage(facts)
	if graph.ReferenceLedgerDomainMismatch {
		switch stage {
		case StageComputeContributions, StageReconcileArtifacts, StageEmitOutputContractAnswer, StageComplete:
			return StageRepairReferenceDomain
		}
	}
	if stage == StageComplete && graph.Status != "" && graph.Status != OutputProjectionStatusSatisfied {
		return StageEmitOutputContractAnswer
	}
	return stage
}

func ResultAnswerPresent(result dataquery.Result) bool {
	return strings.TrimSpace(result.Answer) != "" && !dataquery.AnswerLooksLikeArtifactSummary(result.Answer)
}

// ResultIsFinalAnswerCandidate judges whether one record's result can stand
// as the workflow's final answer. satisfaction carries the cross-round
// entity-stage materialization fact: the entity arm MUST use the single
// shared obligation predicate (GAP-3/G10, §29.142) — a caller that passes a
// zero-value satisfaction on a materialized workflow would re-open the
// validator/routing split-brain.
func ResultIsFinalAnswerCandidate(plan dataquery.TaskPlan, result dataquery.Result, contract dataquery.CoverageContract, expected dataquery.OutputContract, satisfaction dataquery.LedgerSatisfactionFacts) bool {
	if !ResultAnswerPresent(result) {
		return false
	}
	handoff := ResultIsPreservedAnswerHandoffCandidate(plan, result, expected)
	if plan.ContinueAfter && !ResultHasAssembleAnswerArtifact(result) && !handoff {
		return false
	}
	if !PlanMayProduceFinalAnswer(plan, result) && !handoff {
		return false
	}
	if !ResultAnswerSatisfiesOutputContract(result, expected) {
		return false
	}
	if contract.RuleCoverageRequired && len(result.RuleCoverage) == 0 {
		return false
	}
	if contract.DecisionRecordsRequired && len(result.Rows) == 0 {
		return false
	}
	if contract.EntityResolutionRequired &&
		!dataquery.EntityResolutionObligationSatisfied(len(result.EntityResolutions), satisfaction.EntityStageMaterialized || dataquery.ResultMaterializesEntityStage(result)) {
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

func ResultIsPreservedAnswerHandoffCandidate(plan dataquery.TaskPlan, result dataquery.Result, expected dataquery.OutputContract) bool {
	if !plan.ContinueAfter {
		return false
	}
	if ResultHasAssembleAnswerArtifact(result) {
		return false
	}
	if PlanMayProduceFinalAnswer(plan, result) {
		return false
	}
	return ResultAnswerSatisfiesOutputContract(result, expected)
}

// ResultHasDirectTerminalAnswerAuthority identifies a final answer emitted by
// a plan that is itself terminal and structurally allowed to publish an
// answer. This is distinct from an answer carried through a non-terminal
// helper batch: the latter uses ResultIsPreservedAnswerHandoffCandidate.
// Required-ledger admission remains owned by ResultIsFinalAnswerCandidate;
// this helper only prevents a second completion face from demanding an
// assemble_answer artifact after the final-candidate gate already accepted a
// direct custom_transform/reconcile answer.
func ResultHasDirectTerminalAnswerAuthority(plan dataquery.TaskPlan, result dataquery.Result, expected dataquery.OutputContract) bool {
	return !plan.ContinueAfter &&
		PlanMayProduceFinalAnswer(plan, result) &&
		ResultAnswerSatisfiesOutputContract(result, expected)
}

func ResultAnswerSatisfiesOutputContract(result dataquery.Result, expected dataquery.OutputContract) bool {
	if !ResultAnswerPresent(result) {
		return false
	}
	if strings.TrimSpace(string(expected.Format)) == "" {
		return true
	}
	return dataquery.ValidateAnswer(strings.TrimSpace(result.Answer), expected.Normalize()) == nil
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
