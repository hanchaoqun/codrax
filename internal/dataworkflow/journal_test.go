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
			DeferredPlan: &dataquery.TaskPlan{Actions: []dataquery.DataAction{{
				ID:   "join_next",
				Kind: dataquery.DataActionJoinRecords,
			}}},
			DeferredEvents: []DeferredQueueEvent{{
				Action: DeferredQueueTransitionEnqueue,
				Round:  2,
			}},
		},
		LedgerGraph: BuildLedgerGraph(StageFacts{
			MaterialCoverageSufficient: true,
			ContributionLedgerRequired: true,
		}),
		OutputGraph: BuildOutputProjectionGraph(OutputProjectionGraphInput{
			Output:        dataquery.OutputContract{Format: dataquery.OutputPlainSingleLine, ExplanationAllowed: false},
			AnswerPresent: true,
		}),
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
			Decision:      &WorkflowDecision{Status: "continue", ReasonCode: "ledger_missing_contributions", Reason: "contributions are missing", NextActions: []string{"compute_contributions"}},
		}},
	})
	if err != nil {
		t.Fatalf("marshal WorkflowJournal: %v", err)
	}
	text := string(raw)
	for _, want := range []string{"data_rounds", "repair_rounds", "action_events", "action_graph", "deferred_queue", "deferred_plan", "deferred_events", "ledger_graph", "dependencies", "output_projection_graph", "artifact_graph", "executable_record_aliases", "progress", "repeated_signature_count", "decision", "process_events", "join_next", "batch_purpose", "next_step", "action_summary", "audit_details", "admission", "remainder_actions", "ledger_missing_contributions"} {
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

	blockedWithAllowed := BuildWorkflowDecision(WorkflowDecisionInput{
		AllowedNextActions: []string{string(dataquery.DataActionInspectMaterial)},
		Violations: []WorkflowViolation{{
			Code:              "field_contract_violation",
			Reason:            "missing normalized field",
			RepairActionHints: []string{string(dataquery.DataActionDeriveFields)},
		}},
	})
	if strings.Join(blockedWithAllowed.NextActions, ",") != string(dataquery.DataActionDeriveFields) {
		t.Fatalf("blockedWithAllowed.NextActions=%v, want violation repair hints to override broad allowed actions", blockedWithAllowed.NextActions)
	}
}

func TestBuildWorkflowDecisionUsesLedgerGraphBlocker(t *testing.T) {
	graph := BuildLedgerGraph(StageFacts{
		MaterialCoverageSufficient: true,
		ContributionLedgerRequired: true,
	})
	decision := BuildWorkflowDecision(WorkflowDecisionInput{
		NextStage:          StageComputeContributions,
		AllowedNextActions: []string{string(dataquery.DataActionDeriveFields), string(dataquery.DataActionComputeContribs)},
		LedgerGraph:        graph,
	})
	if decision.Status != "continue" || decision.ReasonCode != "ledger_missing_contributions" {
		t.Fatalf("decision=%+v, want missing contribution ledger reason", decision)
	}
	if !strings.Contains(decision.Reason, "required ledger contributions") {
		t.Fatalf("decision.Reason=%q, want ledger graph reason", decision.Reason)
	}
	if strings.Join(decision.NextActions, ",") != string(dataquery.DataActionComputeContribs) {
		t.Fatalf("decision.NextActions=%v, want graph producer action", decision.NextActions)
	}
}

func TestBuildWorkflowDecisionKeepsAllowedActionsWhenLedgerBlocked(t *testing.T) {
	graph := BuildLedgerGraph(StageFacts{
		MaterialCoverageSufficient: false,
		ContributionLedgerRequired: true,
	})
	decision := BuildWorkflowDecision(WorkflowDecisionInput{
		NextStage:          StageCoverRequiredMaterials,
		AllowedNextActions: []string{string(dataquery.DataActionInspectMaterial)},
		LedgerGraph:        graph,
	})
	if decision.Status != "continue" || decision.ReasonCode != "ledger_blocked_contributions" {
		t.Fatalf("decision=%+v, want blocked contribution ledger reason", decision)
	}
	if !strings.Contains(decision.Reason, LedgerPrerequisiteMaterials) {
		t.Fatalf("decision.Reason=%q, want material prerequisite", decision.Reason)
	}
	if strings.Join(decision.NextActions, ",") != string(dataquery.DataActionInspectMaterial) {
		t.Fatalf("decision.NextActions=%v, want allowed prerequisite action preserved", decision.NextActions)
	}
}

func TestBuildWorkflowDecisionUsesOutputProjectionGraph(t *testing.T) {
	outputGraph := BuildOutputProjectionGraph(OutputProjectionGraphInput{
		Output:          dataquery.OutputContract{Format: dataquery.OutputPlainSingleLine, ExplanationAllowed: false},
		ReconcileGroups: 2,
	})
	decision := BuildWorkflowDecision(WorkflowDecisionInput{
		AllowedNextActions: []string{string(dataquery.DataActionComputeContribs), string(dataquery.DataActionAssembleAnswer)},
		OutputGraph:        outputGraph,
	})
	if decision.Status != "continue" || decision.ReasonCode != "output_missing_projection" {
		t.Fatalf("decision=%+v, want output projection graph reason", decision)
	}
	if strings.Join(decision.NextActions, ",") != string(dataquery.DataActionAssembleAnswer) {
		t.Fatalf("decision.NextActions=%v, want assemble_answer producer", decision.NextActions)
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

func TestBuildWorkflowProcessEventCarriesDecision(t *testing.T) {
	event := BuildWorkflowProcessEvent(WorkflowProcessEventInput{
		Kind: "evaluate",
		Plan: dataquery.TaskPlan{Goal: "answer", WhyThisBatch: "check progress"},
		Decision: WorkflowDecision{
			Status:      "continue",
			ReasonCode:  "ledger_missing_contributions",
			Reason:      "contribution ledger is still missing",
			NextActions: []string{string(dataquery.DataActionComputeContribs)},
		},
	})
	if event.Decision == nil || event.Decision.ReasonCode != "ledger_missing_contributions" {
		t.Fatalf("event.Decision=%+v, want decision carried", event.Decision)
	}
	details := strings.Join(event.AuditDetails, ",")
	for _, want := range []string{"decision_status=continue", "decision_reason_code=ledger_missing_contributions", "decision_next_actions=compute_contributions"} {
		if !strings.Contains(details, want) {
			t.Fatalf("AuditDetails=%v, want %q", event.AuditDetails, want)
		}
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
