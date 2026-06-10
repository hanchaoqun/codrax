package repl

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestWriteWorkflowRunStoreSaveLoadList(t *testing.T) {
	store := NewWriteWorkflowRunStore(t.TempDir())
	run := &types.WriteWorkflowRun{
		RunID:         "wf-1",
		Goal:          "ship controller",
		Status:        types.WriteWorkflowRunInProgress,
		ActiveBatchID: "batch-1",
		Batches: []types.WriteWorkflowBatch{{
			ID:     "batch-1",
			Status: types.WriteWorkflowBatchReadyToPlan,
		}},
		ContextPacks: []types.WriteContextPack{{
			PackID:  "pack-1",
			BatchID: "batch-1",
		}},
	}
	path, err := store.Save(run)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("saved file missing: %v", err)
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Fatalf("tmp file should not remain after successful save, stat err=%v", err)
	}
	loaded, err := store.Load("wf-1")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded == nil || loaded.RunID != "wf-1" || loaded.ActiveBatchID != "batch-1" {
		t.Fatalf("unexpected loaded run: %+v", loaded)
	}
	infos, err := store.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(infos) != 1 || infos[0].ID != "wf-1" || infos[0].Batches != 1 || infos[0].ContextPacks != 1 {
		t.Fatalf("unexpected list info: %+v", infos)
	}
	active, err := store.FindActiveRun()
	if err != nil {
		t.Fatalf("FindActiveRun: %v", err)
	}
	if active == nil || active.RunID != "wf-1" {
		t.Fatalf("unexpected active run: %+v", active)
	}
}

func TestWriteWorkflowRunStoreRejectsInvalidID(t *testing.T) {
	store := NewWriteWorkflowRunStore(t.TempDir())
	if _, err := store.Save(&types.WriteWorkflowRun{RunID: "../escape"}); err == nil {
		t.Fatal("Save should reject traversal-shaped id")
	}
	if _, err := store.Load("../escape"); err == nil {
		t.Fatal("Load should reject traversal-shaped id")
	}
	if err := store.Clear("../escape"); err == nil {
		t.Fatal("Clear should reject traversal-shaped id")
	}
}

func TestWriteWorkflowRunStoreSkipsTmpAndTerminalActive(t *testing.T) {
	dir := t.TempDir()
	store := NewWriteWorkflowRunStore(dir)
	terminal := &types.WriteWorkflowRun{RunID: "wf-done", Status: types.WriteWorkflowRunComplete}
	if _, err := store.Save(terminal); err != nil {
		t.Fatalf("Save terminal: %v", err)
	}
	if err := os.WriteFile(filepath.Join(store.WorkflowDir(), "wf-tmp.json.tmp"), []byte("{}"), 0o644); err != nil {
		t.Fatalf("write tmp: %v", err)
	}
	infos, err := store.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(infos) != 1 || infos[0].ID != "wf-done" {
		t.Fatalf("tmp file should be skipped, got %+v", infos)
	}
	active, err := store.FindActiveRun()
	if err != nil {
		t.Fatalf("FindActiveRun: %v", err)
	}
	if active != nil {
		t.Fatalf("terminal run should not be active: %+v", active)
	}
}
