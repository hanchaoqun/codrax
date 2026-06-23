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
	if !strings.Contains(strings.Join(status.Caveats, " "), "auto trace engine discovered trace_streamer") ||
		!strings.Contains(strings.Join(status.Caveats, " "), "will not fall back") {
		t.Fatalf("auto mode should explain selected trace_streamer readiness: %+v", status.Caveats)
	}
}

func TestBuildTraceToolStatusAutoMissingTraceStreamerSelectsBuiltinForTraceOnly(t *testing.T) {
	dir := t.TempDir()
	status, err := BuildTraceToolStatus(Options{
		TraceEngine:       "auto",
		TraceStreamerPath: filepath.Join(dir, "missing_trace_streamer"),
	})
	if err != nil {
		t.Fatalf("build status: %v", err)
	}
	if status.EngineMode != traceEngineAuto || status.SelectedEngine != traceEngineBuiltin {
		t.Fatalf("auto missing trace_streamer should select built-in for trace-only conversion: %+v", status)
	}
	if !strings.Contains(strings.Join(status.Caveats, " "), "trace-only conversion will use the built-in parser") ||
		!strings.Contains(strings.Join(status.Caveats, " "), "trace+perf htrace still requires trace_streamer") {
		t.Fatalf("auto missing trace_streamer caveat should keep trace+perf SQL-only boundary: %+v", status.Caveats)
	}
}

func TestBuildTraceToolStatusAutoTracePerfInputKeepsSQLOnlyWhenTraceStreamerMissing(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "trace_perf.htrace")
	body := append([]byte("prefix"), syntheticStandaloneProfilerBlock(profilerDataTypeHiperf, "hiperf-plugin", "1.02", []byte("PERF-DATA"))...)
	if err := os.WriteFile(input, body, 0o644); err != nil {
		t.Fatal(err)
	}

	status, err := BuildTraceToolStatus(Options{
		InputPath:         input,
		TraceEngine:       "auto",
		TraceStreamerPath: filepath.Join(dir, "missing_trace_streamer"),
	})
	if err != nil {
		t.Fatalf("build status: %v", err)
	}
	if status.EngineMode != traceEngineAuto || status.SelectedEngine != traceEngineTraceStreamer {
		t.Fatalf("trace+perf input should keep SQL-only selected engine when trace_streamer is missing: %+v", status)
	}
	if !status.InputInspected || status.InputKind != "trace_perf" || !status.InputHasPerfSidecar || status.InputInspectionError != "" {
		t.Fatalf("trace+perf input classification mismatch: %+v", status)
	}
	if !strings.Contains(strings.Join(status.Caveats, " "), "inspected input contains a standalone perf sidecar") ||
		!strings.Contains(strings.Join(status.Caveats, " "), "will not use the built-in parser") {
		t.Fatalf("trace+perf status should explain SQL-only boundary: %+v", status.Caveats)
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
