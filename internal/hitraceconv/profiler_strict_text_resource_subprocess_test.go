package hitraceconv

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"testing"
	"time"
	"unsafe"
)

const (
	profilerStrictTextResourceTargetBytes = (12 << 20) - (1 << 10)
	profilerStrictTextResourceHeapSlack   = uint64(12 << 20)
	profilerStrictTextResourceSampleEvery = uint64(1 << 15)
	profilerStrictTextResourceSinkRows    = 4_096
)

func writeProfilerStrictTextHostileFixture(t *testing.T, path string) profilerHostileResourceFixture {
	t.Helper()
	line := []byte("worker-7  ( 7) [001] ....  5.000000: tracing_mark_write: B|7|valid\n")
	occurrences := profilerStrictTextResourceTargetBytes / len(line)
	if occurrences <= profilerStrictTextResourceSinkRows {
		t.Fatalf("strict text hostile fixture is too small: line=%d occurrences=%d", len(line), occurrences)
	}
	dataBytes := uint64(occurrences * len(line))
	return writeProfilerHostileFrame(t, path, "strict-text", uint64(occurrences), dataBytes,
		func(writer io.Writer) error {
			return writeProfilerHostileRepeated(writer, line, occurrences)
		})
}

func profilerStrictTextResourcePeakLimit(frameBytes uint64) uint64 {
	const mib = uint64(1 << 20)
	return ((frameBytes+mib-1)/mib)*mib + profilerStrictTextResourceHeapSlack
}

func runProfilerStrictTextHostileResourceCase(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	input := filepath.Join(dir, "hostile-strict-text.htrace")
	fixture := writeProfilerStrictTextHostileFixture(t, input)
	if fixture.DataBytes < 8<<20 || fixture.DataBytes > 16<<20 {
		t.Fatalf("strict text hostile data bytes=%d, want 8..16 MiB", fixture.DataBytes)
	}
	info, err := os.Stat(input)
	if err != nil {
		t.Fatal(err)
	}
	header, ok, err := readProfilerTraceHeaderAtPath(input, 0, info.Size())
	if err != nil || !ok {
		t.Fatalf("read strict text hostile profiler header: ok=%t err=%v", ok, err)
	}
	sinkDir := filepath.Join(dir, "sink")
	if err := os.Mkdir(sinkDir, 0o700); err != nil {
		t.Fatal(err)
	}
	sink, err := newTraceDBRowSink(sinkDir, profilerStrictTextResourceSinkRows)
	if err != nil {
		t.Fatal(err)
	}
	cleaned := false
	defer func() {
		if !cleaned {
			_ = sink.cleanup()
		}
	}()

	runtime.GC()
	debug.FreeOSMemory()
	var baseline runtime.MemStats
	runtime.ReadMemStats(&baseline)
	resourceCtx, cancelResource := context.WithCancel(context.Background())
	defer cancelResource()
	limit := profilerStrictTextResourcePeakLimit(fixture.FrameBytes)
	heapCtx := &profilerHostileHeapContext{
		Context: resourceCtx, Cancel: cancelResource,
		sampleEvery: profilerStrictTextResourceSampleEvery,
		Baseline:    baseline.HeapAlloc, Peak: baseline.HeapAlloc, Limit: limit,
	}
	extracted, err := extractProfilerTraceFileWithFrameLimit(
		heapCtx, input, info.Size(), header, sink, maxProfilerPluginFrameBytes)
	heapCtx.sample()
	if err != nil {
		if heapCtx.Exceeded {
			t.Fatalf("strict text hostile live heap crossed fixed resource gate: baseline=%d peak=%d delta=%d limit=%d",
				heapCtx.Baseline, heapCtx.Peak, heapCtx.peakDelta(), heapCtx.Limit)
		}
		t.Fatal(err)
	}

	legacyLowerBound := fixture.FrameBytes + fixture.Occurrences*uint64(unsafe.Sizeof(renderedRow{})) +
		fixture.Occurrences*uint64(len("worker-7  ( 7) [001] ....  5.000000: tracing_mark_write: B|7|valid"))
	if legacyLowerBound <= limit+(8<<20) {
		t.Fatalf("strict text hostile fixture lost deterministic legacy discrimination: lower=%d limit=%d",
			legacyLowerBound, limit)
	}
	if !extracted.Detected || extracted.Kind != "openharmony_profiler_trace_file" ||
		extracted.Messages != 1 || len(extracted.PluginMessages) != 1 ||
		extracted.PluginMessages["ftrace-plugin"] != 1 || extracted.StructuredFtrace != 0 ||
		extracted.MalformedFtrace != 0 || extracted.UnsupportedFtrace != 0 ||
		extracted.RejectedMessages != 0 || extracted.TextPluginMessages != 1 ||
		extracted.TextRows != int(fixture.Occurrences) || extracted.SourceFailClosed ||
		extracted.StandaloneDetected || len(extracted.textMessages) != 0 ||
		len(extracted.pairPublishers) != 0 {
		t.Fatalf("strict text hostile return shape drifted: fixture=%+v extracted=%+v",
			fixture, extracted)
	}
	if sink.stats.RowsAccepted != int(fixture.Occurrences) ||
		sink.publishableRows() != int(fixture.Occurrences) || sink.stats.SpillChunks == 0 ||
		sink.stats.PeakBufferedRows > profilerStrictTextResourceSinkRows ||
		sink.stats.PeakBufferedBytes > defaultTraceDBRowBufferBytes {
		t.Fatalf("strict text hostile rows escaped bounded sorter: fixture=%+v stats=%+v publishable=%d",
			fixture, sink.stats, sink.publishableRows())
	}
	plugin := profilerHostileCoverageByTable(t, extracted.TraceCoverage, "plugin:ftrace-plugin")
	if plugin.Family != "builtin_modern_profiler" || plugin.Role != "query_ready_export" ||
		!plugin.Found || plugin.RowsRead != 1 || plugin.RowsEmitted != int(fixture.Occurrences) ||
		plugin.FieldSources["outcome_strict_legacy_text_frames"] != "1" {
		t.Fatalf("strict text hostile plugin coverage drifted: %+v", plugin)
	}
	if heapCtx.Polls <= fixture.Occurrences || heapCtx.Samples < 2 || heapCtx.Exceeded ||
		heapCtx.peakDelta() > limit {
		t.Fatalf("strict text hostile resource proof escaped fixed gate: fixture=%+v polls=%d samples=%d baseline=%d peak=%d delta=%d limit=%d",
			fixture, heapCtx.Polls, heapCtx.Samples, heapCtx.Baseline, heapCtx.Peak,
			heapCtx.peakDelta(), limit)
	}
	t.Logf("strict text hostile resource proof: data=%d frame=%d rows=%d polls=%d samples=%d baseline=%d peak=%d delta=%d limit=%d legacy_lower=%d",
		fixture.DataBytes, fixture.FrameBytes, fixture.Occurrences, heapCtx.Polls, heapCtx.Samples,
		heapCtx.Baseline, heapCtx.Peak, heapCtx.peakDelta(), limit, legacyLowerBound)

	if err := sink.cleanup(); err != nil {
		t.Fatal(err)
	}
	cleaned = true
	entries, err := os.ReadDir(sinkDir)
	if err != nil || len(entries) != 0 {
		t.Fatalf("strict text hostile cleanup retained temp artifacts: entries=%v err=%v", entries, err)
	}
}

func TestProfilerStrictTextHostileFrameStaysWithinMemoryBudget(t *testing.T) {
	if os.Getenv(profilerHostileResourceChildEnv) != "1" {
		ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
		defer cancel()
		command := exec.CommandContext(ctx, os.Args[0],
			"-test.run=^TestProfilerStrictTextHostileFrameStaysWithinMemoryBudget$", "-test.count=1", "-test.v")
		command.Env = profilerHostileResourceChildEnvironment()
		output, err := command.CombinedOutput()
		if ctx.Err() != nil {
			t.Fatalf("strict text hostile resource subprocess timed out: %v\n%s", ctx.Err(), output)
		}
		if err != nil {
			t.Fatalf("strict text hostile resource subprocess failed: %v\n%s", err, output)
		}
		t.Logf("strict text hostile resource subprocess:\n%s", output)
		return
	}

	previousLimit := debug.SetMemoryLimit(profilerHostileMemoryLimit)
	defer debug.SetMemoryLimit(previousLimit)
	previousGC := debug.SetGCPercent(20)
	defer debug.SetGCPercent(previousGC)
	runProfilerStrictTextHostileResourceCase(t)
}
