package hitraceconv

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

func TestConvertSimpleperfReportFileToPerfTraceRoundTripsThroughTraceQuery(t *testing.T) {
	dir := t.TempDir()
	reportPath := filepath.Join(dir, "simpleperf.txt")
	outPath := filepath.Join(dir, "simpleperf.perftrace")
	if err := os.WriteFile(reportPath, []byte(syntheticSimpleperfReport()), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := ConvertSimpleperfReportFileToPerfTrace(context.Background(), reportPath, outPath); err != nil {
		t.Fatalf("convert simpleperf report: %v", err)
	}
	body, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"perf_sample:",
		"cpu=5",
		"pid=1234",
		"tid=5678",
		"period=10000",
		`event="cpu-cycles"`,
		`symbol="Foo::bar"`,
		`dso="/system/lib64/libfoo.so"`,
		`ip="0x1234"`,
		`callchain="main@/system/lib64/libfoo.so;A@/system/lib64/libfoo.so;Foo::bar@/system/lib64/libfoo.so"`,
		"source=simpleperf_report_sample",
		"cpu_known=true",
		"symbolization_status=symbolized",
		"clock_confidence=assumed",
		"callchain_status=symbolized",
	} {
		if !strings.Contains(string(body), want) {
			t.Fatalf("perftrace missing %q:\n%s", want, string(body))
		}
	}

	idx, err := tracequery.BuildIndex(context.Background(), outPath)
	if err != nil {
		t.Fatalf("parse perftrace: %v", err)
	}
	if len(idx.Events) != 1 {
		t.Fatalf("events: got %d want 1", len(idx.Events))
	}
	ev := idx.Events[0]
	if ev.Type != tracequery.EventPerfSample || ev.CPU != 5 || ev.PerfPID != 1234 || ev.PerfTID != 5678 || ev.PerfPeriod != 10000 {
		t.Fatalf("bad perf sample fields: %+v", ev)
	}
	if ev.PerfEvent != "cpu-cycles" || ev.PerfSymbol != "Foo::bar" || ev.PerfDSO != "/system/lib64/libfoo.so" {
		t.Fatalf("bad perf symbol fields: %+v", ev)
	}
	if ev.PerfCPUKnown == nil || !*ev.PerfCPUKnown || ev.PerfSymbolizationStatus != "symbolized" || ev.PerfClockConfidence != "assumed" || ev.PerfCallchainStatus != "symbolized" {
		t.Fatalf("bad perf quality fields: %+v", ev)
	}
}

func TestConvertFileRunsConfiguredSimpleperfAdapterForDirectPerfData(t *testing.T) {
	dir := t.TempDir()
	perfData := filepath.Join(dir, "perf.data")
	if err := os.WriteFile(perfData, []byte("ANDROID-PERF-DATA"), 0o644); err != nil {
		t.Fatal(err)
	}
	reportFixture := filepath.Join(dir, "report.txt")
	if err := os.WriteFile(reportFixture, []byte(syntheticSimpleperfReport()), 0o644); err != nil {
		t.Fatal(err)
	}
	toolPath := filepath.Join(dir, "report_sample")
	script := `#!/bin/sh
out=""
while [ "$#" -gt 0 ]; do
  if [ "$1" = "-o" ]; then
    shift
    out="$1"
  fi
  shift
done
cp "$SIMPLEPERF_REPORT_FIXTURE" "$out"
`
	if err := os.WriteFile(toolPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SIMPLEPERF_REPORT_FIXTURE", reportFixture)

	output := filepath.Join(dir, "ignored.systrace")
	result, err := ConvertFile(context.Background(), Options{InputPath: perfData, OutputPath: output, SimpleperfReportPath: toolPath})
	if err != nil {
		t.Fatalf("convert direct perf data: %v", err)
	}
	if result.OutputPath != "" || result.EventsWritten != 0 {
		t.Fatalf("direct perf.data should be sidecar-only: %+v", result)
	}
	var perfTrace Artifact
	for _, artifact := range result.Artifacts {
		if artifact.Type == ArtifactPerfTrace {
			perfTrace = artifact
			break
		}
	}
	if perfTrace.Path == "" {
		t.Fatalf("missing perftrace artifact: %+v", result.Artifacts)
	}
	idx, err := tracequery.BuildIndex(context.Background(), perfTrace.Path)
	if err != nil {
		t.Fatalf("parse generated perftrace: %v", err)
	}
	if len(idx.Events) != 1 || idx.Events[0].PerfSymbol != "Foo::bar" {
		t.Fatalf("generated perftrace did not round-trip: %+v", idx.Events)
	}
	bundle, err := os.ReadFile(result.BundlePath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(bundle), `"type": "perftrace"`) || !strings.Contains(string(bundle), perfTrace.Path) {
		t.Fatalf("bundle missing perftrace artifact:\n%s", string(bundle))
	}
	if _, err := os.Stat(output); err == nil {
		t.Fatalf("direct perf.data conversion should not create systrace output %s", output)
	} else if !os.IsNotExist(err) {
		t.Fatal(err)
	}
}

func syntheticSimpleperfReport() string {
	return strings.Join([]string{
		"# ========",
		"# cmdline : simpleperf record -g",
		"Render Thread\t1234/5678 [005] 928.081774: 10000 cpu-cycles:",
		"\t            1234 Foo::bar (/system/lib64/libfoo.so)",
		"\t            2222 A (/system/lib64/libfoo.so)",
		"\t            1111 main (/system/lib64/libfoo.so)",
		"",
	}, "\n")
}
