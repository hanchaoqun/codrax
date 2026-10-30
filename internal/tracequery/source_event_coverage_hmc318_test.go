package tracequery

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func hmc318CoverageTrace(t *testing.T, contents string, opts BuildOptions) *Index {
	t.Helper()
	path := filepath.Join(t.TempDir(), "physical.systrace")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	idx, err := BuildIndexWithOptions(context.Background(), path, opts)
	if err != nil {
		t.Fatal(err)
	}
	return idx
}

func hmc318Frequency(ts string) string {
	return "noise-9 (9) [007] .... " + ts + ": cpu_frequency: state=1000000 cpu_id=7\n"
}

func TestHMC318SourceCoverageZeroEmptyNoiseAndUnsupportedIdentity(t *testing.T) {
	for _, tc := range []struct {
		name, text                string
		complete, hasEvent, known bool
		end                       float64
	}{
		{"zero", hmc318Frequency("0.000000"), true, true, true, 0},
		{"no_valid_event", "# tracer: nop\n", true, false, false, 0},
		{"noise", hmc318Frequency("1.000000") + "# noise-9 (9) [007] .... 8.000000: cpu_frequency: state=1 cpu_id=7\nnoise-9 (9) [007] .... 9.000000:\n", true, true, true, 1},
		{"invalid_cpu", hmc318Frequency("1.000000") + "noise-9 (9) [4294967296] .... 2.000000: cpu_frequency: state=1 cpu_id=7\n", false, false, false, 0},
		{"invalid_tid", hmc318Frequency("1.000000") + "noise-4294967296 (9) [007] .... 2.000000: cpu_frequency: state=1 cpu_id=7\n", false, false, false, 0},
		{"regression", hmc318Frequency("2.000000") + hmc318Frequency("1.000000"), true, true, false, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			idx := hmc318CoverageTrace(t, tc.text, BuildOptions{})
			c := idx.TraceArtifacts[0].eventCoverage
			if c.complete != tc.complete || c.hasEvent != tc.hasEvent || c.maxEventTs != tc.end {
				t.Fatalf("source receipt: %+v", c)
			}
			end, known := idx.measuredPhysicalEventEnd()
			if known != tc.known || (known && end != tc.end) {
				t.Fatalf("coverage end=%g known=%v want %g/%v", end, known, tc.end, tc.known)
			}
			if tc.name == "zero" {
				if end, clipped := schedulerMeasuredTimeEnd(idx, Query{TimeEnd: 1, TimeEndSet: true}); !clipped || end != 0 {
					t.Fatalf("known zero conflated with missing coverage: %g/%v", end, clipped)
				}
			}
		})
	}
}

func TestHMC318SourceCoveragePrivateIdentityAndCacheValue(t *testing.T) {
	idx := hmc318CoverageTrace(t, hmc318Frequency("1.000000"), BuildOptions{})
	source := idx.TraceArtifacts[0]
	for name, mutate := range map[string]func(*TraceArtifactSource){
		"path":       func(s *TraceArtifactSource) { s.SourcePath += ".other" },
		"identity":   func(s *TraceArtifactSource) { s.sourceIdentity = traceFileIdentity{} },
		"clock":      func(s *TraceArtifactSource) { s.ClockAlignment = "calibrated_affine" },
		"isolated":   func(s *TraceArtifactSource) { s.CausalCompatible = false },
		"order":      func(s *TraceArtifactSource) { s.timestampOrder = TraceTimestampOrderUnknown },
		"no_receipt": func(s *TraceArtifactSource) { s.eventCoverage = sourceEventCoverage{} },
	} {
		t.Run(name, func(t *testing.T) {
			changed := source // Value-only provenance, never copy Index's sync.Once.
			mutate(&changed)
			candidate := &Index{TraceArtifacts: []TraceArtifactSource{changed}, Size: idx.Size, LastTs: 1, TimestampOrder: TraceTimestampOrderMonotonic}
			if _, known := candidate.measuredPhysicalEventEnd(); known {
				t.Fatal("changed source acquired coverage")
			}
			if _, clipped := schedulerMeasuredTimeEnd(candidate, Query{TimeEnd: 2, TimeEndSet: true}); clipped {
				t.Fatal("actual source ledger entered legacy fallback")
			}
		})
	}
	t.Run("json", func(t *testing.T) {
		b, err := json.Marshal(source)
		if err != nil {
			t.Fatal(err)
		}
		var decoded TraceArtifactSource
		if err := json.Unmarshal(b, &decoded); err != nil {
			t.Fatal(err)
		}
		candidate := &Index{TraceArtifacts: []TraceArtifactSource{decoded}, Size: idx.Size, LastTs: 1, TimestampOrder: TraceTimestampOrderMonotonic}
		if _, clipped := schedulerMeasuredTimeEnd(candidate, Query{TimeEnd: 2, TimeEndSet: true}); clipped {
			t.Fatal("JSON minted private EOF")
		}
	})
	t.Run("prefix_cache", func(t *testing.T) {
		cache := &traceAnchorCache{items: map[traceAnchorKey]*traceAnchorSet{}}
		key := traceAnchorKeyForIdentity(source.SourcePath, source.sourceIdentity)
		cache.store(key, &traceAnchorSet{sourceEventCoverage: source.eventCoverage})
		cache.store(key, &traceAnchorSet{CoveredLines: 9999, CoveredMaxTs: 900, CoveredLastTs: 900, CoveredLastTsSet: true, TimestampOrder: TraceTimestampOrderMonotonic})
		loaded := cache.load(key)
		if loaded.sourceEventCoverage != source.eventCoverage {
			t.Fatal("prefix merge changed source receipt")
		}
		loaded.sourceEventCoverage.maxEventTs = 999
		if cache.load(key).sourceEventCoverage != source.eventCoverage {
			t.Fatal("cache load aliased coverage")
		}
		old := newAnchorRecorder(&traceAnchorSet{CoveredLines: 99, CoveredLastTs: 8, CoveredLastTsSet: true, TimestampOrder: TraceTimestampOrderMonotonic}, traceAnchor{LineNo: 50}, true)
		old.contiguous = true
		old.finishEOF()
		old.publishSourceEventCoverage(source.SourcePath, source.sourceIdentity)
		if old.set.sourceEventCoverage.complete {
			t.Fatal("old prefix/order reconstructed private permission")
		}
	})
}

func TestHMC318SourceCoverageSameSizeNewGeneration(t *testing.T) {
	idx := hmc318CoverageTrace(t, hmc318Frequency("1.000000"), BuildOptions{})
	before, err := os.Stat(idx.Path)
	if err != nil {
		t.Fatal(err)
	}
	old := idx.TraceArtifacts[0]
	if !old.sourceIdentity.Strong() {
		t.Skip("strong generation unavailable")
	}
	overwriteTraceSameSizeAndRestoreMtime(t, idx.Path, hmc318Frequency("2.000000"), before)
	current, err := BuildIndexWithOptions(context.Background(), idx.Path, BuildOptions{AllowWindowedParse: true, TimeStartSet: true, TimeEndSet: true, TimeEnd: 1.5})
	if err != nil {
		t.Fatal(err)
	}
	if end, known := current.measuredPhysicalEventEnd(); !known || end != 2 {
		t.Fatalf("new source coverage=%g/%v", end, known)
	}
	if old.eventCoverage.matches(current.TraceArtifacts[0].SourcePath, current.TraceArtifacts[0].sourceIdentity) {
		t.Fatal("old generation receipt reused")
	}
}

func TestHMC318SourceCoverageBudgetAndCancellation(t *testing.T) {
	t.Run("hard_budget_prefix", func(t *testing.T) {
		path := anchorTestTrace(t, traceAnchorLineInterval+1024, 0)
		_, err := BuildIndexWithOptions(context.Background(), path, BuildOptions{MaxEvents: traceAnchorLineInterval + 1})
		var denied *IndexEventLimitError
		if !errors.As(err, &denied) {
			t.Fatalf("hard budget=%v", err)
		}
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		cached := anchorCache.load(traceAnchorKeyForInfo(canonicalTraceIndexPath(path), info))
		if cached == nil || len(cached.Anchors) == 0 || cached.sourceEventCoverage.complete {
			t.Fatalf("prefix gained EOF: %+v", cached)
		}
	})
	t.Run("padding_suffix", func(t *testing.T) {
		idx := hmc318CoverageTrace(t, hmc318Frequency("1.000000")+hmc318Frequency("1.010000")+hmc318Frequency("1.020000")+hmc318Frequency("2.000000"), BuildOptions{
			AllowWindowedParse: true, TimeStart: 1, TimeStartSet: true, TimeEnd: 1.01, TimeEndSet: true, TimePaddingAfter: .5, MaxEvents: 2})
		if !idx.PaddingTruncated || len(idx.Events) != 2 {
			t.Fatalf("padding shape changed: %v/%d", idx.PaddingTruncated, len(idx.Events))
		}
		if end, known := idx.measuredPhysicalEventEnd(); !known || end != 2 {
			t.Fatalf("actual O(1) suffix EOF lost: %g/%v", end, known)
		}
	})
	t.Run("line_stop", func(t *testing.T) {
		var b strings.Builder
		for i := 0; i < 1500; i++ {
			b.WriteString(hmc318Frequency(fmt.Sprintf("%.6f", 1+float64(i)/1000)))
		}
		idx := hmc318CoverageTrace(t, b.String(), BuildOptions{AllowWindowedParse: true, LineStart: 1, LineEnd: 2})
		if idx.TraceArtifacts[0].eventCoverage.complete {
			t.Fatal("line stop minted physical EOF")
		}
	})
	t.Run("cold_cancel_and_exact_warm", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "cancel.systrace")
		if err := os.WriteFile(path, []byte(hmc318Frequency("1.000000")), 0o600); err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err := BuildIndex(ctx, path); !errors.Is(err, context.Canceled) {
			t.Fatalf("cold canceled build=%v", err)
		}
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if cached := anchorCache.load(traceAnchorKeyForInfo(canonicalTraceIndexPath(path), info)); cached != nil && cached.sourceEventCoverage.complete {
			t.Fatal("canceled cold build minted EOF")
		}
		idx, err := BuildIndex(context.Background(), path)
		if err != nil {
			t.Fatal(err)
		}
		warm, err := BuildIndex(ctx, path)
		if err != nil || warm != idx {
			t.Fatalf("pre-canceled exact-cache compatibility changed: %v", err)
		}
		q := Query{View: "thread_timeline", PID: 9, TimeEnd: 2, TimeEndSet: true}
		result := Run(warm, q.WithRunContext(ctx))
		if result.Timeline != nil {
			t.Fatal("source receipt granted canceled query output")
		}
	})
}

func TestHMC318SourceCoverageSharedConsumerAndBundleBoundary(t *testing.T) {
	text := "raw-21 (20) [005] .... 3.000000: sched_switch: prev_comm=idle/5 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=raw next_pid=21 next_prio=52\n" +
		"raw-21 (20) [005] .... 3.008000: sched_switch: prev_comm=raw prev_pid=21 prev_prio=52 prev_state=R+ ==> next_comm=idle/5 next_pid=0 next_prio=120\n"
	idx := hmc318CoverageTrace(t, text, BuildOptions{AllowWindowedParse: true, TimeStart: 3, TimeEnd: 3.01, TimeStartSet: true, TimeEndSet: true})
	q := Query{PID: 21, TimeStart: 3, TimeEnd: 3.01, TimeStartSet: true, TimeEndSet: true}
	stats := ComputeWindowStats(idx, q)
	if len(stats.CPU) != 1 || math.Abs(stats.CPU[0].BusyMs-8) > 1e-8 || stats.CPU[0].IdleMs != 0 || stats.CPU[0].BusyIdleReason != "artifact_tail_uncovered" {
		t.Fatalf("global CPU scope: %+v", stats.CPU)
	}
	q.View = "root_cause_rank"
	result := Run(idx, q)
	if result.RootCauseRank == nil {
		t.Fatal("rank absent")
	}
	for _, item := range result.RootCauseRank.Items {
		if item.Type == "scheduler_latency" || item.Type == "runnable_wait" || item.Type == "trace_gap" {
			t.Fatalf("uncovered suffix gained root authority: %+v", item)
		}
	}
	if end, clipped := schedulerMeasuredTimeEnd(idx, Query{TimeEnd: 3.01}); clipped || end != 3.01 {
		t.Fatal("legacy unset endpoint behavior changed")
	}
	other := hmc318CoverageTrace(t, hmc318Frequency("9.000000"), BuildOptions{})
	// Even two individually proven source receipts cannot authorize a global
	// bundle EOF. No mutex-bearing Index is copied and no merged max is used.
	bundle := &Index{TraceArtifacts: []TraceArtifactSource{idx.TraceArtifacts[0], other.TraceArtifacts[0]}, Size: 100, LastTs: 9, TimestampOrder: TraceTimestampOrderMonotonic}
	if end, clipped := schedulerMeasuredTimeEnd(bundle, Query{TimeEnd: 10, TimeEndSet: true}); clipped || end != 10 {
		t.Fatal("bundle max borrowed another source's end")
	}
}

func TestHMC318PublicBundleCannotBorrowMergedEOF(t *testing.T) {
	dir := t.TempDir()
	body, err := os.ReadFile("../../eval/fixtures/hmosperf_business_io_chain/events.systrace")
	if err != nil {
		t.Fatal(err)
	}
	writeBundleProvenanceFixture(t, filepath.Join(dir, "capture.systrace"), string(body))
	writeBundleProvenanceFixture(t, filepath.Join(dir, "capture.perftrace"), "sample-900 (900) [007] .... 9.000000: perf_sample: cpu=7 pid=900 tid=900 period=1 event=cpu-cycles symbol=work dso=libwork.so source=test\n")
	path := filepath.Join(dir, "capture.tracebundle.json")
	writeBundleProvenanceFixture(t, path, `{"version":"test","systrace":"capture.systrace","artifacts":[{"type":"systrace","path":"capture.systrace"},{"type":"perftrace","path":"capture.perftrace","perf_capability":{"time_domain":"trace_seconds","trace_query_ready":true}}],"perf_clock_alignments":[{"artifact_path":"capture.perftrace","perf_time_domain":"trace_seconds","trace_time_domain":"trace_seconds","confidence":"same_domain","calibrated":false}]}`)
	idx, err := BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if len(idx.TraceArtifacts) != 2 || idx.LastTs != 9 {
		t.Fatalf("actual multi-source fixture was not admitted: sources=%d end=%g", len(idx.TraceArtifacts), idx.LastTs)
	}
	if end, clipped := schedulerMeasuredTimeEnd(idx, Query{TimeEnd: 10, TimeEndSet: true}); clipped || end != 10 {
		t.Fatal("merged sort/max acquired per-source physical EOF authority")
	}
	result := Run(idx, Query{View: "thread_timeline", PID: 200, TimeStart: .990, TimeEnd: 1.055, TimeStartSet: true, TimeEndSet: true})
	if result.Timeline == nil {
		t.Fatal("closed source-local state lost")
	}
	for _, iv := range result.Timeline.Intervals {
		if iv.State == StateSSleep && iv.EndLine > 0 && math.Abs(iv.DurationMs-31) < 1e-8 {
			return
		}
	}
	t.Fatalf("known closed sleep was lost with unproven bundle EOF: %+v", result.Timeline.Intervals)
}
