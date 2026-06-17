package hitraceconv

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildPerfToolStatusReportsConfiguredToolsAndRawFallback(t *testing.T) {
	dir := t.TempDir()
	hiperf := filepath.Join(dir, "hiperf_host")
	simpleperf := filepath.Join(dir, "report_sample")
	for _, path := range []string{hiperf, simpleperf} {
		if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	status, err := BuildPerfToolStatus(Options{
		HiperfPath:           hiperf,
		SimpleperfReportPath: simpleperf,
		PerfParser:           "auto",
	})
	if err != nil {
		t.Fatalf("build status: %v", err)
	}
	if status.ParserMode != "auto" || status.SelectedParser != "auto" {
		t.Fatalf("unexpected parser status: %+v", status)
	}
	if !status.Hiperf.Available || status.Hiperf.Path != hiperf || !strings.Contains(status.Hiperf.Source, "configured") {
		t.Fatalf("hiperf status not available: %+v", status.Hiperf)
	}
	if !status.Simpleperf.Available || status.Simpleperf.Path != simpleperf || !strings.Contains(status.Simpleperf.Source, "configured") {
		t.Fatalf("simpleperf status not available: %+v", status.Simpleperf)
	}
	if !status.RawFallback.Available || status.RawFallback.Source != "built-in" {
		t.Fatalf("raw fallback should be available in auto mode: %+v", status.RawFallback)
	}
}

func TestBuildPerfToolStatusOfficialModeDisablesRawFallback(t *testing.T) {
	status, err := BuildPerfToolStatus(Options{PerfParser: "official"})
	if err != nil {
		t.Fatalf("build status: %v", err)
	}
	if status.RawFallback.Available {
		t.Fatalf("official mode should disable raw fallback: %+v", status.RawFallback)
	}
	if !strings.Contains(strings.Join(status.RawFallback.Caveats, " "), "--perf-parser=official") {
		t.Fatalf("missing official-mode caveat: %+v", status.RawFallback)
	}
}

func TestBuildPerfToolStatusRejectsUnknownParser(t *testing.T) {
	if _, err := BuildPerfToolStatus(Options{PerfParser: "mystery"}); err == nil {
		t.Fatalf("expected invalid perf parser mode to fail")
	}
}
