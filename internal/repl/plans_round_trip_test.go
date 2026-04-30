package repl

import (
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

// TestPlanStore_RoundTripPreservesNewFields pins commit 15 #7:
// the three commit-7-onwards persisted fields (PlanCritique,
// UnvalidatedReasons, WriteAnalysisIR pin) survive PlanStore.Save
// + PlanStore.Load without drift. Without this guard a future
// refactor that filters / renames / reshapes ChangePlan during
// persistence could silently drop one of these fields and the
// only signal would be a downstream feature mysteriously
// stopping working.
func TestPlanStore_RoundTripPreservesNewFields(t *testing.T) {
	dir := t.TempDir()
	store := NewPlanStore(dir)
	original := &types.ChangePlan{
		ID:          "plan-roundtrip-test",
		Status:      types.PlanStatusPending,
		CreatedAt:   time.Now(),
		Summary:     "round-trip persistence test",
		TargetPaths: []string{"a.go", "b.go"},
		Changes: []types.FileChange{
			{Path: "a.go", Kind: "modify", NewContent: "package main\n"},
			{Path: "b.go", Kind: "create", NewContent: "package b\n"},
		},
		// commit 7 #2 — plan_critic review persisted.
		PlanCritique: "- one risk\n(confidence: high)",
		// commit 7 #9 — unvalidated stages.
		UnvalidatedReasons: []string{"rust: cargo not in PATH", "kotlin: kotlinc not in PATH"},
		// commit 9 #3 — pinned WriteAnalysisIR.
		WriteAnalysisIR: &types.WriteAnalysisIR{
			Request: types.WriteRequestModel{
				RawRequest: "user request",
				Task: types.WriteTask{
					Kind:    types.WriteTaskFeature,
					Scope:   types.ScopeMicro,
					Summary: "test",
				},
				Risk: types.WriteRiskProfile{
					AffectsPublicAPI: true,
					Overall:          types.RiskBandMedium,
				},
				ExpectedOutcomes: []string{"test outcome"},
			},
		},
	}
	if _, err := store.Save(original); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := store.Load("plan-roundtrip-test")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded == nil {
		t.Fatal("Load returned nil")
	}

	// PlanCritique
	if loaded.PlanCritique != original.PlanCritique {
		t.Errorf("PlanCritique drift: got %q want %q", loaded.PlanCritique, original.PlanCritique)
	}

	// UnvalidatedReasons
	if len(loaded.UnvalidatedReasons) != len(original.UnvalidatedReasons) {
		t.Errorf("UnvalidatedReasons length = %d; want %d",
			len(loaded.UnvalidatedReasons), len(original.UnvalidatedReasons))
	} else {
		for i := range original.UnvalidatedReasons {
			if loaded.UnvalidatedReasons[i] != original.UnvalidatedReasons[i] {
				t.Errorf("UnvalidatedReasons[%d] = %q; want %q",
					i, loaded.UnvalidatedReasons[i], original.UnvalidatedReasons[i])
			}
		}
	}

	// WriteAnalysisIR
	if loaded.WriteAnalysisIR == nil {
		t.Fatal("WriteAnalysisIR lost in round-trip")
	}
	if loaded.WriteAnalysisIR.Request.Task.Kind != types.WriteTaskFeature {
		t.Errorf("Task.Kind drift: got %q", loaded.WriteAnalysisIR.Request.Task.Kind)
	}
	if loaded.WriteAnalysisIR.Request.Task.Summary != "test" {
		t.Errorf("Task.Summary drift: got %q", loaded.WriteAnalysisIR.Request.Task.Summary)
	}
	if !loaded.WriteAnalysisIR.Request.Risk.AffectsPublicAPI {
		t.Error("Risk.AffectsPublicAPI drift")
	}
	if loaded.WriteAnalysisIR.Request.Risk.Overall != types.RiskBandMedium {
		t.Errorf("Risk.Overall drift: got %q", loaded.WriteAnalysisIR.Request.Risk.Overall)
	}
	if len(loaded.WriteAnalysisIR.Request.ExpectedOutcomes) != 1 {
		t.Errorf("ExpectedOutcomes length drift: got %d", len(loaded.WriteAnalysisIR.Request.ExpectedOutcomes))
	}
}

// TestPlanStore_RoundTripMinimalPlan pins the back-compat path:
// a plan with NONE of the new fields (legacy plan from before
// commit 7) loads cleanly with empty new-field values.
func TestPlanStore_RoundTripMinimalPlan(t *testing.T) {
	dir := t.TempDir()
	store := NewPlanStore(dir)
	plan := &types.ChangePlan{
		ID:          "plan-minimal",
		Status:      types.PlanStatusPending,
		CreatedAt:   time.Now(),
		Summary:     "minimal plan",
		TargetPaths: []string{"x.go"},
		Changes:     []types.FileChange{{Path: "x.go", Kind: "modify"}},
		// All three new fields explicitly empty.
	}
	if _, err := store.Save(plan); err != nil {
		t.Fatalf("Save: %v", err)
	}
	loaded, err := store.Load("plan-minimal")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.PlanCritique != "" {
		t.Errorf("PlanCritique should be empty; got %q", loaded.PlanCritique)
	}
	if len(loaded.UnvalidatedReasons) != 0 {
		t.Errorf("UnvalidatedReasons should be nil/empty; got %v", loaded.UnvalidatedReasons)
	}
	if loaded.WriteAnalysisIR != nil {
		t.Errorf("WriteAnalysisIR should be nil; got %+v", loaded.WriteAnalysisIR)
	}
}
