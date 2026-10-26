package types

import (
	"reflect"
	"testing"
)

func TestHMC292BudgetViewSeparatesRosterAndHistoricalLowerBound(t *testing.T) {
	for _, historical := range []int{-1, 0, 1, 4} {
		run := WriteWorkflowRun{Budget: WriteWorkflowBudget{MaxBatches: 5, BatchesUsed: historical}, Batches: []WriteWorkflowBatch{{ID: " one "}, {ID: "one"}, {ID: ""}, {ID: "two"}}}
		before := run
		view := WriteWorkflowBatchBudget(run)
		if view.RosterBatches != 2 || view.RecordedUsed != max(0, historical) || view.Used != max(2, historical) || view.Limit != 5 || !reflect.DeepEqual(before, run) {
			t.Fatalf("view must not count duplicate/blank identities or refund history: %+v run=%+v", view, run)
		}
	}
}
