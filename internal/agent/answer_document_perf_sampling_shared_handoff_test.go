package agent

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestPerfSamplingStatsPublicFinalizerHandoff(t *testing.T) {
	path, err := filepath.Abs("../../eval/fixtures/hmosperf_lossless_sampling/paths.perftrace")
	if err != nil {
		t.Fatal(err)
	}
	start, end := 1.0, 2.0
	dir := t.TempDir()
	ctx := &types.AgentContext{RepoRoot: dir, WorkDir: dir, Language: "zh", AgentName: types.AgentFinalizer, Stage: types.StageFinalize, Mutable: types.NewMutableState("采样路径"), AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{Language: "zh", PerfTrace: &types.PerfBundle{}, RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end, SourceQuote: "1到2秒"}}}}
	var results []types.ToolResult
	var ledger types.ObservationLedger
	for _, view := range []string{"perf_stats", "window_stats", "perf_sampling_tree"} {
		params, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": view, "pid": 11, "time_start": start, "time_end": end})
		result, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), params)
		if err != nil || !result.Success {
			t.Fatalf("%s: %v %+v", view, err, result)
		}
		found := false
		for _, record := range result.Observations {
			p, ok := tool.DecodeTracePerfSamplingTree(record)
			if !ok {
				continue
			}
			found = true
			if p.Status != "available" || p.SampleCount != 5 || p.CohortCount != 3 {
				t.Fatalf("%s: wrong native population %+v", view, p)
			}
			ledger.Records = append(ledger.Records, record)
		}
		if !found {
			t.Fatalf("%s did not hand off the native sampling population", view)
		}
		results = append(results, result)
	}
	before, _ := json.Marshal(ledger)
	text := renderAnswerDocPerfSamplingTrees(ctx, ledger)
	if got := strings.Count(text, "Sampling prefix tree:"); got != 1 {
		t.Fatalf("same-source views rendered %d copies instead of one: %s", got, text)
	}
	after, _ := json.Marshal(ledger)
	if string(before) != string(after) || len(ledger.Records) != 3 {
		t.Fatal("display coalescing changed original receipts")
	}
	for _, order := range [][]types.ObservationRecord{{ledger.Records[2], ledger.Records[1], ledger.Records[0]}, {ledger.Records[0], ledger.Records[0], ledger.Records[1]}} {
		if got := strings.Count(renderAnswerDocPerfSamplingTrees(ctx, types.ObservationLedger{Records: order}), "Sampling prefix tree:"); got != 1 {
			t.Fatalf("receipt order/repetition changed one population's display count: %d", got)
		}
	}
	for _, variation := range []map[string]any{{"sampling_depth": 2}, {"time_end": 1.95}} {
		args := map[string]any{"source": "path", "path": path, "view": "perf_sampling_tree", "pid": 11, "time_start": start, "time_end": end}
		for key, value := range variation {
			args[key] = value
		}
		raw, _ := json.Marshal(args)
		result, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), raw)
		if err != nil || !result.Success {
			t.Fatalf("different native query: %v %+v", err, result)
		}
		other := types.ObservationLedger{Records: append(append([]types.ObservationRecord(nil), ledger.Records...), result.Observations...)}
		if got := strings.Count(renderAnswerDocPerfSamplingTrees(ctx, other), "Sampling prefix tree:"); got != 2 {
			t.Fatalf("distinct scope/display collapsed: %v got=%d", variation, got)
		}
	}
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: results})
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, want := range []string{"sys;root", "/app/A.so", "/app/B.so", "total_weight=35", "unavailable_weight=5", "total_weight=7", "instructions"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("actual stats handoff lost %q", want)
		}
	}
	if strings.Contains(prompt, types.TraceNoteKeyPerfSamplingTree+"={") {
		t.Fatal("raw tree JSON duplicated the native readable handoff")
	}
	// Same-publication conflict still withholds both copies; identical data
	// arriving through another query remains usable and must not be poisoned.
	bad := ledger.Records[0]
	bad.RichNotes = append([]string(nil), bad.RichNotes...)
	for i := range bad.RichNotes {
		bad.RichNotes[i] = strings.ReplaceAll(bad.RichNotes[i], `"display_depth":4`, `"display_depth":3`)
	}
	conflictText := renderAnswerDocPerfSamplingTrees(ctx, types.ObservationLedger{Records: append([]types.ObservationRecord{bad}, ledger.Records...)})
	if got := strings.Count(conflictText, "Sampling prefix tree:"); got != 1 {
		t.Fatalf("independent valid publication lost after one conflicting selector: %s", conflictText)
	}
	if got := renderAnswerDocPerfSamplingTrees(ctx, types.ObservationLedger{Records: []types.ObservationRecord{bad, ledger.Records[0]}}); got != "" {
		t.Fatal("conflicting publication was rehabilitated by display dedupe")
	}
}
