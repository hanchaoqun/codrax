package repl

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestReadRunSnapshotStoreSaveLoadListClear(t *testing.T) {
	store := NewReadRunSnapshotStore(t.TempDir())
	snapshot := &types.ReadRunSnapshot{
		RunID:            "read-1",
		Request:          "explain agents",
		TaskGraphHash:    "hash-1",
		TaskNodeCount:    2,
		NodeStatuses:     map[string]types.NodeExecStatus{"explore": types.NodeExecDone},
		ReadSet:          []string{"internal/agent/agent.go"},
		AcceptedEvidence: []types.AcceptedEvidenceRef{{ID: "ev-agent", Source: "internal/agent/agent.go"}},
		ProgressDecision: types.ProgressDecision{ReasonCode: types.ProgressReasonContinue, ShouldReplan: true},
	}
	path, err := store.Save(snapshot)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("saved file missing: %v", err)
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Fatalf("tmp file should not remain after successful save, stat err=%v", err)
	}
	loaded, err := store.Load("read-1")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded == nil || loaded.RunID != "read-1" || loaded.NodeStatuses["explore"] != types.NodeExecDone {
		t.Fatalf("unexpected loaded snapshot: %+v", loaded)
	}
	infos, err := store.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(infos) != 1 || infos[0].ID != "read-1" || infos[0].ReadFileCount != 1 || infos[0].AcceptedEvidence != 1 {
		t.Fatalf("unexpected list info: %+v", infos)
	}
	if err := os.WriteFile(filepath.Join(store.RunDir(), "ignored.json.tmp"), []byte("{}"), 0o644); err != nil {
		t.Fatalf("write tmp: %v", err)
	}
	infos, err = store.List()
	if err != nil {
		t.Fatalf("List after tmp: %v", err)
	}
	if len(infos) != 1 {
		t.Fatalf("tmp files should be skipped, got %+v", infos)
	}
	if err := store.Clear("read-1"); err != nil {
		t.Fatalf("Clear: %v", err)
	}
	loaded, err = store.Load("read-1")
	if err != nil {
		t.Fatalf("Load after Clear: %v", err)
	}
	if loaded != nil {
		t.Fatalf("Load after Clear should return nil, got %+v", loaded)
	}
}

func TestReadRunSnapshotStoreRejectsInvalidID(t *testing.T) {
	store := NewReadRunSnapshotStore(t.TempDir())
	if _, err := store.Save(&types.ReadRunSnapshot{RunID: "../escape"}); err == nil {
		t.Fatal("Save should reject traversal-shaped id")
	}
	if _, err := store.Load("../escape"); err == nil {
		t.Fatal("Load should reject traversal-shaped id")
	}
	if err := store.Clear("../escape"); err == nil {
		t.Fatal("Clear should reject traversal-shaped id")
	}
}
