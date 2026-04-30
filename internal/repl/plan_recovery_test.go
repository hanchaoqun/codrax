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
	if _, err := store.SaveForTest(&types.ChangePlan{
		ID:      "plan-older",
		Status:  types.PlanStatusApplied,
		Changes: []types.FileChange{{Path: "x.go", Kind: "modify"}},
	}); err != nil {
		t.Fatalf("save older: %v", err)
	}
	// Newer pending plan — the recovery target.
	pendingPath, err := store.SaveForTest(&types.ChangePlan{
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
	if _, err := store.SaveForTest(&types.ChangePlan{
		ID:      "plan-applied",
		Status:  types.PlanStatusApplied,
		Changes: []types.FileChange{{Path: "x.go", Kind: "modify"}},
	}); err != nil {
		t.Fatalf("save: %v", err)
	}
	if _, err := store.SaveForTest(&types.ChangePlan{
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
	_, err := store.SaveForTest(&types.ChangePlan{
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
	if !strings.Contains(got, "recovered pending plan") {
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

// TestPlanStoreSave_RejectsWhenUnsettledExists locks the data-layer
// hard constraint of the single-pending-plan invariant: Save must
// refuse a new plan when ANY other plan in the store is in an
// unsettled state. The error must be ErrUnsettledPlanExists carrying
// the offending plan info so the caller can render a user-visible
// menu.
func TestPlanStoreSave_RejectsWhenUnsettledExists(t *testing.T) {
	store := NewPlanStore(t.TempDir())
	if _, err := store.SaveForTest(&types.ChangePlan{
		ID:      "plan-blocker",
		Status:  types.PlanStatusApplied,
		Changes: []types.FileChange{{Path: "x.go", Kind: "modify"}},
	}); err != nil {
		t.Fatalf("seed blocker: %v", err)
	}
	_, err := store.Save(&types.ChangePlan{
		ID:      "plan-newcomer",
		Status:  types.PlanStatusPending,
		Changes: []types.FileChange{{Path: "y.go", Kind: "modify"}},
	})
	if err == nil {
		t.Fatal("Save should refuse when an unsettled plan already exists")
	}
	var typed *ErrUnsettledPlanExists
	if !errorsAs(err, &typed) {
		t.Fatalf("error must be ErrUnsettledPlanExists; got %T: %v", err, err)
	}
	if typed.Existing.ID != "plan-blocker" {
		t.Errorf("ErrUnsettledPlanExists.Existing.ID = %q, want plan-blocker", typed.Existing.ID)
	}
}

// TestPlanStoreSave_PassesWhenOnlyTerminalPlansExist verifies that
// merged / rejected / applied_failed plans do NOT block new Save —
// they are terminal, so the lifecycle is closed and a new plan is
// allowed.
func TestPlanStoreSave_PassesWhenOnlyTerminalPlansExist(t *testing.T) {
	store := NewPlanStore(t.TempDir())
	for _, terminal := range []string{
		types.PlanStatusMerged,
		types.PlanStatusRejected,
		types.PlanStatusApplyFailed,
	} {
		if _, err := store.SaveForTest(&types.ChangePlan{
			ID:      "plan-" + terminal,
			Status:  terminal,
			Changes: []types.FileChange{{Path: "x.go", Kind: "modify"}},
		}); err != nil {
			t.Fatalf("seed %s: %v", terminal, err)
		}
	}
	if _, err := store.Save(&types.ChangePlan{
		ID:      "plan-fresh",
		Status:  types.PlanStatusPending,
		Changes: []types.FileChange{{Path: "y.go", Kind: "modify"}},
	}); err != nil {
		t.Errorf("Save should pass when only terminal plans exist; got: %v", err)
	}
}

// TestPlanStoreSettle_FlipsToMerged locks the merge transition:
// Status → merged, MergedAt stamped, WorktreePath cleared.
func TestPlanStoreSettle_FlipsToMerged(t *testing.T) {
	store := NewPlanStore(t.TempDir())
	if _, err := store.SaveForTest(&types.ChangePlan{
		ID:           "plan-x",
		Status:       types.PlanStatusApplied,
		WorktreePath: "/tmp/wt",
		Changes:      []types.FileChange{{Path: "a.go", Kind: "modify"}},
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := store.Settle("plan-x", types.PlanStatusMerged, ""); err != nil {
		t.Fatalf("Settle: %v", err)
	}
	got, err := store.Load("plan-x")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Status != types.PlanStatusMerged {
		t.Errorf("Status = %q, want merged", got.Status)
	}
	if got.MergedAt == nil {
		t.Error("MergedAt must be stamped on merge settle")
	}
	if got.WorktreePath != "" {
		t.Errorf("WorktreePath should be cleared on merge; got %q", got.WorktreePath)
	}
}

// TestPlanStoreSettle_FlipsToRejected locks reject transition with reason.
func TestPlanStoreSettle_FlipsToRejected(t *testing.T) {
	store := NewPlanStore(t.TempDir())
	if _, err := store.SaveForTest(&types.ChangePlan{
		ID:      "plan-r",
		Status:  types.PlanStatusPending,
		Changes: []types.FileChange{{Path: "a.go", Kind: "modify"}},
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := store.Settle("plan-r", types.PlanStatusRejected, "user changed mind"); err != nil {
		t.Fatalf("Settle: %v", err)
	}
	got, err := store.Load("plan-r")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Status != types.PlanStatusRejected {
		t.Errorf("Status = %q, want rejected", got.Status)
	}
	if got.RejectedAt == nil {
		t.Error("RejectedAt must be stamped on reject settle")
	}
	if got.RejectionReason != "user changed mind" {
		t.Errorf("RejectionReason = %q, want %q", got.RejectionReason, "user changed mind")
	}
}

// TestModePlan_RefusesWhenUnsettledExists locks layer-2 of the
// invariant: /mode plan does NOT switch when an unsettled plan
// blocks; the menu is printed instead. Status drives which commands
// appear in the menu.
func TestModePlan_RefusesWhenUnsettledExists(t *testing.T) {
	store := NewPlanStore(t.TempDir())
	if _, err := store.SaveForTest(&types.ChangePlan{
		ID:      "plan-blocker",
		Status:  types.PlanStatusApplied,
		Changes: []types.FileChange{{Path: "a.go", Kind: "modify"}},
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	r, out := newScriptedREPL(t, store)
	r.handleModeCmd("/mode plan")

	if r.currentMode == types.ModePlan {
		t.Error("/mode plan must NOT switch when unsettled plan blocks")
	}
	got := out.String()
	for _, want := range []string{"plan-blocker", "/merge", "/reject", "/plan clear"} {
		if !strings.Contains(got, want) {
			t.Errorf("menu missing %q; got: %s", want, got)
		}
	}
}

// errorsAs is a tiny shim for the typed-error assertion (cannot
// import "errors" in a test that already has a different "errors"
// import name). Kept local to plan_recovery_test.go.
func errorsAs(err error, target **ErrUnsettledPlanExists) bool {
	for err != nil {
		if e, ok := err.(*ErrUnsettledPlanExists); ok {
			*target = e
			return true
		}
		// Walk Unwrap chain in case the caller wraps further.
		type unwrapper interface{ Unwrap() error }
		if u, ok := err.(unwrapper); ok {
			err = u.Unwrap()
			continue
		}
		break
	}
	return false
}
