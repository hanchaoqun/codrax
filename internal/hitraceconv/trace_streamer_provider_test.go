package hitraceconv

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

func TestConvertFileTraceStreamerExplicitProducesTraceDBBundle(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake trace_streamer shell fixture uses /bin/sh")
	}
	dir := t.TempDir()
	input := filepath.Join(dir, "capture.htrace")
	if err := os.WriteFile(input, []byte("modern profiler payload"), 0o644); err != nil {
		t.Fatal(err)
	}
	argsLog := filepath.Join(dir, "args.log")
	traceStreamer := writeFakeTraceStreamer(t, dir, 0)
	t.Setenv("TRACE_STREAMER_ARGS_LOG", argsLog)

	output := filepath.Join(dir, "capture.systrace")
	result, err := ConvertFile(context.Background(), Options{
		InputPath:           input,
		OutputPath:          output,
		TraceEngine:         "trace_streamer",
		TraceStreamerPath:   traceStreamer,
		TraceStreamerSoDirs: []string{filepath.Join(dir, "symbols")},
	})
	if err != nil {
		t.Fatalf("convert trace_streamer explicit: %v", err)
	}
	if result.OutputPath != "" || result.EventsWritten != 0 {
		t.Fatalf("trace_streamer DB-only batch should not claim systrace rows: %+v", result)
	}
	var db Artifact
	for _, artifact := range result.Artifacts {
		if artifact.Type == ArtifactTraceDB {
			db = artifact
			break
		}
	}
	if db.Path != strings.TrimSuffix(output, defaultOutputSuffix)+".trace.db" || db.Bytes == 0 {
		t.Fatalf("missing trace DB artifact: %+v artifacts=%+v", db, result.Artifacts)
	}
	args, err := os.ReadFile(argsLog)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{input, "-e", db.Path, "--So_dir", filepath.Join(dir, "symbols")} {
		if !strings.Contains(string(args), want) {
			t.Fatalf("trace_streamer args missing %q:\n%s", want, args)
		}
	}
	bundle, err := os.ReadFile(result.BundlePath)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"type": "trace_db"`, `"trace_provider_decisions"`, `"provider_name": "trace_streamer_db"`} {
		if !strings.Contains(string(bundle), want) {
			t.Fatalf("bundle missing %q:\n%s", want, bundle)
		}
	}
	if strings.Contains(string(bundle), `"systrace"`) {
		t.Fatalf("DB-only bundle must not claim nonexistent systrace output:\n%s", bundle)
	}
}

func TestConvertFileTraceStreamerAutoKeepsDBAndFallsThroughToProfiler(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake trace_streamer shell fixture uses /bin/sh")
	}
	dir := t.TempDir()
	input := filepath.Join(dir, "profiler.htrace")
	textTrace := "worker-1234  ( 1234) [005] ....     1.234567: sched_wakeup: comm=main pid=5678 prio=53 target_cpu=005\n"
	if err := os.WriteFile(input, syntheticProfilerTraceFile(syntheticProfilerPluginData("bytrace_plugin", []byte(textTrace))), 0o644); err != nil {
		t.Fatal(err)
	}
	traceStreamer := writeFakeTraceStreamer(t, dir, 0)

	output := filepath.Join(dir, "out.systrace")
	result, err := ConvertFile(context.Background(), Options{
		InputPath:         input,
		OutputPath:        output,
		TraceStreamerPath: traceStreamer,
		KeepTraceDB:       true,
	})
	if err != nil {
		t.Fatalf("convert auto trace_streamer+profiler: %v", err)
	}
	if result.OutputPath != output || result.EventsWritten != 1 {
		t.Fatalf("auto should keep profiler systrace output: %+v", result)
	}
	if !hasArtifact(result.Artifacts, ArtifactTraceDB) {
		t.Fatalf("keep-trace-db should retain DB artifact: %+v", result.Artifacts)
	}
	if !hasTraceDecision(result.TraceDecisions, traceProviderNameTraceStreamer, true) ||
		!hasTraceDecision(result.TraceDecisions, traceProviderNameBuiltinModern, true) {
		t.Fatalf("expected trace_streamer and builtin decisions: %+v", result.TraceDecisions)
	}
}

func TestConvertFileTraceStreamerAutoFailureFallsBackToProfiler(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake trace_streamer shell fixture uses /bin/sh")
	}
	dir := t.TempDir()
	input := filepath.Join(dir, "profiler.htrace")
	textTrace := "worker-1234  ( 1234) [005] ....     1.234567: sched_wakeup: comm=main pid=5678 prio=53 target_cpu=005\n"
	if err := os.WriteFile(input, syntheticProfilerTraceFile(syntheticProfilerPluginData("bytrace_plugin", []byte(textTrace))), 0o644); err != nil {
		t.Fatal(err)
	}
	traceStreamer := writeFakeTraceStreamer(t, dir, 7)

	output := filepath.Join(dir, "out.systrace")
	result, err := ConvertFile(context.Background(), Options{
		InputPath:         input,
		OutputPath:        output,
		TraceStreamerPath: traceStreamer,
	})
	if err != nil {
		t.Fatalf("auto trace_streamer failure should fall back to profiler: %v", err)
	}
	if result.OutputPath != output || result.EventsWritten != 1 {
		t.Fatalf("auto fallback should keep profiler systrace output: %+v", result)
	}
	if !hasTraceDecision(result.TraceDecisions, traceProviderNameTraceStreamer, false) ||
		!hasTraceDecision(result.TraceDecisions, traceProviderNameBuiltinModern, true) {
		t.Fatalf("expected failed trace_streamer and successful builtin decisions: %+v", result.TraceDecisions)
	}
}

func TestConvertFileTraceStreamerExplicitMissingToolFailsFast(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "capture.htrace")
	if err := os.WriteFile(input, []byte("payload"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := ConvertFile(context.Background(), Options{
		InputPath:         input,
		TraceEngine:       "trace_streamer",
		TraceStreamerPath: filepath.Join(dir, "missing_trace_streamer"),
	})
	if err == nil || !strings.Contains(err.Error(), "trace_streamer is not available") {
		t.Fatalf("expected explicit missing trace_streamer failure, got %v", err)
	}
}

func writeFakeTraceStreamer(t *testing.T, dir string, exitCode int) string {
	t.Helper()
	path := filepath.Join(dir, "trace_streamer")
	script := `#!/bin/sh
if [ -n "$TRACE_STREAMER_ARGS_LOG" ]; then
  printf '%s\n' "$@" > "$TRACE_STREAMER_ARGS_LOG"
fi
out=""
while [ "$#" -gt 0 ]; do
  if [ "$1" = "-e" ]; then
    shift
    out="$1"
  fi
  shift
done
if [ ` + strconv.Itoa(exitCode) + ` -ne 0 ]; then
  echo "fake trace_streamer failed" >&2
  exit ` + strconv.Itoa(exitCode) + `
fi
printf 'SQLite format 3\000fake-db\n' > "$out"
`
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func hasArtifact(artifacts []Artifact, typ string) bool {
	for _, artifact := range artifacts {
		if artifact.Type == typ {
			return true
		}
	}
	return false
}

func hasTraceDecision(decisions []TraceProviderDecision, name string, succeeded bool) bool {
	for _, decision := range decisions {
		if decision.ProviderName == name && decision.Succeeded == succeeded {
			return true
		}
	}
	return false
}
