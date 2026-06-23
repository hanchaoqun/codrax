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
	if status.SysBinaryParity.Name != traceToolGateNameSysBinaryParity ||
		status.SysBinaryParity.State != traceToolGateStatePendingRepresentativeFixture ||
		status.SysBinaryParity.Proven ||
		status.SysBinaryParity.RequiredEvidence == "" {
		t.Fatalf("sys binary parity gate should expose pending representative fixture state: %+v", status.SysBinaryParity)
	}
	if got := strings.Join(status.SysBinaryParity.Caveats, " "); !strings.Contains(got, "built-in sys binary parser remains an explicit guarded lane") ||
		!strings.Contains(got, "trace+perf htrace never falls back") {
		t.Fatalf("sys binary parity gate caveats should explain guarded boundary: %+v", status.SysBinaryParity)
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

func TestBuildTraceToolStatusDiscoversTraceStreamerNextToCodraxBinaryBeforePATH(t *testing.T) {
	dir := t.TempDir()
	binDir := filepath.Join(dir, "bin")
	pathDir := filepath.Join(dir, "path")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(pathDir, 0o755); err != nil {
		t.Fatal(err)
	}
	mode := os.FileMode(0o755)
	if runtime.GOOS == "windows" {
		mode = 0o644
	}
	binTraceStreamer := filepath.Join(binDir, traceStreamerBinaryName())
	pathTraceStreamer := filepath.Join(pathDir, traceStreamerBinaryName())
	for _, candidate := range []string{binTraceStreamer, pathTraceStreamer} {
		if err := os.WriteFile(candidate, []byte("#!/bin/sh\nexit 0\n"), mode); err != nil {
			t.Fatal(err)
		}
	}
	oldExecutable := traceStreamerExecutablePath
	traceStreamerExecutablePath = func() (string, error) {
		return filepath.Join(binDir, "codrax"), nil
	}
	t.Cleanup(func() {
		traceStreamerExecutablePath = oldExecutable
	})
	t.Setenv("CODRAX_TRACE_STREAMER", "")
	t.Setenv("PATH", pathDir)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("OHOS_SDK_HOME", "")
	t.Setenv("HARMONYOS_SDK_HOME", "")
	t.Setenv("DEVECO_SDK_HOME", "")
	t.Setenv("TRACE_STREAMER_HOME", "")

	status, err := BuildTraceToolStatus(Options{TraceEngine: "auto"})
	if err != nil {
		t.Fatalf("build status: %v", err)
	}
	if !status.TraceStreamer.Available || status.TraceStreamer.Path != binTraceStreamer ||
		status.TraceStreamer.Source != "codrax executable directory" {
		t.Fatalf("trace_streamer next to codrax binary should win before PATH: %+v", status.TraceStreamer)
	}
}

func TestBuildTraceToolStatusDiscoversTraceStreamerPlatformSubdirNextToCodraxBinaryBeforePATH(t *testing.T) {
	platformDirs := traceStreamerHostPlatformDirs()
	if len(platformDirs) == 0 {
		t.Fatalf("expected at least one host platform trace_streamer directory")
	}
	dir := t.TempDir()
	binDir := filepath.Join(dir, "bin")
	pathDir := filepath.Join(dir, "path")
	if err := os.MkdirAll(pathDir, 0o755); err != nil {
		t.Fatal(err)
	}
	platformTraceStreamer := filepath.Join(binDir, "trace_streamer", platformDirs[0], traceStreamerBinaryName())
	if err := os.MkdirAll(filepath.Dir(platformTraceStreamer), 0o755); err != nil {
		t.Fatal(err)
	}
	mode := os.FileMode(0o755)
	if runtime.GOOS == "windows" {
		mode = 0o644
	}
	for _, candidate := range []string{
		platformTraceStreamer,
		filepath.Join(pathDir, traceStreamerBinaryName()),
	} {
		if err := os.WriteFile(candidate, []byte("#!/bin/sh\nexit 0\n"), mode); err != nil {
			t.Fatal(err)
		}
	}
	oldExecutable := traceStreamerExecutablePath
	traceStreamerExecutablePath = func() (string, error) {
		return filepath.Join(binDir, "codrax"), nil
	}
	t.Cleanup(func() {
		traceStreamerExecutablePath = oldExecutable
	})
	t.Setenv("CODRAX_TRACE_STREAMER", "")
	t.Setenv("PATH", pathDir)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("OHOS_SDK_HOME", "")
	t.Setenv("HARMONYOS_SDK_HOME", "")
	t.Setenv("DEVECO_SDK_HOME", "")
	t.Setenv("TRACE_STREAMER_HOME", "")

	status, err := BuildTraceToolStatus(Options{TraceEngine: "auto"})
	if err != nil {
		t.Fatalf("build status: %v", err)
	}
	if !status.TraceStreamer.Available || status.TraceStreamer.Path != platformTraceStreamer ||
		status.TraceStreamer.Source != "codrax executable directory" {
		t.Fatalf("platform trace_streamer next to codrax binary should win before PATH: %+v", status.TraceStreamer)
	}
}

func TestTraceStreamerHostPlatformDirsFor(t *testing.T) {
	tests := []struct {
		name   string
		goos   string
		goarch string
		want   []string
	}{
		{
			name:   "darwin arm64",
			goos:   "darwin",
			goarch: "arm64",
			want:   []string{"darwin-aarch64", "darwin-arm64"},
		},
		{
			name:   "linux amd64",
			goos:   "linux",
			goarch: "amd64",
			want:   []string{"linux-x86_64", "linux-amd64"},
		},
		{
			name:   "windows amd64",
			goos:   "windows",
			goarch: "amd64",
			want:   []string{"windows-x86_64", "windows-amd64"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := traceStreamerHostPlatformDirsFor(tt.goos, tt.goarch); strings.Join(got, ",") != strings.Join(tt.want, ",") {
				t.Fatalf("platform dirs mismatch: got %v want %v", got, tt.want)
			}
		})
	}
}

func TestBuildTraceToolStatusRejectsUnknownEngine(t *testing.T) {
	if _, err := BuildTraceToolStatus(Options{TraceEngine: "mystery"}); err == nil {
		t.Fatalf("expected invalid trace engine mode to fail")
	}
}
