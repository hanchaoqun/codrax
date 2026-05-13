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

// TestValidator_Round1FilesOnlyFalseAutoTriggers (Fix H, 2026-05-07
// customer report) supersedes the prior pin that required Round 1
// to REJECT files_only=false. Fix H changes the contract: when the
// LLM expresses files_only=false intent in analyze stage, the
// classification_grep trigger auto-fires instead of forcing a
// retry round-trip. The LLM's explicit choice IS the typed signal
// that line content is needed — the system no longer second-guesses
// it. Budget caps (MaxCalls / MaxTotalBytes) still bound the cost.
func TestValidator_Round1FilesOnlyFalseAutoTriggers(t *testing.T) {
	saveAnalysisLimits(t)
	tool.SetAnalysisLimits(tool.DefaultAnalysisLimits())

	mut := types.NewMutableState("test")
	// Explicitly leave trigger OFF — Round 1 / no declarative
	// candidates surfaced by pre-scan.
	if mut.ClassificationGrepTriggered() {
		t.Fatalf("test setup error: trigger should start OFF")
	}
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
	if res != nil {
		t.Fatalf("Fix H: Round 1 files_only=false must auto-trigger and pass; got rejection: %q", res.Summary)
	}
	// The trigger MUST have flipped on as a side effect, so the
	// per-dispatch budget tracking starts from this call.
	if !mut.ClassificationGrepTriggered() {
		t.Errorf("Fix H: classification_grep trigger must be flipped ON after files_only=false intent")
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

// The reconcileFromObservations helper that originally lived here was
// retired together with AnswerShape (PR2 of the terminal-retirement
// plan) — its sole effect was to nudge AnalyzerHints.Shape toward
// "value" on a quoted-literal hit. The classification-grep capture
// path itself remains tested above; the next axis-specific reconciler
// (e.g. AnswerSubject.Kind) will need its own dedicated tests when
// wired.

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

func TestPrescanHasDeclarativeCandidateResults_IgnoresAuxiliaryHits(t *testing.T) {
	history := []types.ToolResult{
		{
			ToolName: "grep",
			Success:  true,
			Summary: "[grep: 2 matching files]\n" +
				"internal/skill/analysis_contract.go\n" +
				"internal/agent/explorer_test.go\n",
		},
	}
	if prescanHasDeclarativeCandidateResults(history, "", "") {
		t.Fatal("auxiliary/test-only declarative hits must not open ClassificationGrep")
	}

	history = append(history, types.ToolResult{
		ToolName: "grep",
		Success:  true,
		Summary:  "[grep: 1 matching files]\ninternal/config/defaults.go\n",
	})
	if !prescanHasDeclarativeCandidateResults(history, "", "") {
		t.Fatal("production declarative hit should open ClassificationGrep")
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
		// ArkTS / HarmonyOS.
		"entry/src/ohosTest/ets/pages/Index.test.ets",
		"src/__tests__/Widget.spec.ets",
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
		"lib/kernel_test.cu",
		"lib/view_test.mm",
		// Proto / Cangjie use the shared test-container and suffix lanes.
		"tests/service.proto",
		"tests/cangjie/feature_test.cj",
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
		"entry/src/main/ets/pages/Index.ets",
		"src/main/Foo.java",
		"src/main/Foo.kt",
		"lib/foo.cc",
		"lib/kernel.cu",
		"pkg/service.proto",
		"src/cangjie/feature.cj",
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
