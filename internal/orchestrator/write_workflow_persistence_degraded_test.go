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

// FIX-1 (completion-face disclosure — single point): setWriteWorkflowCompletion-
// Result is the one seam every completion terminal routes through. On a degraded
// run it appends the customer-facing persistence caveat after the base result,
// so the note rides the IMMEDIATE completion output (the CLI single-shot bus
// result and the REPL completion turn card), not only a later /workflow show.
func TestSetWriteWorkflowCompletionResultDisclosesPersistenceDegraded(t *testing.T) {
	mu := types.NewMutableState("completion caveat")
	o := &Orchestrator{busCtx: &types.BusContext{Mutable: mu, WorkDir: t.TempDir(), Mode: types.ModeApply}}
	run := types.WriteWorkflowRun{
		RunID:                     "wf-complete-degraded",
		Status:                    types.WriteWorkflowRunComplete,
		PersistenceDegraded:       true,
		PersistenceDegradedReason: types.WriteWorkflowPersistenceDegradedSaveFailed,
	}
	o.setWriteWorkflowCompletionResult(run, "write workflow complete")

	result := mu.Result()
	if !strings.HasPrefix(result, "write workflow complete") {
		t.Fatalf("completion result must keep its base prefix, got: %q", result)
	}
	if !strings.Contains(result, "could not be fully saved") {
		t.Fatalf("degraded completion result must disclose the persistence note, got: %q", result)
	}
}

// FIX-1 negative arm (direct): a healthy run leaves the completion result string
// byte-for-byte unchanged — the persistence lane is orthogonal and never touches
// the happy-path completion output.
func TestSetWriteWorkflowCompletionResultHealthyCompletionUnchanged(t *testing.T) {
	mu := types.NewMutableState("completion healthy")
	o := &Orchestrator{busCtx: &types.BusContext{Mutable: mu, WorkDir: t.TempDir(), Mode: types.ModeApply}}
	run := types.WriteWorkflowRun{RunID: "wf-complete-ok", Status: types.WriteWorkflowRunComplete}
	o.setWriteWorkflowCompletionResult(run, "write workflow complete")

	if result := mu.Result(); result != "write workflow complete" {
		t.Fatalf("healthy completion result must be byte-unchanged, got: %q", result)
	}
}

// FIX-1 (real completion path): a budget-exhausted terminal whose durable record
// degraded during the terminal persist still discloses the note on its immediate
// completion result. Drives the production completeBudgetExhaustedRunIfAllBatches-
// Complete helper (not just the seam) with a failing store so the whole path —
// persist degrades -> result built -> disclosure appended -> bus result set — is
// pinned. The bus result set here is the shared CLI single-shot + REPL turn
// output, so this covers both surfaces.
func TestCompleteBudgetExhaustedRunDisclosesPersistenceOnCompletion(t *testing.T) {
	mu := types.NewMutableState("budget degraded")
	o := &Orchestrator{busCtx: &types.BusContext{Mutable: mu, WorkDir: t.TempDir(), Mode: types.ModeApply}}
	o.writeWorkflowRunStore = &failingWorkflowRunStore{err: errors.New("disk full")}

	run := &types.WriteWorkflowRun{
		RunID:         "wf-budget-degraded",
		Status:        types.WriteWorkflowRunInProgress,
		ActiveBatchID: "batch-1",
		Batches: []types.WriteWorkflowBatch{{
			ID:     "batch-1",
			Status: types.WriteWorkflowBatchComplete,
			Completion: &types.WriteWorkflowCompletion{
				Verdict:    types.WriteWorkflowCompletionVerified,
				ReasonCode: "all_tests_pass",
			},
		}},
	}
	if !o.completeBudgetExhaustedRunIfAllBatchesComplete(run, "controller_turn_budget_all_batches_complete") {
		t.Fatal("all batches complete must terminalize the run")
	}
	result := mu.Result()
	if !strings.HasPrefix(result, "write workflow complete") {
		t.Fatalf("completion result must keep its base prefix, got: %q", result)
	}
	if !strings.Contains(result, "could not be fully saved") {
		t.Fatalf("degraded budget-exhausted completion must disclose the persistence note, got: %q", result)
	}
}

// FIX-1 negative arm (real completion path): the same helper with a healthy
// successful store publishes the typed terminal verdict without a persistence
// caveat. Batch-local verify cards are not the workflow completion authority.
func TestCompleteBudgetExhaustedRunHealthyCompletionPublishesTerminalVerdict(t *testing.T) {
	mu := types.NewMutableState("budget healthy")
	o := &Orchestrator{busCtx: &types.BusContext{Mutable: mu, WorkDir: t.TempDir(), Mode: types.ModeApply}}
	o.writeWorkflowRunStore = &fakeWorkflowRunStore{}

	run := &types.WriteWorkflowRun{
		RunID:         "wf-budget-ok",
		Status:        types.WriteWorkflowRunInProgress,
		ActiveBatchID: "batch-1",
		Batches: []types.WriteWorkflowBatch{{
			ID:     "batch-1",
			Status: types.WriteWorkflowBatchComplete,
			Completion: &types.WriteWorkflowCompletion{
				Verdict:    types.WriteWorkflowCompletionVerified,
				ReasonCode: "all_tests_pass",
			},
		}},
	}
	if !o.completeBudgetExhaustedRunIfAllBatchesComplete(run, "controller_turn_budget_all_batches_complete") {
		t.Fatal("all batches complete must terminalize the run")
	}
	result := mu.Result()
	if !strings.HasPrefix(result, "write workflow complete") ||
		!strings.Contains(result, "最终交付状态：已验证") ||
		strings.Contains(result, "could not be fully saved") {
		t.Fatalf("healthy completion must publish the typed final verdict only, got: %q", result)
	}
}

func TestPublishWriteWorkflowCompletionResultAppendsUnverifiedAfterPassedCard(t *testing.T) {
	mu := types.NewMutableState("terminal truth after local pass")
	mu.SetResult("## 测试通过\n\n1 个测试通过。\n")
	o := &Orchestrator{busCtx: &types.BusContext{Mutable: mu, WorkDir: t.TempDir(), Mode: types.ModeApply, Language: "zh"}}
	run := types.WriteWorkflowRun{
		RunID:         "wf-proof-incomplete",
		Status:        types.WriteWorkflowRunComplete,
		ActiveBatchID: "batch-1-cumulative-review",
		Batches: []types.WriteWorkflowBatch{{
			ID:     "batch-1",
			Status: types.WriteWorkflowBatchComplete,
			Completion: &types.WriteWorkflowCompletion{
				Verdict:    types.WriteWorkflowCompletionVerified,
				ReasonCode: "tests_passed",
			},
		}, {
			ID:     "batch-1-cumulative-review",
			Status: types.WriteWorkflowBatchComplete,
			Completion: &types.WriteWorkflowCompletion{
				Verdict:    types.WriteWorkflowCompletionUnverified,
				ReasonCode: "verification_proof_incomplete",
			},
		}},
		Completion: &types.WriteWorkflowCompletion{
			Verdict:    types.WriteWorkflowCompletionUnverified,
			ReasonCode: "verification_proof_incomplete",
		},
	}

	o.publishWriteWorkflowCompletionResult(run, "write workflow complete")

	result := mu.Result()
	for _, want := range []string{
		"## 测试通过",
		"## 最终交付状态：未完全验证",
		"`batch-1-cumulative-review`（行为或影响证明尚未闭合）",
		"测试结果可能已经通过，但声明的行为或影响证明仍未完全闭合",
	} {
		if !strings.Contains(result, want) {
			t.Fatalf("terminal completion result missing %q:\n%s", want, result)
		}
	}
	if strings.LastIndex(result, "最终交付状态") < strings.LastIndex(result, "测试通过") {
		t.Fatalf("terminal verdict must be the last authority card:\n%s", result)
	}
}

func TestRenderWriteWorkflowTerminalStatusEnglishRunnerUnavailable(t *testing.T) {
	run := types.WriteWorkflowRun{
		Status: types.WriteWorkflowRunComplete,
		Batches: []types.WriteWorkflowBatch{{
			ID:     "batch-2",
			Status: types.WriteWorkflowBatchComplete,
			Completion: &types.WriteWorkflowCompletion{
				Verdict:    types.WriteWorkflowCompletionUnverified,
				ReasonCode: "runner_missing",
			},
		}},
		Completion: &types.WriteWorkflowCompletion{
			Verdict:    types.WriteWorkflowCompletionUnverified,
			ReasonCode: "runner_missing",
		},
	}

	got := renderWriteWorkflowTerminalStatus(run, "en")
	for _, want := range []string{
		"Final delivery status: unverified",
		"`batch-2` (the local test runner was unavailable)",
		"test runner, dependencies, or result parser were unavailable",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("English terminal status missing %q:\n%s", want, got)
		}
	}
}

func TestRenderWriteWorkflowTerminalStatusLocalizesReasonCodesWithoutLeakingProtocolTokens(t *testing.T) {
	run := types.WriteWorkflowRun{
		Status: types.WriteWorkflowRunComplete,
		Batches: []types.WriteWorkflowBatch{
			{ID: "batch-1", Status: types.WriteWorkflowBatchComplete, Completion: &types.WriteWorkflowCompletion{
				Verdict: types.WriteWorkflowCompletionUnverified, ReasonCode: "production_verification_source_static_only",
			}},
			{ID: "batch-future", Status: types.WriteWorkflowBatchComplete, Completion: &types.WriteWorkflowCompletion{
				Verdict: types.WriteWorkflowCompletionUnverified, ReasonCode: "future_internal_reason_code",
			}},
		},
		Completion: &types.WriteWorkflowCompletion{
			Verdict: types.WriteWorkflowCompletionUnverified, ReasonCode: "production_verification_source_static_only",
		},
	}

	for _, tc := range []struct {
		lang string
		want string
	}{
		{lang: "zh", want: "生产验证目前只有静态证据"},
		{lang: "en", want: "production verification currently has static evidence only"},
	} {
		got := renderWriteWorkflowTerminalStatus(run, tc.lang)
		if !strings.Contains(got, tc.want) {
			t.Fatalf("localized terminal status missing %q:\n%s", tc.want, got)
		}
		for _, leaked := range []string{
			"production_verification_source_static_only",
			"future_internal_reason_code",
		} {
			if strings.Contains(got, leaked) {
				t.Fatalf("terminal status leaked internal reason code %q:\n%s", leaked, got)
			}
		}
	}
}

func TestRenderWriteWorkflowTerminalStatusScopesVerifiedClaim(t *testing.T) {
	run := types.WriteWorkflowRun{
		Status: types.WriteWorkflowRunComplete,
		Completion: &types.WriteWorkflowCompletion{
			Verdict: types.WriteWorkflowCompletionVerified, ReasonCode: "all_batches_verified",
		},
	}
	zh := renderWriteWorkflowTerminalStatus(run, "zh")
	for _, want := range []string{"必需的结构化验证义务均已闭合", "不表示其中每一项都获得了独立执行证据"} {
		if !strings.Contains(zh, want) {
			t.Fatalf("Chinese verified status lost scoped claim %q:\n%s", want, zh)
		}
	}
	en := renderWriteWorkflowTerminalStatus(run, "en")
	for _, want := range []string{"required typed verification obligations are closed", "does not claim independent execution evidence"} {
		if !strings.Contains(en, want) {
			t.Fatalf("English verified status lost scoped claim %q:\n%s", want, en)
		}
	}
	for _, leaked := range []string{"all_batches_verified", "all_verified"} {
		if strings.Contains(zh, leaked) || strings.Contains(en, leaked) {
			t.Fatalf("verified status leaked internal enum %q: zh=%q en=%q", leaked, zh, en)
		}
	}
}

func TestRenderWriteWorkflowTerminalStatusHidesVerdictProtocolTokens(t *testing.T) {
	for _, tc := range []struct {
		verdict types.WriteWorkflowCompletionVerdict
		leaked  string
	}{
		{verdict: types.WriteWorkflowCompletionVerified, leaked: "`verified`"},
		{verdict: types.WriteWorkflowCompletionAcceptedFailed, leaked: "`accepted_failed`"},
	} {
		run := types.WriteWorkflowRun{
			Status:     types.WriteWorkflowRunComplete,
			Completion: &types.WriteWorkflowCompletion{Verdict: tc.verdict, ReasonCode: "verification_failed"},
		}
		got := renderWriteWorkflowTerminalStatus(run, "zh")
		if strings.Contains(got, tc.leaked) || strings.Contains(got, "`verification_failed`") {
			t.Fatalf("terminal status leaked internal verdict or reason token:\n%s", got)
		}
	}
}
