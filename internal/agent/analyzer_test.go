package agent

import (
	"encoding/json"
	"strings"
	"testing"

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
	}, nil)
	if !strings.Contains(got2, "Retry attempt 2") {
		t.Errorf("attempt 2 directive should report counter 2:\n%s", got2)
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

// TestAnalyzer_Observe_AppendsPrescanSummaryToMutable pins the
// runtime data channel: every successful pre-scan observation must
// flow through to Mutable.PrescanSummaryBlob so emit_analysis and
// the post-hoc probe can read it.
func TestAnalyzer_Observe_AppendsPrescanSummaryToMutable(t *testing.T) {
	restoreAnalysisLimits(t)
	tool.SetAnalysisLimits(tool.AnalysisLimits{MaxPrescanRounds: 5})

	mu := types.NewMutableState("explain the analyzer path")
	ctx := &types.AgentContext{Stage: types.StageAnalyze, Mutable: mu}
	e := &analyzerEvaluator{}
	e.BuildInitialInstruction(ctx, nil) // resets prescanRounds + blob

	var history []types.ToolResult
	fireOne := func(name, summary string) {
		result := types.ToolResult{ToolName: name, Success: true, Summary: summary}
		history = append(history, result)
		e.Observe(ctx, LoopObservation{
			Phase:          PhaseMidLoop,
			Iteration:      len(history) - 1,
			LastToolResult: &history[len(history)-1],
			AllToolResults: history,
		})
	}

	fireOne("grep", "internal/agent/Analyzer.go\ninternal/tool/emit_analysis.go\n")
	fireOne("repo_map", "Symbol: NewAnalyzerAgent in internal/agent/analyzer.go")

	blob := mu.PrescanSummaryBlob()
	if blob == "" {
		t.Fatal("blob should not be empty after two pre-scan observations")
	}
	// Lowercase-at-write invariant.
	if strings.Contains(blob, "Analyzer.go") {
		t.Errorf("blob should be lowercased; got %q", blob)
	}
	if !strings.Contains(blob, "analyzer.go") {
		t.Errorf("blob missing lowercased summary content; got %q", blob)
	}
	if !strings.Contains(blob, "newanalyzeragent") {
		t.Errorf("blob missing repo_map summary content; got %q", blob)
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
