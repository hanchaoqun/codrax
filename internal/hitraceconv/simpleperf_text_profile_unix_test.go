//go:build unix

package hitraceconv

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

func TestConvertSimpleperfTraceOffCPUUsesExactProfileAndExecutionGate(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "trace-offcpu.perf.data")
	meta := simpleperfMetaPairs(
		"trace_offcpu", "true",
		"event_type_info", "cpu-clock:u,1,0\nsched:sched_switch,2,91",
	)
	if err := os.WriteFile(input, syntheticRawPerfDataWithSimpleperfMeta(meta), 0o600); err != nil {
		t.Fatal(err)
	}
	report := filepath.Join(dir, "report.txt")
	if err := os.WriteFile(report, []byte(syntheticSimpleperfOnOffReport()), 0o600); err != nil {
		t.Fatal(err)
	}
	argsRecord := filepath.Join(dir, "args.txt")
	tool := writeProfileAwareSimpleperfTool(t, dir)
	t.Setenv("SIMPLEPERF_PROFILE_HELP", "modern")
	t.Setenv("SIMPLEPERF_PROFILE_REPORT", report)
	t.Setenv("SIMPLEPERF_PROFILE_ARGS", argsRecord)

	result, err := ConvertFile(context.Background(), Options{InputPath: input, SimpleperfReportPath: tool})
	if err != nil {
		t.Fatalf("convert trace-offcpu profile: %v", err)
	}
	perftrace := simpleperfTextPerfTraceArtifact(t, result)
	args, err := os.ReadFile(argsRecord)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(args)) != "on-off-cpu" {
		t.Fatalf("adapter trace-offcpu mode=%q want on-off-cpu", strings.TrimSpace(string(args)))
	}
	idx, err := tracequery.BuildIndex(context.Background(), perftrace.Path)
	if err != nil {
		t.Fatal(err)
	}
	if len(idx.Events) != 2 || idx.Events[0].PerfFields.SampleKind != "on_cpu" || idx.Events[1].PerfFields.SampleKind != "off_cpu" {
		t.Fatalf("sample kinds=%+v", idx.Events)
	}
	stats := tracequery.ComputeWindowStats(idx, tracequery.Query{TimeStart: 1, TimeEnd: 2})
	if stats.PerfSamples == nil || len(stats.PerfSamples.Cohorts) != 2 {
		t.Fatalf("perf cohorts=%+v", stats.PerfSamples)
	}
	for _, cohort := range stats.PerfSamples.Cohorts {
		if len(cohort.TopSymbols) != 1 {
			t.Fatalf("cohort hotspots=%+v", cohort)
		}
		switch cohort.Event {
		case "cpu-clock:u":
			if !reflect.DeepEqual(cohort.TopSymbols[0].CPUs, []int{1}) {
				t.Fatalf("on-CPU sample lost execution CPU: %+v", cohort.TopSymbols[0])
			}
		case "sched:sched_switch":
			if len(cohort.TopSymbols[0].CPUs) != 0 {
				t.Fatalf("off-CPU sample entered CPU execution: %+v", cohort.TopSymbols[0])
			}
		default:
			t.Fatalf("unexpected cohort=%+v", cohort)
		}
	}
}

func TestConvertSimpleperfTraceOffCPUUnsupportedHelpStaysUnknown(t *testing.T) {
	for _, tc := range []struct {
		mode   string
		reason string
	}{
		{mode: "deceptive", reason: "trace_offcpu_help_incomplete"},
		{mode: "old", reason: "trace_offcpu_help_incomplete"},
		{mode: "failure", reason: "trace_offcpu_help_unavailable"},
	} {
		t.Run(tc.mode, func(t *testing.T) {
			dir := t.TempDir()
			input := filepath.Join(dir, "trace-offcpu.perf.data")
			meta := simpleperfMetaPairs(
				"trace_offcpu", "true",
				"event_type_info", "cpu-clock:u,1,0\nsched:sched_switch,2,91",
			)
			if err := os.WriteFile(input, syntheticRawPerfDataWithSimpleperfMeta(meta), 0o600); err != nil {
				t.Fatal(err)
			}
			report := filepath.Join(dir, "report.txt")
			if err := os.WriteFile(report, []byte(syntheticSimpleperfOnOffReport()), 0o600); err != nil {
				t.Fatal(err)
			}
			argsRecord := filepath.Join(dir, "args.txt")
			tool := writeProfileAwareSimpleperfTool(t, dir)
			t.Setenv("SIMPLEPERF_PROFILE_HELP", tc.mode)
			t.Setenv("SIMPLEPERF_PROFILE_REPORT", report)
			t.Setenv("SIMPLEPERF_PROFILE_ARGS", argsRecord)

			result, err := ConvertFile(context.Background(), Options{InputPath: input, SimpleperfReportPath: tool})
			if err != nil {
				t.Fatalf("convert unsupported-help profile: %v", err)
			}
			perftrace := simpleperfTextPerfTraceArtifact(t, result)
			idx, err := tracequery.BuildIndex(context.Background(), perftrace.Path)
			if err != nil {
				t.Fatal(err)
			}
			for _, event := range idx.Events {
				if event.PerfFields.SampleKind != "" {
					t.Fatalf("unsupported help minted sample kind: %+v", event)
				}
			}
			args, err := os.ReadFile(argsRecord)
			if err != nil {
				t.Fatal(err)
			}
			if strings.TrimSpace(string(args)) != "none" {
				t.Fatalf("unsupported adapter received trace-offcpu mode: %q", strings.TrimSpace(string(args)))
			}
			if perftrace.Perf == nil || !strings.Contains(strings.Join(perftrace.Caveats, "\n"), tc.reason) {
				t.Fatalf("unknown profile disclosure missing: %+v", perftrace)
			}
		})
	}
}

func TestConvertSimpleperfTraceOffCPUAcceptsProvedEventSubsets(t *testing.T) {
	for _, tc := range []struct {
		name   string
		event  string
		symbol string
		kind   string
	}{
		{name: "on_only", event: "cpu-clock:u", symbol: "OnCPU::work", kind: "on_cpu"},
		{name: "off_only", event: "sched:sched_switch", symbol: "OffCPU::wait", kind: "off_cpu"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			input := filepath.Join(dir, "trace-offcpu.perf.data")
			meta := simpleperfMetaPairs(
				"trace_offcpu", "true",
				"event_type_info", "cpu-clock:u,1,0\nsched:sched_switch,2,91",
			)
			if err := os.WriteFile(input, syntheticRawPerfDataWithSimpleperfMeta(meta), 0o600); err != nil {
				t.Fatal(err)
			}
			report := filepath.Join(dir, "report.txt")
			body := "App\t123/456 [001] 1.100000: 10 " + tc.event + ":\n\t            1000 " + tc.symbol + " (/system/lib.so)\n"
			if err := os.WriteFile(report, []byte(body), 0o600); err != nil {
				t.Fatal(err)
			}
			argsRecord := filepath.Join(dir, "args.txt")
			tool := writeProfileAwareSimpleperfTool(t, dir)
			t.Setenv("SIMPLEPERF_PROFILE_HELP", "modern")
			t.Setenv("SIMPLEPERF_PROFILE_REPORT", report)
			t.Setenv("SIMPLEPERF_PROFILE_ARGS", argsRecord)

			result, err := ConvertFile(context.Background(), Options{InputPath: input, SimpleperfReportPath: tool})
			if err != nil {
				t.Fatal(err)
			}
			perftrace := simpleperfTextPerfTraceArtifact(t, result)
			idx, err := tracequery.BuildIndex(context.Background(), perftrace.Path)
			if err != nil {
				t.Fatal(err)
			}
			if len(idx.Events) != 1 || idx.Events[0].PerfFields.SampleKind != tc.kind {
				t.Fatalf("subset sample=%+v want kind=%s", idx.Events, tc.kind)
			}
		})
	}
}

func TestConvertSimpleperfTraceOffCPURejectsEventOutsideProvedSet(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "trace-offcpu.perf.data")
	meta := simpleperfMetaPairs(
		"trace_offcpu", "true",
		"event_type_info", "cpu-clock:u,1,0\nsched:sched_switch,2,91",
	)
	if err := os.WriteFile(input, syntheticRawPerfDataWithSimpleperfMeta(meta), 0o600); err != nil {
		t.Fatal(err)
	}
	report := filepath.Join(dir, "report.txt")
	if err := os.WriteFile(report, []byte("App\t123/456 [001] 1.100000: 10 instructions:\n\t            1000 Other::work (/system/lib.so)\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	argsRecord := filepath.Join(dir, "args.txt")
	tool := writeProfileAwareSimpleperfTool(t, dir)
	t.Setenv("SIMPLEPERF_PROFILE_HELP", "modern")
	t.Setenv("SIMPLEPERF_PROFILE_REPORT", report)
	t.Setenv("SIMPLEPERF_PROFILE_ARGS", argsRecord)

	result, err := ConvertFile(context.Background(), Options{InputPath: input, SimpleperfReportPath: tool})
	if err != nil {
		t.Fatalf("unknown official event should use raw fallback: %v", err)
	}
	if len(result.ProviderDecisions) != 2 || result.ProviderDecisions[0].Reason != "official_output_unreadable" ||
		result.ProviderDecisions[1].ProviderName != perfProviderNameRawFallback || !result.ProviderDecisions[1].Succeeded {
		t.Fatalf("unexpected fallback decisions: %+v", result.ProviderDecisions)
	}
	perftrace := simpleperfTextPerfTraceArtifact(t, result)
	if perftrace.Perf == nil || perftrace.Perf.ProviderKind != "raw_fallback" {
		t.Fatalf("unproved event escaped official profile: %+v", perftrace)
	}
}

func writeProfileAwareSimpleperfTool(t *testing.T, dir string) string {
	t.Helper()
	tool := filepath.Join(dir, "report_sample_profile")
	script := `#!/bin/sh
if [ "$1" = "--help" ]; then
	if [ "$SIMPLEPERF_PROFILE_HELP" = "modern" ]; then
    printf '%s\n' 'usage: report_sample.py --trace-offcpu {on-cpu,off-cpu,on-off-cpu,mixed-on-off-cpu}'
	  elif [ "$SIMPLEPERF_PROFILE_HELP" = "deceptive" ]; then
	    printf '%s\n' 'usage: report_sample.py --trace-offcpu-extra mixed-on-off-cpu-only'
	  elif [ "$SIMPLEPERF_PROFILE_HELP" = "failure" ]; then
	    exit 7
  else
    printf '%s\n' 'usage: old_report_sample.py'
  fi
  exit 0
fi
in=""
out=""
mode="none"
while [ "$#" -gt 0 ]; do
  case "$1" in
    -i) shift; in="$1" ;;
    -o) shift; out="$1" ;;
    --trace-offcpu) shift; mode="$1" ;;
  esac
  shift
done
test -n "$in" && test -r "$in" && test -n "$out" || exit 91
printf '%s\n' "$mode" > "$SIMPLEPERF_PROFILE_ARGS"
cp "$SIMPLEPERF_PROFILE_REPORT" "$out"
`
	if err := os.WriteFile(tool, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return tool
}

func syntheticSimpleperfOnOffReport() string {
	return strings.Join([]string{
		"App\t123/456 [001] 1.100000: 10 cpu-clock:u:",
		"\t            1000 OnCPU::work (/system/lib.so)",
		"",
		"App\t123/456 [001] 1.200000: 20 sched:sched_switch:",
		"\t            2000 OffCPU::wait (/system/lib.so)",
		"",
	}, "\n")
}

func simpleperfTextPerfTraceArtifact(t *testing.T, result Result) Artifact {
	t.Helper()
	for _, artifact := range result.Artifacts {
		if artifact.Type == ArtifactPerfTrace {
			return artifact
		}
	}
	t.Fatalf("missing perftrace artifact: %+v", result.Artifacts)
	return Artifact{}
}
