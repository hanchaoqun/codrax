package types

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNormalizeWriteWorkflowRunPersistsContextPacks(t *testing.T) {
	run := WriteWorkflowRun{
		RunID:         " run-1 ",
		Goal:          " ship workflow ",
		Status:        WriteWorkflowRunInProgress,
		ActiveBatchID: " batch-1 ",
		Batches: []WriteWorkflowBatch{{
			ID:             " batch-1 ",
			Status:         WriteWorkflowBatchNeedsExploration,
			ContextPackIDs: []string{" pack-1 ", "pack-1"},
		}},
		Edges: []WriteWorkflowEdge{{
			FromBatchID: "seed",
			ToBatchID:   "batch-1",
			Kind:        WriteWorkflowEdgeExplore,
		}},
		ContextPacks: []WriteContextPack{{
			PackID:  " pack-1 ",
			BatchID: " batch-1 ",
			Items: []WriteContextItem{{
				Priority: WriteContextP0,
				Kind:     "constraint",
				Text:     "preserve read mode",
			}},
		}},
		Budget: WriteWorkflowBudget{
			MaxBatches:            5,
			MaxExplorationRounds:  2,
			BatchesUsed:           -1,
			ExplorationRoundsUsed: -1,
		},
		ProgressLedger: []WriteWorkflowProgress{{
			BatchID: " batch-1 ",
			Stage:   " explore ",
			Status:  " complete ",
		}},
	}
	got := NormalizeWriteWorkflowRun(run)
	if got.RunID != "run-1" || got.ActiveBatchID != "batch-1" {
		t.Fatalf("run identity not normalized: %+v", got)
	}
	if len(got.ContextPacks) != 1 || got.ContextPacks[0].PackID != "pack-1" {
		t.Fatalf("context pack not persisted/normalized: %+v", got.ContextPacks)
	}
	if len(got.Batches) != 1 || len(got.Batches[0].ContextPackIDs) != 1 || got.Batches[0].ContextPackIDs[0] != "pack-1" {
		t.Fatalf("batch context pack ids not normalized: %+v", got.Batches)
	}
	if got.Budget.BatchesUsed != 0 || got.Budget.ExplorationRoundsUsed != 0 {
		t.Fatalf("negative budget usage not clamped: %+v", got.Budget)
	}
	got.ContextPacks[0].Items[0].Text = "mutated"
	again := NormalizeWriteWorkflowRun(run)
	if again.ContextPacks[0].Items[0].Text != "preserve read mode" {
		t.Fatalf("normalization should return defensive slices")
	}
}

func TestWriteWorkflowRunToFileRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "wf-1.json")
	run := &WriteWorkflowRun{
		RunID:         "wf-1",
		Goal:          "ship controller",
		Status:        WriteWorkflowRunInProgress,
		ActiveBatchID: "batch-1",
		Batches: []WriteWorkflowBatch{{
			ID:     "batch-1",
			Status: WriteWorkflowBatchReadyToPlan,
		}},
	}
	if err := WriteWorkflowRunToFile(run, path); err != nil {
		t.Fatalf("WriteWorkflowRunToFile: %v", err)
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Fatalf("tmp file should not remain after successful write, stat err=%v", err)
	}
	loaded, err := LoadWriteWorkflowRunFromFile(path)
	if err != nil {
		t.Fatalf("LoadWriteWorkflowRunFromFile: %v", err)
	}
	if loaded == nil || loaded.RunID != "wf-1" || loaded.Batches[0].ID != "batch-1" {
		t.Fatalf("unexpected loaded run: %+v", loaded)
	}
}
