package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"unicode/utf8"

	"github.com/hanchaoqun/codrax/internal/hitraceconv"
)

const (
	traceConvertDiagnosticReportProfile = "codrax_trace_conversion_diagnostic/v1"
	// Keep a deliberate margin below the customer collection limit of 1000
	// physical lines. The final receipt line is included in this budget.
	traceConvertDiagnosticReportMaxLines = 900
	traceConvertDiagnosticLineMaxBytes   = 8192
	traceConvertDiagnosticProgressHead   = 64
	traceConvertDiagnosticProgressTail   = 64
)

var traceConvertDiagnosticCapabilities = []string{
	"sql_mixed_precision_wire_sort_v1",
	"clock_regression_first_witness_v1",
	"callstack_exact_name_v1",
	"source_cmdline_official_rawtrace_v1",
	"capture_issue_semantics_v1",
	"callstack_official_field_semantics_v1",
	"callstack_time_local_fence_v1",
	"callstack_local_fence_witness_v1",
	"callstack_rejected_scalar_witness_v1",
	"coverage_witness_sideband_v1",
	"coverage_receipt_sideband_v1",
	"null_duration_raw_closure_census_v1",
	"raw_marker_local_validation_witness_v1",
	"official_raw_marker_zero_pid_header_identity_v1",
	"official_raw_marker_trailing_space_name_v1",
	"official_raw_marker_print_parser_trailing_space_v2",
	"official_raw_marker_print_parser_trailing_space_v3",
	"null_duration_raw_disposition_census_v1",
	"raw_marker_local_validation_reason_witness_v1",
	"official_viewer_typed_only_reason_census_v1",
	"official_viewer_typed_only_final_witness_v1",
	"official_viewer_typed_only_final_witness_v2",
	"standard_sync_pipe_compat_v1",
	"callstack_completed_async_interval_v1",
	"source_rawtrace_authority_inventory_v1",
	"source_rawtrace_partial_decoder_authority_v1",
	"executable_build_fingerprint_v1",
	"unresolved_trace_identity_witnesses_v1",
	"official_raw_page_profile_probe_v1",
	"official_raw_record_decode_ledger_v1",
	"official_raw_record_reconciliation_v2",
	"official_raw_blocked_key_ledger_v1",
	"official_raw_blocked_recovery_v1",
	"official_raw_blocked_family_authority_v1",
	"official_raw_scheduler_lite_decode_v1",
	"official_raw_scheduler_lite_join_v1",
	"official_raw_scheduler_lite_common_pid_nonidentity_v1",
	"official_raw_scheduler_lite_decision_diagnostics_v1",
	"official_raw_scheduler_lite_unmatched_publication_v1",
	"official_raw_scheduler_cpu_fallback_v1",
	"official_raw_scheduler_cpu_lookup_census_v1",
	"scheduler_publication_reconciliation_v1",
	"official_raw_record_decode_budget_v2",
	"official_raw_record_decode_retained_bytes_v3",
	"official_raw_record_family_authority_v1",
	"official_raw_record_decode_elapsed_v1",
	"official_raw_scheduler_lite_wakeup_join_v1",
	"official_raw_dma_wait_recovery_v1",
	"official_raw_dma_lifecycle_point_recovery_v1",
	"official_raw_dma_lifecycle_point_recovery_v2",
	"official_raw_marker_endpoint_ledger_v1",
	"official_raw_marker_sync_recovery_v1",
	"official_raw_marker_local_segment_fence_v1",
	"official_raw_marker_action_census_v1",
	"official_raw_marker_print_legacy_v1",
	"official_raw_marker_print_compact_v1",
	"official_raw_scheduler_lite_geometry_v1",
	"official_raw_scheduler_wakeup_new_geometry_v1",
	"official_raw_scheduler_wakeup_new_name_v1",
	"official_raw_scheduler_compact_profile_v1",
	"official_raw_blocked_subject_census_v1",
	"official_raw_signed_char_array_string_v1",
	"official_raw_marker_name_drift_fence_v1",
	"official_raw_marker_local_pair_fence_v1",
	"official_raw_marker_post_fence_dedup_v1",
	"raw_marker_replacement_closure_v1",
	"raw_marker_pair_diagnostics_v1",
	"task_pool_complete_pair_v1",
	"completed_async_generic_viewer_caveat_v1",
	"official_raw_marker_async_recovery_v1",
	"official_raw_marker_async_join_diagnostics_v1",
	"official_raw_marker_async_mismatch_witness_v1",
	"raw_marker_cpu_unavailable_collision_census_v1",
	"raw_marker_cpu_unavailable_replacement_v1",
	"raw_marker_combined_collision_census_v1",
	"thread_registration_metadata_only_v1",
	"sql_text_fidelity_v1",
	"sql_text_fidelity_v2",
	"official_frame_callstack_relation_v1",
	"official_frame_gpu_relation_v1",
	"official_perf_napi_async_relation_v1",
	"official_ebpf_interval_v1",
}

type traceConvertDiagnosticProgressLog struct {
	total int
	head  []hitraceconv.ProgressEvent
	tail  []hitraceconv.ProgressEvent
}

func (log *traceConvertDiagnosticProgressLog) Add(event hitraceconv.ProgressEvent) {
	if log == nil {
		return
	}
	log.total++
	if len(log.head) < traceConvertDiagnosticProgressHead {
		log.head = append(log.head, event)
		return
	}
	log.tail = append(log.tail, event)
	if len(log.tail) > traceConvertDiagnosticProgressTail {
		copy(log.tail, log.tail[len(log.tail)-traceConvertDiagnosticProgressTail:])
		log.tail = log.tail[:traceConvertDiagnosticProgressTail]
	}
}

func (log traceConvertDiagnosticProgressLog) Events() []hitraceconv.ProgressEvent {
	events := make([]hitraceconv.ProgressEvent, 0, len(log.head)+len(log.tail))
	events = append(events, log.head...)
	events = append(events, log.tail...)
	return events
}

func (log traceConvertDiagnosticProgressLog) Omitted() int {
	omitted := log.total - len(log.head) - len(log.tail)
	if omitted < 0 {
		return 0
	}
	return omitted
}

type traceConvertDiagnosticReportFile struct {
	path string
	file *os.File
}

func openTraceConvertDiagnosticReport(path, input, output, dbOutput string) (*traceConvertDiagnosticReportFile, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, nil
	}
	candidates := []struct {
		label string
		path  string
	}{
		{label: "trace input", path: input},
		{label: "systrace output", path: firstNonEmptyTraceConvertPath(output, hitraceconv.DefaultOutputPath(input))},
		{label: "trace DB output", path: dbOutput},
	}
	for _, candidate := range candidates {
		same, err := traceConvertPathsEqual(path, candidate.path)
		if err != nil {
			return nil, fmt.Errorf("resolve --diagnostic-report path against %s: %w", candidate.label, err)
		}
		if same {
			return nil, fmt.Errorf("--diagnostic-report must not alias %s: %s", candidate.label, path)
		}
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return nil, fmt.Errorf("create --diagnostic-report %s: %w", path, err)
	}
	return &traceConvertDiagnosticReportFile{path: path, file: file}, nil
}

func firstNonEmptyTraceConvertPath(value, fallback string) string {
	if strings.TrimSpace(value) != "" {
		return value
	}
	return fallback
}

func traceConvertPathsEqual(left, right string) (bool, error) {
	if strings.TrimSpace(right) == "" {
		return false, nil
	}
	leftAbs, err := filepath.Abs(left)
	if err != nil {
		return false, err
	}
	rightAbs, err := filepath.Abs(right)
	if err != nil {
		return false, err
	}
	leftAbs = filepath.Clean(leftAbs)
	rightAbs = filepath.Clean(rightAbs)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(leftAbs, rightAbs), nil
	}
	return leftAbs == rightAbs, nil
}

func (report *traceConvertDiagnosticReportFile) Path() string {
	if report == nil {
		return ""
	}
	return report.path
}

func (report *traceConvertDiagnosticReportFile) Write(body []byte) error {
	if report == nil || report.file == nil {
		return nil
	}
	if _, err := report.file.Write(body); err != nil {
		return fmt.Errorf("write --diagnostic-report %s: %w", report.path, err)
	}
	if err := report.file.Sync(); err != nil {
		return fmt.Errorf("sync --diagnostic-report %s: %w", report.path, err)
	}
	if err := report.file.Close(); err != nil {
		report.file = nil
		return fmt.Errorf("close --diagnostic-report %s: %w", report.path, err)
	}
	report.file = nil
	return nil
}

func (report *traceConvertDiagnosticReportFile) Close() error {
	if report == nil || report.file == nil {
		return nil
	}
	err := report.file.Close()
	report.file = nil
	return err
}

type traceConvertDiagnosticLineSet struct {
	lines   []string
	omitted int
}

func (set *traceConvertDiagnosticLineSet) Add(line string) {
	if set == nil {
		return
	}
	if len(set.lines) >= traceConvertDiagnosticReportMaxLines-1 {
		set.omitted++
		return
	}
	set.lines = append(set.lines, traceConvertDiagnosticBoundedLine(line))
}

func (set *traceConvertDiagnosticLineSet) Bytes() []byte {
	if set == nil {
		return nil
	}
	physicalLines := len(set.lines) + 1
	set.lines = append(set.lines, fmt.Sprintf(
		"report_receipt profile=%s physical_lines=%d hard_limit=%d omitted_records=%d",
		traceConvertDiagnosticReportProfile,
		physicalLines,
		traceConvertDiagnosticReportMaxLines,
		set.omitted,
	))
	return []byte(strings.Join(set.lines, "\n") + "\n")
}

func traceConvertDiagnosticReportBody(
	opts hitraceconv.Options,
	result hitraceconv.Result,
	progress traceConvertDiagnosticProgressLog,
	conversionErr error,
) []byte {
	var lines traceConvertDiagnosticLineSet
	outcome := "success"
	if conversionErr != nil {
		outcome = "failure"
	}
	lines.Add(traceConvertDiagnosticReportProfile)
	buildIdentity := resolveCodraxBuildIdentity()
	lines.Add(fmt.Sprintf(
		"outcome=%s codrax_version=%s build_time=%s build_revision=%s build_revision_source=%s goos=%s goarch=%s",
		outcome,
		version,
		buildTime,
		buildIdentity.Revision,
		buildIdentity.RevisionSource,
		runtime.GOOS,
		runtime.GOARCH,
	))
	lines.Add(traceConvertDiagnosticJSONLine("build_identity", buildIdentity))
	lines.Add(traceConvertDiagnosticJSONLine("diagnostic_capabilities", traceConvertDiagnosticCapabilities))
	lines.Add(traceConvertDiagnosticJSONLine("options", map[string]any{
		"input_path":           opts.InputPath,
		"output_path":          opts.OutputPath,
		"archive_member":       opts.ArchiveMember,
		"flavor":               opts.Flavor,
		"trace_engine":         opts.TraceEngine,
		"trace_streamer_path":  opts.TraceStreamerPath,
		"trace_db_output_path": opts.TraceDBOutputPath,
		"keep_trace_db":        opts.KeepTraceDB,
		"perf_parser":          opts.PerfParser,
		"no_perftrace":         opts.DisablePerfAdapter,
	}))
	lines.Add(traceConvertDiagnosticJSONLine("result", map[string]any{
		"input_path":           result.InputPath,
		"output_path":          result.OutputPath,
		"bundle_path":          result.BundlePath,
		"input_bytes":          result.InputBytes,
		"output_bytes":         result.OutputBytes,
		"events_written":       result.EventsWritten,
		"missing_format_count": result.MissingFormatCount,
		"unknown_event_count":  result.UnknownEventCount,
		"first_timestamp_sec":  result.FirstTimestampSec,
		"last_timestamp_sec":   result.LastTimestampSec,
	}))
	lines.Add(fmt.Sprintf("progress_total=%d progress_emitted=%d progress_omitted=%d",
		progress.total, len(progress.Events()), progress.Omitted()))
	for index, event := range progress.Events() {
		lines.Add(traceConvertDiagnosticJSONLine(fmt.Sprintf("progress[%d]", index), map[string]any{
			"stage":       event.Stage,
			"status":      event.Status,
			"message":     event.Message,
			"path":        event.Path,
			"output_path": event.OutputPath,
			"bytes_done":  event.BytesDone,
			"bytes_total": event.BytesTotal,
			"records":     event.Records,
			"elapsed_ns":  event.Elapsed.Nanoseconds(),
		}))
	}
	if conversionErr != nil {
		appendTraceConvertDiagnosticError(&lines, conversionErr)
	}
	if result.ArchiveProvenance != nil {
		lines.Add(traceConvertDiagnosticJSONLine("archive_provenance", result.ArchiveProvenance))
	}
	for index, decision := range result.TraceDecisions {
		lines.Add(traceConvertDiagnosticJSONLine(fmt.Sprintf("trace_provider_decision[%d]", index), decision))
	}
	for index, decision := range result.ProviderDecisions {
		lines.Add(traceConvertDiagnosticJSONLine(fmt.Sprintf("perf_provider_decision[%d]", index), decision))
	}
	for index, artifact := range result.Artifacts {
		lines.Add(traceConvertDiagnosticJSONLine(fmt.Sprintf("artifact[%d]", index), artifact))
	}
	for index, coverage := range result.TraceCoverage {
		lines.Add(traceConvertDiagnosticJSONLine(fmt.Sprintf("trace_coverage[%d]", index), coverage))
	}
	for index, coverage := range result.TraceDBCoverage {
		appendTraceConvertDiagnosticCoverageWitnesses(&lines, index, coverage)
		coverageLine := traceConvertDiagnosticJSONLine(
			fmt.Sprintf("trace_db_coverage[%d]", index), coverage)
		appendTraceConvertDiagnosticCoverageReceipt(
			&lines, index, coverage, coverageLine)
		lines.Add(coverageLine)
	}
	for index, caveat := range result.Caveats {
		lines.Add(traceConvertDiagnosticJSONLine(fmt.Sprintf("caveat[%d]", index), caveat))
	}
	return lines.Bytes()
}

func appendTraceConvertDiagnosticCoverageReceipt(
	lines *traceConvertDiagnosticLineSet,
	index int,
	coverage hitraceconv.TraceDBCoverage,
	fullLine string,
) {
	if lines == nil || len(fullLine) <= traceConvertDiagnosticLineMaxBytes {
		return
	}
	receipt := map[string]any{
		"family":             coverage.Family,
		"table":              coverage.Table,
		"role":               coverage.Role,
		"found":              coverage.Found,
		"rows_read":          coverage.RowsRead,
		"rows_emitted":       coverage.RowsEmitted,
		"elapsed_us":         coverage.ElapsedUS,
		"metric_count":       len(coverage.Metrics),
		"metadata_count":     len(coverage.Metadata),
		"field_source_count": len(coverage.FieldSources),
		"full_line_bytes":    len(fullLine),
		"skipped_present":    coverage.Skipped != "",
		"error_present":      coverage.Error != "",
	}
	if coverage.Skipped != "" {
		receipt["skipped"] =
			traceConvertDiagnosticCoverageReceiptValue(coverage.Skipped)
	}
	if coverage.Error != "" {
		receipt["error"] =
			traceConvertDiagnosticCoverageReceiptValue(coverage.Error)
	}
	lines.Add(traceConvertDiagnosticJSONLine(
		fmt.Sprintf("trace_db_coverage_receipt[%d]", index), receipt))
}

func traceConvertDiagnosticCoverageReceiptValue(value string) string {
	const maximumBytes = 512
	if len(value) <= maximumBytes && utf8.ValidString(value) {
		return value
	}
	return fmt.Sprintf("<omitted original_bytes=%d>", len(value))
}

var traceConvertDiagnosticCoverageWitnessKeys = []string{
	"rejected_callstack_fence_witnesses",
	"raw_async_mismatch_witnesses",
	"raw_marker_local_validation_witnesses",
	"official_viewer_typed_only_sync_witnesses_cpu_unknown_start",
	"official_viewer_typed_only_sync_witnesses_cpu_unknown_end",
	"official_viewer_typed_only_sync_witnesses_cpu_source_tainted",
	"official_viewer_typed_only_sync_witnesses_cpu_lifecycle_rejected",
	"official_viewer_typed_only_sync_witnesses_cpu_alias_ambiguous",
	"official_viewer_typed_only_sync_witnesses_name_unrepresentable",
}

func appendTraceConvertDiagnosticCoverageWitnesses(
	lines *traceConvertDiagnosticLineSet,
	index int,
	coverage hitraceconv.TraceDBCoverage,
) {
	if lines == nil || len(coverage.Metadata) == 0 {
		return
	}
	for _, key := range traceConvertDiagnosticCoverageWitnessKeys {
		value := coverage.Metadata[key]
		if value == "" {
			continue
		}
		lines.Add(traceConvertDiagnosticJSONLine(
			fmt.Sprintf("trace_db_coverage_witness[%d].%s", index, key),
			map[string]string{
				"family": coverage.Family,
				"table":  coverage.Table,
				"value":  value,
			},
		))
	}
}

func appendTraceConvertDiagnosticError(lines *traceConvertDiagnosticLineSet, conversionErr error) {
	lines.Add(traceConvertDiagnosticJSONLine("error", map[string]any{
		"type":    fmt.Sprintf("%T", conversionErr),
		"message": conversionErr.Error(),
	}))
	var inputErr *hitraceconv.ConversionInputError
	if errors.As(conversionErr, &inputErr) {
		lines.Add(traceConvertDiagnosticJSONLine("typed_error_conversion_input", map[string]any{
			"code":  inputErr.Code,
			"stage": inputErr.Stage,
			"path":  inputErr.Path,
		}))
	}
	var providerErr *hitraceconv.TraceProviderFailureError
	if errors.As(conversionErr, &providerErr) {
		lines.Add(traceConvertDiagnosticJSONLine("typed_error_trace_provider", map[string]any{
			"decision":       providerErr.Decision,
			"source":         providerErr.Source,
			"path":           providerErr.Path,
			"stage":          providerErr.Stage,
			"code":           providerErr.Code,
			"caveats":        providerErr.Caveats,
			"rolled_back_db": providerErr.RolledBackDB,
		}))
	}
	var fallbackErr *hitraceconv.TraceProviderFallbackError
	if errors.As(conversionErr, &fallbackErr) {
		lines.Add(traceConvertDiagnosticJSONLine("typed_error_trace_fallback", map[string]any{
			"first_decision": fallbackErr.FirstDecision,
			"first_source":   fallbackErr.FirstSource,
			"first_path":     fallbackErr.FirstPath,
			"first_stage":    fallbackErr.FirstStage,
			"first_code":     fallbackErr.FirstCode,
			"first_caveats":  fallbackErr.FirstCaveats,
			"rolled_back_db": fallbackErr.RolledBackDB,
			"fallback_error": fmt.Sprint(fallbackErr.Fallback),
			"first_error":    fmt.Sprint(fallbackErr.FirstCause),
		}))
	}
	var builtinErr *hitraceconv.BuiltinSysDecodeError
	if errors.As(conversionErr, &builtinErr) {
		lines.Add(traceConvertDiagnosticJSONLine("typed_error_builtin_sys", map[string]any{
			"code":         builtinErr.Code,
			"file_type":    builtinErr.FileType,
			"magic":        builtinErr.Magic,
			"version":      builtinErr.Version,
			"header_bytes": builtinErr.HeaderBytes,
			"segment_type": builtinErr.SegmentType,
			"offset":       builtinErr.Offset,
			"detail":       builtinErr.Detail,
		}))
	}
	var archiveErr *hitraceconv.TraceArchiveError
	if errors.As(conversionErr, &archiveErr) {
		lines.Add(traceConvertDiagnosticJSONLine("typed_error_archive", map[string]any{
			"code":   archiveErr.Code,
			"member": archiveErr.Member,
		}))
	}
	var clockRegression *hitraceconv.TraceClockRegressionWitnessError
	if errors.As(conversionErr, &clockRegression) {
		lines.Add(traceConvertDiagnosticJSONLine("typed_error_clock_regression", map[string]any{
			"previous_line":          clockRegression.PreviousLine,
			"current_line":           clockRegression.CurrentLine,
			"previous_timestamp_sec": clockRegression.PreviousTimestampSec,
			"current_timestamp_sec":  clockRegression.CurrentTimestampSec,
			"previous_event_type":    clockRegression.PreviousEventType,
			"current_event_type":     clockRegression.CurrentEventType,
		}))
	}
}

func traceConvertDiagnosticJSONLine(label string, value any) string {
	body, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprintf("%s={\"marshal_error\":%q}", label, err.Error())
	}
	return label + "=" + string(body)
}

func traceConvertDiagnosticBoundedLine(line string) string {
	line = strings.NewReplacer(
		"\r", `\r`,
		"\n", `\n`,
		"\t", `\t`,
		"\x00", `\0`,
	).Replace(line)
	if len(line) <= traceConvertDiagnosticLineMaxBytes {
		return line
	}
	suffix := fmt.Sprintf("...<truncated original_bytes=%d>", len(line))
	limit := traceConvertDiagnosticLineMaxBytes - len(suffix)
	if limit < 0 {
		return suffix
	}
	for limit > 0 && !utf8.ValidString(line[:limit]) {
		limit--
	}
	return line[:limit] + suffix
}

func traceConvertDiagnosticReportWrittenLine(lang, path string) string {
	if traceConvertUseZh(lang) {
		return "转换诊断摘要已写入：" + path + "（最多 900 行）"
	}
	return "conversion diagnostic report written: " + path + " (maximum 900 lines)"
}
