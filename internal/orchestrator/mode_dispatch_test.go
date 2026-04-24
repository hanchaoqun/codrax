package orchestrator

import (
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
		RequiredAnswerShape: types.ShapeListOfSymbols,
		Language:            "en",
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
	busCtx, err := o.Run("explain X", "/tmp/repo", "main")
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

// TestMode_PlanReachesStub locks that Mode=ModePlan dispatches
// through runPlanPhase, which in Day 3 writes a recognizable
// B0-skeleton placeholder. This is the smoke-test side of the Day
// 3 -> Day 5 transition: when Day 5 replaces the stub body, this
// test will fail and should be updated to expect real plan output.
func TestMode_PlanReachesStub(t *testing.T) {
	busCtx := readModeRun(t, types.ModePlan)
	if busCtx.Mode != types.ModePlan {
		t.Errorf("Mode should stay ModePlan, got %q", busCtx.Mode)
	}
	result := busCtx.Mutable.Result()
	if !strings.Contains(result, "B0 skeleton") {
		t.Errorf("plan mode Result should contain B0 stub marker; got %q", result)
	}
	if !strings.Contains(result, "plan mode") {
		t.Errorf("plan mode Result should mention plan mode; got %q", result)
	}
	// PipelineStage should reflect the plan stage (the stub sets it).
	if busCtx.PipelineStage != types.StagePlan {
		t.Errorf("PipelineStage should be StagePlan, got %q", busCtx.PipelineStage)
	}
}

// TestMode_ApplyReachesAllStubs locks that Mode=ModeApply walks
// plan → apply → verify sequentially. All three stubs run in order;
// the final PipelineStage reflects the last one (verify).
func TestMode_ApplyReachesAllStubs(t *testing.T) {
	busCtx := readModeRun(t, types.ModeApply)
	if busCtx.Mode != types.ModeApply {
		t.Errorf("Mode should stay ModeApply, got %q", busCtx.Mode)
	}
	// After all three stubs ran, PipelineStage reflects the LAST
	// one (verify) because each stub sets busCtx.PipelineStage
	// to its own stage.
	if busCtx.PipelineStage != types.StageVerify {
		t.Errorf("PipelineStage after apply-mode should be StageVerify (last stub), got %q",
			busCtx.PipelineStage)
	}
	// Plan stub set the Result; apply / verify don't overwrite it.
	result := busCtx.Mutable.Result()
	if !strings.Contains(result, "B0 skeleton") {
		t.Errorf("apply mode Result should carry the plan-stub placeholder; got %q", result)
	}
}

// TestMode_VerifyReachesStub covers the standalone verify path
// (e.g. a rerun against an existing plan). Only runVerifyPhase
// fires; plan and apply are skipped.
func TestMode_VerifyReachesStub(t *testing.T) {
	busCtx := readModeRun(t, types.ModeVerify)
	if busCtx.Mode != types.ModeVerify {
		t.Errorf("Mode should stay ModeVerify, got %q", busCtx.Mode)
	}
	if busCtx.PipelineStage != types.StageVerify {
		t.Errorf("PipelineStage should be StageVerify, got %q", busCtx.PipelineStage)
	}
	// Result stays empty — verify stub doesn't write one. Day 5
	// will populate it from emit_test_results + a rendered
	// ChangeReport.
	if busCtx.Mutable.Result() != "" {
		t.Errorf("verify-only mode Result should be empty in B0 stub; got %q",
			busCtx.Mutable.Result())
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
func TestMode_MainRepoRootPopulated(t *testing.T) {
	busCtx := readModeRun(t, "")
	if busCtx.MainRepoRoot != "/tmp/repo" {
		t.Errorf("MainRepoRoot should equal the Run()'s repoRoot arg; got %q",
			busCtx.MainRepoRoot)
	}
	if busCtx.RepoRoot != busCtx.MainRepoRoot {
		t.Errorf("read-mode RepoRoot and MainRepoRoot should match: %q vs %q",
			busCtx.RepoRoot, busCtx.MainRepoRoot)
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
	if o.Mode() != types.ModePlan {
		t.Errorf("post-SetMode Mode() should be ModePlan, got %q", o.Mode())
	}
	// Setting empty clears it.
	o.SetMode("")
	if o.Mode() != "" {
		t.Errorf("SetMode(\"\") should clear to empty, got %q", o.Mode())
	}
}
