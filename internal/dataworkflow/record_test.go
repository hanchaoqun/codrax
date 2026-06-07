package dataworkflow

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/dataquery"
)

func TestWorkflowRecordPreservesCheckpointJSONShape(t *testing.T) {
	raw, err := json.Marshal(WorkflowRecord{
		Plan: dataquery.TaskPlan{
			Status: "ready",
			Actions: []dataquery.DataAction{{
				ID:   "filter",
				Kind: dataquery.DataActionFilterRecords,
			}},
		},
		Result: &dataquery.Result{Answer: "ok"},
		Err:    "previous error",
		Admission: &ActionDAGAdmissionDecision{
			Reason: "split dependency rank",
		},
	})
	if err != nil {
		t.Fatalf("marshal WorkflowRecord: %v", err)
	}
	text := string(raw)
	for _, want := range []string{"Plan", "Result", "Err", "Admission", "split dependency rank"} {
		if !strings.Contains(text, want) {
			t.Fatalf("workflow record json missing %q: %s", want, text)
		}
	}
}

func TestBuildWorkflowJournalEventsFromWorkflowRecords(t *testing.T) {
	records := []WorkflowRecord{{
		Plan: dataquery.TaskPlan{
			Status:       "ready",
			Goal:         "answer the data task",
			WhyThisBatch: "filter records",
			Actions: []dataquery.DataAction{{
				ID:      "filter",
				Kind:    dataquery.DataActionFilterRecords,
				Purpose: "keep relevant rows",
			}},
		},
		Err: "field contract failed",
		Admission: &ActionDAGAdmissionDecision{
			Rewritten: true,
			Reason:    "split dependency rank",
			Remainder: dataquery.TaskPlan{Actions: []dataquery.DataAction{{
				ID:   "compute",
				Kind: dataquery.DataActionComputeContribs,
			}}},
		},
	}}
	events := BuildWorkflowJournalEvents(records)
	if len(events) != 2 {
		t.Fatalf("events len=%d, want 2: %#v", len(events), events)
	}
	if events[0].Kind != "admission" || events[0].Admission == nil || events[0].Admission.RemainderActions != 1 {
		t.Fatalf("admission event=%#v", events[0])
	}
	if events[1].Kind != "action_batch" || events[1].Status != "failed" {
		t.Fatalf("batch event=%#v", events[1])
	}
	if !strings.Contains(events[1].Reason, "field contract failed") {
		t.Fatalf("batch reason=%q", events[1].Reason)
	}
}
