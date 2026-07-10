package tracequery

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestTraceBundleProvenanceDisambiguatesSameLocalLine(t *testing.T) {
	dir := t.TempDir()
	systrace := filepath.Join(dir, "capture.systrace")
	perftrace := filepath.Join(dir, "capture.perftrace")
	bundle := filepath.Join(dir, "capture.tracebundle.json")
	writeBundleProvenanceFixture(t, systrace, `
 app-20 (20) [001] .... 10.000000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=20 next_prio=53
`)
	writeBundleProvenanceFixture(t, perftrace, `
 app-20 (20) [001] .... 10.001000: perf_sample: cpu=1 pid=20 tid=20 period=9000 event=cpu-cycles symbol=App::draw dso=libapp.so source=test
`)
	writeBundleProvenanceFixture(t, bundle, `{
  "version":"test",
  "systrace":"capture.systrace",
  "artifacts":[
    {"type":"systrace","path":"capture.systrace"},
    {"type":"perftrace","path":"capture.perftrace","perf_capability":{"time_domain":"trace_seconds","trace_query_ready":true}}
  ],
  "perf_clock_alignments":[
    {"artifact_path":"capture.perftrace","perf_time_domain":"trace_seconds","trace_time_domain":"trace_seconds","confidence":"same_domain","calibrated":false}
  ]
}`)

	idx, err := BuildIndex(context.Background(), bundle)
	if err != nil {
		t.Fatal(err)
	}
	if len(idx.Events) != 2 || len(idx.TraceArtifacts) != 2 {
		t.Fatalf("bundle shape: events=%d artifacts=%+v", len(idx.Events), idx.TraceArtifacts)
	}
	if idx.Events[0].Line == idx.Events[1].Line {
		t.Fatalf("same local line from two artifacts must have distinct virtual coordinates: %+v", idx.Events)
	}

	result := Run(idx, Query{View: "event_search", Limit: 10})
	if len(result.Events) != 2 || len(result.EvidencePack) != 2 {
		t.Fatalf("event result shape: %+v", result)
	}
	wantPaths := []string{canonicalTraceIndexPath(systrace), canonicalTraceIndexPath(perftrace)}
	for i, event := range result.Events {
		if event.SourcePath != wantPaths[i] || event.LocalLine != 2 || event.TimeDomain != "trace_seconds" {
			t.Fatalf("event %d provenance: %+v", i, event)
		}
		if !strings.Contains(event.Raw, map[bool]string{true: "perf_sample", false: "sched_switch"}[i == 1]) {
			t.Fatalf("event %d raw line was loaded from the wrong physical artifact: %q", i, event.Raw)
		}
		if len(result.EvidencePack[i].SourceSpans) != 1 || result.EvidencePack[i].SourceSpans[0].SourcePath != wantPaths[i] || result.EvidencePack[i].SourceSpans[0].LocalLineStart != 2 {
			t.Fatalf("evidence fact %d provenance: %+v", i, result.EvidencePack[i].SourceSpans)
		}
	}
}

func TestTraceBundleAppliesCalibratedAffineClockMapBeforeMerge(t *testing.T) {
	dir := t.TempDir()
	systrace := filepath.Join(dir, "affine.systrace")
	perftrace := filepath.Join(dir, "affine.perftrace")
	bundle := filepath.Join(dir, "affine.tracebundle.json")
	writeBundleProvenanceFixture(t, systrace, `
 app-20 (20) [001] .... 30.000000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=20 next_prio=53
`)
	writeBundleProvenanceFixture(t, perftrace, `
 app-20 (20) [001] .... 100.001000: perf_sample: cpu=1 pid=20 tid=20 period=7000 event=cpu-cycles symbol=App::layout dso=libapp.so source=test
`)
	writeBundleProvenanceFixture(t, bundle, `{
  "version":"test",
  "systrace":"affine.systrace",
  "artifacts":[
    {"type":"systrace","path":"affine.systrace"},
    {"type":"perftrace","path":"affine.perftrace","perf_capability":{"time_domain":"perf_event_time","trace_query_ready":true}}
  ],
  "perf_clock_alignments":[
    {"artifact_path":"affine.perftrace","perf_time_domain":"perf_event_time","trace_time_domain":"trace_seconds","offset_sec":-70.0,"slope":1.0,"confidence":"calibrated","calibrated":true,"source":"fixture"}
  ]
}`)

	idx, err := BuildIndexWithOptions(context.Background(), bundle, BuildOptions{
		AllowWindowedParse: true,
		TimeStartSet:       true,
		TimeEndSet:         true,
		TimeStart:          30.0,
		TimeEnd:            30.002,
		MaxEvents:          100,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(idx.Events) != 2 {
		t.Fatalf("inverse window mapping must retain both source-domain rows: %+v", idx.Events)
	}
	var perfSource *TraceArtifactSource
	for i := range idx.TraceArtifacts {
		if idx.TraceArtifacts[i].Kind == "perftrace" {
			perfSource = &idx.TraceArtifacts[i]
		}
	}
	if perfSource == nil || !perfSource.CausalCompatible || perfSource.ClockAlignment != TraceClockAlignmentAffine {
		t.Fatalf("perf source admission: %+v", idx.TraceArtifacts)
	}
	if got := perfSource.ToCanonicalTs(100.001); math.Abs(got-30.001) > 1e-9 || math.Abs(perfSource.ToSourceTs(got)-100.001) > 1e-9 {
		t.Fatalf("affine map is not reversible: canonical=%.9f source=%.9f", got, perfSource.ToSourceTs(got))
	}
	result := Run(idx, Query{View: "event_search", EventTypes: []EventType{EventPerfSample}, TimeStart: 30.0, TimeEnd: 30.002, Limit: 10})
	if len(result.Events) != 1 {
		t.Fatalf("canonical-window search missed calibrated perf row: %+v", result.Events)
	}
	event := result.Events[0]
	if math.Abs(event.Ts-30.001) > 1e-9 || math.Abs(event.SourceTs-100.001) > 1e-9 || !event.ClockAligned || event.TimeDomain != "perf_event_time" || event.CanonicalTimeDomain != "trace_seconds" {
		t.Fatalf("calibrated event provenance: %+v", event)
	}
	if !strings.Contains(strings.Join(result.Caveats, "\n"), "tracebundle_clock_alignment_applied") {
		t.Fatalf("applied mapping must be disclosed: %+v", result.Caveats)
	}
}

func TestAffineWindowInverseRoundsOutwardAcrossAdmissionULPs(t *testing.T) {
	dir := t.TempDir()
	systrace := filepath.Join(dir, "boundary.systrace")
	perftrace := filepath.Join(dir, "boundary.perftrace")
	bundle := filepath.Join(dir, "boundary.tracebundle.json")
	writeBundleProvenanceFixture(t, systrace, `# tracer: nop`)
	writeBundleProvenanceFixture(t, perftrace, `app-20 (20) [001] .... 2926561.480787572: perf_sample: cpu=1 pid=20 tid=20 period=1 event=cpu-cycles symbol=Boundary dso=lib.so source=test`)
	writeBundleProvenanceFixture(t, bundle, `{
  "version":"test",
  "systrace":"boundary.systrace",
  "artifacts":[
    {"type":"systrace","path":"boundary.systrace"},
    {"type":"perftrace","path":"boundary.perftrace","perf_capability":{"time_domain":"perf_event_time","trace_query_ready":true}}
  ],
  "perf_clock_alignments":[
    {"artifact_path":"boundary.perftrace","perf_time_domain":"perf_event_time","trace_time_domain":"trace_seconds","offset_sec":776172633.04783404,"slope":48.687019449261946,"calibrated":true}
  ]
}`)
	canonical := 918658188.78239942
	idx, err := BuildIndexWithOptions(context.Background(), bundle, BuildOptions{
		AllowWindowedParse: true,
		TimeStart:          canonical, TimeStartSet: true,
		TimeEnd: canonical, TimeEndSet: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(idx.Events) != 1 || idx.Events[0].Type != EventPerfSample || idx.Events[0].Ts != canonical {
		t.Fatalf("inclusive canonical boundary event was lost to inverse rounding: %+v", idx.Events)
	}
}

func TestTraceBundleRejectsCalibratedFlagWithoutAffineCoefficients(t *testing.T) {
	dir := t.TempDir()
	systrace := filepath.Join(dir, "invalid.systrace")
	perftrace := filepath.Join(dir, "invalid.perftrace")
	bundle := filepath.Join(dir, "invalid.tracebundle.json")
	writeBundleProvenanceFixture(t, systrace, `
 app-20 (20) [001] .... 20.000000: sched_wakeup: comm=app pid=20 prio=53 target_cpu=001
`)
	writeBundleProvenanceFixture(t, perftrace, `
 app-20 (20) [001] .... 20.000000: perf_sample: cpu=1 pid=20 tid=20 period=1 event=cpu-cycles symbol=Coincident dso=lib.so source=test
`)
	writeBundleProvenanceFixture(t, bundle, `{
  "version":"test",
  "systrace":"invalid.systrace",
  "artifacts":[
    {"type":"systrace","path":"invalid.systrace"},
    {"type":"perftrace","path":"invalid.perftrace","perf_capability":{"time_domain":"perf_event_time","trace_query_ready":true}}
  ],
  "perf_clock_alignments":[
    {"artifact_path":"invalid.perftrace","perf_time_domain":"perf_event_time","trace_time_domain":"trace_seconds","confidence":"calibrated","calibrated":true}
  ]
}`)

	idx, err := BuildIndex(context.Background(), bundle)
	if err != nil {
		t.Fatal(err)
	}
	if len(idx.Events) != 1 || len(idx.TraceArtifacts) != 2 || idx.TraceArtifacts[1].CausalCompatible {
		t.Fatalf("a label without coefficients must not authorize cross-domain merge: events=%+v artifacts=%+v", idx.Events, idx.TraceArtifacts)
	}
	if !strings.Contains(idx.TraceArtifacts[1].IsolationReason, "missing a finite affine coefficient") {
		t.Fatalf("unexpected isolation reason: %+v", idx.TraceArtifacts[1])
	}
	standalone, err := BuildIndex(context.Background(), perftrace)
	if err != nil || len(standalone.Events) != 1 || standalone.Events[0].Type != EventPerfSample {
		t.Fatalf("an explicit perftrace path must bypass sibling-bundle promotion for isolated analysis: idx=%+v err=%v", standalone, err)
	}
	if _, err := StreamEventSearch(context.Background(), bundle, Query{View: "event_search", Limit: 10}); err == nil || !strings.Contains(err.Error(), "single physical artifact") {
		t.Fatalf("streaming a manifest must fail honestly instead of parsing JSON as trace rows: %v", err)
	}
}

func TestInvalidClockAlignmentCannotRelabelPrimaryOrAdmitThirdArtifact(t *testing.T) {
	floatPtr := func(value float64) *float64 { return &value }
	tests := []struct {
		name       string
		calibrated bool
		offset     *float64
		slope      *float64
	}{
		{name: "uncalibrated_cross_domain", calibrated: false, offset: floatPtr(20), slope: floatPtr(1)},
		{name: "missing_coefficients", calibrated: true},
		{name: "zero_slope", calibrated: true, offset: floatPtr(0), slope: floatPtr(0)},
		{name: "negative_slope", calibrated: true, offset: floatPtr(0), slope: floatPtr(-1)},
		{name: "nan_offset", calibrated: true, offset: floatPtr(math.NaN()), slope: floatPtr(1)},
		{name: "infinite_slope", calibrated: true, offset: floatPtr(0), slope: floatPtr(math.Inf(1))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			bundlePath := filepath.Join(dir, "authority.tracebundle.json")
			bundle := traceBundleFile{
				Systrace: "primary.systrace",
				Artifacts: []traceBundleArtifact{
					{Type: "systrace", Path: "primary.systrace"},
					{Type: "perftrace", Path: "invalid.perftrace", Perf: &traceBundlePerfCapability{TimeDomain: "bad_clock", TraceQueryReady: true}},
					// This artifact has no alignment of its own. It must not become
					// causally compatible merely because the invalid record above
					// renamed the primary to its claimed target domain.
					{Type: "perftrace", Path: "third.perftrace", Perf: &traceBundlePerfCapability{TimeDomain: "boottime", TraceQueryReady: true}},
				},
				PerfClockAlignments: []traceBundlePerfClockAlignment{{
					ArtifactPath: "invalid.perftrace", PerfTimeDomain: "bad_clock", TraceTimeDomain: "boottime",
					OffsetSec: tt.offset, Slope: tt.slope, Calibrated: tt.calibrated,
				}},
			}

			specs := traceBundleArtifactSpecs(bundlePath, bundle)
			if len(specs) != 3 {
				t.Fatalf("artifact specs=%+v", specs)
			}
			primary, invalid, third := specs[0].source, specs[1].source, specs[2].source
			if primary.TimeDomain != "trace_seconds" || primary.CanonicalTimeDomain != "trace_seconds" || !primary.CausalCompatible || primary.ClockAlignment != TraceClockAlignmentIdentity {
				t.Fatalf("invalid mapping relabeled primary trace: %+v", primary)
			}
			if invalid.CausalCompatible || invalid.ClockAlignment != TraceClockAlignmentIsolated {
				t.Fatalf("invalid mapping entered causal timeline: %+v", invalid)
			}
			if third.TimeDomain != "boottime" || third.CanonicalTimeDomain != "trace_seconds" || third.CausalCompatible || third.ClockAlignment != TraceClockAlignmentIsolated {
				t.Fatalf("invalid mapping enabled unrelated third artifact: %+v", third)
			}
		})
	}
}

func TestValidCalibratedAlignmentMayDefineCanonicalTraceDomain(t *testing.T) {
	dir := t.TempDir()
	systrace := filepath.Join(dir, "valid-domain.systrace")
	perftrace := filepath.Join(dir, "valid-domain.perftrace")
	bundle := filepath.Join(dir, "valid-domain.tracebundle.json")
	writeBundleProvenanceFixture(t, systrace, `app-20 (20) [001] .... 30.000000: sched_wakeup: comm=app pid=20 prio=20 target_cpu=001`)
	writeBundleProvenanceFixture(t, perftrace, `app-20 (20) [001] .... 10.001000: perf_sample: cpu=1 pid=20 tid=20 period=1 event=cpu-cycles symbol=Mapped dso=lib.so source=test`)
	writeBundleProvenanceFixture(t, bundle, `{
  "version":"test",
  "systrace":"valid-domain.systrace",
  "artifacts":[
    {"type":"systrace","path":"valid-domain.systrace"},
    {"type":"perftrace","path":"valid-domain.perftrace","perf_capability":{"time_domain":"perf_event_time","trace_query_ready":true}}
  ],
  "perf_clock_alignments":[
    {"artifact_path":"valid-domain.perftrace","perf_time_domain":"perf_event_time","trace_time_domain":"boottime","offset_sec":20.0,"slope":1.0,"calibrated":true}
  ]
}`)

	idx, err := BuildIndex(context.Background(), bundle)
	if err != nil {
		t.Fatal(err)
	}
	if len(idx.Events) != 2 || len(idx.TraceArtifacts) != 2 {
		t.Fatalf("valid mapped bundle shape: events=%+v artifacts=%+v", idx.Events, idx.TraceArtifacts)
	}
	primary, perf := idx.TraceArtifacts[0], idx.TraceArtifacts[1]
	if primary.TimeDomain != "boottime" || primary.CanonicalTimeDomain != "boottime" || !primary.CausalCompatible {
		t.Fatalf("valid mapping did not define canonical domain: %+v", primary)
	}
	if !perf.CausalCompatible || perf.ClockAlignment != TraceClockAlignmentAffine || perf.CanonicalTimeDomain != "boottime" || math.Abs(idx.Events[1].Ts-30.001) > 1e-9 {
		t.Fatalf("valid mapping was not admitted: events=%+v artifact=%+v", idx.Events, perf)
	}
}

func TestTraceBundleAdmitsOnlyOneSystraceCausalAuthority(t *testing.T) {
	dir := t.TempDir()
	primaryPath := filepath.Join(dir, "primary.systrace")
	secondaryPath := filepath.Join(dir, "secondary.systrace")
	bundlePath := filepath.Join(dir, "multi-systrace.tracebundle.json")
	writeBundleProvenanceFixture(t, primaryPath, `app-20 (20) [001] .... 10.000000: sched_wakeup: comm=app pid=20 prio=20 target_cpu=001`)
	writeBundleProvenanceFixture(t, secondaryPath, `other-30 (30) [002] .... 10.001000: sched_wakeup: comm=other pid=30 prio=20 target_cpu=002`)
	writeBundleProvenanceFixture(t, bundlePath, `{
  "version":"test",
  "systrace":"primary.systrace",
  "artifacts":[
    {"type":"systrace","path":"primary.systrace"},
    {"type":"systrace","path":"secondary.systrace"}
  ]
}`)

	idx, err := BuildIndex(context.Background(), bundlePath)
	if err != nil {
		t.Fatal(err)
	}
	if len(idx.Events) != 1 || len(idx.TraceArtifacts) != 2 || idx.Events[0].PID != 20 {
		t.Fatalf("unproven second systrace entered causal timeline: events=%+v artifacts=%+v", idx.Events, idx.TraceArtifacts)
	}
	primary, secondary := idx.TraceArtifacts[0], idx.TraceArtifacts[1]
	if !primary.CausalCompatible || primary.ClockAlignment != TraceClockAlignmentIdentity {
		t.Fatalf("primary systrace lost authority: %+v", primary)
	}
	if secondary.CausalCompatible || secondary.ClockAlignment != TraceClockAlignmentIsolated || !strings.Contains(secondary.IsolationReason, "only one systrace causal authority") {
		t.Fatalf("second systrace was not explicitly isolated: %+v", secondary)
	}
	if !containsSubstring(idx.Caveats, "only one systrace causal authority") {
		t.Fatalf("second-systrace isolation was not disclosed: %+v", idx.Caveats)
	}

	standalone, err := BuildIndex(context.Background(), secondaryPath)
	if err != nil || len(standalone.Events) != 1 || standalone.Events[0].PID != 30 {
		t.Fatalf("isolated systrace must remain directly queryable: idx=%+v err=%v", standalone, err)
	}
}

func TestCompositeCanonicalSortPreservesPhysicalSchedulerRollbackPoison(t *testing.T) {
	dir := t.TempDir()
	systrace := filepath.Join(dir, "rollback.systrace")
	bundle := filepath.Join(dir, "rollback.tracebundle.json")
	writeBundleProvenanceFixture(t, systrace, strings.Join([]string{
		`idle-0 (0) [000] .... 2.000000: sched_switch: prev_comm=idle/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=20 next_prio=20`,
		`app-20 (20) [000] .... 2.600000: sched_switch: prev_comm=app prev_pid=20 prev_prio=20 prev_state=S ==> next_comm=idle/0 next_pid=0 next_prio=120`,
		`idle-0 (0) [000] .... 2.400000: sched_switch: prev_comm=idle/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=20 next_prio=20`,
	}, "\n"))
	writeBundleProvenanceFixture(t, bundle, `{
  "version":"test",
  "systrace":"rollback.systrace",
  "artifacts":[{"type":"systrace","path":"rollback.systrace"}]
}`)
	idx, err := BuildIndex(context.Background(), bundle)
	if err != nil {
		t.Fatal(err)
	}
	if idx.TimestampOrder != TraceTimestampOrderMonotonic || len(idx.schedulerOrderFailures) == 0 {
		t.Fatalf("canonical merge sort must retain the child-local rollback poison: order=%v failures=%+v", idx.TimestampOrder, idx.schedulerOrderFailures)
	}
	stats := ComputeWindowStats(idx, Query{TimeStart: 1.9, TimeEnd: 2.7})
	if len(stats.CPU) != 0 || len(stats.TopRunning) != 0 || !containsSubstring(stats.Caveats, "scheduler_duration_fail_closed=true") {
		t.Fatalf("sorted composite fabricated scheduler durations across a physical rollback: %+v", stats)
	}
}

func TestCompositeCanonicalSortPreservesPhysicalLifecycleConflict(t *testing.T) {
	dir := t.TempDir()
	systrace := filepath.Join(dir, "lifecycle.systrace")
	bundle := filepath.Join(dir, "lifecycle.tracebundle.json")
	writeBundleProvenanceFixture(t, systrace, strings.Join([]string{
		`old-42 (700) [000] .... 2.500000: sched_switch: prev_comm=idle/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=old next_pid=42 next_prio=20`,
		// Physical order is authoritative for lifecycle. Canonical timestamp
		// sorting moves this creation before the old occupant's row and would
		// otherwise erase the reuse proof.
		`creator-7 (7) [001] .... 2.400000: sched_wakeup_new: comm=new pid=42 prio=30 target_cpu=001`,
	}, "\n"))
	writeBundleProvenanceFixture(t, bundle, `{
  "version":"test",
  "systrace":"lifecycle.systrace",
  "artifacts":[{"type":"systrace","path":"lifecycle.systrace"}]
}`)

	idx, err := BuildIndex(context.Background(), bundle)
	if err != nil {
		t.Fatal(err)
	}
	if len(idx.threadIncarnationFailures) != 1 || idx.threadIncarnationFailures[0].PID != 42 {
		t.Fatalf("canonical merge sort lost the child-local lifecycle poison: %+v", idx.threadIncarnationFailures)
	}
	timeline := ThreadTimeline(idx, Query{PID: 42, TimeStart: 2.3, TimeEnd: 2.6})
	if timeline.IntegrityFailure != "thread_incarnation_conflict" || len(timeline.Intervals) != 0 {
		t.Fatalf("composite lifecycle reuse fabricated a single thread: %+v", timeline)
	}
}

func TestWarmCompositeWindowPreservesPhysicalAuditPoison(t *testing.T) {
	t.Run("scheduler rollback", func(t *testing.T) {
		dir := t.TempDir()
		systrace := filepath.Join(dir, "warm-rollback.systrace")
		bundle := filepath.Join(dir, "warm-rollback.tracebundle.json")
		writeBundleProvenanceFixture(t, systrace, strings.Join([]string{
			`idle-0 (0) [000] .... 2.000000: sched_switch: prev_comm=idle/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=20 next_prio=20`,
			`app-20 (20) [000] .... 2.600000: sched_switch: prev_comm=app prev_pid=20 prev_prio=20 prev_state=S ==> next_comm=idle/0 next_pid=0 next_prio=120`,
			`idle-0 (0) [000] .... 2.400000: sched_switch: prev_comm=idle/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=20 next_prio=20`,
		}, "\n"))
		writeBundleProvenanceFixture(t, bundle, `{"version":"test","systrace":"warm-rollback.systrace","artifacts":[{"type":"systrace","path":"warm-rollback.systrace"}]}`)
		if _, err := BuildIndex(context.Background(), bundle); err != nil {
			t.Fatal(err)
		}
		windowed, err := BuildIndexWithOptions(context.Background(), bundle, BuildOptions{
			AllowWindowedParse: true, TimeStartSet: true, TimeStart: 1.9, TimeEndSet: true, TimeEnd: 2.7,
		})
		if err != nil {
			t.Fatal(err)
		}
		if !windowed.Windowed || len(windowed.schedulerOrderFailures) == 0 {
			t.Fatalf("warm derived composite lost physical rollback poison: %+v", windowed.schedulerOrderFailures)
		}
		stats := ComputeWindowStats(windowed, Query{TimeStart: 1.9, TimeEnd: 2.7})
		if len(stats.CPU) != 0 || !containsSubstring(stats.Caveats, "scheduler_duration_fail_closed=true") {
			t.Fatalf("warm derived composite fabricated scheduler durations: %+v", stats)
		}
	})

	t.Run("thread lifecycle", func(t *testing.T) {
		dir := t.TempDir()
		systrace := filepath.Join(dir, "warm-lifecycle.systrace")
		bundle := filepath.Join(dir, "warm-lifecycle.tracebundle.json")
		writeBundleProvenanceFixture(t, systrace, strings.Join([]string{
			`old-42 (700) [000] .... 2.500000: sched_switch: prev_comm=idle/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=old next_pid=42 next_prio=20`,
			`creator-7 (7) [001] .... 2.400000: sched_wakeup_new: comm=new pid=42 prio=30 target_cpu=001`,
		}, "\n"))
		writeBundleProvenanceFixture(t, bundle, `{"version":"test","systrace":"warm-lifecycle.systrace","artifacts":[{"type":"systrace","path":"warm-lifecycle.systrace"}]}`)
		if _, err := BuildIndex(context.Background(), bundle); err != nil {
			t.Fatal(err)
		}
		windowed, err := BuildIndexWithOptions(context.Background(), bundle, BuildOptions{
			AllowWindowedParse: true, TimeStartSet: true, TimeStart: 2.3, TimeEndSet: true, TimeEnd: 2.6,
		})
		if err != nil {
			t.Fatal(err)
		}
		if !windowed.Windowed || len(windowed.threadIncarnationFailures) == 0 {
			t.Fatalf("warm derived composite lost physical lifecycle poison: %+v", windowed.threadIncarnationFailures)
		}
		timeline := ThreadTimeline(windowed, Query{PID: 42, TimeStart: 2.3, TimeEnd: 2.6})
		if timeline.IntegrityFailure != "thread_incarnation_conflict" || len(timeline.Intervals) != 0 {
			t.Fatalf("warm derived composite merged TID incarnations: %+v", timeline)
		}
	})
}

func TestCompositeLifecycleAuditRejectsUnattestedPerfSchedulerRowsColdAndWarm(t *testing.T) {
	buildFixture := func(t *testing.T, name string) string {
		t.Helper()
		dir := t.TempDir()
		systrace := filepath.Join(dir, name+".systrace")
		perftrace := filepath.Join(dir, name+".perftrace")
		bundle := filepath.Join(dir, name+".tracebundle.json")
		writeBundleProvenanceFixture(t, systrace,
			`old-42 (700) [000] .... 2.000000: sched_switch: prev_comm=idle/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=old next_pid=42 next_prio=20`)
		// Clock compatibility does not attest scheduler provenance. A perf
		// capability describes normalized samples only, so this creation-shaped
		// row must not mint a cross-artifact lifecycle conflict.
		writeBundleProvenanceFixture(t, perftrace,
			`creator-7 (7) [001] .... 2.100000: sched_wakeup_new: comm=new pid=42 prio=30 target_cpu=001`)
		writeBundleProvenanceFixture(t, bundle, fmt.Sprintf(`{
  "version":"test",
  "systrace":%q,
  "artifacts":[
    {"type":"systrace","path":%q},
    {"type":"perftrace","path":%q,"perf_capability":{"time_domain":"trace_seconds","trace_query_ready":true}}
  ],
  "perf_clock_alignments":[
    {"artifact_path":%q,"perf_time_domain":"trace_seconds","trace_time_domain":"trace_seconds","confidence":"same_domain","calibrated":false}
  ]
}`, filepath.Base(systrace), filepath.Base(systrace), filepath.Base(perftrace), filepath.Base(perftrace)))
		return bundle
	}

	assertRejected := func(t *testing.T, idx *Index) {
		t.Helper()
		if idx == nil || len(idx.threadIncarnationFailures) != 0 {
			t.Fatalf("unattested perf scheduler row minted lifecycle proof: %+v", idx)
		}
		if len(idx.Events) != 1 || idx.Events[0].Type != EventSchedSwitch || idx.Events[0].NextPID != 42 {
			t.Fatalf("perf scheduler row entered shared events: %+v", idx.Events)
		}
		timeline := ThreadTimeline(idx, Query{PID: 42, TimeStart: 1.9, TimeEnd: 2.2})
		if timeline.IntegrityFailure != "" || len(timeline.Intervals) != 1 || timeline.Intervals[0].State != StateRunning {
			t.Fatalf("unattested perf row changed the primary scheduler timeline: %+v", timeline)
		}
		if !strings.Contains(strings.Join(idx.Caveats, "\n"), "scheduler_or_cpu_rows_omitted=1") {
			t.Fatalf("perf scheduler rejection was not disclosed: %+v", idx.Caveats)
		}
	}

	t.Run("cold window parse", func(t *testing.T) {
		bundle := buildFixture(t, "cold-cross-artifact")
		idx, err := BuildIndexWithOptions(context.Background(), bundle, BuildOptions{
			AllowWindowedParse: true, TimeStartSet: true, TimeStart: 1.9, TimeEndSet: true, TimeEnd: 2.2,
		})
		if err != nil {
			t.Fatal(err)
		}
		assertRejected(t, idx)
	})

	t.Run("warm full-cache derive", func(t *testing.T) {
		bundle := buildFixture(t, "warm-cross-artifact")
		full, err := BuildIndex(context.Background(), bundle)
		if err != nil {
			t.Fatal(err)
		}
		assertRejected(t, full)
		windowed, err := BuildIndexWithOptions(context.Background(), bundle, BuildOptions{
			AllowWindowedParse: true, TimeStartSet: true, TimeStart: 1.9, TimeEndSet: true, TimeEnd: 2.2,
		})
		if err != nil {
			t.Fatal(err)
		}
		if !windowed.Windowed {
			t.Fatal("expected warm full-cache window derivation")
		}
		assertRejected(t, windowed)
	})
}

func TestCompositeSchedulerAuditOverflowFailsClosed(t *testing.T) {
	dir := t.TempDir()
	systrace := filepath.Join(dir, "overflow-order.systrace")
	bundle := filepath.Join(dir, "overflow-order.tracebundle.json")
	var body strings.Builder
	for i := 0; i < schedulerOrderFailureCap/2+2; i++ {
		high := 10.0 + float64(i*2)
		low := high - 1.0
		fmt.Fprintf(&body, "idle-0 (0) [000] .... %.6f: sched_switch: prev_comm=idle/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=20 next_prio=20\n", high)
		fmt.Fprintf(&body, "idle-0 (0) [000] .... %.6f: sched_switch: prev_comm=idle/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=20 next_prio=20\n", low)
	}
	writeBundleProvenanceFixture(t, systrace, body.String())
	writeBundleProvenanceFixture(t, bundle, `{
  "version":"test",
  "systrace":"overflow-order.systrace",
  "artifacts":[{"type":"systrace","path":"overflow-order.systrace"}]
}`)

	idx, err := BuildIndex(context.Background(), bundle)
	if err != nil {
		t.Fatal(err)
	}
	if len(idx.schedulerOrderFailures) != schedulerOrderFailureCap || !idx.schedulerOrderFailuresCapped {
		t.Fatalf("overflowed physical audit must retain a truncation poison: count=%d capped=%v", len(idx.schedulerOrderFailures), idx.schedulerOrderFailuresCapped)
	}
	stats := ComputeWindowStats(idx, Query{TimeStart: 0, TimeEnd: 100})
	if len(stats.CPU) != 0 || len(stats.TopRunning) != 0 || !containsSubstring(stats.Caveats, "scheduler_duration_fail_closed=true") {
		t.Fatalf("truncated rollback audit published duration aggregates: %+v", stats)
	}
}

func TestCompositeIndexCacheInvalidatesWhenChildArtifactChanges(t *testing.T) {
	dir := t.TempDir()
	systrace := filepath.Join(dir, "cache.systrace")
	perftrace := filepath.Join(dir, "cache.perftrace")
	bundle := filepath.Join(dir, "cache.tracebundle.json")
	writeBundleProvenanceFixture(t, systrace, `app-20 (20) [001] .... 10.000000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=20 next_prio=53`)
	writeBundleProvenanceFixture(t, perftrace, `app-20 (20) [001] .... 10.001000: perf_sample: cpu=1 pid=20 tid=20 period=1 event=cpu-cycles symbol=Old dso=lib.so source=test`)
	writeBundleProvenanceFixture(t, bundle, `{
  "version":"test",
  "systrace":"cache.systrace",
  "artifacts":[
    {"type":"systrace","path":"cache.systrace"},
    {"type":"perftrace","path":"cache.perftrace","perf_capability":{"time_domain":"trace_seconds","trace_query_ready":true}}
  ]
}`)
	first, err := BuildIndex(context.Background(), bundle)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Events) != 2 || first.Events[1].PerfFields == nil || first.Events[1].PerfFields.Symbol != "Old" {
		t.Fatalf("initial bundle parse: %+v", first.Events)
	}
	writeBundleProvenanceFixture(t, perftrace, `app-20 (20) [001] .... 10.001000: perf_sample: cpu=1 pid=20 tid=20 period=1 event=cpu-cycles symbol=New dso=lib.so source=test`)
	stamp := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(perftrace, stamp, stamp); err != nil {
		t.Fatal(err)
	}
	second, err := BuildIndex(context.Background(), bundle)
	if err != nil {
		t.Fatal(err)
	}
	if second == first || len(second.Events) != 2 || second.Events[1].PerfFields == nil || second.Events[1].PerfFields.Symbol != "New" {
		t.Fatalf("child mutation served stale composite cache: first=%p second=%p events=%+v", first, second, second.Events)
	}
}

func TestSiblingArtifactUniverseCannotUseSingleFileStreaming(t *testing.T) {
	dir := t.TempDir()
	systrace := filepath.Join(dir, "sibling.systrace")
	perftrace := filepath.Join(dir, "sibling.perftrace")
	writeBundleProvenanceFixture(t, systrace, `app-20 (20) [001] .... 10.000000: sched_wakeup: comm=app pid=20 prio=20 target_cpu=001`)
	writeBundleProvenanceFixture(t, perftrace, `app-20 (20) [001] .... 10.001000: perf_sample: cpu=1 pid=20 tid=20 period=1 event=cpu-cycles symbol=App dso=lib.so source=test`)
	if !tracePathRequiresCompositeIndex(systrace) {
		t.Fatal("systrace with a sibling artifact must be treated as a composite source universe")
	}
	if !TracePathRequiresCompositeIndex(systrace) {
		t.Fatal("exported composite guard must agree with the engine admission guard")
	}
	if tracePathRequiresCompositeIndex(perftrace) {
		t.Fatal("explicit perftrace must remain the per-domain single-artifact escape hatch")
	}
	if _, err := StreamEventSearch(context.Background(), systrace, Query{View: "event_search", Limit: 10}); err == nil || !strings.Contains(err.Error(), "single physical artifact") {
		t.Fatalf("streaming must refuse to bypass sibling provenance: %v", err)
	}
}

func TestCompositeCacheabilityUsesTotalArtifactBytes(t *testing.T) {
	dir := t.TempDir()
	systrace := filepath.Join(dir, "large.systrace")
	perftrace := filepath.Join(dir, "large.perftrace")
	if err := os.WriteFile(systrace, []byte("small"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(perftrace, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Truncate(perftrace, maxCachedTraceIndexBytes+1); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(systrace)
	if err != nil {
		t.Fatal(err)
	}
	total, key, err := traceIndexSourceIdentity(systrace, info)
	if err != nil {
		t.Fatal(err)
	}
	if total <= maxCachedTraceIndexBytes || key == "" || shouldCacheTraceIndex(total, BuildOptions{}) {
		t.Fatalf("composite cacheability used primary size instead of total: total=%d key=%q", total, key)
	}
}

func TestSameDomainExplicitAffineMapIsApplied(t *testing.T) {
	dir := t.TempDir()
	systrace := filepath.Join(dir, "same-domain.systrace")
	perftrace := filepath.Join(dir, "same-domain.perftrace")
	bundle := filepath.Join(dir, "same-domain.tracebundle.json")
	writeBundleProvenanceFixture(t, systrace, `app-20 (20) [001] .... 30.000000: sched_wakeup: comm=app pid=20 prio=20 target_cpu=001`)
	writeBundleProvenanceFixture(t, perftrace, `app-20 (20) [001] .... 10.001000: perf_sample: cpu=1 pid=20 tid=20 period=1 event=cpu-cycles symbol=Mapped dso=lib.so source=test`)
	writeBundleProvenanceFixture(t, bundle, `{
  "version":"test",
  "systrace":"same-domain.systrace",
  "artifacts":[
    {"type":"systrace","path":"same-domain.systrace"},
    {"type":"perftrace","path":"same-domain.perftrace","perf_capability":{"time_domain":"trace_seconds","trace_query_ready":true}}
  ],
  "perf_clock_alignments":[
    {"artifact_path":"same-domain.perftrace","perf_time_domain":"trace_seconds","trace_time_domain":"trace_seconds","offset_sec":20.0,"slope":1.0,"calibrated":true,"confidence":"calibrated"}
  ]
}`)
	idx, err := BuildIndex(context.Background(), bundle)
	if err != nil {
		t.Fatal(err)
	}
	if len(idx.Events) != 2 || math.Abs(idx.Events[1].Ts-30.001) > 1e-9 {
		t.Fatalf("same-domain non-identity map was ignored: %+v", idx.Events)
	}
	if len(idx.TraceArtifacts) != 2 || idx.TraceArtifacts[1].ClockAlignment != TraceClockAlignmentAffine {
		t.Fatalf("same-domain explicit map must be disclosed as affine: %+v", idx.TraceArtifacts)
	}
}

func TestSameDomainInvalidCalibratedMapIsIsolated(t *testing.T) {
	dir := t.TempDir()
	systrace := filepath.Join(dir, "same-invalid.systrace")
	perftrace := filepath.Join(dir, "same-invalid.perftrace")
	bundle := filepath.Join(dir, "same-invalid.tracebundle.json")
	writeBundleProvenanceFixture(t, systrace, `app-20 (20) [001] .... 10.000000: sched_wakeup: comm=app pid=20 prio=20 target_cpu=001`)
	writeBundleProvenanceFixture(t, perftrace, `app-20 (20) [001] .... 10.001000: perf_sample: cpu=1 pid=20 tid=20 period=1 event=cpu-cycles symbol=Bad dso=lib.so source=test`)
	writeBundleProvenanceFixture(t, bundle, `{
  "version":"test",
  "systrace":"same-invalid.systrace",
  "artifacts":[
    {"type":"systrace","path":"same-invalid.systrace"},
    {"type":"perftrace","path":"same-invalid.perftrace","perf_capability":{"time_domain":"trace_seconds","trace_query_ready":true}}
  ],
  "perf_clock_alignments":[
    {"artifact_path":"same-invalid.perftrace","perf_time_domain":"trace_seconds","trace_time_domain":"trace_seconds","offset_sec":0.0,"slope":0.0,"calibrated":true}
  ]
}`)
	idx, err := BuildIndex(context.Background(), bundle)
	if err != nil {
		t.Fatal(err)
	}
	if len(idx.Events) != 1 || len(idx.TraceArtifacts) != 2 || idx.TraceArtifacts[1].CausalCompatible || !strings.Contains(idx.TraceArtifacts[1].IsolationReason, "finite affine") {
		t.Fatalf("invalid same-domain mapping must fail closed: events=%+v artifacts=%+v", idx.Events, idx.TraceArtifacts)
	}
}

func TestConflictingDuplicateAlignmentIsIsolated(t *testing.T) {
	dir := t.TempDir()
	systrace := filepath.Join(dir, "dup.systrace")
	perftrace := filepath.Join(dir, "dup.perftrace")
	bundle := filepath.Join(dir, "dup.tracebundle.json")
	writeBundleProvenanceFixture(t, systrace, `app-20 (20) [001] .... 30.000000: sched_wakeup: comm=app pid=20 prio=20 target_cpu=001`)
	writeBundleProvenanceFixture(t, perftrace, `app-20 (20) [001] .... 10.000000: perf_sample: cpu=1 pid=20 tid=20 period=1 event=cpu-cycles symbol=Dup dso=lib.so source=test`)
	writeBundleProvenanceFixture(t, bundle, `{
  "version":"test",
  "systrace":"dup.systrace",
  "artifacts":[
    {"type":"systrace","path":"dup.systrace"},
    {"type":"perftrace","path":"dup.perftrace","perf_capability":{"time_domain":"perf_event_time","trace_query_ready":true}}
  ],
  "perf_clock_alignments":[
    {"artifact_path":"dup.perftrace","perf_time_domain":"perf_event_time","trace_time_domain":"trace_seconds","offset_sec":20.0,"slope":1.0,"calibrated":true},
    {"artifact_path":"dup.perftrace","perf_time_domain":"perf_event_time","trace_time_domain":"trace_seconds","offset_sec":21.0,"slope":1.0,"calibrated":true}
  ]
}`)
	idx, err := BuildIndex(context.Background(), bundle)
	if err != nil {
		t.Fatal(err)
	}
	if len(idx.Events) != 1 || idx.TraceArtifacts[1].CausalCompatible || !strings.Contains(idx.TraceArtifacts[1].IsolationReason, "conflicting duplicate") {
		t.Fatalf("duplicate last-wins alignment must be rejected: events=%+v artifacts=%+v", idx.Events, idx.TraceArtifacts)
	}
}

func TestConflictingDuplicateAlignmentCannotRelabelPrimaryTrace(t *testing.T) {
	dir := t.TempDir()
	systrace := filepath.Join(dir, "dup-domain.systrace")
	perftrace := filepath.Join(dir, "dup-domain.perftrace")
	bundle := filepath.Join(dir, "dup-domain.tracebundle.json")
	writeBundleProvenanceFixture(t, systrace, `app-20 (20) [001] .... 30.000000: sched_wakeup: comm=app pid=20 prio=20 target_cpu=001`)
	writeBundleProvenanceFixture(t, perftrace, `app-20 (20) [001] .... 10.000000: perf_sample: cpu=1 pid=20 tid=20 period=1 event=cpu-cycles symbol=Dup dso=lib.so source=test`)
	writeBundleProvenanceFixture(t, bundle, `{
  "version":"test",
  "systrace":"dup-domain.systrace",
  "artifacts":[
    {"type":"systrace","path":"dup-domain.systrace"},
    {"type":"perftrace","path":"dup-domain.perftrace","perf_capability":{"time_domain":"perf_event_time","trace_query_ready":true}}
  ],
  "perf_clock_alignments":[
    {"artifact_path":"dup-domain.perftrace","perf_time_domain":"perf_event_time","trace_time_domain":"boottime","offset_sec":20.0,"slope":1.0,"calibrated":true},
    {"artifact_path":"dup-domain.perftrace","perf_time_domain":"perf_event_time","trace_time_domain":"trace_seconds","offset_sec":20.0,"slope":1.0,"calibrated":true}
  ]
}`)

	idx, err := BuildIndex(context.Background(), bundle)
	if err != nil {
		t.Fatal(err)
	}
	if len(idx.TraceArtifacts) != 2 || len(idx.Events) != 1 {
		t.Fatalf("conflicting alignment source must stay isolated: events=%+v artifacts=%+v", idx.Events, idx.TraceArtifacts)
	}
	traceSource, perfSource := idx.TraceArtifacts[0], idx.TraceArtifacts[1]
	if traceSource.TimeDomain != "trace_seconds" || traceSource.CanonicalTimeDomain != "trace_seconds" || !traceSource.CausalCompatible {
		t.Fatalf("rejected perf alignment relabeled primary trace: %+v", traceSource)
	}
	if perfSource.CausalCompatible || perfSource.ClockOffsetSec != nil || perfSource.ClockSlope != nil || !strings.Contains(perfSource.IsolationReason, "conflicting duplicate") {
		t.Fatalf("conflicting perf coefficients leaked as provenance: %+v", perfSource)
	}
}

func TestConflictingCanonicalDomainsIsolateAllMappedArtifacts(t *testing.T) {
	dir := t.TempDir()
	systrace := filepath.Join(dir, "domain-conflict.systrace")
	perfA := filepath.Join(dir, "domain-a.perftrace")
	perfB := filepath.Join(dir, "domain-b.perftrace")
	bundle := filepath.Join(dir, "domain-conflict.tracebundle.json")
	writeBundleProvenanceFixture(t, systrace, `app-20 (20) [001] .... 30.000000: sched_wakeup: comm=app pid=20 prio=20 target_cpu=001`)
	writeBundleProvenanceFixture(t, perfA, `app-20 (20) [001] .... 10.000000: perf_sample: cpu=1 pid=20 tid=20 period=1 event=cpu-cycles symbol=A dso=lib.so source=test`)
	writeBundleProvenanceFixture(t, perfB, `app-20 (20) [001] .... 20.000000: perf_sample: cpu=1 pid=20 tid=20 period=1 event=cpu-cycles symbol=B dso=lib.so source=test`)
	writeBundleProvenanceFixture(t, bundle, `{
  "version":"test",
  "systrace":"domain-conflict.systrace",
  "artifacts":[
    {"type":"systrace","path":"domain-conflict.systrace"},
    {"type":"perftrace","path":"domain-a.perftrace","perf_capability":{"time_domain":"perf_a","trace_query_ready":true}},
    {"type":"perftrace","path":"domain-b.perftrace","perf_capability":{"time_domain":"perf_b","trace_query_ready":true}}
  ],
  "perf_clock_alignments":[
    {"artifact_path":"domain-a.perftrace","perf_time_domain":"perf_a","trace_time_domain":"trace_seconds","offset_sec":20.0,"slope":1.0,"calibrated":true},
    {"artifact_path":"domain-b.perftrace","perf_time_domain":"perf_b","trace_time_domain":"boottime","offset_sec":10.0,"slope":1.0,"calibrated":true}
  ]
}`)
	idx, err := BuildIndex(context.Background(), bundle)
	if err != nil {
		t.Fatal(err)
	}
	if len(idx.Events) != 1 || len(idx.TraceArtifacts) != 3 {
		t.Fatalf("canonical-domain conflict must retain only primary trace: events=%+v artifacts=%+v", idx.Events, idx.TraceArtifacts)
	}
	for _, source := range idx.TraceArtifacts[1:] {
		if source.CausalCompatible || !strings.Contains(source.IsolationReason, "conflicting canonical") {
			t.Fatalf("mapped artifact escaped canonical conflict: %+v", source)
		}
	}
}

func TestForeignClockAlignmentCannotInfluenceDeclaredArtifacts(t *testing.T) {
	dir := t.TempDir()
	systrace := filepath.Join(dir, "foreign.systrace")
	perftrace := filepath.Join(dir, "foreign.perftrace")
	bundle := filepath.Join(dir, "foreign.tracebundle.json")
	writeBundleProvenanceFixture(t, systrace, `app-20 (20) [001] .... 30.000000: sched_wakeup: comm=app pid=20 prio=20 target_cpu=001`)
	writeBundleProvenanceFixture(t, perftrace, `app-20 (20) [001] .... 10.001000: perf_sample: cpu=1 pid=20 tid=20 period=1 event=cpu-cycles symbol=Mapped dso=lib.so source=test`)
	writeBundleProvenanceFixture(t, bundle, `{
  "version":"test",
  "systrace":"foreign.systrace",
  "artifacts":[
    {"type":"systrace","path":"foreign.systrace"},
    {"type":"perftrace","path":"foreign.perftrace","perf_capability":{"time_domain":"perf_event_time","trace_query_ready":true}}
  ],
  "perf_clock_alignments":[
    {"artifact_path":"foreign.perftrace","perf_time_domain":"perf_event_time","trace_time_domain":"trace_seconds","offset_sec":20.0,"slope":1.0,"calibrated":true},
    {"artifact_path":"stale.perftrace","perf_time_domain":"stale_clock","trace_time_domain":"boottime","offset_sec":999.0,"slope":1.0,"calibrated":true}
  ]
}`)

	idx, err := BuildIndex(context.Background(), bundle)
	if err != nil {
		t.Fatal(err)
	}
	if len(idx.Events) != 2 || len(idx.TraceArtifacts) != 2 {
		t.Fatalf("foreign alignment changed the declared source universe: events=%+v artifacts=%+v", idx.Events, idx.TraceArtifacts)
	}
	if idx.TraceArtifacts[0].ClockAlignment != TraceClockAlignmentIdentity || idx.TraceArtifacts[0].CanonicalTimeDomain != "trace_seconds" {
		t.Fatalf("foreign alignment relabeled the primary trace: %+v", idx.TraceArtifacts[0])
	}
	if idx.TraceArtifacts[1].ClockAlignment != TraceClockAlignmentAffine || !idx.TraceArtifacts[1].CausalCompatible || math.Abs(idx.Events[1].Ts-30.001) > 1e-9 {
		t.Fatalf("foreign alignment conflicted with a declared perf mapping: events=%+v artifact=%+v", idx.Events, idx.TraceArtifacts[1])
	}
}

func TestSystraceTargetedClockAlignmentHasNoAuthority(t *testing.T) {
	dir := t.TempDir()
	systrace := filepath.Join(dir, "primary.systrace")
	bundle := filepath.Join(dir, "primary.tracebundle.json")
	writeBundleProvenanceFixture(t, systrace, `app-20 (20) [001] .... 10.000000: sched_wakeup: comm=app pid=20 prio=20 target_cpu=001`)
	writeBundleProvenanceFixture(t, bundle, `{
  "version":"test",
  "systrace":"primary.systrace",
  "artifacts":[{"type":"systrace","path":"primary.systrace"}],
  "perf_clock_alignments":[
    {"artifact_path":"primary.systrace","perf_time_domain":"fake_perf_clock","trace_time_domain":"boottime","offset_sec":20.0,"slope":1.0,"calibrated":true}
  ]
}`)

	idx, err := BuildIndex(context.Background(), bundle)
	if err != nil {
		t.Fatal(err)
	}
	if len(idx.Events) != 1 || len(idx.TraceArtifacts) != 1 {
		t.Fatalf("primary bundle shape: events=%+v artifacts=%+v", idx.Events, idx.TraceArtifacts)
	}
	source := idx.TraceArtifacts[0]
	if source.Kind != "systrace" || source.TimeDomain != "trace_seconds" || source.CanonicalTimeDomain != "trace_seconds" || source.ClockAlignment != TraceClockAlignmentIdentity || !source.CausalCompatible {
		t.Fatalf("systrace-targeted perf alignment changed primary provenance: %+v", source)
	}
	if math.Abs(idx.Events[0].Ts-10.0) > 1e-12 {
		t.Fatalf("systrace-targeted perf alignment changed primary time: %+v", idx.Events[0])
	}
}

func TestAffineMappingOverflowFailsClosed(t *testing.T) {
	dir := t.TempDir()
	systrace := filepath.Join(dir, "overflow.systrace")
	perftrace := filepath.Join(dir, "overflow.perftrace")
	bundle := filepath.Join(dir, "overflow.tracebundle.json")
	writeBundleProvenanceFixture(t, systrace, `app-20 (20) [001] .... 10.000000: sched_wakeup: comm=app pid=20 prio=20 target_cpu=001`)
	writeBundleProvenanceFixture(t, perftrace, `app-20 (20) [001] .... 10.000000: perf_sample: cpu=1 pid=20 tid=20 period=1 event=cpu-cycles symbol=Overflow dso=lib.so source=test`)
	writeBundleProvenanceFixture(t, bundle, `{
  "version":"test",
  "systrace":"overflow.systrace",
  "artifacts":[
    {"type":"systrace","path":"overflow.systrace"},
    {"type":"perftrace","path":"overflow.perftrace","perf_capability":{"time_domain":"perf_event_time","trace_query_ready":true}}
  ],
  "perf_clock_alignments":[
    {"artifact_path":"overflow.perftrace","perf_time_domain":"perf_event_time","trace_time_domain":"trace_seconds","offset_sec":0.0,"slope":1.7976931348623157e308,"calibrated":true}
  ]
}`)
	if _, err := BuildIndex(context.Background(), bundle); err == nil || !strings.Contains(err.Error(), "safely and reversibly represent timestamp") {
		t.Fatalf("overflowing affine transform must fail closed, got %v", err)
	}
}

func TestFiniteAffinePrecisionCollapseFailsClosed(t *testing.T) {
	tests := []struct {
		name      string
		timestamp string
		offset    string
		slope     string
	}{
		{
			name:      "huge finite offset",
			timestamp: "10.000000",
			offset:    "1e308",
			slope:     "1.0",
		},
		{
			name:      "finite slope underflow",
			timestamp: "0.0000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000001",
			offset:    "0.0",
			slope:     "1e-300",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			systrace := filepath.Join(dir, "precision.systrace")
			perftrace := filepath.Join(dir, "precision.perftrace")
			bundle := filepath.Join(dir, "precision.tracebundle.json")
			writeBundleProvenanceFixture(t, systrace, `app-20 (20) [001] .... 10.000000: sched_wakeup: comm=app pid=20 prio=20 target_cpu=001`)
			writeBundleProvenanceFixture(t, perftrace, "app-20 (20) [001] .... "+tt.timestamp+`: perf_sample: cpu=1 pid=20 tid=20 period=1 event=cpu-cycles symbol=Precision dso=lib.so source=test`)
			writeBundleProvenanceFixture(t, bundle, `{
  "version":"test",
  "systrace":"precision.systrace",
  "artifacts":[
    {"type":"systrace","path":"precision.systrace"},
    {"type":"perftrace","path":"precision.perftrace","perf_capability":{"time_domain":"perf_event_time","trace_query_ready":true}}
  ],
  "perf_clock_alignments":[
    {"artifact_path":"precision.perftrace","perf_time_domain":"perf_event_time","trace_time_domain":"trace_seconds","offset_sec":`+tt.offset+`,"slope":`+tt.slope+`,"calibrated":true}
  ]
}`)
			if _, err := BuildIndex(context.Background(), bundle); err == nil {
				t.Fatal("finite but non-reversible affine transform must fail closed")
			}
		})
	}
}

func TestAffineMappingRejectsObservedTimestampCollision(t *testing.T) {
	dir := t.TempDir()
	systrace := filepath.Join(dir, "collision.systrace")
	perftrace := filepath.Join(dir, "collision.perftrace")
	bundle := filepath.Join(dir, "collision.tracebundle.json")
	const offset = 1e9
	sourceA := 1e9
	var sourceB float64
	for i := 0; i < 1024; i++ {
		candidate := math.Nextafter(sourceA, math.Inf(1))
		mappedA, mappedB := sourceA+offset, candidate+offset
		if mappedA == mappedB {
			sourceB = candidate
			break
		}
		sourceA = candidate
	}
	if sourceB == 0 {
		t.Fatal("fixture could not find adjacent source timestamps collapsed by the affine map")
	}
	writeBundleProvenanceFixture(t, systrace, `app-20 (20) [001] .... 2000000000.000000: sched_wakeup: comm=app pid=20 prio=20 target_cpu=001`)
	writeBundleProvenanceFixture(t, perftrace, strings.Join([]string{
		"app-20 (20) [001] .... " + strconv.FormatFloat(sourceA, 'f', -1, 64) + `: perf_sample: cpu=1 pid=20 tid=20 period=1 event=cpu-cycles symbol=A dso=lib.so source=test`,
		"app-20 (20) [001] .... " + strconv.FormatFloat(sourceB, 'f', -1, 64) + `: perf_sample: cpu=1 pid=20 tid=20 period=1 event=cpu-cycles symbol=B dso=lib.so source=test`,
	}, "\n"))
	writeBundleProvenanceFixture(t, bundle, `{
  "version":"test",
  "systrace":"collision.systrace",
  "artifacts":[
    {"type":"systrace","path":"collision.systrace"},
    {"type":"perftrace","path":"collision.perftrace","perf_capability":{"time_domain":"perf_event_time","trace_query_ready":true}}
  ],
  "perf_clock_alignments":[
    {"artifact_path":"collision.perftrace","perf_time_domain":"perf_event_time","trace_time_domain":"trace_seconds","offset_sec":1000000000.0,"slope":1.0,"calibrated":true}
  ]
}`)
	if _, err := BuildIndex(context.Background(), bundle); err == nil || !strings.Contains(err.Error(), "collapses distinct source timestamps") {
		t.Fatalf("observed affine collision must fail closed, got %v", err)
	}
}

func TestRawLineReadRejectsArtifactChangedAfterIndexBuild(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "raw-identity.systrace")
	original := `app-20 (20) [001] .... 10.000000: sched_wakeup: comm=app pid=20 prio=20 target_cpu=001`
	writeBundleProvenanceFixture(t, path, original)
	idx, err := BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	mutated := strings.Replace(original, "comm=app", "comm=bad", 1)
	if len(mutated) != len(original) {
		t.Fatal("fixture mutation must preserve size")
	}
	writeBundleProvenanceFixture(t, path, mutated)
	stamp := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(path, stamp, stamp); err != nil {
		t.Fatal(err)
	}
	res := Run(idx, Query{View: "event_search", Limit: 10})
	if len(res.Events) != 1 || res.Events[0].Raw != "" || res.Events[0].RawUnavailableReason != "artifact_identity_changed" {
		t.Fatalf("old typed event was paired with mutated raw text: %+v", res.Events)
	}
	if !containsSubstring(res.Caveats, "raw_artifact_identity_mismatch") {
		t.Fatalf("raw identity mismatch caveat missing: %v", res.Caveats)
	}
}

func writeBundleProvenanceFixture(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
