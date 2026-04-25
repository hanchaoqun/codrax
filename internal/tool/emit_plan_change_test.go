package tool

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// installSkeletonForTest seeds Mutable.PartialChangePlan via the
// public emit_plan_skeleton path. Centralised so the change tests
// don't repeat the boilerplate.
func installSkeletonForTest(t *testing.T, bus *types.BusContext, params string) {
	t.Helper()
	res, err := (&EmitPlanSkeleton{}).Execute(bus, json.RawMessage(params))
	if err != nil {
		t.Fatalf("seed skeleton err: %v", err)
	}
	if !res.Success {
		t.Fatalf("seed skeleton rejected: %s", res.Summary)
	}
}

// TestEmitPlanChange_PartialFillReportsProgress: filling some but
// not all slots returns success with a "remaining" hint, and does
// NOT promote the plan.
func TestEmitPlanChange_PartialFillReportsProgress(t *testing.T) {
	bus := &types.BusContext{Mutable: types.NewMutableState("")}
	installSkeletonForTest(t, bus, `{
		"request": "x",
		"summary": "Two-file plan to demonstrate partial fill — first emit_plan_change must report progress, not finalize.",
		"changes": [
			{"path": "a.go", "kind": "create", "rationale": "first file"},
			{"path": "b.go", "kind": "create", "rationale": "second file"}
		]
	}`)
	res, err := (&EmitPlanChange{}).Execute(bus, json.RawMessage(`{
		"path": "a.go",
		"new_content": "package a\n"
	}`))
	if err != nil {
		t.Fatalf("execute err: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success on partial fill, got: %s", res.Summary)
	}
	if !strings.Contains(res.Summary, "1/2") {
		t.Errorf("summary should report 1/2 filled, got: %s", res.Summary)
	}
	if !strings.Contains(res.Summary, "b.go") {
		t.Errorf("summary should hint at remaining file b.go, got: %s", res.Summary)
	}
	if bus.Mutable.ChangePlan() != nil {
		t.Errorf("ChangePlan should still be nil before final fill")
	}
	if bus.Mutable.PartialChangePlan() == nil {
		t.Errorf("PartialChangePlan should still be present")
	}
}

// TestEmitPlanChange_FinalFillFinalizesPlan: filling the LAST slot
// promotes Partial → ChangePlan.
func TestEmitPlanChange_FinalFillFinalizesPlan(t *testing.T) {
	bus := &types.BusContext{Mutable: types.NewMutableState("")}
	bus.RepoRoot = t.TempDir() // empty repo — V1/V2/V3 degrade to no-op
	installSkeletonForTest(t, bus, `{
		"request": "x",
		"summary": "Single-file create plan; the one emit_plan_change call should finalize the plan.",
		"changes": [
			{"path": "a.go", "kind": "create", "rationale": "the only file"}
		]
	}`)
	res, err := (&EmitPlanChange{}).Execute(bus, json.RawMessage(`{
		"path": "a.go",
		"new_content": "package a\n"
	}`))
	if err != nil {
		t.Fatalf("execute err: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success on final fill, got: %s", res.Summary)
	}
	if !strings.Contains(res.Summary, "FINALIZED") {
		t.Errorf("summary should say FINALIZED, got: %s", res.Summary)
	}
	if bus.Mutable.ChangePlan() == nil {
		t.Fatalf("ChangePlan should be installed after finalize")
	}
	if bus.Mutable.PartialChangePlan() != nil {
		t.Errorf("PartialChangePlan should be cleared after promote")
	}
}

// TestEmitPlanChange_NoSkeletonRejects: calling emit_plan_change
// without a prior skeleton returns a clear protocol-error message.
func TestEmitPlanChange_NoSkeletonRejects(t *testing.T) {
	bus := &types.BusContext{Mutable: types.NewMutableState("")}
	res, _ := (&EmitPlanChange{}).Execute(bus, json.RawMessage(`{"path": "a.go", "new_content": "x"}`))
	if res.Success {
		t.Fatalf("expected rejection without skeleton")
	}
	if !strings.Contains(res.Summary, "no skeleton") {
		t.Errorf("rejection should explain skeleton precondition, got: %s", res.Summary)
	}
}

// TestEmitPlanChange_UnknownPathRejects: a path not in the skeleton
// must reject — the planner should re-emit the skeleton or correct
// the path, not silently extend the plan.
func TestEmitPlanChange_UnknownPathRejects(t *testing.T) {
	bus := &types.BusContext{Mutable: types.NewMutableState("")}
	installSkeletonForTest(t, bus, `{
		"request": "x",
		"summary": "Skeleton declares a.go only; emit_plan_change for b.go should reject because b.go is not in changes[].",
		"changes": [{"path": "a.go", "kind": "create", "rationale": "x"}]
	}`)
	res, _ := (&EmitPlanChange{}).Execute(bus, json.RawMessage(`{"path": "b.go", "new_content": "x"}`))
	if res.Success {
		t.Fatalf("expected rejection for unknown path")
	}
	if !strings.Contains(res.Summary, "not in the skeleton") {
		t.Errorf("rejection should mention skeleton, got: %s", res.Summary)
	}
}

// TestEmitPlanChange_DeleteKindRejects: kind=delete needs no body —
// the tool refuses the call so the LLM doesn't waste a round.
func TestEmitPlanChange_DeleteKindRejects(t *testing.T) {
	bus := &types.BusContext{Mutable: types.NewMutableState("")}
	installSkeletonForTest(t, bus, `{
		"request": "x",
		"summary": "Skeleton with one create and one delete; emit_plan_change must refuse for the delete entry.",
		"changes": [
			{"path": "a.go",   "kind": "create", "rationale": "x"},
			{"path": "old.go", "kind": "delete", "rationale": "y"}
		]
	}`)
	res, _ := (&EmitPlanChange{}).Execute(bus, json.RawMessage(`{"path": "old.go", "new_content": "x"}`))
	if res.Success {
		t.Fatalf("expected rejection for delete kind")
	}
	if !strings.Contains(res.Summary, "kind=delete") {
		t.Errorf("rejection should explain delete kinds, got: %s", res.Summary)
	}
}

// TestEmitPlanChange_OnlyDeleteSkeletonFinalizesImmediately:
// a skeleton whose only change is a delete needs ZERO emit_plan_change
// calls — but the user must trigger finalize somehow. Document the
// current behaviour: we DO NOT auto-finalize at skeleton time. The
// planner is expected to emit at least one emit_plan_change for any
// non-trivial plan; an all-delete plan still has to call
// emit_change_plan in single-shot mode (no body is needed there
// either, the validators are content-independent for deletes).
func TestEmitPlanChange_OnlyDeleteSkeletonStaysIncomplete(t *testing.T) {
	bus := &types.BusContext{Mutable: types.NewMutableState("")}
	installSkeletonForTest(t, bus, `{
		"request": "x",
		"summary": "All-delete skeleton — no emit_plan_change calls are valid since deletes need no body. The planner should use emit_change_plan single-shot for this kind of plan.",
		"changes": [{"path": "old.go", "kind": "delete", "rationale": "x"}]
	}`)
	// PartialChangePlan is installed but no emit_plan_change can
	// promote it — by design (deletes have no body to fill, so the
	// completeness check trivially passes but the tool was never
	// called and so the promote path was never taken).
	if bus.Mutable.PartialChangePlan() == nil {
		t.Errorf("PartialChangePlan should be installed by skeleton")
	}
	if bus.Mutable.ChangePlan() != nil {
		t.Errorf("ChangePlan should NOT auto-finalize at skeleton time")
	}
}
