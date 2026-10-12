package tool

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestTraceQueryPerfCohortStatisticalCaliberPhysicalSupportPublic(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"capture.systrace": "app-20 (20) [000] .... 1.000000: print: C|20|frame|1\napp-20 (20) [000] .... 2.000000: print: C|20|frame|2\n",
		"first.perftrace":  "app-20 (20) [000] .... 1.100000: perf_sample: pid=20 tid=20 cpu=0 cpu_known=true period=11 event=cpu-cycles symbol=First source=first sample_kind=on_cpu\n",
		"second.perftrace": "app-20 (20) [000] .... 1.200000: perf_sample: pid=20 tid=20 cpu=0 cpu_known=true period=13 event=cpu-cycles symbol=Second source=second sample_kind=on_cpu\n",
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	bundlePath := filepath.Join(dir, "capture.tracebundle.json")
	writeToolTraceBundleV2Fixture(t, bundlePath, []byte(`{
  "systrace":"capture.systrace",
  "artifacts":[
    {"type":"systrace","path":"capture.systrace"},
    {"type":"perftrace","path":"first.perftrace","perf_capability":{"time_domain":"trace_seconds","thread_identity":"present_valid_sample_pid_tid_only","cpu_identity":"present_valid_sample_cpu_else_unknown","trace_query_ready":true}},
    {"type":"perftrace","path":"second.perftrace","perf_capability":{"time_domain":"trace_seconds","thread_identity":"present_valid_sample_pid_tid_only","cpu_identity":"present_valid_sample_cpu_else_unknown","trace_query_ready":true}}
  ],
  "perf_clock_alignments":[
    {"artifact_path":"first.perftrace","perf_time_domain":"trace_seconds","trace_time_domain":"trace_seconds","confidence":"same_domain","calibrated":false},
    {"artifact_path":"second.perftrace","perf_time_domain":"trace_seconds","trace_time_domain":"trace_seconds","confidence":"same_domain","calibrated":false}
  ]
}`))
	ctx, _, _ := hmc17NamedPathContext(t)
	result := hmc17NamedQuery(t, ctx, map[string]any{"source": "path", "path": bundlePath, "view": "perf_stats", "pid": 20, "time_start": 1, "time_end": 2})
	payload := hmc17NamedPayload(t, result)
	if payload.PerfStats == nil || payload.PerfStats.SampleCount != 2 || len(payload.PerfStats.Cohorts) != 2 || len(payload.PerfStats.TopSymbols) != 0 || payload.PerfStats.TotalPeriod != 0 {
		t.Fatalf("fixture must retain independent physical cohorts without a merged mirror: perf=%+v summary=%s", payload.PerfStats, result.Summary)
	}
	var caliber *types.ObservationRecord
	for i := range result.Observations {
		if result.Observations[i].Predicate == "perf_sample_statistical_caliber" {
			caliber = &result.Observations[i]
			break
		}
	}
	if caliber == nil || caliber.Value != "2" || caliber.Object != "partitioned_same_event_cohorts_only" {
		t.Fatalf("public statistical caliber missing: %+v", caliber)
	}
	if len(caliber.SupportRefs) != 2 || caliber.Span.LineStart <= 0 || caliber.Span.LineStart != caliber.Span.LineEnd {
		t.Fatalf("cohort sources lost or replaced by a cross-source range: %+v", caliber)
	}
	for _, cohort := range payload.PerfStats.Cohorts {
		if cohort.Scope == nil || cohort.Scope.SourcePath == "" {
			t.Fatalf("native cohort source missing: %+v", cohort)
		}
		want := cohort.Scope.SourcePath + ":1"
		if !slices.Contains(caliber.SupportRefs, want) || !strings.Contains(caliber.Span.Selector, cohort.Scope.SourcePath+":1-1[trace_seconds]") {
			t.Fatalf("public support must retain each physical cohort, want %q: %+v", want, caliber)
		}
	}
}
