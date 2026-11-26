package tool

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/traceinput"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestKnownFrameBundleReusesExactWindowStats(t *testing.T) {
	path := b1625TargetOutsideTopNTrace(t)
	dir := t.TempDir()
	material, err := traceinput.Prepare(context.Background(), traceinput.Options{
		InputPath: path, RuntimeAnchor: dir, PreviewBytes: 1024,
	})
	if err != nil {
		t.Fatal(err)
	}
	m := types.NewMutableState("Analyze the frame on thread 42 between 1 and 1.020 seconds")
	m.SetRequestModel(types.RequestModel{
		RuntimeQuestionProfile:      &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeCausalDiagnosis, FrameCausalityRequested: true},
		RuntimeArtifactScopeProfile: requestWindowTestProfile(1, 1.020),
		RuntimeTargetProfile:        &types.RuntimeTargetProfile{Declaration: types.RuntimeTargetDeclarationNamedTarget, SourceQuote: "thread 42"},
		RuntimeTargets:              []types.RuntimeTarget{{Kind: types.RuntimeTargetKindThread, PID: 42, Source: "user_explicit"}},
	})
	ctx := &types.BusContext{
		PipelineStage: types.StageExplore, WorkDir: dir, RepoRoot: dir, Mutable: m,
		AttachedHitrace: material.Preview(), AttachedHitraceSource: path, AttachedTraceMaterial: material,
	}
	query := &TraceQuery{}
	run := func(raw json.RawMessage) types.ToolResult {
		t.Helper()
		result, err := query.Execute(ctx, raw)
		if err != nil || !result.Success {
			t.Fatalf("trace query: %v %s", err, result.Summary)
		}
		return result
	}
	bundle := run(json.RawMessage(`{"view":"frame_root_cause_bundle","pid":42,"time_start":1,"time_end":1.020}`))
	m.AppendDispatchToolResult(bundle)
	reused := run(json.RawMessage(`{"view":"window_stats","pid":42,"time_start":1,"time_end":1.020}`))
	if !strings.Contains(reused.Summary, "Reused window_stats") || reused.RawRef != bundle.RawRef || len(reused.Observations) != 0 {
		t.Fatalf("same scope did not reuse the original reference: %.300s", reused.Summary)
	}
	narrowed := run(json.RawMessage(`{"view":"window_stats","pid":42,"time_start":1,"time_end":1.005}`))
	if strings.Contains(narrowed.Summary, "Reused window_stats") || len(narrowed.Observations) == 0 {
		t.Fatal("a new subwindow must run through the normal evidence producer")
	}
}
