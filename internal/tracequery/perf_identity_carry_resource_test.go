package tracequery

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracebundle"
)

func TestBuildIndexWarmWindowCarriesPerfGenerationFromPrefix(t *testing.T) {
	path := filepath.Join(t.TempDir(), "perf-generation-prefix.ftrace")
	body := "old-77 (77) [000] .... 1.000000: sched_wakeup: comm=old pid=77 prio=20 target_cpu=000\n" +
		"creator-9 (9) [000] .... 2.000000: sched_wakeup_new: comm=new pid=77 prio=20 target_cpu=000\n" +
		"new-77 (7) [000] .... 3.000000: perf_sample: cpu=0 cpu_known=true pid=7 tid=77 thread_comm=new sample_weight=1 event=cpu-cycles symbol=New source=fixture sample_kind=on_cpu\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := BuildIndex(context.Background(), path); err != nil {
		t.Fatalf("warm full-cache build: %v", err)
	}
	window, err := BuildIndexWithOptions(context.Background(), path, BuildOptions{
		TimeStart: 2.5, TimeEnd: 3.5, TimeStartSet: true, TimeEndSet: true, AllowWindowedParse: true,
	})
	if err != nil {
		t.Fatalf("derive warm window: %v", err)
	}
	if len(window.Events) != 1 || window.Events[0].Type != EventPerfSample {
		t.Fatalf("window fixture drifted: %+v", window.Events)
	}
	_, identity, ok := ensurePerfIdentityLedger(window).identityForEventOrdinal(0)
	if !ok || identity.TID != 77 || identity.Generation != 2 {
		t.Fatalf("prefix lifecycle reset was lost at warm window head: ok=%t identity=%+v caveats=%v", ok, identity, ensurePerfIdentityLedger(window).caveats())
	}
}

func TestBuildIndexColdWindowCarriesPerfGenerationFromPrefix(t *testing.T) {
	path := filepath.Join(t.TempDir(), "perf-generation-cold.ftrace")
	body := "old-77 (7) [000] .... 1.000000: perf_sample: cpu=0 cpu_known=true pid=7 tid=77 thread_comm=old sample_weight=1 event=cpu-cycles symbol=Old source=fixture sample_kind=on_cpu\n" +
		"creator-9 (9) [000] .... 2.000000: sched_wakeup_new: comm=new pid=77 prio=20 target_cpu=000\n" +
		"new-77 (8) [000] .... 3.000000: perf_sample: cpu=0 cpu_known=true pid=8 tid=77 thread_comm=new sample_weight=1 event=cpu-cycles symbol=New source=fixture sample_kind=on_cpu\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	resetAnchorCaches()
	window, err := BuildIndexWithOptions(context.Background(), path, BuildOptions{
		TimeStart: 1.5, TimeEnd: 3.5, TimeStartSet: true, TimeEndSet: true, AllowWindowedParse: true,
	})
	if err != nil {
		t.Fatalf("build cold window: %v", err)
	}
	if len(window.Events) != 2 {
		t.Fatalf("cold window fixture drifted: %+v", window.Events)
	}
	_, identity, ok := ensurePerfIdentityLedger(window).identityForEventOrdinal(1)
	if !ok || identity.TID != 77 || identity.Generation != 2 {
		t.Fatalf("arbitrary prefix header did not seed cold generation: ok=%t identity=%+v caveats=%v heads=%v candidates=%v artifacts=%+v", ok, identity, ensurePerfIdentityLedger(window).caveats(), window.perfGenerationHeads, perfGenerationCandidateTIDsByScope(window), window.TraceArtifacts)
	}
}

func TestBuildIndexColdLineWindowCarriesPerfGenerationFromPrefix(t *testing.T) {
	path := filepath.Join(t.TempDir(), "perf-generation-line.ftrace")
	body := "old-77 (77) [000] .... 1.000000: sched_wakeup: comm=old pid=77 prio=20 target_cpu=000\n" +
		"creator-9 (9) [000] .... 2.000000: sched_wakeup_new: comm=new pid=77 prio=20 target_cpu=000\n" +
		"new-77 (8) [000] .... 3.000000: perf_sample: cpu=0 cpu_known=true pid=8 tid=77 thread_comm=new sample_weight=1 event=cpu-cycles symbol=New source=fixture sample_kind=on_cpu\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	resetAnchorCaches()
	window, err := BuildIndexWithOptions(context.Background(), path, BuildOptions{
		LineStart: 3, LineEnd: 3, AllowWindowedParse: true,
	})
	if err != nil {
		t.Fatalf("build cold line window: %v", err)
	}
	if len(window.Events) != 1 || window.Events[0].Type != EventPerfSample {
		t.Fatalf("line window fixture drifted: %+v", window.Events)
	}
	_, identity, ok := ensurePerfIdentityLedger(window).identityForEventOrdinal(0)
	if !ok || identity.Generation != 2 {
		t.Fatalf("line-window prefix generation was lost: ok=%t identity=%+v caveats=%v", ok, identity, ensurePerfIdentityLedger(window).caveats())
	}
}

func TestBuildIndexTimeWindowInclusiveBoundaryUsesPhysicalLineOrder(t *testing.T) {
	path := filepath.Join(t.TempDir(), "perf-generation-inclusive.ftrace")
	body := "old-77 (77) [000] .... 1.000000: sched_wakeup: comm=old pid=77 prio=20 target_cpu=000\n" +
		"creator-9 (9) [000] .... 2.000000: sched_wakeup_new: comm=new pid=77 prio=20 target_cpu=000\n" +
		"new-77 (8) [000] .... 2.000000: perf_sample: cpu=0 cpu_known=true pid=8 tid=77 thread_comm=new sample_weight=1 event=cpu-cycles symbol=New source=fixture sample_kind=on_cpu\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	resetAnchorCaches()
	window, err := BuildIndexWithOptions(context.Background(), path, BuildOptions{
		TimeStart: 2, TimeEnd: 2, TimeStartSet: true, TimeEndSet: true, AllowWindowedParse: true,
	})
	if err != nil {
		t.Fatalf("build inclusive time window: %v", err)
	}
	if len(window.Events) != 2 {
		t.Fatalf("inclusive boundary dropped same-ts row: %+v", window.Events)
	}
	_, identity, ok := ensurePerfIdentityLedger(window).identityForEventOrdinal(1)
	if !ok || identity.Generation != 2 {
		t.Fatalf("same-ts physical order did not advance generation: ok=%t identity=%+v caveats=%v", ok, identity, ensurePerfIdentityLedger(window).caveats())
	}
}

func TestBuildIndexAnchorSeekCarriesPerfGenerationFromPrefix(t *testing.T) {
	path := filepath.Join(t.TempDir(), "perf-generation-anchor.ftrace")
	var body strings.Builder
	body.WriteString("old-77 (7) [000] .... 100.000001: perf_sample: cpu=0 cpu_known=true pid=7 tid=77 thread_comm=old sample_weight=1 event=cpu-cycles symbol=Old source=fixture sample_kind=on_cpu\n")
	lastTs := 0.0
	for i := 0; i < traceAnchorLineInterval+160; i++ {
		ts := 100.000002 + float64(i)*0.00001
		lastTs = ts
		if i == traceAnchorLineInterval+80 {
			fmt.Fprintf(&body, "creator-9 (9) [000] .... %.6f: sched_wakeup_new: comm=new pid=77 prio=20 target_cpu=000\n", ts)
			continue
		}
		fmt.Fprintf(&body, "noise-9 (9) [001] .... %.6f: sched_wakeup: comm=noise pid=9 prio=20 target_cpu=001\n", ts)
	}
	sampleTs := lastTs + 0.00001
	fmt.Fprintf(&body, "new-77 (8) [000] .... %.6f: perf_sample: cpu=0 cpu_known=true pid=8 tid=77 thread_comm=new sample_weight=1 event=cpu-cycles symbol=New source=fixture sample_kind=on_cpu\n", sampleTs)
	if err := os.WriteFile(path, []byte(body.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	resetAnchorCaches()
	if _, err := BuildIndex(context.Background(), path); err != nil {
		t.Fatalf("seed anchors: %v", err)
	}
	indexCache = newTraceIndexCache(traceIndexCacheBudgetBytes)
	window, err := BuildIndexWithOptions(context.Background(), path, BuildOptions{
		TimeStart: sampleTs - 0.000005, TimeEnd: sampleTs + 0.00002,
		TimeStartSet: true, TimeEndSet: true, AllowWindowedParse: true,
	})
	if err != nil {
		t.Fatalf("build anchor-seek window: %v", err)
	}
	var sampleOrdinal = -1
	for ordinal := range window.Events {
		if window.Events[ordinal].Type == EventPerfSample {
			sampleOrdinal = ordinal
		}
	}
	if sampleOrdinal < 0 {
		t.Fatalf("anchor fixture lost perf sample: %+v", window.Events)
	}
	_, identity, ok := ensurePerfIdentityLedger(window).identityForEventOrdinal(sampleOrdinal)
	if !ok || identity.TID != 77 || identity.Generation != 2 {
		t.Fatalf("anchor-seek prefix generation was lost: ok=%t identity=%+v caveats=%v heads=%v candidates=%v order=%q regressions=%d artifacts=%+v", ok, identity, ensurePerfIdentityLedger(window).caveats(), window.perfGenerationHeads, perfGenerationCandidateTIDsByScope(window), window.TimestampOrder, window.ClockRegressions, window.TraceArtifacts)
	}
}

func TestBuildIndexV2PerfChildSharesSystracePrefixGeneration(t *testing.T) {
	dir := t.TempDir()
	systrace := filepath.Join(dir, "capture.systrace")
	perftrace := filepath.Join(dir, "capture.perftrace")
	bundle := filepath.Join(dir, "capture.tracebundle.json")
	if err := os.WriteFile(systrace, []byte(
		"old-77 (77) [000] .... 1.000000: sched_wakeup: comm=old pid=77 prio=20 target_cpu=000\n"+
			"creator-9 (9) [000] .... 2.000000: sched_wakeup_new: comm=new pid=77 prio=20 target_cpu=000\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(perftrace, []byte(
		"new-77 (8) [000] .... 3.000000: perf_sample: cpu=0 cpu_known=true pid=8 tid=77 thread_comm=new sample_weight=1 event=cpu-cycles symbol=New source=fixture sample_kind=on_cpu\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	manifest := `{"version":"test","systrace":"capture.systrace","artifacts":[` +
		`{"type":"systrace","path":"capture.systrace"},` +
		`{"type":"perftrace","path":"capture.perftrace","perf_capability":{"time_domain":"trace_seconds","thread_identity":"sample_pid_tid_thread_comm","cpu_identity":"sample_cpu","trace_query_ready":true}}` +
		`],"perf_clock_alignments":[{"artifact_path":"capture.perftrace","perf_time_domain":"trace_seconds","trace_time_domain":"trace_seconds","calibrated":false}]}`
	writeTraceBundleV2ForTest(t, bundle, []byte(manifest))
	resetAnchorCaches()
	window, err := BuildIndexWithOptions(context.Background(), bundle, BuildOptions{
		TimeStart: 2.5, TimeEnd: 3.5, TimeStartSet: true, TimeEndSet: true, AllowWindowedParse: true,
	})
	if err != nil {
		t.Fatalf("build V2 child window: %v", err)
	}
	if len(window.Events) != 1 || window.Events[0].Type != EventPerfSample {
		t.Fatalf("V2 window fixture drifted: %+v", window.Events)
	}
	_, identity, ok := ensurePerfIdentityLedger(window).identityForEventOrdinal(0)
	if !ok || identity.TID != 77 || identity.Generation != 2 {
		t.Fatalf("V2 perf child did not share systrace prefix generation: ok=%t identity=%+v caveats=%v artifacts=%+v", ok, identity, ensurePerfIdentityLedger(window).caveats(), window.TraceArtifacts)
	}
}

func TestBuildIndexV2CanonicalPrefixReplaysPerfChildBeforeSystraceReset(t *testing.T) {
	dir := t.TempDir()
	systrace := filepath.Join(dir, "capture.systrace")
	perftrace := filepath.Join(dir, "capture.perftrace")
	bundle := filepath.Join(dir, "capture.tracebundle.json")
	if err := os.WriteFile(systrace, []byte(
		"creator-9 (9) [000] .... 2.000000: sched_wakeup_new: comm=new pid=77 prio=20 target_cpu=000\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	perfBody := "old-77 (7) [000] .... 1.000000: perf_sample: cpu=0 cpu_known=true pid=7 tid=77 thread_comm=old sample_weight=1 event=cpu-cycles symbol=Old source=fixture sample_kind=on_cpu\n" +
		"new-77 (8) [000] .... 3.000000: perf_sample: cpu=0 cpu_known=true pid=8 tid=77 thread_comm=new sample_weight=1 event=cpu-cycles symbol=New source=fixture sample_kind=on_cpu\n"
	if err := os.WriteFile(perftrace, []byte(perfBody), 0o644); err != nil {
		t.Fatal(err)
	}
	manifest := `{"version":"test","systrace":"capture.systrace","artifacts":[` +
		`{"type":"systrace","path":"capture.systrace"},` +
		`{"type":"perftrace","path":"capture.perftrace","perf_capability":{"time_domain":"trace_seconds","thread_identity":"sample_pid_tid_thread_comm","cpu_identity":"sample_cpu","trace_query_ready":true}}` +
		`],"perf_clock_alignments":[{"artifact_path":"capture.perftrace","perf_time_domain":"trace_seconds","trace_time_domain":"trace_seconds","calibrated":false}]}`
	writeTraceBundleV2ForTest(t, bundle, []byte(manifest))
	resetAnchorCaches()
	window, err := BuildIndexWithOptions(context.Background(), bundle, BuildOptions{
		TimeStart: 2.5, TimeEnd: 3.5, TimeStartSet: true, TimeEndSet: true, AllowWindowedParse: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(window.Events) != 1 || window.Events[0].Type != EventPerfSample {
		t.Fatalf("V2 cross-child prefix fixture drifted: %+v", window.Events)
	}
	_, identity, ok := ensurePerfIdentityLedger(window).identityForEventOrdinal(0)
	if !ok || identity.Generation != 2 {
		t.Fatalf("V2 canonical prefix did not replay perf-child existence before systrace reset: ok=%t identity=%+v caveats=%v", ok, identity, ensurePerfIdentityLedger(window).caveats())
	}
}

func TestPerfGenerationDeduplicatesPreservedAndMergedBoundaryProof(t *testing.T) {
	sources := []TraceArtifactSource{
		{SourcePath: "/capture/sched.systrace", BundleSchema: tracebundle.SchemaV2, CaptureID: "cap", CausalCompatible: true, VirtualLineBase: 0, LocalLineCount: 10},
		{SourcePath: "/capture/samples.perftrace", BundleSchema: tracebundle.SchemaV2, CaptureID: "cap", CausalCompatible: true, VirtualLineBase: 100, LocalLineCount: 10},
	}
	idx := &Index{TraceArtifacts: sources, Events: []Event{
		{Line: 1, Ts: 1, Type: EventSchedWakeup, Name: "sched_wakeup", PID: 1, WakeePID: 77},
		{Line: 2, Ts: 2, Type: EventSchedWakeup, Name: "sched_wakeup_new", PID: 1, WakeePID: 77},
		perfIdentityTestSample(101, 2.5, 77, 8, "new"),
	}, threadIncarnationFailures: []threadIncarnationConflict{
		{PID: 77, PreviousLine: 1, BoundaryLine: 2, PreviousTs: 1, BoundaryTs: 2, Signal: "sched_wakeup_new", SourcePath: "/capture/sched.systrace", LocalPreviousLine: 1, LocalBoundaryLine: 2},
		{PID: 77, PreviousLine: 101, BoundaryLine: 2, PreviousTs: 1, BoundaryTs: 2, Signal: "sched_wakeup_new"},
	}}
	_, identity, ok := ensurePerfIdentityLedger(idx).identityForEventOrdinal(2)
	if !ok || identity.Generation != 2 {
		t.Fatalf("one physical boundary was counted twice or withdrawn: ok=%t identity=%+v caveats=%v", ok, identity, ensurePerfIdentityLedger(idx).caveats())
	}
}

func TestPerfGenerationUnboundSchedulerSourceCannotAdvancePerfOnlySibling(t *testing.T) {
	idx := &Index{TraceArtifacts: []TraceArtifactSource{
		{SourcePath: "/capture/a.systrace", CausalCompatible: true, VirtualLineBase: 0, LocalLineCount: 10},
		{SourcePath: "/capture/b.perftrace", CausalCompatible: true, VirtualLineBase: 100, LocalLineCount: 10},
	}, Events: []Event{
		{Line: 1, Ts: 1, Type: EventSchedWakeup, Name: "sched_wakeup", PID: 1, WakeePID: 77},
		{Line: 2, Ts: 2, Type: EventSchedWakeup, Name: "sched_wakeup_new", PID: 1, WakeePID: 77},
		perfIdentityTestSample(101, 2.5, 77, 8, "source-b"),
	}}
	_, identity, ok := ensurePerfIdentityLedger(idx).identityForEventOrdinal(2)
	if !ok || identity.Generation != 1 {
		t.Fatalf("unbound source A advanced source B generation: ok=%t identity=%+v caveats=%v", ok, identity, ensurePerfIdentityLedger(idx).caveats())
	}
}

func TestPerfGenerationCompositeRegressedChildWithdrawsOnlyTouchedTID(t *testing.T) {
	sources := []TraceArtifactSource{
		{
			SourcePath: "/capture/regressed.systrace", BundleSchema: tracebundle.SchemaV2,
			CaptureID: "capture-v2", CausalCompatible: true, VirtualLineBase: 0, LocalLineCount: 20,
			timestampOrder: TraceTimestampOrderRegressed, clockRegressions: 1,
		},
		{
			SourcePath: "/capture/healthy.perftrace", BundleSchema: tracebundle.SchemaV2,
			CaptureID: "capture-v2", CausalCompatible: true, VirtualLineBase: 100, LocalLineCount: 20,
			timestampOrder: TraceTimestampOrderMonotonic,
		},
	}
	idx := &Index{TraceArtifacts: sources, Events: []Event{
		{Line: 1, Ts: 5, Type: EventSchedWakeup, Name: "sched_wakeup", PID: 9, WakeePID: 77, WakeeComm: "old"},
		{Line: 2, Ts: 4, Type: EventSchedWakeup, Name: "sched_wakeup_new", PID: 9, WakeePID: 77, WakeeComm: "new"},
		perfIdentityTestSample(101, 6, 77, 7, "new"),
		perfIdentityTestSample(102, 6.1, 88, 8, "healthy"),
	}}
	ledger := ensurePerfIdentityLedger(idx)
	if _, _, ok := ledger.identityForEventOrdinal(2); ok {
		t.Fatalf("canonical merge repaired a physically regressed child for touched tid=77: caveats=%v", ledger.caveats())
	}
	if _, identity, ok := ledger.identityForEventOrdinal(3); !ok || identity.TID != 88 || identity.Generation != 1 {
		t.Fatalf("regressed child poisoned untouched healthy sibling: ok=%t identity=%+v caveats=%v", ok, identity, ledger.caveats())
	}
	if got := strings.Join(ledger.caveats(), "\n"); !strings.Contains(got, "reason=source_nonmonotonic") || !strings.Contains(got, "tid=77") {
		t.Fatalf("source-local nonmonotonic withdrawal lacked typed candidate caveat: %q", got)
	}
}

func TestDerivedV2LineWindowCrossChildSameTimestampFailsClosedInEitherArtifactOrder(t *testing.T) {
	for _, sampleFirst := range []bool{true, false} {
		name := "boundary_first"
		if sampleFirst {
			name = "sample_first"
		}
		t.Run(name, func(t *testing.T) {
			firstKind, secondKind := "systrace", "perftrace"
			firstPath, secondPath := "/capture/boundary.systrace", "/capture/sample.perftrace"
			boundaryLine, sampleLine := 1, 101
			if sampleFirst {
				firstKind, secondKind = secondKind, firstKind
				firstPath, secondPath = secondPath, firstPath
				sampleLine, boundaryLine = 1, 101
			}
			sources := []TraceArtifactSource{
				{SourcePath: firstPath, Kind: firstKind, BundleSchema: tracebundle.SchemaV2, CaptureID: "capture-v2", CausalCompatible: true, VirtualLineBase: 0, LocalLineCount: 20, timestampOrder: TraceTimestampOrderMonotonic},
				{SourcePath: secondPath, Kind: secondKind, BundleSchema: tracebundle.SchemaV2, CaptureID: "capture-v2", CausalCompatible: true, VirtualLineBase: 100, LocalLineCount: 20, timestampOrder: TraceTimestampOrderMonotonic},
			}
			boundary := Event{Line: boundaryLine, Ts: 5, Type: EventSchedWakeup, Name: "sched_wakeup_new", PID: 9, WakeePID: 77, WakeeComm: "new"}
			sample := perfIdentityTestSample(sampleLine, 5, 77, 7, "new")
			full := &Index{
				TraceArtifacts: sources, TimestampOrder: TraceTimestampOrderMonotonic,
				Events: []Event{boundary, sample}, FirstTs: 5, LastTs: 5,
			}
			if sampleFirst {
				full.Events[0], full.Events[1] = sample, boundary
			}
			window := deriveWindowedIndex(full, BuildOptions{LineStart: sampleLine, LineEnd: sampleLine})
			if window == nil || len(window.Events) != 1 || window.Events[0].Type != EventPerfSample {
				t.Fatalf("line-window fixture drifted: %+v", window)
			}
			ledger := ensurePerfIdentityLedger(window)
			if _, _, ok := ledger.identityForEventOrdinal(0); ok {
				t.Fatalf("artifact order minted a generation across a simultaneous cross-child boundary: sources=%+v caveats=%v", sources, ledger.caveats())
			}
			if got := strings.Join(ledger.caveats(), "\n"); !strings.Contains(got, "perf_thread_generation_prefix_unproven=true") {
				t.Fatalf("simultaneous head withdrawal lacked typed caveat: %q", got)
			}
		})
	}
}

func TestBuildIndexColdV2LineWindowAuditsUnknownChildPhysicalOrder(t *testing.T) {
	dir := t.TempDir()
	systrace := filepath.Join(dir, "capture.systrace")
	perftrace := filepath.Join(dir, "capture.perftrace")
	bundle := filepath.Join(dir, "capture.tracebundle.json")
	// The line-window parser stops at global line 3, so this child's retained
	// TimestampOrder is Unknown. The dedicated prefix scan must still read the
	// held child to EOF and detect the physical 5s -> 4s rollback before any
	// canonical cross-child replay can reorder it away.
	if err := os.WriteFile(systrace, []byte(
		"old-77 (77) [000] .... 5.000000: sched_wakeup: comm=old pid=77 prio=20 target_cpu=000\n"+
			"creator-9 (9) [000] .... 4.000000: sched_wakeup_new: comm=new pid=77 prio=20 target_cpu=000\n"+
			"new-77 (8) [000] .... 6.000000: perf_sample: cpu=0 cpu_known=true pid=8 tid=77 thread_comm=new sample_weight=1 event=cpu-cycles symbol=New source=fixture sample_kind=on_cpu\n"+
			"noise-9 (9) [000] .... 7.000000: sched_wakeup: comm=noise pid=9 prio=20 target_cpu=000\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(perftrace, []byte(
		"peer-88 (8) [001] .... 8.000000: perf_sample: cpu=1 cpu_known=true pid=8 tid=88 thread_comm=peer sample_weight=1 event=cpu-cycles symbol=Peer source=fixture sample_kind=on_cpu\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	manifest := `{"version":"test","systrace":"capture.systrace","artifacts":[` +
		`{"type":"systrace","path":"capture.systrace"},` +
		`{"type":"perftrace","path":"capture.perftrace","perf_capability":{"time_domain":"trace_seconds","thread_identity":"sample_pid_tid_thread_comm","cpu_identity":"sample_cpu","trace_query_ready":true}}` +
		`],"perf_clock_alignments":[{"artifact_path":"capture.perftrace","perf_time_domain":"trace_seconds","trace_time_domain":"trace_seconds","calibrated":false}]}`
	writeTraceBundleV2ForTest(t, bundle, []byte(manifest))
	resetAnchorCaches()
	idx, err := BuildIndexWithOptions(context.Background(), bundle, BuildOptions{
		LineStart: 3, LineEnd: 3, AllowWindowedParse: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(idx.Events) != 1 || idx.Events[0].Type != EventPerfSample || idx.TraceArtifacts[0].timestampOrder != TraceTimestampOrderUnknown {
		t.Fatalf("cold V2 unknown-order fixture drifted: events=%+v artifacts=%+v", idx.Events, idx.TraceArtifacts)
	}
	ledger := ensurePerfIdentityLedger(idx)
	if _, _, ok := ledger.identityForEventOrdinal(0); ok {
		t.Fatalf("canonical replay repaired an unknown child with physical rollback: caveats=%v artifacts=%+v", ledger.caveats(), idx.TraceArtifacts)
	}
	if got := strings.Join(ledger.caveats(), "\n"); !strings.Contains(got, "reason=nonmonotonic_order") {
		t.Fatalf("cold physical-order withdrawal lacked typed prefix caveat: %q", got)
	}
}

func TestBuildIndexColdV2SkippedChildMalformedLifecycleFailsClosed(t *testing.T) {
	dir := t.TempDir()
	systrace := filepath.Join(dir, "capture.systrace")
	perftrace := filepath.Join(dir, "capture.perftrace")
	bundle := filepath.Join(dir, "capture.tracebundle.json")
	if err := os.WriteFile(systrace, []byte(
		"old-77 (77) [000] .... 1.000000: sched_wakeup: comm=old pid=77 prio=20 target_cpu=000\n"+
			"BROKEN sched_wakeup_new: comm=new pid=77 prio=20 target_cpu=000\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(perftrace, []byte(
		"new-77 (8) [001] .... 3.000000: perf_sample: cpu=1 cpu_known=true pid=8 tid=77 thread_comm=new sample_weight=1 event=cpu-cycles symbol=New source=fixture sample_kind=on_cpu\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	manifest := `{"version":"test","systrace":"capture.systrace","artifacts":[` +
		`{"type":"systrace","path":"capture.systrace"},` +
		`{"type":"perftrace","path":"capture.perftrace","perf_capability":{"time_domain":"trace_seconds","thread_identity":"sample_pid_tid_thread_comm","cpu_identity":"sample_cpu","trace_query_ready":true}}` +
		`],"perf_clock_alignments":[{"artifact_path":"capture.perftrace","perf_time_domain":"trace_seconds","trace_time_domain":"trace_seconds","calibrated":false}]}`
	writeTraceBundleV2ForTest(t, bundle, []byte(manifest))
	systraceInfo, err := os.Stat(systrace)
	if err != nil {
		t.Fatal(err)
	}
	perfVirtualBase, err := traceArtifactVirtualLineReserve(systraceInfo.Size(), 0)
	if err != nil {
		t.Fatal(err)
	}
	perfGlobalLine := perfVirtualBase + 1
	resetAnchorCaches()
	idx, err := BuildIndexWithOptions(context.Background(), bundle, BuildOptions{
		LineStart: perfGlobalLine, LineEnd: perfGlobalLine, AllowWindowedParse: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(idx.Events) != 1 || idx.Events[0].Type != EventPerfSample {
		t.Fatalf("skipped-child malformed fixture drifted: events=%+v artifacts=%+v", idx.Events, idx.TraceArtifacts)
	}
	ledger := ensurePerfIdentityLedger(idx)
	if _, _, ok := ledger.identityForEventOrdinal(0); ok {
		t.Fatalf("malformed lifecycle row outside the strict ftrace envelope disappeared from prefix integrity: caveats=%v", ledger.caveats())
	}
	if got := strings.Join(ledger.caveats(), "\n"); !strings.Contains(got, "reason=malformed_scheduler_identity") {
		t.Fatalf("skipped-child malformed lifecycle withdrawal lacked typed prefix caveat: %q", got)
	}
}
