package agent

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Declarative pre-scan tests. The analyze-stage runtime gate is strictly
// evidence-lite: files_only=false grep is rejected before execution, and no
// rendered grep Summary is allowed back into analyzer classification state.

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

// TestValidator_FilesOnlyFalseRejected pins the current
// evidence-lite red line: line-level grep belongs to explore, not analyze.
func TestValidator_FilesOnlyFalseRejected(t *testing.T) {
	saveAnalysisLimits(t)
	tool.SetAnalysisLimits(tool.DefaultAnalysisLimits())

	mut := types.NewMutableState("test")
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
		t.Fatal("files_only=false must be rejected in analyze stage")
	}
	if !strings.Contains(res.Summary, "files_only=true") {
		t.Fatalf("rejection should redirect to files_only=true, got %q", res.Summary)
	}
}

// TestValidator_LegacyMaxCountDoesNotWeakenFilesOnlyBoundary covers stale
// params/config shape: max_count does not create a line-level analyzer lane.
func TestValidator_LegacyMaxCountDoesNotWeakenFilesOnlyBoundary(t *testing.T) {
	saveAnalysisLimits(t)
	tool.SetAnalysisLimits(tool.DefaultAnalysisLimits())

	mut := types.NewMutableState("test")
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

	res := validateAnalyzerPrescanToolCall(ctx, tc)
	if res == nil {
		t.Fatal("files_only=false must be rejected even when legacy trigger is on")
	}
	if !strings.Contains(res.Summary, "files_only=true") {
		t.Fatalf("rejection should redirect to files_only=true, got %q", res.Summary)
	}
}

// TestValidator_LegacyDisabledFlagDoesNotWeakenFilesOnlyBoundary covers the
// config-compat flag: it can no longer create a separate reason because the
// hard files_only boundary is unconditional.
func TestValidator_LegacyDisabledFlagDoesNotWeakenFilesOnlyBoundary(t *testing.T) {
	saveAnalysisLimits(t)
	limits := tool.DefaultAnalysisLimits()
	limits.ClassificationGrepEnabled = false
	tool.SetAnalysisLimits(limits)

	mut := types.NewMutableState("test")
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
	if !strings.Contains(res.Summary, "files_only=true") {
		t.Errorf("rejection must surface files_only=true reason, got %q", res.Summary)
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

// TestPrescanHasDeclarativeCandidate_MatchesAndMisses covers the legacy
// trigger-helper retained for diagnostics/config compatibility.
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
	history := withReadCoverage([]types.ToolResult{
		{
			ToolName: "grep",
			Success:  true,
			Summary: "[grep: 2 matching files]\n" +
				"internal/skill/analysis_contract.go\n" +
				"internal/agent/explorer_test.go\n",
		},
	})
	if prescanHasDeclarativeCandidateResults(history, "", "") {
		t.Fatal("auxiliary/test-only declarative hits must not match the legacy diagnostic trigger")
	}

	history = withReadCoverage(append(history, types.ToolResult{
		ToolName: "grep",
		Success:  true,
		Summary:  "[grep: 1 matching files]\ninternal/config/defaults.go\n",
	}))
	if !prescanHasDeclarativeCandidateResults(history, "", "") {
		t.Fatal("production declarative hit should match the legacy diagnostic trigger")
	}
}
