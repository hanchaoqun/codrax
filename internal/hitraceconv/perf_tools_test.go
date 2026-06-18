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
		HiperfPath:             hiperf,
		HiperfSymbolDirs:       []string{filepath.Join(dir, "symbols")},
		SimpleperfReportPath:   simpleperf,
		SimpleperfSymfsDir:     filepath.Join(dir, "symfs"),
		SimpleperfKallsymsPath: filepath.Join(dir, "kallsyms"),
		PerfParser:             "auto",
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
	if status.Hiperf.CheckCommand == "" || status.Hiperf.InstallCommand == "" || !strings.Contains(status.Hiperf.DocsURL, "developtools_hiperf") {
		t.Fatalf("hiperf status should expose check/install/docs guidance: %+v", status.Hiperf)
	}
	if got := strings.Join(status.Hiperf.AuxiliaryChecks, " "); !strings.Contains(got, "symbol_root=") || !strings.Contains(got, "test -d") {
		t.Fatalf("hiperf status should expose symbol-root checks: %+v", status.Hiperf)
	}
	if !status.Simpleperf.Available || status.Simpleperf.Path != simpleperf || !strings.Contains(status.Simpleperf.Source, "configured") {
		t.Fatalf("simpleperf status not available: %+v", status.Simpleperf)
	}
	if status.Simpleperf.CheckCommand == "" || status.Simpleperf.InstallCommand == "" || !strings.Contains(status.Simpleperf.DocsURL, "simpleperf") {
		t.Fatalf("simpleperf status should expose check/install/docs guidance: %+v", status.Simpleperf)
	}
	if got := strings.Join(status.Simpleperf.AuxiliaryChecks, " "); !strings.Contains(got, "symfs=") || !strings.Contains(got, "kallsyms=") {
		t.Fatalf("simpleperf status should expose symfs/kallsyms checks: %+v", status.Simpleperf)
	}
	if !status.RawFallback.Available || status.RawFallback.Source != "built-in" {
		t.Fatalf("raw fallback should be available in auto mode: %+v", status.RawFallback)
	}
	if status.RawFallback.CheckCommand == "" || status.RawFallback.InstallCommand != "built-in" {
		t.Fatalf("raw fallback status should expose built-in check guidance: %+v", status.RawFallback)
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

func TestBuildPerfToolStatusExplainsConfiguredSimpleperfReportLibWithoutWrapper(t *testing.T) {
	dir := t.TempDir()
	libPath := filepath.Join(dir, "simpleperf_report_lib.py")
	if err := os.WriteFile(libPath, []byte("# library only\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	status, err := BuildPerfToolStatus(Options{SimpleperfReportPath: libPath})
	if err != nil {
		t.Fatalf("build status: %v", err)
	}
	if status.Simpleperf.Available {
		t.Fatalf("library without report_sample.py wrapper should not be executable provider: %+v", status.Simpleperf)
	}
	got := strings.Join(status.Simpleperf.Caveats, " ")
	for _, want := range []string{"simpleperf_report_lib.py", "report_sample.py wrapper", "--simpleperf-report-sample"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing caveat %q: %+v", want, status.Simpleperf)
		}
	}
}

func TestBuildPerfToolStatusRejectsUnknownParser(t *testing.T) {
	if _, err := BuildPerfToolStatus(Options{PerfParser: "mystery"}); err == nil {
		t.Fatalf("expected invalid perf parser mode to fail")
	}
}
