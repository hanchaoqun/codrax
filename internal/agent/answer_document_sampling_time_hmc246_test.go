package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC246SamplingTimeActualFinalizer(t *testing.T) {
	ctx := samplingSelectionAnalysisContext(t)
	var body strings.Builder
	for i := 0; i < 40; i++ {
		fmt.Fprintf(&body, "document-worker-11 (10) [000] .... 1.%03d000: perf_sample: pid=10 tid=11 thread_comm=document-worker cpu=0 cpu_known=true sample_weight=1 event=cycles symbol=work dso=/app/A.so source=raw sample_kind=unknown\n", 100+i)
	}
	for _, ts := range []string{"1.800000", "1.900000"} {
		fmt.Fprintf(&body, "document-worker-11 (10) [001] .... %s: perf_sample: pid=10 tid=11 thread_comm=document-worker cpu=1 cpu_known=true sample_weight=10 event=cycles symbol=later dso=/app/B.so source=raw sample_kind=unknown\n", ts)
	}
	path := filepath.Join(t.TempDir(), "time-facts.perftrace")
	if err := os.WriteFile(path, []byte(body.String()), 0600); err != nil {
		t.Fatal(err)
	}
	ctx.AttachedHitrace, ctx.AttachedHitraceSource = body.String(), path
	var results []types.ToolResult
	for _, view := range []string{"perf_stats", "event_search"} {
		params := map[string]any{"source": "path", "path": path, "view": view, "pid": 11, "time_start": 1, "time_end": 2, "limit": 100}
		if view == "perf_stats" {
			params["pattern"] = "thread_comm=document-worker"
		}
		raw, _ := json.Marshal(params)
		r, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), raw)
		if err != nil || !r.Success {
			t.Fatalf("public %s: %v %s", view, err, r.Summary)
		}
		results = append(results, r)
	}
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: results})
	prompt := dependencyObservationMessages(t, ctx)
	_, facts, found := strings.Cut(prompt, "### 已核对的测量表 / Verified measurement tables")
	if !found {
		t.Fatal("actual finalizer omitted native sampling facts")
	}
	if end := strings.Index(facts, "\n### "); end >= 0 {
		facts = facts[:end]
	}
	for _, want := range []string{"热点记录时刻包络（秒）", "查询总体记录时刻包络（秒）", "[1.100000, 1.139000]", "[1.100000, 1.900000]", "未确立（样本类型未知）", "不是连续执行"} {
		if !strings.Contains(facts, want) {
			t.Errorf("actual finalizer lost full-set time/state fact %q", want)
		}
	}
	if !strings.Contains(prompt, "prompt_member_rows=32/32") || !strings.Contains(prompt, `"prompt_rows_omitted":10`) {
		t.Fatal("fixture did not exercise the actual 32-row preview budget")
	}
	if !strings.Contains(prompt, "display_rows are preview only, not population time bounds") {
		t.Fatal("member preview does not distinguish its time scope from the native population")
	}
	if decided, allowed := types.RuntimeTraceReportShapeAuthority(&ctx.AnalysisIR.RequestModel); !decided || allowed {
		t.Fatal("sampling bounds acquired causal authority")
	}
}
