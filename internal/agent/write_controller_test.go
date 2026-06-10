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

func TestWriteControllerPromptFiltersPlannerProbeReports(t *testing.T) {
	mut := types.NewMutableState("stale probe")
	mut.SetWriteWorkflowRun(&types.WriteWorkflowRun{
		RunID:         "wf-probe",
		Status:        types.WriteWorkflowRunInProgress,
		ActiveBatchID: "batch-1",
		Batches: []types.WriteWorkflowBatch{{
			ID:     "batch-1",
			Status: types.WriteWorkflowBatchVerifying,
			PlanID: "plan-current",
		}},
	})
	mut.SetChangePlan(&types.ChangePlan{ID: "plan-current", Status: types.PlanStatusPending})
	mut.SetChangeReport(&types.ChangeReport{
		PlanID:         "plan-current",
		Channel:        types.ChangeReportChannelPlannerProbe,
		Passed:         false,
		FailureSummary: "stale dry-run failure",
	})
	eval := &writeControllerEvaluator{}
	got := eval.BuildInitialInstruction(&types.AgentContext{Mutable: mut}, nil)
	if strings.Contains(got, "stale dry-run failure") || strings.Contains(got, "change_report:") {
		t.Fatalf("planner probe report must not render as authoritative controller artifact:\n%s", got)
	}

	mut.SetChangeReport(&types.ChangeReport{
		PlanID:         "plan-current",
		Channel:        types.ChangeReportChannelPostApplyVerify,
		Passed:         true,
		FailureSummary: "",
	})
	got = eval.BuildInitialInstruction(&types.AgentContext{Mutable: mut}, nil)
	if !strings.Contains(got, "change_report: plan_id=plan-current channel=post_apply_verify passed=true") {
		t.Fatalf("post-apply report should render as authoritative controller artifact:\n%s", got)
	}
}

func TestRenderWriteControllerRunSection_CanonicalAttemptState(t *testing.T) {
	mu := types.NewMutableState("canonical state")
	run := types.WriteWorkflowRun{
		RunID:         "wf-canon",
		Status:        types.WriteWorkflowRunInProgress,
		ActiveBatchID: "batch-1",
		Batches: []types.WriteWorkflowBatch{{
			ID:     "batch-1",
			Goal:   "repair verify",
			Status: types.WriteWorkflowBatchReadyToPlan,
			PlanID: "plan-1",
			Attempts: []types.WriteWorkflowAttempt{
				{Kind: "verify", Status: "failed", ReasonCode: "tests_failed", ReportID: "plan-1.report.json"},
			},
		}},
		ProgressLedger: []types.WriteWorkflowProgress{{
			BatchID:    "batch-1",
			Status:     "progress",
			ReasonCode: "verify_failed",
			Message:    "verify failed: red tests",
		}},
	}
	mu.SetWriteWorkflowRun(&run)
	ctx := &types.AgentContext{Mutable: mu}
	section := renderWriteControllerRunSection(ctx)
	if !strings.Contains(section, "state=needs_replan") {
		t.Fatalf("active batch must render the canonical derived phase, got:\n%s", section)
	}
	if !strings.Contains(section, "cause=tests_failed") {
		t.Fatalf("derived phase must carry the typed cause, got:\n%s", section)
	}
	if !strings.Contains(section, "failed_verify_attempts=1") {
		t.Fatalf("failed verify attempt count missing:\n%s", section)
	}
	if strings.Contains(section, "state=ready_to_plan") {
		t.Fatalf("contradictory ready_to_plan reading must not render for a failed-verify batch:\n%s", section)
	}
	if !strings.Contains(section, "last_event: batch=batch-1 event=verify_failed") {
		t.Fatalf("progress should render as a labeled event, got:\n%s", section)
	}
}
