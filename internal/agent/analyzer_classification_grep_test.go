package agent

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Session 11 G5 — C0' ClassificationGrep + Declarative Classifier
// tests. These lock the Round-aware validator, the budget cap, the
// sidecar isolation, and the reconciler rule table.

// saveAnalysisLimits snapshots the current AnalysisLimits so tests
// that mutate the globals via SetAnalysisLimits can restore cleanly
// (t.Cleanup keeps other tests deterministic under -count=N runs).
func saveAnalysisLimits(t *testing.T) {
	t.Helper()
	prev := tool.CurrentAnalysisLimits()
	t.Cleanup(func() { tool.SetAnalysisLimits(prev) })
}

// makeGrepCall builds an llm.ToolCall targeting the grep tool with
// the given params (marshalled to JSON). Panic on marshal failure —
// the failure would be a test bug, not production.
func makeGrepCall(params map[string]any) llm.ToolCall {
	raw, err := json.Marshal(params)
	if err != nil {
		panic(err)
	}
	return llm.ToolCall{Name: "grep", Params: raw}
}

// TestValidator_Round1StillRejectsFilesOnlyFalse guards the
// evidence-lite boundary: Round 1 (trigger NOT set) must reject
// any files_only=false grep regardless of other state. The
// rejection message must mention "files_only=true" so the LLM
// sees how to recover.
func TestValidator_Round1StillRejectsFilesOnlyFalse(t *testing.T) {
	saveAnalysisLimits(t)
	tool.SetAnalysisLimits(tool.DefaultAnalysisLimits())

	mut := types.NewMutableState("test")
	// Explicitly leave trigger OFF — this is Round 1.
	ctx := &types.AgentContext{
		Stage:   types.StageAnalyze,
		Mutable: mut,
	}
	tc := makeGrepCall(map[string]any{
		"pattern":    "Skill:",
		"path":       "internal/orchestrator/topology.go",
		"files_only": false,
	})

	res := validateAnalyzerPrescanToolCall(ctx, tc)
	if res == nil {
		t.Fatal("Round 1 files_only=false must be rejected, got nil (pass)")
	}
	if res.Success {
		t.Errorf("rejection must have Success=false, got Success=true")
	}
	if !strings.Contains(res.Summary, "files_only=true") {
		t.Errorf("rejection message must instruct retry with files_only=true; got %q", res.Summary)
	}
}

// TestValidator_Round2WithTriggerAllowsFilesOnlyFalse flips the
// ClassificationGrep trigger on MutableState and asserts a
// files_only=false grep now passes the validator. This is the
// positive path that lets Round-2 line-level classification work.
func TestValidator_Round2WithTriggerAllowsFilesOnlyFalse(t *testing.T) {
	saveAnalysisLimits(t)
	tool.SetAnalysisLimits(tool.DefaultAnalysisLimits())

	mut := types.NewMutableState("test")
	mut.SetClassificationGrepTriggered(true)
	ctx := &types.AgentContext{
		Stage:   types.StageAnalyze,
		Mutable: mut,
	}
	tc := makeGrepCall(map[string]any{
		"pattern":    "Skill:",
		"path":       "internal/orchestrator/topology.go",
		"files_only": false,
		"max_count":  20,
	})

	if res := validateAnalyzerPrescanToolCall(ctx, tc); res != nil {
		t.Errorf("Round 2 with trigger ON must pass, got rejection: %q", res.Summary)
	}
}

// TestValidator_BudgetExhaustedRejects asserts that once the
// per-dispatch call budget is spent, the next files_only=false
// grep is blocked with a message pointing at the emit_analysis
// escape hatch.
func TestValidator_BudgetExhaustedRejects(t *testing.T) {
	saveAnalysisLimits(t)
	limits := tool.DefaultAnalysisLimits()
	limits.ClassificationGrepMaxCalls = 2
	tool.SetAnalysisLimits(limits)

	mut := types.NewMutableState("test")
	mut.SetClassificationGrepTriggered(true)
	// Simulate 2 prior admitted calls — budget is exhausted.
	mut.BumpClassificationGrepCall(100)
	mut.BumpClassificationGrepCall(100)

	ctx := &types.AgentContext{Stage: types.StageAnalyze, Mutable: mut}
	tc := makeGrepCall(map[string]any{
		"pattern":    "Skill:",
		"path":       "topology.go",
		"files_only": false,
	})
	res := validateAnalyzerPrescanToolCall(ctx, tc)
	if res == nil {
		t.Fatal("budget-exhausted call must be rejected, got nil (pass)")
	}
	if !strings.Contains(res.Summary, "call budget exhausted") {
		t.Errorf("rejection must mention budget exhaustion, got %q", res.Summary)
	}
}

// TestValidator_DisabledFlagBlocksEvenWithTrigger covers the
// operator-level disable switch: even when trigger+budget say
// "allow", setting ClassificationGrepEnabled=false in codrax.yaml
// must keep the analyzer in evidence-lite mode.
func TestValidator_DisabledFlagBlocksEvenWithTrigger(t *testing.T) {
	saveAnalysisLimits(t)
	limits := tool.DefaultAnalysisLimits()
	limits.ClassificationGrepEnabled = false
	tool.SetAnalysisLimits(limits)

	mut := types.NewMutableState("test")
	mut.SetClassificationGrepTriggered(true)
	ctx := &types.AgentContext{Stage: types.StageAnalyze, Mutable: mut}
	tc := makeGrepCall(map[string]any{
		"pattern":    "Skill:",
		"path":       "topology.go",
		"files_only": false,
	})
	res := validateAnalyzerPrescanToolCall(ctx, tc)
	if res == nil {
		t.Fatal("disabled flag must keep validator in rejecting mode")
	}
	if !strings.Contains(res.Summary, "disabled") {
		t.Errorf("rejection must surface `disabled` reason, got %q", res.Summary)
	}
}

// TestValidator_NonAnalyzerStageIsPassThrough asserts the gate
// does not leak into explore/extract/finalize. The validator
// returns nil (pass) for any non-analyze stage, regardless of
// grep params. This isolation is what lets the explorer continue
// to use line-level grep normally.
func TestValidator_NonAnalyzerStageIsPassThrough(t *testing.T) {
	saveAnalysisLimits(t)
	tool.SetAnalysisLimits(tool.DefaultAnalysisLimits())

	for _, stage := range []types.PipelineStage{
		types.StageExplore, types.StageExtract, types.StageFinalize,
	} {
		ctx := &types.AgentContext{Stage: stage, Mutable: types.NewMutableState("test")}
		tc := makeGrepCall(map[string]any{"pattern": "x", "files_only": false})
		if res := validateAnalyzerPrescanToolCall(ctx, tc); res != nil {
			t.Errorf("stage=%s must pass-through (got rejection: %q)", stage, res.Summary)
		}
	}
}

// TestPostProcess_BumpsBudgetAndAppendsObs drives the post-process
// hook directly: a successful line-level grep in analyze stage with
// the trigger on must bump the call counter AND append one
// ClassificationObs per parseable match line.
func TestPostProcess_BumpsBudgetAndAppendsObs(t *testing.T) {
	saveAnalysisLimits(t)
	tool.SetAnalysisLimits(tool.DefaultAnalysisLimits())

	mut := types.NewMutableState("test")
	mut.SetClassificationGrepTriggered(true)
	ctx := &types.AgentContext{Stage: types.StageAnalyze, Mutable: mut}

	tc := makeGrepCall(map[string]any{
		"pattern":    "Skill:",
		"path":       "internal/orchestrator/topology.go",
		"files_only": false,
	})
	result := &types.ToolResult{
		ToolName: "grep",
		Summary: "[grep: 2 matching lines]\n" +
			"internal/orchestrator/topology.go:19:    StageExplore:  {Agent: types.AgentExplorer, Skill: \"explore-skill\"},\n" +
			"internal/orchestrator/topology.go:20:    StageExtract:  {Agent: types.AgentExtractor, Skill: \"extract-skill\"},\n",
		Success: true,
	}

	analyzerPostProcessToolResult(ctx, tc, result)

	if calls := mut.ClassificationGrepCalls(); calls != 1 {
		t.Errorf("ClassificationGrepCalls=%d, want 1", calls)
	}
	if bytes := mut.ClassificationGrepBytes(); bytes == 0 {
		t.Errorf("ClassificationGrepBytes=%d, want >0", bytes)
	}
	obs := mut.ClassificationObservations()
	if len(obs) != 2 {
		t.Fatalf("ClassificationObservations=%d, want 2 (banner skipped)", len(obs))
	}
	if obs[0].Path != "internal/orchestrator/topology.go" {
		t.Errorf("obs[0].Path=%q, want internal/orchestrator/topology.go", obs[0].Path)
	}
	if obs[0].Line != 19 {
		t.Errorf("obs[0].Line=%d, want 19", obs[0].Line)
	}
	if !strings.Contains(obs[0].Text, "explore-skill") {
		t.Errorf("obs[0].Text=%q, want to contain `explore-skill`", obs[0].Text)
	}
}

// TestPostProcess_RejectsWhenTriggerOff verifies the hook is a
// no-op when the trigger flag is off — crucial for the "Round 1
// files_only=true" happy path to keep producing zero observations.
func TestPostProcess_RejectsWhenTriggerOff(t *testing.T) {
	mut := types.NewMutableState("test")
	// trigger NOT set.
	ctx := &types.AgentContext{Stage: types.StageAnalyze, Mutable: mut}
	tc := makeGrepCall(map[string]any{
		"pattern": "Skill:", "path": "x.go", "files_only": false,
	})
	result := &types.ToolResult{ToolName: "grep", Summary: "x.go:1:hit", Success: true}

	analyzerPostProcessToolResult(ctx, tc, result)

	if obs := mut.ClassificationObservations(); obs != nil {
		t.Errorf("expected no obs captured when trigger off, got %d", len(obs))
	}
	if calls := mut.ClassificationGrepCalls(); calls != 0 {
		t.Errorf("expected 0 calls counted when trigger off, got %d", calls)
	}
}

// TestReconcileFromObservations_AnyQuotedLiteralNudgesShape asserts
// the generic post-audit behaviour: any quoted literal in a
// Round-2 grep observation nudges AnalyzerHints.Shape toward
// "value" without picking a specific AnswerSubject.Kind — that
// kind-specific decision remains the analyzer-stage intent
// inference's job (internal/agent/analyzer_intent.go), which
// already consults the subject taxonomy.
//
// The literal's domain semantics (skill name? agent name?
// handler route?) are deliberately NOT encoded here — reconciler
// stays language- and project-neutral.
func TestReconcileFromObservations_AnyQuotedLiteralNudgesShape(t *testing.T) {
	rm := &types.RequestModel{
		AnalyzerHints: types.AnalyzerHints{Shape: "config_value"},
	}
	obs := []types.ClassificationObs{{
		Pattern: "Skill:",
		Path:    "internal/orchestrator/topology.go",
		Line:    19,
		Text:    `StageExplore:  {Agent: types.AgentExplorer, Skill: "explore-skill"},`,
	}}
	logs := reconcileFromObservations(rm, obs)
	if rm.AnalyzerHints.Shape != "value" {
		t.Errorf("AnalyzerHints.Shape=%q, want value", rm.AnalyzerHints.Shape)
	}
	if len(logs) == 0 {
		t.Errorf("expected ≥1 reconcile log entry, got %d", len(logs))
	}
}

// TestReconcileFromObservations_NoLiteralNoChange asserts that an
// observation line without any quoted literal is a no-op — the
// reconciler only fires when there is structural evidence of a
// literal-shape answer.
func TestReconcileFromObservations_NoLiteralNoChange(t *testing.T) {
	rm := &types.RequestModel{
		AnalyzerHints: types.AnalyzerHints{Shape: "config_value"},
	}
	obs := []types.ClassificationObs{{
		Text: `func Register(...)`, // no quoted literal
	}}
	logs := reconcileFromObservations(rm, obs)
	if rm.AnalyzerHints.Shape != "config_value" {
		t.Errorf("Shape mutated to %q despite no quoted literals", rm.AnalyzerHints.Shape)
	}
	if len(logs) != 0 {
		t.Errorf("expected 0 log entries, got %v", logs)
	}
}

// TestReconcileFromObservations_NoObs_IsNoop covers the empty path:
// when C0' never fires, reconciler must not mutate anything.
func TestReconcileFromObservations_NoObs_IsNoop(t *testing.T) {
	rm := &types.RequestModel{
		AnswerSubject: types.AnswerSubject{Kind: types.SubjectConfigKey},
		AnalyzerHints: types.AnalyzerHints{Shape: "config_value"},
	}
	logs := reconcileFromObservations(rm, nil)
	if len(logs) != 0 {
		t.Errorf("empty obs should produce no log entries, got %v", logs)
	}
	if rm.AnswerSubject.Kind != types.SubjectConfigKey {
		t.Errorf("AnswerSubject.Kind mutated to %s despite empty obs", rm.AnswerSubject.Kind)
	}
	if rm.AnalyzerHints.Shape != "config_value" {
		t.Errorf("AnalyzerHints.Shape mutated to %q despite empty obs", rm.AnalyzerHints.Shape)
	}
}

// TestResetClassificationGrep_ClearsAllFields verifies the per-
// dispatch reset wipes trigger + budget + observations so
// cross-dispatch retries start with a clean slate.
func TestResetClassificationGrep_ClearsAllFields(t *testing.T) {
	mut := types.NewMutableState("test")
	mut.SetClassificationGrepTriggered(true)
	mut.BumpClassificationGrepCall(123)
	mut.AppendClassificationObs(types.ClassificationObs{Pattern: "p", Path: "f", Line: 1, Text: "x"})

	mut.ResetClassificationGrep()

	if mut.ClassificationGrepTriggered() {
		t.Errorf("trigger not reset")
	}
	if mut.ClassificationGrepCalls() != 0 {
		t.Errorf("calls not reset: %d", mut.ClassificationGrepCalls())
	}
	if mut.ClassificationGrepBytes() != 0 {
		t.Errorf("bytes not reset: %d", mut.ClassificationGrepBytes())
	}
	if mut.ClassificationObservations() != nil {
		t.Errorf("observations not reset: %v", mut.ClassificationObservations())
	}
}

// TestPrescanHasDeclarativeCandidate_MatchesAndMisses covers the
// trigger-helper used in analyzerEvaluator.Observe to decide
// whether Round-2 line-level grep should be opened.
func TestPrescanHasDeclarativeCandidate_MatchesAndMisses(t *testing.T) {
	hits := []string{
		"internal/orchestrator/topology.go — 1 match",
		"found file: internal/skill/defaults.go",
		"routes.go: 3 matches",
		"[schema: user.json]",
		"matches in internal/wire/init.go",
	}
	misses := []string{
		"internal/agent/explorer.go: 120 matches",
		"no files matched",
		"",
	}
	for _, blob := range hits {
		if !prescanHasDeclarativeCandidate(strings.ToLower(blob)) {
			t.Errorf("expected declarative candidate in blob: %q", blob)
		}
	}
	for _, blob := range misses {
		if prescanHasDeclarativeCandidate(strings.ToLower(blob)) {
			t.Errorf("expected NO declarative candidate in blob: %q", blob)
		}
	}
}
