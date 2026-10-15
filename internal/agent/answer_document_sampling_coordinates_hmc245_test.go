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

func TestHMC245SamplingRecordedCPUActualFinalizer(t *testing.T) {
	ctx := samplingSelectionAnalysisContext(t)
	body := strings.Join([]string{
		`document-worker-11 (10) [000] .... 1.100000: perf_sample: pid=10 tid=11 thread_comm=document-worker cpu=0 cpu_known=true sample_weight=3 event=cycles symbol=work dso=/app/A.so source=raw sample_kind=unknown`,
		`document-worker-11 (10) [001] .... 1.200000: perf_sample: pid=10 tid=11 thread_comm=document-worker cpu=1 cpu_known=true sample_weight=9 event=cycles symbol=work dso=/app/A.so source=raw sample_kind=unknown`,
		`document-worker-11 (10) [000] .... 1.300000: perf_sample: pid=10 tid=11 thread_comm=document-worker cpu=0 cpu_known=false sample_weight=2 event=cycles symbol=other dso=/app/B.so source=raw sample_kind=unknown`,
	}, "\n") + "\n"
	path := filepath.Join(t.TempDir(), "cpu-facts.perftrace")
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	ctx.AttachedHitrace, ctx.AttachedHitraceSource = body, path
	raw, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "perf_stats", "pid": 11, "time_start": 1, "time_end": 2})
	result, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), raw)
	if err != nil || !result.Success {
		t.Fatalf("actual native query: %v %s", err, result.Summary)
	}
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{result}})
	before, _ := json.Marshal(result.Observations)
	prompt := dependencyObservationMessages(t, ctx)
	_, facts, found := strings.Cut(prompt, "### 已核对的测量表 / Verified measurement tables")
	if !found {
		t.Fatal("actual finalizer omitted measurement lane")
	}
	if end := strings.Index(facts, "\n### "); end >= 0 {
		facts = facts[:end]
	}
	for _, want := range []string{"记录占比", "2/3", "12/14", "记录CPU（不代表执行状态）", "CPU 0", "CPU 1", "未知", "不是热点子集质量", "不等于耗时"} {
		if !strings.Contains(facts, want) {
			t.Errorf("actual finalizer lost %q", want)
		}
	}
	after, _ := json.Marshal(result.Observations)
	if string(before) != string(after) {
		t.Fatal("display mutated native facts")
	}
	if decided, allowed := types.RuntimeTraceReportShapeAuthority(&ctx.AnalysisIR.RequestModel); !decided || allowed {
		t.Fatal("recorded CPU display granted causal authority")
	}
}
