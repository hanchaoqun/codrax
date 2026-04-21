package agent

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/types"
)

// TestLogTriager_SkippedWhenDisabled verifies the stage short-circuits
// when log_triage_enabled=false.
func TestLogTriager_SkippedWhenDisabled(t *testing.T) {
	agent := &logTriager{
		base:     &BaseAgent{name: types.AgentLogTriager, deps: &Dependencies{}},
		settings: LogTriageSettings{Enabled: false, MinBytes: 50, MaxLLMCalls: 8},
	}
	ctx := &types.AgentContext{
		AttachedLog: "panic: ...",
		Mutable:     types.NewMutableState("t"),
	}
	out, err := agent.Execute(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if out == nil || !strings.Contains(out.StageReport, "log_triage_enabled=false") {
		t.Errorf("expected disabled skip, got %+v", out)
	}
	if ctx.Mutable.LogTriage() != nil {
		t.Error("bundle should stay nil when disabled")
	}
}

// TestLogTriager_SkippedWhenAttachedLogEmpty verifies the gate when
// the user did not attach a log.
func TestLogTriager_SkippedWhenAttachedLogEmpty(t *testing.T) {
	agent := &logTriager{
		base:     &BaseAgent{name: types.AgentLogTriager, deps: &Dependencies{}},
		settings: DefaultLogTriageSettings(),
	}
	ctx := &types.AgentContext{
		AttachedLog: "",
		Mutable:     types.NewMutableState("t"),
	}
	out, err := agent.Execute(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if out == nil || !strings.Contains(out.StageReport, "AttachedLog empty") {
		t.Errorf("expected AttachedLog-empty skip, got %+v", out)
	}
}

// TestLogTriager_SkippedWhenBelowMinBytes confirms the size floor.
func TestLogTriager_SkippedWhenBelowMinBytes(t *testing.T) {
	agent := &logTriager{
		base:     &BaseAgent{name: types.AgentLogTriager, deps: &Dependencies{}},
		settings: LogTriageSettings{Enabled: true, MinBytes: 100, MaxLLMCalls: 8},
	}
	ctx := &types.AgentContext{
		AttachedLog: "short",
		Mutable:     types.NewMutableState("t"),
	}
	out, err := agent.Execute(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if out == nil || !strings.Contains(out.StageReport, "below min") {
		t.Errorf("expected min-bytes skip, got %+v", out)
	}
}

// TestLogTriager_ShouldEscalate_NilBundleTrue covers the escalation
// predicate — a nil bundle always requires escalation.
func TestLogTriager_ShouldEscalate_NilBundleTrue(t *testing.T) {
	agent := &logTriager{settings: DefaultLogTriageSettings()}
	if !agent.shouldEscalate(nil) {
		t.Error("nil bundle should escalate")
	}
}

// TestLogTriager_ShouldEscalate_HighCoverageFalse verifies that a
// bundle above the coverage threshold stays single-shot.
func TestLogTriager_ShouldEscalate_HighCoverageFalse(t *testing.T) {
	agent := &logTriager{settings: LogTriageSettings{TwoStepCoverage: 0.3}}
	b := &types.LogBundle{Coverage: 0.9}
	if agent.shouldEscalate(b) {
		t.Error("high coverage should NOT escalate")
	}
}

// TestLogTriager_ShouldEscalate_LowCoverageTrue covers the symmetric
// case — bundle below threshold triggers escalation.
func TestLogTriager_ShouldEscalate_LowCoverageTrue(t *testing.T) {
	agent := &logTriager{settings: LogTriageSettings{TwoStepCoverage: 0.3}}
	b := &types.LogBundle{Coverage: 0.1}
	if !agent.shouldEscalate(b) {
		t.Error("low coverage should escalate")
	}
}

// TestLogTriagerEvaluator_ParseOutput_BundlePresent_Success covers the
// happy path where the emit tool already populated Mutable.LogTriage.
func TestLogTriagerEvaluator_ParseOutput_BundlePresent_Success(t *testing.T) {
	eval := &logTriagerEvaluator{settings: DefaultLogTriageSettings()}
	ctx := &types.AgentContext{Mutable: types.NewMutableState("t")}
	ctx.Mutable.SetLogTriage(&types.LogBundle{
		Meta:          types.LogMeta{Lang: "go", Signals: []types.LogSignal{types.SignalPanic}},
		Errors:        []types.LogError{{Type: "runtime error"}},
		ResolvedFiles: []string{"main.go"},
		Entities:      []string{"main", "crashyFunc"},
		IntentHint:    types.IntentRootCause,
		Coverage:      0.95,
	})

	out, err := eval.ParseOutput(ctx, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if out.Error != "" {
		t.Errorf("unexpected error: %s", out.Error)
	}
	if out.StageReport == "" {
		t.Error("expected StageReport populated")
	}
	// Report should mention language + signal + error count.
	if !strings.Contains(out.StageReport, "Lang: go") ||
		!strings.Contains(out.StageReport, "Signals: panic") ||
		!strings.Contains(out.StageReport, "Errors: 1") {
		t.Errorf("report lacks key content:\n%s", out.StageReport)
	}
}

// TestLogTriagerEvaluator_ParseOutput_NoEmit_FailLoud verifies fail-
// loud when neither emit_log_triage nor emit_log_segmentation was
// called (ReAct loop exited without emit).
func TestLogTriagerEvaluator_ParseOutput_NoEmit_FailLoud(t *testing.T) {
	eval := &logTriagerEvaluator{settings: DefaultLogTriageSettings()}
	ctx := &types.AgentContext{Mutable: types.NewMutableState("t")}

	out, err := eval.ParseOutput(ctx, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if out.Error == "" {
		t.Error("expected fail-loud Error on no-emit")
	}
	if !strings.Contains(out.Error, "did not call emit_log_triage") {
		t.Errorf("error message not informative: %s", out.Error)
	}
}

// TestLogTriagerEvaluator_ParseOutput_RejectedEmit_CarriesReason
// verifies the StageOutput.Error carries the rejection reason when
// the emit tool rejected the payload.
func TestLogTriagerEvaluator_ParseOutput_RejectedEmit_CarriesReason(t *testing.T) {
	eval := &logTriagerEvaluator{settings: DefaultLogTriageSettings()}
	ctx := &types.AgentContext{Mutable: types.NewMutableState("t")}
	// Simulate an emit_log_triage call that rejected.
	results := []types.ToolResult{
		{ToolName: "emit_log_triage", Success: false,
			Summary: "emit_log_triage rejected: empty bundle"},
	}

	out, err := eval.ParseOutput(ctx, nil, results, nil)
	if err != nil {
		t.Fatal(err)
	}
	if out.Error == "" {
		t.Error("expected Error set")
	}
	if !strings.Contains(out.Error, "empty bundle") {
		t.Errorf("rejection reason lost: %s", out.Error)
	}
}

// TestLogTriagerEvaluator_Observe_SuccessfulEmitFlipsFlag covers the
// stop detection path: a successful emit_log_triage mid-loop flips
// emitSeen and returns a StopRequested signal.
func TestLogTriagerEvaluator_Observe_SuccessfulEmitFlipsFlag(t *testing.T) {
	eval := &logTriagerEvaluator{settings: DefaultLogTriageSettings()}
	obs := LoopObservation{
		Phase: PhaseMidLoop,
		LastToolResult: &types.ToolResult{
			ToolName: "emit_log_triage",
			Success:  true,
		},
	}
	sig := eval.Observe(nil, obs)
	if !sig.StopRequested {
		t.Error("expected StopRequested on successful emit")
	}
	if !eval.emitSeen {
		t.Error("emitSeen flag not set")
	}
}

// TestLogTriagerEvaluator_Observe_FailedEmitDoesNotStop verifies the
// loop continues on a failed emit so the LLM can retry within the
// same dispatch.
func TestLogTriagerEvaluator_Observe_FailedEmitDoesNotStop(t *testing.T) {
	eval := &logTriagerEvaluator{settings: DefaultLogTriageSettings()}
	obs := LoopObservation{
		Phase: PhaseMidLoop,
		LastToolResult: &types.ToolResult{
			ToolName: "emit_log_triage",
			Success:  false,
		},
	}
	sig := eval.Observe(nil, obs)
	if sig.StopRequested {
		t.Error("failed emit must NOT stop the loop (LLM needs to retry)")
	}
}

// TestLogTriagerEvaluator_ShouldStop_FiresAfterEmitSeen pins the
// ReAct terminator: once emitSeen is true, ShouldStop returns true.
func TestLogTriagerEvaluator_ShouldStop_FiresAfterEmitSeen(t *testing.T) {
	eval := &logTriagerEvaluator{settings: DefaultLogTriageSettings()}
	if eval.ShouldStop(llmResponseStub(), 0) {
		t.Error("should NOT stop before emit")
	}
	eval.emitSeen = true
	if !eval.ShouldStop(llmResponseStub(), 0) {
		t.Error("should stop after emit")
	}
}

// TestNewLogTriagerAgent_CapsMaxIterations confirms the constructor
// caps MaxIterations at 6 to prevent runaway loops.
func TestNewLogTriagerAgent_CapsMaxIterations(t *testing.T) {
	deps := &Dependencies{MaxIterations: 100}
	a := NewLogTriagerAgent(deps, DefaultLogTriageSettings()).(*logTriager)
	if a.base.deps.MaxIterations > 6 {
		t.Errorf("MaxIterations = %d, cap is 6", a.base.deps.MaxIterations)
	}
}

// TestNewLogTriagerAgent_ZeroSettingsInheritsDefaults verifies the
// zero-value shortcut uses DefaultLogTriageSettings.
func TestNewLogTriagerAgent_ZeroSettingsInheritsDefaults(t *testing.T) {
	a := NewLogTriagerAgent(&Dependencies{}, LogTriageSettings{}).(*logTriager)
	if !a.settings.Enabled {
		t.Error("zero settings should inherit Enabled=true")
	}
	if a.settings.MinBytes != 50 {
		t.Errorf("MinBytes = %d, want 50", a.settings.MinBytes)
	}
	if a.settings.TwoStepBytes != 32*1024 {
		t.Errorf("TwoStepBytes = %d, want 32768", a.settings.TwoStepBytes)
	}
}

// TestLogTriager_LookupSkillSafe_NoRegistry verifies the defensive
// error path when Skills is nil.
func TestLogTriager_LookupSkillSafe_NoRegistry(t *testing.T) {
	base := &BaseAgent{deps: &Dependencies{}}
	_, err := lookupSkillSafe(base, "log-segmentation-skill")
	if err == nil {
		t.Error("expected error when Skills registry not wired")
	}
}

// TestLogTriager_LookupSkillSafe_WithRegistry verifies happy path.
func TestLogTriager_LookupSkillSafe_WithRegistry(t *testing.T) {
	reg := skill.NewRegistry()
	skill.RegisterDefaults(reg)
	base := &BaseAgent{deps: &Dependencies{Skills: reg}}
	cfg, err := lookupSkillSafe(base, "log-segmentation-skill")
	if err != nil {
		t.Fatal(err)
	}
	if cfg == nil {
		t.Fatal("nil skill config")
	}
	if cfg.Name != "log-segmentation-skill" {
		t.Errorf("wrong skill: %s", cfg.Name)
	}
}

// TestIsExtractableSegment covers the Step B filter predicate.
func TestIsExtractableSegment(t *testing.T) {
	cases := map[string]bool{
		"stack":     true,
		"caused_by": true,
		"trace":     true,
		"header":    false,
		"context":   false,
		"noise":     false,
	}
	for kind, want := range cases {
		if got := isExtractableSegment(kind); got != want {
			t.Errorf("isExtractableSegment(%q) = %v, want %v", kind, got, want)
		}
	}
}

// TestRenderLogTriageStageReport_IncludesCauseChain verifies the
// report renderer walks recursive Cause chains.
func TestRenderLogTriageStageReport_IncludesCauseChain(t *testing.T) {
	// Build a 3-level chain: root → A → B.
	b := &types.LogError{
		Type: "RootException",
		Cause: &types.LogError{
			Type: "LevelA",
			Cause: &types.LogError{
				Type: "LevelB",
			},
		},
	}
	bundle := &types.LogBundle{
		Meta:     types.LogMeta{Lang: "java"},
		Errors:   []types.LogError{*b},
		Coverage: 1.0,
	}
	got := renderLogTriageStageReport(bundle)
	for _, want := range []string{"RootException", "caused by LevelA", "caused by LevelB"} {
		if !strings.Contains(got, want) {
			t.Errorf("report missing %q:\n%s", want, got)
		}
	}
}

// TestRenderLogTriageStageReport_NilBundle returns clean placeholder.
func TestRenderLogTriageStageReport_NilBundle(t *testing.T) {
	got := renderLogTriageStageReport(nil)
	if got == "" {
		t.Error("expected non-empty placeholder")
	}
}

// llmResponseStub returns a minimal llm.Response for tests that don't
// care about its content — ShouldStop ignores it anyway.
func llmResponseStub() llm.Response { return llm.Response{} }
