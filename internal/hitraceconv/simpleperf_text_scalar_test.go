package hitraceconv

import (
	"bytes"
	"context"
	"errors"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

func TestSimpleperfTextStrictScalarBoundaries(t *testing.T) {
	tests := []struct {
		name       string
		header     string
		wantPID    int64
		wantTID    int64
		wantCPU    int64
		wantTimeNS uint64
		wantPeriod uint64
	}{
		{
			name:    "present zero idle pseudo tuple",
			header:  "idle\t0/0 [000] 0.000000: 0 cpu-cycles:",
			wantPID: 0, wantTID: 0, wantCPU: 0, wantTimeNS: 0, wantPeriod: 0,
		},
		{
			name:    "inclusive scalar maxima",
			header:  "max\t2147483647/2147483647 [4095] 18446744073.709551: 9223372036854775807 cpu-cycles:",
			wantPID: math.MaxInt32, wantTID: math.MaxInt32, wantCPU: 4095,
			wantTimeNS: 18_446_744_073_709_551_000, wantPeriod: math.MaxInt64,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			samples, err := parseSimpleperfReport(context.Background(), strings.NewReader(simpleperfTextSample(tc.header, "leaf", "lib.so")))
			if err != nil {
				t.Fatal(err)
			}
			if len(samples) != 1 {
				t.Fatalf("samples=%d, want 1", len(samples))
			}
			got := samples[0]
			if got.PID != tc.wantPID || got.TID != tc.wantTID || got.CPU != tc.wantCPU || got.TimestampNS != tc.wantTimeNS || got.Period != tc.wantPeriod {
				t.Fatalf("sample=%+v", got)
			}
			var wire bytes.Buffer
			if err := writeSimpleperfPerfTrace(context.Background(), &wire, samples); err != nil {
				t.Fatal(err)
			}
			line := firstSimpleperfPerfLine(t, wire.String())
			if timestampNS, ok := tracequery.ParseLineTimestampNS(line); !ok || timestampNS != tc.wantTimeNS {
				t.Fatalf("wire timestamp=(%d,%t), want %d:\n%s", timestampNS, ok, tc.wantTimeNS, line)
			}
			path := filepath.Join(t.TempDir(), "sample.perftrace")
			if err := os.WriteFile(path, wire.Bytes(), 0o600); err != nil {
				t.Fatal(err)
			}
			idx, err := tracequery.BuildIndex(context.Background(), path)
			if err != nil || len(idx.Events) != 1 || idx.Events[0].PerfFields == nil {
				t.Fatalf("strict wire round-trip err=%v events=%+v", err, idx)
			}
			event := idx.Events[0]
			if int64(event.PerfFields.PID) != tc.wantPID || int64(event.PerfFields.TID) != tc.wantTID ||
				int64(event.CPU) != tc.wantCPU || uint64(event.PerfFields.Period) != simpleperfExpectedWirePeriod(tc.wantPeriod) {
				t.Fatalf("round-trip scalar tuple drifted: %+v", event)
			}
		})
	}
}

func TestSimpleperfTextZeroIdentityOwnedRoundTrip(t *testing.T) {
	dir := t.TempDir()
	inputPath := filepath.Join(dir, "idle-report.txt")
	outputPath := filepath.Join(dir, "idle.perftrace")
	report := simpleperfTextSample("idle\t0/0 [000] 0.000000: 0 cpu-cycles:", "idle", "[kernel.kallsyms]")
	if err := os.WriteFile(inputPath, []byte(report), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := ConvertSimpleperfReportFileToPerfTrace(context.Background(), inputPath, outputPath); err != nil {
		t.Fatal(err)
	}
	idx, err := tracequery.BuildIndex(context.Background(), outputPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(idx.Events) != 1 {
		t.Fatalf("events=%d, want 1", len(idx.Events))
	}
	event := idx.Events[0]
	if event.Type != tracequery.EventPerfSample || event.PID != 0 || event.TGID != 0 || event.CPU != 0 ||
		event.PerfFields == nil || event.PerfFields.PID != 0 || event.PerfFields.TID != 0 || event.PerfFields.Period != 1 || event.PerfFields.PerfTextIntegrity != "" {
		t.Fatalf("zero producer tuple drifted: %+v", event)
	}
}

func TestSimpleperfTextHighTimestampOrderingStaysExact(t *testing.T) {
	report := strings.Join([]string{
		"late\t1/2 [001] 8589934592.000002: 1 cycles:",
		"\t               2 late (lib.so)",
		"",
		"early\t1/2 [001] 8589934592.000001: 1 cycles:",
		"\t               1 early (lib.so)",
		"",
	}, "\n")
	samples, err := parseSimpleperfReport(context.Background(), strings.NewReader(report))
	if err != nil {
		t.Fatal(err)
	}
	if len(samples) != 2 || samples[0].TimestampNS-samples[1].TimestampNS != 1_000 {
		t.Fatalf("source microseconds collapsed: %+v", samples)
	}
	var wire bytes.Buffer
	if err := writeSimpleperfPerfTrace(context.Background(), &wire, samples); err != nil {
		t.Fatal(err)
	}
	lines := simpleperfPerfLines(wire.String())
	if len(lines) != 2 || !strings.Contains(lines[0], `symbol="early"`) || !strings.Contains(lines[1], `symbol="late"`) {
		t.Fatalf("integer stable ordering drifted:\n%s", wire.String())
	}
	want := []uint64{8_589_934_592_000_001_000, 8_589_934_592_000_002_000}
	for index, line := range lines {
		if got, ok := tracequery.ParseLineTimestampNS(line); !ok || got != want[index] {
			t.Fatalf("line %d timestamp=(%d,%t), want %d:\n%s", index, got, ok, want[index], line)
		}
	}
}

func TestSimpleperfTextDuplicateSamplesPreservePhysicalMultiplicity(t *testing.T) {
	sample := simpleperfTextSample("app\t1/2 [003] 1.000001: 7 cycles:", "same", "lib.so")
	samples, err := parseSimpleperfReport(context.Background(), strings.NewReader(sample+sample))
	if err != nil {
		t.Fatal(err)
	}
	if len(samples) != 2 {
		t.Fatalf("duplicate physical samples=%d, want 2", len(samples))
	}
	first, second := samples[0], samples[1]
	if first.Comm != second.Comm || first.PID != second.PID || first.TID != second.TID || first.CPU != second.CPU ||
		first.TimestampNS != second.TimestampNS || first.Period != second.Period || first.Event != second.Event || first.Leaf != second.Leaf ||
		len(first.CallFrames) != len(second.CallFrames) {
		t.Fatalf("duplicate physical samples were collapsed or rewritten: %+v", samples)
	}
	var wire bytes.Buffer
	if err := writeSimpleperfPerfTrace(context.Background(), &wire, samples); err != nil {
		t.Fatal(err)
	}
	if lines := simpleperfPerfLines(wire.String()); len(lines) != 2 || lines[0] != lines[1] {
		t.Fatalf("duplicate wire multiplicity drifted: %q", lines)
	}
}

func TestSimpleperfTextVariableFieldsDoNotDriveParserControlFlow(t *testing.T) {
	report := strings.Join([]string{
		"# worker : renamed\t1/2 [003] 1.000001: 7 event : subtype:",
		"\t               1 symbol : leaf (lib : path.so)",
		"\ttracing data:",
		"\t\tfield : value",
		"\t\t1 frame : shaped value (lib.so)",
		"\t\tkey : fake\t9/10 [007] 9.000000: 99 forged-event:",
		"",
		"tracing data:\t3/4 [005] 1.000002: 8 cycles:",
		"\t               2 second (lib.so)",
		"",
	}, "\n")
	samples, err := parseSimpleperfReport(context.Background(), strings.NewReader(report))
	if err != nil {
		t.Fatal(err)
	}
	if len(samples) != 2 || samples[0].Comm != "# worker : renamed" || samples[0].Event != "event : subtype" ||
		samples[0].Leaf.Symbol != "symbol : leaf" || samples[0].Leaf.DSO != "lib : path.so" || samples[1].Comm != "tracing data:" {
		t.Fatalf("variable text was reinterpreted as control flow: %+v", samples)
	}
}

func TestSimpleperfTextMalformedScalarsFailClosed(t *testing.T) {
	tests := []struct {
		name   string
		header string
		field  string
	}{
		{name: "pid above int31", header: "app\t2147483648/2 [000] 1.000000: 1 cycles:", field: "pid"},
		{name: "tid above int31", header: "app\t1/2147483648 [000] 1.000000: 1 cycles:", field: "tid"},
		{name: "cpu above bound", header: "app\t1/2 [4096] 1.000000: 1 cycles:", field: "cpu"},
		{name: "timestamp multiply add overflow", header: "app\t1/2 [000] 18446744073.709552: 1 cycles:", field: "timestamp"},
		{name: "seconds overflow", header: "app\t1/2 [000] 18446744073709551616.000000: 1 cycles:", field: "timestamp_seconds"},
		{name: "period above int64", header: "app\t1/2 [000] 1.000000: 9223372036854775808 cycles:", field: "period"},
		{name: "negative pid", header: "app\t-1/2 [000] 1.000000: 1 cycles:", field: "header"},
		{name: "negative tid", header: "app\t1/-2 [000] 1.000000: 1 cycles:", field: "header"},
		{name: "negative cpu", header: "app\t1/2 [-1] 1.000000: 1 cycles:", field: "header"},
		{name: "negative period", header: "app\t1/2 [000] 1.000000: -1 cycles:", field: "header"},
		{name: "negative timestamp", header: "app\t1/2 [000] -1.000000: 1 cycles:", field: "header"},
		{name: "nan timestamp", header: "app\t1/2 [000] NaN: 1 cycles:", field: "header"},
		{name: "infinite timestamp", header: "app\t1/2 [000] Inf: 1 cycles:", field: "header"},
		{name: "exponent timestamp", header: "app\t1/2 [000] 1e3: 1 cycles:", field: "header"},
		{name: "five digit fraction", header: "app\t1/2 [000] 1.00000: 1 cycles:", field: "header"},
		{name: "seven digit fraction", header: "app\t1/2 [000] 1.0000000: 1 cycles:", field: "header"},
		{name: "missing pid", header: "app\t/2 [000] 1.000000: 1 cycles:", field: "header"},
		{name: "missing tid", header: "app\t1/ [000] 1.000000: 1 cycles:", field: "header"},
		{name: "missing cpu", header: "app\t1/2 [] 1.000000: 1 cycles:", field: "header"},
		{name: "missing timestamp", header: "app\t1/2 [000] : 1 cycles:", field: "header"},
		{name: "missing period", header: "app\t1/2 [000] 1.000000: cycles:", field: "header"},
		{name: "empty event", header: "app\t1/2 [000] 1.000000: 1  :", field: "event"},
		{name: "comment-shaped malformed header", header: "# app\t1/2 [000] NaN: 1 cycles:", field: "header"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseSimpleperfReport(context.Background(), strings.NewReader(simpleperfTextSample(tc.header, "leaf", "lib.so")))
			var typed *simpleperfReportError
			if !errors.As(err, &typed) || typed.Field != tc.field {
				t.Fatalf("error=%T %v typed=%+v, want field=%s", err, err, typed, tc.field)
			}
		})
	}
}

func TestSimpleperfTextSampleFramingFailsClosed(t *testing.T) {
	good := strings.TrimRight(simpleperfTextSample("good\t1/2 [000] 1.000000: 1 cycles:", "good", "lib.so"), "\n")
	badHeader := "bad\t1/2 [000] NaN: 1 cycles:\n\t               2 bad (lib.so)"
	tests := []struct {
		name   string
		report string
		field  string
		reason string
	}{
		{name: "good then malformed sibling without blank", report: good + "\n" + badHeader + "\n", field: "header", reason: "malformed"},
		{name: "malformed then good", report: badHeader + "\n" + good + "\n", field: "header", reason: "malformed"},
		{name: "header-shaped row cannot become a frame", report: good + "\ndead\t1 [000] 2.000000: 1 cycles (bogus)\n", field: "header", reason: "malformed"},
		{name: "space-prefixed header shape cannot become a frame", report: good + "\n dead\t1 [000] 2.000000: 1 cycles (bogus)\n", field: "row", reason: "unrecognized"},
		{name: "leaf before header", report: "\t               1 leaf (lib.so)\n", field: "frame", reason: "before_sample_header"},
		{name: "missing leaf at blank", report: "app\t1/2 [000] 1.000000: 1 cycles:\n\n", field: "sample", reason: "missing_leaf_frame"},
		{name: "missing leaf at next header", report: "app\t1/2 [000] 1.000000: 1 cycles:\n" + good + "\n", field: "sample", reason: "missing_leaf_frame"},
		{name: "complete sample missing physical boundary", report: good + "\n" + good + "\n", field: "sample", reason: "missing_sample_boundary"},
		{name: "missing leaf at eof", report: "app\t1/2 [000] 1.000000: 1 cycles:", field: "sample", reason: "missing_leaf_frame"},
		{name: "unknown row inside sample", report: "app\t1/2 [000] 1.000000: 1 cycles:\nunknown\n", field: "row", reason: "unrecognized"},
		{name: "tracing data before leaf", report: "app\t1/2 [000] 1.000000: 1 cycles:\n\ttracing data:\n", field: "tracing_data", reason: "invalid_boundary"},
		{name: "unclosed tracing field", report: "app\t1/2 [000] 1.000000: 1 cycles:\n\t               1 leaf (lib.so)\n\ttracing data:\nnot-a-field\n", field: "tracing_data", reason: "unrecognized_field"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseSimpleperfReport(context.Background(), strings.NewReader(tc.report))
			var typed *simpleperfReportError
			if !errors.As(err, &typed) || typed.Field != tc.field || typed.Reason != tc.reason {
				t.Fatalf("error=%T %v typed=%+v, want field=%s reason=%s", err, err, typed, tc.field, tc.reason)
			}
		})
	}
}

func TestSimpleperfTextScalarFailureUsesRawFallback(t *testing.T) {
	dir := t.TempDir()
	inputPath := filepath.Join(dir, "capture.perf.data")
	if err := os.WriteFile(inputPath, syntheticRawPerfData(), 0o600); err != nil {
		t.Fatal(err)
	}
	toolPath := writeFakeSimpleperfReportTool(t, dir)
	badReport := filepath.Join(dir, "bad-report.txt")
	if err := os.WriteFile(badReport, []byte(simpleperfTextSample("app\t2147483648/2 [000] 1.000000: 1 cycles:", "leaf", "lib.so")), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SIMPLEPERF_REPORT_FIXTURE", badReport)

	result, err := ConvertFile(context.Background(), Options{InputPath: inputPath, SimpleperfReportPath: toolPath})
	if err != nil {
		t.Fatalf("strict official output should use raw fallback: %v", err)
	}
	if len(result.ProviderDecisions) != 2 || result.ProviderDecisions[0].Reason != "official_output_unreadable" ||
		result.ProviderDecisions[0].Succeeded || !result.ProviderDecisions[1].Succeeded || result.ProviderDecisions[1].ProviderName != perfProviderNameRawFallback {
		t.Fatalf("provider fallback provenance=%+v", result.ProviderDecisions)
	}
}

func TestSimpleperfTextRejectedReportPublishesNoOutput(t *testing.T) {
	dir := t.TempDir()
	inputPath := filepath.Join(dir, "bad-report.txt")
	outputPath := filepath.Join(dir, "bad.perftrace")
	if err := os.WriteFile(inputPath, []byte(simpleperfTextSample("app\t1/2 [4096] 1.000000: 1 cycles:", "leaf", "lib.so")), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := ConvertSimpleperfReportFileToPerfTrace(context.Background(), inputPath, outputPath); err == nil {
		t.Fatal("malformed report unexpectedly converted")
	}
	if _, err := os.Lstat(outputPath); !os.IsNotExist(err) {
		t.Fatalf("rejected report published output: %v", err)
	}
}

func TestSimpleperfTextParserIsFixedWidthAndIntegerTimestamped(t *testing.T) {
	body, err := os.ReadFile("simpleperf_text.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	for _, forbidden := range []string{"strconv.Atoi(", "Timestamp float64", "%12.6f"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("simpleperf text producer restored host/float authority %q", forbidden)
		}
	}
	for _, required := range []string{"TimestampNS uint64", "parseSimpleperfReportTimestamp", "sort.SliceStable"} {
		if !strings.Contains(text, required) {
			t.Fatalf("simpleperf text producer lost structural pin %q", required)
		}
	}
	capability := perfCapabilityForSimpleperfReportSample(perfInputLinuxPerfData, "fixture")
	caveats := ""
	if capability != nil {
		caveats = strings.Join(capability.Caveats, " ")
	}
	if capability == nil || !strings.Contains(caveats, "microsecond precision") ||
		!strings.Contains(caveats, "zero PID or TID") || !strings.Contains(caveats, "zero period") {
		t.Fatalf("simpleperf text precision disclosure missing: %+v", capability)
	}
}

func simpleperfTextSample(header, symbol, dso string) string {
	return header + "\n\t               1 " + symbol + " (" + dso + ")\n\n"
}

func firstSimpleperfPerfLine(t *testing.T, wire string) string {
	t.Helper()
	lines := simpleperfPerfLines(wire)
	if len(lines) != 1 {
		t.Fatalf("perf lines=%d, want 1:\n%s", len(lines), wire)
	}
	return lines[0]
}

func simpleperfPerfLines(wire string) []string {
	var lines []string
	for _, line := range strings.Split(wire, "\n") {
		if strings.Contains(line, "perf_sample:") {
			lines = append(lines, line)
		}
	}
	return lines
}

func simpleperfExpectedWirePeriod(value uint64) uint64 {
	if value == 0 {
		return 1
	}
	return value
}
