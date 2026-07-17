package tracequery

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestTraceInputAdmissionRejectsBeforeEveryParserLane(t *testing.T) {
	path := filepath.Join(t.TempDir(), "binary-disguised.systrace")
	writeTraceInputAdmissionFixture(t, path, append([]byte("PERFILE2"), make([]byte, 128)...))

	assertRejected := func(t *testing.T, err error) {
		t.Helper()
		var admission *TraceInputAdmissionError
		if !errors.As(err, &admission) {
			t.Fatalf("error=%v (%T), want typed TraceInputAdmissionError", err, err)
		}
		if admission.Code != TraceInputAdmissionCodeConversionRequired ||
			(admission.Path != canonicalTraceIndexPath(path) && admission.Path != path) ||
			!strings.Contains(admission.Reason, "linux_perf_data") {
			t.Fatalf("admission verdict=%+v", admission)
		}
		for _, want := range []string{"codrax trace convert --input", "this rejected input was not parsed"} {
			if !strings.Contains(admission.Error(), want) {
				t.Fatalf("admission error missing %q: %s", want, admission.Error())
			}
		}
	}

	if idx, err := BuildIndex(context.Background(), path); err == nil || idx != nil {
		t.Fatalf("BuildIndex admitted binary: idx=%+v err=%v", idx, err)
	} else {
		assertRejected(t, err)
	}
	if result, err := StreamEventSearch(context.Background(), path, Query{View: "event_search", Limit: 10}); err == nil {
		t.Fatalf("StreamEventSearch admitted binary: result=%+v", result)
	} else {
		assertRejected(t, err)
	}
	if result, err := StreamWindowSweep(context.Background(), path, Query{
		View: ViewWindowSweep, TimeStart: 1, TimeEnd: 2, TimeStartSet: true, TimeEndSet: true,
	}); err == nil {
		t.Fatalf("StreamWindowSweep admitted binary: result=%+v", result)
	} else {
		assertRejected(t, err)
	}
	callbacks := 0
	if idx, err := StreamScan(context.Background(), path, TraceFlavorAuto, func(Event) bool {
		callbacks++
		return true
	}); err == nil || idx != nil {
		t.Fatalf("StreamScan admitted binary: idx=%+v err=%v", idx, err)
	} else {
		assertRejected(t, err)
	}
	if result, err := StreamStateCluster(context.Background(), path, Query{View: "window_stats"}, 10); err == nil {
		t.Fatalf("StreamStateCluster admitted binary: result=%+v", result)
	} else {
		assertRejected(t, err)
	}
	for _, strategy := range []WindowDiscoveryStrategy{WindowDiscoveryPairingIntegrity, WindowDiscoveryTraceMarkCarry} {
		_, err := DiscoverWindows(context.Background(), path, TraceFlavorAuto, WindowDiscoveryRequest{
			Strategy: strategy, TimeStart: 1, TimeEnd: 2, TimeStartSet: true, TimeEndSet: true,
		})
		if err == nil {
			t.Fatalf("DiscoverWindows(%s) admitted binary", strategy)
		}
		assertRejected(t, err)
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if idx, err := StreamScanHeldFile(context.Background(), file, path, TraceFlavorAuto, 1<<20, func(Event) bool {
		callbacks++
		return true
	}); err == nil || idx != nil {
		t.Fatalf("held scan admitted binary: idx=%+v err=%v", idx, err)
	} else {
		assertRejected(t, err)
	}
	if callbacks != 0 {
		t.Fatalf("held scan invoked %d callbacks before admission", callbacks)
	}
}

func TestTraceInputAdmissionSeparatesEmptyFromConversion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty.htrace")
	writeTraceInputAdmissionFixture(t, path, nil)
	err := ValidateTraceInputPath(context.Background(), path)
	var admission *TraceInputAdmissionError
	if !errors.As(err, &admission) || admission.Code != TraceInputAdmissionCodeEmpty {
		t.Fatalf("empty verdict=%+v err=%v", admission, err)
	}
	if strings.Contains(admission.Error(), "trace convert") || !strings.Contains(admission.Error(), "collect a non-empty text trace") {
		t.Fatalf("empty capture got misleading recovery: %s", admission.Error())
	}
}

func TestTraceInputAdmissionIsContentBasedAndPreservesTextParse(t *testing.T) {
	path := filepath.Join(t.TempDir(), "text-with-binary-suffix.sys")
	row := "app-20 (20) [001] .... 10.000000: sched_wakeup: comm=app pid=20 prio=20 target_cpu=001\n"
	writeTraceInputAdmissionFixture(t, path, []byte(row))
	if err := ValidateTraceInputPath(context.Background(), path); err != nil {
		t.Fatalf("text .sys was rejected by path suffix: %v", err)
	}
	idx, err := BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if idx.Path != canonicalTraceIndexPath(path) || idx.ParsedKnown != 1 || len(idx.Events) != 1 || idx.Events[0].PID != 20 || idx.Events[0].Line != 1 {
		t.Fatalf("text parse changed after admission: path=%q known=%d events=%+v", idx.Path, idx.ParsedKnown, idx.Events)
	}
}

func TestStreamingLanesPreCanceledColdResolverDoesNoAdmissionOrDigestWork(t *testing.T) {
	resetTraceInputAdmissionsForTest()
	resetTraceBundleDigestAttestationsForTest()

	traceInputAdmissions.mu.Lock()
	originalAdmissionMeasure := traceInputAdmissions.measure
	admissionMeasurements := 0
	traceInputAdmissions.measure = func(ctx context.Context, file *os.File, identity traceFileIdentity, displayPath string) error {
		admissionMeasurements++
		return originalAdmissionMeasure(ctx, file, identity, displayPath)
	}
	traceInputAdmissions.mu.Unlock()

	bundleDigestAttestations.mu.Lock()
	originalDigestMeasure := bundleDigestAttestations.measure
	digestMeasurements := 0
	bundleDigestAttestations.measure = func(ctx context.Context, file *os.File) (int64, string, traceFileIdentity, error) {
		digestMeasurements++
		return originalDigestMeasure(ctx, file)
	}
	bundleDigestAttestations.mu.Unlock()

	t.Cleanup(func() {
		resetTraceInputAdmissionsForTest()
		traceInputAdmissions.mu.Lock()
		traceInputAdmissions.measure = originalAdmissionMeasure
		traceInputAdmissions.mu.Unlock()

		resetTraceBundleDigestAttestationsForTest()
		bundleDigestAttestations.mu.Lock()
		bundleDigestAttestations.measure = originalDigestMeasure
		bundleDigestAttestations.mu.Unlock()
	})

	dir := t.TempDir()
	child := filepath.Join(dir, "capture.systrace")
	bundle := filepath.Join(dir, "capture.tracebundle.json")
	writeTraceInputAdmissionFixture(t, child, []byte("app-20 (20) [001] .... 10.000000: sched_wakeup: comm=app pid=20 prio=20 target_cpu=001\n"))
	writeTraceBundleV2ForTest(t, bundle, []byte(`{
  "version":"test",
  "systrace":"capture.systrace",
  "artifacts":[{"type":"systrace","path":"capture.systrace"}]
}`))

	lanes := []struct {
		name string
		run  func(context.Context) error
	}{
		{name: "stream_scan", run: func(ctx context.Context) error {
			_, err := StreamScan(ctx, child, TraceFlavorAuto, func(Event) bool { return true })
			return err
		}},
		{name: "stream_event_search", run: func(ctx context.Context) error {
			_, err := StreamEventSearch(ctx, child, Query{View: "event_search", Limit: 1})
			return err
		}},
		{name: "stream_state_cluster", run: func(ctx context.Context) error {
			_, err := StreamStateCluster(ctx, child, Query{View: "window_stats"}, 1)
			return err
		}},
		{name: "stream_window_sweep", run: func(ctx context.Context) error {
			_, err := StreamWindowSweep(ctx, child, Query{View: ViewWindowSweep, TimeStart: 1, TimeEnd: 2, TimeStartSet: true, TimeEndSet: true})
			return err
		}},
	}
	for _, lane := range lanes {
		t.Run(lane.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			if err := lane.run(ctx); !errors.Is(err, context.Canceled) {
				t.Fatalf("pre-canceled lane returned %v, want context.Canceled", err)
			}
		})
	}
	if admissionMeasurements != 0 || digestMeasurements != 0 {
		t.Fatalf("pre-canceled cold streaming resolver performed work: text_admission=%d digest=%d", admissionMeasurements, digestMeasurements)
	}
	traceInputAdmissions.mu.Lock()
	admissionEntries, admissionInflight := len(traceInputAdmissions.entries), len(traceInputAdmissions.inflight)
	traceInputAdmissions.mu.Unlock()
	bundleDigestAttestations.mu.Lock()
	digestEntries, digestInflight := len(bundleDigestAttestations.entries), len(bundleDigestAttestations.inflight)
	bundleDigestAttestations.mu.Unlock()
	if admissionEntries != 0 || admissionInflight != 0 || digestEntries != 0 || digestInflight != 0 {
		t.Fatalf("pre-canceled cold streaming resolver populated caches: text=(%d,%d) digest=(%d,%d)",
			admissionEntries, admissionInflight, digestEntries, digestInflight)
	}
}

func TestTraceInputAdmissionExplicitBundleRejectsBinaryChild(t *testing.T) {
	resetTraceBundleDigestAttestationsForTest()
	dir := t.TempDir()
	child := filepath.Join(dir, "capture.systrace")
	bundle := filepath.Join(dir, "capture.tracebundle.json")
	writeTraceInputAdmissionFixture(t, child, []byte{'P', 'K', 0x03, 0x04, 0, 1, 2, 3})
	writeTraceBundleV2ForTest(t, bundle, []byte(`{
  "version":"test",
  "systrace":"capture.systrace",
  "artifacts":[{"type":"systrace","path":"capture.systrace"}]
}`))

	err := ValidateTraceInputPath(context.Background(), bundle)
	var admission *TraceInputAdmissionError
	if !errors.As(err, &admission) || admission.Code != TraceInputAdmissionCodeTextExportRequired || admission.Path != canonicalTraceIndexPath(child) {
		t.Fatalf("explicit bundle did not name/reject binary child: admission=%+v err=%v", admission, err)
	}
}

func TestTraceInputAdmissionInvalidExplicitManifestIsSourceUnavailable(t *testing.T) {
	bundle := filepath.Join(t.TempDir(), "capture.tracebundle.json")
	writeTraceInputAdmissionFixture(t, bundle, []byte(`{"version":"test","artifacts":[`))
	err := ValidateTraceInputPath(context.Background(), bundle)
	var admission *TraceInputAdmissionError
	if !errors.As(err, &admission) || admission.Code != TraceInputAdmissionCodeSourceUnavailable {
		t.Fatalf("invalid explicit manifest verdict=%+v err=%v", admission, err)
	}
	if admission.Path != canonicalTraceIndexPath(bundle) && admission.Path != bundle {
		t.Fatalf("invalid manifest repair lost source path: %+v", admission)
	}
	if !strings.Contains(admission.Error(), "repair the explicit tracebundle manifest") ||
		strings.Contains(admission.Error(), "trace convert") {
		t.Fatalf("invalid manifest got unsafe/misleading recovery: %s", admission.Error())
	}
}

func TestTraceInputAdmissionInvalidOptionalBundleCannotHijackDirectText(t *testing.T) {
	resetTraceBundleDigestAttestationsForTest()
	dir := t.TempDir()
	systrace := filepath.Join(dir, "capture.systrace")
	perftrace := filepath.Join(dir, "capture.perftrace")
	bundle := filepath.Join(dir, "capture.tracebundle.json")
	row := "app-20 (20) [001] .... 10.000000: sched_wakeup: comm=app pid=20 prio=20 target_cpu=001\n"
	writeTraceInputAdmissionFixture(t, systrace, []byte(row))
	writeTraceInputAdmissionFixture(t, perftrace, append([]byte("PERFILE2"), make([]byte, 32)...))
	writeTraceBundleV2ForTest(t, bundle, []byte(`{
  "version":"test",
  "systrace":"capture.systrace",
  "artifacts":[
    {"type":"systrace","path":"capture.systrace"},
    {"type":"perftrace","path":"capture.perftrace","perf_capability":{"trace_query_ready":true}}
  ]
}`))

	if err := ValidateTraceInputPath(context.Background(), systrace); err != nil {
		t.Fatalf("invalid optional sibling made direct text unusable: %v", err)
	}
	idx, err := BuildIndex(context.Background(), systrace)
	if err != nil {
		t.Fatal(err)
	}
	if idx.Path != canonicalTraceIndexPath(systrace) || len(idx.TraceArtifacts) != 1 || len(idx.Events) != 1 {
		t.Fatalf("optional sibling was not atomically ignored: path=%q artifacts=%+v events=%+v", idx.Path, idx.TraceArtifacts, idx.Events)
	}
}

func TestHeldTraceAdmissionDoesNotReopenRenamedPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not provide the same rename-open-file contract")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "held.systrace")
	renamed := filepath.Join(dir, "renamed.systrace")
	row := "app-20 (20) [001] .... 10.000000: sched_wakeup: comm=app pid=20 prio=20 target_cpu=001\n"
	writeTraceInputAdmissionFixture(t, path, []byte(row))
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err := os.Rename(path, renamed); err != nil {
		t.Fatal(err)
	}
	idx, err := StreamScanHeldFile(context.Background(), file, path, TraceFlavorAuto, 1<<20, func(Event) bool { return true })
	if err != nil {
		t.Fatalf("held descriptor admission reopened advisory path: %v", err)
	}
	if idx == nil || idx.ParsedKnown != 1 {
		t.Fatalf("held renamed source parse=%+v", idx)
	}
}

func TestHeldTraceAdmissionRejectsChangedDescriptorGeneration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "changing.systrace")
	writeTraceInputAdmissionFixture(t, path, []byte("sched_switch: prev_pid=1 next_pid=2\n"))
	file, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	identity, err := traceFileIdentityFromFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteAt([]byte("changed-generation"), 0); err != nil {
		t.Fatal(err)
	}
	if err := validateHeldTraceInput(context.Background(), file, identity, path, true); err == nil || !strings.Contains(err.Error(), "identity changed") {
		t.Fatalf("changed held generation was admitted: %v", err)
	}
}

func writeTraceInputAdmissionFixture(t *testing.T, path string, body []byte) {
	t.Helper()
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatal(err)
	}
}
