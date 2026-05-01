package repl

import (
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

// TestHandleMergeCmd_RoutesGroupIDToGroupHandler pins commit
// 31: the dispatcher in handleMergeCmd detects a "group-"
// prefix in any positional token and routes the call to
// handleMergeGroupCmd instead of falling through to per-plan
// merge. Pre-commit-31 this surface was an info-only hint;
// commit 31 promoted it to a real group operation.
//
// We assert by behavioural side effect: the merge engine
// (runMerge) is NOT invoked for a missing group — the group
// handler surfaces its own diagnostic prefix.
func TestHandleMergeCmd_RoutesGroupIDToGroupHandler(t *testing.T) {
	dir := t.TempDir()
	store := NewPlanStore(dir)
	groupStore := NewPlanGroupStore(dir)
	r, out := newScriptedREPL(t, store)
	r.planGroupStore = groupStore
	r.handleMergeCmd("/merge group-1234567")
	got := out.String()
	if !strings.Contains(got, "/merge group:") {
		t.Errorf("expected group-handler diagnostic prefix; got %q", got)
	}
	if !strings.Contains(got, "group-1234567") {
		t.Errorf("expected the supplied group id to be echoed; got %q", got)
	}
}

// TestHandleRejectCmd_RoutesGroupIDToGroupHandler pins commit
// 31: the dispatcher in handleRejectCmd detects a "group-"
// prefix on the first reason token and routes the call to
// handleRejectGroupCmd instead of treating the token as a
// rejection-reason string. Without the routing, the user's
// intent (discard the whole group) gets silently mistreated
// as "record group-<id> as the rejection reason for one
// pending plan."
//
// Pre-commit-31 this surface was an info-only hint; commit 31
// promoted it to a real group operation. We assert by side
// effect: the per-plan store should be UNTOUCHED (because no
// phase plans exist for this fake group id), and the group
// handler should surface its own diagnostic.
func TestHandleRejectCmd_RoutesGroupIDToGroupHandler(t *testing.T) {
	dir := t.TempDir()
	store := NewPlanStore(dir)
	groupStore := NewPlanGroupStore(dir)
	r, out := newScriptedREPL(t, store)
	r.planGroupStore = groupStore
	// Seed a pending plan so the per-plan reject path's "no
	// pending plan" check would have passed; the routing test
	// confirms it WASN'T taken.
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

	// The pending plan should be UNTOUCHED — the dispatcher
	// routed to the group handler, not to per-plan reject.
	loaded, err := store.Load("plan-reject-target")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.Status != types.PlanStatusPending {
		t.Errorf("pending plan must NOT be rejected by group routing; got %q", loaded.Status)
	}
	// Group handler surfaces "cannot load" because the group
	// doesn't exist in the store.
	got := out.String()
	if !strings.Contains(got, "/reject group:") {
		t.Errorf("expected group-handler diagnostic prefix; got %q", got)
	}
}

// TestHandleRejectGroupCmd_SettlesAllPhasesAsRejected pins
// commit 31's group-level /reject: every phase plan in the
// group transitions to PlanStatusRejected with the supplied
// reason; the group transitions to PlanGroupRolledBack;
// settle failures don't abort.
func TestHandleRejectGroupCmd_SettlesAllPhasesAsRejected(t *testing.T) {
	dir := t.TempDir()
	planStore := NewPlanStore(dir)
	groupStore := NewPlanGroupStore(dir)
	r, _ := newScriptedREPL(t, planStore)
	r.planGroupStore = groupStore
	r.writeEnabled = true

	// Seed a 2-phase group with both phase plans + a worktree
	// path so the test exercises the worktree-discard branch.
	now := time.Now()
	plan1 := &types.ChangePlan{
		ID:           "plan-rej-phase1",
		Status:       types.PlanStatusApplied,
		PhaseGroupID: "group-rej-target",
		PhaseIndex:   0,
		WorktreePath: t.TempDir(), // exists; DiscardByPath is a no-op for non-git dirs
		CreatedAt:    now,
		Changes:      []types.FileChange{{Path: "a.go", Kind: "create"}},
	}
	plan2 := &types.ChangePlan{
		ID:           "plan-rej-phase2",
		Status:       types.PlanStatusApplied,
		PhaseGroupID: "group-rej-target",
		PhaseIndex:   1,
		CreatedAt:    now,
		Changes:      []types.FileChange{{Path: "b.go", Kind: "create"}},
	}
	if _, err := planStore.Save(plan1); err != nil {
		t.Fatalf("Save plan1: %v", err)
	}
	if _, err := planStore.Save(plan2); err != nil {
		t.Fatalf("Save plan2: %v", err)
	}
	g := &types.PlanGroup{
		ID:        "group-rej-target",
		Goal:      "demo",
		CreatedAt: now,
		Status:    types.PlanGroupInFlight,
		Decision:  "linear",
		ActiveIdx: 1,
		Phases: []types.PhaseRecord{
			{Index: 0, Goal: "p0", Status: types.PhaseAccepted, PlanID: "plan-rej-phase1"},
			{Index: 1, Goal: "p1", Status: types.PhaseInProgress, PlanID: "plan-rej-phase2"},
		},
	}
	if _, err := groupStore.Save(g); err != nil {
		t.Fatalf("Save group: %v", err)
	}

	r.handleRejectGroupCmd("group-rej-target", "wrong direction overall")

	// Both phase plans now rejected.
	for _, id := range []string{"plan-rej-phase1", "plan-rej-phase2"} {
		loaded, err := planStore.Load(id)
		if err != nil {
			t.Fatalf("Load %s: %v", id, err)
		}
		if loaded.Status != types.PlanStatusRejected {
			t.Errorf("plan %s should be rejected; got %q", id, loaded.Status)
		}
		if loaded.RejectionReason != "wrong direction overall" {
			t.Errorf("plan %s reason = %q; want 'wrong direction overall'", id, loaded.RejectionReason)
		}
	}

	// Group transitioned to RolledBack.
	loadedG, err := groupStore.Load("group-rej-target")
	if err != nil {
		t.Fatalf("Load group: %v", err)
	}
	if loadedG.Status != types.PlanGroupRolledBack {
		t.Errorf("group should be RolledBack; got %q", loadedG.Status)
	}
}

// TestHandleRejectGroupCmd_RefusesCompletedGroup pins the
// guard: a Completed group should be /merge'd, not rejected.
func TestHandleRejectGroupCmd_RefusesCompletedGroup(t *testing.T) {
	dir := t.TempDir()
	planStore := NewPlanStore(dir)
	groupStore := NewPlanGroupStore(dir)
	r, out := newScriptedREPL(t, planStore)
	r.planGroupStore = groupStore
	r.writeEnabled = true

	g := &types.PlanGroup{
		ID:        "group-completed",
		Status:    types.PlanGroupCompleted,
		Decision:  "linear",
		ActiveIdx: 2,
		Phases: []types.PhaseRecord{
			{Index: 0, Goal: "p0", Status: types.PhaseAccepted},
			{Index: 1, Goal: "p1", Status: types.PhaseAccepted},
		},
	}
	if _, err := groupStore.Save(g); err != nil {
		t.Fatalf("Save group: %v", err)
	}

	r.handleRejectGroupCmd("group-completed", "")
	got := out.String()
	if !strings.Contains(got, "already Completed") {
		t.Errorf("expected refusal hint about Completed; got %q", got)
	}
}

// TestHandleMergeGroupCmd_RefusesNonCompletedGroup pins the
// happy-path guard: in_flight or planning groups can't be
// merged without --include-failed.
func TestHandleMergeGroupCmd_RefusesNonCompletedGroup(t *testing.T) {
	dir := t.TempDir()
	planStore := NewPlanStore(dir)
	groupStore := NewPlanGroupStore(dir)
	r, out := newScriptedREPL(t, planStore)
	r.planGroupStore = groupStore
	r.writeEnabled = true

	g := &types.PlanGroup{
		ID:       "group-pending",
		Status:   types.PlanGroupPlanning,
		Decision: "linear",
		Phases:   []types.PhaseRecord{{Index: 0, Goal: "p0", Status: types.PhasePending}},
	}
	if _, err := groupStore.Save(g); err != nil {
		t.Fatalf("Save group: %v", err)
	}

	r.handleMergeGroupCmd("group-pending", "/merge group-pending")
	got := out.String()
	if !strings.Contains(got, "only Completed groups merge") {
		t.Errorf("expected Completed-only refusal; got %q", got)
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
