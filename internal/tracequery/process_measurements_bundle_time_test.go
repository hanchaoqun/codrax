package tracequery

import (
	"context"
	"math"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracewire"
)

// A calibrated perf companion may map perf samples, but does not grant its
// injected process-measure rows admission or change the primary trace clock.
func TestProcessMeasurementsDiscoveryBundleClockAuthority(t *testing.T) {
	dir := t.TempDir()
	primary := filepath.Join(dir, "primary.systrace")
	perf := filepath.Join(dir, "companion.perftrace")
	other := filepath.Join(dir, "other.systrace")
	bundle := filepath.Join(dir, "capture.tracebundle.json")
	unknown := processMeasureTestRecord(4, 0, 1, 1, 101)
	unknown.StartNS = tracewire.ProcessMeasureScalar{Status: "null", StorageClass: "null"}
	writeBundleProvenanceFixture(t, primary, processMeasureTestText(t,
		processMeasureTestRecord(1, 30001000000, 1, 1, 101),
		processMeasureTestRecord(2, 30002000000, 1, 1, 101),
		processMeasureTestRecord(3, -1, 1, 1, 101), unknown))
	writeBundleProvenanceFixture(t, perf, processMeasureTestText(t,
		processMeasureTestRecord(99, 100001000000, 1, 1, 101))+
		"app-20 (20) [001] .... 100.001000: perf_sample: cpu=1 pid=20 tid=20 period=1 event=cpu-cycles symbol=draw dso=lib.so source=test\n")
	writeBundleProvenanceFixture(t, other, processMeasureTestText(t,
		processMeasureTestRecord(98, 30001000000, 1, 1, 101)))
	writeTraceBundleV2ForTest(t, bundle, []byte(`{"systrace":"primary.systrace","artifacts":[
		{"type":"systrace","path":"other.systrace"},
		{"type":"perftrace","path":"companion.perftrace","perf_capability":{"time_domain":"perf_event_time","trace_query_ready":true}}],
		"perf_clock_alignments":[{"artifact_path":"companion.perftrace","perf_time_domain":"perf_event_time","trace_time_domain":"trace_seconds","offset_sec":-70,"slope":1,"calibrated":true}]}`))
	idx, err := BuildIndex(context.Background(), bundle)
	if err != nil {
		t.Fatal(err)
	}
	got := Run(idx, Query{View: "event_search", EventTypes: []EventType{EventProcessMeasureInterval}, TimeStart: 30, TimeEnd: 30.002, Limit: 10})
	if len(got.Events) != 1 || got.Events[0].PluginFields.ProcessMeasure.RowID != 1 || got.Events[0].SourcePath != canonicalTraceIndexPath(primary) || got.Events[0].ClockAligned {
		t.Fatalf("mapped, isolated, right-edge, or unknown process time admitted: %+v", got.Events)
	}
	all := Run(idx, Query{View: "event_search", EventTypes: []EventType{EventProcessMeasureInterval}, Limit: 10})
	if len(all.Events) != 4 {
		t.Fatalf("raw primary observations lost or perf/foreign rows admitted: %+v", all.Events)
	}
	perfResult := Run(idx, Query{View: "event_search", EventTypes: []EventType{EventPerfSample}, TimeStart: 30, TimeEnd: 30.002})
	if len(perfResult.Events) != 1 || !perfResult.Events[0].ClockAligned || math.Abs(perfResult.Events[0].Ts-30.001) > 1e-9 {
		t.Fatalf("test did not exercise admitted affine companion: %+v", perfResult.Events)
	}
	if _, err := StreamEventSearch(context.Background(), bundle, Query{View: "event_search"}); err == nil {
		t.Fatal("stream bypassed bundle provenance")
	}
	writeTraceBundleV2ForTest(t, bundle, []byte(`{"systrace":"primary.systrace","perf_clock_alignments":[{"artifact_path":"primary.systrace","perf_time_domain":"trace_seconds","trace_time_domain":"trace_seconds","offset_sec":1,"slope":1,"calibrated":true}]}`))
	if changed, err := BuildIndex(context.Background(), bundle); err == nil || changed != nil || !strings.Contains(err.Error(), "references unbound perf child") {
		t.Fatalf("primary process carrier acquired affine authority: idx=%v err=%v", changed, err)
	}
}
