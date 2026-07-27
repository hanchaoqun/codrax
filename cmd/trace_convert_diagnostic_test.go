package cmd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/hitraceconv"
)

func TestTraceConvertDiagnosticReportHardLimitAndPhysicalLineSafety(t *testing.T) {
	result := hitraceconv.Result{
		InputPath:     "capture.sys",
		EventsWritten: 17,
	}
	for index := 0; index < 1200; index++ {
		result.Caveats = append(result.Caveats, fmt.Sprintf("caveat-%04d\ncontinued", index))
	}
	body := traceConvertDiagnosticReportBody(
		hitraceconv.Options{InputPath: "capture.sys", TraceEngine: "trace_streamer"},
		result,
		traceConvertDiagnosticProgressLog{},
		errors.New("normalize failed\nsecond physical line must not escape"),
	)
	if got := bytes.Count(body, []byte("\n")); got > traceConvertDiagnosticReportMaxLines {
		t.Fatalf("diagnostic report exceeded hard line limit: got=%d limit=%d", got, traceConvertDiagnosticReportMaxLines)
	}
	text := string(body)
	for _, want := range []string{
		traceConvertDiagnosticReportProfile,
		"build_time=",
		"build_revision=",
		`diagnostic_capabilities=["sql_mixed_precision_wire_sort_v1","clock_regression_first_witness_v1","callstack_exact_name_v1","source_cmdline_official_rawtrace_v1","capture_issue_semantics_v1","callstack_official_field_semantics_v1","callstack_time_local_fence_v1"]`,
		`normalize failed\nsecond physical line must not escape`,
		"hard_limit=900",
		"omitted_records=",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("diagnostic report missing %q:\n%s", want, text[:min(len(text), 4096)])
		}
	}
	if strings.Contains(text, "normalize failed\nsecond physical line") {
		t.Fatal("diagnostic value injected an unescaped physical line")
	}
}

func TestTraceConvertDiagnosticProgressKeepsBoundedHeadAndTail(t *testing.T) {
	var progress traceConvertDiagnosticProgressLog
	for index := 0; index < 200; index++ {
		progress.Add(hitraceconv.ProgressEvent{
			Stage:  fmt.Sprintf("stage-%03d", index),
			Status: hitraceconv.ProgressStatusProgress,
		})
	}
	body := string(traceConvertDiagnosticReportBody(
		hitraceconv.Options{InputPath: "capture.sys"},
		hitraceconv.Result{},
		progress,
		nil,
	))
	for _, want := range []string{
		"progress_total=200 progress_emitted=128 progress_omitted=72",
		`"stage":"stage-000"`,
		`"stage":"stage-063"`,
		`"stage":"stage-136"`,
		`"stage":"stage-199"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("bounded progress report missing %q", want)
		}
	}
	if strings.Contains(body, `"stage":"stage-064"`) || strings.Contains(body, `"stage":"stage-135"`) {
		t.Fatal("bounded progress report leaked an event from the deliberately omitted middle")
	}
}

func TestTraceConvertDiagnosticReportFileNeverOverwritesOrAliases(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "capture.sys")
	output := filepath.Join(dir, "capture.systrace")
	reportPath := filepath.Join(dir, "diagnostic.txt")
	if err := os.WriteFile(input, []byte("input"), 0o600); err != nil {
		t.Fatal(err)
	}
	report, err := openTraceConvertDiagnosticReport(reportPath, input, output, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := report.Write([]byte("receipt\n")); err != nil {
		t.Fatal(err)
	}
	if _, err := openTraceConvertDiagnosticReport(reportPath, input, output, ""); err == nil || !errors.Is(err, os.ErrExist) {
		t.Fatalf("existing diagnostic report was not rejected with os.ErrExist: %v", err)
	}
	if _, err := openTraceConvertDiagnosticReport(input, input, output, ""); err == nil || !strings.Contains(err.Error(), "must not alias trace input") {
		t.Fatalf("diagnostic report aliasing input was not rejected: %v", err)
	}
	if _, err := openTraceConvertDiagnosticReport(output, input, output, ""); err == nil || !strings.Contains(err.Error(), "must not alias systrace output") {
		t.Fatalf("diagnostic report aliasing output was not rejected: %v", err)
	}
}

func TestTraceConvertDiagnosticFailureRetainsTypedInputCode(t *testing.T) {
	conversionErr := &hitraceconv.ConversionInputError{
		Code:  hitraceconv.ConversionInputCodeGenerationChanged,
		Stage: "external_tool",
		Path:  "capture.sys",
		Cause: context.Canceled,
	}
	body := string(traceConvertDiagnosticReportBody(
		hitraceconv.Options{InputPath: "capture.sys"},
		hitraceconv.Result{},
		traceConvertDiagnosticProgressLog{},
		conversionErr,
	))
	for _, want := range []string{
		"outcome=failure",
		"typed_error_conversion_input=",
		`"code":"source_generation_changed"`,
		`"stage":"external_tool"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("typed failure report missing %q:\n%s", want, body)
		}
	}
}

func TestTraceConvertDiagnosticRetainsClockRegressionWitness(t *testing.T) {
	witness := &hitraceconv.TraceClockRegressionWitnessError{
		PreviousLine:         101,
		CurrentLine:          102,
		PreviousTimestampSec: 8.0000012,
		CurrentTimestampSec:  8.000001,
		PreviousEventType:    "frame_map",
		CurrentEventType:     "trace_mark",
	}
	conversionErr := &hitraceconv.TraceProviderFailureError{
		Stage: "trace_db_normalize",
		Code:  "trace_db_normalize_failed",
		Cause: witness,
	}
	body := string(traceConvertDiagnosticReportBody(
		hitraceconv.Options{InputPath: "capture.sys"},
		hitraceconv.Result{},
		traceConvertDiagnosticProgressLog{},
		conversionErr,
	))
	for _, want := range []string{
		"typed_error_clock_regression=",
		`"previous_line":101`,
		`"current_line":102`,
		`"previous_timestamp_sec":8.0000012`,
		`"current_timestamp_sec":8.000001`,
		`"previous_event_type":"frame_map"`,
		`"current_event_type":"trace_mark"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("clock regression diagnostic missing %q:\n%s", want, body)
		}
	}
	if got := bytes.Count([]byte(body), []byte("\n")); got > traceConvertDiagnosticReportMaxLines {
		t.Fatalf("clock regression witness exceeded diagnostic budget: got=%d", got)
	}
}
