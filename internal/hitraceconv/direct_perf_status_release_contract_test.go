package hitraceconv

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A typed direct-perf input has no trace body. Both status and execution must
// select that route before consulting embedded, PATH, or known-location
// trace_streamer providers; raw perf conversion remains fully functional.
func TestReleaseDirectPerfPositiveRouteBypassesTraceStreamerResolution(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "capture.perf.data")
	if err := os.WriteFile(input, syntheticRawPerfData(), 0o644); err != nil {
		t.Fatal(err)
	}
	oldAssets := embeddedTraceStreamerAssetsFS
	oldEnabled := embeddedTraceStreamerTagEnabled
	oldGap := embeddedTraceStreamerPlatformGap
	resolverCalls := 0
	embeddedTraceStreamerAssetsFS = func() fs.FS {
		resolverCalls++
		return nil
	}
	embeddedTraceStreamerTagEnabled = true
	embeddedTraceStreamerPlatformGap = "must not be consulted"
	t.Cleanup(func() {
		embeddedTraceStreamerAssetsFS = oldAssets
		embeddedTraceStreamerTagEnabled = oldEnabled
		embeddedTraceStreamerPlatformGap = oldGap
	})
	opts := Options{
		InputPath:  input,
		OutputPath: filepath.Join(dir, "unused.systrace"),
		PerfParser: "raw",
	}
	status, err := BuildTraceToolStatus(opts)
	if err != nil {
		t.Fatal(err)
	}
	if status.FirstLane != traceEngineDirectPerf || status.PreflightEngine != traceEngineDirectPerf || status.ExecutionBlocker != "" {
		t.Fatalf("positive direct-perf status route malformed: %+v", status)
	}
	result, err := ConvertFile(context.Background(), opts)
	if err != nil {
		t.Fatalf("positive direct-perf conversion: %v", err)
	}
	if resolverCalls != 0 {
		t.Fatalf("direct-perf route consulted trace_streamer resolver %d time(s)", resolverCalls)
	}
	if result.OutputPath != "" || result.BundlePath == "" {
		t.Fatalf("direct-perf result falsely claimed a trace body or omitted bundle: %+v", result)
	}
	artifact, ok := releaseArtifactByType(result.Artifacts, ArtifactPerfTrace)
	if !ok || !strings.Contains(artifact.Converter, "raw-perfdata") {
		t.Fatalf("direct-perf route omitted normalized perftrace: %+v", result.Artifacts)
	}
}

// Once input inspection deterministically selects the direct-perf route,
// status and execution consume the same trace-only-option conflict decision.
// Status remains read-only and returns normally, but must expose an explicit
// blocker instead of advertising a route that execution will immediately
// reject.
func TestReleaseDirectPerfStatusAndExecutionShareTraceOptionBlocker(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "capture.perf.data")
	if err := os.WriteFile(input, syntheticRawPerfData(), 0o644); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(dir, "capture.systrace")
	opts := Options{
		InputPath:           input,
		OutputPath:          output,
		TraceStreamerPath:   filepath.Join(dir, "trace_streamer"),
		TraceStreamerSoDirs: []string{filepath.Join(dir, "symbols")},
		TraceDBOutputPath:   filepath.Join(dir, "capture.trace.db"),
		KeepTraceDB:         true,
	}

	status, err := BuildTraceToolStatus(opts)
	if err != nil {
		t.Fatalf("status-only direct-perf inspection should return a typed blocker, not fail: %v", err)
	}
	if !status.InputInspected || status.InputKind != "direct_perf" ||
		status.FirstLane != traceEngineDirectPerf || status.PreflightEngine != traceEngineDirectPerf {
		t.Fatalf("direct-perf typed status route malformed: %+v", status)
	}
	if status.ExecutionBlocker == "" {
		t.Fatalf("direct-perf status silently omitted execution blocker: %+v", status)
	}
	for _, want := range []string{"direct perf input has no trace body", "--keep-trace-db", "--trace-db-output", "--trace-streamer", "--trace-streamer-so-dir"} {
		if !strings.Contains(status.ExecutionBlocker, want) {
			t.Fatalf("status blocker missing %q: %s", want, status.ExecutionBlocker)
		}
	}
	if !releaseContains(status.Caveats, "execution_blocked: "+status.ExecutionBlocker) {
		t.Fatalf("status caveats do not carry the typed blocker: %+v", status.Caveats)
	}

	_, convertErr := ConvertFile(context.Background(), opts)
	if convertErr == nil {
		t.Fatal("execution accepted direct perf with trace-only options")
	}
	if convertErr.Error() != status.ExecutionBlocker {
		t.Fatalf("status/execution conflict decision drifted:\nstatus=%q\nexecute=%q", status.ExecutionBlocker, convertErr)
	}
	for _, unexpected := range []string{output, opts.TraceDBOutputPath, opts.TraceDBOutputPath + ".ohos.ts"} {
		if _, err := os.Lstat(unexpected); !os.IsNotExist(err) {
			t.Fatalf("blocked direct-perf conversion mutated %s: err=%v", unexpected, err)
		}
	}
}

func TestReleaseDirectPerfTraceEngineAuthorityMatrix(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "capture.perf.data")
	if err := os.WriteFile(input, syntheticRawPerfData(), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, engine := range []string{traceEngineAuto, traceEngineBuiltin} {
		t.Run(engine+"-direct-blocker", func(t *testing.T) {
			opts := Options{
				InputPath:         input,
				OutputPath:        filepath.Join(dir, engine+".systrace"),
				TraceEngine:       engine,
				TraceStreamerPath: filepath.Join(dir, "missing-trace_streamer"),
			}
			status, err := BuildTraceToolStatus(opts)
			if err != nil {
				t.Fatal(err)
			}
			if status.FirstLane != traceEngineDirectPerf || status.ExecutionBlocker == "" {
				t.Fatalf("%s status did not select the blocked direct route: %+v", engine, status)
			}
			_, convertErr := ConvertFile(context.Background(), opts)
			if convertErr == nil || convertErr.Error() != status.ExecutionBlocker {
				t.Fatalf("%s status/execution authority drifted: status=%q execute=%v", engine, status.ExecutionBlocker, convertErr)
			}
		})
	}

	t.Run("explicit-trace-streamer-remains-sql", func(t *testing.T) {
		opts := Options{
			InputPath:         input,
			OutputPath:        filepath.Join(dir, "explicit-sql.systrace"),
			TraceEngine:       traceEngineTraceStreamer,
			TraceStreamerPath: filepath.Join(dir, "missing-trace_streamer"),
		}
		status, err := BuildTraceToolStatus(opts)
		if err != nil {
			t.Fatal(err)
		}
		if status.FirstLane != traceEngineTraceStreamer || status.PreflightEngine != traceEngineTraceStreamer || status.ExecutionBlocker != "" {
			t.Fatalf("explicit trace_streamer was incorrectly reclassified as direct perf: %+v", status)
		}
		_, convertErr := ConvertFile(context.Background(), opts)
		if convertErr == nil || !strings.Contains(convertErr.Error(), "trace_streamer") || strings.Contains(convertErr.Error(), "direct perf input") {
			t.Fatalf("explicit trace_streamer did not preserve SQL-only failure semantics: %v", convertErr)
		}
	})
}
