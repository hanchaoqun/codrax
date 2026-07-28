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
		`build_identity={"revision":`,
		`"executable_hash_status":"available"`,
		`diagnostic_capabilities=["sql_mixed_precision_wire_sort_v1","clock_regression_first_witness_v1","callstack_exact_name_v1","source_cmdline_official_rawtrace_v1","capture_issue_semantics_v1","callstack_official_field_semantics_v1","callstack_time_local_fence_v1","standard_sync_pipe_compat_v1","callstack_completed_async_interval_v1","source_rawtrace_authority_inventory_v1","executable_build_fingerprint_v1","unresolved_trace_identity_witnesses_v1","official_raw_page_profile_probe_v1","official_raw_record_decode_ledger_v1","official_raw_record_reconciliation_v2","official_raw_blocked_key_ledger_v1","official_raw_blocked_recovery_v1","official_raw_scheduler_lite_decode_v1","official_raw_scheduler_lite_join_v1","official_raw_scheduler_lite_common_pid_nonidentity_v1","official_raw_scheduler_lite_decision_diagnostics_v1","official_raw_record_decode_budget_v2","official_raw_scheduler_lite_wakeup_join_v1","official_raw_dma_wait_recovery_v1","official_raw_marker_endpoint_ledger_v1","official_raw_marker_sync_recovery_v1","official_raw_marker_action_census_v1","official_raw_marker_print_legacy_v1","official_raw_marker_print_compact_v1","official_raw_scheduler_lite_geometry_v1","official_raw_scheduler_wakeup_new_geometry_v1","official_raw_scheduler_wakeup_new_name_v1","official_raw_scheduler_compact_profile_v1","official_raw_blocked_subject_census_v1","official_raw_signed_char_array_string_v1","official_raw_marker_name_drift_fence_v1","official_raw_marker_local_pair_fence_v1","official_raw_marker_post_fence_dedup_v1","raw_marker_replacement_closure_v1","raw_marker_pair_diagnostics_v1","task_pool_complete_pair_v1","completed_async_generic_viewer_caveat_v1"]`,
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
