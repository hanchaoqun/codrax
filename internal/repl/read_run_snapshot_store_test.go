package repl

import (
	"os"
	"path/filepath"
	"testing"
	"time"

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

func TestReadRunSnapshotStoreLoadComparablePrior(t *testing.T) {
	store := NewReadRunSnapshotStore(t.TempDir())
	base := types.ReadRunSnapshot{
		Request:       "same request",
		RequestHash:   types.ReadRunRequestHash("same request"),
		RepoRoot:      "/tmp/repo",
		TaskGraphHash: "graph-1",
	}
	snapshots := []types.ReadRunSnapshot{
		{RunID: "read-old-1", RequestHash: base.RequestHash, RepoRoot: base.RepoRoot, TaskGraphHash: base.TaskGraphHash},
		{RunID: "read-old-2", RequestHash: base.RequestHash, RepoRoot: base.RepoRoot + "/", TaskGraphHash: base.TaskGraphHash},
		{RunID: "read-other-request", RequestHash: "different", RepoRoot: base.RepoRoot, TaskGraphHash: base.TaskGraphHash},
		{RunID: "read-current", RequestHash: base.RequestHash, RepoRoot: base.RepoRoot, TaskGraphHash: base.TaskGraphHash},
		{RunID: "read-future", RequestHash: base.RequestHash, RepoRoot: base.RepoRoot, TaskGraphHash: base.TaskGraphHash},
	}
	for _, snapshot := range snapshots {
		snapshot.SchemaVersion = types.ReadRunSnapshotSchemaVersion
		if _, err := store.Save(&snapshot); err != nil {
			t.Fatalf("Save %s: %v", snapshot.RunID, err)
		}
	}
	baseTime := time.Date(2026, 6, 22, 10, 0, 0, 0, time.UTC)
	for i, id := range []string{"read-old-1", "read-other-request", "read-old-2", "read-current", "read-future"} {
		path := filepath.Join(store.RunDir(), id+".json")
		ts := baseTime.Add(time.Duration(i) * time.Minute)
		if err := os.Chtimes(path, ts, ts); err != nil {
			t.Fatalf("Chtimes %s: %v", id, err)
		}
	}
	current, err := store.Load("read-current")
	if err != nil {
		t.Fatalf("Load current: %v", err)
	}
	prior, err := store.LoadComparablePrior(*current)
	if err != nil {
		t.Fatalf("LoadComparablePrior: %v", err)
	}
	if prior == nil || prior.RunID != "read-old-2" {
		t.Fatalf("prior = %+v, want read-old-2", prior)
	}
	missingKey := *current
	missingKey.RequestHash = ""
	prior, err = store.LoadComparablePrior(missingKey)
	if err != nil {
		t.Fatalf("LoadComparablePrior missing key: %v", err)
	}
	if prior != nil {
		t.Fatalf("missing comparable key should not select prior, got %+v", prior)
	}
}
