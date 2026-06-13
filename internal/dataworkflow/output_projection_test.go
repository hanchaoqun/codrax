package dataworkflow

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/dataquery"
)

func TestBuildOutputProjectionGraphAcceptsOrdinaryAnswer(t *testing.T) {
	graph := BuildOutputProjectionGraph(OutputProjectionGraphInput{
		AnswerPresent: true,
	})
	if graph.Status != OutputProjectionStatusSatisfied || graph.Required {
		t.Fatalf("graph=%+v, want satisfied ordinary answer", graph)
	}
}

func TestBuildOutputProjectionGraphRequiresStrictAssembleProjection(t *testing.T) {
	graph := BuildOutputProjectionGraph(OutputProjectionGraphInput{
		Output:           dataquery.OutputContract{Format: dataquery.OutputPlainSingleLine, ExplanationAllowed: false},
		AnswerPresent:    true,
		ReconcilePresent: true,
		ReconcileGroups:  2,
	})
	if graph.Status != OutputProjectionStatusMissingProjection || !graph.StrictContract {
		t.Fatalf("graph=%+v, want strict missing projection", graph)
	}
}

func TestBuildOutputProjectionGraphReportsReferenceIncomplete(t *testing.T) {
	graph := BuildOutputProjectionGraph(OutputProjectionGraphInput{
		Output:              dataquery.OutputContract{Format: dataquery.OutputPlainSingleLine, ExplanationAllowed: false},
		AnswerPresent:       true,
		ReferenceGapPresent: true,
		ReferenceKeyCount:   3,
		AnswerItemCount:     2,
	})
	if graph.Status != OutputProjectionStatusIncompleteReference || !graph.ReferenceCompleteRequired || graph.ReferenceComplete {
		t.Fatalf("graph=%+v, want incomplete reference projection", graph)
	}
	if graph.ReferenceKeyCount != 3 || graph.AnswerItemCount != 2 {
		t.Fatalf("graph=%+v, want reference/answer counts", graph)
	}
}

func TestResultIsFinalAnswerCandidateUsesTypedOutputPolicy(t *testing.T) {
	contract := dataquery.CoverageContract{ContributionLedgerRequired: true}
	expected := dataquery.OutputContract{Format: dataquery.OutputPlainSingleLine}
	artifactSummary := dataquery.Result{
		Answer:         "4 artifact(s)",
		OutputContract: expected,
		Contributions:  []dataquery.ContributionRecord{{GroupKey: dataquery.LooseText("g"), Value: dataquery.LooseText("1")}},
	}
	if ResultIsFinalAnswerCandidate(dataquery.TaskPlan{}, artifactSummary, contract, expected) {
		t.Fatalf("artifact summary answer should not be a final answer candidate")
	}
	missingLedger := dataquery.Result{
		Answer:         "42",
		OutputContract: expected,
	}
	if ResultIsFinalAnswerCandidate(dataquery.TaskPlan{}, missingLedger, contract, expected) {
		t.Fatalf("answer without required contribution ledger should not be final")
	}
	ready := dataquery.Result{
		Answer:         "42",
		OutputContract: expected,
		Contributions:  []dataquery.ContributionRecord{{GroupKey: dataquery.LooseText("g"), Value: dataquery.LooseText("42")}},
	}
	if !ResultIsFinalAnswerCandidate(dataquery.TaskPlan{}, ready, contract, expected) {
		t.Fatalf("typed answer with required contribution ledger should be final")
	}
	continuePlan := dataquery.TaskPlan{ContinueAfter: true}
	if ResultIsFinalAnswerCandidate(continuePlan, ready, contract, expected) {
		t.Fatalf("continue_after plan should not be terminal final answer")
	}
	assembled := ready
	assembled.Artifacts = []dataquery.DataArtifact{{Kind: string(dataquery.DataActionAssembleAnswer)}}
	if !ResultIsFinalAnswerCandidate(continuePlan, assembled, contract, expected) {
		t.Fatalf("continue_after plan with assembled final projection should be terminal")
	}
}

func TestResultIsFinalAnswerCandidateAcceptsPromotedJSONOnlyPayloadWithoutLedgers(t *testing.T) {
	plan := dataquery.TaskPlan{
		OutputContract: dataquery.OutputContract{Format: dataquery.OutputJSONOnly, ExplanationAllowed: false},
		Actions: []dataquery.DataAction{{
			Kind:   dataquery.DataActionCustomTransform,
			Script: `emit({"ids":["u1","u3"]})`,
		}},
	}
	result := dataquery.Result{
		Answer:         `{"ids":["u1","u3"]}`,
		OutputContract: plan.OutputContract,
	}
	if !ResultIsFinalAnswerCandidate(plan, result, dataquery.CoverageContract{}, plan.OutputContract) {
		t.Fatalf("promoted json_only payload should satisfy output projection when no ledgers are declared")
	}
	withLedgerRequirement := dataquery.CoverageContract{ContributionLedgerRequired: true}
	if ResultIsFinalAnswerCandidate(plan, result, withLedgerRequirement, plan.OutputContract) {
		t.Fatalf("promoted payload must not bypass explicitly required contribution ledger")
	}
}

func TestResultIsFinalAnswerCandidatePromotesPreservedStrictAnswerAcrossContinuation(t *testing.T) {
	expected := dataquery.OutputContract{Format: dataquery.OutputJSONOnly, ExplanationAllowed: false}
	continuation := dataquery.TaskPlan{
		ContinueAfter: true,
		Actions: []dataquery.DataAction{{
			Kind: dataquery.DataActionMappingCandidate,
		}},
	}
	result := dataquery.Result{
		Answer:         `{"ids":["u1","u3"]}`,
		OutputContract: dataquery.OutputContract{Format: dataquery.OutputFreeform, ExplanationAllowed: true},
	}
	if !ResultIsPreservedAnswerHandoffCandidate(continuation, result, expected) {
		t.Fatalf("strict json answer carried through non-terminal continuation should be a handoff candidate")
	}
	if !ResultIsFinalAnswerCandidate(continuation, result, dataquery.CoverageContract{}, expected) {
		t.Fatalf("strict json handoff should satisfy final candidate checks")
	}

	invalid := result
	invalid.Answer = `ids: u1,u3`
	if ResultIsFinalAnswerCandidate(continuation, invalid, dataquery.CoverageContract{}, expected) {
		t.Fatalf("handoff must still satisfy the effective json_only output contract")
	}

	ledgerRequired := dataquery.CoverageContract{ContributionLedgerRequired: true}
	if ResultIsFinalAnswerCandidate(continuation, result, ledgerRequired, expected) {
		t.Fatalf("handoff must not bypass required workflow ledgers")
	}
}

func TestPlanMayProduceFinalAnswerAllowsReconcileAction(t *testing.T) {
	result := dataquery.Result{Reconcile: &dataquery.ReconcileReport{
		ActualAnswer: dataquery.LooseText("42"),
	}}
	plan := dataquery.TaskPlan{Actions: []dataquery.DataAction{{
		Kind: dataquery.DataActionReconcile,
	}}}
	if !PlanMayProduceFinalAnswer(plan, result) {
		t.Fatalf("reconcile action with reconcile answer should be able to produce final answer")
	}
}
