package repl

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// TestPlanClear_ByIDDeletesPlanAndReport verifies the targeted form
// `/plan clear <plan-id>` removes both the plan JSON and its
// sibling .report.json, while leaving other plans untouched.
func TestPlanClear_ByIDDeletesPlanAndReport(t *testing.T) {
	store := NewPlanStore(t.TempDir())
	target, err := store.SaveForTest(&types.ChangePlan{
		ID: "plan-target", Status: types.PlanStatusVerifyFailed,
		Changes: []types.FileChange{{Path: "x.go", Kind: "modify"}},
	})
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	other, err := store.SaveForTest(&types.ChangePlan{
		ID: "plan-other", Status: types.PlanStatusApplied,
		Changes: []types.FileChange{{Path: "y.go", Kind: "modify"}},
	})
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	// Synthesize the sibling report file for plan-target.
	reportPath := filepath.Join(store.PlanDir(), "plan-target.report.json")
	if err := os.WriteFile(reportPath, []byte(`{}`), 0o644); err != nil {
		t.Fatalf("write report: %v", err)
	}

	r, _ := newScriptedREPL(t, store)
	r.handlePlanCmd("/plan clear plan-target")

	// Target plan + report gone.
	if _, err := os.Stat(target); err == nil {
		t.Errorf("plan-target JSON should be deleted")
	}
	if _, err := os.Stat(reportPath); err == nil {
		t.Errorf("plan-target.report.json should be deleted")
	}
	// Other plan untouched.
	if _, err := os.Stat(other); err != nil {
		t.Errorf("plan-other should NOT have been deleted; got: %v", err)
	}
}

// TestPlanClear_ByIDClearsPendingPointerWhenItMatches verifies the
// in-session pendingPlanPath is dropped when the plan it points at
// is the one being cleared. Otherwise a stale pointer would let
// /approve / /plan show resurrect a phantom path.
func TestPlanClear_ByIDClearsPendingPointerWhenItMatches(t *testing.T) {
	store := NewPlanStore(t.TempDir())
	target, _ := store.SaveForTest(&types.ChangePlan{
		ID: "plan-pending", Status: types.PlanStatusPending,
		Changes: []types.FileChange{{Path: "x.go", Kind: "modify"}},
	})
	r, _ := newScriptedREPL(t, store)
	r.pendingPlanPath = target

	r.handlePlanCmd("/plan clear plan-pending")

	if r.pendingPlanPath != "" {
		t.Errorf("pendingPlanPath should be cleared after deleting the plan it pointed at; got %q",
			r.pendingPlanPath)
	}
}

// TestPlanClear_ByIDPreservesPointerWhenIDDiffers covers the
// inverse: clearing some other plan must NOT zap the pending
// pointer. Without this guard, deleting a stale rejected plan
// would silently drop the user's current /mode write output.
func TestPlanClear_ByIDPreservesPointerWhenIDDiffers(t *testing.T) {
	store := NewPlanStore(t.TempDir())
	pendingPath, _ := store.SaveForTest(&types.ChangePlan{
		ID: "plan-keep", Status: types.PlanStatusPending,
		Changes: []types.FileChange{{Path: "x.go", Kind: "modify"}},
	})
	_, _ = store.SaveForTest(&types.ChangePlan{
		ID: "plan-trash", Status: types.PlanStatusRejected,
		Changes: []types.FileChange{{Path: "y.go", Kind: "modify"}},
	})
	r, _ := newScriptedREPL(t, store)
	r.pendingPlanPath = pendingPath

	r.handlePlanCmd("/plan clear plan-trash")

	if r.pendingPlanPath != pendingPath {
		t.Errorf("pendingPlanPath should survive deletion of an unrelated plan; got %q want %q",
			r.pendingPlanPath, pendingPath)
	}
}

// TestPlanClear_ByID_NotFound covers the user-error path: typo'd
// plan id. Should print the planNotFound message and not delete
// anything.
func TestPlanClear_ByID_NotFound(t *testing.T) {
	store := NewPlanStore(t.TempDir())
	_, _ = store.SaveForTest(&types.ChangePlan{
		ID: "plan-real", Status: types.PlanStatusApplied,
		Changes: []types.FileChange{{Path: "x.go", Kind: "modify"}},
	})
	r, out := newScriptedREPL(t, store)

	r.handlePlanCmd("/plan clear plan-typo-NOPE")

	if !strings.Contains(out.String(), "plan-typo-NOPE") {
		t.Errorf("error should name the missing id; got: %q", out.String())
	}
	// plan-real must still exist.
	if _, err := os.Stat(filepath.Join(store.PlanDir(), "plan-real.json")); err != nil {
		t.Errorf("unrelated plan should not be touched; got: %v", err)
	}
}

// TestPlanClear_StatusFilterScripted covers the bulk-by-status form
// in scripted mode (no interactive confirm). Only plans with the
// matching status get deleted.
func TestPlanClear_StatusFilterScripted(t *testing.T) {
	store := NewPlanStore(t.TempDir())
	for _, p := range []*types.ChangePlan{
		{ID: "plan-a", Status: types.PlanStatusRejected, Changes: []types.FileChange{{Path: "x.go", Kind: "modify"}}},
		{ID: "plan-b", Status: types.PlanStatusRejected, Changes: []types.FileChange{{Path: "y.go", Kind: "modify"}}},
		{ID: "plan-c", Status: types.PlanStatusApplied, Changes: []types.FileChange{{Path: "z.go", Kind: "modify"}}},
	} {
		if _, err := store.SaveForTest(p); err != nil {
			t.Fatalf("save %s: %v", p.ID, err)
		}
	}
	r, _ := newScriptedREPL(t, store)

	r.handlePlanCmd("/plan clear --status=rejected")

	// Both rejected plans gone.
	for _, id := range []string{"plan-a", "plan-b"} {
		if _, err := os.Stat(filepath.Join(store.PlanDir(), id+".json")); err == nil {
			t.Errorf("%s should be deleted", id)
		}
	}
	// applied plan untouched.
	if _, err := os.Stat(filepath.Join(store.PlanDir(), "plan-c.json")); err != nil {
		t.Errorf("plan-c (applied) should NOT be deleted; got: %v", err)
	}
}

// TestPlanClear_AllScripted covers the most-aggressive form:
// `/plan clear --all` in scripted mode wipes every plan in store.
func TestPlanClear_AllScripted(t *testing.T) {
	store := NewPlanStore(t.TempDir())
	for _, id := range []string{"p1", "p2", "p3"} {
		_, _ = store.SaveForTest(&types.ChangePlan{
			ID: id, Status: types.PlanStatusApplied,
			Changes: []types.FileChange{{Path: id + ".go", Kind: "modify"}},
		})
	}
	r, _ := newScriptedREPL(t, store)

	r.handlePlanCmd("/plan clear --all")

	infos, _ := store.List()
	if len(infos) != 0 {
		t.Errorf("after --all clear, store should be empty; got %d plans", len(infos))
	}
}

// TestPlanClear_StatusFilter_NoMatch verifies the friendly no-op
// path: --status= with a value that no plan matches just prints
// a message instead of erroring.
func TestPlanClear_StatusFilter_NoMatch(t *testing.T) {
	store := NewPlanStore(t.TempDir())
	_, _ = store.SaveForTest(&types.ChangePlan{
		ID: "p1", Status: types.PlanStatusApplied,
		Changes: []types.FileChange{{Path: "x.go", Kind: "modify"}},
	})
	r, out := newScriptedREPL(t, store)

	r.handlePlanCmd("/plan clear --status=rejected")

	if !strings.Contains(out.String(), "no plans match") {
		t.Errorf("should print 'no plans match' message; got: %q", out.String())
	}
	// p1 untouched.
	if _, err := os.Stat(filepath.Join(store.PlanDir(), "p1.json")); err != nil {
		t.Errorf("p1 should NOT be deleted on no-match; got: %v", err)
	}
}

// TestPlanClear_BareStillWorks verifies the existing pending-only
// form is unchanged: `/plan clear` (no arg) drops the pending
// pointer + deletes the file pointed at.
func TestPlanClear_BareStillWorks(t *testing.T) {
	store := NewPlanStore(t.TempDir())
	pendingPath, _ := store.SaveForTest(&types.ChangePlan{
		ID: "pending-only", Status: types.PlanStatusPending,
		Changes: []types.FileChange{{Path: "x.go", Kind: "modify"}},
	})
	r, _ := newScriptedREPL(t, store)
	r.pendingPlanPath = pendingPath

	r.handlePlanCmd("/plan clear")

	if _, err := os.Stat(pendingPath); err == nil {
		t.Errorf("pending plan file should be deleted")
	}
	if r.pendingPlanPath != "" {
		t.Errorf("pendingPlanPath should be cleared; got %q", r.pendingPlanPath)
	}
}
