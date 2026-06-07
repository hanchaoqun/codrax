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
		ActionGraph: ActionGraph{
			Deferred: []ActionNode{{ID: "join_next", Status: ActionStatusDeferred}},
			DeferredQueue: DeferredQueueSnapshot{
				Actions:         1,
				FirstActionID:   "join_next",
				FirstActionKind: string(dataquery.DataActionJoinRecords),
			},
		},
		ArtifactGraph: ArtifactGraphState{Nodes: []ArtifactGraphNode{{
			ID:                    "records",
			PrimaryAlias:          "records.json",
			ExecutableRecordInput: true,
		}}, NodeCount: 1, ExecutableRecordAliases: []string{"records.json"}},
		Progress: ProgressWindow{Latest: ProgressFrame{Round: 2, Signature: "rows=2"}, RepeatedSignatureCount: 1},
		Decision: WorkflowDecision{Status: "continue", ReasonCode: "compute_contributions", NextActions: []string{"compute_contributions"}},
		ProcessEvents: []WorkflowJournalEvent{{
			Kind:          "evaluate",
			Round:         2,
			Goal:          "answer the data question",
			BatchPurpose:  "compute current batch",
			NextStep:      "assemble final answer",
			ActionSummary: "extract:extract_records",
			AuditDetails:  []string{"2 consumed material(s)"},
			Admission:     &ActionDAGAdmissionSummary{Status: "rewritten", RemainderActions: 1},
		}},
	})
	if err != nil {
		t.Fatalf("marshal WorkflowJournal: %v", err)
	}
	text := string(raw)
	for _, want := range []string{"data_rounds", "repair_rounds", "action_events", "action_graph", "deferred_queue", "artifact_graph", "executable_record_aliases", "progress", "repeated_signature_count", "decision", "process_events", "join_next", "batch_purpose", "next_step", "action_summary", "audit_details", "admission", "remainder_actions"} {
		if !strings.Contains(text, want) {
			t.Fatalf("journal json missing %q: %s", want, text)
		}
	}
}

func TestBuildAdmissionProcessEventSummarizesDecision(t *testing.T) {
	decision := ActionDAGAdmissionDecision{
		Plan: dataquery.TaskPlan{Actions: []dataquery.DataAction{{
			ID:      "derive",
			Kind:    dataquery.DataActionDeriveFields,
			Purpose: "derive fields",
		}}},
		Remainder: dataquery.TaskPlan{Actions: []dataquery.DataAction{{
			ID:   "compute",
			Kind: dataquery.DataActionComputeContribs,
		}}},
		Guard:     NewGuardResult("intra_batch_dependency", "error", RepairNeedsTypedAction, "split dependency"),
		Reason:    "split dependency rank",
		Rewritten: true,
	}
	event := BuildAdmissionProcessEvent(3, decision)
	if event.Kind != "admission" || event.Status != "rewritten" || event.Admission == nil {
		t.Fatalf("event=%+v, want rewritten admission event", event)
	}
	if event.Admission.RemainderActions != 1 || event.Admission.GuardCode != "intra_batch_dependency" {
		t.Fatalf("Admission=%+v, want remainder and guard code", event.Admission)
	}
	if !strings.Contains(strings.Join(event.AuditDetails, ","), "admission_remainder_actions=1") {
		t.Fatalf("AuditDetails=%v, want admission details", event.AuditDetails)
	}
}

func TestBuildWorkflowDecisionFromTypedState(t *testing.T) {
	decision := BuildWorkflowDecision(WorkflowDecisionInput{
		NextStage:          StageComputeContributions,
		AllowedNextActions: []string{string(dataquery.DataActionComputeContribs)},
	})
	if decision.Status != "continue" || decision.ReasonCode != StageComputeContributions || !strings.Contains(strings.Join(decision.NextActions, ","), string(dataquery.DataActionComputeContribs)) {
		t.Fatalf("decision=%+v, want continue compute_contributions", decision)
	}

	blocked := BuildWorkflowDecision(WorkflowDecisionInput{
		Violations: []WorkflowViolation{{
			Code:              "field_contract_violation",
			Reason:            "missing amount field",
			RepairActionHints: []string{string(dataquery.DataActionDeriveFields)},
		}},
	})
	if blocked.Status != "blocked" || blocked.ReasonCode != "field_contract_violation" || blocked.Reason != "missing amount field" || !strings.Contains(strings.Join(blocked.NextActions, ","), string(dataquery.DataActionDeriveFields)) {
		t.Fatalf("blocked decision=%+v, want typed violation decision", blocked)
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
