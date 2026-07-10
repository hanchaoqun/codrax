package tool

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestTraceQueryTypedObservationsUsePhysicalBundleSupportRefs(t *testing.T) {
	offset, slope := 20.0, 1.0
	result := tracequery.Result{
		View:       "event_search",
		SourcePath: "/captures/run.tracebundle.json",
		TraceArtifacts: []tracequery.TraceArtifactSource{
			{
				SourcePath:          "/captures/run.systrace",
				Kind:                "systrace",
				TimeDomain:          "trace_seconds",
				CanonicalTimeDomain: "trace_seconds",
				LocalLineCount:      10,
				CausalCompatible:    true,
				ClockAlignment:      tracequery.TraceClockAlignmentIdentity,
			},
			{
				SourcePath:          "/captures/run.perftrace",
				Kind:                "perftrace",
				TimeDomain:          "perf_event_time",
				CanonicalTimeDomain: "trace_seconds",
				VirtualLineBase:     100,
				LocalLineCount:      10,
				CausalCompatible:    true,
				ClockAlignment:      tracequery.TraceClockAlignmentAffine,
				ClockCalibrated:     true,
				ClockOffsetSec:      &offset,
				ClockSlope:          &slope,
			},
		},
		EvidencePack: []tracequery.EvidenceFact{
			{Subject: "hot", Predicate: "perf_sample", Summary: "hot sample", LineStart: 102, LineEnd: 102, Confidence: 0.9},
			{Subject: "joined", Predicate: "correlation", Summary: "cross-artifact correlation", LineStart: 2, LineEnd: 105, Confidence: 0.8},
		},
	}

	records := traceQueryTypedObservations(result, "attached_trace", "payload.json", "raw.txt", "", time.Unix(1, 0))
	if len(records) != 2 {
		t.Fatalf("records=%+v", records)
	}
	perf := records[0]
	if perf.SourceRef.Path != result.SourcePath {
		t.Fatalf("bundle path must remain the capture/partition identity: %+v", perf.SourceRef)
	}
	if perf.SourceRef.TimeDomain != "perf_event_time" || perf.SourceRef.CanonicalTimeDomain != "trace_seconds" {
		t.Fatalf("clock provenance missing: %+v", perf.SourceRef)
	}
	if perf.SourceRef.ClockAlignment != tracequery.TraceClockAlignmentAffine || !perf.SourceRef.ClockCalibrated || perf.SourceRef.ClockOffsetSec == nil || *perf.SourceRef.ClockOffsetSec != 20 || perf.SourceRef.ClockSlope == nil || *perf.SourceRef.ClockSlope != 1 {
		t.Fatalf("affine coefficient provenance missing: %+v", perf.SourceRef)
	}
	formatted := types.FormatObservationSourceRef(perf.SourceRef, 120)
	for _, want := range []string{"clock_alignment=affine", "clock_calibrated=true", "clock_offset_sec=20", "clock_slope=1"} {
		if !strings.Contains(formatted, want) {
			t.Fatalf("prompt source provenance missing %q: %s", want, formatted)
		}
	}
	if len(perf.SupportRefs) != 1 || perf.SupportRefs[0] != "/captures/run.perftrace:2" {
		t.Fatalf("support ref must address the physical artifact/local line: %+v", perf.SupportRefs)
	}
	if !strings.Contains(perf.Span.Selector, "/captures/run.perftrace:2-2[perf_event_time]") {
		t.Fatalf("reversible artifact span missing: %+v", perf.Span)
	}

	joined := records[1]
	if len(joined.SupportRefs) != 2 || joined.SupportRefs[0] != "/captures/run.systrace:2-10" || joined.SupportRefs[1] != "/captures/run.perftrace:1-5" {
		t.Fatalf("cross-artifact fact must retain both physical spans: %+v", joined.SupportRefs)
	}
	if joined.SourceRef.TimeDomain != "multiple_aligned_domains" || joined.SourceRef.CanonicalTimeDomain != "trace_seconds" {
		t.Fatalf("cross-domain-but-aligned fact must remain explicit: %+v", joined.SourceRef)
	}
	if joined.SourceRef.ClockAlignment != "multiple" || joined.SourceRef.ClockOffsetSec != nil || joined.SourceRef.ClockSlope != nil {
		t.Fatalf("cross-artifact fact must not claim one artifact's affine coefficients: %+v", joined.SourceRef)
	}
}

func TestTraceQueryLargePatternCompositeFallsBackToIndexedPath(t *testing.T) {
	oldThreshold := traceQueryWindowedIndexMinBytes
	traceQueryWindowedIndexMinBytes = 1
	t.Cleanup(func() { traceQueryWindowedIndexMinBytes = oldThreshold })

	dir := t.TempDir()
	systrace := filepath.Join(dir, "fallback.systrace")
	perftrace := filepath.Join(dir, "fallback.perftrace")
	bundle := filepath.Join(dir, "fallback.tracebundle.json")
	if err := os.WriteFile(systrace, []byte(strings.Join([]string{
		`app-20 (20) [001] .... 10.000000: print: B|20|TargetFrame`,
		`app-20 (20) [001] .... 10.010000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=20 next_prio=53`,
		`app-20 (20) [001] .... 10.020000: sched_switch: prev_comm=app prev_pid=20 prev_prio=53 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120`,
		`app-20 (20) [001] .... 10.030000: print: E|20`,
	}, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(perftrace, []byte(`app-20 (20) [001] .... 10.015000: perf_sample: cpu=1 pid=20 tid=20 period=1 event=cpu-cycles symbol=Draw dso=libapp.so source=test`), 0o644); err != nil {
		t.Fatal(err)
	}
	manifest := `{
  "version":"test",
  "systrace":"fallback.systrace",
  "artifacts":[
    {"type":"systrace","path":"fallback.systrace"},
    {"type":"perftrace","path":"fallback.perftrace","perf_capability":{"time_domain":"trace_seconds","trace_query_ready":true}}
  ],
  "perf_clock_alignments":[
    {"artifact_path":"fallback.perftrace","perf_time_domain":"trace_seconds","trace_time_domain":"trace_seconds","confidence":"same_domain","calibrated":false}
  ]
}`
	if err := os.WriteFile(bundle, []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if !tracequery.TracePathRequiresCompositeIndex(bundle) {
		t.Fatal("fixture must exercise the composite streaming guard")
	}

	params, err := json.Marshal(map[string]any{
		"source":  "path",
		"path":    bundle,
		"view":    "window_stats",
		"pid":     20,
		"pattern": "TargetFrame",
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := (&TraceQuery{}).Execute(&types.BusContext{RepoRoot: dir, WorkDir: dir}, params)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Success {
		t.Fatalf("composite pattern query must fall back to indexed execution: %s", result.Summary)
	}
	if strings.Contains(result.Summary, "requires a single physical artifact") || strings.Contains(result.Summary, "failed to locate") {
		t.Fatalf("composite streaming guard leaked as a user-visible failure:\n%s", result.Summary)
	}
	for _, want := range []string{"# Trace Query: window_stats", "trace_artifact", "fallback.systrace", "fallback.perftrace"} {
		if !strings.Contains(result.Summary, want) {
			t.Fatalf("indexed composite result missing %q:\n%s", want, result.Summary)
		}
	}
}

func TestTraceQueryLargeRecipeDiscoverySkipsCompositeStreaming(t *testing.T) {
	oldThreshold := traceQueryLargeRecipeDiscoveryMinBytes
	traceQueryLargeRecipeDiscoveryMinBytes = 1
	t.Cleanup(func() { traceQueryLargeRecipeDiscoveryMinBytes = oldThreshold })

	dir := t.TempDir()
	systrace := filepath.Join(dir, "recipe.systrace")
	perftrace := filepath.Join(dir, "recipe.perftrace")
	bundle := filepath.Join(dir, "recipe.tracebundle.json")
	if err := os.WriteFile(systrace, []byte(`app-20 (20) [001] .... 10.000000: print: C|20|jank_frames|1`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(perftrace, []byte(`app-20 (20) [001] .... 10.000000: perf_sample: cpu=1 pid=20 tid=20 symbol=Draw`), 0o644); err != nil {
		t.Fatal(err)
	}
	manifest := `{
  "version":"test",
  "systrace":"recipe.systrace",
  "artifacts":[
    {"type":"systrace","path":"recipe.systrace"},
    {"type":"perftrace","path":"recipe.perftrace"}
  ]
}`
	if err := os.WriteFile(bundle, []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}

	p := traceQueryParams{View: "recipe", RecipeName: "jank"}
	query := &TraceQuery{}
	ctx := &types.BusContext{RepoRoot: dir, WorkDir: dir}
	for _, path := range []string{bundle, systrace} {
		if !tracequery.TracePathRequiresCompositeIndex(path) {
			t.Fatalf("fixture path must require the composite index: %s", path)
		}
		if result, ok := query.maybeLargeRecipeAutoWindow(ctx, p, path, "path", ""); ok {
			t.Fatalf("recipe auto-window streamed composite path %s: %+v", path, result)
		}
		if result, ok := query.maybeLargeRecipeDiscovery(ctx, p, path, "path"); ok {
			t.Fatalf("recipe discovery streamed composite path %s: %+v", path, result)
		}
	}
}
