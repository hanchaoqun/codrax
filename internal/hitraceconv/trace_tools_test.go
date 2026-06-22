package hitraceconv

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestBuildTraceToolStatusReportsConfiguredTraceStreamer(t *testing.T) {
	dir := t.TempDir()
	traceStreamer := filepath.Join(dir, traceStreamerBinaryName())
	mode := os.FileMode(0o755)
	if runtime.GOOS == "windows" {
		mode = 0o644
	}
	if err := os.WriteFile(traceStreamer, []byte("#!/bin/sh\nexit 0\n"), mode); err != nil {
		t.Fatal(err)
	}

	status, err := BuildTraceToolStatus(Options{
		TraceEngine:         "auto",
		TraceStreamerPath:   traceStreamer,
		TraceStreamerSoDirs: []string{filepath.Join(dir, "symbols")},
		TraceDBOutputPath:   filepath.Join(dir, "trace.db"),
	})
	if err != nil {
		t.Fatalf("build status: %v", err)
	}
	if status.EngineMode != traceEngineAuto || status.SelectedEngine != traceEngineTraceStreamer {
		t.Fatalf("unexpected engine status: %+v", status)
	}
	if !status.TraceStreamer.Available || status.TraceStreamer.Path != traceStreamer || !strings.Contains(status.TraceStreamer.Source, "configured") {
		t.Fatalf("trace_streamer status not available: %+v", status.TraceStreamer)
	}
	if got := strings.Join(status.TraceStreamer.AuxiliaryChecks, " "); !strings.Contains(got, "so_dir=") || !strings.Contains(got, "db_output=") {
		t.Fatalf("trace_streamer status should expose so_dir/db checks: %+v", status.TraceStreamer)
	}
	if !status.BuiltinModern.Available || status.BuiltinModern.Source != "built-in" {
		t.Fatalf("builtin modern status should be available: %+v", status.BuiltinModern)
	}
	if !strings.Contains(strings.Join(status.Caveats, " "), "tries trace_streamer DB export") {
		t.Fatalf("auto mode should explain discovered trace_streamer readiness: %+v", status.Caveats)
	}
}

func TestBuildTraceToolStatusDiscoversEnvTraceStreamer(t *testing.T) {
	dir := t.TempDir()
	traceStreamer := filepath.Join(dir, traceStreamerBinaryName())
	mode := os.FileMode(0o755)
	if runtime.GOOS == "windows" {
		mode = 0o644
	}
	if err := os.WriteFile(traceStreamer, []byte("#!/bin/sh\nexit 0\n"), mode); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CODRAX_TRACE_STREAMER", traceStreamer)

	status, err := BuildTraceToolStatus(Options{TraceEngine: "trace-streamer"})
	if err != nil {
		t.Fatalf("build status: %v", err)
	}
	if status.EngineMode != traceEngineTraceStreamer || status.SelectedEngine != traceEngineTraceStreamer {
		t.Fatalf("explicit trace_streamer mode not reflected: %+v", status)
	}
	if !status.TraceStreamer.Available || status.TraceStreamer.Source != "CODRAX_TRACE_STREAMER" {
		t.Fatalf("env trace_streamer not discovered: %+v", status.TraceStreamer)
	}
}

func TestBuildTraceToolStatusRejectsUnknownEngine(t *testing.T) {
	if _, err := BuildTraceToolStatus(Options{TraceEngine: "mystery"}); err == nil {
		t.Fatalf("expected invalid trace engine mode to fail")
	}
}
