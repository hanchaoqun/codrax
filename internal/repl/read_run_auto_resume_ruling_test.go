package repl

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// User ruling 2026-07-30 (customer incident: a stale read-run snapshot
// revived after /clear and skipped exploration): auto-resume is
// DEFAULT OFF, and /clear wipes the snapshot store.

func TestReadRunAutoResume_DefaultOffGate(t *testing.T) {
	r, _, _ := newApprovalREPL(t, "", &writeCapableRunner{})
	r.currentMode = types.ModeRead
	r.readRunSnapshotStore = NewReadRunSnapshotStore(t.TempDir())
	if _, err := r.readRunSnapshotStore.Save(&types.ReadRunSnapshot{
		RunID:       "run-stale-1",
		RequestHash: types.ReadRunRequestHash("同一个问题"),
		RepoRoot:    r.repoRoot,
	}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Default: gate closed — even a matching candidate must not seed.
	runID, cleanup := r.installReadRunAutoResumeSeed("同一个问题")
	if runID != "" || cleanup != nil {
		t.Fatalf("default-off gate breached: runID=%q", runID)
	}
}

func TestReadRunSnapshotStore_ClearAll(t *testing.T) {
	store := NewReadRunSnapshotStore(t.TempDir())
	for _, id := range []string{"r1", "r2"} {
		if _, err := store.Save(&types.ReadRunSnapshot{RunID: id, RequestHash: "h-" + id}); err != nil {
			t.Fatalf("Save %s: %v", id, err)
		}
	}
	removed, err := store.ClearAll()
	if err != nil || removed != 2 {
		t.Fatalf("ClearAll = %d, %v; want 2, nil", removed, err)
	}
	infos, err := store.List()
	if err != nil || len(infos) != 0 {
		t.Fatalf("store must be empty after ClearAll; got %d", len(infos))
	}
	// Idempotent on empty.
	if removed, err := store.ClearAll(); err != nil || removed != 0 {
		t.Fatalf("second ClearAll = %d, %v", removed, err)
	}
}

// TestReadRunSnapshotStore_ClearAllRemovesDuplicateFilenames pins
// SWEEPFIX S19: /clear must remove every snapshot FILE — a duplicated
// file whose name differs from its internal run_id (backup copy,
// sync-conflict artifact) must not survive as a live auto-resume
// candidate while the user is told everything was cleared.
func TestReadRunSnapshotStore_ClearAllRemovesDuplicateFilenames(t *testing.T) {
	dir := t.TempDir()
	store := NewReadRunSnapshotStore(dir)
	if _, err := store.Save(&types.ReadRunSnapshot{
		RunID:       "run-a",
		RequestHash: types.ReadRunRequestHash("q"),
		RepoRoot:    "/tmp/repo",
	}); err != nil {
		t.Fatal(err)
	}
	src, err := os.ReadFile(filepath.Join(store.runDir, "run-a.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(store.runDir, "run-b.json"), src, 0o600); err != nil {
		t.Fatal(err)
	}
	removed, err := store.ClearAll()
	if err != nil {
		t.Fatal(err)
	}
	if removed != 2 {
		t.Fatalf("removed = %d, want 2 actual file deletions", removed)
	}
	left, err := os.ReadDir(store.runDir)
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	for _, e := range left {
		if strings.HasSuffix(e.Name(), ".json") {
			t.Fatalf("snapshot file %s survived /clear — live auto-revival candidate", e.Name())
		}
	}
}
