package orchestrator

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/types"
	"github.com/hanchaoqun/codrax/internal/writeflow"
)

type b1703WorkflowStore struct {
	fakeWorkflowRunStore
	load func() *types.WriteWorkflowRun
}

func b1703PlanResultFixture() (*Orchestrator, *writeflow.WriteBatchPlan) {
	mu := types.NewMutableState("bounded follow-up result scope")
	run := &types.WriteWorkflowRun{RunID: "current-run", Status: types.WriteWorkflowRunInProgress, ActiveBatchID: "followup", Batches: []types.WriteWorkflowBatch{
		{ID: "source", PlanID: "source-plan", Status: types.WriteWorkflowBatchComplete, Attempts: []types.WriteWorkflowAttempt{{Kind: "apply", Status: "applied", PlanID: "source-plan"}}},
		{ID: "followup", Purpose: "verification_proof_followup", Status: types.WriteWorkflowBatchReadyToPlan, DependsOn: []string{"source"}},
	}}
	mu.SetWriteWorkflowRun(run)
	o := New(types.PipelineSettings{WriteWorkflowEngine: types.WriteWorkflowEngineController}, nil, nil, nil)
	o.busCtx = &types.BusContext{Mutable: mu, Mode: types.ModeApply, PipelineStage: types.StagePlan, Language: "zh"}
	return o, &writeflow.WriteBatchPlan{ID: "followup"}
}

func TestB1703PlanPostHookScopeBoundariesAndUnchangedError(t *testing.T) {
	type testCase struct {
		name     string
		before   func(*Orchestrator, *writeflow.WriteBatchPlan)
		during   func(*Orchestrator)
		preserve bool
	}
	changeRun := func(edit func(*types.WriteWorkflowRun)) func(*Orchestrator) {
		return func(o *Orchestrator) {
			run := o.busCtx.Mutable.WriteWorkflowRun()
			edit(run)
			o.busCtx.Mutable.SetWriteWorkflowRun(run)
		}
	}
	cases := []testCase{
		{name: "exact_current_followup", preserve: true},
		{name: "impact_and_proof", during: changeRun(func(r *types.WriteWorkflowRun) { r.Batches[1].Purpose = "impact_and_verification_proof_followup" }), preserve: true},
		{name: "impact_only", during: changeRun(func(r *types.WriteWorkflowRun) { r.Batches[1].Purpose = "impact_obligation_followup" }), preserve: true},
		{name: "plan_only", during: func(o *Orchestrator) { o.busCtx.Mode = types.ModePlan }},
		{name: "read_mode", during: func(o *Orchestrator) { o.busCtx.Mode = types.ModeRead }},
		{name: "verify_mode", during: func(o *Orchestrator) { o.busCtx.Mode = types.ModeVerify }},
		{name: "not_plan_stage", during: func(o *Orchestrator) { o.busCtx.PipelineStage = types.StageExplore }},
		{name: "nil_run", before: func(o *Orchestrator, _ *writeflow.WriteBatchPlan) { o.busCtx.Mutable.ResetWriteWorkflowRun() }},
		{name: "empty_run_id", before: func(o *Orchestrator, _ *writeflow.WriteBatchPlan) {
			changeRun(func(r *types.WriteWorkflowRun) { r.RunID = "" })(o)
		}},
		{name: "empty_batch_id", before: func(_ *Orchestrator, b *writeflow.WriteBatchPlan) { b.ID = "" }},
		{name: "wrong_batch", before: func(_ *Orchestrator, b *writeflow.WriteBatchPlan) { b.ID = "source" }},
		{name: "empty_active_batch", before: func(o *Orchestrator, _ *writeflow.WriteBatchPlan) {
			changeRun(func(r *types.WriteWorkflowRun) { r.ActiveBatchID = "" })(o)
		}},
		{name: "run_reset_after_dispatch", during: func(o *Orchestrator) { o.busCtx.Mutable.ResetWriteWorkflowRun() }},
		{name: "run_changed_after_dispatch", during: changeRun(func(r *types.WriteWorkflowRun) { r.RunID = "other-run" })},
		{name: "batch_changed_after_dispatch", during: changeRun(func(r *types.WriteWorkflowRun) { r.ActiveBatchID = "source" })},
		{name: "mutable_changed_after_dispatch", during: func(o *Orchestrator) {
			run := o.busCtx.Mutable.WriteWorkflowRun()
			o.busCtx.Mutable = types.NewMutableState("other owner")
			o.busCtx.Mutable.SetWriteWorkflowRun(run)
		}},
		{name: "nonactive_run", during: changeRun(func(r *types.WriteWorkflowRun) { r.Status = types.WriteWorkflowRunComplete })},
		{name: "blocked_run", during: changeRun(func(r *types.WriteWorkflowRun) { r.Status = types.WriteWorkflowRunBlocked })},
		{name: "missing_active_batch", during: changeRun(func(r *types.WriteWorkflowRun) { r.Batches = r.Batches[:1] })},
		{name: "ordinary_followup", during: changeRun(func(r *types.WriteWorkflowRun) { r.Batches[1].Purpose = "feature" })},
		{name: "nonplanning_followup", during: changeRun(func(r *types.WriteWorkflowRun) { r.Batches[1].Status = types.WriteWorkflowBatchVerifying })},
		{name: "applied_followup", during: changeRun(func(r *types.WriteWorkflowRun) {
			r.Batches[1].Attempts = []types.WriteWorkflowAttempt{{Kind: "apply", Status: "applied", PlanID: "repair-plan"}}
		})},
		{name: "failed_verification", during: changeRun(func(r *types.WriteWorkflowRun) {
			r.Batches[1].Attempts = []types.WriteWorkflowAttempt{{Kind: "verify", Status: "failed", PlanID: "repair-plan"}}
		})},
		{name: "failure_handoff", during: func(o *Orchestrator) {
			o.busCtx.Mutable.SetVerifyFailureHandoff(&types.VerifyFailureHandoff{BatchID: "followup", PlanID: "repair-plan", FailureKind: types.FailureKindTestsFailed})
		}},
		{name: "parent_only_complete_label", during: changeRun(func(r *types.WriteWorkflowRun) { r.Batches[0].Attempts = nil })},
		{name: "parent_apply_failed", during: changeRun(func(r *types.WriteWorkflowRun) { r.Batches[0].Attempts[0].Status = "failed" })},
		{name: "parent_apply_wrong_plan", during: changeRun(func(r *types.WriteWorkflowRun) { r.Batches[0].Attempts[0].PlanID = "unrelated-plan" })},
		{name: "parent_plan_missing", during: changeRun(func(r *types.WriteWorkflowRun) { r.Batches[0].PlanID = "" })},
		{name: "unfinished_parent", during: changeRun(func(r *types.WriteWorkflowRun) { r.Batches[0].Status = types.WriteWorkflowBatchVerifying })},
		{name: "first_plan_no_parent", during: changeRun(func(r *types.WriteWorkflowRun) { r.Batches = r.Batches[1:] })},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			o, batch := b1703PlanResultFixture()
			if tc.before != nil {
				tc.before(o, batch)
			}
			scope := newControllerPlanResultScope(o.busCtx.Mutable, batch)
			const original = "MODEL-owned bytes: 本轮没生成改动方案 is quoted data.\n  Keep this.\n"
			o.controllerWriteStageFn = func(_ types.PipelineStage, _ *int) (*agent.StageOutput, error) {
				if tc.during != nil {
					tc.during(o)
				}
				o.busCtx.Mutable.SetResult(original)
				before := o.busCtx.Mutable.WriteWorkflowRun()
				err := planPostHook(o, nil)
				if err == nil || err.Error() != plannerProseFallbackMessage(o.busCtx) {
					t.Fatalf("plan error/control flow changed: %v", err)
				}
				if !reflect.DeepEqual(before, o.busCtx.Mutable.WriteWorkflowRun()) {
					t.Fatal("result routing mutated workflow/proof state")
				}
				got := o.busCtx.Mutable.Result()
				if tc.preserve {
					if got != original {
						t.Fatalf("qualified prior result changed bytes: %q", got)
					}
				} else if got != plannerProseFallbackMessage(o.busCtx) {
					t.Fatalf("unqualified lane suppressed existing fallback: %q", got)
				}
				return nil, err
			}
			steps := 0
			if _, err := o.runControllerPlanStageInResultScope(scope, &steps); err == nil {
				t.Fatal("dispatch swallowed planning error")
			}
			if o.controllerPlanResultScope != nil {
				t.Fatal("call-stack presentation scope leaked")
			}
		})
	}
}

func TestB1703PlanResultScopeIsCallLocal(t *testing.T) {
	o, batch := b1703PlanResultFixture()
	mu := o.busCtx.Mutable
	const original = "prior model result"
	mu.SetResult(original)
	// An ambient workflow snapshot without a controller dispatch is not enough.
	if err := planPostHook(o, nil); err == nil || mu.Result() != plannerProseFallbackMessage(o.busCtx) {
		t.Fatal("ambient snapshot borrowed scoped suppression")
	}
	if newControllerPlanResultScope(nil, batch) != nil || newControllerPlanResultScope(mu, nil) != nil {
		t.Fatal("missing dispatch input produced a scope")
	}
	scope := newControllerPlanResultScope(mu, batch)
	o.controllerWriteStageFn = func(_ types.PipelineStage, _ *int) (*agent.StageOutput, error) {
		mu.SetResult(original)
		err := planPostHook(o, nil)
		if mu.Result() != original {
			t.Fatal("first scoped dispatch did not preserve bytes")
		}
		return nil, err
	}
	steps := 0
	_, _ = o.runControllerPlanStageInResultScope(scope, &steps)
	mu.SetResult(original)
	_ = planPostHook(o, nil)
	if mu.Result() != plannerProseFallbackMessage(o.busCtx) {
		t.Fatal("completed dispatch retained presentation scope")
	}
	o.controllerWriteStageFn = func(_ types.PipelineStage, _ *int) (*agent.StageOutput, error) { panic("interrupted dispatch") }
	func() {
		defer func() {
			if recover() == nil {
				t.Error("test dispatch did not panic")
			}
		}()
		_, _ = o.runControllerPlanStageInResultScope(scope, &steps)
	}()
	if o.controllerPlanResultScope != nil {
		t.Fatal("panicking dispatch retained presentation scope")
	}
}

func (s *b1703WorkflowStore) FindActiveRun() (*types.WriteWorkflowRun, error) {
	return s.load(), nil
}

// Resume via the public Run entry: real persisted source plan/commit, typed
// completed parent and optional unstarted follow-up; only model responses are
// stubbed. The real StagePlan post-hook and terminal publisher both execute.
func TestB1703PublicRunFollowupRejectionPreservesPriorResult(t *testing.T) {
	for _, prior := range []string{"", "MODEL exact  statement\n\nKeep `identifier` and spacing.", "验证记录：静态检查通过；JavaScript 行为尚未验证。\n"} {
		t.Run(prior, func(t *testing.T) {
			repo := setupGitFixture(t, "source.js", "module.exports = 0\n")
			if err := os.WriteFile(filepath.Join(repo, "source.js"), []byte("module.exports = 1\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			runGitForWorkflowRestoreTest(t, repo, "add", "source.js")
			runGitForWorkflowRestoreTest(t, repo, "commit", "-m", "applied source repair")
			commit := strings.TrimSpace(runGitForWorkflowRestoreTest(t, repo, "rev-parse", "HEAD"))
			artifactDir := t.TempDir()
			now := time.Now()
			plan := &types.ChangePlan{ID: "source-plan", Status: types.PlanStatusApplied, AppliedAt: &now, AppliedCommitSHA: commit, TargetPaths: []string{"source.js"}, Changes: []types.FileChange{{Path: "source.js", Kind: "modify", NewContent: "module.exports = 1\n"}}}
			if err := types.WritePlanToFile(plan, filepath.Join(artifactDir, plan.ID+".json")); err != nil {
				t.Fatal(err)
			}
			report := &types.ChangeReport{PlanID: plan.ID, Channel: types.ChangeReportChannelPostApplyVerify, Passed: true, TestResults: []types.TestResult{{Kind: types.TestResultKindUnit, AssertionID: "static-check", ObservationScope: types.TestObservationScopeAggregate, Passed: true}}}
			if err := types.WriteChangeReportToFile(report, filepath.Join(artifactDir, plan.ID+".report.json")); err != nil {
				t.Fatal(err)
			}
			run := types.WriteWorkflowRun{RunID: "wf-b1703", Status: types.WriteWorkflowRunInProgress, ActiveBatchID: "proof-followup", Batches: []types.WriteWorkflowBatch{
				{ID: "source-batch", PlanID: plan.ID, Status: types.WriteWorkflowBatchComplete, ApplyRef: commit, VerifyRef: plan.ID + ".report.json", Completion: &types.WriteWorkflowCompletion{Verdict: types.WriteWorkflowCompletionVerified}, Attempts: []types.WriteWorkflowAttempt{{Kind: "apply", Status: "applied", PlanID: plan.ID, ArtifactRef: commit}, {Kind: "verify", Status: "passed", PlanID: plan.ID, ReportID: plan.ID + ".report.json"}}},
				{ID: "proof-followup", Purpose: "verification_proof_followup", DependsOn: []string{"source-batch"}, Status: types.WriteWorkflowBatchReadyToPlan, ExpectedPaths: []string{"source.js"}},
			}}
			planCalls := 0
			ar, sr, sar := buildRegistries(map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
				types.AgentAnalyzer: dagAnalyzerFn(dagIR(types.AnswerContract{Language: "en"})),
				types.AgentWriteController: func(ctx *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
					ctx.Mutable.SetResult(prior)
					decision := writeflow.WriteWorkflowDecision{Action: writeflow.ActionPlanBatch, Batch: &writeflow.WriteBatchPlan{ID: "proof-followup", Goal: "obtain missing runtime proof", Purpose: "verification_proof_followup", ExpectedPaths: []string{"source.js"}, DependsOn: []string{"source-batch"}}}
					raw, _ := json.Marshal(decision)
					ctx.Mutable.SetWriteWorkflowDecisionJSON(raw)
					return &agent.StageOutput{Data: raw}, nil
				},
				types.AgentPlanner: func(ctx *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
					planCalls++
					return &agent.StageOutput{Error: "structured verification proposal rejected"}, nil
				},
			})
			o := New(types.PipelineSettings{WriteWorkflowEngine: types.WriteWorkflowEngineController}, ar, sr, sar)
			o.SetMode(types.ModeApply)
			o.SetMaxSteps(15)
			o.SetLanguage("zh")
			o.reportDir = artifactDir
			store := &b1703WorkflowStore{fakeWorkflowRunStore: fakeWorkflowRunStore{savePath: filepath.Join(artifactDir, "workflow.json")}}
			store.load = func() *types.WriteWorkflowRun {
				identity := o.currentWriteWorkflowRepoIdentity(o.currentWriteWorkflowGoalHashSource())
				run.Identity = &identity
				copy := types.CloneWriteWorkflowRun(run)
				return &copy
			}
			o.writeWorkflowRunStore = store
			bus, err := o.Run("finish the already-applied repair", repo, "main")
			if err != nil {
				t.Fatalf("public Run: %v", err)
			}
			if planCalls == 0 {
				t.Fatal("public fixture bypassed planner post-hook")
			}
			got := bus.Mutable.Result()
			for _, wrong := range []string{"本轮没生成改动方案", "把目标说具体", "/mode auto", "No actionable change plan"} {
				if strings.Contains(got, wrong) {
					t.Errorf("follow-up failure erased delivery context with first-plan advice %q: %s", wrong, got)
				}
			}
			if prior != "" && !strings.HasPrefix(got, strings.TrimRight(prior, "\n")) {
				t.Errorf("prior model/verify result changed: %q", got)
			}
			if !strings.Contains(got, "未完全验证") || !strings.Contains(got, "交付内容已保留") {
				t.Errorf("missing typed terminal boundary: %s", got)
			}
			finalRun := bus.Mutable.WriteWorkflowRun()
			if finalRun == nil || finalRun.Completion == nil || finalRun.Completion.Verdict != types.WriteWorkflowCompletionUnverified || finalRun.Completion.ReasonCode != "plan_batch_followup_unverified" {
				t.Fatalf("terminal authority changed: %+v", finalRun)
			}
			if actual := strings.TrimSpace(runGitForWorkflowRestoreTest(t, repo, "rev-parse", "HEAD")); actual != commit {
				t.Fatal("message path changed delivered source")
			}
		})
	}
}
