package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// TestAnalyzerParseOutputCapturesSummary verifies that the analyzer's
// ParseOutput packs the LLM's free-form text into Data (under "result")
// alongside the structured classification fields derived from an
// emit_analysis-emitted RequestModel.
func TestAnalyzerParseOutputCapturesSummary(t *testing.T) {
	e := &analyzerEvaluator{}

	// Pretend a previous emit_analysis call already populated the
	// RequestModel carrier. Under v3 the analyzer reads its hints
	// from MutableState.RequestModel(), not a separate carrier.
	mut := types.NewMutableState("explain the project")
	mut.SetRequestModel(types.RequestModel{
		Intent:     types.IntentExplain,
		Scenario:   types.ScenarioArchitectureExplain,
		Complexity: types.ComplexityModerate,
		AnalyzerHints: types.AnalyzerHints{
			Kind:     "mechanism",
			Keywords: []string{"orchestrator", "agent", "pipeline"},
			Entities: []string{"Orchestrator", "BaseAgent"},
		},
	})

	ctx := &types.AgentContext{
		Stage:   types.StageAnalyze,
		Mutable: mut,
	}
	msgs := []llm.Message{
		{Role: "assistant", Content: "Classified as mechanism: ordered steps."},
	}

	out, err := e.ParseOutput(ctx, msgs, []types.ToolResult{emitResult(true)}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var data map[string]any
	if err := json.Unmarshal(out.Data, &data); err != nil {
		t.Fatalf("Data not valid JSON: %v", err)
	}
	if s, _ := data["result"].(string); s == "" {
		t.Error("Data.result should contain the assistant's summary")
	}
	if got, _ := data["question_kind"].(string); got != "mechanism" {
		t.Errorf("Data.question_kind = %q, want mechanism", got)
	}
	// JSON numbers decode as float64 in map[string]any.
	if got, _ := data["entity_count"].(float64); got != 2 {
		t.Errorf("Data.entity_count = %v, want 2", got)
	}
	if got, _ := data["keyword_count"].(float64); got != 3 {
		t.Errorf("Data.keyword_count = %v, want 3", got)
	}

	// Mutable.Objective must be unchanged — analyzer should not clobber
	// what the bootstrap or a previous dispatch put there.
	if obj := mut.Objective(); obj != "explain the project" {
		t.Errorf("Mutable.Objective was clobbered, got %q", obj)
	}
	if rm := mut.RequestModel(); rm == nil || rm.AnalyzerHints.Kind != "mechanism" {
		t.Errorf("RequestModel was clobbered, got %+v", rm)
	}
}

func TestAnalyzerGraphForNormalize_SkipsEagerLoadForRuntimeArtifactOptionalSource(t *testing.T) {
	repo := t.TempDir()
	writeAnalyzerGraphFixture(t, repo)
	ctx := &types.AgentContext{
		Stage:           types.StageAnalyze,
		RepoRoot:        repo,
		AttachedHitrace: "sched_switch prev=app next=worker",
		Mutable:         types.NewMutableState("attached trace"),
	}
	rm := types.RequestModel{
		Intent:   types.IntentTrace,
		Scenario: types.ScenarioPerformanceBottleneck,
		AnalyzerHints: types.AnalyzerHints{
			Entities: []string{"Widget"},
		},
	}
	if got := analyzerGraphForNormalize(ctx, rm); got != nil {
		t.Fatalf("runtime artifact with optional current-source lane should not eager-load analyzer source graph, got %+v", got.Metadata)
	}
	if ctx.Mutable.SearchGraph() != nil {
		t.Fatal("optional-source runtime artifact must not publish a search graph during analyze post-processing")
	}
}

func TestAnalyzerGraphForNormalize_SourceTurnStillLoadsGraph(t *testing.T) {
	repo := t.TempDir()
	writeAnalyzerGraphFixture(t, repo)
	ctx := &types.AgentContext{
		Stage:    types.StageAnalyze,
		RepoRoot: repo,
		Mutable:  types.NewMutableState("explain Widget"),
	}
	rm := types.RequestModel{
		Intent:   types.IntentExplain,
		Scenario: types.ScenarioArchitectureExplain,
		AnalyzerHints: types.AnalyzerHints{
			Entities: []string{"Widget"},
		},
	}
	got := analyzerGraphForNormalize(ctx, rm)
	if got == nil {
		t.Fatal("ordinary source turn should keep analyzer source graph loading")
	}
	if ctx.Mutable.SearchGraph() == nil {
		t.Fatal("ordinary source graph load should publish the reusable search graph")
	}
}

func TestAnalyzerGraphForNormalize_CurrentSourceDimensionReopensRuntimeArtifactGraph(t *testing.T) {
	repo := t.TempDir()
	writeAnalyzerGraphFixture(t, repo)
	ctx := &types.AgentContext{
		Stage:           types.StageAnalyze,
		RepoRoot:        repo,
		AttachedHitrace: "sched_switch prev=app next=worker",
		Mutable:         types.NewMutableState("attached trace plus current implementation"),
	}
	rm := types.RequestModel{
		Intent:   types.IntentRootCause,
		Scenario: types.ScenarioPerformanceBottleneck,
		SourceScopeProfile: &types.SourceScopeProfile{
			RequestedScope: types.SourceScopeProduction,
			Confidence:     0.9,
		},
		RequestedAnswerDimensions: &types.RequestedAnswerDimensionProfile{
			IsDimensionedAnswer: true,
			Dimensions: []types.RequestedAnswerDimension{{
				Role:     types.RequestedAnswerDimensionCurrentKeyCode,
				Required: true,
				Index:    1,
			}},
			Confidence: 0.9,
		},
		AnalyzerHints: types.AnalyzerHints{
			Entities: []string{"Widget"},
		},
	}
	if got := analyzerGraphForNormalize(ctx, rm); got == nil {
		t.Fatal("typed current-source dimension should reopen analyzer source graph loading")
	}
}

func writeAnalyzerGraphFixture(t *testing.T, repo string) {
	t.Helper()
	path := filepath.Join(repo, "widget.go")
	if err := os.WriteFile(path, []byte("package fixture\n\ntype Widget struct{}\n"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
}

// TestAnalyzerParseOutputFailLoud_ZeroEmit verifies the fail-loud
// contract: when the LLM never calls emit_analysis, ParseOutput
// returns a StageOutput with a populated Error and a nil
// AnalysisIR so the orchestrator's runAnalyzePhase retries.
func TestAnalyzerParseOutputFailLoud_ZeroEmit(t *testing.T) {
	e := &analyzerEvaluator{}

	mut := types.NewMutableState("what does this do?")
	ctx := &types.AgentContext{
		Stage:     types.StageAnalyze,
		Objective: "what does this do?",
		Mutable:   mut,
	}
	msgs := []llm.Message{
		{Role: "assistant", Content: "I'm thinking about it..."},
	}

	out, err := e.ParseOutput(ctx, msgs, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out == nil {
		t.Fatal("expected non-nil output")
	}
	if out.Error == "" {
		t.Fatal("expected populated Error on 0-emit fail-loud path")
	}
	if out.AnalysisIR != nil {
		t.Error("expected nil AnalysisIR on 0-emit path")
	}
}

// TestAnalyzerParseOutputNoMutable verifies the evaluator does not
// crash if Mutable is nil (defensive — production always has it set,
// but tests should not).
func TestAnalyzerParseOutputNoMutable(t *testing.T) {
	e := &analyzerEvaluator{}
	ctx := &types.AgentContext{Stage: types.StageAnalyze}
	msgs := []llm.Message{
		{Role: "assistant", Content: "ok"},
	}
	out, err := e.ParseOutput(ctx, msgs, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out == nil {
		t.Fatal("expected non-nil output")
	}
}

// -----------------------------------------------------------------------------
// Deterministic post-processing pipeline (AnalysisIR construction)
// -----------------------------------------------------------------------------

// TestAnalyzer_IRIsBuiltFromTaskItem validates the analyzer's
// deterministic post-processing pipeline: given a populated
// TaskItem and raw objective, ParseOutput must produce a full
// AnalysisIR whose RequestModel, TaskGraph, and QualityGate are
// all consistent with the inputs.
func TestAnalyzer_IRIsBuiltFromRequestModel(t *testing.T) {
	mut := types.NewMutableState("请解释 explorer 是如何决定 ShouldStop 的")
	mut.SetRequestModel(types.RequestModel{
		Intent:     types.IntentExplain,
		Scenario:   types.ScenarioArchitectureExplain,
		Complexity: types.ComplexityModerate,
		AnalyzerHints: types.AnalyzerHints{
			Keywords: []string{"explorer", "ShouldStop", "explore"},
			Entities: []string{"Explorer", "ShouldStop"},
			Kind:     "mechanism",
		},
	})
	ctx := &types.AgentContext{
		Objective: "请解释 explorer 是如何决定 ShouldStop 的",
		Mutable:   mut,
	}

	eval := &analyzerEvaluator{}
	out, err := eval.ParseOutput(ctx, []llm.Message{
		{Role: "assistant", Content: "classification complete"},
	}, []types.ToolResult{emitResult(true)}, nil)
	if err != nil {
		t.Fatalf("ParseOutput: %v", err)
	}
	if out.AnalysisIR == nil {
		t.Fatal("expected non-nil AnalysisIR")
	}

	ir := out.AnalysisIR
	if ir.Version != types.AnalysisIRVersion {
		t.Errorf("version: got %q want %q", ir.Version, types.AnalysisIRVersion)
	}
	if ir.RequestModel.Language != "zh" {
		t.Errorf("language: got %q want zh", ir.RequestModel.Language)
	}
	if ir.RequestModel.Intent != types.IntentExplain {
		t.Errorf("intent: got %q want explain (mechanism)", ir.RequestModel.Intent)
	}
	// Rule 6 (mechanism + 2 entities → complex) fires here because
	// the fixture has entities=["Explorer","ShouldStop"] + kind=mechanism.
	// Before Rule 6, this was moderate; after, it reconciles to complex.
	if ir.RequestModel.Complexity != types.ComplexityComplex {
		t.Errorf("complexity: got %q want complex (mechanism + 2 entities)", ir.RequestModel.Complexity)
	}
	if len(ir.TaskGraph.Nodes) < 3 {
		t.Errorf("expected ≥3 task nodes from architecture_explain template; got %d", len(ir.TaskGraph.Nodes))
	}
	if len(ir.HypothesisSet) == 0 {
		t.Errorf("hypothesis planner must never produce an empty set")
	}
	// Normalizer should have found both terms. "ShouldStop" is
	// CamelCase so it lands on code:shouldstop; lowercase "explorer"
	// is a concept word without a symbol resolver, so it lands on
	// en:explorer. Both are valid canonicalisations — the explorer
	// at runtime will resolve en:explorer to code:explorer via its
	// SymbolResolver hook.
	foundExplorer := false
	foundShouldStop := false
	for _, c := range ir.RequestModel.TermGraph.Canonical {
		if c.ID == "en:explorer" || c.ID == "code:explorer" {
			foundExplorer = true
		}
		if c.ID == "code:shouldstop" {
			foundShouldStop = true
		}
	}
	if !foundExplorer || !foundShouldStop {
		t.Errorf("term graph missing expected canonical terms; got %+v", ir.RequestModel.TermGraph.Canonical)
	}
	// Gate result is reported, not enforced. Whatever it says, the IR
	// must carry the report so downstream analytics can diff it.
	if len(ir.QualityGate.Checks) == 0 {
		t.Errorf("quality gate must always report checks")
	}
}

// (TestAnalyzer_FailsafeTaskInstallsIR deleted: fail-loud contract
// no longer synthesizes an IR when emit_analysis was not called.)

// TestAnalyzer_StageOutputCarriesIR validates the propagation path:
// StageOutput.AnalysisIR produced by the analyzer must survive the
// applyStageOutput step and end up on BusContext.AnalysisIR. This
// catches wiring regressions between the analyzer and orchestrator
// without needing a full LLM run.
func TestAnalyzer_StageOutputCarriesIR(t *testing.T) {
	mut := types.NewMutableState("explain the finalizer's translation mode")
	mut.SetRequestModel(types.RequestModel{
		Intent:     types.IntentExplain,
		Scenario:   types.ScenarioArchitectureExplain,
		Complexity: types.ComplexityModerate,
	})
	ctx := &types.AgentContext{
		Objective: "explain the finalizer's translation mode",
		Mutable:   mut,
	}
	eval := &analyzerEvaluator{}
	out, err := eval.ParseOutput(ctx, nil, []types.ToolResult{emitResult(true)}, nil)
	if err != nil {
		t.Fatalf("ParseOutput: %v", err)
	}
	if out.AnalysisIR == nil {
		t.Fatal("StageOutput.AnalysisIR must be non-nil")
	}
}

// TestAnalyzer_BuildInitialInstruction_IsEmpty pins the Skill/Evaluator
// boundary contract documented on analyzerEvaluator.BuildInitialInstruction:
// every static input the analyze stage needs (field enums, rules,
// workflow) is owned by the analysis-skill and rendered as system
// sections by context.BuildPromptContext, and every dynamic input it
// needs (user request, language preference, retry directive) is
// rendered as user sections by the same builder. The evaluator's
// per-dispatch supplement slot is therefore intentionally empty. A
// future commit that re-seeds static prompt text here — drifting the
// contract away from the SSOT in internal/skill/analysis_contract.go —
// must fail this test.
func TestAnalyzer_BuildInitialInstruction_IsEmpty(t *testing.T) {
	eval := &analyzerEvaluator{}

	cases := []struct {
		name string
		ctx  *types.AgentContext
	}{
		{"nil-ctx", nil},
		{"empty-ctx", &types.AgentContext{}},
		{
			"fully-populated-ctx",
			&types.AgentContext{
				Objective:    "trace how the orchestrator dispatches the analyze stage",
				MissingPiece: types.MissingFacts,
				Preferences:  []string{"Respond to the user in Simplified Chinese (zh)."},
				RetryHint:    "previous analyze attempt produced a nil IR; try again",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := eval.BuildInitialInstruction(tc.ctx, nil); got != "" {
				t.Errorf("analyzer BuildInitialInstruction must stay empty "+
					"(Skill/Evaluator boundary contract); got %d bytes:\n%s",
					len(got), got)
			}
		})
	}
}

// TestAnalyzer_BuildInitialInstruction_RetryDirective verifies that
// the "TERMINAL FORCING" directive is emitted when EmitStageRetryAttempt
// > 0 (terminal forcing on retry). The directive must include the
// retry-attempt counter, the literal tool name, and a strong NO-prose
// instruction so a model that produced text-only on the prior attempt
// is funneled toward the structured emit on the next.
func TestAnalyzer_BuildInitialInstruction_RetryDirective(t *testing.T) {
	eval := &analyzerEvaluator{}

	// Attempt 0 — happy path, no directive even with non-nil ctx.
	got0 := eval.BuildInitialInstruction(&types.AgentContext{
		Stage:                 types.StageAnalyze,
		EmitStageRetryAttempt: 0,
	}, nil)
	if got0 != "" {
		t.Errorf("attempt 0: directive should NOT fire, got %d bytes:\n%s", len(got0), got0)
	}

	// Attempt 1 — directive must fire and include key markers.
	got1 := eval.BuildInitialInstruction(&types.AgentContext{
		Stage:                 types.StageAnalyze,
		EmitStageRetryAttempt: 1,
		Language:              "en",
	}, nil)
	required := []string{
		"TERMINAL FORCING",
		"Retry attempt 1",
		"emit_analysis",
		"fail loud",
	}
	for _, marker := range required {
		if !strings.Contains(got1, marker) {
			t.Errorf("attempt 1 directive missing marker %q in:\n%s", marker, got1)
		}
	}

	// Attempt 2 — counter reflects real attempt number.
	got2 := eval.BuildInitialInstruction(&types.AgentContext{
		Stage:                 types.StageAnalyze,
		EmitStageRetryAttempt: 2,
		Language:              "en",
	}, nil)
	if !strings.Contains(got2, "Retry attempt 2") {
		t.Errorf("attempt 2 directive should report counter 2:\n%s", got2)
	}

	gotZh := eval.BuildInitialInstruction(&types.AgentContext{
		Stage:                 types.StageAnalyze,
		EmitStageRetryAttempt: 1,
		Language:              "zh",
	}, nil)
	for _, marker := range []string{"终止式重试约束", "第 1 次重试", "emit_analysis", "不要输出额外散文"} {
		if !strings.Contains(gotZh, marker) {
			t.Errorf("zh retry directive missing marker %q in:\n%s", marker, gotZh)
		}
	}
}

// -----------------------------------------------------------------------------
// emit_analysis call-count gate — 0 / 1 / N tests
// -----------------------------------------------------------------------------
//
// The analyzer's ParseOutput counts emit_analysis invocations in the
// dispatch's tool-result stream and branches on 0 / 1 / >1. These
// tests pin the three branches plus the strict-policy knob so a
// future prompt change or validator refactor cannot silently restore
// the old "prompt says once, runtime accepts anything" behavior.
//
// Each test restores tool.CurrentAnalysisLimits on cleanup so the
// strict-policy sub-test cannot leak into sibling tests.

// emitResult constructs a ToolResult entry with ToolName=emit_analysis
// and the given success flag. The ParseOutput counter is
// success-agnostic — every attempt counts toward the exactly-once
// contract — so the helper makes that symmetry easy to exercise.
func emitResult(success bool) types.ToolResult {
	return types.ToolResult{
		ToolName: "emit_analysis",
		Success:  success,
		Summary:  "test-emit",
	}
}

// parseWithToolResults runs analyzerEvaluator.ParseOutput against a
// synthetic context + tool-result stream and returns the decoded
// StageOutput.Data map plus the StageOutput itself for error checks.
func parseWithToolResults(t *testing.T, mu *types.MutableState, results []types.ToolResult) (map[string]any, *StageOutput) {
	t.Helper()
	ctx := &types.AgentContext{
		Stage:     types.StageAnalyze,
		Objective: "pin the call-count gate",
		Mutable:   mu,
	}
	out, err := (&analyzerEvaluator{}).ParseOutput(ctx, nil, results, nil)
	if err != nil {
		t.Fatalf("ParseOutput: %v", err)
	}
	if out == nil {
		t.Fatal("ParseOutput returned nil StageOutput")
	}
	var decoded map[string]any
	if err := json.Unmarshal(out.Data, &decoded); err != nil {
		t.Fatalf("StageOutput.Data is not valid JSON: %v", err)
	}
	return decoded, out
}

// getFloat is a tiny helper — the encoding/json unmarshal turns every
// numeric field into float64, so the individual tests would otherwise
// need a cast+check dance for each assertion.
func getFloat(t *testing.T, m map[string]any, key string) float64 {
	t.Helper()
	v, ok := m[key]
	if !ok {
		t.Fatalf("StageOutput.Data missing key %q; keys=%v", key, mapKeys(m))
	}
	f, ok := v.(float64)
	if !ok {
		t.Fatalf("StageOutput.Data[%q] = %T(%v), want float64", key, v, v)
	}
	return f
}

func getBool(t *testing.T, m map[string]any, key string) bool {
	t.Helper()
	v, ok := m[key]
	if !ok {
		t.Fatalf("StageOutput.Data missing key %q; keys=%v", key, mapKeys(m))
	}
	b, ok := v.(bool)
	if !ok {
		t.Fatalf("StageOutput.Data[%q] = %T(%v), want bool", key, v, v)
	}
	return b
}

func mapKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// restoreAnalysisLimits snapshots the current limits and installs a
// cleanup hook that restores them. Every test that mutates the
// package-global policy must call this so cross-test pollution is
// impossible.
func restoreAnalysisLimits(t *testing.T) {
	t.Helper()
	prev := tool.CurrentAnalysisLimits()
	t.Cleanup(func() { tool.SetAnalysisLimits(prev) })
}

// TestAnalyzer_CallCountGate_ZeroCalls pins the fail-loud 0-call
// branch: ParseOutput returns a StageOutput with Error populated
// and AnalysisIR nil so runAnalyzePhase can retry. There is no
// "failsafe" synthesis anymore.
func TestAnalyzer_CallCountGate_ZeroCalls(t *testing.T) {
	restoreAnalysisLimits(t)
	tool.SetAnalysisLimits(tool.AnalysisLimits{RejectMultipleEmit: true})

	mu := types.NewMutableState("explain the analyzer 0-call path")

	data, out := parseWithToolResults(t, mu, nil)

	if got := getFloat(t, data, "analysis_emit_calls"); got != 0 {
		t.Errorf("analysis_emit_calls = %v, want 0", got)
	}
	if out.Error == "" {
		t.Error("0-call branch must set StageOutput.Error on fail-loud")
	}
	if out.AnalysisIR != nil {
		t.Error("0-call branch must NOT produce an AnalysisIR")
	}
}

// TestAnalyzer_CallCountGate_SingleCall pins the happy path: one
// emit_analysis call before ParseOutput, fallback_used must be false,
// count must be 1, no Error, and the emitted RequestModel's fields
// must survive into the IR.
func TestAnalyzer_CallCountGate_SingleCall(t *testing.T) {
	restoreAnalysisLimits(t)

	mu := types.NewMutableState("explain the analyzer 1-call path")
	mu.SetRequestModel(types.RequestModel{
		Intent:     types.IntentExplain,
		Scenario:   types.ScenarioArchitectureExplain,
		Complexity: types.ComplexityModerate,
		AnalyzerHints: types.AnalyzerHints{
			Keywords: []string{"a", "b", "c", "d", "e", "f", "g", "h"},
			Entities: []string{"Orchestrator"},
			Kind:     "mechanism",
		},
	})

	data, out := parseWithToolResults(t, mu, []types.ToolResult{emitResult(true)})

	if got := getFloat(t, data, "analysis_emit_calls"); got != 1 {
		t.Errorf("analysis_emit_calls = %v, want 1", got)
	}
	if out.Error != "" {
		t.Errorf("single-call branch must not set StageOutput.Error, got %q", out.Error)
	}
	if out.AnalysisIR == nil {
		t.Fatal("single-call branch must produce an AnalysisIR")
	}
	if out.AnalysisIR.RequestModel.Intent != types.IntentExplain {
		t.Errorf("IR lost the LLM's Intent: got %q", out.AnalysisIR.RequestModel.Intent)
	}
}

// TestAnalyzer_CallCountGate_MultipleCalls_WarnAndContinue pins the
// default policy for the N>1 branch: warn, continue, keep the last
// write. Mutable.RequestModel was set (simulating the final
// effective tool call), so fallback_used must be false and the IR
// must reflect the latest hints.
func TestAnalyzer_CallCountGate_MultipleCalls_WarnAndContinue(t *testing.T) {
	restoreAnalysisLimits(t)
	tool.SetAnalysisLimits(tool.AnalysisLimits{RejectMultipleEmit: false})

	mu := types.NewMutableState("multi-call warn path")
	mu.SetRequestModel(types.RequestModel{
		Intent:     types.IntentRootCause,
		Scenario:   types.ScenarioRootCause,
		Complexity: types.ComplexityComplex,
		AnalyzerHints: types.AnalyzerHints{
			Keywords: []string{"a", "b", "c", "d", "e", "f", "g", "h"},
			Entities: []string{"Foo"},
			Kind:     "root_cause",
		},
	})
	// Three emit_analysis attempts — mix of successes and failures to
	// confirm the counter is success-agnostic.
	results := []types.ToolResult{emitResult(true), emitResult(false), emitResult(true)}

	data, out := parseWithToolResults(t, mu, results)

	if got := getFloat(t, data, "analysis_emit_calls"); got != 3 {
		t.Errorf("analysis_emit_calls = %v, want 3", got)
	}
	if out.Error != "" {
		t.Errorf("warn-and-continue policy must NOT set StageOutput.Error, got %q", out.Error)
	}
	if out.AnalysisIR == nil || out.AnalysisIR.RequestModel.Intent != types.IntentRootCause {
		t.Errorf("warn branch must still build the IR from the last write, got %+v", out.AnalysisIR)
	}
}

// TestAnalyzer_CallCountGate_MultipleCalls_Reject pins the strict
// policy: RejectMultipleEmit=true must set StageOutput.Error with a
// descriptive message while STILL populating the IR so downstream
// stages can continue when the operator ignores the error signal.
func TestAnalyzer_CallCountGate_MultipleCalls_Reject(t *testing.T) {
	restoreAnalysisLimits(t)
	tool.SetAnalysisLimits(tool.AnalysisLimits{RejectMultipleEmit: true})

	mu := types.NewMutableState("multi-call reject path")
	mu.SetRequestModel(types.RequestModel{
		Intent:     types.IntentExplain,
		Scenario:   types.ScenarioArchitectureExplain,
		Complexity: types.ComplexityModerate,
	})
	results := []types.ToolResult{emitResult(true), emitResult(true)}

	data, out := parseWithToolResults(t, mu, results)

	if got := getFloat(t, data, "analysis_emit_calls"); got != 2 {
		t.Errorf("analysis_emit_calls = %v, want 2", got)
	}
	if out.Error == "" {
		t.Error("strict policy must set StageOutput.Error when emit_calls > 1")
	}
	if !strings.Contains(out.Error, "2 times") {
		t.Errorf("strict-policy Error should report the call count, got %q", out.Error)
	}
	if !strings.Contains(out.Error, "policy=reject") {
		t.Errorf("strict-policy Error should cite the policy, got %q", out.Error)
	}
	if out.AnalysisIR == nil {
		t.Error("strict policy must still populate the IR so downstream can continue")
	}
}

// TestAnalyzer_CallCountGate_IgnoresOtherTools asserts the counter
// only reacts to ToolName=="emit_analysis". Under the current
// ToolSuggestions contract no other tool can appear in the analyzer's
// ReAct loop, but the counter must be conservative: if a future
// change ever lets the analyzer call a second tool, the gate should
// not mistake that for a repeat emit_analysis.
func TestAnalyzer_CallCountGate_IgnoresOtherTools(t *testing.T) {
	restoreAnalysisLimits(t)

	mu := types.NewMutableState("mixed tool stream")
	mu.SetRequestModel(types.RequestModel{
		Intent:     types.IntentExplain,
		Scenario:   types.ScenarioArchitectureExplain,
		Complexity: types.ComplexityModerate,
	})
	results := []types.ToolResult{
		{ToolName: "grep", Success: true},
		emitResult(true),
		{ToolName: "read_file", Success: true},
	}

	data, out := parseWithToolResults(t, mu, results)

	if got := getFloat(t, data, "analysis_emit_calls"); got != 1 {
		t.Errorf("analysis_emit_calls = %v, want 1 (only the single emit_analysis entry)", got)
	}
	if out.Error != "" {
		t.Errorf("single-call branch must not set Error, got %q", out.Error)
	}
}

// observePrescan is a test helper that fires a single PhaseMidLoop
// observation with `lastTool` as the LastToolResult. Used by the
// pre-scan budget tests so each test reads like a sequence of "round
// 1: grep" / "round 2: repo_map" statements. The helper also feeds
// the monotonically growing AllToolResults slice every iteration,
// matching BaseAgent's real observation stream.
func observePrescan(e *analyzerEvaluator, iteration int, lastTool string, history *[]types.ToolResult) LoopSignal {
	result := types.ToolResult{ToolName: lastTool, Success: true}
	*history = append(*history, result)
	obs := LoopObservation{
		Phase:          PhaseMidLoop,
		Iteration:      iteration,
		LastToolResult: &(*history)[len(*history)-1],
		AllToolResults: *history,
	}
	return e.Observe(nil, obs)
}

// TestAnalyzer_PrescanBudget_WithinBudget verifies that two pre-scan
// rounds leave Observe returning an empty signal when MaxPrescanRounds=2
// (the historical default). The prescanRounds counter climbs to exactly
// 2, nothing fires, and the next iteration with emit_analysis as
// LastToolResult does NOT advance the counter further.
func TestAnalyzer_PrescanBudget_WithinBudget(t *testing.T) {
	restoreAnalysisLimits(t)
	// Explicit default reinstall so the test is self-contained even if
	// the global default drifts.
	tool.SetAnalysisLimits(tool.AnalysisLimits{MaxPrescanRounds: 2})

	e := &analyzerEvaluator{}
	e.BuildInitialInstruction(nil, nil) // reset prescanRounds = 0
	var history []types.ToolResult

	if sig := observePrescan(e, 0, "grep", &history); sig.StopRequested {
		t.Errorf("round 1 (grep): unexpected StopRequested (reason=%q)", sig.StopReason)
	}
	if e.prescanRounds != 1 {
		t.Errorf("after round 1: prescanRounds = %d, want 1", e.prescanRounds)
	}
	if sig := observePrescan(e, 1, "repo_map", &history); sig.StopRequested {
		t.Errorf("round 2 (repo_map): unexpected StopRequested (reason=%q)", sig.StopReason)
	}
	if e.prescanRounds != 2 {
		t.Errorf("after round 2: prescanRounds = %d, want 2", e.prescanRounds)
	}
	// A third observation whose LastToolResult is emit_analysis must
	// NOT advance the counter, but SHOULD request stop (emit_analysis
	// is the analyzer's terminal action — no further iteration needed).
	if sig := observePrescan(e, 2, "emit_analysis", &history); !sig.StopRequested {
		t.Errorf("emit_analysis observation: expected StopRequested=true")
	}
	if e.prescanRounds != 2 {
		t.Errorf("emit_analysis must not advance prescanRounds; got %d, want 2", e.prescanRounds)
	}
}

// TestAnalyzer_PrescanBudget_MustEmitHintOnLastLegalRound verifies
// that the runtime injects a strong must-emit hint the moment the
// LLM consumes its final legal prescan slot — before the hard stop
// fires. Without this grace hint the audit on unfamiliar repos
// (glamour / bubbletea / lipgloss) caught the LLM burning 3 rounds
// on entity verification with zero warning that the stage was about
// to fail loud with nothing emitted.
func TestAnalyzer_PrescanBudget_MustEmitHintOnLastLegalRound(t *testing.T) {
	restoreAnalysisLimits(t)
	tool.SetAnalysisLimits(tool.AnalysisLimits{MaxPrescanRounds: 2})

	e := &analyzerEvaluator{}
	e.BuildInitialInstruction(nil, nil)
	var history []types.ToolResult

	// Round 1: no hint — plenty of budget remaining.
	sig := observePrescan(e, 0, "grep", &history)
	if sig.HintRequested {
		t.Errorf("round 1: unexpected must-emit hint (prescanRounds=%d)", e.prescanRounds)
	}

	// Round 2: prescanRounds now == max. MUST get the must-emit hint,
	// must NOT request stop (the LLM still deserves one grace response
	// to call emit_analysis).
	sig = observePrescan(e, 1, "repo_map", &history)
	if sig.StopRequested {
		t.Fatalf("round 2 (at budget): unexpected StopRequested=true; the grace round is the whole point (reason=%q)", sig.StopReason)
	}
	if !sig.HintRequested {
		t.Fatalf("round 2 (at budget): HintRequested=false, expected must-emit hint")
	}
	if sig.HintKey != "analyzer.must-emit" {
		t.Errorf("HintKey = %q, want \"analyzer.must-emit\"", sig.HintKey)
	}
	if !strings.Contains(sig.Hint, "emit_analysis") {
		t.Errorf("Hint must mention emit_analysis by name; got %q", sig.Hint)
	}
	if strings.Contains(sig.Hint, "SAME response") || strings.Contains(sig.Hint, "batch the grep") {
		t.Errorf("must-emit hint must not invite batched prescan after budget; got %q", sig.Hint)
	}
	if !strings.Contains(sig.Hint, "even batched with emit_analysis") {
		t.Errorf("must-emit hint must make batched prescan forbidden; got %q", sig.Hint)
	}

	// Round 3: LLM ignored the hint and kept prescanning. NOW the
	// stage must force-stop — fail-loud contract intact.
	sig = observePrescan(e, 2, "list_files", &history)
	if !sig.StopRequested {
		t.Errorf("round 3 (over budget after hint ignored): StopRequested=false, want true")
	}
}

func TestAnalyzer_PrescanBudget_PathScopedGrepHintsEmitBeforeBudgetWall(t *testing.T) {
	restoreAnalysisLimits(t)
	tool.SetAnalysisLimits(tool.AnalysisLimits{MaxPrescanRounds: 3})

	mu := types.NewMutableState("explicit file import inventory")
	ctx := &types.AgentContext{Stage: types.StageAnalyze, Mutable: mu}
	e := &analyzerEvaluator{}
	e.BuildInitialInstruction(ctx, nil)

	result := types.ToolResult{
		ToolName: "grep",
		Success:  true,
		Summary:  "[grep: 1 matching files]\n[grep params: pattern=import path=internal/tool/emit_analysis.go files_only=true]\ninternal/tool/emit_analysis.go\n",
		PathDiscovery: &types.ToolPathDiscovery{
			Kind:        types.ToolPathDiscoveryKindGrep,
			Path:        "internal/tool/emit_analysis.go",
			FilesOnly:   true,
			ResultCount: 1,
		},
	}
	sig := e.Observe(ctx, LoopObservation{
		Phase:          PhaseMidLoop,
		Iteration:      0,
		LastToolResult: &result,
		AllToolResults: []types.ToolResult{result},
	})
	if !sig.HintRequested {
		t.Fatal("path-scoped files-only grep should ask analyzer to emit classification")
	}
	if sig.HintKey != "analyzer.path-scoped-prescan-ready" {
		t.Fatalf("HintKey = %q, want analyzer.path-scoped-prescan-ready", sig.HintKey)
	}
	if sig.StopRequested {
		t.Fatalf("path-scoped hint should be soft, got stop reason %q", sig.StopReason)
	}
	if !strings.Contains(sig.Hint, "line-level proof") || !strings.Contains(sig.Hint, "emit_analysis") {
		t.Fatalf("hint should preserve analyzer/explorer boundary, got %q", sig.Hint)
	}
}

func TestAnalyzer_PrescanReady_DefaultBudgetForcesEmitOnlySurface(t *testing.T) {
	restoreAnalysisLimits(t)
	tool.SetAnalysisLimits(tool.AnalysisLimits{MaxPrescanRounds: 2})

	mu := types.NewMutableState("explicit file import inventory")
	ctx := &types.AgentContext{Stage: types.StageAnalyze, Mutable: mu}
	e := &analyzerEvaluator{}
	e.BuildInitialInstruction(ctx, nil)

	result := types.ToolResult{
		ToolName: "grep",
		Success:  true,
		Summary:  "[grep: 1 matching files]\n[grep params: pattern=import path=internal/tool/emit_analysis.go files_only=true]\ninternal/tool/emit_analysis.go\n",
		PathDiscovery: &types.ToolPathDiscovery{
			Kind:        types.ToolPathDiscoveryKindGrep,
			Path:        "internal/tool/emit_analysis.go",
			FilesOnly:   true,
			ResultCount: 1,
		},
	}
	sig := e.Observe(ctx, LoopObservation{
		Phase:          PhaseMidLoop,
		Iteration:      0,
		LastToolResult: &result,
		AllToolResults: []types.ToolResult{result},
	})
	if !sig.HintRequested || sig.HintKey != "analyzer.prescan-ready.emit-only" {
		t.Fatalf("expected prescan-ready emit-only hint, got %+v", sig)
	}
	if !mu.PrescanReady() {
		t.Fatal("path-scoped default-budget prescan should mark analyzer prescan ready")
	}
	gotSchemas := e.FilterToolSchemas(ctx, []llm.ToolSchema{
		{Name: "emit_analysis"},
		{Name: "repo_map"},
		{Name: "grep"},
		{Name: "list_files"},
	})
	if len(gotSchemas) != 1 || gotSchemas[0].Name != "emit_analysis" {
		t.Fatalf("prescan-ready analyzer should expose only emit_analysis, got %+v", gotSchemas)
	}
	rejected := validateAnalyzerToolBoundary(ctx, llm.ToolCall{
		Name:   "grep",
		Params: json.RawMessage(`{"pattern":"import","files_only":true}`),
	})
	if rejected == nil || rejected.Success {
		t.Fatalf("terminal prescan-ready boundary should reject handwritten grep, got %+v", rejected)
	}
	if rejected.Repair == nil || rejected.Repair.Code != analyzerPrescanTerminalEmitModeCode {
		t.Fatalf("expected terminal emit repair code, got %+v", rejected.Repair)
	}
}

func TestAnalyzer_PrescanReady_LargerBudgetDoesNotCloseOnFirstScopedProbe(t *testing.T) {
	restoreAnalysisLimits(t)
	tool.SetAnalysisLimits(tool.AnalysisLimits{MaxPrescanRounds: 4})

	mu := types.NewMutableState("multi component inventory")
	ctx := &types.AgentContext{Stage: types.StageAnalyze, Mutable: mu}
	e := &analyzerEvaluator{}
	e.BuildInitialInstruction(ctx, nil)

	result := types.ToolResult{
		ToolName: "grep",
		Success:  true,
		Summary:  "[grep: 1 matching files]\n[grep params: pattern=foo path=internal/agent files_only=true]\ninternal/agent/a.go\n",
		PathDiscovery: &types.ToolPathDiscovery{
			Kind:        types.ToolPathDiscoveryKindGrep,
			Path:        "internal/agent",
			FilesOnly:   true,
			ResultCount: 1,
		},
	}
	sig := e.Observe(ctx, LoopObservation{
		Phase:          PhaseMidLoop,
		LastToolResult: &result,
		AllToolResults: []types.ToolResult{result},
	})
	if sig.HintKey != "analyzer.path-scoped-prescan-ready" {
		t.Fatalf("first scoped probe under larger budget should stay soft-ready, got %+v", sig)
	}
	if mu.PrescanReady() {
		t.Fatal("larger-budget analyzer should not close prescan after the first scoped probe")
	}
	names := analyzerSchemaNames(e.FilterToolSchemas(ctx, []llm.ToolSchema{
		{Name: "emit_analysis"},
		{Name: "repo_map"},
		{Name: "grep"},
		{Name: "list_files"},
	}))
	for _, want := range []string{"emit_analysis", "repo_map", "grep", "list_files"} {
		if !names[want] {
			t.Fatalf("larger-budget first probe should still expose %s; got %+v", want, names)
		}
	}
}

func TestAnalyzer_PrescanReady_ListFilesChildScopeListingForcesEmitOnly(t *testing.T) {
	restoreAnalysisLimits(t)
	tool.SetAnalysisLimits(tool.AnalysisLimits{MaxPrescanRounds: 4})

	mu := types.NewMutableState("mixed source inventory")
	ctx := &types.AgentContext{Stage: types.StageAnalyze, Mutable: mu}
	e := &analyzerEvaluator{}
	e.BuildInitialInstruction(ctx, nil)

	result := types.ToolResult{
		ToolName: "list_files",
		Success:  true,
		Summary: strings.Join([]string{
			"[list_files: path=src modules recursive=false]",
			"src modules/api",
			"src modules/config",
			"src modules/routes",
			"src modules/runtime",
			"src modules/web",
		}, "\n"),
		PathDiscovery: &types.ToolPathDiscovery{
			Kind:        types.ToolPathDiscoveryKindListFiles,
			Path:        "src modules",
			ResultCount: 5,
		},
	}
	sig := e.Observe(ctx, LoopObservation{
		Phase:          PhaseMidLoop,
		LastToolResult: &result,
		AllToolResults: []types.ToolResult{result},
	})
	if !sig.HintRequested || sig.HintKey != "analyzer.prescan-ready.emit-only" {
		t.Fatalf("child-scope list_files should force emit-only, got %+v", sig)
	}
	if !mu.PrescanReady() {
		t.Fatal("child-scope list_files should mark analyzer prescan ready")
	}
	if !strings.Contains(sig.Hint, "source-inventory rows") {
		t.Fatalf("hint should preserve analyze/explore boundary, got %q", sig.Hint)
	}
	gotSchemas := e.FilterToolSchemas(ctx, []llm.ToolSchema{
		{Name: "emit_analysis"},
		{Name: "repo_map"},
		{Name: "grep"},
		{Name: "list_files"},
	})
	if len(gotSchemas) != 1 || gotSchemas[0].Name != "emit_analysis" {
		t.Fatalf("list_files-ready analyzer should expose only emit_analysis, got %+v", gotSchemas)
	}
	sigs := mu.AnalyzerDecisions()
	if len(sigs) != 1 || !strings.Contains(sigs[0].Reason, "directory listing") {
		t.Fatalf("expected directory-listing analyzer decision, got %+v", sigs)
	}
}

func TestAnalyzer_PrescanReady_SourceInventoryNavigationForcesEmitOnly(t *testing.T) {
	restoreAnalysisLimits(t)
	tool.SetAnalysisLimits(tool.AnalysisLimits{MaxPrescanRounds: 4})

	mu := types.NewMutableState("source member inventory")
	ctx := &types.AgentContext{Stage: types.StageAnalyze, Mutable: mu}
	e := &analyzerEvaluator{}
	e.BuildInitialInstruction(ctx, nil)

	result := types.ToolResult{
		ToolName: "repo_map",
		Success:  true,
		Summary:  "[repo_map: view=source_inventory path=. roles=type]\nstructured navigation only\n",
		Observations: []types.ObservationRecord{{
			Producer:  "repo_map",
			Predicate: types.RepoMapNavigationObservationPredicate,
			Object:    string(types.RepoMapNavigationRouteSourceInventory),
			Value:     string(types.RepoMapNavigationRouteSourceInventory),
		}},
	}
	sig := e.Observe(ctx, LoopObservation{
		Phase:          PhaseMidLoop,
		LastToolResult: &result,
		AllToolResults: []types.ToolResult{result},
	})
	if !sig.HintRequested || sig.HintKey != "analyzer.prescan-ready.emit-only" {
		t.Fatalf("source_inventory navigation should force emit-only readiness, got %+v", sig)
	}
	if !mu.PrescanReady() {
		t.Fatal("source_inventory navigation should mark analyzer prescan ready")
	}
	if !strings.Contains(sig.Hint, "source-inventory") || !strings.Contains(sig.Hint, "emit_analysis") {
		t.Fatalf("hint should preserve source-inventory classification boundary, got %q", sig.Hint)
	}
	rejected := validateAnalyzerPrescanToolCall(ctx, llm.ToolCall{
		Name:   "repo_map",
		Params: json.RawMessage(`{"path":".","view":"source_inventory","roles":["type"],"top_n":24}`),
	})
	if rejected == nil || rejected.Success {
		t.Fatalf("prescan-ready source_inventory should reject stale follow-up navigation, got %+v", rejected)
	}
	if rejected.Repair == nil || rejected.Repair.Code != analyzerPrescanTerminalEmitModeCode {
		t.Fatalf("expected terminal emit repair code, got %+v", rejected.Repair)
	}
	if !strings.Contains(rejected.Summary, "emit_analysis") {
		t.Fatalf("stale source_inventory rejection should redirect to emit_analysis, got %q", rejected.Summary)
	}
}

func TestAnalyzer_PrescanReady_SmallListFilesRemainsSoftUnderLargerBudget(t *testing.T) {
	restoreAnalysisLimits(t)
	tool.SetAnalysisLimits(tool.AnalysisLimits{MaxPrescanRounds: 4})

	mu := types.NewMutableState("small scoped listing")
	ctx := &types.AgentContext{Stage: types.StageAnalyze, Mutable: mu}
	e := &analyzerEvaluator{}
	e.BuildInitialInstruction(ctx, nil)

	result := types.ToolResult{
		ToolName: "list_files",
		Success:  true,
		Summary:  "[list_files: path=src/api recursive=false]\nsrc/api/routes.ts\nsrc/api/config.yaml\n",
		PathDiscovery: &types.ToolPathDiscovery{
			Kind:        types.ToolPathDiscoveryKindListFiles,
			Path:        "src/api",
			ResultCount: 2,
		},
	}
	sig := e.Observe(ctx, LoopObservation{
		Phase:          PhaseMidLoop,
		LastToolResult: &result,
		AllToolResults: []types.ToolResult{result},
	})
	if sig.HintKey != "analyzer.path-scoped-prescan-ready" {
		t.Fatalf("small scoped list_files should remain soft-ready under larger budget, got %+v", sig)
	}
	if mu.PrescanReady() {
		t.Fatal("small scoped list_files should not close larger-budget prescan")
	}
}

func TestAnalyzer_PrescanResultSupportsClassificationStop_GuardsBroadOrLineGreps(t *testing.T) {
	tests := []struct {
		name string
		res  types.ToolResult
		want bool
	}{
		{
			name: "summary-only files-only grep ignored",
			res: types.ToolResult{
				ToolName: "grep", Success: true,
				Summary: "[grep: 2 matching files]\n[grep params: pattern=foo path=internal/agent files_only=true]\ninternal/agent/a.go\n",
			},
		},
		{
			name: "path scoped files-only match",
			res: types.ToolResult{
				ToolName: "grep", Success: true,
				Summary: "transparent summary is not authoritative",
				PathDiscovery: &types.ToolPathDiscovery{
					Kind:        types.ToolPathDiscoveryKindGrep,
					Path:        "internal/agent",
					FilesOnly:   true,
					ResultCount: 2,
				},
			},
			want: true,
		},
		{
			name: "path with spaces stays path scoped",
			res: types.ToolResult{
				ToolName: "grep", Success: true,
				PathDiscovery: &types.ToolPathDiscovery{
					Kind:        types.ToolPathDiscoveryKindGrep,
					Path:        "sample project/src",
					FilesOnly:   true,
					ResultCount: 1,
				},
			},
			want: true,
		},
		{
			name: "repo root too broad",
			res: types.ToolResult{
				ToolName: "grep", Success: true,
				PathDiscovery: &types.ToolPathDiscovery{
					Kind:        types.ToolPathDiscoveryKindGrep,
					Path:        ".",
					FilesOnly:   true,
					ResultCount: 2,
				},
			},
		},
		{
			name: "up-directory path ignored",
			res: types.ToolResult{
				ToolName: "grep", Success: true,
				PathDiscovery: &types.ToolPathDiscovery{
					Kind:        types.ToolPathDiscoveryKindGrep,
					Path:        "../other",
					FilesOnly:   true,
					ResultCount: 1,
				},
			},
		},
		{
			name: "failed grep ignored",
			res: types.ToolResult{
				ToolName: "grep", Success: false,
				PathDiscovery: &types.ToolPathDiscovery{
					Kind:        types.ToolPathDiscoveryKindGrep,
					Path:        "internal/agent",
					FilesOnly:   true,
					ResultCount: 1,
				},
			},
		},
		{
			name: "non-files-only grep ignored",
			res: types.ToolResult{
				ToolName: "grep", Success: true,
				PathDiscovery: &types.ToolPathDiscovery{
					Kind:        types.ToolPathDiscoveryKindGrep,
					Path:        "internal/agent",
					ResultCount: 2,
				},
			},
		},
		{
			name: "line grep not analyzer classification proof",
			res: types.ToolResult{
				ToolName: "grep", Success: true,
				PathDiscovery: &types.ToolPathDiscovery{
					Kind:        types.ToolPathDiscoveryKindGrep,
					Path:        "internal/agent",
					ResultCount: 2,
				},
			},
		},
		{
			name: "repo map ignored",
			res: types.ToolResult{
				ToolName: "repo_map", Success: true,
				Summary: "internal/agent/analyzer.go",
			},
		},
		{
			name: "scoped list files with child entries",
			res: types.ToolResult{
				ToolName: "list_files", Success: true,
				Summary: "transparent summary is not authoritative",
				PathDiscovery: &types.ToolPathDiscovery{
					Kind:        types.ToolPathDiscoveryKindListFiles,
					Path:        "src services",
					ResultCount: 2,
				},
			},
			want: true,
		},
		{
			name: "summary-only list files ignored",
			res: types.ToolResult{
				ToolName: "list_files", Success: true,
				Summary: "[list_files: path=src services recursive=false]\nsrc services/api\nsrc services/config\n",
			},
		},
		{
			name: "root list files ignored",
			res: types.ToolResult{
				ToolName: "list_files", Success: true,
				PathDiscovery: &types.ToolPathDiscovery{
					Kind:        types.ToolPathDiscoveryKindListFiles,
					Path:        ".",
					ResultCount: 3,
				},
			},
		},
		{
			name: "parent escape list files ignored",
			res: types.ToolResult{
				ToolName: "list_files", Success: true,
				PathDiscovery: &types.ToolPathDiscovery{
					Kind:        types.ToolPathDiscoveryKindListFiles,
					Path:        "../outside",
					ResultCount: 2,
				},
			},
		},
		{
			name: "failed list files ignored",
			res: types.ToolResult{
				ToolName: "list_files", Success: false,
				PathDiscovery: &types.ToolPathDiscovery{
					Kind:        types.ToolPathDiscoveryKindListFiles,
					Path:        "src",
					ResultCount: 1,
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := analyzerPrescanResultSupportsClassificationStop(tt.res); got != tt.want {
				t.Fatalf("supportsClassificationStop = %t, want %t", got, tt.want)
			}
		})
	}
}

func TestAnalyzer_SameBatchPrescanSignalSkipsSiblingFanoutWithoutGlobalReady(t *testing.T) {
	mu := types.NewMutableState("source inventory shape")
	mu.SetPrescanRoundLimit(4)
	ctx := &types.AgentContext{Stage: types.StageAnalyze, Mutable: mu}
	result := &types.ToolResult{
		ToolName: "grep",
		Success:  true,
		PathDiscovery: &types.ToolPathDiscovery{
			Kind:        types.ToolPathDiscoveryKindGrep,
			Path:        ".",
			FilesOnly:   true,
			ResultCount: 228,
		},
	}

	if !applyAnalyzerSameBatchPrescanBoundary(ctx, result) {
		t.Fatal("root-scoped files-only grep should close only the current assistant batch to prevent fan-out")
	}
	if mu.PrescanReady() {
		t.Fatal("root-scoped files-only grep must not become global PrescanReady")
	}
	skipped := analyzerSameBatchStalePrescanSkipResult(ctx, llm.ToolCall{
		Name:   "list_files",
		Params: json.RawMessage(`{"path":"."}`),
	}, true)
	if skipped == nil || !skipped.Success {
		t.Fatalf("sibling prescan should be skipped as non-failure, got %+v", skipped)
	}
	if !strings.Contains(skipped.Summary, "same assistant response") ||
		!strings.Contains(skipped.Summary, "not evidence absence") {
		t.Fatalf("skip summary should explain local fan-out suppression without absence semantics: %q", skipped.Summary)
	}
}

func TestAnalyzer_FilterToolSchemas_NormalDropsContentTools(t *testing.T) {
	e := &analyzerEvaluator{}
	ctx := &types.AgentContext{Stage: types.StageAnalyze, Mutable: types.NewMutableState("question")}
	ctx.Mutable.SetPrescanRoundLimit(3)
	schemas := []llm.ToolSchema{
		{Name: "emit_analysis"},
		{Name: "repo_map"},
		{Name: "grep"},
		{Name: "list_files"},
		{Name: "read_file"},
		{Name: "exec_command"},
	}

	got := e.FilterToolSchemas(ctx, schemas)
	names := analyzerSchemaNames(got)
	for _, want := range []string{"emit_analysis", "repo_map", "grep", "list_files"} {
		if !names[want] {
			t.Fatalf("normal analyze schema missing %s; got %+v", want, got)
		}
	}
	for _, forbidden := range []string{"read_file", "exec_command"} {
		if names[forbidden] {
			t.Fatalf("normal analyze schema exposed forbidden content tool %s; got %+v", forbidden, got)
		}
	}
}

func TestAnalyzer_FilterToolSchemas_EmitOnlyAfterPrescanLimit(t *testing.T) {
	e := &analyzerEvaluator{}
	mu := types.NewMutableState("question")
	mu.SetPrescanRoundLimit(2)
	mu.AppendPrescanSummary("round one")
	mu.AppendPrescanSummary("round two")
	ctx := &types.AgentContext{Stage: types.StageAnalyze, Mutable: mu}
	schemas := []llm.ToolSchema{
		{Name: "emit_analysis"},
		{Name: "repo_map"},
		{Name: "grep"},
		{Name: "list_files"},
	}

	got := e.FilterToolSchemas(ctx, schemas)
	if len(got) != 1 || got[0].Name != "emit_analysis" {
		t.Fatalf("prescan limit should leave only emit_analysis, got %+v", got)
	}
}

func TestAnalyzer_FilterToolSchemas_ExplicitRuntimeArtifactPathEmitOnly(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "runtime_path_panic.log")
	if err := os.WriteFile(logPath, []byte("panic: boom\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := &types.AgentContext{
		Stage:     types.StageAnalyze,
		RepoRoot:  dir,
		WorkDir:   dir,
		Objective: "只分析 " + logPath + " 这个日志文件，不分析代码。",
	}
	schemas := []llm.ToolSchema{
		{Name: "emit_analysis"},
		{Name: "repo_map"},
		{Name: "grep"},
		{Name: "list_files"},
	}

	got := (&analyzerEvaluator{}).FilterToolSchemas(ctx, schemas)
	if len(got) != 1 || got[0].Name != "emit_analysis" {
		t.Fatalf("explicit runtime artifact path should leave only emit_analysis, got %+v", got)
	}
}

func TestAnalyzer_RejectsContentToolBeforeExecution(t *testing.T) {
	ctx := &types.AgentContext{Stage: types.StageAnalyze, Mutable: types.NewMutableState("question")}
	res := validateAnalyzerToolBoundary(ctx, llm.ToolCall{Name: "read_file", Params: json.RawMessage(`{"path":"internal/agent/analyzer.go","limit":"200"}`)})
	if res == nil {
		t.Fatal("expected analyzer tool-boundary rejection for read_file")
	}
	if res.Success {
		t.Fatalf("rejected read_file must be unsuccessful: %+v", res)
	}
	if res.Repair == nil || res.Repair.Code != analyzerToolNotAllowedCode {
		t.Fatalf("expected structured repair code %q, got %+v", analyzerToolNotAllowedCode, res.Repair)
	}
	if !strings.Contains(res.Summary, "emit_analysis") {
		t.Fatalf("rejection should direct the model to emit_analysis, got %q", res.Summary)
	}
	intents := ctx.Mutable.AnalyzerBlockedToolIntents()
	if len(intents) != 1 {
		t.Fatalf("blocked read intent should be recorded once, got %+v", intents)
	}
	if intents[0].ToolName != "read_file" || intents[0].Path != "internal/agent/analyzer.go" {
		t.Fatalf("blocked intent = %+v", intents[0])
	}
	if ctx.Mutable.PrescanRoundCount() != 0 {
		t.Fatalf("blocked deep read must not spend prescan rounds, got %d", ctx.Mutable.PrescanRoundCount())
	}
	if blob := ctx.Mutable.PrescanSummaryBlob(); !strings.Contains(blob, "internal/agent/analyzer.go") {
		t.Fatalf("blocked path should be available to analysis validation corpus, got %q", blob)
	}
	gotSchemas := (&analyzerEvaluator{}).FilterToolSchemas(ctx, []llm.ToolSchema{
		{Name: "emit_analysis"},
		{Name: "repo_map"},
		{Name: "grep"},
		{Name: "list_files"},
	})
	if len(gotSchemas) != 1 || gotSchemas[0].Name != "emit_analysis" {
		t.Fatalf("blocked deep-read intent should force next analyzer request to emit-only, got %+v", gotSchemas)
	}
}

func TestAnalyzer_Observe_ExplicitRuntimeArtifactPathHintIsEmitOnly(t *testing.T) {
	ctx := &types.AgentContext{
		Stage:     types.StageAnalyze,
		Objective: "只分析 /Users/han/opt/codrax/eval/fixtures/runtime_path_panic.log 这个日志文件，不分析代码。",
		Mutable:   types.NewMutableState("question"),
	}
	res := validateAnalyzerToolBoundary(ctx, llm.ToolCall{
		Name:   "list_files",
		Params: json.RawMessage(`{"path":"/Users/han/opt/codrax/eval/fixtures"}`),
	})
	if res == nil || res.Repair == nil || res.Repair.Code != analyzerExplicitRuntimeArtifactPathEmitOnlyCode {
		t.Fatalf("expected explicit runtime artifact path repair, got %+v", res)
	}
	sig := (&analyzerEvaluator{}).Observe(ctx, LoopObservation{
		Phase:          PhaseMidLoop,
		LastToolResult: res,
		AllToolResults: []types.ToolResult{*res},
	})
	if !sig.HintRequested {
		t.Fatalf("expected runtime-artifact emit-only hint, got %+v", sig)
	}
	for _, want := range []string{"runtime artifact path", "emit_analysis exactly once", "trace_query"} {
		if !strings.Contains(sig.Hint, want) {
			t.Fatalf("hint missing %q in %q", want, sig.Hint)
		}
	}
	for _, forbidden := range []string{"grep(files_only=true)", "repo_map overview", "list_files for light"} {
		if strings.Contains(sig.Hint, forbidden) {
			t.Fatalf("runtime-artifact hint should not invite navigation tools, got %q", sig.Hint)
		}
	}
}

func TestAnalyzer_Observe_BlockedToolHintMatchesTerminalEmitOnlySurface(t *testing.T) {
	ctx := &types.AgentContext{Stage: types.StageAnalyze, Mutable: types.NewMutableState("question")}
	res := validateAnalyzerToolBoundary(ctx, llm.ToolCall{
		Name:   "read_file",
		Params: json.RawMessage(`{"path":"internal/agent/analyzer.go","limit":200}`),
	})
	if res == nil || res.Repair == nil || res.Repair.Code != analyzerToolNotAllowedCode {
		t.Fatalf("expected read_file boundary repair, got %+v", res)
	}
	sig := (&analyzerEvaluator{}).Observe(ctx, LoopObservation{
		Phase:          PhaseMidLoop,
		LastToolResult: res,
		AllToolResults: []types.ToolResult{*res},
	})
	if !sig.HintRequested {
		t.Fatalf("expected terminal correction hint, got %+v", sig)
	}
	if !strings.Contains(sig.Hint, "emit_analysis exactly once") {
		t.Fatalf("hint should force terminal emit_analysis, got %q", sig.Hint)
	}
	for _, forbidden := range []string{"use only repo_map", "grep(files_only=true)", "Retry grep"} {
		if strings.Contains(sig.Hint, forbidden) {
			t.Fatalf("terminal hint should not invite navigation tools, got %q", sig.Hint)
		}
	}
}

func TestAnalyzer_TerminalEmitOnlyRejectsHandwrittenPrescanTool(t *testing.T) {
	ctx := &types.AgentContext{Stage: types.StageAnalyze, Mutable: types.NewMutableState("question")}
	first := validateAnalyzerToolBoundary(ctx, llm.ToolCall{
		Name:   "read_file",
		Params: json.RawMessage(`{"path":"internal/agent/analyzer.go"}`),
	})
	if first == nil || first.Success {
		t.Fatalf("expected initial deep-read rejection, got %+v", first)
	}

	got := validateAnalyzerToolBoundary(ctx, llm.ToolCall{
		Name:   "grep",
		Params: json.RawMessage(`{"pattern":"buildAnalysisIR","files_only":true}`),
	})
	if got == nil {
		t.Fatal("expected terminal emit-only runtime boundary to reject handwritten grep")
	}
	if got.Success {
		t.Fatalf("terminal emit-only rejection must be unsuccessful: %+v", got)
	}
	if got.Repair == nil || got.Repair.Code != analyzerPrescanTerminalEmitModeCode {
		t.Fatalf("expected repair code %q, got %+v", analyzerPrescanTerminalEmitModeCode, got.Repair)
	}
	if !strings.Contains(got.Summary, "available tools here: emit_analysis") {
		t.Fatalf("summary should expose only emit_analysis, got %q", got.Summary)
	}
	if ctx.Mutable.PrescanRoundCount() != 0 {
		t.Fatalf("rejected handwritten prescan tool must not spend prescan rounds, got %d", ctx.Mutable.PrescanRoundCount())
	}
	if ok := validateAnalyzerToolBoundary(ctx, llm.ToolCall{Name: "emit_analysis", Params: json.RawMessage(`{}`)}); ok != nil {
		t.Fatalf("emit_analysis should remain allowed in terminal emit-only mode, got %+v", ok)
	}
}

func TestAnalyzer_BlockedUnsafePathDoesNotEnterPrescanCorpus(t *testing.T) {
	ctx := &types.AgentContext{Stage: types.StageAnalyze, Mutable: types.NewMutableState("question")}
	res := validateAnalyzerToolBoundary(ctx, llm.ToolCall{Name: "read_file", Params: json.RawMessage(`{"path":"../huge-repo/secret.go"}`)})
	if res == nil || res.Success {
		t.Fatalf("expected rejected unsafe read_file, got %+v", res)
	}
	if got := ctx.Mutable.AnalyzerBlockedToolIntentCount(); got != 1 {
		t.Fatalf("blocked intent count = %d, want 1", got)
	}
	if blob := ctx.Mutable.PrescanSummaryBlob(); strings.Contains(blob, "../huge-repo") {
		t.Fatalf("unsafe boundary-crossing path must not enter prescan corpus, got %q", blob)
	}
}

func TestAnalyzer_Observe_StopsWhenEmitAnalysisSucceededEarlierInBatch(t *testing.T) {
	e := &analyzerEvaluator{}
	emit := types.ToolResult{ToolName: "emit_analysis", Success: true, Summary: "analysis emitted"}
	rejected := types.ToolResult{
		ToolName: "grep",
		Success:  false,
		Summary:  "analyze is already in terminal emit mode",
		Repair:   &types.ToolRepair{Code: analyzerPrescanTerminalEmitModeCode},
	}
	sig := e.Observe(&types.AgentContext{Stage: types.StageAnalyze}, LoopObservation{
		Phase:              PhaseMidLoop,
		LastToolResult:     &rejected,
		AllToolResults:     []types.ToolResult{emit, rejected},
		CurrentToolResults: []types.ToolResult{emit, rejected},
	})
	if !sig.StopRequested {
		t.Fatalf("successful emit_analysis earlier in the batch should terminate analyze, got %+v", sig)
	}
	if sig.StopReason != "emit_analysis called" {
		t.Fatalf("StopReason = %q, want emit_analysis called", sig.StopReason)
	}
}

func TestAnalyzer_Observe_BudgetRejectedPrescanGetsEmitOnlyHint(t *testing.T) {
	e := &analyzerEvaluator{}
	result := types.ToolResult{
		ToolName: "grep",
		Success:  false,
		Summary:  "pre-scan budget already reached",
		Repair:   &types.ToolRepair{Code: analyzerPrescanBudgetReachedCode},
	}
	sig := e.Observe(&types.AgentContext{Stage: types.StageAnalyze}, LoopObservation{
		Phase:          PhaseMidLoop,
		LastToolResult: &result,
		AllToolResults: []types.ToolResult{result},
	})
	if sig.StopRequested {
		t.Fatalf("budget-rejected prescan should get an emit-only correction before stage retry, got stop=%q", sig.StopReason)
	}
	if !sig.HintRequested || !strings.Contains(sig.Hint, "emit_analysis") {
		t.Fatalf("expected emit_analysis hint, got %+v", sig)
	}
	if e.prescanRounds != 0 {
		t.Fatalf("rejected over-budget tool should not increment prescanRounds; got %d", e.prescanRounds)
	}
}

func TestAnalyzer_Observe_BudgetRejectedAfterMustEmitGetsBoundedCorrectionTurns(t *testing.T) {
	restoreAnalysisLimits(t)
	const limit = 2
	tool.SetAnalysisLimits(tool.AnalysisLimits{EmitOnlyCorrectionRetries: limit})

	e := &analyzerEvaluator{terminalEmitOnlyInstructionIssued: true}
	result := types.ToolResult{
		ToolName: "list_files",
		Success:  false,
		Summary:  "pre-scan budget already reached",
		Repair:   &types.ToolRepair{Code: analyzerPrescanBudgetReachedCode},
	}
	sig := e.Observe(&types.AgentContext{Stage: types.StageAnalyze}, LoopObservation{
		Phase:              PhaseMidLoop,
		LastToolResult:     &result,
		AllToolResults:     []types.ToolResult{result},
		CurrentToolResults: []types.ToolResult{result},
	})
	for i := 0; i < limit; i++ {
		if sig.StopRequested {
			t.Fatalf("terminal pre-scan rejection %d should get a correction turn, got %+v", i+1, sig)
		}
		if !sig.HintRequested || !strings.Contains(sig.Hint, "emit_analysis") {
			t.Fatalf("expected emit-only correction hint on rejection %d, got %+v", i+1, sig)
		}
		sig = e.Observe(&types.AgentContext{Stage: types.StageAnalyze}, LoopObservation{
			Phase:              PhaseMidLoop,
			LastToolResult:     &result,
			AllToolResults:     []types.ToolResult{result},
			CurrentToolResults: []types.ToolResult{result},
		})
	}
	if !sig.StopRequested {
		t.Fatalf("terminal pre-scan rejection after correction budget should stop current attempt, got %+v", sig)
	}
	if !strings.Contains(sig.StopReason, "correction budget exhausted") {
		t.Fatalf("StopReason = %q, want correction budget context", sig.StopReason)
	}
}

func TestAnalyzer_Observe_EmitOnlyCorrectionRetriesZeroStopsImmediately(t *testing.T) {
	restoreAnalysisLimits(t)
	tool.SetAnalysisLimits(tool.AnalysisLimits{EmitOnlyCorrectionRetries: 0})

	e := &analyzerEvaluator{terminalEmitOnlyInstructionIssued: true}
	result := types.ToolResult{
		ToolName: "list_files",
		Success:  false,
		Summary:  "pre-scan budget already reached",
		Repair:   &types.ToolRepair{Code: analyzerPrescanBudgetReachedCode},
	}
	sig := e.Observe(&types.AgentContext{Stage: types.StageAnalyze}, LoopObservation{
		Phase:              PhaseMidLoop,
		LastToolResult:     &result,
		AllToolResults:     []types.ToolResult{result},
		CurrentToolResults: []types.ToolResult{result},
	})
	if !sig.StopRequested {
		t.Fatalf("zero correction retry budget should stop immediately, got %+v", sig)
	}
	if !strings.Contains(sig.StopReason, "correction budget exhausted") {
		t.Fatalf("StopReason = %q, want correction budget context", sig.StopReason)
	}
}

func TestAnalyzer_LLMRequestTimeout_TerminalEmitOnlyUsesConfiguredBudget(t *testing.T) {
	restoreAnalysisLimits(t)
	tool.SetAnalysisLimits(tool.AnalysisLimits{TerminalEmitOnlyRequestTimeoutSeconds: 2})

	e := &analyzerEvaluator{terminalEmitOnlyInstructionIssued: true}
	got, reason := e.LLMRequestTimeout(&types.AgentContext{Stage: types.StageAnalyze}, LLMRequestBudgetObservation{
		ToolSurfaceKnown:   true,
		AvailableToolNames: map[string]bool{"emit_analysis": true},
	})
	if got != 2*time.Second {
		t.Fatalf("timeout = %s, want 2s", got)
	}
	if reason != "analyzer_terminal_emit_only" {
		t.Fatalf("reason = %q", reason)
	}

	cold, coldReason := (&analyzerEvaluator{}).LLMRequestTimeout(&types.AgentContext{Stage: types.StageAnalyze}, LLMRequestBudgetObservation{
		ToolSurfaceKnown:   true,
		AvailableToolNames: map[string]bool{"emit_analysis": true, "repo_map": true},
	})
	if cold != 0 || coldReason != "" {
		t.Fatalf("non-terminal analyzer request should not get short budget, got %s %q", cold, coldReason)
	}
}

func TestAnalyzer_Observe_TerminalEmitOnlyNoToolStopsCurrentAttempt(t *testing.T) {
	e := &analyzerEvaluator{terminalEmitOnlyInstructionIssued: true}
	sig := e.Observe(&types.AgentContext{Stage: types.StageAnalyze}, LoopObservation{
		Phase:              PhaseSoftStop,
		Iteration:          2,
		Response:           llm.Response{Content: "I should call emit_analysis now."},
		ToolSurfaceKnown:   true,
		AvailableToolNames: map[string]bool{"emit_analysis": true},
	})
	if !sig.StopRequested {
		t.Fatalf("terminal emit-only no-tool response should stop current attempt, got %+v", sig)
	}
	if sig.HintRequested {
		t.Fatalf("terminal emit-only no-tool response should not inject another broad hint, got %+v", sig)
	}
	if !strings.Contains(sig.StopReason, "emit_analysis") {
		t.Fatalf("stop reason should name missing emit_analysis, got %q", sig.StopReason)
	}
}

func TestAnalyzer_Observe_TerminalRejectedBatchWithEmitAnalysisAttemptDoesNotStop(t *testing.T) {
	e := &analyzerEvaluator{terminalEmitOnlyInstructionIssued: true}
	failedEmit := types.ToolResult{
		ToolName: "emit_analysis",
		Success:  false,
		Summary:  "emit_analysis rejected: predicates object missing",
	}
	rejectedPrescan := types.ToolResult{
		ToolName: "list_files",
		Success:  false,
		Summary:  "pre-scan budget already reached",
		Repair:   &types.ToolRepair{Code: analyzerPrescanBudgetReachedCode},
	}
	sig := e.Observe(&types.AgentContext{Stage: types.StageAnalyze}, LoopObservation{
		Phase:              PhaseMidLoop,
		LastToolResult:     &rejectedPrescan,
		AllToolResults:     []types.ToolResult{failedEmit, rejectedPrescan},
		CurrentToolResults: []types.ToolResult{failedEmit, rejectedPrescan},
	})
	if sig.StopRequested || sig.HintRequested {
		t.Fatalf("failed emit_analysis in the same batch should get its own validation retry, not terminal pre-scan stop/hint: %+v", sig)
	}
}

func TestAnalyzer_Observe_RetryTerminalRejectedPrescanStopsAttempt(t *testing.T) {
	restoreAnalysisLimits(t)
	const limit = 2
	tool.SetAnalysisLimits(tool.AnalysisLimits{EmitOnlyCorrectionRetries: limit})

	e := &analyzerEvaluator{}
	ctx := &types.AgentContext{Stage: types.StageAnalyze, EmitStageRetryAttempt: 1}
	e.BuildInitialInstruction(ctx, nil)
	result := types.ToolResult{
		ToolName: "grep",
		Success:  false,
		Summary:  "analyze retry is already in terminal emit mode",
		Repair:   &types.ToolRepair{Code: analyzerPrescanTerminalEmitModeCode},
	}
	sig := e.Observe(ctx, LoopObservation{
		Phase:              PhaseMidLoop,
		LastToolResult:     &result,
		AllToolResults:     []types.ToolResult{result},
		CurrentToolResults: []types.ToolResult{result},
	})
	for i := 0; i < limit; i++ {
		if sig.StopRequested {
			t.Fatalf("retry terminal pre-scan rejection %d should get a correction turn, got %+v", i+1, sig)
		}
		if !sig.HintRequested || !strings.Contains(sig.Hint, "emit_analysis") {
			t.Fatalf("expected retry correction hint on rejection %d, got %+v", i+1, sig)
		}
		sig = e.Observe(ctx, LoopObservation{
			Phase:              PhaseMidLoop,
			LastToolResult:     &result,
			AllToolResults:     []types.ToolResult{result},
			CurrentToolResults: []types.ToolResult{result},
		})
	}
	if !sig.StopRequested {
		t.Fatalf("retry terminal pre-scan rejection after correction budget should stop current attempt, got %+v", sig)
	}
}

func TestAnalyzer_Observe_GrepFilesOnlyRejectedGetsShapeHint(t *testing.T) {
	e := &analyzerEvaluator{}
	result := types.ToolResult{
		ToolName: "grep",
		Success:  false,
		Summary:  "grep in analyze stage must use files_only=true",
		Repair:   &types.ToolRepair{Code: analyzerGrepFilesOnlyRequiredCode},
	}
	sig := e.Observe(&types.AgentContext{Stage: types.StageAnalyze}, LoopObservation{
		Phase:          PhaseMidLoop,
		LastToolResult: &result,
		AllToolResults: []types.ToolResult{result},
	})
	if sig.StopRequested {
		t.Fatalf("files_only shape rejection should get one correction hint before stage retry, got stop=%q", sig.StopReason)
	}
	if !sig.HintRequested || !strings.Contains(sig.Hint, "files_only=true") || !strings.Contains(sig.Hint, "emit_analysis") {
		t.Fatalf("expected files_only/emit_analysis hint, got %+v", sig)
	}
	if e.prescanRounds != 0 {
		t.Fatalf("rejected grep shape should not increment prescanRounds; got %d", e.prescanRounds)
	}
}

func analyzerSchemaNames(schemas []llm.ToolSchema) map[string]bool {
	out := make(map[string]bool, len(schemas))
	for _, schema := range schemas {
		out[schema.Name] = true
	}
	return out
}

// TestAnalyzer_PrescanBudget_OverBudget_ForcesStop verifies that a
// third pre-scan tool after two in-budget rounds triggers a
// force-stop with a descriptive StopReason.
func TestAnalyzer_PrescanBudget_OverBudget_ForcesStop(t *testing.T) {
	restoreAnalysisLimits(t)
	tool.SetAnalysisLimits(tool.AnalysisLimits{MaxPrescanRounds: 2})

	e := &analyzerEvaluator{}
	e.BuildInitialInstruction(nil, nil)
	var history []types.ToolResult

	_ = observePrescan(e, 0, "grep", &history)
	_ = observePrescan(e, 1, "repo_map", &history)

	sig := observePrescan(e, 2, "list_files", &history)
	if !sig.StopRequested {
		t.Fatalf("round 3 (list_files over budget): StopRequested=false, want true (prescanRounds=%d)", e.prescanRounds)
	}
	if !strings.Contains(sig.StopReason, "budget exhausted") {
		t.Errorf("StopReason = %q, want it to contain \"budget exhausted\"", sig.StopReason)
	}
	if !strings.Contains(sig.StopReason, "3") || !strings.Contains(sig.StopReason, "2") {
		t.Errorf("StopReason = %q, want it to mention rounds count (3) and max (2)", sig.StopReason)
	}
	if e.prescanRounds != 3 {
		t.Errorf("after round 3: prescanRounds = %d, want 3", e.prescanRounds)
	}
}

// TestAnalyzer_PrescanBudget_EmitAnalysisNotCounted verifies that
// emit_analysis tool results never advance the prescan counter
// regardless of how many times they fire — only the three pre-scan
// navigation tools (repo_map, grep, list_files) count as "pre-scan
// rounds". emit_analysis DOES request a stop (it is the analyzer's
// terminal action) but leaves prescanRounds unchanged.
func TestAnalyzer_PrescanBudget_EmitAnalysisNotCounted(t *testing.T) {
	restoreAnalysisLimits(t)
	tool.SetAnalysisLimits(tool.AnalysisLimits{MaxPrescanRounds: 2})

	e := &analyzerEvaluator{}
	e.BuildInitialInstruction(nil, nil)
	var history []types.ToolResult

	// Five consecutive emit_analysis-as-last observations must leave
	// prescanRounds at 0, but each should request stop.
	for i := 0; i < 5; i++ {
		sig := observePrescan(e, i, "emit_analysis", &history)
		if !sig.StopRequested {
			t.Errorf("iteration %d: emit_analysis should trigger stop", i)
		}
	}
	if e.prescanRounds != 0 {
		t.Errorf("after 5 emit_analysis observations: prescanRounds = %d, want 0", e.prescanRounds)
	}
	// A single pre-scan tool after the emit observations counts as
	// round 1.
	if sig := observePrescan(e, 5, "grep", &history); sig.StopRequested {
		t.Errorf("first real pre-scan round: unexpected StopRequested (reason=%q)", sig.StopReason)
	}
	if e.prescanRounds != 1 {
		t.Errorf("after first real pre-scan: prescanRounds = %d, want 1", e.prescanRounds)
	}
}

// TestAnalyzer_PrescanBudget_NilLastToolResult is a defensive check
// for the branch where BaseAgent dispatches a PhaseMidLoop
// observation with nil LastToolResult — this happens when the
// iteration had no successful tool execution. Observe must return
// an empty signal and leave the counter alone.
func TestAnalyzer_PrescanBudget_NilLastToolResult(t *testing.T) {
	restoreAnalysisLimits(t)
	tool.SetAnalysisLimits(tool.AnalysisLimits{MaxPrescanRounds: 2})

	e := &analyzerEvaluator{}
	e.BuildInitialInstruction(nil, nil)

	obs := LoopObservation{
		Phase:          PhaseMidLoop,
		Iteration:      0,
		LastToolResult: nil,
	}
	sig := e.Observe(nil, obs)
	if sig.StopRequested || sig.HintRequested || sig.Progress {
		t.Errorf("nil LastToolResult: got non-empty signal %+v, want zero value", sig)
	}
	if e.prescanRounds != 0 {
		t.Errorf("nil LastToolResult: prescanRounds = %d, want 0", e.prescanRounds)
	}
}

// TestAnalyzer_PrescanBudget_DisabledWithZero verifies that setting
// MaxPrescanRounds=0 disables the runtime gate entirely: five
// consecutive pre-scan rounds all return empty signals even though
// each advances the internal counter.
func TestAnalyzer_PrescanBudget_DisabledWithZero(t *testing.T) {
	restoreAnalysisLimits(t)
	tool.SetAnalysisLimits(tool.AnalysisLimits{MaxPrescanRounds: 0})

	e := &analyzerEvaluator{}
	e.BuildInitialInstruction(nil, nil)
	var history []types.ToolResult

	tools := []string{"grep", "repo_map", "list_files", "grep", "repo_map"}
	for i, name := range tools {
		sig := observePrescan(e, i, name, &history)
		if sig.StopRequested {
			t.Errorf("iteration %d (%s) with gate disabled: unexpected StopRequested (reason=%q)",
				i, name, sig.StopReason)
		}
	}
	if e.prescanRounds != len(tools) {
		t.Errorf("counter should still advance even with gate disabled; got %d, want %d",
			e.prescanRounds, len(tools))
	}
}

// TestAnalyzer_Observe_AppendsTypedPrescanCorpusToMutable pins the runtime
// data channel: successful pre-scan observations flow through typed carriers,
// not rendered ToolResult.Summary text, before emit_analysis and the post-hoc
// probe read Mutable.PrescanSummaryBlob.
func TestAnalyzer_Observe_AppendsTypedPrescanCorpusToMutable(t *testing.T) {
	restoreAnalysisLimits(t)
	tool.SetAnalysisLimits(tool.AnalysisLimits{MaxPrescanRounds: 5})

	mu := types.NewMutableState("explain the analyzer path")
	ctx := &types.AgentContext{Stage: types.StageAnalyze, Mutable: mu}
	e := &analyzerEvaluator{}
	e.BuildInitialInstruction(ctx, nil) // resets prescanRounds + blob

	var history []types.ToolResult
	fireOne := func(result types.ToolResult) {
		history = append(history, result)
		e.Observe(ctx, LoopObservation{
			Phase:          PhaseMidLoop,
			Iteration:      len(history) - 1,
			LastToolResult: &history[len(history)-1],
			AllToolResults: history,
		})
	}

	fireOne(types.ToolResult{
		ToolName: "grep",
		Success:  true,
		Summary:  "ShouldNotAppear Internal/Agent/Analyzer.go",
		PathDiscovery: &types.ToolPathDiscovery{
			Kind:           types.ToolPathDiscoveryKindGrep,
			Pattern:        "NewAnalyzerAgent",
			Path:           "internal/agent",
			FilesOnly:      true,
			ResultCount:    1,
			CandidateFiles: []string{"internal/agent/analyzer.go"},
		},
	})
	fireOne(types.ToolResult{
		ToolName: "repo_map",
		Success:  true,
		Summary:  "ShouldNotAppear Symbol: NewAnalyzerAgent in prose",
		Observations: []types.ObservationRecord{{
			ID:        "nav-1",
			Producer:  "repo_map",
			Predicate: types.RepoMapNavigationObservationPredicate,
			Value:     string(types.RepoMapNavigationRouteSourceInventory),
			Object:    "source_inventory",
			SourceRef: types.ObservationSourceRef{Path: "internal/agent/analyzer.go"},
		}},
		SourceInventory: &types.SourceInventoryObservation{
			Active: true,
			Sets: []types.SourceInventoryObservationSet{{
				Role:     types.AnswerCandidateRoleType,
				Complete: true,
				Members: []types.SourceInventoryObservationMember{{
					Name:       "NewAnalyzerAgent",
					File:       "internal/agent/analyzer.go",
					Line:       42,
					SupportRef: "internal/agent/analyzer.go:42",
				}},
			}},
		},
	})

	blob := mu.PrescanSummaryBlob()
	if blob == "" {
		t.Fatal("blob should not be empty after two pre-scan observations")
	}
	// Lowercase-at-write invariant.
	if strings.Contains(blob, "Analyzer.go") {
		t.Errorf("blob should be lowercased; got %q", blob)
	}
	if !strings.Contains(blob, "analyzer.go") {
		t.Errorf("blob missing typed candidate path content; got %q", blob)
	}
	if !strings.Contains(blob, "newanalyzeragent") {
		t.Errorf("blob missing typed repo_map/source_inventory content; got %q", blob)
	}
	if strings.Contains(blob, "shouldnotappear") {
		t.Errorf("rendered tool Summary must not enter pre-scan corpus; got %q", blob)
	}
	// Non-pre-scan tool results should NOT advance the blob.
	nonPrescan := types.ToolResult{ToolName: "emit_analysis", Success: true, Summary: "ShouldNotAppear"}
	history = append(history, nonPrescan)
	e.Observe(ctx, LoopObservation{
		Phase:          PhaseMidLoop,
		LastToolResult: &history[len(history)-1],
		AllToolResults: history,
	})
	if strings.Contains(mu.PrescanSummaryBlob(), "shouldnotappear") {
		t.Errorf("emit_analysis summary should NOT be appended to the blob")
	}
}

// TestAnalyzer_BuildInitialInstruction_ResetsPrescanBlob pins the
// per-dispatch reset invariant: cross-dispatch retries must start
// with a fresh blob so state doesn't leak between analyze attempts.
func TestAnalyzer_BuildInitialInstruction_ResetsPrescanBlob(t *testing.T) {
	mu := types.NewMutableState("question")
	mu.AppendPrescanSummary("stale data from prior dispatch")
	if mu.PrescanSummaryBlob() == "" {
		t.Fatal("setup invariant: blob should start non-empty")
	}

	e := &analyzerEvaluator{prescanRounds: 99}
	ctx := &types.AgentContext{Stage: types.StageAnalyze, Mutable: mu}
	e.BuildInitialInstruction(ctx, nil)

	if e.prescanRounds != 0 {
		t.Errorf("prescanRounds not reset, got %d", e.prescanRounds)
	}
	if got := mu.PrescanSummaryBlob(); got != "" {
		t.Errorf("PrescanSummaryBlob not reset, got %q", got)
	}
}

// TestAnalyzer_ParseOutput_SurfacesQualityProbe exercises the
// end-to-end post-hoc probe: Observe appends pre-scan summaries,
// ParseOutput reads them via computeQualityProbeFromContext and
// writes the result to StageOutput.Data under
// `analysis_quality_probe`.
func TestAnalyzer_ParseOutput_SurfacesQualityProbe(t *testing.T) {
	restoreAnalysisLimits(t)
	tool.SetAnalysisLimits(tool.AnalysisLimits{MaxPrescanRounds: 5})

	mu := types.NewMutableState("how does emit_analysis work")
	mu.SetRequestModel(types.RequestModel{
		Intent:     types.IntentExplain,
		Scenario:   types.ScenarioArchitectureExplain,
		Complexity: types.ComplexityModerate,
		AnalyzerHints: types.AnalyzerHints{
			Keywords: []string{"emit", "analyzer", "probe", "missing"},
			Entities: []string{"EmitAnalysis", "Analyzer", "Missing"},
		},
	})
	// Seed the blob so the probe has something to match against.
	mu.AppendPrescanSummary("internal/tool/emit_analysis.go\ninternal/agent/analyzer.go\n")

	ctx := &types.AgentContext{
		Stage:     types.StageAnalyze,
		Objective: "how does emit_analysis work",
		Mutable:   mu,
	}

	e := &analyzerEvaluator{prescanRounds: 2}
	out, err := e.ParseOutput(ctx, nil, []types.ToolResult{emitResult(true)}, nil)
	if err != nil {
		t.Fatalf("ParseOutput: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(out.Data, &decoded); err != nil {
		t.Fatalf("Data is not JSON: %v", err)
	}
	probeRaw, ok := decoded["analysis_quality_probe"]
	if !ok {
		t.Fatal("Data missing analysis_quality_probe key")
	}
	probe, ok := probeRaw.(map[string]any)
	if !ok {
		t.Fatalf("analysis_quality_probe = %T(%v), want map", probeRaw, probeRaw)
	}
	// `emit` and `analyzer` appear in the blob → 2 of 4 keywords → 0.5.
	// `probe` and `missing` do not appear.
	if got := probe["keyword_hits"]; got != float64(2) {
		t.Errorf("keyword_hits = %v, want 2", got)
	}
	if got := probe["keyword_total"]; got != float64(4) {
		t.Errorf("keyword_total = %v, want 4", got)
	}
	if got := probe["keyword_hit_ratio"]; got != 0.5 {
		t.Errorf("keyword_hit_ratio = %v, want 0.5", got)
	}
	// `EmitAnalysis` (as substring `emitanalysis`? no — blob contains
	// `emit_analysis.go` which substring-matches "emit" but not
	// "emitanalysis"). `Analyzer` matches. `Missing` doesn't.
	// So 1 of 3 entities hit → ~0.33.
	if got := probe["entity_hits"]; got != float64(1) {
		t.Errorf("entity_hits = %v, want 1 (only `Analyzer` substring-matches the blob)", got)
	}
	if got := probe["entity_total"]; got != float64(3) {
		t.Errorf("entity_total = %v, want 3", got)
	}
	if got := probe["prescan_rounds"]; got != float64(2) {
		t.Errorf("prescan_rounds = %v, want 2", got)
	}
}

// TestAnalyzer_ParseOutput_SurfacesPrescanDiagnostics fires three
// pre-scan observations (exceeding the budget of 2), then runs
// ParseOutput and asserts the new diagnostic keys appear on
// StageOutput.Data with the correct values. This pins the end-to-end
// contract the operator reads from StageOutput.Data.
func TestAnalyzer_ParseOutput_SurfacesPrescanDiagnostics(t *testing.T) {
	restoreAnalysisLimits(t)
	tool.SetAnalysisLimits(tool.AnalysisLimits{MaxPrescanRounds: 2})

	e := &analyzerEvaluator{}
	e.BuildInitialInstruction(nil, nil)
	var history []types.ToolResult
	for i, name := range []string{"grep", "repo_map", "list_files"} {
		_ = observePrescan(e, i, name, &history)
	}
	if e.prescanRounds != 3 {
		t.Fatalf("setup invariant: prescanRounds = %d, want 3", e.prescanRounds)
	}

	// Drive ParseOutput on the SAME evaluator instance so the counter
	// state survives into the diagnostic.
	mu := types.NewMutableState("runtime gate diagnostics")
	mu.SetRequestModel(types.RequestModel{
		Intent:     types.IntentExplain,
		Scenario:   types.ScenarioArchitectureExplain,
		Complexity: types.ComplexityModerate,
	})
	ctx := &types.AgentContext{
		Stage:     types.StageAnalyze,
		Objective: "runtime gate diagnostics",
		Mutable:   mu,
	}
	out, err := e.ParseOutput(ctx, nil, history, nil)
	if err != nil {
		t.Fatalf("ParseOutput: %v", err)
	}
	if out == nil {
		t.Fatal("ParseOutput returned nil")
	}
	var decoded map[string]any
	if err := json.Unmarshal(out.Data, &decoded); err != nil {
		t.Fatalf("StageOutput.Data is not valid JSON: %v", err)
	}
	if got := getFloat(t, decoded, "analysis_prescan_rounds"); got != 3 {
		t.Errorf("analysis_prescan_rounds = %v, want 3", got)
	}
	if got := getBool(t, decoded, "analysis_prescan_budget_exhausted"); !got {
		t.Errorf("analysis_prescan_budget_exhausted = false, want true")
	}
}
