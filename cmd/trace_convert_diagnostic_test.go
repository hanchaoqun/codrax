package cmd

import (
	"bytes"
	"context"
	"encoding/json"
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
		`build_identity={"revision":`,
		`"executable_hash_status":"available"`,
		`"official_raw_record_decode_budget_v2","official_raw_record_decode_retained_bytes_v3","official_raw_record_family_authority_v1","official_raw_record_decode_elapsed_v1","official_raw_scheduler_lite_wakeup_join_v1"`,
		`normalize failed\nsecond physical line must not escape`,
		"hard_limit=900",
		"omitted_records=",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("diagnostic report missing %q:\n%s", want, text[:min(len(text), 4096)])
		}
	}
	var identityLine string
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, "build_identity=") {
			identityLine = line
			break
		}
	}
	if len(identityLine) == 0 || !strings.Contains(identityLine, `"executable_sha256":"`) {
		t.Fatalf("diagnostic report missing exact executable fingerprint: %s", identityLine)
	}
	if strings.Contains(text, "normalize failed\nsecond physical line") {
		t.Fatal("diagnostic value injected an unescaped physical line")
	}
}

func TestTraceConvertDiagnosticCoverageWitnessSidebandSurvivesOversizedCoverage(t *testing.T) {
	const durationWitness = "row_id=1/tid=101/itid=1/ts_ns=1000/reason=invalid_duration/dur=text_bytes=3/b64=MTAw"
	body := string(traceConvertDiagnosticReportBody(
		hitraceconv.Options{InputPath: "capture.sys"},
		hitraceconv.Result{
			TraceDBCoverage: []hitraceconv.TraceDBCoverage{{
				Family:      "slice",
				Table:       "callstack",
				Role:        "query_ready_export",
				RowsRead:    321,
				RowsEmitted: 123,
				ElapsedUS:   456789,
				Skipped:     "localized exact rejection",
				Metadata: map[string]string{
					"raw_async_mismatch_witnesses":                                strings.Repeat("oversized-neighbor;", 800),
					"rejected_callstack_fence_witnesses":                          durationWitness,
					"raw_marker_local_validation_witnesses":                       "emitter=1/start_ns=2/end_ns=3/reason=invalid_begin_payload_pid",
					"official_viewer_typed_only_sync_witnesses_cpu_unknown_start": "reason=cpu_unknown_start/tid=7/start_ns=8/end_ns=9",
				},
			}},
		},
		traceConvertDiagnosticProgressLog{},
		nil,
	))
	for _, want := range []string{
		"coverage_witness_sideband_v1",
		"coverage_receipt_sideband_v1",
		"trace_db_coverage_receipt[0]=",
		`"elapsed_us":456789`,
		`"rows_emitted":123`,
		`"rows_read":321`,
		`"skipped":"localized exact rejection"`,
		"trace_db_coverage_witness[0].rejected_callstack_fence_witnesses=",
		durationWitness,
		"trace_db_coverage_witness[0].raw_async_mismatch_witnesses=",
		"trace_db_coverage_witness[0].raw_marker_local_validation_witnesses=",
		"trace_db_coverage_witness[0].official_viewer_typed_only_sync_witnesses_cpu_unknown_start=",
		"reason=cpu_unknown_start/tid=7/start_ns=8/end_ns=9",
		"<truncated original_bytes=",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("sideband diagnostic missing %q:\n%s", want, body)
		}
	}
	if got := bytes.Count([]byte(body), []byte("\n")); got > traceConvertDiagnosticReportMaxLines {
		t.Fatalf("sideband diagnostic exceeded line limit: got=%d", got)
	}
	for _, line := range strings.Split(body, "\n") {
		if !strings.HasPrefix(line, "trace_db_coverage_receipt[0]=") {
			continue
		}
		var receipt map[string]any
		if err := json.Unmarshal(
			[]byte(strings.TrimPrefix(
				line, "trace_db_coverage_receipt[0]=")),
			&receipt); err != nil {
			t.Fatalf("coverage receipt is not valid JSON: %v line=%q",
				err, line)
		}
		return
	}
	t.Fatal("coverage receipt sideband missing")
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
