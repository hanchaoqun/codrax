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
			Items: []types.WriteContextItem{{
				Priority: types.WriteContextP1,
				Kind:     "evidence_ref",
				Text:     "internal/foo.go:12",
			}},
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
	contextPath := filepath.Join(store.WorkflowDir(), "contexts", "wf-1", "wf-1-batch-1-pack-1.json")
	pack, err := types.LoadWriteContextPackFromFile(contextPath)
	if err != nil {
		t.Fatalf("LoadWriteContextPackFromFile: %v", err)
	}
	if pack == nil || pack.PackID != "wf-1-batch-1-pack-1" {
		t.Fatalf("unexpected context pack artifact: %+v", pack)
	}
	loaded, err = store.Load("wf-1")
	if err != nil {
		t.Fatalf("Load after context pack save: %v", err)
	}
	if len(loaded.Batches[0].ContextPackIDs) != 1 || loaded.Batches[0].ContextPackIDs[0] != "wf-1-batch-1-pack-1" {
		t.Fatalf("batch should reference context artifact: %+v", loaded.Batches[0].ContextPackIDs)
	}
	active, err := store.FindActiveRun()
	if err != nil {
		t.Fatalf("FindActiveRun: %v", err)
	}
	if active == nil || active.RunID != "wf-1" {
		t.Fatalf("unexpected active run: %+v", active)
	}
	if err := store.Clear("wf-1"); err != nil {
		t.Fatalf("Clear: %v", err)
	}
	if _, err := os.Stat(contextPath); !os.IsNotExist(err) {
		t.Fatalf("context artifact dir should be cleared, stat err=%v", err)
	}
	loaded, err = store.Load("wf-1")
	if err != nil {
		t.Fatalf("Load after Clear: %v", err)
	}
	if loaded != nil {
		t.Fatal("Load after Clear should return nil run")
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

func TestWriteWorkflowRunStoreFindsPendingApprovalAsActive(t *testing.T) {
	store := NewWriteWorkflowRunStore(t.TempDir())
	if _, err := store.Save(&types.WriteWorkflowRun{
		RunID:         "wf-pending-approval",
		Status:        types.WriteWorkflowRunInProgress,
		ActiveBatchID: "batch-approval",
		Batches: []types.WriteWorkflowBatch{{
			ID:     "batch-approval",
			Status: types.WriteWorkflowBatchPendingApproval,
			PlanID: "plan-high-risk",
		}},
	}); err != nil {
		t.Fatalf("Save pending approval workflow: %v", err)
	}
	active, err := store.FindActiveRun()
	if err != nil {
		t.Fatalf("FindActiveRun: %v", err)
	}
	if active == nil || active.RunID != "wf-pending-approval" {
		t.Fatalf("pending approval workflow should remain active, got %+v", active)
	}
	if len(active.Batches) != 1 || active.Batches[0].Status != types.WriteWorkflowBatchPendingApproval {
		t.Fatalf("active workflow should preserve pending approval batch: %+v", active)
	}
}
