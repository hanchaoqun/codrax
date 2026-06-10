package agent

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestWriteControllerPromptConsumesTypedArtifactsAndAvoidsProseRouting(t *testing.T) {
	mut := types.NewMutableState("implement workflow")
	mut.SetWriteAnalysisIR(&types.WriteAnalysisIR{
		Request: types.WriteRequestModel{
			Task: types.WriteTask{
				Kind:    types.WriteTaskFeature,
				Scope:   types.ScopeCross,
				Summary: "ship workflow controller",
			},
			Risk: types.WriteRiskProfile{Overall: types.RiskBandMedium},
		},
	})
	mut.SetWriteContextPack(&types.WriteContextPack{
		PackID:  "pack-1",
		BatchID: "batch-1",
		Items: []types.WriteContextItem{{
			Priority:  types.WriteContextP0,
			Kind:      "constraint",
			Text:      "preserve legacy write path",
			Consumers: []types.WriteContextConsumer{types.WriteConsumerController},
		}},
	})
	mut.SetWriteWorkflowRun(&types.WriteWorkflowRun{
		RunID:         "wf-1",
		Status:        types.WriteWorkflowRunInProgress,
		ActiveBatchID: "batch-1",
		Batches: []types.WriteWorkflowBatch{{
			ID:     "batch-1",
			Goal:   "first batch",
			Status: types.WriteWorkflowBatchNeedsExploration,
		}},
		Budget: types.WriteWorkflowBudget{MaxBatches: 5, MaxExplorationRounds: 2},
	})
	eval := &writeControllerEvaluator{}
	got := eval.BuildInitialInstruction(&types.AgentContext{Mutable: mut}, nil)
	for _, want := range []string{
		"## Typed write task",
		"ship workflow controller",
		"## Workflow run state",
		"wf-1",
		"## Priority write context pack",
		"emit_write_workflow_decision",
		"typed action enum",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("controller prompt missing %q:\n%s", want, got)
		}
	}
	for _, banned := range []string{
		"keyword",
		"if the request says",
		"if the user says",
		"summary contains",
		"rationale contains",
		"parse prose",
	} {
		if strings.Contains(strings.ToLower(got), banned) {
			t.Fatalf("controller prompt contains prose-routing smell %q:\n%s", banned, got)
		}
	}
}

func TestWriteControllerParseOutputReadsStoredDecisionJSON(t *testing.T) {
	mut := types.NewMutableState("implement workflow")
	mut.SetWriteWorkflowDecisionJSON([]byte(`{"action":"finish","reason_code":"done"}`))
	eval := &writeControllerEvaluator{}
	out, err := eval.ParseOutput(&types.AgentContext{Mutable: mut}, nil, nil, nil)
	if err != nil {
		t.Fatalf("ParseOutput error: %v", err)
	}
	if out.Error != "" {
		t.Fatalf("ParseOutput returned error: %s", out.Error)
	}
	if !strings.Contains(out.StageReport, "Action: finish") {
		t.Fatalf("StageReport missing action: %s", out.StageReport)
	}
	if string(out.Data) != `{"action":"finish","reason_code":"done"}` {
		t.Fatalf("Data should preserve stored normalized JSON, got %s", out.Data)
	}
}
