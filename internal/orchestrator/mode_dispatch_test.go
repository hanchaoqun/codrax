package orchestrator

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/types"
)

// ---------------------------------------------------------------------------
// Test fixture: a minimal read-mode setup that still exercises the
// Phase 2 runTaskPhase path. Reuses dagIR from orchestrator_dag_test.go
// so the happy-path read flow is the SAME shape the session-33 T1
// byte-identity test compares against.
// ---------------------------------------------------------------------------

// readModeRun is the canonical Run() invocation for the mode tests.
// Returns the post-Run BusContext so callers can inspect Mutable
// Result, TaskState, and Mode normalization.
//
// mode is passed to SetMode only when non-empty; passing "" models
// the pre-B0 caller who never calls SetMode. The returned BusContext
// MUST be compared only on non-timestamp fields — TraceID is
// time.Now().UnixNano()-derived and changes every Run.
func readModeRun(t *testing.T, mode types.PipelineMode) *types.BusContext {
	t.Helper()
	ir := dagIR(types.AnswerContract{
		Language: "en",
	})
	agentFns := map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentAnalyzer: dagAnalyzerFn(ir),
		types.AgentExplorer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			return &agent.StageOutput{MissingPiece: types.MissingFacts}, nil
		},
		types.AgentFinalizer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			return &agent.StageOutput{
				MissingPiece: types.MissingNone,
				FinalAnswer:  "- `Foo` (file.go:1)",
			}, nil
		},
	}
	ar, sr, sar := buildRegistries(agentFns)
	o := New(types.PipelineSettings{}, ar, sr, sar)
	o.SetMaxSteps(20)
	if mode != "" {
		o.SetMode(mode)
	}
	// plan/apply modes hit planPreHook which calls DetectRepoState;
	// authorize the bare-dir gate so the gate falls through to the
	// rest of the pipeline.
	o.SetAutoInitRepo(true)
	o.SetScaffoldEnabled(true)
	// t.TempDir() is cross-platform (Windows / macOS / Linux); avoid
	// hardcoding /tmp paths. Drop a sentinel file so the empty-repo
	// short-circuit (read mode against an effectively-empty dir
	// returns an intro message instead of dispatching the pipeline)
	// does NOT fire — these tests want the dispatch path.
	repoRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(repoRoot, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("seed sentinel file: %v", err)
	}
	busCtx, err := o.Run("explain X", repoRoot, "main")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	return busCtx
}

// TestMode_DefaultIsRead covers the zero-value path: a caller that
// never invokes SetMode reaches Run() with o.mode == "", which
// Normalize() coerces to ModeRead. The L1 red line depends on this
// coercion being lossless.
func TestMode_DefaultIsRead(t *testing.T) {
	busCtx := readModeRun(t, "")
	if busCtx.Mode != types.ModeRead {
		t.Errorf("zero-value Mode should Normalize to %q, got %q",
			types.ModeRead, busCtx.Mode)
	}
	if !busCtx.TaskState.IsTerminal {
		t.Error("Run should mark TaskState.IsTerminal")
	}
	// Real answer (from the finalizer mock), not a B0 stub placeholder.
	result := busCtx.Mutable.Result()
	if !strings.Contains(result, "Foo") {
		t.Errorf("expected finalizer answer in Result; got %q", result)
	}
	if strings.Contains(result, "B0 skeleton") {
		t.Errorf("read-mode Result must not contain B0 stub placeholder; got %q", result)
	}
}

// TestRunMode_ReadByteIdentical is the session-33 T1. Locks the L1
// red line: a caller using Mode="" and a caller using Mode=ModeRead
// MUST produce equivalent BusContext output. Non-timestamp fields
// are compared pairwise; TraceID is excluded because it embeds
// UnixNano (different across two Runs by definition).
//
// If this test fails, session-33 Day 3 broke read-mode byte identity
// and the L1 red line is violated.
func TestRunMode_ReadByteIdentical(t *testing.T) {
	baseline := readModeRun(t, "")
	explicit := readModeRun(t, types.ModeRead)

	// Both must have coerced to ModeRead (one via Normalize, one via
	// explicit SetMode).
	if baseline.Mode != explicit.Mode {
		t.Errorf("Mode mismatch: baseline=%q explicit=%q", baseline.Mode, explicit.Mode)
	}
	if baseline.Mode != types.ModeRead {
		t.Errorf("Mode should be ModeRead, got %q", baseline.Mode)
	}

	// Mutable.Result MUST match exactly — same stub answer,
	// no mode-dependent text.
	if baseline.Mutable.Result() != explicit.Mutable.Result() {
		t.Errorf("Result differs across Mode=\"\" vs ModeRead:\n  baseline: %q\n  explicit: %q",
			baseline.Mutable.Result(), explicit.Mutable.Result())
	}

	// Terminal state + error paths must match.
	if baseline.TaskState.IsTerminal != explicit.TaskState.IsTerminal {
		t.Errorf("IsTerminal mismatch: baseline=%v explicit=%v",
			baseline.TaskState.IsTerminal, explicit.TaskState.IsTerminal)
	}
	if baseline.TaskState.LastError != explicit.TaskState.LastError {
		t.Errorf("LastError mismatch:\n  baseline: %q\n  explicit: %q",
			baseline.TaskState.LastError, explicit.TaskState.LastError)
	}

	// Per-Run identifiers differ by design (TraceID is UnixNano-based)
	// so we do NOT compare them. We also skip Branch / RepoRoot /
	// MainRepoRoot / Language because they're set from the same
	// constant inputs in both runs — equality is trivially preserved.

	// Tool results / evidence / chains / symbols — each should be
	// equal in length (content may differ trivially in pointer
	// identities but the stub agents emit nothing so both are zero).
	if len(baseline.ToolResults) != len(explicit.ToolResults) {
		t.Errorf("ToolResults length mismatch: %d vs %d",
			len(baseline.ToolResults), len(explicit.ToolResults))
	}
	if len(baseline.EvidenceItems) != len(explicit.EvidenceItems) {
		t.Errorf("EvidenceItems length mismatch: %d vs %d",
			len(baseline.EvidenceItems), len(explicit.EvidenceItems))
	}
}

// TestMode_PlanReachesDispatch locks that Mode=ModePlan dispatches
// through runPlanPhase, which in Day 5 calls dispatchStage(StagePlan).
// In this test no AgentPlanner stub is wired (buildRegistries only
// covers the 5 read-mode agents + 3 write-mode stubs registered with
// nil execFn), so the mock planner returns a zero-value StageOutput
// with no ChangePlan on Mutable. runPlanPhase then surfaces a
// fail-loud "no ChangePlan was installed" error into LastError.
//
// The invariant this test locks is that dispatch REACHES the plan
// stage. Real planner output quality is covered by the dedicated
// planner_test.go / plan_mode_e2e_test.go files in Day 5.
func TestMode_PlanReachesDispatch(t *testing.T) {
	busCtx := readModeRun(t, types.ModePlan)
	if busCtx.Mode != types.ModePlan {
		t.Errorf("Mode should stay ModePlan, got %q", busCtx.Mode)
	}
	// PipelineStage reflects the plan stage (runPlanPhase sets it).
	if busCtx.PipelineStage != types.StagePlan {
		t.Errorf("PipelineStage should be StagePlan, got %q", busCtx.PipelineStage)
	}
	// With no planner stub wiring a ChangePlan, runPlanPhase
	// surfaces an error message to Result + LastError.
	result := busCtx.Mutable.Result()
	// Wording was simplified post-2026-04-30 to drop internal
	// terminology. Match either locale of the new message.
	if !strings.Contains(result, "change plan") && !strings.Contains(result, "改动方案") {
		t.Errorf("plan mode Result should describe why no plan was produced; got %q", result)
	}
	if busCtx.TaskState.LastError == "" {
		t.Errorf("plan mode without a ChangePlan should populate LastError")
	}
}

// TestMode_ApplyReachesDispatch locks that Mode=ModeApply walks
// plan → apply → verify in order. The apply and verify stages are
// implemented as fail-loud stubs (coderEvaluator / verifierEvaluator)
// — each returns a StageOutput.Error describing the B0 stub state.
// The plan phase fails first (no planner stub), so run aborts
// before apply fires and PipelineStage stays at Plan.
func TestMode_ApplyReachesDispatch(t *testing.T) {
	busCtx := readModeRun(t, types.ModeApply)
	if busCtx.Mode != types.ModeApply {
		t.Errorf("Mode should stay ModeApply, got %q", busCtx.Mode)
	}
	// Apply mode runs plan first; plan fails (no ChangePlan), so
	// the switch short-circuits before apply+verify. PipelineStage
	// stays at StagePlan. This is the correct "fail-loud" semantic.
	if busCtx.PipelineStage != types.StagePlan {
		t.Errorf("PipelineStage after apply-mode should be StagePlan (plan failed first), got %q",
			busCtx.PipelineStage)
	}
	if busCtx.TaskState.LastError == "" {
		t.Errorf("apply mode without a ChangePlan should populate LastError")
	}
}

// TestMode_VerifyReachesStub covers the standalone verify path
// (e.g. a rerun against an existing plan). Only runVerifyPhase
// fires; the B0 verifier stub returns a fail-loud error that
// surfaces as LastError.
//
// Because mockAgent does NOT route through verifierEvaluator (the
// mock returns its execFn's StageOutput directly, bypassing the
// agent's Evaluator), this test wires a mock-verifier that mimics
// the real stub — returning a StageOutput.Error with the B0
// skeleton marker. This keeps the test isolated from LLM state
// while still locking the runVerifyPhase → StageOutput.Error →
// LastError path.
func TestMode_VerifyReachesStub(t *testing.T) {
	ir := dagIR(types.AnswerContract{Language: "en"})
	agentFns := map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentAnalyzer: dagAnalyzerFn(ir),
		types.AgentVerifier: func(_ *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
			return &agent.StageOutput{
				MissingPiece: types.MissingFacts,
				Error:        "[B0 skeleton] verify stage stub — mimicking verifierEvaluator output for test isolation",
			}, nil
		},
	}
	ar, sr, sar := buildRegistries(agentFns)
	o := New(types.PipelineSettings{}, ar, sr, sar)
	o.SetMaxSteps(20)
	o.SetMode(types.ModeVerify)
	o.SetAutoInitRepo(true) // verify hits stage hooks; tmp dir is bare
	o.SetScaffoldEnabled(true)
	busCtx, err := o.Run("rerun verify", t.TempDir(), "main")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if busCtx.Mode != types.ModeVerify {
		t.Errorf("Mode should stay ModeVerify, got %q", busCtx.Mode)
	}
	if busCtx.PipelineStage != types.StageVerify {
		t.Errorf("PipelineStage should be StageVerify, got %q", busCtx.PipelineStage)
	}
	result := busCtx.Mutable.Result()
	if !strings.Contains(result, "B0 skeleton") && !strings.Contains(result, "verify") {
		t.Errorf("verify mode Result should describe stub state; got %q", result)
	}
}

// TestMode_UnknownRejected locks the defensive default branch in
// the Mode switch. An invalid mode (one that Normalize() passes
// through unchanged because it is non-empty) is surfaced as
// TaskState.LastError rather than silently falling through to read.
func TestMode_UnknownRejected(t *testing.T) {
	busCtx := readModeRun(t, types.PipelineMode("bogus"))
	if busCtx.Mode != types.PipelineMode("bogus") {
		t.Errorf("Mode should stay bogus (Normalize is pass-through for non-empty), got %q",
			busCtx.Mode)
	}
	if busCtx.TaskState.LastError == "" {
		t.Error("unknown mode should set TaskState.LastError")
	}
	if !strings.Contains(busCtx.TaskState.LastError, "bogus") {
		t.Errorf("LastError should mention the bogus mode; got %q",
			busCtx.TaskState.LastError)
	}
}

// TestMode_MainRepoRootPopulated pins the Day 2/3 contract: every
// Run populates BusContext.MainRepoRoot with the user-supplied
// repoRoot, even in read mode (where the field is unused). This
// lets the worktree-cleanup defer access the canonical repo path
// regardless of whether the apply stage swapped RepoRoot.
//
// Cross-platform note: uses a self-managed temp dir (instead of the
// shared readModeRun helper) so the assertion can compare against
// the exact path passed to Run on every OS.
func TestMode_MainRepoRootPopulated(t *testing.T) {
	ir := dagIR(types.AnswerContract{Language: "en"})
	agentFns := map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentAnalyzer: dagAnalyzerFn(ir),
		types.AgentExplorer: func(_ *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
			return &agent.StageOutput{MissingPiece: types.MissingFacts}, nil
		},
		types.AgentFinalizer: func(_ *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
			return &agent.StageOutput{MissingPiece: types.MissingNone, FinalAnswer: "- `Foo` (file.go:1)"}, nil
		},
	}
	ar, sr, sar := buildRegistries(agentFns)
	o := New(types.PipelineSettings{}, ar, sr, sar)
	o.SetMaxSteps(20)
	repoRoot := t.TempDir()
	// Drop a sentinel file so the read-mode empty-repo short-circuit
	// does not pre-empt the dispatch — this test asserts the
	// MainRepoRoot population invariant which requires the pipeline
	// to actually start.
	if err := os.WriteFile(filepath.Join(repoRoot, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("seed sentinel file: %v", err)
	}
	busCtx, err := o.Run("explain X", repoRoot, "main")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if busCtx.MainRepoRoot != repoRoot {
		t.Errorf("MainRepoRoot should equal the Run()'s repoRoot arg; got %q want %q",
			busCtx.MainRepoRoot, repoRoot)
	}
	if busCtx.RepoRoot != busCtx.MainRepoRoot {
		t.Errorf("read-mode RepoRoot and MainRepoRoot should match: %q vs %q",
			busCtx.RepoRoot, busCtx.MainRepoRoot)
	}
}

// TestMode_ApplyWithPlanPathSkipsPlanPhase pins the B1.5 invariant
// that `ModeApply` with a pre-supplied PlanPath skips runPlanPhase
// entirely. Without this skip, /approve (REPL) and single-shot
// `--mode=write --write-phase=apply --plan-file=<path>` would re-dispatch the planner
// and overwrite the reviewed plan with a fresh emission.
//
// Signal: a planner mock that marks the test failed if invoked,
// combined with an assertion that PipelineStage advances past
// StagePlan (here: StageApply, failing because no worktreeBase is
// configured — that is the next-phase error we WANT to see).
func TestMode_ApplyWithPlanPathSkipsPlanPhase(t *testing.T) {
	// Write a dummy plan file to disk so runApplyPhase's substep-1
	// loader succeeds.
	planDir := t.TempDir()
	planPath := planDir + "/plan-skip-test.json"
	plan := &types.ChangePlan{
		ID:          "plan-skip-test",
		Summary:     "plan-skip invariant test",
		Status:      "pending_approval",
		Changes:     []types.FileChange{{Path: "main.go", Kind: "modify", NewContent: "x"}},
		TargetPaths: []string{"main.go"},
	}
	data, err := json.Marshal(plan)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	if err := os.WriteFile(planPath, data, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	ir := dagIR(types.AnswerContract{Language: "en"})
	plannerCalled := false
	agentFns := map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentAnalyzer: dagAnalyzerFn(ir),
		types.AgentPlanner: func(_ *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
			plannerCalled = true
			return &agent.StageOutput{MissingPiece: types.MissingNone}, nil
		},
	}
	ar, sr, sar := buildRegistries(agentFns)
	o := New(types.PipelineSettings{}, ar, sr, sar)
	o.SetMaxSteps(20)
	o.SetMode(types.ModeApply)
	o.SetAutoInitRepo(true) // plan/apply stage gate; tests run against tmp dirs
	o.SetScaffoldEnabled(true)
	o.SetPlanPath(planPath)
	// Intentionally omit SetWorktreeBase — apply phase will fail
	// with "worktree base not configured" which is the signal we
	// reached StageApply without going through plan.

	// Use the same synthesized phrasing handleApproveCmd produces;
	// passing the bare slash form would trip the orchestrator's
	// REPL-control-input fail-loud guard.
	busCtx, err := o.Run("Apply approved plan plan-skip-test", t.TempDir(), "main")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if plannerCalled {
		t.Fatal("planner agent must NOT be dispatched when PlanPath is pre-set")
	}
	if busCtx.PipelineStage != types.StageApply {
		t.Errorf("PipelineStage should advance past plan (stage=apply); got %q",
			busCtx.PipelineStage)
	}
	if !strings.Contains(busCtx.TaskState.LastError, "worktree") {
		t.Errorf("apply phase should fail on missing worktree base; got LastError=%q",
			busCtx.TaskState.LastError)
	}
}

// TestOrchestrator_ModeGetter verifies SetMode / Mode() round-trip
// and that the empty zero-value is returned when SetMode was never
// called.
func TestOrchestrator_ModeGetter(t *testing.T) {
	ar, sr, sar := buildRegistries(nil)
	o := New(types.PipelineSettings{}, ar, sr, sar)
	if o.Mode() != "" {
		t.Errorf("pre-SetMode Mode() should be empty, got %q", o.Mode())
	}
	o.SetMode(types.ModePlan)
	o.SetAutoInitRepo(true) // plan stage's new bare-dir gate; tests run against tmp dirs
	o.SetScaffoldEnabled(true)
	if o.Mode() != types.ModePlan {
		t.Errorf("post-SetMode Mode() should be ModePlan, got %q", o.Mode())
	}
	// Setting empty clears it.
	o.SetMode("")
	if o.Mode() != "" {
		t.Errorf("SetMode(\"\") should clear to empty, got %q", o.Mode())
	}
}
