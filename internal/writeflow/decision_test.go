package writeflow

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/toolparam"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestNormalizeAndValidateWriteWorkflowDecisionExploreCode(t *testing.T) {
	decision := WriteWorkflowDecision{
		Action:     ActionExploreCode,
		ReasonCode: "  need_source_context  ",
		ExplorationRequest: &types.WriteExplorationRequest{
			BatchID:              " batch-1 ",
			Goal:                 " inspect planner ",
			CandidatePaths:       []string{" internal/agent/planner.go ", "internal/agent/planner.go"},
			ExplorationQuestions: []string{" where are retry caps set? "},
			MaxRounds:            -1,
		},
	}
	got := NormalizeWriteWorkflowDecision(decision)
	if got.ReasonCode != "need_source_context" {
		t.Fatalf("reason code not trimmed: %+v", got)
	}
	if got.ExplorationRequest == nil || got.ExplorationRequest.Goal != "inspect planner" {
		t.Fatalf("exploration request not normalized: %+v", got.ExplorationRequest)
	}
	if got.ExplorationRequest.MaxRounds != 0 {
		t.Fatalf("negative max_rounds not normalized: %+v", got.ExplorationRequest)
	}
	if len(got.ExplorationRequest.CandidatePaths) != 1 {
		t.Fatalf("candidate paths not deduped: %+v", got.ExplorationRequest.CandidatePaths)
	}
	if errs := ValidateWriteWorkflowDecision(got); len(errs) != 0 {
		t.Fatalf("valid decision rejected: %v", errs)
	}
}

func TestValidateWriteWorkflowDecisionRequiresTypedPayloads(t *testing.T) {
	tests := []struct {
		name     string
		decision WriteWorkflowDecision
		want     string
	}{
		{
			name:     "explore requires request",
			decision: WriteWorkflowDecision{Action: ActionExploreCode},
			want:     "exploration_request",
		},
		{
			name:     "plan requires batch",
			decision: WriteWorkflowDecision{Action: ActionPlanBatch},
			want:     "batch",
		},
		{
			name:     "ask user requires questions",
			decision: WriteWorkflowDecision{Action: ActionAskUser},
			want:     "questions_for_user",
		},
		{
			name: "ask user requires typed missing facts",
			decision: WriteWorkflowDecision{
				Action:           ActionAskUser,
				QuestionsForUser: []string{"Which runtime owns this deploy?"},
			},
			want: "missing_facts",
		},
		{
			name:     "block requires reason",
			decision: WriteWorkflowDecision{Action: ActionBlock},
			want:     "reason",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := ValidateWriteWorkflowDecision(tt.decision)
			if !strings.Contains(strings.Join(errs, "\n"), tt.want) {
				t.Fatalf("expected error mentioning %q, got %v", tt.want, errs)
			}
		})
	}
}

func TestValidateWriteWorkflowDecisionNamesMissingBatchGoal(t *testing.T) {
	decision := NormalizeWriteWorkflowDecision(WriteWorkflowDecision{
		Action: ActionReplanBatch,
		Batch: &WriteBatchPlan{
			ID:     "batch-1",
			Status: BatchReadyForChangePlan,
		},
	})
	if decision.Batch == nil || decision.Batch.ID != "batch-1" {
		t.Fatalf("normalization must preserve non-empty typed batch payload: %+v", decision)
	}
	errs := ValidateWriteWorkflowDecision(decision)
	if !strings.Contains(strings.Join(errs, "\n"), "replan_batch requires batch.goal") {
		t.Fatalf("missing goal should be precise, got %v", errs)
	}
}

func TestHydrateWriteWorkflowDecisionFromRunFillsActiveBatchGoal(t *testing.T) {
	decision := WriteWorkflowDecision{
		Action: ActionReplanBatch,
		Batch: &WriteBatchPlan{
			ID:     "batch-1",
			Status: BatchReadyForChangePlan,
		},
	}
	run := types.WriteWorkflowRun{
		RunID:         "wf-1",
		Goal:          "overall change",
		ActiveBatchID: "batch-1",
		Batches: []types.WriteWorkflowBatch{{
			ID:     "batch-1",
			Goal:   "repair the failing behavior",
			Status: types.WriteWorkflowBatchReadyToPlan,
		}},
	}
	got := HydrateWriteWorkflowDecisionFromRun(decision, run)
	if got.Batch == nil || got.Batch.Goal != "repair the failing behavior" {
		t.Fatalf("hydrated decision = %+v", got)
	}
	if errs := ValidateWriteWorkflowDecision(got); len(errs) != 0 {
		t.Fatalf("hydrated decision should validate: %v", errs)
	}
}

func TestValidateWriteWorkflowDecisionRejectsDeprecatedActionAliases(t *testing.T) {
	for _, alias := range []WorkflowAction{"plan_change_batch", "apply_ready_plan", "verify"} {
		decision := NormalizeWriteWorkflowDecision(WriteWorkflowDecision{Action: alias})
		if decision.Action != alias {
			t.Fatalf("deprecated alias %q must not normalize to canonical action, got %q", alias, decision.Action)
		}
		errs := ValidateWriteWorkflowDecision(decision)
		joined := strings.Join(errs, "\n")
		if !strings.Contains(joined, "not one of") {
			t.Fatalf("deprecated alias %q should be rejected with enum error, got %v", alias, errs)
		}
	}
}

func TestValidateWriteWorkflowDecisionAllowsTypedAskUser(t *testing.T) {
	errs := ValidateWriteWorkflowDecision(WriteWorkflowDecision{
		Action:           ActionAskUser,
		ReasonCode:       "missing_runtime_owner",
		QuestionsForUser: []string{"Which runtime owns this deploy?"},
		MissingFacts: []MissingUserFact{{
			Kind:        "runtime_constraint",
			Description: "deployment runtime owner is not present in repo evidence",
			EvidenceRef: "context-pack:p2-runtime",
			Consumer:    "controller",
		}},
	})
	if len(errs) != 0 {
		t.Fatalf("typed ask_user should validate, got %v", errs)
	}
}

func TestMissingUserFactKeysIgnoreQuestionProse(t *testing.T) {
	first := MissingUserFactKeys([]MissingUserFact{{
		Kind:        "runtime_constraint",
		Description: "first wording",
		EvidenceRef: "report-1",
		Consumer:    "planner",
	}})
	second := MissingUserFactKeys([]MissingUserFact{{
		Kind:        "runtime_constraint",
		Description: "completely different wording",
		EvidenceRef: "report-1",
		Consumer:    "planner",
	}})
	if len(first) != 1 || len(second) != 1 || first[0] != second[0] {
		t.Fatalf("fact keys should use typed lanes/evidence refs, got %v and %v", first, second)
	}
}

func TestWriteWorkflowDecisionSchemaToolParamCompat(t *testing.T) {
	raw := json.RawMessage(`{
		"action": "explore_code",
		"reason_code": "need_source_context",
		"workflow": {
			"goal": "make write mode staged",
			"success_criteria": "tests pass; read mode unchanged",
			"status": "in_progress",
			"max_batches": "3"
		},
		"exploration_request": {
			"batch_id": "batch-1",
			"goal": "inspect existing planner",
			"exploration_questions": "where does planner get retry context, what tests cover it",
			"candidate_paths": "internal/agent/planner.go, internal/orchestrator/phase_scheduler.go",
			"constraints": "do not change read mode",
			"max_rounds": "4",
			"evidence_requirements": "file:line refs, test names"
		},
		"safety_notes": "read-only exploration"
	}`)

	repaired, report := toolparam.Normalize(raw, WriteWorkflowDecisionSchema(), types.DefaultToolParamCompatConfig())
	if !report.Changed() {
		t.Fatalf("expected schema-driven repairs")
	}
	var decoded WriteWorkflowDecision
	if err := json.Unmarshal(repaired, &decoded); err != nil {
		t.Fatalf("repaired payload must unmarshal: %v\n%s", err, repaired)
	}
	decoded = NormalizeWriteWorkflowDecision(decoded)
	if decoded.Workflow.MaxBatches != 3 {
		t.Fatalf("max_batches = %d; want 3", decoded.Workflow.MaxBatches)
	}
	if len(decoded.Workflow.SuccessCriteria) != 2 {
		t.Fatalf("success criteria not split: %+v", decoded.Workflow.SuccessCriteria)
	}
	if decoded.ExplorationRequest == nil || decoded.ExplorationRequest.MaxRounds != 4 {
		t.Fatalf("exploration request not repaired: %+v", decoded.ExplorationRequest)
	}
	if got := decoded.ExplorationRequest.CandidatePaths; len(got) != 2 {
		t.Fatalf("candidate paths not split: %+v", got)
	}
	if got := decoded.SafetyNotes; len(got) != 1 || got[0] != "read-only exploration" {
		t.Fatalf("safety notes not repaired: %+v", got)
	}
	if errs := ValidateWriteWorkflowDecision(decoded); len(errs) != 0 {
		t.Fatalf("repaired decision should validate, got %v", errs)
	}
}
