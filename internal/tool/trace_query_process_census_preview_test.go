package tool

import (
	"math"
	"os"
	"strings"
	"testing"
)

func TestTraceQueryProcessCensusPreviewKeepsAccountAndCaliberTogether(t *testing.T) {
	trace := strings.NewReplacer("com.baidu.tieba", "org.example.reader", "NetworkService", "ContentService").Replace(wsrB3CensusTraceText())
	for _, tc := range []struct {
		name string
		view string
		pid  int
	}{
		{"process owner", "window_stats", 59566},
		{"member TID", "window_stats", 60595},
		{"global has no census", "window_stats", 0},
		{"composite keeps payload-only census", "root_cause_rank", 59566},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, path := businessRefTestContext(t, trace)
			params := map[string]any{"path": path, "view": tc.view, "time_start": 10, "time_end": 10.2, "trace_flavor": "harmony_hitrace"}
			if tc.pid > 0 {
				params["pid"] = tc.pid
			}
			result := businessRefTestQuery(t, ctx, params)
			payload := businessSpanSchedulerPublicPayload(t, result)
			if payload.WindowStats == nil {
				t.Fatal("public query omitted the window stats payload")
			}
			census := payload.WindowStats.ProcessDomainCensus
			if tc.pid == 0 {
				if census != nil || strings.Contains(result.Summary, "- process_domain_census") {
					t.Fatal("target-less query acquired a process census")
				}
				return
			}
			if census == nil || census.Process.PID != 59566 || census.Target.PID != tc.pid || census.ThreadCount != 39 ||
				census.RunningThreadCount != 10 || math.Abs(census.TotalRunningMs-49.135) > 1e-6 || census.FoldedThreadCount != 2 || math.Abs(census.FoldedRunningMs-6) > 1e-6 {
				t.Fatalf("public query changed process membership or pre-cap totals: %+v", census)
			}
			if tc.view != "window_stats" {
				if strings.Contains(result.Summary, "- process_domain_census") {
					t.Fatal("census display expanded into a composite view")
				}
				if !strings.Contains(result.Summary, "root_cause_rank_preview status=") {
					t.Fatal("composite lost its existing rank preview")
				}
				return
			}
			if !strings.HasSuffix(result.RawRef, ".txt") {
				t.Fatalf("fixture did not exercise public StoreBlob: %q", result.RawRef)
			}
			raw, err := os.ReadFile(result.RawRef)
			if err != nil {
				t.Fatal(err)
			}
			if len(raw) <= MaxInlineBytes || string(raw) == result.Summary {
				t.Fatal("fixture did not cross the unchanged blob preview budget")
			}
			var block strings.Builder
			writeTraceProcessDomainCensus(&block, census)
			if !strings.Contains(result.Summary, block.String()) {
				t.Errorf("public preview separated census values from their roster/fold/caliber (raw=%d preview=%d)", len(raw), len(result.Summary))
			}
			if strings.Count(string(raw), "- process_domain_census(进程域普查)") != 1 ||
				strings.Count(string(raw), "their threads= counts survivors, not the process") != 1 {
				t.Fatal("preview repair duplicated the account or its scope guidance")
			}
			for _, want := range []string{
				"process=org.example.reader-59566 threads=39 running_threads=10 running_total=49.135cpu·ms",
				"process_domain_census_thread ContentService-60595 running=13.135ms cpus=0,1,2",
				"their threads= counts survivors, not the process",
				"跨线程合计为 cpu·ms,不可当作墙钟耗时",
			} {
				if !strings.Contains(result.Summary, want) {
					t.Errorf("public census preview lost %q", want)
				}
			}
		})
	}
}
