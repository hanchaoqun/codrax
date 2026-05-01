package repl

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

// TestPlanStore_LoadRejectsInvalidID pins P1 #8 — Load refuses
// any id that doesn't match [a-zA-Z0-9_-]+ so a crafted
// path-traversal id can't escape planDir.
func TestPlanStore_LoadRejectsInvalidID(t *testing.T) {
	store := NewPlanStore(t.TempDir())
	cases := []string{
		"../etc/passwd",
		"foo/bar",
		"plan-1; rm -rf /",
		"plan with spaces",
		"..",
		"./.",
	}
	for _, id := range cases {
		_, err := store.Load(id)
		if err == nil {
			t.Errorf("PlanStore.Load(%q) should error; got nil", id)
			continue
		}
		if !strings.Contains(err.Error(), "invalid plan id") {
			t.Errorf("PlanStore.Load(%q) error = %v; want 'invalid plan id'", id, err)
		}
	}
}

// TestPlanGroupStore_LoadRejectsInvalidID pins the P1 #8
// sibling on the group store.
func TestPlanGroupStore_LoadRejectsInvalidID(t *testing.T) {
	store := NewPlanGroupStore(t.TempDir())
	cases := []string{
		"../etc/passwd",
		"foo/bar",
		"group-1; rm -rf /",
		"..",
	}
	for _, id := range cases {
		_, err := store.Load(id)
		if err == nil {
			t.Errorf("PlanGroupStore.Load(%q) should error; got nil", id)
			continue
		}
		if !strings.Contains(err.Error(), "invalid group id") {
			t.Errorf("PlanGroupStore.Load(%q) error = %v; want 'invalid group id'", id, err)
		}
	}
}

// TestPlanStore_LoadAcceptsCanonicalID pins the happy path so
// the validator doesn't accidentally reject production IDs.
func TestPlanStore_LoadAcceptsCanonicalID(t *testing.T) {
	store := NewPlanStore(t.TempDir())
	plan := &types.ChangePlan{
		ID:        "plan-1730834521-12345",
		Status:    types.PlanStatusPending,
		CreatedAt: time.Now(),
		Summary:   "ok",
		Changes:   []types.FileChange{{Path: "a", Kind: "create"}},
	}
	if _, err := store.Save(plan); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := store.Load("plan-1730834521-12345")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.ID != plan.ID {
		t.Errorf("Load id = %q; want %q", got.ID, plan.ID)
	}
}

// TestPlanStore_SameGroupSiblingsCoexist pins P0 #2 — multi-
// phase ChangePlans sharing a PhaseGroupID can both be saved
// in unsettled state. Without the carve-out the second Save
// returns ErrUnsettledPlanExists and phase plans never reach
// disk.
func TestPlanStore_SameGroupSiblingsCoexist(t *testing.T) {
	store := NewPlanStore(t.TempDir())
	now := time.Now()
	a := &types.ChangePlan{
		ID:           "plan-phase-a",
		Status:       types.PlanStatusApplied, // unsettled
		PhaseGroupID: "group-multi-phase",
		PhaseIndex:   0,
		CreatedAt:    now,
		Changes:      []types.FileChange{{Path: "a", Kind: "create"}},
	}
	b := &types.ChangePlan{
		ID:           "plan-phase-b",
		Status:       types.PlanStatusPending, // unsettled
		PhaseGroupID: "group-multi-phase",
		PhaseIndex:   1,
		CreatedAt:    now,
		Changes:      []types.FileChange{{Path: "b", Kind: "create"}},
	}
	if _, err := store.Save(a); err != nil {
		t.Fatalf("Save a: %v", err)
	}
	if _, err := store.Save(b); err != nil {
		t.Fatalf("Save b should succeed for same-group sibling; got %v", err)
	}
}

// TestPlanStore_DifferentGroupSiblingsBlocked pins the
// invariant still applies across groups: two unsettled plans
// in DIFFERENT groups must still trip ErrUnsettledPlanExists.
func TestPlanStore_DifferentGroupSiblingsBlocked(t *testing.T) {
	store := NewPlanStore(t.TempDir())
	now := time.Now()
	a := &types.ChangePlan{
		ID:           "plan-phase-a",
		Status:       types.PlanStatusApplied,
		PhaseGroupID: "group-one",
		CreatedAt:    now,
		Changes:      []types.FileChange{{Path: "a", Kind: "create"}},
	}
	b := &types.ChangePlan{
		ID:           "plan-phase-b",
		Status:       types.PlanStatusPending,
		PhaseGroupID: "group-two",
		CreatedAt:    now,
		Changes:      []types.FileChange{{Path: "b", Kind: "create"}},
	}
	if _, err := store.Save(a); err != nil {
		t.Fatalf("Save a: %v", err)
	}
	_, err := store.Save(b)
	if err == nil {
		t.Fatalf("Save b across different group should fail; got nil")
	}
	var unsettled *ErrUnsettledPlanExists
	if !errors.As(err, &unsettled) {
		t.Errorf("expected ErrUnsettledPlanExists; got %v", err)
	}
}

// TestPlanStore_SinglePhaseStillBlocked pins the original
// invariant on plans that have no PhaseGroupID — they must
// still see every other unsettled plan as a blocker.
func TestPlanStore_SinglePhaseStillBlocked(t *testing.T) {
	store := NewPlanStore(t.TempDir())
	now := time.Now()
	a := &types.ChangePlan{
		ID:        "plan-singleton-a",
		Status:    types.PlanStatusPending,
		CreatedAt: now,
		Changes:   []types.FileChange{{Path: "a", Kind: "create"}},
	}
	b := &types.ChangePlan{
		ID:        "plan-singleton-b",
		Status:    types.PlanStatusPending,
		CreatedAt: now,
		Changes:   []types.FileChange{{Path: "b", Kind: "create"}},
	}
	if _, err := store.Save(a); err != nil {
		t.Fatalf("Save a: %v", err)
	}
	_, err := store.Save(b)
	if err == nil {
		t.Fatalf("Save b without group should fail with sibling unsettled; got nil")
	}
}

