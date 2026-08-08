package types

import (
	"os"
	"runtime"
	"strings"
	"testing"
)

func TestPipelineStageSourceNarrativePreservesWriteLane(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test source path")
	}
	raw, err := os.ReadFile(strings.TrimSuffix(file, "pipeline_stage_test.go") + "enums.go")
	if err != nil {
		t.Fatalf("read enums.go: %v", err)
	}
	text := string(raw)
	for _, want := range []string{"default", "read-mode lane", "write Auto Pilot lane", "worktree/risk/approval gates"} {
		if !strings.Contains(text, want) {
			t.Fatalf("PipelineStage authority comment missing %q", want)
		}
	}
	if strings.Contains(text, "codrax is a read-only analysis tool") {
		t.Fatal("PipelineStage authority comment must not erase the separate write lane")
	}
}

// TestPipelineStage_IsWrite pins the write-mode stage membership.
// Drift here silently reactivates read-mode artifact propagation
// into planner/coder/verifier prompts — the session-35 regression
// that caused the planner LLM to skip emit_change_plan after
// pattern-matching on the analyzer's <think> preamble.
func TestPipelineStage_IsWrite(t *testing.T) {
	cases := []struct {
		stage PipelineStage
		want  bool
	}{
		// Write-mode stages: IsWrite MUST be true.
		{StageWriteAnalyze, true},
		{StageWriteController, true},
		{StagePlan, true},
		{StageApply, true},
		{StageVerify, true},

		// Read-mode stages: IsWrite MUST be false. StageAnalyze runs
		// inside both pipelines as a classifier and is NOT a write
		// stage — its read-mode inputs are required for correct
		// classification regardless of the outer mode.
		{StageAnalyze, false},
		{StageExplore, false},
		{StageExtract, false},
		{StageFinalize, false},
		{StageLogTriage, false},
		{StagePerfTriage, false},

		// Zero value / unknown strings: conservative false — any
		// future stage must opt in explicitly rather than inherit
		// write-mode by accident.
		{PipelineStage(""), false},
		{PipelineStage("bogus"), false},
		{PipelineStage("PLAN"), false}, // case-sensitive by design
	}
	for _, c := range cases {
		if got := c.stage.IsWrite(); got != c.want {
			t.Errorf("PipelineStage(%q).IsWrite() = %v, want %v", c.stage, got, c.want)
		}
	}
}
