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

// Session-22 follow-up B4: when the LLM rated its shape
// classification at or above the confidence floor (0.7), C0'
// reconcile must defer and leave AnalyzerHints.Shape alone. The
// quoted-literal heuristic is a safety net for hedged classifications,
// not a veto against confident ones. Mirrors the
// kindConfidenceFloorForNarrowing = 0.7 pattern in explorer.go.
func TestReconcileFromObservations_SkipsOnHighShapeConfidence(t *testing.T) {
	rm := &types.RequestModel{
		AnalyzerHints:   types.AnalyzerHints{Shape: "explanation"},
		ShapeConfidence: 0.85, // LLM was confident
	}
	obs := []types.ClassificationObs{{
		Pattern: "TurnA|TurnB",
		Path:    "internal/agent/explorer.go",
		Line:    4186,
		Text:    `ctx.Mutable.SetTurnAArtifacts(artifacts) // "snapshot handoff"`,
	}}
	logs := reconcileFromObservations(rm, obs)
	if rm.AnalyzerHints.Shape != "explanation" {
		t.Errorf("high-confidence shape must survive C0' reconcile; got Shape=%q", rm.AnalyzerHints.Shape)
	}
	if len(logs) != 0 {
		t.Errorf("high-confidence guard should suppress log entries; got %v", logs)
	}
}

// B4 lower-bound: a confidence exactly at the floor still defers.
// Guard uses `>= floor` so the threshold value itself is trusted.
func TestReconcileFromObservations_SkipsAtConfidenceFloor(t *testing.T) {
	rm := &types.RequestModel{
		AnalyzerHints:   types.AnalyzerHints{Shape: "explanation"},
		ShapeConfidence: shapeConfidenceFloorForC0Reconcile, // exactly 0.7
	}
	obs := []types.ClassificationObs{{
		Path: "internal/orchestrator/topology.go", Line: 19,
		Text: `"explore-skill"`,
	}}
	_ = reconcileFromObservations(rm, obs)
	if rm.AnalyzerHints.Shape != "explanation" {
		t.Errorf("confidence = floor (0.7) should still skip reconcile; got Shape=%q", rm.AnalyzerHints.Shape)
	}
}

// B4 low-confidence path: below the floor, C0' still overrides
// hedged classifications. This is the original design intent that
// must survive the confidence-guard addition — the rule exists to
// catch LLMs that were unsure about shape.
func TestReconcileFromObservations_FiresOnLowShapeConfidence(t *testing.T) {
	rm := &types.RequestModel{
		AnalyzerHints:   types.AnalyzerHints{Shape: "config_value"},
		ShapeConfidence: 0.4, // LLM was hedged
	}
	obs := []types.ClassificationObs{{
		Path: "internal/orchestrator/topology.go", Line: 19,
		Text: `"explore-skill"`,
	}}
	logs := reconcileFromObservations(rm, obs)
	if rm.AnalyzerHints.Shape != "value" {
		t.Errorf("low-confidence should allow reconcile; got Shape=%q", rm.AnalyzerHints.Shape)
	}
	if len(logs) == 0 {
		t.Error("expected reconcile log entry when confidence is low")
	}
}

// B4 zero-confidence (LLM declined to rate): treated as low —
// matches the kindConfidenceFloorForNarrowing convention where
// zero is distinct from "genuinely low" but still permits the
// aggressive rule to fire. Without this branch, an LLM that forgot
// to fill shape_confidence would be exempt from every safety net
// the hint chain provides.
func TestReconcileFromObservations_FiresOnZeroConfidence(t *testing.T) {
	rm := &types.RequestModel{
		AnalyzerHints:   types.AnalyzerHints{Shape: "config_value"},
		ShapeConfidence: 0, // LLM declined to rate
	}
	obs := []types.ClassificationObs{{
		Path: "internal/orchestrator/topology.go", Line: 19,
		Text: `"explore-skill"`,
	}}
	_ = reconcileFromObservations(rm, obs)
	if rm.AnalyzerHints.Shape != "value" {
		t.Errorf("zero-confidence must NOT be treated as 'high'; reconcile should fire; got Shape=%q", rm.AnalyzerHints.Shape)
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

// Session-22 follow-up root cause fix: the C0' observation sidecar
// must ignore grep hits that land in test / spec / fixture files.
// A test assertion string (e.g. `"flag=on must write TurnAArtifacts"`
// inside an _test.go file) passes extractQuotedLiterals exactly the
// same way a production declarative literal does, and the
// first-match-wins reconciler cannot tell them apart. Pinning the
// filter here so the m1a regression (shape=explanation silently
// downgraded to shape=value because Round 2 grep hit a _test.go
// file) cannot recur silently.
func TestPostProcess_SkipsTestFileObservations(t *testing.T) {
	saveAnalysisLimits(t)
	tool.SetAnalysisLimits(tool.DefaultAnalysisLimits())

	mut := types.NewMutableState("test")
	mut.SetClassificationGrepTriggered(true)
	ctx := &types.AgentContext{Stage: types.StageAnalyze, Mutable: mut}

	tc := makeGrepCall(map[string]any{
		"pattern":    "TurnA|TurnB",
		"path":       "internal/agent",
		"files_only": false,
	})
	// One production hit + one test-file hit, mirroring the m1a
	// failure shape where a _test.go file quoted literal poisoned
	// the reconciler.
	result := &types.ToolResult{
		ToolName: "grep",
		Summary: "[grep: 2 matching lines]\n" +
			"internal/agent/explorer.go:4186:ctx.Mutable.SetTurnAArtifacts(artifacts)\n" +
			"internal/agent/explorer_evaluator_test.go:469:want := \"flag=on must write TurnAArtifacts\"\n",
		Success: true,
	}

	analyzerPostProcessToolResult(ctx, tc, result)

	obs := mut.ClassificationObservations()
	if len(obs) != 1 {
		t.Fatalf("expected exactly 1 obs (test-file line filtered), got %d: %+v", len(obs), obs)
	}
	if obs[0].Path != "internal/agent/explorer.go" {
		t.Errorf("surviving obs must be the production-source line, got Path=%q", obs[0].Path)
	}
}

// isTestFilePath dispatches cross-language test conventions; pin
// each ecosystem explicitly so a future regression (e.g. the Go arm
// shifting to filepath.Base and breaking on forward-slash paths)
// fails visibly rather than silently re-admitting test files.
func TestIsTestFilePath_CoversLanguageConventions(t *testing.T) {
	testFiles := []string{
		// Go
		"internal/agent/foo_test.go",
		"foo_test.go",
		// Python (suffix + pytest prefix convention)
		"pkg/bar_test.py",
		"pkg/test_bar.py",
		// Ruby
		"spec/foo_spec.rb",
		"test/foo_test.rb",
		// JavaScript / TypeScript — .test.* and .spec.*, plus jsx/tsx.
		"src/foo.test.js",
		"src/foo.spec.ts",
		"src/foo.test.jsx",
		"src/foo.spec.tsx",
		// Java / Kotlin — Test / Tests suffix + Test / IT prefix.
		"src/main/FooTest.java",
		"src/main/FooTests.java",
		"src/main/TestFoo.java",
		"src/main/ITFoo.java",
		"src/main/FooTest.kt",
		// C / C++ / google-test
		"lib/foo_test.cc",
		"lib/foo_test.cpp",
		"lib/foo_unittest.cc",
	}
	for _, p := range testFiles {
		if !isTestFilePath(p) {
			t.Errorf("isTestFilePath(%q) = false, want true", p)
		}
	}
	productionFiles := []string{
		// Production code across the same ecosystems.
		"internal/agent/foo.go",
		"pkg/bar.py",
		"lib/foo.rb",
		"src/foo.js",
		"src/foo.ts",
		"src/main/Foo.java",
		"src/main/Foo.kt",
		"lib/foo.cc",
		// Tricky near-misses: "test" in the middle or as a directory.
		"internal/test/helpers.go", // directory named "test", file itself is production
		"pkg/contest.py",           // not a test file despite "test" substring
	}
	for _, p := range productionFiles {
		if isTestFilePath(p) {
			t.Errorf("isTestFilePath(%q) = true, want false", p)
		}
	}
}
