package tool

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestTraceQueryBusinessTreePublicIdentityMeasurementsAndScope(t *testing.T) {
	path, err := filepath.Abs("../../eval/fixtures/hmosperf_marker_tree/events.systrace")
	if err != nil {
		t.Fatal(err)
	}
	ctx := &types.BusContext{RepoRoot: t.TempDir(), WorkDir: t.TempDir(), Mutable: types.NewMutableState("business nesting")}
	result := businessRefTestQuery(t, ctx, map[string]any{"path": path, "view": "window_stats", "pid": 700, "time_start": 5, "time_end": 5.012})
	payload := businessSpanSchedulerPublicPayload(t, result)
	if payload.WindowStats == nil || payload.WindowStats.BusinessTree == nil || payload.WindowStats.BusinessTree.NodeCount != 4 {
		t.Fatalf("public native tree missing: %+v", payload.WindowStats)
	}
	facts := map[string]TraceBusinessTreeFact{}
	for _, r := range result.Observations {
		if r.Predicate != types.TraceBusinessTreePredicate {
			continue
		}
		f, ok := DecodeTraceBusinessTreeFact(r)
		if !ok {
			t.Fatalf("public tree fact cannot be decoded: %+v", r)
		}
		if f.Node.SourcePath != path || r.SourceRef.Path != path || r.SourceRef.QueryScopeID == "" || len(r.SupportRefs) == 0 {
			t.Fatalf("source/query receipt missing: %+v", r)
		}
		facts[r.Subject+":"+r.Object] = f
		if f.Node.Closure == "closed" && f.Node.Self.States.Values != nil {
			for _, mutate := range []func(*TraceBusinessTreeFact){
				func(f *TraceBusinessTreeFact) { f.Node.Self.DurationMs = -1 },
				func(f *TraceBusinessTreeFact) { f.Node.Inclusive.DurationMs++ },
				func(f *TraceBusinessTreeFact) { f.Node.Self.States.Values.RunningMs++ },
				func(f *TraceBusinessTreeFact) {
					f.Node.Self.States.Values.SleepIOWaitMs = f.Node.Self.States.Values.SleepMs + 1
				},
				func(f *TraceBusinessTreeFact) { f.Node.ParentID = f.Node.ID },
				func(f *TraceBusinessTreeFact) { f.Node.SourcePath += ".other" },
			} {
				bytes, _ := json.Marshal(f)
				var copyFact TraceBusinessTreeFact
				if err := json.Unmarshal(bytes, &copyFact); err != nil {
					t.Fatal(err)
				}
				mutate(&copyFact)
				bytes, _ = json.Marshal(copyFact)
				copyRecord := r
				copyRecord.RichNotes = []string{types.TraceNoteKeyBusinessTreeNode + "=" + string(bytes)}
				if _, ok := DecodeTraceBusinessTreeFact(copyRecord); ok {
					t.Errorf("inconsistent structured measurements accepted: %s", bytes)
				}
			}
		}
		if r.Predicate == types.TraceBusinessSpanPredicate || r.Role != types.AnswerAggregateRoleSupportingCoverage {
			t.Fatal("new tree must not mint a work relation or root seat")
		}
		for _, mutation := range []func(*types.ObservationRecord){
			func(r *types.ObservationRecord) { r.SourceRef.Path += ".other" },
			func(r *types.ObservationRecord) { r.SourceRef.QueryWindowEndTs += .001 },
			func(r *types.ObservationRecord) { r.Object += " other" },
			func(r *types.ObservationRecord) { r.Subject = "other-700" },
			func(r *types.ObservationRecord) { r.Span.LineStart++ },
			func(r *types.ObservationRecord) { r.Span.LineEnd++ },
			func(r *types.ObservationRecord) { r.Span.StartTs += .0001 },
			func(r *types.ObservationRecord) { r.Span.EndTs += .0001 },
			func(r *types.ObservationRecord) { r.Value = "999" },
			func(r *types.ObservationRecord) { r.Unit = "ns" },
			func(r *types.ObservationRecord) { r.Role = "principal_answer" },
			func(r *types.ObservationRecord) { r.ProvenanceLane = "" },
			func(r *types.ObservationRecord) { r.SourceRef.Kind = "" },
			func(r *types.ObservationRecord) { r.SourceRef.QueryScopeID = "" },
		} {
			copy := r
			mutation(&copy)
			if _, ok := DecodeTraceBusinessTreeFact(copy); ok {
				t.Errorf("mismatched tree receipt accepted: %+v", copy)
			}
		}
	}
	if len(facts) != 4 {
		t.Fatalf("typed tree truncated by old span budget: %d", len(facts))
	}
	outer := facts["business-700:OpenDocument"].Node
	middle := facts["business-700:LoadIndex"].Node
	leaf := facts["business-700:DecodeRecord"].Node
	if middle.ParentID != outer.ID || leaf.ParentID != middle.ID || facts["background-900:OpenDocument"].Node.ParentID != "" {
		t.Fatalf("true nesting/source owner lost: %+v", facts)
	}
	for name, want := range map[string][2]float64{"OpenDocument": {10, 5}, "LoadIndex": {5, 4}, "DecodeRecord": {1, 1}} {
		n := facts["business-700:"+name].Node
		if n.Inclusive == nil || n.Self == nil || math.Abs(n.Inclusive.DurationMs-want[0]) > 1e-6 || math.Abs(n.Self.DurationMs-want[1]) > 1e-6 {
			t.Errorf("%s cost = %+v", name, n)
		}
	}
	if outer.Self.States == nil || outer.Self.States.Values == nil || math.Abs(outer.Self.States.Values.RunningMs-5) > 1e-6 || outer.Self.States.Values.RunnableMs != 0 {
		t.Errorf("self states borrowed total or query states: %+v", outer.Self)
	}
	if facts["background-900:OpenDocument"].Node.Inclusive.States.Values != nil {
		t.Fatal("unknown background states became known zeros")
	}
	var b strings.Builder
	writeTraceBusinessTree(&b, payload.WindowStats.BusinessTree, 2)
	if !strings.Contains(b.String(), "摘要另省略 2") || !strings.Contains(b.String(), "不是唤醒链") {
		t.Fatalf("preview lost independent omission/causal boundary: %s", b.String())
	}
	// A display carrier alone must leave the causal projection byte-identical.
	var without []types.ObservationRecord
	for _, r := range result.Observations {
		if r.Predicate != types.TraceBusinessTreePredicate {
			without = append(without, r)
		}
	}
	before, _ := json.Marshal(types.TraceCausalProjectionFromObservationRecords(without))
	after, _ := json.Marshal(types.TraceCausalProjectionFromObservationRecords(result.Observations))
	if string(before) != string(after) {
		t.Fatal("business tree changed causal projection")
	}
}

func TestTraceQueryBusinessTreePublicBundlePhysicalCitations(t *testing.T) {
	dir := t.TempDir()
	first, second := filepath.Join(dir, "first.systrace"), filepath.Join(dir, "second.perftrace")
	for _, p := range []string{first, second} {
		if err := os.WriteFile(p, []byte("# tracer: nop\nworker-7 (7) [001] .... 1.000000: tracing_mark_write: B|7|Same\nworker-7 (7) [001] .... 1.010000: tracing_mark_write: E|7\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	bundle := filepath.Join(dir, "both.tracebundle.json")
	writeToolTraceBundleV2Fixture(t, bundle, []byte(`{"version":"test","systrace":"first.systrace","artifacts":[{"type":"systrace","path":"first.systrace"},{"type":"perftrace","path":"second.perftrace","perf_capability":{"time_domain":"trace_seconds","trace_query_ready":true}}],"perf_clock_alignments":[{"artifact_path":"second.perftrace","perf_time_domain":"trace_seconds","trace_time_domain":"trace_seconds","confidence":"same_domain","calibrated":false}]}`))
	first, _ = filepath.EvalSymlinks(first)
	second, _ = filepath.EvalSymlinks(second)
	bundle, _ = filepath.EvalSymlinks(bundle)
	ctx := &types.BusContext{RepoRoot: dir, WorkDir: dir, Mutable: types.NewMutableState("source references")}
	r := businessRefTestQuery(t, ctx, map[string]any{"path": bundle, "view": "window_stats", "time_start": 1, "time_end": 1.01})
	if !r.Success {
		t.Fatal(r.Summary)
	}
	seen := map[string]bool{}
	for _, record := range r.Observations {
		if record.Predicate != types.TraceBusinessTreePredicate {
			continue
		}
		f, ok := DecodeTraceBusinessTreeFact(record)
		if !ok {
			t.Fatalf("undecodable composite record: %+v", record)
		}
		seen[f.Node.SourcePath] = true
		if record.SourceRef.Path != bundle || f.IndexPath != bundle || len(record.SupportRefs) != 1 || record.SupportRefs[0] != f.Node.SourcePath+":2-3" {
			t.Errorf("mixed physical source / virtual lines: %+v", record)
		}
		if f.Node.ParentID != "" {
			t.Fatal("two physical siblings became parent/child")
		}
	}
	// The public manifest admits one scheduler/marker authority. A perf
	// sibling's arbitrary B/E text cannot become a second business capture.
	if len(seen) != 1 || !seen[first] || seen[second] {
		t.Fatalf("public bundle broadened marker authority: %+v", seen)
	}
}

func TestTraceQueryBusinessTreeCompositePublisherMapsVirtualCoordinates(t *testing.T) {
	end := 1.01
	n := tracequery.TraceMarkerTreeNode{ID: "second:7:102", SourcePath: "/captures/second.systrace", Thread: tracequery.ThreadRef{Comm: "worker", PID: 7}, Name: "Work", StartLine: 102, EndLine: 103, ActualStartTs: 1, ActualEndTs: &end, ParentStatus: "observed_root", Closure: "closed",
		Inclusive: &tracequery.TraceMarkerTreeAccount{DurationMs: 10, Segments: []tracequery.TraceMarkerTreeWindow{{StartTs: 1, EndTs: end}}},
		Self:      &tracequery.TraceMarkerTreeAccount{DurationMs: 10, Segments: []tracequery.TraceMarkerTreeWindow{{StartTs: 1, EndTs: end}}}}
	r := tracequery.Result{SourcePath: "/captures/both.tracebundle.json", TimeStart: 1, TimeEnd: end, TraceArtifacts: []tracequery.TraceArtifactSource{
		{SourcePath: "/captures/first.systrace", LocalLineCount: 3, TimeDomain: "trace_seconds", CanonicalTimeDomain: "trace_seconds", CausalCompatible: true},
		{SourcePath: n.SourcePath, VirtualLineBase: 100, LocalLineCount: 3, TimeDomain: "trace_seconds", CanonicalTimeDomain: "trace_seconds", CausalCompatible: true},
	}, WindowStats: &tracequery.WindowStats{Window: tracequery.TimeWindow{StartTs: 1, EndTs: end}, BusinessTree: &tracequery.TraceMarkerTreeStats{Window: tracequery.TraceMarkerTreeWindow{StartTs: 1, EndTs: end}, NodeCount: 1, Nodes: []tracequery.TraceMarkerTreeNode{n}}}}
	records := traceQueryTypedObservations(r, "path", "/captures/result.json", "/captures/raw.txt", "", time.Now(), tracequery.Query{View: "window_stats", TimeStart: 1, TimeEnd: end, TimeStartSet: true, TimeEndSet: true})
	found := false
	for _, record := range records {
		if record.Predicate != types.TraceBusinessTreePredicate {
			continue
		}
		found = true
		if _, ok := DecodeTraceBusinessTreeFact(record); !ok {
			t.Fatalf("coordinate-mapped fact rejected: %+v", record)
		}
		if len(record.SupportRefs) != 1 || record.SupportRefs[0] != "/captures/second.systrace:2-3" || record.SourceRef.Path != r.SourcePath || record.Span.LineStart != 102 {
			t.Fatalf("virtual coordinates leaked into physical citations: %+v", record)
		}
	}
	if !found {
		t.Fatal("publisher dropped second-source fact")
	}
}

func TestTraceQueryBusinessTreePublicLineScopeAndZeroEndpoint(t *testing.T) {
	for _, tc := range []struct {
		name, raw string
		params    map[string]any
	}{
		{"lines", "worker-7 (7) [001] .... 1.000000: tracing_mark_write: B|7|Work\nworker-7 (7) [001] .... 1.010000: tracing_mark_write: E|7\n", map[string]any{"line_start": 1, "line_end": 2}},
		{"zero", "worker-7 (7) [001] .... 0.000000000: tracing_mark_write: B|7|Work\nworker-7 (7) [001] .... 0.000000001: tracing_mark_write: E|7\n", map[string]any{"time_start": 0, "time_end": 0.000000001}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, path := businessRefTestContext(t, tc.raw)
			tc.params["path"], tc.params["view"] = path, "window_stats"
			r := businessRefTestQuery(t, ctx, tc.params)
			if !r.Success {
				t.Fatal(r.Summary)
			}
			found := false
			for _, record := range r.Observations {
				if record.Predicate != types.TraceBusinessTreePredicate {
					continue
				}
				found = true
				f, ok := DecodeTraceBusinessTreeFact(record)
				if !ok {
					t.Fatalf("bad fact: %+v", record)
				}
				if tc.name == "lines" {
					if f.WindowUnavailableReason == "" || record.SourceRef.QueryWindowKnown || !strings.Contains(TraceBusinessTreeWindowText(f), "按行范围") {
						t.Fatal("line scope invented a time window")
					}
				} else if record.Span.StartTs != 0 || f.Node.Inclusive.Segments[0].StartTs != 0 || !strings.Contains(strings.Join(record.RichNotes, "\n"), `"start_ts":0`) {
					t.Fatal("zero start lost")
				}
			}
			if !found {
				t.Fatal("missing tree fact")
			}
		})
	}
}
