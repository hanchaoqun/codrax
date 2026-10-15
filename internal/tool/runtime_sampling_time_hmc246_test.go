package tool

import (
	"encoding/json"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func samplingTimeFixtureHMC246(t *testing.T) string {
	t.Helper()
	var body strings.Builder
	for i := 0; i < 40; i++ {
		fmt.Fprintf(&body, "worker-11 (10) [000] .... 1.%03d000: perf_sample: pid=10 tid=11 thread_comm=worker cpu=0 cpu_known=true sample_weight=1 event=cycles symbol=work dso=/app/A.so source=raw sample_kind=unknown\n", 100+i)
	}
	for _, ts := range []string{"1.800000", "1.900000"} {
		fmt.Fprintf(&body, "worker-11 (10) [001] .... %s: perf_sample: pid=10 tid=11 thread_comm=worker cpu=1 cpu_known=true sample_weight=10 event=cycles symbol=later dso=/app/B.so source=raw sample_kind=unknown\n", ts)
	}
	body.WriteString("worker-11 (10) [002] .... 2.000000: perf_sample: pid=10 tid=11 thread_comm=worker cpu=2 cpu_known=true sample_weight=900 event=cycles symbol=work dso=/app/A.so source=raw sample_kind=on_cpu\n")
	path := filepath.Join(t.TempDir(), "recorded-time.perftrace")
	if err := os.WriteFile(path, []byte(body.String()), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestHMC246SamplingTimePublicQueryEmitSurfaces(t *testing.T) {
	path := samplingTimeFixtureHMC246(t)
	r, payload := samplingIdentityQueryHMC239(t, path, "perf_stats", map[string]any{"pid": 11, "pattern": "thread_comm=worker", "limit": 1})
	if payload.PerfStats == nil || len(payload.PerfStats.Cohorts) != 1 {
		t.Fatalf("missing population: %+v", payload.PerfStats)
	}
	// Decode the public JSON, so the initial RED compiles before the additive
	// native field exists and proves the actual query path omitted the fact.
	raw, _ := json.Marshal(payload.PerfStats.Cohorts[0])
	var cohort map[string]json.RawMessage
	_ = json.Unmarshal(raw, &cohort)
	if len(cohort["recorded_time"]) == 0 {
		t.Error("public native cohort has no complete recorded-time envelope before Top-N")
	}
	contract := types.BuildRuntimeMeasurementContract(types.ObservationLedgerInput{ToolResults: []types.ToolResult{r}})
	var table types.RuntimeMeasurementTable
	for _, item := range contract.Choices() {
		if item.View == types.RuntimeMeasurementSummary && strings.Contains(item.Label, "/app/A.so") {
			table = item
		}
	}
	if table.ObservationID == "" {
		t.Fatal("missing native hotspot")
	}
	columns := map[string]int{}
	for i, label := range table.Columns {
		columns[label] = i
	}
	for _, label := range []string{"热点记录时刻包络（秒）", "查询总体记录时刻包络（秒）", "执行状态依据"} {
		if _, ok := columns[label]; !ok {
			t.Errorf("native fact surface omitted %q", label)
		}
	}
	if t.Failed() {
		return
	}
	row := table.Rows[0]
	if row[columns["热点记录时刻包络（秒）"]] != "[1.100000, 1.139000]" || row[columns["查询总体记录时刻包络（秒）"]] != "[1.100000, 1.900000]" || row[columns["执行状态依据"]] != "未确立（样本类型未知）" {
		t.Fatalf("preview or Top-N changed complete bounds/state: %v", row)
	}
	if row[4] != "40" || row[6] != "42" || row[7] != "60" {
		t.Fatalf("time display changed existing independent measures: %v", row)
	}
	dir := t.TempDir()
	rm := nativeFactDisplayRequest(types.RequestedAnswerDimensionObservedValue)
	ctx := &types.BusContext{RepoRoot: dir, WorkDir: dir, Mutable: types.NewMutableState("sampling facts"), AnalysisIR: &types.AnalysisIR{RequestModel: rm}}
	ctx.Mutable.SetRequestModel(rm)
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{r}})
	for _, kind := range []types.AnswerBlockKind{types.BlockSummary, types.BlockTable, types.BlockDiagram} {
		blocks := []any{map[string]any{"id": "facts", "kind": kind, "runtime_measurement": map[string]any{"observation_id": table.ObservationID, "view": "summary"}}}
		if kind != types.BlockSummary {
			blocks = append([]any{map[string]any{"id": "lead", "kind": "summary", "text": "Recorded sampling facts."}}, blocks...)
		}
		out := b1659bExecuteAnswer(t, ctx, map[string]any{"blocks": blocks}, false)
		if !out.Success {
			t.Fatalf("public %s emit: %s", kind, out.Summary)
		}
		visible := html.UnescapeString(render.RenderAnswerDocument(ctx.Mutable.AnswerDocumentV2(), "zh"))
		for _, want := range []string{"1.139000", "1.900000", "未确立（样本类型未知）", "不是连续执行"} {
			if !strings.Contains(visible, want) {
				t.Errorf("%s lost producer fact %q", kind, want)
			}
		}
		if strings.Contains(visible, "-->") || strings.Contains(visible, "900/960") {
			t.Fatalf("%s invented a relation or included the half-open endpoint", kind)
		}
	}
}

func TestHMC246SamplingTimePublicSelections(t *testing.T) {
	path := samplingTimeFixtureHMC246(t)
	for _, tc := range []struct {
		name   string
		params map[string]any
		count  int
		first  float64
		last   float64
	}{
		{"all_before_topn", map[string]any{"pid": 11, "limit": 1}, 42, 1.1, 1.9},
		{"explicit_filter", map[string]any{"pid": 11, "pattern": "dso=/app/B.so", "limit": 1}, 2, 1.8, 1.9},
		{"line_filter", map[string]any{"pid": 11, "line_end": 41, "limit": 1}, 41, 1.1, 1.8},
		{"other_target", map[string]any{"pid": 12}, 0, 0, 0},
		{"empty_window", map[string]any{"pid": 11, "time_start": 3, "time_end": 4}, 0, 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, view := range []string{"perf_stats", "window_stats", "trace_perf_bundle"} {
				_, p := samplingIdentityQueryHMC239(t, path, view, tc.params)
				stats := p.PerfStats
				if stats == nil && p.WindowStats != nil {
					stats = p.WindowStats.PerfSamples
				}
				if tc.count == 0 {
					if stats != nil && len(stats.Cohorts) != 0 {
						t.Fatalf("%s minted a time envelope for no selected samples", view)
					}
					continue
				}
				if stats == nil || len(stats.Cohorts) != 1 {
					t.Fatalf("%s lost selected population: %+v", view, stats)
				}
				c := stats.Cohorts[0]
				assertSamplingTimeHMC246(t, c.RecordedTime, tc.count, tc.first, tc.last)
				if c.Scope.Window.EndInclusive {
					t.Fatal("explicit half-open population acquired closed endpoint")
				}
				for _, h := range c.TopSymbols {
					if h.SampleCount == 40 {
						assertSamplingTimeHMC246(t, h.RecordedTime, 40, 1.1, 1.139)
					}
				}
			}
		})
	}
}

func assertSamplingTimeHMC246(t *testing.T, b *tracequery.PerfSampleTimeBounds, n int, first, last float64) {
	t.Helper()
	if b == nil || b.KnownSampleCount != n || b.UnknownSampleCount != 0 || b.FirstTs == nil || b.LastTs == nil || *b.FirstTs != first || *b.LastTs != last {
		t.Fatalf("recorded time lost full selected set: got=%+v want=%d [%g,%g]", b, n, first, last)
	}
}

func TestHMC246SamplingTimeIndependentCohortsAndZero(t *testing.T) {
	body := strings.Join([]string{
		`worker-11 (10) [000] .... 0.000000: perf_sample: pid=10 tid=11 thread_comm=worker cpu=0 cpu_known=true sample_weight=3 event=cycles symbol=work dso=/app/A.so source=raw sample_kind=unknown clock=clockA clock_confidence=assumed`,
		`worker-11 (10) [001] .... 0.100000: perf_sample: pid=10 tid=11 thread_comm=worker cpu=1 cpu_known=true sample_weight=5 event=cycles symbol=work dso=/app/A.so source=raw sample_kind=on_cpu clock=clockA`,
		`worker-11 (10) [001] .... 0.200000: perf_sample: pid=10 tid=11 thread_comm=worker cpu=1 cpu_known=true sample_weight=7 event=cycles symbol=work dso=/app/A.so source=raw sample_kind=off_cpu clock=clockA`,
		`worker-11 (10) [001] .... 0.300000: perf_sample: pid=10 tid=11 thread_comm=worker cpu=1 cpu_known=true sample_weight=100 event=instructions symbol=work dso=/app/A.so source=raw sample_kind=unknown clock=clockA`,
		`worker-11 (10) [001] .... 0.400000: perf_sample: pid=10 tid=11 thread_comm=worker cpu=1 cpu_known=true sample_weight=11 event=cycles symbol=work dso=/app/A.so source=other sample_kind=unknown clock=clockB`,
	}, "\n") + "\n"
	path := filepath.Join(t.TempDir(), "independent.perftrace")
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	r, p := samplingIdentityQueryHMC239(t, path, "perf_stats", map[string]any{"pid": 11, "time_start": 0, "time_end": 1, "limit": 8})
	if p.PerfStats == nil || len(p.PerfStats.Cohorts) != 5 {
		t.Fatalf("same name mixed independent kinds/clocks/events: %+v", p.PerfStats)
	}
	want := map[int64]float64{3: 0, 5: .1, 7: .2, 100: .3, 11: .4}
	for _, c := range p.PerfStats.Cohorts {
		assertSamplingTimeHMC246(t, c.RecordedTime, 1, want[c.TotalPeriod], want[c.TotalPeriod])
		if len(c.TopSymbols) != 1 {
			t.Fatal("same-name hotspot disappeared")
		}
		assertSamplingTimeHMC246(t, c.TopSymbols[0].RecordedTime, 1, want[c.TotalPeriod], want[c.TotalPeriod])
	}
	seen := map[string]bool{}
	for _, table := range types.BuildRuntimeMeasurementContract(types.ObservationLedgerInput{ToolResults: []types.ToolResult{r}}).Choices() {
		if table.View != types.RuntimeMeasurementSummary {
			continue
		}
		for _, row := range table.Rows {
			seen[row[14]] = true
			if row[5] == "3" && (row[12] != "[0.000000, 0.000000]" || row[14] != "未确立（样本类型未知）") {
				t.Fatalf("zero/unknown/recorded CPU lost: %v", row)
			}
		}
	}
	for _, state := range []string{"未确立（样本类型未知）", "已记录on_cpu（仅采样点）", "已记录off_cpu（非阻塞区间）"} {
		if !seen[state] {
			t.Errorf("native sample type interpretation missing %s", state)
		}
	}
}

func TestHMC246SamplingTimeLegacyUnknownDisplay(t *testing.T) {
	zero := 0.0
	for _, tc := range []struct {
		b    *tracequery.PerfSampleTimeBounds
		n    int
		want string
	}{
		{nil, 1, "未知（未提供完整时刻统计）"},
		{&tracequery.PerfSampleTimeBounds{UnknownSampleCount: 1}, 1, "未知（记录时刻不可用）"},
		{&tracequery.PerfSampleTimeBounds{KnownSampleCount: 1, UnknownSampleCount: 1, FirstTs: &zero, LastTs: &zero}, 2, "[0.000000, 0.000000]（时刻已知1/2）"},
		{&tracequery.PerfSampleTimeBounds{KnownSampleCount: 1, FirstTs: &zero, LastTs: &zero}, 2, "未知（未提供完整时刻统计）"},
	} {
		if got := samplingFactRecordedTime(tc.b, tc.n); got != tc.want {
			t.Fatalf("recorded-time unknown policy: got=%s want=%s", got, tc.want)
		}
	}
}
