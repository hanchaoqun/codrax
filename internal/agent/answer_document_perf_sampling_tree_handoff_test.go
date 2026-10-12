package agent

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestPerfSamplingTreePublicFinalizerHandoff(t *testing.T) {
	path, _ := filepath.Abs("../../eval/fixtures/hmosperf_lossless_sampling/paths.perftrace")
	dir := t.TempDir()
	start, end := 1.0, 2.0
	ctx := &types.AgentContext{RepoRoot: dir, WorkDir: dir, Language: "zh", AgentName: types.AgentFinalizer, Stage: types.StageFinalize, Mutable: types.NewMutableState("采样树热点"), AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{Language: "zh", PerfTrace: &types.PerfBundle{}, RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end, SourceQuote: "1到2秒"}}}}
	params, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "perf_sampling_tree", "time_start": start, "time_end": end})
	result, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), params)
	if err != nil || !result.Success {
		t.Fatalf("public query: %v %+v", err, result)
	}
	var record types.ObservationRecord
	for _, r := range result.Observations {
		if p, ok := tool.DecodeTracePerfSamplingTree(r); ok {
			record = r
			if p.CohortCount != 3 || p.SampleCount != 5 {
				t.Fatalf("fixture population %+v", p)
			}
		}
	}
	if record.ID == "" {
		t.Fatal("no native tree receipt")
	}
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{result}})
	ledger := types.ObservationLedger{Records: []types.ObservationRecord{record}}
	text := renderAnswerDocPerfSamplingTrees(ctx, ledger)
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	if strings.Contains(prompt, types.TraceNoteKeyPerfSamplingTree+"={") {
		t.Fatal("raw sampling transport JSON duplicated the readable native handoff")
	}
	for _, want := range []string{"sys;root", "/app/A.so", "/app/B.so", "total_weight=35", "unavailable_weight=5", "instructions", "不是实际调用"} {
		if !strings.Contains(text, want) {
			t.Fatalf("finalizer lost %q: %s", want, text)
		}
		if !strings.Contains(prompt, want) {
			t.Fatalf("actual finalizer prompt lost native sampling fact %q", want)
		}
	}
	if len(text) > 64<<10 {
		t.Fatal("unbounded finalizer projection")
	}
	for _, mutate := range []func(*types.ObservationRecord){func(r *types.ObservationRecord) { r.Negative = true }, func(r *types.ObservationRecord) { r.SourceRef.QueryWindowStartTs = 0 }, func(r *types.ObservationRecord) { r.SourceRef.Path = "/different.perftrace" }} {
		bad := record
		mutate(&bad)
		if got := renderAnswerDocPerfSamplingTrees(ctx, types.ObservationLedger{Records: []types.ObservationRecord{bad}}); got != "" {
			t.Fatal("forged/out-of-scope record rendered")
		}
	}
	ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile.TimeStart = new(float64)
	*ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile.TimeStart = 4
	*ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile.TimeEnd = 5
	if got := renderAnswerDocPerfSamplingTrees(ctx, ledger); got != "" {
		t.Fatal("wrong-window tree rendered")
	}
}
