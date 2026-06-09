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
		PlanTransitions: []PlanTransitionEvent{{
			Source:          "continue",
			Round:           2,
			Actions:         1,
			FirstActionKind: string(dataquery.DataActionComputeContribs),
		}},
	})
	if err != nil {
		t.Fatalf("marshal WorkflowJournal: %v", err)
	}
	text := string(raw)
	for _, want := range []string{"data_rounds", "repair_rounds", "action_events", "action_graph", "deferred_queue", "deferred_plan", "deferred_events", "ledger_graph", "dependencies", "output_projection_graph", "artifact_graph", "executable_record_aliases", "progress", "repeated_signature_count", "decision", "process_events", "plan_transitions", "join_next", "batch_purpose", "next_step", "action_summary", "audit_details", "admission", "remainder_actions", "ledger_missing_contributions"} {
		if !strings.Contains(text, want) {
			t.Fatalf("journal json missing %q: %s", want, text)
		}
	}
}

func TestWorkflowRuntimeBuildJournalSnapshotUsesRuntimeState(t *testing.T) {
	rt := NewWorkflowRuntime(dataquery.TaskPlan{})
	current := rt.SwitchCurrentPlan(2, "continue", dataquery.TaskPlan{Actions: []dataquery.DataAction{{
		ID:   "compute",
		Kind: dataquery.DataActionComputeContribs,
	}}}, "next")
	rt.SetDeferredQueue(NewDeferredQueue(dataquery.TaskPlan{Actions: []dataquery.DataAction{{
		ID:   "assemble",
		Kind: dataquery.DataActionAssembleAnswer,
	}}}))
	rt.AppendRecord(WorkflowRecord{
		Plan: current,
		Result: &dataquery.Result{
			Answer: "42",
			Artifacts: []dataquery.DataArtifact{{
				ID: "result",
			}},
		},
	})
	previewRecords := rt.RecordsWith(WorkflowRecord{Plan: current, Err: "blocked"})
	snapshot := rt.BuildJournalSnapshot(WorkflowJournalBuildInput{
		Status:                     "checkpoint",
		Reason:                     "blocked",
		DataRounds:                 2,
		RepairRounds:               1,
		Records:                    previewRecords,
		CurrentPlan:                current,
		Decision:                   WorkflowDecision{ReasonCode: "existing"},
		DecisionFallbackReasonCode: "fallback",
		Guards: []GuardResult{NewGuardResult("field_contract", "error", RepairNeedsTypedAction, "missing field", WorkflowViolation{
			Code:          "field_contract",
			MissingFields: []string{"amount"},
		})},
		GuardRound: 2,
	})
	if snapshot.RecordCount != 2 || snapshot.Status != "checkpoint" || snapshot.Reason != "blocked" {
		t.Fatalf("snapshot header=%+v", snapshot)
	}
	if len(snapshot.PlanTransitions) != 1 || snapshot.PlanTransitions[0].Source != "continue" {
		t.Fatalf("PlanTransitions=%+v", snapshot.PlanTransitions)
	}
	if len(snapshot.ActionEvents) != 2 || snapshot.ActionEvents[1].Status != ActionStatusFailed {
		t.Fatalf("ActionEvents=%+v", snapshot.ActionEvents)
	}
	if len(snapshot.ProcessEvents) < 3 || snapshot.ProcessEvents[len(snapshot.ProcessEvents)-1].Kind != "guard" {
		t.Fatalf("ProcessEvents=%+v", snapshot.ProcessEvents)
	}
	if len(snapshot.WorkflowViolations) != 1 || snapshot.WorkflowViolations[0].Code != "field_contract" {
		t.Fatalf("WorkflowViolations=%+v", snapshot.WorkflowViolations)
	}
	if snapshot.Decision.Status != "checkpoint" || snapshot.Decision.ReasonCode != "existing" || snapshot.Decision.Reason != "blocked" {
		t.Fatalf("Decision=%+v", snapshot.Decision)
	}
	if snapshot.Resume == nil || len(snapshot.Resume.Records) == 0 || len(snapshot.Resume.CurrentPlan) == 0 || len(snapshot.Resume.DeferredPlan) == 0 {
		t.Fatalf("Resume=%+v", snapshot.Resume)
	}
	if len(rt.Records()) != 1 {
		t.Fatalf("preview records mutated runtime records: %+v", rt.Records())
	}
}

func TestWorkflowJournalCompleteMovesPriorErrorToLineage(t *testing.T) {
	rt := NewWorkflowRuntime(dataquery.TaskPlan{})
	rt.AppendRecord(WorkflowRecord{
		Plan: dataquery.TaskPlan{
			Status: "ready",
			Actions: []dataquery.DataAction{{
				ID:   "filter_active",
				Kind: dataquery.DataActionFilterRecords,
			}},
		},
		Err: "data planning incomplete: action 1 references missing field status",
	})
	snapshot := rt.BuildJournalSnapshot(WorkflowJournalBuildInput{
		Status:    "complete",
		LastError: "data planning incomplete: action 1 references missing field status",
	})

	if snapshot.Status != "complete" {
		t.Fatalf("Status=%q, want complete", snapshot.Status)
	}
	if snapshot.LastError != "" {
		t.Fatalf("LastError=%q, want empty final-state error for complete terminal", snapshot.LastError)
	}
	if snapshot.LastNonterminalError != "data planning incomplete: action 1 references missing field status" {
		t.Fatalf("LastNonterminalError=%q", snapshot.LastNonterminalError)
	}
	if len(snapshot.PriorErrors) != 1 {
		t.Fatalf("PriorErrors=%+v, want one prior error", snapshot.PriorErrors)
	}
	prior := snapshot.PriorErrors[0]
	if prior.Round != 1 || prior.Error != snapshot.LastNonterminalError || prior.ActionIDs[0] != "filter_active" || prior.ActionKinds[0] != string(dataquery.DataActionFilterRecords) {
		t.Fatalf("PriorErrors[0]=%+v", prior)
	}
	if strings.Contains(snapshot.Decision.Reason, "missing field status") {
		t.Fatalf("Decision.Reason=%q, complete terminal should not inherit stale prior error", snapshot.Decision.Reason)
	}
}

func TestWorkflowJournalCompleteSnapshotOverridesStaleBlockedDecision(t *testing.T) {
	rt := NewWorkflowRuntime(dataquery.TaskPlan{})
	rt.AppendRecord(WorkflowRecord{
		Plan: dataquery.TaskPlan{
			Status: "ready",
			Actions: []dataquery.DataAction{{
				ID:   "assemble_too_early",
				Kind: dataquery.DataActionAssembleAnswer,
			}},
		},
		Err: "data planning incomplete: assemble_answer was outside the earlier allowed stage",
	})
	snapshot := rt.BuildJournalSnapshot(WorkflowJournalBuildInput{
		Status:    "complete",
		LastError: "data planning incomplete: assemble_answer was outside the earlier allowed stage",
		State: WorkflowStateSnapshot{
			StageFacts: StageFacts{
				MaterialCoverageSufficient: true,
				RuleCoverageRequired:       true,
				RuleCoverageRecords:        1,
				DecisionRecordsRequired:    true,
				DecisionRecords:            1,
				ContributionLedgerRequired: true,
				ContributionRecords:        1,
				ReconcileRequired:          true,
				HasReconcile:               true,
				HasAnswer:                  true,
			},
			LedgerGraph: LedgerGraph{
				NextStage: StageComplete,
				Dependencies: []LedgerDependency{{
					Ledger:   string(LedgerFinalProjection),
					Required: true,
					Present:  true,
					Status:   LedgerStatusSatisfied,
					Stage:    StageEmitOutputContractAnswer,
				}},
			},
			OutputGraph: OutputProjectionGraph{
				Status:                    OutputProjectionStatusSatisfied,
				AnswerPresent:             true,
				ProjectionArtifactPresent: true,
				ReferenceComplete:         true,
			},
			Decision: WorkflowDecision{
				Status:      "blocked",
				ReasonCode:  "complete",
				Reason:      "older blocked decision",
				Violations:  []string{"action_dependency_violation"},
				NextActions: []string{string(dataquery.DataActionQualifyRecords)},
			},
		},
	})

	if snapshot.Status != "complete" || snapshot.Decision.Status != "complete" {
		t.Fatalf("snapshot status=%q decision=%+v, want completed terminal state", snapshot.Status, snapshot.Decision)
	}
	if snapshot.LastError != "" {
		t.Fatalf("LastError=%q, want stale error moved out of final-state field", snapshot.LastError)
	}
	if !strings.Contains(snapshot.LastNonterminalError, "assemble_answer was outside") {
		t.Fatalf("LastNonterminalError=%q, want stale error lineage", snapshot.LastNonterminalError)
	}
	if snapshot.Decision.Reason != "" || len(snapshot.Decision.Violations) != 0 || len(snapshot.Decision.NextActions) != 0 {
		t.Fatalf("Decision=%+v, want stale blocked decision details retired from terminal decision", snapshot.Decision)
	}
}

func TestWorkflowJournalNonCompletePreservesFinalError(t *testing.T) {
	rt := NewWorkflowRuntime(dataquery.TaskPlan{})
	rt.AppendRecord(WorkflowRecord{
		Plan: dataquery.TaskPlan{Actions: []dataquery.DataAction{{
			ID:   "reconcile",
			Kind: dataquery.DataActionReconcile,
		}}},
		Err: "final reconcile failed",
	})
	snapshot := rt.BuildJournalSnapshot(WorkflowJournalBuildInput{
		Status:    "failed",
		LastError: "final reconcile failed",
	})

	if snapshot.Status != "failed" {
		t.Fatalf("Status=%q, want failed", snapshot.Status)
	}
	if snapshot.LastError != "final reconcile failed" {
		t.Fatalf("LastError=%q, want final error preserved", snapshot.LastError)
	}
	if snapshot.LastNonterminalError != "" {
		t.Fatalf("LastNonterminalError=%q, want empty for non-complete final error", snapshot.LastNonterminalError)
	}
	if len(snapshot.PriorErrors) != 1 || snapshot.PriorErrors[0].Error != "final reconcile failed" {
		t.Fatalf("PriorErrors=%+v", snapshot.PriorErrors)
	}
	if snapshot.Decision.Reason != "final reconcile failed" {
		t.Fatalf("Decision.Reason=%q, want final error reason", snapshot.Decision.Reason)
	}
}

func TestWorkflowJournalAdmissionGuardRecordsBlockedAction(t *testing.T) {
	action := dataquery.DataAction{
		ID:         "qualify_active",
		Kind:       dataquery.DataActionQualifyRecords,
		InputPaths: []string{"enriched.json"},
	}
	guard := NewGuardResult("field_contract_violation", "error", RepairNeedsTypedAction, "missing field status", WorkflowViolation{
		Code:       "field_contract_violation",
		ActionID:   "qualify_active",
		ActionKind: string(dataquery.DataActionQualifyRecords),
		Reason:     "missing field status",
	})
	rt := NewWorkflowRuntime(dataquery.TaskPlan{})
	rt.AppendRecord(WorkflowRecord{
		Plan: dataquery.TaskPlan{Actions: []dataquery.DataAction{action}},
		Err:  guard.ErrorText(),
		Admission: &ActionDAGAdmissionDecision{
			Plan:          dataquery.TaskPlan{Actions: []dataquery.DataAction{action}},
			Original:      dataquery.TaskPlan{Actions: []dataquery.DataAction{action}},
			Guard:         guard,
			FinalGuard:    guard,
			GuardErr:      guard.ErrorText(),
			FinalGuardErr: guard.ErrorText(),
		},
	})

	snapshot := rt.BuildJournalSnapshot(WorkflowJournalBuildInput{Status: "failed"})
	if len(snapshot.ActionEvents) != 1 || snapshot.ActionEvents[0].Status != ActionStatusBlocked {
		t.Fatalf("ActionEvents=%+v, want admission guard as blocked action", snapshot.ActionEvents)
	}
}

func TestWorkflowRuntimeBuildJournalSnapshotPrefersLiveProcessEvents(t *testing.T) {
	rt := NewWorkflowRuntime(dataquery.TaskPlan{})
	rt.AppendRecord(WorkflowRecord{
		Plan: dataquery.TaskPlan{
			WhyThisBatch: "first batch",
			Actions: []dataquery.DataAction{{
				ID:   "extract",
				Kind: dataquery.DataActionExtractRecords,
			}},
		},
	})
	rt.AppendProcessEvent(WorkflowJournalEvent{Kind: "evaluate", Round: 1, Reason: "continue"})
	previewRecords := rt.RecordsWith(WorkflowRecord{
		Plan: dataquery.TaskPlan{
			Actions: []dataquery.DataAction{{
				ID:   "preview",
				Kind: dataquery.DataActionFilterRecords,
			}},
		},
		Err: "blocked preview",
	})

	snapshot := rt.BuildJournalSnapshot(WorkflowJournalBuildInput{
		Status:  "checkpoint",
		Records: previewRecords,
	})
	if len(snapshot.ProcessEvents) != 3 {
		t.Fatalf("ProcessEvents=%+v, want live events plus preview record event", snapshot.ProcessEvents)
	}
	if snapshot.ProcessEvents[0].Kind != "action_batch" || snapshot.ProcessEvents[1].Kind != "evaluate" {
		t.Fatalf("ProcessEvents=%+v, want runtime live event order preserved", snapshot.ProcessEvents)
	}
	if got := snapshot.ProcessEvents[2]; got.Round != 2 || got.Status != "failed" || got.Kind != "action_batch" {
		t.Fatalf("preview event=%+v, want failed action batch round 2", got)
	}
}

func TestWorkflowJournalEventsPromoteRecordExecutionViolations(t *testing.T) {
	rt := NewWorkflowRuntime(dataquery.TaskPlan{})
	rt.AppendRecord(WorkflowRecord{
		Plan: dataquery.TaskPlan{Actions: []dataquery.DataAction{{
			ID:             "filter_paid",
			Kind:           dataquery.DataActionFilterRecords,
			InputPaths:     []string{"records.json"},
			OutputArtifact: "paid.json",
		}}},
		Err: "filter failed",
		Violations: []dataquery.DataTaskViolation{{
			Code:          "field_contract_violation",
			Summary:       "status is missing",
			ActionID:      "filter_paid",
			ActionKind:    string(dataquery.DataActionFilterRecords),
			InputAlias:    "records.json",
			MissingFields: []string{"status"},
			RepairHint:    string(dataquery.DataActionDeriveFields),
		}},
	})

	events := rt.ProcessEvents()
	if len(events) != 1 || events[0].Guard == nil || events[0].ViolationSummary.Total != 1 {
		t.Fatalf("events=%+v, want record execution guard with violation summary", events)
	}
	if events[0].Guard.Code != "field_contract_violation" || events[0].ViolationSummary.FirstActionID != "filter_paid" {
		t.Fatalf("event guard/summary=%+v / %+v", events[0].Guard, events[0].ViolationSummary)
	}
	snapshot := rt.BuildJournalSnapshot(WorkflowJournalBuildInput{Status: "failed"})
	if len(snapshot.WorkflowViolations) != 1 || snapshot.WorkflowViolations[0].Code != "field_contract_violation" {
		t.Fatalf("WorkflowViolations=%+v, want process-event violation promoted", snapshot.WorkflowViolations)
	}
	if snapshot.WorkflowViolationSummary.Total != 1 || snapshot.WorkflowViolationSummary.FirstInputAlias != "records.json" {
		t.Fatalf("WorkflowViolationSummary=%+v, want journal terminal summary", snapshot.WorkflowViolationSummary)
	}
}

func TestWorkflowRuntimeBuildJournalSnapshotPromotesProcessEventGuards(t *testing.T) {
	rt := NewWorkflowRuntime(dataquery.TaskPlan{})
	guard := NewGuardResult("missing_required_ledger", "error", RepairNeedsTypedAction, "contributions missing", WorkflowViolation{
		Code:       "missing_required_ledger",
		ActionKind: string(dataquery.DataActionComputeContribs),
		Reason:     "contributions missing",
	})
	rt.AppendGuardEvent("completion_gate", 3, dataquery.TaskPlan{Actions: []dataquery.DataAction{{
		ID:   "compute",
		Kind: dataquery.DataActionComputeContribs,
	}}}, guard)

	snapshot := rt.BuildJournalSnapshot(WorkflowJournalBuildInput{
		Status: "failed",
		Guards: []GuardResult{NewGuardResult("missing_required_ledger", "error", RepairNeedsTypedAction, "contributions missing", WorkflowViolation{
			Code:       "missing_required_ledger",
			ActionKind: string(dataquery.DataActionComputeContribs),
			Reason:     "contributions missing",
		})},
		GuardRound: 3,
	})
	if len(snapshot.WorkflowViolations) != 1 || snapshot.WorkflowViolations[0].Code != "missing_required_ledger" {
		t.Fatalf("WorkflowViolations=%+v, want deduped process-event guard violation", snapshot.WorkflowViolations)
	}
	if len(snapshot.ProcessEvents) != 2 || snapshot.ProcessEvents[0].Kind != "completion_gate" || snapshot.ProcessEvents[0].Guard == nil {
		t.Fatalf("ProcessEvents=%+v, want live completion guard plus input guard event", snapshot.ProcessEvents)
	}
}

func TestWorkflowRuntimeBuildJournalSnapshotPrefersStateSnapshot(t *testing.T) {
	rt := NewWorkflowRuntime(dataquery.TaskPlan{})
	snapshot := rt.BuildJournalSnapshot(WorkflowJournalBuildInput{
		Status: "checkpoint",
		State: WorkflowStateSnapshot{
			ActionGraph:   ActionGraph{Ready: []ActionNode{{ID: "snapshot-ready"}}},
			LedgerGraph:   LedgerGraph{NextStage: StageComputeContributions},
			OutputGraph:   OutputProjectionGraph{Status: "needs_projection"},
			ArtifactGraph: ArtifactGraphState{NodeCount: 7},
			Progress:      ProgressWindow{LatestSignature: "snapshot-progress"},
			WorkflowViolations: []WorkflowViolation{{
				Code: "snapshot_violation",
			}},
			Decision: WorkflowDecision{ReasonCode: "snapshot_decision"},
		},
		ActionGraph:   ActionGraph{Ready: []ActionNode{{ID: "legacy-ready"}}},
		LedgerGraph:   LedgerGraph{NextStage: StageEmitOutputContractAnswer},
		OutputGraph:   OutputProjectionGraph{Status: "legacy"},
		ArtifactGraph: ArtifactGraphState{NodeCount: 1},
		Progress:      ProgressWindow{LatestSignature: "legacy-progress"},
		WorkflowViolations: []WorkflowViolation{{
			Code: "legacy_violation",
		}},
		Decision: WorkflowDecision{ReasonCode: "legacy_decision"},
	})
	if snapshot.ActionGraph.Ready[0].ID != "snapshot-ready" {
		t.Fatalf("ActionGraph=%+v, want state snapshot", snapshot.ActionGraph)
	}
	if snapshot.LedgerGraph.NextStage != StageComputeContributions || snapshot.OutputGraph.Status != "needs_projection" || snapshot.ArtifactGraph.NodeCount != 7 {
		t.Fatalf("state graphs not preferred: %+v / %+v / %+v", snapshot.LedgerGraph, snapshot.OutputGraph, snapshot.ArtifactGraph)
	}
	if snapshot.Progress.LatestSignature != "snapshot-progress" {
		t.Fatalf("Progress=%+v, want snapshot progress", snapshot.Progress)
	}
	if len(snapshot.WorkflowViolations) != 1 || snapshot.WorkflowViolations[0].Code != "snapshot_violation" {
		t.Fatalf("WorkflowViolations=%+v, want snapshot violations", snapshot.WorkflowViolations)
	}
	if snapshot.Decision.ReasonCode != "snapshot_decision" {
		t.Fatalf("Decision=%+v, want snapshot decision", snapshot.Decision)
	}
}

func TestWorkflowJournalDoesNotOverrideIncompleteIRWithCompleteStatus(t *testing.T) {
	rt := NewWorkflowRuntime(dataquery.TaskPlan{})
	state := WorkflowStateSnapshot{
		Decision: WorkflowDecision{
			Status:     "continue",
			ReasonCode: "output_missing_answer",
			Reason:     "workflow has not produced a final answer yet",
		},
		OutputGraph: OutputProjectionGraph{Status: OutputProjectionStatusMissingAnswer},
	}
	snapshot := rt.BuildJournalSnapshot(WorkflowJournalBuildInput{
		Status: "complete",
		State:  state,
	})
	if snapshot.Status == "complete" || snapshot.Decision.Status == "complete" {
		t.Fatalf("snapshot=%+v, want complete downgraded by IR decision", snapshot)
	}
	if snapshot.Status != "continue" || snapshot.Reason != "workflow has not produced a final answer yet" {
		t.Fatalf("snapshot status/reason=%q/%q, want IR status and reason", snapshot.Status, snapshot.Reason)
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

func TestBuildWorkflowDecisionDowngradesExplicitCompleteWithGraphBlocker(t *testing.T) {
	graph := BuildLedgerGraph(StageFacts{
		MaterialCoverageSufficient: true,
		ContributionLedgerRequired: true,
	})
	decision := BuildWorkflowDecision(WorkflowDecisionInput{
		Status:      "complete",
		NextStage:   StageComplete,
		LedgerGraph: graph,
	})
	if decision.Status == "complete" || decision.ReasonCode != "ledger_missing_contributions" {
		t.Fatalf("decision=%+v, want non-complete ledger blocker", decision)
	}

	outputDecision := BuildWorkflowDecision(WorkflowDecisionInput{
		Status: "complete",
		OutputGraph: OutputProjectionGraph{
			Status:          OutputProjectionStatusMissingAnswer,
			ProducesActions: []string{string(dataquery.DataActionAssembleAnswer)},
		},
	})
	if outputDecision.Status == "complete" || outputDecision.ReasonCode != "output_missing_answer" {
		t.Fatalf("outputDecision=%+v, want non-complete output blocker", outputDecision)
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
