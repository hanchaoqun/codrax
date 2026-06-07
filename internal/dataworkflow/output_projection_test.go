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
