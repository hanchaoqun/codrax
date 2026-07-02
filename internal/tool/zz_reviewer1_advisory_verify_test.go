package tool

// Reviewer-1 verification probe (temporary, NOT to be committed).

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// Residual divergence-A window, tool side: explore-stage bus with a
// required-file-hints-only trace carrier (mirror=false while the tool IS on
// the agent surface). Does the zero-match refinement / no-match body still
// steer to trace_query (pre-change behavior) or fall back to generic grep
// guidance (post-change suppression)?
func TestZZReviewer1_AdvisorySuppression_RequiredFileHintsOnly(t *testing.T) {
	dir := t.TempDir()
	rel := "logs/record_trace.systrace"
	abs := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(abs, []byte("com.app-123 (123) [001] .... 100.000001: sched_switch\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rm := types.RequestModel{}
	rm.AnalyzerHints.RequiredFileHints = []types.RequiredFileHint{{Path: rel}}
	ctx := &types.BusContext{
		RepoRoot:      dir,
		PipelineStage: types.StageExplore,
		AnalysisIR:    &types.AnalysisIR{RequestModel: rm},
	}
	params := grepToolParams{Pattern: "no_such_event", Path: abs}

	if !grepParamsTargetTraceQueryArtifactFile(ctx, params) {
		t.Fatalf("matcher did not classify the file as trace_query artifact — no divergence possible")
	}
	if types.TraceQueryContextActiveFromBus(ctx) {
		t.Fatalf("mirror unexpectedly true — divergence shape not reproduced")
	}
	hint := grepZeroMatchRefinement(ctx, params, abs)
	if hint == nil {
		t.Logf("refinement: nil")
	} else {
		t.Logf("refinement: reason=%q preferred_next_tool=%q", hint.ReasonCode, hint.PreferredNextTool)
	}
	body := grepNoMatchBody(ctx, params)
	t.Logf("no-match body mentions trace_query: %v", strings.Contains(body, "trace_query"))

	advisory := execCommandSearchShapeAdvisory(ctx, "grep -c 'no_such_event' "+abs, "", errExitStatusOne{})
	t.Logf("exec advisory mentions trace_query: %v", strings.Contains(advisory, "trace_query"))
}

type errExitStatusOne struct{}

func (errExitStatusOne) Error() string { return "exit status 1" }
