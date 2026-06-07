package dataworkflow

import (
	"encoding/json"
	"strings"
	"testing"
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
