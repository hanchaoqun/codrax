package orchestrator

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/types"
)

// failingWorkflowRunStore is a WriteWorkflowRunSaver whose Save always returns
// an error, exercising the persistence-degradation path (§29.213 排期件4
// PERSIST-1). saveCount lets tests assert the store was actually consulted so a
// "no degradation" result cannot silently come from Save never being called.
type failingWorkflowRunStore struct {
	saveCount int
	err       error
}

func (s *failingWorkflowRunStore) Save(run *types.WriteWorkflowRun) (string, error) {
	s.saveCount++
	if s.err != nil {
		return "", s.err
	}
	return "", errors.New("save failed")
}

func persistDegradedTestRun(runID string) *types.WriteWorkflowRun {
	return &types.WriteWorkflowRun{
		RunID:         runID,
		Status:        types.WriteWorkflowRunInProgress,
		ActiveBatchID: "batch-1",
		Batches: []types.WriteWorkflowBatch{{
			ID:     "batch-1",
			Status: types.WriteWorkflowBatchApplying,
		}},
	}
}

// 件1 (Save error): a failing store stamps the precise typed signal on the run
// pointer AND the bus copy, instead of the old silent Warning log.
func TestPersistWriteWorkflowRunSaveErrorSetsPersistenceDegraded(t *testing.T) {
	mu := types.NewMutableState("save error")
	o := &Orchestrator{busCtx: &types.BusContext{Mutable: mu, WorkDir: t.TempDir(), Mode: types.ModeApply}}
	store := &failingWorkflowRunStore{err: errors.New("disk full")}
	o.writeWorkflowRunStore = store

	run := persistDegradedTestRun("wf-save-err")
	o.persistWriteWorkflowRun(run)

	if store.saveCount == 0 {
		t.Fatal("expected Save to be attempted")
	}
	if !run.PersistenceDegraded {
		t.Fatalf("Save error must set PersistenceDegraded on the run pointer; got false")
	}
	if run.PersistenceDegradedReason != types.WriteWorkflowPersistenceDegradedSaveFailed {
		t.Fatalf("PersistenceDegradedReason=%q, want %q", run.PersistenceDegradedReason, types.WriteWorkflowPersistenceDegradedSaveFailed)
	}
	busRun := mu.WriteWorkflowRun()
	if busRun == nil || !busRun.PersistenceDegraded || busRun.PersistenceDegradedReason != types.WriteWorkflowPersistenceDegradedSaveFailed {
		t.Fatalf("bus copy must disclose the typed degradation, got %+v", busRun)
	}
}

// 件1 (store == nil): a run driven with no durable store is degraded with the
// no_durable_store reason on every persist, in memory and on the bus.
func TestPersistWriteWorkflowRunNilStoreSetsPersistenceDegraded(t *testing.T) {
	mu := types.NewMutableState("nil store")
	o := &Orchestrator{busCtx: &types.BusContext{Mutable: mu, WorkDir: t.TempDir(), Mode: types.ModeApply}}

	run := persistDegradedTestRun("wf-nil-store")
	o.persistWriteWorkflowRun(run)

	if !run.PersistenceDegraded || run.PersistenceDegradedReason != types.WriteWorkflowPersistenceDegradedNoStore {
		t.Fatalf("nil store must set no_durable_store degradation, got degraded=%v reason=%q", run.PersistenceDegraded, run.PersistenceDegradedReason)
	}
	busRun := mu.WriteWorkflowRun()
	if busRun == nil || !busRun.PersistenceDegraded || busRun.PersistenceDegradedReason != types.WriteWorkflowPersistenceDegradedNoStore {
		t.Fatalf("bus copy must disclose nil-store degradation, got %+v", busRun)
	}
}

// 件4 positive arm: a successful Save never degrades — zero regression.
func TestPersistWriteWorkflowRunSuccessKeepsPersistenceNotDegraded(t *testing.T) {
	mu := types.NewMutableState("save ok")
	o := &Orchestrator{busCtx: &types.BusContext{Mutable: mu, WorkDir: t.TempDir(), Mode: types.ModeApply}}
	o.writeWorkflowRunStore = &fakeWorkflowRunStore{}

	run := persistDegradedTestRun("wf-ok")
	o.persistWriteWorkflowRun(run)

	if run.PersistenceDegraded || run.PersistenceDegradedReason != "" {
		t.Fatalf("successful Save must not degrade, got degraded=%v reason=%q", run.PersistenceDegraded, run.PersistenceDegradedReason)
	}
	if busRun := mu.WriteWorkflowRun(); busRun == nil || busRun.PersistenceDegraded {
		t.Fatalf("bus copy must not be degraded on the happy path, got %+v", busRun)
	}
}

// Sticky lifetime: once degraded, the flag rides forward on the run pointer and
// is flushed to disk on the next successful Save ("下次 Save 成功即落盘"). This
// also pins the mirror onto the caller's pointer — remove it and the second
// persist starts clean and never flushes the gap.
func TestPersistWriteWorkflowRunDegradedFlagStickyFlushedOnNextSuccess(t *testing.T) {
	mu := types.NewMutableState("sticky")
	o := &Orchestrator{busCtx: &types.BusContext{Mutable: mu, WorkDir: t.TempDir(), Mode: types.ModeApply}}

	run := persistDegradedTestRun("wf-sticky")
	o.persistWriteWorkflowRun(run) // no store wired -> degrade
	if !run.PersistenceDegraded {
		t.Fatal("first persist without a store must degrade")
	}

	ok := &fakeWorkflowRunStore{}
	o.writeWorkflowRunStore = ok
	o.persistWriteWorkflowRun(run) // succeeds; the sticky flag must be flushed

	if ok.last == nil || !ok.last.PersistenceDegraded {
		t.Fatalf("sticky degraded flag must be flushed on the next successful Save, saved=%+v", ok.last)
	}
	if !run.PersistenceDegraded {
		t.Fatal("degraded flag must stay sticky after a later successful Save")
	}
}

// 件2 (pre-apply checkpoint): with a failing store, the transition discloses the
// degraded pre-apply checkpoint BEFORE the irreversible apply and still proceeds
// to dispatch the apply (no hard block). Drives the real apply transition so the
// disclosure call site — not just a helper — is pinned.
func TestRunControllerApplyPlanTransitionDisclosesDegradedPreApplyCheckpoint(t *testing.T) {
	mu := types.NewMutableState("pre-apply degraded")
	plan := &types.ChangePlan{
		ID:          "plan-preapply",
		Status:      types.PlanStatusPending,
		TargetPaths: []string{"pkg/x.go"},
		Changes:     []types.FileChange{{Path: "pkg/x.go", Kind: "patch"}},
	}
	mu.SetChangePlan(plan)
	o := &Orchestrator{busCtx: &types.BusContext{Mutable: mu, WorkDir: t.TempDir(), Mode: types.ModeApply}}
	o.writeWorkflowRunStore = &failingWorkflowRunStore{err: errors.New("disk full")}

	applyCalled := false
	o.controllerWriteStageFn = func(stage types.PipelineStage, stepsUsed *int) (*agent.StageOutput, error) {
		if stage == types.StageApply {
			applyCalled = true
		}
		return nil, fmt.Errorf("apply infra failed")
	}

	run := &types.WriteWorkflowRun{
		RunID:         "wf-preapply",
		Status:        types.WriteWorkflowRunInProgress,
		ActiveBatchID: "batch-1",
		Batches: []types.WriteWorkflowBatch{{
			ID:     "batch-1",
			Status: types.WriteWorkflowBatchPlanned,
			PlanID: "plan-preapply",
		}},
	}
	steps := 0
	if _, err := o.runControllerApplyPlanTransition(run, &steps, "test_apply"); err == nil {
		t.Fatal("expected the injected apply failure to surface")
	}

	if !applyCalled {
		t.Fatal("expected the apply stage to be dispatched after the pre-apply disclosure (no hard block)")
	}
	if !writeWorkflowRunHasProgressReasonForBatch(*run, "batch-1", writeWorkflowPreApplyCheckpointDegradedReason) {
		t.Fatalf("degraded pre-apply checkpoint must be disclosed; progress=%+v", run.ProgressLedger)
	}

	preIdx, applyIdx := -1, -1
	for i, p := range run.ProgressLedger {
		switch p.ReasonCode {
		case writeWorkflowPreApplyCheckpointDegradedReason:
			if preIdx < 0 {
				preIdx = i
			}
		case "apply_failed":
			if applyIdx < 0 {
				applyIdx = i
			}
		}
	}
	if preIdx < 0 {
		t.Fatal("missing pre_apply_checkpoint_degraded progress entry")
	}
	if applyIdx >= 0 && preIdx > applyIdx {
		t.Fatalf("pre-apply disclosure (idx %d) must be recorded before the apply outcome (idx %d)", preIdx, applyIdx)
	}
}

// 件3 (orchestrator guidance): a blocked run with a degraded record carries the
// customer-facing persistence disclosure in the published guidance.
func TestPublishBlockedRunGuidanceDisclosesPersistenceDegraded(t *testing.T) {
	mu := types.NewMutableState("guidance degraded")
	o := &Orchestrator{busCtx: &types.BusContext{Mutable: mu, WorkDir: t.TempDir(), Mode: types.ModeApply, Language: "en"}}
	run := &types.WriteWorkflowRun{
		RunID:                     "wf-guidance",
		Status:                    types.WriteWorkflowRunBlocked,
		ActiveBatchID:             "batch-1",
		PersistenceDegraded:       true,
		PersistenceDegradedReason: types.WriteWorkflowPersistenceDegradedSaveFailed,
		Batches: []types.WriteWorkflowBatch{{
			ID:     "batch-1",
			Status: types.WriteWorkflowBatchBlocked,
		}},
	}
	o.publishBlockedRunGuidance(run, "verify_retry_budget_exhausted")

	result := mu.Result()
	if !strings.Contains(result, "could not be fully saved") {
		t.Fatalf("blocked guidance must disclose persistence degradation, got:\n%s", result)
	}
}

// 件3 negative arm: a healthy run never shows the disclosure in guidance.
func TestPublishBlockedRunGuidanceNoPersistenceNoteWhenHealthy(t *testing.T) {
	mu := types.NewMutableState("guidance healthy")
	o := &Orchestrator{busCtx: &types.BusContext{Mutable: mu, WorkDir: t.TempDir(), Mode: types.ModeApply, Language: "en"}}
	run := &types.WriteWorkflowRun{
		RunID:         "wf-guidance-ok",
		Status:        types.WriteWorkflowRunBlocked,
		ActiveBatchID: "batch-1",
		Batches: []types.WriteWorkflowBatch{{
			ID:     "batch-1",
			Status: types.WriteWorkflowBatchBlocked,
		}},
	}
	o.publishBlockedRunGuidance(run, "verify_retry_budget_exhausted")

	if result := mu.Result(); strings.Contains(result, "could not be fully saved") {
		t.Fatalf("healthy run must not disclose persistence degradation, got:\n%s", result)
	}
}
