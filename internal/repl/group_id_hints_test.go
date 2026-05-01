package repl

import (
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

// TestHandleMergeCmd_GroupIDHints pins commit 25's cross-
// command nudge: when the user types `/merge group-X`, /merge
// detects the group-id token and surfaces a hint pointing at
// /phase show instead of silently ignoring it. Group-level
// merge is not implemented yet; the hint is the contract.
func TestHandleMergeCmd_GroupIDHints(t *testing.T) {
	store := NewPlanStore(t.TempDir())
	r, out := newScriptedREPL(t, store)
	r.handleMergeCmd("/merge group-1234567")
	got := out.String()
	if !strings.Contains(got, "/phase show") {
		t.Errorf("expected hint pointing at /phase show; got %q", got)
	}
	if !strings.Contains(got, "group-1234567") {
		t.Errorf("expected the supplied group id to be echoed; got %q", got)
	}
	if !strings.Contains(got, "does not yet operate on groups") {
		t.Errorf("expected explicit unsupported message; got %q", got)
	}
}

// TestHandleRejectCmd_GroupIDHints pins commit 25's nudge
// for /reject: when the reason text starts with a group id,
// the user almost certainly wanted to discard the whole
// group, not record the id as the rejection reason for a
// single plan. Surface a hint and refuse.
func TestHandleRejectCmd_GroupIDHints(t *testing.T) {
	store := NewPlanStore(t.TempDir())
	r, out := newScriptedREPL(t, store)
	// Seed a pending plan so /reject's "no pending plan" check
	// passes and we exercise the group-id detection code path.
	plan := &types.ChangePlan{
		ID:        "plan-reject-target",
		Status:    types.PlanStatusPending,
		CreatedAt: time.Now(),
		Changes:   []types.FileChange{{Path: "x.go", Kind: "create"}},
	}
	if _, err := store.Save(plan); err != nil {
		t.Fatalf("Save: %v", err)
	}
	r.pendingPlanPath = store.PlanDir() + "/plan-reject-target.json"

	r.handleRejectCmd("/reject group-9876543")
	got := out.String()
	if !strings.Contains(got, "/phase show") {
		t.Errorf("expected hint pointing at /phase show; got %q", got)
	}
	if !strings.Contains(got, "group-9876543") {
		t.Errorf("expected the supplied group id to be echoed; got %q", got)
	}
	// Confirm the plan was NOT settled — the hint refused before
	// any state mutation.
	loaded, err := store.Load("plan-reject-target")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.Status != types.PlanStatusPending {
		t.Errorf("plan should not have been rejected; got status=%q", loaded.Status)
	}
}

// TestPlanInfoSurfacesPhaseGroup pins commit 25's PlanStore
// list probe: PhaseGroupID + PhaseIndex are extracted from
// the on-disk JSON so /plan list rendering can show the
// "group=X phase=N" suffix without loading the full plan.
func TestPlanInfoSurfacesPhaseGroup(t *testing.T) {
	store := NewPlanStore(t.TempDir())
	plan := &types.ChangePlan{
		ID:           "plan-with-phase",
		Status:       types.PlanStatusApplied,
		PhaseGroupID: "group-X",
		PhaseIndex:   1,
		CreatedAt:    time.Now(),
		Changes:      []types.FileChange{{Path: "a", Kind: "create"}},
	}
	if _, err := store.Save(plan); err != nil {
		t.Fatalf("Save: %v", err)
	}
	infos, err := store.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(infos) != 1 {
		t.Fatalf("expected 1 plan; got %d", len(infos))
	}
	got := infos[0]
	if got.PhaseGroupID != "group-X" {
		t.Errorf("PhaseGroupID drift; got %q", got.PhaseGroupID)
	}
	if got.PhaseIndex != 1 {
		t.Errorf("PhaseIndex drift; got %d", got.PhaseIndex)
	}
}
