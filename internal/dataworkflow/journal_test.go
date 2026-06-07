package dataworkflow

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/dataquery"
)

func TestWorkflowJournalJSONContract(t *testing.T) {
	raw, err := json.Marshal(WorkflowJournal{
		Status:       "failed",
		DataRounds:   2,
		RepairRounds: 1,
		ActionEvents: []ActionEvent{{
			Status: ActionStatusExecuted,
		}},
		ActionGraph: ActionGraph{Deferred: []ActionNode{{ID: "join_next", Status: ActionStatusDeferred}}},
		ProcessEvents: []WorkflowJournalEvent{{
			Kind:          "evaluate",
			Round:         2,
			Goal:          "answer the data question",
			BatchPurpose:  "compute current batch",
			NextStep:      "assemble final answer",
			ActionSummary: "extract:extract_records",
			AuditDetails:  []string{"2 consumed material(s)"},
		}},
	})
	if err != nil {
		t.Fatalf("marshal WorkflowJournal: %v", err)
	}
	text := string(raw)
	for _, want := range []string{"data_rounds", "repair_rounds", "action_events", "action_graph", "process_events", "join_next", "batch_purpose", "next_step", "action_summary", "audit_details"} {
		if !strings.Contains(text, want) {
			t.Fatalf("journal json missing %q: %s", want, text)
		}
	}
}

func TestBuildWorkflowProcessEventUsesTypedPlanIntent(t *testing.T) {
	event := BuildWorkflowProcessEvent(WorkflowProcessEventInput{
		Kind:   "execute",
		Round:  2,
		Status: "running",
		Plan: dataquery.TaskPlan{
			Goal:         "answer the requested data question",
			WhyThisBatch: "materialize normalized records",
			NextBatch:    "compute grouped contributions",
			Actions: []dataquery.DataAction{{
				ID:      "derive",
				Kind:    dataquery.DataActionDeriveFields,
				Purpose: "derive reusable fields from the current record artifact",
			}},
		},
	})
	if event.Goal != "answer the requested data question" || event.BatchPurpose != "materialize normalized records" || event.NextStep != "compute grouped contributions" {
		t.Fatalf("event intent fields not preserved: %+v", event)
	}
	if !strings.Contains(event.ActionSummary, "derive reusable fields") {
		t.Fatalf("ActionSummary=%q, want action purpose", event.ActionSummary)
	}
}

func TestBuildWorkflowProcessEventAddsNeutralResultAudit(t *testing.T) {
	result := dataquery.Result{
		ConsumedPaths: []string{"orders.csv"},
		Contributions: []dataquery.ContributionRecord{{
			ItemID:    dataquery.LooseText("row-1"),
			GroupKey:  dataquery.LooseText("A"),
			Metric:    dataquery.LooseText("value"),
			Value:     dataquery.LooseText("10"),
			Operation: dataquery.LooseText("add"),
		}},
	}
	event := BuildWorkflowProcessEvent(WorkflowProcessEventInput{
		Kind:   "result",
		Round:  1,
		Result: &result,
	})
	text := strings.Join(event.AuditDetails, "\n")
	for _, want := range []string{"consumed_materials=1", "contributions=1"} {
		if !strings.Contains(text, want) {
			t.Fatalf("audit details missing %q: %+v", want, event.AuditDetails)
		}
	}
}
