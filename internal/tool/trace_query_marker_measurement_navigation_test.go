package tool

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

const markerNavigationPrefix = "marker_query_navigation role=navigation_only recommended_views="

func TestTraceMarkerNavigationPublicEndpointKinds(t *testing.T) {
	for _, tc := range []struct {
		name, rows, views string
	}{
		{"sync", "1.000000: tracing_mark_write: B|7|OpenDocument\n1.010000: tracing_mark_write: E|7", "window_stats,span_window"},
		{"end_without_begin", "1.010000: tracing_mark_write: E|7", "window_stats,span_window"},
		{"async", "1.000000: tracing_mark_write: S|7|AsyncWork|41\n1.010000: tracing_mark_write: F|7|AsyncWork|41", "span_window"},
		{"mixed", "1.000000: tracing_mark_write: B|7|OpenDocument\n1.001000: tracing_mark_write: S|7|AsyncWork|41\n1.010000: tracing_mark_write: E|7", "window_stats,span_window"},
		{"counter", "1.000000: tracing_mark_write: C|7|OpenDocument|10", ""},
		{"instant", "1.000000: tracing_mark_write: I|7|OpenDocument", ""},
		{"scheduler_name_is_not_marker", "1.000000: sched_wakeup: comm=OpenDocument pid=7 prio=120 target_cpu=001", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "events.systrace")
			var body strings.Builder
			body.WriteString("# tracer: nop\n")
			for _, row := range strings.Split(tc.rows, "\n") {
				body.WriteString("worker-7 (7) [001] .... " + row + "\n")
			}
			if err := os.WriteFile(path, []byte(body.String()), 0600); err != nil {
				t.Fatal(err)
			}
			ctx := &types.BusContext{RepoRoot: dir, WorkDir: dir, Mutable: types.NewMutableState("marker navigation")}
			r := businessRefTestQuery(t, ctx, map[string]any{"path": path, "view": "event_search"})
			payload := businessSpanSchedulerPublicPayload(t, r)
			if len(payload.Events) == 0 {
				t.Fatalf("negative must still publish actual parsed events: %+v", payload)
			}
			if tc.views == "" {
				if strings.Contains(r.Summary, markerNavigationPrefix) {
					t.Fatalf("non-endpoint event advertised marker pairing: %s", r.Summary)
				}
				return
			}
			if !strings.Contains(r.Summary, markerNavigationPrefix+tc.views+"\n") || strings.Count(r.Summary, markerNavigationPrefix) != 1 {
				t.Fatalf("actual query lacks one precise route: %s", r.Summary)
			}
			for _, boundary := range []string{
				"not proof of completed pairing", "parent/child relationships", "inclusive/self duration",
				"Reuse the original query artifact(s), target and explicit time/line bounds",
				"A matched-event envelope is not a requested time window", "missing state data is not zero",
				"does not require another call or grant causal authority",
			} {
				if !strings.Contains(r.Summary, boundary) {
					t.Errorf("navigation lost %q: %s", boundary, r.Summary)
				}
			}
			if tc.name == "async" || tc.name == "mixed" {
				if !strings.Contains(r.Summary, "S/F markers do not establish a synchronous child") {
					t.Fatal("async endpoints acquired synchronous nesting/CPU meaning")
				}
			}
			if payload.WindowStats != nil || len(payload.SpanWindows) != 0 {
				t.Fatalf("navigation fabricated measurements: %+v", payload)
			}
			for _, record := range r.Observations {
				if record.Predicate == types.TraceBusinessTreePredicate || record.Predicate == types.TraceBusinessSpanPredicate {
					t.Fatalf("navigation minted a measured observation: %+v", record)
				}
			}
			before, _ := json.Marshal(payload)
			var b strings.Builder
			writeTraceMarkerQueryNavigation(&b, payload)
			after, _ := json.Marshal(payload)
			if string(before) != string(after) {
				t.Fatal("advisory mutated native event inventory")
			}
			if tc.name == "async" {
				follow := businessRefTestQuery(t, ctx, map[string]any{"path": path, "view": "span_window", "span_name": "AsyncWork", "time_start": 1, "time_end": "1.010000s"})
				measured := businessSpanSchedulerPublicPayload(t, follow)
				if len(measured.SpanWindows) != 1 || measured.SpanWindows[0].Kind != "async" || measured.SpanWindows[0].SchedulerStates != nil {
					t.Fatalf("advertised async route must pair without claiming CPU states: %+v", measured.SpanWindows)
				}
			}
		})
	}
}

func TestTraceMarkerNavigationPublicScopesAndFollowup(t *testing.T) {
	path, err := filepath.Abs("../../eval/fixtures/hmosperf_marker_tree/events.systrace")
	if err != nil {
		t.Fatal(err)
	}
	for _, bounds := range []struct {
		name   string
		params map[string]any
	}{
		{"missing_window", map[string]any{}},
		{"explicit_window", map[string]any{"time_start": "5.000000s", "time_end": "5.012000s"}},
		{"zero_start", map[string]any{"time_start": 0, "time_end": "5.012000s"}},
		{"line_scope", map[string]any{"line_start": 3, "line_end": 12}},
	} {
		t.Run(bounds.name, func(t *testing.T) {
			ctx := &types.BusContext{RepoRoot: t.TempDir(), WorkDir: t.TempDir(), Mutable: types.NewMutableState("scope")}
			params := map[string]any{"path": path, "view": "event_search", "event_types": []string{"trace_mark"}, "pid": 700}
			for key, value := range bounds.params {
				params[key] = value
			}
			r := businessRefTestQuery(t, ctx, params)
			if !strings.Contains(r.Summary, markerNavigationPrefix+"window_stats,span_window") {
				t.Fatalf("discovery did not publish measurement route: %s", r.Summary)
			}
			payload := businessSpanSchedulerPublicPayload(t, r)
			if payload.EventSearchCoverage == nil || payload.SourcePath != path {
				t.Fatalf("source/scope was not preserved: %+v", payload)
			}
			wantScope := map[string]string{
				"missing_window":  "line_start= line_end= time_start= time_end=",
				"explicit_window": "time_start=5.000000 time_end=5.012000",
				"zero_start":      "selected_window=0.000000..5.012000 seconds",
				"line_scope":      "line_start=3 line_end=12 time_start= time_end=",
			}[bounds.name]
			if !strings.Contains(r.Summary, wantScope) {
				t.Fatalf("navigation changed caller's explicit/missing bounds (%q): %s", wantScope, r.Summary)
			}
			if bounds.name == "zero_start" && (payload.TimeStart != 0 || payload.TimeEnd != 5.012) {
				t.Fatalf("zero-time selection was widened or replaced by the matched envelope: %+v", payload)
			}
			if bounds.name != "explicit_window" {
				return
			}
			follow := businessRefTestQuery(t, ctx, map[string]any{"path": path, "view": "window_stats", "pid": 700, "time_start": "5.000000s", "time_end": "5.012000s"})
			measured := businessSpanSchedulerPublicPayload(t, follow)
			if measured.WindowStats == nil || measured.WindowStats.BusinessTree == nil || measured.WindowStats.BusinessTree.NodeCount != 4 {
				t.Fatalf("advertised native route did not deliver real tree: %+v", measured.WindowStats)
			}
			if strings.Contains(follow.Summary, markerNavigationPrefix) {
				t.Fatal("measurement view must not repeat discovery navigation")
			}
		})
	}
}

func TestTraceMarkerNavigationPublicIndependentSources(t *testing.T) {
	dir := t.TempDir()
	// The public manifest has one scheduler/marker authority. An unrelated
	// perf sibling containing an end row cannot close the primary's begin.
	for name, body := range map[string]string{
		"first.systrace":   "# tracer: nop\nworker-7 (7) [001] .... 1.000000: tracing_mark_write: B|7|Same\n",
		"second.perftrace": "# tracer: nop\nworker-7 (7) [001] .... 1.010000: tracing_mark_write: E|7\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	bundle := filepath.Join(dir, "both.tracebundle.json")
	writeToolTraceBundleV2Fixture(t, bundle, []byte(`{"version":"test","systrace":"first.systrace","artifacts":[{"type":"systrace","path":"first.systrace"},{"type":"perftrace","path":"second.perftrace","perf_capability":{"time_domain":"trace_seconds","trace_query_ready":true}}],"perf_clock_alignments":[{"artifact_path":"second.perftrace","perf_time_domain":"trace_seconds","trace_time_domain":"trace_seconds","confidence":"same_domain","calibrated":false}]}`))
	ctx := &types.BusContext{RepoRoot: dir, WorkDir: dir, Mutable: types.NewMutableState("independent sources")}
	r := businessRefTestQuery(t, ctx, map[string]any{"path": bundle, "view": "event_search", "event_types": []string{"trace_mark"}})
	payload := businessSpanSchedulerPublicPayload(t, r)
	if len(payload.TraceArtifacts) != 2 || len(payload.Events) != 1 || payload.Events[0].SpanAction != "B" || !strings.Contains(r.Summary, "Do not pair across physical sources or join same-name threads") {
		t.Fatalf("composite navigation lost native provenance boundary: %+v\n%s", payload, r.Summary)
	}
	follow := businessRefTestQuery(t, ctx, map[string]any{"path": bundle, "view": "window_stats", "time_start": 1, "time_end": "1.010000s"})
	measured := businessSpanSchedulerPublicPayload(t, follow)
	if measured.WindowStats == nil || measured.WindowStats.BusinessTree == nil || len(measured.WindowStats.BusinessTree.Nodes) != 1 {
		t.Fatalf("source-scoped followup unavailable: %+v", measured.WindowStats)
	}
	for _, node := range measured.WindowStats.BusinessTree.Nodes {
		if node.ParentID != "" || node.ActualEndTs != nil || node.Inclusive != nil || node.Self != nil {
			t.Fatalf("unadmitted sibling endpoint became a parent or completed pair: %+v", node)
		}
	}
}

func TestTraceMarkerNavigationDoesNotReadUnparsedProseOrFilters(t *testing.T) {
	for _, result := range []tracequery.Result{
		{View: "event_search"},
		{View: "event_search", Events: []tracequery.EventView{{Event: tracequery.Event{Type: tracequery.EventSchedWakeup, Name: "B|7|OpenDocument", SpanAction: "B"}}}},
		{View: "event_search", Events: []tracequery.EventView{{Event: tracequery.Event{Type: tracequery.EventTraceMark, SpanAction: "C", SpanName: "B|7|OpenDocument"}}}},
		{View: "window_stats", Events: []tracequery.EventView{{Event: tracequery.Event{Type: tracequery.EventTraceMark, SpanAction: "B"}}}},
	} {
		var b strings.Builder
		writeTraceMarkerQueryNavigation(&b, result)
		if b.Len() != 0 {
			t.Fatalf("non-endpoint/result prose triggered navigation: %s", b.String())
		}
	}
	for _, route := range [][2]string{{"window_stats", "business_tree"}, {"span_window", "trace_spans"}} {
		if !traceMarkerNavigationCatalogHasMetric(route[0], route[1]) {
			t.Fatalf("advice detached from catalog: %+v", route)
		}
	}
}
