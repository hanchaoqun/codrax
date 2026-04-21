package orchestrator

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/types"
)

// TestLogTriageStage_FiresWhenAttachedLogNonEmpty verifies the
// conditional pre-stage dispatches the log_triager agent when the
// user provided an AttachedLog payload. The mock log_triager
// records that it was invoked; any failure to route through
// preStages would leave the invocation counter at zero.
func TestLogTriageStage_FiresWhenAttachedLogNonEmpty(t *testing.T) {
	triagerCalled := 0
	analyzerCalled := 0
	agentFns := map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentLogTriager: func(ctx *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
			triagerCalled++
			// Simulate the real log_triager writing a populated bundle
			// so the analyzer's downstream consumer sees the non-nil
			// handle on Mutable.LogTriage.
			if ctx.Mutable != nil {
				ctx.Mutable.SetLogTriage(&types.LogBundle{
					Meta:       types.LogMeta{Lang: "go"},
					Errors:     []types.LogError{{Type: "panic"}},
					IntentHint: types.IntentRootCause,
					Coverage:   1.0,
				})
			}
			return &agent.StageOutput{StageReport: "triaged"}, nil
		},
		types.AgentAnalyzer: func(_ *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
			analyzerCalled++
			// Return a degenerate stage output that fails the analyze
			// retry budget — we only care about pre-stage ordering and
			// invocation count, not a full end-to-end pipeline.
			return &agent.StageOutput{Error: "stub analyzer fail (expected)"}, nil
		},
	}
	ar, sr, sar := buildRegistries(agentFns)
	o := New(types.PipelineSettings{MaxRetriesPerStage: 1},
		ar, sr, sar)
	o.SetAttachedLog("panic: runtime error\n\ngoroutine 1 [running]:\nmain.crashy()\n\t/src/main.go:42 +0x1e\n")

	_, _ = o.Run("test question", "/tmp/repo", "main")

	if triagerCalled != 1 {
		t.Errorf("log_triager called %d times, want 1", triagerCalled)
	}
	// Analyzer should still be attempted at least once (pre-stage
	// failure is non-fatal, so even if triager returned an error the
	// main pipeline proceeds).
	if analyzerCalled == 0 {
		t.Error("analyzer was never dispatched after log_triage")
	}
}

// TestLogTriageStage_SkippedWhenAttachedLogEmpty confirms the Guard
// short-circuits when no log was attached.
func TestLogTriageStage_SkippedWhenAttachedLogEmpty(t *testing.T) {
	triagerCalled := 0
	analyzerCalled := 0
	agentFns := map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentLogTriager: func(_ *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
			triagerCalled++
			return &agent.StageOutput{}, nil
		},
		types.AgentAnalyzer: func(_ *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
			analyzerCalled++
			return &agent.StageOutput{Error: "stub"}, nil
		},
	}
	ar, sr, sar := buildRegistries(agentFns)
	o := New(types.PipelineSettings{MaxRetriesPerStage: 1},
		ar, sr, sar)
	// No SetAttachedLog — guard should skip.

	_, _ = o.Run("test question", "/tmp/repo", "main")

	if triagerCalled != 0 {
		t.Errorf("log_triager called %d times, want 0 (AttachedLog empty)", triagerCalled)
	}
	if analyzerCalled == 0 {
		t.Error("analyzer not dispatched without log_triage")
	}
}

// TestLogTriageStage_FailureDoesNotBlockMainPipeline verifies that a
// pre-stage returning an error is advisory — the analyzer still fires
// and the Run continues past it.
func TestLogTriageStage_FailureDoesNotBlockMainPipeline(t *testing.T) {
	analyzerCalled := 0
	agentFns := map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentLogTriager: func(_ *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
			return &agent.StageOutput{Error: "simulated triage failure"}, nil
		},
		types.AgentAnalyzer: func(_ *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
			analyzerCalled++
			return &agent.StageOutput{Error: "stub"}, nil
		},
	}
	ar, sr, sar := buildRegistries(agentFns)
	o := New(types.PipelineSettings{MaxRetriesPerStage: 1},
		ar, sr, sar)
	o.SetAttachedLog("panic log")

	_, _ = o.Run("test question", "/tmp/repo", "main")

	if analyzerCalled == 0 {
		t.Error("analyzer MUST still run when pre-stage fails (advisory semantics)")
	}
}

// TestLogTriageStage_BundleFlowsToAnalyzerContext verifies the full
// plumbing: the pre-stage's Mutable.SetLogTriage lands on
// ctx.Mutable.LogTriage() inside the analyzer agent's Execute call.
// This is the integration contract the session-20 refactor depends
// on: analyzer reads bus.LogTriage() instead of running
// deleted logparse bolt-on.
func TestLogTriageStage_BundleFlowsToAnalyzerContext(t *testing.T) {
	var seenBundle *types.LogBundle
	agentFns := map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentLogTriager: func(ctx *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
			if ctx.Mutable != nil {
				ctx.Mutable.SetLogTriage(&types.LogBundle{
					Meta:     types.LogMeta{Lang: "go"},
					Errors:   []types.LogError{{Type: "runtime error"}},
					Entities: []string{"main", "crashy"},
					Coverage: 1.0,
				})
			}
			return &agent.StageOutput{}, nil
		},
		types.AgentAnalyzer: func(ctx *types.AgentContext, _ *skill.Config) (*agent.StageOutput, error) {
			// Read-back assertion: the bundle must be visible here.
			if ctx.Mutable != nil {
				seenBundle = ctx.Mutable.LogTriage()
			}
			return &agent.StageOutput{Error: "stub analyzer fail (expected)"}, nil
		},
	}
	ar, sr, sar := buildRegistries(agentFns)
	o := New(types.PipelineSettings{MaxRetriesPerStage: 1},
		ar, sr, sar)
	o.SetAttachedLog("panic fixture")

	_, _ = o.Run("q", "/tmp/repo", "main")

	if seenBundle == nil {
		t.Fatal("analyzer could not read bus.Mutable.LogTriage() set by pre-stage")
	}
	if seenBundle.Meta.Lang != "go" {
		t.Errorf("bundle Lang lost in plumbing: got %q", seenBundle.Meta.Lang)
	}
	if len(seenBundle.Entities) != 2 {
		t.Errorf("bundle Entities lost: %v", seenBundle.Entities)
	}
}

// TestPreStageDeclaresLogTriage pins the declarative list membership
// so a future refactor that accidentally strips log_triage from
// preStages surfaces as a test failure rather than a silent feature
// regression.
func TestPreStageDeclaresLogTriage(t *testing.T) {
	found := false
	for _, pre := range preStages {
		if pre.Stage == types.StageLogTriage {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("preStages missing StageLogTriage entry")
	}
}

// TestPreStageGuard_LogTriageFiresOnlyWithAttachedLog verifies the
// guard predicate directly — a fresh BusContext without AttachedLog
// must return false; setting the field must flip the guard to true.
// Whitespace-only payloads are treated as empty (TrimSpace) so a
// user's trailing newline does not accidentally trigger a stage for
// no content.
func TestPreStageGuard_LogTriageFiresOnlyWithAttachedLog(t *testing.T) {
	var logTriageGuard func(*types.BusContext) bool
	for _, pre := range preStages {
		if pre.Stage == types.StageLogTriage {
			logTriageGuard = pre.Guard
			break
		}
	}
	if logTriageGuard == nil {
		t.Fatal("no guard found for StageLogTriage")
	}

	bus := &types.BusContext{}
	if logTriageGuard(bus) {
		t.Error("guard fired on empty AttachedLog")
	}
	bus.AttachedLog = "   \n\t  "
	if logTriageGuard(bus) {
		t.Error("guard fired on whitespace-only AttachedLog")
	}
	bus.AttachedLog = "panic: oops"
	if !logTriageGuard(bus) {
		t.Error("guard did not fire on real AttachedLog")
	}
	if logTriageGuard(nil) {
		t.Error("guard fired on nil bus")
	}
}
