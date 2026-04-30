package repl

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

func newTestGroup(id, status string, phases ...types.PhaseRecord) *types.PlanGroup {
	return &types.PlanGroup{
		ID:        id,
		Goal:      "test goal",
		CreatedAt: time.Now(),
		Status:    types.PlanGroupStatus(status),
		Decision:  "linear",
		Phases:    phases,
	}
}

// TestPlanGroupStore_SaveLoad covers the basic round trip.
func TestPlanGroupStore_SaveLoad(t *testing.T) {
	store := NewPlanGroupStore(t.TempDir())
	original := newTestGroup("group-test-001", "planning",
		types.PhaseRecord{Index: 0, Goal: "phase one", Status: types.PhasePending},
		types.PhaseRecord{Index: 1, Goal: "phase two", Status: types.PhasePending},
	)
	path, err := store.Save(original)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if !strings.HasSuffix(path, "group-test-001.json") {
		t.Errorf("Save path drift: got %q", path)
	}
	loaded, err := store.Load("group-test-001")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded == nil {
		t.Fatal("Load returned nil")
	}
	if loaded.ID != "group-test-001" {
		t.Errorf("ID drift: got %q", loaded.ID)
	}
	if len(loaded.Phases) != 2 {
		t.Errorf("Phases length drift: got %d", len(loaded.Phases))
	}
}

// TestPlanGroupStore_LoadMissing covers the absent-id path.
func TestPlanGroupStore_LoadMissing(t *testing.T) {
	store := NewPlanGroupStore(t.TempDir())
	g, err := store.Load("nonexistent")
	if err != nil {
		t.Errorf("missing id should not error; got %v", err)
	}
	if g != nil {
		t.Errorf("missing id should yield nil; got %+v", g)
	}
}

// TestPlanGroupStore_NilGuards pins the defensive paths.
func TestPlanGroupStore_NilGuards(t *testing.T) {
	var s *PlanGroupStore
	if _, err := s.Save(&types.PlanGroup{ID: "x"}); err == nil {
		t.Error("nil store Save should reject")
	}
	if _, err := s.Load("x"); err == nil {
		t.Error("nil store Load should reject")
	}
	if err := s.Clear("x"); err == nil {
		t.Error("nil store Clear should reject")
	}
	if got := s.GroupDir(); got != "" {
		t.Errorf("nil store GroupDir should be empty; got %q", got)
	}

	store := NewPlanGroupStore(t.TempDir())
	if _, err := store.Save(nil); err == nil {
		t.Error("nil group must reject")
	}
	if _, err := store.Save(&types.PlanGroup{}); err == nil {
		t.Error("empty ID must reject")
	}
	if _, err := store.Load(""); err == nil {
		t.Error("empty id must reject")
	}
	if err := store.Clear(""); err == nil {
		t.Error("empty id must reject")
	}
}

// TestPlanGroupStore_Clear covers the delete path including
// idempotency on missing files.
func TestPlanGroupStore_Clear(t *testing.T) {
	store := NewPlanGroupStore(t.TempDir())
	g := newTestGroup("group-clear-test", "planning")
	if _, err := store.Save(g); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := store.Clear("group-clear-test"); err != nil {
		t.Errorf("Clear: %v", err)
	}
	if g, _ := store.Load("group-clear-test"); g != nil {
		t.Error("group should be gone after Clear")
	}
	// Idempotent: clearing a non-existent group is OK.
	if err := store.Clear("group-clear-test"); err != nil {
		t.Errorf("idempotent Clear: %v", err)
	}
}

// TestPlanGroupStore_List covers the directory walk + status
// probe + newest-first sorting.
func TestPlanGroupStore_List(t *testing.T) {
	store := NewPlanGroupStore(t.TempDir())
	for i, st := range []string{"planning", "in_flight", "completed"} {
		g := newTestGroup(fmt.Sprintf("group-list-%d", i), st)
		if _, err := store.Save(g); err != nil {
			t.Fatalf("Save: %v", err)
		}
		// Ensure ordering by mtime is deterministic.
		time.Sleep(10 * time.Millisecond)
	}
	infos, err := store.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(infos) != 3 {
		t.Fatalf("expected 3 groups; got %d", len(infos))
	}
	// Newest-first ordering — group-list-2 was saved last.
	if infos[0].ID != "group-list-2" {
		t.Errorf("expected group-list-2 first (newest); got %q", infos[0].ID)
	}
	for _, inf := range infos {
		if inf.Decision != "linear" {
			t.Errorf("expected decision='linear'; got %q on %s", inf.Decision, inf.ID)
		}
	}
}

// TestPlanGroupStore_FindActiveGroup pins the active-group
// lookup used by /phase show without an explicit ID.
func TestPlanGroupStore_FindActiveGroup(t *testing.T) {
	store := NewPlanGroupStore(t.TempDir())
	// Three groups: two terminal, one active. FindActiveGroup
	// returns the active one regardless of mtime ordering.
	g1 := newTestGroup("group-done", "completed")
	g2 := newTestGroup("group-active", "in_flight")
	g3 := newTestGroup("group-failed", "failed")
	for _, g := range []*types.PlanGroup{g1, g3, g2} {
		if _, err := store.Save(g); err != nil {
			t.Fatalf("Save: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	got, err := store.FindActiveGroup()
	if err != nil {
		t.Fatalf("FindActiveGroup: %v", err)
	}
	if got == nil || got.ID != "group-active" {
		t.Errorf("expected group-active; got %v", got)
	}
}

// TestPlanGroupStore_FindActiveGroup_NoneActive covers the
// all-terminal path.
func TestPlanGroupStore_FindActiveGroup_NoneActive(t *testing.T) {
	store := NewPlanGroupStore(t.TempDir())
	if _, err := store.Save(newTestGroup("done", "completed")); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := store.FindActiveGroup()
	if err != nil {
		t.Errorf("err: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil when all groups terminal; got %+v", got)
	}
}

// TestPlanGroupStore_TmpFileSkipped pins the .tmp filter — a
// .tmp file from an interrupted Save shouldn't appear in List.
func TestPlanGroupStore_TmpFileSkipped(t *testing.T) {
	store := NewPlanGroupStore(t.TempDir())
	if err := os.MkdirAll(store.GroupDir(), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(store.GroupDir(), "leftover.json.tmp"), []byte("{}"), 0o644); err != nil {
		t.Fatalf("seed tmp: %v", err)
	}
	infos, err := store.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	for _, inf := range infos {
		if strings.HasSuffix(inf.ID, ".tmp") {
			t.Errorf(".tmp file leaked into List output: %s", inf.ID)
		}
	}
}
