package types

import "testing"

// 件3 (typed view): the next-action view mirrors the run's persistence-degraded
// signal so renderers disclose it without re-deriving state.
func TestDeriveWriteWorkflowNextActionViewCarriesPersistenceDegraded(t *testing.T) {
	run := WriteWorkflowRun{
		RunID:                     "wf-view",
		Status:                    WriteWorkflowRunInProgress,
		ActiveBatchID:             "b",
		PersistenceDegraded:       true,
		PersistenceDegradedReason: WriteWorkflowPersistenceDegradedSaveFailed,
		Batches: []WriteWorkflowBatch{{
			ID:     "b",
			Status: WriteWorkflowBatchApplying,
		}},
	}
	view := DeriveWriteWorkflowNextActionView(run)
	if !view.PersistenceDegraded || view.PersistenceDegradedReason != WriteWorkflowPersistenceDegradedSaveFailed {
		t.Fatalf("view must mirror persistence degradation, got degraded=%v reason=%q", view.PersistenceDegraded, view.PersistenceDegradedReason)
	}
}

// A healthy run never lights the view flag.
func TestDeriveWriteWorkflowNextActionViewHealthyRunNotDegraded(t *testing.T) {
	run := WriteWorkflowRun{
		RunID:         "wf-view-ok",
		Status:        WriteWorkflowRunInProgress,
		ActiveBatchID: "b",
		Batches: []WriteWorkflowBatch{{
			ID:     "b",
			Status: WriteWorkflowBatchApplying,
		}},
	}
	if view := DeriveWriteWorkflowNextActionView(run); view.PersistenceDegraded {
		t.Fatalf("healthy run view must not be degraded, got %+v", view)
	}
}

// Normalization keeps the reason paired with the flag: a reason with the flag
// off is cleared (precise signal — no orphan reason string).
func TestNormalizeWriteWorkflowRunPersistenceReasonPairing(t *testing.T) {
	cleared := NormalizeWriteWorkflowRun(WriteWorkflowRun{
		RunID:                     "wf-norm-clear",
		PersistenceDegraded:       false,
		PersistenceDegradedReason: WriteWorkflowPersistenceDegradedSaveFailed,
	})
	if cleared.PersistenceDegradedReason != "" {
		t.Fatalf("reason must be cleared when not degraded, got %q", cleared.PersistenceDegradedReason)
	}

	kept := NormalizeWriteWorkflowRun(WriteWorkflowRun{
		RunID:                     "wf-norm-keep",
		PersistenceDegraded:       true,
		PersistenceDegradedReason: WriteWorkflowPersistenceDegradedNoStore,
	})
	if !kept.PersistenceDegraded || kept.PersistenceDegradedReason != WriteWorkflowPersistenceDegradedNoStore {
		t.Fatalf("degraded run must keep its typed reason, got degraded=%v reason=%q", kept.PersistenceDegraded, kept.PersistenceDegradedReason)
	}
}

// FIX-3 symmetry: the flag-on/reason-empty asymmetry backfills a stable typed
// fallback so a degraded run always carries a non-empty reason (mirror of the
// flag-off clear above). The live stamp site always pairs the two, so this only
// covers hand-crafted / legacy inputs — the disclosure must never render a
// degraded run with a blank reason.
func TestNormalizeWriteWorkflowRunPersistenceReasonBackfillWhenFlagOnEmpty(t *testing.T) {
	backfilled := NormalizeWriteWorkflowRun(WriteWorkflowRun{
		RunID:               "wf-norm-backfill",
		PersistenceDegraded: true,
	})
	if !backfilled.PersistenceDegraded {
		t.Fatalf("flag must stay set, got degraded=%v", backfilled.PersistenceDegraded)
	}
	if backfilled.PersistenceDegradedReason != WriteWorkflowPersistenceDegradedUnspecified {
		t.Fatalf("flag-on with an empty reason must backfill the generic fallback, got %q", backfilled.PersistenceDegradedReason)
	}
}
