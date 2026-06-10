package writeflow

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/toolparam"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestNormalizeAndValidateWorkflowPlan(t *testing.T) {
	plan := WriteWorkflowPlan{
		Goal:                "  add safer write workflow  ",
		SuccessCriteria:     []string{" tests pass ", "tests pass", "plan is batch-scoped"},
		Status:              "unexpected",
		MissingObservations: []string{"  "},
		NextBatch: &WriteBatchPlan{
			Goal:                 " inspect current planner ",
			Status:               "unknown",
			NeedsCodeExploration: true,
			ExploreTargets:       []string{" internal/agent/planner.go ", "internal/agent/planner.go"},
		},
	}

	got := NormalizeWorkflowPlan(plan)
	if got.Goal != "add safer write workflow" {
		t.Fatalf("goal = %q", got.Goal)
	}
	if got.Status != WorkflowPlanned {
		t.Fatalf("status = %q; want %q", got.Status, WorkflowPlanned)
	}
	if got.NextBatch == nil || got.NextBatch.Status != BatchPending {
		t.Fatalf("next batch not normalized: %+v", got.NextBatch)
	}
	if len(got.SuccessCriteria) != 2 {
		t.Fatalf("success criteria not deduped: %+v", got.SuccessCriteria)
	}
	if errs := ValidateWorkflowPlan(got); len(errs) != 0 {
		t.Fatalf("normalized plan should validate, got %v", errs)
	}
}

func TestValidateWorkflowPlanNeedsConcreteCriteria(t *testing.T) {
	errs := ValidateWorkflowPlan(WriteWorkflowPlan{Goal: "do it", Status: WorkflowInProgress})
	joined := strings.Join(errs, "\n")
	if !strings.Contains(joined, "success_criteria") {
		t.Fatalf("expected success criteria error, got %v", errs)
	}
	if !strings.Contains(joined, "next_batch") {
		t.Fatalf("expected next_batch error, got %v", errs)
	}
}

func TestWriteWorkflowPlanSchemaToolParamCompat(t *testing.T) {
	raw := json.RawMessage(`{
		"goal": "make write mode safer",
		"known_constraints": "preserve read mode, keep write_enabled gate",
		"success_criteria": "risk shown; tests pass",
		"status": "in_progress",
		"max_batches": "4",
		"next_batch": {
			"id": "batch-1",
			"goal": "inspect planner",
			"needs_code_exploration": "true",
			"explore_targets": "internal/agent/planner.go, internal/orchestrator/write_scheduler.go",
			"depends_on": "bootstrap"
		}
	}`)

	repaired, report := toolparam.Normalize(raw, WriteWorkflowPlanSchema(), types.DefaultToolParamCompatConfig())
	if !report.Changed() {
		t.Fatalf("expected schema-driven repairs")
	}
	var decoded WriteWorkflowPlan
	if err := json.Unmarshal(repaired, &decoded); err != nil {
		t.Fatalf("repaired payload must unmarshal: %v\n%s", err, repaired)
	}
	decoded = NormalizeWorkflowPlan(decoded)
	if decoded.MaxBatches != 4 {
		t.Fatalf("max_batches = %d; want 4", decoded.MaxBatches)
	}
	if len(decoded.SuccessCriteria) != 2 {
		t.Fatalf("success_criteria = %+v; want split array", decoded.SuccessCriteria)
	}
	if decoded.NextBatch == nil || !decoded.NextBatch.NeedsCodeExploration {
		t.Fatalf("next batch boolean was not repaired: %+v", decoded.NextBatch)
	}
	if got := decoded.NextBatch.ExploreTargets; len(got) != 2 {
		t.Fatalf("explore_targets = %+v; want two split targets", got)
	}
	if got := decoded.NextBatch.DependsOn; len(got) != 1 || got[0] != "bootstrap" {
		t.Fatalf("depends_on = %+v; want singleton array", got)
	}
}

func TestWorkflowSeedFromWriteAnalysisUsesPhaseProposal(t *testing.T) {
	ir := &types.WriteAnalysisIR{}
	ir.Request.Task.Summary = "add staged write workflow"
	ir.Request.ExpectedOutcomes = []string{"risk is visible", "tests pass"}
	ir.Request.Constraints = []types.WriteConstraint{
		{Kind: "preserve_read_mode", Target: "*", Note: "do not alter read scheduler"},
	}
	ir.PhaseProposal.Split = "sequential"
	ir.PhaseProposal.Phases = []types.PhaseSeed{
		{Goal: "add workflow schema", RoughTargetPaths: []string{"internal/writeflow"}},
		{Goal: "wire scheduler", RoughTargetPaths: []string{"internal/orchestrator"}},
	}

	got := WorkflowSeedFromWriteAnalysis(ir)
	if got.Goal != "add staged write workflow" {
		t.Fatalf("goal = %q", got.Goal)
	}
	if got.NextBatch == nil || got.NextBatch.ID != "batch-1" {
		t.Fatalf("next batch = %+v", got.NextBatch)
	}
	if len(got.PlannedBatches) != 2 {
		t.Fatalf("planned batches = %+v; want 2", got.PlannedBatches)
	}
	if got.NextBatch.ExploreTargets[0] != "internal/writeflow" {
		t.Fatalf("next batch targets = %+v", got.NextBatch.ExploreTargets)
	}
	if len(got.KnownConstraints) != 1 || !strings.Contains(got.KnownConstraints[0], "preserve_read_mode") {
		t.Fatalf("constraints = %+v", got.KnownConstraints)
	}
}

func TestWorkflowSeedFromWriteAnalysis_MicroScopeWithAnchorsSeedsReadyForPlan(t *testing.T) {
	ir := &types.WriteAnalysisIR{}
	ir.Request.Task = types.WriteTask{Kind: types.WriteTaskBugfix, Scope: types.ScopeMicro, Summary: "fix typo"}
	ir.Request.ScopeAnchors = []string{"src/util.c"}
	seed := WorkflowSeedFromWriteAnalysis(ir)
	if seed.NextBatch == nil {
		t.Fatal("seed must produce a next batch")
	}
	if seed.NextBatch.Status != BatchReadyForChangePlan {
		t.Fatalf("micro scope with anchors should seed ready-for-plan, got %s", seed.NextBatch.Status)
	}
	if seed.NextBatch.NeedsCodeExploration {
		t.Fatal("micro scope with anchors should not demand exploration")
	}
}

func TestWorkflowSeedFromWriteAnalysis_MicroWithoutAnchorsStillExplores(t *testing.T) {
	ir := &types.WriteAnalysisIR{}
	ir.Request.Task = types.WriteTask{Kind: types.WriteTaskBugfix, Scope: types.ScopeMicro, Summary: "fix typo"}
	seed := WorkflowSeedFromWriteAnalysis(ir)
	if seed.NextBatch == nil || seed.NextBatch.Status != BatchNeedsExploration {
		t.Fatalf("micro scope without anchors must keep exploration, got %+v", seed.NextBatch)
	}
	ir2 := &types.WriteAnalysisIR{}
	ir2.Request.Task = types.WriteTask{Kind: types.WriteTaskFeature, Scope: types.ScopePackage, Summary: "wider change"}
	ir2.Request.ScopeAnchors = []string{"pkg/"}
	seed2 := WorkflowSeedFromWriteAnalysis(ir2)
	if seed2.NextBatch == nil || seed2.NextBatch.Status != BatchNeedsExploration {
		t.Fatalf("package scope must keep exploration, got %+v", seed2.NextBatch)
	}
}
