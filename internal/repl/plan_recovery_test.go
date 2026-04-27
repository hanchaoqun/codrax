package repl

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// TestRecoverPendingPlanFromStore_FindsLatestPending verifies the
// helper picks the most recent Status=pending_approval plan when
// PlanStore has multiple entries with mixed statuses.
func TestRecoverPendingPlanFromStore_FindsLatestPending(t *testing.T) {
	store := NewPlanStore(t.TempDir())
	// Older applied plan — should be skipped.
	if _, err := store.Save(&types.ChangePlan{
		ID:      "plan-older",
		Status:  types.PlanStatusApplied,
		Changes: []types.FileChange{{Path: "x.go", Kind: "modify"}},
	}); err != nil {
		t.Fatalf("save older: %v", err)
	}
	// Newer pending plan — the recovery target.
	pendingPath, err := store.Save(&types.ChangePlan{
		ID:      "plan-pending",
		Status:  types.PlanStatusPending,
		Changes: []types.FileChange{{Path: "y.go", Kind: "modify"}},
	})
	if err != nil {
		t.Fatalf("save pending: %v", err)
	}

	r, _ := newScriptedREPL(t, store)
	got, ok := r.recoverPendingPlanFromStore()
	if !ok {
		t.Fatal("recoverPendingPlanFromStore returned ok=false; want hit")
	}
	if got.ID != "plan-pending" {
		t.Errorf("recovered ID = %q, want plan-pending", got.ID)
	}
	if got.Path != pendingPath {
		t.Errorf("recovered Path = %q, want %q", got.Path, pendingPath)
	}
}

// TestRecoverPendingPlanFromStore_NoPendingReturnsFalse verifies the
// helper returns false when only applied / failed / rejected plans
// exist. The /plan show caller falls through to the existing
// "no pending plan" message.
func TestRecoverPendingPlanFromStore_NoPendingReturnsFalse(t *testing.T) {
	store := NewPlanStore(t.TempDir())
	if _, err := store.Save(&types.ChangePlan{
		ID:      "plan-applied",
		Status:  types.PlanStatusApplied,
		Changes: []types.FileChange{{Path: "x.go", Kind: "modify"}},
	}); err != nil {
		t.Fatalf("save: %v", err)
	}
	if _, err := store.Save(&types.ChangePlan{
		ID:      "plan-rejected",
		Status:  types.PlanStatusRejected,
		Changes: []types.FileChange{{Path: "y.go", Kind: "modify"}},
	}); err != nil {
		t.Fatalf("save: %v", err)
	}

	r, _ := newScriptedREPL(t, store)
	if _, ok := r.recoverPendingPlanFromStore(); ok {
		t.Error("expected ok=false when no pending plan exists")
	}
}

// TestPlanShow_RecoversFromStoreWhenPointerEmpty is the integration
// test for the bug fix: /plan show with empty pendingPlanPath now
// finds and rebinds the most recent pending plan from PlanStore
// instead of printing "no pending plan". This is the user-visible
// recovery after /approve fails pre-flight.
func TestPlanShow_RecoversFromStoreWhenPointerEmpty(t *testing.T) {
	store := NewPlanStore(t.TempDir())
	_, err := store.Save(&types.ChangePlan{
		ID:      "plan-recovery-target",
		Summary: "recovery target",
		Status:  types.PlanStatusPending,
		Changes: []types.FileChange{
			{Path: "main.go", Kind: "modify", Rationale: "test"},
		},
		TargetPaths: []string{"main.go"},
	})
	if err != nil {
		t.Fatalf("PlanStore.Save: %v", err)
	}

	r, out := newScriptedREPL(t, store)
	// Simulate the bug-trigger state: pointer empty even though a
	// pending plan is on disk.
	r.pendingPlanPath = ""
	r.handlePlanCmd("/plan show")

	got := out.String()
	if !strings.Contains(got, "recovered pending plan from PlanStore") {
		t.Errorf("expected recovery banner; got: %q", got)
	}
	if !strings.Contains(got, "plan-recovery-target") {
		t.Errorf("expected recovered plan id in output; got: %q", got)
	}
	if r.pendingPlanPath == "" {
		t.Error("pendingPlanPath should be rebound after recovery")
	}
}

// TestPlanShow_NoRecoveryWhenNothingPending verifies the original
// "no pending plan" message still prints when there's truly nothing
// to recover (empty PlanStore).
func TestPlanShow_NoRecoveryWhenNothingPending(t *testing.T) {
	store := NewPlanStore(t.TempDir())
	r, out := newScriptedREPL(t, store)
	r.pendingPlanPath = ""
	r.handlePlanCmd("/plan show")

	got := out.String()
	if !strings.Contains(got, "no pending plan") {
		t.Errorf("expected 'no pending plan' message; got: %q", got)
	}
	if strings.Contains(got, "recovered pending plan") {
		t.Errorf("recovery banner should not fire on empty store; got: %q", got)
	}
}
