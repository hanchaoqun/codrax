package tracequery

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const zeroTimestampSchedulerTrace = "idle-0 (0) [000] .... 0.000000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=41 next_prio=20\n" +
	"app-41 (41) [000] .... 0.010000: sched_switch: prev_comm=app prev_pid=41 prev_prio=20 prev_state=S ==> next_comm=idle next_pid=0 next_prio=120\n"

func zeroTimestampPath(t *testing.T, raw string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "zero.ftrace")
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestIndexZeroTimestampPublicColdWarmAndNativeAccount(t *testing.T) {
	path := zeroTimestampPath(t, zeroTimestampSchedulerTrace)
	for _, view := range []string{"thread_timeline", "window_stats"} {
		t.Run(view, func(t *testing.T) {
			idx, err := BuildIndex(context.Background(), path)
			if err != nil {
				t.Fatal(err)
			}
			if idx.FirstTs != 0 || idx.LastTs != .01 {
				t.Fatalf("parsed zero must remain the first timestamp: first=%g last=%g", idx.FirstTs, idx.LastTs)
			}
			q := Query{View: view, PID: 41}
			r := Run(idx, q)
			a := r.TargetWindowStates
			if a == nil || a.Window.StartTs != 0 || !a.Window.StartSet || math.Abs(a.RunningMs-10) > 1e-6 || math.Abs(a.TotalMs-10) > 1e-6 {
				t.Fatalf("physical 0.. .01 scheduler interval must publish 10ms: %+v", a)
			}
			if q.TimeStartSet || q.TimeEndSet {
				t.Fatal("artifact-derived zero must not rewrite caller endpoint flags")
			}
		})
	}
}

func TestIndexZeroTimestampPublicStreamAndSearchScope(t *testing.T) {
	path := zeroTimestampPath(t, zeroTimestampSchedulerTrace)
	shell, err := StreamScan(context.Background(), path, TraceFlavorAuto, func(Event) bool { return true })
	if err != nil {
		t.Fatal(err)
	}
	if shell.FirstTs != 0 || shell.LastTs != .01 || len(shell.Events) != 0 {
		t.Fatalf("streaming metadata cannot depend on retained events: %+v", shell)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	held, err := StreamScanHeldFile(context.Background(), f, path, TraceFlavorAuto, 1<<20, func(Event) bool { return true })
	if err != nil || held == nil || held.FirstTs != 0 || held.LastTs != .01 {
		t.Fatalf("held-file scan must preserve zero: index=%+v err=%v", held, err)
	}
	r, err := StreamEventSearch(context.Background(), path, Query{View: "event_search", Pattern: "prev_comm=app", TimeStartSet: true, TimeEnd: .02, TimeEndSet: true})
	if err != nil {
		t.Fatal(err)
	}
	if r.TimeStart != 0 || r.TimeEnd != .02 || r.EventSearchCoverage == nil ||
		r.EventSearchCoverage.ScopeTimeStart != 0 || r.EventSearchCoverage.MatchedTimeStart != .01 {
		t.Fatalf("explicit zero, physical scope and matched-row scope must remain separate: %+v", r)
	}
}

func TestIndexZeroTimestampPublicDeriveDomains(t *testing.T) {
	for _, tc := range []struct {
		name string
		raw  string
	}{
		{"contiguous", zeroTimestampSchedulerTrace},
		{"noncontiguous", strings.Replace(zeroTimestampSchedulerTrace, "app-41 (41)", "other-2 (2) [001] .... 1.000000: cpu_idle: state=0 cpu_id=1\napp-41 (41)", 1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			full, err := BuildIndex(context.Background(), zeroTimestampPath(t, tc.raw))
			if err != nil {
				t.Fatal(err)
			}
			before := append([]Event(nil), full.Events...)
			derived := deriveWindowedIndex(full, BuildOptions{TimeStartSet: true, TimeEnd: .02, TimeEndSet: true})
			if len(derived.Events) != 2 || derived.FirstTs != 0 || derived.LastTs != .01 {
				t.Fatalf("derived retained domain must initialize independently and retain zero: %+v", derived)
			}
			if !reflect.DeepEqual(before, full.Events) {
				t.Fatal("derivation modified parent events")
			}
		})
	}
}

func TestIndexZeroTimestampPublicUnknownAndEmpty(t *testing.T) {
	unknown := "worker-9 (9) [000] .... 0.000000: unknown_producer_event: payload=kept\n"
	idx, err := BuildIndex(context.Background(), zeroTimestampPath(t, unknown+strings.SplitAfter(zeroTimestampSchedulerTrace, "\n")[1]))
	if err != nil || idx == nil || idx.FirstTs != 0 {
		t.Fatalf("successfully parsed unknown event participates in timestamp metadata: %+v %v", idx, err)
	}
	for _, raw := range []string{"", "# no parsed rows\n", strings.ReplaceAll(zeroTimestampSchedulerTrace, "0.010000", "0.000000")} {
		empty, err := BuildIndex(context.Background(), zeroTimestampPath(t, raw))
		if raw == "" {
			if err == nil || empty != nil {
				t.Fatal("physically empty source must still fail source admission")
			}
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		if r := Run(empty, Query{View: "thread_timeline", PID: 41}); r.TargetWindowStates != nil {
			t.Fatalf("empty or zero-width input must not invent positive scheduler time: %+v", r.TargetWindowStates)
		}
	}
}

func TestIndexZeroTimestampPublicPresenceIsNotRetentionOrCount(t *testing.T) {
	carrier := strings.Replace(testTraceDBTextRecordLine("schema", 0, []byte(`{"version":1}`)), "ts_ns=1234567890", "ts_ns=0", 1) + "\n"
	for _, tc := range []struct {
		name, raw string
		known     int
		events    int
		present   bool
	}{
		{"known_zero", strings.SplitAfter(zeroTimestampSchedulerTrace, "\n")[0], 1, 1, true},
		{"unknown_zero", "worker-9 (9) [000] .... 0.000000: unknown_producer_event: payload=kept\n", 0, 1, true},
		{"preservation_zero", carrier, 1, 0, true},
		{"empty", "", 0, 0, false},
		{"comment", "# no timestamp was parsed\n", 0, 0, false},
		{"malformed", "worker-9 [000] .... missing: sched_switch: incomplete\n", 0, 0, false},
		{"negative_source_not_admitted", "worker-9 (9) [000] .... -1.000000: cpu_idle: state=0 cpu_id=0\n", 0, 0, false},
		{"nonfinite_source_not_admitted", "worker-9 (9) [000] .... NaN: cpu_idle: state=0 cpu_id=0\n", 0, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			idx, err := BuildIndex(context.Background(), zeroTimestampPath(t, tc.raw))
			if tc.raw == "" {
				if err == nil || idx != nil {
					t.Fatal("physically empty source must still fail source admission")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if idx.hasTimestampBounds() != tc.present || idx.timestampBoundsSeen != tc.present || idx.FirstTs != 0 || idx.LastTs != 0 || idx.ParsedKnown != tc.known || len(idx.Events) != tc.events {
				t.Fatalf("presence must follow admitted timestamps, not storage or known counts: present=%v index=%+v", idx.hasTimestampBounds(), idx)
			}
			copy := *idx
			if copy.hasTimestampBounds() != tc.present {
				t.Fatal("Index value copy lost timestamp presence")
			}
			q := normalizeQuery(idx, Query{View: "thread_timeline", PID: 41})
			if q.timeStartBackfilled != tc.present || q.TimeStartSet || q.TimeEndSet {
				t.Fatalf("only known physical envelope can stamp automatic provenance: %+v", q)
			}
			if Run(idx, Query{View: "thread_timeline", PID: 41}).TargetWindowStates != nil {
				t.Fatal("zero-width, missing, or preservation-only input invented a positive state account")
			}
		})
	}
}

func TestIndexZeroTimestampBoundsCompatibilityAndReset(t *testing.T) {
	for _, times := range [][]float64{{0, .01}, {.01, 0, .02}, {0, 0, .01}, {.02, .01, 0}} {
		idx := &Index{}
		for _, ts := range times {
			idx.observeTimestampBounds(ts)
		}
		if idx.FirstTs != 0 || !idx.hasTimestampBounds() || idx.LastTs != timesMaximum(times) {
			t.Fatalf("first zero is a value, never an uninitialized sentinel: times=%v index=%+v", times, idx)
		}
		idx.resetTimestampBounds()
		if idx.hasTimestampBounds() || idx.FirstTs != 0 || idx.LastTs != 0 {
			t.Fatal("reset retained a borrowed envelope or presence")
		}
		idx.observeTimestampBounds(.03)
		if idx.FirstTs != .03 || idx.LastTs != .03 || !idx.hasTimestampBounds() {
			t.Fatal("reset did not start an independent observed domain")
		}
	}
	for _, bad := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		idx := &Index{}
		idx.observeTimestampBounds(bad)
		if idx.hasTimestampBounds() || idx.timestampBoundsSeen {
			t.Fatalf("invalid timestamp minted presence: %v", bad)
		}
	}
	for _, tc := range []struct {
		name string
		idx  *Index
		want bool
	}{
		{"nil", nil, false}, {"empty", &Index{}, false},
		{"count_is_not_presence", &Index{ParsedKnown: 99, Events: []Event{{Ts: 0}}}, false},
		{"legacy_positive", &Index{FirstTs: 1, LastTs: 2}, true},
		{"legacy_zero_start", &Index{LastTs: .01}, true},
		{"legacy_unknown_zero", &Index{}, false},
		{"legacy_negative", &Index{FirstTs: -1, LastTs: 1}, false},
		{"legacy_nan", &Index{FirstTs: math.NaN(), LastTs: 1}, false},
		{"legacy_inf", &Index{LastTs: math.Inf(1)}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.idx.hasTimestampBounds() != tc.want {
				t.Fatalf("legacy explicit envelope compatibility changed: %+v", tc.idx)
			}
		})
	}
	// Signed derived domains already had min(observed), max(0, observed).
	// The zero fix must not silently redefine that legacy policy.
	signed := &Index{}
	signed.observeTimestampBounds(-2)
	signed.observeTimestampBounds(-1)
	if signed.FirstTs != -2 || signed.LastTs != 0 || !signed.timestampBoundsSeen {
		t.Fatalf("unrelated signed-domain policy changed: %+v", signed)
	}
}

func timesMaximum(times []float64) float64 {
	maximum := times[0]
	for _, ts := range times[1:] {
		if ts > maximum {
			maximum = ts
		}
	}
	return maximum
}

func TestIndexZeroTimestampPublicWarmWindowAndSourceGeneration(t *testing.T) {
	ctx := context.Background()
	path := zeroTimestampPath(t, zeroTimestampSchedulerTrace)
	full, err := BuildIndex(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	warm, err := BuildIndex(ctx, path)
	if err != nil || warm != full || !warm.timestampBoundsSeen {
		t.Fatalf("warm index must retain parser-owned presence: same=%v err=%v", warm == full, err)
	}
	opts := BuildOptions{AllowWindowedParse: true, TimeStartSet: true, TimeEnd: .02, TimeEndSet: true}
	derived, err := BuildIndexWithOptions(ctx, path, opts)
	if err != nil {
		t.Fatal(err)
	}
	cold, err := BuildIndexWithOptions(ctx, zeroTimestampPath(t, zeroTimestampSchedulerTrace), opts)
	if err != nil {
		t.Fatal(err)
	}
	for _, idx := range []*Index{derived, cold} {
		if !idx.Windowed || !idx.timestampBoundsSeen || idx.FirstTs != 0 || idx.LastTs != .01 || len(idx.Events) != 2 {
			t.Fatalf("cold/warm window differs on zero-based retained domain: %+v", idx)
		}
	}
	// An out-of-range derived index must not inherit a parent's known envelope.
	empty := deriveWindowedIndex(full, BuildOptions{TimeStart: 2, TimeStartSet: true, TimeEnd: 3, TimeEndSet: true})
	if empty.hasTimestampBounds() || len(empty.Events) != 0 {
		t.Fatalf("empty retained domain borrowed parent time: %+v", empty)
	}
	oldSource, err := CaptureTraceSourceVersion(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(strings.ReplaceAll(zeroTimestampSchedulerTrace, "0.000000", "0.005000")), 0o644); err != nil {
		t.Fatal(err)
	}
	replaced, err := BuildIndex(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	newSource, err := CaptureTraceSourceVersion(path)
	if err != nil {
		t.Fatal(err)
	}
	if replaced == full || replaced.FirstTs != .005 || !replaced.timestampBoundsSeen || oldSource.Fingerprint() == newSource.Fingerprint() || full.FirstTs != 0 {
		t.Fatal("new source generation reused stale zero envelope or mutated prior index")
	}
}

func TestIndexZeroTimestampPublicSweepAndCluster(t *testing.T) {
	ctx := context.Background()
	path := zeroTimestampPath(t, zeroTimestampSchedulerTrace)
	r, err := StreamWindowSweep(ctx, path, Query{View: ViewWindowSweep, TimeStartSet: true, TimeEnd: .02, TimeEndSet: true, BucketMs: 50})
	if err != nil {
		t.Fatal(err)
	}
	if r.WindowSweep == nil || len(r.WindowSweep.Coverage) != 1 || r.WindowSweep.Coverage[0].StartTs != 0 || r.WindowSweep.Coverage[0].SchedSwitches != 2 {
		t.Fatalf("the bucket at zero must count its actual event: %+v", r.WindowSweep)
	}
	// The existing cluster response retains caller endpoints rather than
	// normalizing its public window from scan metadata. Keep that contract;
	// this control only protects zero-based native interval accounting.
	cluster, err := StreamStateCluster(ctx, path, Query{View: "window_stats", PID: 41, TimeStartSet: true, TimeEnd: .01, TimeEndSet: true}, 8)
	if err != nil {
		t.Fatal(err)
	}
	if cluster.TimeStart != 0 || cluster.TimeEnd != .01 || cluster.WindowStats == nil || len(cluster.WindowStats.TopRunning) != 1 || math.Abs(cluster.WindowStats.TopRunning[0].DurationMs-10) > 1e-6 {
		t.Fatalf("stream cluster must retain the complete native running interval: %+v", cluster)
	}
	// Result limit controls display only; scope and matches keep independent
	// zero-aware domains and a whole-artifact search remains a whole artifact.
	search, err := StreamEventSearch(ctx, path, Query{View: "event_search", Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(search.Events) != 1 || search.EventSearchCoverage == nil || search.EventSearchCoverage.ScopeTimeStart != 0 || search.EventSearchCoverage.ScopeTimeEnd != .01 || search.EventSearchCoverage.MatchedTimeStart != 0 || search.EventSearchCoverage.MatchedTimeEnd != .01 {
		t.Fatalf("display cap changed complete scope or matched timestamp census: %+v", search.EventSearchCoverage)
	}
}

func TestIndexZeroTimestampPublicCacheEpochAndExplicitZero(t *testing.T) {
	path := zeroTimestampPath(t, zeroTimestampSchedulerTrace)
	var actualKey parseCacheKey
	idx, err := buildIndexWithObserver(context.Background(), path, BuildOptions{}, func(phase traceIndexBuildPhase, key parseCacheKey) {
		if phase == traceIndexPhaseSelectionFrozen {
			actualKey = key
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if actualKey.version != "tracequery-v46" {
		t.Fatalf("actual public parser key did not advance its zero-presence epoch: %+v", actualKey)
	}
	oldKey := actualKey
	oldKey.version = "tracequery-v42"
	if oldKey == actualKey {
		t.Fatal("old index generation aliases the new parser")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	anchorKey := traceAnchorKeyForInfo(path, info)
	if anchorKey.version != actualKey.version {
		t.Fatal("anchor proof cache failed to advance with timestamp parsing")
	}
	for _, original := range []Query{
		{TimeStartSet: true},
		{TimeEndSet: true},
		{TimeStartSet: true, TimeEndSet: true},
		{TimeStart: .001, TimeStartSet: true, TimeEnd: .009, TimeEndSet: true},
	} {
		q := normalizeQuery(idx, original)
		if q.TimeStartSet != original.TimeStartSet || q.TimeEndSet != original.TimeEndSet ||
			(original.TimeStartSet && q.TimeStart != original.TimeStart) ||
			(original.TimeEndSet && q.TimeEnd != original.TimeEnd) {
			t.Fatalf("observed envelope changed an explicit caller endpoint: before=%+v after=%+v", original, q)
		}
	}
}

func TestIndexZeroTimestampPublicBundleMappedDomain(t *testing.T) {
	for _, tc := range []struct {
		name, trace string
		perfTimes   []float64
		first, last float64
		present     bool
	}{
		{"identity_zero", zeroTimestampSchedulerTrace, nil, 0, .01, true},
		{"affine_zero", strings.ReplaceAll(zeroTimestampSchedulerTrace, "0.000000", "0.005000"), []float64{10}, 0, .01, true},
		{"negative_then_zero", "# no scheduler rows\n", []float64{9, 10}, 0, 0, true},
		{"negative_only", "# no scheduler rows\n", []float64{9}, 0, 0, false},
		{"empty_children", "# no scheduler rows\n", nil, 0, 0, false},
	} {
		for _, reverse := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/reverse=%t", tc.name, reverse), func(t *testing.T) {
				dir := t.TempDir()
				trace, perf, bundle := filepath.Join(dir, "capture.systrace"), filepath.Join(dir, "sample.perftrace"), filepath.Join(dir, "capture.tracebundle.json")
				writeBundleProvenanceFixture(t, trace, tc.trace)
				var rows strings.Builder
				rows.WriteString("# perf source\n")
				for _, ts := range tc.perfTimes {
					fmt.Fprintf(&rows, "app-41 (41) [000] .... %.6f: perf_sample: cpu=0 pid=41 tid=41 period=1 event=cpu-cycles symbol=App dso=libapp.so source=test\n", ts)
				}
				writeBundleProvenanceFixture(t, perf, rows.String())
				artifacts := `{"type":"systrace","path":"capture.systrace"},{"type":"perftrace","path":"sample.perftrace","perf_capability":{"time_domain":"perf_event_time","trace_query_ready":true}}`
				if reverse {
					artifacts = `{"type":"perftrace","path":"sample.perftrace","perf_capability":{"time_domain":"perf_event_time","trace_query_ready":true}},{"type":"systrace","path":"capture.systrace"}`
				}
				writeTraceBundleV2ForTest(t, bundle, []byte(`{"version":"test","systrace":"capture.systrace","artifacts":[`+artifacts+`],"perf_clock_alignments":[{"artifact_path":"sample.perftrace","perf_time_domain":"perf_event_time","trace_time_domain":"trace_seconds","offset_sec":-10,"slope":1,"calibrated":true}]}`))
				idx, err := BuildIndex(context.Background(), bundle)
				if err != nil {
					t.Fatal(err)
				}
				if idx.FirstTs != tc.first || idx.LastTs != tc.last || idx.timestampBoundsSeen != tc.present {
					t.Fatalf("merged envelope must be minted from admitted mapped timestamps, not child presence: %+v", idx)
				}
				if len(tc.perfTimes) > 0 {
					var found int
					for _, ev := range idx.Events {
						if ev.Type == EventPerfSample {
							if ev.Ts != tc.perfTimes[found]-10 {
								t.Fatalf("mapping or retained signed event changed: %+v", ev)
							}
							found++
						}
					}
					if found != len(tc.perfTimes) {
						t.Fatalf("timestamp bookkeeping removed source events: %d", found)
					}
				}
				if !tc.present && normalizeQuery(idx, Query{}).timeStartBackfilled {
					t.Fatal("empty/non-admitted canonical domain manufactured a zero time boundary")
				}
			})
		}
	}
}
