package agent

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// TestDecomposeQuestion_MechanismMultiEntity pins the 3-step
// decomposition for "explorer是怎么调用subagent的" style questions.
func TestDecomposeQuestion_MechanismMultiEntity(t *testing.T) {
	rm := types.RequestModel{
		RawRequest: "explorer是怎么调用subagent的",
		Complexity: types.ComplexityComplex,
		AnalyzerHints: types.AnalyzerHints{
			Kind:     "mechanism",
			Entities: []string{"explorer", "subagent"},
		},
	}
	plan := DecomposeQuestion(rm)
	if plan == nil {
		t.Fatal("expected non-nil plan")
	}
	if len(plan.Steps) != 3 {
		t.Fatalf("expected 3 steps, got %d", len(plan.Steps))
	}
	if plan.Steps[0].Strategy != types.StrategyLocate {
		t.Errorf("step 0 strategy = %q, want locate", plan.Steps[0].Strategy)
	}
	if plan.Steps[1].Strategy != types.StrategyTraceChain {
		t.Errorf("step 1 strategy = %q, want trace_chain", plan.Steps[1].Strategy)
	}
	if plan.Steps[2].Strategy != types.StrategyAggregate {
		t.Errorf("step 2 strategy = %q, want aggregate", plan.Steps[2].Strategy)
	}
	if len(plan.Steps[1].DependsOn) == 0 || plan.Steps[1].DependsOn[0] != "step-0" {
		t.Errorf("step 1 should depend on step-0, got %v", plan.Steps[1].DependsOn)
	}
}

// TestDecomposeQuestion_EnumerationWithCondition pins the 3-step
// decomposition for "有几个agent可以调用subagent" style questions.
func TestDecomposeQuestion_EnumerationWithCondition(t *testing.T) {
	rm := types.RequestModel{
		RawRequest: "有几个agent可以调用subagent",
		Complexity: types.ComplexityModerate,
		AnalyzerHints: types.AnalyzerHints{
			Kind:     "enumeration",
			Entities: []string{"agent", "subagent"},
		},
	}
	plan := DecomposeQuestion(rm)
	if plan == nil || len(plan.Steps) != 3 {
		t.Fatalf("expected 3 steps, got %v", plan)
	}
	if plan.Steps[0].Strategy != types.StrategyEnumerate {
		t.Errorf("step 0 = %q, want enumerate", plan.Steps[0].Strategy)
	}
	if plan.Steps[1].Strategy != types.StrategyVerifyCondition {
		t.Errorf("step 1 = %q, want verify_condition", plan.Steps[1].Strategy)
	}
	if plan.Steps[2].Strategy != types.StrategyAggregate {
		t.Errorf("step 2 = %q, want aggregate", plan.Steps[2].Strategy)
	}
}

// TestDecomposeQuestion_CallChainMultiEntity — call_chain triggers
// the same 3-step plan as mechanism when 2+ entities present.
func TestDecomposeQuestion_CallChainMultiEntity(t *testing.T) {
	rm := types.RequestModel{
		RawRequest: "how does A call B",
		Complexity: types.ComplexityComplex,
		AnalyzerHints: types.AnalyzerHints{
			Kind:     "call_chain",
			Entities: []string{"A", "B"},
		},
	}
	plan := DecomposeQuestion(rm)
	if plan == nil || len(plan.Steps) != 3 {
		t.Fatalf("expected 3 steps for call_chain + 2 entities, got %d", len(plan.Steps))
	}
}

// TestDecomposeQuestion_SimpleQuestionDegrades1Step ensures simple
// questions don't get decomposed — they fall through to a 1-step
// plan with StrategyGeneric, preserving existing ERM behavior.
func TestDecomposeQuestion_SimpleQuestionDegrades1Step(t *testing.T) {
	rm := types.RequestModel{
		RawRequest: "repomap是什么",
		Complexity: types.ComplexitySimple,
		AnalyzerHints: types.AnalyzerHints{
			Kind:     "mechanism",
			Entities: []string{"repomap"},
		},
	}
	plan := DecomposeQuestion(rm)
	if plan == nil || len(plan.Steps) != 1 {
		t.Fatalf("simple question should get 1 step, got %d", len(plan.Steps))
	}
	// Single entity + mechanism → StrategyTraceChain as default
	if plan.Steps[0].Strategy != types.StrategyTraceChain {
		t.Errorf("step strategy = %q, want trace_chain (default for mechanism)", plan.Steps[0].Strategy)
	}
}

// TestDecomposeQuestion_ComplexMechanism1Entity — complex mechanism
// with 1 entity gets a 2-step plan (locate → trace).
func TestDecomposeQuestion_ComplexMechanism1Entity(t *testing.T) {
	rm := types.RequestModel{
		RawRequest: "explain how the scheduler works in detail",
		Complexity: types.ComplexityComplex,
		AnalyzerHints: types.AnalyzerHints{
			Kind:     "mechanism",
			Entities: []string{"scheduler"},
		},
	}
	plan := DecomposeQuestion(rm)
	if plan == nil || len(plan.Steps) != 2 {
		t.Fatalf("expected 2 steps for complex 1-entity mechanism, got %d", len(plan.Steps))
	}
	if plan.Steps[0].Strategy != types.StrategyLocate {
		t.Errorf("step 0 = %q, want locate", plan.Steps[0].Strategy)
	}
	if plan.Steps[1].Strategy != types.StrategyTraceChain {
		t.Errorf("step 1 = %q, want trace_chain", plan.Steps[1].Strategy)
	}
}

// TestInvestigationPlan_Lifecycle tests the plan's state machine:
// pending → active → satisfied, with dependency gating.
func TestInvestigationPlan_Lifecycle(t *testing.T) {
	plan := &types.InvestigationPlan{
		Steps: []types.InvestigationStep{
			{ID: "s0", Strategy: types.StrategyLocate, Status: types.StepPending},
			{ID: "s1", Strategy: types.StrategyTraceChain, DependsOn: []string{"s0"}, Status: types.StepPending},
			{ID: "s2", Strategy: types.StrategyAggregate, DependsOn: []string{"s1"}, Status: types.StepPending},
		},
	}

	// Initially: only s0 is ready.
	ready := plan.ReadySteps()
	if len(ready) != 1 || ready[0].ID != "s0" {
		t.Fatalf("expected only s0 ready, got %v", ready)
	}

	// CurrentStep = s0.
	if c := plan.CurrentStep(); c == nil || c.ID != "s0" {
		t.Errorf("CurrentStep = %v, want s0", c)
	}

	// AllSatisfied = false.
	if plan.AllSatisfied() {
		t.Error("AllSatisfied should be false initially")
	}

	// Satisfy s0 → s1 becomes ready.
	plan.MarkSatisfied("s0", "done")
	ready = plan.ReadySteps()
	if len(ready) != 1 || ready[0].ID != "s1" {
		t.Fatalf("after s0 satisfied, expected s1 ready, got %v", ready)
	}

	// Satisfy s1 → s2 becomes ready.
	plan.MarkSatisfied("s1", "done")
	if c := plan.CurrentStep(); c == nil || c.ID != "s2" {
		t.Errorf("after s1 satisfied, CurrentStep = %v, want s2", c)
	}

	// Satisfy s2 → AllSatisfied.
	plan.MarkSatisfied("s2", "done")
	if !plan.AllSatisfied() {
		t.Error("AllSatisfied should be true after all steps satisfied")
	}
	if c := plan.CurrentStep(); c != nil {
		t.Errorf("CurrentStep should be nil when all satisfied, got %v", c)
	}
}

// TestCheckStepSatisfaction_TraceChain tests the structural
// completion criterion for trace_chain steps.
func TestCheckStepSatisfaction_TraceChain(t *testing.T) {
	step := &types.InvestigationStep{
		Strategy: types.StrategyTraceChain,
		Entities: []string{"explorer", "subagent"},
	}

	t.Run("unsatisfied when target missing", func(t *testing.T) {
		// Neither Subject nor Summary mentions "subagent" — target missing.
		evidence := []types.EvidenceItem{
			{Kind: types.EvidenceMechanism, Subject: "Orchestrator", Summary: "dispatch logic"},
			{Kind: types.EvidenceMechanism, Subject: "Registry", Summary: "explorer registration"},
			{Kind: types.EvidenceMechanism, Subject: "Topology", Summary: "explorer stage mapping"},
		}
		if CheckStepSatisfaction(step, evidence, nil) {
			t.Error("should be unsatisfied when target entity 'subagent' not in evidence")
		}
	})

	t.Run("satisfied when both entities present with 3+ items", func(t *testing.T) {
		evidence := []types.EvidenceItem{
			{Kind: types.EvidenceMechanism, Subject: "ExplorerAgent", Summary: "explorer creates subagent call"},
			{Kind: types.EvidenceMechanism, Subject: "SubAgentRuntime", Summary: "subagent dispatch"},
			{Kind: types.EvidenceMechanism, Subject: "SubExplorer", Summary: "subagent execution"},
		}
		if !CheckStepSatisfaction(step, evidence, nil) {
			t.Error("should be satisfied when both entities present with 3+ items")
		}
	})

	t.Run("unsatisfied with only 2 items", func(t *testing.T) {
		evidence := []types.EvidenceItem{
			{Kind: types.EvidenceMechanism, Subject: "ExplorerAgent", Summary: "explorer info"},
			{Kind: types.EvidenceMechanism, Subject: "SubExplorer", Summary: "subagent info"},
		}
		if CheckStepSatisfaction(step, evidence, nil) {
			t.Error("should be unsatisfied with only 2 relevant items (need ≥3)")
		}
	})
}

// TestCheckStepSatisfaction_Aggregate always satisfied.
func TestCheckStepSatisfaction_Aggregate(t *testing.T) {
	step := &types.InvestigationStep{Strategy: types.StrategyAggregate}
	if !CheckStepSatisfaction(step, nil, nil) {
		t.Error("aggregate step should always be satisfied")
	}
}

// TestMaybeRefinePlan_TraceChainStall tests that a stalled
// trace_chain step gets refined into locate + trace sub-steps.
func TestMaybeRefinePlan_TraceChainStall(t *testing.T) {
	plan := &types.InvestigationPlan{
		Steps: []types.InvestigationStep{
			{ID: "step-0", Strategy: types.StrategyLocate, Status: types.StepSatisfied},
			{ID: "step-1", Strategy: types.StrategyTraceChain, Status: types.StepActive,
				Entities: []string{"A", "B"}, RetryCount: replanRetryThreshold},
			{ID: "step-2", Strategy: types.StrategyAggregate, DependsOn: []string{"step-1"}, Status: types.StepPending},
		},
	}

	refined := MaybeRefinePlan(plan)
	if !refined {
		t.Fatal("expected plan to be refined")
	}
	if len(plan.Steps) != 5 { // original 3 + 2 new sub-steps
		t.Fatalf("expected 5 steps after refinement, got %d", len(plan.Steps))
	}
	// step-1 should be satisfied (marked by refiner)
	if plan.Steps[1].Status != types.StepSatisfied {
		t.Errorf("stalled step should be marked satisfied, got %q", plan.Steps[1].Status)
	}
	// New sub-steps should be step-1-a and step-1-b
	if plan.Steps[2].ID != "step-1-a" {
		t.Errorf("first refined step ID = %q, want step-1-a", plan.Steps[2].ID)
	}
}

// TestMaybeRefinePlan_NoStall — below threshold, no refinement.
func TestMaybeRefinePlan_NoStall(t *testing.T) {
	plan := &types.InvestigationPlan{
		Steps: []types.InvestigationStep{
			{ID: "step-0", Strategy: types.StrategyTraceChain, Status: types.StepActive,
				Entities: []string{"A", "B"}, RetryCount: 1},
		},
	}
	if MaybeRefinePlan(plan) {
		t.Error("should not refine when retry count is below threshold")
	}
}

// TestCheckAnswerStepCoverage verifies answer-step alignment.
func TestCheckAnswerStepCoverage(t *testing.T) {
	plan := &types.InvestigationPlan{
		Steps: []types.InvestigationStep{
			{ID: "s0", Strategy: types.StrategyLocate, Entities: []string{"explorer"}},
			{ID: "s1", Strategy: types.StrategyTraceChain, Entities: []string{"explorer", "subagent"}},
			{ID: "s2", Strategy: types.StrategyAggregate},
		},
	}

	t.Run("no gaps when answer mentions all entities", func(t *testing.T) {
		answer := "The explorer dispatches subagent via the orchestrator."
		gaps := CheckAnswerStepCoverage(plan, answer)
		if len(gaps) != 0 {
			t.Errorf("expected no gaps, got %v", gaps)
		}
	})

	t.Run("gap when answer misses an entity", func(t *testing.T) {
		answer := "The system dispatches tasks via the orchestrator."
		gaps := CheckAnswerStepCoverage(plan, answer)
		if len(gaps) == 0 {
			t.Error("expected gaps when answer doesn't mention 'explorer' or 'subagent'")
		}
	})

	t.Run("single-step plan never reports gaps", func(t *testing.T) {
		single := &types.InvestigationPlan{
			Steps: []types.InvestigationStep{
				{ID: "s0", Strategy: types.StrategyGeneric, Entities: []string{"missing"}},
			},
		}
		gaps := CheckAnswerStepCoverage(single, "unrelated answer")
		if len(gaps) != 0 {
			t.Error("single-step plan should never report gaps")
		}
	})
}
