package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestTraceDependencyWindowPublicModelHandoff(t *testing.T) {
	caseBytes, err := os.ReadFile("../../eval/cases/trace_query_wakeup_causal_io_chain.case")
	if err != nil {
		t.Fatal(err)
	}
	_, trace, ok := strings.Cut(string(caseBytes), "HTRACE='")
	if !ok {
		t.Fatal("trace absent")
	}
	trace, _, ok = strings.Cut(trace, "\n'\n")
	if !ok {
		t.Fatal("trace not closed")
	}
	trace = strings.ReplaceAll(trace, "network", "transport")
	dir := t.TempDir()
	path := filepath.Join(dir, "window.ftrace")
	if err := os.WriteFile(path, []byte(trace), 0600); err != nil {
		t.Fatal(err)
	}
	params, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "root_cause_rank", "pid": 100, "time_start": 2, "time_end": 2.020, "trace_flavor": "harmony_hitrace"})
	result, err := (&tool.TraceQuery{}).Execute(&types.BusContext{RepoRoot: dir, WorkDir: dir}, params)
	if err != nil || !result.Success {
		t.Fatalf("query: %v %s", err, result.Summary)
	}
	ctx := tracePrincipalValueAuthorityTestContext("app-100", 100, result.Observations)
	start, end := 2.0, 2.020
	ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile = &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end, SourceQuote: "2..2.020"}
	ctx.Mutable.SetRequestModel(ctx.AnalysisIR.RequestModel)
	before, _ := json.Marshal(answerDocObservationLedger(ctx))
	got := renderAnswerDocObservationLedger(ctx)
	for _, want := range []string{"dependency analysis window", "not a continuous state interval", "2.002000", "2.016000", "analysis_window=2.001000..2.018000"} {
		if !strings.Contains(got, want) {
			t.Errorf("model handoff missing %q", want)
		}
	}
	for _, record := range result.Observations {
		if record.Subject != "transport-300" || record.Predicate != "wakeup_causal_impact" {
			continue
		}
		for _, zh := range []bool{true, false} {
			line := traceQueryObservationSupplementText(record, zh)
			if !strings.Contains(line, "依赖分析窗口") && !strings.Contains(line, "dependency analysis window") {
				t.Errorf("reader evidence locator mislabels state bounds: %s", line)
			}
		}
	}
	after, _ := json.Marshal(answerDocObservationLedger(ctx))
	if string(before) != string(after) {
		t.Fatal("handoff changed ledger or query scope")
	}
}

func TestTraceStateDrilldownPublicDisjointModelHandoff(t *testing.T) {
	trace, err := os.ReadFile("../tool/testdata/state_drilldown_disjoint.ftrace")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "states.ftrace")
	if err := os.WriteFile(path, trace, 0600); err != nil {
		t.Fatal(err)
	}
	params, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "window_stats", "pid": 100, "time_start": 1, "time_end": 1.020, "trace_flavor": "harmony_hitrace"})
	result, err := (&tool.TraceQuery{}).Execute(&types.BusContext{RepoRoot: dir, WorkDir: dir}, params)
	if err != nil || !result.Success {
		t.Fatalf("query: %v %s", err, result.Summary)
	}
	ctx := tracePrincipalValueAuthorityTestContext("worker-100", 100, result.Observations)
	start, end := 1.0, 1.020
	ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile = &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end, SourceQuote: "1..1.020"}
	ctx.Mutable.SetRequestModel(ctx.AnalysisIR.RequestModel)
	before, _ := json.Marshal(answerDocObservationLedger(ctx))
	got := renderAnswerDocObservationLedger(ctx)
	for _, want := range []string{"cumulative state measurement scope", "not a continuous state interval", "measurement_window=1.000000..1.012000"} {
		if !strings.Contains(got, want) {
			t.Errorf("assembled model handoff missing %q", want)
		}
	}
	found := false
	for _, record := range result.Observations {
		if record.Subject != "worker-100" || record.Predicate != "state_drilldown" || record.Object != "s_sleep" {
			continue
		}
		found = true
		if record.Value != "8.000" || record.Span.StartTs != 1 || record.Span.EndTs != 1.012 {
			t.Fatalf("public cumulative sleep changed: %+v", record)
		}
		for _, zh := range []bool{true, false} {
			line := traceQueryObservationSupplementText(record, zh)
			if !strings.Contains(line, "累计状态统计范围") && !strings.Contains(line, "cumulative state measurement scope") {
				t.Errorf("reader presents accumulated sleep as one interval: %s", line)
			}
			if strings.Contains(line, "主要睡眠段") || strings.Contains(line, "top sleep interval") {
				t.Errorf("typed source label still claims one occurrence: %s", line)
			}
		}
	}
	if !found {
		t.Fatal("public cumulative sleep missing")
	}
	after, _ := json.Marshal(answerDocObservationLedger(ctx))
	if string(before) != string(after) {
		t.Fatal("model or reader handoff changed ledger or explicit scope")
	}
}
