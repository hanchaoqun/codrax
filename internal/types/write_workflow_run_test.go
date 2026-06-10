package types

import (
	"os"
	"path/filepath"
	"testing"
	"time"
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
			DependsOn:      []string{" batch-0 ", "batch-0"},
			PlanRef:        " plan-ref ",
			ApplyRef:       " apply-ref ",
			VerifyRef:      " verify-ref ",
			ApprovalRef:    " approval-ref ",
			ContextPackIDs: []string{" pack-1 ", "pack-1"},
			Attempts: []WriteWorkflowAttempt{{
				ID:         " attempt-1 ",
				Kind:       " plan ",
				Status:     " complete ",
				ReasonCode: " ok ",
				PlanID:     " plan-1 ",
				StartedAt:  time.Date(2026, 6, 10, 1, 2, 3, 0, time.UTC),
			}},
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
			At:      time.Date(2026, 6, 10, 1, 3, 0, 0, time.UTC),
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
	if len(got.Batches[0].DependsOn) != 1 || got.Batches[0].DependsOn[0] != "batch-0" {
		t.Fatalf("batch dependencies not normalized: %+v", got.Batches[0].DependsOn)
	}
	if got.Batches[0].PlanRef != "plan-ref" || got.Batches[0].ApplyRef != "apply-ref" ||
		got.Batches[0].VerifyRef != "verify-ref" || got.Batches[0].ApprovalRef != "approval-ref" {
		t.Fatalf("batch refs not normalized: %+v", got.Batches[0])
	}
	if len(got.Batches[0].Attempts) != 1 || got.Batches[0].Attempts[0].ID != "attempt-1" ||
		got.Batches[0].Attempts[0].PlanID != "plan-1" || got.Batches[0].Attempts[0].StartedAt.IsZero() {
		t.Fatalf("attempts not normalized/preserved: %+v", got.Batches[0].Attempts)
	}
	if len(got.ProgressLedger) != 1 || got.ProgressLedger[0].At.IsZero() {
		t.Fatalf("progress timestamp not preserved: %+v", got.ProgressLedger)
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
