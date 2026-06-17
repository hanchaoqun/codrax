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
				ID:                " attempt-1 ",
				Kind:              " plan ",
				Status:            " complete ",
				ReasonCode:        " ok ",
				FailureReasonCode: " subreason ",
				PlanID:            " plan-1 ",
				ReportID:          " plan-1.report.json ",
				ArtifactRef:       " refs/codrax/applied/plan-1 ",
				SurfaceRef:        " plan-1.attempt-1.surface.json ",
				StartedAt:         time.Date(2026, 6, 10, 1, 2, 3, 0, time.UTC),
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
			FactKeys: []string{
				" runtime_constraint|planner|report-1 ",
				"runtime_constraint|planner|report-1",
			},
			At: time.Date(2026, 6, 10, 1, 3, 0, 0, time.UTC),
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
		got.Batches[0].Attempts[0].PlanID != "plan-1" ||
		got.Batches[0].Attempts[0].FailureReasonCode != "subreason" ||
		got.Batches[0].Attempts[0].ReportID != "plan-1.report.json" ||
		got.Batches[0].Attempts[0].ArtifactRef != "refs/codrax/applied/plan-1" ||
		got.Batches[0].Attempts[0].SurfaceRef != "plan-1.attempt-1.surface.json" ||
		got.Batches[0].Attempts[0].StartedAt.IsZero() {
		t.Fatalf("attempts not normalized/preserved: %+v", got.Batches[0].Attempts)
	}
	if len(got.ProgressLedger) != 1 || got.ProgressLedger[0].At.IsZero() {
		t.Fatalf("progress timestamp not preserved: %+v", got.ProgressLedger)
	}
	if len(got.ProgressLedger[0].FactKeys) != 1 || got.ProgressLedger[0].FactKeys[0] != "runtime_constraint|planner|report-1" {
		t.Fatalf("progress fact keys not normalized: %+v", got.ProgressLedger[0].FactKeys)
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

func TestNormalizeWriteWorkflowRunBackfillsCompletionVerdicts(t *testing.T) {
	finished := time.Date(2026, 6, 16, 8, 0, 0, 0, time.UTC)
	got := NormalizeWriteWorkflowRun(WriteWorkflowRun{
		RunID:         "wf-unverified",
		Status:        WriteWorkflowRunComplete,
		ActiveBatchID: "batch-1",
		Batches: []WriteWorkflowBatch{{
			ID:     "batch-1",
			Status: WriteWorkflowBatchComplete,
			Attempts: []WriteWorkflowAttempt{{
				Kind:       "verify",
				Status:     "unverified",
				ReasonCode: "parser_error",
				FinishedAt: finished,
			}},
		}},
	})
	if got.Batches[0].Completion == nil ||
		got.Batches[0].Completion.Verdict != WriteWorkflowCompletionUnverified ||
		got.Batches[0].Completion.ReasonCode != "parser_error" {
		t.Fatalf("batch completion not backfilled: %+v", got.Batches[0].Completion)
	}
	if got.Completion == nil ||
		got.Completion.Verdict != WriteWorkflowCompletionUnverified ||
		got.Completion.ReasonCode != "parser_error" {
		t.Fatalf("run completion not backfilled: %+v", got.Completion)
	}
}

func TestNormalizeWriteWorkflowRunClearsNonTerminalCompletion(t *testing.T) {
	got := NormalizeWriteWorkflowRun(WriteWorkflowRun{
		RunID:  "wf-in-progress",
		Status: WriteWorkflowRunInProgress,
		Completion: &WriteWorkflowCompletion{
			Verdict:    WriteWorkflowCompletionVerified,
			ReasonCode: "stale",
		},
		Batches: []WriteWorkflowBatch{{
			ID:     "batch-1",
			Status: WriteWorkflowBatchReadyToPlan,
			Completion: &WriteWorkflowCompletion{
				Verdict:    WriteWorkflowCompletionUnverified,
				ReasonCode: "stale",
			},
		}},
	})
	if got.Completion != nil {
		t.Fatalf("non-terminal run completion should be cleared: %+v", got.Completion)
	}
	if got.Batches[0].Completion != nil {
		t.Fatalf("non-terminal batch completion should be cleared: %+v", got.Batches[0].Completion)
	}
}

func TestNormalizeWriteWorkflowRunPersistsSliceState(t *testing.T) {
	finished := time.Date(2026, 6, 16, 9, 0, 0, 0, time.UTC)
	got := NormalizeWriteWorkflowRun(WriteWorkflowRun{
		RunID:         "wf-slices",
		Status:        WriteWorkflowRunInProgress,
		ActiveBatchID: "batch-1",
		Batches: []WriteWorkflowBatch{{
			ID:     " batch-1 ",
			Status: WriteWorkflowBatchApplying,
			Slices: []WriteWorkflowSlice{{
				ID:              " slice-001 ",
				Status:          ChangePlanSliceVerified,
				PlanID:          " plan-1 ",
				ChangeIndexes:   []int{1, 0, 1, -1},
				Paths:           []string{" src/a.py ", "src/a.py", "tests/test_a.py"},
				DependsOnSlices: []string{" slice-000 ", "slice-000"},
				ApplyRef:        " apply-ref ",
				ObserveRef:      " observe-ref ",
				VerifyRef:       " verify-ref ",
				ApprovalRef:     " approval-ref ",
				Checkpoint: &WriteWorkflowCheckpoint{
					Ref:          " refs/codrax/applied/plan-1 ",
					CommitSHA:    " abc123 ",
					WorktreePath: " /tmp/wt ",
					Source:       " slice_apply ",
					ReasonCode:   " applied ",
					At:           finished,
				},
				Restore: &WriteWorkflowRestore{
					CheckpointRef: " refs/codrax/applied/plan-1 ",
					CommitSHA:     " abc123 ",
					WorktreePath:  " /tmp/wt ",
					Source:        " planner_probe ",
					ReasonCode:    " restored ",
					At:            finished,
				},
				ContextPackIDs: []string{" pack-1 ", "pack-1"},
				Completion: &WriteWorkflowCompletion{
					Verdict:    WriteWorkflowCompletionVerified,
					ReasonCode: "slice_passed",
					At:         finished,
				},
				Attempts: []WriteWorkflowAttempt{{
					ID:         " attempt-1 ",
					Kind:       " observe ",
					Status:     " passed ",
					FinishedAt: finished,
				}},
			}, {
				ID:     " slice-002 ",
				Status: ChangePlanSliceApplying,
				Completion: &WriteWorkflowCompletion{
					Verdict:    WriteWorkflowCompletionVerified,
					ReasonCode: "stale",
				},
			}, {
				ID: " ",
			}},
			SliceEvents: []WriteWorkflowSliceEvent{{
				SliceID:     " slice-001 ",
				Event:       WriteWorkflowSliceEventObserveCompleted,
				Status:      ChangePlanSliceVerified,
				ReasonCode:  " passed ",
				PlanID:      " plan-1 ",
				ReportID:    " report-1 ",
				ArtifactRef: " artifact-1 ",
				SurfaceRef:  " surface-1 ",
				At:          finished,
			}, {}},
		}},
	})
	if len(got.Batches) != 1 || got.Batches[0].ActiveSliceID != "slice-001" {
		t.Fatalf("active slice should default to first normalized slice: %+v", got.Batches)
	}
	if len(got.Batches[0].Slices) != 2 {
		t.Fatalf("expected two normalized slices, got %+v", got.Batches[0].Slices)
	}
	first := got.Batches[0].Slices[0]
	if first.ID != "slice-001" || first.PlanID != "plan-1" || first.ApplyRef != "apply-ref" ||
		first.ObserveRef != "observe-ref" || first.VerifyRef != "verify-ref" ||
		first.ApprovalRef != "approval-ref" {
		t.Fatalf("slice refs not normalized: %+v", first)
	}
	if first.Checkpoint == nil || first.Checkpoint.Ref != "refs/codrax/applied/plan-1" ||
		first.Checkpoint.CommitSHA != "abc123" || first.Checkpoint.WorktreePath != "/tmp/wt" ||
		first.Checkpoint.Source != "slice_apply" {
		t.Fatalf("slice checkpoint not normalized: %+v", first.Checkpoint)
	}
	if first.Restore == nil || first.Restore.CheckpointRef != "refs/codrax/applied/plan-1" ||
		first.Restore.CommitSHA != "abc123" || first.Restore.WorktreePath != "/tmp/wt" ||
		first.Restore.Source != "planner_probe" {
		t.Fatalf("slice restore not normalized: %+v", first.Restore)
	}
	if len(first.ChangeIndexes) != 2 || first.ChangeIndexes[0] != 1 || first.ChangeIndexes[1] != 0 {
		t.Fatalf("slice change indexes should preserve order while deduping: %+v", first.ChangeIndexes)
	}
	if len(first.Paths) != 2 || first.Paths[0] != "src/a.py" || first.Paths[1] != "tests/test_a.py" {
		t.Fatalf("slice paths not normalized: %+v", first.Paths)
	}
	if len(first.DependsOnSlices) != 1 || first.DependsOnSlices[0] != "slice-000" {
		t.Fatalf("slice deps not normalized: %+v", first.DependsOnSlices)
	}
	if first.Completion == nil || first.Completion.Verdict != WriteWorkflowCompletionVerified {
		t.Fatalf("terminal slice completion not preserved: %+v", first.Completion)
	}
	if got.Batches[0].Slices[1].Completion != nil {
		t.Fatalf("non-terminal slice completion should be cleared: %+v", got.Batches[0].Slices[1].Completion)
	}
	if len(got.Batches[0].SliceEvents) != 1 ||
		got.Batches[0].SliceEvents[0].Event != WriteWorkflowSliceEventObserveCompleted ||
		got.Batches[0].SliceEvents[0].SliceID != "slice-001" {
		t.Fatalf("slice events not normalized: %+v", got.Batches[0].SliceEvents)
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
